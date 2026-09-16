# Strategic Research Report: `dotf` CLI — Features, Enhancements & Fixes

**Research Date**: 2026-07-04
**Output Directory**: `docs/research/dotf-cli-enhancements/`
**Method**: Asymmetric Research Squad — 8 personas → Crucible (ACH + contradiction map) → Emergent insight (tension/pattern/negative-space/innovation) → synthesis
**Grounding**: every code claim verified against the repo at `file:line`; external claims cited to chezmoi/aqua/mise/ubi/Terraform/Homebrew docs

---

## Executive Synthesis

You asked what features, enhancements, and fixes to add to `dotf` — including finishing
the migration of the 19 `bin/install-*.sh` shims into native TOML backends. The squad's
strongest, most counter‑intuitive conclusion is this: **the migration is not the first
thing to do, and finishing it "all the way" is the wrong goal.** The evidence — 8
independent perspectives plus direct code verification — converges on four load‑bearing
facts:

1. **`dotf`'s read path is broken and disconnected from its write path.** `Installed()`
   is a bare `LookPath(pkg.Name)` in 4 of 7 backends (`custom_script.go:49-55`,
   `github_release.go:80-87`), and `pkg.Name` is a _group label_, not a binary — so
   `dotf status`/`dotf diff` **lie about ≥8 of the 19 opt‑in tools** (`aws-tools`,
   `azure-cli`, `hashicorp-tools`, `container-tools`, `rust`, `tpm`, `tmux-sessionizer`,
   `gnu-utilities`). Worse, `apply` **never consults `Installed()` at all**
   (`install.go:96-104`): every run re‑executes every script and re‑downloads every
   asset. You cannot safely retire a script whose replacement you cannot verify, and you
   cannot claim reproducibility from a tool that re‑runs everything blindly. **This one
   fix is the keystone** — it is simultaneously the last correctness bug, the first schema
   capability, and the precondition for `status`‑honesty, `doctor`, `update`, and any
   lockfile.

2. **The backends cannot yet hold the scripts they must replace.** `github-release` — the
   backend most migrations would target — can only install a single file whose basename
   equals `pkg.Name`, from a `.tar.gz`/`.zip` only, and its `{{.OS}}` template renders the
   **distro family** (`arch`/`debian`/`redhat`/`macos`), so it can **never spell `darwin`**
   and spells `linux` only as an unrecognized‑distro fallback; `{{.Arch}}` renders only
   `amd64`/`arm64`, never `x86_64`/`aarch64`. A single `sha256` field can't cover a
   templated multi‑arch asset. **Migrating with today's backends is provably impossible
   for most tools** — the one existing native port (`starship`) proves it: it's in the
   default `base` intent and its pinned hash is a rotating‑installer time‑bomb that fails
   `dotf apply`'s checksum on hash rotation.

3. **Grounded in the code, only ~4–5 of the 19 are mechanically portable today, ~5 more
   after new capabilities, and ~9 are genuine setup‑steps that should stay shell forever.**
   Every mature comparable tool — chezmoi, Terraform, Ansible, Bazel, Homebrew, mise —
   keeps a _governed_ escape hatch permanently. Eliminating `custom-script` entirely would
   be "the Nix mistake in miniature." The PRD's "**≥80% of shell LOC retired**" metric is
   the wrong _shape_: it's the strangler‑fig anti‑pattern that never finishes, and it
   Goodhart‑mis‑directs effort onto exactly the setup‑step scripts that should be kept.

4. **`custom-script` won 19‑of‑37 because dotf's incentive gradient is inverted.** In
   Homebrew/aqua/mise the declarative path is _cheap_ and the escape hatch is _costly_; in
   `dotf` a `custom-script` entry is a one‑liner while a native backend entry is laborious
   and under‑powered. The hatch grew for mechanical reasons, not laziness — so the fix is
   to **invert the gradient** (make native cheaper via auto‑detection) and **fence the
   hatch** (a CI ratchet blocking new entries), not to hand‑port scripts one by one.

**Key Findings**

1. **Highest‑leverage single change**: add a `Package.Binary`/`provides` + `installed_when`
   probe field and wire a real `Installed()` gate into `apply` — reusing the
   authority‑query technique that _already exists_ in `pkg.go:109-142`. Named independently
   by 6 of 8 personas.
2. **The answer is a sequence, not a winner**: correctness → capabilities → vertical
   per‑tool migration → day‑2/governance. "Migrate now with today's backends" and "do
   nothing" were both eliminated by the ACH.
3. **Re‑scope the goal**: from "≥80% LOC retired" to "`custom-script` ≤ ~5 entries, each
   named + justified + idempotent," measured per‑tool as _delete‑the‑script‑on‑cutover_.

**Most Surprising Discovery**: The migration is _less_ blocked than "capability‑first"
implies — 4 tools (`lefthook`, `codex`, `claude`, `opencode`) are npm‑installable **today**
via the `toolchain` backend, which has **0 production uses** and is completely untested.
The easy path exists; it's just never been walked (and carries an undeclared `node` dep).

**Highest‑Impact Insight**: Two new schema fields — `probe` (`provides`/`installed_when`)
and `class` (`pinned`/`rolling`/`managed`) — together resolve the _majority_ of the
corpus: honest status, idempotent apply, the lockfile‑vs‑rolling‑tools conflict, `update`
semantics, and the determinism over‑claim, all at once.

---

## Multi‑Perspective Analysis

### Theme 1 — Correctness first: the read/write path is the foundation everything else needs

**Overview.** Before any feature or migration, four confirmed defects must be fixed
because they make the tool _lie_ or _do redundant work_, and because every desirable
feature (migration verification, `doctor`, `update`, lockfile) is built on top of a
truthful "is this installed?" answer.

**Current State (Journalist, Systems Thinker, Negative Space; code‑verified).**

- **The `Installed()` bug** (`custom_script.go:49-55`, `github_release.go:80-87`): bare
  `LookPath(pkg.Name)`. Mis‑reports ≥8 tools; `fabric` can false‑_positive_ against
  Python's `fabric`. The correct pattern (query the install authority) already lives in
  `pkg.go:109-142` — it just isn't diffused to the other backends.
- **The apply/status disconnect** (`install.go:96-104`, `applyphases.go:135-143`): `apply`
  calls `Install()` unconditionally; nothing gates on installed‑state. `apply` is
  idempotent only because the _scripts_ self‑skip — an engine that trusts a contract it
  doesn't enforce.
- **The starship pin** (`packages.toml:222-225`, in the **default** `base` intent): sha256
  of a rotating vendor installer; fails closed on rotation. The one native `script-url`
  port is the migration's own broken proof‑of‑concept.
- **`apt-repo` hard‑codes `noble`** (`packages.toml:218`): 404s on any non‑24.04 Ubuntu and
  on Debian; no `rpm-repo` backend → Fedora/openSUSE second‑class.

**Critical Perspective (Contrarian).** These aren't roadmap "nice‑to‑haves"; they are the
reason the read‑only commands can't be trusted and the reason a migration can't be
verified. Fixing them is a prerequisite, not a feature.

**Evidence Quality**: **High** — all four are code‑verified (F1, F2, F4, F5 in the
verification log). Only the _live_ starship hash is Medium (re‑fetch to confirm
"broken‑today" vs "armed").

### Theme 2 — Capability before migration: the backends must grow before the scripts can die

**Overview.** The scripts encode real platform knowledge (arch mappings, GPG fingerprints,
git‑clone setup, post‑install steps, non‑GitHub hosts) that the current backends cannot
express. A naive port _loses_ that knowledge and introduces new bugs.

**Historical Context (Archaeologist, Historian).** mise's successful migration off asdf's
shell plugins is the precedent: the win came _only_ after native backends
(aqua/ubi/github) existed to hold the tools — build capabilities first, then migrate.
Terraform's `provisioner` has been "a last resort" for ~9 years and never shrank; mise
still has <25% of tools on its insecure path after a funded multi‑year campaign — and
that's the _success_ story. Left alone, a "shrinking escape hatch" trends **up**.

**Cross‑Domain Insight (Analogist).** The single highest‑leverage borrow is
**ubi/eget‑style auto‑asset‑detection**: resolve the GitHub‑release asset by OS/arch/libc
automatically instead of a hand‑authored template. This _inverts the incentive gradient_ —
it makes the declarative path cheaper than the escape hatch, which is what actually stops
the hatch from growing. Pair with **"detect once, pin the result"** (the go.mod/go.sum,
Cargo, aqua‑checksums pattern): auto‑detect at add‑time, freeze the resolved
tag+asset+sha256 in a `dotf.lock`, verify at apply‑time.

**The concrete capability gaps** (Archaeologist, Negative Space; code‑verified):
`ReleaseOS()` mapper (distro‑family → `linux`/`darwin`); `x86_64`/`aarch64` arch variants;
per‑OS/arch checksum map; bare‑binary and `.gz` extraction; per‑OS binary‑name override;
`checksums.txt`/sidecar lookup; arbitrary (non‑GitHub) release host; a `git-clone` backend;
a _typed, constrained_ `post_install` step list; and `rpm-repo`.

**Evidence Quality**: **High** on the gaps (code‑verified); **Medium** on the auto‑detect
edge‑case tail (needs a code spike against 5 real release pages).

### Theme 3 — Triage, don't retire: the escape hatch is a permanent, governed backend

**Overview.** The user's framing ("remove all install‑*.sh, make them native") collides
with a near‑unanimous squad finding: ~9 of the 19 are genuinely imperative and should stay
shell **forever**, governed rather than eliminated.

**The per‑tool verdict** (Negative Space Phase‑3 — _the artifact the corpus was missing_,
grounded in the scripts + backend code):

| Bucket                                                                                                   | Count | Tools                                                                                                                                                                                                                                                                          |
| -------------------------------------------------------------------------------------------------------- | ----- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Mechanical TODAY** (0 new capability, via the untested `toolchain`/npm path)                           | ~4–5  | lefthook, codex, claude, opencode (+ pkg‑slice of lua) — each with a caveat (undeclared `node` dep / lost brew‑first cascade)                                                                                                                                                  |
| **Mechanical AFTER new capabilities** (bare‑binary, arch/OS vocab, checksums.txt, non‑GH host, rpm‑repo) | ~5    | turso, tea, container‑tools, tflint‑slice of hashicorp‑tools, ecs‑cli‑slice of aws‑tools                                                                                                                                                                                       |
| **Genuine setup‑steps / keep‑as‑script**                                                                 | ~9    | mise, rust, tpm (git‑clone), tmux‑sessionizer (cargo‑git), gnu‑utilities (brew multi‑pkg + os‑gating), aws‑tools (bundle/`.pkg`), azure‑cli (rpm‑repo + per‑OS), fabric (`go install` + name collision), coderabbit (rotating installer + `claude` dep), + setup‑half of atuin |

**Critical Perspective (Contrarian, Historian, Systems Thinker).** chezmoi reached the
identical conclusion by design ("chezmoi has no concept of packages… only scripts") and
closed reconciliation/purge requests won't‑fix. The residual imperative set is
Promise‑Theory‑correct, not failure. Reframe `custom-script` from "shrinking escape hatch"
(a paragraph in the PRD) to "**permanent adapter layer for imperative setup‑steps**," and
the real migration backlog drops from 19 to ~10 mechanical tools.

**Governance (Analogist, Systems Thinker).** Fence the hatch with three gradient‑benders:
(1) cheaper native (auto‑detect); (2) a **CI lint that fails on new `custom-script`
entries** (a monotonic ratchet) + a count line in `dotf status`; (3) remove the
maintenance tax that pushes toward unpinned scripts (Renovate‑style tag+sha256 auto‑bump).

**Evidence Quality**: **High** — 7‑of‑8 personas + six independent tool precedents converge.

### Theme 4 — Day‑2 UX: the whole "operate an installed machine" verb set is missing

**Overview.** `dotf` has install‑time verbs (plan/install/apply/link/sysconfig) but almost
no day‑2 verbs. The Journalist's 2025‑2026 capability matrix shows peers ship these as
table stakes.

**What competitors ship that dotf lacks (in‑scope subset):**

- **`dotf doctor`** — chezmoi and yadm both ship it as the universal "new machine
  misbehaves, run this first." dotf's most conspicuous daily‑UX gap. (Must follow the probe
  fix, or it lies.)
- **`dotf self-update`** — ~1‑day feature via `creativeprojects/go-selfupdate` + the
  GoReleaser `checksums.txt` the shim already verifies. Every peer single‑binary ships it.
- **`--only <pkg>` / `--skip`** — required for the _per‑tool migration loop_ (port one
  tool, apply just it, verify, delete its script). The capability exists internally (TUI
  per‑package select) but isn't exposed on headless paths (which filter only by `--intent`).
- **`--json` output** on `status`/`plan`/`diff`/`doctor` — highest‑value AI‑integration
  bet for this agent‑heavy repo; "the CLI is the new MCP." Achievable within stdlib `flag`.
- **`NO_COLOR` + stdout/stderr stream discipline** — cheap modern‑CLI hygiene (flagged as
  an audit item, not yet code‑verified).

**Out of scope / EDGE (preserve the boundary):** `dotf update` (update installed _tools_)
collides with additive‑only and duplicates topgrade — defer or scope narrowly to rolling
managers. `--prune`/reconciliation, Go templating, and secrets orchestration stay Won't‑build.

**Evidence Quality**: **High** on the gap; **Medium** on exact priority (some are cheap
wins, some depend on the probe fix).

### Theme 5 — Supply‑chain hardening: verify what you install

**Overview.** `dotf` downloads GitHub‑release assets and runs vendor `curl|bash`; the
Futurist shows "verify what you install" (checksum lockfiles → cosign/SLSA) has gone from
advanced to baseline for 2026‑2028.

**Near‑term, in‑scope steps (Futurist, Journalist, Analogist):**

- A **checksum lockfile** (`dotf.lock`, regenerable via `dotf lock`) — copies aqua's
  `aqua-checksums.json` model; fixes the `script-url` pin‑drift _and_ enables version‑aware
  `Installed()`. **Split by tool class**: pinned artifacts are locked; rolling managers
  (mise/rust) float by design.
- **cosign keyless signing + syft SBOM in `.goreleaser.yml`** (near‑trivial blocks; the
  release currently has `checksums.txt` but no `signs:`/`sboms:`).
- **Optional `cosign verify-blob` in `bootstrap.sh`** (graceful‑skip) and **pin action
  SHAs** in the release workflow.

**Contrarian caution / productive tension.** A lockfile _implies_ a Renovate auto‑bump
pipeline — and **the repo's origin is a self‑hosted Forgejo on Tailscale with a GitHub
push‑mirror; there is no `renovate.json`**. Whether Renovate can even attach to the forge
is an unresolved, decision‑blocking unknown. Ship pinning + optional `dotf lock` _before_
any automation.

**Evidence Quality**: **High** on the trajectory; **Medium** on Renovate feasibility
(un‑verified — needs a 2‑hour spike).

---

## Critical Contradictions (preserved, not smoothed)

| #   | Tension                                                                                           | Which way the evidence leans                                                                                                                                                   |
| --- | ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| C1  | **Retire `custom-script`** (your ask + PRD metric) vs **keep it as a permanent governed adapter** | Decisively toward **triage**: migrate ~5–10 mechanical tools; bless ~9 setup‑steps forever. 7/8 personas + 6 tool precedents.                                                  |
| C2  | **≥80% LOC‑retired metric** (genuine goal) vs **wrong‑shaped vanity metric**                      | Strongly toward **re‑scope** to "`custom-script` ≤ ~5, per‑tool delete‑on‑cutover." Horizontal LOC% never finishes and mis‑directs effort.                                     |
| C3  | **Lockfile** = essential supply‑chain fix vs **maintenance tax** that collides with rolling tools | **Both true — split by tool class.** Lock pinned artifacts; let rolling managers float. Pinning before automation.                                                             |
| C4  | **`update`/`upgrade` verb** = day‑2 essential vs **scope‑creep** vs additive‑only                 | Split the verb: **`self-update` (binary) is in‑scope + cheap**; **`update` (tools) is EDGE** — defer.                                                                          |
| C5  | **`{{.OS}}`/`{{.Arch}}` templating** "absent" vs "present"                                        | **Resolved by code**: present, but renders distro‑family/`amd64`—so unusable for real assets without a `ReleaseOS()` mapper + `x86_64`/`aarch64` vocab + per‑arch checksums.   |
| C6  | **Auto‑detect (ubi/eget)** vs **pinning discipline**                                              | Reconciled by **"detect once, pin the result"** — different lifecycle stages (add‑time vs apply‑time). mise's `ubi`→`github` deprecation confirms: own detection _and_ verify. |
| C7  | **Reproducibility narrative** vs **rolling tools float + apply re‑runs everything**               | The narrative **over‑claims**; fix by wiring `Installed()`, making self‑update opt‑in, and not promising determinism for intentionally‑floating managers.                      |

---

## Strategic Implications — The Recommended Sequence

The ACH collapsed six competing strategies into **one dependency‑ordered sequence**.
Building later phases first makes them launch broken (a `doctor` before the probe fix
_lies_; a migration before the capabilities is _impossible_; automation before the
lockfile is _negative‑value_).

### Phase 0 — Free reframes (documentation, zero code) — **do immediately**

- Reframe `custom-script` as a **permanent governed adapter** for setup‑steps (PRD + CLAUDE.md).
- **Re‑scope the success metric** to "`custom-script` ≤ ~5 entries; per‑tool delete‑on‑cutover."
- Fix the **doc drift**: `bootstrap.sh` is **307 lines**, not "~45"; the manifest is ~37
  packages, not a "7‑package skeleton"; README still teaches the retired `bin/install-*.sh`
  path.
- **Effort: trivial. Value: high (stops mis‑directed effort). Risk: none.**

### Phase 1 — Correctness + the keystone probe + cheap enablers — **the hinge**

- **The keystone**: add `Package.Binary`/`provides` + `installed_when` (path‑exists /
  authority‑query / version‑probe) and wire a real `Installed()` gate into `Engine.Installed`
  - `installPackage` (one gate each; the 4 broken backends stay as fallback). Reuse
    `pkg.go:109-142`.
- Fix **starship** (pin an immutable versioned asset or demote to a tolerant shell‑out;
  re‑fetch the live hash first) and **`apt-repo` codename** (template from host).
- Ship **`--only <pkg>`/`--skip`** (unlocks the per‑tool migration loop) and **`--json`**.
- Add an **apply‑twice idempotency test** and a test for the `Installed()` fix.
- **Effort: low‑med. Value: very high (honest status + idempotent apply + verifiable
  migration + AI‑consumable output). Risk: low.**

### Phase 2 — Backend capabilities — **unblock the migration**

- A **`github-binary` auto‑detecting backend** (or enhance `github-release`): ubi/eget‑style
  OS/arch/libc resolution, archive introspection, bare‑binary + `.gz`, per‑OS binary‑name
  override, `x86_64`/`aarch64`/`darwin` vocabulary, per‑OS/arch checksum map + **`dotf.lock`**.
- A **`git-clone` backend** and a **typed, constrained `post_install` step list** (argv,
  not shell — the fence that stops it becoming `custom-script` v2).
- **Effort: med (the auto‑detect filter pipeline is the real work). Value: high (makes
  native cheaper than the hatch). Risk: med — spike the edge‑case tail first.**

### Phase 3 — Vertical, per‑tool migration — **delete scripts on cutover**

- Migrate the ~4–5 mechanical tools first (start with **lefthook via `toolchain`/npm** — the
  lowest‑risk spike; decide the `node`‑dependency question), then the ~5 capability‑gated
  tools. **Delete each script the moment its native port is verified by the probe.**
- Explicitly **keep the ~9 setup‑steps**; each gets an `installed_when` sentinel so it's
  idempotent and status‑honest.
- **Effort: med, incremental. Value: high (the user's actual goal, done safely). Risk: low
  once Phases 1‑2 land.**

### Phase 4 — Day‑2 + governance — **make it stick**

- `dotf doctor` (probe‑backed), `dotf self-update` (shim‑checksum‑backed), the **CI ratchet**
  blocking new `custom-script` entries + a `dotf status --backends` count line, cosign/SBOM
  in GoReleaser. Resolve the Renovate‑on‑Forgejo question before wiring auto‑bump.
- **Effort: low‑med. Value: high (locks in convergence). Risk: low.**

### Effort / Value / Risk quadrant (Tension Mapper)

- **Cheap · high‑value (do first):** metric reframe + doc fixes; probe+gate; `--only`/`--json`;
  starship + apt‑repo fixes; self‑update; CI ratchet.
- **Expensive · high‑value (sequence):** `github-binary` auto‑detect + `dotf.lock`; `doctor`;
  git‑clone + post_install.
- **Cheap · low‑value (judgment):** `dotf add` scaffolding, `--verbose`.
- **Expensive · NEGATIVE‑value (avoid):** Renovate‑before‑lockfile; hand‑porting setup‑step
  scripts to native; `--prune`/reconciliation; Go templating; secrets orchestration.

---

## Research Gaps — Decision‑Blocking Unknowns (resolve before acting; each < 1 day)

1. **Is the starship pin actually stale on your network right now?** `curl -fsSL
https://starship.rs/install.sh | sha256sum` vs `e9f0bb84…`. Match → Phase‑1 rider;
   mismatch → **immediate hotfix** (default `apply` is broken for every fresh machine).
2. **Confirm the per‑tool migration table** by porting exactly one tool end‑to‑end
   (lefthook/npm) on a throwaway branch — converts the whole table from asserted to measured.
3. **Can Renovate attach to the Forgejo origin?** (self‑hosted on Tailscale; GitHub is a
   mirror; no `renovate.json`.) Gates the entire lockfile‑automation half. 2‑hour spike.
4. **Does one `provides []string` field cleanly express multi‑binary bundles**
   (hashicorp‑tools, aws‑tools, container‑tools) — all‑present vs any‑present — or must
   bundles split into per‑tool entries (changing the metric denominator)? Prototype it.
5. **Real per‑tool binary names** (esp. `fabric` vs Python `fabric` collision) — `command -v`
   each on a provisioned host before setting `Binary`.
6. **Build auto‑detect natively vs vendor a library** — spike the filter pipeline against 5
   real release pages (turso, lefthook, hadolint, tea, coderabbit) and count special cases.

**Shared blind spots (no persona examined):** the symlink engine's clobber‑safety,
`sysconfig` enhancements, the `completion` command, the macOS execution path, uninstall/
rollback beyond snapper, and the overlap between `dotf apply` and the **shell‑side
`system_health`/`list_missing_tools` in `common/tools.sh`** (a shell `doctor` that would
duplicate the proposed `dotf doctor` — reconcile them).

---

## Novel Feature Designs (from the Innovation synthesis — see `synthesis/innovation.md` for full TOML+Go sketches)

1. **State‑probe ladder** — `provides`/`installed_when` + `Installed()` gate. _Feasibility: High._ The keystone.
2. **`github-binary` auto‑detect backend + `dotf lock`** — "detect once, pin the result." _Feasibility: Med._
3. **First‑class `class = pinned|rolling|managed`** — one field resolves lockfile + update + determinism. _Feasibility: High._
4. **`git-clone` backend + typed `post_install`** (argv‑not‑shell fence). _Feasibility: Med._
5. **`dotf doctor` (probe‑backed) + `dotf self-update` (shim‑checksum‑backed).** _Feasibility: Med/High._
6. **Governance‑as‑code**: monotonic CI ratchet + `dotf status --backends` count. _Feasibility: High._
7. **Migration harness**: `--only`/`--skip` + `--json` + `dotf add`. _Feasibility: High._
8. **Lifecycle verbs** (scope extension): a single `Lifecycle` action (`install|update|remove`) threaded through action‑parametric backends — `dotf install <name>`, `dotf install <name> --update|--remove` (reactivating the scripts' already‑shipped `--update`/`--uninstall`), plus a `setup <domain>` verb over a typed `module` schema. _Feasibility: High (custom‑script passthrough) → Med (`setup`/modules)._ See `addendum-lifecycle-and-user-config.md` §A.
9. **User config overlay** (scope extension): `manifest.Merge` + an XDG `~/.config/dotf/packages.d/*.toml` layer so users extend the embedded manifest without forking; `--no-user-config` pins CI to the pristine embedded manifest. _Feasibility: Med._ See addendum §B.

Two schema additions — **`probe`** and **`class`** — carry the bulk of the value; two more — **`actions`** (lifecycle) and a **`module`** table (setup domains) — plus one new function (**`manifest.Merge`**) carry the scope extension.

---

## Recommendations

**For the maintainer (do, in order):**

1. Ship **Phase 0** this week (docs + metric reframe — a PR of prose).
2. Resolve **Unknowns #1 and #2** (starship hash; port lefthook) — < 1 day, decides
   hotfix‑vs‑rider and calibrates the whole plan.
3. Build the **probe + gate + `--only`/`--json`** (Phase 1) — the hinge; unlocks four
   downstream features and makes the migration verifiable.
4. Then the **`github-binary` backend + `dotf.lock`** (Phase 2), then **vertical
   per‑tool migration deleting scripts on cutover** (Phase 3), then **doctor/self‑update/
   ratchet** (Phase 4).
5. **Do not** hand‑port the 9 setup‑steps, adopt reconciliation/templating/secrets, or wire
   Renovate before the lockfile exists.

**For anyone cloning the repo:** the migration is invisible to you until `status` is
honest — the probe fix is what makes `dotf status`/`dotf diff` trustworthy on a fresh
machine, which is the single most user‑facing improvement here.

---

## Conclusion

The instinct behind the request — "finish the job, make the scripts native" — is right in
spirit but wrong in first move and wrong in end‑state. `dotf` doesn't primarily need more
migration; it needs a **truthful read path**, then **backends strong enough to hold the
scripts**, then a **vertical, verifiable, delete‑on‑cutover migration of only the ~10
mechanical tools**, while **blessing ~9 setup‑steps as a permanent, governed
`custom-script` backend**. The highest‑value work is also the least glamorous: one schema
field (`probe`) and one gate wire the disconnected read/write paths together, fix a
status bug that makes the tool lie, make `apply` idempotent, and turn "did the migration
work?" from unanswerable into a unit test. Everything else — auto‑detection, lockfile,
`doctor`, `self-update`, governance ratchet — hangs off that single hinge.

**Bottom Line**: Fix the read path and grow the backends _before_ touching the scripts;
then migrate the ~10 mechanical tools vertically and keep the ~9 setup‑steps as a governed
permanent hatch — and re‑scope "≥80% LOC retired" to "`custom-script` ≤ ~5, verified
per‑tool."

---

## Appendices

### A. Research Artifacts

- `objective.md` — research objective + grounding brief
- `persona-findings/*.md` — 8 persona findings (historian, contrarian, analogist, systems‑thinker, journalist, archaeologist, futurist, negative‑space)
- `synthesis/crucible-analysis.md` — ACH (6 hypotheses → sequence)
- `synthesis/contradiction-mapping.md` — 17 mapped tensions
- `synthesis/tension-mapping.md` — effort/value/risk sequencing
- `synthesis/pattern-recognition.md` — the causal ladder + unlock graph
- `synthesis/negative-space.md` — the 19‑tool migration table + decision‑blocking unknowns
- `synthesis/innovation.md` — 7 concrete feature designs (TOML + Go sketches)
- `evidence/verification-log.md` — orchestrator code‑verified ground truth (F1–F6, C1–C3)
- `addendum-lifecycle-and-user-config.md` — **scope extension (2026‑07‑04)**: anytime per‑tool lifecycle verbs (`dotf install <name> --update|--remove`, `dotf setup <domain>`) + an XDG `~/.config/dotf` manifest overlay. Grounds both in the code and comparators; shows both hang off the same keystone probe and stay in scope.

### B. Persona Contributions

- **Historian**: mise‑off‑asdf precedent; "shrinking hatch" base‑rate; strangler‑fig metric shape.
- **Contrarian**: the starship live‑hash bug; "a permanent hatch is correct design"; trap features.
- **Analogist**: ubi/eget auto‑detect keystone; inverted incentive gradient; detect‑once‑pin‑the‑result.
- **Systems Thinker**: apply/status disconnect; the escape‑hatch reinforcing loop; Meadows leverage ranking.
- **Journalist**: 2025‑2026 competitor capability matrix; `doctor`/`self-update`/`--json` as table stakes.
- **Archaeologist**: the buried per‑script capability requirements; `installed_when` (Ansible `creates:`); authority‑query idempotency.
- **Futurist**: supply‑chain trajectory (cosign/SLSA/lockfiles); auto‑resolution + verification convergence.
- **Negative Space**: the missing verb set; exhaustive schema‑gap list; the un‑enumerated 19‑tool migration table; the untested `toolchain`/npm path.

_Conducted via the Asymmetric Research Squad methodology — 8 personas → crucible → emergent insight → synthesis, with direct code verification of all load‑bearing claims._
