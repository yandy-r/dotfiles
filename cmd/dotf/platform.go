package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// platformOpts holds the parsed flag values for `dotf platform`.
type platformOpts struct {
	env, write bool
}

// setPlatformFlags registers platform's flags on fs and returns the opts
// pointer they parse into. Single source of truth for platform's flag surface,
// shared by runPlatform and the help renderer (DRY — usage strings live only
// here).
func setPlatformFlags(fs *flag.FlagSet) *platformOpts {
	o := &platformOpts{}
	fs.BoolVar(&o.env, "env", false,
		"print the platform as shell export KEY=VALUE lines (the platform env cache format)")
	fs.BoolVar(&o.write, "write", false,
		"atomically write the platform env cache to ${XDG_CACHE_HOME:-~/.cache}/dotfiles/platform.env")
	return o
}

// runPlatform prints the detected platform Info. With no flags it prints
// machine-greppable key=value pairs (a machine-greppable analogue of
// common/platform.sh:print_platform_info). --env prints the platform env
// cache lines instead; --write writes them to the cache file (silently on
// success); both together write and print.
func runPlatform(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	fs := flag.NewFlagSet("platform", flag.ContinueOnError)
	fs.SetOutput(stderr)
	o := setPlatformFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "%s platform: unexpected argument %q\n", progName, fs.Arg(0))
		return 2
	}

	info := platform.Detect(r)
	if !o.env && !o.write {
		if _, err := fmt.Fprintf(stdout, "os=%s arch=%s pkg=%s container=%s distro=%s\n",
			info.OS, info.Arch, info.PkgMgr, info.Container, info.Distro); err != nil {
			return reportErr(stderr, err)
		}
		return 0
	}

	content := renderPlatformEnv(r, info)
	if o.write {
		if err := writePlatformCache(content); err != nil {
			return reportErr(stderr, err)
		}
	}
	if o.env {
		if _, err := stdout.Write(content); err != nil {
			return reportErr(stderr, err)
		}
	}
	return 0
}

// renderPlatformEnv renders info as the platform env cache file content for
// this host.
func renderPlatformEnv(r exec.Runner, info platform.Info) []byte {
	// An unresolvable hostname renders as empty; the shell loader then ignores
	// the cache (host mismatch) and falls back to its own detection.
	host, _ := os.Hostname()
	var buf bytes.Buffer
	_ = platform.WriteEnv(&buf, info, host, platform.HomebrewPrefix(r, info))
	return buf.Bytes()
}

// writePlatformCache writes content to the platform env cache path.
func writePlatformCache(content []byte) error {
	home, _ := os.UserHomeDir()
	path, err := platform.CachePath(os.Getenv, home)
	if err != nil {
		return err
	}
	return platform.WriteEnvCache(path, content)
}

// refreshPlatformCache rewrites the platform env cache from an
// already-detected info after a successful apply/link run. A failure is a
// single stderr warning and never changes the caller's exit code.
func refreshPlatformCache(stderr io.Writer, r exec.Runner, info platform.Info) {
	if err := writePlatformCache(renderPlatformEnv(r, info)); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: warning: %v\n", progName, err)
	}
}
