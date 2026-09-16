package plasma

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		{"Tiling", "Tiling", true},
		{"Tiling", "TilingX", false},
		{"*", "", true},
		{"*", "anything", true},
		{"Id_*", "Id_1", true},
		{"Id_*", "Id_", true},
		{"Id_*", "Number", false},
		{"*Time*", "AnimationTime", true},
		{"*Time*", "TimeLine", true},
		{"*Time*", "Duration", false},
		{"Session: *", "Session: saved at previous logout", true},
		{"Session: *", "Session", false},
		{"Colors:*", "Colors:Header", true},
		{"Colors:*", "ColorEffects:Inactive", false},
		{"Session*", "Session: saved", true},
		{"*-Ctrl+Alt+Space", "DEADBEEF-Ctrl+Alt+Space", true},
		{"* screen: *", "Window Width screen: 3840x2160 total", true},
		{"* screen: *", "Window Width", false},
		{"a*b*c", "axxbyyc", true},
		{"a*b*c", "axxbyy", false},
	}
	for _, tc := range tests {
		if got := globMatch(tc.pattern, tc.s); got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pattern, tc.s, got, tc.want)
		}
	}
}

// TestGroupGlob_SegmentCount is the rule most likely to be got wrong: a pattern
// with N bracket pairs matches ONLY groups with exactly N segments. "[Tiling]"
// must not reach [Tiling][uuid], and "[Tiling][*]" must not reach the bare
// parent — each depth is listed separately in the table on purpose.
func TestGroupGlob_SegmentCount(t *testing.T) {
	tests := []struct {
		pattern string
		group   []string
		want    bool
	}{
		{"[Tiling]", []string{"Tiling"}, true},
		{"[Tiling]", []string{"Tiling", "uuid"}, false},
		{"[Tiling][*]", []string{"Tiling"}, false},
		{"[Tiling][*]", []string{"Tiling", "uuid"}, true},
		{"[Tiling][*]", []string{"Tiling", "uuid", "out"}, false},
		{"[Tiling][*][*]", []string{"Tiling", "uuid", "out"}, true},
		{"[Colors:*]", []string{"Colors:Header"}, true},
		{"[Colors:*]", []string{"Colors:Header", "Inactive"}, false},
		{"[Colors:*][*]", []string{"Colors:Header", "Inactive"}, true},
		{"[MainWindow][*]", []string{"MainWindow", "Toolbar"}, true},
		{"[MainWindow][*]", []string{"MainWindow"}, false},
	}
	for _, tc := range tests {
		rule, ok := parseGroupRule(tc.pattern)
		if !ok {
			t.Fatalf("parseGroupRule(%q) failed", tc.pattern)
		}
		if got := rule.matchesGroup(tc.group); got != tc.want {
			t.Errorf("%q.matchesGroup(%v) = %v, want %v", tc.pattern, tc.group, got, tc.want)
		}
	}
}

// TestKeyGlob_AnyDepthWildcard covers the one special form: "[*]" as a key
// rule's group matcher means any group at any depth, not any one-segment group.
func TestKeyGlob_AnyDepthWildcard(t *testing.T) {
	rule, ok := parseKeyRule("[*]/Recent *")
	if !ok {
		t.Fatal("parseKeyRule failed")
	}
	if !rule.anyDepth {
		t.Fatal("[*]/… must set anyDepth")
	}
	for _, group := range [][]string{nil, {"General"}, {"A", "B", "C"}} {
		if !rule.matchesGroup(group) {
			t.Errorf("anyDepth rule did not match %v", group)
		}
	}
}

// TestKeyGlob_SlashInKeyName proves the split is at the first '/' AFTER the
// bracket run: mimeapps.list keys legitimately contain slashes.
func TestKeyGlob_SlashInKeyName(t *testing.T) {
	rule, ok := parseKeyRule("[Default Applications]/x-scheme-handler/mailto")
	if !ok {
		t.Fatal("parseKeyRule failed")
	}
	if rule.key != "x-scheme-handler/mailto" {
		t.Errorf("key glob = %q, want %q", rule.key, "x-scheme-handler/mailto")
	}
	if !rule.matchesGroup([]string{"Default Applications"}) {
		t.Error("group matcher did not match [Default Applications]")
	}
}

func TestParseRules_Rejections(t *testing.T) {
	badGroups := []string{"", "Tiling", "[Tiling", "[Tiling]/x"}
	for _, pattern := range badGroups {
		if _, ok := parseGroupRule(pattern); ok {
			t.Errorf("parseGroupRule(%q) unexpectedly succeeded", pattern)
		}
	}
	badKeys := []string{"[G]", "[G]/", "G/key", "[G/key"}
	for _, pattern := range badKeys {
		if _, ok := parseKeyRule(pattern); ok {
			t.Errorf("parseKeyRule(%q) unexpectedly succeeded", pattern)
		}
	}
	badValues := []string{"[G]/key", "[G]/=v", "[G]"}
	for _, pattern := range badValues {
		if _, ok := parseValueRule(pattern); ok {
			t.Errorf("parseValueRule(%q) unexpectedly succeeded", pattern)
		}
	}
}

// TestStrip_GroupsThenKeysThenValues walks the real kwinrc rules over a
// document holding the shapes the audit found, including the two-UUID nested
// [Tiling] headers.
func TestStrip_GroupsThenKeysThenValues(t *testing.T) {
	spec := specFor(t, "kwinrc")
	raw := strings.Join([]string{
		"[Desktops]",
		"Id_1=00000000-0000-4000-8000-000000000001",
		"Id_2=00000000-0000-4000-8000-000000000002",
		"Number=10",
		"Rows=1",
		"",
		"[Tiling]",
		"padding=4",
		"",
		"[Tiling][00000000-0000-4000-8000-000000000001]",
		"tiles={\"layoutDirection\":\"horizontal\"}",
		"",
		"[Tiling][00000000-0000-4000-8000-000000000001][00000000-0000-4000-8000-000000000002]",
		"tiles={\"width\":0.25}",
		"",
		"[Tiling][00000000-0000-4000-8000-000000000001][]",
		"padding=4",
		"",
		"[Windows]",
		"PerOutputVirtualDesktops=true",
		"",
		"[Xwayland]",
		"Scale=1",
		"",
	}, "\n")

	got := Strip(mustParse(t, "kwinrc", []byte(raw)), spec, false)

	if g := findGroup(got, []string{"Tiling"}); g != nil {
		t.Error("[Tiling] survived the group rule")
	}
	if g := findGroup(got, []string{"Tiling", "00000000-0000-4000-8000-000000000001"}); g != nil {
		t.Error("[Tiling][uuid] survived the group rule")
	}
	if g := findGroup(got, []string{"Tiling",
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002"}); g != nil {
		t.Error("[Tiling][uuid][uuid] survived the group rule")
	}
	if g := findGroup(got, []string{"Tiling",
		"00000000-0000-4000-8000-000000000001", ""}); g != nil {
		t.Error("[Tiling][uuid][] survived the group rule")
	}
	if g := findGroup(got, []string{"Xwayland"}); g != nil {
		t.Error("[Xwayland] survived the group rule")
	}

	desktops := findGroup(got, []string{"Desktops"})
	if desktops == nil {
		t.Fatal("[Desktops] was removed; only its Id_* keys are volatile")
	}
	if findEntry(desktops, "Id_1", "") != nil || findEntry(desktops, "Id_2", "") != nil {
		t.Error("Id_* keys survived the key rule")
	}
	// The portable intent must survive intact.
	for _, key := range []string{"Number", "Rows"} {
		if findEntry(desktops, key, "") == nil {
			t.Errorf("[Desktops] %s was stripped; it is portable intent", key)
		}
	}
	if findEntry(findGroup(got, []string{"Windows"}), "PerOutputVirtualDesktops", "") == nil {
		t.Error("[Windows] PerOutputVirtualDesktops was stripped")
	}
}

// TestStrip_FlagAndLocaleNormalisation proves globs match the DECODED BARE key:
// the pattern "Speedbar Width" strips the on-disk entry "Speedbar Width[$e]",
// and the pattern "Recent *" strips "Recent Files[fr]" — a flag suffix and a
// locale tag are both invisible to the matcher, because the parser has already
// lifted them out of Key.
//
// The vehicle used to be "View Style[$e]"; that key stopped being a rule when
// the F20 audit found it was a plain enum written by the same
// KFileWidget::writeViewConfig() call as its kept siblings. The contract under
// test is unchanged and now also covers the locale half of its own name, so
// View Style appears below as a must-survive control instead.
func TestStrip_FlagAndLocaleNormalisation(t *testing.T) {
	spec := specFor(t, "kdeglobals")
	raw := strings.Join([]string{
		"[KFileDialog Settings]",
		"Speedbar Width[$e]=154",
		"Recent Files[fr]=/tmp/rapport.odt",
		"Automatically select filename extension=true",
		"View Style=DetailTree",
		"",
	}, "\n")

	got := Strip(mustParse(t, "kdeglobals", []byte(raw)), spec, false)
	g := findGroup(got, []string{"KFileDialog Settings"})
	if g == nil {
		t.Fatal("[KFileDialog Settings] was removed; only some of its keys are volatile")
	}
	if findEntry(g, "Speedbar Width", "") != nil {
		t.Error(`"Speedbar Width[$e]" survived the "Speedbar Width" key rule`)
	}
	if findEntry(g, "Recent Files", "fr") != nil {
		t.Error(`"Recent Files[fr]" survived the "Recent *" key rule`)
	}
	if findEntry(g, "Automatically select filename extension", "") == nil {
		t.Error("a non-volatile key in the same group was stripped")
	}
	if findEntry(g, "View Style", "") == nil {
		t.Error(`"View Style" is no longer a volatile key and must survive`)
	}
}

// TestStrip_ValueGlob covers the mimeapps.list escape hatch: Thunderbird mints
// a fresh random suffix per machine, so only a value rule can catch it.
//
// [Added Associations] is the harder half. Its values are semicolon-terminated
// desktop-id LISTS, so an anchored "=userapp-*.desktop" glob matched none of
// them and the leak survived on every key the enumerated list above happened
// not to name. The rule matches the substring for exactly that reason.
func TestStrip_ValueGlob(t *testing.T) {
	spec := specFor(t, "mimeapps.list")
	raw := strings.Join([]string{
		"[Default Applications]",
		"application/pdf=org.kde.okular.desktop",
		"text/html=userapp-Thunderbird-GUF4N3.desktop",
		"",
		"[Added Associations]",
		"image/webp=com.github.PintaProject.Pinta.desktop;",
		"text/calendar=userapp-Thunderbird-ELF5N3.desktop;",
		"message/rfc822=org.kde.kmail2.desktop;userapp-Thunderbird-GUF4N3.desktop;",
		"",
	}, "\n")

	got := Strip(mustParse(t, "mimeapps.list", []byte(raw)), spec, false)
	g := findGroup(got, []string{"Default Applications"})
	if g == nil {
		t.Fatal("[Default Applications] was removed")
	}
	if findEntry(g, "text/html", "") != nil {
		t.Error("a userapp-*.desktop value survived the value rule")
	}
	if findEntry(g, "application/pdf", "") == nil {
		t.Error("a normal handler was stripped by the value rule")
	}

	added := findGroup(got, []string{"Added Associations"})
	if added == nil {
		t.Fatal("[Added Associations] was removed; only some of its keys are volatile")
	}
	if findEntry(added, "text/calendar", "") != nil {
		t.Error("a single-element userapp list survived the value rule")
	}
	if findEntry(added, "message/rfc822", "") != nil {
		t.Error("a userapp id in a NON-leading list position survived the value rule")
	}
	if findEntry(added, "image/webp", "") == nil {
		t.Error("an ordinary list-valued association was stripped by the value rule")
	}
}

// TestStrip_GroupRuleBeatsKeyRule pins the ordering of §7.2: a group removed
// wholesale never has its keys considered, so a surviving key inside it is
// impossible regardless of the key rules.
func TestStrip_GroupRuleBeatsKeyRule(t *testing.T) {
	spec := FileSpec{
		Path:           "test",
		VolatileGroups: []string{"[Gone]"},
		VolatileKeys:   []string{"[*]/keep"},
	}
	raw := "[Gone]\nkeep=1\nother=2\n\n[Stay]\nkeep=3\nother=4\n"

	got := Strip(mustParse(t, "test", []byte(raw)), spec, false)
	if findGroup(got, []string{"Gone"}) != nil {
		t.Error("[Gone] survived")
	}
	stay := findGroup(got, []string{"Stay"})
	if stay == nil {
		t.Fatal("[Stay] was removed")
	}
	if findEntry(stay, "keep", "") != nil {
		t.Error("the key rule did not apply to a surviving group")
	}
	if findEntry(stay, "other", "") == nil {
		t.Error("a non-matching key was stripped")
	}
}

// TestStrip_LeavesInputUntouched: Strip returns a new document. An in-place
// strip would corrupt the caller's live model, which diff still needs.
func TestStrip_LeavesInputUntouched(t *testing.T) {
	spec := specFor(t, "kwinrc")
	doc := mustParse(t, "kwinrc", []byte("[Tiling]\npadding=4\n\n[Windows]\nx=1\n"))
	before := countEntries(doc)

	_ = Strip(doc, spec, false)
	if got := countEntries(doc); got != before {
		t.Errorf("Strip mutated its input: %d entries, want %d", got, before)
	}
}

// TestColorSchemeGuard covers judgement call 1 in both directions: with a stock
// scheme the derived colour block is stripped, and with a scheme that has no
// .colors file anywhere the block survives and an advisory fires — that is the
// signature of a hand-rolled palette living only inside kdeglobals.
func TestColorSchemeGuard(t *testing.T) {
	const kdeglobals = "[Colors:Header]\nBackgroundNormal=27,30,32\n\n" +
		"[General]\nColorScheme=MyScheme\nColorSchemeHash=abc\nwidgetStyle=Breeze\n\n" +
		"[WM]\nactiveBackground=49,54,59\n"

	t.Run("no .colors file keeps the block", func(t *testing.T) {
		e, _, configDir := testEngine(t)
		seedLive(t, configDir, "kdeglobals", kdeglobals)

		keep, advisory := e.colorSchemeGuard()
		if !keep {
			t.Fatal("guard did not fire for a scheme with no .colors file")
		}
		if !strings.Contains(advisory, "MyScheme") {
			t.Errorf("advisory = %q, want it to name the scheme", advisory)
		}

		got := Strip(mustParse(t, "kdeglobals", []byte(kdeglobals)), specFor(t, "kdeglobals"), keep)
		if findGroup(got, []string{"Colors:Header"}) == nil {
			t.Error("[Colors:Header] was stripped despite the guard")
		}
		if findGroup(got, []string{"WM"}) == nil {
			t.Error("[WM] was stripped despite the guard")
		}
		if findEntry(findGroup(got, []string{"General"}), "ColorSchemeHash", "") == nil {
			t.Error("ColorSchemeHash was stripped despite the guard")
		}
	})

	t.Run("matching .colors file strips the block", func(t *testing.T) {
		e, _, configDir := testEngine(t)
		seedLive(t, configDir, "kdeglobals", kdeglobals)
		schemes := filepath.Join(e.dataHome, "color-schemes")
		if err := os.MkdirAll(schemes, 0o755); err != nil {
			t.Fatalf("MkdirAll error = %v", err)
		}
		writeFile(t, filepath.Join(schemes, "MyScheme.colors"), "[ColorEffects:Disabled]\n")

		keep, advisory := e.colorSchemeGuard()
		if keep {
			t.Fatal("guard fired even though the scheme has a .colors file")
		}
		if advisory != "" {
			t.Errorf("advisory = %q, want empty", advisory)
		}

		got := Strip(mustParse(t, "kdeglobals", []byte(kdeglobals)), specFor(t, "kdeglobals"), keep)
		if findGroup(got, []string{"Colors:Header"}) != nil {
			t.Error("[Colors:Header] survived the unguarded strip")
		}
		if findEntry(findGroup(got, []string{"General"}), "ColorSchemeHash", "") != nil {
			t.Error("ColorSchemeHash survived the unguarded strip")
		}
		if findEntry(findGroup(got, []string{"General"}), "widgetStyle", "") == nil {
			t.Error("a non-colour [General] key was stripped")
		}
	})

	t.Run("kdedefaults overrides the fallback", func(t *testing.T) {
		e, _, configDir := testEngine(t)
		seedLive(t, configDir, "kdeglobals", "[General]\nColorScheme=Fallback\n")
		seedLive(t, configDir, "kdedefaults/kdeglobals", "[General]\nColorScheme=Overlay\n")

		if got := e.activeColorScheme(); got != "Overlay" {
			t.Errorf("activeColorScheme() = %q, want %q", got, "Overlay")
		}
	})

	t.Run("no scheme named means no palette to protect", func(t *testing.T) {
		e, _, configDir := testEngine(t)
		seedLive(t, configDir, "kdeglobals", "[General]\nwidgetStyle=Breeze\n")

		if keep, _ := e.colorSchemeGuard(); keep {
			t.Error("guard fired with no scheme named")
		}
	})
}

// TestColorSchemeGuard_OnlyGuardsKdeglobals: the guard must not accidentally
// widen to protect unrelated rules or other files.
func TestColorSchemeGuard_OnlyGuardsKdeglobals(t *testing.T) {
	if isColorSchemeRule("kdedefaults/kdeglobals", "[Colors:*]") {
		t.Error("the guard reaches kdedefaults/kdeglobals, which only NAMES the scheme")
	}
	if isColorSchemeRule("kdeglobals", "[Icons]") {
		t.Error("the guard protects [Icons], which is not a derived colour block")
	}
	if !isColorSchemeRule("kdeglobals", "[General]/ColorSchemeHash") {
		t.Error("the guard does not protect ColorSchemeHash")
	}
}
