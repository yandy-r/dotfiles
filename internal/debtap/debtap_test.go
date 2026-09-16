package debtap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
)

// debFixture returns the bytes of a minimal but structurally valid .deb: the ar
// global header followed by a first member named "debian-binary". Only the
// header is meaningful to validateDeb, which is the point — the file is a .deb
// because of its structure, not its name.
func debFixture() []byte {
	var b []byte
	b = append(b, arMagic...)
	b = append(b, []byte("debian-binary   ")...) // 16-byte name field, space padded
	b = append(b, []byte("1700000000  0     0     100644  4         `\n")...)
	b = append(b, []byte("2.0\n")...)
	return b
}

// writeFixture writes content into a fresh temp file named name and returns its
// path.
func writeFixture(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// serveBytes starts a TLS httptest server returning body, and points the
// package's httpGet seam at a client that trusts it.
//
// It must be a TLS server: Acquire refuses plaintext http outright, so a
// cleartext test server would exercise the refusal rather than the download.
func serveBytes(t *testing.T, body []byte) string {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	orig := httpGet
	httpGet = srv.Client().Get
	t.Cleanup(func() { httpGet = orig })
	return srv.URL + "/app.deb"
}

// TestAcquire_URL_DownloadsAndVerifiesDigest walks the whole acquisition stage
// over real HTTP: the body is streamed to disk, hashed, structurally validated,
// and compared against a matching --sha256.
func TestAcquire_URL_DownloadsAndVerifiesDigest(t *testing.T) {
	body := debFixture()
	sum := sha256.Sum256(body)
	want := hex.EncodeToString(sum[:])

	url := serveBytes(t, body)
	c := New(nil, Dirs{Cache: t.TempDir(), DB: t.TempDir(), Pkgfile: t.TempDir()})

	path, source, got, err := c.Acquire(Options{Input: url, SHA256: want})
	if err != nil {
		t.Fatalf("Acquire() error = %v, want nil", err)
	}
	if source != SourceURL {
		t.Errorf("source = %q, want %q", source, SourceURL)
	}
	if got != want {
		t.Errorf("sha256 = %q, want %q", got, want)
	}

	// The digest must describe what actually landed on disk, not what the
	// server claimed to send.
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(onDisk) != string(body) {
		t.Errorf("downloaded %d bytes, want %d", len(onDisk), len(body))
	}
}

// TestAcquire_URL_FailsClosedOnDigestMismatch proves --sha256 is a gate, not a
// report: a body that hashes to something else is rejected.
func TestAcquire_URL_FailsClosedOnDigestMismatch(t *testing.T) {
	url := serveBytes(t, debFixture())
	c := New(nil, Dirs{Cache: t.TempDir(), DB: t.TempDir(), Pkgfile: t.TempDir()})

	_, _, got, err := c.Acquire(Options{
		Input:  url,
		SHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err == nil {
		t.Fatal("Acquire() error = nil, want a checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("Acquire() error = %v, want it to mention the checksum", err)
	}
	// The computed digest is still reported, so a caller can see what it got.
	if got == "" {
		t.Error("sha256 = \"\", want the computed digest even on mismatch")
	}
}

// TestAcquire_RejectsPlaintextHTTP proves the http:// refusal has no escape
// hatch: the artifact is destined to be installed as root.
func TestAcquire_RejectsPlaintextHTTP(t *testing.T) {
	c := New(nil, Dirs{Cache: t.TempDir(), DB: t.TempDir(), Pkgfile: t.TempDir()})

	_, _, _, err := c.Acquire(Options{Input: "http://example.com/app.deb"})
	if !errors.Is(err, ErrInsecureURL) {
		t.Errorf("Acquire() error = %v, want ErrInsecureURL", err)
	}
}

// TestAcquire_RejectsNonDebByMagicBytes proves classification is by structure,
// not by extension: an HTML error page saved as "app.deb" is caught here rather
// than confusing debtap later.
func TestAcquire_RejectsNonDebByMagicBytes(t *testing.T) {
	path := writeFixture(t, "app.deb", []byte("<!doctype html><title>404</title>"))
	c := New(nil, Dirs{Cache: t.TempDir(), DB: t.TempDir(), Pkgfile: t.TempDir()})

	_, source, sum, err := c.Acquire(Options{Input: path})
	if !errors.Is(err, ErrNotDeb) {
		t.Fatalf("Acquire() error = %v, want ErrNotDeb", err)
	}
	if source != SourceFile {
		t.Errorf("source = %q, want %q", source, SourceFile)
	}
	if sum == "" {
		t.Error("sha256 = \"\", want the computed digest even for a rejected file")
	}
}

// TestParseControl_SkipsContinuationLines proves an indented "Package:" inside
// a long Description cannot be mistaken for the real field.
func TestParseControl_SkipsContinuationLines(t *testing.T) {
	ctl := parseControl(strings.Join([]string{
		"Package: slack-desktop",
		"Version: 4.36.140",
		"Description: a chat app",
		" Package: not-the-real-field",
		" continued prose",
		"Architecture: amd64",
	}, "\n"))

	if ctl.Package != "slack-desktop" {
		t.Errorf("Package = %q, want %q", ctl.Package, "slack-desktop")
	}
	if ctl.Version != "4.36.140" {
		t.Errorf("Version = %q, want %q", ctl.Version, "4.36.140")
	}
}

// TestFlagLikeMetadataNeverReachesPacmanArgv is an argument-injection
// regression test.
//
// A .deb's control data and the built package's .PKGINFO are attacker
// controlled, and pacman parses any operand beginning with "-" as a FLAG with
// no reliable "--" terminator. A crafted `depend = --dbpath=…` must therefore
// never reach an argv, or it would redirect the dependency check at a database
// the attacker supplied and make it report that everything is satisfied.
func TestFlagLikeMetadataNeverReachesPacmanArgv(t *testing.T) {
	const evil = "--dbpath=/tmp/evil"

	f := exec.NewFake()
	// Only the benign dependency may be queried.
	f.AddCommand("pacman", []string{"-T", "gtk3"}, nil, nil)

	c := New(f, Dirs{Cache: t.TempDir(), DB: t.TempDir(), Pkgfile: t.TempDir()})

	_, unresolvable, err := c.CheckDeps([]string{"gtk3", evil})
	if err != nil {
		t.Fatalf("CheckDeps() error = %v, want nil", err)
	}
	if !slices.Contains(unresolvable, evil) {
		t.Errorf("unresolvable = %v, want it to contain %q", unresolvable, evil)
	}

	// A flag-like name has no native counterpart and is never installed.
	got, perr := c.Precedence(evil)
	if perr != nil {
		t.Fatalf("Precedence() error = %v, want nil", perr)
	}
	if got != PrecedenceNone {
		t.Errorf("Precedence(%q) = %q, want %q", evil, got, PrecedenceNone)
	}
	if installed, _ := c.VerifyInstalled(evil); installed {
		t.Errorf("VerifyInstalled(%q) = true, want false", evil)
	}

	// The decisive assertion: the hostile string is in no argv, anywhere.
	for _, call := range f.CallsSnapshot() {
		for _, arg := range call.Args {
			if strings.Contains(arg, evil) {
				t.Fatalf("hostile value reached argv: %s %v", call.Name, call.Args)
			}
		}
	}
}

// TestParsePkgInfo_CollectsEveryDepend proves .PKGINFO's repeated depend keys
// accumulate rather than overwrite.
func TestParsePkgInfo_CollectsEveryDepend(t *testing.T) {
	pi := parsePkgInfo(strings.Join([]string{
		"# Generated by makepkg",
		"pkgname = slack-desktop",
		"pkgver = 4.36.140-1",
		"size = 112894976",
		"depend = gtk3",
		"depend = libxss",
		"unknown-future-key = ignored",
	}, "\n"))

	if pi.Name != "slack-desktop" || pi.Version != "4.36.140-1" || pi.Size != "112894976" {
		t.Errorf("PkgInfo = %+v, want name/version/size populated", pi)
	}
	if len(pi.Depends) != 2 || pi.Depends[0] != "gtk3" || pi.Depends[1] != "libxss" {
		t.Errorf("Depends = %v, want [gtk3 libxss]", pi.Depends)
	}
}
