# Roadmap

This repository is a cross-platform dotfiles and provisioning system: a shared shell configuration layer under
`common/`, `bash/` and `zsh/`, and a Go provisioning CLI (`dotf`) that owns symlinks, packages, per-OS system
configuration and the curated KDE Plasma 6 config set. After the Fedora work described below, the supported matrix is
macOS, Debian/Ubuntu, RedHat/Fedora (including Fedora Asahi Remix), the Arch family, openSUSE and WSL, on both
`x86_64` and `arm64`.

This file records _direction_ only — what has shipped, what is queued, and what has been deliberately excluded.
Authoritative per-feature specifications live under `docs/plans/`; where this page and a plan disagree, the plan wins.

## Shipped

- **Phase 4 — native symlink engine.** `dotf link` (`internal/symlink`) replaced the Ansible playbook as the symlink
  provisioner. `playbooks/symlinks.yml` is retained as a historical rollback path only.
- **Phase 5 — `dotf sysconfig`.** Per-OS hostname, timezone, locale, login shell, firewall and ssh-server domains
  (`internal/sysconfig`), each planned before it is applied.
- **Phase 6 — `dotf apply`.** Headless orchestration: sysconfig → packages → symlinks → plasma → firewall/ssh-server in
  one ordered pass, with `bootstrap.sh` flag/env parity, an opt-in pre-apply snapper snapshot and a run-wide non-fatal
  failure summary.
- **Phase 7 — release pipeline.** `.goreleaser.yml` and the release workflows landed, and `bootstrap.sh` became a POSIX
  download-and-exec shim that fetches the checksum-verified `dotf` binary from GitHub Releases and execs `dotf apply`.
- **`dotf plasma`.** A native Go KConfig codec — no `kwriteconfig6` subprocesses — with `export`/`apply`/`diff`/`status`
  verbs over a governed managed-file table, plus the matching opt-in `dotf apply --plasma` phase.

Fedora first-class support then landed as eight workstreams:

- **First-boot unblock.** `bootstrap.sh` gained a tag-pinned binary-download fallback for the case where
  `/releases/latest/download` 404s on a prerelease tag, the Go platform detector maps every Fedora Asahi Remix
  os-release `ID` candidate onto the RedHat family, and `dotf apply --dry-run` now filters its package preview through
  backend applicability so the printed list matches what would actually install.
- **Package correctness.** `names` became the applicability declaration for the `pkg` backend, so a manager with no
  mapped name reports one clean not-applicable row instead of aborting `dotf status`. Fedora ships no `lazygit` or
  `yazi` RPM in any release or updates branch, so both gained `os = "redhat"` github-release siblings rather than
  converting the shared entries; `lutris` gained a `lutris-pkg` sibling.
- **Symlink safety and facets.** Directory destinations are backed up before they are removed — the previous
  un-backed-up recursive delete was a live data-loss risk on a populated daily driver. Facets are now named by
  capability rather than by distro, and `--hyprland` is the primary flag with `--linux-desktop` retained as a
  deprecated alias.
- **The per-OS shell module loader (ADR-14).** The three loaders in `common/{aliases,exports,functions}.sh` sourced
  `$SCRIPT_DIR/$DOTFILES_OS/…`, but `DOTFILES_OS` holds a distro token (`debian`, `redhat`, `arch`, `suse`) and never
  `linux` — so `common/linux/aliases.sh` loaded on no Linux distro at all. `os_module_dirs()` in `common/platform.sh`
  is now the single source of truth for a family-then-overlay search path. The same change stopped the dnf and yum
  `history` aliases shadowing the shell builtin (they are `pkghistory` now), routed the RedHat and openSUSE firewall
  aliases to firewalld, probed `systemsettings` instead of hardcoding the Plasma 5 binary name, and taught
  `locale_exists` to fold the codeset separator so `de_DE.UTF-8` matches glibc's `de_DE.utf8` spelling.
- **CI.** The Go suite and `dotf` itself now run against a real Fedora userspace container, and every tracked shell
  script is gated with `shellcheck`.
- **Desktop layer.** Plasma honours `XDG_DATA_HOME`/`XDG_DATA_DIRS` for colour schemes, resolves the KWin reconfigure
  through a full-argv candidate table, and no longer manages the host-local default browser. Fonts got a native `font`
  backend instead of a `custom-script` shortcut, and the pinned Nerd Font families install through it.
- **Sysconfig Fedora domains.** Locale installs `glibc-langpack-<lang>` rather than downgrading `LANG`, firewalld is
  installed when missing, `chsh` is guarded with `LookPath` and falls back to `usermod -s`, ufw installs through the
  host family's own package manager, the Homebrew bootstrap is declined on arm64 Linux with an explicit stderr note,
  `dotf doctor` reports the SELinux mode, and os-release is scanned once to expose `VersionID`.
- **Repo backend.** Third-party repositories dispatch by package-manager family through a single `repo` backend with
  `apt_repo` and `dnf_repo` sub-tables, which is what makes docker-ce install on Fedora.

Alongside the code, `docs/fedora-first-run.md` records the Apple Silicon first-run acceptance checklist,
`docs/fedora-rehearsal.md` the local container rehearsal loop, and `.docs/fedora-package-mapping.md` the dnf name
mapping with the release it was verified against.

## Near-term

Worth doing, but off the first-boot path:

- **`CHANGELOG.md` backfill and the full multi-file documentation drift pass.** The Fedora work corrected the docs it
  touched; the rest of the tree has not been swept.
- **Ratchet-_down_ candidates for the `custom-script` budget.** `atuin` and `azure-cli` both have official Fedora
  packages and are strong candidates for a native backend. The rule is worth stating plainly: `customScriptBaseline`
  in `internal/manifest/ratchet_test.go` is an exact-match tripwire in **both** directions, currently **16** — moving
  an entry off `custom-script` requires lowering the baseline in the same commit, exactly as adding one requires
  raising it.
- **openSUSE third-party repos** as a `zypper_repo` sub-table on the existing `repo` backend. Explicitly _not_ a new
  backend — ADR-5 made `repo` the family-dispatching seam precisely so this stays a sub-table.
- **Retiring or wiring the orphaned `bin/install-*.sh` scripts.** `install-docker.sh`, `install-native-packages.sh`,
  `install-arch-pkgs.sh`, `install-modern-cli.sh` and `install-brew.sh` are among those unreachable from
  `bootstrap.sh → dotf apply`. Either they become reachable or they go.
- **Wiring the `sysconfig` `[[module]]` step kind and its `ports` list.** `dotf setup` is not reachable from the
  bootstrap pipeline at all, so the module surface is currently inert.
- **Threading `privilege.Sudo` into the backends.** `internal/backend/register.go:15` is literally `_ = sudo`. New code
  follows the existing literal-`"sudo"` argv convention so that a future fix stays one refactor rather than several.
- **A dependency graph on `Package`.** The npm/cargo/go toolchain entries (`codex`, `lefthook`, `tmux-sessionizer`,
  `fabric`) assume a node, cargo or Go toolchain that is provisioned on **no** distro. This is a pre-existing
  cross-platform hole, not a Fedora one.
- **Widening `bin/style.sh`'s shell file set.** It selects shell files by a `.sh` extension, so the extension-less
  ones — `bash/bashrc`, `zsh/zshrc`, `zsh/zprofile`, `bash/profile` — are linted by neither CI nor the pre-commit hook.
- **Priming sudo in `dotf apply`'s sysconfig phases.** `cmd/dotf/apply.go` never consults `FirewallPlanNeedsSudo` or
  `SSHServerPlanNeedsSudo`; only `cmd/dotf/sysconfig.go:76` does. This is pre-existing, but the firewalld tri-state
  read widened its blast radius: `dotf apply --firewall auto` on Fedora now plans against an unprimed sudo and defers
  the domain as UNKNOWN.
- **Reconciling `common/platform.sh`'s `detect_package_manager` with the Go detector.** After the arm64 work the Go
  detector demotes brew below the native manager on aarch64 Linux; the shell side still probes `brew` first,
  unconditionally.

## Explicitly deferred

Each row is a decision with its reason, not an oversight.

| Item                                                                                | Why deferred                                                                                                                                                                                                                                                                                                   |
| ----------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A distinct `platform.OSFedora` / `OSAsahi` constant                                 | Fedora **is** `OSRedHat`. A new value ripples through the sysconfig registry, the symlink facet switch, the OS gate, manifest validation, the TUI's known-OS list and a dozen test tables for zero behavioural gain.                                                                                           |
| rpm-ostree / Silverblue / Kinoite / Bazzite                                         | The target device is a traditional rpm/dnf install. Immutable hosts need a new `PkgMgr` value plus several backend switches, for a host nobody is testing.                                                                                                                                                     |
| Implementing **or** removing `platform.PkgMgrYUM`                                   | Orthogonal: Fedora always has dnf, so it can never fire there. The `names` change already downgrades a yum-only host from "aborts `dotf status`" to a clean not-applicable row.                                                                                                                                |
| A `platform.Info` capability table (`Caps`)                                         | Fields with no call site are speculative surface. Only `VersionID` was added, and only in the workstream that needed `$releasever`.                                                                                                                                                                            |
| aarch64 CI, and an arm64 + systemd emulated environment                             | ADR-15. The binary is a CGO-free cross-compile with no arch-specific codegen; hosted arm64 runners are paid on a private repo; and a self-hosted runner on the target laptop would invert the safety property this work protects. arm64 + systemd is unreachable under `qemu-user`.                            |
| Porting `bin/setup-fingerprint.sh` to Fedora/authselect                             | Touch ID runs through Apple's Secure Enclave and sits in Asahi's "TBA — no active work" tier, so a correct implementation would be dead code on the target device. The module is gated instead.                                                                                                                |
| RPM Fusion enablement                                                               | Nothing in the manifest needs it — every resolving dnf name comes from Fedora's own release/updates repos. Adding a privileged third-party repo speculatively is worse than not having it.                                                                                                                     |
| Unifying `internal/symlink`'s private `isLinuxFamily` with `platform.IsLinuxFamily` | The `platform.OSLinux` exclusion in the local predicate is deliberate and is pinned by golden test rows. Any facet resolving Linux must go through the local predicate.                                                                                                                                        |
| SELinux `semanage port` labelling                                                   | Current exposure is genuinely low: symlinks write only under `$HOME`, sysconfig copies into existing `/etc` inodes, and Plasma renames from the destination's own directory. Labelling only becomes necessary when sshd moves off port 22.                                                                     |
| The Fedora `/etc/bashrc` severance                                                  | Fedora's bash is not built with `-DSYS_BASHRC`, so replacing `~/.bashrc` severs `/etc/profile.d` for the interactive non-login shell a Konsole tab starts. The loader fix deliberately did not touch the login path; adding a guarded source is a behaviour change on four platforms and needs its own review. |
| Speculative tests                                                                   | Each plan commits to a named critical test set and nothing beyond it. A test with no failure it can catch is maintenance cost without coverage.                                                                                                                                                                |

## Known risks carried forward

These are live, understood and accepted for now; each ships with the mitigation that makes it survivable. The full
risk register lives in the design document under `docs/plans/fedora-support/`.

- **Homebrew on arm64 Linux (R3).** Homebrew publishes no aarch64-Linux bottles and its installer refuses on that host.
  _Mitigation:_ `--homebrew=true` is now declined on arm64 Linux with an explicit stderr note, and the package-manager
  detector demotes brew below the native manager on aarch64 Linux **only** — an `x86_64` host passing
  `--homebrew=true` still routes packages through brew exactly as documented.
- **rpm-ostree variants (R6).** On Silverblue, Kinoite, Bluefin, Bazzite and the ostree Asahi spin, `/usr/bin/dnf` is
  absent by design, so detection returns `PkgMgrUnknown` and every `pkg` package fails with an
  `UnsupportedPkgMgrError` — a wall of errors rather than one legible message. _Mitigation:_ recorded, not fixed. If it
  ever enters scope, the canonical detection signal is `/run/ostree-booted`, and the right behaviour is to skip the
  packages phase with a single message while keeping symlinks, plasma and sysconfig — not to abort the run.
- **Snapper gives false rollback confidence on Fedora (R7).** `internal/snapper` gates only on the Linux family plus
  `LookPath("snapper")` — no btrfs check, no snapper-config check — and emits `sudo snapper create` with no `-c`
  argument. Fedora ships no snapper and no config, and its btrfs layout uses bare `root`/`home` subvolumes rather than
  openSUSE's `@`. Snapshot failures are never recorded in the failure collector, so `dotf apply --snapper` exits **0**
  having protected nothing. _Mitigation:_ `--snapper` is on the Fedora first-run checklist's DO-NOT-USE list; a
  preflight tarball is the real rollback.

## Proposing a change

- **Issues** use the YAML form templates in `.github/ISSUE_TEMPLATE/` — `bug_report.yml`, `feature_request.yml` and
  `docs_request.yml`. Blank issues are disabled; do not bypass a template with a title-only issue.
- **Labels** come from the colon-prefixed taxonomy (`type:`, `area:`, `platform:`, `priority:`, `status:`) plus the
  standalone GitHub set. Never invent an ad-hoc label.
- **Design documents** belong under `docs/plans/<feature-name>/`, committed with `docs(internal):` so they stay out of
  the release notes. A change large enough to need a roadmap entry is large enough to need a plan.

Last updated: 2026-08-28
