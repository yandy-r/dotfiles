package symlink

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// UnlinkOptions select and gate an Unlink.
type UnlinkOptions struct {
	// Dests are $HOME-relative table dests to unlink; a leading "~/" or an
	// absolute path under $HOME (a shell-expanded ~/x) is accepted. Ignored
	// when All is set.
	Dests []string
	// All targets every table entry, regardless of facet.
	All bool
	// Restore restores each removed link's dest from the newest manifest
	// backup holding it (through Engine.Restore); dests without a backup are
	// only unlinked.
	Restore bool
	// DryRun computes the report without writing.
	DryRun bool
}

// Unlink actions recorded in UnlinkItem.Action.
const (
	// UnlinkActionRemove removes (or, in a dry run, would remove) the link.
	UnlinkActionRemove = "remove"
	// UnlinkActionSkip leaves the dest untouched; UnlinkItem.Reason says why.
	UnlinkActionSkip = "skip"
)

// Unlink skip reasons recorded in UnlinkItem.Reason.
const (
	unlinkSkipMissing  = "missing"
	unlinkSkipRealFile = "real file"
	unlinkSkipForeign  = "not a dotfiles link"
	unlinkSkipCopy     = "copy entry"
	unlinkSkipInRepo   = "inside repo"
	// unlinkSkipRemoveFailed marks the removal that returned the error.
	unlinkSkipRemoveFailed = "remove failed"
	// unlinkSkipNotAttempted marks removals not reached after a failure.
	unlinkSkipNotAttempted = "not attempted"
)

// UnlinkItem is one targeted table dest and what Unlink did (or would do).
type UnlinkItem struct {
	// Dest is the table entry's $HOME-relative Dest.
	Dest string
	// DestAbs is filepath.Join(home, Dest).
	DestAbs string
	// Action is UnlinkActionRemove or UnlinkActionSkip.
	Action string
	// Reason is the skip reason ("missing", "real file", "not a dotfiles
	// link", "copy entry", "inside repo", or after a removal error "remove
	// failed" / "not attempted"); empty for a removal.
	Reason string
	// BackupID is, with Restore, the newest non-legacy backup set holding
	// the dest; empty when none holds it (or without Restore). It names the
	// planned restore source and stays set when the restore is not
	// performed; see Restored.
	BackupID string
	// Restored reports that the dest was restored from BackupID (or, in
	// DryRun, that the restore is planned). False with a non-empty BackupID
	// means the link was left unlinked: not removed, or its restore was not
	// reached or failed.
	Restored bool
	// target is the raw link target observed at plan time; the removal
	// re-checks the link still reads it.
	target string
}

// UnlinkReport is the outcome of Unlink.
type UnlinkReport struct {
	// Items are the targeted dests in table order.
	Items []UnlinkItem
	// Restored are the restores performed (or, in DryRun, planned).
	Restored []RestoreItem
	// DryRun records whether nothing was written.
	DryRun bool
}

// UnknownDestError is returned by Unlink, before any change, when a named
// dest is not a table entry's Dest.
type UnknownDestError struct {
	// Dests are the unknown names as given.
	Dests []string
}

// Error implements the error interface.
func (e *UnknownDestError) Error() string {
	quoted := make([]string, len(e.Dests))
	for i, d := range e.Dests {
		quoted[i] = strconv.Quote(d)
	}
	return fmt.Sprintf("symlink: unknown dest(s) %s: not in the link table", strings.Join(quoted, ", "))
}

// Unlink removes the managed symlinks at the selected table dests (opts.Dests,
// or every entry with opts.All; facets are ignored). Unknown names fail with
// an UnknownDestError and an unsafe dest with an UnsafeDestError, both before
// any change. Only a symlink whose first hop (see readLinkTarget) lies in the
// repo is removed (dangling ones count); Copy entries, missing dests, real
// files, dests whose parent dirs resolve inside the repo and foreign symlinks
// are skipped with a reason. Each removal re-checks, immediately before
// os.Remove, that the dest is still the same symlink and still outside the
// repo; a changed dest is reported as skipped. With Restore, each removed dest
// held by a non-legacy backup is then restored from the newest such backup via
// Engine.Restore; every restore is first dry-run, and any error aborts before
// a link is removed. DryRun writes nothing and fills Restored with the
// planned restores. A missing repo root yields a MissingSourceTreeError. On a
// removal error the failed item and later unremoved ones become skips
// ("remove failed", "not attempted"), the dests already removed are still
// restored, and the removal and any restore error are returned joined. A
// restore error stops the remaining restore groups. Only items whose restore
// completed (or, in DryRun, is planned) have Restored set.
func (e *Engine) Unlink(specs []LinkSpec, opts UnlinkOptions) (*UnlinkReport, error) {
	rootReal, err := e.resolveRepoRoot()
	if err != nil {
		return nil, err
	}
	selected, err := selectUnlinkSpecs(specs, opts, e.home)
	if err != nil {
		return nil, err
	}

	report := &UnlinkReport{DryRun: opts.DryRun}
	for _, spec := range selected {
		report.Items = append(report.Items, e.planUnlink(spec, rootReal))
	}
	if !opts.Restore {
		return report, e.removeLinks(report, rootReal, opts.DryRun)
	}

	sets, err := ListBackups(e.home)
	if err != nil {
		return nil, err
	}
	for i := range report.Items {
		if item := &report.Items[i]; item.Action == UnlinkActionRemove {
			item.BackupID = newestBackupHolding(sets, item.Dest)
		}
	}

	if !opts.DryRun {
		// Validate every restore before touching anything: Restore does all of
		// its checks at plan time, and the dests are symlinks (never a
		// conflict), so any error here would also fail the real restore.
		order, groups := restoreGroups(report.Items)
		for _, id := range order {
			if _, err := e.Restore(id, RestoreOptions{Paths: groups[id], DryRun: true}); err != nil {
				return nil, err
			}
		}
	}

	// A removal error does not stop the restores: the dests already removed
	// are restored so a failure never leaves a backed-up dest unlinked.
	removeErr := e.removeLinks(report, rootReal, opts.DryRun)
	return report, errors.Join(removeErr, e.restoreRemoved(report, opts.DryRun))
}

// removeLinks removes every "remove" item of report (nothing in a dry run).
// A dest that changed before removal becomes a "not a dotfiles link" skip; on
// a removal error the failed and later unremoved items become skips (see
// markNotRemoved) and the error is returned.
func (e *Engine) removeLinks(report *UnlinkReport, rootReal string, dryRun bool) error {
	if dryRun {
		return nil
	}
	for i := range report.Items {
		item := &report.Items[i]
		if item.Action != UnlinkActionRemove {
			continue
		}
		removed, err := removeManagedLink(item, rootReal)
		if err != nil {
			markNotRemoved(report.Items[i:], unlinkSkipRemoveFailed)
			return err
		}
		if !removed {
			item.Action, item.Reason = UnlinkActionSkip, unlinkSkipForeign
		}
	}
	return nil
}

// restoreRemoved restores the removed (in a dry run, to-be-removed) items
// held by a backup, one Engine.Restore per backup set in first-seen order,
// appending the performed (or planned) restores to report.Restored and
// setting Restored on their items. It stops at the first restore error.
func (e *Engine) restoreRemoved(report *UnlinkReport, dryRun bool) error {
	order, groups := restoreGroups(report.Items)
	for _, id := range order {
		rep, err := e.Restore(id, RestoreOptions{Paths: groups[id], DryRun: dryRun})
		if rep != nil {
			report.Restored = append(report.Restored, rep.Items...)
			markRestored(report.Items, id, rep.Items)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// restoreGroups groups the dests of the "remove" items with a BackupID by
// backup set, returning the set IDs in first-seen order.
func restoreGroups(items []UnlinkItem) ([]string, map[string][]string) {
	var order []string
	groups := map[string][]string{}
	for _, it := range items {
		if it.Action != UnlinkActionRemove || it.BackupID == "" {
			continue
		}
		if _, ok := groups[it.BackupID]; !ok {
			order = append(order, it.BackupID)
		}
		groups[it.BackupID] = append(groups[it.BackupID], it.Dest)
	}
	return order, groups
}

// markRestored sets Restored on the "remove" items restored from backup id,
// i.e. those whose dest appears in done.
func markRestored(items []UnlinkItem, id string, done []RestoreItem) {
	restored := make(map[string]bool, len(done))
	for _, r := range done {
		restored[path.Clean(r.Dest)] = true
	}
	for i := range items {
		it := &items[i]
		if it.Action == UnlinkActionRemove && it.BackupID == id && restored[path.Clean(it.Dest)] {
			it.Restored = true
		}
	}
}

// markNotRemoved turns the "remove" items of items into skips so a report
// never names an unperformed removal: the first item (whose removal failed)
// gets reason, the rest "not attempted".
func markNotRemoved(items []UnlinkItem, reason string) {
	for i := range items {
		it := &items[i]
		if it.Action != UnlinkActionRemove {
			continue
		}
		it.Action, it.Reason = UnlinkActionSkip, reason
		reason = unlinkSkipNotAttempted
	}
}

// selectUnlinkSpecs resolves opts to the targeted specs in table order,
// deduplicated by Dest. A name may be $HOME-relative, "~/"-prefixed or an
// absolute path under home; an absolute path outside home is unknown. Every
// name is checked before returning, so unknown names fail before any change.
func selectUnlinkSpecs(specs []LinkSpec, opts UnlinkOptions, home string) ([]LinkSpec, error) {
	want := map[string]bool{}
	if !opts.All {
		known := make(map[string]bool, len(specs))
		for _, spec := range specs {
			known[path.Clean(spec.Dest)] = true
		}
		var unknown []string
		for _, d := range opts.Dests {
			key, ok := unlinkDestKey(d, home)
			if !ok || !known[key] {
				unknown = append(unknown, d)
				continue
			}
			want[key] = true
		}
		if len(unknown) > 0 {
			return nil, &UnknownDestError{Dests: unknown}
		}
	}
	var out []LinkSpec
	seen := map[string]bool{}
	for _, spec := range specs {
		key := path.Clean(spec.Dest)
		if seen[key] || (!opts.All && !want[key]) {
			continue
		}
		if err := validateDest(spec.Dest); err != nil {
			return nil, err
		}
		seen[key] = true
		out = append(out, spec)
	}
	return out, nil
}

// unlinkDestKey maps a user-supplied dest onto table Dest form. An absolute
// path is made $HOME-relative first; one outside home reports false.
func unlinkDestKey(d, home string) (string, bool) {
	if filepath.IsAbs(d) {
		rel, err := filepath.Rel(home, d)
		if err != nil || rel == "." || !within(home, filepath.Clean(d)) {
			return "", false
		}
		d = rel
	}
	return normalizeRestorePath(d), true
}

// planUnlink classifies one targeted spec without touching the filesystem.
func (e *Engine) planUnlink(spec LinkSpec, rootReal string) UnlinkItem {
	destAbs := filepath.Join(e.home, filepath.FromSlash(path.Clean(spec.Dest)))
	item := UnlinkItem{Dest: spec.Dest, DestAbs: destAbs, Action: UnlinkActionSkip}
	if spec.Copy {
		item.Reason = unlinkSkipCopy
		return item
	}
	fi, err := os.Lstat(destAbs)
	switch {
	case err != nil:
		item.Reason = unlinkSkipMissing
		return item
	case fi.Mode()&os.ModeSymlink == 0:
		item.Reason = unlinkSkipRealFile
		return item
	}
	if !parentOutsideRepo(destAbs, rootReal) {
		item.Reason = unlinkSkipInRepo
		return item
	}
	raw, hop, err := readLinkTarget(destAbs)
	if err != nil || !e.pointsIntoRepo(hop, rootReal) {
		item.Reason = unlinkSkipForeign
		return item
	}
	item.Action, item.target = UnlinkActionRemove, raw
	return item
}

// parentOutsideRepo reports whether destAbs, with its parent dirs
// symlink-resolved, lies outside the repo. An unresolvable location is
// treated as inside (refused).
func parentOutsideRepo(destAbs, rootReal string) bool {
	loc, ok := resolvedLocation(destAbs)
	return ok && !within(rootReal, loc)
}

// removeManagedLink removes item's link after re-checking that it is still a
// symlink reading the planned target and still outside the repo. It reports
// false (no error) when the dest changed or vanished.
func removeManagedLink(item *UnlinkItem, rootReal string) (bool, error) {
	if !parentOutsideRepo(item.DestAbs, rootReal) || !stillLinksTo(item.DestAbs, item.target) {
		return false, nil
	}
	if err := os.Remove(item.DestAbs); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("symlink: remove link %q: %w", item.DestAbs, err)
	}
	return true, nil
}

// newestBackupHolding returns the ID of the first (newest, in ListBackups
// order) non-legacy set whose manifest holds dest, or "".
func newestBackupHolding(sets []BackupSet, dest string) string {
	key := path.Clean(dest)
	for _, set := range sets {
		if set.Legacy {
			continue
		}
		for _, ent := range set.Entries {
			if path.Clean(ent.Dest) == key {
				return set.ID
			}
		}
	}
	return ""
}
