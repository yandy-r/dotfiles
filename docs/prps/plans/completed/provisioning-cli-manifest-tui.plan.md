# Plan: Manifest schema + intent-selection TUI (dotf Phase 2)

## Summary

Build the declarative TOML manifest — the single source of truth for the `dotf`
rewrite — as a dependency-free typed schema in `internal/manifest` (parse,
strict-validate, `go:embed` default, `--manifest`/`$DOTF_MANIFEST` override), then
render it through a `charmbracelet/huh` multi-step TUI (OS → pre-checked intents →
editable package plan → confirm → dry-run preview). Covers Forgejo tracker **#17**
= issue **#5** (manifest) + issue **#6** (TUI). Per the resolved schema decision,
the manifest also reserves a data-only `[[symlink]]` table (ported from
`playbooks/vars/symlinks.yml`) so Phase 4 reuses one parser and one embed.

## User Story

As the maintainer provisioning a fresh machine/VM/container/VPS, I want a
hand-editable declarative TOML manifest of intents and packages that renders as a
pre-checked, add/remove-able selector, so that I can declare what a machine is for
and preview/adjust the exact install plan before applying — adding a new tool by
editing **one TOML entry with zero Go/control-flow edits**.

## Problem → Solution

Package lists today are imperative bash arrays (`bootstrap.sh:746-763,787-799,1108-1133`)
with the ordered section list hand-duplicated in 2–3 synchronized places, and there is
no previewable/selectable install plan → one embedded declarative TOML manifest
(`[intents.<name>]` with `default`, `[[package]]` → `backend` + per-manager `names`
map, `[[symlink]]` reserved) parsed and strict-validated in `internal/manifest`,
rendered by a `huh` multi-step wizard that round-trips through a dry-run preview.

## Metadata

- **Complexity**: Large (13 files across 4 packages; first third-party deps + first `go:embed`)
- **Source PRD**: `docs/prps/prds/provisioning-cli.prd.md`
- **PRD Phase**: Phase 2 — Manifest schema + TUI (tracker #17: issues #5, #6)
- **Estimated Files**: 13 (11 create, 2 update)

## Batches

Tasks grouped by dependency for parallel execution. Tasks within a batch run
concurrently; batches run in order. Tasks in the same batch never touch the same file.

| Batch | Tasks         | Depends On | Parallel Width |
| ----- | ------------- | ---------- | -------------- |
| B1    | 1.1, 1.2, 1.3 | —          | 3              |
| B2    | 2.1, 2.2      | B1         | 2              |
| B3    | 3.1           | B2         | 1              |
| B4    | 4.1           | B3         | 1              |
| B5    | 5.1           | B4         | 1              |

- **Total tasks**: 8
- **Total batches**: 5
- **Max parallel width**: 3

> **Contract-first invariant**: Task 1.1 (`internal/manifest/types.go`) is the frozen
> schema imported by Phase 3 (backends) and Phase 4 (symlinks). It lands in B1 before
> any consumer. After merge, **additions are safe; renames/removals are forbidden**.

---

## UX Design

### Before

Today's `bootstrap.sh` (1,848 lines) offers no previewable/selectable plan:

- `--force`/headless (`bootstrap.sh:203-216`, linear flow `1766-1820`): runs a hard-coded ~40-function section list top-to-bottom; no OS choice, no tool selection, no preview.
- `--interactive` (`bootstrap.sh:1751-1765`, `prompt_continue` `230-242`): pauses AFTER each section with `[Y/n] (n=stop)` — a continue/abort gate, **not** a plan selector. You can only stop, never curate up front.
- Default run: ~30 sub-installers each emit their own `[y/N]` prompts (`bootstrap.sh:153`) — the "babysit and hope nothing drifted" experience.
- OS auto-detected only, no override (`bootstrap.sh:283`); package lists are opaque bash arrays.

### After

`dotf plan` guided flow — `charmbracelet/huh` multi-step form:

```
[1] OS ─────────────────┐  [2] Intents ────────────────┐  [3] Packages (dev) ───────────┐
 Detected: arch (x86_64) │   [x] base       (default)   │   [x] neovim   [x] ripgrep     │
 ( ) override…           │   [x] dev        (default)   │   [x] fzf      [ ] emacs       │
   debian macos fedora   │   [ ] gaming                 │   [-] remove: docker           │
 [ Next ]                │   [ Next ]                   │   [ Next ]                     │
─────────────────────────┘  (pre-check ← default=true) ─┘  (per-package add/remove) ─────┘
                                       ▼
[4] Confirm / dry-run preview ─────────────────────────────────────────
 OS: arch   Intents: base, dev  |  PLAN (nothing applied):
  pacman: neovim ripgrep fzf     github-release: mise
 [ Apply → deferred to Phase 6 ]   [ Back ]   [ Cancel ]
```

`--headless`/`--force` (a later phase) bypasses steps 1–4: applies manifest
`default=true` for the detected OS+intents, no prompts. Non-TTY routes to accessible
mode via `huh`'s `WithAccessible`.

### Interaction Changes

| Touchpoint       | Before                                               | After                                                           | Notes                                        |
| ---------------- | ---------------------------------------------------- | --------------------------------------------------------------- | -------------------------------------------- |
| OS selection     | Auto-detect only (`bootstrap.sh:283-368`)            | Auto-detect shown + selectable override (huh step 1)            | Uses canonical `internal/platform`           |
| Intent selection | None — fixed section list (`bootstrap.sh:1752`)      | huh MultiSelect, pre-checked from manifest `default=true`       | Headline UX; mirrors archinstall profiles    |
| Per-package edit | None — opaque bash arrays                            | Pre-checked package screen, toggle off, add extra tools         | omakub-style per-app organization            |
| Plan preview     | None; runs blind                                     | Step 4 dry-run summary BEFORE any apply                         | Closes the core "previewable/selectable" gap |
| Sub-tool prompts | ~30 scripts each prompt `[y/N]` (`bootstrap.sh:153`) | Single confirm gate; no per-tool prompting                      | Eliminates babysitting                       |
| Non-TTY          | `--force` is the only non-interactive path           | `huh.WithAccessible` fallback; non-TTY → headless (later phase) | Phase-2 wires the accessible flag            |

---

## Mandatory Reading

Files that MUST be read before implementing:

| Priority       | File                                                              | Lines   | Why                                                                                               |
| -------------- | ----------------------------------------------------------------- | ------- | ------------------------------------------------------------------------------------------------- |
| P0 (critical)  | `docs/prps/prds/provisioning-cli.prd.md`                          | 116-127 | Manifest schema + backend taxonomy + delivery decisions                                           |
| P0 (critical)  | `internal/platform/types.go`                                      | all     | Typed-string enum + struct convention to mirror; `PkgMgr`/`OS` vocab the `names` map keys against |
| P0 (critical)  | `internal/platform/platform.go`                                   | 14-52   | `Detect(r)` + injectable-path pattern to mirror for `--manifest`                                  |
| P0 (critical)  | `internal/platform/osrelease.go`                                  | 11-58   | Parse-from-path + fail-fast-on-partial-read pattern                                               |
| P1 (important) | `internal/exec/exec.go`                                           | 1-35    | Interface seam + package-doc no-shell-interpolation contract                                      |
| P1 (important) | `internal/exec/fake.go`                                           | 42-100  | Test-double + `fmt.Errorf("pkg: … %q")` error style                                               |
| P1 (important) | `internal/platform/platform_test.go`                              | 11-47   | Table-driven + `t.TempDir()` fixture harness to mirror                                            |
| P1 (important) | `cmd/dotf/main.go`                                                | 15-60   | Tiny-main + `run()`-returns-int + subcommand switch + key=value stdout                            |
| P1 (important) | `cmd/dotf/main_test.go`                                           | 12-110  | `captureOutput` helper + regexp stdout assertions                                                 |
| P1 (important) | `playbooks/vars/symlinks.yml`                                     | 40-90   | `{src,dest,type}` + OS-faceted lists to port into `[[symlink]]`                                   |
| P2 (reference) | `docs/prps/plans/completed/provisioning-cli-foundation.plan.md`   | all     | Phase 1 plan format, task/test style to mirror                                                    |
| P2 (reference) | `dotenv/direnv.toml`, `aerospace/aerospace.toml`, `starship.toml` | all     | Repo TOML conventions (tables, arrays, banner comments)                                           |
| P2 (reference) | `.golangci.yml`                                                   | all     | Lint gate (`revive`; `_test.go` errcheck-exempt)                                                  |

## External Documentation

| Topic                               | Source                                                 | Key Takeaway                                                                                                                                                                                                                                         |
| ----------------------------------- | ------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| TOML strict decode (**chosen lib**) | https://pkg.go.dev/github.com/pelletier/go-toml/v2     | **Use `pelletier/go-toml/v2`.** `Decoder.DisallowUnknownFields()` → typed `*toml.StrictMissingError` (line-pointed). First-class fail-fast.                                                                                                          |
| BurntSushi alternative (rejected)   | https://github.com/pelletier/go-toml/discussions/506   | No strict toggle — must inspect `MetaData.Undecoded()` manually. Effectively unmaintained. Rejected.                                                                                                                                                 |
| Strict-mode caveat                  | https://pkg.go.dev/github.com/pelletier/go-toml/v2     | Unknown-field rejection works only against **structs**, not `map[string]T`. Model `[[package]]` as `[]Package` structs; `[intents.*]` as `map[string]Intent` (map value is a struct).                                                                |
| Array-of-tables + named maps        | https://github.com/pelletier/go-toml/blob/v2/README.md | `[[package]]`→`[]Package`; `[intents.<name>]`→`map[string]Intent`; explicit `toml:"…"` tags (v2 matching is case-insensitive).                                                                                                                       |
| Decode from `io.Reader`/`fs.FS`     | https://pkg.go.dev/github.com/pelletier/go-toml/v2     | `d := toml.NewDecoder(r); d.DisallowUnknownFields(); d.Decode(&m)` — composes with `fs.FS.Open()` for embed + override with one code path.                                                                                                           |
| huh MultiSelect + pre-check         | https://github.com/charmbracelet/huh                   | `huh.NewOption(label,val).Selected(true)` = pre-checked → maps `default=true`. `.Value(&slice)` binds results.                                                                                                                                       |
| huh multi-step Group/Form           | https://github.com/charmbracelet/huh                   | Each `NewGroup` = a wizard page. `huh.NewForm(g1,g2,…).Run()` → OS → intents → packages → confirm.                                                                                                                                                   |
| huh dynamic dependent options       | https://github.com/charmbracelet/huh                   | `.OptionsFunc(fn, &binding)` re-evaluates when a prior binding changes — packages depend on selected intents.                                                                                                                                        |
| huh accessible / non-TTY            | https://github.com/charmbracelet/huh                   | `.WithAccessible(true)` fallback. GOTCHA (#547): accessible mode can double-count pre-selected answers — dedupe results.                                                                                                                             |
| huh version pick                    | https://github.com/charmbracelet/huh/releases          | v2 stable (v2.0.x, on Bubble Tea v2). Pin exact version; isolate the dep to the TUI package only.                                                                                                                                                    |
| go:embed default + override         | https://pkg.go.dev/embed                               | `//go:embed manifest/*.toml` (glob non-recursive; dotfiles excluded). GOTCHA: `embed.FS` keeps the `manifest/` prefix; `os.DirFS(dir)` roots at `dir` — normalize with `fs.Sub(embedded,"manifest")` so both branches use identical `fs.FS` loading. |

---

## Patterns to Mirror

Code patterns discovered in the codebase. Follow these exactly.

### NAMING_CONVENTION

```go
// SOURCE: internal/platform/types.go:12-28
// Typed-string enum: `type X string` + a doc-commented const block of canonical
// values. MIRROR for `type Backend string` (7 backends) and reuse platform.OS/PkgMgr.
type OS string
const (
	OSMacOS  OS = "macos"
	OSDebian OS = "debian"
	OSArch   OS = "arch"
)
```

```go
// SOURCE: internal/platform/types.go:85-102 — struct = parsed record of typed fields.
// MIRROR for Manifest/Intent/Package (doc-commented, typed fields, explicit toml tags).
type Info struct { OS OS; Arch Arch; PkgMgr PkgMgr /* … */ }
```

- Package name == dir, no stutter; package doc comment states its single source of truth (`internal/platform/types.go:1-10`).
- `types.go` holds enums+structs; behaviour lives in concern-named files (`platform.go`, `osrelease.go`). One file per concern (`exec.go` real vs `fake.go` double).
- Only the entry point is exported (`Detect`); helpers are unexported lowerCamel (`detectOS`). Constructors `New`/`NewFake` (`exec.go:35`, `fake.go:42`).
- **No package-level `var ErrX` sentinels exist yet** — errors are built inline. Introduce a sentinel only at the rule-of-three (3+ `errors.Is` callers).

### ERROR_HANDLING

```go
// SOURCE: internal/exec/fake.go:86 — package-prefixed, %q-quoted context.
return nil, fmt.Errorf("exec.Fake: no canned output registered for %q", CommandKey(name, args...))
```

```go
// SOURCE: internal/platform/osrelease.go:49-55 — fail-fast on partial read:
// on scanner error, discard partial data and return the zero value rather than
// silently returning partial results. MIRROR: manifest Parse must return (nil, err)
// on any decode/validate failure — NEVER a half-populated *Manifest.
return "", ""
```

- **CONTRAST — do NOT copy platform's graceful degradation.** `platform.Detect` degrades to `"unknown"` on failure because it probes an untrusted host; the manifest override (`--manifest`/`$DOTF_MANIFEST`) is user-supplied config and must **fail-fast**, not degrade. Document this divergence in `load.go`.
- Wrap the decoder error with `%w` so callers can `errors.As(err, *toml.StrictMissingError)` and `errors.Is` sentinels (`exec_test.go:69-78` shows the `errors.Is` assertion style).
- Exit-code convention (`cmd/dotf/main.go:22-46`): parse error → 2; usage/unknown → 1; stderr `progName: msg`.

### REPOSITORY_PATTERN (data access / parsing)

```go
// SOURCE: internal/platform/osrelease.go:22-35 — open→scan parse-from-path.
// MIRROR for manifest.LoadPath (but fail-fast instead of degrade).
f, err := os.Open(path)
if err != nil { return "" }
defer f.Close()
scanner := bufio.NewScanner(f)
```

```go
// SOURCE: internal/platform/platform.go:33, osrelease.go:11 — injectable path:
// a default const + a function param so tests override without touching the real host.
// MIRROR for the override resolver: const defaultManifest… + Resolve(flag, env, fsys).
const defaultOSReleasePath = "/etc/os-release"
func detectOS(r exec.Runner, osReleasePath, procVersionPath string) OS { /* … */ }
```

```yaml
# SOURCE: playbooks/vars/symlinks.yml:42-52 — the {src,dest,type} contract + OS-faceted
# lists (macos_configs / linux_configs / archlinux_configs). Port data-only into [[symlink]].
prompt_config:
  - { src: 'starship.toml', dest: '.config/starship.toml' }
app_configs:
  - { src: 'nvim', dest: '.config/nvim', type: 'directory' }
```

### SERVICE_PATTERN (composition + dispatch)

```go
// SOURCE: internal/exec/exec.go:21-35 — interface seam defined at the producer, real
// impl + fake double. MIRROR: take io.Reader/fs.FS in the parser so tests feed hostile
// TOML without disk; reuse exec.Runner in the plan command, never call os/exec directly.
type Runner interface {
	Output(name string, args ...string) ([]byte, error)
	LookPath(file string) (string, error)
}
func New() Runner { return osRunner{} }
```

```go
// SOURCE: cmd/dotf/main.go:35-60 — subcommand switch + machine-greppable stdout.
// MIRROR: add `case "plan": return runPlan(...)`; preview prints key=value lines.
switch cmd := fs.Arg(0); cmd {
case "version": /* … */
}
fmt.Printf("os=%s arch=%s pkg=%s\n", info.OS, info.Arch, info.PkgMgr)
```

```go
// SOURCE: huh (grounded via api/tech researcher) — multiselect pre-check + multi-step form.
// default=true → .Selected(intent.Default); each NewGroup is one wizard page.
huh.NewForm(
	huh.NewGroup(osSelect),
	huh.NewGroup(intentSelect), // .Options(opt.Selected(intent.Default))
	huh.NewGroup(packageSelect),
	huh.NewGroup(confirm),
).WithAccessible(accessible).Run()
```

### TEST_STRUCTURE

```go
// SOURCE: internal/platform/platform_test.go:11-47 — table-driven + t.Run + in-test
// fixtures written to t.TempDir() (NO committed testdata/ elsewhere in the repo; either
// inline strings or TempDir is idiomatic here). Assertions: t.Errorf("Fn() = %q, want %q").
func writeFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { t.Fatalf("…") }
	return path
}
```

- Fake wiring for the exec seam: `exec.NewFake()` + `AddCommand`/`AddPath` (`platform_test.go:39-40`); compile-time assertion `var _ Runner = (*Fake)(nil)` (`fake.go:100`).
- Save/restore package vars before mutating (`version_test.go:31-32`); `captureOutput(t, fn)` + `regexp.MustCompile(...).MatchString` for command stdout (`main_test.go:12-50, 90-108`).
- Propagation asserted via `errors.Is(err, wantErr)` (`exec_test.go:69-78`). `_test.go` is errcheck-exempt (`.golangci.yml:15-19`).

---

## Files to Change

| File                                       | Action | Justification                                                                                                                                                                                                  |
| ------------------------------------------ | ------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/manifest/types.go`               | CREATE | Frozen schema contract: `Manifest`/`Intent`/`Package`/`Symlink` structs + `type Backend string` enum + `toml:` tags. **Dependency-free** (struct tags only) so Phase 3/4 import it without pulling `huh`.      |
| `internal/manifest/load.go`                | CREATE | Strict `Parse([]byte)` (`DisallowUnknownFields`), `LoadFS(fsys, name)`, and `Resolve(flag, env, embedded)` precedence (flag > env > embedded).                                                                 |
| `internal/manifest/validate.go`            | CREATE | Semantic `Validate(*Manifest)`: backend ∈ enum, per-backend required fields, `names` keys ∈ `platform` vocab, intent→package refs resolve, sha256 hex, no dup names. `errors.Join` of typed `ValidationError`. |
| `internal/manifest/manifest_test.go`       | CREATE | Table-driven parse/validate/precedence tests + `internal/manifest/testdata/*.toml` fixtures.                                                                                                                   |
| `internal/manifest/testdata/`              | CREATE | `valid.toml` (intents + all backend shapes + symlinks) and `invalid_*.toml` (one per failure mode).                                                                                                            |
| `embed.go` (repo root, `package dotfiles`) | CREATE | `//go:embed manifest/*.toml` → `var ManifestFS embed.FS`. Root pkg needed because `go:embed` cannot reach `../manifest` from `internal/`; zero internal imports keeps it cycle-free.                           |
| `manifest/packages.toml`                   | CREATE | Embedded default data: `[intents.*]` (`default=true`), `[[package]]` sample per backend, `[[symlink]]` ported from `playbooks/vars/symlinks.yml`.                                                              |
| `internal/tui/options.go`                  | CREATE | Pure, TTY-free mappers: `IntentOptions(m) []huh.Option[string]` with `.Selected(intent.Default)`; `PackagesForIntents(m, sel)` deduped union. Unit-testable without a terminal.                                |
| `internal/tui/plan.go`                     | CREATE | `Run(m *manifest.Manifest, info platform.Info) (Selection, error)`: assembles the 4-group `huh` form; `WithAccessible` for non-TTY.                                                                            |
| `internal/tui/tui_test.go`                 | CREATE | Unit-test the pure mappers (`IntentOptions`, `PackagesForIntents`) — no `form.Run()`.                                                                                                                          |
| `cmd/dotf/plan.go`                         | CREATE | `runPlan(args, stdout, stderr, r)`: sub-`FlagSet` `--manifest`, `manifest.Resolve` → `platform.Detect` → `tui.Run` → dry-run preview (key=value stdout).                                                       |
| `cmd/dotf/main.go`                         | UPDATE | Add `case "plan": return runPlan(...)` to the subcommand switch (line 35); update usage string.                                                                                                                |
| `cmd/dotf/main_test.go`                    | UPDATE | Dispatch test for `plan` (mirror `TestRun_Platform`), asserting preview stdout shape via regexp.                                                                                                               |
| `go.mod` + `go.sum`                        | UPDATE | Add `github.com/pelletier/go-toml/v2` and `charmbracelet/huh` (v2, exact-pinned); `go mod tidy`.                                                                                                               |

## NOT Building

- **Backend execution** — Phase 2 models `backend` + per-backend fields as _data only_; no installs run (Phase 3). No generic plugin/registry/interface abstraction — one consumer today; model as a validated typed string (rule-of-three unmet).
- **Symlink creation/apply** — the `[[symlink]]` table is reserved as data; no `os.Symlink`/backup/validate/broken-link logic (Phase 4). `playbooks/vars/symlinks.yml` and the Ansible playbook are **left unmodified** (parallel-then-cutover rollback).
- **Headless plan application / `--headless` / `--force` / `--snapper`** — the TUI produces + previews a plan; executing it and the full flag-parity surface are Phase 6. Phase 2 only wires the `huh.WithAccessible` fallback flag.
- **sysconfig / firewall modeling** in the `[[package]]` schema — Phase 5.
- **Finalizing `apt-repo` keyring/sources.list contents** — the schema reserves the envelope; exact values are Phase 3 (PRD open question).
- **No full `bubbletea` `tea.Model`/Update/View app** — `huh`'s high-level `Form`/`Group` covers issue #6.
- **No manifest meta-features** — no templating, `include`/`import`, remote/HTTP fetch, env interpolation, or schema-version migration engine. Scope = parse + strict-validate + `go:embed` default + `--manifest`/`$DOTF_MANIFEST` override.
- **No selection persistence/caching layer** — the `plan` command previews only; storing a selection for a later `apply` is out of scope.
- **`internal/manifest` must NOT depend on `charmbracelet/huh`** — the TUI dep is isolated to `internal/tui`.

---

## Step-by-Step Tasks

### Task 1.1: Frozen manifest schema types — Depends on [none]

- **BATCH**: B1
- **ACTION**: Create `internal/manifest/types.go` with the typed schema contract and a `Backend` enum. No third-party imports.
- **IMPLEMENT**: `type Manifest struct { Intents map[string]Intent \`toml:"intents"\`; Packages []Package \`toml:"package"\`; Symlinks []Symlink \`toml:"symlink"\` }`. `Intent{ Description string; Default bool; Packages []string }`. `Package{ Name string; Backend Backend; Names map[string]string; Repo, Asset, SHA256, URL, Script, Tool string }`(superset of fields across all 7 backends, data-only).`Symlink{ Src, Dest, Type string; OS string }`(OS-faceted variant of the`{src,dest,type}`contract).`type Backend string` + const block for the 7 values (`pkg`, `github-release`, `aur`, `toolchain`, `apt-repo`, `script-url`, `custom-script`) + a `func (b Backend) Valid() bool`.
- **MIRROR**: NAMING_CONVENTION (`types.go:12-28` enum + `85-102` struct).
- **IMPORTS**: none (struct tags only).
- **GOTCHA**: Do NOT import `go-toml` or `huh` here — this file is imported by Phase 3/4 and must stay dependency-free. `names` map values are package names; keys are `platform.PkgMgr` strings but keep the field `map[string]string` (validation checks keys, not the type).
- **VALIDATE**: `go build ./internal/manifest/` compiles; `go vet ./internal/manifest/`.

### Task 1.2: Add TOML + huh dependencies — Depends on [none]

- **BATCH**: B1
- **ACTION**: Add the two runtime dependencies to `go.mod`/`go.sum`.
- **IMPLEMENT**: `go get github.com/pelletier/go-toml/v2@latest` and `go get github.com/charmbracelet/huh@latest` (pin the resolved exact versions — v2 line for huh). Run `go mod tidy`.
- **MIRROR**: n/a (dependency bootstrap).
- **IMPORTS**: n/a.
- **GOTCHA**: Confirm the huh import path/version actually resolves (`github.com/charmbracelet/huh` v2.x); pin exact versions in `go.mod` — no `latest` left in the file. These are the repo's **first** runtime deps (Phase 1 was std-lib only).
- **VALIDATE**: `go build ./...` still succeeds; `go.mod` lists both deps with concrete versions; `go mod verify`.

### Task 1.3: Embedded default manifest — Depends on [none]

- **BATCH**: B1
- **ACTION**: Create the root `embed.go` and the embedded `manifest/packages.toml` data file (both required together — `go:embed` fails to compile with no matching file).
- **IMPLEMENT**: `embed.go` at repo root: `package dotfiles` with `import "embed"`, `//go:embed manifest/*.toml`, `var ManifestFS embed.FS`. Author `manifest/packages.toml` following the schema in Task 1.1: 2-3 `[intents.<name>]` (at least one `default = true`), one `[[package]]` per backend as a worked example, and a handful of `[[symlink]]` entries ported from `playbooks/vars/symlinks.yml` (starship, nvim, wezterm, aerospace with `os = "macos"`). Use repo TOML banner-comment style.
- **MIRROR**: REPOSITORY_PATTERN (`symlinks.yml:42-52`); repo TOML conventions (`dotenv/direnv.toml`, `aerospace/aerospace.toml`).
- **IMPORTS**: `embed`.
- **GOTCHA**: `go:embed` cannot reference parent dirs, so the embed var MUST live at repo root (top-level `manifest/` is PRD-locked). `embed.FS` retains the `manifest/` path prefix — the loader (Task 2.1) normalizes with `fs.Sub`. Keep `embed.go` free of any `internal/` import (cycle-free).
- **VALIDATE**: `go build ./...` compiles (proves the glob matches); the file is discoverable as `dotfiles.ManifestFS` from `internal/manifest` in Task 2.1.

### Task 2.1: Manifest loader + strict validation — Depends on [1.1, 1.2, 1.3]

- **BATCH**: B2
- **ACTION**: Create `internal/manifest/load.go` and `internal/manifest/validate.go` plus `manifest_test.go` and `testdata/*.toml`.
- **IMPLEMENT**: `load.go`: `Parse(r io.Reader) (*Manifest, error)` using `d := toml.NewDecoder(r); d.DisallowUnknownFields(); d.Decode(&m)`, then call `Validate`; return `(nil, err)` on any failure. `Resolve(flagPath, envVal string, embedded fs.FS) (*Manifest, error)` applying precedence flag > `$DOTF_MANIFEST` > embedded (`fs.Sub(dotfiles.ManifestFS, "manifest")`); when an explicit override path is set but unreadable, **error out — never silently fall back to embedded**. `validate.go`: `Validate(*Manifest) error` — `Backend.Valid()` for each package, per-backend required fields (`github-release` needs `Repo`+`Asset`; `script-url` needs `URL`; `custom-script` needs `Script`), `names` keys ∈ known `platform.PkgMgr` set, `SHA256` is 64-char lowercase hex when present, every `intent.Packages` ref resolves to a `[[package]].Name`, no duplicate package names; aggregate via `errors.Join`.
- **MIRROR**: REPOSITORY_PATTERN (`osrelease.go:22-35` parse; `platform.go:33` injectable path); ERROR_HANDLING (fail-fast, `%w` wrap); TEST_STRUCTURE (`platform_test.go:11-47`).
- **IMPORTS**: `github.com/pelletier/go-toml/v2`, `io`, `io/fs`, `os`, `errors`, `github.com/yandy-r/dotfiles` (for `ManifestFS`), `github.com/yandy-r/dotfiles/internal/platform`.
- **GOTCHA**: `DisallowUnknownFields` only works against structs — it is a no-op for the `map[string]Intent`, so validate intent contents explicitly. Return `(nil, err)` — never a half-populated `*Manifest`. Wrap the decode error with `%w` so `errors.As(err, &strictErr)` works. Do NOT import `huh` here.
- **VALIDATE**: `go test -race ./internal/manifest/` passes: valid fixture round-trips; unknown-key, bad-backend, dangling-intent-ref, missing-required-field, bad-sha256, and unknown `names` key all error; flag>env>embedded precedence proven; embedded `packages.toml` parses+validates clean.

### Task 2.2: TUI option mappers — Depends on [1.1, 1.2]

- **BATCH**: B2
- **ACTION**: Create `internal/tui/options.go` (pure mappers) and `internal/tui/tui_test.go`.
- **IMPLEMENT**: `IntentOptions(m *manifest.Manifest) []huh.Option[string]` — one option per intent, `.Selected(intent.Default)`. `PackagesForIntents(m *manifest.Manifest, selected []string) []string` — deduped union of `intent.Packages` for the selected intents, stable-sorted. Keep both TTY-free (no `form.Run()`), unit-testable.
- **MIRROR**: SERVICE_PATTERN (huh option pre-check); TEST_STRUCTURE (table-driven).
- **IMPORTS**: `github.com/charmbracelet/huh`, `github.com/yandy-r/dotfiles/internal/manifest`, `sort`.
- **GOTCHA**: Different files from Task 2.1 (safe in same batch). Dedupe the union — an accessible-mode double-count bug (huh #547) is worsened by many pre-checked options, so return a de-duplicated, deterministically ordered slice.
- **VALIDATE**: `go test -race ./internal/tui/` passes: `IntentOptions` marks exactly the `default=true` intents selected; `PackagesForIntents` returns the deduped sorted union for overlapping intents.

### Task 3.1: huh multi-step form runner — Depends on [2.2]

- **BATCH**: B3
- **ACTION**: Create `internal/tui/plan.go` with the interactive form runner.
- **IMPLEMENT**: `type Selection struct { OS platform.OS; Intents, Packages []string }` and `Run(m *manifest.Manifest, info platform.Info, accessible bool) (Selection, error)`. Build `huh.NewForm` with four groups: (1) OS `Select` seeded from `info.OS` with override options; (2) intents `MultiSelect` from `IntentOptions(m)`; (3) packages `MultiSelect` whose options derive from selected intents (`OptionsFunc` bound to the intent selection); (4) a `Confirm`. Apply `.WithAccessible(accessible)`. Return the collected `Selection`.
- **MIRROR**: SERVICE_PATTERN (huh multi-step form).
- **IMPORTS**: `github.com/charmbracelet/huh`, `internal/manifest`, `internal/platform`.
- **GOTCHA**: `plan.go` and `options.go` share `package tui` — `tui_test.go` (Task 2.2) tests only the mappers, so it compiles without `plan.go`; this task adds `plan.go` cleanly. Do not call `Run` in a unit test (needs a TTY) — cover it only through `cmd/dotf` dispatch and manual validation.
- **VALIDATE**: `go build ./internal/tui/` compiles; `go vet ./internal/tui/`; manual `dotf plan` smoke after Task 5.1.

### Task 4.1: `dotf plan` subcommand — Depends on [2.1, 3.1]

- **BATCH**: B4
- **ACTION**: Create `cmd/dotf/plan.go` with `runPlan`.
- **IMPLEMENT**: `runPlan(args []string, stdout, stderr io.Writer, r exec.Runner) int` — a sub-`FlagSet` with `--manifest string`; resolve via `manifest.Resolve(flagManifest, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS)`; `platform.Detect(r)`; `tui.Run(m, info, accessible)` (accessible from `!isTTY || $ACCESSIBLE`); on success print the dry-run preview as key=value lines to `stdout` (e.g. `os=… intents=… packages=…`, then per-backend package groups). Return the exit code.
- **MIRROR**: SERVICE_PATTERN (`main.go:35-60` switch + key=value stdout); ERROR_HANDLING (exit codes: parse→2, error→1).
- **IMPORTS**: `flag`, `io`, `os`, `internal/exec`, `internal/platform`, `internal/manifest`, `internal/tui`, `github.com/yandy-r/dotfiles`.
- **GOTCHA**: New file — does not touch `main.go` (that's Task 5.1, next batch, avoiding a concurrent write). Preview must apply nothing (Phase 6 owns apply). Keep the preview machine-greppable (mirrors the `version`/platform stdout style).
- **VALIDATE**: `go build ./cmd/dotf/`; `go vet ./cmd/dotf/`.

### Task 5.1: Wire `plan` into main + dispatch test — Depends on [4.1]

- **BATCH**: B5
- **ACTION**: Update `cmd/dotf/main.go` to route `plan`, and `cmd/dotf/main_test.go` to cover it.
- **IMPLEMENT**: Add `case "plan": return runPlan(fs.Args()[1:], os.Stdout, os.Stderr, exec.New())` to the subcommand switch (`main.go:35`); extend the usage string. In `main_test.go`, add a dispatch test mirroring `TestRun_Platform` using the `--manifest testdata/valid.toml` override so it runs without a TTY path (or assert usage/error for the no-TTY case), asserting preview stdout shape with `regexp.MustCompile`.
- **MIRROR**: `main.go:35-45` switch; `main_test.go:12-110` `captureOutput` + regexp.
- **IMPORTS**: `internal/exec` (already present in `main.go`).
- **GOTCHA**: `main.go` and `main_test.go` are edited together in one task (no other task touches them) — safe. Ensure the dispatch test does not block on interactive input: exercise the non-interactive/preview path via an override manifest or assert the accessible/error branch.
- **VALIDATE**: `go test -race ./cmd/dotf/`; full `go build ./... && go vet ./... && go test -race ./... && golangci-lint run ./...` green.

---

## Testing Strategy

### Unit Tests

| Test                                       | Input                                          | Expected Output                                      | Edge Case? |
| ------------------------------------------ | ---------------------------------------------- | ---------------------------------------------------- | ---------- |
| `Parse` valid manifest                     | `testdata/valid.toml`                          | `*Manifest` with intents+packages+symlinks           | No         |
| `Parse` unknown field                      | TOML with a stray key                          | `(nil, err)`; `errors.As` `*toml.StrictMissingError` | Yes        |
| `Validate` bad backend                     | `[[package]]` `backend = "script-ur"`          | non-nil error naming the package                     | Yes        |
| `Validate` dangling intent ref             | intent lists a package name that doesn't exist | non-nil error                                        | Yes        |
| `Validate` missing required field          | `github-release` with no `asset`               | non-nil error naming package+field                   | Yes        |
| `Validate` bad sha256                      | `sha256 = "XYZ"` (non-hex / wrong length)      | non-nil error                                        | Yes        |
| `Validate` unknown `names` key             | `names = { brrew = "…" }`                      | non-nil error (key ∉ platform vocab)                 | Yes        |
| `Resolve` precedence                       | flag + env + embedded all present              | flag wins; then env; then embedded                   | Yes        |
| `Resolve` unreadable override              | `--manifest /nonexistent`                      | error — NOT a silent embedded fallback               | Yes        |
| Embedded default parses                    | `dotfiles.ManifestFS` via `fs.Sub`             | parses + validates clean                             | No         |
| `IntentOptions` pre-check                  | manifest with mixed `default`                  | exactly the `default=true` options `.Selected`       | No         |
| `PackagesForIntents` union                 | two overlapping intents                        | deduped, stable-sorted union                         | Yes        |
| `runPlan` dispatch (`--manifest` override) | `plan --manifest testdata/valid.toml`          | preview stdout matches key=value regexp              | No         |

### Edge Cases Checklist

- [ ] Empty manifest (no packages) parses or errors deterministically
- [ ] Unknown / mis-cased TOML key rejected (strict decode)
- [ ] `map[string]Intent` contents validated (strict decode is a no-op on maps)
- [ ] `Parse` never returns a partially populated `*Manifest` alongside an error
- [ ] `--manifest` override path unreadable → error, no embedded fallback
- [ ] `$DOTF_MANIFEST` set but `--manifest` also set → flag wins
- [ ] Accessible / non-TTY path does not double-count pre-selected options (huh #547)
- [ ] `internal/manifest` imports neither `huh` nor `bubbletea` (`go list -deps`)

---

## Validation Commands

### Static Analysis

```bash
go build ./...
go vet ./...
```

EXPECT: Zero build/vet errors.

### Unit Tests

```bash
go test -race ./internal/manifest/ ./internal/tui/ ./cmd/dotf/
```

EXPECT: All tests pass (race-clean).

### Full Test Suite + Lint

```bash
go test -race ./...
golangci-lint run ./...   # or: bin/lint.sh --go
gofmt -l . && goimports -l .   # expect empty output
```

EXPECT: No regressions, no lint findings, nothing unformatted.

### Dependency-Isolation Check

```bash
go list -deps ./internal/manifest/ | grep -E 'charmbracelet|bubbletea' && echo "LEAK" || echo "clean"
```

EXPECT: `clean` — the TUI dependency must not leak into `internal/manifest`.

### Manual Validation

- [ ] `go run ./cmd/dotf plan` launches the 4-step huh form (OS → intents → packages → confirm) in a real terminal
- [ ] Intents with `default=true` in `manifest/packages.toml` render pre-checked
- [ ] Toggling a package off / adding one updates the confirm-step preview
- [ ] The confirm step prints a dry-run plan and applies nothing
- [ ] `DOTF_MANIFEST=/path/to/other.toml go run ./cmd/dotf plan` loads the override
- [ ] `go run ./cmd/dotf plan --manifest ./internal/manifest/testdata/valid.toml` previews without error

---

## Acceptance Criteria

Merged from issues #5 + #6 and the PRD Phase-2 success metrics:

- [ ] Adding a sample tool = **1 TOML `[[package]]` entry, 0 Go/control-flow edits** (verified by diff review)
- [ ] Manifest parses both table families: `[intents.<name>]` (`default`) and `[[package]]` (`backend` + `names` + backend fields), plus the reserved `[[symlink]]`
- [ ] Schema validation rejects malformed manifests **fail-fast** with a clear error (no silent fallback, no partial `*Manifest`)
- [ ] Manifest embedded via `go:embed manifest/*.toml` (self-contained on a fresh machine)
- [ ] Override via `--manifest <path>` flag and `$DOTF_MANIFEST` env, precedence flag > env > embedded
- [ ] `default = true` maps 1:1 to a pre-checked `huh` selection (`.Selected(true)`)
- [ ] Selecting intents produces a correct, editable (add/remove) package plan
- [ ] The plan round-trips through a dry-run preview before any apply
- [ ] The multi-step form composes OS → intents → manifest → confirm
- [ ] `backend` validated against the closed 7-value enum; unknown value is a fatal parse error
- [ ] `sha256` modeled first-class and validated as 64-char lowercase hex when present
- [ ] `internal/manifest` has **no** `charmbracelet/huh` dependency
- [ ] All validation commands pass; tests written and passing; no vet/lint errors

## Completion Checklist

- [ ] `internal/manifest/types.go` merged and frozen as the FIRST artifact, before the TUI (B1)
- [ ] `[[package]].backend` accepts all 7 values; each backend's data fields present as reserved struct fields
- [ ] `[[package]].names` keys validated against `internal/platform` `PkgMgr` vocab
- [ ] `go:embed manifest/*.toml` ownership documented (root `embed.go`); Phase 4 reuses this embed for `[[symlink]]`
- [ ] `--manifest` flag + `$DOTF_MANIFEST` env both proven over the embedded default
- [ ] Symlink-modeling decision (reserve `[[symlink]]` now) recorded in Notes so Phase 4 is unblocked
- [ ] Phase 1 contracts untouched: no edits to `internal/platform` enums or `internal/exec.Runner` signature
- [ ] `bootstrap.sh`, `bin/install-*.sh`, `playbooks/vars/symlinks.yml` left unmodified (parallel-then-cutover)
- [ ] Code follows discovered patterns; error handling fail-fast; tests table-driven with `t.TempDir()`
- [ ] No hardcoded values; conventional commit `feat(dotf):`/`feat(go):`; self-contained (no open questions)

## Risks

| Risk                                                                                                              | Likelihood | Impact | Mitigation                                                                                                                                      |
| ----------------------------------------------------------------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Manifest struct omits a field for 1+ of the 7 backends → Phase 3 re-cuts the shared type → Phase 6 re-integration | High       | High   | Reserve the full per-backend field envelope (all 7) as data-only fields in Task 1.1; land + freeze the type first (B1).                         |
| Phase 3 & 4 build against a schema still churning in Phase 2                                                      | High       | High   | `internal/manifest/types.go` is Phase 2's first merged artifact; after freeze, additions safe, renames/removals forbidden.                      |
| `--manifest`/`$DOTF_MANIFEST` accepts arbitrary TOML; a typo/hostile key parses "successfully"                    | High       | High   | Strict `DisallowUnknownFields`; treat `*toml.StrictMissingError` as fatal; no lenient `Unmarshal`.                                              |
| Partial application — decode fills some structs then errors, caller consumes half-built manifest                  | Med        | High   | `Parse` validates whole manifest before returning; always `(nil, err)` on first failure.                                                        |
| `backend` free string → misspelled value only caught at Phase 3 exec time                                         | Med        | High   | `type Backend string` closed enum + `Valid()` checked in `Validate`; reject unknown at load.                                                    |
| `script-url` supply chain — `sha256` "optional" in PRD lets a remote installer be unpinned                        | Med        | High   | Model `sha256` first-class; validate hex+length when present; surface "unpinned script-url" as a validation warning (exec deferred to Phase 3). |
| go:embed fallback masks a real error by silently using the embedded default                                       | Med        | Med    | When an explicit override is set but unreadable → error out, never fall back.                                                                   |
| First third-party dep (`huh`/`bubbletea`) bloats go.sum/CI; Phase 1 kept it std-lib only                          | Low        | Low    | Scope the dep to `internal/tui`; keep `internal/manifest` dependency-free; pin exact versions; `go list -deps` gate.                            |
| huh accessible mode double-counts pre-selected answers (issue #547)                                               | Low        | Med    | Dedupe `PackagesForIntents`; test the accessible path.                                                                                          |

## Notes

- **Scope**: Forgejo tracker **#17** (Phase 2) = issue **#5** (declarative TOML manifest) + issue **#6** (intent-selection TUI). Runs in **parallel** with Phase 3 (backends) and Phase 4 (symlinks); all three depend only on Phase 1. Phase 6 is the integration barrier.
- **Resolved schema decision (this plan)**: the manifest **reserves a data-only `[[symlink]]` table now** (OS-faceted, ported from `playbooks/vars/symlinks.yml`), giving Phase 4 **one parser + one `go:embed`** to reuse. Phase 2 does not create/apply symlinks — data only. This resolves the PRD's internal ambiguity (L83/L120/L184 "symlink engine reads {src,dest,type} from TOML" vs L119/L171 "only intents+packages").
- **Contract-first ordering**: `internal/manifest/types.go` (Task 1.1) is the single source of truth imported by Phase 3/4. It lands in B1 and is frozen on merge — additions safe, renames/removals forbidden.
- **Divergence from the platform package**: `internal/platform` _degrades gracefully_ to `"unknown"` because it probes an untrusted host; the manifest override is user-supplied config and _fails fast_. Document this contrast in `load.go`.
- **Library choices** (grounded): `pelletier/go-toml/v2` (only mainstream lib with first-class `DisallowUnknownFields()`); `charmbracelet/huh` v2 (`.Selected(true)` maps 1:1 to manifest `default`, multi-step Groups). Both are the repo's first runtime deps.
- **Root `package dotfiles`**: `embed.go` lives at repo root because `go:embed` can't reach a parent dir and PRD locks top-level `manifest/`. It imports nothing internal (cycle-free); the loader in `internal/manifest` normalizes the `manifest/` prefix with `fs.Sub`.
- **Open item for a later phase (not this plan)**: whether `dotf plan` persists a selection for a later `apply` — deferred to Phase 6 (headless/apply). Phase 2 previews only.
- **Confidence Score**: 8/10. Schema-as-contract and parallel-dev risks are well understood and mitigated by contract-first ordering. Residual uncertainty: exact `huh` v2 cross-group `OptionsFunc` API should be confirmed against the pinned version at implement time (Task 3.1 GOTCHA); the sample `packages.toml` field names beyond `backend`/`names`/`default` are set by this plan, not a pre-existing fixture.
