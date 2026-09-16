package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// TestProbeInstalled exercises every probe path: a Provides list (all present
// vs one missing), an InstalledWhen.Path (present, absent, and ~-expanded), and
// the default name-based PATH fallback. HOME is set so the tilde case is
// hermetic, so this test does not run in parallel.
func TestProbeInstalled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A real directory for the InstalledWhen.Path (no ~) case.
	existingDir := t.TempDir()

	// A marker under HOME for the tilde-expansion case.
	if err := os.WriteFile(filepath.Join(home, "marker"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(marker) error = %v", err)
	}

	tests := []struct {
		name  string
		pkg   manifest.Package
		found []string // binary names AddPath should resolve
		want  bool
	}{
		{
			name:  "provides all present",
			pkg:   manifest.Package{Name: "aws-tools", Provides: []string{"aws", "eksctl"}},
			found: []string{"aws", "eksctl"},
			want:  true,
		},
		{
			name:  "provides one missing",
			pkg:   manifest.Package{Name: "aws-tools", Provides: []string{"aws", "eksctl"}},
			found: []string{"aws"},
			want:  false,
		},
		{
			name:  "provides_any one present",
			pkg:   manifest.Package{Name: "fabric", ProvidesAny: []string{"fabric", "fabric-ai"}},
			found: []string{"fabric-ai"},
			want:  true,
		},
		{
			name: "provides_any none present",
			pkg:  manifest.Package{Name: "fabric", ProvidesAny: []string{"fabric", "fabric-ai"}},
			want: false,
		},
		{
			name:  "combined provides and provides_any both satisfied",
			pkg:   manifest.Package{Name: "lua", Provides: []string{"luarocks"}, ProvidesAny: []string{"lua", "lua5.1", "luajit"}},
			found: []string{"luarocks", "lua5.1"},
			want:  true,
		},
		{
			name:  "combined provides present but no any-of member",
			pkg:   manifest.Package{Name: "lua", Provides: []string{"luarocks"}, ProvidesAny: []string{"lua", "lua5.1", "luajit"}},
			found: []string{"luarocks"},
			want:  false,
		},
		{
			name:  "combined any-of present but a provides member missing",
			pkg:   manifest.Package{Name: "lua", Provides: []string{"luarocks"}, ProvidesAny: []string{"lua", "lua5.1", "luajit"}},
			found: []string{"lua"},
			want:  false,
		},
		{
			name: "installed_when path exists",
			pkg:  manifest.Package{Name: "tpm", InstalledWhen: &manifest.Probe{Path: existingDir}},
			want: true,
		},
		{
			name: "installed_when path absent",
			pkg:  manifest.Package{Name: "tpm", InstalledWhen: &manifest.Probe{Path: filepath.Join(existingDir, "nope")}},
			want: false,
		},
		{
			name: "installed_when tilde expansion",
			pkg:  manifest.Package{Name: "tpm", InstalledWhen: &manifest.Probe{Path: "~/marker"}},
			want: true,
		},
		{
			name:  "default fallback name found",
			pkg:   manifest.Package{Name: "atuin"},
			found: []string{"atuin"},
			want:  true,
		},
		{
			name: "default fallback name not found",
			pkg:  manifest.Package{Name: "atuin"},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := exec.NewFake()
			for _, name := range tc.found {
				fake.AddPath(name, "/usr/bin/"+name)
			}

			got, err := probeInstalled(fake, tc.pkg)
			if err != nil {
				t.Fatalf("probeInstalled() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("probeInstalled() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCustomScript_Installed_BundleProvides models the toolchain rg/ripgrep
// precedent for a bundle package: the manifest name ("aws-tools") is never a
// PATH binary, so the Provides list is the authoritative probe — Installed
// reports true when the provided binary (aws) resolves and false when it does
// not, regardless of the package name.
func TestCustomScript_Installed_BundleProvides(t *testing.T) {
	t.Parallel()

	pkg := manifest.Package{
		Name:     "aws-tools",
		Backend:  manifest.BackendCustomScript,
		Script:   "bin/install-aws-tools.sh",
		Provides: []string{"aws"},
	}

	t.Run("provided binary present", func(t *testing.T) {
		t.Parallel()

		fake := exec.NewFake()
		fake.AddPath("aws", "/usr/local/bin/aws")
		b := newCustomScriptBackend(fake, t.TempDir())

		got, err := b.Installed(pkg, platform.Info{})
		if err != nil {
			t.Fatalf("Installed() error = %v, want nil", err)
		}
		if !got {
			t.Error("Installed() = false, want true when the provided binary (aws) is on PATH though the package name (aws-tools) is not")
		}
	})

	t.Run("provided binary absent", func(t *testing.T) {
		t.Parallel()

		fake := exec.NewFake()
		b := newCustomScriptBackend(fake, t.TempDir())

		got, err := b.Installed(pkg, platform.Info{})
		if err != nil {
			t.Fatalf("Installed() error = %v, want nil", err)
		}
		if got {
			t.Error("Installed() = true, want false when the provided binary is missing")
		}
	})
}
