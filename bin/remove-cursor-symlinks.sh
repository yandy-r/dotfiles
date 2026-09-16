#!/usr/bin/env bash
# Manual use only. Not wired into bootstrap, playbooks, or CI.
# ================================================================
# Remove Cursor Symlinks Script
# ================================================================
# Safely removes Cursor-related symlinks from dotfiles management
# Only removes symlinks (not regular files) for safety
#
# Usage:
#   ./bin/remove-cursor-symlinks.sh           # Run removal
#   ./bin/remove-cursor-symlinks.sh -n        # Dry run (preview only)
#   ./bin/remove-cursor-symlinks.sh --dry-run # Same as -n
#   ./bin/remove-cursor-symlinks.sh -v        # Verbose output
#   ./bin/remove-cursor-symlinks.sh -h        # Show help
# ================================================================

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script options
DRY_RUN=false
VERBOSE=false

# Files and directories to check (relative to Cursor User directory)
CURSOR_ITEMS=(
    "settings.json"
    "keybindings.json"
    "extensions.json"
    "chat-settings.json"
    "mcp.json"
    "snippets"
    "agents"
    "commands"
    "workflows"
)

# Additional items in ~/.cursor
CURSOR_HOME_ITEMS=(
    "settings.json"
    "keybindings.json"
    "extensions.json"
    "chat-settings.json"
    "mcp.json"
    "cli-config.json"
    "snippets"
    "agents"
    "commands"
    "workflows"
)

# ================================================================
# Helper Functions
# ================================================================

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_verbose() {
    if [[ "$VERBOSE" == true ]]; then
        echo -e "${BLUE}[DEBUG]${NC} $1"
    fi
}

show_help() {
    cat << EOF
Remove Cursor Symlinks Script

Safely removes Cursor-related symlinks that were previously managed by dotfiles.
Only removes symlinks (not regular files) for safety.

Usage:
    $(basename "$0") [OPTIONS]

Options:
    -n, --dry-run    Preview changes without removing anything
    -v, --verbose    Show detailed output
    -h, --help       Show this help message

Examples:
    $(basename "$0")              # Remove symlinks
    $(basename "$0") -n           # Preview what would be removed
    $(basename "$0") -n -v        # Verbose preview

Targets:
    macOS:   ~/Library/Application Support/Cursor/User/
    Linux:   ~/.config/Cursor/User/
    Windows: ~/AppData/Roaming/Cursor/User/
    All:     ~/.cursor/

EOF
}

detect_os() {
    case "$(uname -s)" in
        Darwin)
            echo "macos"
            ;;
        Linux)
            if grep -qi microsoft /proc/version 2>/dev/null; then
                echo "wsl"
            else
                echo "linux"
            fi
            ;;
        MINGW*|MSYS*|CYGWIN*)
            echo "windows"
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

remove_if_symlink() {
    local path="$1"

    if [[ ! -e "$path" && ! -L "$path" ]]; then
        print_verbose "Does not exist: $path"
        return 0
    fi

    if [[ -L "$path" ]]; then
        local target
        target=$(readlink "$path" 2>/dev/null || echo "unknown")

        if [[ "$DRY_RUN" == true ]]; then
            print_warning "Would remove symlink: $path -> $target"
        else
            rm "$path"
            print_success "Removed symlink: $path -> $target"
        fi
    else
        print_warning "Skipping (not a symlink): $path"
    fi
}

process_directory() {
    local base_path="$1"
    local -n items=$2

    if [[ ! -d "$base_path" ]]; then
        print_verbose "Directory does not exist: $base_path"
        return 0
    fi

    print_info "Processing: $base_path"

    for item in "${items[@]}"; do
        remove_if_symlink "$base_path/$item"
    done
}

# ================================================================
# Main Logic
# ================================================================

main() {
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -n|--dry-run)
                DRY_RUN=true
                shift
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                print_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done

    local os
    os=$(detect_os)

    print_info "Detected OS: $os"

    if [[ "$DRY_RUN" == true ]]; then
        print_warning "DRY RUN MODE - No changes will be made"
        echo ""
    fi

    # Determine paths based on OS
    local cursor_user_path=""
    local home_dir="${HOME:-~}"

    case "$os" in
        macos)
            cursor_user_path="$home_dir/Library/Application Support/Cursor/User"
            ;;
        linux|wsl)
            cursor_user_path="$home_dir/.config/Cursor/User"
            ;;
        windows)
            cursor_user_path="$home_dir/AppData/Roaming/Cursor/User"
            ;;
        *)
            print_error "Unsupported OS: $os"
            exit 1
            ;;
    esac

    # Process platform-specific Cursor User directory
    if [[ -n "$cursor_user_path" ]]; then
        process_directory "$cursor_user_path" CURSOR_ITEMS
    fi

    # Process ~/.cursor directory (common to all platforms)
    local cursor_home_path="$home_dir/.cursor"
    process_directory "$cursor_home_path" CURSOR_HOME_ITEMS

    echo ""
    if [[ "$DRY_RUN" == true ]]; then
        print_info "Dry run complete. Run without -n to apply changes."
    else
        print_success "Cursor symlink removal complete!"
    fi
}

main "$@"
