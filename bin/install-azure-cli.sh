#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOTFILES_ROOT="$(dirname "$SCRIPT_DIR")"

# shellcheck source=../common/platform.sh
source "$DOTFILES_ROOT/common/platform.sh"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
DRY_RUN="${DRY_RUN:-false}"

print_header() {
    echo -e "${CYAN}================================${NC}"
    echo -e "${CYAN}  $1${NC}"
    echo -e "${CYAN}================================${NC}"
}

print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

show_usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Azure CLI Installer - Install Azure CLI from official Microsoft sources

OPTIONS:
    -c, --check       Check installed version without installing
    --dry-run         Show what would be done without installing
    -h, --help        Show this help message

EXAMPLES:
    $0                # Install Azure CLI
    $0 --check        # Check installed version
    $0 --dry-run      # Preview installation steps

NOTES:
    This script uses Microsoft's official installation method:
    - Debian/Ubuntu: Uses Microsoft's APT repository
    - Arch Linux: Uses AUR package
    - macOS: Uses Homebrew
    - Other: Uses pip3 fallback

EOF
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Print the installed Azure CLI version, or nothing when it cannot be parsed.
#
# The trailing `|| true` matters: the script runs under `set -euo pipefail`, so a
# grep that matches nothing would fail the pipeline and abort the script from
# inside a `version=$(get_version)` substitution. A version banner is cosmetic —
# it must never fail the installer.
get_version() {
    if command_exists az; then
        az version --output json 2>/dev/null | grep '"azure-cli":' | awk -F'"' '{print $4}' || true
    else
        echo "not installed"
    fi
}

check_azure_cli() {
    print_header "Azure CLI Status"
    echo
    
    if command_exists az; then
        local version
        version=$(get_version || true)
        print_success "Azure CLI: ${version:-unknown}"
    else
        print_warning "Azure CLI: not installed"
    fi
}

install_azure_cli_debian() {
    print_status "Installing Azure CLI on Debian/Ubuntu..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would run Microsoft's official installation script"
        print_status "Script: https://aka.ms/InstallAzureCLIDeb"
        return 0
    fi
    
    # Use Microsoft's official installation script
    print_status "Downloading and running Microsoft's official installation script..."
    curl -fsSL https://aka.ms/InstallAzureCLIDeb | sudo bash
}

install_azure_cli_arch() {
    print_status "Installing Azure CLI on Arch Linux..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would install: azure-cli (from AUR or community repo)"
        return 0
    fi
    
    # Check if yay or paru is available for AUR
    if command_exists yay; then
        print_status "Installing via yay..."
        yay -S --noconfirm azure-cli
    elif command_exists paru; then
        print_status "Installing via paru..."
        paru -S --noconfirm azure-cli
    elif command_exists pacman; then
        print_status "Installing via pacman (community repo)..."
        sudo pacman -S --noconfirm azure-cli
    else
        print_error "No suitable package manager found for Arch Linux"
        print_error "Please install yay, paru, or ensure azure-cli is in community repo"
        return 1
    fi
}

install_azure_cli_rpm() {
    print_status "Installing Azure CLI on RHEL/Fedora/CentOS..."

    local mgr
    if command_exists dnf; then
        mgr="dnf"
    elif command_exists yum; then
        mgr="yum"
    else
        print_error "Neither dnf nor yum found"
        return 1
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would install azure-cli via $mgr from the distro repositories"
        print_status "Would fall back to the Microsoft repository if unavailable"
        return 0
    fi

    # Prefer the distro's own azure-cli: Fedora ships it as a noarch package, so
    # it installs on every architecture. Microsoft's packages-microsoft-prod repo
    # targets RHEL and publishes no aarch64 azure-cli, which is why it is the
    # fallback rather than the first choice.
    print_status "Installing Azure CLI via $mgr..."
    if sudo "$mgr" install -y azure-cli; then
        return 0
    fi

    print_warning "azure-cli not available from the distro repositories"
    print_status "Importing Microsoft GPG key..."
    sudo rpm --import https://packages.microsoft.com/keys/microsoft.asc

    print_status "Adding Microsoft repository..."
    sudo "$mgr" install -y https://packages.microsoft.com/config/rhel/9.0/packages-microsoft-prod.rpm

    print_status "Installing Azure CLI via $mgr..."
    sudo "$mgr" install -y azure-cli
}

install_azure_cli_macos() {
    print_status "Installing Azure CLI on macOS..."
    
    if ! command_exists brew; then
        print_error "Homebrew is required for macOS installation"
        print_error "Install Homebrew from https://brew.sh"
        return 1
    fi
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would run: brew install azure-cli"
        return 0
    fi
    
    print_status "Installing via Homebrew..."
    brew install azure-cli
}

install_azure_cli_pip() {
    print_status "Installing Azure CLI via pip3 (fallback method)..."
    
    if ! command_exists pip3; then
        print_error "pip3 is required for installation"
        print_error "Please install Python 3 and pip3 first"
        return 1
    fi
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would run: pip3 install --user azure-cli"
        return 0
    fi
    
    print_status "Installing via pip3..."
    pip3 install --user azure-cli
    
    # Check if ~/.local/bin is in PATH
    if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
        print_warning "$HOME/.local/bin is not in PATH"
        print_warning "Add this to your shell config: export PATH=\"\$HOME/.local/bin:\$PATH\""
    fi
}

install_azure_cli() {
    print_status "Installing Azure CLI..."
    
    if command_exists az; then
        print_warning "Azure CLI is already installed: $(get_version)"
        return 0
    fi
    
    if is_macos; then
        install_azure_cli_macos
    elif is_linux; then
        # Dispatch on the detected distro FAMILY, not the raw /etc/os-release ID:
        # a derivative such as Fedora Asahi Remix reports ID=fedora-asahi-remix,
        # which no raw-ID case arm matches and which would fall all the way
        # through to the pip3 path. detect_os already folds derivatives into
        # debian/redhat/arch/suse via ID_LIKE.
        local os_family="${DOTFILES_OS:-$(detect_os)}"

        case "$os_family" in
            debian)
                install_azure_cli_debian
                ;;
            arch)
                install_azure_cli_arch
                ;;
            redhat)
                install_azure_cli_rpm
                ;;
            *)
                print_warning "Unsupported distribution: $(get_distro_name) (family: $os_family)"
                print_status "Falling back to pip3 installation..."
                install_azure_cli_pip
                ;;
        esac
    else
        print_warning "Unsupported platform: $DOTFILES_OS"
        print_status "Falling back to pip3 installation..."
        install_azure_cli_pip
    fi
    
    print_success "Azure CLI installed successfully"
}

main() {
    local check_only=false
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            -c|--check)
                check_only=true
                shift
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            -h|--help)
                show_usage
                exit 0
                ;;
            *)
                print_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done
    
    if [[ "$check_only" == "true" ]]; then
        check_azure_cli
        exit 0
    fi
    
    print_header "Azure CLI Installer"
    print_status "Platform: $DOTFILES_OS ($DOTFILES_ARCH)"
    [[ "$DRY_RUN" == "true" ]] && print_warning "DRY RUN MODE - No actual changes will be made"
    echo
    
    install_azure_cli
    
    echo
    print_header "Installation Complete"
    check_azure_cli
}

main "$@"
