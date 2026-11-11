# Monitor Auto-Switch

ACPI-driven monitor sync for the generic Hyprland desktop path in this repository.

This component keeps the checked-in fallback monitor layout and the runtime Hyprland state aligned on:

- session startup
- lid close
- lid open
- external monitor connect/disconnect detection

## Ownership Model

The monitor stack is split across three surfaces:

- [`hypr/hyprland.lua`](../hypr/hyprland.lua): owns the startup hook
- [`hypr/monitors.lua`](../hypr/monitors.lua): human-readable fallback/default layout
- [`monitor-auto-switch/acpi/hypr-utils.sh`](./acpi/hypr-utils.sh): runtime source of truth for live positions, scale, and workspace distribution

The installer validates that the Hypr root config already contains:

```lua
hl.on("hyprland.start", function()
  hl.exec_cmd("/etc/acpi/check-lid-on-startup.sh")
end)
hl.exec_cmd("/etc/acpi/check-lid-on-startup.sh")
```

It does not patch those lines into the config anymore.

## Current Baseline

The checked-in defaults target this layout:

- external `DP-2` at `3840x2160@180`, scale `1`, position `0x0`
- laptop `eDP-1` at `3456x2160@60`, scale `1.125`
- laptop position `3840x0` when docked
- laptop position `0x0` when running solo

Workspace behavior is intentionally simple:

- docked: workspaces `1-9` stay on the external monitor
- laptop-only: all workspaces stay on the laptop

There are no dedicated docked laptop workspaces in the current baseline.

## Prerequisites

- Hyprland session running under a Linux distro with ACPI support
- `acpid`
- `hyprctl`
- sudo/root access for installation
- the repo-owned Hypr config deployed or copied into `~/.config/hypr`

## Install

From the repository root:

```bash
sudo monitor-auto-switch/scripts/install-monitor-auto-switch.sh
```

The installer will:

- copy ACPI scripts into `/etc/acpi`
- create ACPI event files under `/etc/acpi/events`
- validate the startup hook in `~/.config/hypr/hyprland.lua`
- restart `acpid` unless `--no-restart` is passed
- run `check-lid-on-startup.sh` immediately when Hyprland is active

### Dry run

```bash
sudo monitor-auto-switch/scripts/install-monitor-auto-switch.sh --dry-run
```

### Skip service restart

```bash
sudo monitor-auto-switch/scripts/install-monitor-auto-switch.sh --no-restart
```

### Custom source directory

```bash
sudo monitor-auto-switch/scripts/install-monitor-auto-switch.sh --src /path/to/acpi
```

## Hardware Customization

Edit the installed runtime defaults in `/etc/acpi/hypr-utils.sh`:

```bash
sudo editor /etc/acpi/hypr-utils.sh
```

Update these values to match your machine:

```bash
LAPTOP_MONITOR="eDP-1"
EXTERNAL_MONITOR="DP-2"
EXTERNAL_RESOLUTION="3840x2160@180"
LAPTOP_RESOLUTION="3456x2160@60"
EXTERNAL_SCALE="1"
LAPTOP_SCALE="1.125"
EXTERNAL_POSITION="0x0"
LAPTOP_POSITION_DUAL="3840x0"
LAPTOP_POSITION_SOLO="0x0"
EXTERNAL_WS="${EXTERNAL_WS:-1 2 3 4 5 6 7 8 9}"
LAPTOP_WS="${LAPTOP_WS:-}"
```

After changing monitor defaults, keep [`hypr/monitors.lua`](../hypr/monitors.lua) in sync.

## Finding Monitor Names

```bash
hyprctl monitors
```

Use the `Monitor` names from that output, for example `eDP-1`, `DP-2`, or `HDMI-A-1`.

## Validation Flow

1. Preview generic desktop deployment:

   ```bash
   dotf link --dry-run --linux-desktop
   ```

2. Verify the deployed config surface:

   ```bash
   bash hypr/verify-symlink.sh
   ```

3. Dry-run the monitor installer:

   ```bash
   sudo monitor-auto-switch/scripts/install-monitor-auto-switch.sh --dry-run
   ```

4. Install live, then follow the manual checks in [`testing.md`](./testing.md).

## Troubleshooting

### Startup sync did not run

Check the Hypr root config:

```bash
grep -n "check-lid-on-startup" ~/.config/hypr/hyprland.lua
```

If the lines are missing, redeploy the repo-owned Hypr config before reinstalling the monitor scripts.

### External monitor not detected

1. Inspect `hyprctl monitors`
2. Update `EXTERNAL_MONITOR` in `/etc/acpi/hypr-utils.sh`
3. Re-run the startup check:

   ```bash
   sudo /etc/acpi/check-lid-on-startup.sh
   ```

### Workspaces end up on the wrong display

Review both files together:

- `/etc/acpi/hypr-utils.sh`
- `~/.config/hypr/monitors.lua`

They should agree on monitor names, scale, positions, and the external-workspace ownership model.

### Installer says the hook is missing

The installer only validates the repo-owned hook. Add or restore the lines in `~/.config/hypr/hyprland.lua`, then rerun the installer.

## Uninstall

Use the bundled uninstall script:

```bash
sudo monitor-auto-switch/scripts/uninstall-monitor-auto-switch.sh
```

That script removes the ACPI assets and the startup hook from the repo-owned `~/.config/hypr/hyprland.lua`, with backups unless you are in dry-run mode.

## Runtime Notes

- The scripts keep both monitors enabled when both are present.
- External monitor detection checks sysfs first, then `hyprctl`.
- The scripts lock out concurrent lid handlers with `/run/lid-switch.lock`.
- Logs are emitted with the `lid-events` tag and can be tailed with:

  ```bash
  journalctl -t lid-events -f
  ```
