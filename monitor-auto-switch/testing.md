# Testing Monitor Auto-Switch

Manual verification checklist for the generic Hyprland monitor workflow.

## Preflight

Confirm the repo-owned Hypr config and startup hook are in place:

```bash
bash hypr/verify-symlink.sh
grep -n "check-lid-on-startup" ~/.config/hypr/hyprland.lua
```

Preview monitor installation before running it live:

```bash
sudo monitor-auto-switch/scripts/install-monitor-auto-switch.sh --dry-run
```

## Watch Live Signals

Use separate terminals while testing:

```bash
# ACPI events
sudo acpi_listen

# Monitor script logs
journalctl -t lid-events -f

# Hyprland state
watch -n 1 'hyprctl monitors; echo; hyprctl workspaces'
```

## Test 1: Startup Sync

1. Start or restart Hyprland with the repo-owned config deployed.
2. Confirm the startup hook exists in `~/.config/hypr/hyprland.lua`.
3. Check the logs for a `check-lid-on-startup.sh` run.
4. Confirm the live monitor layout matches `hypr/monitors.lua` when docked.

Expected result:

- docked: `DP-2` on the left, `eDP-1` on the right
- laptop-only: `eDP-1` at `0x0`
- docked workspaces `1-9` land on the external display

## Test 2: Lid Close and Lid Open

1. With an external monitor connected, tail `journalctl -t lid-events -f`.
2. Close the laptop lid.
3. Wait for the lid-close handler to finish.
4. Open the lid again.
5. Check the logs for the lid-open handler.

Expected result:

- no concurrent handler errors beyond normal debounce messages
- external monitor remains usable throughout
- workspaces return to the expected display ownership after each event

## Test 3: Manual Script Invocation

Use this when physical lid testing is inconvenient:

```bash
# Startup sync
sudo /etc/acpi/check-lid-on-startup.sh

# Lid open
sudo /etc/acpi/lid-open.sh

# Lid close
sudo /etc/acpi/lid-close.sh
```

If the close path depends on the physical lid state on your machine, close the lid before invoking `lid-close.sh`.

## Test 4: Workspace Ownership

After startup sync, lid open, and lid close, verify workspace placement:

```bash
hyprctl workspaces
```

Expected baseline:

- docked: workspaces `1-9` on the external display
- docked: no dedicated laptop workspaces
- laptop-only: all workspaces on `eDP-1`

If that does not match the live result, compare:

```bash
grep -E "MONITOR|RESOLUTION|SCALE|POSITION|_WS" /etc/acpi/hypr-utils.sh
sed -n '1,80p' ~/.config/hypr/monitors.lua
```

## Test 5: Service and Asset Checks

```bash
systemctl status acpid
ls -la /etc/acpi/lid-open.sh /etc/acpi/lid-close.sh /etc/acpi/check-lid-on-startup.sh
ls -la /etc/acpi/events/lid-open /etc/acpi/events/lid-close
```

Expected result:

- `acpid` is active
- all three scripts exist in `/etc/acpi`
- both event files exist in `/etc/acpi/events`

## Troubleshooting

### No logs appear

```bash
journalctl -t lid-events -n 50
systemctl status acpid
```

If `acpid` is not running, restart it and repeat the test.

### External monitor is detected under a different name

```bash
hyprctl monitors
```

Update `/etc/acpi/hypr-utils.sh` and `hypr/monitors.lua` together so the checked-in fallback and runtime defaults stay aligned.

### Startup hook validation fails

Restore these lines in `~/.config/hypr/hyprland.lua`:

```lua
hl.on("hyprland.start", function()
  hl.exec_cmd("/etc/acpi/check-lid-on-startup.sh")
end)
hl.exec_cmd("/etc/acpi/check-lid-on-startup.sh")
```

Then rerun:

```bash
bash hypr/verify-symlink.sh
sudo monitor-auto-switch/scripts/install-monitor-auto-switch.sh --dry-run
```

### Workspaces drift after changing monitor geometry

Treat `hypr/monitors.lua` and `/etc/acpi/hypr-utils.sh` as one subsystem. Update both, then rerun `sudo /etc/acpi/check-lid-on-startup.sh`.
