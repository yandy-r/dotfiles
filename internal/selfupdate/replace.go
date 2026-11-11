package selfupdate

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// httpGet is the artifact-download seam (tarball + checksums.txt). Tests replace
// it to aim at an httptest.Server. The default is a client with a total timeout
// (net/http has none by default).
var httpGet = httpClient.Get

// executablePath is the running-binary seam so tests can point the replace at a
// temp file instead of the test binary itself.
var executablePath = os.Executable

// replaceBinary performs the verify-before-replace flow for the running dotf
// binary: resolve its real path, fetch and parse checksums.txt (fail-fast when
// the asset line is missing or empty), stream+verify+extract the tarball into a
// staging dir, then atomically rename the new binary over the old one. It
// returns the resolved target path (populated even on failure, for reporting)
// and the first error encountered. The original binary is untouched on any
// pre-rename error (rollback-by-construction).
func replaceBinary(repo, tag string, info platform.Info) (string, error) {
	exe, err := resolveExecutable()
	if err != nil {
		return exe, err
	}
	dir := filepath.Dir(exe)
	asset := AssetName(info)

	// Fail-fast on a missing/empty checksum BEFORE any download: VerifySHA256
	// SKIPS verification on an empty want, so an unverified replace must be
	// impossible (issue #53 security corrective).
	wantSHA, err := fetchChecksum(repo, tag, asset)
	if err != nil {
		return exe, err
	}

	staging, err := os.MkdirTemp("", "dotf-selfupdate-*")
	if err != nil {
		return exe, fmt.Errorf("self-update: create staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()

	if err := downloadVerifyExtract(repo, tag, asset, wantSHA, staging); err != nil {
		return exe, err
	}

	newBinary, err := findBinary(staging, binaryName)
	if err != nil {
		return exe, err
	}

	mode := os.FileMode(0o755)
	if fi, statErr := os.Stat(exe); statErr == nil {
		mode = fi.Mode().Perm()
	}

	if err := atomicReplace(exe, dir, newBinary, mode); err != nil {
		return exe, err
	}
	return exe, nil
}

// resolveExecutable returns the real path of the running binary, following a
// symlink (e.g. ~/.local/bin/dotf → the actual file) so the rename swaps the
// concrete file, not the link.
func resolveExecutable() (string, error) {
	exe, err := executablePath()
	if err != nil {
		return "", fmt.Errorf("self-update: locate running binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, fmt.Errorf("self-update: resolve running binary %q: %w", exe, err)
	}
	return resolved, nil
}

// fetchChecksum downloads checksums.txt for repo@tag and returns the hex sha256
// recorded for asset. A missing asset line, an empty checksum, or a read error
// is a typed error — the caller never proceeds to replace with an empty want.
func fetchChecksum(repo, tag, asset string) (string, error) {
	checksumsURL := releaseAssetURL(repo, tag, ChecksumsAsset)
	resp, err := httpGet(checksumsURL)
	if err != nil {
		return "", fmt.Errorf("self-update: fetch %s: %w", ChecksumsAsset, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("self-update: fetch %s: unexpected status %s", ChecksumsAsset, resp.Status)
	}
	return parseChecksum(resp.Body, asset)
}

// parseChecksum scans a coreutils sha256sum file ("<sha256>␠␠<filename>", a
// single space and an optional "*" binary-mode marker tolerated) for asset and
// returns its checksum. A leading path on the filename is stripped to its base.
// A missing line or an empty checksum is a typed error (fail closed).
func parseChecksum(r io.Reader, asset string) (string, error) {
	want := path.Base(asset)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		if path.Base(strings.TrimPrefix(fields[1], "*")) != want {
			continue
		}
		if fields[0] == "" {
			return "", fmt.Errorf("self-update: %s has an empty checksum for asset %q", ChecksumsAsset, want)
		}
		return fields[0], nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("self-update: read %s: %w", ChecksumsAsset, err)
	}
	return "", fmt.Errorf("self-update: %s has no line for asset %q", ChecksumsAsset, want)
}

// downloadVerifyExtract streams the release tarball into a 0600 temp file
// through a sha256 TeeReader, verifies the digest against wantSHA BEFORE any
// extraction (verify-before-execute), then extracts the tar.gz into destDir
// using the shared backend primitives so the checksum/extraction contract lives
// in one Go place.
func downloadVerifyExtract(repo, tag, asset, wantSHA, destDir string) error {
	assetURL := releaseAssetURL(repo, tag, asset)
	resp, err := httpGet(assetURL)
	if err != nil {
		return fmt.Errorf("self-update: download %q: %w", assetURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("self-update: download %q: unexpected status %s", assetURL, resp.Status)
	}

	tmp, err := os.CreateTemp("", "dotf-selfupdate-dl-*")
	if err != nil {
		return fmt.Errorf("self-update: create temp download file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("self-update: chmod temp download file: %w", err)
	}

	hasher := sha256.New()
	if _, err := io.Copy(tmp, io.TeeReader(resp.Body, hasher)); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("self-update: write temp download file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("self-update: close temp download file: %w", err)
	}

	if err := backend.VerifySHA256(hasher, wantSHA); err != nil {
		return err
	}
	return backend.ExtractTarGz(tmpPath, destDir)
}

// atomicReplace writes newBinary's bytes to a temp file in dir (the SAME
// filesystem as exe, so the rename is atomic), preserves mode, fsyncs, then
// renames over exe. A permission failure on the create or the rename is
// translated into a fail-fast unprivileged-reinstall hint (self-update never
// escalates to sudo).
func atomicReplace(exe, dir, newBinary string, mode os.FileMode) error {
	tmpNew, err := os.CreateTemp(dir, ".dotf-update-*")
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return permissionDeniedErr(exe)
		}
		return fmt.Errorf("self-update: stage replacement next to %q: %w", exe, err)
	}
	tmpPath := tmpNew.Name()

	src, err := os.Open(newBinary)
	if err != nil {
		_ = tmpNew.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("self-update: open extracted binary: %w", err)
	}
	if _, err := io.Copy(tmpNew, src); err != nil {
		_ = src.Close()
		_ = tmpNew.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("self-update: write replacement binary: %w", err)
	}
	_ = src.Close()

	if err := tmpNew.Chmod(mode); err != nil {
		_ = tmpNew.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("self-update: chmod replacement binary: %w", err)
	}
	if err := tmpNew.Sync(); err != nil {
		_ = tmpNew.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("self-update: sync replacement binary: %w", err)
	}
	if err := tmpNew.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("self-update: close replacement binary: %w", err)
	}

	if err := os.Rename(tmpPath, exe); err != nil {
		_ = os.Remove(tmpPath)
		if errors.Is(err, fs.ErrPermission) {
			return permissionDeniedErr(exe)
		}
		return fmt.Errorf("self-update: replace %q: %w", exe, err)
	}
	return nil
}

// permissionDeniedErr reports that the running binary lives in a location the
// current user cannot write, and points at a manual reinstall. Self-update is
// deliberately unprivileged — it never invokes sudo — so a root-owned path is a
// hard stop, not an escalation.
func permissionDeniedErr(exe string) error {
	return fmt.Errorf("self-update: permission denied replacing %s; reinstall manually as the owner of that path "+
		"(e.g. re-run the bootstrap installer) — self-update does not use sudo", exe)
}

// findBinary walks root for a regular file whose basename equals name (the dotf
// binary among the tar.gz's LICENSE/README siblings), returning its path.
func findBinary(root, name string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type().IsRegular() && filepath.Base(p) == name {
			found = p
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("self-update: search extracted archive: %w", err)
	}
	if found == "" {
		return "", fmt.Errorf("self-update: binary %q not found in release archive", name)
	}
	return found, nil
}

// releaseAssetURL builds the download URL for a release asset. A non-empty tag
// pins the exact release; an empty tag falls back to the floating latest
// release. self-update owns this builder (rather than depending on an
// unexported backend helper) so its release-URL shape is explicit and testable.
func releaseAssetURL(repo, tag, asset string) string {
	if tag != "" {
		return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s",
			repo, url.PathEscape(tag), url.PathEscape(asset))
	}
	return fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repo, url.PathEscape(asset))
}
