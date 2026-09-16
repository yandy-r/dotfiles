# Pattern Research: shell-startup-optimization

The shell configuration system uses a modular, cross-platform architecture where both bash and zsh
source the same common files in a fixed order. Startup performance is already a concern the codebase
addresses (lazy-completion loading exists), but there are significant remaining bottlenecks:
redundant `compinit` calls, eager `eval "$(tool init ...)"` invocations, excessive
`command -v`/`has_command` checks (~170 across common files), duplicate `mkdir -p` calls, and PATH
manipulation that runs grep/awk/sed in subshells. The zsh config calls `compinit` three times (lines
94, 416, 431 of zshrc), which is one of the most expensive zsh operations.

## Relevant Files

- `/home/yandy/.config/dotfiles/zsh/zshrc`: Zsh entry point; loads common modules, zsh-specific
  options, completion, tools, and appended third-party blocks. Calls `compinit` THREE times (lines
  94, 416, 431).
- `/home/yandy/.config/dotfiles/bash/bashrc`: Bash entry point; loads common modules in same order,
  has re-entrant guard (`DOTFILES_BASHRC_LOADED`).
- `/home/yandy/.config/dotfiles/common/platform.sh`: Platform detection engine. Auto-runs
  `setup_platform_env()` which spawns ~10 subshells via `$(detect_os)`, `$(detect_arch)`, etc.
- `/home/yandy/.config/dotfiles/common/exports.sh`: Environment variables. Runs `mkdir -p` for XDG
  dirs (duplicated from platform.sh). Contains `$(command -v python3)` and `$(nproc)` subshell calls
  at source time.
- `/home/yandy/.config/dotfiles/common/paths.sh`: PATH construction. 700+ lines with platform-aware
  ordering, deduplication via grep/awk in subshells. Has guard `DOTFILES_PATHS_LOADED`.
- `/home/yandy/.config/dotfiles/common/aliases.sh`: 680+ lines of aliases. 23 `has_command` calls at
  source time to conditionally define aliases.
- `/home/yandy/.config/dotfiles/common/functions.sh`: Utility functions. 700+ lines, loads
  platform-specific functions at end.
- `/home/yandy/.config/dotfiles/common/tools.sh`: Tool setup orchestrator. Calls `setup_all_tools()`
  which triggers 12+ setup functions, each with `command -v` checks. Loads lazy-completions.sh and
  completions.sh.
- `/home/yandy/.config/dotfiles/common/lazy-completions.sh`: Lazy-loading system for kubectl,
  docker, helm, aws, gcloud, terraform completions. Uses function-wrapper pattern.
- `/home/yandy/.config/dotfiles/common/completions.sh`: Eager completion loading. 51
  `command -v`/`has_command` checks. For bash, exits early (line 578); for zsh, loads everything
  eagerly.
- `/home/yandy/.config/dotfiles/bash/prompt.sh`: Custom bash prompt fallback (when Starship
  unavailable).
- `/home/yandy/.config/dotfiles/common/linux/aliases.sh`: Linux-specific aliases (20 `has_command`
  checks).
- `/home/yandy/.config/dotfiles/common/macos/aliases.sh`: macOS-specific aliases.
- `/home/yandy/.config/dotfiles/common/macos/functions.sh`: macOS-specific functions.
- `/home/yandy/.config/dotfiles/docs/architecture/common-system.md`: Existing documentation of the
  module dependency graph and load order.
- `/home/yandy/.config/dotfiles/docs/features/shell-configuration.md`: User-facing shell
  configuration guide with basic troubleshooting.
- `/home/yandy/.config/dotfiles/shellcheck/shellcheckrc`: ShellCheck configuration for linting.

## Architectural Patterns

- **Fixed Sequential Module Loading**: Both `bashrc` and `zshrc` source exactly the same 6 common
  modules in the same order: platform.sh -> exports.sh -> paths.sh -> aliases.sh -> functions.sh ->
  tools.sh. This is the fundamental startup pipeline. Example:
  `/home/yandy/.config/dotfiles/zsh/zshrc` lines 16-21.

- **Auto-Init on Source with Guard Variables**: Each module auto-initializes when sourced,
  controlled by guard/disable variables. Five distinct guards exist: `DOTFILES_NO_AUTO_SETUP`
  (platform.sh), `DOTFILES_NO_AUTO_PATH_SETUP` + `DOTFILES_PATHS_LOADED` (paths.sh),
  `DOTFILES_NO_AUTO_TOOL_SETUP` (tools.sh), `DOTFILES_NO_AUTO_COMPLETIONS` (completions.sh),
  `DOTFILES_NO_LAZY_COMPLETIONS` (lazy-completions.sh). Example:
  `/home/yandy/.config/dotfiles/common/paths.sh` line 731.

- **Shell-Agnostic Detection via `${ZSH_VERSION:-}` / `${BASH_VERSION:-}`**: All common files use
  runtime shell detection to branch between bash and zsh behavior. Example:
  `/home/yandy/.config/dotfiles/common/tools.sh` lines 315-331 (`setup_fzf()`).

- **Platform-Specific Extension Loading**: Base modules load platform-specific extensions at their
  tail via `source "$SCRIPT_DIR/$DOTFILES_OS/aliases.sh"`. Example:
  `/home/yandy/.config/dotfiles/common/aliases.sh` lines 685-688.

- **Lazy Completion Pattern (Function Wrapper)**: Expensive completions use a function-wrapper
  pattern where the wrapper unsets itself, loads the real completion, marks state via
  `LAZY_LOADED_*` variable, then delegates to `command <tool> "$@"`. This is the existing
  optimization pattern for completions only. Example:
  `/home/yandy/.config/dotfiles/common/lazy-completions.sh` lines 41-61.

- **Conditional Tool Setup via `setup_*()` Functions**: Each tool category has a dedicated
  `setup_*()` function (e.g., `setup_fzf`, `setup_zoxide`, `setup_mise`, `setup_autosuggestions`).
  These are orchestrated by `setup_all_tools()` in tools.sh. Example:
  `/home/yandy/.config/dotfiles/common/tools.sh` lines 733-770.

- **Dependency Fallback with Inline Source Guards**: Each common file checks if `DOTFILES_OS` is
  set; if not, it sources platform.sh itself. This ensures modules can be sourced independently but
  adds overhead when loaded normally. Example: `/home/yandy/.config/dotfiles/common/exports.sh`
  lines 28-33.

- **Cross-Platform `has_command` Abstraction**: The `has_command()` function from platform.sh wraps
  `command -v "$1" &> /dev/null`. Used 170 times across all common files during source time,
  generating 170 PATH lookups during every shell startup. Example:
  `/home/yandy/.config/dotfiles/common/platform.sh` lines 161-163.

## Code Conventions

### Naming Conventions

- **Files**: lowercase with hyphens for multi-word (e.g., `lazy-completions.sh`), `.sh` extension
  for all shell scripts.
- **Functions**: lowercase with underscores (e.g., `setup_fzf`, `detect_os`, `has_command`).
  Private/internal functions prefixed with underscore (e.g., `_setup_lazy_kubectl`).
- **Environment Variables**: Uppercase with underscores. Dotfiles-specific vars prefixed with
  `DOTFILES_` (e.g., `DOTFILES_OS`, `DOTFILES_HOMEBREW_PATH`). Lazy-load state vars prefixed with
  `LAZY_LOADED_`.
- **Guard Variables**: `DOTFILES_NO_AUTO_*` to disable auto-init, `DOTFILES_*_LOADED` to prevent
  re-execution.

### File Organization

- Every `.sh` file begins with a standardized header block using
  `# ================================================================` borders containing: Purpose,
  Dependencies, Key Functions/Variables, and Environment variables.
- Within each file, sections are delimited by
  `# ================================================================` with a section title in ALL
  CAPS (e.g., `# TOOL CATEGORIES`, `# SETUP FUNCTIONS`).
- Each function and section has a JSDoc-style comment block explaining purpose, usage, and
  arguments.
- Platform-specific code lives in `common/$DOTFILES_OS/` subdirectories, loaded at the end of each
  base module.

### Source/Import Patterns

- All common files use `#!/usr/bin/env bash` shebang (even though they are sourced by both bash and
  zsh).
- Cross-shell path resolution:
  `SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-${(%):-%x}}")" && pwd)"` -- this works in both
  bash and zsh.
- Modules check for their dependencies being loaded before self-sourcing them:
  `if [[ -z "$DOTFILES_OS" ]]; then source platform.sh; fi`.

### Style Conventions

- Indentation: tabs in bashrc, spaces (4) in zshrc and most common files.
- Conditional tool checks: `has_command <tool>` for aliases/functions,
  `command -v <tool> >/dev/null 2>&1` in zshrc (inconsistent -- zshrc uses raw `command -v` instead
  of `has_command`).
- Error suppression: `2>/dev/null` for optional operations, `|| true` or `|| return 0` for
  non-critical failures.

## Error Handling

- **Missing Tools**: Graceful degradation is the primary pattern. `has_command` checks guard all
  tool-specific code. When a tool is missing, the code simply does nothing (returns 0). Example:
  `setup_lazygit()` in tools.sh line 354: `if ! tool_exists lazygit; then return 0; fi`.
- **Missing Files**: Conditional file sourcing with `[[ -f "$path" ]] && source "$path"` is used
  extensively. Failures are silently ignored. Example: bashrc line 133:
  `[[ -f ~/.aliases ]] && source ~/.aliases`.
- **Fallback Chains**: Multi-level fallbacks for critical tools. Editor: nvim -> vim -> nano -> vi.
  Pager: bat -> less -> more. ls: eza -> ls (platform-specific flags). Prompt: starship -> custom
  prompt -> minimal PS1.
- **Debug Mode**: `DOTFILES_DEBUG=1` enables verbose output in tools.sh and paths.sh. Only tools.sh
  uses `DOTFILES_DEBUG` for conditional logging (5 debug checkpoints).
- **No Error Logging**: There is no structured error logging or timing infrastructure. No startup
  profiling hooks exist.
- **Subshell Failure Handling**: Most `eval "$(tool init ...)"` calls suppress stderr via
  `2>/dev/null` but do not check exit codes.

## Testing Approach

- **No Automated Tests**: The codebase has no shell script test suite. No test framework (bats,
  shunit2, etc.) is used.
- **Manual Testing Commands**: Documentation suggests: `bash -l -c 'echo "loaded"'`,
  `zsh -l -c 'echo "loaded"'`, `exec $SHELL -l` for reload.
- **Timing Commands**: Documented in `docs/features/shell-configuration.md`: `time zsh -i -c exit`
  and `time bash -i -c exit`.
- **Diagnostic Functions**: `system_health()`, `list_missing_tools()`, `print_platform_info()`,
  `print_path()`, `lazy_completion_status()` provide runtime diagnostics.
- **Dry-Run Support**: Ansible symlink playbook supports `--extra-vars "dryrun=true"` but this is
  for symlink management, not shell config.
- **ShellCheck**: `/home/yandy/.config/dotfiles/shellcheck/shellcheckrc` exists for linting.
- **Debug Mode**: `DOTFILES_DEBUG=1` provides limited debug output during startup.

## Patterns to Follow

### When Implementing Startup Optimization

1. **Follow the Guard Variable Pattern**: Any new optimization should use the existing
   `DOTFILES_NO_*` / `DOTFILES_*_LOADED` convention. New guards should be named consistently (e.g.,
   `DOTFILES_FAST_STARTUP` or `DOTFILES_NO_LAZY_TOOLS`).

2. **Extend the Lazy-Loading Pattern**: The function-wrapper pattern from `lazy-completions.sh`
   should be extended to tool initialization (`eval "$(starship init ...)"`,
   `eval "$(direnv hook ...)"`, `eval "$(atuin init ...)"`, `eval "$(mise activate ...)"`,
   `eval "$(zoxide init ...)"`, `eval "$(thefuck --alias)"`). These are the most expensive startup
   operations.

3. **Maintain Shell-Agnostic Code**: All optimization code must work in both bash and zsh. Use the
   `${ZSH_VERSION:-}` / `${BASH_VERSION:-}` detection pattern.

4. **Preserve the Module Load Order**: The 6-module pipeline (platform -> exports -> paths ->
   aliases -> functions -> tools) is a hard dependency chain. Optimizations must not break this
   ordering.

5. **Use the `setup_*()` Function Pattern**: New optimization functionality should be encapsulated
   in `setup_*()` functions, consistent with the tools.sh pattern.

6. **Maintain Fallback Behavior**: All optimizations must degrade gracefully. If a cached value is
   stale or missing, fall back to the current eager behavior.

7. **Use Section Header Convention**: New code blocks must use the
   `# ================================================================` / `# SECTION NAME` /
   `# ================================================================` delimiter pattern.

## Edgecases

- **Triple `compinit` in zshrc**: `compinit` is called at lines 94, 416, and 431 of zshrc. The calls
  at 416 and 431 were appended by external tools (rfmedia and Docker Desktop). Each `compinit` call
  can take 100-300ms. This is the single biggest zsh startup bottleneck.
- **Duplicate `mkdir -p` for XDG dirs**: Both `platform.sh` (line 313) and `exports.sh` (line 49)
  call `mkdir -p` for the same four XDG directories, causing redundant filesystem operations.
- **PATH cleanup in zshrc uses subshells**: Lines 374-389 of zshrc run grep/awk/sed in subshells
  multiple times for PATH deduplication/cleanup. This duplicates work already done by
  `cleanup_path()` in paths.sh.
- **`is_macos()` and `is_linux()` call `detect_os()` every time**: These convenience functions call
  `$(detect_os)` which runs `uname -s` and potentially reads `/etc/os-release` in a subshell on
  every invocation, rather than checking the cached `$DOTFILES_OS` variable. Called many times
  during startup.
- **`get_homebrew_path()` calls `brew --prefix`**: This is expensive (~50-100ms) and is called
  during `setup_platform_env()`, but the result is cached in `$DOTFILES_HOMEBREW_PATH`. However,
  `detect_package_manager()` also calls `command -v brew` separately.
- **Zshrc appended blocks break patterns**: Lines 414-432 of zshrc contain blocks appended by
  external tools (rfmedia, pnpm, Docker, Antigravity) that duplicate PATH additions and `compinit`
  calls already handled by the common system.
- **`thefuck --alias` eval in zshrc**: Line 228 runs `eval "$(thefuck --alias)"` which can take
  200-500ms. This is not lazy-loaded.
- **`op completion zsh` eval in zshrc**: Line 129 runs `eval "$(op completion zsh)"` eagerly, even
  though 1Password CLI is rarely used interactively.
- **`cargo/env` sourced twice**: Both bashrc (line 154) and tools.sh `setup_rust()` (line 489)
  source `~/.cargo/env`, causing double execution.
- **`DOTFILES_PATHS_LOADED` is not exported**: Set as a shell variable (not exported), so it
  correctly allows child shells to re-run path setup. This is intentional per the comment on line
  733 of paths.sh.
- **`DOTFILES_BASHRC_LOADED` IS exported**: Unlike `DOTFILES_PATHS_LOADED`, the bashrc guard is
  exported (line 18), which means subshells will skip bashrc loading entirely. This could cause
  issues in nested bash sessions.

## Other Docs

- `/home/yandy/.config/dotfiles/docs/architecture/common-system.md` -- Detailed module dependency
  graph and API reference
- `/home/yandy/.config/dotfiles/docs/architecture/overview.md` -- High-level architecture overview
- `/home/yandy/.config/dotfiles/docs/features/shell-configuration.md` -- Shell setup guide with
  troubleshooting
- `/home/yandy/.config/dotfiles/docs/features/completions.md` -- Completion system documentation
- `/home/yandy/.config/dotfiles/common/README.md` -- Common system README with extension guides
