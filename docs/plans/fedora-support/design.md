# Fedora Support Design

Fedora is not a detection problem. `internal/platform/platform.go:91` already maps os-release `ID=fedora` to
`platform.OSRedHat`, `internal/backend/pkg.go:130,186` already emits correct `sudo dnf install -y` / `rpm -q` argv,
`internal/sysconfig/linux.go:498` already installs `openssh-server` via dnf and routes RedHat to firewalld,
`.goreleaser.yml` already builds `linux/arm64` with `CGO_ENABLED=0`, and `bootstrap.sh:167` already resolves
`aarch64` to the `dotf_linux_arm64.tar.gz` asset. The M1 will classify itself correctly on the first try. What
actually breaks, **in the order the user will hit it**, is: (0) the binary may never download, because
`github_download_release_file` returns 1 at `bootstrap.sh:401` without ever trying the tag-pinned URL it just
resolved; (1) `dotf apply` exits 1 because `manifest/packages.toml:157,176` assert `dnf = "lazygit"` and
`dnf = "yazi"` — RPMs that exist in no Fedora branch; (2) the shell layer is silently half-dead because
`common/aliases.sh:856` sources `common/$DOTFILES_OS/aliases.sh` and `common/redhat/` does not exist, and because
Fedora's bash is not built with `-DSYS_BASHRC`, so replacing `~/.bashrc` severs `/etc/profile.d` for the exact
non-login shell a Konsole window starts; (3) on a later run, once the user has customised anything,
`internal/symlink/symlink.go:206-209` recursively deletes `~/.config/nvim` and six siblings **with no backup**,
because `BackupEligible` is set on only the 14 single-file entries.

The plan is therefore ordered by what stops the user, not by architectural tidiness. Sysconfig's real Fedora gaps
(langpacks, firewalld install, the polkit-gated convergence probe) are sequenced **late** — verified reason:
`manifest/packages.toml:548-563` ships the `[sysconfig]` table commented out, so a bare `dotf apply` plans zero
sysconfig steps and none of those gaps is on the first-boot path.

## Relevant Files

- `/bootstrap.sh`: 593-line POSIX download-and-exec shim (the docs' "~307 lines" is stale). Owns OS/arch detection
  at `:157-169`, asset naming at `:245`, and the private-repo/prerelease download fallback at `:382-421`.
- `/internal/platform/platform.go`: canonical detection. `osFamilyFromID` `:87-112`; `detectPkgMgr` `:175-194`.
- `/common/platform.sh`: the string-locked shell mirror. `detect_os` `:50-87`; `detect_arch` `:102-109`.
- `/internal/backend/backend.go`: `osGateAllows` `:88-101`, `platformGated` `:106-108`, `Engine.Applicable` `:145-157`.
- `/internal/backend/pkg.go`: the four `PkgMgr` argv switches; **no `applicable()` method** (only `apt_repo.go:48`
  and `aur.go:36` implement it).
- `/internal/backend/apt_repo.go`: the Debian-only third-party-repo backend and the template for its generalisation.
- `/cmd/dotf/status.go`: `driftReport` `:95-136` — a probe error aborts the whole report (`:105-107`, `:117-120`).
- `/cmd/dotf/applyphases.go`: phase applicability. `applySnapshot` `:23-36`; `printApplyDryRun` `:417-431`.
- `/manifest/packages.toml`: 36 packages, 8 intents. The two fictional dnf names live at `:157` and `:176`.
- `/manifest/dotf.lock`: exactly one artifact, keyed `os = 'arch'` / `arch = 'amd64'`.
- `/internal/manifest/ratchet_test.go`: `customScriptBaseline = 16` at `:22` — an exact-match tripwire both ways.
- `/internal/manifest/manifest_test.go`: `supportedPkgMgrs` `:364`; the coverage test `:450-472`.
- `/internal/symlink/table.go`: `Facet` enum `:22-35`; the seven unprotected directory entries; `isLinuxFamily` `:143-150`.
- `/internal/sysconfig/linux.go`: 554 lines (over the ~500 soft cap). `planLocale` `:115-153`, `chsh` `:228-229`,
  `planFirewalld` `:345-370`, `ufwInstallArgv` `:372-378`, `sshInstallArgv` `:493-505`.
- `/internal/plasma/reload.go`: `qdbusCandidates = []string{"qdbus6", "qdbus"}` at `:43` — no Fedora candidate.
- `/internal/brew/brew.go`: `Applicable` `:32-41` — no `info.Arch` reference at all.
- `/.github/workflows/go.yml`: the only Go CI. Path filters `:5-11` / `:15-21` exclude `bootstrap.sh`.

## Relevant Docs

**`/docs/plans/arch-os-enhance/parallel-plan.md`**: You _must_ read this for the house plan grammar and, more
importantly, for its closing Advice — "the major risk is not package detection but destructive deployment" applies
verbatim here, with `dotf link` in place of the Ansible playbook.

**`/docs/reference/manifest-backends.md`**: You _must_ read this before proposing any backend change; it is the
governed decision ladder and the `custom-script` fence that freezes `customScriptBaseline`.

**`/.docs/arch-package-mapping.md`**: You _must_ read this before writing the Fedora mapping table; it fixes the
column shape (package | distro package | repository provenance | notes) this design mirrors.

**`/docs/architecture/platform-detection.md`**: You _must_ read this when touching detection — and you must
**correct** it, because it documents only the shell detector and omits `rocky`/`almalinux`, every Arch derivative,
the whole `suse` arm, and the entire `ID_LIKE` fallback.

**`/monitor-auto-switch/testing.md`**: You _must_ read this as the model for the M1 first-run manual checklist;
it is the repo's only precedent for a hand-written hardware verification doc.

**`https://asahilinux.org/fedora/`**: You _must_ read this when confirming the target release and the installer's
partition/subvolume layout.

**`https://docs.fedoraproject.org/en-US/quick-docs/dnf5-changes/`**: You _must_ read this before writing any
`dnf config-manager` argv — the repo verbs, not the install verbs, are what changed in dnf5.

---

## 1. Goal and Non-Goals

### Goal

Bring Fedora to first-class parity with the existing macOS / Arch / Debian support, satisfying the two explicit
user requirements:

1. **Same install flow as others.** Fedora goes through the identical
   `bootstrap.sh` → `dotf apply` → sysconfig / packages / symlinks / plasma pipeline. No bespoke side path, no
   Fedora-only entry point, no `bin/install-fedora-pkgs.sh`.
2. **Dotfiles support.** The shell/config/symlink layer works correctly on Fedora — which, as this design
   discovered, currently means fixing a loader bug that silently disables `common/linux/aliases.sh` on Fedora,
   **Debian and Arch alike**.

Success is a single measurable acceptance test: clone on the M1, run `./bootstrap.sh`, reach a working
environment, and then be able to iterate on that box repeatedly without fear of data loss.

### Non-Goals (scope fence)

These are deliberately excluded. Each is a decision, not an oversight.

| Excluded                                                                            | Why                                                                                                                                                                                                                                                                                                                                             |
| ----------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A distinct `platform.OSFedora` / `OSAsahi` constant                                 | Fedora **is** `OSRedHat`. A new value ripples through `sysconfig/register.go`, `symlink/table.go`'s facet switch, `backend.go:osGateAllows`, `validate.go:knownPackageOS`, `tui/plan.go:knownOSValues` and a dozen test tables for zero behavioural gain.                                                                                       |
| rpm-ostree / Silverblue / Kinoite / Bazzite support                                 | The target device is a traditional rpm/dnf Fedora Asahi Remix install. Immutable hosts need a new `PkgMgr` value plus four backend switches, for a host nobody is testing.                                                                                                                                                                      |
| Implementing **or** removing `platform.PkgMgrYUM`                                   | Orthogonal: Fedora always has dnf, so `PkgMgrYUM` can never fire there. WS2's `applicableFor` change downgrades a yum-only host from "aborts `dotf status`" to "clean not-applicable", which is sufficient. Removing it reaches into `common/tools.sh`, `common/linux/aliases.sh` and manifest testdata — scope creep on a Fedora change.       |
| A `platform.Info` capability table (`Caps`) with `Immutable` / `SystemRC` fields    | Fields with no call site are speculative surface. Only `VersionID` is added, and only in the workstream that needs `$releasever`.                                                                                                                                                                                                               |
| aarch64 CI of any kind, and an arm64+systemd emulated environment                   | The binary is a CGO-free cross-compile with no arch-specific codegen; hosted arm64 runners are paid on a private repo; a self-hosted runner on the M1 would invert the safety property this plan protects. arm64 + systemd is unreachable under `qemu-user` (D-Bus dies `226/NAMESPACE`), so that intersection belongs to the device checklist. |
| Porting `bin/setup-fingerprint.sh` to Fedora/authselect                             | Touch ID runs through Apple's Secure Enclave and sits in Asahi's "TBA — no active work" tier. A correct Fedora implementation would be dead code on the target device. Gate the module, do not implement it.                                                                                                                                    |
| RPM Fusion enablement                                                               | Nothing in the manifest needs it; every resolving dnf name comes from Fedora's own release/updates repos. Adding a privileged third-party repo speculatively is worse than not having it.                                                                                                                                                       |
| Retiring or wiring the orphaned `bin/install-*.sh` scripts                          | `install-docker.sh`, `install-native-packages.sh`, `install-arch-pkgs.sh`, `install-modern-cli.sh`, `install-brew.sh` are unreachable from `bootstrap.sh → dotf apply`. Fixing their Fedora branches would be fixing code nothing runs.                                                                                                         |
| Unifying `internal/symlink`'s private `isLinuxFamily` with `platform.IsLinuxFamily` | The `OSLinux` exclusion at `table.go:143-150` is deliberate and pinned by a golden test row. Any new `FacetLinux` must resolve via the **local** predicate.                                                                                                                                                                                     |
| Wiring the `sysconfig` `[[module]]` step kind and its `ports` list                  | `dotf setup` is not reachable from the bootstrap pipeline at all, so it cannot affect requirement (1).                                                                                                                                                                                                                                          |
| Threading `privilege.Sudo` into the backends                                        | `internal/backend/register.go:15` is literally `_ = sudo`. Real debt, cross-cutting, orthogonal. New code follows the existing literal-`"sudo"` convention so a future fix stays one refactor.                                                                                                                                                  |
| A dependency graph on `Package`                                                     | The npm/cargo toolchain entries (`codex`, `lefthook`, `tmux-sessionizer`) have no provisioned node/cargo on **any** distro. Pre-existing cross-platform hole.                                                                                                                                                                                   |
| `ROADMAP.md`, `CHANGELOG.md` backfill, the full 20-file doc drift pass              | Worth doing; not on the first-boot path. This design ships the two docs the rollout actually needs.                                                                                                                                                                                                                                             |
| Speculative tests                                                                   | See §7. Anything not in the critical set is explicitly out of scope per the "no unnecessary tests" instruction.                                                                                                                                                                                                                                 |

---

## 2. Target Device and Exact Constraints

| Property        | Value                                                                         | Consequence for this design                                                                                                                                                                                            |
| --------------- | ----------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Hardware        | Apple MacBook Pro, M1 (Apple Silicon)                                         | `uname -m` = `aarch64`. Homebrew-on-Linux is unavailable (no aarch64 bottles).                                                                                                                                         |
| Distro          | Fedora Asahi Remix (KDE spin)                                                 | os-release `ID` is **contested** — see WS0. Resolves to `platform.OSRedHat` under every candidate reading.                                                                                                             |
| Kernel          | Asahi kernel with **16 KiB pages**                                            | Go's arm64 linker uses `0x10000` PT_LOAD alignment (a multiple of 16 KiB) and `.goreleaser.yml` sets `CGO_ENABLED=0`, so `dotf` itself loads correctly. The risk surface is C-linked third-party binaries, not `dotf`. |
| Desktop         | KDE Plasma 6 on Wayland                                                       | `KDE_FULL_SESSION=true`, `XDG_CURRENT_DESKTOP=KDE`, `XDG_SESSION_TYPE=wayland`. Settings binary is `systemsettings`, not `systemsettings5`. Clipboard is `wl-copy`/`wl-paste`.                                         |
| Package manager | dnf5 (Fedora 41+ ships `/usr/bin/dnf` as dnf5)                                | Install/upgrade/remove/`rpm -q` argv are source-compatible across dnf4/dnf5 — **the pkg backend needs no change**. Only the _repo_ verbs diverged.                                                                     |
| Firewall        | firewalld (never ufw)                                                         | `internal/sysconfig/linux.go:254` already routes `FirewallAuto` on RedHat to firewalld. Fedora Workstation/KDE default to the `FedoraWorkstation` zone, which already permits ssh.                                     |
| SELinux         | Enforcing, targeted policy                                                    | `internal/symlink` writes only under `$HOME`; `sysconfig`'s `sudo cp` writes into existing `/etc` inodes, preserving labels. Exposure is low but must be observed, not assumed.                                        |
| Filesystem      | btrfs, root subvolume literally named `root` (Fedora convention, **not** `@`) | No snapper, no snapper config by default. `--snapper` is a no-op that promises a rollback it cannot deliver.                                                                                                           |
| Status          | **Daily driver**, already populated                                           | `~/.config/nvim` and siblings exist. The un-backed-up recursive delete in `dotf link` is a live data-loss risk, not a theoretical one.                                                                                 |

**The device is the only place several facts can be established.** Four surveys contradicted each other on the
os-release `ID`, on whether `chsh` ships in base `util-linux` on this release, and on the target Fedora release
number (f43 vs f44 package datasets). This design resolves those by **measurement** (WS0), not by argument, and
every fact-dependent decision below is either deferred behind that capture or written to be correct under every
candidate reading.

---

## 3. Current-State Assessment

| Subsystem                                 | Status                               | Verdict                                                                                                                                                                                                                                        |
| ----------------------------------------- | ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Release artifacts (arm64)                 | **fully supported**                  | `.goreleaser.yml` builds `linux/arm64` CGO-free; `bootstrap.sh:167` maps `aarch64`→`arm64`; `internal/platform/types.go:76-87` `ReleaseArchAliases` covers `{aarch64, arm64}`. Nothing to do.                                                  |
| Binary **download**                       | **partially supported**              | `bootstrap.sh:401` returns 1 on an empty token without ever trying the tag-pinned URL. Prerelease-only releases mean `/releases/latest/download` 404s.                                                                                         |
| Platform detection (Go + shell)           | **partially supported**              | `ID=fedora` → `OSRedHat` works. Asahi may survive only via the `ID_LIKE` fallback at `platform.go:103`, which is incidental, untested, and adjacent to a silent-failure mode.                                                                  |
| pkg backend (dnf argv)                    | **fully supported**                  | `pkg.go:130/149/168/186` are dnf5-compatible verbatim. Do not "fix" them.                                                                                                                                                                      |
| pkg backend **applicability**             | **absent**                           | `pkgBackend` has no `applicable()`, so a missing `names[dnf]` key becomes `MissingPkgNameError` and `status.go:117-120` aborts the whole `dotf status`/`diff` run. Fail-late.                                                                  |
| Manifest package data                     | **partially supported**              | 11/13 pkg entries carry a real Fedora name. `lazygit` (`:157`) and `yazi` (`:176`) do not. `docker-ce` is apt-only; `lutris` is Arch-only.                                                                                                     |
| Third-party repos (COPR / vendor `.repo`) | **absent**                           | `apt_repo.go:48` gates on apt; there is no dnf/COPR counterpart. No rung on the ladder above `custom-script`.                                                                                                                                  |
| Lockfile coverage                         | **partially supported**              | `manifest/dotf.lock` holds one artifact at `os='arch' arch='amd64'`. A redhat/arm64 host misses every pin and installs off a live API call (still checksum-verified via the sidecar — see §5, ADR-9).                                          |
| sysconfig: firewall + ssh-server          | **fully supported**                  | `linux.go:254` firewalld routing, `:498` `dnf install -y openssh-server`, `:484-490` `sshd` unit name. Already correct.                                                                                                                        |
| sysconfig: locale                         | **absent on Fedora**                 | `linux.go:127` gates generation on `LookPath("locale-gen")`; Fedora has neither `locale-gen` nor `/etc/locale.gen`. Silently downgrades LANG to `C.UTF-8`/`C` while reporting `ok`.                                                            |
| sysconfig: shell (`chsh`)                 | **partially supported**              | `linux.go:228-229` hardcodes `sudo chsh -s` with no `LookPath` guard. Contested whether `chsh` is present.                                                                                                                                     |
| sysconfig: ufw install argv               | **partially supported**              | `ufwInstallArgv` (`:372-378`) returns `sudo apt-get install -y ufw` for every non-Arch host — wrong on Fedora. Has drifted from `sshInstallArgv` (`:493-505`), which is correct.                                                               |
| Symlink engine (Fedora applicability)     | **partially supported**              | FacetAll works; `common/systemd/user/tmux.service` is gated `FacetArch` (`table.go:116`) despite being distro-agnostic, so Fedora loses it.                                                                                                    |
| Symlink engine (**backup safety**)        | **absent**                           | Seven `FacetAll` directory entries carry no `BackupEligible` and are recursively removed even under `--backup`.                                                                                                                                |
| Shell layer (per-OS module loader)        | **absent on every distro**           | `common/aliases.sh:856`, `common/functions.sh:834`, `common/exports.sh:598` source `common/$DOTFILES_OS/…`; only `common/linux/` and `common/macos/` exist, so 495 lines of `common/linux/aliases.sh` are dead on Fedora, Debian **and** Arch. |
| Shell layer (`/etc/profile.d`)            | **absent on Fedora**                 | Fedora's bash is not built with `-DSYS_BASHRC`; replacing `~/.bashrc` severs `/etc/bashrc` and therefore `/etc/profile.d` for interactive non-login shells.                                                                                    |
| Shell layer (locale)                      | **partially supported**              | `common/exports.sh:236-255` compares `de_DE.UTF-8` against `locale -a`'s `de_DE.utf8` without collapsing the codeset, then force-exports `LC_ALL`. Cross-platform bug, masked for `en_US` users.                                               |
| Plasma engine (gating)                    | **fully supported**                  | `plasma.Support` is `IsLinuxFamily`; per-key merge; atomic same-dir write is SELinux-safe.                                                                                                                                                     |
| Plasma engine (reload)                    | **absent on Fedora**                 | `reload.go:43` probes only `{"qdbus6", "qdbus"}`. Fedora's `qt6-qttools` ships only `/usr/bin/qdbus-qt6`, so the KWin reconfigure silently resolves nothing.                                                                                   |
| Plasma snapshot content                   | **partially supported**              | `plasma/files/mimeapps.list` pins `google-chrome.desktop` for `text/html` and `http`/`https` — a desktop id with no aarch64 Linux build — and apply merges those keys into the live file.                                                      |
| Homebrew bootstrap                        | **partially supported**              | `brew.go:32-41` gates on OS family + tristate with **no** `info.Arch` check; `detectPkgMgr:180` probes brew first with no OS/arch gate.                                                                                                        |
| Snapper phase                             | **partially supported (misleading)** | `applyphases.go:23-26` states failures are _"NEVER recorded in the failure collector"_, so `--snapper` on Fedora exits 0 having produced no rollback point.                                                                                    |
| `dotf apply --dry-run` honesty            | **partially supported**              | `applyphases.go:429` groups packages by backend with no `engine.Applicable` call, so Fedora previews `plan.apt-repo=docker-ce` for a package `apt_repo.go:48` guarantees will be skipped.                                                      |
| CI coverage                               | **absent**                           | One `ubuntu-latest` job. No Fedora, no shellcheck, and `bootstrap.sh` is not in the path filters — so `TestAssetNaming_MatchesBootstrap` never runs when only the shim changes.                                                                |

---

## 4. Fedora Package Mapping

> **Provenance discipline.** Package **names** are stable across recent Fedora branches and are what this table
> commits to. Version numbers sampled during research disagreed (one dataset reported `fc43` NEVRs, another
> `fc44`), so **no version is asserted here**. WS0 re-runs `dnf list --available` on the device and this table's
> header records the release and date it was verified against before the manifest is finalised. This table is the
> seed for `.docs/fedora-package-mapping.md`, which mirrors `.docs/arch-package-mapping.md`'s column shape.

Legend for **Fedora path**: `official` = Fedora release/updates repos · `copr:<owner>/<project>` · `github-release`
= upstream release asset via the existing backend · `vendor .repo` = third-party dnf repository · `toolchain` =
language package manager · `n/a` = correctly not-applicable on Fedora.

### pkg backend (13 entries)

| Package     | Line       | Current `dnf` name | Fedora path                                             | aarch64 | Verdict                                                                                      |
| ----------- | ---------- | ------------------ | ------------------------------------------------------- | ------- | -------------------------------------------------------------------------------------------- |
| ripgrep     | `:109-112` | `ripgrep`          | official                                                | yes     | correct                                                                                      |
| eza         | `:114-117` | `eza`              | official (updates)                                      | yes     | correct                                                                                      |
| bat         | `:119-122` | `bat`              | official                                                | yes     | correct — Fedora's binary is `bat` (Debian's is `batcat`), so no alias shim                  |
| fd          | `:124-127` | `fd-find`          | official                                                | yes     | correct — Fedora's `fd-find` ships `/usr/bin/fd` (Debian ships `fdfind`)                     |
| fzf         | `:129-132` | `fzf`              | official                                                | yes     | correct                                                                                      |
| delta       | `:134-137` | `git-delta`        | official                                                | yes     | correct — Fedora also ships an unrelated package literally named `delta`; do **not** shorten |
| zoxide      | `:139-142` | `zoxide`           | official                                                | yes     | correct                                                                                      |
| jq          | `:144-147` | `jq`               | official                                                | yes     | correct                                                                                      |
| gh          | `:149-152` | `gh`               | official                                                | yes     | correct — Fedora packages it as `gh` (Arch uses `github-cli`); no vendor repo needed         |
| **lazygit** | `:154-157` | `lazygit`          | **ABSENT** → `github-release` (`jesseduffield/lazygit`) | yes     | **BROKEN** — see ADR-3                                                                       |
| tmux        | `:159-162` | `tmux`             | official                                                | yes     | correct                                                                                      |
| **yazi**    | `:173-176` | `yazi`             | **ABSENT** → `github-release` (`sxyazi/yazi`)           | yes     | **BROKEN** — see ADR-3                                                                       |
| direnv      | `:178-181` | `direnv`           | official                                                | yes     | correct                                                                                      |

### Non-pkg backends (23 entries)

| Package          | Line       | Backend                       | Fedora path                                                                                                           | Verdict                                         |
| ---------------- | ---------- | ----------------------------- | --------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------- |
| neovim           | `:164-171` | custom-script                 | installer emits `linux-arm64`; Fedora also ships `neovim` (official)                                                  | works unchanged                                 |
| atuin            | `:289-295` | custom-script                 | official `atuin` exists — future ratchet-**down** candidate                                                           | works; see §8 note on the AppImage desktop path |
| mise             | `:298-304` | custom-script                 | absent from Fedora; `copr:jdxcode/mise`, or upstream RPM repo                                                         | vendor installer works on aarch64               |
| rust             | `:310-316` | custom-script                 | rustup supports `aarch64-unknown-linux-gnu`; Fedora also ships `rustup`                                               | works unchanged                                 |
| lua              | `:328-336` | custom-script                 | installer already has a dnf branch; `lua`, `lua-devel`, `luarocks` official                                           | works unchanged                                 |
| claude           | `:339-346` | custom-script                 | vendor installer, arm64 asset                                                                                         | works unchanged                                 |
| codex            | `:352-356` | toolchain/npm                 | needs `nodejs` + `nodejs-npm` (official, versioned as `nodejs22`/`nodejs24`) — **not provisioned on any distro**      | pre-existing cross-platform hole (non-goal)     |
| opencode         | `:358-365` | custom-script                 | vendor installer, arm64 asset                                                                                         | works unchanged                                 |
| fabric           | `:372-379` | custom-script                 | `go install`; needs Go on PATH — not provisioned on any distro                                                        | pre-existing hole                               |
| coderabbit       | `:381-388` | custom-script                 | vendor installer; upstream `linux-arm64` availability unverified                                                      | **arm64 risk**, opt-in `ai` intent only         |
| aws-tools        | `:393-399` | custom-script                 | AWS CLI v2 + SAM publish aarch64; `ecs-cli` is x86_64-only and EOL, so the all-present probe can never satisfy        | opt-in `cloud` intent; pre-existing             |
| azure-cli        | `:403-409` | custom-script                 | official `azure-cli` (noarch) — ratchet-down candidate; the script's RHEL-9 Microsoft repo path has no aarch64 RPM    | opt-in `cloud` intent                           |
| hashicorp-tools  | `:412-418` | custom-script                 | vendor `.repo` at `rpm.releases.hashicorp.com/fedora/hashicorp.repo`; script's `yum-config-manager` is absent on dnf5 | opt-in `cloud` intent                           |
| container-tools  | `:421-427` | custom-script                 | no distro branches; arm64 tokens match. `hadolint` also official                                                      | works unchanged                                 |
| turso            | `:436-442` | github-release                | upstream publishes `turso-cli_Linux_arm64.tar.gz` + `checksums.txt`                                                   | works; lock miss → sidecar verification         |
| tea              | `:445-451` | custom-script                 | has a `redhat` branch falling back to a binary download                                                               | works unchanged                                 |
| lefthook         | `:456-459` | toolchain/npm                 | same latent node/npm dependency as `codex`                                                                            | pre-existing hole                               |
| tpm              | `:466-470` | git-clone                     | OS-agnostic https clone; needs `git` (official)                                                                       | works unchanged                                 |
| tmux-sessionizer | `:474-480` | custom-script                 | `cargo install`; needs the `runtimes` intent's rust first                                                             | pre-existing cross-intent ordering hole         |
| gnu-utilities    | `:490-497` | custom-script, `os = "macos"` | correctly not-applicable                                                                                              | no work                                         |
| **lutris**       | `:227-229` | aur                           | official `lutris` **exists on Fedora**                                                                                | Arch-gated; see ADR-4                           |
| **docker-ce**    | `:249-253` | apt-repo                      | vendor `.repo` at `download.docker.com/linux/fedora/docker-ce.repo`; Fedora also ships `moby-engine` and `podman`     | apt-gated; see ADR-5                            |
| starship         | `:261-264` | script-url                    | upstream publishes `starship-aarch64-unknown-linux-musl.tar.gz`; `copr:atim/starship` exists                          | works unchanged                                 |

### Summary

- **11 of 13** pkg entries are Fedora-correct today.
- **2** are outright wrong (`lazygit`, `yazi`) and cause a non-zero exit on every Fedora `dotf apply`.
- **2** are structurally unreachable (`docker-ce`, `lutris`) and vanish **silently** as `not-applicable`.
- Everything else either works or degrades in ways that are not Fedora-specific.

---

## 5. Design Decisions (ADR)

### ADR-1 — Fedora stays `platform.OSRedHat`; all Asahi ID candidates are listed explicitly

**Decision.** Do not add `OSFedora`. Extend the explicit case list at `internal/platform/platform.go:91` to
`case "fedora", "fedora-asahi-remix", "asahi-fedora-remix", "rhel", "centos", "rocky", "almalinux"`, and mirror the
same three tokens into `common/platform.sh:60`.

**Rationale.** The family enum is the dispatch axis and it is already correct. Four surveys disagreed on the Asahi
`ID`: three claim `fedora`, one claims `fedora-asahi-remix` (citing the shipped release RPM's payload), and a fifth
source cites the archived-2025 prototype spelling `asahi-fedora-remix`. Under the `ID=fedora` reading the explicit
case fires; under the others, only the `strings.Contains(idLike, "fedora")` fallback at `platform.go:103` fires.
Both produce `OSRedHat` today — but only **by accident** under the second reading. This matters because
`internal/symlink/table.go:143-150`'s private `isLinuxFamily` deliberately excludes `OSLinux`, so any future
tightening of the `ID_LIKE` branch would silently drop **every** Linux-facet symlink with no error. Listing all
three candidates costs three string literals and is correct under every reading.

**Rejected.** (a) Introducing `OSFedora` — ripples through eight switch sites plus every per-OS test table for zero
gain. (b) Picking one token and hoping — WS0 has not yet run.

### ADR-2 — `names` becomes the applicability declaration for the `pkg` backend

**Decision.** Add one optional interface to `internal/backend/backend.go`:

```
packageGated interface { applicableFor(pkg manifest.Package, info platform.Info) bool }
```

implemented **only** by `pkgBackend` as `_, ok := pkg.Names[string(info.PkgMgr)]; return ok`, and checked in
`Engine.Applicable` **after** `osGateAllows` and **before** the existing `platformGated` branch.

**Rationale.** Verified: `pkgBackend` implements no `applicable()` (only `apt_repo.go:48` and `aur.go:36` do), so
`Engine.Applicable` returns `true` unconditionally and a missing `Names` key becomes `MissingPkgNameError`, which
`driftReport` turns into `return 0, fmt.Errorf(...)` at `cmd/dotf/status.go:117-120` — **aborting the entire
`dotf status`/`diff` run**, not skipping one row. That is fail-_late_, which repo policy forbids. The fix mirrors
the existing `platformGated`/`lockable` optional-interface shape exactly, and it converts every remaining Fedora
package gap into pure TOML: an entry is applicable exactly where it has a name.

**On the fail-fast tension.** This converts a loud probe error into a silent `not-applicable`. That is correct
_only because_ the boundary check moves to manifest-validate time, where it belongs — the manifest **is** the
boundary. The rewritten coverage test (ADR-3) is what makes that true and **must land in the same commit**. This
is the single riskiest change in the plan; see §8.

**Rejected.** (a) Widening `platformGated`'s signature to take the package — breaks `apt_repo.go` and `aur.go` for
no benefit. (b) Per-entry `os = "redhat"` gates on every sibling — works, but must be maintained in lockstep with
`names`, and cannot express "applicable on every manager _except_ dnf", which is exactly what `lazygit`/`yazi` need.

### ADR-3 — Fix `lazygit`/`yazi` with `os = "redhat"` github-release **siblings**, not by converting the shared entries

**Decision.** Keep the `lazygit` and `yazi` `pkg` entries for pacman/apt/homebrew/zypper, **delete only their
fictional `dnf` keys**, and add `os = "redhat"` `github-release` siblings with a pinned `tag` and
`checksums_asset = "checksums.txt"`. Reference both names from `[intents.dev]`. Simultaneously rewrite
`TestEmbeddedManifest_PkgNamesCoverSupportedManagers` (`internal/manifest/manifest_test.go:450-472`) from
per-manager name **presence** to per-manager reachable-**install-path** coverage across `pkg` + `aur` + `apt-repo` +
`github-release`.

**Rationale.** Converting the shared entries wholesale would silently downgrade Arch and macOS from distro-managed
packages to unmanaged `~/.local/bin` binaries — a cross-platform regression paid to serve Fedora. The sibling
approach is only _possible_ because ADR-2's `applicableFor` makes a dnf-less entry a clean skip rather than a hard
abort; the two are one atomic change. The coverage test rewrite is not optional: its current contract ("every
intent-referenced pkg entry carries a name for all five managers") is the **direct cause** of the fictional names —
it pressures an author facing a tool Fedora does not package into inventing a plausible RPM name to make CI green.
The same trap is armed for zypper today and for every future distro.

**On COPR vs github-release for `lazygit`.** Research confirmed `copr:atim/lazygit` has live `fedora-*-aarch64`
chroots, but also that its **newest build FAILED** with `0.47.1` as the last success. Chroot existence and build
health are different questions. `github-release` is lock-pinnable and independent of a third party's build queue,
so it wins. `copr:lihaohong/yazi` is current and healthy, but `yazi` takes github-release too, for symmetry and
because it needs no new backend.

**Caveat to encode.** `yazi` upstream ships a second binary, `ya` (the plugin manager). The single-binary
`github-release` backend will install only `yazi`; state this in the entry comment rather than discovering it later.

### ADR-4 — `lutris` gets an `os = "redhat"` pkg sibling; the aur entry is untouched

**Decision.** Add `lutris-pkg` with `backend = "pkg"`, `os = "redhat"`, `names = { dnf = "lutris" }`, referenced
from `[intents.gaming]` alongside the existing Arch-only `lutris`.

**Rationale.** `aur.go:36` gates on `OSArch`, so the entire `gaming` intent is a silent no-op on Fedora even though
Fedora ships `lutris` in its official updates repo. Package names must be unique
(`internal/manifest/validate.go:113`), hence the distinct name. **Caveat, stated deliberately:** on aarch64 Asahi
Lutris is of limited practical use — there is no x86 Wine/Proton without the `muvm`+FEX 4 KiB-page microVM. This is
recorded as a conscious low-value inclusion, not an oversight; `gaming` is opt-in.

### ADR-5 — Third-party repos become a family-**dispatching** `repo` backend, deferred to the last workstream

**Decision.** When the repo tier is built, generalise `apt-repo` into a single `repo` backend with optional
`apt_repo` / `dnf_repo` sub-tables under **one** package name. `applicable(info)` returns true when the sub-table
matching the host's package manager is present. `apt-repo` survives one release as a deprecated alias mapped to the
same registered implementation. `dnf_repo` has three modes: `copr = "owner/project"`, `repofile = "<url>"`, and
explicit `{id, baseurl, gpgkey}`.

**Rationale.** A peer `dnf-repo` backend would force duplicate entries — `docker-ce` **and** `docker-ce-dnf`, both
listed in `[intents.dev]` — with one permanently `applicable=false`. Verified consequence:
`cmd/dotf/status.go:110-113` emits exactly one `package=… applicable=false` line per non-applicable package
whenever `!onlyMissing`, so that shape buys permanent visible noise on **every** Debian, Arch and macOS host to
serve Fedora. The dispatching shape keeps one logical tool as one entry and stops the ladder in
`docs/reference/manifest-backends.md` growing one rung per package-manager family (openSUSE later is a
`zypper_repo` sub-table, not a tenth backend).

**Honest governance accounting.** This is **8 declared enum values + 1 deprecated alias**, not "still eight". The
alias still requires churn in `Backend.Valid()`, `classByBackend`, `EffectiveClass`, `validatePackage`,
`register.go` and `register_test.go`. Stating it any other way would misprice the review.

**Rejected.** A ninth peer `dnf-repo` backend (the obvious minimal move) — duplicates the whole
key-import/write-repo-file/install pipeline, forces duplicate manifest entries, and pays every governance gate
again for openSUSE.

### ADR-6 — `dnf_repo`'s file modes write `/etc/yum.repos.d/<id>.repo` directly; never `dnf config-manager`

**Decision.** The `repofile` and explicit modes `curl` the repo body to a temp file, then
`sudo install -m 644 <tmp> /etc/yum.repos.d/<id>.repo`. Only the `copr` mode shells out to dnf
(`sudo dnf -y copr enable <owner>/<project>`), and only that mode needs a dnf-version signal for its plugin
prerequisite.

**Rationale.** This dissolves a contradiction the surveys could not settle rather than branching on it. The dnf5
breakage everyone flagged is confined to the _repo_ verbs — `config-manager --add-repo <url>` became
`config-manager addrepo --from-repofile=<url>`, and the plugin package changed from `dnf-plugins-core` (dnf4) to
`dnf5-plugins` (dnf5) — and sources even disagreed on whether the dnf5 form takes `=` or a space. Writing the file
directly is version-independent, needs **no** plugin prerequisite at all, and reuses `apt_repo.go:102`'s exact
`sudo install -m 644` pattern — which is also the SELinux-correct one, because `install` creates the file at the
destination path and receives the right default type label, where a `mv` from `/tmp` would carry `tmp_t`.

### ADR-7 — Do **not** split `PkgMgrDNF`; the dnf-version probe is one field with one consumer

**Decision.** Keep a single `PkgMgrDNF`. When the `copr` mode lands, add a narrow `dnfMajor` probe local to that
backend, parsing `dnf --version` (falling back to os-release `VERSION_ID >= 41`).

**Rationale.** All four `PkgMgr` switches in `internal/backend/pkg.go` are dnf4/dnf5 source-compatible — verified;
nothing there changes. Splitting the enum would ripple through `validate.go:knownPkgMgrs`, all four switches, every
`dnf =` key in `packages.toml` and `manifest_test.go`'s `supportedPkgMgrs`, to serve one plugin package name.
**Correction applied:** do _not_ probe via `LookPath("dnf5")` — on Fedora 40 `dnf5` can be installed alongside dnf4
while `/usr/bin/dnf` remains dnf4, so that probe reports 5 for a dnf4 host. Parse `dnf --version`.

### ADR-8 — `chsh` is guarded with `LookPath` and falls back to `usermod -s`

**Decision.** In `internal/sysconfig/linux.go:225-232`, guard the `chsh` step with `LookPath` and fall back to
`sudo usermod -s <path> <user>`. Keep `chsh` unconditionally on darwin (`darwin.go:167`), where `usermod` does not
exist.

**Rationale.** Two surveys contradict each other: one files a missing `chsh` as a Fedora **blocker** requiring
`util-linux-user`; another states as verified-from-RPM-metadata that Fedora 43/44 folded `chsh` back into base
`util-linux` and that `util-linux-user` no longer exists. A `LookPath` guard is correct under **both** readings and
costs four lines. `usermod` comes from `shadow-utils`, present on Fedora, Debian, Arch and SUSE base installs.

**Rejected.** (a) Switching to `usermod` unconditionally — changes working behaviour on three platforms to fix a
bug that may not exist. (b) Adding a `dnf install -y util-linux-user` step — on a release where that subpackage is
gone, the install step itself fails.

### ADR-9 — The lock is widened for **reproducibility**, not because installs would fail

**Decision.** Run `dotf lock --platform redhat/arm64 --platform redhat/amd64` and commit `manifest/dotf.lock`,
in the same workstream that introduces the new `github-release` entries.

**Rationale, corrected.** An earlier framing called this _required_ because pinned-class packages "fail closed via
`pinnedNoChecksumErr`". Verified overstatement: `internal/backend/github_release.go:79-87` falls through to
`resolveAsset` on a lock miss, and `resolveAsset` populates `wantSHA` from the `checksums_asset` sidecar, so the
pinned check at `:88-90` passes. The real cost of a lock miss is that the Asahi box would be the **only** host
installing off a live GitHub API call rather than a reproducible committed pin. That is reason enough — the lock is
`go:embed`-ed (`embed.go:21`), so it must ship in the release the device downloads.

### ADR-10 — Sysconfig's Fedora gaps are sequenced **late**

**Decision.** Locale langpacks, the firewalld install step, the polkit-gated `--permanent --query-service=ssh`
convergence probe, and `ufwInstallArgv`'s apt-get fallback all land in WS6, after the device is already usable.

**Rationale.** Verified: `manifest/packages.toml:548-563` ships the `[sysconfig]` table **commented out**, and
`cmd/dotf/apply.go:91-101` resolves firewall/ssh to `Unset` from it — so a bare `dotf apply` on the Asahi box plans
and executes **zero** sysconfig steps. None of these gaps is on the first-boot path; they block `dotf sysconfig`,
which the checklist gates to one domain at a time. Sequencing them late buys the user a working box days earlier
and lets the device answer the `chsh` question before code is written against it.

### ADR-11 — Backup becomes the default in the symlink engine

**Decision.** Set `BackupEligible: true` on the seven unprotected `FacetAll` directory entries and teach the
`ActionBackup` executor to copy a directory tree. Treat this as a **device blocker**, not a latent nice-to-have.

**Rationale.** Verified: `internal/symlink/symlink.go:206-209` emits `ActionBackup` only
`if spec.BackupEligible && opts.Backup`, then emits an unconditional `ActionRemove` documented at `symlink.go:42`
as _"recursive for dirs"_. Only 14 entries carry the flag, all single files. The seven unprotected ones are
`nvim` (`table.go:91`), `wezterm` (`:92`), `ghostty` (`:93`), `mise` (`:94`), `alacritty` (`:95`), `bin` (`:119`)
and `dotenv`→`.config/direnv` (`:124`). On a _fresh_ install these paths do not exist, so run 1 is safe — which is
exactly why this is dangerous. The task brief asks that the user iterate locally on the box; run 5, after they have
customised nvim, silently `rm -rf`s it **even with `--backup` passed**. A safety flag that does not cover the
majority of what it removes is a broken contract, and process discipline is not a substitute on a daily driver.

**Amendment (PR #36 review).** The decision above is correct but was scoped too narrowly, and the paragraphs above
describe the pre-PR baseline — they are kept as the historical record, not as the shipped state. Facet gating has
no bearing on destructiveness: `ActionRemove` is `os.RemoveAll` for **every** `IsDir` spec, whatever facet let it
through. So the seven `FacetHyprland` directories and the `FacetMacOS` `aerospace` directory were left recursively
deletable under `--backup` — and `LinuxDesktopGuardError` names `--backup` as the safety mechanism for exactly the
Hyprland set, a promise that was false. ADR-12's retag of the tmux user unit from `FacetArch` to `FacetLinux`
compounded it by newly managing that destination on Debian/RedHat/SUSE/WSL without making it eligible. The shipped
rule is therefore wider than this ADR: **every** `IsDir` entry is backup-eligible regardless of facet, plus the
tmux user unit — 30 of 41 entries. Two invariants are encoded in the `BackupEligible` doc comment in
`internal/symlink/table.go` and pinned by `TestTable_BackupEligibleSet` and `TestTable_MatchesPlaybook`: every
directory entry is eligible, and widening a spec's facet must never widen the set of destinations deleted without
a copy.

### ADR-12 — Facet names describe capabilities, not distros

**Decision.** Add `FacetLinux` (resolving via the **local** `isLinuxFamily`, preserving its deliberate `OSLinux`
exclusion) and retag `common/systemd/user/tmux.service` (`table.go:116`) from `FacetArch` to it. Add a composite
`FacetArchDesktop` and move `launchers/applications/arch-update.desktop` (`:111`) onto it. Rename
`FacetLinuxDesktop` → `FacetHyprland`, keeping `--linux-desktop` as a back-compat flag alias.

**Rationale.** The tmux unit orders on `graphical-session-pre.target` and runs a repo script — nothing
Arch-specific. It sits behind `FacetArch` purely because Arch was the only Linux target when
`playbooks/vars/symlinks.yml` was written (where the group is literally `archlinux_configs`, alongside an
already-declared-but-empty `linux_configs: []`). Meanwhile `arch-update.desktop` is CachyOS-branded
(`Name=Cachy-Update`, `Exec=… arch-update`) yet installs on any Linux host under `--linux-desktop`, and the whole
`FacetLinuxDesktop` set is the Hyprland/wlroots stack — categorically wrong for a KDE host. Naming the facet
honestly makes the wrong flag unattractive by construction, rather than relying on a documentation warning.

**Explicitly not done.** Unifying the private `isLinuxFamily` with `platform.IsLinuxFamily`. The `OSLinux`
exclusion is pinned by a golden test row and is not on Fedora's path.

### ADR-13 — Fonts get a native backend, not a `custom-script` shortcut

**Decision.** Ship the Nerd Font families via a new native `font` backend in WS5 that delegates to the host package
manager where a patched package exists (`pacman` `ttf-*-nerd`, Homebrew casks) and otherwise extracts the
checksum-verified upstream `ryanoasis/nerd-fonts` archive into `$XDG_DATA_HOME/fonts/<name>/`. Leave
`customScriptBaseline` at 16.

**Rationale.** 22 config sites pin patched family names by exact string; Fedora ships only unpatched upstream
builds, and the `che/nerd-fonts` COPR is **symbols-only** (it Provides `font(symbolsnerdfont)` and installs a
fontconfig alias file — it never creates the family names the configs pin). But tofu in a prompt is annoying, not
blocking. A `custom-script` shortcut would move the repo's scarcest governance budget (16→17) permanently, to buy a
day, on cosmetics. The native backend also fixes macOS and Arch, where fonts are today installed only by
`bin/install-arch-pkgs.sh:474-488` — a function referenced nowhere in the manifest.

### ADR-14 — The shell per-OS loader is fixed as a **search path**, not a new directory

**Decision.** Add one `os_module_dirs()` resolver to `common/platform.sh` yielding the family (`linux`/`macos`)
then the distro overlay, and replace the three copy-pasted dispatch blocks. Fix the three regressions that go live
in the same change.

**Rationale.** Creating `common/redhat/aliases.sh` would fix Fedora and leave Debian and Arch broken, and would
duplicate 495 lines. A search path fixes all three distros with one function and leaves a genuine Fedora-only
overlay slot. Critically, the loader fix makes ~495 previously-dead lines live on **three** platforms at once, so
the latent regressions it exposes must be fixed in the **same** workstream.

**Amendment (PR #36 review).** The first pass named exactly three — the `alias history=` builtin shadow, the
ufw-only firewall block, and the Plasma 5 `systemsettings5` binary — and asserted that enumeration was complete.
It was not: the review reproduced four more broken aliases in the same newly-live file. Do not read any fixed
number here as the audited total. The audited **classes**, all now fixed in `common/linux/aliases.sh`, are:

- **Shell-builtin shadows** (aliases win over builtins in both bash and zsh): `history` in both the `dnf` and
  `yum` arms → `pkghistory`; `suspend` → `syssuspend`; `enable`/`disable` → `sysenable`/`sysdisable`.
- **Real-command shadows whose body has a different shape**: `mount` (piped through `column -t`, so it swallowed
  every argument) and `netstat` (silently became an `ss` socket table) are deleted; `gcc`/`g++` are renamed to
  `gccw`/`gxxw`; Arch's `info` is renamed to `pkginfo` in both the yay and paru arms.
- **Package-manager assumptions hardcoded to apt**: `updateall` and `cleanup-packages` move into the
  `case "$DOTFILES_PKG_MGR"` dispatch so each manager gets a correct body, with snap/flatpak appended only when
  those managers are actually installed.
- **Desktop and firewall assumptions**: the `systemsettings5` guard and the firewalld branch, as originally
  planned.

`alias make='make -j"$(nproc)"'` is deliberately **kept** with an explicit accepted-risk comment: make takes the
last `-j`, so a user's own value still wins — a preference change, not the argument-swallowing defect `mount` had.

**Residual risk (named, not closed).** A `comm -12` sweep of the live alias names against `compgen -b` now returns
empty, so no builtin shadow remains. Four aliases still shadow a PATH binary by design: `install`, `poweroff`,
`reboot` and `top`. `install` is the one that matters — it shadows coreutils `install(1)` with a completely
different shape and is the centrepiece of this file's generic package verbs (`install`/`search`/`update`/`remove`).
Renaming it is a deliberate follow-up scope decision, not an oversight.

### ADR-15 — No aarch64 CI; two local containers instead

**Decision.** Add one `container: fedora:latest` job (distro axis) and one shellcheck job to both CI systems. Buy
no arm64 runner. Document a local two-container rehearsal loop: a native amd64 privileged systemd Fedora container
for sysconfig/firewalld/sshd, and an emulated aarch64 `fedora:latest` for arch/package behaviour.

**Rationale.** Distro variance and architecture variance are separable, and only distro variance is cheap to
automate. The binary is a CGO-free cross-compile with no arch-specific codegen; what _is_ arch-specific is package
availability and asset matching, which the emulated container plus existing unit rows cover. arm64 + systemd is
unreachable under `qemu-user` (measured: `dbus-broker` dies `226/NAMESPACE`), so a full `qemu-system-aarch64` TCG VM
would be the only path and is not worth building. Hosted arm64 runners are paid on a private repo, and a
self-hosted runner on the Asahi laptop would invert the safety property this design exists to protect.

---

## 6. Workstreams

Dependency graph:

```
WS0 (device capture, read-only)
 ├─→ WS1 (first-boot unblock) ─┐
 │                             ├─→ "day-one branch"
 └─→ WS2 (package correctness)─┘
        ├─→ WS4 (CI + rehearsal)
        └─→ WS7 (repo backend, docker-ce, lock)
WS3 (symlink safety + facets)   ── independent
WS5 (desktop: plasma + fonts)   ── depends on WS0, WS1
WS6 (sysconfig Fedora domains)  ── depends on WS0, WS4
WS8 (docs)                      ── depends on all
```

### WS0 — Device ground truth capture (read-only) · Depends on [none]

**READ THESE BEFORE TASK**

- `/internal/platform/platform.go`
- `/manifest/packages.toml`

**Files to Create / Modify:** none. This is a copy-pasteable block the **user** runs on the M1 before code is
written, added to the WS1 checklist doc.

Capture, verbatim: `cat /etc/os-release`; `uname -m -r`; `dnf --version`;
`rpm -qf "$(command -v chsh)" 2>&1`; `command -v qdbus qdbus6 qdbus-qt6 busctl gdbus`;
`command -v gh python3`; `getenforce`; `sudo firewall-cmd --get-default-zone`;
`systemctl is-enabled sshd`; `findmnt -no FSTYPE /`; `command -v snapper`;
`printenv XDG_CURRENT_DESKTOP KDE_FULL_SESSION XDG_SESSION_TYPE`;
`dnf list --available lazygit yazi lutris docker-ce moby-engine 2>&1 | tail -20`; and
`ls -d ~/.config/{nvim,wezterm,ghostty,mise,alacritty,direnv} ~/.bin 2>&1`.

**Verification.** The user pastes the output back. Every downstream decision that encodes a device fact cites a
line from it. Specifically: the os-release block becomes the **verbatim fixture** in WS1's `TestDetectOS` rows; the
`rpm -qf chsh` line settles ADR-8's contested premise; the `dnf list --available` block settles the mapping table's
release header; and the final `ls` decides whether ADR-11's urgency is "already dangerous" or "dangerous soon".

**Risk.** The user runs it after WS1 has started, so WS1's fixtures encode a guess. Mitigated because WS1's
detection change is correct under every candidate ID.

### WS1 — First-boot unblock · Depends on [none, informed by WS0]

**READ THESE BEFORE TASK**

- `/bootstrap.sh` (`:157-169`, `:243-250`, `:378-421`)
- `/internal/platform/platform.go` (`:84-112`)
- `/cmd/dotf/applyphases.go` (`:417-431`)

**Files to Create**

- `/docs/fedora-first-run.md`

**Files to Modify**

- `/bootstrap.sh` — in `github_download_release_file`, after `tag="$(github_resolve_latest_tag || true)"` succeeds
  (`:387`) and **before** the `if [ -z "$GITHUB_AUTH_TOKEN" ]; then return 1` bail (`:401-403`), insert an
  unauthenticated tag-pinned attempt against `${REPO_ORIGIN}/releases/download/${tag}/${name}`. Extend the guard
  hint text with the Fedora prerequisite line (`sudo dnf install -y gh`).
- `/internal/platform/platform.go` — add `"fedora-asahi-remix"` and `"asahi-fedora-remix"` to the `case` at `:91`.
- `/common/platform.sh` — mirror the same two tokens into the `fedora|rhel|centos|rocky|almalinux)` arm at `:60`.
- `/internal/platform/platform_test.go` — three rows in the existing `TestDetectOS` table; one in `TestDistroName`.
- `/cmd/dotf/applyphases.go` — filter `pkgs` through `engine.Applicable` in the dry-run composer before
  `printApplyDryRun` (`:429`), and emit a separate `plan.not-applicable=` line.
- `/.github/workflows/go.yml`, `/.forgejo/workflows/go.yml` — add `bootstrap.sh`, `.goreleaser.yml`,
  `.goreleaser.forgejo.yml` to both `paths:` lists.

The new `docs/fedora-first-run.md` carries the WS0 capture block plus the ordered device checklist (detection →
doctor → `dotf link --dry-run` **with the un-backed-up-directory warning** → `dotf apply --dry-run` → first
mutation, one domain at a time → packages → symlinks), and an explicit **DO NOT USE** section naming
`--linux-desktop` (it deploys the Hyprland set from `table.go:101-113` onto a KDE host) and `--snapper`
(`applyphases.go:23-26` states failures are _never_ recorded, so the run exits 0 having protected nothing).

**Verification.** On the device: `./bootstrap.sh` obtains `dotf_linux_arm64.tar.gz` with `gh` absent and no token.
`dotf platform` prints `os=redhat arch=arm64 pkg=dnf distro=<captured>`. `dotf apply --dry-run --headless` no longer
prints `plan.apt-repo=docker-ce`. Locally: `go test ./internal/platform/... ./internal/selfupdate/...`;
`shellcheck bootstrap.sh`; a PR touching only `bootstrap.sh` now triggers CI.

**Risk.** The unauthenticated tag-pinned URL 404s on a _private_ repo, so the fallback helps only the
token-present-but-`python3`-absent case; and `github_resolve_latest_tag` itself needs `gh` or `python3` before any
fallback runs. The documented backstop is `--install-go` from the clone. See the correction in §8.

### WS2 — Package correctness · Depends on [WS0, WS1]

**READ THESE BEFORE TASK**

- `/internal/backend/backend.go` (`:88-157`)
- `/internal/manifest/manifest_test.go` (`:355-472`)
- `/docs/reference/manifest-backends.md`

**Files to Modify**

- `/internal/backend/backend.go` — add the `packageGated` optional interface; check it in `Engine.Applicable`.
- `/internal/backend/pkg.go` — implement `applicableFor`.
- `/internal/backend/pkg_test.go` — one row: a pkg entry with no key for the host manager is `not-applicable`, not
  an error.
- `/internal/manifest/manifest_test.go` — rewrite `TestEmbeddedManifest_PkgNamesCoverSupportedManagers` to the
  reachable-install-path contract.
- `/manifest/packages.toml` — delete the `dnf` keys from `lazygit` (`:157`) and `yazi` (`:176`); add
  `lazygit-bin` / `yazi-bin` `github-release` siblings with `os = "redhat"`, pinned tags and `checksums_asset`,
  referenced from `[intents.dev]`; add `lutris-pkg` (`os = "redhat"`, `dnf = "lutris"`) to `[intents.gaming]`;
  **audit all 13 pkg entries** for a `dnf` key against the WS0 capture.
- `/manifest/dotf.lock` — regenerate via `dotf lock --platform redhat/arm64 --platform redhat/amd64`.
- `/.docs/fedora-package-mapping.md` (**create**) — §4's table, with a header recording the exact release and date
  it was verified against.

**Verification.** `go test ./internal/backend/... ./internal/manifest/... ./cmd/dotf/...`.
`TestCustomScriptRatchet` still passes at exactly **16**. Manual proof of the closed hole: a pkg entry with only a
`pacman` name now yields one `package=… applicable=false` line on a dnf host instead of aborting the report.
`dotf status` on macOS/Arch/Debian gains at most one row per new sibling.

**Risk.** Highest-risk workstream. See §8.

### WS3 — Symlink safety and facet correctness · Depends on [none]

**READ THESE BEFORE TASK**

- `/internal/symlink/table.go` (`:20-36`, `:88-133`, `:143-169`)
- `/internal/symlink/symlink.go` (`:195-215`)

**Files to Modify**

- `/internal/symlink/table.go` — `BackupEligible: true` on the seven directory entries (`:91`, `:92`, `:93`, `:94`,
  `:95`, `:119`, `:124`); add `FacetLinux` and `FacetArchDesktop`; retag `tmux.service` (`:116`) and
  `arch-update.desktop` (`:111`); rename `FacetLinuxDesktop` → `FacetHyprland`.
- `/internal/symlink/apply.go` — teach the `ActionBackup` executor to copy a directory tree.
- `/internal/symlink/symlink.go`, `/cmd/dotf/link.go`, `/cmd/dotf/applyopts.go` — thread the facet rename; keep
  `--linux-desktop` as a back-compat alias.
- `/internal/symlink/table_test.go` — update the **three** golden locks together (total count, per-facet counts,
  inline golden spec list); add redhat rows to the faceting truth table.
- `/playbooks/vars/symlinks.yml` — move the tmux entry from `archlinux_configs` to the empty `linux_configs` group,
  honouring the SYNC contract at `table.go:62`.
- `/launchers/applications/wifi-tui.desktop` — replace the hardcoded `/home/yandy/.bin/…` path with a PATH lookup
  plus `TryExec`.

**Verification.** `go test ./internal/symlink/...`. Manual on the device: create a sentinel under `~/.config/nvim`,
run `dotf link --backup`, confirm the sentinel survives in `$HOME/.config/dotfiles_backup_<epoch>`.
`dotf link --dry-run` on Fedora now lists `.config/systemd/user/tmux.service` (it does not today) and, under the
desktop flag, no longer lists `arch-update.desktop`.

**Risk.** The three golden locks must move together or CI fails loudly — that is the intended guard. Backup dirs
grow; acceptable versus silent data loss.

### WS4 — CI and rehearsal harness · Depends on [WS1, WS2]

**Files to Create**

- `/.github/workflows/shell.yml`, `/.forgejo/workflows/shell.yml`
- `/docs/fedora-rehearsal.md`

**Files to Modify**

- `/.github/workflows/go.yml`, `/.forgejo/workflows/go.yml` — add a `fedora` job with `container: fedora:latest`
  whose **first** step is `dnf -y install --setopt=install_weak_deps=False git gcc tar gzip findutils diffutils`
  (git or `actions/checkout` degrades to a REST tarball; gcc or `go test -race` errors with `-race requires cgo`),
  then checkout, `setup-go` from `go.mod`, `go vet`, `go test -race`, and a smoke step asserting `dotf platform`
  contains `os=redhat` and `pkg=dnf` and that `dotf apply --dry-run --headless` and `dotf link --dry-run` exit 0.
  Add `**.sh`, `common/**`, `bin/**` to the path filters.

The Forgejo variants use only `actions/*` actions (already used there) or a raw-shell Go install, per that
workflow's documented `DEFAULT_ACTIONS_URL` constraint. `docs/fedora-rehearsal.md` documents the two local
containers and states plainly that arm64 + systemd is unreachable under emulation.

**Verification.** The Fedora job is green on a PR touching only `manifest/packages.toml`; the shell job triggers on
a PR touching only `common/platform.sh`.

**Risk.** `actions/setup-go` may be flaky inside a Forgejo container; the documented fallback is a raw-shell Go
tarball install parsing the version from `go.mod`.

### WS5 — Desktop layer · Depends on [WS0, WS1]

**Files to Modify**

- `/internal/plasma/reload.go` — extend `qdbusCandidates` (`:43`) to `{"qdbus6", "qdbus-qt6", "qdbus"}` and prefer
  the Qt-free `busctl --user call org.kde.KWin /KWin org.kde.KWin reconfigure` as the leading candidate; keep
  `cmd/dotf/plasmaverbs.go`'s substituted-binary pairing in sync.
- `/internal/plasma/plasma.go` — derive `dataHome` from `$XDG_DATA_HOME` when absolute; seed the system
  colour-scheme directory list from `$XDG_DATA_DIRS`.
- `/internal/plasma/table.go` + `/internal/plasma/table_test.go` — add `VolatileKeys` rules for the browser and
  AI-app `x-scheme-handler/*` and `text/html` keys in `mimeapps.list`, which pin `google-chrome.desktop` (no
  aarch64 Linux build) and would otherwise be merged into the live file.
- `/internal/backend/font.go` (**create**), `/internal/backend/download.go` (xz branch),
  `/internal/backend/register.go`, `/internal/manifest/types.go`, `/manifest/packages.toml` — the `font` backend
  and its `fonts` (default) / `desktop-fonts` (opt-in) intents.

**Device work.** After configuring the trackpad, battery profile and browser default by hand, run
`dotf plasma export --only kcminputrc,powerdevilrc,mimeapps.list,kglobalshortcutsrc` and commit.

**Verification.** `dotf plasma apply --backup --dry-run`, then `--backup`, then run the printed reload commands by
hand (**never** `--reload` on the first pass); log out and back in. `fc-list | grep -c 'JetBrainsMono Nerd Font'`
is non-zero and the Starship prompt renders its glyphs. `customScriptBaseline` still equals 16.

### WS6 — Sysconfig Fedora domains · Depends on [WS0, WS4]

**Files to Create**

- `/internal/sysconfig/linux_locale.go`, `/internal/sysconfig/linux_firewall.go` — `linux.go` is 554 lines against
  the ~500 soft cap and every fix here lands in it; split in the same change.

**Files to Modify**

- `/internal/sysconfig/linux.go` — extract one `pkgInstallArgv(info, pkg)` helper from the four-way switch already
  spelled out in `sshInstallArgv` (`:493-505`) and rewrite `ufwInstallArgv` (`:372-378`, which emits `apt-get` on
  Fedora) as a caller; restructure `planLocale` (`:115-153`) so the strategy is chosen by `info.OS` **first** and
  `LookPath` second, adding an `OSRedHat` arm to `localeGenSteps` emitting `sudo dnf install -y glibc-langpack-<lang>`;
  guard `chsh` (`:228-229`) with `LookPath` and fall back to `usermod -s`; give `planFirewalld` (`:345-370`) a
  `LookPath("firewall-cmd")`-gated install step mirroring `planUfw`'s shape and harden its `--state` test; replace
  the unprivileged `firewall-cmd --permanent --query-service=ssh` probes with a `sudo -n` tri-state modelled on
  `ufwStatus`, flipping the RedHat/SUSE exclusions in `FirewallPlanNeedsSudo` and `SSHServerPlanNeedsSudo`.
- `/internal/sysconfig/locale_resolve.go` — add `langpackFor(locale)`; correct the doc comment that currently
  claims parity with `common/exports.sh:locale_exists` (a parity that does not exist until WS-shell lands).
- `/internal/manifest/merge.go` — add the missing `overlay.SSHServer` branch; the field is silently dropped
  whenever the base manifest has any `[sysconfig]` table.
- `/internal/sysconfig/linux_test.go` — RedHat rows for locale, shell and `ufwInstallArgv`.

**Verification.** **Two-phase, non-negotiable.** Phase one: land the extraction and split with the existing
856-line `linux_test.go` passing **unmodified** — that is the behaviour-preservation proof for Arch/Debian/SUSE.
Phase two only then adds Fedora rows and the deliberate `*PlanNeedsSudo` flips, whose test churn is itself the
intentionality signal. On the device, one domain at a time per the checklist; `--firewall auto --yes` run twice must
print `sysconfig=converged` on the second pass (it cannot today).

### WS7 — Repo backend, docker-ce, `$releasever` · Depends on [WS0, WS2, WS4, WS6]

**Files to Create**

- `/internal/backend/repo.go`, `/internal/backend/repo_test.go`

**Files to Modify**

- `/internal/backend/apt_repo.go` — lift `installAPT` **verbatim** (no rewrite) into the generalised backend.
- `/internal/backend/register.go`, `/internal/backend/register_test.go` — register `repo`; keep `apt-repo` as the
  deprecated alias mapped to the same implementation.
- `/internal/manifest/types.go`, `/internal/manifest/validate.go`, `/internal/manifest/class.go` — `BackendRepo`,
  the `DNFRepo` config struct, its required-field case, and `{ClassManaged: true}`.
- `/internal/platform/types.go`, `/internal/platform/osrelease.go`, `/internal/platform/platform.go` — add
  `Info.VersionID` (the first field with a real consumer: `$releasever`), and refactor `Detect` to scan
  `/etc/os-release` **once** rather than three times, keeping the existing path-taking helpers as thin wrappers so
  current tests compile unchanged.
- `/manifest/packages.toml` — give `docker-ce` a `dnf_repo` sub-table pointing at
  `https://download.docker.com/linux/fedora/docker-ce.repo` under the same package name; gate the `fingerprint`
  module `os = "arch"` (Touch ID is unsupported under Linux on Apple Silicon).
- `/docs/reference/manifest-backends.md` — rung 3 becomes `repo`, with the apt/dnf sub-table decision ladder and an
  explicit answer for "a COPR-only tool".

**Verification.** `dotf install docker-ce` on the device writes `/etc/yum.repos.d/docker-ce.repo` and installs;
a re-run reports already-installed. `dotf status` shows exactly **one** applicable docker entry per host across
Fedora, Debian and macOS. `customScriptBaseline` still equals 16 — a native backend costs nothing against it.
`repo_test.go` asserts exact argv for all three dnf modes **and** carries an apt regression case proving the lifted
Debian path is byte-identical.

**Risk.** `apt-repo` is live in the headless default set via `docker-ce`, so a regression breaks Debian
provisioning. Mitigated by lifting `installAPT` verbatim and keeping the existing apt assertions pointed at the new
backend unchanged.

### WS8 — Documentation · Depends on [WS1–WS7]

**Files to Modify**

- `/docs/architecture/platform-detection.md` — rewrite in two labelled halves: the canonical Go detector first
  (currently the doc describes only the shell detector and omits `rocky`/`almalinux`, every Arch derivative, the
  whole `suse` arm and the `ID_LIKE` fallback), plus a Fedora-family subsection recording the verified ID mapping.
- `/docs/development/testing.md` (**create**) — the directory `docs/README.md:12,44-50,140-143` has advertised
  through two feature cycles and which does not exist. Documents the two-tier testing reality and hosts the
  permanent home for the M1 checklist.
- `/docs/features/modern-tools.md`, `/docs/features/environment-management.md` — a Fedora `dnf` block beside each
  existing Ubuntu/Arch pair, noting that Fedora's `fd-find` ships `/usr/bin/fd` and `bat` ships `/usr/bin/bat`, so
  the Debian-style rename shims are unnecessary.
- `/CLAUDE.md`, `/AGENTS.md` — correct the "~307 line" description of `bootstrap.sh`; it is **593 lines**.
- `/docs/releases/<tag>.md` (**create** before tagging) — both release workflows pass `--release-notes` and fail
  loudly if the file is absent. The Install block must state Fedora and aarch64 explicitly.

---

## 7. Critical Test Set

The user explicitly forbade unnecessary tests. Six tests. Five are **rows added to existing tables**; one new test
file is justified because it covers a brand-new backend that writes to `/etc` under `sudo`.

| #   | Test                                                                                                                                             | Location                                               | One-line justification                                                                                                                                                                                                        |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Three `TestDetectOS` rows: bare `ID=fedora` with **no** `ID_LIKE`, plus both Asahi ID candidates with `ID_LIKE=fedora`; one `TestDistroName` row | `internal/platform/platform_test.go` (existing tables) | The primary target platform has **no** test row today — Fedora is covered only transitively through the `rocky` `ID_LIKE` case, and a misclassification is not a loud failure but a silent loss of every Linux-facet symlink. |
| 2   | Rewritten `TestEmbeddedManifest_PkgNamesCoverSupportedManagers` (presence → reachable install path)                                              | `internal/manifest/manifest_test.go:450-472`           | Dual-purpose and non-negotiable: it is the sole safety net for ADR-2's one real regression risk, **and** its current form is the direct cause of the two fictional dnf names.                                                 |
| 3   | One `pkg_test.go` row: a pkg entry with no key for the host manager reports `not-applicable`, not an error                                       | `internal/backend/pkg_test.go` (existing table)        | Pins ADR-2's new semantics at the exact seam that changed; without it a future refactor silently restores the fail-late path that aborts the whole `dotf status` run.                                                         |
| 4   | One `apply_test.go` row: a **directory** destination with real content emits `ActionBackup` ordered before `ActionRemove` under `opts.Backup`    | `internal/symlink/apply_test.go` (existing table)      | The only test guarding data-destroying behaviour, and the first host to exercise the new directory-copy path is the user's populated daily driver.                                                                            |
| 5   | `repo_test.go`: exact-argv table for the three `dnf_repo` modes **plus** an apt regression case                                                  | `internal/backend/repo_test.go` (**new file**)         | A new backend that writes files under `/etc` with `sudo` is not shippable untested, and the apt case is the proof that generalising `apt-repo` did not break the live `docker-ce` path in Debian's headless default set.      |
| 6   | Fedora CI smoke assertions: `dotf platform` contains `os=redhat`/`pkg=dnf`; `dotf apply --dry-run --headless` and `dotf link --dry-run` exit 0   | `.github/workflows/go.yml` (job step, not a Go test)   | The only thing in the repo that ever executes against a real Fedora userspace, a real `/etc/os-release`, real dnf5 and real rpm — every other Fedora assertion runs against `exec.Fake`.                                      |

**Explicitly not written** (listed here so their absence is a decision, not an omission): a `capability_test.go`
for a capability table this design does not build; per-installer tests for the orphaned `bin/install-*.sh` scripts;
an arm64 execution test (no runner, and the binary is a CGO-free cross-compile); a `TestDetectDNFMajor` (no
consumer until WS7's COPR mode); and any test for `PkgMgrYUM`, `rpm-ostree`, or the fingerprint module.

---

## 8. Risks and Mitigations

### R1 — `applicableFor` inverts a loud failure into a silent one on three working platforms · **highest**

ADR-2 changes applicability semantics for **every** `pkg` package on **every** platform. Today a dropped `names`
key is loud (it aborts `dotf status`); afterwards it is quiet (one `applicable=false` row). A typo on macOS or Arch
would silently stop installing a package — the same class of failure the `docker-ce`-on-Fedora hole already
demonstrates. **Mitigation:** the rewritten coverage test (critical test #2) is the only guard and **must land in
the same commit**, reviewed with equal care. If its per-manager reachability logic is weak, this design trades a
Fedora bug for a silent macOS/Arch regression. This is the change to reject in review if the test is not convincing.

### R2 — The download blocker's stated cause · **correction**

An earlier framing attributed the `bootstrap.sh` failure primarily to the repo being private. The **dominant**
cause is `.goreleaser.yml`'s `prerelease: auto` combined with `-beta` tags: GitHub's `/releases/latest` excludes
prereleases, so `/releases/latest/download` 404s on a **public** repo too, and `ALT_BASE`
(`/releases/download/latest`) points at a literal tag named `latest` that does not exist. State it that way so
nobody "fixes" it by flipping repository visibility. Additionally, the WS1 fallback helps only the
token-present-but-`python3`-absent case on a private repo, and `github_resolve_latest_tag` itself needs `gh` or
`python3` first — the real backstop is `--install-go` from the clone, which fetches `go1.26.5.linux-arm64.tar.gz`
with no sudo.

### R3 — Homebrew on arm64 Linux

`internal/brew/brew.go:32-41` gates on OS family + the `--homebrew` tristate with **no** `info.Arch` reference, and
`prefix()` returns `/home/linuxbrew/.linuxbrew` for any non-macOS host. Homebrew publishes no aarch64-Linux bottles
and its installer refuses on that host, so `--homebrew=true` on the M1 runs a doomed installer whose failure is
recorded and flips the run to exit 1. Compounding it, `detectPkgMgr:180` probes `brew` **first** with no OS or arch
gate — so an x86_64 Fedora box with Linuxbrew silently routes every package through `brew install` and never
consults the 13 `dnf` names. **Mitigation:** gate `Applicable` on `info.Arch != platform.ArchARM64` for the Linux
arm, emit an explicit stderr note when the flag is declined, and demote brew below the native manager on Linux.
Folded into WS6 as a two-line change with an outsized blast radius. Not a first-boot blocker (the flag is opt-in),
which is why it is not in WS1.

### R4 — SELinux

Enforcing by default. Current exposure is genuinely low and should not be over-invested in: `internal/symlink`
writes only under `$HOME` (files inherit the parent's context); `sysconfig`'s `installFile` does
`sudo cp <tmp> <dest>` into an **existing** inode, preserving context and mode; and `internal/plasma`'s
`writeAtomic` creates its temp in the **destination's own directory** then renames, so the context is inherited and
preserved. Exposure begins the moment the domain set grows: sshd on a non-default port needs
`semanage port -a -t ssh_port_t -p tcp <port>`, and anything sshd reads under `~/.ssh` needs `ssh_home_t`.
**Mitigation:** WS6 adds an _informational_ `getenforce` reading to `dotf doctor` and the checklist records
`getenforce` before and after every mutation, treating a flip to Permissive as stop-the-line. `semanage` port
labelling is explicitly deferred (non-goal). Note `semanage` lives in `policycoreutils-python-utils`, which is only
transitively present on the KDE spin — guard any use with `command -v`.

### R5 — dnf5

`/usr/bin/dnf` **is** dnf5 on Fedora 41+. Install/upgrade/remove and the `rpm -q` probe are source-compatible, so
the pkg backend needs no change — say so explicitly in the design so nobody "fixes" it. The divergence is confined
to the repo verbs, and ADR-6 routes around it entirely by writing `.repo` files directly. ADR-7's version probe
parses `dnf --version` (not `LookPath("dnf5")`, which misreports on Fedora 40) and has exactly one consumer.

### R6 — rpm-ostree variants

On Silverblue/Kinoite/Bluefin/Bazzite and the ostree Asahi spin, `/usr/bin/dnf` is absent by design, so
`detectPkgMgr` returns `PkgMgrUnknown` and every `pkg` package fails with `UnsupportedPkgMgrError` — a wall of
errors rather than one legible message. This is a **non-goal** (the target device is a traditional rpm/dnf install),
but the failure mode should be recorded. If it ever enters scope, the canonical detection signal is
`/run/ostree-booted`, and the right behaviour is to skip the packages phase with one message while keeping
symlinks/plasma/sysconfig — not to abort the run.

### R7 — Snapper gives false rollback confidence on Fedora

`internal/snapper/snapper.go` gates only on Linux family + `LookPath("snapper")` — no btrfs check, no
snapper-config check — and `Create` emits `sudo snapper create` with no `-c <config>`. Fedora ships no snapper and
no config, and its btrfs layout uses bare `root`/`home` subvolumes (not openSUSE's `@`). Critically,
`cmd/dotf/applyphases.go:23-26` states verbatim that failures are _"NEVER recorded in the failure collector"_, so
`dotf apply --snapper` exits **0** having protected nothing. **Resolution of the survey contradiction:** the
"flows through failure.Collector" reading is wrong; the code comment and body are unambiguous. **Mitigation:**
`--snapper` goes in the checklist's DO-NOT-USE list, and the Preflight tarball is the real rollback.

### R8 — Data loss on the daily driver

See ADR-11. Until WS3 lands, the checklist's mitigation is a mandatory read of the `link.remove=` line and a manual
`tar czf` per non-backup-eligible path. That is a stopgap, not the fix.

### R9 — Stale package data

The two research datasets disagreed (f43 vs f44 NEVRs, and on `atuin`/`eza`/`gh`/`lutris` versions). Neither is
authoritative for the device. **Mitigation:** §4 commits to **names**, not versions; `.docs/fedora-package-mapping.md`
records the release and date it was verified against; and WS2 re-runs the queries via the WS0 capture before the
manifest is finalised.

### R10 — WS6's refactor breadth

`internal/sysconfig` is the only subsystem that emits privileged, system-mutating argv against a daily driver, and
WS6 rewrites parts of it on behalf of a distro that already gets firewalld and `openssh-server` for free. The
failure mode is regressing Arch/Debian/macOS. **Mitigation:** the two-phase gate in WS6's verification section is
non-negotiable and must not be collapsed.

---

## 9. Verification Plan

### Provable in CI (automated, every PR)

| Claim                                                                        | Mechanism                                                                                                                              |
| ---------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Fedora detection, `dotf platform` output contract                            | Fedora container job smoke step (`os=redhat`, `pkg=dnf`)                                                                               |
| dnf5 present and `rpm -q` works against a real RPM database                  | Fedora container job (`fedora:latest` ships dnf/dnf5/rpm/curl/tar/sudo; **not** git, gcc or python3 — hence the explicit install step) |
| Go unit behaviour under a real Fedora `/etc/os-release`                      | `go vet` + `go test -race` inside the container                                                                                        |
| `dotf apply --dry-run --headless` and `dotf link --dry-run` exit 0 on Fedora | Fedora container job smoke step                                                                                                        |
| Asset-name drift (goreleaser ↔ `bootstrap.sh` ↔ `selfupdate.AssetName`)      | `TestAssetNaming_MatchesBootstrap`, now actually triggered by the WS1 path-filter fix                                                  |
| Shell correctness of every Fedora branch                                     | New shellcheck job over `**.sh`, `common/**`, `bin/**`, `bootstrap.sh`                                                                 |
| Custom-script budget unchanged at 16                                         | `TestCustomScriptRatchet` (exact match, both directions)                                                                               |
| Per-manager install-path reachability                                        | Rewritten coverage test (critical test #2)                                                                                             |
| Directory backup before recursive removal                                    | Critical test #4                                                                                                                       |
| `repo` backend argv, all three dnf modes + apt regression                    | Critical test #5                                                                                                                       |

### Provable only in a local container (developer machine, pre-PR)

- Real `dotf sysconfig --timezone … --firewall firewalld --ssh-server on --yes` reaching `failed=0` — requires a
  **native amd64 privileged systemd** Fedora container.
- A real multi-package `dnf` transaction and `rpm -q` on **aarch64** — requires an emulated `linux/arm64`
  `fedora:latest` (one `binfmt` install; expect roughly an 8–9× wall-clock penalty on the RPM layer, none on the Go
  binary itself).

### Provable only on the physical M1

These are the accepted residual risks. Each has a numbered step in `docs/fedora-first-run.md`.

| Claim                                                                                                    | Why only the device                                                                                                                                                                            |
| -------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| The **verbatim** `/etc/os-release` `ID` and `VARIANT_ID`                                                 | Four sources contradict each other; only the machine settles it (WS0).                                                                                                                         |
| Whether `chsh` is present, and from which package                                                        | Same contradiction; ADR-8 is correct either way, but the answer should be recorded.                                                                                                            |
| `bootstrap.sh` actually downloading the binary on a private prerelease repo                              | No CI job exercises the shim against a private prerelease origin. **This is step 1 of the checklist, run with `GH_TOKEN` deliberately unset**, so success or failure is observed in isolation. |
| SELinux behaviour under real provisioning                                                                | No container on an x86 host exposes `/sys/fs/selinux`. Checklist records `getenforce` before and after.                                                                                        |
| 16 KiB page-size interaction with any C-linked third-party binary                                        | Not modelled by any x86 container.                                                                                                                                                             |
| A live KDE Plasma 6 session: the merge's effect on a Retina display, and `--reload`'s component restarts | `dotf plasma diff` runs headless (`plasma.session=unknown`), but the visible result cannot be judged headlessly. **Never pass `--reload` on the first run.**                                   |
| `hostnamectl set-hostname`                                                                               | Cannot be exercised in any container (`/etc/hostname` is owned by the runtime).                                                                                                                |
| btrfs / snapper behaviour                                                                                | Containers are overlayfs; the snapshot path is exercised only through the fake.                                                                                                                |
| arm64 **plus** systemd together                                                                          | Unreachable under `qemu-user` (`226/NAMESPACE`); see ADR-15.                                                                                                                                   |
| The real NetworkManager/firewalld zone on a roaming laptop                                               | A container's zone is not a laptop's zone.                                                                                                                                                     |

### Device acceptance gate

`docs/fedora-first-run.md` is walked **end to end on the actual M1 before the release tag is cut**. Following the
Arch precedent's own closing Advice — _"the existing manual verification surfaces should be part of the
implementation acceptance criteria, not post-hoc checks"_ — the checklist is a gate, not a report.

---

## Advice

- Keep `manifest/packages.toml`, `internal/backend/backend.go`, `internal/symlink/table.go` and
  `internal/sysconfig/linux.go` **single-owner** across workstreams. They are the highest-risk coordination
  surfaces, and three of the four have already drifted internally at least once.
- Land ADR-2's `applicableFor` and the rewritten coverage test in **one commit**. Separating them ships a silent
  regression window on macOS and Arch.
- WS3 before anyone runs `dotf link` on the M1. A Fedora rollout whose first mutating command deletes the user's
  editor config is a failed rollout regardless of how correct the dnf argv is.
- Do not "fix" `internal/backend/pkg.go`'s dnf argv. They are dnf5-correct. The dnf5 problem is entirely in the
  repo verbs, which ADR-6 routes around.
- Do not unify `internal/symlink`'s private `isLinuxFamily` with `platform.IsLinuxFamily`. The `OSLinux` exclusion
  is deliberate and pinned by a golden test; `FacetLinux` must use the local predicate.
- Correct documentation drift as part of the implementation, not after. The user's next test is a clean Fedora
  Asahi install, so a `platform-detection.md` that documents only the shell detector and a `CLAUDE.md` that claims
  `bootstrap.sh` is 307 lines will create avoidable false negatives on the exact file the first boot depends on.
- Treat `dotf apply --dry-run` as a **safety-critical surface**, not a convenience. The whole device-first plan
  rests on it being honest, which is why WS1 fixes its applicability filtering before any mutation is attempted.
