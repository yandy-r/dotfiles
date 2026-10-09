package backend

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/privilege"
)

const yayAURRepo = "https://aur.archlinux.org/yay.git"

// AURUnsupportedOSError is returned when the AUR backend is invoked on a
// non-Arch host.
type AURUnsupportedOSError struct {
	OS platform.OS
}

// Error implements the error interface.
func (e *AURUnsupportedOSError) Error() string {
	return fmt.Sprintf("aur: unsupported on OS %q (requires %q)", e.OS, platform.OSArch)
}

type aurBackend struct {
	r    exec.Runner
	drop privilege.Dropper
}

// applicable reports whether the AUR backend can run on this host: Arch
// family only, mirroring ensureArch's guard.
func (b *aurBackend) applicable(info platform.Info) bool {
	return info.OS == platform.OSArch
}

func newAURBackend(r exec.Runner, drop privilege.Dropper) Backend {
	return &aurBackend{r: r, drop: drop}
}

func (b *aurBackend) Install(pkg manifest.Package, info platform.Info) error {
	return b.yayInstall(pkg, info, []string{"yay", "-S", "--noconfirm", aurPackageName(pkg)}, "install")
}

// Update re-runs yay with --needed so only an out-of-date package is rebuilt.
func (b *aurBackend) Update(pkg manifest.Package, info platform.Info) error {
	return b.yayInstall(pkg, info, []string{"yay", "-S", "--noconfirm", "--needed", aurPackageName(pkg)}, "update")
}

// Remove uninstalls the package with pacman as the invoking user (pacman
// self-elevates through sudo, like yay).
func (b *aurBackend) Remove(pkg manifest.Package, info platform.Info) error {
	if err := ensureArch(info); err != nil {
		return err
	}
	home, err := aurHome()
	if err != nil {
		return err
	}
	if err := b.runAsUser([]string{"sudo", "pacman", "-Rns", "--noconfirm", aurPackageName(pkg)}, home); err != nil {
		return fmt.Errorf("aur: remove %q: %w", pkg.Name, err)
	}
	return nil
}

func (b *aurBackend) yayInstall(pkg manifest.Package, info platform.Info, argv []string, verb string) error {
	if err := ensureArch(info); err != nil {
		return err
	}

	home, err := aurHome()
	if err != nil {
		return err
	}

	if _, err := b.r.LookPath("yay"); err != nil {
		if err := b.bootstrapYay(home); err != nil {
			return fmt.Errorf("aur: bootstrap yay: %w", err)
		}
	}

	if err := b.runAsUser(argv, home); err != nil {
		return fmt.Errorf("aur: %s %q: %w", verb, pkg.Name, err)
	}
	return nil
}

func (b *aurBackend) Installed(pkg manifest.Package, info platform.Info) (bool, error) {
	if err := ensureArch(info); err != nil {
		return false, err
	}

	if _, err := b.r.Output("pacman", "-Q", aurPackageName(pkg)); err != nil {
		return false, nil
	}
	return true, nil
}

func ensureArch(info platform.Info) error {
	if info.OS != platform.OSArch {
		return &AURUnsupportedOSError{OS: info.OS}
	}
	return nil
}

func aurHome() (string, error) {
	if os.Geteuid() != 0 {
		home := os.Getenv("HOME")
		if home == "" {
			return "", fmt.Errorf("aur: HOME is unset")
		}
		return home, nil
	}

	u, err := privilege.RealUser()
	if err != nil {
		return "", err
	}

	// Prefer numeric lookup: SUDO_USER may not exist in /etc/passwd (CI, minimal
	// containers) while SUDO_UID still identifies the invoking account.
	if usr, err := user.LookupId(strconv.FormatUint(uint64(u.UID), 10)); err == nil && usr.HomeDir != "" {
		return usr.HomeDir, nil
	}

	if usr, err := user.Lookup(u.Name); err == nil && usr.HomeDir != "" {
		return usr.HomeDir, nil
	}

	// passwd may omit SUDO_USER/SUDO_UID on CI and minimal containers; sudo often
	// preserves HOME, and /home/<user> is the conventional Arch layout.
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	return filepath.Join("/home", u.Name), nil
}

func (b *aurBackend) bootstrapYay(home string) error {
	if _, err := b.r.LookPath("makepkg"); err != nil {
		return fmt.Errorf("makepkg not found (install base-devel first): %w", err)
	}

	dir := filepath.Join(home, ".cache", "dotf", "yay-bootstrap")

	// git refuses to clone into an existing non-empty directory, so clear any
	// stale state from a prior failed attempt and clean up when done (mirrors
	// the github-release staging-dir pattern). In-process RemoveAll works
	// whether dotf runs as root or the user, and git recreates the directory
	// with correct ownership via runAsUser.
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove stale bootstrap dir %q: %w", dir, err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if err := b.runAsUser([]string{"git", "clone", yayAURRepo, dir}, home); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	argv := []string{"env", "--chdir=" + dir, "makepkg", "-si", "--noconfirm"}
	if err := b.runAsUser(argv, home); err != nil {
		return fmt.Errorf("makepkg: %w", err)
	}
	return nil
}

func (b *aurBackend) runAsUser(argv []string, home string) error {
	if len(argv) == 0 {
		return fmt.Errorf("aur: empty argv")
	}

	if os.Geteuid() == 0 {
		if _, err := privilege.RealUser(); err != nil {
			return err
		}
		return b.drop.Run(argv, home)
	}
	return b.r.Run(argv[0], argv[1:]...)
}

var (
	_ Updater = (*aurBackend)(nil)
	_ Remover = (*aurBackend)(nil)
)

// aurPackageName allows a logical manifest name with an explicit AUR identifier.
func aurPackageName(pkg manifest.Package) string {
	if name := pkg.Names["pacman"]; name != "" {
		return name
	}
	return pkg.Name
}
