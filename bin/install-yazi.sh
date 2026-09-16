#!/usr/bin/env bash
# ================================================================
# Yazi Terminal File Manager Installer
# ================================================================
# Installs yazi - a blazing fast terminal file manager written in Rust
# with async I/O and image preview support.
# https://yazi-rs.github.io/
# ================================================================

set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[YAZI]"
readonly INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
readonly CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/yazi"

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
                    opensuse*|sles) echo "suse" ;;
                    *)
                        case "${ID_LIKE:-}" in
                            *debian*|*ubuntu*) echo "debian" ;;
                            *fedora*|*rhel*) echo "redhat" ;;
                            *arch*) echo "arch" ;;
                            *suse*) echo "suse" ;;
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
        x86_64|amd64) echo "x86_64" ;;
        aarch64|arm64) echo "aarch64" ;;
        armv7l) echo "armv7" ;;
        *) echo "unknown" ;;
    esac
}

# ================================================================
# CHECK FUNCTION
# ================================================================

check_yazi() {
    if command -v yazi >/dev/null 2>&1; then
        local version
        version=$(yazi --version 2>/dev/null | head -1 || echo "unknown")
        log_success "yazi is installed: $version"

        # Check for optional dependencies
        log ""
        log "Checking optional dependencies for full functionality:"

        # Image preview support
        if command -v ueberzugpp >/dev/null 2>&1; then
            log_success "  ueberzugpp (image preview): installed"
        elif command -v ueberzug >/dev/null 2>&1; then
            log_success "  ueberzug (image preview): installed"
        elif command -v chafa >/dev/null 2>&1; then
            log_success "  chafa (image preview fallback): installed"
        else
            log_warning "  Image preview: none installed (consider ueberzugpp or chafa)"
        fi

        # File preview tools
        if command -v bat >/dev/null 2>&1; then
            log_success "  bat (syntax highlighting): installed"
        else
            log_warning "  bat (syntax highlighting): not installed"
        fi

        if command -v rg >/dev/null 2>&1; then
            log_success "  ripgrep (file search): installed"
        else
            log_warning "  ripgrep (file search): not installed"
        fi

        if command -v fd >/dev/null 2>&1; then
            log_success "  fd (file finder): installed"
        else
            log_warning "  fd (file finder): not installed"
        fi

        if command -v fzf >/dev/null 2>&1; then
            log_success "  fzf (fuzzy finder): installed"
        else
            log_warning "  fzf (fuzzy finder): not installed"
        fi

        if command -v zoxide >/dev/null 2>&1; then
            log_success "  zoxide (smart cd): installed"
        else
            log_warning "  zoxide (smart cd): not installed"
        fi

        if command -v ffmpegthumbnailer >/dev/null 2>&1; then
            log_success "  ffmpegthumbnailer (video preview): installed"
        else
            log_warning "  ffmpegthumbnailer (video preview): not installed"
        fi

        if command -v poppler >/dev/null 2>&1 || command -v pdftoppm >/dev/null 2>&1; then
            log_success "  poppler (PDF preview): installed"
        else
            log_warning "  poppler (PDF preview): not installed"
        fi

        return 0
    fi

    log_warning "yazi is not installed"
    return 1
}

# ================================================================
# INSTALLATION FUNCTIONS
# ================================================================

install_via_homebrew() {
    if ! command -v brew >/dev/null 2>&1; then
        return 1
    fi

    log "Installing yazi via Homebrew..."

    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would run: brew install yazi"
        return 0
    fi

    brew install yazi
    return $?
}

install_via_cargo() {
    if ! command -v cargo >/dev/null 2>&1; then
        log_warning "Cargo not found. Install Rust first: https://rustup.rs/"
        return 1
    fi

    log "Installing yazi via Cargo..."

    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would run: cargo install --locked yazi-fm yazi-cli"
        return 0
    fi

    cargo install --locked yazi-fm yazi-cli
    return $?
}

install_via_package_manager() {
    local os="$1"

    case "$os" in
        arch)
            if command -v yay >/dev/null 2>&1; then
                log "Installing yazi via yay (AUR)..."
                if [[ "$DRY_RUN" == "true" ]]; then
                    log "DRY-RUN: Would run: yay -S yazi"
                    return 0
                fi
                yay -S yazi
                return $?
            elif command -v paru >/dev/null 2>&1; then
                log "Installing yazi via paru (AUR)..."
                if [[ "$DRY_RUN" == "true" ]]; then
                    log "DRY-RUN: Would run: paru -S yazi"
                    return 0
                fi
                paru -S yazi
                return $?
            elif command -v pacman >/dev/null 2>&1; then
                # yazi is in the official Arch repos
                log "Installing yazi via pacman..."
                if [[ "$DRY_RUN" == "true" ]]; then
                    log "DRY-RUN: Would run: sudo pacman -S yazi"
                    return 0
                fi
                sudo pacman -S yazi
                return $?
            fi
            ;;
        debian)
            # Check if available in apt (may need newer Ubuntu/Debian)
            if apt-cache show yazi >/dev/null 2>&1; then
                log "Installing yazi via apt..."
                if [[ "$DRY_RUN" == "true" ]]; then
                    log "DRY-RUN: Would run: sudo apt install yazi"
                    return 0
                fi
                sudo apt install -y yazi
                return $?
            else
                log "yazi not in apt repositories, trying alternative methods..."
            fi
            ;;
        redhat)
            # Check if available in dnf
            if dnf info yazi >/dev/null 2>&1; then
                log "Installing yazi via dnf..."
                if [[ "$DRY_RUN" == "true" ]]; then
                    log "DRY-RUN: Would run: sudo dnf install yazi"
                    return 0
                fi
                sudo dnf install -y yazi
                return $?
            else
                log "yazi not in dnf repositories, trying alternative methods..."
            fi
            ;;
        suse)
            if zypper search yazi >/dev/null 2>&1; then
                log "Installing yazi via zypper..."
                if [[ "$DRY_RUN" == "true" ]]; then
                    log "DRY-RUN: Would run: sudo zypper install yazi"
                    return 0
                fi
                sudo zypper install -y yazi
                return $?
            fi
            ;;
    esac

    return 1
}

install_via_binary() {
    local os arch
    os=$(detect_os)
    arch=$(detect_arch)

    log "Attempting to install yazi from pre-built binary..."

    # Get latest release version
    local latest_version
    latest_version=$(curl -fsSL "https://api.github.com/repos/sxyazi/yazi/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/')

    if [[ -z "$latest_version" ]]; then
        log_error "Could not determine latest yazi version"
        return 1
    fi

    log "Latest version: $latest_version"

    # Determine download URL based on OS/arch
    local download_url=""
    local archive_name=""

    case "$os" in
        macos)
            if [[ "$arch" == "aarch64" ]]; then
                archive_name="yazi-aarch64-apple-darwin.zip"
            else
                archive_name="yazi-x86_64-apple-darwin.zip"
            fi
            ;;
        *)  # Linux
            if [[ "$arch" == "aarch64" ]]; then
                archive_name="yazi-aarch64-unknown-linux-gnu.zip"
            elif [[ "$arch" == "x86_64" ]]; then
                archive_name="yazi-x86_64-unknown-linux-gnu.zip"
            else
                log_error "Unsupported architecture: $arch"
                return 1
            fi
            ;;
    esac

    download_url="https://github.com/sxyazi/yazi/releases/download/v${latest_version}/${archive_name}"

    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would download from: $download_url"
        log "DRY-RUN: Would extract to: $INSTALL_DIR"
        return 0
    fi

    # Create temp directory
    local tmp_dir
    tmp_dir=$(mktemp -d)
    # shellcheck disable=SC2064  # expand tmp_dir now, deliberately
    trap "rm -rf '$tmp_dir'" EXIT

    log "Downloading yazi v${latest_version}..."
    if ! curl -fsSL -o "$tmp_dir/yazi.zip" "$download_url"; then
        log_error "Failed to download yazi"
        return 1
    fi

    log "Extracting..."
    if ! unzip -q "$tmp_dir/yazi.zip" -d "$tmp_dir"; then
        log_error "Failed to extract archive"
        return 1
    fi

    # Find the extracted directory
    local extracted_dir
    extracted_dir=$(find "$tmp_dir" -maxdepth 1 -type d -name "yazi-*" | head -1)

    if [[ -z "$extracted_dir" ]]; then
        log_error "Could not find extracted yazi directory"
        return 1
    fi

    # Ensure install directory exists
    mkdir -p "$INSTALL_DIR"

    # Install binaries
    log "Installing to $INSTALL_DIR..."
    cp "$extracted_dir/yazi" "$INSTALL_DIR/"
    chmod +x "$INSTALL_DIR/yazi"

    if [[ -f "$extracted_dir/ya" ]]; then
        cp "$extracted_dir/ya" "$INSTALL_DIR/"
        chmod +x "$INSTALL_DIR/ya"
    fi

    log_success "yazi installed to $INSTALL_DIR"
    return 0
}

install_yazi() {
    local os
    os=$(detect_os)
    local arch
    arch=$(detect_arch)

    log "Detected: OS=$os, Arch=$arch"

    # Check if already installed
    if command -v yazi >/dev/null 2>&1; then
        local current_version
        current_version=$(yazi --version 2>/dev/null | head -1 || echo "unknown")
        log "yazi is already installed: $current_version"

        if prompt_confirmation "Would you like to reinstall/update yazi?"; then
            log "Proceeding with reinstallation..."
        else
            return 0
        fi
    fi

    log "Installing yazi..."

    # Try Homebrew first (works on both macOS and Linux)
    if install_via_homebrew; then
        log_success "yazi installed via Homebrew"
        post_install
        return 0
    fi

    # Try native package manager
    if install_via_package_manager "$os"; then
        log_success "yazi installed via package manager"
        post_install
        return 0
    fi

    # Try pre-built binary
    if install_via_binary; then
        log_success "yazi installed via pre-built binary"
        post_install
        return 0
    fi

    # Fall back to cargo
    if install_via_cargo; then
        log_success "yazi installed via Cargo"
        post_install
        return 0
    fi

    error_exit "Failed to install yazi. Please install manually: https://yazi-rs.github.io/docs/installation"
}

install_optional_deps() {
    local os
    os=$(detect_os)

    log ""
    log "Installing optional dependencies for enhanced functionality..."

    case "$os" in
        macos)
            if command -v brew >/dev/null 2>&1; then
                log "Installing via Homebrew..."
                if [[ "$DRY_RUN" == "true" ]]; then
                    log "DRY-RUN: Would run: brew install ffmpegthumbnailer poppler"
                else
                    brew install ffmpegthumbnailer poppler 2>/dev/null || true
                fi
            fi
            ;;
        arch)
            local pkgs="ffmpegthumbnailer poppler"
            if command -v yay >/dev/null 2>&1; then
                if [[ "$DRY_RUN" == "true" ]]; then
                    log "DRY-RUN: Would run: yay -S --needed $pkgs"
                else
                    yay -S --needed $pkgs 2>/dev/null || true
                fi
            elif command -v pacman >/dev/null 2>&1; then
                if [[ "$DRY_RUN" == "true" ]]; then
                    log "DRY-RUN: Would run: sudo pacman -S --needed $pkgs"
                else
                    sudo pacman -S --needed $pkgs 2>/dev/null || true
                fi
            fi
            ;;
        debian)
            if [[ "$DRY_RUN" == "true" ]]; then
                log "DRY-RUN: Would run: sudo apt install -y ffmpegthumbnailer poppler-utils"
            else
                sudo apt install -y ffmpegthumbnailer poppler-utils 2>/dev/null || true
            fi
            ;;
        redhat)
            if [[ "$DRY_RUN" == "true" ]]; then
                log "DRY-RUN: Would run: sudo dnf install -y ffmpegthumbnailer poppler-utils"
            else
                sudo dnf install -y ffmpegthumbnailer poppler-utils 2>/dev/null || true
            fi
            ;;
    esac
}

post_install() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would show post-install instructions"
        return 0
    fi

    log ""
    log_success "yazi installed successfully!"
    log ""
    log "Shell integration (already configured in dotfiles):"
    log "  The 'yy' function allows changing directory on exit."
    log "  Usage: yy [path]  # Opens yazi, changes to selected dir on quit"
    log ""
    log "Keybindings in yazi:"
    log "  j/k        - Navigate up/down"
    log "  h/l        - Navigate parent/child"
    log "  Enter      - Open file/directory"
    log "  Space      - Select file"
    log "  y          - Yank (copy)"
    log "  x          - Cut"
    log "  p          - Paste"
    log "  d          - Delete"
    log "  a          - Create file"
    log "  A          - Create directory"
    log "  r          - Rename"
    log "  .          - Toggle hidden files"
    log "  z          - Jump with zoxide"
    log "  /          - Search"
    log "  q          - Quit"
    log ""
    log "Configuration:"
    log "  Config directory: $CONFIG_DIR"
    log "  Main config: $CONFIG_DIR/yazi.toml"
    log "  Keymap: $CONFIG_DIR/keymap.toml"
    log "  Theme: $CONFIG_DIR/theme.toml"
    log ""
    log "For image preview support, consider installing:"
    log "  - ueberzugpp (Linux terminal image preview)"
    log "  - chafa (fallback, works in most terminals)"
    log ""
    log "Documentation: https://yazi-rs.github.io/docs/quick-start"
}

# ================================================================
# USAGE
# ================================================================

show_usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install yazi - a blazing fast terminal file manager written in Rust.

yazi features async I/O, image preview, and vim-like keybindings for
efficient file management in the terminal.

OPTIONS:
    --check, -c       Check if yazi is installed and show status
    --deps            Also install optional dependencies
    --dry-run         Show what would be done without making changes
    --force           Force installation even if already installed
    --help, -h        Show this help message

ENVIRONMENT VARIABLES:
    NON_INTERACTIVE=true    Skip confirmation prompts
    FORCE_INSTALL=true      Same as --force
    DRY_RUN=true            Same as --dry-run
    INSTALL_DIR=path        Custom installation directory (default: ~/.local/bin)

EXAMPLES:
    $SCRIPT_NAME              # Install yazi interactively
    $SCRIPT_NAME --check      # Check installation status
    $SCRIPT_NAME --deps       # Install with optional dependencies
    $SCRIPT_NAME --dry-run    # Show what would be done

For more information: https://yazi-rs.github.io/
EOF
}

# ================================================================
# MAIN
# ================================================================

main() {
    local install_deps=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --check|-c)
                check_yazi
                exit $?
                ;;
            --deps)
                install_deps=true
                shift
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

    install_yazi

    if [[ "$install_deps" == "true" ]]; then
        install_optional_deps
    fi
}

# Run main if executed directly
[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
