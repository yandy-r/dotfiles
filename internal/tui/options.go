// Package tui provides pure, TTY-free helpers that translate a decoded
// provisioning manifest into huh form primitives and derived selections.
// Nothing in this file touches a terminal: no form.Run(), no I/O. That
// keeps intent-to-option and selection-to-package mapping unit-testable
// without a TTY, while the interactive form wiring lives elsewhere.
package tui

import (
	"fmt"
	"slices"
	"sort"

	"charm.land/huh/v2"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

// IntentOptions builds one huh.Option per manifest intent, keyed by the
// intent's name so the caller can round-trip the selected values straight
// back into manifest.Manifest.Intents lookups. Options whose intent has
// Default set to true are pre-checked via Option.Selected. Intents are
// sorted by name for deterministic output, since map iteration order is
// randomized by the Go runtime.
func IntentOptions(m *manifest.Manifest) []huh.Option[string] {
	names := make([]string, 0, len(m.Intents))
	for name := range m.Intents {
		names = append(names, name)
	}
	sort.Strings(names)

	options := make([]huh.Option[string], 0, len(names))
	for _, name := range names {
		intent := m.Intents[name]

		label := name
		if intent.Description != "" {
			label = fmt.Sprintf("%s — %s", name, intent.Description)
		}

		options = append(options, huh.NewOption(label, name).Selected(intent.Default))
	}

	return options
}

// PackagesForIntents returns the deduped, stable-sorted union of package
// names referenced by the given selected intent names. Unknown intent
// names are silently skipped (the caller is expected to have sourced
// `selected` from IntentOptions, whose values are always valid intent
// keys).
//
// huh's accessible mode can double-report pre-selected multiselect options
// (charmbracelet/huh#547), an effect that worsens with more pre-checked
// options. Deduping here, rather than trusting the selection to already be
// unique, keeps the resulting package list correct regardless of how the
// selection was produced.
func PackagesForIntents(m *manifest.Manifest, selected []string) []string {
	packages := make([]string, 0)

	for _, name := range selected {
		intent, ok := m.Intents[name]
		if !ok {
			continue
		}

		packages = append(packages, intent.Packages...)
	}

	return dedupeSorted(packages)
}

// PackageOptions builds the Packages multiselect's options for the given
// selected intents, preserving the user's per-package check state across
// re-evaluations. huh v2 re-runs the field's OptionsFunc whenever the bound
// intent selection changes and then overwrites the bound value with every
// option marked selected, so pre-checking all options unconditionally would
// silently revert the user's deselections (the guarantee in cmd/dotf/apply.go
// is that the exact confirmed set — including deselections — is honored).
//
// offered is the caller-owned memory of package names shown on previous
// evaluations; PackageOptions adds every name it returns. A package never
// offered before arrives checked (the first visit checks everything, and a
// newly added intent's packages default to checked); a previously offered
// package keeps the user's state from selected.
func PackageOptions(m *manifest.Manifest, intents []string, offered map[string]struct{}, selected []string) []huh.Option[string] {
	names := PackagesForIntents(m, intents)

	options := make([]huh.Option[string], 0, len(names))
	for _, name := range names {
		_, seen := offered[name]
		checked := slices.Contains(selected, name)
		options = append(options, huh.NewOption(name, name).Selected(!seen || checked))
		offered[name] = struct{}{}
	}

	return options
}

// packageSelectYOffset is the number of non-option lines (title +
// description) huh's updateViewportSize subtracts from a MultiSelect's
// height before sizing the options viewport.
const packageSelectYOffset = 2

// packageSelectHeight returns the explicit field height for the Packages
// multiselect: one row per package the manifest could ever resolve (the
// union across intents can never exceed the flat package list) plus the
// title/description offset huh subtracts. Without an explicit height, huh
// v2's OptionsFunc pins the field at defaultHeight (10, i.e. 8 visible
// rows) regardless of terminal size, silently hiding pre-checked options
// below the fold. With one, tall terminals show every row, and huh's
// WindowSizeMsg shrink clamp still bounds the field on short terminals,
// where cursor movement scrolls the list.
func packageSelectHeight(m *manifest.Manifest) int {
	return len(m.Packages) + packageSelectYOffset
}

// dedupeSorted returns the stable-sorted deduplication of items. huh's
// accessible mode can double-report pre-selected multiselect options
// (charmbracelet/huh#547); callers use this as the final defense against
// duplicate selections landing in derived package lists.
func dedupeSorted(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	deduped := make([]string, 0, len(items))

	for _, item := range items {
		if _, dup := seen[item]; dup {
			continue
		}
		seen[item] = struct{}{}
		deduped = append(deduped, item)
	}

	sort.Strings(deduped)

	return deduped
}
