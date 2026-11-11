package plasma

import (
	"bytes"
	"errors"
	"os"
)

// Diff computes per-key drift in both directions.
//
// It compares PARSED MODELS at key granularity — never bytes. That is what
// makes the codec's one-pass normalisations (the \; escape asymmetry, KDE's
// flag reordering, group-[$i] propagation) incapable of surfacing as phantom
// drift.
//
// The comparison itself is diffDocs, which is also what Converged is a
// predicate over — so `dotf plasma diff`, Validate, Status, doctor, and the
// convergence short-circuit PlanApply uses all resolve to one loop. Diff is the
// reporting face of that loop, not a second source of truth.
//
// Results are never discarded on error: a *DriftError is returned ALONGSIDE the
// full []FileDiff whenever any file has an actionable difference.
func (e *Engine) Diff(files []FileSpec, opts Options) ([]FileDiff, error) {
	if _, err := os.Stat(e.repoRoot); err != nil {
		return nil, &MissingSourceTreeError{Root: e.repoRoot}
	}

	// The §7.4 guard is resolved ONCE per run, exactly as PlanExport does it,
	// so the volatile classification below asks the same question export will
	// answer: a colour rule the guard disables is not a rule this run strips.
	keepColors, _ := e.colorSchemeGuardFor(files)

	var (
		diffs   []FileDiff
		drifted []string
	)
	for _, spec := range files {
		if err := checkSpec(spec); err != nil {
			return diffs, err
		}
		fd, err := e.diffFile(spec, opts, keepColors)
		if err != nil {
			return diffs, err
		}
		if fd == nil {
			continue
		}
		diffs = append(diffs, *fd)
		if fd.Drifted() {
			drifted = append(drifted, spec.Path)
		}
	}

	if len(drifted) > 0 {
		return diffs, &DriftError{Files: drifted}
	}
	return diffs, nil
}

// diffFile compares one managed file, or returns nil when it is absent on both
// sides and therefore has nothing to say.
//
// It resolves the SOURCE document — plasma/files/ overlaid with
// plasma/recommended/ — before deciding whether there is anything to compare,
// for the same reason planApplyFile does: under --recommended, a file the
// overlay names and plasma/files/ does not is a real source with real keys, and
// short-circuiting on the missing snapshot reported those files "converged"
// while none of their settings had ever been applied.
func (e *Engine) diffFile(spec FileSpec, opts Options, keepColors bool) (*FileDiff, error) {
	repoData, repoPresent, err := readOptional(e.repoPath(spec))
	if err != nil {
		return nil, err
	}
	liveData, livePresent, err := readOptional(e.livePath(spec))
	if err != nil {
		return nil, err
	}

	fd := &FileDiff{Spec: spec, RepoPresent: repoPresent, LivePresent: livePresent}

	if spec.Format == FormatRaw {
		if !repoPresent && !livePresent {
			return nil, nil
		}
		if !repoPresent {
			return fd, nil
		}
		switch {
		case !livePresent:
			fd.Keys = []KeyDiff{{Key: rawContentKey, Kind: DiffMissingLive, Repo: repoData}}
		case !bytes.Equal(repoData, liveData):
			fd.Keys = []KeyDiff{{
				Key: rawContentKey, Kind: DiffValueMismatch, Repo: repoData, Live: liveData,
			}}
		}
		return fd, nil
	}

	source, err := e.applySource(spec, e.repoPath(spec), repoData, repoPresent, opts)
	if err != nil {
		return nil, err
	}
	if source == nil {
		// Nothing is asserted about this file by this run: no snapshot, no
		// overlay. A live-only file is still listed (status reports it as
		// missing-repo) but has no keys to compare against.
		if !livePresent {
			return nil, nil
		}
		return fd, nil
	}

	var liveDoc *Doc
	if livePresent {
		if liveDoc, err = Parse(e.livePath(spec), liveData); err != nil {
			return nil, err
		}
	}

	fd.Keys = classifyVolatile(diffDocs(source, liveDoc), e.volatileMatcher(spec, keepColors))
	if !opts.ShowVolatile {
		fd.Keys = dropExpectedVolatile(fd.Keys)
	}
	return fd, nil
}

// volatileMatcher compiles a file's strip rules once and returns the predicate
// "would export remove this live key?". It is the same rule set Strip applies,
// compiled the same way, so a key classified expected-volatile here is exactly
// a key that can never reach plasma/files/.
func (e *Engine) volatileMatcher(spec FileSpec, keepColorScheme bool) func([]string, string, []byte) bool {
	groupRules := compileRules(spec.VolatileGroups, parseGroupRule, keepColorScheme, spec.Path)
	keyRules := compileRules(spec.VolatileKeys, parseKeyRule, keepColorScheme, spec.Path)
	valueRules := compileRules(spec.VolatileValues, parseValueRule, keepColorScheme, spec.Path)

	return func(group []string, key string, value []byte) bool {
		return matchAny(groupRules, group, "", nil) ||
			matchAny(keyRules, group, key, nil) ||
			matchAny(valueRules, group, key, value)
	}
}

// classifyVolatile re-labels the live-only differences the strip table
// explains. Only DiffMissingRepo can be re-labelled: every other kind involves
// a key the repo DOES name, which by definition survived the strip.
func classifyVolatile(keys []KeyDiff, isVolatile func([]string, string, []byte) bool) []KeyDiff {
	for i := range keys {
		if keys[i].Kind != DiffMissingRepo {
			continue
		}
		if isVolatile(keys[i].Group, keys[i].Key, keys[i].Live) {
			keys[i].Kind = DiffExpectedVolatile
		}
	}
	return keys
}

// dropExpectedVolatile removes the expected live-only keys, which is the
// default: they are the strip table working as designed, and on a real desktop
// they outnumber genuinely unmanaged keys by an order of magnitude.
// Options.ShowVolatile keeps them.
func dropExpectedVolatile(keys []KeyDiff) []KeyDiff {
	out := keys[:0]
	for _, k := range keys {
		if k.Kind == DiffExpectedVolatile {
			continue
		}
		out = append(out, k)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Status summarises the managed set without computing full key detail beyond
// the drift counts. It resolves to Diff, so "what status reports" and "what
// apply would do" can never disagree.
//
// A *DriftError from Diff is expected — drift is exactly what Status reports —
// so it is absorbed here rather than propagated; any other error is fatal.
func (e *Engine) Status(files []FileSpec, opts Options) (*StatusReport, error) {
	diffs, err := e.Diff(files, opts)
	var drift *DriftError
	if err != nil && !errors.As(err, &drift) {
		return nil, err
	}

	report := &StatusReport{Managed: len(files)}
	somewhere := make(map[string]bool, len(diffs))
	for _, fd := range diffs {
		if fd.RepoPresent {
			report.RepoPresent++
		}
		if fd.LivePresent {
			report.LivePresent++
		}
		if fd.RepoPresent || fd.LivePresent {
			somewhere[fd.Spec.Path] = true
		}
		switch {
		case fd.RepoPresent && !fd.LivePresent:
			report.MissingLive = append(report.MissingLive, fd.Spec.Path)
		case !fd.RepoPresent && fd.LivePresent:
			report.MissingRepo = append(report.MissingRepo, fd.Spec.Path)
		}
		if fd.Drifted() {
			report.DriftedFiles = append(report.DriftedFiles, fd.Spec.Path)
		}
		for _, k := range fd.Keys {
			if k.Kind.Actionable() {
				report.DriftedKeys++
			}
		}
	}

	for _, spec := range files {
		if !somewhere[spec.Path] {
			report.Absent = append(report.Absent, spec.Path)
		}
	}
	return report, nil
}

// StatusReport is the read-only summary `dotf plasma status` renders. It holds
// only what Status DERIVES from the filesystem; the run switches stay on
// Options, which the CLI already has.
type StatusReport struct {
	// Managed is len(files) after --only filtering.
	Managed int
	// RepoPresent counts managed files present in plasma/files/.
	RepoPresent int
	// LivePresent counts managed files present under the KDE config dir.
	LivePresent int
	// MissingLive lists files present in the repo and absent live.
	MissingLive []string
	// MissingRepo lists files present live and absent from the repo.
	MissingRepo []string
	// Absent lists managed files present on NEITHER side: never exported and
	// never written by this desktop. It is a distinct state from "present",
	// which is what the JSON renderer used to call them by default, and from
	// either "missing" set, which each imply the file exists on the other side.
	// A fresh host has plenty of these — every recommended-only member before
	// its first `apply --recommended`.
	Absent []string
	// DriftedFiles lists files with at least one actionable difference.
	DriftedFiles []string
	// DriftedKeys counts actionable differences across every file.
	DriftedKeys int
}
