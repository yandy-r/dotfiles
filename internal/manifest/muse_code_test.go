package manifest_test

import (
	"slices"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

// Pin the per-OS native routes for Meta's Muse Code CLI: the official installer
// wrapper on every Linux distro (probed at the official launcher location so a
// foreign muse never reports converged) and the official Homebrew cask on macOS,
// both in the opt-in ai intent and selected together by `--only muse-code`.
func TestEmbeddedManifest_MuseCode(t *testing.T) {
	m := loadEmbeddedManifest(t)
	byName := map[string]manifest.Package{}
	for _, p := range m.Packages {
		byName[p.Name] = p
	}

	linux, ok := byName["muse-code"]
	if !ok {
		t.Fatal("muse-code missing from manifest")
	}
	if linux.Backend != manifest.BackendCustomScript || linux.Script != "bin/install-muse-code.sh" {
		t.Fatalf("muse-code must use its official-installer wrapper: %+v", linux)
	}
	if linux.OS != "linux" || len(linux.ExcludeOS) != 0 {
		t.Fatalf("muse-code must cover every Linux distro: os=%q exclude_os=%v", linux.OS, linux.ExcludeOS)
	}
	if !slices.Equal(linux.Arch, []string{"x86_64", "arm64"}) {
		t.Fatalf("muse-code arch = %v", linux.Arch)
	}
	if linux.InstalledWhen == nil || linux.InstalledWhen.Path != "~/.local/bin/muse" || len(linux.Provides) != 0 {
		t.Fatalf("muse-code probe = %+v provides=%v, want installed_when path ~/.local/bin/muse", linux.InstalledWhen, linux.Provides)
	}
	if !slices.Equal(linux.Actions, []string{"update", "remove"}) {
		t.Fatalf("muse-code actions = %v", linux.Actions)
	}
	if linux.EffectiveClass() != manifest.ClassRolling || linux.Justification == "" {
		t.Fatal("muse-code must justify its rolling installer")
	}

	mac, ok := byName["muse-code-macos"]
	if !ok {
		t.Fatal("muse-code-macos missing from manifest")
	}
	if mac.Backend != manifest.BackendPkg || mac.OS != "macos" || mac.Names["homebrew"] != "muse-code" {
		t.Fatalf("muse-code-macos must be the official homebrew cask: %+v", mac)
	}

	ai := m.Intents["ai"]
	if ai.Default || !slices.Contains(ai.Packages, "muse-code") || !slices.Contains(ai.Packages, "muse-code-macos") {
		t.Fatal("muse-code and muse-code-macos must belong to the opt-in ai intent")
	}
	for name, intent := range m.Intents {
		if intent.Default && (slices.Contains(intent.Packages, "muse-code") || slices.Contains(intent.Packages, "muse-code-macos")) {
			t.Fatalf("muse-code must stay opt-in; found in default intent %q", name)
		}
	}
}
