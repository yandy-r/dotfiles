package manifest

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
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

	for _, p := range m.Packages {
		for _, dep := range p.Requires {
			if dep == p.Name || !names[dep] {
				errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("requires unknown or self package %q", dep)})
			}
		}
	}
	errs = append(errs, validateRequiresCycles(m.Packages)...)

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

	errs = append(errs, validateSysconfig(m.Sysconfig)...)

	errs = append(errs, validateModules(m, names)...)

	return errors.Join(errs...)
}

// validateRequiresCycles rejects every requires cycle across ALL packages,
// ignoring host gates (os/arch/requires_desktop). Install ordering only walks
// the requires of packages applicable to the current host, so a cycle through
// a gated package would otherwise surface on some hosts and not others. Each
// cycle is reported once per DFS back edge, named in traversal order
// (a -> b -> a). Self and unknown references are reported by Validate and
// skipped here.
func validateRequiresCycles(pkgs []Package) []error {
	byName := make(map[string]*Package, len(pkgs))
	for i := range pkgs {
		if _, dup := byName[pkgs[i].Name]; !dup {
			byName[pkgs[i].Name] = &pkgs[i]
		}
	}
	const (
		unvisited = iota
		visiting
		done
	)
	var errs []error
	state := make(map[string]int, len(pkgs))
	var path []string
	var visit func(name string)
	visit = func(name string) {
		state[name] = visiting
		path = append(path, name)
		for _, dep := range byName[name].Requires {
			if dep == name || byName[dep] == nil {
				continue
			}
			switch state[dep] {
			case unvisited:
				visit(dep)
			case visiting:
				start := slices.Index(path, dep)
				cycle := append(slices.Clone(path[start:]), dep)
				errs = append(errs, &ValidationError{Package: dep, Msg: "requires cycle " + strings.Join(cycle, " -> ")})
			}
		}
		path = path[:len(path)-1]
		state[name] = done
	}
	for _, p := range pkgs {
		if state[p.Name] == unvisited {
			visit(p.Name)
		}
	}
	return errs
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
	case BackendRepo:
		errs = append(errs, validateRepo(p)...)
	case BackendGitClone:
		errs = append(errs, validateGitClone(p)...)
	case BackendFont:
		errs = append(errs, validateFont(p)...)
	case BackendAppArtifact:
		errs = append(errs, validateAppArtifact(p)...)
	case BackendPkgURL:
		errs = append(errs, validatePkgURL(p)...)
	case BackendFlatpak:
		errs = append(errs, validateFlatpak(p)...)
	case BackendPkg, BackendAUR:
		// No backend-specific required fields beyond what's checked below.
	}

	errs = append(errs, validateArtifactOnlyFields(p)...)

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
	errs = append(errs, validatePackageArch(p)...)
	errs = append(errs, validateExcludeOS(p)...)

	errs = append(errs, validatePostInstall(p)...)

	errs = append(errs, validateClass(p)...)
	errs = append(errs, validateRollingRelease(p)...)

	errs = append(errs, validateActions(p)...)

	errs = append(errs, validateArgs(p)...)

	errs = append(errs, validateDNFExclude(p)...)

	return errs
}

// validateDNFExclude checks a package's optional dnf_exclude. It is valid ONLY
// on the pkg backend (the sole consumer); an element must be a non-empty glob
// with no whitespace, since each becomes a single --exclude=<glob> argv entry.
func validateDNFExclude(p Package) []error {
	if len(p.DNFExclude) == 0 {
		return nil
	}
	if p.Backend != BackendPkg {
		return []error{&ValidationError{Package: p.Name, Msg: "dnf_exclude is only valid on the pkg backend"}}
	}
	var errs []error
	for _, g := range p.DNFExclude {
		if g == "" || strings.ContainsAny(g, " \t\n,") {
			errs = append(errs, &ValidationError{
				Package: p.Name,
				Msg:     fmt.Sprintf("dnf_exclude entry %q must be a non-empty glob without whitespace or commas", g),
			})
		}
	}
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
// mirroring the old symlink rule). Ref is optional (empty = the remote's default
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

// gpgFingerprintPattern matches a 40-hex OpenPGP fingerprint (case-insensitive),
// the shape of Package.GPGKeyFingerprint.
var gpgFingerprintPattern = regexp.MustCompile(`^[0-9A-Fa-f]{40}$`)

// isHTTPSURL reports whether s parses as an https URL with a host.
func isHTTPSURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != ""
}

// hasSuffixFold reports whether s ends in suffix, ignoring case.
func hasSuffixFold(s, suffix string) bool {
	return strings.HasSuffix(strings.ToLower(s), strings.ToLower(suffix))
}

// urlPathHasSuffix reports whether rawURL's path (query/fragment stripped) ends
// in suffix, ignoring case. An unparseable URL never matches.
func urlPathHasSuffix(rawURL, suffix string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && hasSuffixFold(u.Path, suffix)
}

// relPathProblem returns "" when p is a relative path with no ".." segment, or
// a short reason otherwise.
func relPathProblem(p string) string {
	if path.IsAbs(p) {
		return "must be relative"
	}
	for _, elem := range strings.Split(path.Clean(p), "/") {
		if elem == ".." {
			return "must not contain .."
		}
	}
	return ""
}

// validateArtifactSource checks the download source shared by the app-artifact
// and pkg-url backends: exactly one of repo (GitHub owner/repo) or url (https).
// It returns the errors and whether the repo route is in use. Checksum policy
// is left to the callers.
func validateArtifactSource(p Package, backend Backend) (errs []error, repoRoute bool) {
	switch {
	case p.Repo != "" && p.URL != "":
		errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("%s backend requires exactly one of repo or url, got both", backend)})
	case p.Repo == "" && p.URL == "":
		errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("%s backend requires exactly one of repo or url", backend)})
	case p.Repo != "":
		repoRoute = true
		if !repoSlugPattern.MatchString(p.Repo) {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("%s backend repo %q must be owner/repo", backend, p.Repo)})
		}
	default:
		if !isHTTPSURL(p.URL) {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("%s backend requires https url", backend)})
		}
	}

	for key, sum := range p.Checksums {
		if strings.TrimSpace(key) == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("%s checksums key must be non-empty (e.g. \"linux/x86_64\")", backend)})
		}
		if !sha256Pattern.MatchString(sum) {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("%s checksums[%q] = %q is not 64-char lowercase hex", backend, key, sum)})
		}
	}
	return errs, repoRoute
}

// validateAppArtifact checks an app-artifact package. The source is exactly one
// of repo or https url. The repo route follows the github-release fail-closed
// rule (a checksum source plus a tag); the url route's sha256 is optional (an
// absent one makes the package rolling). app.icon, when set, is an https URL or
// a safe relative path; app.desktop a safe relative path; an AppImage artifact
// (asset or url ending .AppImage) requires app.icon because the image carries
// no installable icon of its own.
func validateAppArtifact(p Package) []error {
	errs, repoRoute := validateArtifactSource(p, BackendAppArtifact)

	if repoRoute && p.Class != ClassRolling {
		hasChecksum := p.SHA256 != "" || len(p.Checksums) > 0 || p.ChecksumsAsset != ""
		if !hasChecksum {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "app-artifact backend requires a checksum source (sha256, checksums, or checksums_asset) on the repo route"})
		}
		if hasChecksum && p.Tag == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "app-artifact backend requires tag when a checksum source is set (a checksum pinned against releases/latest breaks on the next upstream release)"})
		}
	}

	if p.App != nil {
		if icon := p.App.Icon; icon != "" {
			if strings.Contains(icon, "://") {
				if !isHTTPSURL(icon) {
					errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("app.icon %q must be an https URL or a relative path", icon)})
				}
			} else if why := relPathProblem(icon); why != "" {
				errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("app.icon %q %s", icon, why)})
			}
		}
		if d := p.App.Desktop; d != "" {
			if why := relPathProblem(d); why != "" {
				errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("app.desktop %q %s", d, why)})
			}
		}
	}

	appImage := hasSuffixFold(p.Asset, ".AppImage") || (p.URL != "" && urlPathHasSuffix(p.URL, ".AppImage"))
	if appImage && (p.App == nil || p.App.Icon == "") {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "app-artifact AppImage requires app.icon"})
	}

	return errs
}

// pkgURLFormat resolves a pkg-url package's format: the explicit Format, else
// inferred from the asset suffix, else the URL path suffix. It returns "" when
// nothing identifies one.
func pkgURLFormat(p Package) string {
	if p.Format != "" {
		return p.Format
	}
	for _, f := range []string{"rpm", "deb"} {
		if hasSuffixFold(p.Asset, "."+f) || (p.URL != "" && urlPathHasSuffix(p.URL, "."+f)) {
			return f
		}
	}
	return ""
}

// validatePkgURL checks a pkg-url package. The source is exactly one of repo or
// https url; pkg_name is required (it is the installed probe and remove name);
// the format must resolve to rpm or deb (explicit, or from the asset/url
// suffix) and agree with any suffix. gpg_key_url requires a 40-hex
// gpg_fingerprint and an https URL; allow_unsigned requires a justification; a
// deb requires a checksum (sha256, or a repo-route checksum source) because apt
// installs a local .deb with no signature check of its own. A deb on the
// rolling repo route (see onRollingRepoRoute) satisfies that requirement
// implicitly: the resolver always requires and verifies GitHub's sha256 asset
// digest. Pinned, tagged, and non-GitHub debs stay fail-closed.
func validatePkgURL(p Package) []error {
	errs, repoRoute := validateArtifactSource(p, BackendPkgURL)

	if strings.TrimSpace(p.PkgName) == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "pkg-url backend requires pkg_name"})
	}

	format := pkgURLFormat(p)
	switch {
	case format == "":
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "pkg-url backend requires format (rpm or deb) when the url/asset has no .rpm/.deb suffix"})
	case format != "rpm" && format != "deb":
		errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("pkg-url format %q must be one of rpm, deb", format)})
	default:
		for _, other := range []string{"rpm", "deb"} {
			if other != format && (hasSuffixFold(p.Asset, "."+other) || (p.URL != "" && urlPathHasSuffix(p.URL, "."+other))) {
				errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("pkg-url format %q conflicts with the .%s suffix of its asset/url", format, other)})
			}
		}
	}

	if p.GPGKeyURL != "" {
		if !isHTTPSURL(p.GPGKeyURL) {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("pkg-url gpg_key_url %q must be an https URL", p.GPGKeyURL)})
		}
		if p.GPGKeyFingerprint == "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "pkg-url gpg_key_url requires gpg_fingerprint"})
		}
	}
	if fp := p.GPGKeyFingerprint; fp != "" && !gpgFingerprintPattern.MatchString(fp) {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("pkg-url gpg_fingerprint %q not 40-char hex OpenPGP fingerprint", fp)})
	}

	if p.AllowUnsigned && strings.TrimSpace(p.Justification) == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "pkg-url allow_unsigned requires a justification"})
	}

	if p.RPMRepackStripBuildID && (format != "rpm" || !p.AllowUnsigned || p.Justification == "" || (p.SHA256 == "" && (!repoRoute || p.Class != ClassRolling))) {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "rpm_repack_strip_buildid requires unsigned RPM with justification and checksum source"})
	}
	if format == "deb" {
		// The rolling repo route always verifies GitHub's own sha256 asset
		// digest (latestReleaseArtifact fails closed without one), so it
		// needs no separate manifest checksum source.
		hasSum := p.SHA256 != "" || onRollingRepoRoute(p) || (repoRoute && (len(p.Checksums) > 0 || p.ChecksumsAsset != ""))
		if !hasSum {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "pkg-url deb requires sha256 (or a checksums/checksums_asset source on the repo route, or the rolling GitHub repo route's asset digest)"})
		}
	}

	return errs
}

// validateArtifactOnlyFields rejects the app-artifact / pkg-url fields on any
// other backend, so they are never silently ignored.
func validateArtifactOnlyFields(p Package) []error {
	var errs []error

	if p.Backend != BackendAppArtifact && p.App != nil {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "app is only valid for the app-artifact backend"})
	}
	if p.Backend != BackendPkgURL {
		for field, set := range map[string]bool{
			"pkg_name":        p.PkgName != "",
			"format":          p.Format != "",
			"gpg_key_url":     p.GPGKeyURL != "",
			"gpg_fingerprint": p.GPGKeyFingerprint != "",
			"allow_unsigned":  p.AllowUnsigned,
		} {
			if set {
				errs = append(errs, &ValidationError{Package: p.Name, Msg: field + " is only valid for the pkg-url backend"})
			}
		}
	}

	if p.Backend != BackendFlatpak {
		if p.FlatpakID != "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "flatpak_id is only valid for the flatpak backend"})
		}
		if p.FlatpakRemote != "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "flatpak_remote is only valid for the flatpak backend"})
		}
	}

	return errs
}

// flatpakIDPattern matches a Flatpak application ID: at least three
// dot-separated segments of letters, digits, '_' or '-', none starting with a
// digit (the flatpak naming rules), e.g. com.usebottles.bottles.
var flatpakIDPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*(\.[A-Za-z_][A-Za-z0-9_-]*){2,}$`)

// flatpakRemotePattern bounds a remote name to a plain token so it can never
// be read as a flag or a path by the flatpak CLI.
var flatpakRemotePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// validateFlatpak checks a flatpak package: flatpak_id is required and must be
// a well-formed application ID; flatpak_remote, when set, must be a plain
// remote name. names are meaningless here (the ID is the install identifier).
func validateFlatpak(p Package) []error {
	var errs []error
	switch {
	case p.FlatpakID == "":
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "flatpak backend requires flatpak_id"})
	case !flatpakIDPattern.MatchString(p.FlatpakID):
		errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("flatpak_id %q is not a valid application ID (reverse-DNS, e.g. com.usebottles.bottles)", p.FlatpakID)})
	}
	if p.FlatpakRemote != "" && !flatpakRemotePattern.MatchString(p.FlatpakRemote) {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("flatpak_remote %q is not a valid remote name", p.FlatpakRemote)})
	}
	if len(p.Names) > 0 {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "names is not valid for the flatpak backend; set flatpak_id"})
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
func validatePackageArch(p Package) []error {
	var errs []error
	for _, a := range p.Arch {
		if a != "x86_64" && a != "arm64" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("arch %q must be x86_64 or arm64", a)})
		}
	}
	return errs
}

// validateExcludeOS checks the optional exclude_os static gate: every entry
// must be non-empty and one of the same tokens accepted by os (knownPackageOS;
// "wsl" is not one), so a typo never silently excludes nothing.
func validateExcludeOS(p Package) []error {
	var errs []error
	for _, o := range p.ExcludeOS {
		switch {
		case strings.TrimSpace(o) == "":
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "exclude_os entries must be non-empty"})
		case !knownPackageOS[o]:
			errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("exclude_os %q must be one of: arch, debian, linux, macos, redhat, suse, windows", o)})
		}
	}
	return errs
}

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
// The other backends (pkg/repo/aur) query the package authority directly and
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

	if w := p.InstalledWhen; w != nil {
		switch {
		case w.Path != "" && len(w.Paths) > 0:
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "installed_when path and paths are mutually exclusive"})
		case w.Path == "" && len(w.Paths) == 0:
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "installed_when requires a non-empty path or paths"})
		}
		for _, path := range w.Paths {
			if strings.TrimSpace(path) == "" {
				errs = append(errs, &ValidationError{Package: p.Name, Msg: "installed_when paths entries must be non-empty"})
				break
			}
		}
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
