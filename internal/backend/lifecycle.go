package backend

import (
	"fmt"

	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// Action is what the caller wants done to a package: the lifecycle verb threaded
// from the cmd layer through the Engine to the backend. The zero value is
// ActionInstall so existing call sites that don't mention an action are
// unchanged — Action "rides alongside" the installed-state probe: Installed()
// answers "is it here?", Action answers "what do I want done?".
type Action int

const (
	// ActionInstall provisions a package. It is the zero value, so a caller
	// that never mentions an action defaults to install.
	ActionInstall Action = iota
	// ActionUpdate updates an already-installed package in place.
	ActionUpdate
	// ActionRemove removes an installed package.
	ActionRemove
)

// String renders the action as its CLI verb ("install" | "update" | "remove").
func (a Action) String() string {
	switch a {
	case ActionUpdate:
		return "update"
	case ActionRemove:
		return "remove"
	default:
		return "install"
	}
}

// Updater is implemented by backends that can update an already-installed
// package in place. It is an optional capability discovered by type assertion in
// Engine.Update, mirroring platformGated/lockable — the Backend contract is never
// widened. A backend without it yields a clean ActionUnsupportedError.
type Updater interface {
	Update(pkg manifest.Package, info platform.Info) error
}

// Remover is implemented by backends that can remove an installed package. Same
// optional-interface shape as Updater; a backend without it yields
// ActionUnsupportedError.
type Remover interface {
	Remove(pkg manifest.Package, info platform.Info) error
}

// ActionUnsupportedError is the fail-fast returned when a backend does not
// implement the requested lifecycle action (mirrors the "not lockable" error). It
// names the action, backend, and package so the caller can report exactly what
// was refused.
type ActionUnsupportedError struct {
	// Action is the lifecycle verb the backend does not support.
	Action Action
	// Backend is the manifest.Backend value that lacks the capability.
	Backend manifest.Backend
	// Package is the offending package's name.
	Package string
}

// Error implements the error interface.
func (e *ActionUnsupportedError) Error() string {
	return fmt.Sprintf("backend: %s not supported for backend %q (package %q)", e.Action, e.Backend, e.Package)
}

// Update updates pkg in place, dispatching to the backend registered for
// pkg.Backend. It honors the OS gate uniformly with Install (off-platform is a
// no-op) and fails fast with ActionUnsupportedError when the backend does not
// implement Updater.
func (e *Engine) Update(pkg manifest.Package, info platform.Info) error {
	b, ok := e.backends[pkg.Backend]
	if !ok {
		return &UnknownBackendError{Backend: pkg.Backend}
	}
	if !osGateAllows(pkg.OS, info.OS) {
		return nil
	}
	if u, ok := b.(Updater); ok {
		return u.Update(pkg, info)
	}
	return &ActionUnsupportedError{Action: ActionUpdate, Backend: pkg.Backend, Package: pkg.Name}
}

// Remove removes pkg, dispatching to the backend registered for pkg.Backend. It
// is symmetric with Update: OS-gated no-op off-platform, ActionUnsupportedError
// when the backend does not implement Remover.
func (e *Engine) Remove(pkg manifest.Package, info platform.Info) error {
	b, ok := e.backends[pkg.Backend]
	if !ok {
		return &UnknownBackendError{Backend: pkg.Backend}
	}
	if !osGateAllows(pkg.OS, info.OS) {
		return nil
	}
	if rm, ok := b.(Remover); ok {
		return rm.Remove(pkg, info)
	}
	return &ActionUnsupportedError{Action: ActionRemove, Backend: pkg.Backend, Package: pkg.Name}
}

// Perform is the action-parametric entry the cmd layer calls; it routes to the
// matching Engine method. ActionInstall (including the zero value) routes to the
// existing Engine.Install path, so callers that don't opt into a lifecycle are
// unaffected.
func (e *Engine) Perform(action Action, pkg manifest.Package, info platform.Info) error {
	switch action {
	case ActionUpdate:
		return e.Update(pkg, info)
	case ActionRemove:
		return e.Remove(pkg, info)
	default:
		return e.Install(pkg, info)
	}
}
