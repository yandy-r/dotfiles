package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/plasma"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// This file is the COMMAND half of `dotf plasma`: the flag surface, the verb
// parse, the host and --only gates, and the interactive apply confirmation. The
// four verb orchestrations live in plasmaverbs.go, token encoding and the
// key=value line builders in plasmaprint.go, and the --json document shape with
// its builders in plasmajson.go, so all four stay under the ~500-line soft cap.

// plasmaVerbs is the ordered, closed set of `dotf plasma` verbs. It is the
// single source of truth for the dispatch switch, the usage-error messages, and
// (via completionPlasmaVerbs) the generated shell completions, so a fifth verb
// can never be dispatchable without also being completable.
var plasmaVerbs = []string{"export", "apply", "diff", "status"}

// plasmaConfirmPrompt is the interactive gate on `dotf plasma apply`. It is
// written to STDOUT, following runSysconfig's confirm path: on the interactive
// path stdout is the terminal, and a prompt on stderr would be invisible to a
// user who redirected it.
const plasmaConfirmPrompt = "apply KDE Plasma configuration? [y/N] "

// plasmaOpts holds the parsed flag values for `dotf plasma`.
type plasmaOpts struct {
	only         string
	dryRun       bool
	backup       bool
	link         bool
	unlink       bool
	recommended  bool
	reload       bool
	showVolatile bool
	yes          bool
	jsonOut      bool
}

// setPlasmaFlags registers plasma's flags on fs and returns the opts pointer
// they parse into. Single source of truth for plasma's flag surface, shared by
// runPlasma, the help renderer, and the completion generator (DRY — usage
// strings live only here; --json reuses the shared jsonFlagUsage const).
//
// One flagset serves all four verbs. Flags irrelevant to a verb are accepted and
// ignored rather than rejected, because per-verb rejection would need a second
// enumeration of the flag surface that could drift from this one. The three
// exceptions — --link, --unlink and --reload — are checked in runPlasma, since
// silently ignoring them would leave the user believing a destructive mode
// change or a session restart had happened. --show-volatile is deliberately NOT
// one of them:
// widening a report on a verb that emits no key detail is inert and harmless,
// which is exactly the class of flag the shared flagset exists to absorb.
func setPlasmaFlags(fs *flag.FlagSet) *plasmaOpts {
	o := &plasmaOpts{}
	fs.StringVar(&o.only, "only", "", "comma-separated managed file paths to act on (default: all)")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print the plan without executing")
	fs.BoolVar(&o.backup, "backup", false, "back up every live file the run modifies before writing")
	fs.BoolVar(&o.link, "link", false, "symlink each managed file into the repo instead of merging")
	fs.BoolVar(&o.unlink, "unlink", false,
		"restore copy mode: replace each symlinked managed file with a real file")
	fs.BoolVar(&o.recommended, "recommended", false,
		"layer the opt-in plasma/recommended overlay on top of plasma/files")
	fs.BoolVar(&o.reload, "reload", false, "restart the affected KDE components after applying")
	fs.BoolVar(&o.showVolatile, "show-volatile", false,
		"report the live-only keys export strips by design (diff; suppressed by default)")
	fs.BoolVar(&o.yes, "yes", false, "apply without an interactive confirmation prompt")
	fs.BoolVar(&o.jsonOut, "json", false, jsonFlagUsage)
	return o
}

// runPlasma manages the curated KDE Plasma 6 configuration set: export captures
// the live desktop into plasma/files/, apply merges the repo's keys back into
// the live files, and diff/status report per-key drift. All INI work is native
// Go (internal/plasma's KConfig codec); the injected exec.Runner feeds
// platform.Detect and the optional, opt-in live reload, nothing else.
//
// Exit codes: 0 converged, 1 failures (or drift, for diff), 2 usage, 130 on a
// Ctrl+C at the apply confirmation. Exit 3 is reserved for `dotf apply`'s hard
// aborts and plasma never uses it.
func runPlasma(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	fs := flag.NewFlagSet("plasma", flag.ContinueOnError)
	fs.SetOutput(stderr)

	o := setPlasmaFlags(fs)

	// Loop-parse to pull the single <verb> positional order-independently, so
	// `plasma export --dry-run` and `plasma --dry-run export` both work (Go's
	// flag stops at the first non-flag token — see runSetup/runInstall for the
	// same idiom).
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

	verb, ok := plasmaResolveVerb(positionals, stderr)
	if !ok {
		return 2
	}
	if code, ok := plasmaCheckApplyOnlyFlags(o, verb, stderr); !ok {
		return code
	}
	if code, ok := plasmaCheckExclusiveFlags(o, stderr); !ok {
		return code
	}

	// Resolved before the pre-flight failures so every one of them can emit the
	// same --json envelope a successful run would; a machine consumer that gets
	// zero bytes on stdout plus a non-zero exit has to guess.
	session := plasma.DetectSession(os.Getenv)
	display := plasma.DetectDisplayServer(os.Getenv)

	repoRoot := resolveRepoRoot()
	if repoRoot == "" {
		return plasmaFail(stdout, stderr, o, verb, session,
			errors.New("could not locate the dotfiles repo root (set DOTFILES_ROOT)"))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return plasmaFail(stdout, stderr, o, verb, session, fmt.Errorf("resolve home dir: %w", err))
	}
	configDir := xdgConfigHome(home)

	// A non-Linux host is an error, not a usage error: the invocation was
	// well-formed, the host simply has no KDE Plasma.
	info := platform.Detect(r)
	if plasma.Support(info) == plasma.SupportUnsupported {
		return plasmaFail(stdout, stderr, o, verb, session, &plasma.UnsupportedOSError{OS: info.OS})
	}

	files, err := plasmaSelectFiles(plasma.DefaultFiles(), o.only)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
		return 2
	}

	run := &plasmaRun{
		stdout: stdout,
		stderr: stderr,
		stdin:  os.Stdin,
		eng:    plasma.NewEngine(repoRoot, configDir, xdgDataHome(home), plasmaSystemColorSchemeDirs(), r),
		files:  files,
		o:      o,
		verb:   verb,
		popts: plasma.Options{
			DryRun:       o.dryRun,
			Backup:       o.backup,
			Link:         o.link,
			Unlink:       o.unlink,
			Recommended:  o.recommended,
			ShowVolatile: o.showVolatile,
			Reload:       o.reload,
			Session:      session,
			Display:      display,
		},
	}

	switch verb {
	case "export":
		return run.export()
	case "apply":
		return run.apply()
	case "diff":
		return run.diff()
	default:
		return run.status()
	}
}

// plasmaSystemColorSchemeDirs is the ordered system colour-scheme search path:
// "color-schemes" under every $XDG_DATA_DIRS entry. On a default host this is
// /usr/local/share/color-schemes then /usr/share/color-schemes — a superset of
// the single hard-coded directory it replaces.
func plasmaSystemColorSchemeDirs() []string {
	dataDirs := xdgDataDirs()
	dirs := make([]string, 0, len(dataDirs))
	for _, dir := range dataDirs {
		dirs = append(dirs, filepath.Join(dir, "color-schemes"))
	}
	return dirs
}

// plasmaResolveVerb extracts the single verb positional, reporting the exact
// arity or membership error to stderr. It returns ok=false for every usage
// error so the caller maps them all to one exit code.
func plasmaResolveVerb(positionals []string, stderr io.Writer) (string, bool) {
	available := strings.Join(plasmaVerbs, ", ")
	switch len(positionals) {
	case 0:
		_, _ = fmt.Fprintf(stderr, "%s: plasma requires a verb; available: %s\n", progName, available)
		return "", false
	case 1:
		// exactly one verb named — proceed.
	default:
		_, _ = fmt.Fprintf(stderr, "%s: plasma takes exactly one verb, got %d\n", progName, len(positionals))
		return "", false
	}

	verb := positionals[0]
	for _, known := range plasmaVerbs {
		if verb == known {
			return verb, true
		}
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown verb %q; available: %s\n", progName, verb, available)
	return "", false
}

// plasmaCheckApplyOnlyFlags rejects --link, --unlink and --reload on any verb
// but apply. All three would otherwise be silently ignored while promising
// something destructive (a mode switch in either direction) or disruptive
// (restarting plasmashell), which is exactly the class of no-op the
// shared-flagset design must not permit.
func plasmaCheckApplyOnlyFlags(o *plasmaOpts, verb string, stderr io.Writer) (int, bool) {
	if verb == "apply" {
		return 0, true
	}
	for _, f := range []struct {
		set  bool
		name string
	}{{o.link, "--link"}, {o.unlink, "--unlink"}, {o.reload, "--reload"}} {
		if f.set {
			_, _ = fmt.Fprintf(stderr, "%s: %s applies only to \"dotf plasma apply\"\n", progName, f.name)
			return 2, false
		}
	}
	return 0, true
}

// plasmaCheckExclusiveFlags rejects the mutually exclusive flag PAIRS — --link
// with --recommended, and --link with its own inverse --unlink — before any
// engine work happens.
//
// The engine guards the same combinations (plasma.LinkRecommendedError and
// plasma.LinkUnlinkError, raised inside PlanApply) and keeps doing so for the
// embedded `dotf apply --plasma` path, where they are phase failures. Reaching
// the CLI's typed-error path there meant exit 1, but combining two flags the
// tool documents as exclusive is a malformed INVOCATION, not a failed
// operation, and this repo's contract puts usage errors at exit 2 — the same
// code every other plasma flag rejection uses. The diagnostics are the engine's
// own error texts so the wording stays single-sourced.
func plasmaCheckExclusiveFlags(o *plasmaOpts, stderr io.Writer) (int, bool) {
	for _, pair := range []struct {
		set bool
		err error
	}{
		{o.link && o.recommended, &plasma.LinkRecommendedError{}},
		{o.link && o.unlink, &plasma.LinkUnlinkError{}},
	} {
		if pair.set {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, pair.err)
			return 2, false
		}
	}
	return 0, true
}

// plasmaSelectFiles narrows the managed table to the comma-separated --only
// set, preserving DefaultFiles order so the plan's phase ordering is unaffected.
// An unknown path fails fast rather than silently acting on nothing.
func plasmaSelectFiles(all []plasma.FileSpec, only string) ([]plasma.FileSpec, error) {
	if strings.TrimSpace(only) == "" {
		return all, nil
	}

	known := make(map[string]bool, len(all))
	for _, spec := range all {
		known[spec.Path] = true
	}

	want := make(map[string]bool)
	for _, raw := range strings.Split(only, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if !known[name] {
			return nil, fmt.Errorf("unknown --only path %q", name)
		}
		want[name] = true
	}

	selected := make([]plasma.FileSpec, 0, len(want))
	for _, spec := range all {
		if want[spec.Path] {
			selected = append(selected, spec)
		}
	}
	return selected, nil
}

// confirm gates the mutating apply path. It returns ok=false with the exit code
// to use for a refusal, a decline, or an abort.
func (p *plasmaRun) confirm() (int, bool) {
	if p.o.yes {
		return 0, true
	}
	if !stdoutIsTTY(p.stdout) {
		_, _ = fmt.Fprintf(p.stderr, "%s: refusing to run plasma apply without --yes (non-interactive)\n", progName)
		return 1, false
	}
	switch confirmPlasmaApply(p.stdout, p.stdin) {
	case confirmAborted:
		return reportAbort(p.stderr), false
	case confirmNo:
		_, _ = fmt.Fprintf(p.stderr, "%s: plasma not confirmed\n", progName)
		return 1, false
	default:
		return 0, true
	}
}

// confirmPlasmaApply prompts for the plasma apply and reads one line from stdin,
// reusing sysconfig's confirmDecision enum and its SIGINT discipline:
// signal.NotifyContext is installed before the prompt and stopped the moment the
// read returns, so a Ctrl+C during any later execution phase keeps Go's default
// terminate semantics.
//
// It does not call confirmApplyCtx because that function bakes in sysconfig's
// own prompt text, and parameterising it would mean editing the sysconfig
// command and its tests for a string this command owns.
func confirmPlasmaApply(stdout io.Writer, stdin io.Reader) confirmDecision {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return confirmPlasmaApplyCtx(ctx, stdout, stdin)
}

// confirmPlasmaApplyCtx is the pure, context-injected core of
// confirmPlasmaApply. It returns confirmAborted as soon as ctx is done (Ctrl+C),
// confirmYes for an explicit yes, and confirmNo for any other answer, EOF, or
// read error — a decline, deliberately, so an unreadable stdin never mutates the
// desktop. On abort the read goroutine is left blocked on stdin; the process is
// exiting, so that is harmless.
func confirmPlasmaApplyCtx(ctx context.Context, stdout io.Writer, stdin io.Reader) confirmDecision {
	_, _ = fmt.Fprint(stdout, plasmaConfirmPrompt)

	type readResult struct {
		line string
		err  error
	}
	resultCh := make(chan readResult, 1)
	go func() {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		resultCh <- readResult{line: line, err: err}
	}()

	select {
	case <-ctx.Done():
		// Break the line so the caller's "dotf: aborted" starts fresh, not
		// appended to the dangling prompt.
		_, _ = fmt.Fprintln(stdout)
		return confirmAborted
	case res := <-resultCh:
		if res.err != nil && res.line == "" {
			return confirmNo
		}
		switch strings.TrimSpace(strings.ToLower(res.line)) {
		case "y", "yes":
			return confirmYes
		default:
			return confirmNo
		}
	}
}
