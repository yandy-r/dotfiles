package manifest_test

import (
	"slices"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

// Pin the selection, probe and lifecycle contract for the Antigravity CLI: one
// cross-platform entry on the official installer, probed at the official install
// location so a foreign (e.g. Homebrew cask) agy never reports converged.
func TestEmbeddedManifest_Agy(t *testing.T) {
	m := loadEmbeddedManifest(t)
	for _, p := range m.Packages {
		if p.Name != "agy" {
			continue
		}
		if p.Backend != manifest.BackendCustomScript || p.Script != "bin/install-agy.sh" {
			t.Fatalf("agy must use its official-installer wrapper: %+v", p)
		}
		if p.OS != "" || len(p.Arch) != 0 {
			t.Fatalf("agy must apply on every macOS/Linux host: os=%q arch=%v", p.OS, p.Arch)
		}
		if p.InstalledWhen == nil || p.InstalledWhen.Path != "~/.local/bin/agy" || len(p.Provides) != 0 {
			t.Fatalf("agy probe = %+v provides=%v, want installed_when path ~/.local/bin/agy", p.InstalledWhen, p.Provides)
		}
		if !slices.Equal(p.Actions, []string{"update", "remove"}) {
			t.Fatalf("agy actions = %v", p.Actions)
		}
		if p.EffectiveClass() != manifest.ClassRolling || p.Justification == "" {
			t.Fatal("agy must justify its rolling installer")
		}
		ai := m.Intents["ai"]
		if ai.Default || !slices.Contains(ai.Packages, "agy") {
			t.Fatal("agy must belong to the opt-in ai intent")
		}
		return
	}
	t.Fatal("agy missing from manifest")
}
