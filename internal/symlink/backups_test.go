package symlink

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yandy-r/dotfiles/internal/platform"
)

// testSpecs is a small table with one file dest and one dir dest.
var testSpecs = []LinkSpec{
	{Src: "bash/bashrc", Dest: ".bashrc", Facet: FacetAll},
	{Src: "nvim", Dest: ".config/nvim", Facet: FacetAll, IsDir: true},
}

// buildTestSources materializes testSpecs' sources under repoRoot.
func buildTestSources(t *testing.T, repoRoot string) {
	t.Helper()
	writeFile(t, filepath.Join(repoRoot, "bash/bashrc"), []byte("repo bashrc"))
	writeFile(t, filepath.Join(repoRoot, "nvim/init.lua"), []byte("repo init"))
}

// mustReadManifest reads dir's manifest, failing when absent or invalid.
func mustReadManifest(t *testing.T, dir string) *BackupManifest {
	t.Helper()
	m, err := readManifest(dir)
	if err != nil || m == nil {
		t.Fatalf("readManifest(%q) = %v, %v; want manifest", dir, m, err)
	}
	return m
}

// entryKinds maps manifest entries to dest -> kind.
func entryKinds(m *BackupManifest) map[string]string {
	out := make(map[string]string, len(m.Entries))
	for _, e := range m.Entries {
		out[e.Dest] = e.Kind
	}
	return out
}

// makeBackup creates <home>/.config/dotfiles_backup_<id> holding files (paths
// relative to the dir) and, when entries is non-nil, a manifest.
func makeBackup(t *testing.T, home, id string, created time.Time, files map[string]string, entries []BackupEntry) string {
	t.Helper()
	dir := filepath.Join(BackupRoot(home), BackupDirPrefix+id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	for rel, data := range files {
		writeFile(t, filepath.Join(dir, rel), []byte(data))
	}
	if entries != nil {
		m := &BackupManifest{Version: 1, CreatedAt: created, Entries: entries}
		if err := writeManifest(dir, m); err != nil {
			t.Fatalf("writeManifest error = %v", err)
		}
	}
	return dir
}

// snapshotTree records every path under dir with its mode and content (or
// link target), for asserting a tree is untouched.
func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		val := info.Mode().String()
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			val += "->" + l
		case info.Mode().IsRegular():
			b, _ := os.ReadFile(p)
			val += ":" + string(b)
		}
		out[p] = val
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%q) error = %v", dir, err)
	}
	return out
}

func readString(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", p, err)
	}
	return string(b)
}

func TestApply_WritesManifest(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, home string)
		wantKinds map[string]string // nil => no backup dir expected
	}{
		{
			name: "file-and-dir",
			setup: func(t *testing.T, home string) {
				writeFile(t, filepath.Join(home, ".bashrc"), []byte("user"))
				writeFile(t, filepath.Join(home, ".config/nvim/init.lua"), []byte("user init"))
			},
			wantKinds: map[string]string{".bashrc": BackupKindFile, ".config/nvim": BackupKindDir},
		},
		{
			name: "file-only",
			setup: func(t *testing.T, home string) {
				writeFile(t, filepath.Join(home, ".bashrc"), []byte("user"))
			},
			wantKinds: map[string]string{".bashrc": BackupKindFile},
		},
		{
			name:  "nothing-to-back-up",
			setup: func(*testing.T, string) {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoRoot, home := t.TempDir(), t.TempDir()
			buildTestSources(t, repoRoot)
			tt.setup(t, home)
			before := time.Now().UTC().Add(-time.Second)
			report := planApply(t, repoRoot, home, testSpecs, platform.Info{OS: platform.OSDebian}, Options{Backup: true})
			if tt.wantKinds == nil {
				if report.BackupDir != "" {
					t.Fatalf("BackupDir = %q, want empty", report.BackupDir)
				}
				return
			}
			m := mustReadManifest(t, report.BackupDir)
			if m.Version != 1 {
				t.Errorf("Version = %d, want 1", m.Version)
			}
			if m.CreatedAt.Before(before) {
				t.Errorf("CreatedAt = %v, want >= %v", m.CreatedAt, before)
			}
			got := entryKinds(m)
			if len(got) != len(tt.wantKinds) {
				t.Errorf("entries = %v, want %v", got, tt.wantKinds)
			}
			for dest, kind := range tt.wantKinds {
				if got[dest] != kind {
					t.Errorf("entry %q kind = %q, want %q", dest, got[dest], kind)
				}
			}
			for _, e := range m.Entries {
				if e.BackedUpAt.Before(before) {
					t.Errorf("entry %q BackedUpAt = %v, want recent", e.Dest, e.BackedUpAt)
				}
			}
			matches, _ := filepath.Glob(filepath.Join(report.BackupDir, BackupManifestName+".*"))
			if len(matches) != 0 {
				t.Errorf("leftover temp manifests: %v", matches)
			}
		})
	}
}

func TestApply_WritesManifestOnPartialFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission checks do not apply")
	}
	repoRoot, home := t.TempDir(), t.TempDir()
	buildTestSources(t, repoRoot)
	writeFile(t, filepath.Join(home, ".bashrc"), []byte("user"))
	// An unreadable file inside the dir dest makes its backup fail after
	// .bashrc (planned first) was copied.
	secret := filepath.Join(home, ".config/nvim/secret")
	writeFile(t, secret, []byte("x"))
	if err := os.Chmod(secret, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o644) })

	eng := NewEngine(repoRoot, home)
	opts := Options{Backup: true}
	plan, err := eng.Plan(testSpecs, platform.Info{OS: platform.OSDebian}, opts)
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Apply(plan, opts)
	if err == nil {
		t.Fatal("Apply() error = nil, want backup failure")
	}
	got := entryKinds(mustReadManifest(t, report.BackupDir))
	if len(got) != 1 || got[".bashrc"] != BackupKindFile {
		t.Errorf("entries = %v, want only .bashrc", got)
	}
}

func TestApply_RemovesEmptyBackupDirOnFirstCopyFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission checks do not apply")
	}
	repoRoot, home := t.TempDir(), t.TempDir()
	buildTestSources(t, repoRoot)
	// .bashrc is planned (and backed up) first; making it unreadable fails
	// the very first copy, so the fresh backup dir ends up empty.
	bashrc := filepath.Join(home, ".bashrc")
	writeFile(t, bashrc, []byte("user"))
	if err := os.Chmod(bashrc, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bashrc, 0o644) })

	eng := NewEngine(repoRoot, home)
	opts := Options{Backup: true}
	plan, err := eng.Plan(testSpecs, platform.Info{OS: platform.OSDebian}, opts)
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Apply(plan, opts)
	if err == nil {
		t.Fatal("Apply() error = nil, want backup failure")
	}
	if report.BackupDir != "" {
		t.Errorf("report.BackupDir = %q, want empty", report.BackupDir)
	}
	if _, serr := os.Lstat(plan.BackupDir); !os.IsNotExist(serr) {
		t.Errorf("empty backup dir %q left behind (stat err = %v)", plan.BackupDir, serr)
	}
	if sets, lerr := ListBackups(home); lerr != nil || len(sets) != 0 {
		t.Errorf("ListBackups = %d sets, %v; want none", len(sets), lerr)
	}
}

func TestApply_ReuseBackupDir(t *testing.T) {
	repoRoot := t.TempDir()
	buildTestSources(t, repoRoot)
	info := platform.Info{OS: platform.OSDebian}
	bashSpec := testSpecs[:1]
	nvimSpec := testSpecs[1:]

	t.Run("merges-entries", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, ".bashrc"), []byte("user"))
		writeFile(t, filepath.Join(home, ".config/nvim/init.lua"), []byte("user init"))
		first := planApply(t, repoRoot, home, bashSpec, info, Options{Backup: true})
		second := planApply(t, repoRoot, home, nvimSpec, info, Options{Backup: true, BackupDir: first.BackupDir})
		if second.BackupDir != first.BackupDir {
			t.Fatalf("second BackupDir = %q, want reused %q", second.BackupDir, first.BackupDir)
		}
		got := entryKinds(mustReadManifest(t, first.BackupDir))
		want := map[string]string{".bashrc": BackupKindFile, ".config/nvim": BackupKindDir}
		if len(got) != 2 || got[".bashrc"] != want[".bashrc"] || got[".config/nvim"] != want[".config/nvim"] {
			t.Errorf("entries = %v, want %v", got, want)
		}
		sets, err := ListBackups(home)
		if err != nil || len(sets) != 1 {
			t.Errorf("ListBackups = %d sets, %v; want 1 dir", len(sets), err)
		}
	})

	t.Run("rejects-duplicate-dest", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, ".bashrc"), []byte("user"))
		first := planApply(t, repoRoot, home, bashSpec, info, Options{Backup: true})
		// Make .bashrc real again so a second backup is planned.
		if err := os.Remove(filepath.Join(home, ".bashrc")); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(home, ".bashrc"), []byte("second"))
		eng := NewEngine(repoRoot, home)
		opts := Options{Backup: true, BackupDir: first.BackupDir}
		plan, err := eng.Plan(bashSpec, info, opts)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := eng.Apply(plan, opts); err == nil {
			t.Fatal("Apply() error = nil, want duplicate dest error")
		}
		if got := readString(t, filepath.Join(first.BackupDir, ".bashrc")); got != "user" {
			t.Errorf("earlier backup copy = %q, want untouched %q", got, "user")
		}
		if isSymlink(t, filepath.Join(home, ".bashrc")) {
			t.Error(".bashrc was replaced despite the refused backup")
		}
	})

	t.Run("missing-reuse-dir", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, ".bashrc"), []byte("user"))
		eng := NewEngine(repoRoot, home)
		opts := Options{Backup: true, BackupDir: filepath.Join(home, ".config", "dotfiles_backup_1")}
		plan, err := eng.Plan(bashSpec, info, opts)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := eng.Apply(plan, opts); err == nil {
			t.Fatal("Apply() error = nil, want missing reuse dir error")
		}
	})
}

func TestListBackups(t *testing.T) {
	home := t.TempDir()
	if sets, err := ListBackups(home); err != nil || len(sets) != 0 {
		t.Fatalf("ListBackups(no root) = %v, %v; want empty, nil", sets, err)
	}
	ent := []BackupEntry{{Dest: ".bashrc", Kind: BackupKindFile}}
	makeBackup(t, home, "100", time.Unix(100, 0).UTC(), map[string]string{".bashrc": "a"}, ent)
	makeBackup(t, home, "300", time.Unix(300, 0).UTC(), map[string]string{".bashrc": "abc"}, ent)
	makeBackup(t, home, "300_1", time.Unix(300, 0).UTC(), nil, ent)
	makeBackup(t, home, "200", time.Time{}, map[string]string{"x": "12345"}, nil) // legacy
	root := BackupRoot(home)
	// Non-matching names and a symlinked dir are ignored.
	for _, name := range []string{"dotfiles_backup_abc", "dotfiles_backup_1_2_3", "other", "dotfiles_backup_"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(root, "dotfiles_backup_400"), []byte("a file, not a dir"))
	mkSymlink(t, filepath.Join(root, BackupDirPrefix+"100"), filepath.Join(root, "dotfiles_backup_500"))

	sets, err := ListBackups(home)
	if err != nil {
		t.Fatalf("ListBackups error = %v", err)
	}
	var ids []string
	for _, s := range sets {
		ids = append(ids, s.ID)
	}
	if want := []string{"300_1", "300", "200", "100"}; !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	legacy := sets[2]
	if !legacy.Legacy || legacy.Entries != nil || !legacy.CreatedAt.Equal(time.Unix(200, 0)) || legacy.Size != 5 {
		t.Errorf("legacy set = %+v, want Legacy, nil entries, epoch 200, size 5", legacy)
	}
	if sets[1].Legacy || len(sets[1].Entries) != 1 || sets[1].Path != filepath.Join(root, "dotfiles_backup_300") {
		t.Errorf("set 300 = %+v, want manifest set with 1 entry", sets[1])
	}
	if sets[1].Size <= sets[3].Size {
		t.Errorf("size(300)=%d not > size(100)=%d", sets[1].Size, sets[3].Size)
	}
}

func TestFindBackup(t *testing.T) {
	home := t.TempDir()
	makeBackup(t, home, "100", time.Unix(100, 0), nil, []BackupEntry{})
	tests := []struct {
		id       string
		wantErr  bool
		notFound bool
	}{
		{id: "100"},
		{id: "dotfiles_backup_100"},
		{id: "101", wantErr: true, notFound: true},
		{id: "../x", wantErr: true},
		{id: "a/b", wantErr: true},
		{id: "abc", wantErr: true, notFound: true},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			set, err := FindBackup(home, tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("FindBackup(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
			var nf *BackupNotFoundError
			if errors.As(err, &nf) != tt.notFound {
				t.Errorf("FindBackup(%q) error = %v, notFound want %v", tt.id, err, tt.notFound)
			}
			if err == nil && set.ID != "100" {
				t.Errorf("ID = %q, want 100", set.ID)
			}
		})
	}
}

func TestRestore(t *testing.T) {
	fileEntry := BackupEntry{Dest: ".bashrc", Kind: BackupKindFile}
	dirEntry := BackupEntry{Dest: ".config/nvim", Kind: BackupKindDir}
	files := map[string]string{".bashrc": "backed bashrc", ".config/nvim/init.lua": "backed init"}

	tests := []struct {
		name        string
		entries     []BackupEntry // nil => legacy
		files       map[string]string
		setup       func(t *testing.T, home, backupDir string)
		opts        RestoreOptions
		wantErr     any // pointer to error type for errors.As, or true for any error
		wantActions map[string]string
		check       func(t *testing.T, home string, r *RestoreReport)
	}{
		{
			name:        "create-missing",
			entries:     []BackupEntry{fileEntry, dirEntry},
			files:       files,
			wantActions: map[string]string{".bashrc": RestoreActionCreate, ".config/nvim": RestoreActionCreate},
			check: func(t *testing.T, home string, r *RestoreReport) {
				if got := readString(t, filepath.Join(home, ".bashrc")); got != "backed bashrc" {
					t.Errorf(".bashrc = %q", got)
				}
				if got := readString(t, filepath.Join(home, ".config/nvim/init.lua")); got != "backed init" {
					t.Errorf("init.lua = %q", got)
				}
				if r.SafetyBackupDir != "" {
					t.Errorf("SafetyBackupDir = %q, want empty", r.SafetyBackupDir)
				}
			},
		},
		{
			name:    "replace-symlink",
			entries: []BackupEntry{fileEntry, dirEntry},
			files:   files,
			setup: func(t *testing.T, home, _ string) {
				mkSymlink(t, "/nonexistent/bashrc", filepath.Join(home, ".bashrc"))
				target := filepath.Join(home, "repo/nvim")
				writeFile(t, filepath.Join(target, "init.lua"), []byte("repo"))
				mkSymlink(t, target, filepath.Join(home, ".config/nvim"))
			},
			wantActions: map[string]string{".bashrc": RestoreActionReplaceSymlink, ".config/nvim": RestoreActionReplaceSymlink},
			check: func(t *testing.T, home string, _ *RestoreReport) {
				if isSymlink(t, filepath.Join(home, ".bashrc")) || isSymlink(t, filepath.Join(home, ".config/nvim")) {
					t.Error("dest still a symlink after restore")
				}
				if got := readString(t, filepath.Join(home, ".config/nvim/init.lua")); got != "backed init" {
					t.Errorf("init.lua = %q", got)
				}
				if got := readString(t, filepath.Join(home, "repo/nvim/init.lua")); got != "repo" {
					t.Errorf("symlink target modified: %q", got)
				}
			},
		},
		{
			name:    "conflict-refused",
			entries: []BackupEntry{fileEntry, dirEntry},
			files:   files,
			setup: func(t *testing.T, home, _ string) {
				writeFile(t, filepath.Join(home, ".bashrc"), []byte("current"))
			},
			wantErr: new(*RestoreConflictError),
			check: func(t *testing.T, home string, _ *RestoreReport) {
				if got := readString(t, filepath.Join(home, ".bashrc")); got != "current" {
					t.Errorf(".bashrc = %q, want untouched", got)
				}
				if _, err := os.Lstat(filepath.Join(home, ".config/nvim")); !os.IsNotExist(err) {
					t.Error(".config/nvim written despite conflict")
				}
			},
		},
		{
			name:    "force-safety-backup",
			entries: []BackupEntry{fileEntry, dirEntry},
			files:   files,
			setup: func(t *testing.T, home, _ string) {
				writeFile(t, filepath.Join(home, ".bashrc"), []byte("current"))
				writeFile(t, filepath.Join(home, ".config/nvim/extra.lua"), []byte("current extra"))
			},
			opts:        RestoreOptions{Force: true},
			wantActions: map[string]string{".bashrc": RestoreActionReplaceReal, ".config/nvim": RestoreActionReplaceReal},
			check: func(t *testing.T, home string, r *RestoreReport) {
				if r.SafetyBackupDir == "" {
					t.Fatal("SafetyBackupDir empty, want fresh dir")
				}
				if got := readString(t, filepath.Join(r.SafetyBackupDir, ".bashrc")); got != "current" {
					t.Errorf("safety .bashrc = %q", got)
				}
				if got := readString(t, filepath.Join(r.SafetyBackupDir, ".config/nvim/extra.lua")); got != "current extra" {
					t.Errorf("safety extra.lua = %q", got)
				}
				kinds := entryKinds(mustReadManifest(t, r.SafetyBackupDir))
				if kinds[".bashrc"] != BackupKindFile || kinds[".config/nvim"] != BackupKindDir {
					t.Errorf("safety manifest = %v", kinds)
				}
				if got := readString(t, filepath.Join(home, ".bashrc")); got != "backed bashrc" {
					t.Errorf(".bashrc = %q", got)
				}
				if _, err := os.Lstat(filepath.Join(home, ".config/nvim/extra.lua")); !os.IsNotExist(err) {
					t.Error("extra.lua survived restore, want exact dir restore")
				}
			},
		},
		{
			name:        "subset",
			entries:     []BackupEntry{fileEntry, dirEntry},
			files:       files,
			opts:        RestoreOptions{Paths: []string{"~/.config/nvim/"}},
			wantActions: map[string]string{".config/nvim": RestoreActionCreate},
			check: func(t *testing.T, home string, _ *RestoreReport) {
				if _, err := os.Lstat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
					t.Error(".bashrc restored, want only subset")
				}
			},
		},
		{
			name:    "unknown-path",
			entries: []BackupEntry{fileEntry},
			files:   files,
			opts:    RestoreOptions{Paths: []string{".bashrc", ".zshrc"}},
			wantErr: true,
			check: func(t *testing.T, home string, _ *RestoreReport) {
				if _, err := os.Lstat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
					t.Error(".bashrc restored despite unknown path")
				}
			},
		},
		{
			name:    "legacy",
			files:   files,
			wantErr: new(*LegacyBackupError),
		},
		{
			name:    "unsafe-parent",
			entries: []BackupEntry{fileEntry, {Dest: "../escape", Kind: BackupKindFile}},
			files:   files,
			wantErr: new(*UnsafeDestError),
			check: func(t *testing.T, home string, _ *RestoreReport) {
				if _, err := os.Lstat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
					t.Error(".bashrc restored despite unsafe entry")
				}
			},
		},
		{
			name:    "unsafe-absolute",
			entries: []BackupEntry{{Dest: "/etc/passwd", Kind: BackupKindFile}},
			files:   files,
			wantErr: new(*UnsafeDestError),
		},
		{
			name:    "unsafe-into-backup-dir",
			entries: []BackupEntry{{Dest: ".config/dotfiles_backup_1/x", Kind: BackupKindFile}},
			files:   map[string]string{".config/dotfiles_backup_1/x": "x"},
			wantErr: new(*UnsafeDestError),
		},
		{
			name:    "unsafe-backup-root",
			entries: []BackupEntry{{Dest: ".config", Kind: BackupKindDir}},
			files:   map[string]string{".config/x": "x"},
			wantErr: new(*UnsafeDestError),
		},
		{
			name:    "unsafe-backup-root-case-alias",
			entries: []BackupEntry{{Dest: ".CONFIG", Kind: BackupKindDir}},
			files:   map[string]string{".CONFIG/x": "x"},
			wantErr: new(*UnsafeDestError),
		},
		{
			name:    "unsafe-into-backup-dir-case-alias",
			entries: []BackupEntry{{Dest: ".Config/DOTFILES_BACKUP_1/x", Kind: BackupKindFile}},
			files:   map[string]string{".Config/DOTFILES_BACKUP_1/x": "x"},
			wantErr: new(*UnsafeDestError),
		},
		{
			name:    "unsafe-control-char",
			entries: []BackupEntry{{Dest: ".bash\x1b[2Jrc", Kind: BackupKindFile}},
			files:   map[string]string{".bash\x1b[2Jrc": "x"},
			wantErr: new(*UnsafeDestError),
		},
		{
			name:    "unsafe-symlink-alias-into-backup-dir",
			entries: []BackupEntry{{Dest: "cfgalias/dotfiles_backup_1000/f", Kind: BackupKindFile}},
			files:   map[string]string{"cfgalias/dotfiles_backup_1000/f": "x"},
			setup: func(t *testing.T, home, _ string) {
				mkSymlink(t, filepath.Join(home, ".config"), filepath.Join(home, "cfgalias"))
			},
			wantErr: new(*UnsafeDestError),
		},
		{
			name:    "unsafe-symlink-alias-of-backup-root",
			entries: []BackupEntry{{Dest: "sub/cfg/x", Kind: BackupKindFile}},
			files:   map[string]string{"sub/cfg/x": "x"},
			setup: func(t *testing.T, home, _ string) {
				if err := os.MkdirAll(filepath.Join(home, "sub"), 0o755); err != nil {
					t.Fatal(err)
				}
				// The dest's parent resolves into the backup dir itself.
				mkSymlink(t, filepath.Join(home, ".config", "dotfiles_backup_1000"), filepath.Join(home, "sub", "cfg"))
			},
			wantErr: new(*UnsafeDestError),
		},
		{
			name:    "unsafe-symlink-alias-equals-backup-root",
			entries: []BackupEntry{{Dest: "homealias/.config", Kind: BackupKindDir}},
			files:   map[string]string{"homealias/.config/x": "x"},
			setup: func(t *testing.T, home, _ string) {
				mkSymlink(t, home, filepath.Join(home, "homealias"))
			},
			opts:    RestoreOptions{Force: true},
			wantErr: new(*UnsafeDestError),
		},
		{
			name:    "partial-failure-reports-completed-only",
			entries: []BackupEntry{fileEntry, dirEntry},
			files:   files,
			setup: func(t *testing.T, home, _ string) {
				if os.Getuid() == 0 {
					t.Skip("running as root; permission checks do not apply")
				}
				// .bashrc restores; creating .config/nvim then fails.
				cfg := filepath.Join(home, ".config")
				if err := os.Chmod(cfg, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(cfg, 0o755) })
			},
			wantErr: true,
			check: func(t *testing.T, home string, r *RestoreReport) {
				if len(r.Items) != 1 || r.Items[0].Dest != ".bashrc" {
					t.Errorf("Items = %+v, want only .bashrc", r.Items)
				}
				if got := readString(t, filepath.Join(home, ".bashrc")); got != "backed bashrc" {
					t.Errorf(".bashrc = %q, want restored", got)
				}
			},
		},
		{
			name:    "source-symlink-escapes",
			entries: []BackupEntry{{Dest: ".config/evil/file", Kind: BackupKindFile}},
			setup: func(t *testing.T, home, backupDir string) {
				outside := filepath.Join(home, "outside")
				writeFile(t, filepath.Join(outside, "file"), []byte("outside"))
				mkSymlink(t, outside, filepath.Join(backupDir, ".config/evil"))
			},
			wantErr: true,
			check: func(t *testing.T, home string, _ *RestoreReport) {
				if _, err := os.Lstat(filepath.Join(home, ".config/evil")); !os.IsNotExist(err) {
					t.Error("restored through an escaping symlink")
				}
			},
		},
		{
			name:    "source-is-symlink",
			entries: []BackupEntry{fileEntry},
			setup: func(t *testing.T, _, backupDir string) {
				mkSymlink(t, "/etc/hostname", filepath.Join(backupDir, ".bashrc"))
			},
			wantErr: true,
		},
		{
			name:    "kind-mismatch",
			entries: []BackupEntry{{Dest: ".bashrc", Kind: BackupKindDir}},
			files:   files,
			wantErr: true,
		},
		{
			name:    "dry-run-no-writes",
			entries: []BackupEntry{fileEntry, dirEntry},
			files:   files,
			setup: func(t *testing.T, home, _ string) {
				writeFile(t, filepath.Join(home, ".bashrc"), []byte("current"))
			},
			opts:        RestoreOptions{DryRun: true, Force: true},
			wantActions: map[string]string{".bashrc": RestoreActionReplaceReal, ".config/nvim": RestoreActionCreate},
			check: func(t *testing.T, home string, r *RestoreReport) {
				if !r.DryRun || r.SafetyBackupDir != "" {
					t.Errorf("report = %+v, want DryRun and no safety dir", r)
				}
				if got := readString(t, filepath.Join(home, ".bashrc")); got != "current" {
					t.Errorf(".bashrc = %q, want untouched", got)
				}
				if _, err := os.Lstat(filepath.Join(home, ".config/nvim")); !os.IsNotExist(err) {
					t.Error(".config/nvim written under dry-run")
				}
				if sets, _ := ListBackups(home); len(sets) != 1 {
					t.Errorf("backup dirs = %d, want 1 (no safety dir)", len(sets))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			dir := makeBackup(t, home, "1000", time.Unix(1000, 0).UTC(), tt.files, tt.entries)
			if tt.setup != nil {
				tt.setup(t, home, dir)
			}
			before := snapshotTree(t, dir)

			report, err := NewEngine(t.TempDir(), home).Restore("1000", tt.opts)
			switch want := tt.wantErr.(type) {
			case nil:
				if err != nil {
					t.Fatalf("Restore error = %v, want nil", err)
				}
			case bool:
				if err == nil {
					t.Fatal("Restore error = nil, want error")
				}
			default:
				if err == nil || !errors.As(err, want) {
					t.Fatalf("Restore error = %v (%T), want %T", err, err, want)
				}
			}
			if tt.wantActions != nil {
				got := map[string]string{}
				for _, it := range report.Items {
					got[it.Dest] = it.Action
					if !strings.HasPrefix(it.SrcAbs, dir+string(filepath.Separator)) {
						t.Errorf("SrcAbs %q not inside %q", it.SrcAbs, dir)
					}
				}
				if len(got) != len(tt.wantActions) {
					t.Errorf("actions = %v, want %v", got, tt.wantActions)
				}
				for d, a := range tt.wantActions {
					if got[d] != a {
						t.Errorf("action[%q] = %q, want %q", d, got[d], a)
					}
				}
			}
			if after := snapshotTree(t, dir); len(after) != len(before) {
				t.Errorf("backup dir changed: %d entries -> %d", len(before), len(after))
			} else {
				for p, v := range before {
					if after[p] != v {
						t.Errorf("backup dir entry %q changed: %q -> %q", p, v, after[p])
					}
				}
			}
			if tt.check != nil {
				tt.check(t, home, report)
			}
		})
	}
}

func TestRestore_PreservesPerms(t *testing.T) {
	home := t.TempDir()
	dir := makeBackup(t, home, "1", time.Unix(1, 0), nil, []BackupEntry{
		{Dest: ".ssh/config", Kind: BackupKindFile},
		{Dest: ".gnupg", Kind: BackupKindDir},
	})
	p := filepath.Join(dir, ".ssh/config")
	writeFile(t, p, []byte("Host *"))
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".gnupg/gpg.conf"), []byte("x"))
	if err := os.Chmod(filepath.Join(dir, ".gnupg"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEngine(t.TempDir(), home).Restore("1", RestoreOptions{}); err != nil {
		t.Fatalf("Restore error = %v", err)
	}
	for rel, want := range map[string]os.FileMode{".ssh/config": 0o600, ".gnupg": 0o700} {
		fi, err := os.Stat(filepath.Join(home, rel))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", rel, fi.Mode().Perm(), want)
		}
	}
}

func TestRestore_NotFound(t *testing.T) {
	_, err := NewEngine(t.TempDir(), t.TempDir()).Restore("42", RestoreOptions{})
	var nf *BackupNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("Restore error = %v, want *BackupNotFoundError", err)
	}
}

func TestRestore_RefusesAliasContainingBackupRoot(t *testing.T) {
	home := t.TempDir()
	// parentalias -> dirname(home), so parentalias/<base(home)> is $HOME
	// itself: a dest that contains the backup root.
	dest := "parentalias/" + filepath.Base(home)
	makeBackup(t, home, "1", time.Unix(1, 0), map[string]string{dest + "/x": "x"},
		[]BackupEntry{{Dest: dest, Kind: BackupKindDir}})
	mkSymlink(t, filepath.Dir(home), filepath.Join(home, "parentalias"))
	_, err := NewEngine(t.TempDir(), home).Restore("1", RestoreOptions{Force: true})
	var unsafe *UnsafeDestError
	if !errors.As(err, &unsafe) {
		t.Fatalf("Restore error = %v, want *UnsafeDestError", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "dotfiles_backup_1")); err != nil {
		t.Errorf("backup dir gone: %v", err)
	}
}

func TestRestoreConflictError_QuotesDests(t *testing.T) {
	msg := (&RestoreConflictError{Dests: []string{".bashrc", "a\x1b[2Jb"}}).Error()
	if !strings.Contains(msg, `".bashrc", "a\x1b[2Jb"`) || strings.ContainsRune(msg, '\x1b') {
		t.Errorf("Error() = %q, want %%q-quoted dests without raw control chars", msg)
	}
}

func TestRestoreItem_RefusesChangedDest(t *testing.T) {
	tests := []struct {
		name   string
		action string
		setup  func(t *testing.T, dest string)
	}{
		{name: "create-now-exists", action: RestoreActionCreate, setup: func(t *testing.T, dest string) {
			writeFile(t, dest, []byte("appeared"))
		}},
		{name: "replace-symlink-now-real", action: RestoreActionReplaceSymlink, setup: func(t *testing.T, dest string) {
			writeFile(t, dest, []byte("appeared"))
		}},
		{name: "replace-real-now-symlink", action: RestoreActionReplaceReal, setup: func(t *testing.T, dest string) {
			mkSymlink(t, filepath.Join(filepath.Dir(dest), "target"), dest)
		}},
		{name: "replace-symlink-now-missing", action: RestoreActionReplaceSymlink, setup: func(*testing.T, string) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "src")
			writeFile(t, src, []byte("backup"))
			dest := filepath.Join(dir, "home", "dest")
			tt.setup(t, dest)
			before, _ := os.Lstat(dest)
			err := restoreItem(RestoreItem{Dest: "dest", DestAbs: dest, SrcAbs: src, Kind: BackupKindFile, Action: tt.action}, "")
			if err == nil || !strings.Contains(err.Error(), "changed since planning") {
				t.Fatalf("restoreItem error = %v, want changed-since-planning refusal", err)
			}
			after, _ := os.Lstat(dest)
			if (before == nil) != (after == nil) || (before != nil && before.Mode() != after.Mode()) {
				t.Errorf("dest changed despite refusal: %v -> %v", before, after)
			}
		})
	}
}

func TestRestore_FileWrittenViaRename(t *testing.T) {
	home := t.TempDir()
	makeBackup(t, home, "1", time.Unix(1, 0), map[string]string{".bashrc": "backed"},
		[]BackupEntry{{Dest: ".bashrc", Kind: BackupKindFile}})
	outside := filepath.Join(t.TempDir(), "outside")
	writeFile(t, outside, []byte("untouched"))
	mkSymlink(t, outside, filepath.Join(home, ".bashrc"))
	if _, err := NewEngine(t.TempDir(), home).Restore("1", RestoreOptions{}); err != nil {
		t.Fatalf("Restore error = %v", err)
	}
	if got := readString(t, outside); got != "untouched" {
		t.Errorf("symlink target = %q, want untouched", got)
	}
	fi, err := os.Lstat(filepath.Join(home, ".bashrc"))
	if err != nil || !fi.Mode().IsRegular() {
		t.Fatalf(".bashrc = %v, %v; want regular file", fi, err)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "dotf-restore") {
			t.Errorf("leftover temp file %q", e.Name())
		}
	}
}

// TestApply_ZedCopyBackupIsRestorable proves the differing real file at a Copy
// dest is recorded in the manifest and that restore puts it back over the repo
// copy.
func TestApply_ZedCopyBackupIsRestorable(t *testing.T) {
	repoRoot, home := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "zed/settings.json"), []byte("repo zed"))
	dest := filepath.Join(home, ".config/zed/settings.json")
	writeFile(t, dest, []byte("user zed"))
	specs := []LinkSpec{{Src: "zed/settings.json", Dest: ".config/zed/settings.json", Facet: FacetAll, Copy: true}}

	report := planApply(t, repoRoot, home, specs, platform.Info{OS: platform.OSDebian}, Options{Backup: true})
	if report.BackupDir == "" {
		t.Fatal("BackupDir empty; want the Zed copy backed up")
	}
	if got := entryKinds(mustReadManifest(t, report.BackupDir)); len(got) != 1 || got[".config/zed/settings.json"] != BackupKindFile {
		t.Fatalf("manifest entries = %v, want only .config/zed/settings.json (file)", got)
	}
	if got := readString(t, dest); got != "repo zed" {
		t.Fatalf("dest after apply = %q, want repo copy", got)
	}

	id := strings.TrimPrefix(filepath.Base(report.BackupDir), BackupDirPrefix)
	if _, err := NewEngine(repoRoot, home).Restore(id, RestoreOptions{Force: true}); err != nil {
		t.Fatalf("Restore error = %v", err)
	}
	if got := readString(t, dest); got != "user zed" {
		t.Errorf("dest after restore = %q, want user content", got)
	}
	if got := readString(t, filepath.Join(repoRoot, "zed/settings.json")); got != "repo zed" {
		t.Errorf("repo source = %q, want untouched", got)
	}
}

// TestRestore_RefusesDestInsideRepo checks a dest whose parent dir is a
// symlink into the repo is refused before any write, while a symlink at the
// dest itself pointing into the repo is replaced, never written through.
func TestRestore_RefusesDestInsideRepo(t *testing.T) {
	t.Run("symlinked parent", func(t *testing.T) {
		repoRoot, home := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(repoRoot, "zed/settings.json"), []byte("repo zed"))
		makeBackup(t, home, "1", time.Unix(1, 0), map[string]string{".config/zed/settings.json": "backed"},
			[]BackupEntry{{Dest: ".config/zed/settings.json", Kind: BackupKindFile}})
		mkSymlink(t, filepath.Join(repoRoot, "zed"), filepath.Join(home, ".config/zed"))
		before := snapshotTree(t, repoRoot)

		_, err := NewEngine(repoRoot, home).Restore("1", RestoreOptions{Force: true})
		var inRepo *DestInRepoError
		if !errors.As(err, &inRepo) {
			t.Fatalf("Restore error = %v, want *DestInRepoError", err)
		}
		if after := snapshotTree(t, repoRoot); !maps.Equal(before, after) {
			t.Errorf("repo modified: %v -> %v", before, after)
		}
	})
	t.Run("symlink at dest", func(t *testing.T) {
		repoRoot, home := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(repoRoot, "bash/bashrc"), []byte("repo bashrc"))
		makeBackup(t, home, "1", time.Unix(1, 0), map[string]string{".bashrc": "backed"},
			[]BackupEntry{{Dest: ".bashrc", Kind: BackupKindFile}})
		mkSymlink(t, filepath.Join(repoRoot, "bash/bashrc"), filepath.Join(home, ".bashrc"))

		if _, err := NewEngine(repoRoot, home).Restore("1", RestoreOptions{}); err != nil {
			t.Fatalf("Restore error = %v", err)
		}
		if got := readString(t, filepath.Join(repoRoot, "bash/bashrc")); got != "repo bashrc" {
			t.Errorf("repo source = %q, want untouched", got)
		}
		fi, err := os.Lstat(filepath.Join(home, ".bashrc"))
		if err != nil || !fi.Mode().IsRegular() {
			t.Fatalf(".bashrc = %v, %v; want regular file", fi, err)
		}
	})
	t.Run("missing repo root still restores", func(t *testing.T) {
		home := t.TempDir()
		repoRoot := filepath.Join(t.TempDir(), "gone")
		makeBackup(t, home, "1", time.Unix(1, 0), map[string]string{".bashrc": "backed"},
			[]BackupEntry{{Dest: ".bashrc", Kind: BackupKindFile}})
		mkSymlink(t, filepath.Join(repoRoot, "bash/bashrc"), filepath.Join(home, ".bashrc"))

		if _, err := NewEngine(repoRoot, home).Restore("1", RestoreOptions{}); err != nil {
			t.Fatalf("Restore error = %v, want rollback to work without the repo", err)
		}
		if got := readString(t, filepath.Join(home, ".bashrc")); got != "backed" {
			t.Errorf(".bashrc = %q, want %q", got, "backed")
		}
	})
}

func TestReadManifest_RejectsNonRegular(t *testing.T) {
	t.Run("symlinked manifest is legacy", func(t *testing.T) {
		home := t.TempDir()
		valid := makeBackup(t, home, "1", time.Unix(1, 0), map[string]string{".x": "x"},
			[]BackupEntry{{Dest: ".x", Kind: BackupKindFile}})
		dir := makeBackup(t, home, "2", time.Unix(2, 0), map[string]string{".x": "x"}, nil)
		mkSymlink(t, filepath.Join(valid, BackupManifestName), filepath.Join(dir, BackupManifestName))
		if m, err := readManifest(dir); err == nil || m != nil {
			t.Fatalf("readManifest = %v, %v; want error", m, err)
		}
		set, err := FindBackup(home, "2")
		if err != nil || !set.Legacy {
			t.Errorf("FindBackup = %+v, %v; want legacy", set, err)
		}
	})
	t.Run("oversized manifest is legacy", func(t *testing.T) {
		home := t.TempDir()
		dir := makeBackup(t, home, "1", time.Unix(1, 0), nil, nil)
		// Valid JSON padded past the cap with trailing whitespace.
		head := []byte(`{"version":1,"entries":[]}`)
		data := append(head, []byte(strings.Repeat(" ", maxManifestSize))...)
		writeFile(t, filepath.Join(dir, BackupManifestName), data)
		if m, err := readManifest(dir); err == nil || m != nil {
			t.Fatalf("readManifest = %v, %v; want error", m, err)
		}
		set, err := FindBackup(home, "1")
		if err != nil || !set.Legacy {
			t.Errorf("FindBackup = %+v, %v; want legacy", set, err)
		}
	})
}

func TestSelectPrune(t *testing.T) {
	now := time.Unix(100*86400, 0)
	day := 24 * time.Hour
	// Newest first: ages 0, 1, 5, 10, 30 days.
	var sets []BackupSet
	for _, age := range []int{0, 1, 5, 10, 30} {
		sets = append(sets, BackupSet{ID: strings.Repeat("x", age+1), CreatedAt: now.Add(-time.Duration(age) * day)})
	}
	ids := func(ss []BackupSet) []int {
		var out []int
		for _, s := range ss {
			out = append(out, len(s.ID)-1)
		}
		return out
	}
	tests := []struct {
		name string
		opts PruneOptions
		want []int // ages selected
	}{
		{name: "keep-default", opts: PruneOptions{Keep: DefaultBackupKeep, Now: now}},
		{name: "keep-2", opts: PruneOptions{Keep: 2, Now: now}, want: []int{5, 10, 30}},
		{name: "keep-0", opts: PruneOptions{Keep: 0, Now: now}, want: []int{0, 1, 5, 10, 30}},
		{name: "age-only", opts: PruneOptions{Keep: 0, OlderThan: 7 * day, Now: now}, want: []int{10, 30}},
		{name: "keep-and-age", opts: PruneOptions{Keep: 4, OlderThan: 7 * day, Now: now}, want: []int{30}},
		{name: "age-excludes-kept-range", opts: PruneOptions{Keep: 1, OlderThan: 2 * day, Now: now}, want: []int{5, 10, 30}},
		{name: "age-boundary-not-older", opts: PruneOptions{Keep: 0, OlderThan: 30 * day, Now: now}},
		{name: "negative-keep-as-zero", opts: PruneOptions{Keep: -1, Now: now}, want: []int{0, 1, 5, 10, 30}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ids(SelectPrune(sets, tt.opts)); !slices.Equal(got, tt.want) {
				t.Errorf("SelectPrune = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoveBackup(t *testing.T) {
	home := t.TempDir()
	root := BackupRoot(home)
	valid := makeBackup(t, home, "100", time.Unix(100, 0), map[string]string{"a": "a"}, nil)
	other := filepath.Join(root, "nvim")
	writeFile(t, filepath.Join(other, "init.lua"), []byte("keep"))
	nested := filepath.Join(home, "sub", "dotfiles_backup_5")
	writeFile(t, filepath.Join(nested, "f"), []byte("keep"))
	link := filepath.Join(root, "dotfiles_backup_7")
	mkSymlink(t, other, link)
	plainFile := filepath.Join(root, "dotfiles_backup_8")
	writeFile(t, plainFile, []byte("file"))

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "non-matching-name", path: other, wantErr: true},
		{name: "not-under-root", path: nested, wantErr: true},
		{name: "traversal", path: filepath.Join(root, "x", "..", "nvim"), wantErr: true},
		{name: "symlinked-dir", path: link, wantErr: true},
		{name: "plain-file", path: plainFile, wantErr: true},
		{name: "missing", path: filepath.Join(root, "dotfiles_backup_9"), wantErr: true},
		{name: "valid", path: valid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RemoveBackup(home, BackupSet{Path: tt.path})
			if (err != nil) != tt.wantErr {
				t.Fatalf("RemoveBackup(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
			_, serr := os.Lstat(tt.path)
			if tt.wantErr && tt.name != "missing" && serr != nil {
				t.Errorf("%q removed despite refusal", tt.path)
			}
			if !tt.wantErr && !os.IsNotExist(serr) {
				t.Errorf("%q still exists after RemoveBackup", tt.path)
			}
		})
	}
	if got := readString(t, filepath.Join(other, "init.lua")); got != "keep" {
		t.Errorf("symlink target content = %q, want kept", got)
	}
}

func TestRemoveBackup_ReadOnlySubdir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission checks do not apply")
	}
	home := t.TempDir()
	dir := makeBackup(t, home, "1", time.Unix(1, 0), map[string]string{".gnupg/sub/f": "x"}, nil)
	ro := filepath.Join(dir, ".gnupg")
	for _, p := range []string{filepath.Join(ro, "sub"), ro} {
		if err := os.Chmod(p, 0o500); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = os.Chmod(ro, 0o755)
		_ = os.Chmod(filepath.Join(ro, "sub"), 0o755)
	})
	if err := RemoveBackup(home, BackupSet{Path: dir}); err != nil {
		t.Fatalf("RemoveBackup error = %v", err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Errorf("%q still exists", dir)
	}
}
