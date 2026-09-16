#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[CLEANUP-TEMP]"

CLEANUP_BASE_PATH="${CLEANUP_BASE_PATH:-/mnt/data}"
DRY_RUN="${DRY_RUN:-false}"
VERBOSE="${VERBOSE:-false}"
EXCLUDE_DIRS="${EXCLUDE_DIRS:-}"

usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS]

Clean up temporary media processing directories.

OPTIONS:
    -d, --dry-run       Show what would be deleted without actually deleting
    -v, --verbose       Enable verbose output
    -h, --help          Show this help message
    -e, --exclude DIR   Exclude directory from cleanup (can be used multiple times)
    -p, --path PATH     Base path for cleanup (default: /mnt/data)

ENVIRONMENT VARIABLES:
    CLEANUP_BASE_PATH   Base path for cleanup directories
    DRY_RUN            Enable dry-run mode (true/false)
    VERBOSE            Enable verbose mode (true/false)
    EXCLUDE_DIRS       Comma-separated list of directories to exclude

EXAMPLES:
    $SCRIPT_NAME --dry-run
    $SCRIPT_NAME --verbose --exclude movies --exclude music
    CLEANUP_BASE_PATH=/custom/path $SCRIPT_NAME
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

is_excluded() {
    local dir="$1"
    local exclude_list="$2"
    
    if [[ -n "$exclude_list" ]]; then
        IFS=',' read -ra excludes <<< "$exclude_list"
        for exclude in "${excludes[@]}"; do
            if [[ "$dir" == *"$exclude"* ]]; then
                return 0
            fi
        done
    fi
    return 1
}

calculate_size() {
    local path="$1"
    if [[ -d "$path" ]]; then
        du -sh "$path" 2>/dev/null | cut -f1 || echo "0B"
    else
        echo "0B"
    fi
}

cleanup_directory() {
    local target="$1"
    local size_before
    
    if [[ ! -d "$target" ]] && [[ ! -f "$target" ]]; then
        [[ "$VERBOSE" == "true" ]] && log "Skipping $target (does not exist)"
        return 0
    fi
    
    if is_excluded "$target" "$EXCLUDE_DIRS"; then
        log "Excluding $target (in exclude list)"
        return 0
    fi
    
    size_before=$(calculate_size "$target")
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would remove $target (size: $size_before)"
        return 0
    fi
    
    log "Removing $target (size: $size_before)"
    
    if sudo rm -rf "$target" 2>/dev/null; then
        log "Successfully removed $target"
        echo "$size_before" >> "$temp_file"
    else
        log "Failed to remove $target"
        return 1
    fi
}

main() {
    local temp_file
    temp_file=$(mktemp)
    trap 'rm -f "$temp_file"' EXIT
    
    local cleanup_targets=(
        "$CLEANUP_BASE_PATH/usenet/anime/*"
        "$CLEANUP_BASE_PATH/usenet/audiobooks/*"
        "$CLEANUP_BASE_PATH/usenet/comics/*"
        "$CLEANUP_BASE_PATH/usenet/ebooks/*"
        "$CLEANUP_BASE_PATH/usenet/incomplete/*"
        "$CLEANUP_BASE_PATH/usenet/movies/*"
        "$CLEANUP_BASE_PATH/usenet/movies-4k/*"
        "$CLEANUP_BASE_PATH/usenet/music/*"
        "$CLEANUP_BASE_PATH/usenet/nzb/*"
        "$CLEANUP_BASE_PATH/usenet/prowlarr/*"
        "$CLEANUP_BASE_PATH/usenet/queue/*"
        "$CLEANUP_BASE_PATH/usenet/tmp/*"
        "$CLEANUP_BASE_PATH/usenet/tv/*"
        "$CLEANUP_BASE_PATH/usenet/tv-4k/*"
        "$CLEANUP_BASE_PATH/usenet/whisparr"
        "$CLEANUP_BASE_PATH/torrents/whisparr"
        "$CLEANUP_BASE_PATH/media/to-delete/lidarr/*"
        "$CLEANUP_BASE_PATH/media/to-delete/radarr/*"
        "$CLEANUP_BASE_PATH/media/to-delete/radarr-4k/*"
        "$CLEANUP_BASE_PATH/media/to-delete/sonarr/*"
        "$CLEANUP_BASE_PATH/media/to-delete/sonarr-4k/*"
        "$CLEANUP_BASE_PATH/media/to-delete/sonarr-anime/*"
        "/mnt/int-data/usenet/incomplete/*"
        "$CLEANUP_BASE_PATH/media/to-delete/whisparr"
    )
    
    log "Starting cleanup process..."
    [[ "$DRY_RUN" == "true" ]] && log "DRY-RUN MODE: No files will be deleted"
    
    local failed_cleanups=0
    
    for target in "${cleanup_targets[@]}"; do
        if [[ "$target" == *"*" ]]; then
            local base_path="${target%/*}"
            if [[ -d "$base_path" ]]; then
                for item in "$base_path"/*; do
                    [[ -e "$item" ]] && cleanup_directory "$item" || ((failed_cleanups++))
                done
            else
                [[ "$VERBOSE" == "true" ]] && log "Base path $base_path does not exist"
            fi
        else
            cleanup_directory "$target" || ((failed_cleanups++))
        fi
    done
    
    if [[ "$DRY_RUN" != "true" ]] && [[ -s "$temp_file" ]]; then
        local total_freed=0
        while IFS= read -r size; do
            if [[ "$size" =~ ^[0-9.]+[KMGT]?B?$ ]]; then
                total_freed=$((total_freed + 1))
            fi
        done < "$temp_file"
        log "Cleanup completed. Processed $total_freed items"
    fi
    
    if [[ $failed_cleanups -gt 0 ]]; then
        log "Cleanup completed with $failed_cleanups failures"
        exit 1
    else
        log "Cleanup completed successfully"
    fi
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -d|--dry-run)
            DRY_RUN="true"
            shift
            ;;
        -v|--verbose)
            VERBOSE="true"
            shift
            ;;
        -h|--help)
            usage 0
            ;;
        -e|--exclude)
            if [[ -n "$EXCLUDE_DIRS" ]]; then
                EXCLUDE_DIRS="$EXCLUDE_DIRS,$2"
            else
                EXCLUDE_DIRS="$2"
            fi
            shift 2
            ;;
        -p|--path)
            CLEANUP_BASE_PATH="$2"
            shift 2
            ;;
        *)
            error_exit "Unknown option: $1. Use --help for usage information."
            ;;
    esac
done

if [[ ! -d "$CLEANUP_BASE_PATH" ]]; then
    error_exit "Base path $CLEANUP_BASE_PATH does not exist"
fi

if [[ "$DRY_RUN" != "true" ]] && [[ $EUID -ne 0 ]] && ! sudo -n true 2>/dev/null; then
    error_exit "This script requires sudo privileges for cleanup operations"
fi

main "$@"
