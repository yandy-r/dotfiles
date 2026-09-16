# The Contrarian — dotf CLI Enhancements

**Persona:** The Contrarian (Asymmetric Research Squad)
**Date:** 2026-07-04
**Mandate:** Challenge the premise. Argue AGAINST retiring `custom-script`. Find the real bugs.

---

## Executive Summary

The headline plan — "rewrite 19 `custom-script` entries into native Go backends to hit
the PRD's ≥80%-LOC-retirement metric" — is, for a solo maintainer, **the lowest-value,
highest-risk item on the roadmap**, and the research brief itself already smuggles in the
counter-evidence: the single `script-url` entry that _has_ been "natively" ported is
**already broken in the default install path today**. I verified this live:

- The manifest pins `starship` at `sha256 = e9f0bb84…5534c9`
  (`manifest/packages.toml:225`). The live `https://starship.rs/install.sh` now hashes to
  **`52c64f14…cc8fbb`** (verified via `curl … | sha256sum`, 2026-07-04). The pin is stale.
  Because `starship` is in the **default `base` intent** (`packages.toml:35`,
  `default = true`), a fresh `dotf apply` **fails SHA256 verification on starship right
  now** (`script_url.go:107`, `verifySHA256`), records a non-fatal failure, and silently
  leaves starship uninstalled while exiting 1.

That is the whole thesis in one data point: the moment you move a rotating vendor
installer behind a hard checksum, you convert a self-healing shell escape hatch into a
**time-bomb that the maintainer must chase forever**. Porting 19 more scripts into
`github-release`/`script-url`/`toolchain` backends multiplies this maintenance surface,
not reduces it.

Separately — and independent of the migration debate — the current code has a **concrete,
shipping correctness bug**: `custom-script`'s `Installed()` probe (`custom_script.go:49–55`)
is a bare `LookPath(pkg.Name)`, and the package _name_ is a logical group label, not a
binary. **8 of 19 custom-script tools (42%) are permanently mis-reported as "not
installed"** by `dotf status`/`dotf diff` on a fully-provisioned host, with 2 more at
edge-case risk. That is a real, verifiable defect that deserves fixing _before_ any grand
migration.

Confidence: **High** on the code findings (verified against source + live network); **Medium-High**
on the strategic "don't do the big migration" argument (judgment call, but well-supported).

---

## Disconfirming Evidence (against the migration premise)

### 1. The one existing native port is already broken → the premise self-refutes

The strongest disconfirming evidence isn't external, it's in the repo. The project's own
"we replaced a shell-out with a checksummed native backend" showcase (`starship` via
`script-url`) is **stale and failing on the default path** (see Exec Summary). This is
exactly the failure mode the shell scripts were designed to _avoid_: `bin/install-mise.sh`
runs `curl -fsSL https://mise.run | sh` (`install-mise.sh:176`) with **no pin**, so it
never breaks when upstream rotates the script — it just works. The "hardened" `script-url`
backend traded that resilience for a checksum the maintainer must now babysit.

The tension is well-documented externally. Even Chef — who built one of the most carefully
engineered `install.sh` pipelines — conceded there is "one unique risk … the case where
the servers themselves get hacked, and where install.sh is tampered with," and the broader
consensus is that **pinning a mutable, always-latest installer by hash is architecturally
self-defeating**: "a mutable, always-latest script is convenient but un-pinnable, while a
pinned script is verifiable but static."
([Chef Blog](https://www.chef.io/blog/5-ways-to-deal-with-the-install-sh-curl-pipe-bash-problem),
[dev.to](https://dev.to/operous/how-to-build-a-trustworthy-curl-pipe-bash-workflow-4bb),
[lesinskis.com](https://www.lesinskis.com/dont-pipe-curl-into-bash.html))

### 2. Rewriting working, hardened code is the classic negative-value move

Joel Spolsky's "Things You Should Never Do" names rewriting working code "the single worst
strategic mistake that any software company can make," because "old code is hairy for a
reason. Each one of those hairs represents a weird bug that was detected, fixed, and
vetted in real life."
([Joel on Software](https://www.joelonsoftware.com/2000/04/06/things-you-should-never-do-part-i/))
The 13,211 LOC across `bin/install-*.sh` encode exactly that kind of hairs:

- `install-mise.sh` alone tries **Homebrew → yay/paru (AUR) → curl installer** in order,
  with a full `/etc/os-release` + `ID_LIKE` fallback matrix (`install-mise.sh:79–216`).
  The `pkg`/`aur`/`script-url` backends each handle _one_ of those paths; reproducing the
  cascade natively means new Go orchestration logic that doesn't exist yet.
- `install-lua.sh` probes `lua`, `lua5`, **and** `luarocks` (three different binaries);
  `install-fabric.sh` handles both `fabric` and `fabric-ai` names and go/brew/AUR install
  paths. These are per-tool edge cases that a generic backend cannot express without new
  schema fields.

The modern middle ground (from the same debate) is **incremental replacement of the
_clearly_ mechanical cases only** — not a wholesale port to chase a LOC metric.

### 3. The "shrinking escape hatch" was mis-framed; a permanent escape hatch is correct design

The brief calls `custom-script` "the shrinking escape hatch" that has embarrassingly
become the most-used backend (19/37). The contrarian read: **that's not a failure, that's
the design finding its natural equilibrium.** chezmoi — the category leader — reached the
identical conclusion and made it _permanent_: "chezmoi has no concept of brew or packages.
chezmoi only knows about scripts," and package install is delegated to user `run_onchange_`
scripts by explicit design.
([chezmoi discussion #4650](https://github.com/twpayne/chezmoi/discussions/4650),
[chezmoi design FAQ](https://www.chezmoi.io/user-guide/frequently-asked-questions/design/))
A well-scoped shell-out backend is the pragmatic terminus every dotfiles tool converges on;
declaring it "should shrink to zero" is aspirational purity, not engineering.

### 4. Declarative purity is over-rated for a single-host solo setup

The Nix/home-manager cohort is the reductio: maximal declarative reproducibility at the
cost of a learning cliff that critics say "outweigh[s] the benefits for many users,
especially for common use cases like managing dotfiles," where "GNU Stow, chezmoi, or even
a plain Git repository … accomplish 90% of what most people need with a fraction of the
complexity" (search synthesis; no single citable source surfaced). chezmoi's own docs warn
that its Go-template system carries "a learning curve for templates" and even chezmoi
enthusiasts "often use only a subset of its features and keep scripts to a minimum."
([chezmoi templating](https://www.chezmoi.io/user-guide/templating/),
[Atlassian](https://www.atlassian.com/blog/jira-service-management/configuration-management-for-dev-environments))
dotf is a **single-host, additive, solo-maintainer** tool. Every increment of declarative
machinery it adds to avoid a shell-out is complexity spent against a benefit it structurally
cannot cash in.

---

## Expert Critiques (of declarative dotfile tooling)

- **Spolsky, "Things You Should Never Do":** never rewrite working code from scratch;
  refactor. Old code embeds vetted bug fixes.
  ([joelonsoftware.com](https://www.joelonsoftware.com/2000/04/06/things-you-should-never-do-part-i/))
- **Chef Blog, "5 Ways to Deal With the install.sh Curl Pipe Bash problem":** even a
  best-in-class checksummed installer pipeline retains an irreducible tamper risk when the
  bootstrap script itself is mutable.
  ([chef.io](https://www.chef.io/blog/5-ways-to-deal-with-the-install-sh-curl-pipe-bash-problem))
- **Lesinskis, "Another reason why piping curl into bash is a security risk":** HTTPS ≠
  integrity; server-side user-agent sniffing can serve different bytes to a browser vs a
  terminal, so "inspect it in your browser first" is not bulletproof.
  ([lesinskis.com](https://www.lesinskis.com/dont-pipe-curl-into-bash.html))
- **chezmoi maintainer (twpayne):** "scripts break chezmoi's declarative approach and
  should be used sparingly" — yet chezmoi _ships_ the script escape hatch anyway and
  refuses to model packages, because reconciling packages against desired state is out of
  scope. Deletion/purge feature requests were closed "won't implement."
  ([chezmoi #324](https://github.com/twpayne/chezmoi/issues/324),
  [chezmoi #5089](https://github.com/twpayne/chezmoi/issues/5089))
- **aqua vs mise pinning experience:** the "declarative + checksum lockfile" ideal is
  real but carries ongoing operational cost — mise's aqua backend still hits `api.github.com`
  under `--locked`, strips `v` prefixes and 404s, and users report "sometimes commands will
  generate a checksum but oftentimes they do not," making a reproducible upgrade flow hard
  to document.
  ([mise #8677](https://github.com/jdx/mise/discussions/8677),
  [mise #7210](https://github.com/jdx/mise/discussions/7210))
  This is precisely the maintenance tax dotf would import by pinning 19 tools.

---

## Documented Failures (dotfile tools that broke things)

- **chezmoi's first `apply` is destructive by design.** On a fresh machine, `init --apply`
  overwrites hand-edited `$HOME` dotfiles that chezmoi never "wrote," so the "prompt before
  clobber" guardrail does _not_ fire. Universal community advice: always `chezmoi diff` /
  `apply --dry-run` first. dotf's own symlink engine must guarantee the equivalent — and
  it partially does (`--backup`, `Validate`, `BrokenLinkError`), but note `apply`'s symlink
  phase only forces `Backup=true` when `--linux-desktop` is set (`applyphases.go:205–213`);
  a plain `dotf apply` can still replace a real file with a symlink without a backup unless
  `--force`/validation catches it.
  ([chezmoi apply reference](https://www.chezmoi.io/reference/commands/apply/),
  [DEV writeup](https://dev.to/recca0120/chezmoi-one-dotfiles-repo-across-macos-linux-and-windows-2o3))
- **chezmoi age-key footgun:** "lose the key, lose every encrypted file forever" and
  "key.txt itself must never land in the repo." Directly relevant: the PRD _defers_ secrets
  orchestration — good. Anyone lobbying to add it is signing up for this footgun.
- **GNU Stow conflicts:** Stow's most common complaint is that it **refuses** to link over a
  pre-existing real file (a conflict), and the "fix," `--adopt`, _mutates your repo_ — the
  manual warns "this behaviour is specifically intended to alter the contents of your stow
  directory." Lesson for dotf: never silently `--adopt`-style absorb; refuse or back up.
  ([GNU Stow manual](https://www.gnu.org/software/stow/manual/stow.html),
  [System Crafters](https://systemcrafters.net/managing-your-dotfiles/using-gnu-stow/))
- **curl|bash credential exfiltration scenario:** a compromised installer that "still
  installs the tool correctly" while base64-exfiltrating `AWS_/SECRET_/TOKEN/API_KEY` env
  vars is the canonical supply-chain attack. dotf's _unpinned_ vendor `curl|sh` calls
  buried inside custom-scripts (e.g. `install-mise.sh:176`) inherit this risk — but so
  would any native `script-url` port that pins a hash the maintainer then can't keep
  current (see starship). There is no free lunch here.
  ([reflexframework.com](https://reflexframework.com/battlecards/curl-bash1))

---

## Correctness / Design Flaws in Current dotf Code

> Ranked by severity. file:line cited. These stand **independent** of the migration debate.

### FLAW 1 — `custom-script` `Installed()` is a bare `LookPath(pkg.Name)` → 42% of entries permanently mis-reported (CONFIRMED)

`internal/backend/custom_script.go:49–55`:

```go
func (b *customScriptBackend) Installed(pkg manifest.Package, _ platform.Info) (bool, error) {
    _, err := b.r.LookPath(pkg.Name)   // pkg.Name is a GROUP label, not a binary
    ...
}
```

The manifest `name` is a logical grouping label, but the probe assumes it is the installed
binary. Verified mismatches (binary the script actually installs ≠ `pkg.Name`):

| manifest `name`    | real binary(ies) installed                                             | probe result on a provisioned host    |
| ------------------ | ---------------------------------------------------------------------- | ------------------------------------- |
| `rust`             | `cargo`, `rustc`, `rustup` (`install-rust.sh:command -v cargo/rustup`) | **false negative — always "missing"** |
| `azure-cli`        | `az` (`install-azure-cli.sh:75 command_exists az`)                     | **false negative**                    |
| `aws-tools`        | `aws`, `sam`, `ecs-cli`                                                | **false negative**                    |
| `hashicorp-tools`  | `terraform`, `vault`, `packer`, …                                      | **false negative**                    |
| `container-tools`  | `podman`/`docker`/`buildah` (no `container-tools` bin)                 | **false negative**                    |
| `tpm`              | _no binary_ — git clone to `~/.tmux/plugins/tpm` (`install-tpm.sh`)    | **false negative**                    |
| `tmux-sessionizer` | `tms` (`install-tmux-sessionizer.sh:32`)                               | **false negative**                    |
| `gnu-utilities`    | `gcoreutils` etc. (no `gnu-utilities` bin)                             | **false negative**                    |
| `lua`              | may be only `lua5.4`/`luajit` (`install-lua.sh` probes `lua`/`lua5`)   | edge false negative                   |
| `fabric`           | `fabric` **or** `fabric-ai`; collides with Python pyinvoke `fabric`    | false negative **or false positive**  |

**Impact:** `dotf status` (`status.go:101`) and `dotf diff --only-missing`
(`status.go:82–124`, `driftReport`) call `engine.Installed()` and will report **8/19
tools as `installed=false` / `drift=declared-not-installed` forever**, even on a fully
converged machine. The `fabric` case can go the other way: a user with Python's `fabric`
(pyinvoke) on PATH makes dotf report the AI `fabric` as installed when it isn't (false
positive → skipped in status). This is a straightforward correctness bug in the read-only
commands whose entire job is to report truth.

Note the same `LookPath(pkg.Name)` pattern also lives in `github_release.go:81–87`,
`script_url.go:55–66`, and `toolchain.go:74–82` — but there the convention that
`pkg.Name == binary` is enforceable (and `toolchain.go:72` documents it). Only
`custom-script` breaks the convention systemically. **Fix:** add an optional
`Package.Binary` / `Package.Provides []string` field so state detection can probe the
real artifact (mirrors what the scripts already do internally).

### FLAW 2 — `script-url` sha256 pin is stale on the default path → starship broken today (CONFIRMED, re-verify on maintainer network)

`manifest/packages.toml:221–225` pins `starship` install.sh at `e9f0bb84…5534c9`; live
content hashes to `52c64f14…cc8fbb` (2026-07-04). `script_url.go:107` (`verifySHA256`)
therefore fails, `Install` returns an error, `applyPackages` records it non-fatally
(`applyphases.go:135–143`), and `dotf apply` exits 1 with starship uninstalled — on the
**default `base` intent**. This is the durability problem the brief hypothesized, now
demonstrated. **Caveat:** the live hash was fetched from a sandboxed network; re-run
`curl -fsSL https://starship.rs/install.sh | sha256sum` to confirm before acting. **Fix:**
either (a) pin an _immutable versioned_ URL (a tagged release asset, not `/install.sh`),
or (b) accept that a rotating installer cannot be hash-pinned and keep it as a
tolerant shell-out. Do not add 18 more of these.

### FLAW 3 — `apply`/`install` never consult `Installed()` → no converged fast-path; non-deterministic version drift (CONFIRMED)

`installPackage` (`install.go:93–112`) gates only on `Applicable`, then calls
`engine.Install()` **unconditionally**. There is no `if Installed { skip }`. Consequences:

- Every `dotf apply` re-runs `pacman -S --needed …` for all 14 pkg entries **and** re-execs
  all 19 `bin/install-*.sh` via `env NON_INTERACTIVE=true bash` (`custom_script.go:42`),
  relying entirely on each script's internal self-skip for idempotency. Fine for pacman;
  wasteful (fork + re-detect) for scripts.
- For rolling tools, `custom_script.go`'s doc comment (lines 30–35) advertises that
  mise/rust/atuin do a **best-effort self-update** every run. That means **`dotf apply` is
  non-deterministic**: a rolling tool can silently change version on any apply. That
  directly conflicts with the reproducibility ambitions elsewhere in the roadmap. A tool
  that pitches "checksum-verified, pinned" provisioning but self-updates three tools on
  every headless run is sending mixed messages.

**Fix:** consult `Installed()` in the install/apply loop for a real skip path; make
self-update opt-in (`--upgrade`), not a silent every-run side effect.

### FLAW 4 — `apt-repo` hard-codes the `noble` codename → breaks on every non-24.04 Ubuntu/Debian (CONFIRMED)

`manifest/packages.toml:218`: `repo_line = "… https://download.docker.com/linux/ubuntu
noble stable"`. `apt_repo.go` writes this verbatim to `/etc/apt/sources.list.d/docker.list`
(`apt_repo.go:90–98`) with no codename templating. On Ubuntu 22.04 (`jammy`), 25.04, or any
Debian, `apt-get update` (`apt_repo.go:100`) points at `dists/noble/` — wrong suite, and
Docker's Debian repo lives at a different base URL entirely. The manifest comment
(`packages.toml:210–213`) even admits "Edit the codename to match the target Ubuntu
release" — i.e., **the manifest is not portable across the distros dotf claims to support**.
**Fix:** a `{{.Codename}}` template (read from `/etc/os-release VERSION_CODENAME`) or a
per-distro `apt_repo` variant. This is a prerequisite the brief lists as MISSING for a
clean migration, confirmed real.

### FLAW 5 — No `rpm-repo` backend → Fedora silently degrades to script/custom-script

Confirmed by absence (`register.go:12–20` registers 7 backends; no rpm-repo). `docker-ce`
(`apt-repo`) is apt-gated (`apt_repo.go:47–49`), so on Fedora it is skipped as
`not-applicable`; azure/aws fall to custom-scripts. The 7-backend taxonomy is **Debian/Arch-
centric**; Fedora/openSUSE are second-class. Any migration plan that ignores this just
moves the Fedora gap around.

### FLAW 6 — Documentation drift: the "~45-line shim" is 307 lines

`CLAUDE.md` and the objective repeatedly call `bootstrap.sh` a "~45-line POSIX shim";
`wc -l bootstrap.sh` = **307**. (The objective's grounding brief does say "currently ~307
lines," so the drift is CLAUDE.md vs reality.) 307 lines of forge-URL parsing, GitHub-API
asset-ID scraping via `sed` (`bootstrap.sh:234–244`), and dual `/releases/latest/download`
vs `/releases/download/latest` fallback is a lot of un-tested shell in the _entry point_ —
the exact "boring shell that should be minimal" the Go rewrite was meant to eliminate. Not
a bug, but the shim is now its own maintenance surface, and calling it "45 lines" hides
that.

### FLAW 7 — Unpinned vendor `curl|sh` inside custom-scripts (supply chain)

`install-mise.sh:176` (`curl -fsSL https://mise.run | sh`) and peers execute remote code
with no integrity check. This is a _real_ risk — but note it is **not fixed by the
migration** unless the native port pins an immutable artifact, which (per FLAW 2) the
maintainer then can't keep current. The honest options are: pin _versioned_ release assets
(github-release backend, which does verify — `github_release.go:55`), or accept the risk
for genuinely rotating installers. "Port to script-url" is a false fix.

---

## Questionable Assumptions

1. **"≥80% LOC retirement is a success metric."** LOC deleted is a _vanity_ metric. Deleting
   13k lines of vetted, self-skipping shell to add Go backends + a checksum-maintenance
   treadmill can be **negative** engineering value (FLAW 2). Success should be measured in
   _correctness of state detection_ and _fewer 2am breakages_, not lines removed.
2. **"Native Go backend > working shell script."** Rewrite bias. The `install-mise.sh`
   3-way cascade and `install-lua.sh` multi-binary probing are cheaper to keep than to
   re-encode (Spolsky).
3. **"custom-script becoming the biggest backend is embarrassing."** It's the industry-
   standard equilibrium (chezmoi models packages the same way — as opaque scripts).
4. **"Checksum-pinning = hardened/secure."** Only for _immutable_ artifacts. Pinning a
   floating vendor installer is a self-inflicted breakage (proven: starship).
5. **"apply is idempotent."** It is _convergent-by-luck_ (relies on each tool's own
   idempotency; never checks `Installed()` — FLAW 3) and _non-deterministic_ for rolling
   tools. Idempotent ≠ deterministic.
6. **"Additive-only is a limitation to eventually fix with `--prune`."** chezmoi
   deliberately never built reconciliation and closed the requests "won't fix." Additive-
   only is a defensible _feature_ for a single-host tool, not a debt.

---

## Features That Are Traps (steelman the "don't build it" side)

| Requested feature                               | Why it's a trap for THIS tool                                                                                                                                                                                                                                                                                                                                                       |
| ----------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Reconciliation / `--prune` / uninstall**      | chezmoi refused this on purpose. To prune you need a durable model of "what dotf installed," i.e. a state DB — massive scope creep, and the failure mode (auto-removing a package a user wanted) is destructive. The `custom-script` group names make "what did this install?" ambiguous (FLAW 1). **Don't build.** ([chezmoi #324](https://github.com/twpayne/chezmoi/issues/324)) |
| **Templating (Go text/template like chezmoi)**  | Imports the exact learning-curve/complexity chezmoi users route around, for a single-host setup that rarely needs per-host divergence. The TOML manifest + `os = "macos"` faceting already covers the 90% case. **Don't build.** ([chezmoi templating](https://www.chezmoi.io/user-guide/templating/))                                                                              |
| **Secrets orchestration (1Password/Bitwarden)** | The age-key footgun ("lose the key, lose everything") + "never commit key.txt." High-blast-radius, security-critical, and the PRD already scoped it OUT. **Keep out.**                                                                                                                                                                                                              |
| **Lockfile / pinned versions for all tools**    | The aqua/mise pinning experience shows this is a _continuous_ maintenance job (stale pins, API calls under `--locked`, `v`-prefix 404s). For a solo maintainer with rolling tools, a lockfile you don't keep current is worse than none. **Only pin immutable release assets, selectively.** ([mise #8677](https://github.com/jdx/mise/discussions/8677))                           |
| **Binary self-update (`dotf upgrade`)**         | The bootstrap shim already fetches `releases/latest`. A self-updater adds an update channel, rollback semantics, and a signature story for marginal benefit over re-running bootstrap. **Low priority; not a trap but over-valued.**                                                                                                                                                |
| **Fleet / multi-host**                          | Explicitly out of scope; this is where dotfiles tools become config-management systems and drown. **Keep out.**                                                                                                                                                                                                                                                                     |

**Features that are NOT traps (do these instead):** fix `Installed()` state detection
(FLAW 1); make `apply` consult `Installed()` for a real skip path (FLAW 3); template the
apt codename (FLAW 4); un-pin or version-pin starship (FLAW 2). These are boring, high-value
_fixes_, not features — and they're what a staff engineer would demand before any migration.

---

## Conflicts of Interest / Incentives

- **The PRD authored the ≥80% metric, so the roadmap is incentivized to chase it** even
  where deletion is net-negative. A metric measuring the wrong thing pulls effort toward
  vanity work (porting scripts) and away from the unglamorous correctness fixes above.
- **"Native Go is cooler than shell" novelty bias** — the whole repo is mid-rewrite to Go;
  there's momentum/identity pressure to Go-ify everything, including the 3 tools (tpm =
  git clone, tmux-sessionizer = cargo-from-git, gnu-utilities = brew loop) that are
  _inherently_ setup-steps with no clean backend analog.
- **Comparator tools sell their paradigm** — chezmoi docs advocate declarative/templating;
  aqua advocates lockfiles+Renovate; Nix advocates reproducibility. Each has a stake in you
  adopting _their_ complexity. The brief's own bias list ("feature-creep," "framework,"
  "novelty") is the right antidote.

---

## Key Insights

1. **The migration's own proof-of-concept is broken.** starship (`script-url`, default
   intent) fails checksum verification today. Before porting 19 more, fix or rethink the 1.
2. **The real, shippable bug is state detection, not the shell-outs.** `custom-script`
   `Installed()` lies about 8/19 tools (`custom_script.go:49`). This makes `dotf status`
   and `dotf diff` untrustworthy — which undermines the "richer status/diff" UX asks
   elsewhere. Fix the probe first.
3. **`apply` is convergent-by-luck and non-deterministic.** No `Installed()` gate
   (`install.go:104`) + silent self-update of rolling tools = re-does all work every run
   and can drift versions. This contradicts the reproducibility narrative.
4. **A permanent, well-scoped shell-out is correct design, not a failure** — chezmoi
   proves it. Reframe `custom-script` from "escape hatch to shrink to zero" to "supported
   backend for genuinely imperative setup steps," and only migrate the _mechanically clean_
   cases (single binary from a github release with an immutable, verifiable asset).
5. **The taxonomy is Debian/Arch-centric.** Hard-coded `noble` (FLAW 4) + no `rpm-repo`
   (FLAW 5) mean cross-distro portability is aspirational. Fix templating before claiming
   parity.
6. **LOC-deleted is the wrong North Star.** Optimize for correct state detection and fewer
   breakages. By that measure the top of the backlog is bug-fixes, not a rewrite.

---

## Evidence Quality

- **Code findings (FLAW 1–6):** **High.** Read from source (`custom_script.go`, `pkg.go`,
  `script_url.go`, `apt_repo.go`, `install.go`, `applyphases.go`, `applyopts.go`,
  `packages.toml`) and cross-checked against the actual `bin/install-*.sh` binaries via grep.
  Binary-name mismatches independently confirmed (`command_exists az`, `command -v cargo`,
  `tms`, git-clone tpm).
- **starship stale pin (FLAW 2):** **High but re-verify.** Live `sha256sum` differs from the
  pinned value; fetched over a sandboxed network — a captive-portal artifact is unlikely
  (curl `-f` would fail on non-200) but the maintainer should re-run the one-liner.
- **External critiques:** **Medium-High.** Primary/near-primary sources (Joel on Software,
  Chef blog, chezmoi official docs + GitHub issues, GNU Stow manual, mise GitHub
  discussions). The Nix "not worth it" synthesis returned **no citable links** — treat as
  directional community sentiment, not a hard citation.

---

## Contradictions & Uncertainties

- **"Add reconciliation" vs "additive-only is a feature."** Unresolved by design intent:
  chezmoi's closed-wontfix stance strongly supports _keeping_ additive-only; the PRD's
  deferred `--prune` keeps the door open. I come down firmly on **additive-only is
  correct** for a single-host tool.
- **"Pin everything for security" vs "pins rot and break the default path."** The starship
  data point resolves this toward **pin only immutable/versioned artifacts; don't pin
  rotating installers.** Direct conflict with a naive "hardening = more checksums" reading.
- **"Migrate for LOC" vs "keep hardened shell (Spolsky)."** Genuine trade-off; my resolution
  is _selective_ migration of the ~5–6 mechanically-clean, single-binary, immutable-asset
  cases only (e.g. lefthook, tea, turso, coderabbit if they ship verifiable release assets),
  and **explicitly keep** the setup-steps (tpm, tmux-sessionizer, gnu-utilities, mise, rust,
  lua) as custom-scripts forever.
- **Uncertainty:** exact binary names for `coderabbit`, `turso`, `tea`, `opencode`, `codex`
  were not each byte-verified; `coderabbit` script does probe `command -v coderabbit`
  (plausibly correct), but `tea` (Gitea CLI) and `fabric` (pyinvoke collision) carry real
  name-collision risk that needs per-tool confirmation.

---

## Search Queries Executed

1. `chezmoi apply broke my machine footgun overwrite dotfiles horror story`
2. `curl pipe bash install script supply chain risk criticism sha256 pin install.sh rotates`
3. `declarative configuration management over-engineered chezmoi templating complexity criticism "just use a shell script"`
4. `GNU Stow symlink conflict clobber existing files complaints dotfiles problems`
5. `Nix learning curve too steep complexity criticism "not worth it" dotfiles home-manager`
6. `chezmoi does not remove packages additive only no uninstall reconciliation design decision`
7. `aqua cli version manager checksum lockfile maintenance burden vs mise pinning tools`
8. `Joel Spolsky things you should never do rewrite working code from scratch worst strategic mistake`

Plus one live verification: `curl -fsSL https://starship.rs/install.sh | sha256sum`
(2026-07-04) → `52c64f14…cc8fbb` ≠ pinned `e9f0bb84…5534c9`.
</content>
</invoke>
