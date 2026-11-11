package plasma

import (
	"errors"
	"strings"
	"testing"
)

// TestParse_FailLoud walks the §4.6 table. Every one of these is something
// KDE's parser accepts and silently mangles — a dropped line, a misfiled entry,
// a value that keeps its raw escape text. Because plasma/files/ is
// hand-editable, a silent drop is a settings loss the user never sees, so the
// codec refuses the file and names the line instead.
func TestParse_FailLoud(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantLine int
		wantMsg  string // substring
	}{
		{
			name:     "leading UTF-8 BOM",
			raw:      "\xef\xbb\xbf[General]\na=1\n",
			wantLine: 1,
			wantMsg:  "BOM",
		},
		{
			name:     "unterminated group header",
			raw:      "[General]\na=1\n[Broken\nb=2\n",
			wantLine: 3,
			wantMsg:  "unterminated group header",
		},
		{
			name:     "empty group header",
			raw:      "[]\na=1\n",
			wantLine: 1,
			wantMsg:  "empty group header",
		},
		{
			// Nested empties with no named parent are still unaddressable.
			name:     "only empty nested segments",
			raw:      "[][]\na=1\n",
			wantLine: 1,
			wantMsg:  "empty group header",
		},
		{
			name:     "trailing junk after the final bracket",
			raw:      "[General] # note\na=1\n",
			wantLine: 1,
			wantMsg:  "trailing junk",
		},
		{
			name:     "gap between bracket groups",
			raw:      "[A] [B]\na=1\n",
			wantLine: 1,
			wantMsg:  "trailing junk",
		},
		{
			name:     "invalid escape in a group header",
			raw:      "[Bad\\zName]\na=1\n",
			wantLine: 1,
			wantMsg:  "invalid escape",
		},
		{
			name:     "unescaped bracket in a key",
			raw:      "[G]\nBr[ack]et=v\n",
			wantLine: 2,
			wantMsg:  "unescaped '['",
		},
		{
			name:     "key suffix missing its closing bracket",
			raw:      "[G]\nName[de=v\n",
			wantLine: 2,
			wantMsg:  "missing ']'",
		},
		{
			name:     "entry with no equals and no tombstone",
			raw:      "[G]\nLonelyKey\n",
			wantLine: 2,
			wantMsg:  "missing '='",
		},
		{
			name:     "second locale suffix",
			raw:      "[G]\nName[de][fr]=v\n",
			wantLine: 2,
			wantMsg:  "second locale",
		},
		{
			name:     "empty key suffix",
			raw:      "[G]\nName[]=v\n",
			wantLine: 2,
			wantMsg:  "empty key suffix",
		},
		{
			name:     "empty key",
			raw:      "[G]\n=orphan\n",
			wantLine: 2,
			wantMsg:  "empty key",
		},
		{
			name:     "invalid escape in a value",
			raw:      "[G]\na=bad\\zvalue\n",
			wantLine: 2,
			wantMsg:  "invalid escape",
		},
		{
			name:     "octal-style escape in a value",
			raw:      "[G]\na=bad\\101value\n",
			wantLine: 2,
			wantMsg:  "invalid escape",
		},
		{
			name:     "uppercase hex escape prefix in a value",
			raw:      "[G]\na=bad\\X41value\n",
			wantLine: 2,
			wantMsg:  "invalid escape",
		},
		{
			name:     "invalid escape in a key",
			raw:      "[G]\nba\\zd=v\n",
			wantLine: 2,
			wantMsg:  "invalid escape",
		},
		{
			// A locale tag is never decoded by KDE — it is matched as raw bytes
			// — so ANY tag that is not already canonical is refused, whether the
			// escape it carries is well formed or not.
			name:     "escape sequence in a locale tag",
			raw:      "[G]\nName[d\\ze]=v\n",
			wantLine: 2,
			wantMsg:  "not in canonical form",
		},
		{
			name:     "well-formed escape in a locale tag is refused too",
			raw:      "[G]\nName[d\\x65]=v\n",
			wantLine: 2,
			wantMsg:  "not in canonical form",
		},
		{
			name:     "locale tag with edge whitespace",
			raw:      "[G]\nName[ de ]=v\n",
			wantLine: 2,
			wantMsg:  "not in canonical form",
		},
		{
			// "[ \ ]" trims to "\", the lone trailing backslash is dropped, and
			// the segment is gone. KDE files such a group's entries into the
			// ROOT group; we refuse rather than silently move them.
			name:     "group segment that decodes to nothing",
			raw:      "[ \\ ]\nk=1\n",
			wantLine: 1,
			wantMsg:  "decodes to an empty segment",
		},
		{
			name:     "group segment that is a lone backslash",
			raw:      "[\\]\nk=1\n",
			wantLine: 1,
			wantMsg:  "decodes to an empty segment",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := Parse("fixture", []byte(tc.raw))
			if err == nil {
				t.Fatalf("Parse() error = nil, want a *ParseError; got doc = %+v", doc)
			}
			if doc != nil {
				t.Errorf("Parse() returned a document alongside an error; want nil")
			}
			var perr *ParseError
			if !errors.As(err, &perr) {
				t.Fatalf("Parse() error = %v (%T), want *ParseError", err, err)
			}
			if perr.Line != tc.wantLine {
				t.Errorf("ParseError.Line = %d, want %d (msg %q)", perr.Line, tc.wantLine, perr.Msg)
			}
			if !strings.Contains(perr.Msg, tc.wantMsg) {
				t.Errorf("ParseError.Msg = %q, want it to contain %q", perr.Msg, tc.wantMsg)
			}
		})
	}
}

// TestParse_NotErrors is the other half of §4.6: these look malformed but are
// exactly what KDE produces, so accepting them is mandatory.
func TestParse_NotErrors(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		verify func(t *testing.T, doc *Doc)
	}{
		{
			name: "duplicate key: last occurrence wins",
			raw:  "[G]\na=first\na=second\n",
			verify: func(t *testing.T, doc *Doc) {
				t.Helper()
				if n := len(doc.Groups[0].Entries); n != 1 {
					t.Fatalf("entries = %d, want 1", n)
				}
				if got := string(doc.Groups[0].Entries[0].Value); got != "second" {
					t.Errorf("value = %q, want %q", got, "second")
				}
			},
		},
		{
			name: "same key, different locale: both survive",
			raw:  "[G]\na=base\na[de]=deutsch\n",
			verify: func(t *testing.T, doc *Doc) {
				t.Helper()
				if n := len(doc.Groups[0].Entries); n != 2 {
					t.Fatalf("entries = %d, want 2", n)
				}
			},
		},
		{
			name: "re-opened group header coalesces",
			raw:  "[G]\na=1\n[H]\nb=2\n[G]\nc=3\n",
			verify: func(t *testing.T, doc *Doc) {
				t.Helper()
				if n := len(doc.Groups); n != 2 {
					t.Fatalf("groups = %d, want 2", n)
				}
				if n := len(doc.Groups[0].Entries); n != 2 {
					t.Errorf("group G entries = %d, want 2", n)
				}
			},
		},
		{
			name: "immutability on any fragment marks the whole group",
			raw:  "[G]\na=1\n[G][$i]\nb=2\n",
			verify: func(t *testing.T, doc *Doc) {
				t.Helper()
				if !doc.Groups[0].Immutable {
					t.Error("Immutable = false, want true")
				}
			},
		},
		{
			name: "empty value is a real entry",
			raw:  "[G]\nEnable=\n",
			verify: func(t *testing.T, doc *Doc) {
				t.Helper()
				e := doc.Groups[0].Entries[0]
				if len(e.Value) != 0 {
					t.Errorf("Value = %q, want empty", e.Value)
				}
			},
		},
		{
			name: "comment and blank lines are skipped",
			raw:  "# note\n\n[G]\n\n# another\na=1\n",
			verify: func(t *testing.T, doc *Doc) {
				t.Helper()
				if n := len(doc.Groups); n != 1 {
					t.Fatalf("groups = %d, want 1", n)
				}
				if n := len(doc.Groups[0].Entries); n != 1 {
					t.Errorf("entries = %d, want 1", n)
				}
			},
		},
		{
			name: "CRLF line endings",
			raw:  "[G]\r\na=1\r\n",
			verify: func(t *testing.T, doc *Doc) {
				t.Helper()
				if got := string(doc.Groups[0].Entries[0].Value); got != "1" {
					t.Errorf("value = %q, want %q", got, "1")
				}
			},
		},
		{
			name: "unescaped edge whitespace is trimmed away",
			raw:  "[G]\n  spaced   =   value   \n",
			verify: func(t *testing.T, doc *Doc) {
				t.Helper()
				e := doc.Groups[0].Entries[0]
				if e.Key != "spaced" {
					t.Errorf("key = %q, want %q", e.Key, "spaced")
				}
				if got := string(e.Value); got != "value" {
					t.Errorf("value = %q, want %q", got, "value")
				}
			},
		},
		{
			name: "entries before any header land in the root group",
			raw:  "rootkey=v\n[G]\na=1\n",
			verify: func(t *testing.T, doc *Doc) {
				t.Helper()
				if len(doc.Groups[0].Path) != 0 {
					t.Errorf("first group path = %v, want the root group", doc.Groups[0].Path)
				}
			},
		},
		{
			// KWin writes desktop-level tiling as [Tiling][<uuid>][] — an empty
			// trailing nested segment is a real sentinel, not junk.
			name: "empty nested segment under a named parent",
			raw:  "[A][]\na=1\n",
			verify: func(t *testing.T, doc *Doc) {
				t.Helper()
				want := []string{"A", ""}
				if got := doc.Groups[0].Path; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
					t.Errorf("path = %q, want %q", got, want)
				}
				if got := string(doc.Groups[0].Entries[0].Value); got != "1" {
					t.Errorf("value = %q, want %q", got, "1")
				}
			},
		},
		{
			name: "KWin-style empty third Tiling segment",
			raw:  "[Tiling][00000000-0000-4000-8000-000000000001][]\npadding=4\n",
			verify: func(t *testing.T, doc *Doc) {
				t.Helper()
				want := []string{"Tiling", "00000000-0000-4000-8000-000000000001", ""}
				got := doc.Groups[0].Path
				if len(got) != len(want) {
					t.Fatalf("path = %q, want %q", got, want)
				}
				for i := range want {
					if got[i] != want[i] {
						t.Fatalf("path = %q, want %q", got, want)
					}
				}
			},
		},
		{
			name: "empty file",
			raw:  "",
			verify: func(t *testing.T, doc *Doc) {
				t.Helper()
				if len(doc.Groups) != 0 {
					t.Errorf("groups = %d, want 0", len(doc.Groups))
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.verify(t, mustParse(t, "fixture", []byte(tc.raw)))
		})
	}
}
