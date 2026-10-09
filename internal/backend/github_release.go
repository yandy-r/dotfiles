package backend

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

type githubReleaseBackend struct {
	r          exec.Runner
	lock       *lockfile.Lock
	installDir func() (string, error)
}

// newGitHubReleaseBackend constructs the github-release backend. lock is
// nil-tolerant: a nil lock means the read/no-verification path, where Install
// falls back to the manifest's asset template and inline sha256.
func newGitHubReleaseBackend(r exec.Runner, lock *lockfile.Lock) *githubReleaseBackend {
	return &githubReleaseBackend{
		r:    r,
		lock: lock,
		installDir: func() (string, error) {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("resolve home directory: %w", err)
			}
			return filepath.Join(home, ".local", "bin"), nil
		},
	}
}

// releaseTokens maps a platform.Info to the (os, arch) tokens that key a lock
// artifact and template a release asset. It is the SINGLE source used by both
// resolve (record) and Install (lookup) so the two never silently disagree on
// how a host maps to a locked entry.
func releaseTokens(info platform.Info) (osToken, archToken string) {
	return string(info.OS), info.Arch.ReleaseArch()
}

// Install downloads a verified GitHub release asset, extracts (or places) it,
// and puts the package binary on PATH (~/.local/bin/<Binary||Name>).
//
// When the lock carries an artifact for (pkg.Name, host tokens) it downloads
// the LOCKED asset from the LOCKED tag and verifies the LOCKED sha256,
// overriding the manifest. On a lock miss it routes through resolveAsset — the
// SAME resolver that records lock entries — which resolves the asset (explicit
// template or auto-detected) and its checksum (per-target map, inline, or
// sidecar). A Pinnable package that resolves no checksum fails fast, before any
// download.
func (b *githubReleaseBackend) Install(pkg manifest.Package, info platform.Info) error {
	if err := validateGitHubRepo(pkg.Repo); err != nil {
		return err
	}

	osToken, archToken := releaseTokens(info)
	target := binaryName(pkg)

	var asset, tag, wantSHA string
	if art, ok := b.lock.Artifact(pkg.Name, osToken, archToken); ok {
		asset = art.Asset
		wantSHA = art.SHA256
		tag = pkg.Tag
		if lp, ok := b.lock.Package(pkg.Name); ok {
			tag = lp.Tag
		}
	} else {
		ra, err := b.resolveAsset(pkg, info)
		if err != nil {
			return err
		}
		asset = ra.Asset
		tag = ra.Tag
		wantSHA = ra.SHA256
	}

	if lockfile.Pinnable(pkg) && wantSHA == "" {
		return pinnedNoChecksumErr(pkg.Name, osToken, archToken)
	}

	downloadURL := githubReleaseURL(pkg.Repo, tag, asset)

	stagingDir, err := os.MkdirTemp("", "dotf-ghr-*")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	if err := fetchVerifyExtract(downloadURL, wantSHA, stagingDir, asset); err != nil {
		return err
	}

	binaryPath, err := findReleaseBinary(stagingDir, target)
	if err != nil {
		return err
	}

	installDir, err := b.installDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("create install directory %q: %w", installDir, err)
	}

	dest := filepath.Join(installDir, target)
	if err := copyFile(binaryPath, dest, 0o755); err != nil {
		return fmt.Errorf("install %q: %w", target, err)
	}

	return nil
}

// Installed reports whether the package is present, honoring an explicit
// Provides/InstalledWhen probe and falling back to a PATH lookup of pkg.Name.
func (b *githubReleaseBackend) Installed(pkg manifest.Package, _ platform.Info) (bool, error) {
	return probeInstalled(b.r, pkg)
}

// resolvedAsset is the outcome of resolving a github-release package for one
// host: the repo/tag coordinates, the concrete asset filename (explicit or
// auto-detected), and its expected sha256 (per-target map, inline, or sidecar).
// Install downloads it; resolve freezes it into a lockfile.Artifact.
type resolvedAsset struct {
	// Repo is the "owner/repo" slug.
	Repo string

	// Tag is the release tag to download from.
	Tag string

	// Asset is the resolved release asset filename.
	Asset string

	// SHA256 is the expected checksum, or "" when no source resolves one (the
	// caller applies the Pinnable fail-closed rule).
	SHA256 string
}

// resolveAsset resolves pkg for info into concrete download coordinates via the
// shared resolveGitHubArtifact, the SINGLE resolver both the lock-record path
// (resolve) and the lock-miss install path route through.
func (b *githubReleaseBackend) resolveAsset(pkg manifest.Package, info platform.Info) (resolvedAsset, error) {
	a, err := resolveGitHubArtifact(pkg, info, nil)
	if err != nil {
		return resolvedAsset{}, err
	}
	return resolvedAsset{Repo: pkg.Repo, Tag: pkg.Tag, Asset: a.Asset, SHA256: a.SHA256}, nil
}

// resolve records a frozen Artifact for target by routing through resolveAsset —
// the SAME resolver Install uses — so a recorded artifact always matches the
// verify-time lookup. Unlike the pre-#45 path it records the DECLARED/sidecar
// sha256 without a full asset download.
func (b *githubReleaseBackend) resolve(pkg manifest.Package, target platform.Info) (lockfile.Artifact, error) {
	ra, err := b.resolveAsset(pkg, target)
	if err != nil {
		return lockfile.Artifact{}, err
	}

	osToken, archToken := releaseTokens(target)
	return lockfile.Artifact{
		OS:     osToken,
		Arch:   archToken,
		Asset:  ra.Asset,
		Binary: binaryName(pkg),
		SHA256: ra.SHA256,
	}, nil
}

// binaryName is the installed executable name: the explicit Binary override when
// set, else the package Name.
func binaryName(pkg manifest.Package) string {
	if pkg.Binary != "" {
		return pkg.Binary
	}
	return pkg.Name
}

// findReleaseBinary locates the installed binary in the extracted/placed
// staging tree. It prefers a regular file whose basename equals target
// (Binary||Name); when no name matches — a bare binary or a single-file archive
// whose entry is named differently — it falls back to the SOLE extracted regular
// file. Multiple name matches, or no name match with several files, are typed
// errors asking the user to pin `binary`.
func findReleaseBinary(root, target string) (string, error) {
	var nameMatches, allFiles []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.Type().IsRegular() {
			return nil
		}
		allFiles = append(allFiles, path)
		if filepath.Base(path) == target {
			nameMatches = append(nameMatches, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("search extracted archive: %w", err)
	}

	switch len(nameMatches) {
	case 1:
		return nameMatches[0], nil
	case 0:
		// fall through to the single-file fallback below.
	default:
		return "", fmt.Errorf("github-release: multiple files named %q in release archive", target)
	}

	switch len(allFiles) {
	case 0:
		return "", fmt.Errorf("github-release: binary %q not found in release archive", target)
	case 1:
		return allFiles[0], nil
	default:
		return "", fmt.Errorf("github-release: could not identify binary %q among %d extracted files; set `binary`", target, len(allFiles))
	}
}

func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create %q: %w", dest, err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy to %q: %w", dest, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %q: %w", dest, err)
	}
	return nil
}
