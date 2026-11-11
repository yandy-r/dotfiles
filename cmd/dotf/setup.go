package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/privilege"
	"github.com/yandy-r/dotfiles/internal/symlink"
)

// setupOpts holds the parsed flag values for `dotf setup`.
type setupOpts struct {
	manifest string
	dryRun   bool
	yes      bool
}

// setSetupFlags registers setup's flags on fs and returns the opts pointer they
// parse into. Single source of truth for setup's flag surface, shared by
// runSetup and the help renderer (DRY — usage strings live only here).
func setSetupFlags(fs *flag.FlagSet) *setupOpts {
	o := &setupOpts{}
	fs.StringVar(&o.manifest, "manifest", "", manifestFlagUsage)
	fs.BoolVar(&o.dryRun, "dry-run", false, "print the ordered step plan without executing")
	fs.BoolVar(&o.yes, "yes", false, "run non-interactively; required to execute without a terminal")
	return o
}

// runSetup resolves a named [[module]] and runs its ordered, typed steps through
// the shipped engines (custom-script/symlink/pkg via their engines; sysconfig is
// not yet wired and fails fast per step). Step failures are collected
// non-fatally into a run-wide summary, mirroring dotf apply. Exit codes: 0
// converged, 1 step failures (or a non-interactive run without --yes), 2 usage
// (missing/unknown/too-many module names or a bad flag), 3 hard abort (sudo
// prime failure).
func runSetup(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(stderr)

	o := setSetupFlags(fs)
	noUserConfig := setNoUserConfigFlag(fs)

	// Loop-parse to pull the single <module> positional order-independently, so
	// `setup fingerprint --dry-run` and `setup --dry-run fingerprint` both work
	// (Go's flag stops at the first non-flag token — see runInstall for the same
	// idiom).
	var positionals []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		positionals = append(positionals, fs.Arg(0))
		rest = fs.Args()[1:]
	}

	m, err := manifest.Resolve(o.manifest, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS,
		manifestOverlayOptions(noUserConfigEnabled(*noUserConfig), stderr))
	if err != nil {
		return reportErr(stderr, err)
	}

	switch len(positionals) {
	case 0:
		_, _ = fmt.Fprintf(stderr, "%s: setup requires a module name; available: %s\n", progName, availableModules(m))
		return 2
	case 1:
		// exactly one module named — proceed.
	default:
		_, _ = fmt.Fprintf(stderr, "%s: setup takes exactly one module name, got %d\n", progName, len(positionals))
		return 2
	}

	mod, ok := moduleByName(m, positionals[0])
	if !ok {
		_, _ = fmt.Fprintf(stderr, "%s: unknown module %q; available: %s\n", progName, positionals[0], availableModules(m))
		return 2
	}

	info := platform.Detect(r)

	if !platform.GateAllows(mod.OS, info.OS) {
		_, _ = fmt.Fprintf(stderr, "%s: module %q requires os=%s; this host is os=%s\n", progName, mod.Name, mod.OS, info.OS)
		return 1
	}

	if o.dryRun {
		if err := printSetupDryRun(stdout, mod); err != nil {
			return reportErr(stderr, err)
		}
		return 0
	}

	// The named module IS the explicit target, so setup does not prompt; but it
	// mutates the host, so it refuses to execute non-interactively without --yes
	// (the CI/non-terminal acknowledgment), mirroring sysconfig's guard.
	if !o.yes && !stdoutIsTTY(stdout) {
		_, _ = fmt.Fprintf(stderr, "%s: refusing to run setup without --yes (non-interactive)\n", progName)
		return 1
	}

	// Prime sudo once for the whole run (setup scripts and pkg installs may need
	// it) and keep it warm across every step. A prime failure is a hard abort.
	sudo := privilege.NewSudo()
	if err := sudo.Prime(r); err != nil {
		return reportErr(stderr, fmt.Errorf("sudo prime: %w", err))
	}
	cancel := sudo.KeepAlive(context.Background(), r)
	defer cancel()

	engine := backend.Default(r, privilege.NewDropper(), resolveRepoRoot(), nil)
	eng := &realStepEngines{m: m, engine: engine, repoRoot: resolveRepoRoot(), home: setupHome(stderr), r: r}

	results := runModuleSteps(mod, eng, info)
	// Named once per run, before anything that can fail: every symlink step
	// shares one backup dir.
	printSetupBackupDir(stderr, eng.backupDir)

	if err := printSetupResults(stdout, mod.Name, results); err != nil {
		return reportErr(stderr, err)
	}
	printSetupSummary(stderr, mod.Name, results)

	return setupExitCode(results)
}

// setupStepResult records the outcome of a single module step.
type setupStepResult struct {
	index  int // 1-based step position
	kind   manifest.StepKind
	target string
	status string // ok | failed
	err    error
}

// stepEngines dispatches a module step to its backing engine, one method per
// typed kind. It is the orchestrator's injection seam: the production
// implementation (realStepEngines) wires the shipped engines, and tests inject a
// fake to assert steps run in declaration order.
type stepEngines interface {
	customScript(name, script string, info platform.Info) error
	pkg(name string, info platform.Info) error
	symlink(src string, info platform.Info) error
	sysconfig(domain string, ports []string, info platform.Info) error
}

// runModuleSteps executes a module's steps in declaration order via eng,
// collecting each outcome. A step failure never aborts the loop — later steps
// still run — so the caller can report every failure in one run-wide summary.
func runModuleSteps(mod manifest.Module, eng stepEngines, info platform.Info) []setupStepResult {
	results := make([]setupStepResult, 0, len(mod.Steps))
	for i, step := range mod.Steps {
		kind, target, ok := step.Resolve()
		res := setupStepResult{index: i + 1, kind: kind, target: target}
		if !ok {
			res.status = "failed"
			res.err = fmt.Errorf("step %d does not set exactly one kind field", i+1)
			results = append(results, res)
			continue
		}

		var err error
		switch kind {
		case manifest.StepCustomScript:
			err = eng.customScript(mod.Name, target, info)
		case manifest.StepPkg:
			err = eng.pkg(target, info)
		case manifest.StepSymlink:
			err = eng.symlink(target, info)
		case manifest.StepSysconfig:
			err = eng.sysconfig(target, step.Ports, info)
		}

		if err != nil {
			res.status = "failed"
			res.err = err
		} else {
			res.status = "ok"
		}
		results = append(results, res)
	}
	return results
}

// realStepEngines is the production stepEngines: it dispatches each kind to the
// shipped engine. It reuses the SAME primitives install/apply already use, so
// setup adds no new execution surface.
type realStepEngines struct {
	m        *manifest.Manifest
	engine   *backend.Engine
	repoRoot string
	home     string
	r        exec.Runner
	// backupDir is the run's single backup dir: the first symlink step that
	// backs anything up creates it, and every later step reuses it (via
	// symlink.Options.BackupDir) so one setup run never scatters _1/_2 dirs.
	backupDir string
}

// customScript runs a contained repo/overlay-relative script via the
// custom-script backend (the primary path the seed modules use). It builds a
// synthetic package so the same containment fence (repo-root/no-".."/no-absolute)
// and NON_INTERACTIVE handling apply unchanged.
func (e *realStepEngines) customScript(name, script string, info platform.Info) error {
	pkg := manifest.Package{Name: name, Backend: manifest.BackendCustomScript, Script: script}
	return e.engine.Install(pkg, info)
}

// pkg ensures a named manifest package is installed via the backend engine,
// probe-gated and idempotent through the shared performPackage. A "failed"
// result surfaces its error; every other status (ok/already-installed/
// not-applicable) is a converged step.
func (e *realStepEngines) pkg(name string, info platform.Info) error {
	pkg, ok := packageByName(e.m, name)
	if !ok {
		return fmt.Errorf("module pkg step names unknown package %q", name)
	}
	res := performPackage(e.engine, backend.ActionInstall, pkg, info, false)
	if res.status == "failed" {
		return res.err
	}
	return nil
}

// symlink provisions a single built-in link-table entry (matched by src) via the
// internal/symlink engine's plan/apply/validate split, backing up a real file
// at the dest before replacing it (the same default as `dotf link`). All steps
// of one run share the run's backup dir (e.backupDir). A src with no matching
// table entry fails fast rather than silently linking nothing.
func (e *realStepEngines) symlink(src string, info platform.Info) error {
	spec, ok := findLinkSpec(src)
	if !ok {
		return fmt.Errorf("module symlink step src %q has no matching entry in the built-in link table", src)
	}
	se := symlink.NewEngine(e.repoRoot, e.home)
	specs := []symlink.LinkSpec{spec}
	opts := symlink.Options{Backup: true, Validate: true, BackupDir: e.backupDir}
	plan, err := se.Plan(specs, info, opts)
	if err != nil {
		return err
	}
	report, err := se.Apply(plan, opts)
	// Recorded even on a partial failure: real files may already be copied.
	if e.backupDir == "" && report != nil {
		e.backupDir = report.BackupDir
	}
	if err != nil {
		return err
	}
	if _, err := se.Validate(specs, info, opts); err != nil {
		return err
	}
	return nil
}

// printSetupBackupDir names the run's shared backup dir on stderr (same wording
// as `dotf link`) when any step backed something up.
func printSetupBackupDir(stderr io.Writer, dir string) {
	if dir == "" {
		return
	}
	_, _ = fmt.Fprintf(stderr, "%s: link backups: %s\n", progName, dir)
}

// sysconfig is not yet wired into the setup orchestrator: the current
// sysconfig.Config/engine cannot carry a per-domain value from a module step
// (the step names only a domain, and its firewall domain is a mode not a port
// list), and its apply is confirmation-gated. It fails fast with a typed error
// until that wiring lands (the aspirational firewall/ssh module is gated on #34).
func (e *realStepEngines) sysconfig(domain string, ports []string, _ platform.Info) error {
	return &moduleStepUnsupportedError{kind: manifest.StepSysconfig, target: domain, ports: ports}
}

// moduleStepUnsupportedError is the fail-fast returned when a module step kind
// has no engine wiring yet (currently: sysconfig).
type moduleStepUnsupportedError struct {
	kind   manifest.StepKind
	target string
	ports  []string
}

func (e *moduleStepUnsupportedError) Error() string {
	return fmt.Sprintf("setup: step kind %q (%s) is not yet supported by the setup orchestrator", e.kind, e.target)
}

// findLinkSpec returns the built-in link-table entry whose Src equals src.
func findLinkSpec(src string) (symlink.LinkSpec, bool) {
	for _, spec := range symlink.DefaultTable() {
		if spec.Src == src {
			return spec, true
		}
	}
	return symlink.LinkSpec{}, false
}

// packageByName returns the manifest package with the given name.
func packageByName(m *manifest.Manifest, name string) (manifest.Package, bool) {
	for _, pkg := range m.Packages {
		if pkg.Name == name {
			return pkg, true
		}
	}
	return manifest.Package{}, false
}

// moduleByName returns the module with the given name.
func moduleByName(m *manifest.Manifest, name string) (manifest.Module, bool) {
	for _, mod := range m.Modules {
		if mod.Name == name {
			return mod, true
		}
	}
	return manifest.Module{}, false
}

// availableModules returns the sorted, comma-joined module names for a usage
// error, or a placeholder when the manifest declares none.
func availableModules(m *manifest.Manifest) string {
	names := make([]string, 0, len(m.Modules))
	for _, mod := range m.Modules {
		names = append(names, mod.Name)
	}
	if len(names) == 0 {
		return "(none declared)"
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// setupHome resolves $HOME for the symlink engine, warning (best-effort) and
// falling back to "" when it cannot be resolved — a symlink step then fails fast
// in the engine, which is the honest outcome.
func setupHome(stderr io.Writer) string {
	home, err := os.UserHomeDir()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: resolve home dir: %v\n", progName, err)
		return ""
	}
	return home
}

// printSetupDryRun writes the ordered step plan as greppable key=value lines to
// stdout without executing anything or priming sudo.
func printSetupDryRun(stdout io.Writer, mod manifest.Module) error {
	lines := []string{"dry-run=true", "module=" + mod.Name}
	for i, step := range mod.Steps {
		kind, target, ok := step.Resolve()
		value := string(kind) + ":" + target
		if !ok {
			value = "invalid"
		}
		lines = append(lines, fmt.Sprintf("setup.step.%d=%s", i+1, value))
	}
	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return err
}

// printSetupResults writes one greppable result line per executed step to
// stdout, led by the module name.
func printSetupResults(stdout io.Writer, mod string, results []setupStepResult) error {
	lines := make([]string, 0, len(results)+1)
	lines = append(lines, "module="+mod)
	for _, res := range results {
		lines = append(lines, fmt.Sprintf("setup.result.%d=%s:%s:%s", res.index, res.kind, res.target, res.status))
	}
	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return err
}

// printSetupSummary writes an aggregate count and per-failure diagnostics to
// stderr, mirroring printInstallSummary.
func printSetupSummary(stderr io.Writer, mod string, results []setupStepResult) {
	var ok, failed int
	for _, res := range results {
		if res.status == "failed" {
			failed++
		} else {
			ok++
		}
	}
	_, _ = fmt.Fprintf(stderr, "%s: setup summary: module=%s steps=%d ok=%d failed=%d\n",
		progName, mod, len(results), ok, failed)
	for _, res := range results {
		if res.err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: step %d (%s:%s): %v\n", progName, res.index, res.kind, res.target, res.err)
		}
	}
}

// setupExitCode returns 1 when any step failed, else 0.
func setupExitCode(results []setupStepResult) int {
	for _, res := range results {
		if res.status == "failed" {
			return 1
		}
	}
	return 0
}
