package sysconfig

import (
	"path/filepath"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// etcShellsHasPath reports whether path is listed on its own line in an
// /etc/shells file body.
func etcShellsHasPath(content []byte, path string) bool {
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == path {
			return true
		}
	}
	return false
}

// renderEtcShells returns content with path appended on its own line when it
// is absent; when already present, content is returned unchanged. A trailing
// newline is ensured before appending.
func renderEtcShells(content []byte, path string) []byte {
	if etcShellsHasPath(content, path) {
		return content
	}

	s := string(content)
	if len(s) > 0 && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	s += path + "\n"
	return []byte(s)
}

// resolveShellPath resolves a shell name or path to an absolute shell path. An
// absolute name is used only when it resolves to an existing executable (via
// r.LookPath, which existence-checks a path with a slash); a mistyped or
// missing absolute path is NOT blindly returned, since making it the login
// shell risks locking the user out. A bare name is resolved via r.LookPath,
// then via a per-OS candidate list (matching bootstrap.sh's zsh candidates).
// It returns "" (and nil error) when no shell can be found, so the caller can
// warn and skip rather than fail.
func resolveShellPath(r exec.Runner, name string, info platform.Info) (string, error) {
	if name == "" {
		return "", nil
	}

	if filepath.IsAbs(name) {
		if p, err := r.LookPath(name); err == nil {
			return p, nil
		}
		// Absolute path does not exist as an executable — skip rather than
		// return a login shell that would lock the user out.
		return "", nil
	}

	if p, err := r.LookPath(name); err == nil {
		return p, nil
	}

	for _, candidate := range shellCandidates(name, info) {
		if _, err := r.LookPath(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", nil
}

// shellCandidates returns the per-OS absolute-path candidate list for a shell
// name, ported from bootstrap.sh:1690-1695.
func shellCandidates(name string, info platform.Info) []string {
	if info.OS == platform.OSMacOS {
		return []string{"/opt/homebrew/bin/" + name, "/usr/bin/" + name}
	}
	return []string{
		"/home/linuxbrew/.linuxbrew/bin/" + name,
		"/usr/bin/" + name,
		"/bin/" + name,
	}
}
