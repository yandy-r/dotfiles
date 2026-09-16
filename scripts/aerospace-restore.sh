#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[AEROSPACE-RESTORE]"

# Configuration
BACKUP_DIR="${BACKUP_DIR:-$HOME/.config/aerospace/backups}"
BACKUP_NAME="${BACKUP_NAME:-}"
RESTORE_CONFIG="${RESTORE_CONFIG:-true}"
RESTORE_LAYOUT="${RESTORE_LAYOUT:-false}"
DRY_RUN="${DRY_RUN:-false}"
FORCE="${FORCE:-false}"

usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS] [BACKUP_NAME]

Restore AeroSpace configuration and layout from backup.

OPTIONS:
    --backup NAME       Backup name to restore (or use interactive selection)
    --dir DIR           Backup directory (default: ~/.config/aerospace/backups)
    --layout            Restore layout state (experimental)
    --no-config         Don't restore configuration files
    --dry-run           Show what would be restored without doing it
    --force             Overwrite existing files without confirmation
    --list              List available backups
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    BACKUP_DIR          Directory containing backups
    BACKUP_NAME         Backup name to restore
    RESTORE_CONFIG      Restore config files (true/false)
    RESTORE_LAYOUT      Restore layout state (true/false)
    DRY_RUN            Enable dry-run mode (true/false)
    FORCE              Force overwrite (true/false)

EXAMPLES:
    $SCRIPT_NAME                        # Interactive backup selection
    $SCRIPT_NAME pre-update             # Restore specific backup
    $SCRIPT_NAME --layout --backup latest  # Restore with layout
    $SCRIPT_NAME --dry-run              # Preview restore operation
    $SCRIPT_NAME --list                 # List available backups
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

list_backups() {
    if [[ ! -d "$BACKUP_DIR" ]]; then
        echo "No backup directory found: $BACKUP_DIR"
        return 1
    fi
    
    echo "Available Backups:"
    echo "=================="
    
    local backups=()
    mapfile -t backups < <(find "$BACKUP_DIR" -maxdepth 1 \( -type f -name "*.tar.gz" -o -name "*.tar.bz2" -o -type d -name "20*" \) | sort -r)
    
    if [[ ${#backups[@]} -eq 0 ]]; then
        echo "No backups found in $BACKUP_DIR"
        return 1
    fi
    
    for backup in "${backups[@]}"; do
        local backup_name
        backup_name=$(basename "$backup" | sed 's/\.\(tar\.gz\|tar\.bz2\)$//')
        
        local backup_path="$backup"
        local metadata_file=""
        
        # Handle compressed backups
        if [[ "$backup" == *.tar.* ]]; then
            local temp_dir
            temp_dir=$(mktemp -d)
            if [[ "$backup" == *.tar.gz ]]; then
                tar xzf "$backup" -C "$temp_dir" 2>/dev/null || continue
            elif [[ "$backup" == *.tar.bz2 ]]; then
                tar xjf "$backup" -C "$temp_dir" 2>/dev/null || continue
            fi
            metadata_file="$temp_dir/$backup_name/metadata.txt"
        else
            metadata_file="$backup/metadata.txt"
        fi
        
        echo "  $backup_name"
        
        # Show metadata if available
        if [[ -f "$metadata_file" ]]; then
            local created
            created=$(grep "Created:" "$metadata_file" 2>/dev/null | cut -d' ' -f2- || echo "unknown")
            local backup_type
            backup_type=$(grep "Backup Type:" "$metadata_file" 2>/dev/null | cut -d' ' -f3- || echo "unknown")
            echo "    Created: $created"
            echo "    Type: $backup_type"
        fi
        
        # Cleanup temp directory if created
        if [[ "$backup" == *.tar.* ]] && [[ -d "$temp_dir" ]]; then
            rm -rf "$temp_dir"
        fi
        echo
    done
}

select_backup() {
    local backups=()
    mapfile -t backups < <(find "$BACKUP_DIR" -maxdepth 1 \( -type f -name "*.tar.gz" -o -name "*.tar.bz2" -o -type d -name "20*" \) | sort -r)
    
    if [[ ${#backups[@]} -eq 0 ]]; then
        error_exit "No backups found in $BACKUP_DIR"
    fi
    
    echo "Select a backup to restore:"
    echo "=========================="
    
    local i=1
    for backup in "${backups[@]}"; do
        local backup_name
        backup_name=$(basename "$backup" | sed 's/\.\(tar\.gz\|tar\.bz2\)$//')
        echo "$i) $backup_name"
        ((i++))
    done
    
    echo
    read -rp "Enter number (1-${#backups[@]}): " selection
    
    if [[ "$selection" =~ ^[0-9]+$ ]] && [[ $selection -ge 1 ]] && [[ $selection -le ${#backups[@]} ]]; then
        local selected_backup="${backups[$((selection-1))]}"
        basename "$selected_backup" | sed 's/\.\(tar\.gz\|tar\.bz2\)$//'
    else
        error_exit "Invalid selection: $selection"
    fi
}

extract_backup() {
    local backup_name="$1"
    local backup_file=""
    local temp_dir
    temp_dir=$(mktemp -d)
    
    # Find the backup file
    if [[ -d "$BACKUP_DIR/$backup_name" ]]; then
        backup_file="$BACKUP_DIR/$backup_name"
        cp -r "$backup_file" "$temp_dir/"
    elif [[ -f "$BACKUP_DIR/$backup_name.tar.gz" ]]; then
        backup_file="$BACKUP_DIR/$backup_name.tar.gz"
        tar xzf "$backup_file" -C "$temp_dir"
    elif [[ -f "$BACKUP_DIR/$backup_name.tar.bz2" ]]; then
        backup_file="$BACKUP_DIR/$backup_name.tar.bz2"
        tar xjf "$backup_file" -C "$temp_dir"
    else
        rm -rf "$temp_dir"
        error_exit "Backup not found: $backup_name"
    fi
    
    echo "$temp_dir/$backup_name"
}

restore_config() {
    local backup_path="$1"
    
    log "Restoring configuration files..."
    
    # Main aerospace config
    if [[ -f "$backup_path/aerospace.toml" ]]; then
        local target="$HOME/.aerospace.toml"
        
        if [[ "$DRY_RUN" == "true" ]]; then
            log "DRY-RUN: Would restore $target"
        else
            if [[ -f "$target" ]] && [[ "$FORCE" != "true" ]]; then
                read -rp "Overwrite existing $target? [y/N] " confirm
                [[ "$confirm" =~ ^[yY] ]] || return 0
            fi
            
            cp "$backup_path/aerospace.toml" "$target"
            log "Restored: $target"
        fi
    fi
    
    # Dotfiles config
    if [[ -f "$backup_path/aerospace-dotfiles.toml" ]]; then
        local target="$HOME/.config/dotfiles/aerospace/aerospace.toml"
        local target_dir
        target_dir=$(dirname "$target")
        
        if [[ "$DRY_RUN" == "true" ]]; then
            log "DRY-RUN: Would restore $target"
        else
            if [[ -f "$target" ]] && [[ "$FORCE" != "true" ]]; then
                read -rp "Overwrite existing $target? [y/N] " confirm
                [[ "$confirm" =~ ^[yY] ]] || return 0
            fi
            
            mkdir -p "$target_dir"
            cp "$backup_path/aerospace-dotfiles.toml" "$target"
            log "Restored: $target"
        fi
    fi
    
    # Additional config directory
    if [[ -d "$backup_path/aerospace-config" ]]; then
        local target="$HOME/.config/aerospace"
        
        if [[ "$DRY_RUN" == "true" ]]; then
            log "DRY-RUN: Would restore $target/"
        else
            if [[ -d "$target" ]] && [[ "$FORCE" != "true" ]]; then
                read -rp "Overwrite existing $target/? [y/N] " confirm
                [[ "$confirm" =~ ^[yY] ]] || return 0
            fi
            
            mkdir -p "$(dirname "$target")"
            cp -r "$backup_path/aerospace-config" "$target"
            log "Restored: $target/"
        fi
    fi
    
    # Helper scripts
    if [[ -d "$backup_path/scripts" ]]; then
        local target_dir="$HOME/.config/dotfiles/scripts"
        
        if [[ "$DRY_RUN" == "true" ]]; then
            log "DRY-RUN: Would restore scripts to $target_dir/"
        else
            mkdir -p "$target_dir"
            for script in "$backup_path/scripts/"*; do
                local script_name
                script_name=$(basename "$script")
                local target="$target_dir/$script_name"
                
                if [[ -f "$target" ]] && [[ "$FORCE" != "true" ]]; then
                    read -rp "Overwrite existing $target? [y/N] " confirm
                    [[ "$confirm" =~ ^[yY] ]] || continue
                fi
                
                cp "$script" "$target"
                chmod +x "$target"
                log "Restored script: $target"
            done
        fi
    fi
}

restore_layout() {
    local backup_path="$1"
    
    if [[ ! -f "$backup_path/layout.json" ]]; then
        log "No layout information found in backup"
        return 0
    fi
    
    log "Restoring layout state (experimental)..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would attempt to restore layout from $backup_path/layout.json"
        return 0
    fi
    
    # Parse layout JSON and attempt to restore
    if command -v jq &>/dev/null; then
        local target_workspace
        target_workspace=$(jq -r '.current_workspace' "$backup_path/layout.json" 2>/dev/null || echo "")
        
        if [[ -n "$target_workspace" ]] && [[ "$target_workspace" != "null" ]]; then
            log "Switching to workspace: $target_workspace"
            aerospace workspace "$target_workspace" 2>/dev/null || log "Warning: Failed to switch to workspace $target_workspace"
        fi
        
        # TODO: More sophisticated layout restoration could be implemented here
        # This would require workspace recreation and window placement
        log "Layout restoration is experimental and limited to workspace switching"
    else
        log "jq not available, skipping layout restoration"
    fi
}

cleanup_temp() {
    local temp_dir="$1"
    if [[ -d "$temp_dir" ]] && [[ "$temp_dir" == /tmp/* ]]; then
        rm -rf "$temp_dir"
    fi
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --backup)
            BACKUP_NAME="$2"
            shift 2
            ;;
        --dir)
            BACKUP_DIR="$2"
            shift 2
            ;;
        --layout)
            RESTORE_LAYOUT="true"
            shift
            ;;
        --no-config)
            RESTORE_CONFIG="false"
            shift
            ;;
        --dry-run)
            DRY_RUN="true"
            shift
            ;;
        --force)
            FORCE="true"
            shift
            ;;
        --list)
            list_backups
            exit 0
            ;;
        -h|--help)
            usage 0
            ;;
        -*)
            error_exit "Unknown option: $1. Use --help for usage information."
            ;;
        *)
            BACKUP_NAME="$1"
            shift
            ;;
    esac
done

main() {
    if [[ "$RESTORE_CONFIG" == "false" && "$RESTORE_LAYOUT" == "false" ]]; then
        error_exit "At least one of --restore-config or --restore-layout must be enabled"
    fi
    
    check_aerospace
    
    if [[ ! -d "$BACKUP_DIR" ]]; then
        error_exit "Backup directory not found: $BACKUP_DIR"
    fi
    
    # Select backup if not provided
    if [[ -z "$BACKUP_NAME" ]]; then
        BACKUP_NAME=$(select_backup)
    fi
    
    log "Restoring backup: $BACKUP_NAME"
    
    # Extract backup to temp directory
    local backup_path
    backup_path=$(extract_backup "$BACKUP_NAME")
    # shellcheck disable=SC2064  # expand $backup_path now, deliberately
    trap "cleanup_temp \"$(dirname "$backup_path")\"" EXIT
    
    # Show what will be restored
    if [[ -f "$backup_path/metadata.txt" ]]; then
        log "Backup information:"
        cat "$backup_path/metadata.txt"
        echo
    fi
    
    if [[ "$DRY_RUN" != "true" ]] && [[ "$FORCE" != "true" ]]; then
        read -rp "Continue with restore? [y/N] " confirm
        [[ "$confirm" =~ ^[yY] ]] || exit 0
    fi
    
    # Perform restore operations
    if [[ "$RESTORE_CONFIG" == "true" ]]; then
        restore_config "$backup_path"
    fi
    
    if [[ "$RESTORE_LAYOUT" == "true" ]]; then
        restore_layout "$backup_path"
    fi
    
    if [[ "$DRY_RUN" != "true" ]] && [[ "$RESTORE_CONFIG" == "true" ]]; then
        log "Reloading AeroSpace configuration..."
        aerospace reload-config 2>/dev/null || log "Warning: Failed to reload configuration"
    fi
    
    log "Restore completed successfully"
}

main "$@"