# The Systems Thinker — dotf CLI as a Complex System

Persona: **The Systems Thinker** (system dynamics, second-order effects, feedback
loops, leverage points). Squad research question: _what features, enhancements, and
fixes should be added to `dotf`, including migrating `bin/install-*.sh` into native
TOML-manifest backends._

Method: read the actual apply pipeline (`cmd/dotf/apply.go`, `applyphases.go`,
`applyopts.go`, `install.go`), all seven backends (`internal/backend/*`), the manifest
schema + validator (`internal/manifest/*`), the failure collector, platform detection,
brew bootstrap, three representative custom-scripts, the release CI, and `bootstrap.sh`.
Confidence ratings and file:line citations throughout.

---

## Executive Summary

`dotf` is architecturally clean — a well-seamed, testable engine with a static backend
map and a mockable `exec.Runner`. But viewed as a _system in motion over repeated runs_,
three structural properties dominate everything else:

1. **The write path never consults the read path.** `apply`/`install` call
   `engine.Applicable()` then `engine.Install()` directly and **never call
   `Installed()`** (`cmd/dotf/install.go:96-104`, `applyphases.go:135-143`). `Installed()`
   is wired into _exactly one_ place — `dotf status`/`diff`
   (`cmd/dotf/status.go:101`). Idempotency is therefore not a property of `dotf`; it is
   delegated, per-package, to whatever the backend's `Install()` happens to do. This is
   the keystone from which most second-order problems hang.

2. **The idempotency logic lives inside the very scripts the project wants to retire.**
   The custom-script backend is idempotent only because each `bin/install-*.sh`
   _self-skips_ internally (`bin/install-tpm.sh:20-23`, `install-gnu-utilities.sh:36-39`,
   `install-mise.sh:226-234`). So retiring a script is not a mechanical port — it means
   re-implementing that script's self-skip / self-update / platform-detect logic in Go.
   This is the hidden coupling that turns the "shrinking escape hatch" into a _growing_
   one.

3. **`dotf status` structurally lies about the opt-in tools.** custom-script's
   `Installed()` probes `pkg.Name` on `PATH` (`internal/backend/custom_script.go:49-55`),
   but the manifest names are _bundle labels_, not binaries: `rust` (binary is
   `rustc`/`cargo`), `aws-tools` (`aws`), `hashicorp-tools` (`terraform`),
   `container-tools` (`docker`/`podman`), `tpm` (a git clone — **no binary at all**),
   `gnu-utilities` (`gcoreutils` et al.). Every one of these reports
   `drift=declared-not-installed` **forever, even when fully installed**. The
   observability instrument is calibrated wrong.

The single highest-leverage technical change is a **manifest `probe` field wired into a
real `Installed()` gate in the apply loop**: it simultaneously (a) stops `status`/`diff`
lying, (b) makes `apply` genuinely idempotent and network-cheap, and (c) is the
_precondition_ for `dotf update`, `dotf doctor`, and any lockfile. One change, four
unlocked features. The single highest-leverage _conceptual_ change is to **reframe
custom-script from "shrinking escape hatch" to "permanent adapter layer"** for the ~9
genuine setup-steps, so the migration target shrinks to the ~10 mechanical tools and the
"migration is failing" anxiety dissolves.

---

## System Map — The Apply Pipeline

`runApply` (`cmd/dotf/apply.go:39-160`) is an ordered, single-pass orchestrator. The
canonical order is **sysconfig-identity → brew → packages → symlinks → firewall**, with
directory creation and an optional snapper snapshot bracketing the front.

```
                         ┌─────────────────────────────────────────┐
   flags + env ─────────▶│ resolveApplyOptions (applyopts.go:80)    │
   (NON_INTERACTIVE,     │  flag OR env, precedence flag>env        │
    FORCE_INSTALL, ...)  └───────────────┬─────────────────────────┘
                                         │
              ┌──────────────────────────┼───────────────────────────┐
              │ mode gate (apply.go:91-114)                           │
              │  dry-run → preview, never sudo                        │
              │  headless → resolveIntents (opt-in excluded)          │
              │  TTY → tui.Run → honor exact pkg set (verbatim)       │
              │  non-TTY, no --headless → HARD ABORT (exit 3)         │
              └──────────────────────────┬───────────────────────────┘
                                         │
   ┌─────────────────────────────────────────────────────────────────┐
   │ sudo.Prime ONCE + KeepAlive (apply.go:118-123)  ← single warm    │
   │                                                    keep-alive     │
   ├─ P1  snapper snapshot        (best-effort, WARN only, not in coll)│
   ├─ P1b applyDirs              → coll.Record("directories")          │
   ├─ P2  applyIdentity (sysconfig hostname/tz/locale/shell)           │
   ├─ P3b applyBrew  → prependPath + platform.Detect AGAIN (re-detect) │
   ├─ P3  applyPackages  → per-pkg: Applicable? → Install (NO Installed)│
   ├─ P4  applySymlinks  → Plan/Apply/Validate                         │
   ├─ P5  applyFirewall  (2nd sysconfig pass, firewall-only)           │
   └─ P6  applySummary → greppable stdout + failure.Collector summary  │
                                         │
                                    exit 0|1|3|130
   └─────────────────────────────────────────────────────────────────┘
```

### Dependencies and hidden coupling

| Edge                                          | Why it exists                                           | Coupling / fragility                                                                                                                                                                                                                                                                                       |
| --------------------------------------------- | ------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| brew **before** packages (`apply.go:146-149`) | pkg backend shells to `brew install` on macOS/linuxbrew | `applyBrew` mutates process `PATH` and calls `platform.Detect` a **second time** (`applyphases.go:53-54`). The packages phase runs against a _different_ `info` than sysconfig did. Temporal coupling through global `PATH` + re-detection.                                                                |
| sysconfig-identity **before** packages        | none functional                                         | Ordering is by PRD convention, not data dependency. Identity (hostname/tz) has no edge to packages.                                                                                                                                                                                                        |
| firewall **after** packages (`apply.go:156`)  | ufw/firewalld may be _installed_ by the packages phase  | Real dependency, but the firewall pass re-plans with every identity field zeroed, forcing `mergeSysconfigResults` (`applyphases.go:218-242`) to dedupe "applied-vs-skipped" contradictions. Complexity born of running one engine twice.                                                                   |
| symlinks **after** packages                   | tools should exist before configs point at them         | Weak/aspirational — nothing enforces it; symlinks would apply fine first.                                                                                                                                                                                                                                  |
| single sudo prime (`apply.go:118`)            | snapper + pkg + apt-repo + firewall all need root       | Deliberate divergence from per-subcommand priming in `install.go:62`/`sysconfig.go`. Good centralization, but couples the _entire run's_ privilege lifetime to one keep-alive; on a very long opt-in run the ticket refresh is the single point of auth failure (surfaced advisorily, `apply.go:181-183`). |

### What breaks mid-run — failure accumulation

The `failure.Collector` (`internal/failure/collector.go`) is the run's shock absorber: a
failed package **never aborts the loop** (`applyphases.go:137-142`); every phase records
into `coll` and the run reaches P6, prints one summary, returns exit 1 if anything failed
(`collector.go:69-74`). This is a **balancing structure** — a single flaky download can't
strand a fresh machine half-provisioned. Parity with bootstrap's `BOOTSTRAP_FAILURES`.

Second-order cost of that resilience: **failures are soft and undistinguished.** A sha256
mismatch on `starship` (supply-chain red flag) and a transient apt 404 both land as one
line under `packages:<name>` with exit 1. There is no severity axis, no
`--fail-fast`, and no machine-readable failure taxonomy — so CI cannot react
differently to "network blip, retry" vs "checksum failed, stop the world." (Confidence:
high — `collector.go:35-66` records only `{Step, Err}`.)

### Is apply idempotent AND deterministic on re-run?

**No, not intrinsically — it is idempotent only to the extent each backend's `Install()`
is, and it is not deterministic because several backends touch the network every run.**
Traced per backend (all re-run unconditionally because nothing gates on `Installed()`):

| Backend                                       | Re-run behavior                                                                   | Idempotent?                          | Deterministic?                                             |
| --------------------------------------------- | --------------------------------------------------------------------------------- | ------------------------------------ | ---------------------------------------------------------- |
| `pkg` (`pkg.go:47-59`)                        | `pacman -S --needed` / `apt install -y` every run                                 | Yes (mgr no-ops)                     | No — hits mirrors; version floats                          |
| `script-url` starship (`script_url.go:28-52`) | **re-downloads + re-executes the vendor installer every run**                     | Only if the vendor script self-skips | **No** — floating `install.sh`, network each run           |
| `github-release` (`github_release.go:41-78`)  | **re-downloads asset, re-extracts, re-copies binary every run**                   | Overwrites (effectively yes)         | No — network each run; but `tag`+`sha256` pin the artifact |
| `toolchain` (`toolchain.go:60-68`)            | `cargo install` / `npm -g` every run                                              | Rebuilds/updates                     | No                                                         |
| `custom-script` (`custom_script.go:36-46`)    | runs the `.sh` every run; **idempotency delegated to the script's own self-skip** | Yes _if the script self-skips_       | Mixed — mise/rust self-_update_ (rolling)                  |
| `apt-repo` (`apt_repo.go:51-108`)             | re-fetches key, rewrites sources, `apt-get update`+install every run              | Yes (overwrites)                     | No — network each run                                      |

Net: **re-running `dotf apply` on a converged host does real work and network I/O every
time**, and for the rolling-release scripts (mise/rust/atuin) it can silently _change_
the installed version. That is acceptable-but-wasteful today; it becomes a _correctness_
problem the moment anyone adds a lockfile or promises reproducibility. (Confidence: high.)

---

## Feedback Loops

### R1 — The escape-hatch reinforcing loop (THE central dynamic)

```
   need to add a tool
          │
          ▼
   custom-script is the path of least resistance ───────────┐
   (one TOML entry + a bin/install-*.sh you already trust)   │  (+)
          │                                                   │ reinforcing
          ▼                                                   │
   custom-script count grows (19/37 today)                    │
          │                                                   │
          ▼                                                   │
   more idempotency/platform logic trapped in shell ──────────┘
          │
          ▼
   each native migration gets more expensive
   (must re-implement self-skip + self-update + detect in Go)
          │
          ▼
   native path feels even less worth it  → back to top
```

Why it self-reinforces (grounded, not hand-waved):

- The activation energy of custom-script is near-zero: 3 lines of TOML pointing at an
  already-hardened script (`manifest/packages.toml:235-238`). The native path requires
  choosing a backend, pinning `tag`+`sha256` (`validate.go:176-181` _forces_ a tag when
  sha256 is set), computing checksums by hand, and — critically — re-authoring the
  script's self-skip logic in Go, because `Install()` has no idempotency of its own.
- The scripts encode _real_ hard-won logic (platform branches, `NON_INTERACTIVE` gating,
  best-effort self-update). Porting them is a rewrite, not a translation. The PRD's own
  ≥80%-LOC-retired metric is deferred precisely because the cheap 20% (mechanical
  github-release ports) doesn't move the LOC needle; the LOC lives in the _hard_ scripts.
- Result matches the objective's data exactly: custom-script is the **most-used** backend
  (19), the "shrinking" hatch has grown, and only ~12% of bash LOC is gone at Phase 7.

**Confidence: high.** This is the load-bearing systemic finding.

### Candidate balancing loops (B*) to reverse R1 — ranked by leverage

- **B1 (structural, strongest): raise the native path's capability so its activation
  energy drops below custom-script's.** Add the three missing capabilities that block
  clean migration — a `git-clone` backend (tpm), `{{.Version}}` / multi-arch asset
  templating already half-present (`github_release.go:96-102` templates `{{.OS}}`/
  `{{.Arch}}` but nothing pins version), and a `post_install`/link step. When "native"
  is _easier_ than writing a bespoke script, the gradient reverses on its own. This
  attacks the loop at Meadows level 10 (stock-and-flow structure).
- **B2 (rule/information, cheap): a CI gate that counts custom-script entries and fails
  or warns when the count rises without a documented waiver.** Makes the loop _visible_
  and adds friction to the easy path. Meadows level 8/6.
- **B3 (goal/paradigm, highest leverage but disruptive): reframe custom-script as a
  permanent adapter for genuine setup-steps.** Declare tpm, tmux-sessionizer,
  gnu-utilities, mise, rust (and arguably lua) _permanent_ custom-scripts by design.
  Then the migration backlog is only the ~10 mechanical tools, and the reinforcing loop
  has nothing left to reinforce. Meadows level 3.

### B0 — The observability balancing loop that is currently _disconnected_

`dotf status`/`diff` _should_ be the balancing loop that surfaces drift and creates
pressure to converge. But because custom-script `Installed()` probes the wrong name
(`custom_script.go:49-55`), the instrument reports permanent false drift for the opt-in
tools. **The feedback wire is cut.** Reconnecting it (the `probe` field) restores the
loop that keeps the manifest and the machine honest. (Confidence: high — verified the
name/binary mismatch for rust, aws-tools, hashicorp-tools, container-tools, tpm,
gnu-utilities.)

---

## Second-Order Effects of the Major Candidate Features

### `dotf update` / self-update

- **1st:** a command to refresh installed tools.
- **2nd:** requires a real `Installed()` gate _and_ a notion of "current vs desired
  version" — which today only exists inside mise/rust's own self-update
  (`install-mise.sh:226-234`). Without version state, `update` collapses into "re-run
  Install() on everything," i.e. today's `apply`. **`update` is not additive over the
  current design; it forces the idempotency/version-state work first.**
- **2nd (self-update of the binary):** the objective notes no self-update command exists.
  Adding one couples `dotf` to its own release channel (`release.yml` + `.goreleaser.yml`)
  and re-introduces the checksum-verify logic that already lives in `bootstrap.sh:283-294`
  — pulling that 307-line shim's responsibilities _into_ the binary. Net simplification if
  done once, duplication if done twice.

### A lockfile (version pinning)

- **1st:** reproducible installs; `dotf.lock` records resolved versions/checksums.
- **2nd cascade (this is the big one):** pinning → someone must _maintain_ the pins →
  checksums go stale on every upstream release → you need **Renovate/Dependabot** to bump
  them → but the repo has **neither** (verified: no `renovate.json`/`dependabot.yml`) →
  and `github-release` validation _already_ forces `tag`+`sha256` together
  (`validate.go:179-181`), so every pin is a manual `curl | sha256sum` chore today. A
  lockfile without bump automation converts a maintenance _option_ into a maintenance
  _obligation_ for a solo maintainer. **The lockfile's true cost is the Renovate pipeline
  it implies, not the file.**
- **3rd:** a lockfile also collides with the rolling-release scripts (mise/rust _want_ to
  float). Pinning them defeats their purpose; not pinning them makes the lockfile
  partial and its reproducibility promise a half-truth.

### `dotf doctor` / health

- **1st:** diagnoses a broken environment.
- **2nd:** doctor is only as truthful as `Installed()`. Built on today's PATH-probe it
  would confidently report a fully-provisioned cloud host as "5 tools missing." Doctor
  _depends on_ the probe fix; shipping it first would launch a lying diagnostic and burn
  user trust (a reinforcing _distrust_ loop). Sequencing matters: probe → doctor, never
  the reverse. (Confidence: high.)

### `--only` / `--skip` filters

- **1st:** run a subset of phases/packages.
- **2nd:** interacts with the failure model. `--skip symlinks` after a failed package run
  changes what "converged" means; the exit-code contract (0/1) has no vocabulary for
  "partial by request." Low risk, but it wants a "skipped-by-user" status distinct from
  "not-applicable" (`install.go:101`) and "failed."
- **2nd (positive):** `--only packages` is the cheapest possible `dotf update` surface and
  a natural retry primitive after a soft failure — high value, low cost, _if_ idempotency
  is real.

### Reconciliation / `--prune` (explicitly out of scope)

- The additive-only stance is a **deliberate paradigm choice** (matches chezmoi). Adding
  prune would flip `dotf` from "converge-toward" to "enforce-exactly," which requires a
  _desired-state_ model the manifest doesn't have for custom-script (a script that
  installs `aws`, `terraform`, `vault` under one `pkg.Name` has no removable identity).
  **Reconciliation is blocked by the same name/identity gap as the probe.** Respect the
  scope; note that the probe work is a prerequisite if prune is ever reconsidered.

---

## Stakeholder Analysis

| Stakeholder                   | Served today                                                                                           | Underserved / at risk                                                                                                                                                                | Where interests conflict                                                                                                                                    |
| ----------------------------- | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Solo maintainer (Yandy)**   | Clean Go architecture; scripts already battle-tested; opt-in bundles keep default fast                 | Wants fewer scripts (the ≥80% metric); every native migration is a rewrite carrying self-skip logic                                                                                  | Wants _fewer scripts_, but the scripts encode edge-cases a **stranger's** machine needs. Retiring them trades maintainer simplicity for stranger fragility. |
| **Stranger cloning the repo** | `bootstrap.sh` shim + default `base`/`dev` gets them a working shell fast; checksum-verified binary    | `apt-repo` codename hard-coded to `noble` (`packages.toml:218`) breaks on any non-24.04 Ubuntu; opt-in tools show permanent false drift; docs say "~45-line shim" but it's 307 lines | Stranger benefits from the _fat_ scripts (platform detection). Maintainer's migration goal removes exactly that safety net.                                 |
| **CI / headless runs**        | First-class: `--headless`/`--force` + env twins (`applyopts.go:80-93`); soft exit 1; single sudo prime | No `--fail-fast`; no JSON output; no severity distinction; re-download-every-run wastes CI minutes and adds network-flake surface                                                    | CI wants _fast + deterministic_; the network-per-run backends make it slow and flaky. Idempotency gate serves CI most.                                      |
| **Fresh machine**             | Failure collector prevents half-provisioning; dirs pre-created (`applyphases.go:83-106`)               | Long opt-in runs bet the whole run on one sudo keep-alive                                                                                                                            | —                                                                                                                                                           |
| **Converged host (re-run)**   | —                                                                                                      | **Worst-served.** Does full network I/O every run for zero state change; rolling scripts may silently bump versions; status lies about what's installed                              | The converged-host re-run is where the missing `Installed()` gate hurts most — and it's the _most common_ real-world invocation.                            |

**Key conflict:** the maintainer's "retire the scripts" goal and the stranger's "I need
the scripts' edge-case handling" need are in direct tension. The resolution is _not_
binary (retire all vs keep all) — it is **triage**: retire the ~10 mechanical scripts to
native backends (strangers lose nothing — a checksummed binary is _more_ robust than a
`curl | sh`), and _keep + bless_ the ~9 genuine setup-steps as permanent adapters.

---

## Causal Chains (non-obvious)

1. **Missing `Installed()` gate → network-per-run → CI flakiness → soft exit 1 noise →
   maintainer stops trusting exit codes → CI failures ignored → real regressions slip.**
   The absent gate (`install.go:96-104`) doesn't just waste bandwidth; it erodes the
   signal value of the exit code over time. (Confidence: medium-high — the mechanism is
   sound; the trust erosion is inferred.)

2. **custom-script self-skip lives in shell → migration requires re-authoring self-skip in
   Go → migration deferred → custom-script stays the easy path → more self-skip logic
   accumulates in shell.** The idempotency location _is_ the migration blocker. (High.)

3. **PATH-probe on `pkg.Name` → false drift for bundle-named tools → `status`/`diff`
   untrustworthy for opt-in intents → any feature built on `Installed()` (doctor, update,
   prune) inherits the lie → those features can't ship credibly.** One wrong probe poisons
   an entire feature family. (High.)

4. **`apt-repo` codename hard-coded `noble` → works on maintainer's box → silently 404s
   `apt-get update` on a stranger's 22.04/25.04 → docker-ce fails → recorded as one soft
   line → stranger sees "docker-ce: failed" with no hint it's a codename mismatch.** A
   parameter baked as a constant becomes a stranger-only failure. (High —
   `packages.toml:213-218` even documents the footgun in a comment.)

5. **Doc says "~45-line shim" (`CLAUDE.md:28,433`, `AGENTS.md:21`) → reality is 307 lines
   with forge-abstraction + GitHub API fallback + auth-token resolution
   (`bootstrap.sh`) → an agent or contributor trusts the doc → underestimates the shim's
   attack surface and maintenance cost.** Documentation drift is itself a system input.
   (High — verified line count.)

---

## Leverage Points (Meadows hierarchy, ranked)

Ranked by leverage (Meadows: higher number = deeper intervention). I map each candidate
to its level and give a value/effort/risk read.

| #     | Meadows level                       | Intervention                                                                                                                                                                                                    | Value                    | Effort           | Risk               |
| ----- | ----------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------ | ---------------- | ------------------ |
| **1** | **6 Information flows + 12 params** | **Add a manifest `probe`/`check` field (binary name, path, or command) and wire `Installed()` into the apply loop.** Fixes false drift, makes apply idempotent + network-cheap, unlocks update/doctor/`--only`. | **Very high**            | Medium           | Low                |
| **2** | **3 Goals**                         | **Reframe custom-script as a permanent adapter layer**; bless ~9 setup-steps, target only ~10 mechanical tools. Dissolves R1.                                                                                   | **Very high**            | Low (doc/policy) | Low                |
| 3     | 10 Structure                        | Add `git-clone` backend + `{{.Version}}` templating + `post_install` hook — the three capabilities blocking clean migration (B1).                                                                               | High                     | Medium-high      | Medium             |
| 4     | 8 Balancing loop                    | CI gate counting custom-script entries; waiver-or-fail on growth (B2).                                                                                                                                          | Medium                   | Low              | Low                |
| 5     | 12 Parameters                       | Make `apt-repo` codename host-derived from `/etc/os-release VERSION_CODENAME` instead of literal `noble`.                                                                                                       | Medium (stranger-facing) | Low              | Low                |
| 6     | 12 Parameters                       | Failure severity axis + `--fail-fast` + JSON output on the collector.                                                                                                                                           | Medium                   | Medium           | Low                |
| 7     | 4 Self-organization                 | External/pluggable backends.                                                                                                                                                                                    | Low-for-scope            | High             | High (scope creep) |

**Why #1 is the keystone technical change:** it is the _only_ intervention that pays out
in four directions from one implementation — honest `status`/`diff`, idempotent/cheap
`apply`, and the enabling precondition for both `dotf update` and `dotf doctor`. Everything
in the "features" column of the roadmap either depends on it or is degraded without it.

**Why #2 is the highest _conceptual_ leverage:** it changes the system's _goal_, which
Meadows ranks above information flows. Today the implicit goal is "custom-script → 0,"
which the structure actively fights (R1). Changing the goal to "custom-script = the
adapter for genuine setup-steps; native backends for mechanical installs" makes the
current 19-count _success_, not failure, and reduces the real backlog to ~10.

Do #1 and #2 first, together. #1 is code, #2 is a paragraph in the PRD. They are
independent and mutually reinforcing.

---

## Unintended Consequences (of the recommendations themselves)

- **A `probe` field invites a second source of truth.** If `probe` and the script's
  internal self-skip disagree, which wins? Guard: `probe` gates _whether Install runs_;
  the script's self-skip becomes a redundant inner belt. Keep both; document precedence.
- **Idempotency gate can mask a broken install.** If `probe` says "installed" but the
  tool is corrupt, `apply` skips repair. `dotf doctor --repair` or `apply --force`
  (already exists, `applyopts.go:31`) must bypass the gate. Ensure `--force` means "ignore
  probe," not just "recreate symlinks."
- **A CI gate on custom-script count (B2) can incentivize gaming** — folding three tools
  into one script to keep the count flat. Count _scripts referenced_, or weight by LOC.
- **Reframing custom-script as permanent (B3)** risks becoming an excuse to _never_
  migrate anything. Pair it with an explicit, named allowlist of "permanent" scripts so
  anything _not_ on the list is still a migration candidate.
- **A `git-clone` backend** re-opens the update question: git clones drift (upstream
  moves). It needs a pin-to-ref option or it becomes another rolling, non-deterministic
  input.

---

## System Boundaries

What `dotf` treats as inside vs outside its control — and where boundaries are drawn in
the wrong place:

- **Inside:** package install, symlinks, sysconfig identity, firewall, dirs, snapper.
- **Outside (by scope):** package _removal_/reconciliation, secrets, fleet/multi-host,
  Windows-native, GUI. These are principled exclusions (additive-only paradigm).
- **Ambiguous / mis-drawn boundary:** **idempotency and version-state.** Today they sit
  _outside_ the binary (inside the shell scripts and inside mise/rust's self-update). The
  probe recommendation pulls the _decision to install_ inside the binary while leaving the
  _how to install_ in the backends — the correct boundary.
- **Boundary leak:** `applyBrew` mutating global `PATH` and re-running `platform.Detect`
  (`applyphases.go:43-57`) leaks host mutation into the middle of the pipeline. The
  packages phase's environment is not the environment the run started in. Contained today,
  but a boundary worth naming.

---

## Emergent Properties

- **Resilience emerges from the collector, not from any one phase.** No single backend is
  fault-tolerant; the _system_ is, because the collector lets the run complete. Remove the
  collector and you'd get bootstrap's old first-failure-aborts fragility back.
- **Determinism does _not_ emerge** — it would have to be designed in (pins + gate). The
  current composition of floating installers yields a system whose output depends on when
  you run it.
- **"Convergence" is asserted, not measured.** Exit 0 means "no phase errored," not "the
  machine matches the manifest" — because the truth oracle (`Installed()`) isn't consulted
  on the write path. The system _claims_ convergence it never checks.
- **The manifest is a plan, not a state.** It describes intent; there is no reconciliation
  between it and the machine except the (broken-for-opt-in) `diff`. Emergent gap: the
  manifest, the scripts, and the machine are three states that can silently disagree.

---

## Drift & Observability — The Three-State Problem

Three representations that can diverge, and where the blind spots are:

1. **Manifest** (`manifest/packages.toml`) — declares 19 custom-scripts by bundle name.
2. **The scripts** (`bin/install-*.sh`, ~13,211 LOC) — the real install logic; the
   manifest entry is a 3-line pointer that cannot express what the script _does_.
3. **The machine** — actual installed binaries.

Blind spots:

- **`Installed()` PATH-probe** (`custom_script.go:49-55`) compares (1) to (3) using the
  wrong key → false drift for rust/aws-tools/hashicorp-tools/container-tools/tpm/
  gnu-utilities. **The one instrument that bridges manifest↔machine is miscalibrated for
  the majority of opt-in tools.** (High.)
- **Manifest↔scripts** has _no_ checker. If `install-turso.sh` changes what it installs,
  nothing flags that the manifest's `turso` entry is now inaccurate. The `probe` field
  would at least tie the manifest to an observable binary.
- **script-url/vendor installers are unpinned to floating upstream** (`starship`'s
  `install.sh`, brew's `install.sh` in `brew.go:26`) — the machine can drift from the
  checksummed-at-authoring intent without any signal, because the checksum is of the
  _installer_, not the _installed artifact_.

---

## Key Insights

1. **The read path and write path are disconnected.** `Installed()` exists but only
   `status`/`diff` use it (`status.go:101`); `apply`/`install` never gate on it
   (`install.go:96-104`). This one architectural gap is upstream of idempotency,
   determinism, honest status, `update`, and `doctor`.
2. **Idempotency is trapped inside the scripts we want to delete.** That coupling — not
   effort or laziness — is why the "shrinking escape hatch" grew to 19/37.
3. **`status` lies for exactly the tools the migration added.** The observability loop
   that should create pressure to converge is wired to the wrong signal.
4. **A lockfile's real cost is the Renovate pipeline it implies** in a repo that has no
   dependency automation — and it collides with the rolling-release scripts by design.
5. **Two changes unlock most of the roadmap: a `probe` field (code) and a goal reframe
   (a paragraph).** They are independent, cheap-to-medium, low-risk, and mutually
   reinforcing.
6. **Docs have drifted from reality** ("~45-line shim" vs 307 lines) — the system's own
   description is a stale input that mis-sets maintainer expectations.

---

## Evidence Quality

- **High confidence (read directly):** the missing `Installed()` gate; the custom-script
  PATH-probe mismatch (cross-checked binary names for 6 tools); per-backend re-run
  behavior; the failure-collector model; apply ordering + double platform.Detect; the
  apt-repo `noble` constant; absence of Renovate/Dependabot; bootstrap.sh line count vs
  doc; validation forcing tag+sha256.
- **Medium confidence (inferred mechanism):** trust-erosion chain from soft exit codes;
  the precise LOC that would/wouldn't be retired per script (I read tpm, gnu-utilities,
  mise; sampled aws/hashicorp/turso/lua/rust install targets, not every line of all 29).
- **Not verified in-repo (external comparators):** chezmoi's additive stance, aqua's
  registry model, mise/topgrade update semantics — asserted from the objective brief and
  general knowledge, not fetched this pass (see Search Queries). These inform framing, not
  the code claims.

---

## Contradictions & Uncertainties

- **"Add reconciliation" vs "additive-only is a feature."** Genuine tension. Systems read:
  additive-only is the _right_ paradigm for a solo single-host tool, AND reconciliation is
  blocked by the same name/identity gap as the probe — so it's moot until the probe exists.
  Both sides are right; don't smooth it.
- **"Retire custom-script" vs "the scripts encode value a stranger needs."** Resolved by
  triage (mechanical vs setup-step), not by picking a side. The ≥80%-LOC metric is
  arguably the _wrong_ success metric — it counts the hard scripts (setup-steps) as
  failures when they may be correct permanent adapters. **Recommend the squad flag the
  metric itself as suspect.**
- **Uncertainty:** whether a `probe` field can cleanly express multi-binary bundles
  (hashicorp-tools installs terraform + vault + ...). May need `probe = ["terraform",
"vault"]` (all-present = installed) — a schema question worth prototyping before
  committing.
- **Uncertainty:** is `apply`'s re-download-every-run _actually_ biting the maintainer, or
  is it theoretical? The opt-in intents are rarely re-run, so the cost may be latent. The
  fix is still cheap and unlocks other features, so it's worth doing regardless.

---

## Search Queries Executed

Codebase (primary evidence this pass):

- `grep -rn "\.Installed(" cmd/ internal/` → only `status.go:101` + the dispatcher.
- `grep -rn "\.Install(" cmd/ internal/` → only `install.go:104` + dispatcher (no gate).
- binary-name-vs-`pkg.Name` cross-check for rust/aws-tools/hashicorp-tools/
  container-tools/tmux-sessionizer/lua/turso.
- `mise` self-update path (`install-mise.sh:120-234`).
- Renovate/Dependabot presence (none); `release.yml`/`.goreleaser.yml`; `bootstrap.sh`
  line count (307) vs doc ("~45").
- Full reads: apply.go, applyphases.go, applyopts.go, install.go, status.go, backend.go,
  custom_script.go, github_release.go, script_url.go, toolchain.go, pkg.go, apt_repo.go,
  download.go, register.go, collector.go, types.go, validate.go, platform.go, brew.go,
  manifest/packages.toml, bin/install-{tpm,gnu-utilities}.sh.

Web (recommended for the synthesis pass — not fetched here; framing drawn from objective
brief + prior knowledge):

- "chezmoi additive vs reconciliation prune stance"
- "aqua registry checksum lockfile aqua.yaml"
- "mise idempotent install version state self-update"
- "topgrade update-all model second-order"
- "Renovate custom datasource github release checksum bump automation"

---

## Digest (highest-leverage first)

- **#1 — Add a manifest `probe`/`check` field and wire `Installed()` into the apply loop.**
  It is the keystone: one change makes `status`/`diff` honest, makes `apply` idempotent and
  network-cheap, and is the precondition for `dotf update`, `dotf doctor`, and any
  lockfile. Today the write path (`install.go:96-104`) never consults the read path
  (`status.go:101`) — that disconnect is upstream of nearly every roadmap gap.
- **#2 (co-first, conceptual) — Reframe custom-script from "shrinking escape hatch" to
  "permanent adapter layer"** for the ~9 genuine setup-steps (tpm/sessionizer/gnu-utils/
  mise/rust/lua). This dissolves the reinforcing growth loop and shrinks the real
  migration backlog to ~10 mechanical tools. It's a PRD paragraph, not code.
- **The escape-hatch reinforcing loop is real and mechanical:** custom-script's near-zero
  activation energy + idempotency logic trapped _inside_ the scripts (`install-tpm.sh:20`)
  means every migration is a rewrite, so the hatch grew to 19/37. Reverse it structurally
  by making the native path _easier_ (git-clone backend + version templating), not by
  exhortation.
- **`dotf status` structurally lies** about opt-in tools — it PATH-probes bundle names
  (`aws-tools`, `tpm`) that are never binaries (`custom_script.go:49-55`). The observability
  balancing loop that should drive convergence is wired to a dead signal.
- **A lockfile's true cost is the Renovate pipeline it implies** (the repo has no
  dependency automation) and it collides head-on with the rolling-release scripts that
  _want_ to float. Weigh the cascade, not just the file.
- **`apply` is neither intrinsically idempotent nor deterministic:** script-url,
  github-release, apt-repo, and toolchain all re-download/re-execute every run because
  nothing gates on installed-state — cheap to fix, and the fix is the same `probe` gate.
- **Stranger-facing landmines from baked-in parameters:** `apt-repo` codename hard-coded to
  `noble` (`packages.toml:218`) 404s on any other Ubuntu; docs claim a "~45-line shim" that
  is actually 307 lines. Low-effort, high-goodwill fixes.
- **Sequence matters:** ship the probe _before_ `doctor`/`update`, never after — a
  diagnostic built on today's false probe would launch broken and burn trust.
