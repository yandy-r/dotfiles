package manifest_test

import (
	"slices"
	"testing"

	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/privilege"
)

const t3Icon = "https://raw.githubusercontent.com/pingdotgg/t3code/v0.1.7/apps/marketing/public/apple-touch-icon.png"

// TestEmbeddedManifest_T3Code pins the T3 Code routes: separately installable
// client (apt .deb, other-Linux AppImage, macOS cask) and server (the official
// full-runtime installer, never a partial github-release copy).
func TestEmbeddedManifest_T3Code(t *testing.T) {
	m := loadEmbeddedManifest(t)
	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}
	get := func(name string) manifest.Package {
		t.Helper()
		pkg, ok := byName[name]
		if !ok {
			t.Fatalf("embedded manifest has no package %q", name)
		}
		return pkg
	}

	clients := []string{"t3-code-client", "t3-code-client-arm64", "t3-code-client-debian", "t3-code-client-macos"}
	if !slices.Contains(m.Intents["ai"].Packages, "t3-code-server") {
		t.Error("ai intent missing t3-code-server")
	}
	for _, name := range clients {
		if !slices.Contains(m.Intents["desktop"].Packages, name) {
			t.Errorf("desktop intent missing %q", name)
		}
		if slices.Contains(m.Intents["ai"].Packages, name) {
			t.Errorf("ai intent must not contain client %q", name)
		}
	}
	if slices.Contains(m.Intents["desktop"].Packages, "t3-code-server") {
		t.Error("desktop intent must not contain t3-code-server")
	}

	t.Run("appimage clients", func(t *testing.T) {
		for name, asset := range map[string]string{
			"t3-code-client":       "T3-Code-*-x86_64.AppImage",
			"t3-code-client-arm64": "T3-Code-*-arm64.AppImage",
		} {
			pkg := get(name)
			arch := map[string]string{"t3-code-client": "x86_64", "t3-code-client-arm64": "arm64"}[name]
			if pkg.Backend != manifest.BackendAppArtifact || pkg.OS != "linux" || pkg.Repo != "pingdotgg/t3code" ||
				pkg.Asset != asset || pkg.Class != manifest.ClassRolling || !pkg.RequiresDesktop || pkg.Tag != "" || pkg.SHA256 != "" {
				t.Errorf("%s = %+v, want rolling linux desktop app-artifact for %s", name, pkg, asset)
			}
			if !slices.Equal(pkg.Arch, []string{arch}) || !slices.Equal(pkg.ExcludeOS, []string{"debian"}) {
				t.Errorf("%s arch=%v exclude_os=%v, want [%s] and [debian]", name, pkg.Arch, pkg.ExcludeOS, arch)
			}
			if !slices.Equal(pkg.Requires, []string{"appimage-runtime"}) {
				t.Errorf("%s requires = %v, want [appimage-runtime] only", name, pkg.Requires)
			}
			if pkg.App == nil || pkg.App.Icon != t3Icon || pkg.App.DesktopName != "T3 Code" || pkg.App.Categories != "Development;" {
				t.Errorf("%s app = %+v, want T3 Code launcher with official icon", name, pkg.App)
			}
			// Launcher fields mirror the official deb's desktop file so the
			// AppImage handles t3code:// links and groups under one window class.
			if pkg.App != nil && (pkg.App.ExecArgs != "%U" || pkg.App.StartupWMClass != "t3code" ||
				pkg.App.MimeType != "x-scheme-handler/t3code;x-scheme-handler/t3code-dev;") {
				t.Errorf("%s launcher fields = exec_args %q startup_wm_class %q mime_type %q, want %%U / t3code / t3code+t3code-dev scheme handlers",
					name, pkg.App.ExecArgs, pkg.App.StartupWMClass, pkg.App.MimeType)
			}
		}
	})

	t.Run("debian client", func(t *testing.T) {
		pkg := get("t3-code-client-debian")
		if pkg.Backend != manifest.BackendPkgURL || pkg.OS != "debian" || pkg.Repo != "pingdotgg/t3code" ||
			pkg.Asset != "T3-Code-*.deb" || pkg.Format != "deb" || pkg.PkgName != "t3code" ||
			pkg.Class != manifest.ClassRolling || !pkg.RequiresDesktop {
			t.Errorf("t3-code-client-debian = %+v, want rolling debian-family desktop pkg-url deb t3code", pkg)
		}
		if len(pkg.Arch) != 0 || len(pkg.ExcludeOS) != 0 || pkg.SHA256 != "" || len(pkg.Checksums) != 0 || pkg.ChecksumsAsset != "" || pkg.AllowUnsigned {
			t.Errorf("t3-code-client-debian carries arch/exclude/checksum/allow_unsigned fields: %+v", pkg)
		}
	})

	t.Run("macos client", func(t *testing.T) {
		pkg := get("t3-code-client-macos")
		if pkg.Backend != manifest.BackendPkg || pkg.OS != "macos" || !pkg.RequiresDesktop || pkg.Names["homebrew"] != "t3-code" {
			t.Errorf("t3-code-client-macos = %+v, want macos desktop cask t3-code", pkg)
		}
	})

	t.Run("server", func(t *testing.T) {
		pkg := get("t3-code-server")
		if pkg.Backend != manifest.BackendScriptURL || pkg.URL != "https://t3.codes/install.sh" || len(pkg.Args) != 0 {
			t.Errorf("t3-code-server = %+v, want script-url https://t3.codes/install.sh with no args", pkg)
		}
		if !slices.Equal(pkg.Arch, []string{"x86_64", "arm64"}) || pkg.OS != "linux" || pkg.RequiresDesktop {
			t.Errorf("t3-code-server os=%q arch=%v desktop=%t, want linux x86_64+arm64 headless", pkg.OS, pkg.Arch, pkg.RequiresDesktop)
		}
		if !slices.Equal(pkg.Provides, []string{"t3"}) {
			t.Errorf("t3-code-server provides = %v, want [t3]", pkg.Provides)
		}
		if pkg.Backend == manifest.BackendGitHubRelease || pkg.Backend == manifest.BackendCustomScript {
			t.Errorf("t3-code-server must use the official installer, got %s", pkg.Backend)
		}
	})

	t.Run("one client route per host", func(t *testing.T) {
		engine := backend.Default(exec.NewFake(), privilege.NewSudo(), privilege.NewFakeDropper(), t.TempDir(), nil)
		hosts := []struct {
			name string
			info platform.Info
			want string
		}{
			{"debian-x86_64", platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrAPT, Desktop: true}, "t3-code-client-debian"},
			{"debian-arm64", platform.Info{OS: platform.OSDebian, Arch: platform.ArchARM64, PkgMgr: platform.PkgMgrAPT, Desktop: true}, "t3-code-client-debian"},
			{"wsl-apt", platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrAPT, Desktop: true}, "t3-code-client-debian"},
			{"debian-linuxbrew-primary", platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrHomebrew, Desktop: true}, "t3-code-client-debian"},
			{"debian-linuxbrew-primary-arm64", platform.Info{OS: platform.OSDebian, Arch: platform.ArchARM64, PkgMgr: platform.PkgMgrHomebrew, Desktop: true}, "t3-code-client-debian"},
			{"fedora-linuxbrew-primary", platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrHomebrew, Desktop: true}, "t3-code-client"},
			{"fedora-x86_64", platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrDNF, Desktop: true}, "t3-code-client"},
			{"arch-arm64", platform.Info{OS: platform.OSArch, Arch: platform.ArchARM64, PkgMgr: platform.PkgMgrPacman, Desktop: true}, "t3-code-client-arm64"},
			{"suse-x86_64", platform.Info{OS: platform.OSSUSE, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrZypper, Desktop: true}, "t3-code-client"},
			{"macos-arm64", platform.Info{OS: platform.OSMacOS, Arch: platform.ArchARM64, PkgMgr: platform.PkgMgrHomebrew, Desktop: true}, "t3-code-client-macos"},
		}
		for _, h := range hosts {
			var got []string
			for _, name := range clients {
				ok, err := engine.Applicable(get(name), h.info)
				if err != nil {
					t.Fatalf("%s: Applicable(%s) error = %v", h.name, name, err)
				}
				if ok {
					got = append(got, name)
				}
			}
			if !slices.Equal(got, []string{h.want}) {
				t.Errorf("%s: applicable clients = %v, want [%s]", h.name, got, h.want)
			}
		}
	})
}
