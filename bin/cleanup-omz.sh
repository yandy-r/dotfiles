#!/usr/bin/env bash
# ================================================================
# Oh-My-Zsh Cleanup Script
# ================================================================
# Removes Oh-My-Zsh and Powerlevel10k, preparing for Starship

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $*"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

# ================================================================
# Safety Checks
# ================================================================

check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if we're in a dotfiles directory
    if [[ ! -d "$HOME/.config/dotfiles" ]]; then
        log_error "Dotfiles directory not found at $HOME/.config/dotfiles"
        log_info "This script expects dotfiles to be at: ~/.config/dotfiles"
        exit 1
    fi
    
    # Check if Starship is installed
    if ! command -v starship >/dev/null 2>&1; then
        log_warning "Starship is not installed!"
        log_info "Install it with: brew install starship"
        read -p "Continue anyway? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    else
        log_success "Starship is installed: $(starship --version | head -1)"
    fi
}

# ================================================================
# Cleanup Functions
# ================================================================

remove_omz_directory() {
    log_info "Checking for Oh-My-Zsh installation..."
    
    if [[ -d "$HOME/.oh-my-zsh" ]]; then
        log_info "Removing Oh-My-Zsh directory..."
        rm -rf "$HOME/.oh-my-zsh"
        log_success "Oh-My-Zsh directory removed"
    else
        log_info "No Oh-My-Zsh directory found (already clean)"
    fi
}

remove_p10k_config() {
    log_info "Checking for Powerlevel10k configuration..."
    
    if [[ -f "$HOME/.p10k.zsh" ]]; then
        log_info "Removing ~/.p10k.zsh..."
        rm -f "$HOME/.p10k.zsh"
        log_success "Powerlevel10k config removed"
    else
        log_info "No ~/.p10k.zsh found (already clean)"
    fi
}

clean_dotfiles_references() {
    log_info "Cleaning Oh-My-Zsh references from dotfiles..."
    
    cd "$HOME/.config/dotfiles"
    
    local files_to_clean=(
        "bootstrap.sh"
        "bin/install-brew.sh"
    )
    
    for file in "${files_to_clean[@]}"; do
        if [[ -f "$file" ]]; then
            log_info "Cleaning: $file"
            sed -i.bak '/oh-my-zsh/d; /omz/Id; /powerlevel10k/d; /p10k/d' "$file"
            rm -f "${file}.bak"
        fi
    done
}

remove_dotfiles_omz_configs() {
    log_info "Removing Oh-My-Zsh config files from dotfiles..."
    
    cd "$HOME/.config/dotfiles"
    
    if [[ -f "zsh/p10k.zsh" ]]; then
        log_info "Removing zsh/p10k.zsh..."
        rm -f "zsh/p10k.zsh"
    fi
    
    if [[ -d ".claude/shell-snapshots" ]]; then
        log_info "Removing .claude/shell-snapshots..."
        rm -rf ".claude/shell-snapshots"
    fi
}

update_documentation() {
    log_info "Updating documentation files..."
    
    cd "$HOME/.config/dotfiles"
    
    local doc_files=(
        "README.md"
        "CLAUDE.md"
        "playbooks/README.md"
    )
    
    for file in "${doc_files[@]}"; do
        if [[ -f "$file" ]]; then
            log_info "Updating: $file"
            sed -i.bak \
                -e 's/p10k configure/Starship prompt (already configured)/g' \
                -e 's/p10k\.zsh/starship.toml/g' \
                -e 's/Powerlevel10k/Starship/g' \
                -e 's/Oh-My-Zsh/Starship/g' \
                "$file"
            rm -f "${file}.bak"
        fi
    done
}

verify_starship_config() {
    log_info "Verifying Starship configuration..."
    
    if [[ -f "$HOME/.config/starship.toml" ]]; then
        if starship print-config >/dev/null 2>&1; then
            log_success "Starship configuration is valid"
        else
            log_error "Starship configuration has errors!"
            log_info "Fix it manually or regenerate with: starship config"
        fi
    else
        log_warning "No Starship configuration found at ~/.config/starship.toml"
        log_info "Create one or copy from: ~/.config/dotfiles/starship.toml"
    fi
}

# ================================================================
# Main Execution
# ================================================================

main() {
    echo ""
    log_info "═══════════════════════════════════════════════════"
    log_info "  Oh-My-Zsh Cleanup Script"
    log_info "═══════════════════════════════════════════════════"
    echo ""
    
    # Run checks
    check_prerequisites
    
    echo ""
    log_warning "This script will:"
    echo "  1. Remove ~/.oh-my-zsh directory"
    echo "  2. Remove ~/.p10k.zsh configuration"
    echo "  3. Clean OMZ references from dotfiles"
    echo "  4. Update documentation to reference Starship"
    echo ""
    read -p "Continue? (y/N) " -n 1 -r
    echo
    
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Cancelled by user"
        exit 0
    fi
    
    echo ""
    log_info "Starting cleanup..."
    echo ""
    
    # Perform cleanup
    remove_omz_directory
    remove_p10k_config
    clean_dotfiles_references
    remove_dotfiles_omz_configs
    update_documentation
    
    # Verify Starship
    echo ""
    verify_starship_config
    
    # Final summary
    echo ""
    log_info "═══════════════════════════════════════════════════"
    log_success "Cleanup Complete!"
    log_info "═══════════════════════════════════════════════════"
    echo ""
    log_info "Next steps:"
    echo "  1. Restart your shell: exec zsh"
    echo "  2. Verify Starship prompt appears"
    echo "  3. Customize: ~/.config/starship.toml"
    echo ""
    
    # Check for remaining references
    local remaining
    remaining=$(grep -r "oh-my-zsh\|p10k\|powerlevel" "$HOME/.config/dotfiles" \
        --include="*.sh" --include="*.zsh" --include="*.bash" \
        --include="zshrc" --include="bashrc" 2>/dev/null | \
        grep -v ".backup" | \
        grep -v ".omz-migration" | \
        grep -v "MIGRATION_SUMMARY" | \
        wc -l || echo "0")
    
    if [[ "$remaining" -eq 0 ]]; then
        log_success "✅ No OMZ references remaining in active configs"
    else
        log_warning "Found $remaining OMZ references in config files"
        log_info "Run: grep -r 'oh-my-zsh\\|p10k\\|powerlevel' ~/.config/dotfiles"
    fi
    
    echo ""
}

# Run main function
main "$@"
