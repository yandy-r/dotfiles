# Shell Configuration Guide

This guide covers how to use the dotfiles shell configuration with both bash and zsh, including feature parity between shells, customization options, and prompt configuration.

## Overview

The dotfiles repository provides a unified cross-platform shell configuration system that works identically in both bash and zsh. Both shells load the same common configuration files, ensuring consistent behavior regardless of which shell you use.

## Shell Initialization Flow

When you start a new shell session, the following files are loaded in order:

```
bash/bashrc (bash) OR zsh/zshrc (zsh)
    |
    +-- common/platform.sh    (OS/architecture detection)
    +-- common/exports.sh     (environment variables)
    +-- common/paths.sh       (PATH construction)
    +-- common/aliases.sh     (command shortcuts)
    +-- common/functions.sh   (utility functions)
    +-- common/tools.sh       (tool setup and detection)
```

## Choosing Between Bash and Zsh

Both shells are fully supported with equivalent features. Choose based on your preference:

| Feature             | Bash          | Zsh                    |
| ------------------- | ------------- | ---------------------- |
| Vi Mode             | Yes           | Yes                    |
| Modern Keybindings  | Yes           | Yes                    |
| Starship Prompt     | Yes           | Yes                    |
| Tab Completion      | Yes           | Yes (enhanced styling) |
| Autosuggestions     | Via bash-it   | Native plugin          |
| Syntax Highlighting | Via bash-it   | Native plugin          |
| Globstar (`**`)     | Yes (bash 4+) | Yes                    |
| Auto-cd             | Yes (bash 4+) | Yes                    |

### Setting Your Default Shell

```bash
# Check current shell
echo $SHELL

# Change to zsh (recommended for macOS)
chsh -s /bin/zsh

# Change to bash (use Homebrew's modern bash on macOS)
chsh -s /opt/homebrew/bin/bash  # Apple Silicon
chsh -s /usr/local/bin/bash      # Intel Mac
```

## Feature Parity Between Shells

### Vi Mode

Both shells are configured with vi mode for efficient command-line editing:

```bash
# Both shells support:
# - Press 'Escape' or 'jj' to enter normal mode
# - 'i' to enter insert mode
# - 'h', 'l', 'w', 'b' for navigation
# - 'dd', 'yy', 'p' for editing
# - '/' for reverse search in normal mode
```

**Configuration location:**

- Bash: `bash/bashrc` (lines 76-80)
- Zsh: `zsh/zshrc` (lines 35-39)

### Enhanced Keybindings

Available in both shells:

| Key          | Action                     |
| ------------ | -------------------------- |
| `jj`         | Exit insert mode (vi mode) |
| `Ctrl+A`     | Beginning of line          |
| `Ctrl+E`     | End of line                |
| `Up Arrow`   | History search backward    |
| `Down Arrow` | History search forward     |
| `Tab`        | Completion with menu       |

### History Settings

Both shells share the same history configuration via `common/exports.sh`:

```bash
HISTSIZE=50000          # Commands in memory
HISTFILESIZE=100000     # Commands in file
HISTCONTROL=ignoreboth  # Ignore duplicates and space-prefixed
```

Zsh-specific enhancements:

- Extended history with timestamps
- Share history between sessions
- Immediate history append

## Starship Prompt

Both shells use Starship as the primary prompt, configured in `starship.toml`:

### Prompt Segments

The prompt displays (left to right):

1. **OS icon** - macOS/Linux/Ubuntu indicator
2. **Username@hostname** - Current user and machine
3. **Directory** - Current path (truncated to 3 levels)
4. **Git status** - Branch, staged, modified, stashed
5. **Language versions** - Node.js, Python, Go, Rust (when relevant)
6. **Cloud context** - Kubernetes, Docker, AWS (when relevant)

### Color Theme

The prompt uses the Catppuccin Mocha color palette for a consistent, modern look.

### Customizing Starship

Edit `~/.config/dotfiles/starship.toml` to customize:

```toml
# Example: Disable Kubernetes context
[kubernetes]
disabled = true

# Example: Change directory truncation
[directory]
truncation_length = 5

# Example: Show command duration for commands over 5 seconds
[cmd_duration]
min_time = 5000
```

Reload your shell to apply changes:

```bash
exec $SHELL -l
```

### Fallback Prompt (Bash Only)

If Starship is unavailable, bash falls back to a custom prompt defined in `bash/prompt.sh`:

- Shows user@host and current directory
- Git integration (branch name and status indicators)
- Exit code indicator for failed commands
- Vi mode indicator (Normal/Insert)

## Shell-Specific Settings

### Bash-Specific Options

Located in `bash/bashrc`:

```bash
shopt -s histappend    # Append to history
shopt -s checkwinsize  # Update terminal size
shopt -s cdspell       # Auto-correct cd typos
shopt -s dotglob       # Include dotfiles in globs
shopt -s extglob       # Extended pattern matching
shopt -s nocaseglob    # Case-insensitive globbing

# Bash 4+ features
shopt -s globstar      # Enable ** globbing
shopt -s dirspell      # Correct directory spelling
shopt -s autocd        # cd without typing cd
```

### Zsh-Specific Options

Located in `zsh/zshrc`:

```zsh
setopt AUTO_CD              # Change directory without cd
setopt EXTENDED_GLOB        # Extended globbing
setopt GLOB_DOTS            # Include dotfiles
setopt PROMPT_SUBST         # Parameter expansion in prompts
setopt EXTENDED_HISTORY     # Record timestamps
setopt SHARE_HISTORY        # Share between sessions
setopt HIST_VERIFY          # Verify before executing
```

## Adding Personal Customizations

**Important**: Do not modify the core dotfiles directly. Instead, use local override files.

### Bash Customizations

Create `~/.bashrc.local`:

```bash
# Personal bash customizations
# This file is sourced at the end of bashrc

# Example: Add personal aliases
alias myproject="cd ~/Projects/my-project"

# Example: Add personal functions
my_function() {
    echo "Hello from my function"
}

# Example: Set personal environment variables
export MY_VAR="my-value"
```

### Zsh Customizations

Create `~/.zshrc.local`:

```zsh
# Personal zsh customizations
# This file is sourced at the end of zshrc

# Example: Add personal aliases
alias myproject="cd ~/Projects/my-project"

# Example: Add zsh-specific settings
setopt MENU_COMPLETE  # Auto-select first completion

# Example: Personal functions
my_function() {
    echo "Hello from my function"
}
```

### Both Shells

For customizations that should apply to both shells, add them to:

- `~/.aliases` - Personal aliases (sourced by bash)
- `~/.bash_aliases` - Alternative location (sourced by bash)

## Troubleshooting

### Slow Shell Startup

If your shell is slow to start:

1. **Profile startup time (zsh)**:

   ```zsh
   time zsh -i -c exit
   ```

2. **Profile startup time (bash)**:

   ```bash
   time bash -i -c exit
   ```

3. **Check for heavy completions**:
   Completions for kubectl, docker, helm are lazy-loaded to improve startup time.

4. **Disable debug output**:

   ```bash
   unset DOTFILES_DEBUG
   ```

### Testing Configuration Changes

Test without committing changes:

```bash
# Test bash configuration
bash -l -c 'echo "Bash config loaded"'

# Test zsh configuration
zsh -l -c 'echo "Zsh config loaded"'

# Reload current shell
exec $SHELL -l
```

### Checking Platform Detection

```bash
# Show detected values
source ~/.config/dotfiles/common/platform.sh
echo "OS: $DOTFILES_OS"
echo "Arch: $DOTFILES_ARCH"
echo "Package Manager: $DOTFILES_PKG_MGR"
echo "Homebrew Path: $DOTFILES_HOMEBREW_PATH"
```

### Common Issues

**Problem**: Old bash version on macOS (bash 3.x)

**Solution**: Install modern bash via Homebrew:

```bash
brew install bash
# Add to allowed shells
sudo bash -c 'echo /opt/homebrew/bin/bash >> /etc/shells'
# Set as default
chsh -s /opt/homebrew/bin/bash
```

**Problem**: Aliases not working

**Solution**: Check if the underlying command exists:

```bash
# Check if eza is installed (required for ls aliases)
command -v eza

# Check if bat is installed (required for cat aliases)
command -v bat
```

**Problem**: Vi mode not working

**Solution**: Ensure proper shell options are set:

```bash
# Bash
set -o vi

# Zsh
bindkey -v
```

## See Also

- [Modern CLI Tools Guide](modern-tools.md) - Tool integrations and aliases
- [Completion System Guide](completions.md) - Tab completion configuration
- [Environment Management Guide](environment-management.md) - direnv and atuin setup
