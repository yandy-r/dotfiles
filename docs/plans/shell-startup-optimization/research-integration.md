# Integration Research: shell-startup-optimization

This report documents the complete shell startup pipeline, all tool integrations loaded at startup,
external services configured, and configuration sources. Current zsh startup is approximately 3.2
seconds; bash is approximately 0.45 seconds. The zsh configuration is the primary optimization
target.

## Shell Entry Points

### Zsh Loading Order (Primary Shell)

The zsh startup sequence proceeds through these stages. For login shells, `zprofile` runs first.

1. **`/home/yandy/.config/dotfiles/zsh/zprofile`** (login shells only)
   - Sources `common/platform.sh` and `common/paths.sh`
   - Calls `fix_homebrew_path_order()` to counteract macOS `path_helper` reordering
   - Guard: `ZPROFILE_LOADED`

2. **`/home/yandy/.config/dotfiles/zsh/zshrc`** (interactive shells)
   - Sets `DOTFILES_ROOT` to `~/.config/dotfiles`
   - Sources the 6 common modules in fixed order:
     - `common/platform.sh` (lines 16)
     - `common/exports.sh` (line 17)
     - `common/paths.sh` (line 18)
     - `common/aliases.sh` (line 19)
     - `common/functions.sh` (line 20)
     - `common/tools.sh` (line 21)
   - Sets zsh-specific options (vi mode, history, completion)
   - Adds Homebrew paths to `FPATH` (lines 77-83)
   - Configures completion styling (zstyle, lines 86-91)
   - Calls `compinit` and `bashcompinit` (lines 94-95) -- FIRST `compinit`
   - Sources legacy functions (`~/.functions.zsh`, line 102)
   - Loads vault, Azure CLI completions inline
   - Reads 1Password service account token via `$(cat ...)` subshell (line 124)
   - Runs `eval "$(op completion zsh)"` (line 129)
   - Reads GEMINI, OPENAI, ANTHROPIC API keys via `$(cat ...)` subshells (lines 136-148)
   - CDKTF yargs completion setup (line 154-163)
   - FZF integration via `fzf-share` and `~/.fzf.zsh` (lines 192-197)
   - `python_venv()` chpwd hook setup and initial run (lines 206-220)
   - `eval "$(thefuck --alias)"` (line 228)
   - Carapace universal completion with `source <(carapace _carapace)` (line 238)
   - Zsh-specific aliases with ~5 `command -v` checks (lines 248-295)
   - `eval "$(starship init zsh)"` (line 303)
   - `eval "$(direnv hook zsh)"` (line 320)
   - `eval "$(atuin init zsh)"` (line 329)
   - Sources `~/.zshrc.local` (line 341)
   - macOS-specific: Turso PATH, Docker completions fpath, Keychain unlock (lines 346-362)
   - PATH cleanup via grep/awk/sed pipelines (lines 374-406)
   - Calls `compinit` SECOND time (line 416) -- appended by rfmedia
   - pnpm PATH setup (lines 419-423)
   - Antigravity PATH (line 427)
   - Docker Desktop completions fpath + `compinit` THIRD time (lines 429-432)

### Bash Loading Order

3. **`/home/yandy/.config/dotfiles/bash/profile`** (login shells)
   - Sources `~/.bash_profile`
   - Guard: `DOTFILES_PROFILE_LOADED`

4. **`/home/yandy/.config/dotfiles/bash/bash_profile`** (login shells)
   - Sources `~/.bashrc`
   - Sources `~/.cargo/env` (duplicate -- also in bashrc)
   - `eval "$(direnv hook bash)"` (duplicate -- also in bashrc)
   - Guard: `DOTFILES_BASH_PROFILE_LOADED`

5. **`/home/yandy/.config/dotfiles/bash/bashrc`** (interactive shells)
   - Interactive-only check (line 9-12)
   - Re-entrant guard: `DOTFILES_BASHRC_LOADED` (exported, blocks subshells)
   - Sources the same 6 common modules as zsh
   - Bash-specific shopt settings and keybindings
   - `eval "$(starship init bash)"` or fallback to custom prompt (lines 88-108)
   - Bash completion sourcing (lines 115-126)
   - Legacy alias loading (lines 133-134)
   - Sources `~/.cargo/env` (duplicate -- also in bash_profile and tools.sh)
   - `eval "$(direnv hook bash)"` (duplicate -- also in bash_profile)
   - `eval "$(atuin init bash)"` (lines 170-175)
   - Sources `~/.bashrc.local` (line 182)
   - Terminal title setup via PROMPT_COMMAND (lines 185-190)

### Common Module Pipeline (Shared by Both Shells)

Both shells source these 6 modules in identical order:

| Order | Module                | Auto-Init              | Guard Variable                         | Key Cost                                                           |
| ----- | --------------------- | ---------------------- | -------------------------------------- | ------------------------------------------------------------------ |
| 1     | `common/platform.sh`  | `setup_platform_env()` | `DOTFILES_NO_AUTO_SETUP`               | 8+ subshell forks for detection                                    |
| 2     | `common/exports.sh`   | Immediate              | None                                   | `mkdir -p` x4, `$(command -v python3)`, `$(tty)`, `$(nproc)`       |
| 3     | `common/paths.sh`     | `setup_all_paths()`    | `DOTFILES_PATHS_LOADED` (not exported) | Complex PATH construction, `yarn global dir`, PATH dedup pipelines |
| 4     | `common/aliases.sh`   | Immediate              | None                                   | ~23 `has_command` checks                                           |
| 5     | `common/functions.sh` | Immediate              | None                                   | Minimal (defines functions, no execution)                          |
| 6     | `common/tools.sh`     | `setup_all_tools()`    | `DOTFILES_NO_AUTO_TOOL_SETUP`          | Orchestrates 12+ setup functions, each with eval/source calls      |

## Tool Integrations

### Tools Initialized via `eval "$(tool init ...)"` (Expensive)

These tools generate shell code on every startup. Each `eval` call forks a subprocess, generates the
init script, then evals it in the current shell. These are the highest-cost startup operations.

| Tool          | Init Command                                   | Location                                             | Estimated Cost | Notes                                                                        |
| ------------- | ---------------------------------------------- | ---------------------------------------------------- | -------------- | ---------------------------------------------------------------------------- |
| **Starship**  | `eval "$(starship init zsh/bash)"`             | zshrc:303, bashrc:88                                 | 30-80ms        | Prompt initialization; output changes rarely                                 |
| **Direnv**    | `eval "$(direnv hook zsh/bash)"`               | zshrc:320, bashrc:161                                | 10-30ms        | Directory hook; output is stable per shell version                           |
| **Atuin**     | `eval "$(atuin init zsh/bash)"`                | zshrc:329, bashrc:170                                | 20-50ms        | History search hook; output is stable                                        |
| **Zoxide**    | `eval "$(zoxide init zsh/bash)"`               | tools.sh:344-346                                     | 10-30ms        | Directory jumping; output is stable                                          |
| **mise**      | `eval "$(mise activate zsh/bash)"`             | tools.sh:460-462                                     | 20-60ms        | Version manager activation; output may change with config                    |
| **thefuck**   | `eval "$(thefuck --alias)"`                    | zshrc:228                                            | 200-500ms      | Python-based; extremely slow; output is stable                               |
| **Carapace**  | `source <(carapace _carapace)`                 | zshrc:238                                            | 30-80ms        | Universal completions; also loaded in completions.sh and lazy-completions.sh |
| **1Password** | `eval "$(op completion zsh)"`                  | zshrc:129                                            | 20-50ms        | Completion generation; rarely needed                                         |
| **FZF**       | `source <(fzf --zsh)` / `eval "$(fzf --bash)"` | zshrc:197, fzf.zsh:28, .fzf.bash:7, tools.sh:317-331 | 10-30ms        | Key bindings and completion                                                  |

### Tools Initialized via File Sourcing

| Tool                        | Source File                                                         | Location                                            | Notes                                    |
| --------------------------- | ------------------------------------------------------------------- | --------------------------------------------------- | ---------------------------------------- |
| **zsh-autosuggestions**     | System/Homebrew plugin path                                         | tools.sh:530-552                                    | Fish-like type-ahead suggestions         |
| **zsh-syntax-highlighting** | System/Homebrew plugin path                                         | tools.sh:557-574                                    | Command colorization                     |
| **Rust/Cargo**              | `~/.cargo/env`                                                      | bashrc:154, bash_profile:23, tools.sh:489           | Sourced 2-3 times                        |
| **Bash completion**         | `/usr/share/bash-completion/bash_completion` or Homebrew equivalent | bashrc:116-126, tools.sh:598-608                    | Can be expensive                         |
| **Google Cloud SDK**        | Homebrew share path completion files                                | completions.sh:206-215, lazy-completions.sh:179-193 | Loaded eagerly in completions.sh for zsh |

### Tools Using Lazy-Loading (Already Optimized)

These use the function-wrapper pattern from `common/lazy-completions.sh`:

| Tool             | Lazy Wrapper           | State Variable          |
| ---------------- | ---------------------- | ----------------------- |
| **kubectl**      | `kubectl()` / `k()`    | `LAZY_LOADED_KUBECTL`   |
| **Docker**       | `docker()`             | `LAZY_LOADED_DOCKER`    |
| **Helm**         | `helm()`               | `LAZY_LOADED_HELM`      |
| **Carapace**     | `carapace()`           | `LAZY_LOADED_CARAPACE`  |
| **AWS CLI**      | `aws()`                | `LAZY_LOADED_AWS`       |
| **Google Cloud** | `gcloud()`             | `LAZY_LOADED_GCLOUD`    |
| **Terraform**    | `terraform()` / `tf()` | `LAZY_LOADED_TERRAFORM` |

### Tools Detected via `has_command` / `command -v` at Startup

These are tools checked for existence during alias/function/completion setup. Each check is a PATH
lookup (fast individually, expensive in aggregate due to ~170 total checks).

**Modern CLI tools** (checked in aliases.sh and tools.sh): eza, bat, rg, fd, fzf, zoxide, delta,
tldr, htop, ncdu, lazygit, yazi, trash, glances, mise

**DevOps/Cloud** (checked in aliases.sh): op (1Password), kubectx, kubens, docker, vault, cdktf,
terraform, aws, gcloud, az

**Development** (checked in aliases.sh): code, subl, firefox, chrome, git, nvim, curl, wget, tree

**Platform utilities** (checked via `is_macos`/`is_linux`): Runs `detect_os()` -> `uname -s` on
every call (not cached in the helper functions)

### Zsh Completion System (`compinit`)

**Current State**: `compinit` is called THREE times in zshrc:

- Line 94: Intentional initialization before compdef calls
- Line 416: Appended by rfmedia tool installation
- Line 431: Appended by Docker Desktop

Each `compinit` call scans `$fpath` directories for completion functions and builds a cache
(`~/.zcompdump`). Cost: 100-300ms per call. Combined cost: 300-900ms.

**FPATH additions** (before compinit):

- Homebrew zsh site-functions
- Homebrew zsh-completions
- `$DOTFILES_ROOT/zsh/completions` (contains: `_tailscale`, `_op`, `_bws`, `_snitch`)
- `$HOME/.zfunc` (line 415)
- `$HOME/.docker/completions` (lines 353 and 429 -- duplicate)
- System zsh paths (via paths.sh `setup_fpath()`)

## External Services

### Secret/API Key Loading at Startup

These secrets are read from disk during zsh startup via `$(cat file)` subshell forks:

| Secret                  | File Path                                     | Variable                   | Location      |
| ----------------------- | --------------------------------------------- | -------------------------- | ------------- |
| 1Password Service Token | `~/.config/secrets/1password-service-account` | `OP_SERVICE_ACCOUNT_TOKEN` | zshrc:123-125 |
| Gemini API Key          | `~/.config/secrets/gemini_api_key`            | `GEMINI_API_KEY`           | zshrc:136-138 |
| OpenAI API Key          | `~/.config/secrets/openai_api_key`            | `OPENAI_API_KEY`           | zshrc:140-143 |
| Anthropic API Key       | `~/.config/secrets/anthropic_api_key`         | `ANTHROPIC_API_KEY`        | zshrc:145-148 |

**Note**: These only load in zsh, not bash. Each `$(cat file)` forks a subshell. Could use `read`
builtin or `<file` redirection instead.

### Atuin History Sync

- Config: `/home/yandy/.config/dotfiles/atuin/config.toml`
- Sync: `records = true` enables sync v2
- Dotfiles management: `enabled = true`
- No credentials in config (atuin manages its own auth)

### Direnv Environment Management

- Config: `/home/yandy/.config/dotfiles/dotenv/direnv.toml`
- Whitelisted paths: `~/.config/dotfiles`, `~/Projects` (both Linux and macOS variants)
- `hide_env_diff = true` -- suppresses verbose output
- `warn_timeout = "5m"` -- long timeout for slow operations

### SSH/GPG Integration

- `GPG_TTY=$(tty)` -- set in exports.sh at source time (forks subshell)
- `SSH_AUTH_SOCK` -- conditionally set from gpgconf output in exports.sh (forks subshell)
- macOS Keychain unlock on SSH login (zshrc:358-361)

## Configuration Sources

### Environment Variables Affecting Startup

**Control Variables** (disable/modify startup behavior):

| Variable                       | Purpose                               | Default              | Location                |
| ------------------------------ | ------------------------------------- | -------------------- | ----------------------- |
| `DOTFILES_ROOT`                | Base directory for dotfiles           | `~/.config/dotfiles` | bashrc:30, zshrc:13     |
| `DOTFILES_NO_AUTO_SETUP`       | Skip platform detection               | unset                | platform.sh:390         |
| `DOTFILES_NO_AUTO_PATH_SETUP`  | Skip PATH construction                | unset                | paths.sh:731            |
| `DOTFILES_NO_AUTO_TOOL_SETUP`  | Skip tool initialization              | unset                | tools.sh:778            |
| `DOTFILES_NO_AUTO_COMPLETIONS` | Skip completion loading               | unset                | completions.sh:612      |
| `DOTFILES_NO_LAZY_COMPLETIONS` | Skip lazy completion wrappers         | unset                | lazy-completions.sh:302 |
| `DOTFILES_DEBUG`               | Enable debug output                   | unset                | tools.sh, paths.sh      |
| `DOTFILES_CLEANUP_PATHS`       | Remove nonexistent PATH entries       | unset                | paths.sh:695            |
| `DOTFILES_BASHRC_LOADED`       | Bash re-entrant guard (exported)      | unset                | bashrc:18               |
| `DOTFILES_PATHS_LOADED`        | Paths re-entrant guard (not exported) | unset                | paths.sh:734            |

**Detected Variables** (set during platform detection in `platform.sh`):

| Variable                 | Example Value                                 | Detection Method                 |
| ------------------------ | --------------------------------------------- | -------------------------------- |
| `DOTFILES_OS`            | `debian`, `macos`, `arch`                     | `uname -s` + `/etc/os-release`   |
| `DOTFILES_ARCH`          | `x86_64`, `arm64`                             | `uname -m`                       |
| `DOTFILES_CONTAINER`     | `docker`, `kubernetes`, `wsl`, `none`         | `/proc/1/cgroup`, env vars       |
| `DOTFILES_PKG_MGR`       | `homebrew`, `apt`, `pacman`                   | `command -v` for each manager    |
| `DOTFILES_HOMEBREW_PATH` | `/opt/homebrew`, `/home/linuxbrew/.linuxbrew` | `brew --prefix` or hardcoded     |
| `DOTFILES_IS_SSH`        | `true`, `false`                               | `SSH_CLIENT`, `SSH_TTY` env vars |
| `DOTFILES_USE_OSC52`     | `true`, `false`                               | SSH + terminal capability        |
| `DOTFILES_COPY_CMD`      | `pbcopy`, `xclip -selection clipboard`        | OS detection                     |
| `DOTFILES_PASTE_CMD`     | `pbpaste`, `xclip -selection clipboard -o`    | OS detection                     |
| `DOTFILES_OPEN_CMD`      | `open`, `xdg-open`                            | OS detection                     |

### Configuration Files Read at Startup

| File                                                  | Read By                                   | Purpose                     |
| ----------------------------------------------------- | ----------------------------------------- | --------------------------- |
| `~/.config/dotfiles/common/platform.sh`               | bashrc, zshrc, zprofile                   | Platform detection          |
| `~/.config/dotfiles/common/exports.sh`                | bashrc, zshrc                             | Environment variables       |
| `~/.config/dotfiles/common/paths.sh`                  | bashrc, zshrc, zprofile                   | PATH construction           |
| `~/.config/dotfiles/common/aliases.sh`                | bashrc, zshrc                             | Alias definitions           |
| `~/.config/dotfiles/common/functions.sh`              | bashrc, zshrc                             | Function definitions        |
| `~/.config/dotfiles/common/tools.sh`                  | bashrc, zshrc                             | Tool initialization         |
| `~/.config/dotfiles/common/lazy-completions.sh`       | tools.sh                                  | Lazy completion wrappers    |
| `~/.config/dotfiles/common/completions.sh`            | tools.sh                                  | Eager completions           |
| `~/.config/dotfiles/common/$DOTFILES_OS/aliases.sh`   | aliases.sh                                | Platform-specific aliases   |
| `~/.config/dotfiles/common/$DOTFILES_OS/functions.sh` | functions.sh                              | Platform-specific functions |
| `/etc/os-release`                                     | platform.sh                               | Linux distro detection      |
| `~/.cargo/env`                                        | bashrc, bash_profile, tools.sh            | Rust environment            |
| `~/.fzf.zsh` / `~/.fzf.bash`                          | zshrc, tools.sh                           | FZF integration             |
| `~/.bashrc.local` / `~/.zshrc.local`                  | bashrc, zshrc                             | User customizations         |
| `~/.aliases` / `~/.bash_aliases`                      | bashrc                                    | Legacy alias files          |
| `~/.functions.zsh`                                    | zshrc                                     | Legacy zsh functions        |
| `~/.config/secrets/*`                                 | zshrc                                     | API keys (4 files)          |
| `~/.config/dotfiles/atuin/config.toml`                | atuin (at runtime)                        | Atuin configuration         |
| `~/.config/dotfiles/dotenv/direnv.toml`               | direnv (at runtime)                       | Direnv configuration        |
| `~/.config/dotfiles/starship.toml`                    | starship (at runtime)                     | Prompt configuration        |
| `~/.config/ripgrep/config`                            | ripgrep (at runtime, created by tools.sh) | Ripgrep config              |

### Platform Detection Flow

`platform.sh` runs `setup_platform_env()` automatically, which calls these detection functions (each
in a subshell via `$(...)`:

```
setup_platform_env()
  |-- $(detect_os)          -> uname -s, /etc/os-release
  |-- $(detect_arch)        -> uname -m
  |-- $(detect_container)   -> /proc/1/cgroup, env vars
  |-- $(detect_package_manager) -> command -v brew/apt/dnf/pacman/zypper
  |-- $(get_homebrew_path)  -> brew --prefix (SLOW: 50-100ms) or hardcoded
  |-- is_ssh()              -> check SSH_* env vars
  |-- should_use_osc52()    -> check TERM_PROGRAM, TERM
  |-- $(get_copy_command)   -> detect clipboard tool
  |-- $(get_paste_command)  -> detect clipboard tool
  |-- $(get_open_command)   -> detect file opener
  |-- mkdir -p for 4 XDG dirs
```

**Critical Performance Note**: The `is_macos()` and `is_linux()` convenience functions call
`$(detect_os)` on every invocation (spawning a new subshell and running `uname -s`) rather than
simply checking the already-cached `$DOTFILES_OS` variable. These functions are called dozens of
times across startup files.

## Startup Cost Inventory (Estimated)

### Zsh Startup Breakdown (Total: ~3200ms measured)

| Category                                      | Estimated Cost | Details                                                         |
| --------------------------------------------- | -------------- | --------------------------------------------------------------- |
| `compinit` x3                                 | 300-900ms      | Lines 94, 416, 431 of zshrc                                     |
| `eval "$(thefuck --alias)"`                   | 200-500ms      | Python startup overhead                                         |
| `eval "$(starship init zsh)"`                 | 30-80ms        | Prompt setup                                                    |
| `eval "$(atuin init zsh)"`                    | 20-50ms        | History hooks                                                   |
| `eval "$(mise activate zsh)"`                 | 20-60ms        | Version manager                                                 |
| `eval "$(zoxide init zsh)"`                   | 10-30ms        | Directory jumping                                               |
| `eval "$(direnv hook zsh)"`                   | 10-30ms        | Environment hooks                                               |
| `eval "$(op completion zsh)"`                 | 20-50ms        | 1Password completions                                           |
| `source <(carapace _carapace)`                | 30-80ms        | Universal completions (called in both zshrc AND completions.sh) |
| `source <(fzf --zsh)`                         | 10-30ms        | FZF integration                                                 |
| Platform detection subshells                  | 100-200ms      | 10+ subshell forks in `setup_platform_env()`                    |
| `brew --prefix` (macOS)                       | 50-100ms       | Called in `get_homebrew_path()`                                 |
| PATH cleanup pipelines                        | 50-100ms       | grep/awk/sed in zshrc lines 374-406                             |
| `has_command` checks (170+)                   | 50-100ms       | PATH lookups across all modules                                 |
| Secret file reads (4x `cat`)                  | 20-40ms        | Subshell forks for API keys                                     |
| `$(tty)`, `$(nproc)`, `$(command -v python3)` | 15-30ms        | Subshells in exports.sh                                         |
| zsh-autosuggestions + syntax-highlighting     | 20-50ms        | Plugin file sourcing                                            |
| Completions (eagerly loaded in zsh)           | 100-300ms      | 51 `command -v` checks + completion generation                  |
| `setup_all_paths()`                           | 50-150ms       | PATH construction with is_macos/is_linux calls                  |
| Remaining overhead                            | 100-300ms      | File I/O, alias/function definitions, misc                      |

### Bash Startup Breakdown (Total: ~450ms measured)

| Category                        | Estimated Cost | Details                      |
| ------------------------------- | -------------- | ---------------------------- |
| `eval "$(starship init bash)"`  | 30-80ms        | Prompt setup                 |
| `eval "$(atuin init bash)"`     | 20-50ms        | History hooks                |
| `eval "$(direnv hook bash)"`    | 10-30ms        | Environment hooks            |
| Platform detection subshells    | 100-200ms      | Same as zsh                  |
| Common module loading (6 files) | 50-100ms       | File sourcing and evaluation |
| Remaining overhead              | 50-100ms       | Bash completion, misc        |

**Key Insight**: Bash is faster primarily because it skips `compinit`, `thefuck`, carapace,
1Password completions, secret file reads, PATH cleanup pipelines, and most eager completions
(returns early in completions.sh).

## Duplicate/Redundant Operations

| Operation             | Locations                                                          | Impact                                                               |
| --------------------- | ------------------------------------------------------------------ | -------------------------------------------------------------------- |
| `compinit`            | zshrc:94, zshrc:416, zshrc:431, tools.sh:587                       | 3 redundant calls (only 1 needed)                                    |
| `~/.cargo/env` source | bashrc:154, bash_profile:23, tools.sh:489                          | Sourced 2-3 times per session                                        |
| `mkdir -p` XDG dirs   | platform.sh:313, exports.sh:49                                     | Same 4 dirs created twice                                            |
| `direnv hook`         | bash_profile:28, bashrc:161                                        | Duplicate in bash login shells                                       |
| Docker fpath          | zshrc:353, zshrc:429                                               | Same directory added twice                                           |
| PATH cleanup          | zshrc:374-406                                                      | Duplicates cleanup_path() and cleanup_unwanted_paths() from paths.sh |
| Carapace init         | zshrc:235-239, completions.sh:546-557, lazy-completions.sh:122-148 | Three different loading paths                                        |
| FZF setup             | zshrc:192-197, tools.sh:312-332, fzf.zsh                           | Multiple loading paths                                               |
| Homebrew FPATH        | zshrc:77-80, paths.sh `setup_fpath()`, completions.sh:496-515      | Added multiple times                                                 |

## Relevant Files

- `/home/yandy/.config/dotfiles/zsh/zshrc`: Main zsh entry point (432 lines). Biggest optimization
  target.
- `/home/yandy/.config/dotfiles/zsh/zprofile`: Zsh login shell profile. Sources platform.sh and
  paths.sh for PATH ordering.
- `/home/yandy/.config/dotfiles/bash/bashrc`: Main bash entry point (196 lines). Already reasonably
  fast.
- `/home/yandy/.config/dotfiles/bash/bash_profile`: Bash login profile. Sources bashrc + duplicates
  cargo/direnv.
- `/home/yandy/.config/dotfiles/bash/profile`: Profile loader. Sources bash_profile.
- `/home/yandy/.config/dotfiles/common/platform.sh`: Platform detection. 392 lines. Spawns 8+
  subshells during init.
- `/home/yandy/.config/dotfiles/common/exports.sh`: Environment variables. 482 lines. Multiple
  subshell calls.
- `/home/yandy/.config/dotfiles/common/paths.sh`: PATH management. 744 lines. Complex construction
  logic.
- `/home/yandy/.config/dotfiles/common/aliases.sh`: Aliases. 689 lines. 23 has_command checks.
- `/home/yandy/.config/dotfiles/common/functions.sh`: Functions. 703 lines. Low startup cost
  (definitions only).
- `/home/yandy/.config/dotfiles/common/tools.sh`: Tool orchestrator. 780 lines. Calls 12+ setup
  functions.
- `/home/yandy/.config/dotfiles/common/lazy-completions.sh`: Lazy completion wrappers. 305 lines.
  Already optimized.
- `/home/yandy/.config/dotfiles/common/completions.sh`: Eager completions. 614 lines. Heavy for zsh.
- `/home/yandy/.config/dotfiles/common/linux/aliases.sh`: Linux aliases. 496 lines.
- `/home/yandy/.config/dotfiles/common/macos/aliases.sh`: macOS aliases. 219 lines.
- `/home/yandy/.config/dotfiles/common/macos/functions.sh`: macOS functions. 525 lines.
- `/home/yandy/.config/dotfiles/zsh/functions.zsh`: Legacy zsh functions. 300 lines.
- `/home/yandy/.config/dotfiles/zsh/fzf.zsh`: FZF zsh setup. 30 lines.
- `/home/yandy/.config/dotfiles/bash/.fzf.bash`: FZF bash setup. 7 lines.
- `/home/yandy/.config/dotfiles/bash/prompt.sh`: Custom bash prompt fallback. 287 lines.
- `/home/yandy/.config/dotfiles/atuin/config.toml`: Atuin configuration.
- `/home/yandy/.config/dotfiles/dotenv/direnv.toml`: Direnv configuration.
- `/home/yandy/.config/dotfiles/starship.toml`: Starship prompt configuration.

## Other Docs

- `/home/yandy/.config/dotfiles/docs/plans/shell-startup-optimization/research-patterns.md` --
  Codebase patterns research with edge cases
- `/home/yandy/.config/dotfiles/docs/plans/shell-startup-optimization/research-docs.md` --
  Documentation inventory and gaps analysis
- `/home/yandy/.config/dotfiles/docs/architecture/overview.md` -- System architecture with
  initialization flow diagrams
- `/home/yandy/.config/dotfiles/docs/architecture/common-system.md` -- Module dependency graph and
  API reference
- `/home/yandy/.config/dotfiles/docs/features/completions.md` -- Completion system design with lazy
  vs eager strategy
- `/home/yandy/.config/dotfiles/docs/features/shell-configuration.md` -- Shell setup guide with
  slow-startup troubleshooting
- `/home/yandy/.config/dotfiles/docs/features/modern-tools.md` -- Modern CLI tools integration guide
- https://mise.jdx.dev/ -- mise version manager documentation
- https://github.com/rsteube/carapace -- Carapace universal completion engine
- https://starship.rs/ -- Starship prompt documentation
- https://docs.atuin.sh/configuration/config -- Atuin configuration reference
- https://direnv.net/ -- Direnv documentation
