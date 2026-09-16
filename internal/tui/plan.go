package tui

import (
	"os"
	"slices"

	"charm.land/bubbles/v2/key"
	"charm.land/huh/v2"

	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// ErrAborted is returned by Run when the user cancels the wizard with Ctrl+C or
// Esc. It aliases huh's sentinel so callers in cmd/dotf can classify a
// cancellation via errors.Is(err, tui.ErrAborted) without importing huh directly.
var ErrAborted = huh.ErrUserAborted

// knownOSValues lists the platform.OS values a user can explicitly pick in
// the OS override step, in presentation order. platform.OSUnknown is
// intentionally excluded — it's a detection fallback, not a selectable
// target — but osOptions still guarantees the detected value is present as
// an option even when it's platform.OSUnknown, so the Select always has a
// matching default.
var knownOSValues = []platform.OS{
	platform.OSMacOS,
	platform.OSDebian,
	platform.OSRedHat,
	platform.OSArch,
	platform.OSSUSE,
	platform.OSLinux,
	platform.OSWSL,
	platform.OSWindows,
}

// Selection is the user's answers collected from the interactive wizard:
// the (possibly overridden) OS, the selected intents, the resulting deduped
// package list, and whether the final confirmation step was accepted.
type Selection struct {
	// OS is the target platform, either auto-detected or overridden in the wizard.
	OS platform.OS
	// Intents lists the intent names the user selected.
	Intents []string
	// Packages is the deduped, stable-sorted union of packages for the selected intents.
	Packages []string
	// Confirmed is true when the user accepted the final "Apply this selection?" prompt.
	Confirmed bool
}

// osOptions builds the OS override Select's options, with the detected OS
// pre-selected. detected is prepended when it falls outside knownOSValues
// (e.g. platform.OSUnknown on an undetectable host) so it is always a valid,
// selectable option.
func osOptions(detected platform.OS) []huh.Option[platform.OS] {
	values := knownOSValues

	if !slices.Contains(values, detected) {
		values = append([]platform.OS{detected}, values...)
	}

	options := make([]huh.Option[platform.OS], 0, len(values))
	for _, v := range values {
		options = append(options, huh.NewOption(string(v), v).Selected(v == detected))
	}

	return options
}

// configureWizard applies the run mode and output routing to the wizard
// form. The wizard transcript must always go to stderr: stdout is reserved
// for the machine-greppable key=value plan preview, so `dotf plan > plan.txt`
// and `dotf plan | grep packages=` keep prompts visible on the terminal.
// huh's non-accessible mode already defaults to stderr, but its accessible
// mode defaults to stdout (huh v2.0.3 form.go: "Default is STDOUT when
// [Form] is accessible, STDERR otherwise"), so the explicit WithOutput is
// what keeps accessible-mode prompts out of a redirected stdout.
func configureWizard(form *huh.Form, accessible bool) *huh.Form {
	return form.WithAccessible(accessible).WithOutput(os.Stderr).WithKeyMap(abortKeyMap())
}

// abortKeyMap returns huh's default keybindings with the form-level Quit binding
// extended to also fire on Esc, so the user can abort the wizard from any step
// with either Ctrl+C or Esc. huh binds Quit to ctrl+c only by default.
//
// huh checks the form-level Quit before delegating a key to the active field
// (form.go Update), so a form-level Esc→Quit unconditionally shadows any field's
// own Esc handling. huh's Select and MultiSelect are filterable by default and
// bind Esc to set/clear an active "/" filter — which the shadow would silently
// turn into a full-wizard abort mid-filter. To make Esc an unambiguous abort
// from every step, filtering is disabled here (the wizard's lists are short, so
// "/" filtering is not worth the Esc collision): with the Filter bindings
// disabled, "/" never enters filter mode and Esc never has a competing meaning.
func abortKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("esc", "quit"),
	)
	// Disable per-field "/" filtering (and its Esc-bound set/clear) on Select and
	// MultiSelect so Esc is only ever the form-level abort. WithKeyMap propagates
	// these sub-keymaps down to every field.
	for _, b := range []*key.Binding{
		&km.Select.Filter, &km.Select.SetFilter, &km.Select.ClearFilter,
		&km.MultiSelect.Filter, &km.MultiSelect.SetFilter, &km.MultiSelect.ClearFilter,
	} {
		b.SetEnabled(false)
	}
	return km
}

// Run drives the interactive multi-step wizard: OS confirmation/override,
// intent selection, package selection (derived live from the currently
// selected intents), and a final confirmation. accessible runs the form in
// huh's screen-reader-friendly mode (no redrawing).
//
// Run requires a TTY and must not be called from unit tests; it is covered
// only via cmd/dotf dispatch and manual validation.
func Run(m *manifest.Manifest, info platform.Info, accessible bool) (Selection, error) {
	osValue := info.OS
	osSelect := huh.NewSelect[platform.OS]().
		Title("Operating System").
		Description("Detected automatically — override if it's wrong.").
		Options(osOptions(info.OS)...).
		Value(&osValue)

	selectedIntents := make([]string, 0)
	intentSelect := huh.NewMultiSelect[string]().
		Title("Intents").
		Description("Choose which provisioning intents to apply.").
		Options(IntentOptions(m)...).
		Value(&selectedIntents)

	// offered remembers every package name a previous OptionsFunc run has
	// shown, so going back and changing intents re-checks only genuinely new
	// packages instead of reverting the user's deselections. OptionsFunc runs
	// inside a tea.Cmd, but huh serializes per-field evaluations via the
	// bindings hash, so this single closure-shared map needs no locking.
	offered := make(map[string]struct{})
	selectedPackages := make([]string, 0)
	packageSelect := huh.NewMultiSelect[string]().
		Title("Packages").
		Description("Packages resolved from the selected intents.").
		OptionsFunc(func() []huh.Option[string] {
			return PackageOptions(m, selectedIntents, offered, selectedPackages)
		}, &selectedIntents).
		Height(packageSelectHeight(m)).
		Value(&selectedPackages)

	confirmed := false
	confirm := huh.NewConfirm().
		Title("Apply this selection?").
		Value(&confirmed)

	form := configureWizard(huh.NewForm(
		huh.NewGroup(osSelect),
		huh.NewGroup(intentSelect),
		huh.NewGroup(packageSelect),
		huh.NewGroup(confirm),
	), accessible)

	if err := form.Run(); err != nil {
		return Selection{}, err
	}

	return Selection{
		OS:        osValue,
		Intents:   append([]string(nil), selectedIntents...),
		Packages:  dedupeSorted(selectedPackages),
		Confirmed: confirmed,
	}, nil
}
