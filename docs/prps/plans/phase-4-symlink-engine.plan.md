# Plan: Native Go Symlink Engine + `dotf link` (Phase 4)

## Summary

Build a native Go symlink engine (`internal/symlink`) that reproduces the exact
behavior of `ansible-playbook playbooks/symlinks.yml` — link creation, backup-if-not-
symlink, dry-run, post-run validation, broken-link handling, and the Zed copy exception —
and wire it to a new `dotf link` subcommand (`cmd/dotf/link.go`). The link table is
transcribed into **typed, static Go data** (no runtime YAML dependency), OS-faceted via
`internal/platform`. Because every managed destination lives under `$HOME`, the engine uses
pure `os.*` calls (`os.Symlink`/`os.Lstat`/`os.Readlink`/`os.Stat`) and needs **no
privilege escalation** — unlike the Phase 3 install engine. This retires the Ansible
dependency for symlink provisioning; the playbook is kept unexecuted for one release as a
rollback path.

## User Story

As the dotfiles maintainer provisioning a fresh machine across Arch/CachyOS, macOS,
Debian/Ubuntu, and WSL, I want `dotf link` to create the identical set of symlinks the
Ansible playbook creates — with the same safety semantics (backup real files, skip when
the source is missing, never clobber a symlink, guard the Linux-desktop rollout) — so that
provisioning no longer requires Ansible installed, symlink behavior is unit-tested against
`t.TempDir()`, and I can preview every change with `--dry-run` before touching `$HOME`.

## Problem → Solution

Symlinks are provisioned by a 16-file Ansible playbook (`playbooks/symlinks.yml` +
`playbooks/tasks/*.yml` + `playbooks/vars/symlinks.yml`) that hard-requires
`ansible-playbook` on the host, has no unit tests, and forks a Python interpreter to move a
handful of files under `$HOME`. → A dependency-free `internal/symlink` engine driven by a
static Go link table (ported 1:1 from `playbooks/vars/symlinks.yml`), a `Plan`/`Apply`/
`Validate` split exercised entirely through hermetic `t.TempDir()` filesystem tests, and a
`dotf link` command mirroring the existing `run<Cmd>` seam. Ansible drops from a hard
runtime requirement to a retained-but-dormant rollback artifact.

## Metadata

- **Complexity**: Medium-Large
- **Source issue**: Forgejo #10 (Phase 4 — native symlink engine)
- **Source PRD**: `docs/prps/prds/provisioning-cli.prd.md`
- **Research notes (ground truth)**: `docs/prps/plans/.prp-research/phase-4-5-symlink-sysconfig/`
  (`symlink-semantics.md`, `go-cli-patterns.md`, `test-ci-conventions.md`,
  `sysconfig-privilege.md` — the last is Phase 5 context, read for the shared `exec.Runner`/
  platform conventions only)
- **Estimated files**: ~12 (7 CREATE, 5 UPDATE)

---

## Goal & Scope

### In scope

1. `internal/symlink` package: static link table + OS-faceting, and a `Plan`/`Apply`/
   `Validate` engine implementing every edge-case behavior catalogued in
   `symlink-semantics.md` §4, §8, §9, §10.
2. `dotf link` subcommand (`cmd/dotf/link.go`) wired into `cmd/dotf/main.go`, with flags
   `--dry-run`, `--backup`, `--force`, `--no-validate`, `--linux-desktop`, writing to
   injected `stdout`/`stderr`.
3. Table-driven tests (`t.TempDir()`) covering every acceptance-criterion edge case.
4. Establish `dotf link` as the native symlink-provisioning capability (Ansible not required
   by it) and mark the Ansible playbook **RETIRED** (retained for rollback). **`bootstrap.sh`
   is NOT cut over in this phase** — the flow-level bootstrap→binary cutover is Phase 7
   (parallel-then-cutover; consistent with Phase 3, which left `bootstrap.sh` untouched and
   still shells to `bin/install-*.sh`).
5. Documentation updates: mark the playbook retired (banner + note; files kept), update
   `README.md`, `CLAUDE.md`, `AGENTS.md`, `playbooks/README.md`, `tasks/todo.md`.

### Non-goals (explicit)

- **Do NOT delete the Ansible playbook this release.** `playbooks/symlinks.yml`,
  `playbooks/vars/symlinks.yml`, and `playbooks/tasks/*.yml` are retained unexecuted for one
  release as a rollback path (acceptance criterion #4). Deletion is a follow-up once
  `dotf link` has soaked.
- **Do NOT modify `bootstrap.sh` or `bin/install-*.sh`.** Parallel-then-cutover: the bash
  bootstrap stays authoritative and untouched until the Phase 7 shim cutover (Phase 3 set the
  precedent — `bootstrap.sh` has zero `dotf` references). AC #3 ("Ansible no longer required
  to provision symlinks") is satisfied at the **capability** level — `dotf link` provisions
  the full link set with no Ansible dependency and the playbook is retired — not by rewiring
  `bootstrap.sh`. A `git diff` guard verifies bash is untouched.
- **Do NOT migrate the manifest to be the symlink source of truth.** The 4 partial
  `[[symlink]]` example entries in `manifest/packages.toml` and the `manifest.Symlink` type
  stay as-is; `dotf link` does **not** read them (see Data Model). Consolidating the two into
  one source is a later phase.
- **Do NOT add privilege/sudo wiring.** Symlinks live under `$HOME`; the engine is pure
  `os.*`. No `internal/privilege` usage, no `Prime`/`KeepAlive`, no password prompt.
- **Do NOT add `--headless`/`--yes`/`--force` parity flags beyond those listed**, and no
  interactive TUI. Headless/flag-parity is Phase 6 (per `go-cli-patterns.md` §3).
- **Do NOT implement link *removal*/reconciliation.** Like the Phase 3 drift model, this is
  additive; there is no "unlink everything" mode (the playbook has none either).
- **Do NOT reimplement the cosmetic Ansible handlers** (`shell config changed`,
  `hyprctl reload`) as hard behavior. Optional post-hook, or omit (`symlink-semantics.md`
  §3.4).

---

## Context: files to read / mirror

### Ground-truth research (read first, in full)

| File | Why |
| --- | --- |
| `docs/prps/plans/.prp-research/phase-4-5-symlink-sysconfig/symlink-semantics.md` | The 1:1 behavioral contract. §2 = full entry table; §4 = edge-case matrix; §6 = required dirs; §7 = OS faceting; §8 = backup; §9 = dry-run; §10 = Zed; §12 = port checklist. **Authoritative.** |
| `docs/prps/plans/.prp-research/phase-4-5-symlink-sysconfig/go-cli-patterns.md` | `run<Cmd>` seam (§2), flag boilerplate (§3), `reportErr`/exit codes (§7,§9), `main.go` registration diff (§10), engine-as-struct-with-`t.TempDir()` recommendation (§6), command-name rationale (§15). **Correction it flags: the run-func interface is `exec.Runner`, not `exec.Execer`.** |
| `docs/prps/plans/.prp-research/phase-4-5-symlink-sysconfig/test-ci-conventions.md` | Validation gate (§1), `.golangci.yml` constraints (§2), FS-test patterns (§3.2), the exact local commands, lefthook gates, `revive` doc-comment requirement. |

### Source of truth to transcribe (the link table)

- `playbooks/vars/symlinks.yml` — the `{src,dest,type}` group lists + config vars. **This
  is what the Go static table must reproduce entry-for-entry.**
- `playbooks/symlinks.yml` — orchestration order (`pre_tasks`/`import_tasks`/`post_tasks`).
- `playbooks/tasks/{backup,unified-cleanup,zed}.yml` — the three non-trivial semantics
  (backup scope, real-file cleanup, Zed copy).

### Go patterns to mirror (do not reinvent)

| Pattern | Anchor | Reuse |
| --- | --- | --- |
| `run<Cmd>(args, stdout, stderr, r exec.Runner) int` seam | `cmd/dotf/install.go:32`, `plan.go:22`, `status.go:21` | `runLink` copies this signature exactly. |
| Flag block (`flag.NewFlagSet(name, ContinueOnError)` + `SetOutput(stderr)` + parse→`return 2`) | `install.go:33-45` | `link.go` opens identically. |
| `--dry-run` bool flag, gating real mutation behind `!dryRun` | `install.go:40,55` | `link` mutates the host → needs `--dry-run`. |
| `reportErr(stderr, err) int` (progName-prefixed, returns 1) | `plan.go:66` | shared in `package main`; `link.go` calls it directly. |
| Greppable `key=value` stdout, single checked write of a joined slice; `dry-run=true` first line | `install.go:163-181`, `plan.go:88-103` | `link` dry-run/report output style. |
| `resolveRepoRoot()` / `findRepoRootFromCWD()` (`DOTFILES_ROOT` env or CWD walk for `manifest/packages.toml`) | `install.go:136-159` | `link.go` resolves the dotfiles source root the same way. |
| Engine-as-struct with `repoRoot`/`home`, tested against `t.TempDir()` (no FS-mock interface) | `internal/backend/custom_script.go` + `custom_script_test.go` (per `go-cli-patterns.md` §6) | `symlink.Engine` takes `repoRoot`/`home` constructor params. |
| Typed domain errors implementing `Error()`; aggregate via `errors.Join` | `internal/backend/backend.go:25`, `internal/manifest/validate.go:96` | `symlink.LinuxDesktopGuardError`, `MissingSourceTreeError`, `UnsafeDestError`, validation aggregation. |
| Package doc comment starting with package name; every exported symbol doc-commented (`revive`) | `internal/backend/backend.go:1`, `internal/platform/types.go:1` | `internal/symlink` package + all exports. |
| `main.go` two-site registration (`usage` const + dispatch `switch`) | `main.go:19,39-57` | add `link` case. |
| Dest path-safety (reject abs / `..`) | `manifest/validate.go:186-208` | engine re-validates each `Dest` before `filepath.Join(home, dest)`. |

### Privilege note (important)

Symlinks under `$HOME` need **NO privilege**. `runLink` does not construct
`privilege.NewSudo()`, never calls `Prime`/`KeepAlive`, and the engine issues no `sudo`.
`exec.Runner` is passed to `runLink` **only** to feed `platform.Detect(r)`; all filesystem
work is direct `os.*`. (Contrast Phase 3 `install.go:62-69` and Phase 5 `sysconfig`, which
both escalate — see `sysconfig-privilege.md`. `link` is the per-user, unprivileged case per
`go-cli-patterns.md` §11.)

---

## Data Model

### Decision: static typed Go table, ported from `playbooks/vars/symlinks.yml`

The link table is modeled as an **ordered `[]LinkSpec` returned by `DefaultTable()`** in
`internal/symlink/table.go`, transcribed 1:1 from `playbooks/vars/symlinks.yml` in the §2
concatenation order. It is **not** driven by `manifest.Symlink`/`m.Symlinks`.

**Justification** (this overrides the `go-cli-patterns.md` §4/§14 assumption that `link`
consumes `m.Symlinks`, which was written before confirming the manifest is only a partial
mirror):

1. **Acceptance criterion #1 demands an identical set to the playbook.** The playbook
   defines **40 managed entries + 1 Zed copy** (`symlink-semantics.md` §2). The manifest's
   `[[symlink]]` block currently holds **only 4 example entries** (`manifest/packages.toml`
   lines 102-125). Consuming `m.Symlinks` would produce a 4-link set, failing criterion #1.
2. **`manifest.Symlink` cannot express the required semantics.** It has only
   `{Src,Dest,Type,OS}`. It has no way to encode: backup scope (only the 14 shell/common/
   prompt entries are backup-eligible, §8), the Zed copy-not-symlink exception (§10), the
   `manage_linux_desktop` opt-in + safety gate (§7.2), or the required-directory list (§6).
   Extending the manifest schema to carry all of this — under `DisallowUnknownFields` — is a
   larger, riskier change than a static Go table and is out of scope (non-goal).
3. **No new runtime dependency.** Reading `playbooks/vars/symlinks.yml` at runtime would add
   a YAML parser to the production dependency set for data that never changes between builds.
   A static Go slice is compiled in, zero-dep, and directly unit-testable.
4. **Precedent.** This mirrors how `internal/platform` encodes the shell's detection
   vocabulary as typed Go consts rather than sourcing `common/platform.sh` at runtime.

**Maintenance note (load-bearing):** until `playbooks/vars/symlinks.yml` is deleted (a
future release), `DefaultTable()` and the playbook vars are **two copies of the same data
and MUST stay in sync**. Guard this with:
- A prominent `// SYNC: keep in lockstep with playbooks/vars/symlinks.yml until the playbook
  is deleted (Phase 4 rollback window).` comment atop `table.go`.
- **A parity test** (Task 8) — see Test Strategy criterion #1 — that mechanically proves the
  Go table equals the playbook's entries, so drift fails CI rather than silently diverging.

### Types (public API of `internal/symlink`)

```go
// Facet gates a LinkSpec to the OS families / flags on which it applies,
// mirroring the per-group `when:` conditions in playbooks/symlinks.yml.
type Facet int

const (
    FacetAll          Facet = iota // bash/zsh/common/prompt/app/utility/completions/special
    FacetMacOS                     // macos_configs   — os_family == Darwin
    FacetArch                      // archlinux_configs — os_family == Archlinux
    FacetWindows                   // windows_configs — os_family == Windows (empty today)
    FacetLinuxDesktop              // generic_linux_desktop_configs + launcher_configs:
                                   //   is_linux_family && manage_linux_desktop
)

// LinkSpec is one managed dotfiles-to-$HOME mapping, ported from
// playbooks/vars/symlinks.yml. Src is repo-relative; Dest is $HOME-relative.
type LinkSpec struct {
    Src            string // e.g. "bash/bashrc"        (relative to repoRoot)
    Dest           string // e.g. ".bashrc"            (relative to home)
    IsDir          bool   // type: directory — DESCRIPTIVE ONLY; does not change link behavior (§5.1)
    Copy           bool   // Zed exception: copy+overwrite (0644), not symlink (§10)
    Facet          Facet  // OS/flag gate (§7.1)
    BackupEligible bool   // true only for the 14 shell/common/prompt file entries (§8)
}

// Options are the CLI knobs, mapped 1:1 from playbooks/vars/symlinks.yml config vars.
type Options struct {
    DryRun             bool // dry_run           — no filesystem mutations (§9)
    Backup             bool // create_backups    — back up real files before replacing (§8)
    Force              bool // force_symlinks    — retained for parity; inert in practice (§4.1)
    Validate           bool // validate_after    — run post-apply validation (default true)
    ManageLinuxDesktop bool // manage_linux_desktop — opt-in Linux desktop set (§7.2)
}

// ActionKind classifies a planned filesystem operation.
type ActionKind int
const (
    ActionLink   ActionKind = iota // create or repoint a symlink Dest -> Src
    ActionBackup                   // copy a real file to the backup dir (before cleanup)
    ActionRemove                   // remove a real file/dir at Dest (cleanup, recursive for dirs)
    ActionCopy                     // Zed: unlink legacy symlink, copy Src -> Dest (overwrite, 0644)
    ActionSkip                     // source missing, or dest already a correct symlink (no-op)
)

// Action is a single planned operation with absolute, resolved paths.
type Action struct {
    Spec     LinkSpec
    Kind     ActionKind
    SrcAbs   string // filepath.Join(repoRoot, Spec.Src)
    DestAbs  string // filepath.Join(home, Spec.Dest)
    Reason   string // human-readable ("source missing", "wrong target", "real file", ...)
}

// Plan is the computed set of actions plus the resolved run context (no writes yet).
type Plan struct {
    Actions   []Action // in phase-stable order: Backup, Remove, Link, Copy, Skip
    BackupDir string   // home/.config/dotfiles_backup_<epoch>, epoch captured once (§8)
}

// ValidationResult is one post-apply lstat check over a managed dest.
type ValidationResult struct {
    Dest    string // $HOME-relative
    Valid   bool   // stat.islnk && stat.exists  → "Valid symlink"
    Broken  bool   // stat.exists && !stat.islnk → real file where a symlink was expected (FAIL)
}

// Report is the outcome of Apply (+ optional Validate).
type Report struct {
    Applied     []Action           // actions actually performed (empty under DryRun)
    Planned     []Action           // full plan (always populated)
    Validations []ValidationResult
    DryRun      bool
}

type Engine struct { repoRoot, home string } // constructor: NewEngine(repoRoot, home string) *Engine
```

Typed errors (implement `Error()`, mirror `backend`/`manifest`):
- `MissingSourceTreeError{Root string}` — `repoRoot` (dotfiles_dir) does not exist (hard
  fail, §3.1 step 3).
- `LinuxDesktopGuardError` — `is_linux_family && ManageLinuxDesktop && !DryRun && !Backup`
  (§7.2 safety gate).
- `UnsafeDestError{Dest string}` — `Dest` is absolute or contains `..` (belt-and-suspenders
  over `manifest.validateSymlink`).
- `BrokenLinkError{Dests []string}` — validation found ≥1 dest with `exists && !islnk`
  (§4.2 fail condition).

### Methods (the Plan/Apply/Validate split)

```go
func DefaultTable() []LinkSpec // the static ported table (source of truth)

func NewEngine(repoRoot, home string) *Engine

// Plan stats sources + dests and computes Actions WITHOUT touching the filesystem.
// It applies OS/flag faceting, enforces the Linux-desktop safety gate, and rejects
// unsafe dests. Pure/read-only; safe to call in dry-run.
func (e *Engine) Plan(specs []LinkSpec, info platform.Info, opts Options) (*Plan, error)

// Apply performs the plan's mutations in phase order (Backup → Remove → Link → Copy),
// skipping ALL writes when opts.DryRun. Creates the backup dir + required parent dirs
// (skipped under DryRun) before linking.
func (e *Engine) Apply(plan *Plan, opts Options) (*Report, error)

// Validate lstat's every managed (non-Copy) dest and returns per-dest results; it is
// NOT gated by DryRun (only by opts.Validate at the call site). Broken links (dangling
// symlinks) PASS; only exists&&!islnk fails (§4.2).
func (e *Engine) Validate(specs []LinkSpec, info platform.Info, opts Options) ([]ValidationResult, error)
```

`applies(spec LinkSpec, info platform.Info, opts Options) bool` (unexported) implements
§7.1: `FacetAll`→true; `FacetMacOS`→`info.OS==platform.OSMacOS`; `FacetArch`→
`info.OS==platform.OSArch`; `FacetWindows`→`info.OS==platform.OSWindows`;
`FacetLinuxDesktop`→`isLinuxFamily(info.OS) && opts.ManageLinuxDesktop`, where
`isLinuxFamily := info.OS ∈ {OSDebian,OSRedHat,OSArch,OSSUSE}`.

**WSL faceting decision (flag explicitly):** per `symlink-semantics.md` §7.3, modern WSL
resolves to `OSDebian`/etc. and matches Ansible. Only a WSL host *without* `/etc/os-release`
yields `platform.OSWSL`. **Recommendation: treat `OSWSL` as Linux-family** (`isLinuxFamily`
returns true for `OSWSL`) for parity, and add a code comment noting the divergence risk. A
subtest documents this choice.

---

## Ordered Task Breakdown

Batches: tasks in the same batch touch disjoint files and may run in parallel; batches run
in order.

| Batch | Tasks | Depends on |
| --- | --- | --- |
| B1 | 1 | — |
| B2 | 2, 3, 4, 5, 6 (same package; sequence to avoid file collisions or split files) | 1 |
| B3 | 7 | 2-6 |
| B4 | 8 | 2-7 |
| B5 | 9 | 7 |
| B6 | 10 | 7 |

> Tasks 2-6 all land in `internal/symlink` and are described as separate concerns for
> clarity; the implementer may fold them into `symlink.go` + `apply.go` + `validate.go`
> while keeping each file under the ~500-line soft cap (`test-ci-conventions.md` §4). New
> behavior ships with its test in the same task (repo policy).

### Task 1 — Link table + OS-faceting

- **Files**: CREATE `internal/symlink/table.go`, `internal/symlink/table_test.go`.
- **Implement**:
  - Package doc comment on `table.go` (or `symlink.go`) stating the single responsibility
    and that it mirrors `playbooks/vars/symlinks.yml`.
  - `Facet` enum + consts; `LinkSpec` type; `DefaultTable() []LinkSpec` transcribing all 40
    managed entries **in §2 order** (bash→zsh→common→prompt→app→macos→linux-desktop→
    launchers→archlinux→windows→utility→completions→special) plus the Zed `Copy:true` entry
    (`zed/settings.json` → `.config/zed/settings.json`). Set `IsDir` from `type:directory`,
    `BackupEligible:true` for the 14 shell/common/prompt file entries (table rows 1-14),
    `Facet` per §7.1. Include the `// SYNC:` maintenance comment.
  - `isLinuxFamily(os platform.OS) bool` and `applies(...)` predicate.
- **Validate**: `go test ./internal/symlink/ -run TestTable` (asserts count = 41, per-facet
  counts, and BackupEligible set = the 14 shell/common/prompt dests).

### Task 2 — Engine + `Plan` (decision logic)

- **Files**: CREATE `internal/symlink/symlink.go` (Engine, `NewEngine`, `Options`, `Plan`,
  `Action`/`ActionKind`, `Plan` method, typed errors), `internal/symlink/symlink_test.go`.
- **Implement** (`Plan`, pure/read-only, per §4 matrix):
  1. **Linux-desktop safety gate** first: if any applicable spec has `FacetLinuxDesktop`
     and `!opts.DryRun && !opts.Backup` on a Linux-family host → return
     `LinuxDesktopGuardError` (§7.2). (Compute from `info`+`opts`, before statting.)
  2. For each spec where `applies(...)`: resolve `SrcAbs=Join(repoRoot,Src)`,
     `DestAbs=Join(home,Dest)`; re-validate `Dest` (reject abs/`..` → `UnsafeDestError`).
  3. `os.Stat(SrcAbs)` — **if source missing → `ActionSkip` (reason "source missing")**;
     the dest is neither cleaned nor linked (§4 "SRC-missing = total skip").
  4. `os.Lstat(DestAbs)`:
     - not exist → `ActionLink` (reason "create").
     - is symlink (`mode&ModeSymlink != 0`): `os.Readlink`; if target==SrcAbs →
       `ActionSkip` (reason "already correct", idempotent, §4a); else → `ActionLink`
       (reason "repoint", covers wrong-target §4b and dangling §4d — no force needed).
     - is real file/dir → emit `ActionBackup` first **iff** `spec.BackupEligible && opts.Backup`
       (§8), then `ActionRemove` (recursive for dirs, §4c), then `ActionLink`.
     - `Copy` spec (Zed): produce `ActionCopy` (its own unlink-legacy-symlink+copy handled in
       Apply, §10); never `ActionLink`/`ActionBackup`/`ActionRemove`.
  5. Capture `BackupDir = Join(home, ".config", "dotfiles_backup_"+strconv.FormatInt(time.Now().Unix(),10))`
     **once** (§8).
  6. Emit actions in stable phase order (Backup, Remove, Link, Copy, Skip) for deterministic
     output.
- **Validate**: `go test ./internal/symlink/ -run 'TestPlan'` (see Task 8 cases).

### Task 3 — `Apply`: backup + cleanup + link + dry-run gating

- **Files**: CREATE `internal/symlink/apply.go` (+ cases in `symlink_test.go` /
  `apply_test.go`).
- **Implement** (`Apply`, executes `Plan.Actions` in phase order; every mutation guarded by
  `!opts.DryRun`):
  - **Phase 0 (pre-create, skip under DryRun)**: `os.MkdirAll(BackupDir, 0o755)` iff
    `opts.Backup`; the required-dir list + per-dest parent dirs (Task 6).
  - **Backup**: `copyFile(DestAbs, Join(BackupDir, Dest))` preserving mode; create the
    backup subdir first when `filepath.Dir(Dest) != "."` (§8). Backups run **before** any
    Remove (ordering caveat §8).
  - **Remove**: `os.RemoveAll(DestAbs)` (files and dirs alike, §4c/§5.1).
  - **Link**: create the parent dir if needed, then `os.Symlink(SrcAbs, DestAbs)`. For a
    repoint, `os.Remove(DestAbs)` (the old symlink) then `os.Symlink`. `Force` is accepted
    but does not change behavior — cleanup owns real-file removal (§4.1); document this.
  - Under `DryRun`, append nothing to `Report.Applied` and perform **zero** writes.
  - Return `*Report` with `Planned` always set, `Applied` set only when not dry-run.
- **Validate**: `go test ./internal/symlink/ -run 'TestApply'`.

### Task 4 — `Validate` + broken-link semantics

- **Files**: CREATE `internal/symlink/validate.go` (+ cases in `validate_test.go`).
- **Implement** (`Validate`, NOT gated by DryRun):
  - For each applicable **non-Copy** spec: `os.Lstat(DestAbs)`.
  - `Valid = err==nil && info.Mode()&os.ModeSymlink != 0` → "Valid symlink". A **dangling
    symlink still reports Valid** (lstat sees the link itself), matching Ansible §4.2.
  - `Broken = err==nil && info.Mode()&os.ModeSymlink == 0` → real file where a symlink was
    expected. **Fail iff any `Broken`** → aggregate dests into `BrokenLinkError`. Do NOT
    flag dangling links (matching the deliberately-narrow Ansible condition, §4.2).
  - Zed copy dest is **excluded** from validation (§10).
- **Validate**: `go test ./internal/symlink/ -run 'TestValidate'`.

### Task 5 — Zed copy exception

- **Files**: extend `internal/symlink/apply.go` (or CREATE `copy.go`) + tests.
- **Implement** (`ActionCopy` handler, §10, skip under DryRun):
  1. `os.Lstat(DestAbs)`; if it is a symlink → `os.Remove` (migrate legacy symlink to a real
     copied file).
  2. Ensure parent dir (`.config/zed`) exists.
  3. `copyFile(SrcAbs, DestAbs)` with mode `0o644`, **overwrite** (repo wins; no content
     comparison). Skip entirely if `SrcAbs` missing.
  4. `keymap.json` stays a normal symlink (it is a `FacetAll` non-Copy entry in the table).
- **Validate**: `go test ./internal/symlink/ -run 'TestApply_Zed'`.

### Task 6 — Required-directory pre-creation + parent-dir creation

- **Files**: extend `internal/symlink/apply.go` + tests.
- **Implement** (§6, all skipped under DryRun):
  - `requiredDirs(home) []string` = the load-bearing list: `.ssh`, `.zsh/completions`,
    `.config`, `.branchlet`, `.config/atuin`, `.config/lazygit`, `.config/systemd/user`,
    `.config/zed`, `.local/share/applications`. `MkdirAll` each at `0o755`.
  - Additionally `MkdirAll(filepath.Dir(DestAbs), 0o755)` per applicable entry (robustness;
    §6 recommendation) — covers any dest whose parent is not in the fixed list.
  - `os.Symlink` into a missing parent fails `ENOENT`; this pre-creation is what prevents it.
- **Validate**: `go test ./internal/symlink/ -run 'TestApply_CreatesRequiredDirs'`.

### Task 7 — `dotf link` subcommand + main.go wiring

- **Files**: CREATE `cmd/dotf/link.go`, `cmd/dotf/link_test.go`; UPDATE `cmd/dotf/main.go`,
  `cmd/dotf/main_test.go`.
- **Implement**:
  - `func runLink(args []string, stdout, stderr io.Writer, r exec.Runner) int` following the
    seam (`go-cli-patterns.md` §2). Flags via `flag.NewFlagSet("link", ContinueOnError)` +
    `fs.SetOutput(stderr)`:
    - `--dry-run` (bool, "print the plan without executing")
    - `--backup` (bool, "back up existing real files before replacing them")
    - `--force` (bool, "force symlink recreation (parity; cleanup owns real-file removal)")
    - `--no-validate` (bool, "skip post-apply validation") → `opts.Validate = !noValidate`
    - `--linux-desktop` (bool, "manage the opt-in Linux desktop config set")
    - parse error → `return 2`.
  - Orchestration: `repoRoot := resolveRepoRoot()`; if `repoRoot==""` or missing →
    `reportErr(...)` (1). `home, err := os.UserHomeDir()`. `info := platform.Detect(r)`.
    `eng := symlink.NewEngine(repoRoot, home)`. `specs := symlink.DefaultTable()`.
    `plan, err := eng.Plan(specs, info, opts)` (surfaces `LinuxDesktopGuardError`,
    `MissingSourceTreeError` as `reportErr`).
    - If `--dry-run`: print `dry-run=true` + sorted `link.create=` / `link.repoint=` /
      `link.backup=` / `link.remove=` / `link.copy=` / `link.skip=` lines (dests joined,
      sorted by dest) to **stdout**; still run `Validate` for reporting; return 0.
    - Else: `report, err := eng.Apply(plan, opts)`; print applied-action summary lines
      (stdout=data), diagnostics/summary to stderr (mirror `printInstallSummary`). If
      `opts.Validate`: `results, err := eng.Validate(...)`; on `BrokenLinkError` print the
      broken dests to stderr and `return 1` (converged-vs-not signal, `go-cli-patterns.md`
      §9). Non-broken → 0.
  - **No `--manifest`** flag (engine uses the static table; a dead flag would mislead) — note
    this deviation from `go-cli-patterns.md` §3 in a code comment.
  - main.go: extend `usage` const to include `link`; add
    `case "link": return runLink(fs.Args()[1:], os.Stdout, os.Stderr, exec.New())`.
  - main_test.go: add `TestRun_Link` (captureOutput + `link --dry-run`, regex stdout for
    `dry-run=true`), mirroring `TestRun_Install`.
- **Validate**: `go test ./cmd/dotf/ -run 'TestRunLink|TestRun_Link'`.

### Task 8 — Full edge-case test suite + playbook-parity test

- **Files**: extend `internal/symlink/*_test.go`, `cmd/dotf/link_test.go`.
- **Implement**: table-driven, `t.TempDir()` for every FS test (build a fake dotfiles tree +
  fake `$HOME`); assert real on-disk state via `os.Readlink`/`os.Lstat`/`os.ReadFile`
  (`test-ci-conventions.md` §3.2). Cover **every** acceptance-criterion edge case (see Test
  Strategy). Include the **parity test** proving `DefaultTable()` == the playbook's entries.
- **Validate**: `go test -race ./internal/symlink/ ./cmd/dotf/`.

### Task 9 — Bash-untouched guard (NO bootstrap cutover this phase)

- **Files**: NONE modified. `bootstrap.sh` and `bin/install-*.sh` are **deliberately left
  untouched** (parallel-then-cutover; the flow-level cutover is Phase 7).
- **Implement**: nothing to write. This task is a **guard**, not a change:
  - Confirm `git diff --stat -- bootstrap.sh 'bin/install-*.sh'` is **empty** at the end of
    the phase (mirrors the Phase 5 plan's bash-untouched guard).
  - Rationale: cutting `run_symlinks` over to `dotf link` now would make the Go binary a
    bootstrap prerequisite before the Phase 7 shim delivers it, and would diverge from Phase 3
    (which left `bootstrap.sh` shelling to `bin/install-*.sh`). AC #3 is met at the capability
    level (`dotf link` needs no Ansible; playbook retired), not by rewiring bootstrap.
- **Validate**: `git diff --stat -- bootstrap.sh 'bin/install-*.sh'` prints nothing.

### Task 10 — Documentation: mark playbook retired + update references

- **Files**: UPDATE `playbooks/README.md`, `README.md`, `CLAUDE.md`, `AGENTS.md`,
  `tasks/todo.md`.
- **Implement**:
  - `playbooks/README.md`: add a top banner —
    "> RETIRED (retained for rollback). Symlinks are provisioned natively by `dotf link`
    (`internal/symlink`) as of Phase 4. This playbook is kept unexecuted for one release; do
    not run it in normal provisioning. Slated for deletion next release." Keep all existing
    content below.
  - `CLAUDE.md`: present `dotf link` as the **native** symlink engine and mark the playbook
    retired/rollback in the "Symlink Management" and "Stack Overview" sections. **Accuracy
    constraint:** `bootstrap.sh` still shells to the bash provisioning path until the Phase 7
    cutover, so do NOT claim Ansible is entirely removed from provisioning — state that the
    *symlink mechanism* is now native `dotf link` and the playbook is retired-but-retained.
    Keep edits surgical; do not rewrite unrelated sections.
  - `README.md`: document `dotf link` (`--dry-run`, `--backup`, `--linux-desktop`) as the
    native symlink provisioner; note the playbook is retained for rollback.
  - `AGENTS.md`: add a short note that symlinks are provisioned by `dotf link`, playbook
    retired (respect its sync policy with `CLAUDE.md` — do not duplicate content).
  - `tasks/todo.md`: mark Phase 4 items, add an outcome section, and a follow-up to (a) delete
    the playbook next release, (b) cut `bootstrap.sh` over in Phase 7, and (c) consolidate the
    manifest `[[symlink]]` redundancy.
- **Validate**: `npm run lint` (markdown/shell); manual doc review.

---

## Test Strategy — each acceptance criterion → concrete test

All tests are stdlib `testing`, table-driven, hermetic on `t.TempDir()`, asserting real FS
state (`test-ci-conventions.md` §3.2). Helper `buildTree(t, root, files...)` seeds sources;
helper `writeReal(t, dest, data)` / `mkSymlink(t, target, link)` seeds `$HOME` state.

| # | Acceptance criterion | Test(s) |
| --- | --- | --- |
| **1** | Identical link set to the playbook on a test tree | `TestPlan_IdenticalSet_PerOS`: seed a **fully-populated** fake dotfiles tree (all 41 sources) + empty `$HOME`; call `Plan` for each of `{macos, arch, debian, generic-linux}` × `{linux_desktop off/on}`; assert the resulting `ActionLink`+`ActionCopy` `(Src,Dest)` set **exactly equals** a golden list transcribed per-OS from `playbooks/vars/symlinks.yml`. **Plus `TestTable_MatchesPlaybook`** (parity, see below) proving `DefaultTable()` == the playbook data. |
| **2 · backup-if-not-symlink** | Backup only real files, only backup-eligible entries, only with `--backup` | `TestApply_Backup`: (a) real file at `.bashrc` + `--backup` → file copied to `BackupDir/.bashrc` **then** dest is a symlink; (b) a **symlink** at `.bashrc` is NOT backed up; (c) a real file at `.config/nvim` (dir, not backup-eligible) is removed with **no backup**; (d) `--backup` off → real file removed, no backup dir created (`os.ReadDir` shows none). |
| **2 · dryrun** | No filesystem mutations; still plans + validates | `TestApply_DryRun_NoMutations`: seed real files at several dests; `Apply` with `DryRun`; assert `Report.Applied` empty, no backup dir, dests unchanged (`os.ReadDir($HOME)` count unchanged, original files still real). `TestValidate_RunsUnderDryRun`: a pre-existing real file at a managed dest still yields `Broken` under dry-run (§4.2/§9). |
| **2 · validate** | Post-apply validation report + fail condition | `TestValidate`: table of dest states → `{Valid, Broken}`: correct symlink→Valid,!Broken; **dangling symlink→Valid,!Broken** (does not fail); real file→!Valid,Broken; missing→!Valid,!Broken. `TestValidate_FailsOnRealFile` → `BrokenLinkError` lists the dest; all-symlink tree → nil. |
| **2 · broken-link** | Dangling symlink is repointed, not failed | `TestPlan_BrokenSymlink_Repointed`: dest is a dangling symlink → `Plan` yields `ActionLink` (repoint); after `Apply`, `os.Readlink(dest)==SrcAbs`. `TestValidate_DanglingPasses` (covered above). Also `TestPlan_WrongTarget_Repointed` (§4b). |
| **2 · Zed-copy** | Zed settings copied, not linked; legacy symlink migrated; keymap stays a symlink; excluded from validation | `TestApply_Zed_CopyNotSymlink`: source `zed/settings.json` → after `Apply`, `.config/zed/settings.json` is a **regular file** (Lstat: no symlink bit) with matching content (`bytes.Equal`), perm `0o644`; a pre-existing **symlink** at the dest is unlinked then replaced by the copy; `.config/zed/keymap.json` is a **symlink**. `TestValidate_ExcludesZed`: a real `settings.json` does NOT trigger `BrokenLinkError`. |
| **3** | Ansible no longer required to provision symlinks | `TestRun_Link` (dispatch smoke: `dotf link --dry-run` returns 0, stdout has `dry-run=true`); `TestRunLink_AppliesToTempHome` (end-to-end via injected `home`/`repoRoot` in an engine test). **Manual gate**: `dotf link --dry-run` succeeds with `ansible-playbook` absent from `$PATH`. |
| **4** | Playbook retained (unexecuted) for one release | Non-goal / process guard: `TestPlaybookRetained` — assert `playbooks/symlinks.yml` and `playbooks/vars/symlinks.yml` still exist on disk (a cheap regression guard against accidental deletion), plus the `playbooks/README.md` RETIRED banner is present. |

**Parity test detail (criterion #1, the sync guard). DECIDED: inline golden set (zero new
dependency).** `TestTable_MatchesPlaybook` encodes the expected 41-entry set inline in the
test file (`{src,dest,type,facet,backupEligible}`, transcribed from `playbooks/vars/
symlinks.yml` during review) and diffs it against `DefaultTable()`. This fully satisfies
criterion #1 ("identical link set to the playbook"), keeps the module at its current minimal
dependency set (only `huh` + `go-toml`; no test-only YAML parser), and matches this repo's
dependency-hygiene ethos. Drift risk during the rollback window is low because the playbook is
**RETIRED** (frozen, not edited, slated for deletion). The `// SYNC:` comment on `table.go`
plus review discipline guard the playbook side. (A test-only `gopkg.in/yaml.v3` parity parse
was considered for auto-drift-detection and rejected: it adds a production `go.mod` require
line for a frozen artifact, against repo minimalism — the inline golden locks the Go table
just as firmly.)

Additional required tests (from `symlink-semantics.md` §12 checklist +
`test-ci-conventions.md`):
- `TestPlan_SourceMissing_Skips`: source absent → `ActionSkip`; a pre-existing real file at
  the dest is **preserved** (not removed).
- `TestPlan_LinuxDesktopGuard`: linux-family + `--linux-desktop` + `!dryrun` + `!backup` →
  `LinuxDesktopGuardError` and **zero** actions; `--dry-run` OR `--backup` satisfies it.
- `TestNewEngine_MissingSourceTree`: nonexistent `repoRoot` → `MissingSourceTreeError`.
- `TestPlan_UnsafeDest`: a `..`/absolute dest → `UnsafeDestError`, dest dir untouched
  (mirror `download_test.go` traversal shape).
- `TestApply_CorrectSymlink_Idempotent`: re-run over a correct symlink → `ActionSkip`, no
  writes (`os.ReadDir` unchanged).
- `TestFaceting_WSL`: documents the `OSWSL`→Linux-family decision.
- Negative CLI tests: `TestRunLink_BadFlag` (code 2, non-empty stderr) and, if the source
  tree is missing, code 1 with `dotf: ` stderr prefix (`test-ci-conventions.md` §3.4).
- Root/permission guard: skip perm tests under `os.Getuid()==0`; assert graceful typed
  errors, never panic.

---

## Risks & Mitigations

| Risk | Mitigation |
| --- | --- |
| **Static table drifts from `playbooks/vars/symlinks.yml`** during the rollback window (two sources of truth) | The `TestTable_MatchesPlaybook` inline-golden parity test fails CI on any change to `DefaultTable()`; `// SYNC:` comment; the playbook is RETIRED/frozen (low drift risk); todo.md follow-up to delete the playbook next release. |
| **`--force` semantics diverge from Ansible** (a naive `--force` that converts real files at link time removes user files even when it shouldn't) | Keep cleanup (`ActionRemove`) as the sole owner of real-file removal, gated on source-exists; `Force` is accepted but inert (§4.1). Documented + a test asserting `--force` alone does not remove a real file when the source is missing. |
| **Validation "broken link" naming trap**: over-eager flagging of dangling symlinks | Implement the exact §4.2 condition (`fail iff exists && !islnk`); dangling symlinks PASS. `TestValidate_DanglingPasses` locks it. Do not "fix" the arguably-misnamed condition. |
| **Missing parent dir → `os.Symlink` ENOENT** | Pre-create the §6 required-dir list + `MkdirAll(Dir(dest))` per entry before linking (Task 6); `TestApply_CreatesRequiredDirs`. |
| **`bootstrap.sh` cutover runs before `dotf` is built** | Warning-only fallback (locate `bin/dotf`, else `go run ./cmd/dotf`, else warn + skip — not a hard fail), per the "non-essential steps must not poison exit status" lesson. |
| **Backup ordering** (backup after cleanup would copy nothing) | Apply runs Backup phase strictly before Remove phase (§8 caveat); `TestApply_Backup` asserts the backup contains the original bytes. |
| **CI is Linux-only, non-root** (`test-ci-conventions.md` §4) | Faceting is data-driven and exercised for macOS/arch/debian in-process (no OS gating needed for `Plan` tests); perm tests skip under root; the default Linux non-root path is fully covered. |
| **`os.RemoveAll` blast radius** on a mis-resolved dest | `UnsafeDestError` guard + `manifest.validateSymlink`-style abs/`..` rejection on every `Dest` before join; `TestPlan_UnsafeDest`. Only real files/dirs whose source exists are removed. |

---

## Full Local Validation Gate

Run from the repo root; all must be clean before "done" (`test-ci-conventions.md` §1):

```bash
go build ./...                 # 1. build everything
go vet ./...                   # 2. CI "Vet"
go test -race ./...            # 3. CI "Test" (authoritative; -race mandatory)
golangci-lint run ./...        # 4. CI "Lint" (pinned v2.12.2; revive + standard set)
gofmt -l .                     # 5a. format — MUST print nothing
goimports -l .                 # 5b. imports — MUST print nothing
```

Repo wrappers (equivalent): `bin/lint.sh --go`, `bin/format.sh --go`, `bin/style.sh lint --go`.

**Bash-untouched guard (parallel-then-cutover — MUST be empty):**

```bash
git diff --stat -- bootstrap.sh 'bin/install-*.sh'   # EXPECT: no output
```

Doc changes additionally:

```bash
npm run lint                   # full mixed-language lint; pre-push runs this
```

Lefthook gates: **pre-commit** runs `bin/lint.sh --staged --shell --python --go` (staged
only, no auto-stage); **commit-msg** enforces Conventional Commits — use scope `symlink`
(e.g. `feat(symlink): add native Go symlink engine and dotf link`); this plan file under
`docs/prps/**` commits as `docs(internal): …`. **pre-push** runs `npm run lint`.

Constraints new code must satisfy: every exported symbol + the package get a doc comment
starting with the identifier name (`revive`); `staticcheck`/`govet`/`unused`/`ineffassign`
apply to `_test.go` too (only `errcheck` is relaxed there — discard test errors explicitly
via `_, _ = w.Write(...)`); keep each file under the ~500-line soft cap; never `sh -c`; the
engine uses `os.*` directly (no new FS-mock interface — `t.TempDir()` is the seam).

---

## Public API of `internal/symlink` (summary)

- Types: `Facet` (+ `FacetAll`/`FacetMacOS`/`FacetArch`/`FacetWindows`/`FacetLinuxDesktop`),
  `LinkSpec{Src,Dest,IsDir,Copy,Facet,BackupEligible}`,
  `Options{DryRun,Backup,Force,Validate,ManageLinuxDesktop}`,
  `Action`/`ActionKind`, `Plan`, `ValidationResult`, `Report`, `Engine`.
- Errors: `MissingSourceTreeError`, `LinuxDesktopGuardError`, `UnsafeDestError`,
  `BrokenLinkError`.
- Funcs: `DefaultTable() []LinkSpec`, `NewEngine(repoRoot, home string) *Engine`,
  `(*Engine).Plan`, `(*Engine).Apply`, `(*Engine).Validate`.
- CLI: `cmd/dotf/link.go:runLink(args []string, stdout, stderr io.Writer, r exec.Runner) int`,
  registered in `cmd/dotf/main.go` as the `link` subcommand.

## Open Questions — RESOLVED (orchestrator decisions, 2026-07-01)

1. **Parity test dependency** — **RESOLVED: inline golden set, zero new dependency.** See the
   "Parity test detail" section. Keeps the module minimal; the retired/frozen playbook makes
   auto-drift-detection unnecessary.
2. **`bootstrap.sh` binary sourcing** — **RESOLVED: moot.** `bootstrap.sh` is NOT cut over in
   Phase 4 (see Task 9 + Non-goals). The flow-level bootstrap→binary cutover is Phase 7,
   consistent with Phase 3. No Go toolchain becomes a bootstrap prerequisite in this phase.
3. **WSL faceting** — **RESOLVED: treat `platform.OSWSL` as Linux-family** for parity (matches
   the Phase 5 `register.Default` mapping). Documented via a code comment + `TestFaceting_WSL`.
