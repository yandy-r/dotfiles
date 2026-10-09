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
