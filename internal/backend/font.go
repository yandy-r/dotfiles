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

// fontBackend installs a patched font family: through the host package manager
// where a patched build exists, otherwise by extracting the checksum-verified
// upstream release archive into the user's fonts directory.
type fontBackend struct {
	r        exec.Runner
	fontsDir func(platform.Info) (string, error)
}

// newFontBackend constructs the font backend backed by r. fontsDir is a field
// for the same reason githubReleaseBackend.installDir is: tests replace it so
// an install never writes into the developer's real fonts directory.
func newFontBackend(r exec.Runner) *fontBackend {
	return &fontBackend{r: r, fontsDir: defaultFontsDir}
}

// FontNoChecksumError is returned when a download-route font resolves no
// expected sha256. VerifySHA256 treats an empty want as "skip verification", so
// the font backend rejects it BEFORE downloading rather than installing an
// unverified archive. Manifest validation makes this unreachable from a valid
// manifest; this is the defense-in-depth half.
type FontNoChecksumError struct {
	Package string
	Archive string
}

// Error implements the error interface.
func (e *FontNoChecksumError) Error() string {
	return fmt.Sprintf("backend/font: package %q has no checksum source for asset %q; set sha256 or checksums_asset", e.Package, e.Archive)
}

// defaultFontsDir resolves the per-user fonts directory for info: macOS reads
// ~/Library/Fonts; every other host follows the XDG spec — $XDG_DATA_HOME/fonts
// when that variable is absolute, else ~/.local/share/fonts.
func defaultFontsDir(info platform.Info) (string, error) {
	if info.OS == platform.OSMacOS {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, "Library", "Fonts"), nil
	}

	if dataHome := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dataHome) {
		return filepath.Join(dataHome, "fonts"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "fonts"), nil
}

// fontRoute is how one host installs one font family.
type fontRoute int

const (
	fontRouteNone fontRoute = iota
	fontRoutePacman
	fontRouteBrewCask
	fontRouteDownload
)

// fontRouteFor picks how THIS host installs pkg, and the name/archive that
// route needs. Package-manager routes win over the download route because a
// distro-patched build is updated by the package authority. Install and
// Installed both route through it, so the two can never disagree about which
// mechanism owns a family.
func fontRouteFor(pkg manifest.Package, info platform.Info) (fontRoute, string) {
	if info.PkgMgr == platform.PkgMgrPacman {
		if name := pkg.Names[string(platform.PkgMgrPacman)]; name != "" {
			return fontRoutePacman, name
		}
	}
	if info.OS == platform.OSMacOS {
		if name := pkg.Names[string(platform.PkgMgrHomebrew)]; name != "" {
			return fontRouteBrewCask, name
		}
	}
	if pkg.Font != nil && pkg.Font.Archive != "" && pkg.Repo != "" {
		return fontRouteDownload, pkg.Font.Archive
	}
	return fontRouteNone, ""
}

// applicable reports whether the font backend can run on this host. Every
// non-Windows host has either a patched package or the extract route; Windows
// font installation is deferred with the rest of the Windows port.
func (b *fontBackend) applicable(info platform.Info) bool {
	return info.OS != platform.OSWindows
}

// Install provisions the family through the route fontRouteFor picks: the
// distro-patched package where one exists, else the verified upstream archive.
func (b *fontBackend) Install(pkg manifest.Package, info platform.Info) error {
	route, name := fontRouteFor(pkg, info)
	switch route {
	case fontRoutePacman:
		return b.r.Run("sudo", "pacman", "-S", "--noconfirm", "--needed", name)
	case fontRouteBrewCask:
		// --cask, not the bare `brew install` of installArgv: Homebrew ships
		// font formulae as casks only.
		return b.r.Run("brew", "install", "--cask", name)
	case fontRouteDownload:
		return b.installArchive(pkg, info, name)
	default:
		return &UnsupportedPlatformError{
			Backend: manifest.BackendFont,
			OS:      info.OS,
			PkgMgr:  info.PkgMgr,
		}
	}
}

// installArchive downloads "<archiveBase>.zip" from the pinned release, verifies
// it against the resolved sha256, and promotes the extracted family into the
// fonts directory in one atomic rename.
func (b *fontBackend) installArchive(pkg manifest.Package, info platform.Info, archiveBase string) error {
	archive := archiveBase + ".zip"

	wantSHA, err := fontChecksum(pkg, archive)
	if err != nil {
		return err
	}
	if wantSHA == "" {
		return &FontNoChecksumError{Package: pkg.Name, Archive: archive}
	}

	dir, err := b.fontsDir(info)
	if err != nil {
		return err
	}
	dest := filepath.Join(dir, archiveBase)

	// Extraction is atomic: entries land in a staging directory and dest appears
	// only once every one of them is on disk. extractZip writes entries one at a
	// time, so extracting straight into dest would leave a partially populated
	// directory behind on a mid-loop error — or on a SIGINT, which no cleanup can
	// run for at all — and the Installed probe reads any non-empty dest as
	// installed forever. The staging directory lives INSIDE the fonts directory
	// so the promoting rename never crosses a filesystem boundary, and is
	// dot-prefixed to keep what a killed run leaves behind out of the way.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create fonts directory %q: %w", dir, err)
	}
	staging, err := os.MkdirTemp(dir, ".dotf-font-*")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()

	if err := fetchVerifyExtract(githubReleaseURL(pkg.Repo, pkg.Tag, archive), wantSHA, staging, archive); err != nil {
		return err
	}

	// MkdirTemp creates the staging directory 0o700; restore the 0o755 extractZip
	// gives every directory it creates itself, so the promoted family stays
	// readable the way a direct extraction would have left it.
	if err := os.Chmod(staging, 0o755); err != nil {
		return fmt.Errorf("set permissions on %q: %w", staging, err)
	}
	// Rename cannot replace a non-empty directory, and a partial dest left by an
	// earlier interrupted run is exactly what needs replacing.
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("remove previous install %q: %w", dest, err)
	}
	if err := os.Rename(staging, dest); err != nil {
		return fmt.Errorf("install fonts into %q: %w", dest, err)
	}

	// Best-effort cache refresh: a missing or failing fc-cache is never an
	// install failure, since the fonts are on disk and the cache rebuilds at
	// next login.
	if _, err := b.r.LookPath("fc-cache"); err == nil {
		_ = b.r.Run("fc-cache", "-f", dir)
	}
	return nil
}

// fontChecksum resolves the expected sha256 for archive: the inline sha256 when
// pinned, else the release's checksum sidecar. A sidecar error is FAIL-CLOSED —
// it propagates instead of degrading into an unverified download.
func fontChecksum(pkg manifest.Package, archive string) (string, error) {
	if pkg.SHA256 != "" {
		return pkg.SHA256, nil
	}
	if pkg.ChecksumsAsset != "" {
		return checksumFor(pkg.Repo, pkg.Tag, pkg.ChecksumsAsset, archive)
	}
	return "", nil
}

// Installed reports whether the family is present, probing through the SAME
// route Install would take. A package-manager query that errors means "not
// installed", matching pkgBackend.Installed's convention.
func (b *fontBackend) Installed(pkg manifest.Package, info platform.Info) (bool, error) {
	route, name := fontRouteFor(pkg, info)
	switch route {
	case fontRoutePacman:
		_, err := b.r.Output("pacman", "-Q", name)
		return err == nil, nil
	case fontRouteBrewCask:
		_, err := b.r.Output("brew", "list", "--cask", name)
		return err == nil, nil
	case fontRouteDownload:
		return b.installedArchive(pkg, info, name)
	default:
		return false, nil
	}
}

// installedArchive probes a download-route family with fontconfig first, since
// it reports the family name the configs actually pin. When fc-list is absent or
// fails it falls back to the extracted directory, counting entries rather than
// stat-ing the path so an empty directory left by a failed install does not read
// as installed.
func (b *fontBackend) installedArchive(pkg manifest.Package, info platform.Info, archiveBase string) (bool, error) {
	if _, err := b.r.LookPath("fc-list"); err == nil {
		if out, err := b.r.Output("fc-list", ":", "family"); err == nil {
			return strings.Contains(string(out), pkg.Font.Family), nil
		}
	}

	dir, err := b.fontsDir(info)
	if err != nil {
		return false, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, archiveBase))
	return err == nil && len(entries) > 0, nil
}

var (
	_ Backend       = (*fontBackend)(nil)
	_ platformGated = (*fontBackend)(nil)
)
