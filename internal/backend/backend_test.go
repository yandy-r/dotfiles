package backend_test

import (
	"errors"
	"testing"

	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestEngine_UnknownBackend_ReturnsTypedError(t *testing.T) {
	t.Parallel()

	engine := backend.NewEngine(map[manifest.Backend]backend.Backend{})
	pkg := manifest.Package{
		Name:    "example",
		Backend: manifest.Backend("nonexistent"),
	}
	info := platform.Info{}

	if err := engine.Install(pkg, info); err != nil {
		var unknownErr *backend.UnknownBackendError
		if !errors.As(err, &unknownErr) {
			t.Fatalf("Install() error = %T, want *backend.UnknownBackendError", err)
		}
		if unknownErr.Backend != pkg.Backend {
			t.Errorf("UnknownBackendError.Backend = %q, want %q", unknownErr.Backend, pkg.Backend)
		}
	} else {
		t.Fatal("Install() error = nil, want *UnknownBackendError")
	}

	installed, err := engine.Installed(pkg, info)
	if err != nil {
		var unknownErr *backend.UnknownBackendError
		if !errors.As(err, &unknownErr) {
			t.Fatalf("Installed() error = %T, want *backend.UnknownBackendError", err)
		}
		if unknownErr.Backend != pkg.Backend {
			t.Errorf("UnknownBackendError.Backend = %q, want %q", unknownErr.Backend, pkg.Backend)
		}
	} else {
		t.Fatal("Installed() error = nil, want *UnknownBackendError")
	}
	if installed {
		t.Errorf("Installed() = true, want false when backend is unknown")
	}
}
