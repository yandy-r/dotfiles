package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/yandy-r/dotfiles/internal/symlink"
)

// This file is `dotf unlink`: removing managed symlinks from $HOME and,
// optionally, restoring the newest backed-up copy in their place. All
// filesystem work and every safety check lives in internal/symlink
// (Engine.Unlink, which reuses Engine.Restore for --restore); this file owns
// the flag surface, the usage checks, and the output.

// unlinkOpts holds the parsed flag values for `dotf unlink`.
type unlinkOpts struct {
	all, restore, dryRun bool
}

// setUnlinkFlags registers unlink's flags on fs and returns the opts pointer
// they parse into. Single source of truth for unlink's flag surface, shared by
// runUnlink, the help renderer, and the completion generator.
func setUnlinkFlags(fs *flag.FlagSet) *unlinkOpts {
	o := &unlinkOpts{}
	fs.BoolVar(&o.all, "all", false, "unlink every managed link in the link table (all facets)")
	fs.BoolVar(&o.restore, "restore", false,
		"restore each unlinked dest from the newest backup holding it (dests with no backup stay unlinked)")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print what would be unlinked/restored without changing anything")
	return o
}

// runUnlink removes the named managed symlinks (or every table entry with
// --all). Only a symlink pointing into the dotfiles repo is removed; real
// files, foreign symlinks, missing dests and copy entries are skipped and
// reported. Exit codes: 0 ok, 1 runtime failure, 2 usage error or unknown
// dest (rejected before any change).
func runUnlink(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("unlink", flag.ContinueOnError)
	fs.SetOutput(stderr)
	o := setUnlinkFlags(fs)

	// Loop-parse so flags may appear before or after the dest operands (Go's
	// flag stops at the first non-flag token).
	var dests []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		dests = append(dests, fs.Arg(0))
		rest = fs.Args()[1:]
	}

	switch {
	case len(dests) == 0 && !o.all:
		_, _ = fmt.Fprintf(stderr, "%s: unlink requires at least one dest or --all\n", progName)
		return 2
	case len(dests) > 0 && o.all:
		_, _ = fmt.Fprintf(stderr, "%s: unlink: --all cannot be combined with named dests\n", progName)
		return 2
	}

	repoRoot := resolveRepoRoot()
	if repoRoot == "" {
		return reportErr(stderr, errors.New("could not locate the dotfiles repo root (set DOTFILES_ROOT)"))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return reportErr(stderr, fmt.Errorf("resolve home dir: %w", err))
	}

	eng := symlink.NewEngine(repoRoot, home)
	report, err := eng.Unlink(symlink.DefaultTable(), symlink.UnlinkOptions{
		Dests:   dests,
		All:     o.all,
		Restore: o.restore,
		DryRun:  o.dryRun,
	})
	// Print whatever was done before a mid-run failure, so the user sees which
	// links are already gone.
	if report != nil {
		if werr := printUnlinkReport(stdout, stderr, report, o.restore, o.all); werr != nil {
			return reportErr(stderr, werr)
		}
	}
	if err != nil {
		var ude *symlink.UnknownDestError
		if errors.As(err, &ude) {
			_, _ = fmt.Fprintf(stderr, "%s: unlink: %v\n", progName, ude)
			return 2
		}
		return reportErr(stderr, err)
	}
	return 0
}

// printUnlinkReport writes the greppable unlink.remove/skip/restore lines
// (sorted dests, empty buckets omitted, `dry-run=true` header on a preview) to
// stdout in one checked write, and the per-item skip reasons, the no-backup
// and restore-not-performed notices, and the summary line to stderr. The
// unlink.restore bucket lists the items whose restore completed (or, on a
// preview, is planned). Under --all a missing dest (the normal case for
// another host's facets) and the copy entry (never a link) are expected, so
// they are counted in the summary's skipped total but not listed.
func printUnlinkReport(stdout, stderr io.Writer, report *symlink.UnlinkReport, restore, all bool) error {
	var removed, skipped, restored []string
	quiet := 0
	for _, it := range report.Items {
		switch it.Action {
		case symlink.UnlinkActionRemove:
			removed = append(removed, it.Dest)
			switch {
			case !restore:
			case it.Restored:
				restored = append(restored, it.Dest)
			case it.BackupID == "":
				_, _ = fmt.Fprintf(stderr, "%s: unlink: no backup for %s; left unlinked\n", progName, it.Dest)
			default:
				_, _ = fmt.Fprintf(stderr,
					"%s: unlink: restore of %s from backup %s not performed; left unlinked (retry: %s backup restore %s %s)\n",
					progName, it.Dest, it.BackupID, progName, it.BackupID, it.Dest)
			}
		case symlink.UnlinkActionSkip:
			if all && (it.Reason == "missing" || it.Reason == "copy entry") {
				quiet++
				continue
			}
			skipped = append(skipped, it.Dest)
			_, _ = fmt.Fprintf(stderr, "%s: unlink: skip %s: %s\n", progName, it.Dest, it.Reason)
		}
	}

	var lines []string
	if report.DryRun {
		lines = append(lines, "dry-run=true")
	}
	for _, b := range []struct {
		key   string
		dests []string
	}{
		{"unlink.remove", removed},
		{"unlink.skip", skipped},
		{"unlink.restore", restored},
	} {
		if len(b.dests) == 0 {
			continue
		}
		sorted := append([]string(nil), b.dests...)
		sort.Strings(sorted)
		lines = append(lines, b.key+"="+strings.Join(sorted, ","))
	}
	if len(lines) > 0 {
		if _, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n"); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(stderr, "%s: unlink summary: removed=%d skipped=%d restored=%d\n",
		progName, len(removed), len(skipped)+quiet, len(restored))
	return nil
}
