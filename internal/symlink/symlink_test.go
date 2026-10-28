package symlink

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yandy-r/dotfiles/internal/platform"
)

// planActionFor returns the first action in plan whose spec dest equals dest,
// or a zero Action and false when none matches.
func planActionFor(p *Plan, dest string, kind ActionKind) (Action, bool) {
	for _, a := range p.Actions {
		if a.Spec.Dest == dest && a.Kind == kind {
			return a, true
		}
	}
	return Action{}, false
}

func TestPlan_IdenticalSet_PerOS(t *testing.T) {
	repoRoot := t.TempDir()
	buildFullSourceTree(t, repoRoot)

	tests := []struct {
		name        string
		os          platform.OS
		ld          bool
		wantCreated int
		mustHave    []string
		mustNotHave []string
	}{
		{
			name: "macos", os: platform.OSMacOS, wantCreated: 33,
			mustHave:    []string{".bashrc", ".config/aerospace", ".config/herdr/config.toml", ".config/espanso", ".config/zed/settings.json"},
			mustNotHave: []string{".config/hypr", ".config/systemd/user/tmux.service"},
		},
		{
			name: "arch-no-desktop", os: platform.OSArch, wantCreated: 33,
			mustHave:    []string{".config/herdr/config.toml", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace", ".config/hypr"},
		},
		{
			name: "arch-desktop", os: platform.OSArch, ld: true, wantCreated: 44,
			mustHave: []string{".config/hypr", ".local/share/applications/wifi-tui.desktop",
				".local/share/applications/arch-update.desktop", ".config/herdr/config.toml", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace"},
		},
		{
			name: "debian-no-desktop", os: platform.OSDebian, wantCreated: 33,
			mustHave:    []string{".bashrc", ".config/herdr/config.toml", ".config/nvim", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace", ".config/hypr"},
		},
		{
			// Still 39 after ADR-12, but for a different reason: tmux.service is
			// gained (+1) exactly as the Arch-only arch-update.desktop is lost
			// (-1). Spelling that out so the unchanged count does not read as a
			// false pass.
			name: "debian-desktop", os: platform.OSDebian, ld: true, wantCreated: 43,
			mustHave:    []string{".config/herdr/config.toml", ".config/hypr", ".config/waybar", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace", ".local/share/applications/arch-update.desktop"},
		},
		{
			name: "generic-linux", os: platform.OSLinux, ld: true, wantCreated: 32,
			mustHave:    []string{".config/herdr/config.toml", ".config/espanso"},
			mustNotHave: []string{".config/hypr", ".config/aerospace"},
		},
		{
			name: "redhat-no-desktop", os: platform.OSRedHat, wantCreated: 33,
			mustHave:    []string{".bashrc", ".config/herdr/config.toml", ".config/nvim", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace", ".config/hypr", ".local/share/applications/arch-update.desktop"},
		},
		{
			name: "suse-no-desktop", os: platform.OSSUSE, wantCreated: 33,
			mustHave:    []string{".config/herdr/config.toml", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace", ".config/hypr"},
		},
		{
			name: "wsl", os: platform.OSWSL, wantCreated: 33,
			mustHave:    []string{".config/herdr/config.toml", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace", ".config/hypr"},
		},
		{
			name: "windows-excludes-herdr", os: platform.OSWindows, wantCreated: 30,
			mustNotHave: []string{".config/herdr/config.toml", ".config/espanso", ".config/aerospace", ".config/hypr"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			eng := NewEngine(repoRoot, home)
			// DryRun keeps the Hyprland-set guard satisfied without backups.
			opts := Options{ManageHyprland: tt.ld, DryRun: true}

			plan, err := eng.Plan(DefaultTable(), platform.Info{OS: tt.os}, opts)
			if err != nil {
				t.Fatalf("Plan() error = %v, want nil", err)
			}

			created := createdDests(plan)
			if len(created) != tt.wantCreated {
				t.Errorf("created dests = %d, want %d", len(created), tt.wantCreated)
			}
			for _, d := range tt.mustHave {
				if !created[d] {
					t.Errorf("created set missing %q", d)
				}
			}
			for _, d := range tt.mustNotHave {
				if created[d] {
					t.Errorf("created set unexpectedly contains %q", d)
				}
			}
		})
	}
}

func TestPlan_SourceMissing_Skips(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	// Only .bashrc source exists; every other source is missing.
	writeFile(t, filepath.Join(repoRoot, "bash/bashrc"), []byte("rc"))
	// A pre-existing real file at a missing-source dest must be preserved.
	writeFile(t, filepath.Join(home, ".zshrc"), []byte("user zshrc"))

	eng := NewEngine(repoRoot, home)
	plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}

	if _, ok := planActionFor(plan, ".zshrc", ActionSkip); !ok {
		t.Error("expected ActionSkip for .zshrc (source missing)")
	}
	// No remove/link/backup should target .zshrc.
	if _, ok := planActionFor(plan, ".zshrc", ActionRemove); ok {
		t.Error("unexpected ActionRemove for .zshrc — user file must be preserved")
	}
	if _, ok := planActionFor(plan, ".bashrc", ActionLink); !ok {
		t.Error("expected ActionLink for .bashrc (source present)")
	}
}

func TestPlan_BrokenSymlink_Repointed(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "bash/bashrc"), []byte("rc"))
	// Dangling symlink at .bashrc -> nonexistent target.
	mkSymlink(t, filepath.Join(home, "gone"), filepath.Join(home, ".bashrc"))

	eng := NewEngine(repoRoot, home)
	plan, err := eng.Plan(onlyBashSpec(), platform.Info{OS: platform.OSDebian}, Options{})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}

	act, ok := planActionFor(plan, ".bashrc", ActionLink)
	if !ok {
		t.Fatal("expected ActionLink (repoint) for dangling .bashrc symlink")
	}
	if act.Reason != "repoint" {
		t.Errorf("reason = %q, want %q", act.Reason, "repoint")
	}
}

func TestPlan_WrongTarget_Repointed(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "bash/bashrc"), []byte("rc"))
	writeFile(t, filepath.Join(home, "other"), []byte("other"))
	mkSymlink(t, filepath.Join(home, "other"), filepath.Join(home, ".bashrc"))

	eng := NewEngine(repoRoot, home)
	plan, err := eng.Plan(onlyBashSpec(), platform.Info{OS: platform.OSDebian}, Options{})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}

	act, ok := planActionFor(plan, ".bashrc", ActionLink)
	if !ok {
		t.Fatal("expected ActionLink (repoint) for wrong-target .bashrc symlink")
	}
	if act.Reason != "repoint" {
		t.Errorf("reason = %q, want %q", act.Reason, "repoint")
	}
}

func TestPlan_CorrectSymlink_Idempotent(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	src := filepath.Join(repoRoot, "bash/bashrc")
	writeFile(t, src, []byte("rc"))
	mkSymlink(t, src, filepath.Join(home, ".bashrc"))

	eng := NewEngine(repoRoot, home)
	plan, err := eng.Plan(onlyBashSpec(), platform.Info{OS: platform.OSDebian}, Options{})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}

	act, ok := planActionFor(plan, ".bashrc", ActionSkip)
	if !ok {
		t.Fatal("expected ActionSkip for an already-correct symlink")
	}
	if act.Reason != "already correct" {
		t.Errorf("reason = %q, want %q", act.Reason, "already correct")
	}
}

// TestPlan_BackupAction: with Backup on, every real file or dir the plan
// replaces gets an ActionBackup — including dests like .ssh/config and
// .ansible.cfg that the retired per-entry eligibility set left uncopied.
func TestPlan_BackupAction(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	buildFullSourceTree(t, repoRoot)
	dests := []string{".bashrc", ".config/nvim", ".ansible.cfg", ".ssh/config"}
	for _, dest := range dests {
		writeFile(t, filepath.Join(home, dest), []byte("old "+dest))
	}

	eng := NewEngine(repoRoot, home)
	plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{Backup: true})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}

	for _, dest := range dests {
		if _, ok := planActionFor(plan, dest, ActionBackup); !ok {
			t.Errorf("expected ActionBackup for real file at %s", dest)
		}
		if _, ok := planActionFor(plan, dest, ActionRemove); !ok {
			t.Errorf("expected ActionRemove for real file at %s", dest)
		}
	}
}

func TestPlan_BackupOff_NoBackupAction(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	buildFullSourceTree(t, repoRoot)
	writeFile(t, filepath.Join(home, ".bashrc"), []byte("old bashrc"))

	eng := NewEngine(repoRoot, home)
	plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{Backup: false})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}

	if _, ok := planActionFor(plan, ".bashrc", ActionBackup); ok {
		t.Error("unexpected ActionBackup with Backup=false")
	}
	if _, ok := planActionFor(plan, ".bashrc", ActionRemove); !ok {
		t.Error("expected ActionRemove for real .bashrc even without backup")
	}
}

func TestPlan_LinuxDesktopGuard(t *testing.T) {
	repoRoot := t.TempDir()
	buildFullSourceTree(t, repoRoot)

	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{"ld-on-no-dryrun-no-backup", Options{ManageHyprland: true}, true},
		{"ld-on-dryrun", Options{ManageHyprland: true, DryRun: true}, false},
		{"ld-on-backup", Options{ManageHyprland: true, Backup: true}, false},
		{"ld-off", Options{ManageHyprland: false}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			eng := NewEngine(repoRoot, home)
			plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSArch}, tt.opts)

			if tt.wantErr {
				var guard *LinuxDesktopGuardError
				if !errors.As(err, &guard) {
					t.Fatalf("Plan() error = %v, want LinuxDesktopGuardError", err)
				}
				if plan != nil {
					t.Errorf("Plan() = %+v, want nil on guard failure", plan)
				}
				return
			}
			if err != nil {
				t.Fatalf("Plan() error = %v, want nil", err)
			}
		})
	}
}

func TestPlan_LinuxDesktopGuard_NonLinuxUnaffected(t *testing.T) {
	repoRoot := t.TempDir()
	buildFullSourceTree(t, repoRoot)
	home := t.TempDir()

	eng := NewEngine(repoRoot, home)
	// macOS is not Linux-family, so the guard never trips even with ld on.
	if _, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSMacOS},
		Options{ManageHyprland: true}); err != nil {
		t.Fatalf("Plan() error = %v, want nil on macOS", err)
	}
}

func TestPlan_UnsafeDest(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	eng := NewEngine(repoRoot, home)

	tests := []struct {
		name string
		dest string
	}{
		{"parent-traversal", "../evil"},
		{"nested-traversal", ".config/../../evil"},
		{"absolute", "/etc/passwd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specs := []LinkSpec{{Src: "x", Dest: tt.dest, Facet: FacetAll}}
			plan, err := eng.Plan(specs, platform.Info{OS: platform.OSDebian}, Options{})

			var unsafe *UnsafeDestError
			if !errors.As(err, &unsafe) {
				t.Fatalf("Plan() error = %v, want UnsafeDestError", err)
			}
			if plan != nil {
				t.Errorf("Plan() = %+v, want nil on unsafe dest", plan)
			}
		})
	}
}

func TestPlan_MissingSourceTree(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "does-not-exist")
	home := t.TempDir()
	eng := NewEngine(repoRoot, home)

	plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{})
	var missing *MissingSourceTreeError
	if !errors.As(err, &missing) {
		t.Fatalf("Plan() error = %v, want MissingSourceTreeError", err)
	}
	if plan != nil {
		t.Errorf("Plan() = %+v, want nil on missing source tree", plan)
	}
	if missing.Root != repoRoot {
		t.Errorf("MissingSourceTreeError.Root = %q, want %q", missing.Root, repoRoot)
	}
}

func TestPlan_BackupDir_Deterministic(t *testing.T) {
	repoRoot := t.TempDir()
	buildFullSourceTree(t, repoRoot)
	home := t.TempDir()
	eng := NewEngine(repoRoot, home)

	plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}
	wantPrefix := filepath.Join(home, ".config", "dotfiles_backup_")
	if len(plan.BackupDir) <= len(wantPrefix) || plan.BackupDir[:len(wantPrefix)] != wantPrefix {
		t.Errorf("BackupDir = %q, want prefix %q", plan.BackupDir, wantPrefix)
	}
}

// TestNewEngine_EmptyRoot_MissingSourceTree: an empty repo root must not be
// absolutized to the working directory; Plan still reports the missing tree.
func TestNewEngine_EmptyRoot_MissingSourceTree(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cwd, "bash/bashrc"), []byte("rc"))
	t.Chdir(cwd)

	eng := NewEngine("", t.TempDir())
	plan, err := eng.Plan(onlyBashSpec(), platform.Info{OS: platform.OSDebian}, Options{})
	var missing *MissingSourceTreeError
	if !errors.As(err, &missing) {
		t.Fatalf("Plan() = %+v, %v; want MissingSourceTreeError", plan, err)
	}
	if missing.Root != "" {
		t.Errorf("MissingSourceTreeError.Root = %q, want empty", missing.Root)
	}
}

// TestPlan_SymlinkedRepoRoot: the engine is given an alias (symlink) of the
// real repo, as with macOS /var -> /private/var. Links spelled through either
// path are recognised, and containment is judged against the resolved root.
func TestPlan_SymlinkedRepoRoot(t *testing.T) {
	keymap := LinkSpec{Src: "zed/keymap.json", Dest: ".config/zed/keymap.json", Facet: FacetAll}
	tests := []struct {
		name       string
		spec       LinkSpec
		setup      func(t *testing.T, realRepo, aliasRepo, home string)
		wantInRepo bool
	}{
		{
			name: "link-via-alias-already-correct",
			spec: onlyBashSpec()[0],
			setup: func(t *testing.T, _, aliasRepo, home string) {
				mkSymlink(t, filepath.Join(aliasRepo, "bash/bashrc"), filepath.Join(home, ".bashrc"))
			},
		},
		{
			name: "parent-to-real-repo-source-skipped",
			spec: keymap,
			setup: func(t *testing.T, realRepo, _, home string) {
				mkSymlink(t, filepath.Join(realRepo, "zed"), filepath.Join(home, ".config/zed"))
			},
		},
		{
			name: "parent-to-other-real-repo-dir-refused",
			spec: keymap,
			setup: func(t *testing.T, realRepo, _, home string) {
				writeFile(t, filepath.Join(realRepo, "other/keymap.json"), []byte("other"))
				mkSymlink(t, filepath.Join(realRepo, "other"), filepath.Join(home, ".config/zed"))
			},
			wantInRepo: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			realRepo := filepath.Join(base, "real")
			aliasRepo := filepath.Join(base, "alias")
			home := t.TempDir()
			writeFile(t, filepath.Join(realRepo, tt.spec.Src), []byte("repo bytes"))
			mkSymlink(t, realRepo, aliasRepo)
			tt.setup(t, realRepo, aliasRepo, home)

			eng := NewEngine(aliasRepo, home)
			plan, err := eng.Plan([]LinkSpec{tt.spec}, platform.Info{OS: platform.OSDebian}, Options{Backup: true})
			if tt.wantInRepo {
				var inRepo *DestInRepoError
				if !errors.As(err, &inRepo) {
					t.Fatalf("Plan() error = %v, want DestInRepoError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Plan() error = %v, want nil", err)
			}
			if len(plan.Actions) != 1 || plan.Actions[0].Kind != ActionSkip || plan.Actions[0].Reason != "already correct" {
				t.Fatalf("Plan().Actions = %+v, want one ActionSkip \"already correct\"", plan.Actions)
			}
		})
	}
}

// TestPlan_SymlinkedAncestorIntoRepo_MissingParent_Refused: with
// ~/.config -> $REPO/cfg and ~/.config/zed absent, the dest still resolves
// inside the repo. Plan must refuse before anything (including the missing
// parent dir) is created in the repo.
func TestPlan_SymlinkedAncestorIntoRepo_MissingParent_Refused(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "zed/keymap.json"), []byte("repo keymap"))
	cfg := filepath.Join(repoRoot, "cfg")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", cfg, err)
	}
	mkSymlink(t, cfg, filepath.Join(home, ".config"))

	eng := NewEngine(repoRoot, home)
	specs := []LinkSpec{{Src: "zed/keymap.json", Dest: ".config/zed/keymap.json", Facet: FacetAll}}
	plan, err := eng.Plan(specs, platform.Info{OS: platform.OSDebian}, Options{Backup: true})
	var inRepo *DestInRepoError
	if !errors.As(err, &inRepo) {
		t.Fatalf("Plan() = %+v, %v; want DestInRepoError", plan, err)
	}
	if want := filepath.Join(cfg, "zed/keymap.json"); inRepo.Resolved != want {
		t.Errorf("DestInRepoError.Resolved = %q, want %q", inRepo.Resolved, want)
	}
	entries, err := os.ReadDir(cfg)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", cfg, err)
	}
	if len(entries) != 0 {
		t.Errorf("repo cfg/ gained entries %v, want none", entries)
	}
}

// TestPlan_DestViaSymlinkedParentIsSource: when a dest's parent dir is a
// symlink into the repo, the dest already IS the source. Plan must emit only
// a Skip, never a backup/remove/link/copy that would clobber the repo file.
func TestPlan_DestViaSymlinkedParentIsSource(t *testing.T) {
	tests := []struct {
		name string
		spec LinkSpec
	}{
		{name: "link", spec: LinkSpec{Src: "zed/keymap.json", Dest: ".config/zed/keymap.json", Facet: FacetAll}},
		{name: "copy", spec: LinkSpec{Src: "zed/settings.json", Dest: ".config/zed/settings.json", Copy: true, Facet: FacetAll}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			home := t.TempDir()
			writeFile(t, filepath.Join(repoRoot, tt.spec.Src), []byte("repo bytes"))
			mkSymlink(t, filepath.Join(repoRoot, "zed"), filepath.Join(home, ".config/zed"))

			eng := NewEngine(repoRoot, home)
			plan, err := eng.Plan([]LinkSpec{tt.spec}, platform.Info{OS: platform.OSDebian}, Options{Backup: true})
			if err != nil {
				t.Fatalf("Plan() error = %v, want nil", err)
			}
			if len(plan.Actions) != 1 || plan.Actions[0].Kind != ActionSkip {
				t.Fatalf("Plan().Actions = %+v, want exactly one ActionSkip", plan.Actions)
			}
			if plan.Actions[0].Reason != "already correct" {
				t.Errorf("reason = %q, want %q", plan.Actions[0].Reason, "already correct")
			}
		})
	}
}

// TestPlan_DestResolvesInsideRepo_Refused: a dest whose symlinked parent lands
// inside the repo at a path that is not its own source would modify a tracked
// repo file, so Plan refuses with a DestInRepoError.
func TestPlan_DestResolvesInsideRepo_Refused(t *testing.T) {
	tests := []struct {
		name string
		spec LinkSpec
	}{
		{name: "link", spec: LinkSpec{Src: "zed/keymap.json", Dest: ".config/zed/keymap.json", Facet: FacetAll}},
		{name: "copy", spec: LinkSpec{Src: "zed/settings.json", Dest: ".config/zed/settings.json", Copy: true, Facet: FacetAll}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			home := t.TempDir()
			writeFile(t, filepath.Join(repoRoot, tt.spec.Src), []byte("repo bytes"))
			other := filepath.Join(repoRoot, "other")
			writeFile(t, filepath.Join(other, filepath.Base(tt.spec.Dest)), []byte("other repo file"))
			mkSymlink(t, other, filepath.Join(home, ".config/zed"))

			eng := NewEngine(repoRoot, home)
			plan, err := eng.Plan([]LinkSpec{tt.spec}, platform.Info{OS: platform.OSDebian}, Options{Backup: true})
			var inRepo *DestInRepoError
			if !errors.As(err, &inRepo) {
				t.Fatalf("Plan() error = %v, want DestInRepoError", err)
			}
			if plan != nil {
				t.Errorf("Plan() = %+v, want nil on dest inside repo", plan)
			}
			if inRepo.Dest != tt.spec.Dest {
				t.Errorf("DestInRepoError.Dest = %q, want %q", inRepo.Dest, tt.spec.Dest)
			}
		})
	}
}

// TestPlan_EquivalentTarget_AlreadyCorrect: a symlink whose target is spelled
// differently but resolves to the source is already correct, not a repoint.
func TestPlan_EquivalentTarget_AlreadyCorrect(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "bash/bashrc"), []byte("rc"))
	mkSymlink(t, repoRoot+"/bash/../bash/bashrc", filepath.Join(home, ".bashrc"))

	eng := NewEngine(repoRoot, home)
	plan, err := eng.Plan(onlyBashSpec(), platform.Info{OS: platform.OSDebian}, Options{})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}
	if _, ok := planActionFor(plan, ".bashrc", ActionSkip); !ok {
		t.Errorf("Plan().Actions = %+v, want ActionSkip for equivalent target", plan.Actions)
	}
}

// TestPlan_CopyEntry covers classification of the Zed copy exception.
func TestPlan_CopyEntry(t *testing.T) {
	const dest = ".config/zed/settings.json"
	srcBytes := []byte(`{"theme":"repo"}`)
	tests := []struct {
		name      string
		setup     func(t *testing.T, repoRoot, home string)
		backup    bool
		wantKinds []ActionKind
		wantCopy  string
	}{
		{
			name:      "missing",
			setup:     func(_ *testing.T, _, _ string) {},
			backup:    true,
			wantKinds: []ActionKind{ActionCopy},
			wantCopy:  "create",
		},
		{
			name: "identical",
			setup: func(t *testing.T, _, home string) {
				writeFile(t, filepath.Join(home, dest), srcBytes)
			},
			backup:    true,
			wantKinds: []ActionKind{ActionSkip},
		},
		{
			name: "differing-backup-on",
			setup: func(t *testing.T, _, home string) {
				writeFile(t, filepath.Join(home, dest), []byte(`{"theme":"user"}`))
			},
			backup:    true,
			wantKinds: []ActionKind{ActionBackup, ActionCopy},
			wantCopy:  "overwrite",
		},
		{
			name: "differing-backup-off",
			setup: func(t *testing.T, _, home string) {
				writeFile(t, filepath.Join(home, dest), []byte(`{"theme":"user"}`))
			},
			wantKinds: []ActionKind{ActionCopy},
			wantCopy:  "overwrite",
		},
		{
			name: "legacy-symlink",
			setup: func(t *testing.T, repoRoot, home string) {
				mkSymlink(t, filepath.Join(repoRoot, "zed/settings.json"), filepath.Join(home, dest))
			},
			backup:    true,
			wantKinds: []ActionKind{ActionCopy},
			wantCopy:  "replace legacy symlink",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			home := t.TempDir()
			writeFile(t, filepath.Join(repoRoot, "zed/settings.json"), srcBytes)
			tt.setup(t, repoRoot, home)

			eng := NewEngine(repoRoot, home)
			specs := []LinkSpec{{Src: "zed/settings.json", Dest: dest, Copy: true, Facet: FacetAll}}
			plan, err := eng.Plan(specs, platform.Info{OS: platform.OSDebian}, Options{Backup: tt.backup})
			if err != nil {
				t.Fatalf("Plan() error = %v, want nil", err)
			}
			if len(plan.Actions) != len(tt.wantKinds) {
				t.Fatalf("Plan().Actions = %+v, want kinds %v", plan.Actions, tt.wantKinds)
			}
			for i, k := range tt.wantKinds {
				if plan.Actions[i].Kind != k {
					t.Errorf("Actions[%d].Kind = %d, want %d", i, plan.Actions[i].Kind, k)
				}
			}
			if tt.wantCopy != "" {
				act, ok := planActionFor(plan, dest, ActionCopy)
				if !ok {
					t.Fatal("expected ActionCopy")
				}
				if act.Reason != tt.wantCopy {
					t.Errorf("copy reason = %q, want %q", act.Reason, tt.wantCopy)
				}
			}
		})
	}
}

// onlyBashSpec returns a single-entry table for focused Plan tests.
func onlyBashSpec() []LinkSpec {
	return []LinkSpec{{Src: "bash/bashrc", Dest: ".bashrc", Facet: FacetAll}}
}
