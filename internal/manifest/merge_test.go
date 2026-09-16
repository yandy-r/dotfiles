package manifest_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

// pkgPkg returns a minimal valid pkg-backend package (no per-backend required
// fields), the simplest building block for merge tests.
func pkgPkg(name string) manifest.Package {
	return manifest.Package{Name: name, Backend: manifest.BackendPkg}
}

// overlayLayer wraps m as a Layer labelled name.
func overlayLayer(name string, m *manifest.Manifest) manifest.Layer {
	return manifest.Layer{Name: name, Manifest: m}
}

func TestMerge_AppendsNewPackages(t *testing.T) {
	base := &manifest.Manifest{Packages: []manifest.Package{pkgPkg("git")}}
	overlay := &manifest.Manifest{Packages: []manifest.Package{pkgPkg("ripgrep")}}

	merged, collisions, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if len(collisions) != 0 {
		t.Errorf("collisions = %+v, want none for a pure append", collisions)
	}
	if got := packageNames(merged); !reflect.DeepEqual(got, []string{"git", "ripgrep"}) {
		t.Errorf("merged packages = %v, want [git ripgrep]", got)
	}
	// base must be untouched (Merge is pure).
	if got := packageNames(base); !reflect.DeepEqual(got, []string{"git"}) {
		t.Errorf("base mutated: packages = %v, want [git]", got)
	}
}

func TestMerge_PackageLastWinsRecordsCollision(t *testing.T) {
	base := &manifest.Manifest{Packages: []manifest.Package{
		{Name: "tool", Backend: manifest.BackendPkg, Names: map[string]string{"apt": "tool-base"}},
	}}
	overlay := &manifest.Manifest{Packages: []manifest.Package{
		{Name: "tool", Backend: manifest.BackendPkg, Names: map[string]string{"apt": "tool-user"}},
	}}

	merged, collisions, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if len(merged.Packages) != 1 {
		t.Fatalf("merged packages = %d, want 1 (override, not append)", len(merged.Packages))
	}
	if got := merged.Packages[0].Names["apt"]; got != "tool-user" {
		t.Errorf("merged tool apt name = %q, want tool-user (overlay wins)", got)
	}
	want := []manifest.Collision{{Kind: "package", Key: "tool", Won: "frag.toml", Shadowed: "base"}}
	if !reflect.DeepEqual(collisions, want) {
		t.Errorf("collisions = %+v, want %+v", collisions, want)
	}
}

// mod returns a minimal valid single-step custom-script module named name.
func mod(name, script string) manifest.Module {
	return manifest.Module{Name: name, Steps: []manifest.ModuleStep{{CustomScript: script}}}
}

func TestMerge_AppendsNewModules(t *testing.T) {
	base := &manifest.Manifest{Modules: []manifest.Module{mod("fingerprint", "bin/a.sh")}}
	overlay := &manifest.Manifest{Modules: []manifest.Module{mod("onedrive", "bin/b.sh")}}

	merged, collisions, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if len(collisions) != 0 {
		t.Errorf("collisions = %+v, want none for a pure module append", collisions)
	}
	if len(merged.Modules) != 2 || merged.Modules[0].Name != "fingerprint" || merged.Modules[1].Name != "onedrive" {
		t.Errorf("merged modules = %+v, want [fingerprint onedrive]", merged.Modules)
	}
	// base must be untouched (Merge is pure).
	if len(base.Modules) != 1 {
		t.Errorf("base mutated: modules = %d, want 1", len(base.Modules))
	}
}

func TestMerge_ModuleLastWinsRecordsCollision(t *testing.T) {
	base := &manifest.Manifest{Modules: []manifest.Module{mod("fingerprint", "bin/base.sh")}}
	overlay := &manifest.Manifest{Modules: []manifest.Module{mod("fingerprint", "bin/user.sh")}}

	merged, collisions, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if len(merged.Modules) != 1 {
		t.Fatalf("merged modules = %d, want 1 (override, not append)", len(merged.Modules))
	}
	if got := merged.Modules[0].Steps[0].CustomScript; got != "bin/user.sh" {
		t.Errorf("merged fingerprint script = %q, want bin/user.sh (overlay wins)", got)
	}
	want := []manifest.Collision{{Kind: "module", Key: "fingerprint", Won: "frag.toml", Shadowed: "base"}}
	if !reflect.DeepEqual(collisions, want) {
		t.Errorf("collisions = %+v, want %+v", collisions, want)
	}
}

func TestMerge_IntentDeepMergeExtendsPackages(t *testing.T) {
	base := &manifest.Manifest{
		Packages: []manifest.Package{pkgPkg("a"), pkgPkg("b"), pkgPkg("c")},
		Intents:  map[string]manifest.Intent{"dev": {Description: "base dev", Packages: []string{"a", "b"}}},
	}
	// Overlay overlaps on "b" so the union's dedup branch is exercised.
	overlay := &manifest.Manifest{
		Intents: map[string]manifest.Intent{"dev": {Packages: []string{"b", "c"}}},
	}

	merged, collisions, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if len(collisions) != 0 {
		t.Errorf("collisions = %+v, want none for a pure extend (no description override)", collisions)
	}
	got := merged.Intents["dev"]
	if !reflect.DeepEqual(got.Packages, []string{"a", "b", "c"}) {
		t.Errorf("dev packages = %v, want [a b c] (extend + dedup: b appears once, first-seen order)", got.Packages)
	}
	if got.Description != "base dev" {
		t.Errorf("dev description = %q, want base dev (empty overlay description keeps base)", got.Description)
	}
	// base intent slice must be untouched.
	if bp := base.Intents["dev"].Packages; !reflect.DeepEqual(bp, []string{"a", "b"}) {
		t.Errorf("base dev packages mutated: %v, want [a b]", bp)
	}
}

// TestMerge_IntentProvenanceAcrossOverlays proves that when overlay 1 introduces
// a brand-new intent and overlay 2 overrides its description, the recorded
// Collision.Shadowed names overlay 1 (not an empty string) — i.e. adoption
// stamps provenance like package adoption does.
func TestMerge_IntentProvenanceAcrossOverlays(t *testing.T) {
	base := &manifest.Manifest{Packages: []manifest.Package{pkgPkg("a")}}
	ov1 := &manifest.Manifest{Intents: map[string]manifest.Intent{"x": {Description: "from one", Packages: []string{"a"}}}}
	ov2 := &manifest.Manifest{Intents: map[string]manifest.Intent{"x": {Description: "from two", Packages: []string{"a"}}}}

	_, collisions, err := manifest.Merge(base, overlayLayer("01-foo.toml", ov1), overlayLayer("02-bar.toml", ov2))
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}

	var found bool
	for _, c := range collisions {
		if c.Kind == "intent" && c.Key == "x" {
			found = true
			if c.Won != "02-bar.toml" || c.Shadowed != "01-foo.toml" {
				t.Errorf("intent collision = %+v, want won=02-bar.toml shadowed=01-foo.toml", c)
			}
		}
	}
	if !found {
		t.Errorf("no intent collision recorded for x; collisions = %+v", collisions)
	}
}

func TestMerge_NewOverlayIntent(t *testing.T) {
	base := &manifest.Manifest{Packages: []manifest.Package{pkgPkg("a")}}
	overlay := &manifest.Manifest{
		Intents: map[string]manifest.Intent{"extra": {Description: "user extra", Packages: []string{"a"}}},
	}

	merged, _, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if _, ok := merged.Intents["extra"]; !ok {
		t.Errorf("merged intents missing brand-new overlay intent %q", "extra")
	}
}

func TestMerge_OverlayIntentReferencesBaseOnlyPackageValidates(t *testing.T) {
	// The overlay intent references a package that only the BASE defines: this
	// must validate OK because cross-references resolve against the merged whole,
	// proving decode-not-Parse deferral of intent validation.
	base := &manifest.Manifest{Packages: []manifest.Package{pkgPkg("basePkg")}}
	overlay := &manifest.Manifest{
		Intents: map[string]manifest.Intent{"x": {Packages: []string{"basePkg"}}},
	}

	if _, _, err := manifest.Merge(base, overlayLayer("frag.toml", overlay)); err != nil {
		t.Errorf("Merge() error = %v, want nil (overlay intent may reference a base-only package)", err)
	}
}

func TestMerge_DefaultIsORed(t *testing.T) {
	base := &manifest.Manifest{
		Packages: []manifest.Package{pkgPkg("a")},
		Intents:  map[string]manifest.Intent{"dev": {Default: false, Packages: []string{"a"}}},
	}
	overlay := &manifest.Manifest{
		Intents: map[string]manifest.Intent{"dev": {Default: true}},
	}

	merged, collisions, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if !merged.Intents["dev"].Default {
		t.Errorf("dev default = false, want true (base.Default || overlay.Default)")
	}
	if len(collisions) != 0 {
		t.Errorf("collisions = %+v, want none (default promotion is additive, not a collision)", collisions)
	}
}

func TestMerge_DescriptionOverrideRecordsCollision(t *testing.T) {
	base := &manifest.Manifest{
		Packages: []manifest.Package{pkgPkg("a")},
		Intents:  map[string]manifest.Intent{"dev": {Description: "base dev", Packages: []string{"a"}}},
	}
	overlay := &manifest.Manifest{
		Intents: map[string]manifest.Intent{"dev": {Description: "user dev"}},
	}

	merged, collisions, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if got := merged.Intents["dev"].Description; got != "user dev" {
		t.Errorf("dev description = %q, want user dev (non-empty overlay description wins)", got)
	}
	want := []manifest.Collision{{Kind: "intent", Key: "dev", Won: "frag.toml", Shadowed: "base"}}
	if !reflect.DeepEqual(collisions, want) {
		t.Errorf("collisions = %+v, want %+v", collisions, want)
	}
}

func TestMerge_SymlinkDestCollisionIsHardError(t *testing.T) {
	base := &manifest.Manifest{
		Packages: []manifest.Package{pkgPkg("a")},
		Symlinks: []manifest.Symlink{{Src: "repo/gitconfig", Dest: ".gitconfig", Type: "file"}},
	}
	overlay := &manifest.Manifest{
		Symlinks: []manifest.Symlink{{Src: "user/gitconfig", Dest: ".gitconfig", Type: "file"}},
	}

	_, _, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err == nil {
		t.Fatal("Merge() error = nil, want a hard error on symlink Dest collision")
	}
	if !strings.Contains(err.Error(), ".gitconfig") {
		t.Errorf("Merge() error = %v, want it to name the colliding dest", err)
	}
}

func TestMerge_SymlinkAppendsWithoutCollision(t *testing.T) {
	base := &manifest.Manifest{
		Packages: []manifest.Package{pkgPkg("a")},
		Symlinks: []manifest.Symlink{{Src: "repo/gitconfig", Dest: ".gitconfig", Type: "file"}},
	}
	overlay := &manifest.Manifest{
		Symlinks: []manifest.Symlink{{Src: "user/vimrc", Dest: ".vimrc", Type: "file"}},
	}

	merged, _, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if len(merged.Symlinks) != 2 {
		t.Errorf("merged symlinks = %d, want 2 (append, distinct dests)", len(merged.Symlinks))
	}
}

func TestMerge_SysconfigFieldWiseOverride(t *testing.T) {
	base := &manifest.Manifest{
		Packages:  []manifest.Package{pkgPkg("a")},
		Sysconfig: &manifest.Sysconfig{Timezone: "UTC", Locale: "en_US.UTF-8", Firewall: "ufw", SSHServer: "off"},
	}
	overlay := &manifest.Manifest{
		Sysconfig: &manifest.Sysconfig{Timezone: "America/New_York", Shell: "zsh", SSHServer: "on"},
	}

	merged, _, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	got := merged.Sysconfig
	if got.Timezone != "America/New_York" {
		t.Errorf("timezone = %q, want America/New_York (overlay non-empty wins)", got.Timezone)
	}
	if got.Shell != "zsh" {
		t.Errorf("shell = %q, want zsh (overlay adds it)", got.Shell)
	}
	if got.Locale != "en_US.UTF-8" {
		t.Errorf("locale = %q, want en_US.UTF-8 (empty overlay field keeps base)", got.Locale)
	}
	if got.Firewall != "ufw" {
		t.Errorf("firewall = %q, want ufw (empty overlay field keeps base)", got.Firewall)
	}
	if got.SSHServer != "on" {
		t.Errorf("ssh_server = %q, want on (overlay non-empty wins; it was silently dropped before)", got.SSHServer)
	}
	// base sysconfig must be untouched (deep copy).
	if base.Sysconfig.Timezone != "UTC" {
		t.Errorf("base sysconfig mutated: timezone = %q, want UTC", base.Sysconfig.Timezone)
	}
}

func TestMerge_SysconfigNilBaseAdoptsOverlay(t *testing.T) {
	base := &manifest.Manifest{Packages: []manifest.Package{pkgPkg("a")}}
	overlay := &manifest.Manifest{Sysconfig: &manifest.Sysconfig{Firewall: "firewalld"}}

	merged, _, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if merged.Sysconfig == nil || merged.Sysconfig.Firewall != "firewalld" {
		t.Errorf("merged sysconfig = %+v, want firewalld adopted from overlay", merged.Sysconfig)
	}
}

func TestMerge_ValidatesMergedResult(t *testing.T) {
	base := &manifest.Manifest{Packages: []manifest.Package{pkgPkg("a")}}
	// Missing checksum source → the merged manifest must fail Validate.
	overlay := &manifest.Manifest{Packages: []manifest.Package{
		{Name: "bad", Backend: manifest.BackendGitHubRelease, Repo: "o/r", Tag: "v1"},
	}}

	_, _, err := manifest.Merge(base, overlayLayer("frag.toml", overlay))
	if err == nil {
		t.Fatal("Merge() error = nil, want a validation error for an invalid overlay package")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("Merge() error = %v, want it to mention the missing checksum source", err)
	}
}

func TestMerge_TwoOverlaysDeterministicLastWins(t *testing.T) {
	base := &manifest.Manifest{Packages: []manifest.Package{
		{Name: "tool", Backend: manifest.BackendPkg, Names: map[string]string{"apt": "base"}},
	}}
	first := &manifest.Manifest{Packages: []manifest.Package{
		{Name: "tool", Backend: manifest.BackendPkg, Names: map[string]string{"apt": "first"}},
	}}
	second := &manifest.Manifest{Packages: []manifest.Package{
		{Name: "tool", Backend: manifest.BackendPkg, Names: map[string]string{"apt": "second"}},
	}}

	merged, collisions, err := manifest.Merge(base,
		overlayLayer("01-first.toml", first),
		overlayLayer("02-second.toml", second),
	)
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if got := merged.Packages[0].Names["apt"]; got != "second" {
		t.Errorf("merged tool = %q, want second (last overlay wins)", got)
	}
	want := []manifest.Collision{
		{Kind: "package", Key: "tool", Won: "01-first.toml", Shadowed: "base"},
		{Kind: "package", Key: "tool", Won: "02-second.toml", Shadowed: "01-first.toml"},
	}
	if !reflect.DeepEqual(collisions, want) {
		t.Errorf("collisions = %+v, want %+v (provenance chains base→first→second)", collisions, want)
	}
}

func TestMerge_NoOverlaysDeepEqualsBase(t *testing.T) {
	base := &manifest.Manifest{
		Packages:  []manifest.Package{pkgPkg("a"), pkgPkg("b")},
		Intents:   map[string]manifest.Intent{"dev": {Description: "d", Packages: []string{"a"}}},
		Symlinks:  []manifest.Symlink{{Src: "s", Dest: ".d", Type: "file"}},
		Sysconfig: &manifest.Sysconfig{Timezone: "UTC"},
	}

	merged, collisions, err := manifest.Merge(base)
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if len(collisions) != 0 {
		t.Errorf("collisions = %+v, want none", collisions)
	}
	if !reflect.DeepEqual(merged, base) {
		t.Errorf("Merge(base) = %+v, want deep-equal to base %+v", merged, base)
	}
}

// packageNames returns the ordered package names of m, for order-sensitive
// assertions.
func packageNames(m *manifest.Manifest) []string {
	names := make([]string, len(m.Packages))
	for i, p := range m.Packages {
		names[i] = p.Name
	}
	return names
}
