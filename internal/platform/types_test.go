package platform

import (
	"reflect"
	"testing"
)

func TestOS_ReleaseOS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		os   OS
		want string
	}{
		{OSMacOS, "darwin"},
		{OSDebian, "linux"},
		{OSRedHat, "linux"},
		{OSArch, "linux"},
		{OSSUSE, "linux"},
		{OSLinux, "linux"},
		{OSWSL, "linux"},
		{OSWindows, "windows"},
		{OSUnknown, ""},
		{OS("made-up"), ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.os), func(t *testing.T) {
			t.Parallel()
			if got := tt.os.ReleaseOS(); got != tt.want {
				t.Errorf("OS(%q).ReleaseOS() = %q, want %q", tt.os, got, tt.want)
			}
		})
	}
}

func TestOS_ReleaseOSAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		os   OS
		want []string
	}{
		{OSMacOS, []string{"darwin", "macos", "osx", "apple"}},
		{OSArch, []string{"linux"}},
		{OSDebian, []string{"linux"}},
		{OSWSL, []string{"linux"}},
		{OSWindows, []string{"windows", "win"}},
		{OSUnknown, nil},
	}

	for _, tt := range tests {
		t.Run(string(tt.os), func(t *testing.T) {
			t.Parallel()
			if got := tt.os.ReleaseOSAliases(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OS(%q).ReleaseOSAliases() = %v, want %v", tt.os, got, tt.want)
			}
		})
	}
}

func TestArch_ReleaseArchAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		arch Arch
		want []string
	}{
		{ArchX86_64, []string{"x86_64", "amd64", "x64"}},
		{ArchARM64, []string{"aarch64", "arm64"}},
		{ArchARM, []string{"arm", "armv7", "armv6", "armhf"}},
		{Arch("riscv64"), []string{"riscv64"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.arch), func(t *testing.T) {
			t.Parallel()
			if got := tt.arch.ReleaseArchAliases(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Arch(%q).ReleaseArchAliases() = %v, want %v", tt.arch, got, tt.want)
			}
		})
	}
}
