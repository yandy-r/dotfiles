# Starship Bootstrap Configuration

## Overview

The Starship prompt configuration (`starship.toml`) is now part of the dotfiles and automatically symlinked via Ansible playbook. This ensures consistent prompt configuration across all systems.

## What's Included

**File:** `starship.toml`  
**Symlink:** `~/.config/starship.toml` → `~/.config/dotfiles/starship.toml`

## Features

- ✅ **Unified prompt** for Bash and ZSH
- ✅ **Catppuccin Mocha** color scheme
- ✅ **Context-aware** indicators (Python, Node, Go, Rust, etc.)
- ✅ **Cloud integration** (AWS, Azure, GCloud, Kubernetes, Terraform)
- ✅ **Git status** with rich indicators
- ✅ **Vi mode** visual indicators
- ✅ **Command duration** display (>3s)
- ✅ **Direnv** integration

## Automatic Setup

The symlink is automatically created when you run:

```bash
# Full dotfiles setup
ansible-playbook playbooks/symlinks.yml

# Or just the Starship config
ansible-playbook playbooks/symlinks.yml --tags "starship"
```

## Manual Setup on New Systems

If you're setting up on a new system:

### 1. Install Starship

```bash
# macOS / Linux
brew install starship

# Or using curl
curl -sS https://starship.rs/install.sh | sh
```

### 2. Sync Dotfiles

```bash
cd ~/.config/dotfiles
git pull origin main
```

### 3. Run Ansible Playbook

```bash
# Create all symlinks including starship.toml
ansible-playbook playbooks/symlinks.yml

# Or force update if file exists
ansible-playbook playbooks/symlinks.yml --extra-vars "force=true"
```

### 4. Verify Setup

```bash
# Check symlink
ls -lh ~/.config/starship.toml

# Should show:
# ~/.config/starship.toml -> ~/.config/dotfiles/starship.toml

# Verify config is valid
starship print-config | head -20
```

### 5. Restart Shell

```bash
exec zsh
# or
exec bash
```

## Customization

To customize the prompt:

1. **Edit the source:**

   ```bash
   vim ~/.config/dotfiles/starship.toml
   ```

2. **Changes apply immediately** (symlinked file)

3. **Commit changes:**

   ```bash
   cd ~/.config/dotfiles
   git add starship.toml
   git commit -m "Update Starship prompt configuration"
   git push
   ```

4. **Sync to other systems:**

   ```bash
   cd ~/.config/dotfiles
   git pull
   # Changes automatically reflected via symlink
   ```

## Configuration Highlights

### Prompt Segments

- **OS Icon** - macOS/Linux identifier
- **Username** - Always visible
- **Directory** - Smart truncation (3 levels)
- **Git Branch & Status** - With indicators
- **Languages** - Node, Python, Go, Rust (context-aware)
- **Cloud** - K8s, Terraform, AWS, Azure, GCloud
- **Docker** - Context when in Docker projects
- **Time** - 24-hour format
- **Character** - ❯ (green/red for success/error)

### Color Scheme

Using **Catppuccin Mocha** palette:

- Blue for user/OS
- Peach for directory
- Green for git
- Teal for languages
- Mauve for cloud/infra tools

### Vi Mode Support

- Green ❯ - Insert mode (success)
- Red ❯ - Insert mode (error)
- Green ❮ - Command mode

## Troubleshooting

### Symlink not created

```bash
# Force creation
ansible-playbook playbooks/symlinks.yml --tags "starship" --extra-vars "force=true"
```

### Starship not loading

```bash
# Check Starship is installed
starship --version

# Check shell config loads Starship
grep -A2 "starship init" ~/.zshrc
grep -A2 "starship init" ~/.bashrc
```

### Config errors

```bash
# Validate config
starship print-config

# Check for syntax errors
starship config get
```

### Prompt not appearing

```bash
# Verify symlink exists and points correctly
ls -lh ~/.config/starship.toml
readlink ~/.config/starship.toml

# Should point to: ~/.config/dotfiles/starship.toml
```

## Ansible Playbook Integration

The `starship.toml` is managed in `playbooks/symlinks.yml`:

```yaml
# Prompt configuration (Starship)
prompt_config:
  - { src: 'starship.toml', dest: '.config/starship.toml' }
```

### Tags Available

- `starship` - Create Starship symlink only
- `prompt` - Same as starship
- `shells` - All shell configs including Starship

### Variables

- `force=true` - Force overwrite existing files
- `dryrun=true` - Test without making changes
- `backup=true` - Backup existing files (default)

## Migration from Oh-My-Zsh

If you're migrating from Oh-My-Zsh/Powerlevel10k:

1. Run cleanup script: `~/.config/dotfiles/bin/cleanup-omz.sh`
2. Script automatically removes OMZ and creates Starship symlink
3. See `MIGRATION_SUMMARY.md` for full details

## Related Documentation

- **Starship Docs:** <https://starship.rs/config/>
- **Catppuccin:** <https://github.com/catppuccin/starship>
- **Migration Guide:** `MIGRATION_SUMMARY.md`
- **Cleanup Script:** `bin/cleanup-omz.sh`
- **Playbook:** `playbooks/symlinks.yml`

## Benefits

✅ **Consistent** - Same config on all systems  
✅ **Version Controlled** - Track changes in git  
✅ **Automated** - Setup via Ansible  
✅ **Fast** - No framework overhead  
✅ **Portable** - Works on any system with Starship
