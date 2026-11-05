# dotfiles

Cross-platform dotfiles managed by a unified shell configuration system plus a Go provisioning CLI
(`dotf`).

## Installation / Quick-start

As of Phase 7, `bootstrap.sh` is a POSIX download-and-exec shim (OS/arch detection, release download
with prerelease-aware GitHub fallback, checksum verification, and a consent-gated from-source path):
it detects your OS/arch, downloads the checksum-verified `dotf` binary from GitHub Releases
(including betas when no full release exists), and execs `dotf apply`. Private repos need `GH_TOKEN`
/ `GITHUB_TOKEN` or `gh auth login`. All provisioning is done by `dotf`, not by the shim.

```bash
# Clone to the standard location
git clone https://github.com/yandy-r/dotfiles.git ~/.config/dotfiles
cd ~/.config/dotfiles

# Guided install — the shim downloads `dotf` and runs the `dotf apply` intent TUI
./bootstrap.sh

# Headless / unattended install
./bootstrap.sh --force

# No prebuilt asset: install official Go under ~/.local/go and build from this clone
./bootstrap.sh --install-go
```

If no release asset is available, on a TTY the shim can prompt to install official Go 1.26+ under
`~/.local/go` and build from the clone (default **N**). Non-interactive runs need `--install-go` for
that path. Manual escape hatch:

```bash
go build -o dotf ./cmd/dotf
./dotf apply
```

## `dotf` provisioning CLI

Provisioning is **manifest-driven**. `dotf` embeds a declarative TOML manifest
(`manifest/packages.toml`, `go:embed`-ed) that maps intents → packages, and drives a 7-backend
install engine: `pkg` (native package managers), `github-release`, `aur`, `toolchain` (cargo/npm),
`apt-repo`, `script-url`, and `custom-script`. **`dotf apply` is the primary provisioning path.**
The `bin/install-*.sh` scripts are **retained** and invoked by the manifest through the
`custom-script` backend — they are governed adapters for genuine setup steps, not the primary
install path (you can still run them directly for targeted installs; see
[Specialized Installer Scripts](#-specialized-installer-scripts)).

### `dotf link` — native symlink engine

`dotf link` provisions the managed dotfiles symlinks into `$HOME` natively in Go
(`internal/symlink`), and is the symlink provisioner as of Phase 4.

Every real file or directory a link replaces is copied into
`~/.config/dotfiles_backup_<epoch>` first, by default, together with a `.dotf-backup.json`
manifest recording each `$HOME`-relative path, its kind, and when it was copied. The run names that
directory on stderr (`dotf: link backups: <dir>`) and creates it only when something was backed
up. `dotf setup` symlink steps back up the same way, and all steps of one setup run share a single
dir, named once at the end of the run.

| Flag              | Effect                                                                                                                  |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `--dry-run`       | Print the plan without touching the filesystem                                                                          |
| `--no-backup`     | Replace real files without backing them up first                                                                        |
| `--backup`        | Explicit spelling of the default; `--backup=false` does not disable backups (use `--no-backup`); not with `--no-backup` |
| `--linux-desktop` | Manage the opt-in Linux desktop config set (hypr/waybar/swayosd/fuzzel)                                                 |
| `--prune`         | Remove stale repo links first: dangling, or a managed source at an old spot; table links whose source was deleted       |

```bash
dotf link --dry-run          # preview the plan (link.backup= lists what will be copied)
dotf link                    # apply, backing up real files first
dotf link --no-backup        # apply without backups
dotf link --linux-desktop    # include the opt-in Hyprland desktop set
dotf link --prune --dry-run  # list stale links (link.prune=) without removing them
dotf link --prune            # remove stale links, then link as usual
```

`--prune` scans only the parent dirs of the table's dests, one level deep, and judges each symlink
by its own target (never following a chain). A link that is not a table dest is pruned only when
its target in the repo is missing, or is a table entry's source (the same source linked from an
old location); a table dest is pruned only when its source was deleted. Real files are never
touched, and parent-dir links of table dests (e.g. `~/.config/zed -> $REPO/zed`), links under
`plasma/files/` (owned by `dotf plasma`), links to the repo root, and live links to other repo
paths (e.g. `~/.claude -> $REPO/.claude`) are always kept. `dotf doctor` warns about stale links
as `check.symlinks.stale`.

The Ansible playbook (`playbooks/symlinks.yml`) is **historical-rollback-only**; it is not run
during normal provisioning.

### `dotf backup` — list, restore, and prune link backups

`dotf backup` manages the `~/.config/dotfiles_backup_<epoch>[_N]` dirs that `link`, `apply`, and
`setup` create.

| Verb                       | Effect                                                                                                                                                                                                         |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `list`                     | Every backup dir, newest first: id, created time, entry count, size. Dirs from before manifests existed show as `legacy=true`                                                                                  |
| `restore <id> [path...]`   | Copy the manifest's entries (or just the named paths) back into `$HOME`. A missing dest or a symlink is replaced; a real file/dir is refused unless `--force`, which first backs it up to a **new** backup dir |
| `prune`                    | Delete all but the newest `--keep` dirs (default 10), optionally only those older than `--older-than` (`720h`, `30d`). Confirms first; non-interactively it needs `--yes`                                     |

Restore copies from the backup and never modifies it; legacy (manifest-less) dirs and unsafe paths
are refused. Prune only ever removes `dotfiles_backup_<epoch>[_N]` dirs. `dotf doctor` reports the
count and total size (`check.backups.count`) and warns above 10.

```bash
dotf backup list                                # what is there
dotf backup restore 1729543200 --dry-run        # preview the restore plan
dotf backup restore 1729543200 .bashrc          # restore one file
dotf backup restore 1729543200 --force          # overwrite real files (they are backed up first)
dotf backup prune --older-than 30d --dry-run    # preview, then rerun with --yes
```

### `dotf unlink` — remove managed links

`dotf unlink <dest...>` removes the named managed symlinks (`$HOME`-relative link-table names; a
leading `~/` is accepted), and `--all` targets every table entry. Unknown names fail (exit 2)
before anything changes. Only a symlink that points into the repo is removed: real files and
directories are never touched, and foreign symlinks, missing dests and the Zed copy entry are
skipped with a reason on stderr (under `--all`, missing dests and the copy entry are only counted in
the summary). stdout carries `unlink.remove=`, `unlink.skip=` and
`unlink.restore=` lines.

| Flag        | Effect                                                                                                                   |
| ----------- | ------------------------------------------------------------------------------------------------------------------------ |
| `--all`     | Unlink every table entry (all facets); cannot be combined with named dests                                               |
| `--restore` | Restore each unlinked dest from the newest backup holding it, with `dotf backup restore`'s checks; no backup = unlinked  |
| `--dry-run` | Preview what would be unlinked and restored                                                                              |

```bash
dotf unlink .bashrc .zshrc              # remove two links
dotf unlink ~/.config/nvim --restore    # put your pre-dotfiles config back
dotf unlink --all --dry-run             # preview removing every managed link
```

### `dotf sysconfig` — per-OS system configuration

`dotf sysconfig` (`internal/sysconfig`) owns OS-level system configuration — hostname (and
`/etc/hosts`), timezone, locale, default shell, and firewall — per OS/distro family, with an
idempotent plan/apply split (a second run on a converged host is a no-op).

| Flag                | Effect                                                       |
| ------------------- | ------------------------------------------------------------ |
| `--hostname <name>` | Set the static hostname (per-machine; flag only)             |
| `--timezone <tz>`   | Set the IANA timezone (e.g. `America/New_York`)              |
| `--locale <locale>` | Set the system locale/`LANG` (Linux only; macOS no-op)       |
| `--shell <shell>`   | Set the real user's login shell                              |
| `--firewall <mode>` | Enable a firewall backend: `ufw`, `firewalld`, `auto`, `off` |
| `--dry-run`         | Print the plan without executing                             |
| `--yes`             | Apply without an interactive confirmation prompt             |

```bash
dotf sysconfig --hostname web01 --timezone America/New_York \
  --locale en_US.UTF-8 --shell zsh --firewall auto --dry-run
```

Windows is a documented, deferred stub. Non-interactive applies require `--yes`.

### `dotf plasma` — curated KDE Plasma 6 configuration

`dotf plasma` (`internal/plasma`) manages a curated subset of KDE Plasma 6 configuration as
repo-tracked data in [`plasma/`](plasma/README.md). All INI work goes through a native Go KConfig
codec — no `kwriteconfig6` subprocesses — and the managed set is a governed built-in table, so there
is deliberately no `--manifest` flag. Linux only.

| Verb     | Effect                                                                                                                                                 |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `export` | live → `plasma/files/`, stripping volatile state (machine UUIDs, monitor ids, window geometry, migration ledgers) so nothing machine-local reaches git |
| `apply`  | `plasma/files/` → live as a **per-key merge**: every repo key is written, every live-only key is preserved, so an apply is never a blind overwrite     |
| `diff`   | per-key drift in both directions; exits 1 when any file differs (the CI convergence check)                                                             |
| `status` | presence and drift counts for the managed set                                                                                                          |

| Flag             | Effect                                                                                                  |
| ---------------- | ------------------------------------------------------------------------------------------------------- |
| `--only <paths>` | Comma-separated managed file paths to act on (default: all)                                             |
| `--recommended`  | Layer the opt-in `plasma/recommended/` overlay on top of `plasma/files/`                                |
| `--dry-run`      | Print the plan without executing                                                                        |
| `--backup`       | Back up every live file the run modifies before writing                                                 |
| `--link`         | Symlink each managed file into the repo instead of merging (`apply` only)                               |
| `--reload`       | Restart the affected KDE components afterwards (`apply` only; otherwise the exact commands are printed) |
| `--yes`          | Apply without an interactive confirmation prompt                                                        |
| `--json`         | Emit machine-readable JSON to stdout                                                                    |

```bash
dotf plasma status                        # what is managed, present, and drifted
dotf plasma diff --json                   # per-key drift, machine-readable
dotf plasma export --only kwinrc          # refresh one file's snapshot
dotf plasma apply --backup                # merge into live, with an undo copy
dotf plasma apply --recommended --backup  # also layer the opinionated overlay
```

The repo holds **copies**, not symlink targets, so experimenting in System Settings never dirties
the worktree. See [`plasma/README.md`](plasma/README.md) for why, and
[`plasma/recommended/README.md`](plasma/recommended/README.md) for every recommended setting with
its downside.

### `dotf deb` — convert a Debian package for pacman

`dotf deb <url|path>` (`internal/debtap`) converts a `.deb` into a pacman-installable `.pkg.tar.zst`
using [`debtap`](https://aur.archlinux.org/packages/debtap), verifying it at every stage.
Arch/pacman hosts only.

Preconditions are checked up front — `debtap`, `bsdtar` and `pkgfile` installed, and debtap's
translation database populated — and each one that has a known fix is explained and **offered**,
never run without an explicit yes. This exists because debtap emits a single message for five
different preconditions ("run `debtap -u`"), which is wrong for the ones syncing cannot fix.

One of those is a **bug in debtap 3.6.3**: its start-up check greps pkgfile's cache for
`*.files?(.[[:digit:]]{3})`, requiring a three-digit suffix (`core.files.000`), but pkgfile v26
writes plain `core.files`. The check can never pass, so `sudo debtap -u` never helps. `dotf deb`
detects exactly this case and offers a one-line fix: an empty marker file matching that pattern,
which pkgfile ignores because it names no repository.

Conversion changes nothing on the host — `--install` is opt-in and is the only step that elevates.

| Stage        | What is verified                                                                                                                                                                                                                                                          |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Acquire      | `https` only (plaintext `http` is refused outright). The input is downloaded **or copied** into a private 0700 directory and every later stage reads only that copy, so `--sha256` binds the bytes actually converted rather than a path that could be swapped afterwards |
| Input        | The file is a `.deb` by its **ar header**, not its extension — an HTML error page saved as `app.deb` is caught here. `--sha256` compares against an expected digest and fails closed                                                                                      |
| Precedence   | An existing **repo** package of the same name stops the run (`--force` overrides); an **AUR** match warns. A converted `.deb` shadows the native package and never receives updates                                                                                       |
| Artifact     | The built package's own `.PKGINFO` supplies name, version, size and dependencies                                                                                                                                                                                          |
| Dependencies | `pacman -T` reports which translated dependencies are unsatisfied — debtap's Debian→Arch mapping is approximate. Advisory only; pacman is the authority                                                                                                                   |
| Installed    | `pacman -Qq`/`-Qql` confirm what was actually registered and how many files it owns                                                                                                                                                                                       |

| Flag                     | Effect                                                                    |
| ------------------------ | ------------------------------------------------------------------------- |
| `--install`              | Install after converting (prompts unless `--yes`)                         |
| `--yes`                  | Skip the confirmation prompt (requires `--install`)                       |
| `--force`                | Convert despite an existing native package                                |
| `--out-dir <path>`       | Where the converted package is written (default: CWD)                     |
| `--sha256 <digest>`      | Expected digest of the input `.deb`                                       |
| `--pkgname` / `--pkgver` | Override the converted package's metadata                                 |
| `--keep`                 | Keep the staged input and the debtap work directory (both paths reported) |
| `--dry-run`              | Print the plan without executing                                          |
| `--json`                 | Emit machine-readable JSON to stdout                                      |

```bash
dotf deb ./slack-desktop.deb                        # convert only; prints the artifact path
dotf deb https://example.com/app.deb --sha256 9f2b…  # verified download
dotf deb ./app.deb --out-dir ~/pkgs --install       # convert, confirm, install
dotf deb ./app.deb --install --yes                  # non-interactive
```

Every pacman query is chosen for locale independence (`-Ssq`/`-T`/`-Qq`/`-Qql` and `.PKGINFO`, never
`-Si`/`-Qi`, whose field labels are translated), and a query that _fails_ aborts the run rather than
being mistaken for "nothing found" — a gate that could not run must not look like one that passed.

The command refuses to run as root: only `pacman -U` elevates, and it prompts for sudo itself.
Approving an install runs the package's install scripts as root, which the prompt says outright.

### `dotf apply` — headless provisioning orchestration

`dotf apply` runs the full provisioning pipeline in one ordered pass — **system config → packages →
symlinks → plasma → firewall/ssh server** — with `bootstrap.sh` flag/env parity. On a TTY without
`--headless` it drops into the guided intent TUI; headless it applies the resolved intent set with
no prompts. Non-fatal step failures are accumulated (never aborting the run) and reported in one
end-of-run summary; the exit code distinguishes a converged run (0), a completed run with soft
failures (1), a usage error (2), and a hard abort (3).

| Flag / env                               | Effect                                                                  |
| ---------------------------------------- | ----------------------------------------------------------------------- |
| `--headless` / `NON_INTERACTIVE=true`    | Skip the TUI and all prompts; apply the resolved intent set             |
| `--force` / `FORCE_INSTALL=true`         | Imply `--headless`; force symlink recreation                            |
| `--gaming` / `ENABLE_GAMING=true`        | Add the `gaming` intent to the packages phase                           |
| `--enable-ollama` / `ENABLE_OLLAMA=true` | Add the `ollama` intent (no-op until the manifest defines it)           |
| `--linux-desktop` / `LINUX_DESKTOP=true` | Manage the opt-in Linux desktop set (Linux: refused with `--no-backup`) |
| `--snapper` / `SNAPPER=true`             | Take a snapper pre-apply snapshot (Linux only; no-op otherwise)         |
| `--plasma` / `PLASMA=true`               | Run the KDE Plasma config sync phase (Linux/KDE only; no-op otherwise)  |
| `--homebrew=true\|false`                 | Homebrew tristate (`''` = OS default; Linux `true` is a no-op)          |
| `--intent <name>`                        | Narrow the headless intent set (repeatable)                             |
| `--dry-run`                              | Compose the plan without executing; never primes sudo                   |
| `--no-backup`                            | Replace real files in `$HOME` without backing them up (flag only)       |

```bash
dotf apply --dry-run                     # preview the composed plan, mutate nothing
dotf apply --headless --snapper          # unattended run with a rollback snapshot
dotf apply --plasma                      # include the KDE Plasma config phase
NON_INTERACTIVE=true dotf apply          # env-only headless (parity with bootstrap.sh)
```

The `--plasma` phase is deliberately narrower than the standalone `dotf plasma apply`: it never
prompts, never reloads running KDE components, and never layers `plasma/recommended/`. Backups are
forced on, because opting into an optional destructive phase should opt you into its safety net.

Booleans resolve as flag-OR-env, with truthiness locked to exactly `"true"`. As of Phase 7,
`./bootstrap.sh` is a POSIX download-and-exec shim that downloads the checksum-verified `dotf`
binary from GitHub Releases (prerelease-aware) and execs `dotf apply` (booleans still resolve
flag-OR-env, truthy `"true"`). Use `--install-go` to build from source when no asset is available.

The repository also includes a repo-owned generic Linux desktop baseline for CachyOS/Arch-style
Hyprland setups, with `fuzzel`, `waybar`, lock/idle flow, and monitor auto-switch support, activated
via `--linux-desktop` / `LINUX_DESKTOP=true`.

For non-interactive mode details, see [bin/NON_INTERACTIVE.md](bin/NON_INTERACTIVE.md).

### `dotf rc` — Claude Code Remote Control servers per repo

`dotf rc` (`internal/rc`) keeps one [Claude Code Remote Control](https://code.claude.com/docs)
server (`claude remote-control`) running per repo, each in an `rc` tab of that repo's
[Herdr](herdr/) workspace. A server is tied to the directory it starts in, so one per repo keeps
that repo's settings, MCP servers, hooks and git worktrees. The repo list is user-local, in
`${XDG_CONFIG_HOME:-~/.config}/dotf/rc.toml`, so you can add paths at will. See
[`docs/features/dotf-rc.md`](docs/features/dotf-rc.md) for the config format and behavior.

| Verb                 | Effect                                                                                           |
| -------------------- | ------------------------------------------------------------------------------------------------ |
| `add <path>…`        | Add repos (first add writes explicit defaults); `--name`, `--permission-mode`, `--spawn`         |
| `remove <name>…`     | Remove repos by name or path                                                                     |
| `list`               | Show the repos with their effective settings                                                     |
| `up`                 | Start every repo's server in Herdr; asks before restarting one already running (`--yes`)         |
| `status`             | Which repos have a running server, plus the service state                                        |
| `enable` / `disable` | Install / remove the `dotf-rc.service` systemd user unit that runs `dotf rc up` at login (Linux) |

```bash
dotf rc add ~/.config/dotfiles ~/Projects/github.com/me/app
dotf rc up --dry-run      # print each command; touches nothing
dotf rc up                # start now (or run this by hand after boot)
dotf rc enable --now      # ...or start at every login
dotf rc status
```

### `dotf completion` — shell completion scripts

`dotf completion <bash|zsh|fish>` prints a completion script to stdout, or with `--install` writes
it into that shell's standard completion directory. The command and flag set are generated from the
same help registry that drives `dotf help`, so completions never drift from the real CLI — adding a
command or flag updates completions automatically. The shell argument is optional; when omitted it
is detected from `$SHELL`.

| Flag        | Effect                                                                |
| ----------- | --------------------------------------------------------------------- |
| `--install` | Write the script to the shell's completion dir instead of printing it |
| `--force`   | Overwrite an existing installed completion script                     |
| `--dry-run` | With `--install`, report the target path without writing              |

Install targets follow the XDG base directories (all auto-loaded by their shell):

| Shell | Path                                              |
| ----- | ------------------------------------------------- |
| bash  | `$XDG_DATA_HOME/bash-completion/completions/dotf` |
| zsh   | `$XDG_DATA_HOME/zsh/site-functions/_dotf`         |
| fish  | `$XDG_CONFIG_HOME/fish/completions/dotf.fish`     |

```bash
dotf completion zsh                      # print the zsh script to stdout
dotf completion bash --install           # install for bash
dotf completion fish --install --force   # reinstall for fish, overwriting
source <(dotf completion zsh)            # load into the current zsh session
```

## ✨ Key Features

### 🌍 Cross-Platform Compatibility

- **Universal Support**: Works seamlessly across macOS, Linux distributions, and WSL
- **Intelligent Detection**: Automatic OS, architecture, and package manager detection
- **Unified Configuration**: Shared configurations with platform-specific optimizations
- **Smart Fallbacks**: Graceful degradation when tools aren't available

### 🐚 Enhanced Shell Experience

- **Unified Prompt**: Starship prompt provides identical experience in bash and zsh
- **Fast & Modern**: 5x faster than Starship with no framework overhead
- **Feature Parity**: Both bash and zsh have vi mode, modern keybindings, and enhanced functionality
- **Smart Aliases**: Context-aware aliases with fallbacks to native commands
- **Modern Tools Integration**: eza, bat, ripgrep, fd, fzf with intelligent fallbacks
- **Comprehensive Completions**: Universal completion system for 50+ modern CLI tools
- **Cross-Platform Functions**: Unified utility functions that work everywhere

Bash sources its bash-completion framework at top level before shared tools
(candidate paths are owned by `common/paths.sh`); both shells load fzf after tool
completions so their fallback loaders survive. Shared `.venv` auto-activation
waits until the first prompt after local overrides, and leaves manual environments
alone. Set `DOTFILES_NO_VENV_AUTO=1` to disable it. When
`${XDG_DATA_HOME:-~/.local/share}/blesh/ble.sh` is installed, interactive bash
loads it before the common modules and attaches it after `common/local.sh`.
Set `DOTFILES_NO_BLESH=1` to skip it.

### 🛠️ Developer-Focused

- **Cloud Provider CLIs**: Full support for AWS, GCP, Azure with completions
- **Kubernetes Ecosystem**: kubectl, helm, k9s, istioctl completions and aliases
- **Infrastructure as Code**: Terraform, Ansible, Pulumi with proper integrations
- **Language Support**: Node.js (NVM), Rust, Go, Python with version managers
- **Git Workflow**: Enhanced git configuration with modern diff tools (delta)

### 🖥️ Linux Desktop Baseline

- **Generic Hyprland Support**: Repo-owned Hyprland configuration for CachyOS and other Arch-family
  Linux installs
- **Desktop Runtime**: `fuzzel`, `waybar`, `hyprlock`, `hypridle`, screenshots/recording helpers,
  and monitor auto-switch dependencies install through the Arch package path
- **Safe Activation**: Desktop config deployment is opt-in via `--linux-desktop` /
  `LINUX_DESKTOP=true` and is refused with `--no-backup` outside a dry run, so existing
  `~/.config/hypr`, `~/.config/waybar`, or `~/.config/fuzzel` are always backed up first
- **Opt-in Deployment**: Generic Linux desktop configs are only deployed when explicitly requested
  with `--linux-desktop` / `LINUX_DESKTOP=true`

### 🔒 Security & Privacy

- **Telemetry Disabled**: Automatically disables telemetry for Homebrew, npm, .NET, etc.
- **XDG Compliance**: Follows XDG Base Directory Specification
- **Secure Secrets**: 1Password CLI integration with service account support
- **GPG Integration**: Proper GPG setup for commit signing

## 📁 Repository Structure

```
dotfiles/
├── aerospace/              # AeroSpace window manager config (macOS)
├── bash/                   # Bash shell configurations
│   ├── bashrc             # Enhanced bash config (uses common system)
│   ├── bash_profile       # Login shell configuration
│   ├── profile            # Profile configuration
│   ├── prompt.sh          # Modern bash prompt with git integration
│   └── .fzf.bash         # fzf keybindings (sourced by common/tools.sh:setup_fzf)
├── blesh/                 # ble.sh user config (linked to ~/.config/blesh)
├── bin/                   # Custom scripts and executables
├── common/                # Cross-platform configuration system ⭐
│   ├── platform.sh        # OS/architecture detection
│   ├── exports.sh         # Environment variables
│   ├── paths.sh           # PATH management
│   ├── aliases.sh         # Smart aliases with fallbacks
│   ├── functions.sh       # Cross-platform utility functions
│   ├── tools.sh           # Tool detection and setup
│   ├── completions.sh     # Universal completion system
│   ├── macos/             # macOS-specific configurations
│   │   ├── aliases.sh     # Dock, Finder, System Preferences
│   │   └── functions.sh   # Spotlight, Keychain, etc.
│   └── linux/             # Linux-specific configurations
│       └── aliases.sh     # systemd, package managers, containers
├── ghostty/               # Ghostty terminal config
├── herdr/                 # Herdr config (single file linked; runtime logs stay local)
├── fuzzel/                # Fuzzel launcher config
├── hypr/                  # Generic Hyprland desktop config
├── iterm2/                # iTerm2 terminal profiles and themes
├── monitor-auto-switch/   # ACPI lid/monitor automation for Hyprland
├── nvim/                  # Neovim configuration (LazyVim-based)
├── plasma/                # Curated KDE Plasma 6 config set (`dotf plasma`)
│   ├── files/             # `dotf plasma export` snapshot — never hand-edited
│   └── recommended/       # Hand-authored opt-in overlay (`--recommended`)
├── playbooks/             # Ansible automation scripts
│   └── symlinks.yml       # Historical Ansible rollback playbook
├── scripts/               # Utility scripts
├── ssh/                   # SSH client configuration
├── waybar/                # Waybar status bar config
├── wezterm/               # WezTerm terminal configuration
├── zsh/                   # Zsh shell configurations
│   ├── completions/       # Shell completions
│   ├── functions.zsh      # Legacy zsh functions
│   ├── fzf.zsh            # Fuzzy finder integration (sourced by common/tools.sh:setup_fzf)
│   └── zshrc             # Enhanced zsh config (uses common system)
├── starship.toml          # Unified prompt configuration (bash + zsh)
└── bootstrap.sh           # POSIX shim — downloads `dotf` and execs `dotf apply`
```

## 🔧 What the Bootstrap Script Does

`bootstrap.sh` is a POSIX download-and-exec shim: it detects your OS/arch, downloads the
checksum-verified `dotf` binary from GitHub Releases (prerelease-aware; auth via `GH_TOKEN` / `gh`),
and execs `dotf apply`. On download failure from a clone it can prompt (or take `--install-go`) to
install Go under `~/.local/go` and build. Provisioning itself is handled by `dotf` (system config,
packages, symlinks, firewall):

### 🕵️ Environment Detection

- **OS Detection**: macOS, Ubuntu, Fedora, Arch Linux, openSUSE, WSL
- **Architecture**: x86_64, ARM64, Apple Silicon
- **Package Managers**: Homebrew, apt, dnf, pacman, zypper
- **Smart Paths**: Dynamic Homebrew path resolution

### 📦 Package Management

- **Homebrew**: Cross-platform installation (macOS and Linux)
- **Nix**: Declarative package management with nix-darwin on macOS
- **System Packages**: Essential development tools per platform
- **Language Runtimes**: Node.js (NVM), Rust toolchain, Go environment
- **Pinned Herdr Runtime**: Install Herdr 0.8.2 from checksum-verified Linux/macOS release assets
  with `dotf install --intent dev`, then preserve any existing Herdr config while managing the
  repository config with `dotf link` (real files are backed up first)
- **Arch Desktop Runtime**: CachyOS/Arch installs can opt into the generic Hyprland desktop runtime
  with `bin/install-arch-pkgs.sh --hyprland`

### 🐚 Shell Configuration

- **Unified Prompt**: Starship provides consistent experience across bash and zsh
- **Shell Registration**: Adds modern shell paths to `/etc/shells`
- **Default Shell**: Intelligent shell switching with fallbacks
- **Cross-Platform Compatibility**: Unified configuration system
- **Fast Startup**: No framework overhead, 5x faster than Starship

### 🔗 Intelligent Symlink Management

- **`dotf link`**: Native Go symlink engine with backups and dry-run support
- **Conflict Resolution**: Handles existing files gracefully
- **Dry Run Support**: Test changes before applying
- **Validation**: Post-creation verification
- **Desktop Activation Gate**: Generic Linux desktop configs are only managed with `--linux-desktop`
  / `LINUX_DESKTOP=true`

### 🛠️ Development Environment

- **Editor Setup**: Neovim (nightly) with LazyVim, Lua 5.1/LuaJIT and LuaRocks for plugin support
- **Terminal Tools**: Herdr with a managed tmux-compatible config, TPM (Tmux Plugin Manager), and
  tmux-sessionizer
- **AI Coding Assistants**: OpenCode and Claude CLI for terminal-based AI pair programming
- **CLI Utilities**: Modern alternatives with fallback detection
- **Completions**: Universal completion system for 50+ tools
- **Security Tools**: 1Password CLI, GPG setup

## 🌟 Advanced Features

### Cross-Platform Shell System

The new common configuration system provides:

- **Unified Aliases**: Smart aliases that work across all platforms
- **Tool Detection**: Automatic fallback to system commands when modern tools unavailable
- **Path Management**: Intelligent PATH setup for all package managers
- **Environment Variables**: XDG-compliant environment setup
- **Function Library**: Cross-platform utility functions

### Modern Tool Integration

- **Enhanced ls**: eza with icons and colors, falls back to ls
- **Better cat**: bat with syntax highlighting, falls back to cat
- **Smart grep**: ripgrep with configuration, falls back to grep
- **Fast find**: fd for file searching, falls back to find
- **Fuzzy Finding**: fzf integration with smart defaults

### Developer Workflow Enhancements

- **Git Integration**: Delta diff viewer, enhanced git config
- **AI Coding Assistants**: OpenCode and Claude CLI for AI-powered development
- **Python Development**: Auto-virtualenv activation, poetry support
- **Cloud Development**: AWS/GCP/Azure CLI with completions
- **Container Workflows**: Docker/Podman with comprehensive completions
- **Kubernetes**: Full ecosystem support (kubectl, helm, k9s, istio)

## 📋 Manual Steps After Installation

1. **Restart Terminal**: `exec zsh` or `exec bash` to reload with Starship prompt
2. **Git Configuration**: Update personal information:

   ```bash
   git config --global user.name "Your Name"
   git config --global user.email "your.email@example.com"
   ```

3. **SSH Keys**: Add your SSH keys to `~/.ssh/` if needed
4. **Terminal Themes**: Import color schemes from `iterm2/` directory
5. **1Password**: Configure service account token if using 1Password CLI

### Optional: Enable the Hyprland desktop config set

**This set is the Hyprland/wlroots stack and is categorically wrong on a KDE Plasma host — including
Fedora KDE and Fedora Asahi Remix — so do not pass this flag there.**

For CachyOS or another Hyprland install, first install the runtime packages with
`./bin/install-arch-pkgs.sh --hyprland`, then preview the managed desktop config rollout:

```bash
dotf link --dry-run --hyprland
```

If the preview looks correct, apply it (existing real config dirs are backed up first):

```bash
dotf link --hyprland
```

`--linux-desktop` is retained as a deprecated alias for `--hyprland`; both flags are also accepted
by `dotf apply`, alongside the `HYPRLAND` and `LINUX_DESKTOP` environment twins.

This rollout manages `~/.config/hypr`, `~/.config/waybar`, and `~/.config/fuzzel`. It is opt-in and
replaces existing config directories after copying them into `~/.config/dotfiles_backup_<epoch>`;
preview with a dry run first.

## 🎨 Customization

### Platform-Specific Configurations

```bash
# Add macOS-specific aliases
echo 'alias myalias="my command"' >> common/macos/aliases.sh

# Add Linux-specific functions
echo 'myfunction() { echo "Linux only"; }' >> common/linux/functions.sh
```

### Tool Integration

```bash
# Add new tool to detection system
# Edit common/tools.sh and add to appropriate category

# Add completions for new tools
# Edit common/completions.sh
```

### Shell Customization

- **Aliases**: Edit `common/aliases.sh` for cross-platform aliases
- **Functions**: Add to `common/functions.sh` for shared functions
- **Environment**: Modify `common/exports.sh` for environment variables
- **PATH**: Update `common/paths.sh` for path modifications

## 🔧 Specialized Installer Scripts

Provisioning normally runs through `dotf apply`, which invokes these scripts for you via the
manifest's `custom-script` backend — they are governed adapters, not the primary install path. You
can also run any of them **directly** for a targeted or ad-hoc install. The `bin/` directory
includes dedicated installer scripts for cloud tools and modern CLI utilities that require special
installation methods. All scripts support:

- ✅ **Check Mode** (`--check`) - Verify installed versions
- ✅ **Dry Run** (`--dry-run`) - Preview changes without installing
- ✅ **Selective Installation** - Install specific tools only
- ✅ **Multi-Architecture** - Support for x86_64, aarch64, arm64
- ✅ **Official Sources** - Uses only official installation methods

### Available Installers

| Script                       | Tools                                               | Installation Method                   |
| ---------------------------- | --------------------------------------------------- | ------------------------------------- |
| `install-aws-tools.sh`       | AWS CLI v2, SAM CLI, ECS CLI                        | Official AWS installers               |
| `install-azure-cli.sh`       | Azure CLI                                           | Microsoft official repository         |
| `install-1password-cli.sh`   | 1Password CLI (`op`)                                | brew cask / GPG-verified zip / winget |
| `install-hashicorp-tools.sh` | Terraform, TFLint                                   | HashiCorp repository + GitHub         |
| `install-container-tools.sh` | Skaffold, Hadolint                                  | Official installers + GitHub          |
| `install-modern-cli.sh`      | lazygit, delta, bottom, eza, carapace, xh           | GitHub releases                       |
| `k8s-tools.sh`               | kubectl, helm, k9s, kubectx, kustomize, stern, krew | Official K8s ecosystem                |

### Quick Examples

```bash
# Install all AWS tools
./bin/install-aws-tools.sh

# Check what's installed
./bin/install-aws-tools.sh --check

# Preview without installing
./bin/install-modern-cli.sh --dry-run

# Install specific tools
./bin/install-hashicorp-tools.sh terraform
./bin/install-modern-cli.sh lazygit delta eza

# Custom installation directory
./bin/install-modern-cli.sh --dir ~/.local/bin
```

See [bin/INSTALLERS.md](bin/INSTALLERS.md) for complete documentation.

## 🔍 Key Files

### Core System

- `common/platform.sh`: Cross-platform detection engine
- `common/tools.sh`: Tool detection and setup system
- `common/completions.sh`: Universal completion system
- `bootstrap.sh`: POSIX shim that downloads `dotf` and execs `dotf apply`
- `cmd/dotf/`: Go provisioning CLI entry point

### Shell Configurations

- `bash/bashrc`: Modern bash with Starship prompt
- `zsh/zshrc`: Enhanced zsh with Starship prompt
- `~/.config/starship.toml`: Unified prompt configuration
- `bash/prompt.sh`: Git-aware bash prompt (fallback if Starship unavailable)

### Application Configs

- `nvim/`: LazyVim-based Neovim configuration
- `wezterm/`: Modular WezTerm configuration
- `aerospace/`: Tiling window manager (macOS)
- `plasma/`: Curated KDE Plasma 6 config set — `files/` snapshot plus the opt-in `recommended/`
  overlay

## 🧪 Testing

The configuration supports multiple testing modes:

```bash
# Test symlink creation (dry run)
dotf link --dry-run

# Test the KDE Plasma merge (dry run) and check convergence
dotf plasma apply --dry-run
dotf plasma diff

# Test platform detection
source common/platform.sh && echo "OS: $DOTFILES_OS, Arch: $DOTFILES_ARCH"

# Test tool detection
source common/tools.sh && system_health
```

## 🗺️ Roadmap

See [ROADMAP.md](ROADMAP.md) for what has shipped, what is planned, and what is explicitly deferred
(with reasons).

## 🤝 Contributing

1. Fork this repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Test across multiple platforms if possible
4. Commit your changes (`git commit -m 'Add amazing cross-platform feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [Starship](https://starship.rs/) - Fast, customizable cross-shell prompt
- [LazyVim](https://github.com/LazyVim/LazyVim) - Modern Neovim configuration
- [Homebrew](https://brew.sh/) - Cross-platform package manager
- [Modern CLI Tools](https://github.com/ibraheemdev/modern-unix) - Inspiration for tool alternatives
- [Catppuccin](https://github.com/catppuccin) - Soothing color theme

---

⚡ **Powered by cross-platform intelligence** - Works everywhere you do!
