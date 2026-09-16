package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/selfupdate"
)

// selfUpdateRun is the injectable seam for the self-update engine so cmd tests
// exercise key=value/JSON rendering and exit codes without a network. Tests
// override it with a stub Result/error.
var selfUpdateRun = selfupdate.Run

// selfUpdateOpts holds the parsed flag values for `dotf self-update`.
type selfUpdateOpts struct {
	check   bool
	dryRun  bool
	force   bool
	jsonOut bool
}

// setSelfUpdateFlags registers self-update's flags on fs and returns the opts
// pointer they parse into. It is the single source of truth for the flag
// surface: runSelfUpdate calls it before fs.Parse, and the help/completion
// renderers call it on a throwaway flagset to derive the Flags section and
// completions (DRY).
func setSelfUpdateFlags(fs *flag.FlagSet) *selfUpdateOpts {
	o := &selfUpdateOpts{}
	fs.BoolVar(&o.check, "check", false, "report the latest version and whether an update exists; write nothing")
	fs.BoolVar(&o.dryRun, "dry-run", false, "resolve and report the target path/version without downloading or replacing")
	fs.BoolVar(&o.force, "force", false, "replace even when already current, newer, or an unparseable dev build")
	fs.BoolVar(&o.jsonOut, "json", false, jsonFlagUsage)
	return o
}

// runSelfUpdate discovers the latest dotf release and — unless --check/--dry-run
// — verifies and atomically replaces the running binary. It is scoped to the
// dotf binary only and takes no positional arguments.
func runSelfUpdate(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	fs := flag.NewFlagSet("self-update", flag.ContinueOnError)
	fs.SetOutput(stderr)

	o := setSelfUpdateFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "%s: self-update takes no arguments, got %q\n", progName, fs.Arg(0))
		return 2
	}

	info := platform.Detect(r)
	res, err := selfUpdateRun(selfupdate.Options{
		Force:  o.force,
		DryRun: o.dryRun,
		Check:  o.check,
	}, info)
	if err != nil {
		if errors.Is(err, selfupdate.ErrDevBuild) {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
			return 1
		}
		return reportErr(stderr, err)
	}

	if o.jsonOut {
		if err := writeJSON(stdout, buildSelfUpdateReport(res, o)); err != nil {
			return reportErr(stderr, err)
		}
		return 0
	}

	if err := writeSelfUpdateText(stdout, res, o); err != nil {
		return reportErr(stderr, err)
	}
	if res.Updated {
		_, _ = fmt.Fprintf(stderr, "%s: updated to %s; restart dotf to use the new build\n", progName, res.Latest)
	}
	return 0
}

// buildSelfUpdateReport maps the engine Result plus the requested flags into the
// stable --json shape.
func buildSelfUpdateReport(res selfupdate.Result, o *selfUpdateOpts) selfUpdateReport {
	return selfUpdateReport{
		Command:   "self-update",
		Current:   res.Current,
		Latest:    res.Latest,
		Available: res.Available,
		Asset:     res.Asset,
		Target:    res.Target,
		Updated:   res.Updated,
		DryRun:    o.dryRun,
		Checked:   o.check,
	}
}

// writeSelfUpdateText emits the greppable key=value report to stdout. --check
// and --dry-run are read-only and never print updated=true (they add checked=/
// dry-run= markers instead); the default run prints updated=BOOL.
func writeSelfUpdateText(w io.Writer, res selfupdate.Result, o *selfUpdateOpts) error {
	var b strings.Builder
	fmt.Fprintf(&b, "current=%s\n", res.Current)
	fmt.Fprintf(&b, "latest=%s\n", res.Latest)
	fmt.Fprintf(&b, "update.available=%t\n", res.Available)
	fmt.Fprintf(&b, "asset=%s\n", res.Asset)
	if res.Target != "" {
		fmt.Fprintf(&b, "target=%s\n", res.Target)
	}
	switch {
	case o.check:
		b.WriteString("checked=true\n")
	case o.dryRun:
		b.WriteString("dry-run=true\n")
	default:
		fmt.Fprintf(&b, "updated=%t\n", res.Updated)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
