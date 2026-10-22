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
#     terraform, npm, ansible, …) are lazy-loaded via lazy-completions.sh.
#     Eager zsh loading was ~5s (ansible alone ~2.3s from failed probes).
#   - Lightweight setup only: Homebrew fpath, git, fabric static completions.
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

# Check if a command exists (completion support checked in load_completion)
has_completion() {
    local cmd="$1"
    command -v "$cmd" >/dev/null 2>&1
}

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
# KUBERNETES COMPLETIONS
# ================================================================
# Kubernetes ecosystem tools: kubectl, helm, k9s, istioctl, linkerd.
# Also sets up completions for kubectl aliases (k, kc).

setup_kubernetes_completions() {
    # kubectl
    if command -v kubectl >/dev/null 2>&1; then
        load_completion kubectl
        
        # Common kubectl aliases
        if [[ "$CURRENT_SHELL" == "zsh" ]]; then
            compdef k=kubectl
            compdef kc=kubectl
        elif [[ "$CURRENT_SHELL" == "bash" ]]; then
            complete -o default -F __start_kubectl k
            complete -o default -F __start_kubectl kc
        fi
    fi
    
    # helm
    load_completion helm
    
    # kubens and kubectx
    if command -v kubens >/dev/null 2>&1; then
        load_completion kubens
    fi
    
    if command -v kubectx >/dev/null 2>&1; then
        load_completion kubectx
    fi
    
    # k9s
    if command -v k9s >/dev/null 2>&1; then
        load_completion k9s
    fi
    
    # istioctl
    if command -v istioctl >/dev/null 2>&1; then
        load_completion istioctl
    fi
    
    # linkerd
    if command -v linkerd >/dev/null 2>&1; then
        load_completion linkerd
    fi
}

# ================================================================
# CLOUD PROVIDER COMPLETIONS
# ================================================================
# AWS, GCP, and Azure CLI completions.

# AWS CLI v2 and related tools (CDK, SAM, eksctl, Copilot)
setup_aws_completions() {
    # AWS CLI v2
    if command -v aws >/dev/null 2>&1; then
        if [[ "$CURRENT_SHELL" == "zsh" ]]; then
            # Use aws_completer if available
            if command -v aws_completer >/dev/null 2>&1; then
                complete -C aws_completer aws
            fi
        elif [[ "$CURRENT_SHELL" == "bash" ]]; then
            # Bash completion for AWS CLI
            if command -v aws_completer >/dev/null 2>&1; then
                complete -C aws_completer aws
            fi
        fi
    fi
    
    # AWS CDK
    load_completion cdk
    
    # AWS SAM
    if command -v sam >/dev/null 2>&1; then
        load_completion sam
    fi
    
    # eksctl
    load_completion eksctl
    
    # AWS Copilot
    if command -v copilot >/dev/null 2>&1; then
        load_completion copilot
    fi
}

# Google Cloud SDK completions (gcloud, gsutil, bq)
setup_gcp_completions() {
    # Google Cloud SDK
    if command -v gcloud >/dev/null 2>&1; then
        # Load gcloud completion
        if [[ -n "$DOTFILES_HOMEBREW_PATH" ]]; then
            local gcloud_completion="$DOTFILES_HOMEBREW_PATH/share/google-cloud-sdk"
            if [[ -d "$gcloud_completion" ]]; then
                if [[ "$CURRENT_SHELL" == "zsh" ]]; then
                    source "$gcloud_completion/completion.zsh.inc"
                elif [[ "$CURRENT_SHELL" == "bash" ]]; then
                    source "$gcloud_completion/completion.bash.inc"
                fi
            fi
        fi
    fi
}

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
# Infrastructure as Code tools: Terraform, Terragrunt, Packer,
# Vault, Consul, Nomad, CDKTF, Pulumi, Ansible.

# HashiCorp tools and IaC completions
setup_terraform_completions() {
    # Terraform
    if command -v terraform >/dev/null 2>&1; then
        if [[ "$CURRENT_SHELL" == "zsh" ]]; then
            autoload -U +X bashcompinit && bashcompinit
        fi
        complete -C terraform terraform
        
        # tf alias
        complete -C terraform tf
    fi
    
    # Terragrunt
    load_completion terragrunt
    
    # Packer
    if command -v packer >/dev/null 2>&1; then
        if [[ "$CURRENT_SHELL" == "bash" ]]; then
            complete -C packer packer
        fi
    fi
    
    # Vault
    if command -v vault >/dev/null 2>&1; then
        if [[ "$CURRENT_SHELL" == "bash" ]]; then
            complete -C vault vault
        elif [[ "$CURRENT_SHELL" == "zsh" ]]; then
            compctl -f -x 'p[1]' -k '(auth delete list read write)' -- vault
        fi
    fi
    
    # Consul
    if command -v consul >/dev/null 2>&1; then
        if [[ "$CURRENT_SHELL" == "bash" ]]; then
            complete -C consul consul
        fi
    fi
    
    # Nomad
    if command -v nomad >/dev/null 2>&1; then
        if [[ "$CURRENT_SHELL" == "bash" ]]; then
            complete -C nomad nomad
        fi
    fi
    
    # CDKTF
    if command -v cdktf >/dev/null 2>&1; then
        if [[ "$CURRENT_SHELL" == "zsh" ]]; then
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
        fi
    fi
    
    # Pulumi
    load_completion pulumi
    
    # Ansible
    if command -v ansible >/dev/null 2>&1; then
        load_completion ansible
        load_completion ansible-playbook
        load_completion ansible-vault
    fi
}

# ================================================================
# CONTAINER COMPLETIONS
# ================================================================
# Container tools: Docker, Docker Compose, Podman, Buildah, Skopeo.

# Docker and alternative container tools
setup_container_completions() {
    # Docker
    if command -v docker >/dev/null 2>&1; then
        load_completion docker
        
        # Docker Compose
        if command -v docker-compose >/dev/null 2>&1; then
            load_completion docker-compose
        fi
        
        # Docker Machine
        if command -v docker-machine >/dev/null 2>&1; then
            load_completion docker-machine
        fi
    fi
    
    # Podman
    load_completion podman
    
    # Buildah
    load_completion buildah
    
    # Skopeo
    load_completion skopeo
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

# Language-specific tools: Node.js (npm, yarn, pnpm, bun, deno),
# Python (pip, pipenv, poetry), Rust (cargo), Go tools
setup_language_completions() {
    # Node.js tools
    if command -v npm >/dev/null 2>&1; then
        load_completion npm
    fi
    
    if command -v yarn >/dev/null 2>&1; then
        load_completion yarn
    fi
    
    if command -v pnpm >/dev/null 2>&1; then
        load_completion pnpm
    fi
    
    # Deno
    if command -v deno >/dev/null 2>&1; then
        load_completion deno
    fi
    
    # Bun
    if command -v bun >/dev/null 2>&1; then
        load_completion bun
    fi
    
    # Python tools
    if command -v pip >/dev/null 2>&1; then
        load_completion pip
    fi
    
    if command -v pipenv >/dev/null 2>&1; then
        load_completion pipenv
    fi
    
    if command -v poetry >/dev/null 2>&1; then
        load_completion poetry
    fi
    
    # Rust/Cargo
    if command -v cargo >/dev/null 2>&1; then
        if [[ -f "$HOME/.cargo/env" ]]; then
            source "$HOME/.cargo/env"
        fi
        
        # Cargo completions
        if [[ "$CURRENT_SHELL" == "zsh" ]]; then
            if [[ -f "$HOME/.cargo/completions/_cargo" ]]; then
                fpath+=("$HOME/.cargo/completions")
            fi
        elif [[ "$CURRENT_SHELL" == "bash" ]]; then
            if [[ -f "$HOME/.cargo/completions/cargo.bash" ]]; then
                source "$HOME/.cargo/completions/cargo.bash"
            fi
        fi
    fi
    
    # Go tools
    if command -v go >/dev/null 2>&1; then
        # Go doesn't have built-in completion, but some tools do
        load_completion gofmt
        load_completion goimports
    fi
}

# ================================================================
# MODERN CLI TOOLS COMPLETIONS
# ================================================================
# Modern Rust-based CLI tools: ripgrep, fd, bat, eza, starship, delta.
# Note: Some tools have limited completion support.

setup_modern_tools_completions() {
    # ripgrep (rg)
    if command -v rg >/dev/null 2>&1; then
        load_completion rg
    fi
    
    # fd
    if command -v fd >/dev/null 2>&1; then
        load_completion fd
    fi
    
    # bat
    if command -v bat >/dev/null 2>&1; then
        load_completion bat
    fi
    
    # eza
    if command -v eza >/dev/null 2>&1; then
        load_completion eza
    fi
    
    # autojump
    if command -v autojump >/dev/null 2>&1; then
        # autojump completion is handled by its initialization script
        # No additional completion setup needed
        :
    fi
    
    # starship
    if command -v starship >/dev/null 2>&1; then
        load_completion starship
    fi
    
    # delta
    if command -v delta >/dev/null 2>&1; then
        load_completion delta
    fi
    
    # tldr
    if command -v tldr >/dev/null 2>&1; then
        load_completion tldr
    fi
}

# ================================================================
# HOMEBREW COMPLETIONS
# ================================================================
# Homebrew-provided completions for various tools.
# Adds Homebrew completion paths to FPATH (zsh) or sources bash completion.

setup_homebrew_completions() {
    if [[ -n "$DOTFILES_HOMEBREW_PATH" ]]; then
        case "$CURRENT_SHELL" in
            zsh)
                # Add Homebrew completion paths
                if [[ -d "$DOTFILES_HOMEBREW_PATH/share/zsh/site-functions" ]]; then
                    FPATH="$DOTFILES_HOMEBREW_PATH/share/zsh/site-functions:$FPATH"
                fi
                if [[ -d "$DOTFILES_HOMEBREW_PATH/share/zsh-completions" ]]; then
                    FPATH="$DOTFILES_HOMEBREW_PATH/share/zsh-completions:$FPATH"
                fi
                ;;
            bash)
                # Load bash completion
                if [[ -f "$DOTFILES_HOMEBREW_PATH/etc/profile.d/bash_completion.sh" ]]; then
                    source "$DOTFILES_HOMEBREW_PATH/etc/profile.d/bash_completion.sh"
                fi
                ;;
        esac
    fi
}

# ================================================================
# 1PASSWORD COMPLETIONS
# ================================================================
# Password manager CLIs: 1Password (op), Bitwarden (bw, bws).

setup_1password_completions() {
    if command -v op >/dev/null 2>&1; then
        load_completion op
    fi
    
    # Bitwarden CLI
    if command -v bw >/dev/null 2>&1; then
        load_completion bw
    fi
    
    # Bitwarden Secrets CLI
    if command -v bws >/dev/null 2>&1; then
        load_completion bws
    fi
}

# ================================================================
# CARAPACE UNIVERSAL COMPLETION
# ================================================================
# Carapace provides completions for 100+ tools via a single binary.
# Falls back to fish/bash/zsh completions when native ones unavailable.
# https://github.com/rsteube/carapace

setup_carapace_completions() {
    if command -v carapace >/dev/null 2>&1; then
        export CARAPACE_BRIDGES='zsh,fish,bash,inshellisense'
        
        if [[ "$CURRENT_SHELL" == "zsh" ]]; then
            zstyle ':completion:*' format $'\e[2;37mCompleting %d\e[m'
            source <(carapace _carapace)
        elif [[ "$CURRENT_SHELL" == "bash" ]]; then
            source <(carapace _carapace)
        fi
    fi
}

# ================================================================
# MAIN SETUP FUNCTION
# ================================================================
# Orchestrates all completion setup. Bash uses lazy loading for
# performance; zsh loads everything eagerly.

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

    # -i, not bare compinit: a bare compinit stops startup with an interactive
    # "insecure directories" prompt when any fpath entry is group-writable, which
    # is exactly the state a freshly provisioned machine can land in. -i skips
    # those directories instead (never -u, which would load them anyway).
    #
    # _dotfiles_compinit_done (not exported) makes this run once per shell, so
    # re-sourcing ~/.zshrc does not pay for compinit again.
    if [[ -n "${_dotfiles_compinit_done:-}" ]]; then
        :
    elif ! typeset -f compdef >/dev/null 2>&1; then
        autoload -Uz compinit && compinit -i
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
        autoload -Uz compinit && compinit -i -d "${ZDOTDIR:-$HOME}/.zcompdump-dotfiles"
        # shellcheck disable=SC2296 # zsh-only (k) flag
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

    # Set up Homebrew completions first (provides foundation)
    setup_homebrew_completions

    # Core development tools (git completions are fast)
    setup_git_completions
    setup_fabric_completions

    # Heavy completions (kubectl, helm, docker, aws, gcloud, terraform, carapace,
    # plus ansible/npm/pip/…) stay out of the eager path. lazy-completions.sh
    # installs first-use wrappers for the big CLIs. The setup_* helpers above
    # this comment remain available for manual/debug loading.
    #
    # Eager zsh loading previously cost ~5s — mostly ansible probe failures
    # (~2.3s) plus kubectl/npm/pip generation — while bash already returned early.
}

# ================================================================
# INITIALIZATION
# ================================================================
# Automatic completion setup on source. Disable by setting
# DOTFILES_NO_AUTO_COMPLETIONS=1 before sourcing.

if [[ -z "${DOTFILES_NO_AUTO_COMPLETIONS}" ]]; then
    setup_all_completions
fi
