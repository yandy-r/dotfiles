package backend

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// A pinned direct URL resolves to just its declared checksum — no download.
func TestLockArtifactDirectURLPinnedExactHash(t *testing.T) {
	sum := sha256.Sum256([]byte("payload"))
	want := hex.EncodeToString(sum[:])
	info := platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64}

	appPkg := manifest.Package{
		Name: "pinned-app", Backend: manifest.BackendAppArtifact,
		URL: "https://vendor.example/tool.AppImage", SHA256: want,
	}
	got, err := newAppArtifactBackend(exec.NewFake()).resolve(appPkg, info)
	if err != nil {
		t.Fatalf("app resolve() error = %v", err)
	}
	if got != (lockfile.Artifact{SHA256: want}) {
		t.Errorf("app resolve() = %+v, want only SHA256 %s", got, want)
	}

	rpmPkg := manifest.Package{
		Name: "pinned-pkg", Backend: manifest.BackendPkgURL,
		URL: "https://vendor.example/tool.rpm", SHA256: want, PkgName: "tool",
	}
	got, err = newPkgURLBackend(exec.NewFake()).resolve(rpmPkg, info)
	if err != nil {
		t.Fatalf("pkg-url resolve() error = %v", err)
	}
	if got != (lockfile.Artifact{SHA256: want}) {
		t.Errorf("pkg-url resolve() = %+v, want only SHA256 %s", got, want)
	}
}

// A repo package records OS/Arch/Asset/SHA without a download (explicit
// asset + inline checksum resolve network-free).
func TestLockArtifactRepoRecordsCoordinates(t *testing.T) {
	info := platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64}
	pkg := manifest.Package{
		Name: "repo-tool", Backend: manifest.BackendAppArtifact,
		Repo: "owner/tool", Tag: "v1.2.3",
		Asset: "tool-{{.Arch}}.tar.gz", SHA256: "abc123", Binary: "tool",
	}
	got, err := newAppArtifactBackend(exec.NewFake()).resolve(pkg, info)
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	osTok, archTok := releaseTokens(info)
	want := lockfile.Artifact{OS: osTok, Arch: archTok, Asset: "tool-amd64.tar.gz", Binary: "tool", SHA256: "abc123"}
	if got != want {
		t.Errorf("resolve() = %+v, want %+v", got, want)
	}
}

// A locked checksum overrides the manifest on Install: a wrong locked hash
// fails the download before anything is staged.
func TestAppArtifactLockOverrideChecksumFailsInstall(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	elf := make([]byte, 20)
	copy(elf, "\x7fELF\x02\x01")
	elf[18] = 62
	fakeHTTPGet(t, map[string][]byte{"test.AppImage": elf})

	pkg := manifest.Package{
		Name: "locked-app", Backend: manifest.BackendAppArtifact,
		URL: "https://vendor.example/test.AppImage",
	}
	info := platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64, Desktop: true}
	wrong := hex.EncodeToString(bytes.Repeat([]byte{1}, 32))
	lock := &lockfile.Lock{Packages: []lockfile.LockedPackage{{
		Name:      "locked-app",
		Artifacts: []lockfile.Artifact{{SHA256: wrong}},
	}}}
	if err := newAppArtifactBackend(exec.NewFake()).withLock(lock).Install(pkg, info); err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("Install() error = %v; want SHA256 mismatch", err)
	}
	if ok, _ := newAppArtifactBackend(exec.NewFake()).Installed(pkg, info); ok {
		t.Error("Installed() = true after failed locked Install, want false")
	}
}
