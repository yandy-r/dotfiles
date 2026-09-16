#!/usr/bin/env bash

# Install OpenCode AI Coding Agent
# Supports macOS, Linux (Ubuntu, Fedora, Arch, openSUSE, WSL), and other Unix-like systems

set -euo pipefail

# Constants
SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[OPENCODE-INSTALL]"
readonly OPENCODE_INSTALL_PATH="/usr/local/bin/opencode"

# Configuration
ACTION="${ACTION:-install}"
OPENCODE_VERSION="${OPENCODE_VERSION:-latest}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"

# Usage function
usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install, update, or uninstall OpenCode AI coding agent.

OPTIONS:
    --install           Install OpenCode (default)
    --uninstall         Uninstall OpenCode
    --update            Update OpenCode to latest version
    --version VERSION   Install specific version (default: latest)
    --force             Force installation even if already installed
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    ACTION             Action to perform (install, uninstall, update)
    OPENCODE_VERSION   Version to install (default: latest)
    FORCE_INSTALL      Force installation (true/false)

EXAMPLES:
    $SCRIPT_NAME --install
    $SCRIPT_NAME --uninstall
    $SCRIPT_NAME --update
    $SCRIPT_NAME --force

INSTALLATION METHODS (tried in order by platform):
    macOS:
        1. Official script: curl -fsSL https://opencode.ai/install | bash
        2. Homebrew: brew install sst/tap/opencode
        3. NPM: npm install -g opencode-ai

    Linux (Ubuntu, Fedora, openSUSE, WSL):
        1. Official script: curl -fsSL https://opencode.ai/install | bash
        2. NPM: npm install -g opencode-ai

    Linux (Arch):
        1. Official script: curl -fsSL https://opencode.ai/install | bash
        2. Paru: paru -S opencode-bin
        3. NPM: npm install -g opencode-ai

PREREQUISITES:
    - For Homebrew method: brew
    - For official script: curl and bash
    - For npm method: Node.js and npm
    - For Paru method: paru (Arch Linux)
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

    # Auto-approve in non-interactive mode
    if [[ "${NON_INTERACTIVE:-false}" == "true" ]] || [[ "${FORCE_INSTALL:-false}" == "true" ]] || [[ "${NONINTERACTIVE:-0}" == "1" ]]; then
        log "Non-interactive mode: auto-approving prompt"
        return 0
    fi

	if ! is_interactive; then
		log "ERROR: Non-interactive shell detected - cannot prompt for confirmation"
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

# OS Detection
detect_os() {
	if [[ "$(uname)" == "Darwin" ]]; then
		echo "macos"
	elif [[ -f /etc/os-release ]]; then
		local os_id
		os_id=$(grep -oP '^ID=\K.*' /etc/os-release | tr -d '"')
		case "$os_id" in
		ubuntu) echo "ubuntu" ;;
		fedora) echo "fedora" ;;
		arch) echo "arch" ;;
		opensuse* | sles) echo "opensuse" ;;
		*) echo "linux" ;;
		esac
	elif [[ "$(uname)" == "Linux" ]]; then
		echo "linux"
	else
		echo "unknown"
	fi
}

# Check if running in WSL
is_wsl() {
	[[ -n "${WSL_DISTRO_NAME:-}" ]] || grep -qi microsoft /proc/version 2>/dev/null
}

# Architecture detection
detect_arch() {
	case "$(uname -m)" in
	x86_64) echo "amd64" ;;
	arm64 | aarch64) echo "arm64" ;;
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

# Check if OpenCode is already installed
check_opencode_installed() {
	if command -v opencode &>/dev/null; then
		local version
		version="$(opencode --version 2>/dev/null || echo "unknown")"
		log "OpenCode is already installed: ${version}"
		return 0
	else
		return 1
	fi
}

# Uninstall OpenCode
uninstall_opencode() {
	log "Uninstalling OpenCode..."

	# Detect installation method
	local os_type
	os_type="$(detect_os)"

	# Try Homebrew uninstall on macOS
	if [[ "$os_type" == "macos" ]] && command -v brew &>/dev/null; then
		if brew list opencode &>/dev/null; then
			log "Uninstalling OpenCode via Homebrew..."
			brew uninstall opencode
			log "OpenCode uninstalled successfully via Homebrew"
			return 0
		fi
	fi

	# Try Paru uninstall on Arch
	if [[ "$os_type" == "arch" ]] && command -v paru &>/dev/null; then
		if paru -Q opencode-bin &>/dev/null; then
			log "Uninstalling OpenCode via Paru..."
			paru -R opencode-bin
			log "OpenCode uninstalled successfully via Paru"
			return 0
		fi
	fi

	# Try NPM uninstall
	if command -v npm &>/dev/null; then
		if npm list -g opencode-ai &>/dev/null; then
			log "Uninstalling OpenCode via npm..."
			npm uninstall -g opencode-ai
			log "OpenCode uninstalled successfully via npm"
			return 0
		fi
	fi

	# Fallback: manual uninstall
	if [[ ! -f "$OPENCODE_INSTALL_PATH" ]]; then
		log "OpenCode is not installed at $OPENCODE_INSTALL_PATH"
		return 0
	fi

	# Create backup before uninstall
	local backup_path
	backup_path="${HOME}/.opencode_backup_$(date +%Y%m%d_%H%M%S)"
	log "Creating backup at $backup_path"
	if ! cp "$OPENCODE_INSTALL_PATH" "$backup_path" 2>/dev/null; then
		log "Warning: Failed to create backup"
	fi

	# Remove OpenCode binary (requires elevated privileges)
	log "Elevated privileges required to remove OpenCode binary"
	log "The following command will be executed:"
	log "    sudo rm -f $OPENCODE_INSTALL_PATH"
	echo ""

	if ! prompt_confirmation "Do you want to proceed with sudo?"; then
		log "Uninstallation cancelled by user"
		log "To remove manually, run: sudo rm -f $OPENCODE_INSTALL_PATH"
		return 1
	fi

	if sudo rm -f "$OPENCODE_INSTALL_PATH" 2>/dev/null; then
		log "OpenCode uninstalled successfully"
		[[ -f "$backup_path" ]] && log "Backup available at: $backup_path"
	else
		error_exit "Failed to uninstall OpenCode"
	fi
}

# Update OpenCode
update_opencode() {
	log "Updating OpenCode to latest version..."

	if ! check_opencode_installed; then
		log "OpenCode is not installed. Use --install instead."
		return 1
	fi

	local current_version
	current_version="$(opencode --version 2>/dev/null || echo "unknown")"
	log "Current version: $current_version"

	# Detect installation method
	local os_type
	os_type="$(detect_os)"

	# Try Homebrew update on macOS
	if [[ "$os_type" == "macos" ]] && command -v brew &>/dev/null; then
		if brew list opencode &>/dev/null; then
			log "Updating OpenCode via Homebrew..."
			brew upgrade opencode
			verify_installation
			return 0
		fi
	fi

	# Try Paru update on Arch
	if [[ "$os_type" == "arch" ]] && command -v paru &>/dev/null; then
		if paru -Q opencode-bin &>/dev/null; then
			log "Updating OpenCode via Paru..."
			paru -S opencode-bin
			verify_installation
			return 0
		fi
	fi

	# Try NPM update
	if command -v npm &>/dev/null; then
		if npm list -g opencode-ai &>/dev/null; then
			log "Updating OpenCode via npm..."
			install_opencode_npm
			verify_installation
			return 0
		fi
	fi



	# Force reinstall for update
	log "Could not detect installation method, forcing reinstall..."
	FORCE_INSTALL="true"
	install_opencode_main
}

# Install OpenCode via Homebrew (macOS and Linux)
install_opencode_homebrew() {
	log "Installing OpenCode via Homebrew..."

	if ! command -v brew &>/dev/null; then
		log "Homebrew not found"
		return 1
	fi

	# NOTE: Integrity verification is handled by Homebrew's built-in mechanisms:
	#   - Homebrew verifies SHA256 checksums for downloaded formulae
	#   - Formulae are fetched from the official tap repository over HTTPS
	#   - This provides reasonable security for package installation

	# Tap the SST repository if not already tapped
	if ! brew tap | grep -q "sst/tap"; then
		log "Tapping sst/tap..."
		if ! brew tap sst/tap; then
			log "Failed to tap sst/tap"
			return 1
		fi
	fi

	# Install OpenCode
	if ! brew install sst/tap/opencode; then
		log "Failed to install OpenCode via Homebrew"
		return 1
	fi

	log "OpenCode installed successfully via Homebrew"
	return 0
}

# Install OpenCode via official script
install_opencode_official_script() {
	log "Installing OpenCode via official script..."

	if ! command -v curl &>/dev/null; then
		log "curl not found - required for official script installation"
		return 1
	fi

	# Security warning for remote script execution (skip if force install)
	if [[ "$FORCE_INSTALL" != "true" ]]; then
		log ""
		log "WARNING: SECURITY NOTICE"
		log "========================================"
		log "This will download and execute a remote script from:"
		log "    https://opencode.ai/install"
		log ""
		log "The script will be piped directly to bash for execution."
		log ""
		log "INTEGRITY VERIFICATION:"
		log "  - This installation method does NOT verify checksums or signatures"
		log "  - You are trusting the opencode.ai domain and its SSL certificate"
		log "  - The script content is not inspected before execution"
		log ""
		log "For maximum security, consider:"
		log "  1. Reviewing the script first: curl -fsSL https://opencode.ai/install"
		log "  2. Using a package manager (brew on macOS, paru on Arch)"
		log "========================================"
		log ""

		if ! prompt_confirmation "Do you want to proceed with downloading and executing the remote script?"; then
			log "Installation cancelled by user"
			return 1
		fi
	else
		log "Proceeding with official script installation (force mode)..."
	fi

	# Download and run the official install script with environment variables
	# to prevent shell config modification and control installation location
	# NOTE: No integrity verification is performed - relies on HTTPS/SSL trust
	if ! SKIP_SHELL_INTEGRATION=1 \
		NO_UPDATE_PROFILE=1 \
		curl -fsSL https://opencode.ai/install | bash; then
		log "Failed to install OpenCode via official script"
		return 1
	fi

	# Clean up any shell config modifications made by the installer
	cleanup_shell_config_modifications "opencode"

	log "OpenCode installed successfully via official script"
	return 0
}

# Install OpenCode via NPM
install_opencode_npm() {
	log "Installing OpenCode via npm..."

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

	# NOTE: Integrity verification is handled by npm's built-in mechanisms:
	#   - npm verifies package integrity using SHA512 checksums from package-lock.json
	#   - Packages are downloaded from the official npm registry over HTTPS
	#   - npm uses Subresource Integrity (SRI) for package verification
	#   - This provides strong security for package installation

	# Install OpenCode globally via npm
	# NPM will respect NPM_CONFIG_PREFIX if set, which should point to ~/.local or ~/.npm-global
	if ! npm install -g opencode-ai; then
		log "Failed to install OpenCode via npm"
		return 1
	fi

	log "OpenCode installed successfully via npm"
	return 0
}

# Install OpenCode via Paru (Arch Linux)
install_opencode_paru() {
	log "Installing OpenCode via Paru..."

	if ! command -v paru &>/dev/null; then
		log "Paru not found"
		return 1
	fi

	# NOTE: Integrity verification for AUR packages:
	#   - Paru builds packages from PKGBUILD scripts in the AUR
	#   - PKGBUILDs include checksums (sha256sums, sha512sums) for source files
	#   - Paru verifies these checksums before building/installing
	#   - Security depends on the PKGBUILD maintainer and AUR community review
	#   - Consider reviewing the PKGBUILD manually for sensitive installations

	# Install OpenCode
	if ! paru -S --noconfirm opencode-bin; then
		log "Failed to install OpenCode via Paru"
		return 1
	fi

	log "OpenCode installed successfully via Paru"
	return 0
}

# Install OpenCode on macOS
install_opencode_macos() {
	log "Installing OpenCode on macOS..."

	# Try official script first (preferred method for all platforms)
	if install_opencode_official_script; then
		return 0
	fi
	log "Official script installation failed, trying Homebrew..."

	# Try Homebrew as fallback
	if install_opencode_homebrew; then
		return 0
	fi
	log "Homebrew installation failed, trying npm..."

	# Try npm installation
	if install_opencode_npm; then
		return 0
	fi

	error_exit "All installation methods failed for macOS"
}

# Install OpenCode on Linux
install_opencode_linux() {
	local os_type="$1"
	log "Installing OpenCode on Linux (${os_type})..."

	# Try official script first (preferred method for all platforms)
	if install_opencode_official_script; then
		return 0
	fi
	log "Official script installation failed, trying alternative methods..."

	# Special handling for Arch Linux - try Paru as fallback
	if [[ "$os_type" == "arch" ]]; then
		log "Arch Linux detected, trying Paru..."
		if install_opencode_paru; then
			return 0
		fi
		log "Paru installation failed, trying npm..."
	fi

	# Try npm installation
	if install_opencode_npm; then
		return 0
	fi

	error_exit "All installation methods failed for Linux (${os_type})"
}

# Verify OpenCode installation
verify_installation() {
	log "Verifying OpenCode installation..."

	if ! command -v opencode &>/dev/null; then
		log "ERROR: opencode not found in PATH"
		log "Please restart your shell or run: source ~/.zshrc"
		return 1
	fi

	local version
	version="$(opencode --version 2>/dev/null || echo "unknown")"
	log "Installation verified: ${version}"
	log ""
	log "✓ Installation successful!"
	log "Location: $(command -v opencode)"
}

# Main installation function
install_opencode_main() {
	log "Starting OpenCode installation..."

	# Check if already installed and not forcing
	if [[ "$FORCE_INSTALL" != "true" ]] && check_opencode_installed; then
		log "OpenCode is already installed"
		log "Updating to latest version..."
		update_opencode
		return 0
	fi

	# Detect OS and architecture
	local os_type
	os_type="$(detect_os)"
	local arch
	arch="$(detect_arch)"

	log "Detected OS: ${os_type}, Architecture: ${arch}"

	# Check for WSL
	if is_wsl; then
		log "WSL detected, using Linux installation method"
	fi

	# Install based on OS
	case "${os_type}" in
	"macos")
		install_opencode_macos
		;;
	"ubuntu" | "fedora" | "opensuse" | "arch" | "linux")
		install_opencode_linux "${os_type}"
		;;
	*)
		error_exit "Unsupported operating system: ${os_type}"
		;;
	esac

	# Verify installation
	verify_installation

	log "OpenCode installation completed successfully"
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
			OPENCODE_VERSION="$2"
			shift 2
			;;
		--force)
			FORCE_INSTALL="true"
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

	case "$ACTION" in
	"install")
		install_opencode_main
		;;
	"uninstall")
		uninstall_opencode
		;;
	"update")
		update_opencode
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

