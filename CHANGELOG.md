# Changelog

All notable changes to the `dotf` CLI and this dotfiles repository are documented here.
This project adheres to [Conventional Commits](https://www.conventionalcommits.org) and
[Semantic Versioning](https://semver.org).

## [Unreleased]

### Features

- Install OpenCode 2 through `bin/install-opencode.sh`: remove an OpenCode 1 package
  install while preserving configuration and data, then install V2 with the official
  per-OS method. The side-by-side `opencode2` manifest package is retired.
- Install Codex CLI through `bin/install-codex.sh` with the official standalone installer
  instead of npm: npm/pnpm/bun/Homebrew installs are removed first, and the manifest
  `codex` entry moves from `toolchain`/npm to this script (`--update`, `--force`,
  `--uninstall`, `NON_INTERACTIVE`).
- Copy live private env directories and a fixed agent and editor file list to other
  hosts with `bin/sync-configs.sh` (`-n`, `-d`, `-g`, `-l`). This replaces
  `sync-agent-configs.sh` and `sync-private-env.sh`.
- Back up a real file before `dotf link` or `dotf apply` replaces it. Each run
  writes `~/.config/dotfiles_backup_<epoch>` and a `.dotf-backup.json` manifest.
  `dotf backup list`, `restore`, and `prune` manage those directories (#63, #70).
- `dotf link` does not modify files inside the repo. The Zed settings copy is
  written only when the destination is missing, a symlink, or different, and a
  real file that differs is backed up first. The repo root is resolved to an
  absolute path before linking (#71).
- `dotf link --prune` removes stale managed symlinks. `dotf unlink` removes
  named managed links; `--restore` puts back the newest backup of each one (#77).
- Bash and zsh load machine-local overrides and secrets last, from
  `common/local.sh` (#73).
- Cache the output of eval'd tool init, and run zsh `compinit` once (#78).
  `_dotfiles_compinit` keeps `~/.zcompdump` from being rewritten on every start
  when `compaudit` flags `fpath` entries (#85).
- Platform predicates read the cached `DOTFILES_*` variables (#80).
  `dotf platform --env` is what the shell uses for platform detection (#82).
- Each shell setup step has one owner, shared by bash and zsh (fzf, venv
  auto-activation, aliases). Fabric pattern aliases no longer use `declare -A`,
  so they load under macOS `/bin/bash` 3.2 (#89).
- Define kubectl aliases only when `kubectl` is on PATH. Prefer `bat` over
  `batcat` and `fd` over `fdfind` for the `bat`, `less`, and `fd` aliases,
  `PAGER`, and the fzf file commands, so the Debian/Ubuntu names work without
  symlinks (#94).
- Install graphify with `bin/install-graphify.sh` (uv tool `graphifyy`, extra
  `all` by default). The `graphify` function routes calls through the tokenhop
  OpenAI endpoint when `TOKENHOP_OPENAI_API_KEY` is set; `graphify-raw` calls
  the binary directly.
- Add Claude aliases `ccdp` and `cc-rc` (permission checks bypassed; `cc-rc`
  also sets `DO_NOT_TRACK=0` and runs `rc`).
- Add the Espanso `:projdone` snippet, which asks to finish every remaining
  issue in a Linear project.
- Launch-test the arm64 desktop apps on Fedora Asahi Remix. Obsidian's arm64
  AppImage requires `appimage-libz` so `libz.so` is present (#57).

### Fixes

- Leave dnf repo files that dotf does not own in place. `repo_gpgcheck = true`
  on a `baseurl` repo also checks the signed repository metadata (#51).
- Expand `requires` only from packages that apply to this host (#54). Reject
  `requires` cycles when the manifest is loaded, on every host (#56).
- Install only the pinned apt repository key, with its subkeys, into the
  `signed-by` keyring (#60).
- Show git branch, status, and virtualenv text literally in the bash fallback
  prompt. Set the bash terminal title from `$PWD` on every prompt (#64).
- Pass `--non-interactive` to the atuin installer and install the binary in
  `~/.atuin/bin`, which is already on PATH.
- Run `mise reshim` after `bin/install-playwright-cli.sh` installs the npm
  package when `mise` is on PATH, so a mise-managed Node can see
  `playwright-cli`.
- Treat Homebrew Python as user-owned in `bin/install-omp.sh` when ensuring
  debugpy. It does not install the distro debugpy package, and it prints
  `$DEBUG_PYTHON -m pip install --user --break-system-packages debugpy`.
- Non-interactive login bash (`bash -l -c`) sources platform and path setup,
  so `DOTFILES_OS` is set. Reload guards are per-process, so a nested bash
  loads its own config (#91).

### Tests

- Assert one applicable install route per OS family and architecture for every
  manifest package family (#50).
- Run `scripts/test-*.sh` and the bats suite in `tests/shell` on Linux and
  macOS CI (#85, #91).

## [0.3.0] - 2026-09-16

First stable release of `dotf`. Promotes the cross-platform provisioning pipeline after
adding Fedora parity, governed package lifecycle operations, diagnostics and
self-update, reproducible release locking, user overlays, KDE Plasma management, safer
symlink backups, and broader native backend coverage. See
[docs/releases/v0.3.0.md](docs/releases/v0.3.0.md) for full notes.

### Features

- Add first-class Fedora, Fedora Asahi, and RedHat-family provisioning.
- Add package lifecycle, diagnostics, drift, lock/verify, and self-update commands.
- Add KDE Plasma 6 capture, diff, and apply workflows.
- Add native repository, font, git-clone, and GitHub release backends.
- Add Vercel, Railway, Stripe, Neon, and OpenCode 2 npm-managed CLIs.
- Add Herdr provisioning and `dotf deb` Debian-to-pacman conversion.

### Fixes

- Back up populated directory destinations before symlink replacement.
- Repair Linux shell-module loading and reduce zsh startup latency.
- Harden Fedora aarch64 package provisioning and prerelease bootstrap downloads.
- Strip host-local Plasma values that caused persistent export drift.

## [0.2.0-beta] - 2026-07-03

Second beta of `dotf`. Turns the provisioning path from a scaffold into a working
end-to-end setup: the package manifest is populated, the legacy `bin/install-*.sh`
installers and system-setup steps are wired into `dotf apply`, and the interactive
workflow aborts gracefully. See [docs/releases/v0.2.0-beta.md](docs/releases/v0.2.0-beta.md)
for the full notes.

### Features

- `bootstrap.sh` gains a `--repo` flag and forge-aware release downloads.
- Bootstrap Homebrew in `apply` before the packages phase (#32).
- Wire the legacy `bin/install-*.sh` installers into the manifest via opt-in
  `runtimes`/`ai`/`cloud`/`git-tools`/`system` intents (#33).
- Provision system-setup steps — XDG/dev dirs, TPM, sessionizer, GNU utils (#35).
- Graceful Ctrl+C / Esc exit for the interactive `plan`/`apply` workflow, exiting with
  `dotf: aborted` and status 130 (#37).

### Fixes

- Populate the package manifest with the core modern-CLI toolset via the `pkg` backend
  and validate toolchain tools at load time (#26).
- Skip platform-inapplicable backends in `install`, `apply`, `status`, and `diff`
  (#10, #27).
- Make the apt-repo keyring step idempotent and the AUR `yay` bootstrap re-runnable with
  a `makepkg` pre-flight (#22, #24).
- Use exact-match installed probes for zypper and apt (#25).
- Size the Packages multiselect to the manifest and preserve deselections across intent
  changes (#23, #28).
- Pin github-release downloads by tag and add the docker-ce repo codename (#29).
- Reject unknown `--intent` values and route `plan` prompts to stderr so stdout stays
  machine-greppable (#30, #31).

## [0.1.0-beta] - 2026-07-02

First public beta of `dotf`, the native Go provisioning CLI. Marks the cutover from the
legacy bash/Ansible bootstrap to a single, checksum-verified binary distributed via
GitHub Releases. See [docs/releases/v0.1.0-beta.md](docs/releases/v0.1.0-beta.md) for the
full notes.

### Breaking Changes

- `bootstrap.sh` is now a download-and-exec shim for the `dotf` binary; the in-process
  bash provisioning path is retired as the entry point.

### Features

- Provisioning CLI foundation with platform detection and `version`/`platform`/`plan`
  subcommands (Phase 1).
- Manifest schema and intent-selection TUI (Phase 2).
- Multi-backend install engine across Homebrew, apt, dnf, pacman/AUR, and zypper
  (Phase 3).
- Native Go symlink engine and `dotf link` with `--dry-run`/`--backup`/`--linux-desktop`
  (Phase 4).
- Per-OS system config engine and `dotf sysconfig` (Phase 5).
- Headless `dotf apply` orchestration with flag/env parity, snapper snapshots, and a
  run-wide failure summary (Phase 6).
- GoReleaser distribution and tag-triggered release workflow (Phase 7).
- Global and per-command help (`--help`/`-h`).
- `dotf completion` for bash, zsh, and fish.

### Fixes

- Resolve AUR home lookup on CI without a passwd entry.
- Make status and AUR tests pass on Ubuntu CI.

[Unreleased]: https://github.com/yandy-r/dotfiles/compare/v0.2.0-beta...HEAD
[0.2.0-beta]: https://github.com/yandy-r/dotfiles/compare/v0.1.0-beta...v0.2.0-beta
[0.1.0-beta]: https://github.com/yandy-r/dotfiles/releases/tag/v0.1.0-beta
