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

// TestDesktopEntryManifestLauncherFields: manifest app metadata (exec_args,
// startup_wm_class, mime_type, categories) is emitted into the generated entry
// and overrides same-named vendor keys, while Exec and Icon stay dotf-owned.
func TestDesktopEntryManifestLauncherFields(t *testing.T) {
	pkg := manifest.Package{Name: "t3", App: &manifest.AppInstall{
		DesktopName:    "T3 Code",
		Categories:     "Development;",
		ExecArgs:       "%U",
		StartupWMClass: "t3code",
		MimeType:       "x-scheme-handler/t3code;x-scheme-handler/t3code-dev;",
	}}
	vendor := map[string]string{
		"StartupWMClass": "vendor-class",
		"MimeType":       "text/plain;",
		"Exec":           "/evil %f", // not in vendorKeys in practice, must never win
		"Comment":        "Vendor comment",
	}
	e, err := desktopEntry(pkg, "/opt/t3/T3.AppImage", "/opt/t3/icon.png", vendor)
	if err != nil {
		t.Fatalf("desktopEntry() error = %v", err)
	}
	for _, want := range []string{
		"Name=T3 Code\n",
		"Categories=Development;\n",
		"StartupWMClass=t3code\n",
		"MimeType=x-scheme-handler/t3code;x-scheme-handler/t3code-dev;\n",
		`Exec="/opt/t3/T3.AppImage" %U` + "\n",
		"Icon=/opt/t3/icon.png\n",
		"Comment=Vendor comment\n",
	} {
		if !bytes.Contains(e, []byte(want)) {
			t.Errorf("desktop entry missing %q:\n%s", want, e)
		}
	}
	for _, bad := range []string{"vendor-class", "text/plain", "/evil"} {
		if bytes.Contains(e, []byte(bad)) {
			t.Errorf("desktop entry leaked %q:\n%s", bad, e)
		}
	}

	// Without manifest fields, vendor StartupWMClass/MimeType are still copied.
	plain, err := desktopEntry(manifest.Package{Name: "x"}, "/run", "", map[string]string{"StartupWMClass": "v", "MimeType": "a/b;"})
	if err != nil {
		t.Fatalf("desktopEntry() error = %v", err)
	}
	if !bytes.Contains(plain, []byte("StartupWMClass=v\n")) || !bytes.Contains(plain, []byte("MimeType=a/b;\n")) {
		t.Errorf("vendor keys not preserved without manifest override:\n%s", plain)
	}

	// A newline in a manifest-supplied value is rejected like any other key.
	bad := manifest.Package{Name: "x", App: &manifest.AppInstall{MimeType: "a/b;\nExec=evil"}}
	if _, err := desktopEntry(bad, "/run", "", nil); err == nil {
		t.Error("newline in manifest mime_type: want error")
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
