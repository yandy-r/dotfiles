# Recommended Plasma Overlay

Hand-authored, opt-in KDE Plasma 6 settings: **40 settings across 10 files**.

Nothing here is applied unless you ask for it:

```bash
dotf plasma diff  --recommended     # what would change
dotf plasma apply --recommended --dry-run
dotf plasma apply --recommended --backup
```

## How this differs from `../files/`

`../files/` is a snapshot of what your machine currently **is** — written by
`dotf plasma export`, never by hand. This directory is what a good KDE
**should be** in the maintainer's opinion — written by hand, never by `export`.
Keeping them apart is what stops "my machine drifted" and "I changed my mind
about a default" from blurring together in git history.

Each file here is a **partial overlay**, not a whole config. It carries only the
keys listed below. Precedence when `--recommended` is passed:

```text
live  <  plasma/files/<path>  <  plasma/recommended/<path>
```

Live-only keys are still preserved. `--recommended` changes what gets asserted,
never what gets deleted.

## Adopting a subset

There is no per-setting flag. To take some of this and not the rest, delete the
lines you do not want from the file in this directory (or delete the whole file)
and commit that. This directory is yours to edit; `export` will never overwrite
it.

If you instead adopt a value through System Settings, the next `dotf plasma
export` captures it into `../files/`, where it becomes redundant-but-harmless —
both sides then agree. That holds for 36 of the 40 settings below. It does
**not** hold for the four listed in the next section, and knowing which is
which matters.

## The four apply-only settings

`export` filters the live desktop through the governed strip table in
`internal/plasma/table.go`. Anything that table calls volatile — a whole group,
a key glob, or a value glob — never reaches `../files/`. Four of the settings
below sit inside a volatile **group**, and a group rule cannot spare the
intent-bearing keys inside it. They are therefore **apply-only**:

- `dotf plasma apply --recommended` writes them, and they work.
- `dotf plasma export` can never capture them, so they will never appear in
  `../files/`.
- The "adopt it in System Settings, then export" path above silently does
  nothing for them: the value lands live, the export drops it, and the repo
  ends up with no record that you chose anything.
- This directory is consequently the **only** place they are written down. Delete
  the line here and the setting is gone from the repo entirely — there is no
  `../files/` copy to fall back on.

This is the complete list. It is not a sample:

| Setting | Volatile rule that removes it on export | Why the rule is right anyway |
| --- | --- | --- |
| `kwinrc [Xwayland] XwaylandEavesdrops` | `[Xwayland]` volatile **group** | The rest of `[Xwayland]` is the display-local `Scale`, which must not be committed. Splitting the group would mean enumerating KWin's Xwayland keys and re-checking them every release |
| `kcminputrc [Keyboard] NumLock` | `[Keyboard]` volatile **group** | The running session rewrites `[Keyboard]` as a unit — both `libkwin.so.6` and the kded `keyboard` module read and write it — so a captured copy is session state as often as it is intent |
| `kcminputrc [Keyboard] RepeatDelay` | `[Keyboard]` volatile **group** | as above |
| `kcminputrc [Keyboard] RepeatRate` | `[Keyboard]` volatile **group** | as above |

Everything else in this directory round-trips. `krunnerrc [General]
historyBehavior` was a fifth entry until the strip table's `[General]/history*`
key glob was narrowed to `[General]/migrated*`: that glob matched nothing except
this one genuine preference, while the KRunner search history it was aimed at is
removed by the `[PlasmaRunnerManager]` group rule instead. A key glob could be
narrowed; the two group rules above cannot be, without giving up what they are
for.

The split is enforced, not merely asserted.
`TestStripTable_DoesNotStripRecommendedKeys` re-derives this list from the
committed overlay and the live strip table on every run, names these four as the
documented exceptions, and **fails the build on a fifth** — so this section
cannot quietly go stale, in either direction.

## Reading the tables

- **Risk / annoyance** is the honest downside, not a disclaimer. Read it.
- **(taste)** marks a setting that is a preference, not an improvement. Someone
  reasonable would set the opposite.
- **(lower confidence)** marks a value that could not be verified end to end against
  the shipped schemas and binaries on the audited Plasma 6.7.4 host. The row says
  which part is uncertain — usually the runtime behaviour rather than the spelling.

---

## `baloofilerc` — file indexing

Six settings. The theme is: keep file search working, stop indexing a developer
home's worth of build output and toolchain caches.

| Group / key | Value | Why | Risk / annoyance |
| --- | --- | --- | --- |
| `[Basic Settings] Indexing-Enabled` | `true` | Keeps Baloo on so Dolphin and KRunner file search, tags, and ratings still work. The narrowing happens in the keys below rather than by switching indexing off | None if you want search at all. Set `false` only if you live entirely in `fd`/`rg`/`fzf` — then Dolphin's search box and KRunner file results go dead |
| `[General] only basic indexing` | `true` | Indexes filenames and basic filesystem metadata but skips content extraction. The audited host had 653,134 files indexed **with** content extraction — the single largest Baloo CPU, IO, and disk cost on a dev workstation | A real tradeoff, not free: you lose full-text search inside documents and source from Dolphin and KRunner. Filename search still works |
| `[General] folders[$e]` | `$HOME/` | Anchors the index root at the home directory. The `[$e]` suffix is KConfig shell expansion, resolved at read time — that flag is what keeps this portable instead of hardcoding `/home/<you>` | None. This is already the default; it is written explicitly so the overlay is self-documenting and machine-independent |
| `[General] exclude folders[$e]` | 28 `$HOME/…/` prefixes — caches, package stores, container and VM images, model weights (see the file) | Path-prefix exclusion, unlike `exclude filters` which matches basenames. Removes the bulk of a dev home's file count | Anything under those trees becomes unfindable from Dolphin and KRunner. `$HOME/.claude/` in particular hides agent session data — drop that entry if you search it. Keep the `[$e]` suffix. KConfig reads the key with or without it, but the flag is what expands `$HOME` at read time: a `PathList` read of an unflagged entry returns the string verbatim, so all 28 prefixes would reach Baloo as literal `$HOME/…` paths and exclude nothing |
| `[General] exclude filters` | the 100-entry upstream KF6.29 default list plus 50 ecosystem globs (see the file) | Basename glob exclusion, matched against every file and directory name anywhere in the tree. Upstream already covers `.git`, `node_modules`, `__pycache__`, `.venv`, `.terraform`, `CMakeFiles`, and `.ninja_*`; the additions cover the ecosystems it misses (Rust, Go, JS bundlers, Python tool caches, Gradle/Maven, Xcode, Zig, Dart, Scala) | **Writing this key REPLACES upstream's list wholesale — it is not additive**, which is why the full upstream default is reproduced verbatim in the file. `target`, `vendor`, and `dist` are generic words, so a legitimately-named `dist` directory becomes unsearchable; `build` and `out` were deliberately left out as too generic. Baloo may merge newly-shipped upstream defaults back in when `exclude filters version` goes stale after a Plasma upgrade — harmless, but it shows up as `dotf plasma diff` noise |
| `[General] index hidden folders` | `false` | Keeps dotfile trees out of the index. This is the upstream default, pinned explicitly so an accidental System Settings toggle shows up as drift | None beyond the obvious: hidden config files will not appear in search results |

Provenance note: the upstream default list in `exclude filters` was read out of
the installed `libKF6Baloo.so.6.29.0` — the 25-entry folder-filter array followed
by the 75-entry file-filter array, in that order. Regenerate it if you move to a
Plasma version that ships a different default.

---

## `kwinrc` — window manager

Sixteen settings, the largest block. Focus behaviour, multi-monitor pointer
ergonomics, animation cost, and one security posture change.

| Group / key | Value | Why | Risk / annoyance |
| --- | --- | --- | --- |
| `[Windows] FocusStealingPreventionLevel` | `0` | `0` = None. An app launched from a terminal (`zed .`, `code .`, `xdg-open`) gets focus immediately instead of flashing in the task manager. The most-felt setting for a CLI-first workflow; the default is `1` (Low) | Genuinely lowers a guardrail. Any app can pull focus at any moment, so a finishing background build or an update dialog can steal keystrokes mid-typing. `1` is the safe middle |
| `[EdgeBarrier] EdgeBarrier` | `0` | Disables the pointer sticky barrier between monitors (default 100 px of resistance). On a multi-monitor desk the cursor crosses screens instantly | Slightly easier to overshoot onto the wrong screen when aiming at a control near a shared edge |
| `[EdgeBarrier] CornerBarrier` | `false` | Removes the extra sticky zone at shared screen corners, so throwing the cursor into a corner to hit a far monitor's close button works first try | Same overshoot tradeoff. Keep `true` if you rely on corner-slamming for a hot corner |
| `[Windows] ActivationDesktopPolicy` | `BringToCurrentDesktop` | With ten virtual desktops the default (`SwitchToOtherDesktop`) yanks you away from your workspace whenever an already-open app is re-activated. This pulls the window to you instead | Windows accumulate on whichever desktop you are on, quietly undoing a deliberate app-per-desktop arrangement. `DoNothing` only raises a taskbar highlight |
| `[Windows] RollOverDesktops` | `true` | Makes desktop-switch shortcuts wrap from 10 back to 1 instead of dead-ending. On a 10x1 row this turns a nine-step traverse into one | None functionally; you can circle past your target if you hold the key. Does not touch existing Meta+1..0 direct-switch bindings |
| `[Windows] BorderlessMaximizedWindows` | `true` | Maximized windows drop titlebar and border, reclaiming roughly 30–40 px of vertical space for editor and terminal rows | Real: a maximized window has no titlebar to grab, no visible close/minimize buttons, and no titlebar context menu. Use Meta+Q / Alt+F3. Do not enable if you mouse-drag maximized windows |
| `[Windows] BorderSnapZone` | `0` | Turns off the 10 px magnetic pull toward screen edges while dragging. Tile geometry, not an invisible magnet, should decide final position | Freehand placement near an edge no longer auto-aligns, so a dragged window can sit two pixels off. Set back to `10` if you miss it |
| `[Windows] ElectricBorderMaximize` | `false` | Stops auto-maximize when a drag brushes the top screen edge — a frequent misfire on a multi-monitor setup where windows cross the top boundary constantly | You lose drag-to-top as a maximize gesture. `ElectricBorderTiling` (drag-to-side half-snap) is deliberately left alone since it complements the tile layouts |
| `[TabBox] DelayTime` | `0` | Removes the 90 ms hold delay before the Alt+Tab switcher appears, so a quick tab-and-release feels instantaneous | The switcher UI now flashes even for the fastest window flick, which some read as visual noise. (taste) |
| `[TabBox] HighlightWindows` | `false` | Stops KWin raising and spotlighting each window as you scrub the Alt+Tab list, avoiding a cascade of repaints and stacking churn across ten desktops and multiple monitors | You lose the live preview of where the candidate window is; you navigate by thumbnail and title only. (taste) |
| `[Plugins] blurEnabled` | `false` | Background blur is a per-frame GPU pass over every translucent surface. Opaque terminals get nothing from it while it costs GPU time on every panel repaint | Panel, KRunner, and menu backgrounds go flat-translucent instead of frosted. A cosmetic side effect of a performance change; BreezeDark itself is untouched. (taste, if you like frosted glass) |
| `[Plugins] shakecursorEnabled` | `false` | Disables shake-to-magnify, which fires accidentally when flicking the pointer fast between monitors and briefly hijacks cursor size | None, unless you actually use shake-to-find |
| `[Plugins] startupfeedbackEnabled` | `false` | Kills the busy cursor shown while an app launches; on a fast NVMe box the feedback outlives the launch | No visual confirmation that a click started something, so you may double-launch a slow app |
| `[Compositing] AllowTearing` | `false` | KWin 6 defaults this to `true`, letting fullscreen apps present unsynced frames. On a workstation, tear-free presentation is the better tradeoff | Adds up to one frame of latency in fullscreen games and emulators that request tearing. Leave `true` if you game on this box |
| `[NightColor] NightTemperature` | `4000` | NightColor is already active with no explicit temperature, so it uses the 4500 K default. 4000 K is a meaningfully warmer evening target while staying above the roughly 3400 K point where syntax-highlight hues collapse into each other | Colors shift enough that evening color work (design, image editing, checking a UI palette) is misleading. 4500 K is gentler, 3500 K aggressive. (taste) |
| `[Xwayland] XwaylandEavesdrops` | `None` | Security posture: controls how much global keyboard input legacy X11 apps observe through Xwayland. The default `Combinations` lets any X11 client watch modifier-key combos system-wide. `None` closes that channel | **The single most disruptive entry in this overlay.** It breaks X11 apps that legitimately need global hotkeys — Discord and Mumble push-to-talk, some screen recorders, older remote-desktop clients. `Modifiers` is the middle ground; `All` is the permissive, insecure setting |

`[Xwayland]` is a **stripped group** in `../files/kwinrc`, because the captured
file's Xwayland scaling keys are display-local. The overlay re-asserts the one
key that is intent, which makes `XwaylandEavesdrops` one of the four
[apply-only settings](#the-four-apply-only-settings): this file is the only
place it is recorded, and `export` will never pick it up.

---

## `kdeglobals` — shell-wide defaults

| Group / key | Value | Why | Risk / annoyance |
| --- | --- | --- | --- |
| `[KDE] AnimationDurationFactor` | `0.5` | Halves every Plasma and KWin animation — window open/close, desktop switch, Overview, popups. The highest perceived-snappiness setting available, and it drives both shell and compositor from one key | None functionally. `0` disables animations entirely, which makes focus changes hard to follow; `0.5` keeps them legible. Log out and back in, or restart plasmashell, for the shell side to fully pick it up |
| `[KDE] ShowDeleteCommand` | `true` | Adds a permanent Delete entry alongside Move to Trash in Dolphin and all KIO file dialogs, removing the trash-then-empty dance for build output and scratch directories | A permanent-delete entry now sits one menu position from Move to Trash. `kiorc ConfirmDelete=true` is the safety net — do not enable this if you ever turn that off |
| `[General] TerminalApplication` | `ghostty` | Sets the terminal KIO launches for Dolphin's Open Terminal Here, "Run in terminal" in KIO dialogs, and `.desktop` entries with `Terminal=true`. Without it KIO falls back to konsole, then xterm | Must be a binary on `PATH`; a typo silently falls back to konsole/xterm. Swap for `alacritty` or `wezterm` if you prefer. Some KIO callers pass Konsole-style arguments, so an exotic terminal can be invoked with flags it does not understand |
| `[General] XftHintStyle` | `hintslight` | Slight hinting preserves the designed glyph shapes and inter-letter spacing of programming fonts instead of snapping stems to the pixel grid — which is what makes ligature-heavy fonts (Fira Code, JetBrains Mono, Iosevka) look uneven under full hinting | On a low-DPI panel `hintfull` genuinely renders crisper. (taste, and display-dependent — check it on your actual monitor rather than accepting it blind) |
| `[General] XftAntialias` | `true` | Pins font antialiasing on for all Qt/KDE apps so the overlay does not inherit whatever a distro default or a stray KCM click left behind | None — this is the sane default, pinned only so drift becomes visible in `dotf plasma diff`. `XftSubPixel` is deliberately **not** recommended: the correct value depends on your panel's subpixel layout and a wrong guess produces color fringing |

---

## `kcminputrc` — keyboard repeat

| Group / key | Value | Why | Risk / annoyance |
| --- | --- | --- | --- |
| `[Keyboard] RepeatDelay` | `250` | Milliseconds before a held key repeats (Plasma default 600). 250 ms makes held `h`/`j`/`k`/`l` navigation in Neovim or Kate respond almost immediately instead of stalling for over half a second | Below roughly 200 ms you get unintended repeats from ordinary typing, especially on a hall-effect board with a shallow actuation point. 250 is deliberately conservative |
| `[Keyboard] RepeatRate` | `50` | Repeats per second once repeat starts (default 25). Roughly doubles cursor travel for held motions in modal editors and log scrollback | Fast enough to overshoot a target line when you hold a motion a beat too long. Drop to 35 if you constantly correct |
| `[Keyboard] NumLock` | `0` | `0` = turn NumLock on at session start, so the numpad produces digits rather than acting as arrow keys | Only relevant on a full-size board. Values are `0` = on, `1` = off, `2` = leave unchanged. Use `1` if you deliberately use the numpad as a navigation cluster. (taste) |

`[Keyboard]` is a **stripped group** in `../files/kcminputrc`, because the
running session rewrites it as a unit. As with `[Xwayland]` above, the overlay
re-asserts the intent-bearing keys, and all three are
[apply-only](#the-four-apply-only-settings): `export` never captures them, so
deleting a line here deletes the setting from the repo outright. Those three
plus `XwaylandEavesdrops` are the whole of that list — nothing else in this
directory has the problem.

---

## `kscreenlockerrc` — lock screen

| Group / key | Value | Why | Risk / annoyance |
| --- | --- | --- | --- |
| `[Daemon] LockGrace` | `0` | `LockGrace` is the window after the lock screen engages during which it can be dismissed **without** a password. `0` means a deliberate Meta+L always demands the password immediately | None beyond losing the "oops, I meant to keep working" escape hatch in the first seconds after locking. Deliberately does not touch `Autolock`, `Timeout`, or `LockOnResume` |

---

## `ksmserverrc` — session startup

| Group / key | Value | Why | Risk / annoyance |
| --- | --- | --- | --- |
| `[General] loginMode` | `emptySession` | Starts each Plasma session clean instead of resurrecting whatever was open at logout. On a dev box, session restore reopens half-dead editors, stale terminals pointing at deleted worktrees, and browser windows scattered across every desktop — and it slows login measurably | You genuinely lose "pick up where I left off". All three mode tokens are literals in `/usr/bin/ksmserver`: `restorePreviousLogout` (the default), `restoreSavedSession`, and `emptySession` itself. (lower confidence: the spelling is verified, but a string table cannot show whether `emptySession` is matched explicitly or merely lands on the neither-of-the-other-two branch — the session is empty either way) |

---

## `klipperrc` — clipboard history

| Group / key | Value | Why | Risk / annoyance |
| --- | --- | --- | --- |
| `[General] MaxClipItems` | `100` | Clipboard history depth. The stock depth is far too shallow for a workflow copying error strings, commit SHAs, and snippets between terminal, editor, and browser all day; 100 turns Klipper into usable scrollback | A real security consideration: a deeper history means secrets pasted from a password manager linger in memory and in Klipper's saved history much longer. Pair with `KeepClipboardContents=false` if you paste credentials often |
| `[General] IgnoreImages` | `false` | Keeps images in history, so a Spectacle region capture stays retrievable after you copy something else | Images make the history noticeably heavier in RAM and on disk. Set `true` if you notice plasmashell memory growth. (taste) |
| `[General] IgnoreSelection` | `true` | Keeps the PRIMARY selection — anything merely highlighted with the mouse — out of history. Without it, every drag-select in a terminal or editor floods the history and buries what you actually pressed Ctrl+C on | Middle-click paste of the primary selection still works; only the history stops recording selections. Set `false` if you scroll back through past mouse selections |

---

## `dolphinrc` — file manager

| Group / key | Value | Why | Risk / annoyance |
| --- | --- | --- | --- |
| `[General] ShowFullPath` | `true` | Shows the absolute path in the location bar instead of breadcrumbs only, so a path is readable and copyable at a glance for pasting into a terminal | None. Slightly denser location bar for deeply nested trees. Pair with `EditableUrl=true` if you would rather type paths. (taste) |
| `[General] RememberOpenedTabs` | `false` | Dolphin starts at home instead of restoring last session's tabs — which on a dev box means tabs pointing at deleted worktrees, unmounted NAS shares, and cleaned build directories that each stall on startup | You lose Dolphin session restore. Same tradeoff family as `ksmserverrc loginMode`; adopt both or neither for consistency |

---

## `plasmarc` — panel tooltips

| Group / key | Value | Why | Risk / annoyance |
| --- | --- | --- | --- |
| `[PlasmaToolTips] Delay` | `-1` | Disables the large hover tooltips on panel widgets and task-manager entries (default 0.7 s). They pop up constantly while reaching for the system tray and cover the thing you were aiming at | You lose window previews on task-manager hover, which some people use for window picking. A positive number such as `2.0` is the softer option — just a longer delay. (lower confidence: `-1` as the disable sentinel is documented convention, not something verified on a live host — check the tooltip behaviour after applying) |

The theme name lives in `kdedefaults/plasmarc`, a separate managed path. This
file is only the tooltip policy.

---

## `krunnerrc` — launcher

| Group / key | Value | Why | Risk / annoyance |
| --- | --- | --- | --- |
| `[General] RetainPriorSearch` | `false` | KRunner opens empty every time instead of pre-filling and pre-selecting the last query. For a launcher invoked dozens of times an hour, a stale prefilled query is a latent mis-launch waiting for an early Enter | You lose one-keystroke repeat of the previous search. (taste, but it favors safety.) The existing `FreeFloating` setting is untouched |
| `[General] historyBehavior` | `ImmediateCompletion` | Inline-completes from history as you type rather than waiting for the suggestion list, cutting a launch to a few characters plus Enter. Valid values are `Disabled`, `ImmediateCompletion`, and `CompletionSuggestion` (the default) | Aggressive inline completion can complete to the wrong prior command, and an early Enter then launches it. Use `Disabled` for no history influence at all — also the privacy-maximal choice |

---

## Verifying an adoption

```bash
dotf plasma diff --recommended            # per-key drift, both directions
dotf plasma apply --recommended --dry-run # exact bytes that would land
dotf plasma apply --recommended --backup  # apply with an undo copy
```

Several of these need a component restart to take effect in the running session.
`dotf plasma apply` prints the exact reload commands it would run; see
[`../README.md`](../README.md) for why that is opt-in.

## Spelling provenance

A misspelled group or key is not an error. KConfig writes it happily, KDE never
reads it, and the setting is silently inert — which is the worst failure mode a
file like this can have. Every group/key pair above was therefore re-verified
against Plasma 6.7.4 / KF6 6.29 on the audited host, from the shipped schemas
and binaries rather than from memory:

| Source of truth | Covers |
| --- | --- |
| `/usr/share/config.kcfg/baloosettings.kcfg` | `baloofilerc` — including that `folders` and `exclude folders` are `PathList`, which is why KDE writes them with the `[$e]` suffix, while `exclude filters` is a plain `StringList` and correctly carries no suffix. `[$e]` selects **`$VAR` expansion at read time, not readability**: `kreadconfig6` returns a plain `exclude folders=` entry normally, so such an entry on a live host is effective, not dead — but a `PathList` read of it leaves `$HOME` unexpanded, which is the whole reason the suffix has to survive a round-trip through this repo |
| `/usr/share/config.kcfg/kwin.kcfg` | `kwinrc` `[Windows]`, `[EdgeBarrier]`, `[TabBox]`, `[Compositing]`, and `[Xwayland] XwaylandEavesdrops` with its four enum choices |
| `/usr/share/config.kcfg/nightlightsettings.kcfg` | `kwinrc [NightColor] NightTemperature`, and its 4500 default |
| `/usr/share/config.kcfg/kcmaccessibilityshakecursor.kcfg` | `kwinrc [Plugins] shakecursorEnabled`, and with it the `<pluginId>Enabled` naming rule that `blurEnabled` and `startupfeedbackEnabled` follow; the `blur` and `startupfeedback` plugin ids are confirmed by their `preferences-system-windows-effect-<id>` Breeze icons |
| `/usr/share/config.kcfg/krunnersettingsbase.kcfg` | `krunnerrc [General]` — `historyBehavior` has no `key=` override, so the config key really is that lowercase-h spelling, and the three enum choices are as documented |
| `/usr/share/config.kcfg/dolphin_generalsettings.kcfg` | `dolphinrc [General]` |
| `/usr/share/config.kcfg/landingpage_kdeglobalssettings.kcfg` | `kdeglobals [KDE] AnimationDurationFactor` |
| `/usr/share/config.kcfg/workspaceoptions_plasmasettings.kcfg` | `plasmarc [PlasmaToolTips] Delay` |
| `libklipper.so.6`, `libKScreenLocker.so.6` | `klipperrc [General]` `MaxClipItems` / `IgnoreImages` / `IgnoreSelection`, and `kscreenlockerrc [Daemon] LockGrace` |
| `libkworkspace6.so.6` (the key) and `/usr/bin/ksmserver` (the values) | `ksmserverrc [General] loginMode`, plus all three mode tokens — `restorePreviousLogout`, `restoreSavedSession`, `emptySession` |
| `libKF6KIOCore.so.6` | `kdeglobals [General] TerminalApplication` |
| `libkrdb.so` (which reads them into the X resource database) and `plasma/kcms/systemsettings/kcm_fonts.so` (which writes them) | `kdeglobals [General]` `XftAntialias` / `XftHintStyle`, and the `hintslight` token. These are **not** in `libKF6KIOCore.so.6` — the KIO library owns `TerminalApplication` only |
| `kf6/kded/keyboard.so` and `libkwin.so.6` | `kcminputrc [Keyboard]` `NumLock` / `RepeatDelay` / `RepeatRate` — read by both the kded module and KWin, which owns keyboard repeat on Wayland |
| The live `~/.config/kdeglobals` | `[KDE] ShowDeleteCommand` — KDE had already written the key itself under that group, which beats any schema |

The five rows naming shared libraries and binaries cover keys read through plain
`KConfigGroup` calls, so they have no `.kcfg` to point at and had to be read out of the
compiled string tables instead. Both encodings were scanned, because the split is not
uniform: Qt stores the group names (`[General]`, `[Daemon]`, `[Keyboard]`) as UTF-16
`QStringLiteral`s, and so are the `klipperrc` and `kscreenlockerrc` key names, while
`loginMode`, the `Xft*` keys, and the `[Keyboard]` keys are plain ASCII. A
single-encoding `strings` pass confirms only half of any given row.
