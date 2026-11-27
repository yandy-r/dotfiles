package manifest

import (
	"fmt"
	"path"
	"strings"
)

// Module is a named, describable setup bundle: an ordered pipeline of typed
// steps a user opts into via `dotf setup <name>`. Unlike a custom-script
// Package (a single arbitrary script), a Module is a sequence of CONSTRAINED
// steps — each dispatches to one of the shipped engines (symlink, pkg,
// sysconfig, custom-script), so `setup` is a typed orchestrator over existing
// primitives rather than a second free-form shell escape hatch.
type Module struct {
	// Name is the module's canonical identifier, the `dotf setup <name>`
	// positional. It must be unique across modules.
	Name string `toml:"name"`

	// Description is a human-readable summary shown in CLI help and usage
	// errors that list the available modules.
	Description string `toml:"description"`

	// OS restricts this module to a platform, using the same token vocabulary as
	// Package.OS ("macos", "windows", "linux", or a distro family). Empty means
	// every platform. `dotf setup` refuses an off-platform module rather than
	// running its steps.
	OS string `toml:"os"`

	// Steps is the ordered pipeline executed by `dotf setup`. Steps run in
	// declaration order; a failure is collected non-fatally and later steps
	// still run (mirroring the apply/install run-wide failure summary).
	Steps []ModuleStep `toml:"steps"`
}

// ModuleStep is one typed pipeline step. Exactly ONE of the four kind fields
// (Symlink, Pkg, Sysconfig, CustomScript) is set; the closed set is the same
// fence post_install draws around git-clone — it stops `setup` regressing into
// custom-script v2. Ports is auxiliary data meaningful only on a Sysconfig ==
// "firewall" step.
type ModuleStep struct {
	// Symlink is the repo-relative src of a built-in link-table entry to
	// provision via the internal/symlink engine.
	Symlink string `toml:"symlink"`

	// Pkg is the name of a manifest package to ensure installed via the
	// backend engine (probe-gated, idempotent). It must name a known package.
	Pkg string `toml:"pkg"`

	// Sysconfig is the sysconfig domain to apply (firewall folds in here). One
	// of firewall, timezone, locale, shell, hostname.
	Sysconfig string `toml:"sysconfig"`

	// CustomScript is a contained repo/overlay-relative script to run via the
	// custom-script backend (the same relative/no-absolute/no-".." containment
	// as a custom-script package's Script).
	CustomScript string `toml:"custom-script"`

	// Ports lists firewall ports to open; it is only valid together with a
	// Sysconfig == "firewall" step.
	Ports []string `toml:"ports"`

	// ScriptRoot is engine-internal provenance, NOT a user-settable field: the
	// containment root a CustomScript step is resolved against, mirroring
	// Package.ScriptRoot. DiscoverOverlays stamps it with the XDG user config
	// root (~/.config/dotf) on every step of an overlay-sourced module; it is
	// empty for base modules, which fall back to the repo root. `dotf setup`
	// copies it onto the synthetic custom-script package it builds for the
	// step. The `toml:"-"` tag keeps it out of the decoder, so strict
	// DisallowUnknownFields rejects a `script_root` key in a fragment.
	ScriptRoot string `toml:"-"`
}

// StepKind is the typed kind of a module step: exactly one per step.
type StepKind string

// The four canonical module-step kinds, matching the ModuleStep kind fields.
const (
	StepSymlink      StepKind = "symlink"
	StepPkg          StepKind = "pkg"
	StepSysconfig    StepKind = "sysconfig"
	StepCustomScript StepKind = "custom-script"
)

// stepKindTarget pairs a step's resolved kind with its target value (the
// script path, package name, symlink src, or sysconfig domain).
type stepKindTarget struct {
	Kind   StepKind
	Target string
}

// kinds returns the (kind, target) pair for each non-empty kind field. A valid
// step has exactly one; validateModule enforces that at parse time.
func (s ModuleStep) kinds() []stepKindTarget {
	var out []stepKindTarget
	if s.Symlink != "" {
		out = append(out, stepKindTarget{StepSymlink, s.Symlink})
	}
	if s.Pkg != "" {
		out = append(out, stepKindTarget{StepPkg, s.Pkg})
	}
	if s.Sysconfig != "" {
		out = append(out, stepKindTarget{StepSysconfig, s.Sysconfig})
	}
	if s.CustomScript != "" {
		out = append(out, stepKindTarget{StepCustomScript, s.CustomScript})
	}
	return out
}

// Resolve returns the single set kind and its target value. ok is false unless
// exactly one kind field is set — validateModule guarantees this on any parsed
// manifest, so a false ok signals a step that bypassed validation.
func (s ModuleStep) Resolve() (kind StepKind, target string, ok bool) {
	kts := s.kinds()
	if len(kts) != 1 {
		return "", "", false
	}
	return kts[0].Kind, kts[0].Target, true
}

// knownSysconfigDomains is the set of sysconfig domains a module step may name,
// kept in lockstep with the sysconfig engine's per-OS domains.
var knownSysconfigDomains = map[string]bool{
	"firewall": true,
	"timezone": true,
	"locale":   true,
	"shell":    true,
	"hostname": true,
}

// validateModules checks every [[module]] in m: names are required and unique,
// each step sets exactly one kind field, a pkg step names a known package
// (drawn from the pkgNames set Validate builds), a sysconfig step names a legal
// domain (with ports only on firewall), and a custom-script step is contained.
// It is called from Validate with the fully-populated pkgNames set so a module
// may reference any package the manifest declares regardless of order.
func validateModules(m *Manifest, pkgNames map[string]bool) []error {
	var errs []error
	seen := make(map[string]bool, len(m.Modules))
	for _, mod := range m.Modules {
		errs = append(errs, validateModule(mod, pkgNames, seen)...)
	}
	return errs
}

// validateModule checks a single module and records its name for duplicate
// detection across the module set.
func validateModule(mod Module, pkgNames, seen map[string]bool) []error {
	var errs []error

	if mod.Name == "" {
		return append(errs, &ValidationError{Msg: "module missing required name"})
	}
	if seen[mod.Name] {
		errs = append(errs, &ValidationError{Module: mod.Name, Msg: "duplicate module name"})
	}
	seen[mod.Name] = true

	if mod.OS != "" && !knownPackageOS[mod.OS] {
		errs = append(errs, &ValidationError{Module: mod.Name, Msg: fmt.Sprintf("os %q must be one of: arch, debian, linux, macos, redhat, suse, windows", mod.OS)})
	}

	for i, step := range mod.Steps {
		errs = append(errs, validateModuleStep(mod.Name, i, step, pkgNames)...)
	}
	return errs
}

// validateModuleStep enforces the exactly-one-kind rule and the per-kind shape
// rules for a single step (step index is 0-based; messages are 1-based).
func validateModuleStep(modName string, i int, step ModuleStep, pkgNames map[string]bool) []error {
	var errs []error

	kts := step.kinds()
	switch len(kts) {
	case 0:
		return append(errs, &ValidationError{
			Module: modName,
			Msg:    fmt.Sprintf("step %d must set exactly one of symlink, pkg, sysconfig, custom-script", i+1),
		})
	case 1:
		// exactly one kind — validate its shape below.
	default:
		set := make([]string, len(kts))
		for j, kt := range kts {
			set[j] = string(kt.Kind)
		}
		return append(errs, &ValidationError{
			Module: modName,
			Msg:    fmt.Sprintf("step %d must set exactly one kind field, got %d (%s)", i+1, len(kts), strings.Join(set, ", ")),
		})
	}

	kt := kts[0]
	switch kt.Kind {
	case StepPkg:
		if !pkgNames[kt.Target] {
			errs = append(errs, &ValidationError{
				Module: modName,
				Msg:    fmt.Sprintf("step %d pkg %q names an unknown package", i+1, kt.Target),
			})
		}
	case StepSysconfig:
		if !knownSysconfigDomains[kt.Target] {
			errs = append(errs, &ValidationError{
				Module: modName,
				Msg:    fmt.Sprintf("step %d sysconfig domain %q must be one of firewall, hostname, locale, shell, timezone", i+1, kt.Target),
			})
		}
	case StepCustomScript:
		errs = append(errs, validateModuleScript(modName, i, kt.Target)...)
	case StepSymlink:
		// The kind field being non-empty already guarantees a non-empty src;
		// the concrete link target is resolved against the link table at apply.
	}

	// ports is auxiliary to a firewall step only; reject it on any other kind
	// or domain (mirrors post_install being git-clone-only).
	if len(step.Ports) > 0 && (kt.Kind != StepSysconfig || kt.Target != "firewall") {
		errs = append(errs, &ValidationError{
			Module: modName,
			Msg:    fmt.Sprintf("step %d ports is only valid on a sysconfig firewall step", i+1),
		})
	}

	return errs
}

// validateModuleScript applies the same relative/no-absolute/no-".." containment
// invariant to a module custom-script step that validateScriptPath applies to a
// custom-script package's Script — defense-in-depth before the backend re-checks
// against the resolved containment root at run time.
func validateModuleScript(modName string, i int, script string) []error {
	var errs []error

	if path.IsAbs(script) {
		errs = append(errs, &ValidationError{
			Module: modName,
			Msg:    fmt.Sprintf("step %d custom-script %q must be relative", i+1, script),
		})
	}
	for _, elem := range strings.Split(path.Clean(script), "/") {
		if elem == ".." {
			errs = append(errs, &ValidationError{
				Module: modName,
				Msg:    fmt.Sprintf("step %d custom-script %q must not contain ..", i+1, script),
			})
			break
		}
	}

	return errs
}
