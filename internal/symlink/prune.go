package symlink

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Stale link reasons recorded in StaleLink.Reason.
const (
	// StaleReasonOrphaned marks a symlink into the repo that is not the link
	// of any table entry (nor an ancestor of one) and whose first hop is
	// missing or is a table entry's source, e.g. left behind after an entry
	// was removed, renamed or moved to a new dest.
	StaleReasonOrphaned = "orphaned"
	// StaleReasonDangling marks a table dest whose symlink no longer resolves
	// because the entry's source was deleted from the repo; link cannot
	// repair it.
	StaleReasonDangling = "dangling"
)

// plasmaFilesDir is the repo-relative dir whose links belong to dotf plasma
// --link; prune never touches links into it.
const plasmaFilesDir = "plasma/files"

// StaleLink is one stale managed symlink found by FindStale.
type StaleLink struct {
	// Dest is the $HOME-relative link path in slash form (".config/foo").
	Dest string
	// DestAbs is filepath.Join(home, Dest).
	DestAbs string
	// Target is the raw os.Readlink value recorded when the link was found;
	// Prune removes the link only if it still reads exactly this.
	Target string
	// Reason is StaleReasonOrphaned or StaleReasonDangling.
	Reason string
}

// FindStale is read-only: it scans, one level deep, the parent dir of every
// spec's dest (specs is the full table; facets are ignored so turning a facet
// off never orphans its links) and returns the stale symlinks pointing into
// the repo, sorted by Dest. Only a link's first hop is considered: its raw
// target (a relative one joined onto the link's symlink-resolved dir) with
// the parent dirs resolved but the final component never followed, so a link
// to another managed link (~/.zshenv -> .zshrc) is not "into the repo". Links
// to the repo root itself and links under plasma/files are never stale, nor
// is a link at a strict ancestor of any table dest (~/.config/zed ->
// $REPO/zed, a supported parent-dir link). A link at a table dest is stale
// (StaleReasonDangling) only when it does not resolve and the entry's source
// is gone; a Copy dest holding a legacy symlink is not stale (link replaces
// it). Any other link into the repo is stale (StaleReasonOrphaned) only when
// its first hop does not exist or is the source of some table entry (a
// managed source linked from an old location); live links to any other repo
// path (~/.claude -> $REPO/.claude) are left alone. Scan dirs that do not
// exist or whose real path lies inside the repo (e.g. ~/.config/nvim ->
// $REPO/nvim) are skipped, so repo-tracked symlinks are never reported. A
// missing repo root yields a MissingSourceTreeError.
func (e *Engine) FindStale(specs []LinkSpec) ([]StaleLink, error) {
	rootReal, err := e.resolveRepoRoot()
	if err != nil {
		return nil, err
	}

	idx := staleIndex{
		byDest:    make(map[string]LinkSpec, len(specs)),
		ancestors: make(map[string]bool),
		srcs:      make(map[string]bool),
	}
	var dirs []string
	seenDir := make(map[string]bool)
	for _, spec := range specs {
		if err := validateDest(spec.Dest); err != nil {
			return nil, err
		}
		key := path.Clean(spec.Dest)
		if _, ok := idx.byDest[key]; !ok {
			idx.byDest[key] = spec
		}
		for a := path.Dir(key); a != "." && a != "/"; a = path.Dir(a) {
			idx.ancestors[a] = true
		}
		src := filepath.Join(e.repoRoot, filepath.FromSlash(spec.Src))
		idx.srcs[src] = true
		idx.srcs[noFollowPath(src)] = true
		if srcReal, err := filepath.EvalSymlinks(src); err == nil {
			idx.srcs[srcReal] = true
		}
		dir := filepath.Dir(filepath.Join(e.home, filepath.FromSlash(key)))
		if !seenDir[dir] {
			seenDir[dir] = true
			dirs = append(dirs, dir)
		}
	}

	var stale []StaleLink
	for _, dir := range dirs {
		found, err := e.scanStaleDir(dir, rootReal, idx)
		if err != nil {
			return nil, err
		}
		stale = append(stale, found...)
	}
	slices.SortFunc(stale, func(a, b StaleLink) int { return strings.Compare(a.Dest, b.Dest) })
	return stale, nil
}

// staleIndex is the table data FindStale classifies links against.
type staleIndex struct {
	// byDest maps each cleaned table dest to its (first) spec.
	byDest map[string]LinkSpec
	// ancestors holds every strict ancestor dir of a table dest
	// (".config", ".config/zed"); a link there is never stale.
	ancestors map[string]bool
	// srcs holds each table source path as joined onto the repo root, with
	// only its parents resolved, and fully resolved.
	srcs map[string]bool
}

// scanStaleDir classifies the symlinks directly inside dir. A dir that is
// missing or resolves inside the repo yields nothing.
func (e *Engine) scanStaleDir(dir, rootReal string, idx staleIndex) ([]StaleLink, error) {
	if !e.scanDirOutsideRepo(dir, rootReal) {
		return nil, nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			return nil, nil
		}
		return nil, fmt.Errorf("symlink: read dir %q: %w", dir, err)
	}
	var out []StaleLink
	for _, ent := range ents {
		// ReadDir types come from lstat, so a symlink reports ModeSymlink.
		if ent.Type()&fs.ModeSymlink == 0 {
			continue
		}
		linkAbs := filepath.Join(dir, ent.Name())
		raw, hop, err := readLinkTarget(linkAbs)
		if err != nil {
			continue
		}
		if !e.pointsIntoRepo(hop, rootReal) || e.protectedTarget(linkAbs, hop, rootReal) {
			continue
		}
		rel, err := filepath.Rel(e.home, linkAbs)
		if err != nil {
			continue
		}
		dest := filepath.ToSlash(rel)
		reason := StaleReasonOrphaned
		if spec, ok := idx.byDest[dest]; ok {
			if spec.Copy || !e.danglingSourceGone(spec, linkAbs) {
				continue
			}
			reason = StaleReasonDangling
		} else if idx.ancestors[dest] || !orphanedHop(hop, idx.srcs) {
			continue
		}
		out = append(out, StaleLink{Dest: dest, DestAbs: linkAbs, Target: raw, Reason: reason})
	}
	return out, nil
}

// orphanedHop reports whether a non-table link whose first hop is hop is an
// orphan: hop does not exist, or it is a table source (the managed source
// still linked from an old location). A live link to any other repo path is
// not an orphan.
func orphanedHop(hop string, srcs map[string]bool) bool {
	if _, err := os.Lstat(hop); err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	return srcs[hop]
}

// Prune removes each stale link, re-checking it immediately before removal:
// it must still be a symlink reading exactly the recorded Target, its dest
// must be a safe $HOME-relative path, and its parent dir must not resolve
// inside the repo. A link that changed, vanished, or became a real file is
// skipped silently. It returns the removed subset in input order and stops
// at the first removal error (the links already removed are still returned).
// A missing repo root yields a MissingSourceTreeError before any change.
func (e *Engine) Prune(stale []StaleLink) ([]StaleLink, error) {
	rootReal, err := e.resolveRepoRoot()
	if err != nil {
		return nil, err
	}
	var removed []StaleLink
	for _, s := range stale {
		if validateDest(s.Dest) != nil {
			continue
		}
		destAbs := filepath.Join(e.home, filepath.FromSlash(path.Clean(s.Dest)))
		if !e.scanDirOutsideRepo(filepath.Dir(destAbs), rootReal) || !stillLinksTo(destAbs, s.Target) {
			continue
		}
		if err := os.Remove(destAbs); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return removed, fmt.Errorf("symlink: remove stale link %q: %w", destAbs, err)
		}
		removed = append(removed, s)
	}
	return removed, nil
}

// resolveRepoRoot returns the repo root's real path, or a
// MissingSourceTreeError when it does not exist or cannot be resolved.
func (e *Engine) resolveRepoRoot() (string, error) {
	if e.repoRoot == "" {
		return "", &MissingSourceTreeError{Root: e.repoRoot}
	}
	rootReal, err := filepath.EvalSymlinks(e.repoRoot)
	if err != nil {
		return "", &MissingSourceTreeError{Root: e.repoRoot}
	}
	return rootReal, nil
}

// scanDirOutsideRepo reports whether dir exists and its real path lies
// outside the repo. A dir inside the repo holds repo-tracked symlinks that
// must never be removed.
func (e *Engine) scanDirOutsideRepo(dir, rootReal string) bool {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	return !within(rootReal, resolved) && !within(e.repoRoot, resolved)
}

// readLinkTarget returns linkAbs's raw target and its first hop: the target
// (a relative one joined onto the link's symlink-resolved dir, so ".."
// resolves physically) with its parent dirs resolved but its final component
// never followed (see noFollowPath).
func readLinkTarget(linkAbs string) (raw, hop string, err error) {
	raw, err = os.Readlink(linkAbs)
	if err != nil {
		return "", "", err
	}
	p := raw
	if !filepath.IsAbs(p) {
		dir := filepath.Dir(linkAbs)
		if dirReal, err := filepath.EvalSymlinks(dir); err == nil {
			dir = dirReal
		}
		p = dir + string(filepath.Separator) + p
	}
	return raw, noFollowPath(p), nil
}

// noFollowPath returns the absolute path p with its parent dirs
// symlink-resolved (".." handled physically) and its final component
// re-joined unresolved, so a symlink at p is never followed. An unresolvable
// parent falls back to the lexically cleaned path. A final "." or ".." names
// a dir, not a link, so the whole path is resolved.
func noFollowPath(p string) string {
	trimmed := strings.TrimRight(p, string(filepath.Separator))
	if trimmed == "" {
		return string(filepath.Separator)
	}
	i := strings.LastIndexByte(trimmed, filepath.Separator)
	parent, base := trimmed[:i+1], trimmed[i+1:]
	if base == "." || base == ".." {
		parent = trimmed
		base = ""
	}
	parentReal, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return filepath.Clean(trimmed)
	}
	return filepath.Join(parentReal, base)
}

// pointsIntoRepo reports whether a link's first hop (see readLinkTarget) lies
// within the repo root as spelled or its real path. The hop is not required
// to exist, so dangling links count.
func (e *Engine) pointsIntoRepo(hop, rootReal string) bool {
	return within(e.repoRoot, hop) || within(rootReal, hop)
}

// protectedTarget reports whether a link into the repo must never be pruned:
// it targets the repo root itself (how the repo may be reached, e.g.
// ~/.config/dotfiles -> ~/src/dotfiles) or lies under plasma/files (owned by
// dotf plasma --link). Both the first hop and the fully resolved link are
// checked against both spellings of the root.
func (e *Engine) protectedTarget(linkAbs, hop, rootReal string) bool {
	candidates := []string{hop}
	if resolved, err := filepath.EvalSymlinks(linkAbs); err == nil {
		candidates = append(candidates, resolved)
	}
	plasmaDir := filepath.FromSlash(plasmaFilesDir)
	for _, p := range candidates {
		for _, root := range []string{e.repoRoot, rootReal} {
			if p == root || within(filepath.Join(root, plasmaDir), p) {
				return true
			}
		}
	}
	return false
}

// danglingSourceGone reports whether the link at linkAbs does not resolve and
// spec's source no longer exists, i.e. a table link that link cannot repair.
func (e *Engine) danglingSourceGone(spec LinkSpec, linkAbs string) bool {
	if _, err := os.Stat(linkAbs); err == nil {
		return false
	}
	_, err := os.Stat(filepath.Join(e.repoRoot, spec.Src))
	return err != nil
}

// stillLinksTo reports whether destAbs is still a symlink whose raw target is
// exactly target.
func stillLinksTo(destAbs, target string) bool {
	fi, err := os.Lstat(destAbs)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return false
	}
	got, err := os.Readlink(destAbs)
	return err == nil && got == target
}
