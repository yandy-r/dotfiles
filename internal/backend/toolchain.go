package backend

import (
	"fmt"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// toolchainBackend installs packages via language toolchain managers (cargo, npm).
// Installs always run as the current user; this backend never invokes sudo.
type toolchainBackend struct {
	r exec.Runner
}

// newToolchainBackend returns a Backend that provisions toolchain packages via r.Run.
func newToolchainBackend(r exec.Runner) Backend {
	return &toolchainBackend{r: r}
}

// UnknownToolError is returned when pkg.Tool is not a supported toolchain manager.
type UnknownToolError struct {
	// Tool is the manifest Package.Tool value that has no registered installer.
	Tool string
}

// Error implements the error interface.
func (e *UnknownToolError) Error() string {
	return fmt.Sprintf("backend: unknown toolchain tool %q", e.Tool)
}

// installName returns the crate or npm package name passed to the toolchain install
// command. Names[Tool] overrides Name when the manifest supplies a tool-specific
// identifier (e.g. names = { cargo = "ripgrep" } with name = "rg").
func installName(pkg manifest.Package) string {
	if name, ok := pkg.Names[pkg.Tool]; ok && name != "" {
		return name
	}
	return pkg.Name
}

type toolchainInstall struct {
	cmd  string
	args []string
}

func toolchainInstallArgs(tool, name string) (toolchainInstall, error) {
	switch tool {
	case "cargo":
		return toolchainInstall{cmd: "cargo", args: []string{"install", name}}, nil
	case "npm":
		return toolchainInstall{cmd: "npm", args: []string{"install", "-g", name}}, nil
	default:
		return toolchainInstall{}, &UnknownToolError{Tool: tool}
	}
}

func toolchainUpdateArgs(tool, name string) (toolchainInstall, error) {
	switch tool {
	case "cargo":
		return toolchainInstall{cmd: "cargo", args: []string{"install", "--force", name}}, nil
	case "npm":
		return toolchainInstall{cmd: "npm", args: []string{"update", "-g", name}}, nil
	default:
		return toolchainInstall{}, &UnknownToolError{Tool: tool}
	}
}

func toolchainRemoveArgs(tool, name string) (toolchainInstall, error) {
	switch tool {
	case "cargo":
		return toolchainInstall{cmd: "cargo", args: []string{"uninstall", name}}, nil
	case "npm":
		return toolchainInstall{cmd: "npm", args: []string{"uninstall", "-g", name}}, nil
	default:
		return toolchainInstall{}, &UnknownToolError{Tool: tool}
	}
}

// reshim regenerates the mise shims after a toolchain package changes.
//
// When the toolchain manager is itself provided by mise (node/npm are declared
// in mise/config.toml), `npm install -g` drops its executable into the version's
// own bin dir — ~/.local/share/mise/installs/node/<v>/bin — which is NOT on
// PATH. Only the shim directory is, and mise creates a shim per binary at
// reshim time. Without this the package installs successfully yet never resolves
// on PATH, so Installed() keeps reporting it missing and every run reinstalls it.
//
// Best-effort by design: no mise on PATH means nothing to reshim, and a reshim
// failure must not fail an install that already succeeded. `mise reshim` is
// idempotent and harmless for a non-mise toolchain (e.g. rustup's cargo).
func (b *toolchainBackend) reshim() {
	if _, err := b.r.LookPath("mise"); err != nil {
		return
	}
	_ = b.r.Run("mise", "reshim")
}

// Install runs the toolchain manager install command for pkg.
func (b *toolchainBackend) Install(pkg manifest.Package, _ platform.Info) error {
	inst, err := toolchainInstallArgs(pkg.Tool, installName(pkg))
	if err != nil {
		return err
	}
	if err := b.r.Run(inst.cmd, inst.args...); err != nil {
		return fmt.Errorf("toolchain install %q via %s: %w", installName(pkg), pkg.Tool, err)
	}
	b.reshim()
	return nil
}

// Update updates the toolchain package in place (cargo install --force, npm
// update -g). Installs and updates always run as the current user; this backend
// never invokes sudo.
func (b *toolchainBackend) Update(pkg manifest.Package, _ platform.Info) error {
	inst, err := toolchainUpdateArgs(pkg.Tool, installName(pkg))
	if err != nil {
		return err
	}
	if err := b.r.Run(inst.cmd, inst.args...); err != nil {
		return fmt.Errorf("toolchain update %q via %s: %w", installName(pkg), pkg.Tool, err)
	}
	b.reshim()
	return nil
}

// Remove removes the toolchain package (cargo uninstall, npm uninstall -g),
// again user-only with no sudo.
func (b *toolchainBackend) Remove(pkg manifest.Package, _ platform.Info) error {
	inst, err := toolchainRemoveArgs(pkg.Tool, installName(pkg))
	if err != nil {
		return err
	}
	if err := b.r.Run(inst.cmd, inst.args...); err != nil {
		return fmt.Errorf("toolchain remove %q via %s: %w", installName(pkg), pkg.Tool, err)
	}
	b.reshim()
	return nil
}

// Installed reports whether the package is present. An unknown tool is a typed
// error (surfacing a misconfigured manifest). Otherwise it honors an explicit
// Provides/InstalledWhen probe, falling back to a PATH lookup of pkg.Name — the
// binary name — not installName's crate/npm install identifier, which can
// differ (crate "ripgrep" installs binary "rg").
func (b *toolchainBackend) Installed(pkg manifest.Package, _ platform.Info) (bool, error) {
	if _, err := toolchainInstallArgs(pkg.Tool, installName(pkg)); err != nil {
		return false, err
	}
	return probeInstalled(b.r, pkg)
}

var (
	_ Backend = (*toolchainBackend)(nil)
	_ Updater = (*toolchainBackend)(nil)
	_ Remover = (*toolchainBackend)(nil)
)
