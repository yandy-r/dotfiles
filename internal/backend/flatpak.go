package backend

import (
	"fmt"
	"os"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// flathubRemote is the default remote, and the only one the backend adds on
// demand: any other remote must already be configured on the host.
const (
	flathubRemote = "flathub"
	flathubRepo   = "https://dl.flathub.org/repo/flathub.flatpakrepo"
)

// FlatpakNotFoundError is returned by Install/Update/Remove when the flatpak
// executable is absent from PATH. It fails fast (like GitNotFoundError) rather
// than skipping, so a missing dependency is reported; manifest entries declare
// requires = ["flatpak"] so the engine installs it first.
type FlatpakNotFoundError struct {
	Package string
}

// Error implements the error interface.
func (e *FlatpakNotFoundError) Error() string {
	return fmt.Sprintf("flatpak: package %q: flatpak executable not found on PATH", e.Package)
}

// FlatpakRootError is returned when the backend would install as root: a
// --user installation would land in root's home instead of the invoking
// user's, which is never what a dotfiles apply means.
type FlatpakRootError struct {
	Package string
}

// Error implements the error interface.
func (e *FlatpakRootError) Error() string {
	return fmt.Sprintf("flatpak: package %q: refusing to install a --user flatpak as root; run dotf as your user", e.Package)
}

// flatpakBackend installs Flatpak applications into the invoking user's
// installation (--user), so no privilege elevation is involved. Installed,
// Update and Remove address the app ID without --user/--system, so an app the
// user already installed system-wide is recognised and managed in place.
type flatpakBackend struct {
	r    exec.Runner
	euid func() int
}

func newFlatpakBackend(r exec.Runner) *flatpakBackend {
	return &flatpakBackend{r: r, euid: os.Geteuid}
}

// applicable gates the backend to Linux-family hosts: flatpak does not exist
// on macOS or Windows.
func (b *flatpakBackend) applicable(info platform.Info) bool {
	return osGateAllows("linux", info.OS)
}

func flatpakRemoteOf(pkg manifest.Package) string {
	if pkg.FlatpakRemote != "" {
		return pkg.FlatpakRemote
	}
	return flathubRemote
}

func (b *flatpakBackend) requireFlatpak(pkg manifest.Package) error {
	if _, err := b.r.LookPath("flatpak"); err != nil {
		return &FlatpakNotFoundError{Package: pkg.Name}
	}
	return nil
}

// Install adds flathub to the user installation when it is the target remote
// (--if-not-exists makes this idempotent), then installs the app ID
// non-interactively.
func (b *flatpakBackend) Install(pkg manifest.Package, _ platform.Info) error {
	if err := b.requireFlatpak(pkg); err != nil {
		return err
	}
	if b.euid() == 0 {
		return &FlatpakRootError{Package: pkg.Name}
	}

	remote := flatpakRemoteOf(pkg)
	if remote == flathubRemote {
		if err := b.r.Run("flatpak", "remote-add", "--user", "--if-not-exists", flathubRemote, flathubRepo); err != nil {
			return fmt.Errorf("flatpak: package %q: add remote %q: %w", pkg.Name, flathubRemote, err)
		}
	}
	if err := b.r.Run("flatpak", "install", "--user", "--noninteractive", "-y", remote, pkg.FlatpakID); err != nil {
		return fmt.Errorf("flatpak: package %q: install %s from %s: %w", pkg.Name, pkg.FlatpakID, remote, err)
	}
	return nil
}

// Installed reports whether the app ID is present in any flatpak installation
// (user or system). A host without flatpak reports not installed.
func (b *flatpakBackend) Installed(pkg manifest.Package, _ platform.Info) (bool, error) {
	if _, err := b.r.LookPath("flatpak"); err != nil {
		return false, nil
	}
	if _, err := b.r.Output("flatpak", "info", pkg.FlatpakID); err != nil {
		return false, nil
	}
	return true, nil
}

// Update updates the app in whichever installation holds it.
func (b *flatpakBackend) Update(pkg manifest.Package, _ platform.Info) error {
	if err := b.requireFlatpak(pkg); err != nil {
		return err
	}
	if err := b.r.Run("flatpak", "update", "--noninteractive", "-y", pkg.FlatpakID); err != nil {
		return fmt.Errorf("flatpak: package %q: update %s: %w", pkg.Name, pkg.FlatpakID, err)
	}
	return nil
}

// Remove uninstalls the app from whichever installation holds it.
func (b *flatpakBackend) Remove(pkg manifest.Package, _ platform.Info) error {
	if err := b.requireFlatpak(pkg); err != nil {
		return err
	}
	if err := b.r.Run("flatpak", "uninstall", "--noninteractive", "-y", pkg.FlatpakID); err != nil {
		return fmt.Errorf("flatpak: package %q: remove %s: %w", pkg.Name, pkg.FlatpakID, err)
	}
	return nil
}

var (
	_ Backend       = (*flatpakBackend)(nil)
	_ Updater       = (*flatpakBackend)(nil)
	_ Remover       = (*flatpakBackend)(nil)
	_ platformGated = (*flatpakBackend)(nil)
)
