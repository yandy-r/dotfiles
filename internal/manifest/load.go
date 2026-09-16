package manifest

import (
	"fmt"
	"io"
	"io/fs"
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

// decode reads a TOML-encoded manifest from r with strict unknown-field
// checking and returns it WITHOUT validating. It is the shared decode half of
// Parse (Parse = decode + Validate) and is used directly by DiscoverOverlays:
// an overlay fragment's intent→package cross-references resolve only after the
// merge, so each fragment is decoded but validated only as part of the merged
// whole — while strict unknown-field checking still guards every fragment.
//
// The decode error is wrapped with %w so callers can recover the underlying
// *toml.StrictMissingError via errors.As when the document defines a field the
// schema doesn't know about.
func decode(r io.Reader) (*Manifest, error) {
	var m Manifest

	d := toml.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return nil, fmt.Errorf("manifest: decode: %w", err)
	}

	return &m, nil
}

// Parse decodes a TOML-encoded manifest from r with strict unknown-field
// checking, then validates its contents via Validate. It returns (nil, err)
// on ANY failure — decode or validation — so callers never receive a
// half-populated *Manifest.
func Parse(r io.Reader) (*Manifest, error) {
	m, err := decode(r)
	if err != nil {
		return nil, err
	}

	if err := Validate(m); err != nil {
		return nil, fmt.Errorf("manifest: validate: %w", err)
	}

	return m, nil
}

// LoadFS opens name from fsys and parses it as a manifest via Parse.
func LoadFS(fsys fs.FS, name string) (*Manifest, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("manifest: open %q: %w", name, err)
	}
	defer func() { _ = f.Close() }()

	return Parse(f)
}

// ResolveOptions controls the XDG user-config overlay that Resolve layers on top
// of the base manifest. Its zero value disables the overlay (empty ConfigRoot),
// so a caller passing ResolveOptions{} — or ResolveOptions{NoUserConfig: true} —
// gets exactly the pre-overlay base resolution, byte-for-byte.
type ResolveOptions struct {
	// NoUserConfig disables the overlay outright (the --no-user-config flag /
	// DOTF_NO_USER_CONFIG env twin), pinning resolution to the base manifest for
	// pristine/CI/reproducible runs.
	NoUserConfig bool

	// ConfigRoot is the user config root (${XDG_CONFIG_HOME:-~/.config}/dotf)
	// whose packages.d/*.toml fragments overlay the base manifest. An empty
	// ConfigRoot skips the overlay (e.g. when the home dir cannot be resolved).
	ConfigRoot string

	// Logf, when non-nil, receives one formatted line per package/intent
	// Collision the merge resolves, so an overlay override is announced rather
	// than applied silently. It is injected (a stderr logger in production) to
	// keep this package free of I/O.
	Logf func(string, ...any)
}

// Resolve loads the manifest to use: the base manifest (flag > $DOTF_MANIFEST >
// embedded precedence) optionally overlaid with the user's XDG config fragments.
//
// The base precedence — flagPath (from a CLI flag) wins if set, then envVal (the
// value of $DOTF_MANIFEST), and only then the embedded default under
// "manifest/packages.toml" (callers pass dotfiles.ManifestFS) — and its
// fail-fast contract are UNCHANGED from before the overlay existed. That
// precedence deliberately diverges from internal/platform's detection
// philosophy. platform.Detect degrades gracefully to "unknown" on failure
// because it probes an untrusted, unowned host environment — graceful
// degradation is the only sane behavior there. A manifest override (flagPath or
// envVal), by contrast, is user-supplied configuration: an explicit request to
// load one specific file. Silently falling back to the embedded default when
// that file can't be read or fails validation would mask a real
// misconfiguration (a typo'd path, a stale $DOTF_MANIFEST, a malformed TOML
// edit) behind a provisioning run that looks like it worked. Resolve therefore
// fails fast on any explicit override.
//
// After the base loads, the overlay is applied unless opts disables it: when
// opts.NoUserConfig is set or opts.ConfigRoot is empty, the base is returned
// unchanged (this no-overlay path is byte-for-byte identical to the pre-overlay
// behavior). Otherwise DiscoverOverlays reads ConfigRoot/packages.d/*.toml; zero
// fragments returns the base, and one or more are Merge'd onto it (validated as a
// whole), with each resolved Collision emitted via opts.Logf so overlay
// overrides are never silent.
func Resolve(flagPath, envVal string, embedded fs.FS, opts ResolveOptions) (*Manifest, error) {
	base, err := resolveBase(flagPath, envVal, embedded)
	if err != nil {
		return nil, err
	}

	if opts.NoUserConfig || opts.ConfigRoot == "" {
		return base, nil
	}

	overlays, err := DiscoverOverlays(opts.ConfigRoot)
	if err != nil {
		return nil, err
	}
	if len(overlays) == 0 {
		return base, nil
	}

	merged, collisions, err := Merge(base, overlays...)
	if err != nil {
		return nil, err
	}
	if opts.Logf != nil {
		for _, c := range collisions {
			opts.Logf("overlay %s overrides %s %q (shadowed %s)", c.Won, c.Kind, c.Key, c.Shadowed)
		}
	}
	return merged, nil
}

// resolveBase loads the base manifest honoring the flag > $DOTF_MANIFEST >
// embedded precedence and its fail-fast override contract (see Resolve's doc
// comment for the rationale). It is the base half of Resolve, factored out so
// the overlay layering reads cleanly on top of it.
func resolveBase(flagPath, envVal string, embedded fs.FS) (*Manifest, error) {
	switch {
	case flagPath != "":
		return loadPath(flagPath)
	case envVal != "":
		return loadPath(envVal)
	}

	sub, err := fs.Sub(embedded, "manifest")
	if err != nil {
		return nil, fmt.Errorf("manifest: embedded fs.Sub(manifest): %w", err)
	}

	return LoadFS(sub, "packages.toml")
}

// loadPath opens and parses the manifest file at path, used for the
// flag/env override branches of Resolve where the path is an absolute or
// relative filesystem path rather than an fs.FS-relative name.
func loadPath(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("manifest: open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	return Parse(f)
}
