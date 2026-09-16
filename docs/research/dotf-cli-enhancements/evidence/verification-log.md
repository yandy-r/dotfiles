# Evidence Verification Log

Orchestrator-run triangulation of the highest-stakes / disputed persona claims,
verified directly against the code on 2026-07-04.

## High-Confidence Findings (code-verified)

### F1. `Installed()` is a bare `LookPath(pkg.Name)` in 4 of 7 backends

- **Claimed by**: Contrarian, Systems Thinker, Archaeologist, Analogist, Negative Space
- **Verified**: `internal/backend/custom_script.go:49-55` AND `internal/backend/github_release.go:80-87` both do `b.r.LookPath(pkg.Name)`. `pkg.go:109-142` (the `pkg` backend) instead queries the package authority (`pacman -Q`/`dpkg-query`/`rpm -q`/`brew list`) — the correct technique already exists in-repo.
- **Status**: Confirmed. **Confidence: High.**
- **Impact**: `dotf status`/`dotf diff` misreport ≥8 custom-script tools whose `name` is a group label, not a binary (`aws-tools`, `azure-cli`→`az`, `hashicorp-tools`, `container-tools`, `rust`→`cargo`, `tpm`, `tmux-sessionizer`→`tms`, `gnu-utilities`). `fabric` can false-positive via a Python `fabric` collision.

### F2. `apply` never gates `Install()` on `Installed()`

- **Claimed by**: Systems Thinker (`install.go:96-104`, `applyphases.go:135-143`), Contrarian.
- **Verified (structurally consistent)**: install path calls `engine.Install` unconditionally; `Installed()` is referenced by `status.go` only. Every apply re-executes all scripts/downloads; rolling tools (mise/rust/atuin) self-update each run.
- **Status**: Confirmed. **Confidence: High.** apply is idempotent-by-script-discipline, not deterministic-by-engine.

### F3. `github-release` templating EXISTS but its vocabulary is too small (resolves the persona dispute)

- **Dispute**: brief + Archaeologist implied "no templating"; Negative Space corrected "it exists (`github_release.go:96-102`)."
- **Verified ground truth** (CORRECTED 2026-07-04 after Phase-3 Innovation agent flagged an error in the first draft): `templateGitHubAsset` (`github_release.go:96-102`) replaces `{{.OS}}` with `string(info.OS)` and `{{.Arch}}` with `info.Arch.ReleaseArch()`. BUT:
  - `info.OS` is a **distro-family** enum, not an OS-token enum: values are `macos`, `debian`, `redhat`, `arch`, `suse`, `linux`, `wsl`, `windows`, `unknown` (`platform/types.go:18-28`), and `platform.go:89-105` returns `OSDebian`/`OSRedHat`/`OSArch` on recognized Linux distros. So `{{.OS}}` renders `arch`/`debian`/`redhat`/`macos` on real machines — it **can never spell `darwin`**, and spells `linux` **only** as the unrecognized-distro fallback (never on a recognized Debian/Arch/Fedora host). A release asset named `foo_linux_amd64.tar.gz` is therefore **unspellable via `{{.OS}}`** on a normal Arch/Debian box. This is a MORE severe gap than the first draft stated.
  - `Arch.ReleaseArch()` returns only `amd64`/`arm64`/`arm` (`platform/types.go:56-70`) — **cannot spell `x86_64`/`aarch64`** (hadolint, rust, AWS, many vendors use these).
  - **Net**: a proper `ReleaseOS()` mapper (distro-family → `linux`/`darwin`) plus `x86_64`/`aarch64` arch variants are prerequisites; today `{{.OS}}` templating is effectively unusable for real GitHub assets, so the one shipped `github-release` example can only work by hardcoding the OS token in the asset string.
  - `SHA256` is a single string — **cannot carry a per-OS/arch checksum** for a templated asset (manifest comment at `packages.toml:184-185` admits this: "a single sha256 cannot cover {{.Arch}}-templated assets").
  - `archiveFormat` (`download.go`) supports only `.tar.gz`/`.tgz`/`.zip` — **no bare binary, no `.gz`, no `.pkg`/`.dmg`/AppImage**.
  - `findReleaseBinary` requires exactly one extracted file whose basename `== pkg.Name` — no per-OS binary-name override.
- **Status**: Confirmed with nuance. **Confidence: High.** The gap is real but is "vocabulary + per-arch checksum + archive/binary flexibility," not "templating is absent."

### F4. `script-url` starship pin is a rotating-installer time-bomb

- **Claimed by**: Contrarian (live-verified upstream hash drift), Historian, Futurist.
- **Verified (pin exists)**: `manifest/packages.toml:222-225` pins `sha256 = e9f0bb84…` for `https://starship.rs/install.sh`. starship is in the **default `base` intent** (`packages.toml:35`), so a hash rotation breaks the default `dotf apply` for every fresh machine (`script_url.go` fails closed on mismatch).
- **Status**: Pin confirmed in-repo; live upstream-drift asserted by Contrarian's fetch (not re-run offline here). **Confidence: High on the structural risk; Medium on the specific "broken today" hash** (re-verify with a live fetch before acting).

### F5. `apt-repo` hard-codes the `noble` codename

- **Verified**: `packages.toml:218` repo_line pins `…ubuntu noble stable`; the manifest comment says "Edit the codename to match the target Ubuntu release." No host-codename templating.
- **Status**: Confirmed. **Confidence: High.** Breaks on any non-24.04 Ubuntu and on Debian. No `rpm-repo` backend at all → Fedora/openSUSE second-class.

### F6. Doc drift: "~45-line shim" is actually 307 lines

- **Verified**: `wc -l bootstrap.sh` = **307**. CLAUDE.md and PRD (`prd.md:204`) describe "~45 lines." The manifest is ~37 packages; some docs call it a "7-package skeleton."
- **Status**: Confirmed. **Confidence: High.**

## Context-Dependent / Contradictions Requiring Resolution (handed to the Crucible)

### C1. "Retire custom-script" (user goal) vs "a permanent shell-out backend is correct design"

- **Position A** (user's request; PRD ≥80%-LOC metric): migrate the 19 scripts to native backends.
- **Position B** (Contrarian, Historian, Archaeologist, Analogist, Systems Thinker): chezmoi/Terraform/Ansible/Bazel/aqua all keep a governed escape hatch **forever**; ~9 of the 19 are genuinely-imperative setup-steps (tpm git-clone, tmux-sessionizer cargo-git, gnu-utilities brew-loop, mise/rust/lua rolling managers) that a native port would make worse.
- **Resolution direction**: migrate the ~5-10 mechanical single-binary/vendor-installer cases; reframe custom-script as a permanent, _governed_ adapter for the residual; re-scope the ≥80% metric to "custom-script ≤ ~5 entries" / per-tool delete-on-cutover. **This is a productive tension — preserve it.**

### C2. Lockfile: high value vs. real ongoing cost

- **Pro** (Analogist, Journalist, Futurist, Archaeologist): go.sum-style `dotf.lock` closes the supply-chain gap, enables version-aware `Installed()`, fixes script-url drift; matches aqua/mise convergence.
- **Con** (Systems Thinker, Contrarian): a lockfile _implies_ a Renovate/CI auto-bump pipeline (repo has none) and _collides_ with the rolling-release tools that intentionally float.
- **Resolution direction**: lockfile for pinned binary/script-url artifacts; rolling managers stay unpinned by design. Ship pinning + optional `dotf lock` before any Renovate automation.

### C3. `update`/`upgrade` verb — in scope or scope-creep?

- **Con** (Journalist): collides with additive-only stance; topgrade already owns "update everything"; mise's top complaints are update/shim pain.
- **Pro** (Negative Space): "day-2" verbs are the whole missing half of the tool.
- **Resolution direction**: `dotf self-update` (update the binary) is clearly in-scope and cheap; `dotf update` (update installed _tools_) is EDGE — defer or scope narrowly to rolling managers.
