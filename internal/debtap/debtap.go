// Package debtap converts Debian .deb packages into pacman-installable Arch
// packages using the AUR `debtap` tool, verifying the input, the produced
// artifact, and the installed result at each stage.
//
// The pipeline is exposed as discrete methods rather than one Run entry point
// so the command layer can report progress between stages and interpose an
// interactive confirmation before the single step that elevates.
//
// Every external command runs through exec.Runner with an argv slice, never a
// shell, so a hostile filename or URL can never be reinterpreted as shell
// syntax.
package debtap

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// Source classifies where the input .deb came from.
const (
	SourceFile = "file"
	SourceURL  = "url"
)

// UnsupportedOSError reports that the host cannot convert .deb packages because
// it is not pacman-based. Like plasma.UnsupportedOSError this is an ordinary
// error, not a usage error: the invocation was well-formed, the host simply
// cannot serve it.
type UnsupportedOSError struct {
	OS     platform.OS
	PkgMgr platform.PkgMgr
}

func (e *UnsupportedOSError) Error() string {
	return fmt.Sprintf("converting .deb packages requires a pacman-based host (got os=%s pkg=%s)",
		e.OS, e.PkgMgr)
}

// Options is the caller-supplied configuration for one conversion.
type Options struct {
	// Input is the positional argument: an https:// URL or a filesystem path.
	Input string
	// OutDir is where the converted .pkg.tar.zst is written.
	OutDir string
	// SHA256 is the expected hex digest of the input .deb. Empty skips the
	// comparison (the digest is still computed and reported).
	SHA256 string
	// PkgName and PkgVer override the converted package's metadata.
	PkgName string
	PkgVer  string
	// Keep retains the downloaded .deb and the debtap work directory.
	Keep bool
}

// Result accumulates every fact a run establishes, in pipeline order. The
// command layer renders it as key=value lines or as a JSON document; a partial
// Result from a failed run is still meaningful and is still reported.
type Result struct {
	Source string
	// Input is the argument as the user gave it, so a URL run reports the URL
	// rather than the opaque cache path it happened to land in.
	Input string
	// StagedDir is the private directory the input was staged into, reported
	// only when --keep retained it. Every input is staged, from a URL or from
	// disk, so one field describes both honestly.
	StagedDir string
	// WorkDir is debtap's conversion directory, reported only when kept.
	WorkDir      string
	SHA256       string
	DebPackage   string
	DebVersion   string
	Precedence   string
	Artifact     string
	Package      string
	Version      string
	Size         string
	Depends      []string
	Unsatisfied  []string
	Unresolvable []string
	Installed    bool
	Files        int
}

// Dirs locates the on-disk state the converter reads and writes. Injected as a
// struct rather than positional strings so tests can redirect every one of them
// at a temp directory — no test ever touches a real system path — and so adding
// a fourth location later does not silently reorder a call site.
type Dirs struct {
	// Cache is where acquired .deb files are staged.
	Cache string
	// DB is debtap's translation-database directory (normally DefaultDBDir).
	DB string
	// Pkgfile is pkgfile's repository file cache (normally DefaultPkgfileDir).
	Pkgfile string
}

// Converter drives the conversion pipeline.
type Converter struct {
	r          exec.Runner
	cacheDir   string
	dbDir      string
	pkgfileDir string
	// sessionDir is the private 0700 directory this run acquires into, created
	// lazily by session(). Every stage after Acquire reads only from here, so
	// the bytes that were hashed are provably the bytes that get converted.
	sessionDir string
}

// New returns a Converter reading and writing the locations in d.
func New(r exec.Runner, d Dirs) *Converter {
	return &Converter{r: r, cacheDir: d.Cache, dbDir: d.DB, pkgfileDir: d.Pkgfile}
}

// Convert runs debtap on debPath and moves the produced package into
// opts.OutDir, returning its final path.
//
// debtap writes its output beside its input, so the .deb is staged into a temp
// work directory and debtap is run there: the caller's working directory is
// never used as scratch space, and a failed run leaves nothing behind.
// It returns the artifact path and, when opts.Keep is set, the work directory
// it left behind so the caller can tell the user where to find it — a kept
// directory nobody can name is not actually kept.
func (c *Converter) Convert(debPath string, opts Options) (artifact, workDir string, err error) {
	work, err := os.MkdirTemp("", "dotf-deb-")
	if err != nil {
		return "", "", fmt.Errorf("create work dir: %w", err)
	}
	if opts.Keep {
		workDir = work
	} else {
		defer func() { _ = os.RemoveAll(work) }()
	}

	// Validate the destination BEFORE running debtap. Conversion of a large
	// .deb takes minutes, and the deferred RemoveAll below would then delete
	// the freshly built package because of a mkdir that could have been checked
	// in microseconds.
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return "", workDir, fmt.Errorf("create output dir %s: %w", opts.OutDir, err)
	}

	staged := filepath.Join(work, filepath.Base(debPath))
	if err := copyFile(debPath, staged); err != nil {
		return "", workDir, fmt.Errorf("stage %s: %w", debPath, err)
	}

	// -Q is debtap's fully non-interactive mode. Without it debtap stops to ask
	// about metadata, which would strand a user who piped or scripted the run.
	argv := []string{"-Q"}
	if opts.PkgName != "" {
		argv = append(argv, "-P", opts.PkgName)
	}
	if opts.PkgVer != "" {
		argv = append(argv, "-p", opts.PkgVer)
	}
	argv = append(argv, staged)

	if err := c.r.RunDir(work, "debtap", argv...); err != nil {
		return "", workDir, fmt.Errorf("debtap: %w", err)
	}

	built, err := filepath.Glob(filepath.Join(work, "*.pkg.tar*"))
	if err != nil || len(built) == 0 {
		return "", workDir, fmt.Errorf("debtap produced no package for %s", filepath.Base(debPath))
	}
	// Sorted so a debtap that somehow emitted more than one package still
	// resolves to the same artifact on every run.
	sort.Strings(built)

	dest := filepath.Join(opts.OutDir, filepath.Base(built[0]))
	if err := moveFile(built[0], dest); err != nil {
		return "", workDir, fmt.Errorf("place artifact: %w", err)
	}
	return dest, workDir, nil
}

// Cleanup removes the private acquisition directory and everything staged into
// it. When keep is set nothing is removed and the directory's path is returned
// so the caller can tell the user where the retained files are.
//
// Only this package's own directory is ever removed; an input the user already
// had on disk was copied, never adopted, so it is untouched either way.
func (c *Converter) Cleanup(keep bool) string {
	if c.sessionDir == "" {
		return ""
	}
	if keep {
		return c.sessionDir
	}
	_ = os.RemoveAll(c.sessionDir)
	c.sessionDir = ""
	return ""
}

// Install installs artifact with pacman. This is the only step that elevates;
// acquisition, verification and conversion all run as the invoking user.
//
// noConfirm suppresses pacman's own prompt and is passed only when the caller
// already answered the command's confirmation non-interactively (--yes).
// Otherwise pacman prompts too, so a package-replacement decision the
// command's own prompt cannot anticipate still reaches the user.
func (c *Converter) Install(artifact string, noConfirm bool) error {
	argv := []string{"pacman", "-U"}
	if noConfirm {
		argv = append(argv, "--noconfirm")
	}
	argv = append(argv, artifact)

	if err := c.r.Run("sudo", argv...); err != nil {
		return fmt.Errorf("install %s: %w", filepath.Base(artifact), err)
	}
	return nil
}

// copyFile copies src to dst, creating dst with mode 0600.
//
// O_NOFOLLOW: dst may live in a directory the user chose (--out-dir) and does
// not necessarily control. Without it, an attacker who pre-creates the artifact
// filename as a symlink gets this call to truncate and overwrite the link's
// target instead. The attacker can predict that filename exactly, because a
// package's name and version — and so its artifact name — come from the .deb
// they authored.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// moveFile renames src to dst, falling back to copy-then-remove when the two
// live on different filesystems (the normal case here: a temp work dir under
// /tmp and an output dir under $HOME are routinely separate mounts, and
// os.Rename fails with EXDEV across them).
func moveFile(src, dst string) error {
	// os.Rename REPLACES a symlink rather than following it, so it is safe on
	// its own; this Lstat exists so the two paths agree, and so a pre-planted
	// link is reported rather than silently clobbered.
	if fi, err := os.Lstat(dst); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to write %s: it is a symlink", dst)
	}
	if err := os.Rename(src, dst); err != nil {
		// EXDEV: the temp work dir (/tmp) and the output dir (often $HOME) are
		// routinely separate mounts, where rename cannot work.
		if cerr := copyFile(src, dst); cerr != nil {
			return cerr
		}
		if rerr := os.Remove(src); rerr != nil {
			return rerr
		}
	}
	// Applied on BOTH paths so the artifact's mode does not depend on whether
	// the move happened to cross a filesystem boundary. A package is meant to
	// be read and installed, not kept secret; 0644 is what makepkg leaves.
	return os.Chmod(dst, 0o644)
}
