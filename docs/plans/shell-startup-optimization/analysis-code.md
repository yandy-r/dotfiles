# Code Analysis: shell-startup-optimization

## Executive Summary

The shell startup system uses a fixed 6-module sequential pipeline (platform -> exports -> paths ->
aliases -> functions -> tools) sourced identically by both `zsh/zshrc` and `bash/bashrc`. The
primary optimization target is zsh (~3.2s startup) where `compinit` is called 3 times, 7
`eval "$(tool init ...)"` calls occur, ~170 `has_command`/`command -v` lookups execute, and 8+
subshell forks happen in `setup_platform_env()`. The existing lazy-completion wrapper pattern in
`lazy-completions.sh` provides the proven template for extending lazy loading to tool initialization
(eval caching), while the `DOTFILES_*_LOADED` guard variable convention prevents re-entrant
execution.

## Existing Code Structure

### Related Components

- `/home/yandy/.config/dotfiles/common/platform.sh`: Platform detection engine; foundational module
  sourced first by all shells. Contains `setup_platform_env()` which spawns 8+ subshells via
  `$(detect_*)` calls, and convenience functions `is_macos()`/`is_linux()` that re-detect OS on
  every invocation instead of using cached `$DOTFILES_OS`.
- `/home/yandy/.config/dotfiles/common/exports.sh`: Environment variable definitions; runs
  `mkdir -p` for XDG dirs (duplicated from platform.sh), plus `$(command -v python3)`, `$(tty)`,
  `$(nproc)` subshells at source time.
- `/home/yandy/.config/dotfiles/common/paths.sh`: PATH construction with 743 lines; multiple
  `is_macos`/`is_linux` calls spawning subshells, a slow `yarn global dir` call, and PATH dedup
  pipelines using `tr`/`grep`/`awk`/`sed`. Has `DOTFILES_PATHS_LOADED` re-entrant guard.
- `/home/yandy/.config/dotfiles/common/aliases.sh`: 688 lines of aliases with ~23 `has_command`
  checks and 7 `is_macos`/`is_linux` calls at source time, plus loads platform-specific alias files.
- `/home/yandy/.config/dotfiles/common/functions.sh`: Utility functions; low startup cost since it
  only defines functions. Contains ~8 `is_macos`/`is_linux` convenience calls inside function bodies
  (not at source time).
- `/home/yandy/.config/dotfiles/common/tools.sh`: Tool setup orchestrator; `setup_all_tools()`
  triggers 12+ `setup_*()` functions, each with `command -v` checks and `eval` calls. Loads
  `lazy-completions.sh` and `completions.sh`.
- `/home/yandy/.config/dotfiles/common/lazy-completions.sh`: Reference implementation for lazy
  loading via function-wrapper pattern; wraps kubectl, docker, helm, carapace, aws, gcloud,
  terraform with state tracking variables.
- `/home/yandy/.config/dotfiles/common/completions.sh`: Eager completion loading for zsh (51+
  `command -v` checks); bash exits early at line 578 since heavy completions are lazy-loaded.
- `/home/yandy/.config/dotfiles/zsh/zshrc`: Primary optimization target (432 lines); triple
  `compinit` (lines 94, 416, 431), 7 `eval` calls (starship, direnv, atuin, op, thefuck, carapace,
  mise), PATH cleanup pipelines, secret file reads via `$(cat)`, appended third-party blocks.
- `/home/yandy/.config/dotfiles/zsh/zprofile`: Login shell profile; sources `platform.sh` and
  `paths.sh` before zshrc duplicates the same work.
- `/home/yandy/.config/dotfiles/bash/bashrc`: Bash entry point (195 lines); already fast (~0.45s),
  shares same 6-module pipeline, has `DOTFILES_BASHRC_LOADED` guard.
- `/home/yandy/.config/dotfiles/bash/bash_profile`: Bash login shell; duplicates `cargo/env`
  sourcing and `direnv hook` from bashrc.
- `/home/yandy/.config/dotfiles/common/linux/aliases.sh`: Linux-specific aliases with ~20
  `has_command` checks for tools like yay, paru, flatpak, snap, nmcli, iwconfig, xclip, xsel, ufw,
  iptables, podman, gdb, valgrind, openssl, gpg, htop, iotop, nethogs, ncdu.
- `/home/yandy/.config/dotfiles/common/macos/aliases.sh`: macOS-specific aliases; no `has_command`
  checks (all are static macOS commands).

### File Organization Pattern

All common/shared shell configurations live in `common/` with a flat structure for the 6 core
modules, plus `macos/` and `linux/` subdirectories for platform-specific extensions. Each module
follows a consistent pattern: header comment block, dependency check/load, categorized sections with
section header comments, a main `setup_*()` orchestrator function, and an auto-initialization block
guarded by `DOTFILES_NO_AUTO_*` environment variables.

## Implementation Patterns

### Pattern: Auto-Init with Guard Variables

**Description**: Each module auto-initializes when sourced, controlled by `DOTFILES_NO_AUTO_*`
disable flags and `DOTFILES_*_LOADED` re-entrant guards. This ensures idempotent loading and allows
disabling modules for testing or profiling. **Example**: See
`/home/yandy/.config/dotfiles/common/paths.sh` lines 731-743

```bash
if [[ -z "${DOTFILES_NO_AUTO_PATH_SETUP}" && -z "${DOTFILES_PATHS_LOADED}" ]]; then
    DOTFILES_PATHS_LOADED=1
    # ... minimal PATH safety check ...
    setup_all_paths
fi
```

**Apply to**: Any new optimization modules (e.g., `common/lazy-init.sh`, profiling infrastructure)
must follow this convention. Use `DOTFILES_NO_AUTO_LAZY_INIT` as disable flag and
`DOTFILES_LAZY_INIT_LOADED` as guard.

### Pattern: Lazy Completion via Function Wrapper

**Description**: Wrapper functions that replace the real command, unset themselves on first
invocation, load completions, set a state variable, then delegate to the real binary via
`command <tool>`. This is the proven optimization approach and should be extended to tool
initialization (eval caching). **Example**: See
`/home/yandy/.config/dotfiles/common/lazy-completions.sh` lines 41-61

```bash
function kubectl() {
    unset -f kubectl 2>/dev/null
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        source <(command kubectl completion zsh) 2>/dev/null
    elif [[ -n "${BASH_VERSION:-}" ]]; then
        eval "$(command kubectl completion bash 2>/dev/null)" 2>/dev/null
    fi
    LAZY_LOADED_KUBECTL=1
    command kubectl "$@"
}
```

**Apply to**: Tool initialization (starship, direnv, atuin, mise, zoxide, thefuck, carapace).
Instead of running `eval "$(tool init shell)"` at startup, cache the output to a file and source it,
with a wrapper function to regenerate on first use if cache is stale.

### Pattern: Shell-Agnostic Detection via Version Variables

**Description**: All common files use `${ZSH_VERSION:-}` / `${BASH_VERSION:-}` for runtime shell
branching. This is the only acceptable method for shell detection in shared code. **Example**: See
`/home/yandy/.config/dotfiles/common/tools.sh` lines 315-331

```bash
if [[ -n "${ZSH_VERSION:-}" ]]; then
    eval "$(zoxide init zsh)"
elif [[ -n "${BASH_VERSION:-}" ]]; then
    eval "$(zoxide init bash)"
fi
```

**Apply to**: All optimization code must use this pattern, never `$SHELL` or `basename $0`.

### Pattern: Conditional Tool Setup via setup\_\*() Functions

**Description**: Each tool has a dedicated `setup_*()` function orchestrated by `setup_all_tools()`
in tools.sh. Functions check for tool existence first with `tool_exists` or `command -v`, then
configure the tool. This modular approach allows selective loading and testing. **Example**: See
`/home/yandy/.config/dotfiles/common/tools.sh` lines 733-770

```bash
setup_all_tools() {
    setup_mise
    setup_rust
    setup_modern_alternatives
    setup_fzf
    setup_zoxide
    setup_atuin
    setup_lazygit
    setup_yazi
    setup_direnv
    setup_autosuggestions
    setup_syntax_highlighting
    setup_python_venv
    # Load lazy completions and completions
    source "$SCRIPT_DIR/lazy-completions.sh"
    source "$SCRIPT_DIR/completions.sh"
}
```

**Apply to**: New lazy-init functions should follow the `setup_lazy_*()` naming and be called from
within `setup_all_tools()` or a new orchestrator.

### Pattern: Platform-Specific Extension Loading

**Description**: Core modules load platform-specific extensions at the end via `$DOTFILES_OS`
variable lookup, sourcing files from `common/$DOTFILES_OS/` subdirectories. **Example**: See
`/home/yandy/.config/dotfiles/common/aliases.sh` lines 685-688

```bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$SCRIPT_DIR/$DOTFILES_OS/aliases.sh" ]]; then
    source "$SCRIPT_DIR/$DOTFILES_OS/aliases.sh"
fi
```

**Apply to**: Any optimization that requires different behavior per-platform should use this loading
mechanism rather than inline conditionals.

### Pattern: SCRIPT_DIR Resolution via BASH_SOURCE

**Description**: Every module computes its own `SCRIPT_DIR` using
`$(cd "$(dirname "${BASH_SOURCE[0]:-${(%):-%x}}")" && pwd)` for zsh/bash compatibility. This spawns
a subshell every time and is duplicated across all modules. **Example**: See
`/home/yandy/.config/dotfiles/common/exports.sh` line 29,
`/home/yandy/.config/dotfiles/common/aliases.sh` line 32,
`/home/yandy/.config/dotfiles/common/functions.sh` line 35

**Apply to**: This is a micro-optimization target. Since `DOTFILES_ROOT` is already exported by both
shell entry points (zshrc line 13, bashrc line 30), SCRIPT_DIR can be derived as
`$DOTFILES_ROOT/common` without subshell forks.

## Integration Points

### Files to Create

- `/home/yandy/.config/dotfiles/common/lazy-init.sh`: New module for lazy tool initialization.
  Should contain cached-eval functions that save `eval "$(tool init shell)"` output to
  `$XDG_CACHE_HOME/dotfiles/init-cache/` and source from cache on subsequent startups. Follow the
  `lazy-completions.sh` pattern with `LAZY_INIT_*` state variables.
- `/home/yandy/.config/dotfiles/common/profiling.sh`: Optional benchmarking module. Should provide
  `dotfiles_profile_start`/`dotfiles_profile_end` functions for measuring module load times. Gate
  behind `DOTFILES_PROFILE=1` environment variable.
- `/home/yandy/.config/dotfiles/zsh/zshrc.optimized` (or inline changes to `zsh/zshrc`):
  Consolidated zsh configuration that eliminates triple compinit, deduplicates tool initialization,
  and removes appended third-party blocks that duplicate work already done by common modules.

### Files to Modify

- `/home/yandy/.config/dotfiles/common/platform.sh`: Rewrite `is_macos()`, `is_linux()`, `is_wsl()`,
  `is_arm64()`, `is_x86_64()` to use cached `$DOTFILES_OS`/`$DOTFILES_ARCH` variables instead of
  spawning subshells via `$(detect_*)`. Rewrite `setup_platform_env()` to compute values with
  builtins rather than command substitution where possible. Eliminate redundant `detect_os()` calls
  in `get_homebrew_path()`, `get_copy_command()`, `get_paste_command()`, `get_open_command()`.
- `/home/yandy/.config/dotfiles/common/exports.sh`: Remove duplicated `mkdir -p` for XDG dirs
  (already done in platform.sh line 313). Replace `$(command -v python3)` (line 199), `$(tty)` (line
  285), and `$(nproc)` (line 268) subshells with lazy or cached alternatives. Remove
  `$(cd "$(dirname ...")` subshell for SCRIPT_DIR.
- `/home/yandy/.config/dotfiles/common/paths.sh`: Replace `is_macos`/`is_linux` calls with
  `[[ "$DOTFILES_OS" == "macos" ]]` direct variable checks. Eliminate or defer `yarn global dir`
  call (line 564, extremely slow). Replace PATH dedup pipelines (tr/grep/awk/sed in subshells) with
  pure bash/zsh string operations. Remove SCRIPT_DIR subshell.
- `/home/yandy/.config/dotfiles/common/aliases.sh`: Replace `has_command` and `is_macos`/`is_linux`
  calls with cached variable checks. Replace SCRIPT_DIR subshell.
- `/home/yandy/.config/dotfiles/common/tools.sh`: Integrate `lazy-init.sh` loading. Wrap `eval`
  calls in `setup_zoxide()`, `setup_mise()` with caching. Remove SCRIPT_DIR subshell.
- `/home/yandy/.config/dotfiles/zsh/zshrc`: Consolidate triple `compinit` to single call. Remove
  duplicated tool init blocks (carapace lines 235-239, op completion lines 128-131, thefuck line
  228, fzf lines 192-197). Remove duplicated python_venv function (lines 206-220, already set up by
  tools.sh). Remove duplicated eza/op/rg aliases (lines 276-295, already in common/aliases.sh).
  Consolidate PATH cleanup pipelines (lines 370-407) into a single function. Replace
  `$(cat ~/.config/secrets/*)` with direct `read` builtins.
- `/home/yandy/.config/dotfiles/zsh/zprofile`: Add guard to prevent re-sourcing platform.sh/paths.sh
  when zshrc will do the same.
- `/home/yandy/.config/dotfiles/bash/bash_profile`: Remove duplicated `cargo/env` sourcing (already
  done in bashrc via tools.sh) and duplicated `direnv hook` (already in bashrc).
- `/home/yandy/.config/dotfiles/common/linux/aliases.sh`: Batch `has_command` checks using a loop or
  pre-computed availability hash to reduce PATH lookups.

## Code Conventions

### Naming

- **Files**: lowercase with hyphens for multi-word names (`lazy-completions.sh`, `lazy-init.sh`)
- **Functions**: `snake_case` with category prefix (`setup_fzf`, `detect_os`, `has_command`,
  `cleanup_path`)
- **Private/internal functions**: prefixed with underscore (`_setup_lazy_kubectl`, `_load_carapace`)
- **Guard variables**: `DOTFILES_*_LOADED` (non-exported, shell-local) for re-entrant prevention
- **Disable flags**: `DOTFILES_NO_AUTO_*` (checked before auto-init blocks)
- **State variables**: `LAZY_LOADED_*` for lazy-loading tracking (non-exported)
- **Exported variables**: `DOTFILES_*` prefix for all dotfiles-owned exports

### Error Handling

- Silent failures: Functions use `2>/dev/null` or `|| true` to suppress errors from missing tools
- Guard-first: Functions check `command -v tool &>/dev/null || return 0` before attempting setup
- No `set -e`: None of the common modules use `set -e`; errors are handled per-command
- Fallback chains: Editor/pager detection uses `if has_command X; elif has_command Y; else Z`
  patterns

### Testing

- No formal test framework exists. Validation is manual via:
  - `time zsh -i -c exit` / `time bash -i -c exit` for startup benchmarks
  - `DOTFILES_DEBUG=1` environment variable for verbose output
  - `system_health` function for tool availability checks
  - `lazy_completion_status` for lazy-loading verification
  - `print_path` for PATH debugging
  - Dry-run mode for symlink changes:
    `ansible-playbook playbooks/symlinks.yml --extra-vars "dryrun=true"`

## Dependencies and Services

### Available Utilities

- `has_command()`: Check if a binary exists in PATH (single `command -v` call)
- `tool_exists()`: Extended check with NVM/mise awareness
- `add_to_path()` / `prepend_path()` / `append_path()`: Safe PATH manipulation with existence and
  duplicate checks
- `force_path_front()`: Priority PATH insertion with remove-then-prepend
- `cleanup_path()`: PATH deduplication
- `detect_current_shell()`: Returns "zsh", "bash", or "unknown"
- `$DOTFILES_ROOT`: Absolute path to dotfiles repo (always `~/.config/dotfiles`)
- `$DOTFILES_OS`: Cached OS detection result
- `$DOTFILES_ARCH`: Cached architecture detection result
- `$DOTFILES_HOMEBREW_PATH`: Cached Homebrew prefix
- `$XDG_CACHE_HOME`: Cache directory (default `~/.cache`), suitable for init cache files

### Required Dependencies

- No external dependencies needed for optimization work. All optimization targets are internal shell
  scripts.
- Cache directory `$XDG_CACHE_HOME/dotfiles/` will need to be created for eval caching.

## Gotchas and Warnings

- **Triple compinit in zshrc**: `compinit` is called at lines 94, 416, and 431 of zshrc. Each call
  is expensive (~650ms). Line 94 is the intentional one; line 416 is from an rfmedia completion
  block, and line 431 is from Docker Desktop's auto-appended block. Both appended blocks also modify
  `fpath`. Consolidation must handle fpath additions that happen before the later compinit calls.
- **is_macos()/is_linux() spawn subshells**: These functions call `$(detect_os)` which runs
  `uname -s` and potentially sources `/etc/os-release` every single time. They are called ~15+ times
  across the module pipeline during startup. Rewriting them to use `$DOTFILES_OS` is the single
  highest-impact low-risk optimization.
- **SCRIPT_DIR subshell duplication**: Every common module independently computes `SCRIPT_DIR` via
  `$(cd "$(dirname ...")` which spawns a subshell. Since `$DOTFILES_ROOT` is always set by the entry
  points before any module is sourced, these can all be replaced with `$DOTFILES_ROOT/common`.
- **cargo/env sourced up to 4 times**: `~/.cargo/env` is sourced in: (1) `paths.sh`
  `setup_rust_env()`, (2) `tools.sh` `setup_rust()`, (3) `bashrc` line 154 / `bash_profile` line 23,
  (4) `completions.sh` line 417. Each sourcing re-runs cargo's PATH modification.
- **Duplicated eval calls in zshrc**: `starship init zsh`, `direnv hook zsh`, and `atuin init zsh`
  are called both in `tools.sh` setup functions AND directly in `zshrc` (lines 303, 320, 329). The
  tools.sh calls happen during the common pipeline, then zshrc re-runs them.
- **zshrc Python venv function duplication**: `python_venv()` is defined both in `tools.sh`
  `setup_python_venv()` (with chpwd hook) and again in `zshrc` lines 206-220 (with another chpwd
  hook registration). This results in the function running twice per directory change.
- **zshrc alias duplication**: eza, ripgrep, 1password, git, and editor aliases are defined in both
  `common/aliases.sh` and again in `zshrc` lines 248-295. These are redundant overwrites.
- **PATH cleanup pipelines use external tools**: `zshrc` lines 370-389 run 4 separate
  `echo | tr | grep | awk | sed` pipeline chains for PATH cleanup. These spawn ~16 subshells total.
  Pure shell string manipulation would eliminate all of them.
- **Secret file reads via $(cat)**: `zshrc` lines 123-148 use `$(cat ~/.config/secrets/*)` subshells
  to read API keys. These can be replaced with `read` builtins or `< file` redirection.
- **yarn global dir is extremely slow**: `paths.sh` line 564 calls `$(yarn global dir 2>/dev/null)`
  which can take 500ms+. This should be deferred or cached.
- **exports.sh MAKEFLAGS subshell**: Line 268 runs
  `$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1)` at source time. This spawns 1-2
  subshells.
- **completions.sh double-loads with zshrc**: `setup_all_completions()` in `completions.sh` is
  called from `tools.sh`, but `zshrc` also directly initializes carapace (line 238), op completions
  (line 129), and cdktf completions (line 154-163) duplicating work.
- **fix_homebrew_path_order() uses zsh-specific syntax**: Line 165 uses `"${(@s/:/)PATH}"` which is
  zsh-specific array splitting. On bash, this silently fails. The function is currently only called
  from zsh contexts but could break if called from a bash path.
- **GPG_TTY=$(tty) always runs**: `exports.sh` line 285 spawns a subshell on every shell startup
  even when GPG is not used. Should be lazy or gated behind `has_command gpg`.
- **Bash exits completions early but zsh loads everything eagerly**: `completions.sh` line 578
  returns early for bash, meaning all the heavy completion setup only affects zsh. This is by design
  but means zsh bears the full completion cost even though `lazy-completions.sh` already handles the
  heaviest tools.

## Task-Specific Guidance

- **For platform.sh optimization**: Rewrite the 6 convenience functions (`is_macos`, `is_linux`,
  `is_wsl`, `is_container`, `is_arm64`, `is_x86_64`) as one-liner variable comparisons:
  `is_macos() { [[ "$DOTFILES_OS" == "macos" ]]; }`. Rewrite `setup_platform_env()` to call
  `detect_os` once, store result, and derive all dependent values from it without additional
  subshells. Move `get_copy_command`/`get_paste_command`/`get_open_command` to use `$DOTFILES_OS`
  local variable instead of calling `detect_os` again.
- **For eval caching (lazy-init)**: Create `$XDG_CACHE_HOME/dotfiles/init-cache/` directory. For
  each tool (starship, direnv, atuin, mise, zoxide, thefuck, carapace), generate the init output
  once with `tool init shell > cache_file`, then source from cache on subsequent startups.
  Invalidate cache when tool binary mtime changes (compare with `stat`). Follow the
  `lazy-completions.sh` state variable pattern with `LAZY_INIT_STARSHIP`, etc.
- **For compinit consolidation**: Move all fpath modifications to happen before the single compinit
  call. Remove the two appended compinit calls at zshrc lines 416 and 431. Add their fpath additions
  (rfmedia `~/.zfunc` and docker `~/.docker/completions`) to the fpath setup section before line 94.
- **For PATH cleanup**: Replace the 4 pipeline chains in zshrc lines 370-389 with a single
  pure-shell function that iterates PATH segments once, filtering and deduplicating in a single pass
  using associative arrays (zsh) or a seen-list pattern.
- **For has_command batching**: In aliases.sh and linux/aliases.sh, pre-compute which tools are
  available into shell variables at the top of the file (`_HAS_EZA=...; _HAS_BAT=...`) and use those
  variables in conditionals instead of calling `has_command` repeatedly.
- **For subshell elimination in exports.sh**: Replace `$(command -v python3)` with a variable that
  defaults to `/usr/bin/python3`; replace `$(tty)` with a function that caches the result in a
  variable; replace `$(nproc ...)` with `/proc/cpuinfo` line count on Linux or a cached value.
- **For duplicate removal in zshrc**: Delete the following duplicated sections from zshrc: lines
  109-116 (vault/azure completions already in completions.sh), lines 128-131 (op completion already
  in completions.sh), lines 154-163 (cdktf already in completions.sh), lines 192-197 (fzf already in
  tools.sh), lines 206-220 (python_venv already in tools.sh), lines 227-229 (thefuck), lines 235-239
  (carapace already in lazy-completions.sh and completions.sh), lines 248-295 (aliases already in
  common/aliases.sh), lines 302-312 (starship already called in tools.sh or should be centralized),
  lines 319-321 (direnv already handled), lines 328-333 (atuin already handled).
- **For bash_profile cleanup**: Remove lines 22-25 (cargo/env, already sourced in bashrc via
  tools.sh) and lines 28-30 (direnv hook, already in bashrc).
