// Package brew bootstraps Homebrew on hosts that lack it, so the pkg backend's
// `brew install` path (internal/backend/pkg.go) has a working brew to shell out
// to. It mirrors the legacy bootstrap.sh install_homebrew() step: install
// Homebrew via the official installer when absent — on macOS always (brew is the
// only package manager there) and on Linux only when explicitly requested
// (linuxbrew via --homebrew=true).
//
// The installer is executed through the exec.Runner seam so tests assert the
// argv without touching the network. The installer command is a compile-time
// constant (no interpolation of caller-controlled input), which is why the
// otherwise-avoided `bash -c` form is acceptable here.
package brew

import (
	"fmt"
	"path/filepath"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// installCommand runs the official Homebrew installer non-interactively.
// NONINTERACTIVE=1 is Homebrew's own flag for suppressing its confirmation
// prompt; the outer `bash -c` is a constant string, never templated with
// caller input.
const installCommand = `curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh | NONINTERACTIVE=1 /bin/bash`

// Applicable reports whether a Homebrew bootstrap should run on info given the
// --homebrew tristate. macOS always bootstraps brew (it is the canonical
// package manager); a Linux-family host bootstraps linuxbrew only when the
// caller opted in with --homebrew=true AND the architecture is one Homebrew
// actually supports. Homebrew publishes no aarch64-Linux bottles and its
// installer refuses to run there, so an arm64 Linux host is declined rather
// than handed a doomed installer whose failure would fail the whole run.
// Everything else is a no-op.
func Applicable(info platform.Info, homebrew string) bool {
	switch {
	case info.OS == platform.OSMacOS:
		return true
	case platform.IsLinuxFamily(info.OS) && homebrew == "true":
		return info.Arch != platform.ArchARM64
	default:
		return false
	}
}

// DeclinedUnsupportedArch reports whether info+homebrew asked for a Linux
// Homebrew bootstrap that Applicable refuses purely because of the
// architecture, so the caller can say so on stderr instead of silently doing
// nothing with an opt-in flag the user explicitly passed.
func DeclinedUnsupportedArch(info platform.Info, homebrew string) bool {
	return platform.IsLinuxFamily(info.OS) && homebrew == "true" && info.Arch == platform.ArchARM64
}

// Bootstrap installs Homebrew when it is absent and applicable to this host. It
// is a no-op — returning (false, nil) — when brew is already on PATH or when a
// bootstrap does not apply to this host/tristate. On a successful install it
// returns (true, nil); the caller should then add BinDir(info) to PATH and
// re-run platform.Detect so PkgMgrHomebrew is selected for the packages phase.
// An installer failure returns (false, err) so the caller can record it
// non-fatally.
func Bootstrap(r exec.Runner, info platform.Info, homebrew string) (bool, error) {
	if _, err := r.LookPath("brew"); err == nil {
		return false, nil // already installed
	}
	if !Applicable(info, homebrew) {
		return false, nil
	}
	if err := r.Run("bash", "-c", installCommand); err != nil {
		return false, fmt.Errorf("brew: bootstrap Homebrew: %w", err)
	}
	return true, nil
}

// BinDir returns the bin directory of the Homebrew prefix for info: Apple
// Silicon macOS uses /opt/homebrew, Intel macOS /usr/local, and every
// Linux-family host the shared linuxbrew prefix. The freshly-run installer does
// not touch the current process PATH, so the caller prepends this directory
// before re-detecting the package manager and running `brew install`.
func BinDir(info platform.Info) string {
	return filepath.Join(prefix(info), "bin")
}

// prefix returns the Homebrew installation prefix for info, mirroring
// bin/install-brew.sh's prefix selection.
func prefix(info platform.Info) string {
	switch {
	case info.OS == platform.OSMacOS && info.Arch == platform.ArchARM64:
		return "/opt/homebrew"
	case info.OS == platform.OSMacOS:
		return "/usr/local"
	default:
		return "/home/linuxbrew/.linuxbrew"
	}
}
