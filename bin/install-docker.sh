#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[DOCKER-INSTALL]"

DOCKER_VERSION="${DOCKER_VERSION:-latest}"
DRY_RUN="${DRY_RUN:-false}"
SKIP_POST_INSTALL="${SKIP_POST_INSTALL:-false}"
NON_INTERACTIVE="${NON_INTERACTIVE:-false}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"

DOTFILES_DIR="${DOTFILES_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
readonly PLATFORM_SCRIPT="${DOTFILES_DIR}/common/platform.sh"

usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install Docker Engine from official repositories with multi-OS support.

OPTIONS:
    --version VERSION   Docker version to install (default: latest)
                        Use 'latest' for the most recent stable release
    --skip-post         Skip post-installation setup (docker group, service start)
    --dry-run           Show what would be installed without actually installing
    --force             Reinstall/upgrade without prompting for confirmation
    --non-interactive   Auto-approve prompts (no TTY required)
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    DOCKER_VERSION      Docker version to install
    DRY_RUN             Enable dry-run mode (true/false)
    SKIP_POST_INSTALL   Skip post-installation setup (true/false)
    NON_INTERACTIVE     Auto-approve prompts (true/false)
    FORCE_INSTALL       Force installation without prompts (true/false)

EXAMPLES:
    $SCRIPT_NAME
    $SCRIPT_NAME --version latest
    $SCRIPT_NAME --dry-run
    $SCRIPT_NAME --skip-post
    $SCRIPT_NAME --force
    $SCRIPT_NAME --non-interactive
    DRY_RUN=true $SCRIPT_NAME
    FORCE_INSTALL=true $SCRIPT_NAME

SUPPORTED PLATFORMS:
    - Ubuntu/Debian (via official Docker apt repository)
    - macOS (via Homebrew - Docker Desktop)
    - Fedora (via official Docker dnf repository)
    - Arch Linux (via official repositories)

NOTES:
    - Ubuntu/Debian: Installs Docker Engine from official Docker repository
    - macOS: Installs Docker Desktop via Homebrew Cask
    - Post-installation adds current user to docker group
    - Service start/enable handled automatically on supported platforms
    - Requires sudo/root privileges for system-level installation
EOF
	exit "${1:-0}"
}

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

is_interactive() {
	[[ -t 0 ]] && [[ -t 1 ]]
}

# Membership recorded in the group database (takes effect on next login).
docker_group_configured() {
	id -nG "$USER" 2>/dev/null | tr ' ' '\n' | grep -qx docker
}

# Membership active in the *current* process credentials. A freshly added
# supplementary group is absent here until the user re-logs in, so this is what
# decides whether `docker` works without sudo right now.
docker_group_active() {
	id -nG 2>/dev/null | tr ' ' '\n' | grep -qx docker
}

prompt_confirmation() {
	local prompt_message="$1"
	local response

	if [[ "${NON_INTERACTIVE:-false}" == "true" ]] || [[ "${FORCE_INSTALL:-false}" == "true" ]] || [[ "${NONINTERACTIVE:-0}" == "1" ]]; then
		log "Non-interactive mode: auto-approving prompt"
		return 0
	fi

	if ! is_interactive; then
		log "ERROR: Non-interactive shell detected - cannot prompt for confirmation"
		log "Set NON_INTERACTIVE=true or FORCE_INSTALL=true to proceed automatically"
		return 1
	fi

	echo -n "${LOG_PREFIX} ${prompt_message} [y/N]: " >&2
	read -r response

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

validate_environment() {
	log "Validating environment..."

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN MODE: Skipping environment validation"
		return 0
	fi

	local required_cmds=(curl)
	for cmd in "${required_cmds[@]}"; do
		if ! check_command "$cmd"; then
			error_exit "Required command not found: $cmd"
		fi
	done

	if ! curl -s --connect-timeout 5 https://download.docker.com >/dev/null; then
		error_exit "No internet connectivity or Docker servers unreachable"
	fi

	log "Environment validation passed"
}

check_existing_docker() {
	if check_command docker; then
		local docker_version
		docker_version="$(timeout 5 docker --version 2>/dev/null || echo "unknown")"
		log "Docker is already installed: $docker_version"

		if [[ "${FORCE_INSTALL:-false}" != "true" ]]; then
			log "FORCE_INSTALL not set, prompting for confirmation..."
			if ! prompt_confirmation "Docker is already installed. Do you want to reinstall/upgrade?"; then
				log "Installation cancelled - Docker is already installed"
				log "To force reinstallation, run with FORCE_INSTALL=true or NON_INTERACTIVE=true"
				exit 0
			fi
		else
			log "FORCE_INSTALL=true, skipping confirmation prompt"
		fi

		return 0
	fi
	return 1
}

install_docker_ubuntu_debian() {
	log "Installing Docker on Ubuntu/Debian..."

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would install Docker via official apt repository"
		return 0
	fi

	log "Removing old Docker packages if present..."
	for pkg in docker.io docker-doc docker-compose docker-compose-v2 podman-docker containerd runc; do
		if dpkg -l | grep -q "^ii.*$pkg"; then
			log "Removing old package: $pkg"
			sudo apt-get remove -y "$pkg" 2>/dev/null || true
		fi
	done

	log "Updating package index..."
	sudo apt-get update

	log "Installing prerequisites..."
	sudo apt-get install -y \
		ca-certificates \
		curl \
		gnupg

	log "Setting up Docker GPG key..."
	sudo install -m 0755 -d /etc/apt/keyrings
	if [[ -f /etc/apt/keyrings/docker.gpg ]]; then
		sudo rm /etc/apt/keyrings/docker.gpg
	fi

	curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
	sudo chmod a+r /etc/apt/keyrings/docker.gpg

	log "Adding Docker repository..."
	local os_id
	os_id="$(. /etc/os-release && echo "$ID")"

	if [[ "$os_id" == "debian" ]]; then
		echo \
			"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian \
			$(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
			sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
	else
		echo \
			"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
			$(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
			sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
	fi

	log "Updating package index with Docker repository..."
	sudo apt-get update

	log "Installing Docker Engine, CLI, and plugins..."
	sudo apt-get install -y \
		docker-ce \
		docker-ce-cli \
		containerd.io \
		docker-buildx-plugin \
		docker-compose-plugin

	log "Docker installed successfully on Ubuntu/Debian"
}

install_docker_fedora() {
	log "Installing Docker on Fedora..."

	local pkg_mgr
	if check_command dnf; then
		pkg_mgr="dnf"
	elif check_command yum; then
		pkg_mgr="yum"
	else
		error_exit "No RPM package manager found (dnf or yum required)"
	fi

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would install Docker via official Docker $pkg_mgr repository"
		return 0
	fi

	log "Removing old Docker packages if present..."
	for pkg in docker docker-client docker-client-latest docker-common \
		docker-latest docker-latest-logrotate docker-logrotate \
		docker-selinux docker-engine-selinux docker-engine; do
		if rpm -q "$pkg" &>/dev/null; then
			log "Removing old package: $pkg"
			sudo "$pkg_mgr" remove -y "$pkg" 2>/dev/null || true
		fi
	done

	# Install docker's ready-made .repo file directly rather than calling
	# `config-manager`, whose repo verbs and plugin package differ between dnf4
	# (`--add-repo`) and dnf5 (`addrepo --from-repofile`). This mirrors how
	# manifest/packages.toml provisions the same repo.
	log "Setting up Docker repository..."
	curl -fsSL https://download.docker.com/linux/fedora/docker-ce.repo |
		sudo tee /etc/yum.repos.d/docker-ce.repo >/dev/null

	log "Installing Docker Engine, CLI, and plugins..."
	if ! sudo "$pkg_mgr" install -y \
		docker-ce \
		docker-ce-cli \
		containerd.io \
		docker-buildx-plugin \
		docker-compose-plugin; then
		error_exit "Failed to install Docker packages"
	fi

	log "Docker installed successfully on Fedora"
}

install_docker_arch() {
	log "Installing Docker on Arch Linux..."

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would install Docker via pacman"
		return 0
	fi

	log "Updating package database..."
	sudo pacman -Sy

	log "Installing Docker and related packages..."
	if ! sudo pacman -S --needed --noconfirm docker docker-compose docker-buildx; then
		error_exit "Failed to install Docker packages"
	fi

	log "Docker installed successfully on Arch Linux"
}

install_docker_macos() {
	log "Installing Docker Desktop on macOS..."

	if ! check_command brew; then
		error_exit "Homebrew is required to install Docker Desktop on macOS. Please install Homebrew first."
	fi

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would install Docker Desktop via Homebrew Cask"
		return 0
	fi

	log "Installing Docker Desktop via Homebrew..."
	if brew list --cask docker &>/dev/null; then
		log "Docker Desktop already installed via Homebrew"
		log "Upgrading to latest version..."
		brew upgrade --cask docker || log "Docker Desktop is already up to date"
	else
		brew install --cask docker
	fi

	log "Docker Desktop installed successfully on macOS"
	log "Please launch Docker Desktop from Applications to complete setup"
}

post_install_setup() {
	if [[ "$SKIP_POST_INSTALL" == "true" ]]; then
		log "Skipping post-installation setup"
		return 0
	fi

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would perform post-installation setup"
		return 0
	fi

	log "Performing post-installation setup..."

	case "$DOTFILES_OS" in
	debian | redhat | arch | linux)
		if getent group docker >/dev/null; then
			log "Docker group already exists"
		else
			log "Creating docker group..."
			sudo groupadd docker || true
		fi

		if docker_group_configured; then
			log "User $USER is already configured in docker group"
		else
			log "Adding user $USER to docker group..."
			sudo usermod -aG docker "$USER"
		fi

		if docker_group_active; then
			log "Docker group membership is active in this session"
		else
			log "Docker group membership is not active in this session"
			log "Log out and back in, or run: newgrp docker"
		fi

		if check_command systemctl; then
			log "Enabling Docker service..."
			sudo systemctl enable docker.service || log "Warning: Failed to enable docker service"
			sudo systemctl enable containerd.service || log "Warning: Failed to enable containerd service"

			log "Starting Docker service..."
			if ! sudo systemctl is-active --quiet docker.service; then
				sudo systemctl start docker.service || error_exit "Failed to start docker service"
			else
				log "Docker service is already running"
			fi
		else
			log "systemctl not available - skipping service management"
		fi
		;;
	macos)
		log "macOS detected - Docker Desktop must be started manually from Applications"
		log "Post-installation setup completed (Docker Desktop requires GUI launch)"
		;;
	*)
		log "Unknown OS - skipping post-installation setup"
		;;
	esac

	log "Post-installation setup completed"
}

verify_installation() {
	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Skipping installation verification"
		return 0
	fi

	log "Verifying Docker installation..."

	if ! check_command docker; then
		error_exit "Docker command not found after installation"
	fi

	local docker_version
	docker_version="$(docker --version 2>/dev/null || echo "unknown")"
	log "Installed: $docker_version"

	if [[ "$DOTFILES_OS" == "macos" ]]; then
		log "Docker Desktop verification:"
		log "  - Please ensure Docker Desktop is running from Applications"
		log "  - Verification will be limited until Docker Desktop starts"
		return 0
	fi

	log "Testing Docker with hello-world image..."
	if docker run --rm hello-world >/dev/null 2>&1; then
		log "Docker is working correctly!"
	elif docker_group_configured && ! docker_group_active &&
		check_command sg && sg docker -c "docker run --rm hello-world" >/dev/null 2>&1; then
		log "Docker works via the docker group, but this session predates the group change"
		log "Log out and back in, or run: newgrp docker"
	else
		log "Warning: Docker hello-world test failed"
		log "This may be normal if:"
		log "  - You just added yourself to the docker group (log out/in required)"
		log "  - Docker daemon is not running yet"
		log "  - No internet connection for pulling images"
	fi

	log "Installation verification completed"
}

show_next_steps() {
	log "Docker installation completed successfully!"
	echo
	log "Next steps:"

	case "$DOTFILES_OS" in
	debian | redhat | arch | linux)
		if docker_group_active; then
			log "1. Docker group membership active in this session"
		elif docker_group_configured; then
			log "1. Docker group membership configured but NOT active in this session"
			log "   Log out and log back in, OR run: newgrp docker"
		else
			log "1. Log out and log back in to apply docker group membership"
			log "   OR run: newgrp docker"
		fi
		log "2. Test Docker: docker run hello-world"
		log "3. Check Docker status: systemctl status docker"
		log "4. View Docker info: docker info"
		;;
	macos)
		log "1. Launch Docker Desktop from Applications"
		log "2. Wait for Docker Desktop to start"
		log "3. Test Docker: docker run hello-world"
		log "4. View Docker info: docker info"
		;;
	*)
		log "1. Verify Docker installation: docker --version"
		log "2. Test Docker: docker run hello-world"
		;;
	esac

	echo
	log "Documentation:"
	log "  - Docker docs: https://docs.docker.com/"
	log "  - Docker CLI: https://docs.docker.com/engine/reference/commandline/cli/"
	log "  - Docker Compose: https://docs.docker.com/compose/"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--version)
		DOCKER_VERSION="$2"
		shift 2
		;;
	--skip-post)
		SKIP_POST_INSTALL="true"
		shift
		;;
	--dry-run)
		DRY_RUN="true"
		shift
		;;
	--force)
		FORCE_INSTALL="true"
		shift
		;;
	--non-interactive)
		NON_INTERACTIVE="true"
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
	log "Starting Docker installation process..."
	[[ "$DRY_RUN" == "true" ]] && log "DRY-RUN MODE: No changes will be made"

	load_platform_detection
	validate_environment
	check_existing_docker || true

	case "$DOTFILES_OS" in
	debian)
		install_docker_ubuntu_debian
		;;
	macos)
		install_docker_macos
		;;
	redhat)
		install_docker_fedora
		;;
	arch)
		install_docker_arch
		;;
	*)
		error_exit "Unsupported operating system: $DOTFILES_OS"
		;;
	esac

	post_install_setup
	verify_installation
	show_next_steps

	log "Docker installation process completed successfully"
}

main "$@"
