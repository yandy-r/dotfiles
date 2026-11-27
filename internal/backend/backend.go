// Package backend defines the install-engine contract: a Backend interface
// implemented once per manifest.Backend value, and an Engine that dispatches
// Install/Installed calls to the correct concrete backend via a static map.
package backend

import (
	"fmt"
	"slices"

	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// Backend is the install mechanism contract for a single manifest.Backend
// value. Every canonical backend implements this interface.
type Backend interface {
	// Install provisions pkg on the host described by info.
	Install(pkg manifest.Package, info platform.Info) error

	// Installed reports whether pkg is already present on the host.
	Installed(pkg manifest.Package, info platform.Info) (bool, error)
}

// UnknownBackendError is returned when Engine dispatch cannot find a Backend
// implementation for pkg.Backend.
type UnknownBackendError struct {
	// Backend is the manifest.Backend value that has no registered handler.
	Backend manifest.Backend
}

// Error implements the error interface.
func (e *UnknownBackendError) Error() string {
	return fmt.Sprintf("backend: unknown backend %q", e.Backend)
}

// Engine routes Install and Installed calls to the Backend registered for
// each package's Backend field.
type Engine struct {
	backends map[manifest.Backend]Backend
}

// NewEngine constructs an Engine backed by the given dispatch map.
func NewEngine(backends map[manifest.Backend]Backend) *Engine {
	return &Engine{backends: backends}
}

// Install delegates to the Backend registered for pkg.Backend, first honoring
// the package's OS gate: installing an off-platform package is a no-op. See
// Installed for the defense-in-depth rationale — all three Engine methods
// enforce pkg.OS uniformly so a direct caller cannot run an installer on the
// wrong OS.
func (e *Engine) Install(pkg manifest.Package, info platform.Info) error {
	b, ok := e.backends[pkg.Backend]
	if !ok {
		return &UnknownBackendError{Backend: pkg.Backend}
	}
	if !HostAllows(pkg, info) {
		return nil
	}
	return b.Install(pkg, info)
}

// Installed delegates to the Backend registered for pkg.Backend, first honoring
// the package's OS gate: an off-platform package is never installed here. The
// gate on Install/Installed is defense-in-depth — every current caller
// (installPackage, driftReport) checks Applicable first, which already gates on
// pkg.OS, so these guard only a hypothetical direct caller. Applicable, Install,
// and Installed all short-circuit on pkg.OS so the gate is uniform across the
// read and write paths.
func (e *Engine) Installed(pkg manifest.Package, info platform.Info) (bool, error) {
	b, ok := e.backends[pkg.Backend]
	if !ok {
		return false, &UnknownBackendError{Backend: pkg.Backend}
	}
	if !HostAllows(pkg, info) {
		return false, nil
	}
	return b.Installed(pkg, info)
}

// HostAllows reports whether pkg's static host gates — OS, exclude_os, arch,
// and requires_desktop — admit the host described by info. exclude_os rejects
// the host when ANY listed token matches it under osGateAllows semantics (so
// "linux" excludes every Linux-family host), independent of the host's package
// manager. It is pure (runs no commands) and ignores backend-specific
// applicability; every Engine entry point
// (Applicable/Installed/Install/Update/Remove) uses it, and package
// selection uses it to decide whose prerequisites to follow.
func HostAllows(pkg manifest.Package, info platform.Info) bool {
	if !osGateAllows(pkg.OS, info.OS) {
		return false
	}
	for _, tok := range pkg.ExcludeOS {
		if tok != "" && osGateAllows(tok, info.OS) {
			return false
		}
	}
	return (len(pkg.Arch) == 0 || slices.Contains(pkg.Arch, string(info.Arch))) &&
		(!pkg.RequiresDesktop || info.Desktop)
}

// osGateAllows reports whether a package restricted to the manifest os token
// `want` may run on the host OS. An empty want matches every host. "linux"
// matches every Linux-family host (via platform.IsLinuxFamily — the single
// source of truth for family membership); "macos"/"windows" and the specific
// distro tokens (debian/redhat/arch/suse) match exactly. The token set is kept
// in lockstep with manifest validation (validatePackageOS), which rejects any
// other value at parse time.
func osGateAllows(want string, host platform.OS) bool {
	switch want {
	case "":
		return true
	case "macos":
		return host == platform.OSMacOS
	case "windows":
		return host == platform.OSWindows
	case "linux":
		return platform.IsLinuxFamily(host)
	default:
		return string(host) == want
	}
}

// platformGated is implemented by backends that can only run on certain
// hosts (repo requires apt, aur requires Arch). Backends without this
// method are applicable on every host.
type platformGated interface {
	applicable(info platform.Info) bool
}

// packageGated is implemented by backends whose applicability depends on the
// PACKAGE as well as the host: the pkg backend can only install an entry that
// declares a name for the host's package manager. Backends without this method
// are package-agnostic. Same optional-interface shape as platformGated.
type packageGated interface {
	applicableFor(pkg manifest.Package, info platform.Info) bool
}

// lockable is implemented by backends that can resolve a manifest package to a
// frozen lockfile.Artifact (download + hash the release asset). Only the
// download backends (github-release, script-url) implement it; backends without
// it are not lockable. Same optional-interface shape as platformGated.
type lockable interface {
	resolve(pkg manifest.Package, target platform.Info) (lockfile.Artifact, error)
}

// Resolve resolves pkg for the target platform to a frozen lockfile.Artifact,
// dispatching to the backend registered for pkg.Backend. It backs `dotf lock`.
// A backend that does not implement lockable is not lockable and yields a typed
// error, mirroring Applicable's optional-interface dispatch. An artifact that
// resolves with an empty sha256 is an error, never a lock row.
func (e *Engine) Resolve(pkg manifest.Package, target platform.Info) (lockfile.Artifact, error) {
	b, ok := e.backends[pkg.Backend]
	if !ok {
		return lockfile.Artifact{}, &UnknownBackendError{Backend: pkg.Backend}
	}
	l, ok := b.(lockable)
	if !ok {
		return lockfile.Artifact{}, fmt.Errorf("backend %q is not lockable", pkg.Backend)
	}
	art, err := l.resolve(pkg, target)
	if err != nil {
		return lockfile.Artifact{}, err
	}
	// Backstop for every lockable backend: a locked artifact must carry a
	// checksum, so an unresolved one is a per-target lock failure, never a row.
	if art.SHA256 == "" {
		osToken, archToken := releaseTokens(target)
		return lockfile.Artifact{}, unresolvedChecksumErr(pkg.Name, osToken, archToken)
	}
	return art, nil
}

// unresolvedChecksumErr is returned when `dotf lock` resolves no checksum for a
// pinned package on a target (no per-target checksums entry, no inline sha256,
// no checksums_asset). Recording an empty sha256 would wedge Install on that
// host, so the target fails instead. The message names the manifest remedy
// (re-running `dotf lock` cannot fix it).
func unresolvedChecksumErr(name, osToken, archToken string) error {
	return fmt.Errorf("pinned artifact %q resolves no checksum for %s/%s; add a checksums entry for this target (or sha256 / checksums_asset) to the manifest", name, osToken, archToken)
}

// pinnedNoChecksumErr is the shared fail-fast error returned when a Pinnable
// package resolves to neither a locked artifact nor an inline sha256: a pinned
// artifact with no checksum must not install unverified. The message names the
// remedy so a fresh machine can self-serve.
func pinnedNoChecksumErr(name, osToken, archToken string) error {
	return fmt.Errorf("pinned artifact %q has no locked checksum for %s/%s; run `dotf lock`", name, osToken, archToken)
}

// Applicable reports whether the backend registered for pkg.Backend can run
// on the host described by info. Read-only commands (status, diff) use this
// to treat platform-incompatible manifest entries as not applicable instead
// of failing on the Install/Installed unsupported-platform error. A backend
// may also veto a single package (packageGated) — a pkg entry that declares no
// name for the host's package manager is not applicable there.
func (e *Engine) Applicable(pkg manifest.Package, info platform.Info) (bool, error) {
	b, ok := e.backends[pkg.Backend]
	if !ok {
		return false, &UnknownBackendError{Backend: pkg.Backend}
	}
	if !HostAllows(pkg, info) {
		return false, nil
	}
	if pg, ok := b.(packageGated); ok && !pg.applicableFor(pkg, info) {
		return false, nil
	}
	if g, ok := b.(platformGated); ok {
		return g.applicable(info), nil
	}
	return true, nil
}
