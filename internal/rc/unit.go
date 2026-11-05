package rc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
)

// UnitName is the systemd user unit `dotf rc enable` installs.
const UnitName = "dotf-rc.service"

// unitMarker is the first line of every unit dotf writes; enable/disable only
// overwrite or delete a unit file that carries it.
const unitMarker = "# Managed by dotf rc - regenerate with `dotf rc enable`."

// UnitPath is ${XDG_CONFIG_HOME:-<home>/.config}/systemd/user/dotf-rc.service.
func UnitPath(home string) string {
	base := filepath.Join(home, ".config")
	if v := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(v) {
		base = v
	}
	return filepath.Join(base, "systemd", "user", UnitName)
}

// RenderUnit returns the unit text. dotfBin is the absolute dotf executable;
// configPath, when non-empty, is passed to `dotf rc up --config`; pathEnv is
// the PATH the unit runs with, so herdr resolves under systemd's
// otherwise minimal user environment. The unit is a oneshot: `dotf rc up`
// starts the servers in Herdr and exits. KillMode=process keeps a stop or
// restart of the unit from killing the Herdr server (and every pane in it)
// that `up` may have launched into the unit's cgroup.
func RenderUnit(dotfBin, configPath, pathEnv string) string {
	var b strings.Builder
	b.WriteString(unitMarker + "\n")
	b.WriteString("[Unit]\n")
	b.WriteString("Description=Claude Code Remote Control servers (dotf rc)\n")
	b.WriteString("Documentation=https://github.com/yandy-r/dotfiles/blob/main/docs/features/dotf-rc.md\n")
	// No After=default.target: a target orders itself after the units it
	// wants, so that would be a cycle. network-online.target is a system-only
	// target; a user unit cannot order against it.
	b.WriteString("\n")
	b.WriteString("[Service]\n")
	b.WriteString("Type=oneshot\n")
	b.WriteString("RemainAfterExit=yes\n")
	fmt.Fprintf(&b, "Environment=%s\n", systemdQuote("PATH="+pathEnv))
	exec := execQuote(dotfBin) + " rc up"
	if configPath != "" {
		exec += " --config " + execQuote(configPath)
	}
	fmt.Fprintf(&b, "ExecStart=%s\n", exec)
	b.WriteString("KillMode=process\n")
	// No start timeout: up's per-repo waits (stop, shell ready, verify) add up
	// with several repos on a slow login, and a timeout would kill it midway.
	b.WriteString("TimeoutStartSec=infinity\n\n")
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=default.target\n")
	return b.String()
}

// systemdQuote double-quotes s for a unit file when it contains whitespace or
// quotes; systemd also expands % specifiers, so % is doubled.
func systemdQuote(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	if !strings.ContainsAny(s, " \t\"'\\") {
		return s
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// execQuote quotes an ExecStart= argument: systemdQuote, plus $ doubled,
// because systemd expands $VAR in command lines (not in Environment=).
func execQuote(s string) string {
	return systemdQuote(strings.ReplaceAll(s, "$", "$$"))
}

// UnitState describes the unit file on disk.
type UnitState int

// Unit file states.
const (
	UnitAbsent  UnitState = iota // no file
	UnitManaged                  // a file dotf wrote
	UnitForeign                  // a file dotf did not write
)

// ReadUnitState classifies the file at path.
func ReadUnitState(path string) (UnitState, error) {
	data, err := os.ReadFile(path) //nolint:gosec // fixed unit path
	if errors.Is(err, fs.ErrNotExist) {
		return UnitAbsent, nil
	}
	if err != nil {
		return UnitAbsent, err
	}
	if strings.HasPrefix(string(data), unitMarker) {
		return UnitManaged, nil
	}
	return UnitForeign, nil
}

// WriteUnit writes text to path, creating the parent directory.
func WriteUnit(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644) //nolint:gosec // unit files are world-readable by convention
}

// Systemctl runs `systemctl --user <args...>`.
func Systemctl(r exec.Runner, args ...string) error {
	_, err := r.Output("systemctl", append([]string{"--user"}, args...)...)
	return stderrOf(err)
}

// UnitEnabled reports `systemctl --user is-enabled` and `is-active` for the
// unit as plain strings ("enabled"/"disabled"/..., "active"/"inactive"/...).
// A missing systemctl yields "unknown" for both.
func UnitEnabled(r exec.Runner) (enabled, active string) {
	q := func(verb string) string {
		out, _ := r.Output("systemctl", "--user", verb, UnitName)
		if s := strings.TrimSpace(string(out)); s != "" {
			return s
		}
		return "unknown"
	}
	return q("is-enabled"), q("is-active")
}
