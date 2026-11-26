package backend

import (
	"errors"
	"slices"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func flatpakPkg() manifest.Package {
	return manifest.Package{Name: "bottles", Backend: manifest.BackendFlatpak, FlatpakID: "com.usebottles.bottles"}
}

func fedoraInfo() platform.Info {
	return platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrDNF}
}

func newTestFlatpak(f *exec.Fake, euid int) *flatpakBackend {
	b := newFlatpakBackend(f)
	b.euid = func() int { return euid }
	return b
}

func callArgv(c exec.FakeCall) []string {
	return append([]string{c.Name}, c.Args...)
}

func TestFlatpak_Applicable(t *testing.T) {
	t.Parallel()
	b := newFlatpakBackend(exec.NewFake())
	for _, tc := range []struct {
		os   platform.OS
		want bool
	}{
		{platform.OSRedHat, true},
		{platform.OSDebian, true},
		{platform.OSArch, true},
		{platform.OSSUSE, true},
		{platform.OSMacOS, false},
	} {
		if got := b.applicable(platform.Info{OS: tc.os}); got != tc.want {
			t.Errorf("applicable(%s) = %v, want %v", tc.os, got, tc.want)
		}
	}
}

func TestFlatpak_Install_Flathub(t *testing.T) {
	t.Parallel()
	f := exec.NewFake()
	f.AddPath("flatpak", "/usr/bin/flatpak")

	if err := newTestFlatpak(f, 1000).Install(flatpakPkg(), fedoraInfo()); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	calls := f.CallsSnapshot()
	want := [][]string{
		{"flatpak", "remote-add", "--user", "--if-not-exists", "flathub", flathubRepo},
		{"flatpak", "install", "--user", "--noninteractive", "-y", "flathub", "com.usebottles.bottles"},
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if got := callArgv(calls[i]); !slices.Equal(got, want[i]) {
			t.Errorf("call[%d] = %v, want %v", i, got, want[i])
		}
	}
}

func TestFlatpak_Install_CustomRemoteSkipsRemoteAdd(t *testing.T) {
	t.Parallel()
	f := exec.NewFake()
	f.AddPath("flatpak", "/usr/bin/flatpak")
	pkg := flatpakPkg()
	pkg.FlatpakRemote = "flathub-beta"

	if err := newTestFlatpak(f, 1000).Install(pkg, fedoraInfo()); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	calls := f.CallsSnapshot()
	want := []string{"flatpak", "install", "--user", "--noninteractive", "-y", "flathub-beta", "com.usebottles.bottles"}
	if len(calls) != 1 || !slices.Equal(callArgv(calls[0]), want) {
		t.Fatalf("calls = %v, want [%v]", calls, want)
	}
}

func TestFlatpak_Install_MissingFlatpak(t *testing.T) {
	t.Parallel()
	err := newTestFlatpak(exec.NewFake(), 1000).Install(flatpakPkg(), fedoraInfo())
	var nf *FlatpakNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("Install() error = %T(%v), want *FlatpakNotFoundError", err, err)
	}
}

func TestFlatpak_Install_RefusesRoot(t *testing.T) {
	t.Parallel()
	f := exec.NewFake()
	f.AddPath("flatpak", "/usr/bin/flatpak")
	err := newTestFlatpak(f, 0).Install(flatpakPkg(), fedoraInfo())
	var re *FlatpakRootError
	if !errors.As(err, &re) {
		t.Fatalf("Install() error = %T(%v), want *FlatpakRootError", err, err)
	}
	if n := len(f.CallsSnapshot()); n != 0 {
		t.Errorf("ran %d commands as root, want 0", n)
	}
}

func TestFlatpak_Installed(t *testing.T) {
	t.Parallel()

	t.Run("present", func(t *testing.T) {
		t.Parallel()
		f := exec.NewFake()
		f.AddPath("flatpak", "/usr/bin/flatpak")
		f.AddCommand("flatpak", []string{"info", "com.usebottles.bottles"}, []byte("Bottles"), nil)
		got, err := newTestFlatpak(f, 1000).Installed(flatpakPkg(), fedoraInfo())
		if err != nil || !got {
			t.Fatalf("Installed() = %v, %v; want true, nil", got, err)
		}
	})

	t.Run("absent", func(t *testing.T) {
		t.Parallel()
		f := exec.NewFake()
		f.AddPath("flatpak", "/usr/bin/flatpak")
		f.AddCommand("flatpak", []string{"info", "com.usebottles.bottles"}, nil, errors.New("not installed"))
		got, err := newTestFlatpak(f, 1000).Installed(flatpakPkg(), fedoraInfo())
		if err != nil || got {
			t.Fatalf("Installed() = %v, %v; want false, nil", got, err)
		}
	})

	t.Run("no flatpak", func(t *testing.T) {
		t.Parallel()
		got, err := newTestFlatpak(exec.NewFake(), 1000).Installed(flatpakPkg(), fedoraInfo())
		if err != nil || got {
			t.Fatalf("Installed() = %v, %v; want false, nil", got, err)
		}
	})
}

func TestFlatpak_UpdateRemove(t *testing.T) {
	t.Parallel()
	f := exec.NewFake()
	f.AddPath("flatpak", "/usr/bin/flatpak")
	b := newTestFlatpak(f, 1000)

	if err := b.Update(flatpakPkg(), fedoraInfo()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if err := b.Remove(flatpakPkg(), fedoraInfo()); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	calls := f.CallsSnapshot()
	want := [][]string{
		{"flatpak", "update", "--noninteractive", "-y", "com.usebottles.bottles"},
		{"flatpak", "uninstall", "--noninteractive", "-y", "com.usebottles.bottles"},
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if got := callArgv(calls[i]); !slices.Equal(got, want[i]) {
			t.Errorf("call[%d] = %v, want %v", i, got, want[i])
		}
	}
}
