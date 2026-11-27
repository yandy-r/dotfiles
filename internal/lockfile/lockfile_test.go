package lockfile_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
)

const validLock = `version = 1

[[package]]
name = "eza"
backend = "github-release"
tag = "v0.21.0"

[[package.artifact]]
os = "linux"
arch = "amd64"
asset = "eza_x86_64.tar.gz"
binary = "eza"
sha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[[package]]
name = "starship"
backend = "script-url"
url = "https://starship.rs/install.sh"

[[package.artifact]]
sha256 = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
`

func TestParse_ValidRoundTrips(t *testing.T) {
	l, err := lockfile.Parse(strings.NewReader(validLock))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if l.Version != lockfile.Version {
		t.Errorf("Version = %d, want %d", l.Version, lockfile.Version)
	}
	if len(l.Packages) != 2 {
		t.Fatalf("Packages = %d, want 2", len(l.Packages))
	}
	if l.Packages[0].Name != "eza" || l.Packages[0].Tag != "v0.21.0" {
		t.Errorf("package[0] = %+v, want eza/v0.21.0", l.Packages[0])
	}
	if got := l.Packages[0].Artifacts[0].Binary; got != "eza" {
		t.Errorf("eza artifact binary = %q, want eza", got)
	}
	if l.Packages[1].URL != "https://starship.rs/install.sh" {
		t.Errorf("starship url = %q", l.Packages[1].URL)
	}
}

func TestParse_UnknownFieldRejected(t *testing.T) {
	const bad = "version = 1\nbogus = true\n"
	if _, err := lockfile.Parse(strings.NewReader(bad)); err == nil {
		t.Fatal("Parse() error = nil, want strict unknown-field rejection")
	}
}

func TestParse_UnsupportedVersion(t *testing.T) {
	const bad = "version = 2\n"
	_, err := lockfile.Parse(strings.NewReader(bad))
	if err == nil {
		t.Fatal("Parse() error = nil, want unsupported-version error")
	}
	if !strings.Contains(err.Error(), "unsupported version 2") {
		t.Errorf("Parse() error = %v, want unsupported version 2", err)
	}
}

func TestParse_EmptyVersionRejected(t *testing.T) {
	// An empty document decodes Version=0, which is not the supported version.
	if _, err := lockfile.Parse(strings.NewReader("")); err == nil {
		t.Fatal("Parse(empty) error = nil, want unsupported-version error")
	}
}

func TestResolve_ExplicitOverrideMissingIsHardError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.lock")
	embedded := fstest.MapFS{"manifest/dotf.lock": {Data: []byte("version = 1\n")}}
	if _, err := lockfile.Resolve(missing, "", "", embedded); err == nil {
		t.Fatal("Resolve() error = nil, want hard error on missing --lock override")
	}
}

func TestResolve_ExplicitOverrideMalformedIsHardError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dotf.lock")
	if err := os.WriteFile(path, []byte("version = 9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	embedded := fstest.MapFS{"manifest/dotf.lock": {Data: []byte("version = 1\n")}}
	if _, err := lockfile.Resolve(path, "", "", embedded); err == nil {
		t.Fatal("Resolve() error = nil, want hard error on malformed --lock override")
	}
}

func TestResolve_ManifestSibling(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "packages.toml")
	if err := os.WriteFile(filepath.Join(dir, "dotf.lock"), []byte(validLock), 0o644); err != nil {
		t.Fatal(err)
	}
	embedded := fstest.MapFS{"manifest/dotf.lock": {Data: []byte("version = 1\n")}}

	l, err := lockfile.Resolve("", manifestPath, "", embedded)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if len(l.Packages) != 2 {
		t.Errorf("resolved sibling packages = %d, want 2", len(l.Packages))
	}
}

func TestResolve_MissingSiblingIsEmptyLock(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "packages.toml") // no dotf.lock beside it
	embedded := fstest.MapFS{"manifest/dotf.lock": {Data: []byte("version = 1\n")}}

	l, err := lockfile.Resolve("", manifestPath, "", embedded)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil (missing sibling tolerated)", err)
	}
	if l.Version != lockfile.Version || len(l.Packages) != 0 {
		t.Errorf("resolved = %+v, want empty &Lock{Version:1}", l)
	}
}

func TestResolve_EmbeddedDefault(t *testing.T) {
	l, err := lockfile.Resolve("", "", "", dotfiles.ManifestFS)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if l.Version != lockfile.Version {
		t.Errorf("embedded lock version = %d, want %d", l.Version, lockfile.Version)
	}
}

func TestArtifactLookup(t *testing.T) {
	l, err := lockfile.Parse(strings.NewReader(validLock))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if _, ok := l.Package("eza"); !ok {
		t.Error("Package(eza) ok = false, want true")
	}
	if _, ok := l.Package("absent"); ok {
		t.Error("Package(absent) ok = true, want false")
	}

	if a, ok := l.Artifact("eza", "linux", "amd64"); !ok || a.Asset != "eza_x86_64.tar.gz" {
		t.Errorf("Artifact(eza, linux, amd64) = %+v, %v; want the linux/amd64 asset", a, ok)
	}
	if _, ok := l.Artifact("eza", "darwin", "arm64"); ok {
		t.Error("Artifact(eza, darwin, arm64) ok = true, want false (no such target)")
	}

	// A platform-agnostic script-url entry matches a host lookup with tokens.
	if a, ok := l.Artifact("starship", "linux", "amd64"); !ok || a.SHA256 == "" {
		t.Errorf("Artifact(starship, linux, amd64) = %+v, %v; want empty-os/arch fallback hit", a, ok)
	}
	// And an exact empty-token lookup.
	if _, ok := l.Artifact("starship", "", ""); !ok {
		t.Error("Artifact(starship, \"\", \"\") ok = false, want true")
	}
}

func TestArtifactLookup_NilReceiver(t *testing.T) {
	var l *lockfile.Lock
	if _, ok := l.Package("x"); ok {
		t.Error("nil.Package ok = true, want false")
	}
	if _, ok := l.Artifact("x", "linux", "amd64"); ok {
		t.Error("nil.Artifact ok = true, want false")
	}
}

func TestWrite_DeterministicRoundTrip(t *testing.T) {
	l := &lockfile.Lock{
		Version: lockfile.Version,
		Packages: []lockfile.LockedPackage{
			{
				Name:    "eza",
				Backend: "github-release",
				Tag:     "v0.21.0",
				Artifacts: []lockfile.Artifact{
					{OS: "linux", Arch: "arm64", Asset: "a", Binary: "eza", SHA256: strings.Repeat("a", 64)},
					{OS: "linux", Arch: "amd64", Asset: "b", Binary: "eza", SHA256: strings.Repeat("b", 64)},
				},
			},
			{
				Name:      "atool",
				Backend:   "github-release",
				Tag:       "v1",
				Artifacts: []lockfile.Artifact{{OS: "linux", Arch: "amd64", Asset: "c", Binary: "atool", SHA256: strings.Repeat("c", 64)}},
			},
		},
	}

	var buf bytes.Buffer
	if err := lockfile.Write(&buf, l); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Packages sort by name (atool before eza); artifacts by os then arch
	// (amd64 before arm64).
	out := buf.String()
	if strings.Index(out, `name = "atool"`) > strings.Index(out, `name = "eza"`) {
		t.Errorf("Write() did not sort packages by name:\n%s", out)
	}
	if strings.Index(out, `arch = "amd64"`) > strings.Index(out, `arch = "arm64"`) {
		t.Errorf("Write() did not sort artifacts by arch:\n%s", out)
	}

	// Re-writing the same lock is byte-identical.
	var buf2 bytes.Buffer
	if err := lockfile.Write(&buf2, l); err != nil {
		t.Fatalf("Write() second error = %v", err)
	}
	if buf.String() != buf2.String() {
		t.Error("Write() is not deterministic across calls")
	}

	// Parse(Write(l)) equals the canonically-sorted lock.
	got, err := lockfile.Parse(strings.NewReader(out))
	if err != nil {
		t.Fatalf("Parse(Write(l)) error = %v", err)
	}
	want := &lockfile.Lock{
		Version: lockfile.Version,
		Packages: []lockfile.LockedPackage{
			{
				Name:      "atool",
				Backend:   "github-release",
				Tag:       "v1",
				Artifacts: []lockfile.Artifact{{OS: "linux", Arch: "amd64", Asset: "c", Binary: "atool", SHA256: strings.Repeat("c", 64)}},
			},
			{
				Name:    "eza",
				Backend: "github-release",
				Tag:     "v0.21.0",
				Artifacts: []lockfile.Artifact{
					{OS: "linux", Arch: "amd64", Asset: "b", Binary: "eza", SHA256: strings.Repeat("b", 64)},
					{OS: "linux", Arch: "arm64", Asset: "a", Binary: "eza", SHA256: strings.Repeat("a", 64)},
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(Write(l)) =\n%+v\nwant\n%+v", got, want)
	}
}

func TestWrite_EmptyLock(t *testing.T) {
	var buf bytes.Buffer
	if err := lockfile.Write(&buf, &lockfile.Lock{Version: lockfile.Version}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := lockfile.Parse(&buf)
	if err != nil {
		t.Fatalf("Parse(Write(empty)) error = %v", err)
	}
	if got.Version != lockfile.Version || len(got.Packages) != 0 {
		t.Errorf("round-tripped empty lock = %+v", got)
	}
}

func TestPinnable(t *testing.T) {
	cases := []struct {
		name string
		pkg  manifest.Package
		want bool
	}{
		{"github-release infers pinned", manifest.Package{Backend: manifest.BackendGitHubRelease}, true},
		{"script-url with sha is pinned", manifest.Package{Backend: manifest.BackendScriptURL, SHA256: strings.Repeat("a", 64)}, true},
		{"script-url without sha is rolling", manifest.Package{Backend: manifest.BackendScriptURL}, false},
		{"pkg is managed", manifest.Package{Backend: manifest.BackendPkg}, false},
		{"aur is managed", manifest.Package{Backend: manifest.BackendAUR}, false},
		{"repo is managed", manifest.Package{Backend: manifest.BackendRepo}, false},
		{"toolchain is managed", manifest.Package{Backend: manifest.BackendToolchain}, false},
		{"custom-script without class is not pinned", manifest.Package{Backend: manifest.BackendCustomScript}, false},
		{"explicit rolling on github overrides to false", manifest.Package{Backend: manifest.BackendGitHubRelease, Class: manifest.ClassRolling}, false},
		{"explicit pinned custom-script is true", manifest.Package{Backend: manifest.BackendCustomScript, Class: manifest.ClassPinned}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lockfile.Pinnable(tc.pkg); got != tc.want {
				t.Errorf("Pinnable(%+v) = %v, want %v", tc.pkg, got, tc.want)
			}
		})
	}
}

// TestParse_EmptySHARejected pins YAN-1417: a lock artifact with an empty,
// blank or missing sha256 is rejected rather than accepted as a lock hit with
// nothing to verify.
func TestParse_EmptySHARejected(t *testing.T) {
	cases := map[string]string{
		"empty": `version = 1

[[package]]
name = "tool"
backend = "github-release"
tag = "v1"

[[package.artifact]]
os = "debian"
arch = "arm64"
asset = "tool-arm64.tar.gz"
binary = "tool"
sha256 = ""
`,
		"blank": `version = 1

[[package]]
name = "tool"
backend = "github-release"
tag = "v1"

[[package.artifact]]
os = "debian"
arch = "arm64"
asset = "tool-arm64.tar.gz"
sha256 = "   "
`,
		"missing": `version = 1

[[package]]
name = "pin"
backend = "script-url"
url = "https://example.com/install.sh"

[[package.artifact]]
`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			l, err := lockfile.Parse(strings.NewReader(doc))
			if err == nil {
				t.Fatalf("Parse() = %+v, nil; want an empty-sha256 error", l)
			}
			if l != nil {
				t.Errorf("Parse() lock = %+v, want nil on error", l)
			}
			if !strings.Contains(err.Error(), "empty sha256") {
				t.Errorf("Parse() error = %v, want it to name the empty sha256", err)
			}
		})
	}
}

// TestWrite_EmptySHARejected asserts the write side refuses an empty checksum
// too, so `dotf lock` can never emit a row Parse would reject.
func TestWrite_EmptySHARejected(t *testing.T) {
	l := &lockfile.Lock{
		Version: lockfile.Version,
		Packages: []lockfile.LockedPackage{{
			Name: "tool", Backend: "github-release", Tag: "v1",
			Artifacts: []lockfile.Artifact{
				{OS: "linux", Arch: "amd64", Asset: "a", Binary: "tool", SHA256: strings.Repeat("a", 64)},
				{OS: "debian", Arch: "arm64", Asset: "b", Binary: "tool"},
			},
		}},
	}
	var buf bytes.Buffer
	err := lockfile.Write(&buf, l)
	if err == nil || !strings.Contains(err.Error(), "debian/arm64") {
		t.Fatalf("Write() error = %v, want an empty-sha256 error naming debian/arm64", err)
	}
	if buf.Len() != 0 {
		t.Errorf("Write() emitted %q despite the validation error", buf.String())
	}
}
