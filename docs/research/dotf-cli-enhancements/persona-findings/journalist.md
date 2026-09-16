# The Journalist — Current State of the Art (2025–2026)

> Persona mandate: report the CURRENT dotfiles + provisioning landscape and what
> `dotf`'s peers ship TODAY. Dated citations, confidence ratings, in-scope/out-of-scope
> tags against `dotf`'s declared scope. Ground state-of-dotf claims in the repo.
> Research window: searches run 2026-07-04.

---

## Executive Summary

The dotfiles/provisioning world has bifurcated since ~2023 into two cleanly separated
layers, and `dotf` straddles both:

1. **The dotfile layer** (symlink/copy config into `$HOME`): **chezmoi** is the
   consensus winner of 2025-2026 — single Go binary, templating, age/gpg encryption,
   `doctor`, `diff`/`status`, externals, run-once scripts, password-manager refs.
   yadm (git-wrapper), dotbot (symlink bootstrapper), GNU Stow (symlink farm), rcm,
   and Nix/home-manager (declarative) fill the long tail.
2. **The tool-installer layer** (get binaries onto the box): **mise** and **aqua** are
   the 2025-2026 zeitgeist — declarative TOML/YAML manifests, a shared **aqua
   registry**, **lockfiles with checksums + SLSA provenance**, and **Renovate**
   auto-bumps. Below them sit single-purpose fetchers **ubi**/**eget** and the "run
   anything" **pkgx**; **topgrade** is the cross-ecosystem `update-everything` layer.

`dotf` today implements the _mechanics_ of both layers (native symlink engine, a
7-backend package installer, `plan`/`status`/`diff`/`apply`) but is missing the
_maturity features_ that every serious peer now ships: **a lockfile/checksum-pinning
story for its 19 `custom-script` shell-outs**, **`doctor`/health**, **`update`/
`self-update`**, **`--json` output**, **`NO_COLOR`**, and **`--only`/`--skip`
filters**. The single highest-leverage current-state signal for `dotf`: the entire
tool-installer industry (aqua, mise) has converged on **manifest + lockfile + checksum

- Renovate** as the answer to exactly the supply-chain risk that `dotf`'s 19
  `custom-script` / 1 `script-url` / vendor-`curl|bash` entries currently carry unpinned.

The most important framing insight: **`dotf`'s "shrinking escape hatch" that never
shrank (19/37 packages are `custom-script`) is not a unique failure — it is the same
migration every tool fought.** aqua's whole pitch is "you don't have to maintain shell
scripts anymore." mise's whole pitch is a registry that retires per-tool plugins. `dotf`
is mid-migration on a path the industry has already paved.

---

## Current State — Dotfiles + Tool-Installer Landscape (2025–2026)

### The dotfile layer

| Tool                   | Model                                       | 2025-2026 status                                                                                                                                                                         |
| ---------------------- | ------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **chezmoi**            | Go binary, copies real files (not symlinks) | Dominant. Actively developed (v2.64.0 built 2025-08-06 per discussion #4594). Templating + age/gpg + doctor + externals + password managers.                                             |
| **yadm**               | Bare-git wrapper                            | Stable, git-savvy niche. `##os.Linux`/`##os.Darwin` alternates, git-crypt, bootstrap. Users migrate off it to chezmoi for templating (its j2cli/envtpl template deps went unmaintained). |
| **dotbot**             | Python, config-driven symlink bootstrapper  | Minimal "one-click" install. No templating/encryption.                                                                                                                                   |
| **GNU Stow**           | Perl symlink-farm manager                   | The Unix-purist classic; symlink-only, painful migration off.                                                                                                                            |
| **rcm / vcsh**         | Bash multi-file / git-per-repo              | Legacy long-tail.                                                                                                                                                                        |
| **Nix + home-manager** | Declarative, immutable, reproducible        | Growing but polarizing — "reproducibility vs friction" (see Contemporary Debates).                                                                                                       |

chezmoi's own comparison table (chezmoi.io/comparison-table, current) is the clearest
map of the layer — see the Capability Matrix below.

### The tool-installer layer (the 2025-2026 action)

- **mise** (mise.jdx.dev): dev-env manager; tools declared in `mise.toml [tools]`;
  installs via **backends** — `aqua` (recommended default), `github`, `gitlab`, `ubi`,
  `npm`, `cargo`, etc. Ships **`mise.lock`** (opt-in `lockfile=true`) recording exact
  versions + tarball checksums + download URLs (avoids GitHub API rate limits),
  **`mise self-update`** (binary) and **`mise upgrade [--bump]`** (tools), and native
  **SLSA provenance verification without the slsa-verifier CLI**.
- **aqua** (aquaproj.github.io): "Declarative CLI Version manager written in Go.
  Support Lazy Install, Registry, and continuous update with Renovate." Central
  **standard registry** maps friendly names → backends; **`aqua-checksums.json`** +
  **`aqua update`** (since v2.14.0) + **Renovate preset** auto-bumps versions AND
  checksums. Explicit thesis: _"you don't have to maintain shell scripts anymore."_
  mise **compiles the aqua registry into its own binary** — the registry, not the CLI.
- **ubi** (houseabsolute/ubi, Rust): "Universal Binary Installer" — pulls one binary
  from GitHub/GitLab/Codeberg/Forgejo releases, filters by OS/arch/extension, `--matching`
  disambiguation, token auth per forge.
- **eget** (zyedidia/eget, Go): fetch+extract a single prebuilt binary from a repo/URL;
  TOML config at `~/.eget.toml`/XDG; hits GitHub API rate limits without `GITHUB_TOKEN`.
- **pkgx** (pkgxdev, 4 MiB binary): "run anything" — downloads to `~/.pkgx` and runs
  without permanently installing.
- **topgrade** (topgrade-rs, Rust, v15.x mid-2026): one command updates _everything_ —
  apt/brew/dnf/pacman/nix/cargo/rustup/pip/npm/go/flatpak/snap + editor plugins + git
  dotfile repos; `--dry-run`, `--only`, `--disable`, `--list-steps`, `[git] repos`,
  `[commands]`, remote-over-ssh. This is the `dotf update`-shaped niche.

> Confidence: **High.** All sourced from primary docs / official GitHub, all current as
> of the 2026-07-04 search window.

---

## Capability Matrix — Competitors vs `dotf`

`dotf` current state grounded in repo (`cmd/dotf/*.go`, `internal/*`, `manifest/packages.toml`).
Scope tags reflect the PRD (objective.md): **IN** = fits single-host/additive/TOML/Go/stdlib-flag
scope; **OUT** = explicitly deferred/excluded; **EDGE** = plausible but debatable.

### Dotfile-layer features

| Feature                           |  chezmoi  |     yadm      | dotbot  |   Stow    |   Nix/HM   |           **dotf today**            | Scope for dotf                              |
| --------------------------------- | :-------: | :-----------: | :-----: | :-------: | :--------: | :---------------------------------: | ------------------------------------------- |
| Single-binary distribution        |    ✅     |      ❌       |   ❌    |    ❌     |     ❌     |       ✅ (GoReleaser + shim)        | —                                           |
| Symlink/copy into `$HOME`         |   copy    |      git      | symlink |  symlink  | store-link | ✅ `dotf link` (`internal/symlink`) | —                                           |
| `diff` before apply               |    ✅     |      ✅       |   ❌    |    ❌     |     ✅     |           ✅ `dotf diff`            | —                                           |
| `status`                          |    ✅     |      ✅       |   ❌    |    ❌     |     ✅     |          ✅ `dotf status`           | —                                           |
| Dry-run                           |    ✅     |       —       |   ✅    | ✅ (`-n`) |   build    |           ✅ `--dry-run`            | —                                           |
| Templating (per-host render)      |    ✅     |      ✅       |   ❌    |    ❌     |     ✅     |                 ❌                  | **EDGE** (TOML-first; heavy)                |
| Per-host / profile config         | templates |   alt-files   |   ❌    | branches  |   flakes   |    ⚠️ intents only (no per-host)    | **IN** (intents ≈ profiles; add host match) |
| Encrypted secrets (age/gpg)       |    ✅     |      ✅       |   ❌    |    ❌     |     ✅     |                 ❌                  | **OUT** (secrets orchestration excluded)    |
| Password-manager refs             |    ✅     |      ❌       |   ❌    |    ❌     |     ✅     |                 ❌                  | **OUT**                                     |
| Run scripts                       |    ✅     |      ✅       |   ✅    |    ❌     |     ✅     | ⚠️ via `custom-script`/system steps | **IN**                                      |
| Run-**once** scripts              |    ✅     |      ✅       |   ❌    |    ❌     |     ✅     |      ❌ (no state/once-ledger)      | **IN** (high value)                         |
| Externals (remote files/archives) |    ✅     |      ❌       |   ❌    |    ❌     |     ✅     |                 ❌                  | **EDGE**                                    |
| File removal / prune              |    ✅     |      ✅       |   ❌    |    ✅     |     ✅     |         ❌ (additive-only)          | **OUT** (deferred `--prune`)                |
| Shell completion                  |    ✅     |      ✅       |   ❌    |    ❌     |     ✅     | ✅ `dotf completion` (+`--install`) | —                                           |
| `doctor` / health                 |    ✅     | ✅ (`doctor`) |   ❌    |    ❌     |     —      |                 ❌                  | **IN** (high value)                         |
| Archive import/export bundle      |    ✅     |      ❌       |   ❌    |    ❌     |     ❌     |                 ❌                  | **EDGE**                                    |

### Tool-installer-layer features

| Feature                                |         mise          |           aqua           |     ubi/eget     |        topgrade         |                  **dotf today**                   | Scope for dotf                     |
| -------------------------------------- | :-------------------: | :----------------------: | :--------------: | :---------------------: | :-----------------------------------------------: | ---------------------------------- |
| Declarative manifest                   |      `mise.toml`      |       `aqua.yaml`        |   `.eget.toml`   |     `topgrade.toml`     |            ✅ `manifest/packages.toml`            | —                                  |
| Native pkg-mgr install                 |      via distro       |            ❌            |        ❌        |        (updates)        |                 ✅ `pkg` backend                  | —                                  |
| GitHub-release binary fetch            |    `github`/`ubi`     |         ✅ core          |     ✅ core      |           ❌            | ✅ `github-release` (1 use, needs `tag`+`sha256`) | **IN**                             |
| `{{.OS}}`/`{{.Arch}}` asset templating |          ✅           |            ✅            | ✅ (auto-detect) |            —            |           ❌ (arch-explicit asset only)           | **IN** (blocks migrations)         |
| Version pinning                        |          ✅           |            ✅            |     tag arg      |            —            |      partial (`tag` on github-release only)       | **IN**                             |
| **Lockfile + checksums**               |    ✅ `mise.lock`     | ✅ `aqua-checksums.json` |        ❌        |           ❌            |                        ❌                         | **IN** (highest-value gap)         |
| **SLSA provenance / cosign**           |       ✅ native       |            ✅            |     ubi: no      |           ❌            |                        ❌                         | **EDGE** (nice, heavier)           |
| Renovate auto-bump                     |   ✅ (renovate mgr)   |        ✅ preset         |        ❌        |           ❌            |                        ❌                         | **IN** (CI, cheap)                 |
| Registry (shared names)                |     ✅ (aqua reg)     |            ✅            |        ❌        |           ❌            |           ❌ (hand-written `names={}`)            | **OUT** (over-scope for solo host) |
| `self-update` (the binary)             | ✅ `mise self-update` |       ✅ installer       |      manual      |         pkg-mgr         |                        ❌                         | **IN** (high value)                |
| `update`/`upgrade` (installed tools)   |   ✅ `mise upgrade`   |     ✅ `aqua update`     |        ❌        |         ✅ core         |                        ❌                         | **EDGE/IN**                        |
| `--only`/`--skip`/`--disable` filters  |      via config       |            —             |        —         | ✅ `--only`/`--disable` |                ⚠️ `--intent` only                 | **IN** (cheap, high value)         |
| `--dry-run` preview                    |          ✅           |            —             |        ✅        |           ✅            |                        ✅                         | —                                  |
| JSON output                            |          ✅           |            —             |        —         |            —            |                        ❌                         | **IN** (cheap, stdlib-friendly)    |
| `NO_COLOR`                             |      (respected)      |            —             |        —         |            —            |                    ❌/unknown                     | **IN** (trivial)                   |

> Confidence: **High** for competitor cells (primary docs); **High** for dotf cells
> (grounded in repo). The `NO_COLOR` dotf cell is **Medium** — not verified in code,
> flagged as an audit item.

---

## Key Players (who's driving the state of the art)

- **twpayne/chezmoi** — sets the dotfile-layer feature bar. Everything `dotf`'s
  `link`/`diff`/`status`/`apply` competes against is measured here. v2.64.0 (2025-08).
  A "v3" is _discussed_ (#2673) — unified config file, tree-based state — but the
  maintainer cautions it "may or may not happen" (v1→v2 took ~a year). Signal: even the
  leader's roadmap is conservative; `dotf` need not chase bleeding edge.
- **jdx/mise** — sets the tool-installer bar. `mise.toml [tools]` + `mise.lock` +
  aqua-backend + `self-update`. The reference design for "manifest + lockfile +
  provenance." Actively shipping through 2026 (lock-resolution + attestation fixes).
- **aquaproj/aqua** + **suzuki-shunsuke** — the supply-chain-security player.
  Registry + `aqua-checksums.json` + Renovate + cosign/SLSA. The canonical answer to
  "retire your install shell scripts." mise adopting its registry is the strongest
  ecosystem endorsement of 2025.
- **houseabsolute/ubi** + **zyedidia/eget** — the minimal "one binary from a release"
  primitives; the mental model for a _better_ `dotf github-release` backend.
- **topgrade-rs/topgrade** — owns `update-everything`; the model for a future
  `dotf update`.
- **creativeprojects/go-selfupdate** + **minio/selfupdate** — the two Go libraries that
  make `dotf self-update` a ~1-day feature (both GoReleaser-`checksums.txt`-aware).

---

## Latest Developments (dated)

- **2025-01** — Feature request to add native checksum verification to `curl` itself;
  maintainer Daniel Stenberg rejects streaming verification as impractical (must buffer
  whole file). Takeaway: **`curl|bash` verification stays the caller's job** — exactly
  the gap in `dotf`'s `script-url`/vendor-installer entries. (curl.se/mail/archive-2025-01)
- **2025-04** — Ian Lewis "TIL: Aqua CLI Version Manager" popularizes aqua's
  checksum-auto-maintenance thesis. (ianlewis.org/til/2025/04/01)
- **2025-08-06** — chezmoi v2.64.0; `--age-recipient`/`--age-recipient-file` being
  promoted to global flags (discussion #4594).
- **2025** — mise `mise.lock` records **verified provenance type** (slsa/cosign/
  minisign/github-attestations) by downloading + cryptographically verifying at lock
  time for the current platform; stores download URLs to dodge GitHub API limits.
  (mise.jdx.dev/dev-tools/mise-lock)
- **2025-Q3/Q4** — chezmoi maintainer flags a wave of new-user confusion
  (`.chezmoiignore` ≠ `.gitignore`; source-of-truth editing) → quick-start rewrite
  effort (issue #4955). Signal: **onboarding docs are a real, current pain even for the
  leader.**
- **2025-07** — OMG Ubuntu / Tom's Hardware coverage of topgrade as the one-command
  updater; topgrade at v15.x by mid-2026.
- **2025-12-19** — creativeprojects/go-selfupdate latest release (still actively
  maintained). (pkg.go.dev/github.com/creativeprojects/go-selfupdate)
- **2025-09 / 2026-06** — AUR supply-chain attacks ("Atomic"/Arch, 1,500+ packages,
  Rust infostealer + eBPF rootkit) drive community "pin your sources / verify checksums"
  tooling. Reinforces the manifest+lockfile+checksum zeitgeist. (javapro.io 2026-03,
  AUR-scanner repos)
- **2026-04** — fresh chezmoi "one repo across macOS/Linux/Windows" walkthroughs still
  being published — the category is not slowing down. (recca0120.github.io)

---

## Modern Go-CLI UX Conventions `dotf` Could Adopt (within stdlib-flag)

All achievable **without** Cobra — flagged per the project's hard "no framework" constraint.

1. **`--json` / `--plain` structured output.** 2025 convention: human tables by default
   (stdlib `text/tabwriter`), machine output opt-in. `dotf status`/`plan`/`diff` are the
   obvious first targets. **No framework needed** — a shared output-mode helper +
   `encoding/json`. **Value: high, effort: low-med.**
2. **Stream separation:** progress/hints → **stderr**, data → **stdout**, so `dotf
status --json | jq` stays clean. **No framework needed.** Audit current code for
   leakage. **Value: med, effort: low.**
3. **`NO_COLOR` env respect** + auto-disable color under `--json`/non-TTY. This is a
   settled community standard. **Trivial** (`os.Getenv("NO_COLOR")` + `isatty`).
   **Value: med, effort: trivial.**
4. **`dotf doctor` / `diagnose` subcommand.** Both chezmoi and yadm ship `doctor` and it
   is _the_ "new machine misbehaves, run this first" affordance. For `dotf`: verify
   platform detection, pkg-mgr presence, brew bootstrap, required tools on PATH, manifest
   validity, symlink drift, sudo/privilege availability, snapper availability. Reuses the
   same `--json` plumbing. **This is the single most-recommended missing UX affordance.**
   **Value: high, effort: med.**
5. **`--only` / `--skip` package/intent filters** (topgrade's `--only`/`--disable`
   pattern). `dotf` has `--intent` but no per-package include/exclude within a run.
   Cheap with stdlib flag (comma-split). **Value: high, effort: low.**
6. **Progress affordance during long `apply`.** `charmbracelet/huh` is already a dep;
   `bubbles/spinner` or a simple `[3/12] installing …` counter fits without Cobra.
   **Value: med, effort: low-med.**
7. **Richer `--dry-run` diff rendering** for `apply` (mise/topgrade both preview). `dotf`
   has `--dry-run`; making it render a _plan diff_ (would-install / would-skip /
   already-present) closes the gap. **Value: med, effort: med.**
8. **XDG state dir for a lockfile / once-ledger** (`$XDG_STATE_HOME/dotf/`). Enables
   run-once scripts and a lockfile without touching config. **Value: high (enabler),
   effort: med.**

> Confidence: **High** these are current conventions (Go CLI best-practices sources,
> gogcli spec, zli); **High** they're stdlib-compatible.

---

## Self-Update Landscape

The industry norm for single-binary Go/Rust CLIs is a **first-class `self-update`/
`upgrade` command** — `gh extension upgrade`, `mise self-update`, rclone/minio
`selfupdate`, `cloudlens update`. `dotf` ships via GoReleaser + a POSIX shim but has
**no `dotf self-update`** (explicitly listed as "known absent" in objective.md).

Two off-the-shelf Go libraries make this a ~1-day feature, both GoReleaser-native:

- **creativeprojects/go-selfupdate** (batteries-included): `DetectLatest` → compare
  semver git tag → download per-OS/arch asset → verify → `UpdateTo`. Ships a
  `ChecksumValidator{UniqueFilename: "checksums.txt"}` that consumes GoReleaser's
  checksum file directly. Handles archive formats, ARM naming, macOS universal fallback.
  Last release 2025-12-19. **Best fit** — `dotf` already releases via GoReleaser with a
  checksums file that the shim already verifies.
- **minio/selfupdate** (low-level): you fetch, it `Apply`s an `io.Reader` as the new
  binary via atomic rename (`.target.old`/`.target.new` dance) with SHA256 validation
  (default) + rollback + optional bsdiff patching + code-signing. Use if you want full
  control of the download source.

Design note for `dotf`: because the binary is installed by the shim to a known path and
GoReleaser already emits `checksums.txt`, a `dotf self-update` that mirrors
`gh extension upgrade` (user-initiated, checksum-verified, `--dry-run` to preview the
version bump) is low-risk and closes a conspicuous gap. **Scope: IN** (single-host,
additive, no framework). **Value: high, effort: low.**

> Confidence: **High.**

---

## Contemporary Debates

### Declarative (Nix) vs imperative — and "Nix fatigue"

- **The pro-declarative case (2025):** home-manager gives reproducible,
  version-controlled `$HOME`; flakes' `flake.lock` pins exact input commits, "banishing
  works-on-my-machine." No dead symlinks, no long bash scripts.
- **The fatigue case (2025):** immutable workflow friction (can't edit the generated
  file; must `home-manager switch`), **incomplete wrapper coverage** (`extraConfig`
  escape hatches abound; not all programs wrapped), daunting option surface, edge-case
  breakage (tmpfs `$HOME`, standalone installs).
- **The emerging pragmatic consensus:** _it's not all-or-nothing_ — use home-manager
  just for `home.packages`, keep native config formats via `home.file`, adopt
  incrementally (move `ripgrep`/`fzf`/`git` first).

**Relevance to `dotf`:** `dotf`'s **additive, native-format, TOML-first, "stop anytime"**
stance is squarely the pragmatic middle the fatigue discourse is converging _toward_ —
same territory chezmoi occupies ("files are real files; stop using it anytime"). This is
**evidence FOR `dotf`'s deliberately-narrow scope**, not against it. The `custom-script`
"keep the working shell script" instinct mirrors home-manager's `extraConfig` reality:
**even the most declarative tool keeps an imperative escape hatch.** So the contrarian
"is retiring custom-script even worth it?" question has real industry support — the goal
should be _pinning/verifying_ the escape hatch, not _eliminating_ it.

### Symlink vs copy

chezmoi's headline argument — copy real files so you can walk away — is a knock on the
symlink-farm model (`dotf link`, Stow, dotbot). But `dotf`'s symlink approach is
mainstream and shared by yadm-adjacent workflows; **out of scope** to re-litigate
(the native symlink engine is a settled Phase-4 decision).

### "Retire the shell scripts" vs "keep hard-won platform logic"

aqua's explicit thesis ("you don't have to maintain shell scripts anymore") pulls toward
full migration; the **rewrite-bias guard** in objective.md and the home-manager
`extraConfig` reality pull toward keeping genuinely-complex scripts (mise/rust rolling
version managers, `tpm` git-clone, `tmux-sessionizer`, `gnu-utilities`). **The industry
splits the difference: migrate the mechanical GitHub-release/`curl|sh` cases to a
verified native backend; keep the genuinely-imperative setup steps** — but even those get
a **checksum/lock** wrapper.

> Confidence: **Medium-High** — debate is real and well-sourced, but sentiment sources
> skew enthusiast-blog (noted in Evidence Quality).

---

## User Pain Points & Feature Requests (recent, 2025–2026)

Recurring themes across chezmoi + mise issue trackers — the concrete things users
complain about NOW, mapped to `dotf` relevance:

**chezmoi (GitHub issues/discussions, 2025-Q3/Q4):**

- **Onboarding confusion is #1** — `.chezmoiignore` ≠ `.gitignore`; users reinvent
  built-in features; source-of-truth editing model unclear (#4955, #4420). → `dotf`
  lesson: **invest in docs + a `doctor`/explain affordance**; the leader's biggest
  current pain is _comprehension_, not features.
- **`update` should offer re-add** (#3731) — after `update` shows a changed file, users
  must break flow to `re-add`. → `dotf` has no `update` yet; if added, bake in the
  reconcile affordance.
- **Skip encrypted files during `update`** (#3794); **`--exclude` once crashed** (#1163).
  → validates **`--only`/`--skip` filters** as a real need.
- **Config-apply ordering** (#4934); **`chezmoi mv`** requested and **declined** (#3918)
  — even the leader says no to scope creep. → **evidence for `dotf`'s scope discipline.**
- **externals: `chezmoi add` breaks inside externally-managed dirs** (2025-12
  write-up); huge-archive externals are re-validated every `diff`/`apply` (perf
  warning). → if `dotf` ever adds externals, learn the cache/ignore lessons.

**mise (GitHub discussions, 2025):**

- **shims vs `activate` PATH conflicts** (#4444, #6871), wrong-version-first-in-PATH,
  **`activate` silently no-ops in non-interactive scripts**, stale-state
  `ignored-configs` blacklisting, WSL PATH leakage, `reshim` not clearing errors.
  → These are **runtime-integration** pains largely orthogonal to `dotf` (it doesn't do
  shims), but they're a caution: **PATH/shell-integration magic generates the most
  support load.** `dotf`'s explicit, non-magic install model dodges this whole class.

**topgrade:** the standing request is simply broader coverage + fewer silent failures;
its `--only`/`--disable`/`--list-steps` are the most-praised ergonomics.

> Confidence: **High** — direct issue-tracker citations with numbers/dates.

---

## Key Insights (for `dotf`)

1. **The whole tool-installer industry has already answered `dotf`'s central tension.**
   aqua + mise converged on **declarative manifest → lockfile → checksum → SLSA →
   Renovate**. `dotf` has the manifest; the **missing rung is a lockfile/checksum-pin
   story** — and it maps 1:1 onto the objective.md "unpinned `script-url`/vendor
   installer supply-chain" risk. This is the current-state through-line.
2. **`custom-script`-heavy is a normal migration stage, not a unique failure.** aqua
   exists _because_ everyone had unmaintainable install scripts; home-manager keeps
   `extraConfig`. The industry move is **pin + verify the escape hatch**, then migrate
   the _mechanical_ cases (GitHub-release/`curl|sh`) to a native verified backend, and
   _keep_ the genuinely-imperative ones (version managers, git-clone setup).
3. **`doctor` is table stakes.** chezmoi + yadm both ship it; it's the universal
   "new-machine-misbehaves" entry point. `dotf`'s absence here is its most conspicuous
   day-to-day UX gap. Cheap, high-value, stdlib-friendly.
4. **`self-update` is a solved 1-day feature** via go-selfupdate + the GoReleaser
   `checksums.txt` `dotf` already produces. Every peer single-binary ships it.
5. **`--only`/`--skip` filters + `--json` + `NO_COLOR`** are the cheap modern-CLI
   hygiene wins — all achievable within stdlib flag, all validated by current
   convention and real chezmoi/topgrade user demand.
6. **`dotf`'s narrow, additive, native-format scope is vindicated by the "Nix fatigue"
   discourse**, which is converging on exactly that pragmatic middle. Don't let
   feature-envy erode it. The declined `chezmoi mv` (#3918) is a model of scope discipline.
7. **`{{.OS}}`/`{{.Arch}}` asset templating + version pinning are the specific backend
   capabilities blocking clean `custom-script`→`github-release` migration.** ubi/eget/
   mise all auto-detect arch; `dotf`'s single-`sha256`-per-arch-explicit-asset model is
   the concrete gap (objective.md confirms `github-release` "cannot cover
   `{{.Arch}}`-templated assets" today).
8. **Onboarding/comprehension, not features, is the leader's current pain.** A little
   docs + explainability investment (a `doctor` that _explains_, clear `plan` output)
   punches above its weight.

---

## Evidence Quality

- **Primary-source strength: High.** Competitor claims are grounded in official docs
  (chezmoi.io, mise.jdx.dev, aquaproj.github.io) and official GitHub repos/issues with
  numbers and dates. `dotf` current-state grounded directly in the repo
  (`manifest/packages.toml`, `cmd/dotf/*.go` listing, CLAUDE.md/objective.md).
- **Recency: High.** Most citations 2025–2026; several 2025-Q3/Q4 issue threads and a
  2025-12 library release; 2026-Q1/Q2 news coverage.
- **Weakest evidence: the "Nix fatigue" sentiment** — sources skew enthusiast blogs,
  wikis, vendor posts rather than a neutral survey. Directionally consistent but not
  quantified. The mise/chezmoi _pain points_ are stronger (direct issue trackers).
- **Unverified in-repo:** `dotf`'s `NO_COLOR` handling and exact `stdout`/`stderr`
  stream discipline were not code-audited by this persona — flagged as audit items, not
  asserted as gaps with certainty.
- **US-only search bias** (WebSearch constraint) may under-sample non-English community
  sentiment.

---

## Contradictions & Uncertainties

1. **"Retire custom-script" (aqua thesis) vs "keep hard-won scripts" (rewrite-bias
   guard + home-manager `extraConfig` reality).** Both are legitimate and current. The
   synthesis — migrate mechanical cases, pin/verify the rest — is _my_ reconciliation,
   not a settled external consensus. **Uncertainty: which of the 19 are "mechanical" is
   a per-script call** (other personas own that classification).
2. **`update`/`upgrade` scope.** topgrade/mise both ship it, but for `dotf` a full
   `update` risks colliding with the additive-only / no-reconciliation scope (a re-run
   of `apply` is already idempotent-ish). Whether `dotf update` should exist vs. lean on
   `topgrade` is a genuine open question. **EDGE, not clearly IN.**
3. **Lockfile vs additive-only.** A lockfile implies _pinning/reproducibility_, which is
   adjacent to but not the same as _reconciliation/prune_ (explicitly OUT). A lockfile is
   IN even though prune is OUT — but the boundary invites confusion and should be stated
   explicitly if pursued.
4. **Templating.** chezmoi's flagship feature, but for a TOML-first single-host tool with
   `intents`, full Go-template rendering may be over-scope. Tagged **EDGE** — defensible
   either way; per-host _conditionals_ (like `os = "macos"` on symlinks, already present)
   may be enough without a template engine.
5. **SLSA/cosign provenance.** aqua/mise ship it, but it's heavier than checksum pinning
   and depends on upstreams publishing attestations (aqua notes "very few tools" do).
   For `dotf`, **checksum+lockfile is the 80%; SLSA is the aspirational 20%.**

---

## Search Queries Executed

1. `chezmoi 2025 new features release doctor age encryption profiles`
2. `mise mise.toml 2025 tools lockfile self-update backends aqua`
3. `aqua registry Renovate auto-update tool installer 2025 SLSA provenance checksum`
4. `topgrade 2025 update all tools upgrade dotfiles package managers`
5. `chezmoi vs yadm vs dotbot vs GNU Stow 2025 comparison dotfiles manager`
6. `eget ubi proto pkgx 2025 single binary installer github release fetch`
7. `Go CLI best practices 2025 NO_COLOR structured JSON output doctor subcommand stdlib flag`
8. `Go single binary self-update command 2025 gh extension upgrade minio selfupdate go-selfupdate GoReleaser`
9. `chezmoi issues feature requests 2025 pain points GitHub discussions apply exclude update`
10. `Nix home-manager fatigue 2025 declarative vs imperative dotfiles debate reproducibility`
11. `chezmoi externals run_once scripts .chezmoiexternal archive package bundles 2025`
12. `curl bash install script security 2025 supply chain checksum verification unpinned risk`
13. `mise reddit 2025 complaints problems shims activate PATH issues user feedback`

- WebFetch: `chezmoi.io/comparison-table` (capability matrix grounding)

### Key sources (dated)

- chezmoi comparison table — https://www.chezmoi.io/comparison-table/ (current)
- chezmoi release history — https://www.chezmoi.io/reference/release-history/
- chezmoi age encryption — https://www.chezmoi.io/user-guide/encryption/age/
- chezmoi discussion #4594 (global `--age-recipient`), v2.64.0 built 2025-08-06
- chezmoi issues #4955, #3731, #3794, #1163, #4934, #3918; discussions #2673, #4420 (2025)
- chezmoi externals — https://www.chezmoi.io/reference/special-files/chezmoiexternal-format/; stoeps.de/posts/2025/, mischavandenburg.com (2025-12)
- mise lockfile — https://mise.jdx.dev/dev-tools/mise-lock.html
- mise aqua backend — https://mise.jdx.dev/dev-tools/backends/aqua.html
- mise ubi backend — https://mise.jdx.dev/dev-tools/backends/ubi.html
- mise upgrade / self-update — https://mise.jdx.dev/cli/upgrade.html
- mise shims/PATH discussions #4444, #6871, #5037; troubleshooting docs (2025)
- Renovate mise manager — https://docs.renovatebot.com/modules/manager/mise/
- aqua docs / Renovate guide / cosign-slsa — https://aquaproj.github.io/docs/ (+ /guides/renovate/, /reference/security/cosign-slsa/)
- Ian Lewis, aqua TIL — https://www.ianlewis.org/til/2025/04/01/aqua-cli-version-manager
- ubi — https://github.com/houseabsolute/ubi ; eget — https://github.com/zyedidia/eget ; pkgx — https://github.com/pkgxdev/pkgx
- topgrade — https://github.com/topgrade-rs/topgrade ; omgubuntu.co.uk/2025/07 ; tomshardware.com
- go-selfupdate — https://github.com/creativeprojects/go-selfupdate (rel. 2025-12-19); minio/selfupdate — https://github.com/minio/selfupdate ; one2n.io self-updatable Go CLI
- Go CLI conventions — gogcli.sh/spec.html ; pkg.go.dev/zgo.at/zli ; awesome-go standard-cli
- home-manager / Nix — nixos.wiki/wiki/Home_Manager ; callistaenterprise.se/blogg/2025/04/10 ; programming-helper.com nixos-2026
- curl|bash security — curl.se/mail/archive-2025-01/0019.html (checksum FR, Stenberg reply); sysdig.com friends-dont-let-friends-curl-bash; javapro.io/2026/03/25; reflexframework.com; checksum.sh

---

### Digest — most actionable current-state findings for `dotf`

- **Lockfile + checksum pinning is the industry's answer to `dotf`'s exact supply-chain
  gap.** aqua (`aqua-checksums.json`) and mise (`mise.lock`, with SLSA verified at lock
  time) both converged on manifest→lockfile→checksum→Renovate; `dotf` has the manifest
  and needs the pinning rung — directly retiring the risk in its 19 `custom-script` / 1
  `script-url` / vendor-`curl|bash` entries. **IN scope, highest leverage.**
- **`dotf doctor` is table stakes** — chezmoi and yadm both ship it as the universal
  "new machine misbehaves, run this first"; its absence is `dotf`'s most conspicuous
  daily-UX gap. Cheap, stdlib-friendly, high value. **IN.**
- **`dotf self-update` is a ~1-day feature** via `creativeprojects/go-selfupdate` +
  the GoReleaser `checksums.txt` the shim already verifies; every peer single-binary
  (mise/gh/rclone) ships it. **IN.**
- **Cheap modern-CLI hygiene, all within stdlib flag (no Cobra):** `--only`/`--skip`
  filters (topgrade model + real chezmoi demand), `--json` output for
  `status`/`plan`/`diff`, `NO_COLOR` respect, stdout/stderr stream separation. High
  value-to-effort. **IN.**
- **The `custom-script` glut is a normal mid-migration stage, not a unique failure** —
  aqua exists precisely to retire install scripts; home-manager keeps `extraConfig`. The
  move is _pin+verify the escape hatch, migrate the mechanical GitHub-release/`curl|sh`
  cases, keep the genuinely-imperative setup steps_ (version managers, `tpm` git-clone).
- **Two concrete backend capabilities block clean migration:** `{{.OS}}`/`{{.Arch}}`
  asset templating and version pinning — ubi/eget/mise auto-detect arch; `dotf`'s
  single-`sha256`-per-arch-explicit-asset `github-release` backend is the specific gap.
- **`dotf`'s narrow additive/native-format scope is vindicated by "Nix fatigue"
  discourse** converging on the pragmatic middle; and by chezmoi _declining_ scope-creep
  requests (`chezmoi mv` #3918). Guard against feature-envy — the leader's biggest current
  pain is onboarding/comprehension (#4955), not missing features.
- **Skip the magic that generates support load:** mise's top 2025 complaints are all
  shims/`activate`/PATH-integration pain. `dotf`'s explicit, non-magic install model
  should stay that way — and `update`/`upgrade` is **EDGE** (may collide with
  additive-only scope; `topgrade` already owns update-everything).
