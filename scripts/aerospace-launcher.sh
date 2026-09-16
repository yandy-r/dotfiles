#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[AEROSPACE-LAUNCHER]"

# Configuration
LAUNCHER_TYPE="${LAUNCHER_TYPE:-menu}"
AUTO_WORKSPACE="${AUTO_WORKSPACE:-true}"
CONFIG_FILE="${CONFIG_FILE:-$HOME/.config/dotfiles/aerospace/aerospace.toml}"

# Application to workspace mappings (based on aerospace.toml)
declare -A APP_WORKSPACES=(
    # Meetings
    ["Zoom"]="9"
    
    # Browsers
    ["Google Chrome"]="B"
    ["Safari"]="S"
    
    # Communication
    ["Discord"]="C"
    ["Slack"]="C"
    ["Messages"]="C"
    ["WhatsApp"]="C"
    ["Telegram"]="C"
    
    # Entertainment
    ["Plex"]="E"
    ["Plexamp"]="E"
    ["Spotify"]="E"
    ["IINA"]="E"
    ["Infuse 7"]="E"
    
    # Religious
    ["Logos Bible Software"]="G"
    
    # Microsoft Office
    ["Microsoft Excel"]="M"
    ["Microsoft PowerPoint"]="M"
    ["Microsoft Word"]="M"
    
    # Notes
    ["Simplenote"]="N"
    ["Microsoft OneNote"]="N"
    
    # Personal Productivity
    ["Things 3"]="P"
    ["Asana"]="P"
    ["Linear"]="P"
    
    # Terminal
    ["WezTerm"]="T"
    ["Ghostty"]="T"
    ["iTerm2"]="T"
    ["Terminal"]="T"
    
    # Virtualization
    ["Parallels Desktop"]="V"
    
    # Work
    ["Microsoft Teams (work or school)"]="W"
    ["Microsoft Outlook"]="W"
    ["Fantastical - Calendar & Tasks"]="W"
    ["Spark – Email App by Readdle"]="W"
    
    # Development Tools
    ["Transmit 5"]="X"
    ["Postman"]="X"
    
    # Remote Access
    ["Citrix Workspace"]="Z"
)

usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS] [APP_NAME]

Launch applications with optional automatic workspace assignment.

OPTIONS:
    --type TYPE         Launcher type: menu, rofi, fzf, dmenu (default: menu)
    --no-workspace      Don't automatically move to workspace
    --list              List all available applications
    --workspace WS      Force specific workspace (overrides auto-assignment)
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    LAUNCHER_TYPE       Launcher interface (menu, rofi, fzf, dmenu)
    AUTO_WORKSPACE      Automatically assign workspace (true/false)
    CONFIG_FILE         Path to aerospace config file

EXAMPLES:
    $SCRIPT_NAME                        # Show application menu
    $SCRIPT_NAME "Google Chrome"        # Launch Chrome directly
    $SCRIPT_NAME --type fzf             # Use fzf for selection
    $SCRIPT_NAME --workspace B Safari   # Launch Safari in workspace B
    $SCRIPT_NAME --list                 # List available apps
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

get_installed_apps() {
    # Get list of installed applications
    find /Applications -name "*.app" -maxdepth 1 -exec basename {} .app \; | sort
    find /System/Applications -name "*.app" -maxdepth 1 -exec basename {} .app \; 2>/dev/null | sort || true
    find /System/Applications/Utilities -name "*.app" -maxdepth 1 -exec basename {} .app \; 2>/dev/null | sort || true
}

get_workspace_for_app() {
    local app_name="$1"
    echo "${APP_WORKSPACES[$app_name]:-}"
}

launch_app() {
    local app_name="$1"
    local target_workspace="${2:-}"
    
    log "Launching $app_name"
    
    # Launch the application
    if ! open -a "$app_name"; then
        error_exit "Failed to launch $app_name"
    fi
    
    # Move to workspace if specified or auto-assigned
    if [[ -n "$target_workspace" ]]; then
        log "Moving to workspace $target_workspace"
        sleep 0.5  # Give app time to start
        aerospace workspace "$target_workspace" 2>/dev/null || log "Warning: Failed to switch to workspace $target_workspace"
    elif [[ "$AUTO_WORKSPACE" == "true" ]]; then
        local auto_workspace
        auto_workspace=$(get_workspace_for_app "$app_name")
        if [[ -n "$auto_workspace" ]]; then
            log "Auto-assigning to workspace $auto_workspace"
            sleep 0.5  # Give app time to start
            aerospace workspace "$auto_workspace" 2>/dev/null || log "Warning: Failed to switch to workspace $auto_workspace"
        fi
    fi
}

menu_launcher() {
    local apps
    mapfile -t apps < <(get_installed_apps)
    
    echo "Select an application to launch:"
    echo "================================"
    
    local i=1
    for app in "${apps[@]}"; do
        local workspace
        workspace=$(get_workspace_for_app "$app")
        if [[ -n "$workspace" ]]; then
            printf "%3d) %-40s [→ %s]\n" $i "$app" "$workspace"
        else
            printf "%3d) %s\n" $i "$app"
        fi
        ((i++))
    done
    
    echo
    read -rp "Enter number (1-${#apps[@]}): " selection
    
    if [[ "$selection" =~ ^[0-9]+$ ]] && [[ $selection -ge 1 ]] && [[ $selection -le ${#apps[@]} ]]; then
        local selected_app="${apps[$((selection-1))]}"
        launch_app "$selected_app"
    else
        error_exit "Invalid selection: $selection"
    fi
}

fzf_launcher() {
    if ! command -v fzf &>/dev/null; then
        error_exit "fzf is not installed"
    fi
    
    local apps
    mapfile -t apps < <(get_installed_apps)
    
    local selected_app
    selected_app=$(printf '%s\n' "${apps[@]}" | fzf --prompt="Launch app: " --height=40% --reverse)
    
    if [[ -n "$selected_app" ]]; then
        launch_app "$selected_app"
    else
        log "No application selected"
    fi
}

rofi_launcher() {
    if ! command -v rofi &>/dev/null; then
        error_exit "rofi is not installed"
    fi
    
    local apps
    mapfile -t apps < <(get_installed_apps)
    
    local selected_app
    selected_app=$(printf '%s\n' "${apps[@]}" | rofi -dmenu -p "Launch app")
    
    if [[ -n "$selected_app" ]]; then
        launch_app "$selected_app"
    else
        log "No application selected"
    fi
}

dmenu_launcher() {
    if ! command -v dmenu &>/dev/null; then
        error_exit "dmenu is not installed"
    fi
    
    local apps
    mapfile -t apps < <(get_installed_apps)
    
    local selected_app
    selected_app=$(printf '%s\n' "${apps[@]}" | dmenu -p "Launch app:")
    
    if [[ -n "$selected_app" ]]; then
        launch_app "$selected_app"
    else
        log "No application selected"
    fi
}

list_apps() {
    echo "Available Applications:"
    echo "======================"
    
    local apps
    mapfile -t apps < <(get_installed_apps)
    
    for app in "${apps[@]}"; do
        local workspace
        workspace=$(get_workspace_for_app "$app")
        if [[ -n "$workspace" ]]; then
            printf "%-40s → Workspace %s\n" "$app" "$workspace"
        else
            echo "$app"
        fi
    done
}

# Parse command line arguments
FORCE_WORKSPACE=""
APP_NAME=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --type)
            LAUNCHER_TYPE="$2"
            shift 2
            ;;
        --no-workspace)
            AUTO_WORKSPACE="false"
            shift
            ;;
        --workspace)
            FORCE_WORKSPACE="$2"
            shift 2
            ;;
        --list)
            list_apps
            exit 0
            ;;
        -h|--help)
            usage 0
            ;;
        -*)
            error_exit "Unknown option: $1. Use --help for usage information."
            ;;
        *)
            APP_NAME="$1"
            shift
            ;;
    esac
done

main() {
    # Check if aerospace is available
    if ! command -v aerospace &>/dev/null; then
        log "Warning: AeroSpace not found. Workspace assignment will be skipped."
        AUTO_WORKSPACE="false"
    fi
    
    # If app name provided directly, launch it
    if [[ -n "$APP_NAME" ]]; then
        launch_app "$APP_NAME" "$FORCE_WORKSPACE"
        return 0
    fi
    
    # Otherwise, use the specified launcher
    case "$LAUNCHER_TYPE" in
        "menu")
            menu_launcher
            ;;
        "fzf")
            fzf_launcher
            ;;
        "rofi")
            rofi_launcher
            ;;
        "dmenu")
            dmenu_launcher
            ;;
        *)
            error_exit "Invalid launcher type: $LAUNCHER_TYPE. Use menu, fzf, rofi, or dmenu."
            ;;
    esac
}

main "$@"