package symlink

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yandy-r/dotfiles/internal/platform"
)

// Options are the CLI knobs, mapped 1:1 from playbooks/vars/symlinks.yml config
// vars.
type Options struct {
	// DryRun disables all filesystem mutations (Ansible dry_run).
	DryRun bool
	// Backup copies every real file or directory the plan replaces into
	// BackupDir before removing it. The CLIs default it to true (--no-backup
	// opts out); false removes real files without a copy.
	Backup bool
	// Force is retained for parity with Ansible force_symlinks; it is inert in
	// practice because cleanup (ActionRemove) owns real-file removal.
	Force bool
	// Validate runs post-apply validation (Ansible validate_after; default true).
	Validate bool
	// ManageHyprland opts into the Hyprland/wlroots desktop config set (Ansible
	// manage_linux_desktop). The CLI spells it --hyprland, with --linux-desktop
	// retained as a back-compat alias.
	ManageHyprland bool
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
	// ActionCopy is the Zed exception: unlink a legacy symlink, then copy
	// Src -> Dest (overwrite, 0644).
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
}

// ValidationResult is one post-apply lstat check over a managed dest.
type ValidationResult struct {
	// Dest is the $HOME-relative destination that was checked.
	Dest string
	// Valid is true when Dest exists and is a symlink (a dangling symlink is
	// still Valid, matching Ansible).
	Valid bool
	// Broken is true when Dest exists but is not a symlink — a real file where
	// a symlink was expected (the validation fail condition).
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
// linking into home ($HOME).
func NewEngine(repoRoot, home string) *Engine {
	return &Engine{repoRoot: repoRoot, home: home}
}

// MissingSourceTreeError is returned when the dotfiles source tree does not
// exist (Ansible pre_task step 3, a hard pre-flight failure).
type MissingSourceTreeError struct {
	// Root is the missing repo root path.
	Root string
}

// Error implements the error interface.
func (e *MissingSourceTreeError) Error() string {
	return fmt.Sprintf("symlink: dotfiles source tree %q does not exist", e.Root)
}

// LinuxDesktopGuardError is returned when the Linux-desktop set is requested on
// a Linux-family host with neither dry-run nor backups enabled (the Ansible
// safety gate that prevents clobbering real ~/.config desktop directories).
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

// BrokenLinkError is returned by Validate when one or more managed dests are
// real files where a symlink was expected (exists && !islnk).
type BrokenLinkError struct {
	// Dests lists the $HOME-relative dests that failed validation.
	Dests []string
}

// Error implements the error interface.
func (e *BrokenLinkError) Error() string {
	return fmt.Sprintf("symlink: %d dest(s) are real files where a symlink was expected: %s",
		len(e.Dests), strings.Join(e.Dests, ", "))
}

// Plan stats sources and dests and computes the ordered set of Actions without
// touching the filesystem. It hard-fails if the repo root is missing, enforces
// the Linux-desktop safety gate, rejects unsafe dests, and applies OS/flag
// faceting. It is pure/read-only and safe to call in dry-run.
func (e *Engine) Plan(specs []LinkSpec, info platform.Info, opts Options) (*Plan, error) {
	if _, err := os.Stat(e.repoRoot); err != nil {
		return nil, &MissingSourceTreeError{Root: e.repoRoot}
	}

	if DesktopGuardApplies(info.OS) && opts.ManageHyprland && !opts.DryRun && !opts.Backup {
		return nil, &LinuxDesktopGuardError{}
	}

	backupDir := filepath.Join(e.home, ".config",
		"dotfiles_backup_"+strconv.FormatInt(time.Now().Unix(), 10))

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

		if _, err := os.Stat(srcAbs); err != nil {
			skips = append(skips, newAction(spec, ActionSkip, srcAbs, destAbs, "source missing"))
			continue
		}

		if spec.Copy {
			copies = append(copies, newAction(spec, ActionCopy, srcAbs, destAbs, "copy"))
			continue
		}

		fi, err := os.Lstat(destAbs)
		switch {
		case err != nil:
			links = append(links, newAction(spec, ActionLink, srcAbs, destAbs, "create"))
		case fi.Mode()&os.ModeSymlink != 0:
			if target, rerr := os.Readlink(destAbs); rerr == nil && target == srcAbs {
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

	return &Plan{Actions: ordered, BackupDir: backupDir}, nil
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
