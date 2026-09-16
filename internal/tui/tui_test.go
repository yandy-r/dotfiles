package tui

import (
	"errors"
	"os"
	"reflect"
	"slices"
	"testing"

	"charm.land/huh/v2"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

// TestAbortKeyMap asserts the wizard's form-level Quit binding fires on both
// Ctrl+C and Esc, so the user can abort from any step. huh binds Quit to ctrl+c
// only by default; the Esc addition is what makes "graceful exit on Esc" work.
func TestAbortKeyMap(t *testing.T) {
	got := abortKeyMap().Quit.Keys()
	for _, want := range []string{"ctrl+c", "esc"} {
		if !slices.Contains(got, want) {
			t.Errorf("abortKeyMap Quit keys = %v, want to contain %q", got, want)
		}
	}
}

// TestAbortKeyMapDisablesFiltering guards the Esc collision: huh's Select and
// MultiSelect bind Esc to set/clear an active "/" filter, and the form-level
// Quit(esc) shadows that binding. Filtering must therefore be disabled so Esc is
// only ever the abort, never a silently-shadowed filter-clear that aborts the
// wizard mid-filter.
func TestAbortKeyMapDisablesFiltering(t *testing.T) {
	km := abortKeyMap()
	bindings := map[string]bool{
		"Select.Filter":           km.Select.Filter.Enabled(),
		"Select.SetFilter":        km.Select.SetFilter.Enabled(),
		"Select.ClearFilter":      km.Select.ClearFilter.Enabled(),
		"MultiSelect.Filter":      km.MultiSelect.Filter.Enabled(),
		"MultiSelect.SetFilter":   km.MultiSelect.SetFilter.Enabled(),
		"MultiSelect.ClearFilter": km.MultiSelect.ClearFilter.Enabled(),
	}
	for name, enabled := range bindings {
		if enabled {
			t.Errorf("abortKeyMap %s is enabled; want disabled so Esc cannot be shadowed by filter handling", name)
		}
	}
}

// TestErrAbortedAliasesHuh guards the contract that tui.ErrAborted is huh's
// abort sentinel, so cmd/dotf can classify a wizard cancellation via
// errors.Is(err, tui.ErrAborted) without importing huh directly.
func TestErrAbortedAliasesHuh(t *testing.T) {
	if !errors.Is(ErrAborted, huh.ErrUserAborted) {
		t.Errorf("tui.ErrAborted must match huh.ErrUserAborted")
	}
}

// optionSelected reads huh.Option[string]'s unexported "selected" field via
// reflection. huh v2 (charm.land/huh/v2@v2.0.3, see option.go) only exposes
// selection through the fluent Option.Selected(bool) setter used when
// *building* a form field; there is no public getter. Reflect's typed
// accessors (Bool, in this case) can read unexported struct fields directly
// without going through Interface(), so this works without unsafe.
func optionSelected(t *testing.T, opt huh.Option[string]) bool {
	t.Helper()

	f := reflect.ValueOf(opt).FieldByName("selected")
	if !f.IsValid() {
		t.Fatalf("huh.Option[string] has no field named %q; huh API may have changed", "selected")
	}

	return f.Bool()
}

// formOutputPointer reads huh.Form's unexported "output" field via
// reflection and returns the pointer identity of the concrete writer stored
// in it (0 when unset). huh v2 exposes output only through the fluent
// Form.WithOutput setter — there is no public getter — and reflect's
// Pointer accessor can read unexported fields without going through
// Interface(), mirroring the optionSelected helper above.
func formOutputPointer(t *testing.T, form *huh.Form) uintptr {
	t.Helper()

	f := reflect.ValueOf(form).Elem().FieldByName("output")
	if !f.IsValid() {
		t.Fatalf("huh.Form has no field named %q; huh API may have changed", "output")
	}
	if f.IsNil() {
		return 0
	}

	return f.Elem().Pointer()
}

// TestConfigureWizard asserts the wizard form always routes its transcript
// to stderr. stdout is reserved for the machine-greppable key=value plan
// preview; without the explicit WithOutput, huh's accessible mode defaults
// to stdout, so `dotf plan > plan.txt` would swallow every prompt into the
// redirect and appear hung.
func TestConfigureWizard(t *testing.T) {
	tests := []struct {
		name       string
		accessible bool
	}{
		{name: "accessible mode writes prompts to stderr", accessible: true},
		{name: "tui mode writes to stderr", accessible: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := configureWizard(huh.NewForm(huh.NewGroup(huh.NewConfirm())), tt.accessible)

			got := formOutputPointer(t, form)
			want := reflect.ValueOf(os.Stderr).Pointer()
			if got != want {
				t.Errorf("configureWizard(accessible=%v) form output pointer = %#x, want os.Stderr (%#x)", tt.accessible, got, want)
			}
		})
	}
}

func testManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Intents: map[string]manifest.Intent{
			"base": {
				Description: "Core shell + CLI tooling",
				Default:     true,
				Packages:    []string{"git", "zsh", "starship"},
			},
			"dev": {
				Description: "Development toolchains",
				Default:     true,
				Packages:    []string{"git", "go", "rustup"},
			},
			"desktop": {
				Description: "GUI apps and window management",
				Default:     false,
				Packages:    []string{"aerospace", "ghostty"},
			},
		},
	}
}

func TestIntentOptions(t *testing.T) {
	m := testManifest()

	options := IntentOptions(m)

	if len(options) != len(m.Intents) {
		t.Fatalf("IntentOptions() len = %d, want %d", len(options), len(m.Intents))
	}

	// Options must be sorted by intent name (map key), since map iteration
	// order is randomized.
	wantOrder := []string{"base", "desktop", "dev"}
	for i, opt := range options {
		if opt.Value != wantOrder[i] {
			t.Errorf("options[%d].Value = %q, want %q", i, opt.Value, wantOrder[i])
		}
	}

	wantSelected := map[string]bool{
		"base":    true,
		"dev":     true,
		"desktop": false,
	}

	for _, opt := range options {
		want := wantSelected[opt.Value]
		if got := optionSelected(t, opt); got != want {
			t.Errorf("optionSelected(%q) = %v, want %v", opt.Value, got, want)
		}
	}
}

func TestIntentOptions_LabelIncludesDescription(t *testing.T) {
	m := testManifest()

	options := IntentOptions(m)

	for _, opt := range options {
		intent := m.Intents[opt.Value]
		if intent.Description == "" {
			continue
		}
		if opt.Key == opt.Value {
			t.Errorf("options label for %q = %q, want it to include the description %q", opt.Value, opt.Key, intent.Description)
		}
	}
}

func TestIntentOptions_Empty(t *testing.T) {
	m := &manifest.Manifest{Intents: map[string]manifest.Intent{}}

	options := IntentOptions(m)

	if len(options) != 0 {
		t.Fatalf("IntentOptions() len = %d, want 0", len(options))
	}
}

func TestPackageSelectHeight(t *testing.T) {
	makePackages := func(n int) []manifest.Package {
		pkgs := make([]manifest.Package, n)
		for i := range pkgs {
			pkgs[i] = manifest.Package{Name: string(rune('a' + i))}
		}
		return pkgs
	}

	tests := []struct {
		name string
		m    *manifest.Manifest
		want int
	}{
		{
			name: "empty manifest yields just the title/description offset",
			m:    &manifest.Manifest{},
			want: packageSelectYOffset,
		},
		{
			name: "one package",
			m:    &manifest.Manifest{Packages: makePackages(1)},
			want: 1 + packageSelectYOffset,
		},
		{
			name: "more packages than huh's 8-row default cap",
			m:    &manifest.Manifest{Packages: makePackages(12)},
			want: 12 + packageSelectYOffset,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := packageSelectHeight(tt.m); got != tt.want {
				t.Errorf("packageSelectHeight() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPackageOptions(t *testing.T) {
	m := testManifest()

	tests := []struct {
		name         string
		intents      []string
		offered      []string
		selected     []string
		wantValues   []string
		wantSelected map[string]bool
	}{
		{
			name:       "first visit checks everything",
			intents:    []string{"base", "dev"},
			offered:    nil,
			selected:   nil,
			wantValues: []string{"git", "go", "rustup", "starship", "zsh"},
			wantSelected: map[string]bool{
				"git": true, "go": true, "rustup": true, "starship": true, "zsh": true,
			},
		},
		{
			name:       "deselection survives adding an intent",
			intents:    []string{"base", "dev", "desktop"},
			offered:    []string{"git", "go", "rustup", "starship", "zsh"},
			selected:   []string{"git", "rustup", "starship", "zsh"}, // user unchecked "go"
			wantValues: []string{"aerospace", "ghostty", "git", "go", "rustup", "starship", "zsh"},
			wantSelected: map[string]bool{
				"aerospace": true, // newly offered by "desktop" → checked
				"ghostty":   true, // newly offered by "desktop" → checked
				"git":       true,
				"go":        false, // user's deselection preserved
				"rustup":    true,
				"starship":  true,
				"zsh":       true,
			},
		},
		{
			name:       "deselection survives removing and re-adding overlap",
			intents:    []string{"base"},
			offered:    []string{"git", "go", "rustup", "starship", "zsh"},
			selected:   []string{"go", "rustup", "starship", "zsh"}, // user unchecked "git"
			wantValues: []string{"git", "starship", "zsh"},
			wantSelected: map[string]bool{
				"git":      false, // previously offered, still unchecked
				"starship": true,
				"zsh":      true,
			},
		},
		{
			name:         "no intents yields no options",
			intents:      nil,
			offered:      nil,
			selected:     nil,
			wantValues:   []string{},
			wantSelected: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offered := make(map[string]struct{}, len(tt.offered))
			for _, name := range tt.offered {
				offered[name] = struct{}{}
			}

			options := PackageOptions(m, tt.intents, offered, tt.selected)

			gotValues := make([]string, 0, len(options))
			for _, opt := range options {
				gotValues = append(gotValues, opt.Value)
			}
			if !reflect.DeepEqual(gotValues, tt.wantValues) {
				t.Fatalf("PackageOptions() values = %v, want %v", gotValues, tt.wantValues)
			}

			for _, opt := range options {
				want := tt.wantSelected[opt.Value]
				if got := optionSelected(t, opt); got != want {
					t.Errorf("optionSelected(%q) = %v, want %v", opt.Value, got, want)
				}
			}

			// Every returned option must now be recorded as offered so the
			// next evaluation preserves the user's state for it.
			for _, opt := range options {
				if _, ok := offered[opt.Value]; !ok {
					t.Errorf("offered missing %q after PackageOptions()", opt.Value)
				}
			}
		})
	}
}

func TestPackagesForIntents(t *testing.T) {
	m := testManifest()

	tests := []struct {
		name     string
		selected []string
		want     []string
	}{
		{
			name:     "single intent",
			selected: []string{"desktop"},
			want:     []string{"aerospace", "ghostty"},
		},
		{
			name:     "overlapping intents dedupe union",
			selected: []string{"base", "dev"},
			want:     []string{"git", "go", "rustup", "starship", "zsh"},
		},
		{
			name:     "unknown intent skipped",
			selected: []string{"base", "does-not-exist"},
			want:     []string{"git", "starship", "zsh"},
		},
		{
			name:     "no selection",
			selected: nil,
			want:     []string{},
		},
		{
			name:     "order independent, still stable sorted",
			selected: []string{"dev", "base"},
			want:     []string{"git", "go", "rustup", "starship", "zsh"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PackagesForIntents(m, tt.selected)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PackagesForIntents(%v) = %v, want %v", tt.selected, got, tt.want)
			}
		})
	}
}
