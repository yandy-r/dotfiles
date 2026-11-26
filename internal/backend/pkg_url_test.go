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

var (
	debMagic = []byte("!<arch>\ndebian-binary")
	// debianBrew is a Debian-family host whose primary manager was detected as
	// Linuxbrew; native apt/dpkg still own .deb packages there.
	debianBrew = platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrHomebrew}
	debianAPT  = platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrAPT}
	fedoraBrew = platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrHomebrew}
)

func debPkg() manifest.Package {
	return manifest.Package{Name: "t3-code-client-debian", Backend: manifest.BackendPkgURL, PkgName: "t3code", Format: "deb", OS: "debian"}
}

func TestPkgURLEffectiveMgr(t *testing.T) {
	deb, rpm := debPkg(), manifest.Package{Name: "r", Backend: manifest.BackendPkgURL, PkgName: "r", Format: "rpm"}
	for _, tc := range []struct {
		name string
		pkg  manifest.Package
		info platform.Info
		want platform.PkgMgr
	}{
		{"deb on apt", deb, debianAPT, platform.PkgMgrAPT},
		{"deb on debian linuxbrew", deb, debianBrew, platform.PkgMgrAPT},
		{"deb on non-debian linuxbrew unchanged", deb, fedoraBrew, platform.PkgMgrHomebrew},
		{"deb on dnf unchanged", deb, rpmInfo, platform.PkgMgrDNF},
		{"rpm on debian linuxbrew unchanged", rpm, debianBrew, platform.PkgMgrHomebrew},
		{"rpm on dnf", rpm, rpmInfo, platform.PkgMgrDNF},
	} {
		if got := effectiveMgr(tc.pkg, tc.info); got != tc.want {
			t.Errorf("%s: effectiveMgr = %q, want %q", tc.name, got, tc.want)
		}
	}
	b := newPkgURLBackend(exec.NewFake())
	if !b.applicableFor(deb, debianBrew) || !b.applicableFor(deb, debianAPT) {
		t.Error("deb must be applicable on Debian with apt or Linuxbrew primary")
	}
	if b.applicableFor(deb, fedoraBrew) || b.applicableFor(deb, rpmInfo) {
		t.Error("deb must not be applicable on non-Debian hosts")
	}
	if b.applicableFor(rpm, debianBrew) {
		t.Error("rpm must stay inapplicable on a Debian Linuxbrew host")
	}
}

func TestPkgURLDebOnDebianLinuxbrewUsesNativeAPT(t *testing.T) {
	// Installed uses dpkg-query, not brew, and honours the dpkg status.
	f := exec.NewFake()
	f.AddCommand("dpkg-query", []string{"-W", "-f=${db:Status-Status}", "t3code"}, []byte("installed"), nil)
	if ok, err := newPkgURLBackend(f).Installed(debPkg(), debianBrew); err != nil || !ok {
		t.Fatalf("Installed(installed) = (%t, %v), want (true, nil)", ok, err)
	}
	f = exec.NewFake()
	f.AddCommand("dpkg-query", []string{"-W", "-f=${db:Status-Status}", "t3code"}, []byte("config-files"), nil)
	if ok, err := newPkgURLBackend(f).Installed(debPkg(), debianBrew); err != nil || ok {
		t.Fatalf("Installed(config-files) = (%t, %v), want (false, nil)", ok, err)
	}
	for _, c := range f.CallsSnapshot() {
		if c.Name == "brew" {
			t.Errorf("unexpected brew call: %v", c)
		}
	}

	// Remove runs apt-get remove through sudo env, never brew.
	f = exec.NewFake()
	if err := newPkgURLBackend(f).Remove(debPkg(), debianBrew); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	calls := f.CallsSnapshot()
	if len(calls) != 1 || calls[0].Name != "sudo" ||
		strings.Join(calls[0].Args, " ") != "env DEBIAN_FRONTEND=noninteractive apt-get remove -y t3code" {
		t.Errorf("Remove calls = %+v, want sudo env DEBIAN_FRONTEND=noninteractive apt-get remove -y t3code", calls)
	}

	// Install (and therefore Update) uses noninteractive apt-get install.
	sum := sha256.Sum256(debMagic)
	fakeHTTPGet(t, map[string][]byte{"t3code.deb": debMagic})
	r := &pkgRunner{Fake: exec.NewFake(), name: "t3code", arch: "amd64"}
	pkg := debPkg()
	pkg.URL, pkg.SHA256 = "https://v.example/t3code.deb", hex.EncodeToString(sum[:])
	for name, run := range map[string]func(manifest.Package, platform.Info) error{
		"Install": newPkgURLBackend(r).Install,
		"Update":  newPkgURLBackend(r).Update,
	} {
		if err := run(pkg, debianBrew); err != nil {
			t.Fatalf("%s() error = %v", name, err)
		}
	}
	var installs int
	for _, c := range r.CallsSnapshot() {
		if c.Name != "sudo" {
			continue
		}
		args := strings.Join(c.Args, " ")
		if !strings.HasPrefix(args, "env DEBIAN_FRONTEND=noninteractive apt-get install -y ") {
			t.Errorf("unexpected sudo call %q", args)
		}
		installs++
	}
	if installs != 2 {
		t.Errorf("sudo apt-get install ran %d times, want 2 (Install+Update)", installs)
	}

	// A non-Debian Linuxbrew host stays unsupported and runs nothing.
	f = exec.NewFake()
	if err := newPkgURLBackend(f).Remove(debPkg(), fedoraBrew); err == nil {
		t.Error("Remove(deb, fedora linuxbrew) = nil, want UnsupportedPlatformError")
	}
	if len(f.CallsSnapshot()) != 0 {
		t.Errorf("unsupported Remove ran commands: %v", f.CallsSnapshot())
	}
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

// TestPkgURLFormatInference checks the backend infers format exactly as the
// validator does (YAN-1415): a query string or uppercase extension on the url
// must still resolve rpm/deb, so a validated entry is not silently skipped.
// An explicit format always wins.
func TestPkgURLFormatInference(t *testing.T) {
	b := newPkgURLBackend(exec.NewFake())
	for _, tc := range []struct {
		name string
		pkg  manifest.Package
		info platform.Info
		want string
	}{
		{"rpm url with query", manifest.Package{URL: "https://example.com/foo.rpm?download=1"}, rpmInfo, "rpm"},
		{"uppercase RPM url", manifest.Package{URL: "https://example.com/Foo.RPM"}, rpmInfo, "rpm"},
		{"uppercase DEB url with query", manifest.Package{URL: "https://example.com/Foo.DEB?x=1"}, debianAPT, "deb"},
		{"uppercase asset", manifest.Package{Repo: "o/r", Asset: "foo.x86_64.RPM"}, rpmInfo, "rpm"},
		{"explicit format wins over url suffix", manifest.Package{URL: "https://example.com/foo.deb", Format: "rpm"}, rpmInfo, "rpm"},
		{"explicit format with suffixless url", manifest.Package{URL: "https://example.com/latest/stable", Format: "deb"}, debianAPT, "deb"},
	} {
		tc.pkg.Name, tc.pkg.Backend, tc.pkg.PkgName = "p", manifest.BackendPkgURL, "p"
		if got := pkgFormat(tc.pkg); got != tc.want {
			t.Errorf("%s: pkgFormat = %q, want %q", tc.name, got, tc.want)
		}
		if got := manifest.InferPkgFormat(tc.pkg.Asset, tc.pkg.URL); tc.pkg.Format == "" && got != tc.want {
			t.Errorf("%s: backend %q disagrees with manifest.InferPkgFormat %q", tc.name, tc.want, got)
		}
		if !b.applicableFor(tc.pkg, tc.info) {
			t.Errorf("%s: must be applicable on its %s host", tc.name, tc.want)
		}
	}
}
