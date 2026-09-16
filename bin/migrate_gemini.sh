#!/bin/bash
# Manual use only. Not wired into bootstrap, playbooks, or CI.

set -e

DOTFILES_DIR="$HOME/.config/dotfiles"
GEMINI_DIR="$HOME/.gemini"
TARGET_DIR="$DOTFILES_DIR/.gemini"

echo "Starting migration of $GEMINI_DIR to $TARGET_DIR..."

if [ -L "$GEMINI_DIR" ]; then
    echo "$GEMINI_DIR is already a symlink."
    ls -l "$GEMINI_DIR"
    exit 0
fi

if [ ! -d "$GEMINI_DIR" ]; then
    echo "$GEMINI_DIR does not exist."
    exit 1
fi

# Move contents to target (which already has .gitignore)
echo "Moving contents..."
rsync -av --remove-source-files "$GEMINI_DIR/" "$TARGET_DIR/"
find "$GEMINI_DIR" -type d -empty -delete 2>/dev/null || rmdir "$GEMINI_DIR"

# Create symlink
ln -s "$TARGET_DIR" "$GEMINI_DIR"
echo "Done. $GEMINI_DIR -> $TARGET_DIR"