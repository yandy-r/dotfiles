package backend

import (
	"errors"
	"github.com/yandy-r/dotfiles/internal/privilege/privilegetest"
	"os"
	osexec "os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/privilege"
)

func archInfo() platform.Info {
	return platform.Info{OS: platform.OSArch, PkgMgr: platform.PkgMgrPacman}
}

func TestAUR_UnsupportedOS_Install(t *testing.T) {
	t.Parallel()

	b := newAURBackend(exec.NewFake(), privilegetest.NewFakeDropper())
	pkg := manifest.Package{Name: "yazi", Backend: manifest.BackendAUR}
	info := platform.Info{OS: platform.OSDebian}

	err := b.Install(pkg, info)
	var unsupported *AURUnsupportedOSError
	if !errors.As(err, &unsupported) {
		t.Fatalf("Install() error = %T(%v), want *AURUnsupportedOSError", err, err)
	}
	if unsupported.OS != platform.OSDebian {
		t.Errorf("AURUnsupportedOSError.OS = %q, want %q", unsupported.OS, platform.OSDebian)
	}
}

func TestAUR_UnsupportedOS_Installed(t *testing.T) {
	t.Parallel()

	b := newAURBackend(exec.NewFake(), privilegetest.NewFakeDropper())
	pkg := manifest.Package{Name: "yazi", Backend: manifest.BackendAUR}
	info := platform.Info{OS: platform.OSMacOS}

	installed, err := b.Installed(pkg, info)
	if err == nil {
		t.Fatal("Installed() error = nil, want *AURUnsupportedOSError")
	}
	var unsupported *AURUnsupportedOSError
	if !errors.As(err, &unsupported) {
		t.Fatalf("Installed() error = %T, want *AURUnsupportedOSError", err)
	}
	if installed {
		t.Errorf("Installed() = true, want false")
	}
}

func TestAUR_Install_yayPresent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires non-root euid")
	}

	f := exec.NewFake()
	f.AddPath("yay", "/usr/bin/yay")

	b := newAURBackend(f, privilegetest.NewFakeDropper())
	pkg := manifest.Package{Name: "yazi", Backend: manifest.BackendAUR}

	if err := b.Install(pkg, archInfo()); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := f.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("len(Calls) = %d, want 1 (yay install only)", len(calls))
	}
	want := []string{"yay", "-S", "--noconfirm", "yazi"}
	if calls[0].Name != want[0] || len(calls[0].Args) != len(want[1:]) {
		t.Fatalf("Calls[0] = %+v, want yay install argv", calls[0])
	}
	for i, arg := range want[1:] {
		if calls[0].Args[i] != arg {
			t.Fatalf("Calls[0].Args[%d] = %q, want %q", i, calls[0].Args[i], arg)
		}
	}
}

func TestAUR_Install_bootstrapYayWhenAbsent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires non-root euid")
	}

	t.Setenv("HOME", t.TempDir())

	f := exec.NewFake()
	// LookPath("yay") returns not found; Run succeeds for bootstrap + install.
	f.AddPath("makepkg", "/usr/bin/makepkg")
	b := newAURBackend(f, privilegetest.NewFakeDropper())
	pkg := manifest.Package{Name: "yazi", Backend: manifest.BackendAUR}

	if err := b.Install(pkg, archInfo()); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := f.CallsSnapshot()
	if len(calls) != 3 {
		t.Fatalf("len(Calls) = %d, want 3 (git clone, makepkg, yay install)", len(calls))
	}

	if calls[0].Name != "git" || len(calls[0].Args) < 2 || calls[0].Args[0] != "clone" {
		t.Errorf("Calls[0] = %+v, want git clone", calls[0])
	}
	if calls[0].Args[len(calls[0].Args)-2] != "https://aur.archlinux.org/yay.git" {
		t.Errorf("git clone URL = %q, want yay AUR repo", calls[0].Args[len(calls[0].Args)-2])
	}

	if calls[1].Name != "env" || len(calls[1].Args) < 2 {
		t.Errorf("Calls[1] = %+v, want env --chdir=... makepkg", calls[1])
	}
	if calls[1].Args[1] != "makepkg" {
		t.Errorf("Calls[1].Args[1] = %q, want makepkg", calls[1].Args[1])
	}

	if calls[2].Name != "yay" {
		t.Errorf("Calls[2].Name = %q, want yay", calls[2].Name)
	}
}

func TestAUR_Install_bootstrapMissingMakepkg(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires non-root euid")
	}

	t.Setenv("HOME", t.TempDir())

	f := exec.NewFake()
	// Neither yay nor makepkg resolve on PATH.
	b := newAURBackend(f, privilegetest.NewFakeDropper())
	pkg := manifest.Package{Name: "yazi", Backend: manifest.BackendAUR}

	err := b.Install(pkg, archInfo())
	if err == nil {
		t.Fatal("Install() error = nil, want makepkg pre-flight failure")
	}
	if !strings.Contains(err.Error(), "base-devel") {
		t.Errorf("Install() error = %q, want mention of base-devel", err)
	}
	if calls := f.CallsSnapshot(); len(calls) != 0 {
		t.Errorf("len(Calls) = %d, want 0 (fail before cloning)", len(calls))
	}
}

func TestAUR_Install_bootstrapClearsStaleDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires non-root euid")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	// Simulate a prior failed bootstrap: the fixed cache dir already exists
	// and is non-empty, which git clone would refuse to reuse.
	dir := filepath.Join(home, ".cache", "dotf", "yay-bootstrap")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PKGBUILD"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	f := exec.NewFake()
	f.AddPath("makepkg", "/usr/bin/makepkg")
	b := newAURBackend(f, privilegetest.NewFakeDropper())
	pkg := manifest.Package{Name: "yazi", Backend: manifest.BackendAUR}

	if err := b.Install(pkg, archInfo()); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := f.CallsSnapshot()
	if len(calls) != 3 {
		t.Fatalf("len(Calls) = %d, want 3 (git clone, makepkg, yay install)", len(calls))
	}
	if calls[0].Name != "git" || calls[0].Args[0] != "clone" {
		t.Errorf("Calls[0] = %+v, want git clone despite pre-existing dir", calls[0])
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("bootstrap dir %q still exists after Install, want removed", dir)
	}
}

func TestAUR_Install_rerunnableAfterMakepkgFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires non-root euid")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".cache", "dotf", "yay-bootstrap")

	f := exec.NewFake()
	f.AddPath("makepkg", "/usr/bin/makepkg")
	makepkgKey := exec.CommandKey("env", "--chdir="+dir, "makepkg", "-si", "--noconfirm")
	f.RunErrors[makepkgKey] = &osexec.ExitError{}

	b := newAURBackend(f, privilegetest.NewFakeDropper())
	pkg := manifest.Package{Name: "yazi", Backend: manifest.BackendAUR}

	if err := b.Install(pkg, archInfo()); err == nil {
		t.Fatal("Install() error = nil, want makepkg failure")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("bootstrap dir %q still exists after failed bootstrap, want removed", dir)
	}

	// Retry after "installing base-devel": makepkg now succeeds and the
	// bootstrap re-runs git clone from a clean slate.
	delete(f.RunErrors, makepkgKey)
	if err := b.Install(pkg, archInfo()); err != nil {
		t.Fatalf("second Install() error = %v, want nil", err)
	}

	calls := f.CallsSnapshot()
	if len(calls) != 5 {
		t.Fatalf("len(Calls) = %d, want 5 (clone, makepkg fail, clone, makepkg, yay)", len(calls))
	}
	if calls[2].Name != "git" || calls[2].Args[0] != "clone" {
		t.Errorf("Calls[2] = %+v, want git clone on retry", calls[2])
	}
	if calls[4].Name != "yay" {
		t.Errorf("Calls[4].Name = %q, want yay", calls[4].Name)
	}
}

func TestAUR_Installed_viaPacmanQ(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	f.AddCommand("pacman", []string{"-Q", "yazi"}, []byte("yazi 0.1\n"), nil)

	b := newAURBackend(f, privilegetest.NewFakeDropper())
	pkg := manifest.Package{Name: "yazi", Backend: manifest.BackendAUR}

	installed, err := b.Installed(pkg, archInfo())
	if err != nil {
		t.Fatalf("Installed() error = %v, want nil", err)
	}
	if !installed {
		t.Error("Installed() = false, want true")
	}
}

func TestAUR_Installed_notInstalled(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	f.AddCommand("pacman", []string{"-Q", "missing"}, nil, &osexec.ExitError{})

	b := newAURBackend(f, privilegetest.NewFakeDropper())
	pkg := manifest.Package{Name: "missing", Backend: manifest.BackendAUR}

	installed, err := b.Installed(pkg, archInfo())
	if err != nil {
		t.Fatalf("Installed() error = %v, want nil", err)
	}
	if installed {
		t.Error("Installed() = true, want false")
	}
}

func TestAUR_Install_rootMissingSudoUser(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root euid")
	}

	t.Setenv("SUDO_USER", "")
	t.Setenv("SUDO_UID", "")
	t.Setenv("SUDO_GID", "")

	f := exec.NewFake()
	f.AddPath("yay", "/usr/bin/yay")
	b := newAURBackend(f, privilegetest.NewFakeDropper())
	pkg := manifest.Package{Name: "yazi", Backend: manifest.BackendAUR}

	err := b.Install(pkg, archInfo())
	if !errors.Is(err, privilege.ErrNoSudoUser) {
		t.Fatalf("Install() error = %v, want ErrNoSudoUser", err)
	}
	if len(f.CallsSnapshot()) != 0 {
		t.Errorf("len(Calls) = %d, want 0 when SUDO_USER missing", len(f.CallsSnapshot()))
	}
}

func TestAUR_Install_dropNeverUIDZero(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root euid")
	}

	t.Setenv("SUDO_USER", "builder")
	t.Setenv("SUDO_UID", "1000")
	t.Setenv("SUDO_GID", "1000")

	f := exec.NewFake()
	f.AddPath("yay", "/usr/bin/yay")
	drop := privilegetest.NewFakeDropper()
	b := newAURBackend(f, drop)
	pkg := manifest.Package{Name: "yazi", Backend: manifest.BackendAUR}

	if err := b.Install(pkg, archInfo()); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}
	if len(drop.Runs) != 1 {
		t.Fatalf("len(drop.Runs) = %d, want 1", len(drop.Runs))
	}
	if drop.Runs[0].UID == 0 {
		t.Errorf("drop.Runs[0].UID = 0, want non-zero dropped uid")
	}
	if drop.Runs[0].Argv[0] != "yay" {
		t.Errorf("drop.Runs[0].Argv = %v, want yay install argv", drop.Runs[0].Argv)
	}
	if len(f.CallsSnapshot()) != 0 {
		t.Errorf("len(Calls) = %d, want 0 (install routed through dropper)", len(f.CallsSnapshot()))
	}
}

func TestAUR_UpdateRemove_OffArchUnsupported(t *testing.T) {
	pkg := manifest.Package{Name: "yazi", Backend: manifest.BackendAUR}
	info := platform.Info{OS: platform.OSDebian}
	b := newAURBackend(exec.NewFake(), privilegetest.NewFakeDropper())
	if err := b.(Updater).Update(pkg, info); !isAURUnsupported(err) {
		t.Errorf("Update() = %v, want *AURUnsupportedOSError", err)
	}
	if err := b.(Remover).Remove(pkg, info); !isAURUnsupported(err) {
		t.Errorf("Remove() = %v, want *AURUnsupportedOSError", err)
	}
}

func isAURUnsupported(err error) bool {
	var u *AURUnsupportedOSError
	return errors.As(err, &u)
}

func TestAUR_UpdateRemove_Argv(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires non-root euid")
	}

	pkg := manifest.Package{Name: "yazi", Backend: manifest.BackendAUR}
	tests := []struct {
		name string
		run  func(b Backend) error
		want exec.FakeCall
	}{
		{"update", func(b Backend) error { return b.(Updater).Update(pkg, archInfo()) },
			exec.FakeCall{Name: "yay", Args: []string{"-S", "--noconfirm", "--needed", "yazi"}}},
		{"remove", func(b Backend) error { return b.(Remover).Remove(pkg, archInfo()) },
			exec.FakeCall{Name: "sudo", Args: []string{"pacman", "-Rns", "--noconfirm", "yazi"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := exec.NewFake()
			f.AddPath("yay", "/usr/bin/yay")
			if err := tt.run(newAURBackend(f, privilegetest.NewFakeDropper())); err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			calls := f.CallsSnapshot()
			if len(calls) != 1 || calls[0].Name != tt.want.Name || !slices.Equal(calls[0].Args, tt.want.Args) {
				t.Errorf("calls = %+v, want one %+v", calls, tt.want)
			}
		})
	}
}
