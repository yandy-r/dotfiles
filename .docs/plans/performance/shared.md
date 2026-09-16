# Performance Optimization

The dotfiles repository has several performance optimizations in place, including lazy loading for expensive operations, shell startup time optimization, and modern tool usage for better performance. The current implementation includes conditional loading of expensive tools like NVM, completion caching, and performance-focused shell configurations. This plan focuses on further optimizing shell startup times, implementing more sophisticated lazy loading, and adding performance monitoring capabilities.

## Relevant Files

- `common/exports.sh`: Performance optimizations including dotfile globbing and bash completion caching
- `common/tools.sh`: Lazy loading setup for version managers and expensive tools
- `zsh/zshrc`: Zsh-specific performance optimizations and conditional loading
- `bash/bashrc`: Bash-specific performance optimizations
- `common/completions.sh`: Completion system optimization and caching
- `nvim/lua/config/lazy.lua`: Neovim lazy loading configuration as reference pattern
- `bootstrap.sh`: Performance-aware setup and tool installation

## Relevant Tables

- Shell startup timing measurements and benchmarks
- Tool loading order and dependency chains
- Performance impact analysis of different configurations

## Relevant Patterns

**Lazy Loading Pattern**: Conditional sourcing of expensive tools only when needed (`common/tools.sh:292-294` for NVM example)

**Performance Optimization Pattern**: Disabling expensive features and enabling caching (`common/exports.sh:375-382`)

**Conditional Loading Pattern**: Platform and environment-based conditional loading to avoid unnecessary overhead (`zsh/zshrc` and `bash/bashrc`)

**Caching Pattern**: Bash completion caching and tool detection result caching (`common/exports.sh:381-382`)

## Relevant Docs

**`CLAUDE.md`**: You _must_ read this when working on performance optimization guidelines, shell performance best practices, and tool detection optimization.

**`.cursorrules`**: You _must_ read this when working on performance optimization standards, lazy loading patterns, and shell startup time minimization.

**`README.md`**: You _must_ read this when working on performance feature documentation and user experience optimization.
