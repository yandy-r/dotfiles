# The Analogist — Cross-Domain Parallels for `dotf`

Persona: The Analogist (Asymmetric Research Squad)
Subject: features/enhancements/fixes for the `dotf` CLI, incl. migrating
`bin/install-*.sh` into native TOML-manifest backends.
Date: 2026-07-04

---

## Executive Summary

`dotf` today sits at an unusual point in the design space: it has a _declarative
TOML front-end_ but its most-used backend (`custom-script`, 19 of ~37 packages) is a
raw imperative shell-out. Every mature tool that has faced this exact tension —
package managers (Homebrew, aqua, mise, ubi/eget), IaC (Terraform, Ansible), and
build systems (Bazel) — has converged on the _same three structural moves_:

1. **Auto-derive the boring 80%** so the declarative path is _cheaper_ than the escape
   hatch (ubi/eget asset auto-detection; aqua's shared registry). dotf's
   `github-release` backend is the mirror image — it _requires_ hand-authored
   `asset` templates, pinned `tag`, and `sha256`, so it is _more_ work than writing a
   `custom-script` entry. This inverted gradient is why the escape hatch won.
   **This is dotf's single highest-leverage fix: a ubi/eget-style
   `github-binary` auto-detection backend.**

2. **Make the escape hatch structurally uncomfortable** — never remove it (all five
   domains keep it), but _govern_ it: Ansible's `ansible-lint`
   `command-instead-of-module` rule, Terraform's "last resort" doc framing +
   opaque-plan penalty, Bazel's sandbox that _detects_ non-hermetic escapes. dotf
   should add a **lint that blocks _new_ `custom-script` entries** and a
   status-line that counts them, converting a silent gradient into a visible one.

3. **Content-address for idempotency & drift** — chezmoi's `run_onchange_` SHA256
   hashing, Cargo.lock/go.sum checksums, Nix's input-hash store. dotf's
   `Installed()` is a bare `LookPath(name)` PATH probe with no version awareness; a
   **lockfile + resolved-version record** closes the drift gap without importing
   Nix's complexity (Go's own `go.mod`/`go.sum` split is the exact model dotf should
   copy, since dotf _is_ Go).

The through-line: **the maintainer always takes the path of least resistance; the
only durable fix is to bend the resistance gradient so the native backend is the
easy path and the escape hatch is the effortful, visible, linted one.** That is a
Donella-Meadows leverage-point intervention (change the _rules and information
flows_, not the individual entries).

---

## Package-Manager / Registry Analogies

This is the _closest_ analogue domain — the transferable mechanisms are near-literal.

### A1. ubi / eget — universal auto-asset-detection (the keystone borrow)

**Transferability: HIGH.** `ubi` (houseabsolute, Rust) and `eget` (zyedidia, Go)
both install prebuilt binaries from GitHub/GitLab releases **with zero per-tool
config**: you give `owner/repo` and they deduce the correct asset by a multi-stage
filter pipeline over the release asset filenames
([ubi](https://github.com/houseabsolute/ubi),
[eget DOCS](https://github.com/zyedidia/eget/blob/master/DOCS.md)):

ubi's pipeline: (1) filter by recognized extension → (2) OS regex → (3) CPU-arch
regex (careful: rejects "64" from a _version_ string, only real arch tokens like
`x86-64`/`arm64`) → (4) libc filter (musl-vs-gnu, _prefers_ an explicit `-musl`
asset on a musl host, falls back to no-libc-indicator) → (5) 64-over-32-bit + best
match → (6) deterministic alphabetical tie-break so it "always picks the same one
every time" → (7) look _inside_ the archive for the executable.

Crucially it also solves the **repo-name ≠ binary-name** problem (`BurntSushi/ripgrep`
→ `rg`, not `ripgrep`) via an `exe` field, and **checksum sidecars** (`xxx.sha256`)
are auto-verified. eget adds `--asset` string filters (repeatable, `^` anti-match)
and auto-verifies `.sha256`/`.sha256sum` sidecars.

**The direct mapping to dotf's gap.** dotf's `github_release.go` does the _opposite_:
`templateGitHubAsset()` substitutes only `{{.OS}}` and `{{.Arch}}` into a
**hand-authored asset string**, requires an explicit `tag`, requires an explicit
`sha256`, and `findReleaseBinary()` matches a file whose basename `==` `pkg.Name`
_exactly_ (no "starts-with" fallback, no `exe` override, no musl/libc handling, no
`.exe` logic). So every `github-release` entry is per-asset boilerplate — _more_
effort than a `custom-script` one-liner. **Borrow ubi's filter pipeline** as a new
`github-binary` backend (or an auto-mode of the existing one) where `asset` becomes
_optional_ and, when omitted, dotf runs the OS→arch→libc→tie-break filter itself.
This alone could retire a large share of the 19 custom-script tools that are just
GitHub-release binaries (atuin, lefthook, tea, coderabbit, several of the
`*-tools`).

_Caveat worth flagging (MED confidence):_ ubi's maintainer explicitly says "the exact
set of steps … is not considered part of the API and may change." Auto-detection is
a _heuristic_; dotf should keep the explicit `asset`/`sha256` path as the pinned,
reproducible override for anything security-sensitive (see reproducibility section).

### A2. aqua — a shared _registry_ + Renovate version-bumps (kill the maintenance tax)

**Transferability: MED-HIGH (mechanism), LOW (adopt-wholesale).** aqua (aquaproj, Go)
splits _what to install_ into a **Standard Registry** (a community-maintained
`registry.yaml` pinned to an immutable ref) and _your selection_ into a tiny
`aqua.yaml`. Adding a new tool = a PR to the registry, not a plugin you maintain
([aqua repo](https://github.com/aquaproj/aqua),
[aqua config](https://aquaproj.github.io/docs/reference/config/)). The decisive
mechanism for a **solo maintainer**: the `# renovate: depName=aquaproj/aqua-registry`
comment lets **Renovate auto-bump both versions _and_ checksums**. aqua's own pitch is
exactly dotf's problem: _"If you verify the checksum in a shell script, you have to
update the checksum along with the version, which is bothersome … using aqua you
don't have to maintain shell scripts anymore"_
([aqua DEV.to](https://dev.to/suzukishunsuke/aqua-declarative-cli-version-manager-1ibe)).

**Mapping to dotf.** dotf can't outsource to aqua's registry (different scope), but it
can borrow the **Renovate-comment convention**: annotate each `github-release`
`tag =`/`sha256 =` line in `packages.toml` with a Renovate regex-manager marker so
`tag` and `sha256` bumps become automated PRs. This directly attacks the _reason_
pinned `github-release` entries feel expensive — the manual "bump tag → recompute
sha256" chore that pushes the maintainer toward the unpinned `custom-script`. (dotf's
release memory already notes GoReleaser/Forgejo tooling; Renovate config is a small
additive.)

### A3. mise — an _explicit backend-preference ladder_ (governance as a default)

**Transferability: HIGH (as a doctrine, not code).** mise supports ~16 backends and
publishes a **recommended decision order**: prefer `aqua` (curated + checksums) →
`github`/`gitlab` (direct release binaries) → language managers (`cargo`/`npm`/`go`/
`pipx`) → `http` (last resort); _avoid_ `asdf`/`vfox` for new tools on supply-chain
grounds ([mise backends](https://mise.jdx.dev/dev-tools/backends/),
[backend architecture](https://mise.jdx.dev/dev-tools/backend_architecture.html)).
Notably **mise deprecated its `ubi` backend in favor of a native `github` backend**
("replace `ubi:owner/repo` with `github:owner/repo` … ubi is not accepted for new
registry entries") — evidence that _owning_ the auto-detection in-tree (vs. shelling
to an external) is the direction of travel, which validates building it natively in
dotf rather than shelling to `ubi`/`eget`.

**Mapping to dotf.** Publish an explicit **"backend selection order" doctrine** in
`CLAUDE.md`/`packages.toml` header: `pkg` (distro-native, gets security updates free)
→ `github-binary` (auto-detect) → `toolchain` → `apt-repo`/`aur` → `script-url`
(pinned vendor installer) → `custom-script` (_genuine_ setup-steps only). This turns
the migration from a vague goal into a _rule with a ranking_, so every future package
has an obvious "correct" home and `custom-script` is visibly the bottom rung.

### A4. Homebrew — declarative header + Ruby `def install` escape hatch, _managed_

**Transferability: HIGH (the two-tier shape).** A Homebrew formula is declarative at
the top (`url`, `sha256`, `resource`, `patch`, `bottle`, `livecheck`) and drops to a
full-Ruby `def install`/`post_install` only when needed
([Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)). The governance that keeps
the escape hatch honest: (a) `brew bump-formula-pr` **automates** `url`+`sha256`
updates (same lever as aqua/Renovate); (b) `livecheck do` declares _how to detect a
new version_ declaratively; (c) `post_install` is a **structured, declared** hook, not
free-form — it runs regardless of build-vs-bottle, and even its network access is
policy-controlled (`HOMEBREW_FORMULA_POSTINSTALL_NETWORK`).

**Mapping to dotf.** Two borrows: (1) a **`post_install` field** on `Package` (a
declared, bounded step) is the pressure valve for the "binary install + one setup
command" tools (e.g. `tpm` = clone-then-nothing, `tmux-sessionizer`), letting them
leave `custom-script` for `github-binary` + a small declared hook rather than a whole
script. (2) a **`livecheck`-style version-source field** so `dotf update` knows where
to look for the latest tag — the missing half of a real `update` command.

### A5. topgrade — the unified `update` command + `--only`/`--skip` + failure summary

**Transferability: HIGH (feature model).** topgrade (Rust) auto-detects installed
tools and runs each one's updater, with `--dry-run`, `-y`, `--only`, `--disable`,
`--list-steps`, a TOML config with `[commands]` custom steps, and a **run-wide
failure summary** that keeps going and reports at the end
([topgrade](https://github.com/topgrade-rs/topgrade)). dotf already has a failure
summary in `apply` (per the brief) — topgrade shows the natural next features:
**`dotf update`** (re-resolve floating tools + bump), **`--only`/`--skip` filters**,
and **`--list-steps`** for discoverability. topgrade's "missing tools silently
skipped so the same binary+config works across machines" is exactly dotf's
single-binary, additive philosophy — strong validation that these fit dotf's scope.

---

## IaC / Build-System Escape-Hatch Analogies

The central-tension domain: how declarative systems _frame and fence_ the imperative
escape hatch without deleting it.

### B1. Terraform `provisioner` / `local-exec` — "a last resort," and _why_

**Transferability: HIGH (framing + governance).** HashiCorp's docs literally brand
provisioners a **"last resort"** and enumerate the costs that make them one
([TF provisioners](https://developer.hashicorp.com/terraform/language/provisioners),
[Spacelift](https://spacelift.io/blog/terraform-provisioners)):

- **`plan` can't see inside them** — the change is opaque to review. (dotf parallel:
  `custom-script` is a black box to `dotf diff`/`status`; its `Installed()` can only
  PATH-probe.)
- **Actions aren't tracked in state** — remove the provisioner and nothing reverts.
  (dotf: no record of _what_ a script did; drift is invisible.)
- **Idempotency becomes _your_ responsibility** — "`useradd`, `mkdir`, `git clone`
  … written for first-time setup … fail on a second run." (dotf's `custom_script.go`
  comment admits exactly this: it leaves `FORCE_INSTALL` unset and _relies on the
  scripts self-skipping_.)

The **decision framework** is the transferable gold: Terraform teaches a _ladder of
alternatives to exhaust first_ ("Is there a resource for this? → use it. One-time
side effect? → maybe a provisioner"). This is the same ladder as mise's backend order
(A3). Best practices that map cleanly: make commands idempotent; be explicit about the
interpreter/shell (dotf already forces `bash`); `on_failure = continue` for
non-critical steps (maps to dotf's non-fatal failure summary); **document thoroughly
_because_ plans are opaque.**

### B2. Ansible `command`/`shell` vs native modules — the lintable governance rule

**Transferability: HIGH (this is the concrete governance mechanism dotf lacks).**
`command`/`shell` are **not idempotent by default** — they _always_ report `changed`
because "Ansible cannot know if the command actually changed anything"
([oneuptime](https://oneuptime.com/blog/post/2026-01-24-ansible-changed-status-idempotency/view)).
The doctrine: **always check for a dedicated module first**; when you must shell out,
restore idempotency with `creates:`/`removes:` (file-based guards), `changed_when:`
(output-based), or `when:` (state checks). And — the keystone governance artifact —
**`ansible-lint` ships a `command-instead-of-module` rule that flags shelling out
where a native module exists.**

**Mapping to dotf (highest-value governance borrow).** Add a **repo lint that fails CI
when a _new_ `custom-script` entry is added** (or when count exceeds a ratchet
baseline that only ever decreases). This is the economic gradient-bender (see D2): it
makes the escape hatch _cost something at commit time_, exactly as `ansible-lint`
does. Also borrow the **`creates:`/`removes:` idea** as a `Package.creates` field — a
path whose existence means "already installed" — replacing the fragile
`LookPath(name)` heuristic in `custom_script.go` and `github_release.go` with an
explicit, script-agnostic idempotency guard (many of the 19 tools install to a known
path, e.g. `~/.tmux/plugins/tpm`, `~/.local/bin/mise`).

### B3. Bazel `genrule` — the escape hatch, and the _escalation path_ above it

**Transferability: MED-HIGH.** Bazel's `genrule` runs arbitrary shell and "is easy to
reference arbitrary programs on PATH, however this makes the command non-hermetic and
may not be reproducible"
([Bazel general rules](https://bazel.build/reference/be/general)). The transferable
structure is the **graduated escalation ladder**: for a single tool prefer
`run_binary` over raw `genrule`; escalate to a **custom Starlark rule** only when
logic is complex, needs testability, or is platform/toolchain-aware
([Aspect blog](https://blog.aspect.build/genrule-bestrule)). And Bazel **sandboxes to
_detect_ escapes** — "allows you to escape hermeticity but makes it easy to stay
inside: just use the standard rules."

**Mapping to dotf.** Two intermediate rungs between "pinned `github-binary`" and
"full `custom-script`" would absorb most of the remaining 19: (1) a **`post_install`
declared hook** (A4) = dotf's `run_binary` equivalent — a bounded single command; (2)
keep `custom-script` only for genuinely complex, platform-branching setup logic
(mise/rust rolling version managers, `gnu-utilities`), the analogue of a full custom
rule. The lesson: _don't try to force everything native — provide the missing middle
rung so the jump to `custom-script` is only for the genuinely irreducible._

---

## Biological Analogies

### C1. Homeostasis / idempotent convergence — control-theory feedback loop

**Transferability: MED (conceptual framing, sharpens the idempotency spec).**
Configuration management maps onto a **control-theoretic feedback system**: desired
state = _set point_, the reconcile loop (detect drift → compare → correct) =
homeostasis, the engine = the _controller_ minimizing error between actual and desired
([IaC idempotency](https://www.vipeducationcommunity.com/infrastructure-as-code-idempotency-and-state-management-building-predictable-systems-that-never-misbehave/)).
The sharp research finding: **idempotency failures silently break convergence** —
Puppet studies show a non-idempotent step (an `unzip` that's skipped on rerun) leaves
"dormant bugs undetected," and "a stuck configuration that will never converge"
([convergence research](https://www.researchgate.net/publication/306058901_Asserting_Reliable_Convergence_for_Configuration_Management_Scripts)).

**Mapping to dotf.** dotf's `Installed()` PATH-probe is a _weak controller_: it senses
"binary present" but not "correct version." A rolling tool (mise/rust/atuin) that
self-updates inside `custom-script` is doing _homeostasis by side-effect_ — invisible
to dotf's loop. Borrow the **"tolerance band" idea** (drift correction only past a
threshold): dotf should record a _resolved version_ per package (lockfile, E1) so the
controller can compare set-point-vs-actual and report drift, rather than treating
"present" as "converged."

### C2. Keystone species / Meadows leverage points — _which one change unlocks the most_

**Transferability: MED-HIGH (prioritization lens — directly answers "the 20%").**
Ecology's _keystone species_ = a component whose impact vastly exceeds its size;
remove it and the arch collapses
([keystone species](https://en.wikipedia.org/wiki/Keystone_species)). Meadows'
**leverage points** rank _where to intervene_: low-leverage = tweaking parameters
(#12); high-leverage = changing the system's _rules/incentives_ (#5), _information
flows_ (#6), and _goals_ (#3)
([Meadows](https://donellameadows.org/archives/leverage-points-places-to-intervene-in-a-system/)).

**Mapping to dotf — this is the sequencing answer.** Migrating 19 scripts one-by-one
is a _#12 parameter tweak_ (low leverage). The **keystone interventions** are:

- **Rules (#5):** the `github-binary` auto-detect backend — one feature that unlocks
  the _most_ migrations at once (every release-binary tool). This is the keystone.
- **Information flows (#6):** a `custom-script`-count in `dotf status` + the CI lint —
  makes the escape-hatch gradient _visible_.
- **Incentives (#5):** Renovate auto-bump — removes the maintenance tax that _drives_
  the wrong-direction choice.

Meadows' warning that people "push the leverage point in the wrong direction" is
apt: retiring `custom-script` _manually, entry by entry_ is pushing the low-leverage
lever hard; building the auto-detect backend + lint is the high-leverage move.

### C3. Immune system — self/non-self drift detection

**Transferability: LOW-MED (aspirational, mostly out-of-scope).** Artificial Immune
Systems detect anomalies as "non-self" against a learned "self" baseline, and are
explicitly designed for _changing_ environments (OS upgrades, migrations)
([AIS anomaly detection](https://link.springer.com/chapter/10.1007/978-3-540-78297-1_11)).
The transferable kernel for dotf: a **`dotf doctor`/drift check** that treats the
lockfile-resolved state as "self" and flags "non-self" (a tool whose on-disk version
≠ recorded version, a missing binary, a stale symlink). This stays firmly inside
dotf's _additive, single-host_ scope — it _detects and reports_, it does not
auto-reconcile/prune (which the PRD explicitly defers). Don't over-reach into
auto-immunity; a read-only `doctor` is the right dose.

---

## Economic Analogies

### D1. Technical debt = principal + _compounding_ interest

**Transferability: MED (framing for prioritization + the "why now").** The debt
metaphor (Cunningham; Fowler's compounding refinement): _principal_ = cost to fix;
_interest_ = the recurring extra cost of not fixing; and the interest **compounds via
dependencies** — "if the code has many dependencies, the cost of rework can be
combinatorially higher"
([tech debt](https://en.wikipedia.org/wiki/Technical_debt),
[InfoQ](https://www.infoq.com/articles/technical-debt-tells-you/)). Empirically,
**architectural debt accrues interest 3–5× faster than code-level debt.**

**Mapping to dotf.** The 19 `custom-script` entries + ~13,211 LOC of
`bin/install-*.sh` are _architectural_ debt (the highest interest rate). The interest
is real and compounding: every new tool copies the path-of-least-resistance
`custom-script` pattern; every distro dotf adds multiplies the untested shell
surface; drift between manifest and scripts widens. The metaphor's _actionable_
lesson (SIG/Fowler): debt is only worth repaying where interest is high **and**
touched often — so prioritize migrating the `custom-script` tools that (a) are simple
release-binaries (low principal) and (b) change versions often (high interest):
atuin, lefthook, tea, coderabbit first; leave mise/rust (high principal, genuine
setup logic) as _accepted_ debt.

### D2. Incentive design / path of least resistance — bending the gradient

**Transferability: HIGH (the core behavioral mechanism).** The tech-debt literature's
sharpest point: organizations "take the path of least resistance and defer" because
the metaphor "focuses only on cost" and doesn't surface _what you give up_
([InfoQ](https://www.infoq.com/articles/technical-debt-tells-you/)). For a **solo
maintainer**, the path of least resistance is _always_ another `custom-script` — it's
one line vs. authoring an `asset` template + pinning a tag + computing a sha256.

**Mapping to dotf — the "boiling frog" fix.** You change behavior by changing the
_gradient_, not by willpower. Three gradient-benders, each borrowed above:

1. **Make native cheaper** than the escape hatch (ubi auto-detect, A1) — removes the
   incentive at the source.
2. **Make the escape hatch cost something visible** (ansible-lint-style CI rule, B2;
   status count, C2) — a small friction at commit time that stops the silent boil.
3. **Remove the recurring tax** that pushes toward unpinned scripts (Renovate
   auto-bump, A2) — so choosing the pinned native path isn't punished with chores.

Absent all three, the frog boils: `custom-script` will keep growing because it will
keep being the easiest choice, regardless of stated intent.

---

## Reproducibility / Content-Addressing Analogies

### E1. Lockfiles (Cargo.lock / go.sum) — the _right-sized_ reproducibility for dotf

**Transferability: HIGH — and dotf gets it nearly free because it's Go.** A lockfile
records "which packages, at what versions, from where, with what checksums" so builds
are reproducible and tamper-evident
([lockfile design](https://nesbitt.io/2026/01/17/lockfile-format-design-and-tradeoffs.html)).
The **Go model is the exact template**: `go.mod` (intent) + `go.sum` (integrity),
where _deterministic resolution_ (minimal version selection) means `go.sum` is _purely
integrity_, not resolution-pinning. Cargo's lock is a TOML file recording exact
versions of all deps.

**Mapping to dotf.** dotf's `packages.toml` is the `go.mod` (intent); it has **no
`go.sum` equivalent** — no record of _what was actually resolved and installed_. Add a
**`dotf.lock`** (TOML) recording, per package: resolved tag, the auto-detected asset
filename, and the sha256 that was actually fetched. Payoffs: (1) `github-binary`
auto-detection becomes _reproducible_ — the heuristic runs once, the result is
pinned, re-applies are deterministic (this is the answer to ubi's "detection may
change" caveat, A1); (2) drift detection (C1/C3) has a set-point to compare against;
(3) supply-chain integrity for `script-url`/vendor installers. Keep the _intrinsic
data_ minimal (name, version, source, checksum) — the literature warns lockfiles that
carry _extrinsic_ metadata drift toward noisy SBOMs
([lockfile design](https://nesbitt.io/2026/01/17/lockfile-format-design-and-tradeoffs.html)).

### E2. chezmoi `run_onchange_` — SHA256-in-a-comment re-run trigger

**Transferability: HIGH (near-literal, solves the `custom-script` re-run problem).**
chezmoi content-addresses scripts by SHA256: `run_once_` runs once per unique
_content_ hash; `run_onchange_` re-runs whenever _that script's rendered content_
changes ([chezmoi scripts](https://www.chezmoi.io/user-guide/use-scripts-to-perform-actions/)).
The killer pattern: to re-run a script when a _separate_ file changes, **embed that
file's hash in a comment** so the script's own hash shifts —
`# hash: {{ include "dot_Brewfile.d/core" | sha256sum }}`. For git-repo externals,
embed `git rev-parse HEAD`. State lives in a `scriptState` bucket keyed by content
hash; chezmoi's doctrine: _"Scripts break chezmoi's declarative approach and should be
used sparingly. All scripts should be idempotent."_

**Mapping to dotf.** dotf re-runs `custom-script` on _every_ `apply` and leans on the
script self-skipping (a fragile contract). Borrow chezmoi's content-addressing:
**record the SHA256 of each `bin/install-*.sh` (and any referenced file) in
`dotf.lock`; skip re-execution unless the hash changed.** This gives dotf's escape
hatch _real_ idempotency at the engine level (not delegated to script discipline) and
a `run_onchange` semantic for the genuinely-must-stay-shell tools — precisely the
`creates:`/`changed_when` idea from Ansible (B2), implemented via content hash.

### E3. Nix content-addressed store — the ceiling to _not_ import

**Transferability: LOW (deliberately — a boundary marker).** Nix generalizes the
checksum idea to _every input in the build closure_ (compiler, linker, kernel
headers), pinned by input hash in `/nix/store` + `flake.lock`
([nixpkgs Rust](https://ryantm.github.io/nixpkgs/languages-frameworks/rust/)). This is
the fully-hermetic ideal — and it's the **anti-pattern for dotf's scope**: the PRD
explicitly rejects Nix-level complexity. The transferable _insight_ is the spectrum:
lockfile (E1) → deterministic resolution (go.sum) → full content-addressing (Nix).
dotf should sit at the **lockfile rung** (integrity + a resolved-version record) and
_not_ climb toward hermetic closure. Naming the ceiling prevents scope-creep toward
"reproducibility theater."

---

## Cross-Domain Patterns

Five patterns recur across all five domains — these are the _invariants_, the
strongest signal for what dotf should adopt:

1. **Two-tier: declarative default + governed escape hatch.** Homebrew (header +
   `def install`), Terraform (resources + provisioners), Bazel (rules + genrule),
   Ansible (modules + shell), chezmoi (managed files + scripts), mise (backends +
   `http`). _No mature tool deletes the escape hatch; every one fences it._ → dotf
   keeps `custom-script`, but fences it (lint + count + doctrine).

2. **Auto-derive the boring 80% so the declarative path is the cheap path.** ubi/eget
   asset detection, aqua registry, `brew bump-formula-pr`, mise auto-backend-pick.
   _The declarative path must be cheaper than the escape hatch or the escape hatch
   wins._ → dotf's #1 fix (ubi-style auto-detect).

3. **A ranked ladder of backends/alternatives to exhaust before the escape hatch.**
   mise's preference order, Terraform's decision framework, Bazel's
   `run_binary`→custom-rule escalation, Ansible's "check for a module first." → dotf
   publishes an explicit backend-selection doctrine.

4. **Content-hash / checksum as the unit of idempotency & integrity.** chezmoi
   SHA256, Cargo.lock/go.sum, Nix input hashes, ubi/eget/Homebrew sha256 sidecars,
   aqua checksum auto-update. → dotf adds `dotf.lock` + content-addressed script
   re-run.

5. **Governance = make the wrong choice _visible and slightly costly_, not
   impossible.** ansible-lint rules, Terraform's "last resort" + opaque-plan penalty,
   Bazel's sandbox that _detects_ escapes, tech-debt's "surface what you give up." →
   dotf's CI lint + status count.

---

## Transferable Solutions — mapped to dotf's 7 backends

| #   | Borrowed mechanism                                                 | Source domain                               | dotf change (backend/field)                                                                                                                                                                | Transferability | Effort   | Value                    |
| --- | ------------------------------------------------------------------ | ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------- | -------- | ------------------------ |
| S1  | **ubi/eget auto-asset-detection** (OS→arch→libc→tie-break→extract) | ubi, eget, mise `github`                    | New `github-binary` backend _or_ auto-mode of `github-release`: make `asset` optional; add filter pipeline + `exe` field (repo≠binary) + archive introspection with "starts-with" fallback | HIGH            | Med-High | **Very High (keystone)** |
| S2  | **Renovate version+checksum auto-bump**                            | aqua, Homebrew `bump-formula-pr`            | Add Renovate regex-manager comments to `github-release` `tag`/`sha256` lines; ships `renovate.json`                                                                                        | HIGH            | Low      | High                     |
| S3  | **Explicit backend-preference doctrine**                           | mise decision order, TF/Ansible ladder      | Doc + `packages.toml` header ranking: `pkg`→`github-binary`→`toolchain`→`apt-repo`/`aur`→`script-url`→`custom-script`                                                                      | HIGH            | Low      | High                     |
| S4  | **Lint that blocks _new_ `custom-script`** (ratchet)               | ansible-lint `command-instead-of-module`    | CI check: fail if `custom-script` count rises above a monotonically-decreasing baseline                                                                                                    | HIGH            | Low      | High                     |
| S5  | **`dotf.lock` (resolved version + sha256)**                        | go.sum, Cargo.lock                          | New lockfile written by `apply`; records resolved tag/asset/sha256 per package                                                                                                             | HIGH            | Med      | High                     |
| S6  | **Content-addressed script re-run** (`run_onchange`)               | chezmoi SHA256-in-comment                   | Record `custom-script`/`script-url` file hash in `dotf.lock`; skip unless changed                                                                                                          | HIGH            | Med      | Med-High                 |
| S7  | **`post_install` declared hook** (bounded middle rung)             | Homebrew `post_install`, Bazel `run_binary` | Add `Package.post_install` (single command/step) so binary-tools needing one setup step leave `custom-script`                                                                              | HIGH            | Med      | Med-High                 |
| S8  | **`creates`/`removes` idempotency guard**                          | Ansible `creates:`                          | Add `Package.creates` (path); replace fragile `LookPath(name)` in `Installed()`                                                                                                            | HIGH            | Low      | Med                      |
| S9  | **`dotf update` + `--only`/`--skip` + `--list-steps`**             | topgrade                                    | New `update` verb (re-resolve floating tools/bump); filter flags on `apply`/`install`                                                                                                      | HIGH            | Med      | High                     |
| S10 | **`dotf doctor` drift report (read-only)**                         | AIS self/non-self, homeostasis              | Compare `dotf.lock` "self" vs on-disk; report only (no prune — respects PRD)                                                                                                               | MED             | Med      | Med                      |
| S11 | **`livecheck`-style version source**                               | Homebrew `livecheck`                        | Optional `Package.version_source` so `update` knows where "latest" lives                                                                                                                   | MED             | Low      | Med                      |

**Per-backend impact summary:**

- **`github-release`** → gains S1 (auto-detect), S2 (Renovate), S5 (lock). Biggest
  winner; absorbs the ~6–8 release-binary `custom-script` tools (atuin, lefthook,
  tea, coderabbit, turso, parts of `*-tools`).
- **`custom-script`** → shrinks via S1/S7; hardened by S4 (ratchet), S6 (content
  re-run), S8 (`creates`). Survives for genuine setup-steps (mise, rust,
  gnu-utilities, tpm-as-clone, tmux-sessionizer).
- **`script-url`** → gains S5/S6 (lock + hash) for supply-chain integrity on vendor
  installers (claude, codex, opencode, fabric `curl|sh`).
- **`toolchain`** → currently 0 uses; S3 doctrine makes it the correct home for
  `cargo`/`npm`-delivered tools, pulling some out of `custom-script`.
- **`pkg` / `aur` / `apt-repo`** → unaffected mechanically; S3 ranks `pkg` top (free
  security updates), S2's Renovate idea can extend to `apt-repo` codenames.

---

## Key Insights

1. **dotf's gradient is inverted vs. every mature analogue.** In Homebrew/aqua/mise,
   the declarative path is the _cheap_ path and the escape hatch is effortful. In dotf
   it's reversed: `github-release` requires hand-authored asset+tag+sha256 while
   `custom-script` is a one-liner. **Fix the gradient (make native cheaper) and the
   migration largely happens by itself** — this is why S1 is the keystone.

2. **The single highest-leverage borrow is ubi/eget-style auto-asset-detection**
   (S1). It's a _rules-level_ Meadows intervention that unlocks the _most_ migrations
   per unit effort, and mise's deprecation of its `ubi` backend in favor of a native
   `github` backend proves owning it in-tree is the right architecture.

3. **You cannot retire the escape hatch — you can only fence it.** All five domains
   keep theirs forever. dotf's realistic goal is _shrink + govern_, not _eliminate_.
   The PRD's "≥80% LOC retired" is achievable for release-binaries but mise/rust/
   gnu-utilities are _accepted_ architectural debt (high principal, real logic).

4. **Governance is three gradient-benders, not willpower:** cheaper native (S1),
   costly-visible escape hatch (S4 lint + status count), removed maintenance tax (S2
   Renovate). Ship all three or the frog boils.

5. **dotf gets reproducibility almost free because it's Go.** The go.mod/go.sum split
   is the exact right-sized model: `packages.toml` = intent, `dotf.lock` = integrity.
   Sit at the lockfile rung; explicitly do _not_ climb toward Nix hermeticity.

6. **Content-hashing is the universal idempotency primitive.** chezmoi's
   SHA256-in-comment (S6) + Ansible's `creates` (S8) fix dotf's weakest spot — the
   `LookPath(name)` PATH-probe `Installed()` that has no version awareness and no real
   drift detection.

---

## Evidence Quality

- **HIGH confidence, primary/official sources:** ubi (GitHub README + docs.rs), eget
  (DOCS.md), aqua (official docs + repo), mise (official backend docs), Homebrew
  (Formula Cookbook), Terraform (HashiCorp docs), Bazel (build encyclopedia),
  chezmoi (official user guide), Cargo Book, topgrade (repo). All are the tools'
  own documentation — authoritative and current (2024–2026 dated sources
  throughout).
- **HIGH confidence, codebase-grounded:** dotf claims verified directly against
  `internal/backend/github_release.go`, `custom_script.go`, and
  `internal/manifest/types.go` (read this session) — the "asset+tag+sha256 required,"
  "`LookPath(name)` Installed()," and "FORCE_INSTALL unset, relies on self-skip"
  findings are from source, not the brief.
- **MED confidence:** the technical-debt interest-rate figures (3–5× architectural,
  20–40% of estate) come from secondary summaries of empirical studies (SIG,
  McKinsey via blog) — directionally solid, exact numbers not independently verified.
- **LOW-MED confidence / analogical-only:** immune-system (C3), keystone/Meadows
  (C2), homeostasis (C1) are _framing_ borrows, not engineering evidence — used to
  prioritize and name boundaries, not to justify specific code. Rated accordingly.

---

## Contradictions & Uncertainties

1. **Auto-detection vs. reproducibility (real tension).** ubi's maintainer warns the
   detection heuristic "is not part of the API and may change" — pure auto-detect is
   _non-deterministic across ubi versions_. This contradicts dotf's pinned-sha256
   ethos. **Resolution:** S1 (auto-detect) _feeds_ S5 (lockfile) — detect once, pin
   the result. Auto-detection for _onboarding a tool_, lockfile for _reproducing_ it.
   Unverified: whether dotf's maintainer wants the added lockfile surface.

2. **mise deprecated `ubi` — is auto-detection itself the future, or just _external_
   ubi?** mise replaced `ubi:` with a _native_ `github:` backend (same capability,
   in-tree). This supports _building_ auto-detect natively (not shelling to ubi/eget)
   but leaves open whether dotf should vendor a library or reimplement the pipeline.
   Uncertain; needs a code-spike estimate (out of Analogist scope).

3. **"Retire ≥80% of LOC" vs. "some scripts encode hard-won logic" (PRD's own
   rewrite-bias warning).** The analogies _support both_: Terraform/Bazel keep the
   escape hatch for a reason. mise/rust/gnu-utilities are likely _correct_ to remain
   `custom-script`. So the 80% target should be measured against
   _release-binary/vendor-installer_ tools, not the genuine setup-steps — the metric
   may need re-scoping. Contradiction left un-smoothed.

4. **Lockfile vs. additive-only philosophy.** A lockfile implies a notion of "current
   resolved state," which edges toward the reconciliation/prune the PRD defers. **Not
   a true contradiction** — a lockfile can be read-only/report-only (S10 doctor) and
   still additive — but the boundary must be drawn deliberately or scope-creep
   follows.

5. **Not investigated (gaps):** whether Renovate supports Forgejo (dotf's origin is
   Forgejo per memory) as smoothly as GitHub for the regex-manager auto-bump (S2);
   the exact effort of a Go-native asset-detection pipeline vs. its edge-case tail
   (musl, `.exe`, multi-artifact); how dotf's `huh` TUI would surface a `doctor`/drift
   report.

---

## Search Queries Executed

1. `ubi universal binary installer GitHub release asset auto-detect OS arch libc`
2. `eget vs ubi GitHub release binary installer asset auto-detection matching`
3. `aqua declarative CLI version manager registry aqua.yaml Renovate version bump standard registry`
4. `mise asdf backends aqua ubi cargo npm go pipx which backend to choose tool install`
5. `Terraform provisioner local-exec "last resort" declarative escape hatch best practices`
6. `Ansible command shell module vs native modules idempotency changed_when best practice avoid`
7. `idempotency convergence desired state configuration management homeostasis self-healing drift correction`
8. `technical debt interest metaphor compounding shell scripts maintenance cost economics incentive path of least resistance`
9. `chezmoi run_onchange run_once scripts hash content addressing externals lockfile dotfiles`
10. `lockfile Cargo.lock package-lock reproducible builds pinning checksum content-addressed store Nix transferable concept`
11. `Homebrew formula DSL url sha256 declarative install block post_install custom install ruby escape hatch`
12. `Donella Meadows leverage points systems thinking keystone intervention highest leverage place to intervene`
13. `topgrade update all tools package managers unified upgrade command step config skip`
14. `Bazel genrule escape hatch hermeticity non-hermetic last resort discouraged custom rule vs genrule`
15. `immune system analogy software self non-self anomaly detection configuration drift keystone species ecosystem software architecture`

(Plus codebase reads: `internal/backend/github_release.go`, `custom_script.go`,
`internal/manifest/types.go`.)
