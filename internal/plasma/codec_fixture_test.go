package plasma

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// Fixture roots. See testdata/kconfig/README.md for provenance and the
// scrubbing contract.
const (
	fixtureCanonical    = "testdata/kconfig/canonical"
	fixtureNonCanonical = "testdata/kconfig/noncanonical"
	fixtureReal         = "testdata/kconfig/real"
)

// fixturePaths returns every regular file under root, relative to the test's
// working directory, sorted.
func fixturePaths(t testing.TB, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() == "README.md" {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%q) error = %v", root, err)
	}
	if len(out) == 0 {
		t.Fatalf("no fixtures found under %q", root)
	}
	return out
}

func readFixture(t testing.TB, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return data
}

// assertByteIdentical is the Family A / Family C acceptance bar: our writer
// reproduces KConfigIniBackend's writer byte for byte.
func assertByteIdentical(t testing.TB, path string, raw []byte) {
	t.Helper()
	got := mustWrite(t, mustParse(t, path, raw))
	if bytes.Equal(got, raw) {
		return
	}
	t.Errorf("Write(Parse(%s)) is not byte-identical to the kwriteconfig6 output", path)
	for i, line := range bytes.Split(raw, []byte("\n")) {
		gotLines := bytes.Split(got, []byte("\n"))
		if i >= len(gotLines) {
			t.Errorf("  line %d: missing from our output, want %q", i+1, line)
			continue
		}
		if !bytes.Equal(gotLines[i], line) {
			t.Errorf("  line %d: got %q, want %q", i+1, gotLines[i], line)
		}
	}
}

// assertConverges is the Family B acceptance bar: one normalisation pass, then
// idempotence forever.
func assertConverges(t testing.TB, path string, raw []byte) []byte {
	t.Helper()
	once := mustWrite(t, mustParse(t, path, raw))
	twice := mustWrite(t, mustParse(t, path, once))
	if !bytes.Equal(once, twice) {
		t.Errorf("%s: serialisation is not idempotent after one pass\n once = %q\ntwice = %q", path, once, twice)
	}
	return once
}

// TestCodec_FamilyA_ByteIdentical is the strongest assertion in the package.
//
// The fixtures under testdata/kconfig/canonical were produced by the real
// /usr/bin/kwriteconfig6 (kconfig 6.29) against a scratch XDG_CONFIG_HOME, so
// they are already in KDE's canonical form. Reproducing them byte for byte
// proves the escape table, both comparators and the blank-line layout all match
// the implementation this codec has to interoperate with.
func TestCodec_FamilyA_ByteIdentical(t *testing.T) {
	for _, path := range fixturePaths(t, fixtureCanonical) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw := readFixture(t, path)
			assertByteIdentical(t, path, raw)
			assertConverges(t, path, raw)
		})
	}
}

// TestCodec_FamilyB_ConvergesInOnePass covers the hand-written non-canonical
// inputs: CRLF, the single-backslash `\;` form, unsorted groups, duplicate
// keys, re-opened headers, unknown flag letters, group-level [$i] and the
// `#`-key divergence. These cannot be byte-identical by construction — that is
// the point — so the contract is that one pass normalises and every pass after
// changes nothing.
func TestCodec_FamilyB_ConvergesInOnePass(t *testing.T) {
	for _, path := range fixturePaths(t, fixtureNonCanonical) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw := readFixture(t, path)
			once := assertConverges(t, path, raw)
			if bytes.Equal(once, raw) {
				t.Errorf("%s is already canonical; move it to %s or make it non-canonical", path, fixtureCanonical)
			}
		})
	}
}

// TestCodec_FamilyB_NormalisedForms pins WHAT the one normalisation pass does,
// so a change of heart about any of these shows up as a test diff rather than
// as a surprise in a user's live config.
func TestCodec_FamilyB_NormalisedForms(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{
			name: "CRLF is normalised to LF",
			path: "crlf.ini",
			want: "[Compositing]\nEnabled=true\n" +
				"\n[Windows]\nBorderlessMaximizedWindows=true\nFocusPolicy=ClickToFocus\n",
		},
		{
			name: "the .desktop carve-out converges on the doubled form",
			path: "desktop_escapes.ini",
			want: "[Desktop Entry]\n" +
				`Commas=a\\,b\\,c` + "\n" +
				`Doubled=Qt\\;KDE\\;` + "\n" +
				`Mixed=x\\;y\\;z` + "\n" +
				`Single=Qt\\;KDE\\;System\\;` + "\n",
		},
		{
			name: "flag order normalises and unknown letters survive",
			path: "flag_order.ini",
			want: "[G]\n" +
				"Dead[$d]\n" +
				"DeadValue[$d]\n" +
				"Dupes[$i]=5\n" +
				"Key[$ie]=$HOME/a\n" +
				"Key2[$ie]=$HOME/b\n" +
				"Key3[$ie]=$HOME/c\n" +
				"Mixed[$iq]=4\n" +
				"Unknown[$q]=1\n" +
				"Unknown2[$xyz]=3\n",
		},
		{
			name: "group immutability propagates onto every entry",
			path: "group_immutable.ini",
			want: "[Immutable][$i]\na[$i]=1\nb[$i]=2\nc[$i]=3\n",
		},
		{
			name: "groups sort, duplicate keys collapse, comments are dropped",
			path: "unsorted_dupes.ini",
			want: "rootentry=1\n\n[Alpha]\na=2\n\n[Zed]\ny=2\nz=1\n",
		},
		{
			name: "a leading-hash key round-trips as \\x23",
			path: "hash_key.ini",
			want: "[G]\n\\x23hashkey=v\nnormal=1\n",
		},
		{
			name: "malformed escapes recover the way KConfig's reader does",
			path: "odd_escapes.ini",
			want: "[G]\n" +
				`badhex=axb` + "\n" +
				// The lone trailing backslash is DROPPED, matching
				// printableToString. Re-encoding it as `\\` would hand KDE one
				// character more than the file on disk held.
				`lonebs=trailing` + "\n" +
				`shortx=ax` + "\n",
		},
		{
			name: "unescaped edge whitespace is trimmed, not preserved",
			path: "untrimmed.ini",
			want: "[G]\nspaced=value\ntrailing=v\n",
		},
		{
			// A group segment that carries an escape is trimmed by KDE's reader
			// and one that does not is taken verbatim, so the two spellings of
			// the SAME name converge and two different names stay apart:
			// "[a\tb ]" is the group "a<TAB>b", while "[\sa\tb\s]" keeps both
			// spaces and has to re-emit them escaped to survive.
			//
			// kwriteconfig6 6.29 collapses those two headers into a single
			// "[a\tb]" carrying both keys — a group silently merged into
			// another. Keeping them apart is a deliberate divergence.
			name: "group segments trim only when they carry an escape",
			path: "group_trim.ini",
			want: `[\sa\tb\s]` + "\nn=1\n" +
				"\n[ ab ]\nj=1\n" +
				"\n[a\\tb]\nk=1\n" +
				"\n[ab ]\nm=1\n" +
				// A decoded 0x1D is the segment joiner: KDE addresses
				// "[x\x1dy]" as [x][y] and rewrites the header that way.
				"\n[x][y]\np=1\n",
		},
		{
			// The propagated group [$i] is the one flag 'd' does not suppress.
			name: "a tombstone in an immutable group keeps the propagated flag",
			path: "immutable_tombstone.ini",
			want: "[G][$i]\nalive[$i]=1\ndead[$di]\ngone[$di]\n",
		},
		{
			// printableToString DROPS a lone trailing backslash, in every
			// context. Both lines below are byte-identical to kwriteconfig6's
			// own rewrite of this fixture.
			name: "a lone trailing backslash disappears from key, value and group",
			path: "lone_backslash.ini",
			want: "[G]\nkey=1\nplain=trailing\n\n[grp]\nk=2\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full := filepath.Join(fixtureNonCanonical, tc.path)
			got := mustWrite(t, mustParse(t, full, readFixture(t, full)))
			if string(got) != tc.want {
				t.Fatalf("Write(Parse(%s)) =\n%q\nwant\n%q", tc.path, got, tc.want)
			}
		})
	}
}

// TestCodec_FamilyC_RealCorpus runs the codec over scrubbed copies of the
// user's actual KDE files. Every identifier in them has been replaced by a
// fixed synthetic constant (see testdata/kconfig/README.md); the scrubbing
// preserves bracket nesting, escape sequences and ordering, which is all these
// tests exercise.
//
// They were written by KDE, so they are canonical and the byte-identical bar
// applies — which is what proves the codec handles the shapes that actually
// occur in the wild, not just the ones we thought to construct.
func TestCodec_FamilyC_RealCorpus(t *testing.T) {
	for _, path := range fixturePaths(t, fixtureReal) {
		rel, err := filepath.Rel(fixtureReal, path)
		if err != nil {
			t.Fatalf("Rel(%q) error = %v", path, err)
		}
		t.Run(rel, func(t *testing.T) {
			raw := readFixture(t, path)
			assertByteIdentical(t, path, raw)
			assertConverges(t, path, raw)
		})
	}
}

// TestCodec_FamilyC_IsScrubbed is a committed-data guard, not a codec test.
// These fixtures live in a public repo, so a future refresh that forgets to run
// the scrubber has to fail loudly here rather than in a git history.
//
// It is the corpus-specific half of the pair: it knows the literal names that
// occur in THESE files, which a shape-based scan cannot infer.
// TestPlasmaTree_NoMachineIdentifiers is the other half — shape-based, and
// scoped to everything the feature contributes rather than to this directory.
func TestCodec_FamilyC_IsScrubbed(t *testing.T) {
	// Each needle is a literal that must never reappear in the corpus: real
	// UUID shapes are covered by requiring the synthetic prefix instead.
	banned := []string{
		"/home/yandy", "/Users/", "yandy",
		"nas02", "anthropic", "Dropbox",
		"Logitech", "Keychron",
	}
	for _, path := range fixturePaths(t, fixtureReal) {
		rel, _ := filepath.Rel(fixtureReal, path)
		t.Run(rel, func(t *testing.T) {
			data := readFixture(t, path)
			for _, needle := range banned {
				if bytes.Contains(data, []byte(needle)) {
					t.Errorf("fixture leaks %q; re-run the scrubber before committing", needle)
				}
			}
			for _, line := range bytes.Split(data, []byte("\n")) {
				if looksLikeUUID(line) && !bytes.Contains(line, []byte(syntheticUUIDPrefix)) {
					t.Errorf("fixture leaks a non-synthetic UUID: %q", line)
				}
			}
		})
	}
}

// looksLikeUUID reports whether line contains something shaped like a UUID
// (8-4-4-4-12 hex groups), without pulling in a regexp dependency.
func looksLikeUUID(line []byte) bool {
	const shape = "........-....-....-....-............"
	for i := 0; i+len(shape) <= len(line); i++ {
		window := line[i : i+len(shape)]
		ok := true
		for j := 0; j < len(shape); j++ {
			if shape[j] == '-' {
				ok = window[j] == '-'
			} else {
				_, isHex := hexValue(window[j])
				ok = isHex
			}
			if !ok {
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
