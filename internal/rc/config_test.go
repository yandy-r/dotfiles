package rc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_MissingFileIsEmpty(t *testing.T) {
	cfg, exists, err := Load(filepath.Join(t.TempDir(), "rc.toml"))
	if err != nil || exists || len(cfg.Repos) != 0 {
		t.Fatalf("Load(missing) = %+v, %v, %v; want empty, false, nil", cfg, exists, err)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "rc.toml")
	cfg := NewConfig()
	cfg.Repos = []Repo{{Path: "~/a"}, {Path: "/x/b", Name: "bee", PermissionMode: "plan", Spawn: "same-dir"}}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "# dotf rc:") {
		t.Errorf("saved file lacks header:\n%s", data)
	}
	got, exists, err := Load(path)
	if err != nil || !exists {
		t.Fatalf("Load = %v, %v", exists, err)
	}
	if len(got.Repos) != 2 || got.Repos[1].Name != "bee" || got.Repos[1].Spawn != "same-dir" || got.Env["DO_NOT_TRACK"] != "0" {
		t.Errorf("round trip lost data: %+v", got)
	}
}

func TestLoad_Rejects(t *testing.T) {
	cases := map[string]string{
		"unknown key":     "bogus = 1\n",
		"bad mode":        "permission_mode = 'yolo'\n",
		"bad repo spawn":  "[[repo]]\npath = '/a'\nspawn = 'fork'\n",
		"missing path":    "[[repo]]\nname = 'a'\n",
		"duplicate names": "[[repo]]\npath = '/x/app'\n[[repo]]\npath = '/y/app'\n",
		"bad env key":     "[env]\n'1X' = 'y'\n",
		"bad name":        "[[repo]]\npath = '/a'\nname = 'has space'\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rc.toml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := Load(path); err == nil {
				t.Errorf("Load(%q) succeeded; want error", body)
			}
		})
	}
}

func TestAddRemove(t *testing.T) {
	home := "/home/u"
	cfg := NewConfig()
	if ok, err := cfg.Add(Repo{Path: "~/p/app"}, home); !ok || err != nil {
		t.Fatalf("first Add = %v, %v", ok, err)
	}
	if ok, err := cfg.Add(Repo{Path: "/home/u/p/app"}, home); ok || err != nil {
		t.Fatalf("duplicate Add (expanded) = %v, %v; want false, nil", ok, err)
	}
	if _, err := cfg.Add(Repo{Path: "/other/app"}, home); err == nil {
		t.Fatal("Add with a clashing derived name succeeded")
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("failed Add was not rolled back: %+v", cfg.Repos)
	}
	if ok, _ := cfg.Add(Repo{Path: "/other/app", Name: "app2"}, home); !ok {
		t.Fatal("Add with explicit name failed")
	}
	if r, ok, _ := cfg.Remove("app2", home); !ok || r.Path != "/other/app" {
		t.Fatalf("Remove by name = %+v, %v", r, ok)
	}
	if _, ok, _ := cfg.Remove("/home/u/p/app", home); !ok {
		t.Fatal("Remove by expanded path failed")
	}
	if _, ok, _ := cfg.Remove("nope", home); ok {
		t.Fatal("Remove of unknown key matched")
	}
}

func TestResolve_DefaultsAndOverrides(t *testing.T) {
	f := false
	cfg := &Config{AllowBypass: &f, Repos: []Repo{{Path: "~/a"}, {Path: "/b", PermissionMode: "plan", Spawn: "session"}}}
	got, err := cfg.Resolve("/home/u")
	if err != nil {
		t.Fatal(err)
	}
	a, b := got[0], got[1]
	if a.Path != "/home/u/a" || a.Name != "a" || a.PermissionMode != DefaultPermissionMode || a.Spawn != DefaultSpawn ||
		a.Claude != DefaultClaude || a.AllowBypass {
		t.Errorf("defaults not applied: %+v", a)
	}
	if b.PermissionMode != "plan" || b.Spawn != "session" {
		t.Errorf("overrides not applied: %+v", b)
	}
	unset, _ := (&Config{Repos: []Repo{{Path: "/z"}}}).Resolve("/h")
	if !unset[0].AllowBypass {
		t.Error("unset allow_bypass should default to true")
	}
}

func TestResolve_SymlinksAndClaudeTilde(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "data", "app")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "data"), filepath.Join(root, "code")); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(realDir)
	got, err := (&Config{Claude: "~/.local/bin/claude", Repos: []Repo{{Path: filepath.Join(root, "code", "app")}}}).Resolve("/home/u")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Path != want {
		t.Errorf("Path = %s, want symlink-resolved %s", got[0].Path, want)
	}
	if got[0].Claude != "/home/u/.local/bin/claude" {
		t.Errorf("Claude = %s, want ~ expanded", got[0].Claude)
	}
}

func TestDefaultPath_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := DefaultPath("/home/u"); got != "/xdg/dotf/rc.toml" {
		t.Errorf("DefaultPath = %s", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "relative")
	if got := DefaultPath("/home/u"); got != "/home/u/.config/dotf/rc.toml" {
		t.Errorf("DefaultPath with relative XDG = %s", got)
	}
}
