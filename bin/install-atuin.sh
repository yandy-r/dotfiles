#!/usr/bin/env bash
set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[ATUIN]"

# Desktop installation flag - will be auto-detected
INSTALL_DESKTOP=false

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

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

# Clean up shell config modifications made by installers
# This removes PATH exports and other modifications that installers might add
cleanup_shell_config_modifications() {
	local tool_name="$1"
	local shell_configs=(
		"$HOME/.zshrc"
		"$HOME/.bashrc"
		"$HOME/.bash_profile"
		"$HOME/.profile"
	)

	for config in "${shell_configs[@]}"; do
		if [[ -f "$config" ]]; then
			# Create backup
			local backup
			backup="${config}.backup.$(date +%Y%m%d_%H%M%S)"

			# Remove lines containing the tool's PATH export
			# This handles various formats: export PATH=...tool..., # tool, etc.
			# Use 'command grep' to bypass any grep aliases (e.g., rg/ripgrep)
			if command grep -q "$tool_name" "$config" 2>/dev/null; then
				log "Cleaning up shell config modifications in $config"
				cp "$config" "$backup"

				# Remove lines related to this tool (PATH exports, comments, etc.)
				sed -i.tmp "/# ${tool_name}/d; /.${tool_name}\/bin/d; /${tool_name}_INSTALL/d" "$config" 2>/dev/null || \
					command grep -v -e "# ${tool_name}" -e ".${tool_name}/bin" -e "${tool_name}_INSTALL" "$config" > "${config}.tmp" && mv "${config}.tmp" "$config"

				rm -f "${config}.tmp"
				log "Backup created at: $backup"
			fi
		fi
	done
}

detect_desktop_environment() {
    # Check if running in a desktop environment (GUI available)
    local os_type
    os_type="$(uname -s)"

    case "$os_type" in
        Darwin*)
            # macOS always has GUI available
            return 0
            ;;
        Linux*)
            # Check for X11
            if [[ -n "${DISPLAY:-}" ]] || [[ -n "${WAYLAND_DISPLAY:-}" ]]; then
                return 0
            fi

            # Check for common desktop environments
            if [[ -n "${XDG_CURRENT_DESKTOP:-}" ]] || [[ -n "${DESKTOP_SESSION:-}" ]]; then
                return 0
            fi

            # Check for X11 or Wayland session type
            if [[ "${XDG_SESSION_TYPE:-}" == "x11" ]] || [[ "${XDG_SESSION_TYPE:-}" == "wayland" ]]; then
                return 0
            fi

            # Running in terminal without GUI
            return 1
            ;;
        *)
            # Windows or other - assume no desktop for this script
            return 1
            ;;
    esac
}

detect_gui_support() {
    # Legacy function - kept for compatibility with get_desktop_platform
    detect_desktop_environment
}

get_desktop_platform() {
    local os_type arch
    os_type="$(uname -s)"
    arch="$(uname -m)"

    case "$os_type" in
        Darwin*)
            case "$arch" in
                arm64|aarch64) echo "dmg-aarch64" ;;
                x86_64) echo "dmg-x86_64" ;;
                *) echo "" ;;
            esac
            ;;
        Linux*)
            if ! detect_gui_support; then
                return 1
            fi
            case "$arch" in
                x86_64) echo "appimage-x86_64" ;;
                aarch64|arm64) echo "appimage-aarch64" ;;
                *) echo "" ;;
            esac
            ;;
        MINGW*|MSYS*|CYGWIN*)
            # Windows via Git Bash/MSYS2
            case "$arch" in
                x86_64) echo "msi-x86_64" ;;
                *) echo "" ;;
            esac
            ;;
        *)
            echo ""
            ;;
    esac
}



install_atuin_desktop() {
    local platform download_url temp_file

    platform="$(get_desktop_platform)"

    if [[ -z "$platform" ]]; then
        log_warning "Desktop installation not supported on this platform"
        return 0
    fi

    log "Installing atuin desktop..."
    download_url="https://cdn.crabnebula.app/download/atuin/atuin-desktop/latest/platform/$platform"

    case "$platform" in
        dmg-*)
            temp_file="/tmp/atuin-desktop.dmg"
            log "Downloading atuin desktop from $download_url..."

            if ! curl -L --fail "$download_url" -o "$temp_file"; then
                log_warning "Failed to download atuin desktop"
                return 0
            fi

            log "Mounting DMG..."
            local mount_point
            mount_point=$(hdiutil attach "$temp_file" | grep Volumes | awk '{print $3}')

            if [[ -z "$mount_point" ]]; then
                log_error "Failed to mount DMG"
                rm -f "$temp_file"
                return 1
            fi

            log "Installing atuin desktop to /Applications..."
            cp -R "$mount_point"/*.app /Applications/ 2>/dev/null || {
                log_error "Failed to copy app to /Applications"
                hdiutil detach "$mount_point" 2>/dev/null
                rm -f "$temp_file"
                return 1
            }

            log "Unmounting DMG..."
            hdiutil detach "$mount_point" >/dev/null 2>&1
            rm -f "$temp_file"

            log_success "atuin desktop installed successfully"
            ;;

        appimage-*)
            temp_file="$HOME/.local/bin/atuin-desktop.AppImage"
            mkdir -p "$HOME/.local/bin"

            log "Downloading atuin desktop from $download_url..."
            if ! curl -L --fail "$download_url" -o "$temp_file"; then
                log_warning "Failed to download atuin desktop"
                return 0
            fi

            chmod +x "$temp_file"

            # Create desktop entry
            local desktop_file="$HOME/.local/share/applications/atuin-desktop.desktop"
            mkdir -p "$HOME/.local/share/applications"

            cat > "$desktop_file" << EOF
[Desktop Entry]
Name=Atuin Desktop
Exec=$temp_file
Icon=atuin
Type=Application
Categories=Utility;
EOF

            log_success "atuin desktop installed successfully"
            log "You can launch it from your application menu or run: $temp_file"
            ;;

        msi-*)
            temp_file="/tmp/atuin-desktop.msi"
            log "Downloading atuin desktop from $download_url..."

            if ! curl -L --fail "$download_url" -o "$temp_file"; then
                log_warning "Failed to download atuin desktop"
                return 0
            fi

            log "Please run the downloaded installer: $temp_file"
            log_warning "Automatic installation on Windows requires administrator privileges"
            log_success "Installer downloaded successfully"
            ;;

        *)
            log_warning "Unsupported desktop platform: $platform"
            return 0
            ;;
    esac
}

update_atuin() {
    log "Updating atuin to latest version..."

    # Get current version
    local current_version
    current_version="$(atuin --version 2>/dev/null || echo "unknown")"
    log "Current version: $current_version"

    # Try Homebrew first if available
    if command -v brew >/dev/null 2>&1; then
        log "Attempting update via Homebrew..."
        if brew upgrade atuin 2>/dev/null; then
            local new_version
            new_version="$(atuin --version 2>/dev/null || echo "unknown")"

            if [[ "$new_version" != "$current_version" ]]; then
                log_success "atuin updated successfully via Homebrew"
                log "New version: $new_version"
            else
                log_success "atuin is already at the latest version (Homebrew)"
            fi

            cleanup_shell_config_modifications "atuin"
            return 0
        else
            log_warning "Homebrew upgrade failed or atuin not installed via Homebrew"
        fi
    fi

    # Fall back to official installer
    log "Attempting update via official installer..."
    mkdir -p "$HOME/.local/bin"

    if ATUIN_INSTALL_DIR="$HOME/.local" \
        SKIP_SHELL_INTEGRATION=1 \
        NO_UPDATE_PROFILE=1 \
        FORCE_INSTALL="true" \
        curl --proto '=https' --tlsv1.2 -LsSf https://setup.atuin.sh | sh; then

        local new_version
        new_version="$(atuin --version 2>/dev/null || echo "unknown")"

        if [[ "$new_version" != "$current_version" ]]; then
            log_success "atuin updated successfully via official installer"
            log "New version: $new_version"
        else
            log_success "atuin is already at the latest version"
        fi

        cleanup_shell_config_modifications "atuin"
        return 0
    else
        log_error "Failed to update atuin via official installer"
        return 1
    fi
}

install_atuin() {
    local os_type
    os_type="$(detect_os)"

    if command -v atuin >/dev/null 2>&1; then
        log "atuin is already installed ($(atuin --version))"
        log "Updating to latest version..."
        update_atuin
        return 0
    fi

    log "Installing atuin..."

    # Try official installer first (preferred method for all platforms)
    # Skip security prompt if FORCE_INSTALL is set (e.g., during bootstrap)
    if [[ "${FORCE_INSTALL:-false}" != "true" ]]; then
        log "Installing atuin via official installer..."
        log "This will install atuin CLI to your system"
        echo
        log_warning "⚠️  SECURITY WARNING ⚠️"
        log_warning "This will download and execute a remote script from:"
        log_warning "    https://setup.atuin.sh"
        log_warning "The script will be piped directly to your shell."
        echo

        if ! prompt_confirmation "Do you want to proceed with downloading and executing the remote script?"; then
            log_warning "Installation cancelled by user, trying Homebrew instead..."
            # Fall through to Homebrew
        else
            # User confirmed, try official installer
            mkdir -p "$HOME/.local/bin"

            if ATUIN_INSTALL_DIR="$HOME/.local" \
                SKIP_SHELL_INTEGRATION=1 \
                NO_UPDATE_PROFILE=1 \
                curl --proto '=https' --tlsv1.2 -LsSf https://setup.atuin.sh | sh; then

                cleanup_shell_config_modifications "atuin"
                log_success "atuin installed successfully via official installer"
                return 0
            else
                log_warning "Official installer failed, trying Homebrew..."
            fi
        fi
    else
        # FORCE_INSTALL mode - skip prompt and try official installer
        log "Installing atuin via official installer (force mode)..."
        mkdir -p "$HOME/.local/bin"

        if ATUIN_INSTALL_DIR="$HOME/.local" \
            SKIP_SHELL_INTEGRATION=1 \
            NO_UPDATE_PROFILE=1 \
            curl --proto '=https' --tlsv1.2 -LsSf https://setup.atuin.sh | sh; then

            cleanup_shell_config_modifications "atuin"
            log_success "atuin installed successfully via official installer"
            return 0
        else
            log_warning "Official installer failed, trying Homebrew..."
        fi
    fi

    # Try Homebrew as fallback
    if command -v brew >/dev/null 2>&1; then
        log "Installing atuin via Homebrew..."
        if brew install atuin; then
            log_success "atuin installed via Homebrew"
            return 0
        else
            error_exit "All installation methods failed for atuin"
        fi
    else
        error_exit "Failed to install atuin - no installation methods available"
    fi
}

configure_atuin() {
    local config_dir="$HOME/.config/atuin"
    local config_file="$config_dir/config.toml"

    mkdir -p "$config_dir"

    if [[ -f "$config_file" ]]; then
        log "atuin config already exists at $config_file"
        return 0
    fi

    log "Creating atuin configuration..."

    cat > "$config_file" << 'EOF'
[sync]
records = true

[dotfiles]
enabled = true

[search]
inline_height = 20

[keys]
scroll_exits = true

[stats]
common_subcommands = [
    "apt",
    "cargo",
    "composer",
    "dnf",
    "docker",
    "git",
    "go",
    "ip",
    "kubectl",
    "nix",
    "nmcli",
    "npm",
    "pecl",
    "pnpm",
    "podman",
    "port",
    "systemctl",
    "tmux",
    "yarn",
]
common_prefix = ["sudo"]
EOF

    log_success "atuin configuration created at $config_file"
    log "You can customize it by editing $config_file"
}

import_existing_history() {
    if ! command -v atuin >/dev/null 2>&1; then
        log_warning "atuin not found in PATH, skipping history import"
        return 0
    fi

    log "Importing existing shell history..."
    if atuin import auto 2>/dev/null; then
        log_success "Shell history imported successfully"
    else
        log_warning "Could not import existing history (this is normal if history is empty)"
    fi
}

show_next_steps() {
    log_success "atuin installation completed!"
    echo
    log "Next steps:"
    log "1. Restart your shell or run: exec \$SHELL"
    log "2. (Optional) Register for sync: atuin register -u <USERNAME> -e <EMAIL>"
    log "3. (Optional) Setup sync: atuin sync"
    log "4. Use Ctrl-R or Up Arrow to search history"

    if [[ "$INSTALL_DESKTOP" == true ]] && [[ -n "$(get_desktop_platform)" ]]; then
        echo
        log "atuin Desktop:"
        case "$(uname -s)" in
            Darwin*)
                log "  - Launch from Applications folder"
                ;;
            Linux*)
                log "  - Launch from your application menu or run: ~/.local/bin/atuin-desktop.AppImage"
                ;;
            MINGW*|MSYS*|CYGWIN*)
                log "  - Run the downloaded MSI installer to complete installation"
                ;;
        esac
    fi

    echo
    log "Configuration file: ~/.config/atuin/config.toml"
    log "Documentation: https://docs.atuin.sh"
}

show_usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install atuin CLI and desktop application.

OPTIONS:
    --desktop           Force desktop application installation (auto-detected by default)
    --no-desktop        Skip desktop application installation
    -h, --help          Show this help message

EXAMPLES:
    $SCRIPT_NAME                    # Auto-detect and install accordingly
    $SCRIPT_NAME --no-desktop       # Install CLI only
    $SCRIPT_NAME --desktop          # Force desktop installation

NOTES:
    - Desktop environment is auto-detected
    - macOS: Desktop version always available
    - Linux: Checks for X11/Wayland session
    - Headless/SSH: CLI only installation

EOF
}

main() {
    local desktop_override=false

    # Parse command-line arguments first
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --no-desktop)
                INSTALL_DESKTOP=false
                desktop_override=true
                shift
                ;;
            --desktop)
                INSTALL_DESKTOP=true
                desktop_override=true
                shift
                ;;
            -h|--help)
                show_usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done

    # Auto-detect desktop environment if not explicitly set via arguments
    if [[ "$desktop_override" == "false" ]]; then
        if detect_desktop_environment; then
            INSTALL_DESKTOP=true
            log "Desktop environment detected - will install desktop application"
        else
            INSTALL_DESKTOP=false
            log "No desktop environment detected - CLI only installation"
        fi
    elif [[ "$INSTALL_DESKTOP" == "true" ]]; then
        log "Desktop installation forced via --desktop flag"
    else
        log "Desktop installation disabled via --no-desktop flag"
    fi

    log "Starting atuin installation..."
    echo

    install_atuin

    if ! command -v atuin >/dev/null 2>&1; then
        export PATH="$HOME/.atuin/bin:$PATH"
    fi

    configure_atuin
    import_existing_history

    # Install desktop application if enabled and supported
    if [[ "$INSTALL_DESKTOP" == true ]]; then
        echo
        install_atuin_desktop || log_warning "Desktop installation failed, but CLI installation succeeded"
    fi

    show_next_steps

    return 0
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
