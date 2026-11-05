package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/yandy-r/dotfiles/internal/rc"
)

// This file holds `dotf rc enable` and `dotf rc disable`: the systemd user
// unit that runs `dotf rc up` at login.

// rcGOOS and rcExecutable are seams so tests can exercise the Linux path and a
// stable executable path on any host.
var (
	rcGOOS       = runtime.GOOS
	rcExecutable = os.Executable
)

// rcServiceSupported reports whether a systemd user unit can be managed here.
func rcServiceSupported(env rcEnv) bool {
	if rcGOOS != "linux" {
		return false
	}
	_, err := env.r.LookPath("systemctl")
	return err == nil
}

// rcUnitPathEnv builds the unit's PATH: the directories holding dotf, herdr
// and claude (so the oneshot finds them under systemd's minimal environment),
// then the standard system directories, de-duplicated in order.
func rcUnitPathEnv(env rcEnv, dotfBin string) string {
	dirs := []string{filepath.Dir(dotfBin)}
	for _, name := range []string{"herdr", "claude"} {
		if p, err := env.r.LookPath(name); err == nil {
			dirs = append(dirs, filepath.Dir(p))
		}
	}
	dirs = append(dirs, filepath.Join(env.home, ".local", "bin"), "/usr/local/bin", "/usr/bin", "/bin")
	var out []string
	for _, d := range dirs {
		if !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return strings.Join(out, ":")
}

// rcEnable writes the unit, reloads systemd, and enables it (with --now, also
// starts it, which runs `dotf rc up` immediately).
func rcEnable(env rcEnv) int {
	if !rcServiceSupported(env) {
		return reportErr(env.stderr, fmt.Errorf("rc enable needs Linux with systemd (systemctl --user); run `dotf rc up` manually instead"))
	}
	exe, err := rcExecutable()
	if err != nil {
		return reportErr(env.stderr, fmt.Errorf("resolve dotf executable: %w", err))
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil && !strings.Contains(resolved, string(os.PathSeparator)+"go-build") {
		exe = resolved
	}
	unitPath := rc.UnitPath(env.home)
	state, err := rc.ReadUnitState(unitPath)
	if err != nil {
		return reportErr(env.stderr, err)
	}
	if state == rc.UnitForeign && !env.o.force {
		return reportErr(env.stderr, fmt.Errorf("%s exists and was not written by dotf; pass --force to overwrite it", unitPath))
	}
	// Always pin the config path: the systemd user manager need not see the
	// shell's XDG_CONFIG_HOME, so the default could resolve differently there.
	text := rc.RenderUnit(exe, env.cfgPath, rcUnitPathEnv(env, exe))
	enableArgs := []string{"enable"}
	if env.o.now {
		enableArgs = append(enableArgs, "--now")
	}
	enableArgs = append(enableArgs, rc.UnitName)
	if env.o.dryRun {
		_, _ = fmt.Fprintf(env.stdout, "dry-run=true\nrc.unit=%s\n", unitPath)
		_, _ = fmt.Fprint(env.stdout, text)
		_, _ = fmt.Fprintf(env.stdout, "rc.exec=systemctl --user daemon-reload\nrc.exec=systemctl --user %s\n", strings.Join(enableArgs, " "))
		return 0
	}
	if err := rc.WriteUnit(unitPath, text); err != nil {
		return reportErr(env.stderr, err)
	}
	if err := rc.Systemctl(env.r, "daemon-reload"); err != nil {
		return reportErr(env.stderr, err)
	}
	if err := rc.Systemctl(env.r, enableArgs...); err != nil {
		return reportErr(env.stderr, err)
	}
	_, _ = fmt.Fprintf(env.stdout, "rc.unit=%s enabled=true started=%t\n", unitPath, env.o.now)
	if !env.o.now {
		_, _ = fmt.Fprintf(env.stderr, "%s: %s will run `dotf rc up` at next login (start now with: systemctl --user start %s)\n",
			progName, rc.UnitName, rc.UnitName)
	}
	return 0
}

// rcDisable disables the unit (with --now, also stops it; servers already
// running in Herdr are left alone) and removes the unit file dotf wrote.
func rcDisable(env rcEnv) int {
	if !rcServiceSupported(env) {
		return reportErr(env.stderr, fmt.Errorf("rc disable needs Linux with systemd (systemctl --user)"))
	}
	unitPath := rc.UnitPath(env.home)
	state, err := rc.ReadUnitState(unitPath)
	if err != nil {
		return reportErr(env.stderr, err)
	}
	switch {
	case state == rc.UnitAbsent:
		_, _ = fmt.Fprintf(env.stdout, "rc.unit=%s status=not-installed\n", unitPath)
		return 0
	case state == rc.UnitForeign && !env.o.force:
		return reportErr(env.stderr, fmt.Errorf("%s was not written by dotf; pass --force to remove it", unitPath))
	}
	disableArgs := []string{"disable"}
	if env.o.now {
		disableArgs = append(disableArgs, "--now")
	}
	disableArgs = append(disableArgs, rc.UnitName)
	if env.o.dryRun {
		_, _ = fmt.Fprintf(env.stdout, "dry-run=true\nrc.exec=systemctl --user %s\nrc.remove=%s\nrc.exec=systemctl --user daemon-reload\n",
			strings.Join(disableArgs, " "), unitPath)
		return 0
	}
	if err := rc.Systemctl(env.r, disableArgs...); err != nil {
		return reportErr(env.stderr, err)
	}
	if err := os.Remove(unitPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return reportErr(env.stderr, err)
	}
	if err := rc.Systemctl(env.r, "daemon-reload"); err != nil {
		return reportErr(env.stderr, err)
	}
	_, _ = fmt.Fprintf(env.stdout, "rc.unit=%s removed=true\n", unitPath)
	return 0
}
