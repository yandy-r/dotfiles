package backend

import (
	"fmt"
	"path/filepath"
	"strings"
)

// containedJoin joins rel onto root and returns the cleaned path, rejecting any
// rel that is empty, absolute, or escapes root via ".." segments. It is the
// single path-containment primitive shared by the git-clone backend (for both
// the clone dest and each post_install exec), mirroring the reject-absolute /
// reject-".." / filepath.Rel technique used by custom_script.go's resolveScript
// and download.go's safeExtractPath. A rejected path yields a typed error rather
// than a silently clamped join.
func containedJoin(root, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("path is empty")
	}

	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("reject absolute path %q", rel)
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("reject path traversal %q", rel)
	}

	target := filepath.Clean(filepath.Join(root, clean))
	relToRoot, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", rel, err)
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes %q", rel, root)
	}

	return target, nil
}
