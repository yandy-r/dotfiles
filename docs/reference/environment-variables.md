# Environment Variables Reference

This document provides a complete reference for all environment variables set by the dotfiles repository. Variables are organized by category and include information about defaults and requirements.

## Table of Contents

1. [Environment Variables Reference](#environment-variables-reference)
   1. [Table of Contents](#table-of-contents)
   2. [Platform Detection Variables](#platform-detection-variables)
      1. [Detection Logic](#detection-logic)
   3. [XDG Base Directory Variables](#xdg-base-directory-variables)
   4. [Editor Configuration](#editor-configuration)
   5. [Pager Configuration](#pager-configuration)
   6. [Terminal Configuration](#terminal-configuration)
      1. [Terminal Type Handling](#terminal-type-handling)
   7. [History Configuration](#history-configuration)
      1. [Zsh History](#zsh-history)
      2. [Bash History](#bash-history)
      3. [Common History Variables](#common-history-variables)
   8. [Language and Locale](#language-and-locale)
   9. [Development Environment](#development-environment)
      1. [Go](#go)
      2. [Rust](#rust)
      3. [Python](#python)
      4. [Node.js](#nodejs)
      5. [Deno](#deno)
      6. [Bun](#bun)
   10. [Cloud Provider Configuration](#cloud-provider-configuration)
       1. [AWS](#aws)
       2. [Google Cloud](#google-cloud)
       3. [Azure](#azure)
   11. [Kubernetes Configuration](#kubernetes-configuration)
   12. [Docker Configuration](#docker-configuration)
   13. [Build Tools Configuration](#build-tools-configuration)
   14. [Security and Privacy](#security-and-privacy)
       1. [GPG and SSH](#gpg-and-ssh)
       2. [Telemetry Opt-Out](#telemetry-opt-out)
   15. [Tool-Specific Configuration](#tool-specific-configuration)
       1. [FZF (Fuzzy Finder)](#fzf-fuzzy-finder)
       2. [Ripgrep](#ripgrep)
       3. [Bat (Syntax Highlighting)](#bat-syntax-highlighting)
       4. [Eza (Enhanced ls)](#eza-enhanced-ls)
       5. [Delta (Git Diff)](#delta-git-diff)
   16. [WezTerm Configuration](#wezterm-configuration)
       1. [Theme](#theme)
       2. [Wallpaper Settings](#wallpaper-settings)
   17. [Ansible Configuration](#ansible-configuration)
   18. [Terraform Configuration](#terraform-configuration)
   19. [Platform-Specific Variables](#platform-specific-variables)
       1. [macOS-Specific](#macos-specific)
       2. [Debian/Ubuntu-Specific](#debianubuntu-specific)
       3. [RedHat/Fedora-Specific](#redhatfedora-specific)
       4. [Arch Linux-Specific](#arch-linux-specific)
       5. [WSL-Specific](#wsl-specific)
   20. [Bash Completion](#bash-completion)
   21. [How Variables Are Loaded](#how-variables-are-loaded)
       1. [Loading Example](#loading-example)
   22. [Customizing Variables](#customizing-variables)
   23. [Source Files](#source-files)

---

## Platform Detection Variables

These variables are set by `common/platform.sh` and provide information about the current system.

| Variable                 | Description                      | Example Values                                                 | Required |
| ------------------------ | -------------------------------- | -------------------------------------------------------------- | -------- |
| `DOTFILES_OS`            | Detected operating system        | `macos`, `debian`, `redhat`, `arch`, `linux`, `wsl`, `windows` | Auto-set |
| `DOTFILES_ARCH`          | CPU architecture                 | `x86_64`, `arm64`, `arm`                                       | Auto-set |
| `DOTFILES_CONTAINER`     | Container environment            | `docker`, `kubernetes`, `wsl`, `none`                          | Auto-set |
| `DOTFILES_PKG_MGR`       | Primary package manager          | `homebrew`, `apt`, `dnf`, `yum`, `pacman`, `zypper`            | Auto-set |
| `DOTFILES_HOMEBREW_PATH` | Homebrew installation path       | `/opt/homebrew`, `/usr/local`, `/home/linuxbrew/.linuxbrew`    | Auto-set |
| `DOTFILES_IS_SSH`        | Running over SSH                 | `true`, `false`                                                | Auto-set |
| `DOTFILES_USE_OSC52`     | Should use OSC 52 clipboard      | `true`, `false`                                                | Auto-set |
| `DOTFILES_COPY_CMD`      | Platform clipboard copy command  | `pbcopy`, `xclip -selection clipboard`, `clip.exe`             | Auto-set |
| `DOTFILES_PASTE_CMD`     | Platform clipboard paste command | `pbpaste`, `xclip -selection clipboard -o`                     | Auto-set |
| `DOTFILES_OPEN_CMD`      | Platform file/URL open command   | `open`, `xdg-open`, `explorer.exe`                             | Auto-set |

### Detection Logic

**Operating System Detection:**

```bash
# macOS
uname -s == "Darwin"    -> macos

# Linux variants (from /etc/os-release ID field)
ubuntu, debian          -> debian
fedora, rhel, centos    -> redhat
arch                    -> arch
(with WSL detection)    -> wsl
(other)                 -> linux
```

**Architecture Detection:**

```bash
uname -m:
x86_64, amd64           -> x86_64
aarch64, arm64          -> arm64
armv7l, armv6l          -> arm
```

**Container Detection:**

```bash
/.dockerenv exists      -> docker
KUBERNETES_SERVICE_HOST -> kubernetes
WSL_DISTRO_NAME set     -> wsl
(none)                  -> none
```

---

## XDG Base Directory Variables

Standard directories following the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html).

| Variable          | Default              | Description              |
| ----------------- | -------------------- | ------------------------ |
| `XDG_CONFIG_HOME` | `$HOME/.config`      | User configuration files |
| `XDG_DATA_HOME`   | `$HOME/.local/share` | User data files          |
| `XDG_CACHE_HOME`  | `$HOME/.cache`       | User cache files         |
| `XDG_STATE_HOME`  | `$HOME/.local/state` | User state files         |

**Note:** These directories are created automatically if they don't exist.

---

## Editor Configuration

Variables controlling the default text editor.

| Variable     | Default   | Detection Priority     | Description            |
| ------------ | --------- | ---------------------- | ---------------------- |
| `EDITOR`     | `nvim`    | nvim > vim > nano > vi | Default editor         |
| `VISUAL`     | `nvim`    | nvim > vim > nano > vi | Visual editor          |
| `GIT_EDITOR` | `$EDITOR` | Same as EDITOR         | Editor for Git commits |

**Detection Logic:**

```bash
if has_command nvim; then
    EDITOR="nvim"
elif has_command vim; then
    EDITOR="vim"
elif has_command nano; then
    EDITOR="nano"
else
    EDITOR="vi"
fi
```

---

## Pager Configuration

Variables controlling output pagination.

| Variable   | Default                              | Fallback            | Description    |
| ---------- | ------------------------------------ | ------------------- | -------------- |
| `PAGER`    | `bat --paging=always`                | `less` -> `more`    | Default pager  |
| `MANPAGER` | `bat --language=man --paging=always` | `less -X` -> `more` | Man page pager |
| `LESS`     | `-R -F -X`                           | N/A                 | Less options   |

**Less Options Explained:**

- `-R`: Allow raw control characters (for colors)
- `-F`: Quit if entire file fits on one screen
- `-X`: Don't clear screen after quitting

---

## Terminal Configuration

Variables for terminal behavior and appearance.

| Variable      | Default                  | Description                                    |
| ------------- | ------------------------ | ---------------------------------------------- |
| `TERM`        | `xterm-256color`         | Terminal type (with Ghostty fallback handling) |
| `CLICOLOR`    | `1`                      | Enable CLI colors                              |
| `FORCE_COLOR` | `1`                      | Force color output                             |
| `LSCOLORS`    | `ExFxCxDxBxegedabagacad` | macOS ls colors                                |
| `LS_COLORS`   | `di=1;34:ln=1;35:...`    | Linux ls colors                                |
| `GREP_COLOR`  | `1;33`                   | Grep match color (yellow)                      |
| `GREP_COLORS` | `mt=1;33`                | Grep match text color                          |

### Terminal Type Handling

The dotfiles handle modern terminals like Ghostty:

```bash
if [ "$TERM" = "xterm-ghostty" ]; then
    # Fallback if terminfo not available
    if ! infocmp xterm-ghostty >/dev/null 2>&1; then
        export TERM="xterm-256color"
    fi
fi
```

---

## History Configuration

Variables controlling shell history behavior.

### Zsh History

| Variable   | Default              | Description                       |
| ---------- | -------------------- | --------------------------------- |
| `HISTFILE` | `$HOME/.zsh_history` | History file location             |
| `HISTSIZE` | `10000000`           | Maximum history entries in memory |
| `SAVEHIST` | `10000000`           | Maximum history entries saved     |

### Bash History

| Variable       | Default                | Description                       |
| -------------- | ---------------------- | --------------------------------- |
| `HISTFILE`     | `$HOME/.bash_history`  | History file location             |
| `HISTSIZE`     | `100000`               | Maximum history entries in memory |
| `HISTFILESIZE` | `100000`               | Maximum history entries saved     |
| `HISTCONTROL`  | `ignoreboth:erasedups` | History control options           |

### Common History Variables

| Variable         | Default                                                               | Description                      |
| ---------------- | --------------------------------------------------------------------- | -------------------------------- |
| `HISTTIMEFORMAT` | `%F %T`                                                               | Timestamp format for history     |
| `HISTIGNORE`     | `ls:ls *:cd:cd -:cd ~:pwd:exit:date:history:clear:cls:* --help:bg:fg` | Commands to exclude from history |

---

## Language and Locale

Variables controlling language and character encoding.

| Variable   | Default       | Description                  |
| ---------- | ------------- | ---------------------------- |
| `LANG`     | `en_US.UTF-8` | Primary language             |
| `LC_ALL`   | `en_US.UTF-8` | Override all locale settings |
| `LC_CTYPE` | `en_US.UTF-8` | Character classification     |

---

## Development Environment

Variables for programming language environments.

### Go

| Variable      | Default                           | Description         |
| ------------- | --------------------------------- | ------------------- |
| `GOPATH`      | `$HOME/go`                        | Go workspace        |
| `GOBIN`       | `$GOPATH/bin`                     | Go binary directory |
| `GO111MODULE` | `on`                              | Enable Go modules   |
| `GOPROXY`     | `https://proxy.golang.org,direct` | Module proxy        |
| `GOSUMDB`     | `sum.golang.org`                  | Checksum database   |

### Rust

| Variable      | Default         | Description           |
| ------------- | --------------- | --------------------- |
| `CARGO_HOME`  | `$HOME/.cargo`  | Cargo home directory  |
| `RUSTUP_HOME` | `$HOME/.rustup` | Rustup home directory |

### Python

| Variable                     | Default                   | Description                            |
| ---------------------------- | ------------------------- | -------------------------------------- |
| `PYTHONSTARTUP`              | `$HOME/.pythonrc`         | Python startup file                    |
| `PYTHONUSERBASE`             | `$HOME/.local`            | User base directory                    |
| `PIP_USER`                   | `false`                   | Install packages to user directory     |
| `PIPX_HOME`                  | `$HOME/.local/share/pipx` | Pipx home directory                    |
| `PIPX_BIN_DIR`               | `$HOME/.local/bin`        | Pipx binary directory                  |
| `WORKON_HOME`                | `$HOME/.virtualenvs`      | Virtualenv home directory              |
| `VIRTUALENVWRAPPER_PYTHON`   | `$(command -v python3)`   | Python for virtualenvwrapper           |
| `VIRTUAL_ENV_DISABLE_PROMPT` | `1`                       | Disable virtualenv prompt modification |

### Node.js

| Variable                | Default                            | Description              |
| ----------------------- | ---------------------------------- | ------------------------ |
| `NODE_REPL_HISTORY`     | `$XDG_DATA_HOME/node_repl_history` | Node REPL history file   |
| `NPM_CONFIG_USERCONFIG` | `$XDG_CONFIG_HOME/npm/npmrc`       | NPM config file location |

**Note:** `NPM_CONFIG_PREFIX` is set conditionally in `paths.sh` to avoid conflicts with NVM.

### Deno

| Variable       | Default       | Description                 |
| -------------- | ------------- | --------------------------- |
| `DENO_INSTALL` | `$HOME/.deno` | Deno installation directory |

### Bun

| Variable      | Default      | Description                |
| ------------- | ------------ | -------------------------- |
| `BUN_INSTALL` | `$HOME/.bun` | Bun installation directory |

---

## Cloud Provider Configuration

Variables for cloud provider CLIs.

### AWS

| Variable              | Default      | Description           |
| --------------------- | ------------ | --------------------- |
| `AWS_DEFAULT_OUTPUT`  | `table`      | Default output format |
| `AWS_CLI_AUTO_PROMPT` | `on-partial` | Auto-prompt mode      |
| `AWS_PAGER`           | (empty)      | Disable AWS CLI pager |
| `AWS_SDK_LOAD_CONFIG` | `1`          | Load config from file |

### Google Cloud

| Variable          | Default                 | Description       |
| ----------------- | ----------------------- | ----------------- |
| `CLOUDSDK_PYTHON` | `$(command -v python3)` | Python for gcloud |

### Azure

| Variable           | Default                  | Description                |
| ------------------ | ------------------------ | -------------------------- |
| `AZURE_CONFIG_DIR` | `$XDG_CONFIG_HOME/azure` | Azure CLI config directory |

---

## Kubernetes Configuration

Variables for Kubernetes tools.

| Variable           | Default                 | Description              |
| ------------------ | ----------------------- | ------------------------ |
| `KUBECONFIG`       | `$HOME/.kube/config`    | Kubeconfig file path     |
| `KUBE_EDITOR`      | `$EDITOR`               | Editor for kubectl edit  |
| `KREW_ROOT`        | `$HOME/.krew`           | Krew plugin manager root |
| `HELM_CACHE_HOME`  | `$XDG_CACHE_HOME/helm`  | Helm cache directory     |
| `HELM_CONFIG_HOME` | `$XDG_CONFIG_HOME/helm` | Helm config directory    |
| `HELM_DATA_HOME`   | `$XDG_DATA_HOME/helm`   | Helm data directory      |

---

## Docker Configuration

Variables for Docker and container tools.

| Variable                   | Default | Description               |
| -------------------------- | ------- | ------------------------- |
| `DOCKER_BUILDKIT`          | `1`     | Enable BuildKit           |
| `COMPOSE_DOCKER_CLI_BUILD` | `1`     | Use Docker CLI for builds |
| `DOCKER_SCAN_SUGGEST`      | `false` | Disable scan suggestions  |

---

## Build Tools Configuration

Variables for build systems.

| Variable             | Default                                  | Description                             |
| -------------------- | ---------------------------------------- | --------------------------------------- |
| `MAKEFLAGS`          | `-j$(nproc)` or `-j$(sysctl -n hw.ncpu)` | Parallel make jobs                      |
| `CMAKE_GENERATOR`    | `Ninja`                                  | CMake generator                         |
| `CMAKE_MAKE_PROGRAM` | `$(command -v ninja)`                    | CMake make program (if ninja available) |

---

## Security and Privacy

Variables for security tools and privacy settings.

### GPG and SSH

| Variable        | Default  | Description      |
| --------------- | -------- | ---------------- |
| `GPG_TTY`       | `$(tty)` | TTY for GPG      |
| `SSH_AUTH_SOCK` | Varies   | SSH agent socket |

### Telemetry Opt-Out

The following variables disable telemetry for various tools:

| Variable                        | Value           | Tool                 |
| ------------------------------- | --------------- | -------------------- |
| `DOTNET_CLI_TELEMETRY_OPTOUT`   | `1`             | .NET CLI             |
| `GATSBY_TELEMETRY_DISABLED`     | `1`             | Gatsby               |
| `NEXT_TELEMETRY_DISABLED`       | `1`             | Next.js              |
| `DO_NOT_TRACK`                  | `1`             | Universal opt-out    |
| `HOMEBREW_NO_ANALYTICS`         | `1`             | Homebrew             |
| `HOMEBREW_NO_AUTO_UPDATE`       | `1`             | Homebrew auto-update |
| `HOMEBREW_NO_INSECURE_REDIRECT` | `1`             | Homebrew security    |
| `HOMEBREW_CASK_OPTS`            | `--require-sha` | Cask verification    |

---

## Tool-Specific Configuration

Variables for specific CLI tools.

### FZF (Fuzzy Finder)

| Variable              | Default                                                           | Description            |
| --------------------- | ----------------------------------------------------------------- | ---------------------- |
| `FZF_DEFAULT_OPTS`    | `--height 40% --layout=reverse --border --inline-info`            | Default FZF options    |
| `FZF_DEFAULT_COMMAND` | `fd --type f --hidden --follow --exclude .git` (fallback to find) | Default file search    |
| `FZF_CTRL_T_COMMAND`  | Same as `FZF_DEFAULT_COMMAND`                                     | Ctrl+T file search     |
| `FZF_ALT_C_COMMAND`   | `fd --type d --hidden --follow --exclude .git` (fallback to find) | Alt+C directory search |

### Ripgrep

| Variable              | Default                           | Description         |
| --------------------- | --------------------------------- | ------------------- |
| `RIPGREP_CONFIG_PATH` | `$XDG_CONFIG_HOME/ripgrep/config` | Ripgrep config file |

### Bat (Syntax Highlighting)

| Variable    | Default          | Description     |
| ----------- | ---------------- | --------------- |
| `BAT_THEME` | `Sublime Snazzy` | Bat color theme |
| `BAT_PAGER` | `less -RF`       | Pager for bat   |

### Eza (Enhanced ls)

| Variable     | Default           | Description             |
| ------------ | ----------------- | ----------------------- |
| `EZA_COLORS` | `da=1;34:gm=1;34` | Eza color configuration |

### Delta (Git Diff)

| Variable      | Default    | Description     |
| ------------- | ---------- | --------------- |
| `DELTA_PAGER` | `less -RF` | Pager for delta |

---

## WezTerm Configuration

Variables for WezTerm terminal emulator.

### Theme

| Variable             | Default            | Description          |
| -------------------- | ------------------ | -------------------- |
| `WEZTERM_THEME`      | `Catppuccin Mocha` | WezTerm color theme  |
| `WEZTERM_AUTO_THEME` | `false`            | Auto theme switching |

### Wallpaper Settings

| Variable                            | Default    | Description                    |
| ----------------------------------- | ---------- | ------------------------------ |
| `WEZTERM_WALLPAPER`                 | `true`     | Enable wallpapers              |
| `WEZTERM_WALLPAPER_CATEGORY`        | `Fiction`  | Wallpaper category             |
| `WEZTERM_WALLPAPER_OPACITY`         | `0.8`      | Wallpaper opacity              |
| `WEZTERM_WALLPAPER_BRIGHTNESS`      | `0.1`      | Wallpaper brightness           |
| `WEZTERM_WALLPAPER_ROTATE`          | `false`    | Rotate wallpapers              |
| `WEZTERM_WALLPAPER_TIME_BASED`      | `true`     | Time-based wallpaper selection |
| `WEZTERM_WALLPAPER_TIME_BRIGHTNESS` | `true`     | Time-based brightness          |
| `WEZTERM_WALLPAPER_H_ALIGN`         | `Center`   | Horizontal alignment           |
| `WEZTERM_WALLPAPER_V_ALIGN`         | `Middle`   | Vertical alignment             |
| `WEZTERM_WALLPAPER_REPEAT`          | `NoRepeat` | Wallpaper repeat mode          |

---

## Ansible Configuration

Variables for Ansible automation.

| Variable           | Default                                | Description         |
| ------------------ | -------------------------------------- | ------------------- |
| `ANSIBLE_CONFIG`   | `$XDG_CONFIG_HOME/ansible/ansible.cfg` | Ansible config file |
| `ANSIBLE_LOG_PATH` | `$HOME/.ansible/ansible.log`           | Ansible log file    |
| `ANSIBLE_DEBUG`    | `False`                                | Debug mode          |

---

## Terraform Configuration

Variables for Terraform infrastructure as code.

| Variable              | Default                                  | Description            |
| --------------------- | ---------------------------------------- | ---------------------- |
| `TF_CLI_CONFIG_FILE`  | `$XDG_CONFIG_HOME/terraform/terraformrc` | Terraform config file  |
| `TF_PLUGIN_CACHE_DIR` | `$XDG_CACHE_HOME/terraform`              | Plugin cache directory |

---

## Platform-Specific Variables

Variables that are set differently based on the operating system.

### macOS-Specific

| Variable              | Value/Default                                           | Description            |
| --------------------- | ------------------------------------------------------- | ---------------------- |
| `HOMEBREW_PREFIX`     | `$DOTFILES_HOMEBREW_PATH`                               | Homebrew prefix        |
| `HOMEBREW_CELLAR`     | `$HOMEBREW_PREFIX/Cellar`                               | Homebrew Cellar        |
| `HOMEBREW_REPOSITORY` | `$HOMEBREW_PREFIX`                                      | Homebrew repository    |
| `JAVA_HOME`           | Auto-detected from `/Library/Java/JavaVirtualMachines/` | Java home              |
| `LDFLAGS`             | `-L$DOTFILES_HOMEBREW_PATH/lib`                         | Linker flags           |
| `CPPFLAGS`            | `-I$DOTFILES_HOMEBREW_PATH/include`                     | Compiler flags         |
| `PKG_CONFIG_PATH`     | `$DOTFILES_HOMEBREW_PATH/lib/pkgconfig`                 | pkg-config search path |
| `HOMEBREW_CASK_OPTS`  | `--require-sha --no-quarantine`                         | Cask options           |

### Debian/Ubuntu-Specific

| Variable          | Value/Default               | Description           |
| ----------------- | --------------------------- | --------------------- |
| `DEBIAN_FRONTEND` | `noninteractive`            | Non-interactive apt   |
| `JAVA_HOME`       | `/usr/lib/jvm/default-java` | Java home (if exists) |

### RedHat/Fedora-Specific

| Variable    | Value/Default       | Description           |
| ----------- | ------------------- | --------------------- |
| `JAVA_HOME` | `/usr/lib/jvm/java` | Java home (if exists) |

### Arch Linux-Specific

| Variable    | Value/Default          | Description           |
| ----------- | ---------------------- | --------------------- |
| `JAVA_HOME` | `/usr/lib/jvm/default` | Java home (if exists) |

### WSL-Specific

| Variable          | Value/Default               | Description                     |
| ----------------- | --------------------------- | ------------------------------- |
| `WSL_DISTRO_NAME` | Auto-detected               | WSL distribution name           |
| `WSLENV`          | `USERPROFILE/up:USERNAME/u` | Windows environment passthrough |
| `TZ`              | From `/etc/timezone`        | Timezone (fixes clock drift)    |

---

## Bash Completion

| Variable                   | Default                            | Description               |
| -------------------------- | ---------------------------------- | ------------------------- |
| `BASH_COMPLETION_USER_DIR` | `$XDG_CONFIG_HOME/bash-completion` | User completion directory |

---

## How Variables Are Loaded

The environment variables are loaded in this order:

1. **`common/platform.sh`** - Platform detection and `DOTFILES_*` variables
2. **`common/exports.sh`** - Main environment configuration
3. **`common/paths.sh`** - PATH configuration
4. **`common/$DOTFILES_OS/exports.sh`** - Platform-specific exports (if exists)

### Loading Example

```bash
# In your shell profile (bashrc/zshrc)
source "$HOME/.config/dotfiles/common/platform.sh"
source "$HOME/.config/dotfiles/common/exports.sh"
source "$HOME/.config/dotfiles/common/paths.sh"
```

---

## Customizing Variables

To override any of these variables, you can:

1. **Set before sourcing**: Set the variable before sourcing the dotfiles

   ```bash
   export EDITOR="code"
   source ~/.config/dotfiles/common/exports.sh
   ```

2. **Set in local config**: Add to `~/.bashrc.local` or `~/.zshrc.local`

   ```bash
   export EDITOR="code"
   export FZF_DEFAULT_OPTS="--height 80%"
   ```

3. **Use direnv**: Create a `.envrc` file in your project

   ```bash
   export EDITOR="vim"
   export AWS_PROFILE="development"
   ```

---

## Source Files

- **Platform Detection**: `/Users/yandy/.config/dotfiles/common/platform.sh`
- **Environment Variables**: `/Users/yandy/.config/dotfiles/common/exports.sh`
- **PATH Configuration**: `/Users/yandy/.config/dotfiles/common/paths.sh`
