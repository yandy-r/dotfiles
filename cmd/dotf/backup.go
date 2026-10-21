package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/yandy-r/dotfiles/internal/symlink"
)

// This file is `dotf backup`: listing, restoring, and pruning the
// ~/.config/dotfiles_backup_<epoch>[_N] dirs that link/apply/setup create when
// they replace real files. All filesystem work lives in internal/symlink
// (ListBackups, Engine.Restore, SelectPrune, RemoveBackup); this file owns the
// flag surface, the verb parse, the prune confirmation, and the output.

// backupVerbs is the ordered, closed set of `dotf backup` verbs. It is the
// single source of truth for the dispatch switch, the usage-error messages, and
// (via completionBackupVerbs) the generated shell completions.
var backupVerbs = []string{"list", "restore", "prune"}

// backupPrunePrompt is the interactive gate on `dotf backup prune`, written to
// stdout like the plasma/deb prompts (stderr under --json, keeping stdout pure
// JSON).
const backupPrunePrompt = "delete these backup dirs? [y/N] "

// backupOpts holds the parsed flag values for `dotf backup`.
type backupOpts struct {
	jsonOut   bool
	dryRun    bool
	force     bool
	yes       bool
	keep      int
	olderThan string
}

// setBackupFlags registers backup's flags on fs and returns the opts pointer
// they parse into. Single source of truth for backup's flag surface, shared by
// runBackup, the help renderer, and the completion generator. One flagset serves
// all three verbs; backupFlagVerbs rejects a flag given to a verb it does not
// apply to, since silently ignoring --force or --yes would mislead.
func setBackupFlags(fs *flag.FlagSet) *backupOpts {
	o := &backupOpts{}
	fs.BoolVar(&o.jsonOut, "json", false, jsonFlagUsage)
	fs.BoolVar(&o.dryRun, "dry-run", false, "print what restore/prune would do without writing (restore, prune)")
	fs.BoolVar(&o.force, "force", false,
		"restore over real files, backing them up to a new backup dir first (restore)")
	fs.BoolVar(&o.yes, "yes", false, "prune without an interactive confirmation prompt (prune)")
	fs.IntVar(&o.keep, "keep", symlink.DefaultBackupKeep, "number of newest backup dirs to keep (prune)")
	fs.StringVar(&o.olderThan, "older-than", "",
		"only prune backups older than this age, e.g. 720h or 30d (prune)")
	return o
}

// backupFlagVerbs maps each verb-specific flag to the verbs that accept it.
// --json is accepted by every verb and is absent here.
var backupFlagVerbs = map[string][]string{
	"dry-run":    {"restore", "prune"},
	"force":      {"restore"},
	"yes":        {"prune"},
	"keep":       {"prune"},
	"older-than": {"prune"},
}

// backupInteractive reports whether the prune confirmation can be asked: both
// stdin and the prompt writer (stdout, or stderr under --json) must be
// terminals. A package var so tests can drive the prompt with an injected
// stdin.
var backupInteractive = func(stdin io.Reader, promptOut io.Writer) bool {
	f, ok := stdin.(*os.File)
	return ok && isTTY(f) && stdoutIsTTY(promptOut)
}

// runBackup lists, restores, and prunes symlink backup dirs. Exit codes: 0 ok,
// 1 runtime failure or refusal (restore conflict, legacy dir, non-interactive
// prune without --yes, declined prompt), 2 usage error, 130 on a Ctrl+C at the
// prune confirmation.
func runBackup(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	o := setBackupFlags(fs)

	// Loop-parse so flags may appear before or after the verb and its
	// positionals (Go's flag stops at the first non-flag token).
	var positionals []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		positionals = append(positionals, fs.Arg(0))
		rest = fs.Args()[1:]
	}

	if len(positionals) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: backup requires a verb; available: %s\n", progName, strings.Join(backupVerbs, ", "))
		return 2
	}
	verb, operands := positionals[0], positionals[1:]
	if !slices.Contains(backupVerbs, verb) {
		_, _ = fmt.Fprintf(stderr, "%s: unknown verb %q; available: %s\n", progName, verb, strings.Join(backupVerbs, ", "))
		return 2
	}
	if !backupCheckFlags(fs, verb, stderr) {
		return 2
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return reportErr(stderr, fmt.Errorf("resolve home dir: %w", err))
	}

	switch verb {
	case "list":
		if len(operands) > 0 {
			_, _ = fmt.Fprintf(stderr, "%s: backup list takes no arguments, got %d\n", progName, len(operands))
			return 2
		}
		return backupList(stdout, stderr, home, o)
	case "restore":
		if len(operands) == 0 {
			_, _ = fmt.Fprintf(stderr, "%s: backup restore requires a backup id (see dotf backup list)\n", progName)
			return 2
		}
		return backupRestore(stdout, stderr, home, operands[0], operands[1:], o)
	default:
		if len(operands) > 0 {
			_, _ = fmt.Fprintf(stderr, "%s: backup prune takes no arguments, got %d\n", progName, len(operands))
			return 2
		}
		return backupPrune(stdout, stderr, stdin, home, o)
	}
}

// backupCheckFlags rejects a verb-specific flag explicitly set on a verb it
// does not apply to.
func backupCheckFlags(fs *flag.FlagSet, verb string, stderr io.Writer) bool {
	ok := true
	fs.Visit(func(f *flag.Flag) {
		verbs, scoped := backupFlagVerbs[f.Name]
		if ok && scoped && !slices.Contains(verbs, verb) {
			_, _ = fmt.Fprintf(stderr, "%s: --%s does not apply to \"dotf backup %s\"\n", progName, f.Name, verb)
			ok = false
		}
	})
	return ok
}

// ----------------------------------------------------------------------------
// list
// ----------------------------------------------------------------------------

// backupSetJSON is one backup dir in `dotf backup list --json`.
type backupSetJSON struct {
	ID         string                `json:"id"`
	Path       string                `json:"path"`
	CreatedAt  time.Time             `json:"created_at"`
	Legacy     bool                  `json:"legacy"`
	EntryCount int                   `json:"entry_count"`
	Size       int64                 `json:"size"`
	Entries    []symlink.BackupEntry `json:"entries,omitempty"`
}

// backupListJSON is the `dotf backup list --json` document.
type backupListJSON struct {
	Command string          `json:"command"` // always "backup"
	Verb    string          `json:"verb"`    // always "list"
	Backups []backupSetJSON `json:"backups"`
}

func toBackupSetJSON(s symlink.BackupSet) backupSetJSON {
	return backupSetJSON{
		ID: s.ID, Path: s.Path, CreatedAt: s.CreatedAt, Legacy: s.Legacy,
		EntryCount: len(s.Entries), Size: s.Size, Entries: s.Entries,
	}
}

// backupList prints every backup dir, newest first.
func backupList(stdout, stderr io.Writer, home string, o *backupOpts) int {
	sets, err := symlink.ListBackups(home)
	if err != nil {
		return reportErr(stderr, err)
	}
	if o.jsonOut {
		doc := backupListJSON{Command: "backup", Verb: "list", Backups: []backupSetJSON{}}
		for _, s := range sets {
			doc.Backups = append(doc.Backups, toBackupSetJSON(s))
		}
		if err := writeJSON(stdout, doc); err != nil {
			return reportErr(stderr, err)
		}
		return 0
	}

	var b strings.Builder
	var total int64
	for _, s := range sets {
		total += s.Size
		fmt.Fprintf(&b, "backup.id=%s created=%s entries=%d size=%d legacy=%t path=%s\n",
			s.ID, s.CreatedAt.UTC().Format(time.RFC3339), len(s.Entries), s.Size, s.Legacy, s.Path)
	}
	fmt.Fprintf(&b, "backup.count=%d backup.size=%d\n", len(sets), total)
	if _, err := io.WriteString(stdout, b.String()); err != nil {
		return reportErr(stderr, err)
	}
	_, _ = fmt.Fprintf(stderr, "%s: %s\n", progName, backupSummary(len(sets), total))
	return 0
}

// backupSummary renders "N backup(s), <size>" for list and doctor.
func backupSummary(count int, size int64) string {
	noun := "backups"
	if count == 1 {
		noun = "backup"
	}
	return fmt.Sprintf("%d %s, %s", count, noun, humanBytes(size))
}

// humanBytes formats n with binary (IEC) units: "512 B", "3.4 MiB".
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ----------------------------------------------------------------------------
// restore
// ----------------------------------------------------------------------------

// backupRestoreItemJSON is one restored (or planned) entry.
type backupRestoreItemJSON struct {
	Dest   string `json:"dest"`
	Kind   string `json:"kind"`
	Action string `json:"action"`
}

// backupRestoreJSON is the `dotf backup restore --json` document.
type backupRestoreJSON struct {
	Command         string                  `json:"command"` // always "backup"
	Verb            string                  `json:"verb"`    // always "restore"
	ID              string                  `json:"id"`
	DryRun          bool                    `json:"dry_run"`
	Items           []backupRestoreItemJSON `json:"items"`
	SafetyBackupDir string                  `json:"safety_backup_dir,omitempty"`
	Error           string                  `json:"error,omitempty"`
}

// backupRestore copies a backup set's entries (or the named subset) back into
// $HOME. The plan is printed even when the engine refuses on a conflict, so the
// user sees which dests block the restore. The engine is rooted at the dotfiles
// repo when it can be located, so a dest resolving inside the repo is refused.
func backupRestore(stdout, stderr io.Writer, home, id string, paths []string, o *backupOpts) int {
	eng := symlink.NewEngine(resolveRepoRoot(), home)
	report, err := eng.Restore(id, symlink.RestoreOptions{Paths: paths, DryRun: o.dryRun, Force: o.force})

	if o.jsonOut {
		doc := backupRestoreJSON{Command: "backup", Verb: "restore", ID: id, DryRun: o.dryRun,
			Items: []backupRestoreItemJSON{}}
		if report != nil {
			for _, it := range report.Items {
				doc.Items = append(doc.Items, backupRestoreItemJSON{Dest: it.Dest, Kind: it.Kind, Action: it.Action})
			}
			doc.SafetyBackupDir = report.SafetyBackupDir
		}
		if err != nil {
			doc.Error = err.Error()
		}
		if werr := writeJSON(stdout, doc); werr != nil {
			return reportErr(stderr, werr)
		}
		if err != nil {
			return reportErr(stderr, err)
		}
		return 0
	}

	if report != nil {
		var b strings.Builder
		if report.DryRun {
			b.WriteString("dry-run=true\n")
		}
		for _, it := range report.Items {
			fmt.Fprintf(&b, "restore.%s=%s kind=%s\n", it.Action, it.Dest, it.Kind)
		}
		if report.SafetyBackupDir != "" {
			fmt.Fprintf(&b, "restore.safety_backup=%s\n", report.SafetyBackupDir)
		}
		if _, werr := io.WriteString(stdout, b.String()); werr != nil {
			return reportErr(stderr, werr)
		}
		if report.SafetyBackupDir != "" {
			_, _ = fmt.Fprintf(stderr, "%s: replaced content backed up to %s\n", progName, report.SafetyBackupDir)
		}
	}
	if err != nil {
		return reportErr(stderr, err)
	}
	if !o.dryRun {
		_, _ = fmt.Fprintf(stderr, "%s: restored %d entr%s from backup %s\n",
			progName, len(report.Items), pluralY(len(report.Items)), id)
	}
	return 0
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// ----------------------------------------------------------------------------
// prune
// ----------------------------------------------------------------------------

// backupPruneJSON is the `dotf backup prune --json` document.
type backupPruneJSON struct {
	Command string          `json:"command"` // always "backup"
	Verb    string          `json:"verb"`    // always "prune"
	DryRun  bool            `json:"dry_run"`
	Kept    int             `json:"kept"`
	Removed []backupSetJSON `json:"removed"`
	Error   string          `json:"error,omitempty"`
}

// olderThanDaysRE matches the "<N>d" day shorthand --older-than accepts on top
// of Go durations.
var olderThanDaysRE = regexp.MustCompile(`^([0-9]+)d$`)

// parseOlderThan parses --older-than: "" (no filter), "<N>d", or a Go duration.
// Negative durations and day counts overflowing time.Duration are rejected.
func parseOlderThan(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	if m := olderThanDaysRE.FindStringSubmatch(s); m != nil {
		days, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid --older-than %q: %w", s, err)
		}
		if days < 0 || days > math.MaxInt64/int64(24*time.Hour) {
			return 0, fmt.Errorf("invalid --older-than %q: day count out of range", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid --older-than %q: want a duration like 720h or 30d", s)
	}
	if d < 0 {
		return 0, fmt.Errorf("invalid --older-than %q: must not be negative", s)
	}
	return d, nil
}

// backupPrune deletes the backup dirs beyond --keep (intersected with
// --older-than), after confirmation unless --yes or --dry-run.
func backupPrune(stdout, stderr io.Writer, stdin io.Reader, home string, o *backupOpts) int {
	if o.keep < 0 {
		_, _ = fmt.Fprintf(stderr, "%s: --keep must not be negative, got %d\n", progName, o.keep)
		return 2
	}
	olderThan, err := parseOlderThan(o.olderThan)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
		return 2
	}

	sets, err := symlink.ListBackups(home)
	if err != nil {
		return reportErr(stderr, err)
	}
	victims := symlink.SelectPrune(sets, symlink.PruneOptions{Keep: o.keep, OlderThan: olderThan, Now: time.Now()})
	kept := len(sets) - len(victims)

	if o.dryRun || len(victims) == 0 {
		return backupPrintPrune(stdout, stderr, o, victims, kept, nil)
	}

	if code, ok := backupConfirmPrune(stdout, stderr, stdin, o, victims); !ok {
		return code
	}

	var removed []symlink.BackupSet
	var errs []error
	for _, s := range victims {
		if rerr := symlink.RemoveBackup(home, s); rerr != nil {
			errs = append(errs, rerr)
			kept++
			continue
		}
		removed = append(removed, s)
	}
	return backupPrintPrune(stdout, stderr, o, removed, kept, errors.Join(errs...))
}

// backupConfirmPrune gates the deletion. It returns ok=false with the exit code
// to use for a refusal, a decline, or an abort.
func backupConfirmPrune(stdout, stderr io.Writer, stdin io.Reader, o *backupOpts, victims []symlink.BackupSet) (int, bool) {
	if o.yes {
		return 0, true
	}
	// Under --json stdout carries only the JSON document, so the victim list
	// and the prompt go to stderr.
	promptOut := stdout
	if o.jsonOut {
		promptOut = stderr
	}
	if !backupInteractive(stdin, promptOut) {
		_, _ = fmt.Fprintf(stderr, "%s: refusing to prune %d backup dir(s) without --yes (non-interactive)\n",
			progName, len(victims))
		return 1, false
	}
	for _, s := range victims {
		_, _ = fmt.Fprintf(promptOut, "  %s  %s  %s\n", s.ID, s.CreatedAt.Local().Format(time.RFC3339), humanBytes(s.Size))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	switch confirmPromptCtx(ctx, promptOut, backupPrunePrompt, stdin) {
	case confirmAborted:
		return reportAbort(stderr), false
	case confirmNo:
		_, _ = fmt.Fprintf(stderr, "%s: prune not confirmed; nothing deleted\n", progName)
		return 1, false
	default:
		return 0, true
	}
}

// backupPrintPrune writes the prune outcome (or dry-run preview) and maps a
// removal error to exit 1.
func backupPrintPrune(stdout, stderr io.Writer, o *backupOpts, sets []symlink.BackupSet, kept int, err error) int {
	if o.jsonOut {
		doc := backupPruneJSON{Command: "backup", Verb: "prune", DryRun: o.dryRun, Kept: kept, Removed: []backupSetJSON{}}
		for _, s := range sets {
			doc.Removed = append(doc.Removed, toBackupSetJSON(s))
		}
		if err != nil {
			doc.Error = err.Error()
		}
		if werr := writeJSON(stdout, doc); werr != nil {
			return reportErr(stderr, werr)
		}
	} else {
		var b strings.Builder
		if o.dryRun {
			b.WriteString("dry-run=true\n")
		}
		var size int64
		for _, s := range sets {
			size += s.Size
			fmt.Fprintf(&b, "prune.remove=%s path=%s\n", s.ID, s.Path)
		}
		fmt.Fprintf(&b, "prune.removed=%d prune.kept=%d\n", len(sets), kept)
		if _, werr := io.WriteString(stdout, b.String()); werr != nil {
			return reportErr(stderr, werr)
		}
		verbPhrase := "removed"
		if o.dryRun {
			verbPhrase = "would remove"
		}
		_, _ = fmt.Fprintf(stderr, "%s: %s %s; kept %d\n", progName, verbPhrase, backupSummary(len(sets), size), kept)
	}
	if err != nil {
		return reportErr(stderr, err)
	}
	return 0
}
