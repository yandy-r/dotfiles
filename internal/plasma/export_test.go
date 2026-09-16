package plasma

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanExport_MissingSourceTree(t *testing.T) {
	e := NewEngine(filepath.Join(t.TempDir(), "absent"), t.TempDir(), "", nil, nil)
	_, err := e.PlanExport(DefaultFiles(), Options{})

	var target *MissingSourceTreeError
	if !errors.As(err, &target) {
		t.Fatalf("PlanExport error = %v, want *MissingSourceTreeError", err)
	}
}

func TestPlanExport_NoLiveFiles(t *testing.T) {
	e, _, _ := testEngine(t)
	_, err := e.PlanExport(DefaultFiles(), Options{})

	var target *NoLiveFilesError
	if !errors.As(err, &target) {
		t.Fatalf("PlanExport error = %v, want *NoLiveFilesError", err)
	}
}

// TestPlanExport_StripsAndSerialises proves the whole live → repo path: the
// volatile groups never reach Content, the portable keys do, and Content is
// canonical (which is why a first apply does not reflow the file).
func TestPlanExport_StripsAndSerialises(t *testing.T) {
	e, _, configDir := testEngine(t)
	seedLive(t, configDir, "kwinrc", strings.Join([]string{
		"[Desktops]",
		"Id_1=00000000-0000-4000-8000-000000000001",
		"Number=10",
		"",
		"[Tiling][00000000-0000-4000-8000-000000000001]",
		"tiles={\"width\":0.25}",
		"",
	}, "\n"))

	plan, err := e.PlanExport([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanExport error = %v", err)
	}
	act, ok := planFileFor(t, plan, "kwinrc")
	if !ok {
		t.Fatal("no action planned for kwinrc")
	}
	if act.Kind != ActionExport {
		t.Fatalf("Kind = %v, want ActionExport", act.Kind)
	}
	if !act.ToRepo {
		t.Error("an export action must write to the repo side")
	}
	got := string(act.Content)
	if got != "[Desktops]\nNumber=10\n" {
		t.Errorf("Content = %q, want only the portable [Desktops] Number", got)
	}
}

// TestPlanExport_EmptyAfterStrip covers the two halves of §6.6.1: DefaultFiles
// is a list of CANDIDATES, so a file that reduces to nothing is skipped, and a
// stale repo file that now reduces to nothing is pruned.
func TestPlanExport_EmptyAfterStrip(t *testing.T) {
	t.Run("no repo file yet", func(t *testing.T) {
		e, _, configDir := testEngine(t)
		seedLive(t, configDir, "kwinrc", "[Tiling]\npadding=4\n")

		plan, err := e.PlanExport([]FileSpec{specFor(t, "kwinrc")}, Options{})
		if err != nil {
			t.Fatalf("PlanExport error = %v", err)
		}
		act, _ := planFileFor(t, plan, "kwinrc")
		if act.Kind != ActionSkip || act.Reason != "empty after strip" {
			t.Errorf("Kind = %v (%q), want ActionSkip \"empty after strip\"", act.Kind, act.Reason)
		}
		if act.Content != nil {
			t.Error("a skip must carry no Content")
		}
	})

	t.Run("stale repo file is pruned", func(t *testing.T) {
		e, repoRoot, configDir := testEngine(t)
		seedLive(t, configDir, "kwinrc", "[Tiling]\npadding=4\n")
		repoPath := seedRepo(t, repoRoot, "kwinrc", "[Desktops]\nNumber=10\n")

		plan, err := e.PlanExport([]FileSpec{specFor(t, "kwinrc")}, Options{})
		if err != nil {
			t.Fatalf("PlanExport error = %v", err)
		}
		act, _ := planFileFor(t, plan, "kwinrc")
		if act.Kind != ActionPrune {
			t.Fatalf("Kind = %v, want ActionPrune", act.Kind)
		}

		if _, err := e.Execute(plan, Options{}); err != nil {
			t.Fatalf("Execute error = %v", err)
		}
		if _, err := os.Stat(repoPath); !os.IsNotExist(err) {
			t.Errorf("the pruned repo file still exists: %v", err)
		}
	})
}

// TestPrune_RefusesOutsideRepoFiles: a table typo must never become an
// arbitrary unlink.
func TestPrune_RefusesOutsideRepoFiles(t *testing.T) {
	e, _, _ := testEngine(t)
	victim := filepath.Join(t.TempDir(), "important")
	writeFile(t, victim, "keep me")

	err := e.doPrune(FileAction{Kind: ActionPrune, SrcAbs: victim})
	if err == nil {
		t.Fatal("doPrune outside plasma/files/ succeeded")
	}
	if _, serr := os.Stat(victim); serr != nil {
		t.Errorf("the file outside plasma/files/ was deleted: %v", serr)
	}
}

// TestPlanExport_MissingLiveFile covers the "live absent" row of §6.6.
func TestPlanExport_MissingLiveFile(t *testing.T) {
	e, _, configDir := testEngine(t)
	seedLive(t, configDir, "kwinrc", "[Desktops]\nNumber=10\n")

	plan, err := e.PlanExport([]FileSpec{specFor(t, "kwinrc"), specFor(t, "katerc")}, Options{})
	if err != nil {
		t.Fatalf("PlanExport error = %v", err)
	}
	act, _ := planFileFor(t, plan, "katerc")
	if act.Kind != ActionSkip || act.Reason != "source missing" {
		t.Errorf("Kind = %v (%q), want ActionSkip \"source missing\"", act.Kind, act.Reason)
	}
}

// TestPlanExport_RawTakesTheBytePath proves kdedefaults/package never enters the
// INI codec: it is a single bare line that would parse to an empty document and
// a rewrite would blank it.
func TestPlanExport_RawTakesTheBytePath(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	const pkg = "org.kde.breezedark.desktop\n"
	seedLive(t, configDir, "kdedefaults/package", pkg)

	spec := specFor(t, "kdedefaults/package")
	plan, err := e.PlanExport([]FileSpec{spec}, Options{})
	if err != nil {
		t.Fatalf("PlanExport error = %v", err)
	}
	act, _ := planFileFor(t, plan, "kdedefaults/package")
	if act.Kind != ActionCopyRaw {
		t.Fatalf("Kind = %v, want ActionCopyRaw", act.Kind)
	}
	if string(act.Content) != pkg {
		t.Errorf("Content = %q, want the verbatim bytes %q", act.Content, pkg)
	}

	if _, err := e.Execute(plan, Options{}); err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if got := readFile(t, filepath.Join(repoRoot, repoFilesRel, "kdedefaults", "package")); got != pkg {
		t.Errorf("exported file = %q, want %q", got, pkg)
	}

	// A second run is converged and must not reopen the file.
	plan2, err := e.PlanExport([]FileSpec{spec}, Options{})
	if err != nil {
		t.Fatalf("PlanExport error = %v", err)
	}
	act2, _ := planFileFor(t, plan2, "kdedefaults/package")
	if act2.Kind != ActionSkip {
		t.Errorf("second run Kind = %v, want ActionSkip", act2.Kind)
	}
}

// TestPlanExport_PrivacyLeak: the scan runs at PLAN time so a violation aborts
// with nothing written.
func TestPlanExport_PrivacyLeak(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedLive(t, configDir, "katerc", "[General]\nSomePath=/home/someone/notes.md\n")

	_, err := e.PlanExport([]FileSpec{specFor(t, "katerc")}, Options{})
	var leak *PrivacyLeakError
	if !errors.As(err, &leak) {
		t.Fatalf("PlanExport error = %v, want *PrivacyLeakError", err)
	}
	if leak.Path != "katerc" {
		t.Errorf("PrivacyLeakError.Path = %q, want katerc", leak.Path)
	}
	if _, serr := os.Stat(filepath.Join(repoRoot, repoFilesRel)); !os.IsNotExist(serr) {
		t.Error("PlanExport wrote into the repo despite the privacy failure")
	}
}

// TestPlanExport_WallpaperMRUIsStrippedNotRejected is the regression for the
// export that aborted with "plasmarc:5 matches /home/<user>/". The image
// wallpaper plugin appends every image picked through "Add Image…" to
// [Wallpapers] usersWallpapers as an ABSOLUTE path, so any desktop whose owner
// has ever chosen their own wallpaper tripped the §14.1 privacy scan and could
// not export at all. The strip rule is the fix; the neighbouring portable key
// still has to reach the repo.
func TestPlanExport_WallpaperMRUIsStrippedNotRejected(t *testing.T) {
	e, _, configDir := testEngine(t)
	seedLive(t, configDir, "plasmarc",
		"[PlasmaToolTips]\nDelay=-1\n\n[Wallpapers]\nusersWallpapers=/home/someone/Pictures/wallpapers/a.png\n")

	plan, err := e.PlanExport([]FileSpec{specFor(t, "plasmarc")}, Options{})
	if err != nil {
		t.Fatalf("PlanExport error = %v, want the wallpaper MRU stripped rather than rejected", err)
	}
	act, ok := planFileFor(t, plan, "plasmarc")
	if !ok || act.Kind != ActionExport {
		t.Fatalf("plasmarc action = %v, want ActionExport", act.Kind)
	}
	if got := string(act.Content); strings.Contains(got, "usersWallpapers") {
		t.Errorf("exported content still carries the wallpaper MRU:\n%s", got)
	} else if !strings.Contains(got, "Delay=-1") {
		t.Errorf("exported content lost the portable tooltip preference:\n%s", got)
	}
}

// TestDiff_WallpaperMRUIsExpectedVolatile: the same key must stop being
// reported as unmanaged live-only drift, because export can never capture it.
func TestDiff_WallpaperMRUIsExpectedVolatile(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "plasmarc", "[PlasmaToolTips]\nDelay=-1\n")
	seedLive(t, configDir, "plasmarc",
		"[PlasmaToolTips]\nDelay=-1\n\n[Wallpapers]\nusersWallpapers=/home/someone/Pictures/wallpapers/a.png\n")

	diffs, err := e.Diff([]FileSpec{specFor(t, "plasmarc")}, Options{})
	if err != nil {
		t.Fatalf("Diff error = %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("len(diffs) = %d, want 1", len(diffs))
	}
	if got := diffs[0].Keys; len(got) != 0 {
		t.Errorf("Diff reported %v, want the wallpaper MRU classified expected-volatile and dropped", got)
	}
}

// TestScanPrivacy_AllowsUnexpandedHome: "$HOME" is the PORTABLE form the design
// wants — the recommended baloofilerc folders[$e]=$HOME/ entry relies on it —
// so the scan must not reject it.
func TestScanPrivacy_AllowsUnexpandedHome(t *testing.T) {
	e, _, _ := testEngine(t)

	if err := e.scanPrivacy("baloofilerc", []byte("[General]\nfolders[$e]=$HOME/\n")); err != nil {
		t.Errorf("scanPrivacy rejected the unexpanded $HOME form: %v", err)
	}
	for _, bad := range []string{
		"[G]\nk=/home/testuser/x\n",
		"[G]\nk=/Users/testuser/x\n",
		"[G]\nk=file:///home/\n",
	} {
		if err := e.scanPrivacy("x", []byte(bad)); err == nil {
			t.Errorf("scanPrivacy accepted %q", bad)
		}
	}

	// The expanded home directory itself is caught even when it is not under
	// /home — a t.TempDir home is exactly that case.
	leaky := "[G]\nk=" + e.homeDir() + "/notes\n"
	var target *PrivacyLeakError
	if err := e.scanPrivacy("x", []byte(leaky)); !errors.As(err, &target) {
		t.Errorf("scanPrivacy(%q) error = %v, want *PrivacyLeakError", leaky, err)
	}
}

// TestPlanExport_LinkedFileIsNamed: re-stripping through a symlink is correct
// but surprising, so the plan says so.
func TestPlanExport_LinkedFileIsNamed(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	repoPath := seedRepo(t, repoRoot, "kwinrc", "[Desktops]\nNumber=10\n")
	livePath := filepath.Join(configDir, "kwinrc")
	if err := os.Symlink(repoPath, livePath); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}

	plan, err := e.PlanExport([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanExport error = %v", err)
	}
	act, _ := planFileFor(t, plan, "kwinrc")
	if act.Reason != "re-stripping a linked file" && act.Kind != ActionSkip {
		t.Errorf("Reason = %q, want the linked-file advisory", act.Reason)
	}
}

// TestExecute_LinkedExportReStrips: KDE writes volatile state into the repo
// file through the symlink, and re-exporting is how a linked setup gets
// re-cleaned. The KConfig lock is taken beside the SYMLINK, which is where
// KConfig itself takes it, and it must not survive the write.
func TestExecute_LinkedExportReStrips(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	repoPath := seedRepo(t, repoRoot, "kwinrc",
		"[Desktops]\nId_1=00000000-0000-4000-8000-000000000001\nNumber=10\n")
	livePath := filepath.Join(configDir, "kwinrc")
	if err := os.Symlink(repoPath, livePath); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}

	plan, err := e.PlanExport([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanExport error = %v", err)
	}
	if _, err := e.Execute(plan, Options{}); err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	if got := readFile(t, repoPath); got != "[Desktops]\nNumber=10\n" {
		t.Errorf("repo file = %q, want the re-stripped content", got)
	}
	if _, serr := os.Stat(livePath + ".lock"); !os.IsNotExist(serr) {
		t.Errorf("the KConfig lock survived beside the symlink: %v", serr)
	}
	// The symlink itself must survive: export writes through it, never over it.
	fi, err := os.Lstat(livePath)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the live symlink was replaced (err %v)", err)
	}
}

// TestPlanExport_Ordering pins the phase order: the kdedefaults/ overlay first,
// then kdeglobals, then everything else, path-sorted within a phase.
func TestPlanExport_Ordering(t *testing.T) {
	e, _, configDir := testEngine(t)
	for _, rel := range []string{"kwinrc", "kdeglobals", "kdedefaults/kwinrc", "baloofilerc"} {
		seedLive(t, configDir, rel, "[General]\nk=v\n")
	}

	files := []FileSpec{
		specFor(t, "kwinrc"), specFor(t, "baloofilerc"),
		specFor(t, "kdeglobals"), specFor(t, "kdedefaults/kwinrc"),
	}
	plan, err := e.PlanExport(files, Options{})
	if err != nil {
		t.Fatalf("PlanExport error = %v", err)
	}

	var got []string
	for _, act := range plan.Files {
		got = append(got, act.Spec.Path)
	}
	want := []string{"kdedefaults/kwinrc", "kdeglobals", "baloofilerc", "kwinrc"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// TestPlanExport_ColorGuardAdvisory: the engine never writes to a stream, so the
// advisory travels on the plan for the CLI to print.
func TestPlanExport_ColorGuardAdvisory(t *testing.T) {
	e, _, configDir := testEngine(t)
	seedLive(t, configDir, "kdeglobals",
		"[Colors:Header]\nBackgroundNormal=1,2,3\n\n[General]\nColorScheme=Handrolled\n")

	plan, err := e.PlanExport([]FileSpec{specFor(t, "kdeglobals")}, Options{})
	if err != nil {
		t.Fatalf("PlanExport error = %v", err)
	}
	if len(plan.Advisories) != 1 || !strings.Contains(plan.Advisories[0], "Handrolled") {
		t.Fatalf("Advisories = %v, want one naming the scheme", plan.Advisories)
	}
	act, _ := planFileFor(t, plan, "kdeglobals")
	if !strings.Contains(string(act.Content), "[Colors:Header]") {
		t.Errorf("Content = %q, want the guarded colour block kept", act.Content)
	}
}

// TestPlanExport_DryRunMutatesNothing: the guarantee that makes --dry-run
// trustworthy is Execute's first-statement short-circuit.
func TestPlanExport_DryRunMutatesNothing(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedLive(t, configDir, "kwinrc", "[Desktops]\nNumber=10\n")

	plan, err := e.PlanExport([]FileSpec{specFor(t, "kwinrc")}, Options{DryRun: true})
	if err != nil {
		t.Fatalf("PlanExport error = %v", err)
	}
	report, err := e.Execute(plan, Options{DryRun: true})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if !report.DryRun || report.Applied != nil {
		t.Errorf("report = %+v, want DryRun with nil Applied", report)
	}
	if len(report.Planned) != len(plan.Files) {
		t.Error("Planned must always be the full plan, skips included")
	}
	if _, serr := os.Stat(filepath.Join(repoRoot, repoFilesRel)); !os.IsNotExist(serr) {
		t.Error("a dry run created plasma/files/")
	}
}
