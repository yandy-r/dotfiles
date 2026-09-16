# Context Analysis: shell-startup-optimization

## Executive Summary

The shell startup system loads 6 common modules sequentially (platform, exports, paths, aliases,
functions, tools) from both bash and zsh. Zsh is the sole optimization target at ~3.2s startup vs
bash's ~0.45s, with three redundant `compinit` calls consuming ~86% of zsh measured time (1960ms).
The remaining cost comes from 7+ uncached `eval "$(tool init ...)"` calls, 36 subshell-spawning
`is_macos`/`is_linux` invocations, ~170 `has_command` PATH lookups, duplicate `cargo/env` sourcing,
and PATH cleanup pipelines using grep/awk/sed. The existing `lazy-completions.sh` function-wrapper
pattern is the proven approach to extend for tool initialization caching.

## Architecture Context

- **System Structure**: Six common modules in `common/` sourced by both `bash/bashrc` and
  `zsh/zshrc` in fixed order: `platform.sh` -> `exports.sh` -> `paths.sh` -> `aliases.sh` ->
  `functions.sh` -> `tools.sh`. The `tools.sh` module internally sources `lazy-completions.sh` and
  `completions.sh`. Platform-specific extensions live in `common/macos/` and `common/linux/`, loaded
  at the tail of their parent modules. Zsh additionally runs `zsh/zprofile` before `zshrc` on login
  shells, duplicating platform.sh and paths.sh loading.
- **Data Flow**: Shell entry point sets `DOTFILES_ROOT` -> sources 6 modules (each auto-initializes
  via bottom-of-file INITIALIZATION section) -> shell-specific config (options, completions, tool
  evals) -> local overrides (`~/.zshrc.local`). Guard variables (`DOTFILES_NO_AUTO_*`,
  `DOTFILES_*_LOADED`) control re-entrant loading. The zshrc then runs inline tool inits (starship,
  direnv, atuin, thefuck, carapace, op) and PATH cleanup before appended third-party blocks
  (rfmedia, pnpm, Docker Desktop) that add redundant `compinit` calls.
- **Integration Points**: New optimization code plugs in at four levels: (1) rewriting
  `is_macos()`/`is_linux()` in `platform.sh` to use cached `$DOTFILES_OS`, (2) adding eval-output
  caching alongside existing `setup_*()` functions in `tools.sh`, (3) consolidating `compinit` to a
  single cached call in `zshrc`, (4) extending the `lazy-completions.sh` wrapper pattern to tool
  init (starship, direnv, atuin, etc.).

## Critical Files Reference

- `/home/yandy/.config/dotfiles/zsh/zshrc`: Primary optimization target (432 lines). Triple
  `compinit` (lines 94, 416, 431), 7 eval calls, PATH cleanup pipelines, secret file reads via
  `$(cat)`, appended third-party blocks.
- `/home/yandy/.config/dotfiles/zsh/zprofile`: Login shell pre-loader. Sources platform.sh and
  paths.sh before zshrc does the same, causing duplicate work on login shells.
- `/home/yandy/.config/dotfiles/common/platform.sh`: Platform detection engine.
  `setup_platform_env()` spawns 8+ subshells. `is_macos()`/`is_linux()` bypass cached `$DOTFILES_OS`
  and re-detect via `uname -s` on every call (36 times total across modules).
- `/home/yandy/.config/dotfiles/common/tools.sh`: Tool orchestrator (780 lines). `setup_all_tools()`
  triggers 12+ setup functions each with command checks and eval calls; sources lazy-completions.sh
  and completions.sh.
- `/home/yandy/.config/dotfiles/common/paths.sh`: PATH construction (743 lines). Complex ordering
  with `is_macos`/`is_linux` subshell calls, `yarn global dir` (slow), dedup pipelines. Has
  `DOTFILES_PATHS_LOADED` guard (not exported -- intentional).
- `/home/yandy/.config/dotfiles/common/lazy-completions.sh`: Reference implementation for the
  function-wrapper lazy-loading pattern. Wraps kubectl, docker, helm, carapace, aws, gcloud,
  terraform.
- `/home/yandy/.config/dotfiles/common/completions.sh`: Eager completion loading. Bash exits early
  (line 578); zsh loads everything with 51+ `command -v` checks.
- `/home/yandy/.config/dotfiles/common/exports.sh`: Environment variables. Contains redundant
  `mkdir -p` (also in platform.sh), plus `$(command -v python3)`, `$(tty)`, `$(nproc)` subshells at
  source time.
- `/home/yandy/.config/dotfiles/common/aliases.sh`: 688 lines with ~23 `has_command` checks and 7
  `is_macos`/`is_linux` calls at source time.
- `/home/yandy/.config/dotfiles/bash/bash_profile`: Duplicates cargo/env sourcing and direnv hook
  from bashrc.

## Patterns to Follow

- **Guard Variable Convention**: Use `DOTFILES_NO_AUTO_*` for opt-out disabling and
  `DOTFILES_*_LOADED` for re-entrant guards. New guards must follow this naming (e.g.,
  `DOTFILES_EVAL_CACHE_LOADED`). Example: `/home/yandy/.config/dotfiles/common/paths.sh` line 731.
- **Lazy Function-Wrapper Pattern**: Wrapper function unsets itself, loads real implementation, sets
  `LAZY_LOADED_*` state variable, delegates via `command <tool> "$@"`. Extend this to tool init
  (eval caching). Example: `/home/yandy/.config/dotfiles/common/lazy-completions.sh` lines 41-61.
- **`setup_*()` Function Pattern**: Each tool has a dedicated setup function orchestrated by
  `setup_all_tools()`. New optimization functions should follow this convention. Example:
  `/home/yandy/.config/dotfiles/common/tools.sh` lines 733-770.
- **Shell-Agnostic Branching**: Use `${ZSH_VERSION:-}` / `${BASH_VERSION:-}` for runtime shell
  detection. All code in `common/` must work in both shells. Example:
  `/home/yandy/.config/dotfiles/common/tools.sh` lines 315-331.
- **Section Header Convention**: Use
  `# ================================================================` borders with ALL CAPS section
  titles. All new code must follow this formatting.
- **Auto-Init on Source**: Each module has an INITIALIZATION section at file bottom that runs on
  source, controlled by guard variables. New modules must follow this.
- **Graceful Degradation**: Missing tools silently return 0. Stale/missing cache must fall back to
  eager behavior. Example: `setup_lazygit()` in tools.sh:
  `if ! tool_exists lazygit; then return 0; fi`.

## Cross-Cutting Concerns

- **Shell Compatibility**: Every optimization must work identically in bash and zsh. The `common/`
  modules use `#!/usr/bin/env bash` shebang but are sourced by zsh. Cross-shell path resolution
  uses: `SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-${(%):-%x}}")" && pwd)"`.
- **No Automated Test Suite**: Zero shell tests exist (no bats, shunit2, etc.). Manual testing only:
  `time zsh -i -c exit`, `time bash -i -c exit`. Any optimization work should consider adding at
  least basic regression tests.
- **Cache Invalidation**: Eval output caching requires a versioning strategy. Tool version changes,
  config changes, and shell version changes must invalidate cached init scripts. Cache location
  should be `$XDG_CACHE_HOME/dotfiles/` (already defined).
- **Login vs Interactive Shell Paths**: Zsh login shells run zprofile before zshrc, causing double
  platform.sh loading. Optimizations must account for both login and non-login startup paths.
- **Appended Third-Party Blocks**: Lines 414-432 of zshrc were appended by external tool installers
  (rfmedia, pnpm, Docker Desktop, Antigravity). These add redundant `compinit` calls and PATH/fpath
  modifications. They must be consolidated, not just worked around.
- **Secret Loading**: Four API keys read via `$(cat file)` subshells in zshrc (lines 123-148).
  Should use `read` builtin or `<file` redirection to avoid forks.
- **Debug/Profiling Infrastructure**: Only `DOTFILES_DEBUG=1` exists for verbose output in tools.sh
  and paths.sh. No startup timing infrastructure, no per-module profiling hooks. Adding benchmark
  infrastructure should be early work.

## Parallelization Opportunities

- **Independent Optimization Targets**: These can be worked on simultaneously with no shared file
  conflicts:
  - `platform.sh` is_macos/is_linux rewrite (self-contained in platform.sh)
  - Eval caching infrastructure (new file, e.g., `common/eval-cache.sh`)
  - Profiling/benchmarking infrastructure (new file or utility)
- **Shared File Modifications (Need Coordination)**:
  - `zsh/zshrc` changes (compinit consolidation, inline eval replacement, PATH cleanup, secret
    reads, third-party block cleanup) -- this is the highest-contention file
  - `common/tools.sh` changes (eval caching integration into setup\_\* functions, cargo/env dedup)
  - `common/completions.sh` changes (zsh eager loading reduction)
- **Sequential Dependencies**:
  - Platform detection caching must land before PATH/alias optimization (since those modules call
    is_macos/is_linux)
  - Eval caching infrastructure must exist before individual tool inits can be cached
  - compinit consolidation should be done before completion system restructuring

## Implementation Constraints

- **Module Load Order is Immutable**: The 6-module pipeline (platform -> exports -> paths -> aliases
  -> functions -> tools) is a hard dependency chain. Cannot be reordered or parallelized.
- **Backward Compatibility**: `~/.zshrc.local` and `~/.bashrc.local` are user override files that
  must continue to work. Existing `DOTFILES_NO_AUTO_*` flags must continue to disable their
  respective modules.
- **`DOTFILES_PATHS_LOADED` Is Intentionally Not Exported**: This allows child shells to re-run path
  setup. Do not export it.
- **`DOTFILES_BASHRC_LOADED` IS Exported**: This prevents subshells from re-loading bashrc. Be aware
  this means nested bash sessions skip bashrc entirely.
- **No External Dependencies for Core Startup**: Optimization code must not require installing new
  tools. Pure shell builtins and POSIX utilities only for the startup path.
- **Zsh `typeset -U path`**: Zsh has a built-in mechanism for PATH deduplication that should replace
  the grep/awk/sed pipelines.
- **compinit -C**: Using `compinit -C` skips the security check and uses the cached zcompdump
  directly, but requires a strategy for when to regenerate (e.g., once per day via file age check).
- **thefuck is Python-based**: Its `eval "$(thefuck --alias)"` costs 200-500ms. This should be
  either lazy-loaded (on first invocation) or its output cached aggressively, as the output is
  stable.

## Startup Cost Priority Matrix

| Optimization                                          | Estimated Savings | Difficulty | Risk   |
| ----------------------------------------------------- | ----------------- | ---------- | ------ |
| Consolidate compinit to 1 call with `-C` caching      | 600-1800ms        | Low        | Low    |
| Cache thefuck --alias output                          | 200-500ms         | Low        | Low    |
| Rewrite is_macos/is_linux to use $DOTFILES_OS         | 50-150ms          | Low        | Low    |
| Cache eval output (starship/direnv/atuin/zoxide/mise) | 100-300ms         | Medium     | Medium |
| Add platform.sh load guard for zprofile/zshrc         | 50-100ms          | Low        | Low    |
| Replace PATH cleanup with zsh builtins                | 50-100ms          | Low        | Low    |
| Deduplicate cargo/env sourcing                        | 10-30ms           | Low        | Low    |
| Remove duplicate carapace initialization              | 30-80ms           | Low        | Low    |
| Replace $(cat) with read builtin for secrets          | 20-40ms           | Low        | Low    |
| Reduce eager completion loading in zsh                | 100-300ms         | Medium     | Medium |

## Key Recommendations

- **Phase 1 -- Quick Wins (no new infrastructure)**: Consolidate compinit to single `compinit -C`
  call, rewrite is_macos/is_linux to use `$DOTFILES_OS`, add platform.sh load guard, deduplicate
  cargo/env, remove duplicate carapace init, replace PATH cleanup with `typeset -U path`, replace
  `$(cat)` with `read` builtin.
- **Phase 2 -- Eval Caching Infrastructure**: Build a generic eval-caching module
  (`common/eval-cache.sh`) that generates cache files keyed by tool version + shell type, stores in
  `$XDG_CACHE_HOME/dotfiles/`, and sources from cache with fallback to live eval. Apply to starship,
  direnv, atuin, zoxide, mise, thefuck, op, carapace.
- **Phase 3 -- Lazy Tool Init**: Extend the lazy-completions.sh wrapper pattern to defer tool
  initialization (especially thefuck, op completion, carapace) until first use rather than startup.
- **Phase 4 -- Completion System Restructure**: Unify the three completion loading paths
  (lazy-completions.sh, completions.sh, zshrc inline) into a single coherent system. Move zsh toward
  lazy/deferred completion loading similar to bash's early-return strategy.
- **Benchmark Before and After Each Phase**: Add timing infrastructure early. Measure
  `time zsh -i -c exit` and per-module timing via `zmodload zsh/zprof` before and after each change.
- **Clean Up Appended Third-Party Blocks**: Lines 414-432 of zshrc must be properly integrated into
  the common system or removed, not left as dangling appended code.

## Relevant Docs

- `/home/yandy/.config/dotfiles/docs/architecture/overview.md`: System architecture with
  initialization flow diagrams
- `/home/yandy/.config/dotfiles/docs/architecture/common-system.md`: Module dependency graph,
  disable flags, API reference
- `/home/yandy/.config/dotfiles/docs/architecture/platform-detection.md`: Platform detection
  subshell flow
- `/home/yandy/.config/dotfiles/docs/features/completions.md`: Lazy vs eager loading strategy,
  per-tool classifications
- `/home/yandy/.config/dotfiles/docs/features/shell-configuration.md`: Shell setup guide with
  slow-startup troubleshooting
- `/home/yandy/.config/dotfiles/common/README.md`: Common system structure and extension patterns
