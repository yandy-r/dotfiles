package sysconfig

import (
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// Default constructs an Engine with the linux, darwin, and deferred windows
// Configurers registered, backed by r and a FileSystem rooted at root ("" =
// the real /). The six systemd-family OS values share one linuxConfigurer
// instance; OSWSL is treated as linux-family for parity. platform.OSUnknown
// is intentionally NOT registered, so Engine.Plan returns *UnsupportedOSError
// for it.
func Default(r exec.Runner, root string) *Engine {
	fs := NewOSFileSystem(root)
	linux := newLinuxConfigurer(r, fs)

	cfgs := map[platform.OS]Configurer{
		platform.OSMacOS:   newDarwinConfigurer(r, fs),
		platform.OSDebian:  linux,
		platform.OSRedHat:  linux,
		platform.OSArch:    linux,
		platform.OSSUSE:    linux,
		platform.OSLinux:   linux,
		platform.OSWSL:     linux,
		platform.OSWindows: newWindowsConfigurer(),
	}

	return NewEngine(cfgs, r, fs)
}
