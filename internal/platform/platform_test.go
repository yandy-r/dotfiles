package platform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
)

// writeOSRelease writes a minimal os-release file under t.TempDir and
// returns its path.
func writeOSRelease(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

func TestArch_ReleaseArch(t *testing.T) {
	tests := []struct {
		name string
		arch Arch
		want string
	}{
		{"x86_64", ArchX86_64, "amd64"},
		{"arm64", ArchARM64, "arm64"},
		{"arm", ArchARM, "arm"},
		{"unknown passthrough", Arch("riscv64"), "riscv64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.arch.ReleaseArch(); got != tt.want {
				t.Errorf("ReleaseArch() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectArch(t *testing.T) {
	tests := []struct {
		name   string
		unameM string
		want   Arch
	}{
		{"amd64 drift case", "amd64", ArchX86_64},
		{"x86_64", "x86_64", ArchX86_64},
		{"aarch64", "aarch64", ArchARM64},
		{"arm64", "arm64", ArchARM64},
		{"armv7l", "armv7l", ArchARM},
		{"armv6l", "armv6l", ArchARM},
		{"unknown passthrough", "riscv64", Arch("riscv64")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := exec.NewFake()
			r.AddCommand("uname", []string{"-m"}, []byte(tt.unameM+"\n"), nil)

			if got := detectArch(r); got != tt.want {
				t.Errorf("detectArch() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectArch_UnameUnavailable_FallsBackGracefully(t *testing.T) {
	r := exec.NewFake() // no "uname" command registered -> LookPath/Output errors

	// Must not panic; result is whatever runtime.GOARCH maps to (possibly a
	// raw passthrough), but must be non-empty given a real runtime.GOARCH.
	got := detectArch(r)
	if got == "" {
		t.Errorf("detectArch() = %q, want non-empty fallback", got)
	}
}

func TestDetectOS(t *testing.T) {
	tests := []struct {
		name          string
		osRelease     string
		noOSRelease   bool
		procVersion   string
		noProcVersion bool
		want          OS
	}{
		{
			name:      "cachyos maps to arch",
			osRelease: "ID=cachyos\nID_LIKE=arch\n",
			want:      OSArch,
		},
		{
			name:      "rocky maps to redhat",
			osRelease: "ID=rocky\nID_LIKE=\"rhel centos fedora\"\n",
			want:      OSRedHat,
		},
		{
			name:      "fedora maps to redhat",
			osRelease: "ID=fedora\n",
			want:      OSRedHat,
		},
		{
			name:      "fedora-asahi-remix maps to redhat without ID_LIKE",
			osRelease: "ID=fedora-asahi-remix\n",
			want:      OSRedHat,
		},
		{
			name:      "asahi-fedora-remix maps to redhat without ID_LIKE",
			osRelease: "ID=asahi-fedora-remix\n",
			want:      OSRedHat,
		},
		{
			name:      "unknown ID falls back to ID_LIKE debian",
			osRelease: "ID=somedistro\nID_LIKE=debian\n",
			want:      OSDebian,
		},
		{
			name:      "unknown ID falls back to ID_LIKE rhel",
			osRelease: "ID=somedistro\nID_LIKE=rhel\n",
			want:      OSRedHat,
		},
		{
			name:      "unknown ID falls back to ID_LIKE arch",
			osRelease: "ID=somedistro\nID_LIKE=arch\n",
			want:      OSArch,
		},
		{
			name:      "unknown ID falls back to ID_LIKE suse",
			osRelease: "ID=somedistro\nID_LIKE=suse\n",
			want:      OSSUSE,
		},
		{
			name:      "ubuntu maps to debian",
			osRelease: "ID=ubuntu\n",
			want:      OSDebian,
		},
		{
			name:      "opensuse-tumbleweed maps to suse",
			osRelease: "ID=opensuse-tumbleweed\n",
			want:      OSSUSE,
		},
		{
			name:          "no os-release, no proc/version -> linux",
			noOSRelease:   true,
			noProcVersion: true,
			want:          OSLinux,
		},
		{
			name:        "no os-release, proc/version has microsoft -> wsl",
			noOSRelease: true,
			procVersion: "Linux version 5.15.0 (Microsoft@Microsoft.com)\n",
			want:        OSWSL,
		},
		{
			name:        "no os-release, proc/version unrelated -> linux",
			noOSRelease: true,
			procVersion: "Linux version 6.1.0-generic\n",
			want:        OSLinux,
		},
		{
			name:      "empty os-release file -> linux fallback",
			osRelease: "",
			want:      OSLinux,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := exec.NewFake()
			r.AddCommand("uname", []string{"-s"}, []byte("Linux\n"), nil)

			osReleasePath := filepath.Join(t.TempDir(), "missing-os-release")
			if !tt.noOSRelease {
				osReleasePath = writeOSRelease(t, tt.osRelease)
			}

			procVersionPath := filepath.Join(t.TempDir(), "missing-proc-version")
			if !tt.noProcVersion && tt.procVersion != "" {
				procVersionPath = filepath.Join(t.TempDir(), "proc-version")
				if err := os.WriteFile(procVersionPath, []byte(tt.procVersion), 0o644); err != nil {
					t.Fatalf("WriteFile(proc-version) error = %v", err)
				}
			}

			if got := detectOS(r, osReleasePath, procVersionPath); got != tt.want {
				t.Errorf("detectOS() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectOS_Darwin(t *testing.T) {
	r := exec.NewFake()
	r.AddCommand("uname", []string{"-s"}, []byte("Darwin\n"), nil)

	if got := detectOS(r, "/nonexistent/os-release", "/nonexistent/proc-version"); got != OSMacOS {
		t.Errorf("detectOS() = %q, want %q", got, OSMacOS)
	}
}

func TestDetectOS_Windows(t *testing.T) {
	r := exec.NewFake()
	r.AddCommand("uname", []string{"-s"}, []byte("MINGW64_NT-10.0\n"), nil)

	if got := detectOS(r, "/nonexistent/os-release", "/nonexistent/proc-version"); got != OSWindows {
		t.Errorf("detectOS() = %q, want %q", got, OSWindows)
	}
}

func TestDetectOS_UnknownUname(t *testing.T) {
	r := exec.NewFake()
	r.AddCommand("uname", []string{"-s"}, []byte("FreeBSD\n"), nil)

	if got := detectOS(r, "/nonexistent/os-release", "/nonexistent/proc-version"); got != OSUnknown {
		t.Errorf("detectOS() = %q, want %q", got, OSUnknown)
	}
}

func TestDetectOS_UnameUnavailable_DoesNotPanic(t *testing.T) {
	r := exec.NewFake() // no "uname" registered

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("detectOS() panicked: %v", r)
		}
	}()

	_ = detectOS(r, "/nonexistent/os-release", "/nonexistent/proc-version")
}

func TestDetectPkgMgr(t *testing.T) {
	tests := []struct {
		name  string
		os    OS
		arch  Arch
		setup func(r *exec.Fake)
		want  PkgMgr
	}{
		{
			name: "pacman only",
			os:   OSArch, arch: ArchX86_64,
			setup: func(r *exec.Fake) {
				r.AddPath("pacman", "/usr/bin/pacman")
			},
			want: PkgMgrPacman,
		},
		{
			name: "macos: brew wins over any native manager",
			os:   OSMacOS, arch: ArchARM64,
			setup: func(r *exec.Fake) {
				r.AddPath("brew", "/home/linuxbrew/.linuxbrew/bin/brew")
				r.AddPath("apt", "/usr/bin/apt")
			},
			want: PkgMgrHomebrew,
		},
		{
			name: "linux x86_64: brew still wins so --homebrew=true works as documented",
			os:   OSDebian, arch: ArchX86_64,
			setup: func(r *exec.Fake) {
				r.AddPath("brew", "/home/linuxbrew/.linuxbrew/bin/brew")
				r.AddPath("apt", "/usr/bin/apt")
			},
			want: PkgMgrHomebrew,
		},
		{
			name: "linux arm64: native dnf wins over an incidental linuxbrew (no aarch64 bottles)",
			os:   OSRedHat, arch: ArchARM64,
			setup: func(r *exec.Fake) {
				r.AddPath("brew", "/home/linuxbrew/.linuxbrew/bin/brew")
				r.AddPath("dnf", "/usr/bin/dnf")
			},
			want: PkgMgrDNF,
		},
		{
			name: "linux arm64: linuxbrew still selected when no native manager exists",
			os:   OSLinux, arch: ArchARM64,
			setup: func(r *exec.Fake) {
				r.AddPath("brew", "/home/linuxbrew/.linuxbrew/bin/brew")
			},
			want: PkgMgrHomebrew,
		},
		{
			name: "dnf only",
			os:   OSRedHat, arch: ArchX86_64,
			setup: func(r *exec.Fake) {
				r.AddPath("dnf", "/usr/bin/dnf")
			},
			want: PkgMgrDNF,
		},
		{
			name:  "none found -> unknown",
			os:    OSLinux,
			arch:  ArchX86_64,
			setup: func(*exec.Fake) {},
			want:  PkgMgrUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := exec.NewFake()
			tt.setup(r)

			if got := detectPkgMgr(r, tt.os, tt.arch); got != tt.want {
				t.Errorf("detectPkgMgr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDistroName(t *testing.T) {
	tests := []struct {
		name      string
		osRelease string
		missing   bool
		want      string
	}{
		{name: "ubuntu", osRelease: "ID=ubuntu\n", want: "ubuntu"},
		{name: "quoted ID", osRelease: `ID="cachyos"` + "\n", want: "cachyos"},
		{name: "single-quoted ID", osRelease: `ID='cachyos'` + "\n", want: "cachyos"},
		{name: "fedora-asahi-remix", osRelease: "ID=fedora-asahi-remix\n", want: "fedora-asahi-remix"},
		{name: "missing file -> unknown", missing: true, want: "unknown"},
		{name: "no ID field -> unknown", osRelease: "NAME=Test\n", want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "missing")
			if !tt.missing {
				path = writeOSRelease(t, tt.osRelease)
			}

			if got := distroName(path); got != tt.want {
				t.Errorf("distroName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOSReleaseCodename(t *testing.T) {
	tests := []struct {
		name      string
		osRelease string
		want      string
	}{
		{
			name:      "VERSION_CODENAME present",
			osRelease: "ID=ubuntu\nVERSION_CODENAME=noble\n",
			want:      "noble",
		},
		{
			name:      "only UBUNTU_CODENAME present",
			osRelease: "ID=ubuntu\nUBUNTU_CODENAME=jammy\n",
			want:      "jammy",
		},
		{
			name:      "both present -> VERSION_CODENAME wins",
			osRelease: "ID=ubuntu\nVERSION_CODENAME=noble\nUBUNTU_CODENAME=jammy\n",
			want:      "noble",
		},
		{
			name:      "neither present -> empty",
			osRelease: "ID=arch\nID_LIKE=arch\n",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeOSRelease(t, tt.osRelease)

			if got := osReleaseCodename(path); got != tt.want {
				t.Errorf("osReleaseCodename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseOSRelease_PermissionDenied_GracefulFallback(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission checks do not apply")
	}

	path := writeOSRelease(t, "ID=arch\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	defer func() { _ = os.Chmod(path, 0o644) }()

	id, idLike := parseOSRelease(path)
	if id != "" || idLike != "" {
		t.Errorf("parseOSRelease() = (%q, %q), want (\"\", \"\") on permission error", id, idLike)
	}
}

func TestDetectContainer_DoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("detectContainer() panicked: %v", r)
		}
	}()

	got := detectContainer()
	switch got {
	case ContainerDocker, ContainerKubernetes, ContainerWSL, ContainerNone:
		// valid
	default:
		t.Errorf("detectContainer() = %q, want one of the canonical Container values", got)
	}
}

func TestDetect_Smoke(t *testing.T) {
	r := exec.NewFake()
	r.AddCommand("uname", []string{"-s"}, []byte("Linux\n"), nil)
	r.AddCommand("uname", []string{"-m"}, []byte("x86_64\n"), nil)
	r.AddPath("pacman", "/usr/bin/pacman")

	info := Detect(r)

	if info.Arch != ArchX86_64 {
		t.Errorf("Detect().Arch = %q, want %q", info.Arch, ArchX86_64)
	}
	if info.PkgMgr != PkgMgrPacman {
		t.Errorf("Detect().PkgMgr = %q, want %q", info.PkgMgr, PkgMgrPacman)
	}
	if info.Container == "" {
		t.Error("Detect().Container is empty, want a canonical value")
	}
}

func TestIsLinuxFamily(t *testing.T) {
	tests := []struct {
		os   OS
		want bool
	}{
		{OSDebian, true},
		{OSRedHat, true},
		{OSArch, true},
		{OSSUSE, true},
		{OSLinux, true},
		{OSWSL, true},
		{OSMacOS, false},
		{OSWindows, false},
		{OSUnknown, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.os), func(t *testing.T) {
			if got := IsLinuxFamily(tt.os); got != tt.want {
				t.Errorf("IsLinuxFamily(%q) = %v, want %v", tt.os, got, tt.want)
			}
		})
	}
}
