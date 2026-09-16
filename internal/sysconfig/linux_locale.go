// Locale planning for the systemd Linux family: locale-gen / localectl. Split
// out of linux.go, which exceeded the repo's ~500-line soft cap.

package sysconfig

import (
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// planLocale generates the requested locale (when it is not already available
// and is not a built-in) and sets it via localectl. The generate strategy is
// dispatched by distro family FIRST — RedHat installs the matching
// `glibc-langpack-<lang>` RPM, since Fedora ships no locale-gen at all — and
// only then by tool presence. The generate Step(s) always precede the set Step.
// When no strategy applies it falls back through resolveLocale (ultimately C).
func (c *linuxConfigurer) planLocale(cfg Config, info platform.Info, p *Plan) {
	if cfg.Locale == "" {
		p.Skipped = append(p.Skipped, "locale")
		return
	}

	avail, _ := c.r.Output("locale", "-a")

	target := cfg.Locale
	var genSteps []Step

	if !localeAvailable(avail, cfg.Locale) && !isBuiltinLocale(cfg.Locale) {
		genSteps = c.localeGenSteps(cfg.Locale, info)
	}
	if len(genSteps) == 0 && !localeAvailable(avail, cfg.Locale) {
		target = resolveLocale(cfg.Locale, avail)
	}

	current := currentLocaleLinux(c.r)
	needSet := normalizeLocale(current) != normalizeLocale(target)
	if len(genSteps) > 0 {
		// A freshly generated locale must be set even if the reported current
		// value happens to match the target string.
		needSet = true
	}

	if len(genSteps) == 0 && !needSet {
		p.Skipped = append(p.Skipped, "locale")
		return
	}

	p.Steps = append(p.Steps, genSteps...)
	p.Steps = append(p.Steps, Step{
		Domain: "locale",
		Desc:   "locale LANG=" + target,
		Argv:   []string{"sudo", "localectl", "set-locale", "LANG=" + target},
	})
}

// localeGenSteps builds the generate Steps for target, choosing the strategy by
// distro family FIRST and tool presence second:
//   - RedHat: no locale-gen exists; glibc ships pre-generated locales in
//     `glibc-langpack-<lang>` RPMs, so install that.
//   - Arch: uncomment the entry in /etc/locale.gen (a file step) then run
//     bare `locale-gen`.
//   - everything else: `locale-gen <target>` directly.
//
// Returns nil when no strategy applies (no locale-gen on PATH, or no derivable
// langpack); the caller then falls back through resolveLocale.
func (c *linuxConfigurer) localeGenSteps(target string, info platform.Info) []Step {
	if info.OS == platform.OSRedHat {
		pkg := langpackFor(target)
		if pkg == "" {
			return nil
		}
		return []Step{{
			Domain: "locale",
			Desc:   "install " + pkg,
			Argv:   pkgInstallArgv(info, pkg),
		}}
	}

	if _, err := c.r.LookPath("locale-gen"); err != nil {
		return nil
	}

	if info.OS == platform.OSArch {
		var steps []Step
		content, _ := c.fs.ReadFile("/etc/locale.gen")
		if newContent, changed := uncommentLocaleGen(content, target); changed {
			steps = append(steps, Step{
				Domain: "locale",
				Desc:   "uncomment " + target + " in /etc/locale.gen",
				File:   &FileInstall{Dest: "/etc/locale.gen", Content: newContent, Backup: true},
			})
		}
		steps = append(steps, Step{
			Domain: "locale",
			Desc:   "locale-gen",
			Argv:   []string{"sudo", "locale-gen"},
		})
		return steps
	}

	return []Step{{
		Domain: "locale",
		Desc:   "locale-gen " + target,
		Argv:   []string{"sudo", "locale-gen", target},
	}}
}

// currentLocaleLinux reads the current system LANG via localectl status,
// returning "" when it cannot be read.
func currentLocaleLinux(r exec.Runner) string {
	out, err := r.Output("localectl", "status")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if idx := strings.Index(line, "LANG="); idx >= 0 {
			fields := strings.Fields(line[idx+len("LANG="):])
			if len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return ""
}
