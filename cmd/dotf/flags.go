package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

// manifestFlagUsage is the single copy of the --manifest flag description.
// It lives here (not in each command) so the runners, the help renderer, and
// the multiple commands that expose --manifest can never drift apart on the
// wording. setInstallFlags, setSysconfigFlags, and setApplyFlags reference it
// too, so the string exists in exactly one place.
const manifestFlagUsage = "path to a manifest TOML file, overriding the embedded default"

// setManifestFlag registers the shared --manifest flag on fs and returns a
// pointer to its parsed value. plan, status, and diff each expose --manifest
// (via setReadFlags) and install/sysconfig/apply reuse it too, so they share
// this one setter rather than re-declaring the flag; their distinct behaviour
// lives in the help registry prose, not the flag.
func setManifestFlag(fs *flag.FlagSet) *string {
	return fs.String("manifest", "", manifestFlagUsage)
}

// jsonFlagUsage is the single copy of the --json flag description, shared by the
// runners, the help renderer, and the completion model so they cannot drift.
const jsonFlagUsage = "emit machine-readable JSON to stdout instead of the default output"

// setJSONFlag registers the shared --json boolean flag on fs and returns a
// pointer to its parsed value. The read-only reporting commands (plan, status,
// diff) opt into a stable JSON output shape through this one flag.
func setJSONFlag(fs *flag.FlagSet) *bool {
	return fs.Bool("json", false, jsonFlagUsage)
}

// setReadFlags registers the shared flag surface of the read-only reporting
// commands — --manifest and --json — on fs and returns pointers to both parsed
// values. plan, status, and diff share this single setter so their runners and
// their help-registry entries can never disagree on which flags exist; the
// help renderer and completion model discard the returned pointers and read the
// flags straight off the flagset.
func setReadFlags(fs *flag.FlagSet) (manifestPath *string, jsonOut *bool) {
	return setManifestFlag(fs), setJSONFlag(fs)
}

// verboseFlagUsage is the single copy of doctor's --verbose flag description,
// shared by the runner, the help renderer, and the completion model so they
// cannot drift.
const verboseFlagUsage = "also print ok-check evidence on stderr (expanded diagnostics)"

// setDoctorFlags registers doctor's flag surface — the shared read-only flags
// (--manifest, --json via setReadFlags) plus doctor's own --verbose — on fs and
// returns pointers to all three parsed values. runDoctor and doctor's help entry
// share this single setter so its runner, help text, and generated completions
// can never disagree on which flags exist; --no-user-config is added separately
// (like status/diff/plan) via setNoUserConfigFlag, keeping this setter's arity
// aligned with the read-only family.
func setDoctorFlags(fs *flag.FlagSet) (manifestPath *string, jsonOut, verbose *bool) {
	manifestPath, jsonOut = setReadFlags(fs)
	verbose = fs.Bool("verbose", false, verboseFlagUsage)
	return manifestPath, jsonOut, verbose
}

// countsFlagUsage is the single copy of the --counts flag description, shared by
// the runners, the help renderer, and the completion model so they cannot drift.
const countsFlagUsage = "append an aggregate backend/class package-count summary (governance histogram)"

// setDriftFlags registers status/diff's flag surface — setReadFlags plus the
// governance --counts flag — on fs and returns pointers to all three parsed
// values. status and diff share this setter (plan keeps plain setReadFlags: it
// has no host-probe rows to summarize), so their runners, help entries, and
// generated completions stay in lockstep on the exact flag set.
func setDriftFlags(fs *flag.FlagSet) (manifestPath *string, jsonOut, counts *bool) {
	manifestPath, jsonOut = setReadFlags(fs)
	counts = fs.Bool("counts", false, countsFlagUsage)
	return manifestPath, jsonOut, counts
}

// noUserConfigFlagUsage is the single copy of the --no-user-config flag
// description, shared by every manifest-consuming command so the wording cannot
// drift.
const noUserConfigFlagUsage = "disable the ~/.config/dotf user-config overlay (pristine/CI resolution)"

// setNoUserConfigFlag registers the shared --no-user-config boolean flag on fs
// and returns a pointer to its parsed value. install, apply, status, diff, plan,
// sysconfig, and lock each resolve the XDG user-config overlay and expose this
// flag to pin resolution to the base manifest; link is excluded (it has no
// --manifest and uses the built-in symlink table). It is deliberately a
// STANDALONE setter — called separately alongside setReadFlags/setDriftFlags
// rather than folded into them — so those setters keep their existing return
// arities while this one flag still spans every manifest-consuming command.
// (apply registers it inside setApplyFlags instead, since apply resolves every
// boolean's env twin through its own applyFlags struct.)
func setNoUserConfigFlag(fs *flag.FlagSet) *bool {
	return fs.Bool("no-user-config", false, noUserConfigFlagUsage)
}

// noUserConfigEnabled ORs the parsed --no-user-config flag with its
// DOTF_NO_USER_CONFIG environment twin (flag || env, via envTrue), matching the
// DOTF_MANIFEST and apply env-twin conventions so CI can disable the overlay
// without passing a flag.
func noUserConfigEnabled(flagValue bool) bool {
	return flagValue || envTrue(os.Getenv("DOTF_NO_USER_CONFIG"))
}

// manifestOverlayOptions builds the manifest.ResolveOptions that layer the
// ~/.config/dotf user-config overlay onto the base manifest. It computes
// ConfigRoot = ${XDG_CONFIG_HOME:-~/.config}/dotf and wires a progName-prefixed
// stderr Logf so overlay package/intent overrides are announced, never silent.
// When noUserConfig is set the overlay is disabled up front (ConfigRoot left
// empty). If the home dir cannot be resolved, ConfigRoot is likewise left empty
// (Resolve skips the overlay) and a warning is emitted to stderr, so a
// genuinely-misconfigured environment is not masked (fail-loud).
func manifestOverlayOptions(noUserConfig bool, stderr io.Writer) manifest.ResolveOptions {
	opts := manifest.ResolveOptions{
		NoUserConfig: noUserConfig,
		Logf: func(format string, args ...any) {
			_, _ = fmt.Fprintf(stderr, progName+": "+format+"\n", args...)
		},
	}
	if noUserConfig {
		return opts
	}

	home, err := os.UserHomeDir()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: user-config overlay disabled: resolve home dir: %v\n", progName, err)
		return opts
	}
	opts.ConfigRoot = filepath.Join(xdgConfigHome(home), "dotf")
	return opts
}
