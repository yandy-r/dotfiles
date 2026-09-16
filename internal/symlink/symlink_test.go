package symlink

import (
	"errors"
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
			name: "macos", os: platform.OSMacOS, wantCreated: 30,
			mustHave:    []string{".bashrc", ".config/aerospace", ".config/herdr/config.toml", ".config/zed/settings.json"},
			mustNotHave: []string{".config/hypr", ".config/systemd/user/tmux.service"},
		},
		{
			name: "arch-no-desktop", os: platform.OSArch, wantCreated: 30,
			mustHave:    []string{".config/herdr/config.toml", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace", ".config/hypr"},
		},
		{
			name: "arch-desktop", os: platform.OSArch, ld: true, wantCreated: 41,
			mustHave: []string{".config/hypr", ".local/share/applications/wifi-tui.desktop",
				".local/share/applications/arch-update.desktop", ".config/herdr/config.toml", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace"},
		},
		{
			name: "debian-no-desktop", os: platform.OSDebian, wantCreated: 30,
			mustHave:    []string{".bashrc", ".config/herdr/config.toml", ".config/nvim", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace", ".config/hypr"},
		},
		{
			// Still 39 after ADR-12, but for a different reason: tmux.service is
			// gained (+1) exactly as the Arch-only arch-update.desktop is lost
			// (-1). Spelling that out so the unchanged count does not read as a
			// false pass.
			name: "debian-desktop", os: platform.OSDebian, ld: true, wantCreated: 40,
			mustHave:    []string{".config/herdr/config.toml", ".config/hypr", ".config/waybar", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace", ".local/share/applications/arch-update.desktop"},
		},
		{
			name: "generic-linux", os: platform.OSLinux, ld: true, wantCreated: 29,
			mustHave:    []string{".config/herdr/config.toml"},
			mustNotHave: []string{".config/hypr", ".config/aerospace"},
		},
		{
			name: "redhat-no-desktop", os: platform.OSRedHat, wantCreated: 30,
			mustHave:    []string{".bashrc", ".config/herdr/config.toml", ".config/nvim", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace", ".config/hypr", ".local/share/applications/arch-update.desktop"},
		},
		{
			name: "suse-no-desktop", os: platform.OSSUSE, wantCreated: 30,
			mustHave:    []string{".config/herdr/config.toml", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace", ".config/hypr"},
		},
		{
			name: "wsl", os: platform.OSWSL, wantCreated: 30,
			mustHave:    []string{".config/herdr/config.toml", ".config/systemd/user/tmux.service"},
			mustNotHave: []string{".config/aerospace", ".config/hypr"},
		},
		{
			name: "windows-excludes-herdr", os: platform.OSWindows, wantCreated: 28,
			mustNotHave: []string{".config/herdr/config.toml", ".config/aerospace", ".config/hypr"},
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

func TestPlan_BackupAction(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	buildFullSourceTree(t, repoRoot)
	// Real files at three dests: a backup-eligible file entry, a directory
	// entry made backup-eligible by ADR-11, and a still-non-eligible entry.
	writeFile(t, filepath.Join(home, ".bashrc"), []byte("old bashrc"))
	writeFile(t, filepath.Join(home, ".config/nvim"), []byte("old nvim"))
	writeFile(t, filepath.Join(home, ".ansible.cfg"), []byte("old ansible cfg"))

	eng := NewEngine(repoRoot, home)
	plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{Backup: true})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}

	if _, ok := planActionFor(plan, ".bashrc", ActionBackup); !ok {
		t.Error("expected ActionBackup for backup-eligible .bashrc real file")
	}
	if _, ok := planActionFor(plan, ".config/nvim", ActionBackup); !ok {
		t.Error("expected ActionBackup for backup-eligible .config/nvim (ADR-11)")
	}
	if _, ok := planActionFor(plan, ".ansible.cfg", ActionBackup); ok {
		t.Error("unexpected ActionBackup for non-eligible .ansible.cfg")
	}
	if _, ok := planActionFor(plan, ".config/nvim", ActionRemove); !ok {
		t.Error("expected ActionRemove for real file at .config/nvim")
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

// onlyBashSpec returns a single-entry table for focused Plan tests.
func onlyBashSpec() []LinkSpec {
	return []LinkSpec{{Src: "bash/bashrc", Dest: ".bashrc", Facet: FacetAll, BackupEligible: true}}
}
