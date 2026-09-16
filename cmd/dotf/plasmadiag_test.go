package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
)

// These tests cover `dotf plasma`'s DIAGNOSTIC surface: what a failed or
// partially-failed run tells its two consumers. They live apart from
// plasma_test.go's behavioural cases because every one of them asserts on a
// report rather than on an effect, and because plasma_test.go is already well
// past the file-size soft cap.
//
// Like every other test in this package they use t.Setenv and therefore never
// call t.Parallel.

// immutableKiorc is a live file carrying KConfig's bare file-level [$i] marker.
// Applying a differing repo copy over it is the canonical non-fatal per-file
// failure: the write is refused, the run continues, and the exit code is 1.
const immutableKiorc = "[$i]\n[Confirmations]\nConfirmDelete=true\n"

// decodePlasmaJSON parses a --json document, failing the test on malformed
// output so every caller below can assert on fields directly.
func decodePlasmaJSON(t *testing.T, stdout string) plasmaReport {
	t.Helper()
	var got plasmaReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", stdout, err)
	}
	return got
}

// TestRunPlasmaApply_JSONCarriesPerFileDiagnostic: a refused write must be
// EXPLAINED in the --json document, not merely counted. The document used to
// carry files:[] and summary.failed with no error text anywhere, so a machine
// consumer saw a non-zero failure count and had nothing to act on — the two
// sentences naming the immutability refusal went only to stderr.
func TestRunPlasmaApply_JSONCarriesPerFileDiagnostic(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kiorc", immutableKiorc)
	env.writeRepoPlasma(t, "kiorc", repoKiorc)

	code, stdout, _ := runPlasmaCapture(t, newPlasmaFake(t),
		"apply", "--yes", "--json", "--only", "kiorc")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout:\n%s", code, stdout)
	}

	got := decodePlasmaJSON(t, stdout)
	if len(got.Failures) == 0 {
		t.Fatalf("failures = %v, want the refusal diagnostic", got.Failures)
	}
	if !strings.Contains(got.Failures[0], "immutable") {
		t.Errorf("failures[0] = %q, want it to name the immutability refusal", got.Failures[0])
	}

	if len(got.Files) != 1 {
		t.Fatalf("files = %+v, want the one refused file", got.Files)
	}
	row := got.Files[0]
	if row.Path != "kiorc" {
		t.Errorf("files[0].path = %q, want %q", row.Path, "kiorc")
	}
	if !strings.Contains(row.Error, "immutable") {
		t.Errorf("files[0].error = %q, want it to name the immutability refusal", row.Error)
	}
}

// TestRunPlasmaApply_TextReportsRefusedFile: the same refusal must appear in
// the key=value output too. Rendering only Report.Applied dropped every skipped
// action, so the real run and its own --dry-run preview reported different file
// sets for the identical tree.
func TestRunPlasmaApply_TextReportsRefusedFile(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kiorc", immutableKiorc)
	env.writeRepoPlasma(t, "kiorc", repoKiorc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t),
		"apply", "--yes", "--only", "kiorc")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "plasma.skip=kiorc") {
		t.Errorf("stdout = %q, want a plasma.skip= bucket naming kiorc", stdout)
	}

	_, preview, _ := runPlasmaCapture(t, newPlasmaFake(t),
		"apply", "--dry-run", "--only", "kiorc")
	if !strings.Contains(preview, "plasma.skip=kiorc") {
		t.Fatalf("dry-run stdout = %q, want a plasma.skip= bucket naming kiorc", preview)
	}
}

// TestRunPlasmaApply_DriftIsNotASecondFailure: the refused write and the
// post-apply DriftError it necessarily causes are ONE problem. summary.failed
// counted both, and summary.driftKeys stayed pinned at 0 while driftFiles was
// filled from the DriftError — so the two fields meant different things on
// apply than they did on diff.
func TestRunPlasmaApply_DriftIsNotASecondFailure(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kiorc", immutableKiorc)
	env.writeRepoPlasma(t, "kiorc", repoKiorc)

	_, stdout, _ := runPlasmaCapture(t, newPlasmaFake(t),
		"apply", "--yes", "--json", "--only", "kiorc")
	got := decodePlasmaJSON(t, stdout)

	if got.Summary.Failed != 1 {
		t.Errorf("summary.failed = %d, want 1: the refusal and the drift it causes are one problem",
			got.Summary.Failed)
	}
	if got.Summary.Failed != len(got.Failures) {
		t.Errorf("summary.failed = %d but failures has %d entries; they must agree",
			got.Summary.Failed, len(got.Failures))
	}
	if got.Summary.DriftFiles != 1 || got.Summary.DriftKeys != 1 {
		t.Errorf("summary drift = files:%d keys:%d, want 1/1 (same rule diff uses)",
			got.Summary.DriftFiles, got.Summary.DriftKeys)
	}

	// The identical tree through `diff` must agree, which is the whole point of
	// the two verbs sharing one tally.
	_, diffOut, _ := runPlasmaCapture(t, newPlasmaFake(t), "diff", "--json", "--only", "kiorc")
	wantDiff := decodePlasmaJSON(t, diffOut)
	if wantDiff.Summary.DriftFiles != got.Summary.DriftFiles ||
		wantDiff.Summary.DriftKeys != got.Summary.DriftKeys {
		t.Errorf("apply drift = files:%d keys:%d, diff drift = files:%d keys:%d; want identical",
			got.Summary.DriftFiles, got.Summary.DriftKeys,
			wantDiff.Summary.DriftFiles, wantDiff.Summary.DriftKeys)
	}
}

// TestRunPlasmaApply_ReloadCommandsArePrintedResolved: Engine.Reload probes
// qdbus6 before qdbus and RETURNS what it ran; the CLI discarded that and
// printed the plan's unresolved argv, so the line the tool prints precisely so
// a user can paste it named a binary no Plasma 6 host has.
func TestRunPlasmaApply_ReloadCommandsArePrintedResolved(t *testing.T) {
	setup := func(t *testing.T) plasmaEnv {
		t.Helper()
		env := newPlasmaEnv(t)
		t.Setenv("KDE_FULL_SESSION", "true")
		env.writeLive(t, "kwinrc", "[Windows]\nFocusPolicy=FocusFollowsMouse\n")
		env.writeRepoPlasma(t, "kwinrc", "[Windows]\nFocusPolicy=ClickToFocus\n")
		return env
	}
	fake := func(t *testing.T) *exec.Fake {
		t.Helper()
		f := newPlasmaFake(t)
		// Only the Plasma 6 name exists, exactly as on a real host.
		f.AddPath("qdbus6", "/usr/bin/qdbus6")
		return f
	}

	setup(t)
	code, stdout, stderr := runPlasmaCapture(t, fake(t),
		"apply", "--yes", "--reload", "--only", "kwinrc")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "plasma.reload.1=qdbus6 org.kde.KWin /KWin reconfigure") {
		t.Errorf("stdout = %q, want the RESOLVED qdbus6 reload line", stdout)
	}

	setup(t)
	_, jsonOut, _ := runPlasmaCapture(t, fake(t),
		"apply", "--yes", "--reload", "--json", "--only", "kwinrc")
	got := decodePlasmaJSON(t, jsonOut)
	if len(got.Reloads) != 1 || got.Reloads[0] != "qdbus6 org.kde.KWin /KWin reconfigure" {
		t.Errorf("reloads = %v, want the resolved qdbus6 command", got.Reloads)
	}
}

// TestRunPlasmaExport_SessionAdvisoryIsExportShaped: export writes
// plasma/files/, which no KDE process touches, so warning that the session "may
// overwrite your changes" described a hazard that cannot exist in that
// direction.
func TestRunPlasmaExport_SessionAdvisoryIsExportShaped(t *testing.T) {
	env := newPlasmaEnv(t)
	t.Setenv("KDE_FULL_SESSION", "true")
	env.writeLive(t, "plasmarc", "[Theme]\nname=breeze\n")

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "export", "--only", "plasmarc")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.Contains(stderr, "overwritten before you log out") {
		t.Errorf("stderr = %q, want no apply-direction overwrite warning on export", stderr)
	}
	if !strings.Contains(stderr, "plasmarc is rewritten by the running session") {
		t.Errorf("stderr = %q, want the session-ownership notice", stderr)
	}
	if !strings.Contains(stderr, "snapshot") {
		t.Errorf("stderr = %q, want export-direction wording about the captured snapshot", stderr)
	}
}

// TestRunPlasmaApply_SessionAdvisoryOncePerFile: the advisory loop walked
// plan.Files — a list of ACTIONS — so a file carrying both a --backup action and
// its merge warned twice, reading as two independent problems.
func TestRunPlasmaApply_SessionAdvisoryOncePerFile(t *testing.T) {
	env := newPlasmaEnv(t)
	t.Setenv("KDE_FULL_SESSION", "true")
	env.writeLive(t, "plasmarc", "[Theme]\nname=breeze\n")
	env.writeRepoPlasma(t, "plasmarc", "[Theme]\nname=oxygen\n")

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t),
		"apply", "--yes", "--backup", "--only", "plasmarc")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if n := strings.Count(stderr, "plasmarc is rewritten by the running session"); n != 1 {
		t.Errorf("advisory count = %d, want 1 (once per FILE, not per action)\nstderr:\n%s", n, stderr)
	}
}
