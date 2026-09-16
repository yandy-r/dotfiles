package plasma

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yandy-r/dotfiles/internal/exec"
)

// NOTE: no t.Parallel() anywhere in this package. See codec_test.go — the
// t.Parallel + package-global-stub combination produces a -race failure that is
// green locally and red on CI, and the rule is cheaper to keep than to audit.

// testEngine builds an Engine over two fresh temp dirs and shrinks the lock
// timeout so a contention test finishes in milliseconds rather than 45s. The
// system colour-scheme directory is pointed at a temp path so the §7.4 guard
// never reads the host's /usr/share.
func testEngine(t *testing.T) (*Engine, string, string) {
	t.Helper()
	repoRoot := t.TempDir()
	home := t.TempDir()
	configDir := filepath.Join(home, ".config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", configDir, err)
	}

	e := NewEngine(repoRoot, configDir, "", []string{filepath.Join(t.TempDir(), "absent")}, exec.NewFake())
	e.lockTimeout = 150 * time.Millisecond
	e.lockPoll = 10 * time.Millisecond
	return e, repoRoot, configDir
}

// writeFile writes data at path, creating parents. Mode is 0644 so a test that
// asserts mode preservation has a value distinguishable from the 0600 default.
func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

// seedRepo writes a file into plasma/files/<rel>.
func seedRepo(t *testing.T, repoRoot, rel, data string) string {
	t.Helper()
	path := filepath.Join(repoRoot, repoFilesRel, filepath.FromSlash(rel))
	writeFile(t, path, data)
	return path
}

// seedRecommended writes a file into plasma/recommended/<rel>.
func seedRecommended(t *testing.T, repoRoot, rel, data string) string {
	t.Helper()
	path := filepath.Join(repoRoot, repoRecommendedRel, filepath.FromSlash(rel))
	writeFile(t, path, data)
	return path
}

// seedLive writes a file into the KDE config dir.
func seedLive(t *testing.T, configDir, rel, data string) string {
	t.Helper()
	path := filepath.Join(configDir, filepath.FromSlash(rel))
	writeFile(t, path, data)
	return path
}

// readFile reads a file or fails.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}

// planFileFor returns the plan's action for a managed path.
func planFileFor(t *testing.T, p *Plan, path string) (FileAction, bool) {
	t.Helper()
	for _, act := range p.Files {
		if act.Spec.Path == path && act.Kind != ActionBackup {
			return act, true
		}
	}
	return FileAction{}, false
}

// countKind counts actions of a kind in a plan.
func countKind(p *Plan, kind ActionKind) int {
	n := 0
	for _, act := range p.Files {
		if act.Kind == kind {
			n++
		}
	}
	return n
}

// specFor returns the DefaultFiles entry for a path, failing when absent.
func specFor(t *testing.T, path string) FileSpec {
	t.Helper()
	for _, spec := range DefaultFiles() {
		if spec.Path == path {
			return spec
		}
	}
	t.Fatalf("DefaultFiles has no entry for %q", path)
	return FileSpec{}
}

// countEntries totals the entries in a document, ignoring group structure.
func countEntries(doc *Doc) int {
	n := 0
	for i := range doc.Groups {
		n += len(doc.Groups[i].Entries)
	}
	return n
}
