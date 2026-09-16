# Arch OS Enhance

The current CachyOS path already enters the generic `arch` bootstrap flow through `/common/platform.sh`, so the real migration seam is not distro detection but the desktop layer deployed after `/bootstrap.sh` calls `/playbooks/symlinks.yml`. Today the checked-in Hyprland config was a legacy overlay: `/hypr/hyprland.conf`, `/hypr/bindings.conf`, `/hypr/hyprlock.conf`, and `/hypr/hypridle.conf` previously assumed external overlay source files, theme assets, and launcher helpers. Monitor behavior is further split between `/hypr/monitors.conf` and `/monitor-auto-switch/acpi/hypr-utils.sh`, so any CachyOS-safe baseline has to keep those two configuration surfaces aligned. Ghostty and WezTerm are already portable, while the missing pieces are a generic Hyprland baseline plus new Waybar and launcher config/deployment targets.

## Relevant Files

- /bootstrap.sh: Main setup entrypoint and Arch execution path.
- /common/platform.sh: Maps CachyOS into the normalized `arch` family.
- /bin/install-arch-pkgs.sh: Arch package install surface for new desktop dependencies.
- /playbooks/symlinks.yml: Controls whether `~/.config/hypr` is replaced or skipped.
- /hypr/README.md: Documents the Hypr config scope and deployment.
- /hypr/hyprland.conf: Main Hyprland entrypoint and source graph.
- /hypr/bindings.conf: Keybindings for launcher, terminal, and browser.
- /hypr/monitors.conf: User monitor defaults that must match runtime scripts.
- /hypr/hyprlock.conf: Lockscreen config (repo-owned theme).
- /hypr/hypridle.conf: Idle behavior and lock flow.
- /monitor-auto-switch/scripts/install-monitor-auto-switch.sh: Injects Hyprland ACPI hooks into managed configs.
- /monitor-auto-switch/acpi/hypr-utils.sh: Monitor and workspace runtime logic via `hyprctl`.
- /monitor-auto-switch/testing.md: Manual verification procedure for monitor automation.
- /ghostty/config: Reusable terminal config with minor Hyprland-specific tuning.
- /wezterm/wezterm.lua: Reusable Wayland-capable terminal config.

## Relevant Patterns

**Normalized OS Family Detection**: Distro IDs like CachyOS are collapsed into shared platform families so bootstrap and installers can stay generic. See [/common/platform.sh](/common/platform.sh).

**Declarative Symlink Inventories**: Config deployment is driven by Ansible manifest lists and `when:` gates, not ad hoc shell linking. See [/playbooks/symlinks.yml](/playbooks/symlinks.yml).

**Layered Hyprland Config Fragments**: Hyprland config is split into sourced fragments for bindings, monitors, input, and visual behavior. See [/hypr/hyprland.conf](/hypr/hyprland.conf).

**Helper Script Runtime Logic**: Complex monitor behavior lives in reusable shell helpers instead of inline compositor config. See [/monitor-auto-switch/acpi/hypr-utils.sh](/monitor-auto-switch/acpi/hypr-utils.sh).

**Portable Terminal Config Parity**: Terminal configs stay tool-specific but visually and behaviorally aligned across platforms. See [/ghostty/config](/ghostty/config) and [/wezterm/wezterm.lua](/wezterm/wezterm.lua).

## Relevant Docs

**/hypr/README.md**: You _must_ read this when replacing or extending the Hyprland tree because it documents scope and deployment.

**/playbooks/README.md**: You _must_ read this when changing deployment behavior or activating new symlink targets on Linux.

**/docs/architecture/overview.md**: You _must_ read this when changing bootstrap-to-deployment flow across multiple repo layers.

**/docs/architecture/platform-detection.md**: You _must_ read this when deciding whether CachyOS needs special handling beyond the existing `arch` mapping.

**/docs/features/arch-package-installer.doc.md**: You _must_ read this when adding Waybar, launcher, or Hyprland runtime packages to the Arch bootstrap path.

**/monitor-auto-switch/README.md**: You _must_ read this when preserving monitor, lid, and workspace behavior during the migration.

**https://wiki.cachyos.org/en/configuration/desktop_environments/hyprland/**: You _must_ read this when aligning with current CachyOS Hyprland expectations and avoiding deprecated upstream assumptions.

**https://wiki.hypr.land/Configuring/Monitors/**: You _must_ read this when reworking monitor syntax, layout, scale, or hotplug behavior.

**https://wiki.hypr.land/Configuring/Binds/**: You _must_ read this when translating keybindings into generic Hyprland bindings.

**https://wiki.hypr.land/Hypr-Ecosystem/hypridle/**: You _must_ read this when replacing idle behavior with native Hyprland ecosystem tooling.

**https://wiki.hypr.land/Hypr-Ecosystem/hyprlock/**: You _must_ read this when rebuilding the lockscreen with repo-owned theme assets.

**https://github.com/Alexays/Waybar/wiki/Styling**: You _must_ read this when adding a themed Waybar layer.

**https://github.com/Alexays/Waybar/wiki/Module:-Clock**: You _must_ read this when adding the clock widget.

**https://github.com/Alexays/Waybar/wiki/Module:-Battery**: You _must_ read this when adding the battery widget.

**https://github.com/Alexays/Waybar/wiki/Module:-Network**: You _must_ read this when adding the network widget.
