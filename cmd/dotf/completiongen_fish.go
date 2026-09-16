package main

import (
	"fmt"
	"strings"
)

// This file renders the fish half of the completion model: declarative
// `complete` lines, plus the guard clauses that keep a command name appearing
// as another command's positional value from claiming the wrong rules.

// fishQuote escapes a string for a single-quoted fish word. Inside single quotes
// fish treats \ and ' specially, so both are backslash-escaped.
func fishQuote(s string) string {
	s = firstLine(s)
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "'", `\'`)
	return s
}

// fishValueOpts returns the trailing `complete` options for a value flag's
// argument: -x plus the enum set, -r -F (require + force-files) for a path, or a
// bare -r (require, no candidates — the global -f suppresses file fallback) for
// a freeform value.
func fishValueOpts(f completionFlag) string {
	switch f.kind() {
	case valueEnum:
		return " -x -a '" + strings.Join(f.Values, " ") + "'"
	case valuePath:
		return " -r -F"
	default:
		return " -r"
	}
}

// fishSeenNone renders the "; and not __fish_seen_subcommand_from …" clauses
// that exclude every name in names, or "" when there is nothing to exclude.
// Duplicates are dropped so a name reachable two ways — `plasma` is both a
// command and one of help's candidates — is excluded exactly once.
func fishSeenNone(names []string) string {
	var b strings.Builder
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		b.WriteString("; and not __fish_seen_subcommand_from " + n)
	}
	return b.String()
}

// fishSubcommandGuard renders the -n condition that holds only when name is the
// REAL subcommand of the line being completed.
//
// __fish_seen_subcommand_from matches a token ANYWHERE on the line, so a
// command name that is also another command's positional value satisfies both
// commands' rules: `dotf plasma apply --<TAB>` was offering the top-level apply
// command's flags alongside plasma's, every one of which is an exit-2 usage
// error for plasma. Excluding the commands that can put name on the line as an
// ARGUMENT (positionalOwnersOf) makes the winner unambiguous.
func fishSubcommandGuard(m completionModel, name string) string {
	return fishGuard(name, positionalOwnersOf(m, name))
}

// fishGuard renders "this token is present, and none of exclude is".
func fishGuard(name string, exclude []string) string {
	return "__fish_seen_subcommand_from " + name + fishSeenNone(exclude)
}

// fishPositionalGuard renders the -n condition for a positional command's
// candidate rule: the command must be the real subcommand (fishSubcommandGuard),
// no OTHER positional command may be on the line, and the positional must not
// already have been supplied — a candidate on the line means it has.
func fishPositionalGuard(m completionModel, p positionalCommand) string {
	exclude := positionalOwnersOf(m, p.Command)
	for _, q := range positionalCommands(m) {
		if q.Command != p.Command {
			exclude = append(exclude, q.Command)
		}
	}
	for _, cand := range p.Candidates {
		if cand != p.Command {
			exclude = append(exclude, cand)
		}
	}
	return fishGuard(p.Command, exclude)
}

// generateFishCompletion renders declarative `complete` lines. A global -f turns
// off fish's default file fallback so flag-only subcommands do not dump the cwd;
// path flags re-enable files with -F. Subcommands gate on __fish_use_subcommand
// and per-command rules on __fish_seen_subcommand_from.
func generateFishCompletion(m completionModel) string {
	p := m.Prog
	var b strings.Builder
	line := func(s string) { fmt.Fprintln(&b, s) }

	line("# fish completion for " + p)
	line("# Install with: " + p + " completion fish --install")
	line("")
	line("# Disable the default file fallback; path flags re-enable it with -F.")
	line("complete -c " + p + " -f")
	line("complete -c " + p + " -s h -l help -d 'Show help'")
	line("")
	line("# subcommands")
	for _, c := range m.Commands {
		line("complete -c " + p + " -n '__fish_use_subcommand' -a " + c.Name +
			" -d '" + fishQuote(c.Summary) + "'")
	}

	for _, c := range m.Commands {
		if len(c.Flags) == 0 {
			continue
		}
		line("")
		line("# " + c.Name + " flags")
		guard := fishSubcommandGuard(m, c.Name)
		for _, f := range c.Flags {
			s := "complete -c " + p + " -n '" + guard +
				"' -l " + f.Name + " -d '" + fishQuote(f.Desc) + "'"
			if f.TakesValue {
				s += fishValueOpts(f)
			}
			line(s)
		}
	}

	// The positional rules for completion/help/setup/plasma must not cross-fire:
	// __fish_seen_subcommand_from matches a token anywhere on the line, so each
	// rule explicitly excludes the others and its own already-supplied argument.
	for _, pc := range positionalCommands(m) {
		line("")
		line("# " + pc.Command + ": " + pc.Label + " argument")
		if pc.IsPath {
			// -F forces file completion, which fish otherwise suppresses for a
			// command that has any -a rules at all.
			line("complete -c " + p + " -n '" + fishPositionalGuard(m, pc) + "' -F")
			continue
		}
		line("complete -c " + p + " -n '" + fishPositionalGuard(m, pc) + "' -a '" +
			strings.Join(pc.Candidates, " ") + "' -d '" + pc.Label + "'")
	}

	return b.String()
}
