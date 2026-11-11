package main

import (
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/yandy-r/dotfiles/internal/plasma"
)

// This file holds the four verb orchestrations of `dotf plasma` and the shared
// per-run context they hang off. The flag surface, verb parse, and host gates
// live in plasma.go; the key=value line builders live in plasmaprint.go and the
// --json document shape and its builders in plasmajson.go.

// plasmaRun bundles one invocation's resolved inputs so the four verb methods
// share a single, typed context instead of a nine-parameter signature.
//
// stdin is injected rather than read from os.Stdin at the point of use so the
// interactive confirmation — the only exit-130 path in this command — is
// drivable from a test without a pseudo-terminal.
type plasmaRun struct {
	stdout, stderr io.Writer
	stdin          io.Reader
	eng            *plasma.Engine
	files          []plasma.FileSpec
	o              *plasmaOpts
	popts          plasma.Options
	verb           string
}

// header returns the three lines every verb leads with: the dry-run marker (a
// dry run's FIRST line), the verb, and the detected session state.
func (p *plasmaRun) header() []string {
	lines := make([]string, 0, 3)
	if p.o.dryRun {
		lines = append(lines, "dry-run=true")
	}
	return append(lines,
		"plasma.verb="+p.verb,
		"plasma.session="+p.popts.Session.String(),
	)
}

// report builds the --json document shared by every verb, with the per-verb
// Files rows and Summary supplied by the caller.
func (p *plasmaRun) report(files []plasmaFileReport, reloads []string, sum plasmaSummary) *plasmaReport {
	if files == nil {
		files = []plasmaFileReport{}
	}
	return &plasmaReport{
		Command: "plasma",
		Verb:    p.verb,
		DryRun:  p.o.dryRun,
		Session: p.popts.Session.String(),
		Files:   files,
		Reloads: reloads,
		Summary: sum,
	}
}

// fail is this run's error exit: the JSON error document on stdout under
// --json, the diagnostic on stderr, exit 1. Every verb routes its engine
// failures through it so `--json` never answers with zero bytes.
func (p *plasmaRun) fail(err error) int {
	return plasmaFail(p.stdout, p.stderr, p.o, p.verb, p.popts.Session, err)
}

// emit writes the run's stdout: the JSON document when --json is set, otherwise
// the assembled key=value lines. --json REPLACES the key=value output; it never
// augments it and never changes the exit code.
func (p *plasmaRun) emit(lines []string, doc *plasmaReport) error {
	if p.o.jsonOut {
		return writeJSON(p.stdout, doc)
	}
	return writePlasmaLines(p.stdout, lines)
}

// advise writes the run's non-fatal stderr advisories, before any summary.
//
// They are suppressed under --dry-run: a preview's stderr is asserted empty by
// the CLI tests, and an advisory about what a real run would encounter is noise
// in a preview that encounters nothing.
func (p *plasmaRun) advise(plan *plasma.Plan) {
	if p.o.dryRun {
		return
	}
	for _, advisory := range plan.Advisories {
		_, _ = fmt.Fprintf(p.stderr, "%s: %s\n", progName, advisory)
	}
	if p.popts.Session != plasma.SessionLive {
		_, _ = fmt.Fprintf(p.stderr, "%s: plasma: no KDE session detected; applying to files only\n", progName)
		return
	}
	// Once per FILE, not once per planned ACTION: plan.Files carries a --backup
	// entry alongside the mutating one, so keying the loop on the slice printed
	// the same sentence twice for the same path and it read as two independent
	// problems.
	warned := make(map[string]bool, len(plan.Files))
	for _, act := range plan.Files {
		if !act.Spec.SessionOwned || act.Kind == plasma.ActionSkip || warned[act.Spec.Path] {
			continue
		}
		warned[act.Spec.Path] = true
		_, _ = fmt.Fprintf(p.stderr, "%s: plasma: %s\n", progName,
			plasmaSessionHazard(p.verb, act.Spec.Path))
	}
}

// plasmaSessionHazard phrases the SessionOwned notice for the verb's write
// DIRECTION, because the two directions have opposite hazards and only one of
// them was ever described.
//
// apply writes the live file, which the session then rewrites over — that is the
// "may be overwritten" warning. export writes plasma/files/, which no KDE
// process opens, so nothing can overwrite it; what export actually risks is the
// opposite, that the session is still holding settings in memory and the
// captured snapshot predates their sync. Sharing the apply wording with export
// warned about a process that cannot touch what export just wrote.
func plasmaSessionHazard(verb, path string) string {
	if verb == "export" {
		return path + " is rewritten by the running session; the captured snapshot " +
			"may miss settings it has not written to disk yet"
	}
	return path + " is rewritten by the running session; changes may be overwritten " +
		"before you log out"
}

// export runs the live → repo capture: strip each live file's volatile state and
// write the canonical form into plasma/files/.
func (p *plasmaRun) export() int {
	plan, err := p.eng.PlanExport(p.files, p.popts)
	if err != nil {
		return p.fail(err)
	}
	p.advise(plan)

	if p.o.dryRun {
		lines := append(p.header(), plasmaActionLines("plasma", plan.Files)...)
		sum := plasmaSummary{Managed: len(p.files), Planned: len(plan.Files)}
		if err := p.emit(lines, p.report(plasmaFileReports(plan.Files), nil, sum)); err != nil {
			return reportErr(p.stderr, err)
		}
		return 0
	}

	rep, execErr := p.eng.Execute(plan, p.popts)
	failures := plasmaErrors(execErr)
	reported := plasmaExecutedActions(rep)

	lines := append(p.header(), plasmaActionLines("plasma", reported)...)
	sum := plasmaSummary{
		Managed: len(p.files),
		Planned: len(rep.Planned),
		Applied: len(rep.Applied),
		Failed:  len(failures),
	}
	doc := p.report(plasmaFileReports(reported), nil, sum)
	doc.Failures = plasmaFailureStrings(failures)
	if err := p.emit(lines, doc); err != nil {
		return reportErr(p.stderr, err)
	}

	printPlasmaSummary(p.stderr, len(rep.Applied), len(rep.Planned))
	printPlasmaFailures(p.stderr, failures)
	if len(failures) > 0 {
		return 1
	}
	return 0
}

// apply runs the repo → live merge: every key in the repo file is written to the
// live file and every live-only key is preserved. It confirms before writing,
// then optionally reloads the affected KDE components and always validates.
func (p *plasmaRun) apply() int {
	plan, err := p.eng.PlanApply(p.files, p.popts)
	if err != nil {
		return p.fail(err)
	}
	p.advise(plan)

	if p.o.dryRun {
		return p.applyPreview(plan)
	}

	if code, ok := p.confirm(); !ok {
		return code
	}

	rep, execErr := p.eng.Execute(plan, p.popts)
	failures := plasmaErrors(execErr)

	ran, reloadErr := p.reload(plan)
	failures = append(failures, plasmaErrors(reloadErr)...)
	// Report.Reloaded is documented as "the reload commands actually run" and no
	// engine path fills it. p.reload is the only caller that holds the answer,
	// so it lands here and everything downstream reads the resolved set from the
	// report rather than re-deriving it from the plan.
	rep.Reloaded = ran
	reloads := plasmaResolvedReloads(plan.Reloads, rep.Reloaded)

	reported := plasmaExecutedActions(rep)
	lines := append(p.header(), plasmaActionLines("plasma", reported)...)
	// The change line stays on Applied, not on the reported set: it names the
	// keys that were actually WRITTEN, and a planned-but-refused merge carries
	// plan-time keys that never landed.
	if line := plasmaChangeLine(rep.Applied); line != "" {
		lines = append(lines, line)
	}
	lines = append(lines, plasmaReloadLines(reloads)...)

	// Validate is the post-apply audit and resolves to the same Diff comparison
	// PlanApply used, so "applied cleanly" and "converged" can never disagree.
	//
	// Its DriftError is REPORTED, never counted as a failure: post-apply drift
	// is normally the consequence of a per-file failure already in `failures`,
	// so folding it in reported one problem as two. It still forces exit 1.
	var driftErr error
	driftFiles, driftKeys := 0, 0
	if diffs, verr := p.eng.Diff(p.files, p.popts); verr != nil {
		var drift *plasma.DriftError
		if !errors.As(verr, &drift) {
			return p.fail(verr)
		}
		driftErr = verr
		driftFiles, driftKeys, _ = plasmaDriftTally(diffs)
	}

	sum := plasmaSummary{
		Managed:    len(p.files),
		Planned:    len(rep.Planned),
		Applied:    len(rep.Applied),
		DriftFiles: driftFiles,
		DriftKeys:  driftKeys,
		Failed:     len(failures),
	}
	doc := p.report(plasmaFileReports(reported), plasmaReloadStrings(reloads), sum)
	doc.Failures = plasmaFailureStrings(failures)
	if err := p.emit(lines, doc); err != nil {
		return reportErr(p.stderr, err)
	}

	printPlasmaSummary(p.stderr, len(rep.Applied), len(rep.Planned))
	printPlasmaFailures(p.stderr, failures)
	if driftErr != nil {
		printPlasmaFailures(p.stderr, []error{driftErr})
	}
	if len(failures) > 0 || driftErr != nil {
		return 1
	}
	return 0
}

// plasmaExecutedActions is the action set a MUTATING verb reports: the whole
// plan, with each entry replaced by its executed form where Execute produced
// one.
//
// Reporting Report.Applied alone made the run's file set depend on --dry-run.
// Applied excludes every ActionSkip, and a refused write IS a skip, so the one
// file worth reporting — the immutability refusal — vanished from stdout and
// from the --json files array while the same verb's --dry-run rendered it. The
// plan is the honest set; summary.applied still counts only what landed.
//
// Applied is an in-order subsequence of Planned and Execute never changes an
// applied action's Kind (a write-time downgrade to ActionSkip is dropped from
// Applied rather than carried into it), so a two-pointer walk pairs the two
// slices exactly, without inventing a key.
func plasmaExecutedActions(rep *plasma.Report) []plasma.FileAction {
	out := make([]plasma.FileAction, 0, len(rep.Planned))
	next := 0
	for _, act := range rep.Planned {
		if next < len(rep.Applied) &&
			rep.Applied[next].Spec.Path == act.Spec.Path &&
			rep.Applied[next].Kind == act.Kind {
			out = append(out, rep.Applied[next])
			next++
			continue
		}
		out = append(out, act)
	}
	return out
}

// applyPreview prints the apply plan and ALWAYS returns 0. A dry run is a
// preview, not a drift detector: drift is visible on stdout as the merge/create
// buckets and the plasma.change= line, so the exit code stays clean (the
// `dotf link --dry-run` rule). `dotf plasma diff` is the drift detector.
//
// Plan.LinkConflicts is the one thing a preview must report that is not a
// planned action: --dry-run itself is what authorised those mode transitions,
// so the same invocation without it would be REFUSED with exit 1. Printing the
// plan and staying silent about that made "--dry-run to preview" advice that
// previewed the wrong run.
func (p *plasmaRun) applyPreview(plan *plasma.Plan) int {
	lines := append(p.header(), plasmaActionLines("plasma", plan.Files)...)
	if line := plasmaChangeLine(plan.Files); line != "" {
		lines = append(lines, line)
	}
	lines = append(lines, plasmaLinkConflictLines(plan.LinkConflicts)...)
	lines = append(lines, plasmaReloadLines(plan.Reloads)...)

	sum := plasmaSummary{Managed: len(p.files), Planned: len(plan.Files)}
	doc := p.report(plasmaFileReports(plan.Files), plasmaReloadStrings(plan.Reloads), sum)
	doc.LinkConflicts = plan.LinkConflicts
	if err := p.emit(lines, doc); err != nil {
		return reportErr(p.stderr, err)
	}
	return 0
}

// reload runs the plan's reload commands when --reload was given and a live KDE
// session is present, and returns the commands that RAN. Requesting a reload
// without a session is an advisory, not a failure: the settings still landed and
// are picked up at next login.
//
// The return value is not decoration. Engine.Reload resolves each command — the
// kwin reconfigure comes from a candidate table (busctl first, then the qdbus
// variants, whose binary name differs per distribution), and the whole argv is
// replaced — so only the returned slice names what actually executed.
// Discarding it and printing plan.Reloads made the pasteable line unpasteable.
func (p *plasmaRun) reload(plan *plasma.Plan) ([]plasma.ReloadCommand, error) {
	if p.o.reload && p.popts.Session != plasma.SessionLive {
		_, _ = fmt.Fprintf(p.stderr,
			"%s: plasma: --reload requested but no KDE session detected; skipping\n", progName)
		return nil, nil
	}
	return p.eng.Reload(plan, p.popts)
}

// plasmaSubstitutedBinary is the PLACEHOLDER argv[0] the plan emits for the kwin
// reconfigure — the one command whose entire argv internal/plasma's resolveArgv
// may replace, from its candidate table. Every other command keeps the name the
// plan gave it, which is what lets plasmaResolvedReloads compare argv[0]
// directly everywhere else.
const plasmaSubstitutedBinary = "qdbus"

// plasmaResolvedReloads overlays the argv Engine.Reload actually resolved onto
// the planned reload set, so both renderers print what ran rather than what was
// planned, and a command that was planned but never executed is still listed.
//
// Reload iterates plan.Reloads in order and appends, so `ran` is an in-order
// subsequence of `planned`; a two-pointer walk pairs them. The predicate needs
// argv[0] as well as the tail because plasmashell contributes TWO commands with
// the same component and the same argument list (kquitapp6 / kstart), and
// pairing on component plus tail alone would print one of them twice whenever
// the other failed to resolve.
func plasmaResolvedReloads(planned, ran []plasma.ReloadCommand) []plasma.ReloadCommand {
	if len(ran) == 0 {
		return planned
	}
	out := make([]plasma.ReloadCommand, 0, len(planned))
	next := 0
	for _, cmd := range planned {
		if next < len(ran) && plasmaSameReload(cmd, ran[next]) {
			out = append(out, ran[next])
			next++
			continue
		}
		out = append(out, cmd)
	}
	return out
}

// plasmaSameReload reports whether a planned reload command and an executed one
// are the same command. Only argv[0] can differ between the two, and only when
// the plan named the binary the engine probes alternatives for.
func plasmaSameReload(planned, ran plasma.ReloadCommand) bool {
	if planned.Component != ran.Component || len(planned.Argv) != len(ran.Argv) {
		return false
	}
	if len(planned.Argv) == 0 || !slices.Equal(planned.Argv[1:], ran.Argv[1:]) {
		return false
	}
	return planned.Argv[0] == ran.Argv[0] || planned.Argv[0] == plasmaSubstitutedBinary
}

// diff reports per-key drift in both directions and exits 1 when any file has a
// difference apply would act on.
//
// Exiting 1 on drift is a DELIBERATE divergence from `dotf link --dry-run`'s
// always-0 rule: diff's whole job is signalling non-convergence to CI, exactly
// like `dotf diff`, so --dry-run is accepted and inert here (it prints the
// marker line and leaves the exit code to this rule).
func (p *plasmaRun) diff() int {
	diffs, err := p.eng.Diff(p.files, p.popts)
	var drift *plasma.DriftError
	if err != nil && !errors.As(err, &drift) {
		return p.fail(err)
	}

	lines := append(p.header(), plasmaDiffLines(diffs)...)
	sum := plasmaSummary{Managed: len(p.files)}
	sum.DriftFiles, sum.DriftKeys, sum.VolatileKeys = plasmaDriftTally(diffs)
	if err := p.emit(lines, p.report(plasmaDiffReports(diffs), nil, sum)); err != nil {
		return reportErr(p.stderr, err)
	}

	if drift != nil {
		return 1
	}
	return 0
}

// status summarises the managed set: how many files are present on each side,
// which are missing, and how much drift there is. It never fails on drift —
// reporting drift is the whole point.
func (p *plasmaRun) status() int {
	st, err := p.eng.Status(p.files, p.popts)
	if err != nil {
		return p.fail(err)
	}

	lines := append(p.header(), plasmaStatusLines(st)...)
	sum := plasmaSummary{
		Managed:    st.Managed,
		DriftFiles: len(st.DriftedFiles),
		DriftKeys:  st.DriftedKeys,
	}
	if err := p.emit(lines, p.report(plasmaStatusReports(p.files, st), nil, sum)); err != nil {
		return reportErr(p.stderr, err)
	}
	return 0
}
