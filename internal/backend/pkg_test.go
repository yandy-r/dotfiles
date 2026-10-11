package backend

import (
	"errors"
	"fmt"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestPkgBackend_Install_Argv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pkgMgr platform.PkgMgr
		want   exec.FakeCall
	}{
		{
			pkgMgr: platform.PkgMgrPacman,
			want: exec.FakeCall{
				Name: "sudo",
				Args: []string{"pacman", "-S", "--noconfirm", "--needed", "ripgrep"},
			},
		},
		{
			pkgMgr: platform.PkgMgrAPT,
			want: exec.FakeCall{
				Name: "sudo",
				Args: []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "ripgrep"},
			},
		},
		{
			pkgMgr: platform.PkgMgrDNF,
			want: exec.FakeCall{
				Name: "sudo",
				Args: []string{"dnf", "install", "-y", "ripgrep"},
			},
		},
		{
			pkgMgr: platform.PkgMgrZypper,
			want: exec.FakeCall{
				Name: "sudo",
				Args: []string{"zypper", "--non-interactive", "install", "ripgrep"},
			},
		},
		{
			pkgMgr: platform.PkgMgrHomebrew,
			want: exec.FakeCall{
				Name: "brew",
				Args: []string{"install", "ripgrep"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.pkgMgr), func(t *testing.T) {
			t.Parallel()

			fake := exec.NewFake()
			b := newPkgBackend(fake)

			pkg := manifest.Package{
				Name:    "ripgrep",
				Backend: manifest.BackendPkg,
				Names: map[string]string{
					string(tt.pkgMgr): "ripgrep",
				},
			}
			info := platform.Info{PkgMgr: tt.pkgMgr}

			if err := b.Install(pkg, info); err != nil {
				t.Fatalf("Install() error = %v, want nil", err)
			}

			calls := fake.CallsSnapshot()
			if len(calls) != 1 {
				t.Fatalf("CallsSnapshot() len = %d, want 1", len(calls))
			}
			if calls[0].Name != tt.want.Name {
				t.Errorf("call name = %q, want %q", calls[0].Name, tt.want.Name)
			}
			if fmt.Sprint(calls[0].Args) != fmt.Sprint(tt.want.Args) {
				t.Errorf("call args = %v, want %v", calls[0].Args, tt.want.Args)
			}
			if tt.pkgMgr == platform.PkgMgrHomebrew && calls[0].Name == "sudo" {
				t.Error("brew install must never run under sudo")
			}
		})
	}
}

func TestPkgBackend_DNFExclude_Argv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		pkgMgr platform.PkgMgr
		run    func(*pkgBackend, manifest.Package, platform.Info) error
		want   []string
	}{
		{
			name:   "install on dnf",
			pkgMgr: platform.PkgMgrDNF,
			run:    (*pkgBackend).Install,
			want:   []string{"dnf", "install", "--exclude=kf6-kmime*", "--exclude=foo", "-y", "vlc"},
		},
		{
			name:   "update on dnf",
			pkgMgr: platform.PkgMgrDNF,
			run:    (*pkgBackend).Update,
			want:   []string{"dnf", "upgrade", "--exclude=kf6-kmime*", "--exclude=foo", "-y", "vlc"},
		},
		{
			name:   "ignored on pacman",
			pkgMgr: platform.PkgMgrPacman,
			run:    (*pkgBackend).Install,
			want:   []string{"pacman", "-S", "--noconfirm", "--needed", "vlc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := exec.NewFake()
			b := newPkgBackend(fake).(*pkgBackend)
			pkg := manifest.Package{
				Name:       "vlc",
				Backend:    manifest.BackendPkg,
				Names:      map[string]string{string(tt.pkgMgr): "vlc"},
				DNFExclude: []string{"kf6-kmime*", "foo"},
			}

			if err := tt.run(b, pkg, platform.Info{PkgMgr: tt.pkgMgr}); err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			calls := fake.CallsSnapshot()
			if len(calls) != 1 {
				t.Fatalf("calls len = %d, want 1", len(calls))
			}
			if fmt.Sprint(calls[0].Args) != fmt.Sprint(tt.want) {
				t.Errorf("args = %v, want %v", calls[0].Args, tt.want)
			}
		})
	}
}

func TestPkgBackend_Update_Argv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pkgMgr platform.PkgMgr
		want   exec.FakeCall
	}{
		{platform.PkgMgrPacman, exec.FakeCall{Name: "sudo", Args: []string{"pacman", "-S", "--noconfirm", "--needed", "ripgrep"}}},
		{platform.PkgMgrAPT, exec.FakeCall{Name: "sudo", Args: []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "--only-upgrade", "-y", "ripgrep"}}},
		{platform.PkgMgrDNF, exec.FakeCall{Name: "sudo", Args: []string{"dnf", "upgrade", "-y", "ripgrep"}}},
		{platform.PkgMgrZypper, exec.FakeCall{Name: "sudo", Args: []string{"zypper", "--non-interactive", "update", "ripgrep"}}},
		{platform.PkgMgrHomebrew, exec.FakeCall{Name: "brew", Args: []string{"upgrade", "ripgrep"}}},
	}

	for _, tt := range tests {
		t.Run(string(tt.pkgMgr), func(t *testing.T) {
			t.Parallel()

			fake := exec.NewFake()
			b := newPkgBackend(fake).(*pkgBackend)
			pkg := manifest.Package{
				Name:    "ripgrep",
				Backend: manifest.BackendPkg,
				Names:   map[string]string{string(tt.pkgMgr): "ripgrep"},
			}
			info := platform.Info{PkgMgr: tt.pkgMgr}

			if err := b.Update(pkg, info); err != nil {
				t.Fatalf("Update() error = %v, want nil", err)
			}
			calls := fake.CallsSnapshot()
			if len(calls) != 1 {
				t.Fatalf("CallsSnapshot() len = %d, want 1", len(calls))
			}
			if calls[0].Name != tt.want.Name || fmt.Sprint(calls[0].Args) != fmt.Sprint(tt.want.Args) {
				t.Errorf("Update() call = %s %v, want %s %v", calls[0].Name, calls[0].Args, tt.want.Name, tt.want.Args)
			}
			if tt.pkgMgr == platform.PkgMgrHomebrew && calls[0].Name == "sudo" {
				t.Error("brew upgrade must never run under sudo")
			}
		})
	}
}

func TestPkgBackend_Remove_Argv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pkgMgr platform.PkgMgr
		want   exec.FakeCall
	}{
		{platform.PkgMgrPacman, exec.FakeCall{Name: "sudo", Args: []string{"pacman", "-Rns", "--noconfirm", "ripgrep"}}},
		{platform.PkgMgrAPT, exec.FakeCall{Name: "sudo", Args: []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "remove", "-y", "ripgrep"}}},
		{platform.PkgMgrDNF, exec.FakeCall{Name: "sudo", Args: []string{"dnf", "remove", "-y", "ripgrep"}}},
		{platform.PkgMgrZypper, exec.FakeCall{Name: "sudo", Args: []string{"zypper", "--non-interactive", "remove", "ripgrep"}}},
		{platform.PkgMgrHomebrew, exec.FakeCall{Name: "brew", Args: []string{"uninstall", "ripgrep"}}},
	}

	for _, tt := range tests {
		t.Run(string(tt.pkgMgr), func(t *testing.T) {
			t.Parallel()

			fake := exec.NewFake()
			b := newPkgBackend(fake).(*pkgBackend)
			pkg := manifest.Package{
				Name:    "ripgrep",
				Backend: manifest.BackendPkg,
				Names:   map[string]string{string(tt.pkgMgr): "ripgrep"},
			}
			info := platform.Info{PkgMgr: tt.pkgMgr}

			if err := b.Remove(pkg, info); err != nil {
				t.Fatalf("Remove() error = %v, want nil", err)
			}
			calls := fake.CallsSnapshot()
			if len(calls) != 1 {
				t.Fatalf("CallsSnapshot() len = %d, want 1", len(calls))
			}
			if calls[0].Name != tt.want.Name || fmt.Sprint(calls[0].Args) != fmt.Sprint(tt.want.Args) {
				t.Errorf("Remove() call = %s %v, want %s %v", calls[0].Name, calls[0].Args, tt.want.Name, tt.want.Args)
			}
			if tt.pkgMgr == platform.PkgMgrHomebrew && calls[0].Name == "sudo" {
				t.Error("brew uninstall must never run under sudo")
			}
		})
	}
}

func TestPkgBackend_Installed_QueryArgv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pkgMgr platform.PkgMgr
		want   exec.FakeCall
		out    string
	}{
		{
			pkgMgr: platform.PkgMgrPacman,
			want:   exec.FakeCall{Name: "pacman", Args: []string{"-Q", "ripgrep"}},
			out:    "ok",
		},
		{
			pkgMgr: platform.PkgMgrAPT,
			want:   exec.FakeCall{Name: "dpkg-query", Args: []string{"-W", "-f=${db:Status-Status}", "ripgrep"}},
			out:    "installed",
		},
		{
			pkgMgr: platform.PkgMgrDNF,
			want:   exec.FakeCall{Name: "rpm", Args: []string{"-q", "ripgrep"}},
			out:    "ok",
		},
		{
			pkgMgr: platform.PkgMgrZypper,
			want:   exec.FakeCall{Name: "rpm", Args: []string{"-q", "ripgrep"}},
			out:    "ok",
		},
		{
			pkgMgr: platform.PkgMgrHomebrew,
			want:   exec.FakeCall{Name: "brew", Args: []string{"list", "ripgrep"}},
			out:    "ok",
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.pkgMgr), func(t *testing.T) {
			t.Parallel()

			fake := exec.NewFake()
			fake.AddCommand(tt.want.Name, tt.want.Args, []byte(tt.out), nil)

			b := newPkgBackend(fake)
			pkg := manifest.Package{
				Name: "ripgrep",
				Names: map[string]string{
					string(tt.pkgMgr): "ripgrep",
				},
			}
			info := platform.Info{PkgMgr: tt.pkgMgr}

			installed, err := b.Installed(pkg, info)
			if err != nil {
				t.Fatalf("Installed() error = %v, want nil", err)
			}
			if !installed {
				t.Fatal("Installed() = false, want true")
			}

			calls := fake.CallsSnapshot()
			if len(calls) != 1 {
				t.Fatalf("CallsSnapshot() len = %d, want 1", len(calls))
			}
			if calls[0].Name != tt.want.Name {
				t.Errorf("call name = %q, want %q", calls[0].Name, tt.want.Name)
			}
			if fmt.Sprint(calls[0].Args) != fmt.Sprint(tt.want.Args) {
				t.Errorf("call args = %v, want %v", calls[0].Args, tt.want.Args)
			}
		})
	}
}

func TestPkgBackend_Installed_APTStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		out  string
		want bool
	}{
		{name: "installed", out: "installed\n", want: true},
		{name: "config-files", out: "config-files\n", want: false},
		{name: "not-installed", out: "not-installed\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := exec.NewFake()
			fake.AddCommand("dpkg-query", []string{"-W", "-f=${db:Status-Status}", "docker-ce"}, []byte(tt.out), nil)

			b := newPkgBackend(fake)
			pkg := manifest.Package{
				Name:  "docker-ce",
				Names: map[string]string{string(platform.PkgMgrAPT): "docker-ce"},
			}
			info := platform.Info{PkgMgr: platform.PkgMgrAPT}

			installed, err := b.Installed(pkg, info)
			if err != nil {
				t.Fatalf("Installed() error = %v, want nil", err)
			}
			if installed != tt.want {
				t.Fatalf("Installed() = %v, want %v", installed, tt.want)
			}
		})
	}
}

func TestPkgBackend_Installed_NotPresent(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	fake.AddCommand("pacman", []string{"-Q", "missing"}, nil, errors.New("not found"))

	b := newPkgBackend(fake)
	pkg := manifest.Package{
		Name:  "missing",
		Names: map[string]string{string(platform.PkgMgrPacman): "missing"},
	}
	info := platform.Info{PkgMgr: platform.PkgMgrPacman}

	installed, err := b.Installed(pkg, info)
	if err != nil {
		t.Fatalf("Installed() error = %v, want nil", err)
	}
	if installed {
		t.Fatal("Installed() = true, want false")
	}
}

func TestPkgBackend_MissingPkgName_ReturnsTypedError(t *testing.T) {
	t.Parallel()

	b := newPkgBackend(exec.NewFake())
	pkg := manifest.Package{
		Name:  "ripgrep",
		Names: map[string]string{string(platform.PkgMgrAPT): "ripgrep"},
	}
	info := platform.Info{PkgMgr: platform.PkgMgrPacman}

	if err := b.Install(pkg, info); err != nil {
		var missingErr *MissingPkgNameError
		if !errors.As(err, &missingErr) {
			t.Fatalf("Install() error = %T, want *MissingPkgNameError", err)
		}
		if missingErr.Package != "ripgrep" {
			t.Errorf("MissingPkgNameError.Package = %q, want ripgrep", missingErr.Package)
		}
		if missingErr.PkgMgr != platform.PkgMgrPacman {
			t.Errorf("MissingPkgNameError.PkgMgr = %q, want pacman", missingErr.PkgMgr)
		}
	} else {
		t.Fatal("Install() error = nil, want *MissingPkgNameError")
	}

	installed, err := b.Installed(pkg, info)
	if err != nil {
		var missingErr *MissingPkgNameError
		if !errors.As(err, &missingErr) {
			t.Fatalf("Installed() error = %T, want *MissingPkgNameError", err)
		}
	} else {
		t.Fatal("Installed() error = nil, want *MissingPkgNameError")
	}
	if installed {
		t.Error("Installed() = true, want false")
	}
}

func TestPkgBackend_UnsupportedPkgMgr_ReturnsTypedError(t *testing.T) {
	t.Parallel()

	b := newPkgBackend(exec.NewFake())
	pkg := manifest.Package{
		Name:  "ripgrep",
		Names: map[string]string{string(platform.PkgMgrUnknown): "ripgrep"},
	}
	info := platform.Info{PkgMgr: platform.PkgMgrUnknown}

	if err := b.Install(pkg, info); err != nil {
		var unsupportedErr *UnsupportedPkgMgrError
		if !errors.As(err, &unsupportedErr) {
			t.Fatalf("Install() error = %T, want *UnsupportedPkgMgrError", err)
		}
		if unsupportedErr.PkgMgr != platform.PkgMgrUnknown {
			t.Errorf("UnsupportedPkgMgrError.PkgMgr = %q, want unknown", unsupportedErr.PkgMgr)
		}
	} else {
		t.Fatal("Install() error = nil, want *UnsupportedPkgMgrError")
	}

	installed, err := b.Installed(pkg, info)
	if err != nil {
		var unsupportedErr *UnsupportedPkgMgrError
		if !errors.As(err, &unsupportedErr) {
			t.Fatalf("Installed() error = %T, want *UnsupportedPkgMgrError", err)
		}
	} else {
		t.Fatal("Installed() error = nil, want *UnsupportedPkgMgrError")
	}
	if installed {
		t.Error("Installed() = true, want false")
	}
}
