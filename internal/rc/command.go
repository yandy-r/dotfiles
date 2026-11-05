package rc

import (
	"regexp"
	"slices"
	"strings"
)

// safeWordRe matches words a POSIX shell (and fish) reads literally, so they
// need no quoting.
var safeWordRe = regexp.MustCompile(`^[A-Za-z0-9_./:@%+,=-]+$`)

// ShellQuote returns s quoted for a POSIX shell: unchanged when it is a plain
// word, otherwise single-quoted with embedded single quotes escaped.
func ShellQuote(s string) string {
	if s != "" && safeWordRe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Args is the claude argv (without the executable) that starts r's server.
func (r Resolved) Args() []string {
	var args []string
	if r.AllowBypass {
		args = append(args, "--allow-dangerously-skip-permissions")
	}
	return append(args,
		"remote-control",
		"--name", r.Name,
		"--permission-mode", r.PermissionMode,
		"--spawn", r.Spawn,
	)
}

// ShellLine is the line typed into the Herdr pane's interactive shell. It
// starts with the env assignments (sorted, for stable output) and runs claude
// through `command`, so a user alias such as
// `alias claude='claude --dangerously-skip-permissions'` cannot expand and
// override the configured permission mode.
func (r Resolved) ShellLine() string {
	keys := make([]string, 0, len(r.Env))
	for k := range r.Env {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys)+len(r.Args())+2)
	for _, k := range keys {
		parts = append(parts, k+"="+ShellQuote(r.Env[k]))
	}
	parts = append(parts, "command", ShellQuote(r.Claude))
	for _, a := range r.Args() {
		parts = append(parts, ShellQuote(a))
	}
	return strings.Join(parts, " ")
}
