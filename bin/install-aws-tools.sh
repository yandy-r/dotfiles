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

AWS_TOOLS=(
    "aws:AWS CLI v2"
    "sam:AWS SAM CLI"
    "ecs-cli:Amazon ECS CLI"
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

AWS Tools Installer - Install AWS CLI tools from official sources

TOOLS:
    aws          AWS CLI v2
    sam          AWS SAM CLI
    ecs-cli      Amazon ECS CLI
    all          Install all tools (default)

OPTIONS:
    -c, --check       Check installed versions without installing
    -d, --dir DIR     Installation directory (default: /usr/local/bin)
    --dry-run         Show what would be done without installing
    -h, --help        Show this help message

EXAMPLES:
    $0                     # Install all AWS tools
    $0 aws sam             # Install only AWS CLI and SAM CLI
    $0 --check             # Check installed versions
    $0 --dry-run           # Preview installation steps

EOF
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

require_command() {
	local cmd="$1"
	local hint="${2:-Please install $cmd first}"

	if command_exists "$cmd"; then
		return 0
	fi

	print_error "$cmd is required"
	print_error "$hint"
	return 1
}

get_version() {
    local tool="$1"
    case "$tool" in
        aws)
            if command_exists aws; then
                aws --version 2>&1 | awk '{print $1}' | cut -d'/' -f2
            fi
            ;;
        sam)
            if command_exists sam; then
                sam --version 2>&1 | awk '{print $4}'
            fi
            ;;
        ecs-cli)
            if command_exists ecs-cli; then
                ecs-cli --version 2>&1 | awk '{print $3}'
            fi
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

check_tools() {
    print_header "AWS Tools Status"
    echo
    
    for tool_entry in "${AWS_TOOLS[@]}"; do
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

install_aws_cli() {
    print_status "Installing AWS CLI v2..."

    if command_exists aws; then
        print_warning "AWS CLI is already installed: $(get_version aws)"
        return 0
    fi

    # main() calls this as `if ! install_tool ...`, which disables errexit
    # inside the function, so every step must check its own status (YAN-1388).
    local arch url temp_dir
    arch=$(detect_arch)

    if is_macos; then
        if [[ "$arch" == "arm64" ]]; then
            url="https://awscli.amazonaws.com/AWSCLIV2-arm64.pkg"
        else
            url="https://awscli.amazonaws.com/AWSCLIV2.pkg"
        fi
    elif is_linux; then
        # detect_arch canonicalizes uname -m to "arm64"; accept the raw
        # "aarch64" too so this arm matches regardless of which spelling a
        # caller supplies.
        case "$arch" in
            x86_64) url="https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" ;;
            arm64|aarch64) url="https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip" ;;
            *)
                print_error "Unsupported architecture: $arch"
                return 1
                ;;
        esac
    else
        print_error "Unsupported platform for AWS CLI v2: $DOTFILES_OS"
        return 1
    fi

    print_status "Downloading AWS CLI v2 for $DOTFILES_OS ($arch)..."
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would download: $url"
        return 0
    fi

    if ! require_command curl "Please install curl first"; then
        return 1
    fi

    temp_dir=$(mktemp -d)

    if is_macos; then
        if ! curl -fsSL "$url" -o "$temp_dir/AWSCLIV2.pkg"; then
            print_error "Failed to download AWS CLI v2 from $url"
            rm -rf "$temp_dir"
            return 1
        fi
        print_status "Installing AWS CLI v2..."
        if ! sudo installer -pkg "$temp_dir/AWSCLIV2.pkg" -target /; then
            print_error "AWS CLI v2 installer failed"
            rm -rf "$temp_dir"
            return 1
        fi
    else
        if ! require_command unzip "Please install unzip first"; then
            rm -rf "$temp_dir"
            return 1
        fi
        if ! curl -fsSL "$url" -o "$temp_dir/awscliv2.zip"; then
            print_error "Failed to download AWS CLI v2 from $url"
            rm -rf "$temp_dir"
            return 1
        fi
        if ! unzip -q "$temp_dir/awscliv2.zip" -d "$temp_dir"; then
            print_error "Failed to extract AWS CLI v2 installer"
            rm -rf "$temp_dir"
            return 1
        fi
        print_status "Installing AWS CLI v2..."
        if ! sudo "$temp_dir/aws/install" --bin-dir "$INSTALL_DIR" --install-dir /usr/local/aws-cli --update; then
            print_error "AWS CLI v2 installer failed"
            rm -rf "$temp_dir"
            return 1
        fi
    fi

    rm -rf "$temp_dir"
    hash -r

    if ! command_exists aws; then
        print_error "AWS CLI v2 installer completed, but aws is not on PATH"
        print_error "Ensure $INSTALL_DIR is on PATH, then rerun this installer"
        return 1
    fi

    print_success "AWS CLI v2 installed"
}

install_sam_cli() {
    print_status "Installing AWS SAM CLI..."
    
    if command_exists sam; then
        print_warning "AWS SAM CLI is already installed: $(get_version sam)"
        return 0
    fi

    local arch temp_dir url
	arch=$(detect_arch)
	temp_dir=$(mktemp -d)

	if is_linux; then
		case "$arch" in
			x86_64)
				url="https://github.com/aws/aws-sam-cli/releases/latest/download/aws-sam-cli-linux-x86_64.zip"
				;;
			arm64)
				url="https://github.com/aws/aws-sam-cli/releases/latest/download/aws-sam-cli-linux-arm64.zip"
				;;
			*)
				print_error "Unsupported architecture for AWS SAM CLI: $arch"
				rm -rf "$temp_dir"
				return 1
				;;
		esac

		if [[ "$DRY_RUN" == "true" ]]; then
			print_status "Would download: $url"
			print_status "Would install with: sudo ./sam-installation/install --bin-dir $INSTALL_DIR --install-dir /usr/local/aws-sam-cli"
			rm -rf "$temp_dir"
			return 0
		fi

		if ! require_command curl "Please install curl first"; then
			rm -rf "$temp_dir"
			return 1
		fi
		if ! require_command unzip "Please install unzip first"; then
			rm -rf "$temp_dir"
			return 1
		fi

		print_status "Downloading AWS SAM CLI for Linux ($arch)..."
		if ! curl -fsSL "$url" -o "$temp_dir/aws-sam-cli.zip"; then
			print_error "Failed to download AWS SAM CLI from $url"
			rm -rf "$temp_dir"
			return 1
		fi

		if ! unzip -q "$temp_dir/aws-sam-cli.zip" -d "$temp_dir/sam-installation"; then
			print_error "Failed to extract AWS SAM CLI installer"
			rm -rf "$temp_dir"
			return 1
		fi

		print_status "Installing AWS SAM CLI..."
		if ! sudo "$temp_dir/sam-installation/install" --bin-dir "$INSTALL_DIR" --install-dir /usr/local/aws-sam-cli; then
			print_error "AWS SAM CLI installer failed"
			rm -rf "$temp_dir"
			return 1
		fi
	elif is_macos; then
		case "$arch" in
			x86_64)
				url="https://github.com/aws/aws-sam-cli/releases/latest/download/aws-sam-cli-macos-x86_64.pkg"
				;;
			arm64)
				url="https://github.com/aws/aws-sam-cli/releases/latest/download/aws-sam-cli-macos-arm64.pkg"
				;;
			*)
				print_error "Unsupported architecture for AWS SAM CLI: $arch"
				rm -rf "$temp_dir"
				return 1
				;;
		esac

		if [[ "$DRY_RUN" == "true" ]]; then
			print_status "Would download: $url"
			print_status "Would install with: sudo installer -pkg aws-sam-cli.pkg -target /"
			rm -rf "$temp_dir"
			return 0
		fi

		if ! require_command curl "Please install curl first"; then
			rm -rf "$temp_dir"
			return 1
		fi

		print_status "Downloading AWS SAM CLI for macOS ($arch)..."
		if ! curl -fsSL "$url" -o "$temp_dir/aws-sam-cli.pkg"; then
			print_error "Failed to download AWS SAM CLI from $url"
			rm -rf "$temp_dir"
			return 1
		fi

		print_status "Installing AWS SAM CLI..."
		if ! sudo installer -pkg "$temp_dir/aws-sam-cli.pkg" -target /; then
			print_error "AWS SAM CLI installer failed"
			rm -rf "$temp_dir"
			return 1
		fi
	else
		print_error "Unsupported platform for AWS SAM CLI: $DOTFILES_OS"
		rm -rf "$temp_dir"
		return 1
	fi

	rm -rf "$temp_dir"
	hash -r

	if ! command_exists sam; then
		print_error "AWS SAM CLI installer completed, but sam is not on PATH"
		print_error "Ensure $INSTALL_DIR is on PATH, then rerun this installer"
		return 1
	fi

	print_success "AWS SAM CLI installed"

	if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
		print_warning "$INSTALL_DIR is not in PATH"
		print_warning "Add this to your shell config: export PATH=\"$INSTALL_DIR:\$PATH\""
	fi
}

install_ecs_cli() {
    print_status "Installing Amazon ECS CLI..."
    
    if command_exists ecs-cli; then
        print_warning "Amazon ECS CLI is already installed: $(get_version ecs-cli)"
        return 0
    fi
    
    local os arch latest_version url
    
    # Get latest version from GitHub API
    latest_version=$(curl -fsSL https://api.github.com/repos/aws/amazon-ecs-cli/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
    
    if [[ -z "$latest_version" ]]; then
        print_error "Failed to determine latest ECS CLI version"
        return 1
    fi
    
    print_status "Latest version: $latest_version"
    
    if is_macos; then
        os="darwin"
        if is_arm64; then
            print_warning "No native darwin-arm64 ECS CLI build; using amd64 binary via Rosetta 2"
        fi
        arch="amd64"
    else
        os="linux"
        arch=$(detect_arch)
        if [[ "$arch" == "x86_64" ]]; then
            arch="amd64"
        elif [[ "$arch" == "arm64" ]] || [[ "$arch" == "aarch64" ]]; then
            arch="arm64"
        fi
    fi
    
    url="https://amazon-ecs-cli.s3.amazonaws.com/ecs-cli-${os}-${arch}-v${latest_version}"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        print_status "Would download: $url"
        print_status "Would install to: $INSTALL_DIR/ecs-cli"
    else
        print_status "Downloading ECS CLI from AWS S3..."
        local temp_file
        temp_file=$(mktemp)
        
        if ! curl -fsSL "$url" -o "$temp_file"; then
            print_error "Failed to download ECS CLI from $url"
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
        
        print_status "Installing to $INSTALL_DIR/ecs-cli..."
        local mv_cmd=(mv)
        [[ -w "$INSTALL_DIR" ]] || mv_cmd=(sudo mv)
        if ! "${mv_cmd[@]}" "$temp_file" "$INSTALL_DIR/ecs-cli"; then
            print_error "Failed to install ECS CLI to $INSTALL_DIR/ecs-cli"
            rm -f "$temp_file"
            return 1
        fi
    fi
    
    print_success "Amazon ECS CLI installed"
}

install_tool() {
    local tool="$1"
    
    case "$tool" in
        aws) install_aws_cli ;;
        sam) install_sam_cli ;;
        ecs-cli) install_ecs_cli ;;
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
                for tool_entry in "${AWS_TOOLS[@]}"; do
                    IFS=: read -r tool _ <<< "$tool_entry"
                    tools+=("$tool")
                done
                shift
                ;;
            aws|sam|ecs-cli)
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
        for tool_entry in "${AWS_TOOLS[@]}"; do
            IFS=: read -r tool _ <<< "$tool_entry"
            tools+=("$tool")
        done
    fi
    
    print_header "AWS Tools Installer"
    print_status "Platform: $DOTFILES_OS ($DOTFILES_ARCH)"
    print_status "Install directory: $INSTALL_DIR"
    [[ "$DRY_RUN" == "true" ]] && print_warning "DRY RUN MODE - No actual changes will be made"
    echo
    
    print_status "Installing ${#tools[@]} tools..."
    echo
    
    local fatal_failures=0

    for tool in "${tools[@]}"; do
        if ! install_tool "$tool"; then
            case "$tool" in
                ecs-cli)
                    print_warning "Amazon ECS CLI installation failed; continuing with remaining tools"
                    ;;
                aws|sam)
                    print_error "$tool installation failed"
                    fatal_failures=$((fatal_failures + 1))
                    ;;
            esac
        fi
    done

    echo
    if [[ "$fatal_failures" -gt 0 ]]; then
        print_error "$fatal_failures required tool(s) failed to install"
        exit 1
    fi

    print_header "Installation Complete"
    check_tools
}

main "$@"
