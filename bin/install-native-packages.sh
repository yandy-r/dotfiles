#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[NATIVE-PKG]"

# Configuration
DRY_RUN="${DRY_RUN:-false}"
SKIP_CATEGORY="${SKIP_CATEGORY:-}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
NON_INTERACTIVE="${NON_INTERACTIVE:-false}"

# Detect OS
detect_os() {
  if [[ -f /etc/os-release ]]; then
    # shellcheck disable=SC1091
    source /etc/os-release
    OS_ID="${ID}"
    OS_VERSION_ID="${VERSION_ID:-unknown}"
    OS_NAME="${NAME:-Unknown Linux}"
  elif [[ -f /etc/debian_version ]]; then
    OS_ID="debian"
    OS_VERSION_ID="$(cat /etc/debian_version)"
    OS_NAME="Debian"
  elif [[ -f /etc/arch-release ]]; then
    OS_ID="arch"
    OS_VERSION_ID="rolling"
    OS_NAME="Arch Linux"
  else
    return 1
  fi
  
  case "$OS_ID" in
    ubuntu|debian)
      PKG_MANAGER="apt"
      ;;
    arch|cachyos|manjaro|endeavouros)
      PKG_MANAGER="pacman"
      ;;
    *)
      # Fallback: use ID_LIKE for unknown derivatives
      case "${ID_LIKE:-}" in
        *debian*|*ubuntu*) PKG_MANAGER="apt" ;;
        *arch*) PKG_MANAGER="pacman" ;;
        *) return 1 ;;
      esac
      ;;
  esac
  
  return 0
}

usage() {
  cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install development tools and utilities using native package managers.

OPTIONS:
    --dry-run           Show what would be installed without actually installing
    --skip-category     Skip a specific category (e.g., "development", "network")
    --force             Force installation even if packages appear to be installed
    --non-interactive   Run without prompting for confirmation
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    DRY_RUN            Enable dry-run mode (true/false)
    SKIP_CATEGORY      Comma-separated list of categories to skip
    FORCE_INSTALL      Force installation (true/false)
    NON_INTERACTIVE    Run non-interactively (true/false)

SUPPORTED DISTRIBUTIONS:
    - Debian 11+ (Bullseye and newer)
    - Ubuntu 22.04+ (Jammy and newer)
    - Arch Linux (rolling)

EXAMPLES:
    $SCRIPT_NAME --dry-run
    $SCRIPT_NAME --skip-category development
    DRY_RUN=true $SCRIPT_NAME
    NON_INTERACTIVE=true $SCRIPT_NAME

PACKAGE CATEGORIES:
    - core: Essential CLI tools (bash, coreutils, curl, wget)
    - development: Programming languages and build tools
    - git: Git and related tools
    - network: Network utilities and diagnostics
    - system: System monitoring and management
    - editors: Text editors and formatters
    - containers: Container and cloud tools
    - multimedia: Media processing tools
    - shell: Shell enhancements (zsh, fzf, etc.)
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

# Check if running in interactive mode
is_interactive() {
  [[ -t 0 ]] && [[ -t 1 ]]
}

# Prompt user for confirmation
prompt_confirmation() {
  local prompt_message="$1"
  local response

  # Auto-approve in non-interactive mode
  if [[ "${NON_INTERACTIVE}" == "true" ]] || [[ "${FORCE_INSTALL}" == "true" ]]; then
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

# Check if a package is installed
is_installed() {
  local package="$1"
  
  case "$PKG_MANAGER" in
    apt)
      dpkg -l "$package" 2>/dev/null | grep -q "^ii"
      ;;
    pacman)
      pacman -Qi "$package" &>/dev/null
      ;;
    *)
      return 1
      ;;
  esac
}

# Install a single package
install_package() {
  local package="$1"
  local category="${2:-general}"
  
  # Check if category should be skipped
  if [[ -n "$SKIP_CATEGORY" ]] && [[ "$SKIP_CATEGORY" == *"$category"* ]]; then
    log "Skipping $package (category: $category)"
    return 0
  fi
  
  # Skip if already installed (unless forced)
  if [[ "$FORCE_INSTALL" != "true" ]] && is_installed "$package"; then
    log "Package $package already installed, skipping"
    return 0
  fi
  
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: Would install $package (category: $category)"
    return 0
  fi
  
  log "Installing $package (category: $category)"
  
  case "$PKG_MANAGER" in
    apt)
      if ! sudo apt-get install -y "$package"; then
        log "WARNING: Failed to install $package"
        return 1
      fi
      ;;
    pacman)
      if ! sudo pacman -S --noconfirm "$package"; then
        log "WARNING: Failed to install $package"
        return 1
      fi
      ;;
  esac
  
  log "Successfully installed $package"
  return 0
}

# Install packages in batch
install_packages() {
  local category="$1"
  shift
  local packages=("$@")
  
  log "Installing $category packages (${#packages[@]} total)..."
  
  local failed_packages=()
  local installed_count=0
  
  for package in "${packages[@]}"; do
    if install_package "$package" "$category"; then
      installed_count=$((installed_count + 1))
    else
      failed_packages+=("$package")
    fi
  done
  
  log "Category $category: $installed_count/${#packages[@]} packages installed successfully"
  
  if [[ ${#failed_packages[@]} -gt 0 ]]; then
    log "WARNING: Failed packages in $category: ${failed_packages[*]}"
  fi
}

# Update package cache
update_package_cache() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: Would update package cache"
    return 0
  fi
  
  log "Updating package cache..."
  
  case "$PKG_MANAGER" in
    apt)
      if ! sudo apt-get update; then
        error_exit "Failed to update apt cache"
      fi
      ;;
    pacman)
      if ! sudo pacman -Sy; then
        error_exit "Failed to update pacman database"
      fi
      ;;
  esac
  
  log "Package cache updated successfully"
}

# Create binary symlinks for Debian-specific packages
create_debian_symlinks() {
  if [[ "$PKG_MANAGER" != "apt" ]]; then
    return 0
  fi
  
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: Would create Debian-specific symlinks"
    return 0
  fi
  
  log "Creating Debian-specific binary symlinks..."
  
  # bat: batcat -> bat
  if command -v batcat &>/dev/null && ! command -v bat &>/dev/null; then
    log "Creating symlink: batcat -> bat"
    sudo ln -sf "$(command -v batcat)" /usr/local/bin/bat
  fi
  
  # fd: fdfind -> fd
  if command -v fdfind &>/dev/null && ! command -v fd &>/dev/null; then
    log "Creating symlink: fdfind -> fd"
    sudo ln -sf "$(command -v fdfind)" /usr/local/bin/fd
  fi
  
  log "Symlinks created successfully"
}

# Install core packages
install_core_packages() {
  log "=== Installing Core Packages ==="
  
  local packages=()
  
  case "$PKG_MANAGER" in
    apt)
      packages=(
        bash
        coreutils
        curl
        wget
        ca-certificates
        gnupg
        lsb-release
        software-properties-common
        apt-transport-https
        build-essential
        file
      )
      ;;
    pacman)
      packages=(
        bash
        coreutils
        curl
        wget
        ca-certificates
        gnupg
        base-devel
        file
      )
      ;;
  esac
  
  install_packages "core" "${packages[@]}"
}

# Install development tools
install_development_packages() {
  log "=== Installing Development Packages ==="
  
  local packages=()
  
  case "$PKG_MANAGER" in
    apt)
      packages=(
        gcc
        g++
        make
        cmake
        clang
        clang-format
        pkg-config
        python3
        python3-pip
        pipx
        python3-venv
        ruby
        ruby-lsp
        lua5.4
        luarocks
        golang-go
        protobuf-compiler
      )
      ;;
    pacman)
      packages=(
        gcc
        make
        cmake
        clang
        pkgconf
        python
        python-pip
        python-pipx
        ruby
        ruby-lsp
        lua
        luarocks
        go
        protobuf
      )
      ;;
  esac
  
  install_packages "development" "${packages[@]}"
}

# Install Git and related tools
install_git_packages() {
  log "=== Installing Git Packages ==="
  
  local packages=()
  
  case "$PKG_MANAGER" in
    apt)
      packages=(
        git
        git-lfs
        tig
        libgit2-dev
        gh
        lazygit
      )
      # git-cliff and git-delta are not in standard repos (installed via install-modern-cli.sh)
      ;;
    pacman)
      packages=(
        git
        git-cliff
        git-lfs
        git-delta
        github-cli
        lazygit
        libgit2
      )
      ;;
  esac
  
  install_packages "git" "${packages[@]}"
}

# Install network utilities
install_network_packages() {
  log "=== Installing Network Packages ==="
  
  local packages=()
  
  case "$PKG_MANAGER" in
    apt)
      packages=(
        net-tools
        iputils-ping
        dnsutils
        netcat-openbsd
        nmap
        tcpdump
        mtr
        fping
        sipcalc
        httpie
      )
      ;;
    pacman)
      packages=(
        net-tools
        iputils
        bind
        openbsd-netcat
        nmap
        tcpdump
        mtr
        fping
        httpie
      )
      ;;
  esac
  
  install_packages "network" "${packages[@]}"
}

# Install system utilities
install_system_packages() {
  log "=== Installing System Packages ==="
  
  local packages=()
  
  case "$PKG_MANAGER" in
    apt)
      packages=(
        tmux
        screen
        tree
        jq
        yq
        fzf
        bat
        fd-find
        ripgrep
        eza
        htop
        btop
        zoxide
        procps
        strace
        gawk
      )
      # bottom (btm) not in standard Debian repos
      ;;
    pacman)
      packages=(
        tmux
        screen
        tree
        jq
        yq
        fzf
        bat
        fd
        ripgrep
        eza
        htop
        bottom
        btop
        procps-ng
        strace
        gawk
      )
      ;;
  esac
  
  install_packages "system" "${packages[@]}"
}

# Install editor and formatting tools
install_editor_packages() {
  log "=== Installing Editor & Formatting Packages ==="
  
  local packages=()
  
  case "$PKG_MANAGER" in
    apt)
      packages=(
        vim
        neovim
        pandoc
        shellcheck
      )
      # prettier not in standard repos
      ;;
    pacman)
      packages=(
        vim
        neovim
        pandoc
        shellcheck
        prettier
      )
      ;;
  esac
  
  install_packages "editors" "${packages[@]}"
}

# Install container and cloud tools
install_container_packages() {
  log "=== Installing Container & Cloud Packages ==="
  
  local packages=()
  
  case "$PKG_MANAGER" in
    apt)
      packages=(
        ansible
        lazydocker
      )
      # Docker, AWS CLI, Azure CLI require special installation methods
      ;;
    pacman)
      packages=(
        ansible
        ansible-language-server
        docker
        docker-compose
        lazydocker
      )
      ;;
  esac
  
  install_packages "containers" "${packages[@]}"
}

# Install multimedia tools
install_multimedia_packages() {
  log "=== Installing Multimedia Packages ==="
  
  local packages=()
  
  case "$PKG_MANAGER" in
    apt)
      packages=(
        ffmpeg
        libfreetype6
        libjpeg-turbo8
        libpng16-16
        libimage-exiftool-perl
        rtmpdump
      )
      ;;
    pacman)
      packages=(
        ffmpeg
        freetype2
        libjpeg-turbo
        libpng
        perl-image-exiftool
        rtmpdump
      )
      ;;
  esac
  
  install_packages "multimedia" "${packages[@]}"
}

# Install shell enhancements
install_shell_packages() {
  log "=== Installing Shell Enhancement Packages ==="
  
  local packages=()
  
  case "$PKG_MANAGER" in
    apt)
      packages=(
        zsh
        zsh-autosuggestions
        zsh-syntax-highlighting
        direnv
        autojump
        asciinema
      )
      ;;
    pacman)
      packages=(
        zsh
        zsh-autosuggestions
        zsh-syntax-highlighting
        zsh-completions
        direnv
        asciinema
      )
      ;;
  esac
  
  install_packages "shell" "${packages[@]}"
}

# Install additional CLI tools (various categories)
install_additional_packages() {
  log "=== Installing Additional CLI Tools ==="
  
  local packages=()
  
  case "$PKG_MANAGER" in
    apt)
      packages=(
        ack
        speedtest-cli
      )
      # bitwarden-cli, xh not in standard repos
      ;;
    pacman)
      # bitwarden-cli omitted: can conflict with nodejs-lts (e.g. CachyOS). Use: npm install -g @bitwarden/cli
      packages=(
        speedtest-cli
        xh
        thefuck
      )
      ;;
  esac
  
  install_packages "additional" "${packages[@]}"
}

# Validate environment
validate_environment() {
  log "Validating environment..."
  
  # Check if running as root
  if [[ $EUID -eq 0 ]]; then
    error_exit "This script should not be run as root. It will use sudo when needed."
  fi
  
  # Check if sudo is available
  if ! command -v sudo &>/dev/null; then
    error_exit "sudo is required but not found"
  fi
  
  # Check sudo access (skip in dry-run mode)
  if [[ "$DRY_RUN" != "true" ]]; then
    if ! sudo -v; then
      error_exit "Unable to obtain sudo privileges"
    fi
  fi
  
  # Detect OS and package manager
  if ! detect_os; then
    error_exit "Unsupported operating system. Supported: Debian, Ubuntu, Arch Linux"
  fi
  
  log "Detected: $OS_NAME ($OS_ID $OS_VERSION_ID) with $PKG_MANAGER"
  
  # Check available disk space (require at least 2GB)
  local available_space
  available_space=$(df /usr | tail -1 | awk '{print $4}')
  if [[ $available_space -lt 2097152 ]]; then # 2GB in KB
    error_exit "Insufficient disk space. At least 2GB required in /usr"
  fi
  
  # Check network connectivity (unless dry-run)
  if [[ "$DRY_RUN" != "true" ]]; then
    local test_url
    case "$PKG_MANAGER" in
      apt) test_url="http://archive.ubuntu.com" ;;
      pacman) test_url="https://archlinux.org" ;;
    esac
    
    if ! curl -s --connect-timeout 5 "$test_url" >/dev/null; then
      error_exit "No internet connectivity or package repository unreachable"
    fi
  fi
  
  log "Environment validation passed"
}

# Show installation summary
show_summary() {
  log "=== Installation Summary ==="
  log "OS: $OS_NAME ($OS_ID $OS_VERSION_ID)"
  log "Package Manager: $PKG_MANAGER"
  log "Dry Run: $DRY_RUN"
  log "Force Install: $FORCE_INSTALL"
  log "Non-Interactive: $NON_INTERACTIVE"
  [[ -n "$SKIP_CATEGORY" ]] && log "Skipped Categories: $SKIP_CATEGORY"
  log ""
}

# Main installation flow
install_all_packages() {
  install_core_packages
  install_development_packages
  install_git_packages
  install_network_packages
  install_system_packages
  install_editor_packages
  install_container_packages
  install_multimedia_packages
  install_shell_packages
  install_additional_packages
  
  # Create Debian-specific symlinks if needed
  create_debian_symlinks
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
  --dry-run)
    DRY_RUN="true"
    shift
    ;;
  --skip-category)
    SKIP_CATEGORY="$2"
    shift 2
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
  log "Starting native package installation process..."
  [[ "$DRY_RUN" == "true" ]] && log "DRY-RUN MODE: No packages will be installed"
  
  validate_environment
  show_summary
  
  # Confirm before proceeding (unless non-interactive or dry-run)
  if [[ "$DRY_RUN" != "true" ]] && [[ "$NON_INTERACTIVE" != "true" ]]; then
    if ! prompt_confirmation "Proceed with package installation?"; then
      log "Installation cancelled by user"
      exit 0
    fi
  fi
  
  update_package_cache
  install_all_packages
  
  log "=== Installation Complete ==="
  log "Native package installation completed successfully"
  log ""
  log "Note: Some packages may require additional setup:"
  log "  - Docker: Add user to docker group (usermod -aG docker \$USER)"
  log "  - Zsh: Set as default shell (chsh -s \$(which zsh))"
  log "  - Direnv: Add hook to shell rc file"
  log "  - FZF: Run fzf installation script for shell integration"
  log ""
  log "Packages not available in native repos (install separately):"
  log "  - AWS CLI (use official installer)"
  log "  - Azure CLI (use official installer)"
  log "  - GitHub CLI (gh) - may need external repo"
  log "  - lazygit (Debian/Ubuntu only)"
  log "  - git-delta (Debian/Ubuntu only)"
  log "  - bottom/btm (Debian/Ubuntu only)"
  log "  - xh (Debian/Ubuntu only)"
  log "  - bitwarden-cli (Debian/Ubuntu only)"
  log ""
  log "Debian/Ubuntu users: Consider using Homebrew or snap for missing packages"
  log "Arch users: Consider using AUR for additional packages"
}

main "$@"
