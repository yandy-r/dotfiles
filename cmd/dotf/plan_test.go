package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/tui"
)

func testPlanManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Packages: []manifest.Package{
			{Name: "git", Backend: manifest.BackendPkg},
			{Name: "starship", Backend: manifest.BackendScriptURL, URL: "https://example.com/install.sh", SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			{Name: "eza", Backend: manifest.BackendGitHubRelease, Repo: "eza-community/eza", Asset: "eza.tar.gz", SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			{Name: "rust", Backend: manifest.BackendToolchain, Tool: "rust"},
		},
	}
}

func TestSortedBackendKeys(t *testing.T) {
	groups := map[string][]string{
		"toolchain":  {"rust"},
		"pkg":        {"git"},
		"script-url": {"starship"},
	}

	got := sortedBackendKeys(groups)
	want := []string{"pkg", "script-url", "toolchain"}

	if len(got) != len(want) {
		t.Fatalf("sortedBackendKeys() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sortedBackendKeys()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGroupPackagesByBackend(t *testing.T) {
	m := testPlanManifest()

	got := groupPackagesByBackend(m, []string{"git", "starship", "rust", "missing"})
	want := map[string][]string{
		"pkg":        {"git"},
		"script-url": {"starship"},
		"toolchain":  {"rust"},
	}

	if len(got) != len(want) {
		t.Fatalf("groupPackagesByBackend() = %v, want %v", got, want)
	}
	for backend, names := range want {
		if !strings.EqualFold(strings.Join(got[backend], ","), strings.Join(names, ",")) {
			t.Errorf("groupPackagesByBackend()[%q] = %v, want %v", backend, got[backend], names)
		}
	}
	if _, ok := got[""]; ok {
		t.Error("groupPackagesByBackend() produced empty backend key for missing package")
	}
}

func TestPrintPlanPreview(t *testing.T) {
	m := testPlanManifest()
	sel := tui.Selection{
		OS:       platform.OSArch,
		Intents:  []string{"base", "dev"},
		Packages: []string{"git", "starship", "rust"},
	}

	var buf bytes.Buffer
	if err := printPlanPreview(&buf, m, sel); err != nil {
		t.Fatalf("printPlanPreview() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	wantPrefix := []string{
		"os=arch",
		"intents=base,dev",
		"packages=git,starship,rust",
	}
	for i, prefix := range wantPrefix {
		if !strings.HasPrefix(lines[i], prefix) {
			t.Errorf("line[%d] = %q, want prefix %q", i, lines[i], prefix)
		}
	}

	backendLines := lines[len(wantPrefix):]
	if len(backendLines) != 3 {
		t.Fatalf("backend preview lines = %d, want 3", len(backendLines))
	}
	if backendLines[0] != "plan.pkg=git" {
		t.Errorf("first backend line = %q, want plan.pkg=git", backendLines[0])
	}
	if backendLines[1] != "plan.script-url=starship" {
		t.Errorf("second backend line = %q, want plan.script-url=starship", backendLines[1])
	}
	if backendLines[2] != "plan.toolchain=rust" {
		t.Errorf("third backend line = %q, want plan.toolchain=rust", backendLines[2])
	}
}

// TestWritePlanJSON drives the --json plan emission path with a confirmed
// selection (the same headless approach as TestPrintPlanPreview, which
// constructs a tui.Selection directly). It asserts the stable shape: command
// "plan", each selected package resolved to its manifest backend with
// action=install, applicable=true, and no installed state (plan probes nothing).
func TestWritePlanJSON(t *testing.T) {
	m := testPlanManifest()
	sel := tui.Selection{
		OS:       platform.OSArch,
		Intents:  []string{"base", "dev"},
		Packages: []string{"git", "starship", "rust", "missing"},
	}

	var buf bytes.Buffer
	if err := writePlanJSON(&buf, m, sel); err != nil {
		t.Fatalf("writePlanJSON() error = %v", err)
	}

	var rep commandReport
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal plan JSON: %v\nstdout = %q", err, buf.String())
	}
	if rep.Command != "plan" {
		t.Errorf("command = %q, want plan", rep.Command)
	}

	// "missing" is not in the manifest, so it is skipped just as
	// groupPackagesByBackend skips it.
	if len(rep.Packages) != 3 {
		t.Fatalf("packages = %d, want 3 (missing skipped):\n%+v", len(rep.Packages), rep.Packages)
	}

	wantBackend := map[string]string{
		"git":      "pkg",
		"starship": "script-url",
		"rust":     "toolchain",
	}
	// Effective class per package: git=pkg→managed, starship=script-url+sha256→
	// pinned, rust=toolchain→managed. Plan rows carry the effective class (#48).
	wantClass := map[string]string{
		"git":      "managed",
		"starship": "pinned",
		"rust":     "managed",
	}
	for _, p := range rep.Packages {
		if want, ok := wantBackend[p.Name]; !ok || p.Backend != want {
			t.Errorf("package %+v, want backend %q", p, want)
		}
		if p.Class != wantClass[p.Name] {
			t.Errorf("package %q class = %q, want %q", p.Name, p.Class, wantClass[p.Name])
		}
		if !p.Applicable {
			t.Errorf("package %q applicable = false, want true", p.Name)
		}
		if p.Action != "install" {
			t.Errorf("package %q action = %q, want install", p.Name, p.Action)
		}
		if p.Installed != nil {
			t.Errorf("package %q installed = %v, want nil (plan probes nothing)", p.Name, p.Installed)
		}
	}
}
