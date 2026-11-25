package backend

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

const (
	rollbackKeyURL      = "https://download.docker.com/linux/ubuntu/gpg"
	rollbackKeyring     = "/etc/apt/keyrings/docker.gpg"
	rollbackSourcesPath = "/etc/apt/sources.list.d/docker.list"
)

func rollbackPkg() manifest.Package {
	return manifest.Package{
		Name:    "docker-ce",
		Backend: manifest.BackendRepo,
		Names:   map[string]string{"apt": "docker-ce"},
		AptRepo: &manifest.AptRepo{
			KeyURL:         rollbackKeyURL,
			Keyring:        rollbackKeyring,
			RepoLine:       "deb [arch={{.Arch}} signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/{{.BaseDistro}} {{.BaseCodename}} stable",
			SourcesPath:    rollbackSourcesPath,
			KeyFingerprint: dockerKeyFingerprint,
		},
	}
}

var rollbackInfo = platform.Info{
	OS: platform.OSDebian, PkgMgr: platform.PkgMgrAPT, Arch: platform.ArchX86_64,
	Codename: "trixie", UbuntuCodename: "trixie", BaseDistro: "debian", BaseCodename: "trixie",
}

// TestRepoBackend_Install_AptUpdateFailureRollsBack is the YAN-1411 repro
// (TestZZ_AptUpdateFailureLeavesSources): when apt-get update fails, the
// sources file and keyring installAPT just wrote must not stay behind, or every
// later apt-get update on the host (and every following apt-repo package in
// the same run) fails too. Files dotf created are removed; files it replaced
// get their prior content back.
func TestRepoBackend_Install_AptUpdateFailureRollsBack(t *testing.T) {
	t.Parallel()

	const (
		priorSources = "deb [signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian bookworm stable\n"
		priorKeyring = "\x99prior-keyring"
	)

	tests := []struct {
		name     string
		readFile func(string) ([]byte, error)
		// tail is the expected sudo argv after the failed `apt-get update`;
		// "<tmp>" matches the staged restore temp file.
		tail         [][]string
		wantWritten  map[string]string
		wantErrParts []string
	}{
		{
			name:     "files dotf created are removed",
			readFile: readNotExist,
			tail: [][]string{
				{"rm", "-f", rollbackSourcesPath},
				{"rm", "-f", rollbackKeyring},
			},
			wantErrParts: []string{"apt-get update", "update boom"},
		},
		{
			name: "files dotf replaced get their prior content back",
			readFile: func(p string) ([]byte, error) {
				switch p {
				case rollbackSourcesPath:
					return []byte(priorSources), nil
				case rollbackKeyring:
					return []byte(priorKeyring), nil
				}
				return nil, os.ErrNotExist
			},
			tail: [][]string{
				{"install", "-m", "644", "<tmp>", rollbackSourcesPath},
				{"install", "-m", "0644", "<tmp>", rollbackKeyring},
			},
			wantWritten:  map[string]string{rollbackSourcesPath: priorSources, rollbackKeyring: priorKeyring},
			wantErrParts: []string{"apt-get update", "update boom"},
		},
		{
			name: "pre-existing keyring, new sources file",
			readFile: func(p string) ([]byte, error) {
				if p == rollbackKeyring {
					return []byte(priorKeyring), nil
				}
				return nil, os.ErrNotExist
			},
			tail: [][]string{
				{"rm", "-f", rollbackSourcesPath},
				{"install", "-m", "0644", "<tmp>", rollbackKeyring},
			},
			wantWritten:  map[string]string{rollbackKeyring: priorKeyring},
			wantErrParts: []string{"apt-get update"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := pinnedKeyFake(dockerKeyFingerprint)
			r.AddCommand("curl", []string{"-fsSL", rollbackKeyURL}, []byte("key"), nil)
			r.RunErrors[exec.CommandKey("sudo", "apt-get", "update")] = errors.New("update boom")

			b := newRepoBackend(r)
			b.readFile = tt.readFile
			err := b.Install(rollbackPkg(), rollbackInfo)
			if err == nil {
				t.Fatal("Install() error = nil, want apt-get update failure")
			}
			for _, part := range tt.wantErrParts {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("Install() error = %q, want it to mention %q", err, part)
				}
			}

			calls := r.CallsSnapshot()
			updateAt := -1
			for i, c := range calls {
				if c.Name == "sudo" && strings.Join(c.Args, " ") == "apt-get update" {
					updateAt = i
				}
				if c.Name == "sudo" && len(c.Args) > 0 && c.Args[0] == "apt-get" && c.Args[1] == "install" {
					t.Fatalf("apt-get install ran after a failed update: %v", calls)
				}
			}
			if updateAt < 0 {
				t.Fatalf("no sudo apt-get update call: %v", calls)
			}
			tail := calls[updateAt+1:]
			if len(tail) != len(tt.tail) {
				t.Fatalf("calls after apt-get update = %v, want %v", tail, tt.tail)
			}
			for i, want := range tt.tail {
				got := tail[i]
				if got.Name != "sudo" || len(got.Args) != len(want) {
					t.Fatalf("rollback call %d = %v, want sudo %v", i, got, want)
				}
				for j, w := range want {
					if w == "<tmp>" {
						continue
					}
					if got.Args[j] != w {
						t.Fatalf("rollback call %d = %v, want sudo %v", i, got, want)
					}
				}
			}
			for path, want := range tt.wantWritten {
				if got := r.written[path]; got != want {
					t.Errorf("%s restored content = %q, want %q", path, got, want)
				}
			}
		})
	}
}

// TestRepoBackend_Install_AptUnreadablePriorFileRefused: a keyring or sources
// file that exists but cannot be read could not be restored after a failed
// apt-get update, so installAPT refuses before any sudo step touches the host.
func TestRepoBackend_Install_AptUnreadablePriorFileRefused(t *testing.T) {
	t.Parallel()

	for _, unreadable := range []string{rollbackKeyring, rollbackSourcesPath} {
		t.Run(unreadable, func(t *testing.T) {
			t.Parallel()

			r := pinnedKeyFake(dockerKeyFingerprint)
			r.AddCommand("curl", []string{"-fsSL", rollbackKeyURL}, []byte("key"), nil)

			b := newRepoBackend(r)
			b.readFile = func(p string) ([]byte, error) {
				if p == unreadable {
					return nil, fs.ErrPermission
				}
				return nil, os.ErrNotExist
			}
			err := b.Install(rollbackPkg(), rollbackInfo)
			if err == nil || !strings.Contains(err.Error(), unreadable) {
				t.Fatalf("Install() error = %v, want a refusal naming %s", err, unreadable)
			}
			for _, c := range r.CallsSnapshot() {
				if c.Name == "sudo" {
					t.Fatalf("sudo ran despite an unrestorable prior file: %v", c)
				}
			}
		})
	}
}

// TestRepoBackend_Install_AptInstallFailureKeepsRepo: once apt-get update has
// accepted the new sources entry the repo is valid, so a package install
// failure must not tear it down.
func TestRepoBackend_Install_AptInstallFailureKeepsRepo(t *testing.T) {
	t.Parallel()

	r := pinnedKeyFake(dockerKeyFingerprint)
	r.AddCommand("curl", []string{"-fsSL", rollbackKeyURL}, []byte("key"), nil)
	r.RunErrors[exec.CommandKey("sudo", "apt-get", "install", "-y", "docker-ce")] = errors.New("install boom")

	b := newRepoBackend(r)
	b.readFile = readNotExist
	if err := b.Install(rollbackPkg(), rollbackInfo); err == nil || !strings.Contains(err.Error(), "install boom") {
		t.Fatalf("Install() error = %v, want the install failure", err)
	}
	for _, c := range r.CallsSnapshot() {
		if c.Name == "sudo" && len(c.Args) > 0 && c.Args[0] == "rm" {
			t.Fatalf("repo rolled back after a successful update: %v", c)
		}
	}
}

// TestEmbeddedManifest_AptRepoSuitesPerDistro renders the real docker-ce and
// dropbox repo lines for each supported apt host and pins the URL + suite:
// every one must name a dists/<suite> the vendor publishes (verified live for
// YAN-1411), never an Ubuntu path with a Debian or Mint codename.
func TestEmbeddedManifest_AptRepoSuitesPerDistro(t *testing.T) {
	t.Parallel()

	sub, err := fs.Sub(dotfiles.ManifestFS, "manifest")
	if err != nil {
		t.Fatalf("fs.Sub() error = %v", err)
	}
	m, err := manifest.LoadFS(sub, "packages.toml")
	if err != nil {
		t.Fatalf("LoadFS(packages.toml) error = %v", err)
	}
	lines := map[string]string{}
	for _, p := range m.Packages {
		if (p.Name == "docker-ce" || p.Name == "dropbox") && p.AptRepo != nil {
			lines[p.Name] = p.AptRepo.RepoLine
		}
	}
	if len(lines) != 2 {
		t.Fatalf("embedded manifest apt repo lines = %v, want docker-ce and dropbox", lines)
	}

	// Info values as platform.Detect derives them from each host's os-release
	// (see platform.TestReadOSRelease_AptBase).
	hosts := []struct {
		name         string
		info         platform.Info
		docker, dbox string
	}{
		{
			name:   "ubuntu noble",
			info:   platform.Info{Codename: "noble", UbuntuCodename: "noble", BaseDistro: "ubuntu", BaseCodename: "noble"},
			docker: "https://download.docker.com/linux/ubuntu noble stable",
			dbox:   "https://linux.dropbox.com/ubuntu noble main",
		},
		{
			name:   "debian bookworm",
			info:   platform.Info{Codename: "bookworm", UbuntuCodename: "bookworm", BaseDistro: "debian", BaseCodename: "bookworm"},
			docker: "https://download.docker.com/linux/debian bookworm stable",
			dbox:   "https://linux.dropbox.com/debian bookworm main",
		},
		{
			name:   "debian trixie",
			info:   platform.Info{Codename: "trixie", UbuntuCodename: "trixie", BaseDistro: "debian", BaseCodename: "trixie"},
			docker: "https://download.docker.com/linux/debian trixie stable",
			dbox:   "https://linux.dropbox.com/debian trixie main",
		},
		{
			name:   "mint wilma",
			info:   platform.Info{Codename: "wilma", UbuntuCodename: "noble", BaseDistro: "ubuntu", BaseCodename: "noble"},
			docker: "https://download.docker.com/linux/ubuntu noble stable",
			dbox:   "https://linux.dropbox.com/ubuntu noble main",
		},
		{
			name:   "elementary circe",
			info:   platform.Info{Codename: "circe", UbuntuCodename: "noble", BaseDistro: "ubuntu", BaseCodename: "noble"},
			docker: "https://download.docker.com/linux/ubuntu noble stable",
			dbox:   "https://linux.dropbox.com/ubuntu noble main",
		},
	}
	for _, h := range hosts {
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			h.info.Arch = platform.ArchX86_64
			for pkg, want := range map[string]string{"docker-ce": h.docker, "dropbox": h.dbox} {
				got, err := renderRepoLine(lines[pkg], h.info)
				if err != nil {
					t.Fatalf("renderRepoLine(%s) error = %v", pkg, err)
				}
				if !strings.Contains(got, want) {
					t.Errorf("renderRepoLine(%s) = %q, want it to contain %q", pkg, got, want)
				}
			}
		})
	}
}
