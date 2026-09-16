# Dotfiles Symlinks Management

> **RETIRED — historical-rollback-only.** Symlinks are provisioned natively by `dotf link`
> (`internal/symlink`) as of Phase 4, and as of Phase 7 the `dotf` CLI is the provisioning entry
> point: `bootstrap.sh` is a shim that downloads `dotf` and execs `dotf apply`, which drives
> `dotf link` for symlinks. This playbook is kept **unexecuted** as a rollback path only — do not
> run it in normal provisioning.

This directory contains Ansible playbooks for managing dotfiles symlinks with comprehensive safety features and validation.

## Enhanced Symlinks Playbook

### Features

#### 🛡️ Safety Features

- **Backup mechanism**: Automatically backs up existing configurations before creating symlinks
- **Symlink detection**: Only removes files that are not already symlinks
- **Dry run mode**: Test changes before applying them
- **Force mode**: Override existing symlinks when needed
- **Pre-flight validation**: Checks for dotfiles directory existence

#### 📦 Comprehensive Coverage

- **Shell configurations**: Bash and Zsh dotfiles with completions
- **Development tools**: Neovim, WezTerm, Ghostty configurations
- **Platform-specific**: macOS-only configs (AeroSpace, iTerm2)
- **Optional Linux desktop**: Generic `hypr`, `waybar`, `swayosd`, and `fuzzel` configs for CachyOS/Arch-style Hyprland setups
- **Utilities**: Binary directories, SSH configs, common tools
- **Special files**: Ansible config, Git settings

#### 🔧 Advanced Options

- **Tagged execution**: Run specific sections only
- **Variable-driven**: Easily customizable paths and behavior
- **Cross-platform**: Handles macOS, Linux, and Windows differences
- **Validation**: Post-execution checks for broken symlinks
- **Reporting**: Detailed summary of operations

### Usage

#### Basic Usage

```bash
# Run the complete symlinks setup
ansible-playbook playbooks/symlinks.yml

# Run with backup (default)
ansible-playbook playbooks/symlinks.yml -e backup=true

# Run without backup
ansible-playbook playbooks/symlinks.yml -e backup=false
```

#### Advanced Usage

```bash
# Dry run to see what would happen
ansible-playbook playbooks/symlinks.yml -e dryrun=true

# Preview generic Linux desktop rollout
ansible-playbook playbooks/symlinks.yml -e dryrun=true -e linux_desktop=true

# Force recreate all symlinks
ansible-playbook playbooks/symlinks.yml -e force=true

# Run only shell configurations
ansible-playbook playbooks/symlinks.yml --tags shells

# Run only application configs
ansible-playbook playbooks/symlinks.yml --tags applications

# Run only macOS-specific configs
ansible-playbook playbooks/symlinks.yml --tags macos

# Skip validation
ansible-playbook playbooks/symlinks.yml -e validate=false
```

#### Generic Linux Desktop Activation

The repo-owned Linux desktop baseline is opt-in. It manages:

- `~/.config/hypr`
- `~/.config/waybar`
- `~/.config/swayosd`
- `~/.config/fuzzel`

Use this rollout on the generic CachyOS/Arch Hyprland path. The playbook requires a dry run or backups before replacing existing `~/.config/hypr`, `~/.config/waybar`, `~/.config/swayosd`, or `~/.config/fuzzel`.

```bash
# Preview first
ansible-playbook playbooks/symlinks.yml -e dryrun=true -e linux_desktop=true

# Apply live only with backups enabled
ansible-playbook playbooks/symlinks.yml -e backup=true -e linux_desktop=true
```

#### Available Tags

- `shells` - Bash, Zsh, and shell completions
- `applications` - Neovim, WezTerm, Ghostty
- `macos` - macOS-specific configs (AeroSpace)
- `linux` - Linux-specific configs
- `windows` - Windows-specific configs
- `utilities` - Binary directories and utilities
- `completions` - Shell completion files
- `special` - SSH, Ansible, and other special configs
- `backup` - Backup operations only
- `validation` - Validation and reporting only

### Configuration Variables

You can customize the playbook behavior by setting these variables:

```yaml
# Core directories
dotfiles_dir: '{{ ansible_env.HOME }}/.config/dotfiles'
backup_dir: '{{ ansible_env.HOME }}/.config/dotfiles_backup_{{ ansible_date_time.epoch }}'

# Options
force_symlinks: false # Force overwrite existing symlinks
create_backups: true # Create backups before linking
dry_run: false # Show what would happen without doing it
validate_after: true # Validate symlinks after creation
manage_linux_desktop: false # Opt-in deployment of hypr/waybar/swayosd/fuzzel on Linux
```

### Safety Mechanisms

#### 1. Backup System

- Creates timestamped backup directory
- Only backs up real files (not existing symlinks)
- Preserves original permissions and metadata

#### 2. Symlink Detection

- Checks if target already exists
- Distinguishes between files and symlinks
- Only removes/replaces non-symlink files

#### 3. Directory Creation

- Ensures target directories exist before linking
- Creates `.ssh`, `.zsh/completions`, `.config` as needed
- Handles missing parent directories gracefully

#### 4. Validation

- Scans for broken symlinks after creation
- Reports any issues found
- Provides summary of operations

#### 5. Generic Linux Desktop Rollout Guardrails

- Requires `linux_desktop=true` to manage `hypr`, `waybar`, `swayosd`, or `fuzzel`
- Requires either `dryrun=true` or `backup=true` for generic Linux desktop activation
- Leaves stock Hyprland configs untouched unless the generic rollout is explicitly requested with `linux_desktop=true`

### Troubleshooting

#### Common Issues

### Playbook fails with "dotfiles directory not found"

```bash
# Ensure dotfiles are in the expected location
ls -la ~/.config/dotfiles
```

### Permission denied errors

```bash
# Check directory permissions
chmod 755 ~/.config/dotfiles
```

### Broken symlinks reported

```bash
# Check if source files exist
ls -la ~/.config/dotfiles/[category]/[file]
```

#### Recovery

##### Restore from backup

```bash
# Find your backup directory
ls -la ~/.config/dotfiles_backup_*

# Restore specific files
cp ~/.config/dotfiles_backup_[timestamp]/.bashrc ~/.bashrc
```

##### Remove all symlinks

```bash
# Find and remove dotfiles symlinks
find ~ -maxdepth 1 -type l -lname "*dotfiles*" -delete
find ~/.config -maxdepth 2 -type l -lname "*dotfiles*" -delete
```

### Examples

#### Selective Deployment

```bash
# Only set up shell configurations on a new system
ansible-playbook playbooks/symlinks.yml --tags shells

# Add development tools to existing setup
ansible-playbook playbooks/symlinks.yml --tags applications

# Update macOS-specific configs only
ansible-playbook playbooks/symlinks.yml --tags macos
```

#### Safe Testing

```bash
# See what would change without doing it
ansible-playbook playbooks/symlinks.yml -e dryrun=true

# Preview the generic Linux desktop migration path
ansible-playbook playbooks/symlinks.yml -e dryrun=true -e linux_desktop=true

# Test with backups disabled
ansible-playbook playbooks/symlinks.yml -e dryrun=true -e backup=false

# Validate existing setup
ansible-playbook playbooks/symlinks.yml --tags validation
```

### Best Practices

1. **Always test first**: Use dry run mode on new systems
2. **Keep backups**: Don't disable backups unless necessary
3. **Use tags**: Deploy incrementally using specific tags
4. **Validate regularly**: Run validation to check for broken links
5. **Document changes**: Note any custom modifications in your fork

### Playbook Structure

The playbook is split into modular files for maintainability:

```
playbooks/
├── symlinks.yml              # Main orchestrator (pre_tasks, import_tasks, post_tasks, handlers)
├── vars/
│   └── symlinks.yml          # All variable definitions (symlink mappings, options)
└── tasks/
    ├── backup.yml            # Backup existing configs before linking
    ├── unified-cleanup.yml   # Source/dest checks, remove non-symlinks
    ├── shells.yml            # Bash, Zsh, and common shell files
    ├── prompt.yml            # Starship prompt configuration
    ├── applications.yml      # App directories (nvim, wezterm, ghostty, mise, alacritty)
    ├── macos.yml             # macOS-specific configs (aerospace)
    ├── linux.yml             # Linux-specific configs
    ├── linux-desktop.yml     # Hyprland desktop (hypr, waybar, swayosd, mako, wlogout, fuzzel, satty)
    ├── launchers.yml         # Desktop .desktop file launchers
    ├── archlinux.yml         # Arch-specific (systemd user services)
    ├── windows.yml           # Windows-specific configs
    ├── utilities.yml         # Utility directories (bin)
    ├── completions.yml       # Shell completions
    ├── special.yml           # Special symlinks (ssh, ansible, direnv, branchlet, atuin, lazygit)
    └── zed.yml               # Zed settings (copy-based, not symlink)
```

### Contributing

When adding new configurations:

1. Add the source file to the appropriate category in `dotfiles/`
2. Add the symlink mapping to the corresponding variable list in `vars/symlinks.yml`
3. If creating a new category, create a new task file in `tasks/` and add an `import_tasks` entry in `symlinks.yml`
4. Test with dry run mode, and use `linux_desktop=true` for desktop rollout paths
5. Document any special requirements or platform dependencies
