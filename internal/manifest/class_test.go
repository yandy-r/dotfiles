package manifest_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

func TestClass_Valid(t *testing.T) {
	cases := map[manifest.Class]bool{
		manifest.ClassPinned:  true,
		manifest.ClassRolling: true,
		manifest.ClassManaged: true,
		"":                    false,
		"frozen":              false,
		"Pinned":              false,
	}
	for c, want := range cases {
		if got := c.Valid(); got != want {
			t.Errorf("Class(%q).Valid() = %v, want %v", c, got, want)
		}
	}
}

func TestPackage_EffectiveClass(t *testing.T) {
	cases := []struct {
		name string
		pkg  manifest.Package
		want manifest.Class
	}{
		{"pkg infers managed", manifest.Package{Backend: manifest.BackendPkg}, manifest.ClassManaged},
		{"aur infers managed", manifest.Package{Backend: manifest.BackendAUR}, manifest.ClassManaged},
		{"apt-repo infers managed", manifest.Package{Backend: manifest.BackendAPTRepo}, manifest.ClassManaged},
		{"repo infers managed", manifest.Package{Backend: manifest.BackendRepo}, manifest.ClassManaged},
		{"toolchain infers managed", manifest.Package{Backend: manifest.BackendToolchain}, manifest.ClassManaged},
		{"git-clone infers managed", manifest.Package{Backend: manifest.BackendGitClone}, manifest.ClassManaged},
		{"github-release infers pinned", manifest.Package{Backend: manifest.BackendGitHubRelease}, manifest.ClassPinned},
		{"script-url with sha256 infers pinned", manifest.Package{Backend: manifest.BackendScriptURL, SHA256: "abc"}, manifest.ClassPinned},
		{"script-url without sha256 infers rolling", manifest.Package{Backend: manifest.BackendScriptURL}, manifest.ClassRolling},
		{"app-artifact repo route (sha256) infers pinned", manifest.Package{Backend: manifest.BackendAppArtifact, Repo: "o/r", SHA256: "abc"}, manifest.ClassPinned},
		{"app-artifact repo route (checksums_asset) infers pinned", manifest.Package{Backend: manifest.BackendAppArtifact, Repo: "o/r", ChecksumsAsset: "SHA256SUMS"}, manifest.ClassPinned},
		{"app-artifact url with sha256 infers pinned", manifest.Package{Backend: manifest.BackendAppArtifact, URL: "https://x/y", SHA256: "abc"}, manifest.ClassPinned},
		{"app-artifact url without sha256 infers rolling", manifest.Package{Backend: manifest.BackendAppArtifact, URL: "https://x/y"}, manifest.ClassRolling},
		{"pkg-url repo route infers pinned", manifest.Package{Backend: manifest.BackendPkgURL, Repo: "o/r", ChecksumsAsset: "SHA256SUMS"}, manifest.ClassPinned},
		{"pkg-url url with sha256 infers pinned", manifest.Package{Backend: manifest.BackendPkgURL, URL: "https://x/y.deb", SHA256: "abc"}, manifest.ClassPinned},
		{"pkg-url url without sha256 infers rolling", manifest.Package{Backend: manifest.BackendPkgURL, URL: "https://x/y.rpm"}, manifest.ClassRolling},
		{"custom-script has no safe default", manifest.Package{Backend: manifest.BackendCustomScript}, ""},
		{"explicit class overrides inference", manifest.Package{Backend: manifest.BackendScriptURL, SHA256: "abc", Class: manifest.ClassRolling}, manifest.ClassRolling},
		{"explicit class on custom-script", manifest.Package{Backend: manifest.BackendCustomScript, Class: manifest.ClassManaged}, manifest.ClassManaged},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.pkg.EffectiveClass(); got != tc.want {
				t.Errorf("EffectiveClass() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLegalClasses(t *testing.T) {
	cases := []struct {
		backend manifest.Backend
		want    []manifest.Class
	}{
		{manifest.BackendPkg, []manifest.Class{manifest.ClassManaged}},
		{manifest.BackendAUR, []manifest.Class{manifest.ClassManaged}},
		{manifest.BackendAPTRepo, []manifest.Class{manifest.ClassManaged}},
		{manifest.BackendRepo, []manifest.Class{manifest.ClassManaged}},
		{manifest.BackendToolchain, []manifest.Class{manifest.ClassManaged}},
		{manifest.BackendGitClone, []manifest.Class{manifest.ClassManaged}},
		{manifest.BackendGitHubRelease, []manifest.Class{manifest.ClassPinned}},
		{manifest.BackendScriptURL, []manifest.Class{manifest.ClassPinned, manifest.ClassRolling}},
		{manifest.BackendCustomScript, []manifest.Class{manifest.ClassManaged, manifest.ClassPinned, manifest.ClassRolling}},
		{manifest.BackendAppArtifact, []manifest.Class{manifest.ClassPinned, manifest.ClassRolling}},
		{manifest.BackendPkgURL, []manifest.Class{manifest.ClassPinned, manifest.ClassRolling}},
	}
	for _, tc := range cases {
		t.Run(string(tc.backend), func(t *testing.T) {
			if got := manifest.LegalClasses(tc.backend); !slices.Equal(got, tc.want) {
				t.Errorf("LegalClasses(%q) = %v, want %v (sorted)", tc.backend, got, tc.want)
			}
		})
	}
}

// TestPartitionByClass proves the taxonomy cleanly buckets a package corpus:
// every package resolves to a non-empty class, and the buckets partition the
// input (acceptance 3 — the accessor #46/update consume).
func TestPartitionByClass(t *testing.T) {
	pkgs := []manifest.Package{
		{Name: "ripgrep", Backend: manifest.BackendPkg},
		{Name: "some-tool", Backend: manifest.BackendGitHubRelease},
		{Name: "starship", Backend: manifest.BackendScriptURL},
		{Name: "atuin", Backend: manifest.BackendCustomScript, Class: manifest.ClassRolling},
		{Name: "tpm", Backend: manifest.BackendCustomScript, Class: manifest.ClassManaged},
	}

	buckets := map[manifest.Class][]string{}
	for _, p := range pkgs {
		c := p.EffectiveClass()
		if c == "" {
			t.Fatalf("package %q partitions to empty class", p.Name)
		}
		if !c.Valid() {
			t.Fatalf("package %q partitions to invalid class %q", p.Name, c)
		}
		buckets[c] = append(buckets[c], p.Name)
	}

	total := 0
	for _, names := range buckets {
		total += len(names)
	}
	if total != len(pkgs) {
		t.Errorf("partition total = %d, want %d (no package double-counted or dropped)", total, len(pkgs))
	}
	if got := buckets[manifest.ClassManaged]; !slices.Contains(got, "ripgrep") || !slices.Contains(got, "tpm") {
		t.Errorf("managed bucket = %v, want ripgrep and tpm", got)
	}
	if got := buckets[manifest.ClassRolling]; !slices.Contains(got, "starship") || !slices.Contains(got, "atuin") {
		t.Errorf("rolling bucket = %v, want starship and atuin", got)
	}
	if got := buckets[manifest.ClassPinned]; !slices.Contains(got, "some-tool") {
		t.Errorf("pinned bucket = %v, want some-tool", got)
	}
}

// parseClassManifest wraps a single [[package]] body in a minimal manifest and
// parses it, returning the parse error so the class-validation tests can assert
// on it. The package is referenced by an intent so validation stays focused on
// the class rules under test.
func parseClassManifest(body string) error {
	data := "[intents.t]\ndescription = \"t\"\npackages = [\"p\"]\n\n" + body
	_, err := manifest.Parse(strings.NewReader(data))
	return err
}

func assertValidationErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Parse() error = nil, want error mentioning %q", want)
	}
	var valErr *manifest.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("Parse() error = %v, want *manifest.ValidationError", err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("Parse() error = %q, want it to mention %q", err.Error(), want)
	}
}

func TestValidate_ClassEnum(t *testing.T) {
	err := parseClassManifest(`[[package]]
name = "p"
backend = "github-release"
repo = "o/r"
tag = "v1"
asset = "a.tar.gz"
sha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
class = "frozen"
`)
	assertValidationErrorContains(t, err, "pinned, rolling, managed")
}

func TestValidate_ClassLegalForBackend(t *testing.T) {
	t.Run("pkg rejects pinned", func(t *testing.T) {
		err := parseClassManifest(`[[package]]
name = "p"
backend = "pkg"
names = { pacman = "p" }
class = "pinned"
`)
		assertValidationErrorContains(t, err, "managed")
	})

	t.Run("github-release rejects rolling", func(t *testing.T) {
		err := parseClassManifest(`[[package]]
name = "p"
backend = "github-release"
repo = "o/r"
tag = "v1"
asset = "a.tar.gz"
sha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
class = "rolling"
`)
		assertValidationErrorContains(t, err, "pinned")
	})

	t.Run("script-url accepts rolling", func(t *testing.T) {
		err := parseClassManifest(`[[package]]
name = "p"
backend = "script-url"
url = "https://example.com/i.sh"
class = "rolling"
`)
		if err != nil {
			t.Errorf("Parse() error = %v, want nil (rolling is legal for script-url)", err)
		}
	})

	t.Run("script-url rejects managed", func(t *testing.T) {
		err := parseClassManifest(`[[package]]
name = "p"
backend = "script-url"
url = "https://example.com/i.sh"
class = "managed"
`)
		assertValidationErrorContains(t, err, "pinned, rolling")
	})

	t.Run("git-clone rejects pinned", func(t *testing.T) {
		err := parseClassManifest(`[[package]]
name = "p"
backend = "git-clone"
url = "https://github.com/example/p"
dest = ".config/p"
class = "pinned"
`)
		assertValidationErrorContains(t, err, "managed")
	})
}

func TestValidate_CustomScriptRequiresClass(t *testing.T) {
	err := parseClassManifest(`[[package]]
name = "p"
backend = "custom-script"
script = "bin/install-p.sh"
justification = "tracks latest"
`)
	assertValidationErrorContains(t, err, "explicit class")
}

func TestValidate_CustomScriptRequiresJustification(t *testing.T) {
	err := parseClassManifest(`[[package]]
name = "p"
backend = "custom-script"
script = "bin/install-p.sh"
class = "rolling"
`)
	assertValidationErrorContains(t, err, "requires a justification")
}

func TestValidate_CustomScriptRejectsWhitespaceJustification(t *testing.T) {
	err := parseClassManifest(`[[package]]
name = "p"
backend = "custom-script"
script = "bin/install-p.sh"
class = "rolling"
justification = "   "
`)
	assertValidationErrorContains(t, err, "requires a justification")
}

func TestValidate_CustomScriptClassifiedOK(t *testing.T) {
	err := parseClassManifest(`[[package]]
name = "p"
backend = "custom-script"
script = "bin/install-p.sh"
class = "rolling"
justification = "self-updating installer; no pinnable artifact"
`)
	if err != nil {
		t.Errorf("Parse() error = %v, want nil for a classified+justified custom-script", err)
	}
}

func TestValidate_JustificationOptionalElsewhere(t *testing.T) {
	err := parseClassManifest(`[[package]]
name = "p"
backend = "script-url"
url = "https://example.com/i.sh"
justification = "free-form docs are allowed here"
`)
	if err != nil {
		t.Errorf("Parse() error = %v, want nil (justification is allowed on non-custom-script backends)", err)
	}
}

// TestValidate_CustomScriptPathContainment exercises the static defense-in-depth
// Script-containment fence (validateScriptPath, #49): an overlay fragment must
// not smuggle an absolute or parent-escaping custom-script path past load.
func TestValidate_CustomScriptPathContainment(t *testing.T) {
	t.Run("absolute rejected", func(t *testing.T) {
		err := parseClassManifest(`[[package]]
name = "evil"
backend = "custom-script"
script = "/etc/evil.sh"
class = "rolling"
justification = "test"
`)
		assertValidationErrorContains(t, err, "must be relative")
	})

	t.Run("traversal rejected", func(t *testing.T) {
		err := parseClassManifest(`[[package]]
name = "evil"
backend = "custom-script"
script = "../escape.sh"
class = "rolling"
justification = "test"
`)
		assertValidationErrorContains(t, err, "must not contain ..")
	})
}
