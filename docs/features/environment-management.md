# Environment Management Guide

This guide covers environment management tools in the dotfiles, including direnv for automatic directory-based environment variables and atuin for enhanced shell history management.

## Overview

The dotfiles integrate two powerful environment management tools:

- **direnv** - Automatic loading/unloading of environment variables per directory
- **atuin** - Enhanced shell history with sync, search, and analytics

## direnv Setup and Usage

direnv automatically loads and unloads environment variables when you enter and leave directories.

### Installation

```bash
# macOS
brew install direnv

# Ubuntu/Debian
apt install direnv

# Fedora
dnf install direnv

# Arch Linux
pacman -S direnv
```

### Configuration

The dotfiles automatically initialize direnv in both shells:

**Bash** (`bash/bashrc`):

```bash
if command -v direnv >/dev/null 2>&1; then
    eval "$(direnv hook bash)"
fi
```

**Zsh** (`zsh/zshrc`):

```zsh
if command -v direnv >/dev/null 2>&1; then
    eval "$(direnv hook zsh)"
fi
```

### Creating .envrc Files

#### Quick Setup with `envrc` Function

The dotfiles provide an `envrc` helper function:

```bash
# Create .envrc with project template
envrc                    # Uses current directory name as project
envrc my-project-name    # Specify custom project name

# This creates:
# - .envrc with PROJECT_NAME export
# - .env for non-secret local values (gitignored)
# - .env.example as template
# - op/local.env, a committed template of 1Password op:// references
# - .gitignore entries (.env, .env.*, !.env.example)
```

See [Secrets with 1Password](#secrets-with-1password) for how secrets are loaded.

#### Manual Setup

Create a `.envrc` file in any directory:

```bash
# Navigate to your project
cd ~/Projects/my-project

# Create .envrc file
cat > .envrc << 'EOF'
# Project environment variables
export PROJECT_NAME="my-project"
export DATABASE_URL="postgres://localhost/mydb"

# Source .env file if it exists
dotenv_if_exists .env
EOF

# Allow the .envrc file (required first time)
direnv allow
```

### Security: Allowing .envrc Files

For security, direnv requires explicit approval before executing `.envrc` files:

```bash
# Allow current directory's .envrc
direnv allow          # or alias: da

# Deny current directory's .envrc
direnv deny           # or alias: dd

# Edit .envrc file
direnv edit           # or alias: de
```

### Available Aliases

| Alias   | Command         | Description                         |
| ------- | --------------- | ----------------------------------- |
| `da`    | `direnv allow`  | Allow .envrc in current directory   |
| `dr`    | `direnv reload` | Reload direnv                       |
| `ds`    | `direnv status` | Show direnv status                  |
| `dd`    | `direnv deny`   | Deny .envrc in current directory    |
| `de`    | `direnv edit`   | Edit .envrc in current directory    |
| `envrc` | `init-envrc`    | Initialize new .envrc with template |

### Auto-Allow Whitelist

To avoid typing `direnv allow` for trusted directories, configure a whitelist in `~/.config/direnv/direnv.toml`:

```toml
[whitelist]
# Auto-allow all .envrc files under these directories
prefix = [
    "/Users/yourname/.config/dotfiles",
    "/Users/yourname/Projects",
    "/Users/yourname/code",
]

# Auto-allow specific directories only (not subdirectories)
exact = [
    "/Users/yourname/specific-project",
]
```

**Important Notes:**

- Paths must be **absolute** (use `/Users/username/...`, NOT `~`)
- `prefix` matches the directory and all subdirectories
- `exact` matches only the specific directory
- **Security Warning**: Only whitelist directories you completely trust

### Secrets with 1Password

Secrets live in 1Password, never in a file. A project commits only templates of
`op://` references, one per environment (`op/local.env`, `op/staging.env`,
`op/prod.env`):

```bash
# op/local.env (committed; references only, never values)
DB_PASSWORD=op://EnvSecrets/MYAPP_LOCAL_SECRETS/DB_PASSWORD
API_KEY=op://EnvSecrets/MYAPP_LOCAL_SECRETS/API_KEY
```

Run commands that need secrets through `with-secrets` (`bin/with-secrets`, on
`PATH` via `~/.bin`):

```bash
with-secrets -- npm run dev                # op/local.env
with-secrets -e prod -- npm run migrate    # op/prod.env
with-secrets -f op/a.env -f op/b.env -- make test
```

- The template holds `op://` references: the command runs under
  `op run --env-file=<template> -- <cmd>` (output masking stays on; pass
  `--no-masking` for interactive use).
- No template, or no references in it: the command runs as-is.
- References but `op` is missing or not signed in: it fails fast (exit 1) with a
  hint. It never signs in for you.

The shell helpers stay available for ad-hoc use: `opr -- <cmd>` is
`op run --no-masking --`, and `ope <file> -- <cmd>` is
`op run --no-masking --env-file <file>`. To export resolved secrets into the
shell on `cd` instead (slower, may prompt the 1Password app), uncomment the
`op inject` block that `envrc` writes into `.envrc`.

Local setup for a new machine:

1. Install the CLI (`bin/install-1password-cli.sh`, part of the default setup).
2. Turn on 1Password app integration (Settings → Developer → Integrate with
   1Password CLI), or export `OP_SERVICE_ACCOUNT_TOKEN` for headless hosts.
3. Check with `op whoami`, then run `with-secrets -- <cmd>`.

`scripts/secrets-policy-check.sh` (in `npm run lint`, the pre-push hook, and the
`Secrets policy` CI workflow) fails on the retired encrypted-env tooling,
tracked env files, and `op/*.env` lines that are not `op://` references.

### Common .envrc Patterns

#### Basic Environment Variables

```bash
# .envrc
export APP_ENV="development"
export DEBUG="true"
export LOG_LEVEL="debug"
```

#### Loading from .env File

```bash
# .envrc
dotenv_if_exists .env
dotenv_if_exists .env.local
```

#### Project-Specific PATH

```bash
# .envrc
PATH_add ./bin
PATH_add ./node_modules/.bin
```

#### Python Virtual Environment

```bash
# .envrc
layout python3
# Automatically creates/activates .venv
```

#### Node.js Version (with mise)

```bash
# .envrc
use mise
```

#### Cloud Provider Profiles

```bash
# .envrc
export AWS_PROFILE="my-project"
export AWS_REGION="us-east-1"
```

#### 1Password Integration

```bash
# .envrc
# Load secrets from 1Password
export DATABASE_URL="op://vault/item/field"
```

### direnv.toml Configuration

The dotfiles include a pre-configured `dotenv/direnv.toml`:

```toml
[global]
# Hide verbose environment diff output
hide_env_diff = true

# Increase timeout for slow operations
warn_timeout = "5m"

# Better compatibility mode
strict_env = false

[whitelist]
prefix = [
    "/Users/yandy/.config/dotfiles",
    "/Users/yandy/Projects",
]
```

## Migrating from autoenv

If you previously used autoenv, migrate to direnv:

```bash
# Rename autoenv files to direnv format
mv .env .envrc

# Remove autoenv leave file (direnv handles cleanup automatically)
rm .env.leave

# Allow the new .envrc
direnv allow
```

**Key Differences:**

| Feature           | autoenv             | direnv             |
| ----------------- | ------------------- | ------------------ |
| Automatic cleanup | Manual (.env.leave) | Automatic          |
| Shell support     | bash/zsh            | All major shells   |
| Performance       | Shell script        | Go binary (faster) |
| Security          | Arbitrary code      | Sandboxed          |
| Claude CLI        | Known issues        | Compatible         |

## atuin History Management

atuin replaces traditional shell history with an SQLite database, providing enhanced search, sync, and analytics.

### Installation

```bash
# macOS
brew install atuin

# Fedora
dnf install atuin

# Using installer script
./bin/install-atuin.sh

# Or manual installation
curl https://setup.atuin.sh | bash
```

### Initial Setup

```bash
# Import existing shell history
atuin import auto

# Or import specific shell
atuin import bash
atuin import zsh
```

### Configuration

The dotfiles include atuin configuration at `atuin/config.toml`:

```toml
# Enable sync v2 for better performance
[sync]
records = true

# Enable dotfiles management
[dotfiles]
enabled = true

# Search configuration
[search]
inline_height = 20

# Exit on scroll past boundaries
[keys]
scroll_exits = true

# Command statistics
[stats]
common_subcommands = [
    "git", "docker", "kubectl", "npm", "cargo"
]
common_prefix = ["sudo"]

# UI settings
[ui]
show_preview = true
```

### Basic Usage

#### History Search

```bash
# Press Ctrl+R or Up Arrow for interactive search
# - Type to filter commands
# - Up/Down to navigate
# - Enter to execute
# - Tab to insert without executing
```

#### Command Line Interface

```bash
# Search history
atuin search "git commit"

# Show history statistics
atuin stats

# List recent commands
atuin history list

# Show top commands
atuin stats --count 20
```

### Optional: Sync Setup

atuin supports end-to-end encrypted sync across machines:

```bash
# Register for sync (optional)
atuin register -u <username> -e <email>

# Or login to existing account
atuin login -u <username>

# Sync history
atuin sync

# Check sync status
atuin sync status
```

### Search Modes

atuin supports multiple search modes:

| Mode     | Description                |
| -------- | -------------------------- |
| prefix   | Match beginning of command |
| fulltext | Match anywhere in command  |
| fuzzy    | Fuzzy matching             |
| skim     | Skim-style matching        |

Configure default mode:

```toml
# In config.toml
[search]
filter_mode = "global"
search_mode = "fuzzy"
```

### Key Bindings

| Key      | Action                      |
| -------- | --------------------------- |
| Ctrl+R   | Open history search         |
| Up Arrow | History search (filtered)   |
| Tab      | Insert command (no execute) |
| Enter    | Execute command             |
| Ctrl+N/P | Navigate results            |
| Esc      | Cancel search               |

### Statistics and Analytics

```bash
# View command statistics
atuin stats

# Example output:
# Total commands: 15234
# Unique commands: 4521
# Top commands:
#   1. git status (823)
#   2. ls (654)
#   3. cd (543)
```

### Dotfiles Sync

atuin can sync aliases and environment variables:

```bash
# Enable in config.toml
[dotfiles]
enabled = true

# Set an alias
atuin dotfiles alias set myalias "my command"

# Set environment variable
atuin dotfiles var set MY_VAR "my value"

# List synced items
atuin dotfiles list
```

## Troubleshooting

### direnv Issues

**Problem**: "direnv: error .envrc is blocked"

**Solution**:

```bash
direnv allow
```

**Problem**: Environment not loading

**Solution**:

```bash
# Check if hook is installed
direnv status

# Reload direnv
direnv reload

# Check shell integration
which direnv
```

**Problem**: Slow loading

**Solution**: Check `.envrc` for expensive operations and add to whitelist:

```bash
# Increase timeout in direnv.toml
[global]
warn_timeout = "10s"
```

### atuin Issues

**Problem**: History not importing

**Solution**:

```bash
# Check history file location
echo $HISTFILE

# Force import
atuin import auto --overwrite

# Check atuin database
ls -la ~/.local/share/atuin/
```

**Problem**: Ctrl+R not working

**Solution**:

```bash
# Check if atuin is initialized
which atuin

# Reinitialize
# Bash
eval "$(atuin init bash)"

# Zsh
eval "$(atuin init zsh)"
```

**Problem**: Sync conflicts

**Solution**:

```bash
# Force sync from server
atuin sync --force

# Check sync status
atuin sync status
```

## Best Practices

### direnv

1. **Never commit `.envrc` with secrets** - Use `.env` files that are gitignored
2. **Use dotenv_if_exists** - Gracefully handle missing `.env` files
3. **Keep whitelist minimal** - Only whitelist truly trusted directories
4. **Use `PATH_add`** - Don't overwrite PATH, extend it

### atuin

1. **Import history early** - Do this after installation
2. **Enable sync** - For cross-machine access (optional)
3. **Review statistics** - Identify automation opportunities
4. **Use Tab** - Insert commands without executing to modify them

## See Also

- [Shell Configuration Guide](shell-configuration.md) - Shell setup and customization
- [Modern CLI Tools Guide](modern-tools.md) - Tool integrations
- [Completion System Guide](completions.md) - Tab completion configuration
