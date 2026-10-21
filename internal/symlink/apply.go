package symlink

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"
)

const (
	// dirPerm is the mode for directories the engine creates (Ansible 0755).
	dirPerm os.FileMode = 0o755
	// zedMode is the mode for the copied Zed settings file (Ansible 0644).
	zedMode os.FileMode = 0o644
)

// Apply performs the plan's mutations in phase order (Backup, Remove, Link,
// Copy), skipping all writes when opts.DryRun. Before mutating it creates the
// backup dir (only when the plan holds at least one ActionBackup, so a run
// with nothing to back up leaves no empty dir) and the required parent dirs.
// The backup dir is created exclusively: if plan.BackupDir already exists (a
// second run within the same second) a _1, _2, … suffix is chosen instead, so
// an earlier run's copies are never overwritten; plan.BackupDir and
// report.BackupDir record the dir actually used. When opts.BackupDir is set
// (an earlier Apply's report.BackupDir in the same run) that existing dir is
// reused instead, and a dest already recorded in its manifest is an error
// raised before any write.
// Whenever anything was backed up — including when Apply fails partway — it
// writes (or, for a reused dir, merges into) the dir's BackupManifestName
// atomically, one entry per completed ActionBackup.
// Applied records only the mutations actually performed — ActionSkip no-ops
// are excluded, so a converged run reports applied=0; Planned always reflects
// the full plan (skips included). Applied is empty under DryRun.
func (e *Engine) Apply(plan *Plan, opts Options) (report *Report, err error) {
	report = &Report{Planned: plan.Actions, DryRun: opts.DryRun}
	if opts.DryRun {
		return report, nil
	}

	var manifest *BackupManifest
	fresh := false
	if plan.HasBackups() {
		dir, m, berr := prepareBackupDir(plan, opts.BackupDir)
		if berr != nil {
			return report, berr
		}
		plan.BackupDir = dir
		report.BackupDir = dir
		manifest = m
		fresh = opts.BackupDir == ""
	}
	before := 0
	if manifest != nil {
		before = len(manifest.Entries)
		defer func() {
			if len(manifest.Entries) == before {
				// Nothing was backed up (ensureRequiredDirs or the first copy
				// failed): drop a dir this run created rather than leave a
				// manifest-less legacy-looking set. os.Remove only succeeds
				// when it is empty, so a partial copy is never discarded.
				if fresh && os.Remove(plan.BackupDir) == nil {
					report.BackupDir = ""
				}
				return
			}
			err = errors.Join(err, writeManifest(plan.BackupDir, manifest))
		}()
	}
	if err := e.ensureRequiredDirs(); err != nil {
		return report, err
	}

	for _, act := range plan.Actions {
		if act.Kind == ActionBackup {
			if berr := e.backupAction(act, plan.BackupDir, plan.rootReal, manifest); berr != nil {
				return report, berr
			}
		} else if err := e.applyAction(act, plan.rootReal); err != nil {
			return report, err
		}
		if act.Kind != ActionSkip {
			report.Applied = append(report.Applied, act)
		}
	}
	return report, nil
}

// prepareBackupDir returns the backup dir for plan and the manifest to extend.
// With reuse empty it exclusively creates plan.BackupDir (or a _N sibling) and
// starts a fresh manifest. Otherwise reuse must be an existing real dir; its
// manifest (if any) is loaded and every planned backup dest is checked against
// it, so an earlier copy is never overwritten.
func prepareBackupDir(plan *Plan, reuse string) (string, *BackupManifest, error) {
	if reuse == "" {
		dir, err := createBackupDir(plan.BackupDir)
		if err != nil {
			return "", nil, err
		}
		return dir, &BackupManifest{Version: backupManifestVersion, CreatedAt: time.Now().UTC()}, nil
	}
	fi, err := os.Lstat(reuse)
	if err != nil {
		return "", nil, fmt.Errorf("symlink: reuse backup dir %q: %w", reuse, err)
	}
	if !fi.IsDir() {
		return "", nil, fmt.Errorf("symlink: reuse backup dir %q: not a real directory", reuse)
	}
	m, err := readManifest(reuse)
	if err != nil {
		return "", nil, err
	}
	if m == nil {
		m = &BackupManifest{Version: backupManifestVersion, CreatedAt: time.Now().UTC()}
	}
	seen := make(map[string]bool, len(m.Entries))
	for _, ent := range m.Entries {
		seen[path.Clean(ent.Dest)] = true
	}
	for _, act := range plan.Actions {
		if act.Kind != ActionBackup {
			continue
		}
		dest := path.Clean(act.Spec.Dest)
		if seen[dest] {
			return "", nil, fmt.Errorf("symlink: backup dir %q already holds a copy of %q", reuse, act.Spec.Dest)
		}
		seen[dest] = true
	}
	return reuse, m, nil
}

// maxBackupDirSuffix bounds the _N suffixes createBackupDir tries before
// giving up; reaching it means something other than same-second reruns is
// squatting on the names.
const maxBackupDirSuffix = 1000

// createBackupDir creates base, or base_1, base_2, … when it already exists,
// with os.Mkdir so an existing dir is never reused, and returns the path it
// created. The parent dir is created as needed.
func createBackupDir(base string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(base), dirPerm); err != nil {
		return "", fmt.Errorf("symlink: create backup dir parent %q: %w", filepath.Dir(base), err)
	}
	dir := base
	for i := 1; ; i++ {
		err := os.Mkdir(dir, dirPerm)
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, fs.ErrExist) || i > maxBackupDirSuffix {
			return "", fmt.Errorf("symlink: create backup dir %q: %w", dir, err)
		}
		dir = base + "_" + strconv.Itoa(i)
	}
}

// HasBackups reports whether the plan contains at least one ActionBackup.
func (p *Plan) HasBackups() bool {
	for _, act := range p.Actions {
		if act.Kind == ActionBackup {
			return true
		}
	}
	return false
}

// applyAction executes a single non-backup planned action against the
// filesystem; Apply routes ActionBackup through backupAction so it can record
// the manifest entry. Before any mutating step it re-checks that the dest does
// not resolve inside the repo (rootReal), closing the window in which a dest's
// parent could be swapped for a symlink into the repo after Plan ran.
func (e *Engine) applyAction(act Action, rootReal string) error {
	if act.Kind != ActionSkip {
		if err := guardOutsideRepo(act.Spec.Dest, act.DestAbs, rootReal); err != nil {
			return err
		}
	}
	switch act.Kind {

	case ActionRemove:
		if err := os.RemoveAll(act.DestAbs); err != nil {
			return fmt.Errorf("symlink: remove %q: %w", act.DestAbs, err)
		}
		return nil
	case ActionLink:
		return e.doLink(act)
	case ActionCopy:
		return e.doCopy(act)
	case ActionSkip:
		return nil
	default:
		return fmt.Errorf("symlink: unknown action kind %d", act.Kind)
	}
}

// backupAction copies act's real file or dir tree into backupDir via
// backupInto and appends its manifest entry, so every engine backup — link
// blockers and the differing Zed copy dest alike — is restorable. Like
// applyAction it first re-checks that neither the dest nor its backup copy
// resolves inside the repo.
//
// ActionBackup is planned only for a dest Plan's Lstat saw as neither missing
// nor a symlink: the link default: arm, or a differing regular file at a Copy
// dest. DestAbs is therefore never a symlink, satisfying backupInto.
func (e *Engine) backupAction(act Action, backupDir, rootReal string, manifest *BackupManifest) error {
	if err := guardOutsideRepo(act.Spec.Dest, act.DestAbs, rootReal); err != nil {
		return err
	}
	if err := guardOutsideRepo(act.Spec.Dest, filepath.Join(backupDir, act.Spec.Dest), rootReal); err != nil {
		return err
	}
	kind, err := backupInto(act.DestAbs, backupDir, act.Spec.Dest)
	if err != nil {
		return err
	}
	manifest.Entries = append(manifest.Entries, BackupEntry{
		Dest: path.Clean(act.Spec.Dest), Kind: kind, BackedUpAt: time.Now().UTC(),
	})
	return nil
}

// guardOutsideRepo returns a DestInRepoError when path, with its parent dirs
// symlink-resolved, lies inside rootReal. Plan already skips a dest that is its
// own source, so any planned mutation that now lands in the repo is refused.
// An empty rootReal (a Plan not built by Engine.Plan) disables the check.
func guardOutsideRepo(dest, path, rootReal string) error {
	if rootReal == "" {
		return nil
	}
	if loc, ok := resolvedLocation(path); ok && within(rootReal, loc) {
		return &DestInRepoError{Dest: dest, Resolved: loc}
	}
	return nil
}

// doLink creates or repoints a symlink Dest -> Src. By link time any real file
// has already been removed (Remove phase), so an existing Dest is a symlink
// that is unlinked before relinking.
func (e *Engine) doLink(act Action) error {
	if err := os.MkdirAll(filepath.Dir(act.DestAbs), dirPerm); err != nil {
		return fmt.Errorf("symlink: create parent dir for %q: %w", act.DestAbs, err)
	}
	if _, err := os.Lstat(act.DestAbs); err == nil {
		if err := os.Remove(act.DestAbs); err != nil {
			return fmt.Errorf("symlink: remove existing link %q: %w", act.DestAbs, err)
		}
	}
	if err := os.Symlink(act.SrcAbs, act.DestAbs); err != nil {
		return fmt.Errorf("symlink: link %q -> %q: %w", act.DestAbs, act.SrcAbs, err)
	}
	return nil
}

// doCopy implements the Zed exception: unlink a legacy symlink at the dest,
// then copy Src -> Dest (overwrite, 0644). Plan schedules it only for a dest
// that is missing, a symlink, or differing; a differing real file has already
// been saved by the Backup phase when backups are on.
func (e *Engine) doCopy(act Action) error {
	if fi, err := os.Lstat(act.DestAbs); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(act.DestAbs); err != nil {
			return fmt.Errorf("symlink: remove legacy symlink %q: %w", act.DestAbs, err)
		}
	}
	if err := copyFile(act.SrcAbs, act.DestAbs, zedMode); err != nil {
		return fmt.Errorf("symlink: copy %q -> %q: %w", act.SrcAbs, act.DestAbs, err)
	}
	return nil
}

// ensureRequiredDirs creates the load-bearing parent directories the linker
// depends on (os.Symlink does not create missing parents).
func (e *Engine) ensureRequiredDirs() error {
	for _, dir := range requiredDirs(e.home) {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return fmt.Errorf("symlink: create required dir %q: %w", dir, err)
		}
	}
	return nil
}

// requiredDirs returns the fixed list of directories the playbook pre-creates
// (symlink-semantics §6), covering the parent of every managed dest.
func requiredDirs(home string) []string {
	return []string{
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".zsh", "completions"),
		filepath.Join(home, ".config"),
		filepath.Join(home, ".branchlet"),
		filepath.Join(home, ".config", "atuin"),
		filepath.Join(home, ".config", "lazygit"),
		filepath.Join(home, ".config", "systemd", "user"),
		filepath.Join(home, ".config", "zed"),
		filepath.Join(home, ".local", "share", "applications"),
	}
}

// copyFile copies src to dst with the given mode, creating dst's parent
// directory. It overwrites an existing regular file at dst. Chmod enforces the
// mode exactly, independent of the process umask that WriteFile is subject to.
func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

// copyTree recursively copies the directory tree at src to dst, recreating
// directories, regular files (with their permission bits) and symlinks. It is
// the ActionBackup path for directory destinations (ADR-11).
func copyTree(src, dst string) error { return copyTreeWith(src, dst, copyFile) }

// copyTreeWith is copyTree with the regular-file copier injected, so restore
// can write files atomically while link/apply keep copyFile.
func copyTreeWith(src, dst string, copyRegular func(src, dst string, mode os.FileMode) error) error {
	// Directories are created with dirPerm during the walk (WalkDir visits a
	// parent before its children, so recreating a restrictive mode such as
	// 0500 up front would make copying that dir's children fail) and given
	// their source permission bits afterwards, deepest first.
	type dirMode struct {
		path string
		mode os.FileMode
	}
	var dirs []dirMode
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		switch {
		case d.IsDir():
			dirs = append(dirs, dirMode{target, info.Mode().Perm()})
			return os.MkdirAll(target, dirPerm)
		case info.Mode()&os.ModeSymlink != 0:
			// Recreate the link rather than follow it, so a plugin-link-heavy
			// config dir cannot fan out into an unbounded copy.
			link, lerr := os.Readlink(p)
			if lerr != nil {
				return lerr
			}
			if merr := os.MkdirAll(filepath.Dir(target), dirPerm); merr != nil {
				return merr
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			return copyRegular(p, target, info.Mode().Perm())
		default:
			// Sockets, fifos and device nodes carry no configuration state and
			// cannot be meaningfully recreated; skip them.
			return nil
		}
	})
	if err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if cerr := os.Chmod(dirs[i].path, dirs[i].mode); cerr != nil {
			return cerr
		}
	}
	return nil
}
