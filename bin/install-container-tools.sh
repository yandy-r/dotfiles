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

CONTAINER_TOOLS=(
    "skaffold:Kubernetes development tool"
    "hadolint:Dockerfile linter"
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

Container/K8s Tools Installer - Install container and Kubernetes development tools

TOOLS:
    skaffold     Kubernetes development workflow tool
    hadolint     Dockerfile linter
    all          Install all tools (default)

OPTIONS:
    -c, --check       Check installed versions without installing
    -d, --dir DIR     Installation directory (default: /usr/local/bin)
    --dry-run         Show what would be done without installing
    -h, --help        Show this help message

EXAMPLES:
    $0                     # Install all container tools
    $0 skaffold hadolint   # Install specific tools
    $0 --check             # Check installed versions
    $0 --dry-run           # Preview installation steps

EOF
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
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
        skaffold)
            if command_exists skaffold; then
                skaffold version 2>&1 | grep -o 'v[0-9.]*' | head -1 | tr -d 'v' || true
            fi
            ;;
        hadolint)
            if command_exists hadolint; then
                # hadolint --version prints "Haskell Dockerfile Linter 2.15.1" —
                # the product name, not "Hadolint" — so match the trailing
                # version number instead of a fixed banner prefix.
                hadolint --version 2>&1 | grep -oE '[0-9]+(\.[0-9]+)+' | head -1 || true
            fi
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

check_tools() {
    print_header "Container/K8s Tools Status"
    echo
    
    for tool_entry in "${CONTAINER_TOOLS[@]}"; do
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

install_skaffold() {
    print_status "Installing Skaffold..."
    
    if command_exists skaffold; then
        print_warning "Skaffold is already installed: $(get_version skaffold)"
        return 0
    fi
    
    if is_macos; then
        if ! command_exists brew; then
            print_error "Homebrew is required for macOS installation"
            return 1
        fi
        
        if [[ "$DRY_RUN" == "true" ]]; then
            print_status "Would run: brew install skaffold"
        else
            brew install skaffold
        fi
    elif is_linux; then
        # Use official Skaffold installation script
        if [[ "$DRY_RUN" == "true" ]]; then
            print_status "Would download and run official Skaffold installer"
            print_status "Script: https://storage.googleapis.com/skaffold/releases/latest/skaffold-linux-$(detect_arch)"
        else
            local arch
            arch=$(detect_arch)
            if [[ "$arch" == "x86_64" ]]; then
                arch="amd64"
            elif [[ "$arch" == "aarch64" ]]; then
                arch="arm64"
            fi
            
            local url="https://storage.googleapis.com/skaffold/releases/latest/skaffold-linux-${arch}"
            
            print_status "Downloading Skaffold..."
            local temp_file
            temp_file=$(mktemp)
            
            if ! curl -fsSL "$url" -o "$temp_file"; then
                print_error "Failed to download Skaffold from $url"
                rm -f "$temp_file"
                return 1
            fi
            
            chmod +x "$temp_file"
            
            print_status "Installing to $INSTALL_DIR/skaffold..."
            if [[ -w "$INSTALL_DIR" ]]; then
                mv "$temp_file" "$INSTALL_DIR/skaffold"
            else
                sudo mv "$temp_file" "$INSTALL_DIR/skaffold"
            fi
        fi
    fi
    
    print_success "Skaffold installed"
}

install_hadolint() {
    print_status "Installing Hadolint..."
    
    if command_exists hadolint; then
        print_warning "Hadolint is already installed: $(get_version hadolint)"
        return 0
    fi
    
    local version arch os url temp_file
    
    # Get latest version from GitHub API
    version=$(curl -fsSL https://api.github.com/repos/hadolint/hadolint/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
    
    if [[ -z "$version" ]]; then
        print_error "Failed to determine latest Hadolint version"
        return 1
    fi
    
    print_status "Latest version: $version"
    
    if is_macos; then
        os="macos"
    else
        os="linux"
    fi
    
    arch=$(detect_arch)
    if [[ "$arch" == "aarch64" ]]; then
        arch="arm64"
    fi
    
    url="https://github.com/hadolint/hadolint/releases/download/v${version}/hadolint-${os}-${arch}"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would download: $url"
        print_status "Would install to: $INSTALL_DIR/hadolint"
        return 0
    fi
    
    temp_file=$(mktemp)
    
    print_status "Downloading Hadolint from GitHub..."
    if ! curl -fsSL "$url" -o "$temp_file"; then
        print_error "Failed to download Hadolint from $url"
        rm -f "$temp_file"
        return 1
    fi
    
    chmod +x "$temp_file"
    
    # Verify the binary works
    if ! "$temp_file" --version &>/dev/null; then
        print_error "Downloaded binary is not valid"
        rm -f "$temp_file"
        return 1
    fi
    
    print_status "Installing to $INSTALL_DIR/hadolint..."
    if [[ -w "$INSTALL_DIR" ]]; then
        mv "$temp_file" "$INSTALL_DIR/hadolint"
    else
        sudo mv "$temp_file" "$INSTALL_DIR/hadolint"
    fi
    
    print_success "Hadolint installed"
}

install_tool() {
    local tool="$1"
    
    case "$tool" in
        skaffold) install_skaffold ;;
        hadolint) install_hadolint ;;
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
                for tool_entry in "${CONTAINER_TOOLS[@]}"; do
                    IFS=: read -r tool _ <<< "$tool_entry"
                    tools+=("$tool")
                done
                shift
                ;;
            skaffold|hadolint)
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
        for tool_entry in "${CONTAINER_TOOLS[@]}"; do
            IFS=: read -r tool _ <<< "$tool_entry"
            tools+=("$tool")
        done
    fi
    
    print_header "Container/K8s Tools Installer"
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
