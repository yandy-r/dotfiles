package backend

import (
	"errors"
	"os"
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

			b := newRepoBackend(r)
			b.readFile = readNotExist
			if err := b.Install(tt.pkg, tt.info); err != nil {
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

// TestRepoBackend_UpdateRemove_Argv: once the repo is configured, update/remove
// are plain package-manager ops (no repo setup, no key/curl), gated like
// Install. A dnf update against a kept non-dotf repo file enables its section.
func TestRepoBackend_UpdateRemove_Argv(t *testing.T) {
	t.Parallel()

	pkg := manifest.Package{
		Name: "docker-ce", Backend: manifest.BackendRepo,
		Names:   map[string]string{"apt": "docker-ce-apt", "dnf": "docker-ce-dnf"},
		AptRepo: &manifest.AptRepo{KeyURL: "https://k", Keyring: "/k.gpg", RepoLine: "deb x", SourcesPath: "/s.list"},
		DNFRepo: &manifest.DNFRepo{ID: "docker-ce", RepoFile: "https://r.repo"},
	}
	apt := platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT}
	dnf := platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrDNF}
	update := func(b *repoBackend, i platform.Info) error { return b.Update(pkg, i) }
	remove := func(b *repoBackend, i platform.Info) error { return b.Remove(pkg, i) }
	disabled := func(string) ([]byte, error) { return []byte("[docker-ce]\nenabled=0\n"), nil }
	owned := func(string) ([]byte, error) { return []byte(dnfRepoMarker + "\n[docker-ce]\nenabled=1\n"), nil }

	tests := []struct {
		name     string
		info     platform.Info
		readFile func(string) ([]byte, error)
		op       func(b *repoBackend, info platform.Info) error
		want     []string
	}{
		{"apt update", apt, readNotExist, update,
			[]string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "--only-upgrade", "-y", "docker-ce-apt"}},
		{"apt remove", apt, readNotExist, remove,
			[]string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "remove", "-y", "docker-ce-apt"}},
		{"dnf update", dnf, readNotExist, update, []string{"dnf", "upgrade", "-y", "docker-ce-dnf"}},
		{"dnf update dotf-owned repo file", dnf, owned, update, []string{"dnf", "upgrade", "-y", "docker-ce-dnf"}},
		{"dnf update kept disabled repo section", dnf, disabled, update,
			[]string{"dnf", "upgrade", "--enablerepo=docker-ce", "-y", "docker-ce-dnf"}},
		{"dnf remove", dnf, disabled, remove, []string{"dnf", "remove", "-y", "docker-ce-dnf"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := exec.NewFake()
			b := newRepoBackend(r)
			b.readFile = tt.readFile
			if err := tt.op(b, tt.info); err != nil {
				t.Fatalf("error = %v", err)
			}
			calls := r.CallsSnapshot()
			if len(calls) != 1 {
				t.Fatalf("calls = %v, want exactly 1", calls)
			}
			assertSudoCall(t, calls[0], tt.want[0], tt.want[1:]...)
		})
	}

	// Unsupported manager (and sub-table-less host) -> UnsupportedPlatformError, no commands.
	brew := platform.Info{OS: platform.OSMacOS, PkgMgr: platform.PkgMgrHomebrew}
	r := exec.NewFake()
	b := newRepoBackend(r)
	for name, err := range map[string]error{"update": b.Update(pkg, brew), "remove": b.Remove(pkg, brew)} {
		var u *UnsupportedPlatformError
		if !errors.As(err, &u) {
			t.Errorf("%s on brew: error = %T(%v), want *UnsupportedPlatformError", name, err, err)
		}
	}
	if len(r.CallsSnapshot()) != 0 {
		t.Errorf("unsupported host ran commands: %v", r.CallsSnapshot())
	}
}

// readNotExist is the readFile seam for a host with no existing repo file.
func readNotExist(string) ([]byte, error) { return nil, os.ErrNotExist }

// installCapture records the staged temp file's content when the backend
// runs `sudo install -m 644 <tmp> <dest>`, before the deferred cleanup
// removes it.
type installCapture struct {
	*exec.Fake
	written map[string]string
}

func (c installCapture) Run(name string, args ...string) error {
	if name == "sudo" && len(args) == 5 && args[0] == "install" {
		body, err := os.ReadFile(args[3])
		if err != nil {
			return err
		}
		c.written[args[4]] = string(body)
	}
	return c.Fake.Run(name, args...)
}

// TestRepoBackend_Install_DNFExistingRepoFile pins the ownership rule for
// /etc/yum.repos.d/<id>.repo: dotf writes a missing file and refreshes one it
// owns (marker first line, or the exact legacy body it used to render), but
// leaves any other existing file untouched and still installs the package.
func TestRepoBackend_Install_DNFExistingRepoFile(t *testing.T) {
	t.Parallel()

	const (
		dest       = "/etc/yum.repos.d/vendor.repo"
		repoFile   = "https://vendor.example/vendor.repo"
		baseURL    = "https://vendor.example/rpm"
		gpgKey     = "https://vendor.example/key.asc"
		vendorBody = "[vendor]\nname=Vendor\nbaseurl=https://vendor.example/rpm\ngpgcheck=1\nrepo_gpgcheck=1\n"
		legacyBody = "[vendor]\nname=vendor\nbaseurl=" + baseURL + "\nenabled=1\ngpgcheck=1\ngpgkey=" + gpgKey + "\n"
	)
	existing := func(body string) func(string) ([]byte, error) {
		return func(string) ([]byte, error) { return []byte(body), nil }
	}
	baseRepo := func(repoGPG bool) *manifest.DNFRepo {
		return &manifest.DNFRepo{ID: "vendor", BaseURL: baseURL, GPGKey: gpgKey, RepoGPGCheck: repoGPG}
	}
	renderedBase := func(repoGPG bool) string {
		extra := ""
		if repoGPG {
			extra = "repo_gpgcheck=1\n"
		}
		return dnfRepoMarker + "\n[vendor]\nname=vendor\nbaseurl=" + baseURL +
			"\nenabled=1\ngpgcheck=1\n" + extra + "gpgkey=" + gpgKey + "\n"
	}

	tests := []struct {
		name       string
		repo       *manifest.DNFRepo
		readFile   func(string) ([]byte, error)
		wantErr    bool
		wantCurl   bool
		wantWrite  string // expected dest content; "" = dest must not be written
		wantEnable bool   // dnf install carries --enablerepo=vendor
	}{
		{"baseurl missing file is written", baseRepo(false), readNotExist, false, false, renderedBase(false), false},
		{"baseurl repo_gpgcheck rendered", baseRepo(true), readNotExist, false, false, renderedBase(true), false},
		{"baseurl marker-owned file refreshed", baseRepo(true), existing(dnfRepoMarker + "\n[vendor]\nbaseurl=old\n"), false, false, renderedBase(true), false},
		{"baseurl legacy dotf body migrated when repo_gpgcheck changes it", baseRepo(true), existing(legacyBody), false, false, renderedBase(true), false},
		{"baseurl legacy-equal body without repo_gpgcheck preserved", baseRepo(false), existing(legacyBody), false, false, "", true},
		{"baseurl foreign disabled section preserved and enabled", baseRepo(true), existing("[vendor]\nname=Vendor\nbaseurl=" + baseURL + "\nenabled=0\n"), false, false, "", true},
		{"baseurl read error fails closed", baseRepo(false), func(string) ([]byte, error) { return nil, os.ErrPermission }, true, false, "", false},
		{"repofile missing file written with marker", &manifest.DNFRepo{ID: "vendor", RepoFile: repoFile}, readNotExist, false, true, dnfRepoMarker + "\n" + vendorBody, false},
		{"repofile marker-owned file refreshed", &manifest.DNFRepo{ID: "vendor", RepoFile: repoFile}, existing(dnfRepoMarker + "\nold\n"), false, true, dnfRepoMarker + "\n" + vendorBody, false},
		{"repofile foreign file preserved without fetch", &manifest.DNFRepo{ID: "vendor", RepoFile: repoFile}, existing(vendorBody), false, false, "", true},
		{"repofile foreign file with other section ids not enabled", &manifest.DNFRepo{ID: "vendor", RepoFile: repoFile}, existing("[vendor-stable]\nenabled=1\n [vendor ]\n"), false, false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := exec.NewFake()
			f.AddCommand("curl", []string{"-fsSL", repoFile}, []byte(vendorBody), nil)
			c := installCapture{Fake: f, written: map[string]string{}}
			b := newRepoBackend(c)
			// Reads after a write see the written file, as on a real host.
			b.readFile = func(path string) ([]byte, error) {
				if body, ok := c.written[path]; ok {
					return []byte(body), nil
				}
				return tt.readFile(path)
			}

			pkg := manifest.Package{
				Name: "vendor-app", Backend: manifest.BackendRepo,
				Names: map[string]string{"dnf": "vendor-app"}, DNFRepo: tt.repo,
			}
			err := b.Install(pkg, platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrDNF, Arch: platform.ArchX86_64})
			calls := f.CallsSnapshot()
			if tt.wantErr {
				if err == nil {
					t.Fatal("Install() error = nil, want read error")
				}
				if len(calls) != 0 {
					t.Fatalf("Install() ran commands after a read error: %v", calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Install() error = %v", err)
			}

			got, wrote := c.written[dest]
			switch {
			case tt.wantWrite == "" && wrote:
				t.Errorf("existing %s overwritten with %q, want it preserved", dest, got)
			case tt.wantWrite != "" && got != tt.wantWrite:
				t.Errorf("%s content = %q, want %q", dest, got, tt.wantWrite)
			}
			curled := false
			for _, call := range calls {
				if call.Name == "curl" {
					curled = true
				}
			}
			if curled != tt.wantCurl {
				t.Errorf("curl fetched = %v, want %v: %v", curled, tt.wantCurl, calls)
			}
			want := []string{"install"}
			if tt.wantEnable {
				want = append(want, "--enablerepo=vendor")
			}
			assertSudoCall(t, calls[len(calls)-1], "dnf", append(want, "-y", "vendor-app")...)
		})
	}
}
