package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/privilege"
	"github.com/yandy-r/dotfiles/internal/sysconfig"
)

// runSysconfig resolves the manifest and CLI flags into a desired system
// Config (flag > manifest > skip), detects the host platform, and drives the
// sysconfig engine's plan/apply split for hostname/timezone/locale/shell/
// firewall. --dry-run never primes sudo (its ufw probe is `sudo -n` and fails
// closed to "status unknown"). The real apply path is gated behind an
// interactive confirmation (or --yes) and primes sudo once with a keep-alive
// before executing the plan. The one exception is a managed ufw firewall: its
// idempotency probe needs root, so sudo is primed BEFORE Plan (even when the
// host turns out converged) so a converged host still reports converged rather
// than re-enabling ufw every run. It returns exit code 1 when any step fails (or
// the confirmation is declined), and 130 when the user aborts the confirmation
// prompt with Ctrl+C.
func runSysconfig(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	fs := flag.NewFlagSet("sysconfig", flag.ContinueOnError)
	fs.SetOutput(stderr)

	o := setSysconfigFlags(fs)
	noUserConfig := setNoUserConfigFlag(fs)

	if err := fs.Parse(args); err != nil {
		return 2
	}

	m, err := manifest.Resolve(o.manifest, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS,
		manifestOverlayOptions(noUserConfigEnabled(*noUserConfig), stderr))
	if err != nil {
		return reportErr(stderr, err)
	}

	info := platform.Detect(r)
	cfg := resolveSysconfig(o.hostname, o.timezone, o.locale, o.shell, o.firewall, o.sshServer, m.Sysconfig)
	cfg.User = realUserName()

	// Fail fast on a mistyped --firewall (or manifest) value before any Plan or
	// privilege work: an unknown mode would otherwise silently skip on Linux or
	// flip the wrong backend on macOS. Exit 2 marks it as a usage error.
	if !sysconfig.ValidFirewallMode(cfg.Firewall) {
		_, _ = fmt.Fprintf(stderr, "%s: invalid --firewall %q: want one of ufw, firewalld, auto, off\n", progName, cfg.Firewall)
		return 2
	}
	if !sysconfig.ValidSSHServerMode(cfg.SSHServer) {
		_, _ = fmt.Fprintf(stderr, "%s: invalid --ssh-server %q: want one of on, off\n", progName, cfg.SSHServer)
		return 2
	}

	engine := sysconfig.Default(r, "")

	// A managed ufw firewall needs a privileged `sudo -n ufw status` read to
	// compute an idempotent plan, so prime sudo before Plan on the apply path.
	// Read-only --dry-run is excluded: its probe fails closed to "unknown"
	// instead of prompting.
	var sudo *privilege.Sudo
	cancel := func() {}
	defer func() { cancel() }()
	if !o.dryRun && (sysconfig.FirewallPlanNeedsSudo(cfg.Firewall, info) || sysconfig.SSHServerPlanNeedsSudo(cfg.SSHServer, info)) {
		sudo = privilege.NewSudo()
		if err := sudo.Prime(r); err != nil {
			return reportErr(stderr, fmt.Errorf("sudo prime: %w", err))
		}
		cancel = sudo.KeepAlive(context.Background(), r)
	}

	plan, err := engine.Plan(cfg, info)
	if err != nil {
		return reportErr(stderr, err)
	}

	if o.dryRun {
		if err := printSysconfigDryRun(stdout, cfg, plan); err != nil {
			return reportErr(stderr, err)
		}
		return 0
	}

	if len(plan.Steps) == 0 {
		if _, err := io.WriteString(stdout, "sysconfig=converged\n"); err != nil {
			return reportErr(stderr, err)
		}
		return 0
	}

	if !o.yes {
		if !stdoutIsTTY(stdout) {
			_, _ = fmt.Fprintf(stderr, "%s: refusing to apply without --yes (non-interactive)\n", progName)
			return 1
		}
		switch confirmApply(stdout, os.Stdin) {
		case confirmAborted:
			return reportAbort(stderr)
		case confirmNo:
			_, _ = fmt.Fprintf(stderr, "%s: sysconfig not confirmed\n", progName)
			return 1
		case confirmYes:
			// proceed to apply
		}
	}

	if sudo == nil {
		sudo = privilege.NewSudo()
		if err := sudo.Prime(r); err != nil {
			return reportErr(stderr, fmt.Errorf("sudo prime: %w", err))
		}
		cancel = sudo.KeepAlive(context.Background(), r)
	}

	res, err := engine.Execute(plan)
	if err != nil {
		return reportErr(stderr, err)
	}
	if err := printSysconfigResults(stdout, res); err != nil {
		return reportErr(stderr, err)
	}
	printSysconfigSummary(stderr, res, sudo.KeepAliveErr())

	return sysconfigExitCode(res)
}

// sysconfigOpts holds the parsed flag values for `dotf sysconfig`.
type sysconfigOpts struct {
	manifest, hostname, timezone, locale, shell, firewall, sshServer string
	dryRun, yes                                                      bool
}

// setSysconfigFlags registers sysconfig's flags on fs and returns the opts
// pointer they parse into. Single source of truth for sysconfig's flag surface,
// shared by runSysconfig and the help renderer (DRY — usage strings live only
// here; --manifest reuses manifestFlagUsage).
func setSysconfigFlags(fs *flag.FlagSet) *sysconfigOpts {
	o := &sysconfigOpts{}
	fs.StringVar(&o.manifest, "manifest", "", manifestFlagUsage)
	fs.StringVar(&o.hostname, "hostname", "", "set the static hostname (per-machine; flag only)")
	fs.StringVar(&o.timezone, "timezone", "", "set the IANA timezone (e.g. America/New_York)")
	fs.StringVar(&o.locale, "locale", "", "set the system locale/LANG (Linux only)")
	fs.StringVar(&o.shell, "shell", "", "set the real user's login shell")
	fs.StringVar(&o.firewall, "firewall", "", "firewall backend to enable: ufw|firewalld|auto|off")
	fs.StringVar(&o.sshServer, "ssh-server", "", "install & enable the OpenSSH server: on|off")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print the plan without executing")
	fs.BoolVar(&o.yes, "yes", false, "apply without an interactive confirmation prompt")
	return o
}

// resolveSysconfig builds the desired sysconfig.Config from CLI flags and the
// optional manifest [sysconfig] defaults, precedence flag > manifest > "".
// Hostname is flag-only — it is per-machine and never sourced from the
// repo-wide manifest.
func resolveSysconfig(flagHostname, flagTimezone, flagLocale, flagShell, flagFirewall, flagSSHServer string, m *manifest.Sysconfig) sysconfig.Config {
	var mTimezone, mLocale, mShell, mFirewall, mSSHServer string
	if m != nil {
		mTimezone, mLocale, mShell, mFirewall, mSSHServer = m.Timezone, m.Locale, m.Shell, m.Firewall, m.SSHServer
	}

	return sysconfig.Config{
		Hostname:  flagHostname,
		Timezone:  firstNonEmpty(flagTimezone, mTimezone),
		Locale:    firstNonEmpty(flagLocale, mLocale),
		Shell:     firstNonEmpty(flagShell, mShell),
		Firewall:  sysconfig.FirewallMode(firstNonEmpty(flagFirewall, mFirewall)),
		SSHServer: sysconfig.SSHServerMode(firstNonEmpty(flagSSHServer, mSSHServer)),
	}
}

// firstNonEmpty returns a when it is non-empty, otherwise b.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// realUserName resolves the invoking non-root account whose login shell chsh
// targets. It falls back to $USER when RealUser errors and the process is not
// root; running as root without a resolvable real user yields "" so the shell
// domain is skipped rather than changing root's shell.
func realUserName() string {
	if u, err := privilege.RealUser(); err == nil {
		return u.Name
	}
	if os.Geteuid() != 0 {
		return os.Getenv("USER")
	}
	return ""
}

// stdoutIsTTY reports whether stdout is a terminal. A non-*os.File writer (a
// test's bytes.Buffer) is definitively non-interactive, so it returns false.
func stdoutIsTTY(stdout io.Writer) bool {
	f, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	return isTTY(f)
}

// confirmDecision is the outcome of the interactive sysconfig apply prompt.
type confirmDecision int

const (
	confirmNo      confirmDecision = iota // declined ("n", empty, EOF, or read error)
	confirmYes                            // explicit "y"/"yes"
	confirmAborted                        // Ctrl+C while the prompt was waiting
)

// confirmApply prompts for the sysconfig apply and reads one line from stdin.
// SIGINT handling is scoped to this read: signal.NotifyContext is installed
// before the prompt and stopped (restoring Go's default SIGINT behavior) the
// moment it returns, so a Ctrl+C during any later execution phase keeps its
// normal terminate semantics. Only os.Interrupt is registered.
func confirmApply(stdout io.Writer, stdin io.Reader) confirmDecision {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return confirmApplyCtx(ctx, stdout, stdin)
}

// confirmApplyCtx is the pure, context-injected core of confirmApply. It prompts
// on stdout, reads a line from stdin in a goroutine, and returns confirmAborted
// as soon as ctx is done (Ctrl+C), confirmYes for an explicit yes, or confirmNo
// for any other answer, EOF, or read error. On abort the read goroutine is left
// blocked on stdin; the process is exiting, so that is harmless.
func confirmApplyCtx(ctx context.Context, stdout io.Writer, stdin io.Reader) confirmDecision {
	_, _ = fmt.Fprint(stdout, "apply system configuration? [y/N] ")

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

// printSysconfigDryRun writes the resolved Config and planned Steps as
// machine-greppable key=value lines to stdout, led by a literal dry-run=true.
// Nothing is executed. The preview is emitted with a single checked write so a
// broken stdout surfaces rather than being silently dropped.
func printSysconfigDryRun(stdout io.Writer, cfg sysconfig.Config, plan sysconfig.Plan) error {
	lines := []string{"dry-run=true"}
	if cfg.Hostname != "" {
		lines = append(lines, "hostname="+cfg.Hostname)
	}
	if cfg.Timezone != "" {
		lines = append(lines, "timezone="+cfg.Timezone)
	}
	if cfg.Locale != "" {
		lines = append(lines, "locale="+cfg.Locale)
	}
	if cfg.Shell != "" {
		lines = append(lines, "shell="+cfg.Shell)
	}
	if cfg.Firewall != "" {
		lines = append(lines, "firewall="+string(cfg.Firewall))
	}
	if cfg.SSHServer != "" {
		lines = append(lines, "ssh-server="+string(cfg.SSHServer))
	}

	for _, step := range plan.Steps {
		lines = append(lines, fmt.Sprintf("step.%s=%s", step.Domain, sysconfigStepValue(step)))
	}
	if len(plan.Skipped) > 0 {
		skipped := append([]string(nil), plan.Skipped...)
		sort.Strings(skipped)
		lines = append(lines, "skipped="+strings.Join(skipped, ","))
	}

	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return err
}

// sysconfigStepValue renders a Step for greppable output: the space-joined
// argv for a command step, or "file:<dest>" for a file step.
func sysconfigStepValue(s sysconfig.Step) string {
	if s.File != nil {
		return "file:" + s.File.Dest
	}
	return strings.Join(s.Argv, " ")
}

// printSysconfigResults writes one result.<domain>=ok|failed line per executed
// step and a trailing skipped= line to stdout. It writes nothing when no step
// ran and no domain was skipped.
func printSysconfigResults(stdout io.Writer, res sysconfig.Result) error {
	lines := make([]string, 0, len(res.Applied)+1)
	for _, sr := range res.Applied {
		status := "ok"
		if sr.Err != nil {
			status = "failed"
		}
		lines = append(lines, fmt.Sprintf("result.%s=%s", sr.Step.Domain, status))
	}
	if len(res.Skipped) > 0 {
		skipped := append([]string(nil), res.Skipped...)
		sort.Strings(skipped)
		lines = append(lines, "skipped="+strings.Join(skipped, ","))
	}

	if len(lines) == 0 {
		return nil
	}
	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return err
}

// printSysconfigSummary writes an aggregate count, per-failure diagnostics, and
// any sudo keep-alive failure to stderr, mirroring printInstallSummary.
func printSysconfigSummary(stderr io.Writer, res sysconfig.Result, keepAliveErr error) {
	var failed, succeeded int
	for _, sr := range res.Applied {
		if sr.Err != nil {
			failed++
		} else {
			succeeded++
		}
	}

	_, _ = fmt.Fprintf(stderr, "%s: sysconfig summary: total=%d succeeded=%d failed=%d\n",
		progName, len(res.Applied), succeeded, failed)

	for _, sr := range res.Applied {
		if sr.Err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %s: %v\n", progName, sr.Step.Domain, sr.Err)
		}
	}
	if keepAliveErr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: sudo keep-alive: %v\n", progName, keepAliveErr)
	}
}

// sysconfigExitCode returns 1 when any executed step failed, else 0.
func sysconfigExitCode(res sysconfig.Result) int {
	for _, sr := range res.Applied {
		if sr.Err != nil {
			return 1
		}
	}
	return 0
}
