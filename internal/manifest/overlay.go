package manifest

import (
	"fmt"
	"os"
	"path/filepath"
)

// DiscoverOverlays finds and strictly decodes the XDG user-config overlay
// fragments under configRoot/packages.d/*.toml, returning one Layer per fragment
// in sorted (filepath.Glob) order so the downstream Merge is deterministic. Each
// fragment is decoded with the SAME strict DisallowUnknownFields decoder as the
// base manifest — a typo errors loudly, wrapped with the offending file path —
// but is NOT independently Validate'd: an overlay intent may reference a
// base-only package, so cross-references are only checked against the merged
// whole (see Merge). Every package and every module step in a fragment is
// stamped with ScriptRoot=configRoot, so a user-local custom-script (package or
// module step) resolves and is contained under ~/.config/dotf rather than the
// repo root.
//
// A missing packages.d directory (or an empty configRoot) yields (nil, nil): an
// absent overlay is the normal default, unlike an explicit --manifest whose
// absence is a hard error. modules.d/*.toml is intentionally deferred to the
// module work.
func DiscoverOverlays(configRoot string) ([]Layer, error) {
	if configRoot == "" {
		return nil, nil
	}

	pattern := filepath.Join(configRoot, "packages.d", "*.toml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("manifest: glob overlays %q: %w", pattern, err)
	}
	if len(matches) == 0 {
		return nil, nil
	}

	layers := make([]Layer, 0, len(matches))
	for _, path := range matches {
		layer, err := loadOverlay(path, configRoot)
		if err != nil {
			return nil, err
		}
		layers = append(layers, layer)
	}
	return layers, nil
}

// loadOverlay opens and strictly decodes one overlay fragment, stamps every
// package's and module step's ScriptRoot with configRoot, and returns it as a Layer labelled by
// the fragment path. The decode error is wrapped with the file path so a typo in
// one fragment names exactly which file to fix.
func loadOverlay(path, configRoot string) (Layer, error) {
	f, err := os.Open(path)
	if err != nil {
		return Layer{}, fmt.Errorf("manifest: open overlay %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	m, err := decode(f)
	if err != nil {
		return Layer{}, fmt.Errorf("manifest: overlay %q: %w", path, err)
	}

	stampScriptRoot(m, configRoot)
	return Layer{Name: path, Manifest: m}, nil
}

// stampScriptRoot sets ScriptRoot on every package and every module step in m,
// marking configRoot as the containment root the custom-script backend resolves
// a package's Script (or a step's CustomScript, via the synthetic package
// `dotf setup` builds) against (see internal/backend/custom_script.go). It is
// provenance the decoder cannot set (Package.ScriptRoot and
// ModuleStep.ScriptRoot are toml:"-"), so a user cannot smuggle a different
// root through the fragment.
func stampScriptRoot(m *Manifest, configRoot string) {
	for i := range m.Packages {
		m.Packages[i].ScriptRoot = configRoot
	}
	for i := range m.Modules {
		for j := range m.Modules[i].Steps {
			m.Modules[i].Steps[j].ScriptRoot = configRoot
		}
	}
}
