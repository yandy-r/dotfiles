# Pattern Research: arch-os-enhance

## Architectural Patterns

**Centralized Platform Detection**: OS family detection is normalized in shell helpers, then reused by installers and shared shell modules. The repo distinguishes normalized families like `arch` from raw distro IDs like `cachyos`, which is the right seam for a CachyOS migration.

- Example: [common/platform.sh](/Users/yandy/.config/dotfiles/common/platform.sh)
- Example: [bootstrap.sh](/Users/yandy/.config/dotfiles/bootstrap.sh)
- Example: [bin/install-arch-pkgs.sh](/Users/yandy/.config/dotfiles/bin/install-arch-pkgs.sh)

**Declarative Deployment Manifest**: Symlink deployment is inventory-style, with lists of `{ src, dest, type }` and OS-gated task loops in Ansible instead of ad hoc linking logic. Generic Linux desktop configs are gated by `linux_desktop=true`.

- Example: [playbooks/symlinks.yml](/Users/yandy/.config/dotfiles/playbooks/symlinks.yml)
- Example: [playbooks/README.md](/Users/yandy/.config/dotfiles/playbooks/README.md)

**Layered Config Overrides**: Hyprland uses a layered source pattern: upstream/default config first, local overrides second. The pattern is reusable for a generic CachyOS base plus local overrides.

- Example: [hypr/hyprland.conf](/Users/yandy/.config/dotfiles/hypr/hyprland.conf)
- Example: [hypr/README.md](/Users/yandy/.config/dotfiles/hypr/README.md)
- Example: [hypr/monitors.conf](/Users/yandy/.config/dotfiles/hypr/monitors.conf)

**Helper Module / Thin Entrypoint Split**: Reusable logic lives in helper modules, while entry scripts stay thin. In shell this is `hypr-utils.sh`; in Lua it is `functions.lua` plus feature modules.

- Example: [monitor-auto-switch/acpi/hypr-utils.sh](/Users/yandy/.config/dotfiles/monitor-auto-switch/acpi/hypr-utils.sh)
- Example: [monitor-auto-switch/acpi/lid-open.sh](/Users/yandy/.config/dotfiles/monitor-auto-switch/acpi/lid-open.sh)
- Example: [wezterm/functions.lua](/Users/yandy/.config/dotfiles/wezterm/functions.lua)
- Example: [wezterm/wezterm.lua](/Users/yandy/.config/dotfiles/wezterm/wezterm.lua)

**Reusable App Config Parity**: Terminal/app configs are separate per tool, but intentionally aligned in behavior and appearance. That favors shared conventions over distro-specific branching.

- Example: [ghostty/config](/Users/yandy/.config/dotfiles/ghostty/config)
- Example: [wezterm/appearance.lua](/Users/yandy/.config/dotfiles/wezterm/appearance.lua)
- Example: [wezterm/keybindings.lua](/Users/yandy/.config/dotfiles/wezterm/keybindings.lua)

## Code Conventions

- Shell files and scripts use kebab-case filenames; shell functions use `snake_case`; predicates use `is_*`, getters use `get_*`, checks use `has_*` or `detect_*`.
- Shell constants and exported env vars are uppercase: `DOTFILES_OS`, `LOG_PREFIX`, `DRY_RUN`, `EXTERNAL_MONITOR`.
- Shell modules are imported with `source` or `.` and usually resolve paths from `SCRIPT_DIR` or a fixed runtime location.
- Lua modules follow `local M = {}` plus `return M`, imported with `require('module')`.
- Hyprland is organized as small `.conf` fragments sourced from the main config rather than one monolithic file.
- Feature directories are modular by concern: installer scripts in [`monitor-auto-switch/scripts/install-monitor-auto-switch.sh`](/Users/yandy/.config/dotfiles/monitor-auto-switch/scripts/install-monitor-auto-switch.sh), runtime hooks in [`monitor-auto-switch/acpi/hypr-utils.sh`](/Users/yandy/.config/dotfiles/monitor-auto-switch/acpi/hypr-utils.sh), user-facing docs in [`monitor-auto-switch/README.md`](/Users/yandy/.config/dotfiles/monitor-auto-switch/README.md).
- No repository/service-layer or class-heavy architecture is present here; the dominant abstractions are shell helper libraries, declarative Ansible manifests, Lua modules, and sourced config fragments.

## Error Handling

- Fail-fast shell mode is standard: `set -euo pipefail` appears in the relevant scripts.
- Install/bootstrap scripts use explicit logging helpers plus `error_exit()` for hard failures.
- Validation happens early: OS/distro checks, package manager checks, disk/network checks, source existence checks, and post-link validation.
- Optional behavior degrades with warnings instead of killing the run, for example optional AUR packages in [`bin/install-arch-pkgs.sh`](/Users/yandy/.config/dotfiles/bin/install-arch-pkgs.sh).
- Runtime monitor hooks are intentionally softer: if Hyprland is not ready, they log and `exit 0` to avoid breaking login/boot flow.
- Logging is context-specific:
  - colored stderr prefixes in [`bootstrap.sh`](/Users/yandy/.config/dotfiles/bootstrap.sh) and [`bin/install-arch-pkgs.sh`](/Users/yandy/.config/dotfiles/bin/install-arch-pkgs.sh)
  - `logger -t lid-events` in [`monitor-auto-switch/acpi/hypr-utils.sh`](/Users/yandy/.config/dotfiles/monitor-auto-switch/acpi/hypr-utils.sh)
  - `wezterm.log_info/warn/error` wrapped by [`wezterm/functions.lua`](/Users/yandy/.config/dotfiles/wezterm/functions.lua)
- Ansible uses `failed_when`, guarded `when:` clauses, and explicit validation/fail tasks in [`playbooks/symlinks.yml`](/Users/yandy/.config/dotfiles/playbooks/symlinks.yml).

## Testing Approach

- No automated shell test framework is evident for this area; the pattern is dry-run plus manual verification.
- Symlink changes are tested with `dryrun=true` and post-run validation in [`playbooks/symlinks.yml`](/Users/yandy/.config/dotfiles/playbooks/symlinks.yml) and documented in [`playbooks/README.md`](/Users/yandy/.config/dotfiles/playbooks/README.md).
- Hyprland linkage has a dedicated verification script in [`hypr/verify-symlink.sh`](/Users/yandy/.config/dotfiles/hypr/verify-symlink.sh).
- Monitor behavior is validated manually through `journalctl`, `hyprctl`, ACPI events, and lid open/close tests in [`monitor-auto-switch/testing.md`](/Users/yandy/.config/dotfiles/monitor-auto-switch/testing.md) and [`monitor-auto-switch/README.md`](/Users/yandy/.config/dotfiles/monitor-auto-switch/README.md).
- Installer scripts commonly support `--dry-run` and environment validation before mutation; [`bin/install-arch-pkgs.sh`](/Users/yandy/.config/dotfiles/bin/install-arch-pkgs.sh) is the clearest example.
- No mocking pattern was found; verification is system-level and behavior-oriented.

## Patterns to Follow

- Reuse the normalized OS-family pattern from [`common/platform.sh`](/Users/yandy/.config/dotfiles/common/platform.sh): treat CachyOS as `arch` family, and only use raw distro checks where a repo URL or distro-specific package source truly needs it.
- Use the generic Linux desktop gate in [`playbooks/symlinks.yml`](/Users/yandy/.config/dotfiles/playbooks/symlinks.yml) so Hypr/Waybar/Fuzzel are deployed only when explicitly requested.
- Preserve the layered-config model from [`hypr/hyprland.conf`](/Users/yandy/.config/dotfiles/hypr/hyprland.conf), with only repo-owned sources or local fragments.
- Put reusable Hyprland monitor/layout logic in helper scripts, following [`monitor-auto-switch/acpi/hypr-utils.sh`](/Users/yandy/.config/dotfiles/monitor-auto-switch/acpi/hypr-utils.sh), not inline inside one large `hyprland.conf`.
- Add new deployment targets by extending manifest lists in [`playbooks/symlinks.yml`](/Users/yandy/.config/dotfiles/playbooks/symlinks.yml), not by adding one-off shell linking scripts.
- Keep terminal/app configs generic and reusable, matching the current Ghostty/WezTerm pattern rather than distro-specific forks.
- Validate the migration with the repo’s existing safety model: dry-run Ansible, source existence checks, post-link validation, and manual Hyprland verification before replacing any existing managed path.
