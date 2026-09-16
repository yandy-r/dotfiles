# Hyprland Configuration

Repo-owned Hyprland baseline for the generic Linux desktop path in this dotfiles repository.

## Scope

This directory is intended for a generic CachyOS or Arch-family Hyprland install. It is a standalone repo-owned baseline with no external overlay dependencies.

The dotfiles playbook only manages this config when you opt in:

```bash
ansible-playbook playbooks/symlinks.yml --extra-vars "dryrun=true linux_desktop=true"
ansible-playbook playbooks/symlinks.yml --extra-vars "backup=true linux_desktop=true"
```

Use the dry run first. The live activation path requires `backup=true` because it can replace an existing `~/.config/hypr` tree.

## Configuration Graph

`hyprland.lua` is the repo-owned root contract (Hyprland 0.55+). It `require()`s only local fragments:

- `monitors.lua`
- `input.lua`
- `bindings.lua`
- `envs.lua`
- `looknfeel.lua`
- `windows.lua`
- `autostart.lua`

The root config also owns the monitor startup hook:

```lua
hl.on("hyprland.start", function()
  hl.exec_cmd("/etc/acpi/check-lid-on-startup.sh")
end)
hl.exec_cmd("/etc/acpi/check-lid-on-startup.sh")
```

That hook is validated by `monitor-auto-switch/scripts/install-monitor-auto-switch.sh`. The installer does not patch `hyprland.lua` for you.

## Lua Migration (Hyprland 0.55+)

Hyprland 0.55 deprecates hyprlang in favor of Lua. Compositor config lives entirely in `.lua` files; separate ecosystem tools (`hypridle`, `hyprlock`, `hyprpaper`, etc.) still use their own tool-specific configs.

See [`LUA-MIGRATION.md`](./LUA-MIGRATION.md) for the migration tracker, ecosystem config notes, and validation commands.

Hyprland may auto-generate a stub `hyprland.conf` in a live session. It is gitignored and safe to delete; only `hyprland.lua` should own the compositor root.

## File Guide

- `hyprland.lua`: root config; `require()` graph plus monitor startup hook ownership
- `monitors.lua`: monitor fallback/default layout; keep aligned with `monitor-auto-switch/acpi/hypr-utils.sh`
- `LUA-MIGRATION.md`: migration tracker for Hyprland 0.55+ Lua cutover
- `bindings.lua`: launcher, terminal, browser, screenshot, recording, and window-group binds
- `input.lua`: input device configuration
- `envs.lua`: runtime environment variables, including screenshot and recording paths
- `session-env.lua`: reads `~/.config/env/dropbox_dir` and sets Dropbox-backed Hypr env vars via `hl.env()`
- `show-keybindings.lua`: searchable keybinding popup sourced from `bindings.lua` (rofi UI)
- `calc-copy.sh`: rofi/qalc clipboard helper (kept as shell — external CLI orchestration)
- `screenshot.sh`: grim/slurp/satty capture pipeline (kept as shell — process pipeline)
- `record-screen.sh`: repo-owned wf-recorder toggle helper with codec selection and failure notifications (kept as shell — process lifecycle)
- `looknfeel.lua`: shared Hyprland palette, window chrome, and group styling defaults; palette is duplicated manually in `hyprlock.conf`
- `windows.lua`: repo-owned window behavior extension point
- `autostart.lua`: session startup commands, currently including Waybar, Mako, and SwayOSD
  - also exports the session environment to D-Bus activation so Nautilus/Sushi and other D-Bus-launched Wayland apps inherit the correct Hypr/NVIDIA variables
- `../swayosd/style.css`: repo-owned transient OSD theme for volume and brightness feedback
- `../mako/config`: repo-owned Mako notification behavior, including click-to-focus fallback hooks
- `hypridle.conf`: idle to lock to DPMS flow (separate hypridle daemon config)
- `hyprlock.conf`: repo-owned lockscreen (separate hyprlock config)
- `hyprpaper.conf`: desktop wallpaper config (separate hyprpaper daemon config); add images to `~/.config/wallpapers/desktop`
- `hyprsunset.conf`: optional display temperature settings (separate hyprsunset config)
- `xdph.conf`: xdg-desktop-portal-hyprland screencopy settings
- `verify-symlink.sh`: post-deploy verification for Hypr, Waybar, SwayOSD, and Fuzzel symlinks

### Desktop wallpapers

Hyprpaper starts and recovers on reload from `autostart.lua`, then reads
`hyprpaper.conf` from the deployed `~/.config/hypr` directory. Add desktop
wallpapers to:

```text
~/.config/wallpapers/desktop
```

Wallpapers live outside the dotfiles repo (keeps binary assets out of git
history).

The current config uses that directory as a random rotating wallpaper source with
cover fitting. `timeout` is in seconds, so `timeout = 900` rotates every 15
minutes. Hyprpaper supports files or directories here; subdirectories are not
scanned unless `recursive = true` is added to `hyprpaper.conf`.

Keep files in a format Hyprpaper can decode, such as real JPEG or PNG data. A
file extension alone is not enough; for example, an AVIF file renamed to `.jpg`
can still fail to load.

## Desktop Defaults

### Launcher and terminals

- `SUPER+SPACE`: `fuzzel`
- `SUPER+R`: `fuzzel`
- `SUPER+=`: `rofi-calc`
- `SUPER+SHIFT+=`: floating `qalculate-gtk`
- `SUPER+CTRL+=`: custom `qalc` prompt that copies the result on Enter
- `SUPER+RETURN`: `ghostty`
- `SUPER+SHIFT+RETURN`: `wezterm`
- `SUPER+SHIFT+N`: `$EDITOR` with `nvim` fallback in Ghostty

### Floating windows

Floating windows in this baseline are movable. The rules in [`windows.lua`](./windows.lua) set the initial state and startup placement for matching apps, but they do not lock the window after it opens.

Current float-related controls from [`bindings.lua`](./bindings.lua):

- `SUPER+V`: toggle floating for the focused window
- `SUPER + left-drag`: move a floating window
- `SUPER + right-drag`: resize a floating window

Useful float rule patterns:

- `float = yes`: keep utility apps and transient tools out of the tiling layout
- `center = on`: good default for dialogs, previews, and screenshot editors
- `size = (...) (...)`: keep utility windows predictably sized across different monitor dimensions
- `move = x y`: place a window at a known startup position when centering is not desired
- `pin`: optional Hyprland behavior for windows that should stay visible across workspace changes; this baseline does not pin any windows by default

If a floating window reopens centered or in a fixed spot, that usually means its startup rule is being reapplied when the app creates a new window. Once the window is open, you can still move it with the drag bind unless you intentionally add a rule that changes that behavior.

Floating windows also stay above tiled windows in normal Hyprland behavior. If you want one out of the way temporarily, the practical approach is the special workspace scratchpad rather than trying to send it behind tiled windows.

### Scratchpad / special workspace

This baseline already uses Hyprland's special workspace as a scratchpad:

- `SUPER+SHIFT+S`: move the focused window to `special:magic`
- `SUPER+S`: show or hide the `magic` scratchpad

There is no dedicated "move out of scratchpad" bind in this baseline. To bring a scratchpad window back to a normal workspace:

- toggle the scratchpad visible with `SUPER+S`
- focus the window you want
- use the existing workspace move bind, e.g. `SUPER+SHIFT+1` through `SUPER+SHIFT+0`, to send it to a normal workspace

In other words, `SUPER+SHIFT+S` is one-way only because that key is explicitly bound to `movetoworkspace, special:magic`. Moving a window back out uses the normal `movetoworkspace` binds for numbered workspaces.

### Window groups

- `SUPER+G`: Toggle group mode on focused window
- `SUPER+ALT+G`: Remove window from group
- `SUPER+ALT+Arrow`: Move window into adjacent group
- `SUPER+ALT+Tab`: Cycle forward through grouped windows
- `SUPER+ALT+SHIFT+Tab`: Cycle backward through grouped windows
- `SUPER+ALT+L`: Lock/unlock active group (prevents adding/removing)
- `SUPER+ALT+{1-9}`: Jump to Nth window in group

### Browser and web shortcuts

Browser and web-style binds use `xdg-open`.

### Bar and notifications

Hyprland uses **Waybar** (not swaybar). `autostart.lua` launches `waybar` on session start. The repo-owned baseline lives in [`waybar/config.jsonc`](../waybar/config.jsonc) and [`waybar/style.css`](../waybar/style.css).

The baseline also uses **Mako** for desktop notifications. `autostart.lua` launches `mako` on login and also re-spawns it on Hypr reload if it is missing. The repo-owned baseline lives in [`mako/config`](../mako/config) and uses an Omarchy-style left-click handler: clicking a notification first tries the app's native default action, then falls back to focusing an existing Hypr window that matches the sender metadata, and finally dismisses the popup.

The baseline also uses **SwayOSD** for transient volume and brightness feedback. `autostart.lua` launches `swayosd-server` on login and also re-spawns it on Hypr reload if it is missing. The multimedia bindings in [`bindings.lua`](./bindings.lua) call `swayosd-client` directly so the key handling stays aligned with the upstream tool.

Because notification daemons only receive the metadata supplied by the app, exact browser-tab or conversation targeting is not guaranteed. When an app does not provide a default action, the fallback is limited to focusing the most relevant matching app window. This baseline favors correct left-click behavior over a richer notification center UI.

**Bar not visible?**

1. **Install Waybar**: `sudo pacman -S waybar` (or your distro’s package).
2. **Deploy config**: The bar config is only symlinked when the playbook is run with `linux_desktop=true`. Run:

   ```bash
   ansible-playbook playbooks/symlinks.yml -e "linux_desktop=true" -e "backup=true"
   ```

   so that `~/.config/waybar` points at the repo’s `waybar/` directory.
3. **See errors**: In a terminal under Hyprland run `waybar` and check for missing modules or fonts (e.g. `CaskaydiaCove Nerd Font` or `CaskaydiaMono Nerd Font`). Install any missing fonts or adjust `waybar/style.css` if you don’t use Nerd Fonts.
4. **Restart**: After installing or fixing config, run `waybar` once from a terminal or log out and back in so `exec-once` runs again.

### Screenshots and recording

`bindings.lua` uses:

- `grim`
- `slurp`
- `wl-copy`
- `wf-recorder`

The paths come from [`session-env.lua`](./session-env.lua), which is applied in `envs.lua` and re-synced to D-Bus in `autostart.lua`. It reads `~/.config/env/dropbox_dir` so the compositor gets the same Dropbox-backed paths even when the login manager did not source your shell exports first. `common/exports.sh` can load the same value for interactive shells.

```lua
local session_env = require("session-env")
session_env.apply(hl)
```

`CTRL+SHIFT+4`, `CTRL+SHIFT+5`, and `CTRL+SHIFT+6` are the screenshot binds for region edit, region quick-save, and full-screen edit respectively. `CTRL+SHIFT+7` calls [`record-screen.sh`](./record-screen.sh) in `screen` mode and `CTRL+SHIFT+8` calls it in `region` mode. `session-env.lua` seeds `DROPBOX_DIR`, `HYPR_SCREENSHOT_DIR`, and `HYPR_SCREENRECORD_DIR` into the running Hypr session from `~/.config/env/dropbox_dir` on login and on config reload. The screenshot helper and `quick-region` mode both use `HYPR_SCREENSHOT_DIR` when it is set, and otherwise fall back to `~/Pictures/Screenshots`. The recording helper creates the output path, toggles `wf-recorder`, uses `slurp` for region selection, and auto-picks a codec. It tries `h264_nvenc` first when ffmpeg exposes it, then falls back to `libx264` if `wf-recorder` cannot initialize NVENC in the current process context. If you want to force a specific codec, set `HYPR_WF_RECORDER_CODEC` in `envs.lua`.

If `DROPBOX_DIR` is unset, recordings fall back to `~/Pictures/Recordings`.

### Shell helpers kept intentionally

Some Hypr-adjacent helpers remain shell scripts because they orchestrate external CLI pipelines, process lifecycle, or deployment checks rather than compositor config:

- `calc-copy.sh`: rofi, qalc, wl-copy, and notify-send orchestration
- `screenshot.sh`: grim, slurp, satty, hyprctl, and jq capture pipeline (includes `quick-region` quick-save mode)
- `record-screen.sh`: wf-recorder toggling, signal handling, codec fallback, and temp logs
- `verify-symlink.sh`: standalone post-deploy symlink verification (not compositor config)

Converting these to Lua would mostly replace shell pipelines with `io.popen` and `os.execute` without improving clarity.

#### Converting recordings to GIF

`record-screen.sh` writes MP4 via `wf-recorder`, which is the right format for uploads, editing, and local playback. For README embeds and small animated demos, convert to GIF with [`gifski`](https://gif.ski). Install with `sudo pacman -S gifski`.

gifski uses a 256-color palette per frame with temporal dithering, which handles dark UIs and subpixel text noticeably better than ffmpeg's built-in `paletteuse`.

Common conversions:

```bash
# Hero GIF (~2 MB target): 1280 wide, 20 fps
gifski -o out.gif --fps 20 --width 1280 --quality 95 input.mp4

# Inline feature GIF (~1 MB target): 960 wide, 15 fps
gifski -o out.gif --fps 15 --width 960 --quality 90 input.mp4

# Tiny preview (~400 KB): 640 wide, 12 fps
gifski -o out.gif --fps 12 --width 640 --quality 80 input.mp4
```

To trim a range before converting, cut the MP4 with ffmpeg first:

```bash
ffmpeg -ss 00:00:03 -to 00:00:08 -i input.mp4 -c copy /tmp/trimmed.mp4
gifski -o out.gif --fps 15 --width 960 --quality 90 /tmp/trimmed.mp4
```

If `gifski` is unavailable, the two-pass ffmpeg palette approach is the best fallback:

```bash
ffmpeg -i input.mp4 -vf "fps=15,scale=960:-1:flags=lanczos,palettegen=max_colors=256:stats_mode=diff" -y /tmp/palette.png
ffmpeg -i input.mp4 -i /tmp/palette.png -lavfi "fps=15,scale=960:-1:flags=lanczos [x]; [x][1:v] paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" -y out.gif
```

Sizing hints:

- Lower `--fps` before lowering `--width` when trimming file size. Dropping 30 → 15 fps roughly halves the file with no perceived quality loss on UI captures.
- Record at native resolution and let `gifski --width` downscale. Pre-scaling in `wf-recorder` resamples through a weaker scaler than gifski's.
- `--fast` encodes about 3× quicker at a ~5% quality cost; useful while iterating.

## Monitor Ownership Model

This repo uses a strict split:

- `hypr/hyprland.lua`: owns startup hook inclusion
- `hypr/monitors.lua`: human-readable fallback/default layout
- `monitor-auto-switch/acpi/hypr-utils.sh`: runtime source of truth for live positions, scale, and workspace distribution

Keep `monitors.lua` and `hypr-utils.sh` aligned. The current checked-in default is:

- external `DP-2` at `3840x2160@180`, scale `1`, position `0x0`
- laptop `eDP-1` at `3456x2160@60`, scale `1.125`, position `3840x0` when docked

When an external display is connected, the runtime scripts keep workspaces `1-9` on the external monitor and leave the laptop display without dedicated workspaces. When no external display is present, all workspaces stay local to the laptop.

See [`monitor-auto-switch/README.md`](./monitor-auto-switch/README.md) for installation and runtime behavior.

## Lock and Idle Flow

The lock and idle chain is repo-owned:

- `hypridle` locks the session after 30 minutes
- `hyprlock` provides the lockscreen
- DPMS turns off one minute after the lock event
- DPMS turns back on when the session resumes

The lockscreen styling is derived from the palette in `looknfeel.lua`, but Hyprlock uses its own duplicated color values because it cannot source Hyprland variables directly.

## Validation

### Verify deployment state

```bash
bash hypr/verify-symlink.sh
```

This checks:

- `~/.config/hypr`
- `~/.config/waybar`
- `~/.config/swayosd`
- `~/.config/mako`
- `~/.config/fuzzel`
- the presence of the startup hook in `~/.config/hypr/hyprland.lua`

### Preview playbook changes

```bash
ansible-playbook playbooks/symlinks.yml --extra-vars "dryrun=true linux_desktop=true"
```

### Apply with backups

```bash
ansible-playbook playbooks/symlinks.yml --extra-vars "backup=true linux_desktop=true"
```

### Monitor hook sanity check

```bash
grep -n "check-lid-on-startup" ~/.config/hypr/hyprland.lua
```

## Notes

- `windows.lua` is intentionally lightweight and exists as the repo-owned extension point for future generic window rules.
- `looknfeel.lua` sets concrete defaults for borders, gaps, opacity, and rounding as repo-owned defaults.
