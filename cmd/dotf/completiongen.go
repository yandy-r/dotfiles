package main

import (
	"flag"
	"io"
	"regexp"
	"slices"
	"strings"
)

// This file builds the completion MODEL from the help registry. The model is
// derived from helpRegistry and each command's setFlags (the same source the
// help renderer uses), so a generated completion can never drift from the real
// command/flag surface — adding a command or flag updates completions for free.
//
// The three renderers live in sibling files (completiongen_bash.go,
// completiongen_zsh.go, completiongen_fish.go), so each shell's quoting rules
// stay in one place and every file stays under the ~500-line soft cap. The
// command dispatch, flag parsing, and install logic live in completion.go.

// completionShells is the ordered set of shells `dotf completion` supports. It
// backs shell-name validation, $SHELL detection, and the shell-argument
// completions embedded in the generated scripts.
var completionShells = []string{"bash", "zsh", "fish"}

// completionModules is the baked-in set of `dotf setup <domain>` module names,
// offered as the setup positional's completion candidates. Modules are a tiny,
// repo-curated set (unlike the large, overlay-dynamic package set), so a static
// bake is safe — and TestCompletionModules_MatchManifest guards it against
// drifting from the embedded manifest's [[module]] names.
var completionModules = []string{"fingerprint", "onedrive"}

// completionPlasmaVerbs is the `dotf plasma <verb>` positional's completion
// candidate set. It aliases plasmaVerbs rather than restating it, so the
// dispatch switch, the usage-error messages, and all three generated scripts
// resolve to one list — a fifth verb becomes completable the moment it becomes
// dispatchable.
var completionPlasmaVerbs = plasmaVerbs

// enumUsageRe extracts a "value set" from a flag's usage string: a run of
// pipe-separated bareword tokens like "ufw|firewalld|auto|off" or "true|false".
// This lets a flag whose usage already documents its choices offer them as
// completion candidates without hardcoding the set (kept DRY with the usage
// text). A flag whose usage has no such run degrades to path/no completion.
var enumUsageRe = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9._+-]*(?:\|[A-Za-z0-9._+-]+)+`)

// valueKind classifies how a value-taking flag's argument should be completed.
type valueKind int

const (
	valueNone valueKind = iota // freeform: offer nothing (safer than misleading files)
	valuePath                  // a filesystem path: offer file completion
	valueEnum                  // a fixed set parsed from the usage string
)

// completionFlag is one flag's completion-relevant metadata, distilled from a
// flag.Flag: whether it consumes a value, whether it is repeatable (the --intent
// multiFlag, so zsh allows repeats), and — for value flags — its documented
// value set (Values) or whether the value is a path (IsPath).
type completionFlag struct {
	Name       string
	Desc       string
	TakesValue bool
	Repeatable bool
	Values     []string
	IsPath     bool
}

// kind reports how flag f's argument should be completed. Meaningful only when
// f.TakesValue.
func (f completionFlag) kind() valueKind {
	switch {
	case len(f.Values) > 0:
		return valueEnum
	case f.IsPath:
		return valuePath
	default:
		return valueNone
	}
}

// completionCommand is one subcommand: its name, one-line summary, and flags.
type completionCommand struct {
	Name    string
	Summary string
	Flags   []completionFlag
}

// completionModel is the whole CLI's completion surface.
type completionModel struct {
	Prog     string
	Commands []completionCommand
}

// buildCompletionModel derives the model from helpRegistry. Flags are read off a
// throwaway flagset exactly as the help renderer does (renderFlags), so usage
// text and the value/repeatable/enum classification come from the real flag
// definitions, never retyped. flag.VisitAll yields flags lexicographically, so
// the generated flag lists are deterministic.
func buildCompletionModel() completionModel {
	m := completionModel{Prog: progName}
	for _, ch := range helpRegistry {
		cc := completionCommand{Name: ch.name, Summary: ch.summary}
		if ch.setFlags != nil {
			fs := flag.NewFlagSet(ch.name, flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			ch.setFlags(fs)
			fs.VisitAll(func(f *flag.Flag) {
				placeholder, usage := flag.UnquoteUsage(f)
				usage = firstLine(usage)
				_, repeatable := f.Value.(*multiFlag)
				cf := completionFlag{
					Name:       f.Name,
					Desc:       usage,
					TakesValue: placeholder != "",
					Repeatable: repeatable,
				}
				if cf.TakesValue {
					cf.Values = parseEnumUsage(usage)
					cf.IsPath = cf.Values == nil && strings.Contains(strings.ToLower(usage), "path")
				}
				cc.Flags = append(cc.Flags, cf)
			})
		}
		m.Commands = append(m.Commands, cc)
	}
	return m
}

// parseEnumUsage returns the pipe-separated value set documented in a usage
// string (e.g. "ufw|firewalld|auto|off" -> [ufw firewalld auto off]), or nil
// when the usage documents no such set.
func parseEnumUsage(usage string) []string {
	if m := enumUsageRe.FindString(usage); m != "" {
		return strings.Split(m, "|")
	}
	return nil
}

// generateCompletion renders the model as a completion script for shell. shell
// must already be validated (isSupportedShell); an unknown shell yields "".
func generateCompletion(shell string, m completionModel) string {
	switch shell {
	case "bash":
		return generateBashCompletion(m)
	case "zsh":
		return generateZshCompletion(m)
	case "fish":
		return generateFishCompletion(m)
	default:
		return ""
	}
}

// commandNames returns the command names in registry order.
func (m completionModel) commandNames() []string {
	names := make([]string, len(m.Commands))
	for i, c := range m.Commands {
		names[i] = c.Name
	}
	return names
}

// command returns the named command, or a zero value when absent.
func (m completionModel) command(name string) completionCommand {
	for _, c := range m.Commands {
		if c.Name == name {
			return c
		}
	}
	return completionCommand{}
}

// firstLine trims text to its first line, so a flag usage string embedded in a
// completion description is always single-line.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// ----------------------------------------------------------------------------
// positional model
// ----------------------------------------------------------------------------

// positionalCommand is one command whose first non-flag argument is a
// positional drawn from a baked candidate set.
type positionalCommand struct {
	// Command is the subcommand that owns the positional.
	Command string
	// Label names the positional wherever a generated script shows it: zsh's
	// _arguments tag and fish's description column ("verb", "domain", …). It
	// matches the placeholder in the command's documented usage line.
	Label string
	// Candidates is the completable value set for that positional. It is empty
	// when IsPath is set.
	Candidates []string
	// IsPath marks a positional that takes a filesystem path rather than a
	// value from a fixed set, so each shell offers FILE completion for it.
	// Without this a path positional would offer nothing at all, since the
	// generated scripts complete only what the model describes.
	IsPath bool
}

// positionalCommands is the ordered set of positional-taking commands and the
// candidate sets they complete into. It is the single answer to "which words
// can appear on a `dotf` line WITHOUT being the subcommand", which is what both
// generated-script disambiguation problems reduce to:
//
//   - fish's __fish_seen_subcommand_from matches a token ANYWHERE on the line,
//     so `dotf plasma apply` satisfies both plasma's and the top-level apply
//     command's rules (see fishSubcommandGuard);
//   - bash scans for the first non-flag word after the subcommand, which is only
//     the positional once a value flag's argument has been skipped (see the
//     generated pos_seen loop).
//
// Deriving both from this one list means a fifth plasma verb — or a new module
// — that collides with a command name is disambiguated the moment it is added,
// with no second enumeration to keep in sync.
func positionalCommands(m completionModel) []positionalCommand {
	return []positionalCommand{
		{Command: "completion", Label: "shell", Candidates: completionShells},
		{Command: "deb", Label: "path", IsPath: true},
		{Command: "help", Label: "command", Candidates: m.commandNames()},
		{Command: "plasma", Label: "verb", Candidates: completionPlasmaVerbs},
		{Command: "setup", Label: "domain", Candidates: completionModules},
	}
}

// positionalOf returns the positional description for command name, and whether
// it has one.
func positionalOf(m completionModel, name string) (positionalCommand, bool) {
	for _, p := range positionalCommands(m) {
		if p.Command == name {
			return p, true
		}
	}
	return positionalCommand{}, false
}

// positionalOwnersOf returns the positional-taking commands whose candidate set
// contains name — i.e. every command that can legitimately put name on the line
// as an ARGUMENT rather than as the subcommand. name itself is never returned.
func positionalOwnersOf(m completionModel, name string) []string {
	var owners []string
	for _, p := range positionalCommands(m) {
		if p.Command != name && slices.Contains(p.Candidates, name) {
			owners = append(owners, p.Command)
		}
	}
	return owners
}
