#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[BREW-UPDATE]"

# Configuration
DRY_RUN="${DRY_RUN:-false}"
PARALLEL_JOBS="${PARALLEL_JOBS:-4}"
SKIP_CLEANUP="${SKIP_CLEANUP:-false}"
EXCLUDE_PACKAGES="${EXCLUDE_PACKAGES:-}"
NOTIFY="${NOTIFY:-false}"

usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS]

Update Homebrew and installed packages with enhanced features.

OPTIONS:
    --dry-run           Show what would be updated without actually updating
    --parallel JOBS     Number of parallel jobs for updates (default: 4)
    --skip-cleanup      Skip cleanup after updates
    --exclude PACKAGE   Exclude package from updates (can be used multiple times)
    --notify            Send notification when complete (macOS only)
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    DRY_RUN            Enable dry-run mode (true/false)
    PARALLEL_JOBS      Number of parallel jobs (default: 4)
    SKIP_CLEANUP       Skip cleanup (true/false)
    EXCLUDE_PACKAGES   Comma-separated list of packages to exclude
    NOTIFY             Send notification when complete (true/false)

EXAMPLES:
    $SCRIPT_NAME --dry-run
    $SCRIPT_NAME --parallel 8 --notify
    $SCRIPT_NAME --exclude node --exclude python
    DRY_RUN=true $SCRIPT_NAME
EOF
    exit "${1:-0}"
}

log() {
    echo "${LOG_PREFIX} [$(date '+%Y-%m-%d %H:%M:%S')] $*" >&2
}

error_exit() {
    log "ERROR: $1" >&2
    exit "${2:-1}"
}

check_prerequisites() {
    log "Checking prerequisites..."
    
    # Check if Homebrew is installed
    if ! command -v brew &>/dev/null; then
        error_exit "Homebrew is not installed"
    fi
    
    # Check network connectivity
    if ! curl -s --connect-timeout 5 https://formulae.brew.sh >/dev/null; then
        error_exit "No internet connectivity or Homebrew servers unreachable"
    fi
    
    # Check available disk space (require at least 1GB)
    local available_space
    available_space=$(df "$(brew --prefix)" | tail -1 | awk '{print $4}')
    if [[ $available_space -lt 1048576 ]]; then  # 1GB in KB
        error_exit "Insufficient disk space. At least 1GB required."
    fi
    
    log "Prerequisites check passed"
}

update_homebrew() {
    log "Updating Homebrew..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would update Homebrew"
        return 0
    fi
    
    if ! brew update; then
        error_exit "Failed to update Homebrew"
    fi
    
    log "Homebrew updated successfully"
}

get_outdated_packages() {
    local outdated_formulas outdated_casks
    
    log "Checking for outdated packages..."
    
    # Get outdated formulas
    outdated_formulas=$(brew outdated --formula --quiet 2>/dev/null || true)
    
    # Get outdated casks
    outdated_casks=$(brew outdated --cask --quiet 2>/dev/null || true)
    
    # Filter excluded packages
    if [[ -n "$EXCLUDE_PACKAGES" ]]; then
        IFS=',' read -ra excludes <<< "$EXCLUDE_PACKAGES"
        for exclude in "${excludes[@]}"; do
            outdated_formulas=$(echo "$outdated_formulas" | grep -v "^$exclude$" || true)
            outdated_casks=$(echo "$outdated_casks" | grep -v "^$exclude$" || true)
        done
    fi
    
    echo "$outdated_formulas"
    echo "$outdated_casks"
}

upgrade_packages() {
    local outdated_formulas outdated_casks

    log "Checking for outdated packages..."

    # Get outdated formulas and casks
    outdated_formulas=$(brew outdated --formula --quiet 2>/dev/null || true)
    outdated_casks=$(brew outdated --cask --quiet 2>/dev/null || true)

    # Filter excluded packages
    if [[ -n "$EXCLUDE_PACKAGES" ]]; then
        local -a excludes
        IFS=',' read -ra excludes <<< "$EXCLUDE_PACKAGES"
        for exclude in "${excludes[@]}"; do
            outdated_formulas=$(echo "$outdated_formulas" | grep -v "^$exclude$" || true)
            outdated_casks=$(echo "$outdated_casks" | grep -v "^$exclude$" || true)
        done
    fi

    # Remove empty lines
    outdated_formulas=$(echo "$outdated_formulas" | sed '/^$/d')
    outdated_casks=$(echo "$outdated_casks" | sed '/^$/d')

    if [[ -z "$outdated_formulas" ]] && [[ -z "$outdated_casks" ]]; then
        log "All packages are up to date"
        return 0
    fi

    log "Packages to update:"
    if [[ -n "$outdated_formulas" ]]; then
        while read -r package; do
            log "  - $package (formula)"
        done <<< "$outdated_formulas"
    fi
    if [[ -n "$outdated_casks" ]]; then
        while read -r package; do
            log "  - $package (cask)"
        done <<< "$outdated_casks"
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would upgrade the above packages"
        return 0
    fi

    # Upgrade formulas
    if [[ -n "$outdated_formulas" ]]; then
        log "Upgrading formulas..."
        while read -r formula; do
            brew upgrade "$formula" < /dev/null
        done <<< "$outdated_formulas"
    fi

    # Upgrade casks
    if [[ -n "$outdated_casks" ]]; then
        log "Upgrading casks..."
        while read -r cask; do
            brew upgrade --cask "$cask" < /dev/null
        done <<< "$outdated_casks"
    fi

    log "Package upgrades completed"
}

cleanup_homebrew() {
    if [[ "$SKIP_CLEANUP" == "true" ]]; then
        log "Skipping cleanup (--skip-cleanup specified)"
        return 0
    fi
    
    log "Cleaning up old versions..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would run brew cleanup"
        return 0
    fi
    
    # Cleanup with parallel jobs if supported
    if brew cleanup --help | grep -q -- "--jobs"; then
        brew cleanup --jobs="$PARALLEL_JOBS"
    else
        brew cleanup
    fi
    
    log "Cleanup completed"
}

show_summary() {
    log "Update summary:"
    
    local updated_count
    updated_count=$(brew list --versions | wc -l | xargs)
    log "Total packages installed: $updated_count"
    
    local outdated
    outdated=$(get_outdated_packages)
    if [[ -z "$outdated" ]]; then
        log "All packages are up to date"
    else
        local outdated_count
        outdated_count=$(echo "$outdated" | grep -c . || echo "0")
        log "Packages still outdated: $outdated_count"
    fi
}

send_notification() {
    if [[ "$NOTIFY" == "true" ]] && [[ "$(uname)" == "Darwin" ]]; then
        osascript -e 'display notification "Homebrew update completed" with title "Brew Update" sound name "Glass"' 2>/dev/null || true
    fi
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)
            DRY_RUN="true"
            shift
            ;;
        --parallel)
            PARALLEL_JOBS="$2"
            shift 2
            ;;
        --skip-cleanup)
            SKIP_CLEANUP="true"
            shift
            ;;
        --exclude)
            if [[ -n "$EXCLUDE_PACKAGES" ]]; then
                EXCLUDE_PACKAGES="$EXCLUDE_PACKAGES,$2"
            else
                EXCLUDE_PACKAGES="$2"
            fi
            shift 2
            ;;
        --notify)
            NOTIFY="true"
            shift
            ;;
        -h|--help)
            usage 0
            ;;
        *)
            error_exit "Unknown option: $1. Use --help for usage information."
            ;;
    esac
done

main() {
    log "Starting Homebrew update process..."
    [[ "$DRY_RUN" == "true" ]] && log "DRY-RUN MODE: No changes will be made"
    
    check_prerequisites
    update_homebrew
    upgrade_packages
    cleanup_homebrew
    show_summary
    send_notification
    
    log "Homebrew update process completed successfully"
}

main "$@"
