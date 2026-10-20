package manifest_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

const validSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// TestValidate_GitHubRelease covers the reworked github-release contract: Asset
// is optional, at least one checksum source is required (fail-closed), a tag is
// required whenever any checksum source is set, Checksums values/keys are
// validated, and Match.Libc is enum-checked.
func TestValidate_GitHubRelease(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		wantErr string // "" means the manifest must parse successfully
	}{
		{
			name: "asset optional: auto-detect with inline sha accepted",
			pkg: `
name = "tool"
backend = "github-release"
repo = "owner/tool"
tag = "v1.2.3"
sha256 = "` + validSHA + `"`,
			wantErr: "",
		},
		{
			name: "checksums map accepted without asset or sha256",
			pkg: `
name = "tool"
backend = "github-release"
repo = "owner/tool"
tag = "v1.2.3"
checksums = { "linux/x86_64" = "` + validSHA + `" }`,
			wantErr: "",
		},
		{
			name: "checksums_asset sidecar accepted",
			pkg: `
name = "tool"
backend = "github-release"
repo = "owner/tool"
tag = "v1.2.3"
checksums_asset = "checksums.txt"`,
			wantErr: "",
		},
		{
			name: "match.libc = musl accepted",
			pkg: `
name = "tool"
backend = "github-release"
repo = "owner/tool"
tag = "v1.2.3"
sha256 = "` + validSHA + `"
match = { libc = "musl" }`,
			wantErr: "",
		},
		{
			name: "no checksum source rejected (fail-closed)",
			pkg: `
name = "tool"
backend = "github-release"
repo = "owner/tool"
tag = "v1.2.3"
asset = "tool.tar.gz"`,
			wantErr: "requires a checksum source",
		},
		{
			name: "checksum source without tag rejected",
			pkg: `
name = "tool"
backend = "github-release"
repo = "owner/tool"
sha256 = "` + validSHA + `"`,
			wantErr: "requires tag when a checksum source is set",
		},
		{
			name: "bad checksums value rejected",
			pkg: `
name = "tool"
backend = "github-release"
repo = "owner/tool"
tag = "v1.2.3"
checksums = { "linux/x86_64" = "NOThex" }`,
			wantErr: "not 64-char lowercase hex",
		},
		{
			name: "empty checksums key rejected",
			pkg: `
name = "tool"
backend = "github-release"
repo = "owner/tool"
tag = "v1.2.3"
checksums = { "" = "` + validSHA + `" }`,
			wantErr: "checksums key must be non-empty",
		},
		{
			name: "invalid match.libc rejected",
			pkg: `
name = "tool"
backend = "github-release"
repo = "owner/tool"
tag = "v1.2.3"
sha256 = "` + validSHA + `"
match = { libc = "glibc" }`,
			wantErr: "match.libc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "[[package]]" + tt.pkg + "\n"
			m, err := manifest.Parse(strings.NewReader(data))

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse() error = %v, want nil", err)
				}
				if m == nil {
					t.Fatal("Parse() manifest = nil, want non-nil")
				}
				return
			}

			if err == nil {
				t.Fatalf("Parse() error = nil, want %q", tt.wantErr)
			}
			var valErr *manifest.ValidationError
			if !errors.As(err, &valErr) {
				t.Fatalf("Parse() error = %v, want *manifest.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestValidate_AppArtifact covers the app-artifact contract: exactly one of
// repo/url (https), the github-release fail-closed checksum rule on the repo
// route, app.icon/https+relative-path shape, and the AppImage icon requirement.
func TestValidate_AppArtifact(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		wantErr string // "" means the manifest must parse successfully
	}{
		{
			name: "repo route with tag and sha256 accepted",
			pkg: `
name = "obsidian"
backend = "app-artifact"
repo = "obsidianmd/obsidian-releases"
tag = "v1.7.7"
asset = "Obsidian-1.7.7.AppImage"
sha256 = "` + validSHA + `"
[package.app]
icon = "obsidian.png"`,
			wantErr: "",
		},
		{
			name: "repo route with checksums_asset accepted",
			pkg: `
name = "krita"
backend = "app-artifact"
repo = "owner/krita"
tag = "v5"
checksums_asset = "SHA256SUMS"
app = { icon = "icons/krita.png", desktop = "krita.desktop" }`,
			wantErr: "",
		},
		{
			name: "url tarball without sha256 accepted",
			pkg: `
name = "zed"
backend = "app-artifact"
url = "https://zed.dev/api/releases/latest/zed-linux-x86_64.tar.gz"
app = { icon = "https://zed.dev/icon.png", desktop_name = "Zed", categories = "Development;", exec_args = "--new-window" }`,
			wantErr: "",
		},
		{
			name: "url AppImage with https icon accepted",
			pkg: `
name = "kdenlive"
backend = "app-artifact"
url = "https://download.kde.org/stable/kdenlive/kdenlive-24.12-x86_64.AppImage?mirrorlist"
sha256 = "` + validSHA + `"
app = { icon = "https://kdenlive.org/wp-content/icon.png" }`,
			wantErr: "",
		},
		{
			name: "both repo and url rejected",
			pkg: `
name = "app"
backend = "app-artifact"
repo = "owner/app"
url = "https://example.com/app.AppImage"`,
			wantErr: "exactly one of repo or url",
		},
		{
			name: "neither repo nor url rejected",
			pkg: `
name = "app"
backend = "app-artifact"`,
			wantErr: "exactly one of repo or url",
		},
		{
			name: "bad repo slug rejected",
			pkg: `
name = "app"
backend = "app-artifact"
repo = "owner/app/extra"`,
			wantErr: "must be owner/repo",
		},
		{
			name: "http url rejected",
			pkg: `
name = "app"
backend = "app-artifact"
url = "http://example.com/app.tar.gz"`,
			wantErr: "requires https url",
		},
		{
			name: "repo without checksum source rejected (fail-closed)",
			pkg: `
name = "app"
backend = "app-artifact"
repo = "owner/app"
tag = "v1"`,
			wantErr: "requires a checksum source",
		},
		{
			name: "repo checksum without tag rejected",
			pkg: `
name = "app"
backend = "app-artifact"
repo = "owner/app"
sha256 = "` + validSHA + `"`,
			wantErr: "requires tag when a checksum source is set",
		},
		{
			name: "bad checksums value rejected",
			pkg: `
name = "app"
backend = "app-artifact"
repo = "owner/app"
tag = "v1"
checksums = { "linux/x86_64" = "NOThex" }`,
			wantErr: "not 64-char lowercase hex",
		},
		{
			name: "http icon rejected",
			pkg: `
name = "app"
backend = "app-artifact"
url = "https://example.com/app.tar.gz"
app = { icon = "http://example.com/icon.png" }`,
			wantErr: "app.icon",
		},
		{
			name: "absolute icon rejected",
			pkg: `
name = "app"
backend = "app-artifact"
url = "https://example.com/app.tar.gz"
app = { icon = "/usr/share/icons/app.png" }`,
			wantErr: "must be relative",
		},
		{
			name: "icon with .. rejected",
			pkg: `
name = "app"
backend = "app-artifact"
url = "https://example.com/app.tar.gz"
app = { icon = "../icons/app.png" }`,
			wantErr: "must not contain ..",
		},
		{
			name: "desktop with .. rejected",
			pkg: `
name = "app"
backend = "app-artifact"
url = "https://example.com/app.tar.gz"
app = { desktop = "../evil.desktop" }`,
			wantErr: "app.desktop",
		},
		{
			name: "AppImage url without icon rejected",
			pkg: `
name = "app"
backend = "app-artifact"
url = "https://example.com/app-x86_64.AppImage"`,
			wantErr: "AppImage requires app.icon",
		},
		{
			name: "AppImage asset without icon rejected",
			pkg: `
name = "app"
backend = "app-artifact"
repo = "owner/app"
tag = "v1"
asset = "app.AppImage"
sha256 = "` + validSHA + `"`,
			wantErr: "AppImage requires app.icon",
		},
		{
			name: "app table rejected on script-url",
			pkg: `
name = "app"
backend = "script-url"
url = "https://example.com/install.sh"
app = { icon = "icon.png" }`,
			wantErr: "app is only valid for the app-artifact backend",
		},
	}
	runValidateCases(t, tests)
}

// TestValidate_PkgURL covers the pkg-url contract: exactly one of repo/url
// (https), required pkg_name, rpm/deb format resolution (explicit or from the
// url/asset suffix), gpg_key_url/fingerprint pairing, the allow_unsigned
// justification, and the deb checksum requirement.
func TestValidate_PkgURL(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		wantErr string // "" means the manifest must parse successfully
	}{
		{
			name: "rpm url with suffix inferred accepted",
			pkg: `
name = "slack"
backend = "pkg-url"
url = "https://downloads.slack-edge.com/desktop-releases/linux/x64/4.41/slack-4.41-0.1.el8.x86_64.rpm"
pkg_name = "slack"`,
			wantErr: "",
		},
		{
			name: "deb url with sha256 accepted",
			pkg: `
name = "code"
backend = "pkg-url"
url = "https://update.code.visualstudio.com/latest/linux-deb-x64/stable"
format = "deb"
pkg_name = "code"
sha256 = "` + validSHA + `"
gpg_key_url = "https://packages.microsoft.com/keys/microsoft.asc"
gpg_fingerprint = "BC528686B50D79E339D3721CEB3E94ADBE1229CF"`,
			wantErr: "",
		},
		{
			name: "deb repo route with checksums source accepted",
			pkg: `
name = "code"
backend = "pkg-url"
repo = "owner/code"
asset = "code_amd64.deb"
pkg_name = "code"
checksums_asset = "SHA256SUMS"`,
			wantErr: "",
		},
		{
			name: "allow_unsigned with justification accepted",
			pkg: `
name = "vendor-app"
backend = "pkg-url"
url = "https://vendor.example.com/vendor-app-latest.x86_64.rpm"
pkg_name = "vendor-app"
allow_unsigned = true
justification = "vendor publishes no signing key"`,
			wantErr: "",
		},
		{
			name: "both repo and url rejected",
			pkg: `
name = "app"
backend = "pkg-url"
repo = "owner/app"
url = "https://example.com/app.rpm"
pkg_name = "app"`,
			wantErr: "exactly one of repo or url",
		},
		{
			name: "neither repo nor url rejected",
			pkg: `
name = "app"
backend = "pkg-url"
pkg_name = "app"`,
			wantErr: "exactly one of repo or url",
		},
		{
			name: "http url rejected",
			pkg: `
name = "app"
backend = "pkg-url"
url = "http://example.com/app.rpm"
pkg_name = "app"`,
			wantErr: "requires https url",
		},
		{
			name: "missing pkg_name rejected",
			pkg: `
name = "app"
backend = "pkg-url"
url = "https://example.com/app.rpm"`,
			wantErr: "requires pkg_name",
		},
		{
			name: "invalid format rejected",
			pkg: `
name = "app"
backend = "pkg-url"
url = "https://example.com/app.rpm"
pkg_name = "app"
format = "snap"`,
			wantErr: "must be one of rpm, deb",
		},
		{
			name: "no suffix and no format rejected",
			pkg: `
name = "app"
backend = "pkg-url"
url = "https://example.com/download"
pkg_name = "app"`,
			wantErr: "requires format",
		},
		{
			name: "format conflicting with suffix rejected",
			pkg: `
name = "app"
backend = "pkg-url"
url = "https://example.com/app.rpm"
pkg_name = "app"
format = "deb"`,
			wantErr: "conflicts with the .rpm suffix",
		},
		{
			name: "gpg_key_url without fingerprint rejected",
			pkg: `
name = "app"
backend = "pkg-url"
url = "https://example.com/app.rpm"
pkg_name = "app"
gpg_key_url = "https://example.com/key.asc"`,
			wantErr: "gpg_key_url requires gpg_fingerprint",
		},
		{
			name: "http gpg_key_url rejected",
			pkg: `
name = "app"
backend = "pkg-url"
url = "https://example.com/app.rpm"
pkg_name = "app"
gpg_key_url = "http://example.com/key.asc"
gpg_fingerprint = "BC528686B50D79E339D3721CEB3E94ADBE1229CF"`,
			wantErr: "must be an https URL",
		},
		{
			name: "bad fingerprint rejected",
			pkg: `
name = "app"
backend = "pkg-url"
url = "https://example.com/app.rpm"
pkg_name = "app"
gpg_key_url = "https://example.com/key.asc"
gpg_fingerprint = "nothex"`,
			wantErr: "not 40-char hex",
		},
		{
			name: "allow_unsigned without justification rejected",
			pkg: `
name = "app"
backend = "pkg-url"
url = "https://example.com/app.rpm"
pkg_name = "app"
allow_unsigned = true`,
			wantErr: "allow_unsigned requires a justification",
		},
		{
			name: "deb without checksum rejected",
			pkg: `
name = "app"
backend = "pkg-url"
url = "https://example.com/app.deb"
pkg_name = "app"`,
			wantErr: "deb requires sha256",
		},
		{
			name: "pkg-url fields rejected on script-url",
			pkg: `
name = "app"
backend = "script-url"
url = "https://example.com/install.sh"
pkg_name = "app"`,
			wantErr: "pkg_name is only valid for the pkg-url backend",
		},
		{
			name: "format rejected on github-release",
			pkg: `
name = "tool"
backend = "github-release"
repo = "owner/tool"
tag = "v1"
sha256 = "` + validSHA + `"
format = "rpm"`,
			wantErr: "format is only valid for the pkg-url backend",
		},
		{
			name: "gpg fields rejected on github-release",
			pkg: `
name = "tool"
backend = "github-release"
repo = "owner/tool"
tag = "v1"
sha256 = "` + validSHA + `"
gpg_key_url = "https://example.com/key.asc"
gpg_fingerprint = "BC528686B50D79E339D3721CEB3E94ADBE1229CF"`,
			wantErr: "gpg_key_url is only valid for the pkg-url backend",
		},
		{
			name: "allow_unsigned rejected on github-release",
			pkg: `
name = "tool"
backend = "github-release"
repo = "owner/tool"
tag = "v1"
sha256 = "` + validSHA + `"
allow_unsigned = true`,
			wantErr: "allow_unsigned is only valid for the pkg-url backend",
		},
	}
	runValidateCases(t, tests)
}

// runValidateCases runs the shared table harness: each entry is a [[package]]
// body; wantErr "" means Parse must succeed, otherwise the returned error must
// be a *ValidationError containing wantErr.
func runValidateCases(t *testing.T, tests []struct {
	name    string
	pkg     string
	wantErr string
},
) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "[[package]]" + tt.pkg + "\n"
			m, err := manifest.Parse(strings.NewReader(data))

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse() error = %v, want nil", err)
				}
				if m == nil {
					t.Fatal("Parse() manifest = nil, want non-nil")
				}
				return
			}

			if err == nil {
				t.Fatalf("Parse() error = nil, want %q", tt.wantErr)
			}
			var valErr *manifest.ValidationError
			if !errors.As(err, &valErr) {
				t.Fatalf("Parse() error = %v, want *manifest.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestValidate_Actions covers the custom-script lifecycle actions capability
// field: it is accepted only on the custom-script backend, every entry must be a
// known lifecycle verb, and an empty/absent list is fine (install-only script).
func TestValidate_Actions(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		wantErr string // "" means the manifest must parse successfully
	}{
		{
			name: "accepted on custom-script",
			pkg: `
name = "claude"
backend = "custom-script"
script = "bin/install-claude.sh"
actions = ["update", "remove"]
class = "rolling"
justification = "vendor installer tracks latest"`,
			wantErr: "",
		},
		{
			name: "install verb accepted",
			pkg: `
name = "claude"
backend = "custom-script"
script = "bin/install-claude.sh"
actions = ["install"]
class = "rolling"
justification = "vendor installer tracks latest"`,
			wantErr: "",
		},
		{
			name: "rejected on non-custom-script backend",
			pkg: `
name = "rg"
backend = "toolchain"
tool = "cargo"
actions = ["update"]`,
			wantErr: "actions is only valid for the custom-script backend",
		},
		{
			name: "unknown action rejected",
			pkg: `
name = "claude"
backend = "custom-script"
script = "bin/install-claude.sh"
actions = ["upgrade"]
class = "rolling"
justification = "vendor installer tracks latest"`,
			wantErr: `action "upgrade" must be one of install, update, remove`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "[[package]]" + tt.pkg + "\n"
			m, err := manifest.Parse(strings.NewReader(data))

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse() error = %v, want nil", err)
				}
				if m == nil {
					t.Fatal("Parse() manifest = nil, want non-nil")
				}
				return
			}

			if err == nil {
				t.Fatalf("Parse() error = nil, want %q", tt.wantErr)
			}
			var valErr *manifest.ValidationError
			if !errors.As(err, &valErr) {
				t.Fatalf("Parse() error = %v, want *manifest.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestValidate_Args covers the script-url installer argv field: it is accepted
// only on the script-url backend (where it drives a prompting vendor installer
// non-interactively), rejects an empty element, and an absent list is fine.
func TestValidate_Args(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		wantErr string // "" means the manifest must parse successfully
	}{
		{
			name: "accepted on script-url",
			pkg: `
name = "starship"
backend = "script-url"
url = "https://starship.rs/install.sh"
args = ["--yes"]`,
			wantErr: "",
		},
		{
			name: "absent is fine",
			pkg: `
name = "starship"
backend = "script-url"
url = "https://starship.rs/install.sh"`,
			wantErr: "",
		},
		{
			name: "rejected on non-script-url backend",
			pkg: `
name = "rg"
backend = "toolchain"
tool = "cargo"
args = ["--yes"]`,
			wantErr: "args is only valid for the script-url backend",
		},
		{
			name: "empty element rejected",
			pkg: `
name = "starship"
backend = "script-url"
url = "https://starship.rs/install.sh"
args = ["--yes", ""]`,
			wantErr: "args must not contain an empty element",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "[[package]]" + tt.pkg + "\n"
			m, err := manifest.Parse(strings.NewReader(data))

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse() error = %v, want nil", err)
				}
				if m == nil {
					t.Fatal("Parse() manifest = nil, want non-nil")
				}
				return
			}

			if err == nil {
				t.Fatalf("Parse() error = nil, want %q", tt.wantErr)
			}
			var valErr *manifest.ValidationError
			if !errors.As(err, &valErr) {
				t.Fatalf("Parse() error = %v, want *manifest.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate_DNFExclude(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		wantErr string
	}{
		{"accepted on pkg", `
name = "vlc"
backend = "pkg"
names = { dnf = "vlc" }
dnf_exclude = ["kf6-kmime*"]`, ""},
		{"rejected off pkg", `
name = "starship"
backend = "script-url"
url = "https://starship.rs/install.sh"
dnf_exclude = ["kf6-kmime*"]`, "dnf_exclude is only valid on the pkg backend"},
		{"empty entry rejected", `
name = "vlc"
backend = "pkg"
names = { dnf = "vlc" }
dnf_exclude = [""]`, "dnf_exclude entry"},
		{"whitespace rejected", `
name = "vlc"
backend = "pkg"
names = { dnf = "vlc" }
dnf_exclude = ["a b"]`, "dnf_exclude entry"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manifest.Parse(strings.NewReader("[[package]]" + tt.pkg + "\n"))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Parse() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

// TestValidate_PackageOS covers the optional package os platform gate: the
// accepted token set (macos/windows/linux + the distro families), an absent
// gate, and rejection of unknown tokens — including "wsl", which is
// intentionally not accepted (a modern WSL host resolves to a distro family).
func TestValidate_PackageOS(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		wantErr string // "" means the manifest must parse successfully
	}{
		{name: "no gate", pkg: `
name = "tool"
backend = "pkg"
names = { pacman = "tool" }`, wantErr: ""},
		{name: "macos accepted", pkg: `
name = "tool"
backend = "pkg"
names = { pacman = "tool" }
os = "macos"`, wantErr: ""},
		{name: "linux family accepted", pkg: `
name = "tool"
backend = "pkg"
names = { pacman = "tool" }
os = "linux"`, wantErr: ""},
		{name: "distro token accepted", pkg: `
name = "tool"
backend = "pkg"
names = { pacman = "tool" }
os = "arch"`, wantErr: ""},
		{name: "windows accepted", pkg: `
name = "tool"
backend = "pkg"
names = { pacman = "tool" }
os = "windows"`, wantErr: ""},
		{name: "unknown token rejected", pkg: `
name = "tool"
backend = "pkg"
names = { pacman = "tool" }
os = "darwin"`, wantErr: `os "darwin" must be one of`},
		{name: "wsl intentionally rejected", pkg: `
name = "tool"
backend = "pkg"
names = { pacman = "tool" }
os = "wsl"`, wantErr: `os "wsl" must be one of`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "[[package]]" + tt.pkg + "\n"
			m, err := manifest.Parse(strings.NewReader(data))

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse() error = %v, want nil", err)
				}
				if m == nil {
					t.Fatal("Parse() manifest = nil, want non-nil")
				}
				return
			}

			if err == nil {
				t.Fatalf("Parse() error = nil, want %q", tt.wantErr)
			}
			var valErr *manifest.ValidationError
			if !errors.As(err, &valErr) {
				t.Fatalf("Parse() error = %v, want *manifest.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestValidate_KeyFingerprint covers the optional apt_repo.key_fingerprint:
// a 40-hex value (either case) or empty is accepted; anything else fails.
func TestValidate_KeyFingerprint(t *testing.T) {
	tests := []struct {
		name    string
		fp      string
		wantErr string
	}{
		{name: "40 hex upper", fp: "31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE"},
		{name: "40 hex lower", fp: "31ddde24ddfab679f42d7bd2baa929ff1a7ecace"},
		{name: "empty", fp: ""},
		{name: "garbage", fp: "not-a-fingerprint", wantErr: "key_fingerprint"},
		{name: "too short", fp: "31DDDE24", wantErr: "key_fingerprint"},
		{name: "non-hex 40", fp: strings.Repeat("Z", 40), wantErr: "key_fingerprint"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := `[[package]]
name = "tool"
backend = "repo"
[package.apt_repo]
key_url = "https://example.com/key.asc"
keyring = "/usr/share/keyrings/t.gpg"
repo_line = "deb https://example.com stable main"
sources_path = "/etc/apt/sources.list.d/t.list"
key_fingerprint = "` + tt.fp + `"
`
			_, err := manifest.Parse(strings.NewReader(data))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Parse() error = nil, want %q", tt.wantErr)
			}
			var valErr *manifest.ValidationError
			if !errors.As(err, &valErr) {
				t.Fatalf("Parse() error = %v, want *manifest.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestValidate_DNFRepoGPGCheck covers dnf_repo.repo_gpgcheck: accepted only in
// baseurl mode with a gpgkey, since it is rendered into the dotf-built body
// and dnf needs the key to verify the metadata signature.
func TestValidate_DNFRepoGPGCheck(t *testing.T) {
	tests := []struct {
		name    string
		repo    string
		wantErr string
	}{
		{name: "baseurl with gpgkey", repo: "id = \"t\"\nbaseurl = \"https://example.com/rpm\"\ngpgkey = \"https://example.com/key.asc\"\n"},
		{name: "baseurl without gpgkey", repo: "id = \"t\"\nbaseurl = \"https://example.com/rpm\"\n", wantErr: "repo_gpgcheck requires gpgkey"},
		{name: "repofile mode", repo: "id = \"t\"\nrepofile = \"https://example.com/t.repo\"\n", wantErr: "repo_gpgcheck applies only to baseurl mode"},
		{name: "copr mode", repo: "copr = \"owner/project\"\n", wantErr: "repo_gpgcheck applies only to baseurl mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "[[package]]\nname = \"tool\"\nbackend = \"repo\"\n[package.dnf_repo]\nrepo_gpgcheck = true\n" + tt.repo
			_, err := manifest.Parse(strings.NewReader(data))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse() error = %v, want nil", err)
				}
				return
			}
			var valErr *manifest.ValidationError
			if !errors.As(err, &valErr) {
				t.Fatalf("Parse() error = %v, want *manifest.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestValidate_SSHServer covers the optional [sysconfig] ssh_server enum:
// on/off/empty are accepted, unknown values fail with *ValidationError, and
// ssh_server decodes under DisallowUnknownFields (schema field, not strict
// decode rejection).
func TestValidate_SSHServer(t *testing.T) {
	tests := []struct {
		name      string
		sysconfig string
		wantErr   string // "" means the manifest must parse successfully
		wantSSH   string // expected SSHServer after parse; ignored when wantErr != ""
	}{
		{
			name:      "on accepted",
			sysconfig: `ssh_server = "on"`,
			wantSSH:   "on",
		},
		{
			name:      "off accepted",
			sysconfig: `ssh_server = "off"`,
			wantSSH:   "off",
		},
		{
			name:      "empty explicit accepted",
			sysconfig: `ssh_server = ""`,
			wantSSH:   "",
		},
		{
			name:      "empty omitted accepted",
			sysconfig: `firewall = "ufw"`,
			wantSSH:   "",
		},
		{
			name:      "invalid value rejected",
			sysconfig: `ssh_server = "bogus"`,
			wantErr:   `sysconfig ssh_server "bogus" must be one of on, off`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "[sysconfig]\n" + tt.sysconfig + "\n"
			m, err := manifest.Parse(strings.NewReader(data))

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse() error = %v, want nil", err)
				}
				if m == nil || m.Sysconfig == nil {
					t.Fatal("Parse() sysconfig = nil, want non-nil [sysconfig]")
				}
				if m.Sysconfig.SSHServer != tt.wantSSH {
					t.Errorf("SSHServer = %q, want %q", m.Sysconfig.SSHServer, tt.wantSSH)
				}
				return
			}

			if err == nil {
				t.Fatalf("Parse() error = nil, want %q", tt.wantErr)
			}
			var valErr *manifest.ValidationError
			if !errors.As(err, &valErr) {
				t.Fatalf("Parse() error = %v, want *manifest.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateRequiresAndArch(t *testing.T) {
	m := &manifest.Manifest{Packages: []manifest.Package{{Name: "app", Backend: manifest.BackendPkg, Names: map[string]string{"dnf": "app"}, Arch: []string{"ppc64le"}, Requires: []string{"missing", "app"}}}}
	err := manifest.Validate(m)
	if err == nil {
		t.Fatal("Validate accepted invalid requires/arch")
	}
	for _, want := range []string{"arch \"ppc64le\"", "requires unknown or self package \"missing\"", "requires unknown or self package \"app\""} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}
