package backend

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// GitNotFoundError is returned by gitCloneBackend.Install when the git
// executable is absent from PATH. It is a fail-fast typed error (repo policy:
// fail fast at boundaries) rather than a silent not-applicable skip, so a host
// missing git surfaces the missing dependency instead of quietly no-op'ing.
type GitNotFoundError struct {
	// Package names the git-clone package whose install could not find git.
	Package string
}

// Error implements the error interface.
func (e *GitNotFoundError) Error() string {
	return fmt.Sprintf("git-clone: package %q: git executable not found on PATH", e.Package)
}

// gitCloneBackend provisions a package by shallow-cloning a git remote into a
// $HOME-relative destination and running each declared post_install step from
// inside the clone. It has no applicable() method — a git clone runs on every
// host — so a missing git surfaces as *GitNotFoundError from Install, not a
// not-applicable skip.
type gitCloneBackend struct {
	r    exec.Runner
	home func() (string, error)
}

// newGitCloneBackend constructs a git-clone backend. The home seam resolves the
// $HOME that Dest is joined onto; tests override it for a hermetic temp home.
func newGitCloneBackend(r exec.Runner) *gitCloneBackend {
	return &gitCloneBackend{
		r:    r,
		home: os.UserHomeDir,
	}
}

// Install clones pkg.URL into the $HOME-relative pkg.Dest at depth 1 (checking
// out pkg.Ref when set), then runs each post_install step from inside the clone.
// It self-guards: when the dest already exists it returns nil BEFORE shelling to
// git, so a re-apply — even under --force, which bypasses the engine's
// Installed() gate — never re-clones into a populated directory. A missing git
// executable is a fail-fast *GitNotFoundError, not a silent skip.
func (b *gitCloneBackend) Install(pkg manifest.Package, _ platform.Info) error {
	dest, err := b.resolveDest(pkg.Dest)
	if err != nil {
		return fmt.Errorf("git-clone: package %q: %w", pkg.Name, err)
	}

	if _, statErr := os.Stat(dest); statErr == nil {
		return nil // already cloned — idempotent no-op
	}

	if _, lookErr := b.r.LookPath("git"); lookErr != nil {
		return &GitNotFoundError{Package: pkg.Name}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("git-clone: package %q: create parent directory: %w", pkg.Name, err)
	}

	args := []string{"clone", "--depth", "1"}
	if pkg.Ref != "" {
		args = append(args, "--branch", pkg.Ref)
	}
	args = append(args, pkg.URL, dest)
	if err := b.r.Run("git", args...); err != nil {
		return fmt.Errorf("git-clone: package %q: clone %q: %w", pkg.Name, pkg.URL, err)
	}

	// Roll back the freshly-cloned dest if a post_install step fails, so the
	// half-installed directory is not left behind: Installed() and the dest-exists
	// self-guard above both equate dest presence with a COMPLETED install, so a
	// clone-succeeded-but-post_install-failed dir would otherwise report installed
	// forever and never re-run post_install (even under --force). dest was absent
	// before this call (the guard returned early otherwise), so removing it
	// restores the pre-install state and a re-apply re-clones + re-runs post_install.
	if err := b.runPostInstall(pkg, dest); err != nil {
		_ = os.RemoveAll(dest)
		return err
	}

	return nil
}

// Installed reports whether pkg's clone dest already exists on disk. The dest is
// the intrinsic installed-state probe for a git-clone package (no PATH binary,
// no user-declared probe), so a present dest means installed.
func (b *gitCloneBackend) Installed(pkg manifest.Package, _ platform.Info) (bool, error) {
	dest, err := b.resolveDest(pkg.Dest)
	if err != nil {
		return false, fmt.Errorf("git-clone: package %q: %w", pkg.Name, err)
	}
	if _, statErr := os.Stat(dest); statErr != nil {
		return false, nil
	}
	return true, nil
}

// resolveDest joins the $HOME-relative dest onto the resolved home directory,
// re-checking containment at runtime (the same fence validateGitClone enforces
// at load) via containedJoin, so the clone can never land outside $HOME.
func (b *gitCloneBackend) resolveDest(dest string) (string, error) {
	home, err := b.home()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return containedJoin(home, dest)
}

// runPostInstall executes each post_install step from inside the just-cloned
// dest. Every step's Run[0] is resolved RELATIVE TO and contained WITHIN dest
// (containedJoin), so a step can only execute code the clone itself shipped — it
// can never reach a host binary, the repo's bin/install-*.sh, or a shell string.
// This is the fence that keeps git-clone from becoming custom-script v2. The
// exec runs with CWD = dest (via RunDir) because a cloned artifact's own
// installer generally expects to run from the clone root.
func (b *gitCloneBackend) runPostInstall(pkg manifest.Package, dest string) error {
	for _, step := range pkg.PostInstall {
		if len(step.Run) == 0 {
			return fmt.Errorf("git-clone: package %q: post_install step has empty run", pkg.Name)
		}
		exe, err := containedJoin(dest, step.Run[0])
		if err != nil {
			return fmt.Errorf("git-clone: package %q: post_install: %w", pkg.Name, err)
		}
		if err := b.r.RunDir(dest, exe, step.Run[1:]...); err != nil {
			return fmt.Errorf("git-clone: package %q: post_install %q: %w", pkg.Name, exe, err)
		}
	}
	return nil
}

var _ Backend = (*gitCloneBackend)(nil)
