// Package sysconfig owns OS-level system configuration for the dotf CLI:
// hostname (plus /etc/hosts), timezone, locale, default login shell,
// firewall, and ssh-server, per OS/distro family.
//
// The package follows a plan/apply split. A Configurer reads the host's
// current state and computes an ordered, read-only Plan of the Steps needed
// to converge to a desired Config; the Engine dispatches to the Configurer
// registered for the detected platform.OS and owns the OS-agnostic Execute
// that performs the mutations. Idempotency is structural: a converged host
// produces an empty Plan, so a second Execute runs nothing.
//
// Every privileged mutation is built as a sudo-prefixed argv and run through
// the injected internal/exec.Runner; system files (/etc/hosts, /etc/shells,
// /etc/locale.gen) are read through the FileSystem seam and installed via a
// staged "sudo cp" so nothing bypasses the mockable seams. It mirrors the
// firewall/ssh, chsh, and /etc/shells handling that bootstrap.sh performs,
// and adds hostname/timezone/localectl handling that has no bash precedent.
//
// Windows is a documented, deferred stub returning ErrNotImplemented.
package sysconfig

import (
	"errors"
	"fmt"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// Config is the resolved desired system state. An empty string field means
// the domain is left UNMANAGED (skipped) — it is never defaulted
// destructively. Firewall and SSHServer use their Unset ("") value to skip.
type Config struct {
	// Hostname is the desired static hostname. "" skips the domain. It is
	// per-machine and supplied flag-only, never from the manifest.
	Hostname string

	// Timezone is the desired IANA "Area/City" timezone (e.g.
	// "America/New_York"). "" skips the domain.
	Timezone string

	// Locale is the desired LANG value (e.g. "en_US.UTF-8"). "" skips the
	// domain. Linux only — on macOS the locale domain is a no-op.
	Locale string

	// Shell is the desired login shell name or path (e.g. "zsh" or
	// "/usr/bin/zsh"). "" skips the domain.
	Shell string

	// Firewall selects the firewall backend to enable. FirewallUnset skips.
	Firewall FirewallMode

	// SSHServer selects whether to enable the SSH server. SSHServerUnset skips.
	SSHServer SSHServerMode

	// User is the real (non-root) account whose login shell chsh targets. The
	// command layer sets this from privilege.RealUser().Name so root's shell
	// is never changed by accident.
	User string
}

// FirewallMode selects the firewall backend. "auto" resolves per OS family.
type FirewallMode string

// Canonical FirewallMode values.
const (
	// FirewallUnset leaves the firewall domain unmanaged (skip).
	FirewallUnset FirewallMode = ""

	// FirewallOff is an explicit skip; it does NOT disable an active firewall
	// (sysconfig is additive-only and never tears down a firewall).
	FirewallOff FirewallMode = "off"

	// FirewallAuto resolves per OS family: ufw on debian/arch, firewalld on
	// redhat/suse, socketfilterfw on macOS.
	FirewallAuto FirewallMode = "auto"

	// FirewallUfw selects the ufw backend explicitly.
	FirewallUfw FirewallMode = "ufw"

	// FirewallFirewalld selects the firewalld backend explicitly.
	FirewallFirewalld FirewallMode = "firewalld"
)

// validFirewallModes is the set of accepted FirewallMode values. FirewallUnset
// ("") is included: an empty mode leaves the firewall domain unmanaged.
var validFirewallModes = map[FirewallMode]bool{
	FirewallUnset:     true,
	FirewallOff:       true,
	FirewallAuto:      true,
	FirewallUfw:       true,
	FirewallFirewalld: true,
}

// ValidFirewallMode reports whether mode is one of the accepted firewall
// backends ("", off, auto, ufw, firewalld). The command layer uses it to fail
// fast on a mistyped --firewall value before any Plan or privilege work.
func ValidFirewallMode(mode FirewallMode) bool {
	return validFirewallModes[mode]
}

// SSHServerMode selects whether to enable the SSH server. SSHServerUnset skips.
type SSHServerMode string

// Canonical SSHServerMode values.
const (
	// SSHServerUnset leaves the ssh-server domain unmanaged (skip).
	SSHServerUnset SSHServerMode = ""

	// SSHServerOff is an explicit skip; it does NOT disable or stop an active
	// SSH server (sysconfig is additive-only and never tears down sshd).
	SSHServerOff SSHServerMode = "off"

	// SSHServerOn enables the SSH server.
	SSHServerOn SSHServerMode = "on"
)

// validSSHServerModes is the set of accepted SSHServerMode values. SSHServerUnset
// ("") is included: an empty mode leaves the ssh-server domain unmanaged.
var validSSHServerModes = map[SSHServerMode]bool{
	SSHServerUnset: true,
	SSHServerOff:   true,
	SSHServerOn:    true,
}

// ValidSSHServerMode reports whether mode is one of the accepted ssh-server
// modes ("", off, on). The command layer uses it to fail fast on a mistyped
// --ssh-server value before any Plan or privilege work.
func ValidSSHServerMode(mode SSHServerMode) bool {
	return validSSHServerModes[mode]
}

// Step is one planned convergence action. Exactly one of Argv or File is set.
type Step struct {
	// Domain is the sysconfig domain this Step belongs to: one of "hostname",
	// "timezone", "locale", "shell", "firewall", or "ssh-server".
	Domain string

	// Desc is a greppable one-line summary of the Step (e.g. "hostname=web01").
	Desc string

	// Argv is the exact command argv for a command step, sudo-prefixed where
	// the operation is privileged. Nil for a file step.
	Argv []string

	// File describes a system-file install for a file step. Nil for a command
	// step.
	File *FileInstall
}

// FileInstall installs Content at Dest via a privileged copy. Execute writes
// Content to a 0600 temp file (FileSystem.WriteTemp), backs up Dest when
// Backup is set ("sudo cp Dest Dest.dotf.bak"), then "sudo cp <tmp> Dest".
// The write is deferred to Execute so Plan stays read-only and --dry-run
// touches nothing.
type FileInstall struct {
	// Dest is the absolute path of the system file to install.
	Dest string

	// Content is the fully rendered file body to install at Dest.
	Content []byte

	// Backup, when true, copies Dest to Dest.dotf.bak before installing.
	Backup bool
}

// Plan is the ordered set of Steps needed to converge to a Config. An empty
// Steps slice means the host is already converged (an idempotent no-op).
// Skipped lists the domains that produced no Step (unmanaged or converged).
type Plan struct {
	// Steps is the ordered convergence actions to run.
	Steps []Step

	// Skipped lists domains that produced no Step.
	Skipped []string
}

// StepResult records the outcome of executing a single Step. Err is non-nil
// when that Step failed; failures are collected non-fatally.
type StepResult struct {
	// Step is the Step that was executed.
	Step Step

	// Err is the execution error, or nil on success.
	Err error
}

// Result records the outcome of Executing a Plan. Applied holds one
// StepResult per executed Step (failures collected non-fatally); Skipped
// carries through the Plan's skipped domains.
type Result struct {
	// Applied holds one StepResult per executed Step, in order.
	Applied []StepResult

	// Skipped carries through the Plan's skipped domains.
	Skipped []string
}

// Configurer computes the Plan for one OS family. It holds the injected
// exec.Runner and FileSystem and never mutates during Plan.
type Configurer interface {
	// Plan computes the ordered Steps needed to converge the host described
	// by info to cfg. It is read-only. info is passed through from Engine.Plan
	// so a single shared linux Configurer can still distinguish distro
	// families (e.g. pacman vs apt for the ufw install step).
	Plan(cfg Config, info platform.Info) (Plan, error)
}

// ErrNotImplemented is returned by the deferred Windows Configurer: sysconfig
// is not implemented on Windows yet.
var ErrNotImplemented = errors.New("sysconfig: not implemented on this OS (deferred)")

// UnsupportedOSError is returned when Engine dispatch finds no Configurer
// registered for a platform.OS value (e.g. platform.OSUnknown).
type UnsupportedOSError struct {
	// OS is the platform.OS value that has no registered Configurer.
	OS platform.OS
}

// Error implements the error interface.
func (e *UnsupportedOSError) Error() string {
	return fmt.Sprintf("sysconfig: unsupported OS %q", e.OS)
}

// Engine dispatches Plan to the Configurer registered for a platform.OS and
// owns Execute (the OS-agnostic step runner).
type Engine struct {
	configurers map[platform.OS]Configurer
	r           exec.Runner
	fs          FileSystem
}

// NewEngine constructs an Engine backed by the given dispatch map, exec.Runner,
// and FileSystem.
func NewEngine(cfgs map[platform.OS]Configurer, r exec.Runner, fs FileSystem) *Engine {
	return &Engine{configurers: cfgs, r: r, fs: fs}
}

// Plan dispatches to the Configurer registered for info.OS and returns its
// read-only Plan. It returns *UnsupportedOSError when no Configurer is
// registered for info.OS. The windows Configurer IS registered and returns
// ErrNotImplemented, so an unregistered OS means a truly unknown platform.
func (e *Engine) Plan(cfg Config, info platform.Info) (Plan, error) {
	c, ok := e.configurers[info.OS]
	if !ok {
		return Plan{}, &UnsupportedOSError{OS: info.OS}
	}
	return c.Plan(cfg, info)
}

// Execute runs each Step in p, collecting per-step failures non-fatally into
// the returned Result (it never aborts mid-plan). Command steps run through
// the exec.Runner; file steps are staged via FileSystem.WriteTemp and
// installed with a privileged copy. Inspect Result.Applied for per-step
// outcomes.
func (e *Engine) Execute(p Plan) Result {
	res := Result{Skipped: p.Skipped}

	for _, step := range p.Steps {
		sr := StepResult{Step: step}
		switch {
		case step.File != nil:
			sr.Err = e.installFile(step.File)
		case len(step.Argv) > 0:
			sr.Err = e.r.Run(step.Argv[0], step.Argv[1:]...)
		default:
			sr.Err = fmt.Errorf("sysconfig: step %q has neither argv nor file", step.Domain)
		}
		res.Applied = append(res.Applied, sr)
	}

	return res
}

// installFile stages fi.Content to a 0600 temp file, optionally backs up the
// destination, then installs the staged file with a privileged copy.
func (e *Engine) installFile(fi *FileInstall) error {
	if fi.Backup && e.fs.Exists(fi.Dest) {
		if err := e.r.Run("sudo", "cp", fi.Dest, fi.Dest+".dotf.bak"); err != nil {
			return fmt.Errorf("sysconfig: back up %q: %w", fi.Dest, err)
		}
	}

	tmp, err := e.fs.WriteTemp("dotf-sysconfig", fi.Content)
	if err != nil {
		return fmt.Errorf("sysconfig: stage %q: %w", fi.Dest, err)
	}

	if err := e.r.Run("sudo", "cp", tmp, fi.Dest); err != nil {
		return fmt.Errorf("sysconfig: install %q: %w", fi.Dest, err)
	}
	return nil
}
