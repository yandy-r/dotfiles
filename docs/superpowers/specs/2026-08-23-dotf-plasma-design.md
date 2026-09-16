# `dotf plasma` — KDE Plasma 6 settings management

Status: design specification, **reconciled with the shipped implementation**.
Date: 2026-08-23. Last reconciled: 2026-08-23 (post-review amendment pass, then an independent spec-vs-code audit pass — A11/A12, then a post-PR-review reconciliation pass against the fixed branch — A14–A19, then a code-first audit pass that found two whole shipped surfaces undocumented — A20–A25).
Scope: one new top-level `dotf` subcommand, one new `internal/plasma` package, one new `plasma/` repo data tree, and an opt-in phase in `dotf apply`.

This document is a single contiguous specification. It intentionally exceeds the
repo's ~500-line soft cap for source files; that cap governs Go and shell
modules, not design documents.

## Amendment log

This spec was written **before** implementation. A seven-dimension adversarial
review of the shipped branch then found 23 findings (20 confirmed defects, 3 of
them critical), and several of the fixes proved the **spec itself** was wrong.
The code is now the ground truth and this document has been amended to match it.
Every amended passage carries an inline `**Amended**` note so a reader never
mistakes corrected text for text that was always there. The substantive changes:

| # | Section | What changed |
| --- | --- | --- |
| A1 | §6.2, §6.7, §5.3 | **The merge moved from plan time into the lock at write time.** The original design put the read-modify-write cycle either side of the confirmation prompt with the lock over the rename only — a data-destroying race that was inevitable _by design_. Plan-time `Content` is now a preview |
| A2 | §6.6 | A fifth presence state, **PRESENT BUT UNREADABLE**, was never modelled. It is now a per-file failure, never `ActionCreateFile` |
| A3 | §6.6, §12.2, §3.2 | The presence matrix is expressed over the **source document** (`files/` ∪ `recommended/`), resolved _before_ the presence check |
| A4 | §6.9 | A **failed backup refuses** the action it protects |
| A5 | §8.2, §9.2, §5.8 | `--link` and `--recommended` are now **mutually exclusive** |
| A6 | §4.1–§4.6 | Escape/decode table corrections proven against kconfig 6.29: the lone trailing backslash, group-segment edge space, the decoded 0x1D joiner, the leading-BOM key, the never-decoded locale tag, and the `d`-flag nuance |
| A7 | §5.4, §8.3 | New `DiffExpectedVolatile` kind; `diff` key detail is **one line per entry** |
| A8 | §7.3 | Four strip rules were wrong and are corrected: `kdeglobals` `[Icons]` and `View Style`, `krunnerrc` `[General]/history*`, `mimeapps.list`'s value glob |
| A9 | §12, §14.2, §14.5 | New governed invariant on `recommended/` vs the strip table; two denylist additions; a new delivery-checklist for repo doc surfaces |
| A10 | §5, §13 | File layout and test tables reconciled with the real tree |
| A11 | §4.2, §5.5, §8.2, §8.4, §8.5, §8.7, §8.9 | **Independent spec-vs-code audit corrections.** Seven passages described code that was never written that way: the UTF-8 "validator", the `#` escape's context guard, `Engine`'s field list, `--json`'s usage string and its DRY seam, the stderr advisory set and its suppression rule, `plasmaKeyReport.Encoding`, the confirm-helper reuse claim, and the verb-dispatch names |
| A12 | §8.2, §8.6 | `*LinkRecommendedError` moves into the **usage bucket, exit 2**, joining the other two flag-applicability rules. It was exiting 1 only because the check sat in the engine |
| A13 | §2.1, §14.3, §14.4, §15 | **The spec was leaking the identifiers it argues must never be committed.** §14.3's table and one §2.1 example quoted real UUIDs, a per-install token, an adapter MAC, an internal host and a panel serial verbatim; `table_glob_test.go` repeated three of them. All are now the synthetic constants, the scrubbing convention is restated as repository-wide rather than fixture-only, and a second, shape-based guard covers every file the feature contributes |

A round of PR-review fixes then deleted three declared members, collapsed the
duplicated convergence predicate, and changed what the mutating verbs report.
The following rows reconcile this document with that code (PR #95).

| # | Section | What changed |
| --- | --- | --- |
| A14 | §5.3, §5.4, §5.8, §8.3 | **Three declared members are gone from the implementation.** `OwnershipConflictError`, `KeyAction.Flags` and `StatusReport.Session` were all zero-reader declarations. The §3.1 ownership guarantee is enforced at **build** time instead, by `TestNoOverlapWithSymlinkTable`. `StatusReport` also gains the `Absent` set this listing predated, and §8.3 gains its `plasma.absent=` line |
| A15 | §6.1 | **`Converged` is a thin predicate over `diffDocs`, not a second subset loop.** The design intent ("one predicate") was always right; the code carried two independently written loops over the same triple. `TestConvergedMatchesDiffDocs` now pins them together |
| A16 | §8.3, §8.4 | **Mutating verbs render the plan AS EXECUTED**, so a real run emits the same file set — `plasma.skip=` bucket included — that its own `--dry-run` emits. The session-owned advisory is now direction-specific (apply vs export) and is emitted **once per file**, not once per planned action |
| A17 | §8.5, §8.6 | **`--json` gains a top-level `failures` array**, one diagnostic per non-fatal failure. `summary.failed` no longer counts apply's post-apply `DriftError` — drift is a REPORT carried by `driftFiles`/`driftKeys` — and now always equals `len(failures)`. Both drift counters mean the same thing on every verb |
| A18 | §5.3, §5.7, §10.1, §10.3 | **Reload argv is resolved at PLAN time** by `Engine.resolveReloads`, so the printed commands name the binary that exists on the host even when `--reload` is absent and nothing runs. `Report.Reloaded` is now populated — by `cmd/dotf`, not by the engine |
| A19 | §12.3 | **Data correction.** The claim that a plain `exclude folders` (no `[$e]`) is ignored by KConfig is false, verified against `kreadconfig6` on a KF6.29 host. `[$e]` controls shell/environment EXPANSION of the value at read time, not whether the key is read at all |

An independent **code-first** audit of the same branch then read the tree without
reference to this document and found that two whole shipped surfaces were absent
from it: `--unlink` (which the branch added to fix `--link`'s one-way door) and
`--show-volatile` (which §5.4 still described as unshipped future work, and which
shipped with a different design from the one §5.4 prescribed). The rows below
reconcile those, and the listing drift around them, with the code.

| # | Section | What changed |
| --- | --- | --- |
| A20 | §5, §5.3, §5.8, §8.2, §8.3, §8.5, §8.6, §8.9, §9.1–§9.3, §13 | **`--unlink` shipped and this document never mentioned it.** `--link` was a one-way door: once a managed file was a symlink into the repo, reading the live path read the repo's own bytes, so every later merge saw a converged file and skipped it — the link survived every apply forever. `--unlink` is `--link`'s explicit inverse, carried by `Options.Unlink`, `ActionUnlink`, `LinkUnlinkError`, `Plan.LinkConflicts`, the `unlink` stdout bucket, and a state machine extracted into `internal/plasma/applylink.go`. §9.2's matrix gains its third column and one of its old cells is **corrected**: a plain apply over a file linked into the repo is `ActionSkip`, not a conflict |
| A21 | §5.4, §8.2, §8.3, §8.5 | **`--show-volatile` shipped, and not the way §5.4 prescribed.** The flag exists and is wired to `Options.ShowVolatile`. It did **not** land as a new attribute on the existing lines, and expected-volatile keys do **not** fold into `plasma.only-live=`: `plasmaDiffLines` emits a **third bucket**, `plasma.expected-volatile=`, split on `DiffKind`, and `summary.volatileKeys` counts those rows separately from `driftKeys` |
| A22 | §8.2, §8.6, §8.9 | **The exclusive-pair rejection is a second function.** `plasmaCheckApplyOnlyFlags` does only the verb-applicability check for `--link`/`--unlink`/`--reload`; the mutually exclusive PAIRS (`--link`+`--recommended`, `--link`+`--unlink`) are rejected by `plasmaCheckExclusiveFlags`, called immediately after it. A12's attribution named the wrong function and the exit-2 column was incomplete |
| A23 | §8.5, §8.9 | **Structural drift in the two `cmd/dotf` listings.** `plasmaReport` gains `linkConflicts`, `plasmaSummary` gains `volatileKeys`, `plasmaOpts` gains `unlink` and `showVolatile`, and `plasmaRun` gains `stdin io.Reader` — the seam that makes the exit-130 confirm path testable without a pseudo-terminal. The command is **four** files, not three: the `--json` document types live in `plasmajson.go`. `runPlasma` also resolves the session **first**, ahead of every pre-flight failure, so all of them can emit the same `--json` envelope |
| A24 | §8.5 | **Correction to A17.** `status` does not call `plasmaDriftTally`; it fills both drift counters from the engine's own `StatusReport` counters. The numbers agree because `Status` resolves to the same comparison, but "one helper on three verbs" was literally false |
| A25 | §7.2 | **Rule 5's illustration contradicted the shipped data file.** It claimed `exclude folders[$e]` is written **without** the flag suffix in `plasma/recommended/baloofilerc`; the committed file carries the suffix, as §12.3 requires. The rule being illustrated is about the strip **pattern**, not about the recommended file |

---

## 1. Summary and goals

### 1.1 What this builds

`dotf plasma` manages a curated subset of KDE Plasma 6 configuration as
version-controlled data in the dotfiles repo, seeded from the user's currently
running desktop, and coexisting with the existing Hyprland configuration without
sharing a single file or flag.

Four verbs:

| Verb | Direction | Effect |
| --- | --- | --- |
| `export` | live → repo | Capture the managed set into `plasma/files/`, stripping volatile groups and keys |
| `apply` | repo → live | Merge repo keys into the live files, preserving live-only keys |
| `diff` | both | Report per-key drift in both directions |
| `status` | read-only | Managed-file count, presence, drift summary, KDE session detection |

### 1.2 Goals

1. **Copy semantics by default.** The repo holds copies, not symlink targets, so
   live experimentation in System Settings never dirties the git worktree.
   Symlinking is an explicit per-run opt-in (`--link`).
2. **Apply is a merge, never a blind overwrite.** Every key present in the repo
   file is written to the live file; every key present only in the live file is
   preserved byte-for-byte.
3. **A native Go KConfig codec.** No `kwriteconfig6` / `kreadconfig6`
   subprocesses for INI work. KDE binaries are used only for the optional,
   opt-in live reload.
4. **Volatile state never enters git.** Machine UUIDs, monitor EDID-derived
   identifiers, window geometry, migration ledgers, session ledgers, and derived
   colour blocks are stripped on export by a governed, golden-tested table.
5. **Works on a fresh machine.** `apply` runs before the user has ever logged
   into Plasma; the absence of a live KDE session is a warning, not an error.
6. **Repo contract parity.** `key=value` on stdout, diagnostics on stderr,
   `0/1/2` exit codes, `130` on Ctrl+C, `--dry-run` that never mutates,
   completions and help regenerated in the same change.

### 1.3 Explicit non-goals

- **Not a full Plasma backup/restore tool.** Panel layout, applet instances,
  activities, monitor configuration, KDE Connect identity, wallet contents, and
  PIM account state are out of the managed set permanently (§14, §16).
- **Not a symlink facet.** `dotf link` / `symlink.DefaultTable()` is untouched.
  Plasma owns its own static file table inside `internal/plasma`.
- **Not a setup module.** `manifest/packages.toml` gains no `[[module]]` and no
  `[[package]]` entry; the custom-script ratchet baseline is unchanged.
- **Not a Hyprland replacement or migration.** `hypr/`, `waybar/`, `mako/` stay
  on `symlink.FacetLinuxDesktop` behind `--linux-desktop`. There is no shared
  file and no shared flag between the two desktop sets.
- **Not cross-desktop.** macOS and Windows are hard-gated out. WSL is treated as
  Linux for detection but has no KDE session, so `apply` warns and `export`
  fails on a missing live set.
- **Not a theme engine.** `plasma/files/` records which look-and-feel package is
  active; it does not carry generated colour palettes (§7, JC-1).

---

## 2. Background: why symlinking whole Plasma rc files is the wrong default

### 2.1 KDE rc files are not configuration files, they are configuration plus state

The audit of the live `~/.config` tree found that the majority of bytes in the
most valuable Plasma files are machine-local state, not user intent. Concrete,
verified examples:

- **`kwinrc` `[Tiling][…][…]`** — every tiling-layout group header is keyed by
  **two** UUIDs: a virtual-desktop UUID and an **output** UUID. That output UUID
  — written here as `00000000-0000-4000-8000-000000000002`, the synthetic form
  every identifier in this document uses (§14.3) — is the DP-2 monitor's identity
  from `kwinoutputconfig.json`, derived from its EDID. There are 16 `[Tiling]` blocks
  for 10 desktops — six are orphans of deleted desktops — and 14 of the 16 hold
  the identical default `0.25/0.5/0.25` layout. Symlinking `kwinrc` commits ten
  desktop UUIDs and sixteen dead layouts keyed to one specific panel.
- **`kdeglobals` `[Colors:*]` / `[WM]` / `[ColorEffects:*]`** — roughly 70% of
  the file, mechanically regenerated by `KColorSchemeManager` from
  `[General] ColorScheme=BreezeDark`. Every value is verbatim upstream Breeze
  Dark. `[General] ColorSchemeHash` is a checksum _of_ that derived block.
- **`dolphinrc` / `katerc` / `konsolerc`** — resolution-keyed window geometry
  (`… screen: 3840x2160 …`), `[MainWindow] State` byte blobs, `Version` and
  `ViewPropsTimestamp` schema stamps, and a `[PreviewSettings] Plugins`
  enumeration of the thumbnailers installed on _this_ box.
- **`ksmserverrc`** — `[Session: saved at previous logout]` ledgers rewritten on
  every logout.
- **`kconf_updaterc`** — a pure migration ledger of `done=`/`ctime=`/`mtime=`
  stamps. Transplanting it actively suppresses migrations the new machine needs.
- **`plasma-org.kde.plasma.desktop-appletsrc`** — panel layout keyed by
  installation-local integer instance IDs cross-referenced from `plasmashellrc`
  and `kactivitymanagerd-statsrc`, plus activity UUIDs and
  `ItemGeometries-3840x2160`.

A symlink is all-or-nothing: it commits every one of those bytes and it makes
Plasma rewrite the git worktree continuously, because KDE rewrites these files
on nearly every settings change. That is why **copy plus per-key merge plus a
volatile strip table** is the default, and why the strip table is the
highest-risk data in the feature (§7, §15).

### 2.2 KConfig destroys structure on every write

`KConfigIniBackend::writeConfig()` re-parses the file into a map and
re-serialises from scratch. There is no comment model: every `#` line, every
blank line, and the original key ordering are lost the first time anything is
written. Preserving a human-authored layout in `plasma/files/` is therefore
impossible in principle, which removes the usual argument for symlinking a
hand-edited file into place.

### 2.3 KConfig does write through symlinks — so `--link` is viable

Symlinking is rejected as the _default_, not as a _capability_. Probe evidence:
files pre-created by the harness at mode `0644` retained `0644` across
`kwriteconfig6` rewrites, while files created by `kwriteconfig6` itself landed
at `0600`. A writer that unlinked and recreated the target would have produced
`0600` in both cases. KConfig therefore opens and rewrites the existing inode
path rather than replacing it, which is exactly the behaviour that makes writes
follow a symlink into the repo.

Consequences that shape §9:

- `--link` genuinely works: KDE will read and write the repo file in place.
- That is also its hazard — a KDE settings change silently commits volatile
  state into the worktree, defeating goal 1. It is opt-in, per run, and guarded.
- KConfig's `QLockFile` guard is `<path>.lock` **in the same directory as the
  path it opened**, i.e. next to the symlink in `~/.config`, not next to the
  repo file. Our writer takes the lock at the same location (§6.7).

---

## 3. Storage layout

```text
plasma/
├── README.md                       # what is managed, what is stripped, why
├── files/                          # captured snapshot — written by `dotf plasma export`
│   ├── baloofilerc
│   ├── dolphinrc
│   ├── katerc
│   ├── katevirc
│   ├── kcminputrc
│   ├── kded5rc
│   ├── kdedefaults/
│   │   ├── kcminputrc
│   │   ├── kdeglobals
│   │   ├── kwinrc
│   │   ├── ksplashrc
│   │   ├── package             # NOT INI — verbatim byte copy
│   │   └── plasmarc
│   ├── kdeglobals
│   ├── kglobalshortcutsrc
│   ├── kiorc
│   ├── klipperrc
│   ├── konsolerc
│   ├── krunnerrc
│   ├── kscreenlockerrc
│   ├── ksmserverrc
│   ├── kwalletrc
│   ├── kwinrc
│   ├── mimeapps.list
│   ├── plasma-localerc
│   ├── plasmanotifyrc
│   ├── plasmarc
│   └── powerdevilrc
└── recommended/                    # hand-authored overlay — never written by export
    ├── README.md
    ├── baloofilerc
    ├── dolphinrc
    ├── kcminputrc
    ├── kdeglobals
    ├── klipperrc
    ├── krunnerrc
    ├── kscreenlockerrc
    ├── ksmserverrc
    ├── kwinrc
    └── plasmarc
```

### 3.1 Rules

- A path under `plasma/files/` is **relative to the resolved KDE config
  directory** (`$XDG_CONFIG_HOME`, else `$HOME/.config`), so
  `plasma/files/kdedefaults/kwinrc` ↔ `<configDir>/kdedefaults/kwinrc`.
- `plasma/files/` is written **only** by `dotf plasma export`. Hand-editing it is
  supported but the codec validates on load and fails loudly (§4.6).
- `plasma/recommended/` is written **only** by hand. `export` never touches it.
  It is a partial overlay: a file there contains just the recommended keys, not a
  whole config.
- `plasma/files/` holds **27 candidate paths** (the `DefaultFiles()` table, §7.2).
  A candidate that reduces to zero keys after stripping is not written, and a
  previously-written file that now reduces to zero keys is deleted (§6.6).
- Nothing else in the repo may claim any of these destinations. `dotf link` and
  `dotf plasma` must never both own a path; this is asserted by a Go test
  (§13.2, `TestNoOverlapWithSymlinkTable`).

### 3.2 Decision: `klipperrc` and `plasmarc` are in the managed set

The live-config audit (research B) did not list `~/.config/klipperrc` or
`~/.config/plasmarc` as managed, but the recommended overlay (research D)
prescribes settings in both. A file that `recommended/` writes but that
`DefaultFiles()` does not know about would be applied by
`dotf plasma apply --recommended` and then never inspected by `diff` or
`status` — silent, unauditable state.

**Decision:** every file referenced by `plasma/recommended/` is also a
`DefaultFiles()` candidate. `klipperrc` and `plasmarc` are added. Both are pure
preference files with no machine-local state: `~/.config/klipperrc` holds
clipboard-history policy, `~/.config/plasmarc` holds `[PlasmaToolTips] Delay`
(the theme name lives in `kdedefaults/plasmarc`, a separate managed path). The
invariant "`recommended/` ⊆ `files/` candidates" is enforced by
`TestCommittedPayload_IsManaged` (§13.2).

**Amended (A3).** Being a `DefaultFiles()` candidate is necessary but was not
sufficient. A file that `plasma/recommended/` names and that has **no**
`plasma/files/` snapshot is a **first-class source** for both `apply` and
`diff`: the overlay alone is a complete source document. The original
implementation resolved the overlay only _after_ an "is the `plasma/files/` copy
there?" early return, which made `--recommended` a silent no-op for four of the
ten overlay files (`baloofilerc`, `klipperrc`, `ksmserverrc`, `plasmarc`) and
made `diff --recommended` report them converged while none of their settings had
ever been applied. The ordering rule is now normative: **resolve the source
document before any presence decision** (§6.6, §12.2).

---

## 4. The KConfig codec

The codec is a byte-faithful Go reimplementation of `KConfigIniBackend`
(kconfig 6.29.0). It is the correctness core of the feature.

### 4.1 Document model

```go
// Doc is a parsed KConfig INI document.
type Doc struct {
    Groups []Group
}

// Group is one configuration group. Path holds the DECODED bracket segments;
// an empty Path is KConfig's "<default>" root group, whose entries are written
// with no header line at the top of the file.
type Group struct {
    Path      []string
    Immutable bool // header carried a trailing [$i] marker
    Entries   []Entry
}

// Entry is one key=value line. Identity is (Group.Path, Key, Locale); RawFlags
// and Value are attributes of that identity.
type Entry struct {
    Key      string // decoded bare key, flag and locale suffixes removed
    Locale   string // locale tag VERBATIM, which is also its on-disk form ("" = none)
    RawFlags string // verbatim inner flag text WITHOUT the '$' ("ie", "d", "xyz"); "" = none
    Value    []byte // decoded value bytes; may not be valid UTF-8
    Deleted  bool   // RawFlags contains 'd' — a tombstone
    HasValue bool   // an '=' was present on the line
}
```

`Value` is `[]byte`, not `string`, because KConfig values are byte strings: a
value byte ≥ 0x7F that is not part of a well-formed UTF-8 sequence is escaped
per-byte as `\xNN`, while the identical byte in a key or group name is written
raw. Modelling all three contexts as Go `string` and re-encoding uniformly
corrupts one of them.

`RawFlags` is preserved verbatim rather than parsed into an `i`/`e`/`d` enum,
because KDE itself drops unknown flag letters: `a[$q]=1` and `c[$xyz]=3` come
back as plain `a=1` / `c=3` after one rewrite. Our codec must not reproduce that
loss.

**Amended (A6) — `Locale` is never decoded.** The original comment called it the
"decoded locale tag". It is not decoded and cannot be: KConfig never decodes a
locale tag, it matches the **raw bytes** against the process locale. Verified
against kconfig 6.29 under `LANG=de_DE.UTF-8` — `Name[de]` is served for German
while `Name[d\x65]` and `Name[ de ]` are not, and both of those survive a full
`kwriteconfig6` rewrite completely untouched. Modelling a non-canonical tag as
its decoded form would be an active corruption: decode `d\x65` to `de`, write
`[de]` back on the next apply, and an entry KDE was ignoring silently becomes
the active German translation. `Parse` therefore refuses a file carrying a
non-canonical tag and `Write` refuses such a document (§4.6), which keeps
`Entry.Locale`'s in-memory form and its on-disk form the same string by
construction.

### 4.2 Escape table

`G` = group-name segment, `K` = key name, `V` = value. "raw" means the byte is
written literally.

| Input | `G` | `K` | `V` | Notes |
| --- | --- | --- | --- | --- |
| `\` (0x5C) | `\\` | `\\` | `\\` | Always, at any position, no type guard |
| LF (0x0A) | `\n` | `\n` | `\n` | Always |
| TAB (0x09) | `\t` | `\t` | `\t` | Always. A literal TAB never survives on disk. `kglobalshortcutsrc` stores `Meta+L\tMeta+Ctrl+Q,…` with a literal backslash-t |
| CR (0x0D) | `\r` | `\r` | `\r` | Always. CRLF input parses fine and is normalised to LF on write |
| `=` (0x3D) | raw | `\x3d` | raw | Parser splits on the **first** `=`; `a=b=c=d` yields value `b=c=d` |
| `[` (0x5B) | `\x5b` | `\x5b` | raw | Values keep brackets: `tiles={"…":[{…}]}` is stored verbatim |
| `]` (0x5D) | `\x5d` | `\x5d` | raw | Same |
| byte < 0x20, not TAB/LF/CR | `\xNN` | `\xNN` | `\xNN` | Backslash, `x`, exactly two **lowercase** hex digits |
| 0x7F (DEL) | raw | raw | `\x7f` | Value context only |
| byte ≥ 0x80 in well-formed UTF-8 | raw | raw | raw | **Amended (A11).** The original note said overlongs and > U+10FFFF are "rejected by the validator". There is no validator: well-formedness is decided sequence by sequence by the escaper's single `utf8.DecodeRune` call, and an overlong form, a surrogate, or a scalar above U+10FFFF simply _is not_ well-formed, so it falls through to the next row. Nothing rejects such input at parse or at write time |
| byte ≥ 0x80 **not** in well-formed UTF-8 | raw | raw | `\xNN` | Group and key contexts emit invalid bytes raw. This is also where an overlong, a surrogate, or a > U+10FFFF sequence lands — per byte, one `\xNN` each |
| leading SPACE — **first byte only** | raw _when the segment carries no other escape_; `\s` when it does | `\s` | `\s` | `'   leading'` → `\s  leading`; subsequent spaces stay literal. **Amended (A6)** — the group column was a flat `raw` and was wrong; see the note below |
| trailing SPACE — **last byte only** | raw _when the segment carries no other escape_; `\s` when it does | `\s` | `\s` | `'trailing   '` → `trailing  \s`; `'   '` → `\s \s`. Same conditional in `G` |
| leading `$` of the **last** group segment | `\x24` | n/a | n/a | Only when the segment is 1–10 bytes and every byte after `$` is `[a-z]`. `$foo` → `\x24foo`; `$FOO`, `$foo1`, `$foobarbazqux` stay literal |
| leading `$` of a **locale tag** | n/a | n/a | n/a | `\x24` in the locale context, unconditionally: without it `Name[$de]` is re-read as a flag suffix and the tag is lost |
| `#` as the **first byte of a key** | n/a | `\x23` | n/a | **Deliberate divergence** — see §4.4 |
| `#` as the **first byte of a locale tag** | n/a | n/a | n/a | **Amended (A11), new row.** The guard is `keyLike` — `ctx == ctxKey ‖ ctx == ctxLocale` — the same predicate as the `=` row, so the escape fires in the locale context too, not in keys only. It is unreachable in practice: `\x23de` is not a fixed point of the locale escaper, so `localeIsCanonical` is false and both `Parse` and `Write` refuse a `#`-leading tag (§4.3). The rule exists to keep the escaper **total**, not because such a tag can round-trip |
| leading U+FEFF (`EF BB BF`) of a **key** | n/a | `\xef` then `BB BF` raw | n/a | **Amended (A6), new row.** Only when the key _starts_ with the BOM. A root-group key that did not escape it would open the file with a BOM, which §4.6 rejects — so `Write` would emit output its own `Parse` refuses (found by `FuzzCodecRoundTrip`). KDE decodes `\xef` to the identical byte, so the key itself is unchanged. This is a **Write-totality rule, not a KDE divergence** |
| `#` elsewhere, `;` `,` `"` `'` `:` `/` `{` `}` `+`, interior spaces | raw | raw | raw | Never escaped in any context |

Decode rules (reader):

| On disk | Decodes to | Notes |
| --- | --- | --- |
| `\s` `\t` `\n` `\r` `\\` | space, TAB, LF, CR, `\` | All contexts, at any position |
| `\xNN` | byte `NN` | Requires two chars after the `x`. A non-hex digit emits a literal `x` **and still consumes both digits**. Fewer than two chars left emits `x` and stops |
| `\;` and `\,` | the **two** characters `\;` / `\,` | `.desktop` compatibility carve-out — the escape is _preserved_, not resolved. This is the codec's one asymmetry (§4.3) |
| lone trailing `\` | **DROPPED** | End of string only. **Amended (A6)** — the original row said "literal `\`" and was wrong. `printableToString` writes the backslash into its output buffer and then breaks out of the loop **without advancing its write cursor**, so the closing `truncate()` cuts the byte it just wrote back off again. Verified against kconfig 6.29 in all three contexts: `a=abc\` reads as the value `abc`, `abc\=1` is the key `abc`, `[abc\]` is the group `abc`. Keeping it as a literal would re-encode to `\\` and hand KDE one character more than the file it came from held |
| a **decoded** 0x1D anywhere in a bracket segment | a **segment boundary** | **Amended (A6), new row.** 0x1D is KConfig's own segment joiner, so KDE splits the segment on the spot: `[a\x1db]` is the group `[a][b]`. `kreadconfig6` answers it for both `--group a --group b` and `--group $'a\x1db'`, and `kwriteconfig6` rewrites the header as `[a][b]`. Our parser splits identically, so one KDE group has exactly one spelling in our model (§4.3) |
| any other `\X` (`\z`, `\X41`, `\101`) | invalid | KDE writes a single backslash, warns, bumps the file error count and returns without truncating, so the value keeps the raw text. 100 such errors abort the whole parse. **Our codec rejects the file instead** (§4.6) |

Whitespace trimming (reader): the whole line is ASCII-trimmed; the key slice and
the value slice are each trimmed again; and the unescaper trims whenever the
string contained a backslash. Unescaped edge whitespace can therefore never
survive — only `\s`, `\t`, `\xNN` forms do.

**Amended (A6) — the trim is what makes the group-segment edge-space rule
conditional rather than exempt.** The original text treated a group segment as
simply exempt from `\s`, on the premise that KDE reads bracket segments
verbatim. That premise holds for **half** of the cases. `KConfigIniBackend`
reads a bracket segment holding **no** backslash verbatim, spaces and all — so
`[ spaced group ]` really is the group `" spaced group "` — but the moment the
segment holds a single escape sequence it is routed through
`printableToString`, which strips ASCII whitespace off both ends of the **RAW**
field _before_ decoding. So `[\sab\s]` keeps both spaces while `[ ab\t ]` loses
them, and `[a\tb ]` is the group `a<TAB>b`, not `a<TAB>b` plus a trailing space. Verified against
kconfig 6.29 with `kreadconfig6`: a file holding `[Libinput][1][2][My\tPad ]`
answers `--group $'My\tPad'` and not `--group $'My\tPad '`.

Our escaper therefore asks the question empirically rather than from a rule
table: it runs a trial pass with the edge-space rule switched off, and escapes
the edge space **iff** that pass emitted a backslash anywhere in the segment.
That keeps output byte-identical to `kwriteconfig6` everywhere KDE is lossless
and diverges only where KDE would silently rename the group (§4.4 divergence
(d)).

### 4.3 Group and key grammar

```text
group-line := '[' seg ']' ( '[' seg ']' )* ( '[$i]' )? junk?
entry-line := key ( '[' locale ']' )? ( '[$' flags ']' )? '=' value
```

- A line is a group header if, after ASCII-trimming, its first byte is `[`.
- Each additional bracket pair appends a path segment. Segments are joined
  internally with **0x1D (GROUP SEPARATOR)** to form the canonical group name.
  The continuation test is strict: the byte immediately after `]` must be `[`
  with no whitespace between.
- **Amended (A6).** Because 0x1D _is_ the joiner, a `Group.Path` segment can
  never contain one. On read, a segment that **decodes** to bytes containing
  0x1D is split into multiple segments on the spot — `[a\x1db]` is the group
  `[a][b]` — which is exactly what KDE does and gives one KDE group one spelling
  in our model; leaving it whole would let two different `Path` values collide on
  the same 0x1D-joined index key. On write, `Write` **rejects** a `Path` segment
  containing 0x1D rather than emitting `\x1d`, which would produce a header our
  own parser would then re-split into a different path.
- Trailing junk after the final `]` that is not `[` is silently ignored by KDE.
  **Our parser rejects it** (§4.6).
- An unterminated header (`[` with no `]`) makes KDE skip the line and keep the
  _previous_ group, silently misfiling every following entry.
  **Our parser rejects it** (§4.6).
- Before any header the current group is the root group (KConfig's literal
  `<default>`), modelled as `Group.Path == nil`. Its entries are written first,
  with no header line.
- An empty header `[]` yields a group with the empty-string name that KConfig can
  address as neither `""` nor `<default>`. **Our parser rejects it** (§4.6).
- Key suffixes are walked **right-to-left**. A suffix is a flag set if its first
  inner byte is `$` and it is non-empty; otherwise it is a locale tag. Only one
  locale tag per key is legal.
- `[$i]` on a header marks the group immutable; a bare `[$i]` line with nothing
  before it marks the whole **file** immutable. The marker is recognised only as
  the last bracket group on the line with exactly the two inner bytes `$i`.
  There is no group-level `[$d]` or `[$e]`.
- `[$d]` is a tombstone: the entry reads as absent but is preserved in the file,
  and the writer strips any `=`.
- `[$e]` marks shell expansion. The value is stored **unexpanded** on disk and
  expanded at read time by KDE. Our codec never expands anything.

### 4.4 Serialisation — canonical form

Output is a pure function of the document. The previous file's byte layout
contributes nothing.

1. **Three ordered passes:** the root group first (no header line), then every
   other group. (KConfig also has a "primary group" pass; `kwriteconfig6` never
   sets one and neither do we, so it is a no-op.)
2. **Group order:** ascending comparison of the 0x1D-joined path as **UTF-16
   code units** — case-sensitive, uppercase before lowercase. This is _not_
   UTF-8 byte order: group `U+1F600` sorts **before** group `U+FFFD` because the
   leading surrogate `D83D` < `FFFD`. `sort.Strings` is wrong here.
3. **Key order within a group:** ascending **UTF-8 byte** comparison of the
   decoded bare key (plain unsigned memcmp, shorter-is-smaller on a prefix tie),
   tie-broken by the decoded locale tag with `""` sorting first. This
   deliberately uses a _different_ comparator from the group order; key `U+FFFD`
   sorts before key `U+1F600`, the exact opposite of the group result for the
   same two names.
4. **Line format:** `<escaped-key>` + optional `[<locale>]` + optional
   `[$<flags>]` + `=` + `<escaped-value>` + LF. A tombstone omits both the `=`
   and the value. No space around `=`, no indentation, no CR anywhere.
5. **Blank lines:** exactly one LF immediately _before_ each group header,
   except before the very first thing written. So: no leading blank line, one
   blank line between consecutive groups, no trailing blank line, and the file
   ends with a single LF after the last entry.
6. **No comments, no BOM, no blank-line preservation, no original ordering.**

7. **Flag normalisation.** `canonicalFlags` renders an entry's flag letters in
   KConfig's own write order, folding in the group header's `[$i]`. Verified
   against `kwriteconfig6` 6.29: `[$e][$i]` and `[$ei]` both come back as
   `[$ie]`, and `[$ii]` collapses to `[$i]`.

   **Amended (A6) — the `d` suppression is not total.** The original text said
   flatly that `d` suppresses the other flag letters. It suppresses the
   **entry's own** letters — `[$di]`, `[$id]`, `[$de]` and `[$ed]` all come back
   as `[$d]` — but immutability **propagated from a group-level `[$i]` survives
   it and is written after it**. Inside `[G][$i]`, every one of `a[$d]`,
   `a[$di]`, `a[$de]` and a bare `a[$d]` tombstone comes back as `a[$di]` from
   `kwriteconfig6` 6.29. Emitting `[$d]` there instead is semantically identical
   — the header still carries the marker, so KDE re-derives the flag either way
   — but it would make the file a fixed point of our writer and **not** of
   KDE's, i.e. a live config rewritten on every single run once the session
   touches it. So we reproduce KDE's asymmetry exactly.

**Deliberate divergences from KDE's own writer** (each must be covered by a test
that documents it):

- **(a) Leading `#` in a key is escaped as `\x23`.** KDE writes `#hashkey=v`
  unescaped and then swallows the line as a comment on the next read — a genuine
  round-trip data-loss bug. KDE decodes `\x23` back to `#` correctly, so our
  output is readable by KDE and lossless.
- **(b) We never emit a BOM, and we reject one on read.** A leading UTF-8 BOM
  makes KDE misroute every entry into the root group, because the first byte is
  not `[`.
- **(c) Locale tags are never re-stamped.** KDE rewrites a matching locale tag
  from the _process_ locale; under `LANG=de_DE.UTF-8` a single `kwriteconfig6`
  call on a file holding `Name=`, `Name[de]=`, `Name[de_DE]=`, `Name[fr]=`
  **silently deleted `Name[de]`**. Our codec treats the locale as part of the
  entry's identity and preserves every variant verbatim. This can change the
  _order_ of the ambient-locale variant relative to KDE's output; it never
  changes content.
- **(d) A group segment carrying both an escape and an edge space writes the
  space as `\s`.** **Amended (A6), new entry.** KDE writes the space raw, trims
  it back off on the next read, and then **merges** the group into the
  same-trimmed-name group if one exists: `kwriteconfig6` collapses `[a\tb ]` and
  `[\sa\tb\s]` into a single `[a\tb]` holding both sets of keys. Losing a group
  identity by silent rename is not acceptable, so we escape.
- **(e) The key of a LOCALIZED entry is escaped like any other key.**
  **Amended (A6), new entry.** KDE writes such a key completely unescaped, so
  `a\x01b[de]` comes back with a raw control byte in the key and an edge space is
  lost outright. Our output is readable by KDE and lossless; KDE's is neither.
- **(f) Unknown flag letters are preserved.** KDE discards them (`[$q]` comes
  back as no suffix at all). Discarding user data is not acceptable, so unknown
  letters are de-duplicated and appended after the known ones in first-seen
  order — stable under re-parse, so it costs one normalisation pass and nothing
  after.

**Write rejects rather than emits** (the totality rules that keep `Write`'s
output inside `Parse`'s accepted language): an empty group segment, a segment
containing the 0x1D joiner, an empty key, a locale tag that is not already
canonical, a duplicate group path, and a duplicate `(key, locale)` within a
group. Each of these would either be unaddressable to KDE or would re-parse as a
different document.

### 4.5 Round-trip semantics and the `\;` asymmetry

On disk, `x\;y` and `x\\;y` **both** decode to the value `x\;y` (two
characters), and both re-encode to `x\\;y`. Decode→encode is therefore not
idempotent for that one input, but it _converges after a single pass_ and is
idempotent from then on.

**Decision — one discipline, applied on both sides: always re-serialise.**

- `export` writes the canonical form into `plasma/files/`.
- `apply` writes the canonical form into the live file — but **only for files it
  actually changes** (§6.5). A converged file is not opened for writing at all,
  so a converged run reflows nothing.
- `diff`, `status`, and `Validate` compare **parsed models at key granularity**,
  never bytes.

Because comparison is semantic **and because flag comparison is made on the
serialised flag string** (§6.1), the `\;` asymmetry, KDE's flag-order
normalisation (`Key[$e][$i]` and `Key[$ei]` both become `Key[$ie]`), and the
group-`[$i]` propagation (`[Immutable][$i]` + `a=1` → `a[$i]=1` on first write)
can never surface as phantom drift. They surface exactly once, as a one-time
normalisation, on the first write of a given file — and `plasma/files/` is
canonical from birth because `export` created it.

The mixed discipline research warned against — populating `plasma/files/` by
verbatim byte copy while `apply` re-serialises — is explicitly rejected: it makes
the first apply reflow the whole file and produce a huge misleading diff.

The one exception is `kdedefaults/package`, which is not INI at all (§4.7).

### 4.6 Fail-loud parsing

KDE's parser is permissive and silently lossy. Ours is strict, because
`plasma/files/` is hand-editable and a silent drop is a settings loss the user
never sees. `Parse` returns `*ParseError{Path string, Line int, Msg string}` and
no document for any of:

| Condition | KDE behaviour | Our behaviour |
| --- | --- | --- |
| Leading UTF-8 BOM | Every entry misfiled into the root group | Reject |
| Key containing an unescaped `[` | `Invalid entry (missing ']')`, the whole line is dropped | Reject |
| Entry line with no `=` and no `[$d]` | `Invalid entry (missing '=')`, the line is dropped | Reject |
| Unterminated group header | Warn, skip line, keep previous group | Reject |
| Empty header `[]` | Unaddressable group | Reject |
| Trailing junk after the final `]` | Silently ignored | Reject |
| Unknown escape `\z`, `\X41`, `\101` | Warn, leave raw, count toward `MAX_ERRORS` (100 aborts the file) | Reject |
| Second locale suffix on one key | `Invalid entry (second locale!?)` | Reject |
| Key that **decodes** to an empty key (`\=v`) | Drops the entry; and for `\[de]=v` re-reads the line as the group header `[de]` and **misfiles everything after it** | Reject |
| Group segment that **decodes** to an empty segment (`[\]`, `[ \ ]`, `[a\x1d]`) | Files the group's entries into the **root** group | Reject |
| Locale tag that is not already canonical (`Name[d\x65]`, `Name[ de ]`) | Keeps it raw; it then matches no locale and is served to nobody | Reject |

**Amended (A6).** The last three rows are new. The first two are consequences of
the lone-trailing-backslash correction (a raw field that is non-empty on disk can
still decode to nothing) and of the 0x1D split; both were previously
unrepresented, and both are silent misfilings in KDE rather than visible errors.
The locale row is new for the reason given in §4.1, and it means the existing
"Unknown escape" row **no longer covers locale tags**: any non-canonical tag is
refused whether its escape is well formed or not.

Duplicate `(group, key, locale)` triples and re-opened group headers are **not**
errors: last occurrence wins and group fragments coalesce, matching KDE. `Key=`
with an empty value is a real entry, not an error.

### 4.7 Non-INI members

`kdedefaults/package` is a single bare line (`org.kde.breezedark.desktop`) with
no group header and no `=`. Feeding it through the INI codec yields an empty
document and a rewrite would blank it. It carries `Format: FormatRaw` in
`DefaultFiles()` and takes a verbatim byte-copy path in export, apply, and diff
(byte equality). `--link` symlinks it like any other file.

### 4.8 Byte-identical round-trip test strategy

Fixtures live in `internal/plasma/testdata/kconfig/`. Two families:

**Family A — canonical fixtures** (produced by `kwriteconfig6`, therefore already
in KDE's canonical form). Assertion:

```go
got, err := codec.Write(codec.MustParse(t, raw))
// require: bytes.Equal(got, raw)
```

This is the strong assertion: our writer reproduces KDE's writer byte for byte.
Family A must include, at minimum, structural fixtures for every row of the
escape table in §4.2 plus the real-shape payloads verified to round-trip through
`kwriteconfig6`:

- tab-joined shortcut lists: `Meta+L\tMeta+Ctrl+Q,Screensaver\tMeta+L,Lock Session`
- JSON blobs with unescaped brackets: `{"layoutDirection":"horizontal","tiles":[{"width":0.25}]}`
- `file://` URLs, dotted timestamps `2026,3,16,13,30,13.892`
- quoted activity strings: `none,none,Switch to activity "Default"`
- nested headers: `[Colors:Header][Inactive]`,
  `[Libinput][1133][50504][Logitech USB Receiver Mouse]`
- flag and locale suffixes: `Path[$e]`, `Locked[$i]`, `Deleted[$d]`,
  `Name[de]`, `Name[de_DE]`

**Family B — non-canonical fixtures** (hand-written: CRLF, `\;` single-backslash
form, `Key[$e][$i]`, `[Immutable][$i]` with unflagged entries, unsorted groups,
duplicate keys). Assertion: **one-pass normalisation, then idempotence**.

```go
once := codec.Write(codec.MustParse(t, raw))
twice := codec.Write(codec.MustParse(t, once))
// require: bytes.Equal(once, twice)
```

**Family C — scrubbed real files.** Copies of the user's actual managed files
with every identifier replaced by a fixed synthetic constant (UUIDs →
`00000000-0000-4000-8000-0000000000NN`, the `[com.anthropic.Claude]` per-install
token → `DEADBEEF…`, MAC/IP/hostname strings removed entirely). These prove the
codec handles the real corpus and drive the strip-rule tests (§13.1). They are
committed; the scrubbing is a hard requirement of §14.

**Family D — fuzz.** `FuzzCodecRoundTrip` in `internal/plasma`: parse arbitrary
bytes; if parsing succeeds, assert `Write(Parse(Write(doc))) == Write(doc)`.
Seeded from Families A–C. Fuzzing runs in the standard `go test` corpus mode in
CI (no long-running fuzz job).

---

## 5. `internal/plasma` API

Package layout, mirroring `internal/symlink`'s file split:

**Amended (A10)** — this is the layout as shipped. The original plan named
`plan.go`, `execute.go` and `support.go`; the actual split fell out of the
~500-line soft cap and of keeping the write-time merge and the lock protocol
each in one reviewable place.

```text
internal/plasma/
├── codec.go        // Doc/Group/Entry, Parse, ParseError, package doc
├── codec_escape.go // escape/unescape, the three escape contexts, locale canonicity
├── codec_write.go  // Write, canonical ordering, canonicalFlags, write-side rejects
├── table.go        // FileSpec, Format, DefaultFiles, Denylist, checkSpec
├── strip.go        // GroupGlob/KeyGlob/ValueGlob matching, colour-scheme guard
├── merge.go        // Converged, Merge, Overlay, DiffKind/KeyDiff/FileDiff, diffDocs
├── plasma.go       // Options, ActionKind, KeyAction, FileAction, Plan, Report, Engine,
│                   // Support, SessionState, DetectSession
├── errors.go       // EVERY typed error (moved out of plasma.go)
├── export.go       // PlanExport, strip, the privacy scan
├── apply.go        // PlanApply — the PLANNING half only
├── applylink.go    // the --link/--unlink state machine: linkMode (the §9.2 matrix),
│                   // planLinkMode, resolveLinkGuard
├── applyexec.go    // Execute and the WRITE-TIME merge (writeMerged, writeAtomic)
├── lock.go         // the <path>.lock QLockFile protocol
├── diff.go         // Diff, Status, volatile classification
├── validate.go     // Validate — a thin caller of Diff
├── reload.go       // ReloadCommand, ReloadPlan, Reload
└── testdata/…
```

The split of the apply path is load-bearing rather than cosmetic: `apply.go` and
`applylink.go` may not mutate the filesystem, `applyexec.go` is the only file
that may, and `lock.go` is a protocol implementation of somebody else's format.
See §6.7 for why the boundary sits exactly there.

**Amended (A20) — `applylink.go` is the fourth file of that split.** The
`--link`/`--unlink` decision is a **closed** decision surface: every cell of the
§9.2 matrix is a contract with the CLI's exit code, so it is reviewable as one
table only if it sits in one file. It was extracted from `apply.go` for that
reason and because `apply.go` is the merge path, which reads better without a
second state machine interleaved into it. The whole file is planning-time and
`os.Lstat`-only; nothing in it writes.

### 5.1 Package doc

```go
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
```

### 5.2 File table

```go
// Format distinguishes the INI members of the managed set from the one raw
// member (kdedefaults/package).
type Format int

const (
    FormatINI Format = iota
    FormatRaw
)

// FileSpec is one managed configuration file. Path is relative to the KDE
// config directory on the live side and to plasma/files/ (or
// plasma/recommended/) in the repo.
type FileSpec struct {
    // Path is the config-dir-relative path, e.g. "kwinrc" or
    // "kdedefaults/kwinrc". Always forward-slash separated.
    Path string
    // Format selects the INI codec or the verbatim byte-copy path.
    Format Format
    // VolatileGroups are group-header globs stripped on export (§7.1).
    VolatileGroups []string
    // VolatileKeys are "<group-glob>/<key-glob>" patterns stripped on export.
    VolatileKeys []string
    // VolatileValues are "<group-glob>/<key-glob>=<value-glob>" patterns; the
    // entry is stripped when its DECODED value matches the value glob. This is
    // the escape hatch for rules a key-name glob cannot express, such as
    // mimeapps.list's per-machine userapp-*.desktop targets.
    VolatileValues []string
    // SessionOwned marks files a running Plasma session rewrites continuously
    // (kwinrc, kglobalshortcutsrc, plasmarc, klipperrc, plasmanotifyrc). It
    // drives a stderr advisory when apply merges into them on a live session
    // and nothing else.
    SessionOwned bool
    // ApplyOrder is the phase a file is written in (§6.8). Lower runs first.
    ApplyOrder int
}

// DefaultFiles returns the authoritative managed-file table. It is a static,
// typed slice — the plasma analogue of symlink.DefaultTable — locked by a
// golden test so a stray addition cannot silently start deleting real
// settings.
func DefaultFiles() []FileSpec

// Denylist returns config-dir-relative path prefixes the engine must never
// read, write, or traverse, regardless of DefaultFiles. It is a hard fence
// against a future glob widening, not merely an absence (§14.2).
func Denylist() []string
```

### 5.3 Options, actions, plan, report

```go
// Options are the run switches. Like symlink.Options this is a flat value with
// no baked-in defaults; the CLI layer constructs it explicitly and owns every
// default. The same value is passed to PlanExport, PlanApply, Execute, Diff,
// and Validate.
type Options struct {
    // DryRun disables every filesystem mutation. It is Execute's first
    // statement short-circuit.
    DryRun bool
    // Backup copies each live file the run will modify into Plan.BackupDir
    // before touching it.
    Backup bool
    // Validate is a CLI-layer switch only; the engine does not read it.
    // Engine.Validate always validates when called.
    Validate bool
    // Link plans ActionSymlink instead of ActionMerge for every selected file.
    Link bool
    // Unlink is Link's inverse: it plans ActionUnlink for every selected file
    // whose live path is a symlink, replacing the link with a real file holding
    // the merged content (§9.1).
    //
    // It is an EXPLICIT opt-in rather than something a plain apply does on its
    // own. Reading a linked live path reads the repo bytes, so a linked file is
    // converged by construction and a plain apply has genuinely nothing to do
    // with it; silently materialising a copy instead would mean every headless
    // `dotf apply --plasma` tore down a deliberate --link deployment. Requiring
    // the flag is also what lets the link-mode guard stay narrow: with no
    // --unlink there is no transition to refuse. MUTUALLY EXCLUSIVE with Link;
    // see LinkUnlinkError (§5.8, §9.2).
    Unlink bool
    // Recommended layers plasma/recommended/ on top of plasma/files/ (§12.2).
    // It is MUTUALLY EXCLUSIVE with Link; see LinkRecommendedError (§5.8, §9.2).
    Recommended bool
    // ShowVolatile includes EXPECTED live-only keys — the ones this file's own
    // volatile rules deliberately strip on export — in Diff's per-key output,
    // tagged DiffExpectedVolatile. They are suppressed by default (§5.4).
    ShowVolatile bool
    // Reload requests the post-apply live reload; without it Execute still
    // populates Plan.Reloads so the CLI can print the commands it did not run.
    Reload bool
    // Session is the detected KDE session state, resolved by the CLI via
    // DetectSession so the engine performs no environment lookups.
    Session SessionState
}

type ActionKind int

const (
    ActionMerge      ActionKind = iota // rewrite a live INI file with repo keys merged in
    ActionCreateFile                   // live file absent: materialise it from the repo copy
    ActionCopyRaw                      // FormatRaw verbatim byte copy (either direction)
    ActionExport                       // write a stripped repo file
    ActionPrune                        // delete a repo file that no longer has surviving keys
    ActionSymlink                      // --link: replace the live path with a symlink into the repo
    ActionUnlink                       // --unlink: replace a symlinked live path with a REAL file
    ActionBackup                       // copy a live file into Plan.BackupDir
    ActionSkip                         // already converged, or gated out
)

// KeyAction is one (group, key, locale) write. Action granularity is per key,
// not per file, so `plasma.change=` lines carry real detail.
type KeyAction struct {
    Group  []string // decoded group path segments; nil = root group
    Key    string
    Locale string
    // From is the current live value; nil when the key is absent live.
    From []byte
    // To is the value the run will write.
    To []byte
    // Tombstone is true when the repo entry is a [$d] deletion marker.
    Tombstone bool
}

// FileAction is one file's worth of work.
type FileAction struct {
    Spec    FileSpec
    Kind    ActionKind
    SrcAbs  string // repo-side absolute path
    DestAbs string // live-side absolute path
    Reason  string // human-readable; NEVER switched on by the renderer
    // Keys lists the per-key writes a merge or create will perform. On an
    // action returned in Report.Applied these are the writes that ACTUALLY
    // happened, re-derived at write time.
    Keys []KeyAction
    // Content is the serialised target bytes, and its meaning is DIRECTIONAL.
    //
    //   export  → the PAYLOAD. The privacy scan and the dry-run preview both
    //             see exactly what will land.
    //   apply   → a PREVIEW ONLY, for --dry-run and for reporting. It is a
    //             snapshot of what the merge would produce against the live
    //             file as it stood at plan time; Execute re-derives the real
    //             payload under the lock (§6.7).
    //
    // Nil for ActionSymlink, ActionSkip, ActionPrune, ActionBackup.
    Content []byte
    // Source is the repo-side document an apply writes into the live file:
    // plasma/files/<path> overlaid with plasma/recommended/<path> under
    // --recommended. It is the merge's only durable input — everything else
    // comes from the live file as read under the lock — and it is what makes
    // the write-time re-merge possible at all. Nil for every kind except
    // ActionMerge and ActionCreateFile.
    Source *Doc
    // Restated is set by EXECUTE, never by a planner: the write-time re-merge
    // produced different bytes from the plan-time preview, because the live
    // file changed in between. The write is still correct and still
    // non-destructive, but the user asked to apply one thing and a slightly
    // different thing landed, so it is reported rather than absorbed.
    Restated bool
    // Mode is the file mode to write: the existing file's mode when it exists,
    // else 0o600 (KDE's own mode for freshly created config files).
    Mode os.FileMode
    // ToRepo records the write direction. ActionCopyRaw is bidirectional and
    // every other kind is fixed, but Execute receives only a *Plan and cannot
    // otherwise tell an export plan from an apply plan.
    ToRepo bool
    // Err carries a non-fatal, per-file failure: a KConfig immutability marker,
    // or a file that exists but cannot be read (§6.6). The file is skipped or
    // its write refused, the run continues, and Execute folds Err into its
    // returned error so the CLI prints a per-file line and exits 1.
    //
    // On the plan it is the PLAN-TIME preview of that failure. On an action
    // returned in Report.Applied it is the WRITE-TIME set, re-derived under the
    // lock — so an immutable group that appeared, or vanished, between planning
    // and writing is reported as it was actually found.
    Err error
}

// Plan is the read-only result of planning. Files are ordered by
// FileSpec.ApplyOrder, then path-lexicographically within a phase, except that
// a file's ActionBackup entry is always emitted immediately before that same
// file's mutating action so Execute's in-order loop copies the original before
// overwriting it (§6.9).
type Plan struct {
    Files     []FileAction
    BackupDir string // <configDir>/dotf/plasma_backup_<epoch>, captured once
    // Reloads is populated regardless of Options.Reload, and each command's
    // argv[0] is RESOLVED here, at plan time, by Engine.resolveReloads — see
    // §10.1. ReloadPlan itself stays pure and emits the generic binary name.
    Reloads []ReloadCommand
    // LookAndFeel is the package name read from plasma/files/kdedefaults/package.
    // ReloadPlan is pure and reads nothing, so the name is resolved here, where
    // the file is already open. Empty means "unknown", and the lookandfeel
    // reload is then not planned at all rather than planned with a missing arg.
    LookAndFeel string
    // Advisories are non-fatal notices the CLI prints to stderr, such as the
    // §7.4 colour-scheme guard notice. The engine never writes to a stream.
    Advisories []string
    // LinkConflicts lists the config-dir-relative paths whose mode transition
    // only got planned because --dry-run authorised it. A real run without
    // --backup would be refused outright with a *LinkModeConflictError naming
    // exactly these files, so a preview that stayed silent about them printed a
    // confident plan the real invocation then rejected with exit 1 (§9.3). It is
    // always empty outside --dry-run: on a real run the refusal IS the error.
    LinkConflicts []string
}

// Report is the result of executing a plan.
type Report struct {
    // Applied lists actions actually performed; ActionSkip is excluded, so a
    // converged run reports applied=0. Nil under DryRun. Each entry carries the
    // WRITE-TIME Keys, Content, Mode and Err, so `plasma.change=` reports what
    // actually landed rather than what the plan predicted.
    Applied []FileAction
    // Planned is always the full plan, skips included.
    Planned []FileAction
    DryRun  bool
    // Restated lists the config-dir-relative paths whose write-time re-merge
    // differed from the plan-time preview — the live file changed between
    // planning and writing, so the bytes that landed derive from the newer live
    // file. Never a failure; the caller renders it as a notice.
    Restated []string
    // Reloaded lists the reload commands actually run (empty unless
    // Options.Reload and a live session). No ENGINE path assigns it: Execute
    // does not reload, and Engine.Reload returns the set as its own result. It
    // is filled in by cmd/dotf, which is the only caller holding that result,
    // and read back from there by both renderers (§10.3).
    Reloaded []ReloadCommand
}
```

**Amended (A1).** `FileAction.Source`, `FileAction.Restated`, `FileAction.Err`'s
two-phase meaning, `FileAction.ToRepo`, `Report.Restated`, `Options.ShowVolatile`
and `Plan.LookAndFeel`/`Plan.Advisories` are all new since the original draft.
`Source` and `Restated` exist specifically because the merge moved into the lock
at write time (§6.7); the original `Content`-is-the-payload contract was the
mechanism of the branch's most severe defect.

**Amended (A20) — `Options.Unlink`, `ActionUnlink` and `Plan.LinkConflicts`.**
All three are the `--unlink` feature (§9), and none of them appeared in this
listing before. Two details are worth stating precisely because they are
contracts rather than conveniences:

- **`ActionUnlink` sits between `ActionSymlink` and `ActionBackup`**, so it takes
  iota value 6 and `ActionBackup`/`ActionSkip` shift to 7 and 8 relative to the
  original eight-constant listing. Unlike `DiffKind` (§5.4), whose values are a
  machine contract that new members must be **appended** to, `ActionKind`'s
  numeric values are never serialised: the stdout bucket and the `--json`
  `action` field both carry `ActionKind.String()`, so the kind was placed where
  it reads — next to its inverse — rather than at the end.
- **`ActionUnlink` is a distinct kind, not an `ActionMerge` with a flag.** The
  merge path's convergence short-circuits would otherwise swallow it: a linked
  live path reads the repo's own bytes, so it is always "converged". The kind is
  what carries "the point of this write is the destination's TYPE, not its
  content" through planning, execution (`writeRawLive` exempts it from the
  under-lock convergence recheck) and rendering alike.

**Amended (A14) — `KeyAction.Flags` is deleted.** It was declared to carry "the
`RawFlags` string the entry will carry" and nothing ever read it. It could not
have been useful either: §6.1's normative rule is that every flag comparison in
the package runs on `canonicalFlags` output, so a field holding the raw,
pre-canonical text would have been a second, wrong answer to a question the
package already answers in one place. A field the merge does not consult and the
renderers do not print is a contract that reads as a guarantee and enforces
nothing.

**Amended (A18) — `Report.Reloaded` is populated, by the CLI.** It was
write-only in the original branch: the field was declared, documented, and never
assigned, so every consumer that trusted it saw an empty slice after a
successful reload. It is filled now, but honestly attributed — the assignment
lives in `cmd/dotf/plasmaverbs.go`, because `Engine.Reload` returns the executed
set to its caller rather than writing it back into the report. Moving the
assignment inside the engine would be the tidier home for it and is a
deliberately deferred follow-up, not a hidden difference.

**Not yet rendered by the CLI.** `Report.Restated` is engine-level today: no
`plasma.restated=` stdout line and no JSON field consume it. Surfacing it is a
CLI follow-up. The engine contract is fixed now so the renderer can be added
without another API change.

### 5.4 Diff and status types

```go
type DiffKind int

// New kinds are APPENDED: the values are iota-derived and the strings are a
// machine contract for `dotf plasma diff --json`, so reordering them would
// silently relabel every consumer's data.
const (
    DiffValueMismatch DiffKind = iota // key present both sides, values differ
    DiffMissingLive                   // key in repo, absent live
    DiffMissingRepo                   // key live-only AND UNMANAGED — informational
    DiffFlagsMismatch                 // same value, different [$…] flag string
    DiffExpectedVolatile              // key live-only AND explained by this file's strip rules
)

// String renders the kind for the JSON report's "kind" field:
// "value-mismatch", "missing-live", "missing-repo", "flags-mismatch",
// "expected-volatile".
func (k DiffKind) String() string

// Actionable reports whether a difference is one apply would act on. It is the
// SINGLE definition of drift, used by FileDiff.Drifted, Status's key counter,
// and the CLI's drift/only-live split, so the three cannot disagree about which
// kinds are merely informational.
//
//   func (k DiffKind) Actionable() bool {
//       return k != DiffMissingRepo && k != DiffExpectedVolatile
//   }
func (k DiffKind) Actionable() bool

type KeyDiff struct {
    Group  []string
    Key    string
    Locale string
    Kind   DiffKind
    Repo   []byte
    Live   []byte
}

type FileDiff struct {
    Spec FileSpec
    // RepoPresent / LivePresent record file existence on each side.
    RepoPresent bool
    LivePresent bool
    // Keys holds every difference, sorted by (group, key, locale) using the
    // canonical comparators of §4.4.
    Keys []KeyDiff
}

// Drifted reports whether this file has any difference that apply would act on:
// any KeyDiff whose Kind.Actionable() is true.
func (d FileDiff) Drifted() bool

type StatusReport struct {
    Managed      int // len(DefaultFiles) after --only filtering
    RepoPresent  int
    LivePresent  int
    MissingLive  []string
    MissingRepo  []string
    // Absent lists managed files present on NEITHER side: never exported and
    // never written by this desktop. It is a distinct state from "present" and
    // from either missing set, each of which implies the file exists on the
    // other side. A fresh host has plenty of them.
    Absent       []string
    DriftedFiles []string
    DriftedKeys  int
}
```

**Amended (A14) — `Session` is deleted, `Absent` is added.** `StatusReport.Session`
merely echoed `Options.Session` back at the caller who had just supplied it: the
CLI resolves the session via `DetectSession`, passes it in, and prints
`plasma.session=` from its own value, so the field round-tripped an input and no
reader ever chose it over the input. `Options.Session` (§5.3) is unaffected and
stays — that one the engine genuinely reads.

**Amended (A7) — `DiffExpectedVolatile`.** A live-only key is not one thing, it
is two, and the original four-kind vocabulary could not tell them apart. A key
that matches **this file's own** `VolatileGroups`/`VolatileKeys`/`VolatileValues`
rules is one `export` removes **by design**: it can never appear in
`plasma/files/`, so its absence from the repo is the design working, not news.
A key that matches nothing in the strip table is genuinely unmanaged state the
repo does not know about. Reporting both as `DiffMissingRepo` buried the second
under the first — on a real desktop, several hundred `[Tiling][<uuid>]` and
`Id_*` entries against a handful of interesting ones.

The rules:

- Only `DiffMissingRepo` is ever re-labelled to `DiffExpectedVolatile`; every
  other kind involves a key the repo **does** name, which by definition survived
  the strip.
- The classification uses the **same rule set, compiled the same way**, as
  `Strip` — including the §7.4 colour-scheme guard, resolved once per run
  exactly as `PlanExport` resolves it, so a colour rule the guard disables is not
  a rule this run treats as volatile.
- Expected-volatile entries are **suppressed by default** and included by
  `Options.ShowVolatile`.
- Neither live-only flavour is drift: `Actionable()` excludes both.

The engine mechanics are `diff.go`'s two-step: `classifyVolatile` re-labels the
`DiffMissingRepo` entries the file's own compiled strip rules explain, and
`dropExpectedVolatile` then removes them unless `Options.ShowVolatile` is set.

**Amended (A21) — the flag shipped, and it shipped differently from the design
this paragraph used to prescribe.** The superseded text called the kind "not yet
reachable from the command line" and laid out a three-part follow-up: add
`--show-volatile`, then widen `plasmaDiffLines`' bucket test from
`k.Kind == plasma.DiffMissingRepo` to `!k.Kind.Actionable()` so expected-volatile
entries land in `plasma.only-live=`, then do the same in the drift-key counter.

Part (a) landed as written: `--show-volatile` exists, is wired to
`Options.ShowVolatile`, and is completable (§8.2). Parts (b) and (c) did not, and
should not have:

- Folding expected-volatile keys into `plasma.only-live=` would have recreated
  the exact problem `DiffExpectedVolatile` was introduced to solve one level up.
  The two flavours would have been distinguishable only by reading each line's
  `kind=` attribute, so a handful of genuinely unmanaged keys would again be
  buried under several hundred expected ones — this time in the same bucket, one
  `grep` away from being invisible. `plasmaDiffLines` therefore emits a **third
  bucket**, `plasma.expected-volatile=` (§8.3), switching on `DiffKind` so the
  classification stays the engine's.
- The drift-key counter did not need the widening either: `plasmaDriftTally`
  keeps counting drift by `Kind.Actionable()` — the engine's single definition —
  and returns the expected-volatile count as a **separate** third value, surfaced
  as `summary.volatileKeys` (§8.5). `driftKeys` is what apply would act on and
  `volatileKeys` is what export strips by design; neither is derivable from the
  other by subtraction.

`Options.ShowVolatile` is read by `Diff` only, so on `apply` — whose `Validate`
resolves to the same comparison — the flag widens the compared set but changes
nothing the verb renders. That is inert rather than wrong, and it is why
`--show-volatile` is deliberately **not** one of the verb-gated flags (§8.2).

### 5.5 Engine and methods

**Amended (A11) — the struct carries seven fields, not three.** The original
listing showed only the three constructor parameters; the other four are
derived, are load-bearing, and are described elsewhere in this spec (§7.4 for the
data home and the system scheme dir, §6.7.2 for the 45s/100ms lock budget).

```go
// Engine plans and executes plasma operations against a repo root and a KDE
// config directory.
type Engine struct {
    repoRoot  string
    configDir string
    r         exec.Runner
    // dataHome is the XDG data home derived from configDir. It is only read by
    // the colour-scheme guard (§7.4).
    dataHome string
    // systemColorSchemes is the distribution colour-scheme directory; a field
    // rather than a constant so tests stay hermetic.
    systemColorSchemes string
    // lockTimeout and lockPoll govern <path>.lock acquisition (§6.7.2). Fields
    // rather than constants so a test can prove the timeout without waiting 45s.
    lockTimeout time.Duration
    lockPoll    time.Duration
}

// NewEngine constructs an Engine. repoRoot is the dotfiles source tree;
// configDir is the resolved KDE config directory ($XDG_CONFIG_HOME, else
// $HOME/.config). The Runner is used only by Reload.
//
// The four derived fields are set here: dataHome as <parent of configDir>/
// .local/share, systemColorSchemes as systemColorSchemesDir, and the lock budget
// as defaultLockTimeout (45s) / defaultLockPoll (100ms).
//
// The XDG data home is derived from configDir rather than taken as a parameter:
// it is read only by the colour-scheme guard, which is a read-only probe with a
// system-directory fallback, so an extra constructor parameter would buy nothing.
func NewEngine(repoRoot, configDir string, r exec.Runner) *Engine

// PlanExport computes the live → repo plan: read each live file, strip its
// volatile groups/keys/values, serialise canonically, and emit ActionExport,
// ActionCopyRaw, ActionPrune, or ActionSkip per file. Reads the filesystem;
// writes nothing.
func (e *Engine) PlanExport(files []FileSpec, opts Options) (*Plan, error)

// PlanApply computes the repo → live plan: resolve each file's SOURCE document
// (plasma/files/ overlaid with plasma/recommended/ when Options.Recommended),
// merge it into the live document, and emit ActionMerge, ActionCreateFile,
// ActionCopyRaw, ActionSymlink, ActionBackup, or ActionSkip per file. Reads the
// filesystem; writes nothing.
//
// It stores the SOURCE document on each action and a PREVIEW of the merged
// bytes. The preview is for --dry-run and reporting only; Execute re-derives the
// payload under the lock (§6.7).
func (e *Engine) PlanApply(files []FileSpec, opts Options) (*Plan, error)

// Execute performs a plan's mutations in order. It never re-plans — the SET of
// files and the operation chosen for each are fixed at plan time — but for a
// live INI write it DOES re-derive the bytes, under the lock, from the live file
// as it exists at write time (§6.7). DryRun is the first-statement
// short-circuit, so a dry run has provably zero filesystem effect.
//
// It returns a non-nil Report alongside any error so partial progress is
// visible, and a per-file failure never aborts the remaining files. The one
// deliberate exception is a failed ActionBackup, which aborts the action it was
// emitted to protect (§6.9).
func (e *Engine) Execute(plan *Plan, opts Options) (*Report, error)

// Diff computes per-key drift in both directions. It is the single source of
// truth for convergence: PlanApply, Validate, Status, `dotf plasma diff`, and
// doctor all resolve to this comparison. Returns the full results AND a
// *DriftError when any file has an actionable difference; results are never
// discarded on error.
func (e *Engine) Diff(files []FileSpec, opts Options) ([]FileDiff, error)

// Validate is the post-apply audit. It is a thin caller of Diff — deliberately
// not a second implementation, so the validated set can never drift from the
// applied set.
func (e *Engine) Validate(files []FileSpec, opts Options) ([]FileDiff, error)

// Status summarises the managed set without computing full key detail beyond
// the drift counts.
func (e *Engine) Status(files []FileSpec, opts Options) (*StatusReport, error)

// Reload runs the plan's reload commands through the injected Runner. It is a
// no-op returning (nil, nil) when Options.Reload is false or
// Options.Session != SessionLive.
func (e *Engine) Reload(plan *Plan, opts Options) ([]ReloadCommand, error)
```

### 5.6 Platform and session gating

```go
type SupportLevel int

const (
    SupportUnsupported SupportLevel = iota // macOS, Windows
    SupportSupported                       // any Linux family, WSL included
)

// Support classifies a host without reading or running anything, mirroring
// sysconfig.Support. It lets doctor and apply skip the phase without
// constructing an Engine.
func Support(info platform.Info) SupportLevel

type SessionState int

const (
    SessionUnknown SessionState = iota
    SessionAbsent
    SessionLive
)

func (s SessionState) String() string // "unknown" | "absent" | "live"

// DetectSession classifies the KDE session from the environment. getenv is
// injected so the engine performs no environment lookups and the CLI passes
// os.Getenv. A session is Live when KDE_FULL_SESSION == "true" OR
// XDG_CURRENT_DESKTOP contains "KDE" (colon-separated, case-insensitive).
// It is Unknown when neither variable is set at all (a non-graphical shell),
// and Absent when they are set to something else (a different desktop).
func DetectSession(getenv func(string) string) SessionState
```

### 5.7 Reload types

```go
type ReloadCommand struct {
    // Component names what the command reloads: "lookandfeel", "kwin",
    // "plasmashell", "kglobalaccel".
    Component string
    // Argv is the command and arguments. Never a shell string.
    Argv []string
    // Reason names the managed file that triggered it.
    Reason string
}

// ReloadPlan derives the reload set from the files a plan will actually
// modify. Pure: takes no Runner and reads nothing.
func ReloadPlan(plan *Plan) []ReloadCommand
```

### 5.8 Error types

All are pointer-receiver structs whose message begins with the prefix
`plasma:` followed by a space, matched by the CLI with `errors.As` on a `**T`.
Every member is **constructed on some production path**: an exported error the
CLI can never receive is a contract that reads as a guarantee and enforces
nothing.

```go
// MissingSourceTreeError: the dotfiles repo root does not exist.
type MissingSourceTreeError struct{ Root string }
// "plasma: dotfiles source tree %q does not exist"

// MissingRepoTreeError: repoRoot exists but plasma/files/ does not — export has
// not been run yet. Returned by PlanApply only.
type MissingRepoTreeError struct{ Dir string }
// "plasma: %q does not exist; run `dotf plasma export` first"

// NoLiveFilesError: no managed file exists live. Returned by PlanExport only.
type NoLiveFilesError struct{ ConfigDir string }
// "plasma: no managed KDE config files found under %q"

// UnsupportedOSError: the host is not Linux.
type UnsupportedOSError struct{ OS platform.OS }
// "plasma: unsupported on %s (Linux only)"

// UnsafePathError: a FileSpec.Path escapes the config dir or is absolute.
type UnsafePathError struct{ Path string }
// "plasma: unsafe path %q must be relative to the config dir and free of .."

// DenylistError: a path resolves inside a Denylist() prefix.
type DenylistError struct{ Path string }
// "plasma: refusing to touch %q (denylisted: secrets or machine identity)"

// UnreadableFileError: a managed file EXISTS but cannot be read — EACCES on a
// root-owned rc file, EISDIR where a file is expected, EIO on failing media.
// The distinction from ABSENCE is load-bearing (§6.6): absence is a normal
// state with a defined action, unreadability is a per-file failure. Every read
// on a managed path goes through readOptional, which is the only place allowed
// to decide which of the two it is. Unwraps to the filesystem cause.
type UnreadableFileError struct {
    Path string
    Err  error
}
// "plasma: %q exists but cannot be read: %v"

// ParseError: a KConfig file failed strict parsing (§4.6).
type ParseError struct {
    Path string
    Line int
    Msg  string
}
// "plasma: %s:%d: %s"

// PrivacyLeakError: a file about to be written into plasma/files/ contains an
// absolute home path. Returned by PlanExport BEFORE any write (§14.1).
type PrivacyLeakError struct {
    Path    string
    Line    int
    Pattern string
}
// "plasma: %s:%d matches %s; refusing to export an absolute home path"

// ImmutableError: the live file or group carries a KConfig immutability marker.
type ImmutableError struct {
    Path  string
    Group []string // nil = whole file
}
// "plasma: %q is marked immutable by KConfig ([$i]); refusing to write"

// LinkModeConflictError: a file is symlinked into the repo but the run plans a
// merge, or vice versa. Mirrors symlink.LinuxDesktopGuardError: returned from
// PlanApply BEFORE any mutation, satisfiable by --backup or --dry-run.
type LinkModeConflictError struct{ Paths []string }
// "plasma: %d file(s) would switch between --link and merge mode: %s;
//  re-run with --backup (or --dry-run to preview)"

// LinkRecommendedError: --link and --recommended were combined. The two are
// mutually exclusive by construction, not by oversight (§9.2). Returned by
// PlanApply BEFORE any per-file work, because the combination is incoherent for
// every file at once and a partial run would be worse than none.
type LinkRecommendedError struct{}
// "plasma: --link cannot be combined with --recommended: a linked file IS the
//  plasma/files/ snapshot, so the recommended overlay would never land while
//  diff and validate would keep reporting it as drift; drop one of the two flags"

// LinkUnlinkError: --link and --unlink were combined. They are exact inverses,
// so there is no coherent per-file resolution and no order in which both could
// be honoured. Returned by PlanApply immediately after the LinkRecommendedError
// check and before any per-file work, for the same reason (§9.2).
type LinkUnlinkError struct{}
// "plasma: --link cannot be combined with --unlink: they are exact inverses
//  (--link replaces each managed file with a symlink into the repo, --unlink
//  replaces each symlink with a real file); drop one of the two flags"

// BackupRefusedError: a mutating action's protective backup could not be
// written, so the action it protects is refused (§6.9). Scoped to that one
// file; the run continues for every other. Unwraps to the copy failure.
type BackupRefusedError struct {
    Path string
    Err  error
}
// "plasma: refusing to modify %q because its backup failed: %v"

// LockError: <path>.lock could not be acquired within the timeout.
type LockError struct {
    Path string
    Held time.Duration
}
// "plasma: could not acquire %q within %s (a running Plasma session may hold it)"

// DriftError is the many-item collector shape, mirroring
// symlink.BrokenLinkError. Returned by Diff and Validate alongside full results.
type DriftError struct{ Files []string }
// "plasma: %d file(s) differ from the repo: %s"
```

**Amended (A20) — `LinkUnlinkError` was missing from a listing that claims to be
exhaustive.** It is constructed on a production path, as the rule above demands:
`PlanApply` returns it for the `--link`+`--unlink` pair, and `cmd/dotf`'s
`plasmaCheckExclusiveFlags` constructs the same value to print its message, so
the wording stays single-sourced across the two rejection sites (§8.2, §8.6).

**Amended (A14) — there is no `OwnershipConflictError`, and there is no runtime
ownership check.** The original listing declared one for "a managed path is also
claimed by `symlink.DefaultTable`", and it was exactly the dead contract the rule
above forbids: no path constructed it. It could not usefully exist, either.
The §3.1 disjointness guarantee is a property of two **compile-time** tables,
`symlink.DefaultTable()` and `DefaultFiles()`, so a runtime check would recompute
a constant answer on every single command and could only ever fire on a build
that was already broken — reporting a maintainer's table edit to an end user, at
the wrong time, in the wrong process.

The invariant is enforced at **build time** instead, by
`TestNoOverlapWithSymlinkTable` in `internal/plasma/plasma_test.go` (§13.2): it
intersects the two tables and fails the build the moment a widening of either one
double-claims a path. That is where a constant-answer invariant belongs, and it
catches the violation in the commit that introduces it rather than on a user's
machine.

---

## 6. Merge semantics

### 6.1 The convergence predicate is a subset check, not file equality

The single most important rule. A live KConfig file always has more keys than
the repo copy: everything the strip table removed on export still lives there,
plus everything Plasma has written since. Reaching for `bytes.Equal` or a file
hash reports permanent drift forever.

**Converged(file)** ⇔ for every entry `(group, key, locale)` in the repo
document, the live document has an entry with the same identity whose `Value`
is byte-equal and whose **serialised flag string** is equal.

**Amended (A15) — that predicate is computed in exactly one place, and
`Converged` is a thin wrapper over it.** The comparison itself is `diffDocs`;
`Converged(repo, live)` is defined as "`diffDocs` found no **actionable**
difference", which is the same triple spelled the other way round — every
repo-side kind (`DiffMissingLive`, `DiffValueMismatch`, `DiffFlagsMismatch`) is
actionable and every live-only kind is not. So `PlanApply`'s convergence
short-circuit, `Diff`, `Validate`, `Status`, and doctor all resolve to one loop,
and `Diff` is the reporting face of that loop rather than a second source of
truth.

```go
func Converged(repo, live *Doc) bool {
    for _, d := range diffDocs(repo, live) {
        if d.Kind.Actionable() {
            return false
        }
    }
    return true
}
```

The original code did **not** work that way, and the spec's "implemented once"
claim was aspirational: `Converged` and `diffDocs` were two independently written
loops over the same triple. Any fix to one comparison rule left `PlanApply`
(which resolves to `Converged`) and `Diff` (which resolves to `diffDocs`)
disagreeing about the same file — apply rewriting it on every run while diff
reported it converged and validate agreed with diff. There is genuinely one
implementation now, and `TestConvergedMatchesDiffDocs` (`merge_test.go`) is the
anti-divergence tripwire that keeps it that way: it asserts the two agree across
the comparison's edge cases, so re-splitting them into a second loop fails the
build. It passes trivially today, which is precisely the point — a tripwire that
can only start failing once someone reintroduces the duplication.

**Amended (A6) — flags are compared as SERIALISED, never as `Entry.RawFlags`.**
The original text said "whose `RawFlags` string is equal", and that is a bug, not
a simplification. `RawFlags` is the verbatim inner text as it was on disk;
what actually reaches disk is `canonicalFlags(RawFlags, group.Immutable,
Deleted)` — the group's `[$i]` folded in, the tombstone folded in, the letters
reordered. Comparing `RawFlags` therefore reported permanent, unfixable phantom
drift for exactly the case §4.5 promises cannot produce it: a repo group written
`[G][$i]` with unflagged entries, applied, and then re-read with `[$i]` on every
entry. Each side's `RawFlags` differs forever while the files agree byte for
byte, and every subsequent apply rewrites the file to no effect.

**The rule, normative and single:** every flag comparison in the package —
`diffDocs`'s `DiffFlagsMismatch` test, which is what `Converged` resolves to
(A15), and `Merge`'s already-matches skip — runs on `canonicalFlags` output for
**both** sides. This
is what makes §4.5's claim about group-`[$i]` propagation true rather than
aspirational, and it is repeated in `diff.go`'s doc comment so the invariant is
visible where a future edit would break it.

### 6.2 Merge rules

**Amended (A1) — where the merge runs.** The merge function below is pure and
unchanged. What changed is **when** it is called, and that change is the single
most important ordering rule in the package.

`Merge` is called **inside `Execute`, inside the lock**, against the live
document as it exists at **write** time. `PlanApply` calls it too, but only to
produce a preview; it stores the repo-side **source** document `R` on the action
(`FileAction.Source`) and that is the merge's only durable input. Everything
else — `L`, the mode, the per-key `Keys`, the immutability refusals — is
re-derived at write time from the file as read under the lock. See §6.7 for why
the previous placement was unsound.

Given repo document `R` (the action's `Source`) and live document `L` (read
under the lock), the merged document `M` is:

1. `M` starts as a deep copy of `L`.
2. For each group `g` in `R`, in `R`'s order:
   - If `L` has no group with path `g`, **create it** in `M`. Its position in
     the output is decided entirely by the canonical group sort (§4.4), not by
     insertion order.
   - If `L`'s group `g` is marked `Immutable`, **skip the whole group**, record
     an `*ImmutableError` for the file, and continue with the next group. The
     run is not aborted.
3. For each entry `(key, locale)` in `R`'s group `g`:
   - **Overwrite** the matching entry in `M` — `Value`, `RawFlags`, `Deleted`,
     and `HasValue` all come from `R`.
   - If `M` has no such entry, **insert** it.
   - A repo tombstone (`RawFlags` contains `d`) is written verbatim as a
     tombstone. That is the only supported way to express "remove this live
     key"; see 6.4.
4. Every entry in `M` that `R` does not name is left **untouched**, including
   its exact `Value` bytes, `RawFlags`, and locale tag.
5. Every group in `M` that `R` does not name is left **untouched**.
6. `M` is serialised canonically (§4.4).
7. A file-level `[$i]` on `L` is refused outright: `M` is nil and the single
   returned error names the file. Because the check now runs under the lock, a
   file that _became_ immutable between planning and writing is refused, and a
   file that stopped being immutable is written — in both cases the answer is
   the one the planner would have given had it seen the current file.

### 6.3 Ordering

Merge never reorders anything, because order is not part of the model: the
canonical serialiser derives the whole layout from the sorted group and key
comparators. Two documents with the same entry set always serialise identically.

### 6.4 Apply never deletes a live key

There is no "prune live keys not in the repo" mode, not even behind a flag.
Deleting a key the user set through System Settings after the last export would
be indistinguishable from deleting a key KDE added on upgrade. The repo can only
_assert_ values.

The escape hatch is a KConfig tombstone: hand-adding `SomeKey[$d]` to a file
under `plasma/files/` causes `apply` to write that tombstone, which KDE reads as
"absent, and do not fall back to the cascaded default". This is documented in
`plasma/README.md`.

### 6.5 A converged file is not opened for writing

If `Converged(file)` holds, `PlanApply` emits `ActionSkip` with
`Reason: "already converged"`, `Content: nil` and `Source: nil`. `Execute` never
opens the file. This is what keeps a converged run from reflowing anything and
what makes `applied=0` the honest signal of convergence.

**Amended (A1) — convergence is re-checked at write time too.** A file that was
_not_ converged at plan time can become converged before the write, because the
session got there first. `writeMerged` handles that for free: the re-merge finds
nothing to change and the bytes match what is already on disk. For a `FormatRaw`
member, whose write is a byte copy with no merge to redo, the check is explicit:
the action is downgraded to `ActionSkip` with
`Reason: "already converged at write time"`. Either way `applied=0` stays
honest.

**Amended (A20) — `--unlink` is the one deliberate exception.** A symlinked live
path reads the repo's own bytes, so it is converged **by construction**; honouring
this rule there is exactly what made `--link` a one-way door (§9.1). Both
convergence checks are therefore suppressed for an `ActionUnlink`, and the run
writes a file whose content is already correct in order to change the
destination's **type**. It is the only case in the package where a converged file
is opened, and it happens only because the user named the flag.

### 6.6 A managed file that does not exist on one side

**Amended (A2, A3) — the matrix is over the SOURCE document, and it has five
states, not four.**

Two corrections to the original table. First, "repo present" means **the source
document is non-empty** — `plasma/files/<path>` overlaid with
`plasma/recommended/<path>` under `--recommended` — resolved **before** any
presence decision. A file the overlay names and the snapshot does not is
"repo present" (§3.2, §12.2). Second, a file is not either present or absent: a
file can be **present but unreadable**, and that state was missing from the
model entirely.

| Situation | `export` | `apply` | `diff` | `status` |
| --- | --- | --- | --- | --- |
| Source absent, live absent | `ActionSkip` "source missing" | `ActionSkip` "nothing to apply" | no `FileDiff` entry | counted in neither `RepoPresent` nor `LivePresent` |
| Source absent, live present | `ActionExport` (if any key survives) | `ActionSkip` "nothing to apply" | `FileDiff{RepoPresent:false}`, no `KeyDiff`s, not drifted | `MissingRepo` |
| Source present, live absent | `ActionSkip` "source missing" | `ActionCreateFile` — the source document is written verbatim-canonical, mode `0o600`, parents `0o755` | every source key as `DiffMissingLive` → drifted | `MissingLive` |
| Source present, live present, all source keys match | `ActionExport`/`ActionSkip` per 6.6.1 | `ActionSkip` "already converged" | no `KeyDiff`s | converged |
| **Either side PRESENT BUT UNREADABLE** (EACCES, EISDIR, EIO) | per-file failure | `ActionSkip` + `*UnreadableFileError`, **never** `ActionCreateFile` | **hard error**, never `LivePresent:false` | **hard error** |

**6.6.0 Present but unreadable.** A read that fails for any reason other than
`fs.ErrNotExist` is a per-file failure, not evidence of absence. The two must
never be collapsed:

- Collapsing them made `apply` plan `ActionCreateFile` over a live file that is
  present and full of live-only settings — a blind wholesale replacement, and
  the one action `--backup` **cannot** protect, because a create has nothing to
  back up. The failure was therefore unrecoverable, which is why this is a
  correctness rule rather than a nicety.
- The same applies to an unreadable `plasma/files/` or `plasma/recommended/`
  file: `apply` skips it with `Reason: "repo file unreadable"` and the typed
  error, rather than silently applying a partial or empty source.
- In `diff` and `status` it is a **hard error** returned to the caller, not a
  `LivePresent: false` that reports the file converged.

The rule is enforced structurally: every read on a managed path goes through one
helper (`readOptional`) which returns `(nil, false, nil)` for `fs.ErrNotExist`
and `*UnreadableFileError` for everything else. There is no other permitted
shape for a managed-path read in this package.

**6.6.1 Empty after strip.** `DefaultFiles()` is a list of _candidates_.
`PlanExport` writes a repo file only when at least one non-volatile entry
survives the strip. If a candidate reduces to zero entries:

- and no repo file exists → `ActionSkip` "empty after strip"
- and a repo file exists → `ActionPrune`, deleting it

This keeps the table stable as the user's config grows, without committing empty
stubs. It is why `baloofilerc`, `ksmserverrc`, `kwalletrc`, `konsolerc`, and
`plasmanotifyrc` are listed as managed today despite holding no portable keys
yet.

`ActionPrune` never deletes anything outside `plasma/files/`.

### 6.7 The write-time merge, atomic writes, modes, and the KConfig lock

**Amended (A1). This section replaces an unsound design — read the rejection
before the replacement.**

#### 6.7.1 The rejected design, and why it could not work

The original spec put the merge in `PlanApply` (read live → parse → merge →
serialise → store the full merged bytes in `FileAction.Content`) and put lock
acquisition in "`Execute`'s per-file write sequence". That splits one
read-modify-write cycle across the plan/execute boundary, with **an unbounded
interactive confirmation prompt in the middle**, and leaves the lock covering
only the rename.

A lock over the rename protects nothing, because the bytes being renamed were
computed from a read that no lock covered. And the window is not theoretical: it
is exactly as long as a human takes to answer `apply KDE Plasma configuration?
[y/N]`, and the files in question are rewritten by a running Plasma session
continuously — that is what `SessionOwned` exists to say.

Reproduced against the built binary through a pty: with repo
`plasma/files/kwinrc` = `[Windows] FocusPolicy=ClickToFocus` and a live file
holding `FocusPolicy=FocusFollowsMouse`, running `dotf plasma apply --only
kwinrc` and writing a new key and a whole new group into the live file while the
prompt was displayed produced a live file of **exactly the plan-time bytes** —
the new key and the entire new group silently deleted, `applied=1`, exit 0, no
warning. A second run mutating an existing live-only key ended with that key
silently reverted to its pre-prompt value.

This violates the feature's central invariant (§6.4: _apply must never delete a
live key the repo does not name_) and it violates it **by design**, not by
accident of implementation. No amount of care inside `Execute` could have fixed
it while the payload was frozen at plan time. Do not reintroduce it: any change
that makes `FileAction.Content` the apply payload again reopens this exact
defect.

#### 6.7.2 The shipped design

**The entire read-modify-write cycle for a live INI file happens inside
`Execute`, inside the lock** (`Engine.writeMerged`). Per file:

1. **Acquire `<dest>.lock`,** and hold it for everything below.
   `os.OpenFile(lockPath, O_CREAT|O_EXCL|O_WRONLY, 0o600)`, write
   `"<pid>\ndotf\n<hostname>\n"` (QLockFile's on-disk format), so a running
   Plasma respects it. On `EEXIST`, poll every 100 ms up to 45 s (KConfig's own
   timeout). A lock older than the timeout **whose recorded pid is not alive** is
   reclaimed exactly once; anything else returns `*LockError`. Both conditions
   are required — a long-running Plasma holding a fresh lock must never be
   evicted. The lock is released in a `defer`.
   For a `--link`ed destination the lock is taken next to the **symlink** in the
   config dir, matching where KConfig takes it (§2.3).
2. **Read the live file** through `readOptional` (§6.6), so unreadable is
   distinguished from absent.
3. **Capture the mode**: the existing file's `Perm()`, else `0o600`.
4. **Parse** the live bytes.
5. **Merge** the action's `Source` document into the live document (§6.2), which
   also produces the write-time `Keys` and any `*ImmutableError`s.
6. **Serialise** canonically (§4.4).
7. `os.CreateTemp(filepath.Dir(dest), ".dotf-plasma-*")` — **same directory**, so
   the subsequent rename is same-filesystem and therefore atomic. Do not reuse
   `sysconfig`'s `FileSystem.WriteTemp`, which uses `os.CreateTemp("")` and can
   land on another mount.
8. Write the bytes, `Chmod` to the captured mode explicitly (so the process
   umask cannot weaken it), `Sync`, `Close`.
9. `os.Rename(tmp, dest)`.
10. Release the lock.

A crash at any point leaves either the old file or the new file, never a
truncated one.

`PlanApply`'s job is correspondingly narrowed: it resolves and stores the
**source** document (`FileAction.Source`) and a **preview** of the merged bytes.
The plan's `Content` for an apply is a preview for `--dry-run` and reporting —
**never the payload**.

#### 6.7.3 What happens when the live file changed in between

**Re-merge and proceed, then report the divergence.** Aborting would be the
wrong answer: the merge is deterministic and non-destructive by construction (it
starts as a copy of live and only ever overwrites the keys the repo names), so
the newer live file is a strictly _better_ input than the stale snapshot, and
refusing would fail a run whose result is correct.

But what the user is not told cannot be assessed. So when the write-time bytes
differ from the previewed bytes:

- the action's `Restated` is set,
- its `Reason` gains the suffix
  `" (re-merged at write time: the live file changed after the plan was
  computed)"`,
- and its path is appended to `Report.Restated`.

`Report.Applied` carries the **write-time** `Keys`, `Content`, `Mode` and `Err`,
so `plasma.change=` reports what actually landed rather than what the plan
predicted.

#### 6.7.4 The other action kinds

- **`ActionCreateFile`** takes the same `writeMerged` path. A live file that
  appeared between planning and writing is merged into rather than clobbered,
  which is the same guarantee by the same mechanism.
- **`ActionCopyRaw` to the live side** has no merge to redo, but the **decision**
  to write is still re-made under the lock: a raw file the session brought into
  agreement while the prompt was up is downgraded to `ActionSkip` with
  `Reason: "already converged at write time"` and `Restated` set, instead of
  being pointlessly rewritten.
- **`ActionSymlink`** replaces steps 2–9 with `os.Remove(dest)` +
  `os.Symlink(srcAbs, dest)`.
- **`ActionUnlink`** — **new (A20)** — takes the ordinary live-write path,
  `writeMerged` for an INI member and `writeRawLive` for a raw one, since the
  only difference between the two is whether there is a document to re-merge and
  `Format` already says which. No special replace step is needed: `writeAtomic`
  renames over the directory **entry**, so the symlink is replaced rather than
  followed into the repo. It is the one kind exempted from the raw path's
  write-time convergence recheck described in the bullet above, because a linked
  path's bytes are converged by construction and the point of the write is the
  destination's type (§9.1).
- **`ActionExport` and `ActionCopyRaw` to the repo side** take no lock — Plasma
  has no idea `plasma/files/` exists — with one exception: a `--link`ed
  destination means KDE is writing the repo file _through_ the symlink and takes
  its lock beside the symlink in the config dir, so that is where the exporter
  takes it too.

### 6.8 Apply phase ordering

`FileSpec.ApplyOrder` gives three phases. Within a phase, files are sorted
path-lexicographically for determinism.

| Order | Files | Why |
| --- | --- | --- |
| `0` | everything under `kdedefaults/` | The look-and-feel overlay is what regenerates the `[Colors:*]`/`[WM]` blocks that export strips. It must land before anything can trigger a palette recompute |
| `1` | `kdeglobals` | Depends on the scheme named by `kdedefaults/kdeglobals` |
| `2` | every other managed file | Order-independent |

The optional live reload runs strictly after all three phases.

### 6.9 Backups

Unlike a symlink replace — where the repo copy is the truth and the link is
trivially recreatable — a bad merge destroys live settings that exist nowhere
else. **Every file a merge, copy, symlink, or unlink touches is backup-eligible;
there is no per-spec `BackupEligible` flag.** (`needsBackup` additionally
requires the destination to exist — via `os.Stat`, so a linked destination backs
up its target content and a dangling link has nothing to preserve — which is why
an `ActionCreateFile` never gets a backup entry: a create has nothing to back
up.)

`Plan.BackupDir` is computed **once**, inside `PlanExport`/`PlanApply`, as
`filepath.Join(configDir, "dotf", "plasma_backup_"+strconv.FormatInt(time.Now().Unix(), 10))`,
even when `Options.Backup` is false. Determinism per plan is what lets a test
read back `filepath.Join(plan.BackupDir, spec.Path)` afterwards, and it means
`Execute` never re-derives a timestamp mid-run.

The directory lives under `<configDir>/dotf/` rather than directly in the config
dir so a stray `kwinrc` copy can never be picked up by KDE's own scanning.
Backups are never pruned and never auto-restored.

`ActionBackup` entries are emitted in the plan **before** the action they
protect, so `Execute`'s in-order loop always copies the original before
overwriting it.

**Amended (A4) — a FAILED backup refuses the action it protects.** Ordering
alone was not enough. `Execute`'s contract is aggregate-and-continue: a per-file
failure is recorded and the loop walks on to the next plan entry. But because a
file's `ActionBackup` is ordered _immediately_ before that same file's mutating
action, "the next plan entry" after a failed backup is **precisely the
unprotected write** the backup existed to authorise — and `--backup` is what
authorises the destructive step in the first place (a `--link` transition over a
real file is refused without it, §9.3).

So a failed `ActionBackup` records the cause against that file's path, and the
mutating action for that path is then refused with `*BackupRefusedError`
(unwrapping to the copy failure). The refusal is **scoped to that one file**:
every other file in the run proceeds normally, which keeps the aggregate
contract intact. A backup that names a symlinked destination follows the link
deliberately — what needs preserving is the content KDE is reading, not the
link.

---

## 7. Volatile stripping

### 7.1 Glob syntax

Three pattern families, all matched against the **decoded** model, never against
raw bytes.

**Group globs** are the literal KConfig header text including brackets. `*`
matches exactly **one** bracket segment.

```text
[Tiling]           matches only the bare one-segment group
[Tiling][*]        matches [Tiling][<uuid>] and nothing longer
[Tiling][*][*]     matches [Tiling][<uuid>][<uuid>]
[Session: *]       matches a single segment whose text starts "Session: "
[Colors:*]         matches a single segment whose text starts "Colors:"
```

Inside a segment, `*` also acts as a text wildcard matching any run of
characters (including none). A pattern with N bracket pairs matches only groups
with exactly N segments; list a shorter form separately if the parent group must
go too.

**Key globs** are `<group-glob>/<key-glob>`. `[*]` as the group glob means any
group of any depth. The key glob uses `*` as a text wildcard.

```text
[Desktops]/Id_*
[Effect-*]/*Time*
[*]/Recent *
[MainWindow]/*Width*
```

**Value globs** are `<group-glob>/<key-glob>=<value-glob>`, matched against the
decoded value.

```text
[*]/*=*userapp-*.desktop*
```

Note the leading and trailing `*`: a value glob is matched against the **whole**
decoded value, and `mimeapps.list` stores semicolon-terminated **lists** of
desktop ids, so an anchored pattern matches nothing (§7.3.1).

### 7.2 Matching order and normalisation

1. Resolve the colour-scheme guard (§7.4) — it can disable a subset of rules.
2. **Group rules first.** A group whose path matches any `VolatileGroups`
   pattern is removed whole; its keys are never considered.
3. **Key rules next**, over surviving groups.
4. **Value rules last**, over surviving keys.
5. **Key-name normalisation:** globs match the **decoded bare key**, with the
   locale suffix and the `[$…]` flag suffix already removed by the parser. So a
   pattern written `Some Key` strips the on-disk entry `Some Key[$e]` and the
   locale variant `Some Key[de]` alike — one pattern covers every flag and
   locale spelling of that key, and a pattern is never written with a suffix.
   (**Amended (A8)** — this rule was originally illustrated with
   `[KFileDialog Settings]/View Style`, which is no longer a strip rule; see
   §7.3.1. **Amended (A25)** — the replacement illustration then claimed the
   `exclude folders[$e]` recommended key "is written as `exclude folders` in the
   recommended file". It is not, and must not be:
   `plasma/recommended/baloofilerc` carries the `[$e]` suffix verbatim, exactly
   as §12.3 requires, because there the suffix is load-bearing **data** that
   governs read-time expansion. The normalisation described here is a property
   of the strip **pattern**, not of any file.)
6. **Group-path matching uses decoded segments**, so `[with\ttab]` on disk is
   matched by a pattern containing a literal tab, not by `\t`.

### 7.3 The per-file table

`SessionOwned` marks files a running Plasma rewrites continuously.
`Ord` is `ApplyOrder`.

| Path | Fmt | Ord | Owned | Volatile groups | Volatile keys | Volatile values |
| --- | --- | --- | --- | --- | --- | --- |
| `kdedefaults/kdeglobals` | INI | 0 | no | — | — | — |
| `kdedefaults/kwinrc` | INI | 0 | no | — | — | — |
| `kdedefaults/kcminputrc` | INI | 0 | no | — | — | — |
| `kdedefaults/ksplashrc` | INI | 0 | no | — | — | — |
| `kdedefaults/plasmarc` | INI | 0 | no | — | — | — |
| `kdedefaults/package` | **Raw** | 0 | no | — | — | — |
| `kdeglobals` | INI | 1 | no | `[Colors:*]`, `[Colors:*][*]`, `[ColorEffects:*]`, `[WM]`, `[DirSelect Dialog]`, `[KFileDialog Settings][*]` | `[General]/ColorSchemeHash`, `[General]/fixed`, `[General]/font`, `[General]/menuFont`, `[General]/smallestReadableFont`, `[General]/toolBarFont`, `[KFileDialog Settings]/Speedbar Width`, `[KFileDialog Settings]/Recent *` | — |
| `kwinrc` | INI | 2 | **yes** | `[Tiling]`, `[Tiling][*]`, `[Tiling][*][*]`, `[Session*]`, `[SubSession*]`, `[Screens]`, `[Xwayland]` | `[Desktops]/Id_*`, `[Effect-*]/*Time*`, `[Compositing]/LatencyPolicy`, `[Compositing]/OpenGLIsUnsafe`, `[Compositing]/glCore`, `[Compositing]/Backend` | — |
| `kglobalshortcutsrc` | INI | 2 | **yes** | `[ActivityManager]`, `[com.anthropic.Claude]`, `[khotkeys]` | `[*]/switch-to-activity-*`, `[*]/*-Ctrl+Alt+Space` | — |
| `kcminputrc` | INI | 2 | no | `[Keyboard]` | `[Mouse]/cursorSize` | — |
| `kiorc` | INI | 2 | no | — | — | — |
| `kded5rc` | INI | 2 | no | — | — | — |
| `kscreenlockerrc` | INI | 2 | no | — | — | — |
| `powerdevilrc` | INI | 2 | no | — | — | — |
| `krunnerrc` | INI | 2 | no | `[PlasmaRunnerManager]` | `[General]/migrated*` | — |
| `ksmserverrc` | INI | 2 | no | `[Session: *]`, `[LegacySession: *]`, `[Session*]`, `[LegacySession*]`, `[Screen*]` | — | — |
| `kwalletrc` | INI | 2 | no | `[org.freedesktop.secrets]`, `[Auto Allow]`, `[Auto Deny]` | `[Wallet]/First Use`, `[Wallet]/Default Wallet` | — |
| `dolphinrc` | INI | 2 | no | `[KFileDialog Settings][*]`, `[Search]`, `[MainWindow][*]` | `[General]/Version`, `[General]/ViewPropsTimestamp`, `[PreviewSettings]/Plugins`, `[MainWindow]/State`, `[MainWindow]/ToolBarsMovable`, `[MainWindow]/*Height*`, `[MainWindow]/*Width*`, `[KFileDialog Settings]/Speedbar Width`, `[*]/Recent *` | — |
| `konsolerc` | INI | 2 | no | `[MainWindow][*]`, `[Favorite Profiles]` | `[General]/ConfigVersion`, `[MainWindow]/State`, `[MainWindow]/*Height*`, `[MainWindow]/*Width*` | — |
| `katerc` | INI | 2 | no | `[MainWindow]`, `[MainWindow][*]`, `[Recent Files]`, `[Session Data]`, `[KTextEditor Renderer]`, `[Restore Documents]` | `[General]/Last Session`, `[General]/PinnedDocuments`, `[*]/* screen: *` | — |
| `katevirc` | INI | 2 | no | — | `[Kate Vi Input Mode Settings]/Macro Contents`, `[Kate Vi Input Mode Settings]/Macro Registers`, `[Kate Vi Input Mode Settings]/Macro Completions` | — |
| `plasma-localerc` | INI | 2 | no | — | — | — |
| `plasmanotifyrc` | INI | 2 | **yes** | `[DoNotDisturb]` | `[Applications][*]/Seen`, `[Services][*]/Seen` | — |
| `plasmarc` | INI | 2 | **yes** | — | — | — |
| `klipperrc` | INI | 2 | **yes** | — | — | — |
| `baloofilerc` | INI | 2 | no | — | `[General]/dbVersion`, `[General]/index version` | — |
| `mimeapps.list` | INI | 2 | no | `[Removed Associations]` | `[Default Applications]/x-scheme-handler/mailto`, `[Default Applications]/message/rfc822`, `[Default Applications]/x-scheme-handler/mid`, `[Default Applications]/x-scheme-handler/webcal*`, `[Default Applications]/text/calendar`, `[Default Applications]/application/x-extension-ics`, `[Added Associations]/x-scheme-handler/mailto`, `[Added Associations]/x-scheme-handler/mid`, `[Added Associations]/x-scheme-handler/webcal*` | `[*]/*=*userapp-*.desktop*` |

27 candidate paths, 1 of them `FormatRaw`.

**Note on `kcminputrc`.** The `[Libinput][vid][pid][Name]` groups _are_ the
setting — stripping them leaves nothing. They are inert on a machine without
that peripheral, so they are conditional, not volatile, and are kept. Only
`[Keyboard]` (rewritten by the session on layout changes) and
`[Mouse]/cursorSize` (display-scale dependent) are stripped.

**Note on `mimeapps.list`.** The enumerated `userapp-*` key names are kept as a
belt-and-braces fallback; the `VolatileValues` rule is the real mechanism,
because Thunderbird mints a fresh random suffix
(`userapp-Thunderbird-GUF4N3.desktop`) on every machine.

#### 7.3.1 Amended rows (A8)

Four rules in the original table were wrong. Three over-stripped a real setting
and one under-stripped a real leak; all four shipped past the golden test, which
pins **which** files are managed and pinned nothing about **what** each glob may
delete. Each is now additionally covered by a per-glob contract test in
`internal/plasma/table_glob_test.go`.

| Row | Was | Is | Why |
| --- | --- | --- | --- |
| `kdeglobals` groups | included `[Icons]` | `[Icons]` **removed** | `iconssettingsbase.kcfg` declares group `Icons` with the single entry `Theme` ("Name of the current icon theme") — an explicit user choice with no derived content. And no `[Icons][*]` rule existed, so the old rule deleted the parent while keeping the size sub-groups that refine it: incoherent as well as lossy. `kdedefaults/kdeglobals` carries only the look-and-feel package's default, so without this the user's own icon theme reached neither git nor a fresh machine |
| `kdeglobals` keys | included `[KFileDialog Settings]/View Style` | **removed** | A plain enum written by the same `KFileWidget::writeViewConfig()` call as the 13 sibling keys the table keeps, and it matches neither rationale for its two surviving neighbours (`Speedbar Width` = pixel width, display-local; `Recent *` = private paths). Proven lost on the author's live host |
| `krunnerrc` keys | `[General]/history*`, `[General]/migrated*` | `[General]/migrated*` only | `krunnersettingsbase.kcfg` declares exactly three `[General]` entries — `FreeFloating`, `historyBehavior`, `RetainPriorSearch` — so the glob could only ever match `historyBehavior`, a genuine preference §12.12 itself recommends. KRunner's typed history lives in `[PlasmaRunnerManager] history`, already removed by the whole-group rule |
| `mimeapps.list` values | `[*]/*=userapp-*.desktop` | `[*]/*=*userapp-*.desktop*` | `[Added Associations]` values are **semicolon-terminated desktop-id lists**, so the anchored form matched none of them, and the enumerated key names do not cover `text/calendar` or `message/rfc822` on that side — leaving a per-machine Thunderbird id able to reach git. The note above ("the `VolatileValues` rule is the real mechanism") was false as written and is true with this change |

### 7.4 Judgement call 1 — strip `kdeglobals` `[Colors:*]` / `[WM]` / `[ColorEffects:*]`, guarded

These blocks are mechanically regenerated by `KColorSchemeManager` from the
scheme named in `kdedefaults/kdeglobals` `[General] ColorScheme`, itself set by
`[KDE] LookAndFeelPackage`. Stripping is correct because:

1. They are ~70% of the file's bytes for zero user intent — every value is
   verbatim upstream Breeze Dark.
2. They churn on every Plasma release that retunes a shade, producing diff noise
   the user did not author.
3. Shipping them creates split-brain: a merge that writes `[Colors:*]` from an
   old export while the live `ColorScheme` has moved leaves a machine whose
   rendered palette contradicts its declared scheme, fixable only by a theme
   re-apply.

`[General] ColorSchemeHash` is a checksum _of_ that derived block and is
stripped with it; leaving it would make KDE believe a stale block is current.
`Trolltech.conf` is the Qt-side twin and is excluded wholesale (§14.3).

**The guard.** This is implemented as a _conditional_ rule, not an unconditional
strip. Before applying it, `PlanExport` resolves the active scheme name from
`kdedefaults/kdeglobals` `[General] ColorScheme` (falling back to
`kdeglobals` `[General] ColorScheme`) and looks for a matching `.colors` file
under `<XDG data home>/color-schemes/` then `/usr/share/color-schemes/`. If no
matching file is found — the signature of a hand-rolled palette that exists only
inside `kdeglobals` — the `[Colors:*]`, `[Colors:*][*]`, `[WM]`,
`[ColorEffects:*]` group rules and the `ColorSchemeHash` key rule are **not**
applied, and a stderr advisory is printed:

```text
dotf: plasma: color scheme "MyScheme" has no .colors file; keeping [Colors:*] in kdeglobals
```

On the audited host `~/.local/share/color-schemes` does not exist and the scheme
is stock `BreezeDark`, so the strip applies and nothing is lost.

The `.colors` lookup takes the data-home path as a constructor-adjacent
parameter resolved by the CLI, keeping the engine free of environment lookups:
`NewEngine` derives it as `filepath.Join(filepath.Dir(configDir), "share")` when
`configDir` ends in `.config`, and the CLI passes `$XDG_DATA_HOME` through
`Options` only if it needs to override — since this is a read-only probe with a
system fallback, the derived path plus `/usr/share/color-schemes` is sufficient
and no extra Options field is added.

### 7.5 Judgement call 2 — strip `kwinrc` `[Desktops] Id_*` and `[Tiling][…][…]` as a unit

Keep only `[Desktops] Number` and `[Desktops] Rows`.

Every `[Tiling]` header is keyed by _two_ UUIDs: a desktop UUID and an **output**
UUID that is the DP-2 monitor's identity derived from its EDID. Even if the
`Id_N` values were preserved verbatim, the tiling layouts would still be
unreachable on any machine with a different monitor. Keeping the IDs buys
nothing while carrying ten UUIDs and inviting collisions with the UUIDs KWin
mints for itself on a fresh profile — KWin generates an `Id_N` for any index
lacking one, so a partial or merged set is strictly worse than none.

Corroborating evidence that this is accretion, not intent: 16 `[Tiling]` blocks
for 10 desktops (six orphans of deleted desktops), and 14 of the 16 hold the
identical default `0.25/0.5/0.25` layout.

The portable intent — `Number=10`, `Rows=1`, `[Windows]
PerOutputVirtualDesktops=true` — survives intact. Tile layouts are per-monitor
ergonomics that must be redrawn per machine. `plasma/README.md` documents this
so the loss is expected rather than surprising.

### 7.6 Governance

`DefaultFiles()` gets `symlink.DefaultTable()`'s treatment: one exported
function returning a static typed slice, carrying a `GOVERNED:` doc comment,
locked by `TestDefaultFiles_MatchesGolden` against an inline Go golden copy, plus
`TestDefaultFiles_Count`, `TestDefaultFiles_FormatCounts`,
`TestDefaultFiles_ApplyOrderPhases`, and `TestDefaultFiles_PathsUnique`. A stray
addition cannot land silently and start deleting real settings on the next
export.

**Amended (A8) — the golden test is necessary and not sufficient.** It pins
**which** files are managed; it pins nothing about **what** each glob may
delete, which is why all four §7.3.1 defects shipped past it. Governance
therefore has a second layer: `internal/plasma/table_glob_test.go` asserts, per
file, the specific keys each rule set must delete **and the specific keys it must
leave alone** — plus `TestStripTable_MustNotManage` (the paths that must stay
denylisted) and `TestStripTable_DoesNotStripRecommendedKeys` (the §12.13
invariant). A glob widened by one character now fails a named assertion rather
than a byte-for-byte golden nobody reads.

---

## 8. CLI surface

### 8.1 Commands

```text
dotf plasma status  [--only a,b] [--json]
dotf plasma diff    [--only a,b] [--json]
dotf plasma export  [--only a,b] [--dry-run] [--json]
dotf plasma apply   [--only a,b] [--dry-run] [--backup] [--link]
                    [--recommended] [--reload] [--yes] [--json]
```

The verb is a positional, parsed with the repo's order-independent loop-parse
idiom (`setup.go:57-68`), so `dotf plasma export --dry-run` and
`dotf plasma --dry-run export` are equivalent. Zero verbs, more than one verb, or
an unknown verb is a usage error (exit 2):

```text
dotf: plasma requires a verb; available: export, apply, diff, status
dotf: plasma takes exactly one verb, got 2
dotf: unknown verb "aply"; available: export, apply, diff, status
```

### 8.2 Flags

One flagset serves all four verbs, registered by the single-source-of-truth
`setPlasmaFlags`. Sorted flag names, as asserted by
`TestSetFlags_ExactFlagNames`: `backup`, `dry-run`, `json`, `link`, `only`,
`recommended`, `reload`, `show-volatile`, `unlink`, `yes`.

| Flag | Type | Usage string (verbatim, DRY with help and completions) |
| --- | --- | --- |
| `--only` | string | `comma-separated managed file paths to act on (default: all)` |
| `--dry-run` | bool | `print the plan without executing` |
| `--backup` | bool | `back up every live file the run modifies before writing` |
| `--link` | bool | `symlink each managed file into the repo instead of merging` |
| `--unlink` | bool | `restore copy mode: replace each symlinked managed file with a real file` |
| `--recommended` | bool | `layer the opt-in plasma/recommended overlay on top of plasma/files` |
| `--reload` | bool | `restart the affected KDE components after applying` |
| `--show-volatile` | bool | `report the live-only keys export strips by design (diff; suppressed by default)` |
| `--yes` | bool | `apply without an interactive confirmation prompt` |
| `--json` | bool | `emit machine-readable JSON to stdout instead of the default output` |

**Amended (A20, A21) — `--unlink` and `--show-volatile` are shipped flags.**
Both were absent from this table. `--unlink` is `--link`'s inverse and is
described in §9; `--show-volatile` surfaces the `DiffExpectedVolatile` keys the
engine suppresses by default and is described in §5.4. Being generated from
`setPlasmaFlags`, the help text and all three completion scripts carried them
from the moment they were registered — which is exactly what the single-source
rule buys.

**Amended (A11) — the `--json` row and its DRY seam.** The row previously quoted
`emit a JSON report instead of key=value lines`, a string that appears nowhere in
the tree; the verbatim text is `flags.go`'s shared `jsonFlagUsage` const, which
is what `dotf plasma --help` prints.

The seam is the **const, not the setter**. `setPlasmaFlags` does not call
`setJSONFlag`: that helper returns a `*bool` and so cannot bind into the
`plasmaOpts` struct every other plasma flag binds into. It calls
`fs.BoolVar(&o.jsonOut, "json", false, jsonFlagUsage)` directly. Sharing the
const is what the DRY rule is actually protecting — the user-visible string, kept
identical across `plan`, `status`, `diff`, `doctor`, and `plasma` — while
threading a `*bool` back into a struct field would only reintroduce the
out-of-band pointer that `plasmaOpts` exists to avoid.

There is no `--manifest` and no `--no-user-config`: plasma does not resolve the
manifest, exactly like `link` and `self-update`.

**Flag applicability.** Flags irrelevant to a verb are accepted and ignored
rather than rejected — a single shared flagset with per-verb rejection would
duplicate the flag surface in the validator and drift from `setPlasmaFlags`. The
exceptions are the three that would silently do nothing **dangerous**, checked by
`plasmaCheckApplyOnlyFlags`:

- `--link` on any verb other than `apply` → usage error, exit 2, with the
  message `dotf: --link applies only to "dotf plasma apply"`
- `--unlink` on any verb other than `apply` → usage error, exit 2, with the
  message `dotf: --unlink applies only to "dotf plasma apply"`
- `--reload` on any verb other than `apply` → usage error, exit 2, with the
  message `dotf: --reload applies only to "dotf plasma apply"`

**Amended (A20).** `--unlink` joins the gate for the same reason `--link` is in
it: ignoring it silently would leave the user believing a destructive mode change
had happened. `--show-volatile` is deliberately **not** in it — widening a report
on a verb that emits no key detail is inert and harmless, which is precisely the
class of flag the shared flagset exists to absorb.

`--only` values are validated against `DefaultFiles()` paths; an unknown value
is a usage error, exit 2: `dotf: unknown --only path "kwinrcx"`.

**Amended (A5) — `--link` and `--recommended` are mutually exclusive.**
`PlanApply` refuses the pair up front, before any per-file work, with
`*LinkRecommendedError`. The reasoning is in §9.2; the short form is that a
linked live path **is** `plasma/files/<path>`, so the overlay has nowhere to
land, while `diff` and `validate` **do** honour `--recommended` and therefore
reported every recommended key as permanent, unfixable drift on every subsequent
run.

**Exit-code note — amended (A12, corrected by A22).** The pair is a **usage
error, exit 2**, like the flag-applicability rules above it. `PlanApply` still
returns `*LinkRecommendedError` so the engine remains independently correct for a
library caller (and for the embedded `dotf apply --plasma` path, where it is a
phase failure), but the CLI rejects the combination first, before any engine
work. An incoherent flag pair is exactly what exit 2 means, and routing it
through the engine's exit-1 failure bucket made it indistinguishable from a file
that failed to apply.

A12 named the wrong function. The rejection lives in a **second** gate,
`cmd/dotf/plasma.go`'s `plasmaCheckExclusiveFlags`, called immediately after
`plasmaCheckApplyOnlyFlags`. The two are separate because they answer different
questions — "does this flag apply to this verb?" versus "may these two flags
appear together at all?" — and the second is verb-independent. It covers **two**
pairs, each printing the engine's own error text so the wording is
single-sourced:

- `--link` with `--recommended` → `dotf: plasma: --link cannot be combined with
  --recommended: …` (`*LinkRecommendedError`, §5.8, §9.2)
- `--link` with `--unlink` → `dotf: plasma: --link cannot be combined with
  --unlink: …` (`*LinkUnlinkError`, §5.8) — **amended (A20)**, a pair this
  document never mentioned

### 8.3 stdout — `key=value`

All lines are assembled in memory and emitted with **one checked write**
(`io.WriteString(stdout, strings.Join(lines, "\n")+"\n")`), matching
`printLinkDryRun`. A printer with nothing to say writes nothing.

**Token encoding.** File, group, and key names may legally contain `,`, `=`, and
control bytes. Any token embedded in a `plasma.*` value is percent-encoded:
`%` → `%25`, `,` → `%2C`, `=` → `%3D`, and any byte `< 0x20` → `%XX` (uppercase
hex). Nothing else is encoded, so the common case is unchanged and the contract
stays machine-parseable.

**Common lines.**

| Line | Emitted by | Meaning |
| --- | --- | --- |
| `dry-run=true` | any `--dry-run` | always the first line |
| `plasma.verb=<verb>` | all verbs | which verb ran |
| `plasma.session=live\|absent\|unknown` | all verbs | KDE session detection |

**File-bucket lines** — `plasma.<bucket>=<path>,<path>` with buckets emitted in
the fixed order `export`, `prune`, `create`, `merge`, `copy`, `link`, `unlink`,
`backup`, `skip`, `unknown` (**amended (A20)** — `unlink` sits between `link` and
`backup` in `plasmaBucketOrder`, mirroring `ActionKind`'s own ordering); empty
buckets omitted; paths `sort.Strings`-sorted. This is
`linkActionLines`'s shape with prefix `plasma`, except that the bucket is read
directly from `FileAction.Kind` — there is no `Reason`-string switch, because
`Reason` is human-readable prose only.

**Amended (A16) — a mutating verb renders the PLAN AS EXECUTED, not
`Report.Applied`.** `export` and `apply` build their buckets from
`plasmaExecutedActions(rep)`: the whole of `Report.Planned`, with each entry
replaced by its executed form wherever `Execute` produced one. (`Applied` is an
in-order subsequence of `Planned` and `Execute` never changes an applied action's
`Kind`, so a two-pointer walk pairs the two slices exactly, without inventing a
key.)

The normative rule this implements: **the same verb must not report a different
file set depending on `--dry-run`.** Rendering `Report.Applied` broke it in both
directions. `Applied` excludes every `ActionSkip`, and a refused write **is** a
skip, so the one file most worth reporting — an immutability refusal — vanished
from stdout and from the `--json` `files` array on the real run while the same
verb's `--dry-run` rendered it. Two consequences follow, and both are deliberate:

- A converged real `apply` or `export` now prints a **full `plasma.skip=`
  bucket**, exactly as its own `--dry-run` does, rather than printing nothing at
  all. A run with nothing to do says so by name.
- `summary.applied` is untouched by this and still counts **only what landed**,
  so `applied=0` remains the honest signal of convergence (§6.5). The bucket
  lines say what the run's file set was; the summary says what happened to it.

`plasma.change=` is the one line that stays on `Report.Applied`: it names the
keys that were actually **written**, and a planned-but-refused merge carries
plan-time `Keys` that never landed.

**Key-detail lines.**

| Line | Verb | Meaning |
| --- | --- | --- |
| `plasma.change=<file>:<group>/<key>,…` | `apply` | every key the run writes, sorted; comma-joined |
| `plasma.drift=<file>:<group>/<key> kind=<diff-kind>` | `diff` | one line **per difference** apply would act on, sorted by rendered entry |
| `plasma.only-live=<file>:<group>/<key> kind=missing-repo` | `diff` | one line **per live-only key** nothing in the strip table explains — genuinely unmanaged state. Informational, never a failure |
| `plasma.expected-volatile=<file>:<group>/<key> kind=expected-volatile` | `diff --show-volatile` | one line **per live-only key this file's own strip rules remove on export**. Absent from the default output, where the engine drops these entries entirely |

`<group>` is the rendered header text (`[Desktops]`, `[Tiling][x][y]`, or
`[<default>]` for the root group); a locale variant renders as
`<key>[<locale>]`.

**Amended (A7) — the two `diff` lines are one entry per line, not
comma-joined.** A real desktop produced a single `plasma.only-live=` line over
8 KB long: no terminal could display it and no `grep` could narrow it. Repeating
the key keeps every line independently greppable — `grep '^plasma.drift=' | wc -l`
is the drift count — while staying inside the same `key=value` contract as
`plasma.reload.<n>=`, which is likewise one line per item.

**Amended (A21) — the three-bucket split, and why it is a bucket rather than an
attribute.** `plasmaDiffLines` switches on `KeyDiff.Kind`: `DiffMissingRepo` goes
to `plasma.only-live=`, `DiffExpectedVolatile` to `plasma.expected-volatile=`,
and everything else to `plasma.drift=`. Bucketing on the engine's own `DiffKind`
— never on a count or a name pattern — keeps the classification the engine's
rather than the renderer's, and keeps each bucket independently greppable, which
is the whole point of the one-line-per-entry shape. A real desktop carries
hundreds of `[Tiling][<uuid>]` and `Session*` entries that are volatile by
construction; sharing one bucket with the genuinely unmanaged keys made a single
interesting key unfindable even with the `kind=` attribute present to
distinguish them.

**Extension rule.** Each line reads
`<key>=<file>:<group>/<key> <name>=<value>…`: the reference first, so lines sort
by identity, then a space-separated run of attributes describing _that_ entry.
Further per-entry facts append as more `name=value` pairs on the same line, in
one place (`plasmaKeyAttrs`), touching neither the line assembly, the sort, nor
the bucket split. **Amended (A21)** — the attribute this rule previously named as
"next due", the expected-vs-unmanaged classification of a live-only key, needed
no widening in the end: the engine expresses it as a `DiffKind`, so the existing
`kind=` attribute carries it and the bucket split reads the same field.
`plasmaKeyAttrs` therefore still emits exactly one pair, `kind=`.

`plasma.change=` is deliberately left comma-joined for now. It is bounded by
what a single run writes rather than by what a desktop accumulates, so it has
not hit the same wall; if it does, it takes the identical treatment.

**Status lines.**

```text
plasma.managed=27
plasma.repo-present=21
plasma.live-present=24
plasma.missing-live=konsolerc,kwalletrc
plasma.missing-repo=klipperrc
plasma.absent=plasmarc
plasma.drift.files=2
plasma.drift.keys=7
```

**Amended (A14).** `plasma.absent=` is the `StatusReport.Absent` set of §5.4 — managed files present
on **neither** side. It is emitted for the same reason the JSON row carries
`action: "absent"`: leaving it out of the text block while the JSON called those
files "present" made the two renderers disagree about six of the twenty-seven
managed files on a fresh host. Like every other bucket, it is omitted when empty.

**Link-conflict lines — amended (A20), previously undocumented.** `apply
--dry-run` emits two further lines, and only when `Plan.LinkConflicts` is
non-empty (§9.3) — that is, when `--dry-run` **alone** authorised a mode
transition that the same invocation without it would refuse:

| Line | Emitted by | Meaning |
| --- | --- | --- |
| `plasma.link-conflict=<path>,<path>` | `apply --dry-run` | the files whose transition this preview planned but a real run without `--backup` would refuse |
| `plasma.link-conflict.refusal=<message>` | `apply --dry-run` | the exact `*LinkModeConflictError` sentence that real run would print, percent-encoded as one token |

Both are built by `plasmaLinkConflictLines` from the engine's own error value, so
a preview cannot promise a different refusal from the one that will actually
happen. They go to **stdout**, not to the advisory stream, because a dry run's
stderr is asserted empty by contract: the preview channel is where a preview's
bad news belongs. Printing the plan and staying silent about them made
"`--dry-run` to preview it" advice that previewed the wrong run.

**Reload lines.** `plasma.reload.<n>=<argv joined by single spaces>`, `n`
starting at 1, in execution order. These are emitted by `apply` **whether or not
`--reload` was given** — without it they are the commands the run declined to
execute (§10.2).

### 8.4 stderr

Always `progName`-prefixed. **Amended (A11, extended by A16) — the advisory set
is five, and it is conditionally suppressed.**

- Errors: `dotf: <err>`
- Summary line at the end of a mutating run:
  `dotf: plasma summary: applied=%d planned=%d`
- Per-file non-fatal failure lines after the summary: `dotf: plasma: <path>: <err>`
- Advisories (non-fatal, printed before the summary):
  1. the colour-scheme guard advisory of §7.4 — every `Plan.Advisories` entry, first
  2. `dotf: plasma: no KDE session detected; applying to files only`
  3. `apply` only —
     `dotf: plasma: <path> is rewritten by the running session; changes may be overwritten before you log out`
  4. `export` only —
     `dotf: plasma: <path> is rewritten by the running session; the captured snapshot may miss settings it has not written to disk yet`
  5. `dotf: plasma: --reload requested but no KDE session detected; skipping`

**Amended (A16) — the `SessionOwned` advisory is direction-specific.** Advisories
3 and 4 are one advisory phrased for the verb's write **direction**, because the
two directions have opposite hazards and only the `apply` one was ever
described. `apply` writes the live file, which the session then rewrites over —
that is the "may be overwritten" warning. `export` writes into `plasma/files/`,
which no KDE process opens, so nothing can overwrite what export just wrote;
what `export` actually risks is the mirror image, that the session is still
holding settings in memory and the captured snapshot predates their sync.
Sharing the `apply` wording with `export` warned about a process that cannot
touch the file in question.

**Amended (A16) — it is emitted once per FILE, not once per planned action.**
`plan.Files` carries a file's `ActionBackup` entry alongside its mutating one
(§6.9), so keying the loop on the slice printed the same sentence twice for the
same path under `--backup`, and it read as two independent problems. The loop
keeps a per-path seen-set; `ActionSkip` entries never advise at all, because a
skipped file is one the run does not write.

Three suppression and ordering rules govern which of those can appear:

- **`advise()` runs on `export` and `apply` only.** `diff` and `status` never
  advise; both are read-only, and neither the session-ownership warning nor the
  no-session warning describes anything they do.
- **`advise()` returns immediately under `--dry-run`.** A preview encounters
  none of the conditions the advisories warn about, so printing them would be
  noise — and this is the mechanism behind the `stderr == ""` assertions on the
  dry-run paths.
- **Advisory 2 short-circuits advisories 3 and 4.** Without a live session there
  is no session to overwrite anything or to be holding unflushed settings, so
  `advise()` returns after printing it and the per-file `SessionOwned` lines
  never follow.

Advisory 5 is emitted by `reload()`, not `advise()`, so it is subject to none of
the above: it fires on the real `apply` path, after `Execute` and before the
summary, whenever `--reload` was asked for on a host with no live session. It is
an advisory rather than a failure because the settings still landed and are
picked up at next login.

Consequently stderr is empty on success only for `diff`, `status`, and every
`--dry-run`. A real `export` or `apply` always writes at least the summary line.

### 8.5 `--json`

`--json` **replaces** the `key=value` stdout; it does not augment it. The exit
code is unchanged. Emitted through the single existing encoder
`writeJSON(w, v)` (`report.go:201`).

```go
type plasmaReport struct {
    Command string             `json:"command"` // always "plasma"
    Verb    string             `json:"verb"`
    DryRun  bool               `json:"dryRun,omitempty"`
    Session string             `json:"session"`
    Files   []plasmaFileReport `json:"files"`
    Reloads []string           `json:"reloads,omitempty"`
    // LinkConflicts names the files whose mode transition this PREVIEW planned
    // but a real run without --backup would refuse. Empty on every other path.
    LinkConflicts []string      `json:"linkConflicts,omitempty"`
    Summary       plasmaSummary `json:"summary"`
    // Failures carries one diagnostic per NON-FATAL failure, in the order and
    // with the text printPlasmaFailures writes to stderr. len == summary.failed.
    Failures []string `json:"failures,omitempty"`
    // Error is the diagnostic on a failure path, and the ONE field whose
    // presence means the run did not complete.
    Error string `json:"error,omitempty"`
}

type plasmaFileReport struct {
    Path   string            `json:"path"`
    Action string            `json:"action"`
    Reason string            `json:"reason,omitempty"`
    Keys   []plasmaKeyReport `json:"keys,omitempty"`
    Error  string            `json:"error,omitempty"`
}

type plasmaKeyReport struct {
    Group    string `json:"group"`
    Key      string `json:"key"`
    Locale   string `json:"locale,omitempty"`
    Kind     string `json:"kind,omitempty"` // diff verb only
    Repo     string `json:"repo,omitempty"` // base64 when not valid UTF-8
    Live     string `json:"live,omitempty"`
    Encoding string `json:"encoding,omitempty"` // "base64", or absent
}

type plasmaSummary struct {
    Managed    int `json:"managed"`
    Planned    int `json:"planned"`
    Applied    int `json:"applied"`
    DriftFiles int `json:"driftFiles"`
    DriftKeys  int `json:"driftKeys"`
    // VolatileKeys counts the expected-volatile rows separately from DriftKeys.
    // No omitempty: it is present in EVERY document, 0 on the default path.
    VolatileKeys int `json:"volatileKeys"`
    Failed       int `json:"failed"`
}
```

**Amended (A20, A21, A23) — two fields this listing omitted.**

- `linkConflicts` is `Plan.LinkConflicts` (§5.3, §9.3), set by `applyPreview` and
  therefore present only on `apply --dry-run`, and only when the preview planned
  a transition a real run would refuse. It is the JSON twin of the
  `plasma.link-conflict=` stdout line (§8.3), so a `--json` consumer is not the
  one channel left guessing.
- `summary.volatileKeys` is the third value `plasmaDriftTally` returns (§5.4).
  It is filled on `diff` only: `apply` discards it (its post-write `Validate`
  reports drift, and volatility is not drift), `export` and `status` never
  compute it. Unlike every other optional field it carries **no** `omitempty`,
  so the key is present in every document and `0` reliably means "none" rather
  than "this verb does not answer that question" — the same shape the other
  counters already had.

**Amended (A17) — `failures` is the non-fatal diagnostic set; `error` stays the
fatal one.** The two are not alternatives and never both describe the same
problem:

- `error` is present only when the run **did not complete**. It is the failure
  form of the document — the same envelope every verb emits, with `files` empty
  and the diagnostic here — which is what guarantees a `--json` run never answers
  with zero bytes plus a non-zero exit, a shape no consumer could parse.
- `failures` is present when the run **completed with non-fatal failures**: a
  per-file refusal (an immutable group, an unwritable destination, a refused
  backup), or a reload command that ran and failed. Each entry is one
  diagnostic, in the same order and with the same text `printPlasmaFailures`
  writes to stderr, so the two channels can never describe a different failure
  set.

`failures` exists because neither of the other fields could carry that class.
`error` stays empty — the run completed — and the `files` array cannot cover it
either, since a failed reload has no file to attach to. Without it a `--json`
consumer saw a non-zero `summary.failed` with no way to learn what failed or
why, because both sentences went only to stderr.

**Amended (A17) — `summary.failed` counts failures only, and always equals
`len(failures)`.** Apply's post-write `Validate` returns a `*DriftError`
alongside its full results (§5.8), and that error is **reported, never counted**:
post-apply drift is normally the downstream consequence of a per-file failure
already in `failures`, so folding it in reported one problem as two. Drift has
its own two counters, and it still forces exit 1 — see §8.6.

**Amended (A17, corrected by A24) — `driftFiles` and `driftKeys` mean the same
thing on every verb.** On `diff` and on apply's post-write `Validate` — which is
that same comparison (§6.1) — both are filled by one helper, `plasmaDriftTally`,
over the `[]FileDiff` the comparison returned. `status` is the exception A17
overstated: it fills them from the engine's own counters
(`len(st.DriftedFiles)`, `st.DriftedKeys`), because `Status` has already done
that tally and returning a `[]FileDiff` for the CLI to re-count would be a second
answer to a question the engine answered. The numbers agree — `Status` resolves
to `Diff`, and `StatusReport.DriftedKeys` counts by the same
`DiffKind.Actionable()` rule — but the claim "one helper, on three verbs" was
literally false and is withdrawn. `apply` previously filled
`driftFiles` from the `DriftError`'s path list and left `driftKeys` pinned at
`0`, so a consumer reading `driftKeys` to decide whether post-apply validation
had found anything concluded "no keys drifted" after every single apply.

Slices always encode as arrays, never `null`. Every optional field is
`omitempty` so the default shape stays stable for consumers. A value that is not
valid UTF-8 is base64-encoded and the containing `plasmaKeyReport` gains
`"encoding":"base64"`. **Amended (A11)** — the struct listing above previously
omitted the `Encoding` field this sentence depends on, so the declared type did
not match the documented JSON shape. `Encoding` is a per-**row** flag, not a
per-side one: it is set to `"base64"` when **either** `Repo` or `Live` needed
encoding, since a KConfig key that holds a byte string on one side is almost
always one on both, and a second field would buy a distinction no consumer of
this report makes.

### 8.6 Exit codes

Deliberately confined to `0/1/2` plus `130`. Exit `3` is reserved for `dotf
apply`'s hard aborts and plasma never uses it.

| Verb | 0 | 1 | 2 | 130 |
| --- | --- | --- | --- | --- |
| `status` | always, when it can read | repo root / config dir unresolvable, parse error | bad flag, unknown `--only`, `--link`/`--unlink`/`--reload` misuse | — |
| `diff` | **no drift** | **drift**, or any error | bad flag, unknown `--only`, `--link`/`--unlink`/`--reload` misuse | — |
| `export` | wrote (or correctly skipped) everything | any file failed; privacy leak; no live files at all | bad flag, unknown `--only`, `--link`/`--unlink`/`--reload` misuse | — |
| `apply` | converged or applied cleanly | any file failed; post-apply `Validate` still drifted; declined confirmation | bad flag, unknown `--only`, `--link` with `--recommended` (A12), `--link` with `--unlink` (A20) | Ctrl+C at the confirm prompt |
| any `--dry-run` | 0 on drift and on any plan it can compute | a plan-time failure: `MissingRepoTreeError`, `NoLiveFilesError`, `PrivacyLeakError` | bad flag, unknown `--only`, `--link`/`--unlink`/`--reload` misuse | — (a dry run never prompts, so 130 is unreachable) |

The verb-applicability rules cannot fire on `apply` — the three flags they gate
are the `apply`-only ones — which is why that row lists the exclusive **pairs**
instead.

**Amended (A5, revised by A12, corrected by A22 and extended by A20).** Both
mutually exclusive pairs are usage errors, exit **2**: `--link` with
`--recommended`, and `--link` with `--unlink`. `plasmaCheckExclusiveFlags` — not
`plasmaCheckApplyOnlyFlags`, which A12 misnamed — rejects them before the engine
is reached, printing the engine's own message (`dotf: plasma: --link cannot be
combined with --recommended: …` / `… with --unlink: …`). `PlanApply` keeps both
guards so a library caller cannot bypass the rules and so the embedded
`dotf apply --plasma` path still fails cleanly, but on the `dotf plasma` CLI path
those guards are unreachable.

**Amended (A17) — `apply` exits 1 when EITHER failures or drift is present, and
they are counted separately.** The rule is `len(failures) > 0 || driftErr != nil`.
Post-apply drift is a **report**, not a second failure: it is carried by
`summary.driftFiles`/`summary.driftKeys` and printed on stderr as its own line,
while `summary.failed` counts only the run's non-fatal failures and always equals
`len(failures)` (§8.5). So a converged-but-refused apply exits 1 with
`failed=1`, and an apply whose writes all landed but whose post-write `Validate`
still finds drift exits 1 with `failed=0` and non-zero drift counters. A `--json`
consumer branching on `summary.failed` alone would miss the second case; the
exit code covers both.

The `--dry-run` row governs the two **mutating** verbs, `export` and `apply`.
On `diff` and `status` — which never mutate — `--dry-run` is accepted and inert:
it prints the `dry-run=true` marker line and leaves the exit code entirely to
that verb's own rule, so `dotf plasma diff --dry-run` still exits 1 on drift.

`diff` exiting 1 on drift is a **deliberate divergence** from `link`'s
"dry-run always exits 0" rule. `link --dry-run` is a _preview_ and surfaces
problems as a `link.broken=` stdout line; `dotf plasma diff` is a _drift
detector_ whose whole job is signalling non-convergence to CI, exactly like
`dotf diff`. `dotf plasma apply --dry-run` is a preview and follows the
link rule for its _content_: drift is visible on stdout and never sets the exit
code.

**Amended (A26).** The earlier "always 0" wording was wrong, and wrong about a
path a user meets on their first run. Planning happens _before_ the `--dry-run`
branch — `plasmaRun.export()`/`apply()` call `PlanExport`/`PlanApply` and route
any error through `p.fail` — so a dry run exits 1 on a **plan-time failure**
even though it exits 0 on drift. The common cases are the fresh-machine ones:
`apply --dry-run` with no `plasma/files/` yet exits 1 with `MissingRepoTreeError`,
`export --dry-run` with no managed file present exits 1 with `NoLiveFilesError`,
and a `PrivacyLeakError` (§14.1, raised at plan time) does the same. What a dry
run guarantees is that it **mutates nothing**, not that it always succeeds.

Host gating:

- Not Linux (`Support(info) == SupportUnsupported`) → exit 1 with
  `dotf: plasma: unsupported on darwin (Linux only)`. Not a usage error.
- Linux, no KDE session → advisory on stderr, run proceeds. `export` will then
  typically fail with `NoLiveFilesError` (exit 1); `apply` succeeds, which is the
  fresh-machine case that motivates the warn-don't-fail rule.

### 8.7 Confirmation and interrupts

`dotf plasma apply` mutates live settings, so it confirms before writing:

```text
apply KDE Plasma configuration? [y/N]
```

- The prompt is written to **stdout** — following `sysconfig.go:241`, because on
  that path stdout _is_ the terminal.
- `--dry-run` never prompts. `export`, `diff`, `status` never prompt.
- `--yes` skips the prompt.
- Non-interactive (a non-`*os.File` stdout, or a non-TTY) without `--yes`:
  `dotf: refusing to run plasma apply without --yes (non-interactive)`, exit 1
  (the `setup.go:105-108` shape).
- Answering "No" is a decline, not an abort: `dotf: plasma not confirmed`,
  exit 1.
- Ctrl+C at the prompt: `dotf: aborted`, exit 130 via `reportAbort(stderr)`.

Interrupt handling is scoped to the prompt only, via `signal.NotifyContext`
stopped by `defer` the moment the read returns. The post-confirmation execution
phases keep Go's default SIGINT semantics.

**Amended (A11) — what is shared with `sysconfig`, and what is deliberately
not.** The original text claimed plasma "reuses `sysconfig`'s
`confirmApply`/`confirmApplyCtx` … verbatim". It does not, and must not:

- **Shared:** the `confirmDecision` enum (`confirmYes` / `confirmNo` /
  `confirmAborted`), so both commands map the same three outcomes onto the same
  three exit paths, and `reportAbort` for the 130 case.
- **Plasma's own:** `confirmPlasmaApply` / `confirmPlasmaApplyCtx` in
  `cmd/dotf/plasma.go`, and the `plasmaConfirmPrompt` const.

The divergence is intentional. `confirmApplyCtx` **bakes sysconfig's prompt text
into its body** — it is not parameterised on the prompt — so calling it would
either ask the user about system configuration before writing KDE settings, or
force a signature change to `sysconfig`'s command and its tests for the sake of
one string that this command owns. Duplicating ~25 lines of select-on-a-read
goroutine is the cheaper of the two, and the shared enum is what keeps the two
copies from diverging in _behaviour_. The doc comment on `confirmPlasmaApply`
states this rule at the call site so the next reader does not "unify" them.

### 8.8 Registration checklist

Every item is mandatory; the first four are enforced by existing tests that fail
the build without them.

1. `cmd/dotf/main.go` — `case "plasma": return runPlasma(rest, os.Stdout, os.Stderr, exec.New())`, placed after `case "setup"` and before `case "completion"`.
2. `cmd/dotf/helptext.go` — a `commandHelp` entry named `"plasma"`, inserted between `"setup"` and `"completion"` so registry order matches switch order. `setFlags: func(fs *flag.FlagSet) { setPlasmaFlags(fs) }`.
3. `cmd/dotf/help_test.go` — add `"plasma"` to `dispatchableCommands` (and update its doc comment from "fourteen" to "fifteen") and to `allRegistryNames` in registry position; add the `"plasma"` case to `TestSetFlags_ExactFlagNames`.
4. The completion generator — **amended (A10), see §8.8.1**. The generator is
   now four files: `cmd/dotf/completiongen.go` (the model plus the **positional
   model**) and one renderer each in `completiongen_bash.go`,
   `completiongen_zsh.go`, `completiongen_fish.go`. Any spec or comment pointing
   at `completiongen.go` for a shell renderer is stale.
5. `cmd/dotf/completion_test.go` — `TestGeneratedScripts_PlasmaPositional` plus
   the three behavioural tests of §8.8.1.
6. Prose, hand-maintained, no test enforces it — see §14.5 for the full list of
   repo doc surfaces a new subcommand must update. At minimum: `README.md` (a
   `### dotf plasma` section next to `### dotf link`, the feature bullet list,
   and the repository tree), `docs/README.md`, `AGENTS.md`'s subcommand
   enumeration, and **both** of `CLAUDE.md`'s Stack Overview surfaces (the prose
   enumeration and the table row) — whose list is already stale (missing
   `doctor`/`lock`/`self-update`/`setup`) and should be corrected in the same
   change.

#### 8.8.1 The completion-generator contract (amended, A10)

Item 4 as originally written is the **spec-level source** of two shipped
completion defects. It instructed only "extending the existing
`completion`/`help`/`setup` rules' `and not __fish_seen_subcommand_from …`
exclusion lists to also exclude `plasma`" — i.e. it required guarding the
**positional** rules and was silent on the **per-command flag** rules, which is
exactly the hole that let `dotf plasma apply --<TAB>` offer the top-level `apply`
command's flags. It also under-specified bash's scan.

The contract is now:

- **Every generated fish rule — flag rules included, not just positional rules —
  must be guarded against the commands that can put its name on the line as a
  positional value.** `__fish_seen_subcommand_from` matches a token _anywhere_
  on the line, so `dotf plasma apply` satisfies both `plasma`'s rules and the
  top-level `apply` command's.
- **The guards are derived from one positional model, never hand-listed.**
  `positionalCommands` is the single answer to "which words can appear on a
  `dotf` line without being the subcommand", and `positionalOwnersOf(name)`
  turns it into the exclusion list for `name`. A fifth plasma verb, or a new
  setup module, that collides with a command name is then disambiguated the
  moment it is added, with no second enumeration to keep in sync.
- **bash's positional scan must be flag-value aware.** The first non-flag word
  after the subcommand is only the positional once a **value** flag's argument
  has been skipped; a `--flag=value` word carries its own value and needs no
  skip. Without this, `dotf plasma --only kwinrc <TAB>` read `kwinrc` as the
  verb and lost the plasma arm entirely. The per-command arms additionally
  complete a value flag's argument via `$prev` **first**.
- The generated scripts are **behaviourally executed** in test — real `fish` and
  real `bash` — not merely syntax-checked, by
  `TestGeneratedFish_FlagRulesDoNotCrossFire`,
  `TestGeneratedFish_PlasmaVerbFlagsInRealFish`, and
  `TestGeneratedBash_PositionalScanSkipsFlagValues`.

`cmd/dotf/help.go` needs **no** change: `lookupHelp`, `isHelpToken`,
`wantsHelp`, `runHelp`, `printGlobalHelp`, `printCommandHelp`, `writeCommands`,
`writeUsageError`, and `renderFlags` are all registry-driven. `embed.go`,
`.goreleaser.yml` need no change — `plasma/` is repo data read from
`resolveRepoRoot()` at runtime, like `bin/*.sh`. `.github/workflows/go.yml`
gains `plasma/**` in its path filters only if a test reads repo fixtures; the
committed fixtures live in `internal/plasma/testdata/`, which `**.go` filters
already miss, so add `internal/plasma/testdata/**` to the trigger paths.

### 8.9 `runPlasma` shape

```go
func runPlasma(args []string, stdout, stderr io.Writer, r exec.Runner) int
```

Body order, copied from `link.go:31-85` and `setup.go:46-130`:

1. `fs := flag.NewFlagSet("plasma", flag.ContinueOnError); fs.SetOutput(stderr)`
2. `o := setPlasmaFlags(fs)`
3. loop-parse for the single positional verb; arity and membership checks → exit 2
4. `plasmaCheckApplyOnlyFlags` — `--link`/`--unlink`/`--reload` verb applicability → exit 2
5. `plasmaCheckExclusiveFlags` — `--link` with `--recommended`, `--link` with `--unlink` → exit 2 (A12, A20, A22)
6. `session := plasma.DetectSession(os.Getenv)` — **first** of the resolution steps, deliberately (see below)
7. `resolveRepoRoot()`; empty → `plasmaFail(…, errors.New("could not locate the dotfiles repo root (set DOTFILES_ROOT)"))`
8. `os.UserHomeDir()`; error → `plasmaFail(…, fmt.Errorf("resolve home dir: %w", err))`
9. resolve `configDir` from `$XDG_CONFIG_HOME`, else `filepath.Join(home, ".config")`
10. `info := platform.Detect(r)`; `Support(info)` gate → `plasmaFail(…, &plasma.UnsupportedOSError{…})`, exit 1 on unsupported
11. filter `plasma.DefaultFiles()` by `--only`; unknown path → exit 2
12. build the `plasmaRun` context — `stdout`, `stderr`, `stdin`, `eng := plasma.NewEngine(repoRoot, configDir, r)`, the selected `files`, `o`, `verb`, and the `plasma.Options` built inline from `o` plus the session
13. dispatch to `run.export()` / `run.apply()` / `run.diff()` / `run.status()`
14. printers → `key=value` (or `writeJSON`) on stdout, summary on stderr

**Amended (A22, A23) — the flag gate is two calls, and the session is resolved
first.** The original listing folded both flag checks into one step and placed
`DetectSession` at step 9, after every pre-flight resolution. The code resolves
it before all of them, and the ordering is load-bearing: the three pre-flight
failures (steps 7, 8 and 10) all exit through `plasmaFail`, which emits the same
`--json` envelope a successful run would, and that envelope carries
`session`. Resolving the session late would have made those three documents the
only ones missing the field — and a machine consumer that gets zero bytes on
stdout plus a non-zero exit has to guess. `DetectSession` reads only the
environment and cannot fail, so nothing is risked by hoisting it.

**Amended (A11) — steps 12 and 13.** The original listing named four free
functions, `runPlasmaExport` / `runPlasmaApply` / `runPlasmaDiff` /
`runPlasmaStatus`. They do not exist. The four verbs are **methods on a
`plasmaRun` struct** that bundles one invocation's resolved inputs, so they share
a single typed context instead of repeating a six-parameter signature four times;
`header`, `report`, `emit`, `advise`, `confirm`, and `reload` hang off the same
receiver.

```go
type plasmaRun struct {
    stdout, stderr io.Writer
    // stdin is injected rather than read from os.Stdin at the point of use so
    // the interactive confirmation — the only exit-130 path in this command —
    // is drivable from a test without a pseudo-terminal.
    stdin io.Reader
    eng   *plasma.Engine
    files []plasma.FileSpec
    o     *plasmaOpts
    popts plasma.Options
    verb  string
}
```

```go
type plasmaOpts struct {
    only         string
    dryRun       bool
    backup       bool
    link         bool
    unlink       bool
    recommended  bool
    reload       bool
    showVolatile bool
    yes          bool
    jsonOut      bool
}

func setPlasmaFlags(fs *flag.FlagSet) *plasmaOpts
```

**Amended (A23) — `plasmaRun.stdin`, and two more `plasmaOpts` fields.**
`stdin` is not decoration: `runPlasma` assigns `os.Stdin` once and `confirm()`
reads through the field, which is what lets `plasma_signal_test.go` drive the
confirm prompt — including the Ctrl+C/exit-130 path of §8.7 — with an ordinary
pipe instead of a pseudo-terminal. Reading `os.Stdin` at the point of use would
have made that path testable only by process-level tricks, and an untested
130 path is how the abort discipline regresses silently. `unlink` and
`showVolatile` are the two flags of A20/A21.

**Amended (A11, corrected by A23) — the split shipped, and it is four files, not
three.** This was written as a contingency ("if `plasma.go` approaches the
~500-line soft cap, split the printers out"). It did approach it, and the
`--json` document types then pushed `plasmaprint.go` towards the cap in turn, so
they moved again. The command is now four files under the cap, each with a stated
boundary — and the boundary each file's own header comment states:

| File | Holds |
| --- | --- |
| `cmd/dotf/plasma.go` | flag surface (`plasmaOpts`, `setPlasmaFlags`), verb parse, the two flag gates and the `--only` filter, host gating, `plasmaRun` construction, and the interactive confirm |
| `cmd/dotf/plasmaverbs.go` | the `plasmaRun` type and the four verb methods, plus the shared `header` / `report` / `emit` / `advise` / `reload` helpers, `applyPreview`, and the reload/executed-action reconcilers |
| `cmd/dotf/plasmaprint.go` | `key=value` rendering only — token percent-encoding, the line builders (including `plasmaLinkConflictLines`), and the stderr summary/failure printers |
| `cmd/dotf/plasmajson.go` | the `--json` document types (`plasmaReport`, `plasmaFileReport`, `plasmaKeyReport`, `plasmaSummary`), the converters that fill them, `plasmaDriftTally`, and `plasmaFail` — the single failure exit that guarantees a `--json` run never answers with zero bytes |

Any spec text, comment, or review note that expects the printers to live in
`plasma.go`, expects a two- or three-file split, or expects the `--json` types in
`plasmaprint.go`, is stale.

---

## 9. `--link` / `--unlink` modes and the `--backup` interaction

### 9.1 What `--link` does, and how `--unlink` undoes it

`dotf plasma apply --link` plans `ActionSymlink` instead of `ActionMerge` for
every selected file: the live path is removed and replaced by a symlink to
`<repoRoot>/plasma/files/<path>`. Because KConfig writes through symlinks
(§2.3), KDE then reads _and writes_ the repo file directly.

It is a real capability with a real cost: any settings change KDE makes lands in
the git worktree, including the volatile state the strip table exists to keep
out. It is per-run and never persisted; there is no per-file `link: true` field
in `DefaultFiles()`, because a persistent per-file mode would make the repo's
state depend on how it was last applied.

`--link` is only accepted on `apply` (§8.2).

**Amended (A20) — `--unlink` is the way back, and without it `--link` was a
one-way door.** `dotf plasma apply --unlink` plans `ActionUnlink`: the live path,
currently a symlink, is replaced by a **real file** holding the merged content,
restoring copy mode. `writeAtomic` renames over the directory **entry**, so the
link is replaced rather than followed into the repo; the repo file is untouched.

The defect it fixes is structural, not cosmetic. Once a managed file was a
symlink into `plasma/files/`, every read of the live path read the repo's own
bytes — so the comparison was the repo against itself, the merge always saw a
converged file, and the symlink survived every subsequent `dotf plasma apply`
forever. There was no flag, and no sequence of flags, that could take a linked
tree back to copies. Two things follow, and both are deliberate:

- **`ActionUnlink` suppresses every convergence short-circuit on its path.** At
  plan time both checks are skipped — `Converged(source, liveDoc)` for an INI
  member and `bytes.Equal(live, repoData)` for a `FormatRaw` one — and at write
  time `writeRawLive`'s under-lock recheck is skipped too. For a linked path all
  three are trivially true, because the read followed the symlink into the repo.
  (The INI write path needs no exemption: `writeMerged` has no convergence
  short-circuit at all, it always re-merges and writes.) The point of the write
  is the destination's **type**, not its content (§5.3).
- **It is an explicit opt-in; a plain apply never unlinks.** A linked file is
  converged by construction, so a plain apply genuinely has nothing to do with
  it — and silently materialising copies would mean every headless
  `dotf apply --plasma` tore down a deliberate `--link` deployment. Requiring the
  flag is also what keeps the guard of §9.2 narrow: with no `--unlink` there is
  no transition to refuse.

`--unlink` is likewise only accepted on `apply`, and is refused in combination
with `--link` (§8.2, §5.8). It needs no `--recommended` rule of its own: an
unlink lands a real file, which is exactly where the overlay can land.

### 9.2 Mode is per file, and the two modes are mutually exclusive

**Amended (A5) — `--link` refuses `--recommended` before anything else.**
`PlanApply`'s first act, ahead of the per-file loop and ahead of the `os.Lstat`
matrix below, is to refuse the pair with `*LinkRecommendedError`. It is refused
run-wide rather than per file because the combination is incoherent for every
file at once, and a partial run would be worse than none.

The reasoning, so this is not re-litigated as an over-strict guard: a `--link`ed
live path **is** `plasma/files/<path>`. There is nowhere for
`plasma/recommended/` to land short of writing the overlay into the exported
snapshot — which the next `dotf plasma export` would strip straight back out.
The original design silently dropped the overlay in that case, and that was the
worst of the three options, because `diff` and `validate` **do** honour
`--recommended`: every recommended key was then reported as drift by the very
validation the same command runs, on every subsequent run, with no way to
converge. Refusing up front is the only honest answer.

**Amended (A20) — `--link` refuses `--unlink` in the same breath.** Immediately
after the `--recommended` check and for the same reason, `PlanApply` refuses the
pair with `*LinkUnlinkError`. They are exact inverses: there is no coherent
per-file resolution and no order in which both could be honoured.

`PlanApply` then inspects each destination with `os.Lstat`. This is
`applylink.go`'s `linkMode`, and the matrix is its contract — every cell is
pinned by a test in `applylink_test.go`:

| Live state | plain `apply` | `--link` | `--unlink` |
| --- | --- | --- | --- |
| regular file | `ActionMerge` | `ActionSymlink`, **conflict** (would discard live-only keys) | `ActionMerge` (no link to undo) |
| symlink into `plasma/files/` | `ActionSkip` "already linked" | `ActionSkip` "already linked" | `ActionUnlink`, **no conflict** |
| symlink elsewhere | `ActionMerge`, **conflict** | `ActionSymlink`, **conflict** | `ActionUnlink`, **conflict** |
| absent | `ActionCreateFile` | `ActionSymlink` | `ActionCreateFile` (no link to undo) |

The decision returned is always the mode the caller **asked for**; the boolean
only says whether performing it is a **transition**. That split is what lets
`--backup` and `--dry-run` authorise the switch (§9.3) rather than merely
downgrading the refusal into a silent skip. Every conflicting path that survives
the narrowing of §9.3 is collected, and `PlanApply` returns
`*LinkModeConflictError{Paths}` **before any mutation**, mirroring
`symlink.LinuxDesktopGuardError`'s shape and placement.

**Amended (A20) — one cell is corrected, and two earn their asymmetry.** This
table previously said a plain apply over a file linked into `plasma/files/` was a
**conflict** ("would materialise a copy"). It is not, and the difference is the
whole of the `--unlink` design:

- **Plain apply over a repo link is `ActionSkip`, not a conflict.** The repo file
  **is** the live file, so there is nothing to converge and nothing to discard.
  Treating it as a transition made a fully linked, converged tree refuse a run
  that would have touched nothing — a refusal whose own premise was false. The
  skip states the way out by name: `already linked; the repo file is the live
  file (--unlink restores copy mode)`.
- **`--unlink` over a repo link is not a conflict either.** It discards nothing —
  the bytes written are the ones KDE was already reading, and the repo copy is
  untouched — and the user asked for it by name, so demanding `--backup` on top
  would be ceremony. The copy → link direction keeps its conflict, because that
  one **does** discard every live-only key the merge preserves.

A symlink pointing somewhere else is never this tool's to reinterpret, so it
conflicts in all three modes. A file with no `plasma/files/` copy is never
linkable in any mode: there is no target, and a symlink to a non-existent path is
a dangling link KDE would silently treat as an empty config — so `--link` skips
it with `nothing to apply` and `--unlink` falls through to the ordinary write
path.

Two of the three transitions can be **judged** inside `planLinkMode`, because a
symlink materialisation and a symlink replacement both always write: the guard
either refuses them there or lets them through. Only the plain-apply-over-a-
foreign-symlink cell must defer its judgement, because whether it writes at all
depends on what the merge produces — which is what §9.3's narrowing is for. Note
the two are not the same as "decided": an `--unlink` verdict is judged in place
but still returns to the merge path, carrying an `Unlink` flag, because the merge
is what computes the content the real file will hold.

### 9.3 The `--backup` interaction

The conflict guard is satisfied by `--backup` (which makes the transition
recoverable) or by `--dry-run` (which makes it a preview). Exactly:

```go
guardSatisfied := opts.DryRun || opts.Backup
// … per-file planning, then:
switch {
case len(conflicts) == 0:
    // nothing transitions, or nothing that transitions would be acted on
case !guardSatisfied:
    return nil, &LinkModeConflictError{Paths: conflicts}
case opts.DryRun && !opts.Backup:
    plan.LinkConflicts = conflicts
}
```

**Amended (A20) — two shipped behaviours this passage did not describe.**

- **A conflict is narrowed to a file the run would actually mutate.**
  `resolveLinkGuard` is the last step of planning any file whose requested mode
  is a transition the merge path had to plan before the guard could judge it, and
  it drops the conflict when the resolved action is `ActionSkip`. Without that
  narrowing, `dotf plasma apply --yes` refused — exit 1 — over twenty **converged**
  files, naming files it would not have touched. A guard that refuses a run on
  behalf of files the run would not modify has a false premise, and this is the
  code that keeps the premise true. (When the guard is unsatisfied, the
  conflicting file's action is additionally rewritten to `ActionSkip` with
  `Reason: "link-mode conflict"`, so a refusal cannot leave a mutating action in
  a plan that is about to be returned as an error.)
- **A dry run carries the refusal out on the plan.** In the `--dry-run` without
  `--backup` case — the one where `--dry-run` **alone** authorised the transition
  — the conflicts travel on `Plan.LinkConflicts` (§5.3), and the CLI renders them
  as the `plasma.link-conflict=` / `plasma.link-conflict.refusal=` stdout lines
  and the `linkConflicts` JSON field (§8.3, §8.5). With `--backup` also given the
  transition is authorised for the real run too, so there is nothing to warn
  about and the field stays empty; outside `--dry-run` the refusal **is** the
  error.

So:

- `dotf plasma apply --link` on a fresh machine (no live files) → plans symlinks,
  no guard triggered.
- `dotf plasma apply --link` on a machine with real live files → refused, with
  the exact remedy in the message.
- `dotf plasma apply --link --backup` → each live file is copied into
  `Plan.BackupDir` first, then replaced by the symlink.
- `dotf plasma apply` on a linked machine → **amended (A20)**, every linked file
  is `ActionSkip` "already linked", with or without `--backup`. This bullet
  previously said the symlink was removed and a merged regular file written; that
  described the pre-`--unlink` behaviour the corrected matrix of §9.2 replaces,
  and no plain apply does it in either direction now.
- `dotf plasma apply --unlink` on a linked machine → each symlink is replaced by
  a real merged file. No guard fires (a repo link is not a conflict), so no
  `--backup` is required; with `--backup` the symlink's _target content_ — the
  repo file, followed via `os.Stat` — is copied into `Plan.BackupDir` first.
- `dotf plasma apply --unlink` over a symlink pointing **outside** the repo →
  conflict, refused with the same message and the same `--backup` / `--dry-run`
  remedy.

`--backup` also applies outside conflicts: with it, every file the run touches is
copied into `Plan.BackupDir` before it is modified (§6.9). Without it, a merge
still happens — the merge is non-destructive to live-only keys by construction —
but there is no undo for an overwritten value.

### 9.4 Linked files and the rest of the pipeline

- `export` on a linked file would read the repo file, strip it, and write it back
  to itself. That is a no-op in the converged case and a _destructive_ re-strip
  in the case where KDE has written volatile state into the repo file. This is
  correct and desirable — it is how a linked setup gets re-cleaned — but it is
  surprising, so `PlanExport` emits `Reason: "re-stripping a linked file"` and
  the run prints the `SessionOwned` advisory (§8.4) for any linked file.
- `diff` on a linked file compares a document with itself and always reports
  converged — **but only without `--recommended`**. **Amended (A5):** with the
  overlay layered on, the comparison is snapshot-plus-overlay against the
  snapshot, so every overlay key the snapshot lacks is real drift that no apply
  can ever fix. That asymmetry is precisely why `--link` and `--recommended` are
  now refused as a pair (§9.2). `status` counts a linked file in `LivePresent`
  and `RepoPresent`.
- `dotf link` and `dotf plasma` never claim the same path (§3.1), so a plasma
  symlink can never be clobbered by the symlink engine.

---

## 10. Live reload

### 10.1 The command table

Derived by `ReloadPlan(plan)` from the set of files a plan will actually modify.
Deduplicated, emitted in this fixed order.

| Component | Triggered by | Argv |
| --- | --- | --- |
| `lookandfeel` | any `kdedefaults/*` change | `plasma-apply-lookandfeel -a <package>` where `<package>` is the single line of `plasma/files/kdedefaults/package` |
| `kwin` | `kwinrc`, `kcminputrc`, `kdeglobals` | `qdbus org.kde.KWin /KWin reconfigure` |
| `kglobalaccel` | `kglobalshortcutsrc` | `systemctl --user restart plasma-kglobalaccel` |
| `plasmashell` | `plasmarc`, `plasmanotifyrc`, `klipperrc`, `krunnerrc`, `kdeglobals` | `kquitapp6 plasmashell` then `kstart plasmashell` (two commands, one `Component`) |

Files not listed above need no reload: they are read by their application at next
start (`dolphinrc`, `katerc`, `katevirc`, `konsolerc`, `kiorc`, `mimeapps.list`,
`baloofilerc`, `kwalletrc`, `ksmserverrc`, `kscreenlockerrc`, `powerdevilrc`,
`kded5rc`, `plasma-localerc`).

**`qdbus` binary resolution, at PLAN time. Amended (A18).** The binary name
varies across distributions: only `qdbus6` ships on a Plasma 6 host. `resolveArgv`
probes `Runner.LookPath` in order `qdbus6`, `qdbus`, and substitutes the first
that resolves.

Resolution runs in `PlanApply`, via `Engine.resolveReloads`, so `Plan.Reloads`
names the binary that actually exists on this host **whether or not `--reload`
was given**. `ReloadPlan` itself stays pure — it takes no `Runner` and reads
nothing — and therefore emits the generic `qdbus` name that `resolveReloads` then
rewrites in place.

That placement is the whole point. Resolution used to live only inside
`Engine.Reload`, which returns early unless `Options.Reload` is set (§10.3) — so
the far more common path, printing the commands the run deliberately did **not**
execute, advertised a binary that does not exist. Those printed lines exist to be
copy-pasted (§10.2), so a command that is unresolvable as printed defeats its
only purpose. `TestPlanApply_ResolvesReloadArgvWithoutTheReloadFlag` pins it.

A command whose binary resolves **nowhere** is treated differently by the two
paths, deliberately:

- `Plan.Reloads` keeps it exactly as planned, so it is still printed. The reload
  set is advisory there, and naming the component that needs reloading is more
  useful than silently omitting it.
- `Engine.Reload` skips it rather than running it, and a missing reload is never
  a run failure. The skip is silent — no stderr line is emitted for it.

Every command goes through the injected `exec.Runner`. Never `sh -c` with an
interpolated string; `internal/exec`'s package doc forbids it and it would make
the tests non-hermetic.

### 10.2 Why it is opt-in

`kquitapp6 plasmashell` tears down the panel, the system tray, every widget, and
any open KRunner or notification, mid-session. `systemctl --user restart
plasma-kglobalaccel` momentarily drops every global shortcut. On a machine the
user is actively working on, an automatic reload is materially disruptive and is
not what someone running `dotf plasma apply` to sync a keybinding expects.

So: **`--reload` is opt-in, and without it `apply` prints the exact commands it
would have run** as `plasma.reload.<n>=` stdout lines. The user can paste them,
or ignore them and pick the changes up at next login. This makes the default
path fully non-disruptive while keeping the knowledge in the tool rather than in
a README.

### 10.3 Gating

`Engine.Reload` returns `(nil, nil)` immediately when `!opts.Reload` or
`opts.Session != SessionLive`. The reload commands are still _planned_, still
_resolved_ (§10.1), and still printed in both cases — they are information, not
an action. A failed reload command is recorded as a per-file-style stderr line
(`dotf: plasma: reload <component>: <err>`) and contributes to exit 1, but never
aborts the remaining reloads.

**Amended (A18) — what gets printed is the executed form where there is one.**
`Engine.Reload` returns the commands that actually **ran**; `cmd/dotf` stores
that slice on `Report.Reloaded` (§5.3) and overlays it onto `Plan.Reloads`, so
both the `plasma.reload.<n>=` lines and the `--json` `reloads` array print what
ran, while a command that was planned but never executed is still listed in
place. `Reload` appends in plan order, so the executed set is an in-order
subsequence of the planned one and a two-pointer walk pairs them; the pairing
predicate compares `argv[0]` as well as the tail, because `plasmashell`
contributes **two** commands with the same `Component` and the same argument
list (`kquitapp6` then `kstart`).

`--reload` with no live session prints:

```text
dotf: plasma: --reload requested but no KDE session detected; skipping
```

---

## 11. `dotf apply` integration

### 11.1 Flag and env twin

`dotf apply` gains an opt-in `--plasma` flag, default **off**, modelled exactly
on `--linux-desktop` and `--snapper`.

Five mechanical edits:

1. `cmd/dotf/applyopts.go` — add `Plasma bool` to `applyFlags`; register it in
   `setApplyFlags`:

   ```go
   fs.BoolVar(&f.Plasma, "plasma", false,
       "sync the KDE Plasma config set (Linux/KDE only, no-op elsewhere)")
   ```

   add `Plasma bool` to `applyOptions`; OR it with its env twin in
   `resolveApplyOptions`:

   ```go
   Plasma: f.Plasma || envTrue(getenv("PLASMA")),
   ```

2. `cmd/dotf/applyphases.go` — the phase function (§11.2) and extensions to
   `applyDryRun` / `printApplyDryRun`.
3. `cmd/dotf/apply.go` — the pipeline call (§11.3) and doc-comment update.
4. `cmd/dotf/helptext.go` — extend the `envHelp` const's apply-twins list and
   `apply`'s `long` prose, both of which enumerate the twins inline.
5. Tests: `help_test.go` `TestSetApplyFlags_Parses` and the `"apply"` case of
   `TestSetFlags_ExactFlagNames`; `applyopts_test.go` gains the `PLASMA` env
   parity row; `apply_test.go` covers the phase and its dry-run output.

**Research correction — the env twin is `PLASMA`, not `DOTF_PLASMA`.** The
agreed design named `DOTF_PLASMA`. The repo's apply-twin convention is
_unprefixed_: `resolveApplyOptions` reads `NON_INTERACTIVE`, `FORCE_INSTALL`,
`ENABLE_GAMING`, `ENABLE_OLLAMA`, `LINUX_DESKTOP`, `SNAPPER` — a `bootstrap.sh`
contract in which only `--no-user-config`'s twin carries the `DOTF_` prefix
(because it is registered outside the twin mechanism). Using `DOTF_PLASMA` would
either break `resolveApplyOptions`'s uniform `getenv(NAME)` shape or introduce a
one-off exception. The spec follows the convention: the twin is `PLASMA`. Its
genericness is accepted as the cost of parity, and `envTrue` is exact-match
(`v == "true"`), so `PLASMA=1` or a stray `PLASMA=kde` is _not_ truthy, which
substantially reduces the collision risk.

### 11.2 The phase function

```go
func applyPlasma(
    pe *plasma.Engine,
    files []plasma.FileSpec,
    opts applyOptions,
    popts plasma.Options,
    stderr io.Writer,
    coll *failure.Collector,
) *plasma.Report
```

Shaped exactly like `applySymlinks` (`applyphases.go:151`):

```go
plan, err := pe.PlanApply(files, popts)
if err != nil {
    coll.Record("plasma", err)
    return nil
}
report, err := pe.Execute(plan, popts)
if err != nil {
    coll.Record("plasma", err)
    return report
}
_, verr := pe.Validate(files, popts)
coll.Record("plasma", verr)
return report
```

Exactly **one** `coll.Record("plasma", …)` call on every path, including
`Record("plasma", nil)` on the happy path so the run-wide step count stays
honest. A phase never aborts the run.

**Requested-but-unavailable is a warning, not a failure**, following
`applySnapshot`'s precedent: when `opts.Plasma` is set but
`plasma.Support(info) == SupportUnsupported`, the phase writes
`dotf: plasma requested but the host is not Linux; skipping` to stderr, records
**nothing**, and returns nil. Same for a missing `plasma/files/` tree
(`*MissingRepoTreeError`) — a repo that has never run `export` is not a failed
apply.

The phase never prompts: confirmation is a `runPlasma` concern, and `dotf apply`
is the headless orchestrator. `plasma.Options` is constructed as:

```go
func applyPlasmaOptions(opts applyOptions) plasma.Options {
    return plasma.Options{
        DryRun:   opts.DryRun,
        Backup:   opts.Plasma, // mirror applySymlinkOptions' Backup: opts.LinuxDesktop
        Validate: true,
        Reload:   false, // never restart the shell from a headless apply
        Session:  plasma.DetectSession(os.Getenv),
    }
}
```

`Backup: opts.Plasma` mirrors `applySymlinkOptions`'s `Backup: opts.LinuxDesktop`:
opting into a destructive optional phase from the orchestrator also opts into its
safety net. `Link` and `Recommended` are deliberately always false from
`dotf apply` — both are deliberate, interactive choices.

### 11.3 Phase ordering

In `runApply`'s phase sequence, `applyPlasma` runs **after** `applySymlinks` and
**before** `applyFirewall`:

```text
snapshot → sysconfig → packages → symlinks → plasma → firewall → summary
```

Rationale: plasma writes into `$HOME/.config`, which is exactly where symlinks
have just been provisioned, and the ownership assertion of §3.1 means the two can
never touch the same path — but running plasma second keeps the invariant that
`$HOME` link topology is settled before any file content is merged. Firewall is
privileged system work and stays last.

Sudo is **not** primed for this phase: plasma is unprivileged `$HOME` work. The
single sudo prime already happens once, earlier, at `apply.go:144-149`.

`applyDryRun` calls only `pe.PlanApply` (never `Execute`/`Validate`) and hands
the plan to `printApplyDryRun`, which appends the plasma bucket lines through the
same `plasmaActionLines("plasma", plan.Files)` formatter used by
`dotf plasma apply --dry-run`. There is no second renderer.

---

## 12. The `recommended/` overlay

### 12.1 What it is

`plasma/recommended/` is a hand-authored, opt-in overlay of **40 settings across
10 files** — power-user defaults that the user has _not_ set on the audited host. It is deliberately separate from
the captured snapshot so that "what my machine is" and "what I think a good KDE
should be" never blur together in git history.

It has its own `plasma/recommended/README.md` restating every risk column below,
because several entries are genuine tradeoffs and one
(`XwaylandEavesdrops=None`) can break unrelated applications.

**The count is 40 settings across 10 files and is correct as written.** It is
called out because an intermediate attempt at the §12.13 fix deleted the four
apply-only settings — taking it to 36 across 9 — before that approach was
abandoned in favour of governing them. The overlay data is intact; a later
editor reconciling notes must not resurrect the smaller numbers.

### 12.2 How it layers

`--recommended` composes three documents per file, in strictly increasing
precedence:

```text
live  <  plasma/files/<path>  <  plasma/recommended/<path>
```

Implementation: `PlanApply` parses `plasma/files/<path>` into `R`, then, when
`Options.Recommended` and `plasma/recommended/<path>` exists, merges the
recommended document into `R` using the _same_ merge rules of §6.2. The combined
`R` is the action's `Source`, and it is merged into the live document at write
time (§6.7).

**Amended (A3) — the layering is resolved BEFORE the presence check.** Either
side may be absent, and **the overlay alone is a complete source**. `applySource`
therefore runs before any "is there anything to do?" decision, in both
`PlanApply` and `Diff`. Resolving it afterwards — the original implementation —
made `--recommended` a silent no-op for the four overlay files that have no
exported snapshot (`baloofilerc`, `klipperrc`, `ksmserverrc`, `plasmarc`), and
made `diff --recommended` report them **converged** while none of their settings
had ever been applied. `nil` means only "this run asserts nothing about this
file": no snapshot, no overlay, or a document naming no entry at all.

Consequences:

- A key in both `files/` and `recommended/` takes the **recommended** value.
- A key only in `recommended/` is added.
- A key only in `files/` is unaffected.
- Live-only keys are still preserved — `--recommended` changes what gets
  asserted, never what gets deleted.
- `export` **never** writes into `recommended/`. If the user adopts a
  recommended value through System Settings, the next `export` captures it into
  `files/`, where it then becomes redundant-but-harmless (both sides agree) —
  **except for the four apply-only settings of §12.13**, which the strip table
  removes on the way out and which therefore never reach `files/` at all.
- `diff` and `status` honour `--recommended` too, so
  `dotf plasma diff --recommended` answers "am I converged on files + overlay?"
  while plain `dotf plasma diff` answers "am I converged on my snapshot?".

Every file named below is a `DefaultFiles()` candidate (§3.2), so every
recommended key is visible to `diff`, `status`, and `doctor` — with **four named
exceptions** that the strip table deletes by design. They are enumerated and
governed in §12.13, and `--link` cannot be combined with `--recommended` at all
(§9.2).

### 12.3 `baloofilerc`

| Group / key | Value | Why | Risk |
| --- | --- | --- | --- |
| `[Basic Settings] Indexing-Enabled` | `true` | Keeps Baloo on so Dolphin/KRunner file search and tag/rating metadata still work, while the following keys narrow what it touches. The alternative, `false`, turns file search off entirely | None if you want search. Set `false` only if you exclusively use fd/rg/fzf — then Dolphin's search box and KRunner file results go dead |
| `[General] only basic indexing` | `true` | Indexes filenames and basic filesystem metadata but skips full content extraction. This box currently has 653,134 files indexed **with** content extraction, the single largest Baloo CPU/IO/disk cost on a dev workstation | You lose full-text search inside documents and source from Dolphin and KRunner; filename search still works. A real tradeoff, not free |
| `[General] folders[$e]` | `$HOME/` | Anchors the index root to the home dir. The `[$e]` flag is KConfig shell expansion, expanded at read time — this is what makes the config portable instead of hardcoding `/home/yandy` | None. This is the default value, written explicitly so the overlay is self-documenting and machine-independent |
| `[General] exclude folders[$e]` | `$HOME/.cache/,$HOME/.local/share/Trash/,$HOME/.local/share/containers/,$HOME/.local/share/Steam/,$HOME/.local/state/,$HOME/.var/,$HOME/.docker/,$HOME/.npm/,$HOME/.nvm/,$HOME/.pyenv/,$HOME/.rbenv/,$HOME/.gradle/,$HOME/.m2/,$HOME/.cargo/,$HOME/.rustup/,$HOME/go/pkg/,$HOME/.conda/,$HOME/.virtualenvs/,$HOME/.ollama/,$HOME/.wine/,$HOME/snap/,$HOME/.mozilla/,$HOME/.thunderbird/,$HOME/.config/Code/,$HOME/.vscode/,$HOME/.vscode-server/,$HOME/.zed/,$HOME/.claude/` | Path-prefix exclusion (unlike `exclude filters`, which matches basenames). Removes toolchain caches, package-manager stores, container/VM images and model weights — collectively the bulk of a dev home's file count | Anything under these trees becomes unfindable via Dolphin/KRunner search. `$HOME/.claude/` in particular hides agent session data — drop that entry if you search it. KDE writes this key **with** the `[$e]` suffix, and so must the overlay — see the note below on what that flag does |
| `[General] exclude filters` | the verbatim upstream KF6.29 default list, plus `target,vendor,dist,.next,.nuxt,.svelte-kit,.turbo,.parcel-cache,.pnpm-store,bower_components,.gradle,.m2,.cargo,.rustup,.ccache,.sccache,.tox,.nox,.mypy_cache,.pytest_cache,.ruff_cache,.eggs,*.egg-info,site-packages,.direnv,.stack-work,.dart_tool,.pub-cache,.zig-cache,zig-out,Pods,DerivedData,.terragrunt-cache,.bloop,.metals,.idea,.vscode-server,*.o,*.a,*.rlib,*.rmeta,*.dylib,*.dll,*.wasm,*.pack,*.idx,*.pdb,*.log,*.lock,*.sqlite,*.sqlite3,*.wal` | Basename glob exclusion matched against every file and directory name anywhere in the tree. Upstream already covers `.git`/`node_modules`/`__pycache__`/`.venv`/`.terraform`/`CMakeFiles`/`.ninja_*`; this adds the ecosystems it misses | **Writing this key REPLACES upstream's list wholesale — it is not additive**, which is why the full default list is reproduced. `target`, `vendor` and `dist` are generic words, so a legitimately-named folder called `dist` becomes unsearchable; `build` and `out` were deliberately left out as too generic. Baloo may merge newly-shipped upstream defaults in when `exclude filters version` is stale — harmless but it will show as `dotf plasma diff` noise after a Plasma upgrade |
| `[General] index hidden folders` | `false` | Keeps dotfile trees out of the index. This is the upstream default, pinned explicitly so an accidental System Settings toggle shows up as drift | None beyond the obvious — hidden config files will not appear in search results |

**Amended (A19) — what `[$e]` actually does.** An earlier revision of this table
claimed that a plain `exclude folders`, written without the `[$e]` suffix, is
"silently ignored" by KConfig. That is false, and it was verified false against
`kreadconfig6` on a KF6.29 host: an unflagged key is read back perfectly
normally.

`[$e]` is the **expansion** flag. It marks the value for shell/environment
expansion **at read time**, so `folders[$e]=$HOME/` is handed to Baloo as
`/home/<user>/` while an unflagged `folders=$HOME/` is handed over with the
literal three characters `$HOME` intact. The flag governs how the value is
interpreted, never whether the key is visible.

The consequence for this overlay is the same either way — keep the `[$e]` — but
for the right reason. These are `PathList` values written as `$HOME/…`
specifically so the repo copy stays portable and machine-independent (§14.1
forbids exporting an absolute home path at all). Drop the flag and the key is
still read, still diffed, and still applied; Baloo simply tries to index a
directory literally named `$HOME`. A key that quietly does the wrong thing is
worse than one that is ignored, which is why the corrected reason matters rather
than being a pedantic footnote.

### 12.4 `kwinrc`

| Group / key | Value | Why | Risk |
| --- | --- | --- | --- |
| `[Windows] FocusStealingPreventionLevel` | `0` | `0` = None. An app launched from a terminal (`zed .`, `code .`, `xdg-open`) gets focus immediately instead of flashing in the task manager. The single most-felt setting for a CLI-first workflow; the default is `1` (Low) | Genuinely lowers a guardrail: any app can pull focus at any moment, so a finishing background build or an update dialog can steal keystrokes mid-typing. `1` is the safe middle |
| `[EdgeBarrier] EdgeBarrier` | `0` | Disables the pointer sticky barrier between monitors (default 100 px of resistance). On a multi-monitor desk the cursor crosses screens instantly | Slightly easier to overshoot onto the wrong screen when aiming at a control near a shared edge |
| `[EdgeBarrier] CornerBarrier` | `false` | Removes the extra sticky zone at shared screen corners, so throwing the cursor into a corner to hit a far-monitor close button works first try | Same overshoot tradeoff. Keep `true` if you rely on corner-slamming for a hot corner |
| `[Windows] ActivationDesktopPolicy` | `BringToCurrentDesktop` | With 10 virtual desktops the default (`SwitchToOtherDesktop`) yanks you away from your workspace whenever an already-open app is re-activated. This pulls the window to you instead | Windows accumulate on whichever desktop you are on, quietly undoing a deliberate app-per-desktop arrangement. `DoNothing` only raises a taskbar highlight |
| `[Windows] RollOverDesktops` | `true` | Makes desktop-switch shortcuts wrap from 10 back to 1 instead of dead-ending. With a 10×1 row this turns a 9-step traverse into 1 | None functionally; you can circle past your target if you hold the key. Does not touch the existing Meta+1..0 direct-switch bindings |
| `[Windows] BorderlessMaximizedWindows` | `true` | Maximized windows drop titlebar and border, reclaiming ~30–40 px of vertical space for editor and terminal rows | Real: a maximized window has no titlebar to grab, no visible close/minimize buttons, and no titlebar context menu. Use Meta+Q / Alt+F3. Do not enable if you mouse-drag maximized windows |
| `[Windows] BorderSnapZone` | `0` | Turns off the 10 px magnetic pull toward screen edges when dragging. Tile geometry, not an invisible magnet, should decide final position | Freehand placement near an edge no longer auto-aligns, so a dragged window can sit 2 px off. Set back to `10` if you miss it |
| `[Windows] ElectricBorderMaximize` | `false` | Stops auto-maximize when a drag brushes the top screen edge — a frequent misfire on a multi-monitor setup where windows cross the top boundary constantly | You lose drag-to-top as a maximize gesture. `ElectricBorderTiling` (drag-to-side half-snap) is deliberately left alone since it complements the tile layouts |
| `[TabBox] DelayTime` | `0` | Removes the 90 ms hold delay before the Alt+Tab switcher appears, so a quick tab-and-release feels instantaneous | The switcher UI now flashes even for the fastest window flick, which some read as visual noise |
| `[TabBox] HighlightWindows` | `false` | Stops KWin raising and spotlighting each window as you scrub the Alt+Tab list, avoiding a cascade of repaints and stacking churn across 10 desktops and multiple monitors | You lose the live preview of where the candidate window is; you navigate by thumbnail and title only |
| `[Plugins] blurEnabled` | `false` | Background blur is a per-frame GPU pass over every translucent surface. The terminals here are opaque, so it buys nothing visually while costing GPU time on every panel repaint | Panel, KRunner and menu backgrounds go flat-translucent instead of frosted. A cosmetic side effect of a performance change; BreezeDark itself is untouched |
| `[Plugins] shakecursorEnabled` | `false` | Disables shake-to-magnify, which fires accidentally when flicking the pointer fast between monitors and briefly hijacks cursor size | None, unless you actually use shake-to-find. `shakecursor` is confirmed in KWin's loaded-effects list on this machine |
| `[Plugins] startupfeedbackEnabled` | `false` | Kills the busy cursor shown while an app launches; on a fast NVMe box the feedback outlives the launch | No visual confirmation that a click started something, so you may double-launch a slow app |
| `[Compositing] AllowTearing` | `false` | KWin 6 defaults this to `true`, letting fullscreen apps present unsynced frames. On a workstation, tear-free presentation is the better tradeoff | Adds up to one frame of latency in fullscreen games and emulators that request tearing. Leave `true` if you game on this box |
| `[NightColor] NightTemperature` | `4000` | NightColor is already `Active=true` with no explicit temperature, so it uses the 4500 K default. 4000 K is a meaningfully warmer evening target while staying above the ~3400 K point where syntax-highlight hues collapse into each other | Colors shift enough that evening color work (design, image editing, checking a UI palette) is misleading. 4500 K is gentler, 3500 K aggressive |
| `[Xwayland] XwaylandEavesdrops` | `None` | Security posture: controls how much global keyboard input legacy X11 apps observe through Xwayland. The default `Combinations` lets any X11 client watch modifier-key combos system-wide. `None` closes that channel | Breaks X11 apps that legitimately need global hotkeys — Discord/Mumble push-to-talk, some screen recorders, older remote-desktop clients. `Modifiers` is the middle ground; `All` is the permissive, insecure setting |

Note: `[Xwayland]` is a **stripped group** in `plasma/files/kwinrc` (§7.3) because
the captured file's Xwayland scaling keys are display-local. The recommended
overlay asserts `XwaylandEavesdrops` explicitly, which is the split the two-tree
design exists for: capture strips the group, the overlay re-asserts the one key
that is intent.

**Amended (A9) — say the consequence too.** The original note presented this
split as unambiguously virtuous and stopped there. It has a real cost:
`XwaylandEavesdrops` is **apply-only**. `apply --recommended` writes it and it
works, but `export` can **never** capture it, so `plasma/recommended/kwinrc` is
its only record in the repo and deleting that line deletes the setting outright
— there is no `plasma/files/` copy to fall back on. This is the tradeoff the
docs were papering over; it is now stated here, in
`plasma/recommended/README.md` under "The four apply-only settings", and
governed by §12.13. If a future maintainer would rather it not be apply-only,
the correct change is a **strip-table narrowing** — split `[Xwayland]` into its
display-local keys — exactly as was done for `krunnerrc` in §7.3.1, not a
deletion of the overlay data.

### 12.5 `kdeglobals`

| Group / key | Value | Why | Risk |
| --- | --- | --- | --- |
| `[KDE] AnimationDurationFactor` | `0.5` | Halves every Plasma and KWin animation (window open/close, desktop switch, Overview, popups). The single highest perceived-snappiness setting, and it applies to both shell and compositor from one location — `kdeglobals` and `AnimationDurationFactor` are both referenced from `libkwin.so.6`, and the Animations KCM writes it here | None functionally. `0` disables animations entirely, which makes focus changes hard to follow; `0.5` keeps them legible. Log out/in or restart plasmashell for the shell side to fully pick it up |
| `[KDE] ShowDeleteCommand` | `true` | Adds a permanent Delete entry alongside Move to Trash in Dolphin and all KIO file dialogs, removing the trash-then-empty dance for build output and scratch dirs. Pairs with the existing `kiorc ConfirmDelete=true` so the destructive path still prompts | A permanent-delete entry now sits one menu position from Move to Trash. `ConfirmDelete=true` is the safety net — do not enable this if you ever turn that off |
| `[General] TerminalApplication` | `ghostty` | Sets the terminal KIO launches for Dolphin's Open Terminal Here, "Run in terminal" in KIO dialogs, and `.desktop` entries with `Terminal=true`. Without it KIO falls back to konsole, then xterm | Must be a binary on PATH; a typo silently falls back to konsole/xterm. Swap for `alacritty` if preferred. Some KIO callers pass Konsole-style arguments, so an exotic terminal can be invoked with flags it does not understand |
| `[General] XftHintStyle` | `hintslight` | Slight hinting preserves designed glyph shapes and inter-letter spacing of programming fonts instead of snapping stems to the pixel grid — which is what makes ligature-heavy fonts (Fira Code, JetBrains Mono, Iosevka) look uneven under full hinting | On a low-DPI panel `hintfull` genuinely renders crisper. Taste, and display-dependent — verify on your actual monitor rather than accepting it blind. The key is confirmed present in `kcm_fonts.so`; the `[General]` placement is the documented kdeglobals convention |
| `[General] XftAntialias` | `true` | Pins font antialiasing on for all Qt/KDE apps so the overlay does not inherit whatever a distro or a stray KCM click left behind | None — this is the sane default, pinned only so drift is visible in `dotf plasma diff`. `XftSubPixel` is deliberately **not** recommended: the correct value depends on your panel's subpixel layout and a wrong guess produces color fringing |

### 12.6 `kcminputrc`

| Group / key | Value | Why | Risk |
| --- | --- | --- | --- |
| `[Keyboard] RepeatDelay` | `250` | Milliseconds before a held key repeats (Plasma default 600). 250 ms makes held h/j/k/l navigation in Neovim/Kate respond almost immediately instead of stalling for over half a second. `Keyboard`, `RepeatDelay` and `RepeatRate` are all read by `libkwin.so.6`, which owns keyboard repeat on Wayland | Below ~200 ms you get unintended repeats from ordinary typing, especially on a hall-effect board like the Keychron K4 HE with a shallow actuation point. 250 is deliberately conservative |
| `[Keyboard] RepeatRate` | `50` | Repeats per second once repeat starts (default 25). Roughly doubles cursor travel for held motions in modal editors and log scrollback | Fast enough to overshoot a target line when you hold a motion a beat too long. Drop to 35 if you constantly correct |
| `[Keyboard] NumLock` | `0` | `0` = turn on at session start, so the numpad produces digits rather than acting as arrow keys | Only relevant on a full-size board; the K4 HE has a numpad. Values are `0`=on, `1`=off, `2`=leave unchanged. Use `1` if you deliberately use the numpad as a navigation cluster |

Note: `[Keyboard]` is a **stripped group** in `plasma/files/kcminputrc` (§7.3);
as with `[Xwayland]` above, the overlay re-asserts the intent-bearing keys —
**and, as with `[Xwayland]`, all three are apply-only.** `export` can never
capture `NumLock`, `RepeatDelay` or `RepeatRate`, so `plasma/recommended/kcminputrc`
is their only record in the repo (§12.4's amendment, §12.13). `[Keyboard]` is
rewritten as a unit by both `libkwin.so.6` and `kf6/kded/keyboard.so`, so
narrowing the group rule would mean enumerating and re-auditing KWin's keyboard
key set every release; the group rule stands and the exception is governed
instead.

### 12.7 `kscreenlockerrc`

| Group / key | Value | Why | Risk |
| --- | --- | --- | --- |
| `[Daemon] LockGrace` | `0` | Security posture that respects the existing `Autolock=false` choice: `LockGrace` is the window after the lockscreen engages during which it can be dismissed **without** a password. `0` means a deliberate Meta+L always demands the password immediately | None beyond losing the "oops, I meant to keep working" escape hatch in the first seconds after locking. Deliberately does not touch `Autolock=false` / `Timeout=0` / `LockOnResume=false` |

### 12.8 `ksmserverrc`

| Group / key | Value | Why | Risk |
| --- | --- | --- | --- |
| `[General] loginMode` | `emptySession` | Starts each Plasma session clean instead of resurrecting whatever was open at logout. On a dev box, session restore reopens half-dead editors, stale terminals in deleted worktrees, and browser windows across all 10 desktops, and slows login measurably. The saved sessions here are already empty (`count=0`), so this makes the intent explicit | You genuinely lose "pick up where I left off". The two named alternatives confirmed in the ksmserver binary are `restorePreviousLogout` (the default) and `restoreSavedSession`; any other value, conventionally `emptySession`, falls through to an empty session |

### 12.9 `klipperrc`

| Group / key | Value | Why | Risk |
| --- | --- | --- | --- |
| `[General] MaxClipItems` | `100` | Clipboard history depth. The stock depth is far too shallow for a workflow copying error strings, commit SHAs and snippets between terminal, editor and browser all day; 100 turns Klipper into usable scrollback | Real security consideration: a deeper history means secrets pasted from a password manager linger in memory and in Klipper's saved history much longer. Pair with `KeepClipboardContents=false` if you paste credentials often |
| `[General] IgnoreImages` | `false` | Keeps images in history, so a Spectacle region capture stays retrievable after you copy something else | Images make the history noticeably heavier in RAM and on disk. Set `true` if you notice plasmashell memory growth |
| `[General] IgnoreSelection` | `true` | Keeps the PRIMARY selection (anything merely highlighted with the mouse) out of history. Without it, every drag-select in a terminal or editor floods the history and buries what you actually pressed Ctrl+C on | Middle-click paste of the primary selection still works; only the _history_ stops recording selections. Set `false` if you scroll back through past mouse-selections |

### 12.10 `dolphinrc`

| Group / key | Value | Why | Risk |
| --- | --- | --- | --- |
| `[General] ShowFullPath` | `true` | Shows the absolute path in the location bar instead of breadcrumb-only, so a path is readable and copyable at a glance for pasting into a terminal | None. Slightly denser location bar for deeply nested trees. Pair with `EditableUrl=true` if you would rather type paths |
| `[General] RememberOpenedTabs` | `false` | Dolphin starts at home instead of restoring last session's tabs — which on a dev box means tabs pointing at deleted worktrees, unmounted NAS shares, and cleaned build dirs that each stall on startup | You lose Dolphin session restore. Same tradeoff family as `ksmserverrc loginMode`; adopt both or neither for consistency |

### 12.11 `plasmarc`

| Group / key | Value | Why | Risk |
| --- | --- | --- | --- |
| `[PlasmaToolTips] Delay` | `-1` | Disables the large hover tooltips on panel widgets and task-manager entries (default 0.7 s). They pop up constantly while reaching for the system tray and cover the thing you were aiming at | You lose window previews on task-manager hover, which some people use for window picking. A positive number (e.g. `2.0`) is the softer option — just a longer delay. `-1` as the disable sentinel is the documented convention, not something verified on this host |

### 12.12 `krunnerrc`

| Group / key | Value | Why | Risk |
| --- | --- | --- | --- |
| `[General] RetainPriorSearch` | `false` | KRunner opens empty every time instead of pre-filling and pre-selecting the last query. For a launcher invoked dozens of times an hour, a stale prefilled query is a latent mis-launch waiting for an early Enter | You lose one-keystroke repeat of the previous search. Taste, but it favors safety. The existing `FreeFloating=true` is untouched |
| `[General] historyBehavior` | `ImmediateCompletion` | Inline-completes from history as you type rather than waiting for the suggestion list, cutting a launch to a few characters plus Enter. Valid values are `Disabled`, `ImmediateCompletion`, `CompletionSuggestion` (default) | Aggressive inline completion can complete to the wrong prior command, and an early Enter then launches it. Use `Disabled` for no history influence at all — also the privacy-maximal choice |

**Amended (A8) — `historyBehavior` is NOT apply-only.** It was a third,
undisclosed apply-only case, because the original `[General]/history*` strip rule
deleted the very key this section recommends. That rule was wrong and has been
removed (§7.3.1), so `export` now captures `historyBehavior` into
`plasma/files/krunnerrc` like any other preference. This is the model for fixing
an apply-only setting: **fix the table where the rule is wrong, fix the data only
where the data is wrong.**

### 12.13 Invariant — `recommended/` keys must survive the strip

**Amended (A9), new section.** The spec previously did not state this at all,
which made the exception list below look like an oversight rather than a
governed contract.

> **No key in `plasma/recommended/` may match a volatile rule for its file**,
> except the four named apply-only settings. A fifth fails the build.

The rationale is that a strip rule which deletes a recommended key means
`apply --recommended` writes a setting the next `export` throws away — a setting
that works, that the project has decided a user should have, and that lives
nowhere in the repo but the overlay.

The standing exception list, in full:

| File | Group / keys | Why it is allowed |
| --- | --- | --- |
| `kcminputrc` | `[Keyboard]` — `NumLock`, `RepeatDelay`, `RepeatRate` | `[Keyboard]` is rewritten as a unit by the session; narrowing the rule means re-auditing KWin's keyboard key set every release (§12.6) |
| `kwinrc` | `[Xwayland]` — `XwaylandEavesdrops` | `[Xwayland]` also holds the display-local `Xwayland Scale` (§12.4) |

Enforced by `TestStripTable_DoesNotStripRecommendedKeys`
(`internal/plasma/table_glob_test.go`), which carries this list as an explicit
allowlist and fails on anything else, backed by the committed-payload guards in
`internal/plasma/payload_test.go`. The test currently logs exactly these four as
"stripped by design" and reports zero undocumented cases.

---

## 13. Testing strategy

All tests are in-package (`package plasma`, `package main`), no external test
packages. **No `t.Parallel()` anywhere in `internal/plasma`** — the package has
no reassigned globals today, and keeping the rule avoids the known
`t.Parallel` + package-global-stub `-race` failure that is green locally and red
on CI.

### 13.1 `internal/plasma`

**Amended (A10) — reconciled with the real tree.** The original table named
`plan_test.go`, `execute_test.go`, `strip_colors_test.go`, `privacy_test.go` and
`support_test.go`; none of those exist under those names. The planning and
execution tests split three ways alongside the source split (§5), the
colour-guard and privacy duties landed in the files that own the code they test,
`table_test.go`'s per-rule duties moved into `table_glob_test.go`, and
`payload_test.go` is new. The rows below are the shipped set.

| File | What it proves |
| --- | --- |
| `codec_test.go` | Table-driven escape/unescape for **every row of §4.2** in all three contexts, including the single-byte-only edge-space rule (`'   '` → `\s \s`, not `\s\s\s`), lowercase two-digit `\xNN`, the `\;`/`\,` carve-out, the `\x24` last-segment `$` rule with all four negative cases, and the invalid-escape rejection. **Amended (A6):** plus the corrected rules — the lone trailing backslash **dropping** in all three contexts, the conditional group-segment edge space (`[\sab\s]` keeps both, `[ ab\t ]` loses both), the decoded-0x1D segment split, the leading-BOM key escape, and `canonicalFlags`' `d`-with-propagated-`i` case |
| `codec_order_test.go` | Group ordering is UTF-16 code-unit (`U+1F600` before `U+FFFD`) while key ordering is UTF-8 byte (`U+FFFD` before `U+1F600`) — the two comparators are provably different. Plus locale tie-breaking with `""` first, and the blank-line/trailing-LF layout rules of §4.4 item 5 |
| `codec_reject_test.go` | Every row of the §4.6 fail-loud table returns a `*ParseError` with the right line number — including the three amended rows (an empty decoded key, an empty decoded group segment, a non-canonical locale tag) — and that duplicate keys / re-opened headers / `Key=` are **not** errors. Plus `Write`'s totality rejections (§4.4) |
| `codec_fixture_test.go` | Families A–C of §4.8. Family A asserts exact byte equality against `kwriteconfig6`-produced fixtures; Family B asserts one-pass normalisation then idempotence; Family C runs both over the scrubbed real corpus |
| `codec_fuzz_test.go` | `FuzzCodecRoundTrip` (Family D), seeded from the fixture corpus |
| `table_test.go` | `TestDefaultFiles_MatchesGolden` against an inline Go golden copy (the `symlink.TestTable_MatchesPlaybook` pattern); `TestDefaultFiles_Count` = 27; `TestDefaultFiles_FormatCounts` = 26 INI / 1 raw; `TestDefaultFiles_ApplyOrderPhases`; `TestDefaultFiles_PathsUnique`; `TestDefaultFiles_RulesParse`; `TestDenylist_NonEmpty`; `TestNoDenylistedPathInDefaultFiles`; `TestCheckSpec_RejectsUnsafeAndDenylisted` |
| `table_glob_test.go` | The **per-glob contract** (§7.6): for each managed file, the specific keys its rules must delete **and the specific keys they must leave alone** — including the four §7.3.1 corrections. Plus `TestStripTable_MustNotManage` (the paths that must stay denylisted, `kactivitymanagerdrc` and `kactivitymanagerd-statsrc` among them) and `TestStripTable_DoesNotStripRecommendedKeys` (the §12.13 invariant and its allowlist) |
| `strip_test.go` | Glob **mechanics**: group-glob segment-count semantics (`[Tiling][*]` does not match `[Tiling]`), any-depth key globs, a `/` inside a key name, rule-parse rejections, the `VolatileValues` predicate, the flag/locale normalisation that makes a bare pattern strip `Key[$e]`, rule ordering (group before key before value), and that `Strip` leaves its input untouched. Also the §7.4 colour guard, both directions, and that it guards `kdeglobals` only |
| `merge_test.go` | Every rule of §6.2: group creation, key overwrite, insert, live-only key preservation byte-for-byte, live-only group preservation, tombstone write-through, immutable-group skip, immutable-file refusal. The §6.1 subset comparator (a live file with 200 extra keys is converged) **and** that it compares SERIALISED flags — `TestConverged_ComparesSerialisedFlags` and `TestMerge_SkipsAnEntryThatOnlyDiffersInFlagNotation` are the §6.1 amendment's guards. Plus `Overlay` precedence and all five `DiffKind`s out of `diffDocs` |
| `apply_test.go` | The shared apply surface: `PlanApply`'s presence matrix (§6.6), missing repo tree, backup ordering, the `*LinkModeConflictError` matrix of §9.2 and its `--backup`/`--dry-run` escape, mode preservation, atomic-write temp cleanup, the recommended overlay, and immutable-file refusal. `TestPlanApply_BackupDirIsStable` asserts the **operational** contract — `Execute` **reads** `Plan.BackupDir` and never re-derives it, provable only by repointing the plan at a path no timestamp derivation could produce and asserting the backup lands there, since a same-second re-derivation reproduces the identical string. (The original row said "`BackupDir` computed once and stable", which was satisfiable by a tautology, and was satisfied by one for the whole life of the branch.) |
| `applyplan_test.go` | The **planning-half** amendments: an unreadable live file is not treated as absent and an unreadable repo file is not silently skipped (§6.6); a recommended-only file is a first-class source, both standalone and merged into a live file (§12.2); `--link` with `--recommended` is refused (§9.2); the `FormatRaw` presence matrix; a lockdown group converges rather than looping |
| `applylink_test.go` | **New (A20).** The `--link`/`--unlink` state machine of §9.2, cell by cell: `--unlink` restores copy mode for an INI member and for a `FormatRaw` member; a plain apply over a repo link is left alone; a fully linked tree is **not** refused; a converged file behind a foreign symlink is not reported as a conflict (the §9.3 narrowing); `--dry-run` previews the refusal on `Plan.LinkConflicts`; `--link` with `--unlink` is refused with `*LinkUnlinkError` |
| `applyexec_test.go` | The **execution-half** contract (§6.7): the live file is re-merged at write time and the re-merge is silent when nothing changed; a file that became immutable in between is refused; the lock is held around the **whole** cycle, not just the rename; a failed backup aborts the write it protects **and does not abort other files** (§6.9); a per-file EXECUTION failure — an unwritable destination directory, or a destination that is not a regular file — is reported and does **not** abort the remaining files, asserted across a plan spanning more than one `ApplyOrder` phase with the failing file ordered **first**; the raw write path and its under-lock convergence recheck. Plus the invariants the original `execute_test.go` row named: dry-run short-circuits with zero filesystem effect (`Report.Applied == nil`, `Planned` full), atomic temp-and-rename in the same directory leaving no stray temp file, mode preservation on an existing `0644` file and `0600` on creation, `Chmod` beating the umask, and a non-nil `Report` returned alongside an error |
| `lock_test.go` | `<path>.lock` is created with the QLockFile payload and removed on success and on error; a held lock times out into `*LockError`; a stale lock with a dead pid is reclaimed exactly once; a fresh lock and a live pid are never evicted |
| `export_test.go` | `PlanExport`: strip-and-serialise, empty-after-strip → `ActionSkip` vs `ActionPrune`, prune refuses to leave `plasma/files/`, the raw byte path, ordering, the §7.4 advisory, and dry-run mutating nothing. Plus the §14.1 privacy scan: `*PrivacyLeakError` before any write, and the unexpanded `$HOME` allowance |
| `diff_test.go` | All **five** `DiffKind`s; neither live-only flavour makes `Drifted()` true; `DiffExpectedVolatile` is distinguished from `DiffMissingRepo`; `Diff` returns full results **and** `*DriftError`; an unreadable file is an error rather than an absence; `--recommended` changes the question, including for a recommended-only file; `Validate` produces byte-identical results to `Diff` for the same inputs (the "one comparator" invariant); `Status` |
| `payload_test.go` | The repo's **own committed payload** — the one place in the package that reads outside a temp dir: every path in `plasma/files/` **and** `plasma/recommended/` is a `DefaultFiles()` path and passes `checkSpec`; every INI parses, names at least one entry, and is **byte-canonical under `Write`** (so the next export does not rewrite an undrifted machine); the §14.1 privacy scan is re-run over what is **already committed**, not only over what an export is about to write; and `plasma/files/` survives its own strip unchanged, since export generated it |
| `plasma_test.go` | `Support` for each `platform.OS`; `DetectSession`'s three states across the `KDE_FULL_SESSION` / `XDG_CURRENT_DESKTOP` matrix, including a colon-separated `XDG_CURRENT_DESKTOP=KDE:plasma` and a case-insensitive match; `ActionKind.String()`; every typed error's message; `TestNoOverlapWithSymlinkTable` (§13.2) |
| `reload_test.go` | `ReloadPlan` derives the right commands from the modified-file set, deduplicates, and orders them; `Reload` is a no-op without `Options.Reload` or a live session; `qdbus6`/`qdbus` probing order and the skip-with-warning path; every command goes through the fake Runner (asserted via `CallsSnapshot`) and never through a shell |
| `helpers_test.go` | `t.Helper()` fixtures only: `writeFile`, `seedRepo`, `seedLive`, `mustParse`, `mustWrite`, `planFileFor(p, path) (FileAction, bool)`, `countKind(p, kind) int` |
| `scrub_guard_test.go` | **New (A13).** `TestPlasmaTree_NoMachineIdentifiers` — the shape-based half of the §14.4 pair. Walks every file under `internal/plasma/` and `plasma/`, source, tests and testdata alike, and fails on a UUID that is not `00000000-0000-4000-8000-…`, an absolute home path naming a non-placeholder account, an IPv4 address outside the RFC 5737 documentation blocks, a MAC address, an email address outside the RFC 2606 reserved domains, or an uppercase hex run of 16+ that is not the `DEADBEEF…` token. It locates the repository root by walking up to `go.mod`, since its scope is wider than its own package. Also home to the `syntheticUUIDPrefix` / `syntheticToken` constants and to `scrubExceptions`, the deliberately tiny per-file table of literals a test is allowed to spell because it is the needle it hunts for |

### 13.2 Cross-package invariants

Both live in `internal/plasma`:

- `TestNoOverlapWithSymlinkTable` (`plasma_test.go`, imports `internal/symlink`)
  — no `DefaultFiles()` path, resolved to a `$HOME`-relative destination,
  collides with any `symlink.DefaultTable()` `Dest`. This is the §3.1 ownership
  guarantee.
- `TestCommittedPayload_IsManaged` (`payload_test.go`) — every file present in
  the repo's `plasma/recommended/` **and** `plasma/files/` trees is a
  `DefaultFiles()` path. This is the §3.2 invariant. It reads the repo trees via
  a `DOTFILES_ROOT`-independent relative path (`../../plasma/…`).

**Amended (A10) — the name changed and the skip is gone.**
`TestRecommendedFilesAreManaged` was never implemented under that name; its
contract is carried by `TestCommittedPayload_IsManaged`, extended to cover
`plasma/files/` as well. More importantly, **the original allowance that it
"skips cleanly if the directory does not exist yet" is dropped.** Both trees are
committed, so an absent or empty tree is a **failure**, not a skip: a guard that
skips itself when its input vanishes is a guard that silently stops guarding,
which is precisely the failure mode that let the committed payload go untested
for the life of the branch. The shipped test `t.Fatalf`s on an empty tree with
"the guard would pass vacuously".

### 13.3 `cmd/dotf`

| File | What it proves |
| --- | --- |
| `plasma_test.go` (new) | With `t.Setenv("DOTFILES_ROOT", t.TempDir())` and `t.Setenv("HOME", t.TempDir())`: each verb's `--dry-run` returns 0 with empty stderr and a `dry-run=true\n` stdout prefix and leaves `$HOME` untouched (`len(os.ReadDir(home)) == 0`); `--bogus` exits 2 with empty stdout; a missing repo tree exits 1 with a `dotf:`-prefixed stderr line; `plasma` with no verb, two verbs, and an unknown verb each exit 2 with the exact message; `--link` on `diff` and `--reload` on `status` exit 2; `--only kwinrcx` exits 2; the loop-parse makes `plasma export --dry-run` and `plasma --dry-run export` produce identical output; non-interactive `apply` without `--yes` exits 1. **Amended (A7):** plus `TestPlasmaDiffLines_OneEntryPerLine`, `TestRunPlasma_DiffRendersOneLinePerDriftedKey`, `TestPlasmaDiffReports_PinsDiffKindLabels` and `TestRunPlasma_DiffJSONCarriesKindLabel`, which pin the one-line-per-entry shape and all five `DiffKind` labels at both the Go and JSON levels |
| `plasmalink_test.go` (new, A20) | The CLI half of `--unlink`: a `--link` → `--unlink` round trip through `runPlasma` leaves a real file again; `--unlink` on a non-`apply` verb exits 2; `apply --dry-run` prints the `plasma.link-conflict=` / `plasma.link-conflict.refusal=` lines and carries `linkConflicts` in the `--json` document; `status` labels both-absent files; the `--json` failure envelope is emitted on a pre-flight failure and **no** stdout is written without `--json` |
| `plasmadiag_test.go` (new, A16/A17) | The reporting contracts of §8.3–§8.5 end to end: a per-file diagnostic reaches both the `--json` `failures` array and the stderr lines; a refused file is still rendered in the text report; post-apply drift is not counted as a second failure; reload commands print **resolved**; the session advisory is export-shaped on `export` and emitted once per file on `apply` |
| `plasma_signal_test.go` (new, `//go:build unix`) | Copies `sysconfig_signal_test.go`: a real `syscall.Kill(syscall.Getpid(), syscall.SIGINT)` synchronised by a `notifyingWriter` proves the confirm prompt yields `dotf: aborted` and exit 130, and that the handler is unregistered afterwards |
| `main_test.go` | `TestRun_Plasma` beside `TestRun_Link`, using `captureOutput` (whose concurrent pipe drain must not be restructured), asserting code 0, empty stderr, `^dry-run=true\n` |
| `help_test.go` | The three list updates of §8.8 item 3. `TestHelpRegistry_CoversDispatch`, `TestRun_Help_Command_All`, `TestRun_CommandDashDashHelp_All`, `TestRun_CommandDashH_All`, and `TestCommandHelp_DRY_NoDrift` then exercise plasma for free. A new `TestSetPlasmaFlags_Parses` asserts each field lands |
| `completion_test.go` | `TestGeneratedScripts_PlasmaPositional`: bash contains a `plasma)` arm and the verb list; zsh contains `1:verb:(export apply diff status)`; fish contains `__fish_seen_subcommand_from plasma` and `-a '…'`, **and** the pre-existing `completion`/`help`/`setup` fish rules now exclude `plasma`. **Amended (A10):** plus three **behavioural** tests that actually execute the generated scripts rather than only syntax-checking them — `TestGeneratedFish_FlagRulesDoNotCrossFire`, `TestGeneratedFish_PlasmaVerbFlagsInRealFish`, and `TestGeneratedBash_PositionalScanSkipsFlagValues` (§8.8.1). `TestGeneratedScripts_ShellSyntax` still runs `bash -n` / `zsh -n` / `fish --no-execute` — note it **skips silently** when a shell is absent, so verify locally on a host with all three |
| `applyopts_test.go` | The `PLASMA` row in the env-parity table: flag-only, env-only, both, and `PLASMA=1`/`PLASMA=TRUE` correctly **not** truthy |
| `apply_test.go` | `--plasma` off by default; the phase records exactly one collector step on success, on plan error, on execute error, and on validate error; requested-on-non-Linux warns to stderr and records nothing; `applyDryRun` composes plasma bucket lines through the shared formatter |
| `doctor_test.go` | The plasma doctor checks of §13.4 |

### 13.4 Doctor integration

`dotf doctor` gains two plasma checks, using the engine read-only with
`plasma.Options{Validate: true}` and every other field false:

- `check.PLASMA.DRIFT` — `warn` when `Diff` reports drifted files, listing them
  as `doctorItem{Kind: "plasma", Action: "drift", Dest: path}`, remedy
  "run `dotf plasma apply`".
- `check.PLASMA.REPO` — `warn` when `plasma/files/` does not exist, remedy
  "run `dotf plasma export`".

Both are skipped entirely (not `ok`, not `warn`) when
`plasma.Support(info) == SupportUnsupported`, matching how `checkSysconfig`
handles an unsupported OS. Neither ever returns `fail`: plasma drift is never a
broken host.

### 13.5 Commands to run before calling this done

```bash
go build -buildvcs=false ./...   # -buildvcs=false is mandatory inside a worktree
go vet ./...
go test -race ./...
go test -race -count=5 ./internal/plasma/...
golangci-lint run ./...
npm run lint
```

---

## 14. Security and privacy

### 14.1 The export privacy scan

`PlanExport` serialises every candidate into `FileAction.Content` _before_
`Execute` runs, so the scan happens at plan time and a violation aborts the run
with nothing written.

The scan rejects, per file, any occurrence in the serialised bytes of:

- `/home/<name>/` or `/Users/<name>/` where `<name>` is any path segment
- `file:///home/` or `file:///Users/`
- the **expanded** home directory path (`configDir`'s parent, e.g.
  `/home/yandy`) appearing anywhere as a literal

The scan deliberately does **not** reject the six-character literal `$HOME`.
That string is an unexpanded KConfig `[$e]` reference and is exactly the
portable form the spec wants — it is what the recommended `baloofilerc`
`folders[$e]=$HOME/` entry relies on.

The scan runs only over documents `PlanExport` is about to write into
`plasma/files/`. `plasma/recommended/` is hand-authored, never produced by
export, and therefore never scanned.

A hit returns `*PrivacyLeakError{Path, Line, Pattern}` and exits 1. The correct
fix is either a new strip rule or moving the setting into
`plasma/recommended/` re-authored with a `[$e]`-flagged `$HOME`-relative value —
which is exactly what the recommended `baloofilerc folders[$e]=$HOME/` entry
demonstrates.

Known absolute-home-path sources, all in files that are already excluded:
`spectaclerc` (`imageSaveLocation` / `videoSaveLocation` pointing into a personal
Dropbox tree), `QtProject.conf` (`lastVisited`, `shortcuts=file:///home/yandy`),
`krdpserverrc` (`Certificate` / `CertificateKey` under `~/.local/share/`).

### 14.2 The hard denylist

`Denylist()` is a fence, not a mere absence from `DefaultFiles()`. The engine
refuses to read, write, or traverse anything under these config-dir-relative
prefixes, and `TestNoDenylistedPathInDefaultFiles` proves the two never overlap:

```text
kdeconnect/           # privateKey.pem + certificate.pem — this device's identity
krdcrc                # private IPs, admin usernames, internal hostnames, password fields
krdpserverrc          # absolute paths to TLS key material
konsolesshconfig      # SSH hostnames, usernames, ports
bluedevilglobalrc     # Bluetooth adapter MAC
trashrc               # nfs:// export path + private IP
ktrashrc              # NAS trash mount path in the group header
ktimezonedrc          # owned by `dotf sysconfig`
kactivitymanagerdrc         # per-installation activity UUID
kactivitymanagerd-statsrc   # per-installation activity UUID
akonadi/              # PIM account state
emailidentities       # email addresses
emaildefaults
kwinoutputconfig.json # EDID hash, panel serial, output UUID
```

The engine never walks `~/.local/share/`, so `kwalletd/` and `keyrings/` are out
of reach structurally; they are listed in `plasma/README.md` as permanently
out of scope.

**Amended (A9) — `kactivitymanagerdrc` and `kactivitymanagerd-statsrc` are now
fenced, not merely unlisted.** §14.3 already classifies both as the
per-installation activity UUID, which is the same class of machine identity as
`kwinoutputconfig.json`'s EDID hash. Adding them to `Denylist()` makes that
classification **enforceable** — `TestStripTable_MustNotManage` asserts it —
rather than a fact that happens to hold because nobody has edited
`DefaultFiles()` yet.

**`kwalletrc` is deliberately managed, and this is not an oversight.** It is
recorded here because it looks like one at a glance and has already been
challenged once. The file that is managed holds wallet _policy_, not wallet
_contents_: on the audited host it is `[Wallet] First Use=false` alone, i.e.
**empty after strip**, so nothing is written to `plasma/files/kwalletrc` today
and the file does not exist in the repo. The actual wallet blobs live in
`~/.local/share/kwalletd/*.kwl`, which the engine cannot reach at all because it
never walks `~/.local/share/` (above). Everything security-relevant _inside_
`kwalletrc` — `[Auto Allow]`, `[Auto Deny]`, `[org.freedesktop.secrets]`,
`[Wallet] Default Wallet`, `[Wallet] First Use` — is already stripped by the
§7.3 rules, pinned by `TestStripTable_Kwalletrc`; only `[Wallet] Enabled` and
its neighbours survive. §14.3, the authoritative "what must never be committed"
table, deliberately does **not** list it.

If a future maintainer still wants it unmanaged, the argument to make is **blast
radius, not data leakage** — applying a repo `[Wallet] Enabled=false` to a fresh
machine silently disables the password wallet — and the change is a one-line
`DefaultFiles()` deletion plus the count 27→26 and the golden-list row.

### 14.3 What must never be committed, and why

Every value quoted below is the **synthetic** form of §14.4, not the value on any
real machine. What the table pins is _which shape_ of identifier lives in which
file; a placeholder of the same shape makes that point exactly as well, and a
document arguing that these identifiers must never be committed cannot itself be
the thing that commits them.

| Item | Where it lives | Why excluded |
| --- | --- | --- |
| KDE Connect private key + certificate | `kdeconnect/privateKey.pem`, `certificate.pem` | A private key inside the very tree the export walker traverses. Hard denylist so a future glob widening cannot commit it |
| RDP host preferences | `krdcrc` | An internal-network IP `198.51.100.87` (twice, once as `rdp://Administrator@…`), an internal hostname `win-example-mgmt`, and `gatewayPassword` / `proxyPassword` / `smartcardName` fields |
| RDP server TLS paths | `krdpserverrc` | `Certificate=/home/yandy/.local/share/krdpserver/krdp.crt` and the matching key |
| Bluetooth adapter MAC | `bluedevilglobalrc` | `[Adapters] AA:BB:CC:DD:EE:FF_powered=true` — the group header _is_ the adapter's MAC |
| NFS topology | `trashrc`, `ktrashrc` | `nfs://10.100.50.52//volume1/data` → `/mnt/nas02-data`, and a group header that _is_ the NAS trash path |
| Monitor identity | `kwinoutputconfig.json` | `edidHash` (a digest of the panel's EDID), `edidIdentifier` (a space-joined vendor code, model number, **panel serial** and manufacture week/year), output UUID, HDR calibration |
| Personal cloud paths | `spectaclerc`, `QtProject.conf` | Absolute Dropbox Screenshots/Recordings paths and a `lastVisited` into the same tree |
| Activity UUID | `kactivitymanagerdrc`, `kactivitymanagerd-statsrc` | `00000000-0000-4000-8000-000000000003` — generated per installation |
| App-minted per-install token | `kglobalshortcutsrc [com.anthropic.Claude]` | `DEADBEEFDEADBEEFDEADBEEFDEADBEEF-Ctrl+Alt+Space`. **Stripped**, not excluded — the group is removed on export because a transplanted entry would be a dead binding |
| Desktop and output UUIDs | `kwinrc [Desktops] Id_*`, `[Tiling][…][…]` | **Stripped** (§7.5) |
| Thunderbird per-machine handlers | `mimeapps.list` `*userapp-*.desktop*` | **Stripped** by the `VolatileValues` rule — they dangle anywhere else. The glob is unanchored on both ends because the values are semicolon-terminated lists (§7.3.1) |
| Qt palette twin | `Trolltech.conf` | 100% derived from the active color scheme; the Qt-side twin of `kdeglobals [Colors:*]` |
| Telemetry timestamp | `KDE/UserFeedback.conf` | `LastEncouragement=@DateTime(…)` |
| Migration ledgers | `kconf_updaterc`, `powermanagementprofilesrc`, `plasma-welcomerc`, `kate-externaltoolspluginrc`, `discoverrc` | Recording which migrations ran on _this_ install; copying them actively suppresses migrations the new machine needs |
| Panel/applet layout | `plasma-org.kde.plasma.desktop-appletsrc`, `plasmashellrc` | Installation-local integer instance IDs cross-referenced across three files, activity UUIDs, resolution-keyed geometries. Can only be applied wholesale, never key-merged (§16) |
| UI geometry | `systemsettingsrc`, `khelpcenterrc` | Column widths, splitter positions |
| Non-desktop app state | `kdenliverc` (46 KB of project/render/MLT state with recent-file paths), `elisarc`, `kde.org/elisa.conf`, `kalarmrc` | Out of scope for a Plasma desktop sync |
| Empty / superseded | `plasmaparc` (zero bytes), `kwinrulesrc` (`rules=` empty; rule bodies are UUID-keyed and installation-local) | Nothing to carry |
| Session ledgers | `session/`, `ksmserverrc [Session: *]` | Per-logout state; the `ksmserverrc` groups are **stripped**, the file itself is managed |

### 14.4 Scrubbing, and the guard that enforces it

The Family C fixtures (§4.8) are copies of real files and are **committed**.
Every identifier in them is replaced by a fixed synthetic constant before
commit: UUIDs become `00000000-0000-4000-8000-0000000000NN`, the per-install app
token becomes a repeating `DEADBEEF…` string, and MAC addresses, IPs, hostnames,
and absolute home paths are removed entirely (they only appear in excluded files,
so no fixture needs them). The scrubbing preserves _structure_ — bracket nesting,
escape sequences, ordering — which is all the tests actually exercise.

**The same constants are the rule everywhere, not just in the fixtures.** Test
tables, doc examples and this spec all use them. That is not fastidiousness: an
identifier is no less committed for appearing in a table-driven test case or a
prose example than in a fixture, and a synthetic value of the same shape teaches
the reader and exercises the code identically.

Two tests enforce it, and the pair is deliberate:

| Test | Scope | Catches |
| --- | --- | --- |
| `TestCodec_FamilyC_IsScrubbed` | `internal/plasma/testdata/kconfig/real` | The **literal** names that occur in this corpus — a home directory, a NAS host, peripheral vendors — which no shape-based scan can infer |
| `TestPlasmaTree_NoMachineIdentifiers` | `internal/plasma/` and `plasma/`, every file including tests and testdata | The **shapes**: non-synthetic UUIDs, absolute home paths, IPv4 and MAC addresses, email addresses, and long uppercase hex tokens |

The second exists because the first was not enough: scoped to the fixture
directory, it let three live UUIDs reach `table_glob_test.go`, a file the
scrubber never touches. Its scope stops at the two directories this feature
owns — `main` already carries absolute home paths, a NAS host name and a private
address in unrelated, already-published files, so a repository-wide scan would
fail on day one and be silenced by the end of the week.

### 14.5 Delivery checklist — the repo doc surfaces a new subcommand must update

**Amended (A9), new section.** The branch shipped `dotf plasma` and
`dotf apply --plasma` while `README.md` and `CLAUDE.md` gained **zero** mentions
of either. That is not an author's oversight; it is a structural gap — no
section of this spec, and no rule in the repo, enumerated the doc surfaces a new
subcommand has to touch, so there was nothing to fail against. The repo already
has a standing rule that every CLI-surface change must regenerate shell
completions; this is its documentation twin.

Every new `dotf` subcommand, and every new `dotf apply` flag/env twin, must
update **all** of:

| Surface | What it is | What to add |
| --- | --- | --- |
| `README.md` — per-engine section list | The `### dotf link` / `### dotf sysconfig` … run of sections | A `### dotf <name>` section in registry order |
| `README.md` — repository tree | The annotated directory listing | The new `internal/<pkg>/` and any new repo data tree (here, `plasma/`) |
| `CLAUDE.md` — Stack Overview **prose** | The paragraph that enumerates the subcommands inline | The new verb, in switch order |
| `CLAUDE.md` — Stack Overview **table** | The `Go (provisioning)` row's subcommand list | The same verb — this is a **second, separate** list in the same file and is the one most often missed |
| `docs/README.md` | The docs index | A pointer if the command gets reference docs |
| `AGENTS.md` | The subcommand enumeration | The new verb |

Deliberately **not** on the list: `docs/reference/`. Its four files
(`aliases.md`, `environment-variables.md`, `functions.md`,
`manifest-backends.md`) document shell aliases, shell functions, and manifest
backends — none of them enumerates the CLI surface, the repository layout, or the
`dotf apply` env twins, so there is no existing list for a new command to join.
Adding one would mean inventing a doc section rather than extending one. If the
repo later wants a canonical `docs/reference/dotf-cli.md`, that is a separate,
deliberate piece of work.

---

## 15. Risks and mitigations

| # | Risk | Severity | Mitigation |
| --- | --- | --- | --- |
| 1 | **The strip table deletes a real setting.** A too-broad glob silently removes user intent on the next export | High | Governance identical to `symlink.DefaultTable()`: one static exported function, a `GOVERNED:` doc comment, an inline Go golden test, and count assertions (§7.6). **Amended (A8):** plus the **per-glob contract test** (`internal/plasma/table_glob_test.go`), which is the mitigation that actually bites — the golden test pins _which_ files are managed and pinned nothing about what each glob may delete, which is why all four §7.3.1 defects shipped past it. Plus the §12.13 recommended-key invariant, `--dry-run` showing the exact bytes, and `--backup` preserving the pre-export live file |
| 2 | **A codec bug corrupts a live config.** A wrong escape or a wrong comparator makes KDE read garbage | High | Byte-identical round-trip against `kwriteconfig6`-produced fixtures (§4.8 Family A) is the acceptance bar, plus fuzzing. Writes are atomic temp-and-rename (§6.7), so a crash never truncates. `--backup` is the recovery path |
| 3 | **Racing a running Plasma session.** KDE rewrites these files on nearly every settings change | **High** (was Medium) | **Amended (A1):** the whole read-modify-write cycle runs inside `<path>.lock`, with KConfig's own QLockFile format and 45 s timeout (§6.7). Severity was raised because the original design — merge at plan time, lock the rename — made this a _certainty_ across the confirmation prompt rather than a race, and it destroyed live keys silently. Plus the `SessionOwned` stderr advisory (§8.4), and `--link` as the alternative for session-owned files, with its own hazard documented |
| 4 | **`--link` commits volatile state to git.** KDE writes through the symlink into the worktree | Medium | Opt-in per run, never persisted in `DefaultFiles()`; mode conflicts refused before any mutation (§9.2); `export` on a linked file re-strips it with an explicit `Reason`; `plasma/README.md` states the tradeoff |
| 5 | **Group-order comparator wrong.** Using `sort.Strings` (UTF-8) for group names reorders any file with non-BMP group names, and every `diff` afterwards shows phantom drift | Medium | The comparator split is spelled out in §4.4 and locked by `codec_order_test.go` with the `U+1F600`/`U+FFFD` pair proving the two orders are opposite. And `diff` compares parsed models, not bytes, so even a comparator regression cannot manufacture drift |
| 6 | **File-level `[$i]` silently overwritten.** A deliberately locked-down kiosk config would be clobbered by a writer that ignores the marker | Medium | `*ImmutableError` refuses the write, recorded per file, run continues, exit 1 (§6.2, §5.8) |
| 7 | **Locale variants destroyed.** KDE itself deletes `Name[de]` when the process locale is `de_DE` | Medium | The native codec is locale-agnostic: the tag is part of the entry identity, preserved verbatim, never re-stamped (§4.4 divergence (c)). This is the single strongest reason the feature does not shell out to `kwriteconfig6` |
| 8 | **A colour-scheme strip loses a hand-rolled palette** | Medium | The §7.4 guard: the strip is conditional on a matching `.colors` file existing, with a stderr advisory when it does not |
| 9 | **`mimeapps.list` double-ownership.** It is XDG, not KDE-exclusive, and could be claimed by `dotf link` later | Low | `TestNoOverlapWithSymlinkTable` (§13.2) fails the build the moment both claim a path. No overlap exists today |
| 10 | **`plasma-localerc` overlaps `dotf sysconfig`'s locale step** | Low | Different mechanism and different scope (per-user Plasma formats vs system `localectl`), so no file-level conflict. `ktimezonedrc`, whose overlap _is_ real, is excluded for exactly that reason. `plasma/README.md` states both |
| 11 | **`--reload` disrupts a working session** | Low | Opt-in; without it the exact commands are printed instead (§10.2). `--reload` is rejected on non-`apply` verbs, and gated on a live session |
| 12 | **Completions silently drift.** A positional-taking command is special-cased by name in three generators | Low | **Amended (A10):** the special-casing is gone — both disambiguation problems are derived from one `positionalCommands` model (§8.8.1), so a new verb or module is handled the moment it is added. `TestGeneratedScripts_PlasmaPositional` catches a structural omission and the three behavioural tests execute real `fish` and `bash`. Note `TestGeneratedScripts_ShellSyntax` skips silently when a shell is absent — verify on a host with bash, zsh, and fish |
| 13 | **A committed fixture, a committed test, or the committed payload leaks private data** | Low | §14.4 scrubbing plus `codec_fixture_test.go`'s `TestCodec_FamilyC_IsScrubbed` over `testdata/`, **and (amended, A10)** `payload_test.go`'s `TestCommittedPayload_CarriesNoHomePath`, which re-runs the §14.1 scan over what is already committed in `plasma/files/` and `plasma/recommended/` rather than only over what an export is about to write. **Amended (A13):** the fixture-scoped guard was the whole exposure — it did not cover the branch's own test tables, which is how three live UUIDs shipped in `table_glob_test.go`. `scrub_guard_test.go`'s `TestPlasmaTree_NoMachineIdentifiers` now scans every file under `internal/plasma/` and `plasma/` for identifier _shapes_ (§14.4) |
| 14 | **The feature is larger than its payoff.** The audit found the live config far sparser than expected — no `[Effect-*]`, no `[Compositing]`, no custom Konsole profiles or color schemes | Low | Accepted and scoped accordingly: the genuinely hand-authored surface is `kglobalshortcutsrc` (by far the largest real payload), `kwinrc [Desktops] Number=10`, and the `kdedefaults/` theme overlay, with `kiorc`/`kded5rc`/`kscreenlockerrc`/`powerdevilrc`/`kcminputrc` as small deliberate tweaks. The merge engine's value is concentrated in `kglobalshortcutsrc`, and the `recommended/` overlay is where most of the _new_ value lives |

---

## 16. Out of scope and future work

### 16.1 Deliberately deferred

- **Panel and applet layout.** `plasma-org.kde.plasma.desktop-appletsrc` plus
  `plasmashellrc` can only be applied wholesale — key-merging into a live shell
  with different containment instance IDs corrupts plasmashell. If the user wants
  it, it ships later as a documented "replace both files, then restart
  plasmashell" path under `plasma/recommended/`, not as a `DefaultFiles()` entry
  and never through the merge engine.
- **Window rules.** `kwinrulesrc` is empty today. Plasma 6 stores each rule body
  in a UUID-named group referenced from `rules=`; the set is internally
  consistent but installation-local. Revisit only when rules actually exist, and
  then ship the whole file as a unit.
- **Konsole profiles.** There are zero custom profiles under
  `~/.local/share/konsole`. When one exists, `konsolerc [Desktop Entry]
  DefaultProfile` plus a shipped `.profile` file becomes worth adding — which
  requires the engine to learn about `~/.local/share/`, currently out of reach by
  design (§14.2).
- **Re-authored Spectacle save locations.** `spectaclerc` is excluded because its
  only content is absolute personal cloud paths. A `plasma/recommended/spectaclerc`
  with `$HOME`- or XDG-relative values is a natural later addition, using the
  same `[$e]` technique as the recommended `baloofilerc` entries.
- **Custom colour schemes.** Nothing exists under
  `~/.local/share/color-schemes`. If the user authors one, the §7.4 guard already
  detects it and keeps `[Colors:*]`; shipping the `.colors` file itself is a
  later, separate decision.
- **KWin compositing and effect tuning.** `kwinrc` currently has no
  `[Compositing]` or `[Effect-*]` groups at all. The recommended overlay already
  carries the highest-value effect toggles; deeper tuning waits for the user to
  actually set something.

### 16.2 Not planned

- Symlinking as the default mode, or a persistent per-file link mode.
- Deleting live keys that the repo does not name (§6.4).
- Shelling out to `kwriteconfig6` / `kreadconfig6` for any INI operation.
- A `manifest/packages.toml` entry of any kind. Plasma is configuration, not
  provisioning; the custom-script ratchet baseline is untouched.
- macOS or Windows support.
- Migrating Hyprland into or out of this tree. The two desktops stay on separate
  file sets and separate flags, permanently.

---

## Appendix A — implementation order

Suggested PR sequence, each independently reviewable and green.

1. **Codec.** `codec.go` + `codec*_test.go` + `testdata/kconfig/`. No CLI
   surface. Acceptance: Family A byte-identity passes.
2. **Table and strip.** `table.go`, `strip.go`, their tests, and the golden.
   Acceptance: `DefaultFiles()` locked at 27, strip rules proven against the
   scrubbed corpus.
3. **Engine.** `plasma.go`, `errors.go`, `export.go`, `apply.go`,
   `applyexec.go`, `lock.go`, `merge.go`, `diff.go`, `validate.go`, `reload.go`
   and tests (§5). Acceptance: the full §13.1 matrix, and specifically that the
   merge runs inside the lock at write time (§6.7).
4. **CLI.** `cmd/dotf/plasma.go` (plus `plasmaverbs.go` / `plasmaprint.go`), the
   §8.8 registration checklist items 1–5 including the §8.8.1 completion
   contract, and `cmd/dotf/plasma_test.go` + `plasma_signal_test.go`.
   Acceptance: the help, completion, and dispatch drift guards pass.
5. **`dotf apply --plasma`.** The five §11.1 edits plus doctor integration.
6. **Data and prose.** Run `dotf plasma export` for real, commit
   `plasma/files/`, hand-author `plasma/recommended/` from §12, write
   `plasma/README.md` and `plasma/recommended/README.md`, and update every doc
   surface in **§14.5** — which supersedes §8.8 item 6's shorter list.

Commit convention: `feat(plasma): …` for steps 1–5, `chore(plasma): …` for the
captured snapshot in step 6, and `docs(internal): …` for this specification.
