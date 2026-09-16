package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// customScriptBackend executes repo-relative install scripts declared in
// manifest Package.Script fields.
type customScriptBackend struct {
	r        exec.Runner
	repoRoot string
}

// newCustomScriptBackend constructs a custom-script backend rooted at
// repoRoot. When repoRoot is empty, DOTFILES_ROOT is used.
func newCustomScriptBackend(r exec.Runner, repoRoot string) *customScriptBackend {
	return &customScriptBackend{r: r, repoRoot: repoRoot}
}

// Install resolves pkg.Script and runs it with no lifecycle flag (the default
// install action). See runScript for the containment/NON_INTERACTIVE rationale.
func (b *customScriptBackend) Install(pkg manifest.Package, _ platform.Info) error {
	return b.runScript(pkg)
}

// Update runs the script with a trailing --update, gated on the package
// declaring the "update" action. dotf must never pass --update to a script that
// cannot parse it (it would error or, worse, reinstall), so an undeclared action
// fails fast with ActionUnsupportedError rather than running the script.
func (b *customScriptBackend) Update(pkg manifest.Package, _ platform.Info) error {
	if !hasAction(pkg, "update") {
		return &ActionUnsupportedError{Action: ActionUpdate, Backend: manifest.BackendCustomScript, Package: pkg.Name}
	}
	return b.runScript(pkg, "--update")
}

// Remove runs the script with a trailing --uninstall, gated on the package
// declaring the "remove" action. Same capability fence as Update: an undeclared
// action fails fast without running the script.
func (b *customScriptBackend) Remove(pkg manifest.Package, _ platform.Info) error {
	if !hasAction(pkg, "remove") {
		return &ActionUnsupportedError{Action: ActionRemove, Backend: manifest.BackendCustomScript, Package: pkg.Name}
	}
	return b.runScript(pkg, "--uninstall")
}

// hasAction reports whether pkg declares the given lifecycle action in its
// Actions field. "install" is always supported and need not be declared; only
// "update"/"remove" gate the passthrough flags.
func hasAction(pkg manifest.Package, action string) bool {
	return slices.Contains(pkg.Actions, action)
}

// runScript resolves pkg.Script under the containment root and executes it via
// bash with NON_INTERACTIVE=true exported and the action's trailing flag(s)
// appended. Install passes none; Update passes --update; Remove passes
// --uninstall. Containment (resolveScript → containScript) and the
// NON_INTERACTIVE env are UNCHANGED across all three verbs — only the trailing
// argv differs — so the passthrough can never escape the script root or block on
// a `read` prompt.
//
// dotf owns interactivity: the wrapped bin/install-*.sh scripts gate their
// confirmation prompts on NON_INTERACTIVE (auto-approving) so they never block on
// a `read`, whether apply runs on a TTY or headless in CI. FORCE_INSTALL is
// deliberately left unset, so a converged host stays idempotent. Idempotency is
// also enforced one level up, at the engine: the cmd layer consults Installed()
// (via probeInstalled) and skips Install()/Update()/Remove() on a converged or
// absent host per the action's gate.
func (b *customScriptBackend) runScript(pkg manifest.Package, extra ...string) error {
	scriptPath, err := b.resolveScript(pkg)
	if err != nil {
		return fmt.Errorf("custom-script: package %q: %w", pkg.Name, err)
	}

	argv := append([]string{"NON_INTERACTIVE=true", "bash", scriptPath}, extra...)
	if err := b.r.Run("env", argv...); err != nil {
		return fmt.Errorf("custom-script: package %q: run %q: %w", pkg.Name, scriptPath, err)
	}
	return nil
}

// Installed reports whether the package is present, honoring an explicit
// Provides/InstalledWhen probe and falling back to a PATH lookup of pkg.Name.
func (b *customScriptBackend) Installed(pkg manifest.Package, _ platform.Info) (bool, error) {
	return probeInstalled(b.r, pkg)
}

// resolveScript resolves pkg.Script to an absolute path contained within its
// containment root. The root is pkg.ScriptRoot when set — an XDG overlay
// package, stamped with ~/.config/dotf so a user-local script under
// ~/.config/dotf/bin/ stays contained there — and otherwise the backend's repo
// root (base-manifest packages). The clean / reject-absolute / reject-".." /
// filepath.Rel containment is identical for both roots, so a user-local script
// keeps the same no-absolute-path, no-parent-escape invariant as a repo script.
func (b *customScriptBackend) resolveScript(pkg manifest.Package) (string, error) {
	root, err := b.containmentRoot(pkg)
	if err != nil {
		return "", err
	}
	return containScript(root, pkg.Script)
}

// containmentRoot returns the absolute root that pkg's Script must resolve
// within: the overlay-stamped pkg.ScriptRoot when present, otherwise the
// backend's repo root (with its DOTFILES_ROOT fallback).
func (b *customScriptBackend) containmentRoot(pkg manifest.Package) (string, error) {
	if pkg.ScriptRoot != "" {
		abs, err := filepath.Abs(pkg.ScriptRoot)
		if err != nil {
			return "", fmt.Errorf("resolve script root: %w", err)
		}
		return abs, nil
	}
	return b.repoRootAbs()
}

// containScript resolves script against root, rejecting an empty, absolute, or
// parent-escaping path and confirming the joined result stays under root. It is
// root-parameterized so the identical fence guards both the repo root and an
// overlay's user config root.
func containScript(root, script string) (string, error) {
	if script == "" {
		return "", fmt.Errorf("script path is empty")
	}
	if filepath.IsAbs(script) {
		return "", fmt.Errorf("script %q must be relative to the script root", script)
	}

	clean := filepath.Clean(script)
	for _, elem := range strings.Split(clean, string(filepath.Separator)) {
		if elem == ".." {
			return "", fmt.Errorf("script %q must not contain parent segments", script)
		}
	}

	resolved := filepath.Clean(filepath.Join(root, clean))
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", fmt.Errorf("resolve script %q: %w", script, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("script %q escapes the script root", script)
	}

	return resolved, nil
}

func (b *customScriptBackend) repoRootAbs() (string, error) {
	root := b.repoRoot
	if root == "" {
		root = os.Getenv("DOTFILES_ROOT")
	}
	if root == "" {
		return "", fmt.Errorf("repo root not configured")
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	return abs, nil
}

var (
	_ Backend = (*customScriptBackend)(nil)
	_ Updater = (*customScriptBackend)(nil)
	_ Remover = (*customScriptBackend)(nil)
)
