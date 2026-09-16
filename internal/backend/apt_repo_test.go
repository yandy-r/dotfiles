package backend

import (
	"errors"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestAptRepoBackend_Install_KeyringSourcesUpdateInstallSequence(t *testing.T) {
	t.Parallel()

	const (
		keyURL      = "https://download.docker.com/linux/ubuntu/gpg"
		keyring     = "/etc/apt/keyrings/docker.gpg"
		sourcesPath = "/etc/apt/sources.list.d/docker.list"
		repoLine    = "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu stable"
		aptName     = "docker-ce"
	)

	r := exec.NewFake()
	r.AddCommand("curl", []string{"-fsSL", keyURL}, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\n"), nil)

	b := newAptRepoBackend(r)
	pkg := manifest.Package{
		Name:    "docker-ce",
		Backend: manifest.BackendAPTRepo,
		Names:   map[string]string{"apt": aptName},
		AptRepo: &manifest.AptRepo{
			KeyURL:      keyURL,
			Keyring:     keyring,
			RepoLine:    repoLine,
			SourcesPath: sourcesPath,
		},
	}
	info := platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	calls := r.CallsSnapshot()
	if len(calls) < 6 {
		t.Fatalf("CallsSnapshot() len = %d, want at least 6", len(calls))
	}

	assertCall(t, calls[0], "curl", "-fsSL", keyURL)
	assertSudoCall(t, calls[1], "install", "-d", "-m", "0755", "/etc/apt/keyrings")
	assertSudoGpgDearmor(t, calls[2], keyring)
	assertSudoInstallSources(t, calls[3], sourcesPath)
	assertSudoCall(t, calls[4], "apt-get", "update")
	assertSudoCall(t, calls[5], "apt-get", "install", "-y", aptName)
}

func TestAptRepoBackend_Install_RerunConverges(t *testing.T) {
	t.Parallel()

	const (
		keyURL      = "https://download.docker.com/linux/ubuntu/gpg"
		keyring     = "/etc/apt/keyrings/docker.gpg"
		sourcesPath = "/etc/apt/sources.list.d/docker.list"
		repoLine    = "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu stable"
		aptName     = "docker-ce"
	)

	r := exec.NewFake()
	r.AddCommand("curl", []string{"-fsSL", keyURL}, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\n"), nil)

	b := newAptRepoBackend(r)
	pkg := manifest.Package{
		Name:    "docker-ce",
		Backend: manifest.BackendAPTRepo,
		Names:   map[string]string{"apt": aptName},
		AptRepo: &manifest.AptRepo{
			KeyURL:      keyURL,
			Keyring:     keyring,
			RepoLine:    repoLine,
			SourcesPath: sourcesPath,
		},
	}
	info := platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("second Install() error = %v", err)
	}

	calls := r.CallsSnapshot()
	if len(calls) != 12 {
		t.Fatalf("CallsSnapshot() len = %d, want 12 (two identical 6-step runs)", len(calls))
	}

	for run := 0; run < 2; run++ {
		base := run * 6
		assertCall(t, calls[base], "curl", "-fsSL", keyURL)
		assertSudoCall(t, calls[base+1], "install", "-d", "-m", "0755", "/etc/apt/keyrings")
		assertSudoGpgDearmor(t, calls[base+2], keyring)
		assertSudoInstallSources(t, calls[base+3], sourcesPath)
		assertSudoCall(t, calls[base+4], "apt-get", "update")
		assertSudoCall(t, calls[base+5], "apt-get", "install", "-y", aptName)
	}
}

func TestAptRepoBackend_Install_UnsupportedPlatform(t *testing.T) {
	t.Parallel()

	b := newAptRepoBackend(exec.NewFake())
	pkg := manifest.Package{
		Name:    "docker-ce",
		Backend: manifest.BackendAPTRepo,
		Names:   map[string]string{"apt": "docker-ce"},
		AptRepo: &manifest.AptRepo{
			KeyURL:      "https://example.com/gpg",
			Keyring:     "/etc/apt/keyrings/example.gpg",
			RepoLine:    "deb [signed-by=/etc/apt/keyrings/example.gpg] https://example.com stable main",
			SourcesPath: "/etc/apt/sources.list.d/example.list",
		},
	}
	info := platform.Info{OS: platform.OSArch, PkgMgr: platform.PkgMgrPacman}

	err := b.Install(pkg, info)
	if err == nil {
		t.Fatal("Install() error = nil, want *UnsupportedPlatformError")
	}

	var unsupported *UnsupportedPlatformError
	if !errors.As(err, &unsupported) {
		t.Fatalf("Install() error = %T, want *UnsupportedPlatformError", err)
	}
	if unsupported.Backend != manifest.BackendAPTRepo {
		t.Errorf("UnsupportedPlatformError.Backend = %q, want %q", unsupported.Backend, manifest.BackendAPTRepo)
	}
}

func TestAptRepoBackend_Installed_DpkgQuery(t *testing.T) {
	t.Parallel()

	const aptName = "docker-ce"

	r := exec.NewFake()
	r.AddCommand("dpkg-query", []string{"-W", "-f=${db:Status-Status}", aptName}, []byte("installed\n"), nil)

	b := newAptRepoBackend(r)
	pkg := manifest.Package{
		Name:  "docker-ce",
		Names: map[string]string{"apt": aptName},
	}

	installed, err := b.Installed(pkg, platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT})
	if err != nil {
		t.Fatalf("Installed() error = %v", err)
	}
	if !installed {
		t.Fatal("Installed() = false, want true")
	}

	calls := r.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("CallsSnapshot() len = %d, want 1", len(calls))
	}
	assertCall(t, calls[0], "dpkg-query", "-W", "-f=${db:Status-Status}", aptName)
}

func TestAptRepoBackend_Installed_ConfigFiles(t *testing.T) {
	t.Parallel()

	const aptName = "docker-ce"

	r := exec.NewFake()
	// Removed-but-not-purged: dpkg exits 0 but the state is "config-files".
	r.AddCommand("dpkg-query", []string{"-W", "-f=${db:Status-Status}", aptName}, []byte("config-files\n"), nil)

	b := newAptRepoBackend(r)
	pkg := manifest.Package{
		Name:  "docker-ce",
		Names: map[string]string{"apt": aptName},
	}

	installed, err := b.Installed(pkg, platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT})
	if err != nil {
		t.Fatalf("Installed() error = %v", err)
	}
	if installed {
		t.Fatal("Installed() = true, want false for config-files state")
	}
}

func TestAptRepoBackend_Installed_NotInstalled(t *testing.T) {
	t.Parallel()

	const aptName = "docker-ce"

	r := exec.NewFake()
	r.AddCommand("dpkg-query", []string{"-W", "-f=${db:Status-Status}", aptName}, nil, errors.New("dpkg-query: no packages found matching docker-ce"))

	b := newAptRepoBackend(r)
	pkg := manifest.Package{
		Name:  "docker-ce",
		Names: map[string]string{"apt": aptName},
	}

	installed, err := b.Installed(pkg, platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT})
	if err != nil {
		t.Fatalf("Installed() error = %v", err)
	}
	if installed {
		t.Fatal("Installed() = true, want false")
	}
}

func TestAptRepoBackend_Installed_UnsupportedPlatform(t *testing.T) {
	t.Parallel()

	b := newAptRepoBackend(exec.NewFake())
	pkg := manifest.Package{
		Name:  "docker-ce",
		Names: map[string]string{"apt": "docker-ce"},
	}
	info := platform.Info{OS: platform.OSMacOS, PkgMgr: platform.PkgMgrHomebrew}

	installed, err := b.Installed(pkg, info)
	if err == nil {
		t.Fatal("Installed() error = nil, want *UnsupportedPlatformError")
	}
	if installed {
		t.Error("Installed() = true, want false on unsupported platform")
	}

	var unsupported *UnsupportedPlatformError
	if !errors.As(err, &unsupported) {
		t.Fatalf("Installed() error = %T, want *UnsupportedPlatformError", err)
	}
}

func TestRenderRepoLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		line    string
		info    platform.Info
		want    string
		wantErr bool
	}{
		{
			name: "templated codename and arch",
			line: "deb [arch={{.Arch}}] https://download.docker.com/linux/ubuntu {{.Codename}} stable",
			info: platform.Info{Codename: "jammy", Arch: platform.ArchX86_64},
			want: "deb [arch=amd64] https://download.docker.com/linux/ubuntu jammy stable",
		},
		{
			name:    "codename placeholder with unknown host codename errors",
			line:    "deb https://download.docker.com/linux/ubuntu {{.Codename}} stable",
			info:    platform.Info{Codename: "", Arch: platform.ArchX86_64},
			wantErr: true,
		},
		{
			name: "no placeholders returned verbatim",
			line: "deb [arch=amd64] https://download.docker.com/linux/ubuntu noble stable",
			info: platform.Info{},
			want: "deb [arch=amd64] https://download.docker.com/linux/ubuntu noble stable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := renderRepoLine(tt.line, tt.info)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("renderRepoLine() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("renderRepoLine() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Errorf("renderRepoLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAptRepoBackend_Install_TemplatedCodename(t *testing.T) {
	t.Parallel()

	const (
		keyURL      = "https://download.docker.com/linux/ubuntu/gpg"
		keyring     = "/etc/apt/keyrings/docker.gpg"
		sourcesPath = "/etc/apt/sources.list.d/docker.list"
		repoLine    = "deb [arch={{.Arch}} signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu {{.Codename}} stable"
		aptName     = "docker-ce"
	)

	r := exec.NewFake()
	r.AddCommand("curl", []string{"-fsSL", keyURL}, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\n"), nil)

	b := newAptRepoBackend(r)
	pkg := manifest.Package{
		Name:    "docker-ce",
		Backend: manifest.BackendAPTRepo,
		Names:   map[string]string{"apt": aptName},
		AptRepo: &manifest.AptRepo{
			KeyURL:      keyURL,
			Keyring:     keyring,
			RepoLine:    repoLine,
			SourcesPath: sourcesPath,
		},
	}
	info := platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT, Codename: "noble", Arch: platform.ArchX86_64}

	if err := b.Install(pkg, info); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
}

func TestAptRepoBackend_Install_MissingCodename_Errors(t *testing.T) {
	t.Parallel()

	const (
		keyURL      = "https://download.docker.com/linux/ubuntu/gpg"
		keyring     = "/etc/apt/keyrings/docker.gpg"
		sourcesPath = "/etc/apt/sources.list.d/docker.list"
		repoLine    = "deb [signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu {{.Codename}} stable"
		aptName     = "docker-ce"
	)

	r := exec.NewFake()
	r.AddCommand("curl", []string{"-fsSL", keyURL}, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\n"), nil)

	b := newAptRepoBackend(r)
	pkg := manifest.Package{
		Name:    "docker-ce",
		Backend: manifest.BackendAPTRepo,
		Names:   map[string]string{"apt": aptName},
		AptRepo: &manifest.AptRepo{
			KeyURL:      keyURL,
			Keyring:     keyring,
			RepoLine:    repoLine,
			SourcesPath: sourcesPath,
		},
	}
	// Codename empty: the {{.Codename}} placeholder cannot be rendered.
	info := platform.Info{OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT, Codename: ""}

	err := b.Install(pkg, info)
	if err == nil {
		t.Fatal("Install() error = nil, want codename error")
	}
	if !strings.Contains(err.Error(), "codename") {
		t.Fatalf("Install() error = %v, want message mentioning codename", err)
	}
}

func assertCall(t *testing.T, call exec.FakeCall, name string, args ...string) {
	t.Helper()
	if call.Name != name {
		t.Fatalf("call name = %q, want %q", call.Name, name)
	}
	if len(call.Args) != len(args) {
		t.Fatalf("call %q args = %v, want %v", name, call.Args, args)
	}
	for i, want := range args {
		if call.Args[i] != want {
			t.Fatalf("call %q arg[%d] = %q, want %q", name, i, call.Args[i], want)
		}
	}
}

func assertSudoCall(t *testing.T, call exec.FakeCall, cmd string, args ...string) {
	t.Helper()
	wantArgs := append([]string{cmd}, args...)
	assertCall(t, call, "sudo", wantArgs...)
}

func assertSudoGpgDearmor(t *testing.T, call exec.FakeCall, keyring string) {
	t.Helper()
	if call.Name != "sudo" {
		t.Fatalf("call name = %q, want sudo", call.Name)
	}
	if len(call.Args) != 7 {
		t.Fatalf("sudo gpg args = %v, want 7 elements", call.Args)
	}
	if call.Args[0] != "gpg" || call.Args[1] != "--batch" || call.Args[2] != "--yes" ||
		call.Args[3] != "--dearmor" || call.Args[4] != "-o" || call.Args[5] != keyring {
		t.Fatalf("sudo gpg args = %v, want gpg --batch --yes --dearmor -o %q <tempfile>", call.Args, keyring)
	}
	if call.Args[6] == "" {
		t.Fatal("sudo gpg temp key path is empty")
	}
}

func assertSudoInstallSources(t *testing.T, call exec.FakeCall, sourcesPath string) {
	t.Helper()
	if call.Name != "sudo" {
		t.Fatalf("call name = %q, want sudo", call.Name)
	}
	if len(call.Args) != 5 {
		t.Fatalf("sudo install args = %v, want 5 elements", call.Args)
	}
	if call.Args[0] != "install" || call.Args[1] != "-m" || call.Args[2] != "644" || call.Args[4] != sourcesPath {
		t.Fatalf("sudo install args = %v, want install -m 644 <tempfile> %q", call.Args, sourcesPath)
	}
	if call.Args[3] == "" {
		t.Fatal("sudo install temp sources path is empty")
	}
}
