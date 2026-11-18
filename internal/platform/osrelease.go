package platform

import (
	"bufio"
	"os"
	"strings"
)

// defaultOSReleasePath is the standard location of the os-release file on
// Linux systems.
const defaultOSReleasePath = "/etc/os-release"

// scanOSRelease reads an os-release file
// (https://www.freedesktop.org/software/systemd/man/os-release.html) at path
// into a KEY->value map, skipping blank and comment lines and unquoting each
// value.
//
// It returns an empty map when the file is missing, unreadable, or a read
// error interrupts the scan — callers are responsible for applying the
// "unknown" fallback, matching the shell's graceful degradation. scanOSRelease
// never panics.
func scanOSRelease(path string) map[string]string {
	fields := make(map[string]string)

	f, err := os.Open(path)
	if err != nil {
		return fields
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(key)] = unquote(strings.TrimSpace(value))
	}

	if scanner.Err() != nil {
		// A line exceeding bufio.Scanner's default token limit (or another
		// read error) stops Scan() early. Degrade gracefully to an empty map
		// (the same "unknown" fallback as a missing/unreadable file) rather
		// than silently returning partial results.
		return map[string]string{}
	}

	return fields
}

// osRelease is the parsed subset of an os-release file dotf consumes: the
// distro family IDs, the Debian/Ubuntu codename, and the release version. It
// exists so Detect scans /etc/os-release exactly ONCE per run instead of once
// per consumed field.
type osRelease struct {
	// Present reports whether the file existed at scan time, mirroring the
	// shell's `[[ -f /etc/os-release ]]` test. It is what distinguishes a
	// missing file (WSL/generic-linux fallback) from a present-but-empty one.
	Present bool
	// ID is the os-release ID field (e.g. "fedora", "ubuntu").
	ID string
	// IDLike is the os-release ID_LIKE field (e.g. "fedora rhel").
	IDLike string
	// Codename is VERSION_CODENAME, falling back to UBUNTU_CODENAME.
	Codename string
	// VersionID is the raw VERSION_ID field (e.g. "42", "24.04"). Empty on
	// rolling distros that set none.
	VersionID string
}

// readOSRelease scans the os-release file at path once and returns every field
// dotf consumes. It degrades to the zero value (Present false, empty fields)
// when the file is missing or unreadable; it never panics.
func readOSRelease(path string) osRelease {
	fields := scanOSRelease(path)
	rel := osRelease{
		Present:   fileExists(path),
		ID:        fields["ID"],
		IDLike:    fields["ID_LIKE"],
		Codename:  fields["VERSION_CODENAME"],
		VersionID: fields["VERSION_ID"],
	}
	if rel.Codename == "" {
		rel.Codename = fields["UBUNTU_CODENAME"]
	}
	return rel
}

// distro returns the raw ID, or "unknown" when os-release defines none,
// mirroring common/platform.sh:get_distro_name.
func (o osRelease) distro() string {
	if o.ID == "" {
		return "unknown"
	}
	return o.ID
}

// unquote strips a single pair of surrounding double or single quotes from
// an os-release value, mirroring shell word-splitting behavior when the
// file is sourced (e.g. ID="ubuntu" becomes ubuntu).
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
