#!/usr/bin/env bash

# Install Claude Code CLI
# Supports macOS, Linux (Ubuntu), and other Unix-like systems

set -euo pipefail

# Constants
SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[CLAUDE-INSTALL]"
# System-wide binary written by the manual (curl) fallback. Overridable so the
# test suite can point it at a throwaway path.
CLAUDE_INSTALL_PATH="${CLAUDE_INSTALL_PATH:-/usr/local/bin/claude}"
readonly CLAUDE_INSTALL_PATH
# Native install layout of the official installer (https://claude.ai/install.sh):
# a launcher symlink in ~/.local/bin pointing into versioned data under
# ~/.local/share/claude. Configuration (~/.claude, ~/.claude.json) lives elsewhere
# and is never touched by --uninstall.
readonly CLAUDE_NATIVE_BIN="${HOME}/.local/bin/claude"
readonly CLAUDE_NATIVE_DATA="${HOME}/.local/share/claude"
readonly NPM_PACKAGE="@anthropic-ai/claude-code"

# Configuration
ACTION="${ACTION:-install}"
CLAUDE_VERSION="${CLAUDE_VERSION:-latest}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"

# Usage function
usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install, update, or uninstall Claude Code CLI.

OPTIONS:
    --install           Install Claude Code (default)
    --uninstall         Uninstall Claude Code
    --update            Update Claude Code (to VERSION when --version is given)
    --version VERSION   Install VERSION: latest (default), stable, or an exact
                        release such as 1.0.58 (install/update only)
    --force             Force installation even if already installed
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    ACTION             Action to perform (install, uninstall, update)
    CLAUDE_VERSION     Version to install: latest, stable, or X.Y.Z (default: latest)
    FORCE_INSTALL      Force installation (true/false)

EXAMPLES:
    $SCRIPT_NAME --install
    $SCRIPT_NAME --uninstall
    $SCRIPT_NAME --update
    $SCRIPT_NAME --install --version 1.0.58
    $SCRIPT_NAME --force

UNINSTALL:
    Removes every install this script can create: the npm global package, the
    native install (~/.local/bin/claude and ~/.local/share/claude), and the
    manual binary at /usr/local/bin/claude (via sudo). ~/.claude and
    ~/.claude.json are preserved. Exits non-zero if claude is still on PATH
    afterwards.

INSTALLATION METHODS (tried in order):
    1. Official script: curl -fsSL https://claude.ai/install.sh | bash
    2. NPM: npm install -g @anthropic-ai/claude-code (requires Node.js 18+)
    3. Manual binary download (fallback)

PREREQUISITES:
    - For npm method: Node.js 18+ and npm
    - For official script: curl and bash
    - For manual method: curl, sudo access, and compatible architecture
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
	elif [[ -f /etc/os-release ]] && grep -q -i '^NAME="Ubuntu"' /etc/os-release; then
		echo "ubuntu"
	elif [[ "$(uname)" == "Linux" ]]; then
		echo "linux"
	else
		echo "unknown"
	fi
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

# Check if Claude is already installed
check_claude_installed() {
	if command -v claude &>/dev/null; then
		local version
		version="$(claude --version 2>/dev/null || echo "unknown")"
		log "Claude Code is already installed: ${version}"
		return 0
	else
		return 1
	fi
}

# True when the npm global Claude Code package is installed.
npm_claude_installed() {
	command -v npm &>/dev/null || return 1
	npm ls -g --depth=0 "$NPM_PACKAGE" &>/dev/null
}

# True when symlink $1 points into directory $2 (the native install's data).
link_points_under() {
	local target
	target="$(readlink "$1")" || return 1
	[[ "$target" == /* ]] || target="$(dirname "$1")/$target"
	[[ "$target" == "$2"/* ]]
}

# Remove the manual-fallback binary at CLAUDE_INSTALL_PATH (needs sudo).
uninstall_system_binary() {
	local backup_path
	backup_path="${HOME}/.claude_backup_$(date +%Y%m%d_%H%M%S)"
	log "Creating backup at $backup_path"
	if ! cp "$CLAUDE_INSTALL_PATH" "$backup_path"; then
		log "Warning: Failed to create backup"
	fi

	log "The following command will be executed:"
	log "    sudo rm -f $CLAUDE_INSTALL_PATH"
	if ! prompt_confirmation "Do you want to proceed with sudo to remove $CLAUDE_INSTALL_PATH?"; then
		log "Removal of $CLAUDE_INSTALL_PATH cancelled by user"
		return 1
	fi
	if ! sudo rm -f "$CLAUDE_INSTALL_PATH"; then
		log "ERROR: Failed to remove $CLAUDE_INSTALL_PATH"
		return 1
	fi
	log "Removed $CLAUDE_INSTALL_PATH"
	[[ -f "$backup_path" ]] && log "Backup available at: $backup_path"
	return 0
}

# Uninstall Claude from every location this script's install methods use:
# npm global, the native install (official script), and the manual binary.
# Reports "not installed" only when none of them exists, and fails when claude
# is still on PATH afterwards, so callers (dotf install --remove) never report
# a removal that did not happen.
uninstall_claude() {
	local found=false failed=false remaining

	log "Uninstalling Claude Code..."

	if npm_claude_installed; then
		found=true
		log "Removing npm global package $NPM_PACKAGE..."
		if ! npm uninstall -g "$NPM_PACKAGE"; then
			log "ERROR: npm uninstall -g $NPM_PACKAGE failed"
			failed=true
		fi
	fi

	if [[ -L "$CLAUDE_NATIVE_BIN" || -e "$CLAUDE_NATIVE_BIN" ]]; then
		found=true
		if [[ -L "$CLAUDE_NATIVE_BIN" ]] && link_points_under "$CLAUDE_NATIVE_BIN" "$CLAUDE_NATIVE_DATA"; then
			log "Removing native launcher $CLAUDE_NATIVE_BIN"
			rm -f -- "$CLAUDE_NATIVE_BIN"
		else
			log "ERROR: Refusing to remove $CLAUDE_NATIVE_BIN; it is not a symlink into $CLAUDE_NATIVE_DATA (not created by the official installer)"
			failed=true
		fi
	fi

	if [[ -d "$CLAUDE_NATIVE_DATA" ]]; then
		found=true
		log "Removing native install data $CLAUDE_NATIVE_DATA"
		rm -rf -- "$CLAUDE_NATIVE_DATA"
	fi

	if [[ -e "$CLAUDE_INSTALL_PATH" || -L "$CLAUDE_INSTALL_PATH" ]]; then
		found=true
		uninstall_system_binary || failed=true
	fi

	hash -r
	remaining="$(command -v claude 2>/dev/null || true)"

	if [[ "$found" == false ]]; then
		if [[ -n "$remaining" ]]; then
			error_exit "claude is on PATH at $remaining, but not from an install this script manages (npm, $CLAUDE_NATIVE_BIN, $CLAUDE_INSTALL_PATH); remove it with the tool that installed it"
		fi
		log "Claude Code is not installed (checked npm global, $CLAUDE_NATIVE_BIN, $CLAUDE_NATIVE_DATA, $CLAUDE_INSTALL_PATH)"
		return 0
	fi

	if [[ -n "$remaining" ]]; then
		error_exit "claude is still on PATH at $remaining after uninstall"
	fi
	if [[ "$failed" == true ]]; then
		error_exit "Claude Code was only partially uninstalled; see the errors above"
	fi
	log "Claude Code uninstalled; configuration in ~/.claude and ~/.claude.json preserved"
}

# Update Claude
update_claude() {
	log "Updating Claude Code to version: ${CLAUDE_VERSION}..."

	if ! check_claude_installed; then
		log "Claude Code is not installed. Use --install instead."
		return 1
	fi

	local current_version
	current_version="$(claude --version 2>/dev/null | grep -o 'v[0-9.]*' || echo "unknown")"
	log "Current version: $current_version"

	# Force reinstall for update
	FORCE_INSTALL="true"
	install_claude_main
}

# Install Claude on macOS
install_claude_macos() {
	log "Installing Claude Code on macOS..."

	# Try official script first (preferred method for all platforms)
	if install_claude_official_script; then
		return 0
	fi
	log "Official script installation failed, trying npm..."

	# Try npm installation as fallback
	if command -v npm &>/dev/null; then
		log "Attempting installation via npm..."
		if install_claude_npm; then
			return 0
		fi
		log "NPM installation failed, trying manual method..."
	fi

	log "All preferred methods failed, falling back to curl method..."
	install_claude_curl "macos"
}

# Install Claude on Linux
install_claude_linux() {
	local os_type="$1"
	log "Installing Claude Code on Linux (${os_type})..."

	# Try official script first (preferred method for all platforms)
	log "Attempting installation via official script..."
	if install_claude_official_script; then
		return 0
	fi
	log "Official script installation failed, trying npm..."

	# Try npm installation as fallback
	if command -v npm &>/dev/null; then
		log "Attempting installation via npm..."
		if install_claude_npm; then
			return 0
		fi
		log "NPM installation failed, trying manual method..."
	fi

	log "All preferred methods failed, falling back to curl method..."
	# Fallback to manual curl installation
	install_claude_curl "linux"
}

# Install Claude via NPM (Alternative Method)
install_claude_npm() {
	log "Installing Claude Code via npm..."

	# Check if Node.js is installed
	if ! command -v node &>/dev/null; then
		log "Node.js is required for npm installation"
		return 1
	fi

	# Check Node.js version (requires 18+)
	local node_version
	node_version=$(node --version | sed 's/v//' | cut -d. -f1)
	if [[ "$node_version" -lt 18 ]]; then
		log "Node.js 18+ is required. Current version: $(node --version)"
		return 1
	fi

	# Install Claude Code globally via npm (without sudo); "latest" is npm's
	# default dist-tag, anything else is passed through as a version/tag.
	local spec="$NPM_PACKAGE"
	[[ "$CLAUDE_VERSION" == "latest" ]] || spec="${NPM_PACKAGE}@${CLAUDE_VERSION}"
	if ! npm install -g "$spec"; then
		log "Failed to install Claude via npm"
		return 1
	fi

	return 0
}

# Install Claude via official script (Preferred Method)
install_claude_official_script() {
	log "Installing Claude Code via official script..."

	# Skip security prompt if FORCE_INSTALL is set (e.g., during updates or bootstrap)
	if [[ "$FORCE_INSTALL" != "true" ]]; then
		log "This will download and execute a remote installation script"
		echo
		log "WARNING: Security Notice"
		log "This will download and execute a remote script from:"
		log "    https://claude.ai/install.sh"
		log "The script will be piped directly to your shell."
		echo

		if ! prompt_confirmation "Do you want to proceed with downloading and executing the remote script?"; then
			log "Installation cancelled by user"
			return 1
		fi
	else
		log "Proceeding with official script installation (force mode)..."
	fi

	# Download and run the official install script with environment variables
	# to prevent shell config modification. The script takes the target
	# (latest, stable, or X.Y.Z) as its first argument: `bash -s <target>`.
	if ! curl -fsSL https://claude.ai/install.sh |
		SKIP_SHELL_INTEGRATION=1 NO_UPDATE_PROFILE=1 bash -s "$CLAUDE_VERSION"; then
		log "Failed to install Claude via official script"
		return 1
	fi

	# Clean up any shell config modifications made by the installer
	cleanup_shell_config_modifications "claude"

	return 0
}

# Install Claude via curl (fallback method)
install_claude_curl() {
	local os_type="$1"
	local arch
	arch="$(detect_arch)"

	log "Installing Claude Code via curl for ${os_type} (${arch})..."
	log "Warning: Using fallback installation method - this may not work reliably"
	log "Consider installing Node.js and using 'npm install -g @anthropic-ai/claude-code' instead"

	# Use the pinned version when one was given; otherwise ask the GitHub API
	# for the latest release (may fail due to rate limits)
	local download_url
	local latest_version=""
	if [[ "$CLAUDE_VERSION" =~ ^[0-9] ]]; then
		latest_version="v${CLAUDE_VERSION}"
	else
		latest_version=$(curl -s --connect-timeout 10 https://api.github.com/repos/anthropics/claude-code/releases/latest 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4)
	fi

	if [[ -z "$latest_version" ]]; then
		log "Warning: Could not fetch latest version from GitHub API"
		log "This fallback method may not work as Claude Code is primarily distributed via npm"
		error_exit "Unable to determine Claude Code version for manual installation"
	fi

	log "Latest Claude Code version: ${latest_version}"

	case "${os_type}" in
	"macos")
		case "${arch}" in
		"arm64") download_url="https://github.com/anthropics/claude-code/releases/download/${latest_version}/claude-${latest_version}-darwin-arm64" ;;
		"amd64") download_url="https://github.com/anthropics/claude-code/releases/download/${latest_version}/claude-${latest_version}-darwin-amd64" ;;
		*) error_exit "Unsupported architecture: ${arch}" ;;
		esac
		;;
	"linux")
		case "${arch}" in
		"arm64") download_url="https://github.com/anthropics/claude-code/releases/download/${latest_version}/claude-${latest_version}-linux-arm64" ;;
		"amd64") download_url="https://github.com/anthropics/claude-code/releases/download/${latest_version}/claude-${latest_version}-linux-amd64" ;;
		*) error_exit "Unsupported architecture: ${arch}" ;;
		esac
		;;
	*) error_exit "Unsupported OS: ${os_type}" ;;
	esac

	# Create temporary directory
	local temp_dir
	temp_dir="$(mktemp -d)"
	local claude_binary="${temp_dir}/claude"
	local checksum_file="${temp_dir}/checksums.txt"

	# Download checksums file (mandatory for security)
	local checksums_url="https://github.com/anthropics/claude-code/releases/download/${latest_version}/checksums.txt"
	log "Downloading checksums from: ${checksums_url}"
	if ! curl -fsSL "${checksums_url}" -o "${checksum_file}"; then
		rm -rf "${temp_dir}"
		error_exit "Failed to download checksums file - cannot verify binary integrity"
	fi

	# Download Claude binary
	log "Downloading from: ${download_url}"
	if ! curl -fsSL "${download_url}" -o "${claude_binary}"; then
		rm -rf "${temp_dir}"
		error_exit "Failed to download Claude binary"
	fi

	# Verify checksum (mandatory)
	log "Verifying checksum..."
	local expected_checksum
	local binary_name
	binary_name="$(basename "$download_url")"
	expected_checksum=$(grep "$binary_name" "$checksum_file" | cut -d' ' -f1)

	if [[ -z "$expected_checksum" ]]; then
		rm -rf "${temp_dir}"
		error_exit "No checksum found for binary in checksums file - cannot verify integrity"
	fi

	# Check for checksum utility
	local actual_checksum
	if command -v sha256sum &>/dev/null; then
		actual_checksum=$(sha256sum "$claude_binary" | cut -d' ' -f1)
	elif command -v shasum &>/dev/null; then
		actual_checksum=$(shasum -a 256 "$claude_binary" | cut -d' ' -f1)
	else
		rm -rf "${temp_dir}"
		error_exit "No checksum utility found (sha256sum or shasum required) - cannot verify integrity"
	fi

	# Verify checksums match
	if [[ "$actual_checksum" != "$expected_checksum" ]]; then
		rm -rf "${temp_dir}"
		error_exit "Checksum verification failed. Expected: $expected_checksum, Got: $actual_checksum"
	fi

	log "Checksum verification passed"

	# Make executable
	chmod +x "${claude_binary}"

	# Install to /usr/local/bin (requires sudo with user consent)
	log "Installing Claude to /usr/local/bin/claude..."
	log "The following command will be executed:"
	log "    sudo mv ${claude_binary} /usr/local/bin/claude"
	echo

	if ! prompt_confirmation "Do you want to proceed with sudo to install Claude Code?"; then
		rm -rf "${temp_dir}"
		log "Installation cancelled by user"
		return 1
	fi

	if ! sudo mv "${claude_binary}" /usr/local/bin/claude; then
		rm -rf "${temp_dir}"
		error_exit "Failed to install Claude to /usr/local/bin/"
	fi

	# Cleanup
	rm -rf "${temp_dir}"

	log "Claude installed successfully via curl"
}

# Verify Claude installation
verify_installation() {
	log "Verifying Claude Code installation..."

	if ! command -v claude &>/dev/null; then
		log "ERROR: claude not found in PATH"
		log "Please restart your shell or run: source ~/.zshrc"
		return 1
	fi

	local version
	version="$(claude --version 2>/dev/null || echo "unknown")"
	if [[ "$CLAUDE_VERSION" =~ ^[0-9] ]] && [[ "$version" != *"$CLAUDE_VERSION"* ]]; then
		log "ERROR: requested version $CLAUDE_VERSION but claude reports: ${version}"
		log "Location: $(command -v claude)"
		return 1
	fi
	log "Installation verified: ${version}"
	log ""
	log "✓ Installation successful!"
	log "Location: $(command -v claude)"
}

# Main installation function
install_claude_main() {
	log "Starting Claude Code installation..."

	# Check if already installed and not forcing
	if [[ "$FORCE_INSTALL" != "true" ]] && check_claude_installed; then
		log "Claude Code is already installed"
		log "Updating to latest version..."
		update_claude
		return 0
	fi

	# Detect OS and architecture
	local os_type
	os_type="$(detect_os)"
	local arch
	arch="$(detect_arch)"

	log "Detected OS: ${os_type}, Architecture: ${arch}"

	# Install based on OS
	case "${os_type}" in
	"macos")
		install_claude_macos
		;;
	"ubuntu" | "linux")
		install_claude_linux "${os_type}"
		;;
	*)
		error_exit "Unsupported operating system: ${os_type}"
		;;
	esac

	# Verify installation
	verify_installation

	log "Claude Code installation completed successfully"
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
			[[ $# -ge 2 && -n "$2" && "$2" != -* ]] ||
				error_exit "--version requires a value: latest, stable, or X.Y.Z"
			CLAUDE_VERSION="$2"
			shift 2
			;;
		--version=*)
			CLAUDE_VERSION="${1#--version=}"
			shift
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

# Normalize and validate CLAUDE_VERSION; reject it where it cannot apply.
validate_version() {
	CLAUDE_VERSION="${CLAUDE_VERSION#v}"
	if [[ ! "$CLAUDE_VERSION" =~ ^(latest|stable|[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?)$ ]]; then
		error_exit "Invalid version '${CLAUDE_VERSION}': expected latest, stable, or X.Y.Z (e.g. 1.0.58)"
	fi
	if [[ "$ACTION" == "uninstall" && "$CLAUDE_VERSION" != "latest" ]]; then
		error_exit "--version applies only to --install and --update, not --uninstall"
	fi
}

# Main function
main() {
	parse_args "$@"
	validate_version

	case "$ACTION" in
	"install")
		install_claude_main
		;;
	"uninstall")
		uninstall_claude
		;;
	"update")
		update_claude
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
