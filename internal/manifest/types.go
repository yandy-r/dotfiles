// Package manifest defines the typed schema for the dotf provisioning
// manifest: the TOML-decoded contract describing intents (named bundles of
// packages (installable units across every supported backend).
//
// This package is the single source of truth for the manifest's shape. It
// is intentionally dependency-free (no go-toml, no huh) so that it can be
// imported by both the TOML decoder (Phase 3) and the interactive TUI
// (Phase 4) without pulling either into this contract.
package manifest

// Manifest is the top-level decoded shape of the provisioning manifest
// file: a set of named intents plus the flat list of packages they reference.
type Manifest struct {
	// Intents maps an intent name (e.g. "base", "dev") to its definition.
	Intents map[string]Intent `toml:"intents"`

	// Packages is the flat list of installable package records, spanning
	// all supported backends.
	Packages []Package `toml:"package"`

	// Sysconfig holds repo-wide default OS-level system-config values. It is
	// optional; a nil pointer means the manifest declares no defaults and
	// every sysconfig domain is left unmanaged.
	Sysconfig *Sysconfig `toml:"sysconfig"`

	// Modules is the flat list of named setup bundles, each an ordered pipeline
	// of typed steps opted into via `dotf setup <name>`.
	Modules []Module `toml:"module"`
}

// Sysconfig holds repo-wide default OS-level system-config values consumed by
// the sysconfig engine (hostname/timezone/locale/shell/firewall per-OS). Every
// field is optional; an empty value leaves that domain unmanaged. Hostname is
// intentionally absent — it is inherently per-machine and supplied only by the
// --hostname flag, never as a repo-wide default.
type Sysconfig struct {
	// Timezone is the IANA "Area/City" default (e.g. "America/New_York").
	Timezone string `toml:"timezone"`

	// Locale is the default LANG value (e.g. "en_US.UTF-8"). Linux only;
	// macOS treats locale as a no-op owned by the shell layer.
	Locale string `toml:"locale"`

	// Shell is the default login shell name or absolute path (e.g. "zsh").
	Shell string `toml:"shell"`

	// Firewall selects the firewall backend: "ufw", "firewalld", "auto", or
	// "off". Empty leaves the firewall domain unmanaged.
	Firewall string `toml:"firewall"`

	// SSHServer selects SSH server management: "on" or "off". Empty leaves
	// the SSH server domain unmanaged.
	SSHServer string `toml:"ssh_server"`
}

// Intent is a named, describable bundle of packages that a user can opt
// into during provisioning (e.g. "base", "dev", "desktop").
type Intent struct {
	// Description is a human-readable summary shown in the interactive
	// TUI and CLI help output.
	Description string `toml:"description"`

	// Default marks whether this intent is pre-selected during
	// provisioning.
	Default bool `toml:"default"`

	// Packages lists the package names (matching Package.Name) included
	// in this intent.
	Packages []string `toml:"packages"`
}

// PlatformSiblingSuffixes are the name suffixes that split one logical tool
// into OS- or arch-gated package entries ("<name>-macos", "<name>-arm64", …).
// `dotf install --only <name>` expands a name with every declared sibling, and
// the gates must leave exactly one of them applicable per host. The order sets
// only the order in which expanded sibling names are emitted.
var PlatformSiblingSuffixes = []string{"-arch", "-debian", "-linux", "-macos", "-fedora", "-arm64"}

// Package is a single installable unit, data-only, superset of the fields
// required across every supported backend (see Backend). Only the
// fields relevant to a given Package.Backend are expected to be set;
// validation of which fields are required per backend happens elsewhere.
type Package struct {
	// Name is the package's canonical identifier, referenced by
	// Intent.Packages.
	Name string `toml:"name"`

	// Backend selects which installation mechanism handles this package.
	Backend Backend `toml:"backend"`

	// Names maps a platform.PkgMgr string (e.g. "apt", "pacman") to the
	// package name used by that package manager, for the "pkg" backend.
	Names map[string]string `toml:"names"`

	// DNFExclude lists package globs passed to dnf as --exclude=<glob> on
	// install and upgrade, for the "pkg" backend. It works around a broken
	// distro update (e.g. a file conflict in a transitive dependency) without
	// editing the host's dnf.conf; other package managers ignore it. Remove an
	// entry once the upstream fix lands.
	DNFExclude []string `toml:"dnf_exclude"`

	// Repo is the GitHub "owner/repo" slug, for the "github-release"
	// backend.
	Repo string `toml:"repo"`

	// Asset is the release asset filename (or template), for the
	// "github-release" backend. It is OPTIONAL: an empty asset triggers
	// release-page auto-detection (list the tag's assets and match the host's
	// OS/arch tokens). When set, it may use {{.OS}} (distro family),
	// {{.ReleaseOS}} (linux/darwin/windows), and {{.Arch}} (amd64/arm64)
	// placeholders.
	Asset string `toml:"asset"`

	// Binary overrides the installed executable name for the "github-release"
	// backend when the binary carried by the asset (or the bare asset itself)
	// is named differently from Name — e.g. a "hadolint-Linux-x86_64" asset
	// that must land on PATH as "hadolint". Empty means the installed name is
	// Name.
	Binary string `toml:"binary"`

	// Checksums is a per-target sha256 map for the "github-release" backend,
	// keyed by "<ReleaseOS>/<canonical-arch>" (e.g. "linux/x86_64",
	// "darwin/arm64"). It lets a single templated/auto-detected package carry a
	// distinct checksum per platform where one SHA256 cannot. Consulted before
	// the inline SHA256. It is scoped to that backend only: the "font" backend
	// fetches a single arch-independent archive, for which a per-target key has
	// no meaning, so validateFont rejects it as a font checksum source.
	Checksums map[string]string `toml:"checksums"`

	// ChecksumsAsset names a release "sidecar" checksum file (e.g.
	// "checksums.txt" or "SHA256SUMS") for the "github-release" and "font"
	// backends. When set, the resolved asset's sha256 is read from that file's
	// matching line — the eget-style verification path for tools that publish a
	// shared checksum list instead of a per-asset sha field.
	ChecksumsAsset string `toml:"checksums_asset"`

	// Match tunes github-release asset auto-detection (libc preference and
	// exclude substrings) when Asset is empty. Nil means default auto-detection
	// (prefer gnu, no exclusions).
	Match *AssetMatch `toml:"match"`

	// Tag is the release tag to pin (e.g. "v1.2.3"), for the
	// "github-release" backend. When set, assets download from that exact
	// release (releases/download/<tag>/) instead of releases/latest.
	// Required whenever SHA256 is set: a checksum pinned against a floating
	// "latest" release breaks on the next upstream publish.
	Tag string `toml:"tag"`

	// SHA256 is the expected checksum of the downloaded artifact, for the
	// "github-release" and "script-url" backends.
	SHA256 string `toml:"sha256"`

	// URL is the source URL, for the "script-url", "git-clone",
	// "app-artifact", and "pkg-url" backends: the install-script location
	// for script-url, the https git remote for git-clone, the direct
	// download URL for app-artifact/pkg-url. Validation is per-backend, so
	// the uses never collide.
	URL string `toml:"url"`

	// Ref is the branch or tag to check out, for the "git-clone" backend.
	// Empty clones the remote's default branch. A raw commit SHA is
	// unsupported — shallow `--branch` takes a branch or tag only.
	Ref string `toml:"ref"`

	// Dest is the clone destination, relative to $HOME, for the "git-clone"
	// backend — a $HOME-relative path, never absolute and never containing "..". It doubles
	// as the intrinsic installed-state probe (the clone existing = installed).
	Dest string `toml:"dest"`

	// PostInstall is an ordered list of activation steps run from inside the
	// clone after a "git-clone" checkout (e.g. an upstream ./install). Each step
	// is a literal argv — no shell — whose executable is path-contained to the
	// clone dest, so it can only run code the clone itself shipped.
	PostInstall []PostInstallStep `toml:"post_install"`

	// Script is the path to (or inline body of) a script to execute, for
	// the "custom-script" backend.
	Script string `toml:"script"`

	// Args is the literal argv appended to the fetched installer, for the
	// "script-url" backend. It is how a vendor installer is driven
	// non-interactively: starship's install.sh prompts for confirmation on
	// /dev/tty and aborts unless passed --yes, which would fail every headless
	// `dotf apply`. Each element is passed verbatim as a separate argument —
	// never through a shell — so no element can be reinterpreted as a command.
	Args []string `toml:"args"`

	// Tool is the toolchain identifier (e.g. "go", "rust"), for the
	// "toolchain" backend.
	Tool string `toml:"tool"`

	// AptRepo configures the APT repository to add before installing, for
	// the "repo" backend.
	AptRepo *AptRepo `toml:"apt_repo"`

	// DNFRepo configures the dnf repository to add before installing, for the
	// "repo" backend on a RedHat-family host. It is the dnf sibling of AptRepo:
	// a package may carry either, or both (one logical tool, one entry), and the
	// backend dispatches on the host's package manager.
	DNFRepo *DNFRepo `toml:"dnf_repo"`

	// Font configures the "font" backend: the fontconfig family name that is
	// the installed-state probe, and the upstream release archive base name
	// used when no host package manager ships a patched build.
	Font *Font `toml:"font"`

	// App configures the "app-artifact" backend: the user-level Linux app
	// install (AppImage or tarball) laid out under
	// ~/.local/share/dotf/apps/<name> with a .desktop entry. Rejected on every
	// other backend.
	App *AppInstall `toml:"app"`

	// PkgName is the installed package name, for the "pkg-url" backend. It is
	// both the installed-state probe name and the name passed to the package
	// manager on remove.
	PkgName string `toml:"pkg_name"`

	// Format is the vendor package format, "rpm" or "deb", for the "pkg-url"
	// backend. Optional: inferred from the URL (or release asset) suffix; required
	// when the URL carries no .rpm/.deb suffix.
	Format string `toml:"format"`

	// GPGKeyURL is the HTTPS URL of the vendor's signing key, for the "pkg-url"
	// backend. Requires GPGKeyFingerprint so the fetched key is verified before
	// it is trusted.
	GPGKeyURL string `toml:"gpg_key_url"`

	// GPGKeyFingerprint is the 40-hex OpenPGP fingerprint of GPGKeyURL's key,
	// for the "pkg-url" backend.
	GPGKeyFingerprint string `toml:"gpg_fingerprint"`

	// AllowUnsigned opts a "pkg-url" package out of signature verification. It
	// requires a non-empty Justification documenting why.
	AllowUnsigned         bool   `toml:"allow_unsigned"`
	RPMRepackStripBuildID bool   `toml:"rpm_repack_strip_buildid"`
	Preview               bool   `toml:"preview"`
	URLJSONField          string `toml:"url_json_field"`

	// FlatpakID is the reverse-DNS application ID (e.g.
	// "com.usebottles.bottles"), for the "flatpak" backend. Required there and
	// rejected on every other backend.
	FlatpakID string `toml:"flatpak_id"`

	// FlatpakRemote names the flatpak remote to install FlatpakID from, for the
	// "flatpak" backend. Empty means "flathub", which the backend adds to the
	// user installation (--if-not-exists) before installing; any other remote
	// must already be configured on the host.
	FlatpakRemote string `toml:"flatpak_remote"`

	// Provides lists the executable names this package installs onto PATH.
	// When set it is the authoritative installed-state probe: the package
	// counts as installed only when EVERY listed binary resolves on PATH (the
	// all-present rule). This overrides a backend's default name-based LookPath,
	// so status/diff/apply tell the truth about bundle/group packages whose
	// manifest name (e.g. "aws-tools") is not itself a binary. It may be
	// combined with ProvidesAny (all Provides AND at least one ProvidesAny); it
	// is mutually exclusive with InstalledWhen. Only meaningful for the
	// PATH-install backends (custom-script, github-release, script-url,
	// toolchain).
	Provides []string `toml:"provides"`

	// ProvidesAny lists candidate executable names of which ANY ONE resolving
	// on PATH satisfies the probe (the any-of rule), for a binary whose name
	// varies by platform: fabric installs `fabric` (official/go) or `fabric-ai`
	// (Homebrew/AUR); a Lua interpreter is `lua`, `lua5.1`, or `luajit`. Like
	// Provides it is authoritative when set, overriding the default name-based
	// LookPath. It may be combined with Provides — the package then counts as
	// installed only when every Provides binary AND at least one ProvidesAny
	// binary resolve, modelling a tool that installs a fixed set of binaries
	// plus one whose name varies (lua: the stable `luarocks` plus one of the
	// interpreter names). Mutually exclusive with InstalledWhen; only meaningful
	// for the PATH-install backends.
	ProvidesAny []string `toml:"provides_any"`

	// InstalledWhen is an explicit installed-state probe for packages that put
	// no binary on PATH (config-only installs). When set it overrides the
	// backend's default name-based LookPath and is mutually exclusive with the
	// Provides/ProvidesAny family (see probeInstalled's Provides+ProvidesAny >
	// InstalledWhen > LookPath order). No manifest entry currently uses this
	// field: the git-clone backend (e.g. tpm) probes its own clone dest via
	// os.Stat.
	InstalledWhen *Probe `toml:"installed_when"`

	// Actions declares which lifecycle actions a custom-script package's script
	// honors as CLI flags. Only "update" (→ script --update) and "remove"
	// (→ script --uninstall) need declaring; "install" is always supported and
	// may be omitted. It is the capability gate that stops dotf passing
	// --update/--uninstall to a script that cannot parse it (which would error
	// or, worse, re-install). Only meaningful for the custom-script backend —
	// the native backends derive update/remove from the package authority. DRY
	// with the probe fields: another small typed fact about a package the engine
	// must not guess.
	Actions []string `toml:"actions"`

	// OS restricts this package to a specific platform, gating
	// Engine.Applicable/Installed so a platform-scoped tool's installed-state
	// probe is only evaluated where the tool actually installs. Empty means
	// every platform. Accepted values: "macos", "windows", the Linux distro
	// families ("debian", "redhat", "arch", "suse"), and "linux" — which matches
	// EVERY Linux-family host. gnu-utilities sets os="macos" so its macOS-only
	// g-prefixed GNU-tool provides sentinel is never falsely evaluated on Linux
	// (where LookPath would always miss). It follows the same gate model as the
	// symlink Facet (macOS/Arch/Linux-desktop) and the platformGated backends
	// (repo with apt_repo requires apt, aur requires Arch). NOTE: a modern WSL host
	// resolves to a concrete distro family, so use "linux" (not a bare "wsl"
	// token, which is intentionally not accepted) for Linux-only tools.
	OS string `toml:"os"`

	// Arch restricts applicability to these canonical host arches
	// (x86_64, arm64). Empty means every architecture.
	Arch []string `toml:"arch"`

	// ExcludeOS is a static OS-family exclusion gate: the package is
	// not-applicable when ANY listed token matches the host, using the same
	// tokens and semantics as OS ("linux" matches every Linux-family host;
	// "debian"/"redhat"/"arch"/"suse"/"macos"/"windows" match exactly). It is
	// evaluated in addition to OS, so a package can say "all Linux except
	// Debian-family". Use it to hand a host to a sibling route (an official
	// .deb on Debian) regardless of which package manager is primary. Empty
	// excludes nothing.
	ExcludeOS []string `toml:"exclude_os"`

	// Requires names prerequisite packages selected alongside this package.
	Requires []string `toml:"requires"`

	// RequiresDesktop marks a GUI package that only makes sense inside a
	// graphical session. When no desktop session is detected
	// (platform.Info.Desktop false — headless SSH/TTY/container; DOTF_DESKTOP
	// or --desktop overrides) the package is skipped
	// as not-applicable, gating Engine.Applicable/Installed/Install exactly like
	// OS (same defense-in-depth model).
	RequiresDesktop bool `toml:"requires_desktop"`

	// Class is the package's update-policy bucket (pinned, rolling, managed).
	// It is optional: EffectiveClass infers it from the backend when empty. It
	// is REQUIRED on the custom-script escape hatch, where no safe default
	// exists (see validateClass).
	Class Class `toml:"class"`

	// Justification is a short free-form reason for the declared Class. It is
	// REQUIRED on the custom-script backend (the escape hatch must document why
	// it exists and how it updates) and optional documentation elsewhere.
	Justification string `toml:"justification"`

	// ScriptRoot is engine-internal provenance, NOT a user-settable field: the
	// containment root a custom-script package's Script is resolved against.
	// DiscoverOverlays stamps it with the XDG user config root
	// (~/.config/dotf) for overlay-sourced packages so a user-local script stays
	// contained there; it is empty for base packages, which fall back to the repo
	// root (see internal/backend/custom_script.go). The `toml:"-"` tag keeps it
	// out of the decoder entirely, so strict DisallowUnknownFields even rejects a
	// user attempting to set a `script_root` key in a fragment.
	ScriptRoot string `toml:"-"`
}

// AssetMatch tunes the github-release asset auto-detector for packages that
// leave Asset empty. Both fields are optional.
type AssetMatch struct {
	// Libc selects the C library variant to prefer when a release ships both
	// gnu and musl builds: "" or "gnu" prefers glibc, "musl" flips to musl.
	Libc string `toml:"libc"`

	// Exclude is a list of case-insensitive substrings; any candidate asset
	// whose filename contains one is dropped (e.g. "completions" to reject a
	// shell-completions look-alike tarball).
	Exclude []string `toml:"exclude"`
}

// AppInstall configures the "app-artifact" backend's desktop integration. Every
// field is optional at the type level; validateAppArtifact enforces which are
// required for a given artifact kind (an AppImage requires Icon).
type AppInstall struct {
	// Icon is an https URL of the app icon, or a path relative to the extracted
	// app directory (no ".." segment, not absolute). Required for AppImages.
	Icon string `toml:"icon"`

	// Desktop is a .desktop file path relative to the extracted app directory
	// (no ".." segment). Empty means dotf generates the entry.
	Desktop string `toml:"desktop"`

	// DesktopName is the generated .desktop entry's display Name. Empty means
	// the package name.
	DesktopName string `toml:"desktop_name"`

	// Categories is the generated .desktop Categories value (e.g. "Network;").
	Categories string `toml:"categories"`

	// ExecArgs is extra arguments appended to the generated .desktop Exec line.
	ExecArgs string `toml:"exec_args"`

	// StartupWMClass is the generated .desktop StartupWMClass value (the
	// window class the shell uses to match windows to the launcher). When set
	// it overrides a same-named key from a vendor .desktop file.
	StartupWMClass string `toml:"startup_wm_class"`

	// MimeType is the generated .desktop MimeType value (e.g.
	// "x-scheme-handler/foo;"). When set it overrides a same-named key from a
	// vendor .desktop file.
	MimeType string `toml:"mime_type"`
}

// PostInstallStep is a single git-clone activation step: a literal argv vector
// executed with NO shell after the clone. Run[0] is resolved relative to and
// contained within the clone dest, so a step can only invoke code the cloned
// artifact shipped — never a host binary, never a shell string. That fence is
// what keeps git-clone's post_install from regressing into an arbitrary hook.
type PostInstallStep struct {
	// Run is the argv vector: Run[0] is the executable (path-contained to the
	// clone dest), Run[1:] its arguments.
	Run []string `toml:"run"`
}

// Probe is an explicit installed-state check. Exactly one facet is used:
// Path (one filesystem path that must exist) or Paths (several that must all
// exist), leaving room for a future command/version probe without reshaping
// the schema.
type Probe struct {
	// Path is a filesystem path that must exist for the package to count as
	// installed. A leading ~ or $HOME/$VARS are expanded.
	Path string `toml:"path"`

	// Paths lists filesystem paths that must all exist, for packages that
	// install several independent parts (codex: the CLI and its app-server
	// daemon). Expanded like Path; mutually exclusive with it.
	Paths []string `toml:"paths"`
}

// AptRepo describes an APT repository (GPG key + sources.list entry) to
// configure before installing a package via the "repo" backend.
type AptRepo struct {
	// KeyURL is the HTTPS URL of the repository's GPG key.
	KeyURL string `toml:"key_url"`

	// KeyFingerprint is the REQUIRED 40-hex OpenPGP fingerprint of the
	// repository's primary signing key. The fetched key file must list it as
	// a primary key (case-insensitive) or the install aborts before any
	// privileged command runs; only that key (with its subkeys) is written to
	// Keyring.
	KeyFingerprint string `toml:"key_fingerprint"`

	// Keyring is the destination path for the binary signed-by keyring file.
	Keyring string `toml:"keyring"`

	// RepoLine is the full deb [signed-by=…] line written to SourcesPath.
	RepoLine string `toml:"repo_line"`

	// SourcesPath is the path under /etc/apt/sources.list.d/ for RepoLine.
	SourcesPath string `toml:"sources_path"`
}

// DNFRepo describes a third-party dnf repository to configure before installing
// a package via the "repo" backend. Exactly one of three modes is used:
//
//	copr     — a COPR project enabled via `dnf -y copr enable <owner>/<project>`
//	repofile — a vendor-published .repo file curl'd and installed verbatim
//	baseurl  — an explicit repo body rendered by dotf
//
// The two file modes write /etc/yum.repos.d/<id>.repo directly with
// `sudo install -m 644` and never shell out to `dnf config-manager`, whose repo
// verbs and plugin package differ between dnf4 and dnf5.
type DNFRepo struct {
	// COPR is an "owner/project" COPR slug. Mutually exclusive with the file
	// modes; it is the only mode that shells out to dnf to add the repo.
	COPR string `toml:"copr"`

	// ID is the repository id. It names the file written to
	// /etc/yum.repos.d/<id>.repo and is REQUIRED for both file modes. It must be
	// a single safe path element (no "/", not "." or "..").
	ID string `toml:"id"`

	// RepoFile is the HTTPS URL of a vendor-published .repo file, installed
	// verbatim. Requires ID.
	RepoFile string `toml:"repofile"`

	// BaseURL is the repository base URL for the explicit mode. It may use the
	// {{.ReleaseVer}} (os-release VERSION_ID) and {{.Arch}} placeholders, the dnf
	// analogue of AptRepo.RepoLine's {{.Codename}}/{{.Arch}}. Requires ID.
	BaseURL string `toml:"baseurl"`

	// GPGKey is the HTTPS URL of the repo's signing key, for the explicit mode.
	// Optional: an empty key renders gpgcheck=0. Supports the same placeholders
	// as BaseURL.
	GPGKey string `toml:"gpgkey"`

	// RepoGPGCheck renders repo_gpgcheck=1 so dnf also verifies the signature
	// on the repository metadata (repodata/repomd.xml.asc), for the explicit
	// mode. Requires GPGKey; set it only for vendors that sign their metadata.
	RepoGPGCheck bool `toml:"repo_gpgcheck"`

	// Priority pins the package to this repository, for the file modes. dnf
	// only considers a package name from the enabled repo with the lowest
	// priority value (default 99), so another repo shipping its own rebuild
	// (e.g. Terra) can neither win on version nor stall upgrades with
	// exact-version subpackages. 1..98; 0 leaves the repo's priority alone.
	// dotf passes --setopt=<id>.priority on its own dnf commands and, on dnf5,
	// persists it under /etc/dnf/repos.override.d/ so a plain `dnf upgrade`
	// honours it too, even when the .repo file is a kept vendor file.
	Priority int `toml:"priority"`

	// ExcludeFrom lists other repo ids (e.g. "terra") that must never supply
	// this package or its subpackages: dotf excludes "<name>" and "<name>-*"
	// (name = names.dnf) from each, in any mode. Unlike Priority it also stops
	// differently-named subpackages, such as a repo's own <name>-terminfo or
	// <name>-zsh-completion, from colliding with files the package ships.
	// dotf's own dnf commands carry --setopt=<id>.excludepkgs for the ids the
	// host has configured, and on dnf5 the exclude is persisted under
	// /etc/dnf/repos.override.d/ for every listed id.
	ExcludeFrom []string `toml:"exclude_from"`
}

// Font describes one font family for the "font" backend. The repo's terminal
// and editor configs pin patched family names by exact string, so the family
// name — not a package name — is the thing that must end up installed.
type Font struct {
	// Family is the fontconfig family name the configs pin (e.g.
	// "JetBrainsMono Nerd Font"). It is the fc-list installed-state probe.
	Family string `toml:"family"`

	// Archive is the base name of the upstream release archive (e.g.
	// "JetBrainsMono"). The backend downloads "<Archive>.zip" from Repo@Tag
	// and extracts it under the XDG fonts directory. Empty means this family
	// is only installable through a host package manager (Names).
	Archive string `toml:"archive"`
}

// Backend identifies the installation mechanism for a Package.
type Backend string

// Canonical Backend values. These are the only installation mechanisms the
// provisioning CLI supports.
const (
	// BackendPkg installs via the host's native package manager, using
	// Package.Names to resolve the manager-specific name.
	BackendPkg Backend = "pkg"

	// BackendGitHubRelease downloads and installs a binary from a GitHub
	// release, using Package.Repo, Package.Asset, and Package.SHA256.
	BackendGitHubRelease Backend = "github-release"

	// BackendAUR installs from the Arch User Repository.
	BackendAUR Backend = "aur"

	// BackendToolchain installs via a language toolchain manager, using
	// Package.Tool.
	BackendToolchain Backend = "toolchain"

	// BackendRepo adds a third-party OS repository before installing: an APT
	// sources entry via apt_repo on a Debian-family host, a dnf repository via
	// dnf_repo on a RedHat-family host. One logical tool is ONE entry; the
	// backend dispatches on the host's package manager and is not applicable on a
	// host with no matching sub-table.
	BackendRepo Backend = "repo"

	// BackendScriptURL downloads and executes an install script from
	// Package.URL, verified against Package.SHA256.
	BackendScriptURL Backend = "script-url"

	// BackendCustomScript executes an arbitrary local script, using
	// Package.Script.
	BackendCustomScript Backend = "custom-script"

	// BackendGitClone shallow-clones a git remote (Package.URL) into a
	// $HOME-relative Package.Dest, optionally checking out Package.Ref and
	// running Package.PostInstall activation steps.
	BackendGitClone Backend = "git-clone"

	// BackendFont installs a font family: via the host package manager when it
	// ships a patched build (Names), otherwise by extracting the checksum-
	// verified upstream release archive named by Font.Archive into the XDG
	// fonts directory.
	BackendFont Backend = "font"

	// BackendAppArtifact installs a user-level Linux app (AppImage or tarball)
	// from a GitHub release (Repo) or direct URL into
	// ~/.local/share/dotf/apps/<name>, with a .desktop entry (Package.App).
	BackendAppArtifact Backend = "app-artifact"

	// BackendPkgURL downloads a vendor .rpm/.deb over HTTPS (Repo or URL) and
	// installs it via dnf/apt; Package.PkgName names the installed package.
	BackendPkgURL Backend = "pkg-url"

	// BackendFlatpak installs a Flatpak application (Package.FlatpakID) into
	// the user installation from Package.FlatpakRemote (default flathub).
	// Linux only.
	BackendFlatpak Backend = "flatpak"
)

// Valid reports whether b is one of the canonical Backend values.
func (b Backend) Valid() bool {
	switch b {
	case BackendPkg, BackendGitHubRelease, BackendAUR, BackendToolchain, BackendRepo, BackendScriptURL, BackendCustomScript, BackendGitClone, BackendFont, BackendAppArtifact, BackendPkgURL, BackendFlatpak:
		return true
	default:
		return false
	}
}
