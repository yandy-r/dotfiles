package backend

import (
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/privilege"
)

// Default constructs an Engine with every canonical backend registered.
// lock is threaded into the download backends (github-release, script-url) for
// pinned-artifact checksum verification; it is nil-tolerant, so read-only
// commands pass nil to keep today's inline-sha256 behavior.
func Default(r exec.Runner, drop privilege.Dropper, repoRoot string, lock *lockfile.Lock) *Engine {
	repo := newRepoBackend(r)
	return NewEngine(map[manifest.Backend]Backend{
		manifest.BackendPkg:           newPkgBackend(r),
		manifest.BackendGitHubRelease: newGitHubReleaseBackend(r, lock),
		manifest.BackendAUR:           newAURBackend(r, drop),
		manifest.BackendToolchain:     newToolchainBackend(r),
		manifest.BackendRepo:          repo,
		manifest.BackendScriptURL:     newScriptURLBackend(r, lock),
		manifest.BackendCustomScript:  newCustomScriptBackend(r, repoRoot),
		manifest.BackendGitClone:      newGitCloneBackend(r),
		manifest.BackendFont:          newFontBackend(r),
		manifest.BackendAppArtifact:   newAppArtifactBackend(r).withLock(lock),
		manifest.BackendPkgURL:        newPkgURLBackend(r).withLock(lock),
	})
}
