#!/usr/bin/env bash
#
# Manual use only. Not wired into bootstrap, playbooks, or CI.
#
# Fix zshrc symlink to use dotfiles version

set -euo pipefail

echo "Fixing ~/.zshrc symlink..."

# Backup existing zshrc if it's not a symlink
if [[ -f "$HOME/.zshrc" ]] && [[ ! -L "$HOME/.zshrc" ]]; then
    echo "  Backing up existing ~/.zshrc to ~/.zshrc.backup"
    mv "$HOME/.zshrc" "$HOME/.zshrc.backup"
fi

# Remove if it's a broken symlink
if [[ -L "$HOME/.zshrc" ]] && [[ ! -e "$HOME/.zshrc" ]]; then
    echo "  Removing broken symlink"
    rm "$HOME/.zshrc"
fi

# Create symlink
if [[ ! -L "$HOME/.zshrc" ]]; then
    echo "  Creating symlink: ~/.zshrc -> ~/.config/dotfiles/zsh/zshrc"
    ln -s "$HOME/.config/dotfiles/zsh/zshrc" "$HOME/.zshrc"
    echo "  ✓ Symlink created"
else
    echo "  ✓ Symlink already exists"
fi

# Verify
if [[ -L "$HOME/.zshrc" ]]; then
    target=$(readlink "$HOME/.zshrc")
    echo "  Current symlink: ~/.zshrc -> $target"
fi

echo ""
echo "Done! Restart your shell or run: exec zsh -l"
