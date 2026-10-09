package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/lockfile"
)

func lockSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// lockTestManifest is a synthetic manifest whose pinnable set is a single
// checksummed script-url (resolved against the test server), alongside a
// rolling script-url and a managed pkg that must be excluded from the lock.
func lockTestManifest(serverURL string) string {
	return fmt.Sprintf(`[[package]]
name = "pinnedtool"
backend = "script-url"
url = "%s/pinned.sh"
sha256 = "%s"

[[package]]
name = "starship"
backend = "script-url"
url = "%s/rolling.sh"

[[package]]
name = "git"
backend = "pkg"
names = { pacman = "git", apt = "git", dnf = "git", zypper = "git", homebrew = "git" }
`, serverURL, strings.Repeat("a", 64), serverURL)
}

// lockTestServer serves the pinned/rolling installer payloads over HTTPS (the
// manifest requires an https script-url); pinnedBody is a pointer so a test can
// mutate it between --check runs to simulate drift. It points http.DefaultClient
// at the server's trusting client for the test's duration so the backend's
// default http.Get accepts the self-signed cert.
func lockTestServer(t *testing.T, pinnedBody *[]byte) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/pinned.sh":
			_, _ = w.Write(*pinnedBody)
		case "/rolling.sh":
			_, _ = w.Write([]byte("#!/bin/sh\necho rolling\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	prev := http.DefaultClient
	http.DefaultClient = srv.Client()
	t.Cleanup(func() { http.DefaultClient = prev })

	return srv
}

func writeLockManifest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "packages.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func TestRunLock_WritesPinnableOnly(t *testing.T) {
	pinned := []byte("#!/bin/sh\necho pinned\n")
	body := pinned
	srv := lockTestServer(t, &body)
	defer srv.Close()

	repoRoot := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	manifestPath := writeLockManifest(t, lockTestManifest(srv.URL))

	var out, errb bytes.Buffer
	code := runLock([]string{"--manifest", manifestPath}, &out, &errb, exec.NewFake())
	if code != 0 {
		t.Fatalf("runLock() code = %d, want 0 (stderr=%q)", code, errb.String())
	}

	data, err := os.ReadFile(filepath.Join(repoRoot, "manifest", "dotf.lock"))
	if err != nil {
		t.Fatalf("reading written lock: %v", err)
	}
	l, err := lockfile.Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Parse(written lock) error = %v\n%s", err, data)
	}

	if _, ok := l.Package("pinnedtool"); !ok {
		t.Error("lock missing pinnedtool")
	}
	if _, ok := l.Package("starship"); ok {
		t.Error("lock includes rolling starship, want it excluded")
	}
	if _, ok := l.Package("git"); ok {
		t.Error("lock includes managed git, want it excluded")
	}
	art, ok := l.Artifact("pinnedtool", "", "")
	if !ok {
		t.Fatal("lock missing pinnedtool artifact")
	}
	if art.SHA256 != lockSHA(pinned) {
		t.Errorf("locked sha = %q, want %q", art.SHA256, lockSHA(pinned))
	}
	if !strings.Contains(out.String(), "locked=pinnedtool") {
		t.Errorf("stdout = %q, want it to name the locked package", out.String())
	}
}

func TestRunLock_CheckDrift(t *testing.T) {
	pinned := []byte("#!/bin/sh\necho v1\n")
	body := pinned
	srv := lockTestServer(t, &body)
	defer srv.Close()

	repoRoot := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	manifestPath := writeLockManifest(t, lockTestManifest(srv.URL))

	// Seed the committed lock.
	if code := runLock([]string{"--manifest", manifestPath}, &bytes.Buffer{}, &bytes.Buffer{}, exec.NewFake()); code != 0 {
		t.Fatalf("seeding runLock() code = %d, want 0", code)
	}

	// --check against the current server is clean.
	var out, errb bytes.Buffer
	if code := runLock([]string{"--manifest", manifestPath, "--check"}, &out, &errb, exec.NewFake()); code != 0 {
		t.Fatalf("check (current) code = %d, want 0 (stderr=%q)", code, errb.String())
	}
	if !strings.Contains(out.String(), "check=current") {
		t.Errorf("check stdout = %q, want check=current", out.String())
	}
	// Nothing was written differently.
	before, _ := os.ReadFile(filepath.Join(repoRoot, "manifest", "dotf.lock"))

	// Mutate the served payload: the resolved sha now drifts from the committed one.
	body = []byte("#!/bin/sh\necho v2-changed\n")

	out.Reset()
	errb.Reset()
	if code := runLock([]string{"--manifest", manifestPath, "--check"}, &out, &errb, exec.NewFake()); code != 1 {
		t.Fatalf("check (drift) code = %d, want 1 (stderr=%q)", code, errb.String())
	}
	if !strings.Contains(out.String(), "check=drift") {
		t.Errorf("drift stdout = %q, want check=drift", out.String())
	}
	after, _ := os.ReadFile(filepath.Join(repoRoot, "manifest", "dotf.lock"))
	if string(before) != string(after) {
		t.Error("--check wrote to the lock file, want no write")
	}
}

func TestRunLock_JSON(t *testing.T) {
	pinned := []byte("#!/bin/sh\necho json\n")
	body := pinned
	srv := lockTestServer(t, &body)
	defer srv.Close()

	t.Setenv("DOTFILES_ROOT", t.TempDir())
	manifestPath := writeLockManifest(t, lockTestManifest(srv.URL))

	var out, errb bytes.Buffer
	code := runLock([]string{"--manifest", manifestPath, "--json"}, &out, &errb, exec.NewFake())
	if code != 0 {
		t.Fatalf("runLock(--json) code = %d, want 0 (stderr=%q)", code, errb.String())
	}

	var rep lockReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("decoding --json output: %v\n%s", err, out.String())
	}
	if rep.Command != "lock" {
		t.Errorf("report command = %q, want lock", rep.Command)
	}
	if len(rep.Packages) != 1 {
		t.Fatalf("report packages = %d, want 1 (pinnable only)", len(rep.Packages))
	}
	p := rep.Packages[0]
	if p.Name != "pinnedtool" || p.Backend != "script-url" || p.URL != srv.URL+"/pinned.sh" {
		t.Errorf("report package = %+v, want pinnedtool script-url with url", p)
	}
	if len(p.Artifacts) != 1 || p.Artifacts[0].SHA256 != lockSHA(pinned) {
		t.Errorf("report artifacts = %+v, want one with the resolved sha", p.Artifacts)
	}
}

func TestRunLock_OnlyDryRunWritesNothing(t *testing.T) {
	pinned := []byte("#!/bin/sh\necho only\n")
	body := pinned
	srv := lockTestServer(t, &body)
	defer srv.Close()

	repoRoot := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	manifestPath := writeLockManifest(t, lockTestManifest(srv.URL))

	var out, errb bytes.Buffer
	code := runLock([]string{"--manifest", manifestPath, "--only", "pinnedtool", "--dry-run"}, &out, &errb, exec.NewFake())
	if code != 0 {
		t.Fatalf("runLock(--only --dry-run) code = %d, want 0 (stderr=%q)", code, errb.String())
	}
	if !strings.Contains(out.String(), "dry-run=true") || !strings.Contains(out.String(), "locked=pinnedtool") {
		t.Errorf("dry-run stdout = %q, want dry-run=true and locked=pinnedtool", out.String())
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "manifest", "dotf.lock")); !os.IsNotExist(err) {
		t.Errorf("--dry-run wrote a lock file, want none")
	}
}

// lockTestManifestTwoPinned adds a second pinnable script-url pointing at a path
// the test server 404s (/dead.sh), so one package resolves and one fails.
func lockTestManifestTwoPinned(serverURL string) string {
	return fmt.Sprintf(`[[package]]
name = "pinnedtool"
backend = "script-url"
url = "%s/pinned.sh"
sha256 = "%s"

[[package]]
name = "deadtool"
backend = "script-url"
url = "%s/dead.sh"
sha256 = "%s"
`, serverURL, strings.Repeat("a", 64), serverURL, strings.Repeat("b", 64))
}

// TestRunLock_DryRunJSON asserts --dry-run --json emits valid JSON (the --json
// format wins over --dry-run's text) and writes no lock file.
func TestRunLock_DryRunJSON(t *testing.T) {
	body := []byte("#!/bin/sh\necho j\n")
	srv := lockTestServer(t, &body)
	defer srv.Close()

	repoRoot := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	manifestPath := writeLockManifest(t, lockTestManifest(srv.URL))

	var out, errb bytes.Buffer
	code := runLock([]string{"--manifest", manifestPath, "--dry-run", "--json"}, &out, &errb, exec.NewFake())
	if code != 0 {
		t.Fatalf("runLock(--dry-run --json) code = %d, want 0 (stderr=%q)", code, errb.String())
	}
	var rep lockReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("--dry-run --json did not emit JSON: %v\n%s", err, out.String())
	}
	if rep.Command != "lock" {
		t.Errorf("report command = %q, want lock", rep.Command)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "manifest", "dotf.lock")); !os.IsNotExist(err) {
		t.Errorf("--dry-run --json wrote a lock file, want none")
	}
}

// TestRunLock_CheckRejectsOnlySkip asserts --check with --only/--skip is a usage
// error (a scoped check would false-drift against the full committed lock).
func TestRunLock_CheckRejectsOnlySkip(t *testing.T) {
	manifestPath := writeLockManifest(t, `[[package]]
name = "git"
backend = "pkg"
names = { pacman = "git", apt = "git", dnf = "git", zypper = "git", homebrew = "git" }
`)
	var out, errb bytes.Buffer
	code := runLock([]string{"--manifest", manifestPath, "--check", "--only", "git"}, &out, &errb, exec.NewFake())
	if code != 2 {
		t.Fatalf("runLock(--check --only) code = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "--check cannot be combined with --only/--skip") {
		t.Errorf("stderr = %q, want the --check/--only conflict message", errb.String())
	}
}

// TestRunLock_CheckErrorOnResolutionFailure asserts --check reports a distinct
// error verdict (not false drift) when resolution fails for a package.
func TestRunLock_CheckErrorOnResolutionFailure(t *testing.T) {
	body := []byte("#!/bin/sh\necho x\n")
	srv := lockTestServer(t, &body)
	defer srv.Close()

	t.Setenv("DOTFILES_ROOT", t.TempDir())
	// deadtool resolves against /dead.sh (404), so resolution fails.
	manifestPath := writeLockManifest(t, lockTestManifestTwoPinned(srv.URL))

	var out, errb bytes.Buffer
	code := runLock([]string{"--manifest", manifestPath, "--check"}, &out, &errb, exec.NewFake())
	if code != 1 {
		t.Fatalf("runLock(--check, resolution fails) code = %d, want 1 (stderr=%q)", code, errb.String())
	}
	if !strings.Contains(out.String(), "check=error") {
		t.Errorf("stdout = %q, want check=error (not false drift)", out.String())
	}
	if strings.Contains(out.String(), "check=drift") {
		t.Errorf("stdout = %q, must not report drift on a resolution failure", out.String())
	}
}

// TestRunLock_PartialFailureExits1 asserts the non-fatal per-package failure
// path: a dead pinnable is summarized as failed (exit 1) but the good pinnable
// is still recorded, and the failed one carries no lock entry.
func TestRunLock_PartialFailureExits1(t *testing.T) {
	pinned := []byte("#!/bin/sh\necho ok\n")
	body := pinned
	srv := lockTestServer(t, &body)
	defer srv.Close()

	repoRoot := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	manifestPath := writeLockManifest(t, lockTestManifestTwoPinned(srv.URL))

	var out, errb bytes.Buffer
	code := runLock([]string{"--manifest", manifestPath}, &out, &errb, exec.NewFake())
	if code != 1 {
		t.Fatalf("runLock() code = %d, want 1 on a partial failure (stderr=%q)", code, errb.String())
	}
	if !strings.Contains(errb.String(), "failed=1") || !strings.Contains(errb.String(), "deadtool") {
		t.Errorf("stderr = %q, want failed=1 naming deadtool", errb.String())
	}

	data, err := os.ReadFile(filepath.Join(repoRoot, "manifest", "dotf.lock"))
	if err != nil {
		t.Fatalf("reading written lock: %v", err)
	}
	l, err := lockfile.Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Parse(written lock) error = %v\n%s", err, data)
	}
	if _, ok := l.Package("pinnedtool"); !ok {
		t.Error("lock missing the good pinnedtool despite the partial failure")
	}
	if _, ok := l.Package("deadtool"); ok {
		t.Error("lock includes deadtool, which failed to resolve")
	}
}

// TestRunLock_MultiPlatformDedup asserts multiple --platform targets over a
// platform-agnostic script-url pin collapse to a single "any" artifact.
func TestRunLock_MultiPlatformDedup(t *testing.T) {
	body := []byte("#!/bin/sh\necho m\n")
	srv := lockTestServer(t, &body)
	defer srv.Close()

	t.Setenv("DOTFILES_ROOT", t.TempDir())
	manifestPath := writeLockManifest(t, lockTestManifest(srv.URL))

	var out, errb bytes.Buffer
	code := runLock([]string{"--manifest", manifestPath, "--only", "pinnedtool",
		"--platform", "linux/amd64", "--platform", "darwin/arm64", "--dry-run"}, &out, &errb, exec.NewFake())
	if code != 0 {
		t.Fatalf("runLock(multi --platform) code = %d, want 0 (stderr=%q)", code, errb.String())
	}
	if n := strings.Count(out.String(), "artifact.pinnedtool.any="); n != 1 {
		t.Errorf("artifact.pinnedtool.any lines = %d, want exactly 1 (dedup across targets)\n%s", n, out.String())
	}
}

// TestRunLock_InvalidPlatform asserts a malformed --platform is a usage error.
func TestRunLock_InvalidPlatform(t *testing.T) {
	manifestPath := writeLockManifest(t, `[[package]]
name = "git"
backend = "pkg"
names = { pacman = "git", apt = "git", dnf = "git", zypper = "git", homebrew = "git" }
`)
	var out, errb bytes.Buffer
	code := runLock([]string{"--manifest", manifestPath, "--platform", "notslashed"}, &out, &errb, exec.NewFake())
	if code != 2 {
		t.Fatalf("runLock(--platform notslashed) code = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "want os/arch") {
		t.Errorf("stderr = %q, want the os/arch usage hint", errb.String())
	}
}

func TestParsePlatform(t *testing.T) {
	ok, err := parsePlatform("linux/amd64")
	if err != nil {
		t.Fatalf("parsePlatform(linux/amd64) error = %v, want nil", err)
	}
	if string(ok.OS) != "linux" || string(ok.Arch) != "x86_64" {
		t.Errorf("parsePlatform(linux/amd64) = %+v, want os=linux arch=x86_64", ok)
	}
	for _, bad := range []string{"", "linux", "/amd64", "linux/"} {
		if _, err := parsePlatform(bad); err == nil {
			t.Errorf("parsePlatform(%q) error = nil, want an error", bad)
		}
	}
}

// TestRunLock_PinnedCustomScriptNotLockable asserts a pinned custom-script (a
// non-lockable backend) surfaces as a non-fatal resolution failure.
func TestRunLock_PinnedCustomScriptNotLockable(t *testing.T) {
	t.Setenv("DOTFILES_ROOT", t.TempDir())
	manifestPath := writeLockManifest(t, `[[package]]
name = "weird"
backend = "custom-script"
script = "bin/install-weird.sh"
class = "pinned"
justification = "hand-pinned in the script"
`)
	var out, errb bytes.Buffer
	code := runLock([]string{"--manifest", manifestPath}, &out, &errb, exec.NewFake())
	if code != 1 {
		t.Fatalf("runLock(pinned custom-script) code = %d, want 1 (stderr=%q)", code, errb.String())
	}
	if !strings.Contains(errb.String(), "not lockable") {
		t.Errorf("stderr = %q, want a 'not lockable' failure", errb.String())
	}
}

// TestRunLock_CheckMissingLockIsDrift asserts --check against a repo with no
// committed lock (fresh repo / first CI gate) reports drift and writes nothing.
func TestRunLock_CheckMissingLockIsDrift(t *testing.T) {
	body := []byte("#!/bin/sh\necho fresh\n")
	srv := lockTestServer(t, &body)
	defer srv.Close()

	repoRoot := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	manifestPath := writeLockManifest(t, lockTestManifest(srv.URL))

	var out, errb bytes.Buffer
	code := runLock([]string{"--manifest", manifestPath, "--check"}, &out, &errb, exec.NewFake())
	if code != 1 {
		t.Fatalf("runLock(--check, no committed lock) code = %d, want 1 (stderr=%q)", code, errb.String())
	}
	if !strings.Contains(out.String(), "check=drift") {
		t.Errorf("stdout = %q, want check=drift", out.String())
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "manifest", "dotf.lock")); !os.IsNotExist(err) {
		t.Errorf("--check wrote a lock file, want none")
	}
}

// TestWriteLockText_Tokens asserts the greppable artifact lines: a per-(os,arch)
// github-release token renders as os/arch, a platform-agnostic script-url pin as
// "any".
func TestWriteLockText_Tokens(t *testing.T) {
	lock := &lockfile.Lock{
		Version: lockfile.Version,
		Packages: []lockfile.LockedPackage{
			{Name: "ghtool", Backend: "github-release", Tag: "v1", Artifacts: []lockfile.Artifact{
				{OS: "linux", Arch: "amd64", Asset: "ghtool.tar.gz", Binary: "ghtool", SHA256: strings.Repeat("c", 64)},
			}},
			{Name: "scripttool", Backend: "script-url", URL: "https://x/i.sh", Artifacts: []lockfile.Artifact{
				{SHA256: strings.Repeat("d", 64)},
			}},
		},
	}
	var buf bytes.Buffer
	if err := writeLockText(&buf, lock, false); err != nil {
		t.Fatalf("writeLockText() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"locked=ghtool,scripttool",
		"artifact.ghtool.linux/amd64=" + strings.Repeat("c", 64),
		"artifact.scripttool.any=" + strings.Repeat("d", 64),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("writeLockText output missing %q\n%s", want, out)
		}
	}
}

// TestRunInstall_VerifiesLockedChecksum proves install actually consults the
// committed lock: a lock seeded with a wrong sha for a pinnable script-url makes
// Install fail on the checksum mismatch and installs nothing.
func TestRunInstall_VerifiesLockedChecksum(t *testing.T) {
	body := []byte("#!/bin/sh\necho payload\n")
	srv := lockTestServer(t, &body)
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("DOTFILES_ROOT", dir)
	manifestPath := filepath.Join(dir, "packages.toml")
	if err := os.WriteFile(manifestPath, []byte(lockTestManifest(srv.URL)), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	// Seed the sibling lock with a WRONG sha for pinnedtool.
	seed := fmt.Sprintf(`version = 1

[[package]]
name = "pinnedtool"
backend = "script-url"
url = "%s/pinned.sh"

[[package.artifact]]
sha256 = "%s"
`, srv.URL, strings.Repeat("e", 64))
	if err := os.WriteFile(filepath.Join(dir, "dotf.lock"), []byte(seed), 0o644); err != nil {
		t.Fatalf("write seed lock: %v", err)
	}

	var out, errb bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath, "--only", "pinnedtool"}, &out, &errb, exec.NewFake())
	if code != 1 {
		t.Fatalf("runInstall() code = %d, want 1 (locked-sha mismatch); stderr=%q", code, errb.String())
	}
	if !strings.Contains(errb.String(), "failed=1") {
		t.Errorf("stderr = %q, want failed=1 from the locked-sha mismatch", errb.String())
	}
	if !strings.Contains(errb.String(), "SHA256 mismatch") {
		t.Errorf("stderr = %q, want the SHA256 mismatch diagnostic", errb.String())
	}
}

func TestRunLock_RepoRootEmptyErrors(t *testing.T) {
	t.Setenv("DOTFILES_ROOT", "")
	t.Chdir(t.TempDir()) // no manifest/packages.toml above this cwd

	// A manifest with no pinnable packages: resolution is a no-op, so the run
	// reaches the write path and fails resolving the output path.
	manifestPath := writeLockManifest(t, `[[package]]
name = "git"
backend = "pkg"
names = { pacman = "git", apt = "git", dnf = "git", zypper = "git", homebrew = "git" }
`)

	var out, errb bytes.Buffer
	code := runLock([]string{"--manifest", manifestPath}, &out, &errb, exec.NewFake())
	if code == 0 {
		t.Fatalf("runLock() code = 0, want non-zero when repo root is unresolved")
	}
	if !strings.Contains(errb.String(), "repo root") {
		t.Errorf("stderr = %q, want it to mention the missing repo root", errb.String())
	}
}
