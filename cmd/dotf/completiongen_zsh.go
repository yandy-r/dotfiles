package main

import (
	"fmt"
	"strings"
)

// This file renders the zsh half of the completion model: an autoloadable
// #compdef function built from _arguments specs, dual-mode so the same file
// works autoloaded off $fpath and sourced directly.

// zshValue escapes a string for a single-quoted zsh word (a _describe
// 'name:desc' entry). Inside single quotes only ' is special, escaped as the
// close-literal-reopen idiom '\”.
func zshValue(s string) string {
	return strings.ReplaceAll(firstLine(s), "'", `'\''`)
}

// zshDesc escapes a string for a _arguments description bracket: a ] would close
// the bracket early so it is backslash-escaped, and ' uses the single-quote
// idiom. The spec strings feed our own controlled usage text, not user input.
func zshDesc(s string) string {
	s = firstLine(s)
	s = strings.ReplaceAll(s, "]", `\]`)
	s = strings.ReplaceAll(s, "'", `'\''`)
	return s
}

// zshFlagSpec renders one flag as an _arguments spec: repeatable flags gain a
// leading *, and a value flag gains an action matching its kind — a (value set)
// for an enum, :_files for a path, or a bare message (no completion) otherwise.
func zshFlagSpec(f completionFlag) string {
	prefix := ""
	if f.Repeatable {
		prefix = "*"
	}
	spec := "'" + prefix + "--" + f.Name + "[" + zshDesc(f.Desc) + "]"
	if f.TakesValue {
		switch f.kind() {
		case valueEnum:
			spec += ":" + f.Name + ":(" + strings.Join(f.Values, " ") + ")"
		case valuePath:
			spec += ":" + f.Name + ":_files"
		default:
			spec += ":" + f.Name + ":"
		}
	}
	return spec + "'"
}

// zshArgumentsBlock renders a backslash-continued _arguments call from specs.
func zshArgumentsBlock(specs []string, indent string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s_arguments \\\n", indent)
	for i, s := range specs {
		suffix := " \\"
		if i == len(specs)-1 {
			suffix = ""
		}
		fmt.Fprintf(&b, "%s    %s%s\n", indent, s, suffix)
	}
	return b.String()
}

// helpSpec is the standard mutually-exclusive -h/--help _arguments spec.
const helpSpec = `'(-h --help)'{-h,--help}'[show help]'`

// zshSubBody renders the args-state body for one subcommand: help takes a
// command-name positional, completion takes its flags plus a shell positional,
// setup takes its flags plus a module positional, plasma takes its flags plus a
// verb positional, and every other command exposes its flags.
func zshSubBody(c completionCommand, m completionModel) string {
	specs := []string{helpSpec}
	for _, f := range c.Flags {
		specs = append(specs, zshFlagSpec(f))
	}
	if p, ok := positionalOf(m, c.Name); ok {
		specs = append(specs, zshPositionalSpec(p))
	}
	return zshArgumentsBlock(specs, "                ")
}

// zshPositionalSpec renders a positional's _arguments spec: _files for a path
// positional, or its candidate set as an explicit action.
func zshPositionalSpec(p positionalCommand) string {
	if p.IsPath {
		return "'1:" + p.Label + ":_files'"
	}
	return "'1:" + p.Label + ":(" + strings.Join(p.Candidates, " ") + ")'"
}

// generateZshCompletion renders an autoloadable #compdef function. The header
// compdef line and the trailing funcstack guard make the same file work both
// when autoloaded off $fpath and when sourced directly (source <(dotf
// completion zsh)) — the battle-tested dual-mode pattern.
func generateZshCompletion(m completionModel) string {
	p := m.Prog

	var cmds strings.Builder
	for _, c := range m.Commands {
		fmt.Fprintf(&cmds, "                '%s:%s'\n", c.Name, zshValue(c.Summary))
	}

	var branches strings.Builder
	for _, c := range m.Commands {
		fmt.Fprintf(&branches, "            %s)\n", c.Name)
		branches.WriteString(zshSubBody(c, m))
		branches.WriteString("                ;;\n")
	}

	const tmpl = `#compdef %[1]s
compdef _%[1]s %[1]s
# zsh completion for %[1]s
# Install with: %[1]s completion zsh --install
_%[1]s() {
    local curcontext="$curcontext" state line
    typeset -A opt_args

    _arguments -C \
        '(-h --help)'{-h,--help}'[show help]' \
        '1: :->command' \
        '*:: :->args' && return 0

    case $state in
        command)
            local -a commands
            commands=(
%[2]s            )
            _describe -t commands '%[1]s command' commands
            ;;
        args)
            case $line[1] in
%[3]s            esac
            ;;
    esac
}

if [ "$funcstack[1]" = "_%[1]s" ]; then
    _%[1]s "$@"
fi
`
	return fmt.Sprintf(tmpl, p, cmds.String(), branches.String())
}
