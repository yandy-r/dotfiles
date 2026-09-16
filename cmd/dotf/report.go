package main

import (
	"encoding/json"
	"io"
)

// This file defines the stable `--json` output shape shared by the read-only
// reporting commands (status, diff, plan) and the encoder that emits it. The
// shape is intentionally command-agnostic so a future probe-backed doctor can
// reuse packageReport/commandReport verbatim rather than inventing a parallel
// schema.

// packageReport is one package's reporting row in the JSON output. It carries
// the identity (Name, Backend), the platform-applicability verdict, the probed
// installed state, and — for plan/diff — the action that would be taken. The
// field set is a superset across commands; each command populates only the
// fields it can honestly report (see the omitempty notes below).
type packageReport struct {
	// Name is the manifest package name.
	Name string `json:"name"`
	// Backend is the install mechanism (pkg, github-release, aur, ...).
	Backend string `json:"backend"`
	// Applicable reports whether this package's backend can run on the host.
	Applicable bool `json:"applicable"`
	// Installed is the probed installed state, or nil when the package was not
	// probed (e.g. a non-applicable backend, or a plan selection that probes
	// nothing). A tri-state pointer distinguishes "not installed" (false) from
	// "not probed" (absent).
	Installed *bool `json:"installed,omitempty"`
	// Action is the planned action ("install" or "none"), populated by the
	// commands that describe intent (plan, diff) and omitted by status.
	Action string `json:"action,omitempty"`
	// Class is the package's effective update-policy class (pinned/rolling/
	// managed) from manifest.Package.EffectiveClass — the explicit class or the
	// one inferred from the backend. Additive and omitempty, so consumers that
	// ignore it are unaffected.
	Class string `json:"class,omitempty"`
}

// classSummary is the aggregate governance histogram emitted by status/diff
// under --counts: the total row count plus per-backend and per-class tallies.
// It answers "how is my package set distributed across install mechanisms and
// update policies?" so backend/class composition is visible at a glance.
type classSummary struct {
	Total     int            `json:"total"`
	ByBackend map[string]int `json:"by_backend"`
	ByClass   map[string]int `json:"by_class"`
}

// commandReport is the top-level JSON object one reporting command emits: the
// command name plus its per-package rows. The packages slice is always encoded
// as an array (never null) so consumers can iterate unconditionally.
type commandReport struct {
	Command  string          `json:"command"`
	Packages []packageReport `json:"packages"`
	// Summary is the aggregate backend/class histogram, populated only by
	// status/diff under --counts and omitted otherwise, so default JSON output is
	// unchanged.
	Summary *classSummary `json:"summary,omitempty"`
}

// summarize builds the aggregate backend/class histogram over reports: the total
// count plus per-backend and per-class tallies. It counts every row it is given,
// so status (all declared packages) and diff (only drift rows) each summarize the
// set they emit.
func summarize(reports []packageReport) classSummary {
	s := classSummary{
		Total:     len(reports),
		ByBackend: make(map[string]int),
		ByClass:   make(map[string]int),
	}
	for _, r := range reports {
		s.ByBackend[r.Backend]++
		if r.Class != "" {
			s.ByClass[r.Class]++
		}
	}
	return s
}

// lockReport is the top-level JSON object `dotf lock --json` emits: the command
// name plus one row per resolved pinnable package. It is intentionally separate
// from commandReport/packageReport — the lock shape is artifact-centric (frozen
// tag/url/os/arch/asset/binary/sha256), not the applicable/installed/action
// shape the read-only reporting commands share.
type lockReport struct {
	Command  string                `json:"command"`
	Packages []lockedPackageReport `json:"packages"`
}

// lockedPackageReport is one locked package's JSON row: its identity, the pin
// coordinate (tag for github-release, url for script-url), and the per-platform
// artifact rows.
type lockedPackageReport struct {
	Name      string           `json:"name"`
	Backend   string           `json:"backend"`
	Tag       string           `json:"tag,omitempty"`
	URL       string           `json:"url,omitempty"`
	Artifacts []artifactReport `json:"artifacts"`
}

// artifactReport is one platform's frozen artifact row in the lock JSON output.
type artifactReport struct {
	OS     string `json:"os,omitempty"`
	Arch   string `json:"arch,omitempty"`
	Asset  string `json:"asset,omitempty"`
	Binary string `json:"binary,omitempty"`
	SHA256 string `json:"sha256"`
}

// checkStatus is a doctor check's verdict. The three values are ordered by
// severity (ok < warn < fail) so a category's worst verdict can be aggregated.
type checkStatus string

// Doctor check verdicts, ordered by increasing severity.
const (
	checkOK   checkStatus = "ok"
	checkWarn checkStatus = "warn"
	checkFail checkStatus = "fail"
)

// doctorItem is one structured finding attached to a doctor check: a missing
// package, a pending symlink, or a drifted plasma file. It carries the identity the check builders
// already compute (package name/backend/class, or symlink action/dest/src) so
// consumers see WHICH thing drifted, not just an aggregate count. The field set
// is a superset across kinds; each kind populates only the fields it can honestly
// report and every field is omitempty, so a package item never encodes a symlink
// field and vice versa.
type doctorItem struct {
	// Kind discriminates the item: "package", "symlink", or "plasma".
	Kind string `json:"kind"`
	// Name is the manifest package name (package kind).
	Name string `json:"name,omitempty"`
	// Backend is the package install mechanism (package kind).
	Backend string `json:"backend,omitempty"`
	// Class is the package effective update-policy class, when set (package kind).
	Class string `json:"class,omitempty"`
	// Action is the pending symlink operation: "create" or "repoint" (symlink kind).
	Action string `json:"action,omitempty"`
	// Dest is the symlink destination, $HOME-relative (symlink kind).
	Dest string `json:"dest,omitempty"`
	// Src is the symlink source, repo-relative (symlink kind).
	Src string `json:"src,omitempty"`
}

// doctorCheck is one diagnostic result: a category/name identity, the verdict,
// a human "what was observed" detail, an actionable remedy (empty when ok), and
// an optional itemized breakdown of the finding. Items is additive and
// omitempty, so a check with no items encodes exactly as it did before this
// field existed — the default --json shape is unchanged for such checks.
type doctorCheck struct {
	Category string       `json:"category"`
	Name     string       `json:"name"`
	Status   checkStatus  `json:"status"`
	Detail   string       `json:"detail"`
	Remedy   string       `json:"remedy,omitempty"`
	Items    []doctorItem `json:"items,omitempty"`
}

// doctorSummary is the aggregate verdict tally over a doctor run's checks.
type doctorSummary struct {
	Total int `json:"total"`
	OK    int `json:"ok"`
	Warn  int `json:"warn"`
	Fail  int `json:"fail"`
}

// doctorReport is the top-level `dotf doctor --json` object. It is intentionally
// distinct from commandReport/packageReport (the applicable/installed/action
// shape) and from lockReport — doctor's shape is check-centric, not
// package-centric — but it reuses the same writeJSON encoder.
type doctorReport struct {
	Command string        `json:"command"` // always "doctor"
	Checks  []doctorCheck `json:"checks"`
	Summary doctorSummary `json:"summary"`
}

// selfUpdateReport is the top-level `dotf self-update --json` object. It is
// intentionally distinct from the package-centric reports (commandReport/
// lockReport) and the check-centric doctorReport — self-update's shape is
// version/artifact-centric (current/latest/asset/target) — but it reuses the
// same writeJSON encoder. --dry-run and --check are read-only markers, omitted
// on the default replace path.
type selfUpdateReport struct {
	Command   string `json:"command"` // always "self-update"
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"update_available"`
	Asset     string `json:"asset,omitempty"`
	Target    string `json:"target,omitempty"`
	Updated   bool   `json:"updated"`
	DryRun    bool   `json:"dry_run,omitempty"`
	Checked   bool   `json:"checked,omitempty"`
}

// writeJSON encodes v as indented JSON to w and returns the encode error. It is
// the single JSON emission point for the reporting commands, mirroring the
// single-checked-write discipline of the key=value printers so a broken stdout
// surfaces rather than being silently ignored.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
