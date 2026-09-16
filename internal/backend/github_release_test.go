package backend

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestGitHubRelease_AssetTemplating(t *testing.T) {
	tests := []struct {
		name      string
		info      platform.Info
		tag       string
		wantAsset string
	}{
		{
			name: "arch linux amd64",
			info: platform.Info{
				OS:   platform.OSArch,
				Arch: platform.ArchX86_64,
			},
			wantAsset: "arch/amd64/tool.tar.gz",
		},
		{
			name: "debian arm64",
			info: platform.Info{
				OS:   platform.OSDebian,
				Arch: platform.ArchARM64,
			},
			wantAsset: "debian/arm64/tool.tar.gz",
		},
		{
			name: "macos arm passthrough",
			info: platform.Info{
				OS:   platform.OSMacOS,
				Arch: platform.ArchARM,
			},
			wantAsset: "macos/arm/tool.tar.gz",
		},
		{
			name: "pinned tag downloads from that release",
			info: platform.Info{
				OS:   platform.OSArch,
				Arch: platform.ArchX86_64,
			},
			tag:       "v1.2.3",
			wantAsset: "arch/amd64/tool.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := []byte("#!/bin/sh\necho tool\n")
			archive := buildTarGz(t, map[string]tarEntry{
				"tool": {data: payload, mode: 0o755},
			})
			wantSHA := sha256Hex(archive)

			var gotURL string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(archive)
			}))
			defer srv.Close()

			prevHTTPGet := httpGet
			httpGet = func(url string) (*http.Response, error) {
				gotURL = url
				return http.Get(srv.URL + "/release.tar.gz")
			}
			t.Cleanup(func() { httpGet = prevHTTPGet })

			home := t.TempDir()

			b := newGitHubReleaseBackend(exec.NewFake(), nil)
			b.installDir = func() (string, error) {
				return filepath.Join(home, ".local", "bin"), nil
			}

			pkg := manifest.Package{
				Name:    "tool",
				Backend: manifest.BackendGitHubRelease,
				Repo:    "test-owner/test-repo",
				Tag:     tt.tag,
				Asset:   "{{.OS}}/{{.Arch}}/tool.tar.gz",
				SHA256:  wantSHA,
			}

			if err := b.Install(pkg, tt.info); err != nil {
				t.Fatalf("Install() error = %v, want nil", err)
			}

			wantURL := "https://github.com/test-owner/test-repo/releases/latest/download/" + tt.wantAsset
			if tt.tag != "" {
				wantURL = "https://github.com/test-owner/test-repo/releases/download/" + tt.tag + "/" + tt.wantAsset
			}
			if gotURL != wantURL {
				t.Errorf("download URL = %q, want %q", gotURL, wantURL)
			}

			installed, err := os.ReadFile(filepath.Join(home, ".local", "bin", "tool"))
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if !bytes.Equal(installed, payload) {
				t.Errorf("installed binary = %q, want %q", installed, payload)
			}
		})
	}
}

func TestGitHubRelease_SHA256Mismatch_AbortsWithoutInstall(t *testing.T) {
	// NOT parallel: this test reassigns the package-global httpGet, which would
	// data-race with any other parallel test that also stubs it under -race.
	payload := []byte("#!/bin/sh\necho tool\n")
	archive := buildTarGz(t, map[string]tarEntry{
		"tool": {data: payload, mode: 0o755},
	})
	wrongSHA := strings.Repeat("0", 64)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	prevHTTPGet := httpGet
	httpGet = func(_ string) (*http.Response, error) {
		return http.Get(srv.URL + "/release.tar.gz")
	}
	t.Cleanup(func() { httpGet = prevHTTPGet })

	home := t.TempDir()
	installDir := filepath.Join(home, ".local", "bin")

	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	b.installDir = func() (string, error) {
		return installDir, nil
	}

	pkg := manifest.Package{
		Name:    "tool",
		Backend: manifest.BackendGitHubRelease,
		Repo:    "test-owner/test-repo",
		Asset:   "tool.tar.gz",
		SHA256:  wrongSHA,
	}
	info := platform.Info{OS: platform.OSLinux, Arch: platform.ArchX86_64}

	err := b.Install(pkg, info)
	if err == nil {
		t.Fatal("Install() error = nil, want SHA256 mismatch error")
	}
	if !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("Install() error = %v, want SHA256 mismatch", err)
	}

	if _, statErr := os.Stat(filepath.Join(installDir, "tool")); !os.IsNotExist(statErr) {
		t.Fatalf("binary installed after checksum failure: stat err = %v", statErr)
	}
}

// TestGitHubRelease_Install_UnresolvedChecksum_FailsClosed proves the security
// fence: a Pinnable github-release whose checksum source does not resolve for the
// running host (a partial Checksums map missing this target, no inline sha256 or
// sidecar) must fail BEFORE any download rather than install unverified.
func TestGitHubRelease_Install_UnresolvedChecksum_FailsClosed(t *testing.T) {
	// NOT parallel: reassigns the package-global httpGet (see the note on
	// TestGitHubRelease_SHA256Mismatch_AbortsWithoutInstall).
	prevHTTPGet := httpGet
	httpGet = func(_ string) (*http.Response, error) {
		t.Fatal("downloaded despite an unresolved checksum")
		return nil, nil
	}
	t.Cleanup(func() { httpGet = prevHTTPGet })

	home := t.TempDir()
	installDir := filepath.Join(home, ".local", "bin")
	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	b.installDir = func() (string, error) { return installDir, nil }

	pkg := manifest.Package{
		Name:    "tool",
		Backend: manifest.BackendGitHubRelease,
		Repo:    "owner/tool",
		Tag:     "v1",
		Asset:   "tool_{{.ReleaseOS}}_{{.Arch}}.tar.gz", // explicit => no listReleaseAssets
		Checksums: map[string]string{
			"darwin/arm64": strings.Repeat("a", 64), // wrong target for the host below
		},
	}
	info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64} // targetKey linux/x86_64, absent

	err := b.Install(pkg, info)
	if err == nil || !strings.Contains(err.Error(), "has no locked checksum") {
		t.Fatalf("Install() error = %v, want a fail-closed 'no locked checksum' error", err)
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "tool")); !os.IsNotExist(statErr) {
		t.Fatalf("binary installed despite unresolved checksum: stat err = %v", statErr)
	}
}

// TestGitHubRelease_Installed_BinaryOverride guards the probe against the Binary
// override: a package installed under Binary (!= Name) must be seen as installed
// when Binary is on PATH, so status/diff are honest and apply skips it.
func TestGitHubRelease_Installed_BinaryOverride(t *testing.T) {
	t.Parallel()

	pkg := manifest.Package{Name: "hadolint-pkg", Binary: "hadolint", Backend: manifest.BackendGitHubRelease}

	fake := exec.NewFake()
	fake.AddPath("hadolint", "/home/user/.local/bin/hadolint") // installed name, not pkg.Name
	b := newGitHubReleaseBackend(fake, nil)
	installed, err := b.Installed(pkg, platform.Info{})
	if err != nil || !installed {
		t.Fatalf("Installed() = (%v, %v), want (true, nil) when the Binary override is on PATH", installed, err)
	}

	// Not installed when neither name resolves.
	b2 := newGitHubReleaseBackend(exec.NewFake(), nil)
	if installed, _ := b2.Installed(pkg, platform.Info{}); installed {
		t.Fatal("Installed() = true, want false when the Binary override is absent from PATH")
	}
}

// TestFindReleaseBinary_Errors covers the three typed error branches of the
// relaxed matcher: multiple name matches, no files, and multiple files with no
// name match.
func TestFindReleaseBinary_Errors(t *testing.T) {
	t.Parallel()

	writeFile := func(dir, rel string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	t.Run("multiple name matches", func(t *testing.T) {
		root := t.TempDir()
		writeFile(root, "x/tool")
		writeFile(root, "y/tool")
		if _, err := findReleaseBinary(root, "tool"); err == nil || !strings.Contains(err.Error(), "multiple files named") {
			t.Fatalf("err = %v, want 'multiple files named'", err)
		}
	})

	t.Run("no files", func(t *testing.T) {
		if _, err := findReleaseBinary(t.TempDir(), "tool"); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("err = %v, want 'not found'", err)
		}
	})

	t.Run("multiple files no name match", func(t *testing.T) {
		root := t.TempDir()
		writeFile(root, "a")
		writeFile(root, "b")
		if _, err := findReleaseBinary(root, "tool"); err == nil || !strings.Contains(err.Error(), "could not identify") {
			t.Fatalf("err = %v, want 'could not identify'", err)
		}
	})
}

func TestGitHubRelease_Installed(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	fake.AddPath("eza", "/home/user/.local/bin/eza")

	b := newGitHubReleaseBackend(fake, nil)
	pkg := manifest.Package{Name: "eza", Backend: manifest.BackendGitHubRelease}

	installed, err := b.Installed(pkg, platform.Info{})
	if err != nil {
		t.Fatalf("Installed() error = %v, want nil", err)
	}
	if !installed {
		t.Fatal("Installed() = false, want true")
	}
}

func TestGitHubRelease_Installed_NotFound(t *testing.T) {
	t.Parallel()

	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	pkg := manifest.Package{Name: "missing-tool", Backend: manifest.BackendGitHubRelease}

	installed, err := b.Installed(pkg, platform.Info{})
	if err != nil {
		t.Fatalf("Installed() error = %v, want nil", err)
	}
	if installed {
		t.Fatal("Installed() = true, want false")
	}
}

func TestGitHubRelease_InvalidRepo(t *testing.T) {
	t.Parallel()

	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	pkg := manifest.Package{
		Name:    "tool",
		Backend: manifest.BackendGitHubRelease,
		Repo:    "not-a-valid-repo",
		Asset:   "tool.tar.gz",
		SHA256:  strings.Repeat("a", 64),
	}

	err := b.Install(pkg, platform.Info{})
	if err == nil {
		t.Fatal("Install() error = nil, want invalid repo error")
	}
	if !strings.Contains(err.Error(), "invalid repo") {
		t.Fatalf("Install() error = %v, want invalid repo", err)
	}
}

func TestTemplateGitHubAsset(t *testing.T) {
	t.Parallel()

	got := templateGitHubAsset("{{.OS}}/{{.Arch}}/app.tar.gz", platform.Info{
		OS:   platform.OSArch,
		Arch: platform.ArchX86_64,
	})
	want := "arch/amd64/app.tar.gz"
	if got != want {
		t.Errorf("templateGitHubAsset() = %q, want %q", got, want)
	}
}

func TestTemplateGitHubAsset_ReleaseOS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info platform.Info
		want string
	}{
		{
			name: "macOS spells darwin",
			info: platform.Info{OS: platform.OSMacOS, Arch: platform.ArchARM64},
			want: "tool_darwin_arm64.tar.gz",
		},
		{
			name: "arch spells linux",
			info: platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64},
			want: "tool_linux_amd64.tar.gz",
		},
		{
			name: "debian spells linux",
			info: platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64},
			want: "tool_linux_amd64.tar.gz",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := templateGitHubAsset("tool_{{.ReleaseOS}}_{{.Arch}}.tar.gz", tt.info)
			if got != tt.want {
				t.Errorf("templateGitHubAsset() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGitHubRelease_Install_AutoDetect resolves an empty Asset by matching the
// faked release page, then installs end-to-end.
func TestGitHubRelease_Install_AutoDetect(t *testing.T) {
	payload := []byte("#!/bin/sh\necho auto\n")
	archive := buildTarGz(t, map[string]tarEntry{"tool": {data: payload, mode: 0o755}})
	wantSHA := sha256Hex(archive)

	origList := listReleaseAssets
	listReleaseAssets = func(_, _ string) ([]string, error) {
		return []string{"tool_Linux_x86_64.tar.gz", "tool_Darwin_arm64.tar.gz", "checksums.txt"}, nil
	}
	t.Cleanup(func() { listReleaseAssets = origList })

	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()
	origHTTPGet := httpGet
	httpGet = func(url string) (*http.Response, error) {
		gotURL = url
		return srv.Client().Get(srv.URL + "/asset.tar.gz")
	}
	t.Cleanup(func() { httpGet = origHTTPGet })

	home := t.TempDir()
	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	b.installDir = func() (string, error) { return filepath.Join(home, ".local", "bin"), nil }

	pkg := manifest.Package{
		Name:    "tool",
		Backend: manifest.BackendGitHubRelease,
		Repo:    "owner/tool",
		Tag:     "v1.2.3",
		SHA256:  wantSHA,
	}
	info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}
	if want := "https://github.com/owner/tool/releases/download/v1.2.3/tool_Linux_x86_64.tar.gz"; gotURL != want {
		t.Errorf("download URL = %q, want the auto-detected %q", gotURL, want)
	}
	if got, err := os.ReadFile(filepath.Join(home, ".local", "bin", "tool")); err != nil || !bytes.Equal(got, payload) {
		t.Errorf("installed binary = %q (err %v), want %q", got, err, payload)
	}
}

// TestGitHubRelease_Install_BinaryOverride installs a bare binary whose asset
// name differs from Name, under the Binary override.
func TestGitHubRelease_Install_BinaryOverride(t *testing.T) {
	payload := []byte("\x7fELF hadolint-ish\n")
	wantSHA := sha256Hex(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	origHTTPGet := httpGet
	httpGet = func(_ string) (*http.Response, error) { return srv.Client().Get(srv.URL + "/bin") }
	t.Cleanup(func() { httpGet = origHTTPGet })

	home := t.TempDir()
	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	b.installDir = func() (string, error) { return filepath.Join(home, ".local", "bin"), nil }

	pkg := manifest.Package{
		Name:    "hadolint-pkg",
		Binary:  "hadolint",
		Backend: manifest.BackendGitHubRelease,
		Repo:    "hadolint/hadolint",
		Tag:     "v2.12.0",
		Asset:   "hadolint-Linux-x86_64",
		SHA256:  wantSHA,
	}
	info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}
	if got, err := os.ReadFile(filepath.Join(home, ".local", "bin", "hadolint")); err != nil || !bytes.Equal(got, payload) {
		t.Errorf("installed binary at .../hadolint = %q (err %v), want %q", got, err, payload)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "hadolint-pkg")); !os.IsNotExist(err) {
		t.Errorf("binary installed under Name, want under Binary override")
	}
}

// TestGitHubRelease_Install_GzArtifact installs a single-file .gz artifact.
func TestGitHubRelease_Install_GzArtifact(t *testing.T) {
	payload := []byte("#!/bin/sh\necho lefthook\n")
	archive := buildGz(payload)
	wantSHA := sha256Hex(archive)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()
	origHTTPGet := httpGet
	httpGet = func(_ string) (*http.Response, error) { return srv.Client().Get(srv.URL + "/lefthook.gz") }
	t.Cleanup(func() { httpGet = origHTTPGet })

	home := t.TempDir()
	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	b.installDir = func() (string, error) { return filepath.Join(home, ".local", "bin"), nil }

	pkg := manifest.Package{
		Name:    "lefthook",
		Backend: manifest.BackendGitHubRelease,
		Repo:    "evilmartians/lefthook",
		Tag:     "v1.6.10",
		Asset:   "lefthook.gz",
		SHA256:  wantSHA,
	}
	info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}
	if got, err := os.ReadFile(filepath.Join(home, ".local", "bin", "lefthook")); err != nil || !bytes.Equal(got, payload) {
		t.Errorf("installed binary = %q (err %v), want %q", got, err, payload)
	}
}

// TestGitHubRelease_Install_PerTargetChecksums selects the sha for the host via
// the Checksums map keyed by "<ReleaseOS>/<canonical-arch>".
func TestGitHubRelease_Install_PerTargetChecksums(t *testing.T) {
	payload := []byte("#!/bin/sh\necho target\n")
	archive := buildTarGz(t, map[string]tarEntry{"tool": {data: payload, mode: 0o755}})
	wantSHA := sha256Hex(archive)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()
	origHTTPGet := httpGet
	httpGet = func(_ string) (*http.Response, error) { return srv.Client().Get(srv.URL + "/asset.tar.gz") }
	t.Cleanup(func() { httpGet = origHTTPGet })

	home := t.TempDir()
	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	b.installDir = func() (string, error) { return filepath.Join(home, ".local", "bin"), nil }

	pkg := manifest.Package{
		Name:    "tool",
		Backend: manifest.BackendGitHubRelease,
		Repo:    "owner/tool",
		Tag:     "v1.2.3",
		Asset:   "tool_{{.ReleaseOS}}_{{.Arch}}.tar.gz",
		Checksums: map[string]string{
			"linux/x86_64": wantSHA,
			"darwin/arm64": strings.Repeat("0", 64),
		},
	}
	info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("Install() error = %v, want nil (per-target sha for linux/x86_64)", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "tool")); err != nil {
		t.Errorf("Install() did not place the binary: %v", err)
	}
}

// TestGitHubRelease_Install_ChecksumsAssetSidecar verifies against a sha read
// from the release's checksum sidecar (checksums.txt).
func TestGitHubRelease_Install_ChecksumsAssetSidecar(t *testing.T) {
	const asset = "tool_Linux_x86_64.tar.gz"
	payload := []byte("#!/bin/sh\necho sidecar\n")
	archive := buildTarGz(t, map[string]tarEntry{"tool": {data: payload, mode: 0o755}})
	wantSHA := sha256Hex(archive)
	sidecar := wantSHA + "  " + asset + "\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "checksums.txt") {
			_, _ = w.Write([]byte(sidecar))
			return
		}
		_, _ = w.Write(archive)
	}))
	defer srv.Close()
	origHTTPGet := httpGet
	httpGet = func(url string) (*http.Response, error) {
		// Preserve the checksums.txt vs asset routing through the same server.
		suffix := "/asset.tar.gz"
		if strings.Contains(url, "checksums.txt") {
			suffix = "/checksums.txt"
		}
		return srv.Client().Get(srv.URL + suffix)
	}
	t.Cleanup(func() { httpGet = origHTTPGet })

	home := t.TempDir()
	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	b.installDir = func() (string, error) { return filepath.Join(home, ".local", "bin"), nil }

	pkg := manifest.Package{
		Name:           "tool",
		Backend:        manifest.BackendGitHubRelease,
		Repo:           "owner/tool",
		Tag:            "v1.2.3",
		Asset:          asset,
		ChecksumsAsset: "checksums.txt",
	}
	info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("Install() error = %v, want nil (sidecar verification)", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "tool")); err != nil {
		t.Errorf("Install() did not place the binary: %v", err)
	}
}

// TestGitHubRelease_Install_AmbiguousAsset fails fast with a typed
// *AssetMatchError when auto-detection cannot disambiguate.
func TestGitHubRelease_Install_AmbiguousAsset(t *testing.T) {
	origList := listReleaseAssets
	listReleaseAssets = func(_, _ string) ([]string, error) {
		return []string{"app-linux-amd64.tar.gz", "app-linux-x86_64.tar.gz"}, nil
	}
	t.Cleanup(func() { listReleaseAssets = origList })

	b := newGitHubReleaseBackend(exec.NewFake(), nil)
	pkg := manifest.Package{
		Name:    "app",
		Backend: manifest.BackendGitHubRelease,
		Repo:    "owner/app",
		Tag:     "v1.0.0",
		SHA256:  strings.Repeat("a", 64),
	}
	info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}

	err := b.Install(pkg, info)
	var ame *AssetMatchError
	if !errors.As(err, &ame) {
		t.Fatalf("Install() error = %v, want *AssetMatchError", err)
	}
	if ame.Kind != "ambiguous" {
		t.Errorf("AssetMatchError.Kind = %q, want ambiguous", ame.Kind)
	}
	if ame.Repo != "owner/app" || ame.Tag != "v1.0.0" {
		t.Errorf("AssetMatchError repo/tag = %q/%q, want owner/app/v1.0.0 (caller enrichment)", ame.Repo, ame.Tag)
	}
}

func TestGitHubReleaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		repo  string
		tag   string
		asset string
		want  string
	}{
		{
			name:  "empty tag falls back to latest",
			repo:  "owner/tool",
			tag:   "",
			asset: "tool.tar.gz",
			want:  "https://github.com/owner/tool/releases/latest/download/tool.tar.gz",
		},
		{
			name:  "pinned tag downloads from that release",
			repo:  "owner/tool",
			tag:   "v1.2.3",
			asset: "tool.tar.gz",
			want:  "https://github.com/owner/tool/releases/download/v1.2.3/tool.tar.gz",
		},
		{
			name:  "tag is path-escaped",
			repo:  "owner/tool",
			tag:   "v1.2.3 rc1",
			asset: "tool.tar.gz",
			want:  "https://github.com/owner/tool/releases/download/v1.2.3%20rc1/tool.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := githubReleaseURL(tt.repo, tt.tag, tt.asset); got != tt.want {
				t.Errorf("githubReleaseURL(%q, %q, %q) = %q, want %q", tt.repo, tt.tag, tt.asset, got, tt.want)
			}
		})
	}
}

func TestEncodeAssetPath(t *testing.T) {
	t.Parallel()

	got := encodeAssetPath("linux amd64/tool.tar.gz")
	want := "linux%20amd64/tool.tar.gz"
	if got != want {
		t.Errorf("encodeAssetPath() = %q, want %q", got, want)
	}
}
