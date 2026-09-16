package plasma

// This file holds every typed error the package returns and the CLI matches
// with errors.As. They live apart from the vocabulary in plasma.go because they
// are a closed, independently-reviewable surface: each one is a contract with
// the CLI's exit code and message, and none of them carries behaviour. Every
// member is CONSTRUCTED on some production path — an exported error the CLI can
// never receive is a contract that reads as a guarantee and enforces nothing.
//
// That is why there is no ownership-conflict error here. The §3.1 guarantee
// that `dotf link` and `dotf plasma` never both own a path is a property of two
// COMPILE-TIME tables, symlink.DefaultTable and DefaultFiles, so a runtime check
// would recompute a constant answer on every command and could only fail on a
// build that was already broken. It is enforced instead by
// TestNoOverlapWithSymlinkTable in plasma_test.go, which fails the build the
// moment a widening of either table double-claims a path.

import (
	"fmt"
	"strings"
	"time"

	"github.com/yandy-r/dotfiles/internal/platform"
)

// MissingSourceTreeError is returned when the dotfiles repo root does not
// exist. It is a hard pre-flight failure for every verb.
type MissingSourceTreeError struct {
	// Root is the missing repo root path.
	Root string
}

// Error implements the error interface.
func (e *MissingSourceTreeError) Error() string {
	return fmt.Sprintf("plasma: dotfiles source tree %q does not exist", e.Root)
}

// MissingRepoTreeError is returned by PlanApply when repoRoot exists but
// plasma/files/ does not — export has not been run yet.
type MissingRepoTreeError struct {
	// Dir is the missing plasma/files directory.
	Dir string
}

// Error implements the error interface.
func (e *MissingRepoTreeError) Error() string {
	return fmt.Sprintf("plasma: %q does not exist; run `dotf plasma export` first", e.Dir)
}

// NoLiveFilesError is returned by PlanExport when no managed file exists live —
// there is nothing to capture.
type NoLiveFilesError struct {
	// ConfigDir is the directory that was searched.
	ConfigDir string
}

// Error implements the error interface.
func (e *NoLiveFilesError) Error() string {
	return fmt.Sprintf("plasma: no managed KDE config files found under %q", e.ConfigDir)
}

// UnsupportedOSError is returned when the host is not Linux. It is an error
// rather than a usage error: the invocation was well-formed, the host is not.
type UnsupportedOSError struct {
	// OS is the detected operating system.
	OS platform.OS
}

// Error implements the error interface.
func (e *UnsupportedOSError) Error() string {
	return fmt.Sprintf("plasma: unsupported on %s (Linux only)", e.OS)
}

// UnsafePathError is returned when a FileSpec.Path escapes the config dir or is
// absolute, guarding filepath.Join(configDir, path) the way
// symlink.UnsafeDestError guards $HOME.
type UnsafePathError struct {
	// Path is the offending config-dir-relative path.
	Path string
}

// Error implements the error interface.
func (e *UnsafePathError) Error() string {
	return fmt.Sprintf("plasma: unsafe path %q must be relative to the config dir and free of ..", e.Path)
}

// DenylistError is returned when a path resolves inside a Denylist prefix.
type DenylistError struct {
	// Path is the offending config-dir-relative path.
	Path string
}

// Error implements the error interface.
func (e *DenylistError) Error() string {
	return fmt.Sprintf("plasma: refusing to touch %q (denylisted: secrets or machine identity)", e.Path)
}

// UnreadableFileError is returned when a managed file EXISTS but cannot be
// read: EACCES on a root-owned rc file, EISDIR where a file is expected, EIO on
// failing media.
//
// The distinction from absence is load-bearing and is why every read in this
// package goes through readOptional. "Could not read it" collapsed into "it is
// not there" would make apply plan a wholesale ActionCreateFile over a file
// that is present and full of live-only settings — a blind overwrite that
// --backup cannot protect, because a create has nothing to back up. Absence is
// a normal state with a defined action; unreadability is a per-file failure.
type UnreadableFileError struct {
	// Path is the file that could not be read.
	Path string
	// Err is the underlying filesystem error.
	Err error
}

// Error implements the error interface.
func (e *UnreadableFileError) Error() string {
	return fmt.Sprintf("plasma: %q exists but cannot be read: %v", e.Path, e.Err)
}

// Unwrap exposes the filesystem cause to errors.Is.
func (e *UnreadableFileError) Unwrap() error { return e.Err }

// PrivacyLeakError is returned by PlanExport, before any write, when a file
// about to land in plasma/files/ contains an absolute home path.
type PrivacyLeakError struct {
	// Path is the file that would have been written.
	Path string
	// Line is the 1-based line the match was found on.
	Line int
	// Pattern names the rule that matched.
	Pattern string
}

// Error implements the error interface.
func (e *PrivacyLeakError) Error() string {
	return fmt.Sprintf("plasma: %s:%d matches %s; refusing to export an absolute home path",
		e.Path, e.Line, e.Pattern)
}

// ImmutableError is returned when the live file or one of its groups carries a
// KConfig immutability marker ([$i]). A deliberately locked-down config is not
// something to clobber, so the write is refused; the run continues and the
// failure is reported per file.
type ImmutableError struct {
	// Path is the live file.
	Path string
	// Group is the immutable group's decoded path; nil means the whole file.
	Group []string
}

// Error implements the error interface.
func (e *ImmutableError) Error() string {
	what := e.Path
	if len(e.Group) > 0 {
		what = e.Path + " group " + renderGroup(e.Group)
	}
	return fmt.Sprintf("plasma: %q is marked immutable by KConfig ([$i]); refusing to write", what)
}

// LinkModeConflictError is returned by PlanApply, before any mutation, when a
// file would switch between --link and merge mode. It mirrors
// symlink.LinuxDesktopGuardError: a guard satisfiable by --backup (which makes
// the transition recoverable) or --dry-run (which makes it a preview).
type LinkModeConflictError struct {
	// Paths lists the config-dir-relative paths in conflict.
	Paths []string
}

// Error implements the error interface.
func (e *LinkModeConflictError) Error() string {
	return fmt.Sprintf("plasma: %d file(s) would switch between --link and merge mode: %s; "+
		"re-run with --backup (or --dry-run to preview)",
		len(e.Paths), strings.Join(e.Paths, ", "))
}

// LinkRecommendedError is returned by PlanApply when --link and --recommended
// are combined. The two are mutually exclusive by construction, not by
// oversight: a --link'ed live path IS plasma/files/<path>, so there is nowhere
// for plasma/recommended/ to land short of writing the overlay into the
// exported snapshot — which the next `dotf plasma export` would strip straight
// back out.
//
// Refusing up front is the honest option because Diff and Validate DO honour
// --recommended: silently dropping the overlay left every recommended key
// reported as drift by the very validation the same command runs, on every
// subsequent run, with no way to converge.
type LinkRecommendedError struct{}

// Error implements the error interface.
func (e *LinkRecommendedError) Error() string {
	return "plasma: --link cannot be combined with --recommended: a linked file IS the " +
		"plasma/files/ snapshot, so the recommended overlay would never land while " +
		"diff and validate would keep reporting it as drift; drop one of the two flags"
}

// LinkUnlinkError is returned by PlanApply when --link and --unlink are
// combined. They are exact inverses — one replaces every managed file with a
// symlink into the repo, the other replaces every symlink with a real file — so
// there is no coherent per-file resolution and no order in which both could be
// honoured.
type LinkUnlinkError struct{}

// Error implements the error interface.
func (e *LinkUnlinkError) Error() string {
	return "plasma: --link cannot be combined with --unlink: they are exact inverses " +
		"(--link replaces each managed file with a symlink into the repo, --unlink " +
		"replaces each symlink with a real file); drop one of the two flags"
}

// BackupRefusedError is returned for a mutating action whose protective backup
// could not be written.
//
// --backup is what AUTHORISES the destructive step (a --link transition over a
// real file is refused without it), so a failed copy must abort the write it was
// emitted to protect rather than let Execute's loop walk on to the very next
// plan entry — which, because a file's ActionBackup is ordered immediately
// before its own mutating action, is precisely the unprotected write.
type BackupRefusedError struct {
	// Path is the config-dir-relative path whose write was refused.
	Path string
	// Err is the backup failure that caused the refusal.
	Err error
}

// Error implements the error interface.
func (e *BackupRefusedError) Error() string {
	return fmt.Sprintf("plasma: refusing to modify %q because its backup failed: %v", e.Path, e.Err)
}

// Unwrap exposes the backup failure to errors.Is and errors.As.
func (e *BackupRefusedError) Unwrap() error { return e.Err }

// LockError is returned when <path>.lock could not be acquired within the
// timeout, which normally means a running Plasma session holds it.
type LockError struct {
	// Path is the file whose lock could not be taken.
	Path string
	// Held is how long acquisition was attempted.
	Held time.Duration
}

// Error implements the error interface.
func (e *LockError) Error() string {
	return fmt.Sprintf("plasma: could not acquire %q within %s (a running Plasma session may hold it)",
		e.Path, e.Held)
}

// DriftError is the many-item collector shape, mirroring
// symlink.BrokenLinkError. Diff and Validate return it ALONGSIDE their full
// results, never instead of them.
type DriftError struct {
	// Files lists the config-dir-relative paths that differ.
	Files []string
}

// Error implements the error interface.
func (e *DriftError) Error() string {
	return fmt.Sprintf("plasma: %d file(s) differ from the repo: %s",
		len(e.Files), strings.Join(e.Files, ", "))
}
