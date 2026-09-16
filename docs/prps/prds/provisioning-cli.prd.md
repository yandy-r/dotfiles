# Provisioning CLI (Go) — Interactive, Manifest-Driven Machine Bootstrap

## Problem Statement

The dotfiles repo has outgrown "a bunch of shell scripts": `bootstrap.sh` is **1,848 lines** with its section list and conditional guards maintained in **2–3 synchronized places**, and **25 `bin/install-*.sh` scripts total ~12,967 lines** of which an estimated **2,000–3,500 are duplicated scaffolding** (logging, OS detection, force/interactive handling). Platform detection exists in **three drifted copies** that already disagree (`amd64` vs `x86_64`). The cost of not solving it: every new tool requires multiple synchronized edits across imperative bash, there is no previewable/selectable install plan, no system-config provisioning (hostname/timezone/locale), and zero tests or CI to catch the drift.

## Evidence

- `bootstrap.sh:1751-1765` (interactive flow) and `bootstrap.sh:1766-1820` (linear flow) hand-duplicate the same ordered ~40-function section list; conditionals are tripled (wrapper funcs `section_*` at 1642-1672 + re-inlined guards at 1773-1803 + each installer's own guard).
- Platform detection drift: `common/platform.sh:50-150` (canonical), `bootstrap.sh:286-368` (hand-maintained copy, header admits "ALIGNED WITH common/platform.sh"), and 13/25 installers carry local `detect_os()` copies — `detect_arch` returns `x86_64` in two and `amd64` in `install-tea.sh:67-73`.
- A shared lib (`bin/scripts/utils.sh`, 741 lines) exists but is used by only **2** scripts and is Docker-branded.
- No declarative package→purpose manifest exists; all package lists are imperative bash arrays (`bootstrap.sh:746-763`, `787-799`, `1108-1133`; `install-native-packages.sh` N-categories × M-managers).
- No hostname/timezone/locale/user handling anywhere (`grep` for `hostnamectl|/etc/hostname|timedatectl|localectl|useradd` → none).
- No `.github/workflows/`, no test harness. lefthook + commitlint + conventional commits are already in place (`lefthook.yml`, `commitlint.config.cjs`).
- The **one** genuinely declarative asset is `playbooks/vars/symlinks.yml` — a clean `{src,dest,type}` model with dryrun/backup/validate semantics, the contract to preserve.

## Proposed Solution

Build a single-binary **Go CLI** that replaces the bash bootstrap by reading a **declarative TOML manifest** (intents → packages, with per-tool multi-backend install methods) and driving an install engine, a native Go symlink engine (retiring the Ansible dependency), an interactive intent-selection TUI (`charmbracelet/huh` pre-checked multiselect with add/remove), and OS-level system config (hostname/timezone/locale/shell/firewall). A ~40-line `bootstrap.sh` shim detects OS/arch and downloads the prebuilt binary from GitHub Releases, then hands off. Go is chosen over Rust/Python/bash because the entire comparable-tool ecosystem (chezmoi, aqua, gum) is Go, it produces a zero-runtime static binary (solving the fresh-machine chicken-and-egg and avoiding the interpreter/hook timeout class the repo already hit with autoenv), has the best checklist-TUI story, and is mockable for tests at a lower contributor bar than Rust.

## Key Hypothesis

We believe a **declarative-manifest Go CLI** will make machine provisioning **reproducible, previewable, and one-edit-to-extend** for the maintainer and anyone cloning the repo.
We'll know we're right when **adding a new tool means editing one TOML manifest entry (zero control-flow edits)**, and a **fresh machine provisions end-to-end either guided or `--headless` with no pre-installed runtime**, with the new tool being measurably **easier to maintain than the 1,848-line script it replaces**.

## What We're NOT Building

- **Secrets orchestration** (1Password/Bitwarden service-account wiring) — out of scope for v1. _Note: `dotenvx` remains a normal manifest install option; only the advanced integration orchestration is deferred._
- **Package removal / full reconciliation** — v1 is additive-install + read-only drift report; removal contradicts solo-maintainer complexity budget and matches chezmoi's deliberate additive-only stance.
- **Fleet / multi-host orchestration** — not competing with Ansible/Nix for managing many machines; single-host, single-user.
- **Windows-native provisioning** — WSL-first; Windows terminal configs are a nice-to-have, native winget/PowerShell paths deferred.
- **A GUI** — terminal TUI only.

## Success Metrics

| Metric                              | Target                                                                   | How Measured                                   |
| ----------------------------------- | ------------------------------------------------------------------------ | ---------------------------------------------- |
| Add-a-tool edit surface             | 1 TOML entry, 0 Go/control-flow edits                                    | Diff review when adding a sample tool          |
| Script retirement (per-tool)        | Each **mechanically-migratable** `bin/install-*.sh` is deleted on cutover to a native backend (`pkg`/`github-release`/`toolchain`/`apt-repo`/`script-url`); measured **per tool**, not as an aggregate LOC percentage. Residual target: `custom-script` converges to **≤ ~5 entries** — the genuine setup-steps intentionally retained as permanent governed adapters (see [Residual setup-step category](#residual-setup-step-category-permanent-governed-adapters)). | Count of `custom-script` manifest entries + a per-tool delete-on-cutover checklist |
| Fresh-machine provision (headless)  | Clone → fully provisioned, 0 pre-installed runtime beyond curl/coreutils | VM run on Arch + Debian + macOS                |
| Platform-detection copies           | 1 (down from 3)                                                          | grep for `detect_os`/`detect_arch` definitions |
| Test coverage of backend argv logic | Every (distro × backend) asserts exact command line                      | `go test ./...` table-driven                   |
| CI                                  | PR workflow (test/vet/lint) green; release workflow on tag               | GitHub Actions                                 |

### Residual setup-step category (permanent governed adapters)

`custom-script` is **not** a backend that must reach zero — it is the permanent, **governed
adapter** for genuine _setup steps_ that a native backend would only make worse. Of the 19
`custom-script` entries in `manifest/packages.toml` today, roughly half are mechanically
migratable (single-binary / vendor-installer cases → `github-release` / `script-url` /
`toolchain` / `pkg`) and form the real retirement backlog; the rest are **intentionally
retained**:

- **Rolling version managers** — `mise`, `rust` (rustup): self-updating toolchains, not one-shot installs.
- **Git-clone / in-place setup** — `tpm` (clone into `~/.tmux/plugins`), `tmux-sessionizer`, `gnu-utilities`.
- **Multi-package, OS-gated setup** — `aws-tools`, `azure-cli` (per-OS keyring / rpm-repo paths), `gnu-utilities` (brew multi-pkg).

These stay as `bin/install-*.sh` scripts invoked via `custom-script` because they encode real,
hard-won imperative platform logic; re-implementing them in Go is higher-risk and lower-value.
Every mature comparator keeps exactly such a governed escape hatch permanently (chezmoi
`run_` scripts, Ansible `command`/`shell`, Terraform `local-exec`) — chasing "zero shell-outs"
is the strangler-fig anti-pattern that never finishes and Goodhart-misdirects effort onto the
setup-step scripts that should be kept.

This reframing does **not** widen scope: the boundaries below still hold — **additive,
single-host, stdlib-`flag`, governed-escape-hatch** — no package removal / reconciliation
(`--prune` deferred), no secrets orchestration, no fleet / multi-host, no Windows-native
provisioning, no GUI, and no CLI framework (Cobra/pflag remain rejected).

## Open Questions

_All resolved 2026-06-30 — see the Decisions Log below._

- [x] **Binary/command name** → **`dotf`** (`cmd/dotf/main.go`; module stays `github.com/yandy-r/dotfiles`). Short, dotfiles-evident, no collision with the POSIX `env` or Foundry's `forge`.
- [x] **Manifest delivery** → **embed + override**: `go:embed manifest/*.toml` into the binary (self-contained on fresh machines), with a `--manifest <path>` flag / `$DOTF_MANIFEST` env override for local dev.
- [x] **Arch/CachyOS firewall** → **`ufw`** — install + configure, giving one consistent firewall abstraction across Debian/Ubuntu and Arch; Fedora keeps `firewalld`, macOS uses `pf`/`socketfilterfw`.
- [x] **`apt-repo` exact steps** for `hashicorp-tools.sh`, `aws-tools.sh`, `azure-cli.sh` → **verify at build time (Phase 3)**. Line-read the three scripts to finalize keyring/sources.list specifics before locking the `apt-repo` schema; not a v1 product gate.
- [x] **Windows sysconfig** (`Rename-Computer`, `Set-TimeZone`, Defender firewall) → **gated/deferred**, consistent with the WSL-first, Windows-native-deferred scope. Section stub present but unimplemented.
- [x] **Dockertest distro matrix** → **deferred to nightly CI**, built only after the backend taxonomy stabilizes; PR workflow stays `go test`/`go vet`/`golangci-lint`.

---

## Users & Context

**Primary User**

- **Who**: The maintainer (Yandy), provisioning a fresh machine, VM, container, or VPS across Arch/CachyOS, macOS, Debian/Ubuntu, and WSL.
- **Current behavior**: Runs `bootstrap.sh` (guided or `--force`), then babysits ~30 installer scripts and hopes nothing drifted; edits multiple files to add a tool.
- **Trigger**: New machine, rebuild, or adding/changing a tool in the setup.
- **Success state**: Declare _what the machine is for_ (intents), preview/adjust the plan, apply guided or headless, and be productive in minutes.

**Job to Be Done**
When I get a new machine or rebuild one, I want to declare what it's for and have the right tools + dotfiles + system config applied, so I can be productive in minutes instead of babysitting a 1,848-line script and hoping nothing drifted.

**Non-Users**
Not general-purpose config management for fleets (Ansible/Nix territory), not Windows-native power users, not GUI users, not enterprise multi-host orchestration.

---

## Solution Detail

### Core Capabilities (MoSCoW)

| Priority | Capability                                                                                                                                                          | Rationale                                                                   |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| Must     | **Declarative TOML manifest** (intents → packages, per-tool multi-backend) embedded in binary                                                                       | Single source of truth; the whole point of the rewrite                      |
| Must     | **7-backend install engine**: `pkg`, `github-release`, `aur`, `toolchain` (cargo/npm), `apt-repo` (data) + `script-url`, `custom-script` (shell-out escape hatches) | Covers all 25 scripts; ~18 become pure data                                 |
| Must     | **Native Go symlink engine** — `{src,dest,type}` from TOML, with backup-if-not-symlink, dryrun, validate, broken-link detection, Zed-copy exception                 | Preserves the Ansible contract while removing the Python/Ansible dependency |
| Must     | **Interactive intent TUI** — OS detect/select → multi-select intents → pre-checked package manifest with add/remove → confirm (`charmbracelet/huh`)                 | The headline UX requirement                                                 |
| Must     | **Headless mode** (`--headless`/`--force` parity) + flag/env parity with current `bootstrap.sh` surface                                                             | Non-interactive VPS/CI installs must keep working                           |
| Must     | **System config**: hostname (`hostnamectl`/`scutil`), timezone (`timedatectl`), locale (`locale-gen`+`localectl`), default shell (`chsh`), firewall (ufw/firewalld) | Explicitly in scope per decisions; net-new capability                       |
| Must     | **Privilege model** — unprivileged default, per-command `sudo` (prime-once + keep-alive), drop to `$SUDO_USER` for yay/makepkg                                      | Correctness (makepkg refuses root) + single password prompt                 |
| Must     | **Unified platform detection** (one Go package, canonical OS/arch/pkg-mgr vocab)                                                                                    | Kills the 3-way drift                                                       |
| Should   | **Drift report** (`status`/`diff`) — installed-vs-manifest, read-only                                                                                               | Cheap (install path already enumerates installed), high visibility          |
| Should   | **`bootstrap.sh` shim** (~40 lines) downloading the release binary + checksum verify                                                                                | Replaces the 1,848-line script as the entry point                           |
| Should   | **Snapper integration parity** (`--snapper` snapshots, Linux)                                                                                                       | Preserve existing rollback feature                                          |
| Could    | **Failure-summary accumulation** (non-fatal, end-of-run report) parity                                                                                              | Preserve current `BOOTSTRAP_FAILURES` UX                                    |
| Won't    | **Package removal / reconcile**                                                                                                                                     | Deferred behind future `--prune`; additive-only v1                          |
| Won't    | **Secrets-integration orchestration**                                                                                                                               | Out of scope                                                                |
| Won't    | **Windows-native install paths**                                                                                                                                    | WSL-first; deferred                                                         |

### MVP Scope

**Full replacement at v1** (per decision): all 7 backends, Go symlink engine (Ansible retired), intent TUI with add/remove, headless mode, and system config (hostname/timezone/locale/shell/firewall) — shipped together so the shim can cut `bootstrap.sh` over to the binary. Migration is **parallel-then-cutover**: the Go tool is built alongside the untouched bash system (which stays default), reaches parity, is dogfooded on a VM, then the shim cuts over and the mechanically-migratable `bin/install-*.sh` scripts are retired **per-tool** as each moves to a native backend, while the genuine setup-step scripts are **retained permanently** behind the `custom-script` governed adapter (see [Residual setup-step category](#residual-setup-step-category-permanent-governed-adapters)).

### User Flow

```
clone repo → ./bootstrap.sh (shim) → detect OS/arch → download binary → exec
   → [guided]  detect OS → multi-select intents → pre-checked manifest (add/remove) → confirm → apply
   → [headless] --headless → apply manifest defaults for detected OS+intents, no prompts
apply = sysconfig (hostname/tz/locale/shell) → packages (per backend) → symlinks → firewall/ssh → summary
```

---

## Technical Approach

**Feasibility**: HIGH — Go 1.26 already present via mise; greenfield (no `go.mod`/CI/tests to untangle); the one declarative asset (symlink data) ports trivially.

**Architecture Notes**

- **Layout**: `cmd/dotfiles/main.go` (thin entrypoint) + `internal/{manifest,backend,platform,symlink,privilege,sysconfig,version}` + top-level `manifest/*.toml` (`go:embed`-ed). One file per backend under `internal/backend/` keeps the ~500-line soft cap.
- **Manifest schema** (TOML, matching repo convention — `starship.toml`/`aerospace.toml`/`direnv.toml`): two table families — `[intents.<name>]` (pre-checked groupings the TUI presents, with `default = true`) and `[[package]]` (logical tool → `backend` + per-distro `names` map / backend-specific fields). `default = true` maps 1:1 to `huh` `.Selected(true)`.
- **Backend taxonomy**: `pkg` (apt/pacman/dnf/zypper/brew dispatch with per-distro name maps), `github-release` (owner/repo + asset template + checksum + extract — highest-value, 11 scripts), `aur` (yay, bootstrap-from-source if absent), `toolchain` (`cargo install`/`npm i -g`), `apt-repo` (key+sources.list+install), `script-url` (vendor installer + optional pinned `sha256`), `custom-script` (path to a retained shell script — the permanent governed adapter for genuine setup steps, not a shrinking escape hatch; see [Residual setup-step category](#residual-setup-step-category-permanent-governed-adapters)).
- **Symlinks**: native Go (`os.Symlink`/`os.Lstat`), OS-faceted via the shared platform detector; preserve backup-only-if-not-symlink, dryrun, validate, broken-link, and Zed-copy semantics; keep the Ansible playbook unexecuted for one release as rollback, then delete.
- **Privilege**: `sudo -v` prime-once + `sudo -n -v` keep-alive goroutine (<5-min ticker) for pacman/apt/sysconfig; `syscall.Credential{...NoSetGroups:true}` + explicit `$HOME` to drop to `$SUDO_USER` for yay/makepkg; resolve real user from `SUDO_USER`.
- **System config canonical commands**: Linux `hostnamectl set-hostname` (also writes `/etc/hostname`; remember `/etc/hosts` is NOT auto-updated), macOS `scutil --set {ComputerName,LocalHostName,HostName}`; `timedatectl set-timezone`; `locale-gen` then `localectl set-locale` (repo already handles locale fallback per commit `6e3d0332`); `chsh` (target must be in `/etc/shells`); ufw/firewalld per distro.
- **Distribution**: GoReleaser, `CGO_ENABLED=0` (pure-Go, preserve cross-compile), `goos:[linux,darwin,windows]` × `goarch:[amd64,arm64]`, version via `-ldflags -X .../internal/version.Version`, GitHub Actions release on tag. Module path `github.com/yandy-r/dotfiles` (confirmed from git remote).
- **Reconciliation**: additive install + read-only drift report; no package removal.
- **Testing/CI** (greenfield): exec-interface unit tests (table-driven, assert exact argv per distro) + `testscript` CLI goldens + one PR workflow (`go test`/`go vet`/`golangci-lint`); defer dockertest distro matrix to nightly. Wire `gofmt`/`go vet`/`golangci-lint` into existing lefthook with a `glob: "*.go"` filter; commits scoped `feat(tool):`/`feat(go):`.

**Technical Risks**

| Risk                                                                       | Likelihood | Mitigation                                                                                                    |
| -------------------------------------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------------------------- |
| Symlink port loses an Ansible edge case (backup/Zed-copy/broken-link)      | M          | Table-driven tests against each exact case before retiring the playbook; keep playbook 1 release for rollback |
| `script-url` vendor installers change/break silently                       | M          | Optional pinned `sha256`; keep as data-driven shell-out, not native re-implementation                         |
| `apt-repo` schema wrong for hashicorp/aws/azure (not fully read)           | M          | Line-read those 3 scripts before finalizing schema (open question)                                            |
| sudo/`$SUDO_USER` drop-privilege mistakes (makepkg as root, wrong `$HOME`) | M          | `NoSetGroups:true` + explicit `$HOME`; default unprivileged + per-command escalation                          |
| Full-replacement v1 scope slips                                            | M          | Parallel-then-cutover keeps bash system working the whole time; cut over only at parity                       |
| Cross-distro install correctness unverified                                | M          | Headless VM runs on Arch/Debian/macOS as cutover gate; nightly dockertest later                               |

---

## Implementation Phases

<!--
  STATUS: pending | in-progress | complete
  PARALLEL: phases that can run concurrently
  DEPENDS: phases that must complete first
  PRP: link to generated plan file once created
-->

| #   | Phase                         | Description                                                                                     | Status      | Parallel | Depends    | PRP Plan                                               |
| --- | ----------------------------- | ----------------------------------------------------------------------------------------------- | ----------- | -------- | ---------- | ------------------------------------------------------ |
| 1   | Foundation & platform core    | `go.mod`, project layout, unified `internal/platform` detector, exec interface, CI skeleton     | complete    | -        | -          | [plan](../plans/provisioning-cli-foundation.plan.md)   |
| 2   | Manifest schema + TUI         | TOML schema (`intents`/`package`), parser, `huh` pre-checked multiselect + add/remove           | complete    | with 3   | 1          | [plan](../plans/provisioning-cli-manifest-tui.plan.md) |
| 3   | Install backend engine        | 7 backends + privilege model + drift report; absorb ~18 scripts to data                         | complete    | with 2   | 1          | [plan](../plans/provisioning-cli-install-engine.plan.md) |
| 4   | Symlink engine (Go)           | Port `{src,dest,type}` + backup/dryrun/validate/broken-link/Zed-copy                            | complete    | with 3   | 1          | [plan](../plans/phase-4-symlink-engine.plan.md)        |
| 5   | System config                 | hostname/timezone/locale/shell/firewall per-OS                                                  | complete    | -        | 1          | [plan](../plans/phase-5-sysconfig.plan.md)             |
| 6   | Headless mode + flag parity   | `--headless`/`--force`, snapper, failure summary, env parity with bootstrap                     | complete    | -        | 2, 3, 4, 5 | [plan](../plans/provisioning-cli-headless.plan.md)     |
| 7   | Distribution + shim + cutover | GoReleaser, GitHub Releases, ~40-line `bootstrap.sh` shim, VM dogfood, cut over, retire scripts | complete    | -        | 6          | [plan](../plans/phase-7-distribution.plan.md)          |

### Phase Details

**Phase 1: Foundation & platform core**

- **Goal**: Buildable Go skeleton with the one canonical platform detector and a testable exec seam.
- **Scope**: `go.mod` (`github.com/yandy-r/dotfiles`), `cmd/dotfiles/main.go`, `internal/platform` (OS/arch/pkg-mgr, canonical vocab replacing 3 copies), exec interface for mocking, `.github/workflows/go.yml` (test/vet/lint), lefthook Go branch.
- **Success signal**: `go test ./...` + `go vet` + `golangci-lint` green in CI; platform detector matches `common/platform.sh` vocabulary in tests.

**Phase 2: Manifest schema + TUI**

- **Goal**: A hand-editable TOML manifest renders as a pre-checked, add/remove-able intent selector.
- **Scope**: `internal/manifest` (TOML parse + schema + `go:embed`), `[intents.*]`/`[[package]]` model with `default`, `huh` MultiSelect wizard (OS → intents → manifest → confirm).
- **Success signal**: Selecting intents produces a correct, editable package plan; round-trips through dry-run preview.

**Phase 3: Install backend engine**

- **Goal**: Every package the old scripts install can be installed from manifest data.
- **Scope**: `internal/backend/{pkg,github_release,aur,toolchain,apt_repo,script_url,custom_script}`, `internal/privilege` (sudo keep-alive + drop), drift report (`status`/`diff`). Migrate ~18 scripts to pure data; 5 → `script-url`; 2–3 → `custom-script`.
- **Success signal**: Table-driven tests assert exact argv per (distro × backend); a sample tool installs on a VM.

**Phase 4: Symlink engine (Go)**

- **Goal**: Symlinks created/validated natively; Ansible no longer required.
- **Scope**: `internal/symlink` port of `playbooks/vars/symlinks.yml` data + semantics; dryrun/backup/validate/broken-link/Zed-copy.
- **Success signal**: Produces identical link set to the playbook on a test tree; edge-case tests pass; playbook retained (unexecuted) for rollback.

**Phase 5: System config**

- **Goal**: Tool owns hostname/timezone/locale/shell/firewall.
- **Scope**: `internal/sysconfig` per-OS implementations; `/etc/hosts` update alongside hostname; `locale-gen` before `set-locale`; `chsh` with `/etc/shells` guard.
- **Success signal**: Sets each on a VM idempotently; re-runs are no-ops.

**Phase 6: Headless mode + flag parity**

- **Goal**: Non-interactive installs match today's `bootstrap.sh --force` behavior and surface.
- **Scope**: `--headless`/`--force`, `--snapper`, `--gaming`, `--linux-desktop`, `--enable-ollama`, `--homebrew=`, env equivalents; non-fatal failure accumulation + summary.
- **Success signal**: Headless VM run on Arch + Debian + macOS reaches the same end state as the bash flow.
- **Status**: **complete** — delivered as `dotf apply` (`cmd/dotf/apply.go` + `internal/failure`, `internal/snapper`); sysconfig → packages → symlinks → firewall in PRD order, flag/env parity (flag OR env, truthy `"true"`), opt-in snapper pre-apply snapshot, run-wide non-fatal failure summary with exit-code taxonomy (0 converged / 1 soft-failure / 2 usage / 3 hard-abort). Firewall/shell stay manifest-driven (no invented apply-only defaults); `--enable-ollama` is wired but a no-op until the manifest defines the intent; SSH-server enablement deferred. `dotf apply` is dogfooded but not yet wired into `bootstrap.sh` (Phase 7 cutover). See [plan](../plans/provisioning-cli-headless.plan.md).

**Phase 7: Distribution + shim + cutover**

- **Goal**: One command on a fresh machine downloads the binary and provisions; bash retired.
- **Scope**: `.goreleaser.yml`, release workflow on tag, ~40-line `bootstrap.sh` shim (detect os/arch, download asset, checksum, exec), dogfood, cutover, begin retiring `bin/install-*.sh`.
- **Success signal**: Clone → `./bootstrap.sh` → fully provisioned on a clean VM with no pre-installed runtime. Script retirement then proceeds **per-tool** (see [Success Metrics](#success-metrics)) — each mechanically-migratable installer deleted on cutover, `custom-script` converging to its residual target — rather than chasing an aggregate ≥80% LOC figure.
- **Status**: **complete** — delivered as `.goreleaser.yml` (`CGO_ENABLED=0`, linux/darwin/windows × amd64/arm64, `-ldflags -X …/internal/version.Version`/`.Commit`, version-free archive names + `checksums.txt`), a GitHub Actions release-on-tag workflow (`.github/workflows/release.yml`, GoReleaser v2), and a POSIX `bootstrap.sh` download-and-exec shim (~307 lines — OS/arch detection, release download, sha256 verification, and a from-source fallback; detect OS/arch → download `…/releases/latest/download/<asset>` → sha256-verify → `exec dotf apply`, injecting the mandatory `apply` subcommand). A safe no-release fallback prints an actionable message + a local-build escape hatch (`go build -o dotf ./cmd/dotf`); a fresh clone never hard-crashes. **Conservative (Option A) cutover:** the shim replaces the bash body, but no `bin/install-*.sh` is retired (the manifest is still a 7-package skeleton — SAFE_TO_DELETE is empty), so only ~1.8k bash LOC (~12%) are removed here. VM dogfood and the first `v0.1.0` tag are maintainer gates; per-tool script retirement is the manifest-population follow-up, and the aggregate ≥80%-bash-LOC metric has been **retired** in favor of a per-tool delete-on-cutover metric with a `custom-script` ≤ ~5 residual target (see [Success Metrics](#success-metrics) / [Residual setup-step category](#residual-setup-step-category-permanent-governed-adapters)). See [plan](../plans/phase-7-distribution.plan.md).

### Parallelism Notes

Phases 2, 3, and 4 all depend only on Phase 1 and touch disjoint `internal/` packages (`manifest`/TUI, `backend`, `symlink`), so they can proceed concurrently after the foundation lands. Phase 5 (sysconfig) also depends only on Phase 1 and can overlap. Phase 6 is the integration barrier (needs 2–5), and Phase 7 is the final cutover (needs 6).

---

## Decisions Log

| Decision              | Choice                                     | Alternatives                                     | Rationale                                                                                                                                                                       |
| --------------------- | ------------------------------------------ | ------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Language              | **Go**                                     | Rust, Python, Bash+gum, Nushell/Deno             | Single static binary (zero runtime on fresh machines), entire comparable ecosystem is Go (chezmoi/aqua/gum), best checklist-TUI, mockable exec, lower contributor bar than Rust |
| Manifest format       | **TOML**                                   | YAML, Nix, Lua, code-as-manifest                 | Matches repo convention (starship/aerospace/direnv/topgrade `.toml`), less whitespace-fragile for hand-editing, clean intent+backend tables                                     |
| TUI library           | **charmbracelet/huh**                      | bubbletea raw, inquire/dialoguer (Rust), gum     | Per-option `.Selected(true)` maps 1:1 to a manifest `default` flag; composes multi-step form                                                                                    |
| Symlinks              | **Port to Go**                             | Keep shelling to Ansible                         | Keeping Ansible contradicts the zero-dependency goal; data is trivially portable; ~200 LoC                                                                                      |
| Reconciliation        | **Additive + drift report**                | Pure additive, full reconcile (remove)           | Removal is high blast-radius for a solo tool; chezmoi stays additive deliberately; drift report is nearly free                                                                  |
| Privilege             | **Unprivileged default, per-command sudo** | Run whole tool as root                           | makepkg/yay refuse root; single password prompt via keep-alive; per-command escalation is cleaner                                                                               |
| Distribution          | **Shim downloads prebuilt binary**         | Build on the fly (needs Go), commit binary       | ~40-line shim replaces 1,848-line script; no toolchain on fresh machine                                                                                                         |
| MVP scope             | **Full replacement at v1**                 | Engine+TUI then sysconfig, thin Arch-first slice | Cleanest end state; cut over only at parity so risk is bounded by migration strategy                                                                                            |
| Migration             | **Parallel, then cutover**                 | Strangler-fig (Go entry early, delegate)         | Bash system stays default and working until parity; safest for a single-maintainer daily-driver repo                                                                            |
| Repo slug             | **github.com/yandy-r/dotfiles**            | —                                                | Confirmed from `git remote -v`                                                                                                                                                  |
| Binary name           | **`dotf`**                                 | `forge`, `bootstrap`, `env`                      | Short, dotfiles-evident; `env` collides with the POSIX binary, `forge` with Foundry; locks `cmd/dotf/main.go`                                                                   |
| Manifest delivery     | **Embed + override**                       | Embed-only, read-from-checkout                   | `go:embed` is self-contained on fresh machines (shim ships only a binary); `--manifest`/`$DOTF_MANIFEST` keeps dev iteration fast without rebuilds                              |
| Arch/CachyOS firewall | **`ufw`**                                  | `firewalld`, leave unmanaged                     | One firewall abstraction shared with Debian/Ubuntu; Arch ships no default so picking ufw over firewalld keeps rule syntax consistent across the two most-used Linux paths       |

---

## Research Summary

**Market Context**
The design (OS + intents → pre-checked manifest → add/remove → headless) is essentially **archinstall's profile system** (declarative, unattended-capable) layered on **omakub's per-app installer organization**, generalized cross-platform — validating the approach; neither existing tool is cross-OS the way this needs. **chezmoi** (Go, single binary, `.chezmoidata` + `run_onchange` templated installs) and **aqua/mise** (declarative "one tool → N platforms" registry with `{{.OS}}`/`{{.Arch}}` templating) are the mature references — all Go. chezmoi/archinstall are deliberately **additive-only** for packages (never remove deleted entries), which directly informs the v1 reconciliation stance. Sources: chezmoi.io, aquaproj.github.io, mise.jdx.dev, archinstall (DeepWiki), basecamp/omakub & omarchy, charmbracelet/huh.

**Technical Context**
The 25 scripts collapse to a **7-backend taxonomy** (5 pure-data + 2 shell-out escape hatches), with `github-release` and `pkg` covering the bulk and ~18/25 becoming data. Symlink logic is **~200 LoC of Go** with the data already declarative in `playbooks/vars/symlinks.yml`; retiring Ansible removes a heavy fresh-machine dependency. Privilege handling needs `sudo` keep-alive + `syscall.Credential{NoSetGroups:true}` drop to `$SUDO_USER` for AUR builds. Testing (none exists today) is best served by an exec-interface mock + table-driven argv assertions + `testscript` goldens, with a deferred dockertest distro matrix. Layout: `cmd/` + `internal/` + embedded `manifest/*.toml`, GoReleaser `CGO_ENABLED=0` cross-compile on tag, wired into existing lefthook/commitlint. Canonical sysconfig commands grounded per-OS (`hostnamectl`/`scutil`, `timedatectl`, `locale-gen`+`localectl`, `chsh`, ufw/firewalld).

---

_Generated: 2026-06-30T20:33:38Z_
_Decisions finalized: 2026-06-30_
_Status: READY — open questions resolved, ready for issue breakdown_
