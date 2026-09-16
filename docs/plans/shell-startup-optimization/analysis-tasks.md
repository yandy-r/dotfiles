# Task Structure Analysis: shell-startup-optimization

## Executive Summary

The shell startup optimization should be structured in four phases: a benchmarking/infrastructure
phase to establish measurable baselines, a high-impact quick-wins phase targeting the 86% compinit
bottleneck and subshell elimination, a caching/lazy-loading phase for eval calls and redundancy
removal, and a cleanup/validation phase for zshrc inline code and documentation. The work decomposes
into 14-16 discrete tasks across 12 source files, with significant parallelism possible within each
phase since most modules are independently editable. The critical path runs through platform.sh
(foundation for all other modules) and the compinit consolidation in zshrc (single largest win at
~1960ms).

## Recommended Phase Structure

### Phase 1: Benchmarking Infrastructure and Foundation

**Purpose**: Establish measurable baselines and build the profiling infrastructure needed to
validate every subsequent optimization. Without this, there is no way to prove optimizations
actually improve startup time.

**Suggested Tasks**:

1. **Create startup profiling script** -- Build a `bin/benchmark-startup.sh` script that measures
   wall-clock time for both `zsh -i -c exit` and `bash -i -c exit`, plus per-module timing using
   `DOTFILES_DEBUG` hooks. This gives before/after measurements for every change.
2. **Add per-module timing instrumentation** -- Add optional timing hooks (gated behind
   `DOTFILES_DEBUG` or a new `DOTFILES_PROFILE=1` variable) to each common module's INITIALIZATION
   section. Record timestamps before/after each `setup_*()` call in `tools.sh`.
3. **Add load guard to platform.sh** -- Add `DOTFILES_PLATFORM_LOADED` guard variable to prevent
   `setup_platform_env()` from running twice when both `zprofile` and `zshrc` source it. This is a
   prerequisite for Phase 2 since platform.sh is the foundation module.

**Parallelization**: Tasks 1 and 2 can run in parallel. Task 3 can also run in parallel with both
since it modifies a different file (platform.sh) than the profiling infrastructure.

**Files involved**:

- `bin/benchmark-startup.sh` (new)
- `common/platform.sh` (guard addition)
- `common/tools.sh` (timing hooks)

---

### Phase 2: High-Impact Quick Wins (compinit + subshell elimination)

**Purpose**: Capture the largest performance gains by addressing the two dominant bottlenecks:
triple compinit (86% of zprof time) and subshell-spawning platform detection functions (called 36+
times). These two changes alone should reduce zsh startup by 60-80%.

**Suggested Tasks**:

1. **Consolidate compinit to a single cached call** -- Remove the three `compinit` calls at zshrc
   lines 94, 416, and 431. Replace with a single `compinit -C` call that uses a zcompdump file with
   a daily age check. Move all `fpath` additions (Homebrew, Docker, rfmedia's `~/.zfunc`) BEFORE the
   single compinit call. Remove the appended third-party blocks at lines 414-432 and integrate their
   functionality properly.
   - **Files**: `zsh/zshrc`
   - **Estimated savings**: 1300-1800ms (two compinit calls eliminated, third upgraded to cached)

2. **Rewrite is_macos/is_linux/is_wsl/is_arm64/is_x86_64 to use cached variables** -- Replace
   `[[ "$(detect_os)" == "macos" ]]` with `[[ "$DOTFILES_OS" == "macos" ]]` in all platform check
   functions (platform.sh lines 337-360). This eliminates 36+ subshell forks across common modules.
   - **Files**: `common/platform.sh`
   - **Estimated savings**: 70-120ms (subshell elimination across all callers)

3. **Eliminate subshells from setup_platform_env()** -- Refactor the 8+ `$(detect_*)` command
   substitutions into direct variable assignments. Instead of `export DOTFILES_OS="$(detect_os)"`,
   call `detect_os` as a function that sets the variable directly (e.g.,
   `detect_os; export DOTFILES_OS="$_DETECTED_OS"` or rewrite detect functions to set globals).
   - **Files**: `common/platform.sh`
   - **Estimated savings**: 50-150ms

4. **Replace PATH cleanup pipelines with shell builtins** -- Replace the `tr | grep | awk | sed`
   pipelines in zshrc lines 374-406 with zsh's `typeset -U path` for deduplication and pure
   parameter expansion for filtering. For bash compatibility, use a shell-native loop.
   - **Files**: `zsh/zshrc`, potentially `common/paths.sh`
   - **Estimated savings**: 50-100ms

**Parallelization**: Tasks 1 and 4 both modify `zsh/zshrc` so they must be sequential or carefully
coordinated (task 1 should go first since it removes lines that task 4 references). Tasks 2 and 3
both modify `common/platform.sh` and should be combined into a single task or done sequentially.
Task 4 can run in parallel with tasks 2/3.

**Dependencies**: Phase 1 Task 3 (platform.sh load guard) should complete before Tasks 2/3 here
since they all modify the same file. Benchmark infrastructure from Phase 1 should be available to
measure gains.

---

### Phase 3: Eval Caching and Redundancy Elimination

**Purpose**: Cache the output of expensive `eval "$(tool init ...)"` calls and eliminate all
redundant operations (duplicate sourcing, duplicate mkdir, duplicate fpath additions). This phase
extends the existing lazy-completions.sh pattern to tool initialization.

**Suggested Tasks**:

1. **Build eval caching infrastructure** -- Create a `common/eval-cache.sh` module (or add to an
   existing module) that provides a `cached_eval` function. This function checks
   `$XDG_CACHE_HOME/dotfiles/eval-cache/<tool>-<version>.sh`, regenerates if stale (based on tool
   binary mtime or version string), and sources the cached file. The cache key should include the
   tool version and the shell type (zsh/bash).
   - **Files**: `common/eval-cache.sh` (new)
   - **Pattern reference**: Extends the lazy-completions.sh wrapper pattern

2. **Apply eval caching to all tool init calls in zshrc** -- Replace inline
   `eval "$(starship init zsh)"`, `eval "$(direnv hook zsh)"`, `eval "$(atuin init zsh)"`,
   `eval "$(op completion zsh)"` in zshrc with `cached_eval` calls. Lazy-load or cache
   `thefuck --alias` (200-500ms savings alone).
   - **Files**: `zsh/zshrc`
   - **Dependencies**: Task 1 (eval-cache infrastructure)

3. **Apply eval caching to tool init calls in tools.sh** -- Replace
   `eval "$(mise activate zsh/bash)"` and `eval "$(zoxide init zsh/bash)"` in their respective
   `setup_*()` functions with `cached_eval` calls.
   - **Files**: `common/tools.sh`
   - **Dependencies**: Task 1 (eval-cache infrastructure)

4. **Apply eval caching to bash entry points** -- Replace `eval "$(starship init bash)"`,
   `eval "$(direnv hook bash)"`, `eval "$(atuin init bash)"` in bashrc and bash_profile with
   `cached_eval` calls. Also add guard to prevent double direnv hook in bash_profile + bashrc.
   - **Files**: `bash/bashrc`, `bash/bash_profile`
   - **Dependencies**: Task 1 (eval-cache infrastructure)

5. **Deduplicate cargo/env sourcing** -- Add a `DOTFILES_CARGO_LOADED` guard variable. Source
   `~/.cargo/env` exactly once (in paths.sh where it logically belongs). Remove duplicate sourcing
   from tools.sh, bashrc, bash_profile.
   - **Files**: `common/paths.sh`, `common/tools.sh`, `bash/bashrc`, `bash/bash_profile`
   - **No dependency on eval-cache**

6. **Deduplicate carapace initialization** -- Remove carapace init from `zshrc` line 235-239 and
   `completions.sh` line 546-557. Keep it only in `lazy-completions.sh` which already wraps it
   properly.
   - **Files**: `zsh/zshrc`, `common/completions.sh`
   - **No dependency on eval-cache**

7. **Deduplicate XDG mkdir and miscellaneous redundancies** -- Remove duplicate `mkdir -p` for XDG
   directories (keep in platform.sh, remove from exports.sh). Remove duplicate Docker fpath addition
   in zshrc. Remove duplicate FZF setup paths.
   - **Files**: `common/exports.sh`, `zsh/zshrc`
   - **No dependency on eval-cache**

**Parallelization**: Task 1 is a prerequisite for Tasks 2, 3, and 4. However, Tasks 5, 6, and 7 have
no dependency on the eval-cache and can run in parallel with Task 1 and with each other. Tasks 2, 3,
and 4 can run in parallel with each other once Task 1 completes (they modify different files).

**Dependencies**: Phase 2 should be substantially complete, especially the compinit consolidation
(Task 1) and platform detection rewrite (Tasks 2/3), since those changes affect the same files
modified here.

---

### Phase 4: Cleanup, Integration Testing, and Documentation

**Purpose**: Clean up the zshrc appended third-party blocks, eliminate remaining subshells in
exports.sh, validate all optimizations with before/after benchmarks, and update documentation to
reflect the new architecture.

**Suggested Tasks**:

1. **Eliminate subshells in exports.sh** -- Replace `$(command -v python3)` with
   `has_command python3 && ...`, replace `$(tty)` with a conditional, replace `$(nproc)` /
   `$(sysctl)` with cached or conditional assignment. Replace `$(cat file)` subshells for secret
   loading in zshrc with `read` builtin or `<file` redirection.
   - **Files**: `common/exports.sh`, `zsh/zshrc`

2. **Clean up zshrc appended third-party blocks** -- Properly integrate the pnpm, Antigravity,
   Docker Desktop, and rfmedia blocks (lines 414-432) into the common module system. Move PATH
   additions to `common/paths.sh`, fpath additions before the single compinit, and remove the blocks
   from zshrc.
   - **Files**: `zsh/zshrc`, `common/paths.sh`
   - **Dependencies**: Phase 2 Task 1 (compinit consolidation) must be complete

3. **Optimize has_command aggregate cost** -- Consider caching results of frequently-checked
   commands in an associative array during platform detection, or batch-checking tools. This
   addresses the ~170 `command -v` lookups across modules.
   - **Files**: `common/platform.sh`, potentially `common/aliases.sh`
   - **Risk**: Medium -- this is architecturally complex and may not yield proportional gains

4. **Run full benchmark suite and document results** -- Execute before/after benchmarks for zsh and
   bash. Document per-module timing, total startup time, and remaining bottlenecks. Update
   `docs/features/shell-configuration.md` troubleshooting section.
   - **Files**: `docs/features/shell-configuration.md`, benchmark output files

5. **Update architecture documentation** -- Update `docs/architecture/overview.md`,
   `docs/architecture/common-system.md`, and `common/README.md` to reflect eval-cache system,
   consolidated compinit, and new guard variables.
   - **Files**: Multiple documentation files

**Parallelization**: Tasks 1, 2, and 3 can run in parallel (they touch different areas). Tasks 4 and
5 should run after all code changes are complete.

**Dependencies**: All previous phases should be complete before Task 4 (final benchmarking).

---

## Task Granularity Recommendations

### Appropriate Task Sizes

- **1 file tasks**: Platform detection rewrite (platform.sh only), cargo/env dedup guard (touches 4
  files but same simple change), XDG mkdir dedup (2 files, trivial)
- **2-3 file tasks**: compinit consolidation (zshrc), eval-cache application per entry point (zshrc,
  bashrc, bash_profile each), PATH cleanup rewrite (zshrc + paths.sh)
- **Single new file tasks**: eval-cache.sh infrastructure, benchmark-startup.sh script

### Tasks to Split

- **Phase 2 Tasks 2+3 vs Phase 1 Task 3**: All three modify `common/platform.sh`. While logically
  distinct (load guard vs function rewrite vs subshell elimination), they should either be executed
  sequentially by the same agent or combined into a single "platform.sh optimization" mega-task to
  avoid merge conflicts.
- **Phase 3 Tasks 2+3+4** (apply eval caching): These are currently split by file (zshrc, tools.sh,
  bashrc). This is correct granularity -- do NOT combine them since they touch independent files.

### Tasks to Combine

- **Phase 2 Tasks 2+3** (rewrite is_macos + eliminate setup_platform_env subshells): These both
  modify `common/platform.sh` and are tightly coupled. Combine into a single "Optimize platform.sh
  detection" task.
- **Phase 3 Tasks 5+6+7** (deduplicate cargo, carapace, XDG): These are all simple redundancy
  removals. Could be combined into a single "Eliminate redundant operations" task since each is
  small (1-3 line changes per file), though they are safe to parallelize since they touch different
  lines in different files.
- **Phase 4 Tasks 4+5** (benchmarking + documentation): Could be combined since documentation should
  reflect benchmark results.

---

## Dependency Analysis

### Independent Tasks (Can Run in Parallel)

- Phase 1 Task 1 (benchmark script) -- File: `bin/benchmark-startup.sh` (new)
- Phase 1 Task 2 (timing instrumentation) -- File: `common/tools.sh`
- Phase 1 Task 3 (platform.sh load guard) -- File: `common/platform.sh`
- Phase 3 Task 5 (cargo/env dedup) -- Files: `common/paths.sh`, `common/tools.sh`, `bash/bashrc`,
  `bash/bash_profile`
- Phase 3 Task 6 (carapace dedup) -- Files: `zsh/zshrc`, `common/completions.sh`
- Phase 3 Task 7 (XDG mkdir dedup) -- Files: `common/exports.sh`, `zsh/zshrc`

### Sequential Dependencies

- **Phase 1 Task 3 -> Phase 2 Tasks 2/3**: Platform.sh load guard should be added before the
  function rewrite to avoid conflicting changes.
- **Phase 2 Task 1 -> Phase 2 Task 4**: compinit consolidation must happen before PATH cleanup
  rewrite because the cleanup code references lines that will be removed/moved.
- **Phase 2 Task 1 -> Phase 4 Task 2**: Third-party block cleanup depends on compinit consolidation
  since those blocks contain redundant compinit calls.
- **Phase 3 Task 1 -> Phase 3 Tasks 2/3/4**: Eval-cache infrastructure must exist before it can be
  applied to any entry point.
- **All code tasks -> Phase 4 Task 4**: Final benchmarking must wait for all optimizations.

### Potential Bottlenecks

- **`common/platform.sh`**: Modified in Phase 1 (load guard), Phase 2 (function rewrite + subshell
  elimination), and Phase 4 (has_command caching). This file is on the critical path for 3 separate
  tasks. Recommend combining all platform.sh work into fewer, larger tasks.
- **`zsh/zshrc`**: Modified in Phase 2 (compinit, PATH cleanup), Phase 3 (eval caching, carapace
  dedup, XDG dedup), and Phase 4 (subshell elimination, third-party cleanup). This is the
  most-modified file. Tasks touching it should be carefully ordered.
- **Phase 3 Task 1 (eval-cache infrastructure)**: Three downstream tasks depend on it. This should
  be prioritized early in Phase 3.

---

## File-to-Task Mapping

### Files to Create

| File                       | Suggested Task              | Phase   | Dependencies                          |
| -------------------------- | --------------------------- | ------- | ------------------------------------- |
| `bin/benchmark-startup.sh` | Profiling script            | Phase 1 | None                                  |
| `common/eval-cache.sh`     | Eval caching infrastructure | Phase 3 | None (but enables 3 downstream tasks) |

### Files to Modify

| File                                   | Suggested Task                                                   | Phase             | Dependencies                   |
| -------------------------------------- | ---------------------------------------------------------------- | ----------------- | ------------------------------ |
| `common/platform.sh`                   | Load guard + function rewrite + subshell elimination             | Phase 1 + Phase 2 | None (foundation module)       |
| `zsh/zshrc`                            | compinit consolidation                                           | Phase 2           | Phase 1 platform.sh guard      |
| `zsh/zshrc`                            | PATH cleanup rewrite                                             | Phase 2           | Phase 2 compinit consolidation |
| `zsh/zshrc`                            | Apply eval caching (starship, direnv, atuin, thefuck, op)        | Phase 3           | Phase 3 eval-cache.sh          |
| `zsh/zshrc`                            | Carapace dedup, XDG dedup, secret read optimization              | Phase 3/4         | Phase 2 compinit consolidation |
| `zsh/zshrc`                            | Third-party block cleanup (pnpm, Docker, rfmedia, Antigravity)   | Phase 4           | Phase 2 compinit consolidation |
| `common/tools.sh`                      | Timing hooks + apply eval caching (mise, zoxide)                 | Phase 1 + Phase 3 | Phase 3 eval-cache.sh          |
| `common/tools.sh`                      | Remove redundant cargo/env sourcing                              | Phase 3           | None                           |
| `common/exports.sh`                    | Remove duplicate XDG mkdir + eliminate subshells                 | Phase 3/4         | None                           |
| `common/paths.sh`                      | cargo/env guard + third-party PATH additions                     | Phase 3/4         | None                           |
| `common/completions.sh`                | Remove duplicate carapace init                                   | Phase 3           | None                           |
| `bash/bashrc`                          | Apply eval caching + remove duplicate cargo/env                  | Phase 3           | Phase 3 eval-cache.sh          |
| `bash/bash_profile`                    | Remove duplicate direnv hook + cargo/env                         | Phase 3           | None                           |
| `zsh/zprofile`                         | No changes needed (guard in platform.sh handles the double-load) | N/A               | N/A                            |
| `common/lazy-completions.sh`           | No changes needed (already optimized, serves as reference)       | N/A               | N/A                            |
| `docs/features/shell-configuration.md` | Update troubleshooting with profiling info                       | Phase 4           | All code complete              |
| `docs/architecture/overview.md`        | Document eval-cache system                                       | Phase 4           | All code complete              |
| `docs/architecture/common-system.md`   | Document new guard variables                                     | Phase 4           | All code complete              |

---

## Optimization Opportunities

### Maximize Parallelism

- **Phase 1 is fully parallel**: All three tasks touch different files (new script, tools.sh,
  platform.sh).
- **Phase 2 allows 2 parallel tracks**: platform.sh work (Tasks 2+3 combined) and zshrc work (Tasks
  1 then 4 sequentially).
- **Phase 3 allows 4 parallel tracks**: eval-cache.sh creation, cargo/env dedup, carapace dedup, and
  XDG dedup can all run simultaneously. Once eval-cache.sh is done, 3 more tasks (applying it to
  zshrc, tools.sh, bashrc) can run in parallel.
- **Phase 4 allows 3 parallel tracks initially**: exports.sh subshells, zshrc third-party cleanup,
  and has_command optimization. Then documentation after all code is done.

### Minimize Risk

- **compinit consolidation (Phase 2 Task 1)** is the highest-risk, highest-reward change. Moving all
  fpath additions before a single compinit call could break completions for tools that add their
  fpath entries later in the startup sequence. Requires careful testing of all completion-dependent
  tools (kubectl, docker, helm, aws, gcloud, terraform, 1Password).
- **Platform detection rewrite (Phase 2 Tasks 2/3)** affects every downstream module. If
  `$DOTFILES_OS` is empty or wrong, the entire configuration system breaks. Must verify the variable
  is set before any caller uses it.
- **Eval-cache staleness** could cause tools to use outdated init scripts after version upgrades.
  The cache invalidation strategy (mtime or version check) must be robust.
- **Third-party block removal (Phase 4 Task 2)** could break Docker completions or pnpm PATH if
  their additions are not properly relocated. Test thoroughly.

---

## Implementation Strategy Recommendations

- **Bottom-up approach**: Start with platform.sh (the foundation all modules depend on), then work
  outward to tools.sh, then to the shell entry points (zshrc, bashrc). This ensures each layer is
  stable before its consumers are modified.
- **Benchmark at every phase boundary**: Run the profiling script after completing each phase to
  validate gains and catch regressions. Do not proceed to the next phase without confirming the
  expected improvements materialized.
- **Test both shells after every change**: Even changes that appear zsh-specific (compinit) may
  affect bash due to shared modules. Always verify both `bash -i -c exit` and `zsh -i -c exit` work
  correctly after each task.
- **Preserve rollback capability**: Each phase should be a separate git branch or series of commits.
  If a phase introduces regressions, it should be independently revertable without losing gains from
  other phases.
- **compinit first, everything else second**: The triple compinit is 86% of profiled time. If only
  one task gets done, it should be compinit consolidation. All other optimizations combined are
  worth less than this single change.
- **Do not optimize has_command aggressively**: The ~170 `command -v` calls cost 50-100ms total.
  This is low-priority compared to compinit (1960ms) and eval caching (400-800ms). An associative
  array cache adds architectural complexity for diminishing returns. Consider deferring to a future
  iteration.
- **Test in tmux**: Login shells (tmux, SSH) follow a different startup path (zprofile -> zshrc).
  The platform.sh load guard is critical for these scenarios. Always test `tmux new-session` to
  verify no regressions in login shell startup.
