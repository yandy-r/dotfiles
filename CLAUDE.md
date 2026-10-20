# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is a comprehensive, cross-platform dotfiles repository for development environment setup. It manages shell configurations, development tools, and system settings across macOS, Linux distributions (Ubuntu, Fedora, Arch, openSUSE, WSL), and other Unix-like systems with intelligent detection and unified configuration management.

## Setup and Deployment

### Initial Setup

```bash
# Clone to standard location
git clone https://github.com/yandy-r/dotfiles.git ~/.config/dotfiles
cd ~/.config/dotfiles

# Run automated setup
./bootstrap.sh
```

### Symlink Management

- Symlinks are provisioned by the **native Go engine**: `dotf link` (`internal/symlink`) as of Phase 4
- `dotf link` supports `--dry-run` (preview), `--backup` (back up real files before replacing), and `--linux-desktop` (opt-in Hyprland desktop set), plus post-apply validation
- The Ansible playbook (`ansible-playbook playbooks/symlinks.yml`) is **historical-rollback-only** — do not run it in normal provisioning (see `playbooks/README.md`)
- Symlinks cover bash, zsh, common configs, and application directories, linked to their expected locations in `$HOME`
- As of Phase 7, `bootstrap.sh` is a POSIX download-and-exec shim (~608 lines) that downloads the prebuilt, checksum-verified `dotf` binary from GitHub Releases and runs `dotf apply`; the bash provisioning path is retired as the entry point. The `bin/install-*.sh` installers are retained and now driven by the manifest: the `dev`/`base` intents provision the daily-driver toolset, and the opt-in `runtimes`/`ai`/`cloud`/`git-tools`/`system` intents wrap the legacy installers and setup steps via the `custom-script` backend for full provisioning parity (`--intent <name>`). The opt-in `desktop` intent holds `requires_desktop` GUI packages, including Claude Desktop, espanso, 1Password, ChatGPT, WezTerm, Ghostty, Grok Bot, Cursor, Zed Preview, LM Studio, Obsidian and the reviewed desktop apps documented in `docs/reference/desktop-apps.md`. These packages are not-applicable without a detected graphical session. `claude-desktop` resolves to the official apt repo (Debian/Ubuntu), the Homebrew cask (macOS), or `bin/install-claude-desktop-rpm.sh` on Fedora and Fedora remixes (`x86_64` and `arm64`), which repacks the signature-verified official `.deb` into a local RPM (unsupported by Anthropic). `dotf apply` also ensures the standard XDG/dev directories exist (`create_directories` parity)
- Interactive steps abort cleanly on **Ctrl+C** — the `plan`/`apply` guided wizard also on **Esc** — printing `dotf: aborted` and exiting **130** (Unix SIGINT convention). Answering "No" at the `sysconfig` confirm is a decline (`sysconfig not confirmed`, exit 1), not an abort. Interrupt handling is scoped to the prompts; the post-confirmation execution phases keep default SIGINT behavior

### Manual Post-Setup Steps

- Configure Starship: `Starship prompt (already configured)`
- Update Git configuration with personal details in `~/.gitconfig`
- Import terminal themes from `iterm2/` directory
- Configure 1Password CLI service account token if needed

## Architecture

### Cross-Platform Configuration System ⭐

The repository now uses a unified configuration system that provides feature parity between bash and zsh while maintaining platform-specific optimizations.

#### Core Components

**`common/platform.sh`** - Platform Detection Engine

- OS detection: macOS, Ubuntu, Fedora, Arch, openSUSE, WSL
- Architecture detection: x86_64, ARM64, Apple Silicon
- Package manager detection: Homebrew, apt, dnf, pacman, zypper
- Dynamic path resolution for different platforms

**`common/exports.sh`** - Environment Variables

- XDG Base Directory Specification compliance
- Cross-platform environment setup with platform-specific sections
- Privacy settings (telemetry disabled for various tools)
- Development environment variables (Go, Rust, Node.js, Python)
- Cloud provider configurations (AWS, GCP, Azure)

**`common/paths.sh`** - PATH Management

- Intelligent PATH construction with platform detection
- Package manager path handling (Homebrew variations, system paths)
- Language runtime paths (Cargo, Go, Node, Python)
- Local binary directories with proper ordering
- Duplicate removal and path validation

**`common/aliases.sh`** - Smart Aliases

- Modern tool integration with fallbacks (eza→ls, bat→cat, rg→grep)
- Cross-platform navigation and file operations
- Git workflow aliases with smart defaults
- Development productivity shortcuts
- Platform-aware command alternatives

**`common/functions.sh`** - Utility Functions

- Cross-platform utility functions
- Directory and file operations
- Network utilities and system information
- Development workflow helpers
- Archive and compression functions
- Text processing and encoding utilities

**`common/tools.sh`** - Tool Detection and Setup

- Comprehensive tool detection with version checking
- Automatic installation helpers for missing tools
- Modern CLI tool setup (FZF, atuin, modern alternatives)
- Version manager setup (NVM, Rust, Python venvs)
- Shell enhancements (autosuggestions, syntax highlighting)
- Universal completion system integration

**`common/completions.sh`** - Universal Completion System

- Cross-shell completion support (bash and zsh)
- 50+ modern CLI tools with completions
- Cloud provider CLIs (AWS, GCP, Azure)
- Kubernetes ecosystem (kubectl, helm, k9s, istio)
- Infrastructure as Code tools (Terraform, Ansible, Pulumi)
- Development tools (Git, Docker, language-specific tools)

#### Platform-Specific Configurations

**`common/macos/`** - macOS-Specific Features

- `aliases.sh`: Dock management, Finder operations, System Preferences
- `functions.sh`: Spotlight search, Keychain access, WiFi management, Screenshot utilities, Audio controls, Battery information, Xcode integration

**`common/linux/`** - Linux-Specific Features

- `aliases.sh`: systemd service management, package manager aliases, network management, container tools, firewall management, performance monitoring

### Shell Environment

#### Enhanced Shell Configurations

- **Bash (`bash/bashrc`)**: Complete rewrite using common system
  - Vi mode with modern keybindings
  - Enhanced prompt with git integration (`bash/prompt.sh`)
  - Feature parity with zsh (history, completion, keybindings)
  - Starship prompt support with fallback to custom prompt

- **Zsh (`zsh/zshrc`)**: Refactored to use common system
  - Starship with Starship theme
  - Enhanced zsh options for better UX
  - Advanced completion styling
  - Plugin integration (autoenv, fzf, git, kubectl, docker, terraform)

#### Key Features

- **Feature Parity**: Both shells have vi mode, enhanced history, modern keybindings
- **Cross-Platform PATH**: Automatic detection of Homebrew paths (Linux vs macOS)
- **Modular Loading**: Common configurations loaded by both shells
- **Tool Integration**: FZF, completions for 50+ tools, modern alternatives
- **Smart Fallbacks**: Graceful degradation when tools aren't available

### Package Management

- **Homebrew**: Primary package manager with cross-platform support
  - macOS: `/opt/homebrew` (ARM64) or `/usr/local` (x86_64)
  - Linux: `/home/linuxbrew/.linuxbrew`
- **Nix**: Additional package management with nix-darwin on macOS
- **System Package Managers**: Native support for apt, dnf, pacman, zypper
- **Language Package Managers**: Cargo (Rust), NVM (Node.js), pip (Python)
- **Claude Code**: Cross-platform installation via `bin/install-claude.sh`

### Terminal and Development Tools

- **Neovim**: LazyVim-based configuration in `nvim/`
- **Tmux**: Configuration with TPM (Tmux Plugin Manager)
- **Git**: Global configuration with delta diff viewer
- **SSH**: Centralized SSH client configuration
- **Terminal Emulators**:
  - **iTerm2**: Profiles and color schemes
  - **WezTerm**: Modular configuration with functions, keybindings, statusbar
  - **Ghostty**: Modern terminal configuration

### Window Management

- **AeroSpace**: Tiling window manager config for macOS in `aerospace/aerospace.toml`

## Common Commands

### Essential Commands

```bash
# Initial setup/bootstrap entire environment
./bootstrap.sh

# Live testing with rollback points (section-by-section + snapper snapshots)
./bootstrap.sh --interactive --snapper

# Manage symlinks (native engine — primary deployment method)
dotf link

# Test symlink changes (dry run)
dotf link --dry-run

# Create symlinks with backups
dotf link --backup

# Rollback path only: the retired Ansible playbook is retained one release
# ansible-playbook playbooks/symlinks.yml

# Sync the curated KDE Plasma 6 config set (Linux/KDE only)
dotf plasma status
dotf plasma diff
dotf plasma export                       # live -> plasma/files/
dotf plasma apply --backup               # plasma/files/ -> live, merged per key
dotf plasma apply --recommended --backup # also layer plasma/recommended/

# The same merge as one opt-in phase of a full provisioning pass
dotf apply --plasma

# Debug platform detection
source common/platform.sh && echo "OS: $DOTFILES_OS, Arch: $DOTFILES_ARCH, PKG: $DOTFILES_PKG_MGR"

# Test tool detection and system health
source common/tools.sh && system_health
```

### Shell Testing and Validation

```bash
# Test bash configuration
bash -l -c 'echo "Bash config loaded successfully"'

# Test zsh configuration
zsh -l -c 'echo "Zsh config loaded successfully"'

# Reload current shell configuration
exec $SHELL -l
```

## Common Operations

### Scoping style/lint/format operations

`bin/style.sh` (and its wrappers `bin/lint.sh` / `bin/format.sh`) accept three mutually exclusive git-scope flags:

- `--modified` — all modified files: staged + unstaged + untracked (broadest scope)
- `--staged` — only files in the git index (what `git commit` will record)
- `--unstaged` — only unstaged working-tree changes + untracked files (what's dirty but not yet staged)

Use `--staged` for pre-commit gating of exactly what will be committed (this is what the lefthook pre-commit hook runs). Use `--unstaged` for cleanup of in-progress work before staging. Use `--modified` when you want everything. Omitting the flag runs against the full repo tree.

Markdown linting (`markdownlint-cli2`) is wired through the same dispatcher via the `--docs`
language flag (`npm run lint:md*` / `npm run format:md*`, or `bin/lint.sh --docs` /
`bin/format.sh --docs`). It is **opt-in**: `--docs` is excluded from the no-flag default and from
`--all` (so `npm run lint` and the pre-push hook stay green against the existing markdown
backlog), and the pre-commit hook lints only **staged** Markdown. `format --docs` runs
`markdownlint-cli2 --fix` then `prettier --write`.

### Shell History Management with atuin

Atuin replaces the default shell history with an SQLite database and provides:

- **Enhanced Search**: Fuzzy finding with context-aware filtering
- **History Sync**: End-to-end encrypted sync across machines
- **Statistics**: Command usage patterns and analytics
- **Dotfiles Management**: Sync aliases and environment variables

#### Using atuin

```bash
# Install atuin (automatically included in bootstrap)
./bin/install-atuin.sh

# Import existing history
atuin import auto

# Register for sync (optional)
atuin register -u <username> -e <email>

# Sync history
atuin sync

# Search history with Ctrl-R or Up Arrow
# Tab to select without executing
```

**Configuration**: `~/.config/atuin/config.toml` (automatically symlinked)

### Environment Management with direnv

The repository has migrated from autoenv to direnv for automatic environment variable management. This change improves:

- **Claude CLI Compatibility**: Eliminates shell hook interference that caused command timeouts
- **Performance**: Go-based binary vs shell script execution
- **Security**: Sandboxed environment changes vs arbitrary code execution
- **Shell Compatibility**: Universal shell support vs bash/zsh only

#### Using direnv

```bash
# Quick setup - create .envrc, .env, .env.example, and .gitignore
envrc                    # Uses current directory name as project name
envrc my-project-name    # Or specify project name

# Manual setup
echo 'export PROJECT_NAME="my-project"' > .envrc

# Allow direnv to load the file (first time only)
direnv allow             # Or use alias: da

# Environment automatically loads when entering directory
cd project/ # Loads .envrc
cd ..       # Automatically unloads environment
```

**Useful direnv aliases:**

- `da` - Allow .envrc in current directory
- `dr` - Reload direnv
- `ds` - Show direnv status
- `dd` - Deny .envrc
- `de` - Edit .envrc
- `envrc [name]` - Initialize new .envrc with template

#### Configuring Auto-Allow Whitelist

To avoid typing `direnv allow` for trusted directories, edit `dotenv/direnv.toml`:

```toml
[whitelist]
prefix = [
    "/Users/username/.config/dotfiles",  # Auto-allow dotfiles
    "/Users/username/projects",          # Auto-allow all projects
    "/Users/username/code",              # Auto-allow code directory
]
```

**Important Notes:**

- Paths must be **absolute** (use full path like `/Users/username/...`, NOT `~`)
- Use `prefix` for directory trees (matches subdirectories)
- Use `exact` for single directories only
- **Security Warning**: Only whitelist directories you completely trust - `.envrc` files execute code!

#### Migration from autoenv

Existing `.env` files can be renamed to `.envrc`:

```bash
# Convert autoenv files to direnv
mv .env .envrc
mv .env.leave .envrc  # Manual cleanup no longer needed
direnv allow
```

### Working with the Common System

#### Adding Cross-Platform Aliases

```bash
# Edit common/aliases.sh
echo 'alias myalias="command that works everywhere"' >> common/aliases.sh

# Add platform-specific aliases
echo 'alias macalias="macos-specific command"' >> common/macos/aliases.sh
echo 'alias linuxalias="linux-specific command"' >> common/linux/aliases.sh
```

#### Adding New Tools

1. **Add tool detection** to `common/tools.sh`:

   ```bash
   # Add to appropriate category in tools.sh
   # Add installation logic for different package managers
   # Add setup function for the tool
   ```

2. **Add completions** to `common/completions.sh`:

   ```bash
   # Add completion setup in appropriate section
   setup_my_tool_completions() {
       load_completion mytool
   }
   ```

3. **Update bootstrap script** if installation is needed:

   ```bash
   # Add installation logic to bootstrap.sh
   # Update package lists if necessary
   ```

#### Platform-Specific Customization

```bash
# Test platform in scripts
if is_macos; then
    # macOS-specific logic
elif is_linux; then
    # Linux-specific logic
fi

# Use platform variables
echo "Running on: $DOTFILES_OS ($DOTFILES_ARCH)"
echo "Package manager: $DOTFILES_PKG_MGR"
echo "Homebrew path: $DOTFILES_HOMEBREW_PATH"
```

### Updating Configurations

#### Shell Configuration Updates

1. **Common changes**: Edit files in `common/` directory
2. **Shell-specific**: Edit `bash/bashrc` or `zsh/zshrc` directly
3. **Platform-specific**: Edit files in `common/macos/` or `common/linux/`
4. Changes are automatically available via symlinks
5. Restart shell or source configuration files to apply changes

#### Adding New Applications

1. Add configuration files to appropriate directories
2. Add a `[[symlink]]` entry to `manifest/packages.toml` and run `dotf link`:

   ```toml
   [[symlink]]
   src = "myapp"
   dest = ".config/myapp"
   type = "directory"
   ```

3. To make a tool **installable**, add a `[[package]]` entry and choose a backend.
   Prefer a **native** backend (`pkg`, `aur`, `apt-repo`, `toolchain`,
   `github-release`, `script-url`, `git-clone`) and fall back to the
   `custom-script` escape hatch only as a last resort — see
   [`docs/reference/manifest-backends.md`](docs/reference/manifest-backends.md)
   for the decision ladder and the `custom-script` governance fence (every entry
   needs a `class` + `justification`, and the count is frozen by a committed
   baseline in `internal/manifest/ratchet_test.go`).
4. Add shell integration if needed (aliases, functions, PATH, completions)

### Testing and Validation

#### Platform Detection Testing

```bash
# Test detection system
source common/platform.sh
echo "OS: $DOTFILES_OS"
echo "Architecture: $DOTFILES_ARCH"
echo "Package Manager: $DOTFILES_PKG_MGR"
echo "Homebrew Path: $DOTFILES_HOMEBREW_PATH"
```

#### Tool Detection Testing

```bash
# Test tool detection and health
source common/tools.sh
system_health
list_missing_tools
```

#### Symlink Testing

```bash
# Test symlink creation (dry run)
dotf link --dry-run

# Test with backups enabled
dotf link --backup

# The retired Ansible playbook is historical-rollback-only (see playbooks/README.md)
```

## Key Files and Their Purpose

### Core System Files

- `bootstrap.sh`: POSIX download-and-exec shim (~608 lines) that downloads the checksum-verified `dotf` binary from GitHub Releases and execs `dotf apply`
- `common/platform.sh`: Platform detection and environment variables
- `common/tools.sh`: Tool detection, installation, and setup
- `common/completions.sh`: Universal completion system
- `internal/symlink` + `cmd/dotf/link.go`: native Go symlink engine (`dotf link`) — the symlink provisioner as of Phase 4
- `internal/sysconfig` + `cmd/dotf/sysconfig.go`: per-OS system config (`dotf sysconfig`) — hostname/timezone/locale/shell/firewall/ssh-server
- `internal/plasma` + `cmd/dotf/plasma.go`: native KConfig codec and KDE Plasma 6 sync engine (`dotf plasma`) — `internal/plasma/table.go` is the governed managed-file/strip table
- `internal/debtap` + `cmd/dotf/deb.go`: verified `.deb` → pacman conversion (`dotf deb`), wrapping the AUR `debtap` tool
- `plasma/files/`: `dotf plasma export` snapshot of the live desktop, minus stripped state — written by `export`, never by hand
- `plasma/recommended/`: hand-authored opt-in overlay applied only with `--recommended` — written by hand, never by `export`
- `playbooks/symlinks.yml`: retired-but-retained Ansible symlink playbook (rollback path, kept one release)

### Architecture Overview

The repository uses a **unified cross-platform configuration system** where:

- `common/` directory contains shared configurations used by both bash and zsh
- Platform-specific code is in `common/macos/` and `common/linux/` subdirectories
- Individual application configs are organized by tool/app name
- The `bootstrap.sh` shim downloads the `dotf` binary and runs `dotf apply` to handle initial setup and tool installation
- Symlinks are provisioned natively by `dotf link` (`internal/symlink`); the Ansible playbook (`playbooks/symlinks.yml`) is retired but retained one release as a rollback path
- Both shells (bash/zsh) source the same common system for feature parity

### Shell Configuration Files

- `bash/bashrc`: Modern bash configuration using common system
- `bash/prompt.sh`: Enhanced bash prompt with git integration
- `zsh/zshrc`: Enhanced zsh configuration using common system
- `zsh/starship.toml`: Starship theme configuration

### Application Configuration Files

- `nvim/init.lua`: Neovim entry point (LazyVim-based)
- `wezterm/wezterm.lua`: WezTerm main configuration
- `ghostty/config`: Ghostty terminal configuration
- `aerospace/aerospace.toml`: AeroSpace tiling window manager

### Development Tools Integration

#### Modern CLI Tools

The system provides intelligent integration with modern alternatives:

- **eza** (better ls): Enhanced file listing with colors, icons, git integration
- **bat** (better cat): Syntax highlighting, git integration, paging
- **ripgrep** (better grep): Fast search with smart configuration
- **fd** (better find): Fast file finding with ignore patterns
- **fzf**: Fuzzy finder integration for files, history, commands
- **delta**: Enhanced git diff viewer with syntax highlighting

#### Cloud and DevOps Integration

- **Kubernetes**: kubectl, helm, k9s, istioctl completions and aliases
- **Cloud Providers**: AWS CLI, gcloud, az with completions
- **Infrastructure as Code**: Terraform, Ansible, Pulumi integration
- **Container Tools**: Docker, Podman with comprehensive completions
- **Monitoring**: Prometheus, Grafana, observability tools

#### Environment Management

- **direnv**: Automatic directory-based environment variable management
  - Replaces autoenv for better Claude CLI compatibility
  - Secure sandboxed environment changes
  - Universal shell support (bash, zsh, fish, tcsh)
  - Automatic cleanup when leaving directories

#### Security Tools

- **1Password CLI**: Secure secret management with service account support
- **GPG**: Proper setup for commit signing and encryption
- **SSH**: Enhanced SSH configuration with security best practices
- **Bitwarden CLI**: Alternative secret management solution

## Troubleshooting

### Common Issues

#### Claude CLI Compatibility

- **Issue**: Bash commands hang or timeout with autoenv
- **Solution**: Migrated to direnv which doesn't interfere with command execution
- **Prevention**: New shell sessions will use clean environment management

#### Environment Management

- **Issue**: `.env` files not loading automatically
- **Solution**: Convert to `.envrc` format and run `direnv allow`
- **Note**: direnv requires explicit authorization for security

#### Platform Detection Issues

- Check `/etc/os-release` exists on Linux systems
- Verify `uname -s` and `uname -m` output
- Test platform variables are set correctly

#### Tool Installation Issues

- Check package manager availability
- Verify network connectivity for downloads
- Test individual tool installations manually
- Check PATH configuration for installed tools

#### Symlink Issues

- Use dry-run mode to test changes
- Check file permissions in source and destination
- Verify Ansible is installed and available
- Test backup and restore functionality

### Debugging Commands

```bash
# Debug platform detection
bash -x common/platform.sh

# Debug tool setup
DOTFILES_DEBUG=1 source common/tools.sh

# Debug symlink creation
dotf link --dry-run

# Test individual components
source common/platform.sh && source common/exports.sh && env | grep DOTFILES
```

## GitHub Workflow

### Issue Templates

All issues MUST use the YAML form templates in `.github/ISSUE_TEMPLATE/`:

- **Bug Report** (`bug_report.yml`): Use `gh issue create --template bug_report.yml`
- **Feature Request** (`feature_request.yml`): Use `gh issue create --template feature_request.yml`
- **Compatibility Report** (`compatibility_report.yml`): Use `gh issue create --template compatibility_report.yml`

Blank issues are disabled via `config.yml`. Never bypass templates with `--title`-only issue creation.

Practical CLI limitation:

- `gh issue create` does not support combining `--template` with `--body` or `--body-file`.
- In this repo, `gh issue create --template ...` currently reports `no templates found` for the YAML issue forms, so the CLI is not discovering these form templates reliably.
- If this limitation blocks issue creation, use the GitHub API/tooling to create a fully structured issue body that mirrors the intended form fields, then apply the correct labels. Do not fall back to a vague or title-only issue.

### Pull Requests

PRs auto-populate from `.github/pull_request_template.md`. The template includes:

- `Closes #` issue linkage (always link the related issue)
- Type of Change checkboxes
- Build verification checklist (native build scripts)
- Conditional checks for launch/, profile/, steam/, and UI component changes
- All pull requests must be properly labeled with the appropriate labels.
- All pull requests must be properly linked to the appropriate issue.

CLI completion note:

- Zsh completion for `gh` may be loaded correctly while `gh` itself still returns no positional completions for PR or issue numbers.
- If `gh pr merge <TAB>` does not fill in PR identifiers, verify with `gh __complete pr merge ""`. If it returns only `:0`, that is a `gh` completion limitation, not necessarily a shell setup problem.

### Labels

Use the colon-prefixed label taxonomy -- never create ad-hoc labels:

- `type:` bug, feature, docs, refactor, compatibility, build, migration
- `area:` injection, memory, process, ui, build, profiles, cli
- `platform:` steam-deck, linux, macos, wine, proton
- `priority:` critical, high, medium, low
- `status:` needs-triage, in-progress, blocked, needs-info
- Standalone: `good first issue`, `help wanted`, `duplicate`, `wontfix`

## Best Practices

### Code Organization

- Keep cross-platform code in `common/`
- Use platform-specific directories for OS-specific functionality
- Test changes across multiple platforms when possible
- Use the detection functions provided by `platform.sh`

### Security Considerations

- Never commit secrets or tokens to the repository
- Use 1Password CLI or environment variables for secrets
- Follow XDG Base Directory Specification for config placement
- Disable telemetry and tracking where possible

### Performance Optimization

- Use lazy loading for expensive operations (NVM, completions)
- Cache expensive computations when possible
- Minimize shell startup time with conditional loading
- Use modern tools for better performance when available

---

This enhanced architecture provides a robust, maintainable foundation for cross-platform development environments while maintaining the flexibility and power users expect. The common configuration system ensures consistency across shells and platforms while providing intelligent fallbacks and platform-specific optimizations.

---

## Precedence

1. System, developer, and explicit user instructions for the task.
2. This file and [`AGENTS.md`](AGENTS.md) as repo policy.
3. General best practices when nothing above conflicts.

## MUST / MUST NOT

- **Secrets**: **Never** commit `.env`, `.env.encrypted`, tokens, or API keys. `.envrc` files execute code — treat them as privileged.
- **Issues**: Use the YAML form templates under `.github/ISSUE_TEMPLATE/` when present. **Do not** create title-only or template-bypass issues. If `gh issue create --template` fails, create the issue via GitHub API/tooling with a body that mirrors the form fields, then apply correct labels — **not** a vague one-liner.
- **Pull requests**: Follow `.github/pull_request_template.md` when present. **Always** link the related issue (`Closes #…`). **Label** PRs using the project taxonomy — **never** invent ad-hoc labels.
- **Commits**: Use **Conventional Commits 1.0.0** — `feat|fix|docs|refactor|perf|test|build|ci|chore(scope): …`. Write the title as you want it to appear in `CHANGELOG.md`.
- **Internal docs commits**: Files under `docs/plans`, `docs/research`, or `docs/internal` **must** use `docs(internal): …`. Other non-user-facing churn: prefer `chore(…): …` to stay out of release notes.
- **Large features**: Split into smaller phases and tasks with clear dependencies and order of execution.
- **Cross-platform changes**: Keep shared logic in `common/`; put OS-specific code under `common/macos/` or `common/linux/`. Use the detection helpers from `common/platform.sh`.
- **MCP**: When an MCP server fits the task (GitHub, docs, browser, etc.), **prefer it**. **Read** each tool's schema/descriptor before calling.

## SHOULD (implementation)

- **Default to worktrees**: For non-trivial work (multi-step features, refactors, changes touching multiple files in `common/`, `bash/`, `zsh/`, `bin/`, or `playbooks/`), start in a git worktree instead of the main checkout. See [Git Worktrees](#git-worktrees) below. Fall back to the main checkout only when the task is a one-liner, must observe the current working tree state, or worktree creation is blocked (detached HEAD, shallow clone, submodule issues).
- **Shell (Bash/Zsh)**: Use `[[ ]]` over `[ ]`; quote all expansions; prefer `$(…)` over backticks; guard optional tools with `command -v` before use; run `shellcheck` on changes to `bash/`, `zsh/`, `common/`, `bin/`, `scripts/`.
- **JavaScript/TypeScript** (secondary): `PascalCase` components; `camelCase` hooks and functions; prefer `unknown` over `any`; errors-as-values where idiomatic.
- **Python** (secondary): PEP 8 throughout; type hints required on all public API signatures; prefer `ruff` for linting and `mypy --strict` for type checking.
- **Ansible**: Playbooks idempotent; always testable via `--extra-vars "dryrun=true"`; prefer native modules over `command:`/`shell:`.

## Git & Conventional Commits

This project uses **Conventional Commits 1.0.0**. Every commit title must match:

```
<type>[optional scope]: <description>
```

### Types

| Type       | Purpose                                     | Version bump |
| ---------- | ------------------------------------------- | ------------ |
| `feat`     | New user-facing feature                     | minor        |
| `fix`      | User-facing bug fix                         | patch        |
| `docs`     | Documentation only                          | —            |
| `refactor` | Code change that is neither fix nor feature | —            |
| `perf`     | Performance improvement                     | —            |
| `test`     | Adding or correcting tests                  | —            |
| `build`    | Build system or external dependency changes | —            |
| `ci`       | CI/CD configuration changes                 | —            |
| `chore`    | Other non-user-facing changes               | —            |
| `style`    | Formatting/whitespace only                  | —            |

### Scope

`feat(bash): …`, `fix(nvim): …`, `refactor(common): …` — scope is the module, tool dir, or area of change. Keep it concise.

### Breaking changes

Append `!` after the type/scope (`feat!: …`) **or** add a `BREAKING CHANGE: …` footer. Either triggers a major version bump.

### Internal docs

Use `docs(internal): …` for files under `docs/plans`, `docs/research`, or `docs/internal`. These stay out of release notes.

A commit-message template is installed at `.gitmessage`. Activate it once with:

```bash
git config commit.template .gitmessage
```

## Git Worktrees

**Strong preference**: work in a git worktree for any non-trivial task. Worktrees keep the main checkout clean for parallel work, let multiple agents run concurrently without stepping on each other, and make it trivial to abandon a failed attempt (`git worktree remove`). Use the main checkout only when the task is a one-liner, requires observing the current working tree state, or worktree creation is blocked.

The Claude Code harness defaults to creating worktrees inside the current repo at `<repo>/.claude/worktrees/`. That location pollutes the working tree, shows up in `git status` of unrelated repos, and is easy to forget and hard to reap.

- **Preferred parent**: `~/.claude-worktrees/` for all agent-managed worktrees, named `<repo>-<branch>/`. Keeps them outside every repo and trivially bulk-clean.
- **Manual creation**: when invoking `git worktree add` yourself, target `~/.claude-worktrees/<repo>-<branch>/` — never a path inside the current repo. The `wt` / `wtc` / `wtl` / `wtd` aliases in `common/aliases.sh` wrap `branchlet`; if you use `branchlet`, note that its default `worktreePathTemplate` (`branchlet/settings.json`) places worktrees as siblings to the repo, not under `~/.claude-worktrees/` — override the template if you want the global convention.
- **Harness-created worktrees** (`isolation: "worktree"`, `EnterWorktree`): the only way to relocate these is a `WorktreeCreate` hook in `~/.claude/settings.json`. No environment variable or settings key controls the parent directory directly — the hook receives the intended path and returns a replacement path.
- **Repo hygiene**: if the harness has already created `<repo>/.claude/worktrees/`, add `.claude/worktrees/` to `.gitignore` before committing. Do **not** commit harness worktree state into this repo.
- **Symlinks**: when a worktree needs the same `~/.config/dotfiles` symlinks the main checkout already has, do not re-run `playbooks/symlinks.yml` against the worktree path — the playbook is keyed to a single source root. Test inside the worktree using sourced configs (`source common/platform.sh`) instead.

## Testing & Verification

Run the following before marking any task complete:

```bash
npm run lint          # shell + python + go
npm run lint:md       # markdown (opt-in; excluded from the default lint)
npm run go:build      # go build -o dotf ./cmd/dotf
```

See [docs/development/testing.md](docs/development/testing.md) for the three verification tiers (CI, local container rehearsal, device acceptance).

For shell/symlink changes, additionally run:

```bash
bash -l -c 'echo "Bash config loaded successfully"'
zsh  -l -c 'echo "Zsh config loaded successfully"'
dotf link --dry-run
```

For Go (`dotf` CLI) changes, additionally run:

```bash
go build ./...
go vet ./...
go test -race ./...
golangci-lint run ./...   # or: npm run lint  (shell + python + go)
```

After substantive changes, confirm the output matches expectations — do not rely solely on "it ran without error".

## Stack Overview

This repo is a **Go + shell mono-repo**: the shell configuration system remains the
default, and a Go provisioning CLI (`dotf`) is being grown alongside it under
`cmd/dotf` + `internal/*` (module `github.com/yandy-r/dotfiles`, Go 1.26). The bash
system stays untouched and authoritative during the rewrite (parallel-then-cutover).

As of Phase 4/5, `dotf link` (`internal/symlink`) natively provisions the symlink set (the
Ansible `playbooks/symlinks.yml` is retired but retained one release for rollback) and
`dotf sysconfig` (`internal/sysconfig`) owns per-OS system config
(hostname/timezone/locale/shell/firewall/ssh-server; ssh-server is a no-op on macOS; Windows deferred). As of Phase 6, `dotf apply`
(`cmd/dotf/apply.go`) is the headless orchestration command — it runs sysconfig → packages →
symlinks → plasma → firewall/ssh-server in one ordered pass with `bootstrap.sh` flag/env parity
(`--headless`/`--force` + env twins), an opt-in `--snapper` pre-apply snapshot, an opt-in
`--plasma`/`PLASMA=true` Plasma phase (Linux/KDE only, no-op elsewhere), and a run-wide non-fatal
failure summary. As of Phase 7, `bootstrap.sh` is a POSIX download-and-exec shim (~608 lines) that downloads the
checksum-verified `dotf` binary from GitHub Releases and execs `dotf apply` — Phase 7 shipped
`.goreleaser.yml`, `.github/workflows/release.yml`, and the shim, making `dotf apply` the entry
point. The bash provisioning path is retired as the entry point; the `bin/install-*.sh` installers
are retained and driven by the manifest (`dev`/`base` for the daily-driver toolset, opt-in
`runtimes`/`ai`/`cloud`/`git-tools`/`system` intents wrapping the legacy installers and setup steps
via `custom-script` for provisioning parity); the Ansible symlink playbook is historical-rollback-only.
`dotf plasma` (`internal/plasma`) owns the curated KDE Plasma 6 configuration set — a native Go
KConfig codec (no `kwriteconfig6` subprocesses) with `export`/`apply`/`diff`/`status` verbs over a
governed managed-file table, and a matching opt-in `dotf apply --plasma` / `PLASMA=true` phase.
`dotf deb` (`internal/debtap`) converts a Debian `.deb` into a pacman-installable package via
the AUR `debtap` tool, verifying the input by its ar header (not its extension), gating on an
existing native package, reading the built artifact's own `.PKGINFO`, and checking translated
dependencies with `pacman -T`. Arch/pacman only; conversion is read-only with respect to the
host and `--install` is the sole elevating step.
The `dotf` subcommands are `version`, `platform`, `plan`, `install`,
`status`, `diff`, `doctor`, `link`, `sysconfig`, `apply`, `lock`, `self-update`, `setup`,
`plasma` (KDE Plasma 6 config set), `deb` (.deb → pacman conversion), `completion` (generate
bash/zsh/fish completion scripts, with `--install`/`--force`/`--dry-run`), and `help` (rich
global/per-command help, also via `--help`/`-h` on any command).

**CLI flag convention (settled — do not "fix"):** `dotf` uses the Go standard-library `flag`
package by deliberate choice, not Cobra/pflag or any framework. Flags are shown with GNU-style
double dashes (`--dry-run`, `--manifest`); a single dash (`-dry-run`) is **equivalent** because
the stdlib `flag` parser is dash-agnostic (its docs: "one or two dashes may be used; they are
equivalent"). Double-dash long flags are the dominant modern Go-CLI convention (chezmoi, gh,
kubectl, docker all use it) — they are **not** a violation. A framework was evaluated and
rejected: the command surface is complete, none of Cobra/pflag's added features (short-flag
clustering, `-x`/`--long` pairs, interspersed args) are used, and adopting one would fight the
load-bearing `0/1/2/3` exit-code + `key=value`-stdout/stderr contract and the XDG `--install`
completion placement (which has no framework equivalent). Reopen only to gain dynamic
always-in-sync completion or if the subcommand set grows substantially.

| Layer               | Technology              | Notes                                                           |
| ------------------- | ----------------------- | --------------------------------------------------------------- |
| Primary language    | **mixed** (shell-first) | Bash + Zsh configurations, helper scripts                       |
| Go (provisioning)   | Go 1.26                 | `dotf` CLI: `cmd/dotf` entrypoint + `internal/*`; subcommands `version`/`platform`/`plan`/`install`/`status`/`diff`/`doctor`/`link` (native symlink engine)/`sysconfig` (per-OS system config)/`apply` (headless orchestration + flag/env parity, snapper, plasma phase, failure summary)/`lock`/`self-update`/`setup`/`plasma` (curated KDE Plasma 6 config set, native KConfig codec)/`deb` (verified .deb → pacman conversion via debtap)/`completion` (bash/zsh/fish completion scripts, `--install`/`--force`/`--dry-run`)/`help` (rich global + per-command help, also `--help`/`-h`); canonical platform detection |
| Secondary languages | JavaScript, Python      | dev tooling (commitlint), utility scripts                       |
| Package manager     | npm                     | dev-only: linters, commit hooks                                 |
| Infrastructure      | Ansible (retired for symlinks) | Symlinks use native `dotf link` (`internal/symlink`); as of Phase 7 `bootstrap.sh` is a shim that downloads `dotf` and execs `dotf apply`. `playbooks/symlinks.yml` is historical-rollback-only |
| CI                  | GitHub Actions          | `.github/workflows/go.yml` (Go test/vet/golangci-lint)          |

`bin/style.sh` (and its `bin/lint.sh` / `bin/format.sh` wrappers) lint and format
**Go, shell, and Python** in one bundle — pass `--go` to scope the Go side
(`go vet`, `golangci-lint`, `gofmt`/`goimports`). There is a single style dispatcher at
`bin/` and a single lefthook config; do not introduce a competing `scripts/style.sh`.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

Rules:

- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).

## Branching & releases

[`RELEASING.md`](RELEASING.md) is the source of truth for branches and releases. Branch
off `main` and PR back into it; every release is tagged from `main`. Never merge one
long-lived branch into another to sync it.
