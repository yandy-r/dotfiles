# Documentation Research: arch-os-enhance

## Architecture Docs

- /docs/architecture/overview.md: High-level repo architecture, module relationships, and symlink deployment model.
- /docs/architecture/common-system.md: Shared config load order, platform-aware module boundaries, and where Linux-specific behavior fits.
- /docs/architecture/platform-detection.md: OS/package-manager detection model; important for how Arch-derived distros like CachyOS are classified.

## API Docs

No dedicated API docs exist for this feature area. Closest material is `/docs/reference/*`, but it covers shell aliases/functions/environment variables rather than CachyOS, Hyprland, or Waybar interfaces.

## Development Guides

- /docs/features/arch-package-installer.doc.md: Arch-native package/bootstrap strategy; useful for CachyOS package planning.
- /monitor-auto-switch/testing.md: Validation workflow for the current Hyprland monitor/workspace automation.

## README Files

- /README.md: Cross-platform repo overview, bootstrap behavior, and Linux/Arch support context.
- /docs/README.md: Documentation index and map of architecture/features/reference coverage.
- /playbooks/README.md: Symlink deployment workflow and safety model.
- /hypr/README.md: Hypr config scope, file map, and deployment instructions.
- /monitor-auto-switch/README.md: Current monitor auto-switch design, install flow, hardware variables, and troubleshooting.

## Must-Read Documents

- /hypr/README.md: File map, deployment, and validation.
- /hypr/hyprland.conf: Source graph, override boundaries, NVIDIA envs, and upstream Hyprland doc link.
- /hypr/monitors.conf: Current monitor model; comments say runtime monitor config is delegated to `/etc/acpi/hypr-utils.sh`.
- /hypr/bindings.conf: Launcher, webapp, and keybindings.
- /hypr/hyprlock.conf and /hypr/hypridle.conf: Lock/idle behavior and theme.
- /monitor-auto-switch/README.md: Current dual-monitor/lid workflow and safety rationale.
- /monitor-auto-switch/testing.md: Expected behavior and test procedure before changing the baseline.
- /monitor-auto-switch/acpi/hypr-utils.sh: Hardware-specific monitor/workspace variables, detection logic, and workspace binding helpers.
- /monitor-auto-switch/scripts/install-monitor-auto-switch.sh: How Hyprland hooks are injected into `~/.config/hypr/hyprland.conf`.
- /playbooks/symlinks.yml: Generic Linux desktop symlink behavior; manages `~/.config/hypr` when `linux_desktop=true`.
- https://wiki.cachyos.org/en/configuration/desktop_environments/hyprland/: Official CachyOS Hyprland doc; current docs frame CachyOS Hyprland settings as deprecated.
- https://wiki.hypr.land/Configuring/Monitors/: Official monitor syntax and monitor-management behavior.
- https://wiki.hypr.land/Configuring/Binds/: Official keybinding syntax and bind behavior.
- https://wiki.hypr.land/Hypr-Ecosystem/hypridle/: Official idle daemon/listener model.
- https://wiki.hypr.land/Hypr-Ecosystem/hyprlock/: Official lockscreen config model.
- https://github.com/Alexays/Waybar/wiki/Styling: Official Waybar styling/CSS model.
- https://github.com/Alexays/Waybar/wiki/Module:-Clock: Official clock module options.
- https://github.com/Alexays/Waybar/wiki/Module:-Battery: Official battery module options.
- https://github.com/Alexays/Waybar/wiki/Module:-Network: Official network module options.

## Documentation Gaps

- No local doc previously described a generic CachyOS + Hyprland baseline; local Hypr docs now cover the repo-owned baseline.
- No local Waybar documentation or `waybar/` config directory exists.
- No local launcher documentation or launcher config directory exists.
- No migration doc explains how to replace overlay-sourced Hypr files with distro-native Hyprland/Waybar configs.
- /docs/architecture/platform-detection.md documents `arch`, but not CachyOS explicitly; Arch-derived distro behavior is underdocumented.
- /docs/README.md advertises `/docs/development/*`, but that section is effectively empty.
- /monitor-auto-switch/README.md links to testing docs; the file on disk is `/monitor-auto-switch/testing.md`.
