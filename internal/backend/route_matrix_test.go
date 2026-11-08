package backend_test

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/privilege"
)

// routeHost is one cell of the OS family × arch matrix every sibling family is
// checked against. Desktop is always on: requires_desktop must not hide a gap.
type routeHost struct {
	os     platform.OS
	pkgMgr platform.PkgMgr
	arch   platform.Arch
}

func (h routeHost) String() string { return fmt.Sprintf("%s/%s", h.os, h.arch) }

func (h routeHost) info() platform.Info {
	return platform.Info{OS: h.os, PkgMgr: h.pkgMgr, Arch: h.arch, Desktop: true}
}

var (
	debianX86   = routeHost{platform.OSDebian, platform.PkgMgrAPT, platform.ArchX86_64}
	debianARM   = routeHost{platform.OSDebian, platform.PkgMgrAPT, platform.ArchARM64}
	redhatX86   = routeHost{platform.OSRedHat, platform.PkgMgrDNF, platform.ArchX86_64}
	redhatARM   = routeHost{platform.OSRedHat, platform.PkgMgrDNF, platform.ArchARM64}
	archX86     = routeHost{platform.OSArch, platform.PkgMgrPacman, platform.ArchX86_64}
	archARM     = routeHost{platform.OSArch, platform.PkgMgrPacman, platform.ArchARM64}
	macosX86    = routeHost{platform.OSMacOS, platform.PkgMgrHomebrew, platform.ArchX86_64}
	macosARM    = routeHost{platform.OSMacOS, platform.PkgMgrHomebrew, platform.ArchARM64}
	routeMatrix = []routeHost{debianX86, debianARM, redhatX86, redhatARM, archX86, archARM, macosX86, macosARM}
)

// unsupportedRoutes is the allow-list of intentional gaps: sibling families
// with no applicable install route on a host. Every entry carries a one-line
// reason so dropping (or gaining) platform support is a visible diff here. A
// listed gap that becomes covered fails the test too, keeping the list exact.
var unsupportedRoutes = map[string]map[routeHost]string{
	"1password": {
		debianARM: "vendor arm64 apt tree carries only 1password-cli; desktop app ships as a tarball (YAN-1074)",
		redhatARM: "vendor aarch64 rpm tree carries only 1password-cli; desktop app ships as a tarball (YAN-1074)",
	},
	"beekeeper-studio": {
		debianX86: "no Debian route declared; Fedora takes the vendor rpm, Arch the AUR",
		debianARM: "no Debian route declared; Fedora takes the vendor rpm, Arch the AUR",
	},
	"claude-desktop": {
		archX86: "no Arch route declared (apt repo on Debian, repacked .deb on Fedora, cask on macOS)",
		archARM: "no Arch route declared (apt repo on Debian, repacked .deb on Fedora, cask on macOS)",
	},
	"discord": {
		debianARM: "upstream ships no Linux arm64 build (YAN-1073)",
		redhatARM: "upstream ships no Linux arm64 build (YAN-1073)",
		archARM:   "upstream ships no Linux arm64 build (YAN-1073)",
	},
	"ghostty": {
		debianX86: "no verified Debian route exists",
		debianARM: "no verified Debian route exists",
	},
	"grok-bot": {
		debianX86: "no Debian route declared; Fedora takes the dnf repo, Arch the AUR",
		debianARM: "no Debian route declared; Fedora takes the dnf repo, Arch the AUR",
	},
	"herdr-app": {
		macosX86: "official macOS app installer supports Apple silicon only; Intel Macs are unsupported",
	},
	"localsend": {
		debianARM: "Linux app-artifact route is gated to x86_64; no arm64 build declared",
		redhatARM: "Linux app-artifact route is gated to x86_64; no arm64 build declared",
		archARM:   "Linux app-artifact route is gated to x86_64; no arm64 build declared",
	},
	"mediawriter": {
		debianX86: "absent from Debian/Ubuntu official repos",
		debianARM: "absent from Debian/Ubuntu official repos",
	},
	"ruff": {
		debianX86: "no apt package name declared; ruff is pacman/dnf/homebrew only",
		debianARM: "no apt package name declared; ruff is pacman/dnf/homebrew only",
	},
	"slack": {
		debianX86: "no Debian route declared; Fedora takes the vendor rpm, Arch the AUR",
		debianARM: "no Debian route declared; Fedora takes the vendor rpm, Arch the AUR",
		redhatARM: "vendor rpm route is gated to x86_64; no aarch64 build declared",
	},
	"teams-for-linux": {
		debianX86: "no Debian route declared; Fedora takes the vendor rpm, Arch the AUR",
		debianARM: "no Debian route declared; Fedora takes the vendor rpm, Arch the AUR",
	},
	"winboat": {
		debianX86: "no Debian route declared; Fedora takes the vendor rpm, Arch the AUR",
		debianARM: "no Debian route declared; Fedora takes the vendor rpm, Arch the AUR",
		redhatARM: "vendor rpm route is gated to x86_64; no aarch64 build declared",
		macosX86:  "Linux-only (runs Windows apps in a Linux container)",
		macosARM:  "Linux-only (runs Windows apps in a Linux container)",
	},
	"zoom": {
		debianX86: "no Debian route declared; Fedora takes the vendor rpm, Arch the AUR",
		debianARM: "no Debian route declared; Fedora takes the vendor rpm, Arch the AUR",
		redhatARM: "vendor rpm route is gated to x86_64; no aarch64 build declared",
	},
}

// routeFamilies groups the manifest's packages into sibling families keyed by
// the base name, using the same manifest.PlatformSiblingSuffixes that
// `--only <name>` expands with. An entry joins a family only when stripping a
// suffix yields another declared package (so "teams-for-linux" stays its own
// base rather than a "-linux" sibling of a nonexistent "teams-for"). Only
// families with two or more entries are returned: a single-entry tool that is
// Linux- or macOS-only by design has no sibling routes to collide or drop.
func routeFamilies(m *manifest.Manifest) map[string][]manifest.Package {
	declared := make(map[string]bool, len(m.Packages))
	for _, pkg := range m.Packages {
		declared[pkg.Name] = true
	}
	all := make(map[string][]manifest.Package)
	for _, pkg := range m.Packages {
		base := pkg.Name
		for _, suffix := range manifest.PlatformSiblingSuffixes {
			if b, ok := strings.CutSuffix(pkg.Name, suffix); ok && declared[b] {
				base = b
				break
			}
		}
		all[base] = append(all[base], pkg)
	}
	families := make(map[string][]manifest.Package)
	for base, pkgs := range all {
		if len(pkgs) > 1 {
			families[base] = pkgs
		}
	}
	return families
}

// TestEmbeddedManifest_OneApplicableRoutePerHost asserts, over the real
// embedded manifest and the real backend gates, that every sibling family has
// exactly one applicable install route on each OS family × arch host, or zero
// where unsupportedRoutes documents the gap. Two applicable routes means one
// `--only <name>` would install the tool twice; an unexpected zero means a
// gate edit silently dropped a platform. No network: Applicable is pure.
func TestEmbeddedManifest_OneApplicableRoutePerHost(t *testing.T) {
	t.Parallel()

	sub, err := fs.Sub(dotfiles.ManifestFS, "manifest")
	if err != nil {
		t.Fatalf("fs.Sub() error = %v", err)
	}
	m, err := manifest.LoadFS(sub, "packages.toml")
	if err != nil {
		t.Fatalf("LoadFS(embedded, packages.toml) error = %v", err)
	}
	engine := backend.Default(exec.NewFake(), privilege.NewSudo(), privilege.NewFakeDropper(), t.TempDir(), nil)

	families := routeFamilies(m)
	if len(families) == 0 {
		t.Fatal("embedded manifest declares no sibling families; the route matrix is vacuous")
	}
	for base := range unsupportedRoutes {
		if _, ok := families[base]; !ok {
			t.Errorf("unsupportedRoutes lists %q, which is not a sibling family in the manifest; drop the stale entry", base)
		}
	}

	for base, pkgs := range families {
		for _, host := range routeMatrix {
			var applicable []string
			for _, pkg := range pkgs {
				ok, err := engine.Applicable(pkg, host.info())
				if err != nil {
					t.Fatalf("%s on %s: Applicable(%s) error = %v", base, host, pkg.Name, err)
				}
				if ok {
					applicable = append(applicable, pkg.Name)
				}
			}
			reason, gap := unsupportedRoutes[base][host]
			switch {
			case len(applicable) > 1:
				t.Errorf("%s on %s: %d applicable routes %v, want exactly one", base, host, len(applicable), applicable)
			case len(applicable) == 0 && !gap:
				t.Errorf("%s on %s: no applicable route; add one or document the gap in unsupportedRoutes", base, host)
			case len(applicable) == 1 && gap:
				t.Errorf("%s on %s: route %s is applicable but unsupportedRoutes lists the gap (%q); drop the stale entry",
					base, host, applicable[0], reason)
			}
		}
	}
}
