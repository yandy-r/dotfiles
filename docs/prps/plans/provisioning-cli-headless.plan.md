# Plan: `dotf apply` — headless orchestration + flag/env parity, snapper, failure summary (Phase 6)

## Summary

Add a top-level `dotf apply` subcommand that orchestrates the existing engines
(`sysconfig` → packages → symlinks → firewall) in the PRD's canonical order, with
**headless/`--force` parity** with `bootstrap.sh` (flags **and** env equivalents), an
opt-in **snapper** pre-apply snapshot, and a **run-wide non-fatal failure accumulator**
that emits one end-of-run summary and one exit code. This is the Phase 6 integration
barrier: it composes Phases 2–5 (already merged) without reshaping any of their APIs.

Two tiny new internal packages carry the net-new mechanisms — `internal/failure`
(cross-domain accumulator, #14) and `internal/snapper` (snapshot, #13) — each
single-responsibility and unit-testable in isolation against `exec.Fake`. All
orchestration and flag/env resolution stays in `package main` (`cmd/dotf`), reusing the
helpers already there (`resolveSysconfig`, `packagesForInstall`, `realUserName`,
`resolveRepoRoot`, `firstNonEmpty`, `reportErr`, `installResult`, the `printX*Results`
stdout printers) with **zero extraction**.

This is **parallel-then-cutover**: `bootstrap.sh` and `bin/install-*.sh` stay authoritative
and untouched. `dotf apply` is dogfooded but is not wired into `bootstrap.sh` — the shim
cutover is Phase 7. A `git diff` guard verifies bash is unmodified.

Closes Forgejo **#12** (headless + flag/env parity), **#13** (snapper parity), **#14**
(failure-summary accumulation), and their tracking parent **#21**.

## User Story

> As someone provisioning a fresh VM headlessly, I run
> `dotf apply --headless --snapper` (or `NON_INTERACTIVE=true dotf apply`) and the tool
> takes a snapper snapshot, then runs system config, package installs, symlinks, and
> firewall in one ordered pass with **no prompts**. If three packages fail, it does not
> abort — it finishes the run and prints one summary (`apply summary: steps=N ok=… failed=3`)
> listing every failure, and exits non-zero. On a host without snapper (or non-Linux),
> `--snapper` is a silent no-op. A `--dry-run` composes the same plan and mutates nothing.

## Problem → Solution

Phases 1–5 delivered the composable pieces (`plan`, `install`, `status`, `diff`, `link`,
`sysconfig`) but there is **no top-level orchestration command** — `cmd/dotf/plan.go:21`
even reserves the name: *"It applies nothing to the host — Phase 6 owns apply."* There is
also no headless/env parity surface, no snapper equivalent, and no **run-wide** failure
accumulator (each of `install`/`sysconfig` has its own per-command one). Phase 6 delivers
`dotf apply` sequencing the engines in PRD order (`provisioning-cli.prd.md:107`) behind a
headless-vs-guided gate with full `bootstrap.sh` flag/env parity, plus the two net-new
mechanisms (#13 snapper, #14 accumulator).

## Metadata

| Field          | Value                                                                                          |
| -------------- | ---------------------------------------------------------------------------------------------- |
| Issues         | Forgejo #21 (tracking) closing #12 (headless+parity, Large), #13 (snapper, Small), #14 (failure summary, Small) |
| Module         | `github.com/yandy-r/dotfiles`, Go 1.26                                                         |
| Commit scope   | `feat(dotf): …` (apply/headless), `feat(snapper): …` (#13), `feat(failure): …` (#14); docs under `docs/prps/**` → `docs(internal): …` |
| Depends on     | Phases 2 (manifest/TUI), 3 (backend/privilege), 4 (symlink), 5 (sysconfig) — all merged to `main` |
| New packages   | `internal/failure`, `internal/snapper`                                                         |
| New subcommand | `dotf apply`                                                                                    |
| Command name   | **`apply`** (pre-named at `plan.go:21`; idiomatic converge verb — see Decision below)           |

## Batches

| Batch | Tasks              | Can run in parallel | Rationale                                                       |
| ----- | ------------------ | ------------------- | -------------------------------------------------------------- |
| B1    | 1.1, 1.2, 1.3      | yes (disjoint files) | Accumulator, snapper, and pure flag/env resolver compile standalone |
| B2    | 2.1                | —                   | `runApply` orchestration consumes 1.1/1.2/1.3                   |
| B3    | 3.1                | —                   | `main.go` dispatch after `runApply` exists                     |
| B4    | 4.1, 4.2 (opt)     | 4.2 optional        | Docs; optional DRY refactor is independent                     |

---

## Decision: command name + package layout (stated plainly)

- **Command name: `apply`.** Not a close call. `cmd/dotf/plan.go:21` already reserves it
  (*"Phase 6 owns apply"*); it is the idiomatic converge verb (`terraform apply`,
  `kubectl apply`); and it avoids the collisions the alternatives carry: `run` collides
  with `main.go:run()` and `exec.Runner.Run`; `bootstrap` collides with `bootstrap.sh`
  during the parallel window; `provision` is accurate but longer and unclaimed.
- **Orchestration lives in `package main`** across two focused files
  (`cmd/dotf/apply.go`, `cmd/dotf/applyopts.go`). The composition helpers
  (`resolveSysconfig`, `packagesForInstall`, `realUserName`, `resolveRepoRoot`,
  `firstNonEmpty`, `reportErr`, `installResult`, `printSysconfigResults`,
  `printInstallResults`, `printLinkResults`, `groupPackagesByBackendSlice`,
  `sortedBackendKeys`, `sysconfigStepValue`, `linkActionLines`) are **already in
  `package main`** — moving orchestration to an `internal/provision` package would force
  extracting all of them for no payoff and would be the lone subcommand not tested via
  `captureOutput`+`exec.Fake`. `exec.Fake` testability is fully preserved: the three
  mutating engine steps (sysconfig, packages, firewall) run through `exec.Runner`; the
  symlink step uses direct `os.*` and is tested with `t.Setenv("HOME"/"DOTFILES_ROOT",
  t.TempDir())` exactly like `link_test.go`.
- **Two net-new `internal/` packages** are worth carving out because each is a
  self-contained, single-responsibility mechanism reusable beyond apply:
  - `internal/failure` — cross-domain accumulator (#14): `Collector` type, ~60 lines.
  - `internal/snapper` — snapshot (#13): `Snapshotter` type, ~70 lines.
- **No new firewall subsystem.** Firewall is already a `sysconfig` domain
  (`sysconfig.Config.Firewall` + `linuxConfigurer.planFirewall/planUfw`). The PRD's
  "firewall/ssh last" step is a **second, firewall-only `sysconfig` pass** reusing the same
  engine instance — not a new package.
- **Sudo is primed ONCE for the whole run** (one `Prime` + one `KeepAlive`), a deliberate
  divergence from the per-subcommand priming in `install.go`/`sysconfig.go`. No engine
  calls `Prime` itself; every engine emits literal `sudo …` argv kept warm by the single
  keep-alive.

---

## Flag / env parity table (carried from research; the #12 contract)

Precedence: **string knobs** = flag > env > manifest > default; **boolean "enable" knobs**
= logical OR of "flag present" and "env truthy" (either turns the mode ON; no env force-off,
mirroring bootstrap). **Truthy = exactly `"true"`** (matches bootstrap's `== "true"` checks;
`"1"`/`"yes"`/`"TRUE"` do **not** flip a knob — locked by test).

| `dotf apply` flag        | Type      | Env equivalent(s)                                   | Effect / phase                                                                                          |
| ------------------------ | --------- | --------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| `--headless`             | bool      | `NON_INTERACTIVE=true`                              | Skip the TUI + all prompts; apply the resolved intent set with zero interaction.                       |
| `--force`                | bool      | `FORCE_INSTALL=true`                                | Implies `--headless`; also sets `symlink.Options.Force`. (`HOMEBREW_NO_ASK`/`NONINTERACTIVE` flow to child shellouts via inherited process env.) |
| `--gaming`               | bool      | `ENABLE_GAMING=true`                               | Adds the existing `gaming` intent → packages phase. (Arch-scoped: its package is `aur`.)               |
| `--enable-ollama`        | bool      | `ENABLE_OLLAMA=true`                               | Adds the `ollama` intent name → packages phase. **Degrades to a no-op selection** until the manifest defines an `ollama` intent (see Open Questions G1). |
| `--linux-desktop`        | bool      | `LINUX_DESKTOP=true`                               | `symlink.Options.ManageLinuxDesktop=true` **and forces `Backup=true`** (the engine guard requires DryRun or Backup) → symlinks phase. |
| `--snapper`              | bool      | `SNAPPER=true`                                     | Snapper pre-apply snapshot (Linux-only, no-op without the binary) → snapshot phase.                    |
| `--homebrew=true\|false` | string    | *(none; flag-only tristate; `""` = OS default)*    | Accepted for surface parity. macOS already installs via `PkgMgrHomebrew`; on Linux `--homebrew=true` is an accepted no-op with a one-line warning (Homebrew-on-Linux is not modeled). |
| `--intent <name>`        | []string  | *(none)*                                            | Repeatable; **narrows** the headless intent set to exactly these (mirrors `install.go`'s `--intent`). Guided mode ignores it (TUI drives selection). |
| `--dry-run`              | bool      | *(none)*                                            | Compose each engine's preview; **never prime sudo**, mutate nothing; exit 0.                           |
| `--manifest <path>`      | string    | `DOTF_MANIFEST`                                     | `manifest.Resolve` override (parity with every other subcommand).                                      |

**Env honored but not a flag:** `DOTFILES_ROOT` (repo root via `resolveRepoRoot`, symlink
source + `custom-script`), `DOTF_MANIFEST` (manifest override). Detection env
(`DOTFILES_OS`/`ARCH`/`PKG_MGR`…) is **produced** by `platform.Detect(r)`, never read.

**Interactive-only bootstrap flags with no apply flag:** `--interactive` maps to apply's
**default guided (TUI) mode** on a TTY; its per-section pause is a bash-ism folded into the
`--snapper` snapshot. Not reproduced.

---

## Ordered apply pipeline (what each phase calls)

PRD order (`provisioning-cli.prd.md:107`):
`sysconfig (hostname/tz/locale/shell) → packages → symlinks → firewall/ssh → summary`.
Firewall is split into a **second sysconfig pass** to honor "firewall last." Sudo is primed
once before Phase 1.

```
runApply(args, stdout, stderr, r):
  P0  parse flags (ContinueOnError) → exit 2 on parse error
      opts := resolveApplyOptions(flags, os.Getenv)          // flag > env  (applyopts.go)
      validate --homebrew ∈ {"","true","false"}              → exit 2 on bad value
      m := manifest.Resolve(flagManifest, DOTF_MANIFEST, …)  → reportFatal (3)
      info := platform.Detect(r)
      home := os.UserHomeDir(); repoRoot := resolveRepoRoot() → reportFatal (3) if either fails
      fwMode := manifest firewall (default "" = skip); ValidFirewallMode → exit 2 on bad value
      mode gate:
        if opts.DryRun            → headless-style preview (below), exit 0   [never prime sudo]
        else if opts.Headless     → intents := resolveIntents(m, opts)
        else if stdout is a TTY    → sel := tui.Run(m, info, accessible);
                                     if !sel.Confirmed → "apply not confirmed", exit 1
                                     intents := sel.Intents
        else                      → "refusing to apply without --headless/--force
                                     (non-interactive)", reportFatal (3)
  P0b (apply only) sudo := privilege.NewSudo(); sudo.Prime(r) → reportFatal (3)
      cancel := sudo.KeepAlive(ctx, r); defer cancel()
  P1  snapshot (opts.Snapper): snap := snapper.New(r, info)
        if !snap.Available() → warn to stderr (requested but unavailable)
        else out := snap.Create("pre-apply"); if out.Err → warn to stderr   [NOT recorded in coll]
      var coll failure.Collector
  P2  sysconfig identity: cfg := resolveSysconfig("","","","","", m.Sysconfig)
        cfg.Firewall = sysconfig.FirewallUnset; cfg.User = realUserName()
        eng := sysconfig.Default(r, "")
        plan,err := eng.Plan(cfg, info); if err → coll.Record("sysconfig", err)
        else res,_ := eng.Execute(plan); for each sr → coll.Record("sysconfig:"+sr.Step.Domain, sr.Err)
  P3  packages: pkgs := packagesForInstall(m, intents)
        engine := backend.Default(r, sudo, privilege.NewDropper(), repoRoot)
        for pkg → err := engine.Install(pkg, info); coll.Record("packages:"+pkg.Name, err)
                  build installResult{name,backend,status,err} for the stdout printer
  P4  symlinks: se := symlink.NewEngine(repoRoot, home); specs := symlink.DefaultTable()
        sopts := symlink.Options{Backup: opts.LinuxDesktop, Force: opts.Force,
                                 Validate: true, ManageLinuxDesktop: opts.LinuxDesktop}
        plan,err := se.Plan(specs, info, sopts); if err → coll.Record("symlinks", err)
        else report,_ := se.Apply(plan, sopts)
             _,verr := se.Validate(specs, info, sopts); coll.Record("symlinks", verr)  // nil when clean
  P5  firewall (2nd sysconfig pass): fwCfg := sysconfig.Config{Firewall: fwMode, User: realUserName()}
        plan,err := eng.Plan(fwCfg, info); if err → coll.Record("sysconfig", err)
        else res,_ := eng.Execute(plan); for each sr → coll.Record("sysconfig:"+sr.Step.Domain, sr.Err)
  P6  summary: printSysconfigResults / printInstallResults / printLinkResults → stdout (greppable)
        coll.WriteSummary(stderr, progName); if sudo.KeepAliveErr() != nil → append a line
        return coll.ExitCode()        // 0 or 1
```

Notes: firewall runs **after** packages so ufw/firewalld (if the operator declares them, or
the linux configurer installs ufw) are present; sudo primed in P0b makes the firewall
`sudo -n ufw status` idempotency probe succeed so a converged host reports converged. When
`m.Sysconfig` leaves firewall unset (the shipped default), P5 produces an empty plan (no-op)
— **apply does not invent a firewall/shell default**; those domains are opt-in via the
manifest `[sysconfig]` table, exactly like `dotf sysconfig` (see Open Questions).

---

## Exit-code taxonomy (deliverable #2 of #14)

| Code | Meaning                                                                             | Detect                                            |
| ---- | ---------------------------------------------------------------------------------- | ------------------------------------------------- |
| 0    | All steps applied/converged; `failed=0`                                            | `apply summary: … failed=0`                       |
| 1    | **Completed-with-failures (soft):** ran to the end, ≥1 non-fatal step failed       | `apply summary: … failed=N>0` on stderr           |
| 2    | **Usage error:** flag parse or invalid `--homebrew`/firewall value                 | single `dotf:`/flagset diagnostic, no summary     |
| 3    | **Hard abort:** a precondition failed (manifest resolve, home/repo resolve, sudo prime, non-TTY refusal) | single `dotf:` diagnostic, no summary |

Code **3** is an **apply-only additive extension** over the sibling 0/1/2 convention, added
via a local `reportFatal(stderr, err) int` (returns 3) so hard aborts are distinguishable
from soft failures in CI. Existing subcommands are unchanged. This is the one deliberate
exit-code divergence and is documented in `apply.go` + `tasks/lessons.md`.

---

## Files to Change

| File                                         | Action | Justification                                                                                    |
| -------------------------------------------- | ------ | ------------------------------------------------------------------------------------------------ |
| `internal/failure/collector.go`              | CREATE | `Collector` + `Failure{Step,Err}` + `Record`/`Failed`/`Failures`/`WriteSummary`/`ExitCode` (#14) |
| `internal/failure/collector_test.go`         | CREATE | Unit tests: all-success, accumulate-all, nil-ignored, defensive-copy                              |
| `internal/snapper/snapper.go`                | CREATE | `Snapshotter` + `New` + `Available` + `Create(desc) Outcome` (#13)                                |
| `internal/snapper/snapper_test.go`           | CREATE | Table argv: Linux create, non-Linux no-op, missing-binary no-op, Run-error non-fatal, custom desc, `isLinux` table |
| `cmd/dotf/applyopts.go`                       | CREATE | `applyFlags`, `applyOptions`, `resolveApplyOptions(f, getenv)`, `resolveIntents(m, opts)`, `envTrue` |
| `cmd/dotf/applyopts_test.go`                  | CREATE | Flag/env parity matrix (injected `getenv`), precedence, truthiness lock, intent resolution        |
| `cmd/dotf/apply.go`                           | CREATE | `runApply` + phase helpers + `printApplyDryRun` + `printApplyResults` + `reportFatal`             |
| `cmd/dotf/apply_test.go`                      | CREATE | Orchestration table: dry-run, headless happy path + ordering, failure accumulation, snapper, fatal aborts |
| `cmd/dotf/main.go`                            | UPDATE | Add `case "apply"` to the switch; add `apply` to `usage`                                          |
| `cmd/dotf/main_test.go`                       | UPDATE | `TestRun_Apply` dispatch smoke (`captureOutput` + `apply --dry-run`, hermetic HOME/DOTFILES_ROOT) |
| `internal/manifest/types.go` (or `load.go`)  | UPDATE | **(optional, Task 4.2)** add `(*Manifest).PackagesForIntents`; rewrite `packagesForInstall` + `tui.PackagesForIntents` to delegate (DRY) |
| `docs/prps/plans/provisioning-cli-headless.plan.md` | (this) | The plan                                                                                    |
| `docs/prps/prds/provisioning-cli.prd.md`     | UPDATE | Flip Phase 6 status → complete; link this plan                                                    |
| `CLAUDE.md`, `AGENTS.md`, `README.md`        | UPDATE | Document `dotf apply` in the subcommand list / Stack Overview                                     |
| `tasks/todo.md`, `tasks/lessons.md`          | UPDATE | Mark Phase 6 done + outcome; record the exit-code-3 and manifest-driven-firewall decisions       |

---

## NOT Building (explicit non-goals)

- **Guided-TUI polish beyond wiring.** The guided path reuses `tui.Run`/`plan.go`'s exact
  wiring verbatim; no new TUI steps, styling, or flows. Headless is the priority path.
- **Windows apply.** `sysconfig` already returns `ErrNotImplemented` on Windows; snapper is
  Linux-only. No Windows provisioning is added.
- **Package removal / reconciliation.** Additive-install only, matching the PRD's additive
  stance and Phases 3–5. `apply` never removes packages or tears down a firewall.
- **SSH server enablement (G7).** bootstrap installs `openssh-server` + `ssh-keygen -A` +
  `systemctl enable --now sshd`; `internal/sysconfig` only opens the firewall port. There is
  no ssh module — SSH server enablement is **out of Phase 6 scope**. The `Collector`
  accommodates a future `Record("ssh", …)`; flagged as an Open Question.
- **`ollama` manifest content / `bin/install-ollama.sh` (G1).** `--enable-ollama` is wired
  (parity surface), but adding the `ollama` intent/package and porting bootstrap's
  install+enable shell is deferred; the flag degrades to a no-op selection today
  (`packagesForInstall` silently skips unknown intents). Open Question.
- **Homebrew-on-Linux provisioning (G2).** `--homebrew=` is accepted for surface parity;
  installing linuxbrew is a separate future feature.
- **`bootstrap.sh` shim cutover.** Wiring `apply` into `bootstrap.sh` is Phase 7. Bash stays
  authoritative and untouched (`git diff` guard).
- **Per-section snapshots.** Headless is one pass; a single pre-apply snapshot is the correct
  granularity (bootstrap's per-section snapshots were an `--interactive` affordance). A
  linked snapper pre/post pair is noted as a future enhancement, not built.
- **testscript CLI goldens.** No `rogpeppe/go-internal` dep exists; the PRD lists it as a
  "Should," not a "Must." Deferred to Phase 7. Coverage uses the existing
  `captureOutput`+`exec.Fake` pattern (zero new deps).

---

## Step-by-Step Tasks

### Task 1.1: `internal/failure` accumulator (#14) — Depends on [none]

- **BATCH**: B1
- **ACTION**: Create `internal/failure/collector.go` (+ `collector_test.go`) — a tiny shared
  run-level accumulator so `apply` reports every failure at the end and returns one exit code,
  mirroring `BOOTSTRAP_FAILURES`. Engines keep their own domain `Result` types unchanged.
- **IMPLEMENT**:
  ```go
  // Package failure accumulates non-fatal step outcomes across a multi-step
  // provisioning run so dotf apply reports every failure at the end instead of
  // aborting on the first, mirroring bootstrap.sh's BOOTSTRAP_FAILURES.
  package failure

  type Failure struct { Step string; Err error }   // Step: "packages:rg","sysconfig:hostname","symlinks"

  type Collector struct { total int; failures []Failure }   // zero value ready; NOT concurrency-safe

  func (c *Collector) Record(step string, err error)  // nil err = success (counts ok); non-nil accumulates
  func (c *Collector) Failed() bool
  func (c *Collector) Failures() []Failure             // defensive copy, run order
  func (c *Collector) WriteSummary(w io.Writer, prog string)  // always prints the aggregate line, then one line/failure
  func (c *Collector) ExitCode() int                   // 1 if any failure else 0
  ```
  `WriteSummary` line format mirrors `printInstallSummary`:
  `"%s: apply summary: steps=%d ok=%d failed=%d\n"` then per failure `"%s: %s: %v\n"`
  (`prog`, `f.Step`, `f.Err`). `Record` is called once per **sub-outcome** (one per sysconfig
  domain, one per package, one for the symlink phase, one per firewall step) so `steps=`
  matches `BOOTSTRAP_FAILURES` granularity. Snapper outcomes are **never** `Record`ed (pure
  warnings, parity with bootstrap's `log_warning`).
- **MIRROR**: `install.go` `printInstallSummary`/`installExitCode` (stderr style, sorting is
  by run order here — do NOT sort, preserve insertion order to match bootstrap).
- **IMPORTS**: `fmt`, `io`.
- **GOTCHA**: `Failures()` MUST return a defensive copy. The aggregate line prints even on
  full success (mild, intentional upgrade over bootstrap's silence-on-zero). Do not fold
  snapper/keep-alive into `steps=`.
- **VALIDATE**:
  ```bash
  go build ./internal/failure/
  go test ./internal/failure/
  ```
  Tests: `TestCollector_AllSuccess` (3× nil → `Failed()==false`, `ExitCode()==0`, summary
  `steps=3 ok=3 failed=0`, no per-failure lines); `TestCollector_AccumulatesAll`
  (`Record("a",nil)`,`Record("b",errB)`,`Record("c",errC)` → `Failures()==[b,c]` in order,
  `ExitCode()==1`, summary `steps=3 ok=1 failed=2` + a line per failure);
  `TestCollector_NilErrIgnored`; `TestCollector_FailuresIsCopy` (mutating the returned slice
  does not affect the collector).

### Task 1.2: `internal/snapper` snapshot (#13) — Depends on [none]

- **BATCH**: B1
- **ACTION**: Create `internal/snapper/snapper.go` (+ `snapper_test.go`) — a single-shot
  pre-apply snapshot mechanism over `exec.Runner`, Linux-guarded, `LookPath`-guarded,
  non-fatal. Mirrors `bootstrap.sh:create_snapper_snapshot` (single `snapper create -d`).
- **IMPLEMENT**:
  ```go
  // Package snapper takes a best-effort snapper rollback snapshot before a dotf
  // apply run. It is Linux-only and a silent no-op when snapper is unavailable,
  // mirroring bootstrap.sh's create_snapper_snapshot guards. Failures are
  // non-fatal: returned for the caller to log as a warning, never aborting apply.
  package snapper

  const binary = "snapper"

  type Snapshotter struct { r exec.Runner; info platform.Info }
  func New(r exec.Runner, info platform.Info) *Snapshotter

  // Available reports Linux-family OS AND a resolvable snapper binary.
  func (s *Snapshotter) Available() bool

  // Outcome is the non-fatal result of a Create call.
  type Outcome struct { Created, Skipped bool; Reason string; Err error }

  // Create runs `sudo snapper create -d "dotf apply: <desc>"`. It is a no-op
  // (Skipped) when Available is false; a snapper failure yields Err (non-fatal).
  func (s *Snapshotter) Create(desc string) Outcome
  ```
  `Create`: not Linux-family → `Outcome{Skipped:true, Reason:"non-linux"}`; `LookPath(binary)`
  fails → `Outcome{Skipped:true, Reason:"snapper-not-installed"}`; else
  `s.r.Run("sudo","snapper","create","-d","dotf apply: "+desc)`; on error →
  `Outcome{Err: fmt.Errorf("snapper: create snapshot: %w", err)}`; else `Outcome{Created:true}`.
  `isLinux(os platform.OS) bool` mirrors `sysconfig.Default`'s Linux-family set
  (`OSDebian/OSRedHat/OSArch/OSSUSE/OSLinux/OSWSL`); macOS/Windows/Unknown → false.
- **MIRROR**: `sysconfig.Default`'s Linux-family membership; `exec.Fake` argv-assertion style
  from `internal/sysconfig` tests. `LookPath` is not recorded in `Calls`, so `Available()`'s
  probe never pollutes argv assertions.
- **IMPORTS**: `fmt`, `internal/exec`, `internal/platform`.
- **GOTCHA**: `Run` (not `Output`) — nothing to capture. The single-shot `create -d` matches
  bootstrap parity; the argv is `["sudo","snapper","create","-d","dotf apply: <desc>"]`.
  Non-Linux and missing-binary MUST record **zero** calls (guard before any `Run`).
- **VALIDATE**:
  ```bash
  go build ./internal/snapper/
  go test ./internal/snapper/
  ```
  Tests (table-driven, `exec.NewFake()`, exact argv via `CallsSnapshot()`+`fmt.Sprint`):
  `TestCreate_LinuxWithBinary` (`AddPath("snapper",…)` → `Created==true`, one call
  `sudo snapper create -d "dotf apply: pre-apply"`); `TestCreate_NonLinuxNoOp` (macOS →
  `Skipped==true`, zero calls, `Available()==false`); `TestCreate_MissingBinary` (linux, no
  `AddPath` → `Skipped==true`, zero calls); `TestCreate_RunErrorNonFatal`
  (`RunErrors[CommandKey("sudo","snapper","create","-d","dotf apply: pre-apply")]=err` →
  `Outcome.Err != nil`, one call); `TestCreate_CustomDesc` (desc threads into argv);
  `TestIsLinux` (true for the 6 Linux-family values, false for macOS/Windows/Unknown).

### Task 1.3: `cmd/dotf/applyopts.go` flag/env resolution (#12 core) — Depends on [none]

- **BATCH**: B1
- **ACTION**: Create `cmd/dotf/applyopts.go` (+ `applyopts_test.go`) — the pure flag+env
  resolver and intent selector, testable with an injected `getenv` and no process env.
- **IMPLEMENT**:
  ```go
  type applyFlags struct {
      Manifest, Homebrew   string
      Headless, Force      bool
      Gaming, Ollama       bool
      LinuxDesktop, Snapper bool
      DryRun               bool
      Intents              multiFlag   // reuse the type from install.go
  }

  type applyOptions struct {
      Manifest, Homebrew   string
      Headless, Force      bool
      Gaming, Ollama       bool
      LinuxDesktop, Snapper bool
      DryRun               bool
      Intents              []string
  }

  // resolveApplyOptions merges parsed flags with env equivalents; precedence
  // flag > env for booleans is a logical OR. getenv is injected (os.Getenv in
  // prod, a map lookup in tests) so parity is table-testable without process env.
  func resolveApplyOptions(f applyFlags, getenv func(string) string) applyOptions

  // envTrue reports whether v is exactly "true" (bootstrap's == "true" contract).
  func envTrue(v string) bool { return v == "true" }

  // optInIntents are intents added only by their feature flag, never in the
  // headless default set.
  var optInIntents = map[string]bool{"gaming": true, "ollama": true}

  // resolveIntents computes the headless intent set: --intent when given, else
  // every manifest intent NOT in optInIntents (sorted); then gaming/ollama
  // appended when their flags/env are set. Guided mode uses tui.Selection.Intents
  // instead and does not call this.
  func resolveIntents(m *manifest.Manifest, opts applyOptions) []string
  ```
  Resolution:
  `Headless = f.Headless || f.Force || envTrue(getenv("NON_INTERACTIVE")) || envTrue(getenv("FORCE_INSTALL"))`;
  `Force = f.Force || envTrue(getenv("FORCE_INSTALL"))`;
  `Gaming = f.Gaming || envTrue(getenv("ENABLE_GAMING"))`;
  `Ollama = f.Ollama || envTrue(getenv("ENABLE_OLLAMA"))`;
  `LinuxDesktop = f.LinuxDesktop || envTrue(getenv("LINUX_DESKTOP"))`;
  `Snapper = f.Snapper || envTrue(getenv("SNAPPER"))`;
  `Homebrew = f.Homebrew` (flag-only tristate); `DryRun/Manifest/Intents` pass through.
  `resolveIntents`: if `len(opts.Intents)>0` → base set = `opts.Intents`; else base set =
  sorted `[name for name in m.Intents if !optInIntents[name]]`; then `if opts.Gaming` append
  `"gaming"`, `if opts.Ollama` append `"ollama"`; dedupe preserving order.
- **MIRROR**: `install.go` `multiFlag` (reuse the existing type — do NOT redefine);
  `sysconfig.go` `firstNonEmpty` precedence idiom.
- **IMPORTS**: `sort`, `internal/manifest`.
- **GOTCHA**: Truthy = exactly `"true"` — assert `"1"`/`"yes"`/`"TRUE"` do NOT flip (bootstrap
  contract; `HOMEBREW_NO_ASK=1`/`NONINTERACTIVE=1` are bootstrap *outputs*, not inputs). Do
  not read process env directly — everything flows through the injected `getenv`.
- **VALIDATE**:
  ```bash
  go build ./cmd/dotf/
  go test ./cmd/dotf/ -run 'ApplyOptions|ResolveIntents'
  ```
  Tests: per-field parity (flag vs env each set the field: `--force`→`Headless&&Force`,
  `NON_INTERACTIVE=true`→`Headless`, `FORCE_INSTALL=true`→`Headless&&Force`, `--gaming` vs
  `ENABLE_GAMING=true`, `--enable-ollama` vs `ENABLE_OLLAMA=true`, `--linux-desktop` vs
  `LINUX_DESKTOP=true`, `--snapper` vs `SNAPPER=true`); precedence (flag on + env off still
  on; env alone works); truthiness lock (`ENABLE_GAMING=1|yes|TRUE` → `Gaming==false`);
  `--homebrew=false`→`"false"`, absent→`""`; `resolveIntents` (default = base+dev sorted,
  gaming when flagged, ollama name appended when flagged, `--intent x` narrows to `[x]`).

### Task 2.1: `cmd/dotf/apply.go` orchestration — Depends on [1.1, 1.2, 1.3]

- **BATCH**: B2
- **ACTION**: Create `cmd/dotf/apply.go` (+ `apply_test.go`) — `runApply(args, stdout, stderr,
  r) int` implementing the ordered pipeline, headless-vs-guided gate, sudo-once, dry-run
  preview, and unified summary via `failure.Collector`.
- **IMPLEMENT**: `flag.NewFlagSet("apply", flag.ContinueOnError)`, `fs.SetOutput(stderr)`.
  Register the flags from the parity table into an `applyFlags`. `fs.Parse` failure → return 2.
  Then follow the **Ordered apply pipeline** above exactly. Reuse:
  - `resolveSysconfig("","","","","", m.Sysconfig)` (sysconfig.go) → identity `Config`; set
    `.Firewall = sysconfig.FirewallUnset`, `.User = realUserName()`.
  - `sysconfig.Default(r, "")` engine instance shared by P2 + P5; `eng.Plan` / `eng.Execute`.
  - `packagesForInstall(m, intents)` (install.go); `backend.Default(r, sudo,
    privilege.NewDropper(), repoRoot)`; per-pkg `engine.Install(pkg, info)`.
  - `symlink.NewEngine(repoRoot, home)` / `DefaultTable()` / `Plan` / `Apply` / `Validate`;
    `symlink.Options{Backup: opts.LinuxDesktop, Force: opts.Force, Validate: true,
    ManageLinuxDesktop: opts.LinuxDesktop}`.
  - stdout printers: `printSysconfigResults`, `printInstallResults` (build `[]installResult`),
    `printLinkResults`.
  - `firstNonEmpty`, `reportErr`, `isTTY`.
  Add local helpers:
  - `reportFatal(stderr io.Writer, err error) int` → prints `"%s: %v\n"` and returns **3**.
  - `printApplyDryRun(stdout, m, intents, identityPlan, fwPlan, linkPlan) error` → emits
    `dry-run=true` then `intents=`, `packages=`, `plan.<backend>=` (reuse
    `groupPackagesByBackendSlice`+`sortedBackendKeys`), `sysconfig.step.<domain>=<argv>`
    (reuse `sysconfigStepValue` over identity+firewall steps), `link.<bucket>=<dests>`
    (reuse `linkActionLines("link", linkPlan.Actions)`); one checked `io.WriteString`.
  - Homebrew warning: when `opts.Homebrew == "true"` and `info.OS` is a Linux family, print
    one stderr line (`"%s: --homebrew=true is a no-op on Linux; using native package
    manager\n"`). Non-fatal, not recorded.
  If `apply.go` approaches the ~500-line soft cap, split the four phase helpers
  (`applySysconfigIdentity`, `applyPackages`, `applySymlinks`, `applyFirewall`, each taking
  `*failure.Collector`) into `cmd/dotf/applyphases.go`.
- **MIRROR**: `install.go` (prime+keepalive 62-69, `installResult`, dry-run/summary split,
  `resolveRepoRoot`); `sysconfig.go` (`resolveSysconfig`, `realUserName`,
  `printSysconfigResults`, `KeepAliveErr` in the summary); `plan.go` (`reportErr`, `isTTY`,
  guided TUI wiring 40-54, confirm-gate → exit 1); `link.go` (symlink Plan/Apply/Validate +
  `BrokenLinkError` handling).
- **IMPORTS**: `context`, `flag`, `fmt`, `io`, `os`, `dotfiles`, `internal/backend`,
  `internal/exec`, `internal/failure`, `internal/manifest`, `internal/platform`,
  `internal/privilege`, `internal/snapper`, `internal/symlink`, `internal/sysconfig`,
  `internal/tui`.
- **GOTCHA**: **Never prime sudo on `--dry-run`** (compose previews only). Prime **once**
  before P1 (snapper needs root). Always `defer cancel()`. Firewall runs in P5 (after
  packages) but its `sudo -n ufw status` probe already works because sudo was primed in P0b.
  `--linux-desktop` MUST force `Backup=true` or the engine guard errors on Linux. Snapper
  outcomes are warnings — **never** `coll.Record`ed. `KeepAliveErr` is advisory (no exit
  effect). Symlink `Validate` returns `*BrokenLinkError` (a real file where a link was
  expected) → `coll.Record("symlinks", verr)` so it flips exit to 1, matching `link.go`'s
  exit-1-on-broken. Guided mode is only reached on a TTY without `--headless`/`--force`;
  `--dry-run` never enters the TUI.
- **VALIDATE**:
  ```bash
  go build ./cmd/dotf/
  go test ./cmd/dotf/ -run Apply
  go vet ./cmd/dotf/
  ```
  See Testing Strategy for the full `apply_test.go` matrix. Minimum: dry-run mutates nothing +
  exit 0; headless happy-path ordering (snapshot < sysconfig < package installs) + exit 0 +
  `failed=0` summary; package failure accumulates (later packages still run) + exit 1 +
  `packages:<name>:` line; multi-phase failures all listed (`failed=2`); snapper before
  mutations + non-Linux no-op; sudo-prime failure → exit 3; non-TTY without headless → exit 3
  + `refusing`; bad flag → exit 2; invalid `--homebrew` → exit 2.

### Task 3.1: Wire `apply` into `main` — Depends on [2.1]

- **BATCH**: B3
- **ACTION**: Update `cmd/dotf/main.go` (switch + `usage`) and add `TestRun_Apply` to
  `cmd/dotf/main_test.go`.
- **IMPLEMENT**: Change `usage` (main.go:19) to
  `"usage: dotf <version|platform|plan|install|status|diff|link|sysconfig|apply> [args]"`;
  add before `default:`:
  ```go
  case "apply":
      return runApply(fs.Args()[1:], os.Stdout, os.Stderr, exec.New())
  ```
  Add `TestRun_Apply` mirroring `TestRun_Link`: hermetic
  `t.Setenv("DOTFILES_ROOT", t.TempDir())` + `t.Setenv("HOME", t.TempDir())`, run
  `run([]string{"apply","--dry-run"})` via `captureOutput`, assert exit 0, empty stderr,
  stdout matches `^dry-run=true\n`. (The embedded default manifest ships `[sysconfig]`
  commented, so firewall is unset → no `sudo -n ufw status` probe; packages default to
  base+dev and are only listed; symlink sources under the empty temp repo root are skipped —
  the run is fully hermetic.)
- **MIRROR**: `main.go:39-57` dispatch; `main_test.go` `TestRun_Link`/`TestRun_Sysconfig`.
- **IMPORTS**: none new (`runApply` is in `package main`).
- **GOTCHA**: Keep this the only task touching `main.go`/`main_test.go`. Update `usage` or the
  unknown-subcommand help drifts (lessons 2026-04-17: surface new subcommands in top-level
  help).
- **VALIDATE**:
  ```bash
  go test ./cmd/dotf/ -run 'TestRun_Apply'
  go build ./... && go vet ./...
  ```

### Task 4.1: Docs — Depends on [3.1]

- **BATCH**: B4
- **ACTION**: Update `CLAUDE.md`, `AGENTS.md`, `README.md`, `docs/prps/prds/provisioning-cli.prd.md`,
  `tasks/todo.md`, `tasks/lessons.md`.
- **IMPLEMENT**: Add `apply` to the dotf subcommand list in `CLAUDE.md` (Stack Overview) and
  `README.md`. In `AGENTS.md` (sync-locked to `CLAUDE.md`, "Sync policy") add only the
  one-line dotf reference if the subcommand list is echoed. Flip Phase 6 status → complete in
  the PRD table and link this plan. In `tasks/todo.md` move Phase 6 to done with an outcome
  section (command, packages, tests, ACs met, documented divergences). In `tasks/lessons.md`
  record: (a) apply's exit-code-3 for hard aborts is an additive apply-only extension; (b)
  sysconfig identity + firewall are manifest-driven in apply (no invented defaults), so bare
  `apply` is not literally bootstrap `--force`'s end state until the manifest is populated.
  Commit docs under `docs/prps/**` with `docs(internal): …`; root docs with `docs(dotf): …`.
- **MIRROR**: existing dotf mentions in `AGENTS.md:14-22` and `CLAUDE.md` Stack Overview.
- **IMPORTS**: n/a.
- **GOTCHA**: Do NOT duplicate `CLAUDE.md` prose into `AGENTS.md` (its sync policy forbids it).
- **VALIDATE**:
  ```bash
  npm run lint   # markdown + shell + python + go bundle
  git diff --stat -- bootstrap.sh 'bin/install-*.sh'   # EXPECT: empty
  ```

### Task 4.2 (OPTIONAL): `Manifest.PackagesForIntents` DRY refactor — Depends on [none]

- **BATCH**: B4 (independent; do last, or skip if time-constrained)
- **ACTION**: Extract the intent→package resolution duplicated across
  `install.packagesForInstall` (install.go:109) and `tui.PackagesForIntents` (tui/options.go:56)
  into `internal/manifest`.
- **IMPLEMENT**: Add `func (m *Manifest) PackagesForIntents(intents []string) []Package` to
  `internal/manifest` (dedup, manifest order). Rewrite `packagesForInstall` to delegate
  (return `m.Packages` when `intents` is empty, else `m.PackagesForIntents(intents)`); rewrite
  `tui.PackagesForIntents` to map `m.PackagesForIntents(selected)` to names. `apply` continues
  to call `packagesForInstall` (unchanged call site).
- **MIRROR**: existing dedup logic in both call sites.
- **IMPORTS**: none new.
- **GOTCHA**: Preserve existing behavior exactly (dedup + ordering) so `install_test.go`,
  `plan_test.go`, and `tui_test.go` still pass unchanged. This is a pure refactor — no
  behavior change. Skip entirely if it risks the schedule; `apply` does not require it.
- **VALIDATE**:
  ```bash
  go test ./internal/manifest/ ./internal/tui/ ./cmd/dotf/
  ```

---

## Testing Strategy

Stdlib `testing`, table-driven, `exec.Fake` + `t.Setenv("HOME"/"DOTFILES_ROOT", t.TempDir())`.
Argv/order asserted via `f.CallsSnapshot()`; reuse `sudoCallIndex`/`countSudoCalls`/
`assertNoSudoCalls` (already in `sysconfig_test.go`, same `package main` — directly reusable;
promote to a shared `helpers_test.go` only if convenient). Env parity is tested at the pure
`resolveApplyOptions` level with an injected `getenv` — no `os.Setenv`. No testscript, no new
deps.

### Shared test helpers to add (in `apply_test.go`)

- `newApplyFake(t) *exec.Fake` — registers `uname -s`/`uname -m` for a chosen distro (copy
  `archPacmanFake`/`status_test.go`), the pkg-manager path, and `cargo`/`snapper`/`ufw` paths
  as needed; primes `sudo -v`/`sudo -n -v`/`sudo -n ufw status` where a happy path needs them.
- `writeApplyManifest(t, body) string` — temp manifest (reuse `writeTestManifest`) with a
  couple of packages + `[intents]` (incl. `gaming`) + optional `[sysconfig]` firewall.
- `applyEnv(map[string]string) func(string) string` — closure for the injected `getenv`.

### Issue → concrete test mapping

| Issue / AC | Test (file → name) | Assertion |
| --- | --- | --- |
| **#12** flag/env parity | `applyopts_test.go` (full matrix) + `main_test.go` `TestRun_Apply` | Every flag and its env twin sets the right `applyOptions` field; truthiness locked to `"true"`; dispatch wired. |
| **#12** same-mechanism end state as `bootstrap.sh --force` | `apply_test.go` `TestRunApply_HeadlessHappyPath_OrderAndEndState` | `--headless`, all-success fake → exit 0; `CallsSnapshot()` indices prove ordering: `sudo -v` prime < first sysconfig mutation < first package install; symlink created in temp `$HOME` (filesystem assert); stdout has `result.*`/`link.*`; stderr `apply summary: … failed=0`. |
| **#12** `--gaming`/`--enable-ollama`/`--linux-desktop`/`--homebrew=` | `applyopts_test.go` + `apply_test.go` `TestRunApply_LinuxDesktopFlagPassthrough` | gaming intent selected; ollama degrades to no-op; desktop links appear in temp `$HOME` (proves `ManageLinuxDesktop`+forced `Backup`); `--homebrew=true` on Linux warns, does not fail. |
| **#13** snapshot on Linux, before mutations | `snapper_test.go` `TestCreate_LinuxWithBinary` + `apply_test.go` `TestRunApply_SnapperInvokedBeforeMutations` | `sudo snapper create -d "dotf apply: pre-apply"` recorded; its index precedes the first sysconfig/package mutation; snapshot `RunErrors` does **not** change exit code (still 0) and emits a stderr warning. |
| **#13** no-op on non-snapper systems | `snapper_test.go` `TestCreate_NonLinuxNoOp`/`_MissingBinary` + `apply_test.go` `TestRunApply_SnapperNoOpNonLinux` | macOS/absent-binary → zero `sudo snapper` calls; exit unaffected. |
| **#14** non-fatal accumulation, no abort | `apply_test.go` `TestRunApply_PackageFailureAccumulatesExit1` | `RunErrors[CommandKey("cargo","install","rg")]=os.ErrPermission`; the next package still installs after `rg` fails (assert via `CallsSnapshot`); exit 1; stderr `apply summary: … failed=1` + `packages:rg:` line. |
| **#14** end-of-run summary lists ALL failures | `apply_test.go` `TestRunApply_MultiPhaseFailuresAllListed` + `collector_test.go` `TestCollector_AccumulatesAll` | Inject a sysconfig-domain failure AND a package failure → exit 1; summary `failed=2` with both `sysconfig:<domain>:` and `packages:<name>:` lines in run order. |
| **#14** soft-fail vs hard-abort exit codes | `_PackageFailureAccumulatesExit1` (1) vs `_SudoPrimeFailureFatal`/`_ManifestResolutionErrorFatal`/`_NonTTYWithoutHeadlessRefuses` (3) vs `_BadFlagExit2`/`_InvalidHomebrewExit2` (2) | Distinct codes 1/2/3; no `apply summary:` line on 2/3. |
| dry-run mutates nothing, no sudo | `apply_test.go` `TestRunApply_DryRunNoSudoNoMutation` | `--dry-run` → exit 0, stdout `^dry-run=true\n`, `CallsSnapshot()` has **no** `sudo` calls, temp `$HOME` untouched. |
| symlink broken-link accumulates | `apply_test.go` `TestRunApply_SymlinkBrokenLinkAccumulates` | Seed a real file where a link is expected → `Validate` `BrokenLinkError` → exit 1 + `symlinks:` summary line; sysconfig/packages phases still ran. |

### Additional unit tests

| Test | Input | Expected | Edge? |
| --- | --- | --- | --- |
| headless implies no TUI | `--headless`, non-TTY | never calls `tui.Run`; proceeds to apply | Yes |
| env-only headless | `getenv NON_INTERACTIVE=true`, no flag, non-TTY | headless apply runs (no refusal) | Yes |
| firewall runs after packages | happy path w/ manifest `firewall="auto"` | `sudo ufw …` call index > last `sudo <pkgmgr> …` index | Yes |
| firewall unset → no firewall steps | default manifest | zero `ufw`/`firewall-cmd` calls | Yes |
| KeepAlive error surfaced, not counted | `RunErrors[CommandKey("sudo","-n","-v")]=err` | summary shows a keep-alive line; `failed` count unchanged; exit unchanged | Yes |
| guided not-confirmed → exit 1 | TTY fake, `tui.Run` returns `Confirmed=false` | exit 1, `apply not confirmed`, no mutations | Yes (covered via manual/TTY note) |

### Edge-case checklist

- [ ] `--dry-run` never primes sudo and writes zero files/temp files.
- [ ] Sudo primed exactly once; single `KeepAlive`; `defer cancel()` always runs.
- [ ] Snapper: Linux create argv exact; non-Linux/absent → zero calls; failure non-fatal, warning-only, not in `steps=`.
- [ ] Non-fatal package/sysconfig/symlink failures accumulate; later steps still run; exit 1.
- [ ] Hard aborts (manifest/home/repo/prime/non-TTY) → exit 3, no `apply summary:` line.
- [ ] Flag parse / invalid `--homebrew` → exit 2, no plan/privilege work.
- [ ] `--linux-desktop` forces `Backup=true` (no `LinuxDesktopGuardError`).
- [ ] Truthiness locked to `"true"` (`1`/`yes`/`TRUE` do not flip).
- [ ] Bash untouched (`git diff` empty for `bootstrap.sh`/`bin/install-*.sh`).

---

## Validation Commands (the full local gate — "Level 5")

Run from the repo root. All must be clean before "done".

```bash
# 1. Build everything.
go build ./...

# 2. Vet — CI step "Vet".
go vet ./...

# 3. Race-enabled tests — CI step "Test" (authoritative).
go test -race ./...
#    Targeted while iterating:
go test -race ./internal/failure/ ./internal/snapper/ ./cmd/dotf/

# 4. Lint — CI step "Lint" (golangci-lint; standard + revive).
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

Manual / dry-run smoke (Phase 6 signal):

- [ ] `go run ./cmd/dotf apply --dry-run` — prints `dry-run=true` + composed plan, mutates
      nothing, exits 0, no sudo prompt.
- [ ] `go run ./cmd/dotf apply --headless --dry-run` — same, headless path (no TUI).
- [ ] `NON_INTERACTIVE=true go run ./cmd/dotf apply --dry-run` — env-only headless, same output.
- [ ] `go run ./cmd/dotf apply --snapper --dry-run` on a non-snapper host — snapshot no-op,
      exit 0.
- [ ] On a scratch VM: `dotf apply --headless --snapper` (manifest with `[sysconfig]` populated)
      → snapshot taken, sysconfig/packages/symlinks/firewall applied in order, one summary;
      re-run reaches `failed=0` with converged sub-plans.

---

## Acceptance Criteria

- [ ] All tasks (1.1–4.1; 4.2 optional) complete; every validation command green.
- [ ] `dotf apply` wired into `main.go` (`usage` + switch); dispatch smoke passes.
- [ ] **#12**: `--headless`/`--force` + `--gaming`/`--enable-ollama`/`--linux-desktop`/
      `--homebrew=`/`--snapper` and their env twins (`NON_INTERACTIVE`, `FORCE_INSTALL`,
      `ENABLE_GAMING`, `ENABLE_OLLAMA`, `LINUX_DESKTOP`, `SNAPPER`) resolve per the parity
      table (flag OR env for booleans, truthy = `"true"`); a no-prompt run executes the full
      ordered pipeline reaching the manifest-declared end state with zero prompts.
- [ ] **#13**: `--snapper`/`SNAPPER=true` takes a Linux `sudo snapper create -d` snapshot
      before any mutation; non-Linux/absent-binary is a silent no-op; failures are non-fatal
      warnings that never affect the exit code.
- [ ] **#14**: non-fatal failures across sysconfig/packages/symlinks/firewall accumulate
      without aborting; one end-of-run `apply summary:` lists every failure; exit 1 on any
      soft failure, distinct from hard-abort (3) and usage (2).
- [ ] Sudo primed exactly once for the whole run; `--dry-run` never primes and mutates nothing.
- [ ] Firewall runs as a second sysconfig pass after packages (PRD order); unset firewall is a
      no-op; `--linux-desktop` forces `Backup=true`.
- [ ] Every mutating command asserted by **exact argv** via `exec.Fake`; env parity tested via
      injected `getenv` (no `os.Setenv`).
- [ ] Bash untouched (`git diff` empty for `bootstrap.sh`/`bin/install-*.sh`); no lint/vet errors.
- [ ] Docs updated (`CLAUDE.md`/`AGENTS.md`/`README.md`/PRD Phase 6 status + `tasks/*`).

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
| --- | --- | --- | --- |
| Headless run cannot be tested without mutating the host | High | High | All mutation flows through `exec.Runner`; symlinks tested against `t.TempDir()` HOME/DOTFILES_ROOT. Assert argv/ordering via `exec.Fake` — never real syscalls. |
| Firewall pass re-enables ufw every run (idempotency break) | Med | High | Sudo primed in P0b so the `sudo -n ufw status` probe succeeds at Plan time (Phase 5), yielding a converged empty plan. Reuse `FirewallPlanNeedsSudo` semantics. |
| `--linux-desktop` trips `LinuxDesktopGuardError` | Med | Med | Apply forces `symlink.Options.Backup=true` whenever `ManageLinuxDesktop` is set (parity with bootstrap's forced `backup=true`); asserted by test. |
| Exit-code-3 divergence surprises automation | Low | Low | Documented apply-only additive extension (siblings unchanged); recorded in `apply.go` + `tasks/lessons.md`; codes 0/1/2/3 mapped in this plan. |
| Snapshots accumulate unbounded (no cleanup) | Low | Low | Single pre-apply snapshot per run (parity with bootstrap's `create -d`); a linked pre/post pair with `--cleanup-algorithm number` is noted as a future enhancement, not built. |
| Bare `dotf apply --headless` ≠ literal `bootstrap.sh --force` end state (firewall/shell/ollama unset by default) | Med | Med | Documented: sysconfig/firewall are manifest-driven (opt-in via `[sysconfig]`), matching `dotf sysconfig`; `--enable-ollama` degrades to no-op until the manifest gains the intent. Phase 6 delivers the mechanism; end-state equivalence completes as the manifest is populated (Phase 7). Open Questions G1/G7. |
| `apply.go` drifts past the ~500-line soft cap | Med | Low | Split the four phase helpers into `applyphases.go`; keep flag/env in `applyopts.go`, accumulator in `internal/failure`, snapshot in `internal/snapper`. |
| Adding `apply` breaks `main`'s usage/help | Low | Low | Single task (3.1) updates `usage` + switch + dispatch test together. |

## Open Questions

- **G1 (`--enable-ollama` content):** confirm whether Phase 6 should also add an
  `[intents.ollama]` + `custom-script` `[[package]]` and a `bin/install-ollama.sh` (porting
  `bootstrap.sh:1449-1517` install + `systemctl enable --now ollama`), or leave the flag as a
  wired no-op until a later manifest-population task. **This plan defers it** (flag wired,
  degrades to no-op) to keep the shipped embedded manifest unchanged and avoid test churn.
- **G7 (SSH server enablement):** the PRD's "firewall/ssh" phase currently only opens the
  firewall port; installing/enabling `sshd` (`openssh-server`, `ssh-keygen -A`,
  `systemctl enable --now sshd`) has no Go module. **Out of Phase 6 scope**; confirm it lands
  as a future `sysconfig`/`custom-script` step (the `Collector` already accommodates a
  `Record("ssh", …)`).
- **Firewall/shell headless default:** this plan keeps sysconfig (incl. firewall) fully
  **manifest-driven** in apply (skip when unset) rather than inventing an apply-only
  `firewall=auto`/`shell=zsh` default, to preserve the Phase 5 "no destructive shipped
  default" invariant and keep `dotf sysconfig`/`dotf apply` consistent. Confirm this is the
  desired parity interpretation (mechanism-parity, manifest-declared end state) vs. hardcoding
  bootstrap's firewall/zsh defaults into apply.
- **Exit code 3 for hard aborts:** confirm the additive apply-only taxonomy (vs. keeping every
  setup error at `reportErr`=1 and distinguishing by the summary line). This plan adopts code 3.

## Notes

- **Reuse over extraction:** `runApply` stays in `package main` and calls the existing helpers
  directly — no `internal/provision` churn. Only the two genuinely reusable mechanisms
  (`internal/failure`, `internal/snapper`) are new packages.
- **Sudo-once:** the single `Prime` + `KeepAlive` spanning all phases is the one intentional
  divergence from the per-subcommand priming in `install.go`/`sysconfig.go`; every engine
  emits literal `sudo …` argv kept warm by that keep-alive (no engine primes itself).
- **Firewall split:** the two-call sysconfig approach (identity in P2, firewall-only in P5)
  honors the PRD's explicit "firewall/ssh last" ordering with no API change — `Step.Domain`
  tagging already partitions cleanly.
- **Greppable output stays consistent:** apply reuses `printSysconfigResults`/
  `printInstallResults`/`printLinkResults` for stdout `result.*`/`link.*` lines and the
  `failure.Collector` for the single stderr `apply summary:` line + exit code.
- **Confidence: 8/10.** High on the pipeline shape, reuse surface, and exec/fs test seams (all
  grounded in merged code). The −2 is residual: the exact headless end-state parity is bounded
  by manifest coverage (Open Questions G1/G7) and the guided-TUI path is validated only via
  dispatch + manual, per `tui.Run`'s "must not be called from unit tests" contract.
