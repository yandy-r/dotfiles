package manifest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

// writeFragment writes content to configRoot/packages.d/name, creating the dir.
func writeFragment(t *testing.T, configRoot, name, content string) {
	t.Helper()
	dir := filepath.Join(configRoot, "packages.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
}

const overlayPkgFragment = `[[package]]
name = "ripgrep"
backend = "pkg"
`

func TestDiscoverOverlays_SortedOrder(t *testing.T) {
	root := t.TempDir()
	writeFragment(t, root, "02-second.toml", "[[package]]\nname = \"second\"\nbackend = \"pkg\"\n")
	writeFragment(t, root, "01-first.toml", "[[package]]\nname = \"first\"\nbackend = \"pkg\"\n")

	layers, err := manifest.DiscoverOverlays(root)
	if err != nil {
		t.Fatalf("DiscoverOverlays() error = %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("layers = %d, want 2", len(layers))
	}
	if !strings.HasSuffix(layers[0].Name, "01-first.toml") {
		t.Errorf("layers[0].Name = %q, want it to end with 01-first.toml (sorted)", layers[0].Name)
	}
	if !strings.HasSuffix(layers[1].Name, "02-second.toml") {
		t.Errorf("layers[1].Name = %q, want it to end with 02-second.toml (sorted)", layers[1].Name)
	}
}

func TestDiscoverOverlays_MissingDirIsNilNil(t *testing.T) {
	root := t.TempDir() // exists, but has no packages.d
	layers, err := manifest.DiscoverOverlays(root)
	if err != nil {
		t.Fatalf("DiscoverOverlays() error = %v, want nil (absent overlay is the default)", err)
	}
	if layers != nil {
		t.Errorf("layers = %+v, want nil for a missing packages.d dir", layers)
	}
}

func TestDiscoverOverlays_EmptyConfigRootIsNilNil(t *testing.T) {
	layers, err := manifest.DiscoverOverlays("")
	if err != nil || layers != nil {
		t.Errorf("DiscoverOverlays(\"\") = (%+v, %v), want (nil, nil)", layers, err)
	}
}

func TestDiscoverOverlays_StrictDecodeErrorNamesFile(t *testing.T) {
	root := t.TempDir()
	writeFragment(t, root, "bad.toml", "[[package]]\nname = \"x\"\nbackend = \"pkg\"\nbogus_field = true\n")

	_, err := manifest.DiscoverOverlays(root)
	if err == nil {
		t.Fatal("DiscoverOverlays() error = nil, want a strict-decode error for an unknown field")
	}
	if !strings.Contains(err.Error(), "bad.toml") {
		t.Errorf("DiscoverOverlays() error = %v, want it to name the offending fragment", err)
	}
}

func TestDiscoverOverlays_StampsScriptRoot(t *testing.T) {
	root := t.TempDir()
	writeFragment(t, root, "scripts.toml", `[[package]]
name = "mytool"
backend = "custom-script"
script = "bin/install-mytool.sh"
class = "rolling"
justification = "user-local tool"
`)

	layers, err := manifest.DiscoverOverlays(root)
	if err != nil {
		t.Fatalf("DiscoverOverlays() error = %v", err)
	}
	if len(layers) != 1 || len(layers[0].Manifest.Packages) != 1 {
		t.Fatalf("layers = %+v, want one fragment with one package", layers)
	}
	if got := layers[0].Manifest.Packages[0].ScriptRoot; got != root {
		t.Errorf("ScriptRoot = %q, want the config root %q", got, root)
	}
}

func TestDiscoverOverlays_IgnoresNonToml(t *testing.T) {
	root := t.TempDir()
	writeFragment(t, root, "keep.toml", overlayPkgFragment)
	writeFragment(t, root, "notes.txt", "not a fragment")

	layers, err := manifest.DiscoverOverlays(root)
	if err != nil {
		t.Fatalf("DiscoverOverlays() error = %v", err)
	}
	if len(layers) != 1 {
		t.Errorf("layers = %d, want 1 (only *.toml discovered)", len(layers))
	}
}

// embeddedBase returns an fstest.MapFS holding a minimal valid base manifest at
// manifest/packages.toml, mirroring how the CLI passes dotfiles.ManifestFS.
func embeddedBase(content string) fstest.MapFS {
	return fstest.MapFS{"manifest/packages.toml": {Data: []byte(content)}}
}

const resolveBaseManifest = `[[package]]
name = "git"
backend = "pkg"

[intents.dev]
description = "developer tools"
packages = ["git"]
`

func TestResolve_OverlayMergesOntoBase(t *testing.T) {
	root := t.TempDir()
	writeFragment(t, root, "extra.toml", overlayPkgFragment)

	m, err := manifest.Resolve("", "", embeddedBase(resolveBaseManifest),
		manifest.ResolveOptions{ConfigRoot: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if names := packageNames(m); !contains(names, "git") || !contains(names, "ripgrep") {
		t.Errorf("packages = %v, want both base git and overlay ripgrep", names)
	}
}

func TestResolve_NoUserConfigShortCircuitsToBase(t *testing.T) {
	root := t.TempDir()
	writeFragment(t, root, "extra.toml", overlayPkgFragment)

	m, err := manifest.Resolve("", "", embeddedBase(resolveBaseManifest),
		manifest.ResolveOptions{ConfigRoot: root, NoUserConfig: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if names := packageNames(m); contains(names, "ripgrep") {
		t.Errorf("packages = %v, want base-only (overlay disabled by NoUserConfig)", names)
	}
}

func TestResolve_MissingOverlayIdenticalToBase(t *testing.T) {
	root := t.TempDir() // no packages.d

	m, err := manifest.Resolve("", "", embeddedBase(resolveBaseManifest),
		manifest.ResolveOptions{ConfigRoot: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if names := packageNames(m); len(names) != 1 || names[0] != "git" {
		t.Errorf("packages = %v, want base-only [git] when no overlay present", names)
	}
}

func TestResolve_OverlayDecodeErrorFailsFast(t *testing.T) {
	root := t.TempDir()
	writeFragment(t, root, "typo.toml", "[[package]]\nname = \"x\"\nbackend = \"pkg\"\nnope = 1\n")

	_, err := manifest.Resolve("", "", embeddedBase(resolveBaseManifest),
		manifest.ResolveOptions{ConfigRoot: root})
	if err == nil {
		t.Fatal("Resolve() error = nil, want a fail-fast decode error for a malformed fragment")
	}
	if !strings.Contains(err.Error(), "typo.toml") {
		t.Errorf("Resolve() error = %v, want it to name the offending fragment", err)
	}
}

func TestResolve_LogfReceivesOneLinePerCollision(t *testing.T) {
	root := t.TempDir()
	// Overlay overrides the base "git" package → one package Collision logged.
	writeFragment(t, root, "override.toml", "[[package]]\nname = \"git\"\nbackend = \"pkg\"\nnames = { apt = \"git-user\" }\n")

	var logged []string
	m, err := manifest.Resolve("", "", embeddedBase(resolveBaseManifest),
		manifest.ResolveOptions{
			ConfigRoot: root,
			Logf:       func(format string, _ ...any) { logged = append(logged, format) },
		})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(logged) != 1 {
		t.Errorf("Logf calls = %d, want exactly 1 (one collision announced)", len(logged))
	}
	if m.Packages[0].Names["apt"] != "git-user" {
		t.Errorf("git apt name = %q, want git-user (overlay override applied)", m.Packages[0].Names["apt"])
	}
}

// contains reports whether s appears in list.
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// TestDiscoverOverlays_StampsModuleStepScriptRoot pins YAN-1418: every step of
// an overlay [[module]] carries the overlay's configRoot as its ScriptRoot, the
// same provenance overlay packages get, so `dotf setup` resolves a module
// custom-script under ~/.config/dotf rather than the repo root.
func TestDiscoverOverlays_StampsModuleStepScriptRoot(t *testing.T) {
	root := t.TempDir()
	writeFragment(t, root, "mods.toml", `[[module]]
name = "mine"
description = "user module"
steps = [
  { custom-script = "bin/foo.sh" },
  { custom-script = "bin/bar.sh" },
]
`)

	layers, err := manifest.DiscoverOverlays(root)
	if err != nil {
		t.Fatalf("DiscoverOverlays() error = %v", err)
	}
	if len(layers) != 1 || len(layers[0].Manifest.Modules) != 1 {
		t.Fatalf("layers = %+v, want one layer with one module", layers)
	}
	steps := layers[0].Manifest.Modules[0].Steps
	if len(steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(steps))
	}
	for i, s := range steps {
		if s.ScriptRoot != root {
			t.Errorf("steps[%d].ScriptRoot = %q, want %q (overlay configRoot)", i, s.ScriptRoot, root)
		}
	}
}

// TestDiscoverOverlays_ModuleStepScriptRootNotUserSettable proves a fragment
// cannot pick its own step root: ModuleStep.ScriptRoot is toml:"-", so a
// `script_root` key is an unknown field the strict decoder rejects.
func TestDiscoverOverlays_ModuleStepScriptRootNotUserSettable(t *testing.T) {
	root := t.TempDir()
	writeFragment(t, root, "smuggle.toml", `[[module]]
name = "mine"
steps = [{ custom-script = "bin/foo.sh", script_root = "/etc" }]
`)

	_, err := manifest.DiscoverOverlays(root)
	if err == nil {
		t.Fatal("DiscoverOverlays() error = nil, want a strict-decode error for script_root on a step")
	}
	if !strings.Contains(err.Error(), "smuggle.toml") {
		t.Errorf("DiscoverOverlays() error = %v, want it to name the offending fragment", err)
	}
}

// TestMerge_BaseModuleStepsStayRepoRooted pins that only overlay modules are
// stamped: a base-manifest module keeps an empty step ScriptRoot (repo-rooted)
// through the merge, while the overlay module's steps keep the overlay root.
func TestMerge_BaseModuleStepsStayRepoRooted(t *testing.T) {
	base, err := manifest.LoadFS(embeddedBase(`[[module]]
name = "base-mod"
steps = [{ custom-script = "bin/base.sh" }]
`), "manifest/packages.toml")
	if err != nil {
		t.Fatalf("LoadFS() error = %v", err)
	}

	root := t.TempDir()
	writeFragment(t, root, "mods.toml", "[[module]]\nname = \"user-mod\"\nsteps = [{ custom-script = \"bin/user.sh\" }]\n")
	layers, err := manifest.DiscoverOverlays(root)
	if err != nil {
		t.Fatalf("DiscoverOverlays() error = %v", err)
	}

	merged, _, err := manifest.Merge(base, layers...)
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}
	got := map[string]string{}
	for _, mod := range merged.Modules {
		got[mod.Name] = mod.Steps[0].ScriptRoot
	}
	if got["base-mod"] != "" {
		t.Errorf("base-mod step ScriptRoot = %q, want empty (repo-rooted)", got["base-mod"])
	}
	if got["user-mod"] != root {
		t.Errorf("user-mod step ScriptRoot = %q, want %q", got["user-mod"], root)
	}
}
