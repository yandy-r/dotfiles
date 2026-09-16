#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[AEROSPACE-STATUS]"

# Configuration
OUTPUT_FORMAT="${OUTPUT_FORMAT:-text}"
SHOW_WINDOWS="${SHOW_WINDOWS:-false}"
SHOW_MONITORS="${SHOW_MONITORS:-false}"
COMPACT="${COMPACT:-false}"

usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS]

Display AeroSpace workspace and window status information.

OPTIONS:
    --format FORMAT     Output format: text, json, minimal (default: text)
    --windows           Show windows in each workspace
    --monitors          Show monitor information
    --compact           Use compact output format
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    OUTPUT_FORMAT       Output format (text, json, minimal)
    SHOW_WINDOWS        Show windows in workspaces (true/false)
    SHOW_MONITORS       Show monitor information (true/false)
    COMPACT             Use compact format (true/false)

EXAMPLES:
    $SCRIPT_NAME                    # Basic status
    $SCRIPT_NAME --windows          # Include window list
    $SCRIPT_NAME --format json      # JSON output
    $SCRIPT_NAME --compact          # Minimal output
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

check_aerospace() {
    if ! command -v aerospace &>/dev/null; then
        error_exit "AeroSpace is not installed or not in PATH"
    fi
}

get_current_workspace() {
    aerospace list-workspaces --focused
}

get_all_workspaces() {
    aerospace list-workspaces --all
}

get_workspace_windows() {
    local workspace="$1"
    aerospace list-windows --workspace "$workspace" --format '%{app-name} (%{window-title})'
}

get_monitor_info() {
    aerospace list-monitors
}

show_text_status() {
    local current_workspace
    current_workspace=$(get_current_workspace)
    
    echo "AeroSpace Status"
    echo "================"
    echo "Current Workspace: $current_workspace"
    echo
    
    if [[ "$SHOW_MONITORS" == "true" ]]; then
        echo "Monitors:"
        get_monitor_info | while read -r monitor; do
            echo "  $monitor"
        done
        echo
    fi
    
    echo "Workspaces:"
    get_all_workspaces | while read -r workspace; do
        local indicator=""
        if [[ "$workspace" == "$current_workspace" ]]; then
            indicator=" (current)"
        fi
        
        if [[ "$COMPACT" == "true" ]]; then
            echo "  $workspace$indicator"
        else
            local window_count
            window_count=$(aerospace list-windows --workspace "$workspace" | wc -l | xargs)
            echo "  $workspace$indicator - $window_count windows"
            
            if [[ "$SHOW_WINDOWS" == "true" ]] && [[ $window_count -gt 0 ]]; then
                get_workspace_windows "$workspace" | while read -r window; do
                    echo "    - $window"
                done
            fi
        fi
    done
}

show_json_status() {
    local current_workspace
    current_workspace=$(get_current_workspace)
    
    local json_output='{"current_workspace":"'$current_workspace'","workspaces":['
    local first=true
    
    get_all_workspaces | while read -r workspace; do
        if [[ "$first" != "true" ]]; then
            json_output+=","
        fi
        first=false
        
        local window_count
        window_count=$(aerospace list-windows --workspace "$workspace" | wc -l | xargs)
        
        json_output+='{"name":"'$workspace'","window_count":'$window_count',"is_current":'
        if [[ "$workspace" == "$current_workspace" ]]; then
            json_output+='true'
        else
            json_output+='false'
        fi
        
        if [[ "$SHOW_WINDOWS" == "true" ]]; then
            json_output+=',"windows":['
            local window_first=true
            get_workspace_windows "$workspace" | while read -r window; do
                if [[ "$window_first" != "true" ]]; then
                    json_output+=","
                fi
                window_first=false
                json_output+='"'$window'"'
            done
            json_output+=']'
        fi
        
        json_output+='}'
    done
    
    json_output+=']'
    
    if [[ "$SHOW_MONITORS" == "true" ]]; then
        json_output+=',"monitors":['
        local monitor_first=true
        get_monitor_info | while read -r monitor; do
            if [[ "$monitor_first" != "true" ]]; then
                json_output+=","
            fi
            monitor_first=false
            json_output+='"'$monitor'"'
        done
        json_output+=']'
    fi
    
    json_output+='}'
    echo "$json_output" | jq '.' 2>/dev/null || echo "$json_output"
}

show_minimal_status() {
    local current_workspace
    current_workspace=$(get_current_workspace)
    
    local workspace_list
    workspace_list=$(get_all_workspaces | tr '\n' ' ' | sed 's/ $//')
    
    echo "$current_workspace | $workspace_list"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --format)
            OUTPUT_FORMAT="$2"
            shift 2
            ;;
        --windows)
            SHOW_WINDOWS="true"
            shift
            ;;
        --monitors)
            SHOW_MONITORS="true"
            shift
            ;;
        --compact)
            COMPACT="true"
            shift
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
    check_aerospace
    
    case "$OUTPUT_FORMAT" in
        "text")
            show_text_status
            ;;
        "json")
            show_json_status
            ;;
        "minimal")
            show_minimal_status
            ;;
        *)
            error_exit "Invalid output format: $OUTPUT_FORMAT. Use text, json, or minimal."
            ;;
    esac
}

main "$@"