#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[NVIM-INSTALL]"

# Configuration
NVIM_VERSION="${NVIM_VERSION:-stable}"
INSTALL_DIR="${INSTALL_DIR:-/opt}"
DRY_RUN="${DRY_RUN:-false}"

usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install Neovim from official GitHub releases with pre-release support.

OPTIONS:
    --version VERSION   Neovim version to install (default: stable)
                        Use 'nightly' for latest pre-release
                        Use 'stable' for latest stable release
                        Use specific version like 'v0.10.0'
    --update            Update an existing installation (uses stable by default)
    --install-dir DIR   Installation directory (default: /opt)
    --dry-run           Show what would be installed without actually installing
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    NVIM_VERSION       Neovim version to install
    INSTALL_DIR        Installation directory
    DRY_RUN            Enable dry-run mode (true/false)

EXAMPLES:
    $SCRIPT_NAME --version nightly
    $SCRIPT_NAME --version stable
    $SCRIPT_NAME --version v0.10.0
    DRY_RUN=true $SCRIPT_NAME
EOF
	exit "${1:-0}"
}

# Logging functions
log() {
	echo "${LOG_PREFIX} [$(date '+%Y-%m-%d %H:%M:%S')] $*" >&2
}

error_exit() {
	log "ERROR: $1" >&2
	exit "${2:-1}"
}

check_command() {
	if ! command -v "$1" &>/dev/null; then
		return 1
	fi
	return 0
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

detect_platform() {
	local os_type arch_type

	os_type="$(uname -s)"
	arch_type="$(uname -m)"

	# Normalize architecture naming
	case "$arch_type" in
	"aarch64") arch_type="arm64" ;;
	"amd64") arch_type="x86_64" ;;
	esac

	case "$os_type" in
	"Darwin")
		# macOS (supports x86_64 and arm64)
		case "$arch_type" in
		"x86_64") echo "macos-x86_64" ;;
		"arm64") echo "macos-arm64" ;;
		*) error_exit "Unsupported macOS architecture: $arch_type" ;;
		esac
		;;
	"Linux")
		# Linux distributions (Ubuntu, Debian, Fedora, RHEL, CentOS, Arch, openSUSE, WSL)
		# All supported Linux distros use the same Neovim binaries
		case "$arch_type" in
		"x86_64") echo "linux-x86_64" ;;
		"arm64") echo "linux-arm64" ;;
		"armv7l" | "armv6l") error_exit "ARM 32-bit not officially supported. Consider building from source." ;;
		*) error_exit "Unsupported Linux architecture: $arch_type" ;;
		esac
		;;
	"CYGWIN"* | "MINGW"* | "MSYS"*)
		# Windows (Git Bash, MSYS2, Cygwin)
		case "$arch_type" in
		"x86_64") echo "win64" ;;
		*) error_exit "Unsupported Windows architecture: $arch_type" ;;
		esac
		;;
	*)
		error_exit "Unsupported operating system: $os_type"
		;;
	esac
}

get_download_url() {
	local version="$1"
	local platform="$2"

	case "$version" in
	"nightly")
		echo "https://github.com/neovim/neovim/releases/download/nightly/nvim-${platform}.tar.gz"
		;;
	"stable")
		echo "https://github.com/neovim/neovim/releases/latest/download/nvim-${platform}.tar.gz"
		;;
	v*)
		echo "https://github.com/neovim/neovim/releases/download/${version}/nvim-${platform}.tar.gz"
		;;
	*)
		error_exit "Invalid version format: $version. Use 'nightly', 'stable', or 'v0.10.0' format"
		;;
	esac
}

validate_environment() {
	log "Validating environment..."

	# Check required commands
	for cmd in curl tar; do
		if ! check_command "$cmd"; then
			error_exit "Required command not found: $cmd"
		fi
	done

	# Check network connectivity
	if ! curl -s --connect-timeout 5 https://github.com >/dev/null; then
		error_exit "No internet connectivity or GitHub unreachable"
	fi

	# Check write permissions for install directory
	if [[ ! -w "$INSTALL_DIR" ]] && [[ "$EUID" -ne 0 ]]; then
		log "Warning: $INSTALL_DIR is not writable. You may need sudo privileges."
	fi

	log "Environment validation passed"
}

remove_existing_neovim() {
	local platform="$1"
	local nvim_dir="${INSTALL_DIR}/nvim-${platform}"

	if [[ -d "$nvim_dir" ]]; then
		log "Removing existing Neovim installation at $nvim_dir"
		if [[ "$DRY_RUN" == "true" ]]; then
			log "DRY-RUN: Would remove $nvim_dir"
		else
			if [[ -w "$INSTALL_DIR" ]]; then
				rm -rf "$nvim_dir"
			else
				log "Elevated privileges required to remove existing Neovim installation"
				log "The following command will be executed:"
				log "    sudo rm -rf $nvim_dir"
				echo

				if ! prompt_confirmation "Do you want to proceed with sudo?"; then
					error_exit "Cannot remove existing installation without elevated privileges"
				fi

				sudo rm -rf "$nvim_dir"
			fi
		fi
	fi
}

install_neovim() {
	local version="$1"
	local platform="$2"
	local download_url
	local temp_file
	local nvim_dir="${INSTALL_DIR}/nvim-${platform}"

	download_url="$(get_download_url "$version" "$platform")"
	temp_file="/tmp/nvim-${platform}-$$.tar.gz"

	log "Installing Neovim $version for $platform"
	log "Download URL: $download_url"

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would download from $download_url"
		log "DRY-RUN: Would extract to $nvim_dir"
		log "DRY-RUN: Would create symlink at /usr/local/bin/nvim"
		return 0
	fi

	# Download
	log "Downloading Neovim..."
	if ! curl -L --progress-bar -o "$temp_file" "$download_url"; then
		rm -f "$temp_file"
		error_exit "Failed to download Neovim from $download_url"
	fi

	# Attempt to download and verify checksum
	local checksum_url="${download_url}.sha256sum"
	local checksum_file="${temp_file}.sha256sum"
	local checksum_verified=false

	log "Attempting to download checksum file..."
	if curl -L -f -s -o "$checksum_file" "$checksum_url" 2>/dev/null; then
		log "Checksum file downloaded, verifying..."

		# Verify checksum using sha256sum or shasum
		if check_command sha256sum; then
			if (cd "$(dirname "$temp_file")" && sha256sum -c "$checksum_file" 2>/dev/null | grep -q "$(basename "$temp_file"): OK"); then
				checksum_verified=true
				log "Checksum verification passed"
			else
				rm -f "$temp_file" "$checksum_file"
				error_exit "Checksum verification failed - file may be corrupted or tampered with"
			fi
		elif check_command shasum; then
			if (cd "$(dirname "$temp_file")" && shasum -a 256 -c "$checksum_file" 2>/dev/null | grep -q "$(basename "$temp_file"): OK"); then
				checksum_verified=true
				log "Checksum verification passed"
			else
				rm -f "$temp_file" "$checksum_file"
				error_exit "Checksum verification failed - file may be corrupted or tampered with"
			fi
		else
			log "Warning: Neither sha256sum nor shasum available, cannot verify checksum"
		fi

		rm -f "$checksum_file"
	else
		log "Warning: Checksum file not available at $checksum_url"
		log "Warning: Cannot verify download integrity - proceeding without checksum verification"
	fi

	# Validate downloaded file is a valid gzip archive
	# Use gzip -t if file command is not available
	local is_valid_gzip=false
	if check_command file; then
		if file "$temp_file" | grep -q "gzip compressed data"; then
			is_valid_gzip=true
		fi
	elif check_command gzip; then
		if gzip -t "$temp_file" 2>/dev/null; then
			is_valid_gzip=true
		fi
	else
		# If neither file nor gzip is available, check file size and magic bytes
		if [[ -s "$temp_file" ]]; then
			# Check for gzip magic bytes (1f 8b)
			if [[ "$(od -An -t x1 -N 2 "$temp_file" 2>/dev/null | tr -d ' ')" == "1f8b" ]]; then
				is_valid_gzip=true
			fi
		fi
	fi

	if [[ "$is_valid_gzip" == "false" ]]; then
		log "Downloaded file is not a valid gzip archive. Contents:"
		head -n 5 "$temp_file" >&2
		rm -f "$temp_file"
		error_exit "Downloaded file is not a valid tar.gz archive. URL may be incorrect: $download_url"
	fi

	# Remove existing installation
	remove_existing_neovim "$platform"

	# Extract
	log "Extracting Neovim to $INSTALL_DIR..."
	if [[ -w "$INSTALL_DIR" ]]; then
		tar -C "$INSTALL_DIR" -xzf "$temp_file"
	else
		log "Elevated privileges required to extract to $INSTALL_DIR"
		log "The following command will be executed:"
		log "    sudo tar -C $INSTALL_DIR -xzf $temp_file"
		echo

		if ! prompt_confirmation "Do you want to proceed with sudo?"; then
			rm -f "$temp_file"
			error_exit "Cannot extract without elevated privileges"
		fi

		sudo tar -C "$INSTALL_DIR" -xzf "$temp_file"
	fi

	# Clean up
	rm -f "$temp_file"

	# Create symlink
	log "Creating symlink at /usr/local/bin/nvim..."
	if [[ -w "/usr/local/bin" ]]; then
		ln -sf "${nvim_dir}/bin/nvim" /usr/local/bin/nvim
	else
		log "Elevated privileges required to create symlink in /usr/local/bin"
		log "The following command will be executed:"
		log "    sudo ln -sf ${nvim_dir}/bin/nvim /usr/local/bin/nvim"
		echo

		if ! prompt_confirmation "Do you want to proceed with sudo?"; then
			log "Skipping symlink creation - you may need to add Neovim to your PATH manually"
			log "Add this to your shell configuration: export PATH=\"${nvim_dir}/bin:\$PATH\""
		else
			sudo ln -sf "${nvim_dir}/bin/nvim" /usr/local/bin/nvim
		fi
	fi

	log "Neovim $version installed successfully"
}

verify_installation() {
	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Skipping installation verification"
		return 0
	fi

	log "Verifying installation..."

	if ! check_command nvim; then
		error_exit "Neovim command not found after installation"
	fi

	local nvim_version
	nvim_version="$(nvim --version | head -n1)"
	log "Installed: $nvim_version"

	log "Installation verified successfully"
}

update_neovim() {
	local platform="$1"
	
	log "Updating Neovim to latest version..."
	
	# Get current version
	local current_version
	current_version="$(nvim --version | head -n1)"
	log "Current version: $current_version"
	
	# Remove existing installation
	remove_existing_neovim "$platform"
	
	# Install new version
	install_neovim "$NVIM_VERSION" "$platform"
	
	# Verify new version after update
	verify_installation
	
	log "Neovim update completed successfully"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
	case "$1" in
	--version)
		NVIM_VERSION="$2"
		shift 2
		;;
	--update)
		shift
		;;
	--install-dir)
		INSTALL_DIR="$2"
		shift 2
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

main() {
	log "Starting Neovim installation process..."
	[[ "$DRY_RUN" == "true" ]] && log "DRY-RUN MODE: No changes will be made"

	validate_environment

	local platform
	platform="$(detect_platform)"
	log "Detected platform: $platform"

	# Check if nvim is already installed
	if command -v nvim &>/dev/null; then
		local current_version
		current_version="$(nvim --version | head -n1)"
		log "Neovim is already installed: $current_version"
		update_neovim "$platform"
		return 0
	fi

	install_neovim "$NVIM_VERSION" "$platform"
	verify_installation

	log "Neovim installation completed successfully"
}

main "$@"
