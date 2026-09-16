# The Archaeologist — Buried Wisdom & Revivable Old Solutions

**Persona:** The Archaeologist (Asymmetric Research Squad)
**Subject:** Features/enhancements/fixes for the `dotf` CLI, incl. migrating `bin/install-*.sh` → native TOML-manifest backends
**Date:** 2026-07-04
**Two digs:** (a) obsolete provisioning approaches worth reviving; (b) the hard-won platform knowledge already encoded in dotf's 29 `bin/install-*.sh` scripts that a naive Go port would discard.

---

## Executive Summary

The central tension the user names — `custom-script` is the "shrinking escape hatch" that instead became the _most-used_ backend (19/37 packages) — is not primarily a laziness problem. It is a **capability-deficit problem**. I read the actual backend Go and the actual scripts, and the scripts encode concrete platform knowledge that the current 7 backends **structurally cannot express**. You cannot retire a script until the backend that would replace it gains the capability the script encodes. This report inventories those capabilities per script.

Three findings dominate:

1. **The `github-release` backend is far weaker than the scripts it would replace.** It (a) can only install **one binary per package** whose filename **exactly equals** `pkg.Name` (`github_release.go:124-146`); (b) only understands `.tar.gz`/`.tgz`/`.zip` archives — **not bare binaries, not `.gz`-compressed single binaries, not `.pkg`/`.dmg`/`.msi`/AppImage** (`download.go:104-114`); (c) its `{{.Arch}}` token can only ever render `amd64`/`arm64`/`arm` — **never** `x86_64`/`aarch64` (`types.go:56-66`), which several vendors require; and (d) its `{{.OS}}` token renders the **distro family** (`debian`/`arch`/`redhat`), **not** the release token `linux`/`darwin` (`github_release.go:96-102` + `platform/types.go:19-27`). Half the cloud/container scripts use vendor asset vocabularies this template literally cannot produce.

2. **The "is it installed?" heuristic is a regression waiting to happen — except in one backend that already got it right.** Four backends (`github-release`, `script-url`, `toolchain`, `custom-script`) decide "installed" with a bare `LookPath(pkg.Name)` (`github_release.go:81-87`, `script_url.go:55-66`, `toolchain.go:74-82`, `custom_script.go:49-55`). The `pkg` backend, by contrast, already does the _archaeologically-correct_ thing: it queries the package database (`pacman -Q`, `dpkg-query -W -f=${db:Status-Status}`, `rpm -q`, `brew list`) and even distinguishes `installed` from `config-files` (removed-but-not-purged) states (`pkg.go:109-142`). The old, robust "ask the authority, don't probe PATH" technique is already in the codebase — it just needs to spread.

3. **A handful of scripts are genuinely NOT package installs and should never be forced into a binary-download backend.** `tpm` is a `git clone` into `~/.tmux/plugins/tpm` (`install-tpm.sh:30-33`); `gnu-utilities` is a macOS-only Homebrew formula loop (`install-gnu-utilities.sh:17-42`); `tmux-sessionizer` is `cargo install --git <url>` (`install-tmux-sessionizer.sh:43`) which the toolchain backend **cannot express** (no `--git`, `toolchain.go:48-57`); `atuin`/`mise`/`rust` are rolling-release version managers with self-update semantics and post-install config/history-import steps. These are "setup steps," and the right archaeological move is to give the manifest a **first-class `git-clone` and `post-install step` vocabulary** rather than pretend they are packages.

The revival thesis: dotf should reclaim **four old ideas** — (1) chezmoi/CFEngine-style **content-hash run-once** state (replace `LookPath` with a stamp/hash ledger), (2) **aqua-style checksum lockfile** for the binary backends (fixes the "sha256 pins one arch" trap), (3) GNU Stow's **ownership + two-phase conflict scan** for the symlink engine, and (4) Ansible's **`creates:` sentinel** pattern as the manifest's idempotency primitive for setup-steps. None require a framework; all fit the stdlib-flag + TOML + Go scope.

---

## Buried Wisdom in dotf's Own Scripts

Per-script excavation. For each: the platform knowledge it encodes, and the **capability a native backend must gain to replace it**. Backend-capability gaps are the load-bearing column — they are the reason the migration stalled.

### Cross-cutting knowledge encoded in nearly every script

- **Arch vocabulary translation is per-vendor, not universal.** `install-mise.sh:106-113` maps to `x64`/`arm64`/`armv7`; `install-tea.sh:66-72` maps `x86_64|amd64`→`amd64`, `arm64|aarch64`→`arm64`; `install-hashicorp-tools.sh:246-251` maps `x86_64`→`amd64`, `aarch64`→`arm64`; `install-container-tools.sh:200-204` (hadolint) keeps `x86_64` **as-is** and only maps `aarch64`→`arm64`; `install-aws-tools.sh:159-187` needs `x86_64`/`aarch64` **literally** in the URL. **Five scripts, four different arch vocabularies.** dotf's single `ReleaseArch()` (`amd64`/`arm64`/`arm` only, `types.go:56-66`) cannot express three of them.
- **OS token casing/spelling is per-vendor.** hadolint wants `Linux`/`Darwin` capitalized (`install-container-tools.sh:194-198`); tea/terraform/ecs want `linux`/`darwin` lower (`install-tea.sh:133-136`, `install-hashicorp-tools.sh:240-244`); AWS macOS pkg has no OS token at all (`install-aws-tools.sh:133-155`). dotf's `{{.OS}}` renders `debian`/`arch`/`redhat`/`macos` — matching **none** of these.
- **Latest-version resolution via API.** hashicorp/tflint/hadolint/ecs-cli/lefthook/tea all `curl` a `releases/latest` JSON/`checksums.txt` and `grep` the tag (`install-hashicorp-tools.sh:231,290`; `install-tea.sh:112-118`). The github-release backend has **no `latest` tag resolution** — it either pins a `tag` or hits the `releases/latest/download/` redirect, and **cannot pin a sha256 to a floating latest** (validation forbids it, `github_release.go:106-114`). So "track upstream latest AND verify checksum" is inexpressible.
- **`sudo` vs `~/.local/bin` fallback.** Every binary installer probes `[[ -w "$INSTALL_DIR" ]]` and falls back to `sudo mv` for `/usr/local/bin`, or targets `~/.local/bin` (`install-hashicorp-tools.sh:270-274`, `install-tea.sh:151`). github-release **hard-codes** `~/.local/bin` (`github_release.go:29-35`) — fine for user tools, but it silently changes install location vs. the scripts' `/usr/local/bin`, and offers no system-wide option.

### Per-script capability table

| Script (file:line)                          | What it encodes                                                                                                                                                                                                                          | Native backend that _could_ replace it         | **Capability the backend must gain first**                                                                                                                                                  | Confidence                                 |
| ------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------ |
| **install-tpm.sh** (`:20-33`)               | `git clone --depth 1` into `~/.tmux/plugins/tpm`; idempotent on dir-present; skips if no git                                                                                                                                             | _none today_                                   | **New `git-clone` backend**: repo, ref/depth, dest dir, dir-exists idempotency                                                                                                              | High                                       |
| **install-gnu-utilities.sh** (`:17-42`)     | macOS-only; loops 14 Homebrew formulae; `brew list` idempotency; no-op off-macOS                                                                                                                                                         | `pkg` (homebrew)                               | **Multi-package-per-entry** + **OS gating** (`os = "macos"`) on packages; today only symlinks have `os` (`types.go:150`)                                                                    | High                                       |
| **install-tmux-sessionizer.sh** (`:20-43`)  | `cargo install --git <repo> tmux-sessionizer`; sources `~/.cargo/env` if cargo not on PATH; binary name `tms`≠crate                                                                                                                      | `toolchain` (cargo)                            | **`cargo install --git URL`** support (toolchain only does `cargo install <name>`, `toolchain.go:48-57`); **PATH-priming** (`source ~/.cargo/env`)                                          | High                                       |
| **install-rust.sh** (`:49-94`)              | rustup via `sh.rustup.rs` with `-y --no-modify-path --default-toolchain stable`; sources cargo env; best-effort `rustup self update`; extra crates loop                                                                                  | `script-url` (+ post-steps)                    | **Post-install step vocab** (source env, install N crates); **rolling-update semantics** (self-update, not reinstall). `script-url` runs one script, no post-steps (`script_url.go:28-52`)  | High                                       |
| **install-mise.sh** (`:140-267`)            | 4-way install cascade (brew→yay/paru→curl `mise.run`→fail); self-update if present; `mise list` status                                                                                                                                   | `script-url` **or** `pkg`+fallback             | **Ordered multi-method fallback** ("try brew, else AUR, else vendor script"); **self-update** path. No backend chains methods                                                               | Medium                                     |
| **install-atuin.sh** (`:315-515`)           | vendor installer w/ `ATUIN_INSTALL_DIR`/`SKIP_SHELL_INTEGRATION`/`NO_UPDATE_PROFILE` env; **desktop** AppImage/DMG/MSI arch matrix; **shell-config cleanup** (strips PATH lines); writes `config.toml`; `atuin import auto`; self-update | `script-url` **+** heavy post-steps            | **Env-var passthrough to installer**; **post-install config-file write**; **post-install command** (`atuin import`); desktop-artifact matrix is out-of-scope. Biggest gap set of any script | High                                       |
| **install-aws-tools.sh** (`:122-292`)       | 3 tools, 3 mechanisms: AWS CLI = `.pkg` (macOS) / `.zip`+`./aws/install` (Linux); SAM = `pip3 --user`; ecs-cli = bare GH binary w/ `--version` sanity check                                                                              | mixed                                          | **`.pkg` installer** (`sudo installer -pkg`); **run vendor `install` script inside archive**; **`pip`/toolchain=pip**; **bare-binary asset** (no archive). 3 distinct gaps in one script    | High                                       |
| **install-azure-cli.sh** (`:95-243`)        | Debian = MS apt script `aka.ms/InstallAzureCLIDeb`; Arch = AUR; RPM = MS GPG import + `packages-microsoft-prod.rpm` repo; macOS = brew; fallback `pip3 --user`                                                                           | `apt-repo` + `aur` + **`rpm-repo`** + `pkg`    | **`rpm-repo` backend** (GPG import + `.rpm` repo config — explicitly deferred in objective); **per-OS backend selection** for one logical package                                           | High                                       |
| **install-hashicorp-tools.sh** (`:115-341`) | terraform via HashiCorp apt/rpm repo or pacman or binary; tflint via GH `.zip`; GPG dearmor + fingerprint verify; keyring idempotency (`:123`,`:157`)                                                                                    | `apt-repo` + **`rpm-repo`** + `github-release` | **`rpm-repo`**; **GPG fingerprint verification** step; **multi-tool intent** (terraform+tflint under one entry)                                                                             | High                                       |
| **install-container-tools.sh** (`:115-239`) | skaffold = bare GS binary; hadolint = bare GH binary `hadolint-Linux-x86_64` (capital OS, raw `x86_64`); post-download `--version` sanity gate (`:225`)                                                                                  | `github-release` (bare)                        | **Bare-binary (non-archive) asset**; **`{{.OS}}`=`Linux`/`Darwin` casing**; **`{{.Arch}}`=`x86_64`**; **post-install binary sanity check**                                                  | High                                       |
| **install-tea.sh** (`:111-303`)             | pacman→brew→binary cascade; `checksums.txt` fetch+verify w/ sha256sum                                                                                                                                                                    |                                                | shasum; `~/.local/bin`; generates **bash/zsh/fish completions** into fpath dirs (`:266-303`)                                                                                                | `github-release` (Gitea host) + post-steps | **Non-GitHub release host** (`gitea.com` API/URL); **`checksums.txt` line-lookup** (not a single pinned sha); **completion-generation post-step** | High   |
| **install-lefthook.sh** (`:84-177`)         | brew→npm `-g`→`go install`→GH `.gz` single-binary cascade; `lefthook_${ver}_${os}_${arch}.gz`                                                                                                                                            | `toolchain`(npm) **or** `github-release`       | **`.gz` single-binary decompress** (download.go has no `.gz`, only `.tar.gz`, `:104-114`); **method cascade**                                                                               | High                                       |
| **install-turso.sh** (`:135-294`)           | brew tap→manual `~/.local/bin` binary w/ checksum; removes stale binary first (`:135-138`)                                                                                                                                               | `github-release` + post                        | **checksums.txt lookup**; brew-tap-then-binary cascade                                                                                                                                      | Medium                                     |
| **install-claude.sh** (`:238-354`)          | vendor `claude.ai/install.sh`                                                                                                                                                                                                            | npm `-g @anthropic-ai/claude-code`             | curl bare-binary fallback w/ arch map                                                                                                                                                       | `script-url` **or** `toolchain`(npm)       | **npm-scoped-package** (`@scope/name`) install identifier; method cascade                                                                         | Medium |
| **install-lua.sh** (`:184-223`)             | per-distro pkg (`lua5.1`/`lua51`/`lua@5.1`) + luarocks; version-param `LUA_VERSION`; source-build fallback (gcc/make/readline)                                                                                                           | `pkg` (multi-name)                             | **Version-suffixed names** already OK via `names`; **companion package** (luarocks) under one entry; source-build is out-of-scope                                                           | Medium                                     |

**The pattern:** the scripts encode five backend capabilities that don't exist yet — (1) **non-archive asset handling** (bare binary, `.gz`, `.pkg`), (2) **richer arch/OS templating vocabulary** (`x86_64`/`aarch64`, `Linux`/`Darwin`, `darwin`≠`macos`), (3) **post-install steps** (source env, write config, generate completions, import data, sanity-check), (4) **an `rpm-repo` backend** and **`git-clone` backend**, and (5) **ordered multi-method cascades** with self-update semantics. Until these land, the 19 scripts are irreplaceable — the migration is blocked on backend features, not effort.

---

## Obsolete Approaches Worth Reviving

### 1. CFEngine's _convergence_ (not "idempotence") — Mark Burgess, 1993

CFEngine predates Puppet/Chef/Ansible by 12+ years and introduced **convergent operators**: however many times the agent runs, state only ever gets _closer_ to the ideal end-state, and an already-converged host is a fast no-op. Burgess is explicit that "convergence" is _stronger_ than "idempotence" — it implies both a defined desired end-state **and** an idempotent error-correcting operator ([CFEngine — Wikipedia](https://en.wikipedia.org/wiki/CFEngine); [A Tiny Overview of Cfengine (SciTePress PDF)](https://www.scitepress.org/Papers/2005/11592/11592.pdf); [Promise Theory / Network World](https://www.networkworld.com/article/926422/sdn-promise-theory-mark-burgess-cfengine-sdn-cisco-aci-apic-opflex.html)).

**Revive for dotf:** dotf's `apply` already _aspires_ to this (the manifest comment at `packages.toml:227-234` says converged hosts "stay idempotent"), but the mechanism is weak — `LookPath` "is the binary present" is a **coarse** convergence probe. CFEngine's insight: convergence needs a _declared_ end-state and a _cheap check_ that measures distance to it. dotf should declare, per package, a **convergence predicate** richer than "binary on PATH" (see Historical Idempotency below). **Also revive the "autonomy / don't impose" ethic**: Burgess's Promise Theory grew from the observation that blindly overwriting host state is bad form — which is exactly why dotf's _additive-only, no-reconciliation_ stance (objective §scope) is defensible, not a limitation. The Archaeologist's verdict: **the additive stance is a Promise-Theory-correct design choice, worth defending against "add reconciliation" pressure.**
**Confidence:** High (concept), Medium (direct applicability).

### 2. GNU Stow's _ownership model_ + _two-phase conflict scan_ — Bob Glickstein, 1993

Stow (same vintage as CFEngine, 1993) manages symlink farms with two ideas dotf's symlink engine should mine: **ownership** ("Stow only ever deletes things it owns — symlinks pointing into a stow package; it recomputes them if lost; it never touches foreign files") and, **since v2.0, a two-phase algorithm that scans the whole tree for conflicts _before_ making any change** ([GNU Stow manual](https://www.gnu.org/software/stow/manual/stow.html); [Conflicts node](https://www.gnu.org/software/stow/manual/html_node/Conflicts.html)). It also has **tree-folding** (fold a whole subtree into one symlink when the target is empty; split it open only when another package needs a sibling) and an **`--adopt`** move-existing-file-into-package option.

**Revive for dotf:** dotf's `link` already has `--dry-run`/`--backup`, but Stow's **ownership predicate** ("is this existing link one _I_ made, pointing where I want? → no-op; is it a foreign real file? → conflict, don't clobber") is a more principled idempotency/safety rule than backup-and-replace. The **pre-scan-for-conflicts-then-act** two-phase model gives a truthful dry-run and prevents half-applied states. **Worth reviving: an ownership check + a conflict pre-scan pass in `internal/symlink`.**
**Confidence:** High.

### 3. Ansible's `creates:` / `removes:` sentinel — the stamp-file primitive

Ansible's `command`/`shell` modules are "not idempotent," so Ansible bolts on **`creates:`** (skip if path exists) and **`removes:`** (skip if path absent); for commands with no natural output artifact, the idiom is a **stamp/sentinel file** — and, crucially, **version-encode the sentinel path** (`/opt/app/version-{{ver}}.installed`) so a version bump re-triggers ([Ansible creates/removes + stamp pattern](https://oneuptime.com/blog/post/2026-02-21-how-to-use-the-ansible-command-module-with-creates-parameter/view)).

**Revive for dotf:** this is the _exact_ primitive the `custom-script` and setup-step packages need. dotf's `custom_script.go:49-55` uses `LookPath(pkg.Name)` — useless for `tpm` (no binary), `gnu-utilities` (no single binary), and version-manager self-updates. A manifest field like `installed_when = { path = "~/.tmux/plugins/tpm" }` (Ansible `creates:`) would make `tpm` idempotent _without_ a `LookPath` that can never succeed. **Worth reviving: a declarative `creates:`-style sentinel/path predicate per package.**
**Confidence:** High.

### 4. Debian `debconf` preseeding — declarative answers to imperative installers

debconf (Wichert Akkerman/Joey Hess, late-1990s) separates _asking_ from _answering_: `debconf-set-selections < answers` loads answers into `/var/cache/debconf/config.dat` **ahead of time**, so `apt-get install` runs unattended, and a preseed file is deliberately **not a shell script** (no `if`, no vars) — pure declarative data ([Debian Preseed wiki](https://wiki.debian.org/PackageManagement/Preseed); [debconf concepts](https://docs.openstack.org/newton/install-guide-debconf/debconf/debconf-concepts.html)).

**Revive for dotf:** dotf already does the _spirit_ of this — `NON_INTERACTIVE=true` (`custom_script.go:42`) and `DEBIAN_FRONTEND=noninteractive` (`pkg.go:97`). The revivable idea is the **strict data/code separation**: today the "answers" live _inside_ 13k LOC of imperative bash. debconf's lesson is that the answer set should be **declarative TOML data** the engine consumes, with the imperative logic centralized in the (Go) engine. That is literally the migration's thesis — debconf is the historical proof that "declare the answers, centralize the mechanism" scales to "hundreds of machines without being in front of the screen."
**Confidence:** High (as a design-principle anchor).

### 5. Solaris JumpStart / RedHat Kickstart — the _profile_ as a single declarative end-state file

JumpStart (Solaris, 1990s) and Kickstart (Red Hat, late-1990s) established the pattern of **one declarative text file describing a machine's desired end-state** (partitioning, packages, timezone), consumed unattended, with a `rules.ok` file matching _host attributes_ (arch/hostname/IP) to a **profile** ([JumpStart — Wikipedia](<https://en.wikipedia.org/wiki/JumpStart_(Solaris)>); [Kickstart syntax — Red Hat](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/7/html/installation_guide/sect-kickstart-syntax); [softpanorama JumpStart](https://softpanorama.org/Solaris/Installation/solaris_jumpstart.shtml)).

**Revive for dotf:** the `rules.ok`-matches-host-attributes-to-profile idea is the ancestor of **per-host / per-profile manifests**. dotf's `intents` are a coarse version; JumpStart's _attribute-matched profile selection_ (this arch/host → this bundle) is the mature form. Given dotf's single-host scope, a lightweight **host-attribute → intent auto-selection** (e.g. "on macos, auto-add `gnu-utilities`") is the in-scope slice of this idea. Fleet/multi-host is explicitly out of scope, so don't over-revive.
**Confidence:** Medium (partially in-scope).

### 6. Makefile-driven & rc.d-fragment dotfiles — the humble ancestors

Pre-chezmoi, dotfiles were managed by `Makefile` targets (`make install` symlinking files) and **rc.d fragment loading** (`~/.bashrc.d/*.sh`, `.mkshrc`-style sourced fragments). dotf's `common/` already _is_ fragment-loading for shell config. The revivable Makefile idea is **explicit target dependencies** ("symlinks depend on dirs; sessionizer depends on rust") — dotf's `apply` runs a fixed phase order (sysconfig→packages→symlinks→firewall) but has **no intra-package dependency graph** (e.g. `tmux-sessionizer` needs `rust` first — currently enforced only by putting them in different intents and by the script's own cargo-missing skip, `install-tmux-sessionizer.sh:37-40`). **Worth a small revival: a `requires = ["rust"]` edge on packages** so `apply` can topologically order within a run.
**Confidence:** Medium.

---

## Discontinued Methods (buried — leave them buried, with reasons)

- **`stow`-style pure-symlink _binary_ management** (symlink `/usr/local/bin/foo`→`stow/foo/bin/foo`): elegant for source-built trees, but wrong for downloaded release binaries and pkg-manager installs. dotf's copy-to-`~/.local/bin` is correct here. _Leave buried for binaries; revive Stow's ideas only for the dotfile symlink engine._
- **CFEngine's bespoke policy language / promise agents / pull-from-central-server**: the daemon+DSL+fleet model is the opposite of a single-binary, single-host, additive tool. Revive the _convergence concept_, not the machinery. _Buried._
- **debconf's interactive TUI priority system**: dotf uses `huh` for TUI; debconf's priority-gated question flow is package-config-specific. _Buried; only the data/code-separation principle is revivable._
- **JumpStart begin/finish Bourne scripts + PXE/NFS/TFTP netboot**: bare-metal install-time machinery, entirely out of dotf's post-OS scope. _Buried; only the profile concept is revivable._
- **`pip3 --user` as an install backend** (aws-sam, azure fallback — `install-aws-tools.sh:213`, `install-azure-cli.sh:197`): PEP-668 "externally-managed-environment" now blocks `pip install --user` on most 2024+ distros. These fallbacks are _already_ semi-broken; don't port them as a `pip` toolchain — prefer `pipx` or the vendor binary. _Mostly buried; flag the scripts as fragile._
- **`cursor-agent`/`smartsheet-cli`/`neovim`-from-source scripts**: not in the 19 custom-script set; several are legacy one-offs. _Not in scope._

---

## Historical Idempotency / State Techniques (better than `LookPath`)

dotf's binary-download backends decide "already installed?" with `LookPath(pkg.Name)` (`github_release.go:81-87`, `script_url.go:55-66`, `toolchain.go:74-82`, `custom_script.go:49-55`). This is the _weakest_ historical technique and fails in known ways:

- **False positive:** a _different_ tool with the same basename shadows on PATH → dotf thinks it's installed.
- **False negative for non-binary installs:** `tpm` (git clone, no binary), `gnu-utilities` (14 formulae, no single binary) → `LookPath("tpm")`/`LookPath("gnu-utilities")` _always_ fails → the package is _never_ considered installed → re-runs every apply (the scripts' own internal `[[ -d ]]`/`brew list` guards save correctness, but dotf's _status/diff_ reporting is wrong).
- **Version-blind:** can't tell an outdated tool from a current one → no `dotf update` is possible.

Historically-proven, more-robust techniques (oldest→newest), ranked by fit:

1. **Ask the package authority (already in dotf's `pkg` backend).** `pacman -Q`, `dpkg-query -W -f=${db:Status-Status}` (distinguishes `installed` vs `config-files`), `rpm -q`, `brew list` (`pkg.go:109-142`). _This is the gold standard and it's already written_ — the fix is to make the _other_ backends reuse the same "query the authority" discipline where an authority exists (brew/pkg-managed tools).
2. **Sentinel / stamp file (Ansible `creates:`, 2012-era; ancestor: BSD `/etc/rc` flag files).** A declared path that, if present, means "done" — and **version-encode the path** so a bump re-triggers. Perfect for `tpm` (`~/.tmux/plugins/tpm`), `gnu-utilities`, and any setup-step. _Cheapest high-value fix._
3. **Content-hash run-once (chezmoi `run_once_`/`run_onchange_`).** chezmoi decides re-execution by whether **a script with the same contents has run successfully before** — i.e. it hashes script _contents_ and stores the result in its state DB; `run_once_` runs only if new/changed/previously-failed, `run_onchange_` runs whenever contents change ([chezmoi target-types](https://www.chezmoi.io/reference/target-types/)). _This is the exact primitive for the rolling-release scripts (`mise`/`rust`/`atuin`): re-run the setup step only when the script (or a pinned version token) changes, not on every apply._
4. **Checksum lockfile keyed to the resolved artifact (aqua `aqua-checksums.json`).** aqua pins tool _versions_ and stores per-asset SHA256 in a lockfile keyed by `github_release/…/<asset>`, generated by `aqua update-checksum`, committed alongside the config; `require_checksum` fails closed if an asset isn't in the lockfile ([aqua checksum guide](https://aquaproj.github.io/docs/guides/checksum/); [aqua checksum reference](https://aquaproj.github.io/docs/reference/config/checksum/)). _This directly fixes dotf's "a single `sha256` pins exactly one arch" trap (`packages.toml:184-185`): a **lockfile of per-(tool,version,os,arch) checksums** lets one package entry be secure across arches, and gives `Installed()` a version-and-integrity-aware check instead of `LookPath`._
5. **Capability probing over PATH probing (autoconf `./configure` heritage).** Instead of "is `foo` on PATH," test "does `foo --version` succeed and satisfy a version constraint" — the scripts already _do_ this (hadolint/ecs-cli run `--version` as a post-download sanity gate, `install-container-tools.sh:225`, `install-aws-tools.sh:277`). _Revive as: `Installed()` may run a declared probe command + version regex, not just `LookPath`._

**Recommended layering for dotf:** (a) authority-query where a pkg manager owns the tool; (b) sentinel path (`installed_when`) for setup-steps; (c) content-hash/version-token run-once for rolling installers; (d) a checksum lockfile for binary backends. All four are 1990s–2010s proven; none needs a framework.

---

## Revival Candidates (ranked by value ÷ risk, in-scope only)

| #   | Candidate                                                                                                                                                           | Origin                    | Value | Effort | Risk | Why it fits dotf's scope                                                                                                                 |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------- | ----- | ------ | ---- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | **Sentinel/`creates:` predicate** (`installed_when = {path=…}`) per package                                                                                         | Ansible / BSD rc          | High  | Low    | Low  | Makes `tpm`/`gnu-utilities`/setup-steps idempotent & status-correct without a real binary. Pure TOML field + one `os.Stat`.              |
| 2   | **Spread the `pkg` backend's authority-query discipline** to brew/pkg-owned tools; stop bare `LookPath` where an authority exists                                   | dpkg/rpm/pacman era       | High  | Low    | Low  | Code already exists in `pkg.go:109-142`; reuse it. Fixes false-positive/negative `Installed()`.                                          |
| 3   | **`git-clone` backend** (repo, ref, depth, dest, dir-exists idempotency)                                                                                            | Makefile/git era          | High  | Low    | Low  | Retires `tpm` cleanly; `git clone --depth 1` is 4 lines. Genuinely a setup-step, not a package.                                          |
| 4   | **Richer asset templating** (`{{.Arch}}` variants `x86_64`/`aarch64`; `ReleaseOS()` → `linux`/`darwin`; case helpers) + **non-archive assets** (bare binary, `.gz`) | GH-release-era installers | High  | Med    | Med  | Unblocks retiring hadolint/skaffold/ecs-cli/tflint/lefthook/tea. This is the single biggest script-retirement lever.                     |
| 5   | **Checksum lockfile** for github-release/script-url (per os/arch), `dotf lock` to generate                                                                          | aqua                      | High  | Med    | Med  | Fixes "sha256 pins one arch"; enables secure multi-arch + version-aware `Installed()`. Additive file, no framework.                      |
| 6   | **Post-install steps** vocab (`post_install = ["source ~/.cargo/env", "atuin import auto", "gen-completions"]`)                                                     | rc.d / installer scripts  | High  | Med    | Med  | Retires the "setup" half of rust/atuin/tea/mise. Must be a _constrained_ step list, not arbitrary bash, or it recreates `custom-script`. |
| 7   | **`rpm-repo` backend** (GPG import + `.rpm` repo config)                                                                                                            | yum/dnf era               | Med   | Med    | Low  | Explicitly-deferred gap; unblocks azure/hashicorp on Fedora/RHEL. Symmetric to existing `apt-repo`.                                      |
| 8   | **Content-hash run-once ledger** for rolling installers                                                                                                             | chezmoi/CFEngine          | Med   | Med    | Med  | Re-run mise/rust/atuin only when their pinned version/script changes; enables a real `dotf update`.                                      |
| 9   | **Stow-style ownership + two-phase conflict pre-scan** in `internal/symlink`                                                                                        | GNU Stow                  | Med   | Med    | Low  | Safer, truthful dry-run; never clobbers foreign files; principled idempotency for links.                                                 |
| 10  | **`requires = [...]` dependency edges** on packages → topological order in `apply`                                                                                  | Makefile                  | Med   | Med    | Med  | `tmux-sessionizer`→`rust`; removes reliance on intent-ordering + in-script skips.                                                        |
| 11  | **Ordered multi-method cascade** per package (`methods = ["pkg","script-url"]`)                                                                                     | mise/tea/lefthook scripts | Med   | High   | Med  | Elegant but risks complexity blow-up; several scripts' cascades are just "brew else vendor." Lower priority.                             |
| 12  | **Host-attribute → intent auto-selection** (JumpStart `rules.ok`)                                                                                                   | JumpStart/Kickstart       | Low   | Med    | Med  | `os=macos → +gnu-utilities`. Nice, but coarse intents already cover most of it.                                                          |

**The 20% that delivers 80%:** #1 + #2 + #3 (all Low/Low) make the _setup-step_ scripts (`tpm`, `gnu-utilities`, and status-correctness broadly) retireable this week with almost no risk, and fix the `Installed()` correctness bug across the board. #4 + #5 are the next tranche and unblock ~7 of the cloud/container binary installers.

---

## Comparative Analysis (old idea → modern proof → dotf gap)

| Old idea (year)                                   | Modern tool that revived it                            | dotf's current state                                           | Gap                               |
| ------------------------------------------------- | ------------------------------------------------------ | -------------------------------------------------------------- | --------------------------------- |
| Convergence (CFEngine 1993)                       | Puppet/Chef/Ansible; chezmoi "re-apply is safe"        | Aspires to it (`packages.toml:227`) but probe is `LookPath`    | Convergence predicate too coarse  |
| Symlink ownership + conflict pre-scan (Stow 1993) | `stow`, `rcm`, dotbot                                  | `link` has dry-run/backup, no ownership model                  | No foreign-file safety / pre-scan |
| Sentinel/`creates:` (Ansible)                     | chezmoi `run_once_`, Make targets                      | `LookPath` only                                                | No declarative path predicate     |
| Content-hash run-once (chezmoi)                   | chezmoi state DB, aqua                                 | none                                                           | Rolling installers re-run blindly |
| Checksum lockfile (aqua)                          | aqua `aqua-checksums.json`, mise, Renovate             | single inline `sha256`, one-arch-only                          | No lockfile → insecure multi-arch |
| Declarative answers (debconf 1990s)               | cloud-init user-data, Terraform                        | `NON_INTERACTIVE`/`DEBIAN_FRONTEND` set; answers still in bash | 13k LOC of imperative "answers"   |
| Profile per host (JumpStart/Kickstart 1990s)      | cloud-init, Ansible inventory, chezmoi `.chezmoi.<os>` | coarse `intents`                                               | No host-attribute auto-selection  |

The through-line: **every capability the scripts need has a 1990s ancestor and a modern proof-of-concept.** dotf isn't inventing — it's _catching up to_ ideas that are 20–30 years old. That is the archaeologist's central claim.

---

## Key Insights

1. **The migration is blocked on backend capabilities, not effort.** 5 missing capabilities (non-archive assets, richer arch/OS vocab, post-install steps, `rpm-repo`/`git-clone` backends, method cascades) gate ~14 of the 19 scripts. Ship the capabilities first; the TOML entries follow trivially.
2. **`LookPath` is the single worst technique dotf could have chosen for `Installed()` — and dotf already wrote the _right_ one next door** (`pkg.go`'s authority queries). The fix is diffusion, not invention.
3. **Not everything is a package.** `tpm` (git clone), `gnu-utilities` (formula loop), `tmux-sessionizer` (`cargo --git`) are _setup steps_. Forcing them into a binary-download backend is the wrong archaeology; giving the manifest a `git-clone` backend + `installed_when` sentinel is the right one.
4. **dotf's additive-only, no-reconciliation stance is Promise-Theory-correct**, not a shortcoming — Burgess built CFEngine's whole philosophy around _not imposing_ state. Defend it against "add reconciliation" pressure.
5. **The `sha256`-pins-one-arch trap is a genuine design flaw** (`packages.toml:184-185` admits it). aqua solved this in 2020 with a lockfile; reviving that pattern is the cleanest fix and unlocks secure multi-arch.
6. **Two scripts encode capabilities dotf should deliberately _decline_:** atuin's desktop AppImage/DMG/MSI matrix (GUI-app install, out of scope) and lua's from-source build (gcc/make/readline). Keep these as `custom-script` forever, or drop them — don't build backends for them.

---

## Evidence Quality

- **Codebase claims: High.** Every script and backend claim is cited to `file:line` from files I read in full: `github_release.go`, `download.go`, `pkg.go`, `apt_repo.go`, `toolchain.go`, `script_url.go`, `custom_script.go`, `backend.go`, `manifest/types.go`, `manifest/packages.toml`, `platform/types.go`, and the 11 mandated scripts + samples of lefthook/turso/claude/lua. Verified by direct read, not inference.
- **External history: High for the primary claims.** GNU Stow (gnu.org manual — primary), chezmoi (chezmoi.io — primary), aqua (aquaproj.github.io — primary), CFEngine/convergence (Wikipedia + SciTePress paper + Burgess primary), debconf (Debian wiki — primary), JumpStart/Kickstart (Wikipedia + Red Hat docs — primary/authoritative). All stable, mature topics; freshness not a concern (5+ year stable category).
- **Medium-confidence items** are flagged inline (mise/turso/claude cascades read via grep not full file; JumpStart host-selection applicability; chezmoi state-DB storage detail inferred from behavior, not the storage-internals doc).

---

## Contradictions & Uncertainties

- **"Retire custom-script" vs. "some scripts are irreducibly imperative."** The objective's ≥80%-LOC-retirement metric collides with the reality that atuin-desktop, lua-from-source, and multi-method cascades are legitimately shell-shaped. _Resolution:_ target ~80% of the _simple_ installers; accept that a residual `custom-script` set (5-ish genuinely-imperative scripts) is correct, not failure. The metric should be "custom-script ≤ ~5 entries," not "0."
- **Add reconciliation/idempotency ledger vs. keep-it-simple.** A content-hash state DB (chezmoi-style) adds state dotf currently doesn't keep. Contrarian/scope tension: is a `dotf update` worth a state file? _Unresolved_ — flagged for synthesis. My lean: sentinel files (#1) give 80% of the benefit with zero persistent-state machinery.
- **`{{.OS}}` semantics — I claim it renders the distro family (`debian`/`arch`), making it near-useless for release assets.** This is inferred from `string(info.OS)` (`github_release.go:98`) + the OS constants (`platform/types.go:19-27`); I did **not** find a runtime test proving `info.OS`=`debian` on Ubuntu during a github-release install (the backend has no manifest user of `{{.OS}}` today). _Confidence Medium-High; worth a maintainer confirming with one test._ Either way, `darwin`≠`macos` is a definite mismatch.
- **`pip --user` fallbacks** (aws-sam, azure): I assert PEP-668 likely breaks these on modern distros from general knowledge, not a repo test. Flagged Low-Medium; verify before relying on those fallbacks.
- **chezmoi hashing algorithm:** the fetched doc confirms _content-based_ re-run but did not state SHA256/BoltDB explicitly; I describe the behavior, not the storage internals. Confidence Medium on mechanism specifics.

---

## Search Queries Executed

**Codebase (Read/Grep/Bash):**

1. `bin/install-tpm.sh`, `install-tmux-sessionizer.sh`, `install-gnu-utilities.sh`, `install-rust.sh`, `install-mise.sh` (full reads)
2. `bin/install-hashicorp-tools.sh`, `install-azure-cli.sh`, `install-container-tools.sh`, `install-aws-tools.sh`, `install-tea.sh`, `install-atuin.sh` (full reads)
3. `internal/backend/{github_release,backend,custom_script,toolchain,script_url,pkg,apt_repo,download}.go` (full reads)
4. `internal/manifest/types.go`, `manifest/packages.toml` (full reads)
5. grep: lefthook/turso/claude/lua install methods; platform `OS`/`ReleaseArch` constants; toolchain `--git` support

**Web (WebSearch/WebFetch):** 6. GNU Stow symlink farm / tree folding / conflict detection / ownership / history 7. chezmoi `run_once_`/`run_onchange_` script re-run logic + externals (WebFetch chezmoi.io) 8. CFEngine convergence / promise theory / idempotence / Burgess 1993 history 9. aqua `aqua-checksums.json` checksum lockfile / version pinning / github-release binary manager 10. Ansible `command` module `creates`/`removes` / stamp-file idempotency 11. Debian debconf preseeding / declarative non-interactive config / history 12. Solaris JumpStart / RedHat Kickstart declarative provisioning profile / cloud-init lineage

---

_Sources (external):_
[GNU Stow manual](https://www.gnu.org/software/stow/manual/stow.html) ·
[GNU Stow — Conflicts](https://www.gnu.org/software/stow/manual/html_node/Conflicts.html) ·
[chezmoi target-types (run_once_/run_onchange_)](https://www.chezmoi.io/reference/target-types/) ·
[aqua — Enable Checksum Verification](https://aquaproj.github.io/docs/guides/checksum/) ·
[aqua — Checksum config reference](https://aquaproj.github.io/docs/reference/config/checksum/) ·
[CFEngine — Wikipedia](https://en.wikipedia.org/wiki/CFEngine) ·
[A Tiny Overview of Cfengine — Burgess (SciTePress PDF)](https://www.scitepress.org/Papers/2005/11592/11592.pdf) ·
[Promise Theory — Network World](https://www.networkworld.com/article/926422/sdn-promise-theory-mark-burgess-cfengine-sdn-cisco-aci-apic-opflex.html) ·
[Ansible command `creates`/stamp-file pattern](https://oneuptime.com/blog/post/2026-02-21-how-to-use-the-ansible-command-module-with-creates-parameter/view) ·
[Debian Preseed wiki](https://wiki.debian.org/PackageManagement/Preseed) ·
[debconf concepts](https://docs.openstack.org/newton/install-guide-debconf/debconf/debconf-concepts.html) ·
[JumpStart — Wikipedia](<https://en.wikipedia.org/wiki/JumpStart_(Solaris)>) ·
[Kickstart syntax — Red Hat](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/7/html/installation_guide/sect-kickstart-syntax)
