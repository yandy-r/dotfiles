# iTerm2 Configuration Guide

## Overview

This directory contains iTerm2 profiles, color schemes, and configuration files for a consistent terminal experience across macOS systems.

## Files Included

- `Default.json` - Default terminal profile
- `com.googlecode.iterm2.plist` - Main iTerm2 preferences
- `iTerm2 State.itermexport` - Complete iTerm2 state export
- Color schemes:
  - `MaterialDark.itermcolors`
  - `One Dark Pro.itermcolors`
  - `catppuccin-mocha.itermcolors`

## Setup Instructions

### 1. Configure Preferences Directory

```shell
# Set iTerm2 to use this directory for preferences
defaults write com.googlecode.iterm2 PrefsCustomFolder -string "~/.config/dotfiles/iterm2"
```

### 2. Enable Custom Preferences

```shell
# Enable loading preferences from custom folder
defaults write com.googlecode.iterm2 LoadPrefsFromCustomFolder -bool true
```

### 3. Import Color Schemes (Optional)

1. Open iTerm2
2. Go to **Preferences** → **Profiles** → **Colors**
3. Click **Color Presets** → **Import**
4. Select the `.itermcolors` files from this directory

### 4. Import Complete Configuration

For a full setup including all profiles and settings:

1. Open iTerm2
2. Go to **Preferences** → **General** → **Preferences**
3. Click **Load preferences from a custom folder or URL**
4. Browse to this directory: `~/.config/dotfiles/iterm2`

## Notes

- Restart iTerm2 after making configuration changes
- The bootstrap script should handle most of this setup automatically
- Profile settings include font configurations optimized for development
