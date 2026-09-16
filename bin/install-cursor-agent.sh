#!/usr/bin/env bash
#
# Manual use only. Not wired into bootstrap, playbooks, or CI.
#
# Install Cursor Agent CLI
# Cross-platform installation script for Cursor Agent CLI tool
# Supports macOS, Linux (Ubuntu, Fedora, Arch, openSUSE), and WSL
#
# This script provides multiple installation methods:
# 1. Official Cursor installer (primary method)
# 2. NPM package installation (alternative)
# 3. Manual binary download (fallback)
#
# Features:
# - Cross-platform detection using common/platform.sh
# - Multiple installation methods with fallbacks
# - Installation verification and error handling
# - Dry-run mode for testing
# - Backup creation before uninstall/update
# - Automatic PATH management

set -euo pipefail

# Constants
SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[CURSOR-AGENT-INSTALL]"
readonly CURSOR_AGENT_INSTALL_PATH="${HOME}/.local/bin/cursor-agent"

# Source platform detection
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Determine dotfiles root - handle both direct execution and symlink execution
if [[ -f "${SCRIPT_DIR}/common/platform.sh" ]]; then
	# Script is in dotfiles/bin directory
	DOTFILES_ROOT="$(dirname "$SCRIPT_DIR")"
elif [[ -f "${HOME}/.config/dotfiles/common/platform.sh" ]]; then
	# Script is symlinked or copied to another location, use standard dotfiles path
	DOTFILES_ROOT="${HOME}/.config/dotfiles"
else
	# Fallback: try to find dotfiles directory
	DOTFILES_ROOT=""
	for possible_root in "${HOME}/.config/dotfiles" "${HOME}/dotfiles" "${HOME}/.dotfiles"; do
		if [[ -f "${possible_root}/common/platform.sh" ]]; then
			DOTFILES_ROOT="$possible_root"
			break
		fi
	done

	if [[ -z "$DOTFILES_ROOT" ]]; then
		echo "ERROR: Cannot find dotfiles directory with common/platform.sh" >&2
		echo "Please ensure your dotfiles are properly set up" >&2
		exit 1
	fi
fi

# shellcheck source=common/platform.sh
# shellcheck source=${DOTFILES_ROOT}/common/platform.sh
source "${DOTFILES_ROOT}/common/platform.sh"

# Configuration
ACTION="${ACTION:-install}"
CURSOR_AGENT_VERSION="${CURSOR_AGENT_VERSION:-latest}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
DRY_RUN="${DRY_RUN:-false}"

# Usage function
usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install, update, or uninstall Cursor Agent CLI.

OPTIONS:
    --install           Install Cursor Agent (default)
    --uninstall         Uninstall Cursor Agent
    --update            Update Cursor Agent to latest version
    --version VERSION   Install specific version (default: latest)
    --force             Force installation even if already installed
    --dry-run           Show what would be installed without actually installing
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    ACTION                Action to perform (install, uninstall, update)
    CURSOR_AGENT_VERSION  Version to install (default: latest)
    FORCE_INSTALL         Force installation (true/false)
    DRY_RUN              Enable dry-run mode (true/false)

EXAMPLES:
    $SCRIPT_NAME --install
    $SCRIPT_NAME --uninstall
    $SCRIPT_NAME --update
    $SCRIPT_NAME --force
    $SCRIPT_NAME --dry-run

INSTALLATION METHODS (tried in order):
    1. Official Cursor installer: curl https://cursor.com/install -fsS | bash
    2. NPM: npm install -g @cursor/agent (if available)
    3. Manual binary download (fallback)

PREREQUISITES:
    - For official installer: curl and bash
    - For npm method: Node.js and npm
    - For manual method: curl and compatible architecture

PLATFORMS SUPPORTED:
    - macOS (Intel and Apple Silicon)
    - Linux (Ubuntu, Fedora, Arch, openSUSE)
    - WSL (Windows Subsystem for Linux)
EOF
	exit "${1:-0}"
}

# Logging function
log() {
	echo "${LOG_PREFIX} $*" >&2
}

# Error handling
error_exit() {
	log "ERROR: $1" >&2
	exit "${2:-1}"
}

# Check if running in interactive mode
is_interactive() {
	[[ -t 0 ]] && [[ -t 1 ]]
}

# Prompt user for confirmation
prompt_confirmation() {
	local prompt_message="$1"
	local response

	if ! is_interactive; then
		log "ERROR: Non-interactive shell detected - cannot prompt for confirmation" >&2
		return 1
	fi

	echo -n "${LOG_PREFIX} ${prompt_message} [y/N]: " >&2
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

# Check if Cursor Agent is already installed
check_cursor_agent_installed() {
	if command -v cursor-agent &>/dev/null; then
		local version
		version="$(cursor-agent --version 2>/dev/null || echo "unknown")"
		log "Cursor Agent is already installed: ${version}"
		return 0
	else
		return 1
	fi
}

# Check prerequisites
check_prerequisites() {
	log "Checking prerequisites..."

	# Check for curl
	if ! command -v curl &>/dev/null; then
		error_exit "curl is required but not installed. Please install curl first."
	fi

	# Check for bash
	if ! command -v bash &>/dev/null; then
		error_exit "bash is required but not installed."
	fi

	# Check network connectivity
	if ! curl -s --connect-timeout 10 https://cursor.com >/dev/null; then
		error_exit "No internet connectivity or Cursor servers unreachable"
	fi

	log "Prerequisites check passed"
}

# Ensure .local/bin directory exists
ensure_local_bin() {
	local local_bin="${HOME}/.local/bin"
	if [[ ! -d "$local_bin" ]]; then
		log "Creating $local_bin directory..."
		mkdir -p "$local_bin"
	fi

	# Add to PATH for current session if not already there
	# NOTE: We don't modify shell configs - that's handled by dotfiles (common/paths.sh)
	if [[ ":$PATH:" != *":$local_bin:"* ]]; then
		export PATH="$local_bin:$PATH"
		log "Added $local_bin to PATH for current session"
		log "Your shell will have this in PATH automatically via dotfiles"
	fi
}

# Install Cursor Agent via official installer
install_cursor_agent_official() {
	log "Installing Cursor Agent via official installer..."

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would run: curl https://cursor.com/install -fsS | bash"
		return 0
	fi

	# Security warning for remote script execution
	log "WARNING: This will download and execute a remote script from:"
	log "    https://cursor.com/install"
	log "The script will be piped directly to your shell."
	echo

	if ! prompt_confirmation "Do you want to proceed with downloading and executing the remote script?"; then
		log "Installation cancelled by user"
		return 1
	fi

	# Run official installer with environment variables to control installation
	# and prevent shell config modification
	if ! SKIP_SHELL_INTEGRATION=1 \
		NO_UPDATE_PROFILE=1 \
		curl https://cursor.com/install -fsS | bash; then
		log "Official installer failed, trying alternative methods..."
		return 1
	fi

	# Clean up any shell config modifications made by the installer
	cleanup_shell_config_modifications "cursor-agent"
	cleanup_shell_config_modifications "cursor"

	return 0
}

# Install Cursor Agent via npm (alternative method)
install_cursor_agent_npm() {
	log "Installing Cursor Agent via npm..."

	# Check if Node.js is installed
	if ! command -v node &>/dev/null; then
		log "Node.js is required for npm installation"
		return 1
	fi

	# Check if npm is installed
	if ! command -v npm &>/dev/null; then
		log "npm is required for npm installation"
		return 1
	fi

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would run: npm install -g @cursor/agent"
		return 0
	fi

	# Try installing via npm
	if npm install -g @cursor/agent 2>/dev/null; then
		log "Cursor Agent installed successfully via npm"
		return 0
	else
		log "npm installation failed - package may not exist"
		return 1
	fi
}

# Install Cursor Agent manually (fallback method)
install_cursor_agent_manual() {
	log "Installing Cursor Agent manually (fallback method)..."
	log "Warning: This is a fallback method and may not work reliably"

	local os_type="$DOTFILES_OS"
	local arch="$DOTFILES_ARCH"

	# Map our platform detection to Cursor's expected formats
	case "$os_type" in
	"macos")
		case "$arch" in
		"arm64") download_arch="arm64" ;;
		"x86_64") download_arch="amd64" ;;
		*) error_exit "Unsupported architecture for macOS: $arch" ;;
		esac
		download_os="darwin"
		;;
	"debian"|"redhat"|"arch"|"linux")
		case "$arch" in
		"arm64") download_arch="arm64" ;;
		"x86_64") download_arch="amd64" ;;
		*) error_exit "Unsupported architecture for Linux: $arch" ;;
		esac
		download_os="linux"
		;;
	"wsl")
		# WSL is essentially Linux
		case "$arch" in
		"arm64") download_arch="arm64" ;;
		"x86_64") download_arch="amd64" ;;
		*) error_exit "Unsupported architecture for WSL: $arch" ;;
		esac
		download_os="linux"
		;;
	*)
		error_exit "Unsupported operating system: $os_type"
		;;
	esac

	log "Downloading for $download_os-$download_arch..."

	# Try to get latest version from GitHub API
	local latest_version
	local api_response

	# Fetch API response
	api_response=$(curl -s --connect-timeout 10 https://api.github.com/repos/getcursor/cursor/releases/latest 2>/dev/null)

	# Use jq if available for safer JSON parsing
	if command -v jq &>/dev/null; then
		latest_version=$(echo "$api_response" | jq -r '.tag_name // empty' 2>/dev/null)
	else
		# Fallback to grep/cut with validation
		log "Warning: jq not found, using basic parsing (less secure)"
		latest_version=$(echo "$api_response" | grep '"tag_name"' | head -n 1 | cut -d'"' -f4)
	fi

	# Validate version format (should match semver or version pattern)
	if [[ -z "$latest_version" ]]; then
		error_exit "Unable to determine latest Cursor Agent version from GitHub API"
	fi

	# Validate that the version looks like a valid tag (v1.2.3 or 1.2.3)
	if [[ ! "$latest_version" =~ ^v?[0-9]+\.[0-9]+(\.[0-9]+)?(-[a-zA-Z0-9._-]+)?$ ]]; then
		error_exit "Invalid version format received: ${latest_version}"
	fi

	log "Latest Cursor Agent version: ${latest_version}"

	# Construct download URL
	local download_url="https://github.com/getcursor/cursor/releases/download/${latest_version}/cursor-agent-${download_os}-${download_arch}"

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would download from: $download_url"
		return 0
	fi

	# Download and install using the installer's default location
	log "Downloading from: ${download_url}"
	if ! curl -fsSL "${download_url}" -o /tmp/cursor-agent; then
		error_exit "Failed to download Cursor Agent binary"
	fi

	chmod +x /tmp/cursor-agent
	
	# Try to download checksum file for verification
	local checksum_url="${download_url}.sha256"
	log "Attempting to download checksum file for verification..."
	if curl -fsSL "${checksum_url}" -o /tmp/cursor-agent.sha256 2>/dev/null; then
		log "Verifying checksum..."
		if command -v sha256sum &>/dev/null; then
			if ! (cd /tmp && sha256sum -c cursor-agent.sha256 2>/dev/null); then
				rm -f /tmp/cursor-agent /tmp/cursor-agent.sha256
				error_exit "Checksum verification failed"
			fi
		elif command -v shasum &>/dev/null; then
			if ! (cd /tmp && shasum -a 256 -c cursor-agent.sha256 2>/dev/null); then
				rm -f /tmp/cursor-agent /tmp/cursor-agent.sha256
				error_exit "Checksum verification failed"
			fi
		fi
		log "Checksum verification successful"
		rm -f /tmp/cursor-agent.sha256
	else
		log "Warning: Checksum file not available"
	fi

	log "Cursor Agent installed successfully via manual method"
}

# Uninstall Cursor Agent
uninstall_cursor_agent() {
	log "Uninstalling Cursor Agent..."

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would remove $CURSOR_AGENT_INSTALL_PATH"
		return 0
	fi

	# Create backup before uninstall
	if [[ -f "$CURSOR_AGENT_INSTALL_PATH" ]]; then
		local backup_path
		backup_path="${HOME}/.cursor_agent_backup_$(date +%Y%m%d_%H%M%S)"
		log "Creating backup at $backup_path"
		cp "$CURSOR_AGENT_INSTALL_PATH" "$backup_path"
		log "Backup available at: $backup_path"
	fi

	# Remove Cursor Agent binary
	if [[ -f "$CURSOR_AGENT_INSTALL_PATH" ]]; then
		rm -f "$CURSOR_AGENT_INSTALL_PATH"
		log "Cursor Agent uninstalled successfully"
	else
		log "Cursor Agent is not installed at $CURSOR_AGENT_INSTALL_PATH"
	fi

	# Check for other possible locations
	local other_locations=(
		"/usr/local/bin/cursor-agent"
		"/opt/homebrew/bin/cursor-agent"
		"${HOME}/.local/share/cursor-agent"
	)

	for location in "${other_locations[@]}"; do
		if [[ -f "$location" ]]; then
			log "Found Cursor Agent at $location - removing..."
			rm -f "$location"
		fi
	done
}

# Update Cursor Agent
update_cursor_agent() {
	log "Updating Cursor Agent to latest version..."

	if ! check_cursor_agent_installed; then
		log "Cursor Agent is not installed. Use --install instead."
		return 1
	fi

	local current_version
	current_version="$(cursor-agent --version 2>/dev/null | grep -o 'v[0-9.]*' || echo "unknown")"
	log "Current version: $current_version"

	# Force reinstall for update
	FORCE_INSTALL="true"
	install_cursor_agent_main
}

# Verify Cursor Agent installation
verify_installation() {
	log "Verifying Cursor Agent installation..."

	if ! command -v cursor-agent &>/dev/null; then
		log "ERROR: cursor-agent not found in PATH"
		log "Please restart your shell or run: source ~/.zshrc"
		return 1
	fi

	local version
	version="$(cursor-agent --version 2>/dev/null || echo "unknown")"
	log "Installation verified: ${version}"
	log ""
	log "✓ Installation successful!"
	log "Location: $(command -v cursor-agent)"
}

# Main installation function
install_cursor_agent_main() {
	log "Starting Cursor Agent installation..."

	# Check if already installed and not forcing
	if [[ "$FORCE_INSTALL" != "true" ]] && check_cursor_agent_installed; then
		log "Cursor Agent is already installed"
		log "Updating to latest version..."
		update_cursor_agent
		return 0
	fi

	# Check prerequisites
	check_prerequisites

	# Ensure .local/bin directory exists and is in PATH
	ensure_local_bin

	# Log platform information
	log "Platform: $DOTFILES_OS ($DOTFILES_ARCH)"
	log "Package Manager: $DOTFILES_PKG_MGR"

	# Try installation methods in order of preference
	if install_cursor_agent_official; then
		log "Installed via official installer"
	elif install_cursor_agent_npm; then
		log "Installed via npm"
	else
		log "Official methods failed, trying manual installation..."
		install_cursor_agent_manual
	fi

	# Verify installation
	verify_installation

	log "Cursor Agent installation completed successfully"
}

# Parse command line arguments
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
		--update)
			ACTION="update"
			shift
			;;
		--version)
			CURSOR_AGENT_VERSION="$2"
			shift 2
			;;
		--force)
			FORCE_INSTALL="true"
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
}

# Main function
main() {
	parse_args "$@"

	# Set up platform environment
	setup_platform_env

	log "Starting Cursor Agent management..."
	[[ "$DRY_RUN" == "true" ]] && log "DRY-RUN MODE: No changes will be made"

	case "$ACTION" in
	"install")
		install_cursor_agent_main
		;;
	"uninstall")
		uninstall_cursor_agent
		;;
	"update")
		update_cursor_agent
		;;
	*)
		error_exit "Invalid action: $ACTION. Use --help for usage information."
		;;
	esac

	log "Cursor Agent management completed successfully"
}

# Execute main function if script is run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
