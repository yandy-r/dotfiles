package main

// CLI-level regression tests for the link/unlink round trip, the honest
// both-absent status label, and the --json failure document. They share
// plasma_test.go's hermetic plasmaEnv, and like every test there they do NOT
// call t.Parallel: each one uses t.Setenv.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunPlasma_LinkUnlinkRoundTrip drives the whole cycle through the CLI:
// --link deploys symlinks, a plain apply leaves them alone, and --unlink brings
// copy mode back. Before --unlink existed the last step was impossible — the
// linked path read the repo's own bytes, so every apply reported applied=0.
func TestRunPlasma_LinkUnlinkRoundTrip(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kiorc", liveKiorc)
	env.writeRepoPlasma(t, "kiorc", repoKiorc)
	livePath := filepath.Join(env.config, "kiorc")

	code, _, stderr := runPlasmaCapture(t, newPlasmaFake(t),
		"apply", "--link", "--backup", "--yes", "--only", "kiorc")
	if code != 0 {
		t.Fatalf("apply --link exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	if fi, err := os.Lstat(livePath); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("Lstat = %v, %v; want a symlink", fi, err)
	}

	// A plain apply over the linked tree neither refuses nor unlinks. Only the
	// applied set reaches stdout, so a converged run's buckets are empty and
	// the summary reports applied=0.
	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "apply", "--yes", "--only", "kiorc")
	if code != 0 {
		t.Fatalf("plain apply over a linked file exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stderr, "applied=0") {
		t.Errorf("stderr = %q, want applied=0", stderr)
	}
	if strings.Contains(stdout, "plasma.unlink=") {
		t.Errorf("stdout = %q, want no unlink without --unlink", stdout)
	}
	if fi, err := os.Lstat(livePath); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("a plain apply replaced the symlink (%v, %v)", fi, err)
	}

	// --unlink is the way back, and it says so on stdout.
	code, stdout, stderr = runPlasmaCapture(t, newPlasmaFake(t),
		"apply", "--unlink", "--yes", "--only", "kiorc")
	if code != 0 {
		t.Fatalf("apply --unlink exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "plasma.unlink=kiorc") {
		t.Errorf("stdout = %q, want kiorc in the unlink bucket", stdout)
	}
	fi, err := os.Lstat(livePath)
	if err != nil {
		t.Fatalf("Lstat error = %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("the live path is STILL a symlink after `apply --unlink`")
	}
	data, err := os.ReadFile(livePath) //nolint:gosec // test-owned temp path
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if !strings.Contains(string(data), "ConfirmDelete=false") {
		t.Errorf("live content = %q, want the repo's key", data)
	}
}

// TestRunPlasma_UnlinkFlagGates: --unlink is apply-only and cannot be combined
// with its own inverse. Both are usage errors, exit 2, like every other plasma
// flag rejection.
func TestRunPlasma_UnlinkFlagGates(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "unlink on diff",
			args:    []string{"diff", "--unlink"},
			wantErr: `--unlink applies only to "dotf plasma apply"`,
		},
		{
			name:    "link with unlink",
			args:    []string{"apply", "--link", "--unlink"},
			wantErr: "--link cannot be combined with --unlink",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			newPlasmaEnv(t)
			code, _, stderr := runPlasmaCapture(t, newPlasmaFake(t), tc.args...)
			if code != 2 {
				t.Fatalf("exit = %d, want 2 (stderr=%q)", code, stderr)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantErr)
			}
		})
	}
}

// TestRunPlasma_DryRunPreviewsLinkRefusal: --dry-run is what AUTHORISES a mode
// transition, so a preview that stayed silent printed a confident plan the very
// same invocation then refused with exit 1.
func TestRunPlasma_DryRunPreviewsLinkRefusal(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kiorc", liveKiorc)
	env.writeRepoPlasma(t, "kiorc", repoKiorc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t),
		"apply", "--link", "--dry-run", "--only", "kiorc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 for a preview (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "plasma.link-conflict=kiorc") {
		t.Errorf("stdout = %q, want it to name the refused file", stdout)
	}
	if !strings.Contains(stdout, "plasma.link-conflict.refusal=") ||
		!strings.Contains(stdout, "re-run with --backup") {
		t.Errorf("stdout = %q, want the exact refusal the real run would print", stdout)
	}

	// The preview's warning is true: the same run without --dry-run is refused.
	code, _, stderr = runPlasmaCapture(t, newPlasmaFake(t), "apply", "--link", "--yes", "--only", "kiorc")
	if code != 1 {
		t.Fatalf("real run exit = %d, want 1 (stderr=%q)", code, stderr)
	}

	// --backup authorises the switch, so a --backup preview has nothing to warn
	// about.
	code, stdout, stderr = runPlasmaCapture(t, newPlasmaFake(t),
		"apply", "--link", "--backup", "--dry-run", "--only", "kiorc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	if strings.Contains(stdout, "plasma.link-conflict") {
		t.Errorf("stdout = %q, want no conflict warning under --backup", stdout)
	}
}

// TestRunPlasma_DryRunJSONCarriesLinkConflicts: the machine-readable half of the
// same preview.
func TestRunPlasma_DryRunJSONCarriesLinkConflicts(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kiorc", liveKiorc)
	env.writeRepoPlasma(t, "kiorc", repoKiorc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t),
		"apply", "--link", "--dry-run", "--json", "--only", "kiorc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	var got plasmaReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	if len(got.LinkConflicts) != 1 || got.LinkConflicts[0] != "kiorc" {
		t.Errorf("linkConflicts = %v, want [kiorc]", got.LinkConflicts)
	}
}

// TestRunPlasma_StatusLabelsBothAbsentFiles: a file present in NEITHER the repo
// nor the live tree used to fall through to action="present" in JSON while the
// text renderer omitted it entirely — two renderers disagreeing about a file
// that exists nowhere.
func TestRunPlasma_StatusLabelsBothAbsentFiles(t *testing.T) {
	env := newPlasmaEnv(t)
	// kiorc exists on both sides; kwinrc exists on neither.
	env.writeLive(t, "kiorc", liveKiorc)
	env.writeRepoPlasma(t, "kiorc", liveKiorc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t),
		"status", "--json", "--only", "kiorc,kwinrc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	var got plasmaReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	actions := make(map[string]string, len(got.Files))
	for _, f := range got.Files {
		actions[f.Path] = f.Action
	}
	if actions["kwinrc"] != "absent" {
		t.Errorf("kwinrc action = %q, want absent (it exists on neither side)", actions["kwinrc"])
	}
	if actions["kiorc"] != "present" {
		t.Errorf("kiorc action = %q, want present", actions["kiorc"])
	}

	// The text renderer agrees, instead of omitting the file.
	code, stdout, stderr = runPlasmaCapture(t, newPlasmaFake(t), "status", "--only", "kiorc,kwinrc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "plasma.absent=kwinrc") {
		t.Errorf("stdout = %q, want a plasma.absent= line naming kwinrc", stdout)
	}
}

// TestRunPlasma_JSONErrorDocumentOnFailure: an error path under --json used to
// write ZERO bytes to stdout, leaving a machine consumer with nothing but an
// exit code. Every failure now emits the same envelope a success does, with the
// diagnostic in "error".
func TestRunPlasma_JSONErrorDocumentOnFailure(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, env plasmaEnv)
		args    []string
		wantErr string
	}{
		{
			name:    "export with no live files",
			setup:   func(*testing.T, plasmaEnv) {},
			args:    []string{"export", "--json", "--only", "kiorc"},
			wantErr: "no managed KDE config files found",
		},
		{
			name: "apply hitting the link-mode conflict",
			setup: func(t *testing.T, env plasmaEnv) {
				env.writeLive(t, "kiorc", liveKiorc)
				env.writeRepoPlasma(t, "kiorc", repoKiorc)
			},
			args:    []string{"apply", "--link", "--yes", "--json", "--only", "kiorc"},
			wantErr: "would switch between --link and merge mode",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newPlasmaEnv(t)
			tc.setup(t, env)

			code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), tc.args...)
			if code != 1 {
				t.Fatalf("exit = %d, want 1 (stderr=%q)", code, stderr)
			}
			if stdout == "" {
				t.Fatal("stdout is EMPTY on a --json error path")
			}
			var got plasmaReport
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
			}
			if got.Command != "plasma" {
				t.Errorf("command = %q, want plasma", got.Command)
			}
			if !strings.Contains(got.Error, tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", got.Error, tc.wantErr)
			}
			if got.Files == nil {
				t.Error("files = null, want an empty array so the shape never varies")
			}
			// stderr still carries the human diagnostic; --json adds, it does
			// not replace the operator's channel.
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want the same diagnostic", stderr)
			}
		})
	}
}

// TestRunPlasma_NoJSONMeansNoStdoutOnFailure pins the other half of the
// contract: the error document is --json's, not a new default.
func TestRunPlasma_NoJSONMeansNoStdoutOnFailure(t *testing.T) {
	newPlasmaEnv(t)
	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "export", "--only", "kiorc")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr=%q)", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing without --json", stdout)
	}
}
