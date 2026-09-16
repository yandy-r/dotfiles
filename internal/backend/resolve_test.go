package backend

import (
	"errors"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/privilege"
)

// TestEngineResolve_NotLockable asserts a Pinnable-but-non-download backend (a
// pinned custom-script) yields a typed 'not lockable' error from Engine.Resolve,
// so `dotf lock` records it as a failure rather than silently skipping it.
func TestEngineResolve_NotLockable(t *testing.T) {
	eng := Default(exec.NewFake(), privilege.NewSudo(), privilege.NewFakeDropper(), "", nil)

	_, err := eng.Resolve(manifest.Package{
		Name:    "weird",
		Backend: manifest.BackendCustomScript,
		Class:   manifest.ClassPinned,
	}, platform.Info{})
	if err == nil || !strings.Contains(err.Error(), "not lockable") {
		t.Fatalf("Resolve(pinned custom-script) error = %v, want a 'not lockable' error", err)
	}
}

// TestEngineResolve_UnknownBackend asserts Engine.Resolve returns an
// UnknownBackendError for a backend with no registered handler.
func TestEngineResolve_UnknownBackend(t *testing.T) {
	eng := Default(exec.NewFake(), privilege.NewSudo(), privilege.NewFakeDropper(), "", nil)

	_, err := eng.Resolve(manifest.Package{Name: "y", Backend: manifest.Backend("bogus")}, platform.Info{})
	var ub *UnknownBackendError
	if !errors.As(err, &ub) {
		t.Fatalf("Resolve(bogus backend) error = %v, want *UnknownBackendError", err)
	}
}
