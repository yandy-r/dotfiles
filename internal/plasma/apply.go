package plasma

// This file is the PLANNING half of apply: repo → live. The execution half —
// including the write-time re-merge that makes the plan's Content a preview
// rather than a payload — lives in applyexec.go, the --link/--unlink state
// machine lives in applylink.go, and the KConfig lock they take lives in
// lock.go.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// PlanApply computes the repo → live plan: resolve each file's SOURCE document
// (plasma/files/ overlaid with plasma/recommended/ under --recommended), merge
// it into the live document, and emit ActionMerge, ActionCreateFile,
// ActionCopyRaw, ActionSymlink, ActionUnlink, ActionBackup, or ActionSkip per
// file. It reads the filesystem and writes nothing.
//
// Three guarantees shape the result. A converged file gets ActionSkip with nil
// Content and is never opened for writing, which is what keeps a converged run
// from reflowing anything and makes applied=0 the honest signal of convergence.
// A file that would switch between --link and merge mode is collected into a
// *LinkModeConflictError returned BEFORE any mutation, satisfiable by --backup
// (which makes the transition recoverable) or --dry-run (which makes it a
// preview). And the merged bytes stored on each action are a PREVIEW: Execute
// re-derives them under the lock, so a plan held open across a confirmation
// prompt cannot write stale bytes over what the running session did meanwhile.
//
// The guard names only files the run would ACTUALLY act on. A conflict whose
// planned action turns out to be ActionSkip is not reported at all: refusing a
// whole run over files it would not have touched — the state a fully linked,
// converged tree is in — made the refusal's own premise false.
func (e *Engine) PlanApply(files []FileSpec, opts Options) (*Plan, error) {
	if _, err := os.Stat(e.repoRoot); err != nil {
		return nil, &MissingSourceTreeError{Root: e.repoRoot}
	}
	if _, err := os.Stat(e.repoFilesDir()); err != nil {
		return nil, &MissingRepoTreeError{Dir: e.repoFilesDir()}
	}
	if opts.Link && opts.Recommended {
		// Refused here rather than per file: the combination is incoherent for
		// every file at once, and a partial run would be worse than none.
		return nil, &LinkRecommendedError{}
	}
	if opts.Link && opts.Unlink {
		return nil, &LinkUnlinkError{}
	}

	plan := &Plan{
		BackupDir:   e.newBackupDir(),
		LookAndFeel: e.lookAndFeelPackage(),
		Display:     opts.Display,
	}
	var conflicts []string
	// --backup makes a mode transition recoverable and --dry-run makes it a
	// preview; either one satisfies the guard, and the file then actually
	// switches mode instead of being skipped.
	guardSatisfied := opts.DryRun || opts.Backup

	for _, spec := range files {
		if err := checkSpec(spec); err != nil {
			return nil, err
		}

		action, conflict, err := e.planApplyFile(spec, opts, guardSatisfied)
		if err != nil {
			return nil, err
		}
		if conflict {
			conflicts = append(conflicts, spec.Path)
		}
		if opts.Backup && needsBackup(action) {
			plan.Files = append(plan.Files, FileAction{
				Spec: spec, Kind: ActionBackup,
				SrcAbs: e.repoPath(spec), DestAbs: e.livePath(spec),
				Reason: "back up the live file before writing",
			})
		}
		plan.Files = append(plan.Files, action)
	}

	switch {
	case len(conflicts) == 0:
		// Nothing transitions, or nothing that transitions would be acted on.
	case !guardSatisfied:
		return nil, &LinkModeConflictError{Paths: conflicts}
	case opts.DryRun && !opts.Backup:
		// --dry-run alone authorised the transition for the PREVIEW only; the
		// same invocation without --dry-run would be refused. That refusal has
		// to travel with the plan, or the preview prints a confident plan the
		// real run then rejects with exit 1.
		plan.LinkConflicts = conflicts
	}

	orderActions(plan.Files)
	plan.Reloads = e.resolveReloads(ReloadPlan(plan))
	return plan, nil
}

// resolveReloads rewrites each planned reload's argv[0] to the binary that
// actually exists on this host, at PLAN time rather than at run time.
//
// A command whose binary resolves nowhere is left exactly as planned rather
// than dropped: the reload set is advisory here, and telling the user which
// component needs reloading is more useful than silently omitting it.
func (e *Engine) resolveReloads(cmds []ReloadCommand) []ReloadCommand {
	for i, cmd := range cmds {
		if argv, ok := e.resolveArgv(cmd.Argv); ok {
			cmds[i].Argv = argv
		}
	}
	return cmds
}

// needsBackup reports whether an action mutates an existing live file and is
// therefore backed up: a bad merge destroys live settings that exist nowhere
// else, so every mutating action qualifies.
// os.Stat rather than os.Lstat is deliberate: what needs preserving is the
// CONTENT KDE is reading, so a linked destination backs up its target, and a
// dangling symlink has nothing to preserve.
func needsBackup(act FileAction) bool {
	switch act.Kind {
	case ActionMerge, ActionCopyRaw, ActionSymlink, ActionUnlink:
		_, err := os.Stat(act.DestAbs)
		return err == nil
	default:
		return false
	}
}

// readOptional reads a file, distinguishing ABSENT from PRESENT-BUT-UNREADABLE.
// An absent file yields (nil, false, nil); every other error becomes a typed
// *UnreadableFileError.
//
// Every read on a managed path goes through here. Collapsing the two states —
// `if err != nil { treat as absent }` — is what let an EACCES/EISDIR/EIO on a
// live file plan a wholesale ActionCreateFile: a blind full replacement of a
// file that is present and full of live-only settings, which --backup cannot
// protect because a create has nothing to back up.
func readOptional(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // table-derived, denylist-checked path
	if err == nil {
		return data, true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return nil, false, &UnreadableFileError{Path: path, Err: err}
}

// planApplyFile builds one file's action and reports whether the guard must
// refuse or announce it. guardSatisfied says whether --backup or --dry-run has
// authorised a mode transition; without it a conflicting file is skipped and
// collected into the *LinkModeConflictError instead.
//
// The order of the steps is the fix for two distinct bugs and must not be
// rearranged for tidiness: the SOURCE document is resolved before any
// "is there anything to do?" decision, because a file that plasma/recommended/
// names and plasma/files/ does not is a first-class source (that is the only
// way baloofilerc, klipperrc, ksmserverrc and plasmarc are reachable at all);
// and the live read is a readOptional, so an unreadable live file is a
// per-file failure rather than a licence to overwrite it.
func (e *Engine) planApplyFile(spec FileSpec, opts Options, guardSatisfied bool) (FileAction, bool, error) {
	livePath := e.livePath(spec)
	repoPath := e.repoPath(spec)

	act := FileAction{Spec: spec, SrcAbs: repoPath, DestAbs: livePath, Mode: liveMode(livePath)}

	repoData, repoPresent, err := readOptional(repoPath)
	if err != nil {
		act.Kind = ActionSkip
		act.Reason = "repo file unreadable"
		act.Err = err
		return act, false, nil
	}

	v := e.planLinkMode(act, spec, opts, guardSatisfied, repoPresent)
	if v.Decided {
		return v.Act, v.Conflict, nil
	}
	act, conflict := v.Act, v.Conflict

	if spec.Format == FormatRaw {
		if !repoPresent {
			act.Kind = ActionSkip
			act.Reason = "nothing to apply"
			return act, false, nil
		}
		act = e.planApplyRaw(act, livePath, repoData, v.Unlink)
		return resolveLinkGuard(act, conflict, guardSatisfied)
	}

	source, err := e.applySource(spec, repoPath, repoData, repoPresent, opts)
	if err != nil {
		return FileAction{}, false, err
	}
	if source == nil {
		act.Kind = ActionSkip
		act.Reason = "nothing to apply"
		return act, false, nil
	}
	act.Source = source

	liveData, livePresent, err := readOptional(livePath)
	if err != nil {
		act.Kind = ActionSkip
		act.Reason = "live file unreadable"
		act.Err = err
		return act, false, nil
	}

	if !livePresent {
		content, werr := Write(source)
		if werr != nil {
			return FileAction{}, false, fmt.Errorf("plasma: serialise %s: %w", spec.Path, werr)
		}
		act.Kind = ActionCreateFile
		act.Reason = "live file absent; materialise it from the repo copy"
		act.Content = content
		act.Keys = createKeys(source)
		act.Mode = livePerm
		return resolveLinkGuard(act, conflict, guardSatisfied)
	}

	liveDoc, err := Parse(livePath, liveData)
	if err != nil {
		return FileAction{}, false, err
	}
	// An unlink is never converged: a symlinked live path READS the repo bytes,
	// so the comparison is the repo against itself. Skipping on it is exactly
	// what made --link a one-way door — the symlink survived every subsequent
	// apply because the merge kept reporting nothing to do.
	if !v.Unlink && Converged(source, liveDoc) {
		act.Kind = ActionSkip
		act.Reason = "already converged"
		act.Source = nil
		return act, false, nil
	}

	merged, keys, errs := Merge(livePath, source, liveDoc)
	if merged == nil {
		act.Kind = ActionSkip
		act.Reason = "refusing to write an immutable file"
		act.Source = nil
		act.Err = errors.Join(errs...)
		return act, false, nil
	}
	content, err := Write(merged)
	if err != nil {
		return FileAction{}, false, fmt.Errorf("plasma: serialise %s: %w", spec.Path, err)
	}
	act.Kind = ActionMerge
	act.Reason = "merge the repo keys into the live file"
	act.Content = content
	act.Keys = keys
	act.Err = errors.Join(errs...)
	switch {
	case v.Unlink:
		act.Kind = ActionUnlink
		act.Reason = "replace the symlink with a real file holding the merged content"
	case len(keys) == 0 && act.Err == nil:
		// Every repo key already matched; the only difference Converged could
		// have seen was in a group the merge did not touch.
		act.Kind = ActionSkip
		act.Reason = "already converged"
		act.Content = nil
		act.Source = nil
	}
	return resolveLinkGuard(act, conflict, guardSatisfied)
}

// applySource builds the document an apply writes into the live file: the
// plasma/files/ snapshot overlaid with plasma/recommended/ under --recommended,
// giving the three-document precedence live < plasma/files < plasma/recommended.
//
// EITHER side may be absent. A recommended-only file is a first-class source —
// four of the ten overlay files (baloofilerc, klipperrc, ksmserverrc, plasmarc)
// have no exported snapshot at all, and resolving the overlay only after an
// "is the repo file there?" early return is what made --recommended a silent
// no-op for them and made `diff --recommended` report them converged.
//
// nil means this run asserts nothing about the file: no snapshot, no overlay,
// or a document that names no entry at all.
func (e *Engine) applySource(
	spec FileSpec,
	repoPath string,
	repoData []byte,
	repoPresent bool,
	opts Options,
) (*Doc, error) {
	var doc *Doc
	if repoPresent {
		parsed, err := Parse(repoPath, repoData)
		if err != nil {
			return nil, err
		}
		doc = parsed
	}

	if opts.Recommended {
		over, err := e.recommendedDoc(spec)
		if err != nil {
			return nil, err
		}
		if over != nil {
			if doc == nil {
				doc = over
			} else {
				doc = Overlay(doc, over)
			}
		}
	}

	if doc == nil || !hasEntries(doc) {
		return nil, nil
	}
	return doc, nil
}

// planApplyRaw builds the action for a FormatRaw member: a verbatim byte copy
// into the live path, or a skip when the bytes already match. The convergence
// check is re-run under the lock at write time; this one only decides whether
// the file appears in the plan at all.
//
// unlink suppresses the convergence skip for the same reason the INI path does:
// a symlinked live path reads the repo's own bytes, so bytes.Equal is trivially
// true and the link would survive every apply.
func (e *Engine) planApplyRaw(act FileAction, livePath string, repoData []byte, unlink bool) FileAction {
	live, present, err := readOptional(livePath)
	if err != nil {
		act.Kind = ActionSkip
		act.Reason = "live file unreadable"
		act.Err = err
		return act
	}
	if present && bytes.Equal(live, repoData) && !unlink {
		act.Kind = ActionSkip
		act.Reason = "already converged"
		return act
	}
	act.Kind = ActionCopyRaw
	act.Reason = "verbatim copy"
	act.Content = repoData
	if unlink {
		act.Kind = ActionUnlink
		act.Reason = "replace the symlink with a real file holding the repo bytes"
	}
	if !present {
		act.Mode = livePerm
	}
	return act
}

// createKeys renders a whole repo document as per-key writes, so a created file
// reports the same key-level detail on stdout as a merge does.
func createKeys(doc *Doc) []KeyAction {
	var keys []KeyAction
	for i := range doc.Groups {
		g := &doc.Groups[i]
		for j := range g.Entries {
			e := &g.Entries[j]
			keys = append(keys, KeyAction{
				Group: g.Path, Key: e.Key, Locale: e.Locale,
				To: e.Value, Tombstone: e.Deleted,
			})
		}
	}
	sortKeyActions(keys)
	return keys
}

// recommendedDoc loads plasma/recommended/<path>, or (nil, nil) when the
// overlay does not carry that file. The overlay is a PARTIAL document: it holds
// just the recommended keys, not a whole config. An overlay file that exists
// but cannot be read is an error, not an absence — silently applying six of ten
// overlay files is exactly the failure mode this package exists to prevent.
func (e *Engine) recommendedDoc(spec FileSpec) (*Doc, error) {
	if spec.Format != FormatINI {
		return nil, nil
	}
	path := e.recommendedPath(spec)
	data, present, err := readOptional(path)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}
	return Parse(path, data)
}

// lookAndFeelPackage reads the single bare line of
// plasma/files/kdedefaults/package. It is resolved at plan time because
// ReloadPlan is pure and reads nothing.
func (e *Engine) lookAndFeelPackage() string {
	data, err := os.ReadFile(filepath.Join(e.repoFilesDir(), "kdedefaults", "package")) //nolint:gosec // fixed repo-relative path
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// liveMode picks the mode for a live-side write: the existing file's mode when
// it exists, else KDE's own 0600.
func liveMode(path string) os.FileMode {
	if fi, err := os.Stat(path); err == nil {
		return fi.Mode().Perm()
	}
	return livePerm
}
