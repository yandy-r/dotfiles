#!/usr/bin/env zsh
# ================================================================
# FZF Setup - Platform-Aware Configuration
# ================================================================

# Detect Homebrew path for cross-platform compatibility
if [[ -z "$DOTFILES_HOMEBREW_PATH" ]]; then
    if [[ "$(uname -s)" == "Darwin" ]]; then
        if [[ "$(uname -m)" == "arm64" ]]; then
            DOTFILES_HOMEBREW_PATH="/opt/homebrew"
        else
            DOTFILES_HOMEBREW_PATH="/usr/local"
        fi
    else
        DOTFILES_HOMEBREW_PATH="/home/linuxbrew/.linuxbrew"
    fi
fi

# Add FZF to PATH if installed via Homebrew
if [[ -d "$DOTFILES_HOMEBREW_PATH/opt/fzf/bin" ]]; then
    if [[ ! "$PATH" == *"$DOTFILES_HOMEBREW_PATH/opt/fzf/bin"* ]]; then
        PATH="${PATH:+${PATH}:}$DOTFILES_HOMEBREW_PATH/opt/fzf/bin"
    fi
fi

# Load FZF shell integration if available
if command -v fzf >/dev/null 2>&1; then
    source <(fzf --zsh)
fi
