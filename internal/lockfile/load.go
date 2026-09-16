package lockfile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// Parse decodes a TOML-encoded lock from r with strict unknown-field checking
// and rejects an unsupported Version. It returns (nil, err) on ANY failure —
// decode or version — so callers never receive a half-populated *Lock,
// mirroring manifest.Parse's fail-fast contract.
//
// The decode error is wrapped with %w so callers can recover the underlying
// *toml.StrictMissingError via errors.As when the document defines a field the
// schema doesn't know about.
func Parse(r io.Reader) (*Lock, error) {
	var l Lock

	d := toml.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(&l); err != nil {
		return nil, fmt.Errorf("lockfile: decode: %w", err)
	}

	if l.Version != Version {
		return nil, fmt.Errorf("lockfile: unsupported version %d, want %d", l.Version, Version)
	}

	return &l, nil
}

// LoadFS opens name from fsys and parses it as a lock via Parse.
func LoadFS(fsys fs.FS, name string) (*Lock, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("lockfile: open %q: %w", name, err)
	}
	defer func() { _ = f.Close() }()

	return Parse(f)
}

// Resolve loads the lock to verify against, deriving its path from the resolved
// manifest so an override manifest resolves its own sibling lock unambiguously.
// The precedence mirrors manifest.Resolve: an explicit lockOverride wins, then
// the sibling dotf.lock of manifestPath, then of envManifest, and finally the
// embedded default at "manifest/dotf.lock" (callers pass dotfiles.ManifestFS).
//
// The two override kinds differ deliberately. A MISSING on-disk sibling lock is
// tolerated — a lock may legitimately not exist yet — and yields an empty
// &Lock{Version: Version} so install/apply proceed on inline checksums. An
// explicit lockOverride that is missing or malformed is a HARD error: like a
// manifest --manifest override it is user-supplied configuration, and silently
// falling back would mask a real misconfiguration.
func Resolve(lockOverride, manifestPath, envManifest string, embedded fs.FS) (*Lock, error) {
	switch {
	case lockOverride != "":
		return loadPath(lockOverride)
	case manifestPath != "":
		return loadSibling(manifestPath)
	case envManifest != "":
		return loadSibling(envManifest)
	}

	sub, err := fs.Sub(embedded, "manifest")
	if err != nil {
		return nil, fmt.Errorf("lockfile: embedded fs.Sub(manifest): %w", err)
	}

	return LoadFS(sub, "dotf.lock")
}

// loadSibling loads the dotf.lock sibling of manifestPath. A missing sibling is
// tolerated (returns an empty &Lock{Version: Version}); a present-but-malformed
// sibling is a hard error, so a corrupt committed lock never silently passes.
func loadSibling(manifestPath string) (*Lock, error) {
	lockPath := filepath.Join(filepath.Dir(manifestPath), "dotf.lock")
	l, err := loadPath(lockPath)
	if errors.Is(err, os.ErrNotExist) {
		return &Lock{Version: Version}, nil
	}
	return l, err
}

// loadPath opens and parses the lock file at path, used for the lockOverride
// and sibling branches of Resolve where the path is a filesystem path rather
// than an fs.FS-relative name.
func loadPath(path string) (*Lock, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("lockfile: open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	return Parse(f)
}
