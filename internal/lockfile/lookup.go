package lockfile

// Package returns the LockedPackage named name, or false when absent. The nil
// receiver returns false so callers can hold a nil *Lock (the read-command
// no-verification path) without a guard at every call site.
func (l *Lock) Package(name string) (LockedPackage, bool) {
	if l == nil {
		return LockedPackage{}, false
	}
	for _, p := range l.Packages {
		if p.Name == name {
			return p, true
		}
	}
	return LockedPackage{}, false
}

// Artifact returns the locked Artifact for (name, os, arch), or false when
// absent. It first seeks an exact (os, arch) match — the per-platform
// github-release case — and then falls back to a platform-agnostic entry
// (empty OS and Arch) so a script-url pin recorded once matches a host lookup
// regardless of the host's tokens. The nil receiver returns false.
func (l *Lock) Artifact(name, os, arch string) (Artifact, bool) {
	p, ok := l.Package(name)
	if !ok {
		return Artifact{}, false
	}
	for _, a := range p.Artifacts {
		if a.OS == os && a.Arch == arch {
			return a, true
		}
	}
	if os != "" || arch != "" {
		for _, a := range p.Artifacts {
			if a.OS == "" && a.Arch == "" {
				return a, true
			}
		}
	}
	return Artifact{}, false
}
