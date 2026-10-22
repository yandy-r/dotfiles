#!/usr/bin/env bash
# shellcheck shell=bash
# ================================================================
# Machine-Local Overrides Hook (bash + zsh)
# ================================================================
#
# Purpose:
#   Loads per-machine customizations that live outside the dotfiles repo.
#   Sourced as the LAST line of bash/bashrc and zsh/zshrc, so anything here
#   wins over the shared configuration (PATH entries end up first, aliases
#   and exports override the defaults).
#
# Load order (each step only when the file exists):
#   1. ${XDG_CONFIG_HOME:-~/.config}/dotfiles.local/env.sh
#                                                  /paths.sh
#                                                  /aliases.sh
#                                                  /secrets.sh
#   2. Raw token files in ${XDG_CONFIG_HOME:-~/.config}/secrets/, exported as:
#        1password-service-account -> OP_SERVICE_ACCOUNT_TOKEN
#        gemini_api_key            -> GEMINI_API_KEY
#        openai_api_key            -> OPENAI_API_KEY
#        anthropic_api_key         -> ANTHROPIC_API_KEY
#   3. ${XDG_CONFIG_HOME:-~/.config}/secrets/*.sh (sorted)
#   4. Legacy per-shell file: ~/.zshrc.local (zsh) or ~/.bashrc.local (bash)
#
# Safety:
#   Missing files are skipped silently. A candidate is skipped with a one-line
#   `dotfiles: skipping <path> (...)` warning on stderr when:
#     - the file (symlinks followed) is not owned by the current user,
#     - it is not a readable regular file,
#     - the file is world-writable, or group-writable by a group other than the
#       user's private group (on user-private-group systems such as Ubuntu,
#       where `id -gn` equals the user name, umask 002 files still load),
#     - the directory holding it (and, for a symlink, the directory holding its
#       target) is not owned by the current user or is writable as above.
#   The symlink-target directory is only checked where `realpath` exists
#   (GNU coreutils, macOS 13+); otherwise only the candidate's directory is.
#
# zsh completions:
#   If an override file changes $fpath, compinit is re-run afterwards with its
#   own dump (${ZDOTDIR:-$HOME}/.zcompdump-local), so the main dump written by
#   the earlier compinit calls is not regenerated on every startup.
#
# Why the files are sourced at top level:
#   Sourcing inside a function would make `typeset`/`declare` in user files
#   function-local and, in zsh, let local_options revert their `setopt`s. So a
#   helper only builds the candidate list; the `for` loop below sources it at
#   top level, and every helper name is removed afterwards.
# ================================================================

# Queue $2 for loading (kind $1: "source" or "export:VAR") when it is safe.
_dotf_local_add() {
    local kind="$1" file="$2" user="$_dotf_local_user" dir target writable
    [[ -e "$file" ]] || return 0
    if [[ ! -O "$file" ]]; then
        printf 'dotfiles: skipping %s (not owned by %s)\n' "$file" "$user" >&2
        return 0
    fi
    if [[ ! -f "$file" || ! -r "$file" ]]; then
        printf 'dotfiles: skipping %s (not a readable file)\n' "$file" >&2
        return 0
    fi

    # Directories whose writers could swap the file: the one the candidate path
    # lives in, plus the target's directory when the candidate is a symlink.
    local -a dirs=("${file%/*}")
    if [[ -L "$file" ]] && command -v realpath >/dev/null 2>&1 &&
        target="$(realpath "$file" 2>/dev/null)"; then
        dirs+=("${target%/*}")
    fi
    for dir in "${dirs[@]}"; do
        if [[ ! -O "$dir" ]]; then
            printf 'dotfiles: skipping %s (directory %s not owned by %s)\n' "$file" "$dir" "$user" >&2
            return 0
        fi
    done

    # One portable (GNU + BSD) find for the file and its directories. Group
    # write is fine when the group is the user's private group.
    if [[ -n "$_dotf_local_upg_gid" ]]; then
        writable="$(find -L "$file" "${dirs[@]}" -prune \( -perm -002 -o \( -perm -020 ! -group "$_dotf_local_upg_gid" \) \) -print 2>/dev/null)"
    else
        writable="$(find -L "$file" "${dirs[@]}" -prune \( -perm -002 -o -perm -020 \) -print 2>/dev/null)"
    fi
    if [[ -n "$writable" ]]; then
        printf 'dotfiles: skipping %s (group/world-writable)\n' "$file" >&2
        return 0
    fi

    _dotf_local_files+=("$kind:$file")
}

# Build the global _dotf_local_files list in load order.
_dotf_local_collect() {
    local config="${XDG_CONFIG_HOME:-$HOME/.config}"
    local overrides="$config/dotfiles.local" secrets="$config/secrets"
    local name pair
    local -a scripts

    _dotf_local_files=()

    # Identity, computed once for every candidate: user name, and the primary
    # gid when it is a user-private group (group name == user name).
    _dotf_local_user="${USER:-}"
    [[ -n "$_dotf_local_user" ]] || _dotf_local_user="$(id -un)"
    _dotf_local_upg_gid=""
    if [[ "$(id -gn 2>/dev/null)" == "$_dotf_local_user" ]]; then
        _dotf_local_upg_gid="$(id -g)"
    fi

    for name in env paths aliases secrets; do
        _dotf_local_add source "$overrides/$name.sh"
    done

    for pair in \
        "OP_SERVICE_ACCOUNT_TOKEN 1password-service-account" \
        "GEMINI_API_KEY gemini_api_key" \
        "OPENAI_API_KEY openai_api_key" \
        "ANTHROPIC_API_KEY anthropic_api_key"; do
        _dotf_local_add "export:${pair%% *}" "$secrets/${pair#* }"
    done

    # Expand secrets/*.sh with scoped glob options: an empty directory must
    # not trip zsh NOMATCH, and the user's dotglob/nocaseglob stay untouched.
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        setopt local_options null_glob no_glob_dots no_nomatch
        scripts=("$secrets"/*.sh)
    else
        local restore
        restore="$(shopt -p nullglob failglob dotglob nocaseglob)"
        shopt -s nullglob
        shopt -u failglob dotglob nocaseglob
        scripts=("$secrets"/*.sh)
        eval "$restore"
    fi
    for name in "${scripts[@]}"; do
        _dotf_local_add source "$name"
    done

    if [[ -n "${ZSH_VERSION:-}" ]]; then
        _dotf_local_add source "$HOME/.zshrc.local"
    else
        _dotf_local_add source "$HOME/.bashrc.local"
    fi
}

_dotf_local_collect

# zsh: remember fpath so completions added by override files can be loaded.
if [[ -n "${ZSH_VERSION:-}" ]]; then
    # shellcheck disable=SC2154 # zsh's fpath array
    _dotf_local_fpath="${fpath[*]}"
fi

for _dotf_local_entry in "${_dotf_local_files[@]}"; do
    case "$_dotf_local_entry" in
        source:*)
            # shellcheck source=/dev/null
            source "${_dotf_local_entry#source:}"
            ;;
        export:*)
            _dotf_local_entry="${_dotf_local_entry#export:}"
            export "${_dotf_local_entry%%:*}=$(<"${_dotf_local_entry#*:}")"
            ;;
    esac
done

# common/completions.sh:_dotfiles_compinit runs `compinit -i` on an audited
# fpath, so the dump stays stable; plain `compinit -i` when completions.sh was
# not sourced (e.g. DOTFILES_NO_AUTO_TOOL_SETUP=1). A separate dump keeps the
# main one stable (see the header).
if [[ -n "${ZSH_VERSION:-}" && "${fpath[*]}" != "${_dotf_local_fpath:-}" ]]; then
    # shellcheck disable=SC2154 # zsh's functions array
    if (( ${+functions[_dotfiles_compinit]} )); then
        _dotfiles_compinit -d "${ZDOTDIR:-$HOME}/.zcompdump-local"
    else
        autoload -Uz compinit && compinit -i -d "${ZDOTDIR:-$HOME}/.zcompdump-local"
    fi
fi

unset _dotf_local_entry _dotf_local_files _dotf_local_fpath _dotf_local_user _dotf_local_upg_gid
unset -f _dotf_local_add _dotf_local_collect
