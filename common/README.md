# Common Configuration System

The `common/` directory is the heart of the dotfiles cross-platform configuration system. It provides unified configurations that work seamlessly across macOS, Linux distributions (Ubuntu, Fedora, Arch, openSUSE), and WSL.

## Overview

This system provides:

- **Feature parity** between bash and zsh shells
- **Cross-platform compatibility** with intelligent fallbacks
- **Modular architecture** for easy customization
- **Platform-specific optimizations** where needed

## Directory Structure

```
common/
├── platform.sh         # OS, architecture, and package manager detection
├── cached-eval.sh      # _cached_eval: cached tool init output (zoxide, starship, …)
├── exports.sh          # Environment variables (XDG, editor, pager)
├── paths.sh            # PATH management with platform awareness
├── aliases.sh          # Smart aliases with tool fallbacks
├── functions.sh        # Cross-platform utility functions
├── tools.sh            # Tool detection, setup, and lazy loading
├── completions.sh      # Universal completion system (50+ tools)
├── lazy-completions.sh # Lazy-loaded completions for performance
├── local.sh            # Machine-local overrides (last in zshrc; bashrc then attaches ble.sh)
├── macos/              # macOS-specific configurations
│   ├── aliases.sh      # Dock, Finder, System Preferences
│   └── functions.sh    # Spotlight, Keychain, WiFi, Audio
├── linux/              # Linux-specific configurations
│   └── aliases.sh      # systemd, package managers, containers
└── systemd/            # systemd service management
```

## Core Modules

### platform.sh - Platform Detection

Detects the operating system, architecture, and available tools.

**Exported Variables:**

- `DOTFILES_OS` - Operating system (macos, debian, redhat, arch, linux, wsl)
- `DOTFILES_ARCH` - Architecture (x86_64, arm64, arm)
- `DOTFILES_CONTAINER` - Container environment (docker, kubernetes, wsl, none)
- `DOTFILES_PKG_MGR` - Package manager (homebrew, apt, dnf, pacman, zypper)
- `DOTFILES_HOMEBREW_PATH` - Homebrew installation path

**Helper Functions:**

```bash
is_macos          # Returns true if on macOS
is_linux          # Returns true if on Linux
is_wsl            # Returns true if in WSL
is_container      # Returns true if in a container
is_arm64          # Returns true if on ARM64
is_x86_64         # Returns true if on x86_64
is_ssh            # Returns true if connected via SSH
has_command       # Check if a command exists
prepend_path      # Add directory to start of PATH
append_path       # Add directory to end of PATH
```

**Usage:**

```bash
source common/platform.sh

if is_macos; then
    echo "Running on macOS"
elif is_linux; then
    echo "Running on Linux"
fi

echo "Package manager: $DOTFILES_PKG_MGR"
```

### exports.sh - Environment Variables

Sets up environment variables following XDG Base Directory Specification.

It also sets `umask 022` for both shells, so new files are not group-writable
even where the login default is `002` (Ubuntu's `USERGROUPS_ENAB`).

**Key Variables:**

- `XDG_CONFIG_HOME` - User configuration directory (~/.config)
- `XDG_DATA_HOME` - User data directory (~/.local/share)
- `XDG_CACHE_HOME` - User cache directory (~/.cache)
- `XDG_STATE_HOME` - User state directory (~/.local/state)
- `EDITOR`, `VISUAL` - Preferred editor (nvim > vim > nano > vi)
- `PAGER`, `MANPAGER` - Pager with smart defaults (bat > less > more)

**Telemetry Disabled:**

- Homebrew analytics
- npm telemetry
- .NET telemetry
- Various other tools

### paths.sh - PATH Management

Manages PATH with intelligent platform detection.

**Features:**

- Automatic Homebrew path detection (macOS ARM vs Intel, Linux)
- Language runtime paths (Cargo, Go, Node, Python)
- User local binaries (~/.local/bin)
- Existence-guarded CLI directories, including `~/.grok/bin` and `~/.antigravity/antigravity/bin`, in both shells
- Duplicate path removal
- `_dotf_bash_completion_file` resolves the single bash-completion candidate
  list; bashrc sources the result at top level before `tools.sh`, preserving
  global arrays in older bash-completion releases (skipped in POSIX mode or
  when already loaded)

### aliases.sh - Smart Aliases

Provides aliases with automatic tool detection and fallbacks for both shells.
All shell aliases live here or in the platform overlays (`macos/aliases.sh`,
`linux/aliases.sh`); neither `zshrc` nor `bashrc` defines its own set.

**Categories:**

- **Navigation**: `..`, `...`, `~`, `-`, directory shortcuts
- **File operations**: Enhanced ls (eza > ls), safe operations
- **Git shortcuts**: Common git operations
- **Development**: Docker, kubectl, terraform
- **Modern tools**: ripgrep (`rg`, `rgr`), fd, bat, eza
- **Editors**: `v`, `vi`, `vim`, `edit` (all `$EDITOR`)
- **Network**: `mtr` (auto-sudoed when installed)

**Example - Smart ls:**

```bash
# If eza is installed:
alias ls="eza -lg --group-directories-first --color always --icons always"

# Fallback if eza not available:
alias ls="ls --color=auto"  # Linux
alias ls="ls -G"            # macOS
```


### functions.sh - Utility Functions

Cross-platform utility functions for common tasks.

**Directory Operations:**

- `mkd` - Create directory and cd into it
- `mkf` - Create file with directory structure
- `f` - Find files by name (uses `fd` or `fdfind` if available)
- `fd_dir` - Find directories by name

**Network:**

- `myip` - Get public IP address
- `localip` - Get local IP address
- `ports` - Show listening ports

**Development:**

- `serve` - Start a simple HTTP server
- `extract` - Extract various archive formats
- `json` - Pretty print JSON
- `graphify` - Run graphify, via the tokenhop OpenAI endpoint when `TOKENHOP_OPENAI_API_KEY` is set
- `graphify-raw` - Run the graphify binary with no tokenhop environment
- `herdr-run-all` - Send one command to other Herdr panes (bash and zsh)

**Git:**

- `gclean` - Clean merged branches
- `gundo` - Undo last commit

### tools.sh - Tool Detection and Setup

Manages tool detection, initialization, and lazy loading — one owner function
per tool, shared by both shells:

- `setup_fzf` — fzf keybindings/completions; sources `~/.fzf.zsh` /
  `~/.fzf.bash` (both linked by `dotf link`) with the repo files as fallback;
  loads after compinit, bash-completion and carapace so fallback loaders survive
- `setup_python_venv` — `.venv` auto-activation hook in both shells; initial
  activation waits until the first prompt after local overrides, preserving
  their PATH entries on deactivate (disable with `DOTFILES_NO_VENV_AUTO=1`)
- `setup_thefuck` — thefuck alias (cached via `_cached_eval`)

**Tool Categories:**

- **Modern CLI**: eza, bat/batcat, ripgrep, fd/fdfind, fzf, zoxide, delta, atuin
- **Development**: git, nvim, tmux, jq, yq
- **Cloud/DevOps**: docker, kubectl, helm, terraform, ansible
- **Languages**: node, python3, go, rust, cargo

**Functions:**

- `system_health` - Check system health and tool availability
- `list_missing_tools` - Show recommended tools not installed
- `init_mise` - Initialize mise version manager

### local.sh - Machine-Local Overrides

Sourced as the last line of `zsh/zshrc`. `bash/bashrc` sources it and then
runs `ble-attach` when ble.sh was loaded at the top of the file, so whatever
it loads still wins over the shared configuration. Each file is loaded only when present,
in this order:

1. `${XDG_CONFIG_HOME:-~/.config}/dotfiles.local/env.sh`, `paths.sh`,
   `aliases.sh`, `secrets.sh`
2. Raw token files in `${XDG_CONFIG_HOME:-~/.config}/secrets/`, exported as
   `OP_SERVICE_ACCOUNT_TOKEN` (`1password-service-account`), `GEMINI_API_KEY`
   (`gemini_api_key`), `OPENAI_API_KEY` (`openai_api_key`) and
   `ANTHROPIC_API_KEY` (`anthropic_api_key`)
3. `${XDG_CONFIG_HOME:-~/.config}/secrets/*.sh`, sorted
4. The legacy per-shell file: `~/.bashrc.local` or `~/.zshrc.local`

Files not owned by the current user or not readable are skipped with a
`dotfiles: skipping <path> (...)` warning on stderr, as are files whose path
(file, its directory, and a symlink target's directory where `realpath` exists)
is world-writable, group-writable by a group other than the user-private group,
or in a directory not owned by the user. Files are sourced at top level
(not inside a function), so `typeset`/`declare` and zsh `setopt` in them persist.
In zsh, if an override file changes `fpath`, `compinit -i` is re-run afterwards
with a separate dump, `${ZDOTDIR:-$HOME}/.zcompdump-local`.
Regression test: `scripts/test-shell-local-hook.sh`.

### cached-eval.sh - Cached Tool Initialization

`_cached_eval [--dep FILE]... <name> <cmd> [args...]` replaces
`eval "$(cmd args)"` for init commands whose output depends only on the binary
(and the `--dep` files): the output is cached in
`${XDG_CACHE_HOME:-~/.cache}/dotfiles/eval/<name>.<shell>` and regenerated when
the binary's or a dep's inode/size/mtime changes. `DOTFILES_NO_EVAL_CACHE=1`
bypasses it; `dotfiles_clear_eval_cache` empties it. Do not use it for output
that depends on the environment (`mise activate` embeds `$PATH`). Regression
test: `scripts/test-shell-startup-cache.sh`.

### completions.sh - Universal Completions

Provides completion support for 50+ modern CLI tools.
The bash-completion framework is sourced by bashrc at top level, not inside a
setup function; `paths.sh` owns its candidate-file resolver. This module owns
compinit and eager tool registrations (including carapace in zsh and bash 4+).
`tools.sh` loads fzf last to wrap, rather than replace, those registrations.


**Supported Tools:**

- **Cloud**: aws, gcloud, az
- **Kubernetes**: kubectl, helm, k9s, istioctl
- **Infrastructure**: terraform, ansible, pulumi
- **Containers**: docker, podman
- **Development**: git, gh, npm, cargo, go

## Platform-Specific Modules

### macos/aliases.sh

macOS-specific aliases for system management:

- Dock visibility and settings
- Finder operations
- System Preferences access
- Homebrew shortcuts

### macos/functions.sh

macOS-specific functions:

- `spotlight` - Search with Spotlight
- `keychain_get` - Retrieve Keychain passwords
- `wifi_info` - Get WiFi network information
- `screenshot` - Take screenshots
- `audio_volume` - Control system volume
- `battery_info` - Get battery status

### linux/aliases.sh

Linux-specific aliases, shared by every Linux family (Debian, RedHat, Arch,
SUSE, WSL):

- systemd service management
- Package manager shortcuts (apt, dnf, pacman)
- Network management (NetworkManager, systemd-networkd)
- Container tools
- Firewall management (ufw, with a firewalld branch for Fedora/RHEL/SUSE)

A distro may refine — not replace — this file with an overlay directory named
after its `DOTFILES_OS` token: `common/redhat/aliases.sh`,
`common/debian/aliases.sh`, `common/arch/aliases.sh`, `common/suse/aliases.sh`
or `common/wsl/aliases.sh`. The overlay is sourced _after_ `linux/aliases.sh`,
so it sees everything the family file defined and can override individual
entries. No overlay directory exists today; the slot is real but unused.

## How It Works

### Shell Loading Order

Both `bash/bashrc` and `zsh/zshrc` source the common modules in this order:

1. `platform.sh` - Detect environment
2. `exports.sh` - Set environment variables
3. `paths.sh` - Configure PATH
4. `aliases.sh` - Set up aliases
5. `functions.sh` - Load utility functions
6. `tools.sh` - Initialize tools and completions

Shell-specific setup and installer-added blocks follow. `zsh/zshrc` ends by
sourcing `local.sh` (machine-local overrides, secrets, `~/.zshrc.local`).
`bash/bashrc` sources `local.sh` and then attaches ble.sh when it was loaded.
In a bash login shell, `bash/bash_profile` sources `~/.bashrc` first, then the
legacy `~/.path` and `~/.extra`, which therefore run after `local.sh` and
`ble-attach`.

### Platform-Specific Loading

`aliases.sh`, `exports.sh` and `functions.sh` each walk a **search path** rather
than a single directory. `os_module_dirs()` in `platform.sh` is the single
source of truth for that path: it prints the OS family directory first, then the
distro overlay directory when one applies, and each loader sources every file
that exists, in order.

```bash
# In aliases.sh
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
while IFS= read -r os_module_dir; do
    [[ -z "$os_module_dir" ]] && continue
    if [[ -f "$SCRIPT_DIR/$os_module_dir/aliases.sh" ]]; then
        source "$SCRIPT_DIR/$os_module_dir/aliases.sh"
    fi
done <<EOF
$(os_module_dirs)
EOF
```

`os_module_dirs()` maps `DOTFILES_OS` like this:

| `DOTFILES_OS`                                 | Search path             |
| --------------------------------------------- | ----------------------- |
| `macos`                                       | `macos`                 |
| `linux`                                       | `linux`                 |
| `debian` / `redhat` / `arch` / `suse` / `wsl` | `linux`, then the token |
| `windows` / `unknown`                         | (nothing)               |

Because the family file loads first, an overlay refines it instead of replacing
it. `${BASH_SOURCE[0]:-$0}` is deliberate: zsh does not populate `BASH_SOURCE`,
and `$0` is the sourced file's path there.

## Extending the Common System

### Adding Cross-Platform Aliases

Edit `common/aliases.sh`:

```bash
# Add your alias
alias myalias="my-command --with-options"

# Or with platform detection
if has_command my-tool; then
    alias mt="my-tool"
else
    alias mt="fallback-command"
fi
```

### Adding Platform-Specific Aliases

Edit the appropriate platform file:

```bash
# macOS: common/macos/aliases.sh
alias mac-only="macos-specific-command"

# Linux: common/linux/aliases.sh
alias linux-only="linux-specific-command"
```

### Adding New Functions

Edit `common/functions.sh`:

```bash
# Add your function
my_function() {
    local arg="$1"

    # Use platform detection if needed
    if is_macos; then
        # macOS implementation
    else
        # Linux implementation
    fi
}
```

### Adding Tool Detection

Edit `common/tools.sh`:

```bash
# Add to appropriate tool category
declare -A MODERN_TOOLS=(
    # ... existing tools ...
    ["mytool"]="Description of my tool"
)

# Add initialization if needed
init_mytool() {
    if has_command mytool; then
        eval "$(mytool init bash)"
    fi
}
```

### Adding Completions

Edit `common/completions.sh`:

```bash
# Add completion loading
if has_completion mytool; then
    load_completion mytool
fi
```

## Testing Changes

### Test platform detection

```bash
source common/platform.sh
print_platform_info
```

### Test tool availability

```bash
source common/tools.sh
system_health
```

### Test specific module

```bash
# Test in isolation
bash -c 'source common/aliases.sh && type ls'
```

### Full shell test

```bash
# Start new shell
exec $SHELL -l
```

## Debugging

Enable debug mode to see detailed loading information:

```bash
export DOTFILES_DEBUG=1
exec $SHELL -l
```

This will output information about:

- Platform detection results
- PATH modifications
- Tool initialization
- Completion loading

## Related Documentation

- [Architecture Overview](../docs/architecture/overview.md)
- [Bash Configuration](../bash/README.md)
- [Zsh Configuration](../zsh/README.md)
- [Adding Tools Guide](../docs/development/adding-tools.md)
