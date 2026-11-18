package main

import (
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
)

// runStatus resolves the manifest, probes installed state for every declared
// package (read-only), and prints sorted greppable key=value lines to stdout.
func runStatus(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	return runDrift(args, stdout, stderr, r, false)
}

// runDiff resolves the manifest, probes installed state (read-only), and prints
// only declared-not-installed drift as greppable key=value lines. Returns exit
// code 1 when any drift is present.
func runDiff(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	code, err := runDriftWithCount(args, stdout, stderr, r, true)
	if err != nil {
		return reportErr(stderr, err)
	}
	return code
}

func runDrift(args []string, stdout, stderr io.Writer, r exec.Runner, onlyMissing bool) int {
	code, err := runDriftWithCount(args, stdout, stderr, r, onlyMissing)
	if err != nil {
		return reportErr(stderr, err)
	}
	return code
}

func runDriftWithCount(args []string, stdout, stderr io.Writer, r exec.Runner, onlyMissing bool) (int, error) {
	fs := flag.NewFlagSet("drift", flag.ContinueOnError)
	fs.SetOutput(stderr)

	manifestPath, jsonOut, counts := setDriftFlags(fs)
	noUserConfig := setNoUserConfigFlag(fs)

	if err := fs.Parse(args); err != nil {
		return 2, nil
	}

	m, err := manifest.Resolve(*manifestPath, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS,
		manifestOverlayOptions(noUserConfigEnabled(*noUserConfig), stderr))
	if err != nil {
		return 1, err
	}

	info := platform.Detect(r)
	engine := backend.Default(r, nil, os.Getenv("DOTFILES_ROOT"), nil)

	driftCount, err := driftReport(stdout, m, engine, info, onlyMissing, *jsonOut, *counts)
	if err != nil {
		return 1, err
	}

	if onlyMissing && driftCount > 0 {
		return 1, nil
	}

	return 0, nil
}

// driftReport enumerates m.Packages, calls engine.Installed for each package
// (read-only), sorts by package name, and writes the result to stdout. Packages
// whose backend cannot run on this host (repo on a non-apt host, aur on
// non-Arch) are never probed: status emits them with applicable=false, diff
// excludes them from drift entirely. When onlyMissing is false every applicable
// package is emitted with installed=true|false; when true only packages with
// installed=false are emitted with drift=declared-not-installed.
//
// The probe pass builds the human key=value lines and the structured JSON rows
// in a single loop so the two renderings can never disagree. When jsonOut is
// false the greppable lines are written (byte-identical to before --json
// existed); when true a commandReport is emitted instead. driftCount is
// returned regardless of format so diff's exit code is unaffected by --json.
//
// counts (the --counts flag) appends an aggregate backend/class histogram: sorted
// count.* lines in text mode, a commandReport.Summary object in JSON mode. It is
// purely additive — with counts=false the output is byte-identical to before.
// Every row carries the package's effective class so the summary and the JSON
// rows agree.
func driftReport(stdout io.Writer, m *manifest.Manifest, engine *backend.Engine, info platform.Info, onlyMissing, jsonOut, counts bool) (driftCount int, err error) {
	pkgs := append([]manifest.Package(nil), m.Packages...)
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].Name < pkgs[j].Name
	})

	lines := make([]string, 0, len(pkgs))
	reports := make([]packageReport, 0, len(pkgs))
	for _, pkg := range pkgs {
		class := string(pkg.EffectiveClass())
		applicable, probeErr := engine.Applicable(pkg, info)
		if probeErr != nil {
			return 0, fmt.Errorf("package %q: %w", pkg.Name, probeErr)
		}
		if !applicable {
			if !onlyMissing {
				lines = append(lines, fmt.Sprintf("package=%s backend=%s applicable=false", pkg.Name, pkg.Backend))
				reports = append(reports, packageReport{Name: pkg.Name, Backend: string(pkg.Backend), Applicable: false, Class: class})
			}
			continue
		}

		installed, probeErr := engine.Installed(pkg, info)
		if probeErr != nil {
			return 0, fmt.Errorf("package %q: %w", pkg.Name, probeErr)
		}

		if onlyMissing {
			if installed {
				continue
			}
			driftCount++
			lines = append(lines, fmt.Sprintf("drift=declared-not-installed package=%s backend=%s", pkg.Name, pkg.Backend))
			notInstalled := false
			reports = append(reports, packageReport{Name: pkg.Name, Backend: string(pkg.Backend), Applicable: true, Installed: &notInstalled, Action: "install", Class: class})
			continue
		}

		lines = append(lines, fmt.Sprintf("package=%s backend=%s installed=%t", pkg.Name, pkg.Backend, installed))
		state := installed
		reports = append(reports, packageReport{Name: pkg.Name, Backend: string(pkg.Backend), Applicable: true, Installed: &state, Class: class})
	}

	if jsonOut {
		command := "status"
		if onlyMissing {
			command = "diff"
		}
		cr := commandReport{Command: command, Packages: reports}
		if counts {
			s := summarize(reports)
			cr.Summary = &s
		}
		return driftCount, writeJSON(stdout, cr)
	}

	if counts {
		lines = append(lines, countLines(summarize(reports))...)
	}

	if len(lines) == 0 {
		return driftCount, nil
	}

	_, err = io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return driftCount, err
}

// countLines renders a classSummary as sorted greppable count.* lines: the
// total first, then per-backend counts, then per-class counts, each group
// sorted so the output is deterministic and grep-friendly (grep '^count\.').
func countLines(s classSummary) []string {
	lines := []string{fmt.Sprintf("count.total=%d", s.Total)}

	backends := make([]string, 0, len(s.ByBackend))
	for b := range s.ByBackend {
		backends = append(backends, b)
	}
	sort.Strings(backends)
	for _, b := range backends {
		lines = append(lines, fmt.Sprintf("count.backend.%s=%d", b, s.ByBackend[b]))
	}

	classes := make([]string, 0, len(s.ByClass))
	for c := range s.ByClass {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	for _, c := range classes {
		lines = append(lines, fmt.Sprintf("count.class.%s=%d", c, s.ByClass[c]))
	}

	return lines
}
