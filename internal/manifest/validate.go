package manifest

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/yandy-r/dotfiles/internal/platform"
)

// ValidationError describes a single validation failure found in a
// Manifest, identifying the offending package or intent by name where
// possible.
type ValidationError struct {
	// Package, when non-empty, names the [[package]] entry that failed
	// validation.
	Package string

	// Intent, when non-empty, names the [intents.<name>] entry that failed
	// validation.
	Intent string

	// Module, when non-empty, names the [[module]] entry that failed
	// validation.
	Module string

	// Msg is a human-readable description of the failure.
	Msg string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	switch {
	case e.Package != "":
		return fmt.Sprintf("manifest: package %q: %s", e.Package, e.Msg)
	case e.Intent != "":
		return fmt.Sprintf("manifest: intent %q: %s", e.Intent, e.Msg)
	case e.Module != "":
		return fmt.Sprintf("manifest: module %q: %s", e.Module, e.Msg)
	default:
		return fmt.Sprintf("manifest: %s", e.Msg)
	}
}

// sha256Pattern matches a 64-char lowercase hex string, the expected shape
// of a Package.SHA256 checksum.
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// repoSlugPattern matches an "owner/repo" GitHub slug, the expected shape of a
// Package.Repo when the release archive is fetched from a GitHub release.
var repoSlugPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*/[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// validLibc is the set of accepted github-release Match.Libc values. Empty
// means "no preference" (defaults to gnu at match time).
var validLibc = map[string]bool{
	"":     true,
	"gnu":  true,
	"musl": true,
}

// knownPkgMgrs is the set of platform.PkgMgr string values valid as a
// Package.Names key. platform.PkgMgrUnknown is deliberately excluded — it
// is a detection fallback, never a legitimate manifest key.
var knownPkgMgrs = map[string]bool{
	string(platform.PkgMgrHomebrew): true,
	string(platform.PkgMgrAPT):      true,
	string(platform.PkgMgrDNF):      true,
	string(platform.PkgMgrYUM):      true,
	string(platform.PkgMgrPacman):   true,
	string(platform.PkgMgrZypper):   true,
}

// knownToolchainTools is the set of toolchain managers valid as a toolchain
// package's tool value and as its Names keys (toolchain packages key names
// by tool, not by package manager). internal/backend's toolchainInstallArgs
// must be able to execute every entry here; that agreement is guarded by
// TestToolchainInstallArgs_CoversManifestTools in internal/backend.
var knownToolchainTools = map[string]bool{
	"cargo": true,
	"npm":   true,
}

// ToolchainTools returns the toolchain manager names accepted for
// backend = "toolchain" packages, sorted for deterministic messages and
// iteration.
func ToolchainTools() []string {
	tools := make([]string, 0, len(knownToolchainTools))
	for tool := range knownToolchainTools {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	return tools
}

// Validate checks m against the manifest's structural and semantic rules,
// aggregating every failure found (rather than stopping at the first) via
// errors.Join so callers see the full picture in one pass. It returns nil
// when m is valid.
//
// Validate is also where intent contents are checked explicitly: go-toml's
// DisallowUnknownFields only guards struct-shaped fields, and is a no-op
// for the map[string]Intent that backs Manifest.Intents, so intent
// correctness (here, that every referenced package name actually exists)
// can only be enforced here, not by the decoder.
func Validate(m *Manifest) error {
	var errs []error

	names := make(map[string]bool, len(m.Packages))
	for _, p := range m.Packages {
		errs = append(errs, validatePackage(p)...)

		if names[p.Name] {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "duplicate package name"})
		}
		names[p.Name] = true
	}

	for intentName, intent := range m.Intents {
		for _, ref := range intent.Packages {
			if !names[ref] {
				errs = append(errs, &ValidationError{
					Intent: intentName,
					Msg:    fmt.Sprintf("references unknown package %q", ref),
				})
			}
		}
	}

	for _, s := range m.Symlinks {
		errs = append(errs, validateSymlink(s)...)
	}

	errs = append(errs, validateSysconfig(m.Sysconfig)...)

	errs = append(errs, validateModules(m, names)...)

	return errors.Join(errs...)
}

// validFirewallModes is the set of accepted [sysconfig] firewall values. An
// empty value is allowed and means the firewall domain is left unmanaged.
var validFirewallModes = map[string]bool{
	"":          true,
	"ufw":       true,
	"firewalld": true,
	"auto":      true,
	"off":       true,
}

// validSSHServerModes is the set of accepted [sysconfig] ssh_server values. An
// empty value is allowed and means the SSH server domain is left unmanaged.
var validSSHServerModes = map[string]bool{
	"":    true,
	"on":  true,
	"off": true,
}

// validateSysconfig checks the optional [sysconfig] table. It statically
// validates only the firewall enum; timezone/locale/shell shapes are
// validated at runtime against the host (zoneinfo, locale -a, /etc/shells),
// so no static check applies to them. It returns no errors when s is nil.
func validateSysconfig(s *Sysconfig) []error {
	if s == nil {
		return nil
	}

	var errs []error
	if !validFirewallModes[s.Firewall] {
		errs = append(errs, &ValidationError{
			Msg: fmt.Sprintf("sysconfig firewall %q must be one of ufw, firewalld, auto, off", s.Firewall),
		})
	}
	if !validSSHServerModes[s.SSHServer] {
		errs = append(errs, &ValidationError{
			Msg: fmt.Sprintf("sysconfig ssh_server %q must be one of on, off", s.SSHServer),
		})
	}
	return errs
}

// validatePackage checks a single Package against its backend's required
// fields, its Names keys, and its SHA256 shape (when set).
func validatePackage(p Package) []error {
	var errs []error

	if p.Name == "" {
		return append(errs, &ValidationError{Msg: "package missing required name"})
	}

	if !p.Backend.Valid() {
		return append(errs, &ValidationError{
			Package: p.Name,
			Msg:     fmt.Sprintf("unknown backend %q", p.Backend),
		})
	}

	switch p.Backend {
	case BackendGitHubRelease:
		errs = append(errs, validateGitHubRelease(p)...)
	case BackendScriptURL:
		if p.URL == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "script-url backend requires url"})
		} else if u, parseErr := url.Parse(p.URL); parseErr != nil || u.Scheme != "https" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "script-url backend requires https url"})
		}
		// sha256 is optional for script-url: a rotating vendor installer (e.g.
		// starship's install.sh) cannot be pinned because its hash rotates and
		// would fail the checksum closed on every fresh machine. A malformed
		// sha256 is still rejected by the shared sha256Pattern check below; an
		// absent one relies on HTTPS transport instead.
	case BackendCustomScript:
		if p.Script == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "custom-script backend requires script"})
		} else {
			errs = append(errs, validateScriptPath(p)...)
		}
	case BackendToolchain:
		if p.Tool == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "toolchain backend requires tool"})
		} else if !knownToolchainTools[p.Tool] {
			errs = append(errs, &ValidationError{
				Package: p.Name,
				Msg:     fmt.Sprintf("toolchain backend tool %q must be one of: %s", p.Tool, strings.Join(ToolchainTools(), ", ")),
			})
		}
	case BackendRepo, BackendAPTRepo:
		errs = append(errs, validateRepo(p)...)
	case BackendGitClone:
		errs = append(errs, validateGitClone(p)...)
	case BackendFont:
		errs = append(errs, validateFont(p)...)
	case BackendPkg, BackendAUR:
		// No backend-specific required fields beyond what's checked below.
	}

	// Names keys are backend-dependent: toolchain packages key the install
	// identifier by tool (names = { cargo = "ripgrep" }); every other
	// backend keys by package manager. Values are checked here too: an empty
	// name is the boundary check for a typo'd `dnf = ""`, which would
	// otherwise reach a backend and interpolate into `install ""`. No consumer
	// treats an empty value as meaningful — they all read it as absent.
	for k, name := range p.Names {
		if strings.TrimSpace(name) == "" {
			errs = append(errs, &ValidationError{
				Package: p.Name,
				Msg:     fmt.Sprintf("names[%q] must be non-empty", k),
			})
		}
		if p.Backend == BackendToolchain {
			if !knownToolchainTools[k] {
				errs = append(errs, &ValidationError{
					Package: p.Name,
					Msg:     fmt.Sprintf("unknown toolchain tool key %q in names", k),
				})
			}
			continue
		}
		if !knownPkgMgrs[k] {
			errs = append(errs, &ValidationError{
				Package: p.Name,
				Msg:     fmt.Sprintf("unknown package manager key %q in names", k),
			})
		}
	}

	if p.SHA256 != "" && !sha256Pattern.MatchString(p.SHA256) {
		errs = append(errs, &ValidationError{
			Package: p.Name,
			Msg:     fmt.Sprintf("sha256 %q is not 64-char lowercase hex", p.SHA256),
		})
	}

	errs = append(errs, validateProbe(p)...)

	errs = append(errs, validatePackageOS(p)...)

	errs = append(errs, validatePostInstall(p)...)

	errs = append(errs, validateClass(p)...)

	errs = append(errs, validateActions(p)...)

	errs = append(errs, validateArgs(p)...)

	return errs
}

// validateArgs checks a package's optional args. args is valid ONLY on the
// script-url backend, where it is the installer's argv (e.g. --yes to drive a
// vendor installer non-interactively). Rejecting it elsewhere stops it being
// read as a general "pass flags to whatever runs" escape hatch. An empty
// element is rejected: a vendor installer that receives "" generally treats it
// as a malformed flag, and it is always a manifest typo.
func validateArgs(p Package) []error {
	if len(p.Args) == 0 {
		return nil
	}

	var errs []error

	if p.Backend != BackendScriptURL {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "args is only valid for the script-url backend"})
	}

	for _, arg := range p.Args {
		if strings.TrimSpace(arg) == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "args must not contain an empty element"})
			break
		}
	}

	return errs
}

// knownActions is the set of accepted Package.Actions tokens: the three
// lifecycle verbs. "install" is always supported and need not be declared, but
// it is accepted here so a script that spells out its full capability set is not
// rejected.
var knownActions = map[string]bool{"install": true, "update": true, "remove": true}

// validateActions checks a package's optional actions capability list. actions
// is valid ONLY on the custom-script backend (it gates the --update/--uninstall
// passthrough; the native backends derive update/remove from the package
// authority), mirroring post_install being git-clone-only. Every entry must be a
// known lifecycle verb; an unknown value fails fast listing the legal set. An
// empty/absent list is allowed (install-only script).
func validateActions(p Package) []error {
	if len(p.Actions) == 0 {
		return nil
	}

	var errs []error

	if p.Backend != BackendCustomScript {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "actions is only valid for the custom-script backend"})
	}

	for _, action := range p.Actions {
		if !knownActions[action] {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("action %q must be one of install, update, remove", action)})
		}
	}

	return errs
}

// validateGitClone checks a git-clone package: URL is required and must be https
// (reusing the script-url https rule — the git remote is fetched over https),
// and Dest is required and $HOME-contained (not absolute, no ".." segment,
// mirroring validateSymlink). Ref is optional (empty = the remote's default
// branch). git-clone is intentionally excluded from pathInstallBackends, so
// validateProbe already rejects provides/installed_when on it — the dest is the
// intrinsic probe.
func validateGitClone(p Package) []error {
	var errs []error

	if p.URL == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "git-clone backend requires url"})
	} else if u, parseErr := url.Parse(p.URL); parseErr != nil || u.Scheme != "https" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "git-clone backend requires https url"})
	}

	if p.Dest == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "git-clone backend requires dest"})
		return errs
	}
	if path.IsAbs(p.Dest) {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("git-clone dest %q must be relative to $HOME", p.Dest)})
	}
	for _, elem := range strings.Split(path.Clean(p.Dest), "/") {
		if elem == ".." {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("git-clone dest %q must not contain ..", p.Dest)})
			break
		}
	}

	return errs
}

// validatePostInstall checks a package's optional post_install steps. post_install
// is valid ONLY on the git-clone backend (it is the clone's activation hook;
// rejecting it elsewhere stops it being read as a general escape hatch). Each step
// must carry a non-empty Run, and Run[0] must be path-contained to the clone dest
// — not absolute, no ".." segment — the same fence the backend re-checks at
// runtime via containedJoin.
func validatePostInstall(p Package) []error {
	if len(p.PostInstall) == 0 {
		return nil
	}

	var errs []error

	if p.Backend != BackendGitClone {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "post_install is only valid for the git-clone backend"})
	}

	for _, step := range p.PostInstall {
		if len(step.Run) == 0 {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "post_install step requires a non-empty run"})
			continue
		}
		exe := step.Run[0]
		if strings.TrimSpace(exe) == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "post_install run[0] must be non-empty"})
			continue
		}
		if path.IsAbs(exe) {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("post_install run %q must be relative to the clone dest", exe)})
			continue
		}
		clean := path.Clean(exe)
		if clean == ".." || strings.HasPrefix(clean, "../") {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("post_install run %q must not contain ..", exe)})
		}
	}

	return errs
}

// validateGitHubRelease checks a github-release package. Repo is required; Asset
// is OPTIONAL (empty = auto-detect the asset from the release page). At least one
// checksum source (SHA256, Checksums, or ChecksumsAsset) is REQUIRED so an
// artifact never installs unverified (fail-closed), and Tag is required whenever
// any checksum source is set (a checksum pinned against releases/latest breaks on
// the next upstream release). Every Checksums value must be 64-char lowercase hex
// under a non-empty key, and Match.Libc, when present, must be gnu or musl.
func validateGitHubRelease(p Package) []error {
	var errs []error

	if p.Repo == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "github-release backend requires repo"})
	}

	hasChecksum := p.SHA256 != "" || len(p.Checksums) > 0 || p.ChecksumsAsset != ""
	if !hasChecksum {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "github-release backend requires a checksum source (sha256, checksums, or checksums_asset)"})
	}
	if hasChecksum && p.Tag == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "github-release backend requires tag when a checksum source is set (a checksum pinned against releases/latest breaks on the next upstream release)"})
	}

	for key, sum := range p.Checksums {
		if strings.TrimSpace(key) == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "github-release checksums key must be non-empty (e.g. \"linux/x86_64\")"})
		}
		if !sha256Pattern.MatchString(sum) {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("github-release checksums[%q] = %q is not 64-char lowercase hex", key, sum)})
		}
	}

	if p.Match != nil && !validLibc[p.Match.Libc] {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("github-release match.libc %q must be one of gnu, musl (or empty)", p.Match.Libc)})
	}

	return errs
}

// validateFont checks a font package. Font and Font.Family are always required
// — the family name is the installed-state probe. A family with no Font.Archive
// must be installable through a host package manager (Names). A family WITH an
// archive is fetched from a GitHub release, so it needs an owner/repo Repo, a
// pinned Tag, and a checksum source: VerifySHA256 returns nil on an empty want,
// so an archive with no checksum source would extract a remote download
// entirely unverified.
//
// The accepted sources are exactly the two the font backend reads —
// SHA256 then ChecksumsAsset (backend.fontChecksum). The per-target Checksums
// map is deliberately NOT one of them: it is keyed by "<os>/<arch>", which has
// no meaning for a Nerd Font's single arch-independent zip, and accepting it
// here would validate an entry that then hard-fails at install with
// FontNoChecksumError on every host.
func validateFont(p Package) []error {
	var errs []error

	if p.Font == nil {
		return append(errs, &ValidationError{Package: p.Name, Msg: "font backend requires font"})
	}

	if p.Font.Family == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "font backend requires font.family"})
	}

	if p.Font.Archive == "" {
		if len(p.Names) == 0 {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "font backend requires font.archive or names"})
		}
		return errs
	}

	if !repoSlugPattern.MatchString(p.Repo) {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "font backend requires repo owner/repo when font.archive is set"})
	}
	if p.Tag == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "font backend requires tag when font.archive is set"})
	}
	if p.SHA256 == "" && p.ChecksumsAsset == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "font backend requires a checksum source (sha256 or checksums_asset) when font.archive is set"})
	}

	return errs
}

// validateClass checks a package's update-policy class (Class/Justification).
// It runs at the tail of validatePackage: a non-empty class must be a valid
// enum value (garbage short-circuits the rest, to avoid a cascade), an explicit
// class must be legal for the backend, and the custom-script escape hatch must
// declare BOTH an explicit class and a justification (two distinct errors).
// Justification on any other backend is allowed and never rejected.
func validateClass(p Package) []error {
	var errs []error

	if p.Class != "" && !p.Class.Valid() {
		return append(errs, &ValidationError{
			Package: p.Name,
			Msg:     fmt.Sprintf("class %q must be one of pinned, rolling, managed", p.Class),
		})
	}

	if p.Class != "" && !classByBackend[p.Backend][p.Class] {
		errs = append(errs, &ValidationError{
			Package: p.Name,
			Msg:     fmt.Sprintf("class %q is not legal for backend %q (legal: %s)", p.Class, p.Backend, joinClasses(LegalClasses(p.Backend))),
		})
	}

	if p.Backend == BackendCustomScript {
		if p.Class == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "custom-script backend requires an explicit class (pinned, rolling, or managed)"})
		}
		if strings.TrimSpace(p.Justification) == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "custom-script backend requires a justification"})
		}
	}

	return errs
}

// knownPackageOS is the set of accepted Package.OS gate tokens: the macOS and
// Windows platforms, the Linux distro families, and the family token "linux"
// (matching every Linux-family host). It deliberately omits "wsl" — a modern
// WSL host resolves to a concrete distro family, so a bare "wsl" gate would
// almost never match; Linux-only tools use "linux". The empty string (no gate)
// is accepted separately in validatePackageOS. This set is kept in lockstep
// with the backend osGateAllows switch.
var knownPackageOS = map[string]bool{
	"macos":   true,
	"windows": true,
	"linux":   true,
	"debian":  true,
	"redhat":  true,
	"arch":    true,
	"suse":    true,
}

// validatePackageOS checks a package's optional os platform gate: empty (no
// gate) or one of the known tokens. A typo'd token is rejected fail-fast rather
// than silently making the package match no host.
func validatePackageOS(p Package) []error {
	if p.OS == "" || knownPackageOS[p.OS] {
		return nil
	}
	return []error{&ValidationError{
		Package: p.Name,
		Msg:     fmt.Sprintf("os %q must be one of: arch, debian, linux, macos, redhat, suse, windows", p.OS),
	}}
}

// pathInstallBackends is the set of backends that place a binary on PATH and so
// support an explicit installed-state probe (Provides/ProvidesAny/InstalledWhen).
// The other backends (pkg/apt-repo/aur) query the package authority directly and
// are already honest, so a probe there is rejected as meaningless.
var pathInstallBackends = map[Backend]bool{
	BackendCustomScript:  true,
	BackendGitHubRelease: true,
	BackendScriptURL:     true,
	BackendToolchain:     true,
}

// validateProbe checks a package's optional installed-state probe fields
// (Provides, ProvidesAny, InstalledWhen). It runs as a standalone block after
// the per-backend switch: InstalledWhen is mutually exclusive with the
// Provides/ProvidesAny family (Provides and ProvidesAny themselves compose —
// all-present AND any-of), all three are valid only on the PATH-install
// backends, and each carries its own shape rule.
func validateProbe(p Package) []error {
	if len(p.Provides) == 0 && len(p.ProvidesAny) == 0 && p.InstalledWhen == nil {
		return nil
	}

	var errs []error

	// InstalledWhen is a distinct probe mechanism (a filesystem path, not a PATH
	// binary), so it cannot be combined with the Provides/ProvidesAny family.
	// Provides and ProvidesAny DO compose (all-present AND any-of), so they are
	// intentionally not mutually exclusive with each other.
	if p.InstalledWhen != nil && (len(p.Provides) > 0 || len(p.ProvidesAny) > 0) {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "installed_when is mutually exclusive with provides/provides_any"})
	}

	if !pathInstallBackends[p.Backend] {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "provides/provides_any/installed_when is only valid for custom-script, github-release, script-url, toolchain backends"})
	}

	for _, bin := range p.Provides {
		if strings.TrimSpace(bin) == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "provides entries must be non-empty"})
			break
		}
	}

	for _, bin := range p.ProvidesAny {
		if strings.TrimSpace(bin) == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "provides_any entries must be non-empty"})
			break
		}
	}

	if p.InstalledWhen != nil && p.InstalledWhen.Path == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "installed_when requires a non-empty path"})
	}

	return errs
}

// validateScriptPath statically checks a custom-script package's Script for the
// same containment invariant the backend enforces at runtime (see
// internal/backend/custom_script.go): it must be relative (not absolute) and
// contain no ".." segment, so an overlay fragment cannot smuggle an absolute or
// escaping script path past manifest load. Defense-in-depth — the backend
// re-checks against the resolved containment root at install time.
func validateScriptPath(p Package) []error {
	var errs []error

	if path.IsAbs(p.Script) {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("custom-script script %q must be relative", p.Script)})
	}
	for _, elem := range strings.Split(path.Clean(p.Script), "/") {
		if elem == ".." {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("custom-script script %q must not contain ..", p.Script)})
			break
		}
	}

	return errs
}

// validateSymlink checks a single Symlink against path-safety rules enforced
// at manifest-load time so a future apply step cannot join an unsafe dest
// onto $HOME.
func validateSymlink(s Symlink) []error {
	var errs []error

	if s.Src == "" {
		errs = append(errs, &ValidationError{Msg: "symlink missing required src"})
	}
	if s.Dest == "" {
		errs = append(errs, &ValidationError{Msg: "symlink missing required dest"})
	}
	if path.IsAbs(s.Dest) {
		errs = append(errs, &ValidationError{Msg: fmt.Sprintf("symlink dest %q must be relative to $HOME", s.Dest)})
	}

	clean := path.Clean(s.Dest)
	for _, elem := range strings.Split(clean, "/") {
		if elem == ".." {
			errs = append(errs, &ValidationError{Msg: fmt.Sprintf("symlink dest %q must not contain ..", s.Dest)})
			break
		}
	}

	return errs
}
