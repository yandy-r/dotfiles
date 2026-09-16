package plasma

import (
	"errors"
	"path"
	"strings"
	"testing"
)

// The counts below are the governance lock. DefaultFiles is the authoritative
// managed-file table — the manifest is NOT the source of truth here, exactly as
// symlink.DefaultTable is authoritative for links — and a stray addition would
// silently start deleting real settings on the next export. Changing a count
// requires a deliberate edit here in the same change.
const (
	wantManagedFiles = 27
	wantRawFiles     = 1
)

func TestDefaultFiles_Count(t *testing.T) {
	if got := len(DefaultFiles()); got != wantManagedFiles {
		t.Errorf("len(DefaultFiles()) = %d, want %d", got, wantManagedFiles)
	}
}

func TestDefaultFiles_FormatCounts(t *testing.T) {
	var ini, raw int
	for _, spec := range DefaultFiles() {
		switch spec.Format {
		case FormatRaw:
			raw++
		default:
			ini++
		}
	}
	if raw != wantRawFiles {
		t.Errorf("FormatRaw count = %d, want %d", raw, wantRawFiles)
	}
	if ini != wantManagedFiles-wantRawFiles {
		t.Errorf("FormatINI count = %d, want %d", ini, wantManagedFiles-wantRawFiles)
	}
	// kdedefaults/package is the one non-INI member: a single bare line that
	// the INI codec would blank.
	spec := specFor(t, "kdedefaults/package")
	if spec.Format != FormatRaw {
		t.Errorf("kdedefaults/package Format = %v, want FormatRaw", spec.Format)
	}
}

// TestDefaultFiles_MatchesGolden is the inline golden: the full path set in
// phase order. It is what makes an accidental table edit fail the build rather
// than silently change what export deletes.
func TestDefaultFiles_MatchesGolden(t *testing.T) {
	golden := []struct {
		path  string
		order int
		owned bool
	}{
		{"kdedefaults/kdeglobals", 0, false},
		{"kdedefaults/kwinrc", 0, false},
		{"kdedefaults/kcminputrc", 0, false},
		{"kdedefaults/ksplashrc", 0, false},
		{"kdedefaults/plasmarc", 0, false},
		{"kdedefaults/package", 0, false},
		{"kdeglobals", 1, false},
		{"kwinrc", 2, true},
		{"kglobalshortcutsrc", 2, true},
		{"kcminputrc", 2, false},
		{"kiorc", 2, false},
		{"kded5rc", 2, false},
		{"kscreenlockerrc", 2, false},
		{"powerdevilrc", 2, false},
		{"krunnerrc", 2, false},
		{"ksmserverrc", 2, false},
		{"kwalletrc", 2, false},
		{"dolphinrc", 2, false},
		{"konsolerc", 2, false},
		{"katerc", 2, false},
		{"katevirc", 2, false},
		{"plasma-localerc", 2, false},
		{"plasmanotifyrc", 2, true},
		{"plasmarc", 2, true},
		{"klipperrc", 2, true},
		{"baloofilerc", 2, false},
		{"mimeapps.list", 2, false},
	}

	files := DefaultFiles()
	if len(files) != len(golden) {
		t.Fatalf("len(DefaultFiles()) = %d, want %d", len(files), len(golden))
	}
	for i, want := range golden {
		got := files[i]
		if got.Path != want.path {
			t.Errorf("DefaultFiles()[%d].Path = %q, want %q", i, got.Path, want.path)
		}
		if got.ApplyOrder != want.order {
			t.Errorf("%s ApplyOrder = %d, want %d", got.Path, got.ApplyOrder, want.order)
		}
		if got.SessionOwned != want.owned {
			t.Errorf("%s SessionOwned = %v, want %v", got.Path, got.SessionOwned, want.owned)
		}
	}
}

// TestDefaultFiles_ApplyOrderPhases pins the three-phase contract: the
// kdedefaults/ overlay lands before kdeglobals, which lands before everything
// else, because the overlay is what regenerates the colour blocks export
// strips.
func TestDefaultFiles_ApplyOrderPhases(t *testing.T) {
	for _, spec := range DefaultFiles() {
		wantPhase := 2
		switch {
		case strings.HasPrefix(spec.Path, "kdedefaults/"):
			wantPhase = 0
		case spec.Path == "kdeglobals":
			wantPhase = 1
		}
		if spec.ApplyOrder != wantPhase {
			t.Errorf("%s ApplyOrder = %d, want %d", spec.Path, spec.ApplyOrder, wantPhase)
		}
	}
}

func TestDefaultFiles_PathsUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, spec := range DefaultFiles() {
		if seen[spec.Path] {
			t.Errorf("duplicate path %q", spec.Path)
		}
		seen[spec.Path] = true
		if spec.Path != path.Clean(spec.Path) || path.IsAbs(spec.Path) {
			t.Errorf("path %q must be clean and relative", spec.Path)
		}
		if err := checkSpec(spec); err != nil {
			t.Errorf("checkSpec(%q) error = %v", spec.Path, err)
		}
	}
}

// TestDefaultFiles_RulesParse proves every governed glob is well formed. A rule
// that does not parse is silently inert at runtime, which would mean a volatile
// key quietly reaching git — so the table tests, not the export path, are where
// a typo must fail.
func TestDefaultFiles_RulesParse(t *testing.T) {
	for _, spec := range DefaultFiles() {
		for _, pattern := range spec.VolatileGroups {
			if _, ok := parseGroupRule(pattern); !ok {
				t.Errorf("%s: group rule %q does not parse", spec.Path, pattern)
			}
		}
		for _, pattern := range spec.VolatileKeys {
			if _, ok := parseKeyRule(pattern); !ok {
				t.Errorf("%s: key rule %q does not parse", spec.Path, pattern)
			}
		}
		for _, pattern := range spec.VolatileValues {
			if _, ok := parseValueRule(pattern); !ok {
				t.Errorf("%s: value rule %q does not parse", spec.Path, pattern)
			}
		}
	}
}

func TestDenylist_NonEmpty(t *testing.T) {
	if len(Denylist()) == 0 {
		t.Fatal("Denylist() is empty; the fence must not be optional")
	}
	for _, prefix := range Denylist() {
		if prefix == "" || path.IsAbs(prefix) {
			t.Errorf("denylist prefix %q must be non-empty and relative", prefix)
		}
	}
}

// TestNoDenylistedPathInDefaultFiles proves the fence and the table never
// overlap: the denylist is a hard refusal, not merely an absence.
func TestNoDenylistedPathInDefaultFiles(t *testing.T) {
	for _, spec := range DefaultFiles() {
		if denylisted(spec.Path) {
			t.Errorf("managed path %q is denylisted", spec.Path)
		}
	}
}

func TestDenylisted(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"kdeconnect/privateKey.pem", true},
		{"kdeconnect/", true},
		{"krdcrc", true},
		{"kwinoutputconfig.json", true},
		{"akonadi/agent_config", true},
		{"kwinrc", false},
		{"kdeconnectfoo", false}, // prefix fences a DIRECTORY, not a name prefix
		{"kdedefaults/kwinrc", false},
	}
	for _, tc := range tests {
		if got := denylisted(tc.path); got != tc.want {
			t.Errorf("denylisted(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestCheckSpec_RejectsUnsafeAndDenylisted(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"absolute", "/etc/passwd", "unsafe"},
		{"parent escape", "../../.ssh/id_rsa", "unsafe"},
		{"interior parent", "kdedefaults/../../x", "unsafe"},
		{"empty", "", "unsafe"},
		{"denylisted", "kdeconnect/privateKey.pem", "denylist"},
		{"ok", "kwinrc", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkSpec(FileSpec{Path: tc.path})
			switch tc.want {
			case "":
				if err != nil {
					t.Fatalf("checkSpec(%q) error = %v, want nil", tc.path, err)
				}
			case "unsafe":
				var target *UnsafePathError
				if !errors.As(err, &target) {
					t.Fatalf("checkSpec(%q) error = %v, want *UnsafePathError", tc.path, err)
				}
			case "denylist":
				var target *DenylistError
				if !errors.As(err, &target) {
					t.Fatalf("checkSpec(%q) error = %v, want *DenylistError", tc.path, err)
				}
			}
		})
	}
}
