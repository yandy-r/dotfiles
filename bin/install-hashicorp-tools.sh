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
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
DRY_RUN="${DRY_RUN:-false}"

HASHICORP_TOOLS=(
    "terraform:Infrastructure as Code tool"
    "tflint:Terraform linter"
)

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
Usage: $0 [OPTIONS] [TOOLS...]

HashiCorp Tools Installer - Install HashiCorp tools from official sources

TOOLS:
    terraform    Infrastructure as Code tool
    tflint       Terraform linter
    all          Install all tools (default)

OPTIONS:
    -c, --check       Check installed versions without installing
    -d, --dir DIR     Installation directory (default: /usr/local/bin)
    --dry-run         Show what would be done without installing
    -h, --help        Show this help message

EXAMPLES:
    $0                     # Install all HashiCorp tools
    $0 terraform           # Install only Terraform
    $0 --check             # Check installed versions
    $0 --dry-run           # Preview installation steps

EOF
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Name the RPM-family package manager to drive: dnf where present (Fedora, RHEL 8+),
# yum on the older releases that still ship only yum.
rpm_pkg_mgr() {
    if command_exists dnf; then
        echo "dnf"
    else
        echo "yum"
    fi
}

# Print a tool's version, or nothing when it cannot be determined.
#
# Every branch ends in `|| true`: the script runs under `set -euo pipefail`, so a
# grep that matches nothing would otherwise fail the whole pipeline and abort the
# script from inside a `version=$(get_version ...)` substitution. Version banners
# are cosmetic — an unparseable one must never fail the installer.
get_version() {
    local tool="$1"
    case "$tool" in
        terraform)
            if command_exists terraform; then
                # `terraform version -json` pretty-prints with a space after the
                # colon ("terraform_version": "1.16.0"), so the pattern must
                # tolerate it.
                terraform version -json 2>/dev/null |
                    grep -o '"terraform_version":[[:space:]]*"[^"]*"' |
                    cut -d'"' -f4 || true
            fi
            ;;
        tflint)
            if command_exists tflint; then
                tflint --version 2>&1 | grep -o 'version [0-9.]*' | awk '{print $2}' || true
            fi
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

check_tools() {
    print_header "HashiCorp Tools Status"
    echo
    
    for tool_entry in "${HASHICORP_TOOLS[@]}"; do
        IFS=: read -r tool desc <<< "$tool_entry"
        if command_exists "$tool"; then
            local version
            version=$(get_version "$tool" || true)
            print_success "$tool: ${version:-unknown} - $desc"
        else
            print_warning "$tool: not installed - $desc"
        fi
    done
}

add_hashicorp_repo_debian() {
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would add HashiCorp GPG key"
        print_status "Would add HashiCorp repository"
        return 0
    fi
    
    # Check if repository already exists
    if [[ -f /usr/share/keyrings/hashicorp-archive-keyring.gpg ]]; then
        print_status "HashiCorp repository already configured"
        return 0
    fi
    
    print_status "Adding HashiCorp GPG key and repository..."
    
    # Install required packages
    sudo apt-get update && sudo apt-get install -y gnupg software-properties-common
    
    # Add GPG key
    wget -O- https://apt.releases.hashicorp.com/gpg | \
        gpg --dearmor | \
        sudo tee /usr/share/keyrings/hashicorp-archive-keyring.gpg > /dev/null
    
    # Verify key fingerprint
    gpg --no-default-keyring \
        --keyring /usr/share/keyrings/hashicorp-archive-keyring.gpg \
        --fingerprint
    
    # Add repository
    echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | \
        sudo tee /etc/apt/sources.list.d/hashicorp.list
    
    sudo apt-get update
}

add_hashicorp_repo_rpm() {
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would add HashiCorp repository"
        return 0
    fi
    
    # Check if repository already exists
    if [[ -f /etc/yum.repos.d/hashicorp.repo ]]; then
        print_status "HashiCorp repository already configured"
        return 0
    fi
    
    print_status "Adding HashiCorp repository..."

    # Write the .repo file directly rather than calling `yum-config-manager`
    # (dnf4-only; dnf5 renamed the verb to `dnf config-manager addrepo` and drops
    # the yum-utils shim), mirroring how manifest/packages.toml provisions the
    # docker-ce dnf repo.
    curl -fsSL https://rpm.releases.hashicorp.com/fedora/hashicorp.repo |
        sudo tee /etc/yum.repos.d/hashicorp.repo > /dev/null
}

install_terraform() {
    print_status "Installing Terraform..."
    
    if command_exists terraform; then
        print_warning "Terraform is already installed: $(get_version terraform)"
        return 0
    fi
    
    if is_macos; then
        if ! command_exists brew; then
            print_error "Homebrew is required for macOS installation"
            return 1
        fi
        
        if [[ "$DRY_RUN" == "true" ]]; then
            print_status "Would run: brew tap hashicorp/tap"
            print_status "Would run: brew install hashicorp/tap/terraform"
        else
            brew tap hashicorp/tap
            brew install hashicorp/tap/terraform
        fi
    elif is_linux; then
        # Dispatch on the detected distro FAMILY, not the raw /etc/os-release ID:
        # a derivative such as Fedora Asahi Remix reports ID=fedora-asahi-remix,
        # which no raw-ID case arm matches and which would silently fall through
        # to the binary path. detect_os already folds derivatives into
        # debian/redhat/arch/suse via ID_LIKE.
        case "${DOTFILES_OS:-$(detect_os)}" in
            debian)
                add_hashicorp_repo_debian
                if [[ "$DRY_RUN" == "true" ]]; then
                    print_status "Would run: sudo apt-get install terraform"
                else
                    sudo apt-get install -y terraform
                fi
                ;;
            redhat)
                add_hashicorp_repo_rpm
                if [[ "$DRY_RUN" == "true" ]]; then
                    print_status "Would run: sudo dnf install terraform"
                elif ! sudo "$(rpm_pkg_mgr)" install -y terraform; then
                    # HashiCorp's RPM repo does not publish every arch (notably
                    # aarch64 on Fedora); fall back to the release tarball rather
                    # than failing the whole tool.
                    print_warning "terraform not available from the HashiCorp RPM repo; falling back to the binary release"
                    install_terraform_binary
                fi
                ;;
            arch)
                if [[ "$DRY_RUN" == "true" ]]; then
                    print_status "Would run: sudo pacman -S terraform"
                else
                    sudo pacman -S --noconfirm terraform
                fi
                ;;
            *)
                print_warning "Installing from binary release..."
                install_terraform_binary
                ;;
        esac
    fi
    
    print_success "Terraform installed"
}

install_terraform_binary() {
    local version arch os url temp_dir
    
    # Get latest version from HashiCorp releases
    version=$(curl -fsSL https://api.releases.hashicorp.com/v1/releases/terraform/latest | grep -o '"version":"[^"]*"' | head -1 | cut -d'"' -f4)
    
    if [[ -z "$version" ]]; then
        print_error "Failed to determine latest Terraform version"
        return 1
    fi
    
    print_status "Latest version: $version"
    
    if is_macos; then
        os="darwin"
    else
        os="linux"
    fi
    
    arch=$(detect_arch)
    if [[ "$arch" == "x86_64" ]]; then
        arch="amd64"
    elif [[ "$arch" == "aarch64" ]]; then
        arch="arm64"
    fi
    
    url="https://releases.hashicorp.com/terraform/${version}/terraform_${version}_${os}_${arch}.zip"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would download: $url"
        print_status "Would install to: $INSTALL_DIR/terraform"
        return 0
    fi
    
    temp_dir=$(mktemp -d)
    
    print_status "Downloading Terraform..."
    curl -fsSL "$url" -o "$temp_dir/terraform.zip"
    
    print_status "Extracting..."
    unzip -q "$temp_dir/terraform.zip" -d "$temp_dir"
    
    print_status "Installing to $INSTALL_DIR/terraform..."
    if [[ -w "$INSTALL_DIR" ]]; then
        mv "$temp_dir/terraform" "$INSTALL_DIR/terraform"
    else
        sudo mv "$temp_dir/terraform" "$INSTALL_DIR/terraform"
    fi
    
    rm -rf "$temp_dir"
}

install_tflint() {
    print_status "Installing TFLint..."
    
    if command_exists tflint; then
        print_warning "TFLint is already installed: $(get_version tflint)"
        return 0
    fi
    
    local version arch os url temp_dir
    
    # Get latest version from GitHub API
    version=$(curl -fsSL https://api.github.com/repos/terraform-linters/tflint/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
    
    if [[ -z "$version" ]]; then
        print_error "Failed to determine latest TFLint version"
        return 1
    fi
    
    print_status "Latest version: $version"
    
    if is_macos; then
        os="darwin"
    else
        os="linux"
    fi
    
    arch=$(detect_arch)
    if [[ "$arch" == "x86_64" ]]; then
        arch="amd64"
    elif [[ "$arch" == "aarch64" ]]; then
        arch="arm64"
    fi
    
    url="https://github.com/terraform-linters/tflint/releases/download/v${version}/tflint_${os}_${arch}.zip"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would download: $url"
        print_status "Would install to: $INSTALL_DIR/tflint"
        return 0
    fi
    
    temp_dir=$(mktemp -d)
    
    print_status "Downloading TFLint from GitHub..."
    if ! curl -fsSL "$url" -o "$temp_dir/tflint.zip"; then
        print_error "Failed to download TFLint from $url"
        rm -rf "$temp_dir"
        return 1
    fi
    
    print_status "Extracting..."
    unzip -q "$temp_dir/tflint.zip" -d "$temp_dir"
    
    print_status "Installing to $INSTALL_DIR/tflint..."
    if [[ -w "$INSTALL_DIR" ]]; then
        mv "$temp_dir/tflint" "$INSTALL_DIR/tflint"
    else
        sudo mv "$temp_dir/tflint" "$INSTALL_DIR/tflint"
    fi
    
    rm -rf "$temp_dir"
    print_success "TFLint installed"
}

install_tool() {
    local tool="$1"
    
    case "$tool" in
        terraform) install_terraform ;;
        tflint) install_tflint ;;
        *)
            print_error "Unknown tool: $tool"
            return 1
            ;;
    esac
}

main() {
    local tools=()
    local check_only=false
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            -c|--check)
                check_only=true
                shift
                ;;
            -d|--dir)
                INSTALL_DIR="$2"
                shift 2
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            -h|--help)
                show_usage
                exit 0
                ;;
            all)
                for tool_entry in "${HASHICORP_TOOLS[@]}"; do
                    IFS=: read -r tool _ <<< "$tool_entry"
                    tools+=("$tool")
                done
                shift
                ;;
            terraform|tflint)
                tools+=("$1")
                shift
                ;;
            *)
                print_error "Unknown option or tool: $1"
                show_usage
                exit 1
                ;;
        esac
    done
    
    if [[ "$check_only" == "true" ]]; then
        check_tools
        exit 0
    fi
    
    if [[ ${#tools[@]} -eq 0 ]]; then
        for tool_entry in "${HASHICORP_TOOLS[@]}"; do
            IFS=: read -r tool _ <<< "$tool_entry"
            tools+=("$tool")
        done
    fi
    
    print_header "HashiCorp Tools Installer"
    print_status "Platform: $DOTFILES_OS ($DOTFILES_ARCH)"
    print_status "Install directory: $INSTALL_DIR"
    [[ "$DRY_RUN" == "true" ]] && print_warning "DRY RUN MODE - No actual changes will be made"
    echo
    
    print_status "Installing ${#tools[@]} tools..."
    echo
    
    for tool in "${tools[@]}"; do
        install_tool "$tool"
    done
    
    echo
    print_header "Installation Complete"
    check_tools
}

main "$@"
