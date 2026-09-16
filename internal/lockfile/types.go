// Package lockfile owns the schema and IO for dotf.lock: a committed,
// embeddable TOML lockfile that freezes the resolved tag/asset/binary/sha256/
// os/arch for PINNED artifacts only. It mirrors internal/manifest's style —
// dependency-light, importing only internal/manifest, the stdlib, and
// go-toml/v2 — and is the single source of truth for the lockfile's shape.
//
// The lock is scoped to pinned artifacts (see Pinnable). Rolling managers and
// setup-step scripts are intentionally floating/probe-only and carry no lock
// entry, so a re-lock of a converged repo is a byte-identical no-op.
package lockfile

// Version is the current lockfile schema version. Parse rejects any other
// value (fail-fast), mirroring manifest.Parse's strict contract.
const Version = 1

// Lock is the top-level decoded shape of dotf.lock: a schema version plus the
// flat list of locked packages.
type Lock struct {
	// Version is the lockfile schema version; it must equal Version.
	Version int `toml:"version"`

	// Packages is the flat list of locked package records, one per pinned
	// package that was resolved.
	Packages []LockedPackage `toml:"package"`
}

// LockedPackage freezes the resolved release coordinates for one pinned
// package. Tag pins a github-release; URL pins a script-url installer; a given
// package uses exactly one of the two. Artifacts carries a checksum per
// (os, arch) target so a single record can cover templated, per-platform
// assets.
type LockedPackage struct {
	// Name is the manifest package name this record locks.
	Name string `toml:"name"`

	// Backend is the manifest.Backend value that produced this record
	// (github-release or script-url).
	Backend string `toml:"backend"`

	// Tag is the pinned github-release tag (e.g. "v1.2.3"); empty for
	// script-url.
	Tag string `toml:"tag,omitempty"`

	// URL is the pinned script-url installer URL; empty for github-release.
	URL string `toml:"url,omitempty"`

	// Artifacts is the per-target checksum set. A github-release carries one
	// Artifact per locked (os, arch); a platform-agnostic script-url carries a
	// single Artifact with empty OS/Arch/Asset/Binary.
	Artifacts []Artifact `toml:"artifact"`
}

// Artifact is one platform's frozen download coordinates and checksum. A
// per-(os, arch) Artifact is the point: it carries a checksum per platform, so
// a single SHA256 need not cover an {{.OS}}/{{.Arch}}-templated asset. A
// platform-agnostic script-url pin is one Artifact with empty OS/Arch/Asset/
// Binary and just a SHA256.
type Artifact struct {
	// OS is the release OS token (string(platform.OS)); empty for a
	// platform-agnostic script-url pin.
	OS string `toml:"os,omitempty"`

	// Arch is the release arch token (platform.Arch.ReleaseArch()); empty for
	// a platform-agnostic script-url pin.
	Arch string `toml:"arch,omitempty"`

	// Asset is the resolved github-release asset filename; empty for
	// script-url.
	Asset string `toml:"asset,omitempty"`

	// Binary is the binary name extracted from a github-release archive; empty
	// for script-url.
	Binary string `toml:"binary,omitempty"`

	// SHA256 is the hex-encoded checksum of the downloaded artifact.
	SHA256 string `toml:"sha256"`
}
