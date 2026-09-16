package brew

import (
	"errors"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// installerCalled reports whether the official Homebrew installer was invoked
// through f (a constant `bash -c <curl … Homebrew/install …>` call).
func installerCalled(f *exec.Fake) bool {
	for _, c := range f.CallsSnapshot() {
		if c.Name == "bash" && len(c.Args) == 2 && c.Args[0] == "-c" && strings.Contains(c.Args[1], "Homebrew/install") {
			return true
		}
	}
	return false
}

func TestApplicable(t *testing.T) {
	tests := []struct {
		name     string
		info     platform.Info
		homebrew string
		want     bool
	}{
		{"macOS always bootstraps", platform.Info{OS: platform.OSMacOS}, "", true},
		{"macOS ignores an explicit false", platform.Info{OS: platform.OSMacOS}, "false", true},
		{"linux default is a no-op", platform.Info{OS: platform.OSArch}, "", false},
		{"linux opt-in bootstraps linuxbrew", platform.Info{OS: platform.OSDebian}, "true", true},
		{"linux explicit false is a no-op", platform.Info{OS: platform.OSDebian}, "false", false},
		{"linux opt-in on arm64 is declined (no aarch64 bottles)", platform.Info{OS: platform.OSRedHat, Arch: platform.ArchARM64}, "true", false},
		{"linux opt-in on x86_64 still bootstraps", platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64}, "true", true},
		{"unknown never bootstraps", platform.Info{OS: platform.OSUnknown}, "true", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Applicable(tt.info, tt.homebrew); got != tt.want {
				t.Errorf("Applicable(%q, %q) = %v, want %v", tt.info.OS, tt.homebrew, got, tt.want)
			}
		})
	}
}

func TestBootstrap_MacOSInstallsWhenMissing(t *testing.T) {
	f := exec.NewFake()

	bootstrapped, err := Bootstrap(f, platform.Info{OS: platform.OSMacOS, Arch: platform.ArchARM64}, "")
	if err != nil {
		t.Fatalf("Bootstrap() err = %v, want nil", err)
	}
	if !bootstrapped {
		t.Fatal("Bootstrap() = false, want true (brew absent on macOS)")
	}
	if !installerCalled(f) {
		t.Errorf("installer not invoked; calls=%+v", f.CallsSnapshot())
	}
}

func TestBootstrap_NoOpWhenPresent(t *testing.T) {
	f := exec.NewFake()
	f.AddPath("brew", "/opt/homebrew/bin/brew")

	bootstrapped, err := Bootstrap(f, platform.Info{OS: platform.OSMacOS}, "")
	if err != nil || bootstrapped {
		t.Fatalf("Bootstrap() = (%v, %v), want (false, nil) when brew present", bootstrapped, err)
	}
	if installerCalled(f) {
		t.Error("installer must not run when brew is already on PATH")
	}
}

func TestBootstrap_LinuxDefaultNoOp(t *testing.T) {
	f := exec.NewFake()

	bootstrapped, err := Bootstrap(f, platform.Info{OS: platform.OSArch}, "")
	if err != nil || bootstrapped {
		t.Fatalf("Bootstrap() = (%v, %v), want (false, nil) on Linux without --homebrew=true", bootstrapped, err)
	}
	if installerCalled(f) {
		t.Error("installer must not run on Linux without --homebrew=true")
	}
}

func TestBootstrap_LinuxOptInInstallsLinuxbrew(t *testing.T) {
	f := exec.NewFake()

	bootstrapped, err := Bootstrap(f, platform.Info{OS: platform.OSDebian}, "true")
	if err != nil {
		t.Fatalf("Bootstrap() err = %v, want nil", err)
	}
	if !bootstrapped {
		t.Fatal("Bootstrap() = false, want true (linuxbrew opt-in)")
	}
	if !installerCalled(f) {
		t.Errorf("installer not invoked for linuxbrew opt-in; calls=%+v", f.CallsSnapshot())
	}
}

func TestBootstrap_InstallerFailurePropagates(t *testing.T) {
	f := exec.NewFake()
	f.RunErrors[exec.CommandKey("bash", "-c", installCommand)] = errors.New("boom")

	bootstrapped, err := Bootstrap(f, platform.Info{OS: platform.OSMacOS}, "")
	if err == nil {
		t.Fatal("Bootstrap() err = nil, want the installer failure")
	}
	if bootstrapped {
		t.Error("Bootstrap() = true on installer failure, want false")
	}
}

func TestBinDir(t *testing.T) {
	tests := []struct {
		info platform.Info
		want string
	}{
		{platform.Info{OS: platform.OSMacOS, Arch: platform.ArchARM64}, "/opt/homebrew/bin"},
		{platform.Info{OS: platform.OSMacOS, Arch: platform.ArchX86_64}, "/usr/local/bin"},
		{platform.Info{OS: platform.OSDebian}, "/home/linuxbrew/.linuxbrew/bin"},
		{platform.Info{OS: platform.OSArch}, "/home/linuxbrew/.linuxbrew/bin"},
	}
	for _, tt := range tests {
		if got := BinDir(tt.info); got != tt.want {
			t.Errorf("BinDir(%q/%q) = %q, want %q", tt.info.OS, tt.info.Arch, got, tt.want)
		}
	}
}
