#!/usr/bin/env bash

# Install Lua 5.1 and LuaRocks
# Required for LazyVim and Neovim plugins
# Supports macOS, Linux (Ubuntu, Fedora, Arch, openSUSE), and other Unix-like systems

set -euo pipefail

# Constants
SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[LUA-INSTALL]"
readonly LUA_VERSION="${LUA_VERSION:-5.1}"
readonly LUAROCKS_VERSION="${LUAROCKS_VERSION:-3.11.1}"

# Configuration
ACTION="${ACTION:-install}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local}"

# Usage function
usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install Lua 5.1 and LuaRocks for LazyVim/Neovim plugin support.

OPTIONS:
    --install           Install Lua and LuaRocks (default)
    --uninstall         Uninstall Lua and LuaRocks
    --lua-version VER   Lua version to install (default: 5.1)
    --rocks-version VER LuaRocks version to install (default: 3.11.1)
    --install-dir DIR   Installation directory (default: /usr/local)
    --force             Force installation even if already installed
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    ACTION             Action to perform (install, uninstall)
    LUA_VERSION        Lua version (default: 5.1)
    LUAROCKS_VERSION   LuaRocks version (default: 3.11.1)
    FORCE_INSTALL      Force installation (true/false)
    INSTALL_DIR        Installation directory

EXAMPLES:
    $SCRIPT_NAME --install
    $SCRIPT_NAME --uninstall
    $SCRIPT_NAME --lua-version 5.1 --rocks-version 3.11.1
    FORCE_INSTALL=true $SCRIPT_NAME

INSTALLATION METHODS (by platform):
    macOS:
        1. Homebrew: brew install lua@5.1 luarocks
        
    Linux (Ubuntu/Debian):
        1. APT: apt install lua5.1 liblua5.1-dev luarocks
        
    Linux (Fedora/RHEL):
        1. DNF/YUM: dnf install lua luarocks
        
    Linux (Arch):
        1. Pacman: pacman -S lua51 luarocks
        
    Linux (openSUSE):
        1. Zypper: zypper install lua51 lua51-devel luarocks

    Fallback (all platforms):
        2. Build from source (requires build tools)

PREREQUISITES:
    - For package manager methods: respective package manager
    - For source build: gcc, make, readline-devel, ncurses-devel
EOF
	exit "${1:-0}"
}

# Logging functions
log() {
	echo "${LOG_PREFIX} $*" >&2
}

log_success() {
	echo "${LOG_PREFIX} ✓ $*" >&2
}

log_warning() {
	echo "${LOG_PREFIX} ⚠ $*" >&2
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
		source /etc/os-release
		case "${ID:-unknown}" in
		ubuntu | debian) echo "debian" ;;
		fedora | rhel | centos | rocky | almalinux) echo "redhat" ;;
		arch | cachyos | manjaro | endeavouros) echo "arch" ;;
		opensuse* | sles) echo "suse" ;;
		*)
			case "${ID_LIKE:-}" in
			*debian* | *ubuntu*) echo "debian" ;;
			*fedora* | *rhel*) echo "redhat" ;;
			*arch*) echo "arch" ;;
			*suse*) echo "suse" ;;
			*) echo "linux" ;;
			esac
			;;
		esac
	elif grep -qi microsoft /proc/version 2>/dev/null; then
		echo "wsl"
	else
		echo "unknown"
	fi
}

# Check if Lua is already installed
check_lua_installed() {
	if command -v lua &>/dev/null; then
		local version
		version="$(lua -v 2>&1 | head -n1)"
		log "Lua is already installed: ${version}"
		return 0
	else
		return 1
	fi
}

# Check if LuaRocks is already installed
check_luarocks_installed() {
	if command -v luarocks &>/dev/null; then
		local version
		version="$(luarocks --version 2>&1 | head -n1)"
		log "LuaRocks is already installed: ${version}"
		return 0
	else
		return 1
	fi
}

# Install Lua and LuaRocks on macOS via Homebrew
install_lua_macos() {
	log "Installing Lua and LuaRocks on macOS..."

	if ! command -v brew &>/dev/null; then
		error_exit "Homebrew not found. Please install Homebrew first."
	fi

	# Note: Neovim comes with LuaJIT (Lua 5.1 compatible) built-in
	# We install LuaJIT separately for LuaRocks compatibility
	# LuaJIT is Lua 5.1 API compatible and works with most Lua 5.1 code
	
	local lua_package="luajit"
	
	# Install LuaJIT (Lua 5.1 compatible)
	if ! brew list "$lua_package" &>/dev/null 2>&1; then
		log "Installing $lua_package (Lua 5.1 compatible JIT compiler)..."
		if ! brew install "$lua_package"; then
			log_warning "Failed to install $lua_package via Homebrew, trying fallback..."
			# Fallback to regular lua if luajit fails
			lua_package="lua"
			if ! brew install "$lua_package"; then
				error_exit "Failed to install Lua via Homebrew"
			fi
		fi
	else
		log_success "$lua_package is already installed"
	fi

	# Install LuaRocks
	if ! brew list luarocks &>/dev/null 2>&1; then
		log "Installing luarocks..."
		if ! brew install luarocks; then
			error_exit "Failed to install luarocks via Homebrew"
		fi
	else
		log_success "luarocks is already installed"
	fi

	log_success "Lua and LuaRocks installed successfully via Homebrew"
}

# Install Lua and LuaRocks on Debian/Ubuntu
install_lua_debian() {
	log "Installing Lua and LuaRocks on Debian/Ubuntu..."

	# Update package lists
	log "Updating package lists..."
	if ! sudo apt update; then
		error_exit "Failed to update apt package lists"
	fi

	# Install Lua 5.1 and development files
	local packages=(
		"lua5.1"
		"liblua5.1-0-dev"
		"luarocks"
	)

	log "Installing packages: ${packages[*]}"
	if ! sudo apt install -y "${packages[@]}"; then
		error_exit "Failed to install Lua packages via apt"
	fi

	log_success "Lua and LuaRocks installed successfully via apt"
}

# Install Lua and LuaRocks on Fedora/RHEL
install_lua_redhat() {
	log "Installing Lua and LuaRocks on Fedora/RHEL..."

	local pkg_cmd
	if command -v dnf &>/dev/null; then
		pkg_cmd="dnf"
	elif command -v yum &>/dev/null; then
		pkg_cmd="yum"
	else
		error_exit "No package manager found (dnf or yum required)"
	fi

	# Install Lua and LuaRocks
	local packages=(
		"lua"
		"lua-devel"
		"luarocks"
	)

	log "Installing packages: ${packages[*]}"
	if ! sudo "$pkg_cmd" install -y "${packages[@]}"; then
		error_exit "Failed to install Lua packages via $pkg_cmd"
	fi

	log_success "Lua and LuaRocks installed successfully via $pkg_cmd"
}

# Install Lua and LuaRocks on Arch Linux
install_lua_arch() {
	log "Installing Lua and LuaRocks on Arch Linux..."

	# Install Lua 5.1 and LuaRocks
	local packages=(
		"lua51"
		"luarocks"
	)

	log "Installing packages: ${packages[*]}"
	if ! sudo pacman -S --needed --noconfirm "${packages[@]}"; then
		error_exit "Failed to install Lua packages via pacman"
	fi

	log_success "Lua and LuaRocks installed successfully via pacman"
}

# Install Lua and LuaRocks on openSUSE
install_lua_suse() {
	log "Installing Lua and LuaRocks on openSUSE..."

	# Install Lua 5.1 and LuaRocks
	local packages=(
		"lua51"
		"lua51-devel"
		"luarocks"
	)

	log "Installing packages: ${packages[*]}"
	if ! sudo zypper install -y "${packages[@]}"; then
		error_exit "Failed to install Lua packages via zypper"
	fi

	log_success "Lua and LuaRocks installed successfully via zypper"
}

# Build Lua from source (fallback method)
build_lua_from_source() {
	log "Building Lua from source..."
	log_warning "This is a fallback method and requires build tools"

	# Check for required build tools
	local required_tools=("gcc" "make" "curl")
	for tool in "${required_tools[@]}"; do
		if ! command -v "$tool" &>/dev/null; then
			error_exit "Required tool not found: $tool"
		fi
	done

	local temp_dir
	temp_dir="$(mktemp -d -t lua-build-XXXXXX)"
	local lua_tarball="${temp_dir}/lua-${LUA_VERSION}.tar.gz"
	local lua_url="https://www.lua.org/ftp/lua-${LUA_VERSION}.tar.gz"

	log "Using temporary directory: ${temp_dir}"
	log "Downloading Lua ${LUA_VERSION}..."
	if ! curl -fsSL "$lua_url" -o "$lua_tarball"; then
		rm -rf "$temp_dir"
		error_exit "Failed to download Lua source"
	fi

	log "Extracting Lua source..."
	if ! tar -xzf "$lua_tarball" -C "$temp_dir"; then
		rm -rf "$temp_dir"
		error_exit "Failed to extract Lua source"
	fi

	log "Building Lua..."
	local lua_src_dir="${temp_dir}/lua-${LUA_VERSION}"
	
	# Detect platform for make target
	local make_target
	if [[ "$(uname)" == "Darwin" ]]; then
		make_target="macosx"
	else
		make_target="linux"
	fi

	if ! (cd "$lua_src_dir" && make "$make_target" test); then
		rm -rf "$temp_dir"
		error_exit "Failed to build Lua"
	fi

	log "Installing Lua to ${INSTALL_DIR}..."
	if [[ -w "$INSTALL_DIR" ]]; then
		(cd "$lua_src_dir" && make install INSTALL_TOP="$INSTALL_DIR")
	else
		log "Elevated privileges required to install to ${INSTALL_DIR}"
		if ! prompt_confirmation "Do you want to proceed with sudo?"; then
			rm -rf "$temp_dir"
			error_exit "Installation cancelled"
		fi
		(cd "$lua_src_dir" && sudo make install INSTALL_TOP="$INSTALL_DIR")
	fi

	rm -rf "$temp_dir"
	log_success "Lua built and installed from source"
}

# Build LuaRocks from source
build_luarocks_from_source() {
	log "Building LuaRocks from source..."

	# Check for required build tools
	local required_tools=("gcc" "make" "curl")
	for tool in "${required_tools[@]}"; do
		if ! command -v "$tool" &>/dev/null; then
			error_exit "Required tool not found: $tool"
		fi
	done

	# Check if Lua is installed
	if ! command -v lua &>/dev/null; then
		error_exit "Lua must be installed before building LuaRocks"
	fi

	local temp_dir
	temp_dir="$(mktemp -d -t luarocks-build-XXXXXX)"
	local rocks_tarball="${temp_dir}/luarocks-${LUAROCKS_VERSION}.tar.gz"
	local rocks_url="https://luarocks.org/releases/luarocks-${LUAROCKS_VERSION}.tar.gz"

	log "Using temporary directory: ${temp_dir}"
	log "Downloading LuaRocks ${LUAROCKS_VERSION}..."
	if ! curl -fsSL "$rocks_url" -o "$rocks_tarball"; then
		rm -rf "$temp_dir"
		error_exit "Failed to download LuaRocks source"
	fi

	log "Extracting LuaRocks source..."
	if ! tar -xzf "$rocks_tarball" -C "$temp_dir"; then
		rm -rf "$temp_dir"
		error_exit "Failed to extract LuaRocks source"
	fi

	log "Configuring and building LuaRocks..."
	local rocks_src_dir="${temp_dir}/luarocks-${LUAROCKS_VERSION}"
	
	if ! (cd "$rocks_src_dir" && ./configure --prefix="$INSTALL_DIR" --with-lua-include="${INSTALL_DIR}/include"); then
		rm -rf "$temp_dir"
		error_exit "Failed to configure LuaRocks"
	fi

	if ! (cd "$rocks_src_dir" && make); then
		rm -rf "$temp_dir"
		error_exit "Failed to build LuaRocks"
	fi

	log "Installing LuaRocks to ${INSTALL_DIR}..."
	if [[ -w "$INSTALL_DIR" ]]; then
		(cd "$rocks_src_dir" && make install)
	else
		log "Elevated privileges required to install to ${INSTALL_DIR}"
		if ! prompt_confirmation "Do you want to proceed with sudo?"; then
			rm -rf "$temp_dir"
			error_exit "Installation cancelled"
		fi
		(cd "$rocks_src_dir" && sudo make install)
	fi

	rm -rf "$temp_dir"
	log_success "LuaRocks built and installed from source"
}

# Uninstall Lua and LuaRocks
uninstall_lua() {
	log "Uninstalling Lua and LuaRocks..."

	local os_type
	os_type="$(detect_os)"

	case "$os_type" in
	"macos")
		if command -v brew &>/dev/null; then
			brew list lua@5.1 &>/dev/null && brew uninstall lua@5.1
			brew list luarocks &>/dev/null && brew uninstall luarocks
			log_success "Lua and LuaRocks uninstalled via Homebrew"
		fi
		;;
	"debian")
		sudo apt remove -y lua5.1 liblua5.1-0-dev luarocks
		log_success "Lua and LuaRocks uninstalled via apt"
		;;
	"redhat")
		local pkg_cmd
		pkg_cmd="$(command -v dnf || command -v yum)"
		sudo "$pkg_cmd" remove -y lua lua-devel luarocks
		log_success "Lua and LuaRocks uninstalled via $pkg_cmd"
		;;
	"arch")
		sudo pacman -R --noconfirm lua51 luarocks
		log_success "Lua and LuaRocks uninstalled via pacman"
		;;
	"suse")
		sudo zypper remove -y lua51 lua51-devel luarocks
		log_success "Lua and LuaRocks uninstalled via zypper"
		;;
	*)
		log_warning "Manual uninstall required for ${os_type}"
		;;
	esac
}

# Verify installation
verify_installation() {
	log "Verifying Lua and LuaRocks installation..."

	local lua_found=false
	local luarocks_found=false

	# Check for lua or lua5.1
	if command -v lua &>/dev/null; then
		local lua_version
		lua_version="$(lua -v 2>&1 | head -n1)"
		log_success "Lua installed: ${lua_version}"
		log_success "Location: $(command -v lua)"
		lua_found=true
	elif command -v lua5.1 &>/dev/null; then
		local lua_version
		lua_version="$(lua5.1 -v 2>&1 | head -n1)"
		log_success "Lua 5.1 installed: ${lua_version}"
		log_success "Location: $(command -v lua5.1)"
		lua_found=true
	else
		log_warning "Lua not found in PATH"
	fi

	# Check for luarocks
	if command -v luarocks &>/dev/null; then
		local rocks_version
		rocks_version="$(luarocks --version 2>&1 | head -n1)"
		log_success "LuaRocks installed: ${rocks_version}"
		log_success "Location: $(command -v luarocks)"
		luarocks_found=true
	else
		log_warning "LuaRocks not found in PATH"
	fi

	if [[ "$lua_found" == "true" ]] && [[ "$luarocks_found" == "true" ]]; then
		log_success "Installation verified successfully!"
		return 0
	else
		log_warning "Installation verification incomplete"
		return 1
	fi
}

# Main installation function
install_lua_main() {
	log "Starting Lua and LuaRocks installation..."

	# Check if already installed and not forcing
	if [[ "$FORCE_INSTALL" != "true" ]]; then
		local lua_installed=false
		local rocks_installed=false
		
		check_lua_installed && lua_installed=true
		check_luarocks_installed && rocks_installed=true
		
		if [[ "$lua_installed" == "true" ]] && [[ "$rocks_installed" == "true" ]]; then
			log_success "Lua and LuaRocks are already installed"
			verify_installation
			return 0
		fi
	fi

	# Detect OS
	local os_type
	os_type="$(detect_os)"
	log "Detected OS: ${os_type}"

	# Install based on OS
	case "$os_type" in
	"macos")
		install_lua_macos
		;;
	"debian" | "wsl")
		install_lua_debian
		;;
	"redhat")
		install_lua_redhat
		;;
	"arch")
		install_lua_arch
		;;
	"suse")
		install_lua_suse
		;;
	*)
		log_warning "Unsupported OS: ${os_type}, attempting source build..."
		build_lua_from_source
		build_luarocks_from_source
		;;
	esac

	# Verify installation
	verify_installation

	log_success "Lua and LuaRocks installation completed successfully"
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
		--lua-version)
			LUA_VERSION="$2"
			shift 2
			;;
		--rocks-version)
			LUAROCKS_VERSION="$2"
			shift 2
			;;
		--install-dir)
			INSTALL_DIR="$2"
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
		install_lua_main
		;;
	"uninstall")
		uninstall_lua
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
