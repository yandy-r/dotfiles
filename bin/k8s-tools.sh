#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOTFILES_ROOT="$(dirname "$SCRIPT_DIR")"

# shellcheck source=../common/platform.sh
source "$DOTFILES_ROOT/common/platform.sh"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

K8S_TOOLS=(
    "kubectl:Kubernetes command-line tool"
    "helm:Kubernetes package manager"
    "k9s:Kubernetes TUI"
    "kubectx:Switch between kubectl contexts"
    "kubens:Switch between Kubernetes namespaces"
    "kustomize:Kubernetes configuration customization"
    "stern:Multi-pod log tailing"
    "krew:kubectl plugin manager"
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

Kubernetes Tools Installer - Bootstrap common Kubernetes CLI tools

TOOLS:
    kubectl      Kubernetes command-line tool
    helm         Kubernetes package manager
    k9s          Kubernetes TUI
    kubectx      Switch between kubectl contexts
    kubens       Switch between Kubernetes namespaces
    kustomize    Kubernetes configuration customization
    stern        Multi-pod log tailing
    krew         kubectl plugin manager
    all          Install all tools (default)

OPTIONS:
    -c, --check   Check installed versions without installing
    -u, --update  Update existing installations
    -h, --help    Show this help message

EXAMPLES:
    $0                        # Install all tools
    $0 kubectl helm           # Install only kubectl and helm
    $0 --check                # Check installed versions
    $0 --update kubectl k9s   # Update kubectl and k9s

EOF
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

get_version() {
    local tool="$1"
    case "$tool" in
        kubectl)
            if command_exists kubectl; then
                kubectl version --client --short 2>/dev/null | cut -d' ' -f3 | tr -d 'v' || echo "unknown"
            fi
            ;;
        helm)
            if command_exists helm; then
                helm version --short 2>/dev/null | cut -d'+' -f1 | tr -d 'v' || echo "unknown"
            fi
            ;;
        k9s)
            if command_exists k9s; then
                k9s version --short 2>/dev/null | head -1 | awk '{print $2}' | tr -d 'v' || echo "unknown"
            fi
            ;;
        kubectx|kubens)
            if command_exists "$tool"; then
                echo "installed"
            fi
            ;;
        kustomize)
            if command_exists kustomize; then
                kustomize version --short 2>/dev/null | cut -d'/' -f2 | cut -d' ' -f1 | tr -d 'v' || echo "unknown"
            fi
            ;;
        stern)
            if command_exists stern; then
                stern --version 2>/dev/null | awk '{print $3}' | tr -d 'v' || echo "unknown"
            fi
            ;;
        krew)
            if command_exists kubectl-krew; then
                kubectl krew version 2>/dev/null | head -1 | awk '{print $2}' | tr -d 'v' || echo "installed"
            fi
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

check_tools() {
    print_header "Kubernetes Tools Status"
    echo
    
    for tool_entry in "${K8S_TOOLS[@]}"; do
        IFS=: read -r tool desc <<< "$tool_entry"
        if command_exists "$tool" || [[ "$tool" == "krew" ]] && command_exists kubectl-krew; then
            local version
            version=$(get_version "$tool")
            print_success "$tool: $version - $desc"
        else
            print_warning "$tool: not installed - $desc"
        fi
    done
}

install_kubectl() {
    print_status "Installing kubectl..."
    
    if is_macos; then
        if command_exists brew; then
            brew install kubectl
        else
            local arch
            arch=$(detect_arch)
            [[ "$arch" == "x86_64" ]] && arch="amd64"
            curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/darwin/${arch}/kubectl"
            chmod +x kubectl
            sudo mv kubectl /usr/local/bin/
        fi
    elif is_linux; then
        if command_exists brew; then
            brew install kubectl
        else
            local arch
            arch=$(detect_arch)
            [[ "$arch" == "x86_64" ]] && arch="amd64"
            curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/${arch}/kubectl"
            chmod +x kubectl
            sudo mv kubectl /usr/local/bin/
        fi
    fi
    
    print_success "kubectl installed"
}

install_helm() {
    print_status "Installing helm..."
    
    if is_macos; then
        if command_exists brew; then
            brew install helm
        else
            curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
        fi
    elif is_linux; then
        if command_exists brew; then
            brew install helm
        else
            curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
        fi
    fi
    
    print_success "helm installed"
}

install_k9s() {
    print_status "Installing k9s..."
    
    if command_exists brew; then
        brew install k9s
    else
        local os arch version url
        version=$(curl -s https://api.github.com/repos/derailed/k9s/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
        
        if is_macos; then
            os="Darwin"
        else
            os="Linux"
        fi
        
        arch=$(detect_arch)
        if [[ "$arch" == "x86_64" ]]; then
            arch="amd64"
        elif [[ "$arch" == "arm64" ]] && is_macos; then
            arch="arm64"
        fi
        
        url="https://github.com/derailed/k9s/releases/download/v${version}/k9s_${os}_${arch}.tar.gz"
        
        curl -L "$url" | tar xz
        chmod +x k9s
        sudo mv k9s /usr/local/bin/
    fi
    
    print_success "k9s installed"
}

install_kubectx() {
    print_status "Installing kubectx and kubens..."
    
    if command_exists brew; then
        brew install kubectx
    else
        local install_dir="/usr/local/bin"
        sudo curl -L https://raw.githubusercontent.com/ahmetb/kubectx/master/kubectx -o "$install_dir/kubectx"
        sudo curl -L https://raw.githubusercontent.com/ahmetb/kubectx/master/kubens -o "$install_dir/kubens"
        sudo chmod +x "$install_dir/kubectx" "$install_dir/kubens"
    fi
    
    print_success "kubectx and kubens installed"
}

install_kubens() {
    if ! command_exists kubens; then
        install_kubectx
    else
        print_status "kubens already installed via kubectx"
    fi
}

install_kustomize() {
    print_status "Installing kustomize..."
    
    if command_exists brew; then
        brew install kustomize
    else
        curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash
        sudo mv kustomize /usr/local/bin/
    fi
    
    print_success "kustomize installed"
}

install_stern() {
    print_status "Installing stern..."
    
    if command_exists brew; then
        brew install stern
    else
        local os arch version url
        version=$(curl -s https://api.github.com/repos/stern/stern/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
        
        if is_macos; then
            os="darwin"
        else
            os="linux"
        fi
        
        arch=$(detect_arch)
        [[ "$arch" == "x86_64" ]] && arch="amd64"
        
        url="https://github.com/stern/stern/releases/download/v${version}/stern_${version}_${os}_${arch}.tar.gz"
        
        curl -L "$url" | tar xz
        chmod +x stern
        sudo mv stern /usr/local/bin/
    fi
    
    print_success "stern installed"
}

install_krew() {
    print_status "Installing krew..."
    
    if ! command_exists kubectl; then
        print_error "kubectl is required for krew. Installing kubectl first..."
        install_kubectl
    fi
    
    local os arch
    
    if is_macos; then
        os="darwin"
    else
        os="linux"
    fi
    
    arch=$(detect_arch)
    [[ "$arch" == "x86_64" ]] && arch="amd64"
    
    local temp_dir
    temp_dir=$(mktemp -d)
    cd "$temp_dir"
    
    curl -fsSLO "https://github.com/kubernetes-sigs/krew/releases/latest/download/krew-${os}_${arch}.tar.gz"
    tar zxvf "krew-${os}_${arch}.tar.gz"
    ./"krew-${os}_${arch}" install krew
    
    cd - > /dev/null
    rm -rf "$temp_dir"
    
    local krew_path="${KREW_ROOT:-$HOME/.krew}/bin"
    export PATH="${PATH}:${krew_path}"
    
    print_success "krew installed"
    echo
    print_status "Krew has been installed to: ${krew_path}"
    print_status "The path has been added to your current session."
    print_status "To make it permanent, the following is already configured in common/paths.sh:"
    print_status "  export PATH=\"\${KREW_ROOT:-\$HOME/.krew}/bin:\$PATH\""
}

install_tool() {
    local tool="$1"
    
    if command_exists "$tool" || { [[ "$tool" == "krew" ]] && command_exists kubectl-krew; }; then
        print_warning "$tool is already installed"
        return 0
    fi
    
    case "$tool" in
        kubectl) install_kubectl ;;
        helm) install_helm ;;
        k9s) install_k9s ;;
        kubectx) install_kubectx ;;
        kubens) install_kubens ;;
        kustomize) install_kustomize ;;
        stern) install_stern ;;
        krew) install_krew ;;
        *)
            print_error "Unknown tool: $tool"
            return 1
            ;;
    esac
}

update_tool() {
    local tool="$1"
    
    print_status "Updating $tool..."
    
    if command_exists brew; then
        brew upgrade "$tool" 2>/dev/null || print_warning "$tool not managed by brew, reinstalling..."
        if [[ $? -ne 0 ]]; then
            install_tool "$tool"
        fi
    else
        print_status "$tool not managed by brew, reinstalling..."
        if command_exists "$tool" || { [[ "$tool" == "krew" ]] && command_exists kubectl-krew; }; then
            case "$tool" in
                kubectl|k9s|kustomize|stern)
                    sudo rm -f "/usr/local/bin/$tool"
                    ;;
                kubectx|kubens)
                    sudo rm -f /usr/local/bin/kubectx /usr/local/bin/kubens
                    ;;
                krew)
                    rm -rf "${HOME}/.krew"
                    ;;
            esac
        fi
        install_tool "$tool"
    fi
    
    print_success "$tool updated"
}

main() {
    local tools=()
    local check_only=false
    local update_mode=false
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            -c|--check)
                check_only=true
                shift
                ;;
            -u|--update)
                update_mode=true
                shift
                ;;
            -h|--help)
                show_usage
                exit 0
                ;;
            all)
                for tool_entry in "${K8S_TOOLS[@]}"; do
                    IFS=: read -r tool _ <<< "$tool_entry"
                    tools+=("$tool")
                done
                shift
                ;;
            kubectl|helm|k9s|kubectx|kubens|kustomize|stern|krew)
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
        for tool_entry in "${K8S_TOOLS[@]}"; do
            IFS=: read -r tool _ <<< "$tool_entry"
            tools+=("$tool")
        done
    fi
    
    print_header "Kubernetes Tools Installer"
    print_status "Platform: $DOTFILES_OS ($DOTFILES_ARCH)"
    print_status "Package Manager: $DOTFILES_PKG_MGR"
    echo
    
    if [[ "$update_mode" == "true" ]]; then
        print_status "Updating ${#tools[@]} tools..."
        echo
        for tool in "${tools[@]}"; do
            update_tool "$tool"
        done
    else
        print_status "Installing ${#tools[@]} tools..."
        echo
        for tool in "${tools[@]}"; do
            install_tool "$tool"
        done
    fi
    
    echo
    print_header "Installation Complete"
    check_tools
}

main "$@"
