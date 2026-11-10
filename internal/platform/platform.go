package platform

import (
	"os"
	"path/filepath"
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
	getenv := os.Getenv
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
		Desktop:   detectDesktop(os, getenv, hasWaylandSocket),
	}
}

// detectDesktop reports whether a graphical session is available to GUI
// packages (manifest requires_desktop). DOTF_DESKTOP=1|true|0|false overrides
// detection on macOS and the Linux family (never enabling it elsewhere). macOS
// otherwise always has one. On the Linux family it requires XDG_SESSION_TYPE
// x11/wayland, a WAYLAND_DISPLAY name (WSLg sets it), or a live wayland-*
// socket in XDG_RUNTIME_DIR — the last covers tmux/herdr and SSH shells on a
// machine whose desktop session is running but whose session env vars did not
// reach the shell. DISPLAY alone is deliberately ignored because SSH X11
// forwarding sets it on otherwise headless hosts.
func detectDesktop(o OS, getenv func(string) string, waylandSocket func(runtimeDir string) bool) bool {
	if o != OSMacOS && !IsLinuxFamily(o) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(getenv("DOTF_DESKTOP"))) {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	}
	if o == OSMacOS {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(getenv("XDG_SESSION_TYPE"))) {
	case "x11", "wayland":
		return true
	}
	if strings.TrimSpace(getenv("WAYLAND_DISPLAY")) != "" {
		return true
	}
	rt := strings.TrimSpace(getenv("XDG_RUNTIME_DIR"))
	return rt != "" && waylandSocket(rt)
}

// hasWaylandSocket reports whether runtimeDir (an absolute XDG_RUNTIME_DIR)
// holds a wayland-N unix socket, i.e. a compositor is running for this user.
// Lock files and non-socket entries do not count.
func hasWaylandSocket(runtimeDir string) bool {
	if runtimeDir == "" || !filepath.IsAbs(runtimeDir) {
		return false
	}
	matches, _ := filepath.Glob(filepath.Join(runtimeDir, "wayland-*"))
	for _, m := range matches {
		if strings.HasSuffix(m, ".lock") {
			continue
		}
		if fi, err := os.Lstat(m); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return true
		}
	}
	return false
}

// detectOS determines the canonical OS/distro family from the os-release file
// at osReleasePath. It is the path-taking seam the tests drive; Detect scans
// os-release once and calls detectOSFrom directly.
func detectOS(r exec.Runner, osReleasePath, procVersionPath string) OS {
	return detectOSFrom(r, readOSRelease(osReleasePath), procVersionPath)
}

// detectOSFrom determines the canonical OS/distro family from an already
// scanned os-release record. This is the canonical detection;
// common/platform.sh:detect_os is the shell fallback used only when dotf and
// its platform env cache are absent. uname -s is resolved via r, falling back
// to runtime.GOOS if the command is unavailable, so detection never panics
// even on a minimal/sandboxed host.
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
// "linux". Canonical; common/platform.sh:detect_os's Linux* branch is the shell
// fallback.
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
// canonical OS family. Canonical; the ID/ID_LIKE case statement in
// common/platform.sh:detect_os is the shell fallback and follows this mapping.
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
// contains "microsoft" (case-insensitive) — the same WSL signal the shell
// fallback common/platform.sh:detect_os checks. Returns false
// (never panics) if the file is missing or unreadable.
func procVersionHasMicrosoft(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}

// detectArch determines the canonical CPU architecture (shell fallback:
// common/platform.sh:detect_arch). uname -m is resolved via
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

// archFromUname maps a raw `uname -m` value to the canonical Arch, including
// passthrough of unrecognized values. Canonical; the case statement in
// common/platform.sh:detect_arch is the shell fallback.
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
// Linuxbrew is still selected there when no native manager is present. The
// shell fallback common/platform.sh:detect_package_manager uses the same
// priority.
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
// environment (shell fallback: common/platform.sh:detect_container).
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
// "docker" — the same signal as the shell fallback's
// `grep -q docker /proc/1/cgroup` check. Returns false (never
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
