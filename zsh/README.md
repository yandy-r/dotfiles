# Zsh Configuration

This directory contains the zsh shell configuration for the dotfiles repository. The configuration provides an enhanced zsh experience with modern features, plugins, and feature parity with the bash configuration.

## Files

| File                     | Purpose                                       | Symlinked To     |
| ------------------------ | --------------------------------------------- | ---------------- |
| `zshrc`                  | Main interactive shell configuration          | `~/.zshrc`       |
| `zprofile`               | Login shell configuration                     | `~/.zprofile`    |
| `functions.zsh`          | Legacy zsh functions (kept for compatibility) | Sourced by zshrc |
| `fzf.zsh`                | FZF keybindings for zsh                       | Sourced by zshrc |
| `completions/`           | Custom completion files                       | Added to FPATH   |
| `omz-migration-notes.md` | Migration notes from Oh My Zsh                | Reference only   |

## How It Works

### Loading Order

1. **Login shell** (`~/.zprofile`):
   - Sources platform detection
   - Sets up PATH (fixes macOS path_helper issues)
   - Ensures Homebrew paths are prioritized

2. **Interactive shell** (`~/.zshrc`):
   - Sources the common configuration system
   - Applies zsh-specific options and keybindings
   - Initializes completion system
   - Loads plugins (autosuggestions, syntax highlighting)
   - Initializes Starship prompt
   - Loads tool integrations

### Common System Integration

The `zshrc` sources the shared configuration modules:

```zsh
source "$DOTFILES_ROOT/common/platform.sh"   # Platform detection
source "$DOTFILES_ROOT/common/exports.sh"    # Environment variables
source "$DOTFILES_ROOT/common/paths.sh"      # PATH management
source "$DOTFILES_ROOT/common/aliases.sh"    # Shell aliases
source "$DOTFILES_ROOT/common/functions.sh"  # Utility functions
source "$DOTFILES_ROOT/common/tools.sh"      # Tool setup
```

This provides feature parity with bash while adding zsh-specific enhancements.

## Features

### Zsh Options

The configuration sets these zsh options:

```zsh
# Directory navigation
setopt AUTO_CD              # cd without typing cd
setopt EXTENDED_GLOB        # Extended globbing
setopt GLOB_DOTS            # Include dotfiles in globbing
setopt NOMATCH              # Error if pattern has no matches
setopt NOTIFY               # Report background jobs immediately
setopt PROMPT_SUBST         # Parameter expansion in prompts

# History
setopt EXTENDED_HISTORY     # Record timestamp and duration
setopt HIST_EXPIRE_DUPS_FIRST
setopt HIST_FIND_NO_DUPS
setopt HIST_IGNORE_SPACE    # Ignore commands starting with space
setopt HIST_IGNORE_DUPS
setopt HIST_IGNORE_ALL_DUPS
setopt HIST_SAVE_NO_DUPS
setopt HIST_VERIFY          # Show before executing from history
setopt SHARE_HISTORY        # Share history between sessions
setopt INC_APPEND_HISTORY   # Append immediately

# Completion
setopt AUTO_LIST            # List choices on ambiguous completion
setopt AUTO_MENU            # Show menu on successive Tab
setopt ALWAYS_TO_END        # Move cursor to end after completion
setopt COMPLETE_IN_WORD     # Complete from both ends

# Disabled
unsetopt CORRECT            # No "correct X to Y?" prompts
unsetopt CORRECT_ALL        # No argument correction
unsetopt MENU_COMPLETE      # Don't autoselect first entry
```

### Vi Mode

Vi mode is enabled with these keybindings:

```zsh
export KEYTIMEOUT=1         # Faster mode switching
bindkey -v                  # Enable vi mode
bindkey -M viins 'jj' vi-cmd-mode  # jj to exit insert mode
bindkey -M viins '^A' beginning-of-line
bindkey -M viins '^E' end-of-line
```

**Navigation keys:**

- `jj` - Exit insert mode (same as Escape)
- `Ctrl+A` - Beginning of line (insert mode)
- `Ctrl+E` - End of line (insert mode)
- Standard vi keys in normal mode

### Completion System

Enhanced completion with styling:

```zsh
zstyle ':completion:*' menu select              # Menu selection
zstyle ':completion:*' list-colors "${(s.:.)LS_COLORS}"  # Colored output
zstyle ':completion:*' matcher-list 'm:{a-zA-Z}={A-Za-z}'  # Case insensitive
zstyle ':completion:*:descriptions' format '%F{yellow}-- %d --%f'
zstyle ':completion:*:warnings' format '%F{red}No matches found%f'
```

**Custom completions included:**

- `_bws` - Bitwarden Secrets Manager
- `_op` - 1Password CLI
- `_snitch` - Snitch tool
- `_tailscale` - Tailscale CLI

### Plugins

The configuration uses these plugins (loaded via `common/tools.sh`):

**zsh-autosuggestions**

- Shows suggestions based on history
- Accept with right arrow or Ctrl+E

**zsh-syntax-highlighting**

- Real-time syntax highlighting
- Invalid commands shown in red
- Valid commands in green

### Starship Prompt

[Starship](https://starship.rs/) provides the default prompt:

- Git status and branch information
- Language version detection (Node, Python, Rust, Go)
- Command execution time
- Exit code indicators
- Cross-shell consistency with bash

### Carapace (Universal Completion)

If [Carapace](https://github.com/rsteube/carapace-bin) is installed:

```zsh
export CARAPACE_BRIDGES='zsh,fish,bash,inshellisense'
source <(carapace _carapace)
```

Provides completions for 350+ commands across multiple shells.

## Zsh-Specific Aliases

Beyond the common aliases, zsh includes:

```zsh
# Editor shortcuts
alias vi='nvim'
alias vim='nvim'
alias edit='$EDITOR'
alias g="git"
alias v="vim"

# Enhanced eza (if installed)
alias la="eza -lg --all --group-directories-first --color always --icons always"
alias ld="eza -lDg --color always --icons always"
alias lf="eza -lfg --color always --icons always"
alias lh="eza -dlg .* --color always --icons always --group-directories-first"
alias lt="eza -alg --color always --icons always --group-directories-first --sort=modified"

# 1Password run (if installed)
alias opr="op run --no-masking --"
alias opl="op run --no-masking"
alias ope="op run --no-masking --env-file"

# macOS-specific
alias hosts='sudo $EDITOR /etc/hosts'
alias trimcopy="tr -d '\n' | pbcopy"
alias cleanup="find . -name '*.DS_Store' -type f -ls -delete"
```

## Customization

### Local Customizations

Machine-specific settings shared with bash go in
`${XDG_CONFIG_HOME:-~/.config}/dotfiles.local/` (`env.sh`, `paths.sh`,
`aliases.sh`, `secrets.sh`) and secrets in `${XDG_CONFIG_HOME:-~/.config}/secrets/` (raw
`1password-service-account`, `gemini_api_key`, `openai_api_key`,
`anthropic_api_key` token files are exported automatically). Zsh-only settings
go in `~/.zshrc.local`:

```zsh
# ~/.zshrc.local
export MY_CUSTOM_VAR="value"
alias myalias="my-command"

# Custom prompt modifications
# Custom completions
```

All of these are untracked and loaded by `common/local.sh`, the last line of
zshrc (`~/.zshrc.local` comes last). See
[Adding Personal Customizations](../docs/features/shell-configuration.md#adding-personal-customizations).

### Adding Completions

For custom completions, add files to `zsh/completions/`:

```zsh
# zsh/completions/_mycommand
#compdef mycommand

_mycommand() {
    local -a commands
    commands=(
        'start:Start the service'
        'stop:Stop the service'
        'status:Show status'
    )
    _describe 'command' commands
}

_mycommand "$@"
```

The directory is automatically added to FPATH.

### Adding Aliases

For cross-platform aliases (recommended):

```zsh
# Edit common/aliases.sh
alias myalias="my-command"
```

For zsh-only aliases:

```zsh
# Edit ~/.zshrc.local
alias zsh-only-alias="command"
```

## Tool Integration

### Atuin (History)

[Atuin](https://atuin.sh/) replaces default history:

```zsh
eval "$(atuin init zsh)"
```

- `Ctrl+R` or Up arrow for fuzzy history search
- Sync history across machines
- Statistics and analytics

### Direnv (Environment)

[Direnv](https://direnv.net/) loads `.envrc` automatically:

```zsh
eval "$(direnv hook zsh)"
```

### FZF (Fuzzy Finder)

FZF integration:

```zsh
source "$(fzf-share)/key-bindings.zsh"
source "$(fzf-share)/completion.zsh"
```

- `Ctrl+T` - Fuzzy file search
- `Alt+C` - Fuzzy directory change
- `**<Tab>` - Fuzzy completion

### Python Virtual Environments

Auto-activation of `.venv` directories:

```zsh
python_venv() {
    if [[ -d ".venv" ]]; then
        source ".venv/bin/activate"
    else
        [[ -n "${VIRTUAL_ENV:-}" ]] && deactivate
    fi
}
add-zsh-hook chpwd python_venv
```

### thefuck

If [thefuck](https://github.com/nvbn/thefuck) is installed:

```zsh
eval "$(thefuck --alias)"
```

Type `fuck` to correct previous command.

## Troubleshooting

### Slow Shell Startup

If zsh starts slowly:

1. **Profile startup time:**

   ```zsh
   time zsh -i -c exit
   ```

2. **Enable debug mode:**

   ```zsh
   export DOTFILES_DEBUG=1
   exec zsh
   ```

3. **Check for slow plugins:**

   ```zsh
   # Start without plugins
   zsh -f
   ```

### Completions Not Working

If tab completion is broken:

```zsh
# Rebuild completion cache
rm -f ~/.zcompdump*
exec zsh

# Check if compinit ran
whence compinit
```

### Vi Mode Issues

If keybindings conflict with plugins:

```zsh
# Check current keymap
bindkey -L | grep vi

# Reset vi mode bindings
bindkey -v
bindkey -M viins 'jj' vi-cmd-mode
```

### PATH Order Issues

If wrong tool versions are used:

```zsh
# Check PATH order
echo $PATH | tr ':' '\n'

# Force fix Homebrew order
fix_homebrew_path_order
```

## Testing

### Test configuration loads

```zsh
zsh -l -c 'echo "Zsh config loaded successfully"'
```

### Test in isolation

```zsh
# Start zsh without loading config
zsh -f

# Manually source and test
source ~/.zshrc
```

### Reload configuration

```zsh
exec zsh -l
```

## Comparison with Bash

| Feature             | Zsh           | Bash     |
| ------------------- | ------------- | -------- |
| Vi mode             | Yes           | Yes      |
| History search      | Yes           | Yes      |
| Starship prompt     | Yes           | Yes      |
| Tab completion      | Advanced      | Basic    |
| Autosuggestions     | Yes (plugin)  | No       |
| Syntax highlighting | Yes (plugin)  | No       |
| Glob patterns       | More powerful | Extended |
| Spelling correction | Available     | Limited  |

Both shells source the same `common/` modules, providing identical:

- Aliases
- Functions
- Environment variables
- PATH configuration
- Tool integrations

## Related Documentation

- [Common System](../common/README.md) - Shared configuration modules
- [Bash Configuration](../bash/README.md) - Bash equivalent documentation
- [Shell Configuration Guide](../docs/features/shell-configuration.md) - Detailed guide
- [CLAUDE.md](../CLAUDE.md) - Full technical documentation
