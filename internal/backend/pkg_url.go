package backend

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// pkgURLBackend installs a vendor .rpm/.deb downloaded over HTTPS (repo
// release asset or verbatim URL) through dnf/apt. Integrity gates run before
// any privileged step: optional sha256, magic bytes, package-name match and,
// for signed RPMs, a fingerprint-verified vendor key plus rpmkeys --checksig.
type pkgURLBackend struct {
	r    exec.Runner
	lock *lockfile.Lock
}

func newPkgURLBackend(r exec.Runner) *pkgURLBackend { return &pkgURLBackend{r: r} }

// withLock sets the (nil-tolerant) lock that overrides URL/checksum on Install.
func (b *pkgURLBackend) withLock(l *lockfile.Lock) *pkgURLBackend {
	b.lock = l
	return b
}

// resolve implements lockable.
func (b *pkgURLBackend) resolve(pkg manifest.Package, target platform.Info) (lockfile.Artifact, error) {
	return lockArtifact(pkg, target, []string{"." + pkgFormat(pkg)})
}

// pkgFormat is the declared format, else inferred from the url/asset suffix.
func pkgFormat(pkg manifest.Package) string {
	if pkg.Format != "" {
		return pkg.Format
	}
	for _, s := range []string{pkg.URL, pkg.Asset} {
		switch {
		case strings.HasSuffix(s, ".rpm"):
			return "rpm"
		case strings.HasSuffix(s, ".deb"):
			return "deb"
		}
	}
	return ""
}

// effectiveMgr is the package manager that owns a pkg-url artifact on this
// host. A .deb is always an apt concern: on a Debian-family host where
// Linuxbrew was detected as the primary manager (info.PkgMgr == homebrew) the
// native apt/dpkg tools still apply, so a deb resolves to apt there. Every
// other combination (including all RPM routes) uses the detected manager.
func effectiveMgr(pkg manifest.Package, info platform.Info) platform.PkgMgr {
	if pkgFormat(pkg) == "deb" && info.OS == platform.OSDebian && info.PkgMgr == platform.PkgMgrHomebrew {
		return platform.PkgMgrAPT
	}
	return info.PkgMgr
}

func (b *pkgURLBackend) applicableFor(pkg manifest.Package, info platform.Info) bool {
	switch pkgFormat(pkg) {
	case "rpm":
		return effectiveMgr(pkg, info) == platform.PkgMgrDNF
	case "deb":
		return effectiveMgr(pkg, info) == platform.PkgMgrAPT
	}
	return false
}

func (b *pkgURLBackend) Installed(pkg manifest.Package, info platform.Info) (bool, error) {
	mgr := effectiveMgr(pkg, info)
	name, args, err := installedQuery(mgr, pkg.PkgName)
	if err != nil {
		return false, err
	}
	out, err := b.r.Output(name, args...)
	if err != nil {
		return false, nil
	}
	if mgr == platform.PkgMgrAPT {
		return dpkgStatusInstalled(out), nil
	}
	if pkg.RPMRepackStripBuildID {
		out, err := b.r.Output("rpm", "-q", "--qf", "%{RELEASE}", pkg.PkgName)
		return err == nil && strings.Contains(string(out), ".dotf1"), nil
	}
	return true, nil
}

// Install downloads, verifies and installs the package file.
func (b *pkgURLBackend) Install(pkg manifest.Package, info platform.Info) error {
	if !b.applicableFor(pkg, info) {
		return &UnsupportedPlatformError{Backend: manifest.BackendPkgURL, OS: info.OS, PkgMgr: info.PkgMgr}
	}
	format := pkgFormat(pkg)
	allow := []string{"." + format}
	a, err := resolveArtifactLocked(pkg, info, allow, b.lock)
	if err != nil {
		return err
	}
	if a.SHA256 == "" && (pkg.EffectiveClass() == manifest.ClassPinned || format == "deb" || pkg.AllowUnsigned) {
		osTok, archTok := releaseTokens(info)
		return pinnedNoChecksumErr(pkg.Name, osTok, archTok)
	}
	root, err := os.MkdirTemp("", "dotf-pkg-work-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(root) }()
	path, err := fetchVerifyFile(a.URL, a.SHA256, root, "dotf-pkg-*."+format)
	if err != nil {
		return fmt.Errorf("pkg-url %q: %w", pkg.Name, err)
	}
	defer func() { _ = os.Remove(path) }()
	if err := os.Chmod(path, 0o644); err != nil { // dnf/apt read it as root or _apt
		return err
	}
	if err := checkPkgMagic(path, format); err != nil {
		return fmt.Errorf("pkg-url %q: %w", pkg.Name, err)
	}
	if err := b.checkPkgName(path, format, pkg.PkgName); err != nil {
		return fmt.Errorf("pkg-url %q: %w", pkg.Name, err)
	}
	if err := b.checkPkgArch(path, format, info); err != nil {
		return fmt.Errorf("pkg-url %q: %w", pkg.Name, err)
	}
	if pkg.RPMRepackStripBuildID {
		if !pkg.AllowUnsigned || a.SHA256 == "" {
			return fmt.Errorf("RPM repack requires verified checksum and explicit unsigned consent")
		}
		path, err = repackStripBuildID(b.r, pkg, path, root)
		if err != nil {
			return err
		}
	}
	if format == "deb" {
		return b.r.Run("sudo", "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", path)
	}
	gpgcheck := "localpkg_gpgcheck=1"
	if pkg.AllowUnsigned {
		gpgcheck = "localpkg_gpgcheck=0" // explicit, justified manifest consent only
	} else if err := b.verifyRPMSignature(pkg, path); err != nil {
		return fmt.Errorf("pkg-url %q: %w", pkg.Name, err)
	}
	return b.r.Run("sudo", "dnf", "install", "-y", "--setopt="+gpgcheck, path)
}

// Update re-downloads the current artifact; dnf/apt no-op on an equal version.
func (b *pkgURLBackend) Update(pkg manifest.Package, info platform.Info) error {
	return b.Install(pkg, info)
}

// Remove uninstalls by package name; imported vendor keys are left in place.
// A direct call on the wrong manager is refused before any command runs.
func (b *pkgURLBackend) Remove(pkg manifest.Package, info platform.Info) error {
	if !b.applicableFor(pkg, info) {
		return &UnsupportedPlatformError{Backend: manifest.BackendPkgURL, OS: info.OS, PkgMgr: info.PkgMgr}
	}
	if strings.TrimSpace(pkg.PkgName) == "" {
		return fmt.Errorf("pkg-url %q: pkg_name is required for remove", pkg.Name)
	}
	argv, err := removeArgv(effectiveMgr(pkg, info), pkg.PkgName)
	if err != nil {
		return err
	}
	return b.r.Run(argv[0], argv[1:]...)
}

func checkPkgMagic(path, format string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 8)
	n, _ := f.Read(head)
	head = head[:n]
	want := map[string][]byte{"rpm": {0xed, 0xab, 0xee, 0xdb}, "deb": []byte("!<arch>\n")}[format]
	if !bytes.HasPrefix(head, want) {
		return fmt.Errorf("download is not a %s package", format)
	}
	return nil
}

func (b *pkgURLBackend) checkPkgName(path, format, want string) error {
	var out []byte
	var err error
	if format == "rpm" {
		out, err = b.r.Output("rpm", "-qp", "--nosignature", "--qf", "%{NAME}", path)
	} else {
		out, err = b.r.Output("dpkg-deb", "-f", path, "Package")
	}
	if err != nil {
		return fmt.Errorf("read package name: %w", err)
	}
	if got := strings.TrimSpace(string(out)); got != want {
		return fmt.Errorf("package name %q does not match pkg_name %q", got, want)
	}
	return nil
}

// verifyRPMSignature imports the fingerprint-checked vendor key (when declared)
// and requires rpmkeys --checksig to report a valid, known signature.
func (b *pkgURLBackend) verifyRPMSignature(pkg manifest.Package, path string) error {
	if pkg.GPGKeyURL != "" {
		key, err := fetchVerifyFile(pkg.GPGKeyURL, "", "", "dotf-key-*")
		if err != nil {
			return err
		}
		defer func() { _ = os.Remove(key) }()
		keyHome, err := os.MkdirTemp("", "dotf-keyring-*")
		if err != nil {
			return err
		}
		defer func() {
			_ = b.r.Run("gpgconf", "--homedir", keyHome, "--kill", "all")
			_ = os.RemoveAll(keyHome)
		}()
		out, err := b.r.Output("gpg", "--homedir", keyHome, "--batch", "--no-autostart", "--show-keys", "--with-colons", key)
		if err != nil {
			return fmt.Errorf("read key fingerprint: %w", err)
		}
		if !hasFingerprint(out, pkg.GPGKeyFingerprint) {
			return fmt.Errorf("signing key fingerprint mismatch: want %s", pkg.GPGKeyFingerprint)
		}
		if err := b.r.Run("gpg", "--homedir", keyHome, "--batch", "--import", key); err != nil {
			return err
		}
		selected, err := b.r.Output("gpg", "--homedir", keyHome, "--batch", "--armor", "--export", pkg.GPGKeyFingerprint)
		if err != nil || len(selected) == 0 {
			return fmt.Errorf("export pinned signing key: %v", err)
		}
		if err := os.WriteFile(key, selected, 0o644); err != nil {
			return err
		}
		if err := os.Chmod(key, 0o644); err != nil {
			return err
		}
		if err := b.r.Run("sudo", "rpmkeys", "--import", key); err != nil {
			return fmt.Errorf("import signing key: %w", err)
		}
	}
	out, err := b.r.Output("rpmkeys", "--checksig", path)
	s := strings.ToLower(string(out))
	if err != nil || !strings.Contains(s, "digests signatures ok") || strings.Contains(s, "nokey") || strings.Contains(s, "not ok") || strings.Contains(s, "bad") || strings.Contains(s, "missing") {
		return fmt.Errorf("rpm signature not verified (%s); declare gpg_key_url/gpg_fingerprint or allow_unsigned", strings.TrimSpace(s))
	}
	return nil
}

// checkPkgArch verifies the artifact's declared architecture matches the host.
// Foreign-arch packages are rejected before any elevated install.
func (b *pkgURLBackend) checkPkgArch(path, format string, info platform.Info) error {
	var out []byte
	var err error
	if format == "rpm" {
		out, err = b.r.Output("rpm", "-qp", "--nosignature", "--qf", "%{ARCH}", path)
	} else {
		out, err = b.r.Output("dpkg-deb", "-f", path, "Architecture")
	}
	if err != nil {
		return fmt.Errorf("read package architecture: %w", err)
	}
	arch := strings.TrimSpace(string(out))
	// arch-independent packages install on any host.
	if arch == "noarch" || arch == "all" {
		return nil
	}
	if want := hostPkgArch(format, info.Arch); arch != want {
		return fmt.Errorf("package architecture %q does not match host %q", arch, want)
	}
	return nil
}

// hostPkgArch maps the canonical host arch to the rpm/deb package arch token.
func hostPkgArch(format string, arch platform.Arch) string {
	switch arch {
	case platform.ArchX86_64:
		if format == "deb" {
			return "amd64"
		}
		return "x86_64"
	case platform.ArchARM64:
		if format == "deb" {
			return "arm64"
		}
		return "aarch64"
	default:
		return string(arch)
	}
}

// hasFingerprint reports whether gpg --with-colons output lists want as a
// primary-key fingerprint (the first fpr record after a pub record).
func hasFingerprint(out []byte, want string) bool {
	afterPub := false
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, ":")
		switch {
		case f[0] == "pub":
			afterPub = true
		case f[0] == "fpr" && afterPub && len(f) > 9:
			if strings.EqualFold(f[9], want) {
				return true
			}
			afterPub = false
		}
	}
	return false
}

var (
	_ Updater      = (*pkgURLBackend)(nil)
	_ Remover      = (*pkgURLBackend)(nil)
	_ packageGated = (*pkgURLBackend)(nil)
	_ lockable     = (*pkgURLBackend)(nil)
)
