package main

import (
	"encoding/json"
	"errors"
	"flag"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/selfupdate"
)

// stubSelfUpdate swaps the selfUpdateRun seam for a canned Result/error so the
// command layer is exercised without a network. It reassigns a package global,
// so callers must not run in parallel.
func stubSelfUpdate(t *testing.T, res selfupdate.Result, err error) {
	t.Helper()
	orig := selfUpdateRun
	selfUpdateRun = func(_ selfupdate.Options, _ platform.Info) (selfupdate.Result, error) {
		return res, err
	}
	t.Cleanup(func() { selfUpdateRun = orig })
}

func TestSetSelfUpdateFlags_Parses(t *testing.T) {
	fs := flag.NewFlagSet("self-update", flag.ContinueOnError)
	o := setSelfUpdateFlags(fs)
	if err := fs.Parse([]string{"--check", "--dry-run", "--force", "--json"}); err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if !o.check || !o.dryRun || !o.force || !o.jsonOut {
		t.Errorf("flags = %+v, want all true", *o)
	}
}

func TestRunSelfUpdate_KeyValueUpdated(t *testing.T) {
	stubSelfUpdate(t, selfupdate.Result{
		Current: "0.1.0", Latest: "0.2.0", Available: true,
		Asset: "dotf_linux_amd64.tar.gz", Target: "/home/u/.local/bin/dotf", Updated: true,
	}, nil)

	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"self-update"}) })
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr=%q)", code, stderr)
	}
	for _, want := range []string{
		"current=0.1.0\n", "latest=0.2.0\n", "update.available=true\n",
		"asset=dotf_linux_amd64.tar.gz\n", "target=/home/u/.local/bin/dotf\n", "updated=true\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q\n---\n%s", want, stdout)
		}
	}
	// The human "restart" note lands on stderr, not stdout.
	if !strings.Contains(stderr, "updated to 0.2.0") {
		t.Errorf("stderr = %q, want an updated/restart note", stderr)
	}
}

func TestRunSelfUpdate_CheckOmitsUpdated(t *testing.T) {
	stubSelfUpdate(t, selfupdate.Result{
		Current: "0.1.0", Latest: "0.2.0", Available: true, Asset: "dotf_linux_amd64.tar.gz",
	}, nil)

	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"self-update", "--check"}) })
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "checked=true\n") {
		t.Errorf("stdout missing checked=true\n---\n%s", stdout)
	}
	if strings.Contains(stdout, "updated=") {
		t.Errorf("stdout has an updated= line under --check, want none\n---\n%s", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty (no replace happened)", stderr)
	}
}

func TestRunSelfUpdate_DryRunJSON(t *testing.T) {
	stubSelfUpdate(t, selfupdate.Result{
		Current: "0.1.0", Latest: "0.2.0", Available: true,
		Asset: "dotf_linux_amd64.tar.gz", Target: "/opt/dotf",
	}, nil)

	stdout, stderr, code := captureOutput(t, func() int {
		return run([]string{"self-update", "--dry-run", "--json"})
	})
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr=%q)", code, stderr)
	}

	var rep selfUpdateReport
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	if rep.Command != "self-update" {
		t.Errorf("command = %q, want self-update", rep.Command)
	}
	if !rep.DryRun {
		t.Error("dry_run = false, want true")
	}
	if rep.Checked {
		t.Error("checked = true, want false")
	}
	if rep.Current != "0.1.0" || rep.Latest != "0.2.0" || !rep.Available {
		t.Errorf("report = %+v, want current/latest/available populated", rep)
	}
	if rep.Target != "/opt/dotf" {
		t.Errorf("target = %q, want /opt/dotf", rep.Target)
	}
}

func TestRunSelfUpdate_DevBuildExit1(t *testing.T) {
	stubSelfUpdate(t, selfupdate.Result{Current: "dev", Latest: "0.2.0", Available: true}, selfupdate.ErrDevBuild)

	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"self-update"}) })
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "dev build") || !strings.Contains(stderr, "--force") {
		t.Errorf("stderr = %q, want a dev-build --force hint", stderr)
	}
}

func TestRunSelfUpdate_GenericErrorExit1(t *testing.T) {
	stubSelfUpdate(t, selfupdate.Result{}, errors.New("download failed"))

	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"self-update"}) })
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "download failed") {
		t.Errorf("stderr = %q, want the underlying error", stderr)
	}
}

func TestRunSelfUpdate_UnexpectedPositional(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"self-update", "extra"}) })
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "takes no arguments") {
		t.Errorf("stderr = %q, want a no-arguments usage error", stderr)
	}
}

func TestRunSelfUpdate_BadFlag(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"self-update", "--bogus"}) })
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if stderr == "" {
		t.Error("stderr is empty, want a flag-parse error")
	}
}
