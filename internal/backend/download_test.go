package backend

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownload_SHA256Match_TarGz(t *testing.T) {
	destDir := t.TempDir()
	payload := []byte("#!/bin/sh\necho hi\n")
	archive := buildTarGz(t, map[string]tarEntry{
		"bin/tool": {data: payload, mode: 0o755},
	})
	wantSHA := sha256Hex(archive)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	url := srv.URL + "/release.tar.gz"
	if err := fetchVerifyExtract(url, wantSHA, destDir, "release.tar.gz"); err != nil {
		t.Fatalf("fetchVerifyExtract() error = %v, want nil", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "bin", "tool"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("extracted content = %q, want %q", got, payload)
	}

	info, err := os.Stat(filepath.Join(destDir, "bin", "tool"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("file mode = %o, want 0755", info.Mode().Perm())
	}
}

func TestDownload_SHA256Mismatch_AbortsWithoutExtract(t *testing.T) {
	destDir := t.TempDir()
	archive := buildTarGz(t, map[string]tarEntry{
		"bin/tool": {data: []byte("payload"), mode: 0o755},
	})
	wrongSHA := strings.Repeat("0", 64)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	url := srv.URL + "/release.tar.gz"
	err := fetchVerifyExtract(url, wrongSHA, destDir, "release.tar.gz")
	if err == nil {
		t.Fatal("fetchVerifyExtract() error = nil, want SHA256 mismatch error")
	}
	if !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("fetchVerifyExtract() error = %v, want SHA256 mismatch", err)
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("destDir has %d entries after mismatch, want 0", len(entries))
	}
}

func TestDownload_ZipExtract(t *testing.T) {
	destDir := t.TempDir()
	payload := []byte("#!/bin/sh\necho zip\n")
	archive := buildZip(t, map[string]zipEntry{
		"bin/zip-tool": {data: payload, mode: 0o755},
	})
	wantSHA := sha256Hex(archive)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	url := srv.URL + "/release.zip"
	if err := fetchVerifyExtract(url, wantSHA, destDir, "release.zip"); err != nil {
		t.Fatalf("fetchVerifyExtract() error = %v, want nil", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "bin", "zip-tool"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("extracted content = %q, want %q", got, payload)
	}
}

func TestDownload_TarGzRejectsTraversal(t *testing.T) {
	destDir := t.TempDir()
	archive := buildTarGz(t, map[string]tarEntry{
		"../../../etc/passwd": {data: []byte("pwned"), mode: 0o644},
	})
	wantSHA := sha256Hex(archive)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	url := srv.URL + "/evil.tar.gz"
	err := fetchVerifyExtract(url, wantSHA, destDir, "evil.tar.gz")
	if err == nil {
		t.Fatal("fetchVerifyExtract() error = nil, want traversal rejection")
	}
	if !strings.Contains(err.Error(), "traversal") && !strings.Contains(err.Error(), "reject") {
		t.Fatalf("fetchVerifyExtract() error = %v, want traversal rejection", err)
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("destDir has %d entries after traversal attempt, want 0", len(entries))
	}
}

func TestDownload_ZipRejectsTraversal(t *testing.T) {
	destDir := t.TempDir()
	archive := buildZip(t, map[string]zipEntry{
		"../../outside.txt": {data: []byte("pwned"), mode: 0o644},
	})
	wantSHA := sha256Hex(archive)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	url := srv.URL + "/evil.zip"
	err := fetchVerifyExtract(url, wantSHA, destDir, "evil.zip")
	if err == nil {
		t.Fatal("fetchVerifyExtract() error = nil, want traversal rejection")
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("destDir has %d entries after traversal attempt, want 0", len(entries))
	}
}

func TestDownload_SkipsVerifyWhenSHA256Empty(t *testing.T) {
	destDir := t.TempDir()
	archive := buildTarGz(t, map[string]tarEntry{
		"tool": {data: []byte("ok"), mode: 0o755},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	url := srv.URL + "/release.tar.gz"
	if err := fetchVerifyExtract(url, "", destDir, "release.tar.gz"); err != nil {
		t.Fatalf("fetchVerifyExtract() error = %v, want nil", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "tool")); err != nil {
		t.Fatalf("Stat() error = %v, want extracted file", err)
	}
}

// TestDownload_Gz covers a single-file gzip artifact: the gunzipped payload
// lands under the assetName with the trailing ".gz" stripped, executable.
func TestDownload_Gz(t *testing.T) {
	destDir := t.TempDir()
	payload := []byte("#!/bin/sh\necho gz\n")
	archive := buildGz(payload)
	wantSHA := sha256Hex(archive)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	url := srv.URL + "/lefthook_Linux_x86_64.gz"
	if err := fetchVerifyExtract(url, wantSHA, destDir, "lefthook_Linux_x86_64.gz"); err != nil {
		t.Fatalf("fetchVerifyExtract() error = %v, want nil", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "lefthook_Linux_x86_64"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("gunzipped content = %q, want %q", got, payload)
	}
	info, err := os.Stat(filepath.Join(destDir, "lefthook_Linux_x86_64"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("gz output mode = %o, want executable", info.Mode().Perm())
	}
}

// TestDownload_BareBinary covers a bare (uncompressed, extension-less) binary
// asset: it is copied verbatim under the assetName, executable.
func TestDownload_BareBinary(t *testing.T) {
	destDir := t.TempDir()
	payload := []byte("\x7fELF not-really-but-not-gzip-or-zip\n")
	wantSHA := sha256Hex(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	url := srv.URL + "/hadolint-Linux-x86_64"
	if err := fetchVerifyExtract(url, wantSHA, destDir, "hadolint-Linux-x86_64"); err != nil {
		t.Fatalf("fetchVerifyExtract() error = %v, want nil", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "hadolint-Linux-x86_64"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("placed binary = %q, want %q", got, payload)
	}
}

// TestDownload_SniffOverridesExtension proves the format follows magic bytes,
// not the URL suffix: a tar.gz archive served at a ".zip" URL still extracts as
// a tar.gz.
func TestDownload_SniffOverridesExtension(t *testing.T) {
	destDir := t.TempDir()
	payload := []byte("sniffed\n")
	archive := buildTarGz(t, map[string]tarEntry{"tool": {data: payload, mode: 0o755}})
	wantSHA := sha256Hex(archive)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	// URL and assetName both lie ("mislabeled.zip"); magic bytes decide.
	url := srv.URL + "/mislabeled.zip"
	if err := fetchVerifyExtract(url, wantSHA, destDir, "mislabeled.zip"); err != nil {
		t.Fatalf("fetchVerifyExtract() error = %v, want nil", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "tool"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v, want tar.gz entry despite .zip suffix", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("extracted content = %q, want %q", got, payload)
	}
}

type tarEntry struct {
	data []byte
	mode int64
}

func buildTarGz(t *testing.T, files map[string]tarEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, entry := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: entry.mode,
			Size: int64(len(entry.data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%q) error = %v", name, err)
		}
		if _, err := tw.Write(entry.data); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close() error = %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}

	return buf.Bytes()
}

// buildGz returns a single-file gzip stream of data (no tar wrapper), the shape
// of a bare-binary-in-.gz release asset.
func buildGz(data []byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write(data)
	_ = gw.Close()
	return buf.Bytes()
}

type zipEntry struct {
	data []byte
	mode uint32
}

func buildZip(t *testing.T, files map[string]zipEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for name, entry := range files {
		hdr := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}
		hdr.SetMode(os.FileMode(entry.mode))
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("CreateHeader(%q) error = %v", name, err)
		}
		if _, err := w.Write(entry.data); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
	}

	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
