package backend

import (
	"errors"
	"github.com/yandy-r/dotfiles/internal/privilege/privilegetest"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestAction_String(t *testing.T) {
	t.Parallel()

	// The zero value is ActionInstall so callers that never mention an action
	// default to install.
	if Action(0) != ActionInstall {
		t.Errorf("zero Action = %d, want ActionInstall", Action(0))
	}

	cases := map[Action]string{
		ActionInstall: "install",
		ActionUpdate:  "update",
		ActionRemove:  "remove",
	}
	for action, want := range cases {
		if got := action.String(); got != want {
			t.Errorf("Action(%d).String() = %q, want %q", action, got, want)
		}
	}
}

// testLifecycleEngine builds the full Default engine over a fake runner.
func testLifecycleEngine(r exec.Runner) *Engine {
	return Default(r, privilegetest.NewFakeDropper(), "", nil)
}

// TestEngine_Update_UnsupportedBackend asserts backends without an Updater yield
// a typed ActionUnsupportedError rather than a silent no-op.
func TestEngine_Update_UnsupportedBackend(t *testing.T) {
	t.Parallel()

	eng := testLifecycleEngine(exec.NewFake())
	info := platform.Info{OS: platform.OSArch, PkgMgr: platform.PkgMgrPacman}

	// script-url is deliberately absent: re-running its vendor installer is the
	// update (TestScriptURL_UpdateRerunsVerifiedInstaller).
	backends := []manifest.Backend{
		manifest.BackendGitClone,
	}
	for _, b := range backends {
		t.Run(string(b), func(t *testing.T) {
			pkg := manifest.Package{Name: "x", Backend: b}
			err := eng.Update(pkg, info)
			var ue *ActionUnsupportedError
			if !errors.As(err, &ue) {
				t.Fatalf("Update(%s) error = %v, want *ActionUnsupportedError", b, err)
			}
			if ue.Action != ActionUpdate || ue.Backend != b || ue.Package != "x" {
				t.Errorf("ActionUnsupportedError = %+v, want {update %s x}", ue, b)
			}
		})
	}
}

// TestEngine_Remove_UnsupportedBackend is the symmetric guard for Remove.
func TestEngine_Remove_UnsupportedBackend(t *testing.T) {
	t.Parallel()

	eng := testLifecycleEngine(exec.NewFake())
	info := platform.Info{OS: platform.OSArch, PkgMgr: platform.PkgMgrPacman}

	backends := []manifest.Backend{
		manifest.BackendScriptURL,
		manifest.BackendGitHubRelease,
		manifest.BackendGitClone,
	}
	for _, b := range backends {
		t.Run(string(b), func(t *testing.T) {
			pkg := manifest.Package{Name: "x", Backend: b}
			err := eng.Remove(pkg, info)
			var ue *ActionUnsupportedError
			if !errors.As(err, &ue) {
				t.Fatalf("Remove(%s) error = %v, want *ActionUnsupportedError", b, err)
			}
			if ue.Action != ActionRemove || ue.Backend != b || ue.Package != "x" {
				t.Errorf("ActionUnsupportedError = %+v, want {remove %s x}", ue, b)
			}
		})
	}
}

// TestEngine_Update_UnknownBackend covers the unregistered-backend path.
func TestEngine_Update_UnknownBackend(t *testing.T) {
	t.Parallel()

	eng := testLifecycleEngine(exec.NewFake())
	err := eng.Update(manifest.Package{Name: "y", Backend: manifest.Backend("bogus")}, platform.Info{})
	var ub *UnknownBackendError
	if !errors.As(err, &ub) {
		t.Fatalf("Update(bogus) error = %v, want *UnknownBackendError", err)
	}
	err = eng.Remove(manifest.Package{Name: "y", Backend: manifest.Backend("bogus")}, platform.Info{})
	if !errors.As(err, &ub) {
		t.Fatalf("Remove(bogus) error = %v, want *UnknownBackendError", err)
	}
}

// TestEngine_Update_OffPlatformNoop asserts an off-platform package is a no-op
// (nil, no command), uniform with Install's OS gate.
func TestEngine_Update_OffPlatformNoop(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	eng := testLifecycleEngine(fake)

	// A macOS-gated pkg on an Arch host: the gate short-circuits before dispatch.
	pkg := manifest.Package{
		Name:    "mac-tool",
		Backend: manifest.BackendPkg,
		OS:      "macos",
		Names:   map[string]string{string(platform.PkgMgrHomebrew): "mac-tool"},
	}
	info := platform.Info{OS: platform.OSArch, PkgMgr: platform.PkgMgrPacman}

	if err := eng.Update(pkg, info); err != nil {
		t.Errorf("Update(off-platform) error = %v, want nil", err)
	}
	if err := eng.Remove(pkg, info); err != nil {
		t.Errorf("Remove(off-platform) error = %v, want nil", err)
	}
	if calls := fake.CallsSnapshot(); len(calls) != 0 {
		t.Errorf("off-platform Update/Remove ran commands %+v, want none", calls)
	}
}

// TestEngine_Perform_RoutesByAction asserts Perform dispatches Install/Update/
// Remove to the matching engine method. DNF is used because its three verbs
// (install/upgrade/remove) are distinct, unlike pacman where install == upgrade.
func TestEngine_Perform_RoutesByAction(t *testing.T) {
	t.Parallel()

	pkg := manifest.Package{
		Name:    "ripgrep",
		Backend: manifest.BackendPkg,
		Names:   map[string]string{string(platform.PkgMgrDNF): "ripgrep"},
	}
	info := platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrDNF}

	cases := []struct {
		action Action
		want   []string
	}{
		{ActionInstall, []string{"dnf", "install", "-y", "ripgrep"}},
		{ActionUpdate, []string{"dnf", "upgrade", "-y", "ripgrep"}},
		{ActionRemove, []string{"dnf", "remove", "-y", "ripgrep"}},
	}
	for _, tc := range cases {
		t.Run(tc.action.String(), func(t *testing.T) {
			fake := exec.NewFake()
			eng := testLifecycleEngine(fake)

			if err := eng.Perform(tc.action, pkg, info); err != nil {
				t.Fatalf("Perform(%s) error = %v, want nil", tc.action, err)
			}
			calls := fake.CallsSnapshot()
			if len(calls) != 1 {
				t.Fatalf("Perform(%s) made %d calls, want 1", tc.action, len(calls))
			}
			if calls[0].Name != "sudo" {
				t.Errorf("Perform(%s) call name = %q, want sudo", tc.action, calls[0].Name)
			}
			if !stringSliceEqual(calls[0].Args, tc.want) {
				t.Errorf("Perform(%s) args = %v, want %v", tc.action, calls[0].Args, tc.want)
			}
		})
	}
}

// TestEngine_RepoAUR_ImplementLifecycle: repo and aur now
// satisfy the optional lifecycle interfaces (no ActionUnsupportedError).
func TestEngine_RepoAUR_ImplementLifecycle(t *testing.T) {
	t.Parallel()

	eng := testLifecycleEngine(exec.NewFake())
	for _, b := range []manifest.Backend{manifest.BackendRepo, manifest.BackendRepo, manifest.BackendAUR} {
		be := eng.backends[b]
		if _, ok := be.(Updater); !ok {
			t.Errorf("%s: not an Updater", b)
		}
		if _, ok := be.(Remover); !ok {
			t.Errorf("%s: not a Remover", b)
		}
	}
}
