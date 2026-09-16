#!/usr/bin/env bash
# ================================================================
# macOS-Specific Functions
# ================================================================
# Functions that only work on macOS

# ================================================================
# SYSTEM MANAGEMENT
# ================================================================

# Get system information summary
macos_info() {
    echo "=== macOS System Information ==="
    sw_vers
    echo
    system_profiler SPHardwareDataType | grep -E "(Model Name|Model Identifier|Processor|Memory|Serial Number)"
    echo
    echo "Uptime: $(uptime | awk '{print $3, $4}' | sed 's/,//')"
    echo "Disk Usage:"
    df -h | grep -E '^/dev/'
}

# Check for software updates
checkupdates() {
    echo "Checking for macOS updates..."
    softwareupdate -l
    
    if has_command mas; then
        echo -e "\nChecking Mac App Store updates..."
        mas outdated
    fi
    
    if has_command brew; then
        echo -e "\nChecking Homebrew updates..."
        brew outdated
    fi
}

# Install all available updates
installUpdates() {
    echo "Installing macOS updates..."
    sudo softwareupdate -ia
    
    if has_command mas; then
        echo -e "\nInstalling App Store updates..."
        mas upgrade
    fi
    
    if has_command brew; then
        echo -e "\nUpdating Homebrew..."
        brew update && brew upgrade && brew cleanup
    fi
}

# ================================================================
# APPLICATION MANAGEMENT
# ================================================================

# Kill application by name
killapp() {
    local app="$1"
    if [[ -z "$app" ]]; then
        echo "Usage: killapp <application_name>"
        return 1
    fi
    
    pkill -f "$app" && echo "Killed $app" || echo "$app not found or already stopped"
}

# Launch application
launch() {
    local app="$1"
    if [[ -z "$app" ]]; then
        echo "Usage: launch <application_name>"
        return 1
    fi
    
    open -a "$app"
}

# Get application information
appinfo() {
    local app="$1"
    if [[ -z "$app" ]]; then
        echo "Usage: appinfo <application_name>"
        return 1
    fi
    
    local app_path="/Applications/$app.app"
    if [[ ! -d "$app_path" ]]; then
        app_path="$(mdfind "kMDItemDisplayName == '$app'" | grep "\.app$" | head -1)"
    fi
    
    if [[ -n "$app_path" && -d "$app_path" ]]; then
        echo "Application: $app"
        echo "Path: $app_path"
        echo "Version: $(defaults read "$app_path/Contents/Info" CFBundleShortVersionString 2>/dev/null || echo "Unknown")"
        echo "Bundle ID: $(defaults read "$app_path/Contents/Info" CFBundleIdentifier 2>/dev/null || echo "Unknown")"
        echo "Size: $(du -sh "$app_path" | cut -f1)"
    else
        echo "Application '$app' not found"
    fi
}

# ================================================================
# FINDER OPERATIONS
# ================================================================

# Show file in Finder
showfinder() {
    local file="${1:-.}"
    open -R "$file"
}

# Get Finder selection
finderselection() {
    osascript -e 'tell application "Finder" to return POSIX path of (selection as alias)' 2>/dev/null
}

# Set Finder to show/hide hidden files
finder_show_hidden() {
    defaults write com.apple.finder AppleShowAllFiles -bool true
    killall Finder
    echo "Hidden files are now visible in Finder"
}

finder_hide_hidden() {
    defaults write com.apple.finder AppleShowAllFiles -bool false
    killall Finder
    echo "Hidden files are now hidden in Finder"
}

# ================================================================
# DOCK MANAGEMENT
# ================================================================

# Add application to Dock
dock_add() {
    local app="$1"
    if [[ -z "$app" ]]; then
        echo "Usage: dock_add <application_path>"
        return 1
    fi
    
    defaults write com.apple.dock persistent-apps -array-add "<dict><key>tile-data</key><dict><key>file-data</key><dict><key>_CFURLString</key><string>$app</string><key>_CFURLStringType</key><integer>0</integer></dict></dict></dict>"
    killall Dock
}

# Remove application from Dock by name
dock_remove() {
    local app="$1"
    if [[ -z "$app" ]]; then
        echo "Usage: dock_remove <application_name>"
        return 1
    fi
    
    # This is complex in pure bash, recommend using dockutil if available
    if has_command dockutil; then
        dockutil --remove "$app"
    else
        echo "For removing Dock items, install dockutil: brew install dockutil"
    fi
}

# Add spacer to Dock
dock_spacer() {
    defaults write com.apple.dock persistent-apps -array-add '{tile-data={}; tile-type="spacer-tile";}'
    killall Dock
    echo "Spacer added to Dock"
}

# ================================================================
# SCREENSHOTS
# ================================================================

# Enhanced screenshot function
screenshot() {
    local type="${1:-selection}"
    local delay="${2:-0}"
    local filename="${3:-Screenshot-$(date +%Y%m%d-%H%M%S).png}"
    local destination="${4:-$HOME/Desktop}"
    
    case "$type" in
        selection|sel)
            screencapture -T "$delay" "$destination/$filename"
            ;;
        window|win)
            screencapture -T "$delay" -w "$destination/$filename"
            ;;
        screen|full)
            screencapture -T "$delay" -x "$destination/$filename"
            ;;
        *)
            echo "Usage: screenshot [selection|window|screen] [delay] [filename] [destination]"
            return 1
            ;;
    esac
    
    echo "Screenshot saved: $destination/$filename"
}

# Screenshot to clipboard
screenshotclip() {
    local type="${1:-selection}"
    case "$type" in
        selection|sel) screencapture -c -T 3 ;;
        window|win) screencapture -c -w -T 3 ;;
        screen|full) screencapture -c -x -T 3 ;;
        *) echo "Usage: screenshotclip [selection|window|screen]" ;;
    esac
}

# ================================================================
# AUDIO CONTROLS
# ================================================================

# Set volume level
vol() {
    local level="$1"
    if [[ -z "$level" ]]; then
        # Get current volume
        osascript -e "output volume of (get volume settings)"
    elif [[ "$level" =~ ^[0-9]+$ ]] && [[ "$level" -ge 0 ]] && [[ "$level" -le 100 ]]; then
        osascript -e "set volume output volume $level"
        echo "Volume set to $level%"
    else
        echo "Usage: vol [0-100]"
        return 1
    fi
}

# Mute/unmute toggle
mute_toggle() {
    osascript -e "set volume output muted not (output muted of (get volume settings))"
    local muted
    muted=$(osascript -e "output muted of (get volume settings)")
    echo "Audio $([ "$muted" = "true" ] && echo "muted" || echo "unmuted")"
}

# ================================================================
# NETWORKING
# ================================================================

# WiFi management
wifi_status() {
    networksetup -getairportpower en0
}

wifi_on() {
    networksetup -setairportpower en0 on
    echo "WiFi turned on"
}

wifi_off() {
    networksetup -setairportpower en0 off
    echo "WiFi turned off"
}

wifi_connect() {
    local network="$1"
    local password="$2"
    
    if [[ -z "$network" ]]; then
        echo "Available networks:"
        networksetup -listpreferredwirelessnetworks en0
        return 1
    fi
    
    if [[ -n "$password" ]]; then
        networksetup -setairportnetwork en0 "$network" "$password"
    else
        networksetup -setairportnetwork en0 "$network"
    fi
}

wifi_scan() {
    airport -s
}

# Get network information
netinfo() {
    echo "=== Network Information ==="
    echo "WiFi Status: $(networksetup -getairportpower en0)"
    echo "Current Network: $(networksetup -getairportnetwork en0)"
    echo "Internal IP (en0): $(ipconfig getifaddr en0 2>/dev/null || echo "Not connected")"
    echo "Internal IP (en1): $(ipconfig getifaddr en1 2>/dev/null || echo "Not available")"
    echo "External IP: $(curl -s https://ipinfo.io/ip || echo "Not available")"
    echo "DNS Servers: $(networksetup -getdnsservers Wi-Fi | tr '\n' ' ')"
}

# ================================================================
# DISK AND STORAGE
# ================================================================

# Get disk usage for all mounted volumes
diskusage() {
    echo "=== Disk Usage ==="
    df -h | grep -E '^/dev/' | while read line; do
        echo "$line"
    done
    
    echo -e "\n=== Largest Directories ==="
    du -h / 2>/dev/null | sort -rh | head -20
}

# Eject disk by name or path
eject() {
    local disk="$1"
    if [[ -z "$disk" ]]; then
        echo "Available volumes:"
        ls -1 /Volumes/
        return 1
    fi
    
    diskutil eject "$disk" && echo "Ejected $disk" || echo "Failed to eject $disk"
}

# Show mounted volumes
volumes() {
    echo "=== Mounted Volumes ==="
    mount | grep -E '^/dev/' | column -t
}

# ================================================================
# BATTERY INFORMATION
# ================================================================

# Battery status
battery() {
    pmset -g batt | grep -o '[0-9]\+%' | head -1
}

battery_info() {
    echo "=== Battery Information ==="
    pmset -g batt
    echo
    system_profiler SPPowerDataType | grep -E "(Cycle Count|Condition)"
}

# ================================================================
# PLIST MANIPULATION
# ================================================================

# Read plist value
plist_read() {
    local file="$1"
    local key="$2"
    
    if [[ -z "$file" || -z "$key" ]]; then
        echo "Usage: plist_read <plist_file> <key>"
        return 1
    fi
    
    defaults read "$file" "$key"
}

# Write plist value
plist_write() {
    local file="$1"
    local key="$2"
    local value="$3"
    local type="${4:-string}"
    
    if [[ -z "$file" || -z "$key" || -z "$value" ]]; then
        echo "Usage: plist_write <plist_file> <key> <value> [type]"
        echo "Types: string, int, bool, float, date, data, array, dict"
        return 1
    fi
    
    case "$type" in
        bool) defaults write "$file" "$key" -bool "$value" ;;
        int) defaults write "$file" "$key" -int "$value" ;;
        float) defaults write "$file" "$key" -float "$value" ;;
        *) defaults write "$file" "$key" "$value" ;;
    esac
}

# ================================================================
# QUICK LOOK
# ================================================================

# Quick Look preview
ql() {
    if [[ $# -eq 0 ]]; then
        echo "Usage: ql <file> [file2] [file3] ..."
        return 1
    fi
    
    qlmanage -p "$@" >& /dev/null
}

# ================================================================
# XCODE AND DEVELOPMENT
# ================================================================

# Clean Xcode derived data
xcode_clean() {
    local derived_data="$HOME/Library/Developer/Xcode/DerivedData"
    if [[ -d "$derived_data" ]]; then
        echo "Cleaning Xcode derived data..."
        rm -rf "${derived_data:?}"/*
        echo "Xcode derived data cleaned"
    else
        echo "Xcode derived data directory not found"
    fi
}

# Open iOS Simulator
ios_simulator() {
    open -a Simulator
}

# List iOS simulators
ios_list() {
    xcrun simctl list devices
}

# Boot iOS simulator
ios_boot() {
    local device="${1:-iPhone 15}"
    xcrun simctl boot "$device"
}

# ================================================================
# SPOTLIGHT SEARCH
# ================================================================

# Search files with Spotlight
spotlight() {
    local query="$1"
    if [[ -z "$query" ]]; then
        echo "Usage: spotlight <search_term>"
        return 1
    fi
    
    mdfind "$query"
}

# ================================================================
# KEYCHAIN MANAGEMENT
# ================================================================

# Search keychain
keychain_find() {
    local service="$1"
    if [[ -z "$service" ]]; then
        echo "Usage: keychain_find <service_name>"
        return 1
    fi
    
    security find-generic-password -s "$service"
}

# Get password from keychain
keychain_get() {
    local service="$1"
    local account="$2"
    
    if [[ -z "$service" ]]; then
        echo "Usage: keychain_get <service_name> [account]"
        return 1
    fi
    
    if [[ -n "$account" ]]; then
        security find-generic-password -s "$service" -a "$account" -w
    else
        security find-generic-password -s "$service" -w
    fi
}

# ================================================================
# SYSTEM PREFERENCES
# ================================================================

# Open specific System Preference pane
syspref() {
    local pane="$1"
    if [[ -z "$pane" ]]; then
        echo "Available panes: general, desktop, dock, security, network, bluetooth, sound, users, time"
        return 1
    fi
    
    case "$pane" in
        general) open "x-apple.systempreferences:com.apple.preference" ;;
        desktop) open "x-apple.systempreferences:com.apple.preference.desktopscreeneffect" ;;
        dock) open "x-apple.systempreferences:com.apple.preference.dock" ;;
        security) open "x-apple.systempreferences:com.apple.preference.security" ;;
        network) open "x-apple.systempreferences:com.apple.preference.network" ;;
        bluetooth) open "x-apple.systempreferences:com.apple.preference.bluetooth" ;;
        sound) open "x-apple.systempreferences:com.apple.preference.sound" ;;
        users) open "x-apple.systempreferences:com.apple.preferences.users" ;;
        time) open "x-apple.systempreferences:com.apple.preference.datetime" ;;
        *) echo "Unknown preference pane: $pane" ;;
    esac
}

# ================================================================
# HOMEBREW HELPERS
# ================================================================

# Homebrew maintenance
brew_maintenance() {
    echo "Running Homebrew maintenance..."
    brew update
    brew upgrade
    brew cleanup
    brew doctor
    echo "Homebrew maintenance complete"
}

# Find which Homebrew package provides a command
brew_which() {
    local command="$1"
    if [[ -z "$command" ]]; then
        echo "Usage: brew_which <command>"
        return 1
    fi
    
    local path
    path="$(which "$command" 2>/dev/null)"
    if [[ -n "$path" && "$path" == *"/opt/homebrew/"* ]]; then
        local formula
        formula="$(brew --prefix)/bin/$command"
        brew list | grep -E "$(basename "$formula")"
    else
        echo "Command '$command' not found or not installed via Homebrew"
    fi
}