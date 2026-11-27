package backend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestFetchSHA256(t *testing.T) {
	payload := []byte("hello, lockfile\n")
	want := sha256Hex(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = srv.Client().Get
	t.Cleanup(func() { httpGet = origHTTPGet })

	got, err := fetchSHA256(srv.URL + "/asset")
	if err != nil {
		t.Fatalf("fetchSHA256() error = %v, want nil", err)
	}
	if got != want {
		t.Errorf("fetchSHA256() = %q, want %q", got, want)
	}
}

func TestFetchSHA256_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = srv.Client().Get
	t.Cleanup(func() { httpGet = origHTTPGet })

	if _, err := fetchSHA256(srv.URL + "/missing"); err == nil {
		t.Fatal("fetchSHA256() error = nil, want non-200 error")
	}
}

// TestGitHubRelease_Resolve_RecordsDeclaredSHA pins the #45 change: resolve()
// records the DECLARED (inline) sha and the templated asset WITHOUT a full asset
// download — the record path no longer streams+hashes the binary.
func TestGitHubRelease_Resolve_RecordsDeclaredSHA(t *testing.T) {
	called := false
	origHTTPGet := httpGet
	httpGet = func(url string) (*http.Response, error) {
		called = true
		return origHTTPGet(url)
	}
	t.Cleanup(func() { httpGet = origHTTPGet })

	inlineSHA := strings.Repeat("a", 64)
	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	pkg := manifest.Package{
		Name:    "tool",
		Backend: manifest.BackendGitHubRelease,
		Repo:    "owner/tool",
		Tag:     "v1.2.3",
		Asset:   "{{.OS}}/{{.Arch}}/tool.tar.gz",
		SHA256:  inlineSHA,
	}
	info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}

	art, err := b.resolve(pkg, info)
	if err != nil {
		t.Fatalf("resolve() error = %v, want nil", err)
	}
	if art.OS != "arch" || art.Arch != "amd64" {
		t.Errorf("resolve() os/arch = %q/%q, want arch/amd64", art.OS, art.Arch)
	}
	if art.Asset != "arch/amd64/tool.tar.gz" {
		t.Errorf("resolve() asset = %q, want arch/amd64/tool.tar.gz", art.Asset)
	}
	if art.Binary != "tool" {
		t.Errorf("resolve() binary = %q, want tool", art.Binary)
	}
	if art.SHA256 != inlineSHA {
		t.Errorf("resolve() sha256 = %q, want the declared %q", art.SHA256, inlineSHA)
	}
	if called {
		t.Error("resolve() downloaded the asset; #45 records the declared sha without a full download")
	}
}

// TestGitHubRelease_Resolve_AutoDetectSidecar exercises the record path with
// auto-detection (empty Asset) and a ChecksumsAsset sidecar: resolve() lists the
// release, matches the host asset, and reads its sha from the sidecar — again
// without a full asset download.
func TestGitHubRelease_Resolve_AutoDetectSidecar(t *testing.T) {
	const asset = "tool_Linux_x86_64.tar.gz"
	sidecarSHA := strings.Repeat("b", 64)

	origList := listReleaseAssets
	listReleaseAssets = func(_, _ string) ([]string, error) {
		return []string{asset, "tool_Darwin_arm64.tar.gz", "checksums.txt"}, nil
	}
	t.Cleanup(func() { listReleaseAssets = origList })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sidecarSHA + "  " + asset + "\n"))
	}))
	defer srv.Close()
	origHTTPGet := httpGet
	httpGet = func(_ string) (*http.Response, error) { return srv.Client().Get(srv.URL + "/checksums.txt") }
	t.Cleanup(func() { httpGet = origHTTPGet })

	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	pkg := manifest.Package{
		Name:           "tool",
		Backend:        manifest.BackendGitHubRelease,
		Repo:           "owner/tool",
		Tag:            "v1.2.3",
		ChecksumsAsset: "checksums.txt",
	}
	info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}

	art, err := b.resolve(pkg, info)
	if err != nil {
		t.Fatalf("resolve() error = %v, want nil", err)
	}
	if art.Asset != asset {
		t.Errorf("resolve() asset = %q, want auto-detected %q", art.Asset, asset)
	}
	if art.SHA256 != sidecarSHA {
		t.Errorf("resolve() sha256 = %q, want sidecar %q", art.SHA256, sidecarSHA)
	}
}

func TestGitHubRelease_Install_LockHitOverridesTemplateAndSHA(t *testing.T) {
	payload := []byte("#!/bin/sh\necho locked\n")
	archive := buildTarGz(t, map[string]tarEntry{"tool": {data: payload, mode: 0o755}})
	lockedSHA := sha256Hex(archive)

	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = func(url string) (*http.Response, error) {
		gotURL = url
		return http.Get(srv.URL + "/release.tar.gz")
	}
	t.Cleanup(func() { httpGet = origHTTPGet })

	lock := &lockfile.Lock{
		Version: lockfile.Version,
		Packages: []lockfile.LockedPackage{{
			Name:    "tool",
			Backend: "github-release",
			Tag:     "v9.9.9",
			Artifacts: []lockfile.Artifact{{
				OS: "arch", Arch: "amd64", Asset: "locked/tool.tar.gz", Binary: "tool", SHA256: lockedSHA,
			}},
		}},
	}

	home := t.TempDir()
	b := newGitHubReleaseBackend(exec.NewFake(), lock)
	b.installDir = func() (string, error) { return filepath.Join(home, ".local", "bin"), nil }

	// The manifest asset template and inline sha256 both DISAGREE with the lock;
	// the lock must win.
	pkg := manifest.Package{
		Name:    "tool",
		Backend: manifest.BackendGitHubRelease,
		Repo:    "owner/tool",
		Tag:     "v0.0.1",
		Asset:   "{{.OS}}/{{.Arch}}/other.tar.gz",
		SHA256:  strings.Repeat("0", 64),
	}
	info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}
	if want := "https://github.com/owner/tool/releases/download/v9.9.9/locked/tool.tar.gz"; gotURL != want {
		t.Errorf("Install() download URL = %q, want the LOCKED tag+asset %q", gotURL, want)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "tool")); err != nil {
		t.Errorf("Install() did not place the binary: %v", err)
	}
}

func TestGitHubRelease_Install_LockHitWrongSHAAborts(t *testing.T) {
	archive := buildTarGz(t, map[string]tarEntry{"tool": {data: []byte("payload"), mode: 0o755}})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = func(_ string) (*http.Response, error) { return http.Get(srv.URL + "/release.tar.gz") }
	t.Cleanup(func() { httpGet = origHTTPGet })

	lock := &lockfile.Lock{
		Version: lockfile.Version,
		Packages: []lockfile.LockedPackage{{
			Name: "tool", Backend: "github-release", Tag: "v1",
			Artifacts: []lockfile.Artifact{{OS: "arch", Arch: "amd64", Asset: "a.tar.gz", Binary: "tool", SHA256: strings.Repeat("0", 64)}},
		}},
	}

	home := t.TempDir()
	installDir := filepath.Join(home, ".local", "bin")
	b := newGitHubReleaseBackend(exec.NewFake(), lock)
	b.installDir = func() (string, error) { return installDir, nil }

	pkg := manifest.Package{Name: "tool", Backend: manifest.BackendGitHubRelease, Repo: "owner/tool", Asset: "a.tar.gz", SHA256: strings.Repeat("a", 64)}
	info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}

	err := b.Install(pkg, info)
	if err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("Install() error = %v, want SHA256 mismatch (locked sha wins)", err)
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "tool")); !os.IsNotExist(statErr) {
		t.Fatalf("binary installed after locked checksum failure")
	}
}

func TestGitHubRelease_Install_PinnedNoChecksumFailsFast(t *testing.T) {
	called := false
	origHTTPGet := httpGet
	httpGet = func(url string) (*http.Response, error) {
		called = true
		return origHTTPGet(url)
	}
	t.Cleanup(func() { httpGet = origHTTPGet })

	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	pkg := manifest.Package{Name: "tool", Backend: manifest.BackendGitHubRelease, Repo: "owner/tool", Asset: "tool.tar.gz"}
	info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}

	err := b.Install(pkg, info)
	if err == nil || !strings.Contains(err.Error(), "run `dotf lock`") {
		t.Fatalf("Install() error = %v, want the pinned-no-checksum error", err)
	}
	if called {
		t.Error("Install() downloaded despite no resolvable checksum, want fail-fast before download")
	}
}

func TestScriptURL_Install_LockHitVerifiesLockedSHA(t *testing.T) {
	script := []byte("#!/usr/bin/env bash\necho locked-install\n")
	lockedSHA := sha256Hex(script)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(script)
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = srv.Client().Get
	t.Cleanup(func() { httpGet = origHTTPGet })

	lock := &lockfile.Lock{
		Version: lockfile.Version,
		Packages: []lockfile.LockedPackage{{
			Name: "pin", Backend: "script-url", URL: srv.URL + "/install.sh",
			Artifacts: []lockfile.Artifact{{SHA256: lockedSHA}},
		}},
	}

	fake := exec.NewFake()
	b := newScriptURLBackend(fake, lock)
	// Inline sha256 is empty; the LOCKED sha must drive verification.
	pkg := manifest.Package{Name: "pin", Backend: manifest.BackendScriptURL, URL: srv.URL + "/install.sh"}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}
	if len(fake.CallsSnapshot()) != 1 {
		t.Fatalf("Run calls = %d, want 1", len(fake.CallsSnapshot()))
	}
}

func TestScriptURL_Install_LockHitWrongSHAAborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("#!/bin/sh\necho nope\n"))
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = srv.Client().Get
	t.Cleanup(func() { httpGet = origHTTPGet })

	lock := &lockfile.Lock{
		Version: lockfile.Version,
		Packages: []lockfile.LockedPackage{{
			Name: "pin", Backend: "script-url", URL: srv.URL + "/install.sh",
			Artifacts: []lockfile.Artifact{{SHA256: strings.Repeat("0", 64)}},
		}},
	}

	fake := exec.NewFake()
	b := newScriptURLBackend(fake, lock)
	pkg := manifest.Package{Name: "pin", Backend: manifest.BackendScriptURL, URL: srv.URL + "/install.sh"}

	err := b.Install(pkg, platform.Info{})
	if err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("Install() error = %v, want SHA256 mismatch from the locked sha", err)
	}
	if len(fake.CallsSnapshot()) != 0 {
		t.Fatalf("installer ran after locked checksum failure")
	}
}

func TestScriptURL_Install_PinnedNoChecksumFailsFast(t *testing.T) {
	called := false
	origHTTPGet := httpGet
	httpGet = func(url string) (*http.Response, error) {
		called = true
		return origHTTPGet(url)
	}
	t.Cleanup(func() { httpGet = origHTTPGet })

	fake := exec.NewFake()
	b := newScriptURLBackend(fake, nil)
	// Explicit pinned class with no inline sha and no lock: must fail fast.
	pkg := manifest.Package{Name: "pin", Backend: manifest.BackendScriptURL, URL: "https://example.test/install.sh", Class: manifest.ClassPinned}

	err := b.Install(pkg, platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64})
	if err == nil || !strings.Contains(err.Error(), "run `dotf lock`") {
		t.Fatalf("Install() error = %v, want the pinned-no-checksum error", err)
	}
	if called {
		t.Error("Install() downloaded despite no resolvable checksum, want fail-fast")
	}
	if len(fake.CallsSnapshot()) != 0 {
		t.Error("installer ran despite fail-fast")
	}
}

func TestScriptURL_Resolve_RecordsPlatformAgnosticArtifact(t *testing.T) {
	script := []byte("#!/usr/bin/env bash\necho resolve\n")
	wantSHA := sha256Hex(script)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(script)
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = srv.Client().Get
	t.Cleanup(func() { httpGet = origHTTPGet })

	b := newScriptURLBackend(exec.NewFake(), nil)
	pkg := manifest.Package{Name: "pin", Backend: manifest.BackendScriptURL, URL: srv.URL + "/install.sh"}

	art, err := b.resolve(pkg, platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64})
	if err != nil {
		t.Fatalf("resolve() error = %v, want nil", err)
	}
	if art.OS != "" || art.Arch != "" || art.Asset != "" || art.Binary != "" {
		t.Errorf("resolve() = %+v, want platform-agnostic (empty os/arch/asset/binary)", art)
	}
	if art.SHA256 != wantSHA {
		t.Errorf("resolve() sha256 = %q, want %q", art.SHA256, wantSHA)
	}
}

// TestScriptURL_Install_RollingUnverified pins today's behavior: a rolling
// installer (no inline sha, no lock) fetches and runs unverified.
func TestScriptURL_Install_RollingUnverified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("#!/usr/bin/env bash\necho rolling\n"))
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = srv.Client().Get
	t.Cleanup(func() { httpGet = origHTTPGet })

	fake := exec.NewFake()
	b := newScriptURLBackend(fake, nil)
	pkg := manifest.Package{Name: "starship", Backend: manifest.BackendScriptURL, URL: srv.URL + "/install.sh"}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil (rolling runs unverified)", err)
	}
	if len(fake.CallsSnapshot()) != 1 {
		t.Fatalf("Run calls = %d, want 1", len(fake.CallsSnapshot()))
	}
}

// TestGitHubRelease_Resolve_UncoveredTargetErrors is the YAN-1417 repro: a
// pinned package whose checksums table covers only linux/x86_64 must not
// resolve debian/arm64 to an empty sha256 with a nil error. The covered target
// still resolves to its declared checksum.
func TestGitHubRelease_Resolve_UncoveredTargetErrors(t *testing.T) {
	called := false
	origHTTPGet := httpGet
	httpGet = func(url string) (*http.Response, error) {
		called = true
		return origHTTPGet(url)
	}
	t.Cleanup(func() { httpGet = origHTTPGet })

	covered := strings.Repeat("a", 64)
	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	pkg := manifest.Package{
		Name:      "tool",
		Backend:   manifest.BackendGitHubRelease,
		Repo:      "owner/tool",
		Tag:       "v1.2.3",
		Asset:     "tool-{{.Arch}}.tar.gz",
		Checksums: map[string]string{"linux/x86_64": covered},
	}

	art, err := b.resolve(pkg, platform.Info{OS: platform.OSDebian, Arch: platform.ArchARM64})
	if err == nil {
		t.Fatalf("resolve(debian/arm64) = %+v, nil; want an unresolved-checksum error", art)
	}
	if !strings.Contains(err.Error(), "resolves no checksum") || !strings.Contains(err.Error(), "debian/arm64") {
		t.Errorf("resolve(debian/arm64) error = %v, want it to name the uncovered target", err)
	}
	if art.SHA256 != "" || art.Asset != "" {
		t.Errorf("resolve(debian/arm64) artifact = %+v, want the zero artifact on error", art)
	}

	art, err = b.resolve(pkg, platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64})
	if err != nil {
		t.Fatalf("resolve(debian/x86_64) error = %v, want nil for a covered target", err)
	}
	if art.SHA256 != covered {
		t.Errorf("resolve(debian/x86_64) sha256 = %q, want %q", art.SHA256, covered)
	}
	if called {
		t.Error("resolve() downloaded; a declared checksum needs no download")
	}
}

// emptySHAResolver is a lockable backend stub that resolves an artifact with no
// checksum, exercising Engine.Resolve's backstop.
type emptySHAResolver struct{ Backend }

func (emptySHAResolver) resolve(manifest.Package, platform.Info) (lockfile.Artifact, error) {
	return lockfile.Artifact{OS: "linux", Arch: "amd64", Asset: "a"}, nil
}

// TestEngineResolve_EmptySHAIsError asserts the engine never hands `dotf lock`
// an artifact without a checksum, whichever lockable backend produced it.
func TestEngineResolve_EmptySHAIsError(t *testing.T) {
	e := NewEngine(map[manifest.Backend]Backend{manifest.BackendGitHubRelease: emptySHAResolver{}})
	pkg := manifest.Package{Name: "tool", Backend: manifest.BackendGitHubRelease}

	_, err := e.Resolve(pkg, platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64})
	if err == nil || !strings.Contains(err.Error(), "resolves no checksum") {
		t.Fatalf("Engine.Resolve() error = %v, want an unresolved-checksum error", err)
	}
}
