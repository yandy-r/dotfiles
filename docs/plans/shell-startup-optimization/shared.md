# Shell Startup Optimization

The shell startup system uses a 6-module sequential pipeline (platform -> exports -> paths ->
aliases -> functions -> tools) sourced identically by both bash and zsh entry points. Zsh is the
primary optimization target at ~3.2s startup vs bash's ~0.45s, with `compinit` called 3 times
consuming ~86% of measured zsh time (1960ms across compdump, compdef, compinit, and compaudit).
Additional bottlenecks include 8+ subshell forks in `setup_platform_env()`, 7 uncached
`eval "$(tool init ...)"` calls (starship, direnv, atuin, mise, zoxide, thefuck, carapace), ~170
`has_command`/`command -v` PATH lookups, redundant `cargo/env` sourcing (up to 4 times), and PATH
cleanup pipelines using grep/awk/sed in subshells. The existing lazy-completion wrapper pattern in
`lazy-completions.sh` is the proven optimization approach to extend to tool initialization.

## Relevant Files

- common/platform.sh: Platform detection engine; spawns 8+ subshells via setup_platform_env();
  is_macos()/is_linux() re-detect OS every call instead of using cached $DOTFILES_OS
- common/exports.sh: Environment variables; runs mkdir -p for XDG dirs (duplicated from
  platform.sh), $(command -v python3), $(tty), $(nproc) subshells at source time
- common/paths.sh: PATH construction; 743 lines with is_macos/is_linux calls spawning subshells,
  yarn global dir (slow), PATH dedup pipelines; has DOTFILES_PATHS_LOADED guard
- common/aliases.sh: 688 lines of aliases with ~23 has_command checks at source time plus 7
  is_macos/is_linux calls
- common/functions.sh: Utility functions; low startup cost (definitions only), ~8 is_macos/is_linux
  checks at source time
- common/tools.sh: Tool setup orchestrator; setup_all_tools() triggers 12+ setup functions each with
  command -v checks and eval calls; loads lazy-completions.sh and completions.sh
- common/lazy-completions.sh: Reference implementation for lazy loading via function-wrapper
  pattern; wraps kubectl, docker, helm, carapace, aws, gcloud, terraform
- common/completions.sh: Eager completion loading; bash exits early (line 578), zsh loads everything
  with 51+ command -v checks
- zsh/zshrc: Primary optimization target (432 lines); triple compinit (lines 94, 416, 431), 7 eval
  calls, PATH cleanup pipelines, secret file reads via $(cat), appended third-party blocks
- zsh/zprofile: Login shell profile; sources platform.sh and paths.sh before zshrc duplicates the
  same work
- bash/bashrc: Bash entry point (195 lines); already fast, shares same 6-module pipeline
- bash/bash_profile: Bash login shell; duplicates cargo/env and direnv hook from bashrc
- common/macos/aliases.sh: macOS-specific aliases with platform-detection calls
- common/linux/aliases.sh: Linux-specific aliases with ~20 has_command checks

## Relevant Patterns

**Fixed Sequential Module Loading**: Both shells source the same 6 common modules in identical order
(platform -> exports -> paths -> aliases -> functions -> tools). This is a hard dependency chain
that optimizations must preserve. See [zsh/zshrc](../../zsh/zshrc) lines 16-21.

**Auto-Init on Source with Guard Variables**: Each module auto-initializes when sourced, controlled
by `DOTFILES_NO_AUTO_*` disable flags and `DOTFILES_*_LOADED` re-entrant guards. New optimization
code should follow this convention. See [common/paths.sh](../../common/paths.sh) line 731.

**Lazy Completion Pattern (Function Wrapper)**: The proven optimization pattern -- wrapper functions
that unset themselves, load the real completion, mark state via `LAZY_LOADED_*` variable, then
delegate. This pattern should be extended to tool initialization (eval caching). See
[common/lazy-completions.sh](../../common/lazy-completions.sh) lines 41-61.

**Conditional Tool Setup via setup\_\*() Functions**: Each tool has a dedicated `setup_*()` function
orchestrated by `setup_all_tools()` in tools.sh. New optimization functions should follow this
pattern. See [common/tools.sh](../../common/tools.sh) lines 733-770.

**Shell-Agnostic Detection**: All common files use `${ZSH_VERSION:-}` / `${BASH_VERSION:-}` for
runtime shell branching. Optimization code must work in both shells. See
[common/tools.sh](../../common/tools.sh) lines 315-331.

**Platform Detection via Cached Variables**: `setup_platform_env()` exports `DOTFILES_OS`,
`DOTFILES_ARCH`, etc. but convenience functions `is_macos()`/`is_linux()` bypass the cache and
re-detect via subshells. These must be rewritten to use `$DOTFILES_OS` directly. See
[common/platform.sh](../../common/platform.sh) lines 337-361.

## Relevant Docs

**/docs/architecture/overview.md**: You _must_ read this when working on system-wide changes to
understand the full initialization flow and component relationships.

**/docs/architecture/common-system.md**: You _must_ read this when modifying the common module
pipeline to understand module dependencies, load order, and existing disable flags.

**/docs/architecture/platform-detection.md**: You _must_ read this when optimizing platform
detection to understand the subshell-heavy detection flow.

**/docs/features/completions.md**: You _must_ read this when modifying the completion system to
understand the lazy vs eager loading strategy and per-tool load classifications.

**/docs/features/shell-configuration.md**: You _must_ read this when adding profiling or
benchmarking infrastructure to understand existing troubleshooting guidance.

**/common/README.md**: You _must_ read this when adding new modules or modifying the extension
pattern to understand the common system structure.
