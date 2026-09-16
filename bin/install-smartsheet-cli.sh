#!/usr/bin/env bash
# ================================================================
# Smartsheet CLI Installation Script
# ================================================================
# Installs dependencies for the Smartsheet CLI tool
# ================================================================

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

# Get the directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

info "Installing Smartsheet CLI dependencies..."

# Check if Node.js is installed
if ! command -v node &>/dev/null; then
  error "Node.js is not installed!"
  echo "Please install Node.js 18+ first:"
  echo "  brew install node"
  exit 1
fi

# Check Node.js version
NODE_VERSION=$(node --version | cut -d'v' -f2 | cut -d'.' -f1)
if [[ $NODE_VERSION -lt 18 ]]; then
  error "Node.js version 18 or higher is required (you have v${NODE_VERSION})"
  echo "Please upgrade Node.js:"
  echo "  brew upgrade node"
  exit 1
fi

success "Node.js $(node --version) detected"

# Navigate to bin directory
cd "$SCRIPT_DIR"

# Install dependencies
info "Installing npm packages..."
npm install

success "Dependencies installed successfully!"

# Make the CLI executable
chmod +x smartsheet-cli.ts

# Create symbolic link
LINK_PATH="$HOME/.local/bin/smartsheet-cli"

if [[ -L "$LINK_PATH" ]]; then
  warn "Symbolic link already exists, updating..."
  rm "$LINK_PATH"
fi

info "Creating symbolic link to /usr/local/bin/smartsheet-cli..."
ln -s "$SCRIPT_DIR/smartsheet-cli.ts" "$LINK_PATH"

success "Smartsheet CLI installed successfully!"
echo
info "Next steps:"
echo "  1. Set your API key in .env:"
echo "     SMARTSHEETS_API_KEY=\"your-api-key-here\""
echo
echo "  2. Get your API key from Smartsheet:"
echo "     https://app.smartsheet.com/user/account -> API Access"
echo
echo "  3. Try the CLI:"
echo "     smartsheet-cli list templates"
echo "     smartsheet-cli list sheets"
echo "     smartsheet-cli create sheet \"My Project\" -i"
echo
