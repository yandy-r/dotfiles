package backend

import (
	"os"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
)

// probeInstalled evaluates a package's explicit installed-state probe when it
// declares one (Provides / ProvidesAny / InstalledWhen), falling back to a PATH
// lookup of the effective binary name (binaryName: the github-release Binary
// override, else pkg.Name — so a Binary!=Name package is probed under the name
// it was installed as). It is the shared Installed() body for the backends that
// install a binary onto PATH (custom-script, github-release, script-url,
// toolchain), whose default name-based probe lies about bundle/group packages
// whose manifest name is not itself a binary. A failed lookup/stat means "not
// installed" (false, nil), matching the existing swallow-error behavior.
//
// The Provides/ProvidesAny family composes: every Provides binary must resolve
// (all-present) AND, when ProvidesAny is set, at least one of its candidates
// must resolve (any-of). That lets a package require a fixed toolset plus one
// binary whose name varies by platform (lua: the stable `luarocks` plus one of
// `lua`/`lua5.1`/`luajit`). InstalledWhen is consulted only when neither
// Provides nor ProvidesAny is set (they are mutually exclusive with it).
func probeInstalled(r exec.Runner, pkg manifest.Package) (bool, error) {
	switch {
	case len(pkg.Provides) > 0 || len(pkg.ProvidesAny) > 0:
		for _, bin := range pkg.Provides {
			if _, err := r.LookPath(bin); err != nil {
				return false, nil // any missing all-present binary => not installed
			}
		}
		if len(pkg.ProvidesAny) > 0 && !anyOnPath(r, pkg.ProvidesAny) {
			return false, nil // none of the any-of candidates resolved => not installed
		}
		return true, nil
	case pkg.InstalledWhen != nil && (pkg.InstalledWhen.Path != "" || len(pkg.InstalledWhen.Paths) > 0):
		paths := pkg.InstalledWhen.Paths
		if pkg.InstalledWhen.Path != "" {
			paths = []string{pkg.InstalledWhen.Path}
		}
		for _, p := range paths {
			path, err := expandProbePath(p)
			if err != nil {
				return false, nil
			}
			if _, err := os.Stat(path); err != nil {
				return false, nil // any missing path => not installed
			}
		}
		return true, nil
	default:
		if _, err := r.LookPath(binaryName(pkg)); err != nil {
			return false, nil
		}
		return true, nil
	}
}

// anyOnPath reports whether at least one of bins resolves on PATH — the any-of
// half of the Provides/ProvidesAny probe.
func anyOnPath(r exec.Runner, bins []string) bool {
	for _, bin := range bins {
		if _, err := r.LookPath(bin); err == nil {
			return true
		}
	}
	return false
}

// expandProbePath expands a leading "~" and any $VAR references in an
// installed_when path against the current environment.
func expandProbePath(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = home + strings.TrimPrefix(p, "~")
	}
	return os.ExpandEnv(p), nil
}
