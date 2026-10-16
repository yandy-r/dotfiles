package manifest_test

import (
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

// customScriptBaseline is the sanctioned number of custom-script escape-hatch
// packages in the embedded manifest. custom-script is the LAST-RESORT backend
// (see docs/reference/manifest-backends.md): the manifest reaches for it only
// when none of the native backends (pkg, aur, apt-repo, toolchain,
// github-release, script-url, git-clone) can express the install.
//
// This is an EXACT-match tripwire, not a one-directional ceiling: adding OR
// removing a custom-script entry must move this number in the same commit, so
// every change to the escape-hatch population is a visible, reviewed, deliberate
// act. A bare `<=` ceiling would go stale — after migrating a script to a native
// backend (count 16→15) it would still pass at 16, silently re-admitting a
// future custom-script without review.
const customScriptBaseline = 21

// customScriptCount returns the number of custom-script packages in m.
func customScriptCount(m *manifest.Manifest) int {
	n := 0
	for _, p := range m.Packages {
		if p.Backend == manifest.BackendCustomScript {
			n++
		}
	}
	return n
}

// TestCustomScriptRatchet is the escape-hatch growth tripwire (#54): the number
// of custom-script packages in the embedded manifest must exactly equal the
// committed baseline. It freezes growth of the custom-script hatch now that the
// native-backend and setup-step tiers exist — the native path is the expected
// one, so every new escape-hatch entry must be a conscious, reviewed bump of
// customScriptBaseline, and every migration away must ratchet it down.
func TestCustomScriptRatchet(t *testing.T) {
	m := loadEmbeddedManifest(t)
	got := customScriptCount(m)
	switch {
	case got > customScriptBaseline:
		t.Fatalf("custom-script package count = %d, baseline = %d: a new custom-script "+
			"entry was added. Prefer a native backend first "+
			"(see docs/reference/manifest-backends.md). If custom-script is truly "+
			"required, keep its class+justification and set customScriptBaseline = %d.",
			got, customScriptBaseline, got)
	case got < customScriptBaseline:
		t.Fatalf("custom-script package count = %d, baseline = %d: a custom-script entry "+
			"was removed or migrated to a native backend — ratchet the baseline down by "+
			"setting customScriptBaseline = %d.", got, customScriptBaseline, got)
	}
}

// TestCustomScriptEntriesJustified re-asserts, at the manifest level, that every
// custom-script entry declares the governance marker: a non-empty Class and a
// non-empty Justification. validateClass already enforces this at parse time
// (internal/manifest/validate.go), so a bare load would already fail; this test
// is defense-in-depth with a governance-focused message that points at the
// escape-hatch rules directly, so a failure teaches the "native before
// custom-script" fence instead of a bare validation error.
func TestCustomScriptEntriesJustified(t *testing.T) {
	m := loadEmbeddedManifest(t)
	for _, p := range m.Packages {
		if p.Backend != manifest.BackendCustomScript {
			continue
		}
		if p.Class == "" {
			t.Errorf("custom-script package %q has no class; the escape hatch requires an "+
				"explicit class (pinned|rolling|managed) — see docs/reference/manifest-backends.md", p.Name)
		}
		if strings.TrimSpace(p.Justification) == "" {
			t.Errorf("custom-script package %q has no justification; the escape hatch requires a "+
				"justification — see docs/reference/manifest-backends.md", p.Name)
		}
	}
}
