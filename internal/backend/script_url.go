package backend

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

type scriptURLBackend struct {
	r    exec.Runner
	lock *lockfile.Lock
}

// newScriptURLBackend constructs the script-url install backend. lock is
// nil-tolerant: a nil lock means the read/no-verification path, where Install
// falls back to the inline sha256.
func newScriptURLBackend(r exec.Runner, lock *lockfile.Lock) *scriptURLBackend {
	return &scriptURLBackend{r: r, lock: lock}
}

// Install downloads pkg.URL over HTTPS, verifies its checksum when one is
// available, then executes the fetched installer. Verification completes before
// any execution.
//
// The checksum precedence is lock (a platform-agnostic locked artifact for
// pkg.URL) then inline pkg.SHA256 then unverified. Pinning is recommended for
// IMMUTABLE installer URLs. A rotating vendor installer (e.g. starship's
// install.sh) must NOT be pinned: its hash rotates and would fail closed on
// every fresh machine — such an installer is rolling (no inline sha256, no lock
// entry) and runs unverified, relying on HTTPS transport. A Pinnable package
// that resolves no checksum from either source fails fast.
func (b *scriptURLBackend) Install(pkg manifest.Package, _ platform.Info) error {
	if pkg.URL == "" {
		return fmt.Errorf("script-url %q: missing url", pkg.Name)
	}

	wantSHA := pkg.SHA256
	if art, ok := b.lock.Artifact(pkg.Name, "", ""); ok {
		wantSHA = art.SHA256
	}

	if lockfile.Pinnable(pkg) && wantSHA == "" {
		return fmt.Errorf("script-url %q: pinned installer has no locked checksum; run `dotf lock`", pkg.Name)
	}

	scriptPath, err := fetchVerifyScript(pkg.URL, wantSHA)
	if err != nil {
		return fmt.Errorf("script-url %q: %w", pkg.Name, err)
	}
	defer func() { _ = os.Remove(scriptPath) }()

	if err := os.Chmod(scriptPath, 0o700); err != nil {
		return fmt.Errorf("script-url %q: chmod installer: %w", pkg.Name, err)
	}

	// Run under `sh`, not `bash`: a script-url installer is a POSIX vendor
	// script, and some actively refuse a non-POSIX shell — starship's install.sh
	// exits 1 with "Running installation script with non-POSIX `bash` may cause
	// errors. Please use `sh` instead." before doing any work.
	//
	// pkg.Args is appended as literal argv (never through a shell), which is how
	// a prompting installer is driven headlessly: starship needs --yes or it
	// reads /dev/tty for a confirmation that a non-interactive `dotf apply`
	// cannot answer.
	args := append([]string{scriptPath}, pkg.Args...)
	if err := b.r.Run("sh", args...); err != nil {
		return fmt.Errorf("script-url %q: run installer: %w", pkg.Name, err)
	}
	return nil
}

// Installed reports whether the package is present, honoring an explicit
// Provides/InstalledWhen probe and falling back to a PATH lookup of pkg.Name.
func (b *scriptURLBackend) Installed(pkg manifest.Package, _ platform.Info) (bool, error) {
	return probeInstalled(b.r, pkg)
}

// resolve downloads pkg.URL and returns a platform-agnostic Artifact carrying
// only the computed sha256 (empty OS/Arch/Asset/Binary): a script-url installer
// is one URL for every host, so a single checksum covers it. Intended for pinned
// IMMUTABLE installers; a rolling installer is excluded upstream by Pinnable.
func (b *scriptURLBackend) resolve(pkg manifest.Package, _ platform.Info) (lockfile.Artifact, error) {
	if pkg.URL == "" {
		return lockfile.Artifact{}, fmt.Errorf("script-url %q: missing url", pkg.Name)
	}

	sum, err := fetchSHA256(pkg.URL)
	if err != nil {
		return lockfile.Artifact{}, fmt.Errorf("script-url %q: %w", pkg.Name, err)
	}

	return lockfile.Artifact{SHA256: sum}, nil
}

// fetchVerifyScript downloads url and verifies its SHA-256 digest using the
// same verify-before-execute gate as fetchVerifyExtract. The caller must
// remove the returned temp file when finished.
func fetchVerifyScript(url, wantSHA256 string) (string, error) {
	resp, err := httpGet(url)
	if err != nil {
		return "", fmt.Errorf("download %q: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %q: unexpected status %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp("", "dotf-script-*")
	if err != nil {
		return "", fmt.Errorf("create temp script file: %w", err)
	}
	tmpPath := tmp.Name()

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("chmod temp script file: %w", err)
	}

	hasher := sha256.New()
	tee := io.TeeReader(resp.Body, hasher)

	if _, err := io.Copy(tmp, tee); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("write temp script file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("close temp script file: %w", err)
	}

	if err := VerifySHA256(hasher, wantSHA256); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	return tmpPath, nil
}
