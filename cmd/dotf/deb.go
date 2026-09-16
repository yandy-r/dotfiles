package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/yandy-r/dotfiles/internal/debtap"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// This file is the whole of `dotf deb`: the flag surface, the positional parse,
// the stage-by-stage orchestration of internal/debtap, the pre-install
// confirmation, and both output renderings. The conversion logic itself lives
// in internal/debtap so this layer stays a thin, testable driver.

// debConfirmPrompt gates the one step that elevates. It is written to STDOUT,
// following runPlasma and runSysconfig: on the interactive path stdout is the
// terminal, and a prompt on stderr would be invisible to anyone who redirected
// it.
// The prompt names the real consequence. debtap carries the .deb's Debian
// maintainer scripts into the Arch package's .INSTALL scriptlet, so answering
// yes executes shell the package author wrote, as root. That is inherent to
// converting a .deb, but the user should be told before authorizing it.
const debConfirmPrompt = "install the converted package? it runs the package's install scripts as root [y/N] "

// debRemedyPrompt gates each preflight fix.
const debRemedyPrompt = "run this command now? [y/N] "

// debDBDir and debPkgfileDir locate the system caches the preflight inspects.
// They are package vars rather than constants so tests can point them at temp
// directories instead of the real /var/cache trees, following the same
// override-for-tests idiom as privilege.keepAliveInterval.
var (
	debDBDir      = debtap.DefaultDBDir
	debPkgfileDir = debtap.DefaultPkgfileDir
)

// maxRemedies bounds the preflight fix loop. Each accepted remedy is re-checked
// by re-running the preflight, so a remedy that does not actually resolve its
// own precondition stops the loop instead of prompting forever.
const maxRemedies = 6

// debOpts holds the parsed flag values for `dotf deb`.
type debOpts struct {
	install bool
	yes     bool
	force   bool
	outDir  string
	sha256  string
	pkgName string
	pkgVer  string
	keep    bool
	dryRun  bool
	jsonOut bool
}

// setDebFlags registers deb's flags on fs and returns the opts pointer they
// parse into. Single source of truth for the flag surface, shared by runDeb,
// the help renderer, and the completion generator (DRY — usage strings live
// only here). --out-dir's usage says "path" so the completion generator offers
// file completion for its argument.
func setDebFlags(fs *flag.FlagSet) *debOpts {
	o := &debOpts{}
	fs.BoolVar(&o.install, "install", false,
		"install the converted package after verifying it (default: convert only)")
	fs.BoolVar(&o.yes, "yes", false, "install without an interactive confirmation prompt")
	fs.BoolVar(&o.force, "force", false,
		"convert even when a native Arch package of the same name already exists")
	fs.StringVar(&o.outDir, "out-dir", ".", "path to the directory the converted package is written to")
	fs.StringVar(&o.sha256, "sha256", "", "expected SHA-256 digest of the input .deb")
	fs.StringVar(&o.pkgName, "pkgname", "", "override the converted package's name")
	fs.StringVar(&o.pkgVer, "pkgver", "", "override the converted package's version")
	fs.BoolVar(&o.keep, "keep", false, "keep the staged input and the debtap work directory")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print the plan without executing")
	fs.BoolVar(&o.jsonOut, "json", false, jsonFlagUsage)
	return o
}

// runDeb converts a Debian package into a pacman-installable Arch package and,
// with --install, installs it.
//
// Exit codes: 0 success, 1 soft failure (unsupported host, missing debtap, a
// malformed or mismatched input, the precedence gate, a failed conversion or
// install), 2 usage, 130 on a Ctrl+C at the install confirmation. Exit 3 is
// reserved for `dotf apply`'s hard aborts and this command never uses it.
func runDeb(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	fs := flag.NewFlagSet("deb", flag.ContinueOnError)
	fs.SetOutput(stderr)

	o := setDebFlags(fs)

	// Loop-parse to pull the single <url|path> positional order-independently,
	// so `deb ./x.deb --install` and `deb --install ./x.deb` both work (Go's
	// flag package stops at the first non-flag token — same idiom as
	// runPlasma/runSetup).
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

	switch len(positionals) {
	case 1:
	case 0:
		_, _ = fmt.Fprintf(stderr, "%s deb: missing <url|path> argument\n", progName)
		return 2
	default:
		_, _ = fmt.Fprintf(stderr, "%s deb: want exactly one <url|path>, got %d: %s\n",
			progName, len(positionals), strings.Join(positionals, " "))
		return 2
	}

	// --yes only means anything alongside --install; silently accepting it
	// otherwise would let a user believe an install was authorized when the
	// command was never going to install anything.
	if o.yes && !o.install {
		_, _ = fmt.Fprintf(stderr, "%s deb: --yes requires --install\n", progName)
		return 2
	}

	outDir, err := filepath.Abs(o.outDir)
	if err != nil {
		return reportErr(stderr, fmt.Errorf("resolve --out-dir: %w", err))
	}

	opts := debtap.Options{
		Input:   positionals[0],
		OutDir:  outDir,
		SHA256:  o.sha256,
		PkgName: o.pkgName,
		PkgVer:  o.pkgVer,
		Keep:    o.keep,
	}

	// `sudo dotf deb …` is a natural reflex, since the command openly needs
	// sudo for the install and its own preflight suggests `sudo debtap -u`.
	// Under sudo, though, the download, the bsdtar extraction of an untrusted
	// control tarball, and debtap's whole conversion would run as uid 0 — and
	// root bsdtar additionally restores ownership and setuid bits an
	// unprivileged extraction drops. Only the pacman step should ever elevate.
	if os.Geteuid() == 0 && os.Getenv("SUDO_USER") != "" {
		return reportErr(stderr, errors.New(
			"refusing to run as root; run as your normal user — "+
				"only the install step elevates, and it will prompt for sudo itself"))
	}

	conv := debtap.New(r, debtap.Dirs{
		Cache: debCacheDir(), DB: debDBDir, Pkgfile: debPkgfileDir,
	})

	if code, ok := debPreflight(stdout, stderr, o, conv, platform.Detect(r), opts.Input); !ok {
		return code
	}

	if o.dryRun {
		printDebPlan(stdout, o, opts)
		return 0
	}

	return debRun(stdout, stderr, o, opts, conv)
}

// debRun executes the pipeline stage by stage. Each stage records what it
// established into res BEFORE the next can fail, so a partial result from a
// failed run is still reported — a machine consumer that gets a non-zero exit
// and zero bytes on stdout has to guess how far the run got.
func debRun(stdout, stderr io.Writer, o *debOpts, opts debtap.Options, conv *debtap.Converter) int {
	var res debtap.Result
	code, err := debPipeline(stdout, stderr, o, opts, conv, &res)

	// Cleanup runs between the pipeline and the report, so the retained-path
	// fields are populated before the result renders — and so a run that failed
	// partway still cleans up its staging directory.
	res.StagedDir = conv.Cleanup(opts.Keep)

	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
	}
	printDebResult(stdout, o, res)
	return code
}

// debPipeline runs the stages and returns the exit code plus any error to
// report. It never prints the result itself: debRun owns that, so there is
// exactly one place where output is rendered and it always runs after cleanup.
//
// Each stage records what it established into res BEFORE the next can fail, so
// a partial result from a failed run is still reported — a machine consumer
// that gets a non-zero exit and zero bytes on stdout has to guess how far the
// run got.
func debPipeline(stdout, stderr io.Writer, o *debOpts, opts debtap.Options,
	conv *debtap.Converter, res *debtap.Result) (int, error) {
	// Input is what the user typed, so a URL run reports the URL rather than
	// the private path it was staged into.
	res.Input = opts.Input

	local, source, sum, err := conv.Acquire(opts)
	res.Source, res.SHA256 = source, sum
	if err != nil {
		return 1, err
	}

	ctl, err := conv.Control(local)
	if err != nil {
		return 1, err
	}
	res.DebPackage, res.DebVersion = ctl.Package, ctl.Version

	// A gate that could not run must never look like a gate that passed, so a
	// query failure aborts rather than degrading to "no native package".
	prec, err := conv.Precedence(ctl.Package)
	if err != nil {
		return 1, err
	}
	res.Precedence = prec
	if !debCheckPrecedence(stderr, o, *res) {
		// The gate wrote its own multi-line diagnostic, so no error is
		// returned for debRun to restate.
		return 1, nil
	}

	artifact, workDir, err := conv.Convert(local, opts)
	res.WorkDir = workDir
	if err != nil {
		return 1, err
	}
	res.Artifact = artifact

	info, err := conv.Inspect(artifact)
	if err != nil {
		return 1, err
	}
	res.Package, res.Version, res.Size, res.Depends = info.Name, info.Version, info.Size, info.Depends

	// Re-run the gate against the name that will ACTUALLY be installed. The
	// pre-conversion check used the .deb's self-declared Package field, but
	// debtap (or --pkgname) can emit a package under a different name — so a
	// .deb declaring an obscure name would otherwise slip past the gate and
	// install something that shadows a real repo package.
	if info.Name != "" && info.Name != ctl.Package {
		p, err := conv.Precedence(info.Name)
		if err != nil {
			return 1, err
		}
		if p != debtap.PrecedenceNone {
			res.Precedence = p
			if !debCheckPrecedence(stderr, o, debtap.Result{
				Precedence: p, DebPackage: info.Name,
			}) {
				return 1, nil
			}
		}
	}

	res.Unsatisfied, res.Unresolvable, err = conv.CheckDeps(info.Depends)
	if err != nil {
		return 1, err
	}
	debWarnDeps(stderr, *res)

	if o.install {
		// Reject a flag-like package name BEFORE installing rather than after:
		// VerifyInstalled cannot query such a name, so installing first would
		// leave a real package on the system reported as "not installed".
		if !debInstallableName(info.Name) {
			return 1, fmt.Errorf(
				"refusing to install: the built package's name %q is not a usable package name",
				info.Name)
		}
		if code, ok := debConfirm(stdout, stderr, o); !ok {
			return code, nil
		}
		if err := conv.Install(artifact, o.yes); err != nil {
			return 1, err
		}
		res.Installed, res.Files = conv.VerifyInstalled(info.Name)
		if !res.Installed {
			return 1, fmt.Errorf(
				"pacman reports %s is not installed after a successful install", info.Name)
		}
	}

	return 0, nil
}

// debInstallableName reports whether name is a usable pacman package name.
// It mirrors internal/debtap's safeOperand: a name beginning with "-" would be
// parsed by pacman as a flag, so nothing downstream could query it.
func debInstallableName(name string) bool {
	return name != "" && !strings.HasPrefix(name, "-")
}

// debCheckPrecedence enforces the native-package gate. A hit in the official
// repositories is a hard stop unless --force: converting a .deb when a real
// package exists produces one that shadows it and never receives updates. An
// AUR hit is advisory only, because AUR naming is loose enough that blocking on
// it would be noise rather than protection.
func debCheckPrecedence(stderr io.Writer, o *debOpts, res debtap.Result) bool {
	switch res.Precedence {
	case debtap.PrecedenceRepo:
		if o.force {
			_, _ = fmt.Fprintf(stderr,
				"%s: warning: %q is in the official repositories; converting anyway (--force)\n",
				progName, res.DebPackage)
			return true
		}
		_, _ = fmt.Fprintf(stderr,
			"%s: %q is already in the official repositories; install it with `sudo pacman -S %s`.\n"+
				"%s: a converted .deb shadows the native package and never receives updates. "+
				"Re-run with --force to convert anyway.\n",
			progName, res.DebPackage, res.DebPackage, progName)
		return false
	case debtap.PrecedenceAUR:
		_, _ = fmt.Fprintf(stderr,
			"%s: warning: %q also exists in the AUR; a native package is usually preferable\n",
			progName, res.DebPackage)
	}
	return true
}

// debWarnDeps reports dependency problems debtap's approximate Debian-to-Arch
// translation produced. These are warnings, never failures: pacman is the
// authority on installability and gives a better diagnostic than a pre-flight
// guess.
func debWarnDeps(stderr io.Writer, res debtap.Result) {
	if len(res.Unresolvable) == 0 {
		return
	}
	_, _ = fmt.Fprintf(stderr,
		"%s: warning: %d translated dependency/dependencies resolve to no known package: %s\n",
		progName, len(res.Unresolvable), strings.Join(res.Unresolvable, " "))
	_, _ = fmt.Fprintf(stderr,
		"%s: debtap's Debian-to-Arch dependency mapping is approximate; "+
			"pacman will reject the install if they are genuinely required\n", progName)
}

// debConfirm gates the install. It returns ok=false with the exit code to use
// for a refusal, a decline, or an abort.
func debConfirm(stdout, stderr io.Writer, o *debOpts) (int, bool) {
	if o.yes {
		return 0, true
	}
	if !stdoutIsTTY(stdout) {
		_, _ = fmt.Fprintf(stderr,
			"%s: refusing to install without --yes (non-interactive)\n", progName)
		return 1, false
	}
	switch confirmDebInstall(stdout, os.Stdin) {
	case confirmAborted:
		return reportAbort(stderr), false
	case confirmNo:
		_, _ = fmt.Fprintf(stderr, "%s: install not confirmed\n", progName)
		return 1, false
	default:
		return 0, true
	}
}

// confirmDebInstall prompts for the install and reads one line from stdin,
// reusing sysconfig's confirmDecision enum and its SIGINT discipline:
// signal.NotifyContext is installed before the prompt and stopped the moment
// the read returns, so a Ctrl+C during the pacman run that follows keeps Go's
// default terminate semantics.
func confirmDebInstall(stdout io.Writer, stdin io.Reader) confirmDecision {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return confirmDebInstallCtx(ctx, stdout, debConfirmPrompt, stdin)
}

// confirmDebInstallCtx is the pure, context-injected core of
// confirmDebInstall. It returns confirmAborted as soon as ctx is done (Ctrl+C),
// confirmYes for an explicit yes, and confirmNo for any other answer, EOF, or
// read error — a decline, deliberately, so an unreadable stdin never installs
// anything.
func confirmDebInstallCtx(ctx context.Context, stdout io.Writer, prompt string, stdin io.Reader) confirmDecision {
	_, _ = fmt.Fprint(stdout, prompt)

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
		// Break the line so the caller's "dotf: aborted" starts fresh rather
		// than appended to the dangling prompt.
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

// debFail reports err (when non-nil), emits whatever the run established, and
// returns exit 1. The partial result is printed even on failure so --json
// consumers always receive a document.
func debFail(stdout, stderr io.Writer, o *debOpts, res debtap.Result, err error) int {
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
	}
	printDebResult(stdout, o, res)
	return 1
}

// debCacheDir is where downloaded .deb files are staged, following the XDG base
// directory spec.
func debCacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "dotf", "deb")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "dotf-deb")
	}
	return filepath.Join(home, ".cache", "dotf", "deb")
}

// ----------------------------------------------------------------------------
// output
// ----------------------------------------------------------------------------

// debJSON is the --json document. Fields mirror the key=value keys one-for-one
// so the two renderings can never describe different things.
type debJSON struct {
	Source       string   `json:"source,omitempty"`
	Input        string   `json:"input,omitempty"`
	StagedDir    string   `json:"staged_dir,omitempty"`
	WorkDir      string   `json:"work_dir,omitempty"`
	SHA256       string   `json:"sha256,omitempty"`
	DebPackage   string   `json:"deb_package,omitempty"`
	DebVersion   string   `json:"deb_version,omitempty"`
	Precedence   string   `json:"precedence,omitempty"`
	Artifact     string   `json:"artifact,omitempty"`
	Package      string   `json:"package,omitempty"`
	Version      string   `json:"version,omitempty"`
	Size         string   `json:"size,omitempty"`
	Depends      []string `json:"depends,omitempty"`
	Unsatisfied  []string `json:"depend_unsatisfied,omitempty"`
	Unresolvable []string `json:"depend_unresolvable,omitempty"`
	// Installed is a pointer so a run that produced no package omits it
	// entirely rather than asserting "installed": false — a fact it never
	// established. This keeps the two renderings describing the same thing;
	// the key=value side already gates the line on Artifact.
	Installed *bool `json:"installed,omitempty"`
	Files     int   `json:"files,omitempty"`
}

// printDebPlan renders --dry-run: what the run would do, having executed
// nothing. Preflight has already run (it only reads), so reaching this point is
// itself the useful signal that the host is capable of the conversion.
func printDebPlan(w io.Writer, o *debOpts, opts debtap.Options) {
	if o.jsonOut {
		// --json must hold on every path, including this one: a consumer
		// piping to jq gets a parse error otherwise. plasma's report carries
		// its own dryRun field for the same reason.
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(debPlanJSON{
			DryRun: true, Input: opts.Input, OutDir: opts.OutDir, Install: o.install,
		})
		return
	}
	_, _ = fmt.Fprintf(w, "dry-run=true\n")
	_, _ = fmt.Fprintf(w, "input=%s\n", opts.Input)
	_, _ = fmt.Fprintf(w, "out_dir=%s\n", opts.OutDir)
	_, _ = fmt.Fprintf(w, "install=%t\n", o.install)
}

// debPlanJSON is the --dry-run --json document.
type debPlanJSON struct {
	DryRun  bool   `json:"dryRun"`
	Input   string `json:"input,omitempty"`
	OutDir  string `json:"out_dir,omitempty"`
	Install bool   `json:"install"`
}

// printDebResult renders the run's facts as key=value lines, or as a JSON
// document with --json. Empty fields are omitted so the output only ever
// asserts what the run actually established.
func printDebResult(w io.Writer, o *debOpts, res debtap.Result) {
	if o.jsonOut {
		doc := debJSON{
			Source: res.Source, Input: res.Input,
			StagedDir: res.StagedDir, WorkDir: res.WorkDir,
			SHA256: res.SHA256,
			DebPackage: res.DebPackage, DebVersion: res.DebVersion,
			Precedence: res.Precedence, Artifact: res.Artifact,
			Package: res.Package, Version: res.Version, Size: res.Size,
			Depends: res.Depends, Unsatisfied: res.Unsatisfied,
			Unresolvable: res.Unresolvable, Files: res.Files,
		}
		if res.Artifact != "" {
			installed := res.Installed
			doc.Installed = &installed
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(doc)
		return
	}

	writeKV(w, "source", res.Source)
	writeKV(w, "input", res.Input)
	writeKV(w, "staged_dir", res.StagedDir)
	writeKV(w, "work_dir", res.WorkDir)
	writeKV(w, "sha256", res.SHA256)
	writeKV(w, "deb_package", res.DebPackage)
	writeKV(w, "deb_version", res.DebVersion)
	writeKV(w, "precedence", res.Precedence)
	writeKV(w, "artifact", res.Artifact)
	writeKV(w, "package", res.Package)
	writeKV(w, "version", res.Version)
	writeKV(w, "size", res.Size)
	if len(res.Depends) > 0 {
		_, _ = fmt.Fprintf(w, "depends=%d\n", len(res.Depends))
	}
	for _, dep := range res.Unsatisfied {
		_, _ = fmt.Fprintf(w, "depend_unsatisfied=%s\n", dep)
	}
	for _, dep := range res.Unresolvable {
		_, _ = fmt.Fprintf(w, "depend_unresolvable=%s\n", dep)
	}
	if res.Artifact != "" {
		_, _ = fmt.Fprintf(w, "installed=%t\n", res.Installed)
	}
	if res.Files > 0 {
		_, _ = fmt.Fprintf(w, "files=%d\n", res.Files)
	}
}

// writeKV emits one key=value line, skipping empty values so the output never
// claims a fact the run did not establish.
func writeKV(w io.Writer, key, value string) {
	if value == "" {
		return
	}
	_, _ = fmt.Fprintf(w, "%s=%s\n", key, value)
}

// ----------------------------------------------------------------------------
// preflight remediation
// ----------------------------------------------------------------------------

// debPreflight verifies the host and, for a failure that has a known fix,
// explains it and offers to run that fix.
//
// debtap emits ONE message for five different preconditions ("run debtap -u"),
// which is misleading for the ones syncing cannot fix — a user can re-run it
// indefinitely and never learn why. This loop diagnoses the specific failure,
// says what the fix does and does not touch, and re-runs the preflight after
// each accepted remedy so a fix that did not actually work stops the loop
// rather than looping forever.
//
// Nothing is ever run without an explicit yes: without a terminal the fix is
// printed for the user to run themselves.
func debPreflight(stdout, stderr io.Writer, o *debOpts, conv *debtap.Converter,
	info platform.Info, input string) (int, bool) {
	for attempt := 0; ; attempt++ {
		err := conv.Preflight(info)
		if err == nil {
			return 0, true
		}

		var pe *debtap.PreflightError
		if !errors.As(err, &pe) || !pe.Fixable() || attempt >= maxRemedies {
			return debFail(stdout, stderr, o, debtap.Result{Input: input}, err), false
		}

		switch debOfferRemedy(stdout, stderr, pe) {
		case confirmYes:
			if rerr := conv.ApplyRemedy(pe.Remedy); rerr != nil {
				return debFail(stdout, stderr, o, debtap.Result{Input: input}, rerr), false
			}
			_, _ = fmt.Fprintf(stderr, "%s: done; re-checking\n", progName)
		case confirmAborted:
			return reportAbort(stderr), false
		default:
			// Declined, or no terminal to ask at. debOfferRemedy already wrote
			// the full diagnosis, so debFail is given a nil error rather than
			// printing the same paragraph a second time.
			return debFail(stdout, stderr, o, debtap.Result{Input: input}, nil), false
		}
	}
}

// debOfferRemedy prints the diagnosis and the fix, then asks. It returns
// confirmNo without prompting when there is no terminal to prompt at, having
// printed the command for the user to run themselves.
func debOfferRemedy(stdout, stderr io.Writer, pe *debtap.PreflightError) confirmDecision {
	// The diagnosis is a diagnostic, so it goes to stderr; only the prompt
	// itself goes to stdout, where an interactive user is looking.
	_, _ = fmt.Fprintf(stderr, "\n%s: %s\n\n", progName, pe.Problem)
	writeIndented(stderr, pe.Remedy.Detail)
	_, _ = fmt.Fprintf(stderr, "\n  Command:  %s\n", shellQuoteArgv(pe.Remedy.Argv))
	if len(pe.Remedy.Undo) > 0 {
		_, _ = fmt.Fprintf(stderr, "  Undo:     %s\n", shellQuoteArgv(pe.Remedy.Undo))
	}
	_, _ = fmt.Fprintln(stderr)

	if !stdoutIsTTY(stdout) {
		_, _ = fmt.Fprintf(stderr,
			"%s: no terminal to confirm at; run the command above, then retry\n", progName)
		return confirmNo
	}
	return confirmDebRemedy(stdout, os.Stdin)
}

// confirmDebRemedy prompts for a preflight fix, reusing sysconfig's
// confirmDecision enum and its SIGINT discipline.
func confirmDebRemedy(stdout io.Writer, stdin io.Reader) confirmDecision {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return confirmDebInstallCtx(ctx, stdout, debRemedyPrompt, stdin)
}

// shellQuoteArgv renders an argv as a copy-pasteable command line, quoting only
// the elements that need it. The argv is never passed through a shell — this is
// for display, so a user can run the same thing by hand.
func shellQuoteArgv(argv []string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		if a == "" || strings.ContainsAny(a, " \t\n\"'\\$`&|;<>()*?[]{}~#!") {
			quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
			continue
		}
		quoted[i] = a
	}
	return strings.Join(quoted, " ")
}
