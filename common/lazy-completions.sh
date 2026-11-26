#!/usr/bin/env bash
# ================================================================
# Lazy Loading System for Heavy Completions
# ================================================================
# This file implements robust lazy loading for shell completions
# that are expensive to load (kubectl, docker, helm, etc.)
#
# Key design principles:
# 1. Wrapper functions remove themselves BEFORE loading completions
# 2. State tracking prevents double-loading
# 3. Uses `command <tool>` to bypass any remaining wrappers
# 4. Handles both direct commands and aliases
# ================================================================

# State tracking to prevent double-loading
# Using a simple variable approach that works in both bash and zsh
LAZY_LOADED_KUBECTL=""
LAZY_LOADED_DOCKER=""
LAZY_LOADED_HELM=""
# LAZY_LOADED_CARAPACE is set by setup_carapace_completions
# (common/completions.sh), which loads carapace eagerly.
LAZY_LOADED_AWS=""
LAZY_LOADED_GCLOUD=""
LAZY_LOADED_TERRAFORM=""

# ================================================================
# KUBECTL LAZY LOADING
# ================================================================

_setup_lazy_kubectl() {
    # Skip if kubectl not installed
    command -v kubectl &>/dev/null || return 0

    # Skip if already loaded
    [[ -n "$LAZY_LOADED_KUBECTL" ]] && return 0

    # Remove any existing k alias: an alias would shadow the permanent k()
    # function below (and bash expands aliases in function definitions, so
    # `k()` would become `kubectl()`).
    unalias k 2>/dev/null || true

    # The lazy loading wrapper - use 'function' keyword to avoid alias expansion
    function kubectl() {
        # Remove this function FIRST (critical for reliability)
        unset -f kubectl 2>/dev/null

        _lazy_load_kubectl_completions

        # Execute the real command with original args
        command kubectl "$@"
    }

    # Permanent `k` shortcut (YAN-1426). It must NEVER unset itself: the alias
    # from common/aliases.sh was removed above, so once this function is gone
    # nothing named `k` is left. Loads completions on first use, then is a
    # plain pass-through to the real kubectl.
    function k() {
        [[ -n "$LAZY_LOADED_KUBECTL" ]] || _lazy_load_kubectl_completions
        command kubectl "$@"
    }
}

# Load kubectl completions once and register them for kubectl and k/kc.
# Shared by the kubectl() and k() wrappers so whichever runs first loads.
_lazy_load_kubectl_completions() {
    [[ -n "$LAZY_LOADED_KUBECTL" ]] && return 0

    # The kubectl() wrapper is no longer needed once completions are loaded.
    unset -f kubectl 2>/dev/null

    # Load completions based on shell (use eval for bash to avoid FD issues)
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        source <(command kubectl completion zsh) 2>/dev/null
        compdef k=kubectl 2>/dev/null
        compdef kc=kubectl 2>/dev/null
    elif [[ -n "${BASH_VERSION:-}" ]]; then
        eval "$(command kubectl completion bash 2>/dev/null)" 2>/dev/null
        complete -o default -F __start_kubectl k 2>/dev/null
        complete -o default -F __start_kubectl kc 2>/dev/null
    fi

    # Mark as loaded
    LAZY_LOADED_KUBECTL=1
}

# ================================================================
# DOCKER LAZY LOADING
# ================================================================

_setup_lazy_docker() {
    command -v docker &>/dev/null || return 0
    [[ -n "$LAZY_LOADED_DOCKER" ]] && return 0

    function docker() {
        unset -f docker 2>/dev/null

        if [[ -n "${ZSH_VERSION:-}" ]]; then
            source <(command docker completion zsh) 2>/dev/null
        elif [[ -n "${BASH_VERSION:-}" ]]; then
            eval "$(command docker completion bash 2>/dev/null)" 2>/dev/null
        fi

        LAZY_LOADED_DOCKER=1
        command docker "$@"
    }
}

# ================================================================
# HELM LAZY LOADING
# ================================================================

_setup_lazy_helm() {
    command -v helm &>/dev/null || return 0
    [[ -n "$LAZY_LOADED_HELM" ]] && return 0

    function helm() {
        unset -f helm 2>/dev/null

        if [[ -n "${ZSH_VERSION:-}" ]]; then
            source <(command helm completion zsh) 2>/dev/null
        elif [[ -n "${BASH_VERSION:-}" ]]; then
            eval "$(command helm completion bash 2>/dev/null)" 2>/dev/null
        fi

        LAZY_LOADED_HELM=1
        command helm "$@"
    }
}

# ================================================================
# CARAPACE
# ================================================================
# No lazy wrapper: common/completions.sh:setup_carapace_completions is the
# single owner and registers carapace eagerly at startup in both shells.
# LAZY_LOADED_CARAPACE stays as the shared loaded-state flag it sets.

# ================================================================
# AWS CLI LAZY LOADING
# ================================================================

_setup_lazy_aws() {
    command -v aws &>/dev/null || return 0
    command -v aws_completer &>/dev/null || return 0
    [[ -n "$LAZY_LOADED_AWS" ]] && return 0

    function aws() {
        unset -f aws 2>/dev/null
        complete -C aws_completer aws 2>/dev/null
        LAZY_LOADED_AWS=1
        command aws "$@"
    }
}

# ================================================================
# GCLOUD LAZY LOADING
# ================================================================

_setup_lazy_gcloud() {
    command -v gcloud &>/dev/null || return 0
    [[ -n "$LAZY_LOADED_GCLOUD" ]] && return 0

    function gcloud() {
        unset -f gcloud 2>/dev/null

        local gcloud_completion=""
        if [[ -n "$DOTFILES_HOMEBREW_PATH" ]]; then
            gcloud_completion="$DOTFILES_HOMEBREW_PATH/share/google-cloud-sdk"
        fi

        if [[ -d "$gcloud_completion" ]]; then
            if [[ -n "${ZSH_VERSION:-}" ]]; then
                source "$gcloud_completion/completion.zsh.inc" 2>/dev/null
            elif [[ -n "${BASH_VERSION:-}" ]]; then
                source "$gcloud_completion/completion.bash.inc" 2>/dev/null
            fi
        fi

        LAZY_LOADED_GCLOUD=1
        command gcloud "$@"
    }
}

# ================================================================
# TERRAFORM LAZY LOADING
# ================================================================

_setup_lazy_terraform() {
    command -v terraform &>/dev/null || return 0
    [[ -n "$LAZY_LOADED_TERRAFORM" ]] && return 0

    # The `function` keyword is REQUIRED on both wrappers, not stylistic: zsh
    # expands an alias appearing in function-definition position, and
    # common/aliases.sh defines `alias tf="terraform"`. Written as `tf() {`, zsh
    # warns "defining function based on alias `tf'" and then fails with
    # "parse error near `()'", which aborts the REST of this file — taking
    # setup_lazy_completions and every wrapper after it with it. The `function`
    # keyword suppresses that expansion. `unalias` cannot fix this: the body is
    # parsed when this file is sourced, long before any code inside runs.
    function terraform() {
        unset -f terraform 2>/dev/null

        if [[ -n "${ZSH_VERSION:-}" ]]; then
            autoload -U +X bashcompinit && bashcompinit 2>/dev/null
        fi
        complete -C terraform terraform 2>/dev/null
        complete -C terraform tf 2>/dev/null

        LAZY_LOADED_TERRAFORM=1
        command terraform "$@"
    }

    # Also lazy load the tf alias
    function tf() {
        terraform version &>/dev/null  # Triggers load
        unset -f tf 2>/dev/null
        command terraform "$@"
    }
}

# ================================================================
# MAIN SETUP FUNCTION
# ================================================================

setup_lazy_completions() {
    # carapace is NOT lazy anymore: setup_carapace_completions
    # (common/completions.sh) owns it and registers it eagerly in both shells.
    _setup_lazy_kubectl
    _setup_lazy_docker
    _setup_lazy_helm
    _setup_lazy_aws
    _setup_lazy_gcloud
    _setup_lazy_terraform
}

# ================================================================
# DEBUGGING AND UTILITIES
# ================================================================

# Force-load all lazy completions (useful for debugging)
force_load_all_completions() {
    echo "Force-loading all lazy completions..."

    if command -v kubectl &>/dev/null; then
        echo "  Loading kubectl..."
        kubectl version --client &>/dev/null 2>&1 || true
    fi

    if command -v docker &>/dev/null; then
        echo "  Loading docker..."
        docker --version &>/dev/null 2>&1 || true
    fi

    if command -v helm &>/dev/null; then
        echo "  Loading helm..."
        helm version --short &>/dev/null 2>&1 || true
    fi

    if command -v carapace &>/dev/null; then
        echo "  Loading carapace..."
        setup_carapace_completions
    fi

    if command -v aws &>/dev/null; then
        echo "  Loading aws..."
        aws --version &>/dev/null 2>&1 || true
    fi

    if command -v gcloud &>/dev/null; then
        echo "  Loading gcloud..."
        gcloud --version &>/dev/null 2>&1 || true
    fi

    if command -v terraform &>/dev/null; then
        echo "  Loading terraform..."
        terraform version &>/dev/null 2>&1 || true
    fi

    echo "All completions loaded."
}

# Show lazy loading status
lazy_completion_status() {
    echo "Lazy Completion Status:"
    echo "  kubectl:   ${LAZY_LOADED_KUBECTL:-not loaded}"
    echo "  docker:    ${LAZY_LOADED_DOCKER:-not loaded}"
    echo "  helm:      ${LAZY_LOADED_HELM:-not loaded}"
    echo "  carapace:  ${LAZY_LOADED_CARAPACE:-not loaded} (eager; owner is setup_carapace_completions)"
    echo "  aws:       ${LAZY_LOADED_AWS:-not loaded}"
    echo "  gcloud:    ${LAZY_LOADED_GCLOUD:-not loaded}"
    echo "  terraform: ${LAZY_LOADED_TERRAFORM:-not loaded}"
}

# ================================================================
# INITIALIZE
# ================================================================

# Only initialize if not explicitly disabled
if [[ -z "${DOTFILES_NO_LAZY_COMPLETIONS:-}" ]]; then
    setup_lazy_completions
fi
