package rc

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func loadString(t *testing.T, body string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rc.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path)
	return cfg, err
}

func TestProfile_Rejects(t *testing.T) {
	cases := map[string]string{
		"reserved name":        "[profile.claude]\nconfig_dir = '~/.c'\n",
		"bad name":             "[profile.'a b']\nconfig_dir = '~/.c'\n",
		"missing config_dir":   "[profile.work]\npermission_mode = 'auto'\n",
		"relative config_dir":  "[profile.work]\nconfig_dir = 'rel/dir'\n",
		"duplicate config_dir": "[profile.a]\nconfig_dir = '~/.c'\n[profile.b]\nconfig_dir = '~/.c/'\n",
		"bad mode":             "[profile.work]\nconfig_dir = '~/.c'\npermission_mode = 'yolo'\n",
		"bad spawn":            "[profile.work]\nconfig_dir = '~/.c'\nspawn = 'fork'\n",
		"env sets config dir":  "[profile.work]\nconfig_dir = '~/.c'\nenv = { CLAUDE_CONFIG_DIR = '/x' }\n",
		"top env config dir":   "[env]\nCLAUDE_CONFIG_DIR = '/x'\n",
		"bad env key":          "[profile.work]\nconfig_dir = '~/.c'\nenv = { '1X' = 'y' }\n",
		"empty rule":           "[profile.work]\nconfig_dir = '~/.c'\n[profile.work.permissions]\nallow = ['']\n",
		"unknown perm key":     "[profile.work]\nconfig_dir = '~/.c'\n[profile.work.permissions]\nnope = ['x']\n",
		"unknown profile":      "[[repo]]\npath = '/a'\nprofile = 'work'\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadString(t, body); err == nil {
				t.Errorf("Load(%q) succeeded; want error", body)
			}
		})
	}
}

func TestProfile_ResolvePrecedence(t *testing.T) {
	cfg, err := loadString(t, `
permission_mode = 'auto'
spawn = 'worktree'
allow_bypass = true
[env]
DO_NOT_TRACK = '0'
SHARED = 'top'

[profile.work]
config_dir = '~/.claude-work'
permission_mode = 'acceptEdits'
allow_bypass = false
env = { SHARED = 'work' }
[profile.work.permissions]
allow = ['Bash(git *)']

[[repo]]
path = '/x/default'
[[repo]]
path = '/x/work'
profile = 'work'
[[repo]]
path = '/x/override'
profile = 'work'
permission_mode = 'plan'
spawn = 'same-dir'
[[repo]]
path = '/x/explicit'
profile = 'claude'
`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := cfg.Resolve("/home/u")
	if err != nil {
		t.Fatal(err)
	}
	def, work, over, expl := got[0], got[1], got[2], got[3]

	if def.Profile != DefaultProfile || def.ConfigDir != "" || def.PermissionMode != "auto" || !def.AllowBypass ||
		def.Permissions != nil || def.Env["SHARED"] != "top" {
		t.Errorf("default repo = %+v", def)
	}
	if _, ok := def.Env["CLAUDE_CONFIG_DIR"]; ok {
		t.Errorf("default repo sets CLAUDE_CONFIG_DIR: %v", def.Env)
	}
	if expl.Profile != DefaultProfile || expl.ConfigDir != "" {
		t.Errorf("explicit claude profile = %+v", expl)
	}
	if work.Profile != "work" || work.ConfigDir != "/home/u/.claude-work" || work.PermissionMode != "acceptEdits" ||
		work.AllowBypass || work.Spawn != "worktree" {
		t.Errorf("work repo = %+v", work)
	}
	if work.Env["SHARED"] != "work" || work.Env["DO_NOT_TRACK"] != "0" || work.Env["CLAUDE_CONFIG_DIR"] != "/home/u/.claude-work" {
		t.Errorf("work env = %v", work.Env)
	}
	if work.Permissions == nil || work.Permissions.Allow == nil || !slices.Equal(*work.Permissions.Allow, []string{"Bash(git *)"}) {
		t.Errorf("work permissions = %+v", work.Permissions)
	}
	if over.PermissionMode != "plan" || over.Spawn != "same-dir" {
		t.Errorf("override repo = %+v", over)
	}
	// The top-level env map must not be mutated by the profile overlay.
	if cfg.Env["SHARED"] != "top" || cfg.Env["CLAUDE_CONFIG_DIR"] != "" {
		t.Errorf("top-level env mutated: %v", cfg.Env)
	}
}

func TestProfile_RoundTripKeepsOmittedVsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rc.toml")
	empty := []string{}
	allow := []string{"Edit"}
	cfg := NewConfig()
	cfg.Profiles = map[string]Profile{"work": {ConfigDir: "~/.cw", Permissions: &Permissions{Allow: &allow, Ask: &empty}}}
	cfg.Repos = []Repo{{Path: "/a", Profile: "work"}}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, _, err := Load(path)
	if err != nil {
		data, _ := os.ReadFile(path)
		t.Fatalf("Load: %v\n%s", err, data)
	}
	p := got.Profiles["work"].Permissions
	if p == nil || p.Allow == nil || p.Ask == nil || len(*p.Ask) != 0 || p.Deny != nil {
		data, _ := os.ReadFile(path)
		t.Errorf("permissions after round trip = %+v\n%s", p, data)
	}
	if got.Repos[0].Profile != "work" {
		t.Errorf("repo profile lost: %+v", got.Repos[0])
	}
}

func TestShellLine_Profile(t *testing.T) {
	r := Resolved{Name: "api", Path: "/x/api", Profile: "work", PermissionMode: "auto", Spawn: "worktree",
		Claude: "claude", Env: map[string]string{"CLAUDE_CONFIG_DIR": "/home/u/.claude-work", "DO_NOT_TRACK": "0"}}
	got := r.ShellLine()
	want := "CLAUDE_CONFIG_DIR=/home/u/.claude-work DO_NOT_TRACK=0 command claude remote-control --name api --permission-mode auto --spawn worktree"
	if got != want {
		t.Errorf("ShellLine =\n %s\nwant\n %s", got, want)
	}
	if strings.Contains(got, "--settings") {
		t.Error("permissions must not be passed on the command line")
	}
}
