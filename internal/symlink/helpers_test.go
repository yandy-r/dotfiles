package symlink

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates a regular file at path (creating parents) with the given
// bytes, failing the test on error.
func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

// mkSymlink creates a symlink at link pointing at target (creating parents).
func mkSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(link), err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink(%q, %q) error = %v", target, link, err)
	}
}

// isSymlink reports whether path is a symlink, failing the test if it cannot
// be lstat'd.
func isSymlink(t *testing.T, path string) bool {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat(%q) error = %v", path, err)
	}
	return fi.Mode()&os.ModeSymlink != 0
}

// buildFullSourceTree materializes every source in DefaultTable under repoRoot,
// so that Plan sees all sources present. Directory entries get a marker file.
func buildFullSourceTree(t *testing.T, repoRoot string) {
	t.Helper()
	for _, spec := range DefaultTable() {
		src := filepath.Join(repoRoot, spec.Src)
		if spec.IsDir {
			if err := os.MkdirAll(src, 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) error = %v", src, err)
			}
			writeFile(t, filepath.Join(src, "marker"), []byte("marker"))
			continue
		}
		writeFile(t, src, []byte("content of "+spec.Src))
	}
}

// createdDests returns the set of $HOME-relative dests scheduled for an
// ActionLink or ActionCopy in the plan.
func createdDests(p *Plan) map[string]bool {
	out := make(map[string]bool)
	for _, a := range p.Actions {
		if a.Kind == ActionLink || a.Kind == ActionCopy {
			out[a.Spec.Dest] = true
		}
	}
	return out
}

// countKind returns how many of plan's actions have the given kind.
func countKind(p *Plan, kind ActionKind) int {
	n := 0
	for _, a := range p.Actions {
		if a.Kind == kind {
			n++
		}
	}
	return n
}

// planIndexOf returns the position of the first action in p matching dest and
// kind, so phase ordering (backup before remove) is assertable.
func planIndexOf(p *Plan, dest string, kind ActionKind) (int, bool) {
	for i, a := range p.Actions {
		if a.Spec.Dest == dest && a.Kind == kind {
			return i, true
		}
	}
	return 0, false
}
