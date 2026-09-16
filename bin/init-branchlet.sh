#!/usr/bin/env bash
# ================================================================
# Branchlet Initialization Script
# ================================================================
# Creates a .branchlet.json file in the current directory with
# worktree path template configured for: ../worktrees/REPO_NAME/$BRANCH_NAME

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info() {
  echo -e "${GREEN}[INFO]${NC} $*"
}

warn() {
  echo -e "${YELLOW}[WARN]${NC} $*"
}

error() {
  echo -e "${RED}[ERROR]${NC} $*"
}

success() {
  echo -e "${BLUE}[SUCCESS]${NC} $*"
}

# Check if we're in a git repository
if ! git rev-parse --git-dir >/dev/null 2>&1; then
  error "Not in a git repository. Please run this script from a git repository root."
  exit 1
fi

# Get repository name from current directory
REPO_NAME="$(basename "$(pwd)")"
BRANCHLET_FILE=".branchlet.json"

# Check if .branchlet.json already exists
if [[ -f "$BRANCHLET_FILE" ]]; then
  warn ".branchlet.json already exists in this directory"
  read -p "Do you want to overwrite it? [y/N] " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    info "Keeping existing .branchlet.json file"
    exit 0
  fi
fi

# Check for global settings to determine terminal command
GLOBAL_SETTINGS="${HOME}/.branchlet/settings.json"
TERMINAL_CMD="antigravity ."
if [[ -f "$GLOBAL_SETTINGS" ]] && command -v jq >/dev/null 2>&1; then
  info "Found global settings at $GLOBAL_SETTINGS"
  TERMINAL_CMD=$(jq -r '.terminalCommand // "cursor ."' "$GLOBAL_SETTINGS" 2>/dev/null || echo "cursor .")
else
  info "Using default terminal command: cursor ."
fi

# Create .branchlet.json file
info "Creating .branchlet.json for repository: $REPO_NAME"
info "Worktree path template: \$BASE_PATH/../worktrees/$REPO_NAME/\$BRANCH_NAME"

# Create the JSON file
cat >"$BRANCHLET_FILE" <<EOF
{
  "worktreeCopyPatterns": [
    ".env",
    ".env**",
    ".vscode/**",
    ".venv/**",
    "package.json",
    "tsconfig.json",
    "*.config.js",
    "*.config.ts",
    "*.config.json",
    ".gitignore",
    ".editorconfig",
    ".prettierrc*",
    ".eslintrc*"
  ],
  "worktreeCopyIgnores": [
    "**/node_modules/**",
    "**/dist/**",
    "**/build/**",
    "**/.git/**",
    "**/Thumbs.db",
    "**/.DS_Store",
    "**/coverage/**",
    "**/.nyc_output/**",
    "**/logs/**",
    "**/*.log"
  ],
  "worktreePathTemplate": "\$BASE_PATH/../worktrees/$REPO_NAME/\$BRANCH_NAME",
  "postCreateCmd": [
    "npm install || echo 'No package.json found, skipping npm install'",
    "git config --local init.defaultBranch main || echo 'Using existing default branch'"
  ],
  "terminalCommand": "$TERMINAL_CMD",
  "deleteBranchWithWorktree": true
}
EOF

success ".branchlet.json created successfully!"
echo
info "Configuration details:"
echo "  Repository: $REPO_NAME"
echo "  Worktree location: ~/Projects/github.com/yandy-r/worktrees/$REPO_NAME/<branch-name>"
echo
info "Next steps:"
echo "  1. Create a worktree using Branchlet"
echo "  2. Worktrees will be created in: ../worktrees/$REPO_NAME/<branch-name>"
echo
