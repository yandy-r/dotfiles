# Enhanced CLI Tool Integration

The dotfiles repository already has a solid foundation for modern CLI tool integration through the `common/tools.sh` system. The current implementation provides intelligent tool detection, automatic installation helpers, and configuration setup for modern alternatives like eza, bat, ripgrep, fd, fzf, and delta. This plan focuses on enhancing the integration with additional modern tools, improving the fallback system, and adding more sophisticated tool management capabilities.

## Relevant Files

- `common/tools.sh`: Core tool detection and setup system with MODERN_TOOLS, DEV_TOOLS, and CLOUD_TOOLS arrays
- `common/aliases.sh`: Smart aliases with fallbacks to modern tools (eza → ls, bat → cat, etc.)
- `common/completions.sh`: Universal completion system for 50+ modern CLI tools
- `common/exports.sh`: Environment variable setup for tool configurations
- `bootstrap.sh`: Initial setup script that calls tool installation functions
- `playbooks/symlinks.yml`: Ansible playbook for creating tool configuration symlinks

## Relevant Tables

- MODERN_TOOLS associative array: Maps tool names to descriptions
- DEV_TOOLS associative array: Development tool definitions
- CLOUD_TOOLS associative array: Cloud and DevOps tool definitions

## Relevant Patterns

**Tool Detection Pattern**: Uses `tool_exists()` function with PATH checking and version validation (`common/tools.sh:95-105`)

**Intelligent Fallbacks**: Aliases automatically fall back to native commands when modern tools aren't available (`common/aliases.sh:48-51`)

**Configuration Setup Pattern**: Each tool gets dedicated setup function with environment variables and config file creation (`common/tools.sh:180-286`)

**Package Manager Integration**: Auto-installation through platform-specific package managers with tool name mapping (`common/tools.sh:140-165`)

## Relevant Docs

**`CLAUDE.md`**: You _must_ read this when working on tool integration, performance optimization, and cross-platform compatibility.

**`.cursorrules`**: You _must_ read this when working on tool integration guidelines, modern CLI tool priorities, and fallback patterns.

**`README.md`**: You _must_ read this when working on user experience, feature descriptions, and cross-platform compatibility documentation.
