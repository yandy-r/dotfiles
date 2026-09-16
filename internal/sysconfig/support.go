package sysconfig

import "github.com/yandy-r/dotfiles/internal/platform"

// SupportLevel classifies how much of dotf sysconfig applies to a host. It
// mirrors the Configurer set that Default registers (see register.go): the macOS
// and Linux families have a working Configurer (Supported), Windows is
// registered but returns ErrNotImplemented (Deferred), and every other OS —
// notably platform.OSUnknown — has no Configurer at all (Unsupported).
type SupportLevel int

// Support levels, ordered from least to most supported.
const (
	// SupportUnsupported: no Configurer is registered (platform.OSUnknown).
	SupportUnsupported SupportLevel = iota
	// SupportDeferred: a Configurer is registered but returns ErrNotImplemented
	// (Windows).
	SupportDeferred
	// SupportSupported: a working Configurer is registered (macOS plus the Linux
	// families debian/redhat/arch/suse/linux/wsl).
	SupportSupported
)

// Support classifies info's OS against the sysconfig Configurer registry. It is
// pure classification — it reads nothing and runs nothing — kept in lockstep
// with the OS set Default registers (TestSupport_MatchesRegisteredConfigurers
// guards the two against drift), so callers can ask "is this OS supported?"
// without building an Engine or priming sudo. The Linux membership reuses the
// canonical platform.IsLinuxFamily set, which is exactly the Linux family
// register.go maps to the shared linuxConfigurer.
func Support(info platform.Info) SupportLevel {
	switch {
	case info.OS == platform.OSWindows:
		return SupportDeferred
	case info.OS == platform.OSMacOS || platform.IsLinuxFamily(info.OS):
		return SupportSupported
	default:
		return SupportUnsupported
	}
}
