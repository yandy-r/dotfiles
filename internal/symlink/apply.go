package symlink

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// dirPerm is the mode for directories the engine creates (Ansible 0755).
	dirPerm os.FileMode = 0o755
	// zedMode is the mode for the copied Zed settings file (Ansible 0644).
	zedMode os.FileMode = 0o644
)

// Apply performs the plan's mutations in phase order (Backup, Remove, Link,
// Copy), skipping all writes when opts.DryRun. Before mutating it creates the
// backup dir (when opts.Backup) and the required parent dirs. Applied records
// only the mutations actually performed — ActionSkip no-ops are excluded, so a
// converged run reports applied=0; Planned always reflects the full plan
// (skips included). Applied is empty under DryRun.
func (e *Engine) Apply(plan *Plan, opts Options) (*Report, error) {
	report := &Report{Planned: plan.Actions, DryRun: opts.DryRun}
	if opts.DryRun {
		return report, nil
	}

	if opts.Backup {
		if err := os.MkdirAll(plan.BackupDir, dirPerm); err != nil {
			return report, fmt.Errorf("symlink: create backup dir %q: %w", plan.BackupDir, err)
		}
	}
	if err := e.ensureRequiredDirs(); err != nil {
		return report, err
	}

	for _, act := range plan.Actions {
		if err := e.applyAction(act, plan.BackupDir); err != nil {
			return report, err
		}
		if act.Kind != ActionSkip {
			report.Applied = append(report.Applied, act)
		}
	}
	return report, nil
}

// applyAction executes a single planned action against the filesystem.
func (e *Engine) applyAction(act Action, backupDir string) error {
	switch act.Kind {
	case ActionBackup:
		return e.doBackup(act, backupDir)
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

// doBackup copies the real file OR directory tree at act.DestAbs into the
// backup dir, preserving its $HOME-relative path and permission bits. It runs
// before any Remove so the original bytes are still present (ADR-11: the
// managed app-config directories are backup-eligible, and ActionRemove is
// recursive for dirs).
//
// ActionBackup is only ever planned from symlink.go's default: Lstat arm, which
// is reached only when the dest is neither missing nor a symlink, so DestAbs is
// never a symlink and os.Stat following links is not a hazard here.
func (e *Engine) doBackup(act Action, backupDir string) error {
	info, err := os.Stat(act.DestAbs)
	if err != nil {
		return fmt.Errorf("symlink: stat backup source %q: %w", act.DestAbs, err)
	}
	dst := filepath.Join(backupDir, act.Spec.Dest)
	if info.IsDir() {
		if err := copyTree(act.DestAbs, dst); err != nil {
			return fmt.Errorf("symlink: backup %q: %w", act.DestAbs, err)
		}
		return nil
	}
	if err := copyFile(act.DestAbs, dst, info.Mode().Perm()); err != nil {
		return fmt.Errorf("symlink: backup %q: %w", act.DestAbs, err)
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
// then copy Src -> Dest (overwrite, 0644).
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
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
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
			// dirPerm, not the source dir's own mode: WalkDir visits a parent
			// before its children, so recreating a restrictive mode (e.g. 0500)
			// would make the copy of that dir's children fail.
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
			return copyFile(p, target, info.Mode().Perm())
		default:
			// Sockets, fifos and device nodes carry no configuration state and
			// cannot be meaningfully recreated; skip them.
			return nil
		}
	})
}
