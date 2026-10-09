package backend

import (
	"bytes"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// fakeHTTPGet stubs httpGet, serving body by URL path suffix.
func fakeHTTPGet(t *testing.T, files map[string][]byte) {
	t.Helper()
	orig := httpGet
	httpGet = func(url string) (*http.Response, error) {
		for suffix, body := range files {
			if strings.HasSuffix(url, suffix) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
			}
		}
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { httpGet = orig })
}

// TestAppArtifactLifecycle covers Install → Installed → Update(checksum
// mismatch leaves old file + desktop untouched) → Remove.
func TestAppArtifactLifecycle(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	elf := make([]byte, 20)
	copy(elf, "\x7fELF\x02\x01")
	elf[18] = 62
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("i", 8))
	fakeHTTPGet(t, map[string][]byte{"test.AppImage": elf, "icon.png": png})

	pkg := manifest.Package{
		Name:    "test-app",
		Backend: manifest.BackendAppArtifact,
		URL:     "https://vendor.example/test.AppImage",
		App:     &manifest.AppInstall{Icon: "https://vendor.example/icon.png"},
	}
	info := platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64, Desktop: true}
	b := newAppArtifactBackend(exec.NewFake())
	if err := b.Install(pkg, platform.Info{OS: platform.OSRedHat, Arch: platform.ArchARM64, Desktop: true}); err == nil {
		t.Fatal("Install accepted x86_64 ELF on arm64")
	}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	ok, err := b.Installed(pkg, info)
	if err != nil || !ok {
		t.Fatalf("Installed() = %v, %v; want true, nil", ok, err)
	}
	appDir := filepath.Join(dataHome, "dotf", "apps", "test-app")
	got, _ := os.ReadFile(filepath.Join(appDir, "test.AppImage"))
	desktop := filepath.Join(dataHome, "applications", "dotf-test-app.desktop")
	entry, _ := os.ReadFile(desktop)
	if !bytes.Equal(got, elf) {
		t.Errorf("installed binary = %q, want ELF payload", got)
	}
	if !bytes.Contains(entry, []byte(`Exec="`+filepath.Join(appDir, "test.AppImage")+`"`)) {
		t.Errorf("desktop entry missing quoted Exec path:\n%s", entry)
	}

	// Update with a wrong checksum must fail and change nothing.
	pkg.SHA256 = hex.EncodeToString(bytes.Repeat([]byte{0}, 32))
	if err := b.Update(pkg, info); err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("Update() error = %v; want SHA256 mismatch", err)
	}
	got2, _ := os.ReadFile(filepath.Join(appDir, "test.AppImage"))
	entry2, _ := os.ReadFile(desktop)
	if !bytes.Equal(got2, elf) || !bytes.Equal(entry2, entry) {
		t.Error("failed Update modified the installed binary or desktop entry")
	}
	if ok, _ := b.Installed(pkg, info); !ok {
		t.Error("Installed() = false after failed Update, want true")
	}

	if err := b.Remove(pkg, info); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if ok, err := b.Installed(pkg, info); err != nil || ok {
		t.Fatalf("Installed() after Remove = %v, %v; want false, nil", ok, err)
	}
}

func TestDesktopEntryQuotesAndRejectsNewline(t *testing.T) {
	e, err := desktopEntry(manifest.Package{Name: "x"}, "/opt/my app/run", "", nil)
	if err != nil {
		t.Fatalf("desktopEntry() error = %v", err)
	}
	if !bytes.Contains(e, []byte(`Exec="/opt/my app/run"`)) {
		t.Errorf("Exec not space-quoted: %s", e)
	}
	if _, err := desktopEntry(manifest.Package{Name: "x"}, "/a\nc", "", nil); err == nil {
		t.Error("newline in exec path: want error")
	}
	if _, err := desktopEntry(manifest.Package{Name: "x"}, "/run", "", map[string]string{"Comment": "a\nb"}); err == nil {
		t.Error("newline in vendor value: want error")
	}
}
