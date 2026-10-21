# Dotfiles Documentation

This directory contains comprehensive documentation for the dotfiles repository.

## Quick Navigation

| Section                       | Description                                              |
| ----------------------------- | -------------------------------------------------------- |
| [Architecture](architecture/) | System design and how components work together           |
| [Features](features/)         | Guides for specific features and tools                   |
| [Reference](reference/)       | Complete reference for aliases, functions, and variables |
| [Development](development/)   | Testing tiers and verification workflow                  |
| [Plans](plans/)               | Documentation strategy and roadmap                       |

## Documentation Overview

### Architecture

System design documentation explaining how the dotfiles work.

- **Overview** - High-level architecture and design principles
- **Common System** - How `common/` provides cross-platform configuration
- **Platform Detection** - How OS and architecture detection works

### Features

Guides for using specific features of the dotfiles.

- **Shell Configuration** - Bash and Zsh setup and customization
- **Modern Tools** - Integration with eza, bat, ripgrep, fd, fzf
- **Completions** - Universal completion system for 50+ tools
- **Environment Management** - Using direnv and atuin

### Reference

Complete API documentation and reference material.

- **Aliases** - All available shell aliases
- **Functions** - Utility functions from `common/functions.sh`
- **Environment Variables** - Variables exported by the dotfiles
- [**Manifest Backends**](reference/manifest-backends.md) - Choosing a `dotf`
  install backend (native before `custom-script`) and the escape-hatch governance fence

### Development

Information for contributors and developers.

- [**Testing**](development/testing.md) - CI, local container rehearsal, and device acceptance tiers

## Getting Started

If you are new to this dotfiles repository, start here:

1. Read the main [README.md](../README.md) for quick start instructions
2. Review the [CLAUDE.md](../CLAUDE.md) for detailed technical documentation
3. Explore the architecture documentation to understand how components connect

## Quick Links

### Configuration Files

| Shell  | Configuration         | Description                         |
| ------ | --------------------- | ----------------------------------- |
| Bash   | [bash/](../bash/)     | Bash shell configuration            |
| Zsh    | [zsh/](../zsh/)       | Zsh shell configuration             |
| Common | [common/](../common/) | Cross-platform shared configuration |

### Key Scripts

| Script                          | Purpose                        |
| ------------------------------- | ------------------------------ |
| [bootstrap.sh](../bootstrap.sh) | POSIX shim: downloads `dotf` (prerelease-aware), runs `dotf apply` |
| [bin/](../bin/)                 | Utility and installer scripts  |
| [playbooks/](../playbooks/)     | Ansible automation             |

### Application Configs

| Application | Directory                   | Description                 |
| ----------- | --------------------------- | --------------------------- |
| Neovim      | [nvim/](../nvim/)           | LazyVim-based configuration |
| WezTerm     | [wezterm/](../wezterm/)     | Terminal emulator config    |
| Ghostty     | [ghostty/](../ghostty/)     | Terminal emulator config    |
| AeroSpace   | [aerospace/](../aerospace/) | Window manager (macOS)      |

## Common Tasks

### Test platform detection

```bash
source ~/.config/dotfiles/common/platform.sh
echo "OS: $DOTFILES_OS, Arch: $DOTFILES_ARCH"
```

### Check tool availability

```bash
source ~/.config/dotfiles/common/tools.sh
system_health
```

### Manage symlinks

```bash
# Test changes (dry run)
dotf link --dry-run

# Apply symlinks (real files are backed up first)
dotf link
```

### Reload shell configuration

```bash
exec $SHELL -l
```

## File Organization

```
docs/
├── README.md                    # This file - documentation index
├── fedora-first-run.md          # Fedora device acceptance checklist
├── fedora-rehearsal.md          # Local Fedora container rehearsal
├── plans/
│   └── documentation-strategy.md
├── architecture/
│   ├── overview.md              # System architecture
│   ├── common-system.md         # common/ directory design
│   └── platform-detection.md    # Platform detection flow
├── features/
│   ├── shell-configuration.md   # Shell setup guide
│   ├── modern-tools.md          # Modern CLI tools
│   ├── completions.md           # Completion system
│   └── environment-management.md
├── reference/
│   ├── aliases.md               # Alias reference
│   ├── functions.md             # Function reference
│   ├── environment-variables.md
│   └── manifest-backends.md     # dotf install backend ladder + governance
└── development/
    └── testing.md               # Testing tiers and verification
```

## Related Documentation

- [Playbooks README](../playbooks/README.md) - Ansible automation documentation
- [Installers Reference](../bin/installers.md) - Installer scripts documentation
- [Non-Interactive Mode](../bin/NON_INTERACTIVE.md) - Automated installation guide
- [Renovate (Forgejo)](renovate.md) - Self-hosted dependency updates for the Forgejo origin
- [Fedora First Run](fedora-first-run.md) - Device acceptance checklist, walked before a release tag is cut
- [Fedora Rehearsal](fedora-rehearsal.md) - The two local containers that cover what CI cannot

---

Last updated: 2026-08-28
