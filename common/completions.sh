#!/usr/bin/env bash
# ================================================================
# Cross-Platform Completions Setup
# ================================================================
#
# Purpose:
#   Provides shell completions for modern CLI tools, cloud providers,
#   and development tools. Supports both bash and zsh with consistent
#   behavior.
#
# Dependencies:
#   - common/platform.sh (for DOTFILES_OS, DOTFILES_HOMEBREW_PATH)
#
# Completion Categories:
#   - Kubernetes: kubectl, helm, kubectx/kubens, k9s, istioctl
#   - Cloud Providers: AWS CLI, gcloud, Azure CLI, CDK, SAM, eksctl
#   - Terraform/IaC: terraform, terragrunt, packer, vault, ansible, pulumi
#   - Containers: docker, docker-compose, podman, buildah, skopeo
#   - Development: git, gh (GitHub), glab (GitLab), npm, yarn, cargo
#   - Modern CLI: ripgrep, fd, bat, eza, starship, delta
#   - Security: 1Password CLI, Bitwarden CLI
#
# Performance Notes:
#   - Both bash and zsh: heavy completions (kubectl, helm, docker, aws,
#     gcloud, terraform, npm, ansible, …) are lazy-loaded via
#     lazy-completions.sh. Eager zsh loading was ~5s (ansible alone ~2.3s
#     from failed probes).
#   - Eager in both shells: the bash-completion framework (bash), git, fabric
#     static completions, the cheap `complete -C`/compdef registrations
#     (vault, cdktf, azure), and carapace's bridged completions.
#
# Environment:
#   DOTFILES_NO_AUTO_COMPLETIONS - Disable automatic setup on source
#
# ================================================================

# Source platform detection if not already loaded
if [[ -z "$DOTFILES_OS" ]]; then
    # Intentional dual-shell path resolution: BASH_SOURCE in bash, ${(%):-%x} in zsh.
    # shellcheck disable=SC2296
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-${(%):-%x}}")" && pwd)"
    if [[ -f "$SCRIPT_DIR/platform.sh" ]]; then
        source "$SCRIPT_DIR/platform.sh"
    fi
fi

# ================================================================
# SHELL DETECTION
# ================================================================
# Identifies the current shell for loading appropriate completions.

# Detect current shell (zsh, bash, or unknown)
detect_current_shell() {
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        echo "zsh"
    elif [[ -n "${BASH_VERSION:-}" ]]; then
        echo "bash"
    else
        echo "unknown"
    fi
}

export CURRENT_SHELL
CURRENT_SHELL="$(detect_current_shell)"

# ================================================================
# COMPLETION HELPERS
# ================================================================
# Utility functions for checking and loading completions safely.
# Uses eval instead of process substitution to avoid FD exhaustion.

# Load completion script for a tool safely
# Uses eval instead of process substitution to avoid FD exhaustion (segfault fix)
# Tries multiple completion generation patterns (cmd completion, --completion, etc.)
# Usage: load_completion kubectl
load_completion() {
    local cmd="$1"
    local shell="${2:-$CURRENT_SHELL}"
    local completion_script=""

    # Check if command exists
    command -v "$cmd" >/dev/null 2>&1 || return 0

    # Skip known problematic / non-generating commands. Failed probes are not
    # free: each ansible* attempt takes ~250–300ms and exits non-zero, so three
    # patterns × three ansible commands alone cost ~2.3s of shell startup.
    case "$cmd" in
        bw)
            # Bitwarden CLI has broken completion that outputs random strings
            return 0
            ;;
        starship|autojump|eza|bat|rg|fd|delta)
            # Modern Rust tools require explicit shell flags and fail detection
            return 0
            ;;
        ansible|ansible-playbook|ansible-vault|ansible-galaxy|ansible-doc|ansible-config|ansible-inventory)
            # Ansible has no `completion` / `--completion` generator; probes are slow no-ops
            return 0
            ;;
    esac

    # Try different completion generation patterns (single attempt each).
    # Require non-empty output: exit 0 with empty stdout is not a usable script.
    if completion_script=$("$cmd" completion "$shell" 2>/dev/null) && [[ -n "$completion_script" ]]; then
        :  # Success
    elif completion_script=$("$cmd" --completion "$shell" 2>/dev/null) && [[ -n "$completion_script" ]]; then
        :  # Success
    elif completion_script=$("$cmd" generate completion "$shell" 2>/dev/null) && [[ -n "$completion_script" ]]; then
        :  # Success
    else
        return 0  # No completion available
    fi

    # Source using eval (avoids process substitution FD exhaustion)
    eval "$completion_script" 2>/dev/null || true
}

# ================================================================
# CLOUD PROVIDER COMPLETIONS
# ================================================================

# Azure CLI completions
setup_azure_completions() {
    # Azure CLI
    if command -v az >/dev/null 2>&1; then
        if [[ -n "$DOTFILES_HOMEBREW_PATH" && -f "$DOTFILES_HOMEBREW_PATH/etc/bash_completion.d/az" ]]; then
            source "$DOTFILES_HOMEBREW_PATH/etc/bash_completion.d/az"
        fi
    fi
}

# ================================================================
# TERRAFORM AND IaC COMPLETIONS
# ================================================================
# Infrastructure as Code tools: Vault and CDKTF.

# Vault completion, registered eagerly (a single `complete -C` is cheap in both shells).
setup_vault_completions() {
    command -v vault >/dev/null 2>&1 || return 0

    if [[ "$CURRENT_SHELL" == "bash" ]]; then
        complete -C "$(command -v vault)" vault
    elif [[ "$CURRENT_SHELL" == "zsh" ]]; then
        compctl -f -x 'p[1]' -k '(auth delete list read write)' -- vault
    fi
}

# CDKTF completion, registered eagerly (defining the zsh function is cheap;
# the command probe only runs on first <TAB>).
setup_cdktf_completions() {
    command -v cdktf >/dev/null 2>&1 || return 0
    [[ "$CURRENT_SHELL" == "zsh" ]] || return 0

    # Zsh completion function: uses zsh-only `words` array and split-from-subst.
    # shellcheck disable=SC2154,SC2207
    _cdktf_yargs_completions() {
        local reply
        local si=$IFS
        IFS=$'\n' reply=($(COMP_CWORD="$((CURRENT - 1))" COMP_LINE="$BUFFER" COMP_POINT="$CURSOR" cdktf --get-yargs-completions "${words[@]}"))
        IFS=$si
        _describe 'values' reply
    }
    compdef _cdktf_yargs_completions cdktf
}

# ================================================================
# DEVELOPMENT TOOLS COMPLETIONS
# ================================================================
# Git and related tools: git, gh (GitHub CLI), glab (GitLab CLI).

# Git and Git hosting platform CLIs
setup_git_completions() {
    # Git (usually comes with system)
    if [[ "$CURRENT_SHELL" == "bash" ]]; then
        # Git completion is usually provided by the system
        if [[ -f /usr/local/share/bash-completion/completions/git ]]; then
            source /usr/local/share/bash-completion/completions/git
        elif [[ -f /etc/bash_completion.d/git ]]; then
            source /etc/bash_completion.d/git
        fi
    fi
    
    # GitHub CLI
    # `gh completion <shell>` ignores the positional argument and always emits
    # BASH output; only `gh completion --shell <shell>` honors the shell. Running
    # load_completion here therefore eval'd a bash script under bashcompinit,
    # registering `complete -F __start_gh`, whose body calls the bash-only
    # `_init_completion` -> "command not found: _init_completion" on every
    # `gh <TAB>`. zsh already has a real `_gh` on fpath (vendor-completions and
    # ~/.zfunc), so nothing needs generating there.
    if command -v gh >/dev/null 2>&1; then
        if [[ "$CURRENT_SHELL" == "bash" ]]; then
            eval "$(gh completion --shell bash 2>/dev/null)" 2>/dev/null || true
        fi
    fi
    
    # GitLab CLI
    if command -v glab >/dev/null 2>&1; then
        load_completion glab
    fi

    # Gitea/Forgejo CLI
    if command -v tea >/dev/null 2>&1; then
        load_completion tea
    fi

    # Hub (GitHub CLI alternative)
    if command -v hub >/dev/null 2>&1; then
        load_completion hub
    fi
    
    # Git LFS
    if command -v git-lfs >/dev/null 2>&1; then
        load_completion git-lfs
    fi
}

# Fabric AI CLI completions.
# Fabric does not expose a safe completion-generation subcommand in all versions,
# so prefer repo-owned zsh completions and static user files.
setup_fabric_completions() {
	local fabric_cmd=""

	if command -v fabric >/dev/null 2>&1; then
		fabric_cmd="fabric"
	elif command -v fabric-ai >/dev/null 2>&1; then
		fabric_cmd="fabric-ai"
	else
		return 0
	fi

	case "$CURRENT_SHELL" in
	zsh)
		local completion_dir
		for completion_dir in \
			"${DOTFILES_ROOT:-$HOME/.config/dotfiles}/zsh/completions" \
			"${XDG_DATA_HOME:-$HOME/.local/share}/zsh/site-functions" \
			"$HOME/.local/share/zsh/site-functions"; do
				if [[ -d "$completion_dir" ]] && { [[ -f "$completion_dir/_fabric" ]] || [[ -f "$completion_dir/_fabric-ai" ]]; }; then
					fpath=("$completion_dir" "${fpath[@]}")
					break
				fi
			done
			;;
		bash)
			local completion_file
			for completion_file in \
				"${XDG_DATA_HOME:-$HOME/.local/share}/bash-completion/completions/$fabric_cmd.bash" \
				"$HOME/.local/share/bash-completion/completions/$fabric_cmd.bash" \
				"${XDG_DATA_HOME:-$HOME/.local/share}/bash-completion/completions/fabric.bash" \
				"$HOME/.local/share/bash-completion/completions/fabric.bash"; do
				if [[ -f "$completion_file" ]]; then
					source "$completion_file"
					break
				fi
			done
			;;
	esac
}

# ================================================================
# CARAPACE UNIVERSAL COMPLETION
# ================================================================
# Carapace provides completions for 100+ tools via a single binary.
# Falls back to fish/bash/zsh completions when native ones unavailable.
# https://github.com/rsteube/carapace
#
# The single carapace owner: registered eagerly at startup in BOTH shells
# (bridging every tool costs less than the lazy wrapper did), so there is no
# lazy wrapper for it in lazy-completions.sh.

setup_carapace_completions() {
    # Carapace's generated bash loaders require bash 4+.
    if [[ "$CURRENT_SHELL" == bash ]] && (( BASH_VERSINFO[0] < 4 )); then
        return 0
    fi
    command -v carapace >/dev/null 2>&1 || return 0
    [[ -n "${LAZY_LOADED_CARAPACE:-}" ]] && return 0

    export CARAPACE_BRIDGES='zsh,fish,bash,inshellisense'

    # `command` bypasses any remaining wrapper; the explicit shell argument
    # keeps carapace from misdetecting the shell when invoked non-interactively.
    case "$CURRENT_SHELL" in
        zsh)
            zstyle ':completion:*' format $'\e[2;37mCompleting %d\e[m'
            eval "$(command carapace _carapace zsh 2>/dev/null)" || true
            ;;
        bash)
            eval "$(command carapace _carapace bash 2>/dev/null)" || true
            ;;
    esac

    LAZY_LOADED_CARAPACE=1
}

# ================================================================
# MAIN SETUP FUNCTION
# ================================================================
# Orchestrates all completion setup: the cheap registrations below run in
# BOTH shells; the heavy generators stay lazy via lazy-completions.sh.

# _dotfiles_compinit [-d DUMPFILE]: initialize completion with `compinit -i`
# semantics, but decide dump freshness here so an unchanged startup reuses the
# dump on every host layout.
#
# compinit's own freshness check compares the dump header's file count against
# a count it takes after its security audit, while compdump writes a count
# taken differently. The two disagree, so the dump is rewritten on EVERY start,
# whenever compaudit reports:
#   - two or more insecure entries (a checkout group-writable by a shared
#     group reports $DOTFILES_ROOT/zsh/completions and $DOTFILES_ROOT/zsh):
#     zsh 5.9 joins them with a literal `|`, so nothing is excluded from its
#     count while compdump excludes them;
#   - an insecure PARENT of a secure fpath dir (Homebrew's group-writable
#     share/ on admin-user Macs; GitHub's Ubuntu image, whose /usr/share/zsh
#     is 777): compinit keeps loading the dir but leaves its files out of its
#     count, while compdump counts them.
#
# So, in order:
#   1. Every fpath dir compaudit flags itself is dropped. A dir holding a file
#      it flags (foreign-owned) or whose .zwc digest it flags is dropped WHOLE,
#      which is stricter than compinit -i (that skips only the flagged file);
#      nothing compinit -i skipped is ever loaded. Kept on purpose: a secure
#      dir under an insecure parent (compinit -i loads it too; Homebrew's
#      zsh-completions must not vanish), and the dir compinit/compaudit/
#      compdump autoload from, so the completion system itself always
#      resolves. Group write by the user-private group (Ubuntu/Fedora umask
#      002) passes compaudit, so such a checkout keeps its repo completions.
#      Restore skipped directories with `compaudit | xargs chmod g-w,o-w`.
#   2. The completion files on the cleaned fpath are counted with compdump's
#      own formula. When the dump's header is exactly `#files: N<TAB>version:
#      $ZSH_VERSION` for that count, `compinit -C` loads it without auditing
#      or rewriting. That is compinit's own freshness rule (file count plus
#      version), so installing or removing a completion file still rebuilds
#      the dump, as with stock zsh. Skipping the audit is safe: step 1 already
#      removed everything it flags, and `-i` would load the same remaining
#      dirs.
#   3. Otherwise the stale dump is deleted and `compinit -i` rebuilds, then
#      compdump rewrites the dump with _comp_secure unset, so its header uses
#      the step 2 formula and the next start matches. The delete matters:
#      compinit's own (buggy) count can coincidentally equal the stale header,
#      and it would then load that dump instead of rebuilding. (_comp_secure,
#      set by compinit when its audit finds anything, makes compdump also drop
#      group-writable dirs without the user-private-group exemption.)
# A flagged system dir (an insecure zsh install) is the one case left to plain
# `compinit -i`: compinit drops it from fpath, so no stable count exists.
_dotfiles_compinit() {
    emulate -L zsh
    setopt extended_glob
    local out dir entry name line dump="${ZDOTDIR:-$HOME}/.zcompdump" flagged_system=""
    local -a insecure keep system files args=("$@")
    autoload -Uz compaudit compinit compdump
    out="$(compaudit 2>/dev/null)"
    if [[ -n "$out" ]]; then
        # First fpath dir holding each completion-system function: where
        # autoload resolves it.
        for name in compinit compaudit compdump; do
            # shellcheck disable=SC2154 # zsh's fpath array
            for dir in "${fpath[@]}"; do
                if [[ -f "$dir/$name" ]]; then
                    system+=("$dir")
                    break
                fi
            done
        done
        # shellcheck disable=SC2296 # zsh-only (@f) flag
        insecure=("${(@f)out}")
        for dir in "${fpath[@]}"; do
            for entry in "${insecure[@]}"; do
                if [[ "$entry" == "$dir" || "$entry" == "$dir.zwc" ||
                    ("${entry##*/}" == _* && "${entry%/*}" == "$dir") ]]; then
                    if [[ " ${system[*]} " == *" $dir "* ]]; then
                        flagged_system=1
                        break
                    fi
                    continue 2
                fi
            done
            keep+=("$dir")
        done
        fpath=("${keep[@]}")
    fi
    if [[ -n "$flagged_system" ]]; then
        compinit -i "$@"
        return
    fi

    # compinit's -d takes the next argument as the dump unless it is an option.
    while (( $# )); do
        if [[ "$1" == -d && $# -gt 1 && "$2" != -* ]]; then
            dump="$2"
            shift
        fi
        shift
    done
    # Verbatim from compdump; eval keeps the zsh-only glob away from shellcheck.
    eval 'files=( ${^~fpath:/.}/^([^_]*|*~|*.zwc)(N) )'
    line=""
    [[ -f "$dump" ]] && IFS= read -r line <"$dump"
    if [[ "$line" == "#files: ${#files}"$'\t'"version: $ZSH_VERSION" ]]; then
        compinit -C "${args[@]}"
    else
        rm -f -- "$dump"
        compinit -i "${args[@]}"
        unset _comp_secure
        compdump
    fi
}

# Initialize zsh's completion system before anything below registers with it.
#
# This file is sourced from common/tools.sh, which zsh/zshrc loads near the TOP
# of startup — long before zshrc's own `compinit` call further down. Without this
# guard the registrations below run against a shell where `compdef` and
# `complete` do not exist yet, and startup prints:
#
#     zsh: complete: command not found...
#     zsh: compdef: command not found...
#
# fpath is already complete at this point: common/paths.sh:setup_fpath runs when
# zshrc sources paths.sh, which precedes tools.sh — so this is the ONE compinit
# and it sees every completion directory. zsh/zshrc must not call compinit again.
#
# Idempotent and cheap: each half is skipped when the shell already provides it,
# so a caller that initialized the system first pays nothing, and zshrc's later
# call is a no-op. `complete` is a bash builtin, so bash needs neither half.
_ensure_completion_system() {
    [[ -n "${ZSH_VERSION:-}" ]] || return 0

    # _dotfiles_compinit runs an audited compinit (-i on rebuild, -C when the
    # dump is current), never bare compinit: a bare compinit stops startup with
    # an interactive "insecure directories" prompt when any fpath entry is
    # group-writable, which is exactly the state a freshly provisioned machine
    # can land in. Insecure directories are skipped instead (never -u, which
    # would load them anyway).
    #
    # _dotfiles_compinit_done (not exported) makes this run once per shell, so
    # re-sourcing ~/.zshrc does not pay for compinit again.
    if [[ -n "${_dotfiles_compinit_done:-}" ]]; then
        :
    elif ! typeset -f compdef >/dev/null 2>&1; then
        _dotfiles_compinit
    else
        # A system zshrc already ran compinit with the default fpath — Debian/
        # Ubuntu's /etc/zsh/zshrc does unless skip_global_compinit=1 is set in
        # ~/.zshenv (users may set it there to skip that call entirely). Re-run
        # once so our fpath counts, into a separate dump: sharing ~/.zcompdump
        # with the system call (default fpath) would rebuild it on every start.
        #
        # compinit never re-binds a name that is already an autoload stub, so a
        # system copy (e.g. vendor-completions/_docker) would keep winning over
        # ours. Drop every not-yet-loaded completion stub first (~5ms) so the
        # re-run binds each name to the first match on our fpath.
        #
        # The re-run also replaces _comps wholesale from the dump, losing what
        # was registered since the system compinit with a function that is
        # already defined (e.g. zoxide's `compdef __zoxide_z_complete z` from
        # setup_zoxide). Those entries are kept and put back when the new
        # table lacks them.
        local _dotf_fn _dotf_key
        local -A _dotf_kept
        # shellcheck disable=SC2296,SC2154 # zsh-only branch: (k)/(u) flags, $_comps
        for _dotf_key in ${(k)_comps}; do
            _dotf_fn="${_comps[$_dotf_key]}"
            if [[ "${functions[$_dotf_fn]-}" == "builtin autoload -X"* ]]; then
                unfunction "$_dotf_fn"
            elif (( ${+functions[$_dotf_fn]} )); then
                _dotf_kept[$_dotf_key]="$_dotf_fn"
            fi
        done
        _dotfiles_compinit -d "${ZDOTDIR:-$HOME}/.zcompdump-dotfiles"
        # shellcheck disable=SC2296,SC2004 # zsh-only (k) flag
        for _dotf_key in ${(k)_dotf_kept}; do
            (( ${+_comps[$_dotf_key]} )) || _comps[$_dotf_key]="${_dotf_kept[$_dotf_key]}"
        done
    fi
    _dotfiles_compinit_done=1

    # bashcompinit provides `complete`/`compgen` so the bash-style registrations
    # below (complete -C terraform, complete -C aws_completer, …) work in zsh.
    if ! typeset -f complete >/dev/null 2>&1; then
        autoload -U +X bashcompinit && bashcompinit
    fi
}

# Set up all completions - main entry point
setup_all_completions() {
    # Must precede every registration below — see _ensure_completion_system.
    _ensure_completion_system

    # Core development tools (git completions are fast)
    setup_git_completions
    setup_fabric_completions

    # Cheap, command -v-guarded registrations, eager in BOTH shells.
    setup_vault_completions
    setup_cdktf_completions
    setup_azure_completions

    # carapace: the single eager owner of universal bridged completions.
    setup_carapace_completions

    # Heavy completions (kubectl, helm, docker, aws, gcloud, terraform, plus
    # ansible/npm/pip/…) stay out of the eager path. lazy-completions.sh
    # installs first-use wrappers for the big CLIs; the remaining setup_*
    # helpers here stay available for manual/debug loading.
    #
    # Eager zsh loading previously cost ~5s — mostly ansible probe failures
    # (~2.3s) plus kubectl/npm/pip generation.
}

# ================================================================
# INITIALIZATION
# ================================================================
# Automatic completion setup on source. Disable by setting
# DOTFILES_NO_AUTO_COMPLETIONS=1 before sourcing.

if [[ -z "${DOTFILES_NO_AUTO_COMPLETIONS}" ]]; then
    setup_all_completions
fi
