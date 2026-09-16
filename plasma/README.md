# KDE Plasma Configuration

Repo-owned KDE Plasma 6 settings for the Plasma session path in this dotfiles
repository, managed by `dotf plasma`.

This directory holds a **curated subset** of `~/.config`, not a Plasma backup.
It is deliberately small: the parts of a KDE config that express what you meant,
with the parts that express what this machine happens to be left out.

## Copy, not symlink

The repo holds **copies**. Nothing in `~/.config` points here by default.

That is a deliberate inversion of how the rest of this repo works, and the
reason is that KDE rc files are not configuration files — they are configuration
plus state. `kwinrc` carries tiling layouts keyed by your monitor's EDID-derived
UUID. `kdeglobals` carries a ~70% block of color values mechanically regenerated
from the active scheme. `dolphinrc` carries window geometry keyed to
`3840x2160`. And KDE rewrites these files on nearly every settings change.

Symlink them and two things happen: you commit machine identity into git, and
your worktree goes dirty every time you touch a slider in System Settings.

So the snapshot is refreshed **explicitly**:

```bash
dotf plasma export
```

Between exports you can experiment freely in System Settings and git stays
clean. Volatile groups, keys, and values are stripped on the way in by a
governed table, so the snapshot is portable by construction.

One consequence worth stating plainly: KConfig has no comment model. Its writer
re-parses and re-serialises from scratch, so comments, blank lines, and original
key order are destroyed on the first write. The files here are canonical output
(groups sorted, keys sorted, one blank line between groups) and cannot carry
inline explanation. That is what the two READMEs are for.

## `files/` versus `recommended/`

| | `files/` | `recommended/` |
| --- | --- | --- |
| What it is | a snapshot of what your machine **is** | an opinion about what a good KDE **should be** |
| Written by | `dotf plasma export`, never by hand | hand, never by `export` |
| Shape | whole config files, minus stripped state | partial overlays — only the recommended keys |
| Applied | by default | only with `--recommended` |

Precedence when `--recommended` is passed:

```text
live  <  plasma/files/<path>  <  plasma/recommended/<path>
```

A key in both takes the recommended value; a key only in `recommended/` is
added; a key only in `files/` is unaffected. `diff` and `status` honour the flag
too, so `dotf plasma diff` answers "am I converged on my snapshot?" while
`dotf plasma diff --recommended` answers "am I converged on snapshot plus
overlay?".

Every one of the 40 recommended settings is documented — with its downside — in
[`recommended/README.md`](./recommended/README.md). Read that before applying
it; a few entries are genuine tradeoffs and one (`XwaylandEavesdrops=None`) can
break unrelated applications.

One asymmetry is worth knowing before you edit that tree: four of the 40 sit
inside a group the strip table calls volatile, so they are **apply-only** —
`apply --recommended` writes them and `export` can never capture them back.
`recommended/` is the only place they exist, and a test names all four and fails
the build if a fifth appears. `recommended/README.md` lists them.

## Commands

```bash
dotf plasma status                       # what is managed, present, and drifted
dotf plasma diff                         # per-key drift, both directions
dotf plasma export                       # live -> repo (refresh the snapshot)
dotf plasma apply                        # repo -> live (merge)
```

Realistic runs:

```bash
# See what a fresh capture would pick up, without writing anything
dotf plasma export --dry-run

# Refresh only the two files you actually changed
dotf plasma export --only kwinrc,kglobalshortcutsrc

# Preview an apply, then run it with an undo copy
dotf plasma apply --dry-run
dotf plasma apply --backup

# Apply the opinionated overlay on top of the snapshot
dotf plasma apply --recommended --backup

# Apply and restart the affected KDE components right now
dotf plasma apply --reload

# Machine-readable output for any verb
dotf plasma status --json
```

Flags: `--only`, `--dry-run`, `--backup`, `--link`, `--recommended`, `--reload`,
`--yes`, `--json`. `--link` and `--reload` are accepted on `apply` only.
`--dry-run` never mutates anything.

`dotf apply` can run the same merge as one opt-in phase of a full provisioning
pass:

```bash
dotf apply --plasma
PLASMA=true dotf apply
```

That path is headless: it never prompts, never reloads the running shell, and
never layers `recommended/`. Backups are on automatically, because opting into
an optional destructive phase should opt you into its safety net.

## Apply merges — it never deletes a live key

`apply` is a per-key merge, not a file overwrite:

- every key present in the repo file is **written** to the live file
- every key present only in the live file is **preserved**, byte for byte,
  including its value bytes, its `[$…]` flags, and its locale tag
- every group present only in the live file is left untouched

That is why convergence is a **subset** check, not file equality. A live
KConfig file always has more keys than the repo copy — everything the strip
table removed, plus everything Plasma has written since. A converged file is not
opened for writing at all, so `applied=0` is the honest signal that there is
nothing to do.

There is no "prune live keys not in the repo" mode, not even behind a flag.
Deleting a key you set through System Settings after the last export would be
indistinguishable from deleting a key KDE added on upgrade.

The one escape hatch is a KConfig tombstone. Hand-add a `[$d]`-flagged key to a
file under `files/`:

```ini
[Windows]
SomeKey[$d]
```

`apply` writes that tombstone verbatim, and KDE reads it as "absent, and do not
fall back to the cascaded default". This is the only supported way to express
"remove this live key".

## The `--link` escape hatch

`dotf plasma apply --link` replaces each live path with a symlink into
`plasma/files/`. KConfig opens and rewrites the existing inode rather than
replacing it, so KDE genuinely reads **and writes** the repo file through the
link.

That is also the hazard: every settings change KDE makes then lands in the git
worktree, including the volatile state the strip table exists to keep out.

Want it when:

- you are iterating hard on one area and want changes captured continuously
  without running `export` after every click
- you are provisioning a fresh machine that has never run Plasma, so there are
  no live files to lose

Do not want it as a steady state. It is per run and never persisted — there is
no per-file link mode, because the repo's contents should not depend on how it
was last applied.

The mode is per file and the two modes are mutually exclusive. Linking over a
real live file would discard its live-only keys, so `apply` refuses before any
mutation unless you pass `--backup` (which makes the transition recoverable) or
`--dry-run` (which makes it a preview):

```bash
dotf plasma apply --link --backup
```

Going back is the same move in reverse: `dotf plasma apply --backup` on a linked
machine copies the link target's content into the backup directory, removes the
symlink, and writes a merged regular file.

`export` on a linked file re-strips it in place. That is how a linked setup gets
re-cleaned, and it is intentional, but it is surprising the first time.

## What is deliberately not managed

Some of this is stripped out of managed files on export; some of it is behind a
hard denylist the engine refuses to read, write, or traverse at all.

| Not managed | Why |
| --- | --- |
| Monitor configuration (`kwinoutputconfig.json`) | EDID hash, panel serial, output UUID, HDR calibration. It **is** the identity of one specific physical panel |
| Tiling layouts (`kwinrc [Tiling][…][…]`) | Every header is keyed by two UUIDs — a virtual-desktop UUID and the monitor's EDID-derived output UUID — so the layouts are unreachable on any other machine. Desktop `Id_*` values are stripped with them; `Number` and `Rows` survive. Tile layouts are per-monitor ergonomics, redrawn per machine |
| Panel and applet layout (`plasma-org.kde.plasma.desktop-appletsrc`, `plasmashellrc`) | Installation-local integer instance IDs cross-referenced across three files, plus resolution-keyed geometries. It can only ever be applied wholesale; key-merging it into a live shell corrupts plasmashell |
| Activity UUIDs (`kactivitymanagerdrc`, `kactivitymanagerd-statsrc`) | Generated per installation. A transplanted UUID names an activity that does not exist |
| Wallet and secrets (`~/.local/share/kwalletd/`, `keyrings/`) | Secret material. The engine never walks `~/.local/share`, so these are out of reach structurally, not merely absent from a list |
| Device and network identity (`kdeconnect/`, `bluedevilglobalrc`, `krdcrc`, `krdpserverrc`, `konsolesshconfig`, `trashrc`, `ktrashrc`) | Private keys and certificates, adapter MACs, internal hostnames, private IPs, NFS export paths, password fields. These are on the hard denylist so a future glob widening cannot reach them |
| Session ledgers (`session/`, `ksmserverrc [Session: *]`) | Rewritten on every logout. `ksmserverrc` itself is managed; only the ledger groups are stripped |
| Migration ledgers (`kconf_updaterc`, `powermanagementprofilesrc`, `plasma-welcomerc`, `discoverrc`) | A record of which migrations already ran **on this install**. Transplanting one actively suppresses a migration the new machine needs |
| Derived colors (`kdeglobals [Colors:*]`, `[WM]`, `[ColorEffects:*]`, `Trolltech.conf`) | Mechanically regenerated from the active scheme name. Shipping them creates split-brain: a rendered palette that contradicts the declared scheme. Guarded — if your scheme has no matching `.colors` file anywhere, the strip is skipped and you get a stderr advisory, so a hand-rolled palette is never silently lost |
| PIM and per-app state (`akonadi/`, `emailidentities`, `kdenliverc`, `elisarc`, `kalarmrc`) | Email addresses and account state, or simply not desktop configuration |
| Window rules (`kwinrulesrc`) | Empty today, and rule bodies live in UUID-named groups. Revisit when rules actually exist, and then ship the file as a unit |
| Personal cloud paths (`spectaclerc`, `QtProject.conf`) | Their only real content is absolute save/last-visited paths into a personal cloud tree. Excluded rather than stripped, because stripping leaves an empty file. A `recommended/spectaclerc` with `[$e]`-flagged `$HOME`-relative values is the natural way to bring the setting back portably |

Also note `ktimezonedrc` is excluded because timezone is owned by
`dotf sysconfig`. `plasma-localerc` **is** managed — it is per-user Plasma
number/date formats, a different mechanism and scope from `sysconfig`'s
`localectl` work, so the two do not collide.

### One thing that is machine-shaped and kept anyway

`kcminputrc` carries per-device groups whose headers are the setting:

```ini
[Libinput][1133][50504][Logitech USB Receiver Mouse]
NaturalScroll=true
```

The bracket segments are a USB vendor ID, a product ID, and the device's model
name — `1133`/`50504` is `0x046D`/`0xC548`, Logitech's public USB-IF vendor code
and one of its receivers. They are **model** identifiers shared by every unit
ever made, not per-unit serial numbers, and they are the only addressing KConfig
offers for a device preference: strip them and the file is empty, since
`NaturalScroll` alone is unaddressable.

So these are kept, and they do disclose which peripherals the machine has. A
group naming a device you do not own is inert — Plasma reads it, finds no
matching device, and ignores it — so carrying another machine's rows costs
nothing. If you would rather not publish your hardware inventory, drop
`kcminputrc` from the export with `--only`, or delete the file from `files/`;
`apply` simply has nothing to say about a file the repo does not hold.

## Reload

Applying a setting writes a file. It does not make the running session notice.

Reload is **opt-in** because the commands are materially disruptive:
`kquitapp6 plasmashell` tears down the panel, the system tray, every widget, and
any open KRunner or notification, mid-session; restarting `plasma-kglobalaccel`
momentarily drops every global shortcut. That is not what someone running
`dotf plasma apply` to sync one keybinding expects.

So `apply` **always prints** the exact commands it would run, as
`plasma.reload.<n>=` lines on stdout, whether or not you passed `--reload`.
Paste them, or ignore them and pick the changes up at next login.

`--reload` runs them. What runs depends on which files the run actually
modified:

| Component | Triggered by | Command |
| --- | --- | --- |
| `lookandfeel` | any `kdedefaults/*` change | `plasma-apply-lookandfeel -a <package>` |
| `kwin` | `kwinrc`, `kcminputrc`, `kdeglobals` | `qdbus org.kde.KWin /KWin reconfigure` |
| `kglobalaccel` | `kglobalshortcutsrc` | `systemctl --user restart plasma-kglobalaccel` |
| `plasmashell` | `plasmarc`, `plasmanotifyrc`, `klipperrc`, `krunnerrc`, `kdeglobals` | `kquitapp6 plasmashell` then `kstart plasmashell` |

Everything else is read by its application at next start and needs no reload at
all: `dolphinrc`, `katerc`, `katevirc`, `konsolerc`, `kiorc`, `mimeapps.list`,
`baloofilerc`, `kwalletrc`, `ksmserverrc`, `kscreenlockerrc`, `powerdevilrc`,
`kded5rc`, `plasma-localerc`.

`--reload` is gated on a live KDE session and is never run by `dotf apply`. A
failed reload is reported and does not abort the rest.

### Global shortcuts on Wayland

Plasma 6 moved the global-shortcuts server **into the compositor** on Wayland:
`kwin_wayland` owns `org.kde.kglobalaccel`, and `plasma-kglobalaccel.service` is
vestigial there — it starts, finds the bus name already taken, and exits
`0/SUCCESS` in the same second. Restarting it is therefore not a weak reload but
a guaranteed no-op that reports success.

There is no in-session alternative. `org.kde.KGlobalAccel` exposes no reload
method, and `org.kde.KWin`'s `reconfigure` does not cover shortcuts — KWin reads
`kglobalshortcutsrc` only at its own startup. **Logging out and back in is the
only way to apply a shortcut change.**

So on a Wayland session `apply` plans the `kglobalaccel` reload but marks it
skipped, printing this instead of a runnable command:

```
plasma.reload.skipped=kglobalaccel reason=wayland-kwin-owns-server
plasma.reload.note=kglobalshortcutsrc applied; log out and back in for it to take effect
```

X11 sessions are unaffected: there the unit really does own the bus name, and
the `systemctl --user restart` line is planned and run as before. A session that
sets neither `XDG_SESSION_TYPE` nor a socket variable still attempts the
restart, since it is harmless where it cannot work.

**Caveat while the change is pending:** until you log back in, KWin keeps
serving the _old_ shortcuts from memory, and System Settings → Shortcuts shows
that stale in-memory set rather than the file. Opening that page can write the
stale set back over `kglobalshortcutsrc`, silently reverting the apply — so
avoid it between applying and logging back in. `dotf plasma status` reports the
drift if it happens, and re-running `apply` restores the file.

## Coexisting with Hyprland

Both desktops live in this one repo and neither knows about the other.

| | Hyprland | Plasma |
| --- | --- | --- |
| Tree | `hypr/`, `waybar/`, `mako/` | `plasma/` |
| Mechanism | symlinks, via `dotf link` | copy plus per-key merge, via `dotf plasma` |
| Opt-in flag | `--linux-desktop` (`symlink.FacetLinuxDesktop`) | `dotf plasma`, or `dotf apply --plasma` |

There is **no shared file and no shared flag**. `--linux-desktop` does not touch
`plasma/`; `--plasma` does not touch `hypr/`. A test asserts that no path
claimed by `dotf link` is ever claimed by `dotf plasma`, so the two engines can
never fight over a destination — and a plasma symlink under `--link` can never
be clobbered by the symlink engine.

One repo can therefore drive both sessions on the same machine. Log into
Hyprland or into Plasma; each reads its own tree. Migrating one into the other
is explicitly not planned.

## Iterating

The loop is: change it in the GUI, look at the diff, capture it, commit it.

```bash
# 1. Tweak whatever you want in System Settings. Git stays clean.

# 2. See exactly which keys moved
dotf plasma diff

# 3. Capture into the repo (preview first if the diff surprised you)
dotf plasma export --dry-run
dotf plasma export

# 4. Review and commit
git -C ~/.config/dotfiles diff plasma/files
git -C ~/.config/dotfiles add plasma/files
git -C ~/.config/dotfiles commit -m "chore(plasma): refresh captured snapshot"
```

If step 2 shows drift you did not cause, it is usually one of three things: a
Plasma upgrade retuning a default, a key the strip table should be removing but
is not, or a `SessionOwned` file the running session rewrote underneath you
(`kwinrc`, `kglobalshortcutsrc`, `plasmarc`, `klipperrc`, `plasmanotifyrc`).

Going the other way — you edited `plasma/files/` by hand, or pulled someone
else's commit:

```bash
dotf plasma diff
dotf plasma apply --dry-run
dotf plasma apply --backup
```

Hand-editing `files/` is supported. The parser is strict on purpose: it rejects
a malformed line loudly rather than silently dropping it the way KDE's own
parser does, because a silently dropped line is a setting you lose without ever
seeing it go.
