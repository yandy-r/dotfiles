package debtap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yandy-r/dotfiles/internal/backend"
)

// This file owns the INPUT side of the pipeline: turning the positional
// argument into a verified local .deb, and reading that .deb's own control
// metadata. Everything here runs before debtap is invoked.

// arMagic is the ar global header that opens every .deb.
const arMagic = "!<arch>\n"

// arNameSize is the width of the name field in an ar member header.
const arNameSize = 16

// debFirstMember is the required name of a .deb's first ar member. Its
// presence — not the file extension — is what makes a file a Debian package.
const debFirstMember = "debian-binary"

// httpGet is the HTTP fetch seam; tests replace it to aim at an httptest.Server.
// The default is a client with a total timeout.
var httpGet = httpClient.Get

// httpClient bounds the download, including the body read: the zero-value
// net/http client has no timeout, so a stalled mirror would hang the CLI.
var httpClient = &http.Client{Timeout: 10 * time.Minute}

// ErrNotDeb reports that a file is not a Debian package. Callers match it with
// errors.Is to distinguish a malformed input from an I/O failure.
var ErrNotDeb = errors.New("not a Debian package")

// ErrInsecureURL reports a plaintext http:// input. There is deliberately no
// override flag: the artifact is about to be installed as root, and no use case
// justifies taking it over a channel an attacker can rewrite.
var ErrInsecureURL = errors.New("refusing to download over plaintext http; use https")

// controlMemberRe matches the control tarball inside a .deb whatever its
// compression suffix (none, .gz, .xz, .zst), with or without the leading "./"
// that some producers emit.
var controlMemberRe = regexp.MustCompile(`^(\./)?control\.tar(\.[A-Za-z0-9]+)?$`)

// Control is the subset of a .deb's control file this package reads.
type Control struct {
	Package string
	Version string
}

// Acquire resolves opts.Input to a verified .deb inside the Converter's private
// working directory, returning that path, whether it came from a URL or the
// filesystem, and its hex SHA-256.
//
// The input is downloaded or COPIED into a private 0700 directory, and every
// later stage reads only that copy. This is what makes --sha256 a gate rather
// than a report. Hashing a path the caller does not control and then re-opening
// that same path for validation, control extraction and conversion would let
// anyone with write access to the input's directory swap the file after the
// digest was computed — and the swapped bytes are the ones `sudo pacman -U`
// ultimately runs as root. Binding every read to one private copy closes that
// window.
//
// Verification is ordered so nothing expensive happens on an input that was
// never going to be accepted: the digest's FORM is checked before any bytes are
// fetched, then the copy's structure, then its contents.
func (c *Converter) Acquire(opts Options) (path, source, sum string, err error) {
	if err := validateDigest(opts.SHA256); err != nil {
		return "", "", "", err
	}

	dir, err := c.session()
	if err != nil {
		return "", "", "", err
	}

	var h hash.Hash
	switch {
	case isInsecureURL(opts.Input):
		return "", "", "", ErrInsecureURL
	case isSecureURL(opts.Input):
		source = SourceURL
		path, h, err = c.download(opts.Input, dir)
	default:
		source = SourceFile
		path, h, err = copyIntoDir(opts.Input, dir)
	}
	if err != nil {
		return "", source, "", err
	}
	sum = hex.EncodeToString(h.Sum(nil))

	if err := validateDeb(path); err != nil {
		return path, source, sum, err
	}
	// VerifySHA256 treats an empty want as "skip", which is the intended
	// behavior here: --sha256 is optional and the digest is reported either
	// way. Reusing it keeps the constant-time compare in one place repo-wide.
	if err := backend.VerifySHA256(h, opts.SHA256); err != nil {
		return path, source, sum, fmt.Errorf("input checksum: %w", err)
	}
	return path, source, sum, nil
}

// validateDigest rejects a malformed --sha256 before any bytes are fetched, so
// a typo costs an error message rather than a full download.
func validateDigest(s string) error {
	if s == "" {
		return nil
	}
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != sha256.Size {
		return fmt.Errorf("--sha256: want %d hex characters, got %q", sha256.Size*2, s)
	}
	return nil
}

// session returns the Converter's private working directory, creating it on
// first use.
//
// The directory gets an unguessable name from os.MkdirTemp rather than a fixed
// one. The cache base can be a predictable path under a world-writable /tmp
// (when neither XDG_CACHE_HOME nor HOME is set), where os.MkdirAll would
// silently accept a directory — or a symlink to one — that an attacker
// pre-created. A fresh 0700 directory inside it is private regardless of what
// the base turned out to be, and the symlink check below rejects the most
// obvious redirection outright.
func (c *Converter) session() (string, error) {
	if c.sessionDir != "" {
		return c.sessionDir, nil
	}
	if err := os.MkdirAll(c.cacheDir, 0o700); err != nil {
		return "", fmt.Errorf("create cache dir %s: %w", c.cacheDir, err)
	}
	// Lstat, not Stat: a symlink base is exactly what this rejects.
	if fi, err := os.Lstat(c.cacheDir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("cache dir %s is a symlink; refusing to use it", c.cacheDir)
	}
	dir, err := os.MkdirTemp(c.cacheDir, "run-")
	if err != nil {
		return "", fmt.Errorf("create work dir: %w", err)
	}
	c.sessionDir = dir
	return dir, nil
}

// isSecureURL reports whether s is an https URL. The scheme test is
// case-insensitive but otherwise literal: anything that is not recognizably a
// URL is a filesystem path, so a file named "https-notes.deb" is never
// mistaken for one.
func isSecureURL(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), "https://")
}

// isInsecureURL reports whether s is a plaintext http URL.
func isInsecureURL(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), "http://")
}

// download streams url into a mode-0600 file in dir, teeing every byte through
// a SHA-256 hasher so the reported digest covers exactly what landed on disk.
func (c *Converter) download(url, dir string) (string, hash.Hash, error) {
	resp, err := httpGet(url)
	if err != nil {
		// the client error already names the URL, so it is not repeated.
		return "", nil, fmt.Errorf("download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}

	f, err := os.CreateTemp(dir, "download-*.deb")
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(f, io.TeeReader(resp.Body, h)); err != nil {
		_ = os.Remove(f.Name())
		return "", nil, fmt.Errorf("download %s: %w", url, err)
	}
	return f.Name(), h, nil
}

// copyIntoDir copies src into dir under its base name, hashing every byte as it
// is written so the digest describes the private copy that every later stage
// reads — never the caller's path, which may change underneath us.
func copyIntoDir(src, dir string) (string, hash.Hash, error) {
	in, err := os.Open(src)
	if err != nil {
		// os.Open's error already names the path; repeating it produced
		// "open X: open X: no such file or directory".
		return "", nil, err
	}
	defer func() { _ = in.Close() }()

	name := filepath.Base(src)
	if name == "." || name == string(filepath.Separator) || name == ".." {
		name = "input.deb"
	}
	dst := filepath.Join(dir, name)

	// O_EXCL: dir was just created by MkdirTemp, so nothing should be here.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", nil, fmt.Errorf("stage input: %w", err)
	}

	h := sha256.New()
	if _, err := io.Copy(out, io.TeeReader(in, h)); err != nil {
		_ = out.Close()
		return "", nil, fmt.Errorf("read %s: %w", src, err)
	}
	if err := out.Close(); err != nil {
		return "", nil, fmt.Errorf("stage input: %w", err)
	}
	return dst, h, nil
}

// validateDeb reports whether path is structurally a Debian package: the ar
// global header at offset 0, followed immediately by a member whose 16-byte
// name field is "debian-binary".
//
// The extension is deliberately ignored, matching how
// internal/backend/download.go classifies artifacts — a mislabeled or
// extensionless file is judged by its contents, and an HTML error page saved as
// "app.deb" is rejected here rather than confusing debtap later.
func validateDeb(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var head [len(arMagic) + arNameSize]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return fmt.Errorf("%w: %s is too short to be an ar archive", ErrNotDeb, filepath.Base(path))
	}
	if string(head[:len(arMagic)]) != arMagic {
		return fmt.Errorf("%w: %s has no ar archive header", ErrNotDeb, filepath.Base(path))
	}
	// ar pads the name field with spaces; GNU ar also terminates it with '/'.
	name := strings.TrimRight(string(head[len(arMagic):]), " /")
	if name != debFirstMember {
		return fmt.Errorf("%w: %s begins with member %q, want %q",
			ErrNotDeb, filepath.Base(path), name, debFirstMember)
	}
	return nil
}

// Control reads the .deb's declared package name and version.
//
// Go's standard library cannot decompress control.tar.zst or control.tar.xz,
// which is what current .deb producers emit, so extraction goes through bsdtar.
// bsdtar ships with libarchive, a hard dependency of pacman itself, so it is
// guaranteed present wherever this command is applicable — which is why this
// costs no new Go module.
func (c *Converter) Control(debPath string) (Control, error) {
	work, err := os.MkdirTemp("", "dotf-deb-control-")
	if err != nil {
		return Control{}, fmt.Errorf("create work dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }()

	// Listing before extracting avoids guessing the compression suffix and
	// doubles as a structural check of the archive.
	out, err := c.r.Output("bsdtar", "-tf", debPath)
	if err != nil {
		return Control{}, fmt.Errorf("list %s: %w", filepath.Base(debPath), err)
	}

	member := ""
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); controlMemberRe.MatchString(line) {
			member = line
			break
		}
	}
	if member == "" {
		return Control{}, fmt.Errorf("%w: %s contains no control archive",
			ErrNotDeb, filepath.Base(debPath))
	}

	if err := c.r.Run("bsdtar", "-xf", debPath, "-C", work, member); err != nil {
		return Control{}, fmt.Errorf("extract %s: %w", member, err)
	}
	// bsdtar strips a leading "./" when extracting, so the member always lands
	// under its base name.
	inner := filepath.Join(work, filepath.Base(member))
	if err := c.r.Run("bsdtar", "-xf", inner, "-C", work); err != nil {
		return Control{}, fmt.Errorf("extract %s: %w", filepath.Base(member), err)
	}

	ctlPath := filepath.Join(work, "control")
	// The control archive is attacker-supplied and may legitimately contain
	// "control" as a SYMLINK. Reading it would follow the link out of the work
	// directory and surface a line of an unrelated readable file as the
	// package's name or version.
	fi, err := os.Lstat(ctlPath)
	if err != nil {
		return Control{}, fmt.Errorf("read control file: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return Control{}, fmt.Errorf("%w: %s has a non-regular control entry",
			ErrNotDeb, filepath.Base(debPath))
	}

	data, err := os.ReadFile(ctlPath)
	if err != nil {
		return Control{}, fmt.Errorf("read control file: %w", err)
	}
	return parseControl(string(data)), nil
}

// parseControl reads the Package and Version fields from a Debian control
// file. Continuation lines (which begin with a space or tab) belong to the
// preceding field and are skipped: neither field read here is ever multi-line,
// and skipping them prevents an indented "Package:" inside a long Description
// from being mistaken for the real field.
func parseControl(s string) Control {
	var ctl Control
	for _, line := range strings.Split(s, "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "package":
			ctl.Package = strings.TrimSpace(val)
		case "version":
			ctl.Version = strings.TrimSpace(val)
		}
	}
	return ctl
}
