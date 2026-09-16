package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/version"
)

// NOTE: every test here reassigns package-global seams (httpGet/httpDo/
// latestRelease/executablePath) and/or version.Version. Per the repo's -race
// discipline (memory: reassigned globals + t.Parallel == a -race hazard), NONE
// of these tests call t.Parallel.

var linuxAMD64 = platform.Info{OS: platform.OSLinux, Arch: platform.ArchX86_64}

// --- helpers -----------------------------------------------------------------

func buildTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, data := range files {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%q): %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("Write(%q): %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// releaseServer serves the release asset + checksums.txt and counts hits so a
// test can assert that --check/--dry-run/missing-checksum never download.
type releaseServer struct {
	assetHits    atomic.Int64
	checksumHits atomic.Int64
}

func newReleaseServer(t *testing.T, asset string, tarball []byte, checksums string) *releaseServer {
	t.Helper()
	rs := &releaseServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/"+ChecksumsAsset):
			rs.checksumHits.Add(1)
			_, _ = io.WriteString(w, checksums)
		case strings.HasSuffix(r.URL.Path, "/"+asset):
			rs.assetHits.Add(1)
			_, _ = w.Write(tarball)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	orig := httpGet
	httpGet = func(rawurl string) (*http.Response, error) {
		u, err := url.Parse(rawurl)
		if err != nil {
			return nil, err
		}
		return srv.Client().Get(srv.URL + u.Path)
	}
	t.Cleanup(func() {
		httpGet = orig
		srv.Close()
	})
	return rs
}

func withLatest(t *testing.T, tag string) {
	t.Helper()
	orig := latestRelease
	latestRelease = func(string) (string, error) { return tag, nil }
	t.Cleanup(func() { latestRelease = orig })
}

func withVersion(t *testing.T, v string) {
	t.Helper()
	orig := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = orig })
}

// installBinary writes an "old" dotf at a temp path, points executablePath at
// it, and returns the path. mode is forced explicitly so mode-preservation is
// observable regardless of the runner's umask.
func installBinary(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "dotf")
	if err := os.WriteFile(exe, []byte(content), mode); err != nil {
		t.Fatalf("write old binary: %v", err)
	}
	if err := os.Chmod(exe, mode); err != nil {
		t.Fatalf("chmod old binary: %v", err)
	}
	orig := executablePath
	executablePath = func() (string, error) { return exe, nil }
	t.Cleanup(func() { executablePath = orig })
	return exe
}

// --- AssetName ---------------------------------------------------------------

func TestAssetName(t *testing.T) {
	cases := []struct {
		os   platform.OS
		arch platform.Arch
		want string
	}{
		{platform.OSLinux, platform.ArchX86_64, "dotf_linux_amd64.tar.gz"},
		{platform.OSLinux, platform.ArchARM64, "dotf_linux_arm64.tar.gz"},
		{platform.OSMacOS, platform.ArchX86_64, "dotf_darwin_amd64.tar.gz"},
		{platform.OSMacOS, platform.ArchARM64, "dotf_darwin_arm64.tar.gz"},
	}
	for _, tc := range cases {
		if got := AssetName(platform.Info{OS: tc.os, Arch: tc.arch}); got != tc.want {
			t.Errorf("AssetName(%s/%s) = %q, want %q", tc.os, tc.arch, got, tc.want)
		}
	}
}

// --- semver ------------------------------------------------------------------

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b    string
		want    int
		wantErr bool
	}{
		{"0.1.0", "0.2.0", -1, false},
		{"0.2.0", "0.1.0", 1, false},
		{"1.0.0", "1.0.0", 0, false},
		{"v1.2.3", "1.2.3", 0, false},  // leading-v normalization
		{"1.2.3", "v1.2.4", -1, false}, // mixed leading-v
		{"1.0.0-rc1", "1.0.0", -1, false},
		{"1.0.0", "1.0.0-rc1", 1, false},
		{"1.0.0-1", "1.0.0-2", -1, false},           // numeric prerelease identifiers
		{"1.0.0-2", "1.0.0-10", -1, false},          // numeric, not lexical
		{"1.0.0-alpha", "1.0.0-alpha.1", -1, false}, // longer set outranks prefix
		{"2.0.0", "10.0.0", -1, false},              // numeric major, not lexical
		{"dev", "1.0.0", 0, true},                   // unparseable current
		{"1.0.0", "dev", 0, true},                   // unparseable latest
	}
	for _, tc := range cases {
		got, err := compareVersions(tc.a, tc.b)
		if (err != nil) != tc.wantErr {
			t.Errorf("compareVersions(%q,%q) err = %v, wantErr %v", tc.a, tc.b, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("compareVersions(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// --- latestRelease discovery -------------------------------------------------

func withGitHubAPI(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	orig := httpDo
	httpDo = func(req *http.Request) (*http.Response, error) {
		u, err := url.Parse(srv.URL)
		if err != nil {
			return nil, err
		}
		req.URL.Scheme = u.Scheme
		req.URL.Host = u.Host
		return srv.Client().Do(req)
	}
	t.Cleanup(func() {
		httpDo = orig
		srv.Close()
	})
}

func TestLatestRelease_HappyPath(t *testing.T) {
	withGitHubAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			_, _ = io.WriteString(w, `{"tag_name":"v1.2.3","prerelease":false}`)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	tag, err := defaultLatestRelease("yandy-r/dotfiles")
	if err != nil {
		t.Fatalf("defaultLatestRelease err = %v", err)
	}
	if tag != "v1.2.3" {
		t.Errorf("tag = %q, want v1.2.3", tag)
	}
}

func TestLatestRelease_PrereleaseFallback(t *testing.T) {
	withGitHubAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/releases"):
			_, _ = io.WriteString(w, `[{"tag_name":"v0.9.0-rc1"},{"tag_name":"v0.8.0"}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	tag, err := defaultLatestRelease("yandy-r/dotfiles")
	if err != nil {
		t.Fatalf("defaultLatestRelease err = %v", err)
	}
	if tag != "v0.9.0-rc1" {
		t.Errorf("tag = %q, want v0.9.0-rc1 (releases[0])", tag)
	}
}

func TestLatestRelease_NonOKErrors(t *testing.T) {
	withGitHubAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, err := defaultLatestRelease("yandy-r/dotfiles"); err == nil {
		t.Fatal("defaultLatestRelease err = nil, want an unexpected-status error")
	}
}

func TestLatestRelease_SendsBearerToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "tok123")
	authCh := make(chan string, 1)
	withGitHubAPI(t, func(w http.ResponseWriter, r *http.Request) {
		authCh <- r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"tag_name":"v1.0.0"}`)
	})
	if _, err := defaultLatestRelease("yandy-r/dotfiles"); err != nil {
		t.Fatalf("defaultLatestRelease err = %v", err)
	}
	if got := <-authCh; got != "Bearer tok123" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer tok123")
	}
}

// --- Run: end-to-end replace -------------------------------------------------

func TestRun_ReplaceUpdatesBinaryAndPreservesMode(t *testing.T) {
	asset := AssetName(linuxAMD64)
	newBytes := []byte("NEW-DOTF-BINARY-CONTENT")
	tarball := buildTarGz(t, map[string][]byte{"dotf": newBytes, "LICENSE": []byte("MIT")})
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(tarball), asset)

	rs := newReleaseServer(t, asset, tarball, checksums)
	withLatest(t, "v0.2.0")
	withVersion(t, "0.1.0")
	exe := installBinary(t, "OLD", 0o700)

	res, err := Run(Options{}, linuxAMD64)
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if !res.Updated {
		t.Error("Updated = false, want true")
	}
	if res.Current != "0.1.0" || res.Latest != "0.2.0" || !res.Available {
		t.Errorf("Result = %+v, want current=0.1.0 latest=0.2.0 available=true", res)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}
	if !bytes.Equal(got, newBytes) {
		t.Errorf("replaced binary = %q, want %q", got, newBytes)
	}
	fi, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("stat replaced binary: %v", err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("replaced binary mode = %v, want 0700 (preserved)", fi.Mode().Perm())
	}
	if rs.assetHits.Load() != 1 {
		t.Errorf("asset download hits = %d, want 1", rs.assetHits.Load())
	}
}

func TestRun_IdempotentWhenCurrent(t *testing.T) {
	withLatest(t, "v0.2.0")
	withVersion(t, "0.2.0")
	exe := installBinary(t, "OLD", 0o755)

	res, err := Run(Options{}, linuxAMD64)
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Updated {
		t.Error("Updated = true, want false (already current)")
	}
	if res.Available {
		t.Error("Available = true, want false")
	}
	if got, _ := os.ReadFile(exe); string(got) != "OLD" {
		t.Errorf("binary was modified: %q, want OLD", got)
	}
}

func TestRun_ForceReinstall(t *testing.T) {
	asset := AssetName(linuxAMD64)
	newBytes := []byte("REINSTALLED")
	tarball := buildTarGz(t, map[string][]byte{"dotf": newBytes})
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(tarball), asset)

	newReleaseServer(t, asset, tarball, checksums)
	withLatest(t, "v0.2.0")
	withVersion(t, "0.2.0") // equal → only --force acts
	exe := installBinary(t, "OLD", 0o755)

	res, err := Run(Options{Force: true}, linuxAMD64)
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if !res.Updated {
		t.Error("Updated = false, want true (forced reinstall)")
	}
	if got, _ := os.ReadFile(exe); !bytes.Equal(got, newBytes) {
		t.Errorf("binary = %q, want reinstalled bytes", got)
	}
}

func TestRun_ForceDowngrade(t *testing.T) {
	asset := AssetName(linuxAMD64)
	newBytes := []byte("DOWNGRADED")
	tarball := buildTarGz(t, map[string][]byte{"dotf": newBytes})
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(tarball), asset)

	newReleaseServer(t, asset, tarball, checksums)
	withLatest(t, "v0.1.0")
	withVersion(t, "0.5.0") // current newer → downgrade only with --force
	installBinary(t, "OLD", 0o755)

	res, err := Run(Options{Force: true}, linuxAMD64)
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Available {
		t.Error("Available = true, want false (current is newer)")
	}
	if !res.Updated {
		t.Error("Updated = false, want true (forced downgrade)")
	}
}

// --- Run: dev build ----------------------------------------------------------

func TestRun_DevBuild_CheckReportsWithoutError(t *testing.T) {
	withLatest(t, "v0.2.0")
	withVersion(t, "dev")

	res, err := Run(Options{Check: true}, linuxAMD64)
	if err != nil {
		t.Fatalf("Run(check) on dev build err = %v, want nil", err)
	}
	if res.Current != "dev" {
		t.Errorf("Current = %q, want dev", res.Current)
	}
	if !res.Available {
		t.Error("Available = false, want true for a dev build")
	}
	if res.Updated {
		t.Error("Updated = true, want false for --check")
	}
}

func TestRun_DevBuild_DefaultErrDevBuild(t *testing.T) {
	withLatest(t, "v0.2.0")
	withVersion(t, "dev")

	_, err := Run(Options{}, linuxAMD64)
	if !errors.Is(err, ErrDevBuild) {
		t.Fatalf("Run(dev, default) err = %v, want ErrDevBuild", err)
	}
}

func TestRun_DevBuild_ForceReplaces(t *testing.T) {
	asset := AssetName(linuxAMD64)
	newBytes := []byte("DEV-FORCED")
	tarball := buildTarGz(t, map[string][]byte{"dotf": newBytes})
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(tarball), asset)

	newReleaseServer(t, asset, tarball, checksums)
	withLatest(t, "v0.2.0")
	withVersion(t, "dev")
	exe := installBinary(t, "OLD", 0o755)

	res, err := Run(Options{Force: true}, linuxAMD64)
	if err != nil {
		t.Fatalf("Run(dev, force) err = %v", err)
	}
	if !res.Updated {
		t.Error("Updated = false, want true (dev + force)")
	}
	if got, _ := os.ReadFile(exe); !bytes.Equal(got, newBytes) {
		t.Errorf("binary = %q, want forced bytes", got)
	}
}

// --- Run: verification failures ----------------------------------------------

func TestRun_ChecksumMismatchLeavesBinary(t *testing.T) {
	asset := AssetName(linuxAMD64)
	tarball := buildTarGz(t, map[string][]byte{"dotf": []byte("EVIL")})
	// A syntactically valid but wrong checksum → VerifySHA256 mismatch.
	checksums := fmt.Sprintf("%s  %s\n", strings.Repeat("0", 64), asset)

	newReleaseServer(t, asset, tarball, checksums)
	withLatest(t, "v0.2.0")
	withVersion(t, "0.1.0")
	exe := installBinary(t, "OLD", 0o755)

	res, err := Run(Options{}, linuxAMD64)
	if err == nil {
		t.Fatal("Run err = nil, want a checksum mismatch error")
	}
	if res.Updated {
		t.Error("Updated = true, want false on mismatch")
	}
	if got, _ := os.ReadFile(exe); string(got) != "OLD" {
		t.Errorf("binary changed to %q despite mismatch, want OLD", got)
	}
}

func TestRun_MissingChecksumLineAborts(t *testing.T) {
	asset := AssetName(linuxAMD64)
	tarball := buildTarGz(t, map[string][]byte{"dotf": []byte("NEW")})
	// checksums.txt exists but has no line for THIS asset (fail-closed).
	checksums := "deadbeef  some_other_asset.tar.gz\n"

	rs := newReleaseServer(t, asset, tarball, checksums)
	withLatest(t, "v0.2.0")
	withVersion(t, "0.1.0")
	exe := installBinary(t, "OLD", 0o755)

	res, err := Run(Options{}, linuxAMD64)
	if err == nil {
		t.Fatal("Run err = nil, want an abort on a missing checksum line")
	}
	if !strings.Contains(err.Error(), "no line for asset") {
		t.Errorf("err = %v, want it to mention the missing checksum line", err)
	}
	if res.Updated {
		t.Error("Updated = true, want false")
	}
	if rs.assetHits.Load() != 0 {
		t.Errorf("asset download hits = %d, want 0 (must abort before downloading)", rs.assetHits.Load())
	}
	if got, _ := os.ReadFile(exe); string(got) != "OLD" {
		t.Errorf("binary changed to %q, want OLD", got)
	}
}

// --- Run: platform guard -----------------------------------------------------

func TestRun_UnsupportedOS(t *testing.T) {
	for _, info := range []platform.Info{
		{OS: platform.OSWindows, Arch: platform.ArchX86_64},
		{OS: platform.OSUnknown, Arch: platform.ArchX86_64},
	} {
		_, err := Run(Options{Check: true}, info)
		if !errors.Is(err, ErrUnsupportedOS) {
			t.Errorf("Run(%s) err = %v, want ErrUnsupportedOS", info.OS, err)
		}
	}
}

// --- Run: read-only paths never download -------------------------------------

func TestRun_CheckAndDryRunDoNotDownload(t *testing.T) {
	asset := AssetName(linuxAMD64)
	tarball := buildTarGz(t, map[string][]byte{"dotf": []byte("NEW")})
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(tarball), asset)

	withVersion(t, "0.1.0")
	withLatest(t, "v0.2.0")
	exe := installBinary(t, "OLD", 0o755)

	t.Run("check", func(t *testing.T) {
		rs := newReleaseServer(t, asset, tarball, checksums)
		res, err := Run(Options{Check: true}, linuxAMD64)
		if err != nil {
			t.Fatalf("Run(check) err = %v", err)
		}
		if res.Target != "" {
			t.Errorf("Target = %q, want empty for --check", res.Target)
		}
		if rs.assetHits.Load() != 0 || rs.checksumHits.Load() != 0 {
			t.Errorf("hits asset=%d checksum=%d, want 0/0 for --check", rs.assetHits.Load(), rs.checksumHits.Load())
		}
	})

	t.Run("dry-run", func(t *testing.T) {
		rs := newReleaseServer(t, asset, tarball, checksums)
		res, err := Run(Options{DryRun: true}, linuxAMD64)
		if err != nil {
			t.Fatalf("Run(dry-run) err = %v", err)
		}
		if res.Target != exe {
			t.Errorf("Target = %q, want %q for --dry-run", res.Target, exe)
		}
		if res.Updated {
			t.Error("Updated = true, want false for --dry-run")
		}
		if rs.assetHits.Load() != 0 || rs.checksumHits.Load() != 0 {
			t.Errorf("hits asset=%d checksum=%d, want 0/0 for --dry-run", rs.assetHits.Load(), rs.checksumHits.Load())
		}
		if got, _ := os.ReadFile(exe); string(got) != "OLD" {
			t.Errorf("binary changed to %q on dry-run, want OLD", got)
		}
	})
}

// --- Run: permission denied --------------------------------------------------

func TestRun_PermissionDeniedFailsFast(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: a read-only directory is still writable")
	}
	asset := AssetName(linuxAMD64)
	tarball := buildTarGz(t, map[string][]byte{"dotf": []byte("NEW")})
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(tarball), asset)

	newReleaseServer(t, asset, tarball, checksums)
	withLatest(t, "v0.2.0")
	withVersion(t, "0.1.0")

	// A binary in a directory with no write bit: the same-dir temp create (and
	// the rename) hit EACCES, and self-update must fail fast rather than escalate.
	dir := t.TempDir()
	exe := filepath.Join(dir, "dotf")
	if err := os.WriteFile(exe, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // let TempDir cleanup remove it
	orig := executablePath
	executablePath = func() (string, error) { return exe, nil }
	t.Cleanup(func() { executablePath = orig })

	res, err := Run(Options{}, linuxAMD64)
	if err == nil {
		t.Fatal("Run err = nil, want a permission-denied error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("err = %v, want it to mention permission denied", err)
	}
	if res.Updated {
		t.Error("Updated = true, want false")
	}
	if got, _ := os.ReadFile(exe); string(got) != "OLD" {
		t.Errorf("binary changed to %q despite permission failure, want OLD", got)
	}
}

// --- bootstrap.sh cross-check (AC #2) ----------------------------------------

func TestAssetNaming_MatchesBootstrap(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "bootstrap.sh"))
	if err != nil {
		t.Fatalf("read bootstrap.sh: %v", err)
	}
	text := string(data)

	// The asset template literal bootstrap.sh builds from BINARY/OS/ARCH.
	if !strings.Contains(text, `ASSET="${BINARY}_${OS}_${ARCH}.tar.gz"`) {
		t.Error(`bootstrap.sh ASSET= line drifted from "${BINARY}_${OS}_${ARCH}.tar.gz"`)
	}
	if !strings.Contains(text, `BINARY="dotf"`) {
		t.Error(`bootstrap.sh BINARY drifted from "dotf"`)
	}
	// The checksum sidecar literal.
	if !strings.Contains(text, ChecksumsAsset) {
		t.Errorf("bootstrap.sh no longer references %q", ChecksumsAsset)
	}
	if ChecksumsAsset != "checksums.txt" {
		t.Errorf("ChecksumsAsset = %q, want checksums.txt (matches .goreleaser.yml + bootstrap.sh)", ChecksumsAsset)
	}

	// Go AssetName must equal bootstrap's ${BINARY}_${OS}_${ARCH}.tar.gz for
	// every os/arch bootstrap.sh maps (linux/darwin × amd64/arm64).
	combos := []struct {
		os          platform.OS
		arch        platform.Arch
		goos, garch string
	}{
		{platform.OSLinux, platform.ArchX86_64, "linux", "amd64"},
		{platform.OSLinux, platform.ArchARM64, "linux", "arm64"},
		{platform.OSMacOS, platform.ArchX86_64, "darwin", "amd64"},
		{platform.OSMacOS, platform.ArchARM64, "darwin", "arm64"},
	}
	for _, c := range combos {
		want := fmt.Sprintf("dotf_%s_%s.tar.gz", c.goos, c.garch) // ${BINARY}_${OS}_${ARCH}.tar.gz
		if got := AssetName(platform.Info{OS: c.os, Arch: c.arch}); got != want {
			t.Errorf("AssetName(%s/%s) = %q, want %q (bootstrap ${BINARY}_${OS}_${ARCH})", c.os, c.arch, got, want)
		}
	}

	// Prerelease-aware discovery + source-build escape hatch (bootstrap pins).
	for _, marker := range []string{
		"github_resolve_latest_tag",
		"releases/tags/",
		"--install-go",
		".local/go",
		"GO_BOOTSTRAP_VERSION",
		"offer_source_build_or_exit",
	} {
		if !strings.Contains(text, marker) {
			t.Errorf("bootstrap.sh missing pin marker %q", marker)
		}
	}
	// The old line-oriented sed over GET /releases (name-then-id) always missed
	// GitHub's id-before-name JSON; must stay gone.
	brokenSed := `sed -n 's/.*"name"`
	if strings.Contains(text, brokenSed) {
		t.Error("bootstrap.sh still contains broken name-then-id sed over /releases")
	}
}
