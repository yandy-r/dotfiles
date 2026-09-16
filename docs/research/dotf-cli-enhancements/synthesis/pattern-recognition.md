# Pattern Recognition — Emergent Insights Across the dotf Corpus

_Pattern Recognizer (Phase 3, emergent-insight). Asymmetric Research Squad. Date:
2026-07-04. Inputs: all 8 persona findings, `synthesis/crucible-analysis.md`,
`synthesis/contradiction-mapping.md`, `evidence/verification-log.md`._

> **Mandate:** find the non-obvious patterns that emerge only from the _totality_ —
> connections no single persona fully stated. Not a re-summary of the findings; a
> map of the structure _underneath_ them. Where a pattern was named by one persona, the
> job is to show it is the same phenomenon another persona named differently, or to
> extend it to a shape none of them drew.

---

## Executive Summary

The eight personas, the crucible, and the contradiction map read like eight different
instruments recording one seismic event. Laid over each other, they reveal that **almost
every finding in the corpus is a different projection of a single underlying structure**:
a two-tier provisioner whose declarative tier is _more expensive_ than its imperative
escape hatch, whose engine _cannot observe its own state_, and whose success metric
_measures the one axis on which it must fail_. The patterns below are the shadows this
one object casts on eight different walls.

Six patterns dominate, and they are not independent — they form **one causal ladder**
(§Causal Ladder) running from a root cause (idempotency knowledge is _trapped inside the
scripts_) through a mechanism (the effort gradient is therefore _inverted_) to a dynamic
(the hatch _grows_ by reinforcing loop) to a measurement failure (the LOC metric counts
the trapped-knowledge scripts as _failures_ and never converges). Two further structural
facts sit orthogonal to that ladder: the engine's **read path and write path are
disconnected** (it asserts a convergence it never measures), and **two orthogonal
partitions of the 19 scripts turn out to be nearly the same line** (setup-step/mechanical
≈ rolling/pinned), which means one taxonomy field resolves four separate tensions at once.

The most striking _second-order_ pattern is epistemic: **the same keystone fix wears four
different names across four personas** (Systems' `probe`, Negative-Space's `Binary/Provides`,
Archaeologist's `creates:`, Contrarian's "fix `Installed()`"), and the same _inverted
gradient_ wears three (Analogist's economics, Systems' R1 loop, Historian's strangler-fig
base rate). When N deliberately-divergent lenses converge on one locus under N different
vocabularies, that locus is the load-bearing wall — and the corpus points, overwhelmingly,
at **one schema field wired into one code path**.

Confidence: **High** on the structural patterns (each triangulated across ≥3 personas +
code-verified ground truth); **Medium-High** on the cross-domain framings (analogical, used
to name structure, not to justify code).

---

## The Causal Ladder (the spine that unifies six findings)

The single most important thing the totality reveals — and that no persona drew end to
end — is that six of the headline findings are **one causal chain**, not six problems.
Read top to bottom, each rung _causes_ the next:

```
  ROOT CAUSE  ── Idempotency + hard-won platform knowledge is TRAPPED inside the 19 scripts.
  (Systems #2,     Each bin/install-*.sh self-skips, self-updates, and carries per-vendor
   Archaeologist)  arch/OS logic. The manifest entry is a 3-line pointer that cannot express
                   what the script DOES. Migration ≠ translation; it is re-authoring.
        │
        ▼
  MECHANISM  ──── The native path is therefore MORE expensive than the escape hatch.
  (Analogist,      github-release wants a hand-authored asset + pinned tag + computed
   inverted grad.) sha256 + re-implemented self-skip; custom-script is one line at an
                   already-trusted script. The declarative tier costs MORE than the hatch.
        │
        ▼
  DYNAMIC  ─────── Systems drift toward the low-activation-energy path → custom-script grows.
  (Systems R1)     Reinforcing loop: each new tool copies the cheap path; each migration
                   gets more expensive as more logic accretes in shell. 19/37 today.
        │
        ▼
  BASE RATE  ───── Escape hatches don't decay passively; left alone they trend UP.
  (Historian)      Terraform provisioners ~9yr "last resort," still ubiquitous; asdf
                   plugins <25% even after a funded multi-year mise campaign (the WIN).
        │
        ▼
  MEASUREMENT  ── The "≥80% LOC retired" metric measures the aggregate, so it counts the
  FAILURE          trapped-knowledge scripts (where the LOC lives) as failures — and, being
  (Historian,      a HORIZONTAL slice, it "never finishes." The metric is mis-shaped BECAUSE
   Contrarian,     the LOC is concentrated in exactly the scripts that should stay shell.
   Systems)
        │
        ▼
  BLIND SPOT  ─── And the engine can't verify a migration even if it tried: the read path
  (Systems #1,     (Installed()) is wired only to status/diff; apply/install never consult it.
   crucible)       "Convergence" is asserted (exit 0 = no phase errored), never measured.
```

**Why this matters more than any single rung:** it proves the migration did not stall from
_laziness_ (the brief's implicit worry) but from _physics_. You cannot exhort a system up a
gradient. Every persona's prescription that ignores the ladder — "just port the mechanical
ones now" (H4), "freeze new entries by policy" alone (H3), "chase the 80% metric" — pushes
on a rung without re-grading the slope. The **only** interventions that touch the ladder at
its joints are (a) build capabilities so the native path gets _cheaper_ (re-grade the
mechanism rung), and (b) add the probe so migration becomes _verifiable_ (repair the blind
spot). This is why the crucible's Phase-1/Phase-2 ordering is not a preference — it is the
ladder read as a work plan.

---

## Unexpected Patterns

Each pattern: **Type** · **Where it appears** · **Significance** · **Surprise**. Patterns
marked ★ are new (not stated by any single persona); the rest deepen or re-map a candidate.

### P1 — The keystone is an articulation vertex (it faces both directions) ★

- **Type:** Graph-theoretic / structural.
- **Where:** Crucible names the probe field "the hinge between H1 and H2… simultaneously
  the last correctness fix and the first schema capability." Systems calls it "one change,
  four unlocked features." Negative-Space calls it "the tightest blocker — gates
  verification." No persona named its _graph shape_.
- **Significance:** In the roadmap dependency DAG, the probe field
  (`Binary`/`Provides`/`installed_when` + `Installed()` gate) is a **cut vertex** — remove
  it and the graph splits into two disconnected components: _correctness_ (honest status,
  idempotent apply) and _capability_ (verifiable migration, doctor, update, lockfile). It
  is the only node that belongs to both the last correctness fix and the first migration
  capability. That is why it out-ranks every persona's private keystone: they each named a
  node in one component; the probe is the node _joining_ the components. The corpus was
  arguing about which room to fix first without noticing they all connect through one door.
- **Surprise:** High. The candidate framing was "one fix unlocks four features"; the real
  shape is "one fix is the single point whose removal disconnects the entire roadmap."

### P2 — The inverted gradient is triple-encoded (economics = feedback loop = base rate) ★

- **Type:** Cross-domain convergence / systems-dynamics.
- **Where:** Analogist models it as an _economic_ gradient (native path is more work →
  path of least resistance wins). Systems models the _same phenomenon_ as reinforcing loop
  **R1** (activation energy near-zero for the hatch → count grows → migration gets more
  expensive → back to top). Historian models it as an _empirical base rate_ (escape hatches
  measurably don't decay: Terraform ~9yr, asdf <25%). Three personas, three vocabularies,
  one object.
- **Significance:** The triple-encoding is itself the proof of load-bearing-ness. A
  finding one persona reaches is a hypothesis; a finding three independent epistemic
  methods reach under three names is a _law_. And each encoding contributes a different
  actionable: economics says _re-grade the slope_ (make native cheaper — ubi auto-detect);
  systems says _attack the loop at its structure_ (Meadows level 10, not level 12 — don't
  hand-migrate entries); history says _the growth is the default, so stopping growth (a
  freeze) beats shrinking_. You need all three prescriptions because they target three
  points on the same curve.
- **Surprise:** Medium-High. The individual finding is well-stated; the _isomorphism_
  across three personas — that economics, control theory, and historiography are describing
  identical dynamics — is only visible from the totality.

### P3 — Two orthogonal partitions collapse to nearly one line ★ (the hidden organizing principle)

- **Type:** Taxonomic / structural.
- **Where:** The **migration axis** (Contrarian/Systems/Archaeologist): _mechanical-binary_
  (migrate) vs _genuine-setup-step_ (keep shell forever). The **reproducibility axis**
  (contradiction-map PT3, C3/C7): _pinned-immutable-artifact_ (lock, determinism) vs
  _rolling-manager_ (float by design). These were treated as two separate distinctions
  resolving two separate debates.
- **Significance:** They are **nearly the same cut**. The rolling managers (mise, rust,
  atuin) are exactly the tools that stay shell _and_ float by design — they sit on the
  "keep" side of both axes. The pinned binaries (lefthook, tea, coderabbit, turso) sit on
  the "migrate/lock" side of both. The setup-steps are a _superset_ of the rolling managers
  (they also include tpm's git-clone and gnu-utilities' brew-loop, which are neither rolling
  nor pinned — they're path-present-or-not). So the corpus has been debating migration
  triage (C1/C11), lockfile scope (C3), update semantics (C4), and the determinism contract
  (C7) as **four separate tensions when they are four projections of one missing taxonomy.**
  A single first-class manifest field — a _tool class_ (`artifact` | `rolling` |
  `setup-step`) — would simultaneously define: the migration backlog (migrate `artifact`,
  keep the rest), the lockfile scope (lock `artifact`, never `rolling`), the update
  semantics (`update` re-resolves `rolling` only), and the determinism promise (deterministic
  for `artifact`, intentionally-floating for `rolling`). **Contradiction-map pattern #2
  ("split the population") noticed one axis resolves three tensions; the emergent finding is
  that both axes are the same population split, so the manifest should make it first-class
  alongside the probe.** Two schema additions — `probe` (read-side truth) + `class`
  (write-side policy) — resolve the bulk of the corpus.
- **Surprise:** High. This is the "hidden organizing principle" the mandate asked about,
  and the answer is _yes_ — but the deeper surprise is that the tool-class and the
  setup-step tier are the same partition, so naming one names the other.

### P4 — The proof-of-concept refutes the thesis (the keystone counterexample) ★

- **Type:** Epistemic / self-refutation.
- **Where:** Contrarian's thesis and the crucible's "self-refuting proof": the _one_ script
  dotf already ported natively (starship via `script-url`) is **broken on the default path
  today** — a rotating vendor installer pinned to a hard sha256 fails closed when upstream
  rev's the script (F4).
- **Significance:** A single data point does triple duty: (a) it _eliminates_ "migrate-now"
  as a first move (H4) — the showcase migration is the counter-evidence; (b) it _deflates_
  the assumption "pinning = hardening" (true only for immutable artifacts); (c) it
  _validates_ correctness-first (H1). One counterexample, three conclusions. This is the
  rare case where the strongest argument against a strategy is the strategy's own flagship
  example. It also feeds P3: starship is a `rolling`/vendor-installer misfiled as a
  pinnable `artifact` — the exact class-confusion the missing taxonomy would prevent.
- **Surprise:** High. The migration's own success story is its most damning evidence.
  (Caveat carried from ground truth: the _structural_ risk is High/act-on-it; the _specific_
  "broken this minute" hash is Medium — re-run `curl … | sha256sum` on the maintainer's
  network. The pattern holds either way: armed-but-undetonated is still a time-bomb.)

### P5 — The apparent gaps are the actual moat (the inversion) ★

- **Type:** Strategic inversion / historical.
- **Where:** Historian ("dotf's most-criticized design choices are its most historically-
  validated ones"), Journalist ("Nix fatigue vindicates the narrow scope"), Futurist ("the
  imperative middle ground is durable"), Contrarian (additive-only is a feature, not debt).
- **Significance:** Four personas independently observe that the four things that read as
  _incompleteness_ — additive-only (no reconciliation), no lockfile, a permanent escape
  hatch, ease-of-exit (plain symlinks, in-repo scripts, no proprietary state) — are exactly
  the choices the **surviving** tools made (Homebrew's documented "no lockfile, rolling
  release"; chezmoi's closed-wontfix on prune; every mature tool keeps a hatch; the Nix
  exodus explicitly cites Nix's _hard_ exit as the reason to leave). The things that look
  like "features dotf is missing" (full declarative reconciliation) are precisely what
  people are _fleeing_. **The corpus inverts the entire framing of the brief:** the research
  was commissioned to close gaps; the emergent finding is that several of those "gaps" are
  the load-bearing moat, and the correct move is to _document them as deliberate decisions_
  (a doc, not a feature) so they read as intent, not omission.
- **Surprise:** High. It reverses the polarity of the research question for a whole cluster
  of "missing" features.

### P6 — The metric measures the one axis on which it must fail (Goodhart, pre-loaded)

- **Type:** Measurement / incentive (Goodhart family).
- **Where:** Historian (horizontal slice "never finishes"), Contrarian ("vanity metric…
  negative engineering value"), Systems ("counts correct setup-steps as failures"),
  Archaeologist ("should be ≤5 entries, not 0"), Analogist ("measure against release-binaries,
  not setup-steps"). Near-unanimous (C2).
- **Significance:** The candidate framing was "horizontal vs vertical slicing." The deeper,
  emergent point ties it to P3 and the Causal Ladder: **the LOC is concentrated in the
  setup-steps** (mise's 4-way cascade, atuin's desktop matrix, the per-vendor arch logic) —
  i.e., in the scripts that _should_ stay shell. So an LOC-retirement target doesn't merely
  fail to converge; it points the maintainer's effort at _exactly the wrong scripts_ and
  brands the correct permanent tier a failure. The metric is a Goodhart trap that was
  mis-specified _at authoring time_ because it was written before the two-tool-class
  taxonomy (P3) existed to reveal that "lines of shell" is not a coherent thing to minimize.
  The fix is not a threshold change — it's a _shape_ change: "custom-script ≤ ~5 entries,
  each named + justified + idempotent; delete-on-cutover per tool," which is convergent
  because it counts _units retired_, not _aggregate mass_.
- **Surprise:** Medium-High. That the metric is not just non-converging but actively
  mis-directing — and that it mis-directs _because_ of the untaxonomized tool classes — is
  only visible once P3 is in hand.

### P7 — The read/write disconnect makes convergence asserted, never measured ★

- **Type:** Control-theory / architectural.
- **Where:** Systems #1 ("the write path never consults the read path"; "convergence is
  asserted, not measured — exit 0 means 'no phase errored,' not 'the machine matches the
  manifest'"). Analogist's homeostasis framing (C1) touches it; Negative-Space frames it as
  a missing test ("no apply-twice idempotency test — _can't_ be written, because apply never
  consults Installed()").
- **Significance:** In control-theory terms, dotf advertises a **closed-loop** controller
  (detect drift → correct → converge) but is wired **open-loop** (act → assume). It has a
  truth oracle (`Installed()`) it does not consult when acting — a thermostat that heats but
  never reads the thermometer. This is upstream of the determinism narrative over-claiming
  (C7), the idempotency being "by luck" (Contrarian), and the untestability (Negative-Space).
  The probe fix (P1) is not just a bug fix; it is the wire that _closes the loop_ — which is
  why it simultaneously delivers honest status, real idempotency, and the ability to write
  the idempotency test that currently _cannot exist_. The absence of the gate and the
  absence of the test are the same hole.
- **Surprise:** Medium-High. The tool's core value proposition (deterministic, verified
  provisioning) rests on an invariant the engine is structurally incapable of observing.

### P8 — Every layer of description lags the code — including the brief that commissioned this ★

- **Type:** Epistemic / documentation-drift.
- **Where:** Negative-Space + Systems (docs describe a tool that no longer exists: "45-line
  shim" is 307 lines; "7-package skeleton" is 37; README teaches the legacy `bin/install-*.sh`
  path). Ground truth F3/F6. Crucially: the **objective.md brief itself** carried a false
  premise — "no `{{.OS}}`/`{{.Arch}}` templating" — that the verification log had to correct
  (F3, F5's `rpm-repo` phrasing).
- **Significance:** The drift is **directional and recursive**. Every descriptive layer
  describes an _earlier, smaller, simpler_ state: the docs describe a leaner tool than
  exists, the README points at the scripts being retired (the doc actively fights the
  roadmap), and even the research brief mis-stated the code — the ground-truth pass had to
  correct _both the brief and a persona_ (Archaeologist's distro-family claim, C5). Pattern:
  in a fast-moving rewrite, **description is a lagging indicator at every altitude**, and
  the highest-leverage cheap wins (fix the docs, correct the premise) are also the ones most
  likely to be skipped because "it's just docs." Systems names docs "a stale system input"
  — the drift isn't cosmetic; it mis-sets the next contributor's (and the next agent's)
  model of the tool.
- **Surprise:** Medium-High. The research had to correct its own commissioning premise —
  the drift reached all the way up to the question being asked.

### P9 — Every governance lever grows its own evasion surface (recursive Goodhart) ★

- **Type:** Incentive / second-order.
- **Where:** Contradiction-map pattern #6; Systems' unintended-consequences section. Each
  fence-the-hatch prescription breeds a new gaming move: CI count-lint → fold three tools
  into one script to keep the count flat; reframe-as-permanent → excuse to never migrate
  anything; the probe field → a _second_ source of truth that can disagree with the script's
  internal self-skip; the idempotency gate → masks a _broken_ install (probe says "present,"
  tool is corrupt, apply skips repair).
- **Significance:** The near-unanimous "governance beats code" prescription (fence, count,
  freeze, reframe) is correct — but **the corpus's own analysis shows every lever needs a
  paired guard**, or it re-creates the problem one level up (Goodhart applied recursively).
  This matters for the roadmap: the freeze policy needs a _named allowlist_ of permanent
  setup-steps (so "not on the list" stays migratable); the CI count must weight by
  scripts-referenced or LOC (so folding doesn't game it); `--force` must mean "ignore probe"
  (so the gate can't mask corruption); the probe must _defer to_ the script's self-skip as an
  inner belt (documented precedence). The pattern is a checklist: **for each control, name
  its evasion and pair a guard.**
- **Surprise:** Medium. Governance is usually treated as the safe, cheap answer; the totality
  shows it is safe only when instrumented against itself.

### P10 — The debt is carried by one stakeholder; the value accrues to another ★

- **Type:** Economic / externality.
- **Where:** Systems' stakeholder analysis (the sharpest under-explored thread). The
  **maintainer** wants fewer scripts (13k LOC is his to maintain); the **stranger cloning
  the repo** needs exactly those scripts' platform-detection edge-cases to survive on a
  distro the maintainer never tested.
- **Significance:** This explains _why the migration feels compelling yet is risky_. The
  cost of the scripts (maintenance burden) lands on the maintainer, who therefore feels the
  pull to delete; the benefit (works-on-my-weird-box robustness) accrues to the stranger,
  who is absent from the decision. Retiring a fat script trades the maintainer's felt-cost
  for the stranger's unfelt-fragility — a classic externality where the decision-maker
  doesn't bear the downside. It sharpens the triage line (P3): a _checksummed github-release
  binary is more robust than a `curl|sh`_ for the stranger too, so migrating the mechanical
  cases is a genuine win for _both_ stakeholders; but retiring a setup-step's cascade
  (mise's brew→AUR→vendor fallback) removes a safety net the stranger silently depends on.
  The externality is why "migrate the mechanical, keep the setup-steps" is not a compromise
  — it's the split that makes _both_ stakeholders better off.
- **Surprise:** Medium. The stakeholder asymmetry is the mechanism behind the whole
  retire-vs-keep debate, and only one persona surfaced it.

---

## The Unlock Graph — what the keystone fix enables

The candidate hypothesis was "one fix unlocks four features." The totality shows it unlocks
**~11 nodes across three dependency tiers**, and — more importantly — the graph _dictates
the sequence_, because several nodes launch broken if built before the root.

**Root (no parent):** `Package.Binary` / `Provides` / `installed_when` field + a real
`Installed()` gate wired into the apply/install loop, reusing `pkg.go`'s authority-query
discipline (`pacman -Q`/`dpkg-query`/`rpm -q`/`brew list`) where an authority exists.

```
                       ┌───────────────────────────────────────────────┐
                       │  ROOT: probe field + Installed() gate in apply │
                       └───────────────────────────────────────────────┘
                                          │
        ┌──────────────┬──────────────┬───┴────────────┬───────────────┬────────────────┐
        ▼              ▼              ▼                ▼               ▼                ▼
  TIER 1 (direct — depend only on the probe)
  honest         idempotent      network-cheap    verifiable       apply-twice      false-drift
  status/diff    apply           apply            migration         idempotency      eliminated for
  (fixes the     (real skip      (no re-download  (delete-on-       REGRESSION TEST  the 8 opt-in
  confirmed bug) path)           on converged)    cutover proof)    (was impossible) tools
        │                                             │
        │                                             │  (P7: closes the control loop —
        │                                             │   the test can only exist now)
        ▼                                             ▼
  TIER 2 (probe + one more thing)
  ┌────────────────────┐   ┌────────────────────┐   ┌─────────────────────────────┐
  │ doctor             │   │ update (tools)     │   │ lockfile (dotf.lock)        │
  │ = probe + env      │   │ = probe + version- │   │ = probe (read) + resolved-  │
  │   checks           │   │   state            │   │   version record (write)    │
  │ MUST follow probe  │   │ EDGE; scope to     │   │ scope = `artifact` class    │
  │ or it LIES →       │   │ rolling class only │   │ only (P3); never `rolling`  │
  │ trust-erosion loop │   │ (P3)               │   │                             │
  └────────────────────┘   └────────────────────┘   └─────────────┬───────────────┘
                                                                    │
                                                                    ▼
  TIER 3 (depend on the lockfile, which depends on the probe)
  Renovate auto-bump ──▶ deterministic reproduce ──▶ cosign/SBOM verify-at-lock-time
  (needs a lockfile      (lockfile + probe =         (dovetails with lockfile;
   to bump)              set-point + measurement)     supply-chain baseline)

  ALSO GATED ON THE ROOT (if ever reconsidered, currently OUT of scope):
  reconciliation / --prune  ── blocked by the SAME name/identity gap the probe fixes;
                               the probe is its prerequisite, not a separate mechanism.
```

**What the graph shape _forces_ (the sequencing rule that falls out, not in):**

1. **doctor, update, and lockfile cannot precede the probe.** A `doctor` built on today's
   `LookPath` probe would confidently report a fully-provisioned cloud host as "5 tools
   missing" (Systems, Journalist) — a diagnostic that lies is _worse than none_ and ignites
   a distrust reinforcing loop. The graph's edges _are_ the build order.
2. **`--only <pkg>` is the sibling root, not a Tier-1 child.** It's probe-independent
   (expose the per-package selection the TUI already has), but it's the _test harness_ for
   the "verifiable migration" node — you cannot prove one ported tool before deleting its
   script without it. Ship it _with_ the probe, not after.
3. **The migration itself is a Tier-1 node, not the root.** The corpus's deepest reordering:
   "migration" is not the first work — it is the _payoff_ of the probe (verify) + `--only`
   (test) + capabilities (P2's re-grade). The highest-value "migration" work isn't migration;
   it's making migration verifiable, testable, and legible — all cheap, all Phase 0–1.

This is the Michael-Feathers legacy-code pattern in disguise: **the 19 scripts are legacy
code, and you cannot safely delete legacy code you have no characterization test for.** The
probe _is_ the characterization harness. "You cannot delete what you cannot prove you
replaced" is the whole roadmap in one sentence.

---

## Cross-Domain Convergences

Places where independent domains — with no shared lineage — arrived at the same structure.
Convergent evolution is the strongest possible evidence that a structure is _forced by the
problem_, not chosen by fashion.

### CD1 — The two-tier shape (declarative default + governed escape hatch) evolved ≥6 times independently

Homebrew (`url`/`sha256` header + Ruby `def install`), Terraform (resources +
provisioners), Ansible (modules + `command`/`shell`), Bazel (rules + `genrule`), chezmoi
(managed files + `run_onchange_` scripts), mise (native backends + `http`). **Six tools,
six unrelated codebases, one structure: a cheap declarative tier and a fenced imperative
escape hatch that is never deleted.** (Analogist's "cross-domain pattern #1"; corroborated by
Historian, Journalist, Futurist, Contrarian.) This is convergent evolution — like the
camera eye evolving independently ~40 times. When six lineages converge, the structure is a
_law of the domain_, and dotf's goal of retiring the hatch to zero is fighting that law. The
emergent read: dotf is not behind these tools by having a big escape hatch; it is _on-shape_
with them and merely lacks the _fence_ (lint/count/class), not the _deletion_.

### CD2 — "Detect once, pin the result" (add-time / apply-time split) recurs in 5 unrelated ecosystems

go.mod/go.sum, Cargo.lock, aqua's `aqua-checksums.json`, chezmoi's `run_onchange_` content
hash, ubi/eget auto-detect → lockfile. **Every one separates _resolution_ (a heuristic run
once, at add-time) from _integrity_ (a frozen record checked every apply-time).** (Analogist
E1/E2; contradiction-map C6/PT2 — two personas independently reconcile auto-detect-vs-pin
the same way; Futurist confirms mise's `ubi`→`github` deprecation landed there too.) The
convergence dissolves what looked like a hard contradiction (auto-detection _weakens_
verifiability vs pinning _strengthens_ it): they are not opposites, they are **two lifecycle
stages of one pipeline**. The deeper pattern this instances: **many of the corpus's
"contradictions" are actually two lifecycle stages mistaken for two positions** — the same
move resolves C6 (auto-detect vs pin = add-time vs apply-time), and it rhymes with C4
(binary-update vs tool-update = two different verbs) and C15 (doctor-now vs doctor-after-probe
= ordering, not disagreement).

### CD3 — Content-hash as the universal idempotency primitive

chezmoi (SHA256-in-a-comment `run_onchange_`), Cargo.lock/go.sum checksums, Nix input hashes,
aqua/ubi/eget/Homebrew sha256 sidecars, CFEngine convergence. (Analogist cross-domain #4;
Archaeologist's "content-hash run-once"; Historian's Forgotten Alternative #1.) Five domains
all reach for a content hash as the unit of "has this changed / is this what I think it is."
This directly fixes dotf's weakest spot — the version-blind `LookPath` probe — and it is the
_mechanism_ behind the two-tool-class split (P3): `artifact` tools content-address the
downloaded binary; `setup-step`/`rolling` tools content-address the _script_ (re-run only
when the script or its pinned version token changes). One primitive, both classes.

### CD4 — Control-theoretic homeostasis (the loop dotf left open)

Configuration management as a feedback controller (desired state = set-point; reconcile loop
= error-correction) appears in CFEngine's convergence (Burgess 1993), IaC idempotency theory,
and Artificial Immune Systems' self/non-self drift detection. (Analogist C1/C3;
Archaeologist's CFEngine dig.) The convergent lesson — _idempotency failures silently break
convergence; a controller that senses "present" but not "correct version" is a weak
controller_ — is exactly P7. dotf sits at the primitive end of a spectrum every mature CM
tool climbed: from open-loop (act-and-assume) → closed-loop (measure-and-correct). The probe

- lockfile move it one rung up that ladder without importing Nix's hermetic ceiling
  (deliberately named as the boundary NOT to cross — Analogist E3).

---

## Historical Echoes

Where the present rhymes with a documented past — and the past already ran the experiment.

### HE1 — dotf's exact migration has a successful precedent, and the thing that made it work is counter-intuitive

mise's migration off asdf's shell-script plugins onto native backends (Historian's headline).
It succeeded **not** by rewriting scripts faster, but by (1) building native backends _first_
(you can't migrate into a backend that can't hold the entry), (2) _freezing new script
entries by acceptance policy_ (the target stops moving), and (3) _containing rather than
eliminating_ the last quarter (<25% residual is the _win_). **Effort spent on a policy gate
bought more convergence than effort spent on the next rewrite.** This is the single most
load-bearing historical echo: dotf faces the migration mise already completed, documented by
the maintainer who ran it. The echo maps 1:1 onto the crucible's phases (capabilities-first =
mise step 1; freeze policy = step 2; setup-step tier = step 3).

### HE2 — Every capability the 19 scripts need has a 1990s ancestor

Archaeologist's through-line: convergence (CFEngine 1993), symlink ownership + two-phase
conflict pre-scan (GNU Stow 1993), `creates:` sentinel (Ansible, ancestor: BSD `/etc/rc` flag
files), declarative answers separated from imperative mechanism (debconf, late-1990s), profile
matched to host attributes (JumpStart/Kickstart, 1990s). **dotf is not inventing — it is
catching up to ideas 20–30 years old**, and the modern tools it benchmarks against
(chezmoi/aqua/mise) are _themselves_ revivals of the same ideas. The echo reframes the whole
roadmap as _archaeology, not R&D_: nothing on the critical path is novel or risky; it is all
proven mechanism awaiting a TOML field. This pairs with HE3 to explain why dotf's position is
_stable_, not transitional.

### HE3 — The declarative pendulum has a stable center, and dotf is already standing on it

Historian's temporal pattern #1 + Journalist's "Nix fatigue" + Futurist's "durable imperative
middle." The industry swung manual → heavy-declarative (Nix/Ansible for laptops) → _back_ to
lightweight hybrid (Homebrew + chezmoi + mise). The surviving equilibrium across every winner
is **"declarative data + a contained imperative escape hatch"** — never full-declarative
(the Nix exodus flees it) and never full-imperative (the pre-2005 fragility). Three personas,
three evidence bases (dotfiles history, current issue-tracker sentiment, 2026–2030 forecast),
one conclusion: dotf is _at_ the center, not en route to it. This is the historical grounding
under P5 (the gaps are the moat): the center is defined precisely by the "gaps" (additive,
hatch-keeping, ease-of-exit) that the pendulum's excursions taught the survivors to keep.

### HE4 — The security-tier ratchet only clicks forward

Historian's temporal pattern #2 + Futurist's supply-chain trajectory. mise and aqua moved the
whole ecosystem toward checksum/signature/attestation-gated backends and _away_ from arbitrary
third-party bash — mise now _refuses new_ unverified entries; aqua ships Cosign/SLSA; EU CRA
reporting begins Sept 2026. **This ratchet only tightens; standing still means falling behind
as the baseline rises.** dotf's 18 unpinned vendor `curl|bash` custom-scripts sit on the wrong
side of a wheel that does not spin back. The echo says the supply-chain work (lockfile,
cosign-sign the checksum file) is not optional polish — it is _keeping pace with a moving
floor_, and dotf is "one GoReleaser block away" from the current line (Futurist). Note the
tension P4 guards: the ratchet rewards pinning _immutable_ artifacts, and punishes pinning
_rotating_ ones (starship) — the ratchet and the two-tool-class split (P3) must move together.

---

## Key Insights

1. **The corpus is one object seen from eight angles.** The Causal Ladder shows six headline
   findings are one chain (trapped knowledge → inverted gradient → reinforcing growth → sticky
   base rate → mis-shaped metric → unmeasurable convergence). The migration stalled from
   _physics, not laziness_ — which is why every prescription that ignores the ladder pushes on
   a rung without re-grading the slope.

2. **One field is a cut vertex (P1).** The probe (`Binary`/`Provides`/`installed_when` + the
   `Installed()` gate) is the only node whose removal disconnects _correctness_ from
   _capability_. It is the last correctness fix and the first migration capability at once —
   which is exactly why four personas nominated it under four names, and why "migrate now" and
   "capabilities first" both fail as opening moves: you cannot delete a script you cannot prove
   was replaced.

3. **Two schema additions resolve most of the corpus (P3).** `probe` (read-side truth) closes
   the control loop (P7); a first-class `class` field (`artifact`|`rolling`|`setup-step`)
   collapses four separate debates — migration triage, lockfile scope, update semantics, the
   determinism contract — into one taxonomy, because the setup-step/mechanical split and the
   rolling/pinned split are _nearly the same line_. The manifest should make the tool class
   first-class; it is the hidden organizing principle.

4. **The apparent gaps are the moat (P5).** Additive-only, no-lockfile, permanent hatch,
   ease-of-exit — the four most-criticized choices are the four the surviving tools made and
   the four the Nix exodus fled _toward_. Several "missing features" should be closed with a
   _doc that names them as decisions_, not a feature. The research question inverts for this
   cluster.

5. **The strongest evidence is a self-refuting showcase (P4).** The one script already ported
   natively is broken on the default path today. One counterexample eliminates migrate-now,
   deflates "pinning = hardening," and validates correctness-first — and it is itself an
   instance of the class-confusion (P3) that the missing taxonomy would prevent.

6. **Convergent evolution is the corpus's most reliable signal.** The two-tier shape evolved
   ≥6 times (CD1); "detect once, pin the result" recurs in 5 ecosystems (CD2); content-hash is
   the universal idempotency primitive (CD3). When unrelated lineages converge, the structure
   is forced by the problem — dotf should adopt these not because competitors have them but
   because the domain keeps _rediscovering_ them.

7. **Every governance lever needs a paired guard (P9).** The near-unanimous "fence the hatch"
   answer is correct only when instrumented against its own Goodhart evasion: freeze needs an
   allowlist, the count-lint needs LOC-weighting, the gate needs a `--force` bypass, the probe
   must defer to the script's inner self-skip. Governance is safe only when it polices itself.

8. **Description lags reality at every altitude, up to and including the brief (P8).** Docs
   describe a leaner tool than exists; the README fights the roadmap; the objective's own
   premise was wrong about the code (F3). The cheapest high-leverage wins are the ones most
   likely to be dismissed as "just docs" — and their staleness mis-programs the next human and
   the next agent.

---

## Digest — the most powerful patterns for shaping the roadmap

- **Ship the cut vertex first.** The probe field + `Installed()` gate is the single node that
  joins correctness to capability (P1); it fixes the confirmed status-lies bug, closes the
  open control loop (P7), and is the characterization harness without which the migration is
  unverifiable. Nothing in Tier 2–3 (doctor/update/lockfile) may precede it — the graph's
  edges are the build order, and a diagnostic on a lying probe launches broken.

- **Make the tool class first-class — it's the hidden organizing principle.** One `class`
  field (`artifact`|`rolling`|`setup-step`) collapses four debates into one taxonomy (P3),
  because the migration split and the reproducibility split are nearly the same line. Paired
  with the probe, two schema additions resolve the bulk of the corpus.

- **Re-grade the gradient; don't exhort up it.** The hatch won by physics, triple-encoded as
  economics + feedback loop + base rate (P2, Causal Ladder). The only interventions that touch
  the slope are (a) capabilities that make native _cheaper_ (ubi-style auto-detect, richer
  arch/OS vocab, non-archive assets, git-clone backend) and (b) a _freeze_ that stops growth —
  because escape hatches trend up, not down, when left alone.

- **Reframe the metric by shape, not threshold (P6).** "≥80% LOC retired" measures the one
  axis on which it must fail — the LOC lives in the setup-steps that should stay. Replace with
  a convergent per-unit target: "custom-script ≤ ~5, each named + justified + idempotent;
  delete-on-cutover." This is a paragraph, and it belongs in Phase 0 with the probe.

- **Treat the gaps as the moat, and document them as decisions (P5).** Additive-only, the
  permanent hatch, ease-of-exit are historically-validated choices (HE3); write them down as
  intent so they stop reading as incompleteness — the cheapest, highest-goodwill win in the
  corpus, alongside fixing the drifted docs (P8).

- **"Detect once, pin the result" dissolves the auto-detect-vs-pin fight (CD2).** Use
  auto-detection to _add_ a tool cheaply (invert the gradient), freeze the resolved
  asset+checksum in a _partial_ lockfile scoped to `artifact` tools only (rolling managers
  float by design). Add-time vs apply-time, not a contradiction.

- **The self-refuting starship port is the roadmap's compass (P4).** Re-verify the pin on the
  maintainer's network, then either version-pin an immutable asset or accept the rotating
  installer as a tolerant shell-out — and never add 18 more hard-pins to rotating installers.
  Pin immutable, tolerate rolling; the ratchet (HE4) rewards exactly this discrimination.

- **Instrument every governance lever against its own gaming, and remember the stranger
  (P9/P10).** Fence-the-hatch works only with paired guards; and the triage line (migrate
  mechanical, keep setup-steps) is not a compromise but the split that makes _both_ the
  maintainer and the absent stranger better off — a checksummed binary beats a `curl|sh` for
  everyone, while a setup-step's fallback cascade is a safety net the stranger silently needs.
