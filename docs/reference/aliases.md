# Alias Reference

This document provides a complete reference for all shell aliases defined in the dotfiles repository. Aliases are organized by category and platform availability.

## Table of Contents

- [Navigation Aliases](#navigation-aliases)
- [File Operations](#file-operations)
- [Text Processing](#text-processing)
- [Git Aliases](#git-aliases)
- [Branchlet Worktree Management](#branchlet-worktree-management)
- [Docker Aliases](#docker-aliases)
- [Kubernetes Aliases](#kubernetes-aliases)
- [Cloud Provider and Terraform](#cloud-provider-and-terraform)
- [Mise Version Manager](#mise-version-manager)
- [Lazygit Aliases](#lazygit-aliases)
- [Lazydocker Aliases](#lazydocker-aliases)
- [Yazi File Manager](#yazi-file-manager)
- [Zoxide Aliases](#zoxide-aliases)
- [Network Aliases](#network-aliases)
- [System Aliases](#system-aliases)
- [Development Aliases](#development-aliases)
- [AI / Fabric Aliases](#ai--fabric-aliases)
- [Tmux Aliases](#tmux-aliases)
- [Direnv Aliases](#direnv-aliases)
- [Clipboard Aliases](#clipboard-aliases)
- [Utility Aliases](#utility-aliases)
- [Fun Aliases](#fun-aliases)
- [macOS-Specific Aliases](#macos-specific-aliases)
- [Linux-Specific Aliases](#linux-specific-aliases)

---

## Navigation Aliases

Quick directory navigation shortcuts available on all platforms.

| Alias   | Command          | Description                         |
| ------- | ---------------- | ----------------------------------- |
| `..`    | `cd ..`          | Go up one directory                 |
| `cd..`  | `cd ..`          | Go up one directory (typo-friendly) |
| `...`   | `cd ../..`       | Go up two directories               |
| `....`  | `cd ../../..`    | Go up three directories             |
| `.....` | `cd ../../../..` | Go up four directories              |
| `~`     | `cd ~`           | Go to home directory                |
| `-`     | `cd -`           | Go to previous directory            |
| `h`     | `cd ~`           | Go to home directory                |
| `d`     | `cd ~/Desktop`   | Go to Desktop                       |
| `dl`    | `cd ~/Downloads` | Go to Downloads                     |
| `doc`   | `cd ~/Documents` | Go to Documents                     |
| `dev`   | `cd ~/dev`       | Go to dev directory                 |
| `proj`  | `cd ~/Projects`  | Go to Projects directory            |

---

## File Operations

File listing and manipulation aliases with modern tool fallbacks.

### Enhanced ls Aliases (with eza)

When `eza` is installed, these aliases provide enhanced file listing with icons and colors.

| Alias  | Command                                                                            | Description                      |
| ------ | ---------------------------------------------------------------------------------- | -------------------------------- |
| `ls`   | `eza -lg --group-directories-first --color always --icons always`                  | List files with details          |
| `la`   | `eza -lg --all --group-directories-first --color always --icons always`            | List all files including hidden  |
| `ll`   | `eza -alg --group-directories-first --color always --icons always`                 | Long list all files              |
| `ld`   | `eza -lDg --color always --icons always`                                           | List directories only            |
| `lf`   | `eza -lfg --color always --icons always`                                           | List files only                  |
| `lh`   | `eza -dlg .* --color always --icons always --group-directories-first`              | List hidden files only           |
| `lt`   | `eza -alg --color always --icons always --group-directories-first --sort=modified` | List sorted by modification time |
| `tree` | `eza --tree --color always --icons always`                                         | Tree view of directory           |

**Fallback**: When `eza` is not available, standard `ls` commands are used with platform-appropriate color flags (`-G` on macOS, `--color=auto` on Linux).

### File Operations

| Alias   | Command                      | Description                  |
| ------- | ---------------------------- | ---------------------------- |
| `mv`    | `mv -v`                      | Move with verbose output     |
| `cp`    | `cp -v`                      | Copy with verbose output     |
| `rm`    | `rm -v`                      | Remove with verbose output   |
| `rmd`   | `rm -rf`                     | Remove directory recursively |
| `rmdir` | `rm -rf`                     | Remove directory recursively |
| `fs`    | `stat -f '%z bytes'` (macOS) | Get file size in bytes       |
| `du1`   | `du -h -d 1`                 | Disk usage one level deep    |
| `dus`   | `du -sh`                     | Summarized disk usage        |

---

## Text Processing

Text search and manipulation aliases.

### Grep Aliases

| Alias   | Command                    | Description                    | Fallback                 |
| ------- | -------------------------- | ------------------------------ | ------------------------ |
| `rgr`   | `rg --hidden --smart-case` | Ripgrep with smart case        | Only when `rg` available |
| `grep`  | `grep --color=auto`        | Colored grep output            | Default                  |
| `fgrep` | `fgrep --color=auto`       | Fixed string grep with color   | Default                  |
| `egrep` | `egrep --color=auto`       | Extended regex grep with color | Default                  |

### Cat Aliases (with bat)

| Alias  | Command                          | Description               | Fallback                                      |
| ------ | -------------------------------- | ------------------------- | --------------------------------------------- |
| `bat`  | `bat` or `batcat` `--paging=never`  | Syntax highlighted cat    | When `bat` or `batcat` is on PATH (`bat` wins) |
| `less` | `bat` or `batcat` `--paging=always` | Paged syntax highlighting | When `bat` or `batcat` is on PATH (`bat` wins) |

### fd

| Alias | Command  | Description                         | Condition                                        |
| ----- | -------- | ----------------------------------- | ------------------------------------------------ |
| `fd`  | `fdfind` | `fd` name for Debian/Ubuntu's binary | Only when `fd` is absent and `fdfind` is on PATH |

### Text Utilities

| Alias      | Command                            | Description                |
| ---------- | ---------------------------------- | -------------------------- |
| `rot13`    | `tr a-zA-Z n-za-mN-ZA-M`           | ROT13 encode/decode        |
| `trimcopy` | `tr -d '\n' \| $DOTFILES_COPY_CMD` | Copy text without newlines |

---

## Git Aliases

Git workflow shortcuts for common operations.

### Basic Git Operations

| Alias | Command                                      | Description                     |
| ----- | -------------------------------------------- | ------------------------------- |
| `g`   | `git`                                        | Git shortcut                    |
| `gs`  | `git status`                                 | Show working tree status        |
| `ga`  | `git add`                                    | Add files to staging            |
| `gc`  | `git commit`                                 | Commit changes                  |
| `gp`  | `git push`                                   | Push to remote                  |
| `gpl` | `git pull`                                   | Pull from remote                |
| `gd`  | `git diff`                                   | Show differences                |
| `gl`  | `git log --oneline --all --graph --decorate` | Visual log with graph           |
| `glo` | `git log --oneline`                          | Condensed log                   |
| `gb`  | `git branch`                                 | List branches                   |
| `gco` | `git checkout`                               | Switch branches                 |
| `gcb` | `git checkout -b`                            | Create and switch to new branch |
| `gm`  | `git merge`                                  | Merge branches                  |

### Advanced Git Operations

| Alias      | Command                                 | Description                |
| ---------- | --------------------------------------- | -------------------------- |
| `gst`      | `git stash`                             | Stash changes              |
| `gsp`      | `git stash pop`                         | Apply and remove stash     |
| `gr`       | `git remote -v`                         | List remotes               |
| `gf`       | `git fetch`                             | Fetch from remote          |
| `gt`       | `git tag`                               | List tags                  |
| `grs`      | `git reset`                             | Reset HEAD                 |
| `grh`      | `git reset --hard`                      | Hard reset HEAD            |
| `gcp`      | `git cherry-pick`                       | Cherry-pick commit         |
| `groot`    | `cd "$(git rev-parse --show-toplevel)"` | Go to repository root      |
| `undopush` | `git push -f origin HEAD^:master`       | Undo last push (dangerous) |

---

## Branchlet Worktree Management

Aliases for the branchlet worktree management tool.

| Alias | Command              | Description            |
| ----- | -------------------- | ---------------------- |
| `bl`  | `branchlet`          | Branchlet main command |
| `blc` | `branchlet create`   | Create new worktree    |
| `bll` | `branchlet list`     | List worktrees         |
| `bld` | `branchlet delete`   | Delete worktree        |
| `bls` | `branchlet settings` | Show settings          |
| `wt`  | `branchlet`          | Worktree shortcut      |
| `wtc` | `branchlet create`   | Create worktree        |
| `wtl` | `branchlet list`     | List worktrees         |
| `wtd` | `branchlet delete`   | Delete worktree        |

---

## Docker Aliases

Docker and Docker Compose shortcuts.

### Docker Commands

| Alias  | Command             | Description             |
| ------ | ------------------- | ----------------------- |
| `d`    | `docker`            | Docker shortcut         |
| `dc`   | `docker compose`    | Docker Compose          |
| `dps`  | `docker ps`         | List running containers |
| `dpsa` | `docker ps -a`      | List all containers     |
| `di`   | `docker images`     | List images             |
| `dv`   | `docker volume ls`  | List volumes            |
| `dn`   | `docker network ls` | List networks           |

### Docker Compose Commands

| Alias | Command                  | Description                    |
| ----- | ------------------------ | ------------------------------ |
| `dcu` | `docker compose up -d`   | Start containers in background |
| `dcd` | `docker compose down`    | Stop and remove containers     |
| `dcr` | `docker compose restart` | Restart containers             |
| `dcl` | `docker compose logs -f` | Follow container logs          |
| `dcp` | `docker compose pull`    | Pull images                    |
| `dcb` | `docker compose build`   | Build images                   |

---

## Kubernetes Aliases

kubectl and Kubernetes ecosystem shortcuts. The kubectl and resource aliases
below are defined only when `kubectl` is on PATH.

### kubectl Shortcuts

| Alias  | Command            | Description          |
| ------ | ------------------ | -------------------- |
| `k`    | `kubectl`          | kubectl shortcut     |
| `kg`   | `kubectl get`      | Get resources        |
| `kd`   | `kubectl describe` | Describe resources   |
| `kdel` | `kubectl delete`   | Delete resources     |
| `ka`   | `kubectl apply`    | Apply configuration  |
| `kl`   | `kubectl logs`     | View logs            |
| `kex`  | `kubectl exec`     | Execute in container |

### Resource Shortcuts

| Alias  | Command                   | Description                   |
| ------ | ------------------------- | ----------------------------- |
| `pods` | `kubectl get pods`        | List pods                     |
| `svc`  | `kubectl get services`    | List services                 |
| `ing`  | `kubectl get ingress`     | List ingresses                |
| `ns`   | `kubectl get namespaces`  | List namespaces               |
| `pvc`  | `kubectl get pvc`         | List persistent volume claims |
| `pv`   | `kubectl get pv`          | List persistent volumes       |
| `deps` | `kubectl get deployments` | List deployments              |

### Context Management

| Alias  | Command   | Description               | Condition        |
| ------ | --------- | ------------------------- | ---------------- |
| `kctx` | `kubectx` | Switch Kubernetes context | Requires kubectx |
| `kns`  | `kubens`  | Switch namespace          | Requires kubens  |

---

## Cloud Provider and Terraform

Cloud CLI and Infrastructure as Code aliases.

### Terraform

| Alias | Command              | Description            |
| ----- | -------------------- | ---------------------- |
| `tf`  | `terraform`          | Terraform shortcut     |
| `tfi` | `terraform init`     | Initialize Terraform   |
| `tfp` | `terraform plan`     | Plan changes           |
| `tfa` | `terraform apply`    | Apply changes          |
| `tfd` | `terraform destroy`  | Destroy infrastructure |
| `tfs` | `terraform show`     | Show state             |
| `tfv` | `terraform validate` | Validate configuration |
| `tff` | `terraform fmt`      | Format configuration   |

### AWS CLI

| Alias         | Command                                   | Description               |
| ------------- | ----------------------------------------- | ------------------------- |
| `aws-whoami`  | `aws sts get-caller-identity`             | Show current AWS identity |
| `aws-regions` | `aws ec2 describe-regions --output table` | List AWS regions          |

### 1Password CLI

| Alias | Command                          | Description                       | Condition     |
| ----- | -------------------------------- | --------------------------------- | ------------- |
| `opr` | `op run --no-masking --`         | Run command with secrets          | Requires `op` |
| `opl` | `op run --no-masking`            | Run with secrets (no trailing --) | Requires `op` |
| `ope` | `op run --no-masking --env-file` | Run with env file                 | Requires `op` |

---

## Mise Version Manager

Mise (formerly rtx) version manager aliases.

| Alias   | Command            | Description                 | Condition     |
| ------- | ------------------ | --------------------------- | ------------- |
| `m`     | `mise`             | Mise shortcut               | Requires mise |
| `mi`    | `mise install`     | Install tool version        | Requires mise |
| `mu`    | `mise use`         | Use tool version            | Requires mise |
| `mug`   | `mise use -g`      | Use global version          | Requires mise |
| `mul`   | `mise use`         | Use local version           | Requires mise |
| `ml`    | `mise list`        | List installed versions     | Requires mise |
| `mla`   | `mise list --all`  | List all available versions | Requires mise |
| `mr`    | `mise run`         | Run task                    | Requires mise |
| `mx`    | `mise exec`        | Execute with version        | Requires mise |
| `mup`   | `mise upgrade`     | Upgrade tools               | Requires mise |
| `mself` | `mise self-update` | Update mise itself          | Requires mise |
| `mw`    | `mise which`       | Show tool path              | Requires mise |
| `mp`    | `mise plugins`     | List plugins                | Requires mise |
| `mc`    | `mise current`     | Show current versions       | Requires mise |

---

## Lazygit Aliases

Lazygit terminal UI for git.

| Alias | Command          | Description         | Condition        |
| ----- | ---------------- | ------------------- | ---------------- |
| `lg`  | `lazygit`        | Launch lazygit      | Requires lazygit |
| `lgs` | `lazygit status` | Open at status view | Requires lazygit |
| `lgb` | `lazygit branch` | Open at branch view | Requires lazygit |
| `lgl` | `lazygit log`    | Open at log view    | Requires lazygit |
| `lgc` | `lazygit commit` | Open at commit view | Requires lazygit |
| `lgp` | `lazygit pull`   | Pull changes        | Requires lazygit |
| `lgf` | `lazygit fetch`  | Fetch changes       | Requires lazygit |

---

## Lazydocker Aliases

Terminal UI for Docker. The shortcut is `lzd`, not `ld` (the system linker).

| Alias | Command      | Description       | Condition           |
| ----- | ------------ | ----------------- | ------------------- |
| `lzd` | `lazydocker` | Launch lazydocker | Requires lazydocker |

---

## Yazi File Manager

Yazi terminal file manager aliases.

| Alias | Command | Description               | Condition     |
| ----- | ------- | ------------------------- | ------------- |
| `y`   | `yazi`  | Launch yazi               | Requires yazi |
| `ya`  | `yazi`  | Launch yazi (alternative) | Requires yazi |

**Note**: The `yy` function (which changes directory on exit) is defined in `common/tools.sh`.

---

## Zoxide Aliases

Smart directory jumping with zoxide.

| Alias | Command         | Description                    | Condition       |
| ----- | --------------- | ------------------------------ | --------------- |
| `zi`  | `__zoxide_zi`   | Interactive selection with fzf | Requires zoxide |
| `zq`  | `zoxide query`  | Query the database             | Requires zoxide |
| `za`  | `zoxide add`    | Add directory to database      | Requires zoxide |
| `zr`  | `zoxide remove` | Remove directory from database | Requires zoxide |

**Note**: With `--cmd cd` configuration, regular `cd` is replaced by zoxide.

---

## Network Aliases

Network diagnostics and utilities.

### IP Address Aliases

| Alias           | Command                                              | Description              | Platform |
| --------------- | ---------------------------------------------------- | ------------------------ | -------- |
| `get-public-ip` | `dig +short myip.opendns.com @resolver1.opendns.com` | Get public IP            | All      |
| `myip`          | `get-public-ip`                                      | Get public IP (shortcut) | All      |
| `localip`       | `ipconfig getifaddr en0`                             | Get local IP             | macOS    |
| `localip`       | `hostname -I \| awk '{print $1}'`                    | Get local IP             | Linux    |
| `ips`           | Platform-specific                                    | List all IP addresses    | All      |

### Network Utilities

| Alias      | Command                                     | Description          |
| ---------- | ------------------------------------------- | -------------------- |
| `ports`    | `netstat -tulpn` or `netstat -p tcp -p udp` | Show listening ports |
| `whois`    | `whois -h whois-servers.net`                | WHOIS lookup         |
| `httpdump` | `sudo tcpdump ...`                          | Dump HTTP traffic    |

### HTTP Method Aliases

When `curl` is available, these aliases are created:

| Alias     | Command             | Description          |
| --------- | ------------------- | -------------------- |
| `GET`     | `curl -X 'GET'`     | HTTP GET request     |
| `HEAD`    | `curl -X 'HEAD'`    | HTTP HEAD request    |
| `POST`    | `curl -X 'POST'`    | HTTP POST request    |
| `PUT`     | `curl -X 'PUT'`     | HTTP PUT request     |
| `DELETE`  | `curl -X 'DELETE'`  | HTTP DELETE request  |
| `TRACE`   | `curl -X 'TRACE'`   | HTTP TRACE request   |
| `OPTIONS` | `curl -X 'OPTIONS'` | HTTP OPTIONS request |

---

## System Aliases

System monitoring and management.

### Process Management

| Alias  | Command                             | Description         |
| ------ | ----------------------------------- | ------------------- |
| `psg`  | `ps aux \| grep -v grep \| grep -i` | Search processes    |
| `psa`  | `ps aux`                            | List all processes  |
| `topc` | `top -o cpu`                        | Top by CPU usage    |
| `topm` | `top -o mem`                        | Top by memory usage |

### System Monitoring

| Alias     | Command                      | Description                 | Condition         |
| --------- | ---------------------------- | --------------------------- | ----------------- |
| `top`     | `htop`                       | Enhanced top                | Requires htop     |
| `monitor` | `glances` or `htop` or `top` | System monitor              | Fallback chain    |
| `df`      | `df -h`                      | Disk usage (human-readable) | All               |
| `free`    | `free -h` or `vm_stat`       | Memory usage                | Platform-specific |

---

## Development Aliases

Development environment shortcuts.

### Editors

| Alias  | Command   | Description         |
| ------ | --------- | ------------------- | ------------- |
| `v`    | `$EDITOR` | Open default editor |
| `vi`   | `$EDITOR` | Open default editor |
| `vim`  | `$EDITOR` | Open default editor |
| `nano` | `$EDITOR` | Open default editor |
| `c`    | `code`    | Open VS Code        | Requires code |
| `s`    | `subl`    | Open Sublime Text   | Requires subl |

### Node.js

| Alias | Command         | Description            |
| ----- | --------------- | ---------------------- |
| `npi` | `npm install`   | Install packages       |
| `nps` | `npm start`     | Start application      |
| `npt` | `npm test`      | Run tests              |
| `npb` | `npm run build` | Build project          |
| `npd` | `npm run dev`   | Run development server |

### Python

| Alias  | Command           | Description                |
| ------ | ----------------- | -------------------------- |
| `py`   | `python3`         | Python 3                   |
| `pip`  | `pip3`            | Pip for Python 3           |
| `venv` | `python3 -m venv` | Create virtual environment |

### JSON

| Alias    | Command                | Description       |
| -------- | ---------------------- | ----------------- |
| `jsonpp` | `python3 -m json.tool` | Pretty print JSON |

---

## AI / Fabric Aliases

### Claude

| Alias   | Command                                                                                                                                    | Description                                      |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------ |
| `ccdp`  | `claude --allow-dangerously-skip-permissions --dangerously-skip-permissions --permission-mode bypassPermissions`                            | Start Claude with permission checks bypassed    |
| `cc-rc` | `DO_NOT_TRACK=0 claude --dangerously-skip-permissions --allow-dangerously-skip-permissions rc --permission-mode bypassPermissions`         | Same bypass, plus `rc` and `DO_NOT_TRACK=0`     |

Fabric aliases are loaded when either `fabric` or `fabric-ai` is available. If only `fabric-ai` exists, the shell integration normalizes the `fabric` command name to `fabric-ai` for the aliases and helper functions below.

### Explicit Fabric Aliases

| Alias                    | Command                   | Description                       |
| ------------------------ | ------------------------- | --------------------------------- |
| `fab`                    | `fabric`                  | Fabric shortcut                   |
| `fabric-setup`           | `fabric --setup`          | Run Fabric's interactive setup    |
| `fabric-patterns`        | `fabric --listpatterns`   | List available Fabric patterns    |
| `fpatterns`              | `fabric --listpatterns`   | Short list-patterns alias         |
| `fabric-update-patterns` | `fabric --updatepatterns` | Update local Fabric patterns      |
| `fupatterns`             | `fabric --updatepatterns` | Short update-patterns alias       |
| `fabric-models`          | `fabric --listmodels`     | List configured Fabric models     |
| `fmodels`                | `fabric --listmodels`     | Short list-models alias           |
| `fabric-vendors`         | `fabric --listvendors`    | List Fabric vendors               |
| `fvendors`               | `fabric --listvendors`    | Short list-vendors alias          |
| `fabric-serve`           | `fabric --serve`          | Start Fabric's server mode        |
| `fabric-serve-ollama`    | `fabric --serveOllama`    | Start Fabric's Ollama server mode |

### Fabric Prompt Helpers

| Helper       | Command/function            | Description                                       |
| ------------ | --------------------------- | ------------------------------------------------- |
| `fabric-ask` | `fabric-ask` function       | Send arguments or stdin to Fabric with `--stream` |
| `fabric_ask` | `fabric_ask` function       | Underscore-compatible wrapper for `fabric-ask`    |
| `askai`      | `fabric-ask`                | Ask a quick AI question                           |
| `qq`         | `fabric-ask`                | Short quick-question helper                       |
| `yt`         | `yt [-t\|--timestamps] URL` | Send a YouTube URL to Fabric with `-y`            |

`yt -t URL` adds `--transcript-with-timestamps`.

### Generated Pattern Aliases

When `FABRIC_ENABLE_PATTERN_ALIASES` is not `false`, the shell integration scans `~/.config/fabric/patterns` and creates aliases for valid pattern names. A pattern such as `summarize` becomes an alias equivalent to:

```bash
fabric --pattern summarize
```

Set `FABRIC_ALIAS_PREFIX` to prefix generated aliases and reduce collisions. For example, `FABRIC_ALIAS_PREFIX=f_` makes the `summarize` pattern available as `f_summarize`.

Generated pattern aliases are conservative by default: existing commands, aliases, and functions are skipped. Set `FABRIC_ALIAS_OVERWRITE=true` to allow generated pattern aliases to replace existing names.

Bare `??` is not enabled as the default zsh shortcut because this repository keeps zsh `NOMATCH` behavior enabled, and `??` is parsed as a glob pattern before alias expansion. Use `qq` or `askai` for a zsh-safe quick question helper.

---

## Tmux Aliases

Terminal multiplexer shortcuts.

| Alias | Command               | Description         |
| ----- | --------------------- | ------------------- |
| `tl`  | `tmux list-sessions`  | List sessions       |
| `tks` | `tmux kill-server`    | Kill tmux server    |
| `tmr` | `tmux rename-session` | Rename session      |
| `td`  | `tmux detach`         | Detach from session |

**Note**: `ta` (attach), `ts` (new session), and `tk` (kill session) are functions defined in `common/functions.sh` for proper tab completion support.

---

## Direnv Aliases

Directory-based environment management.

| Alias   | Command         | Description                        |
| ------- | --------------- | ---------------------------------- |
| `da`    | `direnv allow`  | Allow .envrc in current directory  |
| `dr`    | `direnv reload` | Reload direnv                      |
| `ds`    | `direnv status` | Show direnv status                 |
| `dd`    | `direnv deny`   | Deny .envrc in current directory   |
| `de`    | `direnv edit`   | Edit .envrc in current directory   |
| `envrc` | `init-envrc`    | Create .envrc in current directory |

---

## Clipboard Aliases

Clipboard operations with cross-platform support.

| Alias            | Command                       | Description            |
| ---------------- | ----------------------------- | ---------------------- |
| `copy`           | `$DOTFILES_COPY_CMD`          | Copy to clipboard      |
| `paste`          | `$DOTFILES_PASTE_CMD`         | Paste from clipboard   |
| `cb`             | `$DOTFILES_COPY_CMD`          | Quick clipboard copy   |
| `cbp`            | `$DOTFILES_PASTE_CMD`         | Quick clipboard paste  |
| `test-clipboard` | Test clipboard functionality  | Test copy to clipboard |
| `test-osc52`     | Test OSC 52 sequence          | Test SSH clipboard     |
| `clipboard-info` | Show clipboard detection info | Debug clipboard setup  |

---

## Utility Aliases

General utility shortcuts.

### Quick Editing

| Alias     | Command                   | Description     |
| --------- | ------------------------- | --------------- |
| `hosts`   | `sudo $EDITOR /etc/hosts` | Edit hosts file |
| `profile` | `$EDITOR ~/.profile`      | Edit profile    |
| `bashrc`  | `$EDITOR ~/.bashrc`       | Edit bashrc     |
| `zshrc`   | `$EDITOR ~/.zshrc`        | Edit zshrc      |

### Path and History

| Alias       | Command                  | Description        |
| ----------- | ------------------------ | ------------------ |
| `path`      | `echo -e ${PATH//:/\\n}` | Print PATH entries |
| `printpath` | `echo -e ${PATH//:/\\n}` | Print PATH entries |
| `h`         | `history`                | Show history       |
| `hg`        | `history \| grep`        | Search history     |

### Rsync

| Alias        | Command                                       | Description                         |
| ------------ | --------------------------------------------- | ----------------------------------- |
| `rsynca`     | `rsync -ravu`                                 | Rsync with archive, update, verbose |
| `rsync-copy` | `rsync -avh --progress`                       | Copy with progress                  |
| `rsync-move` | `rsync -avh --progress --remove-source-files` | Move with progress                  |

---

## Fun Aliases

Entertainment and miscellaneous aliases.

| Alias     | Command             | Description     | Condition     |
| --------- | ------------------- | --------------- | ------------- |
| `weather` | `curl wttr.in`      | Show weather    | Requires curl |
| `moon`    | `curl wttr.in/Moon` | Show moon phase | Requires curl |

---

## macOS-Specific Aliases

Aliases available only on macOS.

### System Management

| Alias      | Command                                                           | Description       |
| ---------- | ----------------------------------------------------------------- | ----------------- |
| `flushdns` | `sudo dscacheutil -flushcache && sudo killall -HUP mDNSResponder` | Flush DNS cache   |
| `cleardns` | `sudo killall -HUP mDNSResponder`                                 | Clear DNS cache   |
| `cpuinfo`  | `sysctl -n machdep.cpu.brand_string`                              | Show CPU info     |
| `meminfo`  | `vm_stat`                                                         | Show memory stats |

### Application Shortcuts

| Alias       | Command                        | Description        |
| ----------- | ------------------------------ | ------------------ |
| `safari`    | `open -a Safari`               | Open Safari        |
| `chrome`    | `open -a 'Google Chrome'`      | Open Chrome        |
| `firefox`   | `open -a Firefox`              | Open Firefox       |
| `finder`    | `open -a Finder`               | Open Finder        |
| `iterm`     | `open -a iTerm`                | Open iTerm         |
| `code`      | `open -a 'Visual Studio Code'` | Open VS Code       |
| `xcode`     | `open -a Xcode`                | Open Xcode         |
| `simulator` | `open -a Simulator`            | Open iOS Simulator |

### Quick Application Launching

| Alias      | Command              | Description     |
| ---------- | -------------------- | --------------- |
| `calc`     | `open -a Calculator` | Open Calculator |
| `notes`    | `open -a Notes`      | Open Notes      |
| `mail`     | `open -a Mail`       | Open Mail       |
| `calendar` | `open -a Calendar`   | Open Calendar   |
| `contacts` | `open -a Contacts`   | Open Contacts   |
| `maps`     | `open -a Maps`       | Open Maps       |
| `photos`   | `open -a Photos`     | Open Photos     |
| `music`    | `open -a Music`      | Open Music      |
| `spotify`  | `open -a Spotify`    | Open Spotify    |

### Finder and Desktop

| Alias         | Command                                                | Description                 |
| ------------- | ------------------------------------------------------ | --------------------------- |
| `showfiles`   | `defaults write ... && killall Finder`                 | Show hidden files in Finder |
| `hidefiles`   | `defaults write ... && killall Finder`                 | Hide hidden files in Finder |
| `showdesktop` | `defaults write ... && killall Finder`                 | Show desktop icons          |
| `hidedesktop` | `defaults write ... && killall Finder`                 | Hide desktop icons          |
| `emptytrash`  | `sudo rm -rfv /Volumes/*/.Trashes; rm -rfv ~/.Trash/*` | Empty trash                 |
| `quicklook`   | `qlmanage -p`                                          | Quick Look preview          |
| `cleanup`     | `find . -name '*.DS_Store' -type f -ls -delete`        | Delete .DS_Store files      |

### Dock Management

| Alias         | Command                               | Description        |
| ------------- | ------------------------------------- | ------------------ |
| `dockrestart` | `killall Dock`                        | Restart Dock       |
| `dockspacer`  | `defaults write ... && killall Dock`  | Add spacer to Dock |
| `dock2d`      | `defaults write ... && killall Dock`  | Switch to 2D Dock  |
| `dock3d`      | `defaults delete ... && killall Dock` | Switch to 3D Dock  |

### Audio Controls

| Alias      | Command                                        | Description           |
| ---------- | ---------------------------------------------- | --------------------- |
| `mute`     | `osascript -e 'set volume output muted true'`  | Mute audio            |
| `unmute`   | `osascript -e 'set volume output muted false'` | Unmute audio          |
| `maxvol`   | `osascript -e 'set volume 10'`                 | Set volume to maximum |
| `stfu`     | `osascript -e 'set volume output muted true'`  | Mute audio            |
| `pumpitup` | `osascript -e 'set volume 10'`                 | Set volume to maximum |

### WiFi and Networking

| Alias        | Command                                 | Description               |
| ------------ | --------------------------------------- | ------------------------- |
| `airport`    | `/System/Library/Private.../airport`    | Airport utility           |
| `wifiscan`   | `airport -s`                            | Scan for WiFi networks    |
| `wifioff`    | `networksetup -setairportpower en0 off` | Turn WiFi off             |
| `wifion`     | `networksetup -setairportpower en0 on`  | Turn WiFi on              |
| `wifi`       | `networksetup -getairportnetwork en0`   | Show current WiFi network |
| `interfaces` | `networksetup -listallhardwareports`    | List network interfaces   |

### Homebrew

| Alias      | Command                                       | Description             |
| ---------- | --------------------------------------------- | ----------------------- |
| `brewup`   | `brew update && brew upgrade && brew cleanup` | Update Homebrew         |
| `brewinfo` | `brew leaves \| xargs brew desc --eval-all`   | Show installed packages |
| `brewdeps` | `brew deps --tree --installed`                | Show dependency tree    |

### System Utilities

| Alias        | Command                          | Description        |
| ------------ | -------------------------------- | ------------------ |
| `plistbuddy` | `/usr/libexec/PlistBuddy`        | PlistBuddy utility |
| `plist`      | `plutil -p`                      | Pretty print plist |
| `drives`     | `diskutil list`                  | List drives        |
| `lockscreen` | `/System/.../CGSession -suspend` | Lock screen        |
| `filevault`  | `sudo fdesetup status`           | FileVault status   |

### System Monitoring

| Alias    | Command                                 | Description       |
| -------- | --------------------------------------- | ----------------- |
| `cpu`    | `top -l 1 -s 0 \| grep 'CPU usage'`     | Show CPU usage    |
| `memory` | `top -l 1 -s 0 \| grep PhysMem`         | Show memory usage |
| `temp`   | `sudo powermetrics ... \| grep -i temp` | Show temperature  |
| `syslog` | `log show --last 1h`                    | Show system logs  |

### Quick Directory Access

| Alias         | Command                      | Description              |
| ------------- | ---------------------------- | ------------------------ |
| `apps`        | `open /Applications`         | Open Applications folder |
| `library`     | `open ~/Library`             | Open Library folder      |
| `preferences` | `open ~/Library/Preferences` | Open Preferences folder  |
| `desktop`     | `open ~/Desktop`             | Open Desktop folder      |
| `documents`   | `open ~/Documents`           | Open Documents folder    |
| `downloads`   | `open ~/Downloads`           | Open Downloads folder    |

### Time Machine

| Alias       | Command                    | Description         |
| ----------- | -------------------------- | ------------------- |
| `tmstatus`  | `tmutil status`            | Time Machine status |
| `tmstart`   | `sudo tmutil startbackup`  | Start backup        |
| `tmstop`    | `sudo tmutil stopbackup`   | Stop backup         |
| `tmexclude` | `sudo tmutil addexclusion` | Add exclusion       |

### Spotlight

| Alias     | Command                 | Description       |
| --------- | ----------------------- | ----------------- |
| `spotoff` | `sudo mdutil -a -i off` | Disable Spotlight |
| `spoton`  | `sudo mdutil -a -i on`  | Enable Spotlight  |
| `reindex` | `sudo mdutil -E /`      | Reindex Spotlight |

### Quick Look

| Alias       | Command                                     | Description             |
| ----------- | ------------------------------------------- | ----------------------- |
| `qlplugins` | `ls /Library/QuickLook ~/Library/QuickLook` | List Quick Look plugins |
| `qlreload`  | `qlmanage -r`                               | Reload Quick Look       |

### iOS Development

| Alias    | Command                         | Description           |
| -------- | ------------------------------- | --------------------- |
| `iossim` | `xcrun simctl list devices`     | List iOS simulators   |
| `iphone` | `xcrun simctl boot 'iPhone 15'` | Boot iPhone simulator |
| `ipad`   | `xcrun simctl boot 'iPad Air'`  | Boot iPad simulator   |

---

## Linux-Specific Aliases

Aliases available only on Linux systems.

### Package Management (APT - Debian/Ubuntu)

| Alias           | Command                                    | Description             |
| --------------- | ------------------------------------------ | ----------------------- |
| `install`       | `sudo apt install`                         | Install package         |
| `search`        | `apt search`                               | Search packages         |
| `update`        | `sudo apt update`                          | Update package lists    |
| `upgrade`       | `sudo apt update && sudo apt upgrade`      | Upgrade packages        |
| `full-upgrade`  | `sudo apt update && sudo apt full-upgrade` | Full upgrade            |
| `remove`        | `sudo apt remove`                          | Remove package          |
| `purge`         | `sudo apt purge`                           | Purge package           |
| `autoremove`    | `sudo apt autoremove`                      | Remove unused packages  |
| `autoclean`     | `sudo apt autoclean`                       | Clean package cache     |
| `show`          | `apt show`                                 | Show package info       |
| `list`          | `apt list`                                 | List packages           |
| `listinstalled` | `apt list --installed`                     | List installed packages |
| `policy`        | `apt policy`                               | Show package policy     |

### Package Management (DNF - Fedora)

| Alias        | Command               | Description            |
| ------------ | --------------------- | ---------------------- |
| `install`    | `sudo dnf install`    | Install package        |
| `search`     | `dnf search`          | Search packages        |
| `update`     | `sudo dnf upgrade`    | Update packages        |
| `remove`     | `sudo dnf remove`     | Remove package         |
| `autoremove` | `sudo dnf autoremove` | Remove unused packages |
| `clean`      | `sudo dnf clean all`  | Clean cache            |
| `show`       | `dnf info`            | Show package info      |
| `pkghistory` | `dnf history`         | Show DNF history       |

`pkghistory`, not `history`: `history` is a shell builtin, and an alias wins over a
builtin. The yum arm defines the same name as `yum history`.

### Package Management (Pacman - Arch)

| Alias       | Command            | Description              |
| ----------- | ------------------ | ------------------------ |
| `install`   | `sudo pacman -S`   | Install package          |
| `search`    | `pacman -Ss`       | Search packages          |
| `update`    | `sudo pacman -Syu` | Update system            |
| `remove`    | `sudo pacman -R`   | Remove package           |
| `removeall` | `sudo pacman -Rns` | Remove with dependencies |
| `clean`     | `sudo pacman -Sc`  | Clean cache              |
| `show`      | `pacman -Si`       | Show package info        |
| `orphans`   | `pacman -Qtdq`     | List orphan packages     |
| `pac`       | `sudo pacman`      | Pacman shortcut          |

### AUR Helpers (yay)

| Alias    | Command    | Description                 |
| -------- | ---------- | --------------------------- |
| `yayi`   | `yay -S`   | Install package             |
| `yays`   | `yay -Ss`  | Search packages             |
| `yayu`   | `yay -Syu` | Update all packages         |
| `yayr`   | `yay -R`   | Remove package              |
| `yayrns` | `yay -Rns` | Remove with deps and config |
| `yayq`   | `yay -Q`   | Query installed             |
| `yayqi`  | `yay -Qi`  | Query package info          |
| `yayql`  | `yay -Ql`  | List package files          |
| `yayqm`  | `yay -Qm`  | List AUR packages           |
| `yayc`   | `yay -Sc`  | Clean cache                 |
| `yayo`   | `yay -Yc`  | Remove unneeded deps        |

### Flatpak

| Alias      | Command             | Description     | Condition        |
| ---------- | ------------------- | --------------- | ---------------- |
| `finstall` | `flatpak install`   | Install flatpak | Requires flatpak |
| `fsearch`  | `flatpak search`    | Search flatpaks | Requires flatpak |
| `fupdate`  | `flatpak update`    | Update flatpaks | Requires flatpak |
| `fremove`  | `flatpak uninstall` | Remove flatpak  | Requires flatpak |
| `flist`    | `flatpak list`      | List flatpaks   | Requires flatpak |
| `frun`     | `flatpak run`       | Run flatpak     | Requires flatpak |

### Snap

| Alias      | Command             | Description  | Condition     |
| ---------- | ------------------- | ------------ | ------------- |
| `sinstall` | `sudo snap install` | Install snap | Requires snap |
| `ssearch`  | `snap search`       | Search snaps | Requires snap |
| `supdate`  | `sudo snap refresh` | Update snaps | Requires snap |
| `sremove`  | `sudo snap remove`  | Remove snap  | Requires snap |
| `slist`    | `snap list`         | List snaps   | Requires snap |

### SystemD Service Management

| Alias      | Command                               | Description      |
| ---------- | ------------------------------------- | ---------------- |
| `services` | `systemctl list-units --type=service` | List services    |
| `start`    | `sudo systemctl start`                | Start service    |
| `stop`     | `sudo systemctl stop`                 | Stop service     |
| `restart`  | `sudo systemctl restart`              | Restart service  |
| `reload`   | `sudo systemctl reload`               | Reload service   |
| `sysenable`  | `sudo systemctl enable`             | Enable service   |
| `sysdisable` | `sudo systemctl disable`            | Disable service  |
| `status`   | `systemctl status`                    | Service status   |
| `mask`     | `sudo systemctl mask`                 | Mask service     |
| `unmask`   | `sudo systemctl unmask`               | Unmask service   |
| `sc`       | `systemctl`                           | SystemD shortcut |
| `scs`      | `systemctl status`                    | Quick status     |
| `scr`      | `sudo systemctl restart`              | Quick restart    |
| `sce`      | `sudo systemctl enable`               | Quick enable     |
| `scd`      | `sudo systemctl disable`              | Quick disable    |

### System Control

| Alias       | Command                    | Description      |
| ----------- | -------------------------- | ---------------- |
| `reboot`    | `sudo systemctl reboot`    | Reboot system    |
| `poweroff`  | `sudo systemctl poweroff`  | Power off system |
| `syssuspend` | `sudo systemctl suspend`  | Suspend system   |
| `hibernate` | `sudo systemctl hibernate` | Hibernate system |

### Journal Logs

| Alias     | Command                  | Description |
| --------- | ------------------------ | ----------- |
| `logs`    | `sudo journalctl -f`     | Follow logs |
| `syslog`  | `sudo journalctl -u`     | Unit logs   |
| `bootlog` | `sudo journalctl -b`     | Boot logs   |
| `kerror`  | `sudo journalctl -p err` | Error logs  |

### System Information

| Alias      | Command     | Description        |
| ---------- | ----------- | ------------------ |
| `cpuinfo`  | `lscpu`     | CPU information    |
| `meminfo`  | `free -h`   | Memory information |
| `diskinfo` | `df -h`     | Disk information   |
| `uptime`   | `uptime -p` | System uptime      |

### Hardware Information

| Alias   | Command            | Description    |
| ------- | ------------------ | -------------- |
| `lshw`  | `sudo lshw -short` | Hardware list  |
| `lsblk` | `lsblk -f`         | Block devices  |
| `lsusb` | `lsusb`            | USB devices    |
| `lspci` | `lspci`            | PCI devices    |
| `lsmod` | `lsmod`            | Kernel modules |

### Network Management

| Alias        | Command                   | Description        |
| ------------ | ------------------------- | ------------------ |
| `interfaces` | `ip link show`            | Network interfaces |
| `ipaddr`     | `ip addr show`            | IP addresses       |
| `iproute`    | `ip route show`           | Routing table      |
| `ports`      | `ss -tuln`                | Listening ports    |
| `listening`  | `ss -tuln \| grep LISTEN` | Listening services |

### NetworkManager

| Alias            | Command                  | Description        | Condition      |
| ---------------- | ------------------------ | ------------------ | -------------- |
| `wifi`           | `nmcli dev wifi`         | WiFi status        | Requires nmcli |
| `wifilist`       | `nmcli dev wifi list`    | List WiFi networks | Requires nmcli |
| `wificonnect`    | `nmcli dev wifi connect` | Connect to WiFi    | Requires nmcli |
| `wifidisconnect` | `nmcli dev disconnect`   | Disconnect         | Requires nmcli |
| `connections`    | `nmcli connection show`  | Show connections   | Requires nmcli |

### Wi-Fi TUI and launcher (Hyprland / CachyOS)

Dotfiles ship a small helper that prefers **nmtui** and falls back to **nmcli** status plus connect hints:

| What                  | How                                                                                                                                                                                                     |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Script                | `~/.bin/wifi-tui.sh` (symlinked from repo `bin/` via Ansible `utility_dirs`)                                                                                                                            |
| Hypr keybind          | **Super+Shift+O** — opens Alacritty (`--class wifi-tui`) running the Wi-Fi helper                                                                                                                       |
| App launcher (fuzzel) | **Wi-Fi (NetworkManager)** — same Alacritty class; `.desktop` in `launchers/applications/wifi-tui.desktop`, installed to `~/.local/share/applications/` via `launcher_configs` in the symlinks playbook |
| Hypr float            | `hypr/windows.lua` rule **float-wifi-tui**: float, centered, **35%** of monitor width and height                                                                                                        |

After running `ansible-playbook playbooks/symlinks.yml`, run `wifi-tui.sh` or use the keybind/launcher. If `nmtui` is missing, install the split package on Arch (e.g. `network-manager-nmtui`) or use the `nmcli` aliases above.

### File System

| Alias    | Command               | Description          |
| -------- | --------------------- | -------------------- |
| `du1`    | `du -h --max-depth=1` | Disk usage (1 level) |
| `mounts` | `findmnt -D`          | Mount points         |
| `umount` | `sudo umount`         | Unmount              |
| `chown`  | `sudo chown`          | Change owner         |

### Firewall (UFW or firewalld)

The same seven names are defined on whichever backend the host has. `ufw` is
probed first (Debian/Ubuntu); `firewall-cmd` is the fallback (Fedora/RHEL,
openSUSE), matching the backend `dotf sysconfig` configures for each family.

| Alias       | UFW                      | firewalld                           | Description      |
| ----------- | ------------------------ | ----------------------------------- | ---------------- |
| `firewall`  | `sudo ufw status`        | `--state` then `--list-all`         | Firewall status  |
| `fwenable`  | `sudo ufw enable`        | `systemctl enable --now firewalld`  | Enable firewall  |
| `fwdisable` | `sudo ufw disable`       | `systemctl disable --now firewalld` | Disable firewall |
| `fwallow`   | `sudo ufw allow`         | `--permanent --add-service`         | Allow rule       |
| `fwdeny`    | `sudo ufw deny`          | `--permanent --remove-service`      | Deny rule        |
| `fwdelete`  | `sudo ufw delete`        | `--permanent --remove-port`         | Delete rule      |
| `fwreset`   | `sudo ufw --force reset` | `--reload`                          | Reset firewall   |

firewalld has no deny or reset primitive: removing a permanent service or port
is the deny, and reloading discards unsaved runtime changes.

### iptables

| Alias        | Command                     | Description | Condition         |
| ------------ | --------------------------- | ----------- | ----------------- |
| `iptlist`    | `sudo iptables -L -n -v`    | List rules  | Requires iptables |
| `iptblock`   | `sudo iptables -A INPUT -s` | Block IP    | Requires iptables |
| `iptunblock` | `sudo iptables -D INPUT -s` | Unblock IP  | Requires iptables |

### Podman

| Alias   | Command         | Description          | Condition       |
| ------- | --------------- | -------------------- | --------------- |
| `pd`    | `podman`        | Podman shortcut      | Requires podman |
| `pps`   | `podman ps`     | List containers      | Requires podman |
| `pimg`  | `podman images` | List images          | Requires podman |
| `prun`  | `podman run`    | Run container        | Requires podman |
| `pexec` | `podman exec`   | Execute in container | Requires podman |
| `plogs` | `podman logs`   | View logs            | Requires podman |

### Build Tools

| Alias      | Command                                      | Description       |
| ---------- | -------------------------------------------- | ----------------- |
| `gccw`     | `gcc -Wall -Wextra`                          | GCC with warnings |
| `gxxw`     | `g++ -Wall -Wextra`                          | G++ with warnings |
| `make`     | `make -j$(nproc)`                            | Parallel make     |
| `debug`    | `gdb`                                        | GDB debugger      |
| `memcheck` | `valgrind --tool=memcheck --leak-check=full` | Memory check      |

### Archive Operations

| Alias      | Command    | Description      |
| ---------- | ---------- | ---------------- |
| `targz`    | `tar -czf` | Create .tar.gz   |
| `tarbz2`   | `tar -cjf` | Create .tar.bz2  |
| `tarxz`    | `tar -cJf` | Create .tar.xz   |
| `untargz`  | `tar -xzf` | Extract .tar.gz  |
| `untarbz2` | `tar -xjf` | Extract .tar.bz2 |
| `untarxz`  | `tar -xJf` | Extract .tar.xz  |

### Security Tools

| Alias        | Command                         | Description          | Condition        |
| ------------ | ------------------------------- | -------------------- | ---------------- |
| `sslcert`    | `openssl x509 -text -noout -in` | View SSL certificate | Requires openssl |
| `sslkey`     | `openssl rsa -text -noout -in`  | View SSL key         | Requires openssl |
| `sslcheck`   | `openssl s_client -connect`     | Check SSL connection | Requires openssl |
| `gpglist`    | `gpg --list-keys`               | List GPG keys        | Requires gpg     |
| `gpgsecret`  | `gpg --list-secret-keys`        | List secret keys     | Requires gpg     |
| `gpgencrypt` | `gpg --encrypt --armor`         | Encrypt file         | Requires gpg     |
| `gpgdecrypt` | `gpg --decrypt`                 | Decrypt file         | Requires gpg     |

### Performance Monitoring

| Alias       | Command        | Description         | Condition        |
| ----------- | -------------- | ------------------- | ---------------- |
| `top`       | `htop`         | Enhanced top        | Requires htop    |
| `cpu`       | `htop`         | CPU monitor         | Requires htop    |
| `iotop`     | `sudo iotop`   | I/O monitor         | Requires iotop   |
| `bandwidth` | `sudo nethogs` | Network bandwidth   | Requires nethogs |
| `diskusage` | `ncdu`         | Disk usage analyzer | Requires ncdu    |

### Cron

| Alias          | Command      | Description    |
| -------------- | ------------ | -------------- |
| `cronjobs`     | `crontab -l` | List cron jobs |
| `cronjobsedit` | `crontab -e` | Edit cron jobs |

### Kernel

| Alias           | Command            | Description    |
| --------------- | ------------------ | -------------- |
| `kernel`        | `uname -a`         | Kernel info    |
| `kernelversion` | `uname -r`         | Kernel version |
| `modules`       | `lsmod`            | List modules   |
| `loadmodule`    | `sudo modprobe`    | Load module    |
| `unloadmodule`  | `sudo modprobe -r` | Unload module  |

### User Management

| Alias          | Command  | Description   |
| -------------- | -------- | ------------- |
| `groups`       | `groups` | Show groups   |
| `id`           | `id`     | Show user ID  |
| `lastlogin`    | `last`   | Last logins   |
| `currentusers` | `who`    | Current users |

### WSL-Specific

| Alias        | Command          | Description             |
| ------------ | ---------------- | ----------------------- |
| `explorer`   | `explorer.exe`   | Open Windows Explorer   |
| `notepad`    | `notepad.exe`    | Open Notepad            |
| `cmd`        | `cmd.exe`        | Open Command Prompt     |
| `powershell` | `powershell.exe` | Open PowerShell         |
| `winget`     | `winget.exe`     | Windows Package Manager |
| `winpath`    | `wslpath -w`     | Convert to Windows path |
| `linuxpath`  | `wslpath -u`     | Convert to Linux path   |

### Maintenance

| Alias              | Command                                     | Description       |
| ------------------ | ------------------------------------------- | ----------------- |
| `cleanup-logs`     | `sudo journalctl --vacuum-time=30d`         | Clean old logs    |
| `cleanup-temp`     | `sudo rm -rf /tmp/* /var/tmp/*`             | Clean temp files  |

`cleanup-packages` and `updateall` are dispatched per package manager rather than
hardcoded to apt, so they are correct on Fedora, openSUSE and Arch as well:

| Package manager | `cleanup-packages`                                     | `updateall` (base)     |
| --------------- | ------------------------------------------------------ | ---------------------- |
| apt             | `sudo apt autoremove && sudo apt autoclean`            | `sudo apt update && sudo apt upgrade` |
| dnf             | `sudo dnf autoremove && sudo dnf clean all`            | `sudo dnf upgrade`     |
| yum             | `sudo yum autoremove && sudo yum clean all`            | `sudo yum update`      |
| pacman          | `sudo pacman -Sc` plus orphan removal when any exist   | `sudo pacman -Syu` (or `yay -Syu` / `paru -Syu` when an AUR helper is present) |
| zypper          | `sudo zypper clean`                                    | `sudo zypper dup`      |

`updateall` appends `sudo snap refresh` and `flatpak update` only when those
tools are actually installed. On an unrecognised package manager it is left
undefined rather than defined to something wrong.

---

## Source Files

- **Cross-Platform**: `/Users/yandy/.config/dotfiles/common/aliases.sh`
- **macOS-Specific**: `/Users/yandy/.config/dotfiles/common/macos/aliases.sh`
- **Linux-Specific**: `/Users/yandy/.config/dotfiles/common/linux/aliases.sh`
