# Integration Research: arch-os-enhance

## API Endpoints

### Existing Related Endpoints

- None: no HTTP API endpoints are defined for this feature area, and no web server/router stack was found in the repository.
- `bootstrap.sh` is the top-level orchestration entry point for install/setup on CachyOS/Arch.
- `bin/install-arch-pkgs.sh` is the package-install integration surface for `pacman` and `yay`.
- `ansible-playbook playbooks/symlinks.yml` is the deployment entry point for linking config into `~/.config` and `$HOME`.
- `/etc/acpi/events/lid-open` and `/etc/acpi/events/lid-close` are OS event hooks created by `monitor-auto-switch/scripts/install-monitor-auto-switch.sh`.
- `hyprctl` is the runtime control interface used by the ACPI scripts to read monitor/workspace state and dispatch monitor/workspace changes.

### Route Organization

There is no route tree in the web sense. The control flow is OS-driven: `bootstrap.sh` detects `cachyos` as `arch`, installs essential `pacman` packages, delegates full Arch package installation to `bin/install-arch-pkgs.sh`, then runs `playbooks/symlinks.yml`. Runtime monitor behavior is event-driven: ACPI lid events call `/etc/acpi/lid-open.sh` or `/etc/acpi/lid-close.sh`, both source `/etc/acpi/hypr-utils.sh`, and then call `hyprctl`. Hyprland startup/reload also calls `/etc/acpi/check-lid-on-startup.sh` via `exec-once` and `exec`. No HTTP middleware exists; the equivalent gating layers are shell root checks, `ansible_facts` conditionals, and `linux_desktop` in the playbook.

## Database

### Relevant Tables

- None: no database tables are involved in `arch-os-enhance`.
- None: no ORM schema, migrations directory, or application persistence layer was found for this feature.

### Schema Details

There is no database schema to reference. The persistent state for this feature is filesystem and runtime state: symlinks under `~/.config`, ACPI scripts under `/etc/acpi`, ACPI event files under `/etc/acpi/events`, the log file `/var/log/lid-events.log`, the lock file `/run/lid-switch.lock`, and Hyprland compositor state exposed through `hyprctl`.

## External Services

- `pacman`: official Arch/CachyOS package source used by `bootstrap.sh` and `bin/install-arch-pkgs.sh`.
- `yay`: AUR helper used by `bin/install-arch-pkgs.sh`; installed by cloning `https://aur.archlinux.org/yay.git` and building with `makepkg`.
- Ansible: local configuration deployment engine used by `playbooks/symlinks.yml`.
- `acpid`: required for lid-open/lid-close event handling; installer restarts it with `systemctl restart acpid`.
- `systemd`: used for service control and for the Arch-only user `tmux.service`.
- Hyprland runtime tools: `hyprctl` is required by the monitor auto-switch scripts.
- `notify-send`: used by the monitor scripts for desktop notifications.
- `uwsm-app` and `xdg-terminal-exec`: referenced by current Hyprland bindings as launcher/session wrappers.
- Ghostty and WezTerm: existing terminal integrations already exist and are symlinked into `~/.config`.
- Desktop applications launched from bindings include `nautilus`, `typora`, `1password`, browser helpers, `hyprlock`, and generic webapp/TUI launchers.
- No third-party credentials are required for the core OS/Hyprland migration itself. Optional WezTerm SSH usage depends on normal SSH credentials, and app-specific logins are outside this repo.

## Internal Services

- `bootstrap.sh`: main setup orchestrator.
- `bin/install-arch-pkgs.sh`: Arch package installation workflow.
- `playbooks/symlinks.yml`: local deployment/inventory layer for config symlinks.
- `common/platform.sh`: distro and package-manager detection; it already maps `cachyos` to `arch`.
- `common/exports.sh`: terminal-related environment setup, including Ghostty TERM fallback and WezTerm theme env vars.
- `monitor-auto-switch/scripts/install-monitor-auto-switch.sh`: installs ACPI scripts/events, injects Hyprland hooks, and optionally restarts `acpid`.
- `monitor-auto-switch/acpi/hypr-utils.sh`: shared runtime library for monitor detection, workspace reassignment, and `hyprctl` dispatches.
- `monitor-auto-switch/acpi/lid-open.sh`, `lid-close.sh`, and `check-lid-on-startup.sh`: event handlers that call into `hypr-utils.sh`.
- `hypr/hyprland.conf` and sourced `*.conf` files: current Hyprland configuration layer (repo-owned baseline).
- `wezterm/wezterm.lua` and its Lua modules: internal modular terminal config.
- `common/systemd/user/tmux.service`: user service linked only on Archlinux by the playbook.
- Inter-service communication is local only: shell execution, sourced shell libraries, Ansible task execution, filesystem symlinks, environment variables, ACPI event files, and Hyprland socket communication through `hyprctl`.

## Configuration

- `bootstrap.sh`, `bin/install-arch-pkgs.sh`, and `common/platform.sh` are the main install/config entry points; relevant env vars include `DOTFILES_OS`, `DOTFILES_PKG_MGR`, `NON_INTERACTIVE`, and `FORCE_INSTALL`.
- `playbooks/symlinks.yml` already links `wezterm/` and `ghostty/` on Linux, and links `hypr/`, `waybar/`, and `fuzzel/` when `linux_desktop=true`.
- `hypr/hyprland.conf` sources only repo-owned fragments; compatible with stock CachyOS Hyprland.
- `hypr/bindings.conf` uses generic launcher commands (fuzzel, xdg-open, Ghostty, hyprlock).
- `hypr/monitors.conf` and `/etc/acpi/hypr-utils.sh` must stay aligned for monitor names, resolutions, scale, positions, and workspace placement.
- `/etc/acpi/hypr-utils.sh` is the key hardware-specific config surface. Relevant variables are `LAPTOP_MONITOR`, `EXTERNAL_MONITOR`, `EXTERNAL_RESOLUTION`, `LAPTOP_RESOLUTION`, `EXTERNAL_SCALE`, `LAPTOP_SCALE`, `EXTERNAL_POSITION`, `LAPTOP_POSITION_DUAL`, `LAPTOP_POSITION_SOLO`, `EXTERNAL_WS`, and `LAPTOP_WS`.
- `/etc/acpi/events/lid-open` and `/etc/acpi/events/lid-close` are installed config files that bind ACPI events to the lid scripts.
- `ghostty/config` already includes a Hyprland-specific performance tweak: `async-backend = epoll`.
- `wezterm/wezterm.lua` uses `WEZTERM_ENABLE_WAYLAND`, `WEZTERM_SSH_HOST`, and `WEZTERM_SSH_USER`; `common/exports.sh` also exports `WEZTERM_THEME` and `WEZTERM_AUTO_THEME`.
- No Waybar/swaybar config directory exists in the repo today, and no `waybar`, `swaybar`, `wofi`, `fuzzel`, or generic Linux launcher config was found. That means the status bar and launcher layer for CachyOS will need new package entries, new config directories, and new symlink targets.
