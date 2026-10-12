// Package platform provides canonical, typed detection of the host
// operating system, CPU architecture, package manager, and container
// environment.
//
// This package is the single source of truth for the vocabulary that
// common/platform.sh has historically defined across the shell ecosystem
// (DOTFILES_OS, DOTFILES_ARCH, DOTFILES_PKG_MGR, DOTFILES_CONTAINER). The
// string values returned here MUST match common/platform.sh exactly so the
// Go CLI and the shell scripts agree (see issue #4).
package platform

// OS identifies the canonical operating system / distro family, mirroring
// common/platform.sh:detect_os.
type OS string

// Canonical OS values. These MUST match common/platform.sh's detect_os
// output exactly.
const (
	OSMacOS   OS = "macos"
	OSDebian  OS = "debian"
	OSRedHat  OS = "redhat"
	OSArch    OS = "arch"
	OSSUSE    OS = "suse"
	OSLinux   OS = "linux"
	OSWSL     OS = "wsl"
	OSWindows OS = "windows"
	OSUnknown OS = "unknown"
)

// Arch identifies the canonical CPU architecture, mirroring
// common/platform.sh:detect_arch.
//
// Arch is a typed string (rather than a closed enum) so that unrecognized
// architectures can carry through the raw `uname -m` value, exactly as the
// shell does. A future ReleaseArch() mapper (Phase 3, for mapping to
// release-artifact naming conventions e.g. GOARCH or GitHub release asset
// suffixes) can be added as a method on this type without reshaping it.
type Arch string

// Canonical Arch values. These MUST match common/platform.sh's detect_arch
// output exactly (e.g. "x86_64", never "amd64").
// Unrecognized architectures are not coerced to a sentinel value — they
// carry through the raw `uname -m` string, exactly as
// common/platform.sh:detect_arch's default case does.
const (
	// ArchX86_64's underscore is deliberate, not a naming-convention slip:
	// it mirrors the raw `x86_64` value exactly (see the "MUST match" note
	// above). Do not "fix" it to ArchX8664 or ArchX86_64 -> ArchX86-64.
	ArchX86_64 Arch = "x86_64" //nolint:revive,stylecheck // intentional: mirrors raw x86_64
	ArchARM64  Arch = "arm64"
	ArchARM    Arch = "arm"
)

// ReleaseArch maps canonical detection arch values to release-artifact
// naming conventions (e.g. GOARCH or GitHub release asset suffixes).
func (a Arch) ReleaseArch() string {
	switch a {
	case ArchX86_64:
		return "amd64"
	case ArchARM64:
		return "arm64"
	case ArchARM:
		return "arm"
	default:
		return string(a)
	}
}

// ReleaseArchAliases returns the set of architecture tokens a GitHub release
// asset may use for this host's CPU, feeding the asset auto-detector. x86_64
// covers {x86_64, x86-64, amd64, x64}; arm64 covers {aarch64, arm64, arm-64};
// 32-bit arm covers its common variants. An unrecognized arch passes through as
// a one-element set carrying its raw value, mirroring ReleaseArch's default
// case. The matcher compares whole, delimited tokens, so "arm" never
// disqualifies an "arm64" asset, and discounts a token nested inside a longer
// one, so the "arm" in "arm-64" is not read as 32-bit arm.
func (a Arch) ReleaseArchAliases() []string {
	switch a {
	case ArchX86_64:
		return []string{"x86_64", "x86-64", "amd64", "x64"}
	case ArchARM64:
		return []string{"aarch64", "arm64", "arm-64"}
	case ArchARM:
		return []string{"arm", "armv7", "armv7l", "armv6", "armv6l", "armhf", "armel"}
	default:
		return []string{string(a)}
	}
}

// ReleaseOS maps the canonical detection OS/distro-family to the release-artifact
// OS token GitHub release assets use, which is independent of the distro-family
// package-manager OS. macOS collapses to "darwin"; every Linux family
// (debian/redhat/arch/suse/linux/wsl) collapses to "linux"; Windows stays
// "windows"; an unknown OS yields "" (no release-OS vocabulary).
func (o OS) ReleaseOS() string {
	switch o {
	case OSMacOS:
		return "darwin"
	case OSWindows:
		return "windows"
	case OSDebian, OSRedHat, OSArch, OSSUSE, OSLinux, OSWSL:
		return "linux"
	default:
		return ""
	}
}

// ReleaseOSAliases returns the set of OS tokens a GitHub release asset may use
// for this host's release OS, feeding the asset auto-detector. darwin covers the
// Apple naming zoo (darwin/macos/osx/apple); linux and windows carry their
// common short forms. An unknown OS yields no aliases (nothing auto-detects).
func (o OS) ReleaseOSAliases() []string {
	switch o.ReleaseOS() {
	case "darwin":
		return []string{"darwin", "macos", "osx", "apple"}
	case "linux":
		return []string{"linux"}
	case "windows":
		return []string{"windows", "win"}
	default:
		return nil
	}
}

// PkgMgr identifies the canonical primary package manager, mirroring
// common/platform.sh:detect_package_manager.
type PkgMgr string

// Canonical PkgMgr values. These MUST match common/platform.sh's
// detect_package_manager output exactly.
const (
	PkgMgrHomebrew PkgMgr = "homebrew"
	PkgMgrAPT      PkgMgr = "apt"
	PkgMgrDNF      PkgMgr = "dnf"
	PkgMgrYUM      PkgMgr = "yum"
	PkgMgrPacman   PkgMgr = "pacman"
	PkgMgrZypper   PkgMgr = "zypper"
	PkgMgrUnknown  PkgMgr = "unknown"
)

// Container identifies the canonical container/virtualization environment,
// mirroring common/platform.sh:detect_container.
type Container string

// Canonical Container values. These MUST match common/platform.sh's
// detect_container output exactly.
const (
	ContainerDocker     Container = "docker"
	ContainerKubernetes Container = "kubernetes"
	ContainerWSL        Container = "wsl"
	ContainerNone       Container = "none"
)

// Info is the full set of detected platform characteristics for the
// current host.
type Info struct {
	// OS is the canonical OS/distro family (e.g. "arch", "debian").
	OS OS

	// Arch is the canonical CPU architecture (e.g. "x86_64", "arm64").
	Arch Arch

	// PkgMgr is the canonical primary package manager (e.g. "pacman").
	PkgMgr PkgMgr

	// Container is the detected container/virtualization environment.
	Container Container

	// Distro is the raw /etc/os-release ID string (e.g. "cachyos",
	// "ubuntu"), unmapped, mirroring common/platform.sh:get_distro_name.
	// "unknown" when /etc/os-release is unavailable or unreadable.
	Distro string

	// Codename is the Debian/Ubuntu release codename from /etc/os-release
	// VERSION_CODENAME (falling back to UBUNTU_CODENAME), e.g. "noble",
	// "jammy". Empty on non-Debian-family hosts and macOS.
	Codename string

	// VersionID is the raw /etc/os-release VERSION_ID (e.g. "42" on Fedora 42,
	// "24.04" on Ubuntu). It is the $releasever the repo backend's dnf_repo
	// templating consumes, and the fallback signal for the dnf4/dnf5 probe.
	// Empty on rolling distros that set no VERSION_ID, and on macOS/Windows.
	VersionID string

	// Desktop reports a detected graphical session (see detectDesktop). It gates
	// manifest packages marked requires_desktop.
	Desktop bool
}
