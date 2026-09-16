#!/usr/bin/env bash
# ================================================================
# Cross-Platform Detection and Configuration
# ================================================================
#
# Purpose:
#   Provides comprehensive platform detection for cross-platform dotfiles
#   compatibility. Detects OS, architecture, container environment, and
#   package managers to enable platform-aware configuration loading.
#
# Dependencies:
#   - None (standalone, foundational script)
#
# Exported Variables:
#   DOTFILES_OS          - Detected OS (macos, debian, redhat, arch, linux, wsl, windows)
#   DOTFILES_ARCH        - CPU architecture (x86_64, arm64, arm)
#   DOTFILES_CONTAINER   - Container type (docker, kubernetes, wsl, none)
#   DOTFILES_PKG_MGR     - Package manager (homebrew, apt, dnf, yum, pacman, zypper)
#   DOTFILES_HOMEBREW_PATH - Homebrew installation prefix
#   DOTFILES_IS_SSH      - "true" if running in SSH session
#   DOTFILES_USE_OSC52   - "true" if OSC 52 clipboard should be used
#   DOTFILES_COPY_CMD    - Platform-appropriate clipboard copy command
#   DOTFILES_PASTE_CMD   - Platform-appropriate clipboard paste command
#   DOTFILES_OPEN_CMD    - Platform-appropriate file/URL opener command
#   XDG_CONFIG_HOME      - XDG config directory (default: ~/.config)
#   XDG_DATA_HOME        - XDG data directory (default: ~/.local/share)
#   XDG_CACHE_HOME       - XDG cache directory (default: ~/.cache)
#   XDG_STATE_HOME       - XDG state directory (default: ~/.local/state)
#
# Usage:
#   source common/platform.sh
#   # Variables are automatically exported
#   echo "Running on: $DOTFILES_OS ($DOTFILES_ARCH)"
#
# Disabling Auto-Setup:
#   export DOTFILES_NO_AUTO_SETUP=1
#   source common/platform.sh
#   # Then call setup_platform_env manually when ready
#
# ================================================================

# ================================================================
# CORE DETECTION FUNCTIONS
# ================================================================
# These functions detect the runtime environment characteristics.
# Each returns a standardized string for consistent cross-platform logic.

# Detect operating system
# Returns: macos, debian, redhat, arch, suse, linux, wsl, windows, or unknown
detect_os() {
    case "$(uname -s)" in
        Darwin*)
            echo "macos"
            ;;
        Linux*)
            if [[ -f /etc/os-release ]]; then
                . /etc/os-release
                case "$ID" in
                    ubuntu|debian) echo "debian" ;;
                    fedora|fedora-asahi-remix|asahi-fedora-remix|rhel|centos|rocky|almalinux) echo "redhat" ;;
                    arch|cachyos|manjaro|endeavouros) echo "arch" ;;
                    opensuse*|sles) echo "suse" ;;
                    *)
                        # Fallback: use ID_LIKE for unknown derivatives
                        case "${ID_LIKE:-}" in
                            *debian*|*ubuntu*) echo "debian" ;;
                            *fedora*|*rhel*) echo "redhat" ;;
                            *arch*) echo "arch" ;;
                            *suse*) echo "suse" ;;
                            *) echo "linux" ;;
                        esac
                        ;;
                esac
            elif [[ -f /proc/version ]] && grep -qi microsoft /proc/version; then
                echo "wsl"
            else
                echo "linux"
            fi
            ;;
        CYGWIN*|MINGW*|MSYS*)
            echo "windows"
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

# Print the per-OS module search path, most-generic-first, one relative
# directory name per line. This is the single source of truth for where
# common/{aliases,exports,functions}.sh look for their platform-specific
# siblings; nothing else should build those paths by hand.
#
# The first line is always the OS *family* directory (macos or linux). When the
# detected OS is a distro family within Linux, a second line names the distro
# overlay directory. Overlays layer on top of the family file instead of
# replacing it, so a future common/redhat/aliases.sh would refine
# common/linux/aliases.sh rather than shadow it. Only common/macos/ and
# common/linux/ exist today; the overlay slot is real, just unused.
#
# Args: $1 - OS token to resolve (default: $DOTFILES_OS, else detect_os)
# Returns: nothing for platforms with no module directory (windows, unknown)
os_module_dirs() {
    local os_type="${1:-${DOTFILES_OS:-$(detect_os)}}"

    case "$os_type" in
        macos)
            echo "macos"
            ;;
        debian|redhat|arch|suse|wsl|linux)
            echo "linux"
            if [[ "$os_type" != "linux" ]]; then
                echo "$os_type"
            fi
            ;;
    esac
}

# Get raw distro ID from /etc/os-release (e.g., "ubuntu", "cachyos", "arch")
# Used by scripts that need the specific distro name for repo URL construction
get_distro_name() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        echo "${ID:-unknown}"
    else
        echo "unknown"
    fi
}

# Detect CPU architecture
# Returns: x86_64, arm64, arm, or raw uname output for unknown architectures
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "x86_64" ;;
        aarch64|arm64) echo "arm64" ;;
        armv7l|armv6l) echo "arm" ;;
        *) echo "$(uname -m)" ;;
    esac
}

# Detect if running in a container environment
# Returns: docker, kubernetes, wsl, or none
detect_container() {
    if [[ -f /.dockerenv ]] || grep -q docker /proc/1/cgroup 2>/dev/null; then
        echo "docker"
    elif [[ -n "${KUBERNETES_SERVICE_HOST:-}" ]]; then
        echo "kubernetes"
    elif [[ -n "${WSL_DISTRO_NAME:-}" ]] || [[ -n "${WSL_INTEROP:-}" ]]; then
        echo "wsl"
    else
        echo "none"
    fi
}

# ================================================================
# PACKAGE MANAGER DETECTION
# ================================================================
# Identifies the primary package manager available on the system.
# Used for automated tool installation and dependency management.

# Detect the primary package manager
# Returns: homebrew, apt, dnf, yum, pacman, zypper, or unknown
# Note: Homebrew is prioritized when available (even on Linux)
detect_package_manager() {
    if command -v brew &> /dev/null; then
        echo "homebrew"
    elif command -v apt &> /dev/null; then
        echo "apt"
    elif command -v dnf &> /dev/null; then
        echo "dnf"
    elif command -v yum &> /dev/null; then
        echo "yum"
    elif command -v pacman &> /dev/null; then
        echo "pacman"
    elif command -v zypper &> /dev/null; then
        echo "zypper"
    else
        echo "unknown"
    fi
}

# Get Homebrew installation path based on OS and architecture
# Returns: Homebrew prefix path or empty string if not available
# macOS ARM64:  /opt/homebrew
# macOS x86_64: /usr/local
# Linux:        /home/linuxbrew/.linuxbrew
get_homebrew_path() {
    local os_type
    os_type="$(detect_os)"
    local arch
    arch="$(detect_arch)"
    
    if command -v brew &> /dev/null; then
        brew --prefix
    elif [[ "$os_type" == "macos" ]]; then
        if [[ "$arch" == "arm64" ]]; then
            echo "/opt/homebrew"
        else
            echo "/usr/local"
        fi
    elif [[ "$os_type" =~ (debian|redhat|arch|suse|linux) ]]; then
        echo "/home/linuxbrew/.linuxbrew"
    fi
}

# ================================================================
# TOOL DETECTION
# ================================================================
# Utility functions for checking tool/file/directory availability.
# These are used throughout the dotfiles for conditional configuration.

# Check if command exists in PATH
# Usage: has_command git && echo "Git installed"
has_command() {
    command -v "$1" &> /dev/null
}

# Check if file exists
has_file() {
    [[ -f "$1" ]]
}

# Check if directory exists
has_dir() {
    [[ -d "$1" ]]
}

# Get command path
get_command_path() {
    if has_command "$1"; then
        command -v "$1"
    else
        echo ""
    fi
}

# ================================================================
# PATH HELPERS
# ================================================================
# Functions for safe PATH manipulation. Prevents duplicates and
# validates directory existence before adding to PATH.

# Add directory to PATH if it exists and is not already present
# Args: $1 = path, $2 = position ("start" or "end", default: "end")
# Usage: add_to_path "/usr/local/bin" "start"
add_to_path() {
    local new_path="$1"
    local position="${2:-end}"  # start or end
    
    if [[ -d "$new_path" ]] && [[ ":$PATH:" != *":$new_path:"* ]]; then
        if [[ "$position" == "start" ]]; then
            export PATH="$new_path:$PATH"
        else
            export PATH="$PATH:$new_path"
        fi
    fi
}

# Add to PATH at start if directory exists
prepend_path() {
    add_to_path "$1" "start"
}

# Add to PATH at end if directory exists
append_path() {
    add_to_path "$1" "end"
}

# ================================================================
# PLATFORM-SPECIFIC SETTINGS
# ================================================================
# Functions that return platform-appropriate commands for common
# operations like clipboard access and file opening.

# Get platform-appropriate clipboard copy command
# Returns: pbcopy (macOS), clip.exe (WSL), xclip/xsel (Linux), or cat (fallback)
get_copy_command() {
    local os_type
    os_type="$(detect_os)"
    case "$os_type" in
        macos) echo "pbcopy" ;;
        wsl) echo "clip.exe" ;;
        *) 
            if has_command xclip; then
                echo "xclip -selection clipboard"
            elif has_command xsel; then
                echo "xsel --clipboard --input"
            else
                echo "cat"
            fi
            ;;
    esac
}

# Get platform-appropriate clipboard paste command
# Returns: pbpaste (macOS), powershell Get-Clipboard (WSL), xclip/xsel (Linux)
get_paste_command() {
    local os_type
    os_type="$(detect_os)"
    case "$os_type" in
        macos) echo "pbpaste" ;;
        wsl) echo "powershell.exe Get-Clipboard" ;;
        *)
            if has_command xclip; then
                echo "xclip -selection clipboard -o"
            elif has_command xsel; then
                echo "xsel --clipboard --output"
            else
                echo "echo '(no clipboard available)'"
            fi
            ;;
    esac
}

# Get platform-appropriate command to open files/URLs
# Returns: open (macOS), explorer.exe (WSL), xdg-open (Linux)
get_open_command() {
    local os_type
    os_type="$(detect_os)"
    case "$os_type" in
        macos) echo "open" ;;
        wsl) echo "explorer.exe" ;;
        *) echo "xdg-open" ;;
    esac
}

# ================================================================
# ENVIRONMENT SETUP
# ================================================================
# Main initialization function that exports all platform detection
# results as environment variables for use by other scripts.

# Initialize platform environment and export all detection results
# Called automatically on source unless DOTFILES_NO_AUTO_SETUP is set
# Exports: DOTFILES_OS, DOTFILES_ARCH, DOTFILES_CONTAINER, etc.
setup_platform_env() {
    # Export detection results
    export DOTFILES_OS
    DOTFILES_OS="$(detect_os)"
    export DOTFILES_ARCH
    DOTFILES_ARCH="$(detect_arch)"
    export DOTFILES_CONTAINER
    DOTFILES_CONTAINER="$(detect_container)"
    export DOTFILES_PKG_MGR
    DOTFILES_PKG_MGR="$(detect_package_manager)"
    export DOTFILES_HOMEBREW_PATH
    DOTFILES_HOMEBREW_PATH="$(get_homebrew_path)"

    # Export SSH and clipboard detection
    if is_ssh; then
        export DOTFILES_IS_SSH="true"
    else
        export DOTFILES_IS_SSH="false"
    fi

    if should_use_osc52; then
        export DOTFILES_USE_OSC52="true"
    else
        export DOTFILES_USE_OSC52="false"
    fi

    # Export platform commands
    export DOTFILES_COPY_CMD
    DOTFILES_COPY_CMD="$(get_copy_command)"
    export DOTFILES_PASTE_CMD
    DOTFILES_PASTE_CMD="$(get_paste_command)"
    export DOTFILES_OPEN_CMD
    DOTFILES_OPEN_CMD="$(get_open_command)"
    
    # Set up common environment variables
    export XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
    export XDG_DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
    export XDG_CACHE_HOME="${XDG_CACHE_HOME:-$HOME/.cache}"
    export XDG_STATE_HOME="${XDG_STATE_HOME:-$HOME/.local/state}"
    
    # Create XDG directories if they don't exist
    mkdir -p "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME" "$XDG_STATE_HOME"
}

# ================================================================
# UTILITY FUNCTIONS
# ================================================================
# Convenience functions for platform checking and debugging.

# Print all detected platform information (useful for debugging)
# Usage: print_platform_info
print_platform_info() {
    echo "Platform Information:"
    echo "  OS: $(detect_os)"
    echo "  Architecture: $(detect_arch)"
    echo "  Container: $(detect_container)"
    echo "  Package Manager: $(detect_package_manager)"
    echo "  Homebrew Path: $(get_homebrew_path)"
    echo "  Copy Command: $(get_copy_command)"
    echo "  Paste Command: $(get_paste_command)"
    echo "  Open Command: $(get_open_command)"
}

# Platform check functions - return 0 (true) or 1 (false)
# Usage: is_macos && echo "On macOS"
is_macos() {
    [[ "$(detect_os)" == "macos" ]]
}

is_linux() {
    [[ "$(detect_os)" =~ (debian|redhat|arch|suse|linux) ]]
}

is_wsl() {
    [[ "$(detect_os)" == "wsl" ]] || [[ "$(detect_container)" == "wsl" ]]
}

is_container() {
    [[ "$(detect_container)" != "none" ]]
}

# Architecture check functions
is_arm64() {
    [[ "$(detect_arch)" == "arm64" ]]
}

is_x86_64() {
    [[ "$(detect_arch)" == "x86_64" ]]
}

# Check if running over SSH (detected via SSH_* environment variables)
is_ssh() {
    [[ -n "${SSH_CLIENT:-}" ]] || [[ -n "${SSH_TTY:-}" ]] || [[ -n "${SSH_CONNECTION:-}" ]]
}

# Check if OSC 52 clipboard escape sequence should be used
# OSC 52 allows copying to local clipboard over SSH in supported terminals
# Returns 0 if SSH session + capable terminal detected
should_use_osc52() {
    # Use OSC 52 if we're over SSH and have a capable terminal
    if is_ssh; then
        # Check if terminal supports OSC 52
        case "${TERM_PROGRAM:-}" in
            iTerm.app|WezTerm|Alacritty|vscode) return 0 ;;
        esac
        case "${TERM:-}" in
            *256color|xterm*|screen*|tmux*|alacritty*|wezterm*) return 0 ;;
        esac
    fi
    return 1
}

# ================================================================
# INITIALIZATION
# ================================================================
# Automatically set up platform environment when this script is sourced.
# Disable by setting DOTFILES_NO_AUTO_SETUP=1 before sourcing.

if [[ -z "${DOTFILES_NO_AUTO_SETUP:-}" ]]; then
    setup_platform_env
fi