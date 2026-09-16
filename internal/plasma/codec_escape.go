package plasma

import (
	"bytes"
	"fmt"
	"unicode/utf8"
)

// escapeContext selects which of KConfigIniBackend's escaping dialects applies
// to a byte run. The dialects are NOT interchangeable — the same byte is
// written literally in one context and escaped in another — so every call site
// must name the context explicitly:
//
//   - ctxGroup: one bracket segment of a group header. '=' is literal here
//     (KDE never splits a header on '='), but '[' and ']' must be escaped
//     because they delimit the segment.
//   - ctxKey: the bare key name. '=' must be escaped because the parser splits
//     the line on the FIRST '=' , and '[' / ']' must be escaped because they
//     introduce the locale/flag suffixes.
//   - ctxLocale: a key's locale tag. Identical to ctxKey plus a leading-'$'
//     escape, without which a locale such as "$de" would be re-read as a flag
//     suffix and the tag would be lost.
//   - ctxValue: the value after the '='. Brackets and '=' are literal here —
//     KWin stores raw JSON in `tiles=` — but a byte >= 0x7F that is not part of
//     a well-formed UTF-8 sequence is escaped per byte, and 0x7F itself always
//     is.
type escapeContext int

const (
	ctxGroup escapeContext = iota
	ctxKey
	ctxLocale
	ctxValue
)

// groupSeparator is the byte KConfig uses to join a nested group header's
// bracket segments into one canonical group name ([A][B] -> "A\x1dB"). It is
// the sort key for the group pass in Write and can never appear inside a
// segment, because a literal 0x1D is escaped as \x1d in every context.
const groupSeparator = "\x1d"

// hexDigits is deliberately lowercase: KConfig emits \xNN with lowercase hex
// digits and Family A asserts byte equality against its output.
const hexDigits = "0123456789abcdef"

// maxDollarSegment is the inclusive upper bound on the length of a group
// segment eligible for the leading-'$' escape, INCLUDING the '$' itself.
// Verified against kwriteconfig6 6.29: "$abcdefghi" (10 bytes) is written
// "\x24abcdefghi" while "$abcdefghij" (11 bytes) is written literally. The
// bound exists because KConfig only needs to disambiguate short marker-shaped
// segments such as "$i", "$e" and "$d" from real group names.
const maxDollarSegment = 10

// appendHexEscape writes b as the four bytes \xNN with lowercase hex digits.
func appendHexEscape(dst []byte, b byte) []byte {
	return append(dst, '\\', 'x', hexDigits[b>>4], hexDigits[b&0x0f])
}

// dollarSegmentNeedsEscape reports whether seg, used as the LAST bracket
// segment of a group header, must have its leading '$' written as \x24.
//
// The guard is narrow on purpose: it only fires for segments that could be
// mistaken for a KConfig marker suffix ([$i] and friends), i.e. 1..10 bytes
// long, starting with '$', with every following byte a lowercase ASCII letter.
// "$FOO", "$foo1" and "$foobarbazqux" are all written literally, which is
// exactly what kwriteconfig6 does.
func dollarSegmentNeedsEscape(seg string) bool {
	if len(seg) == 0 || len(seg) > maxDollarSegment || seg[0] != '$' {
		return false
	}
	for i := 1; i < len(seg); i++ {
		if seg[i] < 'a' || seg[i] > 'z' {
			return false
		}
	}
	return true
}

// appendEscapedString escapes s in ctx and appends it to dst. lastSegment is
// meaningful only for ctxGroup, where it enables the leading-'$' rule.
func appendEscapedString(dst []byte, s string, ctx escapeContext, lastSegment bool) []byte {
	return appendEscaped(dst, []byte(s), ctx, lastSegment)
}

// appendEscaped escapes src in ctx and appends it to dst.
//
// The per-byte rules are the §4.2 escape table of the design spec, in the same
// order. Two of them are positional rather than value-based — the leading and
// trailing single SPACE, and the leading '#' / '$' — so the loop needs the
// index, which is why this is a hand-rolled scan rather than a table lookup.
func appendEscaped(dst, src []byte, ctx escapeContext, lastSegment bool) []byte {
	return escapeInto(dst, src, ctx, lastSegment, escapeEdgeSpaceIn(src, ctx, lastSegment))
}

// escapeEdgeSpaceIn reports whether a leading or trailing SPACE in src has to
// be written as \s for the field to survive KDE's reader.
//
// Outside a group segment the answer is an unconditional yes: the reader trims
// the key, locale and value slices, so an unescaped edge space is simply lost.
//
// A group segment is the exception, and the exception is itself conditional.
// KConfigIniBackend reads a bracket segment that holds NO backslash verbatim,
// spaces and all — "[ spaced group ]" really is a group whose name carries both
// spaces — but the moment the segment holds a single escape sequence it goes
// through printableToString, which strips ASCII whitespace off both ends of the
// RAW field before decoding. Verified against kconfig 6.29: "[a\tb ]" is the
// group "a<TAB>b" to KDE, "[ ab ]" is the group " ab ", and "[ x\yz ]" is
// rewritten "[x\\yz]".
//
// Escaping the edge space only when the segment already carries an escape keeps
// our output byte-identical to kwriteconfig6 everywhere KDE is lossless, and
// diverges from it only where KDE would silently rename the group.
func escapeEdgeSpaceIn(src []byte, ctx escapeContext, lastSegment bool) bool {
	if ctx != ctxGroup {
		return true
	}
	if len(src) == 0 || (src[0] != ' ' && src[len(src)-1] != ' ') {
		return false // Nothing to protect, so skip the probe below.
	}
	// Ask the escaper itself rather than re-deriving its rule set here: if a
	// pass with the edge-space rule switched off emits a backslash anywhere,
	// KDE's reader will trim this segment and the edge space must be escaped.
	return bytes.IndexByte(escapeInto(nil, src, ctx, lastSegment, false), '\\') >= 0
}

// escapeInto is appendEscaped's loop, with the positional edge-space rule
// hoisted into a parameter so escapeEdgeSpaceIn can run it with the rule off.
func escapeInto(dst, src []byte, ctx escapeContext, lastSegment, escapeEdgeSpace bool) []byte {
	n := len(src)
	// Precomputed so the per-byte branch stays a cheap boolean test.
	escapeDollar := ctx == ctxGroup && lastSegment && dollarSegmentNeedsEscape(string(src))
	keyLike := ctx == ctxKey || ctx == ctxLocale

	for i := 0; i < n; {
		b := src[i]
		switch {
		case b == '\\':
			// Always, at any position, in every context: a literal backslash
			// would otherwise open an escape sequence on the next read.
			dst = append(dst, '\\', '\\')
			i++
		case b == '\n':
			dst = append(dst, '\\', 'n')
			i++
		case b == '\t':
			dst = append(dst, '\\', 't')
			i++
		case b == '\r':
			dst = append(dst, '\\', 'r')
			i++
		case b < 0x20:
			// Remaining C0 controls have no letter escape.
			dst = appendHexEscape(dst, b)
			i++
		case b == ' ' && escapeEdgeSpace && (i == 0 || i == n-1):
			// FIRST and LAST byte only. "   " becomes "\s \s", never "\s\s\s":
			// only edge whitespace is ambiguous, because the reader trims the
			// key and value slices. Whether a group segment takes part is
			// escapeEdgeSpaceIn's decision.
			dst = append(dst, '\\', 's')
			i++
		case b == '=' && keyLike:
			// The reader splits the line on the FIRST '=', so a key or locale
			// containing one must escape it. Values keep '=' literally:
			// "a=b=c=d" parses to the value "b=c=d".
			dst = appendHexEscape(dst, b)
			i++
		case (b == '[' || b == ']') && ctx != ctxValue:
			dst = appendHexEscape(dst, b)
			i++
		case b == '#' && i == 0 && keyLike:
			// DELIBERATE DIVERGENCE from KDE's writer, which emits "#key=v"
			// unescaped and then swallows the line as a comment on the next
			// read — a real round-trip data loss. KDE decodes \x23 back to '#'
			// correctly, so our output stays readable by KDE and is lossless.
			dst = appendHexEscape(dst, b)
			i++
		case b == 0xEF && i == 0 && ctx == ctxKey && bytes.HasPrefix(src, utf8BOM):
			// A key opening with the UTF-8 BOM opens the FILE with one when it
			// is the root group's first key, and Parse refuses such a file —
			// KDE decides a line is a header by looking at its first byte, so a
			// BOM misroutes every entry in it. Escaping just the leading 0xEF
			// breaks the sequence; \xef decodes back to the same byte here and
			// in KConfig, so the key itself is unchanged. Without this Write
			// would emit output its own Parse rejects (found by
			// FuzzCodecRoundTrip).
			dst = appendHexEscape(dst, b)
			i++
		case b == '$' && i == 0 && (escapeDollar || ctx == ctxLocale):
			// In a group segment this disambiguates "[$i]"-shaped names; in a
			// locale tag it stops "[$de]" from being re-read as a flag set.
			dst = appendHexEscape(dst, b)
			i++
		case b < 0x7f:
			dst = append(dst, b)
			i++
		case b == 0x7f:
			// DEL is escaped in values only; kwriteconfig6 writes it raw inside
			// group names and keys.
			if ctx == ctxValue {
				dst = appendHexEscape(dst, b)
			} else {
				dst = append(dst, b)
			}
			i++
		default:
			// b >= 0x80. Group and key contexts emit high bytes raw whatever
			// they are. In a value, a well-formed UTF-8 sequence is written raw
			// and anything else — a stray byte, an overlong form, a surrogate,
			// a value above U+10FFFF — is escaped per byte.
			if ctx != ctxValue {
				dst = append(dst, b)
				i++
				continue
			}
			if r, size := utf8.DecodeRune(src[i:]); r != utf8.RuneError || size > 1 {
				dst = append(dst, src[i:i+size]...)
				i += size
				continue
			}
			dst = appendHexEscape(dst, b)
			i++
		}
	}
	return dst
}

// localeIsCanonical reports whether raw — the inner text of a key's locale
// suffix — is already a fixed point of the ctxLocale escaper.
//
// It has to be one, because KConfig NEVER decodes a locale tag: the reader keeps
// the raw bytes and matches them against the process locale byte for byte.
// Verified against kconfig 6.29 under LANG=de_DE.UTF-8 — "Name[de]" is served
// for German while "Name[d\x65]" and "Name[ de ]" are not, and both of those
// survive a full kwriteconfig6 rewrite completely untouched.
//
// A tag that is not already canonical therefore cannot be modelled as its
// decoded form: decode "d\x65" to "de", write "[de]" back on the next apply, and
// an entry KDE was ignoring becomes the active German translation. Parse refuses
// such a file and Write refuses such a document, which keeps Entry.Locale's
// decoded form and its on-disk form the same string by construction.
func localeIsCanonical(raw []byte) bool {
	return bytes.Equal(raw, appendEscaped(nil, raw, ctxLocale, false))
}

// hexValue decodes one ASCII hex digit.
func hexValue(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	default:
		return 0, false
	}
}

// decodeGroupSegment decodes one raw bracket segment of a group header.
//
// It is NOT plain unescape, because KConfigIniBackend reads a bracket segment
// in two different ways depending on whether the segment holds a backslash:
//
//   - No backslash: the raw bytes ARE the group name, edge whitespace included.
//     "[ spaced group ]" addresses the group " spaced group ".
//   - Any backslash: the segment goes through printableToString, which strips
//     ASCII whitespace off both ends of the RAW field before decoding.
//     "[a\tb ]" therefore addresses "a<TAB>b", NOT "a<TAB>b ".
//
// Verified against kconfig 6.29 with kreadconfig6: a file holding
// "[Libinput][1][2][My\tPad ]" answers --group $'My\tPad' and not
// --group $'My\tPad '. Modelling the trailing space as part of the name splits
// one KDE group into two of ours, and the last-wins coalescing that KDE then
// applies resolves a shared key to the wrong side.
func decodeGroupSegment(raw []byte) ([]byte, error) {
	if bytes.IndexByte(raw, '\\') < 0 {
		return bytes.Clone(raw), nil
	}
	return unescape(bytes.Trim(raw, asciiSpace))
}

// unescape decodes one on-disk field (group segment, key, locale tag or value)
// into its byte value. It never trims: the caller has already trimmed the slice
// it hands over, which is why only \s, \t and \xNN can carry edge whitespace
// through.
//
// It returns an error for any escape KDE would merely warn about and leave raw.
// KDE's permissiveness there is a silent settings loss the user never sees, and
// plasma/files/ is hand-editable, so this codec refuses the file instead (§4.6).
func unescape(src []byte) ([]byte, error) {
	first := bytes.IndexByte(src, '\\')
	if first < 0 {
		return bytes.Clone(src), nil
	}

	out := make([]byte, 0, len(src))
	out = append(out, src[:first]...)
	for i := first; i < len(src); {
		if src[i] != '\\' {
			out = append(out, src[i])
			i++
			continue
		}
		if i+1 >= len(src) {
			// A lone trailing backslash is DROPPED, not kept. printableToString
			// writes the backslash into its output buffer and then breaks out of
			// the loop WITHOUT advancing the write cursor, so the closing
			// truncate() cuts the byte it just wrote back off again. Verified
			// against kconfig 6.29 in all three contexts: "a=abc\" reads as
			// "abc", "abc\=1" is the key "abc", and "[abc\]" is the group "abc".
			//
			// Keeping it as a literal instead would re-encode to "\\" and hand
			// KDE one character more than the file it came from held (§4.2).
			break
		}
		switch c := src[i+1]; c {
		case 's':
			out = append(out, ' ')
			i += 2
		case 't':
			out = append(out, '\t')
			i += 2
		case 'n':
			out = append(out, '\n')
			i += 2
		case 'r':
			out = append(out, '\r')
			i += 2
		case '\\':
			out = append(out, '\\')
			i += 2
		case ';', ',':
			// The .desktop compatibility carve-out: KConfig PRESERVES these two
			// escapes rather than resolving them, so the decoded value holds
			// BOTH characters. Re-encoding then yields "\\;", which is why
			// decode->encode is not idempotent for this one input — it
			// converges after a single pass, and every comparison in this
			// package is made on parsed models rather than bytes so the
			// asymmetry can never surface as phantom drift (§4.5).
			out = append(out, '\\', c)
			i += 2
		case 'x':
			if i+3 >= len(src) {
				// Fewer than two digits left: emit a literal 'x' and stop.
				out = append(out, 'x')
				i = len(src)
				continue
			}
			hi, okHi := hexValue(src[i+2])
			lo, okLo := hexValue(src[i+3])
			if !okHi || !okLo {
				// A non-hex digit emits a literal 'x' and STILL consumes both
				// characters, matching KConfig's reader.
				out = append(out, 'x')
			} else {
				out = append(out, hi<<4|lo)
			}
			i += 4
		default:
			return nil, fmt.Errorf("invalid escape sequence %q", string(src[i:i+2]))
		}
	}
	return out, nil
}
