# The Historian — dotf CLI Enhancements & Script-Retirement Migration

_Persona: Historical evolution and forgotten lessons. Research date: 2026-07-04._

## Executive Summary

dotf's central problem — a "shrinking escape hatch" (`custom-script`) that has instead
become the single most-used backend (19 of ~37 packages) while the PRD's headline
"≥80% of shell LOC retired" metric sits explicitly **deferred** — is not novel. It is a
textbook instance of a **migration that has stalled at the last-20% escape-hatch stage**,
and the history of provisioning tooling contains at least one near-perfect precedent that
_succeeded_: **mise's migration off asdf's shell-script plugins onto native backends
(aqua / github)**. That precedent, plus HashiCorp's "provisioners are a last resort"
doctrine and the strangler-fig literature, converge on a single, non-obvious lesson:

> **An escape hatch never shrinks on its own. It shrinks only when (a) native target
> backends exist to receive the migrated entries, (b) _new_ escape-hatch entries are
> frozen by policy so the target stops moving, and (c) deletion is scheduled as
> explicit work. dotf currently has none of the three.**

The good news from history: dotf's _contested_ design choices — additive-only (no
reconciliation), no lockfile, a deliberate escape hatch, plain-file/plain-symlink
"ease of exit" — are the same choices that the **surviving** tools of 2025-2026
(Homebrew, chezmoi, mise, topgrade) made, and the _opposite_ of the choices people are
actively **fleeing** (Nix/home-manager's complexity tax). dotf is on-lineage. The work
is finishing the migration with discipline, not redesigning the philosophy.

---

## Historical Timeline (dotfiles & personal-provisioning tooling)

Each generation solved a specific pain the prior one couldn't. The arc is
**manual → symlink-farming → git-wrapping → templating/secrets → single-binary Go/Rust
declarative**, running in parallel with a **package-install** arc of
**imperative scripts → Ruby/YAML DSL registries → checksum-gated native backends**.

| Era                     | Tool / approach                                                                                                    | Problem it solved                                                                                                                                                                          | New pain it exposed                                                                                                                                                                                               |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| pre-2005                | Manual copy / hand-rolled shell                                                                                    | Any versioning at all                                                                                                                                                                      | Fragile, non-portable one-liners                                                                                                                                                                                  |
| ~2005                   | **GNU Stow** (symlink farm manager)                                                                                | Automatic, safe, _reversible_ linking; directory folding; conflict detection                                                                                                               | Symlinks are "special"; can't do machine-specific content; won't link over an existing real file ([chezmoi design FAQ](https://www.chezmoi.io/user-guide/frequently-asked-questions/design/))                     |
| ~2011                   | **rcm** (thoughtbot), **homesick/homeshick**                                                                       | Documented, reproducible setup + host tagging over raw symlinks                                                                                                                            | Still shell-script glue; still symlink-based                                                                                                                                                                      |
| ~2014                   | **Bare git repo** (`--bare` + worktree, alias)                                                                     | No symlinks, no extra tool — just git                                                                                                                                                      | Host differences need branch-per-host merges ([Arch Wiki: Dotfiles](https://wiki.archlinux.org/title/Dotfiles))                                                                                                   |
| ~2015                   | **dotbot** (YAML bootstrap), **Mackup** (Dropbox sync)                                                             | "One-click" install/upgrade; app-setting discovery                                                                                                                                         | dotbot = still a linker; Mackup = built on a substrate it didn't control (see Failed Attempts)                                                                                                                    |
| ~2015                   | **yadm**                                                                                                           | Git wrapper + system-specific alternates + encryption + bootstrap hooks                                                                                                                    | "It's _just_ a git wrapper"; templating needed unmaintained external deps (j2cli) ([BigGo comparison](https://biggo.com/news/202412191324_dotfile-management-tools-comparison))                                   |
| ~2018                   | **chezmoi** (Go, single binary)                                                                                    | **Regenerate dotfiles as real files** (not symlinks) → enables templates, encryption, per-host, executable/private files; stdlib `text/template` (no external dep); trivial "ease of exit" | Steeper learning curve; filename-metadata (`dot_zshrc`) friction ([Why use chezmoi](https://www.chezmoi.io/why-use-chezmoi/), [design FAQ](https://www.chezmoi.io/user-guide/frequently-asked-questions/design/)) |
| ~2018                   | **NixOS / home-manager**                                                                                           | _Total_ declarative reproducibility from the kernel up                                                                                                                                     | Complexity tax; reproducibility only holds on NixOS; two-package-manager problem on macOS (see Failed Attempts)                                                                                                   |
| **package-install arc** |                                                                                                                    |                                                                                                                                                                                            |                                                                                                                                                                                                                   |
| 2009→                   | **Homebrew formula** (Ruby DSL) → **cask** (same DSL, binaries) → **Brewfile/bundle** (whole-system desired state) | Declarative _desired state_ across formulae/casks/taps/MAS; `brew bundle dump` snapshots a host                                                                                            | Deliberately **no lockfile / no version pinning** (rolling release) ([Brewfile docs](https://docs.brew.sh/Brew-Bundle-and-Brewfile))                                                                              |
| 2019→                   | **asdf** (git-repo bash-plugin per tool)                                                                           | Uniform multi-runtime version management                                                                                                                                                   | Arbitrary third-party bash = supply-chain risk; no Windows; slow; plugin abandonment ([mise plugins](https://mise.jdx.dev/plugins.html))                                                                          |
| 2021→                   | **aqua** (declarative YAML registry, Go)                                                                           | Checksum-verified, lazy, registry-driven CLI installs; Renovate auto-updates pins                                                                                                          | Registry coverage; opt-in checksums by default ([aqua](https://github.com/aquaproj/aqua))                                                                                                                         |
| 2022→                   | **mise** (Rust, single binary)                                                                                     | **Native backends (aqua/github/core-Rust)** replacing bash plugins; contains the irreducible remainder in a governed org                                                                   | Some tools legitimately can't leave scripts — and mise _accepts_ that ([mise↔asdf](https://mise.jdx.dev/dev-tools/comparison-to-asdf.html))                                                                       |
| 2018→                   | **topgrade** (Rust)                                                                                                | Detection-driven "update everything"; silently skips absent tools; TOML config                                                                                                             | Not a provisioner — an updater; but its detect-then-skip model is instructive ([topgrade](https://github.com/topgrade-rs/topgrade))                                                                               |

**Where dotf sits in this lineage.** dotf is a **Go single-binary, TOML-manifest,
native-backend package provisioner fused with an additive symlinker**. It is closest to
chezmoi's _generation_ (Go binary, ease-of-exit, additive) but its center of gravity is
**package provisioning first** rather than dotfile templating first — which makes its
truest analog **a personal-scale aqua/mise (declarative backends + a contained script
tier) bolted onto a Stow-style linker**. That is a coherent, on-trend position.
**Confidence: High** — corroborated by primary docs of every tool named.

---

## Failed Attempts (what history says to avoid)

1. **Full config-management (Ansible/Puppet/Chef) for personal machines — abandoned for
   scale/purpose mismatch.** The recurring first-hand pattern is people _migrating toward_
   Puppet/Ansible for their laptop and then _back off_ it, because "you end up writing
   things twice" — a template _and_ the real file — and the master-agent architecture is
   meaningless for one host ([shibumi.dev](https://shibumi.dev/posts/how-to-handle-dotfiles/)).
   **Lesson for dotf:** resist proposals to re-express dotf as a generic CM engine or to
   add heavy templating "because Ansible has it." The narrow single-host/additive scope is
   the _correct_ survivor. **Confidence: Medium** (primary evidence is experiential blogs,
   not formal studies).

2. **Nix / home-manager — the over-declarative cautionary tale.** Multiple multi-year
   users (4-5 yrs) ditched Nix/nix-darwin/home-manager and returned to **Homebrew + chezmoi
   (+ mise)**, citing a _complexity tax paid for reproducibility that never materialised_
   on macOS, and the "two package managers anyway" trap
   ([Boris Buliga](https://www.d12frosted.io/posts/2025-11-25-nix), [htdocs.dev](https://htdocs.dev/posts/migrating-from-nix-and-home-manager-to-homebrew-and-chezmoi/),
   [BSAG](https://www.rousette.org.uk/archives/a-tour-around-chezmoi/)).
   **Lesson for dotf:** do **not** chase full declarative purity / reconciliation as the
   endgame. The historically-fleeing direction is _more_ declarative-and-controlling; the
   surviving equilibrium is _declarative data + a contained imperative escape hatch_. dotf's
   additive-only stance is a feature, not a debt. **Confidence: High.**

3. **Mackup — building on a substrate you don't control.** Mackup's headline feature
   (symlinked macOS preferences) **broke outright in macOS Sonoma** ("link mode will BREAK
   YOUR PREFERENCES"), and its Dropbox-first default aged badly
   ([Mackup README](https://github.com/lra/mackup)). **Lesson for dotf:** the equivalents of
   "substrate you don't control" in dotf are the **`custom-script` `Installed()` PATH-probe
   heuristic** (`internal/backend/custom_script.go` — `LookPath(pkg.Name)`, which is wrong
   for `aws-tools`/`container-tools`/`gnu-utilities`/`hashicorp-tools` where the package
   name is not a binary on PATH) and the **`apt-repo` codename hard-coding**. Both are
   "works until the substrate shifts." **Confidence: High** (code-verified + Mackup docs).

4. **GUI dotfile managers — never took hold.** No durable GUI dotfile manager emerged; the
   category consolidated on **single static binaries + a text manifest** (chezmoi, aqua,
   mise, topgrade all ship this way). **Lesson for dotf:** the PRD's "NOT building: a GUI"
   is historically vindicated; the `charmbracelet/huh` TUI is the right ceiling.
   **Confidence: Medium** (absence-of-evidence; well-supported by the consolidation trend).

5. **asdf's community-governance attempt — a _premature_ "failure" worth re-reading.**
   asdf tried an `asdf-community` org but _kept giving plugin authors commit access_, which
   "defeats the purpose." mise fixed it by forking the irreducible remainder into
   `mise-plugins` where **only the maintainer + advisory board have commit access**
   ([mise↔asdf](https://mise.jdx.dev/dev-tools/comparison-to-asdf.html)).
   **Lesson for dotf (solo maintainer):** dotf already _has_ the strong version of this —
   all 19 scripts live in-repo under the maintainer's control, not a third-party registry.
   The supply-chain risk is not the scripts dotf owns; it's the **vendor `curl|bash`
   installers those scripts invoke** (see Temporal Patterns). **Confidence: High.**

---

## Forgotten Alternatives (worth reviving for dotf)

Each is a mechanism that older/parallel tools proved out and that maps cleanly onto dotf's
stdlib-flag / TOML / Go / additive constraints.

1. **Content-hash run-once state (chezmoi `run_once_` / `run_onchange_` + `.chezmoidata`).**
   chezmoi persists a **SHA256 of post-render script content** and re-runs a script _only
   when its content/data changes_, tracked in persistent state
   ([use scripts](https://www.chezmoi.io/user-guide/use-scripts-to-perform-actions/),
   [install packages declaratively](https://www.chezmoi.io/user-guide/advanced/install-packages-declaratively/)).
   dotf's `custom-script` today re-invokes every apply and leans on each script's own
   self-skip. **Reclaim:** a tiny per-package content-hash state file makes the escape hatch
   _idempotent and observable_ without abandoning it — the single most valuable "forgotten"
   mechanism for making a shrinking hatch behave while it shrinks. **Effort: Med / Risk: Low
   / Value: High. Confidence: High.**

2. **`brew bundle dump` — snapshot an existing host into the manifest (reverse operation).**
   Homebrew, rcm, and homesick all had a "capture current state" primitive; dotf has only
   forward apply. **Reclaim:** `dotf export` / `dotf snapshot` that emits a `packages.toml`
   fragment from what's installed — huge onboarding value, and it's the historically-proven
   way people bootstrap a manifest without hand-writing it
   ([Brewfile dump](https://docs.brew.sh/Brew-Bundle-and-Brewfile)). **Effort: Med / Risk:
   Low / Value: High. Confidence: Medium** (value is inferred from adoption, not measured).

3. **aqua-style checksum discipline + a Renovate-style refresh.** aqua pins **SHA256 for
   binaries, SHA512 for config**, supports `require_checksum: true`, and — critically —
   uses **Renovate to auto-regenerate checksums** so pins don't rot into staleness
   ([checksum verification](https://dev.to/suzukishunsuke/checksum-verification-by-aqua-5038)).
   dotf's `github-release` and `script-url` backends already pin sha256 — the historically
   _correct_ move. **Reclaim:** the automation half (a CI job / renovate rule that bumps
   pins + checksums) is the answer to the perennial "pinning is a maintenance burden"
   objection. **Effort: Med / Risk: Low / Value: Med. Confidence: High.**

4. **Detection-driven "silently skip if absent" (topgrade) as an explicit contract.**
   topgrade's whole model is `require()`-probe then run-or-skip, so one binary+config is
   portable across machines with different toolsets ([topgrade](https://github.com/topgrade-rs/topgrade)).
   dotf already has `applicable(info)` per backend — the forgotten step is making
   **applicability first-class in the manifest** (per-package `when`/`os`/`arch` predicates)
   so intents compose cleanly. **Effort: Low / Risk: Low / Value: Med. Confidence: High.**

5. **A _named_, honest "setup-step" tier distinct from "package install."** mise separates
   _backends that install a versioned tool_ from _plugins that just run setup_. dotf lumps
   `tpm` (git clone), `tmux-sessionizer` (cargo-from-git), `gnu-utilities` (brew meta-list),
   and `mise`/`rust` (rolling version-manager bootstraps) into the same `custom-script`
   bucket as things that are _really_ just github-release binaries. **Reclaim:** split the
   escape hatch into `custom-script` (the shrinking, migrate-me bucket) vs a legitimate
   `setup-step` (the permanent, contained bucket). Naming the irreducible remainder honestly
   is what lets the _other_ bucket credibly shrink to near-zero. **Effort: Low / Risk: Low /
   Value: High. Confidence: High.**

6. **Homebrew's explicit "no lockfile, rolling release" _decision_ (not a gap).** Homebrew
   documents that it **will not** add a Brewfile lockfile because it is a rolling-release
   manager ([Brewfile docs](https://docs.brew.sh/Brew-Bundle-and-Brewfile)). **Reclaim:**
   dotf should _document_ its additive/no-reconciliation stance as a deliberate design
   decision citing this lineage, so it reads as intent rather than incompleteness.
   **Effort: Low / Risk: Low / Value: Med. Confidence: High.**

---

## Temporal Patterns

1. **The declarative pendulum has a stable center, and it is a hybrid.** The industry swung
   manual → heavy-declarative (Nix/Ansible) → _back_ to lightweight hybrid (Homebrew +
   chezmoi + mise). The surviving equilibrium across all winners is **"declarative data +
   contained imperative escape hatch,"** never full declarative and never full imperative.
   dotf is already at that center. **Confidence: High.**

2. **The 2024-2026 security-tier ratchet.** Both mise and aqua moved the whole ecosystem
   toward **checksum / signature / attestation-gated backends** and _away from arbitrary
   third-party bash_: mise now refuses **new** asdf/vfox registry entries "for supply-chain
   security reasons" and steers everything to aqua/github
   ([mise↔asdf](https://mise.jdx.dev/dev-tools/comparison-to-asdf.html)); aqua ships
   Cosign/SLSA/attestation verification. dotf's **18 unpinned vendor `curl|bash`
   custom-scripts** (only `script-url` pins today) sit on the _wrong_ side of this ratchet.
   The historical direction of travel is unambiguous. **Confidence: High.**

3. **Escape hatches persist for years unless actively fought — a measurable base rate.**
   Terraform provisioners have been "a last resort" since ~2017 and remain widely used
   ([Terraform provisioners docs](https://developer.hashicorp.com/terraform/language/provisioners)).
   asdf bash plugins: even after mise's multi-year migration campaign, **<25% of mise tools
   still default to the insecure backend** — and that is considered a _success_
   ([mise↔asdf](https://mise.jdx.dev/dev-tools/comparison-to-asdf.html)). The base rate says
   dotf's 19 will **not** trend to zero passively; left alone they trend _up_.
   **Confidence: High.**

4. **The winners are all single static Go/Rust binaries with a text manifest.** chezmoi
   (Go), aqua (Go), mise (Rust), topgrade (Rust). dotf (Go 1.26, single binary, TOML) is
   squarely in the winning morphology. **Confidence: High.**

5. **"Ease of exit" quietly became a first-class design value.** chezmoi ("stop using it
   any time — the files are already real"), and the Nix exodus explicitly cites Nix's
   _hard_ exit ("multi-step, sudo, filesystem surgery") as a reason to leave
   ([BSAG](https://www.rousette.org.uk/archives/a-tour-around-chezmoi/)). dotf's additive
   symlinks + plain in-repo scripts + no proprietary persistent state = **excellent exit
   score today.** Protect it: do not add heavy hidden state (this is the one caution on
   Forgotten-Alternative #1 — keep the content-hash state small, local, and inspectable).
   **Confidence: High.**

---

## Lessons for dotf's Script-Retirement Migration (core deliverable)

The strangler-fig literature and the mise/asdf precedent give a concrete, ordered playbook.
The failure mode the PRD is drifting into is documented precisely: _"You rewrite 80% of
functionality in 20% of the time; the last 20% — edge cases, integrations — takes forever…
compatibility layers survive past their usefulness… the system sits in a permanent
half-migrated state… if it's not scheduled, the cleanup won't happen"_
([oneuptime strangler-fig](https://oneuptime.com/blog/post/2026-01-24-strangler-fig-migration-pattern/view),
[scaledbydesign](https://www.scaledbydesign.com/blog/strangler-fig-migration)). dotf has
**already stalled at exactly this stage** (metric deferred at Phase 7, ~12% removed).

1. **Build the missing backend capabilities FIRST — you cannot migrate scripts into
   backends that can't hold them.** mise's mass-migration only became possible _"when
   multiple backends were in place"_ ([mise↔asdf](https://mise.jdx.dev/dev-tools/comparison-to-asdf.html)).
   The capabilities the 19 scripts need and dotf's 7 backends lack: **`{{.OS}}`/`{{.Arch}}`
   asset templating** (github-release), **version pinning**, **multi-artifact
   extraction**, **post-install steps** (symlink/chmod/PATH), a **`git-clone` setup-step**
   (for tpm), and an **`rpm-repo`** variant. Ship these before touching the scripts.
   **This is the true blocker, and it must come first.** **Confidence: High.**

2. **Slice vertically, one tool per change, and _delete_ the script when its replacement
   proves out** — transform → coexist → eliminate. History's warning: horizontal slices
   ("migrate all the AI tools", "migrate the data layer") _never finish_
   ([scaledbydesign](https://www.scaledbydesign.com/blog/strangler-fig-migration)). Each PR
   retires exactly one `bin/install-*.sh` and removes it. mise did this: _"a checklist of
   all remaining tools, went through them one by one."_ **Confidence: High.**

3. **Freeze new escape-hatch entries by policy — this is the single highest-leverage
   anti-stall move.** mise's endgame wasn't heroic rewriting; it was an **acceptance policy
   that gates NEW entries by backend security tier** ("new vfox/asdf tools are not
   accepted… use aqua or github"). Terraform's parallel is "open a feature request with the
   provider" rather than reach for a provisioner. **dotf needs a documented rule: a new
   package MUST use a native backend unless it carries a written reason it can't.** Without
   the freeze, the 19 becomes 25 and the target never stops moving. **Confidence: High.**

4. **Accept an irreducible remainder and _contain_ it — zero is the wrong target.** mise's
   mature position: _"It's not going to be possible to convert every plugin… making
   everything a core plugin was rejected — too much work and maintenance."_ dotf's genuine
   setup-steps — `tpm` (git clone), `tmux-sessionizer` (cargo-from-git), `gnu-utilities`
   (brew meta), `mise`/`rust` (rolling version-manager bootstraps) — should move to a
   _renamed, honest_ `setup-step` tier (Forgotten Alternative #5), given content-hash
   idempotency (Forgotten Alternative #1) and a one-line documented reason each. Redefine
   success as **"every tool that _can_ be native _is_, and every remaining shell-out is
   named, justified, and idempotent"** — a _convergent, completable_ definition, unlike the
   horizontal "≥80% LOC" metric which history says never completes. **Confidence: High.**

5. **Fix `Installed()` before/as part of migration — it's the "invisible to plan, not in
   state" failure mode.** Terraform's core critique of the escape hatch is that _plan can't
   see what the script does and state doesn't record it_, so _idempotency becomes your
   burden_ ([Terraform provisioners](https://developer.hashicorp.com/terraform/language/provisioners)).
   dotf's `custom-script.Installed()` is exactly that gap: a `LookPath(pkg.Name)` PATH-probe
   that is simply wrong for `aws-tools`, `azure-cli`, `container-tools`, `gnu-utilities`,
   `hashicorp-tools` (no binary of that name). Give each package an explicit `binary` /
   `check` field (asdf/aqua both declare the provided binaries rather than guessing). **You
   cannot safely retire a script if you can't reliably tell whether its replacement
   worked** — so this is a _prerequisite_, not a nice-to-have. **Confidence: High**
   (code-verified in `internal/backend/custom_script.go`).

6. **Extend checksum discipline to the escape hatch itself.** The 18 vendor `curl|bash`
   installers are the real supply-chain soft underbelly — server-side payload swapping
   defeats "I read it first," and streamed execution risks partial-run corruption
   ([lukespademan](https://lukespademan.com/blog/the-dangers-of-curlbash/),
   [sysdig](https://www.sysdig.com/blog/friends-dont-let-friends-curl-bash)). dotf's
   `script-url` backend _already_ pins sha256 — the historically correct pattern; the
   migration should route as many of the 18 as possible through it (or `github-release`),
   and pair pins with aqua-style automated refresh (Forgotten Alternative #3).
   **Confidence: High.**

7. **Put the decommission on the roadmap with a date, and freeze feature-adds to the legacy
   path.** The strangler-fig consensus: _"Schedule the deletion. If it's not scheduled, it
   won't happen… set a decommission deadline and stick to it… freeze new feature work in
   the legacy system so the target stops moving."_ The PRD _deferring_ the metric is the
   historically-attested first step toward a permanent half-migration. **Confidence: High.**

---

## Key Insights (the non-obvious, non-common-knowledge findings)

- **dotf's exact migration has a documented, successful precedent — and the thing that made
  it work is counter-intuitive.** mise off asdf shell-plugins succeeded _not_ by rewriting
  scripts faster, but by (1) building native backends first, (2) permanently **freezing new
  script entries via an acceptance policy**, and (3) **containing rather than eliminating**
  the last quarter. Effort spent on a _policy gate_ buys more convergence than effort spent
  on rewriting the next script.
- **The "shrinking escape hatch" has a measurable base rate of _not_ shrinking.** Terraform
  provisioners (≈9 years "last resort," still ubiquitous) and asdf plugins (<25% even after
  a funded multi-year mise campaign, and that's the win) show escape hatches are _sticky by
  default_. dotf's is behaving exactly to type. This reframes the ask: the goal is not "get
  to zero," it's "stop the growth and shrink the _migratable_ subset to zero."
- **The PRD's success metric is the wrong shape.** "≥80% of shell LOC retired" is a
  _horizontal_ slice — the exact framing the strangler-fig literature names as the one that
  "never finishes." A _vertical, per-tool, delete-on-cutover_ metric ("N of 19 scripts
  deleted; remainder named+justified+idempotent") is convergent and completable.
- **dotf's most-criticized design choices are its most historically-validated ones.**
  Additive-only (= Homebrew "no lockfile", chezmoi additive), a deliberate escape hatch
  (= chezmoi/Terraform/aqua all keep one), and ease-of-exit (= the explicit reason people
  flee Nix). The contrarian question "is retiring custom-script even worth it?" answers:
  _retiring the migratable ones, yes; eliminating the hatch entirely, no — that's the Nix
  mistake in miniature._
- **The real supply-chain risk isn't the 19 in-repo scripts — it's the vendor `curl|bash`
  they call.** dotf owns the scripts (unlike asdf's third-party plugins), so the trust
  boundary is already better than the ecosystem's cautionary tales. The exposure is the
  _unpinned_ upstream installers, which `script-url`'s existing sha256 pinning already knows
  how to close.

---

## Evidence Quality

- **High-confidence, primary-source backbone:** chezmoi.io (design FAQ, scripts, install
  packages declaratively), aquaproj (README + checksum article), mise.jdx.dev (asdf
  comparison + plugins — _authored by the maintainer who ran the exact migration dotf
  faces_, a rare primary source), Homebrew docs (Brewfile/bundle), HashiCorp Terraform docs
  (provisioners "last resort"). These directly ground the migration and escape-hatch
  lessons.
- **Medium-confidence, experiential:** the Nix-exodus and Ansible-for-dotfiles narratives
  are well-corroborated _across multiple independent first-hand blogs_ but are not formal
  studies; treat the _direction_ as reliable, specific magnitudes less so.
- **Code-verified (High):** backend counts (19 custom-script / 14 pkg / 1 each of
  toolchain/script-url/github-release/aur/apt-repo) and the `custom-script.Installed()`
  PATH-probe were read directly from `manifest/packages.toml` and
  `internal/backend/custom_script.go` in this repo.
- **Freshness:** the security-tier ratchet (mise/aqua) and the Nix-exodus posts are
  2024-2026 — current for a fast-moving space. Strangler-fig and Terraform-provisioner
  doctrine are stable/evergreen. GNU Stow (~2005) is historical-stable.

---

## Contradictions & Uncertainties

1. **"Use scripts sparingly / declarative is better" (chezmoi, Terraform, aqua) vs. "some
   scripts must stay; converting everything is wrong" (mise).** _Resolution:_ both are true
   and dotf should hold both — **minimize AND contain**; zero is the wrong target. The
   contradiction dissolves once you split "migratable" from "irreducible."
2. **`curl|bash` critics vs. defenders.** Defenders argue _any_ web install is equally
   vulnerable and it reduces to trusting the domain ([btm gist](https://gist.github.com/btm/6700524)).
   _Resolution:_ checksum pinning is the tiebreaker both sides concede improves matters —
   which is precisely dotf's `script-url` posture.
3. **Rolling-release / no-pin (Homebrew) vs. pin-everything (aqua, lockfiles).** Genuinely
   unresolved in the ecosystem. _Implication for dotf:_ either is defensible, but the choice
   must be **explicit and documented**, not accidental.
4. **More-declarative-is-better (aqua, Nix) vs. the Nix exodus (too-declarative is a tax).**
   _Resolution:_ there is a sweet spot, and the 2025-2026 evidence puts it at
   Homebrew+chezmoi+mise-style hybrids — right where dotf already sits.
5. **Uncertainty:** I did not find a _quantified_ study of how often personal-provisioning
   migrations complete vs. stall; the base-rate claim rests on the two strongest available
   proxies (Terraform provisioners' longevity, mise's <25% residual). Confidence in the
   _pattern_ is High; in any precise percentage, Low.

---

## Search Queries Executed

Applied SCAMPER variation (Substitute: aqua-vs-scripts; Combine: chezmoi data+script;
Adapt: Terraform/IaC escape hatches → dotfiles; Modify: personal vs fleet scale; Reverse:
migrating _away from_ Nix; Eliminate: what tools removed/abandoned).

1. `history of dotfiles management tools GNU Stow rcm dotbot yadm chezmoi evolution`
2. `chezmoi why created design decisions vs GNU Stow symlink dotfiles`
3. `Ansible Puppet Chef for personal dotfiles overkill abandoned why failed`
4. `chezmoi run_once script versus declarative package install escape hatch design philosophy`
5. `Terraform local-exec provisioner last resort anti-pattern why discouraged escape hatch`
6. `aqua registry declarative CLI version manager checksum design how it works aquaproj`
7. `asdf mise plugin system evolution shell scripts to core plugins backends registry lessons`
8. `Homebrew formula Ruby DSL history Brewfile bundle cask evolution declarative package definition`
9. `software migration stalls last 20 percent strangler fig pattern never completed legacy escape hatch`
10. `topgrade tool design how it detects package managers step configuration philosophy unify updates`
11. `migrating away from Nix home-manager back to Homebrew chezmoi complexity reproducibility not worth it`
12. `curl pipe bash install script security risks history checksum pinning supply chain vendor installer criticism`
13. `Mackup dotfiles manager problems abandoned Dropbox sync why people stopped using`

Plus targeted repo reads: `manifest/packages.toml` (backend counts), `internal/backend/custom_script.go` (`Installed()` heuristic), `bin/install-{tpm,tmux-sessionizer,gnu-utilities}.sh` (setup-step characterization).
