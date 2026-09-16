package lockfile

import "github.com/yandy-r/dotfiles/internal/manifest"

// Pinnable reports whether pkg is in scope for the lockfile: exactly the
// packages whose effective update-policy class is pinned. This is the single
// scope gate shared by `dotf lock` (which packages to resolve) and the Install
// enforcement (which packages must carry a checksum).
//
// It keys on manifest.Package.EffectiveClass so the class taxonomy is the sole
// source of truth: github-release infers pinned; script-url infers pinned when
// it declares an inline sha256 and rolling when it does not (so a rotating
// installer like starship is never locked); pkg/aur/apt-repo/toolchain infer
// managed; and custom-script must declare its class explicitly. Only pinned is
// lockable.
func Pinnable(pkg manifest.Package) bool {
	return pkg.EffectiveClass() == manifest.ClassPinned
}
