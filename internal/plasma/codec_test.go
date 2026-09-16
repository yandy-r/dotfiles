package plasma

import (
	"bytes"
	"errors"
	"testing"
)

// NOTE: no t.Parallel() anywhere in internal/plasma. The package reassigns no
// globals today, and keeping the rule avoids the t.Parallel + package-global
// stub -race failure that is green locally and red on CI.

// mustParse parses data or fails the test.
func mustParse(t testing.TB, name string, data []byte) *Doc {
	t.Helper()
	doc, err := Parse(name, data)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", name, err)
	}
	return doc
}

// mustWrite serialises doc or fails the test.
func mustWrite(t testing.TB, doc *Doc) []byte {
	t.Helper()
	out, err := Write(doc)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	return out
}

// escaped is a thin wrapper so the escape table reads as data.
func escaped(s string, ctx escapeContext, lastSegment bool) string {
	return string(appendEscapedString(nil, s, ctx, lastSegment))
}

// TestEscape_Table drives every row of the design spec's §4.2 escape table
// through all three contexts at once. The whole point of the table is that the
// SAME input produces THREE different outputs, so each case asserts all three.
func TestEscape_Table(t *testing.T) {
	cases := []struct {
		name string
		in   string
		// lastSegment applies to the group column only.
		lastSegment bool
		group       string
		key         string
		value       string
	}{
		{name: "backslash", in: `a\b`, group: `a\\b`, key: `a\\b`, value: `a\\b`},
		{name: "backslash only", in: `\`, group: `\\`, key: `\\`, value: `\\`},
		{name: "linefeed", in: "a\nb", group: `a\nb`, key: `a\nb`, value: `a\nb`},
		{name: "tab", in: "a\tb", group: `a\tb`, key: `a\tb`, value: `a\tb`},
		{name: "carriage return", in: "a\rb", group: `a\rb`, key: `a\rb`, value: `a\rb`},
		{name: "equals", in: "a=b", group: "a=b", key: `a\x3db`, value: "a=b"},
		{name: "open bracket", in: "a[b", group: `a\x5bb`, key: `a\x5bb`, value: "a[b"},
		{name: "close bracket", in: "a]b", group: `a\x5db`, key: `a\x5db`, value: "a]b"},
		{name: "json blob keeps brackets in value", in: `{"tiles":[{"width":0.25}]}`,
			group: `{"tiles":\x5b{"width":0.25}\x5d}`,
			key:   `{"tiles":\x5b{"width":0.25}\x5d}`,
			value: `{"tiles":[{"width":0.25}]}`},
		{name: "control 0x01", in: "a\x01b", group: `a\x01b`, key: `a\x01b`, value: `a\x01b`},
		{name: "control 0x1f", in: "a\x1fb", group: `a\x1fb`, key: `a\x1fb`, value: `a\x1fb`},
		{name: "group separator 0x1d", in: "a\x1db", group: `a\x1db`, key: `a\x1db`, value: `a\x1db`},
		{name: "DEL 0x7f escapes in values only", in: "a\x7fb", group: "a\x7fb", key: "a\x7fb", value: `a\x7fb`},
		{name: "well-formed UTF-8 stays raw", in: "café → 😀", group: "café → 😀", key: "café → 😀", value: "café → 😀"},
		{name: "leading space", in: " x", group: " x", key: `\sx`, value: `\sx`},
		{name: "trailing space", in: "x ", group: "x ", key: `x\s`, value: `x\s`},
		{name: "single space is both leading and trailing", in: " ", group: " ", key: `\s`, value: `\s`},
		{name: "three spaces escape only the outer two", in: "   ", group: "   ", key: `\s \s`, value: `\s \s`},
		{name: "leading run of spaces", in: "   lead", group: "   lead", key: `\s  lead`, value: `\s  lead`},
		{name: "trailing run of spaces", in: "trail   ", group: "trail   ", key: `trail  \s`, value: `trail  \s`},
		{name: "interior spaces stay literal", in: "a  b   c", group: "a  b   c", key: "a  b   c", value: "a  b   c"},
		{name: "leading hash escapes in keys only", in: "#x", group: "#x", key: `\x23x`, value: "#x"},
		{name: "interior hash stays literal", in: "a#b", group: "a#b", key: "a#b", value: "a#b"},
		{name: "never-escaped punctuation", in: `;,"':/{}+`, group: `;,"':/{}+`, key: `;,"':/{}+`, value: `;,"':/{}+`},
		{name: "dollar segment, last", in: "$foo", lastSegment: true, group: `\x24foo`, key: "$foo", value: "$foo"},
		{name: "dollar segment, not last", in: "$foo", group: "$foo", key: "$foo", value: "$foo"},
		{name: "bare dollar, last", in: "$", lastSegment: true, group: `\x24`, key: "$", value: "$"},
		{name: "marker-shaped dollar, last", in: "$i", lastSegment: true, group: `\x24i`, key: "$i", value: "$i"},
		{name: "dollar at 10 bytes, last", in: "$abcdefghi", lastSegment: true, group: `\x24abcdefghi`, key: "$abcdefghi", value: "$abcdefghi"},
		{name: "dollar at 11 bytes stays literal", in: "$abcdefghij", lastSegment: true, group: "$abcdefghij", key: "$abcdefghij", value: "$abcdefghij"},
		{name: "dollar with uppercase stays literal", in: "$FOO", lastSegment: true, group: "$FOO", key: "$FOO", value: "$FOO"},
		{name: "dollar with digit stays literal", in: "$foo1", lastSegment: true, group: "$foo1", key: "$foo1", value: "$foo1"},
		{name: "interior dollar stays literal", in: "dollar$key", lastSegment: true, group: "dollar$key", key: "dollar$key", value: "dollar$key"},
		{name: "unescaped equals in value", in: "b=c=d", group: "b=c=d", key: `b\x3dc\x3dd`, value: "b=c=d"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escaped(tc.in, ctxGroup, tc.lastSegment); got != tc.group {
				t.Errorf("group context: escaped(%q) = %q, want %q", tc.in, got, tc.group)
			}
			if got := escaped(tc.in, ctxKey, false); got != tc.key {
				t.Errorf("key context: escaped(%q) = %q, want %q", tc.in, got, tc.key)
			}
			if got := escaped(tc.in, ctxValue, false); got != tc.value {
				t.Errorf("value context: escaped(%q) = %q, want %q", tc.in, got, tc.value)
			}
		})
	}
}

// TestEscape_Locale covers the locale dialect, which is the key dialect plus a
// leading-'$' escape. Without that escape a locale tag such as "$de" would be
// re-read as a flag suffix and the tag would silently vanish.
func TestEscape_Locale(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "de", "de"},
		{"region", "de_DE", "de_DE"},
		{"leading dollar", "$de", `\x24de`},
		{"interior dollar", "d$e", "d$e"},
		{"equals", "d=e", `d\x3de`},
		{"brackets", "d[e]", `d\x5be\x5d`},
		{"leading hash", "#de", `\x23de`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escaped(tc.in, ctxLocale, false); got != tc.want {
				t.Errorf("escaped(%q, ctxLocale) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestEscape_HighBytes proves the value context's UTF-8 validator: well-formed
// sequences are written raw, everything else is escaped per byte. Group and key
// contexts emit high bytes raw whatever they are.
func TestEscape_HighBytes(t *testing.T) {
	cases := []struct {
		name       string
		in         []byte
		wantValue  string
		wantRawCtx string
	}{
		{"valid two-byte", []byte("café"), "café", "café"},
		{"valid four-byte", []byte("😀"), "😀", "😀"},
		{"stray 0xff", []byte("bad\xffval"), `bad\xffval`, "bad\xffval"},
		{"overlong solidus", []byte("a\xc0\xafb"), `a\xc0\xafb`, "a\xc0\xafb"},
		{"truncated sequence", []byte("a\xe2\x82"), `a\xe2\x82`, "a\xe2\x82"},
		{"lone surrogate", []byte("a\xed\xa0\x80"), `a\xed\xa0\x80`, "a\xed\xa0\x80"},
		{"above U+10FFFF", []byte("a\xf5\x80\x80\x80"), `a\xf5\x80\x80\x80`, "a\xf5\x80\x80\x80"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(appendEscaped(nil, tc.in, ctxValue, false)); got != tc.wantValue {
				t.Errorf("value context = %q, want %q", got, tc.wantValue)
			}
			if got := string(appendEscaped(nil, tc.in, ctxGroup, false)); got != tc.wantRawCtx {
				t.Errorf("group context = %q, want %q", got, tc.wantRawCtx)
			}
			if got := string(appendEscaped(nil, tc.in, ctxKey, false)); got != tc.wantRawCtx {
				t.Errorf("key context = %q, want %q", got, tc.wantRawCtx)
			}
		})
	}
}

// TestUnescape_Table covers the reader half of §4.2, including the two
// deliberately asymmetric rules: the .desktop \; carve-out and the malformed
// \xNN recovery.
func TestUnescape_Table(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no escapes", "plain value", "plain value"},
		{"space", `\s`, " "},
		{"tab", `a\tb`, "a\tb"},
		{"linefeed", `a\nb`, "a\nb"},
		{"carriage return", `a\rb`, "a\rb"},
		{"backslash", `a\\b`, `a\b`},
		{"hex lowercase", `a\x41b`, "aAb"},
		{"hex uppercase digits accepted", `a\xFFb`, "a\xffb"},
		{"hex zero", `\x00`, "\x00"},
		{"semicolon carve-out keeps both characters", `Qt\;KDE\;`, `Qt\;KDE\;`},
		{"comma carve-out keeps both characters", `a\,b`, `a\,b`},
		{"doubled backslash before semicolon resolves to one", `Qt\\;`, `Qt\;`},
		// printableToString writes the backslash and then breaks WITHOUT
		// advancing its write cursor, so the closing truncate() cuts it off
		// again. Keeping it as a literal would re-encode to `\\` and hand KDE
		// one character more than the file held.
		{"lone trailing backslash is dropped", `trailing\`, `trailing`},
		{"bare backslash decodes to nothing", `\`, ``},
		{"escaped backslash at the end survives", `trailing\\`, `trailing\`},
		{"bad hex emits x and consumes both digits", `a\xZZb`, "axb"},
		{"truncated hex emits x and stops", `a\x`, "ax"},
		{"one hex digit emits x and stops", `a\x4`, "ax"},
		{"escaped edge whitespace survives", `\s  lead`, "   lead"},
		{"tab-joined shortcut list", `Meta+L\tMeta+Ctrl+Q,Screensaver\tMeta+L,Lock Session`,
			"Meta+L\tMeta+Ctrl+Q,Screensaver\tMeta+L,Lock Session"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := unescape([]byte(tc.in))
			if err != nil {
				t.Fatalf("unescape(%q) error = %v", tc.in, err)
			}
			if string(got) != tc.want {
				t.Errorf("unescape(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestUnescape_InvalidSequences proves the fail-loud half: KDE warns and leaves
// these raw, which silently changes the stored value; we refuse the input.
func TestUnescape_InvalidSequences(t *testing.T) {
	for _, in := range []string{`a\zb`, `\X41`, `a\0b`, `\ `, `a\"b`} {
		t.Run(in, func(t *testing.T) {
			if _, err := unescape([]byte(in)); err == nil {
				t.Fatalf("unescape(%q) error = nil, want an invalid-escape error", in)
			}
		})
	}
}

// TestSemicolonCarveOut_ConvergesInOnePass pins the codec's single asymmetry:
// `x\;y` and `x\\;y` both decode to the same three-character value, both
// re-encode to `x\\;y`, and the form is stable from then on.
func TestSemicolonCarveOut_ConvergesInOnePass(t *testing.T) {
	for _, in := range []string{`x\;y`, `x\\;y`} {
		decoded, err := unescape([]byte(in))
		if err != nil {
			t.Fatalf("unescape(%q) error = %v", in, err)
		}
		if string(decoded) != `x\;y` {
			t.Fatalf("unescape(%q) = %q, want %q", in, decoded, `x\;y`)
		}
		once := string(appendEscaped(nil, decoded, ctxValue, false))
		if once != `x\\;y` {
			t.Fatalf("re-encode(%q) = %q, want %q", in, once, `x\\;y`)
		}
		again, err := unescape([]byte(once))
		if err != nil {
			t.Fatalf("unescape(%q) error = %v", once, err)
		}
		if twice := string(appendEscaped(nil, again, ctxValue, false)); twice != once {
			t.Fatalf("second pass = %q, want %q", twice, once)
		}
	}
}

func TestDollarSegmentNeedsEscape(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"$", true},
		{"$i", true},
		{"$e", true},
		{"$d", true},
		{"$foo", true},
		{"$abcdefghi", true},   // 10 bytes: the inclusive upper bound
		{"$abcdefghij", false}, // 11 bytes
		{"$FOO", false},
		{"$foo1", false},
		{"$foo_", false},
		{"$foo-bar", false},
		{"foo", false},
		{"", false},
		{"a$foo", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := dollarSegmentNeedsEscape(tc.in); got != tc.want {
				t.Errorf("dollarSegmentNeedsEscape(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestCanonicalFlags pins the write-order normalisation verified against
// kwriteconfig6 6.29, including the one place we deliberately keep more than
// KDE does: unknown flag letters.
func TestCanonicalFlags(t *testing.T) {
	cases := []struct {
		name           string
		raw            string
		groupImmutable bool
		deleted        bool
		want           string
	}{
		{name: "none", want: ""},
		{name: "immutable", raw: "i", want: "i"},
		{name: "expand", raw: "e", want: "e"},
		{name: "expand then immutable normalises to ie", raw: "ei", want: "ie"},
		{name: "already canonical", raw: "ie", want: "ie"},
		{name: "duplicate letters collapse", raw: "ii", want: "i"},
		{name: "deleted suppresses the others", raw: "di", want: "d"},
		{name: "deleted suppresses expand too", raw: "edi", want: "d"},
		{name: "group immutability propagates", raw: "", groupImmutable: true, want: "i"},
		{name: "group immutability merges", raw: "e", groupImmutable: true, want: "ie"},
		{name: "Deleted field forces d", raw: "i", deleted: true, want: "d"},
		// A group-level [$i] is the ONE flag 'd' does not suppress, and KDE
		// writes it after the 'd'. Verified against kwriteconfig6 6.29: inside
		// "[G][$i]", each of a[$d], a[$di], a[$de] and a bare a[$d] tombstone
		// comes back as a[$di]. Collapsing to [$d] would leave a file that is a
		// fixed point of our writer but not of KDE's.
		{name: "group immutability survives d", raw: "d", groupImmutable: true, want: "di"},
		{name: "group immutability survives d with the entry's own letters", raw: "de", groupImmutable: true, want: "di"},
		{name: "group immutability survives the Deleted field", raw: "", groupImmutable: true, deleted: true, want: "di"},
		{name: "an entry's OWN i is still suppressed by d", raw: "di", want: "d"},
		{name: "unknown letters are preserved", raw: "q", want: "q"},
		{name: "unknown letters sort after known ones", raw: "qi", want: "iq"},
		{name: "unknown letters keep first-seen order", raw: "xyz", want: "xyz"},
		{name: "unknown letters de-duplicate", raw: "qq", want: "q"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canonicalFlags(tc.raw, tc.groupImmutable, tc.deleted); got != tc.want {
				t.Errorf("canonicalFlags(%q, %v, %v) = %q, want %q", tc.raw, tc.groupImmutable, tc.deleted, got, tc.want)
			}
		})
	}
}

// TestParse_EntryShapes covers the entry grammar: locale and flag suffixes,
// tombstones, the first-'=' split, and the empty-value case.
func TestParse_EntryShapes(t *testing.T) {
	raw := []byte("[G]\n" +
		"Plain=v\n" +
		"Empty=\n" +
		"Split=b=c=d\n" +
		"Name[de]=deutsch\n" +
		"Name[de_DE]=deutschland\n" +
		"Path[$e]=$HOME/x\n" +
		"Locked[$i]=1\n" +
		"Gone[$d]\n" +
		"Odd[fr][$ie]=oui\n")
	doc := mustParse(t, "entries", raw)

	if len(doc.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(doc.Groups))
	}
	got := make(map[string]Entry)
	for _, e := range doc.Groups[0].Entries {
		got[e.Key+"|"+e.Locale] = e
	}

	checks := []struct {
		id       string
		value    string
		flags    string
		deleted  bool
		hasValue bool
	}{
		{id: "Plain|", value: "v", hasValue: true},
		{id: "Empty|", value: "", hasValue: true},
		{id: "Split|", value: "b=c=d", hasValue: true},
		{id: "Name|de", value: "deutsch", hasValue: true},
		{id: "Name|de_DE", value: "deutschland", hasValue: true},
		{id: "Path|", value: "$HOME/x", flags: "e", hasValue: true},
		{id: "Locked|", value: "1", flags: "i", hasValue: true},
		{id: "Gone|", value: "", flags: "d", deleted: true, hasValue: false},
		{id: "Odd|fr", value: "oui", flags: "ie", hasValue: true},
	}
	for _, c := range checks {
		e, ok := got[c.id]
		if !ok {
			t.Errorf("entry %q missing", c.id)
			continue
		}
		if string(e.Value) != c.value {
			t.Errorf("entry %q value = %q, want %q", c.id, e.Value, c.value)
		}
		if e.RawFlags != c.flags {
			t.Errorf("entry %q RawFlags = %q, want %q", c.id, e.RawFlags, c.flags)
		}
		if e.Deleted != c.deleted {
			t.Errorf("entry %q Deleted = %v, want %v", c.id, e.Deleted, c.deleted)
		}
		if e.HasValue != c.hasValue {
			t.Errorf("entry %q HasValue = %v, want %v", c.id, e.HasValue, c.hasValue)
		}
	}
}

// TestParse_NestedHeadersAndImmutability covers the header grammar: nested
// segments, the group [$i] marker, and the bare [$i] file marker.
func TestParse_NestedHeadersAndImmutability(t *testing.T) {
	raw := []byte("[$i]\n" +
		"[Colors:Header][Inactive]\n" +
		"BackgroundNormal=32,35,38\n" +
		"[Libinput][1133][50504][Synthetic Mouse]\n" +
		"NaturalScroll=true\n" +
		"[Locked][$i]\n" +
		"a=1\n" +
		"[\\x24i]\n" +
		"k=v\n")
	doc := mustParse(t, "headers", raw)

	if !doc.Immutable {
		t.Error("Doc.Immutable = false, want true for a bare [$i] line")
	}
	want := [][]string{
		{"Colors:Header", "Inactive"},
		{"Libinput", "1133", "50504", "Synthetic Mouse"},
		{"Locked"},
		{"$i"},
	}
	if len(doc.Groups) != len(want) {
		t.Fatalf("groups = %d, want %d", len(doc.Groups), len(want))
	}
	for i, w := range want {
		got := doc.Groups[i].Path
		if len(got) != len(w) {
			t.Fatalf("group %d path = %v, want %v", i, got, w)
		}
		for j := range w {
			if got[j] != w[j] {
				t.Errorf("group %d segment %d = %q, want %q", i, j, got[j], w[j])
			}
		}
	}
	if !doc.Groups[2].Immutable {
		t.Error("[Locked][$i] Immutable = false, want true")
	}
	if doc.Groups[3].Immutable {
		t.Error("[\\x24i] is a group NAMED $i, not an immutability marker")
	}
}

// TestParse_GroupSegmentTrim pins the reader half of KConfig's two-mode group
// header decode, verified against kconfig 6.29 with kreadconfig6.
//
// A bracket segment with NO backslash is the group name verbatim, spaces and
// all. A segment with one is run through printableToString, which strips ASCII
// whitespace off both ends of the RAW field first — so the escape a segment
// carries decides whether its own edge spaces are part of the name.
//
// Modelling the untrimmed form splits one KDE group into two of ours: apply
// then writes both headers, KDE's last-wins coalescing picks a different value
// for any key they share, and diff reports convergence throughout because it
// compares our model against our model.
func TestParse_GroupSegmentTrim(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"no escape keeps both spaces", "[ spaced group ]\nk=1\n", []string{" spaced group "}},
		{"no escape keeps a lone trailing space", "[ab ]\nk=1\n", []string{"ab "}},
		{"an escape makes the trailing space go away", "[a\\tb ]\nk=1\n", []string{"a\tb"}},
		{"an escape makes the leading space go away too", "[ a\\tb]\nk=1\n", []string{"a\tb"}},
		{"every ASCII space byte is trimmed, not just SPACE", "[\t\va\\tb\f\r]\nk=1\n", []string{"a\tb"}},
		{"an escaped space survives the trim", "[\\sa\\tb\\s]\nk=1\n", []string{" a\tb "}},
		{"an escaped space survives on its own", "[ab\\s]\nk=1\n", []string{"ab "}},
		{"the trim runs before the escapes are decoded", "[ \\x41 ]\nk=1\n", []string{"A"}},
		// The rule is per SEGMENT, not per line: the first segment keeps its
		// space because it holds no escape, the second loses its own because it
		// does. kwriteconfig6 6.29 rewrites this header as "[A ][B\t]".
		{"the rule applies per segment, not per header", "[A ][ B\\t]\nk=1\n", []string{"A ", "B\t"}},
		// F9's lone trailing backslash, in group context: the trim runs first,
		// then the backslash is dropped.
		{"a lone trailing backslash is dropped after the trim", "[grp\\]\nk=1\n", []string{"grp"}},
		// F10b: a DECODED 0x1D is the segment joiner, and KDE splits on it.
		{"a decoded 0x1D splits the segment", "[x\\x1dy]\nk=1\n", []string{"x", "y"}},
		{"a decoded 0x1D splits a nested segment too", "[A][x\\x1dy]\nk=1\n", []string{"A", "x", "y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := mustParse(t, "trim", []byte(tc.raw))
			if len(doc.Groups) != 1 {
				t.Fatalf("groups = %d, want 1", len(doc.Groups))
			}
			got := doc.Groups[0].Path
			if len(got) != len(tc.want) {
				t.Fatalf("path = %q, want %q", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("segment %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestEscape_GroupEdgeSpaceIsConditional pins the writer half of the same rule.
//
// A group segment escapes its edge space ONLY when it already carries an
// escape, because that is exactly when KDE's reader trims. Keeping the escape
// off otherwise is what holds the Family A byte-identity for "[ spaced group ]";
// turning it on otherwise is what stops "[a\tb ]" from being read back as the
// group "a<TAB>b". Key, locale and value context always escape the edge space —
// their slices are trimmed unconditionally.
func TestEscape_GroupEdgeSpaceIsConditional(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantGroup string
		wantKey   string
	}{
		{"no escape, both edges", " spaced group ", " spaced group ", `\sspaced group\s`},
		{"no escape, trailing edge", "ab ", "ab ", `ab\s`},
		{"no escape, leading edge", " ab", " ab", `\sab`},
		{"no escape, only spaces", "   ", "   ", `\s \s`},
		{"tab escape plus trailing edge", "a\tb ", `a\tb\s`, `a\tb\s`},
		{"tab escape plus both edges", " a\tb ", `\sa\tb\s`, `\sa\tb\s`},
		{"backslash escape plus trailing edge", `a\b `, `a\\b\s`, `a\\b\s`},
		{"bracket escape plus leading edge", " a[b", `\sa\x5bb`, `\sa\x5bb`},
		{"escape without an edge space is untouched", "a\tb", `a\tb`, `a\tb`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escaped(tc.in, ctxGroup, false); got != tc.wantGroup {
				t.Errorf("group context = %q, want %q", got, tc.wantGroup)
			}
			if got := escaped(tc.in, ctxKey, false); got != tc.wantKey {
				t.Errorf("key context = %q, want %q", got, tc.wantKey)
			}
		})
	}
}

// TestGroupSegment_RoundTrip proves the two halves compose: every name below
// survives Write then Parse unchanged, including the ones whose two spellings
// KDE's own writer collapses into one.
func TestGroupSegment_RoundTrip(t *testing.T) {
	for _, name := range []string{
		" spaced group ", "ab ", " ab", "a\tb", "a\tb ", " a\tb ", `a\b `, " a[b", "   ",
	} {
		t.Run(name, func(t *testing.T) {
			doc := &Doc{Groups: []Group{{Path: []string{name}, Entries: []Entry{{Key: "k", Value: []byte("1"), HasValue: true}}}}}
			out := mustWrite(t, doc)
			back := mustParse(t, "roundtrip", out)
			if len(back.Groups) != 1 || len(back.Groups[0].Path) != 1 {
				t.Fatalf("Parse(%q) produced %d groups", out, len(back.Groups))
			}
			if got := back.Groups[0].Path[0]; got != name {
				t.Errorf("Parse(Write(%q)) = %q via %q", name, got, out)
			}
		})
	}
}

// TestEscape_LeadingBOMInKey covers the one escape that exists purely to keep
// Write's output readable by our own Parse.
//
// A root-group key beginning with U+FEFF would put a UTF-8 BOM in the file's
// first three bytes, and Parse refuses such a file because KDE decides a line is
// a group header from its first byte. Escaping the leading 0xEF breaks the
// sequence and changes nothing else: kwriteconfig6 6.29 rewrites our
// "\xef<BB><BF>=v" to exactly the same bytes it produces from a raw BOM key, so
// KDE reads both spellings as the same key. Found by FuzzCodecRoundTrip.
func TestEscape_LeadingBOMInKey(t *testing.T) {
	const bom = "\ufeff" // U+FEFF, the three bytes EF BB BF in UTF-8

	if got, want := escaped(bom+"key", ctxKey, false), `\xef`+"\xbb\xbf"+"key"; got != want {
		t.Errorf("key context = %q, want %q", got, want)
	}
	// Only the leading position and only the key dialect: a BOM anywhere else
	// is ordinary UTF-8 and stays raw.
	if got, want := escaped("a"+bom, ctxKey, false), "a"+bom; got != want {
		t.Errorf("interior BOM in a key = %q, want %q", got, want)
	}
	if got, want := escaped(bom, ctxGroup, false), bom; got != want {
		t.Errorf("group context = %q, want %q", got, want)
	}
	if got, want := escaped(bom, ctxValue, false), bom; got != want {
		t.Errorf("value context = %q, want %q", got, want)
	}

	doc := &Doc{Groups: []Group{{Entries: []Entry{{Key: bom, Value: []byte("v"), HasValue: true}}}}}
	out := mustWrite(t, doc)
	if bytes.HasPrefix(out, utf8BOM) {
		t.Fatalf("Write() opened the file with a BOM: %q", out)
	}
	if got := mustParse(t, "bomkey", out).Groups[0].Entries[0].Key; got != bom {
		t.Errorf("Parse(Write()) key = %q, want %q", got, bom)
	}
}

// TestWrite_RejectsUnrepresentableDocs proves Write fails fast rather than
// emitting bytes that would not read back as the same document.
func TestWrite_RejectsUnrepresentableDocs(t *testing.T) {
	cases := []struct {
		name string
		doc  *Doc
	}{
		{"nil document", nil},
		{"empty group segment", &Doc{Groups: []Group{{Path: []string{""}}}}},
		{"only empty nested segments", &Doc{Groups: []Group{{Path: []string{"", ""}}}}},
		{"duplicate group", &Doc{Groups: []Group{{Path: []string{"A"}}, {Path: []string{"A"}}}}},
		{"empty key", &Doc{Groups: []Group{{Path: []string{"A"}, Entries: []Entry{{Key: ""}}}}}},
		{"duplicate key identity", &Doc{Groups: []Group{{Path: []string{"A"}, Entries: []Entry{
			{Key: "k", HasValue: true}, {Key: "k", HasValue: true},
		}}}}},
		{"immutable root group", &Doc{Groups: []Group{{Immutable: true, Entries: []Entry{{Key: "k", HasValue: true}}}}}},
		// 0x1D is the joiner itself: "[a\x1db]" is the group [a][b] to KDE, so a
		// segment holding one has no faithful spelling.
		{"0x1D inside a group segment", &Doc{Groups: []Group{{Path: []string{"a\x1db"}}}}},
		// KDE matches a locale tag as raw bytes, so a tag that needs escaping
		// would serve a different locale once written.
		{"locale tag that needs escaping", &Doc{Groups: []Group{{Path: []string{"A"}, Entries: []Entry{
			{Key: "k", Locale: "d\te", HasValue: true},
		}}}}},
		{"locale tag with edge whitespace", &Doc{Groups: []Group{{Path: []string{"A"}, Entries: []Entry{
			{Key: "k", Locale: " de ", HasValue: true},
		}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Write(tc.doc); err == nil {
				t.Fatal("Write() error = nil, want a failure")
			}
		})
	}
}

// TestWrite_EmptyNestedSegmentRoundTrip: KWin's [Tiling][<uuid>][] must survive
// Parse→Write so apply's live-preserving merge can re-serialise without losing
// the empty trailing sentinel.
func TestWrite_EmptyNestedSegmentRoundTrip(t *testing.T) {
	raw := []byte("[A][]\na=1\n")
	got := mustWrite(t, mustParse(t, "fixture", raw))
	want := []byte("[A][]\na=1\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("Write(Parse()) = %q, want %q", got, want)
	}
	again := mustWrite(t, mustParse(t, "fixture", got))
	if !bytes.Equal(again, got) {
		t.Fatalf("second pass = %q, want %q", again, got)
	}
}

// TestWrite_FileImmutableMarker proves the bare [$i] marker survives a round
// trip. KDE refuses to write such a file at all, so dropping the marker would
// silently unlock it.
func TestWrite_FileImmutableMarker(t *testing.T) {
	doc := mustParse(t, "immutable", []byte("[$i]\n[G]\na=1\n"))
	got := mustWrite(t, doc)
	want := []byte("[$i]\n\n[G]\na=1\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("Write() = %q, want %q", got, want)
	}
	if again := mustWrite(t, mustParse(t, "immutable", got)); !bytes.Equal(again, got) {
		t.Fatalf("second pass = %q, want %q", again, got)
	}
}

// TestParseError_Message pins the "plasma: path:line: msg" contract the CLI
// layer matches on.
func TestParseError_Message(t *testing.T) {
	_, err := Parse("kwinrc", []byte("[General\n"))
	var perr *ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("Parse() error = %v, want *ParseError", err)
	}
	if perr.Path != "kwinrc" || perr.Line != 1 {
		t.Errorf("ParseError{Path: %q, Line: %d}, want {kwinrc, 1}", perr.Path, perr.Line)
	}
	if got := perr.Error(); got[:len("plasma: kwinrc:1: ")] != "plasma: kwinrc:1: " {
		t.Errorf("Error() = %q, want a %q prefix", got, "plasma: kwinrc:1: ")
	}
}
