package backend

import (
	"errors"
	"fmt"
	"io/fs"
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
	// modes write it, and snapshots the apt keyring and sources file before
	// installAPT overwrites them. It is os.ReadFile in production; tests inject
	// a fake so they never read the real /etc.
	readFile func(string) ([]byte, error)

	// repoIDs reports the dnf repo ids configured on the host (see
	// hostDNFRepoIDs). Tests inject a fake so they never read the real /etc.
	repoIDs func() map[string]bool
}

func newRepoBackend(r exec.Runner) *repoBackend {
	return &repoBackend{r: r, readFile: os.ReadFile, repoIDs: hostDNFRepoIDs}
}

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

func (b *repoBackend) Install(pkg manifest.Package, info platform.Info) error {
	if !b.applicableFor(pkg, info) {
		return &UnsupportedPlatformError{
			Backend: manifest.BackendRepo,
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
		return &UnsupportedPlatformError{
			Backend: manifest.BackendRepo,
			OS:      info.OS,
			PkgMgr:  info.PkgMgr,
		}
	}
}

// Update upgrades an already-configured repo package via the host package
// manager. The repo itself is not (re)configured, but on dnf a kept
// non-dotf repo file's [<id>] section is enabled for the upgrade and a
// dnf_repo priority is (re)pinned, as on install (see dnfRepoArgs).
func (b *repoBackend) Update(pkg manifest.Package, info platform.Info) error {
	if info.PkgMgr != platform.PkgMgrDNF || pkg.DNFRepo == nil {
		return b.lifecycle(pkg, info, upgradeArgv)
	}
	name := pkg.Names[string(platform.PkgMgrDNF)]
	if name == "" {
		return b.lifecycle(pkg, info, upgradeArgv) // reports MissingPackageNameError
	}
	if err := b.pinDNFRepo(pkg.DNFRepo, name, info); err != nil {
		return err
	}
	enable, err := b.dnfRepoArgs(pkg.DNFRepo, name)
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
		return &UnsupportedPlatformError{Backend: manifest.BackendRepo, OS: info.OS, PkgMgr: info.PkgMgr}
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

	// Render before any download or sudo step: a host the line cannot be
	// rendered for (unknown codename or apt base) must leave nothing behind.
	repoLine, err := renderRepoLine(repo.RepoLine, info)
	if err != nil {
		return err
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

	// Snapshot both root-owned files before either is replaced, so a failed
	// apt-get update can put the host back exactly as it was. A file that
	// exists but cannot be read would be unrestorable, so refuse to touch it.
	keyringPrior, err := b.snapshotAptFile(repo.Keyring)
	if err != nil {
		return err
	}
	sourcesPrior, err := b.snapshotAptFile(repo.SourcesPath)
	if err != nil {
		return err
	}

	keyringDir := filepath.Dir(repo.Keyring)
	if err := b.r.Run("sudo", "install", "-d", "-m", "0755", keyringDir); err != nil {
		return fmt.Errorf("create apt keyring directory %q: %w", keyringDir, err)
	}

	if err := b.r.Run("sudo", "install", "-m", "0644", pinned, repo.Keyring); err != nil {
		return fmt.Errorf("install apt repo keyring %q: %w", repo.Keyring, err)
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
		// A sources entry apt cannot fetch (a 404 suite, a dead mirror) fails
		// every later apt-get update on the host, including the next apt-repo
		// package in this run. Roll back the sources file first, then the
		// keyring, before reporting the failure.
		updateErr := fmt.Errorf("apt-get update: %w", err)
		return errors.Join(updateErr,
			b.restoreAptFile(sourcesPrior, "644"),
			b.restoreAptFile(keyringPrior, "0644"))
	}

	if err := b.r.Run("sudo", "apt-get", "install", "-y", aptName); err != nil {
		return fmt.Errorf("apt-get install %q: %w", aptName, err)
	}

	return nil
}

// aptFileSnapshot is what a root-owned apt file held before installAPT
// overwrote it, so a failed apt-get update can restore it.
type aptFileSnapshot struct {
	path string
	// existed reports the file was present; body is then its prior content.
	existed bool
	body    []byte
}

// snapshotAptFile records path's current content. A missing file snapshots as
// "created by dotf", so rollback removes it. A file that exists but cannot be
// read is an error: overwriting it would make rollback impossible.
func (b *repoBackend) snapshotAptFile(path string) (aptFileSnapshot, error) {
	body, err := b.readFile(path)
	switch {
	case err == nil:
		if body == nil {
			body = []byte{}
		}
		return aptFileSnapshot{path: path, existed: true, body: body}, nil
	case errors.Is(err, fs.ErrNotExist):
		return aptFileSnapshot{path: path}, nil
	default:
		return aptFileSnapshot{}, fmt.Errorf("read existing %q before replacing it: %w", path, err)
	}
}

// restoreAptFile undoes installAPT's write of s.path: it removes a file dotf
// created and rewrites a file it replaced with its prior content (mode).
func (b *repoBackend) restoreAptFile(s aptFileSnapshot, mode string) error {
	if !s.existed {
		if err := b.r.Run("sudo", "rm", "-f", s.path); err != nil {
			return fmt.Errorf("roll back %q: %w", s.path, err)
		}
		return nil
	}
	tmp, err := writeTempFile("dotf-apt-restore-*", s.body)
	if err != nil {
		return fmt.Errorf("roll back %q: %w", s.path, err)
	}
	defer func() { _ = os.Remove(tmp) }()
	if err := b.r.Run("sudo", "install", "-m", mode, tmp, s.path); err != nil {
		return fmt.Errorf("roll back %q: %w", s.path, err)
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
			Backend: manifest.BackendRepo,
			OS:      info.OS,
			PkgMgr:  info.PkgMgr,
		}
	}
}

// renderRepoLine substitutes the host placeholders in a repo_line so the
// suite, distro path and arch track the target machine instead of being
// hardcoded:
//
//   - {{.Codename}}: os-release VERSION_CODENAME (fallback UBUNTU_CODENAME).
//   - {{.UbuntuCodename}}: UBUNTU_CODENAME (fallback VERSION_CODENAME), the
//     suite for repos that publish Ubuntu dists only, so Mint/elementary/Pop
//     resolve to their Ubuntu base (wilma -> noble).
//   - {{.BaseDistro}} / {{.BaseCodename}}: the vendor path ("ubuntu" or
//     "debian") and matching suite, for repos that publish both trees.
//   - {{.Arch}}: the dpkg release arch.
//
// It errors when the line needs a value the host does not report, since a
// bare/empty suite or path would 404 on apt-get update.
func renderRepoLine(line string, info platform.Info) (string, error) {
	for _, v := range []struct{ placeholder, value, what, source string }{
		{"{{.Codename}}", info.Codename, "codename", "os-release VERSION_CODENAME/UBUNTU_CODENAME absent"},
		{"{{.UbuntuCodename}}", info.UbuntuCodename, "Ubuntu codename", "os-release UBUNTU_CODENAME/VERSION_CODENAME absent"},
		{"{{.BaseDistro}}", info.BaseDistro, "apt base distro", "host is neither Ubuntu- nor Debian-based"},
		{"{{.BaseCodename}}", info.BaseCodename, "apt base codename", "no Ubuntu/Debian base codename in os-release"},
	} {
		if strings.Contains(line, v.placeholder) && v.value == "" {
			return "", fmt.Errorf("repo: host %s unknown (%s); cannot render repo_line %q", v.what, v.source, line)
		}
	}
	return strings.NewReplacer(
		"{{.Codename}}", info.Codename,
		"{{.UbuntuCodename}}", info.UbuntuCodename,
		"{{.BaseDistro}}", info.BaseDistro,
		"{{.BaseCodename}}", info.BaseCodename,
		"{{.Arch}}", info.Arch.ReleaseArch(),
	).Replace(line), nil
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
