package platform

import (
	"os"
	"runtime"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
)

// defaultProcVersionPath is the standard location of the kernel version
// banner on Linux, used to detect WSL when /etc/os-release is unavailable.
// Overridable in tests via detectOS's procVersionPath parameter.
const defaultProcVersionPath = "/proc/version"

// Detect probes the host via r and the filesystem and returns its canonical
// platform Info. Detection degrades gracefully to the "unknown"/zero-value
// vocabulary on any failure — it never panics.
func Detect(r exec.Runner) Info {
	rel := readOSRelease(defaultOSReleasePath)
	os := detectOSFrom(r, rel, defaultProcVersionPath)
	arch := detectArch(r)
	return Info{
		OS:        os,
		Arch:      arch,
		PkgMgr:    detectPkgMgr(r, os, arch),
		Container: detectContainer(),
		Distro:    rel.distro(),
		Codename:  rel.Codename,
		VersionID: rel.VersionID,
	}
}

// detectOS determines the canonical OS/distro family from the os-release file
// at osReleasePath. It is the path-taking seam the tests drive; Detect scans
// os-release once and calls detectOSFrom directly.
func detectOS(r exec.Runner, osReleasePath, procVersionPath string) OS {
	return detectOSFrom(r, readOSRelease(osReleasePath), procVersionPath)
}

// detectOSFrom determines the canonical OS/distro family from an already
// scanned os-release record, mirroring common/platform.sh:detect_os. uname -s
// is resolved via r, falling back to runtime.GOOS if the command is
// unavailable, so detection never panics even on a minimal/sandboxed host.
func detectOSFrom(r exec.Runner, rel osRelease, procVersionPath string) OS {
	uname := unameS(r)

	switch {
	case strings.HasPrefix(uname, "Darwin"):
		return OSMacOS
	case strings.HasPrefix(uname, "Linux"):
		return detectLinuxFamily(rel, procVersionPath)
	case strings.HasPrefix(uname, "CYGWIN"), strings.HasPrefix(uname, "MINGW"), strings.HasPrefix(uname, "MSYS"):
		return OSWindows
	default:
		return OSUnknown
	}
}

// unameS returns the `uname -s` kernel name via r, falling back to a
// runtime.GOOS-derived equivalent if the command errors so callers always
// get a usable (if degraded) value.
func unameS(r exec.Runner) string {
	if out, err := r.Output("uname", "-s"); err == nil {
		return strings.TrimSpace(string(out))
	}

	switch runtime.GOOS {
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	case "windows":
		return "MSYS"
	default:
		return ""
	}
}

// detectLinuxFamily resolves the Linux distro family from a scanned
// os-release record, falling back to /proc/version (WSL detection) and finally
// "linux", mirroring common/platform.sh:detect_os's Linux* branch.
func detectLinuxFamily(rel osRelease, procVersionPath string) OS {
	if !rel.Present {
		if procVersionHasMicrosoft(procVersionPath) {
			return OSWSL
		}
		return OSLinux
	}

	return osFamilyFromID(rel.ID, rel.IDLike)
}

// osFamilyFromID maps an os-release ID (falling back to ID_LIKE) to the
// canonical OS family, mirroring common/platform.sh:detect_os's ID/ID_LIKE
// case statement exactly.
func osFamilyFromID(id, idLike string) OS {
	switch id {
	case "ubuntu", "debian":
		return OSDebian
	case "fedora", "fedora-asahi-remix", "asahi-fedora-remix", "rhel", "centos", "rocky", "almalinux":
		return OSRedHat
	case "arch", "cachyos", "manjaro", "endeavouros":
		return OSArch
	}
	if id == "sles" || strings.HasPrefix(id, "opensuse") {
		return OSSUSE
	}

	switch {
	case strings.Contains(idLike, "debian"), strings.Contains(idLike, "ubuntu"):
		return OSDebian
	case strings.Contains(idLike, "fedora"), strings.Contains(idLike, "rhel"):
		return OSRedHat
	case strings.Contains(idLike, "arch"):
		return OSArch
	case strings.Contains(idLike, "suse"):
		return OSSUSE
	default:
		return OSLinux
	}
}

// fileExists reports whether path exists and is readable as a regular
// file/symlink, mirroring the shell's `[[ -f path ]]` test.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// procVersionHasMicrosoft reports whether the kernel version banner at path
// contains "microsoft" (case-insensitive), mirroring common/platform.sh:
// detect_os's `grep -qi microsoft /proc/version` WSL check. Returns false
// (never panics) if the file is missing or unreadable.
func procVersionHasMicrosoft(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}

// detectArch determines the canonical CPU architecture, mirroring
// common/platform.sh:detect_arch. uname -m is resolved via
// r, falling back to runtime.GOARCH if the command errors.
func detectArch(r exec.Runner) Arch {
	return archFromUname(unameM(r))
}

// unameM returns the `uname -m` machine hardware name via r, falling back
// to runtime.GOARCH if the command errors so callers always get a usable
// (if degraded) value. Mirrors unameS's fallback pattern for symmetry.
func unameM(r exec.Runner) string {
	if out, err := r.Output("uname", "-m"); err == nil {
		return strings.TrimSpace(string(out))
	}
	return runtime.GOARCH
}

// archFromUname maps a raw `uname -m` value to the canonical Arch,
// mirroring common/platform.sh:detect_arch's case statement exactly,
// including passthrough of unrecognized values.
func archFromUname(raw string) Arch {
	switch raw {
	case "x86_64", "amd64":
		return ArchX86_64
	case "aarch64", "arm64":
		return ArchARM64
	case "armv7l", "armv6l":
		return ArchARM
	default:
		return Arch(raw)
	}
}

// detectPkgMgr determines the canonical primary package manager. On macOS
// Homebrew IS the package manager, so it is probed first. On x86_64 Linux it is
// still probed first, because `--homebrew=true` deliberately bootstraps
// linuxbrew and expects the packages phase to use it (see cmd/dotf/apply.go's
// applyBrew). On aarch64 Linux the NATIVE manager wins instead: Homebrew
// publishes no aarch64-Linux bottles at all, so an incidental linuxbrew install
// must not silently reroute every package away from the manifest's dnf names.
// Linuxbrew is still selected there when no native manager is present.
//
// NOTE: this deliberately DIVERGES from common/platform.sh:detect_package_manager,
// which still prioritizes brew unconditionally. Reconcile the shell side
// separately; do not "fix" this back.
//
// Note: the shell implementation checks `command -v brew`, which also
// resolves shell functions and aliases, not just PATH executables. r's
// LookPath (like Go's exec.LookPath) only resolves executables on PATH, so
// the two can diverge in the edge case of a brew shim defined as a shell
// function/alias with no corresponding PATH binary.
func detectPkgMgr(r exec.Runner, os OS, arch Arch) PkgMgr {
	type candidate struct {
		bin string
		mgr PkgMgr
	}
	brew := candidate{"brew", PkgMgrHomebrew}
	native := []candidate{
		{"apt", PkgMgrAPT},
		{"dnf", PkgMgrDNF},
		{"yum", PkgMgrYUM},
		{"pacman", PkgMgrPacman},
		{"zypper", PkgMgrZypper},
	}

	candidates := append([]candidate{brew}, native...)
	if IsLinuxFamily(os) && arch == ArchARM64 {
		candidates = append(append([]candidate{}, native...), brew)
	}

	for _, c := range candidates {
		if _, err := r.LookPath(c.bin); err == nil {
			return c.mgr
		}
	}
	return PkgMgrUnknown
}

// detectContainer determines the canonical container/virtualization
// environment, mirroring common/platform.sh:detect_container.
func detectContainer() Container {
	if fileExists("/.dockerenv") || cgroupHasDocker("/proc/1/cgroup") {
		return ContainerDocker
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return ContainerKubernetes
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return ContainerWSL
	}
	return ContainerNone
}

// cgroupHasDocker reports whether the cgroup file at path mentions
// "docker", mirroring common/platform.sh:detect_container's
// `grep -q docker /proc/1/cgroup 2>/dev/null` check. Returns false (never
// panics) if the file is missing or unreadable.
func cgroupHasDocker(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "docker")
}

// IsLinuxFamily reports whether os is in the systemd Linux family that
// sysconfig.Default registers to a single linuxConfigurer: Debian, RedHat,
// Arch, SUSE, the generic OSLinux fallback, and OSWSL (WSL is treated as
// Linux-family for parity). macOS, Windows, and Unknown are false.
//
// This is the canonical membership shared by internal/snapper and dotf apply.
// It intentionally INCLUDES the bare OSLinux fallback; the narrower
// desktop-facet predicate in internal/symlink (isLinuxFamily there)
// deliberately EXCLUDES OSLinux for a different purpose, so the two are not
// interchangeable.
func IsLinuxFamily(os OS) bool {
	switch os {
	case OSDebian, OSRedHat, OSArch, OSSUSE, OSLinux, OSWSL:
		return true
	default:
		return false
	}
}

// GateAllows reports whether a manifest os token ("", "macos", "windows",
// "linux", or a distro family) permits the host OS. An empty want matches every
// host; "linux" matches every Linux-family host via IsLinuxFamily; every other
// token matches its OS value exactly.
//
// It is the canonical form of the gate that internal/backend's osGateAllows and
// internal/manifest's knownPackageOS are kept in lockstep with.
func GateAllows(want string, host OS) bool {
	switch want {
	case "":
		return true
	case "macos":
		return host == OSMacOS
	case "windows":
		return host == OSWindows
	case "linux":
		return IsLinuxFamily(host)
	default:
		return string(host) == want
	}
}
