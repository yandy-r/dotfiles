package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

const testModuleManifest = `
[[module]]
name = "fingerprint"
description = "PAM fingerprint auth"
steps = [{ custom-script = "bin/setup-fingerprint.sh" }]

[[module]]
name = "onedrive"
description = "OneDrive/SharePoint client setup"
steps = [{ custom-script = "bin/onedrive-main-setup.sh" }]
`

// fakeStepEngines records the (kind, target) of each dispatched step in order,
// optionally failing specific keys, so a test can assert steps run in
// declaration order and that a failure does not abort later steps.
type fakeStepEngines struct {
	calls []string
	fail  map[string]error
}

func (f *fakeStepEngines) record(kind, target string) error {
	key := kind + ":" + target
	f.calls = append(f.calls, key)
	if f.fail != nil {
		return f.fail[key]
	}
	return nil
}

func (f *fakeStepEngines) customScript(_ string, script string, _ platform.Info) error {
	return f.record("custom-script", script)
}
func (f *fakeStepEngines) pkg(name string, _ platform.Info) error { return f.record("pkg", name) }
func (f *fakeStepEngines) symlink(src string, _ platform.Info) error {
	return f.record("symlink", src)
}
func (f *fakeStepEngines) sysconfig(domain string, _ []string, _ platform.Info) error {
	return f.record("sysconfig", domain)
}

func TestRunSetup_MissingDomain(t *testing.T) {
	path := writeTestManifest(t, testModuleManifest)
	var stdout, stderr bytes.Buffer
	code := runSetup([]string{"--manifest", path, "--no-user-config"}, &stdout, &stderr, exec.NewFake())

	if code != 2 {
		t.Fatalf("runSetup() code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	out := stderr.String()
	if !strings.Contains(out, "requires a module name") {
		t.Errorf("stderr = %q, want it to name the missing module", out)
	}
	for _, name := range []string{"fingerprint", "onedrive"} {
		if !strings.Contains(out, name) {
			t.Errorf("stderr = %q, want it to list available module %q", out, name)
		}
	}
}

func TestRunSetup_UnknownDomain(t *testing.T) {
	path := writeTestManifest(t, testModuleManifest)
	var stdout, stderr bytes.Buffer
	code := runSetup([]string{"--manifest", path, "--no-user-config", "bogus"}, &stdout, &stderr, exec.NewFake())

	if code != 2 {
		t.Fatalf("runSetup() code = %d, want 2", code)
	}
	out := stderr.String()
	if !strings.Contains(out, `unknown module "bogus"`) {
		t.Errorf("stderr = %q, want unknown-module message", out)
	}
	if !strings.Contains(out, "fingerprint") || !strings.Contains(out, "onedrive") {
		t.Errorf("stderr = %q, want it to list available modules", out)
	}
}

func TestRunSetup_TooManyDomains(t *testing.T) {
	path := writeTestManifest(t, testModuleManifest)
	var stdout, stderr bytes.Buffer
	code := runSetup([]string{"--manifest", path, "--no-user-config", "fingerprint", "onedrive"}, &stdout, &stderr, exec.NewFake())

	if code != 2 {
		t.Fatalf("runSetup() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "exactly one module name") {
		t.Errorf("stderr = %q, want too-many-modules message", stderr.String())
	}
}

func TestRunSetup_DryRun(t *testing.T) {
	path := writeTestManifest(t, testModuleManifest)
	f := exec.NewFake()
	var stdout, stderr bytes.Buffer
	code := runSetup([]string{"--manifest", path, "--no-user-config", "fingerprint", "--dry-run"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runSetup() code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"dry-run=true",
		"module=fingerprint",
		"setup.step.1=custom-script:bin/setup-fingerprint.sh",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want substring %q", out, want)
		}
	}

	// A dry run must never prime sudo.
	for _, call := range f.CallsSnapshot() {
		if call.Name == "sudo" {
			t.Errorf("dry-run invoked sudo (%+v), want no sudo calls", call)
		}
	}
}

// TestRunSetup_DryRunOrderIndependent proves the module positional and flags may
// appear in either order (loop-parse), mirroring install/completion.
func TestRunSetup_DryRunOrderIndependent(t *testing.T) {
	path := writeTestManifest(t, testModuleManifest)
	for _, args := range [][]string{
		{"--manifest", path, "--no-user-config", "onedrive", "--dry-run"},
		{"--manifest", path, "--dry-run", "--no-user-config", "onedrive"},
	} {
		var stdout, stderr bytes.Buffer
		code := runSetup(args, &stdout, &stderr, exec.NewFake())
		if code != 0 {
			t.Fatalf("runSetup(%v) code = %d, want 0 (stderr=%q)", args, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "module=onedrive") {
			t.Errorf("runSetup(%v) stdout = %q, want module=onedrive", args, stdout.String())
		}
	}
}

func TestRunModuleSteps_ExecutesInOrder(t *testing.T) {
	mod := manifest.Module{Name: "m", Steps: []manifest.ModuleStep{
		{CustomScript: "bin/a.sh"},
		{Pkg: "git"},
		{Symlink: "bash/bashrc"},
		{Sysconfig: "firewall", Ports: []string{"22/tcp"}},
	}}
	fake := &fakeStepEngines{}

	results := runModuleSteps(mod, fake, platform.Info{})

	wantCalls := []string{"custom-script:bin/a.sh", "pkg:git", "symlink:bash/bashrc", "sysconfig:firewall"}
	if !equalStringSlices(fake.calls, wantCalls) {
		t.Errorf("dispatch order = %v, want %v", fake.calls, wantCalls)
	}
	if len(results) != 4 {
		t.Fatalf("results = %d, want 4", len(results))
	}
	for i, res := range results {
		if res.status != "ok" {
			t.Errorf("step %d status = %q, want ok", i+1, res.status)
		}
		if res.index != i+1 {
			t.Errorf("step %d index = %d, want %d", i, res.index, i+1)
		}
	}
	if setupExitCode(results) != 0 {
		t.Errorf("setupExitCode = %d, want 0 (all converged)", setupExitCode(results))
	}
}

func TestRunModuleSteps_FailureIsNonFatal(t *testing.T) {
	mod := manifest.Module{Name: "m", Steps: []manifest.ModuleStep{
		{CustomScript: "bin/a.sh"},
		{Pkg: "git"},
		{Symlink: "bash/bashrc"},
	}}
	fake := &fakeStepEngines{fail: map[string]error{"pkg:git": errors.New("boom")}}

	results := runModuleSteps(mod, fake, platform.Info{})

	// The failing step does not abort the loop: the third step still ran.
	wantCalls := []string{"custom-script:bin/a.sh", "pkg:git", "symlink:bash/bashrc"}
	if !equalStringSlices(fake.calls, wantCalls) {
		t.Errorf("dispatch order = %v, want %v (later step must still run)", fake.calls, wantCalls)
	}
	if results[0].status != "ok" || results[2].status != "ok" {
		t.Errorf("non-failing steps status = %q/%q, want ok/ok", results[0].status, results[2].status)
	}
	if results[1].status != "failed" || results[1].err == nil {
		t.Errorf("failing step = %+v, want status failed with err", results[1])
	}
	if setupExitCode(results) != 1 {
		t.Errorf("setupExitCode = %d, want 1 (a step failed)", setupExitCode(results))
	}
}
