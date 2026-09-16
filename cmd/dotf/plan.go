package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/tui"
)

// runPlan resolves the provisioning manifest, detects the host platform,
// drives the interactive TUI wizard, and prints a dry-run preview of the
// resulting selection as machine-greppable key=value lines to stdout. It
// applies nothing to the host — Phase 6 owns apply.
func runPlan(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(stderr)

	manifestPath, jsonOut := setReadFlags(fs)
	noUserConfig := setNoUserConfigFlag(fs)

	if err := fs.Parse(args); err != nil {
		return 2
	}

	m, err := manifest.Resolve(*manifestPath, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS,
		manifestOverlayOptions(noUserConfigEnabled(*noUserConfig), stderr))
	if err != nil {
		return reportErr(stderr, err)
	}

	info := platform.Detect(r)

	tty := os.Stdout
	if f, ok := stdout.(*os.File); ok {
		tty = f
	}
	accessible := !isTTY(tty) || os.Getenv("ACCESSIBLE") != ""

	sel, err := tui.Run(m, info, accessible)
	if err != nil {
		return wizardExit(stderr, err)
	}

	if !sel.Confirmed {
		_, _ = fmt.Fprintf(stderr, "%s: plan not confirmed\n", progName)
		return 1
	}

	if *jsonOut {
		if err := writePlanJSON(stdout, m, sel); err != nil {
			return reportErr(stderr, err)
		}
		return 0
	}

	if err := printPlanPreview(stdout, m, sel); err != nil {
		return reportErr(stderr, err)
	}

	return 0
}

// reportErr writes a progName-prefixed diagnostic to stderr and returns exit
// code 1. A failure to write the diagnostic itself is unactionable (stderr is
// already broken), so that write error is deliberately discarded.
func reportErr(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
	return 1
}

// isTTY reports whether f is attached to a character device (a terminal).
// This uses the dependency-free os.ModeCharDevice check rather than pulling
// in a terminal-detection library, per repo convention of minimizing new
// third-party dependencies.
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// printPlanPreview writes sel as machine-greppable key=value lines to
// stdout, mirroring runPlatform's stdout style in main.go. This is a
// dry-run preview only — nothing is applied. The whole preview is assembled
// in memory and emitted with a single checked write so a broken stdout is
// surfaced rather than silently ignored.
func printPlanPreview(stdout io.Writer, m *manifest.Manifest, sel tui.Selection) error {
	lines := []string{
		fmt.Sprintf("os=%s", sel.OS),
		fmt.Sprintf("intents=%s", strings.Join(sel.Intents, ",")),
		fmt.Sprintf("packages=%s", strings.Join(sel.Packages, ",")),
	}

	byBackend := groupPackagesByBackend(m, sel.Packages)
	for _, backend := range sortedBackendKeys(byBackend) {
		lines = append(lines, fmt.Sprintf("plan.%s=%s", backend, strings.Join(byBackend[backend], ",")))
	}

	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")

	return err
}

// groupPackagesByBackend resolves each selected package name to its
// manifest.Package and groups the names by Backend, so the preview can show
// what would be installed via each installation mechanism. Selected names
// that no longer exist in the manifest are silently skipped — the TUI only
// ever offers names the manifest defines, but Resolve's flag/env override
// path means the resolved manifest here could theoretically differ from the
// one the TUI collected packages against within a single process
// invocation; skipping avoids a spurious nil-Package group with an empty
// Backend key.
func groupPackagesByBackend(m *manifest.Manifest, packages []string) map[string][]string {
	index := packageIndex(m)

	groups := make(map[string][]string)
	for _, name := range packages {
		pkg, ok := index[name]
		if !ok {
			continue
		}
		backend := string(pkg.Backend)
		groups[backend] = append(groups[backend], name)
	}

	return groups
}

// packageIndex maps each manifest package name to its Package so a selected
// name can be resolved back to its backend. Names the manifest no longer
// defines are simply absent from the map (see groupPackagesByBackend for the
// override-path rationale behind skipping them).
func packageIndex(m *manifest.Manifest) map[string]manifest.Package {
	index := make(map[string]manifest.Package, len(m.Packages))
	for _, p := range m.Packages {
		index[p.Name] = p
	}
	return index
}

// planReport builds the stable JSON command report for a confirmed plan
// selection: one packageReport per selected package, resolving each name's
// Backend from the manifest (mirroring groupPackagesByBackend, including the
// skip of names the manifest no longer defines). Plan describes an intended
// selection, so every package carries Applicable=true and Action="install";
// Installed is omitted because plan probes nothing about the host.
func planReport(m *manifest.Manifest, sel tui.Selection) commandReport {
	index := packageIndex(m)

	reports := make([]packageReport, 0, len(sel.Packages))
	for _, name := range sel.Packages {
		pkg, ok := index[name]
		if !ok {
			continue
		}
		reports = append(reports, packageReport{
			Name:       name,
			Backend:    string(pkg.Backend),
			Applicable: true,
			Action:     "install",
			Class:      string(pkg.EffectiveClass()),
		})
	}

	return commandReport{Command: "plan", Packages: reports}
}

// writePlanJSON emits the plan selection as the stable JSON shape to stdout,
// the machine-readable counterpart to printPlanPreview. Nothing is applied.
func writePlanJSON(stdout io.Writer, m *manifest.Manifest, sel tui.Selection) error {
	return writeJSON(stdout, planReport(m, sel))
}

// sortedBackendKeys returns groups' keys in sorted order, so preview output
// is deterministic across runs.
func sortedBackendKeys(groups map[string][]string) []string {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys
}
