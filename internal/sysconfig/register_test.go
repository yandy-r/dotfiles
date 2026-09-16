package sysconfig

import (
	"errors"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestDefault_RegistersEveryFamily(t *testing.T) {
	eng := Default(exec.NewFake(), "")

	// Every systemd-family + macOS + windows OS value must dispatch to a
	// registered Configurer (i.e. NOT return *UnsupportedOSError).
	registered := []platform.OS{
		platform.OSMacOS,
		platform.OSDebian,
		platform.OSRedHat,
		platform.OSArch,
		platform.OSSUSE,
		platform.OSLinux,
		platform.OSWSL,
		platform.OSWindows,
	}

	for _, os := range registered {
		t.Run(string(os), func(t *testing.T) {
			// An all-empty Config leaves every domain unmanaged, so a
			// registered configurer returns a no-op Plan (windows returns
			// ErrNotImplemented). The key assertion: NOT *UnsupportedOSError.
			_, err := eng.Plan(Config{}, platform.Info{OS: os})
			var unsupported *UnsupportedOSError
			if errors.As(err, &unsupported) {
				t.Fatalf("Plan(%s) = *UnsupportedOSError, want a registered configurer", os)
			}
			if os == platform.OSWindows && !errors.Is(err, ErrNotImplemented) {
				t.Errorf("Plan(windows) error = %v, want ErrNotImplemented", err)
			}
			if os != platform.OSWindows && err != nil {
				t.Errorf("Plan(%s) error = %v, want nil for an empty Config", os, err)
			}
		})
	}
}

func TestDefault_UnknownOSUnsupported(t *testing.T) {
	eng := Default(exec.NewFake(), "")

	_, err := eng.Plan(Config{}, platform.Info{OS: platform.OSUnknown})
	var unsupported *UnsupportedOSError
	if !errors.As(err, &unsupported) {
		t.Fatalf("Plan(OSUnknown) error = %v, want *UnsupportedOSError", err)
	}
}

func TestDefault_LinuxFamilySharesInstance(t *testing.T) {
	// Same shared linuxConfigurer must serve every systemd-family OS: an
	// arch-vs-debian firewall install argv proves info (not the instance)
	// drives distro selection.
	family := map[platform.OS][]string{
		platform.OSArch:   {"sudo", "pacman", "-S", "--noconfirm", "--needed", "ufw"},
		platform.OSDebian: {"sudo", "apt-get", "install", "-y", "ufw"},
		platform.OSWSL:    {"sudo", "apt-get", "install", "-y", "ufw"},
	}

	for os, wantInstall := range family {
		t.Run(string(os), func(t *testing.T) {
			eng := Default(exec.NewFake(), t.TempDir())
			plan, err := eng.Plan(Config{Firewall: FirewallAuto, User: "yandy"}, platform.Info{OS: os})
			if err != nil {
				t.Fatalf("Plan(%s) error = %v", os, err)
			}
			if len(plan.Steps) == 0 {
				t.Fatalf("Plan(%s) produced no firewall steps", os)
			}
			assertStepArgv(t, plan.Steps[0], wantInstall)
		})
	}
}
