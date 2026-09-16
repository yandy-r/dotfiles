# Code Analysis: arch-os-enhance

### Executive Summary

This feature sits in the repo’s deployment and desktop-runtime layers, not a normal app stack. CachyOS already normalizes to the shared `arch` path, so the real work is replacing overlay-only Hypr assumptions in the `hypr/` tree, exposing new configs through `playbooks/symlinks.yml`, and keeping that static config aligned with the root-installed ACPI monitor scripts.

### Related Components

- /Users/yandy/.config/dotfiles/common/platform.sh: Normalizes `cachyos|manjaro|endeavouros` to `arch`.
- /Users/yandy/.config/dotfiles/bootstrap.sh: Runs Arch package install and then the symlink playbook.
- /Users/yandy/.config/dotfiles/playbooks/symlinks.yml: Owns whether `~/.config/hypr` and new desktop config dirs are linked or skipped.
- /Users/yandy/.config/dotfiles/hypr/README.md: Documents Hypr scope and file map.
- /Users/yandy/.config/dotfiles/hypr/hyprland.conf: Main Hypr entrypoint and source chain.
- /Users/yandy/.config/dotfiles/hypr/bindings.conf: Launcher and keybinding definitions.
- /Users/yandy/.config/dotfiles/hypr/monitors.conf: Static monitor fallback comments and ACPI ownership note.
- /Users/yandy/.config/dotfiles/hypr/hyprlock.conf: Lockscreen uses repo-owned theme.
- /Users/yandy/.config/dotfiles/hypr/hypridle.conf: Idle behavior uses hyprlock and loginctl.
- /Users/yandy/.config/dotfiles/monitor-auto-switch/scripts/install-monitor-auto-switch.sh: Injects Hypr hooks and validates the standard Hypr config path.
- /Users/yandy/.config/dotfiles/monitor-auto-switch/acpi/hypr-utils.sh: Actual monitor/workspace runtime logic.
- /Users/yandy/.config/dotfiles/bin/install-arch-pkgs.sh: Package surface for Waybar, launcher, Hypr utilities, and `acpid`.

### Implementation Patterns

**OS Family Normalization**: Platform-specific work is deferred until after distro IDs are collapsed into shared families.

- Example: /Users/yandy/.config/dotfiles/common/platform.sh:58
- Apply to: package-install planning, Linux/Arch gating, avoiding CachyOS-specific branches unless absolutely necessary

**Declarative Symlink Inventory With Gated Activation**: Config deployment is driven by list variables plus `when:` clauses, not ad hoc shell linking.

- Example: /Users/yandy/.config/dotfiles/playbooks/symlinks.yml:52
- Apply to: adding `waybar` and launcher config dirs, generic Hypr/Waybar/fuzzel rollout, Linux-only desktop config rollout

**Destructive-Then-Link Deployment**: Included destinations are stat-checked and regular files/dirs are removed before symlinks are created.

- Example: /Users/yandy/.config/dotfiles/playbooks/symlinks.yml:227
- Apply to: planning safe migration of existing `~/.config/hypr`, `~/.config/waybar`, and launcher dirs

**Layered Hypr Fragment Sourcing**: `hyprland.conf` is a thin entrypoint that sources concern-specific fragments.

- Example: /Users/yandy/.config/dotfiles/hypr/hyprland.conf:4
- Apply to: using only repo-owned or local fragments, adding a local `windows.conf`, keeping launcher/bar config references out of one monolithic file

**Helper Module Plus Thin Event Entrypoints**: Real runtime logic lives in `hypr-utils.sh`; ACPI entry scripts only source it, settle, detect, and dispatch.

- Example: /Users/yandy/.config/dotfiles/monitor-auto-switch/acpi/hypr-utils.sh:46
- Apply to: keeping monitor/layout logic centralized, adding new runtime helpers, avoiding duplicated lid/startup behavior

**Defensive Shell Scripting With Optional Warnings**: Scripts fail fast for core prerequisites but downgrade optional post-steps to warnings.

- Example: /Users/yandy/.config/dotfiles/bin/install-arch-pkgs.sh:559
- Apply to: package additions, installer changes, hook injection, runtime dependency checks

### Integration Points

#### Files to Create

- /Users/yandy/.config/dotfiles/waybar/config.jsonc: Waybar module and layout config for clock, battery, and network.
- /Users/yandy/.config/dotfiles/waybar/style.css: Themed Waybar styling layer.
- /Users/yandy/.config/dotfiles/hypr/windows.conf: Repo-owned window behavior extension point.
- /Users/yandy/.config/dotfiles/fuzzel/fuzzel.ini: Wayland-native launcher configuration if the plan standardizes on `fuzzel`.

#### Files to Modify

- /Users/yandy/.config/dotfiles/playbooks/symlinks.yml: Add new app config entries and manage generic Linux desktop configs when `linux_desktop=true`.
- /Users/yandy/.config/dotfiles/hypr/hyprland.conf: Source only repo-owned fragments.
- /Users/yandy/.config/dotfiles/hypr/bindings.conf: Use generic launcher commands (fuzzel, xdg-open, etc.).
- /Users/yandy/.config/dotfiles/hypr/hyprlock.conf: Inline or localize theme/background definitions with repo-owned theme values.
- /Users/yandy/.config/dotfiles/hypr/hypridle.conf: Use hyprlock and loginctl for lock/DPMS flow.
- /Users/yandy/.config/dotfiles/hypr/envs.conf: Use repo-owned env vars for screenshot/recording paths.
- /Users/yandy/.config/dotfiles/hypr/monitors.conf: Clarify fallback monitor behavior vs ACPI-managed runtime layout.
- /Users/yandy/.config/dotfiles/hypr/README.md: Document generic baseline, file map, and deployment.
- /Users/yandy/.config/dotfiles/hypr/autostart.conf: Start Waybar and any chosen launcher helpers with generic launcher and bar tooling.
- /Users/yandy/.config/dotfiles/monitor-auto-switch/scripts/install-monitor-auto-switch.sh: Validate only the standard Hypr config path.
- /Users/yandy/.config/dotfiles/monitor-auto-switch/scripts/uninstall-monitor-auto-switch.sh: Keep hook removal logic aligned with installer changes.
- /Users/yandy/.config/dotfiles/monitor-auto-switch/acpi/hypr-utils.sh: Update defaults if the new baseline changes monitor naming, workspace policy, or notification expectations.
- /Users/yandy/.config/dotfiles/bin/install-arch-pkgs.sh: Add desktop/runtime packages in the existing grouped installer sections.
- /Users/yandy/.config/dotfiles/docs/features/arch-package-installer.doc.md: Document newly required Arch desktop packages.

### Conventions

- Naming: Lowercase feature dirs, kebab-case shell script names, and Hypr fragments named by concern such as `bindings.conf`, `monitors.conf`, `looknfeel.conf`, and `hypridle.conf`.
- Error handling: Shell scripts consistently use `set -euo pipefail`, early prerequisite checks, and helper log/error functions; optional cleanup/restart steps warn instead of aborting.
- Testing: This repo uses dry-runs and manual verification more than automated tests for desktop features; concrete patterns are in `playbooks/README.md`, `hypr/verify-symlink.sh`, and `monitor-auto-switch/testing.md`.

### Gotchas and Warnings

- Enabling a new symlink target is not passive: `playbooks/symlinks.yml` will delete an existing non-symlink destination before linking if the source exists.
- `hyprland.conf` is not standalone now; it sources only repo-owned fragments.
- `bindings.conf` is the highest-risk runtime file because it uses generic launcher commands, not just one terminal binding.
- Monitor policy currently has two sources of truth: `hypr/monitors.conf` says runtime config lives in `/etc/acpi/hypr-utils.sh`, and `hypr-utils.sh` holds the actual values.
- The monitor installer validates the standard Hypr config path.
- The docs already drift from code: current workspace-distribution prose does not match current script defaults, and `testing.md` references helpers that do not exist.
- `hypr/envs.conf` defines screenshot and recording paths; ensure variable names are consistent.

### Task Guidance by Area

- database: none. There is no DB layer here; persistent state is symlinks, `/etc/acpi/*`, log files, and Hypr runtime state.
- api: none in the web/service sense. The closest equivalents are `ansible-playbook`, `pacman`/`yay`, `systemctl`, `acpid`, and `hyprctl`.
- ui: this is desktop config, not app UI. The user-facing layer is Hypr fragments plus the new Waybar and launcher configs, with Ghostty/WezTerm already reusable and low-risk.
