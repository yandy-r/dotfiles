# Modern CLI Tools Guide

This guide covers the modern CLI tools integrated into the dotfiles, including installation, aliases, and how fallbacks work when tools are not installed.

## Overview

The dotfiles integrate several modern replacements for traditional Unix tools. These provide better output formatting, syntax highlighting, and improved functionality while maintaining backwards compatibility through smart fallbacks.

## Tool Summary

| Modern Tool  | Replaces | Key Benefits                           |
| ------------ | -------- | -------------------------------------- |
| eza          | ls       | Icons, git integration, colors         |
| bat          | cat      | Syntax highlighting, git integration   |
| ripgrep (rg) | grep     | Faster, smarter defaults               |
| fd           | find     | Simpler syntax, respects .gitignore    |
| fzf          | -        | Fuzzy finding for files, history       |
| delta        | diff     | Syntax highlighting, side-by-side view |
| zoxide       | cd       | Smart directory jumping                |

## eza (Better ls)

eza is a modern replacement for `ls` with colors, icons, and git integration.

### Installation

```bash
# macOS
brew install eza

# Ubuntu/Debian
apt install eza

# Fedora
dnf install eza

# Arch Linux
pacman -S eza
```

### Integrated Aliases

When eza is installed, these aliases are automatically configured:

| Alias  | Command                                                           | Description                      |
| ------ | ----------------------------------------------------------------- | -------------------------------- |
| `ls`   | `eza -lg --group-directories-first --color always --icons always` | List with icons                  |
| `la`   | `eza -lg --all ...`                                               | List all files including hidden  |
| `ll`   | `eza -alg ...`                                                    | Long list all files              |
| `ld`   | `eza -lDg ...`                                                    | List directories only            |
| `lf`   | `eza -lfg ...`                                                    | List files only                  |
| `lh`   | `eza -dlg .*`                                                     | List hidden files only           |
| `lt`   | `eza -alg ... --sort=modified`                                    | List sorted by modification time |
| `tree` | `eza --tree ...`                                                  | Tree view of directories         |

### Usage Examples

```bash
# List with full details
ll

# Show hidden files
la

# Tree view with 2 levels deep
eza --tree --level=2

# Show git status for files
eza -lg --git

# List by modification time
lt
```

### Fallback Behavior

When eza is **not** installed, the aliases automatically fall back to standard `ls` with appropriate options:

```bash
# macOS fallback (uses -G for colors)
alias ls="ls -G"
alias la="ls -laG"

# Linux fallback
alias ls="ls --color=auto"
alias la="ls -la --color=auto"
```

## bat (Better cat)

bat provides syntax highlighting for viewing files, with git integration and line numbers.

### Installation

```bash
# macOS
brew install bat

# Ubuntu/Debian (note: binary is named 'batcat')
apt install bat
ln -sf /usr/bin/batcat ~/.local/bin/bat

# Fedora (ships /usr/bin/bat — no rename shim needed)
dnf install bat

# Arch Linux
pacman -S bat
```

### Configuration

Environment variables set by the dotfiles:

```bash
# Theme for syntax highlighting
export BAT_THEME="Sublime Snazzy"

# Pager settings
export BAT_PAGER="less -RF"

# Use bat as man page viewer
export MANPAGER="sh -c 'col -bx | bat -l man -p'"
```

### Integrated Aliases

| Alias  | Command               | Description              |
| ------ | --------------------- | ------------------------ |
| `bat`  | `bat --paging=never`  | View file without paging |
| `less` | `bat --paging=always` | View file with paging    |

### Usage Examples

```bash
# View a file with syntax highlighting
bat script.sh

# View multiple files
bat file1.py file2.py

# Show line numbers
bat -n script.sh

# Show non-printable characters
bat -A file.txt

# Compare two files
bat -d file1.txt file2.txt

# View file with specific language highlighting
bat -l python data.txt
```

### Fallback Behavior

When bat is not installed, `cat` is used directly without any modifications.

## ripgrep (Better grep)

ripgrep (`rg`) is an extremely fast search tool that respects `.gitignore` and provides smart defaults.

### Installation

```bash
# macOS
brew install ripgrep

# Ubuntu/Debian
apt install ripgrep

# Fedora
dnf install ripgrep

# Arch Linux
pacman -S ripgrep
```

### Configuration

The dotfiles create a config file at `~/.config/ripgrep/config`:

```
--hidden
--follow
--smart-case
--glob=!.git/*
--glob=!node_modules/*
--glob=!.vscode/*
--glob=!*.lock
--colors=match:fg:yellow
--colors=match:style:bold
--colors=path:fg:green
--colors=line:fg:blue
```

### Integrated Aliases

| Alias      | Command                    | Description            |
| ---------- | -------------------------- | ---------------------- |
| `rgr`      | `rg --hidden --smart-case` | Explicit ripgrep alias |
| `rg` (zsh) | `rg --hidden --smart-case` | Enhanced default       |

### Usage Examples

```bash
# Basic search
rg "function"

# Search specific file type
rg "TODO" -t py

# Search with context (lines before/after)
rg "error" -B 2 -A 2

# Search hidden files
rg --hidden "config"

# Case-sensitive search
rg -s "MyClass"

# Search and replace (preview)
rg "old_name" --replace "new_name"

# Count matches
rg -c "function"

# List files containing matches
rg -l "TODO"
```

### Fallback Behavior

When ripgrep is not installed, standard `grep` is used with color auto-detection:

```bash
alias grep="grep --color=auto"
alias fgrep="fgrep --color=auto"
alias egrep="egrep --color=auto"
```

## fd (Better find)

fd is a simple, fast alternative to `find` with sensible defaults.

### Installation

```bash
# macOS
brew install fd

# Ubuntu/Debian (note: binary is named 'fdfind')
apt install fd-find
ln -sf /usr/bin/fdfind ~/.local/bin/fd

# Fedora (fd-find ships /usr/bin/fd — no rename shim needed)
dnf install fd-find

# Arch Linux
pacman -S fd
```

### Integrated Functions

The dotfiles provide these wrapper functions in `common/functions.sh`:

```bash
# Find files by name
f() {
    if has_command fd; then
        fd "$@"
    else
        find . -name "*$1*" -type f
    fi
}

# Find directories by name
fd_dir() {
    if has_command fd; then
        fd --type d "$@"
    else
        find . -name "*$1*" -type d
    fi
}
```

### Usage Examples

```bash
# Find files named "config"
fd config

# Find specific extension
fd -e py

# Find directories only
fd -t d src

# Find hidden files
fd -H .bashrc

# Find and execute command
fd -e log -x rm {}

# Find with regex
fd '^test.*\.py$'

# Exclude directories
fd config -E node_modules

# Find files modified in last 24 hours
fd --changed-within 24h
```

### Fallback Behavior

When fd is not installed, the `f` and `fd_dir` functions automatically use `find`:

```bash
# Falls back to
find . -name "*pattern*" -type f
```

## fzf (Fuzzy Finder)

fzf is a general-purpose fuzzy finder that integrates with shell history, file finding, and more.

### Installation

```bash
# macOS
brew install fzf
$(brew --prefix)/opt/fzf/install

# Ubuntu/Debian
apt install fzf

# Fedora
dnf install fzf

# Arch Linux
pacman -S fzf
```

### Configuration

The dotfiles set up fzf integration in `common/tools.sh`:

```bash
# Key bindings enabled:
# Ctrl+T - File search
# Ctrl+R - History search (if atuin not installed)
# Alt+C  - Directory change
```

### Usage Examples

```bash
# Interactive file search
fzf

# Preview files while searching
fzf --preview 'bat --color=always {}'

# Search and open in editor
vim $(fzf)

# Search git commits
git log --oneline | fzf

# Kill process interactively
kill -9 $(ps aux | fzf | awk '{print $2}')

# Change to directory
cd $(fd -t d | fzf)

# Search command history (Ctrl+R)
# Type partial command, select with arrow keys
```

### Integration with Other Tools

fzf integrates with:

- **Git**: Branch switching, commit selection
- **History**: Fuzzy search through command history
- **cd**: Interactive directory navigation
- **Environment**: Variable selection

## delta (Better diff)

delta provides syntax-highlighted diffs with side-by-side view support.

### Installation

```bash
# macOS
brew install git-delta

# Ubuntu/Debian
# Download from GitHub releases

# Fedora (the RPM is git-delta; an unrelated package is named 'delta' — do not shorten)
dnf install git-delta

# Arch Linux
pacman -S git-delta
```

### Configuration

The dotfiles automatically configure git to use delta:

```bash
# Set as git pager
export GIT_PAGER="delta"

# Git config settings (set by tools.sh)
git config --global core.pager delta
git config --global interactive.diffFilter "delta --color-only"
git config --global delta.navigate true
git config --global delta.light false
```

### Usage Examples

```bash
# View git diff with delta (automatic)
git diff

# Side-by-side view
git diff --side-by-side

# Compare two files
delta file1.txt file2.txt

# Use with bat for file comparison
diff -u file1.txt file2.txt | delta
```

### Fallback Behavior

The dotfiles provide a `compare` function that falls back to standard diff:

```bash
compare() {
    if has_command delta; then
        delta "$1" "$2"
    elif has_command diff; then
        diff -u "$1" "$2" | (has_command bat && bat -l diff || cat)
    fi
}
```

## zoxide (Smart cd)

zoxide is a smarter `cd` command that learns your most-used directories.

### Installation

```bash
# macOS
brew install zoxide

# Ubuntu/Debian
apt install zoxide

# Fedora
dnf install zoxide

# Arch Linux
pacman -S zoxide
```

### Configuration

zoxide is initialized in `common/tools.sh` and uses the `z` command (does not replace `cd`):

```bash
# Bash
eval "$(zoxide init bash)"

# Zsh
eval "$(zoxide init zsh)"
```

### Integrated Aliases

| Alias | Command         | Description                    |
| ----- | --------------- | ------------------------------ |
| `zi`  | `__zoxide_zi`   | Interactive selection with fzf |
| `zq`  | `zoxide query`  | Query the database             |
| `za`  | `zoxide add`    | Add directory to database      |
| `zr`  | `zoxide remove` | Remove directory from database |

### Usage Examples

```bash
# Jump to frequently used directory
z projects

# Jump to directory matching pattern
z dot    # Jumps to ~/.config/dotfiles

# Interactive selection
zi

# Query without jumping
zoxide query projects

# Add current directory manually
zoxide add .

# List known directories
zoxide query -l

# Remove directory from database
zoxide remove /old/path
```

## Fallback System

The dotfiles implement a smart fallback system that checks for tool availability:

```bash
# Example from common/aliases.sh
if has_command eza; then
    alias ls="eza -lg --group-directories-first --color always --icons always"
else
    if is_macos; then
        alias ls="ls -G"
    else
        alias ls="ls --color=auto"
    fi
fi
```

### Checking Tool Availability

```bash
# Check if a tool exists
has_command eza && echo "eza is installed"

# Check tool health
tool_health eza

# List missing recommended tools
list_missing_tools

# Full system health check
system_health
```

### Installing Missing Tools

```bash
# Install individual tool
install_tool eza

# Install all modern CLI tools
install_modern_tools

# Install all missing tools
install_missing_tools
```

## See Also

- [Shell Configuration Guide](shell-configuration.md) - Shell setup and customization
- [Completion System Guide](completions.md) - Tab completion for these tools
- [Environment Management Guide](environment-management.md) - direnv and atuin setup
