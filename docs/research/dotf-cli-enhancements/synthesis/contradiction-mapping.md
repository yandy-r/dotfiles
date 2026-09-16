# Contradiction Mapping — dotf CLI Enhancements & Script-Retirement

_Contradiction Mapper (Asymmetric Research Squad). Synthesis of 8 Phase-1 persona
findings + the orchestrator's code-verified `evidence/verification-log.md`. Date:
2026-07-04._

> **Rule of the room:** when a persona and the verification log disagree on a _code
> fact_, the verification log wins (it was read directly against source on 2026-07-04).
> This map does **not** smooth disagreements — it separates the ones that are genuine
> and load-bearing from the ones the ground truth dissolves, and it flags the
> _productive_ tensions (both sides true in different contexts) that should be preserved,
> not resolved.

---

## Executive Summary

Across the eight personas there is a **large, surprising consensus floor** and a
**small, sharp set of genuine disagreements** sitting on top of it. The consensus:
(1) the `custom-script` `Installed()` PATH-probe is a real correctness bug; (2) the
migration is blocked on _backend capabilities + verification_, not effort; (3)
additive-only / no-prune is a defensible design choice, not a debt; (4) `custom-script`
should never reach zero — a governed residual is correct; (5) the docs have drifted
("45-line shim" is 307 lines; "7-package skeleton" is ~37). On these, all eight agree.

The genuine contradictions cluster into **two families**. The first is _squad-versus-brief_:
seven of eight personas push back on the user's stated goal (retire `custom-script`) and
on the PRD's `≥80%-LOC-retired` metric, arguing both are the _wrong shape_ — the
disagreement is not among the personas but between the unified squad and the artifact
that commissioned them. The second is _squad-internal and productive_: lockfile value vs.
maintenance tax, `update` verb scope, self-update priority, and cosign/SBOM
baseline-vs-aspirational — each a real trade-off where both sides are right in different
contexts.

The single most important thing the ground truth settles: **the "no `{{.OS}}`/`{{.Arch}}`
templating" premise is false** (F3). Templating exists; the real gaps are _vocabulary_
(`darwin`, `x86_64`, `aarch64`), _per-arch checksums_ (one `sha256` can't cover a
templated asset), and _archive/binary flexibility_. This dissolves an entire apparent
contradiction and even corrects a persona-vs-persona factual dispute about _what the
template renders_. Meanwhile the sharpest _productive_ tension — the escape hatch — has a
clean reconciliation every mature analogue independently confirms: **migrate the ~5–10
mechanical single-binary tools, permanently bless the ~5–9 genuine setup-steps as a
governed adapter, and re-scope the metric to "custom-script ≤ ~5" instead of "0".**

**Tally:** 17 substantive tensions mapped — **6 genuine + reconcilable**, **3 genuine +
productive (preserve)**, **4 apparent (dissolved by ground truth or disambiguation)**,
**2 evidence-quality conflicts**, **2 quantitative/prioritization splits**. Zero are
strictly irreconcilable; two are irreducible _judgment calls_ the maintainer must make.

---

## Major Contradictions

### C1 — Retire `custom-script` vs. keep it as a permanent, governed adapter

- **Position A (retire it):** the user's explicit ask + the PRD's `≥80%-LOC-retired`
  headline metric — migrate the 19 scripts to native TOML-manifest backends.
- **Position B (keep a governed hatch forever):** **Contrarian, Historian, Archaeologist,
  Analogist, Systems Thinker** (and, more softly, Journalist + Futurist). Every mature
  analogue — chezmoi (`run_onchange_`), Terraform (provisioners "last resort"), Ansible
  (`command`/`shell`), Bazel (`genrule`), Homebrew (`def install`), mise (`http` backend)
  — _keeps_ the escape hatch permanently and _fences_ it rather than deleting it. ~5–9 of
  the 19 are genuinely imperative setup-steps (tpm git-clone, tmux-sessionizer
  cargo-from-git, gnu-utilities brew-loop, mise/rust/lua rolling managers) that a native
  port would make _worse_.
- **Evidence quality:** Position B is **High** — grounded in primary docs of six
  independent tools (Historian, Analogist) plus code-verified script characterization
  (Archaeologist read all 19; Systems read the apply pipeline). Position A is a **stated
  objective**, not an evidence claim; its strongest support is the industry thesis that
  "you don't have to maintain shell scripts anymore" (aqua) — which the Journalist notes
  even aqua pairs with a permanent registry-escape path.
- **Type:** **Interpretive** (what _should_ the goal be), riding on a **factual** base
  (F1: `Installed()` is broken; F3: backends lack capability).
- **Reconcilable?** **Yes — and this is the squad's central reconciliation.** Not
  retire-all vs. keep-all but **triage**: migrate the mechanical single-binary/vendor
  cases; permanently bless the setup-steps as a _named_ `setup-step` tier (Historian #5,
  Negative Space, Archaeologist). The verification log (C1) endorses exactly this.
- **What it reveals:** the disagreement is **squad-vs-brief, not squad-internal**. Seven
  personas independently reached the same counter-position against the user's framing.
  When the whole asymmetric squad converges _against_ the commissioning goal, the goal —
  not the squad — is the thing to re-examine.

### C2 — The `≥80%-LOC-retired` metric: genuine success target vs. wrong-shaped vanity metric

- **Position A:** the PRD authored it as the headline success metric (currently
  _deferred_ at Phase 7, ~12% removed).
- **Position B (the metric is mis-shaped):** **Contrarian** ("LOC deleted is a _vanity_
  metric; deleting 13k lines of vetted self-skipping shell to add a checksum-maintenance
  treadmill can be _negative_ engineering value"), **Historian** ("a _horizontal_ slice —
  the exact framing strangler-fig literature names as the one that 'never finishes'; use
  a vertical, per-tool, delete-on-cutover metric"), **Systems Thinker** ("it counts the
  _hard_ scripts as failures when they may be correct permanent adapters — flag the metric
  itself as suspect"), **Archaeologist** ("should be 'custom-script ≤ ~5 entries,' not
  0"), **Analogist** ("measure against release-binary/vendor-installer tools, not the
  genuine setup-steps").
- **Evidence quality:** Position B is **High-to-Medium** — Historian anchors it in
  primary strangler-fig sources + the mise/asdf precedent (a maintainer-authored account
  of the _exact_ migration); the others corroborate from code. Position A is a design
  choice with no external evidence backing the _shape_ of the metric.
- **Type:** **Interpretive** (metric design), with a **temporal** dimension (horizontal
  LOC metrics historically stall at the last-20%).
- **Reconcilable?** **Yes.** Re-scope to a _convergent, completable_ target: "N of ~10
  mechanical scripts deleted; residual ~5–9 named, justified, idempotent." The
  verification log (C1) already recommends this reframing.
- **What it reveals:** a metric that measures the wrong thing actively _pulls effort
  toward vanity work_ (Contrarian's "conflict of interest") and away from the
  high-value correctness fixes. This is the clearest case where the _incentive structure_,
  not any technical fact, is the problem.

### C3 — Lockfile: essential supply-chain fix vs. maintenance tax that collides with rolling tools

- **Position A (high value):** **Analogist** (go.sum is the exact right-sized model; dotf
  gets it nearly free because it's Go), **Journalist** (aqua/mise both converged on
  manifest→lockfile→checksum→Renovate; it's the "highest-value gap"), **Futurist** (a
  checksum lockfile is _the_ reproducibility feature that fits dotf's scope; solves
  script-url pin-drift), **Archaeologist** (fixes the "single sha256 pins one arch" trap).
- **Position B (real ongoing cost):** **Systems Thinker** ("a lockfile's _true cost_ is
  the Renovate pipeline it implies — the repo has **no** renovate.json/dependabot.yml —
  _and_ it collides with the rolling-release scripts that intentionally float; pinning
  them defeats their purpose, not pinning them makes the reproducibility promise a
  half-truth"), **Contrarian** ("a lockfile you don't keep current is worse than none;
  the aqua/mise pinning experience is a _continuous_ maintenance job — stale pins, API
  calls under `--locked`, `v`-prefix 404s").
- **Evidence quality:** Both **High**. Position A cites primary aqua/mise/go docs;
  Position B cites the code (no Renovate present, verified) + primary mise GitHub
  discussions documenting the operational pain (`mise #8677`, `#7210`).
- **Type:** **Interpretive** trade-off over a shared **factual** base (both agree the
  supply-chain gap is real; they weigh the fix's cost differently).
- **Reconcilable?** **Yes, cleanly (productive tension).** Verification log C2:
  _lockfile for pinned binary/script-url artifacts; rolling managers stay unpinned by
  design._ Ship pinning + optional `dotf lock` **before** any Renovate automation. The
  lockfile is partial _on purpose_ — it covers the pinnable subset and explicitly
  excludes the float-by-design subset.
- **What it reveals:** the lockfile debate is really the determinism debate (C7) in
  another costume — dotf has _two classes of tool_ (pinned artifacts vs. rolling managers)
  and any single reproducibility story that ignores the split is a half-truth.

### C4 — `update`/`upgrade` verb: day-2 essential vs. scope-creep that fights additive-only and duplicates topgrade

- **Position A (day-2 essential):** **Negative Space** ("day-2 verbs are the whole
  missing half of the tool"; `update` is the "topgrade-shaped hole," ranked High pain),
  **Analogist** (topgrade validates it fits dotf's additive philosophy).
- **Position B (scope-creep / EDGE):** **Journalist** ("collides with additive-only;
  topgrade already owns 'update everything'; mise's top complaints are update/shim pain —
  **EDGE, not clearly IN**"), **Systems Thinker** ("`update` is _not additive_ over the
  current design — it forces the idempotency/version-state work first; without version
  state it collapses into today's `apply`").
- **Evidence quality:** Both **Medium-High**. Journalist cites the mise/topgrade issue
  trackers directly; Systems cites the code (no version-state exists; `Installed()` never
  gated).
- **Type:** **Interpretive** (scope) + a **factual** dependency (Systems: `update`
  _requires_ the probe/version-state work as a prerequisite).
- **Reconcilable?** **Yes, by splitting the verb.** Verification log C3: `dotf
self-update` (update the _binary_) is clearly in-scope and cheap; `dotf update` (update
  installed _tools_) is EDGE — defer, or scope narrowly to the rolling managers that
  already self-update. Note this _itself_ is contested (see C8).
- **What it reveals:** "update" is overloaded — binary-update and tool-update are
  different features with different scopes. The tool-update half is gated behind the same
  missing `Installed()`/version-state work as the lockfile, so its true cost is hidden.

### C5 — Templating status (FACTUAL — dissolved and _corrected_ by ground truth)

This is the contradiction the verification log most decisively **settles**, and it has
two layers.

- **Layer 1 — does templating exist?** The brief + **Archaeologist** implied "no
  templating"; **Negative Space** corrected "it exists (`github_release.go:96-102`)."
  **Ground truth (F3):** `templateGitHubAsset` **does** replace `{{.OS}}` and `{{.Arch}}`.
  The "no templating" premise is **false**. The real gaps are: vocabulary (`info.OS` ∈
  {`linux`,`macos`} → **cannot spell `darwin`**; `ReleaseArch()` → only `amd64`/`arm64` →
  **cannot spell `x86_64`/`aarch64`**); **per-arch checksums** (one `sha256` can't cover a
  templated asset — the manifest admits this at `packages.toml:184-185`); and **archive
  flexibility** (`.tar.gz`/`.tgz`/`.zip` only — no bare binary, `.gz`, `.pkg`).
- **Layer 2 — a persona-vs-persona factual sub-conflict the ground truth also corrects.**
  **Archaeologist** claims `{{.OS}}` renders the **distro family** (`debian`/`arch`/
  `redhat`) — and self-flagged this Medium-High, "worth a maintainer confirming with one
  test." **Ground truth (F3):** `{{.OS}}` renders `string(info.OS)` ∈ {`linux`,`macos`}.
  Archaeologist **mismapped which field feeds the template** (conflated the OS enum with
  the distro-family detection the `pkg` backend uses). _Operative conclusion survives
  either way_ — whether it renders `macos` (truth) or `debian` (Archaeologist), it is not
  `darwin`/`Darwin`, so the hadolint/tea migration gap is real. But the specific claim is
  corrected.
- **Evidence quality:** Ground truth **High** (orchestrator read `templateGitHubAsset` +
  the OS/Arch constants directly). Archaeologist's distro-family claim **Medium-High,
  self-flagged as unconfirmed** — correctly deferential.
- **Type:** **Factual.** Fully resolvable, and resolved.
- **Reconcilable?** **Resolved, not merely reconcilable.** Everyone converges on the same
  actionable gap once the vocabulary is corrected: richer arch/OS tokens + per-arch
  checksum map + non-archive assets.
- **What it reveals:** even a _code fact_ can fracture across personas when the codebase
  has two OS-ish notions (OS enum vs. distro family). The ground-truth pass is what keeps
  the synthesis from inheriting a wrong premise from _either_ the brief _or_ a persona.

### C6 — Auto-asset-detection (ubi/eget) vs. pinning/reproducibility discipline

- **Position A (auto-detect is the keystone):** **Analogist** ("dotf's _single
  highest-leverage fix_ is a ubi/eget-style `github-binary` auto-detection backend — its
  gradient is inverted vs. every mature analogue; native is _harder_ than the escape
  hatch, which is why the hatch won"), **Futurist** ("auto-resolution won… GitHub-release-
  as-distribution is becoming the universal substrate").
- **Position B (pinning discipline):** dotf's whole ethos is pinned `sha256`; ubi's own
  maintainer warns the detection heuristic "is not part of the API and may change" —
  **pure auto-detect is non-deterministic across ubi versions**, which contradicts the
  reproducibility ethos (Analogist's own Contradiction #1; Futurist's "auto-resolution vs
  explicit-pin").
- **Evidence quality:** **High** on both — ubi/eget/mise primary docs; the load-bearing
  signal is **mise deprecating its `ubi` backend in favor of a native `github` backend and
  refusing new unverified entries "for supply-chain reasons"** (Futurist, Analogist both
  cite it).
- **Type:** **Interpretive/temporal** (where the ecosystem is heading) over a real
  **factual** tension (heuristics drift; pins don't).
- **Reconcilable?** **Yes — the sharpest single reconciliation in the whole corpus:
  "detect once, pin the result."** Auto-detection _feeds_ the lockfile (S1→S5): use
  auto-detect for _onboarding a tool_, use the lockfile for _reproducing it_. The two
  personas that raise the tension both resolve it the same way, independently.
- **What it reveals:** auto-detection and pinning are not opposites — they operate at
  **different lifecycle stages** (add-time vs. apply-time). mise's `ubi`→`github`
  deprecation proves the industry answer is "own the detection in-tree _and_ verify," not
  "choose one."

### C7 — Reproducibility/determinism narrative vs. rolling-tools-float + apply-re-runs-everything reality

- **Position A (the narrative):** CLAUDE.md / PRD / objective pitch "checksum-verified,
  pinned provisioning" with reproducibility ambitions; the manifest comment claims
  converged hosts "stay idempotent."
- **Position B (the reality):** **Systems Thinker** ("determinism does _not_ emerge — it
  would have to be _designed in_; `apply` is idempotent-by-script-discipline, not
  deterministic-by-engine; script-url/github-release/apt-repo/toolchain all re-download +
  re-execute every run because nothing gates on `Installed()`"), **Contrarian** ("apply is
  _convergent-by-luck_ and _non-deterministic_ — a tool that pitches 'pinned' but
  self-updates three rolling tools on every headless run is sending mixed messages"),
  **Futurist** ("pinning a _moving_ vendor installer to one sha256 is brittle; for a
  rolling installer there is no 'correct' permanent hash").
- **Ground truth (F2):** confirmed — `apply` never gates `Install()` on `Installed()`;
  every apply re-executes all scripts/downloads; rolling tools self-update each run.
- **Evidence quality:** **High** (F2 code-verified).
- **Type:** **Factual** (the engine behavior) + **interpretive** (whether the narrative
  over-claims).
- **Reconcilable?** **Yes.** (a) wire `Installed()` into the apply loop for a real
  skip-path; (b) make self-update opt-in, not a silent every-run side effect; (c) lockfile
  only for the pinnable subset; (d) **stop promising reproducibility for the rolling
  managers** — their float is _intentional and correct_ (Homebrew's documented "no
  lockfile, rolling release" stance, Historian #6). Determinism is achievable for the
  pinned artifacts; intentional non-determinism is correct for version managers.
- **What it reveals:** the "reproducibility narrative" is only false _because it's
  applied uniformly_. Split the tool population and both halves become true: deterministic
  for pinned artifacts, deliberately-floating for rolling managers.

### C8 — `dotf self-update`: in-scope/high-value vs. over-valued/low-priority

- **Position A (ship it):** **Journalist** ("a _solved 1-day feature_ via
  creativeprojects/go-selfupdate + the GoReleaser checksums.txt the shim already verifies;
  _every_ peer single-binary ships it — **IN, high value**").
- **Position B (over-valued):** **Contrarian** ("the bootstrap shim already fetches
  `releases/latest`; a self-updater adds an update channel, rollback semantics, and a
  signature story for _marginal_ benefit — low priority"), **Negative Space** (ranks it
  Low-Med — "a re-run substitutes"), **Systems Thinker** ("net simplification if done
  once, duplication if done twice — it re-introduces the checksum-verify logic already in
  `bootstrap.sh`").
- **Evidence quality:** Journalist **High** (names the exact libraries + the existing
  GoReleaser checksums asset); the skeptics **Medium** (judgment: a re-run of the shim
  already updates the binary).
- **Type:** **Interpretive** (priority/value), no factual dispute — all agree it's cheap
  and technically trivial; they disagree on whether it's _worth the surface_.
- **Reconcilable?** **Partially — a genuine judgment split.** The technical cost is
  agreed-low; the value is contested. Reasonable resolution: ship it _because_ it's cheap
  and it de-duplicates the shim's verify logic into the binary (Systems' "done once"), but
  don't rank it above the correctness fixes.
- **What it reveals:** "every peer ships it" (conformance argument) vs. "a re-run already
  does it" (minimalism argument) is a recurring dotf tension — the tool deliberately
  under-builds where a re-run substitutes.

### C9 — cosign/SBOM/SLSA provenance: 2026 baseline/table-stakes vs. aspirational-EDGE for a solo hobby tool

- **Position A (baseline now):** **Futurist** ("dotf is _one GoReleaser block away_ from
  the 2026 supply-chain baseline; cosign `signs:` + syft `sboms:` are near-trivial and the
  _highest-leverage_ bets; EU CRA reporting starts Sept 2026, SBOM mandate Dec 2027").
- **Position B (aspirational / EDGE):** **Journalist** ("checksum+lockfile is the 80%;
  SLSA is the aspirational 20% — **EDGE**, heavier, depends on upstreams publishing
  attestations, which aqua notes 'very few tools' do"), and **Futurist itself concedes**
  "CRA does **not** bind a solo dotfiles tool… don't over-rotate on compliance framing for
  a personal tool."
- **Evidence quality:** **High** on the ecosystem direction (GoReleaser/aqua/mise docs +
  CRA analyses); **the applicability to a solo tool is Medium** and self-qualified by the
  Futurist.
- **Type:** **Perspective** (enterprise-norm lens vs. solo-hobby lens) + **temporal**
  (baseline _now_ vs. _soon_).
- **Reconcilable?** **Yes.** Tier it: **checksum + lockfile = do now** (both agree); **cosign
  keyless signing of the checksum file = cheap, do soon** (GoReleaser one-liner, no
  long-lived keys); **SBOM + attestation-verify = opt-in/later** (heavier, upstream-
  dependent). Keep sha256 as the _non-negotiable floor_ so verification never _depends_ on
  Sigstore availability (Futurist's own wild-card caveat).
- **What it reveals:** the same evidence reads as "table-stakes" through an enterprise/
  regulatory lens and "nice-to-have" through a solo-maintainer lens. The disagreement is
  about _which stakeholder's norms apply_, not about the facts.

### C10 — starship "broken on the default path _today_" (EVIDENCE-QUALITY conflict)

- **Position A (broken now):** **Contrarian** live-fetched `https://starship.rs/install.sh`
  → `52c64f14…` ≠ the pinned `e9f0bb84…`; since starship is in the default `base` intent
  and `script_url.go` fails closed on mismatch, "a fresh `dotf apply` fails SHA256
  verification on starship _right now_." Historian + Futurist cite the same structural
  risk.
- **Position B (re-verify before acting):** **Verification log (F4):** "Pin confirmed
  in-repo; live upstream-drift asserted by Contrarian's fetch (**not re-run offline
  here**). **High** on the structural risk; **Medium** on the specific 'broken today'
  hash — re-verify with a live fetch before acting." Contrarian _itself_ flags the fetch
  was "over a sandboxed network… the maintainer should re-run the one-liner."
- **Evidence quality:** the _structural_ claim (rotating installer + hard pin = time-bomb)
  is **High** and undisputed; the _specific_ "broken this minute" claim is **Medium** —
  one live fetch on a sandboxed network, self-flagged, not independently re-run.
- **Type:** **Evidence** (single-source, environment-dependent measurement).
- **Reconcilable?** **Yes — trivially: re-run `curl -fsSL https://starship.rs/install.sh
| sha256sum` on the maintainer's network.** If it matches the pin, the time-bomb is
  armed-but-not-yet-detonated; if not, it's already broken. Either way the _fix_ is the
  same (pin a versioned/immutable asset, or accept the rotating installer as a tolerant
  shell-out).
- **What it reveals:** a genuinely load-bearing finding (the migration's own
  proof-of-concept may be broken) rests on a _single environment-sensitive measurement_ —
  the exact case where the verification log's "re-verify before acting" discipline earns
  its keep. Don't act on the specific hash; do act on the structural risk.

### C11 — How many of the 19 are "mechanical migrate" vs. "irreducible keep" (QUANTITATIVE split)

- **The spread:** **Contrarian** keep ~6 (tpm, tmux-sessionizer, gnu-utilities, mise,
  rust, lua), migrate ~5–6; **Archaeologist** residual ~5 genuinely-imperative; **Systems
  Thinker** ~9 setup-steps keep, ~10 mechanical migrate; **Historian** ~9 genuine
  setup-steps; **Futurist** keep ~5, migrate ~12–14; **Analogist** keep mise/rust/
  gnu-utilities (+tpm/sessionizer), migrate ~6–8.
- **Evidence quality:** **High** per-persona (all read the scripts) but the _definition_
  of "mechanical" floats — Archaeologist's per-script capability table is the most
  granular source of truth (it shows _why_ each is or isn't mechanical: hadolint needs
  `x86_64` + bare-binary; tea needs a non-GitHub host + checksums.txt lookup; atuin needs
  env-passthrough + config-write + `import`).
- **Type:** **Interpretive** (where to draw the "mechanical" line), grounded in **factual**
  per-script capability needs.
- **Reconcilable?** **Yes, but only _after_ the backend capabilities land** — the count is
  a _function of_ which capabilities exist. With bare-binary + richer arch vocab +
  checksums.txt-lookup + non-GitHub host, ~7 cloud/container binaries become mechanical;
  without them, they're not. The count is downstream of C-family capability work.
- **What it reveals:** "how many can we retire?" has **no fixed answer** — it's set by how
  many backend capabilities you build. This reframes the metric debate (C2): the target
  should be defined _after_ deciding which capabilities to ship, not before.

### C12 — Where the single highest-leverage intervention sits (PRIORITIZATION conflict)

Each persona nominates a _different keystone_ — not contradictory in truth (all are
valuable) but a genuine sequencing disagreement:

| Persona             | Nominated keystone                                                                                                                                              | Meadows/category        |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------- |
| **Systems Thinker** | manifest `probe`/`check` field wired into the apply-loop `Installed()` gate ("one change, four unlocked features") + reframe custom-script as permanent adapter | Information flow + Goal |
| **Analogist**       | ubi/eget-style `github-binary` auto-detect backend ("keystone; unlocks the most migrations at once")                                                            | Rules                   |
| **Negative Space**  | `Package.Binary`/`Provides` schema field ("the tightest blocker — gates _verification_") + `--only <pkg>` targeting                                             | Schema/verification     |
| **Contrarian**      | fix `Installed()` state detection (a _bug fix_, not a feature) — "before any grand migration"                                                                   | Correctness             |
| **Journalist**      | lockfile + checksum pinning ("highest-value gap"); `doctor` ("table stakes")                                                                                    | Supply-chain / UX       |
| **Historian**       | build missing backend capabilities _first_ + **freeze policy** on new escape-hatch entries                                                                      | Capability + Policy     |
| **Archaeologist**   | sentinel/`creates:` predicate + spread the `pkg` authority-query + `git-clone` backend                                                                          | Idempotency primitive   |
| **Futurist**        | cosign + SBOM in `.goreleaser.yml`                                                                                                                              | Supply-chain provenance |

- **Convergence beneath the divergence:** three of these are _the same fix under different
  names_ — Systems' `probe` field, Negative Space's `Binary`/`Provides`, Archaeologist's
  `creates:`/authority-query, and Contrarian's "fix `Installed()`" all point at **a richer,
  explicit install-detection field wired into the apply loop.** That is the true keystone
  the plurality (4 of 8) independently converge on; the ground truth (F1, F2) confirms both
  its brokenness and its non-use on the write path.
- **Type:** **Interpretive** (sequencing/priority).
- **Reconcilable?** **Yes.** The dependency graph resolves the order: **`Installed()`/probe
  fix → (unlocks) honest status, idempotent apply, `--only`, `doctor`, `update`, lockfile.**
  Systems' "ship the probe _before_ doctor/update, never after" is the decisive sequencing
  rule — a diagnostic built on today's false probe "would launch broken and burn trust."
- **What it reveals:** the personas aren't disagreeing about _what's valuable_ so much as
  _what's foundational_. The correctness/verification fix is upstream of nearly every
  feature the others nominate — it's the keystone the plurality points at once you
  de-duplicate the naming.

---

## Contradiction Patterns

Recurring shapes across the tensions:

1. **Squad-vs-brief, not squad-vs-squad.** The two loudest "contradictions" (C1 retire,
   C2 metric) are _the unified squad pushing back on the commissioning artifact_. Seven or
   eight personas agree with each other and disagree with the user's framing. This is the
   dominant pattern and the most important signal.

2. **The "split the population" resolution.** C3 (lockfile), C4 (update), C7
   (determinism) all dissolve the same way: **dotf has two classes of tool — pinned
   artifacts and rolling managers — and any single story that treats them uniformly is a
   half-truth.** Pin/lock/determinism for the first class; intentional float for the
   second. One structural insight resolves three tensions.

3. **The "different lifecycle stage" resolution.** C6 (auto-detect vs. pin) resolves via
   "detect once at add-time, pin at apply-time." The apparent opposition is really two
   phases of one workflow.

4. **Conformance vs. minimalism.** C8 (self-update), C9 (cosign/SBOM), and the `update`
   half of C4 all pit "every peer ships it / it's the 2026 baseline" against "a re-run
   substitutes / wrong complexity budget for a solo tool." dotf deliberately under-builds
   where a re-run or the shim already covers the need.

5. **Ground truth as premise-corrector.** C5 shows the verification log correcting _both_
   the brief (no-templating premise) _and_ a persona (Archaeologist's distro-family claim).
   The most dangerous contradictions are the ones inherited as false premises.

6. **Governance mechanisms carry their own second-order failure.** The "fence the escape
   hatch" prescription (lint/count/freeze) is near-unanimous, yet Systems flags the CI
   count "can incentivize gaming — folding three tools into one script"; the reframe-as-
   permanent move "risks becoming an excuse to never migrate." Every proposed governance
   lever needs a guard against its own gaming.

---

## Contradiction Severity Matrix

| ID  | Contradiction                         | Type                   | Genuine?                 | Reconcilable?          | Blocks migration?           | Severity              |
| --- | ------------------------------------- | ---------------------- | ------------------------ | ---------------------- | --------------------------- | --------------------- |
| C1  | Retire vs. permanent governed adapter | Interpretive           | Genuine (squad-vs-brief) | Yes (triage)           | Directly (goal-shape)       | **Critical**          |
| C2  | ≥80%-LOC metric shape                 | Interpretive/temporal  | Genuine (squad-vs-brief) | Yes (re-scope)         | Misdirects effort           | **Critical**          |
| C12 | Which keystone / sequencing           | Interpretive           | Genuine (priority)       | Yes (dep-graph)        | Sequencing risk             | **High**              |
| C5  | Templating status                     | Factual                | Apparent (resolved)      | Resolved by F3         | Unblocks (corrects premise) | **High (as premise)** |
| C7  | Determinism narrative vs. reality     | Factual+interpretive   | Genuine                  | Yes (split population) | Erodes trust                | **High**              |
| C3  | Lockfile value vs. tax                | Interpretive           | Genuine (productive)     | Yes (partial lockfile) | No                          | **High**              |
| C11 | Count of mechanical vs. keep          | Interpretive/quant     | Genuine (definitional)   | Yes (post-capability)  | Scopes the work             | **Medium-High**       |
| C10 | starship broken _today_               | Evidence               | Genuine (single-source)  | Yes (re-fetch)         | Default path at risk        | **Medium-High**       |
| C4  | `update` verb scope                   | Interpretive           | Genuine (productive)     | Yes (split verb)       | No                          | **Medium**            |
| C6  | Auto-detect vs. pin                   | Interpretive/temporal  | Genuine (productive)     | Yes (detect-once-pin)  | No (enables it)             | **Medium**            |
| C9  | cosign/SBOM baseline vs. EDGE         | Perspective/temporal   | Genuine                  | Yes (tier it)          | No                          | **Medium**            |
| C8  | self-update priority                  | Interpretive           | Genuine (judgment)       | Partial                | No                          | **Low-Medium**        |
| C13 | Templating-as-trap vs. extend         | Factual (equivocation) | Apparent (disambig.)     | Dissolves              | No                          | **Low**               |
| C14 | Lockfile vs. additive-only            | Interpretive           | Apparent (boundary)      | Yes (read-only lock)   | No                          | **Low**               |
| C15 | doctor now vs. after-probe            | Sequencing             | Apparent (ordering)      | Yes (probe first)      | No                          | **Low**               |
| C16 | Renovate answer vs. Forgejo reality   | Factual (unverified)   | Genuine (open)           | Unknown (needs spike)  | No                          | **Low-Medium**        |
| C17 | symlink unlink vs. no-reconciliation  | Interpretive           | Apparent (conflation)    | Yes (unbundle)         | No                          | **Low**               |

---

## Irreconcilable Contradictions

**None are strictly irreconcilable.** Every tension has a resolution direction. But two
reduce to **irreducible maintainer judgment calls** that evidence can inform but not
decide:

1. **Does `dotf self-update` earn its surface? (C8)** All agree it's cheap; they disagree
   on whether "every peer ships it" outweighs "a shim re-run already does it." No amount of
   further research settles a taste question about surface minimalism. _Recommendation:
   ship it because it de-duplicates the shim's verify logic into the binary — but rank it
   below correctness fixes._

2. **Whose norms apply for provenance — enterprise-2026-baseline or solo-hobby-tool? (C9)**
   The evidence (CRA, GoReleaser ease, mise/aqua adoption) is genuine; whether it _binds a
   personal tool_ is a scope judgment the Futurist itself flags as indirect.
   _Recommendation: cosign-sign the checksum file (cheap, keyless); defer SBOM/attestation
   until an upstream or consumer actually needs it._

Everything else is reconcilable via the "split the population," "different lifecycle
stage," "re-scope the metric," or "fix the prerequisite first" moves.

---

## Productive Tensions (preserve — both sides true in different contexts)

These should **not** be smoothed into a single position; the value is in holding both.

### PT1 — The escape hatch: shrink AND contain (the flagship productive tension)

The verification log names this explicitly: _"This is a productive tension — preserve
it."_ **Both are true:** the migratable single-binary tools _should_ be retired (Position
A is right for them), and the genuine setup-steps _should_ stay forever as a governed
adapter (Position B is right for them). The failure mode is picking one side globally —
"retire all" is the Nix mistake in miniature (Historian, Contrarian); "keep all" abandons
the supply-chain and correctness wins. The resolution _is_ the tension held per-tool:
**minimize the migratable subset to ~zero while blessing the irreducible remainder as a
named `setup-step` tier.** Every mature analogue (chezmoi, Terraform, Ansible, Bazel,
Homebrew, mise) independently landed here. This is the single most important thing to
carry into the Crucible un-collapsed.

### PT2 — Auto-detection ↔ pinning (detect once, pin the result)

Auto-detection (convenience, low activation energy, onboarding) and pinning
(reproducibility, integrity, apply-time) are **not opposites** — they are the two ends of
one pipeline (S1 feeds S5). Preserve both: use ubi/eget-style detection to _add_ a tool
cheaply, then freeze the resolved asset+checksum in the lockfile so re-applies are
deterministic. Collapsing to pure auto-detect loses reproducibility (ubi's own "not part
of the API" caveat); collapsing to pure hand-pinning keeps the inverted gradient that made
`custom-script` win.

### PT3 — Rolling float ↔ deterministic pin (two tool classes, two contracts)

Rolling managers (mise/rust/atuin) _want_ to float and self-update; pinned artifacts
(github-release binaries, script-url installers) _want_ to be frozen and verified. The
lockfile is **partial on purpose**. Homebrew's documented "no lockfile, rolling release"
decision (Historian #6) is the precedent: state the two contracts explicitly rather than
forcing a uniform reproducibility promise that's a half-truth either way.

---

## Context-Dependent Truths

Claims that are true or false depending on _which subset / stakeholder / lifecycle stage_:

- **"Retiring the escape hatch is worth it."** TRUE for mechanical single-binary tools;
  FALSE for genuine setup-steps (tpm/gnu-utilities/mise/rust).
- **"Pinning = hardened/secure."** TRUE for immutable versioned assets; FALSE for rotating
  vendor installers (proven by starship, C10).
- **"`apply` is idempotent."** TRUE for the `pkg` backend (manager no-ops); FALSE-ish for
  rolling tools (silent self-update drift) and wasteful for all (re-download every run,
  F2).
- **"A lockfile fits dotf's scope."** TRUE for the pinnable subset; in tension for rolling
  managers (pinning defeats their purpose).
- **"cosign/SBOM is table-stakes."** TRUE as a 2026 _ecosystem norm_; FALSE as a _binding
  requirement_ for a solo hobby tool (CRA doesn't apply).
- **"`update` is essential."** TRUE as a day-2 verb users reach for; FALSE as an _additive_
  feature — it forces version-state work first, so it's not free.
- **"The maintainer benefits from retiring the scripts."** TRUE for maintainer simplicity;
  FALSE for the _stranger_ cloning the repo, who needs the scripts' platform-detection
  edge-case handling (Systems' stakeholder conflict).
- **"`{{.OS}}` is useless for release assets."** TRUE in that it can't spell `darwin`
  (F3); but the _reason_ is vocabulary (`macos`), not the distro-family value Archaeologist
  posited — same conclusion, corrected mechanism.

---

## Evidence Quality Conflicts

Where personas rest on evidence of differing strength, and how to weight them:

1. **starship "broken today" (C10) — single environment-sensitive measurement.**
   Contrarian's live fetch (sandboxed network, self-flagged) vs. verification log's
   "Medium, re-verify." **Weight:** treat the _structural risk_ as High/actionable; treat
   the _specific hash_ as unconfirmed until re-fetched on the maintainer's network. Do not
   ship a fix premised on the exact hash; do ship the fix premised on the pattern.

2. **`{{.OS}}` render value (C5, Layer 2) — persona inference vs. code read.**
   Archaeologist's "distro-family" claim (Medium-High, self-flagged "worth a maintainer
   confirming") vs. verification log's direct read (`string(info.OS)` ∈ {linux,macos},
   High). **Weight:** ground truth wins; Archaeologist's operative conclusion (can't spell
   `darwin`) survives regardless.

3. **Nix-fatigue sentiment — four personas, one shared soft caveat.** Contrarian ("no
   citable links; directional community sentiment"), Historian ("Medium; experiential
   blogs"), Journalist ("weakest evidence; enthusiast blogs"), Futurist ("mid-2025
   community sentiment; directional, not quantified"). **This is not a conflict — it's a
   consensus with a shared honesty caveat.** All four use soft evidence to support the
   _same_ conclusion (dotf's imperative middle-ground is validated). Weight the _direction_
   High, the _magnitude_ Low. No persona over-claims.

4. **Tech-debt interest-rate figures (Analogist, "3–5× architectural").** Secondary
   summaries of empirical studies (SIG/McKinsey via blog), self-rated Medium. **Weight:**
   framing only, not load-bearing for any code decision.

5. **Which-scripts-are-mechanical (C11) — high per-persona confidence, floating
   definition.** Every persona read the scripts (High), but "mechanical" is defined
   differently. **Weight:** Archaeologist's per-script capability table is the most
   granular and should be the reference; the _count_ is downstream of capability decisions.

---

## Recommended Resolution Priorities

Ordered by (blocks-everything-else × evidence-strength × low-regret):

1. **Fix `Installed()` with an explicit detection field, wired into the apply loop
   (C12, C5-adjacent, F1/F2).** The keystone 4-of-8 personas converge on under different
   names (`probe`/`Binary`/`Provides`/`creates:`). Unblocks honest `status`/`diff`,
   idempotent apply, `--only`, `doctor`, `update`, and lockfile. **Ship before any of
   those.** Add the regression test Negative Space flags as missing.

2. **Re-scope the success metric (C2) and reframe custom-script as a permanent
   `setup-step` tier (C1/PT1).** A PRD paragraph, not code. Converts the 19-count from
   "failure" to "success + a small migratable backlog." Do this _with_ #1.

3. **Build the migration-enabling backend capabilities (C5, C11):** richer arch/OS
   vocabulary (`x86_64`/`aarch64`/`darwin`), per-arch checksum map, non-archive assets
   (bare binary/`.gz`), `git-clone` backend, `rpm-repo` backend. The count of retirable
   scripts (C11) is a _function of_ which of these ship.

4. **Add `--only <pkg>`/`--skip` targeting (C12/Negative Space).** Cheap; unblocks the
   vertical-slice, delete-on-cutover migration loop the whole plan depends on.

5. **Ship a partial lockfile for the pinnable subset (C3/C6/PT2/PT3); rolling managers
   stay unpinned by design.** Pair with "detect once, pin the result." Defer Renovate
   until the lockfile exists — and resolve the Forgejo/Renovate question (C16) first.

6. **Re-verify the starship pin on the maintainer's network (C10); then either pin a
   versioned asset or accept it as a tolerant shell-out.** Trivial, closes a default-path
   risk.

7. **`doctor` (after #1), `--json`/`NO_COLOR`/`--verbose` hygiene, cosign-sign the
   checksum file (C9 tier-1).** High value-to-effort; `doctor` _depends on_ #1 (C15) — do
   not ship it first.

8. **Defer as judgment calls:** `dotf update` for tools (C4), `dotf self-update` (C8),
   SBOM/attestation (C9 tier-3). Cheap-but-contested; not foundational.

---

## Unresolved Questions

1. **Is the starship pin actually stale on the maintainer's network right now? (C10)** One
   `curl … | sha256sum` re-run settles it. Until then, "broken today" is Medium-confidence.

2. **Does Renovate support Forgejo (dotf's origin) as smoothly as GitHub for
   regex-manager checksum auto-bump? (C16)** Four personas prescribe Renovate as the answer
   to the lockfile maintenance tax, but the repo is Forgejo-origin and has no Renovate config
   today. The prescribed cure may not fit the forge — needs a spike before committing to the
   lockfile-implies-Renovate story.

3. **Build auto-detection natively or vendor a library? (C6)** mise's `ubi`→`github`
   deprecation says _own it in-tree_, but whether dotf reimplements the OS→arch→libc→
   tie-break pipeline or vendors one is a code-spike estimate no persona made.

4. **Can one detection field express multi-binary bundles? (Systems, Negative Space)**
   `hashicorp-tools` installs terraform+vault+packer under one `name`; does `probe =
["terraform","vault"]` (all-present = installed) work cleanly, or does the bundle need
   splitting into per-tool entries? Schema question worth prototyping.

5. **Is `apply`'s re-download-every-run actually biting, or latent? (Systems)** Opt-in
   intents are rarely re-run, so the cost may be theoretical. The fix is cheap and unlocks
   other features regardless, but the _severity_ is uncertain.

6. **Exact per-tool binary names for `coderabbit`/`turso`/`tea`/`opencode`/`codex`/`fabric`.**
   Both Contrarian and Negative Space flag these as un-byte-verified; `fabric` carries a
   real pyinvoke name-collision risk. Confirm per-tool before porting.

---

## Key Insights

1. **The two loudest contradictions are the squad disagreeing with the brief, not with
   each other.** When seven of eight independent personas converge _against_ "retire
   custom-script" and _against_ the `≥80%-LOC` metric, the correct read is that the goal is
   mis-shaped — reframe it (triage + re-scope), don't force it.

2. **One structural insight — "dotf has two tool classes" — resolves three separate
   tensions** (lockfile, update, determinism). Pinned artifacts get integrity + determinism;
   rolling managers get intentional float. Any uniform reproducibility story is a half-truth.

3. **The keystone the plurality points at is one fix wearing four names.** Systems' `probe`,
   Negative Space's `Binary`/`Provides`, Archaeologist's `creates:`, and Contrarian's "fix
   `Installed()`" are the same explicit-detection-field-wired-into-apply. It is upstream of
   nearly every feature the other personas nominate.

4. **The ground truth's biggest contribution is premise-correction, not tie-breaking.**
   F3 kills the "no templating" premise the brief and a persona both carried, and even
   corrects _what_ the template renders (linux/macos, not distro-family). The most dangerous
   contradictions are inherited false premises.

5. **The escape-hatch tension is productive and must survive the synthesis intact.**
   "Retire" and "keep" are both correct — for different subsets. Collapsing to either pole
   is a documented failure mode (Nix over-declaration vs. supply-chain neglect).

6. **"Detect once, pin the result" is the sharpest reconciliation in the corpus** — two
   personas independently resolve the auto-detect-vs-pin tension the same way, and mise's
   `ubi`→`github` deprecation confirms the industry landed there too.

7. **Every governance lever carries its own gaming risk.** The near-unanimous "fence the
   hatch" prescription (lint/count/freeze/reframe) needs guards against folding-tools-to-
   game-the-count and reframe-as-excuse-to-never-migrate. The mechanism is not self-policing.

8. **Two contradictions reduce to irreducible taste, not evidence** — self-update surface
   (C8) and whose-provenance-norms-apply (C9). Name them as judgment calls; don't pretend
   more research resolves them.

---

## Summary Statistics

- **Persona findings analyzed:** 8 (Historian, Contrarian, Analogist, Systems Thinker,
  Journalist, Archaeologist, Futurist, Negative Space) + 1 code-verified verification log.
- **Substantive tensions mapped:** 17.
  - **Genuine + reconcilable:** 6 (C2, C7, C10, C11, C12, C16).
  - **Genuine + productive (preserve):** 3 (C1/PT1, C3/PT3, C4 & C6/PT2).
  - **Apparent / dissolved by ground truth or disambiguation:** 4 (C5, C13, C14, C17).
  - **Genuine judgment calls (irreducible):** 2 (C8, C9).
  - **Evidence-quality conflicts:** 2 (C10 starship hash; C5-Layer-2 `{{.OS}}` value).
- **Strictly irreconcilable:** 0.
- **Contradiction types:** Factual 4 (C5, C7, C10, C13/C16) · Interpretive 8 · Temporal 3
  (C2, C6, C9) · Perspective 2 (C9, C1-stakeholder) · Evidence 2.
- **Direction of the biggest split:** squad-vs-brief on C1 + C2 (7–8 personas lean
  "reframe/re-scope," the brief leans "retire/hit-80%"). Evidence leans **strongly toward
  reframe**.
- **Ground truth's decisive interventions:** 3 — F1 (confirms the `Installed()` bug,
  ending any doubt), F3 (kills the no-templating premise + corrects the `{{.OS}}` value),
  F4 (downgrades starship-broken-today to Medium pending re-verify).
- **Consensus floor (no persona dissents):** the `Installed()` bug is real; migration is
  capability/verification-blocked not effort-blocked; additive-only is a feature; zero is
  the wrong custom-script target; docs have drifted.

---

### Digest — sharpest tensions and which way the evidence leans

- **Retire `custom-script` vs. permanent governed adapter (C1):** evidence leans
  **decisively toward "triage, don't retire wholesale"** — 7/8 personas + all six mature
  analogues keep a fenced hatch forever. Migrate the ~5–10 mechanical tools; bless the ~5–9
  setup-steps. Preserve this as a per-tool productive tension, not a global verdict.

- **The `≥80%-LOC` metric (C2):** leans **strongly toward "wrong shape, re-scope"** —
  near-unanimous that a horizontal LOC target is the strangler-fig anti-pattern that never
  finishes. Replace with "custom-script ≤ ~5, residual named + justified + idempotent."

- **Lockfile: fix vs. tax (C3):** **both true, split the population** — lockfile for pinned
  artifacts, rolling managers float by design. Ship pinning + optional `dotf lock` _before_
  any Renovate automation; resolve the Forgejo/Renovate question first.

- **Templating status (C5):** **resolved by ground truth** — templating _exists_; the gaps
  are vocabulary (`darwin`/`x86_64`/`aarch64`), per-arch checksums, and archive flexibility.
  Ground truth also corrects Archaeologist's "distro-family" claim to `linux`/`macos`. The
  "no templating" premise is dead.

- **Auto-detect vs. pin (C6):** leans **"detect once, pin the result"** — two personas
  reconcile it identically and mise's `ubi`→`github` deprecation confirms the industry
  answer is "own detection in-tree _and_ verify," not choose one.

- **Determinism narrative vs. reality (C7):** evidence (F2) leans **"the narrative
  over-claims uniformly"** — apply re-runs everything and rolling tools self-update silently.
  Fix: wire `Installed()`, make self-update opt-in, and stop promising reproducibility for
  the intentionally-floating managers.

- **`update` verb + `self-update` (C4, C8):** leans **"split the verb; both are cheap but
  contested"** — `self-update` (binary) is in-scope and de-duplicates shim logic; `update`
  (tools) is EDGE and forces version-state work first. Rank both below the correctness fixes.

- **starship "broken today" (C10):** the **structural risk leans High/act-on-it**; the
  **specific "broken this minute" hash leans Medium/re-verify** — one `curl … | sha256sum`
  on the maintainer's network settles it. Don't ship a fix premised on the exact hash; do
  ship one premised on "rotating installer + hard pin = time-bomb."
