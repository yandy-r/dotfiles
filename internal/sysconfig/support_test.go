package sysconfig

import (
	"errors"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestSupport_PerOS(t *testing.T) {
	cases := []struct {
		os   platform.OS
		want SupportLevel
	}{
		{platform.OSMacOS, SupportSupported},
		{platform.OSDebian, SupportSupported},
		{platform.OSRedHat, SupportSupported},
		{platform.OSArch, SupportSupported},
		{platform.OSSUSE, SupportSupported},
		{platform.OSLinux, SupportSupported},
		{platform.OSWSL, SupportSupported},
		{platform.OSWindows, SupportDeferred},
		{platform.OSUnknown, SupportUnsupported},
	}
	for _, tc := range cases {
		t.Run(string(tc.os), func(t *testing.T) {
			if got := Support(platform.Info{OS: tc.os}); got != tc.want {
				t.Errorf("Support(%q) = %v, want %v", tc.os, got, tc.want)
			}
		})
	}
}

// TestSupport_MatchesRegisteredConfigurers is the lockstep guard against drift
// between Support's hand-written OS classification and the Configurer set
// register.go's Default actually registers. It asserts, for every canonical
// platform.OS, that Support agrees with the real engine: an unregistered OS is
// Unsupported, a Configurer whose empty-Config Plan returns ErrNotImplemented is
// Deferred, and any other registered OS is Supported. If register.go gains or
// drops an OS without Support being updated, this fails loudly.
func TestSupport_MatchesRegisteredConfigurers(t *testing.T) {
	registered := Default(exec.NewFake(), "").configurers

	allOSes := []platform.OS{
		platform.OSMacOS, platform.OSDebian, platform.OSRedHat, platform.OSArch,
		platform.OSSUSE, platform.OSLinux, platform.OSWSL, platform.OSWindows,
		platform.OSUnknown,
	}
	for _, os := range allOSes {
		cfg, ok := registered[os]
		got := Support(platform.Info{OS: os})

		if !ok {
			if got != SupportUnsupported {
				t.Errorf("OS %q: not registered in Default but Support()=%v, want SupportUnsupported", os, got)
			}
			continue
		}

		want := SupportSupported
		if _, err := cfg.Plan(Config{}, platform.Info{OS: os}); errors.Is(err, ErrNotImplemented) {
			want = SupportDeferred
		}
		if got != want {
			t.Errorf("OS %q: Support()=%v, want %v (per register.go)", os, got, want)
		}
	}
}
