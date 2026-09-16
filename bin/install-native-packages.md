# Native Package Installation Script

## Overview

`install-native-packages.sh` is a comprehensive script for installing development tools and utilities using native Linux package managers (APT for Debian/Ubuntu, pacman for Arch Linux).

## Features

- **Multi-Distribution Support**: Automatically detects and supports:
  - Debian 11+ (Bullseye and newer)
  - Ubuntu 22.04+ (Jammy and newer)
  - Arch Linux (rolling release)
- **Organized by Category**: Packages are grouped into logical categories for better management
- **Dry-Run Mode**: Test installations without making changes
- **Skip Categories**: Selectively skip package categories
- **Already Installed Detection**: Skips packages that are already installed
- **Error Handling**: Robust error handling with detailed logging
- **Debian-Specific Fixes**: Automatically creates symlinks for packages with different binary names (bat/batcat, fd/fdfind)

## Usage

### Basic Installation

```bash
# Install all packages (will prompt for confirmation)
./bin/install-native-packages.sh

# Non-interactive mode (auto-approve)
./bin/install-native-packages.sh --non-interactive
```

### Dry-Run Mode

```bash
# See what would be installed without making changes
./bin/install-native-packages.sh --dry-run

# Dry-run with force flag (skip installed checks, faster)
./bin/install-native-packages.sh --dry-run --force
```

### Skip Categories

```bash
# Skip specific package categories
./bin/install-native-packages.sh --skip-category development
./bin/install-native-packages.sh --skip-category multimedia,containers
```

### Environment Variables

```bash
# Alternative way to set options
DRY_RUN=true ./bin/install-native-packages.sh
NON_INTERACTIVE=true ./bin/install-native-packages.sh
SKIP_CATEGORY="multimedia,containers" ./bin/install-native-packages.sh
```

## Package Categories

### Core (8 packages)

Essential CLI tools and certificates

- bash, coreutils, curl, wget, ca-certificates, gnupg, build tools, file

### Development (12 packages)

Programming languages and build systems

- gcc, g++, make, cmake, clang, pkg-config
- Python 3, Ruby, Lua, Go, protobuf

### Git (4-5 packages)

Version control tools

- git, git-lfs, git-delta (Arch only), lazygit (Arch only), libgit2

### Network (10 packages)

Network utilities and diagnostics

- net-tools, ping, DNS utils, netcat, nmap, tcpdump, mtr, fping, sipcalc, httpie

### System (14-15 packages)

System monitoring and CLI utilities

- tmux, screen, tree, jq, yq, fzf, bat, fd, ripgrep, eza
- htop, btop/bottom, procps, strace, gawk

### Editors (4-5 packages)

Text editors and formatting tools

- vim, neovim, pandoc, shellcheck, prettier (Arch only)

### Containers (1-4 packages)

Container and automation tools

- ansible (both distros)
- docker, docker-compose, ansible-language-server (Arch only)

### Multimedia (6 packages)

Media processing and libraries

- ffmpeg, freetype, libjpeg-turbo, libpng, exiftool, rtmpdump

### Shell (6-7 packages)

Shell enhancements

- zsh, zsh-autosuggestions, zsh-syntax-highlighting, zsh-completions (Arch only)
- direnv, asciinema (autojump on Debian/Ubuntu only; zoxide used on Arch)

### Additional (2-5 packages)

Miscellaneous CLI tools

- ack, speedtest-cli (Debian/Ubuntu); speedtest-cli, xh, thefuck (Arch). bitwarden-cli is not installed on Arch by this script to avoid nodejs vs nodejs-lts conflict—use `npm install -g @bitwarden/cli` if needed.

## Debian/Ubuntu Specific Handling

### Binary Name Differences

The script automatically creates symlinks for packages with different binary names on Debian/Ubuntu:

1. **bat** → `batcat` on Debian, symlinked to `/usr/local/bin/bat`
2. **fd** → `fdfind` on Debian, symlinked to `/usr/local/bin/fd`

### Missing Packages

Some packages available on Arch are not in standard Debian/Ubuntu repositories:

- git-delta
- lazygit
- bottom (btm)
- xh
- bitwarden-cli
- prettier
- thefuck
- docker (requires official Docker repo)

**Alternative**: Use Homebrew, snap, or add external repositories for these packages.

## Arch Linux Specific Notes

### bitwarden-cli and nodejs-lts

On Arch/CachyOS, `bitwarden-cli` is not installed by this script because it can pull in `nodejs`, conflicting with `nodejs-lts-jod`. Install the CLI with: `npm install -g @bitwarden/cli`.

### AUR Packages

Some packages may require AUR (Arch User Repository). These are NOT included in this script:

- amazon-ecs-cli
- aws-sam-cli
- azure-cli
- carapace-bin
- cfn-lint (use python-cfn-lint from official repos)
- dotenvx
- gemini-cli
- hadolint-bin
- pyenv
- skaffold
- terraform (use from official repos)
- tflint-bin
- tmux-mem-cpu-load
- tmux-xpanes

**Installation**: Use an AUR helper like `yay` or `paru`:

```bash
yay -S package-name
# or
paru -S package-name
```

## Packages Requiring Special Installation

These packages require separate installation methods and are NOT handled by this script:

### Cloud CLIs

- **AWS CLI**: Use official installer from AWS
- **Azure CLI**: Use official installer from Microsoft
- **Google Cloud SDK**: Use official installer from Google

### GitHub CLI (gh)

- **Debian/Ubuntu**: Add GitHub's official APT repository
- **Arch**: Available in official repos as `github-cli`

### Container Tools (Debian/Ubuntu)

- **Docker**: Use Docker's official APT repository
- **docker-compose**: Install via pip or Docker's official repo

## Post-Installation Steps

### Docker

```bash
# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker  # or logout/login
```

### Zsh

```bash
# Set as default shell
chsh -s $(which zsh)
```

### Direnv

Add to your shell rc file:

```bash
# For bash: ~/.bashrc
eval "$(direnv hook bash)"

# For zsh: ~/.zshrc
eval "$(direnv hook zsh)"
```

### FZF

```bash
# Run FZF installation for shell integration
$(brew --prefix)/opt/fzf/install  # if using Homebrew
# or
/usr/share/doc/fzf/examples/install  # on Debian/Ubuntu
```

## Requirements

- Root/sudo access
- Internet connectivity
- At least 2GB free disk space in `/usr`
- Supported Linux distribution (Debian 11+, Ubuntu 22.04+, or Arch Linux)

## Error Handling

The script:

- Validates environment before installation
- Checks for sudo privileges
- Verifies network connectivity
- Continues on individual package failures
- Reports failed packages per category
- Exits on critical errors (unsupported OS, no sudo, etc.)

## Exit Codes

- `0`: Success
- `1`: General error (sudo failed, unsupported OS, network failure, etc.)

## Logging

All output includes timestamps and the `[NATIVE-PKG]` prefix:

```
[NATIVE-PKG] [2025-11-05 10:17:23] Installing core packages (8 total)...
[NATIVE-PKG] [2025-11-05 10:17:23] Category core: 8/8 packages installed successfully
```

## Examples

### Complete Installation

```bash
# Standard installation with prompts
./bin/install-native-packages.sh

# Automated installation (CI/CD)
NON_INTERACTIVE=true ./bin/install-native-packages.sh
```

### Selective Installation

```bash
# Install only shell and development tools
./bin/install-native-packages.sh --skip-category multimedia,containers,network

# Install everything except multimedia
./bin/install-native-packages.sh --skip-category multimedia
```

### Testing

```bash
# Dry-run to see what would be installed
./bin/install-native-packages.sh --dry-run --force

# Dry-run without checking if packages are installed
DRY_RUN=true FORCE_INSTALL=true ./bin/install-native-packages.sh
```

## Related Scripts

- `install-brew.sh` - Install packages via Homebrew (cross-platform)
- `install-docker.sh` - Install Docker and related tools
- `install-neovim.sh` - Install Neovim from source
- `install-*.sh` - Other specialized installers in the `bin/` directory

## Troubleshooting

### Package Not Found

- **Debian/Ubuntu**: Run `sudo apt update` first
- **Arch**: Run `sudo pacman -Sy` first
- Check if package is in AUR (Arch only)

### Permission Denied

- Ensure you have sudo privileges
- Don't run the script as root directly (it uses sudo internally)

### Network Errors

- Check internet connectivity
- Verify package repository URLs are accessible
- Check if behind a proxy (set `http_proxy` and `https_proxy`)

### Disk Space

- Ensure at least 2GB free in `/usr`
- Clean package cache: `apt clean` or `pacman -Sc`

## Contributing

When adding new packages:

1. Add to appropriate category function
2. Use correct package name for each distro (see `.docs/arch-package-mapping.md`)
3. Update this README with the new package
4. Test on both Debian/Ubuntu and Arch if possible
5. Handle distro-specific differences appropriately

## License

Part of the dotfiles repository. See repository LICENSE for details.
