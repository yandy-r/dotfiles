package symlink

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// BackupDirPrefix is the basename prefix of every backup dir under
	// BackupRoot: <home>/.config/dotfiles_backup_<epoch>[_N].
	BackupDirPrefix = "dotfiles_backup_"
	// BackupManifestName is the manifest file Apply writes into a backup dir.
	BackupManifestName = ".dotf-backup.json"
	// DefaultBackupKeep is how many of the newest backup sets prune keeps by
	// default.
	DefaultBackupKeep = 10
)

// BackupKind values recorded per manifest entry.
const (
	// BackupKindFile is a backed-up regular file.
	BackupKindFile = "file"
	// BackupKindDir is a backed-up directory tree.
	BackupKindDir = "dir"
)

// backupManifestVersion is the only manifest schema version this package
// reads and writes.
const backupManifestVersion = 1

// maxManifestSize caps how many bytes readManifest reads; a larger manifest is
// treated as invalid (the set is listed as legacy).
const maxManifestSize = 1 << 20

// backupNameRE matches backup dir basenames; submatch 1 is the epoch, 2 the
// optional same-second _N suffix.
var backupNameRE = regexp.MustCompile(`^` + BackupDirPrefix + `([0-9]+)(?:_([0-9]+))?$`)

// backupNameFoldRE is backupNameRE matched case-insensitively, for refusing
// restore dests that would alias a backup dir on a case-insensitive filesystem.
var backupNameFoldRE = regexp.MustCompile(`(?i)` + backupNameRE.String())

// BackupEntry is one backed-up dest recorded in a BackupManifest.
type BackupEntry struct {
	// Dest is the $HOME-relative dest in slash form, validated like
	// LinkSpec.Dest; its copy lives at the same relative path in the dir.
	Dest string `json:"dest"`
	// Kind is BackupKindFile or BackupKindDir.
	Kind string `json:"kind"`
	// BackedUpAt is when the copy was taken.
	BackedUpAt time.Time `json:"backed_up_at"`
}

// BackupManifest is the BackupManifestName document in a backup dir.
type BackupManifest struct {
	// Version is the schema version (1).
	Version int `json:"version"`
	// CreatedAt is when the manifest was first written.
	CreatedAt time.Time `json:"created_at"`
	// Entries lists every backed-up dest, in backup order.
	Entries []BackupEntry `json:"entries"`
}

// BackupSet is one backup dir on disk.
type BackupSet struct {
	// ID is the basename minus BackupDirPrefix, e.g. "1729543200" or
	// "1729543200_1".
	ID string
	// Path is the dir path, filepath.Join(BackupRoot(home), BackupDirPrefix+ID).
	Path string
	// CreatedAt is the manifest CreatedAt, else the epoch parsed from the name.
	CreatedAt time.Time
	// Legacy is true when the dir has no readable manifest.
	Legacy bool
	// Entries are the manifest entries; nil when Legacy.
	Entries []BackupEntry
	// Size is the total bytes of regular files under Path, the manifest
	// included (best effort: unreadable subtrees are skipped).
	Size int64
}

// BackupNotFoundError is returned when no backup dir matches an ID.
type BackupNotFoundError struct {
	// ID is the requested backup ID.
	ID string
}

// Error implements the error interface.
func (e *BackupNotFoundError) Error() string {
	return fmt.Sprintf("symlink: backup %q not found", e.ID)
}

// LegacyBackupError is returned when restoring a backup dir that has no
// readable manifest: without one the original dests and kinds are unknown.
type LegacyBackupError struct {
	// ID is the legacy backup's ID.
	ID string
}

// Error implements the error interface.
func (e *LegacyBackupError) Error() string {
	return fmt.Sprintf("symlink: backup %q has no manifest (legacy); restore its files manually", e.ID)
}

// RestoreConflictError is returned when restore targets are real files or
// directories and Force was not set.
type RestoreConflictError struct {
	// Dests are the $HOME-relative dests holding real content.
	Dests []string
}

// Error implements the error interface.
func (e *RestoreConflictError) Error() string {
	quoted := make([]string, len(e.Dests))
	for i, d := range e.Dests {
		quoted[i] = strconv.Quote(d)
	}
	return fmt.Sprintf("symlink: refusing to overwrite real file(s) %s; rerun with --force to back them up and replace them",
		strings.Join(quoted, ", "))
}

// BackupRoot returns the dir holding the backup dirs: <home>/.config.
func BackupRoot(home string) string { return filepath.Join(home, ".config") }

// newBackupBase returns the unsuffixed backup dir path for now; Apply's
// createBackupDir adds a _N suffix when it is taken.
func newBackupBase(home string, now time.Time) string {
	return filepath.Join(BackupRoot(home), BackupDirPrefix+strconv.FormatInt(now.Unix(), 10))
}

// ListBackups returns every backup set under BackupRoot(home), newest first
// (CreatedAt descending, then epoch and suffix descending). Only real dirs
// (not symlinks) whose basename matches dotfiles_backup_<epoch>[_N] are
// listed; a missing root yields an empty list.
func ListBackups(home string) ([]BackupSet, error) {
	root := BackupRoot(home)
	ents, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("symlink: list backups in %q: %w", root, err)
	}
	var sets []BackupSet
	for _, ent := range ents {
		// ReadDir types come from lstat, so a symlinked dir is not IsDir.
		if !ent.IsDir() || !backupNameRE.MatchString(ent.Name()) {
			continue
		}
		sets = append(sets, loadBackupSet(root, ent.Name()))
	}
	slices.SortStableFunc(sets, func(a, b BackupSet) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		ae, as := parseBackupID(a.ID)
		be, bs := parseBackupID(b.ID)
		return cmp.Or(cmp.Compare(be, ae), cmp.Compare(bs, as))
	})
	return sets, nil
}

// FindBackup returns the backup set named by id, which may be the bare ID or
// the full basename. IDs containing path separators are rejected; an absent
// or non-dir match yields *BackupNotFoundError.
func FindBackup(home, id string) (BackupSet, error) {
	if strings.ContainsAny(id, `/\`) {
		return BackupSet{}, fmt.Errorf("symlink: invalid backup id %q: must not contain path separators", id)
	}
	name := id
	if !strings.HasPrefix(name, BackupDirPrefix) {
		name = BackupDirPrefix + name
	}
	if !backupNameRE.MatchString(name) {
		return BackupSet{}, &BackupNotFoundError{ID: id}
	}
	root := BackupRoot(home)
	fi, err := os.Lstat(filepath.Join(root, name))
	if err != nil || !fi.IsDir() {
		return BackupSet{}, &BackupNotFoundError{ID: id}
	}
	return loadBackupSet(root, name), nil
}

// loadBackupSet builds the BackupSet for the dir root/name (name already
// matched backupNameRE).
func loadBackupSet(root, name string) BackupSet {
	id := strings.TrimPrefix(name, BackupDirPrefix)
	set := BackupSet{ID: id, Path: filepath.Join(root, name)}
	if m, err := readManifest(set.Path); err == nil && m != nil {
		set.CreatedAt = m.CreatedAt
		set.Entries = m.Entries
		if set.Entries == nil {
			set.Entries = []BackupEntry{}
		}
	} else {
		epoch, _ := parseBackupID(id)
		set.CreatedAt = time.Unix(epoch, 0).UTC()
		set.Legacy = true
	}
	set.Size = dirSize(set.Path)
	return set
}

// parseBackupID splits a backup ID into its epoch and _N suffix (0 when
// absent). Unparseable parts yield 0.
func parseBackupID(id string) (epoch, suffix int64) {
	m := backupNameRE.FindStringSubmatch(BackupDirPrefix + id)
	if m == nil {
		return 0, 0
	}
	epoch, _ = strconv.ParseInt(m[1], 10, 64)
	if m[2] != "" {
		suffix, _ = strconv.ParseInt(m[2], 10, 64)
	}
	return epoch, suffix
}

// dirSize sums the sizes of regular files under dir without following
// symlinks, skipping anything it cannot read.
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			if info, ierr := d.Info(); ierr == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// readManifest reads dir's manifest. A missing manifest returns (nil, nil);
// anything that is not a regular file (a symlink, FIFO, device...), a manifest
// larger than maxManifestSize, an unparseable one, or an unsupported version
// returns an error.
func readManifest(dir string) (*BackupManifest, error) {
	p := filepath.Join(dir, BackupManifestName)
	lfi, err := os.Lstat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("symlink: read backup manifest %q: %w", p, err)
	}
	if !lfi.Mode().IsRegular() {
		return nil, fmt.Errorf("symlink: backup manifest %q is not a regular file", p)
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("symlink: read backup manifest %q: %w", p, err)
	}
	defer func() { _ = f.Close() }()
	// Re-check the opened file is the one Lstat saw, so a swap to a symlink
	// or FIFO between the two calls is not followed.
	ofi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("symlink: read backup manifest %q: %w", p, err)
	}
	if !ofi.Mode().IsRegular() || !os.SameFile(lfi, ofi) {
		return nil, fmt.Errorf("symlink: backup manifest %q changed while opening", p)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxManifestSize+1))
	if err != nil {
		return nil, fmt.Errorf("symlink: read backup manifest %q: %w", p, err)
	}
	if len(data) > maxManifestSize {
		return nil, fmt.Errorf("symlink: backup manifest %q exceeds %d bytes", p, maxManifestSize)
	}
	var m BackupManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("symlink: parse backup manifest %q: %w", p, err)
	}
	if m.Version != backupManifestVersion {
		return nil, fmt.Errorf("symlink: backup manifest %q: unsupported version %d", p, m.Version)
	}
	return &m, nil
}

// writeManifest atomically replaces dir's manifest: it writes a temp file in
// dir, then renames it over BackupManifestName, so readers never observe a
// partial document.
func writeManifest(dir string, m *BackupManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("symlink: encode backup manifest: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, BackupManifestName+".*.tmp")
	if err != nil {
		return fmt.Errorf("symlink: write backup manifest in %q: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("symlink: write backup manifest %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("symlink: write backup manifest %q: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("symlink: chmod backup manifest %q: %w", tmpName, err)
	}
	final := filepath.Join(dir, BackupManifestName)
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("symlink: write backup manifest %q: %w", final, err)
	}
	return nil
}

// backupInto copies the real file or directory tree at srcAbs to
// backupDir/dest, preserving permission bits, and returns the entry kind.
// srcAbs must not be a symlink (os.Stat follows links).
func backupInto(srcAbs, backupDir, dest string) (string, error) {
	info, err := os.Stat(srcAbs)
	if err != nil {
		return "", fmt.Errorf("symlink: stat backup source %q: %w", srcAbs, err)
	}
	dst := filepath.Join(backupDir, filepath.FromSlash(dest))
	if info.IsDir() {
		if err := copyTree(srcAbs, dst); err != nil {
			return "", fmt.Errorf("symlink: backup %q: %w", srcAbs, err)
		}
		return BackupKindDir, nil
	}
	if err := copyFile(srcAbs, dst, info.Mode().Perm()); err != nil {
		return "", fmt.Errorf("symlink: backup %q: %w", srcAbs, err)
	}
	return BackupKindFile, nil
}

// RestoreOptions select and gate a Restore.
type RestoreOptions struct {
	// Paths is the subset of entry Dests to restore (cleaned, a leading "~/"
	// stripped); empty restores every entry. An unknown path is an error
	// raised before any write.
	Paths []string
	// DryRun returns the plan without writing.
	DryRun bool
	// Force replaces real files/dirs at the dests after first copying them
	// into a fresh backup dir.
	Force bool
}

// Restore actions recorded in RestoreItem.Action.
const (
	// RestoreActionCreate restores into a missing dest.
	RestoreActionCreate = "create"
	// RestoreActionReplaceSymlink replaces a symlink at the dest.
	RestoreActionReplaceSymlink = "replace-symlink"
	// RestoreActionReplaceReal replaces a real file/dir (only with Force).
	RestoreActionReplaceReal = "replace-real"
	// RestoreActionConflict marks a real file/dir that blocks a non-Force
	// restore.
	RestoreActionConflict = "conflict"
)

// RestoreItem is one planned (or performed) restore of a manifest entry.
type RestoreItem struct {
	// Dest is the entry's $HOME-relative dest.
	Dest string
	// DestAbs is filepath.Join(home, Dest).
	DestAbs string
	// SrcAbs is the backed-up copy inside the backup dir.
	SrcAbs string
	// Kind is the entry kind (BackupKindFile or BackupKindDir).
	Kind string
	// Action is one of the RestoreAction* values.
	Action string
}

// RestoreReport is the outcome of Restore.
type RestoreReport struct {
	// Items are the selected entries in manifest order.
	Items []RestoreItem
	// DryRun records whether nothing was written.
	DryRun bool
	// SafetyBackupDir is the fresh backup dir holding the real content Force
	// replaced, else empty.
	SafetyBackupDir string
}

// Restore copies the backup set id's manifest entries (or the opts.Paths
// subset) back into $HOME. It plans every selected entry first; a legacy set
// (*LegacyBackupError), an unsafe entry (*UnsafeDestError: textually, or
// because the dest resolves through symlinks or case aliases onto the backup
// root or into a backup dir; or a backed-up copy resolving outside the backup
// dir), an unknown path, or any "conflict" without Force
// (*RestoreConflictError, returned with the plan) aborts before any write.
// DryRun returns the plan. With Force, real dests are first copied into a
// fresh backup dir with its own manifest, then removed and restored. Each
// dest is re-checked against its planned action just before it is touched,
// and restored files are written via a temp file renamed into place,
// preserving permission bits; the backup dir itself is never modified.
// When the Engine has an existing repo root, a dest whose parent dirs resolve
// inside the repo is refused (*DestInRepoError) at plan time and again just
// before it is written, so a restore never modifies repo files; a symlink at
// the dest itself is replaced, never written through. A missing repo root
// disables that guard (there are no repo files to protect), so restore still
// works as a rollback after the repo was moved or deleted.
func (e *Engine) Restore(id string, opts RestoreOptions) (*RestoreReport, error) {
	rootReal := ""
	if e.repoRoot != "" {
		r, err := filepath.EvalSymlinks(e.repoRoot)
		switch {
		case err == nil:
			rootReal = r
		case !errors.Is(err, fs.ErrNotExist):
			return nil, fmt.Errorf("symlink: resolve repo root %q: %w", e.repoRoot, err)
		}
	}
	set, err := FindBackup(e.home, id)
	if err != nil {
		return nil, err
	}
	if set.Legacy {
		return nil, &LegacyBackupError{ID: set.ID}
	}
	entries, err := selectEntries(set, opts.Paths)
	if err != nil {
		return nil, err
	}
	report := &RestoreReport{DryRun: opts.DryRun}
	var conflicts []string
	for _, ent := range entries {
		item, err := e.planRestore(set, ent, opts.Force, rootReal)
		if err != nil {
			return nil, err
		}
		if item.Action == RestoreActionConflict {
			conflicts = append(conflicts, item.Dest)
		}
		report.Items = append(report.Items, item)
	}
	if len(conflicts) > 0 {
		return report, &RestoreConflictError{Dests: conflicts}
	}
	if opts.DryRun {
		return report, nil
	}
	// A failure from here on returns only the items actually restored (none
	// when the safety backup fails), plus SafetyBackupDir, so callers never
	// report an unperformed restore.
	if err := e.safetyBackup(report); err != nil {
		report.Items = report.Items[:0]
		return report, err
	}
	for i, item := range report.Items {
		if err := restoreItem(item, rootReal); err != nil {
			report.Items = report.Items[:i]
			return report, err
		}
	}
	return report, nil
}

// selectEntries returns the manifest entries named by paths (all when empty),
// in manifest order. An unknown path is an error.
func selectEntries(set BackupSet, paths []string) ([]BackupEntry, error) {
	if len(paths) == 0 {
		return set.Entries, nil
	}
	want := make(map[string]bool, len(paths))
	for _, p := range paths {
		want[normalizeRestorePath(p)] = true
	}
	known := make(map[string]bool, len(set.Entries))
	var out []BackupEntry
	for _, ent := range set.Entries {
		key := path.Clean(ent.Dest)
		known[key] = true
		if want[key] {
			out = append(out, ent)
		}
	}
	for _, p := range paths {
		if !known[normalizeRestorePath(p)] {
			return nil, fmt.Errorf("symlink: path %q is not in backup %q", p, set.ID)
		}
	}
	return out, nil
}

// normalizeRestorePath maps a user-supplied restore path onto manifest Dest
// form: slash-separated, cleaned, with a leading "~/" stripped.
func normalizeRestorePath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "~/")
	return path.Clean(p)
}

// planRestore validates one entry and classifies its dest.
func (e *Engine) planRestore(set BackupSet, ent BackupEntry, force bool, rootReal string) (RestoreItem, error) {
	if err := validateRestoreDest(ent.Dest); err != nil {
		return RestoreItem{}, err
	}
	if ent.Kind != BackupKindFile && ent.Kind != BackupKindDir {
		return RestoreItem{}, fmt.Errorf("symlink: backup %q entry %q has unknown kind %q", set.ID, ent.Dest, ent.Kind)
	}
	rel := filepath.FromSlash(path.Clean(ent.Dest))
	item := RestoreItem{
		Dest:    ent.Dest,
		DestAbs: filepath.Join(e.home, rel),
		SrcAbs:  filepath.Join(set.Path, rel),
		Kind:    ent.Kind,
	}
	if err := checkBackupSource(set.Path, item.SrcAbs, ent.Kind); err != nil {
		return RestoreItem{}, err
	}
	if err := checkRestoreDestFS(e.home, ent.Dest, item.DestAbs); err != nil {
		return RestoreItem{}, err
	}
	if err := guardOutsideRepo(ent.Dest, item.DestAbs, rootReal); err != nil {
		return RestoreItem{}, err
	}

	fi, err := os.Lstat(item.DestAbs)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		item.Action = RestoreActionCreate
	case err != nil:
		return RestoreItem{}, fmt.Errorf("symlink: stat restore dest %q: %w", item.DestAbs, err)
	case fi.Mode()&os.ModeSymlink != 0:
		item.Action = RestoreActionReplaceSymlink
	case force:
		item.Action = RestoreActionReplaceReal
	default:
		item.Action = RestoreActionConflict
	}
	return item, nil
}

// validateRestoreDest extends validateDest for manifest entries: the dest
// must name something below $HOME (not $HOME itself), must be free of control
// characters, must not be the backup root (.config, an ancestor of every
// backup dir) and must not land inside a backup dir, so a restore can never
// write into the backups it reads. The .config and backup-name checks ignore
// case so a case-insensitive filesystem cannot alias them;
// checkRestoreDestFS covers symlinked aliases.
func validateRestoreDest(dest string) error {
	if err := validateDest(dest); err != nil {
		return err
	}
	for i := range len(dest) {
		if dest[i] < 0x20 || dest[i] == 0x7f {
			return &UnsafeDestError{Dest: dest}
		}
	}
	clean := path.Clean(dest)
	if dest == "" || clean == "." || strings.EqualFold(clean, ".config") || strings.ContainsRune(dest, '\\') {
		return &UnsafeDestError{Dest: dest}
	}
	parts := strings.Split(clean, "/")
	if len(parts) >= 2 && strings.EqualFold(parts[0], ".config") && backupNameFoldRE.MatchString(parts[1]) {
		return &UnsafeDestError{Dest: dest}
	}
	return nil
}

// checkRestoreDestFS is the filesystem half of validateRestoreDest: it
// resolves the deepest existing ancestor of destAbs through symlinks and
// refuses (*UnsafeDestError) when the effective dest equals or contains
// BackupRoot(home), or lies inside a backup dir under it. Path identity is
// compared with os.SameFile, so case-insensitive aliases are caught too. A
// symlink at destAbs itself is not followed: restore replaces the link, it
// never writes through it.
func checkRestoreDestFS(home, dest, destAbs string) error {
	rootFI, err := os.Stat(BackupRoot(home))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("symlink: stat backup root %q: %w", BackupRoot(home), err)
	}
	rootResolved, err := filepath.EvalSymlinks(BackupRoot(home))
	if err != nil {
		return fmt.Errorf("symlink: resolve backup root %q: %w", BackupRoot(home), err)
	}

	// Find the deepest existing ancestor of destAbs and resolve it; the
	// missing tail components are re-appended verbatim.
	anc := filepath.Dir(destAbs)
	rest := []string{filepath.Base(destAbs)}
	for {
		_, lerr := os.Lstat(anc)
		if lerr == nil {
			break
		}
		if !errors.Is(lerr, fs.ErrNotExist) {
			return fmt.Errorf("symlink: stat restore dest ancestor %q: %w", anc, lerr)
		}
		parent := filepath.Dir(anc)
		if parent == anc {
			break
		}
		rest = append([]string{filepath.Base(anc)}, rest...)
		anc = parent
	}
	resolvedAnc, err := filepath.EvalSymlinks(anc)
	if err != nil {
		return fmt.Errorf("symlink: resolve restore dest ancestor %q: %w", anc, err)
	}
	eff := filepath.Join(append([]string{resolvedAnc}, rest...)...)

	// Equal or inside: some existing ancestor of eff (eff included) is the
	// backup root, and eff is the root itself or below a backup-named dir.
	for p := eff; ; p = filepath.Dir(p) {
		if fi, lerr := os.Lstat(p); lerr == nil && fi.IsDir() && os.SameFile(fi, rootFI) {
			rel, rerr := filepath.Rel(p, eff)
			if rerr != nil || rel == "." || backupNameFoldRE.MatchString(strings.Split(filepath.ToSlash(rel), "/")[0]) {
				return &UnsafeDestError{Dest: dest}
			}
		}
		if filepath.Dir(p) == p {
			break
		}
	}

	// Contains: eff is an existing real dir that is the backup root or one of
	// its ancestors.
	effFI, err := os.Lstat(eff)
	if err != nil || !effFI.IsDir() {
		return nil
	}
	for p := rootResolved; ; p = filepath.Dir(p) {
		if fi, serr := os.Stat(p); serr == nil && os.SameFile(fi, effFI) {
			return &UnsafeDestError{Dest: dest}
		}
		if filepath.Dir(p) == p {
			return nil
		}
	}
}

// checkBackupSource verifies the backed-up copy at srcAbs exists with the
// entry's kind, is not itself a symlink, and resolves (through any symlinked
// path components) to a location strictly inside the backup dir.
func checkBackupSource(setPath, srcAbs, kind string) error {
	fi, err := os.Lstat(srcAbs)
	if err != nil {
		return fmt.Errorf("symlink: backup copy %q: %w", srcAbs, err)
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("symlink: backup copy %q is a symlink; refusing to restore through it", srcAbs)
	case kind == BackupKindDir && !fi.IsDir():
		return fmt.Errorf("symlink: backup copy %q is not a directory", srcAbs)
	case kind == BackupKindFile && !fi.Mode().IsRegular():
		return fmt.Errorf("symlink: backup copy %q is not a regular file", srcAbs)
	}
	root, err := filepath.EvalSymlinks(setPath)
	if err != nil {
		return fmt.Errorf("symlink: resolve backup dir %q: %w", setPath, err)
	}
	resolved, err := filepath.EvalSymlinks(srcAbs)
	if err != nil {
		return fmt.Errorf("symlink: resolve backup copy %q: %w", srcAbs, err)
	}
	if !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return fmt.Errorf("symlink: backup copy %q resolves outside the backup dir", srcAbs)
	}
	return nil
}

// safetyBackup copies every replace-real dest into a fresh backup dir with a
// manifest before Restore removes it, recording the dir in the report. It is
// a no-op when nothing real is replaced.
func (e *Engine) safetyBackup(report *RestoreReport) error {
	var reals []RestoreItem
	for _, item := range report.Items {
		if item.Action == RestoreActionReplaceReal {
			reals = append(reals, item)
		}
	}
	if len(reals) == 0 {
		return nil
	}
	now := time.Now().UTC()
	dir, err := createBackupDir(newBackupBase(e.home, now))
	if err != nil {
		return err
	}
	report.SafetyBackupDir = dir
	m := &BackupManifest{Version: backupManifestVersion, CreatedAt: now}
	var errs []error
	for _, item := range reals {
		kind, err := backupInto(item.DestAbs, dir, item.Dest)
		if err != nil {
			errs = append(errs, err)
			break
		}
		m.Entries = append(m.Entries, BackupEntry{Dest: item.Dest, Kind: kind, BackedUpAt: time.Now().UTC()})
	}
	if err := writeManifest(dir, m); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// recheckRestoreDest re-Lstats item.DestAbs immediately before restoreItem
// acts on it and errors when its type no longer matches the planned action,
// so a dest swapped since planning is never removed or overwritten.
func recheckRestoreDest(item RestoreItem) error {
	fi, err := os.Lstat(item.DestAbs)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("symlink: stat restore dest %q: %w", item.DestAbs, err)
	}
	var ok bool
	switch item.Action {
	case RestoreActionCreate:
		ok = !exists
	case RestoreActionReplaceSymlink:
		ok = exists && fi.Mode()&os.ModeSymlink != 0
	case RestoreActionReplaceReal:
		ok = exists && fi.Mode()&os.ModeSymlink == 0
	}
	if !ok {
		return fmt.Errorf("symlink: restore dest %q changed since planning (planned %s); refusing to continue", item.DestAbs, item.Action)
	}
	return nil
}

// restoreItem replaces item.DestAbs with a copy of item.SrcAbs after
// re-verifying the dest still matches the planned action and does not
// resolve inside the repo (rootReal; empty disables that check). Regular
// files are written to a temp file in the target dir and renamed into place.
func restoreItem(item RestoreItem, rootReal string) error {
	if err := guardOutsideRepo(item.Dest, item.DestAbs, rootReal); err != nil {
		return err
	}
	if err := recheckRestoreDest(item); err != nil {
		return err
	}
	switch item.Action {
	case RestoreActionReplaceSymlink:
		if err := os.Remove(item.DestAbs); err != nil {
			return fmt.Errorf("symlink: remove symlink %q: %w", item.DestAbs, err)
		}
	case RestoreActionReplaceReal:
		if err := os.RemoveAll(item.DestAbs); err != nil {
			return fmt.Errorf("symlink: remove %q: %w", item.DestAbs, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(item.DestAbs), dirPerm); err != nil {
		return fmt.Errorf("symlink: create parent dir for %q: %w", item.DestAbs, err)
	}
	if item.Kind == BackupKindDir {
		if err := copyTreeWith(item.SrcAbs, item.DestAbs, copyFileAtomic); err != nil {
			return fmt.Errorf("symlink: restore %q: %w", item.DestAbs, err)
		}
		return nil
	}
	info, err := os.Stat(item.SrcAbs)
	if err != nil {
		return fmt.Errorf("symlink: stat backup copy %q: %w", item.SrcAbs, err)
	}
	if err := copyFileAtomic(item.SrcAbs, item.DestAbs, info.Mode().Perm()); err != nil {
		return fmt.Errorf("symlink: restore %q: %w", item.DestAbs, err)
	}
	return nil
}

// copyFileAtomic copies src to dst with mode exactly, writing a temp file in
// dst's dir and renaming it over dst, so dst is never opened for writing
// (a symlink planted at dst is replaced, not followed). It creates dst's
// parent dir.
func copyFileAtomic(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(dst)+".dotf-restore-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// PruneOptions select which backup sets SelectPrune returns.
type PruneOptions struct {
	// Keep is how many of the newest sets to retain; 0 keeps none by count.
	// Negative is invalid (callers validate); it is treated as 0.
	Keep int
	// OlderThan, when > 0, limits candidates to sets whose CreatedAt is before
	// Now-OlderThan; 0 disables the age filter.
	OlderThan time.Duration
	// Now is the reference time for OlderThan.
	Now time.Time
}

// SelectPrune returns the sets to delete: those (sets assumed newest first, as
// ListBackups returns them) beyond index Keep, intersected with the age filter
// when OlderThan is set.
func SelectPrune(sets []BackupSet, opts PruneOptions) []BackupSet {
	keep := max(opts.Keep, 0)
	cutoff := opts.Now.Add(-opts.OlderThan)
	var out []BackupSet
	for i, s := range sets {
		if i < keep {
			continue
		}
		if opts.OlderThan > 0 && !s.CreatedAt.Before(cutoff) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// RemoveBackup deletes set's dir after re-verifying that its basename matches
// dotfiles_backup_<epoch>[_N] and that it is a real dir (not a symlink)
// directly under BackupRoot(home). Dirs inside the set get owner rwx first
// (symlinks are never followed), so read-only dirs copied from $HOME do not
// make the removal fail.
func RemoveBackup(home string, set BackupSet) error {
	p := filepath.Clean(set.Path)
	if !backupNameRE.MatchString(filepath.Base(p)) {
		return fmt.Errorf("symlink: refusing to remove %q: not a backup dir name", set.Path)
	}
	if filepath.Dir(p) != filepath.Clean(BackupRoot(home)) {
		return fmt.Errorf("symlink: refusing to remove %q: not directly under %q", set.Path, BackupRoot(home))
	}
	fi, err := os.Lstat(p)
	if err != nil {
		return fmt.Errorf("symlink: remove backup %q: %w", set.Path, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("symlink: refusing to remove %q: not a real directory", set.Path)
	}
	// Best effort: WalkDir reports entries with Lstat semantics and visits a
	// dir before reading it, so chmod-ing here lets it descend; any remaining
	// failure surfaces from RemoveAll.
	_ = filepath.WalkDir(p, func(sub string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			if info, ierr := d.Info(); ierr == nil && info.Mode().Perm()&0o700 != 0o700 {
				_ = os.Chmod(sub, info.Mode().Perm()|0o700)
			}
		}
		return nil
	})
	if err := os.RemoveAll(p); err != nil {
		return fmt.Errorf("symlink: remove backup %q: %w", set.Path, err)
	}
	return nil
}
