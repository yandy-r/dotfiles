package plasma

import (
	"bytes"
	"testing"
)

// TestOrder_GroupsAreUTF16_KeysAreUTF8 is the load-bearing comparator test.
//
// KConfig sorts group names as QStrings (UTF-16 code units) and key names as
// byte arrays (UTF-8). The two orders are not merely different, they are
// OPPOSITE for the same pair of names: U+1F600 encodes as the surrogate pair
// D83D DE00 in UTF-16 and as F0 9F 98 80 in UTF-8, while U+FFFD encodes as FFFD
// and EF BF BD. So D83D < FFFD puts the emoji FIRST among groups, and
// EF BF BD < F0 9F 98 80 puts it LAST among keys.
//
// Using sort.Strings for group names would silently reorder any file with
// non-BMP group names; this test is what stops that regression.
func TestOrder_GroupsAreUTF16_KeysAreUTF8(t *testing.T) {
	const emoji = "\U0001F600"
	const replacement = "\uFFFD"

	if got := compareUTF16(emoji, replacement); got >= 0 {
		t.Errorf("compareUTF16(%q, %q) = %d, want < 0 (UTF-16 order)", emoji, replacement, got)
	}
	if emoji < replacement {
		t.Errorf("UTF-8 byte order must place %q AFTER %q; the test premise is broken", emoji, replacement)
	}

	doc := &Doc{Groups: []Group{
		{Path: []string{replacement}, Entries: []Entry{{Key: "k", Value: []byte("v")}}},
		{Path: []string{emoji}, Entries: []Entry{{Key: "k", Value: []byte("v")}}},
		{Path: []string{"G"}, Entries: []Entry{
			{Key: replacement, Value: []byte("v")},
			{Key: emoji, Value: []byte("v")},
		}},
	}}

	got := string(mustWrite(t, doc))
	want := "[G]\n" +
		replacement + "=v\n" +
		emoji + "=v\n" +
		"\n[" + emoji + "]\nk=v\n" +
		"\n[" + replacement + "]\nk=v\n"
	if got != want {
		t.Fatalf("Write() =\n%q\nwant\n%q", got, want)
	}
}

// TestOrder_Groups covers the ordinary comparator cases plus nesting, which
// sorts on the 0x1D-joined path rather than segment by segment.
func TestOrder_Groups(t *testing.T) {
	doc := &Doc{Groups: []Group{
		{Path: []string{"Outer", "b"}},
		{Path: []string{"Outer"}},
		{Path: []string{"outer"}},
		{Path: []string{"Outer", "a"}},
		{Path: []string{"$foo"}},
		{Path: []string{"Alpha"}},
	}}
	for i := range doc.Groups {
		doc.Groups[i].Entries = []Entry{{Key: "k", Value: []byte("v")}}
	}

	got := string(mustWrite(t, doc))
	want := "[\\x24foo]\nk=v\n" +
		"\n[Alpha]\nk=v\n" +
		"\n[Outer]\nk=v\n" +
		"\n[Outer][a]\nk=v\n" +
		"\n[Outer][b]\nk=v\n" +
		"\n[outer]\nk=v\n"
	if got != want {
		t.Fatalf("Write() =\n%q\nwant\n%q", got, want)
	}
}

// TestOrder_KeysAndLocales pins the intra-group order: unsigned UTF-8 byte
// comparison of the decoded bare key, tie-broken by locale with "" first.
func TestOrder_KeysAndLocales(t *testing.T) {
	doc := &Doc{Groups: []Group{{Path: []string{"G"}, Entries: []Entry{
		{Key: "Name", Locale: "fr", Value: []byte("francais")},
		{Key: "Name", Locale: "de_DE", Value: []byte("deutschland")},
		{Key: "Name", Value: []byte("base")},
		{Key: "Name", Locale: "de", Value: []byte("deutsch")},
		{Key: "trail ", Value: []byte("t")},
		{Key: "Tab\tKey", Value: []byte("t")},
		{Key: "   ", Value: []byte("s")},
		{Key: " lead", Value: []byte("l")},
	}}}}

	got := string(mustWrite(t, doc))
	want := "[G]\n" +
		"\\s \\s=s\n" +
		"\\slead=l\n" +
		"Name=base\n" +
		"Name[de]=deutsch\n" +
		"Name[de_DE]=deutschland\n" +
		"Name[fr]=francais\n" +
		"Tab\\tKey=t\n" +
		"trail\\s=t\n"
	if got != want {
		t.Fatalf("Write() =\n%q\nwant\n%q", got, want)
	}
}

// TestLayout_BlankLines pins §4.4 item 5: one LF immediately before each group
// header except the first thing written, no leading blank line, no trailing
// blank line, and a single terminating LF.
func TestLayout_BlankLines(t *testing.T) {
	cases := []struct {
		name string
		doc  *Doc
		want string
	}{
		{
			name: "empty document",
			doc:  &Doc{},
			want: "",
		},
		{
			name: "root group only, no header line",
			doc: &Doc{Groups: []Group{{Entries: []Entry{
				{Key: "rootkey", Value: []byte("rootvalue")},
				{Key: "another", Value: []byte("x")},
			}}}},
			want: "another=x\nrootkey=rootvalue\n",
		},
		{
			name: "root group then groups",
			doc: &Doc{Groups: []Group{
				{Entries: []Entry{{Key: "rootkey", Value: []byte("v")}}},
				{Path: []string{"Zed"}, Entries: []Entry{{Key: "k", Value: []byte("v")}}},
				{Path: []string{"Alpha"}, Entries: []Entry{{Key: "k", Value: []byte("v")}}},
			}},
			want: "rootkey=v\n\n[Alpha]\nk=v\n\n[Zed]\nk=v\n",
		},
		{
			name: "empty root group contributes nothing",
			doc: &Doc{Groups: []Group{
				{},
				{Path: []string{"Alpha"}, Entries: []Entry{{Key: "k", Value: []byte("v")}}},
			}},
			want: "[Alpha]\nk=v\n",
		},
		{
			name: "a group with no entries keeps its header",
			doc: &Doc{Groups: []Group{
				{Path: []string{"Empty"}},
				{Path: []string{"Full"}, Entries: []Entry{{Key: "k", Value: []byte("v")}}},
			}},
			want: "[Empty]\n\n[Full]\nk=v\n",
		},
		{
			name: "tombstone omits the equals sign",
			doc: &Doc{Groups: []Group{{Path: []string{"G"}, Entries: []Entry{
				{Key: "Gone", RawFlags: "d", Deleted: true},
				{Key: "Here", Value: []byte("1")},
			}}}},
			want: "[G]\nGone[$d]\nHere=1\n",
		},
		{
			name: "group immutability propagates onto entries",
			doc: &Doc{Groups: []Group{{Path: []string{"Immutable"}, Immutable: true, Entries: []Entry{
				{Key: "a", Value: []byte("1")},
				{Key: "b", Value: []byte("2")},
			}}}},
			want: "[Immutable][$i]\na[$i]=1\nb[$i]=2\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustWrite(t, tc.doc)
			if !bytes.Equal(got, []byte(tc.want)) {
				t.Fatalf("Write() = %q, want %q", got, tc.want)
			}
		})
	}
}
