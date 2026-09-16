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
├── exports.sh          # Environment variables (XDG, editor, pager)
├── paths.sh            # PATH management with platform awareness
├── aliases.sh          # Smart aliases with tool fallbacks
├── functions.sh        # Cross-platform utility functions
├── tools.sh            # Tool detection, setup, and lazy loading
├── completions.sh      # Universal completion system (50+ tools)
├── lazy-completions.sh # Lazy-loaded completions for performance
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
- Duplicate path removal

### aliases.sh - Smart Aliases

Provides aliases with automatic tool detection and fallbacks.

**Categories:**

- **Navigation**: `..`, `...`, `~`, `-`, directory shortcuts
- **File operations**: Enhanced ls (eza > ls), safe operations
- **Git shortcuts**: Common git operations
- **Development**: Docker, kubectl, terraform
- **Modern tools**: ripgrep, fd, bat, fzf integrations

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
- `f` - Find files by name (uses fd if available)
- `fd_dir` - Find directories by name

**Network:**

- `myip` - Get public IP address
- `localip` - Get local IP address
- `ports` - Show listening ports

**Development:**

- `serve` - Start a simple HTTP server
- `extract` - Extract various archive formats
- `json` - Pretty print JSON

**Git:**

- `gclean` - Clean merged branches
- `gundo` - Undo last commit

### tools.sh - Tool Detection and Setup

Manages tool detection, initialization, and lazy loading.

**Tool Categories:**

- **Modern CLI**: eza, bat, ripgrep, fd, fzf, zoxide, delta, atuin
- **Development**: git, nvim, tmux, jq, yq
- **Cloud/DevOps**: docker, kubectl, helm, terraform, ansible
- **Languages**: node, python3, go, rust, cargo

**Functions:**

- `system_health` - Check system health and tool availability
- `list_missing_tools` - Show recommended tools not installed
- `init_mise` - Initialize mise version manager

### completions.sh - Universal Completions

Provides completion support for 50+ modern CLI tools.

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
