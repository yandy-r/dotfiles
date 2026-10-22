// Command dotf is the provisioning CLI for the dotfiles repository.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/version"
)

const progName = "dotf"

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches the requested subcommand and returns the process exit code.
//
// Help is intercepted in two places so every command gets rich, consistent
// help without changing any runX signature: (1) a leading "help"/"-h"/"--help"
// token routes to runHelp BEFORE the top-level parse (the top-level flagset
// discards output, so a help flag would otherwise become a bare parse error and
// exit 2); (2) once a KNOWN command's remaining args request help, its help is
// printed and run returns 0 BEFORE the switch, so version/platform and the
// side-effecting apply/sysconfig get --help/-h for free and never enter their
// runX body.
func run(args []string) int {
	if len(args) > 0 && isHelpToken(args[0]) {
		return runHelp(args[1:], os.Stdout, os.Stderr)
	}

	fs := flag.NewFlagSet(progName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", progName, err)
		return 2
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "%s: missing subcommand\n\n", progName)
		writeUsageError(os.Stderr)
		return 1
	}

	cmd := fs.Arg(0)
	rest := fs.Args()[1:]

	// Command-help short-circuit: only KNOWN commands qualify, so
	// "dotf bogus --help" still falls through to the unknown-subcommand path.
	if ch, ok := lookupHelp(cmd); ok && wantsHelp(rest) {
		printCommandHelp(os.Stdout, ch)
		return 0
	}

	switch cmd {
	case "version":
		runVersion()
		return 0
	case "platform":
		runPlatform()
		return 0
	case "plan":
		return runPlan(rest, os.Stdout, os.Stderr, exec.New())
	case "install":
		return runInstall(rest, os.Stdout, os.Stderr, exec.New())
	case "status":
		return runStatus(rest, os.Stdout, os.Stderr, exec.New())
	case "diff":
		return runDiff(rest, os.Stdout, os.Stderr, exec.New())
	case "doctor":
		return runDoctor(rest, os.Stdout, os.Stderr, exec.New())
	case "link":
		return runLink(rest, os.Stdout, os.Stderr, exec.New())
	case "unlink":
		return runUnlink(rest, os.Stdout, os.Stderr)
	case "backup":
		return runBackup(rest, os.Stdout, os.Stderr, os.Stdin)
	case "sysconfig":
		return runSysconfig(rest, os.Stdout, os.Stderr, exec.New())
	case "apply":
		return runApply(rest, os.Stdout, os.Stderr, exec.New())
	case "lock":
		return runLock(rest, os.Stdout, os.Stderr, exec.New())
	case "self-update":
		return runSelfUpdate(rest, os.Stdout, os.Stderr, exec.New())
	case "setup":
		return runSetup(rest, os.Stdout, os.Stderr, exec.New())
	case "plasma":
		return runPlasma(rest, os.Stdout, os.Stderr, exec.New())
	case "deb":
		return runDeb(rest, os.Stdout, os.Stderr, exec.New())
	case "completion":
		return runCompletion(rest, os.Stdout, os.Stderr)
	default:
		fmt.Fprintf(os.Stderr, "%s: unknown subcommand %q\n\n", progName, cmd)
		writeUsageError(os.Stderr)
		return 1
	}
}

// runVersion prints the CLI's version metadata to stdout.
func runVersion() {
	fmt.Printf("%s %s\n", progName, version.String())
}

// runPlatform prints the detected platform Info as machine-greppable
// key=value pairs, a machine-greppable analogue of
// common/platform.sh:print_platform_info's human-readable output.
func runPlatform() {
	info := platform.Detect(exec.New())
	fmt.Printf("os=%s arch=%s pkg=%s container=%s distro=%s\n",
		info.OS, info.Arch, info.PkgMgr, info.Container, info.Distro)
}
