package plasma

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestMerge_PreservesLiveOnlyKeys is the rule the whole design rests on: apply
// asserts values, it never deletes. Everything the strip table removed on
// export still lives in the live file, so a merge that dropped live-only keys
// would destroy settings on every run.
func TestMerge_PreservesLiveOnlyKeys(t *testing.T) {
	live := mustParse(t, "live", []byte(strings.Join([]string{
		"[Desktops]",
		"Id_1=00000000-0000-4000-8000-000000000001",
		"Number=4",
		"",
		"[Tiling][00000000-0000-4000-8000-000000000001]",
		"tiles={\"width\":0.25}",
		"",
	}, "\n")))
	repo := mustParse(t, "repo", []byte("[Desktops]\nNumber=10\nRows=1\n"))

	merged, keys, errs := Merge("live", repo, live)
	if len(errs) != 0 {
		t.Fatalf("Merge errors = %v, want none", errs)
	}

	desktops := findGroup(merged, []string{"Desktops"})
	if got, _ := lookupValue(merged, []string{"Desktops"}, "Number", ""); string(got) != "10" {
		t.Errorf("Number = %q, want the repo value 10", got)
	}
	if got, _ := lookupValue(merged, []string{"Desktops"}, "Rows", ""); string(got) != "1" {
		t.Errorf("Rows = %q, want the inserted repo value 1", got)
	}
	// The live-only UUID survives byte for byte, and so does the live-only
	// group next to it.
	if e := findEntry(desktops, "Id_1", ""); e == nil ||
		string(e.Value) != "00000000-0000-4000-8000-000000000001" {
		t.Error("the live-only Id_1 key was not preserved byte for byte")
	}
	if findGroup(merged, []string{"Tiling", "00000000-0000-4000-8000-000000000001"}) == nil {
		t.Error("a live-only GROUP was dropped")
	}

	// Only the two keys that actually changed are reported as writes.
	if len(keys) != 2 {
		t.Fatalf("len(keys) = %d, want 2 (%+v)", len(keys), keys)
	}
	if keys[0].Key != "Number" || string(keys[0].From) != "4" || string(keys[0].To) != "10" {
		t.Errorf("keys[0] = %+v, want Number 4 -> 10", keys[0])
	}
	if keys[1].Key != "Rows" || keys[1].From != nil {
		t.Errorf("keys[1] = %+v, want an inserted Rows with no From", keys[1])
	}
}

// TestMerge_CreatesMissingGroup: a group the repo names and the live file lacks
// is created, and its position is decided by the canonical group sort, not by
// insertion order.
func TestMerge_CreatesMissingGroup(t *testing.T) {
	live := mustParse(t, "live", []byte("[Zebra]\nz=1\n"))
	repo := mustParse(t, "repo", []byte("[Alpha]\na=1\n"))

	merged, keys, errs := Merge("live", repo, live)
	if len(errs) != 0 {
		t.Fatalf("Merge errors = %v", errs)
	}
	if findGroup(merged, []string{"Alpha"}) == nil {
		t.Fatal("[Alpha] was not created")
	}
	if len(keys) != 1 || keys[0].Key != "a" {
		t.Fatalf("keys = %+v, want the single inserted key a", keys)
	}

	out := string(mustWrite(t, merged))
	want := "[Alpha]\na=1\n\n[Zebra]\nz=1\n"
	if out != want {
		t.Errorf("Write(merged) =\n%q\nwant\n%q", out, want)
	}
}

// TestMerge_RepoWinsIncludingFlagsAndLocales: Value, RawFlags, Deleted and
// HasValue all come from the repo, and a locale variant is a separate identity
// that never re-stamps its siblings.
func TestMerge_RepoWinsIncludingFlagsAndLocales(t *testing.T) {
	live := mustParse(t, "live", []byte("[G]\nName=live\nName[de]=leben\nPath[$e]=/live\n"))
	repo := mustParse(t, "repo", []byte("[G]\nName=repo\nPath[$ie]=$HOME/x\n"))

	merged, _, errs := Merge("live", repo, live)
	if len(errs) != 0 {
		t.Fatalf("Merge errors = %v", errs)
	}
	g := findGroup(merged, []string{"G"})

	if e := findEntry(g, "Name", ""); string(e.Value) != "repo" {
		t.Errorf("Name = %q, want the repo value", e.Value)
	}
	if e := findEntry(g, "Name", "de"); e == nil || string(e.Value) != "leben" {
		t.Error("the live-only locale variant Name[de] was not preserved")
	}
	path := findEntry(g, "Path", "")
	if string(path.Value) != "$HOME/x" {
		t.Errorf("Path value = %q, want the repo value", path.Value)
	}
	if path.RawFlags != "ie" {
		t.Errorf("Path RawFlags = %q, want the repo flags %q", path.RawFlags, "ie")
	}
}

// TestMerge_TombstoneWritesThrough: a [$d] repo entry is the ONLY supported way
// to express "remove this live key", because a prune mode could not tell a key
// the user just set from a key KDE added on upgrade.
func TestMerge_TombstoneWritesThrough(t *testing.T) {
	live := mustParse(t, "live", []byte("[G]\nGone=still here\nStays=1\n"))
	repo := mustParse(t, "repo", []byte("[G]\nGone[$d]\n"))

	merged, keys, errs := Merge("live", repo, live)
	if len(errs) != 0 {
		t.Fatalf("Merge errors = %v", errs)
	}
	if len(keys) != 1 || !keys[0].Tombstone {
		t.Fatalf("keys = %+v, want one tombstone write", keys)
	}
	out := string(mustWrite(t, merged))
	if !strings.Contains(out, "Gone[$d]\n") {
		t.Errorf("output %q does not carry the tombstone", out)
	}
	if strings.Contains(out, "Gone[$d]=") {
		t.Errorf("output %q kept an '=' on a tombstone", out)
	}
	if !strings.Contains(out, "Stays=1") {
		t.Error("an untouched live key was lost")
	}
}

// TestMerge_ImmutableGroupIsSkipped: the group is left alone, an ImmutableError
// is recorded, and the rest of the merge still happens.
func TestMerge_ImmutableGroupIsSkipped(t *testing.T) {
	live := mustParse(t, "live", []byte("[Locked][$i]\nk=live\n\n[Open]\nk=live\n"))
	repo := mustParse(t, "repo", []byte("[Locked]\nk=repo\n\n[Open]\nk=repo\n"))

	merged, keys, errs := Merge("/live/kwinrc", repo, live)
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want exactly one", errs)
	}
	var imm *ImmutableError
	if !errors.As(errs[0], &imm) {
		t.Fatalf("errs[0] = %v, want *ImmutableError", errs[0])
	}
	if len(imm.Group) != 1 || imm.Group[0] != "Locked" {
		t.Errorf("ImmutableError.Group = %v, want [Locked]", imm.Group)
	}

	if got, _ := lookupValue(merged, []string{"Locked"}, "k", ""); string(got) != "live" {
		t.Errorf("the immutable group was written: k = %q", got)
	}
	if got, _ := lookupValue(merged, []string{"Open"}, "k", ""); string(got) != "repo" {
		t.Errorf("the merge stopped at the immutable group: [Open] k = %q", got)
	}
	if len(keys) != 1 || keys[0].Group[0] != "Open" {
		t.Errorf("keys = %+v, want only the [Open] write", keys)
	}
}

// TestMerge_ImmutableFileIsRefused: a file-level [$i] is a deliberate
// lockdown, so the whole file is refused rather than merged.
func TestMerge_ImmutableFileIsRefused(t *testing.T) {
	live := mustParse(t, "live", []byte("[$i]\n[G]\nk=live\n"))
	repo := mustParse(t, "repo", []byte("[G]\nk=repo\n"))

	merged, keys, errs := Merge("/live/kwinrc", repo, live)
	if merged != nil || keys != nil {
		t.Fatalf("merged = %v, keys = %v, want both nil", merged, keys)
	}
	var imm *ImmutableError
	if len(errs) != 1 || !errors.As(errs[0], &imm) {
		t.Fatalf("errs = %v, want one *ImmutableError", errs)
	}
	if imm.Group != nil {
		t.Errorf("ImmutableError.Group = %v, want nil for a file-level marker", imm.Group)
	}
}

// TestConverged_SubsetNotEquality is the §6.1 predicate. A live file with 200
// extra keys is converged; reaching for bytes.Equal or a hash would report
// permanent drift forever.
func TestConverged_SubsetNotEquality(t *testing.T) {
	var b strings.Builder
	b.WriteString("[General]\nNumber=10\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, "extra_%03d=%d\n", i, i)
	}
	live := mustParse(t, "live", []byte(b.String()))
	repo := mustParse(t, "repo", []byte("[General]\nNumber=10\n"))

	if !Converged(repo, live) {
		t.Error("a live file with 200 extra keys was reported as drifted")
	}
	if countEntries(live) != 201 {
		t.Fatalf("premise broken: live has %d entries", countEntries(live))
	}

	drifted := mustParse(t, "repo", []byte("[General]\nNumber=11\n"))
	if Converged(drifted, live) {
		t.Error("a differing value was reported as converged")
	}
	flagged := mustParse(t, "repo", []byte("[General]\nNumber[$i]=10\n"))
	if Converged(flagged, live) {
		t.Error("a differing flag string was reported as converged")
	}
	missing := mustParse(t, "repo", []byte("[General]\nAbsent=1\n"))
	if Converged(missing, live) {
		t.Error("a repo key absent live was reported as converged")
	}
}

// TestConvergedMatchesDiffDocs is the anti-divergence tripwire for §6.1.
//
// Converged and diffDocs were once two independently written loops over the
// same triple (missing-live / value-mismatch / flags-mismatch). Nothing failed
// when they were equal, and nothing would have failed when they stopped being
// equal: a normalisation added to one comparison would have left `dotf plasma
// apply` (which resolves to Converged) planning a merge for a key that `dotf
// plasma diff` and `validate` (which resolve to diffDocs) called converged —
// a file rewritten on every single run that no command would ever report.
//
// Converged is now a predicate over diffDocs, so this test passes trivially
// today. That is the point: it fails the moment anyone re-forks the comparison,
// including the subtle re-fork of adding a rule to one site only.
func TestConvergedMatchesDiffDocs(t *testing.T) {
	parse := func(name, text string) *Doc {
		if text == "" {
			return nil
		}
		return mustParse(t, name, []byte(text))
	}

	tests := []struct {
		name string
		repo string
		live string
	}{
		{"both absent", "", ""},
		{"repo absent, live has keys", "", "[G]\nk=1\n"},
		{"live absent, repo has keys", "[G]\nk=1\n", ""},
		{"live absent, repo is empty groups", "[G]\n", ""},
		{"identical", "[G]\nk=1\n", "[G]\nk=1\n"},
		{"live superset", "[G]\nk=1\n", "[G]\nk=1\nextra=2\n[Other]\nz=3\n"},
		{"value mismatch", "[G]\nk=1\n", "[G]\nk=2\n"},
		{"key missing live", "[G]\nk=1\n", "[G]\nother=1\n"},
		{"group missing live", "[G]\nk=1\n", "[Other]\nk=1\n"},
		{"flags mismatch", "[G]\nk[$i]=1\n", "[G]\nk=1\n"},
		{"group-[$i] folded onto entry", "[G][$i]\nk[$i]=1\n", "[G][$i]\nk=1\n"},
		{"locale distinguishes identity", "[G]\nk[de]=1\n", "[G]\nk=1\n"},
		{"tombstone not present live", "[G]\nk[$d]=\n", "[G]\nother=1\n"},
		{"tombstone present live", "[G]\nk[$d]=\n", "[G]\nk[$d]=\n"},
		{"nested group path", "[A][B]\nk=1\n", "[A][B]\nk=1\n[A]\nk=2\n"},
		{"empty value both sides", "[G]\nk=\n", "[G]\nk=\n"},
	}

	for _, tc := range tests {
		repo, live := parse("repo", tc.repo), parse("live", tc.live)

		want := true
		for _, d := range diffDocs(repo, live) {
			if d.Kind.Actionable() {
				want = false
				break
			}
		}
		if got := Converged(repo, live); got != want {
			t.Errorf("%s: Converged = %v but diffDocs says %v; the two convergence "+
				"predicates have diverged", tc.name, got, want)
		}
	}
}

func TestConverged_EdgeCases(t *testing.T) {
	if !Converged(nil, nil) {
		t.Error("a nil repo document asserts nothing and must be converged")
	}
	empty := mustParse(t, "repo", []byte("[G]\n"))
	if !Converged(empty, nil) {
		t.Error("a repo document of empty groups asserts nothing")
	}
	if Converged(mustParse(t, "repo", []byte("[G]\nk=1\n")), nil) {
		t.Error("a repo key with no live file at all is not converged")
	}
}

// TestOverlay_RecommendedWins pins the three-document precedence
// live < files < recommended.
func TestOverlay_RecommendedWins(t *testing.T) {
	base := mustParse(t, "files", []byte("[G]\nshared=files\nonly-files=1\n"))
	over := mustParse(t, "recommended", []byte("[G]\nshared=recommended\nonly-rec=1\n"))

	got := Overlay(base, over)
	if v, _ := lookupValue(got, []string{"G"}, "shared", ""); string(v) != "recommended" {
		t.Errorf("shared = %q, want the recommended value", v)
	}
	if _, ok := lookupValue(got, []string{"G"}, "only-files", ""); !ok {
		t.Error("a files-only key was dropped by the overlay")
	}
	if _, ok := lookupValue(got, []string{"G"}, "only-rec", ""); !ok {
		t.Error("a recommended-only key was not added")
	}
	// The overlay must not mutate the base document.
	if v, _ := lookupValue(base, []string{"G"}, "shared", ""); string(v) != "files" {
		t.Errorf("Overlay mutated its base: shared = %q", v)
	}
}

func TestDiffDocs_AllFourKinds(t *testing.T) {
	repo := mustParse(t, "repo", []byte("[G]\nsame=1\nvalue=repo\nflags[$i]=x\nonlyrepo=1\n"))
	live := mustParse(t, "live", []byte("[G]\nsame=1\nvalue=live\nflags=x\nonlylive=1\n"))

	got := diffDocs(repo, live)
	byKey := make(map[string]KeyDiff, len(got))
	for _, d := range got {
		byKey[d.Key] = d
	}

	if _, ok := byKey["same"]; ok {
		t.Error("an identical key was reported as a difference")
	}
	if byKey["value"].Kind != DiffValueMismatch {
		t.Errorf("value Kind = %v, want DiffValueMismatch", byKey["value"].Kind)
	}
	if byKey["flags"].Kind != DiffFlagsMismatch {
		t.Errorf("flags Kind = %v, want DiffFlagsMismatch", byKey["flags"].Kind)
	}
	if byKey["onlyrepo"].Kind != DiffMissingLive {
		t.Errorf("onlyrepo Kind = %v, want DiffMissingLive", byKey["onlyrepo"].Kind)
	}
	if byKey["onlylive"].Kind != DiffMissingRepo {
		t.Errorf("onlylive Kind = %v, want DiffMissingRepo", byKey["onlylive"].Kind)
	}
}

// TestFileDiff_Drifted: a live-only key is informational and must never make a
// file drifted, or every managed file would drift forever.
func TestFileDiff_Drifted(t *testing.T) {
	onlyLive := FileDiff{Keys: []KeyDiff{{Key: "a", Kind: DiffMissingRepo}}}
	if onlyLive.Drifted() {
		t.Error("DiffMissingRepo made a file drifted")
	}
	for _, kind := range []DiffKind{DiffValueMismatch, DiffMissingLive, DiffFlagsMismatch} {
		fd := FileDiff{Keys: []KeyDiff{{Key: "a", Kind: DiffMissingRepo}, {Key: "b", Kind: kind}}}
		if !fd.Drifted() {
			t.Errorf("%v did not make a file drifted", kind)
		}
	}
}

func TestRenderGroup(t *testing.T) {
	tests := []struct {
		path []string
		want string
	}{
		{nil, "[<default>]"},
		{[]string{"Desktops"}, "[Desktops]"},
		{[]string{"Tiling", "x", "y"}, "[Tiling][x][y]"},
	}
	for _, tc := range tests {
		if got := renderGroup(tc.path); got != tc.want {
			t.Errorf("renderGroup(%v) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestConverged_ComparesSerialisedFlags: a group-header [$i] is folded onto
// every entry by Write, so the repo document (already written) and the live
// document (not yet rewritten by KDE) spell the SAME state two ways. Comparing
// Entry.RawFlags verbatim made every lockdown group permanently drifted.
func TestConverged_ComparesSerialisedFlags(t *testing.T) {
	// Repo: the canonical form Write produces. Live: the same file before KDE
	// has rewritten it, with the marker only on the header.
	repo := mustParse(t, "repo", []byte("[Windows][$i]\nFocusPolicy[$i]=FocusFollowsMouse\n"))
	live := mustParse(t, "live", []byte("[Windows][$i]\nFocusPolicy=FocusFollowsMouse\n"))

	if !Converged(repo, live) {
		t.Error("a group-[$i] notation difference was reported as drift")
	}
	if got := diffDocs(repo, live); len(got) != 0 {
		t.Errorf("diffDocs = %+v, want no differences", got)
	}

	// The propagation is not a licence to ignore flags: an entry-level marker
	// the live side genuinely lacks is still a difference.
	unlocked := mustParse(t, "live", []byte("[Windows]\nFocusPolicy=FocusFollowsMouse\n"))
	if Converged(repo, unlocked) {
		t.Error("a genuinely absent immutability marker was reported as converged")
	}
	got := diffDocs(repo, unlocked)
	if len(got) != 1 || got[0].Kind != DiffFlagsMismatch {
		t.Errorf("diffDocs = %+v, want one DiffFlagsMismatch", got)
	}
}

// TestMerge_SkipsAnEntryThatOnlyDIFFERSInFlagNotation: the same canonicalisation
// on the write side, so a converged lockdown group produces no key writes.
func TestMerge_SkipsAnEntryThatOnlyDiffersInFlagNotation(t *testing.T) {
	repo := mustParse(t, "repo", []byte("[G][$i]\nk[$i]=1\n"))
	live := mustParse(t, "live", []byte("[G][$i]\nk=1\n"))

	merged, keys, errs := Merge("live", repo, live)
	// The group is immutable live, so the merge refuses it — but it must refuse
	// with nothing to write, not "refuse AND report a pending change".
	if merged == nil {
		t.Fatal("merged = nil, want the file-level merge to proceed")
	}
	if len(keys) != 0 {
		t.Errorf("keys = %+v, want none", keys)
	}
	if len(errs) != 1 {
		t.Errorf("errs = %v, want the one immutable-group refusal", errs)
	}
}
