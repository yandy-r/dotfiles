package manifest_test

import (
	"errors"
	"github.com/yandy-r/dotfiles/internal/privilege/privilegetest"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	toml "github.com/pelletier/go-toml/v2"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// mustReadFile reads path (relative to this test file's directory) and
// fails the test on error.
func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}

func TestParse_Valid(t *testing.T) {
	data := mustReadFile(t, filepath.Join("testdata", "valid.toml"))

	m, err := manifest.Parse(strings.NewReader(data))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if m == nil {
		t.Fatal("Parse() manifest = nil, want non-nil")
	}

	if _, ok := m.Intents["base"]; !ok {
		t.Error(`Parse() missing intents["base"]`)
	}
	if len(m.Packages) != 7 {
		t.Errorf("len(Packages) = %d, want 7 (one per backend)", len(m.Packages))
	}

	if m.Sysconfig == nil {
		t.Fatal("Parse() Sysconfig = nil, want non-nil [sysconfig] block")
	}
	if got, want := m.Sysconfig.Timezone, "America/New_York"; got != want {
		t.Errorf("Sysconfig.Timezone = %q, want %q", got, want)
	}
	if got, want := m.Sysconfig.Firewall, "ufw"; got != want {
		t.Errorf("Sysconfig.Firewall = %q, want %q", got, want)
	}
}

// TestParse_ScriptURLWithoutSHA256_Valid asserts a script-url package with a
// url but no sha256 parses successfully: pinning is optional for rotating
// vendor installers (see internal/backend/script_url.go's Install doc).
func TestParse_ScriptURLWithoutSHA256_Valid(t *testing.T) {
	const data = `
[intents.base]
description = "test"
default = true
packages = ["starship"]

[[package]]
name = "starship"
backend = "script-url"
url = "https://starship.rs/install.sh"
`

	m, err := manifest.Parse(strings.NewReader(data))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if m == nil {
		t.Fatal("Parse() manifest = nil, want non-nil")
	}
}

// TestParse_ProbeFields asserts the optional installed-state probe fields
// (provides, installed_when) decode and validate on the PATH-install backends.
func TestParse_ProbeFields(t *testing.T) {
	const data = `
[[package]]
name = "aws-tools"
backend = "custom-script"
script = "bin/install-aws-tools.sh"
provides = ["aws", "eksctl"]
class = "rolling"
justification = "official installer tracks latest"

[[package]]
name = "tpm"
backend = "custom-script"
script = "bin/install-tpm.sh"
installed_when = { path = "~/.tmux/plugins/tpm" }
class = "managed"
justification = "git-clone setup step; no upstream version pin"
`

	m, err := manifest.Parse(strings.NewReader(data))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}

	if got := byName["aws-tools"].Provides; len(got) != 2 || got[0] != "aws" || got[1] != "eksctl" {
		t.Errorf("aws-tools.Provides = %v, want [aws eksctl]", got)
	}
	if byName["aws-tools"].InstalledWhen != nil {
		t.Errorf("aws-tools.InstalledWhen = %+v, want nil", byName["aws-tools"].InstalledWhen)
	}

	tpm := byName["tpm"]
	if tpm.InstalledWhen == nil || tpm.InstalledWhen.Path != "~/.tmux/plugins/tpm" {
		t.Errorf("tpm.InstalledWhen = %+v, want path ~/.tmux/plugins/tpm", tpm.InstalledWhen)
	}
	if len(tpm.Provides) != 0 {
		t.Errorf("tpm.Provides = %v, want empty", tpm.Provides)
	}
}

// TestParse_ProvidesAny asserts the any-of probe field decodes on its own
// (fabric: fabric|fabric-ai) and composed with an all-present provides (lua:
// luarocks AND one of lua|lua5.1|luajit) — the two are intentionally not
// mutually exclusive.
func TestParse_ProvidesAny(t *testing.T) {
	const data = `
[[package]]
name = "fabric"
backend = "custom-script"
script = "bin/install-fabric.sh"
provides_any = ["fabric", "fabric-ai"]
class = "rolling"
justification = "go install of the latest package version"

[[package]]
name = "lua"
backend = "custom-script"
script = "bin/install-lua.sh"
provides = ["luarocks"]
provides_any = ["lua", "lua5.1", "luajit"]
class = "managed"
justification = "host package manager owns versioning"
`

	m, err := manifest.Parse(strings.NewReader(data))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}

	fabric := byName["fabric"]
	if !slices.Equal(fabric.ProvidesAny, []string{"fabric", "fabric-ai"}) {
		t.Errorf("fabric.ProvidesAny = %v, want [fabric fabric-ai]", fabric.ProvidesAny)
	}
	if len(fabric.Provides) != 0 {
		t.Errorf("fabric.Provides = %v, want empty", fabric.Provides)
	}

	lua := byName["lua"]
	if !slices.Equal(lua.Provides, []string{"luarocks"}) {
		t.Errorf("lua.Provides = %v, want [luarocks]", lua.Provides)
	}
	if !slices.Equal(lua.ProvidesAny, []string{"lua", "lua5.1", "luajit"}) {
		t.Errorf("lua.ProvidesAny = %v, want [lua lua5.1 luajit]", lua.ProvidesAny)
	}
}

// TestParse_PackageOS asserts the optional package-level os platform gate
// decodes onto Package.OS.
func TestParse_PackageOS(t *testing.T) {
	const data = `
[[package]]
name = "gnu-utilities"
backend = "custom-script"
script = "bin/install-gnu-utilities.sh"
os = "macos"
provides = ["gsed"]
class = "managed"
justification = "brew owns versioning"

[[package]]
name = "ripgrep"
backend = "pkg"
names = { pacman = "ripgrep", apt = "ripgrep" }
`

	m, err := manifest.Parse(strings.NewReader(data))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}

	if got := byName["gnu-utilities"].OS; got != "macos" {
		t.Errorf("gnu-utilities.OS = %q, want macos", got)
	}
	if got := byName["ripgrep"].OS; got != "" {
		t.Errorf("ripgrep.OS = %q, want empty (no gate)", got)
	}
}

// TestParse_GitCloneFields asserts a git-clone package decodes its url, ref,
// dest, and typed post_install argv steps (mirror of TestParse_ProbeFields).
func TestParse_GitCloneFields(t *testing.T) {
	data := mustReadFile(t, filepath.Join("testdata", "valid_git_clone.toml"))

	m, err := manifest.Parse(strings.NewReader(data))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}

	tool := byName["tool"]
	if got, want := tool.URL, "https://github.com/example/tool"; got != want {
		t.Errorf("tool.URL = %q, want %q", got, want)
	}
	if got, want := tool.Ref, "v1.2.3"; got != want {
		t.Errorf("tool.Ref = %q, want %q", got, want)
	}
	if got, want := tool.Dest, ".config/tool"; got != want {
		t.Errorf("tool.Dest = %q, want %q", got, want)
	}
	if len(tool.PostInstall) != 1 || len(tool.PostInstall[0].Run) != 2 ||
		tool.PostInstall[0].Run[0] != "install" || tool.PostInstall[0].Run[1] != "--all" {
		t.Errorf("tool.PostInstall = %+v, want [{Run:[install --all]}]", tool.PostInstall)
	}
}

func TestParse_UnknownField_StrictMissingError(t *testing.T) {
	data := mustReadFile(t, filepath.Join("testdata", "invalid_unknown_field.toml"))

	m, err := manifest.Parse(strings.NewReader(data))
	if err == nil {
		t.Fatal("Parse() error = nil, want error")
	}
	if m != nil {
		t.Errorf("Parse() manifest = %+v, want nil on error", m)
	}

	var strictErr *toml.StrictMissingError
	if !errors.As(err, &strictErr) {
		t.Errorf("errors.As(%v, *toml.StrictMissingError) = false, want true", err)
	}
}

func TestParse_InvalidManifests(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{name: "unknown backend", file: "invalid_backend.toml"},
		{name: "dangling intent reference", file: "invalid_dangling_intent.toml"},
		{name: "missing required backend field", file: "invalid_missing_field.toml"},
		{name: "bad sha256 shape", file: "invalid_sha256.toml"},
		{name: "unknown names/pkgmgr key", file: "invalid_pkgmgr_key.toml"},
		{name: "duplicate package name", file: "invalid_duplicate_name.toml"},
		{name: "missing package name", file: "invalid_missing_package_name.toml"},
		{name: "script-url http scheme", file: "invalid_script_url_http.toml"},
		{name: "github-release with no checksum source", file: "invalid_missing_sha256.toml"},
		{name: "github-release checksum source without tag", file: "invalid_github_release_no_tag.toml"},
		{name: "repo missing required field", file: "invalid_apt_repo_missing_field.toml"},
		{name: "sysconfig bad firewall value", file: "invalid_sysconfig_firewall.toml"},
		{name: "toolchain unknown tool", file: "invalid_toolchain_tool.toml"},
		{name: "toolchain names key not a tool", file: "invalid_toolchain_names_key.toml"},
		{name: "provides and installed_when both set", file: "invalid_probe_both.toml"},
		{name: "provides_any and installed_when both set", file: "invalid_probe_any_and_installed_when.toml"},
		{name: "provides on pkg backend", file: "invalid_probe_wrong_backend.toml"},
		{name: "provides_any on pkg backend", file: "invalid_probe_any_wrong_backend.toml"},
		{name: "installed_when empty path", file: "invalid_probe_empty_path.toml"},
		{name: "installed_when path and paths both set", file: "invalid_probe_path_and_paths.toml"},
		{name: "installed_when empty paths entry", file: "invalid_probe_empty_paths_entry.toml"},
		{name: "probe empty provides entry", file: "invalid_probe_empty_provides.toml"},
		{name: "probe empty provides_any entry", file: "invalid_probe_empty_any.toml"},
		{name: "git-clone missing url", file: "invalid_git_clone_no_url.toml"},
		{name: "git-clone http scheme", file: "invalid_git_clone_http_url.toml"},
		{name: "git-clone missing dest", file: "invalid_git_clone_no_dest.toml"},
		{name: "git-clone dest traversal", file: "invalid_git_clone_dest_traversal.toml"},
		{name: "git-clone with installed_when probe", file: "invalid_git_clone_probe.toml"},
		{name: "post_install on non-git-clone backend", file: "invalid_post_install_wrong_backend.toml"},
		{name: "post_install empty run", file: "invalid_post_install_run_empty.toml"},
		{name: "post_install empty exec", file: "invalid_post_install_empty_exec.toml"},
		{name: "post_install run[0] escaping", file: "invalid_post_install_bad_exec.toml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := mustReadFile(t, filepath.Join("testdata", tt.file))

			m, err := manifest.Parse(strings.NewReader(data))
			if err == nil {
				t.Fatalf("Parse(%s) error = nil, want error", tt.file)
			}
			if m != nil {
				t.Fatalf("Parse(%s) manifest = %+v, want nil on error", tt.file, m)
			}

			var valErr *manifest.ValidationError
			if !errors.As(err, &valErr) {
				t.Errorf("Parse(%s) error = %v, want errors.As match for *manifest.ValidationError", tt.file, err)
			}
		})
	}
}

func TestLoadFS_EmbeddedManifest(t *testing.T) {
	sub, err := fs.Sub(dotfiles.ManifestFS, "manifest")
	if err != nil {
		t.Fatalf("fs.Sub() error = %v", err)
	}

	m, err := manifest.LoadFS(sub, "packages.toml")
	if err != nil {
		t.Fatalf("LoadFS(embedded, packages.toml) error = %v, want nil", err)
	}
	if m == nil {
		t.Fatal("LoadFS(embedded, packages.toml) manifest = nil, want non-nil")
	}
}

// loadEmbeddedManifest loads the default manifest embedded via
// dotfiles.ManifestFS, failing the test on any load or validation error.
func loadEmbeddedManifest(t *testing.T) *manifest.Manifest {
	t.Helper()

	sub, err := fs.Sub(dotfiles.ManifestFS, "manifest")
	if err != nil {
		t.Fatalf("fs.Sub() error = %v", err)
	}

	m, err := manifest.LoadFS(sub, "packages.toml")
	if err != nil {
		t.Fatalf("LoadFS(embedded, packages.toml) error = %v, want nil", err)
	}
	return m
}

// supportedManagerHosts is the representative host for each package manager the
// pkg backend can execute installs for (internal/backend/pkg.go installArgv),
// paired with the distro family that manager belongs to. Reachability is
// evaluated per HOST, not per manager string, because a package's `os` gate and
// its backend's own platform gate both key off platform.Info.OS.
var supportedManagerHosts = []struct {
	mgr  string
	info platform.Info
}{
	{"apt", platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrAPT}},
	{"dnf", platform.Info{OS: platform.OSRedHat, Arch: platform.ArchARM64, PkgMgr: platform.PkgMgrDNF}},
	{"homebrew", platform.Info{OS: platform.OSMacOS, Arch: platform.ArchARM64, PkgMgr: platform.PkgMgrHomebrew}},
	{"pacman", platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrPacman}},
	{"zypper", platform.Info{OS: platform.OSSUSE, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrZypper}},
}

// siblingGroups lists the per-platform sibling sets (ADR-3/ADR-4): entries that
// are alternative install routes for ONE logical tool, whose `os` gates and
// `names` tables must TOGETHER cover every supportedManagerHosts entry. Any
// package not named here is its own sole provider. This map is the only
// hand-maintained half of the contract — adding a sibling entry without listing
// it here fails the test, which is the intended review prompt.
var siblingGroups = [][]string{
	{"lazygit", "lazygit-bin"},
	{"yazi", "yazi-bin", "yazi-debian-bin"},
	{"lutris", "lutris-pkg"},
}

// coreTools are the modern-CLI tools the shared shell configuration
// (common/tools.sh, common/aliases.sh) integrates. Used by
// TestEmbeddedManifest_CoreToolsetProvisioned; the reachability test derives its
// own subject set from the manifest instead, so a new pkg entry is covered
// automatically.
var coreTools = []string{
	"ripgrep", "eza", "bat", "fd", "fzf", "delta", "zoxide", "jq",
	"gh", "lazygit", "tmux", "neovim", "yazi", "direnv",
	"starship", "atuin",
}

// TestEmbeddedManifest_CoreToolsetProvisioned guards against regressing to a
// skeleton manifest (issue #12): every modern-CLI tool the shared shell
// configuration (common/tools.sh) integrates must be defined as a package
// AND referenced by an intent in the headless default set (base or dev), so
// a fresh `dotf apply` actually installs it.
func TestEmbeddedManifest_CoreToolsetProvisioned(t *testing.T) {
	m := loadEmbeddedManifest(t)

	defaultSet := make(map[string]bool)
	for _, intentName := range []string{"base", "dev"} {
		intent, ok := m.Intents[intentName]
		if !ok {
			t.Fatalf("Intents[%q] missing from embedded manifest", intentName)
		}
		for _, name := range intent.Packages {
			defaultSet[name] = true
		}
	}

	packages := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		packages[pkg.Name] = pkg
	}

	for _, tool := range coreTools {
		t.Run(tool, func(t *testing.T) {
			if _, ok := packages[tool]; !ok {
				t.Fatalf("embedded manifest has no [[package]] named %q", tool)
			}
			if !defaultSet[tool] {
				t.Errorf("package %q is not referenced by the base or dev intent, so a fresh `dotf apply` never installs it", tool)
			}
		})
	}
}

func TestEmbeddedManifest_HerdrRelease(t *testing.T) {
	m := loadEmbeddedManifest(t)
	dev, ok := m.Intents["dev"]
	if !ok {
		t.Fatal(`Intents["dev"] missing from embedded manifest`)
	}

	names := []string{"herdr", "herdr-macos"}
	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}

	want := map[string]struct {
		os        string
		checksums map[string]string
	}{
		"herdr": {
			os: "linux",
			checksums: map[string]string{
				"linux/x86_64": "976150a14d490c94b243ea2e1a7eb2dfb67f12e36b182db90936f6728e6aecf4",
				"linux/arm64":  "f55610658e1c2e0d2aaef730b4b2ab885f7f8ba00285ab372bfb14f2e3d5b40d",
			},
		},
		"herdr-macos": {
			os: "macos",
			checksums: map[string]string{
				"darwin/x86_64": "ab50262c8190cd7aa9056d249d255c08c328c3e8716de9cfa29db4f131b8e2c1",
				"darwin/arm64":  "a5d4f4d504d8b309c91f811050559300faba31258425f53c50852fc96f6ae574",
			},
		},
	}

	for _, name := range names {
		if !slices.Contains(dev.Packages, name) {
			t.Errorf("dev intent does not include %q", name)
		}
		pkg, ok := byName[name]
		if !ok {
			t.Fatalf("embedded manifest has no [[package]] named %q", name)
		}
		if pkg.Backend != manifest.BackendGitHubRelease {
			t.Errorf("%s backend = %q, want github-release", name, pkg.Backend)
		}
		if pkg.Repo != "herdrdev/herdr" || pkg.Tag != "v0.8.2" || pkg.Binary != "herdr" {
			t.Errorf("%s release = repo %q tag %q binary %q", name, pkg.Repo, pkg.Tag, pkg.Binary)
		}
		if pkg.OS != want[name].os {
			t.Errorf("%s os = %q, want %q", name, pkg.OS, want[name].os)
		}
		if len(pkg.Checksums) != len(want[name].checksums) {
			t.Errorf("%s checksums = %v, want exactly %v", name, pkg.Checksums, want[name].checksums)
		}
		for target, sum := range want[name].checksums {
			if pkg.Checksums[target] != sum {
				t.Errorf("%s checksum[%q] = %q, want %q", name, target, pkg.Checksums[target], sum)
			}
		}
		if got := pkg.EffectiveClass(); got != manifest.ClassPinned {
			t.Errorf("%s effective class = %q, want pinned", name, got)
		}
	}

	engine := backend.Default(exec.NewFake(), privilegetest.NewFakeDropper(), t.TempDir(), nil)
	targets := []struct {
		name string
		info platform.Info
		want []string
	}{
		{"debian-x86_64", platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64}, []string{"herdr"}},
		{"redhat-arm64", platform.Info{OS: platform.OSRedHat, Arch: platform.ArchARM64}, []string{"herdr"}},
		{"arch-x86_64", platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}, []string{"herdr"}},
		{"suse-x86_64", platform.Info{OS: platform.OSSUSE, Arch: platform.ArchX86_64}, []string{"herdr"}},
		{"generic-linux-arm64", platform.Info{OS: platform.OSLinux, Arch: platform.ArchARM64}, []string{"herdr"}},
		{"wsl-x86_64", platform.Info{OS: platform.OSWSL, Arch: platform.ArchX86_64}, []string{"herdr"}},
		{"darwin-x86_64", platform.Info{OS: platform.OSMacOS, Arch: platform.ArchX86_64}, []string{"herdr-macos"}},
		{"darwin-arm64", platform.Info{OS: platform.OSMacOS, Arch: platform.ArchARM64}, []string{"herdr-macos"}},
		{"windows-excluded", platform.Info{OS: platform.OSWindows, Arch: platform.ArchX86_64}, nil},
	}
	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			var got []string
			for _, name := range names {
				applicable, err := engine.Applicable(byName[name], target.info)
				if err != nil {
					t.Fatalf("Applicable(%q) error = %v", name, err)
				}
				if applicable {
					got = append(got, name)
				}
			}
			if !slices.Equal(got, target.want) {
				t.Errorf("applicable Herdr packages = %v, want %v", got, target.want)
			}
		})
	}
}

// TestEmbeddedManifest_UbuntuMissingAptSiblings pins the Debian-family
// github-release fallbacks for yazi and ruff. Official Ubuntu/Debian archives
// do not ship those apt names, so the shared pkg entries omit apt and these
// siblings must be the only applicable install path on a debian+apt host.
func TestEmbeddedManifest_UbuntuMissingAptSiblings(t *testing.T) {
	m := loadEmbeddedManifest(t)
	dev, ok := m.Intents["dev"]
	if !ok {
		t.Fatal(`Intents["dev"] missing from embedded manifest`)
	}

	names := []string{"yazi", "yazi-bin", "yazi-debian-bin", "ruff", "ruff-macos", "ruff-bin"}
	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}

	for _, name := range names {
		if !slices.Contains(dev.Packages, name) {
			t.Errorf("dev intent does not include %q", name)
		}
		if _, ok := byName[name]; !ok {
			t.Fatalf("embedded manifest has no [[package]] named %q", name)
		}
	}

	yaziDebian := byName["yazi-debian-bin"]
	if yaziDebian.Backend != manifest.BackendGitHubRelease || yaziDebian.OS != "debian" {
		t.Errorf("yazi-debian-bin backend/os = %q/%q, want github-release/debian", yaziDebian.Backend, yaziDebian.OS)
	}
	if yaziDebian.Repo != "sxyazi/yazi" || yaziDebian.Tag != "v26.8.15" || yaziDebian.Binary != "yazi" {
		t.Errorf("yazi-debian-bin release = repo %q tag %q binary %q", yaziDebian.Repo, yaziDebian.Tag, yaziDebian.Binary)
	}
	if yaziDebian.Checksums["linux/x86_64"] == "" || yaziDebian.Checksums["linux/arm64"] == "" {
		t.Errorf("yazi-debian-bin checksums = %v, want linux/x86_64 and linux/arm64 pins", yaziDebian.Checksums)
	}

	ruffBin := byName["ruff-bin"]
	if ruffBin.Backend != manifest.BackendGitHubRelease || ruffBin.OS != "debian" {
		t.Errorf("ruff-bin backend/os = %q/%q, want github-release/debian", ruffBin.Backend, ruffBin.OS)
	}
	if ruffBin.Repo != "astral-sh/ruff" || ruffBin.Tag != "0.16.8" || ruffBin.Binary != "ruff" {
		t.Errorf("ruff-bin release = repo %q tag %q binary %q", ruffBin.Repo, ruffBin.Tag, ruffBin.Binary)
	}
	if ruffBin.ChecksumsAsset != "sha256.sum" {
		t.Errorf("ruff-bin checksums_asset = %q, want sha256.sum", ruffBin.ChecksumsAsset)
	}
	if ruffBin.Match == nil || ruffBin.Match.Libc != "gnu" {
		t.Errorf("ruff-bin match.libc = %v, want gnu", ruffBin.Match)
	}

	if _, ok := byName["yazi"].Names["apt"]; ok {
		t.Error("yazi pkg still declares an apt name; Ubuntu archives do not ship it")
	}
	if _, ok := byName["ruff"].Names["apt"]; ok {
		t.Error("ruff pkg still declares an apt name; Ubuntu archives have no installable candidate")
	}

	engine := backend.Default(exec.NewFake(), privilegetest.NewFakeDropper(), t.TempDir(), nil)
	targets := []struct {
		name string
		info platform.Info
		want []string
	}{
		{
			name: "debian-apt",
			info: platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrAPT},
			want: []string{"yazi-debian-bin", "ruff-bin"},
		},
		{
			name: "redhat-dnf",
			info: platform.Info{OS: platform.OSRedHat, Arch: platform.ArchARM64, PkgMgr: platform.PkgMgrDNF},
			want: []string{"yazi-bin", "ruff"},
		},
		{
			name: "arch-pacman",
			info: platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrPacman},
			want: []string{"yazi", "ruff"},
		},
	}
	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			var got []string
			for _, name := range names {
				applicable, err := engine.Applicable(byName[name], target.info)
				if err != nil {
					t.Fatalf("Applicable(%q) error = %v", name, err)
				}
				if applicable {
					got = append(got, name)
				}
			}
			if !slices.Equal(got, target.want) {
				t.Errorf("applicable yazi/ruff packages = %v, want %v", got, target.want)
			}
		})
	}
}

// TestEmbeddedManifest_NeovimStableRelease prevents distro package versions or
// a floating nightly from re-entering the cross-platform provisioning path.
// Neovim needs its extracted runtime tree, so the governed installer is the
// correct escape hatch instead of the single-binary github-release backend.
func TestEmbeddedManifest_NeovimStableRelease(t *testing.T) {
	m := loadEmbeddedManifest(t)
	var neovim manifest.Package
	for _, pkg := range m.Packages {
		if pkg.Name == "neovim" {
			neovim = pkg
			break
		}
	}

	if neovim.Name == "" {
		t.Fatal("neovim package missing from embedded manifest")
	}
	if neovim.Backend != manifest.BackendCustomScript {
		t.Errorf("neovim backend = %q, want custom-script", neovim.Backend)
	}
	if neovim.Script != "bin/install-neovim.sh" {
		t.Errorf("neovim script = %q, want bin/install-neovim.sh", neovim.Script)
	}
	if !slices.Equal(neovim.Provides, []string{"nvim"}) {
		t.Errorf("neovim provides = %v, want [nvim]", neovim.Provides)
	}
	if !slices.Contains(neovim.Actions, "update") {
		t.Errorf("neovim actions = %v, want update support", neovim.Actions)
	}
	if got := neovim.EffectiveClass(); got != manifest.ClassRolling {
		t.Errorf("neovim effective class = %q, want rolling", got)
	}

	script := mustReadFile(t, filepath.Join("..", "..", neovim.Script))
	if !strings.Contains(script, `NVIM_VERSION="${NVIM_VERSION:-stable}"`) {
		t.Error("install-neovim.sh does not default NVIM_VERSION to stable")
	}
}

// TestEmbeddedManifest_ToolsReachableOnSupportedManagers replaces the retired
// TestEmbeddedManifest_PkgNamesCoverSupportedManagers, which asserted per-manager
// name PRESENCE on every intent-referenced pkg entry. That contract was the
// direct cause of two fictional dnf names (`lazygit`, `yazi` — RPMs that exist in
// no Fedora branch): it pressured an author facing a tool a distro does not
// package into inventing a plausible name to keep CI green, and the same trap was
// armed for zypper and for every future distro.
//
// The replacement contract is per-manager reachable INSTALL PATH: for every
// intent-referenced pkg package (the SAME manifest-derived subject set the
// retired test used, so a future pkg entry is covered automatically) and every
// supported manager host, at least one member of that entry's sibling group must
// be Applicable there, evaluated through the REAL backend.Engine.Applicable — the
// same predicate `dotf status`, `dotf diff` and `dotf install` use, so the test
// cannot drift from the engine. This is also the sole guard on ADR-2's one real
// regression risk: now that `names` is the applicability declaration, a dropped
// key is a silent applicable=false row rather than a loud probe error, and this
// test is what makes that silence safe.
func TestEmbeddedManifest_ToolsReachableOnSupportedManagers(t *testing.T) {
	m := loadEmbeddedManifest(t)

	referenced := make(map[string]bool)
	for _, intent := range m.Intents {
		for _, name := range intent.Packages {
			referenced[name] = true
		}
	}
	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}

	// Reverse-index the sibling groups so a group is evaluated once, as a unit,
	// no matter which member the manifest scan reaches first.
	groupOf := make(map[string][]string)
	for _, group := range siblingGroups {
		for _, name := range group {
			if _, ok := byName[name]; !ok {
				t.Fatalf("siblingGroups names %q, which is not a [[package]] in the embedded manifest", name)
			}
			groupOf[name] = group
		}
	}

	engine := backend.Default(exec.NewFake(), privilegetest.NewFakeDropper(), t.TempDir(), nil)

	seen := make(map[string]bool)
	for _, pkg := range m.Packages {
		if pkg.Backend != manifest.BackendPkg || !referenced[pkg.Name] {
			continue
		}
		// An os-gated pkg entry (lutris-pkg) is by construction a platform-specific
		// install route, so all-hosts reachability is not merely unmet but
		// unsatisfiable for it: osGateAllows makes it applicable=false on every host
		// outside its gate. The contract is about CROSS-PLATFORM tools — an entry
		// with no os gate claims every supported manager, and that claim is what
		// this test holds it to. Its siblings still count as providers when the
		// ungated member of the group is the subject (lazygit -> lazygit-bin).
		if pkg.OS != "" {
			continue
		}
		providers, grouped := groupOf[pkg.Name]
		if !grouped {
			providers = []string{pkg.Name}
		}
		key := strings.Join(providers, "+")
		if seen[key] {
			continue
		}
		seen[key] = true

		t.Run(key, func(t *testing.T) {
			for _, host := range supportedManagerHosts {
				var reachable []string
				for _, name := range providers {
					ok, err := engine.Applicable(byName[name], host.info)
					if err != nil {
						t.Fatalf("Applicable(%q, %s host) error = %v", name, host.mgr, err)
					}
					if ok {
						reachable = append(reachable, name)
					}
				}
				if len(reachable) == 0 {
					t.Errorf("tool %q has no reachable install path on a %s host (os=%s): providers %v are all applicable=false; add a %s name or an os-gated sibling entry",
						key, host.mgr, host.info.OS, providers, host.mgr)
				}
			}
		})
	}
}

// TestEmbeddedManifest_CustomScriptFilesExist asserts every custom-script
// package points at an installer that actually exists in the repo, so a typo or
// a renamed/removed bin/install-*.sh is caught at build time rather than
// surfacing as a runtime "no such file" failure mid-apply. The test runs from
// the package directory (internal/manifest), so the repo root is two levels up.
func TestEmbeddedManifest_CustomScriptFilesExist(t *testing.T) {
	m := loadEmbeddedManifest(t)
	repoRoot := filepath.Join("..", "..")

	for _, pkg := range m.Packages {
		if pkg.Backend != manifest.BackendCustomScript {
			continue
		}
		t.Run(pkg.Name, func(t *testing.T) {
			if pkg.Script == "" {
				t.Fatalf("custom-script package %q has an empty script field", pkg.Name)
			}
			path := filepath.Join(repoRoot, filepath.FromSlash(pkg.Script))
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("script for package %q not found at %q: %v", pkg.Name, path, err)
			}
			if info.IsDir() {
				t.Errorf("script for package %q resolves to a directory: %q", pkg.Name, path)
			}
		})
	}
}

// TestEmbeddedManifest_CustomScriptClassified is the governance ratchet: every
// custom-script package in the embedded manifest must declare an explicit, valid
// class AND a non-empty justification (validateClass enforces this at parse time,
// so an unclassified entry would already fail loadEmbeddedManifest — this test
// pins the intent so a future entry cannot regress the escape-hatch fence).
func TestEmbeddedManifest_CustomScriptClassified(t *testing.T) {
	m := loadEmbeddedManifest(t)

	for _, pkg := range m.Packages {
		if pkg.Backend != manifest.BackendCustomScript {
			continue
		}
		t.Run(pkg.Name, func(t *testing.T) {
			if !pkg.Class.Valid() {
				t.Errorf("custom-script package %q has class %q, want an explicit pinned/rolling/managed", pkg.Name, pkg.Class)
			}
			if strings.TrimSpace(pkg.Justification) == "" {
				t.Errorf("custom-script package %q has no justification", pkg.Name)
			}
		})
	}
}

// TestEmbeddedManifest_Phase3Migrations pins the Phase 3 (#50) vertical
// script-migration cutover: the three tools moved off custom-script onto native
// backends must keep their migrated shape, and their retired bin/install-*.sh
// installers must stay deleted. This is the regression fence that stops a future
// manifest edit from silently reverting a migration or resurrecting a retired
// installer (which would also re-lie via the custom-script LookPath fallback).
func TestEmbeddedManifest_Phase3Migrations(t *testing.T) {
	m := loadEmbeddedManifest(t)

	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}

	t.Run("turso is a pinned github-release", func(t *testing.T) {
		p, ok := byName["turso"]
		if !ok {
			t.Fatal("turso package missing from embedded manifest")
		}
		if p.Backend != manifest.BackendGitHubRelease {
			t.Errorf("turso backend = %q, want github-release", p.Backend)
		}
		if p.Repo != "tursodatabase/turso-cli" {
			t.Errorf("turso repo = %q, want tursodatabase/turso-cli", p.Repo)
		}
		if p.Tag == "" {
			t.Error("turso tag is empty; a pinned github-release must pin a tag")
		}
		if p.Binary != "turso" {
			t.Errorf("turso binary = %q, want turso (asset is turso-cli_<os>_<arch>.tar.gz)", p.Binary)
		}
		if p.ChecksumsAsset == "" {
			t.Error("turso checksums_asset is empty; a pinned github-release needs a checksum source")
		}
		if got := p.EffectiveClass(); got != manifest.ClassPinned {
			t.Errorf("turso effective class = %q, want pinned", got)
		}
	})

	t.Run("opencode V2 uses migration-aware installer", func(t *testing.T) {
		p, ok := byName["opencode"]
		if !ok {
			t.Fatal("opencode package missing from embedded manifest")
		}
		if p.Backend != manifest.BackendCustomScript {
			t.Errorf("opencode backend = %q, want custom-script", p.Backend)
		}
		if p.Script != "bin/install-opencode.sh" {
			t.Errorf("opencode script = %q, want bin/install-opencode.sh", p.Script)
		}
		if p.InstalledWhen == nil || p.InstalledWhen.Path != "~/.local/state/dotf/opencode-v2" {
			t.Errorf("opencode installed_when = %+v, want V2 migration marker", p.InstalledWhen)
		}
		if len(p.Provides) != 0 {
			t.Errorf("opencode provides = %v, want empty (V1 shares the binary name)", p.Provides)
		}
		if _, exists := byName["opencode2"]; exists {
			t.Error("legacy side-by-side opencode2 manifest package still exists")
		}
	})

	t.Run("codex uses native standalone installer", func(t *testing.T) {
		p, ok := byName["codex"]
		if !ok {
			t.Fatal("codex package missing from embedded manifest")
		}
		if p.Backend != manifest.BackendCustomScript || p.Script != "bin/install-codex.sh" {
			t.Fatalf("codex must use bin/install-codex.sh, got backend=%q script=%q", p.Backend, p.Script)
		}
		// A PATH probe would treat an npm-installed codex as converged and skip migration.
		if p.InstalledWhen == nil || !slices.Equal(p.InstalledWhen.Paths, []string{
			"~/.codex/packages/standalone/current/bin/codex",
			"~/.codex/packages/app-server-daemon/current/bin/codex",
		}) {
			t.Errorf("codex installed_when = %+v, want standalone CLI and app-server daemon markers", p.InstalledWhen)
		}
		if !slices.Equal(p.Actions, []string{"update", "remove"}) {
			t.Errorf("codex actions = %v, want [update remove]", p.Actions)
		}
	})

	for _, tc := range []struct {
		name        string
		installName string
	}{
		{name: "vercel", installName: "vercel"},
		{name: "railway", installName: "@railway/cli"},
		{name: "stripe", installName: "@stripe/cli"},
		{name: "neon", installName: "neonctl"},
		{name: "lefthook", installName: "lefthook"},
	} {
		t.Run(tc.name+" is a managed toolchain/npm package", func(t *testing.T) {
			p, ok := byName[tc.name]
			if !ok {
				t.Fatalf("%s package missing from embedded manifest", tc.name)
			}
			if p.Backend != manifest.BackendToolchain {
				t.Errorf("%s backend = %q, want toolchain", tc.name, p.Backend)
			}
			if p.Tool != "npm" {
				t.Errorf("%s tool = %q, want npm", tc.name, p.Tool)
			}
			if got := p.EffectiveClass(); got != manifest.ClassManaged {
				t.Errorf("%s effective class = %q, want managed", tc.name, got)
			}
			installName := p.Name
			if mapped := p.Names["npm"]; mapped != "" {
				installName = mapped
			}
			if installName != tc.installName {
				t.Errorf("%s npm install name = %q, want %q", tc.name, installName, tc.installName)
			}
		})
	}

	// A migration changes the backend, not the intent: each tool must stay in the
	// same opt-in intent it belonged to as a custom-script entry.
	for _, tc := range []struct{ pkg, intent string }{
		{pkg: "turso", intent: "cloud"},
		{pkg: "vercel", intent: "cloud"},
		{pkg: "railway", intent: "cloud"},
		{pkg: "stripe", intent: "cloud"},
		{pkg: "neon", intent: "cloud"},
		{pkg: "codex", intent: "ai"},
		{pkg: "lefthook", intent: "git-tools"},
	} {
		t.Run(tc.pkg+" stays in intent "+tc.intent, func(t *testing.T) {
			in, ok := m.Intents[tc.intent]
			if !ok {
				t.Fatalf("intent %q missing from embedded manifest", tc.intent)
			}
			if !slices.Contains(in.Packages, tc.pkg) {
				t.Errorf("intent %q packages = %v, want to contain %q", tc.intent, in.Packages, tc.pkg)
			}
		})
	}

	// The retired installers must not reappear as any package's script, and the
	// files themselves must stay deleted from the repo.
	repoRoot := filepath.Join("..", "..")
	for _, script := range []string{
		"bin/install-turso.sh",
		"bin/install-lefthook.sh",
	} {
		t.Run("retired "+script, func(t *testing.T) {
			for _, pkg := range m.Packages {
				if pkg.Script == script {
					t.Errorf("package %q still references retired script %q", pkg.Name, script)
				}
			}
			if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(script))); !os.IsNotExist(err) {
				t.Errorf("retired script %q still present (stat err = %v), want removed", script, err)
			}
		})
	}
}

// TestEmbeddedManifest_PlaywrightCLI pins the playwright-cli entry: it installs
// the npm package AND fans the bundled agent skill out to every per-agent skill
// directory, which upstream's `playwright-cli install --skills` cannot do (it
// writes only .claude or .agents). That fan-out is why this is a custom-script
// entry rather than a toolchain/npm one, so a future migration to toolchain
// would silently drop the skill for Cursor, OpenCode, and Antigravity.
func TestEmbeddedManifest_PlaywrightCLI(t *testing.T) {
	m := loadEmbeddedManifest(t)

	var pkg manifest.Package
	var found bool
	for _, p := range m.Packages {
		if p.Name == "playwright-cli" {
			pkg, found = p, true
			break
		}
	}
	if !found {
		t.Fatal("playwright-cli package missing from embedded manifest")
	}

	if pkg.Backend != manifest.BackendCustomScript {
		t.Errorf("playwright-cli backend = %q, want custom-script (the skill fan-out is not expressible on toolchain/npm)", pkg.Backend)
	}
	if pkg.Script != "bin/install-playwright-cli.sh" {
		t.Errorf("playwright-cli script = %q, want bin/install-playwright-cli.sh", pkg.Script)
	}
	if !slices.Equal(pkg.Provides, []string{"playwright-cli"}) {
		t.Errorf("playwright-cli provides = %v, want [playwright-cli]", pkg.Provides)
	}
	if got := pkg.EffectiveClass(); got != manifest.ClassRolling {
		t.Errorf("playwright-cli effective class = %q, want rolling", got)
	}

	ai, ok := m.Intents["ai"]
	if !ok {
		t.Fatal("intent \"ai\" missing from embedded manifest")
	}
	if !slices.Contains(ai.Packages, "playwright-cli") {
		t.Errorf("intent ai packages = %v, want to contain playwright-cli", ai.Packages)
	}
}

// TestEmbeddedManifest_GitHubMCPServer pins github-mcp-server as a native,
// pinned github-release in the opt-in ai intent (no custom-script spend).
func TestEmbeddedManifest_GitHubMCPServer(t *testing.T) {
	m := loadEmbeddedManifest(t)

	var pkg manifest.Package
	var found bool
	for _, p := range m.Packages {
		if p.Name == "github-mcp-server" {
			pkg, found = p, true
			break
		}
	}
	if !found {
		t.Fatal("github-mcp-server package missing from embedded manifest")
	}

	if pkg.Backend != manifest.BackendGitHubRelease {
		t.Errorf("backend = %q, want github-release", pkg.Backend)
	}
	if pkg.Repo != "github/github-mcp-server" {
		t.Errorf("repo = %q, want github/github-mcp-server", pkg.Repo)
	}
	if pkg.Tag != "v2.0.2" {
		t.Errorf("tag = %q, want v2.0.2", pkg.Tag)
	}
	if pkg.Binary != "github-mcp-server" {
		t.Errorf("binary = %q, want github-mcp-server", pkg.Binary)
	}
	if pkg.ChecksumsAsset != "github-mcp-server_2.0.2_checksums.txt" {
		t.Errorf("checksums_asset = %q, want github-mcp-server_2.0.2_checksums.txt", pkg.ChecksumsAsset)
	}
	if pkg.Asset != "" {
		t.Errorf("asset = %q, want empty (auto-detected)", pkg.Asset)
	}
	if got := pkg.EffectiveClass(); got != manifest.ClassPinned {
		t.Errorf("effective class = %q, want pinned", got)
	}

	ai, ok := m.Intents["ai"]
	if !ok {
		t.Fatal("intent \"ai\" missing from embedded manifest")
	}
	if !slices.Contains(ai.Packages, "github-mcp-server") {
		t.Errorf("intent ai packages = %v, want to contain github-mcp-server", ai.Packages)
	}
	if ai.Default {
		t.Error("intent ai must stay opt-in (default = false)")
	}
}

// TestEmbeddedManifest_RetainedCustomScriptSentinels pins the Phase 3 (#51)
// installed-state sentinels: every retained custom-script entry carries an
// explicit probe naming the real binary/binaries it installs, so status/diff
// stop reporting a group name as a binary and apply skips a satisfied setup
// step. As of #81/#82 there are no remaining deferrals — fabric probes an
// any-of (fabric|fabric-ai) and gnu-utilities carries a macOS-scoped provides
// sentinel behind an os gate. The trailing fence stops a future custom-script
// entry from regressing to a lying LookPath(name) fallback.
func TestEmbeddedManifest_RetainedCustomScriptSentinels(t *testing.T) {
	m := loadEmbeddedManifest(t)

	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}

	// Expected provides for every sentineled retained custom-script package.
	// name==binary entries still declare it explicitly; the load-bearing cases
	// are name!=binary (azure-cli->az, tmux-sessionizer->tms) and the all-present
	// bundles (aws-tools, hashicorp-tools, container-tools).
	want := map[string][]string{
		"neovim":           {"nvim"},
		"atuin":            {"atuin"},
		"mise":             {"mise"},
		"rust":             {"cargo", "rustc"},
		"lua":              {"luarocks"},
		"claude":           {"claude"},
		"omp":              {"omp"},
		"coderabbit":       {"coderabbit"},
		"playwright-cli":   {"playwright-cli"},
		"1password-cli":    {"op"},
		"aws-tools":        {"aws", "sam", "ecs-cli"},
		"azure-cli":        {"az"},
		"hashicorp-tools":  {"terraform", "tflint"},
		"container-tools":  {"skaffold", "hadolint"},
		"tea":              {"tea"},
		"tmux-sessionizer": {"tms"},
		"gnu-utilities":    {"gls", "gsed", "gtar", "ggrep"},
	}

	for name, provides := range want {
		t.Run(name, func(t *testing.T) {
			p, ok := byName[name]
			if !ok {
				t.Fatalf("%s package missing from embedded manifest", name)
			}
			if p.Backend != manifest.BackendCustomScript {
				t.Fatalf("%s backend = %q, want custom-script", name, p.Backend)
			}
			if !slices.Equal(p.Provides, provides) {
				t.Errorf("%s provides = %v, want %v", name, p.Provides, provides)
			}
		})
	}

	// Expected provides_any for the packages whose installed binary name varies
	// by platform: fabric probes an any-of (fabric|fabric-ai); lua combines an
	// all-present provides=[luarocks] (asserted above) with an any-of over its
	// interpreter names, so it appears in BOTH maps.
	wantAny := map[string][]string{
		"fabric": {"fabric", "fabric-ai"},
		"lua":    {"lua", "lua5.1", "luajit"},
	}
	for name, providesAny := range wantAny {
		t.Run(name+" provides_any", func(t *testing.T) {
			p, ok := byName[name]
			if !ok {
				t.Fatalf("%s package missing from embedded manifest", name)
			}
			if p.Backend != manifest.BackendCustomScript {
				t.Fatalf("%s backend = %q, want custom-script", name, p.Backend)
			}
			if !slices.Equal(p.ProvidesAny, providesAny) {
				t.Errorf("%s provides_any = %v, want %v", name, p.ProvidesAny, providesAny)
			}
		})
	}

	// OpenCode V1 and V2 share one command name, so its explicit sentinel must be
	// the V2-only marker written after verification, not a PATH probe.
	t.Run("opencode V2 marker", func(t *testing.T) {
		p, ok := byName["opencode"]
		if !ok {
			t.Fatal("opencode package missing from embedded manifest")
		}
		if p.InstalledWhen == nil || p.InstalledWhen.Path != "~/.local/state/dotf/opencode-v2" {
			t.Errorf("opencode installed_when = %+v, want V2 marker", p.InstalledWhen)
		}
	})

	// gnu-utilities' macOS-only g-prefixed sentinel must be scoped by an os gate
	// so it is not evaluated (and always missed) on Linux.
	t.Run("gnu-utilities os gate", func(t *testing.T) {
		p, ok := byName["gnu-utilities"]
		if !ok {
			t.Fatalf("gnu-utilities package missing from embedded manifest")
		}
		if p.OS != "macos" {
			t.Errorf("gnu-utilities os = %q, want macos", p.OS)
		}
	})

	// Fence: every custom-script package must now carry a sentinel (provides,
	// provides_any, or installed_when) — the Phase-3 deferrals (fabric, gnu-
	// utilities) are resolved, so a new entry cannot silently fall back to a
	// LookPath(name) that lies for group/version-suffixed names.
	for _, pkg := range m.Packages {
		if pkg.Backend != manifest.BackendCustomScript {
			continue
		}
		if len(pkg.Provides) == 0 && len(pkg.ProvidesAny) == 0 && pkg.InstalledWhen == nil {
			t.Errorf("custom-script package %q has no installed-state sentinel (provides/provides_any/installed_when); add one", pkg.Name)
		}
	}
}

func TestLoadFS_MissingFile(t *testing.T) {
	dir := t.TempDir()

	_, err := manifest.LoadFS(os.DirFS(dir), "does-not-exist.toml")
	if err == nil {
		t.Fatal("LoadFS() error = nil, want error for missing file")
	}
}

func TestResolve_Precedence(t *testing.T) {
	flagContent := mustReadFile(t, filepath.Join("testdata", "resolve_flag.toml"))
	envContent := mustReadFile(t, filepath.Join("testdata", "resolve_env.toml"))
	embeddedContent := mustReadFile(t, filepath.Join("testdata", "valid.toml"))

	dir := t.TempDir()
	flagPath := filepath.Join(dir, "flag.toml")
	envPath := filepath.Join(dir, "env.toml")

	if err := os.WriteFile(flagPath, []byte(flagContent), 0o644); err != nil {
		t.Fatalf("WriteFile(flag) error = %v", err)
	}
	if err := os.WriteFile(envPath, []byte(envContent), 0o644); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}

	embedded := fstest.MapFS{
		"manifest/packages.toml": {Data: []byte(embeddedContent)},
	}

	tests := []struct {
		name     string
		flagPath string
		envVal   string
		wantDesc string
	}{
		{name: "flag wins over env and embedded", flagPath: flagPath, envVal: envPath, wantDesc: "loaded-from-flag"},
		{name: "env wins over embedded when flag unset", flagPath: "", envVal: envPath, wantDesc: "loaded-from-env"},
		{name: "embedded used when flag and env unset", flagPath: "", envVal: "", wantDesc: "Minimal baseline tools for any machine"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := manifest.Resolve(tt.flagPath, tt.envVal, embedded, manifest.ResolveOptions{})
			if err != nil {
				t.Fatalf("Resolve(%q, %q) error = %v, want nil", tt.flagPath, tt.envVal, err)
			}
			if got := m.Intents["base"].Description; got != tt.wantDesc {
				t.Errorf("Resolve(%q, %q) intents[base].Description = %q, want %q", tt.flagPath, tt.envVal, got, tt.wantDesc)
			}
		})
	}
}

func TestResolve_UnreadableOverride_ErrorsWithoutEmbeddedFallback(t *testing.T) {
	embeddedContent := mustReadFile(t, filepath.Join("testdata", "valid.toml"))
	embedded := fstest.MapFS{
		"manifest/packages.toml": {Data: []byte(embeddedContent)},
	}

	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")

	t.Run("unreadable flag path", func(t *testing.T) {
		m, err := manifest.Resolve(missing, "", embedded, manifest.ResolveOptions{})
		if err == nil {
			t.Fatal("Resolve() error = nil, want error for unreadable flag path")
		}
		if m != nil {
			t.Errorf("Resolve() manifest = %+v, want nil (must not silently fall back to embedded)", m)
		}
	})

	t.Run("unreadable env path", func(t *testing.T) {
		m, err := manifest.Resolve("", missing, embedded, manifest.ResolveOptions{})
		if err == nil {
			t.Fatal("Resolve() error = nil, want error for unreadable env path")
		}
		if m != nil {
			t.Errorf("Resolve() manifest = %+v, want nil (must not silently fall back to embedded)", m)
		}
	})
}

func TestResolve_EmbeddedDotfilesManifestFS(t *testing.T) {
	m, err := manifest.Resolve("", "", dotfiles.ManifestFS, manifest.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve(\"\", \"\", dotfiles.ManifestFS) error = %v, want nil", err)
	}
	if m == nil {
		t.Fatal("Resolve(\"\", \"\", dotfiles.ManifestFS) manifest = nil, want non-nil")
	}
}
