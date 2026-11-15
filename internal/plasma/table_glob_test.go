package plasma

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file is the per-glob contract for the governed strip table. table_test.go
// pins WHICH files are managed; this pins WHAT each file's globs are allowed to
// delete.
//
// Every rule in DefaultFiles() is a silent deleter: a match removes a key from
// the exported document with no diagnostic, so a glob that is one character too
// wide loses real user intent and a glob that is one character too narrow leaks
// machine-local or private data into git. Neither shows up in a round-trip test,
// because export compares the repo against the ALREADY-STRIPPED model. The only
// way to catch it is to name, per rule, a key it must delete AND a neighbouring
// key it must not — which is what stripCase does.
//
// Cases are probed through Strip rather than globMatch so they exercise the real
// precedence (group rules, then key rules, then value rules) and the real
// segment-count semantics, not just the string matcher underneath.

// stripCase is one probe: a single entry in a single group of one managed file,
// plus whether the table must delete it. why states the reason the expectation
// is what it is, so a future edit that flips a case has to argue with a sentence
// rather than a boolean.
type stripCase struct {
	group []string
	key   string
	value string
	strip bool
	why   string
}

// probeStrip builds a one-entry document, runs the real Strip for path's spec,
// and reports whether the entry survived.
func probeStrip(t *testing.T, path string, tc stripCase, keepColorScheme bool) bool {
	t.Helper()
	spec := specFor(t, path)
	doc := &Doc{Groups: []Group{{
		Path:    tc.group,
		Entries: []Entry{{Key: tc.key, Value: []byte(tc.value)}},
	}}}
	out := Strip(doc, spec, keepColorScheme)
	for _, g := range out.Groups {
		for _, e := range g.Entries {
			if e.Key == tc.key {
				return true
			}
		}
	}
	return false
}

func runStripCases(t *testing.T, path string, cases []stripCase) {
	t.Helper()
	for _, tc := range cases {
		name := path + " " + groupHeader(tc.group) + "/" + tc.key
		t.Run(name, func(t *testing.T) {
			survived := probeStrip(t, path, tc, false)
			switch {
			case tc.strip && survived:
				t.Errorf("%s survived the strip but must be removed: %s", name, tc.why)
			case !tc.strip && !survived:
				t.Errorf("%s was stripped but must be kept: %s", name, tc.why)
			}
		})
	}
}

// groupHeader renders a decoded group path the way it appears on disk, for test
// names only.
func groupHeader(path []string) string {
	if len(path) == 0 {
		return "<default>"
	}
	return "[" + strings.Join(path, "][") + "]"
}

func TestStripTable_Kdeglobals(t *testing.T) {
	runStripCases(t, "kdeglobals", []stripCase{
		{group: []string{"Colors:Window"}, key: "BackgroundNormal", strip: true,
			why: "derived from [General] ColorScheme by KColorSchemeManager"},
		{group: []string{"Colors:Header", "Inactive"}, key: "BackgroundNormal", strip: true,
			why: "the [Colors:*][*] rule covers the two-segment inactive variants"},
		{group: []string{"ColorEffects:Disabled"}, key: "Color", strip: true,
			why: "derived colour effect block"},
		{group: []string{"WM"}, key: "activeBackground", strip: true,
			why: "window-manager half of the derived palette"},
		{group: []string{"General"}, key: "ColorSchemeHash", strip: true,
			why: "a checksum OF the derived block; keeping it makes KDE trust a stale palette"},
		{group: []string{"General"}, key: "font", strip: true, why: "font metrics are display- and DPI-local"},
		{group: []string{"General"}, key: "fixed", strip: true, why: "font metrics are display- and DPI-local"},
		{group: []string{"General"}, key: "menuFont", strip: true, why: "font metrics are display- and DPI-local"},
		{group: []string{"General"}, key: "smallestReadableFont", strip: true, why: "font metrics are display- and DPI-local"},
		{group: []string{"General"}, key: "toolBarFont", strip: true, why: "font metrics are display- and DPI-local"},
		{group: []string{"General"}, key: "fontStyleName", strip: false,
			why: "the font rules are exact, not prefixes; widening one to font* would swallow neighbours"},
		{group: []string{"General"}, key: "ColorScheme", strip: false,
			why: "names the scheme the derived blocks come from — the one colour key that IS user intent"},
		{group: []string{"General"}, key: "XftAntialias", strip: false, why: "a plain rendering preference"},
		{group: []string{"DirSelect Dialog"}, key: "History Items", strip: true,
			why: "recently visited directories plus dialog geometry"},
		{group: []string{"KFileDialog Settings", "Places"}, key: "IconSize", strip: true,
			why: "the [KFileDialog Settings][*] rule removes the per-sub-dialog geometry sub-groups"},
		{group: []string{"KFileDialog Settings"}, key: "Speedbar Width", strip: true, why: "a pixel width, so display-local"},
		{group: []string{"KFileDialog Settings"}, key: "Recent Files", strip: true, why: "recently used paths are private"},
		{group: []string{"KFileDialog Settings"}, key: "Recent URLs", strip: true, why: "recently used paths are private"},

		// F12 and its audit sibling: two undisclosed over-strips, both now removed.
		{group: []string{"Icons"}, key: "Theme", strip: false,
			why: "F12 — iconssettingsbase.kcfg's only entry; the explicit user icon-theme choice"},
		{group: []string{"Icons", "Panel"}, key: "Size", strip: false,
			why: "F12 — the size sub-groups were always kept, which is why deleting the parent was incoherent"},
		{group: []string{"KFileDialog Settings"}, key: "View Style", strip: false,
			why: "F20 audit — a plain enum written by the same writeViewConfig() as its 13 kept siblings"},
		{group: []string{"KFileDialog Settings"}, key: "Sort by", strip: false,
			why: "sibling of View Style; both are user intent"},
		{group: []string{"KDE"}, key: "widgetStyle", strip: false, why: "the widget style is user intent"},
	})
}

// TestStripTable_KdeglobalsColorGuard pins which kdeglobals rules the §7.4
// colour-scheme guard suspends. The guard exists to protect a hand-rolled
// palette; it must not become a general amnesty that also keeps the dialog and
// directory-history blocks.
func TestStripTable_KdeglobalsColorGuard(t *testing.T) {
	guarded := []stripCase{
		{group: []string{"Colors:Window"}, key: "BackgroundNormal"},
		{group: []string{"Colors:Header", "Inactive"}, key: "BackgroundNormal"},
		{group: []string{"ColorEffects:Disabled"}, key: "Color"},
		{group: []string{"WM"}, key: "activeBackground"},
		{group: []string{"General"}, key: "ColorSchemeHash"},
	}
	for _, tc := range guarded {
		if !probeStrip(t, "kdeglobals", tc, true) {
			t.Errorf("%s/%s must be KEPT when the colour-scheme guard fires", groupHeader(tc.group), tc.key)
		}
	}
	unguarded := []stripCase{
		{group: []string{"DirSelect Dialog"}, key: "History Items"},
		{group: []string{"KFileDialog Settings", "Places"}, key: "IconSize"},
		{group: []string{"General"}, key: "font"},
	}
	for _, tc := range unguarded {
		if probeStrip(t, "kdeglobals", tc, true) {
			t.Errorf("%s/%s must still be stripped when the guard fires; the guard covers the palette only",
				groupHeader(tc.group), tc.key)
		}
	}
}

func TestStripTable_Kwinrc(t *testing.T) {
	// Synthetic UUIDs, per the scheme in testdata/kconfig/README.md. The strip
	// rules key off the SHAPE of a header segment, never off a particular
	// value, so a synthetic UUID exercises them exactly as a real one would —
	// and TestPlasmaTree_NoMachineIdentifiers requires this form.
	const desktopUUID = syntheticUUIDPrefix + "000000000001"
	const outputUUID = syntheticUUIDPrefix + "000000000002"
	runStripCases(t, "kwinrc", []stripCase{
		{group: []string{"Tiling"}, key: "padding", strip: true, why: "the bare group is a layout root"},
		{group: []string{"Tiling", desktopUUID}, key: "tiles", strip: true, why: "keyed by a virtual-desktop UUID"},
		{group: []string{"Tiling", desktopUUID, outputUUID}, key: "tiles", strip: true,
			why: "keyed by a desktop UUID AND an EDID-derived output UUID"},
		{group: []string{"Tiling", desktopUUID, ""}, key: "padding", strip: true,
			why: "KWin's desktop-level sentinel [Tiling][<uuid>][] — empty third segment still matches [Tiling][*][*]"},
		{group: []string{"Tiling", desktopUUID, outputUUID, "extra"}, key: "tiles", strip: false,
			why: "segment counts are exact: a four-segment header is not covered and must be noticed, not silently eaten"},
		{group: []string{"Session: saved at previous logout"}, key: "count", strip: true, why: "per-logout ledger"},
		{group: []string{"SubSession: 1"}, key: "count", strip: true, why: "per-logout ledger"},
		{group: []string{"Screens"}, key: "ScreenBackend", strip: true, why: "enumerates this box's outputs"},
		{group: []string{"Xwayland"}, key: "Scale", strip: true, why: "an Xwayland scale factor is display-local"},
		{group: []string{"Desktops"}, key: "Id_1", strip: true,
			why: "desktop UUIDs; KWin mints a fresh one for any index lacking it"},
		{group: []string{"Desktops"}, key: "Number", strip: false, why: "the desktop COUNT is the portable setting"},
		{group: []string{"Desktops"}, key: "Rows", strip: false, why: "the desktop grid shape is portable"},
		{group: []string{"Effect-blur"}, key: "AnimationTime", strip: true, why: "effect timings are tuned per machine"},
		{group: []string{"Effect-blur"}, key: "NoiseStrength", strip: false,
			why: "the [Effect-*]/*Time* rule must not swallow the rest of an effect's tuning"},
		{group: []string{"Compositing"}, key: "OpenGLIsUnsafe", strip: true, why: "a driver-crash breadcrumb for THIS GPU"},
		{group: []string{"Compositing"}, key: "LatencyPolicy", strip: true, why: "tuned against this machine's latency"},
		{group: []string{"Compositing"}, key: "glCore", strip: true, why: "GPU capability probe result"},
		{group: []string{"Compositing"}, key: "Backend", strip: true, why: "GPU backend probe result"},
		{group: []string{"Compositing"}, key: "AllowTearing", strip: false, why: "a portable compositing preference"},
		{group: []string{"Windows"}, key: "FocusPolicy", strip: false, why: "the headline portable kwin setting"},
		{group: []string{"NightColor"}, key: "Active", strip: false, why: "a portable preference"},
	})
}

func TestStripTable_Kglobalshortcutsrc(t *testing.T) {
	runStripCases(t, "kglobalshortcutsrc", []stripCase{
		{group: []string{"ActivityManager"}, key: "_k_friendly_name", strip: true,
			why: "the group's only real content is an activity-UUID binding"},
		{group: []string{"com.anthropic.Claude"}, key: "_k_friendly_name", strip: true,
			why: "a per-install minted token group; a transplanted entry is a dead binding"},
		{group: []string{"com.notion.app.desktop.notion"}, key: syntheticToken + "-Ctrl+Shift+J", strip: true,
			why: "Notion mints a per-install token into every key name; a transplanted entry is a dead binding"},
		{group: []string{"khotkeys"}, key: "_k_friendly_name", strip: true, why: "khotkeys state is installation-local"},
		{group: []string{"kwin"}, key: "switch-to-activity-" + syntheticUUIDPrefix + "000000000003", strip: true,
			why: "activity UUIDs are generated per installation"},
		{group: []string{"kwin"}, key: "DEADBEEF-Ctrl+Alt+Space", strip: true,
			why: "the per-install token form; the rule is deliberately depth- and app-agnostic"},
		{group: []string{"kwin"}, key: "Window Close", strip: false, why: "an ordinary binding — the bulk of the file"},
		{group: []string{"kwin"}, key: "switch-to-desktop-3", strip: false,
			why: "switch-to-activity-* must not generalise to switch-to-*"},
		{group: []string{"ksmserver"}, key: "Lock Session", strip: false, why: "an ordinary binding"},
	})
}

func TestStripTable_Kcminputrc(t *testing.T) {
	runStripCases(t, "kcminputrc", []stripCase{
		{group: []string{"Keyboard"}, key: "NumLock", strip: true,
			why: "the session rewrites [Keyboard] on layout changes; plasma/recommended re-asserts it"},
		{group: []string{"Mouse"}, key: "cursorSize", strip: true, why: "a pixel size, so display-local"},
		{group: []string{"Mouse"}, key: "cursorTheme", strip: false, why: "the cursor theme is user intent"},
		{group: []string{"Libinput", "1133", "50504", "Logitech USB Receiver Mouse"}, key: "NaturalScroll", strip: false,
			why: "vid/pid/name IS the setting and is inert without that peripheral — conditional, not volatile"},
	})
}

// TestStripTable_Krunnerrc is the F6 regression guard. The removed
// "[General]/history*" glob matched exactly one key on every host —
// historyBehavior — because KRunner's history lives in [PlasmaRunnerManager].
func TestStripTable_Krunnerrc(t *testing.T) {
	runStripCases(t, "krunnerrc", []stripCase{
		{group: []string{"PlasmaRunnerManager"}, key: "history", strip: true,
			why: "F6 — this, not [General], is where KRunner stores typed search history"},
		{group: []string{"PlasmaRunnerManager"}, key: "PluginPersistentStates", strip: true,
			why: "runner-manager state is session-local"},
		{group: []string{"General"}, key: "historyBehavior", strip: false,
			why: "F6 — a krunnersettingsbase.kcfg enum the recommended overlay itself sets"},
		{group: []string{"General"}, key: "FreeFloating", strip: false, why: "a kcfg-declared preference"},
		{group: []string{"General"}, key: "RetainPriorSearch", strip: false, why: "a kcfg-declared preference"},
		{group: []string{"General"}, key: "migratedToKRunner6", strip: true,
			why: "a migration stamp; copying it suppresses a migration the new machine needs"},
	})
}

func TestStripTable_SessionLedgers(t *testing.T) {
	runStripCases(t, "ksmserverrc", []stripCase{
		{group: []string{"Session: saved at previous logout"}, key: "count", strip: true, why: "per-logout ledger"},
		{group: []string{"LegacySession: saved at previous logout"}, key: "count", strip: true, why: "per-logout ledger"},
		{group: []string{"Screen0"}, key: "count", strip: true, why: "per-output session ledger"},
		{group: []string{"General"}, key: "loginMode", strip: false,
			why: "the restore-session policy is the portable ksmserver setting"},
		{group: []string{"General"}, key: "confirmLogout", strip: false, why: "a portable preference"},
	})
}

func TestStripTable_Kwalletrc(t *testing.T) {
	runStripCases(t, "kwalletrc", []stripCase{
		{group: []string{"Auto Allow"}, key: "kdewallet", strip: true,
			why: "an access-control list naming this box's applications"},
		{group: []string{"Auto Deny"}, key: "kdewallet", strip: true, why: "an access-control list"},
		{group: []string{"org.freedesktop.secrets"}, key: "apiEnabled", strip: true,
			why: "Secret Service exposure is decided per machine"},
		{group: []string{"Wallet"}, key: "First Use", strip: true, why: "a first-run stamp"},
		{group: []string{"Wallet"}, key: "Default Wallet", strip: true, why: "names a wallet that exists only on this box"},
		{group: []string{"Wallet"}, key: "Enabled", strip: false, why: "whether the wallet subsystem runs is user intent"},
	})
}

func TestStripTable_WindowGeometryFiles(t *testing.T) {
	runStripCases(t, "dolphinrc", []stripCase{
		{group: []string{"General"}, key: "Version", strip: true, why: "a schema stamp"},
		{group: []string{"General"}, key: "ViewPropsTimestamp", strip: true, why: "a schema stamp"},
		{group: []string{"General"}, key: "FilterBar", strip: false, why: "a portable preference"},
		{group: []string{"PreviewSettings"}, key: "Plugins", strip: true,
			why: "enumerates the thumbnailers installed on THIS box"},
		{group: []string{"MainWindow"}, key: "State", strip: true, why: "a toolbar-layout byte blob"},
		{group: []string{"MainWindow"}, key: "ToolBarsMovable", strip: true, why: "written alongside State"},
		{group: []string{"MainWindow"}, key: "3840x2160 screen: Height", strip: true, why: "resolution-keyed geometry"},
		{group: []string{"MainWindow"}, key: "3840x2160 screen: Width", strip: true, why: "resolution-keyed geometry"},
		{group: []string{"MainWindow"}, key: "MenuBar", strip: false,
			why: "the geometry rules are key-scoped precisely so this survives"},
		{group: []string{"MainWindow", "Toolbar mainToolBar"}, key: "ToolButtonStyle", strip: true,
			why: "per-toolbar layout state"},
		{group: []string{"Search"}, key: "Location", strip: true, why: "last search scope"},
		{group: []string{"Places"}, key: "Recent Files", strip: true, why: "the [*]/Recent * rule is depth-agnostic"},
		{group: []string{"KFileDialog Settings"}, key: "Speedbar Width", strip: true, why: "a pixel width"},
		{group: []string{"DetailsMode"}, key: "PreviewSize", strip: false, why: "a portable view preference"},
	})
	runStripCases(t, "konsolerc", []stripCase{
		{group: []string{"General"}, key: "ConfigVersion", strip: true, why: "a schema stamp"},
		{group: []string{"MainWindow"}, key: "State", strip: true, why: "a toolbar-layout byte blob"},
		{group: []string{"MainWindow"}, key: "3840x2160 screen: Width", strip: true, why: "resolution-keyed geometry"},
		{group: []string{"MainWindow", "Toolbar sessionToolbar"}, key: "IconText", strip: true, why: "per-toolbar state"},
		{group: []string{"Favorite Profiles"}, key: "Favorites", strip: true,
			why: "names profiles under ~/.local/share, which the engine never carries"},
		{group: []string{"UiSettings"}, key: "ColorScheme", strip: false, why: "a portable preference"},
		{group: []string{"Desktop Entry"}, key: "DefaultProfile", strip: false, why: "a portable preference"},
	})
	runStripCases(t, "katerc", []stripCase{
		{group: []string{"MainWindow"}, key: "3840x2160 screen: Height", strip: true,
			why: "kate's [MainWindow] is geometry and toolbar state only"},
		{group: []string{"Recent Files"}, key: "Files", strip: true, why: "recently opened paths are private"},
		{group: []string{"Session Data"}, key: "Documents", strip: true, why: "open-document paths are private"},
		{group: []string{"KTextEditor Renderer"}, key: "Color Theme", strip: true, why: "renderer state"},
		{group: []string{"Restore Documents"}, key: "Count", strip: true, why: "per-session document ledger"},
		{group: []string{"General"}, key: "Last Session", strip: true, why: "a session pointer"},
		{group: []string{"General"}, key: "PinnedDocuments", strip: true, why: "document paths are private"},
		{group: []string{"General"}, key: "Show Menu Bar", strip: false, why: "a portable preference"},
		{group: []string{"filetree"}, key: "3840x2160 screen: Width", strip: true,
			why: "the geometry rule is depth-agnostic, so it reaches non-[MainWindow] groups"},
		{group: []string{"filetree"}, key: "listMode", strip: false, why: "a portable preference"},
	})
	runStripCases(t, "katevirc", []stripCase{
		{group: []string{"Kate Vi Input Mode Settings"}, key: "Macro Contents", strip: true, why: "recorded macro state"},
		{group: []string{"Kate Vi Input Mode Settings"}, key: "Macro Registers", strip: true, why: "recorded macro state"},
		{group: []string{"Kate Vi Input Mode Settings"}, key: "Macro Completions", strip: true, why: "recorded macro state"},
		{group: []string{"Kate Vi Input Mode Settings"}, key: "Normal Mode Mappings", strip: false,
			why: "hand-authored vi mappings are the reason this file is managed"},
	})
}

func TestStripTable_NotifyAndBaloo(t *testing.T) {
	runStripCases(t, "plasmanotifyrc", []stripCase{
		{group: []string{"DoNotDisturb"}, key: "Until", strip: true, why: "a transient timestamp"},
		{group: []string{"Applications", "org.mozilla.thunderbird"}, key: "Seen", strip: true,
			why: "records that THIS install has seen the app once"},
		{group: []string{"Applications", "org.mozilla.thunderbird"}, key: "ShowPopups", strip: false,
			why: "a per-application notification preference is user intent"},
		{group: []string{"Services", "kwin"}, key: "Seen", strip: true, why: "a first-sighting stamp"},
	})
	runStripCases(t, "baloofilerc", []stripCase{
		{group: []string{"General"}, key: "dbVersion", strip: true, why: "an index schema stamp"},
		{group: []string{"General"}, key: "index version", strip: true, why: "an index schema stamp"},
		{group: []string{"General"}, key: "Indexing-Enabled", strip: false,
			why: "the headline baloo setting the recommended overlay writes"},
		{group: []string{"General"}, key: "exclude folders", strip: false,
			why: "the tuned exclude list is the point of the recommended overlay"},
	})
}

// TestStripTable_Plasmarc pins the one plasmarc rule. [Wallpapers]
// usersWallpapers is the image wallpaper plugin's list of ABSOLUTE paths to
// images the user picked through "Add Image…", so it names this machine's home
// directory and points at files no other machine has. It is the only plasmarc
// key that is not a portable preference, which is why the rule is a key rule
// rather than a group rule.
func TestStripTable_Plasmarc(t *testing.T) {
	runStripCases(t, "plasmarc", []stripCase{
		{group: []string{"Wallpapers"}, key: "usersWallpapers",
			value: "/home/someone/Pictures/wallpapers/a.png", strip: true,
			why: "absolute paths into this machine's home; the §14.1 privacy scan rejects the export otherwise"},
		{group: []string{"PlasmaToolTips"}, key: "Delay", strip: false,
			why: "a plain portable preference and the reason plasmarc is managed at all"},
		{group: []string{"Theme"}, key: "name", strip: false,
			why: "the Plasma style is user intent and must survive"},
	})
}

func TestStripTable_MimeappsList(t *testing.T) {
	runStripCases(t, "mimeapps.list", []stripCase{
		{group: []string{"Removed Associations"}, key: "text/html", value: "firefox.desktop", strip: true,
			why: "negative associations describe what was uninstalled HERE"},
		{group: []string{"Default Applications"}, key: "x-scheme-handler/mailto",
			value: "userapp-Thunderbird-GUF4N3.desktop", strip: true,
			why: "Thunderbird mints a fresh random suffix per machine"},
		{group: []string{"Default Applications"}, key: "x-scheme-handler/webcals",
			value: "userapp-Thunderbird-ELF5N3.desktop", strip: true, why: "the webcal* key glob covers webcals"},
		{group: []string{"Default Applications"}, key: "text/html", value: "google-chrome.desktop", strip: true,
			why: "the default browser is host-local; google-chrome has no aarch64 Linux build"},

		// F20 audit — the under-strip. [Added Associations] values are
		// semicolon-terminated LISTS, which an anchored "=userapp-*.desktop"
		// glob never matched.
		{group: []string{"Added Associations"}, key: "x-scheme-handler/mailto",
			value: "userapp-Thunderbird-GUF4N3.desktop;", strip: true,
			why: "F20 — the list form must be caught by the value rule, not only by the enumerated key names"},
		{group: []string{"Added Associations"}, key: "text/calendar",
			value: "userapp-Thunderbird-ELF5N3.desktop;", strip: true,
			why: "F20 — text/calendar is NOT in the [Added Associations] key list, so only the value rule can catch it"},
		{group: []string{"Added Associations"}, key: "message/rfc822",
			value: "org.kde.kmail2.desktop;userapp-Thunderbird-GUF4N3.desktop;", strip: true,
			why: "F20 — a userapp id anywhere in the list makes the whole entry unportable"},
		{group: []string{"Added Associations"}, key: "image/webp",
			value: "com.github.PintaProject.Pinta.desktop;", strip: false,
			why: "an ordinary list-valued association must survive"},
		{group: []string{"Default Applications"}, key: "x-scheme-handler/claude",
			value: "com.anthropic.Claude.desktop", strip: false,
			why: "a normal desktop id that merely contains no userapp- prefix"},
	})
}

// TestStripTable_LeavesUnruledFilesAlone pins the files that carry NO strip
// rules. An accidental rule added to one of them would silently start deleting
// settings from a file whose whole content is user intent.
func TestStripTable_LeavesUnruledFilesAlone(t *testing.T) {
	unruled := []string{
		"kdedefaults/kdeglobals", "kdedefaults/kwinrc", "kdedefaults/kcminputrc",
		"kdedefaults/ksplashrc", "kdedefaults/plasmarc",
		"kiorc", "kded5rc", "kscreenlockerrc", "powerdevilrc", "plasma-localerc", "klipperrc",
	}
	for _, path := range unruled {
		spec := specFor(t, path)
		if n := len(spec.VolatileGroups) + len(spec.VolatileKeys) + len(spec.VolatileValues); n != 0 {
			t.Errorf("%s declares %d strip rules; it is listed as carrying none", path, n)
		}
	}
}

// TestStripTable_MustNotManage fences the five paths the classification audit
// ruled out of DefaultFiles(). Panel and applet layout can only be applied
// wholesale (spec §16.1) and the other three are machine identity, so a future
// table edit that adds one must fail here rather than on a user's desktop.
func TestStripTable_MustNotManage(t *testing.T) {
	forbidden := []struct {
		path string
		why  string
	}{
		{"plasmashellrc", "installation-local containment instance IDs, cross-referenced from the applets file"},
		{"plasma-org.kde.plasma.desktop-appletsrc", "instance IDs, activity UUIDs and ItemGeometries-<resolution>"},
		{"kactivitymanagerdrc", "the per-installation activity UUID"},
		{"kactivitymanagerd-statsrc", "the per-installation activity UUID plus usage statistics"},
		{"kwinoutputconfig.json", "EDID hash, panel serial and output UUID"},
	}
	managed := make(map[string]bool)
	for _, spec := range DefaultFiles() {
		managed[spec.Path] = true
	}
	for _, f := range forbidden {
		if managed[f.path] {
			t.Errorf("%s is in DefaultFiles() but must never be managed: %s", f.path, f.why)
		}
	}
	// The two activity files additionally carry the hard fence, so a glob
	// widening cannot reach them either.
	for _, path := range []string{"kactivitymanagerdrc", "kactivitymanagerd-statsrc", "kwinoutputconfig.json"} {
		if !denylisted(path) {
			t.Errorf("%s must be denylisted: machine identity is fenced, not merely unlisted", path)
		}
	}
}

// TestStripTable_DoesNotStripRecommendedKeys is the cross-check between the two
// halves of the feature: every key plasma/recommended/ writes is a key the
// project has decided a user should HAVE, so a strip rule that deletes one means
// apply --recommended writes a setting the next export throws away. Two such
// pairs are deliberate and documented in plasma/recommended/README.md; they are
// named here so that a THIRD one fails the build.
func TestStripTable_DoesNotStripRecommendedKeys(t *testing.T) {
	type groupKey struct{ group, key string }
	documented := map[string]map[groupKey]string{
		"kcminputrc": {
			{"[Keyboard]", "NumLock"}:     "README: [Keyboard] is session-rewritten; the overlay re-asserts it",
			{"[Keyboard]", "RepeatDelay"}: "README: [Keyboard] is session-rewritten; the overlay re-asserts it",
			{"[Keyboard]", "RepeatRate"}:  "README: [Keyboard] is session-rewritten; the overlay re-asserts it",
		},
		"kwinrc": {
			{"[Xwayland]", "XwaylandEavesdrops"}: "README: [Xwayland] also holds the display-local Scale",
		},
	}

	dir := filepath.Join("..", "..", "plasma", "recommended")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		rel := entry.Name()
		abs := filepath.Join(dir, rel)
		data, err := os.ReadFile(abs) //nolint:gosec // fixed, repo-relative test data
		if err != nil {
			t.Fatalf("read %s: %v", abs, err)
		}
		doc, err := Parse(abs, data)
		if err != nil {
			t.Fatalf("parse %s: %v", abs, err)
		}
		spec := specFor(t, rel)
		stripped := Strip(doc, spec, false)

		survived := make(map[groupKey]bool)
		for _, g := range stripped.Groups {
			for _, e := range g.Entries {
				survived[groupKey{groupHeader(g.Path), e.Key}] = true
			}
		}
		for _, g := range doc.Groups {
			for _, e := range g.Entries {
				gk := groupKey{groupHeader(g.Path), e.Key}
				if survived[gk] {
					continue
				}
				if why, ok := documented[rel][gk]; ok {
					t.Logf("%s %s/%s: stripped by design — %s", rel, gk.group, gk.key, why)
					continue
				}
				t.Errorf("plasma/recommended/%s sets %s/%s but the strip table deletes it; "+
					"apply --recommended would write a value the next export discards",
					rel, gk.group, gk.key)
			}
		}
	}
}
