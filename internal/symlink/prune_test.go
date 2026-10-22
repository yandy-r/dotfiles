package symlink

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// pruneTestSpecs is a small table covering a home-level file, .config
// entries, a dir link, a Hyprland-faceted entry and the Zed copy.
func pruneTestSpecs() []LinkSpec {
	return []LinkSpec{
		{Src: "bash/bashrc", Dest: ".bashrc", Facet: FacetAll},
		{Src: "cfg/foo", Dest: ".config/foo", Facet: FacetAll},
		{Src: "nvim", Dest: ".config/nvim", IsDir: true, Facet: FacetAll},
		{Src: "nvim/init.lua", Dest: ".config/nvim/init.lua", Facet: FacetAll},
		{Src: "hypr", Dest: ".config/hypr", IsDir: true, Facet: FacetHyprland},
		{Src: "zed/settings.json", Dest: ".config/zed/settings.json", Copy: true, Facet: FacetAll},
		{Src: "zed/keymap.json", Dest: ".config/zed/keymap.json", Facet: FacetAll},
		{Src: "zsh/zshrc", Dest: ".zshrc", Facet: FacetAll},
	}
}

// buildPruneRepo materializes every pruneTestSpecs source under repo.
func buildPruneRepo(t *testing.T, repo string) {
	t.Helper()
	writeFile(t, filepath.Join(repo, "bash/bashrc"), []byte("rc"))
	writeFile(t, filepath.Join(repo, "cfg/foo"), []byte("foo"))
	writeFile(t, filepath.Join(repo, "nvim/init.lua"), []byte("lua"))
	writeFile(t, filepath.Join(repo, "hypr/hyprland.conf"), []byte("hypr"))
	writeFile(t, filepath.Join(repo, "zed/settings.json"), []byte("{}"))
	writeFile(t, filepath.Join(repo, "zed/keymap.json"), []byte("[]"))
	writeFile(t, filepath.Join(repo, "zsh/zshrc"), []byte("zrc"))
	writeFile(t, filepath.Join(repo, "plasma/files/kdeglobals"), []byte("kde"))
}

func TestFindStale(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, repo, home string)
		want  map[string]string // dest -> reason
	}{
		{
			name: "old-location-of-table-source-orphaned",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, filepath.Join(repo, "nvim"), filepath.Join(home, ".oldnvim"))
				mkSymlink(t, filepath.Join(repo, "cfg/foo"), filepath.Join(home, ".config/oldfoo"))
			},
			want: map[string]string{".oldnvim": StaleReasonOrphaned, ".config/oldfoo": StaleReasonOrphaned},
		},
		{
			name: "live-link-to-non-table-repo-path-not-stale",
			setup: func(t *testing.T, repo, home string) {
				writeFile(t, filepath.Join(repo, ".claude/settings.json"), []byte("{}"))
				mkSymlink(t, filepath.Join(repo, ".claude"), filepath.Join(home, ".claude"))
				writeFile(t, filepath.Join(repo, "cfg/old"), []byte("old"))
				mkSymlink(t, filepath.Join(repo, "cfg/old"), filepath.Join(home, ".config/old"))
			},
		},
		{
			name: "parent-dir-link-of-table-dests-not-stale",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, filepath.Join(repo, "zed"), filepath.Join(home, ".config/zed"))
			},
		},
		{
			name: "dangling-parent-dir-link-not-stale",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, filepath.Join(repo, "zed-gone"), filepath.Join(home, ".config/zed"))
			},
		},
		{
			name: "relative-link-to-managed-link-not-stale",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, filepath.Join(repo, "zsh/zshrc"), filepath.Join(home, ".zshrc"))
				mkSymlink(t, ".zshrc", filepath.Join(home, ".zshenv"))
			},
		},
		{
			name: "relative-dotdot-through-symlinked-scan-dir-resolves-physically",
			setup: func(t *testing.T, repo, home string) {
				// ~/.config -> <elsewhere>/deep/cfg, so ".." walks a different
				// chain physically than lexically.
				elsewhere := filepath.Join(t.TempDir(), "deep")
				if err := os.MkdirAll(filepath.Join(elsewhere, "cfg"), 0o755); err != nil {
					t.Fatal(err)
				}
				mkSymlink(t, filepath.Join(elsewhere, "cfg"), filepath.Join(home, ".config"))
				rel, err := filepath.Rel(home, filepath.Join(repo, "nvim"))
				if err != nil {
					t.Fatal(err)
				}
				// From ~/.config lexically, "../"+rel lands on repo/nvim (a
				// table source); physically it lands under elsewhere.
				mkSymlink(t, "../"+rel, filepath.Join(home, ".config/phys"))
			},
		},
		{
			name: "relative-dotdot-through-symlinked-scan-dir-into-repo",
			setup: func(t *testing.T, repo, home string) {
				elsewhere := t.TempDir()
				if err := os.MkdirAll(filepath.Join(elsewhere, "cfg"), 0o755); err != nil {
					t.Fatal(err)
				}
				mkSymlink(t, filepath.Join(elsewhere, "cfg"), filepath.Join(home, ".config"))
				// Physically from <elsewhere>/cfg, this reaches repo/nvim.
				rel, err := filepath.Rel(filepath.Join(elsewhere, "cfg"), filepath.Join(repo, "nvim"))
				if err != nil {
					t.Fatal(err)
				}
				mkSymlink(t, rel, filepath.Join(home, ".config/phys"))
			},
			want: map[string]string{".config/phys": StaleReasonOrphaned},
		},
		{
			name: "dangling-orphan-into-repo",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, filepath.Join(repo, "gone"), filepath.Join(home, ".gonerc"))
			},
			want: map[string]string{".gonerc": StaleReasonOrphaned},
		},
		{
			name: "table-dest-dangling-source-deleted",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, filepath.Join(repo, "cfg/foo"), filepath.Join(home, ".config/foo"))
				if err := os.Remove(filepath.Join(repo, "cfg/foo")); err != nil {
					t.Fatal(err)
				}
			},
			want: map[string]string{".config/foo": StaleReasonDangling},
		},
		{
			name: "table-dest-dangling-source-present",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, filepath.Join(repo, "cfg/moved"), filepath.Join(home, ".config/foo"))
			},
		},
		{
			name: "correct-table-link",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, filepath.Join(repo, "bash/bashrc"), filepath.Join(home, ".bashrc"))
			},
		},
		{
			name: "plasma-files-link",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, filepath.Join(repo, "plasma/files/kdeglobals"), filepath.Join(home, ".config/kdeglobals"))
				mkSymlink(t, filepath.Join(repo, "plasma/files/gone"), filepath.Join(home, ".config/kgone"))
			},
		},
		{
			name: "link-to-repo-root",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, repo, filepath.Join(home, ".config/dotfiles"))
				mkSymlink(t, repo+"/", filepath.Join(home, ".dotfiles"))
			},
		},
		{
			name: "foreign-symlink",
			setup: func(t *testing.T, _, home string) {
				writeFile(t, filepath.Join(home, "elsewhere"), []byte("x"))
				mkSymlink(t, filepath.Join(home, "elsewhere"), filepath.Join(home, ".config/foreign"))
				mkSymlink(t, "/nonexistent/dotf-test", filepath.Join(home, ".config/dangling-foreign"))
			},
		},
		{
			name: "real-file",
			setup: func(t *testing.T, _, home string) {
				writeFile(t, filepath.Join(home, ".config/real"), []byte("x"))
			},
		},
		{
			name: "relative-target-into-repo",
			setup: func(t *testing.T, repo, home string) {
				rel, err := filepath.Rel(filepath.Join(home, ".config"), filepath.Join(repo, "cfg/rel"))
				if err != nil {
					t.Fatal(err)
				}
				mkSymlink(t, rel, filepath.Join(home, ".config/rel"))
			},
			want: map[string]string{".config/rel": StaleReasonOrphaned},
		},
		{
			name: "scan-dir-linked-into-repo-not-scanned",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, filepath.Join(repo, "nvim"), filepath.Join(home, ".config/nvim"))
				// A repo-tracked dangling link, visible as ~/.config/nvim/stale.
				mkSymlink(t, filepath.Join(repo, "nvim/missing"), filepath.Join(repo, "nvim/stale"))
			},
		},
		{
			name: "hyprland-facet-link-not-stale",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, filepath.Join(repo, "hypr"), filepath.Join(home, ".config/hypr"))
			},
		},
		{
			name: "copy-dest-legacy-symlink-not-stale",
			setup: func(t *testing.T, repo, home string) {
				mkSymlink(t, filepath.Join(repo, "zed/settings.json"), filepath.Join(home, ".config/zed/settings.json"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			home := t.TempDir()
			buildPruneRepo(t, repo)
			tt.setup(t, repo, home)

			got, err := NewEngine(repo, home).FindStale(pruneTestSpecs())
			if err != nil {
				t.Fatalf("FindStale() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("FindStale() = %+v, want %v", got, tt.want)
			}
			for i, s := range got {
				if reason, ok := tt.want[s.Dest]; !ok || reason != s.Reason {
					t.Errorf("stale %q reason %q, want %v", s.Dest, s.Reason, tt.want)
				}
				if s.DestAbs != filepath.Join(home, filepath.FromSlash(s.Dest)) {
					t.Errorf("DestAbs = %q for %q", s.DestAbs, s.Dest)
				}
				if i > 0 && got[i-1].Dest >= s.Dest {
					t.Errorf("FindStale() not sorted by Dest: %+v", got)
				}
			}
		})
	}
}

func TestFindStale_MissingRepo(t *testing.T) {
	_, err := NewEngine(filepath.Join(t.TempDir(), "absent"), t.TempDir()).FindStale(pruneTestSpecs())
	var missing *MissingSourceTreeError
	if !errors.As(err, &missing) {
		t.Fatalf("FindStale() error = %v, want MissingSourceTreeError", err)
	}
}

func TestPrune(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(t *testing.T, repo, link string)
		wantRemoved bool
		wantLink    bool // link path still a symlink afterwards
	}{
		{
			name:        "removes-unchanged",
			mutate:      func(_ *testing.T, _, _ string) {},
			wantRemoved: true,
		},
		{
			name: "skips-retargeted",
			mutate: func(t *testing.T, repo, link string) {
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				mkSymlink(t, filepath.Join(repo, "cfg/foo"), link)
			},
			wantLink: true,
		},
		{
			name: "skips-replaced-by-real-file",
			mutate: func(t *testing.T, _, link string) {
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				writeFile(t, link, []byte("real"))
			},
		},
		{
			name: "skips-vanished",
			mutate: func(t *testing.T, _, link string) {
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			home := t.TempDir()
			buildPruneRepo(t, repo)
			link := filepath.Join(home, ".config/old")
			mkSymlink(t, filepath.Join(repo, "cfg/old"), link) // dangling: an orphan

			eng := NewEngine(repo, home)
			stale, err := eng.FindStale(pruneTestSpecs())
			if err != nil || len(stale) != 1 {
				t.Fatalf("FindStale() = %+v, %v; want one stale link", stale, err)
			}
			tt.mutate(t, repo, link)

			removed, err := eng.Prune(stale)
			if err != nil {
				t.Fatalf("Prune() error = %v", err)
			}
			if got := len(removed) == 1; got != tt.wantRemoved {
				t.Errorf("Prune() removed = %+v, want removed %t", removed, tt.wantRemoved)
			}
			fi, err := os.Lstat(link)
			switch {
			case tt.wantRemoved && err == nil:
				t.Errorf("link %q still exists", link)
			case tt.wantLink && (err != nil || fi.Mode()&os.ModeSymlink == 0):
				t.Errorf("link %q not kept as symlink (err=%v)", link, err)
			}
			if tt.name == "skips-replaced-by-real-file" && readString(t, link) != "real" {
				t.Errorf("real file at %q was modified", link)
			}
		})
	}
}
