package backend

import (
	"os"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
)

// TestRepackWinBoat performs an unprivileged real rebuild and metadata/payload
// gate when explicitly supplied a checksum-verified vendor artifact.
func TestRepackWinBoat(t *testing.T) {
	path := os.Getenv("DOTF_TEST_WINBOAT_RPM")
	if path == "" {
		t.Skip("set DOTF_TEST_WINBOAT_RPM to verified vendor RPM")
	}
	out, err := repackStripBuildID(exec.New(), manifest.Package{PkgName: "winboat"}, path, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}
