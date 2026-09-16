package plasma

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanApply_MissingRepoTree(t *testing.T) {
	e, _, _ := testEngine(t)
	_, err := e.PlanApply(DefaultFiles(), Options{})

	var target *MissingRepoTreeError
	if !errors.As(err, &target) {
		t.Fatalf("PlanApply error = %v, want *MissingRepoTreeError", err)
	}
}

// TestPlanApply_PresenceMatrix walks the four rows of §6.6.
func TestPlanApply_PresenceMatrix(t *testing.T) {
	tests := []struct {
		name       string
		repo, live string
		wantKind   ActionKind
		wantReason string
	}{
		{"live absent, repo present", "[G]\nk=1\n", "", ActionCreateFile, ""},
		{"live present, repo absent", "", "[G]\nk=1\n", ActionSkip, "nothing to apply"},
		{"both present, converged", "[G]\nk=1\n", "[G]\nk=1\nextra=2\n", ActionSkip, "already converged"},
		{"both present, drifted", "[G]\nk=1\n", "[G]\nk=2\n", ActionMerge, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, repoRoot, configDir := testEngine(t)
			// plasma/files/ must exist even when this file is not in it.
			seedRepo(t, repoRoot, "keepalive", "[G]\nk=1\n")
			if tc.repo != "" {
				seedRepo(t, repoRoot, "kwinrc", tc.repo)
			}
			if tc.live != "" {
				seedLive(t, configDir, "kwinrc", tc.live)
			}

			plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
			if err != nil {
				t.Fatalf("PlanApply error = %v", err)
			}
			act, ok := planFileFor(t, plan, "kwinrc")
			if !ok {
				t.Fatal("no action planned")
			}
			if act.Kind != tc.wantKind {
				t.Errorf("Kind = %v, want %v (reason %q)", act.Kind, tc.wantKind, act.Reason)
			}
			if tc.wantReason != "" && act.Reason != tc.wantReason {
				t.Errorf("Reason = %q, want %q", act.Reason, tc.wantReason)
			}
			if act.Kind == ActionSkip && act.Content != nil {
				t.Error("a converged file must not be opened for writing: Content is non-nil")
			}
		})
	}
}

// TestExecute_MergePreservesLiveKeys is the end-to-end statement of the merge
// contract, through the real atomic-write path.
func TestExecute_MergePreservesLiveKeys(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[Desktops]\nNumber=10\nRows=1\n")
	livePath := seedLive(t, configDir, "kwinrc",
		"[Desktops]\nId_1=00000000-0000-4000-8000-000000000001\nNumber=4\n")

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	report, err := e.Execute(plan, Options{})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if len(report.Applied) != 1 {
		t.Fatalf("len(Applied) = %d, want 1", len(report.Applied))
	}

	got := readFile(t, livePath)
	for _, want := range []string{"Number=10", "Rows=1", "Id_1=00000000-0000-4000-8000-000000000001"} {
		if !strings.Contains(got, want) {
			t.Errorf("merged file %q is missing %q", got, want)
		}
	}

	// A second run is converged and applies nothing at all.
	plan2, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	report2, err := e.Execute(plan2, Options{})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if len(report2.Applied) != 0 {
		t.Errorf("a converged run applied %d actions, want 0", len(report2.Applied))
	}
}

// TestExecute_ModeAndUmask: an existing file keeps its mode, a created one gets
// KDE's 0600, and Chmod beats the process umask either way.
func TestExecute_ModePreservation(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	seedRepo(t, repoRoot, "katerc", "[G]\nk=repo\n")
	livePath := seedLive(t, configDir, "kwinrc", "[G]\nk=live\n") // writeFile uses 0644
	if err := os.Chmod(livePath, 0o644); err != nil {
		t.Fatalf("Chmod error = %v", err)
	}

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc"), specFor(t, "katerc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	if _, err := e.Execute(plan, Options{}); err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	fi, err := os.Stat(livePath)
	if err != nil {
		t.Fatalf("Stat error = %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Errorf("existing file mode = %v, want the preserved 0644", got)
	}
	created, err := os.Stat(filepath.Join(configDir, "katerc"))
	if err != nil {
		t.Fatalf("Stat error = %v", err)
	}
	if got := created.Mode().Perm(); got != 0o600 {
		t.Errorf("created file mode = %v, want KDE's 0600", got)
	}
}

// TestExecute_AtomicWriteLeavesNoTemp: the temp file must live in the same
// directory (so the rename is same-filesystem) and must not survive.
func TestExecute_AtomicWriteLeavesNoTemp(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	seedLive(t, configDir, "kwinrc", "[G]\nk=live\n")

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	if _, err := e.Execute(plan, Options{}); err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	entries, err := os.ReadDir(configDir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".dotf-plasma-") {
			t.Errorf("temp file %q survived the write", entry.Name())
		}
		if strings.HasSuffix(entry.Name(), ".lock") {
			t.Errorf("lock file %q survived the write", entry.Name())
		}
	}
}

// TestExecute_Backup: a bad merge destroys live settings that exist nowhere
// else, so every mutating action is backup-eligible.
func TestExecute_Backup(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	const original = "[G]\nk=live\n"
	seedLive(t, configDir, "kwinrc", original)

	opts := Options{Backup: true}
	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, opts)
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}

	// The backup entry must be planned BEFORE the action it protects, so an
	// in-order Execute copies the original before overwriting it.
	var order []ActionKind
	for _, act := range plan.Files {
		order = append(order, act.Kind)
	}
	if len(order) != 2 || order[0] != ActionBackup || order[1] != ActionMerge {
		t.Fatalf("plan order = %v, want [ActionBackup ActionMerge]", order)
	}
	if got := countKind(plan, ActionBackup); got != 1 {
		t.Errorf("ActionBackup count = %d, want 1", got)
	}
	if !strings.HasPrefix(plan.BackupDir, filepath.Join(configDir, "dotf")) {
		t.Errorf("BackupDir = %q, want it under <configDir>/dotf/", plan.BackupDir)
	}

	if _, err := e.Execute(plan, opts); err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if got := readFile(t, filepath.Join(plan.BackupDir, "kwinrc")); got != original {
		t.Errorf("backup = %q, want the pre-merge bytes %q", got, original)
	}
}

// TestPlanApply_BackupDirIsStable: BackupDir is computed once, at plan time —
// even without --backup — and Execute reads that field rather than deriving a
// path of its own.
//
// The contract is not decoration. The path is a timestamp, so a re-derivation
// inside Execute silently relocates the backup whenever a run crosses a second
// boundary: `dotf plasma apply --backup` then prints one path in its preview and
// writes the user's only copy of their pre-merge desktop config somewhere else,
// which is discovered when a restore is attempted and not before.
//
// Asserting the field still equals what it did before Execute is necessary but
// not sufficient — a re-derivation within the same second reproduces the same
// string. So the plan is repointed at a path no timestamp could ever produce,
// and the backup must land THERE. Reading plan.BackupDir after Execute, the way
// TestExecute_Backup does, cannot catch any of this: it follows the relocated
// value.
func TestPlanApply_BackupDirIsStable(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	const original = "[G]\nk=live\n"
	seedLive(t, configDir, "kwinrc", original)

	// Captured even without --backup, so a later --backup-less plan is still
	// printable and testable.
	bare, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	if bare.BackupDir == "" {
		t.Fatal("BackupDir is empty without --backup; it must always be captured")
	}
	if want := filepath.Join(configDir, "dotf", "plasma_backup_"); !strings.HasPrefix(bare.BackupDir, want) {
		t.Errorf("BackupDir = %q, want the %q<epoch> shape", bare.BackupDir, want)
	}

	opts := Options{Backup: true}
	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, opts)
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	pinned := filepath.Join(t.TempDir(), "pinned_backup")
	plan.BackupDir = pinned

	if _, err := e.Execute(plan, opts); err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	if plan.BackupDir != pinned {
		t.Errorf("Execute rewrote Plan.BackupDir to %q; the plan-time %q must survive the run",
			plan.BackupDir, pinned)
	}
	if got := readFile(t, filepath.Join(pinned, "kwinrc")); got != original {
		t.Errorf("backup at the plan-time dir = %q, want the pre-merge bytes %q", got, original)
	}
	// A re-derived path would be a fresh <configDir>/dotf/plasma_backup_<epoch>,
	// so that directory must not have been created at all.
	if entries, derr := os.ReadDir(filepath.Join(configDir, "dotf")); derr == nil && len(entries) > 0 {
		t.Errorf("Execute derived its own backup dir: %s holds %d entr(ies)",
			filepath.Join(configDir, "dotf"), len(entries))
	}
}

// TestPlanApply_LinkModeMatrix walks §9.2 in all three modes.
func TestPlanApply_LinkModeMatrix(t *testing.T) {
	tests := []struct {
		name         string
		live         string // "", "file", "link-repo", "link-elsewhere"
		mode         string // "plain", "link", "unlink"
		wantKind     ActionKind
		wantConflict bool
	}{
		{"regular file, merge mode", "file", "plain", ActionMerge, false},
		{"regular file, link mode", "file", "link", ActionSymlink, true},
		// --unlink over a real file has no link to undo, so it is an ordinary
		// merge and never a transition.
		{"regular file, unlink mode", "file", "unlink", ActionMerge, false},
		{"linked, link mode", "link-repo", "link", ActionSkip, false},
		// A linked file IS the repo file, so a plain apply has nothing to
		// converge and nothing to refuse — restoring copy mode is --unlink's
		// job and never a side effect.
		{"linked, merge mode", "link-repo", "plain", ActionSkip, false},
		{"linked, unlink mode", "link-repo", "unlink", ActionUnlink, false},
		// A dangling foreign link reads as an absent live file.
		{"foreign link, merge mode", "link-elsewhere", "plain", ActionCreateFile, true},
		{"foreign link, link mode", "link-elsewhere", "link", ActionSymlink, true},
		{"foreign link, unlink mode", "link-elsewhere", "unlink", ActionCreateFile, true},
		{"absent, merge mode", "", "plain", ActionCreateFile, false},
		{"absent, link mode", "", "link", ActionSymlink, false},
		{"absent, unlink mode", "", "unlink", ActionCreateFile, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			link, unlink := tc.mode == "link", tc.mode == "unlink"
			e, repoRoot, configDir := testEngine(t)
			repoPath := seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
			livePath := filepath.Join(configDir, "kwinrc")
			switch tc.live {
			case "file":
				seedLive(t, configDir, "kwinrc", "[G]\nk=live\n")
			case "link-repo":
				if err := os.Symlink(repoPath, livePath); err != nil {
					t.Fatalf("Symlink error = %v", err)
				}
			case "link-elsewhere":
				if err := os.Symlink(filepath.Join(t.TempDir(), "other"), livePath); err != nil {
					t.Fatalf("Symlink error = %v", err)
				}
			}

			// --dry-run satisfies the guard, so the plan is always returned and
			// the per-file decision is observable.
			opts := Options{Link: link, Unlink: unlink, DryRun: true}
			plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, opts)
			if err != nil {
				t.Fatalf("PlanApply error = %v", err)
			}
			act, _ := planFileFor(t, plan, "kwinrc")
			if act.Kind != tc.wantKind {
				t.Errorf("Kind = %v (%q), want %v", act.Kind, act.Reason, tc.wantKind)
			}
			// A dry run announces exactly the refusal the real run would hit.
			if got := len(plan.LinkConflicts) > 0; got != tc.wantConflict {
				t.Errorf("plan.LinkConflicts = %v, want a conflict = %v", plan.LinkConflicts, tc.wantConflict)
			}

			// Without dry-run or backup the same situation must be refused.
			_, err = e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{Link: link, Unlink: unlink})
			var conflict *LinkModeConflictError
			if got := errors.As(err, &conflict); got != tc.wantConflict {
				t.Errorf("LinkModeConflictError = %v (err %v), want %v", got, err, tc.wantConflict)
			}
			if tc.wantConflict {
				// --backup also satisfies the guard, by making the transition
				// recoverable — and a --backup preview then has nothing to warn
				// about, because that run would not be refused.
				bopts := Options{Link: link, Unlink: unlink, Backup: true}
				if _, berr := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, bopts); berr != nil {
					t.Errorf("--backup did not satisfy the guard: %v", berr)
				}
				bopts.DryRun = true
				bplan, berr := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, bopts)
				if berr != nil {
					t.Fatalf("PlanApply(--backup --dry-run) error = %v", berr)
				}
				if len(bplan.LinkConflicts) != 0 {
					t.Errorf("--backup --dry-run LinkConflicts = %v, want none", bplan.LinkConflicts)
				}
			}
		})
	}
}

// TestExecute_LinkModeReplacesFileWithSymlink is the §9.1 capability: KConfig
// writes through symlinks, so KDE then reads and writes the repo file directly.
func TestExecute_LinkModeReplacesFileWithSymlink(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	repoPath := seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	const original = "[G]\nk=live\n"
	livePath := seedLive(t, configDir, "kwinrc", original)

	opts := Options{Link: true, Backup: true}
	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, opts)
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	if _, err := e.Execute(plan, opts); err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	fi, err := os.Lstat(livePath)
	if err != nil {
		t.Fatalf("Lstat error = %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the live path is not a symlink")
	}
	target, err := os.Readlink(livePath)
	if err != nil {
		t.Fatalf("Readlink error = %v", err)
	}
	if target != repoPath {
		t.Errorf("symlink target = %q, want %q", target, repoPath)
	}
	// The displaced content is recoverable.
	if got := readFile(t, filepath.Join(plan.BackupDir, "kwinrc")); got != original {
		t.Errorf("backup = %q, want %q", got, original)
	}

	// Re-running in link mode is a no-op.
	plan2, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, opts)
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	act, _ := planFileFor(t, plan2, "kwinrc")
	if act.Kind != ActionSkip || !strings.HasPrefix(act.Reason, "already linked") {
		t.Errorf("Kind = %v (%q), want ActionSkip \"already linked…\"", act.Kind, act.Reason)
	}
}

// TestPlanApply_RecommendedOverlay: live < files < recommended.
func TestPlanApply_RecommendedOverlay(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[Windows]\nFocusStealingPreventionLevel=1\nRollOverDesktops=false\n")
	seedRecommended(t, repoRoot, "kwinrc", "[Windows]\nFocusStealingPreventionLevel=0\n")
	seedLive(t, configDir, "kwinrc", "[Windows]\nFocusStealingPreventionLevel=1\nRollOverDesktops=false\nLiveOnly=1\n")

	// Without --recommended the file is converged.
	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	if act, _ := planFileFor(t, plan, "kwinrc"); act.Kind != ActionSkip {
		t.Errorf("without --recommended Kind = %v, want ActionSkip", act.Kind)
	}

	opts := Options{Recommended: true}
	plan, err = e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, opts)
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	if _, err := e.Execute(plan, opts); err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	got := readFile(t, filepath.Join(configDir, "kwinrc"))
	if !strings.Contains(got, "FocusStealingPreventionLevel=0") {
		t.Errorf("%q: the recommended value did not win", got)
	}
	if !strings.Contains(got, "RollOverDesktops=false") {
		t.Errorf("%q: a files-only key was dropped", got)
	}
	if !strings.Contains(got, "LiveOnly=1") {
		t.Errorf("%q: --recommended deleted a live-only key", got)
	}
}

// TestExecute_ImmutableFileIsRefused: the write is refused, the run continues,
// and the failure is reported so the CLI can exit 1.
func TestExecute_ImmutableFileIsRefused(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	seedRepo(t, repoRoot, "katerc", "[G]\nk=repo\n")
	const locked = "[$i]\n[G]\nk=live\n"
	seedLive(t, configDir, "kwinrc", locked)
	seedLive(t, configDir, "katerc", "[G]\nk=live\n")

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc"), specFor(t, "katerc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	report, err := e.Execute(plan, Options{})

	var imm *ImmutableError
	if !errors.As(err, &imm) {
		t.Fatalf("Execute error = %v, want an *ImmutableError", err)
	}
	if report == nil {
		t.Fatal("Execute must return a non-nil report alongside an error")
	}
	if got := readFile(t, filepath.Join(configDir, "kwinrc")); got != locked {
		t.Errorf("the immutable file was written: %q", got)
	}
	if got := readFile(t, filepath.Join(configDir, "katerc")); !strings.Contains(got, "k=repo") {
		t.Errorf("the run stopped at the immutable file: katerc = %q", got)
	}
}
