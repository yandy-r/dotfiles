package sysconfig

import (
	"fmt"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// linuxConfigurer plans system config for the systemd Linux family
// (debian/redhat/arch/suse/linux/wsl): hostnamectl, timedatectl, localectl +
// locale-gen, chsh guarded by /etc/shells, and ufw/firewalld.
type linuxConfigurer struct {
	r  exec.Runner
	fs FileSystem
}

// newLinuxConfigurer constructs a linuxConfigurer backed by r and fs.
func newLinuxConfigurer(r exec.Runner, fs FileSystem) *linuxConfigurer {
	return &linuxConfigurer{r: r, fs: fs}
}

// Plan computes the ordered convergence Steps for the six domains in the
// canonical order hostname → timezone → locale → shell → firewall → ssh-server.
func (c *linuxConfigurer) Plan(cfg Config, info platform.Info) (Plan, error) {
	var p Plan

	c.planHostname(cfg, &p)
	if err := c.planTimezone(cfg, &p); err != nil {
		return Plan{}, err
	}
	c.planLocale(cfg, info, &p)
	if err := c.planShell(cfg, info, &p); err != nil {
		return Plan{}, err
	}
	c.planFirewall(cfg, info, &p)
	c.planSSHServer(cfg, info, &p)

	return p, nil
}

// planHostname sets the static hostname via hostnamectl and ensures the
// Debian-convention 127.0.1.1 line in /etc/hosts (hostnamectl does not touch
// /etc/hosts).
func (c *linuxConfigurer) planHostname(cfg Config, p *Plan) {
	if cfg.Hostname == "" {
		p.Skipped = append(p.Skipped, "hostname")
		return
	}

	changed := false

	current := ""
	if out, err := c.r.Output("hostnamectl", "--static"); err == nil {
		current = strings.TrimSpace(string(out))
	}
	if current != cfg.Hostname {
		p.Steps = append(p.Steps, Step{
			Domain: "hostname",
			Desc:   "hostname=" + cfg.Hostname,
			Argv:   []string{"sudo", "hostnamectl", "set-hostname", cfg.Hostname},
		})
		changed = true
	}

	content, _ := c.fs.ReadFile("/etc/hosts")
	if hostsNeedsUpdate(content, cfg.Hostname) {
		p.Steps = append(p.Steps, Step{
			Domain: "hostname",
			Desc:   "/etc/hosts 127.0.1.1 " + cfg.Hostname,
			File:   &FileInstall{Dest: "/etc/hosts", Content: renderHosts(content, cfg.Hostname), Backup: true},
		})
		changed = true
	}

	if !changed {
		p.Skipped = append(p.Skipped, "hostname")
	}
}

// planTimezone validates the zoneinfo file exists then sets the timezone via
// timedatectl when it differs from the current one.
func (c *linuxConfigurer) planTimezone(cfg Config, p *Plan) error {
	if cfg.Timezone == "" {
		p.Skipped = append(p.Skipped, "timezone")
		return nil
	}

	if !c.fs.Exists("/usr/share/zoneinfo/" + cfg.Timezone) {
		return fmt.Errorf("sysconfig: timezone: unknown timezone %q (no /usr/share/zoneinfo/%s)", cfg.Timezone, cfg.Timezone)
	}

	current := ""
	if out, err := c.r.Output("timedatectl", "show", "-p", "Timezone", "--value"); err == nil {
		current = strings.TrimSpace(string(out))
	}
	if current == cfg.Timezone {
		p.Skipped = append(p.Skipped, "timezone")
		return nil
	}

	p.Steps = append(p.Steps, Step{
		Domain: "timezone",
		Desc:   "timezone=" + cfg.Timezone,
		Argv:   []string{"sudo", "timedatectl", "set-timezone", cfg.Timezone},
	})
	return nil
}

// planShell resolves the shell path, ensures it is present in /etc/shells (a
// file step that MUST precede chsh), then changes the real user's login shell
// via chsh — or usermod where chsh is absent — run as root.
func (c *linuxConfigurer) planShell(cfg Config, info platform.Info, p *Plan) error {
	if cfg.Shell == "" {
		p.Skipped = append(p.Skipped, "shell")
		return nil
	}

	if cfg.User == "" {
		// No resolvable real user (e.g. root with no SUDO_USER). Skip rather
		// than emit `chsh -s <path> ""`, which errors and exits 1 — the shell
		// domain is left unmanaged, not fatally attempted.
		p.Skipped = append(p.Skipped, "shell")
		return nil
	}

	path, err := resolveShellPath(c.r, cfg.Shell, info)
	if err != nil {
		return fmt.Errorf("sysconfig: shell: %w", err)
	}
	if path == "" {
		// No shell found — warn (via Skipped) and skip rather than fail.
		p.Skipped = append(p.Skipped, "shell")
		return nil
	}

	changed := false

	content, _ := c.fs.ReadFile("/etc/shells")
	if !etcShellsHasPath(content, path) {
		p.Steps = append(p.Steps, Step{
			Domain: "shell",
			Desc:   "/etc/shells += " + path,
			File:   &FileInstall{Dest: "/etc/shells", Content: renderEtcShells(content, path), Backup: true},
		})
		changed = true
	}

	if currentShellLinux(c.r, cfg.User) != path {
		p.Steps = append(p.Steps, loginShellStep(c.r, cfg.User, path))
		changed = true
	}

	if !changed {
		p.Skipped = append(p.Skipped, "shell")
	}
	return nil
}

// loginShellStep builds the login-shell change Step for user. chsh is
// preferred, but it is not universally present: on some Fedora releases it
// lives in the separate util-linux-user subpackage, and on others it is folded
// back into base util-linux. When chsh is absent, fall back to `usermod -s`
// from shadow-utils, which is present on Fedora, Debian, Arch and SUSE base
// installs. (macOS keeps chsh unconditionally — see darwin.go — because
// usermod does not exist there.)
func loginShellStep(r exec.Runner, user, path string) Step {
	if _, err := r.LookPath("chsh"); err == nil {
		return Step{
			Domain: "shell",
			Desc:   "chsh " + user + " " + path,
			Argv:   []string{"sudo", "chsh", "-s", path, user},
		}
	}
	return Step{
		Domain: "shell",
		Desc:   "usermod -s " + path + " " + user,
		Argv:   []string{"sudo", "usermod", "-s", path, user},
	}
}

// planSSHServer installs OpenSSH when absent, generates host keys when missing,
// enables the distro-specific ssh unit when inactive, and opens the firewall for
// SSH (firewalld on RedHat/SUSE, ufw elsewhere).
func (c *linuxConfigurer) planSSHServer(cfg Config, info platform.Info, p *Plan) {
	switch cfg.SSHServer {
	case SSHServerUnset, SSHServerOff:
		p.Skipped = append(p.Skipped, "ssh-server")
		return
	case SSHServerOn:
	default:
		p.Skipped = append(p.Skipped, "ssh-server")
		return
	}

	changed := false

	if !c.fs.Exists("/usr/sbin/sshd") {
		p.Steps = append(p.Steps, Step{
			Domain: "ssh-server",
			Desc:   "install openssh",
			Argv:   sshInstallArgv(info),
		})
		changed = true
	}

	if !c.fs.Exists("/etc/ssh/ssh_host_ed25519_key") {
		p.Steps = append(p.Steps, Step{
			Domain: "ssh-server",
			Desc:   "generate host keys",
			Argv:   []string{"sudo", "ssh-keygen", "-A"},
		})
		changed = true
	}

	unit := sshServiceName(info)
	active := ""
	if out, err := c.r.Output("systemctl", "is-active", unit); err == nil {
		active = strings.TrimSpace(string(out))
	}
	if active != "active" {
		p.Steps = append(p.Steps, Step{
			Domain: "ssh-server",
			Desc:   "enable " + unit,
			Argv:   []string{"sudo", "systemctl", "enable", "--now", unit},
		})
		changed = true
	}

	if c.planSSHServerFirewallOpen(info, p) {
		changed = true
	}

	if !changed {
		p.Skipped = append(p.Skipped, "ssh-server")
	}
}

// planSSHServerFirewallOpen appends firewalld or ufw SSH-allow steps when the
// resolved backend lacks an ssh rule. Returns true when any step was appended.
func (c *linuxConfigurer) planSSHServerFirewallOpen(info platform.Info, p *Plan) bool {
	if info.OS == platform.OSRedHat || info.OS == platform.OSSUSE {
		_, lookErr := c.r.LookPath("firewall-cmd")
		allowed, known := c.firewalldSSHStatus(lookErr == nil)
		if !known || allowed {
			return false
		}
		p.Steps = append(p.Steps, firewalldAllowSSHSteps("ssh-server")...)
		return true
	}

	_, lookErr := c.r.LookPath("ufw")
	status, known := c.ufwStatus(lookErr == nil)
	if !known || ufwHasSSHRule(status) {
		return false
	}

	p.Steps = append(p.Steps, Step{
		Domain: "ssh-server",
		Desc:   "ufw allow OpenSSH",
		Argv:   []string{"sudo", "ufw", "allow", "OpenSSH"},
	})
	return true
}

// sshServiceName returns the systemd unit name for the OpenSSH server on info.
func sshServiceName(info platform.Info) string {
	switch info.OS {
	case platform.OSDebian, platform.OSWSL:
		return "ssh"
	default:
		return "sshd"
	}
}

// pkgInstallArgv builds the non-interactive install argv for name using the
// native package manager of info's distro family. It is the single source of
// truth for every sysconfig-owned package install (ufw, firewalld, openssh),
// so a new family is added in exactly one place.
func pkgInstallArgv(info platform.Info, name string) []string {
	switch info.OS {
	case platform.OSArch:
		return []string{"sudo", "pacman", "-S", "--noconfirm", "--needed", name}
	case platform.OSRedHat:
		return []string{"sudo", "dnf", "install", "-y", name}
	case platform.OSSUSE:
		return []string{"sudo", "zypper", "--non-interactive", "install", name}
	default:
		return []string{"sudo", "apt-get", "install", "-y", name}
	}
}

// sshInstallArgv builds the OpenSSH server install argv for the distro family.
// Arch's package is `openssh`; every other family ships it as `openssh-server`.
func sshInstallArgv(info platform.Info) []string {
	name := "openssh-server"
	if info.OS == platform.OSArch {
		name = "openssh"
	}
	return pkgInstallArgv(info, name)
}

// SSHServerPlanNeedsSudo reports whether planning the ssh-server domain for mode
// on info issues a privileged read, so the command layer can prime sudo before
// Plan on the apply path. Every Linux family does: the ufw SSH-allow probe
// (`sudo -n ufw status`) and the firewalld one
// (`sudo -n firewall-cmd --permanent --query-service=ssh`) both need root.
func SSHServerPlanNeedsSudo(mode SSHServerMode, info platform.Info) bool {
	if info.OS == platform.OSMacOS || info.OS == platform.OSWindows {
		return false
	}
	return mode == SSHServerOn
}

// currentShellLinux reads the login shell (passwd field 7) for user via
// getent, returning "" when it cannot be read.
func currentShellLinux(r exec.Runner, user string) string {
	out, err := r.Output("getent", "passwd", user)
	if err != nil {
		return ""
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	fields := strings.Split(line, ":")
	if len(fields) >= 7 {
		return fields[6]
	}
	return ""
}

var _ Configurer = (*linuxConfigurer)(nil)
