#!/usr/bin/env bash
# shellcheck shell=bash
# ================================================================
# Cached Shell-Init Output (bash + zsh)
# ================================================================
#
# Purpose:
#   Tools such as zoxide, atuin, starship, direnv and thefuck are initialized
#   with `eval "$(tool init <shell>)"`, which forks the tool on every shell
#   start even though its output only changes when the tool (or its config)
#   does. `_cached_eval` stores that output in a file and sources the file
#   instead, regenerating it only when the signature below changes.
#
# Usage:
#   _cached_eval [--dep FILE]... <name> <cmd> [args...]
#
#   _cached_eval zoxide zoxide init zsh
#   _cached_eval --dep "$HOME/.config/atuin/config.toml" atuin atuin init zsh
#
# Cache:
#   ${XDG_CACHE_HOME:-$HOME/.cache}/dotfiles/eval/<name>.<zsh|bash>
#   (directory mode 700). Line 1 is a header holding the signature:
#     # dotfiles-cached-eval v1 <shell> <binary> <argv> <metadata>
#   The signature covers the shell, the resolved absolute binary path, the full
#   argv, and the mtime + size + inode of the binary and of every --dep file
#   (symlinks followed; a missing dep is recorded as `missing`). Real file
#   metadata is used, not just `-nt`, because dpkg/brew upgrades keep the
#   package's mtimes. Output that depends on anything else (environment
#   variables, $PATH — e.g. `mise activate`) must NOT go through this helper.
#
# Behavior:
#   - Hit: the cache file is sourced (inside this function, so the init code
#     must not rely on top-level `local`/`typeset`; none of the cached tools do).
#   - Miss: the command runs once. Success with output -> written atomically
#     (temp file + mv) and sourced. Failure or empty output -> what it printed
#     is eval'd once, as `eval "$(cmd)"` would, any stale cache file is
#     removed, nothing is cached, and the command's status is returned.
#   - Trust: the cache dir must be owned by the current user and not group- or
#     world-writable, and so must the cache file. Otherwise the cache is never
#     sourced or written; the output is eval'd uncached on every start. A dir
#     this helper creates gets mode 700.
#   - Binary not on PATH -> returns 127 silently.
#
# Controls:
#   DOTFILES_NO_EVAL_CACHE=1     bypass the cache: plain `eval "$(cmd)"`
#   dotfiles_clear_eval_cache    delete every cached init file
# ================================================================

# _dotfiles_eval_cache_meta <sigvar> <weakvar> <dir> <file> <sigfile>...
# One metadata pass over the cache dir, the cache file and the signature files
# (binary + deps), symlinks followed:
#   <sigvar>  <- "<path>=<inode>:<size>:<mtime> " per signature file, or
#                "<path>=missing " when it does not exist
#   <weakvar> <- "dir"/"file" for each of <dir>/<file> that exists and is group-
#                or world-writable (empty when both are safe or absent)
# zsh uses the zstat builtin (no fork); bash makes ONE `stat -L` call. Locals use
# the __dotf_cem_ prefix so they never shadow the caller's target variables.
_dotfiles_eval_cache_meta() {
    local __dotf_cem_sigvar="$1" __dotf_cem_weakvar="$2"
    shift 2
    local __dotf_cem_f __dotf_cem_meta="" __dotf_cem_weak="" __dotf_cem_out __dotf_cem_line
    local __dotf_cem_rec __dotf_cem_mode __dotf_cem_i=0
    local -a __dotf_cem_present=()

    if [[ -n "${ZSH_VERSION:-}" ]] && zmodload -F zsh/stat b:zstat 2>/dev/null; then
        local -A __dotf_cem_st
        for __dotf_cem_f in "$@"; do
            __dotf_cem_i=$((__dotf_cem_i + 1))
            if [[ -e "$__dotf_cem_f" ]] && zstat -H __dotf_cem_st -- "$__dotf_cem_f" 2>/dev/null; then
                if [[ $__dotf_cem_i -le 2 ]]; then
                    if (( __dotf_cem_st[mode] & 8#022 )); then
                        if [[ $__dotf_cem_i -eq 1 ]]; then __dotf_cem_weak+="dir "; else __dotf_cem_weak+="file "; fi
                    fi
                else
                    __dotf_cem_meta+="$__dotf_cem_f=${__dotf_cem_st[inode]}:${__dotf_cem_st[size]}:${__dotf_cem_st[mtime]} "
                fi
            elif [[ $__dotf_cem_i -gt 2 ]]; then
                __dotf_cem_meta+="$__dotf_cem_f=missing "
            fi
        done
        printf -v "$__dotf_cem_sigvar" '%s' "$__dotf_cem_meta"
        printf -v "$__dotf_cem_weakvar" '%s' "$__dotf_cem_weak"
        return 0
    fi

    # bash: ONE stat for every existing file, one "<inode>:<size>:<mtime>:<octal
    # mode>" line each, in argument order (paths never appear in the output, so
    # a newline in a path cannot desync it). GNU (-c) is tried first; the flavor
    # that works is memoized in _DOTFILES_EVAL_CACHE_STAT so later calls never
    # probe again.
    for __dotf_cem_f in "$@"; do
        [[ -e "$__dotf_cem_f" ]] && __dotf_cem_present+=("$__dotf_cem_f")
    done
    __dotf_cem_out=""
    if [[ ${#__dotf_cem_present[@]} -gt 0 ]]; then
        case "${_DOTFILES_EVAL_CACHE_STAT:-}" in
            gnu) __dotf_cem_out="$(command stat -L -c '%i:%s:%Y:%a' -- "${__dotf_cem_present[@]}" 2>/dev/null)" || return 1 ;;
            bsd) __dotf_cem_out="$(command stat -L -f '%i:%z:%m:%Lp' -- "${__dotf_cem_present[@]}" 2>/dev/null)" || return 1 ;;
            *)
                if __dotf_cem_out="$(command stat -L -c '%i:%s:%Y:%a' -- "${__dotf_cem_present[@]}" 2>/dev/null)"; then
                    _DOTFILES_EVAL_CACHE_STAT=gnu
                elif __dotf_cem_out="$(command stat -L -f '%i:%z:%m:%Lp' -- "${__dotf_cem_present[@]}" 2>/dev/null)"; then
                    _DOTFILES_EVAL_CACHE_STAT=bsd
                else
                    return 1
                fi
                ;;
        esac
    fi
    # Walk "$@" in order, consuming one stat line per existing file.
    local -a __dotf_cem_lines=()
    while IFS= read -r __dotf_cem_line; do
        __dotf_cem_lines+=("$__dotf_cem_line")
    done <<<"$__dotf_cem_out"
    local __dotf_cem_j=0
    for __dotf_cem_f in "$@"; do
        __dotf_cem_i=$((__dotf_cem_i + 1))
        if [[ -e "$__dotf_cem_f" ]]; then
            __dotf_cem_rec="${__dotf_cem_lines[$__dotf_cem_j]:-}"
            __dotf_cem_j=$((__dotf_cem_j + 1))
            [[ -n "$__dotf_cem_rec" ]] || return 1
            __dotf_cem_mode="${__dotf_cem_rec##*:}"
            if [[ $__dotf_cem_i -le 2 ]]; then
                if (( 8#$__dotf_cem_mode & 8#022 )); then
                    if [[ $__dotf_cem_i -eq 1 ]]; then __dotf_cem_weak+="dir "; else __dotf_cem_weak+="file "; fi
                fi
            else
                __dotf_cem_meta+="$__dotf_cem_f=${__dotf_cem_rec%:*} "
            fi
        elif [[ $__dotf_cem_i -gt 2 ]]; then
            __dotf_cem_meta+="$__dotf_cem_f=missing "
        fi
    done
    printf -v "$__dotf_cem_sigvar" '%s' "$__dotf_cem_meta"
    printf -v "$__dotf_cem_weakvar" '%s' "$__dotf_cem_weak"
}

# Every local carries the __dotf_ce_ prefix: the cached init code is sourced
# inside this function, so a plain name like `file` would shadow a global the
# init code assigns.
_cached_eval() {
    local -a __dotf_ce_deps=()
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dep)
                [[ $# -ge 2 ]] || return 2
                __dotf_ce_deps+=("$2")
                shift 2
                ;;
            --)
                shift
                break
                ;;
            *) break ;;
        esac
    done
    [[ $# -ge 2 && -n "$1" && -n "$2" ]] || return 2

    local __dotf_ce_name="$1" __dotf_ce_cmd="$2"
    shift 2

    # Resolve the binary. zsh: the $commands hash, after a `rehash` when the
    # name is not in it yet (a binary installed earlier in this startup, e.g.
    # the starship brew fallback). bash: BASH_CMDS via `hash` (re-searches
    # PATH), else `type -P` — `hash` fails when hashing is off (set +h).
    local __dotf_ce_bin="" __dotf_ce_shell
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        __dotf_ce_shell=zsh
        if [[ "$__dotf_ce_cmd" == */* ]]; then
            [[ -x "$__dotf_ce_cmd" ]] && __dotf_ce_bin="$__dotf_ce_cmd"
        else
            # shellcheck disable=SC2154 # zsh's $commands hash
            __dotf_ce_bin="${commands[$__dotf_ce_cmd]:-}"
            if [[ -z "$__dotf_ce_bin" ]]; then
                rehash
                __dotf_ce_bin="${commands[$__dotf_ce_cmd]:-}"
            fi
        fi
    else
        __dotf_ce_shell=bash
        if [[ "$__dotf_ce_cmd" == */* ]]; then
            [[ -x "$__dotf_ce_cmd" ]] && __dotf_ce_bin="$__dotf_ce_cmd"
        else
            hash -- "$__dotf_ce_cmd" 2>/dev/null && __dotf_ce_bin="${BASH_CMDS[$__dotf_ce_cmd]:-}"
            [[ -n "$__dotf_ce_bin" ]] || __dotf_ce_bin="$(type -P -- "$__dotf_ce_cmd")"
        fi
    fi
    [[ -n "$__dotf_ce_bin" ]] || return 127
    [[ "$__dotf_ce_bin" == /* ]] || __dotf_ce_bin="$PWD/$__dotf_ce_bin"

    if [[ -n "${DOTFILES_NO_EVAL_CACHE:-}" && "$DOTFILES_NO_EVAL_CACHE" != 0 ]]; then
        eval "$("$__dotf_ce_bin" "$@")"
        return
    fi

    local __dotf_ce_dir="${XDG_CACHE_HOME:-$HOME/.cache}/dotfiles/eval"
    local __dotf_ce_file="$__dotf_ce_dir/$__dotf_ce_name.$__dotf_ce_shell"
    local __dotf_ce_argv __dotf_ce_meta="" __dotf_ce_weak="" __dotf_ce_header="" __dotf_ce_line=""
    local __dotf_ce_out __dotf_ce_status __dotf_ce_tmp __dotf_ce_trusted=""

    printf -v __dotf_ce_argv '%q ' "$__dotf_ce_bin" "$@"
    if _dotfiles_eval_cache_meta __dotf_ce_meta __dotf_ce_weak \
        "$__dotf_ce_dir" "$__dotf_ce_file" "$__dotf_ce_bin" "${__dotf_ce_deps[@]}"; then
        __dotf_ce_header="# dotfiles-cached-eval v1 $__dotf_ce_shell $__dotf_ce_bin $__dotf_ce_argv$__dotf_ce_meta"
    fi

    # Trusted: the dir is ours and not group/world-writable, and so is the file
    # (when present). Anything else is never sourced or written into.
    if [[ -d "$__dotf_ce_dir" && -O "$__dotf_ce_dir" && -z "$__dotf_ce_weak" ]] &&
        { [[ ! -e "$__dotf_ce_file" ]] || [[ -f "$__dotf_ce_file" && -O "$__dotf_ce_file" ]]; }; then
        __dotf_ce_trusted=1
    fi

    # Hit: trusted and the header matches the current signature.
    if [[ -n "$__dotf_ce_header" && -n "$__dotf_ce_trusted" && -f "$__dotf_ce_file" ]] &&
        IFS= read -r __dotf_ce_line <"$__dotf_ce_file" &&
        [[ "$__dotf_ce_line" == "$__dotf_ce_header" ]]; then
        set --
        # shellcheck source=/dev/null
        source "$__dotf_ce_file"
        return
    fi

    # Miss: run once.
    __dotf_ce_out="$("$__dotf_ce_bin" "$@")"
    __dotf_ce_status=$?

    # Failure, empty output, or unreadable metadata: never cache, but keep the
    # `eval "$(cmd)"` behavior of running whatever was printed.
    if [[ $__dotf_ce_status -ne 0 || -z "$__dotf_ce_out" || -z "$__dotf_ce_header" ]]; then
        [[ -n "$__dotf_ce_trusted" ]] && command rm -f -- "$__dotf_ce_file" 2>/dev/null
        [[ -n "$__dotf_ce_out" ]] && eval "$__dotf_ce_out"
        return "$__dotf_ce_status"
    fi

    # Create the dir when absent (mode 700 regardless of umask; parents keep the
    # umask). An existing dir that is not trusted is left alone: no write.
    if [[ ! -e "$__dotf_ce_dir" ]] && command mkdir -p -m 700 "$__dotf_ce_dir" 2>/dev/null &&
        [[ -O "$__dotf_ce_dir" ]]; then
        __dotf_ce_trusted=1
    fi
    if [[ -n "$__dotf_ce_trusted" ]] &&
        __dotf_ce_tmp="$(command mktemp "$__dotf_ce_dir/.$__dotf_ce_name.$__dotf_ce_shell.XXXXXX" 2>/dev/null)"; then
        # >| so a user's `setopt noclobber` / `set -C` cannot block the write.
        if printf '%s\n%s\n' "$__dotf_ce_header" "$__dotf_ce_out" >|"$__dotf_ce_tmp" &&
            command mv -f -- "$__dotf_ce_tmp" "$__dotf_ce_file"; then
            set --
            # shellcheck source=/dev/null
            source "$__dotf_ce_file"
            return
        fi
        command rm -f -- "$__dotf_ce_tmp" 2>/dev/null
    fi

    # Could not (or must not) write the cache: behave like the uncached eval.
    eval "$__dotf_ce_out"
}

# Remove every cached init file; the next shell start regenerates them.
dotfiles_clear_eval_cache() {
    command rm -rf -- "${XDG_CACHE_HOME:-$HOME/.cache}/dotfiles/eval"
}
