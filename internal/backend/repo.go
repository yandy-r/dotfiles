package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// UnsupportedPlatformError is returned when the repo backend cannot run on the
// detected host: no sub-table matches the host's package manager.
type UnsupportedPlatformError struct {
	Backend manifest.Backend
	OS      platform.OS
	PkgMgr  platform.PkgMgr
}

// Error implements the error interface.
func (e *UnsupportedPlatformError) Error() string {
	return fmt.Sprintf("backend: %q unsupported on os=%q pkg_mgr=%q", e.Backend, e.OS, e.PkgMgr)
}

// MissingPackageNameError is returned when Names lacks an entry for apt.
type MissingPackageNameError struct {
	Package string
	PkgMgr  platform.PkgMgr
}

// Error implements the error interface.
func (e *MissingPackageNameError) Error() string {
	return fmt.Sprintf("backend: package %q missing names[%q]", e.Package, e.PkgMgr)
}

// repoBackend adds a third-party OS repository before installing, dispatching on
// the host's package manager: apt_repo drives Debian/Ubuntu, dnf_repo drives
// Fedora/RHEL. One logical tool is one manifest entry (see ADR-5); a host with
// no matching sub-table reports not-applicable rather than failing.
type repoBackend struct {
	r exec.Runner

	// readFile reads an existing /etc/yum.repos.d/<id>.repo before the dnf file
	// modes write it. It is os.ReadFile in production; tests inject a fake so
	// they never read the real /etc.
	readFile func(string) ([]byte, error)
}

func newRepoBackend(r exec.Runner) *repoBackend { return &repoBackend{r: r, readFile: os.ReadFile} }

// newAptRepoBackend constructs the repo backend under its deprecated apt-repo
// alias, retained one release so existing manifests keep working.
//
// Deprecated: use newRepoBackend.
func newAptRepoBackend(r exec.Runner) *repoBackend { return newRepoBackend(r) }

// applicableFor reports whether this package can be installed on this host: the
// host's package manager must have a matching sub-table on the entry. It is the
// packageGated gate, not platformGated — a repo entry that carries only
// apt_repo is not applicable on a dnf host even though the backend itself is.
func (b *repoBackend) applicableFor(pkg manifest.Package, info platform.Info) bool {
	switch info.PkgMgr {
	case platform.PkgMgrAPT:
		return pkg.AptRepo != nil
	case platform.PkgMgrDNF:
		return pkg.DNFRepo != nil
	default:
		return false
	}
}

// backendLabel names the backend an error is reported against: the manifest's
// own spelling when set, so an entry written under the deprecated apt-repo
// alias reports apt-repo, and the canonical "repo" otherwise.
func backendLabel(pkg manifest.Package) manifest.Backend {
	if pkg.Backend != "" {
		return pkg.Backend
	}
	return manifest.BackendRepo
}

func (b *repoBackend) Install(pkg manifest.Package, info platform.Info) error {
	if !b.applicableFor(pkg, info) {
		return &UnsupportedPlatformError{
			Backend: backendLabel(pkg),
			OS:      info.OS,
			PkgMgr:  info.PkgMgr,
		}
	}

	switch info.PkgMgr {
	case platform.PkgMgrAPT:
		return b.installAPT(pkg, info)
	case platform.PkgMgrDNF:
		return b.installDNF(pkg, info)
	default:
		// Unreachable past applicableFor; defense-in-depth for a direct caller.
		return &UnsupportedPlatformError{
			Backend: backendLabel(pkg),
			OS:      info.OS,
			PkgMgr:  info.PkgMgr,
		}
	}
}

// Update upgrades an already-configured repo package via the host package
// manager. The repo itself is not (re)configured, but on dnf a kept
// non-dotf repo file's [<id>] section is enabled for the upgrade, as on
// install (see dnfEnableRepoArgs).
func (b *repoBackend) Update(pkg manifest.Package, info platform.Info) error {
	if info.PkgMgr != platform.PkgMgrDNF || pkg.DNFRepo == nil {
		return b.lifecycle(pkg, info, upgradeArgv)
	}
	enable, err := b.dnfEnableRepoArgs(pkg.DNFRepo)
	if err != nil {
		return err
	}
	return b.lifecycle(pkg, info, func(mgr platform.PkgMgr, name string) ([]string, error) {
		argv, err := upgradeArgv(mgr, name)
		if err != nil || len(enable) == 0 {
			return argv, err
		}
		// argv is sudo dnf upgrade …; the flag goes right after the verb.
		return append(append(append([]string{}, argv[:3]...), enable...), argv[3:]...), nil
	})
}

// Remove uninstalls the package via the host package manager; the configured
// third-party repository is left in place.
func (b *repoBackend) Remove(pkg manifest.Package, info platform.Info) error {
	return b.lifecycle(pkg, info, removeArgv)
}

func (b *repoBackend) lifecycle(pkg manifest.Package, info platform.Info, argvFn func(platform.PkgMgr, string) ([]string, error)) error {
	if !b.applicableFor(pkg, info) {
		return &UnsupportedPlatformError{Backend: backendLabel(pkg), OS: info.OS, PkgMgr: info.PkgMgr}
	}
	name := pkg.Names[string(info.PkgMgr)]
	if name == "" {
		return &MissingPackageNameError{Package: pkg.Name, PkgMgr: info.PkgMgr}
	}
	argv, err := argvFn(info.PkgMgr, name)
	if err != nil {
		return err
	}
	return b.r.Run(argv[0], argv[1:]...)
}

func (b *repoBackend) installAPT(pkg manifest.Package, info platform.Info) error {
	aptName, ok := pkg.Names[string(platform.PkgMgrAPT)]
	if !ok || aptName == "" {
		return &MissingPackageNameError{Package: pkg.Name, PkgMgr: platform.PkgMgrAPT}
	}

	repo := pkg.AptRepo
	if repo == nil {
		return fmt.Errorf("backend: package %q missing apt_repo configuration", pkg.Name)
	}
	if repo.KeyFingerprint == "" {
		return fmt.Errorf("backend: package %q apt_repo.key_fingerprint is required", pkg.Name)
	}

	keyData, err := b.r.Output("curl", "-fsSL", repo.KeyURL)
	if err != nil {
		return fmt.Errorf("fetch apt repo key %q: %w", repo.KeyURL, err)
	}

	keyFile, err := writeTempFile("dotf-apt-key-*", keyData)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(keyFile) }()

	pinned, err := b.exportPinnedAptKey(keyFile, repo.KeyFingerprint)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(pinned) }()

	keyringDir := filepath.Dir(repo.Keyring)
	if err := b.r.Run("sudo", "install", "-d", "-m", "0755", keyringDir); err != nil {
		return fmt.Errorf("create apt keyring directory %q: %w", keyringDir, err)
	}

	if err := b.r.Run("sudo", "install", "-m", "0644", pinned, repo.Keyring); err != nil {
		return fmt.Errorf("install apt repo keyring %q: %w", repo.Keyring, err)
	}

	repoLine, err := renderRepoLine(repo.RepoLine, info)
	if err != nil {
		return err
	}

	sourcesFile, err := writeTempFile("dotf-apt-sources-*", []byte(repoLine+"\n"))
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(sourcesFile) }()

	if err := b.r.Run("sudo", "install", "-m", "644", sourcesFile, repo.SourcesPath); err != nil {
		return fmt.Errorf("write apt sources %q: %w", repo.SourcesPath, err)
	}

	if err := b.r.Run("sudo", "apt-get", "update"); err != nil {
		return fmt.Errorf("apt-get update: %w", err)
	}

	if err := b.r.Run("sudo", "apt-get", "install", "-y", aptName); err != nil {
		return fmt.Errorf("apt-get install %q: %w", aptName, err)
	}

	return nil
}

// exportPinnedAptKey checks that keyFile carries fingerprint as a primary key
// and returns a temp file holding only that key (with its subkeys), exported
// in binary form from a throwaway gpg homedir, so the signed-by keyring trusts
// exactly the pinned key. The caller removes the returned file.
func (b *repoBackend) exportPinnedAptKey(keyFile, fingerprint string) (string, error) {
	keyHome, err := os.MkdirTemp("", "dotf-apt-keyring-*")
	if err != nil {
		return "", err
	}
	defer func() {
		_ = b.r.Run("gpgconf", "--homedir", keyHome, "--kill", "all")
		_ = os.RemoveAll(keyHome)
	}()

	out, err := b.r.Output("gpg", "--homedir", keyHome, "--batch", "--no-autostart", "--show-keys", "--with-colons", keyFile)
	if err != nil {
		return "", fmt.Errorf("read apt repo key fingerprint: %w", err)
	}
	if !hasFingerprint(out, fingerprint) {
		return "", fmt.Errorf("apt repo key fingerprint mismatch: expected %s", fingerprint)
	}
	if err := b.r.Run("gpg", "--homedir", keyHome, "--batch", "--import", keyFile); err != nil {
		return "", fmt.Errorf("import apt repo key: %w", err)
	}
	selected, err := b.r.Output("gpg", "--homedir", keyHome, "--batch", "--export", fingerprint)
	if err != nil {
		return "", fmt.Errorf("export pinned apt repo key: %w", err)
	}
	if len(selected) == 0 {
		return "", fmt.Errorf("export pinned apt repo key %s: empty output", fingerprint)
	}

	pinned, err := writeTempFile("dotf-apt-keyring-*", selected)
	if err != nil {
		return "", err
	}
	if err := os.Chmod(pinned, 0o644); err != nil {
		_ = os.Remove(pinned)
		return "", fmt.Errorf("chmod pinned apt repo key: %w", err)
	}
	return pinned, nil
}

// Installed probes installed state through the host's package manager only. It
// deliberately does NOT require the repo sub-table: whether a package is
// present is a question about the package database, not about how the
// repository was configured.
func (b *repoBackend) Installed(pkg manifest.Package, info platform.Info) (bool, error) {
	switch info.PkgMgr {
	case platform.PkgMgrAPT:
		aptName, ok := pkg.Names[string(platform.PkgMgrAPT)]
		if !ok || aptName == "" {
			return false, &MissingPackageNameError{Package: pkg.Name, PkgMgr: platform.PkgMgrAPT}
		}

		cmd, args := dpkgInstalledQuery(aptName)
		out, err := b.r.Output(cmd, args...)
		if err != nil {
			return false, nil
		}
		return dpkgStatusInstalled(out), nil
	case platform.PkgMgrDNF:
		dnfName, ok := pkg.Names[string(platform.PkgMgrDNF)]
		if !ok || dnfName == "" {
			return false, &MissingPackageNameError{Package: pkg.Name, PkgMgr: platform.PkgMgrDNF}
		}

		if _, err := b.r.Output("rpm", "-q", dnfName); err != nil {
			return false, nil
		}
		return true, nil
	default:
		return false, &UnsupportedPlatformError{
			Backend: backendLabel(pkg),
			OS:      info.OS,
			PkgMgr:  info.PkgMgr,
		}
	}
}

// renderRepoLine substitutes the {{.Codename}} and {{.Arch}} placeholders in a
// repo_line with the host's release codename and dpkg release arch, so the
// suite and arch track the target machine instead of being hardcoded. It
// errors when the line needs a codename the host does not report (os-release
// VERSION_CODENAME/UBUNTU_CODENAME absent), since a bare/empty suite would 404
// on apt-get update.
func renderRepoLine(line string, info platform.Info) (string, error) {
	if strings.Contains(line, "{{.Codename}}") && info.Codename == "" {
		return "", fmt.Errorf("apt-repo: host codename unknown (os-release VERSION_CODENAME/UBUNTU_CODENAME absent); cannot render repo_line %q", line)
	}
	return strings.NewReplacer("{{.Codename}}", info.Codename, "{{.Arch}}", info.Arch.ReleaseArch()).Replace(line), nil
}

func writeTempFile(pattern string, content []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	path := f.Name()

	if err := os.Chmod(path, 0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("chmod temp file: %w", err)
	}

	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close temp file: %w", err)
	}

	return path, nil
}

var (
	_ Backend      = (*repoBackend)(nil)
	_ Updater      = (*repoBackend)(nil)
	_ Remover      = (*repoBackend)(nil)
	_ packageGated = (*repoBackend)(nil)
)
