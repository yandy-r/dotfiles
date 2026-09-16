package backend

import (
	"errors"
	"fmt"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestToolchain_Install_Cargo(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	b := newToolchainBackend(fake)
	pkg := manifest.Package{
		Name:    "ripgrep",
		Backend: manifest.BackendToolchain,
		Tool:    "cargo",
	}
	info := platform.Info{}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("Install() made %d calls, want 1", len(calls))
	}
	want := exec.FakeCall{Name: "cargo", Args: []string{"install", "ripgrep"}}
	if calls[0].Name != want.Name || !stringSliceEqual(calls[0].Args, want.Args) {
		t.Errorf("Install() call = %+v, want %+v", calls[0], want)
	}
	for _, call := range calls {
		if call.Name == "sudo" {
			t.Fatalf("Install() invoked sudo, want user-only install")
		}
	}
}

func TestToolchain_Install_Npm(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	b := newToolchainBackend(fake)
	pkg := manifest.Package{
		Name:    "typescript",
		Backend: manifest.BackendToolchain,
		Tool:    "npm",
	}
	info := platform.Info{}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("Install() made %d calls, want 1", len(calls))
	}
	want := exec.FakeCall{Name: "npm", Args: []string{"install", "-g", "typescript"}}
	if calls[0].Name != want.Name || !stringSliceEqual(calls[0].Args, want.Args) {
		t.Errorf("Install() call = %+v, want %+v", calls[0], want)
	}
}

func TestToolchain_Install_UsesNamesOverride(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	b := newToolchainBackend(fake)
	pkg := manifest.Package{
		Name:    "rg",
		Backend: manifest.BackendToolchain,
		Tool:    "cargo",
		Names:   map[string]string{"cargo": "ripgrep"},
	}
	info := platform.Info{}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := fake.CallsSnapshot()
	want := exec.FakeCall{Name: "cargo", Args: []string{"install", "ripgrep"}}
	if calls[0].Name != want.Name || !stringSliceEqual(calls[0].Args, want.Args) {
		t.Errorf("Install() call = %+v, want %+v", calls[0], want)
	}
}

func TestToolchain_Install_UnknownTool_ReturnsTypedError(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	b := newToolchainBackend(fake)
	pkg := manifest.Package{
		Name:    "example",
		Backend: manifest.BackendToolchain,
		Tool:    "pip",
	}
	info := platform.Info{}

	err := b.Install(pkg, info)
	if err == nil {
		t.Fatal("Install() error = nil, want *UnknownToolError")
	}
	var unknownErr *UnknownToolError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("Install() error = %T, want *UnknownToolError", err)
	}
	if unknownErr.Tool != "pip" {
		t.Errorf("UnknownToolError.Tool = %q, want %q", unknownErr.Tool, "pip")
	}
	if len(fake.CallsSnapshot()) != 0 {
		t.Errorf("Install() made %d calls for unknown tool, want 0", len(fake.CallsSnapshot()))
	}
}

func TestToolchain_Update_Argv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tool string
		want exec.FakeCall
	}{
		{"cargo", exec.FakeCall{Name: "cargo", Args: []string{"install", "--force", "ripgrep"}}},
		{"npm", exec.FakeCall{Name: "npm", Args: []string{"update", "-g", "ripgrep"}}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			t.Parallel()

			fake := exec.NewFake()
			b := newToolchainBackend(fake).(*toolchainBackend)
			pkg := manifest.Package{Name: "ripgrep", Backend: manifest.BackendToolchain, Tool: tt.tool}

			if err := b.Update(pkg, platform.Info{}); err != nil {
				t.Fatalf("Update() error = %v, want nil", err)
			}
			calls := fake.CallsSnapshot()
			if len(calls) != 1 {
				t.Fatalf("Update() made %d calls, want 1", len(calls))
			}
			if calls[0].Name != tt.want.Name || !stringSliceEqual(calls[0].Args, tt.want.Args) {
				t.Errorf("Update() call = %+v, want %+v", calls[0], tt.want)
			}
			for _, c := range calls {
				if c.Name == "sudo" {
					t.Fatal("Update() invoked sudo, want user-only")
				}
			}
		})
	}
}

func TestToolchain_Remove_Argv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tool string
		want exec.FakeCall
	}{
		{"cargo", exec.FakeCall{Name: "cargo", Args: []string{"uninstall", "ripgrep"}}},
		{"npm", exec.FakeCall{Name: "npm", Args: []string{"uninstall", "-g", "ripgrep"}}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			t.Parallel()

			fake := exec.NewFake()
			b := newToolchainBackend(fake).(*toolchainBackend)
			pkg := manifest.Package{Name: "ripgrep", Backend: manifest.BackendToolchain, Tool: tt.tool}

			if err := b.Remove(pkg, platform.Info{}); err != nil {
				t.Fatalf("Remove() error = %v, want nil", err)
			}
			calls := fake.CallsSnapshot()
			if len(calls) != 1 {
				t.Fatalf("Remove() made %d calls, want 1", len(calls))
			}
			if calls[0].Name != tt.want.Name || !stringSliceEqual(calls[0].Args, tt.want.Args) {
				t.Errorf("Remove() call = %+v, want %+v", calls[0], tt.want)
			}
		})
	}
}

// TestToolchain_Install_ReshimsWhenMisePresent pins the shim-regeneration step:
// when mise provides the toolchain, `npm install -g` lands the binary in the
// node version's own bin dir, and only `mise reshim` puts it on PATH. Skipping
// it leaves a package that installed successfully yet never resolves.
func TestToolchain_Install_ReshimsWhenMisePresent(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	fake.AddPath("mise", "/home/u/.local/bin/mise")
	b := newToolchainBackend(fake)
	pkg := manifest.Package{
		Name:    "lefthook",
		Backend: manifest.BackendToolchain,
		Tool:    "npm",
	}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := fake.CallsSnapshot()
	if len(calls) != 2 {
		t.Fatalf("Install() made %d calls, want 2 (npm install, mise reshim)", len(calls))
	}
	want := exec.FakeCall{Name: "mise", Args: []string{"reshim"}}
	if calls[1].Name != want.Name || !stringSliceEqual(calls[1].Args, want.Args) {
		t.Errorf("Install() second call = %+v, want %+v", calls[1], want)
	}
}

// TestToolchain_Install_NoReshimWhenMiseAbsent asserts the reshim is skipped
// entirely on a host without mise — there are no shims to regenerate.
func TestToolchain_Install_NoReshimWhenMiseAbsent(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	b := newToolchainBackend(fake)
	pkg := manifest.Package{
		Name:    "lefthook",
		Backend: manifest.BackendToolchain,
		Tool:    "npm",
	}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	for _, call := range fake.CallsSnapshot() {
		if call.Name == "mise" {
			t.Errorf("Install() ran %+v with no mise on PATH, want no reshim", call)
		}
	}
}

func TestToolchain_Installed_LookPathFound(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	fake.AddPath("ripgrep", "/home/user/.cargo/bin/ripgrep")
	b := newToolchainBackend(fake)
	pkg := manifest.Package{
		Name:    "ripgrep",
		Backend: manifest.BackendToolchain,
		Tool:    "cargo",
	}
	info := platform.Info{}

	installed, err := b.Installed(pkg, info)
	if err != nil {
		t.Fatalf("Installed() error = %v, want nil", err)
	}
	if !installed {
		t.Error("Installed() = false, want true")
	}
}

func TestToolchain_Installed_LookPathMissing(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	b := newToolchainBackend(fake)
	pkg := manifest.Package{
		Name:    "ripgrep",
		Backend: manifest.BackendToolchain,
		Tool:    "cargo",
	}
	info := platform.Info{}

	installed, err := b.Installed(pkg, info)
	if err != nil {
		t.Fatalf("Installed() error = %v, want nil", err)
	}
	if installed {
		t.Error("Installed() = true, want false")
	}
}

func TestToolchain_Installed_UsesPackageNameForBinary(t *testing.T) {
	t.Parallel()

	// The names override carries the crate/npm install identifier, which can
	// differ from the installed binary (crate "ripgrep" installs binary
	// "rg"). Installed must probe PATH for pkg.Name, not the install name.
	fake := exec.NewFake()
	fake.AddPath("rg", "/home/user/.cargo/bin/rg")
	b := newToolchainBackend(fake)
	pkg := manifest.Package{
		Name:    "rg",
		Backend: manifest.BackendToolchain,
		Tool:    "cargo",
		Names:   map[string]string{"cargo": "ripgrep"},
	}
	info := platform.Info{}

	installed, err := b.Installed(pkg, info)
	if err != nil {
		t.Fatalf("Installed() error = %v, want nil", err)
	}
	if !installed {
		t.Error("Installed() = false, want true when the binary (pkg.Name) is on PATH")
	}
}

func TestToolchainInstallArgs_CoversManifestTools(t *testing.T) {
	t.Parallel()

	// manifest validation and this backend must agree on the supported tool
	// set: every tool the manifest accepts has to be executable here.
	for _, tool := range manifest.ToolchainTools() {
		if _, err := toolchainInstallArgs(tool, "example"); err != nil {
			t.Errorf("toolchainInstallArgs(%q) error = %v, want nil for manifest-valid tool", tool, err)
		}
	}
}

func TestToolchain_Installed_UnknownTool_ReturnsTypedError(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	b := newToolchainBackend(fake)
	pkg := manifest.Package{
		Name:    "example",
		Backend: manifest.BackendToolchain,
		Tool:    "gem",
	}
	info := platform.Info{}

	installed, err := b.Installed(pkg, info)
	if err == nil {
		t.Fatal("Installed() error = nil, want *UnknownToolError")
	}
	var unknownErr *UnknownToolError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("Installed() error = %T, want *UnknownToolError", err)
	}
	if unknownErr.Tool != "gem" {
		t.Errorf("UnknownToolError.Tool = %q, want %q", unknownErr.Tool, "gem")
	}
	if installed {
		t.Error("Installed() = true, want false when tool is unknown")
	}
}

func TestToolchain_Install_RunError(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	wantErr := fmt.Errorf("install failed")
	fake.RunErrors = map[string]error{
		exec.CommandKey("cargo", "install", "ripgrep"): wantErr,
	}
	b := newToolchainBackend(fake)
	pkg := manifest.Package{
		Name:    "ripgrep",
		Backend: manifest.BackendToolchain,
		Tool:    "cargo",
	}
	info := platform.Info{}

	err := b.Install(pkg, info)
	if err == nil {
		t.Fatal("Install() error = nil, want wrapped run error")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("Install() error = %v, want %v", err, wantErr)
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
