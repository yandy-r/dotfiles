#!/usr/bin/env bash

# Install Daniel Miessler's Fabric CLI
# Supports macOS, Linux (Ubuntu, Fedora, Arch, openSUSE, WSL), and other Unix-like systems

set -euo pipefail

# Constants
SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[FABRIC-INSTALL]"
readonly FABRIC_INSTALL_DIR="${FABRIC_INSTALL_DIR:-$HOME/.local/bin}"
readonly FABRIC_INSTALLER_URL="https://raw.githubusercontent.com/danielmiessler/fabric/main/scripts/installer/install.sh"
readonly FABRIC_GO_PACKAGE="github.com/danielmiessler/fabric/cmd/fabric"

# Configuration
ACTION="${ACTION:-install}"
FABRIC_VERSION="${FABRIC_VERSION:-latest}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"

usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install, update, or uninstall Daniel Miessler's Fabric CLI.

OPTIONS:
    --install           Install Fabric (default)
    --uninstall         Uninstall Fabric
    --update            Update Fabric to latest version
    --version VERSION   Install specific Go package version (default: latest)
    --force             Force installation even if already installed
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    ACTION              Action to perform (install, uninstall, update)
    FABRIC_VERSION      Version to install via Go fallback (default: latest)
    FABRIC_INSTALL_DIR  Official installer destination (default: \$HOME/.local/bin)
    FORCE_INSTALL       Force installation (true/false)
    NON_INTERACTIVE     Auto-approve prompts (true/false or 1/0)
    NONINTERACTIVE      Same as NON_INTERACTIVE (legacy)

EXAMPLES:
    $SCRIPT_NAME --install
    $SCRIPT_NAME --uninstall
    $SCRIPT_NAME --update
    FABRIC_INSTALL_DIR=\$HOME/.local/bin $SCRIPT_NAME --install
    FABRIC_VERSION=v1.4.455 $SCRIPT_NAME --install

INSTALLATION METHODS (tried in order by platform):
    All platforms:
        1. Official installer: $FABRIC_INSTALLER_URL
    macOS and Linux with Homebrew:
        2. Homebrew: brew install fabric-ai
    Arch Linux:
        2. AUR: paru/yay -S fabric-ai
    Any platform with Go:
        3. Go: go install github.com/danielmiessler/fabric/cmd/fabric@latest

NEXT STEP:
    After installation, run: fabric --setup
    If only fabric-ai is available, run: fabric-ai --setup
EOF
	exit "${1:-0}"
}

log() { echo "${LOG_PREFIX} $*" >&2; }
error_exit() {
	log "ERROR: $1" >&2
	exit "${2:-1}"
}

is_interactive() { [[ -t 0 ]] && [[ -t 1 ]]; }

is_non_interactive() {
	[[ "${NON_INTERACTIVE:-false}" == "true" ]] || [[ "${NON_INTERACTIVE:-0}" == "1" ]] || [[ "${NONINTERACTIVE:-0}" == "1" ]]
}

# aur_helper_run <helper> <-S|-R> <package>: run paru/yay, adding the headless
# flags (--noconfirm, plus --needed for -S) when NON_INTERACTIVE is set so the
# update/uninstall paths never block on a confirmation prompt (YAN-1398).
aur_helper_run() {
	local helper="$1" op="$2" pkg="$3"
	local -a args=("$op")

	if is_non_interactive; then
		if [[ "$op" == "-S" ]]; then
			args+=(--needed)
		fi
		args+=(--noconfirm)
	fi

	"$helper" "${args[@]}" "$pkg"
}

prompt_confirmation() {
	local prompt_message="$1"
	local response

	if is_non_interactive || [[ "${FORCE_INSTALL:-false}" == "true" ]]; then
		log "Non-interactive mode: auto-approving prompt"
		return 0
	fi

	if ! is_interactive; then
		log "ERROR: Non-interactive shell detected - cannot prompt for confirmation"
		return 1
	fi

	echo -n "${LOG_PREFIX} ${prompt_message} [y/N]: " >&2
	read -r response
	response="${response:-n}"

	case "$response" in
		[yY] | [yY][eE][sS]) return 0 ;;
		*) return 1 ;;
	esac
}

detect_os() {
	if [[ "$(uname)" == "Darwin" ]]; then
		echo "macos"
	elif [[ -f /etc/os-release ]]; then
		local os_id os_id_like
		os_id="$(sed -n 's/^ID=//p' /etc/os-release | head -n1 | tr -d '"')"
		os_id_like="$(sed -n 's/^ID_LIKE=//p' /etc/os-release | head -n1 | tr -d '"' || true)"
		case "$os_id" in
			ubuntu | debian) echo "ubuntu" ;;
			fedora) echo "fedora" ;;
			arch) echo "arch" ;;
			opensuse* | sles) echo "opensuse" ;;
			*)
				case "$os_id_like" in
					*arch*) echo "arch" ;;
					*ubuntu* | *debian*) echo "ubuntu" ;;
					*fedora* | *rhel*) echo "fedora" ;;
					*suse*) echo "opensuse" ;;
					*) echo "linux" ;;
				esac
				;;
		esac
	elif [[ "$(uname)" == "Linux" ]]; then
		echo "linux"
	else
		echo "unknown"
	fi
}

is_wsl() { [[ -n "${WSL_DISTRO_NAME:-}" ]] || grep -qi microsoft /proc/version 2>/dev/null; }

detect_arch() {
	case "$(uname -m)" in
		x86_64) echo "amd64" ;;
		arm64 | aarch64) echo "arm64" ;;
		*) echo "unknown" ;;
	esac
}

fabric_command() {
	if command -v fabric &>/dev/null; then
		echo "fabric"
		return 0
	fi
	if command -v fabric-ai &>/dev/null; then
		echo "fabric-ai"
		return 0
	fi
	return 1
}

check_fabric_installed() {
	local fabric_cmd version
	if fabric_cmd="$(fabric_command)"; then
		version="$("$fabric_cmd" --version 2>/dev/null || echo "unknown")"
		log "Fabric is already installed as ${fabric_cmd}: ${version}"
		return 0
	fi
	return 1
}

remove_file_if_exists() {
	local path="$1"

	if [[ ! -e "$path" ]]; then
		return 1
	fi

	log "Removing $path"
	rm -f "$path"
	return 0
}

uninstall_fabric() {
	log "Uninstalling Fabric..."

	local os_type removed=false
	os_type="$(detect_os)"

	if command -v brew &>/dev/null && brew list fabric-ai &>/dev/null; then
		log "Uninstalling Fabric via Homebrew..."
		brew uninstall fabric-ai
		removed=true
	fi

	if [[ "$os_type" == "arch" ]]; then
		if command -v paru &>/dev/null && paru -Q fabric-ai &>/dev/null; then
			log "Uninstalling Fabric via Paru..."
			aur_helper_run paru -R fabric-ai
			removed=true
		elif command -v yay &>/dev/null && yay -Q fabric-ai &>/dev/null; then
			log "Uninstalling Fabric via Yay..."
			aur_helper_run yay -R fabric-ai
			removed=true
		fi
	fi

	if remove_file_if_exists "${FABRIC_INSTALL_DIR}/fabric"; then
		removed=true
	fi

	local go_bin
	go_bin="${GOBIN:-${GOPATH:-$HOME/go}/bin}/fabric"
	if remove_file_if_exists "$go_bin"; then
		removed=true
	fi

	if [[ "$removed" == "true" ]]; then
		log "Fabric uninstall completed"
	else
		log "Fabric was not found in known installation locations"
	fi
}

update_fabric() {
	log "Updating Fabric to latest version..."

	if ! check_fabric_installed; then
		log "Fabric is not installed. Use --install instead."
		return 1
	fi

	local os_type
	os_type="$(detect_os)"

	if command -v brew &>/dev/null && brew list fabric-ai &>/dev/null; then
		log "Updating Fabric via Homebrew..."
		if ! brew upgrade fabric-ai; then
			log "Failed to update Fabric via Homebrew"
			return 1
		fi
		verify_installation
		return 0
	fi

	if [[ "$os_type" == "arch" ]]; then
		if command -v paru &>/dev/null && paru -Q fabric-ai &>/dev/null; then
			log "Updating Fabric via Paru..."
			if ! aur_helper_run paru -S fabric-ai; then
				log "Failed to update Fabric via Paru"
				return 1
			fi
			verify_installation
			return 0
		elif command -v yay &>/dev/null && yay -Q fabric-ai &>/dev/null; then
			log "Updating Fabric via Yay..."
			if ! aur_helper_run yay -S fabric-ai; then
				log "Failed to update Fabric via Yay"
				return 1
			fi
			verify_installation
			return 0
		fi
	fi

	log "Could not detect package-manager installation method, trying official installer update..."
	if install_fabric_official; then
		verify_installation
		return 0
	fi

	log "Official installer update failed or was cancelled. Run --install --force to reinstall with fallback methods."
	return 1
}

install_fabric_official() {
	log "Installing Fabric via official installer..."

	if ! command -v curl &>/dev/null; then
		log "curl not found - required for official installer"
		return 1
	fi

	log ""
	log "WARNING: SECURITY NOTICE"
	log "========================================"
	log "This will download and execute a remote script from:"
	log "    ${FABRIC_INSTALLER_URL}"
	log ""
	log "The script will install Fabric into:"
	log "    ${FABRIC_INSTALL_DIR}"
	log ""
	log "INTEGRITY VERIFICATION:"
	log "  - This installation method does NOT verify checksums or signatures"
	log "  - You are trusting the raw.githubusercontent.com domain and its SSL certificate"
	log "  - For maximum security, review the script first:"
	log "    curl -fsSL ${FABRIC_INSTALLER_URL}"
	log "========================================"
	log ""

	if ! prompt_confirmation "Do you want to proceed with downloading and executing the official installer?"; then
		log "Installation cancelled by user"
		return 1
	fi

	local temp_dir installer
	temp_dir="$(mktemp -d)"
	installer="${temp_dir}/install-fabric.sh"

	if ! curl -fsSL "$FABRIC_INSTALLER_URL" -o "$installer"; then
		rm -rf "$temp_dir"
		log "Failed to download official installer"
		return 1
	fi

	chmod +x "$installer"
	mkdir -p "$FABRIC_INSTALL_DIR"

	if ! INSTALL_DIR="$FABRIC_INSTALL_DIR" bash "$installer"; then
		rm -rf "$temp_dir"
		log "Failed to install Fabric via official installer"
		return 1
	fi

	rm -rf "$temp_dir"
	export PATH="${FABRIC_INSTALL_DIR}:${PATH}"
	log "Fabric installed successfully via official installer"
	return 0
}

install_fabric_homebrew() {
	log "Installing Fabric via Homebrew..."

	if ! command -v brew &>/dev/null; then
		log "Homebrew not found"
		return 1
	fi

	if ! brew install fabric-ai; then
		log "Failed to install Fabric via Homebrew"
		return 1
	fi

	log "Fabric installed successfully via Homebrew as fabric-ai"
	return 0
}

install_fabric_aur() {
	local aur_helper=""

	if command -v paru &>/dev/null; then
		aur_helper="paru"
	elif command -v yay &>/dev/null; then
		aur_helper="yay"
	else
		log "No supported AUR helper found (paru or yay)"
		return 1
	fi

	log "Installing Fabric via ${aur_helper}..."
	if ! "$aur_helper" -S --noconfirm fabric-ai; then
		log "Failed to install Fabric via ${aur_helper}"
		return 1
	fi

	log "Fabric installed successfully via ${aur_helper} as fabric-ai"
	return 0
}

install_fabric_go() {
	log "Installing Fabric via Go..."

	if ! command -v go &>/dev/null; then
		log "Go not found"
		return 1
	fi

	local version package
	if [[ "$FABRIC_VERSION" == "latest" ]]; then
		version="latest"
	else
		version="$FABRIC_VERSION"
	fi
	package="${FABRIC_GO_PACKAGE}@${version}"

	if ! go install "$package"; then
		log "Failed to install Fabric via Go"
		return 1
	fi

	local go_bin
	go_bin="${GOBIN:-${GOPATH:-$HOME/go}/bin}"
	export PATH="${go_bin}:${PATH}"
	log "Fabric installed successfully via Go"
	return 0
}

install_fabric_macos() {
	log "Installing Fabric on macOS..."

	if install_fabric_official; then return 0; fi
	log "Official installer failed or was cancelled, trying Homebrew..."

	if install_fabric_homebrew; then return 0; fi
	log "Homebrew installation failed, trying Go..."

	if install_fabric_go; then return 0; fi

	error_exit "All installation methods failed for macOS"
}

install_fabric_linux() {
	local os_type="$1"
	log "Installing Fabric on Linux (${os_type})..."

	if install_fabric_official; then return 0; fi
	log "Official installer failed or was cancelled, trying package manager fallbacks..."

	if command -v brew &>/dev/null && install_fabric_homebrew; then
		return 0
	fi

	if [[ "$os_type" == "arch" ]] && install_fabric_aur; then
		return 0
	fi

	log "Package manager installation failed, trying Go..."
	if install_fabric_go; then return 0; fi

	error_exit "All installation methods failed for Linux (${os_type})"
}

verify_installation() {
	log "Verifying Fabric installation..."

	local fabric_cmd version
	if ! fabric_cmd="$(fabric_command)"; then
		local search_dirs=("${FABRIC_INSTALL_DIR}" "${GOBIN:-${GOPATH:-$HOME/go}/bin}" "$HOME/.local/bin" "/usr/local/bin")
		for dir in "${search_dirs[@]}"; do
			if [[ -x "${dir}/fabric" ]]; then
				log "Fabric binary found at: ${dir}/fabric"
				log ""
				log "WARNING: ${dir} is not in your PATH. To fix, add to your shell profile:"
				log ""
				log "       export PATH=\"${dir}:\$PATH\""
				log ""
				log "Then restart your shell or run: exec \$SHELL -l"
				return 1
			fi
			if [[ -x "${dir}/fabric-ai" ]]; then
				log "Fabric binary found at: ${dir}/fabric-ai"
				log ""
				log "WARNING: ${dir} is not in your PATH. To fix, add to your shell profile:"
				log ""
				log "       export PATH=\"${dir}:\$PATH\""
				log ""
				log "Then restart your shell or run: exec \$SHELL -l"
				return 1
			fi
		done

		log "ERROR: fabric or fabric-ai not found in PATH or common install locations"
		return 1
	fi

	version="$("$fabric_cmd" --version 2>/dev/null || echo "unknown")"
	log "Installation verified: ${version}"
	log ""
	log "Installation successful"
	log "Command: ${fabric_cmd}"
	log "Location: $(command -v "$fabric_cmd")"
	log ""
	log "Manual next step: ${fabric_cmd} --setup"
	if [[ "$fabric_cmd" == "fabric-ai" ]]; then
		log "Note: Fabric is installed as fabric-ai; after shell integration is loaded, Fabric helpers will normalize this command."
	fi
}

install_fabric_main() {
	log "Starting Fabric installation..."

	if [[ "$FORCE_INSTALL" != "true" ]] && check_fabric_installed; then
		log "Fabric is already installed"
		log "Updating to latest version..."
		update_fabric
		return $?
	fi

	local os_type arch
	os_type="$(detect_os)"
	arch="$(detect_arch)"
	log "Detected OS: ${os_type}, Architecture: ${arch}"

	if [[ "$arch" == "unknown" ]]; then
		log "Warning: unsupported architecture for official/package-manager installers; Go fallback may still work"
	fi

	if is_wsl; then
		log "WSL detected, using Linux installation method"
	fi

	case "$os_type" in
		macos)
			install_fabric_macos
			;;
		ubuntu | fedora | arch | opensuse | linux)
			install_fabric_linux "$os_type"
			;;
		*)
			error_exit "Unsupported operating system: ${os_type}"
			;;
	esac

	verify_installation
	log "Fabric installation completed successfully"
}

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
				if [[ $# -lt 2 ]]; then
					error_exit "--version requires a value"
				fi
				FABRIC_VERSION="$2"
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

main() {
	parse_args "$@"

	case "$ACTION" in
		install)
			install_fabric_main
			;;
		uninstall)
			uninstall_fabric
			;;
		update)
			update_fabric
			;;
		*)
			error_exit "Invalid action: $ACTION. Use --help for usage information."
			;;
	esac
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
