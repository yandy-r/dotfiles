package symlink

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/yandy-r/dotfiles/internal/platform"
)

// Options are the CLI knobs, mapped 1:1 from playbooks/vars/symlinks.yml config
// vars.
type Options struct {
	// DryRun disables all filesystem mutations.
	DryRun bool
	// Backup copies every real file or directory the plan replaces into
	// BackupDir before removing it. The CLIs default it to true (--no-backup
	// opts out); false removes real files without a copy.
	Backup bool
	// Force is inert in practice because cleanup (ActionRemove) owns real-file
	// removal.
	Force bool
	// Validate runs post-apply validation (default true).
	Validate bool
	// ManageHyprland opts into the Hyprland/wlroots desktop config set. The CLI
	// spells it --hyprland, with --linux-desktop retained as a back-compat alias.
	ManageHyprland bool
	// BackupDir, when non-empty, is an existing backup dir (an earlier
	// Apply's Report.BackupDir in the same run) that Apply reuses instead of
	// creating a new one, merging its entries into that dir's manifest. Empty
	// keeps the default lazy, exclusive per-Apply dir.
	BackupDir string
}

// ActionKind classifies a planned filesystem operation.
type ActionKind int

// Action kinds produced by Plan.
const (
	// ActionLink creates or repoints a symlink Dest -> Src.
	ActionLink ActionKind = iota
	// ActionBackup copies a real file to the backup dir before cleanup.
	ActionBackup
	// ActionRemove removes a real file/dir at Dest (recursive for dirs).
	ActionRemove
	// ActionCopy is the Zed exception: copy Src -> Dest (overwrite, 0644),
	// first unlinking a legacy symlink at Dest. Plan emits it only when Dest
	// is missing, a symlink, or differs from Src; an identical Dest is an
	// ActionSkip, and a differing real file is preceded by an ActionBackup
	// when backups are on.
	ActionCopy
	// ActionSkip is a no-op: the source is missing or Dest is already correct.
	ActionSkip
)

// Action is a single planned operation with absolute, resolved paths.
type Action struct {
	// Spec is the LinkSpec this action was derived from.
	Spec LinkSpec
	// Kind is the operation to perform.
	Kind ActionKind
	// SrcAbs is filepath.Join(repoRoot, Spec.Src).
	SrcAbs string
	// DestAbs is filepath.Join(home, Spec.Dest).
	DestAbs string
	// Reason is a human-readable explanation of the action.
	Reason string
}

// Plan is the computed set of actions plus the resolved run context. It holds
// no filesystem writes — Apply performs them.
type Plan struct {
	// Actions are in phase-stable order: Backup, Remove, Link, Copy, Skip.
	Actions []Action
	// BackupDir is home/.config/dotfiles_backup_<epoch>, captured once. Apply
	// creates it only when the plan contains an ActionBackup, switching to a
	// _N-suffixed sibling (and updating this field) if the dir already exists.
	BackupDir string
	// rootReal is the symlink-resolved repo root Plan checked containment
	// against; Apply re-checks it before each mutation (empty disables it).
	rootReal string
}

// ValidationResult is one post-apply check over a managed dest.
type ValidationResult struct {
	// Dest is the $HOME-relative destination that was checked.
	Dest string
	// Valid is true when Dest is a symlink that resolves, or a path that
	// already is its source through a symlinked parent dir. A dangling
	// symlink is never Valid.
	Valid bool
	// Broken is true when Dest is a real file where a symlink was expected,
	// or a dangling or looping symlink whose source exists (the validation
	// fail condition).
	Broken bool
}

// Report is the outcome of Apply. Post-apply validation is exposed separately
// via Engine.Validate, whose []ValidationResult the CLI consumes directly.
type Report struct {
	// Applied lists actions actually performed (empty under DryRun).
	Applied []Action
	// Planned is the full plan (always populated).
	Planned []Action
	// DryRun records whether the run was a dry run.
	DryRun bool
	// BackupDir is the plan's backup dir when Apply created it (at least one
	// ActionBackup was planned and the run was not a dry run), else empty.
	BackupDir string
}

// Engine plans, applies, and validates the symlink table against a dotfiles
// repo root and a $HOME directory.
type Engine struct {
	repoRoot string
	home     string
}

// NewEngine constructs an Engine rooted at repoRoot (the dotfiles source tree)
// linking into home ($HOME). repoRoot is made absolute (and cleaned) so link
// targets resolve from $HOME regardless of the caller's working directory; it
// is deliberately not symlink-resolved, so targets keep the user's spelling
// and existing links stay "already correct". An empty repoRoot stays empty
// (Abs would turn it into the working directory), so Plan still fails with a
// MissingSourceTreeError. If the working directory cannot be determined,
// repoRoot is kept as given.
func NewEngine(repoRoot, home string) *Engine {
	if repoRoot != "" {
		if abs, err := filepath.Abs(repoRoot); err == nil {
			repoRoot = abs
		}
	}
	return &Engine{repoRoot: repoRoot, home: home}
}

// MissingSourceTreeError is returned when the dotfiles source tree does not exist.
type MissingSourceTreeError struct {
	// Root is the missing repo root path.
	Root string
}

// Error implements the error interface.
func (e *MissingSourceTreeError) Error() string {
	return fmt.Sprintf("symlink: dotfiles source tree %q does not exist", e.Root)
}

// LinuxDesktopGuardError is returned when the Linux-desktop set is requested on
// a Linux-family host with neither dry-run nor backups enabled (the safety gate
// that prevents clobbering real ~/.config desktop directories).
type LinuxDesktopGuardError struct{}

// Error implements the error interface.
func (e *LinuxDesktopGuardError) Error() string {
	return "symlink: managing the Linux desktop config set requires backups; drop --no-backup or use --dry-run"
}

// DesktopGuardApplies reports whether LinuxDesktopGuardError can fire on a host
// running os: the Linux family (WSL included), where the Hyprland set applies.
// CLIs use it to reject --hyprland with --no-backup as a usage error before
// doing any work, with exactly the engine guard's scope.
func DesktopGuardApplies(os platform.OS) bool { return isLinuxFamily(os) }

// UnsafeDestError is returned when a LinkSpec dest is absolute or contains a
// parent (..) segment, guarding filepath.Join(home, dest) against escaping $HOME.
type UnsafeDestError struct {
	// Dest is the offending destination path.
	Dest string
}

// Error implements the error interface.
func (e *UnsafeDestError) Error() string {
	return fmt.Sprintf("symlink: unsafe dest %q must be relative to $HOME and free of ..", e.Dest)
}

// DestInRepoError is returned by Plan when a dest, resolved through any
// symlinked parent directories, lands inside the dotfiles repo without being
// that spec's own source. Acting on it would back up, remove, or overwrite a
// tracked repo file, so Plan refuses before anything is mutated.
type DestInRepoError struct {
	// Dest is the $HOME-relative dest from the LinkSpec.
	Dest string
	// Resolved is the dest's real location inside the repo.
	Resolved string
}

// Error implements the error interface.
func (e *DestInRepoError) Error() string {
	return fmt.Sprintf("symlink: dest %q resolves inside the dotfiles repo (%s); refusing to modify repo files",
		e.Dest, e.Resolved)
}

// BrokenLinkError is returned by Validate when one or more managed dests are
// not valid symlinks to their source: a real file where a symlink was
// expected, or a dangling or looping symlink whose source exists.
type BrokenLinkError struct {
	// Dests lists the $HOME-relative dests that failed validation.
	Dests []string
}

// Error implements the error interface.
func (e *BrokenLinkError) Error() string {
	return fmt.Sprintf("symlink: %d dest(s) are not valid symlinks to their source: %s",
		len(e.Dests), strings.Join(e.Dests, ", "))
}

// Plan stats sources and dests and computes the ordered set of Actions without
// touching the filesystem. It hard-fails if the repo root is missing, enforces
// the Linux-desktop safety gate, rejects unsafe dests, and applies OS/flag
// faceting. A dest reached through a symlinked parent that already is its
// source is skipped; one that lands elsewhere inside the repo fails with a
// DestInRepoError. It is pure/read-only and safe to call in dry-run.
func (e *Engine) Plan(specs []LinkSpec, info platform.Info, opts Options) (*Plan, error) {
	if _, err := os.Stat(e.repoRoot); err != nil {
		return nil, &MissingSourceTreeError{Root: e.repoRoot}
	}
	rootReal, err := filepath.EvalSymlinks(e.repoRoot)
	if err != nil {
		return nil, &MissingSourceTreeError{Root: e.repoRoot}
	}

	if DesktopGuardApplies(info.OS) && opts.ManageHyprland && !opts.DryRun && !opts.Backup {
		return nil, &LinuxDesktopGuardError{}
	}

	backupDir := newBackupBase(e.home, time.Now())

	var backups, removes, links, copies, skips []Action

	for _, spec := range specs {
		if !applies(spec, info, opts) {
			continue
		}
		if err := validateDest(spec.Dest); err != nil {
			return nil, err
		}

		srcAbs := filepath.Join(e.repoRoot, spec.Src)
		destAbs := filepath.Join(e.home, spec.Dest)

		srcReal, err := filepath.EvalSymlinks(srcAbs)
		if err != nil {
			skips = append(skips, newAction(spec, ActionSkip, srcAbs, destAbs, "source missing"))
			continue
		}

		// A symlinked parent dir can make the dest an alias of a repo path.
		if destLoc, ok := resolvedLocation(destAbs); ok {
			if destLoc == srcReal {
				skips = append(skips, newAction(spec, ActionSkip, srcAbs, destAbs, "already correct"))
				continue
			}
			if within(rootReal, destLoc) {
				return nil, &DestInRepoError{Dest: spec.Dest, Resolved: destLoc}
			}
		}

		fi, err := os.Lstat(destAbs)
		if spec.Copy {
			switch {
			case err != nil:
				copies = append(copies, newAction(spec, ActionCopy, srcAbs, destAbs, "create"))
			case fi.Mode()&os.ModeSymlink != 0:
				copies = append(copies, newAction(spec, ActionCopy, srcAbs, destAbs, "replace legacy symlink"))
			case fi.Mode().IsRegular() && sameContents(srcAbs, destAbs):
				skips = append(skips, newAction(spec, ActionSkip, srcAbs, destAbs, "already correct"))
			case fi.Mode().IsRegular():
				if opts.Backup {
					backups = append(backups, newAction(spec, ActionBackup, srcAbs, destAbs, "backup real file"))
				}
				copies = append(copies, newAction(spec, ActionCopy, srcAbs, destAbs, "overwrite"))
			default:
				// Directories and other non-regular files: Apply fails loudly.
				copies = append(copies, newAction(spec, ActionCopy, srcAbs, destAbs, "overwrite"))
			}
			continue
		}

		switch {
		case err != nil:
			links = append(links, newAction(spec, ActionLink, srcAbs, destAbs, "create"))
		case fi.Mode()&os.ModeSymlink != 0:
			if linksTo(destAbs, srcAbs, srcReal) {
				skips = append(skips, newAction(spec, ActionSkip, srcAbs, destAbs, "already correct"))
			} else {
				links = append(links, newAction(spec, ActionLink, srcAbs, destAbs, "repoint"))
			}
		default:
			if opts.Backup {
				backups = append(backups, newAction(spec, ActionBackup, srcAbs, destAbs, "backup real file"))
			}
			removes = append(removes, newAction(spec, ActionRemove, srcAbs, destAbs, "remove real file"))
			links = append(links, newAction(spec, ActionLink, srcAbs, destAbs, "replace real file"))
		}
	}

	ordered := make([]Action, 0, len(backups)+len(removes)+len(links)+len(copies)+len(skips))
	ordered = append(ordered, backups...)
	ordered = append(ordered, removes...)
	ordered = append(ordered, links...)
	ordered = append(ordered, copies...)
	ordered = append(ordered, skips...)

	return &Plan{Actions: ordered, BackupDir: backupDir, rootReal: rootReal}, nil
}

// resolvedLocation returns destAbs's real location with its ancestor dirs
// symlink-resolved but its own final component left unfollowed. Missing
// ancestors are tolerated: it resolves the nearest existing ancestor and
// re-joins the missing components, so a symlinked ancestor into the repo
// (e.g. ~/.config -> $REPO/cfg with ~/.config/zed absent) is still seen as
// inside the repo. ok is false only on a real resolution error (not ENOENT),
// such as a symlink loop, a permission failure, or a dangling symlink
// ancestor (which exists, so it is not walked past; writes through it fail).
func resolvedLocation(destAbs string) (string, bool) {
	dir := filepath.Dir(destAbs)
	rest := filepath.Base(destAbs)
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			return filepath.Join(resolved, rest), true
		}
		parent := filepath.Dir(dir)
		if !errors.Is(err, fs.ErrNotExist) || parent == dir {
			return "", false
		}
		if _, lerr := os.Lstat(dir); lerr == nil {
			return "", false
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
	}
}

// within reports whether p is root itself or lies under it. Both paths must be
// absolute and clean.
func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// linksTo reports whether the symlink at destAbs already points at the
// source: its target is spelled exactly srcAbs, or it resolves to srcReal.
func linksTo(destAbs, srcAbs, srcReal string) bool {
	if target, err := os.Readlink(destAbs); err == nil && target == srcAbs {
		return true
	}
	resolved, err := filepath.EvalSymlinks(destAbs)
	return err == nil && resolved == srcReal
}

// sameContents reports whether the files at a and b hold identical bytes. Any
// read error reports false, so a dest that cannot be compared is treated as
// differing (backed up and overwritten) rather than skipped.
func sameContents(a, b string) bool {
	ab, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	bb, err := os.ReadFile(b)
	if err != nil {
		return false
	}
	return bytes.Equal(ab, bb)
}

// newAction builds an Action with resolved absolute paths and a reason.
func newAction(spec LinkSpec, kind ActionKind, srcAbs, destAbs, reason string) Action {
	return Action{Spec: spec, Kind: kind, SrcAbs: srcAbs, DestAbs: destAbs, Reason: reason}
}

// validateDest rejects absolute dests and dests containing a parent (..)
// segment, mirroring manifest.validateSymlink so a join onto $HOME cannot escape.
func validateDest(dest string) error {
	if path.IsAbs(dest) {
		return &UnsafeDestError{Dest: dest}
	}
	for _, elem := range strings.Split(path.Clean(dest), "/") {
		if elem == ".." {
			return &UnsafeDestError{Dest: dest}
		}
	}
	return nil
}
