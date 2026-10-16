package manifest_test

import (
	"slices"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

// Pin the user-facing selection, probe and lifecycle contract for oh-my-pi.
func TestEmbeddedManifest_OMP(t *testing.T) {
	m := loadEmbeddedManifest(t)
	for _, p := range m.Packages {
		if p.Name != "omp" {
			continue
		}
		if p.Backend != manifest.BackendCustomScript || p.Script != "bin/install-omp.sh" {
			t.Fatalf("omp must use its lifecycle wrapper: %+v", p)
		}
		if !slices.Equal(p.Provides, []string{"omp"}) || !slices.Equal(p.Actions, []string{"update", "remove"}) {
			t.Fatalf("omp probe/actions = %v/%v", p.Provides, p.Actions)
		}
		if p.EffectiveClass() != manifest.ClassRolling || p.Justification == "" {
			t.Fatal("omp must justify its rolling installer")
		}
		ai := m.Intents["ai"]
		if ai.Default || !slices.Contains(ai.Packages, "omp") {
			t.Fatal("omp must belong to the opt-in ai intent")
		}
		return
	}
	t.Fatal("omp missing from manifest")
}
