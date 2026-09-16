// Package plasma manages a curated subset of KDE Plasma 6 configuration as
// repo-tracked data, seeded from the running desktop.
//
// It reproduces KConfigIniBackend's INI dialect natively — parse, canonical
// serialise, and per-key merge — as a PlanExport/PlanApply/Execute engine
// driven by a static, typed file table (DefaultFiles) with a governed volatile
// strip list. Every managed destination lives under the user's KDE config
// directory, so the engine uses pure os.* calls and needs no privilege
// escalation.
//
// The engine takes repoRoot (the dotfiles source tree) and configDir (the
// resolved $XDG_CONFIG_HOME) as constructor parameters, keeping it free of
// environment lookups so tests can drive it hermetically against t.TempDir().
// The injected exec.Runner is used ONLY for the optional live reload; all INI
// work is native Go.
package plasma

import (
	"bytes"
	"fmt"
	"strings"
)

// Doc is a parsed KConfig INI document.
type Doc struct {
	// Immutable records a bare "[$i]" line, KConfig's file-level immutability
	// marker. KDE refuses to write such a file at all, so it never appears in
	// canonical output; it is modelled here so that reading one and writing it
	// back does not silently drop the marker and unlock the file.
	Immutable bool
	// Groups holds every group in first-appearance order. Serialisation sorts
	// them, so this order is informational only.
	Groups []Group
}

// Group is one configuration group. Path holds the DECODED bracket segments;
// an empty Path is KConfig's "<default>" root group, whose entries are written
// with no header line at the top of the file.
type Group struct {
	// Path is the decoded bracket segments, outermost first. nil or empty means
	// the root group. A nested empty string is a real segment (KWin writes
	// [Tiling][<uuid>][] with a trailing ""); a path of only empty strings is
	// rejected on parse and write.
	Path []string
	// Immutable records a trailing [$i] marker on the header. On write it also
	// propagates an 'i' flag onto every entry of the group, which is what KDE
	// does on the first rewrite of such a file.
	Immutable bool
	// Entries holds the group's key=value lines in first-appearance order.
	// Serialisation sorts them, so this order is informational only.
	Entries []Entry
}

// Entry is one key=value line. Identity is (Group.Path, Key, Locale); RawFlags
// and Value are attributes of that identity.
type Entry struct {
	// Key is the decoded bare key, flag and locale suffixes removed.
	Key string
	// Locale is the locale tag verbatim ("" = none), which is also its on-disk
	// form: KDE matches a tag as raw bytes and never decodes one, so Parse and
	// Write both refuse a tag that is not already canonical (localeIsCanonical).
	// It is part of the entry's identity: KDE re-stamps a matching locale from
	// the PROCESS locale and can delete a variant as a side effect, which this
	// codec never does.
	Locale string
	// RawFlags is the verbatim inner flag text WITHOUT the '$' ("ie", "d",
	// "xyz"); "" = none. It is kept verbatim rather than parsed into an i/e/d
	// enum because KDE DROPS unknown flag letters on rewrite (a[$q]=1 comes
	// back as a=1) and this codec must not reproduce that loss.
	RawFlags string
	// Value is the decoded value bytes; it may not be valid UTF-8. It is
	// []byte rather than string because a high byte outside well-formed UTF-8
	// is escaped in a value but written raw in a key or group name — modelling
	// all three as string and re-encoding uniformly corrupts one of them.
	Value []byte
	// Deleted is true when RawFlags contains 'd' — a tombstone. Such an entry
	// reads as absent but stays in the file, and the writer strips its '='.
	Deleted bool
	// HasValue records whether an '=' was present on the line. "Key=" with an
	// empty value is a real entry, not an absent one.
	HasValue bool
}

// ParseError reports a KConfig file this codec refuses to load. Line is
// 1-based.
//
// KDE's own parser is permissive and silently lossy — it drops malformed
// entries, misfiles entries after an unterminated header, and only aborts after
// 100 bad escapes. Because plasma/files/ is hand-editable, a silent drop here
// is a settings loss the user never sees, so every one of those conditions is
// an error instead (§4.6).
type ParseError struct {
	// Path is the file the error was found in, for diagnostics only.
	Path string
	// Line is the 1-based line number the error was found on.
	Line int
	// Msg describes the violation.
	Msg string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("plasma: %s:%d: %s", e.Path, e.Line, e.Msg)
}

// utf8BOM is rejected on read: KDE keys off the first byte of a line being '[',
// so a BOM makes it misroute every entry in the file into the root group.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// asciiSpace is the set KConfig trims from a line and from the key and value
// slices. A literal TAB, CR or LF therefore never survives unescaped on disk.
const asciiSpace = " \t\r\n\v\f"

// immutableMarker is the exact inner text of the [$i] bracket group that marks
// a group — or, when it is the only bracket group on the line, the whole file —
// immutable.
const immutableMarker = "$i"

// Parse reads a KConfig INI document. path is used only for diagnostics.
//
// Duplicate (group, key, locale) triples and re-opened group headers are NOT
// errors: the last occurrence wins and group fragments coalesce, matching KDE.
func Parse(path string, data []byte) (*Doc, error) {
	if bytes.HasPrefix(data, utf8BOM) {
		return nil, &ParseError{Path: path, Line: 1, Msg: "UTF-8 BOM: KConfig would misfile every entry into the root group"}
	}

	doc := &Doc{}
	index := make(map[string]int) // 0x1D-joined path -> index into doc.Groups
	current := -1                 // index of the group entries are landing in

	for i, raw := range bytes.Split(data, []byte("\n")) {
		lineNo := i + 1
		line := bytes.Trim(raw, asciiSpace)
		if len(line) == 0 {
			continue
		}
		if line[0] == '#' {
			// KConfig has no comment model: comments are skipped on read and
			// never round-trip. Rejecting them would make plasma/files/
			// un-annotatable, so they are dropped rather than refused.
			continue
		}
		if line[0] == '[' {
			segs, immutable, err := parseHeader(path, lineNo, line)
			if err != nil {
				return nil, err
			}
			if len(segs) == 0 {
				// A bare "[$i]": the file, not a group, is immutable. The
				// current group is unchanged.
				doc.Immutable = true
				continue
			}
			current = openGroup(doc, index, segs, immutable)
			continue
		}
		entry, err := parseEntry(path, lineNo, line)
		if err != nil {
			return nil, err
		}
		if current < 0 {
			current = openGroup(doc, index, nil, false)
		}
		putEntry(&doc.Groups[current], entry)
	}
	return doc, nil
}

// openGroup returns the index of the group with the given decoded path,
// creating it if it does not exist yet. Re-opened headers coalesce into the
// existing group, and an immutability marker on any fragment marks the whole
// group, matching KDE's map-based reader.
func openGroup(doc *Doc, index map[string]int, segs []string, immutable bool) int {
	key := strings.Join(segs, groupSeparator)
	if at, ok := index[key]; ok {
		if immutable {
			doc.Groups[at].Immutable = true
		}
		return at
	}
	doc.Groups = append(doc.Groups, Group{Path: segs, Immutable: immutable})
	at := len(doc.Groups) - 1
	index[key] = at
	return at
}

// putEntry appends an entry, or overwrites the existing one with the same
// (Key, Locale) identity. Last occurrence wins, matching KDE.
func putEntry(g *Group, e Entry) {
	for i := range g.Entries {
		if g.Entries[i].Key == e.Key && g.Entries[i].Locale == e.Locale {
			g.Entries[i] = e
			return
		}
	}
	g.Entries = append(g.Entries, e)
}

// parseHeader splits a group-header line into its decoded bracket segments and
// an immutability flag. An empty segment slice with immutable=true is the
// file-level "[$i]" marker.
//
// The continuation test is strict: the byte immediately after a ']' must be '['
// with no whitespace between, and nothing may follow the final ']'. KDE ignores
// such junk silently; we reject it, because "[General] # note" reads as a
// perfectly good header to a human and as something else to KDE.
func parseHeader(path string, lineNo int, line []byte) ([]string, bool, error) {
	var raws [][]byte
	pos := 0
	for pos < len(line) && line[pos] == '[' {
		rel := bytes.IndexByte(line[pos+1:], ']')
		if rel < 0 {
			return nil, false, &ParseError{Path: path, Line: lineNo, Msg: "unterminated group header: missing ']'"}
		}
		raws = append(raws, line[pos+1:pos+1+rel])
		pos += rel + 2
	}
	if pos != len(line) {
		return nil, false, &ParseError{Path: path, Line: lineNo, Msg: fmt.Sprintf("trailing junk %q after group header", string(line[pos:]))}
	}

	immutable := false
	if len(raws) > 0 && string(raws[len(raws)-1]) == immutableMarker {
		immutable = true
		raws = raws[:len(raws)-1]
	}
	if len(raws) == 0 {
		if !immutable {
			return nil, false, &ParseError{Path: path, Line: lineNo, Msg: "empty group header []: the resulting group is unaddressable"}
		}
		return nil, true, nil
	}

	segs := make([]string, 0, len(raws))
	for _, r := range raws {
		// A raw empty bracket pair ("[]") is a real nested segment: KWin writes
		// headers like [Tiling][<desktop-uuid>][] with an empty trailing
		// sentinel. Map it to "" so strip/export/diff can address the group;
		// a header with no non-empty segment is still refused below.
		if len(r) == 0 {
			segs = append(segs, "")
			continue
		}
		dec, err := decodeGroupSegment(r)
		if err != nil {
			return nil, false, &ParseError{Path: path, Line: lineNo, Msg: fmt.Sprintf("group header: %v", err)}
		}
		// A DECODED 0x1D is KConfig's own segment joiner, so KDE splits the
		// segment on the spot: kreadconfig6 answers "[a\x1db]" for both
		// --group a --group b and --group $'a\x1db', and kwriteconfig6 rewrites
		// the header as "[a][b]". Splitting here gives one group one spelling;
		// leaving it whole would let two different Paths collide on the same
		// 0x1D-joined index key.
		for _, part := range bytes.Split(dec, []byte(groupSeparator)) {
			if len(part) == 0 {
				// Either the raw segment decoded to nothing at all — "[ \ ]"
				// and "[\]" do, the trim eats the spaces and the lone trailing
				// backslash is dropped — or a 0x1D left an empty piece behind.
				// KDE files such a group's entries into the ROOT group; this is
				// the loud version of that.
				return nil, false, &ParseError{Path: path, Line: lineNo, Msg: fmt.Sprintf("group header segment %q decodes to an empty segment: the resulting group is unaddressable", string(r))}
			}
			segs = append(segs, string(part))
		}
	}
	if !hasNonEmptySegment(segs) {
		// Lone "[]" (and "[][]", …) — no parent name to hang the group on.
		return nil, false, &ParseError{Path: path, Line: lineNo, Msg: "empty group header []: the resulting group is unaddressable"}
	}
	return segs, immutable, nil
}

// hasNonEmptySegment reports whether path has at least one non-empty segment.
// A raw "[]" nested under a named parent is addressable; a header made only of
// empty brackets is not.
func hasNonEmptySegment(segs []string) bool {
	for _, s := range segs {
		if s != "" {
			return true
		}
	}
	return false
}

// parseEntry splits one "key[locale][$flags]=value" line into an Entry.
//
// The value split is on the FIRST '=' in the line, so "a=b=c=d" yields the
// value "b=c=d"; a key that genuinely contains '=' carries it as \x3d.
func parseEntry(path string, lineNo int, line []byte) (Entry, error) {
	fail := func(msg string) (Entry, error) {
		return Entry{}, &ParseError{Path: path, Line: lineNo, Msg: msg}
	}

	keyPart := line
	var valuePart []byte
	hasValue := false
	if eq := bytes.IndexByte(line, '='); eq >= 0 {
		keyPart = line[:eq]
		valuePart = bytes.Trim(line[eq+1:], asciiSpace)
		hasValue = true
	}
	keyPart = bytes.Trim(keyPart, asciiSpace)

	bare := keyPart
	var flags strings.Builder
	locale := ""
	haveLocale := false
	if at := bytes.IndexByte(keyPart, '['); at >= 0 {
		bare = keyPart[:at]
		pos := at
		for pos < len(keyPart) && keyPart[pos] == '[' {
			rel := bytes.IndexByte(keyPart[pos+1:], ']')
			if rel < 0 {
				return fail("invalid entry: key suffix is missing ']'")
			}
			inner := keyPart[pos+1 : pos+1+rel]
			pos += rel + 2
			switch {
			case len(inner) == 0:
				return fail("invalid entry: empty key suffix []")
			case inner[0] == '$':
				flags.Write(inner[1:])
			case haveLocale:
				return fail("invalid entry: second locale suffix")
			default:
				if !localeIsCanonical(inner) {
					return fail(fmt.Sprintf("invalid entry: locale suffix %q is not in canonical form: KDE matches a locale tag as RAW bytes, so re-encoding it would change which entry the tag serves", string(inner)))
				}
				locale = string(inner)
				haveLocale = true
			}
		}
		if pos != len(keyPart) {
			// An unescaped '[' inside the key itself lands here: KDE drops the
			// whole line with "Invalid entry (missing ']')".
			return fail("invalid entry: unescaped '[' in key (escape it as \\x5b)")
		}
	}

	if len(bare) == 0 {
		return fail("invalid entry: empty key")
	}
	key, err := unescape(bare)
	if err != nil {
		return fail(fmt.Sprintf("invalid entry: key: %v", err))
	}
	if len(key) == 0 {
		// The raw key was non-empty but decoded to nothing — a lone backslash
		// does, because it is dropped rather than kept. KDE drops the whole
		// entry there, and for "\[de]=v" it goes further and re-reads the line
		// as the group header "[de]", misfiling every entry that follows it.
		return fail(fmt.Sprintf("invalid entry: key %q decodes to an empty key", string(bare)))
	}

	rawFlags := flags.String()
	deleted := strings.IndexByte(rawFlags, 'd') >= 0
	if !hasValue && !deleted {
		return fail("invalid entry: missing '='")
	}

	var value []byte
	if hasValue {
		if value, err = unescape(valuePart); err != nil {
			return fail(fmt.Sprintf("invalid entry: value: %v", err))
		}
	}

	return Entry{
		Key:      string(key),
		Locale:   locale,
		RawFlags: rawFlags,
		Value:    value,
		Deleted:  deleted,
		HasValue: hasValue,
	}, nil
}
