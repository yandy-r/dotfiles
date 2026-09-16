package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// This file holds the help LOGIC and rendering only — no prose literals (those
// live in helptext.go). Every printer takes an io.Writer so it is unit-testable
// with a bytes.Buffer and has no runtime side effects.

// lookupHelp returns the registry entry for name and whether it exists.
func lookupHelp(name string) (commandHelp, bool) {
	for _, ch := range helpRegistry {
		if ch.name == name {
			return ch, true
		}
	}
	return commandHelp{}, false
}

// isHelpToken reports whether s is a global-help request as the FIRST argument:
// the bare word "help" or a help flag. Used by run() before the top-level parse
// so "dotf help", "dotf --help", and "dotf -h" all route to global help.
func isHelpToken(s string) bool {
	switch s {
	case "help", "-h", "-help", "--help":
		return true
	default:
		return false
	}
}

// wantsHelp reports whether a command's remaining args request help via a bare
// -h/-help/--help flag before an optional "--" terminator. The bare word "help"
// is deliberately NOT a help request here — inside a command it would just be a
// positional argument. This is a token scan, not flag-arity-aware, so
// "install --intent -h" shows help rather than binding -h as the value; "--"
// stops the scan for the rare literal case.
func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		switch a {
		case "-h", "-help", "--help":
			return true
		}
	}
	return false
}

// runHelp implements "dotf help [command]". With no argument it prints global
// help to stdout (exit 0). With a known command name it prints that command's
// help to stdout (exit 0). With an unknown name it writes an unknown-subcommand
// diagnostic plus the usage block to stderr (exit 1), keeping stdout empty and
// consistent with "dotf <bogus>".
func runHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printGlobalHelp(stdout)
		return 0
	}

	name := args[0]
	ch, ok := lookupHelp(name)
	if !ok {
		_, _ = fmt.Fprintf(stderr, "%s: unknown subcommand %q\n\n", progName, name)
		writeUsageError(stderr)
		return 1
	}

	printCommandHelp(stdout, ch)
	return 0
}

// printGlobalHelp renders the full global help: tagline header, Usage, the
// Commands table, the Conventions block, the Environment section, and the
// per-command hint.
func printGlobalHelp(w io.Writer) {
	_, _ = fmt.Fprintf(w, "%s - %s\n\n", progName, tagline)
	_, _ = io.WriteString(w, "Usage:\n  dotf <command> [flags]\n  dotf help [command]\n\n")
	_, _ = io.WriteString(w, "Commands:\n")
	writeCommands(w)
	_, _ = fmt.Fprintf(w, "\n%s\n", conventions)
	_, _ = io.WriteString(w, "\nEnvironment:\n")
	writeIndented(w, envHelp)
	_, _ = io.WriteString(w, "\nRun \"dotf help <command>\" for detailed help on a command.\n")
}

// printCommandHelp renders one command's detailed help: the "dotf <name> -
// <summary>" header, Usage, the Description (long), the Flags section (only when
// the command has flags), and Examples.
func printCommandHelp(w io.Writer, ch commandHelp) {
	_, _ = fmt.Fprintf(w, "%s %s - %s\n", progName, ch.name, ch.summary)
	_, _ = fmt.Fprintf(w, "\nUsage:\n  %s\n", ch.usage)

	if ch.long != "" {
		_, _ = io.WriteString(w, "\nDescription:\n")
		writeIndented(w, ch.long)
	}

	if ch.setFlags != nil {
		_, _ = io.WriteString(w, "\n")
		renderFlags(w, ch.setFlags)
	}

	if len(ch.examples) > 0 {
		_, _ = io.WriteString(w, "\nExamples:\n")
		for _, ex := range ch.examples {
			_, _ = fmt.Fprintf(w, "  %s\n", ex)
		}
	}
}

// writeCommands writes the aligned "  <name>  <summary>" table generated from
// helpRegistry, so the global-help and error-path command lists are always the
// same and can never drift from the real command set. Column width is computed
// from the longest registry name.
func writeCommands(w io.Writer) {
	width := 0
	for _, ch := range helpRegistry {
		if len(ch.name) > width {
			width = len(ch.name)
		}
	}
	for _, ch := range helpRegistry {
		_, _ = fmt.Fprintf(w, "  %-*s  %s\n", width, ch.name, ch.summary)
	}
}

// writeUsageError writes the lean stderr usage block reused by both error paths
// (missing subcommand, unknown subcommand): a Usage line, the same Commands
// table as global help, and the per-command hint — without the tagline,
// Conventions, or Environment sections.
func writeUsageError(w io.Writer) {
	_, _ = io.WriteString(w, "Usage:\n  dotf <command> [flags]\n\nCommands:\n")
	writeCommands(w)
	_, _ = io.WriteString(w, "\nRun \"dotf help <command>\" for detailed help on a command.\n")
}

// renderFlags prints a command's Flags section by registering its real flags on
// a throwaway flagset and walking them — the usage text comes verbatim from the
// flagset (DRY), never retyped. Flags render as "--name [placeholder]" with
// aligned descriptions. Placeholders come from flag.UnquoteUsage: bool flags
// get none, string flags "string", the repeatable multiFlag "value".
// flag.VisitAll iterates lexicographically, so the section is alphabetical
// (matching flag.PrintDefaults).
func renderFlags(w io.Writer, setFlags func(*flag.FlagSet)) {
	fs := flag.NewFlagSet("help", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	setFlags(fs)

	type flagRow struct{ left, usage string }
	var rows []flagRow
	width := 0
	fs.VisitAll(func(f *flag.Flag) {
		placeholder, usage := flag.UnquoteUsage(f)
		left := "--" + f.Name
		if placeholder != "" {
			left += " " + placeholder
		}
		if len(left) > width {
			width = len(left)
		}
		rows = append(rows, flagRow{left: left, usage: usage})
	})

	if len(rows) == 0 {
		return
	}

	_, _ = io.WriteString(w, "Flags:\n")
	for _, row := range rows {
		_, _ = fmt.Fprintf(w, "  %-*s  %s\n", width, row.left, row.usage)
	}
}

// writeIndented prefixes each newline-separated line of text with two spaces.
func writeIndented(w io.Writer, text string) {
	for _, line := range strings.Split(text, "\n") {
		_, _ = fmt.Fprintf(w, "  %s\n", line)
	}
}
