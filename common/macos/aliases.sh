#!/usr/bin/env bash
# ================================================================
# macOS-Specific Aliases
# ================================================================
# Aliases that only work on macOS

# ================================================================
# SYSTEM MANAGEMENT
# ================================================================

# DNS and network
alias flushdns="sudo dscacheutil -flushcache && sudo killall -HUP mDNSResponder"
alias cleardns="sudo killall -HUP mDNSResponder"

# System information (sysinfo function is defined in common/functions.sh)
alias cpuinfo="sysctl -n machdep.cpu.brand_string"
alias meminfo="vm_stat"

# ================================================================
# APPLICATION MANAGEMENT
# ================================================================

# Application shortcuts
alias safari="open -a Safari"
alias chrome="open -a 'Google Chrome'"
alias firefox="open -a Firefox"
alias finder="open -a Finder"
alias terminal="open -a Terminal"
alias iterm="open -a iTerm"
alias code="open -a 'Visual Studio Code'"
alias subl="open -a 'Sublime Text'"
alias atom="open -a Atom"

# Quick application launching
alias calc="open -a Calculator"
alias notes="open -a Notes"
alias mail="open -a Mail"
alias calendar="open -a Calendar"
alias contacts="open -a Contacts"
alias maps="open -a Maps"
alias photos="open -a Photos"

# ================================================================
# FINDER AND DESKTOP
# ================================================================

# Finder controls
alias showfiles="defaults write com.apple.finder AppleShowAllFiles -bool true && killall Finder"
alias hidefiles="defaults write com.apple.finder AppleShowAllFiles -bool false && killall Finder"
alias showdesktop="defaults write com.apple.finder CreateDesktop -bool true && killall Finder"
alias hidedesktop="defaults write com.apple.finder CreateDesktop -bool false && killall Finder"

# Desktop management
alias desktop="cd ~/Desktop"
alias emptytrash="sudo rm -rfv /Volumes/*/.Trashes; rm -rfv ~/.Trash/*; rm -rfv ~/.Trash/.*"

# Recursively delete .DS_Store files
alias cleanup="find . -name '*.DS_Store' -type f -ls -delete"

# File operations (ql function is in macos/functions.sh)
alias quicklook="qlmanage -p"

# ================================================================
# SYSTEM PREFERENCES
# ================================================================

# Dock management
alias dockrestart="killall Dock"
alias dockspacer="defaults write com.apple.dock persistent-apps -array-add '{tile-data={}; tile-type=\"spacer-tile\";}' && killall Dock"
alias dock2d="defaults write com.apple.dock no-glass -bool true && killall Dock"
alias dock3d="defaults delete com.apple.dock no-glass && killall Dock"

# Screenshot management (screenshot and screenshotclip functions are in macos/functions.sh)
alias screenshotdir="defaults write com.apple.screencapture location"

# ================================================================
# AUDIO AND MEDIA
# ================================================================

# Volume controls
alias mute="osascript -e 'set volume output muted true'"
alias unmute="osascript -e 'set volume output muted false'"
alias maxvol="osascript -e 'set volume 10'"
alias stfu="osascript -e 'set volume output muted true'"
alias pumpitup="osascript -e 'set volume 10'"

# iTunes/Music controls
alias music="open -a Music"
alias itunes="open -a Music"  # For older systems
alias spotify="open -a Spotify"

# ================================================================
# DEVELOPMENT TOOLS
# ================================================================

# Xcode
alias xcode="open -a Xcode"
alias simulator="open -a Simulator"
alias instruments="open -a Instruments"

# iOS Simulator shortcuts
alias iossim="xcrun simctl list devices"
alias iphone="xcrun simctl boot 'iPhone 15'"
alias ipad="xcrun simctl boot 'iPad Air'"

# ================================================================
# NETWORKING
# ================================================================

# Airport (WiFi) utility
alias airport="/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport"
alias wifiscan="airport -s"
alias wifioff="networksetup -setairportpower en0 off"
alias wifion="networksetup -setairportpower en0 on"

# Network interfaces
alias wifi="networksetup -getairportnetwork en0"
alias ethernet="ifconfig en1"
alias interfaces="networksetup -listallhardwareports"

# ================================================================
# HOMEBREW
# ================================================================

# Homebrew shortcuts
alias brewup="brew update && brew upgrade && brew cleanup"
alias brewinfo="brew leaves | xargs brew desc --eval-all"
alias brewdeps="brew deps --tree --installed"
alias brewsize="brew list -f | xargs brew info --json=v1 | jq -r 'map(select(.installed)) | sort_by(.installed[0].installed_on_request | not) | reverse | .[] | \"\(.name): \(.installed[0].used_space)\"'"

# ================================================================
# UTILITY ALIASES
# ================================================================

# PlistBuddy
alias plistbuddy="/usr/libexec/PlistBuddy"
alias plist="plutil -p"

# File system
alias showpath="echo \$PATH | tr ':' '\n' | nl"
alias fspath="stat -f '%N -> %Y' *"

# Disk utility (eject function is in macos/functions.sh)
alias diskutil="diskutil"
alias drives="diskutil list"

# ================================================================
# SECURITY
# ================================================================

# Keychain
alias keychain="open -a 'Keychain Access'"
alias lockscreen="/System/Library/CoreServices/Menu\ Extras/User.menu/Contents/Resources/CGSession -suspend"

# FileVault
alias filevault="sudo fdesetup status"

# ================================================================
# SYSTEM MONITORING
# ================================================================

# Activity Monitor equivalents (battery function is in macos/functions.sh)
alias cpu="top -l 1 -s 0 | grep 'CPU usage'"
alias memory="top -l 1 -s 0 | grep PhysMem"
alias temp="sudo powermetrics --samplers smc -n 1 -i 1000 | grep -i temp"

# System logs
alias syslog="log show --last 1h"
alias consolelog="log show --predicate 'process == \"Console\"' --last 1h"

# ================================================================
# QUICK ACTIONS
# ================================================================

# Open common directories
alias apps="open /Applications"
alias library="open ~/Library"
alias preferences="open ~/Library/Preferences"
alias desktop="open ~/Desktop"
alias documents="open ~/Documents"
alias downloads="open ~/Downloads"

# System directories
alias usr="open /usr"
alias etc="open /etc"
alias var="open /var"
alias tmp="open /tmp"

# ================================================================
# NOTIFICATION CENTER
# ================================================================

# Notification management
alias notifications="open -b com.apple.notificationcenter"
alias dnd="defaults write com.apple.controlcenter 'NSStatusItem Visible FocusModes' -bool true"

# ================================================================
# QUICK LOOK
# ================================================================

# QuickLook plugins
alias qlplugins="ls /Library/QuickLook ~/Library/QuickLook 2>/dev/null"
alias qlreload="qlmanage -r"

# ================================================================
# TIME MACHINE
# ================================================================

# Time Machine controls
alias tmstatus="tmutil status"
alias tmstart="sudo tmutil startbackup"
alias tmstop="sudo tmutil stopbackup"
alias tmexclude="sudo tmutil addexclusion"

# ================================================================
# SPOTLIGHT
# ================================================================

# Spotlight management
alias spotoff="sudo mdutil -a -i off"
alias spoton="sudo mdutil -a -i on"
alias reindex="sudo mdutil -E /"