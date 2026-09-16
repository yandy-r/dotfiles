# Oh-My-Zsh Cleanup Script

## Overview

This script safely removes Oh-My-Zsh and Powerlevel10k from your system and prepares it for Starship prompt. It's designed to be run on multiple systems after syncing your dotfiles.

## What It Does

1. ✅ **Creates timestamped backups** of all removed files
2. ✅ **Removes Oh-My-Zsh** (`~/.oh-my-zsh`)
3. ✅ **Removes Powerlevel10k config** (`~/.p10k.zsh`)
4. ✅ **Cleans dotfiles references** to OMZ/P10K
5. ✅ **Updates documentation** to reference Starship
6. ✅ **Verifies Starship** is ready to use

## Prerequisites

- Dotfiles synced to `~/.config/dotfiles`
- Starship installed (optional but recommended)
- Backup of important configs (script creates backups automatically)

## Usage

### On This System (Already Clean)

```bash
# Already migrated! No need to run again.
```

### On Other Systems

1. **Sync your dotfiles:**

   ```bash
   cd ~/.config/dotfiles
   git pull origin main
   ```

2. **Run the cleanup script:**

   ```bash
   ~/.config/dotfiles/bin/cleanup-omz.sh
   ```

3. **Follow the prompts:**
   - Review what will be removed
   - Confirm to proceed
   - Script creates backups automatically

4. **Restart your shell:**

   ```bash
   exec zsh
   ```

## What Gets Backed Up

All removals are backed up to: `~/.config/dotfiles/.backup-omz-TIMESTAMP/`

- `~/.oh-my-zsh/` directory
- `~/.p10k.zsh` configuration
- Modified dotfiles (before cleaning)
- All original configs

## Safety Features

- ✅ **Interactive confirmation** before any changes
- ✅ **Automatic backups** of everything removed
- ✅ **Idempotent** - safe to run multiple times
- ✅ **Verification** - checks configs after cleanup
- ✅ **Rollback friendly** - all backups preserved

## Installing Starship First

If Starship isn't installed, the script will warn you:

```bash
# Install Starship first
brew install starship

# Then run cleanup
~/.config/dotfiles/bin/cleanup-omz.sh
```

## Output Example

```
[INFO] ═══════════════════════════════════════════════════
[INFO]   Oh-My-Zsh Cleanup Script
[INFO] ═══════════════════════════════════════════════════

[INFO] Checking prerequisites...
[SUCCESS] Starship is installed: starship 1.23.0

[WARNING] This script will:
  1. Backup Oh-My-Zsh and P10K configs
  2. Remove ~/.oh-my-zsh directory
  3. Remove ~/.p10k.zsh configuration
  4. Clean OMZ references from dotfiles
  5. Update documentation to reference Starship

Continue? (y/N) y

[INFO] Creating backup directory: ~/.config/dotfiles/.backup-omz-20251013-143022
[INFO] Backing up: ~/.oh-my-zsh
[SUCCESS] Oh-My-Zsh directory removed
[SUCCESS] Powerlevel10k config removed
[SUCCESS] Cleanup Complete!
```

## Verification

After running, the script checks:

1. **Starship config valid** - `~/.config/starship.toml` parses correctly
2. **No OMZ references** - Active configs are clean
3. **Backups created** - Everything backed up safely

## Troubleshooting

### Starship not found

```bash
# Install Starship
brew install starship

# Verify installation
starship --version
```

### Starship config invalid

```bash
# Check config
starship print-config

# Use the reference config
cp ~/.config/dotfiles/starship.toml ~/.config/starship.toml
```

### Want to rollback

```bash
# Find your backup
ls -la ~/.config/dotfiles/.backup-omz-*/

# Restore Oh-My-Zsh (not recommended)
cp -r ~/.config/dotfiles/.backup-omz-TIMESTAMP/.oh-my-zsh ~/
```

## Files Modified

The script modifies these files in your dotfiles:

- `bootstrap.sh` - Removes OMZ installation code
- `bin/install-brew.sh` - Removes powerlevel9k tap
- `zsh/p10k.zsh` - Removed entirely
- `README.md` - Updated documentation
- `CLAUDE.md` - Updated references
- `playbooks/README.md` - Updated file references

## After Cleanup

1. **Test your shell:**

   ```bash
   exec zsh
   ```

2. **Verify prompt appears:**
   - Should see Starship prompt
   - Git status, directory, etc.

3. **Customize if needed:**

   ```bash
   vim ~/.config/starship.toml
   ```

## Benefits After Migration

- ⚡ **5x faster** shell startup
- 🎨 **Unified prompt** across bash and zsh
- 🚀 **Modern features** - context-aware indicators
- 🔧 **Easy customization** - single TOML file
- 📦 **No framework overhead** - direct plugin loading

## Support

If you encounter issues:

1. Check backups: `~/.config/dotfiles/.backup-omz-*/`
2. Verify Starship: `starship --version`
3. Test configs: `zsh -n ~/.config/dotfiles/zsh/zshrc`
4. Review migration docs: `MIGRATION_SUMMARY.md`

## Related Files

- **Starship config:** `~/.config/starship.toml`
- **Migration summary:** `~/.config/dotfiles/MIGRATION_SUMMARY.md`
- **Migration notes:** `~/.config/dotfiles/zsh/.omz-migration-notes.md`
- **This script:** `~/.config/dotfiles/bin/cleanup-omz.sh`
