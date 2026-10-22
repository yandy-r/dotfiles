package symlink

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// unlinkItemFor returns the report item for dest.
func unlinkItemFor(t *testing.T, rep *UnlinkReport, dest string) UnlinkItem {
	t.Helper()
	for _, it := range rep.Items {
		if it.Dest == dest {
			return it
		}
	}
	t.Fatalf("no unlink item for %q in %+v", dest, rep.Items)
	return UnlinkItem{}
}

func TestUnlink_Named(t *testing.T) {
	tests := []struct {
		name string
		dest func(home string) string
	}{
		{name: "plain", dest: func(string) string { return ".config/foo" }},
		{name: "tilde-prefix", dest: func(string) string { return "~/.config/foo" }},
		{name: "unclean", dest: func(string) string { return ".config//foo/" }},
		{name: "absolute-under-home", dest: func(home string) string { return filepath.Join(home, ".config/foo") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			home := t.TempDir()
			buildPruneRepo(t, repo)
			link := filepath.Join(home, ".config/foo")
			mkSymlink(t, filepath.Join(repo, "cfg/foo"), link)
			other := filepath.Join(home, ".bashrc")
			mkSymlink(t, filepath.Join(repo, "bash/bashrc"), other)

			rep, err := NewEngine(repo, home).Unlink(pruneTestSpecs(), UnlinkOptions{Dests: []string{tt.dest(home)}})
			if err != nil {
				t.Fatalf("Unlink() error = %v", err)
			}
			if len(rep.Items) != 1 || rep.Items[0].Dest != ".config/foo" || rep.Items[0].Action != UnlinkActionRemove {
				t.Fatalf("Unlink() items = %+v, want one remove of .config/foo", rep.Items)
			}
			if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("link %q still exists (err=%v)", link, err)
			}
			if !isSymlink(t, other) {
				t.Errorf("untargeted link %q was touched", other)
			}
			if _, err := os.Stat(filepath.Join(repo, "cfg/foo")); err != nil {
				t.Errorf("repo source removed: %v", err)
			}
		})
	}
}

func TestUnlink_UnknownDest_NoChanges(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	buildPruneRepo(t, repo)
	link := filepath.Join(home, ".config/foo")
	mkSymlink(t, filepath.Join(repo, "cfg/foo"), link)

	outside := filepath.Join(repo, ".config/foo") // absolute, outside home
	_, err := NewEngine(repo, home).Unlink(pruneTestSpecs(), UnlinkOptions{Dests: []string{".config/foo", ".nope", "~/.also-nope", outside}})
	var unknown *UnknownDestError
	if !errors.As(err, &unknown) {
		t.Fatalf("Unlink() error = %v, want UnknownDestError", err)
	}
	if len(unknown.Dests) != 3 || unknown.Dests[0] != ".nope" || unknown.Dests[1] != "~/.also-nope" || unknown.Dests[2] != outside {
		t.Errorf("UnknownDestError.Dests = %v", unknown.Dests)
	}
	if got, want := err.Error(), `symlink: unknown dest(s) ".nope", "~/.also-nope", "`+outside+`": not in the link table`; got != want {
		t.Errorf("Error() = %q", got)
	}
	if !isSymlink(t, link) {
		t.Errorf("link %q changed despite the error", link)
	}
}

func TestUnlink_All_SkipReasons(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	buildPruneRepo(t, repo)

	// .bashrc: managed link (dangling, still into the repo) -> remove.
	mkSymlink(t, filepath.Join(repo, "bash/moved"), filepath.Join(home, ".bashrc"))
	// .config/foo: real file -> skip.
	writeFile(t, filepath.Join(home, ".config/foo"), []byte("real"))
	// .config/nvim: dir link into repo -> remove. Its child dest is a
	// repo-tracked symlink seen through that link -> skip "inside repo"
	// (classified before any removal).
	mkSymlink(t, filepath.Join(repo, "nvim"), filepath.Join(home, ".config/nvim"))
	initLua := filepath.Join(repo, "nvim/init.lua")
	if err := os.Remove(initLua); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "nvim/real.lua"), []byte("lua"))
	mkSymlink(t, "real.lua", initLua)
	// .config/hypr: foreign symlink -> skip.
	writeFile(t, filepath.Join(home, "elsewhere/hypr/x"), []byte("x"))
	mkSymlink(t, filepath.Join(home, "elsewhere/hypr"), filepath.Join(home, ".config/hypr"))
	// .config/zed/settings.json: copy entry -> skip even as a symlink.
	mkSymlink(t, filepath.Join(repo, "zed/settings.json"), filepath.Join(home, ".config/zed/settings.json"))

	specs := pruneTestSpecs()
	specs = append(specs, LinkSpec{Src: "bash/bashrc", Dest: ".bashrc", Facet: FacetMacOS}) // duplicate dest
	rep, err := NewEngine(repo, home).Unlink(specs, UnlinkOptions{All: true})
	if err != nil {
		t.Fatalf("Unlink() error = %v", err)
	}
	want := []struct{ dest, action, reason string }{
		{".bashrc", UnlinkActionRemove, ""},
		{".config/foo", UnlinkActionSkip, "real file"},
		{".config/nvim", UnlinkActionRemove, ""},
		{".config/nvim/init.lua", UnlinkActionSkip, "inside repo"},
		{".config/hypr", UnlinkActionSkip, "not a dotfiles link"},
		{".config/zed/settings.json", UnlinkActionSkip, "copy entry"},
		{".config/zed/keymap.json", UnlinkActionSkip, "missing"},
		{".zshrc", UnlinkActionSkip, "missing"},
	}
	if len(rep.Items) != len(want) {
		t.Fatalf("Unlink() items = %+v, want %d items", rep.Items, len(want))
	}
	for i, w := range want {
		it := rep.Items[i]
		if it.Dest != w.dest || it.Action != w.action || it.Reason != w.reason {
			t.Errorf("item[%d] = %+v, want %+v", i, it, w)
		}
	}
	for _, gone := range []string{".bashrc", ".config/nvim"} {
		if _, err := os.Lstat(filepath.Join(home, gone)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%q still exists (err=%v)", gone, err)
		}
	}
	if readString(t, filepath.Join(home, ".config/foo")) != "real" {
		t.Error("real file modified")
	}
	if !isSymlink(t, filepath.Join(home, ".config/hypr")) || !isSymlink(t, filepath.Join(home, ".config/zed/settings.json")) {
		t.Error("skipped symlinks were removed")
	}
	if _, err := os.Stat(filepath.Join(repo, "nvim/init.lua")); err != nil {
		t.Errorf("repo file touched: %v", err)
	}
}

func TestUnlink_Missing(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	buildPruneRepo(t, repo)
	rep, err := NewEngine(repo, home).Unlink(pruneTestSpecs(), UnlinkOptions{Dests: []string{".bashrc"}})
	if err != nil {
		t.Fatalf("Unlink() error = %v", err)
	}
	if it := unlinkItemFor(t, rep, ".bashrc"); it.Action != UnlinkActionSkip || it.Reason != "missing" {
		t.Errorf("item = %+v, want skip missing", it)
	}
}

func TestUnlink_Restore(t *testing.T) {
	tests := []struct {
		name   string
		dryRun bool
	}{
		{name: "real"},
		{name: "dry-run", dryRun: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			home := t.TempDir()
			buildPruneRepo(t, repo)
			now := time.Now().UTC()
			ent := func(dest string) BackupEntry {
				return BackupEntry{Dest: dest, Kind: BackupKindFile, BackedUpAt: now}
			}
			// Older backup holds .bashrc; the newest holds only .config/foo;
			// the middle one holds .bashrc too and must win for it.
			makeBackup(t, home, "1000", time.Unix(1000, 0).UTC(),
				map[string]string{".bashrc": "oldest"}, []BackupEntry{ent(".bashrc")})
			makeBackup(t, home, "2000", time.Unix(2000, 0).UTC(),
				map[string]string{".bashrc": "middle"}, []BackupEntry{ent(".bashrc")})
			makeBackup(t, home, "3000", time.Unix(3000, 0).UTC(),
				map[string]string{".config/foo": "newest-foo"}, []BackupEntry{ent(".config/foo")})

			bashrc := filepath.Join(home, ".bashrc")
			mkSymlink(t, filepath.Join(repo, "bash/bashrc"), bashrc)
			hyprDir := filepath.Join(home, ".config/hypr")
			mkSymlink(t, filepath.Join(repo, "hypr"), hyprDir) // no backup holds it

			rep, err := NewEngine(repo, home).Unlink(pruneTestSpecs(), UnlinkOptions{
				Dests: []string{".bashrc", ".config/hypr"}, Restore: true, DryRun: tt.dryRun,
			})
			if err != nil {
				t.Fatalf("Unlink() error = %v", err)
			}
			if rep.DryRun != tt.dryRun {
				t.Errorf("DryRun = %t, want %t", rep.DryRun, tt.dryRun)
			}
			if it := unlinkItemFor(t, rep, ".bashrc"); it.Action != UnlinkActionRemove || it.BackupID != "2000" {
				t.Errorf(".bashrc item = %+v, want remove from backup 2000", it)
			}
			if it := unlinkItemFor(t, rep, ".config/hypr"); it.Action != UnlinkActionRemove || it.BackupID != "" {
				t.Errorf(".config/hypr item = %+v, want remove without backup", it)
			}
			if len(rep.Restored) != 1 || rep.Restored[0].Dest != ".bashrc" {
				t.Fatalf("Restored = %+v, want one .bashrc item", rep.Restored)
			}

			if tt.dryRun {
				if rep.Restored[0].Action != RestoreActionReplaceSymlink {
					t.Errorf("planned restore action = %q, want %q", rep.Restored[0].Action, RestoreActionReplaceSymlink)
				}
				if !isSymlink(t, bashrc) || !isSymlink(t, hyprDir) {
					t.Error("dry run removed a link")
				}
				return
			}
			if rep.Restored[0].Action != RestoreActionCreate {
				t.Errorf("restore action = %q, want %q", rep.Restored[0].Action, RestoreActionCreate)
			}
			if isSymlink(t, bashrc) || readString(t, bashrc) != "middle" {
				t.Errorf(".bashrc not restored from backup 2000")
			}
			if _, err := os.Lstat(hyprDir); !errors.Is(err, os.ErrNotExist) {
				t.Errorf(".config/hypr still exists (err=%v)", err)
			}
		})
	}
}

func TestUnlink_MissingRepo(t *testing.T) {
	_, err := NewEngine(filepath.Join(t.TempDir(), "absent"), t.TempDir()).Unlink(pruneTestSpecs(), UnlinkOptions{All: true})
	var missing *MissingSourceTreeError
	if !errors.As(err, &missing) {
		t.Fatalf("Unlink() error = %v, want MissingSourceTreeError", err)
	}
}

func TestUnlink_RemoveFailure_ReportsOnlyPerformed(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	repo := t.TempDir()
	home := t.TempDir()
	buildPruneRepo(t, repo)
	bashrc := filepath.Join(home, ".bashrc")
	mkSymlink(t, filepath.Join(repo, "bash/bashrc"), bashrc)
	cfg := filepath.Join(home, ".config")
	for _, l := range []struct{ src, dest string }{
		{"cfg/foo", ".config/foo"}, {"nvim", ".config/nvim"}, {"hypr", ".config/hypr"},
	} {
		mkSymlink(t, filepath.Join(repo, l.src), filepath.Join(home, l.dest))
	}
	// A read-only ~/.config makes removing .config/foo (the second remove)
	// fail; .config/nvim and .config/hypr come later.
	if err := os.Chmod(cfg, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfg, 0o755) })

	rep, err := NewEngine(repo, home).Unlink(pruneTestSpecs(), UnlinkOptions{All: true})
	if err == nil {
		t.Fatal("Unlink() error = nil, want a removal error")
	}
	want := map[string]struct{ action, reason string }{
		".bashrc":      {UnlinkActionRemove, ""},
		".config/foo":  {UnlinkActionSkip, unlinkSkipRemoveFailed},
		".config/nvim": {UnlinkActionSkip, unlinkSkipNotAttempted},
		".config/hypr": {UnlinkActionSkip, unlinkSkipNotAttempted},
	}
	for dest, w := range want {
		if it := unlinkItemFor(t, rep, dest); it.Action != w.action || it.Reason != w.reason {
			t.Errorf("%s item = %+v, want %s %q", dest, it, w.action, w.reason)
		}
	}
	for _, it := range rep.Items {
		if it.Action != UnlinkActionRemove {
			continue
		}
		if _, err := os.Lstat(it.DestAbs); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("report names removal of %q, but it still exists (err=%v)", it.Dest, err)
		}
	}
	for _, kept := range []string{".config/foo", ".config/nvim", ".config/hypr"} {
		if !isSymlink(t, filepath.Join(home, kept)) {
			t.Errorf("%q removed, want kept", kept)
		}
	}
}

// TestUnlink_RemoveFailure_RestoresRemoved proves a removal error does not
// strand an already-removed dest: it is still restored from its backup, the
// error is returned, and a not-attempted dest keeps its BackupID unrestored.
func TestUnlink_RemoveFailure_RestoresRemoved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	repo := t.TempDir()
	home := t.TempDir()
	buildPruneRepo(t, repo)
	now := time.Now().UTC()
	makeBackup(t, home, "2000", time.Unix(2000, 0).UTC(),
		map[string]string{".bashrc": "saved-bashrc", ".config/nvim": "saved-nvim"},
		[]BackupEntry{
			{Dest: ".bashrc", Kind: BackupKindFile, BackedUpAt: now},
			{Dest: ".config/nvim", Kind: BackupKindFile, BackedUpAt: now},
		})
	bashrc := filepath.Join(home, ".bashrc")
	mkSymlink(t, filepath.Join(repo, "bash/bashrc"), bashrc)
	for _, l := range []struct{ src, dest string }{
		{"cfg/foo", ".config/foo"}, {"nvim", ".config/nvim"},
	} {
		mkSymlink(t, filepath.Join(repo, l.src), filepath.Join(home, l.dest))
	}
	// A read-only ~/.config makes removing .config/foo (after .bashrc) fail.
	cfg := filepath.Join(home, ".config")
	if err := os.Chmod(cfg, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfg, 0o755) })

	rep, err := NewEngine(repo, home).Unlink(pruneTestSpecs(), UnlinkOptions{
		Dests: []string{".bashrc", ".config/foo", ".config/nvim"}, Restore: true,
	})
	if err == nil {
		t.Fatal("Unlink() error = nil, want the removal error")
	}
	if rep == nil {
		t.Fatal("Unlink() report = nil, want the partial report")
	}
	if it := unlinkItemFor(t, rep, ".bashrc"); it.Action != UnlinkActionRemove || it.BackupID != "2000" || !it.Restored {
		t.Errorf(".bashrc item = %+v, want removed and restored from 2000", it)
	}
	if it := unlinkItemFor(t, rep, ".config/foo"); it.Action != UnlinkActionSkip || it.Reason != unlinkSkipRemoveFailed || it.Restored {
		t.Errorf(".config/foo item = %+v, want skip %q", it, unlinkSkipRemoveFailed)
	}
	if it := unlinkItemFor(t, rep, ".config/nvim"); it.Action != UnlinkActionSkip ||
		it.Reason != unlinkSkipNotAttempted || it.BackupID != "2000" || it.Restored {
		t.Errorf(".config/nvim item = %+v, want unrestored skip %q keeping BackupID 2000", it, unlinkSkipNotAttempted)
	}
	if len(rep.Restored) != 1 || rep.Restored[0].Dest != ".bashrc" {
		t.Errorf("Restored = %+v, want one .bashrc item", rep.Restored)
	}
	if isSymlink(t, bashrc) || readString(t, bashrc) != "saved-bashrc" {
		t.Error(".bashrc not restored from backup 2000")
	}
	for _, kept := range []string{".config/foo", ".config/nvim"} {
		if !isSymlink(t, filepath.Join(home, kept)) {
			t.Errorf("%q changed, want the link kept", kept)
		}
	}
}

func TestUnlink_RestorePrecheckFailure_NoChanges(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	buildPruneRepo(t, repo)
	now := time.Now().UTC()
	dir := makeBackup(t, home, "1000", time.Unix(1000, 0).UTC(),
		map[string]string{".bashrc": "saved"},
		[]BackupEntry{{Dest: ".bashrc", Kind: BackupKindFile, BackedUpAt: now}})
	// The manifest still lists .bashrc, but its copy is gone.
	if err := os.Remove(filepath.Join(dir, ".bashrc")); err != nil {
		t.Fatal(err)
	}
	bashrc := filepath.Join(home, ".bashrc")
	mkSymlink(t, filepath.Join(repo, "bash/bashrc"), bashrc)
	foo := filepath.Join(home, ".config/foo")
	mkSymlink(t, filepath.Join(repo, "cfg/foo"), foo) // no backup holds it

	rep, err := NewEngine(repo, home).Unlink(pruneTestSpecs(), UnlinkOptions{
		Dests: []string{".bashrc", ".config/foo"}, Restore: true,
	})
	if err == nil {
		t.Fatalf("Unlink() error = nil, report %+v; want the restore pre-check error", rep)
	}
	if rep != nil {
		t.Errorf("Unlink() report = %+v, want nil (nothing done)", rep)
	}
	if !isSymlink(t, bashrc) || !isSymlink(t, foo) {
		t.Error("a link was removed despite the failed restore pre-check")
	}
}
