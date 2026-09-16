// Firewall planning for the systemd Linux family: ufw (debian/arch/wsl) and
// firewalld (redhat/suse). Split out of linux.go, which exceeded the repo's
// ~500-line soft cap.

package sysconfig

import (
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// planFirewall dispatches to ufw or firewalld based on the resolved firewall
// mode and OS family.
func (c *linuxConfigurer) planFirewall(cfg Config, info platform.Info, p *Plan) {
	switch cfg.Firewall {
	case FirewallUnset, FirewallOff:
		p.Skipped = append(p.Skipped, "firewall")
		return
	case FirewallFirewalld:
		c.planFirewalld(info, p)
		return
	case FirewallUfw:
		c.planUfw(info, p)
		return
	case FirewallAuto:
		if info.OS == platform.OSRedHat || info.OS == platform.OSSUSE {
			c.planFirewalld(info, p)
		} else {
			c.planUfw(info, p)
		}
		return
	default:
		p.Skipped = append(p.Skipped, "firewall")
	}
}

// planUfw installs ufw when absent, allows SSH, and enables the firewall. The
// idempotency probe is a non-interactive `sudo -n ufw status`: reading netfilter
// state needs root, but a read-only Plan must never prompt or hang. When the
// probe cannot run (sudo not primed, e.g. on --dry-run) the status is UNKNOWN
// and the domain is left for a privileged re-plan rather than fabricating an
// inactive firewall — fabricating would emit allow+enable on every run and
// break idempotency. The apply path primes sudo before Plan (see
// FirewallPlanNeedsSudo) so a converged host still yields an empty Plan.
func (c *linuxConfigurer) planUfw(info platform.Info, p *Plan) {
	_, lookErr := c.r.LookPath("ufw")
	installed := lookErr == nil
	if !installed {
		p.Steps = append(p.Steps, Step{
			Domain: "firewall",
			Desc:   "install ufw",
			Argv:   ufwInstallArgv(info),
		})
	}

	status, known := c.ufwStatus(installed)
	if !known {
		// Firewall state is unreadable without privilege on this read-only
		// path. Do not fabricate an inactive firewall.
		p.Skipped = append(p.Skipped, "firewall")
		return
	}

	if !ufwHasSSHRule(status) {
		p.Steps = append(p.Steps, Step{
			Domain: "firewall",
			Desc:   "ufw allow OpenSSH",
			Argv:   []string{"sudo", "ufw", "allow", "OpenSSH"},
		})
	}
	if !ufwActive(status) {
		p.Steps = append(p.Steps, Step{
			Domain: "firewall",
			Desc:   "ufw enable",
			Argv:   []string{"sudo", "ufw", "--force", "enable"},
		})
	}
}

// ufwStatus reads the current ufw status via a non-interactive
// `sudo -n ufw status`, returning the raw output and whether it is known. A
// not-yet-installed ufw is definitionally inactive with no rules, so it returns
// ("", true) without probing (the caller has already planned the install). A
// probe error — sudo not primed on a read-only path, or ufw refusing — yields
// ("", false) so the caller does not fabricate an inactive firewall.
func (c *linuxConfigurer) ufwStatus(installed bool) (status string, known bool) {
	if !installed {
		return "", true
	}
	out, err := c.r.Output("sudo", "-n", "ufw", "status")
	if err != nil {
		return "", false
	}
	return string(out), true
}

// firewalldSSHStatus reports whether firewalld permanently allows the ssh
// service, and whether that is KNOWN. `--permanent --query-service=ssh` is
// polkit-gated on Fedora and exits 1 both when the service is absent and when
// the caller is unauthorized, so the exit status alone is ambiguous: the
// verdict is read from stdout ("yes"/"no") under a non-interactive `sudo -n`,
// and anything else is UNKNOWN. A not-yet-installed firewalld definitionally
// has no rule, so it returns (false, true) without probing — same contract as
// ufwStatus.
func (c *linuxConfigurer) firewalldSSHStatus(installed bool) (allowed, known bool) {
	if !installed {
		return false, true
	}
	out, _ := c.r.Output("sudo", "-n", "firewall-cmd", "--permanent", "--query-service=ssh")
	switch strings.TrimSpace(string(out)) {
	case "yes":
		return true, true
	case "no":
		return false, true
	default:
		return false, false
	}
}

// FirewallPlanNeedsSudo reports whether planning the firewall domain for mode on
// info issues a privileged read, so the command layer can prime sudo before
// Plan on the apply path. BOTH Linux backends do: ufw's `sudo -n ufw status`
// and firewalld's `sudo -n firewall-cmd --permanent --query-service=ssh` (which
// is polkit-gated on Fedora). The macOS Application Firewall reads its state
// unprivileged, and off/unset skip the domain entirely.
func FirewallPlanNeedsSudo(mode FirewallMode, info platform.Info) bool {
	if info.OS == platform.OSMacOS || info.OS == platform.OSWindows {
		return false
	}
	switch mode {
	case FirewallUfw, FirewallFirewalld, FirewallAuto:
		return true
	default:
		return false
	}
}

// planFirewalld installs firewalld when absent, ensures it is running, and
// ensures the ssh service is permanently allowed (permanent + reload). It
// mirrors planUfw's shape: presence is decided by LookPath FIRST, so a host
// without firewall-cmd is not asked to enable a unit that does not exist, and
// the --state read is only interpreted once the binary is known to be present.
func (c *linuxConfigurer) planFirewalld(info platform.Info, p *Plan) {
	_, lookErr := c.r.LookPath("firewall-cmd")
	installed := lookErr == nil
	if !installed {
		p.Steps = append(p.Steps, Step{
			Domain: "firewall",
			Desc:   "install firewalld",
			Argv:   pkgInstallArgv(info, "firewalld"),
		})
	}

	if !installed || !firewalldRunning(c.r) {
		p.Steps = append(p.Steps, Step{
			Domain: "firewall",
			Desc:   "firewalld enable",
			Argv:   []string{"sudo", "systemctl", "enable", "--now", "firewalld"},
		})
	}

	allowed, known := c.firewalldSSHStatus(installed)
	if !known {
		// Rule state is unreadable without privilege on this read-only path.
		// Do not fabricate a missing rule — that would re-add + reload on
		// every run and break idempotency (same contract as planUfw).
		p.Skipped = append(p.Skipped, "firewall")
		return
	}
	if !allowed {
		p.Steps = append(p.Steps, firewalldAllowSSHSteps("firewall")...)
	}
}

// firewalldRunning reports whether `firewall-cmd --state` says running. It is
// only meaningful once firewall-cmd is known to be on PATH; a probe error on a
// present binary means not-running.
func firewalldRunning(r exec.Runner) bool {
	out, err := r.Output("firewall-cmd", "--state")
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "running")
}

// firewalldAllowSSHSteps returns the permanent ssh-allow + reload pair, tagged
// with domain so both the firewall and ssh-server planners emit identical argv.
func firewalldAllowSSHSteps(domain string) []Step {
	return []Step{
		{
			Domain: domain,
			Desc:   "firewalld allow ssh",
			Argv:   []string{"sudo", "firewall-cmd", "--permanent", "--add-service=ssh"},
		},
		{
			Domain: domain,
			Desc:   "firewalld reload",
			Argv:   []string{"sudo", "firewall-cmd", "--reload"},
		},
	}
}

// ufwInstallArgv builds the ufw install argv for the distro family. RedHat and
// SUSE reach this only under an explicit `--firewall ufw`; under `auto` they
// route to firewalld (see planFirewall).
func ufwInstallArgv(info platform.Info) []string {
	return pkgInstallArgv(info, "ufw")
}

// ufwHasSSHRule reports whether ufw status output already allows SSH.
func ufwHasSSHRule(status string) bool {
	return strings.Contains(status, "OpenSSH") || strings.Contains(status, "22/tcp")
}

// ufwActive reports whether ufw status output shows an active firewall.
func ufwActive(status string) bool {
	return strings.Contains(status, "Status: active")
}
