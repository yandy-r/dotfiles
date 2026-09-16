package main

import (
	"flag"
	"sort"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

// applyFlags holds the raw, parsed flag values for `dotf apply` before any
// environment fallbacks are merged in. Hyprland and LinuxDesktop are separate
// fields so both parse independently; resolveApplyOptions ORs them into the one
// resolved Hyprland field.
type applyFlags struct {
	Manifest, Homebrew string
	Headless, Force    bool
	Gaming, Ollama     bool
	Hyprland, Snapper  bool
	LinuxDesktop       bool
	Plasma             bool
	DryRun             bool
	NoUserConfig       bool
	Intents            multiFlag // reuse the type from install.go
	Only, Skip         multiFlag // package-level filters, reuse multiFlag
}

// setApplyFlags registers apply's flags on fs into a fresh applyFlags and
// returns the pointer. Single source of truth for apply's flag surface, shared
// by runApply (which then calls resolveApplyOptions on the parsed value) and the
// help renderer (DRY — usage strings live only here; --manifest reuses
// manifestFlagUsage).
func setApplyFlags(fs *flag.FlagSet) *applyFlags {
	f := &applyFlags{}
	fs.StringVar(&f.Manifest, "manifest", "", manifestFlagUsage)
	fs.StringVar(&f.Homebrew, "homebrew", "", "Homebrew tristate: true|false ('' = OS default; Linux true is a no-op)")
	fs.BoolVar(&f.Headless, "headless", false, "skip the TUI and all prompts; apply the resolved intent set")
	fs.BoolVar(&f.Force, "force", false, "imply --headless, force symlink recreation, and reinstall already-installed packages")
	fs.BoolVar(&f.Gaming, "gaming", false, "add the gaming intent to the packages phase")
	fs.BoolVar(&f.Ollama, "enable-ollama", false, "add the ollama intent (no-op until the manifest defines it)")
	fs.BoolVar(&f.Hyprland, "hyprland", false, "manage the opt-in Hyprland/wlroots desktop config set (forces symlink backups)")
	fs.BoolVar(&f.LinuxDesktop, "linux-desktop", false, "deprecated alias for --hyprland")
	fs.BoolVar(&f.Snapper, "snapper", false, "take a snapper pre-apply snapshot (Linux only, no-op without snapper)")
	fs.BoolVar(&f.Plasma, "plasma", false, "sync the KDE Plasma config set (Linux/KDE only, no-op elsewhere)")
	fs.BoolVar(&f.DryRun, "dry-run", false, "compose the plan without executing; never primes sudo")
	// --no-user-config lives inside setApplyFlags (rather than the standalone
	// setNoUserConfigFlag other commands call) because apply resolves every
	// boolean's env twin through applyFlags→resolveApplyOptions; the usage string
	// still reuses the shared const so the wording cannot drift.
	fs.BoolVar(&f.NoUserConfig, "no-user-config", false, noUserConfigFlagUsage)
	fs.Var(&f.Intents, "intent", "narrow the headless intent set to exactly these (repeatable)")
	fs.Var(&f.Only, "only", "install only these named packages, independent of --intent (repeatable)")
	fs.Var(&f.Skip, "skip", "exclude these named packages after intent/only selection (repeatable)")
	return f
}

// applyOptions is the resolved view of applyFlags after merging each boolean
// with its environment equivalent: Headless with NON_INTERACTIVE/FORCE_INSTALL,
// Force with FORCE_INSTALL, Gaming with ENABLE_GAMING, Ollama with
// ENABLE_OLLAMA, Hyprland with HYPRLAND (and the deprecated LINUX_DESKTOP
// alias), Snapper with SNAPPER, Plasma with PLASMA, and NoUserConfig with
// DOTF_NO_USER_CONFIG. Homebrew is a flag-only tristate.
type applyOptions struct {
	Manifest, Homebrew string
	Headless, Force    bool
	Gaming, Ollama     bool
	Hyprland, Snapper  bool
	Plasma             bool
	DryRun             bool
	NoUserConfig       bool
	Intents            []string
	Only, Skip         []string
}

// envTrue reports whether v is exactly "true" (bootstrap's == "true" contract).
// Values like "1", "yes", or "TRUE" are intentionally not truthy.
func envTrue(v string) bool { return v == "true" }

// optInIntents are intents excluded from the headless default set. gaming and
// ollama are added by their feature flag (--gaming/--enable-ollama); the
// provisioning-parity bundles (runtimes/ai/cloud/git-tools), the system-setup
// bundle, and desktop-fonts (the extra Nerd Font families only the Hyprland
// desktop set pins) have no dedicated flag and are opted into explicitly with
// `--intent <name>` (#31). The `fonts` intent is deliberately absent: the
// families the terminal and editor configs pin belong in the default set.
var optInIntents = map[string]bool{
	"gaming":        true,
	"ollama":        true,
	"runtimes":      true,
	"ai":            true,
	"cloud":         true,
	"git-tools":     true,
	"system":        true,
	"desktop-fonts": true,
}

// resolveApplyOptions merges parsed flags with their env equivalents. For each
// boolean, precedence is a logical OR of the flag and its env variable (flag >
// env), matching bootstrap's behavior. getenv is injected (os.Getenv in prod, a
// map lookup in tests) so env parity is table-testable without touching the
// process environment. Homebrew is a flag-only tristate; DryRun, Manifest,
// Intents, and the package-level Only/Skip filters pass through unchanged (like
// --intent, the package filters have no env twin).
func resolveApplyOptions(f applyFlags, getenv func(string) string) applyOptions {
	return applyOptions{
		Manifest: f.Manifest,
		Homebrew: f.Homebrew,
		Headless: f.Headless || f.Force || envTrue(getenv("NON_INTERACTIVE")) || envTrue(getenv("FORCE_INSTALL")),
		Force:    f.Force || envTrue(getenv("FORCE_INSTALL")),
		Gaming:   f.Gaming || envTrue(getenv("ENABLE_GAMING")),
		Ollama:   f.Ollama || envTrue(getenv("ENABLE_OLLAMA")),
		// --hyprland/HYPRLAND is the primary spelling; --linux-desktop and
		// LINUX_DESKTOP are the retained deprecated aliases, so all four sources
		// OR into one resolved field.
		Hyprland: f.Hyprland || f.LinuxDesktop || envTrue(getenv("HYPRLAND")) || envTrue(getenv("LINUX_DESKTOP")),
		Snapper:  f.Snapper || envTrue(getenv("SNAPPER")),
		// The twin is the unprefixed PLASMA, not DOTF_PLASMA: every apply twin
		// is unprefixed (NON_INTERACTIVE, FORCE_INSTALL, ENABLE_GAMING,
		// ENABLE_OLLAMA, HYPRLAND and its LINUX_DESKTOP alias, SNAPPER) because
		// they are a bootstrap.sh contract, and a one-off prefix here would break
		// this uniform getenv(NAME) shape. envTrue is exact-match, so a stray
		// PLASMA=1 or PLASMA=kde is not truthy.
		Plasma:       f.Plasma || envTrue(getenv("PLASMA")),
		DryRun:       f.DryRun,
		NoUserConfig: f.NoUserConfig || envTrue(getenv("DOTF_NO_USER_CONFIG")),
		Intents:      f.Intents,
		Only:         f.Only,
		Skip:         f.Skip,
	}
}

// resolveIntents computes the headless intent set. When --intent flags were
// given, that explicit set is the base; otherwise the base is every manifest
// intent NOT in optInIntents, sorted for determinism. The gaming/ollama opt-in
// intents are then appended when their flags/env are set, and the result is
// deduplicated preserving first-seen order. Guided mode uses
// tui.Selection.Intents instead and does not call this.
func resolveIntents(m *manifest.Manifest, opts applyOptions) []string {
	var base []string
	if len(opts.Intents) > 0 {
		base = opts.Intents
	} else {
		for name := range m.Intents {
			if !optInIntents[name] {
				base = append(base, name)
			}
		}
		sort.Strings(base)
	}

	combined := make([]string, 0, len(base)+2)
	combined = append(combined, base...)
	if opts.Gaming {
		combined = append(combined, "gaming")
	}
	if opts.Ollama {
		combined = append(combined, "ollama")
	}
	return dedupeStrings(combined)
}

// dedupeStrings returns names with duplicates removed, preserving first-seen
// order.
func dedupeStrings(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
