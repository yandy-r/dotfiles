# Architecture Research: arch-os-enhance

## System Overview

This repository is a cross-platform dotfiles system centered on shared shell modules in `common/`, a Bash bootstrap entrypoint in `/Users/yandy/.config/dotfiles/bootstrap.sh`, and Ansible-based deployment in `/Users/yandy/.config/dotfiles/playbooks/symlinks.yml`. For CachyOS, platform detection already collapses the distro into the `arch` path, but the Hyprland layer was previously modeled as an external overlay stack; the migration is to a standalone standards-based Hyprland profile.

## Relevant Components

- /Users/yandy/.config/dotfiles/bootstrap.sh: Main entrypoint; parses flags, detects OS/package manager, installs Arch packages, ensures `ansible-playbook` exists, and calls the symlink playbook.
- /Users/yandy/.config/dotfiles/common/platform.sh: Shared platform-detection module; maps `cachyos` to `arch`, so CachyOS follows the Arch Linux execution path.
- /Users/yandy/.config/dotfiles/bin/install-arch-pkgs.sh: Arch package installer used by bootstrap; installs general developer and desktop tools plus Ansible and Hyprland runtime dependencies.
- /Users/yandy/.config/dotfiles/playbooks/symlinks.yml: Deployment layer; defines managed config sets; links `/Users/yandy/.config/dotfiles/hypr` to `~/.config/hypr` when generic Linux desktop is enabled via `linux_desktop=true`.
- /Users/yandy/.config/dotfiles/hypr/hyprland.conf: Hyprland entry config; sources only repo-owned fragments.
- /Users/yandy/.config/dotfiles/hypr/bindings.conf: Keybindings for launcher, terminal, browser, and desktop apps.
- /Users/yandy/.config/dotfiles/hypr/hypridle.conf: Idle/lock behavior using hyprlock and loginctl.
- /Users/yandy/.config/dotfiles/hypr/hyprlock.conf: Lock-screen config with repo-owned theme.
- /Users/yandy/.config/dotfiles/monitor-auto-switch/scripts/install-monitor-auto-switch.sh: Installer for ACPI-based monitor logic; copies scripts into `/etc/acpi`, validates the startup hook in `~/.config/hypr/hyprland.conf`.
- /Users/yandy/.config/dotfiles/monitor-auto-switch/acpi/hypr-utils.sh: Core monitor business logic; discovers the Hyprland session, exports `XDG_RUNTIME_DIR` and `HYPRLAND_INSTANCE_SIGNATURE`, calls `hyprctl`, and dynamically rewrites monitor/workspace assignments.
- /Users/yandy/.config/dotfiles/ghostty/config: Independently symlinked terminal config; Hyprland-specific coupling is minimal and limited to a performance tweak for Hyprland.
- /Users/yandy/.config/dotfiles/wezterm/wezterm.lua: Independently symlinked terminal config; Linux-specific coupling is minimal and limited to enabling Wayland.
- /Users/yandy/.config/dotfiles/docs/architecture/overview.md: High-level architecture reference; documents the repo’s common-module, bootstrap, and symlink deployment model.

## Data Flow

The relevant flow is: `./bootstrap.sh` detects CachyOS as `arch`, runs generic package installation plus `/Users/yandy/.config/dotfiles/bin/install-arch-pkgs.sh`, then calls `ansible-playbook /Users/yandy/.config/dotfiles/playbooks/symlinks.yml`. The playbook builds a managed-file list, removes existing non-symlink destinations for any included config, and then creates symlinks; `~/.config/hypr` is included when `linux_desktop=true`. At runtime, Hyprland reads `~/.config/hypr/hyprland.conf`, which sources only repo-owned fragments; keybindings and idle/lock use generic tools (fuzzel, hyprlock, xdg-open). The monitor-auto-switch installer validates the startup hook in the Hypr config; ACPI events and Hyprland reload/startup drive `hypr-utils.sh` for monitor/workspace logic.

## Integration Points

The primary integration point for `arch-os-enhance` is `/Users/yandy/.config/dotfiles/playbooks/symlinks.yml`, because changing CachyOS from “skip Hypr unless explicitly enabled” to “manage Hyprland directly” changes bootstrap behavior and also activates the playbook’s destructive cleanup path for `~/.config/hypr`. The second integration point is the `/Users/yandy/.config/dotfiles/hypr` tree itself, which is a standalone CachyOS/Hyprland config with only repo-owned source fragments and generic launcher/bar/lock tooling. The monitor-auto-switch installer validates only the standard Hypr config path. `/Users/yandy/.config/dotfiles/bin/install-arch-pkgs.sh` is where runtime dependencies (fuzzel, waybar, hyprlock, etc.) are declared. `/Users/yandy/.config/dotfiles/ghostty/config` and `/Users/yandy/.config/dotfiles/wezterm/wezterm.lua` are lower-risk: they are already symlinked independently.

## Key Dependencies

Bash is the orchestration layer in `/Users/yandy/.config/dotfiles/bootstrap.sh` and the monitor scripts, Ansible is the deployment framework in `/Users/yandy/.config/dotfiles/playbooks/symlinks.yml`, and Hyprland plus `hyprctl` are the main runtime interfaces. Arch/CachyOS package management depends on `pacman` and optionally `yay`; monitor automation depends on `acpid`, `/etc/acpi/*`, and `systemctl`. The Hypr stack uses generic tools: fuzzel, waybar, hyprlock, hypridle, xdg-open, Ghostty, and WezTerm.
