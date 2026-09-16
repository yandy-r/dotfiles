package main

import (
	"fmt"
	"slices"
	"strings"
)

// This file renders the bash half of the completion model: the `complete -F
// _dotf` function, its positional/flag case blocks, and the value-flag scan
// that keeps a flag ARGUMENT from being mistaken for a command positional.

// bashFlagWords is the space-joined "--flag" list for a command, plus the help
// flags every command accepts, for compgen -W word lists.
func bashFlagWords(c completionCommand) string {
	words := make([]string, 0, len(c.Flags)+2)
	for _, f := range c.Flags {
		words = append(words, "--"+f.Name)
	}
	words = append(words, "-h", "--help")
	return strings.Join(words, " ")
}

// bashValueCases renders the "$prev" case arms that complete a value flag's
// argument: an enum offers its word set, a path offers file completion (with
// compopt -o filenames so directories gain a trailing slash on bash >= 4;
// harmlessly ignored on bash 3.2), and a freeform flag gets no arm at all.
func bashValueCases(c completionCommand) string {
	var b strings.Builder
	for _, f := range c.Flags {
		if !f.TakesValue {
			continue
		}
		switch f.kind() {
		case valueEnum:
			fmt.Fprintf(&b, "                --%s) COMPREPLY=( $(compgen -W \"%s\" -- \"$cur\") ); return ;;\n",
				f.Name, strings.Join(f.Values, " "))
		case valuePath:
			fmt.Fprintf(&b, "                --%s) compopt -o filenames 2>/dev/null; COMPREPLY=( $(compgen -f -- \"$cur\") ); return ;;\n",
				f.Name)
		}
	}
	return b.String()
}

// bashValueFlagCases renders the `valflags` assignment arms: one per command
// that has at least one value-taking flag, listing that command's value flags.
// The generated positional scan consults the list so a flag's ARGUMENT is never
// counted as the command's positional — without it `dotf plasma --only kwinrc
// <TAB>` reads "kwinrc" as the verb and stops offering verbs entirely.
func bashValueFlagCases(m completionModel) string {
	var b strings.Builder
	for _, c := range m.Commands {
		var words []string
		for _, f := range c.Flags {
			if f.TakesValue {
				words = append(words, "--"+f.Name)
			}
		}
		if len(words) == 0 {
			continue
		}
		fmt.Fprintf(&b, "        %s) valflags=\"%s\" ;;\n", c.Name, strings.Join(words, " "))
	}
	return b.String()
}

// bashPositionalWords returns the compgen -W word-list expression for a
// positional command's candidates. help's set IS the command list the script
// already holds in $commands, so it is referenced rather than restated.
func bashPositionalWords(m completionModel, p positionalCommand) string {
	if slices.Equal(p.Candidates, m.commandNames()) {
		return "$commands"
	}
	return strings.Join(p.Candidates, " ")
}

// bashPositionalCases renders the case block for the positional-taking
// commands. Each arm completes a value flag's argument via "$prev" FIRST — that
// is what stops `plasma --only <TAB>` offering verb names as if they were file
// paths — then offers the flag list on a leading dash, or the positional
// candidates until one has been supplied.
func bashPositionalCases(m completionModel) string {
	var b strings.Builder
	for _, p := range positionalCommands(m) {
		c := m.command(p.Command)
		fmt.Fprintf(&b, "        %s)\n", p.Command)
		if vc := bashValueCases(c); vc != "" {
			b.WriteString("            case \"$prev\" in\n")
			b.WriteString(vc)
			b.WriteString("            esac\n")
		}
		b.WriteString("            if [[ \"$cur\" == -* ]]; then\n")
		fmt.Fprintf(&b, "                COMPREPLY=( $(compgen -W \"%s\" -- \"$cur\") )\n", bashFlagWords(c))
		b.WriteString("            elif [[ -z \"$pos_seen\" ]]; then\n")
		if p.IsPath {
			// compopt -o filenames gives the usual path behaviour (no space
			// after a directory, correct escaping). It is unavailable when the
			// function is invoked outside completion, hence the guard.
			b.WriteString("                COMPREPLY=( $(compgen -f -- \"$cur\") )\n")
			b.WriteString("                compopt -o filenames 2>/dev/null\n")
		} else {
			fmt.Fprintf(&b, "                COMPREPLY=( $(compgen -W \"%s\" -- \"$cur\") )\n", bashPositionalWords(m, p))
		}
		b.WriteString("            fi\n            return\n            ;;\n")
	}
	return b.String()
}

// generateBashCompletion renders a `complete -F _dotf` function. bash completion
// has no native description support, so only bareword flag/command/value names
// are embedded — no description escaping is needed.
func generateBashCompletion(m completionModel) string {
	p := m.Prog

	positional := make(map[string]bool, 4)
	for _, pc := range positionalCommands(m) {
		positional[pc.Command] = true
	}

	var cases strings.Builder
	for _, c := range m.Commands {
		// The positional-taking commands are handled by their own case block
		// above the generic flag case; flagless commands fall through to the
		// -h/--help default preset before the case.
		if positional[c.Name] || len(c.Flags) == 0 {
			continue
		}
		fmt.Fprintf(&cases, "        %s)\n", c.Name)
		if vc := bashValueCases(c); vc != "" {
			cases.WriteString("            case \"$prev\" in\n")
			cases.WriteString(vc)
			cases.WriteString("            esac\n")
		}
		fmt.Fprintf(&cases, "            flags=\"%s\" ;;\n", bashFlagWords(c))
	}

	const tmpl = `# bash completion for %[1]s
# Install with: %[1]s completion bash --install
_%[1]s() {
    local cur prev words cword cmd i j w pos_seen valflags flags
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    words=("${COMP_WORDS[@]}")
    cword=${COMP_CWORD}

    local commands="%[2]s"

    # Find the subcommand: the first non-flag word after argv[0].
    cmd=""
    for (( i=1; i < cword; i++ )); do
        case "${words[i]}" in
            -*) ;;
            *) cmd="${words[i]}"; break ;;
        esac
    done

    # No subcommand yet: complete command names (or the global help flags).
    if [[ -z "$cmd" ]]; then
        if [[ "$cur" == -* ]]; then
            COMPREPLY=( $(compgen -W "-h --help" -- "$cur") )
        else
            COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
        fi
        return
    fi

    # This subcommand's value-taking flags. Their ARGUMENT is a value, not the
    # command's positional, so the scan below skips the word that follows one.
    valflags=""
    case "$cmd" in
%[4]s    esac

    # Commands that take a positional argument. Each offers its positional only
    # until one has been supplied (scan the words after the subcommand). A
    # "--flag=value" word carries its own value and needs no skip; a bare
    # "--flag" listed in valflags consumes the next word.
    pos_seen=""
    for (( j=i+1; j < cword; j++ )); do
        w="${words[j]}"
        case "$w" in
            -*)
                if [[ " $valflags " == *" $w "* ]]; then
                    j=$((j+1))
                fi
                ;;
            *) pos_seen=1; break ;;
        esac
    done
    case "$cmd" in
%[5]s    esac

    # Flag-only commands. A value flag's argument is completed via the "$prev"
    # arms (enum word list or file completion); otherwise offer the flag list on
    # a leading dash.
    flags="-h --help"
    case "$cmd" in
%[3]s    esac

    [[ "$cur" == -* ]] && COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
    return
}
complete -F _%[1]s %[1]s
`
	return fmt.Sprintf(tmpl, p, strings.Join(m.commandNames(), " "),
		cases.String(), bashValueFlagCases(m), bashPositionalCases(m))
}
