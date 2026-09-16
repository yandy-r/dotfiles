package plasma

import (
	"bytes"
	"testing"
)

// FuzzCodecRoundTrip is Family D of the fixture strategy.
//
// The invariant it drives is convergence, not byte-preservation: an arbitrary
// input may be non-canonical, but once the codec has serialised it the result
// must be a fixed point. Formally, for any input the parser accepts:
//
//	Write(Parse(Write(doc))) == Write(doc)
//
// This is what makes `dotf plasma diff` trustworthy — a converged file is never
// reopened for writing (§6.5), so a codec that failed this would rewrite a
// user's live config on every single run.
//
// The corpus is seeded from Families A, B and C so the interesting shapes (the
// escape table, the `\;` carve-out, nested headers, flag and locale suffixes,
// the real KDE corpus) are the starting points the mutator works from. It runs
// in the standard `go test` corpus mode in CI; no long-running fuzz job.
func FuzzCodecRoundTrip(f *testing.F) {
	for _, root := range []string{fixtureCanonical, fixtureNonCanonical, fixtureReal} {
		for _, path := range fixturePaths(f, root) {
			f.Add(readFixture(f, path))
		}
	}
	// A handful of hand-picked degenerate seeds the fixture corpus does not
	// carry, so the mutator starts near the parser's edges too.
	for _, seed := range []string{
		"", "\n", "[]", "[$i]", "[A][$i]", "[A]\n", "a=1", "=1", "a", "a[$d]",
		"[A][]\nk=1", "[Tiling][00000000-0000-4000-8000-000000000001][]\npadding=4",
		"[G]\na=\\", "[G]\na=\\x", "[G]\na=\\;", "[G]\nName[de][$ie]=v",
		"\xef\xbb\xbf[G]\na=1", "[\\x24i]\nk=v", "[ ]\nk=v",
		// The two-mode group header decode and its neighbours: a segment that
		// trims because it carries an escape, one that does not, one that is
		// all escape, the 0x1D split, and the lone trailing backslash in group,
		// key and value context.
		"[a\\tb ]\nk=1", "[ ab ]\nk=1", "[ab\\s]\nk=1", "[\\sa\\tb\\s]\nk=1",
		"[ \\x41 ]\nk=1", "[A ][ B\\t]\nk=1", "[x\\x1dy]\nk=1", "[\\x1d]\nk=1",
		"[grp\\]\nk=1", "[ \\ ]\nk=1", "[\\]\nk=1", "[G]\nkey\\=1",
		// The flag corner 'd' does not suppress, and the locale tags Parse
		// refuses because KDE matches a tag as raw bytes.
		"[G][$i]\na[$d]=1", "[G][$i]\na[$de]", "[G]\nName[d\\x65]=v",
		"[G]\nName[ de ]=v",
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := Parse("fuzz", data)
		if err != nil {
			return // A rejected input carries no round-trip obligation.
		}
		once, err := Write(doc)
		if err != nil {
			t.Fatalf("Write() rejected a document Parse() produced: %v\ninput = %q", err, data)
		}
		reparsed, err := Parse("fuzz", once)
		if err != nil {
			t.Fatalf("Parse() rejected our own output: %v\noutput = %q\ninput = %q", err, once, data)
		}
		twice, err := Write(reparsed)
		if err != nil {
			t.Fatalf("Write() failed on the second pass: %v\noutput = %q", err, once)
		}
		if !bytes.Equal(once, twice) {
			t.Fatalf("serialisation is not a fixed point\n once = %q\ntwice = %q\ninput = %q", once, twice, data)
		}
	})
}
