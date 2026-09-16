package backend

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// httpGet is the HTTP fetch entry point; tests may replace it to aim at an
// httptest.Server.
var httpGet = http.Get

// fetchVerifyExtract downloads url, verifies its SHA-256 digest when
// wantSHA256 is non-empty, and extracts (or places) the artifact into destDir.
//
// The download is streamed into a mode-0600 temp file through io.TeeReader so
// the hasher sees every byte, and verification runs before any byte is written
// to destDir (verify-before-execute). The artifact FORMAT is decided by magic
// bytes (detectFormat), not the URL suffix: gzip → tar.gz or single-file .gz,
// zip → .zip, otherwise a bare binary. assetName names the output for the .gz
// and bare-binary cases (a trailing ".gz" is stripped for .gz); it is unused for
// tar.gz/zip, whose entry names come from the archive.
func fetchVerifyExtract(url, wantSHA256, destDir, assetName string) error {
	resp, err := httpGet(url)
	if err != nil {
		return fmt.Errorf("download %q: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %q: unexpected status %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp("", "dotf-dl-*")
	if err != nil {
		return fmt.Errorf("create temp download file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp download file: %w", err)
	}

	hasher := sha256.New()
	tee := io.TeeReader(resp.Body, hasher)

	if _, err := io.Copy(tmp, tee); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp download file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp download file: %w", err)
	}

	if err := VerifySHA256(hasher, wantSHA256); err != nil {
		return err
	}

	format, err := detectFormat(tmpPath)
	if err != nil {
		return err
	}

	switch format {
	case "tar.gz":
		return ExtractTarGz(tmpPath, destDir)
	case "zip":
		return extractZip(tmpPath, destDir)
	case "gz":
		return extractGz(tmpPath, destDir, gzOutputName(assetName))
	default:
		return placeBinary(tmpPath, destDir, outputName(assetName))
	}
}

// fetchSHA256 downloads url and returns the hex-encoded sha256 of its body,
// performing NO extraction and writing no temp file (the bytes are streamed
// straight through the hasher). It is the record side of the lock workflow:
// `dotf lock` calls it via a backend Resolve to freeze an artifact's checksum.
func fetchSHA256(url string) (string, error) {
	resp, err := httpGet(url)
	if err != nil {
		return "", fmt.Errorf("download %q: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %q: unexpected status %s", url, resp.Status)
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, resp.Body); err != nil {
		return "", fmt.Errorf("hash download %q: %w", url, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// VerifySHA256 compares hasher's accumulated digest against the hex-encoded
// wantSHA256 in constant time, returning nil on a match and a typed mismatch
// error otherwise. An EMPTY wantSHA256 SKIPS verification and returns nil, so
// callers that must fail closed (e.g. self-update) MUST reject an empty want
// BEFORE calling this. It is exported so internal/selfupdate reuses the exact
// same digest-compare primitive bootstrap.sh mirrors, keeping the checksum
// contract in one Go place (issue #53).
func VerifySHA256(hasher hash.Hash, wantSHA256 string) error {
	if wantSHA256 == "" {
		return nil
	}

	want, err := hex.DecodeString(wantSHA256)
	if err != nil {
		return fmt.Errorf("decode expected SHA256: %w", err)
	}
	if len(want) != sha256.Size {
		return fmt.Errorf("expected SHA256 length %d, got %d", sha256.Size, len(want))
	}

	got := hasher.Sum(nil)
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return fmt.Errorf("SHA256 mismatch: verification failed")
	}
	return nil
}

// detectFormat classifies the downloaded artifact at path by magic bytes:
// gzip (1f 8b) resolves to "tar.gz" when the decompressed stream carries the
// tar "ustar" magic, else single-file "gz"; zip (50 4b 03 04) is "zip"; anything
// else is a bare "binary". The extension is deliberately ignored — a mislabeled
// or extension-less asset is classified by its contents.
func detectFormat(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open download for sniffing: %w", err)
	}
	defer func() { _ = f.Close() }()

	magic := make([]byte, 4)
	n, err := io.ReadFull(f, magic)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", fmt.Errorf("read download magic bytes: %w", err)
	}

	switch {
	case n >= 2 && magic[0] == 0x1f && magic[1] == 0x8b:
		if gzIsTar(path) {
			return "tar.gz", nil
		}
		return "gz", nil
	case n >= 4 && magic[0] == 0x50 && magic[1] == 0x4b && magic[2] == 0x03 && magic[3] == 0x04:
		return "zip", nil
	default:
		return "binary", nil
	}
}

// gzIsTar reports whether the gzip stream at path decompresses to a tar archive,
// by checking for the "ustar" magic at offset 257 of the first (512-byte) tar
// header block. A stream that decompresses to fewer than 262 bytes is treated as
// a single-file gzip (not a tar).
func gzIsTar(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return false
	}
	defer func() { _ = gz.Close() }()

	buf := make([]byte, 262)
	if _, err := io.ReadFull(gz, buf); err != nil {
		return false
	}
	return string(buf[257:262]) == "ustar"
}

// extractGz gunzips a single-file gzip artifact at archivePath into destDir
// under outName, marking it executable (0o755). The output path is guarded by
// safeExtractPath just like archive entries.
func extractGz(archivePath, destDir, outName string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open gz artifact: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = gz.Close() }()

	target, err := safeExtractPath(filepath.Clean(destDir), outName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %q: %w", target, err)
	}

	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create file %q: %w", target, err)
	}
	if _, err := io.Copy(out, gz); err != nil {
		_ = out.Close()
		return fmt.Errorf("write file %q: %w", target, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close file %q: %w", target, err)
	}
	return nil
}

// placeBinary copies a bare-binary artifact at srcPath into destDir under
// outName with mode 0o755. The output path is guarded by safeExtractPath.
func placeBinary(srcPath, destDir, outName string) error {
	target, err := safeExtractPath(filepath.Clean(destDir), outName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %q: %w", target, err)
	}
	return copyFile(srcPath, target, 0o755)
}

// outputName derives the on-disk name for a gz/bare artifact from assetName,
// falling back to a generic name when assetName is empty or path-only.
func outputName(assetName string) string {
	base := filepath.Base(assetName)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "artifact"
	}
	return base
}

// gzOutputName is outputName with a single trailing ".gz" stripped, so a
// "lefthook_..._Linux_x86_64.gz" artifact lands as "lefthook_..._Linux_x86_64".
func gzOutputName(assetName string) string {
	return strings.TrimSuffix(outputName(assetName), ".gz")
}

// ExtractTarGz extracts the gzip-compressed tar archive at archivePath into
// destDir, applying the same path-traversal guard (safeExtractPath) as every
// other extractor here. It is exported so internal/selfupdate reuses this exact
// extraction primitive rather than duplicating archive handling (issue #53).
func ExtractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open tar.gz archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = gz.Close() }()

	return extractTar(tar.NewReader(gz), destDir)
}

func extractTar(tr *tar.Reader, destDir string) error {
	destDir = filepath.Clean(destDir)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		target, err := safeExtractPath(destDir, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create directory %q: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create parent directory for %q: %w", target, err)
			}

			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode().Perm())
			if err != nil {
				return fmt.Errorf("create file %q: %w", target, err)
			}

			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return fmt.Errorf("write file %q: %w", target, err)
			}
			if err := out.Close(); err != nil {
				return fmt.Errorf("close file %q: %w", target, err)
			}

			if hdr.FileInfo().Mode()&0o111 != 0 {
				if err := os.Chmod(target, 0o755); err != nil {
					return fmt.Errorf("chmod binary %q: %w", target, err)
				}
			}
		default:
			// Skip symlinks, hard links, and other special entries.
			continue
		}
	}
}

func extractZip(archivePath, destDir string) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("stat zip archive: %w", err)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		return fmt.Errorf("zip reader: %w", err)
	}

	destDir = filepath.Clean(destDir)

	for _, entry := range zr.File {
		target, err := safeExtractPath(destDir, entry.Name)
		if err != nil {
			return err
		}

		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create directory %q: %w", target, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create parent directory for %q: %w", target, err)
		}

		mode := entry.Mode().Perm()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			return fmt.Errorf("create file %q: %w", target, err)
		}

		rc, err := entry.Open()
		if err != nil {
			_ = out.Close()
			return fmt.Errorf("open zip entry %q: %w", entry.Name, err)
		}

		if _, err := io.Copy(out, rc); err != nil {
			_ = rc.Close()
			_ = out.Close()
			return fmt.Errorf("write zip entry %q: %w", entry.Name, err)
		}
		if err := rc.Close(); err != nil {
			_ = out.Close()
			return fmt.Errorf("close zip entry %q: %w", entry.Name, err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("close file %q: %w", target, err)
		}

		if entry.Mode()&0o111 != 0 {
			if err := os.Chmod(target, 0o755); err != nil {
				return fmt.Errorf("chmod binary %q: %w", target, err)
			}
		}
	}

	return nil
}

func safeExtractPath(destDir, name string) (string, error) {
	cleanName := filepath.Clean(name)
	if filepath.IsAbs(cleanName) {
		return "", fmt.Errorf("reject absolute archive path %q", name)
	}
	if cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("reject path traversal in archive entry %q", name)
	}

	target := filepath.Clean(filepath.Join(destDir, cleanName))
	rel, err := filepath.Rel(destDir, target)
	if err != nil {
		return "", fmt.Errorf("resolve archive entry %q: %w", name, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("reject path traversal in archive entry %q", name)
	}

	return target, nil
}
