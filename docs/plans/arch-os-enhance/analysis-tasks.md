# Task Analysis: arch-os-enhance

### Executive Summary

Use a three-phase plan with a short contract-locking phase, a wide implementation phase over disjoint file owners, and a late activation plus documentation phase. The key planning rule is to keep activation surfaces single-owner and late: `playbooks/symlinks.yml`, `hypr/hyprland.conf`, `hypr/bindings.conf`, and `monitor-auto-switch/scripts/install-monitor-auto-switch.sh` should never be edited concurrently.

### Recommended Phase Structure

#### Phase 1: Contract Locking

- purpose: settle the standalone Hyprland shape before any deployment or activation changes
- suggested tasks:
  - define the generic Hypr source graph and ownership of `hypr/hyprland.conf`
  - choose launcher/default-app strategy so `hypr/bindings.conf` has a clear target
  - decide whether monitor startup hooks stay installer-injected or move into the checked-in Hypr config
  - inventory required Arch packages in `bin/install-arch-pkgs.sh`
- parallelization notes:
  - keep one small serial “config contract” task for `hyprland.conf`
  - package inventory can run in parallel
  - do not touch `playbooks/symlinks.yml` yet, because it is the activation gate

#### Phase 2: Isolated Workstreams

- purpose: build the new Linux desktop surfaces in parallel while keeping each task to distinct files
- suggested tasks:
  - Hypr baseline task for `hypr/hyprland.conf` and repo-owned fragments
  - bindings and launcher task for `hypr/bindings.conf` plus new launcher config
  - lock and idle task for `hypr/hypridle.conf` and `hypr/hyprlock.conf`
  - monitor automation task for `hypr/monitors.conf`, `monitor-auto-switch/acpi/hypr-utils.sh`, and hook scripts
  - Waybar task to create the new `waybar/` config tree and styling
  - Arch package task to update `bin/install-arch-pkgs.sh` for Hyprland, Waybar, launcher, and support tools
- dependencies:
  - every Phase 2 task depends on the Phase 1 config contract
  - launcher wiring depends on the chosen launcher binary/config shape
  - monitor automation depends on the hook ownership decision

#### Phase 3: Activation and Verification

- purpose: enable deployment only after the config tree is coherent, then verify end-to-end behavior and document it
- suggested tasks:
  - deployment activation task in `playbooks/symlinks.yml` and related uninstall symmetry
  - verification task covering Ansible dry-run, Hypr startup, binding smoke test, Waybar render, and monitor lid/open-close behavior
  - docs tasks for `hypr/README.md`, `monitor-auto-switch/README.md`, `monitor-auto-switch/testing.md`, and rollout docs
- integration focus:
  - activate `~/.config/hypr` management only after Phase 2 is complete
  - verification should happen against the deployed symlinked config, not only checked-in files

### Task Granularity Guidance

- appropriate task sizes:
  - existing-file tasks: 1-3 files, one runtime concern, one clear verification target
  - new additive config tasks: one directory tree plus one owning entrypoint file
- tasks to split:
  - split Hypr rewrite into `hyprland root`, `bindings/launcher`, `lock/idle`, and `monitor` streams
  - split operational docs from rollout docs when they touch different surfaces
- tasks to combine:
  - combine launcher config and binding rewiring, because `bindings.conf` is the user-facing integration point
  - combine monitor install and uninstall hook updates only when one owner can safely manage both
  - combine Waybar config and autostart only if one owner controls both files

### Dependency Analysis

#### Independent Tasks

- Arch package manifest updates
- Waybar config creation
- lockscreen and idle rewrite
- launcher config creation before final bind wiring
- monitor helper logic updates after hook contract is fixed

#### Sequential Tasks

- Hypr root contract before any fragment task, because `hypr/hyprland.conf` defines the source graph
- launcher selection before final `bindings.conf` edits, because `hypr/bindings.conf` currently encodes the command model
- monitor hook policy before installer changes, because `install-monitor-auto-switch.sh` patches live configs
- deployment activation after all config work, because `playbooks/symlinks.yml` can remove existing non-symlink destinations

#### Potential Bottlenecks

- `playbooks/symlinks.yml`: schema pivot for whether Hypr and future Waybar/launcher dirs are deployed at all
- `hypr/hyprland.conf`: root source chain; any change here affects every downstream fragment
- `hypr/bindings.conf`: launcher/default-app choke point
- `monitor-auto-switch/scripts/install-monitor-auto-switch.sh`: live-config patcher; easy place for drift
- `monitor-auto-switch/acpi/hypr-utils.sh`: hardware-specific monitor schema that must stay aligned with `hypr/monitors.conf`

### Suggested Task Template

- title format:
  - `arch-os-enhance: <surface> - <outcome>`
- dependency annotation format:
  - `Depends on [task-id-1, task-id-2]`
- instruction completeness checklist:
  - exact files owned
  - target runtime behavior
  - explicit non-goals
  - package/runtime prerequisites
  - verification commands and manual checks
  - rollback or safety notes if the task touches deployment or live-config patching
