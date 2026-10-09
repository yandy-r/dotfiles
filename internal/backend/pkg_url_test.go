package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// pkgRunner embeds exec.Fake (Run/LookPath/call recording) but answers Output
// dynamically, because the package/key paths are random temp files.
type pkgRunner struct {
	*exec.Fake
	name, arch, fpr, sig string
}

func (r *pkgRunner) Output(name string, args ...string) ([]byte, error) {
	j := name + " " + strings.Join(args, " ")
	switch {
	case strings.Contains(j, "%{NAME}"), strings.Contains(j, "Package"):
		return []byte(r.name + "\n"), nil
	case strings.Contains(j, "%{ARCH}"), strings.Contains(j, "Architecture"):
		return []byte(r.arch + "\n"), nil
	case name == "gpg" && strings.Contains(j, "--export"):
		if !strings.Contains(j, "--armor --export") {
			return nil, errors.New("RPM key export must be armored")
		}
		return []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\nTEST\n-----END PGP PUBLIC KEY BLOCK-----\n"), nil
	case name == "gpg":
		return []byte("pub:u:::::::::\nfpr:::::::::" + r.fpr + ":\n"), nil
	case name == "rpmkeys":
		return []byte(r.sig), nil
	}
	return nil, errors.New("unexpected command: " + j)
}

func (r *pkgRunner) sudoCalls() (n int) {
	for _, c := range r.CallsSnapshot() {
		if c.Name == "sudo" {
			n++
		}
	}
	return n
}

var (
	rpmMagic = []byte{0xed, 0xab, 0xee, 0xdb, 0, 0, 0, 0}
	rpmInfo  = platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrDNF}
	fprWant  = strings.Repeat("A", 40)
	fprOther = strings.Repeat("B", 40)
)

func tmpPkg(t *testing.T, body []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.pkg")
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheckPkgMagic(t *testing.T) {
	for _, tc := range []struct {
		format string
		body   []byte
		ok     bool
	}{
		{"rpm", rpmMagic, true},
		{"rpm", []byte("!<arch>\n"), false},
		{"deb", []byte("!<arch>\ndebian"), true},
		{"deb", rpmMagic, false},
		{"deb", nil, false},
	} {
		err := checkPkgMagic(tmpPkg(t, tc.body), tc.format)
		if (err == nil) != tc.ok {
			t.Errorf("checkPkgMagic(%s, %q) err = %v, want ok=%v", tc.format, tc.body, err, tc.ok)
		}
	}
}

func TestCheckPkgNameAndArch(t *testing.T) {
	r := &pkgRunner{Fake: exec.NewFake(), name: "other", arch: "aarch64"}
	b := newPkgURLBackend(r)
	p := tmpPkg(t, rpmMagic)
	if err := b.checkPkgName(p, "rpm", "foo"); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Errorf("checkPkgName mismatch err = %v", err)
	}
	if err := b.checkPkgName(p, "deb", "other"); err != nil {
		t.Errorf("checkPkgName match err = %v", err)
	}
	if err := b.checkPkgArch(p, "rpm", rpmInfo); err == nil || !strings.Contains(err.Error(), "architecture") {
		t.Errorf("checkPkgArch mismatch err = %v", err)
	}
	r.arch = "noarch"
	if err := b.checkPkgArch(p, "rpm", rpmInfo); err != nil {
		t.Errorf("checkPkgArch noarch err = %v", err)
	}
}

func TestInstallSignedRejectsBadFingerprintBeforeSudo(t *testing.T) {
	sum := sha256.Sum256(rpmMagic)
	fakeHTTPGet(t, map[string][]byte{"foo.rpm": rpmMagic, "key.asc": []byte("KEY")})
	r := &pkgRunner{Fake: exec.NewFake(), name: "foo", arch: "x86_64", fpr: fprOther}
	pkg := manifest.Package{
		Name: "foo", Backend: manifest.BackendPkgURL, PkgName: "foo",
		URL: "https://v.example/foo.rpm", SHA256: hex.EncodeToString(sum[:]),
		GPGKeyURL: "https://v.example/key.asc", GPGKeyFingerprint: fprWant,
	}
	err := newPkgURLBackend(r).Install(pkg, rpmInfo)
	if err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("Install err = %v; want fingerprint mismatch", err)
	}
	if n := r.sudoCalls(); n != 0 {
		t.Errorf("sudo ran %d times before fingerprint check passed", n)
	}
}

func TestVerifyRPMSignatureImportsArmoredPinnedKey(t *testing.T) {
	fakeHTTPGet(t, map[string][]byte{"key.asc": []byte("KEY")})
	r := &pkgRunner{Fake: exec.NewFake(), fpr: fprWant, sig: "x.rpm: digests signatures OK\n"}
	pkg := manifest.Package{Name: "x", GPGKeyURL: "https://v.example/key.asc", GPGKeyFingerprint: fprWant}
	if err := newPkgURLBackend(r).verifyRPMSignature(pkg, "x.rpm"); err != nil {
		t.Fatalf("verifyRPMSignature err = %v", err)
	}
	imported := false
	for _, c := range r.CallsSnapshot() {
		if c.Name == "sudo" && len(c.Args) > 1 && c.Args[0] == "rpmkeys" && c.Args[1] == "--import" {
			imported = true
		}
	}
	if !imported {
		t.Error("pinned key was not imported with rpmkeys")
	}
}

func TestInstallUnsignedRequiresChecksum(t *testing.T) {
	fakeHTTPGet(t, map[string][]byte{"foo.rpm": rpmMagic})
	r := &pkgRunner{Fake: exec.NewFake(), name: "foo", arch: "x86_64"}
	pkg := manifest.Package{
		Name: "foo", Backend: manifest.BackendPkgURL, PkgName: "foo",
		URL: "https://v.example/foo.rpm", AllowUnsigned: true, Justification: "vendor ships no sig",
	}
	err := newPkgURLBackend(r).Install(pkg, rpmInfo)
	if err == nil || !strings.Contains(err.Error(), "no locked checksum") {
		t.Fatalf("Install err = %v; want no locked checksum", err)
	}
	if len(r.CallsSnapshot()) != 0 {
		t.Errorf("commands ran before checksum gate: %v", r.CallsSnapshot())
	}
}

func TestVerifyRPMSignatureParsing(t *testing.T) {
	for _, tc := range []struct {
		sig string
		ok  bool
	}{
		{"x.rpm: digests signatures OK\n", true},
		{"x.rpm: DIGESTS SIGNATURES OK\n", true},
		{"x.rpm: digests SIGNATURES NOT OK\n", false},
		{"x.rpm: digests signatures OK (NOKEY)\n", false},
		{"x.rpm: digests OK\n", false},
	} {
		r := &pkgRunner{Fake: exec.NewFake(), sig: tc.sig}
		err := newPkgURLBackend(r).verifyRPMSignature(manifest.Package{Name: "x"}, "x.rpm")
		if (err == nil) != tc.ok {
			t.Errorf("verifyRPMSignature(%q) err = %v, want ok=%v", tc.sig, err, tc.ok)
		}
	}
}
