package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/brew"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/failure"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/plasma"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/snapper"
	"github.com/yandy-r/dotfiles/internal/symlink"
	"github.com/yandy-r/dotfiles/internal/sysconfig"
)

// applySnapshot takes a best-effort pre-apply snapper snapshot. It writes a
// warning to stderr when snapper was requested but is unavailable, or when the
// snapshot itself fails — both are non-fatal and are NEVER recorded in the
// failure collector (parity with bootstrap's log_warning).
func applySnapshot(stderr io.Writer, r exec.Runner, info platform.Info) {
	snap := snapper.New(r, info)
	if !snap.Available() {
		_, _ = fmt.Fprintf(stderr, "%s: snapper requested but unavailable (non-Linux or snapper not installed); skipping snapshot\n", progName)
		return
	}
	if out := snap.Create("pre-apply"); out.Err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v (continuing without a snapshot)\n", progName, out.Err)
	}
}

// applyBrew bootstraps Homebrew when it is absent and applicable to this host
// (macOS always; linuxbrew when --homebrew=true), then returns a possibly
// re-detected platform.Info (macOS and x86_64 Linux select PkgMgrHomebrew; on
// aarch64 Linux the native manager wins — see platform.detectPkgMgr). A
// bootstrap that actually ran — or failed — is recorded under "brew"; a no-op
// (brew already present, or not applicable to this host) records nothing, so an
// ordinary Linux apply carries no spurious brew step. A failure is non-fatal:
// the run continues with the original info and the native manager.
func applyBrew(r exec.Runner, stderr io.Writer, info platform.Info, opts applyOptions, coll *failure.Collector) platform.Info {
	if brew.DeclinedUnsupportedArch(info, opts.Homebrew) {
		_, _ = fmt.Fprintf(stderr, "%s: --homebrew declined: Homebrew publishes no aarch64-Linux bottles; using the native package manager instead\n", progName)
	}
	bootstrapped, err := brew.Bootstrap(r, info, opts.Homebrew)
	if bootstrapped || err != nil {
		coll.Record("brew", err)
	}
	if bootstrapped {
		// The freshly-run installer does not touch this process's PATH, so add
		// the Homebrew bin dir before re-detecting: platform.Detect resolves the
		// package manager via LookPath, and the pkg backend later runs `brew
		// install` through the same PATH.
		prependPath(brew.BinDir(info))
		return platform.Detect(r)
	}
	return info
}

// prependPath prepends dir to the process PATH (idempotently — a no-op when dir
// is empty or already present), so freshly-installed tooling under dir resolves
// for the rest of the run without duplicating an existing entry.
func prependPath(dir string) {
	if dir == "" {
		return
	}
	path := os.Getenv("PATH")
	for _, p := range filepath.SplitList(path) {
		if p == dir {
			return
		}
	}
	if path == "" {
		_ = os.Setenv("PATH", dir)
		return
	}
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+path)
}

// userToolDirs are the $HOME-relative directories that provisioning installs
// executables into, most-specific-last (prependPath reverses that into
// most-specific-first). They mirror the PATH construction in common/paths.sh.
var userToolDirs = []string{
	"go/bin",
	".cargo/bin",
	".local/share/mise/shims",
	".local/bin",
}

// systemToolDirs are the absolute directories provisioning installs executables
// into. /usr/local/bin is the INSTALL_DIR every bin/install-*.sh defaults to, so
// it must be searchable even when the caller's PATH omits it.
var systemToolDirs = []string{
	"/usr/local/bin",
}

// applyUserToolPath prepends the tool directories provisioning writes into to
// this process's PATH, so a tool installed EARLIER IN THIS RUN resolves for the
// phases that follow.
//
// dotf inherits the PATH of the shell that launched it. On a fresh machine that
// PATH was built before any of these directories existed, so a shell started
// pre-provisioning never contributes them — which is how `dotf apply` installs
// mise (and with it node/npm under ~/.local/share/mise/shims), then fails the
// very next toolchain package with `exec: "npm": executable file not found in
// $PATH`. The same applies to cargo after the rust step, to ~/.local/bin, and to
// /usr/local/bin, where the bin/install-*.sh scripts drop their binaries — a
// script that cannot see its own previous install re-runs it and can fail on the
// second attempt (the AWS SAM installer refuses to overwrite).
//
// Directories that do not exist are added anyway: they are cheap non-matches for
// exec.LookPath, and a directory created mid-run must still be searched
// afterwards. Prepending is idempotent — an entry already on PATH is skipped.
func applyUserToolPath(home string) {
	for _, dir := range systemToolDirs {
		prependPath(dir)
	}
	for _, rel := range userToolDirs {
		prependPath(filepath.Join(home, rel))
	}
}

// standardDirs are the XDG/dev directories dotf ensures exist, mirroring the
// legacy bootstrap.sh create_directories() step (minus its owner-specific
// ~/Projects path). Each is created with os.MkdirAll, so a converged host is a
// no-op. Paths are relative to $HOME.
var standardDirs = []string{
	".config",
	".local/bin",
	".local/share",
	".cache",
	".zsh/completions",
	"go/src",
	"go/bin",
	"go/pkg",
}

// applyDirs ensures the standard XDG/dev directories exist under home. It records
// one "directories" outcome (the first MkdirAll error, else nil) so a failure is
// visible in the summary without aborting the run. MkdirAll is idempotent, so a
// converged host produces no change.
func applyDirs(home string, coll *failure.Collector) {
	var firstErr error
	for _, rel := range standardDirs {
		if err := os.MkdirAll(filepath.Join(home, rel), 0o755); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("create %s: %w", rel, err)
		}
	}
	coll.Record("directories", firstErr)
}

// applyIdentity plans and executes the identity sysconfig domains
// (hostname/timezone/locale/shell) with the firewall domain explicitly unset —
// firewall runs later as its own pass (applyFirewall) to honor the PRD's
// "firewall last" ordering. Each executed step's outcome is recorded per
// domain; a Plan error is recorded once under "sysconfig".
func applyIdentity(eng *sysconfig.Engine, m *manifest.Manifest, info platform.Info, coll *failure.Collector) sysconfig.Result {
	plan, err := eng.Plan(identityConfig(m), info)
	if err != nil {
		coll.Record("sysconfig", err)
		return sysconfig.Result{}
	}
	res, execErr := eng.Execute(plan)
	if execErr != nil {
		coll.Record("sysconfig", execErr)
	}
	for _, sr := range res.Applied {
		coll.Record("sysconfig:"+sr.Step.Domain, sr.Err)
	}
	return res
}

// applyPackages installs each selected package via the backend engine, recording
// one outcome per package (nil on success) and building the installResult slice
// the stdout printer consumes. Platform-inapplicable packages are skipped as
// "not-applicable" and already-installed ones as "already-installed" (unless
// force is set) — both recorded as ok, matching status/diff — so a converged
// host exits 0. A failed install never aborts the loop — later packages still
// run.
func applyPackages(ctx context.Context, engine *backend.Engine, pkgs []manifest.Package, info platform.Info, force bool, coll *failure.Collector) []installResult {
	results := make([]installResult, 0, len(pkgs))
	statuses := make(map[string]string, len(pkgs))
	for _, pkg := range pkgs {
		if ctx.Err() != nil {
			break
		}
		var res installResult
		if dep := failedPrerequisite(pkg, statuses); dep != "" {
			res = installResult{name: pkg.Name, backend: pkg.Backend, status: "failed", err: fmt.Errorf("prerequisite %q failed", dep)}
		} else {
			res = performPackage(engine, backend.ActionInstall, pkg, info, force)
		}
		statuses[pkg.Name] = res.status
		coll.Record("packages:"+pkg.Name, res.err)
		results = append(results, res)
	}
	return results
}

// applySymlinks plans, applies, and validates the static symlink table into
// $HOME. It records a single "symlinks" outcome: a Plan or Apply error, else the
// post-apply validation error (a *BrokenLinkError when a real file sits where a
// symlink was expected, or a dangling/looping link whose source exists — the
// not-converged signal that flips the exit to 1), or nil when clean.
func applySymlinks(se *symlink.Engine, specs []symlink.LinkSpec, info platform.Info, opts symlink.Options, coll *failure.Collector) *symlink.Report {
	plan, err := se.Plan(specs, info, opts)
	if err != nil {
		coll.Record("symlinks", err)
		return nil
	}
	report, err := se.Apply(plan, opts)
	if err != nil {
		coll.Record("symlinks", err)
		return report
	}
	_, verr := se.Validate(specs, info, opts)
	coll.Record("symlinks", verr)
	return report
}

// applyPlasmaPhase is the opt-in plasma phase's entry point: it gates on host
// support, constructs the engine, and delegates to applyPlasma. It returns nil
// when the phase did not run, so the caller has nothing to print.
//
// Requested-but-unavailable is a WARNING, not a failure, following
// applySnapshot's precedent: asking for a KDE sync on a Mac is a misconfigured
// invocation, not a broken provisioning run, and recording it would flip an
// otherwise-clean apply to exit 1.
func applyPlasmaPhase(r exec.Runner, info platform.Info, opts applyOptions, repoRoot, home string, stderr io.Writer, coll *failure.Collector) *plasma.Report {
	if plasma.Support(info) == plasma.SupportUnsupported {
		_, _ = fmt.Fprintf(stderr, "%s: plasma requested but the host is not Linux; skipping\n", progName)
		return nil
	}
	pe := plasma.NewEngine(repoRoot, xdgConfigHome(home), xdgDataHome(home), plasmaSystemColorSchemeDirs(), r)
	return applyPlasma(pe, plasma.DefaultFiles(), applyPlasmaOptions(opts), stderr, coll)
}

// applyPlasma plans, executes, and validates the repo → live KDE merge, shaped
// exactly like applySymlinks: EXACTLY ONE coll.Record("plasma", …) call on every
// path — including Record("plasma", nil) on the happy path, so the run-wide step
// count stays honest — and the phase never aborts the run.
//
// The one path that records nothing is a repo that has never run
// `dotf plasma export`: plasma/files/ is simply absent, which is a not-yet-used
// feature rather than a failed apply.
//
// It never prompts. Confirmation is a `dotf plasma apply` concern; `dotf apply`
// is the headless orchestrator.
func applyPlasma(pe *plasma.Engine, files []plasma.FileSpec, popts plasma.Options, stderr io.Writer, coll *failure.Collector) *plasma.Report {
	plan, err := pe.PlanApply(files, popts)
	if err != nil {
		var missing *plasma.MissingRepoTreeError
		if errors.As(err, &missing) {
			_, _ = fmt.Fprintf(stderr, "%s: plasma: %v; skipping\n", progName, err)
			return nil
		}
		coll.Record("plasma", err)
		return nil
	}
	report, err := pe.Execute(plan, popts)
	if err != nil {
		coll.Record("plasma", err)
		return report
	}
	_, verr := pe.Diff(files, popts)
	coll.Record("plasma", verr)
	return report
}

// applyPlasmaOptions maps resolved apply options onto the plasma engine's
// Options. Backup follows the --plasma opt-in — opting into a destructive
// optional phase from the orchestrator also opts into its safety net. Link and
// Recommended are deliberately always false: both are interactive, deliberate
// choices that belong to `dotf plasma apply`, and Reload is false because a
// headless apply must never tear down the running shell.
func applyPlasmaOptions(opts applyOptions) plasma.Options {
	return plasma.Options{
		DryRun:  opts.DryRun,
		Backup:  opts.Plasma,
		Reload:  false,
		Session: plasma.DetectSession(os.Getenv),
	}
}

// applyFirewall runs the firewall domain as a second, firewall-only sysconfig
// pass through the SAME engine, after packages so ufw/firewalld is present. An
// unset mode yields an empty plan (no-op). Outcomes are recorded per step.
func applyFirewall(eng *sysconfig.Engine, mode sysconfig.FirewallMode, info platform.Info, coll *failure.Collector) sysconfig.Result {
	cfg := sysconfig.Config{Firewall: mode, User: realUserName()}
	plan, err := eng.Plan(cfg, info)
	if err != nil {
		coll.Record("sysconfig", err)
		return sysconfig.Result{}
	}
	res, execErr := eng.Execute(plan)
	if execErr != nil {
		coll.Record("sysconfig", execErr)
	}
	for _, sr := range res.Applied {
		coll.Record("sysconfig:"+sr.Step.Domain, sr.Err)
	}
	return res
}

// applySSHServer runs the ssh-server domain as a third, ssh-server-only sysconfig
// pass through the SAME engine, after firewall so ufw/firewalld is present for
// the SSH-allow step. An unset mode yields an empty plan (no-op). Outcomes are
// recorded per step.
func applySSHServer(eng *sysconfig.Engine, mode sysconfig.SSHServerMode, info platform.Info, coll *failure.Collector) sysconfig.Result {
	cfg := sysconfig.Config{SSHServer: mode, User: realUserName()}
	plan, err := eng.Plan(cfg, info)
	if err != nil {
		coll.Record("sysconfig", err)
		return sysconfig.Result{}
	}
	res, execErr := eng.Execute(plan)
	if execErr != nil {
		coll.Record("sysconfig", execErr)
	}
	for _, sr := range res.Applied {
		coll.Record("sysconfig:"+sr.Step.Domain, sr.Err)
	}
	return res
}

// identityConfig builds the sysconfig.Config for the identity pass: the
// manifest's timezone/locale/shell defaults, the firewall and ssh-server domains
// forced unset (owned by applyFirewall and applySSHServer), and the real user
// for the shell domain. Hostname is per-machine and flag-only, so it is never
// set here.
func identityConfig(m *manifest.Manifest) sysconfig.Config {
	// resolveSysconfig's string args are (hostname, timezone, locale, shell,
	// firewall, ssh-server). apply has no per-domain identity flags, so all six
	// are empty and the values come from the manifest [sysconfig] table; the
	// firewall and ssh-server domains are then forced unset because applyFirewall
	// and applySSHServer own them as later passes.
	cfg := resolveSysconfig("", "", "", "", "", "", m.Sysconfig)
	cfg.Firewall = sysconfig.FirewallUnset
	cfg.SSHServer = sysconfig.SSHServerUnset
	cfg.User = realUserName()
	return cfg
}

// applySymlinkOptions maps resolved apply options onto the symlink engine's
// Options. Backup is on unless --no-backup, so every real file the phase
// replaces is copied first; --hyprland (aliased by --linux-desktop) opts into
// the desktop set, which the engine guard refuses with backups off outside a
// dry run; DryRun mirrors opts so a preview
// never mutates. Validate is always on.
func applySymlinkOptions(opts applyOptions) symlink.Options {
	return symlink.Options{
		DryRun:         opts.DryRun,
		Backup:         !opts.NoBackup,
		Validate:       true,
		ManageHyprland: opts.Hyprland,
	}
}

// mergeSysconfigResults concatenates the identity, firewall, and ssh-server
// Applied step results (preserving order) and unions their skipped domains
// (deduped) so the stdout printer emits one sysconfig result block for the
// whole run.
func mergeSysconfigResults(results ...sysconfig.Result) sysconfig.Result {
	merged := sysconfig.Result{}
	skippedTotal := 0
	for _, r := range results {
		merged.Applied = append(merged.Applied, r.Applied...)
		skippedTotal += len(r.Skipped)
	}

	// A domain applied in any pass must not also be reported as skipped. The
	// domain-only passes (firewall, ssh-server) plan with every identity field
	// zeroed, so their Configurer re-lists hostname/timezone/locale/shell as
	// skipped even though the identity pass (P2) already applied them. Excluding
	// applied domains keeps the greppable summary from claiming a domain is both
	// converged and skipped.
	applied := make(map[string]struct{}, len(merged.Applied))
	for _, sr := range merged.Applied {
		applied[sr.Step.Domain] = struct{}{}
	}
	skipped := make([]string, 0, skippedTotal)
	for _, r := range results {
		for _, domain := range r.Skipped {
			if _, ok := applied[domain]; ok {
				continue
			}
			skipped = append(skipped, domain)
		}
	}
	merged.Skipped = dedupeStrings(skipped)
	return merged
}

// applyDryRun composes each engine's read-only preview and prints it to stdout,
// mutating nothing and never priming sudo. It returns exit 0 on success. Plan
// errors surface as soft (exit 1) diagnostics, matching the sibling dry-run
// paths in install.go/sysconfig.go.
func applyDryRun(stdout, stderr io.Writer, r exec.Runner, m *manifest.Manifest, opts applyOptions, info platform.Info, home, repoRoot string, fwMode sysconfig.FirewallMode, sshMode sysconfig.SSHServerMode) int {
	intents := resolveIntents(m, opts)
	pkgs := packagesForInstall(m, intents)

	// Honor package-level --only/--skip in the preview so a dry-run reflects the
	// exact set a real apply would install. An unknown package name is a usage
	// error (exit 2), matching the real-apply path in runApply.
	pkgs, ferr := filterPackagesForHost(m, pkgs, opts.Only, opts.Skip, info)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, ferr)
		return 2
	}

	// Applicability filter: the preview must reflect what a real apply would
	// actually do on THIS host, not what the manifest declares. Without it the
	// dry run promises installs the real run silently skips (e.g.
	// plan.apt-repo=docker-ce on a dnf host). Applicable is a pure read-only
	// gate, so this engine never runs a command and never primes sudo —
	// the same construction status.go uses for its probe engine.
	engine := backend.Default(r, nil, repoRoot, nil)
	pkgs, notApplicable := partitionApplicableForPreview(stderr, engine, pkgs, info)

	eng := sysconfig.Default(r, "")
	idPlan, err := eng.Plan(identityConfig(m), info)
	if err != nil {
		return reportErr(stderr, err)
	}
	fwPlan, err := eng.Plan(sysconfig.Config{Firewall: fwMode, User: realUserName()}, info)
	if err != nil {
		return reportErr(stderr, err)
	}
	sshPlan, err := eng.Plan(sysconfig.Config{SSHServer: sshMode, User: realUserName()}, info)
	if err != nil {
		return reportErr(stderr, err)
	}

	se := symlink.NewEngine(repoRoot, home)
	specs := symlink.DefaultTable()
	linkPlan, err := se.Plan(specs, info, applySymlinkOptions(opts))
	if err != nil {
		return reportErr(stderr, err)
	}

	// The plasma preview calls PlanApply ONLY — never Execute or Validate — and
	// is skipped entirely when the phase would not run, so an unsupported host or
	// an un-exported repo previews nothing rather than erroring.
	plasmaPlan := applyPlasmaDryRunPlan(stderr, r, info, opts, repoRoot, home)

	if err := printApplyDryRun(stdout, intents, pkgs, notApplicable, idPlan, fwPlan, sshPlan, linkPlan, plasmaPlan); err != nil {
		return reportErr(stderr, err)
	}
	return 0
}

// applyPlasmaDryRunPlan composes the plasma phase's read-only preview, or nil
// when the phase was not requested or cannot run here. A plan error is a warning
// on stderr rather than a failed dry run, matching applyPlasma's non-fatal
// treatment of the same conditions on the real path.
func applyPlasmaDryRunPlan(stderr io.Writer, r exec.Runner, info platform.Info, opts applyOptions, repoRoot, home string) *plasma.Plan {
	if !opts.Plasma {
		return nil
	}
	if plasma.Support(info) == plasma.SupportUnsupported {
		_, _ = fmt.Fprintf(stderr, "%s: plasma requested but the host is not Linux; skipping\n", progName)
		return nil
	}
	pe := plasma.NewEngine(repoRoot, xdgConfigHome(home), xdgDataHome(home), plasmaSystemColorSchemeDirs(), r)
	plan, err := pe.PlanApply(plasma.DefaultFiles(), applyPlasmaOptions(opts))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: plasma: %v; skipping\n", progName, err)
		return nil
	}
	return plan
}

// partitionApplicableForPreview splits the previewed package set into the
// packages whose backend can actually run on this host and the names of the
// ones that cannot. A probe error is a stderr warning that KEEPS the package
// in the applicable set: a preview must never hide planned work because a
// probe was flaky, and hiding is the dangerous direction for a dry run.
func partitionApplicableForPreview(stderr io.Writer, engine *backend.Engine, pkgs []manifest.Package, info platform.Info) ([]manifest.Package, []string) {
	applicable := make([]manifest.Package, 0, len(pkgs))
	var notApplicable []string
	for _, pkg := range pkgs {
		ok, err := engine.Applicable(pkg, info)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: package %q: %v; previewing as applicable\n", progName, pkg.Name, err)
			applicable = append(applicable, pkg)
			continue
		}
		if !ok {
			notApplicable = append(notApplicable, pkg.Name)
			continue
		}
		applicable = append(applicable, pkg)
	}
	return applicable, notApplicable
}

// printApplyDryRun writes the composed apply preview as machine-greppable
// key=value lines to stdout, led by a literal dry-run=true. It reuses the
// package/sysconfig/symlink formatters so the preview matches the sibling
// subcommands' output shape. The whole preview is emitted with one checked
// write so a broken stdout surfaces rather than being silently dropped. pkgs is
// already applicability-filtered, and the names excluded by that filter are
// reported on the single plan.not-applicable= line.
func printApplyDryRun(stdout io.Writer, intents []string, pkgs []manifest.Package, notApplicable []string, idPlan, fwPlan, sshPlan sysconfig.Plan, linkPlan *symlink.Plan, plasmaPlan *plasma.Plan) error {
	names := make([]string, len(pkgs))
	for i, pkg := range pkgs {
		names[i] = pkg.Name
	}

	lines := []string{
		"dry-run=true",
		"intents=" + strings.Join(intents, ","),
		"packages=" + strings.Join(names, ","),
	}

	byBackend := groupPackagesByBackendSlice(pkgs)
	for _, backendName := range sortedBackendKeys(byBackend) {
		lines = append(lines, fmt.Sprintf("plan.%s=%s", backendName, strings.Join(byBackend[backendName], ",")))
	}
	if len(notApplicable) > 0 {
		lines = append(lines, "plan.not-applicable="+strings.Join(notApplicable, ","))
	}

	for _, step := range idPlan.Steps {
		lines = append(lines, fmt.Sprintf("sysconfig.step.%s=%s", step.Domain, sysconfigStepValue(step)))
	}
	for _, step := range fwPlan.Steps {
		lines = append(lines, fmt.Sprintf("sysconfig.step.%s=%s", step.Domain, sysconfigStepValue(step)))
	}
	for _, step := range sshPlan.Steps {
		lines = append(lines, fmt.Sprintf("sysconfig.step.%s=%s", step.Domain, sysconfigStepValue(step)))
	}

	lines = append(lines, linkActionLines("link", linkPlan.Actions)...)
	if plasmaPlan != nil {
		// The SAME formatter `dotf plasma apply --dry-run` uses, so there is
		// deliberately no second plasma renderer to drift from.
		lines = append(lines, plasmaActionLines("plasma", plasmaPlan.Files)...)
	}

	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return err
}
