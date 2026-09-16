package sysconfig

import (
	"fmt"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// socketfilterfwPath is the macOS Application Firewall control binary.
const socketfilterfwPath = "/usr/libexec/ApplicationFirewall/socketfilterfw"

// darwinConfigurer plans system config for macOS: scutil for hostname,
// systemsetup for timezone, dscl for the login shell, and the Application
// Firewall (socketfilterfw). Locale is a deliberate no-op (owned by the shell
// layer).
type darwinConfigurer struct {
	r  exec.Runner
	fs FileSystem
}

// newDarwinConfigurer constructs a darwinConfigurer backed by r and fs.
func newDarwinConfigurer(r exec.Runner, fs FileSystem) *darwinConfigurer {
	return &darwinConfigurer{r: r, fs: fs}
}

// Plan computes the ordered convergence Steps for macOS. Locale is always
// skipped (macOS locale is per-user and owned by the shell layer).
func (c *darwinConfigurer) Plan(cfg Config, info platform.Info) (Plan, error) {
	var p Plan

	c.planHostname(cfg, &p)
	c.planTimezone(cfg, &p)
	p.Skipped = append(p.Skipped, "locale")
	p.Skipped = append(p.Skipped, "ssh-server")
	if err := c.planShell(cfg, info, &p); err != nil {
		return Plan{}, err
	}
	c.planFirewall(cfg, &p)

	return p, nil
}

// scutilHostnameKeys are the three scutil hostname facets set on macOS.
var scutilHostnameKeys = []string{"ComputerName", "LocalHostName", "HostName"}

// planHostname sets ComputerName/LocalHostName/HostName via scutil, emitting a
// set Step for each facet that differs. "scutil --get HostName" errors when
// never set — that error is treated as "unset" so the set Step is still
// emitted.
func (c *darwinConfigurer) planHostname(cfg Config, p *Plan) {
	if cfg.Hostname == "" {
		p.Skipped = append(p.Skipped, "hostname")
		return
	}

	changed := false
	for _, key := range scutilHostnameKeys {
		current := ""
		if out, err := c.r.Output("scutil", "--get", key); err == nil {
			current = strings.TrimSpace(string(out))
		}
		if current != cfg.Hostname {
			p.Steps = append(p.Steps, Step{
				Domain: "hostname",
				Desc:   "scutil " + key + "=" + cfg.Hostname,
				Argv:   []string{"sudo", "scutil", "--set", key, cfg.Hostname},
			})
			changed = true
		}
	}

	if !changed {
		p.Skipped = append(p.Skipped, "hostname")
	}
}

// planTimezone sets the timezone via systemsetup, reading the current zone via
// systemsetup and falling back to readlink /etc/localtime when systemsetup
// errors (it can require root even for -gettimezone).
func (c *darwinConfigurer) planTimezone(cfg Config, p *Plan) {
	if cfg.Timezone == "" {
		p.Skipped = append(p.Skipped, "timezone")
		return
	}

	current := c.currentTimezone()
	if current == cfg.Timezone {
		p.Skipped = append(p.Skipped, "timezone")
		return
	}

	p.Steps = append(p.Steps, Step{
		Domain: "timezone",
		Desc:   "timezone=" + cfg.Timezone,
		Argv:   []string{"sudo", "systemsetup", "-settimezone", cfg.Timezone},
	})
}

// currentTimezone reads the current macOS timezone, preferring systemsetup and
// falling back to the /etc/localtime symlink target.
func (c *darwinConfigurer) currentTimezone() string {
	if out, err := c.r.Output("systemsetup", "-gettimezone"); err == nil {
		return parseSystemsetupTimezone(string(out))
	}
	if out, err := c.r.Output("readlink", "/etc/localtime"); err == nil {
		link := strings.TrimSpace(string(out))
		link = strings.TrimPrefix(link, "/var/db/timezone/zoneinfo/")
		link = strings.TrimPrefix(link, "/usr/share/zoneinfo/")
		return link
	}
	return ""
}

// parseSystemsetupTimezone extracts the zone from "Time Zone: America/New_York".
func parseSystemsetupTimezone(out string) string {
	if idx := strings.Index(out, ":"); idx >= 0 {
		return strings.TrimSpace(out[idx+1:])
	}
	return strings.TrimSpace(out)
}

// planShell resolves the shell path, ensures it is present in /etc/shells (a
// file step that MUST precede chsh), then changes the real user's login shell
// via chsh run as root. macOS reads the current shell via dscl (getent is
// absent).
func (c *darwinConfigurer) planShell(cfg Config, info platform.Info, p *Plan) error {
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

	if currentShellDarwin(c.r, cfg.User) != path {
		p.Steps = append(p.Steps, Step{
			Domain: "shell",
			Desc:   "chsh " + cfg.User + " " + path,
			Argv:   []string{"sudo", "chsh", "-s", path, cfg.User},
		})
		changed = true
	}

	if !changed {
		p.Skipped = append(p.Skipped, "shell")
	}
	return nil
}

// planFirewall enables the macOS Application Firewall (socketfilterfw). Any
// non-skip firewall mode resolves to socketfilterfw on macOS.
func (c *darwinConfigurer) planFirewall(cfg Config, p *Plan) {
	if cfg.Firewall == FirewallUnset || cfg.Firewall == FirewallOff {
		p.Skipped = append(p.Skipped, "firewall")
		return
	}

	out, _ := c.r.Output(socketfilterfwPath, "--getglobalstate")
	if strings.Contains(string(out), "State = 1") {
		p.Skipped = append(p.Skipped, "firewall")
		return
	}

	p.Steps = append(p.Steps, Step{
		Domain: "firewall",
		Desc:   "socketfilterfw on",
		Argv:   []string{"sudo", socketfilterfwPath, "--setglobalstate", "on"},
	})
}

// currentShellDarwin reads the login shell for user via dscl, returning ""
// when it cannot be read.
func currentShellDarwin(r exec.Runner, user string) string {
	out, err := r.Output("dscl", ".", "-read", "/Users/"+user, "UserShell")
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if idx := strings.Index(s, ":"); idx >= 0 {
		return strings.TrimSpace(s[idx+1:])
	}
	return s
}

var _ Configurer = (*darwinConfigurer)(nil)
