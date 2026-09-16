package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/privilege"
)

// installResult records the outcome of a single package install attempt.
type installResult struct {
	name    string
	backend manifest.Backend
	status  string
	err     error
}

// runInstall resolves the manifest, detects the host platform, and installs
// the selected packages via the backend engine. Failures are collected
// non-fatally; the process exits non-zero when one or more installs fail.
func runInstall(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)

	o := setInstallFlags(fs)
	noUserConfig := setNoUserConfigFlag(fs)

	// Loop-parse to collect positional package names while still parsing the
	// value flags (--intent/--only/--manifest) that may appear before, between,
	// or after them. Go's flag stops at the first non-flag token, so a single
	// fs.Parse would drop any flag that trails a positional; repeatedly parsing
	// and consuming one positional at a time is value-flag-safe and order-
	// independent (`install claude --update`, `install --update claude`, and
	// `install claude codex --update` all resolve correctly).
	var names []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		names = append(names, fs.Arg(0))
		rest = fs.Args()[1:]
	}

	action, err := resolveAction(o.update, o.remove)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
		return 2
	}

	// update/remove are scoped, imperative verbs: they must name a target
	// (positional, --only, or --intent) and never operate on the implicit
	// whole-manifest set — "update/remove the world" is deliberately unreachable.
	if action != backend.ActionInstall && len(o.intents) == 0 && len(o.only) == 0 && len(names) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: install --update/--remove requires a package name, --only, or --intent\n", progName)
		return 2
	}

	m, err := manifest.Resolve(o.manifest, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS,
		manifestOverlayOptions(noUserConfigEnabled(*noUserConfig), stderr))
	if err != nil {
		return reportErr(stderr, err)
	}

	// Load the lock beside the resolved manifest (embedded seed as fallback) so
	// pinned-artifact checksums are verified during Install. A missing sibling
	// lock is tolerated; install has no --lock override.
	lock, err := lockfile.Resolve("", o.manifest, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS)
	if err != nil {
		return reportErr(stderr, err)
	}

	if err := validateIntents(m, o.intents); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
		return 2
	}

	info := platform.Detect(r)
	pkgs := packagesForInstall(m, o.intents)

	// Positional names fold into the --only set: an explicit target selection,
	// independent of --intent and validated (unknown name → exit 2) exactly like
	// --only. Package-level narrowing is applied to both the dry-run and the real
	// install, so the two never disagree.
	only := append(append([]string{}, o.only...), names...)
	pkgs, err = filterPackages(m, pkgs, only, o.skip)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
		return 2
	}

	if o.dryRun {
		if err := printInstallDryRun(stdout, action, pkgs); err != nil {
			return reportErr(stderr, err)
		}
		return 0
	}

	// Put the user-level tool directories on PATH before any install, for the
	// same reason apply does: a toolchain package shells out to npm/cargo, which
	// live under the mise shims and ~/.cargo/bin — directories the invoking
	// shell's PATH may predate. Errors here are impossible (a missing $HOME just
	// yields no additions), so this is unconditional and silent.
	if home, err := os.UserHomeDir(); err == nil {
		applyUserToolPath(home)
	}

	sudo := privilege.NewSudo()
	if err := sudo.Prime(r); err != nil {
		return reportErr(stderr, fmt.Errorf("sudo prime: %w", err))
	}

	ctx := context.Background()
	cancel := sudo.KeepAlive(ctx, r)
	defer cancel()

	drop := privilege.NewDropper()
	engine := backend.Default(r, sudo, drop, resolveRepoRoot(), lock)

	results := make([]installResult, 0, len(pkgs))
	for _, pkg := range pkgs {
		results = append(results, performPackage(engine, action, pkg, info, o.force))
	}

	if err := printInstallResults(stdout, results); err != nil {
		return reportErr(stderr, err)
	}
	printInstallSummary(stderr, results)

	return installExitCode(results)
}

// resolveAction maps the --update/--remove booleans to a backend.Action. At most
// one may be set; both together is a usage error (exit 2). Neither set defaults
// to ActionInstall, so the classic `dotf install` path is unchanged.
func resolveAction(update, remove bool) (backend.Action, error) {
	switch {
	case update && remove:
		return backend.ActionInstall, fmt.Errorf("install: --update and --remove are mutually exclusive")
	case update:
		return backend.ActionUpdate, nil
	case remove:
		return backend.ActionRemove, nil
	default:
		return backend.ActionInstall, nil
	}
}

// performPackage probes backend applicability, applies the action-aware
// installed-state gate, then dispatches the lifecycle action to the engine.
// Packages whose backend cannot run on this host (apt-repo on a non-apt host,
// aur on non-Arch) are skipped with status "not-applicable" and no error, so a
// converged host is never failed by a platform-gated manifest entry. A probe
// error (unknown backend) is recorded as a failure.
//
// The installed-state gate is symmetric by action (see the design table):
//
//   - Install skips an already-installed package as "already-installed" (unless
//     force is set) — the idempotency gate that keeps a re-apply from
//     reinstalling rolling tools every run.
//   - Update skips a not-installed package as "not-installed".
//   - Remove skips a not-installed package as "already-absent".
//
// force bypasses ONLY Install's skip; Update/Remove use an UNCONDITIONAL
// installed guard (force cannot override it — this is what makes a non-idempotent
// script uninstall safe). A probe error never skips and never fails: it falls
// through to dispatch so the host still converges (the flaky-probe rule).
// An ActionUnsupportedError from the engine surfaces as "failed" (exit 1).
func performPackage(engine *backend.Engine, action backend.Action, pkg manifest.Package, info platform.Info, force bool) installResult {
	res := installResult{name: pkg.Name, backend: pkg.Backend}

	applicable, err := engine.Applicable(pkg, info)
	switch {
	case err != nil:
		res.status = "failed"
		res.err = err
		return res
	case !applicable:
		res.status = "not-applicable"
		return res
	}

	// Only probe when the gate can act on the result: Install consults it unless
	// forced; Update/Remove always consult it (force cannot bypass their guard).
	if action != backend.ActionInstall || !force {
		if installed, probeErr := engine.Installed(pkg, info); probeErr == nil {
			switch action {
			case backend.ActionInstall:
				if installed { // !force guaranteed by the outer guard
					res.status = "already-installed"
					return res
				}
			case backend.ActionUpdate:
				if !installed {
					res.status = "not-installed"
					return res
				}
			case backend.ActionRemove:
				if !installed {
					res.status = "already-absent"
					return res
				}
			}
		}
	}

	if err := engine.Perform(action, pkg, info); err != nil {
		res.status = "failed"
		res.err = err
		return res
	}
	res.status = actionSuccessStatus(action)
	return res
}

// actionSuccessStatus is the result status for a successful dispatch of action.
func actionSuccessStatus(action backend.Action) string {
	switch action {
	case backend.ActionUpdate:
		return "updated"
	case backend.ActionRemove:
		return "removed"
	default:
		return "ok"
	}
}

// installOpts holds the parsed flag values for `dotf install`.
type installOpts struct {
	manifest string
	dryRun   bool
	force    bool
	update   bool
	remove   bool
	intents  multiFlag
	only     multiFlag
	skip     multiFlag
}

// setInstallFlags registers install's flags on fs and returns the opts pointer
// they parse into. It is the single source of truth for install's flag surface:
// runInstall calls it before fs.Parse, and the help renderer calls it on a
// throwaway flagset to print the Flags section (DRY — no re-typed usage text).
func setInstallFlags(fs *flag.FlagSet) *installOpts {
	o := &installOpts{}
	fs.StringVar(&o.manifest, "manifest", "", manifestFlagUsage)
	fs.BoolVar(&o.dryRun, "dry-run", false, "print the install plan without executing")
	fs.BoolVar(&o.force, "force", false, "reinstall packages even when already installed")
	fs.BoolVar(&o.update, "update", false, "update the named package(s) in place (backend must support it)")
	fs.BoolVar(&o.remove, "remove", false, "remove the named package(s) (backend must support it)")
	fs.Var(&o.intents, "intent", "limit installs to packages from this intent (repeatable)")
	fs.Var(&o.only, "only", "install only these named packages, independent of --intent (repeatable)")
	fs.Var(&o.skip, "skip", "exclude these named packages after intent/only selection (repeatable)")
	return o
}

// multiFlag collects repeated --intent flag values.
type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

// validateIntents rejects user-typed --intent names that the manifest does not
// define. Without this gate a typo (--intent devs for dev) would be silently
// dropped by packagesForInstall, resolve to an empty package set, and exit 0 —
// silent success on a no-op. Only explicit user input goes through here; the
// gaming/ollama opt-in appends in resolveIntents stay tolerant, preserving the
// "--enable-ollama is a no-op until the manifest defines it" contract.
func validateIntents(m *manifest.Manifest, intents []string) error {
	for _, intentName := range intents {
		if _, ok := m.Intents[intentName]; !ok {
			valid := make([]string, 0, len(m.Intents))
			for name := range m.Intents {
				valid = append(valid, name)
			}
			sort.Strings(valid)
			return fmt.Errorf("unknown intent %q; manifest defines: %s", intentName, strings.Join(valid, ", "))
		}
	}
	return nil
}

// packagesForInstall returns manifest packages to install. With no --intent
// flags, every package in the manifest is included; otherwise only packages
// referenced by the named intents are returned (deduplicated, manifest order).
func packagesForInstall(m *manifest.Manifest, intents []string) []manifest.Package {
	if len(intents) == 0 {
		return m.Packages
	}

	names := make(map[string]struct{})
	for _, intentName := range intents {
		intent, ok := m.Intents[intentName]
		if !ok {
			continue
		}
		for _, name := range intent.Packages {
			names[name] = struct{}{}
		}
	}

	out := make([]manifest.Package, 0, len(names))
	for _, pkg := range m.Packages {
		if _, ok := names[pkg.Name]; ok {
			out = append(out, pkg)
		}
	}
	return out
}

// filterPackages narrows a package set with the package-level --only/--skip
// flags so a single tool can be provisioned or verified in isolation (#42). Its
// semantics, in order:
//
//   - Validate: every name in only and in skip must name a package the manifest
//     declares (m.Packages). The first unknown name is a usage error listing the
//     valid set — mirroring validateIntents — and maps to exit code 2 at the
//     call sites.
//   - Select: when only is non-empty, the base set becomes exactly the manifest
//     packages whose Name is in only (manifest order, deduped), REPLACING the
//     passed-in intent-derived pkgs entirely — this is the "--only selects
//     independent of --intent" contract. Otherwise the base set is pkgs.
//   - Skip: any package whose Name is in skip is then removed from the base set.
//
// The result may be empty; an empty set is a valid "nothing to do" outcome, not
// an error.
func filterPackages(m *manifest.Manifest, pkgs []manifest.Package, only, skip []string) ([]manifest.Package, error) {
	if err := validatePackageNames(m, only); err != nil {
		return nil, err
	}
	if err := validatePackageNames(m, skip); err != nil {
		return nil, err
	}

	base := pkgs
	if len(only) > 0 {
		base = packagesByName(m, only)
	}

	if len(skip) == 0 {
		return base, nil
	}

	excluded := make(map[string]struct{}, len(skip))
	for _, name := range skip {
		excluded[name] = struct{}{}
	}
	out := make([]manifest.Package, 0, len(base))
	for _, pkg := range base {
		if _, drop := excluded[pkg.Name]; drop {
			continue
		}
		out = append(out, pkg)
	}
	return out, nil
}

// validatePackageNames rejects any name the manifest does not declare as a
// package. It mirrors validateIntents: the first unknown name errors with the
// sorted set of valid package names, so a --only/--skip typo fails clearly
// instead of silently selecting nothing. Empty input passes.
func validatePackageNames(m *manifest.Manifest, names []string) error {
	if len(names) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(m.Packages))
	for _, pkg := range m.Packages {
		known[pkg.Name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := known[name]; !ok {
			valid := make([]string, 0, len(m.Packages))
			for _, pkg := range m.Packages {
				valid = append(valid, pkg.Name)
			}
			sort.Strings(valid)
			return fmt.Errorf("unknown package %q; manifest defines: %s", name, strings.Join(valid, ", "))
		}
	}
	return nil
}

// packagesByName returns the manifest packages whose names appear in names, in
// manifest order and deduplicated. It backs dotf apply's guided path, where the
// user confirms an explicit package set in the TUI (tui.Selection.Packages)
// that must be honored verbatim — including an empty selection, which installs
// nothing. This differs from packagesForInstall, whose empty-argument case
// deliberately means "every package".
func packagesByName(m *manifest.Manifest, names []string) []manifest.Package {
	want := make(map[string]struct{}, len(names))
	for _, name := range names {
		want[name] = struct{}{}
	}

	out := make([]manifest.Package, 0, len(want))
	for _, pkg := range m.Packages {
		if _, ok := want[pkg.Name]; ok {
			out = append(out, pkg)
		}
	}
	return out
}

// resolveRepoRoot returns DOTFILES_ROOT when set, otherwise walks upward from
// the working directory looking for manifest/packages.toml.
func resolveRepoRoot() string {
	if root := os.Getenv("DOTFILES_ROOT"); root != "" {
		return root
	}
	return findRepoRootFromCWD()
}

func findRepoRootFromCWD() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, "manifest", "packages.toml")
		if _, err := os.Stat(candidate); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// printInstallDryRun writes the would-apply package set as greppable key=value
// lines to stdout without mutating the host. The leading action= line reflects
// the lifecycle verb (install|update|remove) so the plan is unambiguous.
func printInstallDryRun(stdout io.Writer, action backend.Action, pkgs []manifest.Package) error {
	names := make([]string, len(pkgs))
	for i, pkg := range pkgs {
		names[i] = pkg.Name
	}

	lines := []string{
		fmt.Sprintf("action=%s", action),
		"dry-run=true",
		fmt.Sprintf("packages=%s", strings.Join(names, ",")),
	}

	byBackend := groupPackagesByBackendSlice(pkgs)
	for _, backendName := range sortedBackendKeys(byBackend) {
		lines = append(lines, fmt.Sprintf("plan.%s=%s", backendName, strings.Join(byBackend[backendName], ",")))
	}

	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return err
}

// groupPackagesByBackendSlice groups package names by backend without a
// manifest lookup — the caller already holds the selected Package records.
func groupPackagesByBackendSlice(pkgs []manifest.Package) map[string][]string {
	groups := make(map[string][]string)
	for _, pkg := range pkgs {
		backendName := string(pkg.Backend)
		groups[backendName] = append(groups[backendName], pkg.Name)
	}
	return groups
}

// printInstallResults writes per-backend result lines to stdout in sorted
// backend order. Each line lists package outcomes as name:status pairs.
func printInstallResults(stdout io.Writer, results []installResult) error {
	byBackend := make(map[string][]string)
	for _, res := range results {
		backendName := string(res.backend)
		byBackend[backendName] = append(byBackend[backendName], res.name+":"+res.status)
	}

	keys := sortedBackendKeys(byBackend)
	lines := make([]string, 0, len(keys))
	for _, backendName := range keys {
		sort.Strings(byBackend[backendName])
		lines = append(lines, fmt.Sprintf("result.%s=%s", backendName, strings.Join(byBackend[backendName], ",")))
	}

	if len(lines) == 0 {
		return nil
	}

	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return err
}

// printInstallSummary writes an aggregate count and per-failure diagnostics
// to stderr. Platform-inapplicable, already-installed, and no-op skip
// (already-absent/not-installed) packages are counted in their own buckets so
// they are never reported as succeeded or failed. The succeeded bucket spans the
// action success statuses (ok/updated/removed).
func printInstallSummary(stderr io.Writer, results []installResult) {
	var failed, succeeded, notApplicable, alreadyInstalled, skipped int
	var failures []installResult
	for _, res := range results {
		switch res.status {
		case "failed":
			failed++
			failures = append(failures, res)
		case "not-applicable":
			notApplicable++
		case "already-installed":
			alreadyInstalled++
		case "already-absent", "not-installed":
			skipped++
		default: // ok, updated, removed
			succeeded++
		}
	}

	_, _ = fmt.Fprintf(stderr, "%s: install summary: total=%d succeeded=%d already-installed=%d skipped=%d not-applicable=%d failed=%d\n",
		progName, len(results), succeeded, alreadyInstalled, skipped, notApplicable, failed)

	sort.Slice(failures, func(i, j int) bool {
		return failures[i].name < failures[j].name
	})
	for _, res := range failures {
		_, _ = fmt.Fprintf(stderr, "%s: %s: %v\n", progName, res.name, res.err)
	}
}

func installExitCode(results []installResult) int {
	for _, res := range results {
		if res.status == "failed" {
			return 1
		}
	}
	return 0
}
