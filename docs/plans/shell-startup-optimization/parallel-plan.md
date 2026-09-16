# Shell Startup Optimization Implementation Plan

Zsh startup takes ~3.2s due to three redundant `compinit` calls consuming ~1960ms (86% of profiled
time), 7+ uncached `eval "$(tool init ...)"` calls, 36 subshell-spawning `is_macos`/`is_linux`
invocations across the 6-module common pipeline, ~170 `has_command` PATH lookups, duplicate
`cargo/env` sourcing, and PATH cleanup pipelines using external tools. The approach is four phases:
(1) benchmarking infrastructure and platform.sh foundation fixes, (2) high-impact quick wins
targeting compinit and subshell elimination for 60-80% of total gains, (3) eval caching
infrastructure extending the proven `lazy-completions.sh` wrapper pattern, and (4) cleanup of zshrc
duplications and documentation. Bash (~0.45s) benefits incidentally from shared module optimizations
but is not the primary target.

## Critically Relevant Files and Documentation

- `zsh/zshrc`: Primary optimization target (432 lines). Triple compinit at lines 94, 416, 431. Seven
  eval calls. PATH cleanup pipelines. Secret reads via `$(cat)`. Appended third-party blocks.
- `zsh/zprofile`: Login shell pre-loader. Sources platform.sh and paths.sh before zshrc duplicates
  the same.
- `common/platform.sh`: Platform detection engine. `setup_platform_env()` spawns 8+ subshells.
  `is_macos()`/`is_linux()` bypass cached `$DOTFILES_OS`, re-detecting via `uname -s` on every call.
- `common/tools.sh`: Tool orchestrator (780 lines). `setup_all_tools()` triggers 12+ setup functions
  with command checks and eval calls.
- `common/paths.sh`: PATH construction (743 lines). `is_macos`/`is_linux` subshell calls, slow
  `yarn global dir`, dedup pipelines. Has `DOTFILES_PATHS_LOADED` guard.
- `common/exports.sh`: Environment variables. Redundant `mkdir -p`, plus `$(command -v python3)`,
  `$(tty)`, `$(nproc)` subshells at source time.
- `common/aliases.sh`: 688 lines with ~23 `has_command` checks and 7 `is_macos`/`is_linux` calls at
  source time.
- `common/lazy-completions.sh`: Reference implementation for lazy loading via function-wrapper
  pattern. The template for eval caching.
- `common/completions.sh`: Eager completion loading. Bash exits early; zsh loads everything with 51+
  `command -v` checks.
- `bash/bashrc`: Bash entry point (195 lines). Shares same 6-module pipeline. Has
  `DOTFILES_BASHRC_LOADED` guard.
- `bash/bash_profile`: Duplicates cargo/env sourcing and direnv hook from bashrc.
- `docs/architecture/common-system.md`: Module dependency graph, disable flags, API reference.
- `docs/features/shell-configuration.md`: Shell setup guide with startup troubleshooting.

## Implementation Plan

### Phase 1: Benchmarking Infrastructure and Foundation

#### Task 1.1: Create startup benchmarking script Depends on [none]

**READ THESE BEFORE TASK**

- `common/tools.sh` (the `system_health` function pattern at lines 733-770)
- `docs/features/shell-configuration.md` (existing troubleshooting section)

**Instructions**

Files to Create

- `bin/benchmark-startup.sh`

Create a benchmarking script that:

- Measures wall-clock time for `zsh -i -c exit` and `bash -i -c exit` (10 iterations, report
  min/avg/max)
- Supports `--verbose` flag for per-module timing via `DOTFILES_PROFILE=1` environment variable
- Outputs results in both human-readable table and machine-parseable CSV
- Includes a `--compare` mode that stores baseline results and diffs against them
- Make executable (`chmod +x`)
- Use the existing section header convention
  (`# ================================================================` borders with ALL CAPS
  titles)

#### Task 1.2: Add per-module timing instrumentation to tools.sh Depends on [none]

**READ THESE BEFORE TASK**

- `common/tools.sh` (the `setup_all_tools()` function and its `setup_*()` calls, lines 733-770)
- `common/platform.sh` (the INITIALIZATION section pattern at bottom of file)

**Instructions**

Files to Modify

- `common/tools.sh`

Add optional timing hooks gated behind `DOTFILES_PROFILE=1`:

- At the top of `setup_all_tools()`, check for `DOTFILES_PROFILE` and if set, record
  `$EPOCHREALTIME` (zsh) or `date +%s%N` (bash) before/after each `setup_*()` call
- Store per-function timing in an array or print to stderr with `[PROFILE]` prefix
- Add a `dotfiles_profile_report()` function that summarizes all recorded timings
- Do NOT add timing outside of `setup_all_tools()` — other modules will get their own
  instrumentation in Phase 4
- Guard behind `DOTFILES_PROFILE` so there is zero cost when profiling is disabled

#### Task 1.3: Add load guard to platform.sh and optimize detection functions Depends on [none]

**READ THESE BEFORE TASK**

- `common/platform.sh` (full file — understand `setup_platform_env()`, `detect_os()`, `is_macos()`,
  `is_linux()`, and all convenience functions)
- `common/paths.sh` (lines 731-743 for the `DOTFILES_PATHS_LOADED` guard pattern)
- `zsh/zprofile` (sources platform.sh before zshrc does the same)

**Instructions**

Files to Modify

- `common/platform.sh`

Three changes in one task (all same file, tightly coupled):

1. **Add `DOTFILES_PLATFORM_LOADED` guard**: Wrap the auto-init section at file bottom with
   `if [[ -z "${DOTFILES_PLATFORM_LOADED}" ]]; then DOTFILES_PLATFORM_LOADED=1; ... fi`. This
   prevents `setup_platform_env()` from running twice on login shells (zprofile + zshrc).

2. **Rewrite convenience functions to use cached variables**: Replace subshell-spawning
   implementations:

   ```bash
   # BEFORE: is_macos() { [[ "$(detect_os)" == "macos" ]]; }
   # AFTER:  is_macos() { [[ "$DOTFILES_OS" == "macos" ]]; }
   ```

   Apply to all 6 functions: `is_macos`, `is_linux`, `is_wsl`, `is_container`, `is_arm64`,
   `is_x86_64`.

3. **Eliminate subshells from `setup_platform_env()`**: Refactor `detect_os`, `detect_arch`,
   `detect_distro`, `detect_wsl`, `detect_container` to set global variables directly instead of
   echoing values captured by `$(...)`. Replace `export DOTFILES_OS="$(detect_os)"` with calling the
   function then exporting. Also fix `get_homebrew_path()`, `get_copy_command()`,
   `get_paste_command()`, `get_open_command()` to use `$DOTFILES_OS` directly instead of calling
   `detect_os` again.

Maintain the `DOTFILES_NO_AUTO_PLATFORM` disable flag for testing. Ensure all functions still work
when sourced directly (not just via auto-init).

### Phase 2: High-Impact Quick Wins

#### Task 2.1: Consolidate compinit to single cached call Depends on [1.3]

**READ THESE BEFORE TASK**

- `zsh/zshrc` (full file — identify all three `compinit` calls at lines 94, 416, 431, all `fpath`
  modifications, and the appended third-party blocks at lines 414-432)
- `common/lazy-completions.sh` (understand what tools add to fpath)
- `common/completions.sh` (understand how completions interact with compinit)

**Instructions**

Files to Modify

- `zsh/zshrc`

This is the single highest-impact change (~1300-1800ms savings):

1. **Collect all fpath additions before compinit**: Move Docker Desktop's `~/.docker/completions`
   fpath addition (from appended block ~line 431) and rfmedia's `~/.zfunc` fpath addition (from
   appended block ~line 416) to the fpath setup section BEFORE the first compinit.

2. **Replace triple compinit with single cached call**: Remove all three `compinit` calls. Add a
   single `compinit -C` call that uses the existing zcompdump file. Add a daily freshness check:

   ```zsh
   autoload -Uz compinit
   if [[ -n ${ZDOTDIR:-$HOME}/.zcompdump(#qN.mh+24) ]]; then
       compinit
   else
       compinit -C
   fi
   ```

3. **Remove appended third-party blocks**: Delete the rfmedia block (~lines 414-418), pnpm block,
   Docker Desktop block (~lines 427-432), and Antigravity block. Integrate their necessary
   functionality (fpath additions, PATH additions) into the proper sections above. The pnpm PATH
   addition should go in the PATH setup section. The Docker fpath addition goes in step 1.

4. **Remove duplicated tool init calls**: Delete the following from zshrc that are already handled
   by `common/tools.sh` or `common/completions.sh`:
   - Lines 109-116 (vault/azure completions)
   - Lines 128-131 (op completion)
   - Lines 154-163 (cdktf completions)
   - Lines 192-197 (fzf setup)
   - Lines 206-220 (python_venv — already in tools.sh)
   - Lines 235-239 (carapace — already in lazy-completions.sh)
   - Lines 248-295 (aliases — already in common/aliases.sh)

Test thoroughly: `zsh -i -c 'which kubectl && which docker && which helm'` to verify completions
still work.

#### Task 2.2: Replace PATH cleanup pipelines with shell builtins Depends on [2.1]

**READ THESE BEFORE TASK**

- `zsh/zshrc` (PATH cleanup section, after Task 2.1 changes — the line numbers will have shifted)
- `common/paths.sh` (the `cleanup_path()` function and `typeset -U` usage)

**Instructions**

Files to Modify

- `zsh/zshrc`
- `common/paths.sh`

1. **In zshrc**: Replace the 4 `echo | tr | grep | awk | sed` pipeline chains (currently in the PATH
   cleanup section) with:

   ```zsh
   typeset -U path  # Zsh built-in dedup
   ```

   For any filtering (removing specific patterns), use a pure-shell loop:

   ```zsh
   local -a clean_path=()
   for p in "${path[@]}"; do
       [[ -d "$p" && "$p" != */removed_pattern* ]] && clean_path+=("$p")
   done
   path=("${clean_path[@]}")
   ```

2. **In paths.sh**: If the `cleanup_path()` function uses external tools, add a zsh-specific fast
   path using `typeset -U path` and keep the bash fallback.

This eliminates ~16 subshell forks from PATH cleanup alone.

#### Task 2.3: Replace secret file reads with shell builtins Depends on [2.1]

**READ THESE BEFORE TASK**

- `zsh/zshrc` (lines 123-148 where API keys are read via `$(cat)`)

**Instructions**

Files to Modify

- `zsh/zshrc`

Replace all `$(cat ~/.config/secrets/*)` subshells with the `read` builtin:

```bash
# BEFORE: export API_KEY="$(cat ~/.config/secrets/api-key 2>/dev/null)"
# AFTER:
if [[ -r ~/.config/secrets/api-key ]]; then
    read -r API_KEY < ~/.config/secrets/api-key
    export API_KEY
fi
```

Apply to all 4 secret file reads. Each replacement eliminates one subshell fork (~5-10ms each,
~20-40ms total).

### Phase 3: Eval Caching and Redundancy Elimination

#### Task 3.1: Build eval caching infrastructure Depends on [1.3]

**READ THESE BEFORE TASK**

- `common/lazy-completions.sh` (the function-wrapper pattern with state variables — this is the
  reference implementation to extend)
- `common/tools.sh` (the `setup_*()` functions that currently call `eval`)
- `common/platform.sh` (the guard variable and auto-init patterns)

**Instructions**

Files to Create

- `common/eval-cache.sh`

Create a new module that provides `cached_eval()`:

```bash
# Usage: cached_eval <tool-name> <shell-type> <command...>
# Example: cached_eval starship zsh starship init zsh
cached_eval() {
    local tool="$1" shell_type="$2"
    shift 2
    local cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/dotfiles/eval-cache"
    local cache_file="${cache_dir}/${tool}-${shell_type}.sh"
    local tool_path
    tool_path="$(command -v "$tool" 2>/dev/null)" || return 0

    # Invalidate if tool binary changed (mtime comparison)
    if [[ -f "$cache_file" && "$cache_file" -nt "$tool_path" ]]; then
        source "$cache_file"
        return 0
    fi

    # Generate and cache
    mkdir -p "$cache_dir"
    "$@" > "$cache_file" 2>/dev/null
    source "$cache_file"
}
```

Also provide:

- `clear_eval_cache()` — removes all cached files
- `eval_cache_status()` — lists cached tools with ages
- Guard variable: `DOTFILES_EVAL_CACHE_LOADED`
- Auto-init section at bottom following standard pattern
- Shell-agnostic: must work in both bash and zsh

#### Task 3.2: Apply eval caching to zshrc tool init calls Depends on [3.1, 2.1]

**READ THESE BEFORE TASK**

- `common/eval-cache.sh` (the `cached_eval` function from Task 3.1)
- `zsh/zshrc` (remaining inline eval calls after Task 2.1 cleanup)

**Instructions**

Files to Modify

- `zsh/zshrc`

Replace remaining inline eval calls with `cached_eval`:

```bash
# BEFORE: eval "$(starship init zsh)"
# AFTER:  cached_eval starship zsh starship init zsh

# BEFORE: eval "$(direnv hook zsh)"
# AFTER:  cached_eval direnv zsh direnv hook zsh

# BEFORE: eval "$(atuin init zsh)"
# AFTER:  cached_eval atuin zsh atuin init zsh

# BEFORE: eval "$(thefuck --alias)"
# AFTER:  cached_eval thefuck zsh thefuck --alias
```

`thefuck --alias` is the biggest win here (200-500ms) since its output is stable.

Ensure `common/eval-cache.sh` is sourced before these calls (add to the module loading section near
the top of zshrc, or source it from tools.sh).

#### Task 3.3: Apply eval caching to tools.sh setup functions Depends on [3.1]

**READ THESE BEFORE TASK**

- `common/eval-cache.sh` (from Task 3.1)
- `common/tools.sh` (the `setup_zoxide()`, `setup_mise()`, `setup_direnv()`, `setup_atuin()`
  functions)

**Instructions**

Files to Modify

- `common/tools.sh`

Replace eval calls in setup functions:

```bash
# In setup_zoxide():
# BEFORE: eval "$(zoxide init zsh)"  / eval "$(zoxide init bash)"
# AFTER:  cached_eval zoxide "${shell_type}" zoxide init "${shell_type}"

# In setup_mise():
# BEFORE: eval "$(mise activate zsh)"
# AFTER:  cached_eval mise "${shell_type}" mise activate "${shell_type}"
```

Use `detect_current_shell()` or `${ZSH_VERSION:+zsh}` / `${BASH_VERSION:+bash}` for shell type.

Also source `eval-cache.sh` from within tools.sh (add `source "$SCRIPT_DIR/eval-cache.sh"` near the
top, before `setup_all_tools()`).

#### Task 3.4: Apply eval caching to bash entry points Depends on [3.1]

**READ THESE BEFORE TASK**

- `common/eval-cache.sh` (from Task 3.1)
- `bash/bashrc` (inline eval calls for starship, direnv, atuin)
- `bash/bash_profile` (duplicated direnv hook and cargo/env)

**Instructions**

Files to Modify

- `bash/bashrc`
- `bash/bash_profile`

1. In `bashrc`: Replace inline eval calls with `cached_eval` (same pattern as Task 3.2 but with
   `bash` as shell type).

2. In `bash_profile`: Remove duplicated `cargo/env` sourcing (lines 22-25, already sourced in bashrc
   via tools.sh). Remove duplicated `direnv hook bash` (lines 28-30, already in bashrc).

#### Task 3.5: Deduplicate cargo/env sourcing Depends on [1.3]

**READ THESE BEFORE TASK**

- `common/paths.sh` (the `setup_rust_env()` function where cargo/env is sourced)
- `common/tools.sh` (the `setup_rust()` function)
- `bash/bashrc` (cargo/env sourcing)
- `bash/bash_profile` (cargo/env sourcing)
- `common/completions.sh` (line 417 where cargo/env is sourced again)

**Instructions**

Files to Modify

- `common/paths.sh`
- `common/tools.sh`
- `common/completions.sh`

Add a `DOTFILES_CARGO_LOADED` guard variable:

1. In `paths.sh` `setup_rust_env()`: Add `DOTFILES_CARGO_LOADED=1` after sourcing `~/.cargo/env`.
2. In `tools.sh` `setup_rust()`: Check `[[ -z "$DOTFILES_CARGO_LOADED" ]]` before sourcing
   cargo/env.
3. In `completions.sh`: Check `[[ -z "$DOTFILES_CARGO_LOADED" ]]` before the cargo/env source at
   line 417.

This ensures cargo/env is sourced exactly once (in paths.sh where it logically belongs).

#### Task 3.6: Deduplicate carapace initialization and remove zshrc redundancies Depends on [2.1]

**READ THESE BEFORE TASK**

- `common/lazy-completions.sh` (carapace is already wrapped here)
- `common/completions.sh` (lines 546-557 duplicate carapace init)
- `zsh/zshrc` (after Task 2.1, verify what duplicate sections remain)

**Instructions**

Files to Modify

- `common/completions.sh`

Remove the duplicate carapace initialization block (lines 546-557) since `lazy-completions.sh`
already wraps carapace with the function-wrapper pattern. Keep the carapace completion setup only in
`lazy-completions.sh`.

### Phase 4: Cleanup, Validation, and Documentation

#### Task 4.1: Eliminate subshells in exports.sh Depends on [1.3]

**READ THESE BEFORE TASK**

- `common/exports.sh` (full file — identify all `$(...)` subshells at source time)
- `common/platform.sh` (where `mkdir -p` for XDG dirs already happens)

**Instructions**

Files to Modify

- `common/exports.sh`

1. Remove duplicate `mkdir -p` calls for XDG directories (already done in platform.sh
   `setup_xdg_dirs()`).
2. Replace `$(command -v python3)` with conditional:
   `if has_command python3; then PYTHON3_PATH="$(command -v python3)"; fi` (only compute if needed).
3. Replace `GPG_TTY=$(tty)` with: `[[ -t 0 ]] && GPG_TTY=$(tty) && export GPG_TTY` (skip if not a
   terminal).
4. Replace `$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1)` with a cached approach:
   compute once and store in `DOTFILES_NPROC`.
5. Remove `SCRIPT_DIR` subshell — replace with `"${DOTFILES_ROOT}/common"` since `DOTFILES_ROOT` is
   always set before exports.sh is sourced.

#### Task 4.2: Integrate third-party PATH additions into common system Depends on [2.1]

**READ THESE BEFORE TASK**

- `zsh/zshrc` (verify the appended blocks were removed in Task 2.1)
- `common/paths.sh` (where new PATH additions should go)

**Instructions**

Files to Modify

- `common/paths.sh`

After Task 2.1 removes the appended third-party blocks from zshrc, ensure their PATH additions are
properly placed:

1. Add pnpm PATH (`$PNPM_HOME`) to the Node.js/package manager section of paths.sh (conditional on
   pnpm existing).
2. Add Docker Desktop paths to the container tools section of paths.sh.
3. Remove any Antigravity-related PATH modifications that are no longer needed.

Use the existing `add_to_path()` or `append_path()` helpers. Gate behind `has_command` or directory
existence checks.

#### Task 4.3: Remove SCRIPT_DIR subshells across common modules Depends on [1.3]

**READ THESE BEFORE TASK**

- `common/exports.sh` (line 29)
- `common/aliases.sh` (line 32)
- `common/functions.sh` (line 35)
- `common/paths.sh` (SCRIPT_DIR computation)

**Instructions**

Files to Modify

- `common/aliases.sh`
- `common/functions.sh`
- `common/paths.sh`

Replace the `SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-${(%):-%x}}")" && pwd)"` subshell
computation with `SCRIPT_DIR="${DOTFILES_ROOT}/common"` in each module. Since `DOTFILES_ROOT` is
always set by both `zshrc` (line 13) and `bashrc` (line 30) before any common module is sourced,
this is safe.

This eliminates 4 subshell forks (one per module). Keep the original computation as a fallback for
edge cases:

```bash
SCRIPT_DIR="${DOTFILES_ROOT:+${DOTFILES_ROOT}/common}"
SCRIPT_DIR="${SCRIPT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]:-${(%):-%x}}")" && pwd)}"
```

#### Task 4.4: Run full benchmark suite and document results Depends on [4.1, 4.2, 4.3, 3.2, 3.3, 3.4, 3.5, 3.6, 2.2, 2.3]

**READ THESE BEFORE TASK**

- `bin/benchmark-startup.sh` (from Task 1.1)
- `docs/features/shell-configuration.md` (existing troubleshooting section to update)

**Instructions**

Files to Modify

- `docs/features/shell-configuration.md`

1. Run `bin/benchmark-startup.sh --compare` to measure post-optimization startup times for both zsh
   and bash.
2. Update the "Slow Startup Troubleshooting" section with:
   - New profiling commands: `DOTFILES_PROFILE=1 zsh -i -c exit`
   - Cache management: `clear_eval_cache` and `eval_cache_status`
   - New guard variables documented
3. Add a "Performance" subsection documenting the expected startup times and the optimization
   architecture.

## Advice

- **compinit is 86% of the problem**: If only one task gets done, Task 2.1 (compinit consolidation)
  delivers more savings than all other tasks combined. Prioritize it above everything else.
- **`zsh/zshrc` is the contention bottleneck**: Tasks 2.1, 2.2, 2.3, 3.2, and 3.6 all modify zshrc.
  Execute them strictly in dependency order. After Task 2.1, line numbers will shift dramatically —
  all subsequent zshrc tasks must reference the post-edit state.
- **`common/platform.sh` is the foundation**: Task 1.3 must complete before Tasks 2.1 and 3.1
  because the `is_macos`/`is_linux` rewrite affects every downstream module that calls these
  functions. Verify `$DOTFILES_OS` is correctly set before any caller uses the optimized functions.
- **Eval cache invalidation via mtime is imperfect**: When a tool is upgraded via package manager,
  the binary mtime changes but the cache will be regenerated on next shell start. However, if the
  tool's config changes (e.g., starship.toml), the cache won't invalidate. This is acceptable —
  `clear_eval_cache` is the manual escape hatch.
- **Test in both login and non-login shells**: Login shells (tmux, SSH) follow zprofile -> zshrc
  path. Non-login shells skip zprofile. The platform.sh guard (Task 1.3) is critical for login shell
  correctness. Always test with `tmux new-session -d -s test 'zsh -i -c exit'`.
- **The third-party appended blocks (lines 414-432 of zshrc) were not written by the user**: They
  were auto-appended by tool installers (Docker Desktop, pnpm, rfmedia). They can be safely removed
  and their functionality integrated into the common system. Don't be afraid to delete them.
- **`DOTFILES_PATHS_LOADED` is intentionally NOT exported**: This allows child shells to re-run path
  setup. Do not change this behavior. The new `DOTFILES_PLATFORM_LOADED` guard should follow the
  same convention (not exported).
- **`lazy-completions.sh` is the golden reference**: All lazy-loading patterns (eval caching,
  deferred tool init) should follow its wrapper-function approach. Study lines 41-61 before writing
  any lazy-loading code.
- **Avoid optimizing `has_command` aggregation in this iteration**: The ~170 `command -v` calls cost
  50-100ms total. An associative array cache adds architectural complexity for diminishing returns
  compared to compinit (1960ms) and eval caching (400-800ms). Defer to a future iteration.
