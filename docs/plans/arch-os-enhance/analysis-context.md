# Context Analysis: arch-os-enhance

### Executive Summary

`arch-os-enhance` is a migration from an overlay-based Hyprland setup to a standalone CachyOS + Hyprland baseline. The core approach keeps CachyOS on the existing normalized `arch` path, uses repo-owned Hypr files and generic Hyprland/Wayland commands, and deploys `~/.config/hypr` only when the generic Linux desktop is enabled.

### Architecture Context

- System Structure: Bootstrap already treats CachyOS as `arch` via [common/platform.sh](/Users/yandy/.config/dotfiles/common/platform.sh) and [bootstrap.sh](/Users/yandy/.config/dotfiles/bootstrap.sh), so the migration seam is not OS detection. The real seam is the Linux symlink/deployment layer in [playbooks/symlinks.yml](/Users/yandy/.config/dotfiles/playbooks/symlinks.yml) plus the Hypr config tree in [hypr/hyprland.conf](/Users/yandy/.config/dotfiles/hypr/hyprland.conf).
- Data Flow: `bootstrap.sh` detects `arch`, runs the Arch installer, then calls the symlink playbook; Hyprland then loads `~/.config/hypr/hyprland.conf`, which sources only repo-owned fragments; ACPI monitor hooks are installed separately and mutate runtime monitor/workspace state through `hyprctl`. The implementation plan should treat bootstrap/deploy, compositor config, and ACPI monitor automation as three connected layers.
- Integration Points: The highest-risk integration is [playbooks/symlinks.yml](/Users/yandy/.config/dotfiles/playbooks/symlinks.yml) because once `hypr` is included in the managed Linux config set, the playbook’s cleanup step can delete a real `~/.config/hypr` directory before linking. The second hotspot is keeping [hypr/monitors.conf](/Users/yandy/.config/dotfiles/hypr/monitors.conf) aligned with [monitor-auto-switch/acpi/hypr-utils.sh](/Users/yandy/.config/dotfiles/monitor-auto-switch/acpi/hypr-utils.sh). The third is adding new Waybar and launcher package plus symlink coverage, because the repo deploys terminals and `hypr`/waybar/fuzzel when `linux_desktop=true`.

### Critical Files Reference

- /Users/yandy/.config/dotfiles/docs/plans/arch-os-enhance/shared.md: Canonical planning summary and source of truth for task decomposition.
- /Users/yandy/.config/dotfiles/playbooks/symlinks.yml: Determines whether `~/.config/hypr` is managed and contains the destructive cleanup path.
- /Users/yandy/.config/dotfiles/hypr/README.md: Documents Hypr scope, file map, and deployment.
- /Users/yandy/.config/dotfiles/hypr/hyprland.conf: Main entrypoint and repo-owned source graph.
- /Users/yandy/.config/dotfiles/hypr/bindings.conf: Launcher, terminal, and keybindings.
- /Users/yandy/.config/dotfiles/hypr/hyprlock.conf: Repo-owned lockscreen theme.
- /Users/yandy/.config/dotfiles/hypr/hypridle.conf: Idle-to-lock flow with hyprlock.
- /Users/yandy/.config/dotfiles/hypr/monitors.conf: Static monitor fallback file that must match runtime monitor scripts.
- /Users/yandy/.config/dotfiles/monitor-auto-switch/scripts/install-monitor-auto-switch.sh: Validates startup hook in standard Hypr config path.
- /Users/yandy/.config/dotfiles/monitor-auto-switch/acpi/hypr-utils.sh: Runtime source of truth for monitor names, positions, and workspaces.
- /Users/yandy/.config/dotfiles/monitor-auto-switch/testing.md: Manual validation procedure for lid, monitor, and workspace behavior.
- /Users/yandy/.config/dotfiles/bin/install-arch-pkgs.sh: Package-install seam for Waybar, launcher, and Hyprland runtime dependencies.

### Patterns to Follow

- Pattern: Keep CachyOS inside the normalized `arch` family unless a package source or distro-specific behavior truly forces a branch. Example file: [common/platform.sh](/Users/yandy/.config/dotfiles/common/platform.sh).
- Pattern: Extend the declarative Ansible manifest instead of adding ad hoc shell symlink logic. Example file: [playbooks/symlinks.yml](/Users/yandy/.config/dotfiles/playbooks/symlinks.yml).
- Pattern: Preserve the layered Hypr config model, with only local generic fragments. Example file: [hypr/hyprland.conf](/Users/yandy/.config/dotfiles/hypr/hyprland.conf).
- Pattern: Keep monitor/workspace behavior in reusable helper scripts, not inline compositor config. Example file: [monitor-auto-switch/acpi/hypr-utils.sh](/Users/yandy/.config/dotfiles/monitor-auto-switch/acpi/hypr-utils.sh).
- Pattern: Reuse existing terminal configs instead of forking distro-specific Ghostty/WezTerm variants. Example files: [ghostty/config](/Users/yandy/.config/dotfiles/ghostty/config), [wezterm/wezterm.lua](/Users/yandy/.config/dotfiles/wezterm/wezterm.lua).

### Cross-Cutting Concerns

- Safety: enabling direct Hypr symlinking before the new baseline is complete risks deleting a user’s existing `~/.config/hypr`.
- Behavioral correctness: monitor definitions exist in both static Hypr config and ACPI runtime config; if names, scaling, or positions diverge, workspace movement and hotplug behavior will become inconsistent.
- Dependency completeness: new Waybar and launcher defaults are a full-stack addition, not a config-only change; there are currently no matching config directories or package-install hooks in the repo.
- Testing: this area relies on dry-run and manual system verification, not automated tests. The plan needs explicit verification tasks for symlink safety, Hypr startup, keybindings, and lid/monitor behavior.
- Documentation drift: `monitor-auto-switch/README.md` and `monitor-auto-switch/testing.md` already diverge from the code, so docs cleanup is part of the migration rather than an optional follow-up.

### Parallelization Opportunities

- Independent work area: deployment and package planning for Ansible plus Arch installer changes.
- Independent work area: Hypr baseline conversion covering `hyprland.conf`, `bindings.conf`, `hypridle.conf`, and `hyprlock.conf`.
- Independent work area: new Waybar and launcher defaults, including config structure and package assumptions.
- Independent work area: monitor automation review, especially validating the standard Hypr config path while preserving current workspace behavior.
- Coordination hotspot: the Hypr baseline and monitor automation streams must agree on monitor names, startup hooks, and workspace ownership.
- Coordination hotspot: deployment changes should land after the standalone Hypr baseline exists, otherwise the playbook can switch users onto an incomplete config.

### Implementation Constraints

- No distro-detection rewrite is needed; CachyOS already resolves to `arch`.
- The existing `hypr/` directory is not safe to deploy generically in its current form if it still depended on external overlay paths; the repo-owned baseline is now standalone.
- Ghostty and WezTerm should be reused as-is unless launcher bindings require a default-terminal decision.
- The migration must preserve current monitor behavior and keybinding intent, not just remove overlay references.
- The rollout has to avoid destructive takeover of an existing `~/.config/hypr` until backup, dry-run, and validation strategy are explicit.
- Waybar and launcher work are greenfield inside this repo: package entries, config directories, and symlink targets all need to be planned.

### Planning Recommendations

- Phase 1 should isolate deployment safety and the standalone Hypr source graph before any activation change.
- Phase 2 should run disjoint workstreams in parallel: bindings/launcher, lock/idle, monitor automation, and Waybar.
- Phase 3 should activate deployment late, then update docs and validation helpers against the final rollout model.
- Package/deployment scaffolding can be planned in parallel with Hypr baseline design, but the actual symlink-enable step should depend on baseline completion plus monitor validation.
