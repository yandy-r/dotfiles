package rc

import (
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"plain":         "plain",
		"/a/b-c_d.e":    "/a/b-c_d.e",
		"":              "''",
		"has space":     "'has space'",
		"it's":          `'it'\''s'`,
		"$(rm -rf /)":   "'$(rm -rf /)'",
		"a;b":           "'a;b'",
		"--name=x":      "--name=x",
		"semi`colon`":   "'semi`colon`'",
		"glob*":         "'glob*'",
		"tilde~":        "'tilde~'",
		"new\nline":     "'new\nline'",
		"DO_NOT_TRACK=": "DO_NOT_TRACK=",
	}
	for in, want := range cases {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShellLine(t *testing.T) {
	r := Resolved{
		Name: "app", Path: "/p/app", PermissionMode: "auto", Spawn: "worktree", AllowBypass: true, Claude: "claude",
		Env: map[string]string{"Z": "last", "DO_NOT_TRACK": "0", "SP": "a b"},
	}
	want := "DO_NOT_TRACK=0 SP='a b' Z=last command claude --allow-dangerously-skip-permissions remote-control " +
		"--name app --permission-mode auto --spawn worktree"
	if got := r.ShellLine(); got != want {
		t.Errorf("ShellLine =\n %s\nwant\n %s", got, want)
	}
	r.AllowBypass, r.Env = false, nil
	if got := r.ShellLine(); strings.Contains(got, "allow-dangerously") || !strings.HasPrefix(got, "command claude remote-control") {
		t.Errorf("ShellLine without bypass = %s", got)
	}
}
