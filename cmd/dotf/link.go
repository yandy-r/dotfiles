package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/symlink"
)

// runLink resolves the dotfiles repo root and $HOME, detects the host
// platform, and provisions the static symlink table (symlink.DefaultTable)
// into $HOME via the symlink engine's Plan/Apply/Validate split. Symlinks live
// under $HOME, so the operation is unprivileged: the injected exec.Runner is
// used ONLY to feed platform.Detect; all filesystem work is direct os.* calls
// inside internal/symlink. It writes machine-greppable data to stdout and
// diagnostics to stderr, returning exit code 1 when post-apply validation
// finds a broken link (a real file where a symlink was expected, or a dangling
// or looping link whose source exists).
//
// There is deliberately no --manifest flag: the engine is driven by the static
// DefaultTable (ported 1:1 from playbooks/vars/symlinks.yml), not m.Symlinks
// (which is only a 4-entry partial mirror). A --manifest flag would be dead
// here and mislead — see docs/prps/plans/phase-4-symlink-engine.plan.md Data
// Model.
func runLink(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	fs.SetOutput(stderr)

	o := setLinkFlags(fs)

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if o.backup && o.noBackup {
		_, _ = fmt.Fprintf(stderr, "%s: link: --backup cannot be combined with --no-backup\n", progName)
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

	info := platform.Detect(r)
	// Same scope as the engine's LinuxDesktopGuardError, but reported as a
	// usage error: the flags contradict each other where the desktop set applies.
	if (o.hyprland || o.linuxDesktop) && o.noBackup && !o.dryRun && symlink.DesktopGuardApplies(info.OS) {
		_, _ = fmt.Fprintf(stderr, "%s: link: --hyprland cannot be combined with --no-backup (use --dry-run to preview)\n", progName)
		return 2
	}
	opts := symlink.Options{
		DryRun:         o.dryRun,
		Backup:         !o.noBackup,
		Force:          o.force,
		Validate:       !o.noValidate,
		ManageHyprland: o.hyprland || o.linuxDesktop,
	}

	eng := symlink.NewEngine(repoRoot, home)
	specs := symlink.DefaultTable()

	plan, err := eng.Plan(specs, info, opts)
	if err != nil {
		return reportErr(stderr, err)
	}

	if o.dryRun {
		return linkDryRun(stdout, stderr, eng, specs, info, opts, plan)
	}

	report, err := eng.Apply(plan, opts)
	// Name the backup dir even when Apply fails partway: real files may already
	// be removed, and their copies are exactly what the user needs to find.
	printLinkBackupDir(stderr, report)
	if err != nil {
		return reportErr(stderr, err)
	}
	if err := printLinkResults(stdout, report.Applied); err != nil {
		return reportErr(stderr, err)
	}
	printLinkSummary(stderr, report)

	if !opts.Validate {
		return 0
	}
	return linkValidate(stderr, eng, specs, info, opts)
}

// linkDryRun prints the planned actions and runs Validate for reporting only.
// It ALWAYS returns 0 — a deliberate divergence from Ansible, whose validation
// fail task fires even in check mode. This port treats --dry-run as a pure,
// non-failing preview: broken dests are surfaced on stdout as `link.broken=`
// rather than through a non-zero exit, so a preview never signals drift via the
// exit code (the real apply path still exits 1 on a broken link).
func linkDryRun(stdout, stderr io.Writer, eng *symlink.Engine, specs []symlink.LinkSpec, info platform.Info, opts symlink.Options, plan *symlink.Plan) int {
	broken := []string(nil)
	if opts.Validate {
		if _, verr := eng.Validate(specs, info, opts); verr != nil {
			var ble *symlink.BrokenLinkError
			if errors.As(verr, &ble) {
				broken = ble.Dests
			} else {
				return reportErr(stderr, verr)
			}
		}
	}

	if err := printLinkDryRun(stdout, plan, broken); err != nil {
		return reportErr(stderr, err)
	}
	return 0
}

// linkValidate runs post-apply validation, printing any broken dests to stderr
// and returning exit code 1 when validation fails (the not-converged signal).
func linkValidate(stderr io.Writer, eng *symlink.Engine, specs []symlink.LinkSpec, info platform.Info, opts symlink.Options) int {
	_, verr := eng.Validate(specs, info, opts)
	if verr == nil {
		return 0
	}
	var ble *symlink.BrokenLinkError
	if errors.As(verr, &ble) {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, ble)
		return 1
	}
	return reportErr(stderr, verr)
}

// linkOpts holds the parsed flag values for `dotf link`. hyprland and
// linuxDesktop are OR'd in runLink: --linux-desktop is the deprecated alias.
// Backups are on by default: backup is the explicit (redundant) spelling of
// the default, kept so existing scripts keep working, and noBackup opts out.
type linkOpts struct {
	dryRun, backup, noBackup, force, noValidate, hyprland, linuxDesktop bool
}

// setLinkFlags registers link's flags on fs and returns the opts pointer they
// parse into. Single source of truth for link's flag surface, shared by
// runLink and the help renderer (DRY — usage strings live only here).
func setLinkFlags(fs *flag.FlagSet) *linkOpts {
	o := &linkOpts{}
	fs.BoolVar(&o.dryRun, "dry-run", false, "print the plan without executing")
	fs.BoolVar(&o.backup, "backup", false, "back up real files before replacing them (the default; --backup=false does not disable backups, use --no-backup)")
	fs.BoolVar(&o.noBackup, "no-backup", false, "replace real files without backing them up first")
	fs.BoolVar(&o.force, "force", false, "force symlink recreation (parity; cleanup owns real-file removal)")
	fs.BoolVar(&o.noValidate, "no-validate", false, "skip post-apply validation")
	fs.BoolVar(&o.hyprland, "hyprland", false, "manage the opt-in Hyprland/wlroots desktop config set")
	fs.BoolVar(&o.linuxDesktop, "linux-desktop", false, "deprecated alias for --hyprland")
	return o
}

// actionBucket classifies an Action into a greppable output bucket keyed by
// the operation the engine will perform.
func actionBucket(a symlink.Action) string {
	switch a.Kind {
	case symlink.ActionLink:
		if a.Reason == "create" {
			return "create"
		}
		return "repoint"
	case symlink.ActionBackup:
		return "backup"
	case symlink.ActionRemove:
		return "remove"
	case symlink.ActionCopy:
		return "copy"
	case symlink.ActionSkip:
		return "skip"
	default:
		return "unknown"
	}
}

// linkActionLines groups actions into buckets keyed by operation and returns
// one sorted "prefix.bucket=dest,dest,..." line per non-empty bucket, in a
// fixed bucket order for deterministic output.
func linkActionLines(prefix string, actions []symlink.Action) []string {
	buckets := make(map[string][]string)
	for _, a := range actions {
		bucket := actionBucket(a)
		buckets[bucket] = append(buckets[bucket], a.Spec.Dest)
	}

	order := []string{"create", "repoint", "backup", "remove", "copy", "skip", "unknown"}
	lines := make([]string, 0, len(order))
	for _, bucket := range order {
		dests := buckets[bucket]
		if len(dests) == 0 {
			continue
		}
		sort.Strings(dests)
		lines = append(lines, fmt.Sprintf("%s.%s=%s", prefix, bucket, strings.Join(dests, ",")))
	}
	return lines
}

// printLinkDryRun writes the dry-run preview to stdout: a literal dry-run=true
// header, the per-bucket planned actions, and (when validation found any) the
// currently-broken dests. The whole preview is emitted with a single checked
// write so a broken stdout surfaces rather than being silently dropped.
func printLinkDryRun(stdout io.Writer, plan *symlink.Plan, broken []string) error {
	lines := []string{"dry-run=true"}
	lines = append(lines, linkActionLines("link", plan.Actions)...)
	if len(broken) > 0 {
		b := append([]string(nil), broken...)
		sort.Strings(b)
		lines = append(lines, "link.broken="+strings.Join(b, ","))
	}

	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return err
}

// printLinkResults writes the applied actions as greppable per-bucket lines to
// stdout. It writes nothing when no action was applied.
func printLinkResults(stdout io.Writer, applied []symlink.Action) error {
	lines := linkActionLines("link", applied)
	if len(lines) == 0 {
		return nil
	}
	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return err
}

// printLinkSummary writes an aggregate applied/planned count to stderr,
// mirroring printInstallSummary's stderr diagnostics style.
func printLinkSummary(stderr io.Writer, report *symlink.Report) {
	_, _ = fmt.Fprintf(stderr, "%s: link summary: applied=%d planned=%d\n",
		progName, len(report.Applied), len(report.Planned))
}

// printLinkBackupDir names the backup dir on stderr when the run copied at
// least one real file into it, so the recoverable copies are discoverable.
func printLinkBackupDir(stderr io.Writer, report *symlink.Report) {
	if report == nil || report.BackupDir == "" {
		return
	}
	_, _ = fmt.Fprintf(stderr, "%s: link backups: %s\n", progName, report.BackupDir)
}
