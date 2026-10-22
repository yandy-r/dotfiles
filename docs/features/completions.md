# Completion System Guide

This guide explains how the universal completion system works in the dotfiles, covering supported tools, lazy loading for performance, and how to add completions for new tools.

## Overview

The dotfiles provide a comprehensive completion system that works across both bash and zsh. The system includes:

- **50+ tools** with pre-configured completions
- **Lazy loading** for heavy completions (kubectl, docker, helm)
- **Cross-shell support** with shell-specific optimizations
- **Homebrew integration** for additional completions

## Architecture

The completion system consists of two main components:

1. **`common/completions.sh`** - Eager-loaded completions for lightweight tools
2. **`common/lazy-completions.sh`** - Lazy-loaded completions for heavy tools

```
Shell Startup
    |
    +-- common/tools.sh
           |
           +-- common/lazy-completions.sh  (sets up wrapper functions)
           +-- common/completions.sh       (loads lightweight completions)
```

## Supported Tools

### Kubernetes and Container Tools

| Tool           | Completion Type | Load Strategy |
| -------------- | --------------- | ------------- |
| kubectl        | Dynamic         | Lazy          |
| helm           | Dynamic         | Lazy          |
| docker         | Dynamic         | Lazy          |
| docker-compose | Dynamic         | Eager         |
| podman         | Dynamic         | Eager         |
| k9s            | Dynamic         | Eager         |
| istioctl       | Dynamic         | Eager         |
| linkerd        | Dynamic         | Eager         |
| kubectx        | Dynamic         | Eager         |
| kubens         | Dynamic         | Eager         |

### Cloud Provider CLIs

| Tool    | Completion Type | Load Strategy |
| ------- | --------------- | ------------- |
| aws     | aws_completer   | Lazy          |
| gcloud  | File-based      | Lazy          |
| az      | File-based      | Eager         |
| eksctl  | Dynamic         | Eager         |
| cdk     | Dynamic         | Eager         |
| sam     | Dynamic         | Eager         |
| copilot | Dynamic         | Eager         |

### Infrastructure as Code

| Tool             | Completion Type | Load Strategy |
| ---------------- | --------------- | ------------- |
| terraform        | Built-in        | Lazy          |
| terragrunt       | Dynamic         | Eager         |
| packer           | Built-in        | Eager         |
| vault            | Built-in        | Eager         |
| consul           | Built-in        | Eager         |
| nomad            | Built-in        | Eager         |
| pulumi           | Dynamic         | Eager         |
| cdktf            | Custom          | Eager         |
| ansible          | Dynamic         | Eager         |
| ansible-playbook | Dynamic         | Eager         |

### Development Tools

| Tool              | Completion Type | Load Strategy |
| ----------------- | --------------- | ------------- |
| git               | File-based      | Eager         |
| gh (GitHub CLI)   | Dynamic         | Eager         |
| glab (GitLab CLI) | Dynamic         | Eager         |
| npm               | Dynamic         | Eager         |
| yarn              | Dynamic         | Eager         |
| pnpm              | Dynamic         | Eager         |
| deno              | Dynamic         | Eager         |
| bun               | Dynamic         | Eager         |
| pip               | Dynamic         | Eager         |
| poetry            | Dynamic         | Eager         |
| cargo             | File-based      | Eager         |

### Security Tools

| Tool                    | Completion Type | Load Strategy |
| ----------------------- | --------------- | ------------- |
| op (1Password)          | Dynamic         | Eager         |
| bws (Bitwarden Secrets) | Dynamic         | Eager         |

### Modern CLI Tools

| Tool         | Completion Type | Load Strategy |
| ------------ | --------------- | ------------- |
| rg (ripgrep) | Dynamic         | Eager         |
| fd           | Dynamic         | Eager         |
| bat          | Dynamic         | Eager         |
| eza          | Dynamic         | Eager         |
| starship     | Dynamic         | Eager         |
| delta        | Dynamic         | Eager         |
| tldr         | Dynamic         | Eager         |

## Lazy Loading System

Heavy completions (kubectl, docker, helm, terraform, aws, gcloud) are lazy-loaded to improve shell startup time.

### How It Works

Instead of loading completions at startup, the system creates wrapper functions that:

1. **Remove the wrapper function** when the command is first used
2. **Load the actual completions** for the tool
3. **Execute the command** with original arguments

### Example: kubectl Lazy Loading

```bash
# Initial state: kubectl is a wrapper function
function kubectl() {
    # Remove this function
    unset -f kubectl

    # Load completions
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        source <(command kubectl completion zsh)
    elif [[ -n "${BASH_VERSION:-}" ]]; then
        eval "$(command kubectl completion bash)"
    fi

    # Execute the real command
    command kubectl "$@"
}
```

### Checking Lazy Loading Status

```bash
# Show which completions have been loaded
lazy_completion_status

# Output example:
# Lazy Completion Status:
#   kubectl:   not loaded
#   docker:    1
#   helm:      not loaded
#   terraform: 1
```

### Force Loading All Completions

```bash
# Load all lazy completions immediately
force_load_all_completions
```

## Homebrew Completions

Homebrew provides additional completions that are automatically integrated:

### Zsh

```bash
# Automatically added to FPATH
$DOTFILES_HOMEBREW_PATH/share/zsh/site-functions
$DOTFILES_HOMEBREW_PATH/share/zsh-completions
```

### Bash

```bash
# Automatically sourced
$DOTFILES_HOMEBREW_PATH/etc/profile.d/bash_completion.sh
```

## Adding Completions for New Tools

### Method 1: Dynamic Completions (Recommended)

For tools that support `--completion` or similar:

1. Edit `common/completions.sh`
2. Add to the appropriate setup function:

```bash
# In setup_modern_tools_completions() or similar:
if command -v mytool >/dev/null 2>&1; then
    load_completion mytool
fi
```

The `load_completion` function tries these patterns:

- `mytool completion $shell`
- `mytool --completion $shell`
- `mytool generate completion $shell`

### Method 2: Lazy Loading for Heavy Tools

For tools with slow completion loading, add to `common/lazy-completions.sh`:

```bash
_setup_lazy_mytool() {
    command -v mytool &>/dev/null || return 0
    [[ -n "$LAZY_LOADED_MYTOOL" ]] && return 0

    mytool() {
        unset -f mytool 2>/dev/null

        if [[ -n "${ZSH_VERSION:-}" ]]; then
            source <(command mytool completion zsh) 2>/dev/null
        elif [[ -n "${BASH_VERSION:-}" ]]; then
            eval "$(command mytool completion bash 2>/dev/null)" 2>/dev/null
        fi

        LAZY_LOADED_MYTOOL=1
        command mytool "$@"
    }
}

# Add to setup_lazy_completions():
setup_lazy_completions() {
    # ... existing tools ...
    _setup_lazy_mytool
}
```

### Method 3: File-Based Completions

For tools with static completion files:

```bash
# Zsh
if [[ -f /path/to/mytool-completion.zsh ]]; then
    source /path/to/mytool-completion.zsh
fi

# Bash
if [[ -f /path/to/mytool-completion.bash ]]; then
    source /path/to/mytool-completion.bash
fi
```

### Method 4: Alias Completions

To add completions for aliases:

```bash
# Zsh
compdef k=kubectl
compdef tf=terraform

# Bash
complete -o default -F __start_kubectl k
complete -C terraform tf
```

## Carapace Universal Completion

The dotfiles support Carapace, a universal completion engine:

### Installation

```bash
# macOS
brew install carapace

# From source
go install github.com/rsteube/carapace-bin@latest
```

### Configuration

Carapace bridges completions across shells:

```bash
export CARAPACE_BRIDGES='zsh,fish,bash,inshellisense'
```

In zsh, `zsh/zshrc` sources `carapace _carapace` eagerly, after the single `compinit`, so every bridged completion is
registered at startup. In bash, `common/lazy-completions.sh` loads it the first time the `carapace` command runs.

### Benefits

- Single source of completions works across shells
- Supports tools that don't have native completions
- Enhanced completion descriptions

## Troubleshooting

### Completions Not Working

1. **Check if tool is installed**:

   ```bash
   command -v kubectl
   ```

2. **Check if completions are loaded**:

   ```bash
   # Zsh
   which kubectl

   # Bash
   type kubectl
   ```

3. **Force reload completions**:

   ```bash
   # Reload shell
   exec $SHELL -l

   # Or force load specific completion
   kubectl version --client  # Triggers lazy loading
   ```

### Slow Shell Startup

If startup is slow despite lazy loading:

1. **Profile startup time**:

   ```bash
   # Zsh
   time zsh -i -c exit

   # Bash
   time bash -i -c exit
   ```

2. **Disable completions temporarily**:

   ```bash
   export DOTFILES_NO_AUTO_COMPLETIONS=1
   export DOTFILES_NO_LAZY_COMPLETIONS=1
   ```

3. **Check which completions are slow**:

   ```bash
   DOTFILES_DEBUG=1 exec $SHELL -l
   ```

### Completion Conflicts

If completions from different sources conflict:

```bash
# Zsh: Check completion sources
echo $fpath

# Remove duplicate completion directories
# Edit zshrc.local to adjust fpath

# Bash: Check completion sources
complete -p kubectl
```

### Missing Completion Function

For "command not found: compdef" errors:

```bash
# Ensure compinit is loaded first (zsh)
autoload -Uz compinit && compinit
```

## Configuration Options

### Disable Auto-Loading

```bash
# Add to ~/.bashrc.local or ~/.zshrc.local

# Disable all eager completions
export DOTFILES_NO_AUTO_COMPLETIONS=1

# Disable lazy completions
export DOTFILES_NO_LAZY_COMPLETIONS=1
```

### Custom Completion Paths

```bash
# Zsh: Add custom completion directory
fpath=(~/.local/share/zsh/completions $fpath)

# Bash: Source custom completions
source ~/.local/share/bash/completions/*.bash
```

## Performance Considerations

### Startup Time Impact

| Completion Type | Impact | Recommendation      |
| --------------- | ------ | ------------------- |
| Lazy-loaded     | None   | Use for heavy tools |
| Dynamic (fast)  | ~50ms  | Use for light tools |
| File-based      | ~10ms  | Use when available  |

### Best Practices

1. **Use lazy loading** for tools you don't use frequently
2. **Prefer Homebrew completions** when available (pre-cached)
3. **Avoid sourcing** completion scripts in tight loops
4. **Test startup time** after adding new completions

## See Also

- [Shell Configuration Guide](shell-configuration.md) - Shell setup and customization
- [Modern CLI Tools Guide](modern-tools.md) - Tool integrations
- [Environment Management Guide](environment-management.md) - direnv and atuin setup
