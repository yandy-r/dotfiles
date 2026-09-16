# Negative Space — What the Research Itself Hasn't Resolved

_Negative Space Analyst (Phase 3, emergent-insight). Asymmetric Research Squad.
Date: 2026-07-04. Inputs: all 8 persona findings, `crucible-analysis.md`,
`contradiction-mapping.md`, `evidence/verification-log.md`, plus direct reads of the
`dotf` code (`manifest/packages.toml`, `internal/backend/{github_release,toolchain,download}.go`,
the 19 `bin/install-*.sh`, `git remote -v`, `.github/workflows/`)._

> **My distinct job** (different from the Phase-1 negative-space persona, which mapped
> missing _features/commands/schema_): map what the **research corpus** still leaves
> unresolved — the decisions the maintainer must make before acting, the artifact the
> squad never actually produced, and the subsystems **no persona looked at**.

---

## Executive Summary

The eight personas + crucible produced an unusually strong consensus on **direction**
(fix `Installed()`/probe first; triage the escape hatch; the metric is mis-shaped). But
that consensus **hides four unresolved things** that block the maintainer from starting:

1. **The migration table was never actually built.** Every persona _asserted_ a count
   ("keep ~5, migrate ~12–14" — Futurist; "keep ~9, migrate ~10" — Systems/Historian),
   but the counts disagree ~2× (C11) precisely because **nobody enumerated all 19 tools
   with a concrete target backend + the specific capability each needs + a keep/migrate
   verdict.** I built that table below (grounded in the scripts + the backend code). When
   grounded in the _actual_ backend constraints, the count resolves toward the
   **conservative** end: **only ~4–5 tools are mechanically portable with today's
   backends, ~5 more unlock only after 3–4 new capabilities land, and ~9 are genuine
   setup-steps or blocked.** The Futurist's optimistic "migrate 12–14" assumed
   `github-release` can absorb them — the code proves it cannot without new capabilities.

2. **The single zero-capability migration path everyone missed is the `toolchain` (npm)
   backend — which has never been exercised.** `codex` (`@openai/codex`), `claude`
   (`@anthropic-ai/claude-code`), `opencode` (`opencode-ai`), and `lefthook` are all
   npm-installable **today** via `names = { npm = "…" }` (`toolchain.go:36–41`) with **no
   new backend capability at all**. No persona flagged this — they all defaulted to
   "`github-release` needs `{{.Arch}}` vocab first." But `toolchain` has **0 production
   uses** in the manifest, so this "easy" path is completely untested and carries a hidden
   `node`/`npm` dependency the manifest doesn't declare. This is a _decision_ (accept an
   npm dep + lose the scripts' brew-first cascade?), not a capability gap.

3. **Renovate is prescribed by four personas as the answer to the lockfile maintenance
   tax — but nobody verified the repo can run it.** `origin` is a **self-hosted Forgejo on
   Tailscale** (`git.example-tailnet.ts.net`); `github` is a **push-mirror**; CI is GitHub
   Actions on the mirror (`.github/workflows/{go,release}.yml`); there is **no
   `renovate.json`/`dependabot.yml`**. The prescribed cure may not attach to the forge
   that is the source of truth. This gates the entire "lockfile → Renovate" half of the
   roadmap (H3-automation) and no one closed it.

4. **Half of what `dotf` _does_ got ~zero scrutiny.** All eight personas fixated on the
   package/backend layer. **No persona examined the symlink engine's own correctness, the
   `sysconfig` subcommand, the `completion` command, the macOS execution path, the
   uninstall/rollback story beyond snapper, or the overlap between `dotf apply` and the
   _shell config the same repo ships_ (`common/tools.sh` has its own `system_health` /
   `list_missing_tools` — a shell-side `doctor` that duplicates the proposed `dotf
doctor`).** These are shared blind spots, not gaps any one persona owns.

The through-line: **the corpus is deep on the backend read/write path and shallow
everywhere else, and the one artifact the maintainer most needs — a per-tool migration
table with confidence flags — was the one thing nobody produced.**

---

## Decision-Blocking Unknowns (ranked; each resolvable in < 1 day)

Ranked by "does this gate the first PR / the shape of the schema?"

### 1. Is the starship pin actually stale on the maintainer's network _right now_? (BLOCKS: hotfix-vs-rider)

- **Why it blocks:** if the live hash ≠ pinned `e9f0bb84…`, the **default** `dotf apply`
  is failing SHA256 on every fresh machine today (starship is in `base`,
  `packages.toml:35,222–225`) — which promotes the starship/apt hardening from a Phase-1
  _rider_ to an **immediate hotfix**. The Contrarian's `52c64f14…` was one sandboxed fetch,
  self-flagged, never independently re-run (F4, C10).
- **Resolve (< 5 min):** `curl -fsSL https://starship.rs/install.sh | sha256sum` on the
  maintainer's network. Match → armed-not-detonated (Phase-1 rider). Mismatch → hotfix now.
  Either way the fix is identical: pin a versioned/immutable asset **or** demote to a
  tolerant shell-out.

### 2. Which of the 19 is _actually_ mechanical? (BLOCKS: the whole plan's scope)

- **Why it blocks:** the crucible's Phase-3 "migrate vertically" and the metric reframe
  ("custom-script ≤ ~5") both presuppose a number nobody has. C11 shows a **2× spread**
  (keep-5 to keep-9) because "mechanical" floats. You cannot re-scope the metric to "≤ N"
  without knowing N.
- **Resolve (< 1 day):** the table in this doc is the first pass. Confirm it by **porting
  exactly one tool end-to-end** (lefthook via `toolchain`-npm — the lowest-risk case) with
  a throwaway branch, prove it installs, and calibrate the effort estimate. That single
  spike converts the whole table from asserted to measured.

### 3. Can the repo even run Renovate given Forgejo-origin / GitHub-mirror? (BLOCKS: lockfile-automation half)

- **Why it blocks:** four personas (Analogist S2, Journalist, Futurist, Historian FA#3)
  make Renovate the answer to "pins rot." Systems' C2 warns the lockfile's _true cost_ is
  the Renovate pipeline it implies. If Renovate can't attach to the Tailscale Forgejo (the
  actual PR/CI surface per memory) and the GitHub side is a downstream mirror, the
  prescribed cure has no home.
- **Resolve (< 1 day):** decide the authority (Forgejo vs GitHub-mirror). Renovate
  _does_ support Forgejo as a platform but needs a **self-hosted Renovate runner** with a
  token against the Tailscale host; or accept GitHub-mirror as the automation surface and
  reconcile the mirror-direction. A 2-hour spike (point self-hosted Renovate at the Forgejo
  repo, see if it opens a PR) settles whether the lockfile→Renovate story is real or fiction.

### 4. Does one `probe`/`Binary`/`Provides` field cleanly express multi-binary bundles? (BLOCKS: schema shape)

- **Why it blocks:** the keystone field (6-of-8 convergence) has an unshaped corner:
  `hashicorp-tools` installs terraform+vault+packer, `aws-tools` installs aws+sam+ecs-cli,
  `container-tools` installs skaffold+hadolint (+podman/docker) — all under **one**
  `pkg.Name`. If `provides = ["terraform","vault"]` (all-present ⇒ installed) doesn't work
  cleanly, the manifest must **split bundles into per-tool entries**, which changes the
  intent lists and the metric denominator. (Systems + Negative-Space + crucible all flag
  this; nobody prototyped it.)
- **Resolve (< 1 day):** prototype `Provides []string` on `hashicorp-tools` behind the
  new `Installed()` gate; decide all-present vs any-present semantics before committing the
  field.

### 5. What are the real per-tool binary names? (BLOCKS: per-PR verification in Phase 3)

- **Why it blocks:** `fabric` collides with Python's pyinvoke `fabric` (false-positive
  risk, F1); `tea`/`coderabbit`/`turso`/`opencode`/`codex` binary names were never
  byte-verified by any persona (Contrarian + Negative-Space both caveat this). A wrong
  `Binary` value silently re-breaks `status`.
- **Resolve (< 1 hr):** `command -v` each on a provisioned host, or read the release pages.
  I confirmed from the scripts: `codex`→`codex`, `opencode`→`opencode`, `coderabbit`→
  `coderabbit`, `turso`→`turso`, `fabric`→`fabric`**or**`fabric-ai` (collision — needs a
  version-string probe, not bare `LookPath`).

### 6. Build auto-detect natively vs vendor a library — and what's the edge-case tail? (BLOCKS: Phase-2 keystone choice)

- **Why it blocks:** Analogist's keystone (ubi/eget auto-detect, S1) is either the
  Phase-2 opener (if the musl/`.exe`/multi-artifact tail is cheap) or a later convenience
  (if expensive). mise's `ubi`→`github` deprecation says _own it in-tree_ but nobody
  estimated the code cost of reimplementing the OS→arch→libc→tie-break pipeline.
- **Resolve (< 1 day):** a code spike — prototype the filter pipeline against 5 real
  release pages (turso, lefthook, hadolint, tea, coderabbit) and count the special cases.

### 7. Is `apply`'s re-download-every-run actually biting, or latent? (AFFECTS: sell, not build)

- **Why it's lower:** the idempotency gate's _honest-status_ payoff stands regardless
  (F1/F2). This only changes how loudly to justify the gate. Opt-in intents are rarely
  re-run, so the network cost may be theoretical.
- **Resolve (< 1 hr):** time a second `dotf apply --intent cloud` on a converged host.

---

## The Per-Tool Migration Table (all 19, grounded + confidence-flagged)

**This is the artifact the corpus never produced.** Columns: tool → most-likely target
backend → the specific capability it needs → keep-as-script? → confidence. Grounded in the
scripts and in the backend code (`github_release.go`, `toolchain.go`, `download.go`).

**Legend for "Keep?":** **N** = migrate (mechanical); **Y** = genuine setup-step, keep
forever; **P** = partial (migrate the mechanical slice, keep/drop the residual);
**?** = uncertain, needs a per-tool call.

| #   | Tool (intent)                 | Most-likely target backend                                                         | Capability it needs (that doesn't exist today)                                                                                                                          |               Keep?               |  Conf.   |
| --- | ----------------------------- | ---------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- | :-------------------------------: | :------: |
| 1   | **lefthook** (git-tools)      | `toolchain` (npm `lefthook`) **or** `github-release` (`.gz`)                       | **npm path: NONE — works today.** github-release path: `.gz` single-binary + `{{.Version}}` in asset                                                                    |               **N**               |   High   |
| 2   | **codex** (ai)                | `toolchain` (npm `@openai/codex`)                                                  | **NONE — works today** (`names={npm="@openai/codex"}`); needs `node` present                                                                                            |               **N**               | Med-High |
| 3   | **claude** (ai)               | `toolchain` (npm `@anthropic-ai/claude-code`)                                      | **NONE — works today**; needs `node`; loses vendor-installer fallback                                                                                                   |               **N**               |   Med    |
| 4   | **opencode** (ai)             | `toolchain` (npm `opencode-ai`) **or** `github-release` (sst/opencode)             | npm path: **NONE today**; gh path: bare-binary + arch vocab                                                                                                             |               **N**               |   Med    |
| 5   | **turso** (cloud)             | `github-release` (tursodatabase/turso-cli)                                         | **bare-binary asset** (`turso-linux-amd64`, no archive) + **`.sha256` sidecar lookup** + **`darwin` OS token** (script uses `turso-darwin-arm64`)                       |               **N**               |   Med    |
| 6   | **tea** (git-tools)           | `github-release` w/ **non-GitHub host** (gitea.com)                                | **arbitrary release host** + **`checksums.txt` line-lookup** (not one pinned sha) + completion post-step                                                                |               **P**               |   Med    |
| 7   | **container-tools** (cloud)   | `github-release` (bare) — hadolint + skaffold                                      | **bare-binary** + **`{{.OS}}`=`Linux`/`Darwin` casing** + **`{{.Arch}}`=`x86_64`** + arbitrary URL (skaffold = Google Storage, not GH) + post-install `--version` check |               **P**               |   Med    |
| 8   | **hashicorp-tools** (cloud)   | `apt-repo`+**`rpm-repo`** (terraform) + `github-release` `.zip` (tflint)           | **`rpm-repo` backend** + **GPG-fingerprint verify** + **multi-tool `provides`** (or split terraform/tflint)                                                             |               **P**               |   Med    |
| 9   | **aws-tools** (cloud)         | mixed: `.pkg`(mac) / `.zip`+vendor-`install`(Linux) / pip(sam) / bare-bin(ecs-cli) | **`.pkg` installer**, **run vendor `install` in archive**, **pip/pipx backend**, **bare-binary** — 3–4 distinct gaps in one script                                      | **Y** (split off ecs-cli = **P**) |   High   |
| 10  | **azure-cli** (cloud)         | `apt-repo`+`aur`+**`rpm-repo`**+`pkg`(brew) — per-OS                               | **`rpm-repo`** + **per-OS backend dispatch for one logical package**                                                                                                    |               **P**               |   Med    |
| 11  | **lua** (runtimes)            | `pkg` (multi-name `lua5.1`/`lua51`/`lua@5.1`) + luarocks companion                 | **companion-package** under one entry (already has multi-`names`); drop source-build fallback                                                                           |               **P**               |   Med    |
| 12  | **fabric** (ai)               | `github-release` (danielmiessler/fabric) **or** NEW go-toolchain                   | **`go install` toolchain** (toolchain does only cargo/npm) **or** bare-binary+arch vocab; **pyinvoke name-collision** breaks `LookPath` → needs version-probe           |               **?**               |   Low    |
| 13  | **coderabbit** (ai)           | `script-url` (cli.coderabbit.ai/install.sh) — **rotating**                         | Same **rotating-installer pin trap** as starship; also **depends on `claude`** (requires-edge). Pinnable only if they ship release assets                               |          **?** (leans Y)          |   Low    |
| 14  | **atuin** (dev)               | `pkg`/`github-release` for the **binary**; keep the **setup**                      | binary is mechanical (in distro repos + atuinsh/atuin releases); **post-install**: config-write + `atuin import auto` + shell-cleanup are the residual                  |               **P**               |   Med    |
| 15  | **mise** (runtimes)           | keep — `setup-step` tier                                                           | rolling version manager; 4-way cascade + self-update; a native port makes it _worse_                                                                                    |               **Y**               |   High   |
| 16  | **rust** (runtimes)           | keep — `setup-step` tier                                                           | rustup rolling self-update + extra-crates loop; `script-url`+post-steps can't hold rolling semantics                                                                    |               **Y**               |   High   |
| 17  | **tpm** (system)              | NEW `git-clone` backend                                                            | **`git-clone` backend** (repo/ref/depth/dest + `installed_when={path}`); **no binary at all**                                                                           |      **Y** (via new backend)      |   High   |
| 18  | **tmux-sessionizer** (system) | `toolchain` (cargo) — needs `--git`                                                | **`cargo install --git URL`** (toolchain does only `cargo install <name>`); binary `tms`≠crate; **requires `rust`** first                                               |       **Y** (via cargo-git)       |   High   |
| 19  | **gnu-utilities** (system)    | `pkg` (brew) multi-package, `os=macos`                                             | **multi-package-per-entry** + **per-package `os` gating** (only symlinks have `os` today)                                                                               |               **P**               |   Med    |

### The headline the corpus never stated plainly

Bucketed by what it _actually_ takes (grounded in the backend code, not optimism):

- **Mechanical with TODAY's backends (0 new capability): ~4–5** — lefthook, codex, claude,
  opencode (all via the **never-yet-used `toolchain`-npm** path), + the pkg-slice of lua.
  **Every one carries a caveat** (a `node` dependency the manifest doesn't declare, or the
  loss of the scripts' brew-first cascade). None is a _free_ win.
- **Mechanical only AFTER 3–4 new capabilities** (bare-binary asset, richer `{{.OS}}`/
  `{{.Arch}}` vocab incl. `darwin`/`x86_64`, `checksums.txt`/sidecar lookup, non-GitHub
  host, `rpm-repo`): **~5** — turso, tea, container-tools, the tflint slice of
  hashicorp-tools, the ecs-cli slice of aws-tools.
- **Genuine setup-steps / blocked (keep-as-script): ~9** — mise, rust, tpm,
  tmux-sessionizer, gnu-utilities, aws-tools (bundle/`.pkg`), azure-cli (until `rpm-repo` +
  per-OS dispatch), fabric (`go install` + name-collision), coderabbit (rotating installer
  - `claude` dep), plus the setup-half of atuin.

**So C11 resolves toward the conservative Historian/Systems end (keep ~9), not the
Futurist end (migrate 12–14)** — because the optimistic count assumed `github-release`
could absorb them and the code (`templateGitHubAsset` → `linux`/`macos`+`amd64`/`arm64`
only; `archiveFormat` → `.tar.gz`/`.zip` only; `findReleaseBinary` → basename==`name`)
proves it cannot. **The realistic first-wave delete count is ~4–5, all via one untested
backend.**

---

## Shared Blind Spots (what NO persona examined)

The eight personas triangulated the backend read/write path from six angles and **all
skipped the same territory**. These are systemic blind spots, not any one persona's gap.

1. **The symlink engine's own correctness.** `dotf link` (`internal/symlink/`) is _half_
   of what dotf does and received ~0 scrutiny. Archaeologist praised Stow's ownership model
   - two-phase conflict pre-scan as a _revival idea_ but **never audited dotf's actual
     symlink code** for whether it has them. Reading it: there is **no ownership predicate**
     (is this link _mine_?), the "Zed exception" silently `unlink`s a legacy symlink then
     _copies_ (`symlink.go:43`, `apply.go:102`), and Contrarian noted a plain `dotf apply`
     can replace a real file with a symlink **without a backup** unless `--linux-desktop`/
     `--force` forces it (`applyphases.go:205–213`). Nobody examined the backup/clobber safety
     of the default path — a chezmoi-style "first apply is destructive" footgun is plausible
     here and **unaudited**.

2. **`dotf sysconfig` — completely unexamined.** A whole subcommand + `internal/sysconfig`
   (hostname/timezone/locale/shell/firewall, Windows = `ErrNotImplemented` stub). Not one
   persona proposed a fix or enhancement. The firewall double-pass (running the sysconfig
   engine twice, re-zeroing identity fields, `applyphases.go:218–242`) is touched only as
   an _apply-ordering_ complexity by Systems — never as a sysconfig concern. Locale/shell/
   firewall idempotency, macOS sysconfig behavior, and the Windows stub are all dark.

3. **`dotf completion` — mentioned as "exists," never examined.** CLAUDE.md itself names
   "dynamic always-in-sync completion" as **the one reason to reopen the stdlib-flag-vs-
   framework decision** — and no persona explored whether the static completion drifts from
   the flag set, or whether per-command flag completion is missing. The completion command
   is the tool's most likely _future_ pressure point and got zero coverage.

4. **The macOS execution path is asserted, never exercised.** Everyone says "`darwin`≠
   `macos`" for asset templating, but **nobody traced the macOS provisioning path**: brew
   bootstrap on Apple Silicon (`/opt/homebrew`) vs Intel (`/usr/local`), the `.pkg`/`.dmg`
   gap (aws-tools, codex-cask), gnu-utilities being macOS-only, sudo/privilege differences,
   the `os=macos` aerospace symlink. The whole macOS half of a cross-platform tool is
   claimed by inference from the OS enum, not by execution.

5. **Uninstall / rollback beyond snapper.** Snapper is Linux/btrfs-only, best-effort,
   non-fatal (`applyphases.go:25–34`). **Nobody examined recovery on macOS, on ext4 Linux,
   or after a partial mid-apply failure.** The failure collector lets a run _complete_, but
   what state is the machine in when package 7 of 12 fails and 8–12 still run? No persona
   traced it. `dotf unlink` is absent (Negative-Space noted it; nobody examined the engine's
   capacity to safely reverse a link set).

6. **Real-distro testing — cited as absent, never scoped.** Everyone notes the `exec.Fake`
   mock proves _argv_, not _execution_, and that there's no dockertest/nightly. But **nobody
   asked which migration-table claims are _unfalsifiable_ without it.** Every "this becomes
   mechanical" row above is an argv-level assertion; whether the ported entry actually
   installs on Fedora/openSUSE/Arch/macOS is untested. The migration's success criteria are
   structurally unverifiable in current CI — a bigger problem than any single missing test.

7. **`dotf apply` vs the shell config the _same repo_ ships.** The largest blind spot.
   dotf provisions tools **and** symlinks `common/*.sh`, `bash/`, `zsh/` — and those rc
   files run their _own_ tool detection, PATH construction, lazy-loading, and even install
   helpers (`common/tools.sh` has **`system_health`** and **`list_missing_tools`** — a
   shell-side `doctor` that duplicates the proposed `dotf doctor`!). No persona connected
   the two systems: `github-release`/`toolchain` install to `~/.local/bin`, and whether
   that's on PATH depends on `common/paths.sh`; but `apply` runs in a non-login shell, so
   the `Installed()` PATH-probe may see a _different_ PATH than the user's shell. The
   provisioning tool and the provisioned shell config are two overlapping systems and the
   corpus treated dotf as if it existed alone.

8. **Onboarding for a stranger — end-to-end, on a non-Ubuntu box.** Negative-Space noted
   the README teaches the legacy path, but **nobody traced the actual first-run of a
   stranger on Fedora**: `bootstrap.sh` → `dotf apply` → F5 (no `rpm-repo`, hardcoded
   `noble`) means docker-ce is skipped and azure/aws fall to scripts. The stranger-on-Fedora
   default path may be materially degraded and **no persona walked it**.

9. **`internal/privilege` on a no-sudo host.** Negative-Space listed "multi-user/non-root"
   as an _undiscussed silence_ but **no persona examined the privilege code's behavior**
   when sudo is absent — a locked-down corporate host is a real stranger scenario.

---

## Untested / Unverifiable Claims Needing a Spike

Ranked by how load-bearing the claim is to the roadmap.

| #   | Claim (source)                                                                      | Why it's untrusted                                                                     | Spike to settle it                                                                         |
| --- | ----------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| 1   | starship is broken on the default path **today** (Contrarian)                       | one sandboxed `curl` fetch, self-flagged, never re-run (F4/C10)                        | re-fetch the hash on the maintainer's network (< 5 min)                                    |
| 2   | codex/claude/opencode/lefthook migrate cleanly via `toolchain`-npm (**my** finding) | `toolchain` has **0 production uses**; never exercised; hidden `node` dep              | add one npm entry, run `dotf install --only lefthook` (once `--only` exists) on a real box |
| 3   | "~14 of 19 blocked on 5 capabilities" (Archaeologist) vs "migrate 12–14" (Futurist) | 2× disagreement; nobody built+tested the table                                         | port **one** tool end-to-end (lefthook), measure the real effort                           |
| 4   | `provides = [...]` cleanly handles multi-binary bundles (Systems/crucible)          | never prototyped; schema shape hangs on it                                             | prototype `Provides` on hashicorp-tools                                                    |
| 5   | ubi/eget auto-detect is a cheap Go reimplementation (Analogist)                     | edge-case tail (musl/`.exe`/multi-artifact) never estimated                            | code spike against 5 real release pages                                                    |
| 6   | Renovate solves the pin-rot tax (Analogist/Journalist/Futurist/Historian)           | repo is Forgejo-origin + GitHub-mirror; **no Renovate config**; runnability unverified | point self-hosted Renovate at the Forgejo repo, see if it opens a PR                       |
| 7   | the migrated entries actually install on Fedora/macOS (whole table)                 | argv mocks prove intent, not execution; no dockertest/nightly                          | a minimal dockertest matrix for the _first_ ported tool                                    |
| 8   | `fabric` `Installed()` is reliable                                                  | pyinvoke `fabric` collision → false-positive; untested                                 | run status on a host with Python fabric present                                            |
| 9   | the default `dotf apply` symlink phase never clobbers a real file unsafely          | symlink engine backup/ownership behavior unaudited (Contrarian flag)                   | run `dotf apply` (no flags) over a dir with a real `~/.config/nvim`                        |
| 10  | `apply` re-download-every-run is a real cost                                        | opt-in intents rarely re-run; may be latent (Systems)                                  | time a second converged `apply --intent cloud`                                             |

---

## Research Gaps by Category

- **Codebase-coverage gaps.** Deep: `internal/backend/*`, the apply pipeline, `Installed()`.
  Shallow-to-absent: `internal/symlink`, `internal/sysconfig`, `cmd/dotf/completion*.go`,
  `internal/privilege`, `internal/brew` (PATH-mutation/re-detect correctness), `internal/tui`
  (the selection model that could feed `--only` cheaply), `internal/snapper` (rollback
  semantics). ~Half the binary is unexamined.
- **Comparator gaps.** Nobody compared dotf's **symlink engine** to Stow/chezmoi in depth
  (only as a revival idea); nobody checked how chezmoi/mise handle the **`.pkg`/`.dmg`/macOS**
  case the aws/codex scripts need; nobody looked at whether aqua/mise **run a real-distro CI
  matrix** (a precedent for what dotf's testing gap would cost).
- **Verification gaps.** The entire migration table is argv-level; the `toolchain`-npm path
  is untested; the multi-binary schema is unprototyped; the Forgejo/Renovate fit is
  unverified; macOS is never executed. (See spike table.)
- **Scope-boundary gaps.** The `dotf apply` ↔ shipped-shell-config overlap; the
  stranger-on-Fedora default path; no-sudo hosts; the shell-side `system_health` that
  duplicates `dotf doctor`. The corpus drew dotf's boundary at "the Go binary" and never
  examined that the same repo ships a second, overlapping provisioning surface in bash.

---

## Key Insights

1. **The one artifact the maintainer most needs — a per-tool migration table — is the one
   thing the squad never produced.** Every persona asserted a count; nobody enumerated all
   19 with a concrete backend + capability + verdict. Grounded in the code, the count is
   **more conservative** than the optimists claimed: ~4–5 mechanical now, ~5 after new
   capabilities, ~9 keep. C11's 2× spread collapses toward "keep ~9" once you read
   `github_release.go` instead of hoping.

2. **The only zero-capability migration path is the backend nobody looked at.** `toolchain`
   (npm) can move lefthook/codex/claude/opencode **today** — no new capability — yet it has
   0 production uses and every persona defaulted to the capability-first `github-release`
   story. The migration is _less blocked than the "capability-first" narrative implies_, but
   only along an untested axis with a hidden `node` dependency. This is a **decision**
   (accept npm + lose the cascade?), not an engineering gap.

3. **The consensus keystone (`Installed()`/probe) has an unshaped corner the consensus
   skipped:** multi-binary bundles. 3 of the 19 (hashicorp/aws/container) install several
   binaries under one name. Whether `Provides []string` holds them or forces a bundle-split
   determines the schema _and_ the metric denominator — and it was flagged but never
   prototyped.

4. **Four personas prescribed a cure (Renovate) for a forge nobody confirmed it fits.** The
   repo's real source of truth is a self-hosted Forgejo on Tailscale; the GitHub side is a
   mirror; there's no Renovate config. The lockfile→Renovate half of the roadmap rests on an
   unverified platform assumption.

5. **The corpus is deep on the package layer and blind on the other half of the tool.** The
   symlink engine's clobber-safety, `sysconfig`, `completion`, macOS execution, rollback,
   and — most tellingly — the overlap with the **shell config the same repo ships** (which
   has its own `system_health` doctor) were examined by no one. A roadmap built only on the
   backend findings would harden half the tool and never look at the other half.

6. **The migration's success criteria are structurally unverifiable in current CI.** Every
   "becomes mechanical" claim is an argv-mock assertion; there is no real-distro execution,
   so a ported entry that _passes tests_ still isn't proven to install on Fedora/macOS. The
   probe field fixes _verification of state on a host you can run on_ — it does **not** fix
   the fact that the host matrix itself is never exercised. That gap sits underneath the
   whole plan and no persona surfaced it as the top-level risk it is.

---

## Digest (lead: must-resolve-before-acting + the migration-table headline)

- **Re-fetch the starship hash first (< 5 min).** It's the single measurement that decides
  whether starship/apt hardening is an immediate hotfix (default `apply` failing on every
  fresh box) or a Phase-1 rider. Everything else can wait behind it; this can't.

- **Migration-table headline: of the 19, only ~4–5 are mechanical _today_, ~5 more need
  3–4 new backend capabilities, and ~9 are genuine keep-as-script setup-steps.** The
  research's 2× disagreement (C11) resolves toward the **conservative** end once grounded in
  the actual backend code — the "migrate 12–14" optimism assumed a `github-release` backend
  that provably can't spell `darwin`/`x86_64`, unpack a bare binary/`.gz`, or hit a non-GitHub
  host. **Realistic first-wave delete count: ~4–5.**

- **Those ~4–5 all ride the one backend nobody examined — `toolchain` (npm), which has 0
  production uses.** lefthook/codex/claude/opencode are npm-installable today via
  `names={npm=…}` with no new capability — but that path is untested and drags in an
  undeclared `node` dependency. This is a _decision_ (npm dep vs the scripts' brew-first
  cascade), not a capability gap, and no persona named it.

- **Prove the plan by porting exactly ONE tool end-to-end (lefthook) before re-scoping the
  metric.** That single spike converts the whole asserted table into a measured one and
  calibrates the "custom-script ≤ N" target the crucible wants but can't number.

- **Renovate — the prescribed cure for pin-rot — has an unverified home.** origin is a
  self-hosted Forgejo on Tailscale; GitHub is a mirror; there's no Renovate config. Settle
  Forgejo-vs-mirror authority before committing to "lockfile ⇒ Renovate," or the whole
  automation half is fiction.

- **Prototype `provides = [...]` on a multi-binary bundle before freezing the schema.**
  hashicorp/aws/container install several binaries under one name; whether the keystone probe
  field holds them or forces a bundle-split changes both the schema and the metric.

- **Half the tool is a shared blind spot.** No persona examined the symlink engine's
  clobber-safety, `sysconfig`, `completion`, the macOS execution path, rollback beyond
  snapper, or the overlap with the shipped shell config (which has its own `system_health`
  doctor duplicating the proposed `dotf doctor`). A backend-only roadmap leaves these dark.

- **The migration is unverifiable in current CI.** argv mocks prove intent, not execution;
  no dockertest/nightly means every "becomes mechanical" row is unfalsifiable on a real
  Fedora/macOS host. The probe field verifies state _where you can run_; it does not close
  the untested host-matrix gap sitting under the entire plan.
