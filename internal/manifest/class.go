package manifest

import (
	"sort"
	"strings"
)

// Class is a package's update-policy bucket: the governance vocabulary the
// lockfile (#46) and a future update verb consume to decide whether an artifact
// is checksum-locked, re-resolved to latest, or deferred to an external
// authority. The vocabulary is intentionally capped at three values.
type Class string

// Canonical Class values. These are the only three update-policy buckets.
const (
	// ClassPinned is an artifact locked to an exact, reproducible version
	// (a github-release tag+sha256, a checksummed script-url).
	ClassPinned Class = "pinned"

	// ClassRolling tracks upstream latest: a self-updating installer or an
	// unpinned rotating script with no single lockable artifact.
	ClassRolling Class = "rolling"

	// ClassManaged defers versioning to an external authority (the host
	// package manager, a toolchain manager, or a governed setup step).
	ClassManaged Class = "managed"
)

// Valid reports whether c is one of the three canonical Class values.
func (c Class) Valid() bool {
	switch c {
	case ClassPinned, ClassRolling, ClassManaged:
		return true
	default:
		return false
	}
}

// classByBackend is the per-backend legality table: which classes a given
// Backend may declare. The authority-managed backends (pkg/aur/repo/toolchain/
// git-clone/font) accept only "managed"; github-release is always
// "pinned"; script-url may be "pinned" (checksummed) or "rolling" (rotating
// installer); custom-script — the escape hatch — accepts all three but must
// declare one explicitly (see validateClass).
var classByBackend = map[Backend]map[Class]bool{
	BackendPkg:           {ClassManaged: true},
	BackendAUR:           {ClassManaged: true},
	BackendRepo:          {ClassManaged: true},
	BackendToolchain:     {ClassManaged: true},
	BackendGitClone:      {ClassManaged: true},
	BackendFont:          {ClassManaged: true},
	BackendFlatpak:       {ClassManaged: true},
	BackendGitHubRelease: {ClassPinned: true},
	BackendScriptURL:     {ClassPinned: true, ClassRolling: true},
	BackendCustomScript:  {ClassPinned: true, ClassRolling: true, ClassManaged: true},
	BackendAppArtifact:   {ClassPinned: true, ClassRolling: true},
	BackendPkgURL:        {ClassPinned: true, ClassRolling: true},
}

// LegalClasses returns the classes b may declare, sorted so validation
// messages are stable. An unknown backend yields an empty slice.
func LegalClasses(b Backend) []Class {
	legal := classByBackend[b]
	classes := make([]Class, 0, len(legal))
	for c := range legal {
		classes = append(classes, c)
	}
	sort.Slice(classes, func(i, j int) bool { return classes[i] < classes[j] })
	return classes
}

// joinClasses renders classes as a comma-separated string for error messages.
func joinClasses(classes []Class) string {
	strs := make([]string, len(classes))
	for i, c := range classes {
		strs[i] = string(c)
	}
	return strings.Join(strs, ", ")
}

// EffectiveClass resolves p's update-policy class: the explicit Class when set,
// otherwise the class inferred from the backend. pkg/aur/repo/toolchain,
// git-clone (a governed clone setup step, never a checksummed lockable artifact)
// and font (either the package authority installs it, or the download verifies
// its own checksum at install time) infer "managed"; github-release infers
// "pinned"; script-url infers "pinned" when a sha256 is present and "rolling"
// when it is absent (mirroring validate.go's "sha256 optional distinguishes
// rotating installers" rule, so an unpinned installer auto-classifies as
// rolling with zero manifest churn); app-artifact and pkg-url add the repo
// route as pinned (the checksum table is always lockable there).
// custom-script (and any future backend with no safe default) returns "" — a
// value validation forces the manifest to declare explicitly.
func (p Package) EffectiveClass() Class {
	if p.Class != "" {
		return p.Class
	}
	switch p.Backend {
	case BackendPkg, BackendAUR, BackendRepo, BackendToolchain, BackendGitClone, BackendFont, BackendFlatpak:
		return ClassManaged
	case BackendGitHubRelease:
		return ClassPinned
	case BackendScriptURL:
		if p.SHA256 != "" {
			return ClassPinned
		}
		return ClassRolling
	case BackendAppArtifact, BackendPkgURL:
		// Mirror script-url's sha256 rule, plus the repo route: a repo-route
		// package always carries a checksum source (fail-closed validation),
		// so it is lockable and pinned even when the checksum is a sidecar
		// (checksums/checksums_asset) rather than inline sha256. A url route
		// is pinned only when checksummed.
		if p.Repo != "" || p.SHA256 != "" {
			return ClassPinned
		}
		return ClassRolling
	default:
		return ""
	}
}
