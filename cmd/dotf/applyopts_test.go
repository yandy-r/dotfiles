package main

import (
	"reflect"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

// mapGetenv returns a getenv closure backed by env, so parity tests never touch
// the process environment.
func mapGetenv(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}

// TestResolveApplyOptions exercises the flag/env parity matrix: each field is
// set once by its flag and once by its env equivalent, and the pass-through and
// homebrew tristate fields are checked.
func TestResolveApplyOptions(t *testing.T) {
	tests := []struct {
		name  string
		flags applyFlags
		env   map[string]string
		want  applyOptions
	}{
		{
			name:  "force flag implies headless",
			flags: applyFlags{Force: true},
			want:  applyOptions{Headless: true, Force: true},
		},
		{
			name: "non_interactive env implies headless only",
			env:  map[string]string{"NON_INTERACTIVE": "true"},
			want: applyOptions{Headless: true},
		},
		{
			name: "force_install env implies headless and force",
			env:  map[string]string{"FORCE_INSTALL": "true"},
			want: applyOptions{Headless: true, Force: true},
		},
		{
			name:  "headless flag alone",
			flags: applyFlags{Headless: true},
			want:  applyOptions{Headless: true},
		},
		{
			name:  "gaming flag",
			flags: applyFlags{Gaming: true},
			want:  applyOptions{Gaming: true},
		},
		{
			name: "gaming env",
			env:  map[string]string{"ENABLE_GAMING": "true"},
			want: applyOptions{Gaming: true},
		},
		{
			name:  "hyprland flag",
			flags: applyFlags{Hyprland: true},
			want:  applyOptions{Hyprland: true},
		},
		{
			name: "hyprland env",
			env:  map[string]string{"HYPRLAND": "true"},
			want: applyOptions{Hyprland: true},
		},
		{
			// --linux-desktop/LINUX_DESKTOP are the deprecated aliases: they
			// resolve into the same Hyprland field.
			name:  "linux-desktop flag",
			flags: applyFlags{LinuxDesktop: true},
			want:  applyOptions{Hyprland: true},
		},
		{
			name: "linux-desktop env",
			env:  map[string]string{"LINUX_DESKTOP": "true"},
			want: applyOptions{Hyprland: true},
		},
		{
			name:  "snapper flag",
			flags: applyFlags{Snapper: true},
			want:  applyOptions{Snapper: true},
		},
		{
			name: "snapper env",
			env:  map[string]string{"SNAPPER": "true"},
			want: applyOptions{Snapper: true},
		},
		{
			name:  "plasma flag",
			flags: applyFlags{Plasma: true},
			want:  applyOptions{Plasma: true},
		},
		{
			// The twin is the UNPREFIXED PLASMA: every apply twin is unprefixed
			// (a bootstrap.sh contract), and a one-off DOTF_ prefix here would
			// break resolveApplyOptions' uniform getenv(NAME) shape.
			name: "plasma env",
			env:  map[string]string{"PLASMA": "true"},
			want: applyOptions{Plasma: true},
		},
		{
			name:  "plasma flag and env together",
			flags: applyFlags{Plasma: true},
			env:   map[string]string{"PLASMA": "true"},
			want:  applyOptions{Plasma: true},
		},
		{
			name:  "homebrew false tristate",
			flags: applyFlags{Homebrew: "false"},
			want:  applyOptions{Homebrew: "false"},
		},
		{
			name:  "homebrew absent stays empty",
			flags: applyFlags{},
			want:  applyOptions{Homebrew: ""},
		},
		{
			name:  "dryrun and manifest pass through",
			flags: applyFlags{DryRun: true, Manifest: "/etc/manifest.toml"},
			want:  applyOptions{DryRun: true, Manifest: "/etc/manifest.toml"},
		},
		{
			name:  "no-user-config flag",
			flags: applyFlags{NoUserConfig: true},
			want:  applyOptions{NoUserConfig: true},
		},
		{
			name: "no-user-config env",
			env:  map[string]string{"DOTF_NO_USER_CONFIG": "true"},
			want: applyOptions{NoUserConfig: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveApplyOptions(tt.flags, mapGetenv(tt.env))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolveApplyOptions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestResolveApplyOptions_Precedence verifies flag > env is a logical OR: a flag
// set with its env unset stays on, and the env alone flips the field on.
func TestResolveApplyOptions_Precedence(t *testing.T) {
	// Flag on, env explicitly empty: still on.
	got := resolveApplyOptions(applyFlags{Gaming: true}, mapGetenv(map[string]string{"ENABLE_GAMING": ""}))
	if !got.Gaming {
		t.Errorf("Gaming = false with --gaming and ENABLE_GAMING empty, want true")
	}

	// Env alone: on.
	got = resolveApplyOptions(applyFlags{}, mapGetenv(map[string]string{"ENABLE_GAMING": "true"}))
	if !got.Gaming {
		t.Errorf("Gaming = false with ENABLE_GAMING=true and no flag, want true")
	}

	// Neither: off.
	got = resolveApplyOptions(applyFlags{}, mapGetenv(nil))
	if got.Gaming {
		t.Errorf("Gaming = true with no flag and no env, want false")
	}
}

// TestResolveApplyOptions_TruthinessLock locks the bootstrap contract that only
// the exact string "true" is truthy; "1", "yes", "TRUE", etc. must not flip.
func TestResolveApplyOptions_TruthinessLock(t *testing.T) {
	for _, v := range []string{"1", "yes", "TRUE", "True", "on", "y", ""} {
		t.Run("value="+v, func(t *testing.T) {
			env := map[string]string{
				"ENABLE_GAMING":       v,
				"NON_INTERACTIVE":     v,
				"FORCE_INSTALL":       v,
				"HYPRLAND":            v,
				"LINUX_DESKTOP":       v,
				"SNAPPER":             v,
				"PLASMA":              v,
				"DOTF_NO_USER_CONFIG": v,
			}
			got := resolveApplyOptions(applyFlags{}, mapGetenv(env))
			if got.Gaming || got.Headless || got.Force || got.Hyprland || got.Snapper || got.Plasma || got.NoUserConfig {
				t.Errorf("non-\"true\" value %q flipped a field: %+v", v, got)
			}
		})
	}
}

func testIntentsManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Intents: map[string]manifest.Intent{
			"base":      {},
			"dev":       {},
			"gaming":    {},
			"runtimes":  {},
			"ai":        {},
			"cloud":     {},
			"git-tools": {},
		},
	}
}

// TestResolveIntents covers the headless intent set: sorted default excluding
// opt-in intents, gaming appended when flagged, explicit --intent
// narrowing, and order-preserving dedupe.
func TestResolveIntents(t *testing.T) {
	m := testIntentsManifest()

	tests := []struct {
		name string
		opts applyOptions
		want []string
	}{
		{
			name: "default excludes all opt-in intents (gaming + parity bundles), sorted",
			opts: applyOptions{},
			want: []string{"base", "dev"},
		},
		{
			name: "provisioning-parity bundle opted in via --intent",
			opts: applyOptions{Intents: []string{"runtimes"}},
			want: []string{"runtimes"},
		},
		{
			name: "gaming appended when flagged",
			opts: applyOptions{Gaming: true},
			want: []string{"base", "dev", "gaming"},
		},
		{
			name: "explicit intents narrow and skip manifest scan",
			opts: applyOptions{Intents: []string{"custom"}},
			want: []string{"custom"},
		},
		{
			name: "explicit intents plus gaming flag",
			opts: applyOptions{Intents: []string{"custom"}, Gaming: true},
			want: []string{"custom", "gaming"},
		},
		{
			name: "dedupe preserves first-seen order",
			opts: applyOptions{Intents: []string{"base", "gaming"}, Gaming: true},
			want: []string{"base", "gaming"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveIntents(m, tt.opts)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolveIntents() = %v, want %v", got, tt.want)
			}
		})
	}
}
