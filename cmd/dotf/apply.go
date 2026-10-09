package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/failure"
	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/plasma"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/privilege"
	"github.com/yandy-r/dotfiles/internal/symlink"
	"github.com/yandy-r/dotfiles/internal/sysconfig"
	"github.com/yandy-r/dotfiles/internal/tui"
)

// runApply orchestrates the full provisioning pipeline in PRD order —
// sysconfig identity → packages → symlinks → plasma → firewall — behind a
// headless-vs-guided gate with bootstrap.sh flag/env parity (see applyopts.go).
// Sudo is primed exactly ONCE for the whole run (a deliberate divergence from
// the per-subcommand priming in install.go/sysconfig.go); every engine emits
// literal sudo argv kept warm by the single keep-alive. Non-fatal step failures
// are accumulated in a failure.Collector so the run reaches the end, prints one
// summary, and returns one exit code.
//
// Exit codes: 0 = converged/all-applied; 1 = completed-with-failures (soft);
// 2 = usage error (flag parse / invalid --homebrew or firewall value); 3 = hard
// abort (manifest/home/repo resolve, sudo prime, or non-TTY refusal); 130 = user
// aborted the guided wizard (Ctrl+C/Esc). Code 3 is an apply-only additive
// extension over the sibling 0/1/2 convention (via the local reportFatal), and
// 130 is the Unix SIGINT convention (via wizardExit/reportAbort), so CI can
// distinguish a hard abort and a user cancellation from a soft failure.
func runApply(args []string, stdout, stderr io.Writer, r exec.Runner) (code int) {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(stderr)

	f := setApplyFlags(fs)

	if err := fs.Parse(args); err != nil {
		return 2
	}

	opts := resolveApplyOptions(*f, os.Getenv)

	if !validHomebrew(opts.Homebrew) {
		_, _ = fmt.Fprintf(stderr, "%s: invalid --homebrew %q: want one of \"\", true, false\n", progName, opts.Homebrew)
		return 2
	}

	m, err := manifest.Resolve(opts.Manifest, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS,
		manifestOverlayOptions(opts.NoUserConfig, stderr))
	if err != nil {
		return reportFatal(stderr, err)
	}

	// Load the lock beside the resolved manifest (embedded seed as fallback) so
	// the packages phase verifies pinned-artifact checksums. Reading it is a
	// file op only, so the dry-run path stays no-network.
	lock, err := lockfile.Resolve("", opts.Manifest, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS)
	if err != nil {
		return reportFatal(stderr, err)
	}

	// Reject unknown user-typed --intent names before resolveIntents runs, so
	// a typo cannot silently resolve to an empty package set and exit 0. This
	// single gate covers both the real apply path and the dry-run path.
	if err := validateIntents(m, opts.Intents); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
		return 2
	}

	info := platform.Detect(r)

	home, err := os.UserHomeDir()
	if err != nil {
		return reportFatal(stderr, fmt.Errorf("resolve home dir: %w", err))
	}
	repoRoot := resolveRepoRoot()
	if repoRoot == "" {
		return reportFatal(stderr, errors.New("could not locate the dotfiles repo root (set DOTFILES_ROOT)"))
	}

	fwMode := manifestFirewallMode(m)
	if !sysconfig.ValidFirewallMode(fwMode) {
		_, _ = fmt.Fprintf(stderr, "%s: invalid firewall %q: want one of ufw, firewalld, auto, off\n", progName, fwMode)
		return 2
	}

	sshMode := manifestSSHServerMode(m)
	if !sysconfig.ValidSSHServerMode(sshMode) {
		_, _ = fmt.Fprintf(stderr, "%s: invalid ssh-server %q: want one of on, off\n", progName, sshMode)
		return 2
	}

	// Mode gate. --dry-run composes previews and NEVER primes sudo. Headless
	// resolves the intent set non-interactively. A TTY without --headless drops
	// into the guided TUI, where the user's confirmed per-package selection
	// (sel.Packages) is honored verbatim. A non-TTY without --headless refuses
	// (hard abort).
	var pkgs []manifest.Package
	switch {
	case opts.DryRun:
		return applyDryRun(stdout, stderr, r, m, opts, info, home, repoRoot, fwMode, sshMode)
	case opts.Headless:
		pkgs = packagesForInstall(m, resolveIntents(m, opts))
	case stdoutIsTTY(stdout):
		accessible := os.Getenv("ACCESSIBLE") != ""
		sel, serr := tui.Run(m, info, accessible)
		if serr != nil {
			return wizardExit(stderr, serr)
		}
		if !sel.Confirmed {
			_, _ = fmt.Fprintf(stderr, "%s: apply not confirmed\n", progName)
			return 1
		}
		// Honor the user's exact confirmed package set — including deselecting
		// individual packages within an intent, or all of them. Re-deriving via
		// packagesForInstall(m, sel.Intents) would ignore per-package unchecks
		// and, on an empty intent set, wrongly install the entire manifest.
		pkgs = packagesByName(m, sel.Packages)
	default:
		return reportFatal(stderr, errors.New("refusing to apply without --headless/--force (non-interactive)"))
	}

	// Package-level --only/--skip narrowing, applied to whichever set the mode
	// gate resolved (headless intents or the guided selection) before the package
	// phase runs. --only selects independent of --intent; --skip excludes after.
	// An unknown package name is a usage error (exit 2), matching validateIntents.
	pkgs, err = filterPackages(m, pkgs, opts.Only, opts.Skip)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
		return 2
	}

	// P0b: prime sudo ONCE for the whole run (snapper needs root), keep it warm
	// across every phase, and always cancel the keep-alive on return.
	sudo := privilege.NewSudo()
	if err := sudo.Prime(r); err != nil {
		return reportFatal(stderr, fmt.Errorf("sudo prime: %w", err))
	}
	ctx, r, stop := withExecutionSignals(r)
	defer stop()
	defer func() {
		if ctxAbortExitCode(ctx) && code != 2 {
			_, _ = fmt.Fprintf(stderr, "%s: aborted\n", progName)
			code = abortExitCode
		}
	}()
	cancel := sudo.KeepAlive(ctx, r)
	defer cancel()

	// P1: best-effort snapper snapshot. Its outcome is a warning only — never
	// recorded in the collector, mirroring bootstrap's log_warning.
	if opts.Snapper {
		applySnapshot(stderr, r, info)
	}

	var coll failure.Collector

	// P1b: ensure the standard XDG/dev directories exist before symlinks and
	// tooling reference them. Idempotent; a failure is recorded, not fatal.
	applyDirs(home, &coll)

	// P1c: put the user-level tool directories on this process's PATH before any
	// package work. dotf inherits the invoking shell's PATH, which on a fresh
	// machine predates every directory the run is about to populate.
	applyUserToolPath(home)

	// P2..P5: identity sysconfig, packages, symlinks, firewall. Each phase
	// records its sub-outcomes into coll without aborting the run.
	eng := sysconfig.Default(r, "")
	idRes := applyIdentity(eng, m, info, &coll)

	// P3b: bootstrap Homebrew before packages so the pkg backend's `brew install`
	// has a working brew (macOS always; linuxbrew when --homebrew=true). A fresh
	// install is put on PATH and the platform re-detected so PkgMgrHomebrew is
	// selected for the packages phase; a bootstrap failure is non-fatal.
	info = applyBrew(r, stderr, info, opts, &coll)

	engine := backend.Default(r, sudo, privilege.NewDropper(), repoRoot, lock)
	results := applyPackages(ctx, engine, pkgs, info, opts.Force, &coll)

	se := symlink.NewEngine(repoRoot, home)
	specs := symlink.DefaultTable()
	sopts := applySymlinkOptions(opts)
	report := applySymlinks(se, specs, info, sopts, &coll)

	// P5b: the opt-in KDE Plasma merge. It runs AFTER symlinks — plasma writes
	// into $HOME/.config, exactly where symlinks were just provisioned, and the
	// two engines never claim the same path — and BEFORE firewall, which is
	// privileged system work and stays last. It is unprivileged $HOME work, so
	// no extra sudo prime is needed.
	var plasmaReport *plasma.Report
	if opts.Plasma {
		plasmaReport = applyPlasmaPhase(r, info, opts, repoRoot, home, stderr, &coll)
	}

	fwRes := applyFirewall(eng, fwMode, info, &coll)
	sshRes := applySSHServer(eng, sshMode, info, &coll)

	// P6: greppable stdout results, then the single stderr summary + exit code.
	return applySummary(stdout, stderr, mergeSysconfigResults(idRes, fwRes, sshRes), results, report, plasmaReport, &coll, sudo)
}

// applySummary writes the per-phase greppable result lines to stdout, then the
// run's aggregate summary (plus any advisory keep-alive failure) to stderr, and
// returns the collector's exit code (0 or 1).
func applySummary(stdout, stderr io.Writer, syscfg sysconfig.Result, results []installResult, report *symlink.Report, plasmaReport *plasma.Report, coll *failure.Collector, sudo *privilege.Sudo) int {
	if err := printSysconfigResults(stdout, syscfg); err != nil {
		return reportErr(stderr, err)
	}
	if err := printInstallResults(stdout, results); err != nil {
		return reportErr(stderr, err)
	}
	var applied []symlink.Action
	if report != nil {
		applied = report.Applied
	}
	if err := printLinkResults(stdout, applied); err != nil {
		return reportErr(stderr, err)
	}
	if plasmaReport != nil {
		if err := writePlasmaLines(stdout, plasmaActionLines("plasma", plasmaReport.Applied)); err != nil {
			return reportErr(stderr, err)
		}
	}

	coll.WriteSummary(stderr, progName)
	if err := sudo.KeepAliveErr(); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: sudo keep-alive: %v\n", progName, err)
	}
	return coll.ExitCode()
}

// reportFatal writes a progName-prefixed diagnostic to stderr and returns exit
// code 3. It marks an apply-only HARD abort (a failed precondition) distinct
// from reportErr's soft exit 1, so CI can tell a precondition failure from a
// completed-with-failures run. A failure to write the diagnostic is itself
// unactionable and deliberately discarded.
func reportFatal(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
	return 3
}

// validHomebrew reports whether the --homebrew tristate is one of the accepted
// values ("" for OS default, or an explicit true/false).
func validHomebrew(v string) bool {
	switch v {
	case "", "true", "false":
		return true
	default:
		return false
	}
}

// manifestFirewallMode returns the firewall backend declared in the manifest's
// optional [sysconfig] table, or FirewallUnset when no manifest default exists.
func manifestFirewallMode(m *manifest.Manifest) sysconfig.FirewallMode {
	if m.Sysconfig == nil {
		return sysconfig.FirewallUnset
	}
	return sysconfig.FirewallMode(m.Sysconfig.Firewall)
}

// manifestSSHServerMode returns the ssh-server toggle declared in the manifest's
// optional [sysconfig] table, or SSHServerUnset when no manifest default exists.
func manifestSSHServerMode(m *manifest.Manifest) sysconfig.SSHServerMode {
	if m.Sysconfig == nil {
		return sysconfig.SSHServerUnset
	}
	return sysconfig.SSHServerMode(m.Sysconfig.SSHServer)
}
