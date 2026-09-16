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
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
DRY_RUN="${DRY_RUN:-false}"

MODERN_CLI_TOOLS=(
    "git-cliff:Changelog generator from conventional commits"
    "lazygit:Simple terminal UI for git commands"
    "lazydocker:Terminal UI for Docker"
    "delta:Better diff viewer"
    "bottom:System monitor (btm)"
    "eza:Better ls alternative"
    "carapace:Universal completion generator"
    "xh:Friendly HTTP client"
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

Modern CLI Tools Installer - Install modern CLI tools from GitHub releases

TOOLS:
    git-cliff    Changelog generator from conventional commits
    lazygit      Simple terminal UI for git commands
    lazydocker   Terminal UI for Docker
    delta        Better diff viewer (git-delta)
    bottom       System monitor (btm command)
    eza          Better ls alternative
    carapace     Universal completion generator
    xh           Friendly HTTP client
    all          Install all tools (default)

OPTIONS:
    -c, --check       Check installed versions without installing
    -d, --dir DIR     Installation directory (default: ~/.local/bin)
    --dry-run         Show what would be done without installing
    -h, --help        Show this help message

EXAMPLES:
    $0                     # Install all modern CLI tools
    $0 lazygit lazydocker delta       # Install specific tools
    $0 --check             # Check installed versions
    $0 --dry-run           # Preview installation steps

NOTES:
    - These tools are installed from GitHub releases when not available in repos
    - Installation prefers system package managers when available
    - Falls back to binary releases for older distributions

EOF
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

get_version() {
    local tool="$1"
    case "$tool" in
        git-cliff)
            if command_exists git-cliff; then
                git-cliff --version 2>&1 | awk '{print $2}'
            fi
            ;;
        lazygit)
            if command_exists lazygit; then
                lazygit --version 2>&1 | grep -o 'version=[^ ]*' | cut -d'=' -f2 | tr -d ','
            fi
            ;;
        lazydocker)
            if command_exists lazydocker; then
                lazydocker --version 2>&1 | awk '/Version:/{print $2}'
            fi
            ;;
        delta)
            if command_exists delta; then
                delta --version 2>&1 | awk '{print $2}'
            fi
            ;;
        bottom|btm)
            if command_exists btm; then
                btm --version 2>&1 | awk '{print $2}'
            fi
            ;;
        eza)
            if command_exists eza; then
                eza --version 2>&1 | head -1 | awk '{print $2}' | tr -d 'v'
            fi
            ;;
        carapace)
            if command_exists carapace; then
                carapace --version 2>&1 | awk '{print $3}'
            fi
            ;;
        xh)
            if command_exists xh; then
                xh --version 2>&1 | awk '{print $2}'
            fi
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

check_tools() {
    print_header "Modern CLI Tools Status"
    echo
    
    for tool_entry in "${MODERN_CLI_TOOLS[@]}"; do
        IFS=: read -r tool desc <<< "$tool_entry"
        # Special case for bottom which uses btm command
        local check_tool="$tool"
        [[ "$tool" == "bottom" ]] && check_tool="btm"
        
        if command_exists "$check_tool"; then
            local version
            version=$(get_version "$tool")
            print_success "$tool: $version - $desc"
        else
            print_warning "$tool: not installed - $desc"
        fi
    done
}

ensure_install_dir() {
    if [[ ! -d "$INSTALL_DIR" ]]; then
        if [[ "$DRY_RUN" == "true" ]]; then
            print_status "Would create directory: $INSTALL_DIR"
        else
            mkdir -p "$INSTALL_DIR"
        fi
    fi
    
    # Check if install dir is in PATH
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        print_warning "$INSTALL_DIR is not in PATH"
        print_warning "Add this to your shell config: export PATH=\"$INSTALL_DIR:\$PATH\""
    fi
}

install_git_cliff() {
    print_status "Installing git-cliff..."

    if command_exists git-cliff; then
        print_warning "git-cliff is already installed: $(get_version git-cliff)"
        return 0
    fi

    ensure_install_dir

    local version arch os url temp_dir

    # Get latest version from GitHub API
    version=$(curl -fsSL https://api.github.com/repos/orhun/git-cliff/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')

    if [[ -z "$version" ]]; then
        print_error "Failed to determine latest git-cliff version"
        return 1
    fi

    print_status "Latest version: $version"

    if is_macos; then
        os="apple-darwin"
    else
        os="unknown-linux-gnu"
    fi

    arch=$(detect_arch)
    if [[ "$arch" == "aarch64" ]]; then
        arch="aarch64"
    elif [[ "$arch" == "arm64" ]]; then
        arch="aarch64"
    else
        arch="x86_64"
    fi

    url="https://github.com/orhun/git-cliff/releases/download/v${version}/git-cliff-${version}-${arch}-${os}.tar.gz"

    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would download: $url"
        print_status "Would install to: $INSTALL_DIR/git-cliff"
        return 0
    fi

    temp_dir=$(mktemp -d)

    print_status "Downloading git-cliff from GitHub..."
    if ! curl -fsSL "$url" -o "$temp_dir/git-cliff.tar.gz"; then
        print_error "Failed to download git-cliff from $url"
        rm -rf "$temp_dir"
        return 1
    fi

    print_status "Extracting..."
    tar -xzf "$temp_dir/git-cliff.tar.gz" -C "$temp_dir"

    print_status "Installing to $INSTALL_DIR/git-cliff..."
    find "$temp_dir" -name "git-cliff" -type f -executable -exec mv {} "$INSTALL_DIR/git-cliff" \;
    chmod +x "$INSTALL_DIR/git-cliff"

    rm -rf "$temp_dir"
    print_success "git-cliff installed"
}

install_lazygit() {
    print_status "Installing lazygit..."
    
    if command_exists lazygit; then
        print_warning "lazygit is already installed: $(get_version lazygit)"
        return 0
    fi
    
    ensure_install_dir
    
    local version arch os url temp_dir
    
    # Get latest version from GitHub API
    version=$(curl -fsSL https://api.github.com/repos/jesseduffield/lazygit/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
    
    if [[ -z "$version" ]]; then
        print_error "Failed to determine latest lazygit version"
        return 1
    fi
    
    print_status "Latest version: $version"
    
    if is_macos; then
        os="Darwin"
    else
        os="Linux"
    fi
    
    arch=$(detect_arch)
    if [[ "$arch" == "aarch64" ]]; then
        arch="arm64"
    fi
    
    url="https://github.com/jesseduffield/lazygit/releases/download/v${version}/lazygit_${version}_${os}_${arch}.tar.gz"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would download: $url"
        print_status "Would install to: $INSTALL_DIR/lazygit"
        return 0
    fi
    
    temp_dir=$(mktemp -d)
    
    print_status "Downloading lazygit from GitHub..."
    if ! curl -fsSL "$url" -o "$temp_dir/lazygit.tar.gz"; then
        print_error "Failed to download lazygit from $url"
        rm -rf "$temp_dir"
        return 1
    fi
    
    print_status "Extracting..."
    tar -xzf "$temp_dir/lazygit.tar.gz" -C "$temp_dir"
    
    print_status "Installing to $INSTALL_DIR/lazygit..."
    mv "$temp_dir/lazygit" "$INSTALL_DIR/lazygit"
    chmod +x "$INSTALL_DIR/lazygit"
    
    rm -rf "$temp_dir"
    print_success "lazygit installed"
}

install_lazydocker() {
    print_status "Installing lazydocker..."

    if command_exists lazydocker; then
        print_warning "lazydocker is already installed: $(get_version lazydocker)"
        return 0
    fi

    ensure_install_dir

    local version arch os url temp_dir

    version=$(curl -fsSL https://api.github.com/repos/jesseduffield/lazydocker/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')

    if [[ -z "$version" ]]; then
        print_error "Failed to determine latest lazydocker version"
        return 1
    fi

    print_status "Latest version: $version"

    if is_macos; then
        os="Darwin"
    else
        os="Linux"
    fi

    arch=$(detect_arch)
    if [[ "$arch" == "aarch64" ]]; then
        arch="arm64"
    fi

    url="https://github.com/jesseduffield/lazydocker/releases/download/v${version}/lazydocker_${version}_${os}_${arch}.tar.gz"

    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would download: $url"
        print_status "Would install to: $INSTALL_DIR/lazydocker"
        return 0
    fi

    temp_dir=$(mktemp -d)

    print_status "Downloading lazydocker from GitHub..."
    if ! curl -fsSL "$url" -o "$temp_dir/lazydocker.tar.gz"; then
        print_error "Failed to download lazydocker from $url"
        rm -rf "$temp_dir"
        return 1
    fi

    print_status "Extracting..."
    tar -xzf "$temp_dir/lazydocker.tar.gz" -C "$temp_dir"

    print_status "Installing to $INSTALL_DIR/lazydocker..."
    mv "$temp_dir/lazydocker" "$INSTALL_DIR/lazydocker"
    chmod +x "$INSTALL_DIR/lazydocker"

    rm -rf "$temp_dir"
    print_success "lazydocker installed"
}

install_delta() {
    print_status "Installing git-delta..."
    
    if command_exists delta; then
        print_warning "delta is already installed: $(get_version delta)"
        return 0
    fi
    
    ensure_install_dir
    
    local version arch os url temp_dir
    
    # Get latest version from GitHub API
    version=$(curl -fsSL https://api.github.com/repos/dandavison/delta/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [[ -z "$version" ]]; then
        print_error "Failed to determine latest delta version"
        return 1
    fi
    
    print_status "Latest version: $version"
    
    if is_macos; then
        os="apple-darwin"
    else
        os="unknown-linux-musl"
    fi
    
    arch=$(detect_arch)
    if [[ "$arch" == "aarch64" ]]; then
        arch="aarch64"
    elif [[ "$arch" == "arm64" ]]; then
        arch="aarch64"
    else
        arch="x86_64"
    fi
    
    url="https://github.com/dandavison/delta/releases/download/${version}/delta-${version}-${arch}-${os}.tar.gz"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would download: $url"
        print_status "Would install to: $INSTALL_DIR/delta"
        return 0
    fi
    
    temp_dir=$(mktemp -d)
    
    print_status "Downloading delta from GitHub..."
    if ! curl -fsSL "$url" -o "$temp_dir/delta.tar.gz"; then
        print_error "Failed to download delta from $url"
        rm -rf "$temp_dir"
        return 1
    fi
    
    print_status "Extracting..."
    tar -xzf "$temp_dir/delta.tar.gz" -C "$temp_dir"
    
    print_status "Installing to $INSTALL_DIR/delta..."
    find "$temp_dir" -name "delta" -type f -executable -exec mv {} "$INSTALL_DIR/delta" \;
    chmod +x "$INSTALL_DIR/delta"
    
    rm -rf "$temp_dir"
    print_success "delta installed"
}

install_bottom() {
    print_status "Installing bottom..."
    
    if command_exists btm; then
        print_warning "bottom is already installed: $(get_version bottom)"
        return 0
    fi
    
    ensure_install_dir
    
    local version arch os url temp_dir
    
    # Get latest version from GitHub API
    version=$(curl -fsSL https://api.github.com/repos/ClementTsang/bottom/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [[ -z "$version" ]]; then
        print_error "Failed to determine latest bottom version"
        return 1
    fi
    
    print_status "Latest version: $version"
    
    if is_macos; then
        os="apple-darwin"
    else
        os="unknown-linux-musl"
    fi
    
    arch=$(detect_arch)
    if [[ "$arch" == "aarch64" ]]; then
        arch="aarch64"
    elif [[ "$arch" == "arm64" ]]; then
        arch="aarch64"
    else
        arch="x86_64"
    fi
    
    url="https://github.com/ClementTsang/bottom/releases/download/${version}/bottom_${arch}-${os}.tar.gz"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would download: $url"
        print_status "Would install to: $INSTALL_DIR/btm"
        return 0
    fi
    
    temp_dir=$(mktemp -d)
    
    print_status "Downloading bottom from GitHub..."
    if ! curl -fsSL "$url" -o "$temp_dir/bottom.tar.gz"; then
        print_error "Failed to download bottom from $url"
        rm -rf "$temp_dir"
        return 1
    fi
    
    print_status "Extracting..."
    tar -xzf "$temp_dir/bottom.tar.gz" -C "$temp_dir"
    
    print_status "Installing to $INSTALL_DIR/btm..."
    mv "$temp_dir/btm" "$INSTALL_DIR/btm"
    chmod +x "$INSTALL_DIR/btm"
    
    rm -rf "$temp_dir"
    print_success "bottom installed"
}

install_eza() {
    print_status "Installing eza..."
    
    if command_exists eza; then
        print_warning "eza is already installed: $(get_version eza)"
        return 0
    fi
    
    ensure_install_dir
    
    local version arch url temp_file
    
    # Get latest version from GitHub API
    version=$(curl -fsSL https://api.github.com/repos/eza-community/eza/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
    
    if [[ -z "$version" ]]; then
        print_error "Failed to determine latest eza version"
        return 1
    fi
    
    print_status "Latest version: $version"
    
    arch=$(detect_arch)
    
    if is_macos; then
        # macOS uses zip files
        url="https://github.com/eza-community/eza/releases/download/v${version}/eza_${arch}-apple-darwin.zip"
        
        if [[ "$DRY_RUN" == "true" ]]; then
            print_status "Would download: $url"
            print_status "Would install to: $INSTALL_DIR/eza"
            return 0
        fi
        
        temp_file=$(mktemp)
        
        print_status "Downloading eza from GitHub..."
        if ! curl -fsSL "$url" -o "$temp_file"; then
            print_error "Failed to download eza from $url"
            rm -f "$temp_file"
            return 1
        fi
        
        local temp_dir
        temp_dir=$(mktemp -d)
        unzip -q "$temp_file" -d "$temp_dir"
        mv "$temp_dir/eza" "$INSTALL_DIR/eza"
        chmod +x "$INSTALL_DIR/eza"
        rm -rf "$temp_dir" "$temp_file"
    else
        # Linux uses tar.gz
        url="https://github.com/eza-community/eza/releases/download/v${version}/eza_${arch}-unknown-linux-musl.tar.gz"
        
        if [[ "$DRY_RUN" == "true" ]]; then
            print_status "Would download: $url"
            print_status "Would install to: $INSTALL_DIR/eza"
            return 0
        fi
        
        temp_file=$(mktemp)
        
        print_status "Downloading eza from GitHub..."
        if ! curl -fsSL "$url" -o "$temp_file"; then
            print_error "Failed to download eza from $url"
            rm -f "$temp_file"
            return 1
        fi
        
        print_status "Extracting and installing..."
        tar -xzf "$temp_file" -C "$INSTALL_DIR" ./eza
        chmod +x "$INSTALL_DIR/eza"
        rm -f "$temp_file"
    fi
    
    print_success "eza installed"
}

install_carapace() {
    print_status "Installing carapace..."
    
    if command_exists carapace; then
        print_warning "carapace is already installed: $(get_version carapace)"
        return 0
    fi
    
    ensure_install_dir
    
    local version arch os url temp_dir
    
    # Get latest version from GitHub API
    version=$(curl -fsSL https://api.github.com/repos/carapace-sh/carapace-bin/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
    
    if [[ -z "$version" ]]; then
        print_error "Failed to determine latest carapace version"
        return 1
    fi
    
    print_status "Latest version: $version"
    
    if is_macos; then
        os="darwin"
    else
        os="linux"
    fi
    
    arch=$(detect_arch)
    if [[ "$arch" == "aarch64" ]]; then
        arch="arm64"
    elif [[ "$arch" == "x86_64" ]]; then
        arch="amd64"
    fi
    
    url="https://github.com/carapace-sh/carapace-bin/releases/download/v${version}/carapace-bin_${os}_${arch}.tar.gz"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would download: $url"
        print_status "Would install to: $INSTALL_DIR/carapace"
        return 0
    fi
    
    temp_dir=$(mktemp -d)
    
    print_status "Downloading carapace from GitHub..."
    if ! curl -fsSL "$url" -o "$temp_dir/carapace.tar.gz"; then
        print_error "Failed to download carapace from $url"
        rm -rf "$temp_dir"
        return 1
    fi
    
    print_status "Extracting..."
    tar -xzf "$temp_dir/carapace.tar.gz" -C "$temp_dir"
    
    print_status "Installing to $INSTALL_DIR/carapace..."
    mv "$temp_dir/carapace" "$INSTALL_DIR/carapace"
    chmod +x "$INSTALL_DIR/carapace"
    
    rm -rf "$temp_dir"
    print_success "carapace installed"
}

install_xh() {
    print_status "Installing xh..."
    
    if command_exists xh; then
        print_warning "xh is already installed: $(get_version xh)"
        return 0
    fi
    
    ensure_install_dir
    
    local version arch os url temp_dir
    
    # Get latest version from GitHub API
    version=$(curl -fsSL https://api.github.com/repos/ducaale/xh/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
    
    if [[ -z "$version" ]]; then
        print_error "Failed to determine latest xh version"
        return 1
    fi
    
    print_status "Latest version: $version"
    
    if is_macos; then
        os="apple-darwin"
    else
        os="unknown-linux-musl"
    fi
    
    arch=$(detect_arch)
    if [[ "$arch" == "aarch64" ]]; then
        arch="aarch64"
    elif [[ "$arch" == "arm64" ]]; then
        arch="aarch64"
    else
        arch="x86_64"
    fi
    
    url="https://github.com/ducaale/xh/releases/download/v${version}/xh-v${version}-${arch}-${os}.tar.gz"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would download: $url"
        print_status "Would install to: $INSTALL_DIR/xh"
        return 0
    fi
    
    temp_dir=$(mktemp -d)
    
    print_status "Downloading xh from GitHub..."
    if ! curl -fsSL "$url" -o "$temp_dir/xh.tar.gz"; then
        print_error "Failed to download xh from $url"
        rm -rf "$temp_dir"
        return 1
    fi
    
    print_status "Extracting..."
    tar -xzf "$temp_dir/xh.tar.gz" -C "$temp_dir"
    
    print_status "Installing to $INSTALL_DIR/xh..."
    find "$temp_dir" -name "xh" -type f -executable -exec mv {} "$INSTALL_DIR/xh" \;
    chmod +x "$INSTALL_DIR/xh"
    
    rm -rf "$temp_dir"
    print_success "xh installed"
}

install_tool() {
    local tool="$1"
    
    case "$tool" in
        git-cliff) install_git_cliff ;;
        lazygit) install_lazygit ;;
        lazydocker) install_lazydocker ;;
        delta) install_delta ;;
        bottom) install_bottom ;;
        eza) install_eza ;;
        carapace) install_carapace ;;
        xh) install_xh ;;
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
                for tool_entry in "${MODERN_CLI_TOOLS[@]}"; do
                    IFS=: read -r tool _ <<< "$tool_entry"
                    tools+=("$tool")
                done
                shift
                ;;
            git-cliff|lazygit|lazydocker|delta|bottom|eza|carapace|xh)
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
        for tool_entry in "${MODERN_CLI_TOOLS[@]}"; do
            IFS=: read -r tool _ <<< "$tool_entry"
            tools+=("$tool")
        done
    fi
    
    print_header "Modern CLI Tools Installer"
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
