#!/usr/bin/env bash
# ================================================================
# Tool Detection and Setup
# ================================================================
#
# Purpose:
#   Detects, configures, and optionally installs development tools.
#   Provides a consistent interface for checking tool availability
#   and setting up modern CLI alternatives.
#
# Dependencies:
#   - common/platform.sh (for DOTFILES_OS, DOTFILES_PKG_MGR, has_command)
#
# Tool Categories:
#   - Modern CLI:   eza, bat, rg, fd, fzf, zoxide, delta, tldr, htop, ncdu
#   - Development:  git, nvim, tmux, jq, yq, curl, wget, tree
#   - Cloud/DevOps: docker, kubectl, helm, terraform, ansible, aws, gcloud, az
#   - Languages:    node, npm, yarn, python3, pip3, go, rust, cargo
#
# Key Functions:
#   - tool_exists()      - Check if a tool is installed
#   - tool_version()     - Get version of a tool
#   - tool_health()      - Check tool status with version
#   - install_tool()     - Install a tool via package manager
#   - system_health()    - Display overall tool health report
#   - setup_all_tools()  - Initialize all tool configurations
#
# Environment:
#   DOTFILES_NO_AUTO_TOOL_SETUP - Disable automatic setup on source
#   DOTFILES_DEBUG              - Enable debug output
#
# ================================================================

# Source platform detection if not already loaded
if [[ -z "$DOTFILES_OS" ]]; then
    # Intentional dual-shell path resolution: BASH_SOURCE in bash, ${(%):-%x} in zsh.
    # shellcheck disable=SC2296
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-${(%):-%x}}")" && pwd)"
    if [[ -f "$SCRIPT_DIR/platform.sh" ]]; then
        source "$SCRIPT_DIR/platform.sh"
    fi
fi

# _cached_eval (setup_zoxide) — the rc files source it right after platform.sh;
# this covers tools.sh being sourced on its own.
if ! typeset -f _cached_eval >/dev/null 2>&1; then
    source "${DOTFILES_ROOT:-$HOME/.config/dotfiles}/common/cached-eval.sh"
fi

# ================================================================
# TOOL CATEGORIES
# ================================================================
# Tool definitions organized by category. Each tool has a description
# used by the system_health() diagnostic function.

# Check if bash supports associative arrays (bash 4.0+)
if [[ "${BASH_VERSION%%.*}" -ge 4 ]] 2>/dev/null || [[ -n "$ZSH_VERSION" ]]; then
    # Core modern CLI tools
    declare -A MODERN_TOOLS=(
        ["eza"]="Better ls - list directory contents"
        ["bat"]="Better cat - syntax highlighting"
        ["rg"]="Better grep - ripgrep search"
        ["fd"]="Better find - fast file finder"
        ["fzf"]="Fuzzy finder"
        ["zoxide"]="Smart directory jumping (Rust-based)"
        ["delta"]="Better diff viewer"
        ["tldr"]="Simplified man pages"
        ["htop"]="Better top - process viewer"
        ["ncdu"]="Disk usage analyzer"
        ["direnv"]="Environment variable management"
        ["atuin"]="Shell history sync and search"
    )

    # Development tools
    declare -A DEV_TOOLS=(
        ["git"]="Version control"
        ["nvim"]="Modern text editor"
        ["tmux"]="Terminal multiplexer"
        ["jq"]="JSON processor"
        ["yq"]="YAML processor"
        ["curl"]="HTTP client"
        ["wget"]="File downloader"
        ["tree"]="Directory tree viewer"
        ["branchlet"]="Git worktree manager"
		["opencode"]="AI coding agent"
		["fabric"]="AI prompt pattern CLI"
		["tea"]="Gitea/Forgejo CLI"
	)
    # Cloud and DevOps tools
    declare -A CLOUD_TOOLS=(
        ["docker"]="Container platform"
        ["kubectl"]="Kubernetes client"
        ["helm"]="Kubernetes package manager"
        ["terraform"]="Infrastructure as code"
        ["ansible"]="Configuration management"
        ["aws"]="AWS CLI"
        ["gcloud"]="Google Cloud CLI"
        ["az"]="Azure CLI"
    )

    # Language tools
    declare -A LANG_TOOLS=(
        ["node"]="Node.js runtime"
        ["npm"]="Node package manager"
        ["yarn"]="Alternative Node package manager"
        ["python3"]="Python interpreter"
        ["pip3"]="Python package manager"
        ["go"]="Go language"
        ["rust"]="Rust language"
        ["cargo"]="Rust package manager"
    )
    else
		MODERN_TOOLS_LIST="eza bat rg fd fzf zoxide delta tldr htop ncdu direnv atuin"
		DEV_TOOLS_LIST="git nvim tmux jq yq curl wget tree opencode fabric"
		CLOUD_TOOLS_LIST="docker kubectl helm terraform ansible aws gcloud az"
        LANG_TOOLS_LIST="node npm yarn python3 pip3 go rust cargo"
    fi

# ================================================================
# MISE VERSION MANAGER (Replaces NVM)
# ================================================================
# mise is a modern polyglot runtime manager written in Rust.
# It replaces nvm, pyenv, rbenv, etc. with a single tool.
# Docs: https://mise.jdx.dev/
#
# Key commands:
#   mise install node@20    - Install Node.js 20
#   mise use node@20        - Use in current directory
#   mise use -g node@20     - Set global default
#   mise list               - List installed versions

# Check if mise is available
init_mise() {
    if command -v mise &>/dev/null; then
        return 0
    else
        return 1
    fi
}

# ================================================================
# DETECTION FUNCTIONS
# ================================================================
# Functions for checking tool installation status and versions.
# Special handling for mise-managed tools like node, npm, etc.

# Check if a tool is installed (handles mise-managed tools)
# Usage: tool_exists git && echo "Git is installed"
tool_exists() {
    local tool="$1"

    # For mise-managed tools, ensure mise is initialized first
	case "$tool" in
		fabric)
			command -v fabric &> /dev/null || command -v fabric-ai &> /dev/null
			;;
		node|npm|npx|yarn|pnpm)
			init_mise
			command -v "$tool" &> /dev/null
			;;
		*)
			command -v "$tool" &> /dev/null
			;;
	esac
}

# Get tool version (normalized output, handles various --version formats)
# Usage: tool_version node  # Returns: 20.10.0
tool_version() {
	local tool="$1"

	# Special handling for mise-managed tools
	case "$tool" in
		node|npm|npx|yarn|pnpm)
			# Initialize mise if available
			init_mise
			;;
	esac

	case "$tool" in
		fabric)
			if command -v fabric &> /dev/null; then
				fabric --version 2>/dev/null | head -1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1
			elif command -v fabric-ai &> /dev/null; then
				fabric-ai --version 2>/dev/null | head -1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1
			fi
			;;
		node) node --version 2>/dev/null | sed 's/^v//' ;;
        npm) npm --version 2>/dev/null ;;
        npx) npx --version 2>/dev/null ;;
        yarn) yarn --version 2>/dev/null ;;
        pnpm) pnpm --version 2>/dev/null ;;
        python*) $tool --version 2>&1 | awk '{print $2}' ;;
        pip*) $tool --version 2>/dev/null | awk '{print $2}' ;;
        go) go version 2>/dev/null | awk '{print $3}' | sed 's/go//' ;;
        rust) rustc --version 2>/dev/null | awk '{print $2}' ;;
        *) $tool --version 2>/dev/null | head -1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 ;;
    esac
}

# Check tool health and print status with checkmark/cross
# Usage: tool_health git  # Output: "git 2.43.0" or "git (not installed)"
tool_health() {
    local tool="$1"
    if tool_exists "$tool"; then
        local version
        version=$(tool_version "$tool")
        echo "✓ $tool ${version:-(unknown version)}"
        return 0
    else
        echo "✗ $tool (not installed)"
        return 1
    fi
}

# ================================================================
# INSTALLATION FUNCTIONS
# ================================================================
# Functions for installing tools via the detected package manager.
# Handles Homebrew, apt, dnf, yum, and pacman with tool-specific
# installation methods where needed.

# Install a tool via package manager
# Usage: install_tool eza
# Note: Some tools have custom installers (branchlet, opencode)
install_tool() {
    local tool="$1"
    local force="${2:-false}"

    if tool_exists "$tool" && [[ "$force" != "true" ]]; then
        echo "$tool is already installed"
        return 0
    fi

    echo "Installing $tool..."

    # Special handling for tools with custom installers
    case "$tool" in
        branchlet)
            # Initialize mise to ensure npm is accessible
            init_mise

            if tool_exists npm; then
                npm install -g branchlet
            else
                echo "npm not found - please install Node.js first"
                echo "Try: mise install node && mise use -g node"
                return 1
            fi
            ;;
        opencode)
            # Use custom OpenCode installer script
            local install_script="$SCRIPT_DIR/../bin/install-opencode.sh"
            if [[ -f "$install_script" ]]; then
                bash "$install_script" --install
            else
                echo "OpenCode installer script not found at $install_script"
				echo "Try: curl -fsSL https://opencode.ai/v2/install | bash"
                return 1
            fi
			;;
		fabric)
			# Use custom Fabric installer script
			local install_script="$SCRIPT_DIR/../bin/install-fabric.sh"
			if [[ -f "$install_script" ]]; then
				bash "$install_script" --install
			else
				echo "Fabric installer script not found at $install_script"
				echo "Try: curl -fsSL https://raw.githubusercontent.com/danielmiessler/Fabric/main/scripts/installer/install.sh | bash"
				return 1
			fi
			;;
		*)
            case "$DOTFILES_PKG_MGR" in
                homebrew)
                    brew install "$tool"
                    ;;
                apt)
                    case "$tool" in
                        eza) curl -sSfL https://github.com/eza-community/eza/releases/latest/download/eza_x86_64-unknown-linux-gnu.tar.gz | tar -xz -C ~/.local/bin ;;
                        bat) sudo apt install bat && ln -sf /usr/bin/batcat ~/.local/bin/bat ;;
                        rg) sudo apt install ripgrep ;;
                        fd) sudo apt install fd-find && ln -sf /usr/bin/fdfind ~/.local/bin/fd ;;
                        fzf) sudo apt install fzf ;;
                        delta)
                            local latest
                            latest=$(curl -s https://api.github.com/repos/dandavison/delta/releases/latest | grep -oP '"tag_name": "\K[^"]*')
                            curl -sSfL "https://github.com/dandavison/delta/releases/download/$latest/delta-${latest}-x86_64-unknown-linux-gnu.tar.gz" | tar -xz -C /tmp
                            cp "/tmp/delta-${latest}-x86_64-unknown-linux-gnu/delta" ~/.local/bin/
                            ;;
                        *) sudo apt install "$tool" ;;
                    esac
                    ;;
                dnf|yum)
                    sudo $DOTFILES_PKG_MGR install "$tool"
                    ;;
                pacman)
                    case "$tool" in
                        bat) sudo pacman -S bat ;;
                        rg) sudo pacman -S ripgrep ;;
                        fd) sudo pacman -S fd ;;
                        *) sudo pacman -S "$tool" ;;
                    esac
                    ;;
                *)
                    echo "Package manager not supported for auto-installation"
                    return 1
                    ;;
            esac
            ;;
    esac
}

# Install all modern CLI tools from MODERN_TOOLS list
# Usage: install_modern_tools
install_modern_tools() {
    echo "Installing modern CLI tools..."

    for tool in "${!MODERN_TOOLS[@]}"; do
        install_tool "$tool"
    done
}

# ================================================================
# SETUP FUNCTIONS
# ================================================================
# Tool-specific initialization functions. Each setup_* function
# configures a tool's environment, keybindings, or integrations.

# Set up FZF fuzzy finder with keybindings and completions
# Ctrl-T: File search, Ctrl-R: History search, Alt-C: Directory jump
setup_fzf() {
    if tool_exists fzf; then
        # Set up FZF key bindings and completion
        if [[ -n "${ZSH_VERSION:-}" ]]; then
            # Zsh setup
            if [[ -f ~/.fzf.zsh ]]; then
                source ~/.fzf.zsh
            elif [[ -f "$DOTFILES_HOMEBREW_PATH/opt/fzf/shell/key-bindings.zsh" ]]; then
                source "$DOTFILES_HOMEBREW_PATH/opt/fzf/shell/key-bindings.zsh"
                source "$DOTFILES_HOMEBREW_PATH/opt/fzf/shell/completion.zsh"
            fi
        elif [[ -n "${BASH_VERSION:-}" ]]; then
            # Bash setup
            if [[ -f ~/.fzf.bash ]]; then
                source ~/.fzf.bash
            elif [[ -f "$DOTFILES_HOMEBREW_PATH/opt/fzf/shell/key-bindings.bash" ]]; then
                source "$DOTFILES_HOMEBREW_PATH/opt/fzf/shell/key-bindings.bash"
                source "$DOTFILES_HOMEBREW_PATH/opt/fzf/shell/completion.bash"
            fi
        fi
    fi
}

# Set up zoxide smart directory jumping
# Use 'z' to jump to directories based on frecency (frequency + recency)
# Use 'zi' for interactive selection with fzf
setup_zoxide() {
    if tool_exists zoxide; then
        # Initialize zoxide with default 'z' command (not replacing cd)
        # This avoids issues with shell snapshots and subshells
        # Use 'z' for smart jumping, 'cd' remains standard.
        # Output is cached by common/cached-eval.sh (regenerated on upgrade).
        if [[ -n "${ZSH_VERSION:-}" ]]; then
            _cached_eval zoxide zoxide init zsh
        elif [[ -n "${BASH_VERSION:-}" ]]; then
            _cached_eval zoxide zoxide init bash
        fi
    fi
}

# Set up lazygit terminal UI for git operations
# Press '?' inside lazygit for help
setup_lazygit() {
    if ! tool_exists lazygit; then
        return 0
    fi

    # Config location follows XDG spec (file managed by dotfiles symlinks playbook)
    export LG_CONFIG_FILE="${XDG_CONFIG_HOME:-$HOME/.config}/lazygit/config.yml"
}

# Set up yazi terminal file manager with image preview
# Defines 'yy' function to change directory on exit
setup_yazi() {
    if ! tool_exists yazi; then
        return 0
    fi

    # Yazi wrapper that changes directory on exit
    # This is the recommended way to use yazi for navigation
    yy() {
        local tmp
        tmp="$(mktemp -t "yazi-cwd.XXXXXX")"
        yazi "$@" --cwd-file="$tmp"
        if cwd="$(cat -- "$tmp")" && [[ -n "$cwd" ]] && [[ "$cwd" != "$PWD" ]]; then
            builtin cd -- "$cwd" || return
        fi
        rm -f -- "$tmp"
    }

    # Set up yazi with fzf integration if available
    if command -v fzf &>/dev/null; then
        export YAZI_FILE_ONE="fzf"
    fi
}

# Set up modern CLI alternatives (eza, bat, rg, delta)
# Configures environment variables and creates ripgrep config
setup_modern_alternatives() {
    # Enhanced ls
    if tool_exists eza; then
        export EZA_COLORS="da=1;34:gm=1;34"
        # Note: aliases are set in aliases.sh
    fi

    # Enhanced cat
    if tool_exists bat; then
        export BAT_THEME="${BAT_THEME:-Sublime Snazzy}"
        export BAT_PAGER="less -RF"
        export MANPAGER="sh -c 'col -bx | bat -l man -p'"
    fi

    # Enhanced grep
    if tool_exists rg; then
        export RIPGREP_CONFIG_PATH="$XDG_CONFIG_HOME/ripgrep/config"
        mkdir -p "$(dirname "$RIPGREP_CONFIG_PATH")"

        # Create ripgrep config if it doesn't exist
        if [[ ! -f "$RIPGREP_CONFIG_PATH" ]]; then
            cat > "$RIPGREP_CONFIG_PATH" << 'EOF'
--hidden
--follow
--smart-case
--glob=!.git/*
--glob=!node_modules/*
--glob=!.vscode/*
--glob=!*.lock
--colors=match:fg:yellow
--colors=match:style:bold
--colors=path:fg:green
--colors=line:fg:blue
EOF
        fi
    fi

    # Enhanced diff
    if tool_exists delta; then
        export GIT_PAGER="delta"
        git config --global core.pager delta 2>/dev/null || true
        git config --global interactive.diffFilter "delta --color-only" 2>/dev/null || true
        git config --global delta.navigate true 2>/dev/null || true
        git config --global delta.light false 2>/dev/null || true
    fi
}

# ================================================================
# VERSION MANAGERS SETUP
# ================================================================
# Runtime version managers for Node.js, Python, Rust, etc.
# mise is the preferred tool (replaces nvm, pyenv, rbenv).

# Set up mise polyglot version manager
# Activates mise in the current shell for runtime management
setup_mise() {
    # Skip if mise not installed
    if ! command -v mise &>/dev/null; then
        # Show migration message if NVM was previously used
        if [[ -d "$HOME/.nvm" ]]; then
            if [[ "$DOTFILES_DEBUG" == "1" ]]; then
                echo "[DOTFILES] NVM detected but mise is preferred. Install mise with: bin/install-mise.sh"
            fi
        fi
        return 0
    fi

    # Initialize mise for the current shell
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        eval "$(mise activate zsh)" 2>/dev/null
    elif [[ -n "${BASH_VERSION:-}" ]]; then
        eval "$(mise activate bash)" 2>/dev/null
    fi
}

# Legacy NVM compatibility wrapper - shows migration guide to mise
# This helps users transition from NVM to the preferred mise tool
nvm() {
    echo "WARNING: NVM has been replaced by mise in this dotfiles configuration."
    echo ""
    echo "mise is faster and manages multiple language runtimes (Node, Python, Go, Rust, etc.)"
    echo ""
    echo "Use 'mise' commands instead:"
    echo "  mise install node@20      # Install Node.js 20"
    echo "  mise use node@20          # Use Node.js 20 in current directory"
    echo "  mise use -g node@20       # Set global default"
    echo "  mise list                 # List installed versions"
    echo "  mise current              # Show currently active versions"
    echo ""
    echo "To install mise: ~/.config/dotfiles/bin/install-mise.sh"
    echo ""
    echo "To migrate existing NVM setup:"
    echo "  mise use --global node@\$(node --version 2>/dev/null | tr -d 'v' || echo 'lts')"
    return 1
}

# Set up Rust environment (sources ~/.cargo/env)
setup_rust() {
    if [[ -f "$HOME/.cargo/env" ]]; then
        source "$HOME/.cargo/env"
    fi
}

# Set up Python virtual environment auto-activation
# Automatically activates .venv when entering a directory (zsh only)
setup_python_venv() {
    # Auto-activate Python virtual environments
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        # Zsh version
        python_venv() {
            local venv_dir=".venv"
            if [[ -d "$venv_dir" ]]; then
                source "$venv_dir/bin/activate" 2>/dev/null
            else
                # Try to deactivate if we're in a virtual environment
                if [[ -n "${VIRTUAL_ENV:-}" ]]; then
                    deactivate 2>/dev/null || true
                fi
            fi
        }

        # Set up chpwd hook for zsh
        if ! typeset -f add-zsh-hook > /dev/null; then
            autoload -U add-zsh-hook
        fi
        add-zsh-hook chpwd python_venv
        python_venv  # Run once for current directory
    fi
}

# ================================================================
# SHELL-SPECIFIC ENHANCEMENTS
# ================================================================
# Shell plugins for autosuggestions and syntax highlighting.
# These significantly improve the interactive shell experience.

# Set up zsh-autosuggestions (fish-like suggestions from history)
# Shows faded suggestion as you type, accept with right arrow
setup_autosuggestions() {
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        # Zsh autosuggestions - check multiple locations
        local zsh_autosuggestions=""

        # Try system package manager locations first
        if [[ -f "/usr/share/zsh/plugins/zsh-autosuggestions/zsh-autosuggestions.zsh" ]]; then
            zsh_autosuggestions="/usr/share/zsh/plugins/zsh-autosuggestions/zsh-autosuggestions.zsh"
        elif [[ -f "/usr/share/zsh-autosuggestions/zsh-autosuggestions.zsh" ]]; then
            zsh_autosuggestions="/usr/share/zsh-autosuggestions/zsh-autosuggestions.zsh"
        elif [[ -n "$DOTFILES_HOMEBREW_PATH" && -f "$DOTFILES_HOMEBREW_PATH/share/zsh-autosuggestions/zsh-autosuggestions.zsh" ]]; then
            zsh_autosuggestions="$DOTFILES_HOMEBREW_PATH/share/zsh-autosuggestions/zsh-autosuggestions.zsh"
        fi

        if [[ -n "$zsh_autosuggestions" && -f "$zsh_autosuggestions" ]]; then
            source "$zsh_autosuggestions"
            ZSH_AUTOSUGGEST_HIGHLIGHT_STYLE="fg=8"
        fi
    elif [[ -n "${BASH_VERSION:-}" ]]; then
        # Bash-it or bash-preexec for enhanced bash experience
        if [[ -f "$HOME/.bash_it/bash_it.sh" ]]; then
            source "$HOME/.bash_it/bash_it.sh"
        fi
    fi
}

# Set up zsh-syntax-highlighting (fish-like command highlighting)
# Colors commands based on validity, quotes, etc.
setup_syntax_highlighting() {
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        # Zsh syntax highlighting - check multiple locations
        local zsh_highlighting=""

        # Try system package manager locations first
        if [[ -f "/usr/share/zsh/plugins/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh" ]]; then
            zsh_highlighting="/usr/share/zsh/plugins/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh"
        elif [[ -f "/usr/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh" ]]; then
            zsh_highlighting="/usr/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh"
        elif [[ -n "$DOTFILES_HOMEBREW_PATH" && -f "$DOTFILES_HOMEBREW_PATH/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh" ]]; then
            zsh_highlighting="$DOTFILES_HOMEBREW_PATH/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh"
        fi

        if [[ -n "$zsh_highlighting" && -f "$zsh_highlighting" ]]; then
            source "$zsh_highlighting"
        fi
    fi
}

# ================================================================
# COMPLETIONS SETUP
# ================================================================
# Basic completion initialization. Heavy completions (kubectl, helm,
# docker, aws, terraform, carapace, etc.) are lazy-loaded via
# lazy-completions.sh for faster startup on both bash and zsh.

# Set up shell completions (basic setup, heavy ones are lazy-loaded)
setup_completions() {
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        # Zsh completions
        autoload -U compinit && compinit

        # Additional completion directories
        if [[ -n "$DOTFILES_HOMEBREW_PATH" ]]; then
            fpath=("$DOTFILES_HOMEBREW_PATH/share/zsh/site-functions" "${fpath[@]}")
            fpath=("$DOTFILES_HOMEBREW_PATH/share/zsh-completions" "${fpath[@]}")
        fi

        # Note: kubectl, helm, docker, carapace completions are lazy-loaded
        # via lazy-completions.sh for faster shell startup

    elif [[ -n "${BASH_VERSION:-}" ]]; then
        # Bash completions
        if [[ -n "$DOTFILES_HOMEBREW_PATH" && -f "$DOTFILES_HOMEBREW_PATH/etc/profile.d/bash_completion.sh" ]]; then
            source "$DOTFILES_HOMEBREW_PATH/etc/profile.d/bash_completion.sh"
        elif [[ -f /etc/bash_completion ]]; then
            source /etc/bash_completion
        fi

        # Note: kubectl, helm, docker completions are lazy-loaded
        # via lazy-completions.sh for faster shell startup
    fi
}

# ================================================================
# DIAGNOSTIC FUNCTIONS
# ================================================================
# Functions for diagnosing tool installation and system health.

# Display comprehensive system health report
# Shows all tool categories with installation status and versions
# Usage: system_health
system_health() {
    echo "=== System Health Check ==="
    echo "OS: $DOTFILES_OS"
    echo "Architecture: $DOTFILES_ARCH"
    echo "Package Manager: $DOTFILES_PKG_MGR"
    echo

    echo "=== Core Tools ==="
    for tool in git curl wget; do
        tool_health "$tool"
    done
    echo

    echo "=== Modern CLI Tools ==="
    for tool in "${!MODERN_TOOLS[@]}"; do
        tool_health "$tool"
    done
    echo

    echo "=== Development Tools ==="
    for tool in nvim tmux jq; do
        tool_health "$tool"
    done
    echo

    echo "=== Language Runtimes ==="
    for tool in "${!LANG_TOOLS[@]}"; do
        tool_health "$tool"
    done
}

# List tools from recommended list that are not installed
# Usage: list_missing_tools
list_missing_tools() {
    echo "Missing tools that could be installed:"

    local missing=()
    for tool in "${!MODERN_TOOLS[@]}"; do
        if ! tool_exists "$tool"; then
            missing+=("$tool")
        fi
    done

    for tool in "${!DEV_TOOLS[@]}"; do
        if ! tool_exists "$tool"; then
            missing+=("$tool")
        fi
    done

    if [[ ${#missing[@]} -eq 0 ]]; then
        echo "All recommended tools are installed!"
    else
        printf "%s\n" "${missing[@]}"
        echo
        echo "Install missing tools with: install_missing_tools"
    fi
}

# Install all missing modern CLI tools
# Usage: install_missing_tools
install_missing_tools() {
    echo "Installing missing modern CLI tools..."
    install_modern_tools
}

# ================================================================
# ENVIRONMENT MANAGEMENT SETUP
# ================================================================
# Tools for managing per-directory environment variables.

# Set up direnv for automatic environment management via .envrc files
# Remember to run 'direnv allow' after creating/editing .envrc
setup_direnv() {
    if ! tool_exists "direnv"; then
        if [[ "$DOTFILES_DEBUG" == "1" ]]; then
            echo "direnv not found - install with: brew install direnv"
        fi
        return 1
    fi

    # direnv hook setup is handled in individual shell configs
    # to avoid shell detection complexity here
    if [[ "$DOTFILES_DEBUG" == "1" ]]; then
        echo "direnv available - hooks configured in shell configs"
    fi
}

# Set up atuin for shell history sync and search
# Provides fuzzy search, sync across machines, and statistics
# Press Ctrl-R for interactive history search
setup_atuin() {
    if ! tool_exists "atuin"; then
        if [[ "$DOTFILES_DEBUG" == "1" ]]; then
            echo "atuin not found - install with: brew install atuin or run bin/install-atuin.sh"
        fi
        return 1
    fi

    if [[ ! -d "$HOME/.atuin" ]]; then
        export PATH="$HOME/.atuin/bin:$PATH"
    fi

    if [[ "$DOTFILES_DEBUG" == "1" ]]; then
        echo "atuin available - hooks configured in shell configs"
    fi
}

# ================================================================
# MAIN SETUP FUNCTION
# ================================================================
# Orchestrates all tool initialization in the correct order.
# Called automatically on source unless DOTFILES_NO_AUTO_TOOL_SETUP is set.

# Set up all tools and enhancements - main entry point
setup_all_tools() {
    # Version managers (mise replaces NVM)
    setup_mise
    setup_rust

    # Modern alternatives
    setup_modern_alternatives

    # Interactive tools
    setup_fzf
    setup_zoxide
    setup_atuin

    # Modern CLI tools
    setup_lazygit
    setup_yazi

    # Environment management
    setup_direnv

    # Shell enhancements
    setup_autosuggestions
    setup_syntax_highlighting

    # Python environment
    setup_python_venv

    # Load lazy completions (instead of eager loading)
    # This significantly improves shell startup time
    if [[ -f "$SCRIPT_DIR/lazy-completions.sh" ]]; then
        source "$SCRIPT_DIR/lazy-completions.sh"
    fi

    # Load comprehensive completions for tools not lazy-loaded
    if [[ -f "$SCRIPT_DIR/completions.sh" ]]; then
        source "$SCRIPT_DIR/completions.sh"
    fi
}

# ================================================================
# INITIALIZATION
# ================================================================
# Automatic tool setup on source. Disable by setting
# DOTFILES_NO_AUTO_TOOL_SETUP=1 before sourcing.

if [[ -z "${DOTFILES_NO_AUTO_TOOL_SETUP}" ]]; then
    setup_all_tools
fi
