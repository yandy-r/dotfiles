package manifest_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// arm64SiblingSuffix names the arch-gated twin of a base package; `--only
// <base>` selects both and relies on the arch gates to leave exactly one
// applicable per host.
const arm64SiblingSuffix = "-arm64"

// TestEmbeddedManifest_Arm64SiblingsMirrorBase pins the contract every
// "<base>-arm64" entry depends on: it is gated to arm64 alone, its base excludes
// arm64 (so the two are never both applicable), and the pair shares one OS gate
// and one intent membership (so `--intent <name>` never drops the app on one
// arch).
func TestEmbeddedManifest_Arm64SiblingsMirrorBase(t *testing.T) {
	m := loadEmbeddedManifest(t)

	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}
	intentsOf := func(name string) []string {
		var out []string
		for intent, def := range m.Intents {
			if slices.Contains(def.Packages, name) {
				out = append(out, intent)
			}
		}
		slices.Sort(out)
		return out
	}

	arm64 := string(platform.ArchARM64)
	pairs := 0
	for _, sibling := range m.Packages {
		baseName, ok := strings.CutSuffix(sibling.Name, arm64SiblingSuffix)
		if !ok {
			continue
		}
		base, ok := byName[baseName]
		if !ok {
			t.Errorf("%s: no base package %q; an %q suffix is reserved for arch siblings", sibling.Name, baseName, arm64SiblingSuffix)
			continue
		}
		pairs++

		if !slices.Equal(sibling.Arch, []string{arm64}) {
			t.Errorf("%s: arch = %v, want [%s]", sibling.Name, sibling.Arch, arm64)
		}
		if len(base.Arch) == 0 || slices.Contains(base.Arch, arm64) {
			t.Errorf("%s: arch = %v, want a non-empty gate excluding %s (its %s sibling owns that arch)",
				base.Name, base.Arch, arm64, sibling.Name)
		}
		if sibling.OS != base.OS {
			t.Errorf("%s: os = %q, want %q to match %s", sibling.Name, sibling.OS, base.OS, base.Name)
		}
		if got, want := intentsOf(sibling.Name), intentsOf(base.Name); !slices.Equal(got, want) {
			t.Errorf("%s: intents = %v, want %v to match %s", sibling.Name, got, want, base.Name)
		}
	}
	if pairs == 0 {
		t.Fatalf("embedded manifest declares no %q sibling pairs; the contract test is vacuous", arm64SiblingSuffix)
	}
}
