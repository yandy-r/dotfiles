# Arch OS Enhance Implementation Plan

`arch-os-enhance` should keep CachyOS on the existing normalized `arch` bootstrap path while replacing the overlay-only Hyprland layer with a repo-owned generic Hyprland baseline. The work breaks cleanly into four technical seams: root Hypr source graph, desktop behavior fragments, monitor automation, and late deployment activation. The major risk is not package detection but destructive deployment, because `playbooks/symlinks.yml` can remove a real `~/.config/hypr` before linking if rollout is enabled too early. The plan therefore front-loads the Hypr contract, runs isolated desktop workstreams in parallel, and only then turns on generic Linux deployment plus docs and verification.

## Critically Relevant Files and Documentation

- /bootstrap.sh: Main setup entrypoint and Arch execution path.
- /common/platform.sh: Maps CachyOS into the normalized `arch` family.
- /bin/install-arch-pkgs.sh: Package install surface for new desktop dependencies.
- /playbooks/symlinks.yml: Controls whether Hypr, Waybar, and launcher configs are deployed.
- /playbooks/README.md: Deployment safety model, dry-run usage, and backup guidance.
- /hypr/README.md: Hypr file inventory and deployment.
- /hypr/hyprland.conf: Main Hyprland entrypoint and source chain.
- /hypr/bindings.conf: Keybindings and launcher behavior.
- /hypr/hyprlock.conf: Lockscreen config (repo-owned theme).
- /hypr/hypridle.conf: Idle-to-lock flow (hyprlock, loginctl).
- /hypr/monitors.conf: Static monitor config that must align with runtime monitor scripts.
- /hypr/envs.conf: Environment variables for screenshot/recording paths.
- /hypr/autostart.conf: Startup hook for Waybar and any chosen launcher helpers.
- /hypr/verify-symlink.sh: Existing validation helper for deployed Hypr symlinks.
- /monitor-auto-switch/scripts/install-monitor-auto-switch.sh: Hook validation for standard Hypr config path.
- /monitor-auto-switch/scripts/uninstall-monitor-auto-switch.sh: Hook removal symmetry and cleanup behavior.
- /monitor-auto-switch/acpi/hypr-utils.sh: Runtime source of truth for monitor names, scale, and workspace distribution.
- /monitor-auto-switch/README.md: Current monitor automation behavior and troubleshooting.
- /monitor-auto-switch/testing.md: Existing manual verification checklist for lid and monitor behavior.
- /docs/features/arch-package-installer.doc.md: Arch package-install contract and documentation surface.
- /docs/plans/arch-os-enhance/shared.md: Shared research summary for this feature.

## Implementation Plan

### Phase 1: Contract Locking

#### Task 1.1: Standalone Hypr source graph and environment cleanup Depends on [none]

**READ THESE BEFORE TASK**

- /hypr/README.md
- /hypr/hyprland.conf
- /hypr/envs.conf

**Instructions**

Files to Create

- /hypr/windows.conf

Files to Modify

- /hypr/hyprland.conf
- /hypr/envs.conf

Convert the root Hyprland entrypoint from an overlay into a repo-owned generic source graph. Source only local fragments that exist in this repository, add the new `windows.conf` fragment, and use repo-owned environment variables in `envs.conf`. Preserve the current modular layout so downstream tasks can work in isolated fragments without reopening the root contract. Do not enable deployment yet and do not change keybinding semantics in this task.

#### Task 1.2: Arch desktop runtime package contract Depends on [none]

**READ THESE BEFORE TASK**

- /bin/install-arch-pkgs.sh
- /docs/features/arch-package-installer.doc.md
- /docs/plans/arch-os-enhance/shared.md

**Instructions**

Files to Create

- None

Files to Modify

- /bin/install-arch-pkgs.sh
- /docs/features/arch-package-installer.doc.md

Add the missing desktop/runtime packages needed by the new baseline while keeping CachyOS on the generic Arch path. Lock the package contract in this task instead of deferring it: include `fuzzel` as the launcher package, `waybar` as the bar package, and the Hypr ecosystem/runtime packages needed by later tasks such as `hyprlock`, `hypridle`, `wf-recorder`, and any existing support packages those configs assume. Keep changes inside the existing grouped installer structure, document why each addition exists, and preserve dry-run plus non-interactive behavior. This task should not modify deployment activation or Hypr config; it establishes the concrete runtime package set that later tasks depend on.

### Phase 2: Isolated Desktop Workstreams

#### Task 2.1: Launcher defaults and keybinding rewrite Depends on [1.1, 1.2]

**READ THESE BEFORE TASK**

- /hypr/bindings.conf
- /ghostty/config
- /wezterm/wezterm.lua

**Instructions**

Files to Create

- /fuzzel/fuzzel.ini

Files to Modify

- /hypr/bindings.conf

Use generic Hyprland and Wayland-safe launcher commands while preserving the current mnemonic key layout as much as possible. Standardize on `fuzzel` in `fuzzel/fuzzel.ini`, wire terminal launches to Ghostty by default while leaving WezTerm available as a reusable secondary terminal, use `xdg-open` for URLs and browser/webapp-style binds, and use `$EDITOR` with a `nvim` fallback for editor-oriented binds. Keep this task focused on behavior and launcher defaults only; deployment and docs belong later.

#### Task 2.2: Lockscreen and idle flow (repo-owned) Depends on [1.1, 1.2]

**READ THESE BEFORE TASK**

- /hypr/hyprlock.conf
- /hypr/hypridle.conf
- /hypr/looknfeel.conf

**Instructions**

Files to Create

- None

Files to Modify

- /hypr/hyprlock.conf
- /hypr/hypridle.conf
- /hypr/looknfeel.conf

Rebuild the lock and idle flow so it is fully repo-owned and uses only repo-owned theme and background paths. Keep the lockscreen and idle visuals aligned with the existing look-and-feel choices in the repository, maintain a clean `hypridle -> hyprlock -> DPMS` flow, and ensure all lock commands use `hyprlock` or loginctl. Use only local config values and a repo-defined visual fallback such as a solid-color or gradient treatment derived from `looknfeel.conf`; do not assume user-specific files outside the repo.

#### Task 2.3: Monitor automation and hook ownership reconciliation Depends on [1.1]

**READ THESE BEFORE TASK**

- /hypr/monitors.conf
- /monitor-auto-switch/acpi/hypr-utils.sh
- /monitor-auto-switch/scripts/install-monitor-auto-switch.sh

**Instructions**

Files to Create

- None

Files to Modify

- /hypr/monitors.conf
- /monitor-auto-switch/acpi/hypr-utils.sh
- /monitor-auto-switch/scripts/install-monitor-auto-switch.sh

Adopt this ownership model explicitly: `hypr/hyprland.conf` is the only config that owns startup hook inclusion, `hypr/monitors.conf` remains the human-readable fallback/default reference, and `monitor-auto-switch/acpi/hypr-utils.sh` remains the runtime source of truth for live positions, scale, and workspace distribution. Align monitor names, scale, positions, and workspace distribution between `hypr/monitors.conf` and `hypr-utils.sh`, validate only the standard Hypr config path in the installer, and make the installer either validate the existing generic hook or skip hook injection entirely if the repo-owned Hypr config now owns it. Preserve the current user intent around laptop/external monitor behavior and add acceptance criteria in comments or docs that require parity between the static reference file and runtime script defaults.

#### Task 2.4: Waybar baseline and desktop startup integration Depends on [1.1, 1.2]

**READ THESE BEFORE TASK**

- /hypr/autostart.conf
- /hypr/looknfeel.conf
- /docs/plans/arch-os-enhance/shared.md

**Instructions**

Files to Create

- /waybar/config.jsonc
- /waybar/style.css

Files to Modify

- /hypr/autostart.conf

Introduce a repo-owned Waybar baseline with a custom theme and the initial widget set for clock, battery, and network status. Make the bar styling consistent with the existing Hypr look-and-feel, start it through `hypr/autostart.conf`, using only repo-owned or generic tooling. Keep the initial bar scope tight and reliable; modules beyond the requested core widgets should only be added if they are low-risk and do not complicate rollout.

### Phase 3: Activation, Verification, and Documentation

#### Task 3.1: Safe Linux desktop deployment activation Depends on [1.2, 2.1, 2.2, 2.3, 2.4]

**READ THESE BEFORE TASK**

- /playbooks/symlinks.yml
- /monitor-auto-switch/scripts/uninstall-monitor-auto-switch.sh
- /hypr/verify-symlink.sh

**Instructions**

Files to Create

- None

Files to Modify

- /playbooks/symlinks.yml
- /monitor-auto-switch/scripts/uninstall-monitor-auto-switch.sh
- /hypr/verify-symlink.sh

Add the new generic Linux desktop config targets to the deployment layer only after the standalone Hypr, launcher, and Waybar workstreams are complete. Update `playbooks/symlinks.yml` so generic Hypr/Waybar/fuzzel deployment is explicit and safe, keep generic Linux desktop deployment explicit and opt-in, mirror the final hook model in the uninstall script, and extend the symlink verification helper so it checks the newly managed config surface. This task is the activation gate; include backup and dry-run expectations directly in comments or surrounding logic where future maintainers need them.

#### Task 3.2: Generic Hyprland and monitor workflow documentation Depends on [3.1]

**READ THESE BEFORE TASK**

- /hypr/README.md
- /monitor-auto-switch/README.md
- /monitor-auto-switch/testing.md

**Instructions**

Files to Create

- None

Files to Modify

- /hypr/README.md
- /monitor-auto-switch/README.md
- /monitor-auto-switch/testing.md

Rewrite the Hypr and monitor docs to describe the new generic CachyOS + Hyprland baseline as the repo-owned generic baseline. Document the chosen launcher, Waybar, monitor ownership model, startup hooks, validation flow, and any changed workspace behavior. Fix current documentation drift while you are there, especially stale references to nonexistent helper commands or mismatched workspace distribution prose.

#### Task 3.3: Rollout and installer documentation refresh Depends on [3.1]

**READ THESE BEFORE TASK**

- /README.md
- /docs/features/arch-package-installer.doc.md
- /playbooks/README.md

**Instructions**

Files to Create

- None

Files to Modify

- /README.md
- /docs/features/arch-package-installer.doc.md
- /playbooks/README.md

Refresh the high-level rollout docs so the repository clearly advertises the new Linux desktop support and the safety expectations around enabling it. Update the root README, Arch package installer docs, and playbook docs to mention the new desktop packages, config directories, dry-run-first migration workflow, and the fact that deployment now manages generic Hyprland-related configs as the generic Hyprland/Waybar/fuzzel config set. Keep the instructions accurate for a user who will test this on a clean CachyOS install immediately after the implementation lands.

## Advice

- Keep `playbooks/symlinks.yml`, `hypr/hyprland.conf`, `hypr/bindings.conf`, and `monitor-auto-switch/scripts/install-monitor-auto-switch.sh` single-owner tasks. They are the highest-risk coordination surfaces.
- Do not activate generic Hypr deployment until the standalone source graph, bindings, lock/idle flow, and monitor hook model are all coherent; the playbook’s cleanup behavior makes partial rollout unsafe.
- Treat `hypr/monitors.conf` and `monitor-auto-switch/acpi/hypr-utils.sh` as one conceptual subsystem even though they are edited in different layers. Drift between them will create the hardest-to-debug regressions.
- Reuse Ghostty and WezTerm rather than inventing terminal-specific Linux forks. The migration risk is in Hypr and deployment, not terminal emulator configuration.
- Use the existing manual verification surfaces intentionally: Ansible dry-run, `hypr/verify-symlink.sh`, and `monitor-auto-switch/testing.md` should be part of the implementation acceptance criteria, not post-hoc checks.
- Correct documentation drift as part of the implementation, especially the current mismatch between monitor docs and runtime scripts. The user’s next test is a clean CachyOS install, so stale docs will create avoidable false negatives.
