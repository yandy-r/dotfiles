package main

import (
	"encoding/base64"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/yandy-r/dotfiles/internal/plasma"
)

// This file is the `--json` half of `dotf plasma`'s rendering: the document
// shape every verb emits, the converters from engine values into its rows, and
// the single failure exit that guarantees a --json run never answers with zero
// bytes. The key=value line builders live in plasmaprint.go and the verb
// orchestration in plasmaverbs.go, so all three stay under the ~500-line cap.
//
// --json REPLACES the key=value stdout rather than augmenting it, and never
// changes the exit code.

// plasmaReport is the top-level `dotf plasma --json` object. It is emitted
// through the same writeJSON encoder the other reporting commands use, so the
// single-checked-write discipline holds for JSON too.
//
// It is also the FAILURE document: an error path fills Error and leaves Files
// empty rather than writing nothing at all, so a machine consumer parses one
// shape on every path instead of having to treat zero bytes plus a non-zero
// exit as a document of its own.
type plasmaReport struct {
	Command string             `json:"command"` // always "plasma"
	Verb    string             `json:"verb"`
	DryRun  bool               `json:"dryRun,omitempty"`
	Session string             `json:"session"`
	Files   []plasmaFileReport `json:"files"`
	Reloads []string           `json:"reloads,omitempty"`
	// LinkConflicts names the files whose mode transition this PREVIEW planned
	// but a real run without --backup would refuse. Empty on every other path.
	LinkConflicts []string      `json:"linkConflicts,omitempty"`
	Summary       plasmaSummary `json:"summary"`
	// Failures carries one diagnostic per NON-FATAL failure, in the order and
	// with the text printPlasmaFailures writes to stderr. Its length is always
	// summary.failed.
	//
	// It exists because Error alone could not carry them: a per-file refusal
	// leaves the run completing, so Error stays empty, and the files array
	// cannot cover the class either — a failed reload has no file to attach to.
	// Without it a --json consumer saw a non-zero summary.failed and had no way
	// to learn what failed or why, because both sentences went to stderr.
	Failures []string `json:"failures,omitempty"`
	// Error is the diagnostic on a failure path, and the one field whose
	// presence means the run did not complete.
	Error string `json:"error,omitempty"`
}

// plasmaErrorDoc builds the failure form of the --json document: the same
// envelope every verb emits, with no files and the diagnostic in "error".
func plasmaErrorDoc(verb string, dryRun bool, session plasma.SessionState, err error) *plasmaReport {
	return &plasmaReport{
		Command: "plasma",
		Verb:    verb,
		DryRun:  dryRun,
		Session: session.String(),
		Files:   []plasmaFileReport{},
		Error:   err.Error(),
	}
}

// plasmaFail is the single failure exit for `dotf plasma`: it writes the JSON
// error document to stdout when --json is set, then the human diagnostic to
// stderr, and returns exit 1. A failure to write the JSON replaces the original
// diagnostic, because a broken stdout is the more urgent of the two.
func plasmaFail(
	stdout, stderr io.Writer,
	o *plasmaOpts,
	verb string,
	session plasma.SessionState,
	err error,
) int {
	if o.jsonOut {
		if werr := writeJSON(stdout, plasmaErrorDoc(verb, o.dryRun, session, err)); werr != nil {
			return reportErr(stderr, werr)
		}
	}
	return reportErr(stderr, err)
}

// plasmaFileReport is one managed file's row. Action carries the planned
// ActionKind for the mutating verbs and a presence/drift verdict for the
// read-only ones, so the field is always populated and consumers never have to
// branch on the verb to know whether it exists.
type plasmaFileReport struct {
	Path   string            `json:"path"`
	Action string            `json:"action"`
	Reason string            `json:"reason,omitempty"`
	Keys   []plasmaKeyReport `json:"keys,omitempty"`
	Error  string            `json:"error,omitempty"`
}

// plasmaKeyReport is one (group, key, locale) row. Repo/Live hold the decoded
// values; a value that is not valid UTF-8 is base64-encoded and Encoding is set
// to "base64", because KConfig values are byte strings and JSON strings are not.
type plasmaKeyReport struct {
	Group    string `json:"group"`
	Key      string `json:"key"`
	Locale   string `json:"locale,omitempty"`
	Kind     string `json:"kind,omitempty"` // diff verb only
	Repo     string `json:"repo,omitempty"`
	Live     string `json:"live,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

// plasmaSummary is the aggregate tally every verb reports. VolatileKeys counts
// the expected-volatile rows separately from DriftKeys so a --show-volatile
// report's key totals stay decomposable: driftKeys is what apply would act on,
// volatileKeys is what export strips by design, and neither is inferable from
// the other by subtraction. It stays 0 on the default path, where the engine
// suppresses those keys entirely.
//
// DriftFiles/DriftKeys and Failed answer different questions and are never
// mixed. Failed counts the run's non-fatal FAILURES — refused writes, failed
// reloads — and equals len(plasmaReport.Failures). Drift is a post-apply
// REPORT, so apply's Validate contributes to the two drift counters only:
// counting its DriftError as a failure alongside the refusal that caused it
// reported one problem as two, and both counters are now filled by
// plasmaDriftTally so they mean the same thing on apply as on diff.
type plasmaSummary struct {
	Managed      int `json:"managed"`
	Planned      int `json:"planned"`
	Applied      int `json:"applied"`
	DriftFiles   int `json:"driftFiles"`
	DriftKeys    int `json:"driftKeys"`
	VolatileKeys int `json:"volatileKeys"`
	Failed       int `json:"failed"`
}

// plasmaFailureStrings renders the run's non-fatal failures for the --json
// document: one entry per failure, in the same order and with the same text
// printPlasmaFailures writes to stderr, so the two channels can never describe
// a different failure set. nil when nothing failed, so the field is omitted.
func plasmaFailureStrings(errs []error) []string {
	if len(errs) == 0 {
		return nil
	}
	out := make([]string, len(errs))
	for i, err := range errs {
		out[i] = err.Error()
	}
	return out
}

// plasmaDriftTally counts a diff set's drifted files, actionable keys, and
// expected-volatile keys.
//
// It is shared by `diff` and by apply's post-write Validate — which is the same
// Diff comparison — so summary.driftFiles and summary.driftKeys mean the same
// thing on both verbs. apply used to fill driftFiles from the DriftError's path
// list and leave driftKeys pinned at 0, so a consumer reading driftKeys to
// decide whether post-apply validation found anything concluded "no keys
// drifted" after every single apply.
func plasmaDriftTally(diffs []plasma.FileDiff) (files, keys, volatile int) {
	for _, fd := range diffs {
		if fd.Drifted() {
			files++
		}
		for _, k := range fd.Keys {
			// DiffKind.Actionable is the engine's single definition of "drift",
			// and the same one FileDiff.Drifted uses — counting keys by any
			// local rule let the key tally disagree with the file tally the
			// moment a second informational kind appeared, which is exactly
			// what DiffExpectedVolatile is.
			if k.Kind.Actionable() {
				keys++
			}
			if k.Kind == plasma.DiffExpectedVolatile {
				volatile++
			}
		}
	}
	return files, keys, volatile
}

// plasmaValueJSON renders a KConfig value for JSON: the decoded text when it is
// valid UTF-8, else base64 with a true "encode as base64" flag. A nil value
// (the key is absent on that side) yields "" and false so the field is omitted.
func plasmaValueJSON(v []byte) (string, bool) {
	if len(v) == 0 {
		return "", false
	}
	if utf8.Valid(v) {
		return string(v), false
	}
	return base64.StdEncoding.EncodeToString(v), true
}

// plasmaFileReports converts planned actions into JSON rows. The Keys array is
// populated from KeyAction so the JSON carries exactly the detail the
// plasma.change= line summarises.
func plasmaFileReports(actions []plasma.FileAction) []plasmaFileReport {
	rows := make([]plasmaFileReport, 0, len(actions))
	for _, a := range actions {
		row := plasmaFileReport{
			Path:   a.Spec.Path,
			Action: a.Kind.String(),
			Reason: a.Reason,
		}
		if a.Err != nil {
			row.Error = a.Err.Error()
		}
		for _, k := range a.Keys {
			kr := plasmaKeyReport{
				Group:  plasmaGroupHeader(k.Group),
				Key:    k.Key,
				Locale: k.Locale,
			}
			repo, repoB64 := plasmaValueJSON(k.To)
			live, liveB64 := plasmaValueJSON(k.From)
			kr.Repo, kr.Live = repo, live
			if repoB64 || liveB64 {
				kr.Encoding = "base64"
			}
			row.Keys = append(row.Keys, kr)
		}
		rows = append(rows, row)
	}
	return rows
}

// plasmaDiffReports converts per-file diffs into JSON rows. Action is a verdict
// rather than an operation here — "drift" or "converged" — because diff plans
// nothing.
//
// The verdict reads FileDiff.Drifted, so a file whose only rows are
// informational (missing-repo, or expected-volatile under --show-volatile)
// reports "converged" while still carrying its keys: the rows are the evidence,
// keys[].kind is the classification, and the verdict is never inferred from the
// row count.
func plasmaDiffReports(diffs []plasma.FileDiff) []plasmaFileReport {
	rows := make([]plasmaFileReport, 0, len(diffs))
	for _, fd := range diffs {
		row := plasmaFileReport{Path: fd.Spec.Path, Action: "converged"}
		if fd.Drifted() {
			row.Action = "drift"
		}
		for _, k := range fd.Keys {
			kr := plasmaKeyReport{
				Group:  plasmaGroupHeader(k.Group),
				Key:    k.Key,
				Locale: k.Locale,
				Kind:   k.Kind.String(),
			}
			repo, repoB64 := plasmaValueJSON(k.Repo)
			live, liveB64 := plasmaValueJSON(k.Live)
			kr.Repo, kr.Live = repo, live
			if repoB64 || liveB64 {
				kr.Encoding = "base64"
			}
			row.Keys = append(row.Keys, kr)
		}
		rows = append(rows, row)
	}
	return rows
}

// plasmaStatusReports converts a StatusReport's presence sets into per-file JSON
// rows, so the status verb populates the same Files array as every other verb
// instead of needing a parallel schema.
func plasmaStatusReports(files []plasma.FileSpec, st *plasma.StatusReport) []plasmaFileReport {
	missingLive := plasmaSet(st.MissingLive)
	missingRepo := plasmaSet(st.MissingRepo)
	drifted := plasmaSet(st.DriftedFiles)

	absent := plasmaSet(st.Absent)

	rows := make([]plasmaFileReport, 0, len(files))
	for _, spec := range files {
		// "present" is the fall-through only for a file the engine placed in
		// none of the three sets, i.e. one that really is on both sides.
		// Defaulting to it while the both-absent state had no set of its own
		// meant `status --json` claimed six files existed that existed nowhere.
		row := plasmaFileReport{Path: spec.Path, Action: "present"}
		switch {
		case absent[spec.Path]:
			row.Action = "absent"
		case missingLive[spec.Path]:
			row.Action = "missing-live"
		case missingRepo[spec.Path]:
			row.Action = "missing-repo"
		}
		if drifted[spec.Path] {
			row.Reason = "drift"
		}
		rows = append(rows, row)
	}
	return rows
}

// plasmaSet builds a membership set from a path slice.
func plasmaSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[p] = true
	}
	return set
}

// plasmaReloadStrings renders reload commands as space-joined argv strings for
// the JSON "reloads" array.
func plasmaReloadStrings(cmds []plasma.ReloadCommand) []string {
	out := make([]string, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, strings.Join(c.Argv, " "))
	}
	return out
}
