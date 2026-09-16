# Zoxide - Smart Directory Navigation

Zoxide is a blazingly fast directory jumper that tracks your most used directories, allowing you to navigate to them with minimal keystrokes. It's a Rust-based replacement for autojump and other directory jumping tools.

## Overview

Zoxide learns which directories you use most frequently and allows you to jump to them by typing just part of their name. Instead of typing `cd /home/user/projects/my-awesome-project`, you can simply type `z awesome` and zoxide will take you there.

## Migration from autojump

This dotfiles repository has migrated from autojump to zoxide for several compelling reasons:

### Why Zoxide?

1. **Performance**: Written in Rust, zoxide is significantly faster than autojump (written in Python)
2. **Better Algorithm**: Uses a more sophisticated frecency algorithm that considers both frequency and recency
3. **Modern Features**:
   - Interactive selection with fzf integration
   - Better cross-shell support
   - More configuration options
4. **Active Development**: Zoxide is actively maintained with regular updates
5. **Lower Dependencies**: Single binary with no runtime dependencies

### Breaking Changes

The command prefix has changed:

- **autojump**: `j` command
- **zoxide**: `z` command (with `zi` for interactive mode)

If you were using autojump, you'll need to adjust your muscle memory:

```bash
# Old (autojump)
j projects

# New (zoxide)
z projects
```

## Installation

Zoxide is automatically installed when you run the bootstrap script:

```bash
./bootstrap.sh
```

### Manual Installation

You can manually install zoxide using the Homebrew script:

```bash
# Install zoxide via Homebrew
./bin/install-brew.sh

# Or install directly with Homebrew
brew install zoxide
```

### Verification

Check that zoxide is installed and configured:

```bash
# Check installation
which zoxide
zoxide --version

# Verify shell integration
type z
```

## Usage

### Basic Navigation

Once installed, zoxide automatically tracks directories you visit. Simply `cd` to directories as you normally would, and zoxide builds its database:

```bash
cd ~/projects/dotfiles
cd ~/documents/work/reports
cd ~/code/rust/my-app
```

After visiting directories, you can jump to them using `z`:

```bash
# Jump to a directory by name
z dotfiles        # Takes you to ~/projects/dotfiles

# Jump using partial matches
z rep             # Takes you to ~/documents/work/reports

# Jump using multiple keywords
z rust app        # Takes you to ~/code/rust/my-app
```

### Interactive Mode

Use `zi` to interactively select from matching directories using fzf:

```bash
# Interactive selection
zi project        # Shows list of directories matching "project"

# Interactive selection from all directories
zi                # Browse all tracked directories
```

### Advanced Features

#### Query Without Changing Directory

```bash
# Print the best match without changing directory
zoxide query dotfiles

# Print all matches
zoxide query --list projects

# Print match scores
zoxide query --score rust
```

#### Manual Database Management

```bash
# Add directory to database without visiting
zoxide add /path/to/directory

# Remove directory from database
zoxide remove /path/to/directory

# Edit database directly
zoxide edit
```

#### Import from Other Tools

If you're migrating from autojump or other directory jumpers:

```bash
# Import from autojump
zoxide import --from autojump

# Import from z (bash/zsh z script)
zoxide import --from z
```

## Configuration

### Environment Variables

Customize zoxide behavior using environment variables in your shell configuration:

```bash
# Set custom data directory (default: $XDG_DATA_HOME/zoxide or ~/.local/share/zoxide)
export _ZO_DATA_DIR="$HOME/.config/zoxide"

# Print matched directory before navigating (useful for learning)
export _ZO_ECHO=1

# Exclude certain directories from tracking
export _ZO_EXCLUDE_DIRS="$HOME/temp:$HOME/.cache"

# Custom fzf options for interactive mode
export _ZO_FZF_OPTS="--height=40% --reverse --border"

# Maximum age before old entries are deleted (default: 10000)
export _ZO_MAXAGE=5000

# Resolve symlinks when storing paths (default: false)
export _ZO_RESOLVE_SYMLINKS=1
```

### Custom Command Prefix

If you prefer different command names, you can customize the prefix during initialization:

```bash
# Use 'j' instead of 'z' (like autojump)
eval "$(zoxide init bash --cmd j)"
eval "$(zoxide init zsh --cmd j)"

# Use 'cd' to completely replace cd command
eval "$(zoxide init bash --cmd cd)"
```

**Note**: The dotfiles use the default `z` command. To customize, edit the setup function in `/home/yandy/.config/dotfiles/common/tools.sh`.

## Integration with Dotfiles

Zoxide is integrated into this dotfiles repository in several ways:

### Automatic Shell Integration

The shell integration is handled by `common/tools.sh`:

```bash
# Zoxide is automatically initialized in both bash and zsh
setup_zoxide() {
    if tool_exists zoxide; then
        if [[ -n "${ZSH_VERSION:-}" ]]; then
            eval "$(zoxide init zsh)"
        elif [[ -n "${BASH_VERSION:-}" ]]; then
            eval "$(zoxide init bash)"
        fi
    fi
}
```

### Tool Detection

Zoxide is registered as a modern tool:

```bash
# Check if zoxide is installed
tool_exists zoxide

# Get zoxide version
tool_version zoxide

# Listed in system health check
system_health
```

### Bootstrap Integration

Zoxide is automatically installed during initial setup:

```bash
./bootstrap.sh
```

## Common Commands

### Quick Reference

```bash
# Navigate to a directory
z project                    # Jump to best match for "project"
z doc work                   # Jump using multiple keywords
zi                          # Interactive selection with fzf

# Query database
zoxide query rust            # Print best match
zoxide query --list web      # List all matches
zoxide query --score app     # Show matches with scores

# Manage database
zoxide add /path/to/dir      # Add directory manually
zoxide remove /path/to/dir   # Remove directory
zoxide edit                  # Edit database directly

# Import from other tools
zoxide import --from autojump
zoxide import --from z
```

## Tips and Best Practices

### 1. Let It Learn

The more you use `cd`, the smarter zoxide becomes. Don't force `zoxide add` for everything - let it learn naturally:

```bash
# Good: Natural learning
cd ~/projects/my-app
cd ~/projects/my-app/src
cd ~/documents/work/reports

# Later...
z app                        # Takes you to ~/projects/my-app
z reports                    # Takes you to ~/documents/work/reports
```

### 2. Use Multiple Keywords

When directories have similar names, use multiple keywords:

```bash
# If you have:
# ~/projects/client-app
# ~/projects/server-app
# ~/work/client-dashboard

z client proj                # Goes to ~/projects/client-app
z client work                # Goes to ~/work/client-dashboard
z server                     # Goes to ~/projects/server-app
```

### 3. Interactive Mode for Ambiguity

When you're not sure which match you'll get, use interactive mode:

```bash
zi app                       # Browse all directories matching "app"
```

### 4. Combine with Other Tools

Zoxide works great with fzf and other modern CLI tools:

```bash
# Use with fzf to fuzzy find and cd
cd "$(zoxide query --list | fzf)"

# Use in scripts
PROJECT_DIR=$(zoxide query my-project)
cd "$PROJECT_DIR"
```

### 5. Exclude Temporary Directories

Prevent zoxide from tracking temporary or cache directories:

```bash
export _ZO_EXCLUDE_DIRS="$HOME/.cache:$HOME/Downloads:$HOME/temp"
```

## Troubleshooting

### Command Not Found

If `z` or `zi` commands are not found:

1. **Check installation**: `which zoxide`
2. **Verify shell integration**: `type z`
3. **Reload shell**: `exec $SHELL -l` or open a new terminal
4. **Check tool setup**: Ensure `setup_zoxide` is called in your shell config

### Not Jumping to Expected Directory

If zoxide isn't taking you where you expect:

1. **Check the database**: `zoxide query --list keyword`
2. **View scores**: `zoxide query --score keyword`
3. **Use more specific keywords**: `z specific keyword`
4. **Use interactive mode**: `zi keyword`
5. **Manually add directory**: `zoxide add /path/to/preferred/dir`

### Database Issues

If the database seems corrupted or stale:

```bash
# Edit database manually
zoxide edit

# Remove specific entries
zoxide remove /path/to/old/directory

# Start fresh (careful - this removes all history!)
rm -rf "${_ZO_DATA_DIR:-$HOME/.local/share/zoxide}"
```

### Migration from autojump

If you want to import your autojump history:

```bash
# Import from autojump
zoxide import --from autojump

# Verify import
zoxide query --list
```

## Comparison with Alternatives

### vs. autojump

| Feature          | zoxide        | autojump          |
| ---------------- | ------------- | ----------------- |
| Language         | Rust          | Python            |
| Performance      | Faster        | Slower            |
| Algorithm        | Frecency      | Frequency         |
| Interactive Mode | Built-in (zi) | Via `jc` with fzf |
| Maintenance      | Active        | Less active       |
| Dependencies     | None          | Python runtime    |

### vs. z (bash/zsh script)

| Feature          | zoxide          | z             |
| ---------------- | --------------- | ------------- |
| Language         | Rust            | Shell script  |
| Performance      | Much faster     | Slower        |
| Cross-shell      | Yes (8+ shells) | Bash/zsh only |
| Interactive Mode | Built-in        | Separate      |
| Configuration    | Rich            | Limited       |

### vs. fasd

| Feature        | zoxide           | fasd                  |
| -------------- | ---------------- | --------------------- |
| Focus          | Directories only | Files and directories |
| Performance    | Faster           | Slower                |
| Maintenance    | Active           | Unmaintained          |
| Learning Curve | Simple           | More complex          |

## Advanced Usage

### Shell Hooks

Customize when zoxide updates its database:

```bash
# Update on every prompt (default)
eval "$(zoxide init bash --hook prompt)"

# Update only on directory change
eval "$(zoxide init bash --hook pwd)"

# No automatic updates (manual only)
eval "$(zoxide init bash --hook none)"
```

### Scripting with Zoxide

Use zoxide in your scripts:

```bash
#!/bin/bash

# Jump to project directory
PROJECT=$(zoxide query my-project) || {
    echo "Project not found"
    exit 1
}

cd "$PROJECT"
# ... rest of your script
```

### Custom Aliases

Add convenience aliases in `common/aliases.sh`:

```bash
# Quick jump to common directories
alias zp='z projects'
alias zd='z dotfiles'
alias zw='z work'

# Interactive mode with preview
alias zz='zi'
```

## Resources

- **Official Repository**: <https://github.com/ajeetdsouza/zoxide>
- **Documentation**: <https://github.com/ajeetdsouza/zoxide/wiki>
- **Algorithm Details**: <https://github.com/ajeetdsouza/zoxide/wiki/Algorithm>

## Related Tools

This dotfiles repository also includes other navigation and productivity tools:

- **fzf**: Fuzzy finder (works great with `zi`)
- **eza**: Modern ls replacement with better file listing
- **bat**: Better cat with syntax highlighting
- **fd**: Fast file finder

For comprehensive tool documentation, see the main CLAUDE.md file.

---

**Navigate faster with zoxide!**
