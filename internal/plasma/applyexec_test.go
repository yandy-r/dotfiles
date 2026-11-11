package plasma

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExecute_ReMergesTheLiveFileAtWriteTime is the regression for the split
// read-modify-write cycle.
//
// The window between PlanApply and Execute is REAL and unbounded: `dotf plasma
// apply` shows a confirmation prompt in it, and `dotf apply --plasma` runs the
// two back to back with the rest of a provisioning pass in between. A running
// Plasma session writes these files continuously, so anything it wrote in that
// window used to be deleted by bytes frozen before it existed. The concurrent
// write is injected at the seam itself — between the two engine calls the test
// drives — so there is no sleep and no race.
func TestExecute_ReMergesTheLiveFileAtWriteTime(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[Windows]\nFocusPolicy=ClickToFocus\n")
	livePath := seedLive(t, configDir, "kwinrc",
		"[Windows]\nFocusPolicy=FocusFollowsMouse\nLiveOnlyKey=OLD\n")

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}

	// The running session rewrites the file while the confirmation prompt is
	// displayed: a new key, a whole new group, and a changed live-only value.
	seedLive(t, configDir, "kwinrc",
		"[Windows]\nFocusPolicy=FocusFollowsMouse\nLiveOnlyKey=NEW_FROM_KDE\nBrandNewKey=1\n"+
			"\n[FreshGroup]\nX=2\n")

	report, err := e.Execute(plan, Options{})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	got := readFile(t, livePath)
	for _, want := range []string{
		"FocusPolicy=ClickToFocus", // the repo key still wins
		"LiveOnlyKey=NEW_FROM_KDE", // the session's newer value survives
		"BrandNewKey=1",            // a key that did not exist at plan time
		"[FreshGroup]",             // a whole group that did not exist at plan time
		"X=2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the merged file lost %q:\n%s", want, got)
		}
	}

	// The user is told that what landed is not what was previewed.
	if len(report.Applied) != 1 {
		t.Fatalf("len(Applied) = %d, want 1", len(report.Applied))
	}
	if !strings.Contains(report.Applied[0].Reason, "re-merged at write time") {
		t.Errorf("Reason = %q, want it to explain the re-merge", report.Applied[0].Reason)
	}
	// Report.Applied describes what actually landed, so the reported key writes
	// are the write-time ones, not the preview's.
	if !strings.Contains(string(report.Applied[0].Content), "BrandNewKey=1") {
		t.Error("the reported Content is the stale plan-time preview")
	}
}

// TestExecute_ReMergeIsSilentWhenNothingChanged: the restatement notice is for
// a live file that actually moved, not a tax on every apply.
func TestExecute_ReMergeIsSilentWhenNothingChanged(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	seedLive(t, configDir, "kwinrc", "[G]\nk=live\nextra=1\n")

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	report, err := e.Execute(plan, Options{})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	for _, a := range report.Applied {
		if strings.Contains(a.Reason, "re-merged at write time") {
			t.Errorf("unchanged live file was reported as re-merged: %+v", a)
		}
	}
}

// TestExecute_LiveFileThatBecameImmutableIsRefused: the write-time merge is a
// full re-decision, so a lockdown applied during the window is honoured rather
// than steamrolled by plan-time bytes.
func TestExecute_LiveFileThatBecameImmutableIsRefused(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	livePath := seedLive(t, configDir, "kwinrc", "[G]\nk=live\n")

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	const locked = "[$i]\n[G]\nk=live\n"
	seedLive(t, configDir, "kwinrc", locked)

	report, err := e.Execute(plan, Options{})
	var imm *ImmutableError
	if !errors.As(err, &imm) {
		t.Fatalf("Execute error = %v, want *ImmutableError", err)
	}
	if len(report.Applied) != 0 {
		t.Errorf("Applied = %+v, want none", report.Applied)
	}
	if got := readFile(t, livePath); got != locked {
		t.Errorf("the newly-immutable file was written: %q", got)
	}
}

// TestExecute_MergeTakesTheLockAroundTheWholeCycle: the lock must cover the
// read the merge is computed from, not just the rename. Holding it must
// therefore block the whole operation and leave the file untouched.
func TestExecute_MergeTakesTheLockAroundTheWholeCycle(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	const original = "[G]\nk=live\n"
	livePath := seedLive(t, configDir, "kwinrc", original)

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}

	// A running Plasma holds the lock for the whole of Execute.
	release, err := e.acquireLock(livePath)
	if err != nil {
		t.Fatalf("acquireLock error = %v", err)
	}
	defer release()

	report, err := e.Execute(plan, Options{})
	var lockErr *LockError
	if !errors.As(err, &lockErr) {
		t.Fatalf("Execute error = %v, want *LockError", err)
	}
	if len(report.Applied) != 0 {
		t.Errorf("Applied = %+v, want none", report.Applied)
	}
	if got := readFile(t, livePath); got != original {
		t.Errorf("the file was written while its lock was held: %q", got)
	}
}

// TestExecute_BackupFailureAbortsTheWriteItProtects: --backup is what
// AUTHORISES a destructive step, so a failed copy must stop that step. The
// backup write is made to fail by occupying its destination path with a
// directory — the shape of any real ENOSPC/EACCES/EIO failure, without needing
// either.
func TestExecute_BackupFailureAbortsTheWriteItProtects(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	const original = "[G]\nk=live\nPrecious=irreplaceable\n"
	livePath := seedLive(t, configDir, "kwinrc", original)

	opts := Options{Backup: true}
	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, opts)
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(plan.BackupDir, "kwinrc"), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	report, err := e.Execute(plan, opts)
	if got := readFile(t, livePath); got != original {
		t.Errorf("the file was modified without a backup: %q", got)
	}
	var refused *BackupRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("Execute error = %v, want *BackupRefusedError", err)
	}
	if refused.Path != "kwinrc" {
		t.Errorf("BackupRefusedError.Path = %q, want kwinrc", refused.Path)
	}
	for _, act := range report.Applied {
		if act.Kind == ActionMerge {
			t.Error("the unprotected merge was reported as applied")
		}
	}
}

// TestExecute_BackupFailureDoesNotAbortOtherFiles: the refusal is scoped to the
// file whose backup failed; the run still completes for everything else.
func TestExecute_BackupFailureDoesNotAbortOtherFiles(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	seedRepo(t, repoRoot, "katerc", "[G]\nk=repo\n")
	seedLive(t, configDir, "kwinrc", "[G]\nk=live\n")
	seedLive(t, configDir, "katerc", "[G]\nk=live\n")

	opts := Options{Backup: true}
	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc"), specFor(t, "katerc")}, opts)
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(plan.BackupDir, "kwinrc"), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	if _, err := e.Execute(plan, opts); err == nil {
		t.Fatal("Execute error = nil, want the backup failure reported")
	}
	if got := readFile(t, filepath.Join(configDir, "katerc")); !strings.Contains(got, "k=repo") {
		t.Errorf("the run stopped at the failed backup: katerc = %q", got)
	}
}

// TestExecute_WriteFailureDoesNotAbortTheRemainingFiles is the aggregate-and-
// continue contract stated in Execute's doc comment: "a per-file failure never
// aborts the remaining files".
//
// Nothing else in the suite injects a filesystem EXECUTION failure.
// TestExecute_ImmutableFileIsRefused looks like it does and does not — an
// *ImmutableError is set at PLAN time as an ActionSkip's Err, so it takes the
// skip arm and the executeAction error arm is never entered. Replacing the
// aggregation with a fail-fast `return` therefore left the whole suite green,
// while a real run against a config dir holding one root-owned or read-only
// file — the ordinary state after a distro upgrade seeds /etc defaults, or after
// a sudo-run KDE tool — would stop mid-pass and leave the desktop half applied:
// the phase-0 kdedefaults/ overlay written, kdeglobals and everything in phase 2
// not, with no error naming what was skipped.
//
// So the failing file is deliberately the FIRST entry of a three-phase plan, and
// the two survivors sit in later phases.
func TestExecute_WriteFailureDoesNotAbortTheRemainingFiles(t *testing.T) {
	const failing = "kdedefaults/kwinrc"
	tests := []struct {
		name string
		// inject makes the write of <configDir>/kdedefaults/kwinrc fail. It runs
		// at the plan/execute seam, so the plan is identical in both cases and
		// only the filesystem differs.
		inject func(t *testing.T, configDir string)
		// wantErr matches the aggregated error, proving the failure is REPORTED
		// rather than absorbed into a silent success.
		wantErr func(error) bool
	}{
		{
			// The faithful shape of the real report: an EACCES on the directory
			// the write must create its lock and temp file in.
			name: "unwritable parent directory",
			inject: func(t *testing.T, configDir string) {
				if os.Geteuid() == 0 {
					t.Skip("root ignores directory permissions; the EISDIR case covers this contract")
				}
				dir := filepath.Join(configDir, "kdedefaults")
				t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
				if err := os.Chmod(dir, 0o500); err != nil {
					t.Fatalf("Chmod error = %v", err)
				}
			},
			wantErr: func(err error) bool {
				return strings.Contains(err.Error(), "kdedefaults") &&
					strings.Contains(err.Error(), "permission denied")
			},
		},
		{
			// The same failure without needing an unprivileged uid: a read on a
			// directory fails with EISDIR for every user, root included.
			name: "destination replaced by a directory",
			inject: func(t *testing.T, configDir string) {
				path := filepath.Join(configDir, "kdedefaults", "kwinrc")
				if err := os.Remove(path); err != nil {
					t.Fatalf("Remove error = %v", err)
				}
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatalf("MkdirAll error = %v", err)
				}
			},
			wantErr: func(err error) bool {
				var unreadable *UnreadableFileError
				return errors.As(err, &unreadable) && strings.HasSuffix(unreadable.Path, failing)
			},
		},
	}

	// Phases 0, 1 and 2 respectively, so the survivors are everything the plan
	// reaches AFTER the failure.
	paths := []string{failing, "kdeglobals", "kwinrc"}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, repoRoot, configDir := testEngine(t)
			var specs []FileSpec
			for _, p := range paths {
				seedRepo(t, repoRoot, p, "[G]\nk=repo\n")
				seedLive(t, configDir, p, "[G]\nk=live\n")
				specs = append(specs, specFor(t, p))
			}

			plan, err := e.PlanApply(specs, Options{})
			if err != nil {
				t.Fatalf("PlanApply error = %v", err)
			}
			if len(plan.Files) != 3 || plan.Files[0].Spec.Path != failing {
				t.Fatalf("plan = %v…, want %s first of three", plan.Files[0].Spec.Path, failing)
			}
			tc.inject(t, configDir)

			report, err := e.Execute(plan, Options{})
			if err == nil {
				t.Fatal("Execute error = nil; the per-file failure was swallowed")
			}
			if !tc.wantErr(err) {
				t.Errorf("Execute error = %v, want the %s failure reported", err, failing)
			}
			if report == nil {
				t.Fatal("Execute must return a non-nil report alongside an error")
			}

			// Everything after the failure still ran.
			for _, p := range paths[1:] {
				if got := readFile(t, filepath.Join(configDir, p)); !strings.Contains(got, "k=repo") {
					t.Errorf("the run stopped at %s: %s = %q", failing, p, got)
				}
			}
			if len(report.Applied) != len(paths)-1 {
				t.Errorf("len(Applied) = %d, want %d", len(report.Applied), len(paths)-1)
			}
			for _, act := range report.Applied {
				if act.Spec.Path == failing {
					t.Errorf("%s was reported as applied despite failing", failing)
				}
			}
			if len(report.Planned) != len(paths) {
				t.Errorf("len(Planned) = %d, want the whole plan (%d)", len(report.Planned), len(paths))
			}
		})
	}
}

// TestExecute_RawWritesTheLiveFile covers the ActionCopyRaw (ToRepo=false) arm:
// the only FormatRaw member is kdedefaults/package, whose whole content is the
// global look-and-feel package name.
func TestExecute_RawWritesTheLiveFile(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	const want = "org.kde.breezedark.desktop\n"
	seedRepo(t, repoRoot, "kdedefaults/package", want)
	seedLive(t, configDir, "kdedefaults/package", "org.kde.breeze.desktop\n")

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kdedefaults/package")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	report, err := e.Execute(plan, Options{})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if len(report.Applied) != 1 || report.Applied[0].Kind != ActionCopyRaw {
		t.Fatalf("Applied = %+v, want one ActionCopyRaw", report.Applied)
	}
	if got := readFile(t, filepath.Join(configDir, "kdedefaults", "package")); got != want {
		t.Errorf("live package = %q, want %q", got, want)
	}
}

// TestExecute_RawRechecksConvergenceUnderTheLock: the DECISION to overwrite is
// re-made at write time too, so a file the session brought into agreement
// during the confirmation window is left alone instead of rewritten.
func TestExecute_RawRechecksConvergenceUnderTheLock(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	const want = "org.kde.breezedark.desktop\n"
	seedRepo(t, repoRoot, "kdedefaults/package", want)
	livePath := seedLive(t, configDir, "kdedefaults/package", "org.kde.breeze.desktop\n")

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kdedefaults/package")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	if act, _ := planFileFor(t, plan, "kdedefaults/package"); act.Kind != ActionCopyRaw {
		t.Fatalf("planned Kind = %v, want ActionCopyRaw", act.Kind)
	}
	seedLive(t, configDir, "kdedefaults/package", want)

	report, err := e.Execute(plan, Options{})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if len(report.Applied) != 0 {
		t.Errorf("Applied = %+v, want none: the file converged before the write", report.Applied)
	}
	if got := readFile(t, livePath); got != want {
		t.Errorf("live package = %q, want %q", got, want)
	}
}
