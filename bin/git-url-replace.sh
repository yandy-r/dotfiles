#!/usr/bin/env bash
# ================================================================
# Git URL Replacer - Replace Git Remote URLs
# ================================================================
# Searches for .git/config files in given paths and replaces
# git@ and https:// URLs to point to a different host

set -euo pipefail

# Source platform detection for cross-platform compatibility
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOTFILES_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

if [[ -f "$DOTFILES_ROOT/common/platform.sh" ]]; then
    # shellcheck source=../common/platform.sh
    source "$DOTFILES_ROOT/common/platform.sh"
fi

# ================================================================
# CONFIGURATION
# ================================================================

# Default configuration
DEFAULT_TARGET_HOST="gitea"
DEFAULT_USERNAME="yandy"

# Runtime configuration
TARGET_HOST="${DEFAULT_TARGET_HOST}"
TARGET_USERNAME="${DEFAULT_USERNAME}"
DRY_RUN=false
VERBOSE=false

# ================================================================
# HELPER FUNCTIONS
# ================================================================

print_usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS] [PATHS...]

Replace Git remote URLs in .git/config files to point to a different host.

OPTIONS:
    -d, --dry-run          Show what would be changed without making changes
    -v, --verbose          Show detailed output
    -h, --help             Show this help message
    -u, --user USERNAME    Target username (default: $DEFAULT_USERNAME)
    -H, --host HOST        Target host (default: $DEFAULT_TARGET_HOST)

ARGUMENTS:
    PATHS                  One or more paths to search (defaults to current directory)

EXAMPLES:
    # Replace in current directory (uses defaults: git@gitea:yandy)
    $(basename "$0")

    # Replace in specific directories (dry run)
    $(basename "$0") --dry-run ~/projects ~/work

    # Replace with custom host and user
    $(basename "$0") --host gitlab --user myuser ~/projects

    # Replace with verbose output
    $(basename "$0") -v ~/projects

DESCRIPTION:
    Searches recursively for .git/config files in the specified paths and
    replaces Git remote URLs to point to a different host.
    
    Converts both SSH and HTTPS URLs:
      git@github.com:user/repo.git     -> git@gitea:yandy/repo.git
      git@gitlab.com:user/repo.git     -> git@gitea:yandy/repo.git
      https://github.com/user/repo.git -> git@gitea:yandy/repo.git
      https://gitlab.com/user/repo.git -> git@gitea:yandy/repo.git

EOF
}

log_info() {
    echo "[INFO] $*"
}

log_success() {
    echo "[SUCCESS] $*"
}

log_error() {
    echo "[ERROR] $*" >&2
}

log_verbose() {
    if [[ "$VERBOSE" == true ]]; then
        echo "[VERBOSE] $*"
    fi
}

# ================================================================
# CORE FUNCTIONS
# ================================================================

# Find all .git/config files in given path
find_git_configs() {
    local search_path="$1"
    
    if [[ ! -d "$search_path" ]]; then
        log_error "Path does not exist: $search_path"
        return 1
    fi
    
    find "$search_path" -type f -path "*/.git/config" 2>/dev/null
}

# Check if file contains git URLs that need replacement
contains_git_urls() {
    local file="$1"
    # Check for git@ or https:// URLs
    grep -qE '(git@[^:]+:|https?://[^/]+/)' "$file" 2>/dev/null
}

# Replace git URLs in file
replace_in_file() {
    local file="$1"
    
    if [[ ! -f "$file" ]]; then
        log_error "File does not exist: $file"
        return 1
    fi
    
    if ! contains_git_urls "$file"; then
        log_verbose "Skipping (no git URLs found): $file"
        return 0
    fi
    
    # Create backup
    local backup_file="${file}.bak"
    cp "$file" "$backup_file"
    
    # Create temporary file for modifications
    local temp_file="${file}.tmp"
    local modified=false
    local -a changes=()
    
    # Process file line by line
    while IFS= read -r line; do
        local new_line="$line"
        local line_changed=false
        
        # Match git@ URLs: git@host:user/repo.git or git@host:user/repo
        if [[ "$line" =~ git@[^:]+:([^/]+)/(.+) ]]; then
            local old_user="${BASH_REMATCH[1]}"
            local repo="${BASH_REMATCH[2]}"
            repo="${repo%.git}"
            
            new_line=$(echo "$line" | sed -E "s|git@[^:]+:[^/]+/([^[:space:]]+)|git@${TARGET_HOST}:${TARGET_USERNAME}/\1|g")
            line_changed=true
        fi
        
        # Match https URLs: https://host/user/repo.git or https://host/user/repo
        if [[ "$line" =~ https?://[^/]+/([^/]+)/(.+) ]]; then
            local old_user="${BASH_REMATCH[1]}"
            local repo="${BASH_REMATCH[2]}"
            repo="${repo%.git}"
            
            new_line=$(echo "$line" | sed -E "s|https?://[^/]+/[^/]+/([^[:space:]]+)|git@${TARGET_HOST}:${TARGET_USERNAME}/\1|g")
            line_changed=true
        fi
        
        if [[ "$line_changed" == true ]] && [[ "$line" != "$new_line" ]]; then
            modified=true
            if [[ "$VERBOSE" == true ]]; then
                changes+=("  OLD: $line")
                changes+=("  NEW: $new_line")
            fi
        fi
        
        echo "$new_line"
    done < "$file" > "$temp_file"
    
    if [[ "$modified" == false ]]; then
        log_verbose "Skipping (no matching URLs found): $file"
        rm "$backup_file" "$temp_file"
        return 0
    fi
    
    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY RUN] Would replace in: $file"
        if [[ "$VERBOSE" == true ]] && [[ ${#changes[@]} -gt 0 ]]; then
            for change in "${changes[@]}"; do
                log_verbose "$change"
            done
        fi
        rm "$backup_file" "$temp_file"
        return 0
    fi
    
    # Apply changes
    if mv "$temp_file" "$file"; then
        log_success "Replaced in: $file"
        if [[ "$VERBOSE" == true ]] && [[ ${#changes[@]} -gt 0 ]]; then
            for change in "${changes[@]}"; do
                log_verbose "$change"
            done
        fi
        rm "$backup_file"
    else
        # Restore backup on error
        mv "$backup_file" "$file"
        rm -f "$temp_file"
        log_error "Failed to replace in: $file"
        return 1
    fi
}

# Process a single path
process_path() {
    local search_path="$1"
    local config_count=0
    local modified_count=0
    local process_failed=false
    
    log_info "Processing path: $search_path"
    log_verbose "Searching for .git/config files..."
    
    # Find all .git/config files
    while IFS= read -r config_file; do
        config_count=$((config_count + 1))
        log_verbose "Found: $config_file"
        
        # Count as modified if file has git URLs before processing
        local had_urls=false
        if contains_git_urls "$config_file"; then
            had_urls=true
        fi
        
        if replace_in_file "$config_file"; then
            if [[ "$had_urls" == true ]]; then
                modified_count=$((modified_count + 1))
            fi
        else
            process_failed=true
        fi
    done < <(find_git_configs "$search_path")
    
    if [[ $config_count -eq 0 ]]; then
        log_info "No .git/config files found in: $search_path"
    else
        log_info "Found $config_count .git/config file(s) in: $search_path"
        if [[ "$DRY_RUN" == true ]]; then
            log_info "Would modify $modified_count file(s)"
        else
            log_success "Modified $modified_count file(s)"
        fi
    fi
    
    # Return success unless a file replacement failed
    if [[ "$process_failed" == true ]]; then
        return 1
    fi
    return 0
}

# ================================================================
# MAIN FUNCTION
# ================================================================

main() {
    local paths=()
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d|--dry-run)
                DRY_RUN=true
                shift
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -u|--user)
                if [[ -z "${2:-}" ]] || [[ "$2" == -* ]]; then
                    log_error "Option --user requires a username argument"
                    exit 1
                fi
                TARGET_USERNAME="$2"
                shift 2
                ;;
            -H|--host)
                if [[ -z "${2:-}" ]] || [[ "$2" == -* ]]; then
                    log_error "Option --host requires a hostname argument"
                    exit 1
                fi
                TARGET_HOST="$2"
                shift 2
                ;;
            -h|--help)
                print_usage
                exit 0
                ;;
            -*)
                log_error "Unknown option: $1"
                print_usage
                exit 1
                ;;
            *)
                paths+=("$1")
                shift
                ;;
        esac
    done
    
    # Use current directory if no paths specified
    if [[ ${#paths[@]} -eq 0 ]]; then
        paths=(".")
    fi
    
    # Validate all paths first
    for path in "${paths[@]}"; do
        if [[ ! -d "$path" ]]; then
            log_error "Path does not exist: $path"
            exit 1
        fi
    done
    
    # Show configuration
    log_info "Git URL Replacer"
    log_info "Target Host: $TARGET_HOST"
    log_info "Target User: $TARGET_USERNAME"
    log_info "Target Format: git@${TARGET_HOST}:${TARGET_USERNAME}/<repo>.git"
    if [[ "$DRY_RUN" == true ]]; then
        log_info "Mode: DRY RUN (no changes will be made)"
    fi
    echo ""
    
    # Process each path
    local total_success=0
    local total_failed=0
    
    for path in "${paths[@]}"; do
        # Resolve to absolute path
        local abs_path
        abs_path="$(cd "$path" && pwd)"
        
        if process_path "$abs_path"; then
            total_success=$((total_success + 1))
        else
            total_failed=$((total_failed + 1))
        fi
        echo ""
    done
    
    # Summary
    log_info "Summary:"
    log_info "  Processed paths: ${#paths[@]}"
    log_success "  Successful: $total_success"
    if [[ $total_failed -gt 0 ]]; then
        log_error "  Failed: $total_failed"
        exit 1
    fi
    
    if [[ "$DRY_RUN" == true ]]; then
        echo ""
        log_info "This was a dry run. Re-run without --dry-run to apply changes."
    fi
}

# ================================================================
# ENTRY POINT
# ================================================================

main "$@"
