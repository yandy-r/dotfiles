package backend_test

import (
	"errors"
	"github.com/yandy-r/dotfiles/internal/privilege/privilegetest"
	"testing"

	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestDefault_CoversAllValidBackends(t *testing.T) {
	engine := backend.Default(
		exec.NewFake(),
		privilegetest.NewFakeDropper(),
		t.TempDir(),
		nil,
	)

	validBackends := []manifest.Backend{
		manifest.BackendPkg,
		manifest.BackendGitHubRelease,
		manifest.BackendAUR,
		manifest.BackendToolchain,
		manifest.BackendAPTRepo,
		manifest.BackendRepo,
		manifest.BackendScriptURL,
		manifest.BackendCustomScript,
		manifest.BackendGitClone,
		manifest.BackendFont,
	}

	info := platform.Info{}

	for _, b := range validBackends {
		if !b.Valid() {
			t.Fatalf("fixture backend %q is not Valid()", b)
		}

		pkg := manifest.Package{Name: "test", Backend: b}
		_, err := engine.Installed(pkg, info)
		var unknown *backend.UnknownBackendError
		if errors.As(err, &unknown) {
			t.Errorf("Default() missing registration for backend %q", b)
		}
	}
}
