#!/usr/bin/env bash
set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[TEA]"

# tea is the official command-line client for Gitea/Forgejo servers.
# Upstream: https://gitea.com/gitea/tea

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

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
        x86_64|amd64) echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *) echo "unknown" ;;
    esac
}

# Check if running in interactive mode
is_interactive() {
    [[ -t 0 ]] && [[ -t 1 ]]
}

# Prompt user for confirmation
prompt_confirmation() {
    local prompt_message="$1"
    local response

    # Auto-approve in non-interactive mode
    if [[ "${NON_INTERACTIVE:-false}" == "true" ]] || [[ "${FORCE_INSTALL:-false}" == "true" ]] || [[ "${NONINTERACTIVE:-0}" == "1" ]]; then
        log "Non-interactive mode: auto-approving prompt"
        return 0
    fi

    if ! is_interactive; then
        log_error "Non-interactive shell detected - cannot prompt for confirmation"
        return 1
    fi

    echo -n -e "${YELLOW}${LOG_PREFIX} ${prompt_message} [y/N]: ${NC}" >&2
    read -r response

    # Default to "no" on empty input
    response="${response:-n}"

    case "$response" in
        [yY]|[yY][eE][sS])
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

# Resolve the latest published tea release tag (e.g. "v0.14.1") via the Gitea API.
get_latest_tag() {
    local api_url="https://gitea.com/api/v1/repos/gitea/tea/releases/latest"
    curl -fsSL "$api_url" 2>/dev/null |
        grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' |
        head -1 |
        sed 's/.*"\([^"]*\)"$/\1/'
}

# Install tea by downloading the release binary from gitea.com (used on
# platforms without a native tea package: Debian/Ubuntu, Fedora/RHEL, openSUSE, WSL).
install_tea_binary() {
    local os_type arch
    os_type="$(detect_os)"
    arch="$(detect_arch)"

    if [[ "$arch" == "unknown" ]]; then
        error_exit "Unsupported architecture: $(uname -m)"
    fi

    # tea release assets use darwin/linux in their filenames.
    local asset_os
    case "$os_type" in
        macos) asset_os="darwin" ;;
        *) asset_os="linux" ;;
    esac

    log "Resolving latest tea release..."
    local tag version
    tag="$(get_latest_tag)"
    if [[ -z "$tag" ]]; then
        error_exit "Failed to determine latest tea release tag"
    fi
    version="${tag#v}"
    log "Latest tea version: $version"

    local asset="tea-${version}-${asset_os}-${arch}"
    local base_url="https://gitea.com/gitea/tea/releases/download/${tag}"
    local download_url="${base_url}/${asset}"

    local install_dir="$HOME/.local/bin"
    mkdir -p "$install_dir"

    local temp_dir
    temp_dir="$(mktemp -d)"
    local tea_binary="$temp_dir/tea"

    log "Downloading from: $download_url"
    if ! curl -fsSL "$download_url" -o "$tea_binary"; then
        rm -rf "$temp_dir"
        error_exit "Failed to download tea binary"
    fi

    # Attempt to download and verify checksum from the release-wide checksums.txt.
    local checksum_url="${base_url}/checksums.txt"
    local checksum_file="$temp_dir/checksums.txt"

    log "Attempting to download checksums from: $checksum_url"
    if curl -fsSL "$checksum_url" -o "$checksum_file" 2>/dev/null; then
        log "Checksums downloaded, verifying binary integrity..."

        # checksums.txt lines look like: "<sha256>  tea-<ver>-<os>-<arch>"
        local expected_checksum actual_checksum
        expected_checksum="$(grep -E "[[:space:]]\*?${asset}\$" "$checksum_file" | awk '{print $1}' | head -1)"

        if [[ -z "$expected_checksum" ]]; then
            log_warning "No checksum entry found for $asset"
        elif command -v sha256sum >/dev/null 2>&1; then
            actual_checksum="$(sha256sum "$tea_binary" | awk '{print $1}')"
        elif command -v shasum >/dev/null 2>&1; then
            actual_checksum="$(shasum -a 256 "$tea_binary" | awk '{print $1}')"
        else
            log_warning "Neither sha256sum nor shasum found - cannot verify checksum"
            if [[ "${FORCE_INSTALL:-false}" != "true" ]]; then
                if ! prompt_confirmation "Do you want to proceed without checksum verification?"; then
                    rm -rf "$temp_dir"
                    error_exit "Installation cancelled - checksum verification not possible"
                fi
            else
                log_warning "Proceeding without checksum verification (force mode)"
            fi
        fi

        if [[ -n "${expected_checksum:-}" ]] && [[ -n "${actual_checksum:-}" ]]; then
            if [[ "$expected_checksum" == "$actual_checksum" ]]; then
                log_success "Checksum verification successful"
            else
                rm -rf "$temp_dir"
                error_exit "Checksum verification failed! Expected: $expected_checksum, Got: $actual_checksum"
            fi
        fi
    else
        log_warning "Checksums file not available at: $checksum_url"
        if [[ "${FORCE_INSTALL:-false}" != "true" ]]; then
            log_warning "This could pose a security risk if the download was compromised"
            if ! prompt_confirmation "Do you want to proceed without checksum verification?"; then
                rm -rf "$temp_dir"
                error_exit "Installation cancelled - checksum verification not available"
            fi
        else
            log_warning "Proceeding without checksum verification (force mode)"
        fi
    fi

    chmod +x "$tea_binary"

    if ! mv "$tea_binary" "$install_dir/tea"; then
        rm -rf "$temp_dir"
        error_exit "Failed to install tea to $install_dir"
    fi

    rm -rf "$temp_dir"
    log_success "tea installed successfully to $install_dir/tea"

    if [[ ":$PATH:" != *":$install_dir:"* ]]; then
        log_warning "$install_dir is not in your PATH"
        log_warning "Add it to your shell config: export PATH=\"$install_dir:\$PATH\""
    fi
}

install_tea() {
    local os_type
    os_type="$(detect_os)"

    if command -v tea >/dev/null 2>&1 && [[ "${FORCE_INSTALL:-false}" != "true" ]]; then
        log "tea is already installed ($(tea --version 2>/dev/null | head -1))"
        return 0
    fi

    log "Installing tea CLI..."

    # Arch Linux: tea is in the official 'extra' repository.
    if [[ "$os_type" == "arch" ]] && command -v pacman >/dev/null 2>&1; then
        log "Installing tea via pacman..."
        if sudo pacman -S --needed --noconfirm tea; then
            log_success "tea installed via pacman"
            return 0
        fi
        log_warning "pacman installation failed, trying other methods..."
    fi

    # macOS / Linuxbrew: homebrew-core ships the Gitea 'tea' formula.
    if [[ "$os_type" == "macos" ]] || command -v brew >/dev/null 2>&1; then
        log "Installing tea via Homebrew..."
        if brew install tea; then
            log_success "tea installed via Homebrew"
            return 0
        fi
        log_warning "Homebrew installation failed, falling back to binary install..."
    fi

    # Everything else (Debian/Ubuntu, Fedora/RHEL, openSUSE, WSL): binary download.
    install_tea_binary
}

setup_shell_completions() {
    if ! command -v tea >/dev/null 2>&1; then
        log_warning "tea not found in PATH, skipping shell completions"
        return 0
    fi

    log "Setting up shell completions..."

    # Zsh: write the #compdef autoload file into the fpath completions dir.
    local zsh_completions_dir="$HOME/.zsh/completions"
    mkdir -p "$zsh_completions_dir"
    if tea completion zsh > "$zsh_completions_dir/_tea" 2>/dev/null; then
        log_success "Zsh completions installed to $zsh_completions_dir/_tea"
    else
        log_warning "Failed to generate zsh completions (may not be supported)"
    fi

    # Bash: write the sourceable completion script.
    if command -v bash >/dev/null 2>&1; then
        local bash_completions_dir="$HOME/.bash_completions"
        mkdir -p "$bash_completions_dir"
        if tea completion bash > "$bash_completions_dir/tea" 2>/dev/null; then
            log_success "Bash completions installed to $bash_completions_dir/tea"
        else
            log_warning "Failed to generate bash completions (may not be supported)"
        fi
    fi

    # Fish: written to the standard auto-discovery dir (works even if fish
    # isn't installed yet — fish picks it up on first launch).
    local fish_completions_dir="$HOME/.config/fish/completions"
    mkdir -p "$fish_completions_dir"
    if tea completion fish > "$fish_completions_dir/tea.fish" 2>/dev/null; then
        log_success "Fish completions installed to $fish_completions_dir/tea.fish"
    else
        log_warning "Failed to generate fish completions (may not be supported)"
    fi
}

show_next_steps() {
    log_success "tea installation completed!"
    echo
    log "Next steps:"
    log "1. Restart your shell or run: exec \$SHELL"
    log "2. Add a Gitea/Forgejo login: tea login add"
    log "3. List your logins: tea login list"
    log "4. Explore repos: tea repos list"
    echo
    log "Documentation: https://gitea.com/gitea/tea"
}

main() {
    log "Starting tea installation..."
    echo

    install_tea

    if ! command -v tea >/dev/null 2>&1; then
        export PATH="$HOME/.local/bin:$PATH"
    fi

    setup_shell_completions
    show_next_steps
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
