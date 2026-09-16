package backend

import (
	"fmt"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// pkgBackend installs packages via the host's native package manager.
type pkgBackend struct {
	r exec.Runner
}

// newPkgBackend constructs a pkgBackend backed by r.
func newPkgBackend(r exec.Runner) Backend {
	return &pkgBackend{r: r}
}

// MissingPkgNameError is returned when pkg.Names carries no usable entry for
// the detected package manager — the key is absent, or present with an empty
// value (which would otherwise interpolate into `install ""`).
type MissingPkgNameError struct {
	Package string
	PkgMgr  platform.PkgMgr
}

// Error implements the error interface.
func (e *MissingPkgNameError) Error() string {
	return fmt.Sprintf("backend/pkg: package %q: no name for package manager %q", e.Package, e.PkgMgr)
}

// UnsupportedPkgMgrError is returned when info.PkgMgr is not handled by
// the pkg backend.
type UnsupportedPkgMgrError struct {
	PkgMgr platform.PkgMgr
}

// Error implements the error interface.
func (e *UnsupportedPkgMgrError) Error() string {
	return fmt.Sprintf("backend/pkg: unsupported package manager %q", e.PkgMgr)
}

// Install resolves the manager-specific name and runs the native install
// command for info.PkgMgr.
func (b *pkgBackend) Install(pkg manifest.Package, info platform.Info) error {
	name, err := resolvePkgName(pkg, info)
	if err != nil {
		return err
	}

	argv, err := installArgv(info.PkgMgr, name)
	if err != nil {
		return err
	}

	return b.r.Run(argv[0], argv[1:]...)
}

// Update resolves the manager-specific name and runs the native upgrade command
// for info.PkgMgr. The argv carry sudo exactly as Install's do.
func (b *pkgBackend) Update(pkg manifest.Package, info platform.Info) error {
	name, err := resolvePkgName(pkg, info)
	if err != nil {
		return err
	}

	argv, err := upgradeArgv(info.PkgMgr, name)
	if err != nil {
		return err
	}

	return b.r.Run(argv[0], argv[1:]...)
}

// Remove resolves the manager-specific name and runs the native remove command
// for info.PkgMgr. The argv carry sudo exactly as Install's do.
func (b *pkgBackend) Remove(pkg manifest.Package, info platform.Info) error {
	name, err := resolvePkgName(pkg, info)
	if err != nil {
		return err
	}

	argv, err := removeArgv(info.PkgMgr, name)
	if err != nil {
		return err
	}

	return b.r.Run(argv[0], argv[1:]...)
}

// Installed reports whether pkg is present on the host via a read-only
// package-manager query.
func (b *pkgBackend) Installed(pkg manifest.Package, info platform.Info) (bool, error) {
	name, err := resolvePkgName(pkg, info)
	if err != nil {
		return false, err
	}

	cmd, args, err := installedQuery(info.PkgMgr, name)
	if err != nil {
		return false, err
	}

	out, err := b.r.Output(cmd, args...)
	if err != nil {
		return false, nil
	}
	if info.PkgMgr == platform.PkgMgrAPT {
		return dpkgStatusInstalled(out), nil
	}
	return true, nil
}

func resolvePkgName(pkg manifest.Package, info platform.Info) (string, error) {
	name, ok := pkg.Names[string(info.PkgMgr)]
	if !ok || name == "" {
		return "", &MissingPkgNameError{Package: pkg.Name, PkgMgr: info.PkgMgr}
	}
	return name, nil
}

// applicableFor reports whether pkg can be installed on info through the host's
// native package manager: the entry must declare a NON-EMPTY name for
// info.PkgMgr. It uses the SAME pkg.Names[string(info.PkgMgr)] lookup and the
// same non-empty check as resolvePkgName so applicability and the
// MissingPkgNameError path can never disagree — and so a typo'd `dnf = ""`
// cannot read as applicable and then interpolate into `install ""`. Every other
// Names consumer (repo.go, repo_dnf.go, toolchain.go, font.go) applies the same
// non-empty rule. A manifest entry is applicable exactly where it has a name,
// which turns a genuinely unpackaged tool from a fail-late probe error (aborting
// the entire dotf status run at cmd/dotf/status.go:117-120) into one clean
// applicable=false row.
func (b *pkgBackend) applicableFor(pkg manifest.Package, info platform.Info) bool {
	name, ok := pkg.Names[string(info.PkgMgr)]
	return ok && name != ""
}

func installArgv(mgr platform.PkgMgr, name string) ([]string, error) {
	switch mgr {
	case platform.PkgMgrPacman:
		return []string{"sudo", "pacman", "-S", "--noconfirm", "--needed", name}, nil
	case platform.PkgMgrAPT:
		return []string{"sudo", "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", name}, nil
	case platform.PkgMgrDNF:
		return []string{"sudo", "dnf", "install", "-y", name}, nil
	case platform.PkgMgrZypper:
		return []string{"sudo", "zypper", "--non-interactive", "install", name}, nil
	case platform.PkgMgrHomebrew:
		return []string{"brew", "install", name}, nil
	default:
		return nil, &UnsupportedPkgMgrError{PkgMgr: mgr}
	}
}

// upgradeArgv builds the manager-specific in-place upgrade command for name,
// mirroring installArgv's sudo/non-interactive conventions per manager.
func upgradeArgv(mgr platform.PkgMgr, name string) ([]string, error) {
	switch mgr {
	case platform.PkgMgrPacman:
		return []string{"sudo", "pacman", "-S", "--noconfirm", "--needed", name}, nil
	case platform.PkgMgrAPT:
		return []string{"sudo", "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "--only-upgrade", "-y", name}, nil
	case platform.PkgMgrDNF:
		return []string{"sudo", "dnf", "upgrade", "-y", name}, nil
	case platform.PkgMgrZypper:
		return []string{"sudo", "zypper", "--non-interactive", "update", name}, nil
	case platform.PkgMgrHomebrew:
		return []string{"brew", "upgrade", name}, nil
	default:
		return nil, &UnsupportedPkgMgrError{PkgMgr: mgr}
	}
}

// removeArgv builds the manager-specific remove command for name, mirroring
// installArgv's sudo/non-interactive conventions per manager.
func removeArgv(mgr platform.PkgMgr, name string) ([]string, error) {
	switch mgr {
	case platform.PkgMgrPacman:
		return []string{"sudo", "pacman", "-Rns", "--noconfirm", name}, nil
	case platform.PkgMgrAPT:
		return []string{"sudo", "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "remove", "-y", name}, nil
	case platform.PkgMgrDNF:
		return []string{"sudo", "dnf", "remove", "-y", name}, nil
	case platform.PkgMgrZypper:
		return []string{"sudo", "zypper", "--non-interactive", "remove", name}, nil
	case platform.PkgMgrHomebrew:
		return []string{"brew", "uninstall", name}, nil
	default:
		return nil, &UnsupportedPkgMgrError{PkgMgr: mgr}
	}
}

func installedQuery(mgr platform.PkgMgr, name string) (string, []string, error) {
	switch mgr {
	case platform.PkgMgrPacman:
		return "pacman", []string{"-Q", name}, nil
	case platform.PkgMgrAPT:
		cmd, args := dpkgInstalledQuery(name)
		return cmd, args, nil
	case platform.PkgMgrDNF:
		return "rpm", []string{"-q", name}, nil
	case platform.PkgMgrZypper:
		// zypper's own search (`zypper se -i`) substring-matches, so probe
		// the RPM database directly for an exact name match instead.
		return "rpm", []string{"-q", name}, nil
	case platform.PkgMgrHomebrew:
		return "brew", []string{"list", name}, nil
	default:
		return "", nil, &UnsupportedPkgMgrError{PkgMgr: mgr}
	}
}

// dpkgInstalledQuery returns the exact-status dpkg probe for name. The
// ${db:Status-Status} field (dpkg >= 1.17.11) prints the package state so
// callers can distinguish installed packages from removed-but-not-purged
// ones, for which dpkg still exits 0.
func dpkgInstalledQuery(name string) (string, []string) {
	return "dpkg-query", []string{"-W", "-f=${db:Status-Status}", name}
}

// dpkgStatusInstalled reports whether a dpkg ${db:Status-Status} probe
// output denotes a fully installed package (state "installed", as opposed
// to e.g. "config-files" for removed-but-not-purged packages).
func dpkgStatusInstalled(out []byte) bool {
	return strings.TrimSpace(string(out)) == "installed"
}

var (
	_ Backend      = (*pkgBackend)(nil)
	_ Updater      = (*pkgBackend)(nil)
	_ Remover      = (*pkgBackend)(nil)
	_ packageGated = (*pkgBackend)(nil)
)
