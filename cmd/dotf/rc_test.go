package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/rc"
)

// rcRun runs `dotf rc <args...> --config <cfg>` with r and returns code,
// stdout, stderr.
func rcRun(t *testing.T, r exec.Runner, cfg string, stdin io.Reader, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	code := runRc(append(args, "--config", cfg), &out, &errb, stdin, r)
	return code, out.String(), errb.String()
}

func rcTestEnv(t *testing.T) (cfg, repoA, repoB string) {
	t.Helper()
	vt, vs, rp, rt := rc.VerifyTimeout, rc.VerifySettle, rc.ReadyPoll, rc.ReadyTimeout
	t.Cleanup(func() { rc.VerifyTimeout, rc.VerifySettle, rc.ReadyPoll, rc.ReadyTimeout = vt, vs, rp, rt })
	rc.VerifyTimeout, rc.VerifySettle, rc.ReadyPoll, rc.ReadyTimeout = 0, time.Millisecond, time.Millisecond, 20*time.Millisecond

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	repoA, repoB = filepath.Join(home, "src", "alpha"), filepath.Join(home, "src", "beta")
	for _, d := range []string{repoA, repoB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(home, ".config", "dotf", "rc.toml"), repoA, repoB
}

func TestRc_Usage(t *testing.T) {
	cfg, _, _ := rcTestEnv(t)
	for _, args := range [][]string{
		{},
		{"bogus"},
		{"add"},
		{"list", "extra"},
		{"list", "--yes"},
		{"up", "--name", "x"},
		{"status", "--now"},
		{"add", "/a", "/b", "--name", "x"},
	} {
		if code, _, _ := rcRun(t, exec.NewFake(), cfg, nil, args...); code != 2 {
			t.Errorf("rc %v exit = %d, want 2", args, code)
		}
	}
}

func TestRc_AddListRemove(t *testing.T) {
	cfg, a, b := rcTestEnv(t)
	f := exec.NewFake()
	if code, out, errs := rcRun(t, f, cfg, nil, "add", a, b); code != 0 || !strings.Contains(out, "rc.add=alpha") {
		t.Fatalf("add = %d\n%s\n%s", code, out, errs)
	}
	data, _ := os.ReadFile(cfg)
	if !strings.Contains(string(data), "path = '~/src/alpha'") || !strings.Contains(string(data), "DO_NOT_TRACK = '0'") {
		t.Errorf("config not written with ~ paths and defaults:\n%s", data)
	}
	if code, out, _ := rcRun(t, f, cfg, nil, "add", a); code != 0 || !strings.Contains(out, "already-listed") {
		t.Errorf("re-add = %d %s", code, out)
	}
	if code, out, _ := rcRun(t, f, cfg, nil, "add", b+"x", filepath.Join(a, "missing")); code != 1 || out != "" {
		t.Errorf("add with a failing path = %d, stdout %q; want 1 and nothing claimed", code, out)
	}

	code, out, _ := rcRun(t, f, cfg, nil, "list", "--json")
	var doc struct {
		Repos []struct{ Name, Command string } `json:"repos"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &doc) != nil || len(doc.Repos) != 2 {
		t.Fatalf("list --json = %d %s", code, out)
	}
	if !strings.HasPrefix(doc.Repos[0].Command, "DO_NOT_TRACK=0 command claude --allow-dangerously-skip-permissions remote-control --name alpha") {
		t.Errorf("command = %s", doc.Repos[0].Command)
	}

	if code, _, _ := rcRun(t, f, cfg, nil, "remove", "beta", "--dry-run"); code != 0 {
		t.Fatal("remove --dry-run failed")
	}
	if code, out, _ := rcRun(t, f, cfg, nil, "list"); code != 0 || !strings.Contains(out, "rc.repos=2") {
		t.Errorf("dry-run remove changed the config: %s", out)
	}
	if code, _, _ := rcRun(t, f, cfg, nil, "remove", "beta"); code != 0 {
		t.Fatal("remove failed")
	}
	if code, _, _ := rcRun(t, f, cfg, nil, "remove", "beta"); code != 1 {
		t.Error("removing an unknown repo should exit 1")
	}
	if _, out, _ := rcRun(t, f, cfg, nil, "list"); !strings.Contains(out, "rc.repos=1") {
		t.Errorf("after remove: %s", out)
	}
}

func TestRc_UpStartsEachRepo(t *testing.T) {
	cfg, a, b := rcTestEnv(t)
	if code, _, _ := rcRun(t, exec.NewFake(), cfg, nil, "add", a, b); code != 0 {
		t.Fatal("add failed")
	}
	h := rc.NewFakeHerdr()
	// Exit 1: the fake starts no real claude, so verification fails on a
	// procfs host; the Herdr side effects are what this test checks.
	code, out, errs := rcRun(t, h, cfg, nil, "up", "--json")
	if code > 1 {
		t.Fatalf("up = %d\n%s\n%s", code, out, errs)
	}
	var doc struct {
		Repos []struct{ Name, Action, Tab string } `json:"repos"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || len(doc.Repos) != 2 {
		t.Fatalf("up --json = %s", out)
	}
	for _, r := range doc.Repos {
		if r.Action != "start" || r.Tab == "" {
			t.Errorf("repo %s: %+v", r.Name, r)
		}
	}
	if len(h.Workspaces) != 2 || len(h.Typed) != 2 {
		t.Errorf("workspaces=%v typed=%v", h.Workspaces, h.Typed)
	}

	h2 := rc.NewFakeHerdr()
	if code, _, _ := rcRun(t, h2, cfg, nil, "up", "--only", "beta"); code > 1 || len(h2.Workspaces) != 1 || h2.Workspaces[0].Label != "beta" {
		t.Errorf("--only beta: code=%d workspaces=%v", code, h2.Workspaces)
	}
	h3 := rc.NewFakeHerdr()
	if _, out, _ := rcRun(t, h3, cfg, nil, "up", "--only", "beta,beta"); strings.Count(out, "rc.up=beta") != 1 || len(h3.Typed) != 1 {
		t.Errorf("--only beta,beta acted more than once: %s typed=%v", out, h3.Typed)
	}
	if code, _, _ := rcRun(t, h2, cfg, nil, "up", "--only", "nope"); code != 2 {
		t.Errorf("--only unknown exit = %d, want 2", code)
	}
}

func TestRc_UpFailsWhenNoServerAppears(t *testing.T) {
	if _, err := os.Stat("/proc/self/cmdline"); err != nil {
		t.Skip("needs procfs")
	}
	cfg, a, _ := rcTestEnv(t)
	if code, _, _ := rcRun(t, exec.NewFake(), cfg, nil, "add", a); code != 0 {
		t.Fatal("add failed")
	}
	// The fake types the command but no claude process ever starts.
	code, out, _ := rcRun(t, rc.NewFakeHerdr(), cfg, nil, "up")
	if code != 1 || !strings.Contains(out, "verified=false") || !strings.Contains(out, "rc.failed=1") {
		t.Errorf("up with no server appearing = %d\n%s", code, out)
	}
}

func TestRc_UpDryRunTouchesNothing(t *testing.T) {
	cfg, a, _ := rcTestEnv(t)
	if code, _, _ := rcRun(t, exec.NewFake(), cfg, nil, "add", a); code != 0 {
		t.Fatal("add failed")
	}
	h := rc.NewFakeHerdr()
	code, out, _ := rcRun(t, h, cfg, nil, "up", "--dry-run")
	if code != 0 || !strings.Contains(out, "rc.up=alpha action=start") || !strings.Contains(out, "rc.command=alpha DO_NOT_TRACK=0") {
		t.Fatalf("dry-run = %d\n%s", code, out)
	}
	if h.Called("herdr workspace") || h.Called("herdr tab") || h.Called("herdr pane") {
		t.Errorf("dry run called herdr: %v", h.Calls)
	}
}

func TestRc_UpRequiresHerdr(t *testing.T) {
	cfg, a, _ := rcTestEnv(t)
	if code, _, _ := rcRun(t, exec.NewFake(), cfg, nil, "add", a); code != 0 {
		t.Fatal("add failed")
	}
	if code, _, errs := rcRun(t, exec.NewFake(), cfg, nil, "up"); code != 1 || !strings.Contains(errs, "herdr is not installed") {
		t.Errorf("up without herdr = %d %s", code, errs)
	}
}

func TestRc_EnableDisable(t *testing.T) {
	cfg, _, _ := rcTestEnv(t)
	origOS, origExe := rcGOOS, rcExecutable
	t.Cleanup(func() { rcGOOS, rcExecutable = origOS, origExe })
	rcExecutable = func() (string, error) { return "/opt/bin/dotf", nil }

	rcGOOS = "darwin"
	if code, _, errs := rcRun(t, rc.NewFakeHerdr(), cfg, nil, "enable"); code != 1 || !strings.Contains(errs, "systemd") {
		t.Errorf("enable on darwin = %d %s", code, errs)
	}

	rcGOOS = "linux"
	h := rc.NewFakeHerdr()
	h.Paths["systemctl"] = "/usr/bin/systemctl"
	unit := rc.UnitPath(os.Getenv("HOME"))

	if code, out, _ := rcRun(t, h, cfg, nil, "enable", "--dry-run"); code != 0 || !strings.Contains(out, "ExecStart=/opt/bin/dotf rc up --config ") {
		t.Fatalf("enable --dry-run = %d\n%s", code, out)
	}
	if _, err := os.Stat(unit); err == nil {
		t.Fatal("dry run wrote the unit")
	}
	if code, _, errs := rcRun(t, h, cfg, nil, "enable", "--now"); code != 0 {
		t.Fatalf("enable = %d %s", code, errs)
	}
	if !h.Called("systemctl --user daemon-reload") || !h.Called("systemctl --user enable --now dotf-rc.service") {
		t.Errorf("systemctl calls = %v", h.Calls)
	}
	if s, _ := rc.ReadUnitState(unit); s != rc.UnitManaged {
		t.Fatalf("unit state = %v", s)
	}
	if code, _, _ := rcRun(t, h, cfg, nil, "disable"); code != 0 {
		t.Fatal("disable failed")
	}
	if _, err := os.Stat(unit); err == nil {
		t.Fatal("disable left the unit")
	}
	if code, out, _ := rcRun(t, h, cfg, nil, "disable"); code != 0 || !strings.Contains(out, "not-installed") {
		t.Errorf("second disable = %d %s", code, out)
	}

	if err := os.WriteFile(unit, []byte("[Unit]\nDescription=mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := rcRun(t, h, cfg, nil, "enable"); code != 1 {
		t.Error("enable overwrote a foreign unit without --force")
	}
	if code, _, _ := rcRun(t, h, cfg, nil, "disable"); code != 1 {
		t.Error("disable removed a foreign unit without --force")
	}
	if code, _, _ := rcRun(t, h, cfg, nil, "enable", "--force"); code != 0 {
		t.Error("enable --force failed")
	}
}

func TestRc_Down(t *testing.T) {
	cfg, a, b := rcTestEnv(t)
	if code, _, _ := rcRun(t, exec.NewFake(), cfg, nil, "add", a, b); code != 0 {
		t.Fatal("add failed")
	}
	h := rc.NewFakeHerdr()
	_, _, _ = rcRun(t, h, cfg, nil, "up") // creates an rc tab per repo
	// An unrelated workspace with its own rc tab must survive down.
	h.Workspaces = append(h.Workspaces, rc.Workspace{ID: "wX", Label: "other"})
	h.Tabs = append(h.Tabs, rc.Tab{ID: "wX:t1", Label: rc.DefaultTabLabel, WorkspaceID: "wX"})
	h.Panes = append(h.Panes, rc.Pane{ID: "wX:p1", TabID: "wX:t1", WorkspaceID: "wX", Cwd: a}) // even in a repo dir
	rcTabs := func() int {
		n := 0
		for _, tab := range h.Tabs {
			if tab.Label == rc.DefaultTabLabel {
				n++
			}
		}
		return n
	}
	if rcTabs() != 3 {
		t.Fatalf("setup: %d rc tabs, want 3", rcTabs())
	}

	if code, out, _ := rcRun(t, h, cfg, nil, "down", "--dry-run"); code != 0 || !strings.Contains(out, "action=would-stop") || rcTabs() != 3 {
		t.Fatalf("down --dry-run = %d, rc tabs %d\n%s", code, rcTabs(), out)
	}
	if code, _, errs := rcRun(t, h, cfg, nil, "down"); code != 1 || !strings.Contains(errs, "without --yes") || rcTabs() != 3 {
		t.Fatalf("non-interactive down without --yes = %d, rc tabs %d: %s", code, rcTabs(), errs)
	}
	if code, out, _ := rcRun(t, h, cfg, nil, "down", "--only", "beta", "--yes"); code != 0 || rcTabs() != 2 ||
		!strings.Contains(out, "rc.down=beta action=stop") || strings.Contains(out, "alpha") {
		t.Fatalf("down --only beta = %d, rc tabs %d\n%s", code, rcTabs(), out)
	}
	if code, _, _ := rcRun(t, h, cfg, nil, "down", "--yes"); code != 0 || rcTabs() != 1 {
		t.Fatalf("down --yes left %d rc tabs, want only the unrelated one", rcTabs())
	}
	if code, _, errs := rcRun(t, h, cfg, nil, "down", "--yes"); code != 0 || !strings.Contains(errs, "no Remote Control servers or rc tabs") {
		t.Fatalf("second down = %d %s", code, errs)
	}
}

func TestRc_DownConfirm(t *testing.T) {
	cfg, a, _ := rcTestEnv(t)
	if code, _, _ := rcRun(t, exec.NewFake(), cfg, nil, "add", a); code != 0 {
		t.Fatal("add failed")
	}
	orig := rcInteractive
	t.Cleanup(func() { rcInteractive = orig })
	rcInteractive = func(io.Reader, io.Writer) bool { return true }

	h := rc.NewFakeHerdr()
	_, _, _ = rcRun(t, h, cfg, nil, "up")
	if code, out, errs := rcRun(t, h, cfg, strings.NewReader("n\n"), "down"); code != 1 || !strings.Contains(out, "bring down 1 repo(s)?") || !strings.Contains(errs, "not confirmed") {
		t.Fatalf("declined down = %d\n%s\n%s", code, out, errs)
	}
	if h.Called("herdr tab close") {
		t.Fatal("declined down closed a tab")
	}
	if code, _, _ := rcRun(t, h, cfg, strings.NewReader("y\n"), "down"); code != 0 || !h.Called("herdr tab close") {
		t.Fatalf("confirmed down = %d", code)
	}
}

func TestRc_DownWithoutHerdrServer(t *testing.T) {
	cfg, a, _ := rcTestEnv(t)
	if code, _, _ := rcRun(t, exec.NewFake(), cfg, nil, "add", a); code != 0 {
		t.Fatal("add failed")
	}
	h := rc.NewFakeHerdr()
	h.Running = false
	if code, _, _ := rcRun(t, h, cfg, nil, "down", "--yes"); code != 0 || h.Called("herdr workspace") || h.Called("herdr tab") {
		t.Fatalf("down with no herdr server = %d, calls %v", code, h.Calls)
	}
}

func TestRc_Profiles(t *testing.T) {
	cfg, a, b := rcTestEnv(t)
	home := os.Getenv("HOME")
	if code, _, _ := rcRun(t, exec.NewFake(), cfg, nil, "add", a); code != 0 {
		t.Fatal("add failed")
	}
	if code, _, errs := rcRun(t, exec.NewFake(), cfg, nil, "add", b, "--profile", "work"); code != 1 ||
		!strings.Contains(errs, `profile "work" is not defined`) {
		t.Fatalf("add with undefined profile = %d %s", code, errs)
	}
	f, err := os.OpenFile(cfg, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("\n[profile.work]\nconfig_dir = '~/.claude-work'\npermission_mode = 'acceptEdits'\n" +
		"[profile.work.permissions]\nallow = ['Bash(git *)']\n")
	_ = f.Close()
	if code, out, errs := rcRun(t, exec.NewFake(), cfg, nil, "add", b, "--profile", "work"); code != 0 {
		t.Fatalf("add --profile work = %d %s %s", code, out, errs)
	}

	code, out, _ := rcRun(t, exec.NewFake(), cfg, nil, "list")
	if code != 0 || !strings.Contains(out, "rc.repo=alpha path="+a+" profile=claude") ||
		!strings.Contains(out, "rc.repo=beta path="+b+" profile=work") || !strings.Contains(out, "permission_mode=acceptEdits") {
		t.Fatalf("list = %d\n%s", code, out)
	}

	settings := filepath.Join(home, ".claude-work", "settings.json")
	code, out, _ = rcRun(t, rc.NewFakeHerdr(), cfg, nil, "up", "--dry-run")
	if code != 0 || !strings.Contains(out, "rc.profile=work settings="+settings+" permissions=would-update") ||
		!strings.Contains(out, "rc.command=beta CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude-work")+" DO_NOT_TRACK=0 command claude") ||
		!strings.Contains(out, "rc.command=alpha DO_NOT_TRACK=0 command claude") {
		t.Fatalf("up --dry-run = %d\n%s", code, out)
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Fatal("dry run wrote settings.json")
	}

	// --only alpha does not touch the work profile.
	if _, out, _ := rcRun(t, rc.NewFakeHerdr(), cfg, nil, "up", "--only", "alpha"); strings.Contains(out, "rc.profile=") {
		t.Errorf("--only alpha synced a profile:\n%s", out)
	}

	h := rc.NewFakeHerdr()
	if code, out, errs := rcRun(t, h, cfg, nil, "up", "--only", "beta"); code > 1 ||
		!strings.Contains(out, "rc.profile=work settings="+settings+" permissions=updated") {
		t.Fatalf("up = %d\n%s\n%s", code, out, errs)
	}
	data, err := os.ReadFile(settings)
	if err != nil || !strings.Contains(string(data), `"Bash(git *)"`) {
		t.Fatalf("settings.json = %s, %v", data, err)
	}
	var typed []string
	for _, lines := range h.Typed {
		typed = append(typed, lines...)
	}
	if len(typed) != 1 || !strings.Contains(typed[0], "CLAUDE_CONFIG_DIR=") {
		t.Errorf("typed = %v", h.Typed)
	}
	if _, out, _ := rcRun(t, rc.NewFakeHerdr(), cfg, nil, "up", "--only", "beta"); !strings.Contains(out, "permissions=unchanged") {
		t.Errorf("second up:\n%s", out)
	}

	// Broken settings.json: the profile's repos are skipped, nothing is typed.
	if err := os.WriteFile(settings, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	h = rc.NewFakeHerdr()
	code, out, _ = rcRun(t, h, cfg, nil, "up", "--only", "beta")
	if code != 1 || !strings.Contains(out, "permissions=error") || !strings.Contains(out, "rc.up=beta action=skip") || len(h.Typed) != 0 {
		t.Errorf("up with broken settings = %d typed=%v\n%s", code, h.Typed, out)
	}
}
