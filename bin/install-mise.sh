#!/usr/bin/env bash
# ================================================================
# mise Version Manager Installer
# ================================================================
# Installs mise - a modern polyglot runtime version manager
# Replaces nvm, pyenv, rbenv, goenv, etc.
# https://mise.jdx.dev/
# ================================================================

set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[MISE]"
readonly INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Dry run mode
DRY_RUN="${DRY_RUN:-false}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# ================================================================
# UTILITY FUNCTIONS
# ================================================================

log() {
    echo -e "${CYAN}${LOG_PREFIX}${NC} $*" >&2
}

log_success() {
    echo -e "${GREEN}${LOG_PREFIX} ✓ $*${NC}" >&2
}

log_warning() {
    echo -e "${YELLOW}${LOG_PREFIX} ⚠ $*${NC}" >&2
}

log_error() {
    echo -e "${RED}${LOG_PREFIX} ✗ $*${NC}" >&2
}

error_exit() {
    log_error "$1"
    exit "${2:-1}"
}

is_interactive() {
    [[ -t 0 ]] && [[ -t 1 ]]
}

prompt_confirmation() {
    local prompt_message="$1"

    # Auto-approve in non-interactive mode
    if [[ "${NON_INTERACTIVE:-false}" == "true" ]] || [[ "${FORCE_INSTALL:-false}" == "true" ]]; then
        log "Non-interactive mode: auto-approving"
        return 0
    fi

    if ! is_interactive; then
        log_error "Non-interactive shell - cannot prompt"
        return 1
    fi

    echo -n -e "${YELLOW}${LOG_PREFIX} ${prompt_message} [y/N]: ${NC}" >&2
    read -r response
    [[ "$response" =~ ^[yY] ]]
}

# ================================================================
# DETECTION FUNCTIONS
# ================================================================

detect_os() {
    case "$(uname -s)" in
        Darwin*) echo "macos" ;;
        Linux*)
            if [[ -f /etc/os-release ]]; then
                source /etc/os-release
                case "${ID:-unknown}" in
                    ubuntu|debian) echo "debian" ;;
                    fedora|rhel|centos|rocky|almalinux) echo "redhat" ;;
                    arch|cachyos|manjaro|endeavouros) echo "arch" ;;
                    *)
                        case "${ID_LIKE:-}" in
                            *debian*|*ubuntu*) echo "debian" ;;
                            *fedora*|*rhel*) echo "redhat" ;;
                            *arch*) echo "arch" ;;
                            *) echo "linux" ;;
                        esac
                        ;;
                esac
            else
                echo "linux"
            fi
            ;;
        *) echo "unknown" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "x64" ;;
        aarch64|arm64) echo "arm64" ;;
        armv7l) echo "armv7" ;;
        *) echo "unknown" ;;
    esac
}

# ================================================================
# CHECK FUNCTION
# ================================================================

check_mise() {
    if command -v mise >/dev/null 2>&1; then
        local version
        version=$(mise --version 2>/dev/null || echo "unknown")
        log_success "mise is installed: $version"

        # Show some status
        log "Installed tools:"
        mise list 2>/dev/null | head -10 || log "  (none)"

        return 0
    fi

    log_warning "mise is not installed"
    return 1
}

# ================================================================
# INSTALLATION FUNCTIONS
# ================================================================

install_via_homebrew() {
    if ! command -v brew >/dev/null 2>&1; then
        return 1
    fi

    log "Installing mise via Homebrew..."

    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would run: brew install mise"
        return 0
    fi

    # `brew install` is non-interactive by default; no extra flag is needed
    # (the previously-used --no-ask is not a valid `brew install` flag).
    brew install mise
    return $?
}

install_via_curl() {
    log "Installing mise via official installer..."

    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would run: curl https://mise.run | sh"
        return 0
    fi

    # Security notice
    log_warning "This will download and execute a script from:"
    log_warning "    https://mise.run"

    if ! prompt_confirmation "Proceed with installation?"; then
        log "Installation cancelled"
        return 1
    fi

    # Use official installer
    curl -fsSL https://mise.run | sh
    return $?
}

install_via_package_manager() {
    local os="$1"

    case "$os" in
        arch)
            if command -v yay >/dev/null 2>&1; then
                log "Installing mise via yay (AUR)..."
                if [[ "$DRY_RUN" == "true" ]]; then
                    log "DRY-RUN: Would run: yay -S mise"
                    return 0
                fi
                yay -S --noconfirm --needed mise
                return $?
            elif command -v paru >/dev/null 2>&1; then
                log "Installing mise via paru (AUR)..."
                if [[ "$DRY_RUN" == "true" ]]; then
                    log "DRY-RUN: Would run: paru -S mise"
                    return 0
                fi
                paru -S --noconfirm --needed mise
                return $?
            fi
            ;;
        debian)
            # Check if mise has apt repo
            log "Note: mise is not in official Debian/Ubuntu repos"
            log "Falling back to official installer..."
            ;;
        redhat)
            # Check if mise has dnf repo
            log "Note: mise is not in official Fedora/RHEL repos"
            log "Falling back to official installer..."
            ;;
    esac

    return 1
}

install_mise() {
    local os
    os=$(detect_os)
    local arch
    arch=$(detect_arch)

    log "Detected: OS=$os, Arch=$arch"

    # Check if already installed
    if command -v mise >/dev/null 2>&1; then
        local current_version
        current_version=$(mise --version 2>/dev/null || echo "unknown")
        log "mise is already installed: $current_version"

        if prompt_confirmation "Would you like to update mise?"; then
            if [[ "$DRY_RUN" == "true" ]]; then
                log "DRY-RUN: Would run: mise self-update"
            else
                mise self-update || log_warning "Self-update failed, try reinstalling"
            fi
        fi
        install_declared_runtimes
        return 0
    fi

    # Try package managers in order of preference
    log "Installing mise..."

    # Try Homebrew first (works on both macOS and Linux)
    if install_via_homebrew; then
        log_success "mise installed via Homebrew"
        post_install
        return 0
    fi

    # Try native package manager
    if install_via_package_manager "$os"; then
        log_success "mise installed via package manager"
        post_install
        return 0
    fi

    # Fall back to curl installer
    if install_via_curl; then
        log_success "mise installed via official installer"
        post_install
        return 0
    fi

    error_exit "Failed to install mise"
}

# Install the runtimes the repo's mise config declares (mise/config.toml, which
# `dotf link` symlinks to ~/.config/mise).
#
# Installing the mise BINARY is not enough: without this step node/npm/go/deno
# stay "missing" forever, and every later provisioning step that shells out to
# npm (manifest toolchain packages such as codex and lefthook) fails with
# `exec: "npm": executable file not found in $PATH`. mise is the declared source
# for those runtimes, so provisioning mise means provisioning what it declares.
#
# Failure is a warning, not fatal: a runtime download is network-bound, and one
# unavailable version must not fail the whole mise step.
install_declared_runtimes() {
    if ! command -v mise >/dev/null 2>&1; then
        return 0
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would run: mise install --yes"
        return 0
    fi

    log "Installing runtimes declared in the mise config..."
    if mise install --yes; then
        log_success "mise runtimes installed"
    else
        log_warning "Some mise runtimes failed to install; run 'mise install' manually to retry"
    fi
}

post_install() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would show post-install instructions"
        return 0
    fi

    install_declared_runtimes

    log ""
    log_success "mise installed successfully!"
    log ""
    log "Next steps:"
    log "  1. Restart your shell or run: source ~/.zshrc (or ~/.bashrc)"
    log "  2. Install a runtime: mise install node@20"
    log "  3. Set global default: mise use -g node@20"
    log ""
    log "Common commands:"
    log "  mise install <tool>@<version>  # Install a specific version"
    log "  mise use <tool>@<version>      # Use version in current directory"
    log "  mise use -g <tool>@<version>   # Set global default"
    log "  mise list                      # List installed versions"
    log "  mise current                   # Show active versions"
    log "  mise self-update               # Update mise itself"
    log ""
    log "Supported runtimes:"
    log "  Node.js, Python, Ruby, Go, Rust, Java, PHP, and 450+ more"
    log ""
    log "If migrating from NVM:"
    log "  mise use -g node@\$(node --version | tr -d 'v')"
}

# ================================================================
# USAGE
# ================================================================

show_usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install mise - a modern polyglot runtime version manager.

mise replaces nvm, pyenv, rbenv, goenv, and many others with a single,
fast tool written in Rust.

OPTIONS:
    --check, -c    Check if mise is installed and show status
    --dry-run      Show what would be done without making changes
    --force        Force installation even if already installed
    --help, -h     Show this help message

ENVIRONMENT VARIABLES:
    NON_INTERACTIVE=true    Skip confirmation prompts
    FORCE_INSTALL=true      Same as --force
    DRY_RUN=true            Same as --dry-run

EXAMPLES:
    $SCRIPT_NAME              # Install mise interactively
    $SCRIPT_NAME --check      # Check installation status
    $SCRIPT_NAME --dry-run    # Show what would be done

For more information: https://mise.jdx.dev/
EOF
}

# ================================================================
# MAIN
# ================================================================

main() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --check|-c)
                check_mise
                exit $?
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --force)
                FORCE_INSTALL=true
                shift
                ;;
            --help|-h)
                show_usage
                exit 0
                ;;
            *)
                error_exit "Unknown option: $1. Use --help for usage."
                ;;
        esac
    done

    install_mise
}

# Run main if executed directly
[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
