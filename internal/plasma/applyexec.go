package plasma

// This file is the EXECUTION half of a plan — the only place in the package
// that mutates the filesystem. Its central rule is that a live INI file's
// read-modify-write cycle happens HERE, entirely inside the KConfig lock, and
// never spans the plan/execute boundary.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Execute performs a plan's mutations in order. It never re-plans — the SET of
// files and the operation chosen for each are fixed at plan time — but for a
// live INI write it does re-derive the bytes, under the lock, from the live
// file as it exists at write time. DryRun is the first-statement short-circuit,
// so a dry run has provably zero filesystem effect.
//
// It returns a non-nil Report alongside any error so partial progress is
// visible, and a per-file failure never aborts the remaining files. The one
// deliberate exception is a failed ActionBackup: it aborts the very next
// action, which is by construction the write that backup existed to protect.
func (e *Engine) Execute(plan *Plan, opts Options) (*Report, error) {
	report := &Report{Planned: plan.Files, DryRun: opts.DryRun}
	if opts.DryRun {
		return report, nil
	}

	if opts.Backup {
		if err := os.MkdirAll(plan.BackupDir, dirPerm); err != nil {
			return report, fmt.Errorf("plasma: create backup dir %q: %w", plan.BackupDir, err)
		}
	}

	var failures []error
	backupFailed := make(map[string]error)
	for _, act := range plan.Files {
		if act.Kind == ActionSkip {
			// A skipped file's plan-time Err is its only report — nothing will
			// execute to produce a better one.
			if act.Err != nil {
				failures = append(failures, act.Err)
			}
			continue
		}
		if cause, refused := backupFailed[act.Spec.Path]; refused && mutatesLive(act.Kind) {
			failures = append(failures, &BackupRefusedError{Path: act.Spec.Path, Err: cause})
			continue
		}

		done, err := e.executeAction(act, plan.BackupDir)
		if err != nil {
			if act.Kind == ActionBackup {
				backupFailed[act.Spec.Path] = err
			}
			failures = append(failures, err)
			continue
		}
		// One report per file: the executed action's Err. For kinds with
		// nothing to re-derive it IS the plan-time Err, carried through
		// unchanged; for a document write it is the write-time truth, so an
		// immutable group that appeared — or vanished — between planning and
		// writing is reported as it was found under the lock.
		if done.Err != nil {
			failures = append(failures, done.Err)
		}
		if done.Restated {
			report.Restated = append(report.Restated, done.Spec.Path)
		}
		if done.Kind == ActionSkip {
			// The write-time re-check found nothing left to do, so applied=0
			// stays the honest signal of convergence.
			continue
		}
		report.Applied = append(report.Applied, done)
	}
	return report, errors.Join(failures...)
}

// mutatesLive reports whether a kind writes to or replaces the live file, and
// is therefore the thing an ActionBackup was emitted to protect.
func mutatesLive(kind ActionKind) bool {
	switch kind {
	case ActionMerge, ActionCreateFile, ActionCopyRaw, ActionSymlink, ActionUnlink:
		return true
	default:
		return false
	}
}

// executeAction performs one planned action and returns the action AS
// PERFORMED. For a live document write that is not the action it was handed:
// Content, Keys, Mode, Reason and Restated are re-derived from the write-time
// merge, so Report.Applied describes what actually landed rather than what the
// plan predicted.
func (e *Engine) executeAction(act FileAction, backupDir string) (FileAction, error) {
	switch act.Kind {
	case ActionBackup:
		return act, e.doBackup(act, backupDir)
	case ActionPrune:
		return act, e.doPrune(act)
	case ActionSymlink:
		return act, e.doSymlink(act)
	case ActionExport:
		return act, e.writeRepo(act)
	case ActionMerge, ActionCreateFile:
		return e.writeMerged(act)
	case ActionUnlink:
		// An unlink is a normal live write onto a path that happens to be a
		// symlink: writeAtomic renames over the DIRECTORY ENTRY, so the link is
		// replaced by the real file rather than followed into the repo. The two
		// writers differ only in whether there is a document to re-merge, which
		// is exactly what Format already says.
		if act.Spec.Format == FormatRaw {
			return e.writeRawLive(act)
		}
		return e.writeMerged(act)
	case ActionCopyRaw:
		if act.ToRepo {
			return act, e.writeRepo(act)
		}
		return e.writeRawLive(act)
	case ActionSkip:
		return act, nil
	default:
		return act, fmt.Errorf("plasma: unknown action kind %d", act.Kind)
	}
}

// writeMerged performs the whole read-modify-write cycle for a live INI file
// INSIDE the lock: take <path>.lock, read the live file, parse it, merge the
// action's Source document into it, and write the result atomically.
//
// This is the single most important ordering rule in the package. Doing the
// merge at plan time and writing the frozen bytes here looked equivalent and
// was not: `dotf plasma apply` shows a confirmation prompt between the two, and
// a running Plasma session rewrites these files continuously, so every key KDE
// wrote while the prompt was up — new keys, whole new groups, updated values —
// was silently deleted by bytes computed before they existed. The lock covered
// only the rename, which protects nothing when the content it renames is stale.
//
// The response to a live file that changed underneath is to RE-MERGE and
// proceed rather than abort: the merge is deterministic and non-destructive by
// construction (it starts as a copy of live and only ever overwrites the keys
// the repo names), so the newer live file is a strictly better input than the
// snapshot. What the user is not told cannot be assessed, though, so bytes that
// differ from the previewed ones set Restated, and Reason says why.
func (e *Engine) writeMerged(act FileAction) (FileAction, error) {
	release, err := e.acquireLock(act.DestAbs)
	if err != nil {
		return act, err
	}
	defer release()

	liveData, livePresent, err := readOptional(act.DestAbs)
	if err != nil {
		return act, err
	}
	mode := livePerm
	var liveDoc *Doc
	if livePresent {
		if liveDoc, err = Parse(act.DestAbs, liveData); err != nil {
			return act, err
		}
		mode = liveMode(act.DestAbs)
	}

	merged, keys, errs := Merge(act.DestAbs, act.Source, liveDoc)
	if merged == nil {
		// The file gained a file-level [$i] since planning. Refusing is the
		// same answer the planner would have given.
		return act, errors.Join(errs...)
	}
	content, err := Write(merged)
	if err != nil {
		return act, fmt.Errorf("plasma: serialise %s: %w", act.Spec.Path, err)
	}

	done := act
	done.Content = content
	done.Keys = keys
	done.Mode = mode
	done.Err = errors.Join(errs...)
	if !bytes.Equal(content, act.Content) {
		done.Restated = true
		done.Reason = act.Reason +
			" (re-merged at write time: the live file changed after the plan was computed)"
	}
	if err := writeAtomic(act.DestAbs, content, mode); err != nil {
		return done, err
	}
	return done, nil
}

// writeRawLive copies a FormatRaw member's repo bytes over the live file, with
// the convergence check re-run inside the lock. A raw file has no merge to
// redo, but the DECISION to write is still made against the live file as it is
// at write time, so a file the session brought into agreement while the prompt
// was up is left alone instead of being rewritten.
//
// ActionUnlink is exempt from that check: its bytes are converged BY
// CONSTRUCTION — the read followed the symlink into the repo — and the point of
// the write is the destination's type, not its content.
func (e *Engine) writeRawLive(act FileAction) (FileAction, error) {
	release, err := e.acquireLock(act.DestAbs)
	if err != nil {
		return act, err
	}
	defer release()

	live, present, err := readOptional(act.DestAbs)
	if err != nil {
		return act, err
	}
	if present && bytes.Equal(live, act.Content) && act.Kind != ActionUnlink {
		done := act
		done.Kind = ActionSkip
		done.Reason = "already converged at write time"
		done.Content = nil
		done.Restated = true
		return done, nil
	}

	mode := livePerm
	if present {
		mode = liveMode(act.DestAbs)
	}
	done := act
	done.Mode = mode
	return done, writeAtomic(act.DestAbs, act.Content, mode)
}

// doBackup copies a live file into the backup dir under its config-dir-relative
// path. A symlinked destination is followed deliberately: what needs preserving
// is the CONTENT KDE is reading, not the link.
func (e *Engine) doBackup(act FileAction, backupDir string) error {
	data, err := os.ReadFile(act.DestAbs) //nolint:gosec // table-derived, denylist-checked path
	if err != nil {
		return fmt.Errorf("plasma: read backup source %q: %w", act.DestAbs, err)
	}
	dst := filepath.Join(backupDir, filepath.FromSlash(act.Spec.Path))
	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		return fmt.Errorf("plasma: create backup dir for %q: %w", dst, err)
	}
	if err := os.WriteFile(dst, data, livePerm); err != nil {
		return fmt.Errorf("plasma: backup %q: %w", act.DestAbs, err)
	}
	return os.Chmod(dst, livePerm)
}

// doPrune deletes a repo file whose keys no longer survive the strip. It
// refuses to delete anything outside plasma/files/, because a table typo must
// not become an arbitrary unlink.
func (e *Engine) doPrune(act FileAction) error {
	root := e.repoFilesDir() + string(os.PathSeparator)
	if !strings.HasPrefix(act.SrcAbs, root) {
		return fmt.Errorf("plasma: refusing to prune %q outside %q", act.SrcAbs, e.repoFilesDir())
	}
	if err := os.Remove(act.SrcAbs); err != nil {
		return fmt.Errorf("plasma: prune %q: %w", act.SrcAbs, err)
	}
	return nil
}

// doSymlink replaces the live path with a symlink into the repo.
func (e *Engine) doSymlink(act FileAction) error {
	if err := os.MkdirAll(filepath.Dir(act.DestAbs), dirPerm); err != nil {
		return fmt.Errorf("plasma: create parent dir for %q: %w", act.DestAbs, err)
	}
	if _, err := os.Lstat(act.DestAbs); err == nil {
		if err := os.Remove(act.DestAbs); err != nil {
			return fmt.Errorf("plasma: remove %q: %w", act.DestAbs, err)
		}
	}
	if err := os.Symlink(act.SrcAbs, act.DestAbs); err != nil {
		return fmt.Errorf("plasma: link %q -> %q: %w", act.DestAbs, act.SrcAbs, err)
	}
	return nil
}

// writeRepo writes a repo-side file (an export or a raw capture).
//
// A repo file is normally uncontended — Plasma has no idea it exists — so no
// lock is taken. The exception is a --link'ed destination: KDE is then writing
// the repo file THROUGH the symlink, and it takes its lock next to the path it
// opened, i.e. beside the symlink in the config dir. So that is where this
// takes it too.
func (e *Engine) writeRepo(act FileAction) error {
	if linked, _ := e.isLinkedIntoRepo(act.DestAbs, act.SrcAbs); linked {
		release, err := e.acquireLock(act.DestAbs)
		if err != nil {
			return err
		}
		defer release()
	}
	return writeAtomic(act.SrcAbs, act.Content, act.Mode)
}

// writeAtomic writes content to path via a temp file in the SAME directory,
// then renames. Same-directory is load-bearing: the rename is then guaranteed
// same-filesystem and therefore atomic, so a crash at any point leaves either
// the old file or the new one and never a truncated one.
//
// Chmod is explicit rather than relying on the create mode so the process umask
// cannot weaken it.
func writeAtomic(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("plasma: create parent dir for %q: %w", path, err)
	}
	if mode == 0 {
		mode = livePerm
	}

	tmp, err := os.CreateTemp(dir, ".dotf-plasma-*")
	if err != nil {
		return fmt.Errorf("plasma: create temp file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("plasma: write %q: %w", tmpName, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("plasma: chmod %q: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("plasma: sync %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("plasma: close %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("plasma: rename %q -> %q: %w", tmpName, path, err)
	}
	return nil
}
