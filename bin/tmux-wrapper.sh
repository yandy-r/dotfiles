#!/usr/bin/env bash
# Tmux wrapper to ensure proper PATH inheritance

# Source our path setup to get the correct PATH
source "$HOME/.config/dotfiles/common/platform.sh"
source "$HOME/.config/dotfiles/common/paths.sh"

# Update tmux environment with current PATH
tmux set-environment -g PATH "$PATH" 2>/dev/null || true

# Forward all arguments to tmux
exec tmux "$@"