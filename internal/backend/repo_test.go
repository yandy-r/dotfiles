package backend

import (
	"errors"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// TestRepoBackend_Install_ExactArgv pins the full command sequence each dnf_repo
// mode issues, plus the apt path the generalisation lifted: the backend writes
// under /etc with sudo, so the argv is the contract, not "no error". The apt
// case is the no-regression proof that folding apt-repo into repo left the live
// docker-ce Debian path byte-identical.
func TestRepoBackend_Install_ExactArgv(t *testing.T) {
	t.Parallel()

	const (
		dockerRepoFile    = "https://download.docker.com/linux/fedora/docker-ce.repo"
		dockerKeyURL      = "https://download.docker.com/linux/ubuntu/gpg"
		dockerKeyring     = "/etc/apt/keyrings/docker.gpg"
		dockerSourcesPath = "/etc/apt/sources.list.d/docker.list"
		dockerRepoLine    = "deb [arch={{.Arch}} signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu {{.Codename}} stable"
		exampleBaseURL    = "https://example.com/fedora/{{.ReleaseVer}}/{{.Arch}}"
	)

	tests := []struct {
		name      string
		pkg       manifest.Package
		info      platform.Info
		stub      func(r *exec.Fake)
		wantCalls int
		assert    func(t *testing.T, calls []exec.FakeCall, info platform.Info)
	}{
		{
			name: "dnf repofile installs the vendor file then the package",
			pkg: manifest.Package{
				Name:    "docker-ce",
				Backend: manifest.BackendRepo,
				Names:   map[string]string{"dnf": "docker-ce"},
				DNFRepo: &manifest.DNFRepo{ID: "docker-ce", RepoFile: dockerRepoFile},
			},
			info: platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrDNF, Arch: platform.ArchARM64, VersionID: "42"},
			stub: func(r *exec.Fake) {
				r.AddCommand("curl", []string{"-fsSL", dockerRepoFile}, []byte("[docker-ce-stable]\n"), nil)
			},
			wantCalls: 3,
			assert: func(t *testing.T, calls []exec.FakeCall, _ platform.Info) {
				t.Helper()
				assertCall(t, calls[0], "curl", "-fsSL", dockerRepoFile)
				assertSudoInstallSources(t, calls[1], "/etc/yum.repos.d/docker-ce.repo")
				assertSudoCall(t, calls[2], "dnf", "install", "-y", "docker-ce")
			},
		},
		{
			name: "dnf explicit baseurl renders the body without a fetch",
			pkg: manifest.Package{
				Name:    "example",
				Backend: manifest.BackendRepo,
				Names:   map[string]string{"dnf": "example"},
				DNFRepo: &manifest.DNFRepo{
					ID:      "example",
					BaseURL: exampleBaseURL,
					GPGKey:  "https://example.com/RPM-GPG-KEY",
				},
			},
			info:      platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrDNF, Arch: platform.ArchX86_64, VersionID: "42"},
			wantCalls: 2,
			assert: func(t *testing.T, calls []exec.FakeCall, info platform.Info) {
				t.Helper()
				assertSudoInstallSources(t, calls[0], "/etc/yum.repos.d/example.repo")
				assertSudoCall(t, calls[1], "dnf", "install", "-y", "example")

				got, err := renderRepoTemplate(exampleBaseURL, info)
				if err != nil {
					t.Fatalf("renderRepoTemplate() error = %v, want nil", err)
				}
				if want := "https://example.com/fedora/42/amd64"; got != want {
					t.Errorf("renderRepoTemplate() = %q, want %q", got, want)
				}
				if _, err := renderRepoTemplate(exampleBaseURL, platform.Info{Arch: platform.ArchX86_64}); err == nil {
					t.Error("renderRepoTemplate() with unknown VERSION_ID error = nil, want releasever error")
				}
			},
		},
		{
			name: "dnf copr enables the project through the dnf5 plugin",
			pkg: manifest.Package{
				Name:    "lazygit",
				Backend: manifest.BackendRepo,
				Names:   map[string]string{"dnf": "lazygit"},
				DNFRepo: &manifest.DNFRepo{COPR: "atim/lazygit"},
			},
			info: platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrDNF, VersionID: "42"},
			stub: func(r *exec.Fake) {
				r.AddCommand("dnf", []string{"--version"}, []byte("dnf5 version 5.2.6.2\n"), nil)
			},
			wantCalls: 4,
			assert: func(t *testing.T, calls []exec.FakeCall, _ platform.Info) {
				t.Helper()
				assertCall(t, calls[0], "dnf", "--version")
				assertSudoCall(t, calls[1], "dnf", "install", "-y", "dnf5-plugins")
				assertSudoCall(t, calls[2], "dnf", "-y", "copr", "enable", "atim/lazygit")
				assertSudoCall(t, calls[3], "dnf", "install", "-y", "lazygit")
			},
		},
		{
			name: "apt keeps the keyring sources update install sequence",
			pkg: manifest.Package{
				Name:    "docker-ce",
				Backend: manifest.BackendRepo,
				Names:   map[string]string{"apt": "docker-ce", "dnf": "docker-ce"},
				AptRepo: &manifest.AptRepo{
					KeyURL:      dockerKeyURL,
					Keyring:     dockerKeyring,
					RepoLine:    dockerRepoLine,
					SourcesPath: dockerSourcesPath,
				},
			},
			info: platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT, Codename: "noble", Arch: platform.ArchX86_64},
			stub: func(r *exec.Fake) {
				r.AddCommand("curl", []string{"-fsSL", dockerKeyURL}, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\n"), nil)
			},
			wantCalls: 6,
			assert: func(t *testing.T, calls []exec.FakeCall, _ platform.Info) {
				t.Helper()
				assertCall(t, calls[0], "curl", "-fsSL", dockerKeyURL)
				assertSudoCall(t, calls[1], "install", "-d", "-m", "0755", "/etc/apt/keyrings")
				assertSudoGpgDearmor(t, calls[2], dockerKeyring)
				assertSudoInstallSources(t, calls[3], dockerSourcesPath)
				assertSudoCall(t, calls[4], "apt-get", "update")
				assertSudoCall(t, calls[5], "apt-get", "install", "-y", "docker-ce")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := exec.NewFake()
			if tt.stub != nil {
				tt.stub(r)
			}

			if err := newRepoBackend(r).Install(tt.pkg, tt.info); err != nil {
				t.Fatalf("Install() error = %v", err)
			}

			calls := r.CallsSnapshot()
			if len(calls) != tt.wantCalls {
				t.Fatalf("CallsSnapshot() len = %d, want %d: %v", len(calls), tt.wantCalls, calls)
			}
			tt.assert(t, calls, tt.info)
		})
	}
}

// TestRepoBackend_Install_NotApplicable verifies the package-aware gate: an
// entry that carries only the other family's sub-table is refused on this host
// rather than half-configuring a repository it has no recipe for.
func TestRepoBackend_Install_NotApplicable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		pkg  manifest.Package
		info platform.Info
	}{
		{
			name: "dnf_repo only on an apt host",
			pkg: manifest.Package{
				Name:    "example",
				Backend: manifest.BackendRepo,
				Names:   map[string]string{"apt": "example", "dnf": "example"},
				DNFRepo: &manifest.DNFRepo{ID: "example", RepoFile: "https://example.com/example.repo"},
			},
			info: platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT},
		},
		{
			name: "apt_repo only on a dnf host",
			pkg: manifest.Package{
				Name:    "example",
				Backend: manifest.BackendRepo,
				Names:   map[string]string{"apt": "example", "dnf": "example"},
				AptRepo: &manifest.AptRepo{
					KeyURL:      "https://example.com/gpg",
					Keyring:     "/etc/apt/keyrings/example.gpg",
					RepoLine:    "deb [signed-by=/etc/apt/keyrings/example.gpg] https://example.com stable main",
					SourcesPath: "/etc/apt/sources.list.d/example.list",
				},
			},
			info: platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrDNF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := newRepoBackend(exec.NewFake()).Install(tt.pkg, tt.info)
			if err == nil {
				t.Fatal("Install() error = nil, want *UnsupportedPlatformError")
			}
			var unsupported *UnsupportedPlatformError
			if !errors.As(err, &unsupported) {
				t.Fatalf("Install() error = %T, want *UnsupportedPlatformError", err)
			}
		})
	}
}

// gpgFake answers `gpg --show-keys` with gpgOut whatever the temp key path is.
type gpgFake struct {
	*exec.Fake
	gpgOut string
}

func (g gpgFake) Output(name string, args ...string) ([]byte, error) {
	if name == "gpg" {
		_, _ = g.Fake.Output("true") // record the probe; result ignored
		return []byte(g.gpgOut), nil
	}
	return g.Fake.Output(name, args...)
}

// TestRepoBackend_Install_KeyFingerprint: a pinned key whose fingerprint
// mismatches aborts before any sudo command; a match proceeds.
func TestRepoBackend_Install_KeyFingerprint(t *testing.T) {
	t.Parallel()
	const fp = "31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE"
	pkg := manifest.Package{
		Name: "claude-desktop", Backend: manifest.BackendRepo,
		Names: map[string]string{"apt": "claude-desktop"},
		AptRepo: &manifest.AptRepo{
			KeyURL: "https://k", Keyring: "/usr/share/keyrings/c.gpg",
			RepoLine: "deb https://r stable main", SourcesPath: "/etc/apt/sources.list.d/c.list",
			KeyFingerprint: fp,
		},
	}
	info := platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT, Arch: platform.ArchX86_64}

	for _, tc := range []struct {
		name, out string
		ok        bool
	}{
		{"match lowercase", "pub:-:4096:1:BAA929FF1A7ECACE:::::\nfpr:::::::::" + strings.ToLower(fp) + ":\n", true},
		{"mismatch", "fpr:::::::::" + strings.Repeat("A", 40) + ":\n", false},
		{"no fpr record", "pub:-:4096\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := exec.NewFake()
			f.AddCommand("curl", []string{"-fsSL", "https://k"}, []byte("key"), nil)
			err := newRepoBackend(gpgFake{f, tc.out}).Install(pkg, info)
			sudo := 0
			for _, c := range f.CallsSnapshot() {
				if c.Name == "sudo" {
					sudo++
				}
			}
			if tc.ok && (err != nil || sudo == 0) {
				t.Fatalf("Install() = %v, sudo calls %d; want success", err, sudo)
			}
			if !tc.ok && (err == nil || sudo != 0) {
				t.Fatalf("Install() = %v, sudo calls %d; want error before any sudo", err, sudo)
			}
		})
	}
}
