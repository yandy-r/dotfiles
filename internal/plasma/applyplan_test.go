package plasma

// Plan-side regressions for `dotf plasma apply`: what the planner does when a
// managed file cannot be read, when plasma/recommended/ is the only source,
// when --link and --recommended are combined, on the FormatRaw path, and when a
// group carries a KConfig lockdown marker. The plan/execute split of apply.go
// and applyexec.go is mirrored here.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPlanApply_UnreadableLiveFileIsNotTreatedAsAbsent is the regression for
// the read-error/absence conflation.
//
// `if err != nil { it must not be there }` on the live read planned a wholesale
// ActionCreateFile over a file that WAS there — destroying every live-only key
// in it — and --backup could not save it, because needsBackup excludes a
// create. A directory at the live path (EISDIR) is the deterministic,
// uid-independent shape of "present but unreadable"; the chmod-000 case below
// is the one users actually hit.
func TestPlanApply_UnreadableLiveFileIsNotTreatedAsAbsent(t *testing.T) {
	assertRefused := func(t *testing.T, e *Engine, configDir string) {
		t.Helper()
		plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
		if err != nil {
			t.Fatalf("PlanApply error = %v", err)
		}
		act, ok := planFileFor(t, plan, "kwinrc")
		if !ok {
			t.Fatal("no action planned")
		}
		if act.Kind != ActionSkip {
			t.Fatalf("Kind = %v (%q), want ActionSkip: an unreadable file must never be recreated",
				act.Kind, act.Reason)
		}
		var unreadable *UnreadableFileError
		if !errors.As(act.Err, &unreadable) {
			t.Fatalf("act.Err = %v, want *UnreadableFileError", act.Err)
		}

		report, execErr := e.Execute(plan, Options{})
		if !errors.As(execErr, &unreadable) {
			t.Errorf("Execute error = %v, want the failure reported", execErr)
		}
		if len(report.Applied) != 0 {
			t.Errorf("Applied = %+v, want none", report.Applied)
		}
		if _, statErr := os.Lstat(filepath.Join(configDir, "kwinrc")); statErr != nil {
			t.Errorf("the unreadable live path was replaced: %v", statErr)
		}
	}

	t.Run("a directory where a file is expected", func(t *testing.T) {
		e, repoRoot, configDir := testEngine(t)
		seedRepo(t, repoRoot, "kwinrc", "[Windows]\nFocusPolicy=ClickToFocus\n")
		if err := os.MkdirAll(filepath.Join(configDir, "kwinrc"), 0o755); err != nil {
			t.Fatalf("MkdirAll error = %v", err)
		}
		assertRefused(t, e, configDir)
	})

	t.Run("an unreadable mode", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads a 0000 file regardless of mode")
		}
		e, repoRoot, configDir := testEngine(t)
		seedRepo(t, repoRoot, "kwinrc", "[Windows]\nFocusPolicy=ClickToFocus\n")
		livePath := seedLive(t, configDir, "kwinrc",
			"[Windows]\nFocusPolicy=FocusFollowsMouse\n\n[Precious]\nIrreplaceable=live-only\n")
		if err := os.Chmod(livePath, 0o000); err != nil {
			t.Fatalf("Chmod error = %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(livePath, 0o600) })
		assertRefused(t, e, configDir)
	})
}

// TestPlanApply_UnreadableRepoFileIsNotSilentlySkipped: the same distinction on
// the repo side. An unreadable plasma/files/ entry means "apply nothing at
// all" if it is mistaken for absence — a silently incomplete provisioning run.
func TestPlanApply_UnreadableRepoFileIsNotSilentlySkipped(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "keepalive", "[G]\nk=1\n")
	if err := os.MkdirAll(filepath.Join(repoRoot, repoFilesRel, "kwinrc"), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	seedLive(t, configDir, "kwinrc", "[G]\nk=live\n")

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	act, _ := planFileFor(t, plan, "kwinrc")
	var unreadable *UnreadableFileError
	if act.Kind != ActionSkip || !errors.As(act.Err, &unreadable) {
		t.Fatalf("Kind = %v, Err = %v, want ActionSkip with *UnreadableFileError", act.Kind, act.Err)
	}
}

// TestPlanApply_RecommendedOnlyFileIsAFirstClassSource is the regression for
// the overlay being resolved AFTER the "is there a plasma/files/ copy?" check.
//
// Four of the ten plasma/recommended/ files (baloofilerc, klipperrc,
// ksmserverrc, plasmarc) have no exported snapshot, so `apply --recommended`
// skipped them with "nothing to apply" and exited 0 — the whole baloo indexing
// overlay, klipper's history limits and ksmserver's login mode never landed.
func TestPlanApply_RecommendedOnlyFileIsAFirstClassSource(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "keepalive", "[G]\nk=1\n")
	seedRecommended(t, repoRoot, "baloofilerc", "[Basic Settings]\nIndexing-Enabled=false\n")

	// Without --recommended there is genuinely nothing to apply.
	plan, err := e.PlanApply([]FileSpec{specFor(t, "baloofilerc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	if act, _ := planFileFor(t, plan, "baloofilerc"); act.Kind != ActionSkip {
		t.Errorf("without --recommended Kind = %v, want ActionSkip", act.Kind)
	}

	opts := Options{Recommended: true}
	plan, err = e.PlanApply([]FileSpec{specFor(t, "baloofilerc")}, opts)
	if err != nil {
		t.Fatalf("PlanApply --recommended error = %v", err)
	}
	act, _ := planFileFor(t, plan, "baloofilerc")
	if act.Kind != ActionCreateFile {
		t.Fatalf("Kind = %v (%q), want ActionCreateFile", act.Kind, act.Reason)
	}
	if _, err := e.Execute(plan, opts); err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if got := readFile(t, filepath.Join(configDir, "baloofilerc")); !strings.Contains(got, "Indexing-Enabled=false") {
		t.Errorf("baloofilerc = %q, want the recommended key", got)
	}
}

// TestPlanApply_RecommendedOnlyFileMergesIntoALiveFile: the same file with live
// state present must MERGE, not be recreated — the overlay is a partial
// document and every live-only key has to survive it.
func TestPlanApply_RecommendedOnlyFileMergesIntoALiveFile(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "keepalive", "[G]\nk=1\n")
	seedRecommended(t, repoRoot, "klipperrc", "[General]\nMaxClipItems=20\n")
	seedLive(t, configDir, "klipperrc", "[General]\nMaxClipItems=7\nKeepClipboardContents=true\n")

	opts := Options{Recommended: true}
	plan, err := e.PlanApply([]FileSpec{specFor(t, "klipperrc")}, opts)
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	if act, _ := planFileFor(t, plan, "klipperrc"); act.Kind != ActionMerge {
		t.Fatalf("Kind = %v, want ActionMerge", act.Kind)
	}
	if _, err := e.Execute(plan, opts); err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	got := readFile(t, filepath.Join(configDir, "klipperrc"))
	if !strings.Contains(got, "MaxClipItems=20") {
		t.Errorf("%q: the recommended value did not land", got)
	}
	if !strings.Contains(got, "KeepClipboardContents=true") {
		t.Errorf("%q: a live-only key was deleted", got)
	}
}

// TestPlanApply_LinkWithRecommendedIsRefused pins the resolution of the
// --link/--recommended asymmetry: a linked live path IS plasma/files/<path>, so
// the overlay has nowhere to land, while Diff honours --recommended
// and reported every recommended key as permanent, unfixable drift. The
// combination is refused before anything is touched.
func TestPlanApply_LinkWithRecommendedIsRefused(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[Windows]\nFocusPolicy=ClickToFocus\n")
	seedRecommended(t, repoRoot, "kwinrc", "[Windows]\nBorderlessMaximizedWindows=true\n")
	livePath := seedLive(t, configDir, "kwinrc", "[Windows]\nFocusPolicy=FocusFollowsMouse\n")

	_, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")},
		Options{Link: true, Recommended: true, Backup: true})
	var refused *LinkRecommendedError
	if !errors.As(err, &refused) {
		t.Fatalf("PlanApply error = %v, want *LinkRecommendedError", err)
	}
	// The refusal is pre-flight: nothing was linked or written.
	fi, statErr := os.Lstat(livePath)
	if statErr != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("the live file was replaced despite the refusal (err %v)", statErr)
	}

	// Either flag alone is still fine.
	if _, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{Link: true, Backup: true}); err != nil {
		t.Errorf("--link alone error = %v, want nil", err)
	}
	if _, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{Recommended: true}); err != nil {
		t.Errorf("--recommended alone error = %v, want nil", err)
	}
}

// TestPlanApply_RawPresenceMatrix walks the FormatRaw apply path, which is the
// only way ~/.config/kdedefaults/package — the global look-and-feel package
// name — is ever provisioned.
func TestPlanApply_RawPresenceMatrix(t *testing.T) {
	const repoPkg = "org.kde.breezedark.desktop\n"
	tests := []struct {
		name       string
		repo, live string
		liveIsDir  bool
		wantKind   ActionKind
		wantReason string
		wantMode   os.FileMode
	}{
		{"live absent", repoPkg, "", false, ActionCopyRaw, "verbatim copy", livePerm},
		{"live differs", repoPkg, "org.kde.breeze.desktop\n", false, ActionCopyRaw, "verbatim copy", 0o644},
		{"live matches", repoPkg, repoPkg, false, ActionSkip, "already converged", 0},
		{"repo absent", "", repoPkg, false, ActionSkip, "nothing to apply", 0},
		{"live unreadable", repoPkg, "", true, ActionSkip, "live file unreadable", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, repoRoot, configDir := testEngine(t)
			seedRepo(t, repoRoot, "keepalive", "[G]\nk=1\n")
			if tc.repo != "" {
				seedRepo(t, repoRoot, "kdedefaults/package", tc.repo)
			}
			if tc.live != "" {
				seedLive(t, configDir, "kdedefaults/package", tc.live)
			}
			if tc.liveIsDir {
				if err := os.MkdirAll(filepath.Join(configDir, "kdedefaults", "package"), 0o755); err != nil {
					t.Fatalf("MkdirAll error = %v", err)
				}
			}

			plan, err := e.PlanApply([]FileSpec{specFor(t, "kdedefaults/package")}, Options{})
			if err != nil {
				t.Fatalf("PlanApply error = %v", err)
			}
			act, ok := planFileFor(t, plan, "kdedefaults/package")
			if !ok {
				t.Fatal("no action planned")
			}
			if act.Kind != tc.wantKind || act.Reason != tc.wantReason {
				t.Fatalf("Kind = %v (%q), want %v (%q)", act.Kind, act.Reason, tc.wantKind, tc.wantReason)
			}
			if tc.wantKind == ActionCopyRaw {
				if string(act.Content) != tc.repo {
					t.Errorf("Content = %q, want the repo bytes %q", act.Content, tc.repo)
				}
				if act.Mode != tc.wantMode {
					t.Errorf("Mode = %v, want %v", act.Mode, tc.wantMode)
				}
			} else if act.Content != nil {
				t.Errorf("a skipped raw file carries Content %q", act.Content)
			}
			if tc.name == "live unreadable" {
				var unreadable *UnreadableFileError
				if !errors.As(act.Err, &unreadable) {
					t.Errorf("act.Err = %v, want *UnreadableFileError", act.Err)
				}
			}
		})
	}
}

// TestPlanApply_LockdownGroupConverges is the end-to-end statement of the
// canonical-flag comparison. A group marked immutable on its HEADER — a distro
// or kiosk lockdown, and how a KDE file looks before its first rewrite — has
// its [$i] folded onto every entry by the writer, so the exported repo copy and
// the live file described the same state in two different notations. Compared
// verbatim, that was permanent drift: diff exited 1 forever and apply reopened
// the file on every run while refusing to write the group.
func TestPlanApply_LockdownGroupConverges(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	// The repo copy has been through Write(); the live file has not.
	seedRepo(t, repoRoot, "kwinrc", "[Windows][$i]\nFocusPolicy[$i]=FocusFollowsMouse\n")
	seedLive(t, configDir, "kwinrc", "[Windows][$i]\nFocusPolicy=FocusFollowsMouse\n")
	files := []FileSpec{specFor(t, "kwinrc")}

	plan, err := e.PlanApply(files, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	act, _ := planFileFor(t, plan, "kwinrc")
	if act.Kind != ActionSkip || act.Reason != "already converged" {
		t.Errorf("Kind = %v (%q), want ActionSkip \"already converged\"", act.Kind, act.Reason)
	}
	if act.Err != nil {
		t.Errorf("act.Err = %v, want nil: a converged file is never opened for writing", act.Err)
	}
	if _, err := e.Diff(files, Options{}); err != nil {
		t.Errorf("Diff error = %v, want nil: identical values differed only in flag notation", err)
	}
}
