# Scripts Directory

This directory contains utility scripts, installer scripts, and development tools for the dotfiles
repository. All scripts are designed to be cross-platform and follow consistent conventions.

## Quick Reference

| Category         | Scripts                                                                              | Purpose                     |
| ---------------- | ------------------------------------------------------------------------------------ | --------------------------- |
| Setup            | `bootstrap.sh`                                                                       | Shim: downloads `dotf`, runs `dotf apply` |
| Package Managers | `install-brew.sh`, `up-brew.sh`, `un-brew.sh`                                        | Homebrew management         |
| Cloud Tools      | `install-aws-tools.sh`, `install-azure-cli.sh`, `install-1password-cli.sh`           | Cloud CLI tools             |
| DevOps           | `install-hashicorp-tools.sh`, `k8s-tools.sh`, `install-docker.sh`                    | Infrastructure tools        |
| Modern CLI       | `install-modern-cli.sh`, `install-yazi.sh`, `install-espanso.sh`                     | Modern command-line tools   |
| AI/Development   | `install-claude.sh`, `install-codex.sh`, `install-opencode.sh`, `install-omp.sh`, `install-playwright-cli.sh`, `install-fabric.sh`, `install-neovim.sh` | Development tools           |
| Utilities        | `cursor-export-secrets.sh`, `dc-manage.sh`, `cleanup-*.sh`, `markdown-tools.sh`      | System utilities            |
| Style / Lint     | `style.sh`, `lint.sh`, `format.sh`                                                   | Multi-language lint/format  |

## Key Scripts

### bootstrap.sh (Root Directory)

As of Phase 7, `bootstrap.sh` is a POSIX download-and-exec shim: it detects OS/arch, downloads the
checksum-verified `dotf` binary from GitHub Releases (prerelease-aware fallback when
`/releases/latest` 404s), and execs `dotf apply` (the headless-capable orchestrator). Private
repos need `GH_TOKEN` / `GITHUB_TOKEN` or `gh auth login`. It replaces the retired bash
provisioning body.

```bash
# Guided install (downloads `dotf`, runs the `dotf apply` intent TUI)
./bootstrap.sh

# Non-interactive / headless `dotf apply` via the shim (env parity)
NON_INTERACTIVE=true ./bootstrap.sh

# No prebuilt asset: install official Go under ~/.local/go and build from this clone
./bootstrap.sh --install-go
```

**What it does:**

- Detects OS and architecture
- Downloads the prebuilt, checksum-verified `dotf` binary from GitHub Releases (including betas)
- Execs `dotf apply`, which orchestrates system config → packages → symlinks (`dotf link`) → firewall
- On download failure from a clone: TTY prompt (default N) or `--install-go` to install Go 1.26+
  under `~/.local/go` and build; non-interactive without `--install-go` prints auth + manual steps

### install-brew.sh

Install and configure Homebrew with common development tools.

```bash
# Install Homebrew and packages
./bin/install-brew.sh

# Preview without installing
./bin/install-brew.sh --dry-run

# Skip specific categories
SKIP_CASKS=true ./bin/install-brew.sh
```

**Options:**

- `--dry-run` - Preview what would be installed
- `SKIP_FORMULAS=true` - Skip formula installation
- `SKIP_CASKS=true` - Skip cask installation (macOS)
- `SKIP_MAS=true` - Skip Mac App Store apps (macOS)

### up-brew.sh

Update Homebrew and all installed packages.

```bash
# Update everything
./bin/up-brew.sh

# Check for updates without installing
./bin/up-brew.sh --check
```

### un-brew.sh

Uninstall Homebrew completely (use with caution).

```bash
# Preview what would be removed
./bin/un-brew.sh --dry-run

# Actually uninstall
./bin/un-brew.sh
```

## Installer Scripts

All installer scripts share common features:

- `--check` - Verify installed versions
- `--dry-run` - Preview changes without installing
- `--dir PATH` - Custom installation directory
- Selective installation by specifying tool names

### Cloud Tools

#### install-aws-tools.sh

```bash
# Install all AWS tools
./bin/install-aws-tools.sh

# Install specific tools
./bin/install-aws-tools.sh aws sam ecs-cli

# Check versions
./bin/install-aws-tools.sh --check
```

**Tools:** AWS CLI v2, SAM CLI, ECS CLI

#### install-azure-cli.sh

```bash
# Install Azure CLI
./bin/install-azure-cli.sh

# Check version
./bin/install-azure-cli.sh --check
```

**Tools:** Azure CLI (az)

### DevOps Tools

#### install-hashicorp-tools.sh

```bash
# Install all HashiCorp tools
./bin/install-hashicorp-tools.sh

# Install specific tools
./bin/install-hashicorp-tools.sh terraform tflint

# Check versions
./bin/install-hashicorp-tools.sh --check
```

**Tools:** Terraform, TFLint

#### k8s-tools.sh

```bash
# Install all Kubernetes tools
./bin/k8s-tools.sh

# Install specific tools
./bin/k8s-tools.sh kubectl helm k9s

# Update tools
./bin/k8s-tools.sh --update kubectl
```

**Tools:** kubectl, helm, k9s, kubectx, kubens, kustomize, stern, krew

#### install-container-tools.sh

```bash
# Install container tools
./bin/install-container-tools.sh

# Install specific tools
./bin/install-container-tools.sh skaffold hadolint
```

**Tools:** Skaffold, Hadolint

#### install-docker.sh

```bash
# Install Docker
./bin/install-docker.sh

# Check version
./bin/install-docker.sh --check
```

### Modern CLI Tools

#### install-modern-cli.sh

Install modern command-line tools from GitHub releases.

```bash
# Install all modern CLI tools
./bin/install-modern-cli.sh

# Install specific tools
./bin/install-modern-cli.sh lazygit delta eza

# Custom directory
./bin/install-modern-cli.sh --dir ~/.local/bin
```

**Tools:** lazygit, delta, bottom (btm), eza, carapace, xh

#### install-yazi.sh

Install Yazi terminal file manager.

```bash
./bin/install-yazi.sh
```

### Development Tools

#### install-neovim.sh

Install Neovim (nightly or stable).

```bash
# Install Neovim
./bin/install-neovim.sh

# Check version
./bin/install-neovim.sh --check
```

#### install-lua.sh

Install Lua and LuaRocks for Neovim plugin development.

```bash
./bin/install-lua.sh
```

#### install-mise.sh

Install mise (polyglot runtime manager).

```bash
./bin/install-mise.sh
```

### AI Coding Tools

#### install-claude.sh

Install Claude Code CLI.

```bash
./bin/install-claude.sh

# Check version
./bin/install-claude.sh --check
```

#### install-opencode.sh

Install OpenCode 2. Removes an OpenCode 1 package install (`opencode` formula, `opencode-ai` npm
package, `opencode-bin` AUR package) while preserving `~/.config/opencode` and OpenCode data, then
installs V2 with the official per-OS method.

```bash
# Install OpenCode 2 (migrates away from V1 when present)
./bin/install-opencode.sh --install

# Update OpenCode 2
./bin/install-opencode.sh --update

# Remove binaries but keep configuration and data
./bin/install-opencode.sh --uninstall
```

#### install-codex.sh

Install the OpenAI [Codex CLI](https://learn.chatgpt.com/docs/codex/cli#getting-started) with the
official checksum-verifying standalone installer (`https://chatgpt.com/codex/install.sh`) instead of
npm. Any npm, pnpm, bun, or Homebrew install of `@openai/codex` is removed first; a distro package
(e.g. Terra's `codex` RPM) is reported but left alone. Included in dotf's opt-in `ai` intent.

```bash
dotf install --only codex
dotf install --only codex --update
dotf install --only codex --remove

./bin/install-codex.sh                       # no-op when installed natively (still migrates npm/pnpm)
./bin/install-codex.sh --update
./bin/install-codex.sh --force               # reinstall and approve prompts
./bin/install-codex.sh --version 0.163.0 --update
./bin/install-codex.sh --uninstall
./bin/install-codex.sh --check
./bin/install-codex.sh --dry-run
NON_INTERACTIVE=true ./bin/install-codex.sh --update
```

The command is a symlink at `~/.local/bin/codex` (override with `CODEX_INSTALL_DIR` or `--dir`)
into `~/.codex/packages/standalone`. The PATH block upstream appends to shell profiles is
stripped, because this repo manages PATH. Uninstall removes only the symlink and the standalone
packages; `~/.codex` configuration, auth, sessions, and skills are preserved.

#### install-omp.sh

Install [oh-my-pi](https://omp.sh/) (`omp`) using the official checksum-verifying
standalone binary installer on macOS/Linux (including WSL). No Bun runtime or shell
file edits are needed. Included in dotf's opt-in `ai` intent.

```bash
dotf install --only omp
dotf install --only omp --update
dotf install --only omp --remove
# Or install it with the rest of the AI tools:
dotf apply --intent ai

./bin/install-omp.sh                       # no-op when installed
./bin/install-omp.sh --update
./bin/install-omp.sh --force               # reinstall and approve prompts
./bin/install-omp.sh --version 18.8.9 --force
./bin/install-omp.sh --uninstall
./bin/install-omp.sh --debugger-only        # ensure debugpy without reinstalling omp
./bin/install-omp.sh --skip-debugger        # omit the optional debugger setup
./bin/install-omp.sh --dry-run
```

The binary goes to `~/.local/bin/omp`; override with `PI_INSTALL_DIR` or `--dir`
(an absolute path). `--version` / `OMP_VERSION` accepts release numbers with or
without `v`. `--check` prints the installed version. Updates reinstall the selected
release without deleting the old binary first. Uninstall removes only the standalone
binary, preserving `~/.omp`, XDG configuration/data, credentials, and sessions.
Existing Bun/Homebrew installs are left alone; update/remove those with their original
package manager. Symlink targets are never overwritten or removed.

Direct install/uninstall prompts require a TTY; set `NON_INTERACTIVE=true` for
headless approval for the omp binary (dotf does this automatically). `--dry-run`
performs no downloads or writes. After installation, run `omp` to configure your provider.

Install/update (even when omp is already present) also checks for **debugpy**, the
Python adapter used by omp's builtin debugger. Missing dependencies are explained
and offered with a confirmation prompt. Headless runs automatically install them
with a notice. Use `--debugger-only` to repair just the debugger on existing installs,
including Bun/Homebrew omp installs; use `--skip-debugger` or
`OMP_SKIP_DEBUGGER=true` to omit the optional check.

The check imports `debugpy.adapter` using `python`, matching omp's default command.
On Python3-only systems it falls back to `python3` and also sets up the unversioned
command omp expects. Externally managed Python uses OS packages, with sudo when
needed: Debian/Ubuntu installs `python3-debugpy` plus `python-is-python3` when
`python` is missing; Fedora uses `python3-debugpy` and `python-unversioned-command`;
Arch uses `python-debugpy`. Other interpreters use `python -m pip` (active virtualenv,
writable site, or `--user`), and Python3-only installs offer a user-local
`~/.local/bin/python` symlink if that directory is on PATH. Existing files are
never overwritten. No `sudo pip` or `--break-system-packages` is used.

Missing pip, unsupported OS package managers, or installation/verification failures
produce actionable warnings; optional debugger failures do not undo a successful
omp installation, while `--debugger-only` returns a failure status. `--check` reports
debugger readiness without installing anything. Uninstall keeps debugpy because
other tools may share it. This configures the **Python** debugger only; other
languages need their own DAP adapters. A project virtualenv/custom DAP override
may use a different interpreter and need its own debugpy installation.

#### install-playwright-cli.sh

Install Microsoft's `@playwright/cli` npm package and its bundled agent skill. Skill scope defaults
to global. Every agent receives its own native directory: Claude (`~/.claude/skills`), Codex
(`~/.codex/skills`), Cursor/Cursor Agent (`~/.cursor/skills`), OpenCode
(`~/.config/opencode/skills`), and Antigravity (`~/.gemini/antigravity/skills`), plus the shared
`~/.agents/skills` copy for interoperability. Local installs write to `<project>/.claude/skills`,
`<project>/.codex/skills`, `<project>/.cursor/skills`, `<project>/.opencode/skills`, and
`<project>/.agents/skills`.

```bash
# Install CLI globally and skills for the current user (default)
./bin/install-playwright-cli.sh

# Install CLI globally and skills in the current project
./bin/install-playwright-cli.sh --local

# Install local skills for another project
./bin/install-playwright-cli.sh --local --project ~/src/my-app

# Refresh skills without reinstalling the CLI
./bin/install-playwright-cli.sh --skills-only
```

#### install-fabric.sh

Install Daniel Miessler's Fabric CLI for prompt patterns and terminal AI workflows. The installer
tries the official Fabric installer first, then Homebrew/AUR `fabric-ai`, then `go install` when Go
is available.

```bash
# Install Fabric
./bin/install-fabric.sh --install

# Update Fabric
./bin/install-fabric.sh --update

# Uninstall Fabric from known install locations/package managers
./bin/install-fabric.sh --uninstall
```

After installation, run Fabric's interactive provider setup manually. Use `fabric-ai --setup`
instead if your package manager installed only the `fabric-ai` command and you have not reloaded the
shell integration yet:

```bash
fabric-ai --setup
```

If your package manager installs the binary as `fabric-ai`, the shell integration normalizes Fabric
helper aliases/functions to use that command when `fabric` is not present.

#### install-coderabbit.sh

Install CodeRabbit CLI.

```bash
./bin/install-coderabbit.sh
```

#### install-1password-cli.sh

Install, update, or uninstall the [1Password CLI](https://www.1password.dev/cli) (`op`). macOS uses the
`1password-cli` Homebrew cask, Windows uses winget, and Linux/WSL downloads the official release zip,
verifies its GPG signature against the pinned 1Password key, and installs `/usr/local/bin/op` setgid
`onepassword-cli` (needed for the desktop-app integration). Provisioned by `dotf apply --intent cloud`
or `dotf install --only 1password-cli`.

```bash
./bin/install-1password-cli.sh               # install (no-op when present)
./bin/install-1password-cli.sh --update      # update to latest
./bin/install-1password-cli.sh --version 2.38.1 --force
./bin/install-1password-cli.sh --uninstall
```

#### install-espanso.sh

Install, update, or remove [espanso](https://espanso.org) (text expander). On Linux it detects the
X11 or Wayland session (`XDG_SESSION_TYPE`, `WAYLAND_DISPLAY`/`DISPLAY`, then `loginctl`) and picks
the upstream-recommended method: the official `.deb` on Debian/Ubuntu (amd64), the Terra repo on
Fedora, the AUR (`espanso-x11`/`espanso-wayland` via paru/yay) on Arch, the X11 AppImage elsewhere,
and a cargo source build as the last resort. Wayland installs get `cap_dac_override`, and the systemd
user service is registered and started. macOS uses the `espanso` Homebrew cask; WSL installs the
Windows build via `winget.exe`. Configuration is never touched by `--remove`.

```bash
./bin/install-espanso.sh                     # install (no-op when present)
./bin/install-espanso.sh --update            # update and restart if it was running
./bin/install-espanso.sh --remove            # stop, unregister, uninstall
./bin/install-espanso.sh --x11 --method appimage --dry-run
```

> **Migrated to native `dotf` backends (Phase 3, #50):** `lefthook` and `turso` are no longer
> provisioned by `bin/install-*.sh`. They ship in `manifest/packages.toml` — `lefthook` via the
> `toolchain`/npm backend, `turso` via the `github-release` backend — and are installed by
> `dotf apply` / `dotf install`. Run `lefthook install` in the repo after provisioning to wire
> `.git/hooks/pre-commit`, `pre-push`, and `commit-msg`.

### Utility Scripts

#### dc-manage.sh

Docker Compose management tool.

```bash
# Start services
./bin/dc-manage.sh up

# Stop services
./bin/dc-manage.sh down

# View logs
./bin/dc-manage.sh logs
```

#### tea-tools

Small wrapper around the Gitea/Forgejo `tea` CLI for workflows that `tea` does not expose directly.
Unsupported commands are passed through to `tea` unchanged.

```bash
# Merge a PR and delete the detected source branch remotely and locally
tea-tools pr merge --style squash --delete-branch 7

# Preview the resolved branch and cleanup steps without merging or deleting
tea-tools pr merge --delete-branch --dry-run 7

# Install or refresh the completion file in the default zsh completion directory
tea-tools completion zsh

# Print the generated completion script for inspection/debugging
tea-tools completion print zsh
```

`tea-tools pr merge --delete-branch` resolves the source branch from the PR API, runs `tea pr merge`,
deletes the remote branch through the authenticated API, then deletes the matching local branch. If
the local PR branch is currently checked out, it switches to the PR base branch first only when the
working tree is clean.

Run `tea-tools completion <shell>` whenever the wrapper changes to refresh the generated completion
file. For zsh, the default target is `~/.zsh/completions/_tea-tools`, matching the existing external
completion path used by `_tea`. Bash and fish use their normal user completion directories.

#### cleanup-temp.sh

Clean temporary files and caches.

```bash
./bin/cleanup-temp.sh
```

#### cleanup-omz.sh

Clean up Oh My Zsh remnants after migration.

```bash
./bin/cleanup-omz.sh
```

#### markdown-tools.sh

Markdown file processing utilities.

```bash
./bin/markdown-tools.sh
```

#### standardize-md-names.sh

Standardize markdown filenames.

```bash
./bin/standardize-md-names.sh
```

#### git-url-replace.sh

Replace git URLs (e.g., HTTPS to SSH).

```bash
./bin/git-url-replace.sh
```

#### cursor-export-secrets.sh

Extract the Cursor IDE access token and machine ID from Cursor's local SQLite state database
(`state.vscdb`) and write them as JSON to `${XDG_CONFIG_HOME:-~/.config}/secrets/cursor.json`
(default). Manual utility — not wired into bootstrap. OS (macOS / Linux / WSL) is auto-detected
via `common/platform.sh`, which maps to the correct Cursor User directory. Native Windows is
rejected: POSIX modes cannot enforce private file ACLs there. Use case:
supplying `accessToken` + `machineId` to tools that need Cursor IDE credentials (e.g. the 9router
Cursor IDE provider). Requires `sqlite3` and `jq`.

```bash
# Check dependencies, state DB, and keys (exit 0/1)
./bin/cursor-export-secrets.sh --check

# Preview detected OS, DB path, keys found, and output path (no writes)
./bin/cursor-export-secrets.sh --dry-run

# Export credentials to the default location
./bin/cursor-export-secrets.sh

# Write to a custom output file
./bin/cursor-export-secrets.sh --output ~/.config/secrets/cursor.json
```

**Options:**

- `--dry-run` - Report OS/DB/keys found/output path without writing
- `--check` - Verify `sqlite3`/`jq`, state DB, and keys; exit 0/1
- `--output <path>` - Override output file (default: `${XDG_CONFIG_HOME:-~/.config}/secrets/cursor.json`)
- `-h`, `--help` - Show usage

**Security:** credential values are never printed and never appear in external process arguments
(stdin-piped to `jq`; xtrace disabled). The output file is written atomically (temp-then-rename)
with mode 600; the default parent directory is created/hardened to mode 700. Custom `--output`
parents must already be user-owned and private; symlink parents and symlink/directory output
paths are refused.

### Style / Lint Orchestrator

#### style.sh, lint.sh, format.sh

`style.sh` is the multi-language lint/format orchestrator (Rust, TypeScript, Python, Go, shell,
docs). `lint.sh` and `format.sh` are thin wrappers that forward to `style.sh lint` /
`style.sh format`.

```bash
# Full repo lint / format
bin/lint.sh --all
bin/format.sh --all

# Scope to git-tracked changes (mutually exclusive)
bin/lint.sh --modified --shell --python     # staged + unstaged + untracked
bin/lint.sh --staged --shell --python       # only `git diff --cached` files
bin/lint.sh --unstaged --shell --python     # unstaged + untracked only

bin/format.sh --unstaged --docs             # same flags apply to format
```

Scope flag meanings:

- `--modified` — all modified files: `git diff` + `git diff --cached` +
  `git ls-files --others --exclude-standard`
- `--staged` — `git diff --cached --name-only --diff-filter=ACMR`
- `--unstaged` — `git diff --name-only --diff-filter=ACMR` + untracked files

Full-repo scans also skip common generated/dependency directories such as `node_modules/`,
`vendor/`, `dist/`, `build/`, `coverage/`, `.venv/`, and `target/`.

### Platform-Specific

#### install-arch-pkgs.sh

Install packages on Arch Linux. The Hyprland desktop stack is opt-in via `--hyprland`.

```bash
./bin/install-arch-pkgs.sh
./bin/install-arch-pkgs.sh --hyprland
```

#### install-native-packages.sh

Install packages using native package managers.

```bash
./bin/install-native-packages.sh
```

### Miscellaneous

#### init-branchlet.sh

Initialize Branchlet git worktree manager configuration.

```bash
./bin/init-branchlet.sh
```

#### init-envrc

Initialize `.envrc` for direnv.

```bash
./bin/init-envrc
# Or use the alias:
envrc
```

#### tmux-wrapper.sh

Tmux session wrapper.

```bash
./bin/tmux-wrapper.sh
```

#### tmux-osc52-copy

OSC 52 clipboard copy for tmux over SSH.

```bash
./bin/tmux-osc52-copy
```

## Common Conventions

### Script Structure

All scripts follow this structure:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Constants
readonly SCRIPT_NAME="$(basename "${0}")"

# Color codes
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly NC='\033[0m'

# Functions
usage() { ... }
log_info() { ... }
log_success() { ... }
log_error() { ... }

# Main
main() { ... }
main "$@"
```

### Color-Coded Output

Consistent color scheme across all scripts:

- **BLUE** - Informational messages
- **GREEN** - Success messages
- **YELLOW** - Warnings
- **RED** - Errors
- **CYAN** - Headers/sections

### Platform Detection

Scripts use `common/platform.sh` for detection:

```bash
source "$DOTFILES_ROOT/common/platform.sh"

if is_macos; then
    # macOS specific
elif is_linux; then
    # Linux specific
fi
```

### Error Handling

All scripts use strict error handling:

```bash
set -euo pipefail  # Exit on error, undefined vars, pipe failures
```

## Adding New Scripts

When creating new installer scripts:

1. **Use the template** from existing scripts
2. **Follow naming convention**: `install-{tool}.sh` or `{tool}-tools.sh`
3. **Support common flags**: `--check`, `--dry-run`, `--dir`
4. **Use platform detection** from `common/platform.sh`
5. **Add colored output** for user feedback
6. **Include error handling** with cleanup
7. **Document in this README**
8. **Test on multiple platforms**

### Example Template

```bash
#!/usr/bin/env bash
set -euo pipefail

readonly SCRIPT_NAME="$(basename "${0}")"
readonly DOTFILES_ROOT="${DOTFILES_ROOT:-$HOME/.config/dotfiles}"

# Source platform detection
source "$DOTFILES_ROOT/common/platform.sh"

# Colors
readonly GREEN='\033[0;32m'
readonly RED='\033[0;31m'
readonly NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

usage() {
    cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

OPTIONS:
    --check     Check installed versions
    --dry-run   Preview without installing
    --help      Show this help
EOF
}

main() {
    # Parse arguments
    # Do installation
}

main "$@"
```

## Documentation Files

| File                         | Description                              |
| ---------------------------- | ---------------------------------------- |
| `installers.md`              | Comprehensive installer script reference |
| `NON_INTERACTIVE.md`         | Non-interactive installation guide       |
| `quick-install.md`           | Quick installation reference             |
| `install-native-packages.md` | Native package manager guide             |
| `smartsheet-cli.md`          | Smartsheet CLI documentation             |

## Subdirectories

| Directory       | Contents                                  |
| --------------- | ----------------------------------------- |
| `scripts/`      | Additional utility scripts                |
| `node_modules/` | Node.js dependencies (for smartsheet-cli) |

## Related Documentation

- [installers.md](installers.md) - Detailed installer documentation
- [Main README](../README.md) - Repository overview
- [CLAUDE.md](../CLAUDE.md) - Technical documentation
- [Common System](../common/README.md) - Platform detection details
