package symlink

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yandy-r/dotfiles/internal/platform"
)

// planApply plans and applies specs against repoRoot/home, failing on any
// error, and returns the resulting Report.
func planApply(t *testing.T, repoRoot, home string, specs []LinkSpec, info platform.Info, opts Options) *Report {
	t.Helper()
	eng := NewEngine(repoRoot, home)
	plan, err := eng.Plan(specs, info, opts)
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}
	report, err := eng.Apply(plan, opts)
	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}
	return report
}

func TestApply_DryRun_NoMutations(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	buildFullSourceTree(t, repoRoot)
	writeFile(t, filepath.Join(home, ".bashrc"), []byte("user bashrc"))
	writeFile(t, filepath.Join(home, ".gitconfig"), []byte("user gitconfig"))

	before, err := os.ReadDir(home)
	if err != nil {
		t.Fatalf("ReadDir(home) error = %v", err)
	}

	eng := NewEngine(repoRoot, home)
	opts := Options{DryRun: true}
	plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, opts)
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}
	report, err := eng.Apply(plan, opts)
	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}

	if len(report.Applied) != 0 {
		t.Errorf("Report.Applied = %d actions, want 0 under dry-run", len(report.Applied))
	}
	if _, err := os.Stat(plan.BackupDir); !os.IsNotExist(err) {
		t.Errorf("backup dir exists under dry-run (err=%v), want absent", err)
	}

	after, err := os.ReadDir(home)
	if err != nil {
		t.Fatalf("ReadDir(home) error = %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("home entry count = %d, want %d (dry-run must not mutate)", len(after), len(before))
	}
	// .bashrc is still the original real file, not a symlink.
	if isSymlink(t, filepath.Join(home, ".bashrc")) {
		t.Error(".bashrc became a symlink under dry-run, want unchanged real file")
	}
}

func TestApply_Backup(t *testing.T) {
	repoRoot := t.TempDir()
	buildFullSourceTree(t, repoRoot)

	t.Run("real-file-backed-up-then-linked", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, ".bashrc"), []byte("original bashrc"))

		eng := NewEngine(repoRoot, home)
		opts := Options{Backup: true}
		plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, opts)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if _, err := eng.Apply(plan, opts); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}

		backed, err := os.ReadFile(filepath.Join(plan.BackupDir, ".bashrc"))
		if err != nil {
			t.Fatalf("ReadFile(backup/.bashrc) error = %v", err)
		}
		if !bytes.Equal(backed, []byte("original bashrc")) {
			t.Errorf("backup content = %q, want %q", backed, "original bashrc")
		}
		if !isSymlink(t, filepath.Join(home, ".bashrc")) {
			t.Error(".bashrc is not a symlink after apply")
		}
	})

	t.Run("herdr-config-backed-up-then-linked", func(t *testing.T) {
		home := t.TempDir()
		original := []byte("original herdr config")
		dest := filepath.Join(home, ".config/herdr/config.toml")
		writeFile(t, dest, original)

		eng := NewEngine(repoRoot, home)
		opts := Options{Backup: true}
		plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, opts)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if _, err := eng.Apply(plan, opts); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}

		backed, err := os.ReadFile(filepath.Join(plan.BackupDir, ".config/herdr/config.toml"))
		if err != nil {
			t.Fatalf("ReadFile(backup/.config/herdr/config.toml) error = %v", err)
		}
		if !bytes.Equal(backed, original) {
			t.Errorf("backup content = %q, want %q", backed, original)
		}
		if !isSymlink(t, dest) {
			t.Fatal(".config/herdr/config.toml is not a symlink after apply")
		}
		target, err := os.Readlink(dest)
		if err != nil {
			t.Fatalf("Readlink(.config/herdr/config.toml) error = %v", err)
		}
		wantTarget := filepath.Join(repoRoot, "herdr/config.toml")
		if target != wantTarget {
			t.Errorf("Readlink(.config/herdr/config.toml) = %q, want %q", target, wantTarget)
		}
	})

	// symlink-not-backed-up also pins the lazy backup dir: with Backup on but
	// no real file to replace, Apply must not leave an empty backup dir behind.
	t.Run("symlink-not-backed-up-no-backup-dir", func(t *testing.T) {
		home := t.TempDir()
		mkSymlink(t, filepath.Join(home, "other"), filepath.Join(home, ".zshrc"))

		eng := NewEngine(repoRoot, home)
		opts := Options{Backup: true}
		plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, opts)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		report, err := eng.Apply(plan, opts)
		if err != nil {
			t.Fatalf("Apply() error = %v", err)
		}

		if _, err := os.Stat(plan.BackupDir); !os.IsNotExist(err) {
			t.Errorf("backup dir exists with nothing to back up (err=%v), want absent", err)
		}
		if report.BackupDir != "" {
			t.Errorf("Report.BackupDir = %q, want empty when nothing was backed up", report.BackupDir)
		}
	})

	// directory-backed-up-then-linked is ADR-11 / design §7 test 4: the
	// only test guarding data-destroying behaviour. ActionRemove is recursive
	// for dirs, so the backup must be ordered strictly before it — and the first
	// host to exercise the directory-copy path is a populated daily driver.
	t.Run("directory-backed-up-then-linked", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, ".config/nvim/lua/user/init.lua"), []byte("sentinel"))

		eng := NewEngine(repoRoot, home)
		opts := Options{Backup: true}
		plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, opts)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}

		bi, ok := planIndexOf(plan, ".config/nvim", ActionBackup)
		if !ok {
			t.Fatal("expected ActionBackup for populated .config/nvim directory")
		}
		ri, ok := planIndexOf(plan, ".config/nvim", ActionRemove)
		if !ok {
			t.Fatal("expected ActionRemove for populated .config/nvim directory")
		}
		if bi >= ri {
			t.Fatalf("ActionBackup index = %d, ActionRemove index = %d; backup must precede the recursive remove", bi, ri)
		}

		if _, err := eng.Apply(plan, opts); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}

		backed, err := os.ReadFile(filepath.Join(plan.BackupDir, ".config/nvim/lua/user/init.lua"))
		if err != nil {
			t.Fatalf("ReadFile(backup/.config/nvim/lua/user/init.lua) error = %v", err)
		}
		if !bytes.Equal(backed, []byte("sentinel")) {
			t.Errorf("backup content = %q, want %q", backed, "sentinel")
		}
		if !isSymlink(t, filepath.Join(home, ".config/nvim")) {
			t.Error(".config/nvim is not a symlink after apply")
		}
	})

	// ssh-config-backed-up-then-linked: every real file is backed up, not only
	// the shell dotfiles — .ssh/config was once removed without a copy.
	t.Run("ssh-config-backed-up-then-linked", func(t *testing.T) {
		home := t.TempDir()
		original := []byte("Host personal\n")
		writeFile(t, filepath.Join(home, ".ssh/config"), original)

		eng := NewEngine(repoRoot, home)
		opts := Options{Backup: true}
		plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, opts)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		report, err := eng.Apply(plan, opts)
		if err != nil {
			t.Fatalf("Apply() error = %v", err)
		}

		if report.BackupDir != plan.BackupDir {
			t.Errorf("Report.BackupDir = %q, want %q", report.BackupDir, plan.BackupDir)
		}
		backed, err := os.ReadFile(filepath.Join(plan.BackupDir, ".ssh/config"))
		if err != nil {
			t.Fatalf("ReadFile(backup/.ssh/config) error = %v", err)
		}
		if !bytes.Equal(backed, original) {
			t.Errorf("backup content = %q, want %q", backed, original)
		}
		if !isSymlink(t, filepath.Join(home, ".ssh/config")) {
			t.Error(".ssh/config is not a symlink after apply")
		}
	})

	t.Run("backup-off-removes-real-file-no-backup-dir", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, ".bashrc"), []byte("original bashrc"))

		eng := NewEngine(repoRoot, home)
		opts := Options{Backup: false}
		plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, opts)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if _, err := eng.Apply(plan, opts); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}

		if _, err := os.Stat(plan.BackupDir); !os.IsNotExist(err) {
			t.Errorf("backup dir exists with Backup=false (err=%v), want absent", err)
		}
		if !isSymlink(t, filepath.Join(home, ".bashrc")) {
			t.Error(".bashrc is not a symlink after apply")
		}
	})

	// existing-backup-dir-not-overwritten: a same-second rerun finds
	// plan.BackupDir already present; Apply must pick a fresh sibling instead
	// of writing into (and truncating copies inside) the earlier run's dir.
	t.Run("existing-backup-dir-not-overwritten", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, ".bashrc"), []byte("second run bashrc"))

		eng := NewEngine(repoRoot, home)
		opts := Options{Backup: true}
		plan, err := eng.Plan(DefaultTable(), platform.Info{OS: platform.OSDebian}, opts)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		first := plan.BackupDir
		writeFile(t, filepath.Join(first, ".bashrc"), []byte("first run bashrc"))

		report, err := eng.Apply(plan, opts)
		if err != nil {
			t.Fatalf("Apply() error = %v", err)
		}

		if want := first + "_1"; report.BackupDir != want || plan.BackupDir != want {
			t.Errorf("BackupDir report=%q plan=%q, want %q", report.BackupDir, plan.BackupDir, want)
		}
		kept, err := os.ReadFile(filepath.Join(first, ".bashrc"))
		if err != nil || string(kept) != "first run bashrc" {
			t.Errorf("earlier backup = %q (err=%v), want it untouched", kept, err)
		}
		got, err := os.ReadFile(filepath.Join(report.BackupDir, ".bashrc"))
		if err != nil || string(got) != "second run bashrc" {
			t.Errorf("new backup = %q (err=%v), want %q", got, err, "second run bashrc")
		}
	})
}

func TestApply_CorrectSymlink_Idempotent(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	src := filepath.Join(repoRoot, "bash/bashrc")
	writeFile(t, src, []byte("rc"))
	dest := filepath.Join(home, ".bashrc")
	mkSymlink(t, src, dest)

	// Snapshot the already-correct symlink's Lstat ModTime BEFORE re-applying,
	// so we can prove the idempotent re-run does not rewrite the link (a
	// recreated symlink would bump ModTime). Note: Apply always scaffolds the
	// required parent dirs, so a whole-$HOME entry count is NOT a valid
	// invariant here — the link's own ModTime plus Report.Applied==0 are the
	// precise proof of zero mutation.
	beforeLink, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("Lstat(.bashrc) before = %v", err)
	}

	eng := NewEngine(repoRoot, home)
	opts := Options{}
	plan, err := eng.Plan(onlyBashSpec(), platform.Info{OS: platform.OSDebian}, opts)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	report, err := eng.Apply(plan, opts)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// The only planned action is a skip; a skip is a no-op, so nothing is
	// applied and the link still resolves to src.
	if got := countKind(plan, ActionSkip); got != 1 {
		t.Errorf("ActionSkip count = %d, want 1", got)
	}
	if got := countKind(plan, ActionLink); got != 0 {
		t.Errorf("ActionLink count = %d, want 0 for idempotent re-run", got)
	}
	if len(report.Applied) != 0 {
		t.Errorf("Report.Applied = %d, want 0 (a skip performs no mutation)", len(report.Applied))
	}
	target, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("Readlink error = %v", err)
	}
	if target != src {
		t.Errorf("Readlink(.bashrc) = %q, want %q", target, src)
	}

	// The symlink was NOT rewritten: its Lstat ModTime is unchanged.
	afterLink, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("Lstat(.bashrc) after = %v", err)
	}
	if !afterLink.ModTime().Equal(beforeLink.ModTime()) {
		t.Errorf("symlink ModTime changed %v -> %v; want unchanged (skip must not rewrite the link)",
			beforeLink.ModTime(), afterLink.ModTime())
	}
}

func TestApply_CreatesRequiredDirs(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	buildFullSourceTree(t, repoRoot)

	planApply(t, repoRoot, home, DefaultTable(), platform.Info{OS: platform.OSArch}, Options{})

	for _, dir := range requiredDirs(home) {
		fi, err := os.Stat(dir)
		if err != nil {
			t.Errorf("required dir %q not created: %v", dir, err)
			continue
		}
		if !fi.IsDir() {
			t.Errorf("required path %q is not a directory", dir)
		}
	}
}

func TestApply_RepointDangling(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	src := filepath.Join(repoRoot, "bash/bashrc")
	writeFile(t, src, []byte("rc"))
	mkSymlink(t, filepath.Join(home, "gone"), filepath.Join(home, ".bashrc"))

	planApply(t, repoRoot, home, onlyBashSpec(), platform.Info{OS: platform.OSDebian}, Options{})

	target, err := os.Readlink(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("Readlink error = %v", err)
	}
	if target != src {
		t.Errorf("Readlink(.bashrc) = %q, want %q", target, src)
	}
}

func TestApply_ReplaceRealFile(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	src := filepath.Join(repoRoot, "bash/bashrc")
	writeFile(t, src, []byte("repo rc"))
	writeFile(t, filepath.Join(home, ".bashrc"), []byte("user rc"))

	planApply(t, repoRoot, home, onlyBashSpec(), platform.Info{OS: platform.OSDebian}, Options{})

	dest := filepath.Join(home, ".bashrc")
	if !isSymlink(t, dest) {
		t.Fatal(".bashrc is not a symlink after apply")
	}
	target, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("Readlink error = %v", err)
	}
	if target != src {
		t.Errorf("Readlink(.bashrc) = %q, want %q", target, src)
	}
}

func TestApply_Zed_CopyNotSymlink(t *testing.T) {
	repoRoot := t.TempDir()
	buildFullSourceTree(t, repoRoot)
	settingsSrc := filepath.Join(repoRoot, "zed/settings.json")

	t.Run("fresh-copy-and-keymap-symlink", func(t *testing.T) {
		home := t.TempDir()
		planApply(t, repoRoot, home, DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{})

		settingsDest := filepath.Join(home, ".config/zed/settings.json")
		if isSymlink(t, settingsDest) {
			t.Error("zed settings.json is a symlink, want a regular copied file")
		}
		got, err := os.ReadFile(settingsDest)
		if err != nil {
			t.Fatalf("ReadFile(settings.json) error = %v", err)
		}
		want, err := os.ReadFile(settingsSrc)
		if err != nil {
			t.Fatalf("ReadFile(source settings.json) error = %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("settings.json content = %q, want %q", got, want)
		}
		fi, err := os.Stat(settingsDest)
		if err != nil {
			t.Fatalf("Stat(settings.json) error = %v", err)
		}
		if fi.Mode().Perm() != 0o644 {
			t.Errorf("settings.json mode = %o, want 0644", fi.Mode().Perm())
		}
		// keymap.json is a normal symlink.
		if !isSymlink(t, filepath.Join(home, ".config/zed/keymap.json")) {
			t.Error("zed keymap.json is not a symlink, want a symlink")
		}
	})

	t.Run("legacy-symlink-migrated-to-copy", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".config/zed"), 0o755); err != nil {
			t.Fatalf("MkdirAll error = %v", err)
		}
		mkSymlink(t, settingsSrc, filepath.Join(home, ".config/zed/settings.json"))

		planApply(t, repoRoot, home, DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{})

		settingsDest := filepath.Join(home, ".config/zed/settings.json")
		if isSymlink(t, settingsDest) {
			t.Error("legacy zed settings symlink was not migrated to a regular file")
		}
	})
}

// TestApply_SymlinkedParentIntoRepo_RepoFileUntouched reproduces YAN-1328:
// with ~/.config/zed -> $REPO/zed, a full apply must leave the repo's keymap
// and settings as regular files with their original bytes, and Validate must
// agree with Plan that the dest is already correct (no BrokenLinkError).
func TestApply_SymlinkedParentIntoRepo_RepoFileUntouched(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	buildFullSourceTree(t, repoRoot)
	mkSymlink(t, filepath.Join(repoRoot, "zed"), filepath.Join(home, ".config/zed"))

	planApply(t, repoRoot, home, DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{Backup: true})

	for _, src := range []string{"zed/keymap.json", "zed/settings.json"} {
		p := filepath.Join(repoRoot, src)
		if isSymlink(t, p) {
			t.Errorf("repo file %s became a symlink", src)
			continue
		}
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", p, err)
		}
		if want := "content of " + src; string(got) != want {
			t.Errorf("repo file %s = %q, want %q", src, got, want)
		}
	}

	eng := NewEngine(repoRoot, home)
	results, err := eng.Validate(DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{})
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil (dest already is its source)", err)
	}
	for _, r := range results {
		if r.Dest == ".config/zed/keymap.json" && (!r.Valid || r.Broken) {
			t.Errorf("keymap.json result = %+v, want Valid and not Broken", r)
		}
	}
}

// TestApply_ParentSwappedAfterPlan_Refused exercises Apply's containment
// re-check: ~/.config/zed is a real (empty) dir at Plan time, then swapped for
// a symlink to a different repo dir before Apply. Apply must refuse with a
// DestInRepoError and leave the repo file byte-identical.
func TestApply_ParentSwappedAfterPlan_Refused(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "zed/keymap.json"), []byte("repo keymap"))
	otherKeymap := filepath.Join(repoRoot, "other/keymap.json")
	otherBytes := []byte("other repo keymap")
	writeFile(t, otherKeymap, otherBytes)
	zedDir := filepath.Join(home, ".config/zed")
	if err := os.MkdirAll(zedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", zedDir, err)
	}

	specs := []LinkSpec{{Src: "zed/keymap.json", Dest: ".config/zed/keymap.json", Facet: FacetAll}}
	opts := Options{Backup: true}
	eng := NewEngine(repoRoot, home)
	plan, err := eng.Plan(specs, platform.Info{OS: platform.OSDebian}, opts)
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}

	if err := os.Remove(zedDir); err != nil {
		t.Fatalf("Remove(%q) error = %v", zedDir, err)
	}
	mkSymlink(t, filepath.Join(repoRoot, "other"), zedDir)

	_, err = eng.Apply(plan, opts)
	var inRepo *DestInRepoError
	if !errors.As(err, &inRepo) {
		t.Fatalf("Apply() error = %v, want DestInRepoError", err)
	}
	if isSymlink(t, otherKeymap) {
		t.Fatal("repo file other/keymap.json became a symlink")
	}
	got, err := os.ReadFile(otherKeymap)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", otherKeymap, err)
	}
	if !bytes.Equal(got, otherBytes) {
		t.Errorf("other/keymap.json = %q, want %q", got, otherBytes)
	}
}

// TestApply_ZedCopy_BackupAndIdempotence covers YAN-1327/YAN-1369: a differing
// user settings file is backed up before overwrite, and a converged rerun
// reports zero applied actions.
func TestApply_ZedCopy_BackupAndIdempotence(t *testing.T) {
	const dest = ".config/zed/settings.json"
	specs := []LinkSpec{{Src: "zed/settings.json", Dest: dest, Copy: true, Facet: FacetAll}}
	info := platform.Info{OS: platform.OSDebian}
	srcBytes := []byte(`{"theme":"repo"}`)
	userBytes := []byte(`{"theme":"user"}`)

	tests := []struct {
		name       string
		existing   []byte
		backup     bool
		wantBackup bool
	}{
		{name: "missing", backup: true},
		{name: "identical", existing: srcBytes, backup: true},
		{name: "differing-backup-on", existing: userBytes, backup: true, wantBackup: true},
		{name: "differing-backup-off", existing: userBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			home := t.TempDir()
			writeFile(t, filepath.Join(repoRoot, "zed/settings.json"), srcBytes)
			if tt.existing != nil {
				writeFile(t, filepath.Join(home, dest), tt.existing)
			}
			opts := Options{Backup: tt.backup}

			report := planApply(t, repoRoot, home, specs, info, opts)
			got, err := os.ReadFile(filepath.Join(home, dest))
			if err != nil {
				t.Fatalf("ReadFile(dest) error = %v", err)
			}
			if !bytes.Equal(got, srcBytes) {
				t.Errorf("dest = %q, want source bytes %q", got, srcBytes)
			}

			if tt.wantBackup {
				if report.BackupDir == "" {
					t.Fatal("report.BackupDir empty, want a backup of the differing file")
				}
				saved, err := os.ReadFile(filepath.Join(report.BackupDir, dest))
				if err != nil {
					t.Fatalf("ReadFile(backup) error = %v", err)
				}
				if !bytes.Equal(saved, tt.existing) {
					t.Errorf("backup = %q, want old bytes %q", saved, tt.existing)
				}
			} else if report.BackupDir != "" {
				t.Errorf("report.BackupDir = %q, want none", report.BackupDir)
			}

			second := planApply(t, repoRoot, home, specs, info, opts)
			if len(second.Applied) != 0 {
				t.Errorf("second run Applied = %+v, want none (idempotent)", second.Applied)
			}
		})
	}
}

// TestApply_RelativeRepoRoot_AbsoluteLinks reproduces YAN-1329: a relative
// repo root must still produce absolute link targets that resolve from $HOME.
func TestApply_RelativeRepoRoot_AbsoluteLinks(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(base, "repo/bash/bashrc"), []byte("rc"))
	t.Chdir(base)

	planApply(t, "repo", home, onlyBashSpec(), platform.Info{OS: platform.OSDebian}, Options{})

	dest := filepath.Join(home, ".bashrc")
	target, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("Readlink(%q) error = %v", dest, err)
	}
	if !filepath.IsAbs(target) {
		t.Errorf("link target = %q, want absolute", target)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("Stat(%q) error = %v, want link to resolve", dest, err)
	}
}

func TestApply_PermissionDenied_Graceful(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission checks do not apply")
	}
	repoRoot := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "bash/bashrc"), []byte("rc"))

	// A read-only $HOME makes required-dir/parent creation fail; the engine
	// must surface a wrapped error rather than panic.
	if err := os.Chmod(home, 0o500); err != nil {
		t.Fatalf("Chmod(home) error = %v", err)
	}
	defer func() { _ = os.Chmod(home, 0o755) }()

	eng := NewEngine(repoRoot, home)
	opts := Options{}
	plan, err := eng.Plan(onlyBashSpec(), platform.Info{OS: platform.OSDebian}, opts)
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}
	if _, err := eng.Apply(plan, opts); err == nil {
		t.Fatal("Apply() error = nil, want a permission error on read-only $HOME")
	}
}

func TestApply_EndToEnd(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	buildFullSourceTree(t, repoRoot)

	report := planApply(t, repoRoot, home, DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{})
	if len(report.Applied) == 0 {
		t.Fatal("Report.Applied is empty after a real apply")
	}

	// A file link and a directory link both resolve to their sources.
	for _, dest := range []string{".bashrc", ".config/nvim"} {
		full := filepath.Join(home, dest)
		if !isSymlink(t, full) {
			t.Errorf("%q is not a symlink", dest)
		}
	}
	target, err := os.Readlink(filepath.Join(home, ".config/nvim"))
	if err != nil {
		t.Fatalf("Readlink(.config/nvim) error = %v", err)
	}
	if target != filepath.Join(repoRoot, "nvim") {
		t.Errorf("Readlink(.config/nvim) = %q, want %q", target, filepath.Join(repoRoot, "nvim"))
	}
}
