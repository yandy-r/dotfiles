# Bash Configuration

This directory contains the bash shell configuration for the dotfiles repository. The configuration provides a modern bash experience with feature parity to zsh, including vi mode, enhanced history, and intelligent completions.

## Files

| File           | Purpose                                     | Symlinked To      |
| -------------- | ------------------------------------------- | ----------------- |
| `bashrc`       | Main interactive shell configuration        | `~/.bashrc`       |
| `bash_profile` | Login shell configuration                   | `~/.bash_profile` |
| `profile`      | Profile loader                              | `~/.profile`      |
| `prompt.sh`    | Custom git-aware prompt (Starship fallback) | Sourced by bashrc |
| `.fzf.bash`    | FZF keybindings for bash                    | `~/.fzf.bash` (via `dotf link`) |

## How It Works

### Loading Order

1. **Login shell** (`~/.bash_profile` or `~/.profile`):
   - Sources `~/.bashrc`
   - Sources Cargo environment only in non-interactive shells
   - Initializes direnv when missing

2. **Interactive shell** (`~/.bashrc`):
   - Sources the common configuration system
   - Applies bash-specific settings
   - Initializes Starship prompt
   - Sources bash-completion at top level before tools, then fzf after tool completions
   - Runs `finalize_path` (from `common/paths.sh`) before local overrides
   - Sources `common/local.sh` last

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

Before `tools.sh`, bashrc sources bash-completion at top level (unless POSIX mode
or already loaded), so older frameworks' global arrays survive. The candidate
file list is resolved by `common/paths.sh:_dotf_bash_completion_file`.

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

Machine-specific settings shared with zsh go in
`${XDG_CONFIG_HOME:-~/.config}/dotfiles.local/` (`env.sh`, `paths.sh`,
`aliases.sh`, `secrets.sh`) and secrets in `${XDG_CONFIG_HOME:-~/.config}/secrets/`.
Bash-only settings go in `~/.bashrc.local`:

```bash
# ~/.bashrc.local
export MY_CUSTOM_VAR="value"
alias myalias="my-command"
```

All of these are untracked and loaded by `common/local.sh`, the last line of
bashrc (`~/.bashrc.local` comes last). In a login shell, `~/.path` and
`~/.extra` are sourced by `bash_profile` after bashrc, so they run after the
hook. See
[Adding Personal Customizations](../docs/features/shell-configuration.md#adding-personal-customizations).

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

[FZF](https://github.com/junegunn/fzf) integration is owned by
`common/tools.sh:setup_fzf` for both shells: it sources `~/.fzf.bash` (linked
to `bash/.fzf.bash` by `dotf link`) when present, else the repo file. The
repo file prefers `fzf --bash` and falls back to the packaged
key-bindings/completion scripts. It loads after bash-completion and carapace,
preserving their completion loaders as fzf's fallback (carapace requires bash 4+).

- `Ctrl+R` - Fuzzy history search (if Atuin not installed)
- `Ctrl+T` - Fuzzy file search
- `Alt+C` - Fuzzy directory change

### Python Virtual Environments

`common/tools.sh:setup_python_venv` auto-activates the nearest ancestor `.venv`
in both shells (bash via `PROMPT_COMMAND`, zsh via a `chpwd` hook). Initial
activation waits for the first prompt, after local overrides finish modifying
PATH. Manually activated environments are left alone. Set
`DOTFILES_NO_VENV_AUTO=1` to disable it.


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

### Regression suites

```bash
bash scripts/test-shell-startup-smoke.sh   # rc syntax, quiet hermetic startup, login DOTFILES_OS, time budget
bats tests/shell                           # platform detection, PATH cleanup, rc reload guards
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
