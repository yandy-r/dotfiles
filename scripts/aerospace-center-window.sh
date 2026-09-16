#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[AEROSPACE-CENTER]"

usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS]

Center the currently focused window on screen (for floating windows).

OPTIONS:
    --offset-x X        Horizontal offset from center (default: 0)
    --offset-y Y        Vertical offset from center (default: 0)
    -h, --help          Show this help message

EXAMPLES:
    $SCRIPT_NAME                # Center window exactly
    $SCRIPT_NAME --offset-y -50 # Center with slight upward offset
EOF
    exit "${1:-0}"
}

log() {
    echo "${LOG_PREFIX} $*" >&2
}

error_exit() {
    log "ERROR: $1" >&2
    exit "${2:-1}"
}

# Default offsets
OFFSET_X=0
OFFSET_Y=0

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --offset-x)
            OFFSET_X="$2"
            shift 2
            ;;
        --offset-y)
            OFFSET_Y="$2"
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
    # Check if we have the necessary tools
    if ! command -v osascript &>/dev/null; then
        error_exit "osascript not available (macOS required)"
    fi
    
    # Get screen dimensions and current window info
    local screen_info
    screen_info=$(osascript -e 'tell application "Finder" to get bounds of window of desktop')
    
    # Parse screen bounds (left, top, right, bottom)
    local screen_left screen_top screen_right screen_bottom
    IFS=', ' read -r screen_left screen_top screen_right screen_bottom <<< "$screen_info"
    
    local screen_width=$((screen_right - screen_left))
    local screen_height=$((screen_bottom - screen_top))
    
    # Get frontmost window info
    local window_info
    if ! window_info=$(osascript -e '
        tell application "System Events"
            set frontApp to name of first application process whose frontmost is true
            tell process frontApp
                set frontWindow to window 1
                set windowSize to size of frontWindow
                return (item 1 of windowSize) & "," & (item 2 of windowSize)
            end tell
        end tell
    ' 2>/dev/null); then
        error_exit "Could not get window information. Is there a focused window?"
    fi
    
    # Parse window dimensions
    local window_width window_height
    IFS=',' read -r window_width window_height <<< "$window_info"
    
    # Calculate center position
    local center_x center_y
    center_x=$(( (screen_width - window_width) / 2 + screen_left + OFFSET_X ))
    center_y=$(( (screen_height - window_height) / 2 + screen_top + OFFSET_Y ))
    
    # Ensure window stays within screen bounds
    if [[ $center_x -lt $screen_left ]]; then
        center_x=$screen_left
    elif [[ $((center_x + window_width)) -gt $screen_right ]]; then
        center_x=$((screen_right - window_width))
    fi
    
    if [[ $center_y -lt $screen_top ]]; then
        center_y=$screen_top
    elif [[ $((center_y + window_height)) -gt $screen_bottom ]]; then
        center_y=$((screen_bottom - window_height))
    fi
    
    # Move the window
    if osascript -e "
        tell application \"System Events\"
            set frontApp to name of first application process whose frontmost is true
            tell process frontApp
                set position of window 1 to {$center_x, $center_y}
            end tell
        end tell
    " 2>/dev/null; then
        log "Window centered at position ($center_x, $center_y)"
    else
        error_exit "Failed to center window"
    fi
}

main "$@"