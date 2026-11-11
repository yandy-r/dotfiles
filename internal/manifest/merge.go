package manifest

import (
	"fmt"
	"sort"
)

// baseLabel is the provenance label for entries that originate in the base
// manifest (as opposed to an overlay Layer, labelled by its file path). It is
// used only to fill Collision.Shadowed for a first override.
const baseLabel = "base"

// Layer is one overlay manifest paired with a source label used only for
// diagnostics — a file path for XDG overlay fragments, so a collision or
// validation error can name where the shadowing entry came from.
type Layer struct {
	// Name labels the overlay's source; for XDG overlays it is the fragment's
	// file path, surfaced in Collision.Won.
	Name string

	// Manifest is the decoded overlay contents merged onto the base.
	Manifest *Manifest
}

// Collision records a single overlay-over-base override that Merge resolved by
// last-wins, so the caller can report it (overrides are never silent). It is
// returned as data rather than logged to keep Merge pure and I/O-free.
type Collision struct {
	// Kind is the colliding entity type: "package", "intent", or "module".
	Kind string

	// Key is the colliding identifier: the package name or intent name.
	Key string

	// Won names the layer whose value survived — the overlay's Layer.Name.
	Won string

	// Shadowed names the layer whose value was overridden — "base" or an earlier
	// overlay's Layer.Name.
	Shadowed string
}

// Merge deep-merges one or more overlay Layers onto base and returns the merged,
// validated manifest plus the list of package/intent overrides it resolved.
// Merge is pure: it performs no I/O, mutates neither base nor the overlays
// (base is deep-copied first), and returns collisions as data — the caller logs
// them. The merge is deterministic in overlay order (DiscoverOverlays returns
// fragments in sorted filepath.Glob order), so a re-run yields the same result
// and the same last-wins outcome.
//
// Per-kind policy:
//   - Intents: deep-merged. An overlay intent of the same name EXTENDS the base
//     one — its Packages are unioned onto the base list (first-seen order,
//     deduped), its Default is OR'd (an overlay may promote an intent to default
//     but never demote a curated one), and its Description overrides only when
//     non-empty AND different (that lone destructive case is recorded as a
//     Collision; a pure package-list extension or default promotion is additive
//     and silent).
//   - Packages: appended; on a Name collision the overlay package replaces the
//     base one (last-wins) and a Collision{Kind:"package"} is recorded.
//   - Modules: appended; on a Name collision the overlay module replaces the
//     base one (last-wins) and a Collision{Kind:"module"} is recorded — the same
//     last-wins policy as packages, so a user overlay can redefine a setup bundle.
//   - Sysconfig: field-wise override — each non-empty overlay field wins; a nil
//     base adopts the overlay's table wholesale.
//
// The merged manifest is passed through Validate, so callers always receive a
// guaranteed-valid result (or a wrapped validation/collision error). Validating
// the merged whole (not per-fragment) is why an overlay intent may reference a
// base-only package: cross-references resolve after the merge.
func Merge(base *Manifest, overlays ...Layer) (*Manifest, []Collision, error) {
	merged := cloneManifest(base)

	pkgSource := make(map[string]string, len(merged.Packages))
	for _, p := range merged.Packages {
		pkgSource[p.Name] = baseLabel
	}
	intentSource := make(map[string]string, len(merged.Intents))
	for name := range merged.Intents {
		intentSource[name] = baseLabel
	}
	moduleSource := make(map[string]string, len(merged.Modules))
	for _, mod := range merged.Modules {
		moduleSource[mod.Name] = baseLabel
	}

	var collisions []Collision
	for _, layer := range overlays {
		if layer.Manifest == nil {
			continue
		}
		collisions = append(collisions, mergeIntents(merged, layer, intentSource)...)
		collisions = append(collisions, mergePackages(merged, layer, pkgSource)...)
		collisions = append(collisions, mergeModules(merged, layer, moduleSource)...)
		mergeSysconfig(merged, layer.Manifest.Sysconfig)
	}

	if err := Validate(merged); err != nil {
		return nil, nil, fmt.Errorf("manifest: merged overlay validate: %w", err)
	}

	return merged, collisions, nil
}

// mergeIntents deep-merges layer's intents into merged. A brand-new intent is
// adopted; an existing intent has its Packages unioned (extend) and its Default
// OR'd, with its Description replaced only when the overlay provides a non-empty
// value that differs from the base — the one destructive case, recorded as a
// Collision. Overlay intents are visited in sorted name order so collision
// diagnostics stay deterministic under Go's randomized map iteration. source
// tracks each intent's Description provenance for Collision.Shadowed.
func mergeIntents(merged *Manifest, layer Layer, source map[string]string) []Collision {
	overlay := layer.Manifest.Intents
	if len(overlay) == 0 {
		return nil
	}
	if merged.Intents == nil {
		merged.Intents = make(map[string]Intent, len(overlay))
	}

	var collisions []Collision
	for _, name := range sortedIntentNames(overlay) {
		ov := overlay[name]
		base, exists := merged.Intents[name]
		if !exists {
			merged.Intents[name] = cloneIntent(ov)
			source[name] = layer.Name // record provenance so a later overlay's override chains Shadowed correctly (mirrors mergePackages)
			continue
		}

		next := Intent{
			Description: base.Description,
			Default:     base.Default || ov.Default,
			Packages:    unionStrings(base.Packages, ov.Packages),
		}
		if ov.Description != "" && ov.Description != base.Description {
			next.Description = ov.Description
			collisions = append(collisions, Collision{
				Kind: "intent", Key: name, Won: layer.Name, Shadowed: source[name],
			})
			source[name] = layer.Name
		}
		merged.Intents[name] = next
	}
	return collisions
}

// mergePackages appends layer's packages to merged, overriding any existing
// package of the same Name (last-wins) and recording a Collision for it. Overlay
// packages keep their fragment order so the merge is deterministic. source
// tracks each name's provenance for Collision.Shadowed.
func mergePackages(merged *Manifest, layer Layer, source map[string]string) []Collision {
	overlay := layer.Manifest.Packages
	if len(overlay) == 0 {
		return nil
	}

	index := make(map[string]int, len(merged.Packages))
	for i, p := range merged.Packages {
		index[p.Name] = i
	}

	var collisions []Collision
	for _, p := range overlay {
		if i, exists := index[p.Name]; exists {
			collisions = append(collisions, Collision{
				Kind: "package", Key: p.Name, Won: layer.Name, Shadowed: source[p.Name],
			})
			merged.Packages[i] = p
		} else {
			index[p.Name] = len(merged.Packages)
			merged.Packages = append(merged.Packages, p)
		}
		source[p.Name] = layer.Name
	}
	return collisions
}

// mergeModules appends layer's modules to merged, overriding any existing
// module of the same Name (last-wins) and recording a Collision for it. Overlay
// modules keep their fragment order so the merge is deterministic. source tracks
// each name's provenance for Collision.Shadowed, mirroring mergePackages.
func mergeModules(merged *Manifest, layer Layer, source map[string]string) []Collision {
	overlay := layer.Manifest.Modules
	if len(overlay) == 0 {
		return nil
	}

	index := make(map[string]int, len(merged.Modules))
	for i, mod := range merged.Modules {
		index[mod.Name] = i
	}

	var collisions []Collision
	for _, mod := range overlay {
		if i, exists := index[mod.Name]; exists {
			collisions = append(collisions, Collision{
				Kind: "module", Key: mod.Name, Won: layer.Name, Shadowed: source[mod.Name],
			})
			merged.Modules[i] = mod
		} else {
			index[mod.Name] = len(merged.Modules)
			merged.Modules = append(merged.Modules, mod)
		}
		source[mod.Name] = layer.Name
	}
	return collisions
}

// mergeSysconfig field-wise overrides merged's sysconfig with overlay's
// non-empty fields. A nil base adopts the overlay's table wholesale; a nil
// overlay leaves the base untouched. merged.Sysconfig is already a deep copy
// (cloneManifest), so this never mutates the caller's base.
func mergeSysconfig(merged *Manifest, overlay *Sysconfig) {
	if overlay == nil {
		return
	}
	if merged.Sysconfig == nil {
		sc := *overlay
		merged.Sysconfig = &sc
		return
	}
	if overlay.Timezone != "" {
		merged.Sysconfig.Timezone = overlay.Timezone
	}
	if overlay.Locale != "" {
		merged.Sysconfig.Locale = overlay.Locale
	}
	if overlay.Shell != "" {
		merged.Sysconfig.Shell = overlay.Shell
	}
	if overlay.Firewall != "" {
		merged.Sysconfig.Firewall = overlay.Firewall
	}
	if overlay.SSHServer != "" {
		merged.Sysconfig.SSHServer = overlay.SSHServer
	}
}

// unionStrings returns the concatenation of a and b with duplicates removed,
// preserving first-seen order (base entries first, then overlay-only additions),
// so an overlay intent EXTENDS the base package list deterministically rather
// than replacing it.
func unionStrings(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	seen := make(map[string]struct{}, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, s := range list {
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// sortedIntentNames returns the keys of intents in ascending order.
func sortedIntentNames(intents map[string]Intent) []string {
	names := make([]string, 0, len(intents))
	for name := range intents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// cloneManifest returns a deep-enough copy of m for Merge to mutate the result
// (its intent map, package/module slices, and sysconfig pointer) without
// touching the caller's base manifest. Package/Module inner maps/slices are
// shared but never mutated — Merge only appends or replaces whole values.
func cloneManifest(m *Manifest) *Manifest {
	out := &Manifest{}

	if m.Intents != nil {
		out.Intents = make(map[string]Intent, len(m.Intents))
		for name, intent := range m.Intents {
			out.Intents[name] = cloneIntent(intent)
		}
	}

	if m.Packages != nil {
		out.Packages = make([]Package, len(m.Packages))
		copy(out.Packages, m.Packages)
	}

	if m.Modules != nil {
		out.Modules = make([]Module, len(m.Modules))
		copy(out.Modules, m.Modules)
	}

	if m.Sysconfig != nil {
		sc := *m.Sysconfig
		out.Sysconfig = &sc
	}

	return out
}

// cloneIntent copies in with its own Packages backing array, so unioning an
// overlay's packages onto a merged intent never mutates the base intent's slice.
func cloneIntent(in Intent) Intent {
	out := in
	if in.Packages != nil {
		out.Packages = make([]string, len(in.Packages))
		copy(out.Packages, in.Packages)
	}
	return out
}
