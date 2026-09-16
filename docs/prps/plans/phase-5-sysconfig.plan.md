# Plan: `internal/sysconfig` + `dotf sysconfig` — hostname/timezone/locale/shell/firewall per-OS (Phase 5)

## Summary

Add a net-new `internal/sysconfig` package and a `dotf sysconfig` subcommand that own
OS-level system configuration: **hostname (+ `/etc/hosts`), timezone, locale, default
shell, and firewall**, per OS/distro family. Every privileged mutation routes through the
existing `internal/privilege` sudo model and the `internal/exec.Runner` seam so the whole
package is unit-testable with a fake runner and a rooted filesystem seam — no real syscalls
in tests. Reads compute an idempotent plan; a second `apply` on a converged host issues zero
mutating commands. Windows is a documented, deferred stub returning `ErrNotImplemented`.

This is **parallel-then-cutover**: the bash bootstrap (`bootstrap.sh`, `bin/install-*.sh`)
stays authoritative and untouched. Phase 5 depends only on Phase 1 (foundation) and
Phase 3's `internal/privilege` (already merged, PR #25).

## User Story

> As someone provisioning a fresh VM, I run `dotf sysconfig --hostname web01 --timezone
> America/New_York --locale en_US.UTF-8 --shell zsh --firewall auto`. The tool sets each
> value, updates `/etc/hosts`, generates the locale before setting it, only calls `chsh`
> after ensuring the shell is in `/etc/shells`, and enables the right firewall for my
> distro. Running the exact same command again changes nothing (idempotent no-op).

## Problem → Solution

The repo has **no** hostname/timezone/locale/user handling anywhere (`grep` for
`hostnamectl|/etc/hostname|timedatectl|localectl|useradd` → none; confirmed in the PRD and
`sysconfig-privilege.md §7`). Firewall (`ufw`/`firewalld`), `chsh`, and `/etc/shells`
handling exist only in `bootstrap.sh`. Phase 5 delivers these as a tested, per-OS Go package
driven by CLI flags (primary) plus an optional `[sysconfig]` manifest table for repo-wide
defaults.

## Metadata

| Field            | Value                                                                       |
| ---------------- | --------------------------------------------------------------------------- |
| Issue            | Forgejo #11 (Phase 5: System config)                                        |
| Module           | `github.com/yandy-r/dotfiles`, Go 1.26                                      |
| Commit scope     | `feat(sysconfig): …`, `test(sysconfig): …`; docs under `docs/prps/**` → `docs(internal): …` |
| Depends on       | Phase 1 (platform/exec/manifest), Phase 3 (`internal/privilege`, merged #25) |
| New package      | `internal/sysconfig`                                                        |
| New subcommand   | `dotf sysconfig`                                                            |
| Command name     | `sysconfig` (NOT `config` — `config` is overloaded in a dotfiles tool; see `go-cli-patterns.md §15`) |

## Batches

| Batch | Tasks                     | Can run in parallel | Rationale                                   |
| ----- | ------------------------- | ------------------- | ------------------------------------------- |
| B1    | 1.1, 1.2                  | yes (disjoint files) | Schema + core package compile standalone   |
| B2    | 2.1 → (2.2, 2.3, 2.4)     | 2.2/2.3/2.4 after 2.1 | Shared helpers first, then per-OS configurers |
| B3    | 3.1                       | —                   | `register.Default` wires configurers        |
| B4    | 4.1 → 4.2                 | sequential          | Command, then `main.go` dispatch            |
| B5    | 5.1                       | —                   | Docs                                        |

---

## Mandatory Reading (ground truth for this plan)

Read before implementing. Repo-relative paths.

- `docs/prps/plans/.prp-research/phase-4-5-symlink-sysconfig/sysconfig-privilege.md` — the
  **per-domain command tables** (exact argv per OS/distro), privilege rules, locale ordering,
  `/etc/shells` guard, firewall selection, Windows stub. This is the authority for WHAT
  commands run.
- `docs/prps/plans/.prp-research/phase-4-5-symlink-sysconfig/go-cli-patterns.md` — the
  `run<Cmd>` seam, flag boilerplate, manifest loading, OS dispatch, `Default(...)` wiring,
  error idiom, greppable output, exit codes, `main.go` diff, and the command-name decision.
  **Note the naming correction:** the run-func seam takes `exec.Runner`, NOT `exec.Execer`
  (there is no `Execer` in this repo).
- `docs/prps/plans/.prp-research/phase-4-5-symlink-sysconfig/test-ci-conventions.md` — the
  exact validation gate, `.golangci.yml` rules, `exec.Fake` API, argv-assertion pattern,
  `t.TempDir()` filesystem convention, root-conditional test skips, `assertReadOnlyCalls`.
- `docs/prps/prds/provisioning-cli.prd.md` — Decisions Log (firewall abstraction, Windows
  deferred), apply order (L107), canonical sysconfig commands (L123), Phase 5 scope (L186-189).

### Source files to mirror (exact anchors)

| Concern                     | File to read                                    | What to copy                                             |
| --------------------------- | ----------------------------------------------- | -------------------------------------------------------- |
| Exec seam                   | `internal/exec/exec.go`, `internal/exec/fake.go` | `Runner` (`Output`/`LookPath`/`Run`), `Fake`, `CommandKey`, `AddCommand`/`AddPath`/`RunErrors`/`CallsSnapshot` |
| Privilege model             | `internal/privilege/privilege.go`               | `NewSudo`, `Prime`, `KeepAlive`, `KeepAliveErr`, `Prefix`, `RealUser`/`User` |
| Backend interface + dispatch | `internal/backend/backend.go`                   | `interface` + `Engine{map[K]V}` + `NewEngine` + typed `UnknownBackendError` |
| Default constructor         | `internal/backend/register.go`                  | `Default(r, sudo, drop, repoRoot) *Engine` wiring pattern |
| Concrete impl shape         | `internal/backend/pkg.go`, `custom_script.go`    | struct-holds-`r`, `newXBackend(...)`, `installArgv` returning `[]string{"sudo",…}`, `var _ Iface = (*x)(nil)`, path-safety guard |
| Install command             | `cmd/dotf/install.go`                            | `runInstall` shape, `--manifest`/`--dry-run`, prime+keepalive (62-69), `resolveRepoRoot`, dry-run/summary split (stdout vs stderr), `installExitCode` |
| Drift command (read-only)   | `cmd/dotf/status.go`                             | read-only engine construction, exit-1-on-drift, sorted greppable output |
| Plan command (TTY/confirm)  | `cmd/dotf/plan.go`                              | `reportErr` (66), `isTTY` (75), confirm-gate + "not confirmed"→exit 1 |
| Manifest schema + validate  | `internal/manifest/types.go`, `validate.go`, `load.go` | `Manifest` struct, `errors.Join` validation, `ValidationError`, `Resolve` precedence, `DisallowUnknownFields` |
| Windows stub precedent      | `internal/privilege/drop_other.go`             | `ErrUnsupportedPlatform`-style sentinel (runtime dispatch here — no build tag needed) |
| Test harness                | `cmd/dotf/status_test.go`, `internal/backend/pkg_test.go` | `writeXTestManifest`, `archInfo`, `assertReadOnlyCalls`, `fmt.Sprint(args)` slice compare |

---

## Patterns to Mirror

### NAMING_CONVENTION
Exported (and most unexported) identifiers get a doc comment **starting with the identifier
name** (`revive` enforces this: `// Config is …`, `// ErrNotImplemented is …`). Package doc
lives on exactly one file (`sysconfig.go`) and states the single responsibility + which
`bootstrap.sh`/`common/*.sh` behavior it mirrors. Typed enum values are canonical string
consts (`FirewallUfw FirewallMode = "ufw"`), matching `manifest.Backend` / `platform.OS`.

### ERROR_HANDLING
Typed domain errors as structs implementing `Error()` (`UnsupportedOSError{OS platform.OS}`)
and sentinels via `errors.New` (`ErrNotImplemented`). Wrap with `%w` and a package-prefixed
message: `fmt.Errorf("sysconfig: hostname: %w", err)`. Aggregate multi-step failures
**non-fatally** (warning-only summary, per `tasks/lessons.md` 2026-03-17) like
`printInstallSummary`. Manifest validation aggregates via `errors.Join`.

### SERVICE_PATTERN (dependency injection)
Every OS `Configurer` is a struct holding its injected deps (`r exec.Runner`, `fs FileSystem`)
built by a `newXConfigurer(...)` constructor; the `Engine` wires them in `register.go`. This
mirrors `newPkgBackend(r)` / `backend.Default`. The command layer (`runSysconfig`) is the ONLY
place that constructs the real `exec.New()` runner, the real filesystem root, and touches
`*privilege.Sudo` (prime + keep-alive) — exactly like `runInstall`.

### REPOSITORY_PATTERN (read-only probes)
Idempotency reads go through `r.Output(name, args…)` (no sudo prefix, except `sudo ufw status`
which needs root) and `fs.ReadFile` for `/etc/hosts`, `/etc/shells`, `/etc/locale.gen`,
`/etc/passwd`. Reads NEVER mutate; a converged host yields an empty `Plan`.

### TEST_STRUCTURE
Stdlib `testing` only (no testify). Table-driven `(OS × domain)` subtests asserting **exact
argv** via `f.CallsSnapshot()` + `fmt.Sprint(call.Args)` compare. Filesystem-backed logic
tested against `t.TempDir()`-rooted `FileSystem`. Read-only paths asserted via an
`assertReadOnlyCalls`-style helper. Root-conditional branches gated with
`if os.Geteuid() == 0 { t.Skip(...) }` / the inverse, so the non-root Linux path (what CI runs)
is always covered.

---

## Public API of `internal/sysconfig`

> Design principle: **plan/apply split**. `Plan` reads current state and computes the ordered
> `Step`s needed to converge (read-only, dry-run-safe, deterministic). `Execute` performs the
> mutations. Idempotency is structural: a converged host produces an **empty `Plan`**, so
> `Execute` runs nothing on re-run. OS dispatch mirrors `backend.Engine`
> (`map[platform.OS]Configurer`).

### Core types (`sysconfig.go`)

```go
// Config is the resolved desired system state. An empty string field means the
// domain is left UNMANAGED (skipped) — never defaulted destructively. Firewall
// is FirewallUnset ("") for "skip".
type Config struct {
    Hostname string       // "" => skip. Per-machine; flag-only (never from manifest).
    Timezone string       // "" => skip. IANA "Area/City" (e.g. "America/New_York").
    Locale   string       // "" => skip. Linux only; e.g. "en_US.UTF-8" (macOS = no-op).
    Shell    string       // "" => skip. Name or path (e.g. "zsh" or "/usr/bin/zsh").
    Firewall FirewallMode // FirewallUnset => skip.
    // User is the real (non-root) account whose login shell chsh targets. The
    // command layer sets this from privilege.RealUser().Name so root's shell is
    // never changed by accident.
    User string
}

// FirewallMode selects the firewall backend. "auto" resolves per OS family.
type FirewallMode string
const (
    FirewallUnset     FirewallMode = ""          // skip
    FirewallOff       FirewallMode = "off"        // skip (explicit; does NOT disable an active firewall)
    FirewallAuto      FirewallMode = "auto"       // ufw on debian/arch, firewalld on redhat/suse, socketfilterfw on macOS
    FirewallUfw       FirewallMode = "ufw"
    FirewallFirewalld FirewallMode = "firewalld"
)

// Step is one planned convergence action. Exactly one of Argv or File is set.
type Step struct {
    Domain string       // "hostname"|"timezone"|"locale"|"shell"|"firewall"
    Desc   string       // greppable summary, e.g. "hostname=web01"
    Argv   []string     // command step: exact argv, sudo-prefixed where privileged
    File   *FileInstall // file step: render content then install via privileged cp
}

// FileInstall installs Content at Dest via a privileged copy. Execute writes
// Content to a 0600 temp file (FileSystem.WriteTemp), backs up Dest when Backup
// is set (sudo cp Dest Dest.dotf.bak), then sudo cp <tmp> Dest. The write is
// deferred to Execute so Plan stays read-only and --dry-run touches nothing.
type FileInstall struct {
    Dest    string
    Content []byte
    Backup  bool
}

// Plan is the ordered set of Steps needed to converge to Config. An empty
// Steps slice means already-converged (idempotent no-op). Skipped lists domains
// with no requested value.
type Plan struct {
    Steps   []Step
    Skipped []string
}

// StepResult / Result record Execute outcomes; failures are collected non-fatally.
type StepResult struct { Step Step; Err error }
type Result struct { Applied []StepResult; Skipped []string }

// Configurer computes the Plan for one OS family. It holds r/fs and never mutates.
type Configurer interface {
    Plan(cfg Config) (Plan, error)
}

// Engine dispatches Plan to the Configurer registered for platform.OS and owns
// Execute (OS-agnostic step runner).
type Engine struct {
    configurers map[platform.OS]Configurer
    r           exec.Runner
    fs          FileSystem
}
func NewEngine(cfgs map[platform.OS]Configurer, r exec.Runner, fs FileSystem) *Engine
func (e *Engine) Plan(cfg Config, info platform.Info) (Plan, error)   // read-only; ErrNotImplemented on windows
func (e *Engine) Execute(p Plan) (Result, error)                       // mutating; non-fatal per-step collection

// Typed errors
var ErrNotImplemented = errors.New("sysconfig: not implemented on this OS (deferred)")
type UnsupportedOSError struct { OS platform.OS }
func (e *UnsupportedOSError) Error() string
```

### Filesystem seam (`fs.go`)

```go
// FileSystem is the seam for reading system files during Plan and staging file
// installs during Execute. Production uses osFileSystem{root:""} (real /);
// tests use osFileSystem{root: t.TempDir()} with seeded /etc/* files. This is
// the one seam sysconfig needs beyond exec.Runner (per test-ci-conventions §3.3).
type FileSystem interface {
    ReadFile(name string) ([]byte, error) // reads filepath.Join(root, name)
    WriteTemp(prefix string, content []byte) (string, error) // 0600 temp file, returns path for a subsequent sudo cp
    Exists(name string) bool
}
func NewOSFileSystem(root string) FileSystem
```

### Constructor (`register.go`)

```go
// Default constructs an Engine with the linux, darwin, and (stub) windows
// configurers registered, backed by r and a filesystem rooted at root ("" = /).
func Default(r exec.Runner, root string) *Engine
```

### WHERE input values come from (minimal surface — justified)

**Decision: CLI flags (primary) + an optional `[sysconfig]` manifest table (defaults);
precedence flag > manifest > skip.**

- **CLI flags** on `dotf sysconfig`: `--hostname --timezone --locale --shell --firewall`
  (plus `--manifest --dry-run --yes`). Flags are the primary source and always win.
- **`[sysconfig]` manifest table** (additive; requires the schema in Task 1.1): holds
  repo-wide defaults for `timezone`, `locale`, `shell`, `firewall`. **`hostname` is
  intentionally NOT a manifest field** — it is inherently per-machine and flag-only.
- **Env parity** (`DOTF_HOSTNAME`, …): explicitly **Phase 6**, NOT built here.

Justification for including the manifest table now (vs flags-only):
1. The PRD frames sysconfig as **manifest-driven** ("declarative TOML manifest … and OS-level
   system config", PRD L19) — the manifest is the declarative desired-state source.
2. `go-cli-patterns.md §14` names this the "schema gap to close for Phase 5", and because the
   decoder runs `DisallowUnknownFields`, the `Manifest.Sysconfig` struct field **must exist
   before any `[sysconfig]` TOML is written** — so the schema work is unavoidable if we ever
   want the table.
3. Keeping `hostname` flag-only keeps per-host state out of a shared repo.
4. The embedded default `manifest/packages.toml` ships the `[sysconfig]` block **commented
   out**, so a bare `dotf sysconfig` (no flags, default manifest) resolves to an all-empty
   `Config` and skips every domain — **no destructive shipped default**. Users uncomment/edit
   in their own manifest, or pass flags.

Flags-only was considered but rejected: it would leave the PRD's declarative model unbuilt and
still force the same struct-field addition the moment a `[sysconfig]` table is wanted.

---

## Files to Change

| File                                             | Action | Justification                                                                 |
| ------------------------------------------------ | ------ | ----------------------------------------------------------------------------- |
| `internal/manifest/types.go`                     | UPDATE | Add `Sysconfig` struct + `Manifest.Sysconfig *Sysconfig` field (additive)     |
| `internal/manifest/validate.go`                  | UPDATE | Add `validateSysconfig` (firewall enum) wired into `Validate` via `errors.Join` |
| `internal/manifest/testdata/valid.toml`          | UPDATE | Add a valid `[sysconfig]` block so the loader round-trips it                   |
| `internal/manifest/testdata/invalid_sysconfig_firewall.toml` | CREATE | Golden: bad `firewall` value → `ValidationError`                 |
| `internal/manifest/manifest_test.go`             | UPDATE | Register the new invalid fixture + assert `[sysconfig]` decode                 |
| `manifest/packages.toml`                         | UPDATE | Add a **commented** `[sysconfig]` documentation block (no active defaults)     |
| `internal/sysconfig/sysconfig.go`                | CREATE | Package doc, `Config`/`FirewallMode`/`Step`/`FileInstall`/`Plan`/`Result`, `Configurer`, `Engine` + dispatch + `Execute`, typed errors |
| `internal/sysconfig/sysconfig_test.go`           | CREATE | Engine dispatch (unknown/windows → error), `Execute` step-running + non-fatal collection |
| `internal/sysconfig/fs.go`                       | CREATE | `FileSystem` seam + `osFileSystem{root}` (`ReadFile`/`WriteTemp`/`Exists`)     |
| `internal/sysconfig/fs_test.go`                  | CREATE | Rooted read + temp-write (0600) against `t.TempDir()`                          |
| `internal/sysconfig/hosts.go`                    | CREATE | Shared: parse/render `/etc/hosts` (ensure `127.0.1.1 <name>` line)             |
| `internal/sysconfig/shells.go`                   | CREATE | Shared: `/etc/shells` guard + shell-path resolution (candidate lists)          |
| `internal/sysconfig/locale_resolve.go`           | CREATE | Shared: locale fallback precedence + case/hyphen normalization (port `common/exports.sh`) |
| `internal/sysconfig/helpers_test.go`             | CREATE | Pure-function tests for hosts/shells/locale_resolve                            |
| `internal/sysconfig/linux.go`                    | CREATE | `linuxConfigurer.Plan` — hostname/tz/locale/shell/firewall (systemd family)   |
| `internal/sysconfig/linux_test.go`               | CREATE | `(distro × domain)` exact-argv + idempotency + locale ordering + `/etc/shells` guard |
| `internal/sysconfig/darwin.go`                   | CREATE | `darwinConfigurer.Plan` — scutil/systemsetup/dscl/socketfilterfw; locale = no-op |
| `internal/sysconfig/darwin_test.go`              | CREATE | macOS exact-argv (3× `scutil --set`, socketfilterfw) + idempotency            |
| `internal/sysconfig/windows.go`                  | CREATE | `windowsConfigurer.Plan` → `ErrNotImplemented`; documents deferred commands   |
| `internal/sysconfig/windows_test.go`             | CREATE | Windows Plan returns `ErrNotImplemented`, zero steps                           |
| `internal/sysconfig/register.go`                 | CREATE | `Default(r, root) *Engine` wiring linux/darwin/windows configurers            |
| `internal/sysconfig/register_test.go`            | CREATE | `Default` registers linux+darwin+windows; covers every dispatched `platform.OS` |
| `cmd/dotf/sysconfig.go`                           | CREATE | `runSysconfig`: flags, resolve (flag>manifest), Plan, dry-run, confirm/`--yes`, prime+keepalive, Execute, summary |
| `cmd/dotf/sysconfig_test.go`                      | CREATE | Direct run-func tests (`bytes.Buffer` + `exec.Fake` + rooted fs): dry-run, idempotent no-op, bad-flag→2, manifest-error→1 |
| `cmd/dotf/main.go`                                | UPDATE | Add `case "sysconfig"` to the switch; extend `usage`                          |
| `cmd/dotf/main_test.go`                           | UPDATE | `TestRun_Sysconfig` dispatch smoke (`captureOutput` + `--dry-run`)            |
| `docs/prps/plans/phase-5-sysconfig.plan.md`       | (this)  | The plan                                                                      |
| `CLAUDE.md`, `AGENTS.md`, `README.md`             | UPDATE | Document `dotf sysconfig` (Stack Overview / subcommand list)                   |
| `tasks/todo.md`                                   | UPDATE | Mark Phase 5 in-progress → done with an outcome section                       |

---

## NOT Building (explicit non-goals)

- **Windows sysconfig** — deferred stub ONLY. `windowsConfigurer.Plan` returns
  `ErrNotImplemented`; the file documents the eventual commands (`Rename-Computer`,
  `Set-TimeZone`, `Set-NetFirewallProfile`/Defender) without wiring them (PRD Decisions Log:
  "Windows sysconfig → gated/deferred, section stub present but unimplemented").
- **macOS locale** — no-op in v1. macOS locale is per-user (`defaults`/`AppleLocale`) and the
  shell layer (`common/exports.sh`) already resolves `LANG`/`LC_*` at runtime. `locale-gen`
  and `localectl` are Linux-only tools (`sysconfig-privilege.md §4.3`).
- **User creation / `useradd`** — out of scope; sysconfig configures the invoking real user's
  shell only.
- **Env-var flag parity** (`DOTF_HOSTNAME` …) — Phase 6.
- **Folding `sysconfig` into a combined `apply`** with early/late firewall sequencing and
  `--headless` — Phase 6. Phase 5 ships the standalone `sysconfig` command that runs the five
  domains in canonical order (hostname → timezone → locale → shell → firewall). `Step.Domain`
  tagging lets Phase 6 partition without reshaping the API.
- **Disabling / removing firewall rules** — `FirewallOff`/unset SKIP; sysconfig is
  additive-only (never tears down an active firewall). Consistent with the additive-only,
  read-only-drift stance in Phase 3.
- **pf as the primary macOS firewall** — `socketfilterfw` is the v1 macOS firewall; `pf`
  documented as available/advanced only (`sysconfig-privilege.md §4.5`).
- **Changes to `bootstrap.sh` / `bin/install-*.sh`** — parallel-then-cutover; bash stays
  authoritative and runnable. A `git diff` guard verifies this.
- **A `privilege.Dropper` path** — every sysconfig op *wants* root (the inverse of AUR). `chsh`
  runs **as root targeting the real user**, never dropped (`sysconfig-privilege.md §4.4`).

---

## Step-by-Step Tasks

> Configurers are structs holding `r exec.Runner` + `fs FileSystem` via constructors; the
> `Engine` wires them in `register.go`. Per-domain command specs below are transcribed from
> `sysconfig-privilege.md §4` — treat that table as the source of truth for exact argv.

### Task 1.1: Manifest `[sysconfig]` schema — Depends on [none]

- **BATCH**: B1
- **ACTION**: Add a `Sysconfig` struct + `Manifest.Sysconfig *Sysconfig` field; add validation;
  add fixtures + a commented example.
- **IMPLEMENT**: In `internal/manifest/types.go`:
  ```go
  // Sysconfig holds repo-wide default system-config values. Every field is
  // optional (empty => unmanaged). Hostname is intentionally absent — it is
  // per-machine and supplied only by the --hostname flag.
  type Sysconfig struct {
      Timezone string `toml:"timezone"`
      Locale   string `toml:"locale"`
      Shell    string `toml:"shell"`
      Firewall string `toml:"firewall"` // "ufw"|"firewalld"|"auto"|"off"
  }
  ```
  Add `Sysconfig *Sysconfig \`toml:"sysconfig"\`` to `Manifest`. In `validate.go`, add
  `validateSysconfig(s *Sysconfig) []error` (skip when nil) that rejects a `Firewall` value
  not in `{"", "ufw", "firewalld", "auto", "off"}` with a `&ValidationError{Msg: …}`; wire it
  into `Validate` before the final `errors.Join`. Timezone/locale/shell shape is
  runtime-validated (zoneinfo/`locale -a`/`/etc/shells`), so no static check beyond firewall.
  Add a valid `[sysconfig]` block to `testdata/valid.toml`; create
  `testdata/invalid_sysconfig_firewall.toml` (`firewall = "iptables"`); register it in
  `manifest_test.go`. Add a **commented** `[sysconfig]` documentation block to
  `manifest/packages.toml` (no active values).
- **MIRROR**: ERROR_HANDLING (`validate.go:115-162` per-field checks + `errors.Join`), the
  additive-only frozen-schema discipline used for `AptRepo`.
- **IMPORTS**: none new.
- **GOTCHA**: `Manifest` decodes with `DisallowUnknownFields` — the struct field MUST land in
  the same change as any TOML, or decode fails. Keep the embedded `packages.toml` block
  **commented** so a bare `dotf sysconfig` skips all domains (no shipped destructive default).
  Do not add a `hostname` manifest field.
- **VALIDATE**: `go test ./internal/manifest/` — `valid.toml` round-trips (`Sysconfig` non-nil),
  the invalid firewall fixture yields `*ValidationError`.

### Task 1.2: `internal/sysconfig` core (types + Engine + Execute + fs seam + errors) — Depends on [none]

- **BATCH**: B1
- **ACTION**: Create `internal/sysconfig/sysconfig.go` and `fs.go` — the package's data types,
  `Configurer` interface, `Engine` dispatch + `Execute`, filesystem seam, and typed errors.
  No OS logic yet (compiles standalone against `platform`/`exec`).
- **IMPLEMENT**: All types in the **Public API** section above. `Engine.Plan(cfg, info)` looks
  up `e.configurers[info.OS]`; returns `&UnsupportedOSError{OS: info.OS}` when absent (the
  windows stub IS registered and returns `ErrNotImplemented`, so unregistered means a truly
  unknown OS). `Engine.Execute(p Plan)` iterates `p.Steps`: for an `Argv` step run
  `e.r.Run(step.Argv[0], step.Argv[1:]...)`; for a `File` step, when `File.Backup && e.fs.Exists(Dest)`
  run `sudo cp <Dest> <Dest>.dotf.bak`, then `tmp, _ := e.fs.WriteTemp("dotf-sysconfig", File.Content)`
  and `e.r.Run("sudo", "cp", tmp, File.Dest)`. Collect each step's error into `Result.Applied`
  **non-fatally** (never abort mid-plan); `Result.Skipped = p.Skipped`. `fs.go`:
  `osFileSystem{root string}` where `ReadFile(name)` reads `filepath.Join(root, name)`,
  `WriteTemp` creates a `0o600` temp via `os.CreateTemp("", prefix)` and writes content,
  `Exists(name)` stats `filepath.Join(root, name)`.
- **MIRROR**: `backend.go` (interface + `Engine{map}` + `NewEngine` + typed error);
  `install.go` non-fatal result collection.
- **IMPORTS**: `errors`, `fmt`, `os`, `path/filepath`, `internal/exec`, `internal/platform`.
- **GOTCHA**: `Execute` must NOT prime sudo or detect TTY — that is the command layer's job.
  Keep `Plan` free of any mutation. The temp write in `Execute` is deferred (not done in
  `Plan`) so `--dry-run` writes zero files. Add `var _ Configurer = …` assertions in the OS
  files, not here.
- **VALIDATE**: `go build ./internal/sysconfig/ && go test ./internal/sysconfig/ -run 'Engine|FS'`
  — unknown-OS dispatch returns `*UnsupportedOSError`; `Execute` records argv + non-fatal
  errors; `osFileSystem` reads rooted + writes 0600.

### Task 2.1: Shared helpers — `hosts.go`, `shells.go`, `locale_resolve.go` — Depends on [1.2]

- **BATCH**: B2
- **ACTION**: Create the three pure-function helper files (+ `helpers_test.go`) used by both
  linux and darwin configurers.
- **IMPLEMENT**:
  - `hosts.go`: `hostsNeedsUpdate(content []byte, name string) bool` and
    `renderHosts(content []byte, name string) []byte` — ensure a `127.0.1.1 <name>` line
    (Debian convention), preserving `127.0.0.1 localhost` and all other lines; replace an
    existing `127.0.1.1 …` line rather than duplicating.
  - `shells.go`: `etcShellsHasPath(content []byte, path string) bool`;
    `renderEtcShells(content, path)` (append path if absent);
    `resolveShellPath(r exec.Runner, name string, info platform.Info) (string, error)` —
    if `name` is already an absolute existing path use it; else `LookPath(name)`, else walk a
    per-OS candidate list (macOS: `/opt/homebrew/bin/zsh`, `/usr/bin/zsh`; Linux:
    `/home/linuxbrew/.linuxbrew/bin/zsh`, `/usr/bin/zsh`, `/bin/zsh`) — from
    `bootstrap.sh:1690-1695`.
  - `locale_resolve.go`: `normalizeLocale(s string) string` (lowercase, strip whitespace,
    `utf-8`==`utf8`); `localeAvailable(out []byte, target string) bool` (normalized match over
    `locale -a` output); `resolveLocale(requested string, avail []byte) string` — precedence
    `requested → en_US.UTF-8 → C.UTF-8 → C` (port `common/exports.sh:231-312`
    `pick_dotfiles_locale`, commit `6e3d0332`).
- **MIRROR**: `custom_script.go` path-safety helpers (pure funcs, table tests);
  `common/exports.sh` locale precedence.
- **IMPORTS**: `bytes`, `strings`, `internal/exec`, `internal/platform`.
- **GOTCHA**: These are pure/parse functions — no sudo, no writes. `resolveShellPath` may call
  `r.LookPath`/`r.Output` (injectable) but never mutates. Match `common/exports.sh`
  case/hyphen normalization exactly so `en_us.utf-8 == en_US.UTF-8`.
- **VALIDATE**: `go test ./internal/sysconfig/ -run 'Hosts|Shells|Locale'` — table cases for
  hosts line ensure/replace, `/etc/shells` presence, locale normalization + fallback.

### Task 2.2: `linuxConfigurer` (systemd family) — Depends on [1.2, 2.1]

- **BATCH**: B2
- **ACTION**: Create `internal/sysconfig/linux.go` (+ `linux_test.go`) — `linuxConfigurer.Plan`
  building Steps for all five domains. Applies to `platform.OSDebian/OSRedHat/OSArch/OSSUSE/OSLinux/OSWSL`.
- **IMPLEMENT** (each domain: READ current state → if equals target, add domain to `Skipped`,
  emit no Step; else emit Step(s)). Argv per `sysconfig-privilege.md §4`:
  - **hostname** (`cfg.Hostname != ""`): read `hostnamectl --static` (`r.Output`); if `!= name`
    emit `Step{Argv:["sudo","hostnamectl","set-hostname",name]}`. Then read `/etc/hosts` via
    `fs.ReadFile`; if `hostsNeedsUpdate` emit a `File` step `{Dest:"/etc/hosts",
    Content: renderHosts(...), Backup: true}`.
  - **timezone** (`cfg.Timezone != ""`): validate `fs.Exists("/usr/share/zoneinfo/"+tz)` else
    return a wrapped error; read `timedatectl show -p Timezone --value`; if `!= tz` emit
    `Step{Argv:["sudo","timedatectl","set-timezone",tz]}`.
  - **locale** (`cfg.Locale != ""`): `avail := r.Output("locale","-a")`;
    `target := resolveLocale(cfg.Locale, avail)`. **Ordering is load-bearing:** if
    `!localeAvailable(avail, target)` AND `target ∉ {C,C.UTF-8}` AND `LookPath("locale-gen")`
    succeeds — Arch: read `/etc/locale.gen`, if the `<target> <charset>` line is commented emit
    a `File` step uncommenting it, then emit `Step{Argv:["sudo","locale-gen"]}`;
    Debian/Ubuntu: emit `Step{Argv:["sudo","locale-gen",target]}`. **After** generation, read
    current `LANG` (`localectl status` or `/etc/locale.conf`); if `!= target` emit
    `Step{Argv:["sudo","localectl","set-locale","LANG="+target]}`. The generate Step(s) MUST
    precede the set Step in `Plan.Steps`.
  - **shell** (`cfg.Shell != ""`): `path := resolveShellPath(...)`; if empty → add to `Skipped`
    (warn). Read `/etc/shells` via `fs.ReadFile`; if `!etcShellsHasPath` emit a `File` step
    appending `path` (Dest `/etc/shells`, `Backup:true`) — this MUST precede the chsh Step.
    Read current shell via `getent passwd <cfg.User>` field 7 (`r.Output`); if `!= path` emit
    `Step{Argv:["sudo","chsh","-s",path,cfg.User]}`.
  - **firewall** (`cfg.Firewall` resolves to ufw for linux via `auto`, or explicit `ufw`): if
    `LookPath("ufw")` missing emit the install Step (`sudo pacman -S --noconfirm --needed ufw`
    on Arch, `sudo apt-get install -y ufw` on Debian). Allow SSH: read `sudo ufw status`
    (**sudo prefix on this READ** — needs root); if `OpenSSH`/`22/tcp` rule absent emit
    `Step{Argv:["sudo","ufw","allow","OpenSSH"]}`. If status `!= active` emit
    `Step{Argv:["sudo","ufw","--force","enable"]}`. (RedHat/SUSE with explicit `firewalld`
    handled here too: `sudo systemctl enable --now firewalld`, then
    `firewall-cmd --permanent --query-service=ssh` gate →
    `sudo firewall-cmd --permanent --add-service=ssh` + `sudo firewall-cmd --reload`.)
  - Add `var _ Configurer = (*linuxConfigurer)(nil)`.
- **MIRROR**: `pkg.go:installArgv` (`[]string{"sudo",…}`), `custom_script.go` guards.
- **IMPORTS**: `fmt`, `strings`, `internal/exec`, `internal/manifest`? (no — Config is local),
  `internal/platform`.
- **GOTCHA**: `locale-gen` BEFORE `localectl set-locale` — enforce and test the Step order.
  `sudo ufw status` keeps the sudo prefix (unlike other reads). `chsh` targets `cfg.User`
  (the real user) as **root**, never dropped. Missing `locale-gen` (minimal containers) →
  fall back to `C.UTF-8` and still set it. A read that errors (tool absent) means "unset →
  emit the set Step", not a hard failure — degrade gracefully, never panic.
- **VALIDATE**: `go test ./internal/sysconfig/ -run Linux` — see Testing Strategy for the AC
  mapping (exact argv, idempotent no-op, locale ordering, `/etc/shells` guard, ufw selection).

### Task 2.3: `darwinConfigurer` — Depends on [1.2, 2.1]

- **BATCH**: B2
- **ACTION**: Create `internal/sysconfig/darwin.go` (+ `darwin_test.go`) — `darwinConfigurer.Plan`
  for `platform.OSMacOS`.
- **IMPLEMENT** (argv per `sysconfig-privilege.md §4`):
  - **hostname**: read `scutil --get ComputerName` / `--get LocalHostName` / `--get HostName`
    (each via `r.Output`; `--get HostName` may error "not set" → treat as unset → emit set).
    For each that differs, emit `Step{Argv:["sudo","scutil","--set","ComputerName",name]}` etc.
    (three sets when all differ). **No `/etc/hosts` step on macOS.**
  - **timezone**: read `systemsetup -gettimezone` (fall back to `readlink /etc/localtime` if it
    errors under non-root); if `!= tz` emit `Step{Argv:["sudo","systemsetup","-settimezone",tz]}`.
  - **locale**: **no-op** — add "locale" to `Skipped` (macOS locale owned by the shell layer).
  - **shell**: `path := resolveShellPath(...)`; `/etc/shells` guard (macOS has it too);
    read current shell via `dscl . -read /Users/<cfg.User> UserShell` (`getent` absent on
    macOS); if `!= path` emit `Step{Argv:["sudo","chsh","-s",path,cfg.User]}`.
  - **firewall** (ufw/firewalld resolve to socketfilterfw on macOS via `auto`): read
    `/usr/libexec/ApplicationFirewall/socketfilterfw --getglobalstate`; if not "State = 1"
    emit `Step{Argv:["sudo","/usr/libexec/ApplicationFirewall/socketfilterfw","--setglobalstate","on"]}`.
  - Add `var _ Configurer = (*darwinConfigurer)(nil)`.
- **MIRROR**: same struct/constructor shape as `linuxConfigurer`.
- **IMPORTS**: `fmt`, `strings`, `internal/exec`, `internal/platform`.
- **GOTCHA**: `scutil --get HostName` errors when never set — catch and treat as "unset → set".
  `LocalHostName`/`HostName` should be ASCII/no-spaces; `ComputerName` may contain spaces (do
  not reject spaces for ComputerName). macOS locale is deliberately a no-op — document it.
- **VALIDATE**: `go test ./internal/sysconfig/ -run Darwin` — hostname emits 3× `scutil --set`
  when all differ; idempotent no-op when all equal; firewall = socketfilterfw; locale skipped.

### Task 2.4: `windowsConfigurer` deferred stub — Depends on [1.2]

- **BATCH**: B2
- **ACTION**: Create `internal/sysconfig/windows.go` (+ `windows_test.go`) — a runtime stub for
  `platform.OSWindows`.
- **IMPLEMENT**: `windowsConfigurer` whose `Plan(Config) (Plan, error)` returns
  `Plan{}, ErrNotImplemented`. A doc comment documents the eventual commands (`Rename-Computer
  -NewName`, `Set-TimeZone -Id`, `Set-NetFirewallProfile`/Defender) without wiring them.
  **Runtime dispatch, NO build tag** — all sysconfig ops are argv shell-outs (no unix-only
  syscalls), so a plain `OSWindows` branch keeps `go build ./...` green everywhere
  (`sysconfig-privilege.md §5`). Add `var _ Configurer = (*windowsConfigurer)(nil)`.
- **MIRROR**: `privilege/drop_other.go` sentinel-error precedent (but runtime, not build-tag).
- **IMPORTS**: `internal/platform` (only if referenced).
- **GOTCHA**: Do NOT use build tags — that would split the package and complicate testing on
  Linux CI. The stub compiles and is exercised on `ubuntu-latest`.
- **VALIDATE**: `go test ./internal/sysconfig/ -run Windows` — `Plan` returns
  `errors.Is(err, ErrNotImplemented)` with zero steps.

### Task 3.1: `register.Default` — Depends on [2.2, 2.3, 2.4]

- **BATCH**: B3
- **ACTION**: Create `internal/sysconfig/register.go` (+ `register_test.go`) wiring the
  configurers into an `Engine`.
- **IMPLEMENT**:
  ```go
  func Default(r exec.Runner, root string) *Engine {
      fs := NewOSFileSystem(root)
      linux := newLinuxConfigurer(r, fs)
      cfgs := map[platform.OS]Configurer{
          platform.OSMacOS:   newDarwinConfigurer(r, fs),
          platform.OSDebian:  linux, platform.OSRedHat: linux, platform.OSArch: linux,
          platform.OSSUSE:    linux, platform.OSLinux:  linux, platform.OSWSL:  linux,
          platform.OSWindows: newWindowsConfigurer(),
      }
      return NewEngine(cfgs, r, fs)
  }
  ```
- **MIRROR**: `backend/register.go` `Default(...)` wiring.
- **IMPORTS**: `internal/exec`, `internal/platform`.
- **GOTCHA**: The six systemd-family OS values share one `linuxConfigurer` instance. `OSWSL`
  is treated as linux-family for parity (per `symlink-semantics.md §7.3` recommendation).
  `OSUnknown` is intentionally NOT registered → `Plan` returns `*UnsupportedOSError`.
- **VALIDATE**: `go test ./internal/sysconfig/ -run Default` — asserts every registered
  `platform.OS` dispatches to a non-nil `Configurer`; `OSUnknown` → `*UnsupportedOSError`.

### Task 4.1: `dotf sysconfig` command — Depends on [3.1, 1.1]

- **BATCH**: B4
- **ACTION**: Create `cmd/dotf/sysconfig.go` (+ `sysconfig_test.go`) — `runSysconfig(args,
  stdout, stderr, r)`.
- **IMPLEMENT**: `flag.NewFlagSet("sysconfig", flag.ContinueOnError)`, `fs.SetOutput(stderr)`.
  Flags: `--manifest`, `--hostname`, `--timezone`, `--locale`, `--shell`,
  `--firewall` (`ufw|firewalld|auto|off`), `--dry-run`, `--yes`. `fs.Parse` failure → return 2.
  `m, err := manifest.Resolve(flagManifest, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS)`
  → `reportErr` (1) on error. `info := platform.Detect(r)`. Build `cfg sysconfig.Config` via
  `resolveSysconfig(flags, m.Sysconfig, info)` — precedence flag > manifest > "" (skip);
  set `cfg.User = privilege.RealUser().Name` (fall back to `os.Getenv("USER")` when
  `RealUser` errors and euid != 0). `engine := sysconfig.Default(r, "")`.
  `plan, err := engine.Plan(cfg, info)` → `reportErr` on error (e.g. `ErrNotImplemented` on
  windows). If `--dry-run`: `printSysconfigDryRun(stdout, plan)` (greppable
  `dry-run=true` + `hostname=…`/`timezone=…`/`locale=…`/`step.<domain>=<argv>` lines) → return 0.
  If `len(plan.Steps)==0`: print `sysconfig=converged` → return 0 (**idempotent no-op path**).
  Confirmation gate (mirror `plan.go`): if `!yes`, require a TTY confirm; when stdout is not a
  TTY and `!yes`, print `sysconfig: refusing to apply without --yes (non-interactive)` to
  stderr and return 1 (safe default given firewall/hostname blast radius). Then prime sudo
  once + keep-alive (copy `install.go:62-69`), `defer cancel()`. `res, err := engine.Execute(plan)`
  → `reportErr` on error; `printSysconfigResults(stdout, res)` +
  `printSysconfigSummary(stderr, res, sudo.KeepAliveErr())`; return `sysconfigExitCode(res)`
  (1 if any `StepResult.Err != nil`, else 0 — mirror `installExitCode`).
- **MIRROR**: `install.go` (prime+keepalive, dry-run/summary split, exit code), `status.go`
  (read-only dry-run construction), `plan.go` (`reportErr`, `isTTY`, confirm gate).
- **IMPORTS**: `context`, `flag`, `fmt`, `io`, `os`, `strings`, `dotfiles`, `internal/exec`,
  `internal/manifest`, `internal/platform`, `internal/privilege`, `internal/sysconfig`.
- **GOTCHA**: **Do NOT prime sudo on the `--dry-run` or converged/no-step paths** — read-only
  paths must not trigger a password prompt (mirror `status.go:61` using `NewFakeDropper`; here,
  simply don't call `Prime`). Always `defer cancel()`. Per `go-cli-patterns.md §3` the repo
  deferred `--yes` to Phase 6 — this plan intentionally adds `--yes` NOW because (a) the issue
  #11 flag list requires it and (b) sysconfig's blast radius (hostname/firewall) warrants an
  apply gate; keep the surface to exactly these flags (no `--headless`/`--force`).
- **VALIDATE**: `go test ./cmd/dotf/ -run Sysconfig` — dry-run prints plan + mutates nothing;
  idempotent no-op path prints `sysconfig=converged` and records zero mutating calls; bad
  flag → 2; manifest resolution error → 1 with `dotf:` stderr.

### Task 4.2: Wire `sysconfig` into `main` — Depends on [4.1]

- **BATCH**: B4
- **ACTION**: Update `cmd/dotf/main.go` (switch + `usage`) and `cmd/dotf/main_test.go`.
- **IMPLEMENT**: Change `usage` to
  `"usage: dotf <version|platform|plan|install|status|diff|sysconfig> [args]"`; add
  `case "sysconfig": return runSysconfig(fs.Args()[1:], os.Stdout, os.Stderr, exec.New())` to
  the dispatch switch. Add `TestRun_Sysconfig` mirroring `TestRun_Install`: `captureOutput`
  + a `--dry-run` invocation (with a `--manifest` temp fixture or `--hostname` flag so it does
  not depend on a live host) + a regexp on stdout.
- **MIRROR**: `main.go:39-57` dispatch, `main_test.go` `TestRun_*`.
- **IMPORTS**: none new (functions already in `package main`).
- **GOTCHA**: Keep this the only task touching `main.go`/`main_test.go`, after 4.1 so
  `runSysconfig` exists. Update `usage` or the unknown-subcommand help drifts. Surface the new
  subcommand in top-level help (per `tasks/lessons.md` 2026-04-17).
- **VALIDATE**: `go test ./cmd/dotf/ && go vet ./... && go build ./...` — `sysconfig` dispatches.

### Task 5.1: Docs — Depends on [4.2]

- **BATCH**: B5
- **ACTION**: Update `CLAUDE.md`, `AGENTS.md`, `README.md`, and `tasks/todo.md`.
- **IMPLEMENT**: In `CLAUDE.md` Stack Overview / dotf section, document `dotf sysconfig`
  (owns hostname/timezone/locale/shell/firewall per-OS; flags + `[sysconfig]` manifest table;
  Windows deferred). `AGENTS.md` is sync-locked to `CLAUDE.md` (its "Sync policy" says
  CLAUDE.md is the source of truth) — add only the one-line dotf-CLI reference if the
  subcommand list is echoed there. In `README.md`, add `dotf sysconfig` to any dotf subcommand
  listing. In `tasks/todo.md`, move Phase 5 to done with a short outcome section (package,
  command, tests, ACs met). Commit docs under `docs/prps/**` with `docs(internal): …`; the
  root docs with `docs(sysconfig): …` or `feat(sysconfig): …` as part of the feature.
- **MIRROR**: existing dotf mention in `AGENTS.md:14-18` and `CLAUDE.md` Stack Overview.
- **IMPORTS**: n/a.
- **GOTCHA**: Do NOT duplicate `CLAUDE.md` content into `AGENTS.md` (its sync policy forbids
  it). Keep the manifest schema note (`[sysconfig]`, no hostname) accurate.
- **VALIDATE**: `npm run lint` (mixed shell/python/go + markdown if wired); manual read.

---

## Testing Strategy

Stdlib `testing`, table-driven, `exec.Fake` + rooted `osFileSystem(t.TempDir())`. Argv asserted
via `f.CallsSnapshot()` and `fmt.Sprint(call.Args)` compare. File steps asserted on the `Plan`'s
`FileInstall.Dest`/`Content` (deterministic, read-only) rather than the nondeterministic
`sudo cp <tmp> …` argv.

### Acceptance-criterion → concrete test mapping

| Issue #11 AC | Test (file → name) | Assertion |
| --- | --- | --- |
| **AC1** sets hostname (+`/etc/hosts`), tz, locale, shell, firewall | `linux_test.go` `TestLinux_Plan_AllDomains`; `darwin_test.go` `TestDarwin_Plan_AllDomains`; manual VM step | For a fresh-host `Config` with all five set, `Plan` emits the expected Steps: linux → `sudo hostnamectl set-hostname web01`, a `/etc/hosts` `FileInstall` containing `127.0.1.1 web01`, `sudo timedatectl set-timezone …`, locale gen+set, `/etc/shells` install + `sudo chsh -s <path> <user>`, `sudo ufw allow OpenSSH` + `sudo ufw --force enable`. |
| **AC2** re-runs idempotent no-op | `linux_test.go` `TestLinux_Plan_Idempotent`; `cmd/dotf/sysconfig_test.go` `TestRunSysconfig_ConvergedNoOp` | Seed fake reads to return the target values (`hostnamectl --static`→name, `timedatectl show`→tz, `locale -a`→target, `getent passwd`→shell path, `sudo ufw status`→"active" + OpenSSH rule) and seed the rooted fs with a correct `/etc/hosts` + `/etc/shells` → `Plan.Steps` is empty; `Execute`/command path records **zero** mutating calls (`assertReadOnlyCalls`-style). |
| **AC3a** `locale-gen` before `localectl set-locale` | `linux_test.go` `TestLinux_Locale_GenBeforeSet` | With `locale -a` missing the target and `LookPath("locale-gen")` present, the `Plan.Steps` index of the `locale-gen` Step is **strictly less than** the `localectl set-locale` Step index. |
| **AC3b** `chsh` target verified in `/etc/shells` | `linux_test.go` `TestLinux_Shell_EtcShellsGuard` | (i) shell path absent from seeded `/etc/shells` → `Plan` includes a `/etc/shells` `FileInstall` (appending the path) at an index **before** the `sudo chsh` Step. (ii) path already present → no `/etc/shells` `FileInstall`, just `chsh`. (iii) shell already current → no `chsh` at all. |
| **AC4** ufw (Arch+Debian), firewalld (Fedora), pf/socketfilterfw (macOS) | `linux_test.go` `TestLinux_Firewall_Selection` (table over `OSArch`/`OSDebian` w/ `auto`), `TestLinux_Firewall_Firewalld` (`OSRedHat`/`OSSUSE`), `darwin_test.go` `TestDarwin_Firewall_socketfilterfw` | `OSArch`+`OSDebian` + `auto` → `ufw` argv (+ Arch install via `pacman`, Debian via `apt-get`); `OSRedHat`+`OSSUSE` → `firewall-cmd`/`systemctl … firewalld` argv; `OSMacOS` → `socketfilterfw --setglobalstate on`. Exact argv per branch. |

### Additional unit tests

| Test | Input | Expected | Edge? |
| --- | --- | --- | --- |
| hostname macOS 3× set | `Config{Hostname}`, all three `scutil --get` differ | three `sudo scutil --set {ComputerName,LocalHostName,HostName}` Steps | No |
| hostname macOS HostName unset | `scutil --get HostName` returns error | treated as unset → set Step emitted (no panic) | Yes |
| `/etc/hosts` render | existing `127.0.1.1 old` line | `renderHosts` replaces (not duplicates); `127.0.0.1 localhost` preserved | Yes |
| locale fallback | requested unavailable, no `locale-gen` | falls back to `C.UTF-8`, still emits `set-locale LANG=C.UTF-8` | Yes |
| locale normalization | `en_US.UTF-8` vs `en_us.utf8` in `locale -a` | treated as available (no gen) | Yes |
| ufw status read uses sudo | firewall plan | the `ufw status` READ argv[0] == `sudo` (unlike other reads) | Yes |
| chsh never dropped / targets real user | `Config{Shell,User:"yandy"}` | `sudo chsh -s <path> yandy`; no `privilege.Dropper` invoked | Yes |
| timezone invalid | tz whose zoneinfo file is absent | wrapped error, no `set-timezone` Step | Yes |
| windows deferred | `OSWindows` | `Plan` → `errors.Is(err, ErrNotImplemented)`, zero steps | Yes |
| unknown OS | `OSUnknown` | `*UnsupportedOSError` | Yes |
| dry-run zero mutation | command `--dry-run` | greppable plan on stdout; `CallsSnapshot` has no `sudo`/mutating argv; no temp files written | Yes |
| non-TTY apply without `--yes` | command, non-TTY stdout, no `--yes` | exit 1, `dotf: …refusing…` stderr, zero mutating calls | Yes |
| non-fatal step failure | `Execute` with one Step erroring (`RunErrors`) | remaining Steps still run; `Result` records the failure; exit 1 | Yes |

### Edge cases checklist

- [ ] Missing `locale-gen` → `C.UTF-8` fallback, still set.
- [ ] `scutil --get HostName` "not set" error handled as unset.
- [ ] `sudo ufw status` read keeps the sudo prefix; all other reads have none.
- [ ] `chsh` targets `RealUser().Name` as root, never dropped, never root's shell.
- [ ] Converged host → empty `Plan` → `Execute` runs nothing (idempotency).
- [ ] `--dry-run` and converged paths never call `sudo.Prime` (no password prompt).
- [ ] Windows `Plan` → `ErrNotImplemented`, `go build ./...` green on all platforms (no build tags).
- [ ] Root-conditional tests `t.Skip` on the wrong euid; non-root Linux path fully covered (CI).

---

## Validation Commands (the full local gate)

Run from the repo root. All must be clean before "done" (baseline is currently clean per
`test-ci-conventions.md §1`).

```bash
# 1. Build everything.
go build ./...

# 2. Vet — CI step "Vet".
go vet ./...

# 3. Race-enabled tests — CI step "Test" (authoritative).
go test -race ./...
#    Targeted while iterating:
go test -race ./internal/sysconfig/ ./internal/manifest/ ./cmd/dotf/

# 4. Lint — CI step "Lint" (golangci-lint v2.12.2, default: standard + revive).
golangci-lint run ./...        # or: bin/lint.sh --go

# 5. Format — must produce NO diff (lefthook gofmt + goimports path).
gofmt -l . ; test -z "$(gofmt -l .)"
goimports -l .                 # or: bin/format.sh --go

# 6. Full mixed-language lint (pre-push runs this).
npm run lint
```

Bash-untouched guard (parallel-then-cutover):

```bash
git diff --stat -- bootstrap.sh 'bin/install-*.sh'   # EXPECT: empty
```

Manual VM validation (Phase 5 signal for AC1/AC2):

- [ ] `go run ./cmd/dotf sysconfig --hostname web01 --timezone America/New_York --locale en_US.UTF-8 --shell zsh --firewall auto --dry-run` — prints the plan, mutates nothing.
- [ ] On a scratch VM: same command with `--yes` sets all five; `hostnamectl`/`timedatectl`/`localectl`/`getent passwd`/`ufw status` reflect the values; `/etc/hosts` has `127.0.1.1 web01`.
- [ ] Re-run the same `--yes` command → `sysconfig=converged`, zero changes (idempotent).

---

## Acceptance Criteria

- [ ] All tasks (1.1–5.1) completed; all validation commands green.
- [ ] `internal/sysconfig` owns hostname/timezone/locale/shell/firewall per-OS, dispatched
      purely on `platform.OS` via `map[platform.OS]Configurer` (no scattered OS ifs at the call site).
- [ ] **AC1**: a full `Config` produces the expected set Steps on Linux and macOS (incl.
      `/etc/hosts` on Linux, 3× `scutil --set` on macOS); proven on a VM.
- [ ] **AC2**: a second `Plan` on a converged host is empty; the command/`Execute` records zero
      mutating commands (idempotent no-op) — asserted in tests.
- [ ] **AC3**: `locale-gen` Step precedes `localectl set-locale`; `chsh` only emitted after the
      target path is guaranteed present in `/etc/shells` — both asserted by Step ordering tests.
- [ ] **AC4**: firewall = ufw on Arch+Debian, firewalld on Fedora/SUSE, socketfilterfw on macOS
      — table test asserts exact argv per branch.
- [ ] Every (OS × domain) mutating command asserted by **exact argv** (`FakeCall.Name`+`Args`),
      no substring/wildcard matches.
- [ ] All privileged mutations route through `exec.Runner` with a `sudo` prefix; reads use
      `r.Output`/`fs.ReadFile` (only `ufw status` keeps sudo). No `os/exec` in production code.
- [ ] `chsh` runs as root targeting `RealUser().Name`; `privilege.Dropper` is never used.
- [ ] Windows = deferred stub returning `ErrNotImplemented`; `go build ./...` green on all OSes
      (no build tags).
- [ ] `dotf sysconfig` wired into `main.go` with `--hostname --timezone --locale --shell
      --firewall --dry-run --yes` and `--manifest`; `--dry-run`/converged paths never prime sudo.
- [ ] Manifest `[sysconfig]` table added additively (no hostname field); embedded default is
      commented; bad `firewall` value → `*ValidationError`.
- [ ] Bash untouched (`git diff` empty for `bootstrap.sh`/`bin/install-*.sh`); no lint/type errors.
- [ ] Docs updated (CLAUDE.md/AGENTS.md/README.md + `tasks/todo.md`).

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
| --- | --- | --- | --- |
| Commands can't be safely run in CI/unit tests (would mutate the runner) | High | High | **ALL mutation goes through the injected `exec.Runner`; reads through `fs FileSystem` rooted at `t.TempDir()`. Tests assert argv/`FileInstall` on the `Plan` and `Fake` calls — never real syscalls.** No `os/exec` in production sysconfig code. |
| `/etc/hosts`/`/etc/shells`/`/etc/locale.gen` edits via `sudo tee` don't fit the `Runner` seam (`Run` wires real stdin) | Med | Med | Model file edits as a `FileInstall` step: render in-memory (`fs.WriteTemp`, 0600) then `sudo cp <tmp> <dest>` (pure argv, testable); back up first (`sudo cp <dest> <dest>.dotf.bak`). |
| Locale set before generate → `set-locale` fails / wrong locale | Med | High | Structural ordering in `Plan.Steps` (gen Steps before set Step) + `TestLinux_Locale_GenBeforeSet` asserting the index invariant; `C.UTF-8` fallback when `locale-gen` absent. |
| `chsh` accidentally dropped to the user (re-introduces PAM prompt) or changes root's shell | Med | High | `chsh -s <path> <user>` as **root** targeting `RealUser().Name`; never `privilege.Dropper`; test asserts no dropper invoked and the user arg is the real user. |
| `sudo ufw status` read fails without root (netfilter) | Med | Med | Keep the sudo prefix on that specific READ (unlike other reads); test asserts argv[0]==`sudo`. |
| Applying firewall unattended locks out SSH | Med | High | Always emit the SSH-allow Step (`ufw allow OpenSSH` / `firewall-cmd --add-service=ssh`) **before** enable; non-TTY apply requires `--yes` (safe default). |
| `--yes` added despite `go-cli-patterns.md §3` deferring it to Phase 6 | Low | Low | Intentional per issue #11 flag list + sysconfig blast radius; surface strictly limited to the issue's flags (no `--headless`/`--force`); noted in Task 4.1. |
| Adding `[sysconfig]` manifest section is scope creep | Low | Low | Additive struct only; embedded default commented (no behavior change); required anyway by `DisallowUnknownFields` before any table; hostname stays flag-only. |
| Windows branch breaks cross-compile | Low | Med | Runtime dispatch (no build tags); stub returns `ErrNotImplemented`; exercised on `ubuntu-latest`. |
| Reading real `/etc/*` during production `Plan` differs from test root | Low | Low | `osFileSystem{root}` — production `root=""`, tests `root=t.TempDir()`; only reads + temp-write use `fs`, so behavior is identical modulo the root prefix. |

## Notes

- **Greenfield inside the Go tree**: `internal/sysconfig` is net-new. Hostname/timezone/localectl
  are genuinely new capabilities (no bash precedent); only firewall (`ufw`/`firewalld`), `chsh`,
  and `/etc/shells` pre-exist in `bootstrap.sh` (see `sysconfig-privilege.md §7`).
- **Naming**: command is `sysconfig`, package is `internal/sysconfig` — 1:1 command↔package
  parity (`go-cli-patterns.md §15`). `config` was rejected (overloaded in a dotfiles tool).
- **The exec seam takes `exec.Runner`, not `exec.Execer`** — there is no `Execer` in this repo
  (`go-cli-patterns.md` naming correction).
- **Privilege convention**: privileged mutations hardcode/build a `sudo` argv prefix and run
  through `exec.Runner` (backends discard the `*Sudo` object — `register.go` does `_ = sudo`);
  the command layer alone primes + keep-alives `*Sudo` once. sysconfig mirrors this exactly and
  never uses the `Dropper` (every op wants root).
- **Apply ordering**: Phase 5's standalone command runs the five domains in canonical order.
  Phase 6's combined `apply` will sequence firewall/ssh AFTER packages/symlinks; `Step.Domain`
  tagging lets it partition the plan without an API change (PRD L107, `sysconfig-privilege.md §2`).
- **Confidence: 8/10.** High on the interface/dispatch shape, exec/fs test seams, and per-domain
  argv (all grounded in existing code + the research tables). The −2 is residual: exact macOS
  `systemsetup`/`dscl` read output shapes and the Arch `/etc/locale.gen` edit are best confirmed
  on a real VM during implementation.
- **Research backstops**: `docs/prps/plans/.prp-research/phase-4-5-symlink-sysconfig/{sysconfig-privilege,go-cli-patterns,test-ci-conventions,symlink-semantics}.md`.
