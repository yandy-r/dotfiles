# Cross-Platform Compatibility Improvements

The dotfiles repository has a sophisticated cross-platform detection system implemented in `common/platform.sh` that supports macOS, Linux distributions (Ubuntu, Fedora, Arch, openSUSE), and WSL. The system provides intelligent OS detection, architecture detection, package manager detection, and platform-specific command resolution. This plan focuses on enhancing the cross-platform compatibility, improving platform-specific optimizations, and expanding support for additional platforms and edge cases.

## Relevant Files

- `common/platform.sh`: Core platform detection engine with OS, architecture, and package manager detection
- `common/exports.sh`: Platform-specific environment variable setup and XDG compliance
- `common/paths.sh`: Cross-platform PATH management and cleanup functions
- `common/macos/functions.sh`: macOS-specific functions and utilities
- `common/linux/aliases.sh`: Linux-specific aliases and commands
- `bootstrap.sh`: Cross-platform setup script with intelligent detection
- `playbooks/symlinks.yml`: Platform-aware symlink creation

## Relevant Tables

- Package manager detection results stored in `$DOTFILES_PKG_MGR`
- Platform-specific command mappings for copy, paste, and open operations
- XDG directory structure compliance across all platforms

## Relevant Patterns

**Platform Detection Pattern**: Uses `detect_os()`, `detect_arch()`, and `detect_container()` functions with comprehensive case statements (`common/platform.sh:12-62`)

**Conditional Loading Pattern**: Platform-specific configurations loaded based on `$DOTFILES_OS` variable (`common/exports.sh:336-372`)

**Unified Configuration Pattern**: Shared configurations with platform-specific overrides using conditional sourcing (`common/exports.sh:388-392`)

**Package Manager Abstraction**: Unified interface for different package managers (Homebrew, apt, dnf, pacman, zypper) (`common/platform.sh:69-214`)

## Relevant Docs

**`CLAUDE.md`**: You _must_ read this when working on cross-platform architecture, platform-specific considerations, and unified configuration systems.

**`.cursorrules`**: You _must_ read this when working on platform-specific guidelines, file organization, and coding standards for cross-platform compatibility.

**`README.md`**: You _must_ read this when working on cross-platform feature documentation, quick start guides, and platform support descriptions.
