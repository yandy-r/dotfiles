# Hyprland Lua Migration Tracker

Hyprland 0.55+ deprecates hyprlang (`.conf`) in favor of Lua (`.lua`). This repo has completed the compositor migration; legacy compositor `.conf` fragments were removed after cutover.

**Cutover rule:** `hyprland.lua` is the compositor root config. Hyprland loads it at startup.

**Note:** Hyprland may auto-generate a stub `hyprland.conf` in `~/.config/hypr` during migration or reload. Delete it from the repo — only `hyprland.lua` should own the compositor root.

## Status

| Fragment | Lua (active) | Status | Notes |
| --- | --- | --- | --- |
| Autostart | `autostart.lua` | done | GPU D-Bus marker synced by `bin/gpu-switch.sh` |
| Environment | `envs.lua` | done | GPU CONFIG marker synced by `bin/gpu-switch.sh`; Dropbox paths via `session-env.lua` |
| Monitors | `monitors.lua` | done | Includes `GDK_SCALE`; keep aligned with `monitor-auto-switch/acpi/hypr-utils.sh` |
| Input | `input.lua` | done | `hl.config`, `hl.window_rule`, `hl.gesture`, `hl.device` |
| Bindings | `bindings.lua` | done | `hl.bind()` with `description`; typed `hl.dsp.*` dispatchers |
| Session env | `session-env.lua` | done | Dropbox-backed `hl.env()` values and D-Bus sync helper |
| Keybinding popup | `show-keybindings.lua` | done | Mock-loads `bindings.lua`; rofi UI; `--print` for headless validation |
| Look and feel | `looknfeel.lua` | done | `hl.config()`, `hl.curve()`, `hl.animation()` |
| Windows | `windows.lua` | done | Named `windowrule {}` → `hl.window_rule()` |
| Root config | `hyprland.lua` | done | Owns `require()` graph and monitor startup hook |

## Hypr ecosystem configs (separate tools)

These live in `hypr/` and are deployed with the desktop baseline, but they are **not** sourced by `hyprland.lua`. Upstream still uses tool-specific config files for these daemons; keep them separate unless a tool adds native Lua support.

| File | Tool | Status | Usage |
| --- | --- | --- | --- |
| `hypridle.conf` | hypridle | active (hyprlang) | Idle → lock → DPMS flow; typically enabled via distro `hypridle` user service |
| `hyprlock.conf` | hyprlock | active (hyprlang) | Lockscreen; invoked by `bindings.lua`, `hypridle.conf`, and `wlogout/layout` |
| `hyprpaper.conf` | hyprpaper | active (hyprpaper config) | Desktop wallpaper daemon; started by `autostart.lua` and points at `~/.config/wallpapers/desktop` |
| `hyprsunset.conf` | hyprsunset | active (hyprlang) | Optional display temperature; applies when `hyprsunset` is running |
| `xdph.conf` | xdg-desktop-portal-hyprland | active (hyprlang) | Portal screencopy settings; read by convention at `~/.config/hypr/xdph.conf` |

**Optional gap:** `xdph.conf` references `hyprland-preview-share-picker`, which is not installed by `bin/install-arch-pkgs.sh`. Install manually or remove the custom picker line if unavailable.

**Palette coupling:** `looknfeel.lua` does not auto-sync `hyprlock.conf` colors. Update lockscreen palette manually when changing the compositor theme.

## Root config

Active root config: [`hyprland.lua`](hyprland.lua)

```lua
require("monitors")
require("input")
require("bindings")
require("envs")
require("looknfeel")
require("windows")
require("autostart")

hl.on("hyprland.start", function()
  hl.exec_cmd("/etc/acpi/check-lid-on-startup.sh")
end)
hl.exec_cmd("/etc/acpi/check-lid-on-startup.sh")
```

Validate with:

```bash
hyprctl reload && hyprctl configerrors
```

## Per-fragment mapping reference

| Legacy hyprlang | Lua |
| --- | --- |
| `env = KEY,VALUE` | `hl.env("KEY", "VALUE")` |
| `exec-once = CMD` | `hl.on("hyprland.start", function() hl.exec_cmd("CMD") end)` |
| `exec = CMD` | `hl.exec_cmd("CMD")` |
| `monitor = OUT, MODE, POS, SCALE` | `hl.monitor({ output = "OUT", mode = "MODE", position = "POS", scale = "SCALE" })` |
| `section { key = val }` | `hl.config({ section = { key = val } })` |
| `bezier = name, ...` | `hl.curve("name", { type = "bezier", points = { ... } })` |
| `animation = leaf, ...` | `hl.animation({ leaf = "leaf", ... })` |
| `bindd = ...` | `hl.bind(..., hl.dsp.*, { description = "..." })` |
| `bindmd = ...` | `hl.bind(..., ..., { mouse = true, description = "..." })` |
| `bindeld = ...` | `hl.bind(..., ..., { locked = true, repeating = true, description = "..." })` |
| `bindld = ...` | `hl.bind(..., ..., { locked = true, description = "..." })` |
| `windowrule = match:class ..., effect` | `hl.window_rule({ match = { class = "..." }, effect = val })` |
| `gesture = N, dir, action` | `hl.gesture({ fingers = N, direction = "dir", action = "action" })` |
| `device { name = ..., ... }` | `hl.device({ name = "...", ... })` |
| `# hyprlang if VAR` | `if os.getenv("VAR") then ... end` |
| `source = file.conf` | `require("file")` (drop `.lua` extension) |

## Validation

Mock-load without a running Hyprland session:

```bash
# envs
lua -e 'package.path="hypr/?.lua;"..package.path; hl={env=function(k,v) print(k,v) end,exec_cmd=print}; dofile("hypr/envs.lua")'

# session-env
lua -e 'package.path="hypr/?.lua;"..package.path; local session_env=require("session-env"); local env=session_env.derive(); if env then for k,v in pairs(env) do print(k,v) end else print("no dropbox_dir") end'

# show-keybindings (headless)
hypr/show-keybindings.lua --print | head

# autostart
lua -e 'package.path="hypr/?.lua;"..package.path; hl={exec_cmd=print,env=function() end,on=function(_,f)f()end}; dofile("hypr/autostart.lua")'

# monitors
lua -e 'hl={env=function(k,v) print("env",k,v) end,monitor=function(t) for k,v in pairs(t) do print("monitor",k,v) end print("---") end}; dofile("hypr/monitors.lua")'

# input
lua -e 'hl={config=function() end,window_rule=function(t) print("window_rule",t.name) end,gesture=function(t) print("gesture",t.fingers,t.direction,t.action) end,device=function(t) print("device",t.name,t.sensitivity) end}; dofile("hypr/input.lua")'

# looknfeel
lua -e 'hl={config=function() end,curve=function(n) print("curve",n) end,animation=function(t) print("animation",t.leaf) end}; dofile("hypr/looknfeel.lua")'

# windows
lua -e 'hl={config=function() end,window_rule=function(t) print("window_rule",t.name) end}; dofile("hypr/windows.lua")'

# bindings
lua -e 'local function mk_fn() return function(...) return {} end end; local function mk_ns() return setmetatable({},{__index=function(t,k) t[k]=mk_fn(); return t[k] end}) end; hl={bind=function(k,d,o) print("bind",k,o and o.description or "") end,dsp=setmetatable({},{__index=function(t,k) if k=="window" or k=="group" or k=="workspace" then t[k]=mk_ns() else t[k]=mk_fn() end; return t[k] end})}; dofile("hypr/bindings.lua")'

# root (from hypr/ directory)
cd hypr && lua -e 'package.path="./?.lua;"..package.path; local function mk_fn() return function(...) return {} end end; local function mk_ns() return setmetatable({},{__index=function(t,k) t[k]=mk_fn(); return t[k] end}) end; hl={config=function()end,curve=function()end,animation=function()end,bind=function()end,window_rule=function()end,gesture=function()end,device=function()end,monitor=function()end,env=function()end,on=function(_,f)f()end,exec_cmd=print,dsp=setmetatable({},{__index=function(t,k) if k=="window" or k=="group" or k=="workspace" then t[k]=mk_ns() else t[k]=mk_fn() end; return t[k] end})}; dofile("hyprland.lua")'
```

Shell syntax:

```bash
bash -n bin/gpu-switch.sh hypr/*.sh monitor-auto-switch/scripts/*.sh
```

## Change checklist

When editing compositor fragments:

1. Update the matching `hypr/<name>.lua` file (runtime source of truth)
2. Update any scripts that patch marker blocks (e.g. `bin/gpu-switch.sh`)
3. Add or update mock-load commands above when new `hl.*` calls are introduced
4. Run `hyprctl reload && hyprctl configerrors` in a live session after changes
