# Arch Linux Package Installer

The Arch Linux package installer provides a comprehensive, automated package installation system that maps 110+ Homebrew packages to their Arch Linux equivalents using pacman and the AUR.

## Overview

This feature introduces a unified package installation experience for Arch Linux users that:

- Automatically installs the yay AUR helper for AUR package access
- Maps Homebrew packages to native Arch/AUR equivalents
- Installs the repo-owned Hyprland desktop runtime used by the generic CachyOS path
- Provides intelligent fallback handling for missing packages
- Supports dry-run mode for testing without installing
- Tracks installation statistics and failed packages
- Integrates seamlessly with the bootstrap process

## Why This Feature Exists

Prior to this feature, Arch Linux users faced two suboptimal approaches:

1. **Homebrew on Linux**: Installing Homebrew on Linux works but adds overhead and duplicates functionality that pacman already provides natively
2. **Individual installers**: Using separate installer scripts (AWS tools, Azure CLI, HashiCorp tools, etc.) resulted in fragmented installation logic

The Arch package installer solves this by providing a single, comprehensive installer that leverages Arch's native package ecosystem.

## Installation

### Automatic Installation (Recommended)

The Arch package installer runs automatically during bootstrap on Arch-based systems:

```bash
# Clone dotfiles
git clone https://github.com/yandy-r/dotfiles.git ~/.config/dotfiles
cd ~/.config/dotfiles

# Run bootstrap - automatically detects Arch and uses native installer
./bootstrap.sh
```

On CachyOS, this bootstrap path now also installs the generic Hyprland desktop runtime used by the repo-owned Linux desktop baseline.

### Manual Installation

Run the installer script directly:

```bash
# Install all packages
./bin/install-arch-pkgs.sh

# Dry run (show what would be installed)
./bin/install-arch-pkgs.sh --dry-run

# Skip AUR packages (official repos only)
./bin/install-arch-pkgs.sh --skip-aur

# Skip official packages (AUR only)
./bin/install-arch-pkgs.sh --skip-official

# Non-interactive mode (auto-approve all prompts)
./bin/install-arch-pkgs.sh --non-interactive

# Force reinstall already-installed packages
./bin/install-arch-pkgs.sh --force
```

### Desktop Runtime Rollout

Installing the packages is only the first half of the Linux desktop path. After the runtime is present, preview the managed config rollout before linking any desktop directories:

```bash
# Preview generic Hyprland/Waybar/Fuzzel deployment
ansible-playbook playbooks/symlinks.yml --extra-vars "dryrun=true linux_desktop=true"

# Apply live with backups after reviewing the preview
ansible-playbook playbooks/symlinks.yml --extra-vars "backup=true linux_desktop=true"
```

This separation keeps package installation safe on clean CachyOS systems while preventing accidental replacement of an existing `~/.config/hypr`.

## Package Categories

The installer organizes packages into logical categories:

### Core Packages

- Shells: zsh, bash, shell enhancements
- Core utilities: coreutils, util-linux

### Development Tools

- Version control: git, git-lfs, github-cli
- Build tools: base-devel, cmake, gcc, clang
- Languages: python, ruby, go, lua, nodejs

### Modern CLI Tools

- Search tools: ripgrep, fd, fzf, ack
- Modern alternatives: eza (ls), bat (cat), zoxide (cd), bottom (top)
- Git tools: git-delta, lazygit
- Text processing: jq, yq, gawk, gnu-sed
- HTTP clients: curl, wget, httpie, xh
- Shell enhancements: direnv, thefuck

### Network Tools

- Diagnostics: nmap, mtr, tcpdump
- Testing: fping, sipcalc, speedtest-cli

### Cloud & DevOps Tools

- AWS: aws-cli, aws-sam-cli (AUR), amazon-ecs-cli (AUR)
- Azure: azure-cli
- Terraform: terraform, tflint
- Ansible: ansible, ansible-language-server
- Kubernetes: kubectl
- Containers: docker, docker-compose, skaffold, hadolint

### Tmux Tools

- tmux, tmux-mem-cpu-load (AUR), tmuxp (AUR)

### Media Tools

- ffmpeg, exiftool

### Hyprland Desktop Runtime

- Session/runtime integration: acpid, xdg-desktop-portal, xdg-desktop-portal-hyprland, xdg-utils, libnotify
- Launcher and bar: fuzzel, waybar
- Lock and idle: hyprlock, hypridle
- Capture helpers: grim, slurp, wl-clipboard, wf-recorder

### Development Libraries

- Image libraries: freetype2, libjpeg-turbo, libpng
- Others: libgit2, zeromq, rtmpdump

### AUR Packages

- Cloud CLIs: aws-sam-cli, amazon-ecs-cli, cfn-lint
- Development: flow, golangci-lint, carapace-bin
- Security: bitwarden-cli
- Git extensions: gitflow-avh
- Utilities: asciinema, dotenvx

## How It Works

### Data Flow

1. **Environment Validation**: Checks for Arch Linux, pacman availability, disk space, and network connectivity
2. **System Update**: Updates pacman database
3. **yay Installation**: Automatically installs yay AUR helper if not present
4. **Package Installation**: Installs packages by category using pacman/yay
5. **Cache Cleanup**: Removes orphaned packages and cleans cache
6. **Summary Report**: Displays installation statistics and failed packages

### Bootstrap Integration

When `bootstrap.sh` runs on Arch Linux:

```bash
# In bootstrap.sh main() function
if [[ "$DOTFILES_OS" == "arch" ]]; then
  install_arch_packages  # Runs comprehensive installer
else
  # Other Linux distros use individual installers
  install_aws_tools
  install_azure_cli_tool
  install_hashicorp_tools_set
  install_container_tools
  install_modern_cli_tools
fi
```

This ensures Arch users get native package installation while other distributions continue using their existing installer scripts.

### Package Mapping Strategy

The installer uses a carefully curated mapping of Homebrew packages to Arch equivalents:

- **Official repositories**: Preferred when available (faster, more stable)
- **AUR packages**: Used for packages not in official repos
- **Alternative names**: Handles naming differences (e.g., `gnu-sed` instead of `gsed`)
- **Fallback handling**: Gracefully skips unavailable packages with warnings

For the Linux desktop baseline, the installer also locks the runtime contract for the repo-owned Hyprland configuration. That baseline assumes:

- `fuzzel` for application launching
- `waybar` for the status bar
- `hyprlock` and `hypridle` for lock/idle flow
- `grim`, `slurp`, `wl-clipboard`, and `wf-recorder` for screenshot and recording workflows
- `acpid`, `libnotify`, `xdg-utils`, and the Hyprland portal packages for monitor hooks and Wayland integration

Those packages are paired with repo-owned configs in:

- `hypr/` for the Hyprland session, bindings, lock/idle flow, and monitor defaults
- `waybar/` for the bar baseline
- `fuzzel/` for the launcher baseline

## Configuration

### Environment Variables

Control installer behavior via environment variables:

```bash
# Enable dry-run mode
DRY_RUN=true ./bin/install-arch-pkgs.sh

# Skip AUR packages
SKIP_AUR=true ./bin/install-arch-pkgs.sh

# Skip official repository packages
SKIP_OFFICIAL=true ./bin/install-arch-pkgs.sh

# Non-interactive mode
NON_INTERACTIVE=true ./bin/install-arch-pkgs.sh

# Force reinstallation
FORCE_INSTALL=true ./bin/install-arch-pkgs.sh
```

### Command-Line Options

All environment variables have corresponding command-line flags:

```bash
./bin/install-arch-pkgs.sh --dry-run --skip-aur --non-interactive
```

## Files Involved

### Primary Script

- **/home/yandy/.config/dotfiles/bin/install-arch-pkgs.sh**: Main installer script (684 lines)
  - Environment validation
  - Package installation functions
  - yay AUR helper installation
  - Installation tracking and reporting

### Bootstrap Integration

- **/home/yandy/.config/dotfiles/bootstrap.sh**: Modified to call Arch installer
  - `install_arch_packages()` function (lines 969-983)
  - Conditional logic in `main()` (lines 1088-1106)

### Related Configuration

- **/home/yandy/.config/dotfiles/common/platform.sh**: Platform detection (identifies Arch)
- **/home/yandy/.config/dotfiles/common/tools.sh**: Tool detection and health checks
- **/home/yandy/.config/dotfiles/playbooks/symlinks.yml**: Opt-in deployment of `hypr`, `waybar`, and `fuzzel` with dry-run/backup safety gates

## Usage Examples

### Basic Installation

```bash
# Full installation with defaults
./bin/install-arch-pkgs.sh
```

### Preview Changes

```bash
# See what would be installed without installing
./bin/install-arch-pkgs.sh --dry-run

# See what desktop configs would be linked after install
ansible-playbook playbooks/symlinks.yml --extra-vars "dryrun=true linux_desktop=true"
```

### Selective Installation

```bash
# Install only official packages (skip AUR)
./bin/install-arch-pkgs.sh --skip-aur

# Install only AUR packages (skip official repos)
./bin/install-arch-pkgs.sh --skip-official
```

### Automated Installation

```bash
# Non-interactive installation for scripts/automation
NON_INTERACTIVE=true ./bin/install-arch-pkgs.sh
```

### Force Reinstall

```bash
# Reinstall packages even if already installed
./bin/install-arch-pkgs.sh --force
```

## Installation Output

The installer provides detailed, color-coded output:

```
================================================================
  Installing Core Packages
================================================================
[ARCH-INSTALL] Installing zsh...
[ARCH-INSTALL] ✓ Successfully installed zsh
[ARCH-INSTALL] Package 'bash' already installed, skipping
...

================================================================
  Installation Summary
================================================================

  Installed: 45 packages
  Skipped:   30 packages (already installed)
  Failed:    2 packages

Failed packages:
  - some-unavailable-package
  - another-missing-package
```

## Validation and Safety

### Pre-Installation Checks

The installer validates:

- Running on Arch Linux (checks `/etc/arch-release` and `/etc/os-release`)
- pacman is available
- At least 5GB of free disk space
- Network connectivity to Arch repositories

### Installation Safety

- Uses `--needed` flag to skip reinstalling existing packages
- Provides dry-run mode for testing
- Tracks failed installations without stopping
- Allows skipping package categories
- Auto-confirms prompts in non-interactive mode

### Cleanup and Maintenance

After installation:

- Removes orphaned packages
- Cleans pacman cache
- Cleans yay cache (if installed)

## Troubleshooting

### Installation Failures

If packages fail to install:

```bash
# Check the installation summary for failed packages
# Review the error messages during installation
# Try installing failed packages manually:
pacman -S package-name  # Official repos
yay -S package-name     # AUR packages
```

### yay Installation Issues

If yay fails to install:

```bash
# Ensure base-devel is installed
sudo pacman -S base-devel git

# Manually install yay
git clone https://aur.archlinux.org/yay.git
cd yay
makepkg -si
```

### Disk Space Issues

If installation fails due to disk space:

```bash
# Clean package cache
sudo pacman -Sc

# Remove orphaned packages
sudo pacman -Rns $(pacman -Qtdq)

# Check available space
df -h
```

### Network Issues

If repository access fails:

```bash
# Update mirrorlist
sudo pacman-mirrors --fasttrack

# Or manually update pacman database
sudo pacman -Sy
```

## Performance Considerations

### Installation Time

Full installation typically takes:

- Official packages: 10-20 minutes (depending on internet speed)
- AUR packages: Additional 5-10 minutes (requires building from source)

### Disk Space Requirements

Approximate disk usage:

- Minimum required: 5GB free space (enforced by validation)
- Typical installation: 2-3GB of packages
- Build dependencies (AUR): Additional 500MB-1GB temporary space

### Optimization Tips

```bash
# Skip AUR to save time
./bin/install-arch-pkgs.sh --skip-aur

# Use parallel downloads in /etc/pacman.conf
ParallelDownloads = 5

# Use a faster mirror
sudo pacman-mirrors --fasttrack
```

## Comparison to Other Methods

### vs. Homebrew on Linux

**Arch Package Installer (Native)**:

- Faster installation (binary packages from official repos)
- Better integration with system package manager
- No duplicate functionality
- Smaller disk footprint
- Automatic security updates via pacman

**Homebrew on Linux**:

- Cross-platform consistency
- Access to Homebrew-exclusive packages
- Isolated from system packages
- Slower builds from source

### vs. Individual Installers

**Arch Package Installer (Comprehensive)**:

- Single installer for all packages
- Consistent installation experience
- Centralized error handling
- Unified tracking and reporting

**Individual Installers** (AWS, Azure, HashiCorp, etc.):

- More granular control
- Potentially newer versions
- Useful for single-tool installation

## Integration with Dotfiles System

### Platform Detection

The installer integrates with the platform detection system:

```bash
# In common/platform.sh
DOTFILES_OS="arch"  # Detected automatically
DOTFILES_PKG_MGR="pacman"  # Set for Arch
```

### Tool Detection

Installed packages are detected by the tools system:

```bash
# Source tools system
source ~/.config/dotfiles/common/tools.sh

# Check tool availability
has_tool ripgrep  # Returns true after installation
has_tool bat      # Returns true after installation
```

### Bootstrap Workflow

```
bootstrap.sh
    ├── init_environment()
    ├── install_apt_packages()    # Skipped on Arch
    ├── install_pacman_packages() # Minimal deps only
    ├── install_homebrew()        # Skipped if --homebrew=false
    │
    ├── Platform detection: "$DOTFILES_OS" == "arch"
    │   ├── install_arch_packages()  # NEW: Comprehensive installer
    │   │   └── bin/install-arch-pkgs.sh
    │   │       ├── Core packages
    │   │       ├── Development tools
    │   │       ├── CLI tools
    │   │       ├── Cloud tools
    │   │       └── AUR packages
    │   │
    │   └── Skips individual installers:
    │       ├── install_aws_tools() ✗
    │       ├── install_azure_cli_tool() ✗
    │       ├── install_hashicorp_tools_set() ✗
    │       ├── install_container_tools() ✗
    │       └── install_modern_cli_tools() ✗
    │
    └── Additional tools (all platforms)
        ├── install_1password_cli()
        ├── install_neovim()
        ├── install_claude()
        └── ...
```

## Future Enhancements

Potential improvements for future versions:

1. **Package Groups**: Allow installing specific package groups only
2. **Custom Package Lists**: Support user-defined package lists
3. **Version Pinning**: Pin specific package versions
4. **Conflict Resolution**: Better handling of conflicting packages
5. **Rollback Support**: Ability to rollback failed installations
6. **Parallel Installation**: Install independent packages in parallel

## Contributing

To add new packages to the installer:

1. Edit `bin/install-arch-pkgs.sh`
2. Add package to appropriate category function
3. Specify whether it's official or AUR: `install_package "package-name" "false"` (official) or `install_package "package-name" "true"` (AUR)
4. Test with `--dry-run` flag
5. Verify package name exists in Arch repos or AUR

## Related Documentation

- Platform Detection: `/home/yandy/.config/dotfiles/common/platform.sh`
- Bootstrap Script: `/home/yandy/.config/dotfiles/bootstrap.sh`
- Tool Detection: `/home/yandy/.config/dotfiles/common/tools.sh`
- Main CLAUDE.md: `/home/yandy/.config/dotfiles/CLAUDE.md`

---

**Note**: This installer is designed specifically for Arch Linux and Arch-based distributions (Manjaro, EndeavourOS, etc.). For other distributions, the bootstrap script uses distribution-specific installation methods.
