#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[TAILSCALE-INSTALL]"

# Configuration
DRY_RUN="${DRY_RUN:-false}"
SKIP_POST_INSTALL="${SKIP_POST_INSTALL:-false}"

# Dotfiles directory and platform detection
DOTFILES_DIR="${DOTFILES_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
readonly PLATFORM_SCRIPT="${DOTFILES_DIR}/common/platform.sh"

# Color codes for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly NC='\033[0m' # No Color

# ================================================================
# USAGE AND HELP
# ================================================================

usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install Tailscale VPN with multi-OS support.

OPTIONS:
    --skip-post         Skip post-installation setup (start service, etc.)
    --dry-run           Show what would be installed without actually installing
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    DRY_RUN             Enable dry-run mode (true/false)
    SKIP_POST_INSTALL   Skip post-installation setup (true/false)
    NON_INTERACTIVE     Auto-approve prompts (true/false)
    FORCE_INSTALL       Force installation without prompts (true/false)

EXAMPLES:
    $SCRIPT_NAME
    $SCRIPT_NAME --dry-run
    $SCRIPT_NAME --skip-post
    DRY_RUN=true $SCRIPT_NAME
    FORCE_INSTALL=true $SCRIPT_NAME

SUPPORTED PLATFORMS:
    - Ubuntu/Debian (via official installer script)
    - Fedora/RHEL (via official installer script)
    - Arch Linux (via official installer script)
    - openSUSE (via official installer script)
    - macOS (via Homebrew: brew install tailscale + brew install --cask tailscale)
    - WSL (via official installer script)
    - Windows (manual installation required)

NOTES:
    - Linux: Uses official Tailscale installer: curl -fsSL https://tailscale.com/install.sh | sh
    - macOS: Installs CLI via Homebrew (brew install tailscale) and GUI app (brew install --cask tailscale)
    - Windows: Not supported - manual installation required
    - Post-installation can start/enable the Tailscale service
    - Requires sudo/root privileges for system-level installation (Linux)
    - After installation, run 'sudo tailscale up' to authenticate (Linux) or launch the app (macOS)

SECURITY:
    The Linux installer script is fetched from https://tailscale.com/install.sh
    and piped directly to sh. This is the official Tailscale installation method.
    Review the script first: curl -fsSL https://tailscale.com/install.sh | less

EOF
	exit "${1:-0}"
}

# ================================================================
# LOGGING FUNCTIONS
# ================================================================

log() {
	echo -e "${CYAN}${LOG_PREFIX}${NC} [$(date '+%Y-%m-%d %H:%M:%S')] $*" >&2
}

log_success() {
	echo -e "${GREEN}${LOG_PREFIX}${NC} [$(date '+%Y-%m-%d %H:%M:%S')] ✓ $*" >&2
}

log_warning() {
	echo -e "${YELLOW}${LOG_PREFIX}${NC} [$(date '+%Y-%m-%d %H:%M:%S')] ⚠ $*" >&2
}

log_error() {
	echo -e "${RED}${LOG_PREFIX}${NC} [$(date '+%Y-%m-%d %H:%M:%S')] ✗ $*" >&2
}

log_section() {
	echo ""
	echo "# ================================================================"
	echo -e "${BLUE}${LOG_PREFIX} $*${NC}" >&2
	echo "# ================================================================"
}

error_exit() {
	log_error "$1"
	exit "${2:-1}"
}

# ================================================================
# UTILITY FUNCTIONS
# ================================================================

check_command() {
	if ! command -v "$1" &>/dev/null; then
		return 1
	fi
	return 0
}

is_interactive() {
	[[ -t 0 ]] && [[ -t 1 ]]
}

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
		log_error "Set NON_INTERACTIVE=true or FORCE_INSTALL=true to proceed automatically"
		return 1
	fi

	echo -n "${LOG_PREFIX} ${prompt_message} [y/N]: " >&2
	read -r response

	# Default to "no" on empty input
	response="${response:-n}"

	case "$response" in
	[yY] | [yY][eE][sS])
		return 0
		;;
	*)
		return 1
		;;
	esac
}

# ================================================================
# PLATFORM DETECTION
# ================================================================

load_platform_detection() {
	# Check if platform variables are already set (e.g., by bootstrap.sh)
	if [[ -n "${DOTFILES_OS:-}" ]] && [[ -n "${DOTFILES_ARCH:-}" ]]; then
		log "Using existing platform detection: OS=$DOTFILES_OS, ARCH=$DOTFILES_ARCH"
		return 0
	fi

	# Otherwise, load from platform script
	if [[ ! -f "$PLATFORM_SCRIPT" ]]; then
		error_exit "Platform detection script not found at: $PLATFORM_SCRIPT"
	fi

	# shellcheck source=/dev/null
	source "$PLATFORM_SCRIPT"

	if [[ -z "${DOTFILES_OS:-}" ]] || [[ -z "${DOTFILES_ARCH:-}" ]]; then
		error_exit "Platform detection failed - DOTFILES_OS or DOTFILES_ARCH not set"
	fi

	log "Detected platform: OS=$DOTFILES_OS, ARCH=$DOTFILES_ARCH"
}

# ================================================================
# VALIDATION
# ================================================================

validate_environment() {
	log "Validating environment..."

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN MODE: Skipping environment validation"
		return 0
	fi

	# Check for required commands based on platform
	case "$DOTFILES_OS" in
	macos)
		if ! check_command brew; then
			error_exit "Homebrew is required for macOS installation. Install from: https://brew.sh"
		fi
		;;
	*)
		if ! check_command curl; then
			error_exit "curl is required for installation"
		fi
		;;
	esac

	# Check network connectivity
	if ! curl -s --connect-timeout 5 https://tailscale.com >/dev/null; then
		error_exit "No internet connectivity or Tailscale servers unreachable"
	fi

	log_success "Environment validation passed"
}

# ================================================================
# INSTALLATION CHECK
# ================================================================

check_existing_tailscale() {
	# For macOS, we need to check both CLI and app separately
	# For other platforms, just check the command
	if [[ "$DOTFILES_OS" == "macos" ]]; then
		# Don't exit early on macOS - let install function handle both components
		if check_command tailscale; then
			local tailscale_version
			tailscale_version="$(timeout 5 tailscale version 2>/dev/null || echo "unknown")"
			log "Tailscale CLI detected: $tailscale_version"
		fi
		
		if brew list --cask tailscale &>/dev/null 2>&1; then
			log "Tailscale app detected"
		fi
		
		# Always continue to installation function on macOS to check both components
		return 0
	fi
	
	# For non-macOS platforms, check if tailscale command exists
	if check_command tailscale; then
		local tailscale_version
		tailscale_version="$(timeout 5 tailscale version 2>/dev/null || echo "unknown")"
		log "Tailscale is already installed: $tailscale_version"

		if [[ "${FORCE_INSTALL:-false}" != "true" ]]; then
			log "FORCE_INSTALL not set, prompting for confirmation..."
			if ! prompt_confirmation "Tailscale is already installed. Do you want to reinstall/upgrade?"; then
				log "Installation cancelled - Tailscale is already installed"
				log "To force reinstallation, run with FORCE_INSTALL=true or NON_INTERACTIVE=true"
				exit 0
			fi
		else
			log "FORCE_INSTALL=true, proceeding with installation"
		fi

		return 0
	fi
	return 1
}

# ================================================================
# INSTALLATION FUNCTIONS
# ================================================================

install_tailscale_linux() {
	log_section "Installing Tailscale on Linux"

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would install Tailscale via official installer script"
		log "DRY-RUN: Command: curl -fsSL https://tailscale.com/install.sh | sh"
		return 0
	fi

	log ""
	log_warning "SECURITY WARNING"
	log "This will download and execute the official Tailscale installation script from:"
	log "    https://tailscale.com/install.sh"
	log "The script will be piped directly to sh and may request sudo access."
	log ""
	log "To review the script first, run:"
	log "    curl -fsSL https://tailscale.com/install.sh | less"
	log ""

	if ! prompt_confirmation "Do you want to proceed with downloading and executing the Tailscale installation script?"; then
		log "Installation cancelled by user"
		exit 0
	fi

	log "Downloading and executing Tailscale installer..."
	if ! curl -fsSL https://tailscale.com/install.sh | sh; then
		error_exit "Failed to install Tailscale via official installer"
	fi

	log_success "Tailscale installed successfully on Linux"
}

install_tailscale_macos() {
	log_section "Installing Tailscale on macOS"

	if ! check_command brew; then
		error_exit "Homebrew is required for macOS installation. Install from: https://brew.sh"
	fi

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would install Tailscale via Homebrew"
		log "DRY-RUN: Command: brew install tailscale"
		log "DRY-RUN: Command: brew install --cask tailscale"
		return 0
	fi

	local cli_installed=false
	local app_installed=false

	# Install Tailscale CLI
	log "Checking Tailscale CLI installation..."
	if brew list tailscale &>/dev/null 2>&1; then
		log "Tailscale CLI is already installed"
		log "Checking for updates..."
		if brew upgrade tailscale 2>/dev/null; then
			log_success "Tailscale CLI updated successfully"
		else
			log "Tailscale CLI is already up to date"
		fi
		cli_installed=true
	else
		log "Installing Tailscale CLI..."
		if brew install tailscale; then
			log_success "Tailscale CLI installed successfully"
			cli_installed=true
		else
			log_warning "Failed to install Tailscale CLI, continuing with GUI app installation"
		fi
	fi

	# Install Tailscale GUI app (always check, independent of CLI)
	log "Checking Tailscale GUI app installation..."
	if brew list --cask tailscale &>/dev/null 2>&1; then
		log "Tailscale app is already installed"
		log "Checking for updates..."
		if brew upgrade --cask tailscale 2>/dev/null; then
			log_success "Tailscale app updated successfully"
		else
			log "Tailscale app is already up to date"
		fi
		app_installed=true
	else
		log "Installing Tailscale GUI app..."
		if brew install --cask tailscale; then
			log_success "Tailscale app installed successfully"
			app_installed=true
		else
			log_error "Failed to install Tailscale app via Homebrew"
			if [[ "$cli_installed" == "false" ]]; then
				error_exit "Failed to install both Tailscale CLI and app"
			else
				log_warning "Tailscale CLI is installed, but GUI app installation failed"
			fi
		fi
	fi

	# Summary
	if [[ "$cli_installed" == "true" ]] && [[ "$app_installed" == "true" ]]; then
		log_success "Tailscale CLI and app installed successfully on macOS via Homebrew"
	elif [[ "$cli_installed" == "true" ]]; then
		log_warning "Tailscale CLI installed, but GUI app not available"
	elif [[ "$app_installed" == "true" ]]; then
		log_warning "Tailscale GUI app installed, but CLI not available"
	else
		error_exit "Failed to install Tailscale components"
	fi
}

install_tailscale_windows() {
	log_section "Windows Installation Not Supported"

	log ""
	log_warning "Tailscale installation on Windows requires manual installation"
	log ""
	log "To install Tailscale on Windows:"
	log "  1. Visit: https://tailscale.com/download/windows"
	log "  2. Download the Windows installer"
	log "  3. Run the installer and follow the prompts"
	log "  4. Launch Tailscale from the Start menu"
	log ""
	log "Alternative installation methods:"
	log "  - Windows Package Manager: winget install tailscale.tailscale"
	log "  - Chocolatey: choco install tailscale"
	log ""

	error_exit "Windows installation not supported by this script - please install manually"
}

# ================================================================
# POST-INSTALLATION SETUP
# ================================================================

post_install_setup() {
	if [[ "$SKIP_POST_INSTALL" == "true" ]]; then
		log "Skipping post-installation setup"
		return 0
	fi

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would perform post-installation setup"
		return 0
	fi

	log_section "Post-Installation Setup"

	case "$DOTFILES_OS" in
	debian | redhat | arch | suse | linux | wsl)
		# Enable and start Tailscale service
		if check_command systemctl; then
			log "Enabling Tailscale service..."
			if ! sudo systemctl enable tailscaled 2>/dev/null; then
				log_warning "Failed to enable tailscaled service (may not be using systemd)"
			fi

			log "Starting Tailscale service..."
			if ! sudo systemctl start tailscaled 2>/dev/null; then
				log_warning "Failed to start tailscaled service"
			else
				log_success "Tailscale service started"
			fi

			# Check service status
			if sudo systemctl is-active --quiet tailscaled 2>/dev/null; then
				log_success "Tailscale service is running"
			else
				log_warning "Tailscale service is not running"
			fi
		else
			log_warning "systemctl not available - skipping service management"
			log "You may need to start Tailscale manually"
		fi
		;;
	macos)
		log "macOS detected - Tailscale app must be launched manually"
		log "To start Tailscale:"
		log "  1. Open Tailscale from Applications or Spotlight"
		log "  2. Sign in to your Tailscale account"
		log "  3. Click 'Connect' to join your network"
		;;
	*)
		log_warning "Unknown OS - skipping post-installation setup"
		;;
	esac

	log_success "Post-installation setup completed"
}

# ================================================================
# VERIFICATION
# ================================================================

verify_installation() {
	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Skipping installation verification"
		return 0
	fi

	log_section "Verifying Installation"

	# Check if tailscale command is available
	if ! check_command tailscale; then
		log_error "Tailscale command not found after installation"
		log_warning "You may need to restart your shell or log out and back in"
		return 1
	fi

	# Get version information
	local tailscale_version
	tailscale_version="$(tailscale version 2>/dev/null || echo "unknown")"
	log_success "Installed: $tailscale_version"

	# Check service status on Linux
	case "$DOTFILES_OS" in
	debian | redhat | arch | suse | linux | wsl)
		if check_command systemctl; then
			if sudo systemctl is-active --quiet tailscaled 2>/dev/null; then
				log_success "Tailscale service is running"
			else
				log_warning "Tailscale service is not running"
				log "Start it with: sudo systemctl start tailscaled"
			fi
		fi
		;;
	esac

	log_success "Installation verification completed"
}

# ================================================================
# NEXT STEPS
# ================================================================

show_next_steps() {
	log_section "Installation Complete!"

	echo ""
	log "Next steps:"
	echo ""

	case "$DOTFILES_OS" in
	debian | redhat | arch | suse | linux | wsl)
		log "1. Authenticate and connect to Tailscale:"
		log "   sudo tailscale up"
		log ""
		log "2. Check your Tailscale status:"
		log "   tailscale status"
		log ""
		log "3. Get your Tailscale IP address:"
		log "   tailscale ip"
		log ""
		log "4. View Tailscale service status:"
		log "   sudo systemctl status tailscaled"
		log ""
		log "5. Configure exit node (optional):"
		log "   sudo tailscale up --advertise-exit-node"
		;;
	macos)
		log "1. Launch Tailscale from Applications or Spotlight"
		log "2. Sign in to your Tailscale account"
		log "3. Click 'Connect' to join your Tailscale network"
		log ""
		log "4. Check your Tailscale status:"
		log "   tailscale status"
		log ""
		log "5. Get your Tailscale IP address:"
		log "   tailscale ip"
		;;
	*)
		log "1. Verify Tailscale installation: tailscale version"
		log "2. Connect to Tailscale: sudo tailscale up"
		log "3. Check status: tailscale status"
		;;
	esac

	echo ""
	log "Documentation:"
	log "  - Tailscale docs: https://tailscale.com/kb/"
	log "  - Getting started: https://tailscale.com/kb/1017/install/"
	log "  - Command reference: https://tailscale.com/kb/1080/cli/"
	log "  - Exit nodes: https://tailscale.com/kb/1103/exit-nodes/"
	echo ""
}

# ================================================================
# COMMAND LINE ARGUMENT PARSING
# ================================================================

while [[ $# -gt 0 ]]; do
	case "$1" in
	--skip-post)
		SKIP_POST_INSTALL="true"
		shift
		;;
	--dry-run)
		DRY_RUN="true"
		shift
		;;
	-h | --help)
		usage 0
		;;
	*)
		error_exit "Unknown option: $1. Use --help for usage information."
		;;
	esac
done

# ================================================================
# MAIN EXECUTION
# ================================================================

main() {
	log_section "Starting Tailscale Installation"
	[[ "$DRY_RUN" == "true" ]] && log_warning "DRY-RUN MODE: No changes will be made"

	# Load platform detection
	load_platform_detection

	# Validate environment
	validate_environment

	# Check if already installed
	check_existing_tailscale || true

	# Install based on platform
	case "$DOTFILES_OS" in
	debian | redhat | arch | suse | linux | wsl)
		install_tailscale_linux
		;;
	macos)
		install_tailscale_macos
		;;
	windows)
		install_tailscale_windows
		;;
	*)
		error_exit "Unsupported operating system: $DOTFILES_OS"
		;;
	esac

	# Post-installation setup
	post_install_setup

	# Verify installation
	verify_installation

	# Show next steps
	show_next_steps

	log_section "Tailscale Installation Completed Successfully"
}

main "$@"
