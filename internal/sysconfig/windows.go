package sysconfig

import "github.com/yandy-r/dotfiles/internal/platform"

// windowsConfigurer is the deferred Windows stub. Every sysconfig operation is
// an argv shell-out (no unix-only syscalls), so this is a plain runtime branch
// with NO build tag — the stub compiles and is exercised on Linux CI, and
// "go build ./..." stays green on every platform.
//
// The eventual Windows implementation would run: Rename-Computer -NewName
// <name> for hostname, Set-TimeZone -Id <windows-tz-id> for timezone, and
// Set-NetFirewallProfile / Windows Defender Firewall cmdlets for the firewall.
// Locale and shell have no direct Windows analogue in this model.
type windowsConfigurer struct{}

// newWindowsConfigurer constructs the deferred Windows stub Configurer.
func newWindowsConfigurer() Configurer {
	return &windowsConfigurer{}
}

// Plan returns an empty Plan and ErrNotImplemented: Windows sysconfig is
// deferred.
func (c *windowsConfigurer) Plan(_ Config, _ platform.Info) (Plan, error) {
	return Plan{}, ErrNotImplemented
}

var _ Configurer = (*windowsConfigurer)(nil)
