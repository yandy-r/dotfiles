package plasma

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Glob syntax (§7.1). All three families match against the DECODED model —
// group segments and bare keys as the parser produced them — never against raw
// bytes, so a group header written as [with\ttab] on disk is matched by a
// pattern containing a literal tab.
//
//   - Group globs are the literal header text including brackets. A pattern
//     with N bracket pairs matches only groups with exactly N segments, and
//     inside a segment '*' matches any run of characters including none:
//     "[Tiling]" matches only the bare one-segment group, "[Tiling][*]" matches
//     [Tiling][<uuid>] and nothing longer.
//   - Key globs are "<group-glob>/<key-glob>". The key glob may itself contain
//     '/' — mimeapps.list keys look like "x-scheme-handler/mailto" — so the
//     split is at the first '/' AFTER the leading bracket run, not the first
//     '/' in the pattern.
//   - Value globs are "<group-glob>/<key-glob>=<value-glob>", split at the
//     first '=' after the key glob.
//
// The one special form is a group glob of exactly "[*]", which means "any group
// of any depth" rather than "any one-segment group". It only appears in key and
// value globs, where a depth-agnostic rule is the useful one.
const anyGroupGlob = "[*]"

// stripPattern is a parsed glob: a group matcher plus optional key and value
// matchers. Parsing is done once per rule per file rather than per entry.
type stripPattern struct {
	// segs are the per-segment globs of the group matcher; nil with anyDepth
	// set means the "[*]" wildcard.
	segs []string
	// anyDepth is true for the "[*]" group glob.
	anyDepth bool
	// key is the key-name glob ("" when the rule is a group rule).
	key string
	// value is the value glob ("" when the rule is not a value rule).
	value string
	// hasValue distinguishes an empty value glob from an absent one.
	hasValue bool
}

// parseGroupGlob parses a leading bracket run into per-segment globs and
// returns the offset just past the final ']'. A pattern that does not start
// with '[' yields ok=false, which callers treat as an unmatchable rule rather
// than a panic: a malformed entry in the governed table is a bug caught by the
// table tests, not a reason to abort a user's export.
func parseGroupGlob(pattern string) (segs []string, rest string, ok bool) {
	pos := 0
	for pos < len(pattern) && pattern[pos] == '[' {
		end := strings.IndexByte(pattern[pos+1:], ']')
		if end < 0 {
			return nil, "", false
		}
		segs = append(segs, pattern[pos+1:pos+1+end])
		pos += end + 2
	}
	if len(segs) == 0 {
		return nil, "", false
	}
	return segs, pattern[pos:], true
}

// parseGroupRule parses a VolatileGroups entry.
func parseGroupRule(pattern string) (stripPattern, bool) {
	segs, rest, ok := parseGroupGlob(pattern)
	if !ok || rest != "" {
		return stripPattern{}, false
	}
	return stripPattern{segs: segs, anyDepth: pattern == anyGroupGlob}, true
}

// parseKeyRule parses a VolatileKeys entry, "<group-glob>/<key-glob>".
func parseKeyRule(pattern string) (stripPattern, bool) {
	segs, rest, ok := parseGroupGlob(pattern)
	if !ok || !strings.HasPrefix(rest, "/") {
		return stripPattern{}, false
	}
	key := rest[1:]
	if key == "" {
		return stripPattern{}, false
	}
	return stripPattern{
		segs:     segs,
		anyDepth: strings.HasPrefix(pattern, anyGroupGlob+"/"),
		key:      key,
	}, true
}

// parseValueRule parses a VolatileValues entry,
// "<group-glob>/<key-glob>=<value-glob>". The split is at the first '=' after
// the group run; a key glob that must itself contain '=' is not expressible,
// which is acceptable because such a key is written \x3d on disk and no
// managed file has one.
func parseValueRule(pattern string) (stripPattern, bool) {
	segs, rest, ok := parseGroupGlob(pattern)
	if !ok || !strings.HasPrefix(rest, "/") {
		return stripPattern{}, false
	}
	rest = rest[1:]
	eq := strings.IndexByte(rest, '=')
	if eq < 0 {
		return stripPattern{}, false
	}
	key := rest[:eq]
	if key == "" {
		return stripPattern{}, false
	}
	return stripPattern{
		segs:     segs,
		anyDepth: strings.HasPrefix(pattern, anyGroupGlob+"/"),
		key:      key,
		value:    rest[eq+1:],
		hasValue: true,
	}, true
}

// matchesGroup reports whether a decoded group path satisfies the pattern's
// group matcher. The segment count must match exactly unless the pattern is the
// "[*]" any-depth wildcard, which also matches the root group.
func (p stripPattern) matchesGroup(path []string) bool {
	if p.anyDepth {
		return true
	}
	if len(path) != len(p.segs) {
		return false
	}
	for i := range path {
		if !globMatch(p.segs[i], path[i]) {
			return false
		}
	}
	return true
}

// globMatch reports whether s matches a glob in which '*' is the only
// metacharacter and matches any run of characters including none. It is a
// linear two-pointer matcher with backtracking on the last '*', which is enough
// for the governed table's patterns and avoids pulling in a regexp per rule.
func globMatch(pattern, s string) bool {
	var (
		p, i     int
		star     = -1
		starNext int
	)
	for i < len(s) {
		switch {
		case p < len(pattern) && pattern[p] == '*':
			star, starNext = p, i
			p++
		case p < len(pattern) && pattern[p] == s[i]:
			p++
			i++
		case star >= 0:
			// Backtrack: let the last '*' absorb one more byte.
			p = star + 1
			starNext++
			i = starNext
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

// Strip removes every volatile group, key, and value from doc according to
// spec, returning a new document and leaving doc untouched.
//
// Matching order (§7.2) is load-bearing: group rules run first and remove a
// whole group without its keys ever being considered, then key rules run over
// the surviving groups, then value rules over the surviving keys. Globs match
// the DECODED bare key with the locale and [$…] flag suffixes already removed
// by the parser, which is why the pattern "View Style" correctly strips the
// on-disk entry "View Style[$e]".
//
// keepColorScheme is the §7.4 guard, resolved by the caller because it requires
// filesystem probing the pure strip has no business doing. When true, the
// kdeglobals colour rules are NOT applied: the active scheme has no .colors
// file anywhere, which is the signature of a hand-rolled palette that exists
// only inside kdeglobals and would be destroyed by the strip.
func Strip(doc *Doc, spec FileSpec, keepColorScheme bool) *Doc {
	if doc == nil {
		return nil
	}

	groupRules := compileRules(spec.VolatileGroups, parseGroupRule, keepColorScheme, spec.Path)
	keyRules := compileRules(spec.VolatileKeys, parseKeyRule, keepColorScheme, spec.Path)
	valueRules := compileRules(spec.VolatileValues, parseValueRule, keepColorScheme, spec.Path)

	out := &Doc{Immutable: doc.Immutable}
	for _, g := range doc.Groups {
		if matchAny(groupRules, g.Path, "", nil) {
			continue
		}
		kept := Group{Path: g.Path, Immutable: g.Immutable}
		for _, entry := range g.Entries {
			if matchAny(keyRules, g.Path, entry.Key, nil) {
				continue
			}
			if matchAny(valueRules, g.Path, entry.Key, entry.Value) {
				continue
			}
			kept.Entries = append(kept.Entries, entry)
		}
		out.Groups = append(out.Groups, kept)
	}
	return out
}

// compileRules parses a rule list, dropping any rule the colour-scheme guard
// disables and any rule that does not parse. A malformed rule is silently
// inert rather than fatal: the governed-table tests are where a typo is caught,
// and refusing to export because of one is strictly worse for the user.
func compileRules(
	patterns []string,
	parse func(string) (stripPattern, bool),
	keepColorScheme bool,
	specPath string,
) []stripPattern {
	out := make([]stripPattern, 0, len(patterns))
	for _, pattern := range patterns {
		if keepColorScheme && isColorSchemeRule(specPath, pattern) {
			continue
		}
		if p, ok := parse(pattern); ok {
			out = append(out, p)
		}
	}
	return out
}

// matchAny reports whether any rule matches. A rule with a key glob requires a
// key match, and a rule with a value glob additionally requires a value match.
func matchAny(rules []stripPattern, group []string, key string, value []byte) bool {
	for _, r := range rules {
		if !r.matchesGroup(group) {
			continue
		}
		if r.key != "" && !globMatch(r.key, key) {
			continue
		}
		if r.hasValue && !globMatch(r.value, string(value)) {
			continue
		}
		return true
	}
	return false
}

// colorSchemeGroupRules and colorSchemeKeyRule are the exact kdeglobals rules
// the §7.4 guard can disable. They are named here rather than inferred so that
// widening the kdeglobals strip table does not accidentally widen what the
// guard protects.
var (
	colorSchemeGroupRules = []string{"[Colors:*]", "[Colors:*][*]", "[ColorEffects:*]", "[WM]"}
	colorSchemeKeyRule    = "[General]/ColorSchemeHash"
)

// isColorSchemeRule reports whether a rule belongs to the guarded colour set.
// The guard applies only to kdeglobals; kdedefaults/kdeglobals names the scheme
// rather than carrying its derived palette.
func isColorSchemeRule(specPath, pattern string) bool {
	if specPath != "kdeglobals" {
		return false
	}
	if pattern == colorSchemeKeyRule {
		return true
	}
	for _, r := range colorSchemeGroupRules {
		if pattern == r {
			return true
		}
	}
	return false
}

// colorSchemeGuard resolves the §7.4 guard for a run. It returns whether the
// colour rules must be KEPT (i.e. the strip suppressed) and, when so, an
// advisory naming the scheme.
//
// The active scheme is read from kdedefaults/kdeglobals [General] ColorScheme,
// falling back to kdeglobals's own. An unnamed scheme means there is no
// hand-rolled palette to protect, so the strip proceeds.
func (e *Engine) colorSchemeGuard() (keep bool, advisory string) {
	scheme := e.activeColorScheme()
	if scheme == "" {
		return false, ""
	}
	if e.hasColorSchemeFile(scheme) {
		return false, ""
	}
	return true, "plasma: color scheme " + strconv.Quote(scheme) +
		" has no .colors file; keeping [Colors:*] in kdeglobals"
}

// activeColorScheme reads [General] ColorScheme from the live kdedefaults
// overlay, falling back to the live kdeglobals. A parse failure yields "" —
// the guard is a safety heuristic and must not turn a broken unrelated file
// into a failed export; the file's own parse error surfaces when it is planned.
func (e *Engine) activeColorScheme() string {
	for _, rel := range []string{"kdedefaults/kdeglobals", "kdeglobals"} {
		abs := filepath.Join(e.configDir, filepath.FromSlash(rel))
		data, err := os.ReadFile(abs) //nolint:gosec // path is table-derived and denylist-checked
		if err != nil {
			continue
		}
		doc, err := Parse(abs, data)
		if err != nil {
			continue
		}
		if v, ok := lookupValue(doc, []string{"General"}, "ColorScheme", ""); ok && len(v) > 0 {
			return string(v)
		}
	}
	return ""
}

// hasColorSchemeFile reports whether a <scheme>.colors file exists in the user
// or any system colour-scheme directory. The user directory is searched first,
// then the system ones in order. The match is case-insensitive on the base name
// because KDE's scheme names and file names differ in case on some
// distributions.
func (e *Engine) hasColorSchemeFile(scheme string) bool {
	want := strings.ToLower(scheme) + ".colors"
	dirs := append([]string{filepath.Join(e.dataHome, "color-schemes")}, e.systemColorSchemes...)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if strings.ToLower(entry.Name()) == want {
				return true
			}
		}
	}
	return false
}
