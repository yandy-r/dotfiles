package main

import (
	"bytes"
	"encoding/json"
	"github.com/yandy-r/dotfiles/internal/privilege/privilegetest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// findReport returns the packageReport for name in a decoded commandReport, or
// fails the test when it is absent.
func findReport(t *testing.T, rep commandReport, name string) packageReport {
	t.Helper()
	for _, p := range rep.Packages {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("commandReport has no package %q; got %+v", name, rep.Packages)
	return packageReport{}
}

const statusTestManifest = `# drift report test fixture

[[package]]
name = "git"
backend = "pkg"
names = { pacman = "git", apt = "git", dnf = "git", zypper = "git", homebrew = "git" }

[[package]]
name = "eza"
backend = "github-release"
repo = "eza-community/eza"
tag = "v0.21.0"
asset = "eza.tar.gz"
sha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[[package]]
name = "starship"
backend = "script-url"
url = "https://starship.rs/install.sh"
sha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
`

func writeStatusTestManifest(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "packages.toml")
	if err := os.WriteFile(path, []byte(statusTestManifest), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func archPacmanFake(t *testing.T) *exec.Fake {
	t.Helper()

	fake := exec.NewFake()
	fake.AddCommand("uname", []string{"-s"}, []byte("Linux\n"), nil)
	fake.AddCommand("uname", []string{"-m"}, []byte("x86_64\n"), nil)
	fake.AddPath("pacman", "/usr/bin/pacman")
	fake.AddCommand("pacman", []string{"-Q", "git"}, []byte("git 2.43.0-1\n"), nil)
	return fake
}

func testDriftEngine(t *testing.T, fake *exec.Fake) *backend.Engine {
	t.Helper()

	return backend.Default(fake, privilegetest.NewFakeDropper(), t.TempDir(), nil)
}

func parseManifest(t *testing.T) *manifest.Manifest {
	t.Helper()

	m, err := manifest.Parse(strings.NewReader(statusTestManifest))
	if err != nil {
		t.Fatalf("manifest.Parse() error = %v", err)
	}
	return m
}

func archInfo() platform.Info {
	return platform.Info{
		OS:        platform.OSArch,
		Arch:      platform.ArchX86_64,
		PkgMgr:    platform.PkgMgrPacman,
		Container: platform.ContainerNone,
		Distro:    "arch",
	}
}

func assertReadOnlyCalls(t *testing.T, calls []exec.FakeCall) {
	t.Helper()

	for _, call := range calls {
		if call.Name == "sudo" {
			t.Errorf("mutating command recorded: sudo %v", call.Args)
			continue
		}

		switch call.Name {
		case "uname":
			if len(call.Args) != 1 || (call.Args[0] != "-s" && call.Args[0] != "-m") {
				t.Errorf("unexpected uname args: %v", call.Args)
			}
		case "pacman":
			if len(call.Args) < 2 || call.Args[0] != "-Q" {
				t.Errorf("mutating or unexpected pacman args: %v", call.Args)
			}
		case "dpkg", "rpm", "zypper", "brew", "apt-get", "dnf", "cargo", "npm", "git", "gpg", "curl", "wget":
			t.Errorf("mutating or unexpected command recorded: %s %v", call.Name, call.Args)
		default:
			t.Errorf("unexpected command recorded: %s %v", call.Name, call.Args)
		}
	}
}

func TestDriftReport_StatusOutput(t *testing.T) {
	fake := archPacmanFake(t)
	fake.AddPath("starship", "/usr/bin/starship")

	m := parseManifest(t)
	engine := testDriftEngine(t, fake)

	var buf bytes.Buffer
	driftCount, err := driftReport(&buf, m, engine, archInfo(), false, false, false)
	if err != nil {
		t.Fatalf("driftReport() error = %v", err)
	}
	if driftCount != 0 {
		t.Fatalf("driftReport() driftCount = %d, want 0 for status mode", driftCount)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	want := []string{
		"package=eza backend=github-release installed=false",
		"package=git backend=pkg installed=true",
		"package=starship backend=script-url installed=true",
	}
	if len(lines) != len(want) {
		t.Fatalf("status lines = %d, want %d:\n%v", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestDriftReport_DiffOutput(t *testing.T) {
	fake := archPacmanFake(t)
	fake.AddPath("starship", "/usr/bin/starship")

	m := parseManifest(t)
	engine := testDriftEngine(t, fake)

	var buf bytes.Buffer
	driftCount, err := driftReport(&buf, m, engine, archInfo(), true, false, false)
	if err != nil {
		t.Fatalf("driftReport() error = %v", err)
	}
	if driftCount != 1 {
		t.Fatalf("driftReport() driftCount = %d, want 1", driftCount)
	}

	got := strings.TrimSpace(buf.String())
	want := "drift=declared-not-installed package=eza backend=github-release"
	if got != want {
		t.Errorf("diff output = %q, want %q", got, want)
	}
}

func TestDriftReport_DiffEmptyWhenAligned(t *testing.T) {
	fake := archPacmanFake(t)
	fake.AddPath("eza", "/home/user/.local/bin/eza")
	fake.AddPath("starship", "/usr/bin/starship")

	m := parseManifest(t)
	engine := testDriftEngine(t, fake)

	var buf bytes.Buffer
	driftCount, err := driftReport(&buf, m, engine, archInfo(), true, false, false)
	if err != nil {
		t.Fatalf("driftReport() error = %v", err)
	}
	if driftCount != 0 {
		t.Fatalf("driftReport() driftCount = %d, want 0", driftCount)
	}
	if buf.Len() != 0 {
		t.Errorf("diff output = %q, want empty", buf.String())
	}
}

func TestRunStatus_ReadOnlyAndSorted(t *testing.T) {
	manifestPath := writeStatusTestManifest(t)
	fake := archPacmanFake(t)

	var stdout, stderr bytes.Buffer
	code := runStatus([]string{"--manifest", manifestPath}, &stdout, &stderr, fake)
	if code != 0 {
		t.Fatalf("runStatus() code = %d, want 0; stderr = %q", code, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("stdout lines = %d, want 3:\n%s", len(lines), stdout.String())
	}
	if !strings.HasPrefix(lines[0], "package=eza ") {
		t.Errorf("first line = %q, want package=eza sorted first", lines[0])
	}
	if lines[1] != "package=git backend=pkg installed=true" {
		t.Errorf("second line = %q, want git installed=true", lines[1])
	}

	assertReadOnlyCalls(t, fake.CallsSnapshot())
}

// overlayBaseManifest is a minimal --manifest base for the XDG overlay
// integration test: one github-release package whose installed-state probe is a
// LookPath (so no exec stub or network is needed).
const overlayBaseManifest = `[[package]]
name = "base-tool"
backend = "github-release"
repo = "owner/base-tool"
tag = "v1.0.0"
sha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
`

// overlayFragment adds a new package AND overrides the base package, so the
// merge exercises both an append and a logged package collision.
const overlayFragment = `[[package]]
name = "overlay-tool"
backend = "github-release"
repo = "owner/overlay-tool"
tag = "v2.0.0"
sha256 = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[[package]]
name = "base-tool"
backend = "github-release"
repo = "owner/base-tool-fork"
tag = "v9.9.9"
sha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
`

// writeOverlayEnv points HOME + XDG_CONFIG_HOME at temp dirs, writes the base
// manifest and the overlay fragment, and returns the base manifest path. The
// overlay lands at $XDG_CONFIG_HOME/dotf/packages.d/extra.toml — exactly where
// manifestOverlayOptions computes ConfigRoot.
func writeOverlayEnv(t *testing.T) (manifestPath string) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	cfg := filepath.Join(t.TempDir(), "cfg")
	t.Setenv("XDG_CONFIG_HOME", cfg)

	overlayDir := filepath.Join(cfg, "dotf", "packages.d")
	if err := os.MkdirAll(overlayDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(overlayDir, "extra.toml"), []byte(overlayFragment), 0o644); err != nil {
		t.Fatalf("WriteFile(overlay) error = %v", err)
	}

	base := filepath.Join(t.TempDir(), "packages.toml")
	if err := os.WriteFile(base, []byte(overlayBaseManifest), 0o644); err != nil {
		t.Fatalf("WriteFile(base) error = %v", err)
	}
	return base
}

// TestRunStatus_UserConfigOverlay proves the XDG overlay threads through the
// status command: with the overlay active the extra package surfaces and the
// override is logged, while --no-user-config and DOTF_NO_USER_CONFIG=true both
// revert to the base manifest.
func TestRunStatus_UserConfigOverlay(t *testing.T) {
	t.Run("overlay active surfaces extra package and logs collision", func(t *testing.T) {
		base := writeOverlayEnv(t)
		var stdout, stderr bytes.Buffer
		if code := runStatus([]string{"--manifest", base}, &stdout, &stderr, archPacmanFake(t)); code != 0 {
			t.Fatalf("runStatus() code = %d, want 0; stderr = %q", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "package=overlay-tool") {
			t.Errorf("stdout = %q, want it to include the overlay package", stdout.String())
		}
		if !strings.Contains(stdout.String(), "package=base-tool") {
			t.Errorf("stdout = %q, want it to include the base package", stdout.String())
		}
		if !strings.Contains(stderr.String(), "overrides") {
			t.Errorf("stderr = %q, want the base-tool override announced", stderr.String())
		}
	})

	t.Run("--no-user-config reverts to base", func(t *testing.T) {
		base := writeOverlayEnv(t)
		var stdout, stderr bytes.Buffer
		if code := runStatus([]string{"--manifest", base, "--no-user-config"}, &stdout, &stderr, archPacmanFake(t)); code != 0 {
			t.Fatalf("runStatus() code = %d, want 0; stderr = %q", code, stderr.String())
		}
		if strings.Contains(stdout.String(), "overlay-tool") {
			t.Errorf("stdout = %q, want base-only (--no-user-config)", stdout.String())
		}
		if strings.Contains(stderr.String(), "overrides") {
			t.Errorf("stderr = %q, want no collision log with the overlay disabled", stderr.String())
		}
	})

	t.Run("DOTF_NO_USER_CONFIG=true reverts to base", func(t *testing.T) {
		base := writeOverlayEnv(t)
		t.Setenv("DOTF_NO_USER_CONFIG", "true")
		var stdout, stderr bytes.Buffer
		if code := runStatus([]string{"--manifest", base}, &stdout, &stderr, archPacmanFake(t)); code != 0 {
			t.Fatalf("runStatus() code = %d, want 0; stderr = %q", code, stderr.String())
		}
		if strings.Contains(stdout.String(), "overlay-tool") {
			t.Errorf("stdout = %q, want base-only (DOTF_NO_USER_CONFIG env twin)", stdout.String())
		}
	})
}

func TestRunDiff_ReadOnlyAndDriftExit(t *testing.T) {
	manifestPath := writeStatusTestManifest(t)
	fake := archPacmanFake(t)
	fake.AddPath("starship", "/usr/bin/starship")

	var stdout, stderr bytes.Buffer
	code := runDiff([]string{"--manifest", manifestPath}, &stdout, &stderr, fake)
	if code != 1 {
		t.Fatalf("runDiff() code = %d, want 1 when drift present; stderr = %q", code, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	want := "drift=declared-not-installed package=eza backend=github-release"
	if got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}

	assertReadOnlyCalls(t, fake.CallsSnapshot())
}

func TestRunDiff_NoDriftExitZero(t *testing.T) {
	manifestPath := writeStatusTestManifest(t)
	fake := archPacmanFake(t)
	fake.AddPath("eza", "/home/user/.local/bin/eza")
	fake.AddPath("starship", "/usr/bin/starship")

	var stdout, stderr bytes.Buffer
	code := runDiff([]string{"--manifest", manifestPath}, &stdout, &stderr, fake)
	if code != 0 {
		t.Fatalf("runDiff() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}

	assertReadOnlyCalls(t, fake.CallsSnapshot())
}

func TestRunStatus_BadFlag(t *testing.T) {
	fake := exec.NewFake()
	var stdout, stderr bytes.Buffer

	code := runStatus([]string{"--bogus"}, &stdout, &stderr, fake)
	if code != 2 {
		t.Errorf("runStatus([--bogus]) code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunStatus_ManifestResolutionError(t *testing.T) {
	fake := exec.NewFake()
	var stdout, stderr bytes.Buffer

	code := runStatus([]string{"--manifest", "/nonexistent/does-not-exist.toml"}, &stdout, &stderr, fake)
	if code != 1 {
		t.Errorf("runStatus() code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "dotf:") {
		t.Errorf("stderr = %q, want dotf: prefix", stderr.String())
	}
}

// applicabilityTestManifest exercises platform applicability (issue #9): an
// apt-repo package (apt hosts only) and an aur package (Arch hosts only)
// alongside an everywhere-applicable pkg entry.
const applicabilityTestManifest = `# platform-applicability test fixture

[[package]]
name = "git"
backend = "pkg"
names = { pacman = "git", apt = "git", dnf = "git", zypper = "git", homebrew = "git" }

[[package]]
name = "docker-ce"
backend = "apt-repo"
names = { apt = "docker-ce" }
apt_repo = { key_url = "https://example.com/gpg", key_fingerprint = "9DC858229FC7DD38854AE2D88D81803C0EBFCD88", keyring = "/etc/apt/keyrings/docker.gpg", repo_line = "deb https://example.com stable", sources_path = "/etc/apt/sources.list.d/docker.list" }

[[package]]
name = "lutris"
backend = "aur"
`

// cmdApplicabilityManifest is the command-level variant: pkg + apt-repo only,
// so runStatus/runDiff assertions stay deterministic on any development or CI
// host. The aur backend gates on the detected OS, which platform.Detect reads
// from the real /etc/os-release, so aur coverage lives in the driftReport
// tests where platform.Info is injected.
const cmdApplicabilityManifest = `# platform-applicability command test fixture

[[package]]
name = "git"
backend = "pkg"
names = { pacman = "git", apt = "git", dnf = "git", zypper = "git", homebrew = "git" }

[[package]]
name = "docker-ce"
backend = "apt-repo"
names = { apt = "docker-ce" }
apt_repo = { key_url = "https://example.com/gpg", key_fingerprint = "9DC858229FC7DD38854AE2D88D81803C0EBFCD88", keyring = "/etc/apt/keyrings/docker.gpg", repo_line = "deb https://example.com stable", sources_path = "/etc/apt/sources.list.d/docker.list" }
`

func writeManifestFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "packages.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func parseManifestString(t *testing.T, content string) *manifest.Manifest {
	t.Helper()

	m, err := manifest.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("manifest.Parse() error = %v", err)
	}
	return m
}

func debianInfo() platform.Info {
	return platform.Info{
		OS:        platform.OSDebian,
		Arch:      platform.ArchX86_64,
		PkgMgr:    platform.PkgMgrAPT,
		Container: platform.ContainerNone,
		Distro:    "ubuntu",
	}
}

func TestDriftReport_StatusEmitsNotApplicable_ArchHost(t *testing.T) {
	fake := archPacmanFake(t)
	m := parseManifestString(t, applicabilityTestManifest)
	engine := testDriftEngine(t, fake)

	var buf bytes.Buffer
	driftCount, err := driftReport(&buf, m, engine, archInfo(), false, false, false)
	if err != nil {
		t.Fatalf("driftReport() error = %v", err)
	}
	if driftCount != 0 {
		t.Fatalf("driftReport() driftCount = %d, want 0 for status mode", driftCount)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	want := []string{
		"package=docker-ce backend=apt-repo applicable=false",
		"package=git backend=pkg installed=true",
		"package=lutris backend=aur installed=false",
	}
	if len(lines) != len(want) {
		t.Fatalf("status lines = %d, want %d:\n%v", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestDriftReport_DiffExcludesNotApplicable_ArchHost(t *testing.T) {
	fake := archPacmanFake(t)
	m := parseManifestString(t, applicabilityTestManifest)
	engine := testDriftEngine(t, fake)

	var buf bytes.Buffer
	driftCount, err := driftReport(&buf, m, engine, archInfo(), true, false, false)
	if err != nil {
		t.Fatalf("driftReport() error = %v", err)
	}
	if driftCount != 1 {
		t.Fatalf("driftReport() driftCount = %d, want 1 (docker-ce must not count as drift)", driftCount)
	}

	got := strings.TrimSpace(buf.String())
	want := "drift=declared-not-installed package=lutris backend=aur"
	if got != want {
		t.Errorf("diff output = %q, want %q", got, want)
	}
}

func TestDriftReport_StatusEmitsNotApplicable_DebianHost(t *testing.T) {
	fake := exec.NewFake()
	fake.AddCommand("dpkg-query", []string{"-W", "-f=${db:Status-Status}", "git"}, []byte("installed\n"), nil)

	m := parseManifestString(t, applicabilityTestManifest)
	engine := testDriftEngine(t, fake)

	var buf bytes.Buffer
	driftCount, err := driftReport(&buf, m, engine, debianInfo(), false, false, false)
	if err != nil {
		t.Fatalf("driftReport() error = %v", err)
	}
	if driftCount != 0 {
		t.Fatalf("driftReport() driftCount = %d, want 0 for status mode", driftCount)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	want := []string{
		"package=docker-ce backend=apt-repo installed=false",
		"package=git backend=pkg installed=true",
		"package=lutris backend=aur applicable=false",
	}
	if len(lines) != len(want) {
		t.Fatalf("status lines = %d, want %d:\n%v", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestDriftReport_DiffExcludesNotApplicable_DebianHost(t *testing.T) {
	fake := exec.NewFake()
	fake.AddCommand("dpkg-query", []string{"-W", "-f=${db:Status-Status}", "git"}, []byte("installed\n"), nil)

	m := parseManifestString(t, applicabilityTestManifest)
	engine := testDriftEngine(t, fake)

	var buf bytes.Buffer
	driftCount, err := driftReport(&buf, m, engine, debianInfo(), true, false, false)
	if err != nil {
		t.Fatalf("driftReport() error = %v", err)
	}
	if driftCount != 1 {
		t.Fatalf("driftReport() driftCount = %d, want 1 (lutris must not count as drift)", driftCount)
	}

	got := strings.TrimSpace(buf.String())
	want := "drift=declared-not-installed package=docker-ce backend=apt-repo"
	if got != want {
		t.Errorf("diff output = %q, want %q", got, want)
	}
}

func TestRunStatus_NotApplicableBackendExitsZero(t *testing.T) {
	manifestPath := writeManifestFile(t, cmdApplicabilityManifest)
	fake := archPacmanFake(t)

	var stdout, stderr bytes.Buffer
	code := runStatus([]string{"--manifest", manifestPath}, &stdout, &stderr, fake)
	if code != 0 {
		t.Fatalf("runStatus() code = %d, want 0; stderr = %q", code, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	want := []string{
		"package=docker-ce backend=apt-repo applicable=false",
		"package=git backend=pkg installed=true",
	}
	if len(lines) != len(want) {
		t.Fatalf("stdout lines = %d, want %d:\n%s", len(lines), len(want), stdout.String())
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line[%d] = %q, want %q", i, lines[i], want[i])
		}
	}

	assertReadOnlyCalls(t, fake.CallsSnapshot())
}

func TestRunDiff_NotApplicableBackendNoDrift(t *testing.T) {
	manifestPath := writeManifestFile(t, cmdApplicabilityManifest)
	fake := archPacmanFake(t)

	var stdout, stderr bytes.Buffer
	code := runDiff([]string{"--manifest", manifestPath}, &stdout, &stderr, fake)
	if code != 0 {
		t.Fatalf("runDiff() code = %d, want 0 when only non-applicable packages are missing; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}

	assertReadOnlyCalls(t, fake.CallsSnapshot())
}

// TestRunStatus_JSONShape verifies the --json output for status: every declared
// package is reported with name/backend/applicable/installed, an applicable
// package carries a non-nil installed state, and a non-applicable package
// carries applicable=false with installed omitted (nil).
func TestRunStatus_JSONShape(t *testing.T) {
	manifestPath := writeManifestFile(t, cmdApplicabilityManifest)
	fake := archPacmanFake(t)

	var stdout, stderr bytes.Buffer
	code := runStatus([]string{"--json", "--manifest", manifestPath}, &stdout, &stderr, fake)
	if code != 0 {
		t.Fatalf("runStatus(--json) code = %d, want 0; stderr = %q", code, stderr.String())
	}

	var rep commandReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal status JSON: %v\nstdout = %q", err, stdout.String())
	}
	if rep.Command != "status" {
		t.Errorf("command = %q, want status", rep.Command)
	}
	if len(rep.Packages) != 2 {
		t.Fatalf("packages = %d, want 2:\n%+v", len(rep.Packages), rep.Packages)
	}

	git := findReport(t, rep, "git")
	if git.Backend != "pkg" || !git.Applicable {
		t.Errorf("git report = %+v, want backend=pkg applicable=true", git)
	}
	if git.Installed == nil || !*git.Installed {
		t.Errorf("git installed = %v, want non-nil true", git.Installed)
	}
	if git.Action != "" {
		t.Errorf("git action = %q, want empty (status reports no action)", git.Action)
	}

	docker := findReport(t, rep, "docker-ce")
	if docker.Backend != "apt-repo" || docker.Applicable {
		t.Errorf("docker-ce report = %+v, want backend=apt-repo applicable=false", docker)
	}
	if docker.Installed != nil {
		t.Errorf("docker-ce installed = %v, want nil (not probed)", docker.Installed)
	}

	assertReadOnlyCalls(t, fake.CallsSnapshot())
}

// TestRunStatus_JSONDoesNotAlterHumanOutput proves --json is strictly additive:
// without the flag the greppable key=value lines are unchanged.
func TestRunStatus_JSONDoesNotAlterHumanOutput(t *testing.T) {
	manifestPath := writeManifestFile(t, cmdApplicabilityManifest)

	var human bytes.Buffer
	if code := runStatus([]string{"--manifest", manifestPath}, &human, &bytes.Buffer{}, archPacmanFake(t)); code != 0 {
		t.Fatalf("runStatus() code = %d, want 0", code)
	}
	if !strings.Contains(human.String(), "package=git backend=pkg installed=true") {
		t.Errorf("human output = %q, want the key=value git line", human.String())
	}
	if strings.Contains(human.String(), "{") {
		t.Errorf("human output leaked JSON: %q", human.String())
	}
}

// TestRunDiff_JSONShapeAndExit verifies the --json diff output lists only
// drifted (applicable, not-installed) packages with action=install and, per the
// acceptance criteria, that diff still exits 1 on drift even with --json.
func TestRunDiff_JSONShapeAndExit(t *testing.T) {
	manifestPath := writeStatusTestManifest(t)
	fake := archPacmanFake(t)
	fake.AddPath("starship", "/usr/bin/starship")

	var stdout, stderr bytes.Buffer
	code := runDiff([]string{"--json", "--manifest", manifestPath}, &stdout, &stderr, fake)
	if code != 1 {
		t.Fatalf("runDiff(--json) code = %d, want 1 when drift present; stderr = %q", code, stderr.String())
	}

	var rep commandReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal diff JSON: %v\nstdout = %q", err, stdout.String())
	}
	if rep.Command != "diff" {
		t.Errorf("command = %q, want diff", rep.Command)
	}
	if len(rep.Packages) != 1 {
		t.Fatalf("packages = %d, want 1 (only eza drifts):\n%+v", len(rep.Packages), rep.Packages)
	}

	eza := findReport(t, rep, "eza")
	if eza.Backend != "github-release" || !eza.Applicable {
		t.Errorf("eza report = %+v, want backend=github-release applicable=true", eza)
	}
	if eza.Installed == nil || *eza.Installed {
		t.Errorf("eza installed = %v, want non-nil false", eza.Installed)
	}
	if eza.Action != "install" {
		t.Errorf("eza action = %q, want install", eza.Action)
	}

	assertReadOnlyCalls(t, fake.CallsSnapshot())
}

// TestRunDiff_JSONNoDriftExitZero verifies the aligned case: no drift emits an
// empty packages array and exits 0 (matching the human path's exit code).
func TestRunDiff_JSONNoDriftExitZero(t *testing.T) {
	manifestPath := writeStatusTestManifest(t)
	fake := archPacmanFake(t)
	fake.AddPath("eza", "/home/user/.local/bin/eza")
	fake.AddPath("starship", "/usr/bin/starship")

	var stdout, stderr bytes.Buffer
	code := runDiff([]string{"--json", "--manifest", manifestPath}, &stdout, &stderr, fake)
	if code != 0 {
		t.Fatalf("runDiff(--json) code = %d, want 0 when aligned; stderr = %q", code, stderr.String())
	}

	var rep commandReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal diff JSON: %v\nstdout = %q", err, stdout.String())
	}
	if rep.Command != "diff" {
		t.Errorf("command = %q, want diff", rep.Command)
	}
	if len(rep.Packages) != 0 {
		t.Errorf("packages = %+v, want empty array when no drift", rep.Packages)
	}
}

// TestSummarize checks the aggregate histogram over a set of report rows: the
// total plus the per-backend and per-class tallies.
func TestSummarize(t *testing.T) {
	reports := []packageReport{
		{Name: "git", Backend: "pkg", Class: "managed"},
		{Name: "eza", Backend: "github-release", Class: "pinned"},
		{Name: "starship", Backend: "script-url", Class: "pinned"},
	}
	s := summarize(reports)
	if s.Total != 3 {
		t.Errorf("Total = %d, want 3", s.Total)
	}
	if s.ByBackend["pkg"] != 1 || s.ByBackend["github-release"] != 1 || s.ByBackend["script-url"] != 1 {
		t.Errorf("ByBackend = %v, want one each", s.ByBackend)
	}
	if s.ByClass["managed"] != 1 || s.ByClass["pinned"] != 2 {
		t.Errorf("ByClass = %v, want managed=1 pinned=2", s.ByClass)
	}
}

// TestRunStatus_JSONIncludesClass verifies the additive class field is populated
// on every status --json row with the package's effective class (git=pkg→managed,
// eza=github-release→pinned, starship=script-url+sha256→pinned).
func TestRunStatus_JSONIncludesClass(t *testing.T) {
	manifestPath := writeStatusTestManifest(t)
	fake := archPacmanFake(t)
	fake.AddPath("starship", "/usr/bin/starship")

	var stdout, stderr bytes.Buffer
	code := runStatus([]string{"--json", "--manifest", manifestPath}, &stdout, &stderr, fake)
	if code != 0 {
		t.Fatalf("runStatus(--json) code = %d, want 0; stderr = %q", code, stderr.String())
	}

	var rep commandReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal status JSON: %v\nstdout = %q", err, stdout.String())
	}
	if rep.Summary != nil {
		t.Errorf("Summary = %+v, want nil without --counts", rep.Summary)
	}
	want := map[string]string{"git": "managed", "eza": "pinned", "starship": "pinned"}
	for name, class := range want {
		if got := findReport(t, rep, name).Class; got != class {
			t.Errorf("%s class = %q, want %q", name, got, class)
		}
	}
}

// TestRunStatus_CountsText verifies --counts appends the sorted count.* histogram
// after the per-package lines, and that omitting the flag leaves output with no
// count.* lines (strictly additive).
func TestRunStatus_CountsText(t *testing.T) {
	manifestPath := writeStatusTestManifest(t)

	var withCounts bytes.Buffer
	if code := runStatus([]string{"--counts", "--manifest", manifestPath}, &withCounts, &bytes.Buffer{}, archPacmanFake(t)); code != 0 {
		t.Fatalf("runStatus(--counts) code = %d, want 0", code)
	}
	out := withCounts.String()
	for _, want := range []string{
		"package=git backend=pkg installed=true",
		"count.total=3",
		"count.backend.github-release=1",
		"count.backend.pkg=1",
		"count.backend.script-url=1",
		"count.class.managed=1",
		"count.class.pinned=2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status --counts output missing %q\n%s", want, out)
		}
	}

	var noCounts bytes.Buffer
	if code := runStatus([]string{"--manifest", manifestPath}, &noCounts, &bytes.Buffer{}, archPacmanFake(t)); code != 0 {
		t.Fatalf("runStatus() code = %d, want 0", code)
	}
	if strings.Contains(noCounts.String(), "count.") {
		t.Errorf("default status output leaked count.* lines: %q", noCounts.String())
	}
}

// TestRunStatus_CountsJSON verifies --counts --json sets the summary object with
// the total and the per-backend/per-class tallies.
func TestRunStatus_CountsJSON(t *testing.T) {
	manifestPath := writeStatusTestManifest(t)

	var stdout, stderr bytes.Buffer
	code := runStatus([]string{"--counts", "--json", "--manifest", manifestPath}, &stdout, &stderr, archPacmanFake(t))
	if code != 0 {
		t.Fatalf("runStatus(--counts --json) code = %d, want 0; stderr = %q", code, stderr.String())
	}

	var rep commandReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal status JSON: %v\nstdout = %q", err, stdout.String())
	}
	if rep.Summary == nil {
		t.Fatal("Summary = nil, want a populated summary under --counts")
	}
	if rep.Summary.Total != 3 {
		t.Errorf("Summary.Total = %d, want 3", rep.Summary.Total)
	}
	if rep.Summary.ByClass["pinned"] != 2 || rep.Summary.ByClass["managed"] != 1 {
		t.Errorf("Summary.ByClass = %v, want pinned=2 managed=1", rep.Summary.ByClass)
	}
	if rep.Summary.ByBackend["pkg"] != 1 {
		t.Errorf("Summary.ByBackend = %v, want pkg=1", rep.Summary.ByBackend)
	}
}

// TestRunDiff_CountsBucketsDriftOnly verifies diff --counts summarizes only the
// drift set: with git+starship installed, only eza (github-release, pinned)
// drifts, so the histogram totals exactly one pinned github-release package.
func TestRunDiff_CountsBucketsDriftOnly(t *testing.T) {
	manifestPath := writeStatusTestManifest(t)
	fake := archPacmanFake(t)
	fake.AddPath("starship", "/usr/bin/starship")

	var stdout, stderr bytes.Buffer
	code := runDiff([]string{"--counts", "--json", "--manifest", manifestPath}, &stdout, &stderr, fake)
	if code != 1 {
		t.Fatalf("runDiff(--counts --json) code = %d, want 1 on drift; stderr = %q", code, stderr.String())
	}

	var rep commandReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal diff JSON: %v\nstdout = %q", err, stdout.String())
	}
	if rep.Summary == nil {
		t.Fatal("Summary = nil, want a populated summary under --counts")
	}
	if rep.Summary.Total != 1 {
		t.Errorf("Summary.Total = %d, want 1 (only eza drifts)", rep.Summary.Total)
	}
	if rep.Summary.ByClass["pinned"] != 1 || len(rep.Summary.ByClass) != 1 {
		t.Errorf("Summary.ByClass = %v, want only pinned=1", rep.Summary.ByClass)
	}
	if rep.Summary.ByBackend["github-release"] != 1 || len(rep.Summary.ByBackend) != 1 {
		t.Errorf("Summary.ByBackend = %v, want only github-release=1", rep.Summary.ByBackend)
	}
}

// TestRunDiff_CountsText verifies the diff --counts TEXT path (countLines): the
// histogram summarizes only the drift set. With git+starship installed, only eza
// (github-release, pinned) drifts, so the count.* lines carry the pinned
// github-release bucket and NOT the installed pkg/managed buckets.
func TestRunDiff_CountsText(t *testing.T) {
	manifestPath := writeStatusTestManifest(t)
	fake := archPacmanFake(t)
	fake.AddPath("starship", "/usr/bin/starship")

	var stdout, stderr bytes.Buffer
	code := runDiff([]string{"--counts", "--manifest", manifestPath}, &stdout, &stderr, fake)
	if code != 1 {
		t.Fatalf("runDiff(--counts) code = %d, want 1 on drift; stderr = %q", code, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"drift=declared-not-installed package=eza backend=github-release",
		"count.total=1",
		"count.backend.github-release=1",
		"count.class.pinned=1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("diff --counts output missing %q\n%s", want, out)
		}
	}
	for _, notWant := range []string{"count.backend.pkg", "count.class.managed"} {
		if strings.Contains(out, notWant) {
			t.Errorf("diff --counts leaked non-drift bucket %q (should summarize drift only)\n%s", notWant, out)
		}
	}
}

// TestRunStatus_CountsIncludesNonApplicable verifies status --counts summarizes
// non-applicable rows too: on an arch host docker-ce (apt-repo) is not
// applicable, yet it carries an effective class (managed) and is counted, so the
// histogram totals both declared packages.
func TestRunStatus_CountsIncludesNonApplicable(t *testing.T) {
	manifestPath := writeManifestFile(t, cmdApplicabilityManifest)
	fake := archPacmanFake(t)

	var stdout, stderr bytes.Buffer
	code := runStatus([]string{"--counts", "--json", "--manifest", manifestPath}, &stdout, &stderr, fake)
	if code != 0 {
		t.Fatalf("runStatus(--counts --json) code = %d, want 0; stderr = %q", code, stderr.String())
	}

	var rep commandReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal status JSON: %v\nstdout = %q", err, stdout.String())
	}
	if rep.Summary == nil {
		t.Fatal("Summary = nil, want a populated summary under --counts")
	}
	// git (pkg, applicable) + docker-ce (apt-repo, NOT applicable) — both counted,
	// both effective class managed.
	if rep.Summary.Total != 2 {
		t.Errorf("Summary.Total = %d, want 2 (non-applicable docker-ce still counted)", rep.Summary.Total)
	}
	if rep.Summary.ByBackend["apt-repo"] != 1 || rep.Summary.ByBackend["pkg"] != 1 {
		t.Errorf("Summary.ByBackend = %v, want apt-repo=1 pkg=1", rep.Summary.ByBackend)
	}
	if rep.Summary.ByClass["managed"] != 2 {
		t.Errorf("Summary.ByClass = %v, want managed=2", rep.Summary.ByClass)
	}
	// The non-applicable row still carries its effective class in the JSON.
	if got := findReport(t, rep, "docker-ce"); got.Applicable || got.Class != "managed" {
		t.Errorf("docker-ce report = %+v, want applicable=false class=managed", got)
	}
}
