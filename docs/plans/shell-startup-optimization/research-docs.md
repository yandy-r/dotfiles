# Documentation Research: shell-startup-optimization

This report catalogs all documentation and source files relevant to implementing shell startup
optimization for the dotfiles repository. It identifies required reading, architectural patterns
already in place, current performance bottlenecks visible in the code, and gaps in existing
documentation.

## Architecture Docs

- `/home/yandy/.config/dotfiles/docs/architecture/overview.md`: High-level system architecture with
  Mermaid diagrams showing the shell initialization flow from entry points (bashrc/zshrc) through
  all common modules. Documents the sequential loading order: platform -> exports -> paths ->
  aliases -> functions -> tools -> lazy-completions -> completions. Lists component dependencies and
  the key design principle of "Lazy Loading: Heavy completions load on-demand for fast shell
  startup."
- `/home/yandy/.config/dotfiles/docs/architecture/common-system.md`: Deep dive into the `common/`
  directory structure, module dependency graph, and load order. Documents all disable flags
  (`DOTFILES_NO_AUTO_SETUP`, `DOTFILES_NO_AUTO_PATH_SETUP`, `DOTFILES_NO_AUTO_TOOL_SETUP`,
  `DOTFILES_NO_AUTO_COMPLETIONS`, `DOTFILES_NO_LAZY_COMPLETIONS`) that can skip expensive
  initialization. Also documents `DOTFILES_DEBUG=1` for verbose output.
- `/home/yandy/.config/dotfiles/docs/architecture/platform-detection.md`: Platform detection flow
  documentation. Relevant because `platform.sh` runs first and spawns multiple subshells via
  `$(detect_os)`, `$(detect_arch)`, `$(detect_container)`, `$(detect_package_manager)`,
  `$(get_homebrew_path)`, `$(get_copy_command)`, `$(get_paste_command)`, `$(get_open_command)` --
  each a fork. This is a known startup cost area.

## Development Guides

- `/home/yandy/.config/dotfiles/docs/plans/documentation-strategy.md`: Documentation strategy and
  roadmap. References planned but not yet created `docs/development/` guides for contributing,
  adding tools, and testing. These do not yet exist.

## Feature Docs

- `/home/yandy/.config/dotfiles/docs/features/shell-configuration.md`: Shell configuration guide
  covering bash/zsh initialization flow, shell options, vi mode, Starship prompt, and a
  troubleshooting section specifically about "Slow Shell Startup" with profiling commands
  (`time zsh -i -c exit`, `time bash -i -c exit`) and advice about lazy-loaded completions.
- `/home/yandy/.config/dotfiles/docs/features/completions.md`: Comprehensive completion system
  guide. Documents the lazy loading design pattern with sequence diagram, lists all 50+ tools with
  their load strategy (Lazy vs Eager), and includes a "Performance Considerations" section with
  startup time impact estimates: Lazy-loaded = None, Dynamic (fast) = ~50ms, File-based = ~10ms.
- `/home/yandy/.config/dotfiles/docs/features/modern-tools.md`: Modern CLI tools guide. Relevant
  because tools like zoxide, mise, fzf, and starship all use `eval "$(tool init shell)"` patterns
  that fork subshells during startup.
- `/home/yandy/.config/dotfiles/docs/features/environment-management.md`: Direnv and atuin
  integration guide. Both use `eval "$(tool hook shell)"` during initialization which are
  startup-time costs.

## Reference Docs

- `/home/yandy/.config/dotfiles/docs/reference/environment-variables.md`: Complete environment
  variable reference. Documents the "How Variables Are Loaded" sequence and all variables set during
  startup. Relevant for understanding the export cost during initialization.
- `/home/yandy/.config/dotfiles/docs/reference/aliases.md`: Alias reference (not read in detail,
  lower priority).
- `/home/yandy/.config/dotfiles/docs/reference/functions.md`: Functions reference (not read in
  detail, lower priority).

## README Files

- `/home/yandy/.config/dotfiles/README.md`: Root README with quick start, claims "5x faster than
  Starship with no framework overhead" in the shell experience section. Documents repository
  structure.
- `/home/yandy/.config/dotfiles/CLAUDE.md`: Primary technical reference (18KB). Covers architecture,
  common system, troubleshooting (debugging platform detection, tool setup), shell testing commands,
  and "Performance Optimization" section recommending lazy loading, caching, conditional loading,
  and modern tools.
- `/home/yandy/.config/dotfiles/AGENTS.md`: Duplicate of CLAUDE.md content for AI agent guidance.
- `/home/yandy/.config/dotfiles/common/README.md`: Common system README. Documents directory
  structure, module loading order, extension patterns, and debugging with `DOTFILES_DEBUG=1`.
- `/home/yandy/.config/dotfiles/bash/README.md`: Bash configuration README. Documents loading order
  (login shell -> interactive shell -> common system -> bash-specific). Has a "Slow Shell Startup"
  troubleshooting section with `time bash -i -c exit` and debug mode instructions.
- `/home/yandy/.config/dotfiles/zsh/README.md`: Zsh configuration README. Documents loading order
  and zsh-specific features. Has a "Slow Shell Startup" troubleshooting section plus specific
  advice: rebuild completion cache with `rm -f ~/.zcompdump*`, start without plugins with `zsh -f`.
- `/home/yandy/.config/dotfiles/docs/README.md`: Documentation index. Maps all documentation
  categories.
- `/home/yandy/.config/dotfiles/starship-bootstrap.md`: Starship prompt setup guide, relevant
  because `eval "$(starship init shell)"` is a startup cost.

## Source Files Critical for Implementation

These source files contain the actual startup code and performance-relevant patterns:

- `/home/yandy/.config/dotfiles/zsh/zshrc`: Main zsh entry point. Contains the full initialization
  sequence plus significant post-load work including: triple `compinit` calls (lines 94, 416, 431),
  PATH cleanup with multiple `grep`/`awk`/`sed` pipelines (lines 374-406), reading secret files with
  `$(cat ...)` forks (lines 123-148), `eval "$(starship init zsh)"`, `eval "$(direnv hook zsh)"`,
  `eval "$(atuin init zsh)"`, `eval "$(thefuck --alias)"`, `source <(carapace _carapace)`, and
  appended third-party code at the bottom (pnpm, Antigravity, Docker completions).
- `/home/yandy/.config/dotfiles/bash/bashrc`: Main bash entry point. Simpler than zsh but still
  does: six `source` calls to common modules, `eval "$(starship init bash)"`,
  `eval "$(direnv hook bash)"`, `eval "$(atuin init bash)"`, and bash completion loading.
- `/home/yandy/.config/dotfiles/common/platform.sh`: Platform detection engine. The
  `setup_platform_env()` function spawns 8+ subshells via command substitution: `$(detect_os)`,
  `$(detect_arch)`, `$(detect_container)`, `$(detect_package_manager)`, `$(get_homebrew_path)`,
  `$(get_copy_command)`, `$(get_paste_command)`, `$(get_open_command)`. Each `detect_*` function
  also calls `uname` or reads files. The `is_macos()`, `is_linux()` etc. helpers each call
  `$(detect_os)` again (not cached).
- `/home/yandy/.config/dotfiles/common/exports.sh`: Environment exports. Contains `mkdir -p` calls,
  `$(command -v python3)` subshell, `$(tty)` subshell, `$(nproc)` or `$(sysctl)` subshells, and a
  `has_command` check for each editor priority level. Also calls `cleanup_path_var` for 4 variables
  at the end.
- `/home/yandy/.config/dotfiles/common/paths.sh`: PATH management. Complex PATH construction with
  multiple `has_command`, `is_macos`, `is_linux` calls (each spawning subshells). The
  `fix_homebrew_path_order()` function does `find`, `sort`, `awk`, `grep`, `sed` pipeline
  operations. The `setup_node_env()` calls `yarn global dir` which is slow.
- `/home/yandy/.config/dotfiles/common/aliases.sh`: Alias definitions. Many `has_command` checks
  that each fork a subshell.
- `/home/yandy/.config/dotfiles/common/functions.sh`: Utility function definitions (loaded but
  functions only execute on use, so low startup cost).
- `/home/yandy/.config/dotfiles/common/tools.sh`: Tool setup orchestrator. Calls `setup_mise` ->
  `eval "$(mise activate shell)"`, `setup_fzf` -> sources fzf scripts, `setup_zoxide` ->
  `eval "$(zoxide init shell)"`, `setup_modern_alternatives` -> creates ripgrep config with
  `mkdir -p` and `cat > file`, `setup_autosuggestions` -> sources plugin file,
  `setup_syntax_highlighting` -> sources plugin file. Then sources `lazy-completions.sh` and
  `completions.sh`.
- `/home/yandy/.config/dotfiles/common/lazy-completions.sh`: Lazy completion wrapper functions. Low
  cost at setup time (just defines functions), high cost deferred to first use.
- `/home/yandy/.config/dotfiles/common/completions.sh`: Eager completion loading. For bash, returns
  early after git completions. For zsh, loads all completion categories eagerly.

## Must-Read Documents

Documents that an implementer MUST read before starting work, listed by priority:

1. **`/home/yandy/.config/dotfiles/docs/architecture/overview.md`** - Understand the full
   initialization flow and component relationships
2. **`/home/yandy/.config/dotfiles/docs/architecture/common-system.md`** - Understand module
   dependencies, load order, and existing disable flags
3. **`/home/yandy/.config/dotfiles/zsh/zshrc`** - The most complex entry point with the most
   optimization opportunities (triple compinit, PATH cleanup pipelines, multiple eval forks, secret
   file reads)
4. **`/home/yandy/.config/dotfiles/bash/bashrc`** - The simpler entry point, useful for comparison
5. **`/home/yandy/.config/dotfiles/common/platform.sh`** - The foundation module; its subshell-heavy
   detection pattern is the first optimization target
6. **`/home/yandy/.config/dotfiles/common/tools.sh`** - The heaviest common module; orchestrates all
   tool initialization including multiple `eval "$(tool init)"` calls
7. **`/home/yandy/.config/dotfiles/common/paths.sh`** - Complex PATH construction with
   subshell-heavy utility calls and the expensive `fix_homebrew_path_order()` function
8. **`/home/yandy/.config/dotfiles/docs/features/completions.md`** - Understand the existing lazy
   loading strategy and which tools are deferred vs eager
9. **`/home/yandy/.config/dotfiles/common/lazy-completions.sh`** - Reference implementation for the
   lazy loading pattern already in use
10. **`/home/yandy/.config/dotfiles/docs/features/shell-configuration.md`** - Existing
    troubleshooting guidance for slow startup

## Documentation Gaps

Areas where documentation is missing, outdated, or would be needed for shell startup optimization
work:

1. **No profiling/benchmarking guide**: The troubleshooting sections mention `time shell -i -c exit`
   but there is no documentation on how to profile individual modules, use zsh's `zprof`, or measure
   the cost of specific `eval` calls. A startup profiling methodology doc would be essential.

2. **No documentation of known performance costs**: There is no document that inventories the
   startup-time cost of each module or `eval` call. The completions.md mentions "<0.5s vs 5-10s" for
   lazy vs eager loading, but no other module has measured costs.

3. **No caching strategy documentation**: The CLAUDE.md mentions "Cache expensive computations when
   possible" as a best practice, but there is no documentation of what is currently cached, what
   could be cached (e.g., platform detection results, `eval` outputs), or where cache files would
   live (presumably `$XDG_CACHE_HOME/dotfiles/`).

4. **Undocumented redundant operations in zshrc**: The zshrc file calls `compinit` three times
   (lines 94, 416, 431), has duplicate Docker completion fpath additions (lines 353 and 429), and
   has third-party appended code (pnpm, Antigravity, Docker) that duplicates work done in common
   modules. There is no documentation warning about this.

5. **No documentation of `is_macos()`/`is_linux()` subshell cost**: These convenience functions call
   `$(detect_os)` on every invocation (spawning a subshell each time) rather than checking the
   cached `$DOTFILES_OS` variable. This pattern is used dozens of times across aliases.sh, paths.sh,
   exports.sh, and tools.sh. No documentation flags this as a performance concern.

6. **Missing `docs/development/` directory**: The documentation strategy references planned guides
   for `docs/development/contributing.md`, `docs/development/adding-tools.md`, and
   `docs/development/testing.md` -- none of these exist yet. A testing guide would be essential for
   validating optimization changes.

7. **No documentation of the `eval "$(tool init)"` cost inventory**: Multiple tools use eval-based
   initialization (starship, direnv, atuin, mise, zoxide, thefuck, carapace) but there is no
   comparative analysis of their costs or documentation about which could use cached output.

8. **Outdated references in zshrc**: Several sections in zshrc have comments like "Note: NVM setup
   is now handled by common/tools.sh / This is kept for immediate compatibility" and "Note: FZF
   setup is now handled by common/tools.sh" -- suggesting legacy code that may cause duplicate work.
   No documentation tracks which "immediate compatibility" blocks can be removed.

## External References Found in Code

- XDG Base Directory Specification:
  https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html (referenced in
  `exports.sh`)
- mise version manager docs: https://mise.jdx.dev/ (referenced in `tools.sh`)
- Carapace universal completion engine: https://github.com/rsteube/carapace (referenced in
  `completions.sh`)
- Starship prompt: https://starship.rs/ (referenced in `starship-bootstrap.md`)
- Go module proxy: https://proxy.golang.org (referenced in `exports.sh` and `paths.sh`)

## Key Optimization Opportunities Identified in Documentation/Code

These are patterns found during the documentation research that represent concrete optimization
targets:

1. **Platform detection subshell storm**: `setup_platform_env()` in `platform.sh` forks 8+
   subshells. The `is_macos()`/`is_linux()` helpers re-detect on every call instead of reading
   `$DOTFILES_OS`.
2. **Triple `compinit` in zshrc**: Lines 94, 416, and 431 each call
   `autoload -Uz compinit && compinit`. This is one of the most expensive zsh operations.
3. **Uncached `eval "$(tool init)"` calls**: At least 7 tools (starship, direnv, atuin, mise,
   zoxide, thefuck, carapace) generate shell code at startup. These outputs rarely change and could
   be cached.
4. **PATH cleanup pipelines in zshrc**: Lines 374-406 run multiple `grep | awk | sed` pipelines to
   clean PATH. This work duplicates `cleanup_path()` and `cleanup_unwanted_paths()` already called
   in `paths.sh`.
5. **Secret file reads via `$(cat file)`**: Lines 123-148 of zshrc fork subshells to read API keys.
   Could use `read` builtin or `<file` redirection instead.
6. **`has_command` subshell cost**: `has_command()` uses `command -v "$1" &> /dev/null` which is
   efficient, but it is called dozens of times across modules. Results could be cached for
   frequently-checked tools.
7. **`yarn global dir` in paths.sh**: The `setup_node_env()` function calls
   `yarn global dir 2>/dev/null` which spawns yarn and is notably slow.
8. **Duplicate fpath/completion setup**: Docker completions fpath is added both in zshrc (line 353)
   and by appended code (line 429).
