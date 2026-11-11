package plasma

import (
	"bytes"
	"sort"
	"strings"
)

// DiffKind classifies one per-key difference between the repo and the live
// document.
type DiffKind int

// Diff kinds. Two are informational — DiffMissingRepo and
// DiffExpectedVolatile — and the other three are what apply acts on. New kinds
// are APPENDED: the values are iota-derived and the strings are a machine
// contract for `dotf plasma diff --json`, so reordering them would silently
// relabel every consumer's data.
const (
	// DiffValueMismatch: the key is present on both sides with different values.
	DiffValueMismatch DiffKind = iota
	// DiffMissingLive: the key is in the repo and absent live.
	DiffMissingLive
	// DiffMissingRepo: the key is live-only and UNMANAGED — nothing in the
	// strip table explains it, so it is state the repo simply does not know
	// about. Informational, never a failure: apply never deletes a live key.
	DiffMissingRepo
	// DiffFlagsMismatch: same value, different [$…] flag string.
	DiffFlagsMismatch
	// DiffExpectedVolatile: the key is live-only AND matches one of this file's
	// own volatile strip rules, so export removes it by design and it can never
	// appear in plasma/files/. Its absence from the repo is the design working,
	// not drift — which is why it is reported apart from DiffMissingRepo
	// instead of being mixed into the same list of "keys only live", where a
	// genuine unmanaged key was indistinguishable from several hundred
	// [Tiling][<uuid>] entries.
	DiffExpectedVolatile
)

// String renders a diff kind for the JSON report's "kind" field.
func (k DiffKind) String() string {
	switch k {
	case DiffValueMismatch:
		return "value-mismatch"
	case DiffMissingLive:
		return "missing-live"
	case DiffMissingRepo:
		return "missing-repo"
	case DiffFlagsMismatch:
		return "flags-mismatch"
	case DiffExpectedVolatile:
		return "expected-volatile"
	default:
		return "unknown"
	}
}

// Actionable reports whether a difference is one apply would act on. It is the
// single definition of "drift", used by FileDiff.Drifted, Status's key counter,
// and the CLI's drift/only-live split, so the three cannot disagree about which
// kinds are merely informational.
func (k DiffKind) Actionable() bool {
	return k != DiffMissingRepo && k != DiffExpectedVolatile
}

// KeyDiff is one (group, key, locale) difference.
type KeyDiff struct {
	// Group is the decoded group path; nil means the root group.
	Group []string
	// Key is the decoded bare key.
	Key string
	// Locale is the decoded locale tag ("" = none).
	Locale string
	// Kind classifies the difference.
	Kind DiffKind
	// Repo is the repo-side value; nil when the key is repo-absent.
	Repo []byte
	// Live is the live-side value; nil when the key is live-absent.
	Live []byte
}

// FileDiff is one managed file's worth of differences.
type FileDiff struct {
	// Spec is the FileSpec this diff was computed for.
	Spec FileSpec
	// RepoPresent records whether the repo file exists.
	RepoPresent bool
	// LivePresent records whether the live file exists.
	LivePresent bool
	// Keys holds every difference, sorted by (group, key, locale) using the
	// codec's canonical comparators.
	Keys []KeyDiff
}

// Drifted reports whether this file has any difference that apply would act on.
// A live file always has more keys than the repo copy (everything the strip
// table removed, plus everything Plasma has written since), so neither flavour
// of live-only key can ever mean drift.
func (d FileDiff) Drifted() bool {
	for _, k := range d.Keys {
		if k.Kind.Actionable() {
			return true
		}
	}
	return false
}

// rawContentKey is the pseudo-key a FormatRaw file's byte difference is
// reported under. kdedefaults/package has no groups and no keys, but the diff
// vocabulary is per key, so its whole content is modelled as one root-group
// entry.
const rawContentKey = "<raw>"

// groupKey joins a decoded group path with KConfig's own 0x1D separator, giving
// a map key that cannot collide with a path containing the separator's printable
// neighbours.
func groupKey(path []string) string { return strings.Join(path, groupSeparator) }

// renderGroup formats a decoded group path as the header text a human reads:
// "[Desktops]", "[Tiling][x][y]", or "[<default>]" for the root group. It is
// used in error messages and in the CLI's plasma.change=/plasma.drift= lines.
func renderGroup(path []string) string {
	if len(path) == 0 {
		return "[<default>]"
	}
	var b strings.Builder
	for _, seg := range path {
		b.WriteByte('[')
		b.WriteString(seg)
		b.WriteByte(']')
	}
	return b.String()
}

// findGroup returns the group with the given decoded path, or nil.
func findGroup(doc *Doc, path []string) *Group {
	if doc == nil {
		return nil
	}
	want := groupKey(path)
	for i := range doc.Groups {
		if groupKey(doc.Groups[i].Path) == want {
			return &doc.Groups[i]
		}
	}
	return nil
}

// findEntry returns the entry with the given (key, locale) identity, or nil.
func findEntry(g *Group, key, locale string) *Entry {
	if g == nil {
		return nil
	}
	for i := range g.Entries {
		if g.Entries[i].Key == key && g.Entries[i].Locale == locale {
			return &g.Entries[i]
		}
	}
	return nil
}

// lookupValue returns the decoded value of one (group, key, locale) identity.
func lookupValue(doc *Doc, group []string, key, locale string) ([]byte, bool) {
	e := findEntry(findGroup(doc, group), key, locale)
	if e == nil {
		return nil, false
	}
	return e.Value, true
}

// entryFlags renders the flag string an entry will actually be SERIALISED with:
// its own letters in KConfig's write order, plus the group-header [$i] the
// writer folds onto every entry of an immutable group, plus the 'd' a tombstone
// carries.
//
// Every flag comparison in this package goes through it, and that is not
// optional. Comparing Entry.RawFlags verbatim compares a repo document that has
// been through the writer's normalisation against a live document that has not,
// so a group written as "[Windows][$i]" — a distro lockdown, a kiosk profile, a
// KDE file before its first rewrite — reported flags-mismatch drift on keys
// whose values were identical, forever: diff exited 1 on every run, apply
// re-opened the file on every run, and no sequence of commands could converge
// it. A nil group is the "no group context" case and contributes nothing.
func entryFlags(g *Group, e *Entry) string {
	groupImmutable := g != nil && g.Immutable
	return canonicalFlags(e.RawFlags, groupImmutable, e.Deleted)
}

// Converged reports whether every entry the repo document names is present live
// with a byte-equal value and an equal SERIALISED flag string.
//
// This subset check — not file equality, not a hash — is the single most
// important rule in the package. A live KConfig file ALWAYS has more keys than
// the repo copy, so bytes.Equal would report permanent drift forever.
//
// It is deliberately a thin predicate over diffDocs rather than a second
// comparison loop: convergence is EXACTLY "diffDocs found no actionable
// difference", since every repo-side kind (missing-live, value-mismatch,
// flags-mismatch) is actionable and every live-only kind is not.
// TestConvergedMatchesDiffDocs is the tripwire that keeps them one predicate.
//
// A nil live document needs no special case: findGroup and findEntry are both
// nil-safe, so every repo entry reports missing-live and a repo that names any
// entry at all is correctly not converged.
func Converged(repo, live *Doc) bool {
	for _, d := range diffDocs(repo, live) {
		if d.Kind.Actionable() {
			return false
		}
	}
	return true
}

// hasEntries reports whether a document names any entry at all. A document of
// empty groups asserts nothing and is trivially converged.
func hasEntries(doc *Doc) bool {
	for i := range doc.Groups {
		if len(doc.Groups[i].Entries) > 0 {
			return true
		}
	}
	return false
}

// Merge produces the merged document M from repo document R and live document
// L, along with the per-key writes it performs and any immutability refusals.
//
// The rules (§6.2), all of which exist to make apply non-destructive:
//
//  1. M starts as a deep copy of L, so live-only keys and live-only groups
//     survive byte for byte — including their exact Value bytes, RawFlags, and
//     locale tag.
//  2. A group R names that L lacks is created. Its position in the output is
//     decided entirely by the canonical group sort, not by insertion order.
//  3. A group L marks Immutable is skipped WHOLE, an *ImmutableError is
//     recorded, and the merge continues. The run is not aborted.
//  4. An entry R names is overwritten in M — Value, RawFlags, and Deleted all
//     come from R — or inserted when absent.
//  5. Nothing is ever deleted. A repo tombstone ([$d]) is written through as a
//     tombstone, which is the ONLY supported way to express "remove this live
//     key": deleting a key the user set through System Settings after the last
//     export would be indistinguishable from deleting a key KDE added on
//     upgrade.
//
// A file-level [$i] on L is refused outright: merged is nil and the single
// returned error names the file.
func Merge(livePath string, repo, live *Doc) (merged *Doc, keys []KeyAction, errs []error) {
	if live != nil && live.Immutable {
		return nil, nil, []error{&ImmutableError{Path: livePath}}
	}

	merged = cloneDoc(live)
	if repo == nil {
		return merged, nil, nil
	}

	for i := range repo.Groups {
		rg := &repo.Groups[i]
		mg := findGroup(merged, rg.Path)
		if mg != nil && mg.Immutable {
			errs = append(errs, &ImmutableError{Path: livePath, Group: rg.Path})
			continue
		}
		if mg == nil {
			merged.Groups = append(merged.Groups, Group{
				Path:      append([]string(nil), rg.Path...),
				Immutable: rg.Immutable,
			})
			mg = &merged.Groups[len(merged.Groups)-1]
		}
		for j := range rg.Entries {
			re := rg.Entries[j]
			existing := findEntry(mg, re.Key, re.Locale)
			var from []byte
			if existing != nil {
				if bytes.Equal(existing.Value, re.Value) &&
					entryFlags(rg, &re) == entryFlags(mg, existing) {
					continue
				}
				from = append([]byte(nil), existing.Value...)
			}
			written := cloneEntry(re)
			if existing != nil {
				*existing = written
			} else {
				mg.Entries = append(mg.Entries, written)
			}
			keys = append(keys, KeyAction{
				Group:     append([]string(nil), rg.Path...),
				Key:       re.Key,
				Locale:    re.Locale,
				From:      from,
				To:        append([]byte(nil), re.Value...),
				Tombstone: re.Deleted,
			})
		}
	}

	sortKeyActions(keys)
	return merged, keys, errs
}

// Overlay merges the recommended document into the repo document using the same
// rules as Merge, giving the three-document precedence
// live < plasma/files < plasma/recommended. It returns a new document; base is
// left untouched.
//
// Errors are impossible here — neither hand-authored repo document carries a
// live immutability marker — so the immutability channel is deliberately
// dropped rather than plumbed to a caller that could do nothing with it.
func Overlay(base, over *Doc) *Doc {
	merged, _, _ := Merge("", over, base)
	return merged
}

// cloneDoc deep-copies a document so a merge never aliases the caller's live
// model. A nil input yields an empty document, which is what "live file absent"
// means to the merge.
func cloneDoc(doc *Doc) *Doc {
	out := &Doc{}
	if doc == nil {
		return out
	}
	out.Immutable = doc.Immutable
	out.Groups = make([]Group, 0, len(doc.Groups))
	for i := range doc.Groups {
		g := doc.Groups[i]
		clone := Group{
			Path:      append([]string(nil), g.Path...),
			Immutable: g.Immutable,
			Entries:   make([]Entry, 0, len(g.Entries)),
		}
		for j := range g.Entries {
			clone.Entries = append(clone.Entries, cloneEntry(g.Entries[j]))
		}
		out.Groups = append(out.Groups, clone)
	}
	return out
}

// cloneEntry copies an entry, detaching its Value slice.
func cloneEntry(e Entry) Entry {
	e.Value = append([]byte(nil), e.Value...)
	return e
}

// diffDocs computes every per-key difference between repo and live, in both
// directions. Live-only keys are reported as DiffMissingRepo and are
// informational: they are the strip table's output plus whatever Plasma has
// written since.
//
// It is the package's ONE comparison loop. Engine.Diff renders its output
// directly and Converged is a predicate over it, so `dotf plasma apply`,
// `diff`, `validate`, `status`, and doctor cannot disagree about whether a
// given key differs — a rule changed here changes all of them at once.
func diffDocs(repo, live *Doc) []KeyDiff {
	var out []KeyDiff

	if repo != nil {
		for i := range repo.Groups {
			rg := &repo.Groups[i]
			lg := findGroup(live, rg.Path)
			for j := range rg.Entries {
				re := &rg.Entries[j]
				le := findEntry(lg, re.Key, re.Locale)
				switch {
				case le == nil:
					out = append(out, KeyDiff{
						Group: rg.Path, Key: re.Key, Locale: re.Locale,
						Kind: DiffMissingLive, Repo: re.Value,
					})
				case !bytes.Equal(re.Value, le.Value):
					out = append(out, KeyDiff{
						Group: rg.Path, Key: re.Key, Locale: re.Locale,
						Kind: DiffValueMismatch, Repo: re.Value, Live: le.Value,
					})
				case entryFlags(rg, re) != entryFlags(lg, le):
					out = append(out, KeyDiff{
						Group: rg.Path, Key: re.Key, Locale: re.Locale,
						Kind: DiffFlagsMismatch, Repo: re.Value, Live: le.Value,
					})
				}
			}
		}
	}

	if live != nil {
		for i := range live.Groups {
			lg := &live.Groups[i]
			rg := findGroup(repo, lg.Path)
			for j := range lg.Entries {
				le := &lg.Entries[j]
				if findEntry(rg, le.Key, le.Locale) != nil {
					continue
				}
				out = append(out, KeyDiff{
					Group: lg.Path, Key: le.Key, Locale: le.Locale,
					Kind: DiffMissingRepo, Live: le.Value,
				})
			}
		}
	}

	sortKeyDiffs(out)
	return out
}

// sortKeyDiffs orders differences canonically: group names as UTF-16 code units
// (QString order), then keys as UTF-8 bytes, then locale with "" first. The two
// comparators are deliberately different — that is the codec's rule, and diff
// output must sort the same way the file does.
func sortKeyDiffs(d []KeyDiff) {
	sort.SliceStable(d, func(i, j int) bool {
		return lessIdentity(d[i].Group, d[i].Key, d[i].Locale, d[j].Group, d[j].Key, d[j].Locale)
	})
}

// sortKeyActions orders per-key writes with the same comparator as sortKeyDiffs.
func sortKeyActions(k []KeyAction) {
	sort.SliceStable(k, func(i, j int) bool {
		return lessIdentity(k[i].Group, k[i].Key, k[i].Locale, k[j].Group, k[j].Key, k[j].Locale)
	})
}

// lessIdentity is the shared (group, key, locale) ordering.
func lessIdentity(ga []string, ka, la string, gb []string, kb, lb string) bool {
	if c := compareUTF16(groupKey(ga), groupKey(gb)); c != 0 {
		return c < 0
	}
	if c := strings.Compare(ka, kb); c != 0 {
		return c < 0
	}
	return la < lb
}
