# Architecture Research: shell-startup-optimization

## System Overview

The dotfiles shell startup system is a cross-platform configuration framework that loads 6 common
modules sequentially (platform, exports, paths, aliases, functions, tools) from both bash and zsh
entry points. The zsh startup path is the primary performance concern, with `zprof` data showing
**86.46% of total time** consumed by 3 redundant `compinit` calls and **2009 `compdef`
invocations**. The architecture already includes a lazy-completion system for heavy tools (kubectl,
docker, helm), but significant optimization opportunities remain in compinit deduplication,
`is_macos`/`is_linux` subshell elimination, duplicate `eval "$(...)"` calls, and redundant
`cargo/env` sourcing.

## Relevant Components

### Entry Points (Login/Interactive Shell Startup)

- `/home/yandy/.config/dotfiles/zsh/zprofile`: Login shell -- sources platform.sh and paths.sh,
  calls `fix_homebrew_path_order`. Runs BEFORE zshrc on login shells, duplicating work.
- `/home/yandy/.config/dotfiles/zsh/zshrc`: Primary zsh config (432 lines). Sources 6 common
  modules, then configures zsh-specific options, completions, tool inits, aliases, and PATH cleanup.
- `/home/yandy/.config/dotfiles/bash/profile`: Bash login redirector -- sources bash_profile.
- `/home/yandy/.config/dotfiles/bash/bash_profile`: Bash login shell (30 lines). Sources bashrc,
  cargo/env (duplicate), and direnv hook (duplicate).
- `/home/yandy/.config/dotfiles/bash/bashrc`: Primary bash config (195 lines). Sources same 6 common
  modules, then configures bash-specific features.

### Common Modules (Sourced Sequentially by Both Shells)

- `/home/yandy/.config/dotfiles/common/platform.sh` (391 lines): Platform detection engine. Detects
  OS, arch, container, package manager. Exports `DOTFILES_OS`, `DOTFILES_ARCH`, etc. Runs
  `setup_platform_env()` on source. **Critical issue**: `is_macos()` and `is_linux()` call
  `detect_os()` which runs `uname -s` in a subshell every invocation -- these are called 36 times
  across common/ modules.
- `/home/yandy/.config/dotfiles/common/exports.sh` (481 lines): Environment variables for editor,
  pager, history, dev tools, cloud providers, telemetry opt-outs, and FZF. Runs `mkdir -p` for XDG
  dirs (duplicated in platform.sh). Calls `cleanup_path_var` for 4 variables.
- `/home/yandy/.config/dotfiles/common/paths.sh` (743 lines): PATH construction with platform-aware
  ordering. Contains `setup_all_paths()` which calls 10+ sub-functions. Has dedup/cleanup logic.
  Runs `setup_rust_env()` which sources `cargo/env` (also sourced later by tools.sh).
- `/home/yandy/.config/dotfiles/common/aliases.sh` (688 lines): Shell aliases for navigation, file
  ops, git, docker, k8s, cloud tools. Many `has_command` checks (~7 `is_macos`/`is_linux` calls).
- `/home/yandy/.config/dotfiles/common/functions.sh` (702 lines): Utility functions for file ops,
  networking, git, system. Uses ~8 `is_macos`/`is_linux` checks.
- `/home/yandy/.config/dotfiles/common/tools.sh` (779 lines): Tool detection, setup, and
  initialization. Contains `setup_all_tools()` which calls: `setup_mise`, `setup_rust` (cargo/env
  again), `setup_modern_alternatives`, `setup_fzf`, `setup_zoxide`, `setup_atuin`, `setup_lazygit`,
  `setup_yazi`, `setup_direnv`, `setup_autosuggestions`, `setup_syntax_highlighting`,
  `setup_python_venv`, then sources lazy-completions.sh and completions.sh.

### Completion System

- `/home/yandy/.config/dotfiles/common/lazy-completions.sh` (304 lines): Deferred completion loading
  for kubectl, docker, helm, carapace, aws, gcloud, terraform. Uses wrapper functions that
  self-replace on first invocation.
- `/home/yandy/.config/dotfiles/common/completions.sh` (613 lines): Eager completion loading for
  git, languages, cloud providers, modern CLI tools. In bash, it returns early (line 578) deferring
  to lazy loading. In zsh, it loads everything eagerly including 51 `command -v` checks.

### Platform-Specific Extensions

- `/home/yandy/.config/dotfiles/common/macos/aliases.sh` (8.0K): macOS-specific aliases.
- `/home/yandy/.config/dotfiles/common/macos/functions.sh` (15K): macOS-specific functions
  (Spotlight, Keychain, WiFi, etc.).
- `/home/yandy/.config/dotfiles/common/linux/aliases.sh` (16K): Linux-specific aliases (systemd,
  package managers, etc.).

### Zsh-Specific

- `/home/yandy/.config/dotfiles/zsh/functions.zsh` (7.2K): Legacy zsh functions (loaded
  conditionally from `$HOME/.functions.zsh`).
- `/home/yandy/.config/dotfiles/zsh/fzf.zsh` (953 bytes): FZF zsh integration.
- `/home/yandy/.config/dotfiles/zsh/completions/`: Custom completion scripts directory.

## Data Flow

### Zsh Startup Sequence (Login Shell)

```
/etc/zprofile (macOS path_helper runs here)
  -> zsh/zprofile
     -> common/platform.sh      (1st load - sets DOTFILES_OS, etc.)
     -> common/paths.sh         (1st load - calls setup_all_paths())
     -> fix_homebrew_path_order()
  -> zsh/zshrc
     -> common/platform.sh      (2nd load - re-detects everything since no guard)
     -> common/exports.sh       (mkdir -p XDG dirs again)
     -> common/paths.sh         (2nd load - DOTFILES_PATHS_LOADED guard prevents re-run)
     -> common/aliases.sh       (many command -v and is_macos checks)
     -> common/functions.sh     (many command -v and is_macos checks)
     -> common/tools.sh         (setup_all_tools triggers chain)
        -> setup_mise            (eval "$(mise activate zsh)")
        -> setup_rust            (source cargo/env - 3rd time)
        -> setup_modern_alternatives (command -v checks, mkdir -p, git config calls)
        -> setup_fzf             (source fzf files)
        -> setup_zoxide          (eval "$(zoxide init zsh)")
        -> setup_autosuggestions (source zsh-autosuggestions)
        -> setup_syntax_highlighting (source zsh-syntax-highlighting)
        -> setup_python_venv     (add-zsh-hook, python_venv run)
        -> lazy-completions.sh   (define wrapper functions)
        -> completions.sh        (setup_all_completions for zsh - EAGER)
           -> setup_homebrew_completions (FPATH manipulation)
           -> setup_git_completions
           -> setup_language_completions (many command -v checks)
           -> setup_aws/gcp/azure_completions
           -> setup_kubernetes_completions
           -> setup_container_completions
           -> setup_terraform_completions
           -> setup_modern_tools_completions
           -> setup_1password_completions
           -> setup_carapace_completions
     -> compinit (1st call - line 94)
     -> bashcompinit (line 95)
     -> [zshrc inline: op completion, thefuck, carapace, starship, direnv, atuin evals]
     -> PATH cleanup section (tr/grep/awk pipeline for dedup + platform cleanup)
     -> fix_homebrew_path_order() (2nd call)
     -> compinit (2nd call - line 416, rfmedia)
     -> compinit (3rd call - line 431, Docker Desktop)
```

### Bash Startup Sequence (Login Shell)

```
bash/profile -> bash/bash_profile -> bash/bashrc
  -> common/platform.sh
  -> common/exports.sh
  -> common/paths.sh
  -> common/aliases.sh
  -> common/functions.sh
  -> common/tools.sh
     -> [same tool chain but bash paths]
     -> completions.sh (returns early for bash at line 578)
  -> bash completion system
  -> starship init bash (eval)
  -> direnv hook bash (eval - also in bash_profile = duplicate)
  -> atuin init bash (eval)
  -> cargo/env (also sourced by paths.sh and tools.sh)
```

### Key Observation: Bash is Already Fast

Bash startup measured at **0.026s** vs zsh at **0.142s**. The early return in `completions.sh` for
bash (line 578) is the main reason. Almost all optimization effort should focus on zsh.

## Integration Points

### Where Optimization Code Should Connect

1. **compinit consolidation**: The zshrc file at `/home/yandy/.config/dotfiles/zsh/zshrc` calls
   `compinit` three times (lines 94, 416, 431). `tools.sh` also calls it once via
   `setup_completions()` (line 587). These should be consolidated to a single call, ideally with
   caching (`compinit -C` to skip security check, or use a dump file with `zcompdump` age check).

2. **Platform detection caching**: `/home/yandy/.config/dotfiles/common/platform.sh` lines 337-361
   define `is_macos()`, `is_linux()`, `is_wsl()` etc. which call `detect_os()` spawning `uname -s`
   subshells. These should be rewritten to use the already-exported `$DOTFILES_OS` variable directly
   instead of calling detection functions.

3. **eval caching**: The expensive `eval "$(...)"` calls in zshrc (starship, direnv, atuin, thefuck,
   op, zoxide, mise) at lines 129, 228, 303, 320, 329, and in tools.sh lines 344, 460 should be
   cached to files and sourced from cache when the tool version has not changed.

4. **Lazy loading expansion**: `/home/yandy/.config/dotfiles/common/lazy-completions.sh` provides
   the pattern. The same approach could be applied to: starship, direnv, atuin, thefuck, and
   carapace init in zshrc.

5. **PATH cleanup optimization**: The PATH cleanup section in zshrc (lines 370-406) uses expensive
   `tr | grep | awk | sed` pipelines. This could be replaced with pure shell parameter expansion.

6. **cargo/env deduplication**: `source "$HOME/.cargo/env"` appears in: `paths.sh:527`,
   `tools.sh:490`, `completions.sh:417`, and `bash_profile:24`. It should be sourced exactly once
   with a guard variable.

7. **XDG mkdir deduplication**: `mkdir -p` for XDG directories runs in both `platform.sh:313` and
   `exports.sh:49`. Should be consolidated to one location.

8. **Completion system restructure**: The interaction between
   `tools.sh -> lazy-completions.sh -> completions.sh` (called from within `setup_all_tools()`) and
   then the inline completions in zshrc creates confusion and duplication. The carapace setup
   appears in both `lazy-completions.sh` and `completions.sh`, and zshrc line 238 also sources
   carapace.

## Key Dependencies

### External Tools Invoked During Startup (eval calls)

Each `eval "$(...)"` spawns a subprocess and parses its output:

| Tool     | Call                           | Location                      | Estimated Cost      |
| -------- | ------------------------------ | ----------------------------- | ------------------- |
| starship | `eval "$(starship init zsh)"`  | zshrc:303                     | ~15-30ms            |
| direnv   | `eval "$(direnv hook zsh)"`    | zshrc:320                     | ~5-10ms             |
| atuin    | `eval "$(atuin init zsh)"`     | zshrc:329                     | ~5-10ms             |
| thefuck  | `eval "$(thefuck --alias)"`    | zshrc:228                     | ~100-300ms (Python) |
| op       | `eval "$(op completion zsh)"`  | zshrc:129                     | ~10-20ms            |
| zoxide   | `eval "$(zoxide init zsh)"`    | tools.sh:344                  | ~5-10ms             |
| mise     | `eval "$(mise activate zsh)"`  | tools.sh:460                  | ~5-10ms             |
| carapace | `source <(carapace _carapace)` | zshrc:238, completions.sh:552 | ~10-20ms            |

### Internal Module Dependencies

```
platform.sh   <- standalone (no deps)
exports.sh    <- platform.sh
paths.sh      <- platform.sh
aliases.sh    <- platform.sh
functions.sh  <- platform.sh
tools.sh      <- platform.sh, (lazy-completions.sh, completions.sh loaded from within)
```

### Guard Variables

- `DOTFILES_BASHRC_LOADED` - Prevents double-loading of bashrc
- `DOTFILES_BASH_PROFILE_LOADED` - Prevents circular bash_profile loading
- `DOTFILES_PROFILE_LOADED` - Prevents circular profile loading
- `DOTFILES_PATHS_LOADED` - Prevents re-running `setup_all_paths()` (NOT exported)
- `DOTFILES_NO_AUTO_SETUP` - Disables platform.sh auto-setup
- `DOTFILES_NO_AUTO_PATH_SETUP` - Disables paths.sh auto-setup
- `DOTFILES_NO_AUTO_TOOL_SETUP` - Disables tools.sh auto-setup
- `DOTFILES_NO_AUTO_COMPLETIONS` - Disables completions.sh auto-setup
- `DOTFILES_NO_LAZY_COMPLETIONS` - Disables lazy-completions.sh auto-setup
- `ZPROFILE_LOADED` - Prevents re-running zprofile

## Architectural Patterns

- **Sequential Module Loading**: Both shells source the same 6 modules in the same order, providing
  feature parity but with no parallelism or conditional skipping.
- **Auto-Init on Source**: Every common module has an "INITIALIZATION" section at the bottom that
  runs automatically when sourced, controlled by opt-out environment variables
  (`DOTFILES_NO_AUTO_*`).
- **Platform Detection via Function Calls**: `is_macos()`, `is_linux()`, etc. call `detect_os()`
  which spawns a `uname -s` subshell each time, rather than checking the already-exported
  `$DOTFILES_OS`.
- **Lazy Loading for Heavy Completions**: `lazy-completions.sh` wraps kubectl/docker/helm/etc. in
  self-replacing functions -- a good pattern that could be extended to tool init (starship, direnv,
  atuin).
- **Eager Loading for Zsh Completions**: `completions.sh` loads everything eagerly for zsh (line
  575-603) while bash returns early (line 578). This causes 51+ `command -v` checks during zsh
  startup.
- **Redundant compinit**: zshrc calls `compinit` at lines 94, 416, and 431 (plus once from
  tools.sh). Each call takes ~650ms with 2009 compdef registrations.

## Edgecases

- **compinit is called 3 times in zshrc**: Lines 94, 416, and 431. The last two were appended by
  rfmedia and Docker Desktop installers. Each invocation triggers a full completion dump taking
  ~650ms. The total compinit cost is **1960ms (86% of startup)**.
- **zprofile double-loads platform.sh and paths.sh**: On login shells (tmux, SSH), zprofile sources
  platform.sh and paths.sh before zshrc does the same, but platform.sh lacks a load guard so
  `setup_platform_env()` runs twice.
- **is_macos/is_linux spawn subshells on every call**: These convenience functions
  (platform.sh:337-347) call `detect_os()` which runs `uname -s` in a command substitution. They are
  called 36 times across common modules during startup. Each should use `$DOTFILES_OS` directly.
- **cargo/env sourced up to 4 times**: paths.sh, tools.sh, completions.sh, and bash_profile all
  source it independently with no shared guard.
- **direnv hook registered twice in bash**: Both bash_profile (line 29) and bashrc (line 162) call
  `eval "$(direnv hook bash)"`.
- **thefuck --alias is extremely slow**: This Python tool takes 100-300ms for its eval. It is only
  conditionally loaded but should be lazy-loaded or cached.
- **PATH cleanup uses expensive shell pipelines**: zshrc lines 374-389 use `tr | grep | awk | sed`
  for PATH deduplication, which could be done with pure zsh parameter expansion.
- **brew --prefix called during detection**: `get_homebrew_path()` in platform.sh calls
  `brew --prefix` which can take 100-200ms if brew is slow. This happens during
  `setup_platform_env()`.
- **carapace initialized in 3 locations**: lazy-completions.sh, completions.sh, and zshrc line
  235-239 all set up carapace, with potential triple-initialization.
- **python_venv defined twice in zsh**: Once in tools.sh via `setup_python_venv()` (line 500) and
  again inline in zshrc (line 206). Both register chpwd hooks.

## Profiling Data (zsh, zprof output)

Top consumers by self-time:

| Rank | Function                          | Self Time (ms) | % of Total | Calls |
| ---- | --------------------------------- | -------------- | ---------- | ----- |
| 1    | compdump                          | 698.66         | 30.81%     | 2     |
| 2    | compdef                           | 665.52         | 29.35%     | 2009  |
| 3    | compinit                          | 538.28         | 23.74%     | 3     |
| 4    | compaudit                         | 97.22          | 4.29%      | 6     |
| 5    | setup_platform_env                | 51.04          | 2.25%      | 1     |
| 6    | is_macos                          | 47.22          | 2.08%      | 11    |
| 7    | remove_from_path                  | 32.28          | 1.42%      | 8     |
| 8    | is_linux                          | 24.05          | 1.06%      | 6     |
| 9    | setup_modern_alternatives         | 20.34          | 0.90%      | 1     |
| 10   | \_zsh_highlight_load_highlighters | 9.79           | 0.43%      | 1     |

**Total measured startup: ~2268ms** (though wall-clock was 142ms due to caching and parallel kernel
scheduling).

## Quick-Win Optimization Opportunities (Prioritized)

1. **Consolidate compinit to 1 call with caching** (~86% savings potential): Replace 3 compinit
   calls with a single `compinit -C` that uses a cached zcompdump file, checked once per day.
2. **Rewrite is_macos/is_linux to use $DOTFILES_OS** (~3% savings): Replace
   `[[ "$(detect_os)" == "macos" ]]` with `[[ "$DOTFILES_OS" == "macos" ]]` across all files.
3. **Cache eval output for starship/direnv/atuin/zoxide/mise**: Generate cache files and source
   them, regenerating when tool version changes.
4. **Lazy-load or remove thefuck --alias**: Either use the lazy-loading pattern from
   lazy-completions.sh or cache the output.
5. **Add load guard to platform.sh**: Prevent double setup_platform_env() when both zprofile and
   zshrc source it.
6. **Deduplicate cargo/env sourcing**: Add a guard variable, source once.
7. **Remove duplicate carapace initialization**: Pick one location (lazy-completions.sh) and remove
   from completions.sh and zshrc.
8. **Replace PATH cleanup pipelines with zsh builtins**: Use `typeset -U path` for dedup and zsh
   array operations.

## Other Docs

- `/home/yandy/.config/dotfiles/common/README.md`: Architecture overview of the common configuration
  system.
- `/home/yandy/.config/dotfiles/zsh/README.md`: Zsh-specific configuration documentation.
- `/home/yandy/.config/dotfiles/bash/README.md`: Bash-specific configuration documentation.
- `/home/yandy/.config/dotfiles/zsh/omz-migration-notes.md`: Notes from migrating away from Oh My
  Zsh.
- `/home/yandy/.config/dotfiles/docs/architecture/`: Existing architecture documentation directory.
