package plasma

// Every other test in this package builds its fixtures in a t.TempDir, which
// makes them hermetic and makes them blind: the configuration data `dotf plasma
// apply` ACTUALLY ships — the repo-committed plasma/files/ snapshot and the
// hand-authored plasma/recommended/ overlay — is exercised by none of them. A
// hand edit that mistypes a group header, names a path the table does not
// manage, leaks an absolute home path, or writes a non-canonical byte sequence
// therefore reaches a release, and the first thing to notice is a live
// `dotf plasma apply` on somebody's desktop.
//
// This file is that guard, and it is deliberately the ONE place that reads
// outside its temp dirs. It reads only; nothing here writes anywhere.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// payloadDir resolves one of the repo's committed plasma trees from the package
// directory. The package always runs with its own source dir as the working
// directory, so ../../ is the repo root.
func payloadDir(name string) string {
	return filepath.Join("..", "..", "plasma", name)
}

// payloadFiles walks a committed tree and returns its config-dir-relative,
// forward-slash paths. Markdown is the trees' own documentation and is not
// payload; everything else is, so a stray file shows up as an unmanaged path
// rather than being quietly skipped.
func payloadFiles(t *testing.T, dir string) []string {
	t.Helper()
	var rels []string
	root := filepath.Clean(dir)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	if len(rels) == 0 {
		t.Fatalf("%s holds no payload; the guard would pass vacuously", dir)
	}
	return rels
}

// managedSpecs indexes DefaultFiles by path.
func managedSpecs() map[string]FileSpec {
	byPath := make(map[string]FileSpec)
	for _, spec := range DefaultFiles() {
		byPath[spec.Path] = spec
	}
	return byPath
}

// TestCommittedPayload_IsManaged: every committed file must be a path
// DefaultFiles() claims. A path the table does not name is dead weight that
// apply never reads and export never refreshes — it looks applied and is not.
// checkSpec is re-run on top because the table is data: a path that is
// denylisted or escapes the config dir must fail here, not at the moment
// somebody's private key is opened.
func TestCommittedPayload_IsManaged(t *testing.T) {
	byPath := managedSpecs()
	for _, tree := range []string{"files", "recommended"} {
		for _, rel := range payloadFiles(t, payloadDir(tree)) {
			spec, ok := byPath[rel]
			if !ok {
				t.Errorf("plasma/%s/%s is committed but DefaultFiles() does not manage %q: "+
					"apply never reads it and export never refreshes it", tree, rel, rel)
				continue
			}
			if err := checkSpec(spec); err != nil {
				t.Errorf("plasma/%s/%s: checkSpec = %v", tree, rel, err)
			}
		}
	}
}

// TestCommittedPayload_ParsesAndIsCanonical: every committed INI must parse, and
// re-serialising it must reproduce it byte for byte.
//
// Canonicality is not tidiness. plasma/files/ is what export WRITES, so bytes
// that do not round-trip mean the very next `dotf plasma export` rewrites the
// file on a machine that has drifted in no way at all — a permanently dirty
// working tree, and a diff nobody can read. The same round-trip is what makes
// Converged() trustworthy, since it compares parsed documents against these
// bytes.
func TestCommittedPayload_ParsesAndIsCanonical(t *testing.T) {
	byPath := managedSpecs()
	for _, tree := range []string{"files", "recommended"} {
		dir := payloadDir(tree)
		for _, rel := range payloadFiles(t, dir) {
			spec, ok := byPath[rel]
			if !ok {
				continue // reported by TestCommittedPayload_IsManaged
			}
			abs := filepath.Join(dir, filepath.FromSlash(rel))
			data, err := os.ReadFile(abs) //nolint:gosec // fixed, repo-relative payload
			if err != nil {
				t.Fatalf("read %s: %v", abs, err)
			}
			if spec.Format == FormatRaw {
				// kdedefaults/package is a single bare line; the INI codec would
				// read it as an empty document and blank the file.
				if len(bytes.TrimSpace(data)) == 0 {
					t.Errorf("plasma/%s/%s is empty; a raw member's whole content is its payload", tree, rel)
				}
				continue
			}

			doc, err := Parse(abs, data)
			if err != nil {
				t.Errorf("plasma/%s/%s does not parse: %v", tree, rel, err)
				continue
			}
			if !hasEntries(doc) {
				t.Errorf("plasma/%s/%s names no entry; export prunes such a file rather than committing it",
					tree, rel)
			}
			got, err := Write(doc)
			if err != nil {
				t.Errorf("plasma/%s/%s does not re-serialise: %v", tree, rel, err)
				continue
			}
			if !bytes.Equal(got, data) {
				t.Errorf("plasma/%s/%s is not byte-canonical; the next export would rewrite it "+
					"on an unchanged machine:\n--- committed ---\n%s\n--- canonical ---\n%s",
					tree, rel, data, got)
			}
		}
	}
}

// TestCommittedPayload_CarriesNoHomePath runs the §14.1 privacy rules — the same
// scanPrivacy that refuses the export — over what is ALREADY committed. The
// export-time scan only ever sees the file it is about to write, so a leak that
// arrived by hand edit, by a rule added after the fact, or from a machine whose
// home directory is not this one's has nothing checking it. The rules are
// path-shaped rather than user-shaped precisely so they still bite here.
func TestCommittedPayload_CarriesNoHomePath(t *testing.T) {
	e, _, _ := testEngine(t)
	for _, tree := range []string{"files", "recommended"} {
		dir := payloadDir(tree)
		for _, rel := range payloadFiles(t, dir) {
			abs := filepath.Join(dir, filepath.FromSlash(rel))
			data, err := os.ReadFile(abs) //nolint:gosec // fixed, repo-relative payload
			if err != nil {
				t.Fatalf("read %s: %v", abs, err)
			}
			if err := e.scanPrivacy("plasma/"+tree+"/"+rel, data); err != nil {
				t.Errorf("committed payload leaks a home path: %v", err)
			}
		}
	}
}

// TestCommittedSnapshot_SurvivesItsOwnStrip: plasma/files/ is machine-generated
// BY the strip, so re-stripping it must be a no-op. A surviving volatile key
// means the snapshot predates the rule that now removes it, and the file is
// then a trap in both directions — apply writes a session-rewritten or
// machine-local value onto a fresh machine, and the next export deletes it
// again, so the pair never converges.
//
// The colour-scheme guard is deliberately passed keepColors=false, the value
// export uses whenever the active scheme has a .colors file on disk: the
// committed snapshot must be portable, and a snapshot carrying a derived
// [Colors:*] block is exactly what the guard exists to keep out of the general
// case.
//
// The mirror-image check for plasma/recommended/ — no overlay key may be one
// this table strips — is TestStripTable_DoesNotStripRecommendedKeys, which owns
// the two documented exceptions.
func TestCommittedSnapshot_SurvivesItsOwnStrip(t *testing.T) {
	byPath := managedSpecs()
	dir := payloadDir("files")
	for _, rel := range payloadFiles(t, dir) {
		spec, ok := byPath[rel]
		if !ok || spec.Format == FormatRaw {
			continue
		}
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		data, err := os.ReadFile(abs) //nolint:gosec // fixed, repo-relative payload
		if err != nil {
			t.Fatalf("read %s: %v", abs, err)
		}
		doc, err := Parse(abs, data)
		if err != nil {
			continue // reported by TestCommittedPayload_ParsesAndIsCanonical
		}

		survived := make(map[string]bool)
		for _, g := range Strip(doc, spec, false).Groups {
			for _, ent := range g.Entries {
				survived[groupHeader(g.Path)+"/"+ent.Key] = true
			}
		}
		for _, g := range doc.Groups {
			for _, ent := range g.Entries {
				if key := groupHeader(g.Path) + "/" + ent.Key; !survived[key] {
					t.Errorf("plasma/files/%s commits %s, which its own strip rules delete: "+
						"apply would write it and the next export would remove it again", rel, key)
				}
			}
		}
	}
}
