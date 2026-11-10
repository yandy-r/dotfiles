package backend

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// appArtifactBackend installs a user-level Linux GUI app (AppImage or tarball)
// into $XDG_DATA_HOME/dotf/apps/<name> with a dotf-owned desktop entry at
// $XDG_DATA_HOME/applications/dotf-<name>.desktop. No root, no artifact is ever
// executed during install. Update stages a full verified copy and swaps it in
// only after every step succeeded; Remove touches only those two dotf paths.
type appArtifactBackend struct {
	r    exec.Runner
	lock *lockfile.Lock
}

func newAppArtifactBackend(r exec.Runner) *appArtifactBackend { return &appArtifactBackend{r: r} }

// withLock sets the (nil-tolerant) lock that overrides URL/checksum on Install.
func (b *appArtifactBackend) withLock(l *lockfile.Lock) *appArtifactBackend {
	b.lock = l
	return b
}

// resolve implements lockable.
func (b *appArtifactBackend) resolve(pkg manifest.Package, target platform.Info) (lockfile.Artifact, error) {
	return lockArtifact(pkg, target, nil)
}

const appMetaFile = ".dotf-meta"

type appMeta struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
	Exec   string `json:"exec"` // relative to the app dir
}

func dataHome() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		if !filepath.IsAbs(d) {
			return "", fmt.Errorf("XDG_DATA_HOME %q is not absolute", d)
		}
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share"), nil
}

// appNameRe is the trust boundary for pkg.Name before it becomes a path
// segment: no separators, no leading dot/dash, no "..".
var appNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// appPaths validates name and returns the dotf-owned app dir and desktop entry.
// Any symlink at the dotf root, apps root, app dir or desktop path is refused,
// so a planted link can never redirect an install or a removal.
func appPaths(name string) (appDir, desktop string, err error) {
	if !appNameRe.MatchString(name) || strings.Contains(name, "..") {
		return "", "", fmt.Errorf("app-artifact: unsafe app name %q", name)
	}
	d, err := dataHome()
	if err != nil {
		return "", "", err
	}
	root := filepath.Join(d, "dotf")
	appDir = filepath.Join(root, "apps", name)
	desktop = filepath.Join(d, "applications", "dotf-"+name+".desktop")
	for _, p := range []string{root, filepath.Dir(appDir), appDir, desktop} {
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("app-artifact %q: %s is a symlink; refusing", name, p)
		}
	}
	return appDir, desktop, nil
}

// managedDir reports whether appDir is a real (non-symlink) directory carrying a
// regular dotf meta marker.
func managedDir(appDir string) error {
	fi, err := os.Lstat(appDir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", appDir)
	}
	mi, err := os.Lstat(filepath.Join(appDir, appMetaFile))
	if err != nil || !mi.Mode().IsRegular() {
		return fmt.Errorf("%s is not dotf-managed", appDir)
	}
	return nil
}

// safeAppExec resolves a meta/staging Exec value: it must be a clean relative
// path whose every component is a non-symlink and whose leaf is a regular
// executable file. Returns the absolute path.
func safeAppExec(appDir, rel string) (string, error) {
	sep := string(filepath.Separator)
	if rel == "" || rel == "." || rel == ".." || filepath.IsAbs(rel) ||
		filepath.Clean(rel) != rel || strings.HasPrefix(rel, ".."+sep) {
		return "", fmt.Errorf("unsafe exec path %q", rel)
	}
	parts := strings.Split(rel, sep)
	p := appDir
	for i, part := range parts {
		p = filepath.Join(p, part)
		fi, err := os.Lstat(p)
		if err != nil {
			return "", err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("exec path %q contains a symlink", rel)
		}
		if i < len(parts)-1 {
			if !fi.IsDir() {
				return "", fmt.Errorf("exec path %q: %s is not a directory", rel, part)
			}
		} else if !fi.Mode().IsRegular() || fi.Mode()&0o111 == 0 {
			return "", fmt.Errorf("exec %q is not a regular executable file", rel)
		}
	}
	return p, nil
}

func (b *appArtifactBackend) applicable(info platform.Info) bool {
	return platform.IsLinuxFamily(info.OS)
}

func (b *appArtifactBackend) Installed(pkg manifest.Package, _ platform.Info) (bool, error) {
	appDir, desktop, err := appPaths(pkg.Name)
	if err != nil {
		return false, err
	}
	if managedDir(appDir) != nil {
		return false, nil
	}
	raw, err := os.ReadFile(filepath.Join(appDir, appMetaFile))
	if err != nil {
		return false, nil
	}
	var m appMeta
	if json.Unmarshal(raw, &m) != nil {
		return false, nil
	}
	if _, err := safeAppExec(appDir, m.Exec); err != nil {
		return false, nil
	}
	if fi, err := os.Lstat(desktop); err != nil || !fi.Mode().IsRegular() {
		return false, nil
	}
	return true, nil
}

func (b *appArtifactBackend) Update(pkg manifest.Package, info platform.Info) error {
	return b.Install(pkg, info)
}

// Install (and Update) download + verify + extract into a staging dir next to
// the final location, then swap it in atomically, restoring the previous copy
// if the swap or desktop-entry write fails.
func (b *appArtifactBackend) Install(pkg manifest.Package, info platform.Info) error {
	if !b.applicable(info) {
		return &UnsupportedPlatformError{Backend: manifest.BackendAppArtifact, OS: info.OS, PkgMgr: info.PkgMgr}
	}
	a, err := resolveArtifactLocked(pkg, info, nil, b.lock)
	if err != nil {
		return err
	}
	if a.SHA256 == "" && pkg.EffectiveClass() == manifest.ClassPinned {
		osTok, archTok := releaseTokens(info)
		return pinnedNoChecksumErr(pkg.Name, osTok, archTok)
	}
	appDir, desktop, err := appPaths(pkg.Name)
	if err != nil {
		return err
	}
	parent := filepath.Dir(appDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".staging-"+pkg.Name+"-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staging) }()

	if err := fetchVerifyExtract(a.URL, a.SHA256, staging, a.Asset); err != nil {
		return fmt.Errorf("app-artifact %q: %w", pkg.Name, err)
	}
	execRel, err := appExecutable(staging, pkg.Name, pkg.Binary)
	if err != nil {
		return fmt.Errorf("app-artifact %q: %w", pkg.Name, err)
	}
	if pkg.Binary == "" { // bare AppImage; tarball launchers may be scripts
		if err := checkELF(filepath.Join(staging, execRel), info.Arch); err != nil {
			return fmt.Errorf("app-artifact %q: %w", pkg.Name, err)
		}
	} else if err := checkTreeELF(staging, info.Arch); err != nil {
		return fmt.Errorf("app-artifact %q: %w", pkg.Name, err)
	}
	iconRel, err := appIcon(staging, pkg.App)
	if err != nil {
		return fmt.Errorf("app-artifact %q: %w", pkg.Name, err)
	}
	vendor, err := vendorDesktopKeys(staging, pkg.App)
	if err != nil {
		return fmt.Errorf("app-artifact %q: %w", pkg.Name, err)
	}
	meta, _ := json.Marshal(appMeta{URL: a.URL, SHA256: a.SHA256, Exec: execRel})
	if err := os.WriteFile(filepath.Join(staging, appMetaFile), meta, 0o644); err != nil {
		return err
	}
	entry, err := desktopEntry(pkg, filepath.Join(appDir, execRel), joinIfSet(appDir, iconRel), vendor)
	if err != nil {
		return fmt.Errorf("app-artifact %q: %w", pkg.Name, err)
	}

	// Swap: old -> backup, staging -> final, then the desktop entry. On a rename
	// failure the old dir stays in place; only a half-written staging rename is
	// removed — never the previous artifact.
	backup := ""
	if _, err := os.Lstat(appDir); err == nil {
		if managedDir(appDir) != nil {
			return fmt.Errorf("app-artifact %q: %s exists but is not dotf-managed; refusing to replace", pkg.Name, appDir)
		}
		backup = appDir + ".old"
		if _, err := os.Lstat(backup); err == nil {
			return fmt.Errorf("app-artifact %q: leftover %s from interrupted update; inspect and remove it", pkg.Name, backup)
		}
		if err := os.Rename(appDir, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, appDir); err != nil {
		// Restore the previous artifact; a failed restore is reported, not
		// hidden — old data is never deleted here.
		if backup != "" {
			if rerr := os.Rename(backup, appDir); rerr != nil {
				return fmt.Errorf("stage swap failed: %v; restore of %s failed: %v", err, backup, rerr)
			}
		}
		return err
	}
	if err := writeFileAtomic(desktop, entry, 0o644); err != nil {
		if backup == "" {
			_ = os.RemoveAll(appDir)
			return err
		}
		failed := appDir + ".failed"
		if rerr := os.Rename(appDir, failed); rerr != nil {
			return fmt.Errorf("desktop entry write failed: %v; could not move new copy aside: %v", err, rerr)
		}
		if rerr := os.Rename(backup, appDir); rerr != nil {
			return fmt.Errorf("desktop entry write failed: %v; restore of %s failed: %v", err, backup, rerr)
		}
		_ = os.RemoveAll(failed)
		return err
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	if _, err := b.r.LookPath("update-desktop-database"); err == nil {
		_ = b.r.Run("update-desktop-database", filepath.Dir(desktop)) // best effort
	}
	return nil
}

// Remove deletes only the dotf-managed app dir (must carry the meta marker)
// and the dotf-<name>.desktop entry.
func (b *appArtifactBackend) Remove(pkg manifest.Package, _ platform.Info) error {
	appDir, desktop, err := appPaths(pkg.Name)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(appDir); err == nil {
		if err := managedDir(appDir); err != nil {
			return fmt.Errorf("app-artifact %q: %w; refusing to remove", pkg.Name, err)
		}
		if err := os.RemoveAll(appDir); err != nil {
			return err
		}
	}
	// Only remove a desktop entry dotf wrote.
	if raw, err := os.ReadFile(desktop); err == nil {
		if !bytes.Contains(raw, []byte("X-Dotf-Managed=true")) {
			return fmt.Errorf("app-artifact %q: %s is not dotf-managed; refusing to remove", pkg.Name, desktop)
		}
		if err := os.Remove(desktop); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

// appExecutable returns the executable path relative to dir: the declared
// binary (guarded against traversal), else the sole extracted regular file
// (the AppImage case, renamed to <pkgName>.AppImage), which is made executable.
func appExecutable(dir, pkgName, binary string) (string, error) {
	if binary != "" {
		p, err := safeExtractPath(dir, binary)
		if err != nil {
			return "", err
		}
		fi, err := os.Lstat(p)
		if err != nil || !fi.Mode().IsRegular() {
			return "", fmt.Errorf("binary %q not found as a regular file in artifact", binary)
		}
		if err := os.Chmod(p, 0o755); err != nil {
			return "", err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return "", err
		}
		if _, err := safeAppExec(dir, rel); err != nil {
			return "", err
		}
		return rel, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	if len(entries) != 1 || !entries[0].Type().IsRegular() {
		return "", fmt.Errorf("artifact is not a single file; set `binary` to the executable path")
	}
	name := entries[0].Name()
	// Store a bare AppImage under a version-free name. In-app self-updaters
	// (electron-updater) overwrite the running file in place only when its name
	// carries no x.y.z version; otherwise they write a new versioned file and
	// delete the old one, leaving the dotf launcher pointing at nothing.
	if stable := pkgName + ".AppImage"; strings.HasSuffix(strings.ToLower(name), ".appimage") && name != stable {
		if err := os.Rename(filepath.Join(dir, name), filepath.Join(dir, stable)); err != nil {
			return "", err
		}
		name = stable
	}
	if err := os.Chmod(filepath.Join(dir, name), 0o755); err != nil {
		return "", err
	}
	if _, err := safeAppExec(dir, name); err != nil {
		return "", err
	}
	return name, nil
}

// checkELF requires a little-endian ELF executable for the host architecture.
func checkELF(path string, arch platform.Arch) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := make([]byte, 20)
	if _, err := io.ReadFull(f, h); err != nil || !bytes.Equal(h[:4], []byte("\x7fELF")) || h[5] != 1 {
		return fmt.Errorf("downloaded executable is not a little-endian ELF binary")
	}
	want := map[platform.Arch]uint16{platform.ArchX86_64: 62, platform.ArchARM64: 183}[arch]
	if got := binary.LittleEndian.Uint16(h[18:20]); want == 0 || got != want {
		return fmt.Errorf("ELF architecture %d does not match host %s", got, arch)
	}
	return nil
}

// checkTreeELF rejects archives containing ELF binaries for another architecture.
// Archives with only scripts are accepted; their launcher must exec bundled files.
func checkTreeELF(root string, arch platform.Arch) error {
	found := false
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		magic := make([]byte, 4)
		_, readErr := io.ReadFull(f, magic)
		_ = f.Close()
		if readErr != nil || !bytes.Equal(magic, []byte("\x7fELF")) {
			return nil
		}
		found = true
		return checkELF(path, arch)
	})
	if err == nil && !found {
		return fmt.Errorf("archive contains no ELF executable")
	}
	return err
}

// appIcon materializes the icon inside dir and returns its relative path ("" =
// none). An https icon is downloaded and must be PNG or SVG.
func appIcon(dir string, app *manifest.AppInstall) (string, error) {
	if app == nil || app.Icon == "" {
		return "", nil
	}
	if !strings.HasPrefix(app.Icon, "https://") {
		p, err := safeExtractPath(dir, app.Icon)
		if err != nil {
			return "", err
		}
		if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
			return "", fmt.Errorf("icon %q not found as a regular file in artifact", app.Icon)
		}
		return filepath.Rel(dir, p)
	}
	tmp, err := fetchVerifyFile(app.Icon, "", dir, ".icon-*")
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(tmp)
	if err != nil {
		return "", err
	}
	ext := ""
	switch {
	case bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")):
		ext = ".png"
	case bytes.Contains(raw[:min(len(raw), 512)], []byte("<svg")):
		ext = ".svg"
	default:
		_ = os.Remove(tmp)
		return "", fmt.Errorf("icon %q is neither PNG nor SVG", app.Icon)
	}
	name := ".dotf-icon" + ext
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return "", err
	}
	return name, os.Chmod(filepath.Join(dir, name), 0o644)
}

// vendorKeys are the only keys copied from a vendor .desktop file; Exec, Icon,
// TryExec and actions are never trusted.
var vendorKeys = []string{"Name", "GenericName", "Comment", "Categories", "MimeType", "StartupWMClass", "Keywords"}

func vendorDesktopKeys(dir string, app *manifest.AppInstall) (map[string]string, error) {
	out := map[string]string{}
	if app == nil || app.Desktop == "" {
		return out, nil
	}
	p, err := safeExtractPath(dir, app.Desktop)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("desktop file %q not found as a regular file in artifact", app.Desktop)
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("desktop file %q not found in artifact", app.Desktop)
	}
	defer func() { _ = f.Close() }()
	inMain := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inMain = line == "[Desktop Entry]"
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if inMain && ok {
			for _, want := range vendorKeys {
				if k == want {
					out[k] = v
				}
			}
		}
	}
	return out, sc.Err()
}

// desktopEntry renders the dotf-owned entry. Exec is the quoted absolute path
// (no shell); characters that would need desktop-spec escaping are rejected.
func desktopEntry(pkg manifest.Package, execPath, iconPath string, vendor map[string]string) ([]byte, error) {
	for _, s := range []string{execPath, iconPath} {
		if strings.ContainsAny(s, "\"`$\\%\n\r") {
			return nil, fmt.Errorf("unsafe character in path %q", s)
		}
	}
	keys := map[string]string{"Name": pkg.Name}
	for k, v := range vendor {
		keys[k] = v
	}
	execLine := `"` + execPath + `"`
	if pkg.App != nil {
		if pkg.App.DesktopName != "" {
			keys["Name"] = pkg.App.DesktopName
		}
		if pkg.App.Categories != "" {
			keys["Categories"] = pkg.App.Categories
		}
		if pkg.App.StartupWMClass != "" {
			keys["StartupWMClass"] = pkg.App.StartupWMClass
		}
		if pkg.App.MimeType != "" {
			keys["MimeType"] = pkg.App.MimeType
		}
		if pkg.App.ExecArgs != "" {
			execLine += " " + pkg.App.ExecArgs
		}
	}
	keys["Exec"] = execLine
	if iconPath != "" {
		keys["Icon"] = iconPath
	}
	var buf bytes.Buffer
	buf.WriteString("[Desktop Entry]\nType=Application\nTerminal=false\nX-Dotf-Managed=true\n")
	for _, k := range append([]string{"Exec", "Icon"}, vendorKeys...) {
		v, ok := keys[k]
		if !ok {
			continue
		}
		if strings.ContainsAny(v, "\n\r") {
			return nil, fmt.Errorf("newline in desktop key %s", k)
		}
		fmt.Fprintf(&buf, "%s=%s\n", k, v)
	}
	return buf.Bytes(), nil
}

func joinIfSet(dir, rel string) string {
	if rel == "" {
		return ""
	}
	return filepath.Join(dir, rel)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dotf-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

var (
	_ Updater       = (*appArtifactBackend)(nil)
	_ Remover       = (*appArtifactBackend)(nil)
	_ platformGated = (*appArtifactBackend)(nil)
	_ lockable      = (*appArtifactBackend)(nil)
)
