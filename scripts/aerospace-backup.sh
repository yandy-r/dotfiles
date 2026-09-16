#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[AEROSPACE-BACKUP]"

# Configuration
BACKUP_DIR="${BACKUP_DIR:-$HOME/.config/aerospace/backups}"
BACKUP_NAME="${BACKUP_NAME:-}"
INCLUDE_LAYOUT="${INCLUDE_LAYOUT:-true}"
INCLUDE_CONFIG="${INCLUDE_CONFIG:-true}"
COMPRESSION="${COMPRESSION:-gzip}"
MAX_BACKUPS="${MAX_BACKUPS:-10}"

usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS]

Create backups of AeroSpace configuration and layout state.

OPTIONS:
    --name NAME         Custom backup name (default: timestamp)
    --dir DIR           Backup directory (default: ~/.config/aerospace/backups)
    --no-layout         Don't backup current layout state
    --no-config         Don't backup configuration files
    --compression TYPE  Compression type: gzip, bzip2, none (default: gzip)
    --max-backups NUM   Maximum backups to keep (default: 10)
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    BACKUP_DIR          Directory for backups
    BACKUP_NAME         Custom backup name
    INCLUDE_LAYOUT      Backup layout state (true/false)
    INCLUDE_CONFIG      Backup config files (true/false)
    COMPRESSION         Compression type (gzip, bzip2, none)
    MAX_BACKUPS         Maximum backups to keep

EXAMPLES:
    $SCRIPT_NAME                        # Full backup with timestamp
    $SCRIPT_NAME --name "pre-update"    # Named backup
    $SCRIPT_NAME --no-layout            # Config only
    $SCRIPT_NAME --compression none     # No compression
EOF
    exit "${1:-0}"
}

log() {
    echo "${LOG_PREFIX} [$(date '+%Y-%m-%d %H:%M:%S')] $*" >&2
}

error_exit() {
    log "ERROR: $1" >&2
    exit "${2:-1}"
}

check_aerospace() {
    if ! command -v aerospace &>/dev/null; then
        error_exit "AeroSpace is not installed or not in PATH"
    fi
}

create_backup_dir() {
    if [[ ! -d "$BACKUP_DIR" ]]; then
        log "Creating backup directory: $BACKUP_DIR"
        mkdir -p "$BACKUP_DIR"
    fi
}

get_backup_name() {
    if [[ -n "$BACKUP_NAME" ]]; then
        echo "$BACKUP_NAME"
    else
        date '+%Y%m%d_%H%M%S'
    fi
}

backup_layout() {
    local backup_path="$1"
    local layout_file="$backup_path/layout.json"
    
    log "Backing up current layout state..."
    
    # Get current workspace and window layout
    local ts
    ts="$(date -Iseconds)"
    local current_ws
    current_ws="$(aerospace list-workspaces --focused 2>/dev/null || echo "unknown")"
    local layout_info='{
        "timestamp": "'"$ts"'",
        "current_workspace": "'"$current_ws"'",
        "workspaces": []
    }'
    
    # Create workspace information
    local temp_layout
    temp_layout=$(mktemp)
    echo "$layout_info" > "$temp_layout"
    
    # Get all workspaces and their windows
    if aerospace list-workspaces --all >/dev/null 2>&1; then
        local workspace_data="["
        local first=true
        
        while read -r workspace; do
            [[ -z "$workspace" ]] && continue
            
            if [[ "$first" != "true" ]]; then
                workspace_data+=","
            fi
            first=false
            
            local window_count
            window_count=$(aerospace list-windows --workspace "$workspace" 2>/dev/null | wc -l | xargs || echo "0")
            
            workspace_data+='{
                "name": "'$workspace'",
                "window_count": '$window_count',
                "windows": ['
            
            local window_first=true
            while read -r window_info; do
                [[ -z "$window_info" ]] && continue
                
                if [[ "$window_first" != "true" ]]; then
                    workspace_data+=","
                fi
                window_first=false
                
                # Escape quotes in window info
                window_info=$(echo "$window_info" | sed 's/"/\\"/g')
                workspace_data+='"'$window_info'"'
            done < <(aerospace list-windows --workspace "$workspace" --format '%{app-name}|%{window-title}' 2>/dev/null || true)
            
            workspace_data+=']}'
        done < <(aerospace list-workspaces --all 2>/dev/null || true)
        
        workspace_data+="]"
        
        # Update the JSON with workspace data
        if command -v jq &>/dev/null; then
            echo "$layout_info" | jq --argjson workspaces "$workspace_data" '.workspaces = $workspaces' > "$layout_file"
        else
            # Fallback without jq
            echo "$layout_info" | sed "s/\"workspaces\": \[\]$/\"workspaces\": $workspace_data/" > "$layout_file"
        fi
    else
        echo "$layout_info" > "$layout_file"
    fi
    
    rm -f "$temp_layout"
    log "Layout backed up to: $layout_file"
}

backup_config() {
    local backup_path="$1"
    
    log "Backing up configuration files..."
    
    # Primary config file
    if [[ -f "$HOME/.aerospace.toml" ]]; then
        cp "$HOME/.aerospace.toml" "$backup_path/aerospace.toml"
        log "Backed up ~/.aerospace.toml"
    fi
    
    # Dotfiles config if it's not the same file (e.g., not symlinked to ~/.aerospace.toml)
    if [[ -f "$HOME/.config/dotfiles/aerospace/aerospace.toml" ]] && \
       ! [[ "$HOME/.config/dotfiles/aerospace/aerospace.toml" -ef "$HOME/.aerospace.toml" ]]; then
        cp "$HOME/.config/dotfiles/aerospace/aerospace.toml" "$backup_path/aerospace-dotfiles.toml"
        log "Backed up dotfiles aerospace.toml"
    fi
    
    # Any other aerospace-related configs
    if [[ -d "$HOME/.config/aerospace" ]]; then
        cp -r "$HOME/.config/aerospace" "$backup_path/aerospace-config" 2>/dev/null || true
    fi
    
    # Backup script configurations if they exist
    if [[ -f "$HOME/.config/dotfiles/scripts/aerospace-status.sh" ]]; then
        mkdir -p "$backup_path/scripts"
        cp "$HOME/.config/dotfiles/scripts/aerospace-"*.sh "$backup_path/scripts/" 2>/dev/null || true
        log "Backed up helper scripts"
    fi
}

compress_backup() {
    local backup_path="$1"
    local backup_name="$2"
    
    case "$COMPRESSION" in
        "gzip")
            log "Compressing backup with gzip..."
            cd "$(dirname "$backup_path")"
            tar czf "${backup_name}.tar.gz" "$(basename "$backup_path")"
            rm -rf "$backup_path"
            log "Backup compressed to: ${backup_path}.tar.gz"
            ;;
        "bzip2")
            log "Compressing backup with bzip2..."
            cd "$(dirname "$backup_path")"
            tar cjf "${backup_name}.tar.bz2" "$(basename "$backup_path")"
            rm -rf "$backup_path"
            log "Backup compressed to: ${backup_path}.tar.bz2"
            ;;
        "none")
            log "Backup created without compression: $backup_path"
            ;;
        *)
            error_exit "Invalid compression type: $COMPRESSION"
            ;;
    esac
}

cleanup_old_backups() {
    log "Cleaning up old backups (keeping $MAX_BACKUPS)..."
    
    # Find all backup files/directories
    local backup_files=()
    mapfile -t backup_files < <(find "$BACKUP_DIR" -maxdepth 1 \( -type f -name "*.tar.gz" -o -name "*.tar.bz2" -o -type d -name "20*" \) | sort -r)
    
    if [[ ${#backup_files[@]} -gt $MAX_BACKUPS ]]; then
        local files_to_remove=("${backup_files[@]:$MAX_BACKUPS}")
        for file in "${files_to_remove[@]}"; do
            log "Removing old backup: $(basename "$file")"
            rm -rf "$file"
        done
    fi
}

create_metadata() {
    local backup_path="$1"
    local metadata_file="$backup_path/metadata.txt"
    
    cat > "$metadata_file" << EOF
AeroSpace Backup Metadata
========================
Created: $(date -Iseconds)
Host: $(hostname)
User: $(whoami)
AeroSpace Version: $(aerospace --version 2>/dev/null || echo "unknown")
Backup Type: $(if [[ "$INCLUDE_LAYOUT" == "true" && "$INCLUDE_CONFIG" == "true" ]]; then echo "full"; elif [[ "$INCLUDE_LAYOUT" == "true" ]]; then echo "layout-only"; else echo "config-only"; fi)
Compression: $COMPRESSION

Files Included:
$(find "$backup_path" -type f | sort)
EOF

    log "Metadata created: $metadata_file"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --name)
            BACKUP_NAME="$2"
            shift 2
            ;;
        --dir)
            BACKUP_DIR="$2"
            shift 2
            ;;
        --no-layout)
            INCLUDE_LAYOUT="false"
            shift
            ;;
        --no-config)
            INCLUDE_CONFIG="false"
            shift
            ;;
        --compression)
            COMPRESSION="$2"
            shift 2
            ;;
        --max-backups)
            MAX_BACKUPS="$2"
            shift 2
            ;;
        -h|--help)
            usage 0
            ;;
        *)
            error_exit "Unknown option: $1. Use --help for usage information."
            ;;
    esac
done

main() {
    if [[ "$INCLUDE_LAYOUT" == "false" && "$INCLUDE_CONFIG" == "false" ]]; then
        error_exit "At least one of --include-layout or --include-config must be enabled"
    fi
    
    check_aerospace
    create_backup_dir
    
    local backup_name
    backup_name=$(get_backup_name)
    local backup_path="$BACKUP_DIR/$backup_name"
    
    log "Creating backup: $backup_name"
    mkdir -p "$backup_path"
    
    if [[ "$INCLUDE_CONFIG" == "true" ]]; then
        backup_config "$backup_path"
    fi
    
    if [[ "$INCLUDE_LAYOUT" == "true" ]]; then
        backup_layout "$backup_path"
    fi
    
    create_metadata "$backup_path"
    
    if [[ "$COMPRESSION" != "none" ]]; then
        compress_backup "$backup_path" "$backup_name"
    fi
    
    cleanup_old_backups
    
    log "Backup completed successfully: $backup_name"
}

main "$@"