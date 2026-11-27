package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/symlink"
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

func (f *fakeStepEngines) customScript(_ string, script, _ string, _ platform.Info) error {
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

// TestRealStepEngines_SymlinkBacksUpRealFile pins setup's symlink step to the
// default-on backup: a real ~/.bashrc is copied aside (bytes preserved) before
// it is replaced by the link, and the run's backup dir is recorded for the
// single end-of-run stderr line.
func TestRealStepEngines_SymlinkBacksUpRealFile(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	src := seedRepoFile(t, repoRoot, "bash/bashrc", "# repo bashrc\n")
	seedRepoFile(t, home, ".bashrc", "# user bashrc\n")

	eng := &realStepEngines{repoRoot: repoRoot, home: home}
	if err := eng.symlink("bash/bashrc", platform.Info{OS: platform.OSDebian}); err != nil {
		t.Fatalf("symlink() error = %v", err)
	}

	backups, err := filepath.Glob(filepath.Join(home, ".config", "dotfiles_backup_*"))
	if err != nil {
		t.Fatalf("Glob error = %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("backup dirs = %v, want exactly one", backups)
	}
	got, err := os.ReadFile(filepath.Join(backups[0], ".bashrc"))
	if err != nil {
		t.Fatalf("ReadFile(backup/.bashrc) error = %v", err)
	}
	if string(got) != "# user bashrc\n" {
		t.Errorf("backup content = %q, want the user's original", got)
	}
	target, err := os.Readlink(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("Readlink(.bashrc) error = %v", err)
	}
	if target != src {
		t.Errorf("Readlink(.bashrc) = %q, want %q", target, src)
	}
	if eng.backupDir != backups[0] {
		t.Errorf("eng.backupDir = %q, want %q", eng.backupDir, backups[0])
	}
	var stderr bytes.Buffer
	printSetupBackupDir(&stderr, eng.backupDir)
	if want := "dotf: link backups: " + backups[0] + "\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

// TestRealStepEngines_SymlinkStepsShareOneBackupDir pins AC2: two symlink
// steps over real files in one setup run produce exactly ONE backup dir (no
// _1 sibling) holding both copies and a manifest listing both dests.
func TestRealStepEngines_SymlinkStepsShareOneBackupDir(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	seedRepoFile(t, repoRoot, "bash/bashrc", "# repo bashrc\n")
	seedRepoFile(t, repoRoot, "zsh/zshrc", "# repo zshrc\n")
	seedRepoFile(t, home, ".bashrc", "# user bashrc\n")
	seedRepoFile(t, home, ".zshrc", "# user zshrc\n")

	eng := &realStepEngines{repoRoot: repoRoot, home: home}
	mod := manifest.Module{Name: "two", Steps: []manifest.ModuleStep{
		{Symlink: "bash/bashrc"}, {Symlink: "zsh/zshrc"},
	}}
	for _, res := range runModuleSteps(mod, eng, platform.Info{OS: platform.OSDebian}) {
		if res.status != "ok" {
			t.Fatalf("step %d (%s) = %s: %v", res.index, res.target, res.status, res.err)
		}
	}

	backups, err := filepath.Glob(filepath.Join(home, ".config", "dotfiles_backup_*"))
	if err != nil {
		t.Fatalf("Glob error = %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("backup dirs = %v, want exactly one shared dir", backups)
	}
	for rel, want := range map[string]string{".bashrc": "# user bashrc\n", ".zshrc": "# user zshrc\n"} {
		got, err := os.ReadFile(filepath.Join(backups[0], rel))
		if err != nil || string(got) != want {
			t.Errorf("backup %s = %q (err %v), want %q", rel, got, err, want)
		}
	}
	sets, err := symlink.ListBackups(home)
	if err != nil || len(sets) != 1 {
		t.Fatalf("ListBackups = %+v, %v; want one set", sets, err)
	}
	if sets[0].Legacy {
		t.Fatal("shared backup dir has no manifest")
	}
	var dests []string
	for _, e := range sets[0].Entries {
		dests = append(dests, e.Dest)
	}
	if strings.Join(dests, ",") != ".bashrc,.zshrc" {
		t.Errorf("manifest dests = %v, want [.bashrc .zshrc]", dests)
	}
}

// writeScript creates an executable stub at root/rel.
func writeScript(t *testing.T, root, rel string) string {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

// ranScripts returns the script path of every custom-script invocation
// (`env NON_INTERACTIVE=true bash <script>`) the fake runner recorded.
func ranScripts(t *testing.T, fake *exec.Fake) []string {
	t.Helper()
	var out []string
	for _, c := range fake.CallsSnapshot() {
		if c.Name == "env" && len(c.Args) >= 3 && c.Args[1] == "bash" {
			out = append(out, c.Args[2])
		}
	}
	return out
}

// TestRealStepEngines_OverlayModuleScriptResolvesFromOverlayRoot is the
// YAN-1418 regression: an overlay [[module]] custom-script step resolves
// against the overlay config root, not the repo root. It covers a script that
// exists ONLY in the overlay dir, a same-named script in both places (the
// overlay's runs, never the repo's), a base module that stays repo-rooted, and
// the unchanged no-".." containment applied to the overlay root.
func TestRealStepEngines_OverlayModuleScriptResolvesFromOverlayRoot(t *testing.T) {
	repoRoot := t.TempDir()
	configRoot := t.TempDir()

	repoShared := writeScript(t, repoRoot, "bin/shared.sh")
	overlayOnly := writeScript(t, configRoot, "bin/overlay-only.sh")
	overlayShared := writeScript(t, configRoot, "bin/shared.sh")

	base, err := manifest.Parse(strings.NewReader(`[[module]]
name = "base-mod"
steps = [{ custom-script = "bin/shared.sh" }]
`))
	if err != nil {
		t.Fatalf("Parse(base) error = %v", err)
	}
	fragDir := filepath.Join(configRoot, "packages.d")
	if err := os.MkdirAll(fragDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(fragDir, "mine.toml"), []byte(`[[module]]
name = "user-mod"
steps = [
  { custom-script = "bin/overlay-only.sh" },
  { custom-script = "bin/shared.sh" },
]
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	layers, err := manifest.DiscoverOverlays(configRoot)
	if err != nil {
		t.Fatalf("DiscoverOverlays() error = %v", err)
	}
	m, _, err := manifest.Merge(base, layers...)
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	mods := map[string]manifest.Module{}
	for _, mod := range m.Modules {
		mods[mod.Name] = mod
	}

	run := func(mod manifest.Module) ([]setupStepResult, []string) {
		fake := exec.NewFake()
		eng := &realStepEngines{
			m:        m,
			engine:   backend.Default(fake, nil, repoRoot, nil),
			repoRoot: repoRoot,
		}
		return runModuleSteps(mod, eng, platform.Info{}), ranScripts(t, fake)
	}

	// Overlay module: both steps run from the overlay root.
	results, scripts := run(mods["user-mod"])
	for _, r := range results {
		if r.status != "ok" {
			t.Fatalf("user-mod step %d status = %q (err %v), want ok", r.index, r.status, r.err)
		}
	}
	want := []string{overlayOnly, overlayShared}
	if strings.Join(scripts, "\n") != strings.Join(want, "\n") {
		t.Errorf("user-mod ran %v, want %v (overlay root, never the repo's %s)", scripts, want, repoShared)
	}

	// Base module: stays repo-rooted.
	results, scripts = run(mods["base-mod"])
	if len(results) != 1 || results[0].status != "ok" {
		t.Fatalf("base-mod results = %+v, want one ok step", results)
	}
	if len(scripts) != 1 || scripts[0] != repoShared {
		t.Errorf("base-mod ran %v, want [%s] (repo root)", scripts, repoShared)
	}

	// Containment is unchanged under the overlay root: a parent escape and an
	// absolute path still fail and run nothing.
	escape := manifest.Module{Name: "escape", Steps: []manifest.ModuleStep{
		{CustomScript: "../outside.sh", ScriptRoot: configRoot},
		{CustomScript: overlayOnly, ScriptRoot: configRoot},
	}}
	results, scripts = run(escape)
	for _, r := range results {
		if r.status != "failed" {
			t.Errorf("escape step %d status = %q, want failed", r.index, r.status)
		}
	}
	if len(scripts) != 0 {
		t.Errorf("escape ran %v, want nothing", scripts)
	}
}
