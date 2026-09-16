#!/usr/bin/env bash
set -euo pipefail

# Install CodeRabbit CLI
# Integrates with Claude Code for AI-powered code reviews
# Supports macOS, Linux, and WSL

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[CODERABBIT]"

# Configuration
ACTION="${ACTION:-install}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"

# Color codes
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly NC='\033[0m'

# ================================================================
# LOGGING FUNCTIONS
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

# ================================================================
# USAGE AND HELP
# ================================================================

usage() {
    cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install or uninstall CodeRabbit CLI for AI-powered code reviews.

OPTIONS:
    --install           Install CodeRabbit CLI (default)
    --uninstall         Uninstall CodeRabbit CLI
    --force             Force installation even if already installed
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    ACTION             Action to perform (install, uninstall)
    FORCE_INSTALL      Force installation (true/false)

EXAMPLES:
    $SCRIPT_NAME --install
    $SCRIPT_NAME --uninstall
    $SCRIPT_NAME --force

PREREQUISITES:
    - curl and bash for installation
    - Claude Code CLI should be installed for full integration
    - On Windows: WSL (Windows Subsystem for Linux) is required

POST-INSTALLATION:
    After installation, authenticate within Claude Code by running:

        Run: coderabbit auth login

    Open the displayed URL in your browser, log in to CodeRabbit,
    and paste the authentication token back into Claude Code.

    Authentication persists across Claude Code instances.

DOCUMENTATION:
    https://docs.coderabbit.ai/cli/claude-code-integration
EOF
    exit "${1:-0}"
}

# ================================================================
# PLATFORM DETECTION
# ================================================================

detect_os() {
    case "$(uname -s)" in
        Darwin*) echo "macos" ;;
        Linux*)
            if grep -qi microsoft /proc/version 2>/dev/null; then
                echo "wsl"
            else
                echo "linux"
            fi
            ;;
        CYGWIN*|MINGW*|MSYS*)
            echo "windows"
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "x86_64" ;;
        arm64|aarch64) echo "arm64" ;;
        *) echo "unknown" ;;
    esac
}

# ================================================================
# CHECK FUNCTIONS
# ================================================================

check_coderabbit_installed() {
    if command -v coderabbit &>/dev/null; then
        local version
        version="$(coderabbit --version 2>/dev/null || echo "unknown")"
        log "CodeRabbit CLI is already installed: ${version}"
        return 0
    else
        return 1
    fi
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

check_prerequisites() {
    log "Checking prerequisites..."

    # Check for curl
    if ! command -v curl &>/dev/null; then
        error_exit "curl is required but not installed. Please install curl first."
    fi

    # Check for Claude Code (optional but recommended)
    if ! command -v claude &>/dev/null; then
        log_warning "Claude Code CLI not found"
        log_warning "CodeRabbit integrates with Claude Code for best experience"
        log_warning "Install Claude Code first for full functionality"
    else
        log_success "Claude Code CLI detected: $(claude --version 2>/dev/null || echo 'version unknown')"
    fi

    # Check for WSL on Windows
    local os_type
    os_type="$(detect_os)"
    if [[ "$os_type" == "windows" ]]; then
        log_warning "Windows detected: WSL is required for CodeRabbit CLI"
        log_warning "Please ensure you're running this script in WSL"
    fi

    log_success "Prerequisites check completed"
}

# ================================================================
# INSTALLATION FUNCTIONS
# ================================================================

install_coderabbit() {
    log "Installing CodeRabbit CLI..."

    local os_type arch
    os_type="$(detect_os)"
    arch="$(detect_arch)"

    log "Detected OS: ${os_type}, Architecture: ${arch}"

    # Verify platform compatibility
    case "$os_type" in
        macos|linux|wsl)
            log "Platform is supported"
            ;;
        windows)
            error_exit "Windows detected. Please run this script in WSL (Windows Subsystem for Linux)"
            ;;
        *)
            error_exit "Unsupported operating system: ${os_type}"
            ;;
    esac

    # Download and run the official installer
    log "Downloading and running CodeRabbit installer..."
    log "This will install CodeRabbit CLI to your system"
    echo
    log_warning "⚠️  SECURITY WARNING ⚠️"
    log_warning "This will download and execute a remote script from:"
    log_warning "    https://cli.coderabbit.ai/install.sh"
    log_warning "The script will be piped directly to your shell."
    echo

    if ! prompt_confirmation "Do you want to proceed with downloading and executing the remote script?"; then
        log "Installation cancelled by user"
        exit 0
    fi

    # Judge the installer by what it produced, not by its exit code: the upstream
    # install.sh prints "Installation complete" and leaves a working binary at
    # ~/.local/bin/coderabbit while still exiting 1 (verified against v0.7.5).
    # Trusting the status alone fails the whole package on a successful install.
    if ! curl -fsSL https://cli.coderabbit.ai/install.sh | sh; then
        if [[ ! -x "$HOME/.local/bin/coderabbit" ]] && ! command -v coderabbit &>/dev/null; then
            error_exit "Failed to install CodeRabbit CLI"
        fi
        log_warning "Upstream installer exited non-zero but the coderabbit binary is present; continuing"
    fi

    # Clean up any shell config modifications made by the installer
    cleanup_shell_config_modifications "coderabbit"

    log_success "CodeRabbit CLI installed successfully"
}

verify_installation() {
    log "Verifying CodeRabbit CLI installation..."

    if ! command -v coderabbit &>/dev/null; then
        log_error "ERROR: coderabbit not found in PATH"
        log "Please restart your shell or run: source ~/.zshrc"
        return 1
    fi

    local version
    version="$(coderabbit --version 2>/dev/null || echo "unknown")"
    log "Installation verified: ${version}"
    log ""
    log_success "✓ Installation successful!"
    log "Location: $(command -v coderabbit)"
}

# ================================================================
# UNINSTALL FUNCTION
# ================================================================

uninstall_coderabbit() {
    log "Uninstalling CodeRabbit CLI..."

    if ! command -v coderabbit &>/dev/null; then
        log "CodeRabbit CLI is not installed"
        return 0
    fi

    # Find CodeRabbit installation directory
    local coderabbit_path
    coderabbit_path="$(command -v coderabbit)"

    if [[ -z "$coderabbit_path" ]]; then
        log_warning "Could not determine CodeRabbit installation path"
        return 1
    fi

    log "Found CodeRabbit at: $coderabbit_path"
    log "Creating backup before uninstalling..."

    # Create backup
    local backup_path
    backup_path="${HOME}/.coderabbit_backup_$(date +%Y%m%d_%H%M%S)"
    if ! cp "$coderabbit_path" "$backup_path" 2>/dev/null; then
        log_warning "Failed to create backup"
    else
        log_success "Backup created at: $backup_path"
    fi

    # Remove the binary
    if [[ -w "$coderabbit_path" ]]; then
        rm -f "$coderabbit_path"
    else
        log_warning "Elevated privileges required to remove CodeRabbit binary"
        log "The following command will be executed:"
        log "    sudo rm -f $coderabbit_path"
        echo

        if ! prompt_confirmation "Do you want to proceed with sudo?"; then
            log_error "Cannot remove CodeRabbit without elevated privileges"
            log "To remove manually, run: sudo rm -f $coderabbit_path"
            return 1
        fi

        if ! sudo rm -f "$coderabbit_path"; then
            log_error "Failed to remove CodeRabbit binary"
            return 1
        fi
    fi

    # Clean up configuration (optional)
    if [[ -d "$HOME/.coderabbit" ]]; then
        log "Configuration directory found at: $HOME/.coderabbit"
        log "Keeping configuration for potential reinstall"
        log "To remove configuration, manually delete: rm -rf $HOME/.coderabbit"
    fi

    log_success "CodeRabbit CLI uninstalled successfully"
}

# ================================================================
# POST-INSTALLATION
# ================================================================

show_next_steps() {
    log_success "CodeRabbit CLI installation completed!"
    echo
    log "Next steps:"
    log "1. Restart your shell or run: exec \$SHELL"
    log "2. Verify installation: coderabbit --version"
    log "3. Authenticate with CodeRabbit (in Claude Code):"
    log "   - Ask Claude: 'Run: coderabbit auth login'"
    log "   - Open the displayed URL in your browser"
    log "   - Log in to CodeRabbit and copy the token"
    log "   - Paste the token back into Claude Code"
    log "4. Verify authentication: coderabbit auth status"
    echo
    log "Integration with Claude Code:"
    log "  - CodeRabbit will now be available in Claude Code sessions"
    log "  - Ask Claude to review code, analyze PRs, or suggest improvements"
    log "  - Authentication persists across Claude Code instances"
    echo
    log "Documentation: https://docs.coderabbit.ai/cli/claude-code-integration"
}

# ================================================================
# UPDATE FUNCTION
# ================================================================

update_coderabbit() {
    log "Updating CodeRabbit CLI to latest version..."

    # Get current version
    local current_version
    current_version="$(coderabbit --version 2>/dev/null || echo "unknown")"
    log "Current version: ${current_version}"

    # Set force install to skip prompts during update
    FORCE_INSTALL="true"

    # Call install function to reinstall
    install_coderabbit

    # Clean up any shell config modifications made by the installer
    cleanup_shell_config_modifications "coderabbit"

    # Verify installation and get new version
    if verify_installation; then
        local new_version
        new_version="$(coderabbit --version 2>/dev/null || echo "unknown")"
        log_success "CodeRabbit CLI updated successfully"
        log "Previous version: ${current_version}"
        log "New version: ${new_version}"
    else
        log_warning "Update completed but verification failed"
        log "Please restart your shell and try: coderabbit --version"
    fi
}

# ================================================================
# MAIN INSTALLATION FUNCTION
# ================================================================

install_coderabbit_main() {
    log "Starting CodeRabbit CLI installation..."

    # Check if already installed and not forcing
    if [[ "$FORCE_INSTALL" != "true" ]] && check_coderabbit_installed; then
        log "CodeRabbit CLI is already installed"
        log "Updating to latest version..."
        update_coderabbit
        return 0
    fi

    # Check prerequisites
    check_prerequisites

    # Install CodeRabbit
    install_coderabbit

    # Verify installation
    if verify_installation; then
        show_next_steps
    else
        log_warning "Installation completed but verification failed"
        log "Please restart your shell and try: coderabbit --version"
    fi
}

# ================================================================
# ARGUMENT PARSING
# ================================================================

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --install)
                ACTION="install"
                shift
                ;;
            --uninstall)
                ACTION="uninstall"
                shift
                ;;
            --force)
                FORCE_INSTALL="true"
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
}

# ================================================================
# MAIN FUNCTION
# ================================================================

main() {
    parse_args "$@"

    case "$ACTION" in
        "install")
            install_coderabbit_main
            ;;
        "uninstall")
            uninstall_coderabbit
            ;;
        *)
            error_exit "Invalid action: $ACTION. Use --help for usage information."
            ;;
    esac
}

# Execute main function if script is run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
