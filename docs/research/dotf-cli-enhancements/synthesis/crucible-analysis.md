# Crucible — ACH Analysis of the dotf Roadmap Decision

_Analysis of Competing Hypotheses (Heuer method). Analyst: ACH / Asymmetric Research
Squad. Date: 2026-07-04. Inputs: `objective.md`, `evidence/verification-log.md`
(highest evidentiary weight — code-verified), and the 8 persona findings
(historian, contrarian, analogist, systems-thinker, journalist, archaeologist,
futurist, negative-space)._

**Decision under analysis:** _What is the highest-value roadmap for `dotf`, and
specifically what should be done about the 19 `custom-script` tools and the
backend/schema gaps?_

---

## Executive Summary

Run as a competition among six "what is the first move?" hypotheses, the evidence does
**not** produce a single winner — it produces a **forced ordering**. Two hypotheses are
eliminated outright, one splits, one is demoted to a later phase, and the surviving pair
turn out to be **adjacent phases of one sequence rather than rivals.** The ACH exercise's
real payoff is the disconfirming evidence: it reveals a hard **dependency graph** in
which almost every attractive item (migration, doctor, update, lockfile, Renovate) is
**gated on one missing primitive**.

- **H1 "Correctness-first"** (fix the read path: a `Binary`/`probe` field + wire
  `Installed()` into the apply loop, reusing the authority-query discipline that already
  exists in `pkg.go`; un-pin/fix starship; template the apt codename) **survives with
  essentially no disconfirming evidence** and is a prerequisite for H2, H4, and the
  `doctor` half of H5.
- **H2 "Capability-first"** (build git-clone / arch-OS vocabulary / non-archive assets /
  per-arch checksum / post-install / `installed_when` / ubi-style auto-detect) **survives
  but is disconfirmed _as the first move_** — capabilities without the probe field yield
  an **unverifiable** migration. It is Phase 2, not Phase 0.
- **H4 "Migrate-the-mechanical-now with today's backends"** is **eliminated as a first
  move**: the code-verified backend-vocabulary gap (F3) and the broken-today starship
  port (F4) prove that "today's backends" cannot cleanly hold even the mechanical cases,
  and there is no `--only` to test a single port before deleting its script.
- **H6 "Status quo / minimal fixes, custom-script is permanent"** is **eliminated as a
  posture of inaction** — F1/F2/F4/F5 are confirmed defects that must be fixed — **but
  its kernel is correct and absorbed into the synthesis**: the ≈9 genuine setup-steps
  _should_ stay shell forever, additive-only _is_ right, and the ≥80%-LOC metric _is_ the
  wrong target.
- **H3 "Governance-first"** and **H5 "Day-2 UX-first"** each **split**: their cheap,
  probe-independent halves (reframe the metric + name a `setup-step` tier; `--only` /
  `--json` / `self-update`) are **co-first enablers**; their expensive or probe-dependent
  halves (Renovate automation; `doctor`) are **deferred behind the primitive they need**.

**The single most-leveraged first move** is the one item that sits at the intersection of
H1 and H2 and is named independently by **six of eight personas plus the verification
log**: **add a `Package.Binary` / `Provides` / `installed_when` field and wire a real
`Installed()` gate into the apply loop.** One field + one gate simultaneously (a) fixes
the confirmed shipping bug where `status`/`diff` lie about ≥8 tools, (b) makes `apply`
idempotent and network-cheap, (c) is the **precondition to verify that any migrated tool
actually installed**, and (d) unlocks `doctor`, `update`, and any lockfile. **Confidence:
High.** Its free co-move is a one-paragraph reframe (name the permanent `setup-step` tier;
re-scope the success metric from "≥80% LOC retired" to "custom-script ≤ ~5, delete-on-
cutover").

---

## Hypotheses

Framed as **mutually exclusive claims about the single highest-value FIRST strategic
move** (they are made exclusive by the word _first_; the analysis then tests whether the
exclusivity is real or whether they are phases).

### H1 — Correctness-first

Fix the read path before migrating anything. Add a state-detection field (`Binary` /
`Provides` / `installed_when`) and make `Installed()` truthful (reuse `pkg.go`'s
authority-query technique); wire `Installed()` into the apply/install loop so `apply`
gains a real skip path; un-pin or version-pin starship; derive the apt codename from
`/etc/os-release`. Migration is premature until `status` tells the truth and a port can be
verified.

- **Supported by:** Systems-thinker (keystone #1 — "one change, four unlocked features"),
  Negative-space (#1 — "migration is blocked at _verification_, not translation"),
  Contrarian (FLAW 1 — "the real, shippable bug is state detection"), Historian (lesson
  5 — "fix `Installed()` before/as part of migration"), Archaeologist (#2 — "the right
  technique already exists next door in `pkg.go`"), Analogist (S8 — `creates`/`Binary`
  guard). Verification log **F1, F2** (both High, code-verified).
- **Implication:** cheap, low-risk, resolves confirmed defects, and is the root of the
  dependency graph. Delivers correctness and honest observability but does **not by itself
  retire a single script.**

### H2 — Capability-first

Build the missing backend capabilities first; the scripts then migrate cheaply. The
migration stalled because the 7 backends **structurally cannot hold** what the scripts do:
non-archive assets (bare binary / `.gz` / `.pkg`), richer arch/OS vocabulary
(`x86_64`/`aarch64`, `darwin`≠`macos`), per-arch checksums, post-install steps, a
`git-clone` and an `rpm-repo` backend, and ubi/eget-style auto-detect to **invert the
effort gradient** so native is cheaper than the escape hatch.

- **Supported by:** Archaeologist (headline — "blocked on backend capabilities, not
  effort; 5 gaps gate ~14 of 19"), Historian (lesson 1 — "build the target backends FIRST;
  mise's migration only became possible once backends existed"), Analogist (keystone S1 —
  ubi-style auto-detect), Systems-thinker (B1 — "raise the native path's capability so its
  activation energy drops below custom-script's"), Journalist ({{.OS}}/{{.Arch}} +
  version-pin block migration), Futurist (auto-resolution won, _with_ verification).
  Verification log **F3, F5** (High).
- **Implication:** this is the true migration unblocker — but it presupposes you can
  **verify** the result (H1) and **test one tool in isolation** (H5-lite `--only`).

### H3 — Governance-first

Code won't shrink the hatch; incentives will. Freeze new `custom-script` entries by policy
(CI lint / ratchet), add Renovate auto-bump to kill the pin-maintenance tax, and **reframe
`custom-script` as a permanent, governed adapter** for the irreducible remainder. Process
beats code.

- **Supported by:** Historian (lesson 3 — "freeze new entries: the single highest-leverage
  anti-stall move"; base rate: escape hatches don't shrink passively), Analogist (three
  gradient-benders; S3 doctrine, S4 lint, S2 Renovate), Systems-thinker (#2 reframe =
  highest _conceptual_ leverage, "a paragraph"; B2 CI gate), Contrarian (permanent hatch
  is correct design).
- **Implication:** the **reframe/freeze half is nearly free and high-value**; the
  **Renovate-automation half implies a lockfile and a bump pipeline the repo lacks** (C2)
  and collides with rolling tools — premature until the lockfile exists.

### H4 — Migrate-the-mechanical-now

Skip the meta-work; just port the ~5–10 clean GitHub-release/toolchain cases with today's
backends and accept the residual. Ship deletions now.

- **Supported by (partially):** Contrarian (selective migration of _mechanically clean_
  cases), Journalist (migrate mechanical, keep imperative), Futurist (simple release
  binaries "collapse cleanly onto this backend").
- **Implication:** attractive velocity — **but the code evidence says "today's backends"
  can't actually hold most mechanical cases** (F3), and there is no `--only` to test a port
  or `Binary` field to verify it (Negative-space). The one existing native port is broken
  (F4).

### H5 — Day-2 UX-first

`doctor` / `self-update` / `--json` / `--only` / `NO_COLOR` deliver more real user value
than script retirement — arguably retirement is not worth doing at all versus closing the
day-2 gap.

- **Supported by:** Journalist (doctor table-stakes; self-update a 1-day feature;
  `--only`/`--json` cheap high-value), Negative-space (`--json` is the "most out-of-
  character gap"; day-2 verb set entirely missing; `--only` and `doctor` are migration
  _enablers_), Futurist (`--json` is the best AI bet — "CLI is the new MCP").
- **Implication:** the cheap bits (`--json`, `--only`, `self-update`) are probe-independent
  and _enable_ the migration loop; **`doctor` is only as truthful as `Installed()`** and
  must wait for H1. The strong form ("UX _instead of_ retirement") overreaches.

### H6 — Status quo is correct

`custom-script`-as-permanent is fine; additive-only is a feature; LOC-deleted is a vanity
metric. Do minimal fixes only and stop worrying about the 19.

- **Supported by:** Contrarian (permanent escape hatch is the industry equilibrium;
  rewriting hardened shell is negative-value per Spolsky), Historian (additive-only is on-
  lineage; irreducible remainder is correct), Journalist/Futurist (reject Nix; scope
  discipline).
- **Implication:** the **philosophy is right for the ≈9 setup-steps**, but "do minimal /
  status quo is fine" **ignores four confirmed defects** (F1/F2/F4/F5) that the Contrarian
  itself insists must be fixed.

### H7 (synthesis, emergent) — Sequenced phases, not rivals

The five surviving/partial hypotheses are **stages of one ordered pipeline** joined by a
dependency graph, with H1 at the root. Presented in Sequencing below.

---

## Evidence Analysis

### Matrix (C = consistent/confirms · I = inconsistent/disconfirms · N = neutral)

ACH weights **I (disconfirming)** most heavily. Highest-weight rows first
(verification-log F-items are code-verified ground truth).

| #   | Evidence (weight)                                                                                                               | H1 corr | H2 capab |      H3 govern      | H4 migrate-now | H5 UX | H6 status-quo |
| --- | ------------------------------------------------------------------------------------------------------------------------------- | :-----: | :------: | :-----------------: | :------------: | :---: | :-----------: |
| E1  | **F1** `Installed()`=bare `LookPath`, misreports ≥8; right technique in `pkg.go` (High)                                         |  **C**  |    N     |        **I**        |     **I**      |   I   |     **I**     |
| E2  | **F2** apply never gates on `Installed()`; re-runs all; rolling self-update (High)                                              |  **C**  |    N     |          I          |       I        |   N   |     **I**     |
| E3  | **F3** github-release templating exists but vocab too small; 1 sha256 can't cover arch; archive-only (High)                     |    C    |  **C**   |          N          |     **I**      |   N   |       I       |
| E4  | **F4** starship script-url pin = rotating time-bomb, default intent, likely broken today (High struct.)                         |  **C**  |    N     |          I          |     **I**      |   C   |     **I**     |
| E5  | **F5** apt-repo hard-codes `noble`; no rpm-repo (High)                                                                          |  **C**  |  **C**   |          N          |       I        |   N   |     **I**     |
| E6  | **F6** doc drift 45→307 lines; "7-pkg skeleton"=37 (High)                                                                       |    C    |    N     |          C          |       N        |   N   |       I       |
| E7  | Historian base rate: hatches don't shrink passively; mise won via backends-first + freeze + scheduled deletion (High)           |    N    |  **C**   |        **C**        |       I        |   N   |       I       |
| E8  | ≥80%-LOC metric is wrong shape (horizontal never completes; counts setup-steps as failures) (High)                              |    N    |    N     |        **C**        |       I        |   N   |       C       |
| E9  | Archaeologist: migration blocked on 5 missing capabilities, gate ~14/19 (High)                                                  |   I*    |  **C**   |          N          |     **I**      |   N   |       I       |
| E10 | Analogist: gradient INVERTED — native is _more_ work than the hatch; auto-detect = keystone (High)                              |    N    |  **C**   |          C          |       I        |   N   |       I       |
| E11 | Systems/Neg-space: idempotency+verification trapped in scripts; can't verify a port without `Binary` (High)                     |  **C**  |  **I**   |          I          |     **I**      |   I   |       I       |
| E12 | Contrarian/Spolsky: rewriting hardened shell is negative-value; the one native port is broken → premise self-refutes (Med-High) |  **C**  |  **I**   |          N          |     **I**      |   C   |       C       |
| E13 | Journalist/Futurist: industry = manifest+lockfile+checksum+Renovate+cosign; doctor table-stakes; self-update 1-day (High)       |    N    |    C     |          C          |       C        | **C** |       I       |
| E14 | Systems/Journalist: doctor is only as truthful as `Installed()` → probe BEFORE doctor (High)                                    |  **C**  |    N     |          N          |       N        | **I** |       N       |
| E15 | Neg-space: no `--only<pkg>` → can't test one ported tool → blocks vertical-slice loop (High)                                    |    C    |    I*    |          N          |     **I**      | **C** |       N       |
| E16 | Contrarian/Historian/Arch: ≈9/19 are genuine setup-steps a native port makes worse (High)                                       |    N    |    N     |        **C**        |       I        |   N   |     **C**     |
| E17 | **C2** lockfile high-value but implies a Renovate pipeline the repo lacks + collides with rolling tools (High)                  |    N    |    N     | **I** (autom. half) |       N        |   N   |       C       |
| E18 | Analogist/Journalist: additive-only vindicated by Nix-fatigue; scope discipline is a strength (Med-High)                        |    N    |    N     |          C          |       N        |   N   |     **C**     |

`I*` = a _sufficiency_ tension, not a "wrong-order" disconfirmation: E9/E15 say H1 is
**necessary but not sufficient** (correctness alone doesn't retire scripts), and say H2 is
**necessary but must wait** for the probe/`--only` enablers.

### Critical disconfirming evidence (the load-bearing I's)

1. **Against H4 (migrate-now):** E3 + E9 + E12 + E15. This is the strongest cluster.
   "Today's backends" demonstrably cannot express the mechanical cases (hadolint's raw
   `x86_64`/capital `Linux`, lefthook's `.gz` single binary, tea's Gitea host +
   `checksums.txt` lookup, AWS's `.pkg`), you cannot verify a port landed (no `Binary`
   field), you cannot test one tool without an entire intent (no `--only`), and the single
   port that _was_ attempted (starship) is **broken on the default path today**. H4-first
   walks straight into a wall.
2. **Against H6 (status quo):** E1 + E2 + E4 + E5. Four **code-verified** defects. Even the
   Contrarian — the persona _arguing against migration_ — insists these be fixed. "Do
   minimal" cannot mean "leave `status` lying and starship broken."
3. **Against H2-as-first-move:** E11 + E12 + E15. Capabilities built before the probe field
   produce an **unverifiable** migration; building them to replace working, hardened shell
   is the classic rewrite trap; and you can't even exercise a ported tool in isolation.
   H2 is right, but **second**.
4. **Against H3's automation half:** E17 (+E1/E2). Renovate/lockfile automation presupposes
   a lockfile the repo doesn't have and a maintenance pipeline (none), and collides with the
   rolling tools that _want_ to float. The **reframe/freeze** half (E7/E8/E16) has no such
   disconfirmation and is nearly free.
5. **Against H5's `doctor` half:** E14. A `doctor` built on today's `LookPath` probe would
   confidently report a fully-provisioned host as "5 tools missing" and **burn user trust**
   — a diagnostic that lies is worse than none. `doctor` must follow the probe fix; the
   **rest of H5** (`--only`/`--json`/`self-update`) has no such dependency.

### The convergence signal (strongest confirming evidence)

The probe/`Installed()` fix is independently named as **the** or **a** top move by six of
eight personas (systems-thinker, negative-space, contrarian, historian, archaeologist,
analogist) **and** by verification-log F1/F2. No other candidate has this breadth of
independent, code-grounded agreement. In ACH terms, it is the item most consistent with the
most evidence while attracting the least disconfirmation.

---

## Hypothesis Survival Analysis

### Eliminated

- **H4 (Migrate-the-mechanical-now with today's backends) — ELIMINATED as a first move.**
  Killed by E3/E9/E11/E12/E15. Not eliminated as an _eventual_ activity — vertical,
  delete-on-cutover migration is exactly right _after_ Phase 1–2. Eliminated only as the
  **opening** move, because it is blocked on verification (`Binary`), isolation (`--only`),
  and backend vocabulary the code doesn't yet have. **Confidence: High.**
- **H6 (Status quo / do-minimal) — ELIMINATED as a posture of inaction.** Killed by the
  four confirmed defects E1/E2/E4/E5. **However, its kernel is correct and survives inside
  the synthesis:** additive-only is right (E18), the ≈9 setup-steps _are_ a permanent tier
  (E16), and the ≥80%-LOC metric _is_ the wrong target (E8). H6 is best read not as "do
  nothing" but as a **guardrail against over-migration** — and in that role it is
  vindicated. **Confidence: High** on eliminating inaction; **High** on absorbing the
  kernel.

### Split (partially survive)

- **H3 (Governance-first) — SPLIT.** The **reframe + freeze + metric re-scope + doc-drift
  fixes** survive as a Phase-0 co-move (cheap, high-leverage, well-supported by
  E7/E8/E16). The **Renovate/lockfile-automation** portion is deferred behind the lockfile
  (E17). Governance is _not_ the whole first move, but its cheap half belongs first.
- **H5 (Day-2 UX-first) — SPLIT.** `--only`, `--json`, `self-update`, `NO_COLOR` survive as
  cheap co-first **enablers** (E13/E15 — `--only` is literally the migration test harness).
  `doctor` is demoted to post-probe (E14). The strong "UX instead of retirement" framing is
  disconfirmed by the entanglement of `--only`/`doctor` _with_ the migration.

### Surviving

- **H1 (Correctness-first) — SURVIVES, strongest.** Attracts **no material disconfirming
  evidence**; only a _sufficiency_ caveat (it doesn't retire scripts by itself). It is the
  root of the dependency graph: honest `status`, idempotent `apply`, verifiable migration,
  and truthful `doctor`/`update`/lockfile all hang off it. **Confidence: High.**
- **H2 (Capability-first) — SURVIVES, sequenced second.** The genuine migration unblocker
  (E9/E10/E3), but disconfirmed as the _opening_ move (E11/E12/E15) because it presupposes
  verification and isolation. **Confidence: High** on validity, **Medium** on internal
  ordering of the capabilities.

### The meta-finding

**H1 and H2 are not competitors; they are Phase 1 and Phase 2 of one pipeline**, and the
probe field is the _hinge_ between them — it is simultaneously the last correctness fix
(H1) and the first schema capability (H2). H3-lite and H5-lite are cheap co-first work;
H4 is the payoff phase; H3-automation and H5-doctor are day-2. **The answer is an ordering,
not a winner.**

---

## Relative Strength / Recommended Sequencing

Dependency-ordered. Each phase lists what it unblocks.

**Phase 0 — Free reframes (hours; pure text; H3-lite + H6-kernel + F6).**

- Rename/segregate a **`setup-step` tier** for the ≈9 irreducible tools (tpm git-clone,
  tmux-sessionizer cargo-git, gnu-utilities brew-loop, mise/rust/lua rolling managers).
- **Re-scope the success metric** from "≥80% LOC retired" to **"`custom-script` ≤ ~5
  entries; each residual named, justified, idempotent; delete-on-cutover per tool."**
- Fix doc drift (307-line shim, ~37-package manifest; stop the README teaching the legacy
  `bin/install-*.sh` path).
- _Unblocks:_ dissolves the false "migration is failing" framing; stops chasing a vanity
  metric; makes the current 19-count legible as ~10 migratable + ~9 permanent.

**Phase 1 — The keystone primitive (H1 + H5-lite enablers). THE FIRST CODE.**

- **`Package.Binary` / `Provides []string` / `installed_when {path|command}` field**, and
  a real **`Installed()` gate wired into the apply/install loop**, reusing `pkg.go`'s
  authority-query discipline (`pacman -Q`/`dpkg-query`/`rpm -q`/`brew list`) where an
  authority exists.
- **`--only <pkg>` / `--skip <pkg>`** (expose the per-package selection the TUI already
  has to the headless path) — the migration test harness.
- **`--json`** on `status`/`plan`/`diff`; `NO_COLOR`; stdout/stderr discipline (cheap,
  probe-independent).
- Hardening riders: **un-pin/version-pin starship** (F4), **template the apt codename** from
  `/etc/os-release` (F5). Add the regression test that currently doesn't exist (Negative-
  space: the `Installed()` bug is untested → load-bearing).
- _Unblocks:_ honest `status`/`diff`; idempotent, network-cheap `apply`; **verifiable
  migration**; `doctor`; `update`; lockfile.

**Phase 2 — Capabilities (H2), gradient-inverting.**

- **`git-clone` backend** (retires tpm cleanly). **Arch/OS vocabulary** (`x86_64`/`aarch64`;
  `ReleaseOS()`→`linux`/`darwin`; case helpers) + **non-archive assets** (bare binary,
  `.gz`). **Per-arch checksum / `dotf.lock`** (fixes the "one sha256 can't cover a templated
  asset" trap). **`post_install` bounded hook.** **ubi/eget-style auto-detect** as an
  optional mode so native becomes _cheaper_ than the hatch. **`rpm-repo` backend** (Fedora
  parity). Consider `requires = [...]` edges for the few real dependencies
  (tmux-sessionizer→rust).
- _Unblocks:_ clean migration of the ~10 mechanical tools; multi-arch integrity.

**Phase 3 — Migrate, vertically, delete-on-cutover (H4 done right).**

- One tool per PR: port → prove with `dotf install --only <tool>` → confirm via the new
  `Installed()` probe → **delete the `bin/install-*.sh`**. Order by interest rate (E9/D1):
  simple, frequently-versioned release binaries first (lefthook, tea, coderabbit, atuin,
  turso), setup-steps never.
- _Unblocks:_ the actual LOC reduction, now measured by the Phase-0 metric.

**Phase 4 — Day-2 + governance automation (H3-automation + H5-doctor + futurist).**

- **`doctor`** (now truthful — pin liveness, PATH sanity, toolchain presence, manifest
  validity). **`self-update`** (go-selfupdate + the GoReleaser `checksums.txt` the shim
  already verifies). **CI freeze-lint** (ratchet on `custom-script` count). **Renovate**
  bump for `tag`/`sha256`/lockfile (verify Forgejo support first). **cosign + SBOM** in
  `.goreleaser.yml` (one block; supply-chain baseline).
- _Optional/EDGE:_ `dotf update` for rolling tools only; `export`/`snapshot`; symlink
  `unlink`.

**Why this order dominates every single-hypothesis ordering:** it front-loads the item with
the most independent support and least disconfirmation (Phase 1), refuses to build
capabilities or migrate before you can _verify_ the result (kills the H2-first and H4-first
traps), keeps the free conceptual wins (Phase 0) off the critical path, and defers the two
items with real hidden costs — Renovate automation and `doctor` — until their prerequisites
exist.

---

## Discriminating Evidence Needed

The findings that would **change the recommendation or its ordering**:

1. **Live re-fetch of the starship installer hash.** F4 is High on the _structural_ risk
   but only Medium on "broken today" (fetched over a sandboxed network). Re-run
   `curl -fsSL https://starship.rs/install.sh | sha256sum`. **If it mismatches the pinned
   `e9f0bb84…`, starship un-pin jumps from a Phase-1 rider to an immediate hotfix** (the
   default `dotf apply` is failing on every fresh machine right now).
2. **Can a `probe`/`Binary` field cleanly express multi-binary bundles?** `hashicorp-tools`
   installs terraform+vault+packer under one `name`. Prototype `provides = ["terraform",
"vault"]` (all-present ⇒ installed) before committing the schema. **If it needs per-tool
   splitting, the manifest shape changes** (Systems-thinker uncertainty).
3. **Is re-apply on converged hosts actually biting, or latent?** If opt-in intents are
   rarely re-run, the idempotency payoff of the gate is small — **but the verification /
   honest-status payoff stands regardless**, so this only affects _how loudly_ to sell the
   gate, not whether to build it.
4. **Effort of a Go-native auto-detect pipeline vs. its edge-case tail** (musl, `.exe`,
   multi-artifact). A code spike. **If the tail is cheap, auto-detect becomes the Phase-2
   keystone (invert the gradient early); if expensive, per-arch-checksum + explicit assets
   come first** and auto-detect is a later convenience.
5. **Does Renovate support Forgejo** (dotf's origin) as smoothly as GitHub for regex-manager
   bumps? Gates the Phase-4 automation half of H3.
6. **Per-tool binary-name verification** for coderabbit/turso/tea/opencode/codex/fabric
   (the Contrarian and Negative-space both caveat these). Needed per PR in Phase 3; the
   `fabric`↔pyinvoke collision is a known false-positive risk.

---

## Assumptions Challenged

- **"≥80% of shell LOC retired is the success metric."** **Rejected** by strong consensus
  (Historian: horizontal slice that "never finishes"; Contrarian: vanity metric; Systems-
  thinker: counts correct setup-steps as failures; Archaeologist: should be "≤ ~5
  entries"). The metric's _shape_ is wrong, not just its threshold. Replace with per-tool
  delete-on-cutover.
- **"`custom-script` growing to 19/37 is an embarrassing failure."** **Rejected** — it is
  the natural equilibrium every mature tool reaches (chezmoi models packages _only_ as
  scripts; Terraform provisioners persist ~9 years; asdf residual <25% is counted a
  _success_). Only the _migratable_ subset should trend to zero.
- **"Native Go backend > working hardened shell script, always."** **Rejected** for the
  setup-steps (Spolsky rewrite-trap; the scripts encode real per-vendor arch/OS/cascade
  logic). Accepted only for mechanical single-binary cases.
- **"The github-release backend has no `{{.OS}}`/`{{.Arch}}` templating" (the objective's
  own premise).** **Corrected** (F3, Negative-space): templating _exists_; the real gap is
  **vocabulary + per-arch checksum + archive/binary flexibility**. The brief was partly
  wrong; the crucible must not inherit the error.
- **"Migration is the highest-value work on the roadmap."** **Challenged** (Contrarian: it
  is the lowest-value/highest-risk item; the confirmed bug-fixes are the real backlog top).
  Reconciled by sequencing: correctness first, migration as the _payoff_ of capability work,
  not the opening bet.
- **"Pinning/checksums = more secure."** **Qualified** — true only for _immutable_
  artifacts. Pinning a rotating vendor installer (starship) is self-defeating (F4). Pin
  release assets; keep genuinely-rolling installers tolerant or move them to
  `github-release`.
- **"Additive-only is a limitation to fix with `--prune`."** **Rejected** — it is a
  defensible feature (chezmoi parity; Promise-Theory-correct; vindicated by Nix-fatigue).
  Document it as intent, not debt.
- **"`apply` is idempotent."** **Corrected** — it is convergent-by-script-discipline and
  **non-deterministic** for rolling tools; the gate (Phase 1) is what makes idempotency a
  property of the engine rather than of luck.

---

## Key Insights

1. **The decision is an ordering, not a winner.** ACH's disconfirming lens converts six
   "which first?" rivals into a dependency graph with one root. The productive output is
   the **sequence**, and the sequence is robust because it is derived from _disconfirmation_
   (what each move is blocked on), not from preference.
2. **One field is the hinge of the entire roadmap.** `Binary`/`Provides`/`installed_when` +
   the `Installed()` gate is simultaneously the last correctness fix and the first schema
   capability. It fixes a confirmed shipping bug **and** is the precondition to _verify_ any
   migration — which is why "migrate now" (H4) and "capability-first" (H2) both fail as
   opening moves: **you cannot safely delete a script you cannot prove was replaced.**
3. **The migration stalled for a structural reason, not laziness.** Idempotency and platform
   logic are **trapped inside the scripts** (Systems-thinker), and the native path is
   **more effortful than the escape hatch** (Analogist's inverted gradient). Both are fixed
   by capability + gradient work (Phase 2), not by exhortation — which is why a pure
   governance-first push (H3) without capabilities would just add friction to a wall.
4. **The most valuable "migration" work isn't migration — it's making migration verifiable,
   testable, and legible.** `Binary` (verify), `--only` (test), the metric reframe and
   `setup-step` tier (legible). All three are cheap, and all three are Phase 0–1.
5. **The strongest disconfirming evidence in the whole corpus is self-refuting proof:** the
   _one_ script dotf already "natively" ported (starship via `script-url`) is broken on the
   default path today. That single data point simultaneously eliminates H4-first, deflates
   the "pinning = hardening" assumption, and validates correctness-first.
6. **H6 was never really "do nothing" — it was "don't over-migrate," and in that reading it
   wins its argument.** The synthesis keeps the ≈9 setup-steps as a blessed permanent tier
   _forever_. The contest was never "retire all vs. keep all"; it was always **triage**, and
   the evidence draws the triage line cleanly at mechanical-binary vs. genuine-setup-step.

---

## Methodology Notes

- **Evidentiary weighting:** the code-verified `verification-log.md` (F1–F6) was treated as
  ground truth and used to adjudicate persona disputes (notably F3, which corrects both the
  objective's "no templating" premise _and_ over-strong persona claims). Persona findings
  were weighted by whether a claim was code-grounded (file:line) vs. web-sourced vs.
  analogical framing; confidence tags were carried through.
- **Making the hypotheses exclusive:** H1–H6 as originally posed overlap (they read as
  phases). I forced exclusivity by binding each to **"the single highest-value _first_
  move,"** which lets ACH test the _ordering_ claim directly. The finding that they are
  phases is therefore a _result_ of the method, not an evasion of it.
- **Disconfirmation focus:** per Heuer, hypotheses were ranked by **least disconfirming
  evidence**, not most confirming. This is what demoted H2 (well-confirmed but disconfirmed-
  as-first) below H1 (unconfirmed-as-sufficient but essentially undisconfirmed), and what
  eliminated H4/H6 despite each having genuine supporters.
- **`I*` (sufficiency vs. order) distinction:** I separated "this evidence says H is _wrong_"
  from "this evidence says H is _necessary but not sufficient / mis-ordered_." Collapsing the
  two would have falsely eliminated H1 (not sufficient) or falsely kept H4 (has supporters).
- **Bias guards applied** (from the objective): feature-creep, rewrite, framework, and
  novelty biases were used as active filters — they are why the synthesis defers `doctor`,
  keeps setup-steps as shell, and treats the reframe/metric as first-class deliverables
  rather than "just docs."
- **Residual squad tension deliberately preserved** (C1/C2/C3): migrate-vs-permanent is
  resolved by triage; lockfile is scoped to pinned artifacts only; `self-update` in-scope,
  `update`-tools EDGE. None were smoothed into false consensus.

---

## Confidence Assessment

- **High** — the probe/`Installed()` fix is the correct, highest-leverage first move.
  Six-of-eight-persona + verification-log convergence; F1/F2 are code-verified; no material
  disconfirmation.
- **High** — H4 (migrate-now) and H6-as-inaction are eliminated as opening moves; H6's
  kernel is correct and absorbed.
- **High** — the answer is a **sequence**, and the Phase 0→1 front (reframe + probe +
  `--only`/`--json` + starship/apt fixes) is right.
- **Medium-High** — the Phase 2→4 ordering (capabilities → migrate → day-2/governance). The
  _phase_ boundaries are firm; the _within-phase_ order (auto-detect vs. per-arch-checksum
  first; which tools migrate first) depends on the code-spike and per-tool discriminators
  above.
- **Medium** — the "broken today" starship claim (re-verify live before treating as a
  hotfix) and the multi-binary `provides` schema shape (prototype before committing).
- **Overall evidence quality: High.** Unusually strong base: a code-verified log plus eight
  independent personas citing file:line, converging on the same keystone from six different
  epistemic angles (systems, absence, contrarian, historical, archaeological, analogical).
  The principal residual uncertainty is **effort estimation**, not **direction**.

---

### Digest

- **Surviving answer — a sequence, not a winner.** **H1 (Correctness-first)** is the root;
  **H2 (Capability-first)** is Phase 2, not the opener; **H3** and **H5** split into cheap
  co-first halves (reframe/`--only`/`--json`) and deferred halves (Renovate/`doctor`);
  **H4 (migrate-now)** and **H6 (do-nothing)** are eliminated as first moves, though H6's
  triage philosophy (keep the ≈9 setup-steps forever) is correct and absorbed.
- **Single highest-leverage first move:** add a **`Package.Binary` / `Provides` /
  `installed_when` field and wire a real `Installed()` gate into the apply loop** (reusing
  `pkg.go`'s authority-query discipline). One field + one gate fixes the confirmed
  `status`-lies bug, makes `apply` idempotent, and is the **precondition to verify any
  migration** — you cannot safely delete a script you cannot prove was replaced. Its free
  co-move: **reframe the metric** ("custom-script ≤ ~5, delete-on-cutover") and **name the
  permanent `setup-step` tier**.
- **Confidence: High** on the first move and on the sequence's front; **Medium-High** on the
  later phase ordering; one live re-verify (starship hash) could promote apt/starship
  hardening to an immediate hotfix.
