package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// This file is `dotf completion`: it prints, or with --install writes, a shell
// completion script for bash/zsh/fish. The script content is generated in
// completiongen.go from the help registry, so this file only handles the
// command surface — flag parsing, shell resolution, and install placement.

// completionOpts holds the parsed flag values for `dotf completion`.
type completionOpts struct {
	install, force, dryRun bool
}

// setCompletionFlags registers completion's flags on fs and returns the opts
// pointer they parse into. Single source of truth for completion's flag surface,
// shared by runCompletion and the help renderer (DRY — usage strings live only
// here).
func setCompletionFlags(fs *flag.FlagSet) *completionOpts {
	o := &completionOpts{}
	fs.BoolVar(&o.install, "install", false,
		"install the script into the shell's completion directory instead of printing it")
	fs.BoolVar(&o.force, "force", false,
		"overwrite an existing installed completion script")
	fs.BoolVar(&o.dryRun, "dry-run", false,
		"with --install, report the target path without writing")
	return o
}

// runCompletion prints (or installs) a shell completion script. The shell is an
// optional positional argument (bash|zsh|fish); when omitted it is detected from
// $SHELL. It writes the script to stdout, or with --install writes it to the
// shell's standard completion directory (--force overwrites, --dry-run previews).
func runCompletion(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("completion", flag.ContinueOnError)
	fs.SetOutput(stderr)
	o := setCompletionFlags(fs)

	// The shell is a positional argument and every flag is boolean, so pull the
	// positional out first: this makes "completion zsh --install" and
	// "completion --install zsh" equivalent, sidestepping flag's stop-at-first-
	// positional parsing.
	shell, flagArgs, err := splitCompletionArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s completion: %v\n", progName, err)
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	if shell == "" {
		shell = detectShell()
	}
	if !isSupportedShell(shell) {
		if shell == "" {
			_, _ = fmt.Fprintf(stderr, "%s completion: cannot detect shell from $SHELL; specify one of: %s\n",
				progName, strings.Join(completionShells, ", "))
		} else {
			_, _ = fmt.Fprintf(stderr, "%s completion: unsupported shell %q; want one of: %s\n",
				progName, shell, strings.Join(completionShells, ", "))
		}
		return 2
	}

	script := generateCompletion(shell, buildCompletionModel())

	if !o.install {
		_, _ = io.WriteString(stdout, script)
		return 0
	}
	return installCompletion(shell, script, o, stdout, stderr)
}

// splitCompletionArgs partitions args into the shell positional and the flag
// args. Any token starting with "-" (other than a bare "-") is a flag; the first
// remaining token is the shell. A second positional is an error. A "--"
// terminator is ignored.
func splitCompletionArgs(args []string) (shell string, flagArgs []string, err error) {
	for _, a := range args {
		switch {
		case a == "--":
			continue
		case strings.HasPrefix(a, "-") && a != "-":
			flagArgs = append(flagArgs, a)
		case shell == "":
			shell = a
		default:
			return "", nil, fmt.Errorf("unexpected argument %q (only one shell may be given)", a)
		}
	}
	return shell, flagArgs, nil
}

// isSupportedShell reports whether s is one of the shells completion supports.
func isSupportedShell(s string) bool {
	return slices.Contains(completionShells, s)
}

// detectShell derives the shell from $SHELL's basename, returning "" when it is
// unset or not a supported shell.
func detectShell() string {
	base := filepath.Base(os.Getenv("SHELL"))
	if isSupportedShell(base) {
		return base
	}
	return ""
}

// installCompletion writes script to shell's standard completion path. It
// refuses to clobber an existing file without --force, and --dry-run reports the
// target path (as greppable key=value data on stdout) without writing anything.
func installCompletion(shell, script string, o *completionOpts, stdout, stderr io.Writer) int {
	path, err := completionInstallPath(shell)
	if err != nil {
		return reportErr(stderr, err)
	}

	if o.dryRun {
		_, _ = fmt.Fprintf(stdout, "dry-run=true\ncompletion.shell=%s\ncompletion.path=%s\n", shell, path)
		return 0
	}

	switch _, statErr := os.Lstat(path); {
	case statErr == nil && !o.force:
		_, _ = fmt.Fprintf(stderr, "%s completion: %s already exists (use --force to overwrite)\n", progName, path)
		return 1
	case statErr != nil && !os.IsNotExist(statErr):
		return reportErr(stderr, statErr)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return reportErr(stderr, fmt.Errorf("create completion dir: %w", err))
	}
	// Unlink any existing entry first so a symlink at the target is replaced
	// rather than written through (which would clobber the link's target file);
	// this also closes the Lstat->write gap since the write then creates fresh.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return reportErr(stderr, fmt.Errorf("replace completion script: %w", err))
	}
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		return reportErr(stderr, fmt.Errorf("write completion script: %w", err))
	}

	_, _ = fmt.Fprintf(stderr, "%s: installed %s completion to %s\n", progName, shell, path)
	_, _ = fmt.Fprintf(stderr, "%s: %s\n", progName, completionReloadHint(shell, path))
	return 0
}

// completionInstallPath returns the standard, auto-loaded completion path for
// shell: the bash-completion user dir, a zsh $fpath site-functions dir, or fish's
// completions dir — all under the XDG base directories.
func completionInstallPath(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	switch shell {
	case "bash":
		return filepath.Join(xdgDataHome(home), "bash-completion", "completions", progName), nil
	case "zsh":
		return filepath.Join(xdgDataHome(home), "zsh", "site-functions", "_"+progName), nil
	case "fish":
		return filepath.Join(xdgConfigHome(home), "fish", "completions", progName+".fish"), nil
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
}

// completionReloadHint is the one-liner told to the user after install so the
// new completion takes effect without a full relogin. The zsh hint puts the
// site-functions dir on $fpath itself, since on a first install that dir was
// just created and is not yet on the running shell's $fpath — so `compinit`
// alone would not find the new function.
func completionReloadHint(shell, path string) string {
	switch shell {
	case "bash":
		return "restart your shell, or run: source " + path
	case "zsh":
		return "restart your shell, or run: fpath+=(" + filepath.Dir(path) + ") && autoload -U compinit && compinit"
	case "fish":
		return "completions load automatically in new fish sessions"
	default:
		return "restart your shell to load the new completions"
	}
}

// xdgDataHome returns $XDG_DATA_HOME or the ~/.local/share default. Per the XDG
// spec a relative env value is invalid and ignored, so only an absolute path is
// honored (a relative one would resolve against the cwd and no shell would
// auto-load it).
func xdgDataHome(home string) string {
	if v := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(home, ".local", "share")
}

// xdgDataDirs returns the absolute entries of $XDG_DATA_DIRS in order,
// defaulting to the spec's /usr/local/share:/usr/share when unset or when no
// entry is absolute. Per the XDG spec a relative entry is invalid and dropped
// (same rule as xdgDataHome).
func xdgDataDirs() []string {
	var dirs []string
	for _, dir := range strings.Split(os.Getenv("XDG_DATA_DIRS"), string(os.PathListSeparator)) {
		if filepath.IsAbs(dir) {
			dirs = append(dirs, dir)
		}
	}
	if len(dirs) == 0 {
		return []string{"/usr/local/share", "/usr/share"}
	}
	return dirs
}

// xdgConfigHome returns $XDG_CONFIG_HOME or the ~/.config default, ignoring a
// non-absolute env value per the XDG spec (see xdgDataHome).
func xdgConfigHome(home string) string {
	if v := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(home, ".config")
}
