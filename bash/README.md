# Bash Configuration

This directory contains the bash shell configuration for the dotfiles repository. The configuration provides a modern bash experience with feature parity to zsh, including vi mode, enhanced history, and intelligent completions.

## Files

| File           | Purpose                                     | Symlinked To      |
| -------------- | ------------------------------------------- | ----------------- |
| `bashrc`       | Main interactive shell configuration        | `~/.bashrc`       |
| `bash_profile` | Login shell configuration                   | `~/.bash_profile` |
| `profile`      | Profile loader                              | `~/.profile`      |
| `prompt.sh`    | Custom git-aware prompt (Starship fallback) | Sourced by bashrc |
| `.fzf.bash`    | FZF keybindings for bash                    | Sourced by bashrc |

## How It Works

### Loading Order

1. **Login shell** (`~/.bash_profile` or `~/.profile`):
   - Sources `~/.bashrc`
   - Loads Cargo environment
   - Initializes direnv

2. **Interactive shell** (`~/.bashrc`):
   - Sources the common configuration system
   - Applies bash-specific settings
   - Initializes Starship prompt
   - Loads completions and tools

### Common System Integration

The `bashrc` sources the shared configuration modules:

```bash
source "$DOTFILES_ROOT/common/platform.sh"   # Platform detection
source "$DOTFILES_ROOT/common/exports.sh"    # Environment variables
source "$DOTFILES_ROOT/common/paths.sh"      # PATH management
source "$DOTFILES_ROOT/common/aliases.sh"    # Shell aliases
source "$DOTFILES_ROOT/common/functions.sh"  # Utility functions
source "$DOTFILES_ROOT/common/tools.sh"      # Tool setup
```

This provides feature parity with zsh while maintaining bash-specific optimizations.

## Features

### Shell Options

The configuration enables these bash options:

```bash
shopt -s histappend     # Append to history file
shopt -s checkwinsize   # Update LINES and COLUMNS
shopt -s cdspell        # Correct minor cd spelling errors
shopt -s dotglob        # Include dotfiles in pathname expansion
shopt -s extglob        # Extended pattern matching
shopt -s nocaseglob     # Case-insensitive globbing

# Bash 4.0+ only
shopt -s globstar       # Enable ** globbing
shopt -s dirspell       # Correct directory spelling errors
shopt -s autocd         # Change directory without cd
```

### Vi Mode

Vi mode is enabled by default with these keybindings:

```bash
set -o vi                                    # Enable vi mode
bind -m vi-insert '"jj": vi-movement-mode'   # jj to exit insert mode
```

**Navigation keys:**

- `jj` - Exit insert mode (same as Escape)
- `h`, `j`, `k`, `l` - Navigate in normal mode
- `w`, `b` - Word navigation
- `0`, `$` - Line start/end

### History Search

History search with arrow keys:

```bash
bind '"\e[A": history-search-backward'  # Up arrow
bind '"\e[B": history-search-forward'   # Down arrow
```

Type part of a command and press Up/Down to search history.

### Enhanced Completion

Completion settings for better UX:

```bash
bind 'set completion-ignore-case on'        # Case insensitive
bind 'set show-all-if-ambiguous on'         # Show matches immediately
bind 'set show-all-if-unmodified on'        # Show matches on second Tab
bind 'set colored-stats on'                  # Colored file types
bind 'set visible-stats on'                  # File type indicators
bind 'set mark-symlinked-directories on'    # Slash after symlinked dirs
bind 'set colored-completion-prefix on'     # Colored matching prefix
bind 'set menu-complete-display-prefix on'  # Common prefix in menu
```

### Starship Prompt

The default prompt is [Starship](https://starship.rs/), providing:

- Fast rendering
- Git status and branch information
- Language version detection
- Command execution time
- Cross-shell consistency

If Starship is not available, a custom git-aware prompt is used as fallback.

## Customization

### Local Customizations

Add machine-specific configurations to `~/.bashrc.local`:

```bash
# ~/.bashrc.local
export MY_CUSTOM_VAR="value"
alias myalias="my-command"
```

This file is sourced at the end of bashrc and is not tracked by git.

### Adding Aliases

For cross-platform aliases (recommended):

```bash
# Edit common/aliases.sh
alias myalias="my-command"
```

For bash-only aliases:

```bash
# Edit ~/.bashrc.local
alias bash-only-alias="command"
```

### Adding Functions

For cross-platform functions (recommended):

```bash
# Edit common/functions.sh
my_function() {
    echo "Hello from my function"
}
```

For bash-only functions:

```bash
# Edit ~/.bashrc.local
my_bash_function() {
    echo "Bash only"
}
```

## Tool Integration

### Atuin (History)

[Atuin](https://atuin.sh/) provides enhanced shell history with sync:

```bash
# Initialized automatically if installed
eval "$(atuin init bash)"
```

Use Ctrl+R for fuzzy history search.

### Direnv (Environment)

[Direnv](https://direnv.net/) automatically loads `.envrc` files:

```bash
# Initialized automatically if installed
eval "$(direnv hook bash)"
```

See the main [CLAUDE.md](../CLAUDE.md) for direnv usage instructions.

### FZF (Fuzzy Finder)

[FZF](https://github.com/junegunn/fzf) provides fuzzy finding:

- `Ctrl+R` - Fuzzy history search (if Atuin not installed)
- `Ctrl+T` - Fuzzy file search
- `Alt+C` - Fuzzy directory change

## Troubleshooting

### Old Bash Version Warning

If you see a warning about bash version:

```
Warning: Using bash 3.x. For best experience, install modern bash:
   brew install bash && chsh -s /opt/homebrew/bin/bash
```

Install modern bash for full feature support:

```bash
# macOS
brew install bash
sudo bash -c 'echo /opt/homebrew/bin/bash >> /etc/shells'
chsh -s /opt/homebrew/bin/bash

# Linux (usually has bash 4+ already)
# Check version: bash --version
```

### Slow Shell Startup

If bash starts slowly:

1. **Check for slow tools:**

   ```bash
   time bash -i -c exit
   ```

2. **Enable debug mode:**

   ```bash
   export DOTFILES_DEBUG=1
   exec bash
   ```

3. **Check for network issues** - Some completions fetch data

### Completions Not Working

If tab completion is broken:

```bash
# Check if completion is loaded
shopt -q progcomp && echo "Completion enabled"

# Force reload completions
source ~/.bashrc
```

### Vi Mode Issues

If vi mode keybindings conflict:

```bash
# Check current mode
set -o | grep vi

# Reset to emacs mode if needed
set -o emacs
```

## Testing

### Test configuration loads correctly

```bash
bash -l -c 'echo "Bash config loaded successfully"'
```

### Test in isolation

```bash
# Start bash without loading config
bash --norc --noprofile

# Manually source and test
source ~/.bashrc
```

### Reload configuration

```bash
exec bash -l
```

## Comparison with Zsh

| Feature             | Bash        | Zsh            |
| ------------------- | ----------- | -------------- |
| Vi mode             | Yes         | Yes            |
| History search      | Yes         | Yes            |
| Starship prompt     | Yes         | Yes            |
| Tab completion      | Yes (basic) | Yes (advanced) |
| Autosuggestions     | No          | Yes (plugin)   |
| Syntax highlighting | No          | Yes (plugin)   |
| Glob patterns       | Extended    | More powerful  |

Both shells source the same `common/` modules, providing identical:

- Aliases
- Functions
- Environment variables
- PATH configuration
- Tool integrations

## Related Documentation

- [Common System](../common/README.md) - Shared configuration modules
- [Zsh Configuration](../zsh/README.md) - Zsh equivalent documentation
- [Shell Configuration Guide](../docs/features/shell-configuration.md) - Detailed guide
- [CLAUDE.md](../CLAUDE.md) - Full technical documentation
