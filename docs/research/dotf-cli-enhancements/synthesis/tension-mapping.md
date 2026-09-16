# Tension Mapping — What a Solo Maintainer Can't Co-Optimize

_Tension Mapper (Phase 3, emergent-insight). Asymmetric Research Squad. Date: 2026-07-04._
_Inputs: 8 persona findings, `crucible-analysis.md`, `contradiction-mapping.md`,
`evidence/verification-log.md`._

> **Division of labor.** The Contradiction Mapper cataloged _who disagrees with whom_
> (position A vs B). This map does the orthogonal job: the **trade-off, value, and
> stakeholder tensions that a single maintainer must navigate when deciding what to build
> in what order** — the axes that _cannot be optimized simultaneously_, and the hard vs.
> merely-felt dependencies that fix the sequence. Where the crucible produced a phase
> ordering, this document explains _why the phases can't be collapsed or reordered_ — and
> where the roadmap is a genuine trade rather than a dependency.

---

## Executive Summary

Almost every "disagreement" in this corpus is really a **pair of goods that can't be
maximized at the same time**, forced into apparent conflict by scarcity of one
maintainer's time. Five such tensions govern the whole roadmap:

1. **LOC-deleted vs. correctness.** The commissioning metric (≥80% shell LOC retired)
   and the highest-value work (probe fix, starship/apt hardening) point in _opposite time
   directions_: the top-value fixes delete **zero** LOC, and chasing the LOC metric
   actively pulls effort _away_ from them. You cannot serve both North Stars; the "metric
   reframe" is the act of choosing correctness over deletion.

2. **Convenience vs. determinism (add-time vs. apply-time).** Auto-detection (the
   github-binary backend) minimizes the effort of _adding_ a tool but is non-deterministic;
   pinning maximizes reproducibility at _apply_ time but is high-effort. The same mechanism
   can't do both — which is exactly why the highest-value migration unblocker (auto-detect)
   **cannot ship before** the thing that fixes its side effect (the lockfile).

3. **Reproducibility vs. rolling-tools-that-float.** dotf owns two tool classes. Any single
   reproducibility promise is a half-truth: pin the rolling managers (mise/rust/atuin) and
   you defeat their purpose; don't pin them and the "pinned/verified" narrative is false.
   The only honest resolution is a _deliberately partial_ story — i.e. choosing **not** to
   fully optimize either pole.

4. **Capability vs. maintenance surface (for ONE person).** Every backend, pin, and
   lockfile entry is also a permanent test-matrix and refresh liability. Several
   candidate items **cost more future maintainer-time than they save** — a lockfile with
   no Renovate, a pin on a rotating installer, a native rewrite of a hardened setup-step.
   The starship pin is the corpus's proof: the one thing already "hardened" this way is a
   time-bomb that has (probably) already gone off.

5. **Maintainer-simplicity vs. stranger-robustness.** The maintainer wants fewer scripts;
   the stranger cloning the repo needs the scripts' platform edge-cases. Retiring a script
   trades one for the other — and the sign of that trade _flips at the triage line_: for
   mechanical single-binary tools, a checksummed native backend serves **both**; for
   genuine setup-steps, a native port serves the maintainer at the stranger's expense.

**The one move that relieves the most tension at once is the probe/`Installed()` fix** —
not because it's the highest _value in isolation_, but because it is the **hinge on which
the other tensions can be safely resolved**: it makes migration _verifiable_ (so
deletion stops being a gamble), makes `apply` idempotent (so the determinism claim can be
made true), and is the hard precondition for `doctor`, `update`, and a version-aware
lockfile. Its free co-move — the metric reframe — resolves tension #1 with a paragraph.

Everything expensive and high-value (auto-detect, lockfile, doctor) is **real work that
must be sequenced, not skipped**; everything cheap-and-contested (self-update, git-clone
backend) is a judgment call to make _quickly and cheaply or not at all_; and two items
(Renovate-before-lockfile, native-rewrite-of-setup-steps) are **expensive-negative** —
avoid.

---

## Maximum Disagreement Points (sequencing-focused)

These are the places where personas most sharply disagree **about order and effort
allocation** — distinct from the factual/interpretive splits the contradiction map owns.
Each is a "you can't do both first" fork.

### D1 — Invert-the-gradient-first vs. make-migration-verifiable-first (the keystone fork)

The single biggest sequencing disagreement. **Analogist** nominates the ubi/eget-style
github-binary auto-detect backend as _the_ keystone ("unlocks the most migrations at
once," a Meadows Rules-level intervention). **Systems-thinker, Negative-space, Contrarian,
Archaeologist** nominate the probe/`Installed()` fix.

- These optimize **different things**: auto-detect optimizes migration _throughput_ (make
  native so cheap the migration happens by itself); the probe optimizes migration _safety_
  (make the result _verifiable_ so you can delete a script without gambling).
- **You cannot do both first**, and they trade throughput against safety. The crucible
  resolves it decisively for probe-first: _you cannot safely delete a script you cannot
  prove was replaced_ — safety gates throughput, and auto-detect without the probe yields
  an **unverifiable** migration (crucible E11/E12/E15). Auto-detect is right, but it is
  **Phase 2, not the opener**.
- **The residual tension worth carrying:** Analogist is correct that the _reason_ the
  hatch grew is the inverted gradient (native is _more_ work than the escape hatch). Probe-
  first fixes verification but does **not** reverse the gradient — so if the maintainer
  ships only the probe and stops, the hatch keeps growing. Probe-first buys safety;
  gradient-inversion (Phase 2) is still required to buy _convergence_.

### D2 — Is the commissioned work (migration) even the highest-value work?

**Contrarian** argues the migration is "the lowest-value, highest-risk item on the
roadmap," and the real backlog top is the four confirmed bug-fixes (probe, starship, apt,
gate). **Journalist/Futurist** treat the lockfile/verification story as the highest-value
gap. **Negative-space** treats day-2 verbs (`--only`, `doctor`, `--json`) as the missing
half of the tool. **Historian/Analogist/Systems** treat the migration as real but
_downstream_ of capability + governance work.

- This is a **value-allocation** tension, not just a sequencing one: the work the brief
  _commissioned_ (retire custom-script) may be lower-value than work it _didn't_ (fix the
  read path, add day-2 UX). Seven of eight personas push back on the framing.
- Resolution direction (strong consensus): the migration is the **payoff phase**, not the
  opening bet. Do correctness + capabilities + `--only` first; migrate as the _result_.

### D3 — Lockfile now (value) vs. lockfile later (maintenance cost)

**Journalist, Analogist, Futurist, Archaeologist** rank the lockfile the highest-value
gap (supply-chain, reproducibility, fixes the sha256-pins-one-arch trap). **Systems-
thinker, Contrarian** rank its _true_ cost high (implies a Renovate pipeline the repo
lacks; collides with rolling tools).

- The disagreement is a **now-vs-later where value and cost point in opposite time
  directions**: the value is front-loaded (closes a real gap immediately), the cost is
  back-loaded (a refresh treadmill that compounds every upstream release). A solo
  maintainer feels the cost long after the value has been booked.
- Resolution: ship it **scoped to the pinnable subset only**, _after_ the probe (so it can
  be version-aware), and **before** any Renovate automation — but resolve the
  Forgejo/Renovate spike first (see M4), because without automated refresh the lockfile's
  value-minus-cost may go _negative_.

### D4 — self-update: conformance ("every peer ships it") vs. minimalism ("a re-run does it")

**Journalist** ranks it IN/high ("solved 1-day feature, every peer ships it"). **Contrarian
/ Negative-space / Systems** rank it low/over-valued ("the shim re-run already updates the
binary"). All agree it's _cheap_; they disagree on whether it's _worth the surface_.

- This is an **irreducible taste tension** the evidence can't settle — it's about surface
  minimalism, not facts. It recurs across the corpus (also self-update's cousin: cosign/SBOM
  conformance vs. solo-hobby minimalism).
- Sequencing verdict: it depends on **nothing** and blocks **nothing**, so it can be done
  anytime or never. Ship it _only_ because it de-dupes the shim's verify logic into the
  binary (Systems' tiebreaker) — never ahead of a correctness fix.

### D5 — Which stakeholder's norms set the bar? (provenance)

**Futurist** reads cosign/SBOM as the 2026 baseline (enterprise/regulatory lens);
**Journalist** reads checksum+lockfile as the 80% and SLSA as aspirational-EDGE (solo-tool
lens). Same evidence, different stakeholder frame. Sequencing verdict: tier it — cosign-
sign the checksum file is cheap (do soon); SBOM/attestation is upstream-dependent (defer).

---

## Effort / Value / Risk Tensions (ranked quadrant table)

Ratings synthesized across personas + verification log. **Value** is to _this tool's
scope_ (single-host, additive, solo). **Risk** includes both implementation risk and the
risk of _shipping it in the wrong order_. Quadrant drives the verdict.

| Item                                                        | Effort          | Value                        | Risk      | Quadrant            | Verdict                                                                   |
| ----------------------------------------------------------- | --------------- | ---------------------------- | --------- | ------------------- | ------------------------------------------------------------------------- |
| **Metric reframe + setup-step tier**                        | Trivial         | High                         | Low*      | cheap·high          | **DO FIRST** (a paragraph; dissolves the false "failing migration" frame) |
| **probe / `Installed()` gate**                              | Med             | **Very High**                | Low       | cheap-ish·very-high | **DO FIRST** (the hinge; 6/8 converge)                                    |
| **`--only` / `--skip` + `--json` + `NO_COLOR`**             | Low             | High                         | Low       | cheap·high          | **DO FIRST** (`--only` _is_ the migration test harness)                   |
| **starship un-pin + apt codename template**                 | Low             | High (stranger)              | Low       | cheap·high          | **DO FIRST** (hardening riders; possible live hotfix)                     |
| **CI freeze-lint (ratchet)**                                | Low             | Med-High                     | Low*      | cheap·med-high      | **DO — but after** the setup-step tier exists (else it miscounts)         |
| **git-clone backend**                                       | Low             | Med (retires ~1: tpm)        | Low       | cheap·med           | **DO IF WANTED** — low payoff; tpm is a _blessed setup-step_ anyway       |
| **github-binary auto-detect**                               | Med-High        | **Very High**                | Med       | expensive·very-high | **DO — SEQUENCE (Phase 2)**, gated on probe + lockfile                    |
| **lockfile (pinnable subset)**                              | Med + treadmill | High                         | Med       | expensive·high      | **DO — SEQUENCE + SCOPE**, after probe, before Renovate                   |
| **post-install hooks**                                      | Med             | Med-High                     | Med       | expensive·med-high  | **DO — CONSTRAINED** (unbounded = re-inventing custom-script)             |
| **doctor**                                                  | Med             | High                         | Low-Med** | expensive·high      | **DO — SEQUENCE**, strictly after probe                                   |
| **self-update**                                             | Low             | Contested Low-Med            | Low       | cheap·low           | **JUDGMENT** — do cheaply or skip; never prioritize                       |
| **rpm-repo backend**                                        | Med             | Low (solo, few Fedora users) | Low       | —                   | **DEFER** (Fedora second-class is tolerable for now)                      |
| **Renovate (before lockfile/Forgejo spike)**                | Med             | ~0 until lockfile            | Med-High  | expensive·low       | **AVOID until prerequisites**                                             |
| **Native rewrite of setup-steps** (gnu-utilities/mise/rust) | High            | **Negative**                 | High      | expensive·negative  | **AVOID** (Spolsky rewrite-trap)                                          |
| **cosign-sign the checksum file**                           | Low             | Med                          | Low       | cheap·med           | **DO SOON** (one GoReleaser block)                                        |
| **SBOM / attestation-verify**                               | Med             | Low (solo)                   | Med       | —                   | **DEFER** (upstream-dependent)                                            |

`*` Governance items carry a _second-order_ risk of gaming (fold-tools-to-flatten-count;
reframe-as-excuse-to-never-migrate) — cheap to build, but need a guard, not just the lever.
`**` doctor's risk is **entirely order-dependent**: Low after the probe, _trust-destroying_
before it (a diagnostic that lies is worse than none).

**The quadrant's load-bearing insight:** the four **cheap·high-value** items (reframe,
probe, `--only`/`--json`, starship/apt) are also the four with the _least_ disconfirming
evidence and the _most_ independent support. Effort, value, risk, and consensus all point
at the same front. That alignment is rare and should be trusted.

---

## Maintainer-Time Tensions

Every feature is a maintenance liability amortized over **one person**. The governing
question isn't "is this valuable?" but "**does it cost more future maintainer-time than it
saves?**" Four items invert to net-negative under that test.

### M1 — The lockfile → Renovate → Forgejo obligation cascade

A lockfile is a _file_ (cheap) that implies a _pipeline_ (expensive). Once you pin, you own
a refresh treadmill: checksums rot on every upstream release. The relief valve is Renovate —
but the repo has **no dependency automation** and its origin is **Forgejo**, where
Renovate's regex-manager support is **unverified** (crucible discriminating-evidence #5;
contradiction C16). So the honest cost curve is:

- lockfile **with** working Renovate → net _saves_ time (pins stay fresh automatically).
- lockfile **without** Renovate → net _costs_ time — "a lockfile you don't keep current is
  worse than none" (Contrarian). Converts a maintenance _option_ into an _obligation_.

**Therefore the Forgejo/Renovate spike is a prerequisite to the lockfile's value, not a
follow-up.** If Renovate can't drive Forgejo, either script a native `dotf lock --update`
bump job first, or narrow the lockfile to the handful of truly-immutable release assets and
accept manual bumps for those alone.

### M2 — Pinning a rotating artifact is time-bomb maintenance (the starship proof)

The corpus's sharpest maintainer-time datum: the _one_ artifact dotf already "hardened"
with a pin (starship's `install.sh`) is on the **default `base` intent** and has (probably,
pending re-verify) **already rotated past its pin** — meaning a fresh `dotf apply` fails on
every new machine _right now_. Pinning bought a maintenance obligation and delivered a
default-path breakage. The lesson bounds the whole pinning/lockfile effort: **pin only
immutable/versioned assets; keep genuinely-rolling installers tolerant** (or move them to
`github-release` where the artifact _is_ immutable). Multiplying starship-style pins across
19 tools multiplies the time-bombs, not the security.

### M3 — More backends → more test matrix, on a mock-only CI

Each new backend (git-clone, rpm-repo, github-binary auto-detect, post-install) multiplies
the per-(distro × backend) argv surface. But **there is no real-distro integration CI** —
the entire matrix is asserted via `exec.Fake` mocks (Negative-space; no nightly, no
dockertest). So every new backend adds _mock-verified_ surface with **no real-execution
safety net**, and the maintenance liability accrues silently until a stranger hits it on
Fedora. This caps how many backends are worth adding: **build the capabilities that retire
the most scripts per unit of new test surface** (github-binary auto-detect is the best
ratio; rpm-repo the worst for a solo user base), and treat a real-distro smoke test as the
true cost of any backend expansion.

### M4 — The auto-detect edge-case tail is an unpriced liability

github-binary auto-detect is the highest-value migration unblocker, but its effort is
**genuinely uncertain**: the heuristic's tail (musl vs. gnu, `.exe`, multi-artifact
bundles, checksum sidecars) is where ubi's own maintainer warns "not part of the API, may
change." If the tail is small, auto-detect is the Phase-2 keystone that inverts the
gradient. If the tail is large, maintaining a bespoke detection pipeline can cost more than
hand-authoring ~10 explicit entries. **A code-spike must price the tail before committing**
(crucible discriminating-evidence #4). This is the one Phase-2 item whose effort rating is
a genuine unknown, not a Medium-High estimate.

### M5 — The maintenance-reducing items are the _un-sexy_ ones

The cruel inversion: the work that most _reduces_ future maintainer-time — the probe (kills
network-per-run + false drift), the metric reframe (kills the vanity-work pull), the
starship un-pin (kills a recurring breakage), the CI freeze (stops the surface from
growing) — are **boring fixes and paragraphs**, while the work that most _increases_ it
(lockfile, Renovate, more backends, self-update) is the shiny, conformance-flavored stuff.
Novelty bias points exactly the wrong way. The maintainer-time-optimal sequence is
front-loaded with the least glamorous items.

---

## Scope-Discipline Tensions

Four scope axes where the maintainer must hold a line, and where the _failure mode is
picking a pole_ rather than the tension.

### S1 — Native-purity vs. permanent-hatch (the flagship)

The commissioning goal implies custom-script → 0. Every mature analogue (chezmoi, Terraform,
Ansible, Bazel, Homebrew, mise) keeps a **governed** hatch forever. You cannot simultaneously
have "everything native/declarative" and "keep the vetted, hard-won shell logic." **The
resolution isn't code — it's redefining success**: minimize the _migratable_ subset toward
zero while _blessing_ the ~9 irreducible setup-steps as a named, permanent tier. The
maintainer must psychologically accept that the metric never hits zero _and that this is
correct_. Guard against both failure modes: "retire all" (the Nix mistake in miniature) and
"reframe-as-permanent as an excuse to never migrate anything" (needs an explicit allowlist so
anything _off_ the list stays a migration candidate).

### S2 — Determinism vs. rolling-tools-that-float

dotf has two tool classes with **two incompatible contracts**. Pinned artifacts want to be
frozen and verified; rolling managers (mise/rust/atuin) want to float and self-update. A
uniform reproducibility story is false either way. The scope-disciplined move is to **state
both contracts explicitly** (Homebrew's documented "no lockfile, rolling release" is the
precedent) and make the lockfile _partial on purpose_. The trap is letting the
"checksum-verified, pinned" marketing language imply a determinism the rolling tools
deliberately violate — and letting silent every-run self-update masquerade as idempotency.

### S3 — Additive-only vs. reconciliation/prune

Additive-only is **vindicated** (chezmoi parity, Promise-Theory-correct, Nix-fatigue
signal) — it is a _feature to document_, not a debt to fix. But two things get wrongly
bundled under "no reconciliation": (a) _package_ prune (correctly deferred) and (b) _symlink_
`unlink` (safe, simple, absent, and worth adding). Scope discipline = **unbundle them**:
defend additive-only for packages; ship symlink unlink as the cheap safe inverse. Note also
that prune, if ever reconsidered, is blocked by the _same_ name/identity gap as the probe —
so the probe is a silent prerequisite even for the out-of-scope feature.

### S4 — Stranger-cloning-the-repo vs. maintainer's-own-machine

The stakeholder conflict _is_ the triage line. The maintainer optimizes for fewer scripts;
the stranger needs the scripts' platform detection. **The sign of the trade flips at
mechanical-vs-setup-step:**

- Mechanical single-binary tools → retiring to a checksummed native backend serves **both**
  (a verified binary is _more_ robust than `curl | sh`). No conflict.
- Genuine setup-steps → a native port serves the maintainer at the **stranger's expense**
  (you'd re-author, and likely degrade, the vetted per-vendor cascade). Real conflict.

So the mechanical/setup-step triage isn't just an effort classification — it's the line
where the _stakeholder interests realign_. Retire where interests align; bless-as-permanent
where they don't. The scope error is a global verdict ("retire all" harms strangers on the
hard tools; "keep all" denies both stakeholders the supply-chain win on the easy ones).

---

## Now-vs-Later Dependency Tensions

Separating **hard dependencies** (X is structurally impossible or unsafe before Y) from
**felt-but-false ordering** (X only _seems_ like it should come first).

### Hard dependencies (the sequence is forced)

| Must come first                      | Before                                | Why it's hard, not stylistic                                                                                                                                                                                          |
| ------------------------------------ | ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **probe / `Installed()`**            | `doctor`                              | A doctor on today's `LookPath` reports a provisioned host as "5 tools missing" → launches broken, burns trust. Trust-inverting, not just inaccurate.                                                                  |
| **probe / `Installed()`**            | `update` (tools)                      | `update` needs a "current vs desired version" notion that only the probe/version-state introduces; without it, `update` collapses into today's `apply`.                                                               |
| **probe / `Installed()`**            | **verifiable migration**              | You cannot safely _delete_ a script when you can't prove the native port installed the tool. This is the crucible's central finding — the hinge of the whole roadmap.                                                 |
| **probe / `Installed()`**            | version-aware lockfile                | A lockfile that records "what's installed" needs a truthful installed-probe to compare against.                                                                                                                       |
| **`--only <pkg>`**                   | vertical-slice migration              | The delete-on-cutover loop requires installing/testing **one** ported tool in isolation; today selection is intent-only (headless) — the CLI physically can't do a vertical slice.                                    |
| **metric reframe + setup-step tier** | CI freeze-lint                        | The lint must know what's a _permanent_ setup-step vs. a _violation_; without the named tier it flags legitimate setup-steps as failures.                                                                             |
| **lockfile**                         | Renovate                              | Renovate's job is to bump lockfile entries; no lockfile → nothing to automate.                                                                                                                                        |
| **Forgejo/Renovate spike**           | committing to lockfile→Renovate story | If Renovate can't drive Forgejo, the lockfile's maintenance-relief valve doesn't exist (see M1).                                                                                                                      |
| **github-binary auto-detect**        | trustworthy re-apply                  | Auto-detect is non-deterministic across resolver versions; without "detect once, **pin the result**" (lockfile), it swaps the escape-hatch's determinism problem for a new one. Soft-hard: they must ship _together_. |

### Felt-but-false ordering (looks urgent/first; isn't)

- **github-binary auto-detect _feels_ like the keystone** (Analogist ranks it #1). It's the
  highest-_value_ migration unblocker but is **disconfirmed as the first move** — it
  presupposes verification (probe) and isolation (`--only`). Phase 2, not Phase 0.
- **The migration itself _feels_ like the headline deliverable.** It's the **payoff phase**,
  downstream of probe + capabilities + `--only`. Front-loading it walks into the F3/F4 wall
  (today's backends can't hold the mechanical cases; the one existing native port is broken).
- **The lockfile _feels_ supply-chain-urgent.** The only _urgent_ piece is the **starship
  un-pin** — a one-line hotfix, not a lockfile. The lockfile can wait behind probe +
  capabilities + the Forgejo spike.
- **doctor _feels_ like table-stakes UX to ship early.** Hard-blocked on the probe.
- **self-update _feels_ conformance-necessary.** Nothing depends on it; it's a free-floating
  judgment call (D4).

### The one item that may legitimately jump the queue

**The starship un-pin is a candidate immediate hotfix** — _independent of the entire
sequence_. If the pin is live-broken (High on structural risk, Medium pending a one-line
`curl … | sha256sum` re-verify on the maintainer's network), the default `dotf apply` is
failing on every fresh machine _today_. This is the sole now-vs-later where "now" may be
_forced by a live defect_ rather than chosen. Re-verify first; if stale, it leapfrogs even
the probe.

---

## Key Insights

1. **The roadmap is a sequence of trades, not a menu.** Six of the seven "which is the
   keystone?" disagreements dissolve into one dependency graph once you ask "what is each
   move _blocked on_?" The productive output is the _order_, and the order is robust
   because it's derived from what each item _can't_ precede — not from preference.

2. **The probe fix wins on _tension relief_, not raw value.** It isn't the single most
   valuable feature; it's the one item that lets the _other_ tensions be resolved safely —
   it makes migration verifiable (relieves the delete-gamble), makes idempotency real
   (relieves the determinism-vs-float contradiction on the pinned side), and unblocks the
   entire day-2 verb family. One field, maximum tension relief.

3. **Value and maintenance-cost point in opposite time directions for the shiny items.**
   Lockfile, Renovate, more backends, self-update all book their value up-front and their
   cost forever-after — on one person. The items that _reduce_ net maintainer-time are the
   boring ones (probe, reframe, un-pin, freeze-lint). Novelty bias inverts the correct
   priority; resist it deliberately.

4. **Two items are expensive-_negative_ — the roadmap's only true "avoid."** Renovate
   _before_ a lockfile/Forgejo spike (value ≈ 0, cost real), and a native rewrite of the
   genuine setup-steps (Spolsky rewrite-trap: deleting vetted shell to add fragile Go). Every
   other item is at worst defer-able; these two are actively harmful in the wrong order.

5. **The triage line does triple duty.** Mechanical-vs-setup-step is simultaneously the
   _effort_ boundary (cheap vs. rewrite), the _stakeholder-interest_ boundary (aligned vs.
   conflicting), and the _metric-shape_ boundary (migratable→0 vs. blessed-permanent).
   Drawing that one line correctly resolves the effort, the stakeholder, and the metric
   tensions at once — which is why "reframe the metric + name the setup-step tier" is the
   highest-leverage _paragraph_ in the whole roadmap.

6. **The determinism claim is the honesty debt that gates the reproducibility features.**
   Until the probe gate exists and self-update is opt-in, "checksum-verified, pinned,
   idempotent" is marketing the engine doesn't deliver. Every reproducibility feature
   (lockfile, doctor's pin-liveness check, attestation) inherits that half-truth. Fix the
   claim (probe + opt-in self-update + partial lockfile + explicit two-contract docs)
   _before_ building features that assume it's true.

7. **Effort estimation — not direction — is the principal residual uncertainty.** The
   _sequence_ is High-confidence; the two things that could move it are both **live checks,
   not more analysis**: (a) re-fetch the starship hash (could promote un-pin to an immediate
   hotfix), and (b) code-spike the auto-detect edge-case tail (prices the one Phase-2 item
   whose effort is genuinely unknown). Buy those two facts before committing Phase-2 effort.

---

### Digest — effort/value/risk verdict per major roadmap item

- **probe / `Installed()` gate** — _Med effort · Very-High value · Low risk._ **DO FIRST.**
  The hinge: fixes the shipping `status`-lies bug, makes `apply` idempotent, and is the hard
  precondition for verifiable migration, doctor, update, and a version-aware lockfile. Every
  tension eases once this lands.

- **Metric reframe + setup-step tier** — _Trivial · High · Low._ **DO FIRST (with the probe).**
  A paragraph that resolves the LOC-vs-correctness tension, dissolves the "failing migration"
  frame, and draws the triage line that also realigns the stakeholder conflict. Highest
  leverage-per-word in the corpus.

- **`--only`/`--skip` + `--json` + `NO_COLOR` + starship un-pin + apt codename** — _Low ·
  High · Low._ **DO FIRST (co-enablers/riders).** `--only` is the migration test harness;
  `--json` is the most out-of-character gap for an agent-driven repo; the starship/apt fixes
  are stranger-facing hardening (starship may be a _live_ hotfix — re-verify).

- **github-binary auto-detect backend** — _Med-High (tail-uncertain) · Very-High · Med._
  **DO — Phase 2, gated on probe + lockfile.** The true gradient-inverter and biggest
  script-retirer, but non-deterministic alone and unverifiable without the probe. Price the
  edge-case tail with a code-spike before committing.

- **Lockfile (pinnable subset)** — _Med + refresh treadmill · High · Med._ **DO — Phase 2,
  scoped + sequenced.** High front-loaded value, back-loaded cost. Pin immutable assets only;
  leave rolling managers floating by design; resolve the Forgejo/Renovate spike _first_ or the
  net value can go negative.

- **doctor** — _Med · High · Low-after-probe / trust-destroying-before._ **DO — strictly after
  the probe.** Table-stakes day-2 UX, but a diagnostic built on today's `LookPath` lies and
  burns trust. Pure order dependency.

- **Post-install hooks + git-clone backend** — _Med / Low · Med-High / Med (~1 tool) · Med /
  Low._ **DO — constrained.** Post-install must be a _bounded_ step list or it re-invents
  custom-script; git-clone cleanly retires tpm but tpm is a blessed setup-step anyway, so the
  payoff is modest — build it for elegance, not for the count.

- **self-update, Renovate, rpm-repo, SBOM, native setup-step rewrites** — _the "not now / not
  ever" tier._ self-update: cheap·contested — do quietly or skip (never prioritize). Renovate:
  **avoid until** lockfile + Forgejo spike. rpm-repo/SBOM: **defer** (low value for a solo
  user base). Native rewrite of gnu-utilities/mise/rust: **avoid** — expensive·_negative_
  (rewrite-trap; keep them as blessed permanent setup-steps).
