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
	vt, rp, rt := rc.VerifyTimeout, rc.ReadyPoll, rc.ReadyTimeout
	t.Cleanup(func() { rc.VerifyTimeout, rc.ReadyPoll, rc.ReadyTimeout = vt, rp, rt })
	rc.VerifyTimeout, rc.ReadyPoll, rc.ReadyTimeout = 0, time.Millisecond, 20*time.Millisecond
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
