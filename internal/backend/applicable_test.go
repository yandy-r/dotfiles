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

func TestEngine_Applicable(t *testing.T) {
	t.Parallel()

	engine := backend.Default(exec.NewFake(), privilegetest.NewFakeDropper(), t.TempDir(), nil)

	archPacman := platform.Info{OS: platform.OSArch, PkgMgr: platform.PkgMgrPacman}
	debianAPT := platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT}
	fedoraDNF := platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrDNF}
	macOS := platform.Info{OS: platform.OSMacOS, PkgMgr: platform.PkgMgrHomebrew}
	windows := platform.Info{OS: platform.OSWindows}

	tests := []struct {
		name    string
		backend manifest.Backend
		os      string            // package-level os gate ("" = no gate)
		names   map[string]string // pkg-backend name map (nil for backends that are not package-gated)
		aptRepo bool              // attach an apt_repo sub-table
		dnfRepo bool              // attach a dnf_repo sub-table
		info    platform.Info
		want    bool
	}{
		// The repo backend gates on the PACKAGE too: an entry is applicable only
		// where the host's package manager has a matching sub-table.
		{"apt-repo alias with apt_repo on pacman host", manifest.BackendAPTRepo, "", nil, true, false, archPacman, false},
		{"apt-repo alias with apt_repo on apt host", manifest.BackendAPTRepo, "", nil, true, false, debianAPT, true},
		{"repo with apt_repo on apt host", manifest.BackendRepo, "", nil, true, false, debianAPT, true},
		{"repo with apt_repo only on dnf host", manifest.BackendRepo, "", nil, true, false, fedoraDNF, false},
		{"repo with dnf_repo on dnf host", manifest.BackendRepo, "", nil, false, true, fedoraDNF, true},
		{"aur on debian host", manifest.BackendAUR, "", nil, false, false, debianAPT, false},
		{"aur on arch host", manifest.BackendAUR, "", nil, false, false, archPacman, true},
		{"pkg on pacman host", manifest.BackendPkg, "", map[string]string{"pacman": "example"}, false, false, archPacman, true},
		{"pkg on apt host", manifest.BackendPkg, "", map[string]string{"apt": "example"}, false, false, debianAPT, true},
		// ADR-2: a pkg entry with no name for the HOST's package manager is a clean
		// not-applicable, not a MissingPkgNameError that aborts the whole
		// status/diff run. Pins the exact seam that changed.
		{"pkg with no name for host manager", manifest.BackendPkg, "", map[string]string{"pacman": "example"}, false, false, debianAPT, false},
		{"github-release on any host", manifest.BackendGitHubRelease, "", nil, false, false, debianAPT, true},
		{"script-url on any host", manifest.BackendScriptURL, "", nil, false, false, archPacman, true},
		{"custom-script on any host", manifest.BackendCustomScript, "", nil, false, false, archPacman, true},
		{"toolchain on any host", manifest.BackendToolchain, "", nil, false, false, debianAPT, true},
		{"git-clone on any host", manifest.BackendGitClone, "", nil, false, false, debianAPT, true},
		// os gate (#82): scoped independently of the backend's own platform gate.
		{"os=macos on debian host", manifest.BackendCustomScript, "macos", nil, false, false, debianAPT, false},
		{"os=macos on macos host", manifest.BackendCustomScript, "macos", nil, false, false, macOS, true},
		{"os=linux on debian host", manifest.BackendCustomScript, "linux", nil, false, false, debianAPT, true},
		{"os=linux on arch host", manifest.BackendCustomScript, "linux", nil, false, false, archPacman, true},
		{"os=linux on macos host", manifest.BackendCustomScript, "linux", nil, false, false, macOS, false},
		{"os=debian on debian host", manifest.BackendCustomScript, "debian", nil, false, false, debianAPT, true},
		{"os=debian on arch host", manifest.BackendCustomScript, "debian", nil, false, false, archPacman, false},
		{"os=windows on windows host", manifest.BackendCustomScript, "windows", nil, false, false, windows, true},
		{"os=windows on linux host", manifest.BackendCustomScript, "windows", nil, false, false, debianAPT, false},
		{"empty os gate on any host", manifest.BackendCustomScript, "", nil, false, false, macOS, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pkg := manifest.Package{Name: "example", Backend: tt.backend, OS: tt.os, Names: tt.names}
			if tt.aptRepo {
				pkg.AptRepo = &manifest.AptRepo{
					KeyURL:      "https://example.com/gpg",
					Keyring:     "/etc/apt/keyrings/example.gpg",
					RepoLine:    "deb [signed-by=/etc/apt/keyrings/example.gpg] https://example.com stable main",
					SourcesPath: "/etc/apt/sources.list.d/example.list",
				}
			}
			if tt.dnfRepo {
				pkg.DNFRepo = &manifest.DNFRepo{ID: "example", RepoFile: "https://example.com/example.repo"}
			}
			got, err := engine.Applicable(pkg, tt.info)
			if err != nil {
				t.Fatalf("Applicable() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Applicable(%q, os_gate=%q host_os=%q pkg_mgr=%q) = %t, want %t",
					tt.backend, tt.os, tt.info.OS, tt.info.PkgMgr, got, tt.want)
			}
		})
	}
}

// TestHostAllows_ExcludeOS verifies the static exclude_os gate rejects hosts
// whose OS family matches ANY listed token (same semantics as os, so "linux"
// matches every Linux-family host), independent of the primary package manager,
// while the os/arch/desktop gates still apply.
func TestHostAllows_ExcludeOS(t *testing.T) {
	t.Parallel()

	linuxExceptDebian := manifest.Package{Name: "x", Backend: manifest.BackendCustomScript, OS: "linux", ExcludeOS: []string{"debian"}}
	excludeLinux := manifest.Package{Name: "y", Backend: manifest.BackendCustomScript, ExcludeOS: []string{"macos", "linux"}}
	desktopOnly := manifest.Package{Name: "z", Backend: manifest.BackendCustomScript, OS: "linux", ExcludeOS: []string{"debian"}, RequiresDesktop: true, Arch: []string{"x86_64"}}
	tests := []struct {
		name string
		pkg  manifest.Package
		info platform.Info
		want bool
	}{
		{"debian apt excluded", linuxExceptDebian, platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT}, false},
		{"debian with homebrew primary still excluded", linuxExceptDebian, platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrHomebrew}, false},
		{"fedora dnf allowed", linuxExceptDebian, platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrDNF}, true},
		{"fedora with homebrew primary allowed", linuxExceptDebian, platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrHomebrew}, true},
		{"arch pacman allowed", linuxExceptDebian, platform.Info{OS: platform.OSArch, PkgMgr: platform.PkgMgrPacman}, true},
		{"macos stays os-gated", linuxExceptDebian, platform.Info{OS: platform.OSMacOS, PkgMgr: platform.PkgMgrHomebrew}, false},
		{"linux token excludes every linux family", excludeLinux, platform.Info{OS: platform.OSArch, PkgMgr: platform.PkgMgrPacman}, false},
		{"linux token excludes debian", excludeLinux, platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT}, false},
		{"macos token excludes macos", excludeLinux, platform.Info{OS: platform.OSMacOS, PkgMgr: platform.PkgMgrHomebrew}, false},
		{"windows host not excluded", excludeLinux, platform.Info{OS: platform.OSWindows}, true},
		{"exclude_os keeps arch gate", desktopOnly, platform.Info{OS: platform.OSRedHat, Arch: platform.ArchARM64, Desktop: true}, false},
		{"exclude_os keeps desktop gate", desktopOnly, platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64}, false},
		{"all gates satisfied", desktopOnly, platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64, Desktop: true}, true},
	}
	engine := backend.Default(exec.NewFake(), privilegetest.NewFakeDropper(), t.TempDir(), nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := backend.HostAllows(tt.pkg, tt.info); got != tt.want {
				t.Errorf("HostAllows() = %t, want %t", got, tt.want)
			}
			if got, err := engine.Applicable(tt.pkg, tt.info); err != nil || got != tt.want {
				t.Errorf("Applicable() = (%t, %v), want (%t, nil)", got, err, tt.want)
			}
		})
	}
}

// TestEngine_ExcludeOSLifecycle: an excluded host makes Installed, Install,
// Update and Remove no-ops that run no commands, mirroring the OS/desktop gates.
func TestEngine_ExcludeOSLifecycle(t *testing.T) {
	t.Parallel()
	fake := exec.NewFake()
	fake.AddPath("tool", "/usr/bin/tool")
	engine := backend.Default(fake, privilegetest.NewFakeDropper(), t.TempDir(), nil)
	pkg := manifest.Package{Name: "tool", Backend: manifest.BackendCustomScript, Script: "bin/missing.sh", Provides: []string{"tool"}, OS: "linux", ExcludeOS: []string{"debian"}}
	debian := platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrHomebrew}
	fedora := platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrDNF}

	if ok, err := engine.Installed(pkg, debian); err != nil || ok {
		t.Errorf("Installed(excluded) = (%t, %v), want (false, nil)", ok, err)
	}
	if err := engine.Install(pkg, debian); err != nil {
		t.Errorf("Install(excluded) = %v, want nil no-op", err)
	}
	for _, act := range []backend.Action{backend.ActionUpdate, backend.ActionRemove} {
		if err := engine.Perform(act, pkg, debian); err != nil {
			t.Errorf("Perform(%v, excluded) = %v, want nil no-op", act, err)
		}
	}
	if calls := fake.CallsSnapshot(); len(calls) != 0 {
		t.Errorf("excluded lifecycle ran commands %v, want none", calls)
	}
	if ok, err := engine.Installed(pkg, fedora); err != nil || !ok {
		t.Errorf("Installed(allowed) = (%t, %v), want (true, nil)", ok, err)
	}
}

// TestEngine_Installed_OSGate verifies the os gate short-circuits Engine.Installed
// to (false, nil) off-platform — even when the underlying probe binary IS on
// PATH — so a macOS-scoped sentinel is never (falsely) evaluated on Linux. On
// the matching host the gate is transparent and the real probe runs.
func TestEngine_Installed_OSGate(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	fake.AddPath("gsed", "/opt/homebrew/bin/gsed")
	engine := backend.Default(fake, privilegetest.NewFakeDropper(), t.TempDir(), nil)

	pkg := manifest.Package{
		Name:     "gnu-utilities",
		Backend:  manifest.BackendCustomScript,
		Script:   "bin/install-gnu-utilities.sh",
		OS:       "macos",
		Provides: []string{"gsed"},
	}

	linux := platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT}
	macOS := platform.Info{OS: platform.OSMacOS, PkgMgr: platform.PkgMgrHomebrew}

	if got, err := engine.Installed(pkg, linux); err != nil || got {
		t.Errorf("Installed(os=macos pkg, debian host) = (%t, %v), want (false, nil) — off-platform gate", got, err)
	}
	if got, err := engine.Installed(pkg, macOS); err != nil || !got {
		t.Errorf("Installed(os=macos pkg, macos host) = (%t, %v), want (true, nil) — gsed on PATH", got, err)
	}
}

// TestEngine_Install_OSGate verifies the os gate makes Engine.Install a no-op
// off-platform — the custom-script installer must not run on a non-matching
// host even if reached directly (Applicable-bypassing). On-platform the gate is
// transparent and the backend runs (here erroring only because the script path
// does not exist, which proves the installer WAS invoked past the gate).
func TestEngine_Install_OSGate(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	engine := backend.Default(fake, privilegetest.NewFakeDropper(), t.TempDir(), nil)

	pkg := manifest.Package{
		Name:    "gnu-utilities",
		Backend: manifest.BackendCustomScript,
		Script:  "bin/install-gnu-utilities.sh",
		OS:      "macos",
	}

	// Off-platform: no-op, and crucially the installer is never executed.
	if err := engine.Install(pkg, platform.Info{OS: platform.OSDebian}); err != nil {
		t.Errorf("Install(os=macos pkg, debian host) = %v, want nil (off-platform no-op)", err)
	}
	if n := len(fake.Calls); n != 0 {
		t.Errorf("off-platform Install executed %d command(s), want 0 — the gate must block the installer", n)
	}
}

func TestEngine_Applicable_UnknownBackend(t *testing.T) {
	t.Parallel()

	engine := backend.NewEngine(map[manifest.Backend]backend.Backend{})
	pkg := manifest.Package{Name: "example", Backend: manifest.Backend("nonexistent")}

	got, err := engine.Applicable(pkg, platform.Info{})
	if err == nil {
		t.Fatal("Applicable() error = nil, want *UnknownBackendError")
	}
	var unknownErr *backend.UnknownBackendError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("Applicable() error = %T, want *backend.UnknownBackendError", err)
	}
	if got {
		t.Error("Applicable() = true, want false when backend is unknown")
	}
}

// TestEngine_DesktopGate: requires_desktop packages are not-applicable, never
// probed and never installed without a graphical session.
func TestEngine_DesktopGate(t *testing.T) {
	t.Parallel()
	fake := exec.NewFake()
	fake.AddPath("espanso", "/usr/bin/espanso")
	engine := backend.Default(fake, privilegetest.NewFakeDropper(), t.TempDir(), nil)
	pkg := manifest.Package{Name: "espanso", Backend: manifest.BackendCustomScript, Script: "bin/missing.sh", Provides: []string{"espanso"}, RequiresDesktop: true}
	headless := platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT}
	desktop := headless
	desktop.Desktop = true

	if ok, err := engine.Applicable(pkg, headless); err != nil || ok {
		t.Errorf("Applicable(headless) = (%t, %v), want (false, nil)", ok, err)
	}
	if ok, err := engine.Installed(pkg, headless); err != nil || ok {
		t.Errorf("Installed(headless) = (%t, %v), want (false, nil)", ok, err)
	}
	if err := engine.Install(pkg, headless); err != nil || len(fake.CallsSnapshot()) != 0 {
		t.Errorf("Install(headless) = %v, calls %v; want nil and no commands", err, fake.CallsSnapshot())
	}
	for _, act := range []backend.Action{backend.ActionUpdate, backend.ActionRemove} {
		if err := engine.Perform(act, pkg, headless); err != nil {
			t.Errorf("Perform(%v, headless) = %v, want nil no-op", act, err)
		}
	}
	if calls := fake.CallsSnapshot(); len(calls) != 0 {
		t.Errorf("headless lifecycle ran commands %v, want none", calls)
	}
	if ok, err := engine.Applicable(pkg, desktop); err != nil || !ok {
		t.Errorf("Applicable(desktop) = (%t, %v), want (true, nil)", ok, err)
	}
	if ok, err := engine.Installed(pkg, desktop); err != nil || !ok {
		t.Errorf("Installed(desktop) = (%t, %v), want (true, nil)", ok, err)
	}
}
