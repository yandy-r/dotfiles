# Native Package Installer - Quick Reference

## Quick Start

```bash
# See what would be installed
./bin/install-native-packages.sh --dry-run --force

# Install everything (with prompts)
./bin/install-native-packages.sh

# Install everything (no prompts)
./bin/install-native-packages.sh --non-interactive
```

## Common Options

| Option              | Description                  | Example                      |
| ------------------- | ---------------------------- | ---------------------------- |
| `--dry-run`         | Show what would be installed | `--dry-run`                  |
| `--force`           | Skip installed checks        | `--force`                    |
| `--non-interactive` | No prompts                   | `--non-interactive`          |
| `--skip-category`   | Skip category                | `--skip-category multimedia` |
| `--help`            | Show help                    | `--help`                     |

## Package Summary (Arch Linux)

| Category    | Count  | Key Packages                               |
| ----------- | ------ | ------------------------------------------ |
| Core        | 8      | bash, curl, wget, ca-certificates          |
| Development | 12     | gcc, python, go, ruby, lua, cmake          |
| Git         | 5      | git, git-lfs, git-delta, lazygit           |
| Network     | 9      | nmap, tcpdump, mtr, openbsd-netcat, httpie |
| System      | 15     | tmux, fzf, bat, fd, ripgrep, eza, htop     |
| Editors     | 5      | vim, neovim, pandoc, shellcheck, prettier  |
| Containers  | 4      | ansible, docker, docker-compose            |
| Multimedia  | 6      | ffmpeg, exiftool, rtmpdump                 |
| Shell       | 6      | zsh, zsh-\*, direnv, asciinema             |
| Additional  | 3      | speedtest-cli, xh, thefuck                 |
| **TOTAL**   | **73** |                                            |

## Supported Distributions

- ✅ Arch Linux (all 73 packages)
- ✅ Debian 11+ (~55 packages, missing some Arch-only)
- ✅ Ubuntu 22.04+ (~55 packages, missing some Arch-only)

## Debian/Ubuntu Differences

### Automatically Fixed

- `bat` → symlinked from `batcat`
- `fd` → symlinked from `fdfind`

### Not Available (use Homebrew/snap)

- git-delta, lazygit, bottom
- bitwarden-cli, xh, thefuck
- prettier, ansible-language-server
- docker (requires official repo)

### Arch / CachyOS: bitwarden-cli and nodejs-lts

`bitwarden-cli` is not installed by this script on Arch (avoids nodejs vs nodejs-lts conflict). Use: `npm install -g @bitwarden/cli`.

## Special Installers Required

Not included (install separately):

- AWS CLI (official installer)
- Azure CLI (official installer)
- GitHub CLI / gh (external repo on Debian/Ubuntu)
- AUR packages (Arch only - use yay/paru)

## Post-Install Steps

```bash
# Docker
sudo usermod -aG docker $USER

# Zsh as default shell
chsh -s $(which zsh)

# Direnv hook (add to ~/.bashrc or ~/.zshrc)
eval "$(direnv hook bash)"  # or zsh
```

## Examples

```bash
# Development workstation
./bin/install-native-packages.sh --skip-category multimedia

# Server (no GUI tools)
./bin/install-native-packages.sh --skip-category multimedia,containers

# CI/CD pipeline
NON_INTERACTIVE=true ./bin/install-native-packages.sh --skip-category multimedia,shell

# Test before real install
./bin/install-native-packages.sh --dry-run --force
```

## Files

- `bin/install-native-packages.sh` - Main script (785 lines)
- `bin/install-native-packages.md` - Full documentation
- `bin/install-native-packages-summary.md` - Implementation summary
- `bin/install-native-packages-quickref.md` - This file

## See Also

- `install-brew.sh` - Homebrew installer (cross-platform)
- `.docs/arch-package-mapping.md` - Package name mappings
- `AGENTS.md` - Shell scripting guidelines
