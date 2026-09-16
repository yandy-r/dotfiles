package plasma

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Write serialises doc into KConfig's canonical form.
//
// Output is a pure function of the document: the previous file's byte layout,
// comment lines, blank lines and key ordering contribute nothing, because
// KConfigIniBackend::writeConfig() re-parses and re-serialises from scratch and
// therefore destroys all of them on the first write anyway.
//
// Layout (§4.4):
//
//  1. The root group first, with no header line, then every other group.
//  2. Groups ascend by their 0x1D-joined path compared as UTF-16 CODE UNITS —
//     not UTF-8 bytes; see compareUTF16.
//  3. Keys ascend within a group by UTF-8 BYTE comparison of the decoded bare
//     key, tie-broken by locale with "" first. This is deliberately a DIFFERENT
//     comparator from the group order.
//  4. "key[locale][$flags]=value\n" with no spaces around '=', no indentation
//     and no CR. A tombstone omits both the '=' and the value.
//  5. Exactly one LF immediately before each group header except before the
//     very first thing written, so there is no leading and no trailing blank
//     line and the file ends with a single LF.
//
// Write fails rather than emitting a document that would not read back the same
// way: a path with no non-empty segment (lone ""), a segment holding the 0x1D
// joiner, an empty key, a locale tag that is not already canonical, a duplicate
// group path, or a duplicate (key, locale) within a group. Nested empty
// segments (as in KWin's [Tiling][<uuid>][]) are preserved as "[]" so apply's
// live-preserving merge can round-trip them.
//
// Deliberate divergences from KDE's own writer, each verified against
// kwriteconfig6 6.29 and each covered by a test that documents it:
//
//   - A leading '#' in a key is escaped as \x23 (see appendEscaped). KDE writes
//     it raw and then swallows the line as a comment on the next read.
//   - A group segment that carries an escape AND an edge space writes the space
//     as \s (see escapeEdgeSpaceIn). KDE writes it raw and then trims it back
//     off on the next read, which renames the group — and merges it into the
//     group of the same trimmed name if one exists.
//   - The key of a LOCALIZED entry is escaped the same way as any other key.
//     KDE writes such a key completely unescaped, so "a\x01b[de]" comes back
//     with a raw control byte and an edge space is lost outright.
//   - Unknown flag letters are preserved (see canonicalFlags). KDE discards
//     them.
//   - A group with no entries keeps its header. KDE drops such a group, which
//     also drops a group-level [$i] the user put there on purpose.
//   - Locale tags are never re-stamped. KDE rewrites a matching tag from the
//     PROCESS locale, and under LANG=de_DE a single write to a file holding
//     Name / Name[de] / Name[de_DE] / Name[fr] silently DELETES Name[de].
//   - No BOM is ever emitted (and one is rejected on read).
func Write(doc *Doc) ([]byte, error) {
	if doc == nil {
		return nil, errors.New("plasma: cannot serialise a nil document")
	}

	root, others, err := partitionGroups(doc)
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, 4096)
	wrote := false

	if doc.Immutable {
		out = append(out, '[', '$', 'i', ']', '\n')
		wrote = true
	}
	if root != nil {
		if out, err = appendEntries(out, root); err != nil {
			return nil, err
		}
		wrote = wrote || len(root.Entries) > 0
	}
	for _, g := range others {
		if wrote {
			out = append(out, '\n')
		}
		out = appendHeader(out, g)
		wrote = true
		if out, err = appendEntries(out, g); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// partitionGroups validates every group path and splits the document into its
// root group (if any) and the remaining groups in canonical order.
func partitionGroups(doc *Doc) (*Group, []*Group, error) {
	type keyed struct {
		g     *Group
		units []uint16
		name  string
	}

	var root *Group
	rest := make([]keyed, 0, len(doc.Groups))
	seen := make(map[string]struct{}, len(doc.Groups))

	for i := range doc.Groups {
		g := &doc.Groups[i]
		if len(g.Path) > 0 && !hasNonEmptySegment(g.Path) {
			// Path {""} (and {"",""}, …) would Join to "" and collide with the
			// root group. Nested empties under a named parent are fine.
			return nil, nil, fmt.Errorf("plasma: group %q has an empty path segment", strings.Join(g.Path, groupSeparator))
		}
		for _, seg := range g.Path {
			if strings.Contains(seg, groupSeparator) {
				// 0x1D is the joiner itself. Emitting it as \x1d would produce a
				// header KDE re-splits into extra segments, so the name we wrote
				// and the name KDE reads would not be the same one — and two
				// different Paths would share one joined index key.
				return nil, nil, fmt.Errorf("plasma: group segment %q contains the 0x1D group separator, which KConfig cannot represent inside one segment", seg)
			}
		}
		name := strings.Join(g.Path, groupSeparator)
		if _, dup := seen[name]; dup {
			return nil, nil, fmt.Errorf("plasma: duplicate group %q", name)
		}
		seen[name] = struct{}{}

		if name == "" {
			if g.Immutable {
				// There is no header line to carry the marker on, and the
				// file-level marker is Doc.Immutable's job.
				return nil, nil, errors.New("plasma: the root group cannot be immutable; set Doc.Immutable instead")
			}
			root = g
			continue
		}
		rest = append(rest, keyed{g: g, units: utf16Units(name), name: name})
	}

	sort.Slice(rest, func(i, j int) bool {
		if c := compareUTF16Units(rest[i].units, rest[j].units); c != 0 {
			return c < 0
		}
		// Two distinct names can share a code-unit sequence when one holds
		// invalid UTF-8 that decodes to U+FFFD. Fall back to bytes so the sort
		// stays a total order.
		return rest[i].name < rest[j].name
	})

	groups := make([]*Group, len(rest))
	for i := range rest {
		groups[i] = rest[i].g
	}
	return root, groups, nil
}

// appendHeader writes "[seg][seg]…" plus an optional "[$i]" marker and the
// terminating LF. Only the LAST segment is eligible for the leading-'$' escape.
func appendHeader(out []byte, g *Group) []byte {
	last := len(g.Path) - 1
	for i, seg := range g.Path {
		out = append(out, '[')
		out = appendEscapedString(out, seg, ctxGroup, i == last)
		out = append(out, ']')
	}
	if g.Immutable {
		out = append(out, '[', '$', 'i', ']')
	}
	return append(out, '\n')
}

// appendEntries writes a group's entries in canonical key order.
func appendEntries(out []byte, g *Group) ([]byte, error) {
	ordered, err := orderEntries(g)
	if err != nil {
		return nil, err
	}
	for _, e := range ordered {
		out = appendEscapedString(out, e.Key, ctxKey, false)
		if e.Locale != "" {
			out = append(out, '[')
			out = appendEscapedString(out, e.Locale, ctxLocale, false)
			out = append(out, ']')
		}
		flags := canonicalFlags(e.RawFlags, g.Immutable, e.Deleted)
		if flags != "" {
			out = append(out, '[', '$')
			out = append(out, flags...)
			out = append(out, ']')
		}
		if strings.IndexByte(flags, 'd') < 0 {
			out = append(out, '=')
			out = appendEscaped(out, e.Value, ctxValue, false)
		}
		out = append(out, '\n')
	}
	return out, nil
}

// orderEntries returns a group's entries sorted by decoded key as unsigned
// UTF-8 bytes, tie-broken by locale with "" first. It rejects an empty key and
// a duplicate (key, locale) identity, either of which would not survive a
// round trip.
func orderEntries(g *Group) ([]Entry, error) {
	ordered := make([]Entry, len(g.Entries))
	copy(ordered, g.Entries)

	seen := make(map[string]struct{}, len(ordered))
	for _, e := range ordered {
		if e.Key == "" {
			return nil, fmt.Errorf("plasma: group %q has an entry with an empty key", strings.Join(g.Path, groupSeparator))
		}
		if e.Locale != "" && !localeIsCanonical([]byte(e.Locale)) {
			// KDE matches a locale tag as raw bytes, so a tag that needs
			// escaping cannot be written without changing which entry it
			// serves. Parse refuses to produce one; refuse to emit one too.
			return nil, fmt.Errorf("plasma: entry %q in group %q has a locale tag %q that KConfig cannot represent", e.Key, strings.Join(g.Path, groupSeparator), e.Locale)
		}
		id := e.Key + groupSeparator + e.Locale
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("plasma: duplicate key %q (locale %q) in group %q", e.Key, e.Locale, strings.Join(g.Path, groupSeparator))
		}
		seen[id] = struct{}{}
	}

	sort.Slice(ordered, func(i, j int) bool {
		if c := strings.Compare(ordered[i].Key, ordered[j].Key); c != 0 {
			return c < 0
		}
		return ordered[i].Locale < ordered[j].Locale
	})
	return ordered, nil
}

// canonicalFlags normalises an entry's flag letters into KConfig's write order.
//
// Verified against kwriteconfig6 6.29: [$e][$i] and [$ei] both come back as
// [$ie]; [$ii] collapses to [$i]; and 'd' suppresses the entry's OWN other
// letters, so [$di], [$id], [$de] and [$ed] all come back as [$d].
// groupImmutable folds a group-header [$i] onto every entry, which is what KDE
// does on the first rewrite of such a group.
//
// The one asymmetry, and it is KDE's: the PROPAGATED immutability is not
// suppressed by 'd' the way the entry's own 'i' is, and it is written AFTER it.
// Inside "[G][$i]", every one of a[$d], a[$di], a[$de] and a bare a[$d] tombstone
// comes back as a[$di]. Emitting [$d] there instead is semantically identical —
// the header still carries the marker, so KDE re-derives the flag either way —
// but it makes the file a fixed point of our writer and NOT of KDE's, which is
// a live config rewritten on every single run once the session touches it.
//
// The one divergence: KDE DISCARDS unknown flag letters ([$q] comes back as no
// suffix at all). Discarding user data is not acceptable here, so unknown
// letters are preserved, de-duplicated, appended after the known ones in
// first-seen order. That is stable under re-parse, so it costs one
// normalisation pass and nothing after.
func canonicalFlags(raw string, groupImmutable, deleted bool) string {
	var hasI, hasD, hasE bool
	var unknown []byte
	seen := make(map[byte]struct{})

	for i := 0; i < len(raw); i++ {
		switch c := raw[i]; c {
		case 'i':
			hasI = true
		case 'd':
			hasD = true
		case 'e':
			hasE = true
		default:
			if _, dup := seen[c]; !dup {
				seen[c] = struct{}{}
				unknown = append(unknown, c)
			}
		}
	}
	hasD = hasD || deleted

	out := make([]byte, 0, 3+len(unknown))
	if hasD {
		out = append(out, 'd')
		if groupImmutable {
			out = append(out, 'i')
		}
	} else {
		if hasI || groupImmutable {
			out = append(out, 'i')
		}
		if hasE {
			out = append(out, 'e')
		}
	}
	return string(append(out, unknown...))
}

// utf16Units re-encodes s as the UTF-16 code units QString would hold. Invalid
// UTF-8 decodes to U+FFFD, exactly as QString::fromUtf8 substitutes it.
func utf16Units(s string) []uint16 {
	units := make([]uint16, 0, len(s))
	for _, r := range s {
		if r < 0x10000 {
			units = append(units, uint16(r))
			continue
		}
		r -= 0x10000
		units = append(units, uint16(0xD800+(r>>10)), uint16(0xDC00+(r&0x3FF)))
	}
	return units
}

// compareUTF16Units orders two code-unit sequences the way QString::operator<
// does. This is NOT UTF-8 byte order and sort.Strings is wrong for group names:
// a group named U+1F600 sorts BEFORE one named U+FFFD, because the leading
// surrogate 0xD83D is less than 0xFFFD — the exact opposite of the byte order
// that governs KEY names in the same file.
func compareUTF16Units(a, b []uint16) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}

// compareUTF16 orders two group names as UTF-16 code-unit sequences. It is the
// exported-behaviour twin of the comparator partitionGroups uses; callers that
// need to sort a handful of names without pre-computing units use this.
func compareUTF16(a, b string) int {
	if c := compareUTF16Units(utf16Units(a), utf16Units(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}
