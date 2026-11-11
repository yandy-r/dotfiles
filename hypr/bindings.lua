local mainMod = "SUPER"
local terminal = "ghostty"
local terminalAlt = "wezterm start --always-new-process"
local fileManager = "nautilus"
local launcher = "fuzzel"

local function bindd(keys, description, dispatcher)
  hl.bind(keys, dispatcher, { description = description })
end

local function bindmd(keys, description, dispatcher)
  hl.bind(keys, dispatcher, { mouse = true, description = description })
end

local function bindeld(keys, description, dispatcher)
  hl.bind(keys, dispatcher, { locked = true, repeating = true, description = description })
end

local function bindld(keys, description, dispatcher)
  hl.bind(keys, dispatcher, { locked = true, description = description })
end

-- Swap tiled windows, or reorder group tab order when the active window is grouped
local function swapOrReorder(direction, forward)
  return function()
    local window = hl.get_active_window()
    if window and window.group then
      hl.dispatch(hl.dsp.group.move_window({ forward = forward }))
      return
    end

    hl.dispatch(hl.dsp.window.swap({ direction = direction }))
  end
end

-- Core
bindd(mainMod .. " + RETURN", "Terminal (enter)", hl.dsp.exec_cmd(terminal))
bindd(mainMod .. " + Q", "Close window", hl.dsp.window.close())
bindd(mainMod .. " + SHIFT + Q", "Force-kill window (SIGKILL)", hl.dsp.window.kill())
bindd(mainMod .. " + M", "Logout menu", hl.dsp.exec_cmd("wlogout"))
bindd(mainMod .. " + E", "File manager", hl.dsp.exec_cmd(fileManager))
bindd(mainMod .. " + V", "Toggle floating", hl.dsp.window.float({ action = "toggle" }))
bindd(mainMod .. " + R", "Launcher", hl.dsp.exec_cmd(launcher))
bindd(mainMod .. " + SPACE", "Launcher (space)", hl.dsp.exec_cmd(launcher))
bindd(
  mainMod .. " + equal",
  "Calculator (rofi-calc)",
  hl.dsp.exec_cmd("rofi -show calc -modi calc -no-show-match -no-sort")
)
bindd(mainMod .. " + SHIFT + equal", "Calculator app", hl.dsp.exec_cmd("qalculate-gtk"))
bindd(mainMod .. " + CTRL + equal", "Calculator copy prompt", hl.dsp.exec_cmd("~/.config/hypr/calc-copy.sh"))
bindd(mainMod .. " + P", "Pseudo-tile", hl.dsp.window.pseudo())
bindd(mainMod .. " + T", "Toggle split", hl.dsp.layout("togglesplit"))

-- Toggle maximize (keep waybar/gaps visible)
bindd(
  mainMod .. " + ALT + F",
  "Toggle maximize",
  hl.dsp.window.fullscreen({ mode = "maximized", action = "toggle" })
)

-- Move focus with mainMod + arrow keys
bindd(mainMod .. " + left", "Focus left", hl.dsp.focus({ direction = "l" }))
bindd(mainMod .. " + H", "Focus left", hl.dsp.focus({ direction = "l" }))
bindd(mainMod .. " + right", "Focus right", hl.dsp.focus({ direction = "r" }))
bindd(mainMod .. " + L", "Focus right", hl.dsp.focus({ direction = "r" }))
bindd(mainMod .. " + up", "Focus up", hl.dsp.focus({ direction = "u" }))
bindd(mainMod .. " + K", "Focus up", hl.dsp.focus({ direction = "u" }))
bindd(mainMod .. " + down", "Focus down", hl.dsp.focus({ direction = "d" }))
bindd(mainMod .. " + J", "Focus down", hl.dsp.focus({ direction = "d" }))

-- Swap window position; reorder tabs when the active window is grouped
bindd(mainMod .. " + SHIFT + left", "Swap/group-tab left", swapOrReorder("l", false))
bindd(mainMod .. " + SHIFT + right", "Swap/group-tab right", swapOrReorder("r", true))
bindd(mainMod .. " + SHIFT + up", "Swap/group-tab up", swapOrReorder("u", false))
bindd(mainMod .. " + SHIFT + down", "Swap/group-tab down", swapOrReorder("d", true))
bindd(mainMod .. " + SHIFT + H", "Swap/group-tab left", swapOrReorder("l", false))
bindd(mainMod .. " + SHIFT + L", "Swap/group-tab right", swapOrReorder("r", true))
bindd(mainMod .. " + SHIFT + K", "Swap/group-tab up", swapOrReorder("u", false))
bindd(mainMod .. " + SHIFT + J", "Swap/group-tab down", swapOrReorder("d", true))

-- Move window to adjacent monitor
bindd(mainMod .. " + CTRL + SHIFT + left", "Move window to monitor left", hl.dsp.window.move({ monitor = "l" }))
bindd(mainMod .. " + CTRL + SHIFT + right", "Move window to monitor right", hl.dsp.window.move({ monitor = "r" }))
bindd(mainMod .. " + CTRL + SHIFT + H", "Move window to monitor left", hl.dsp.window.move({ monitor = "l" }))
bindd(mainMod .. " + CTRL + SHIFT + L", "Move window to monitor right", hl.dsp.window.move({ monitor = "r" }))

-- Window groups (Omarchy-inspired)
bindd(mainMod .. " + G", "Toggle group", hl.dsp.group.toggle())
bindd(mainMod .. " + ALT + G", "Move out of group", hl.dsp.window.move({ out_of_group = true }))
bindd(mainMod .. " + ALT + left", "Move into group left", hl.dsp.window.move({ into_group = "l" }))
bindd(mainMod .. " + ALT + H", "Move into group left", hl.dsp.window.move({ into_group = "l" }))
bindd(mainMod .. " + ALT + right", "Move into group right", hl.dsp.window.move({ into_group = "r" }))
bindd(mainMod .. " + ALT + L", "Move into group right", hl.dsp.window.move({ into_group = "r" }))
bindd(mainMod .. " + ALT + up", "Move into group up", hl.dsp.window.move({ into_group = "u" }))
bindd(mainMod .. " + ALT + K", "Move into group up", hl.dsp.window.move({ into_group = "u" }))
bindd(mainMod .. " + ALT + down", "Move into group down", hl.dsp.window.move({ into_group = "d" }))
bindd(mainMod .. " + ALT + J", "Move into group down", hl.dsp.window.move({ into_group = "d" }))
bindd(mainMod .. " + ALT + Tab", "Next group tab", hl.dsp.group.next())
bindd(mainMod .. " + ALT + SHIFT + Tab", "Prev group tab", hl.dsp.group.prev())
bindd(mainMod .. " + CTRL + G", "Lock group", hl.dsp.group.lock_active({ action = "toggle" }))

-- Cycle group tabs
bindd(mainMod .. " + SHIFT + bracketleft", "Cycle group tab left", hl.dsp.group.prev())
bindd(mainMod .. " + SHIFT + bracketright", "Cycle group tab right", hl.dsp.group.next())

-- Move current workspace to adjacent monitor
bindd(mainMod .. " + CTRL + left", "Move workspace to monitor left", hl.dsp.workspace.move({ monitor = "l" }))
bindd(mainMod .. " + CTRL + right", "Move workspace to monitor right", hl.dsp.workspace.move({ monitor = "r" }))
bindd(mainMod .. " + CTRL + H", "Move workspace to monitor left", hl.dsp.workspace.move({ monitor = "l" }))
bindd(mainMod .. " + CTRL + L", "Move workspace to monitor right", hl.dsp.workspace.move({ monitor = "r" }))
bindd(mainMod .. " + ALT + 1", "Group tab 1", hl.dsp.group.active({ index = 1 }))
bindd(mainMod .. " + ALT + 2", "Group tab 2", hl.dsp.group.active({ index = 2 }))
bindd(mainMod .. " + ALT + 3", "Group tab 3", hl.dsp.group.active({ index = 3 }))
bindd(mainMod .. " + ALT + 4", "Group tab 4", hl.dsp.group.active({ index = 4 }))
bindd(mainMod .. " + ALT + 5", "Group tab 5", hl.dsp.group.active({ index = 5 }))
bindd(mainMod .. " + ALT + 6", "Group tab 6", hl.dsp.group.active({ index = 6 }))
bindd(mainMod .. " + ALT + 7", "Group tab 7", hl.dsp.group.active({ index = 7 }))
bindd(mainMod .. " + ALT + 8", "Group tab 8", hl.dsp.group.active({ index = 8 }))
bindd(mainMod .. " + ALT + 9", "Group tab 9", hl.dsp.group.active({ index = 9 }))

-- Workspaces 1–10
for i = 1, 10 do
  local key = i % 10
  bindd(mainMod .. " + " .. key, "Workspace " .. i, hl.dsp.focus({ workspace = i }))
  bindd(mainMod .. " + SHIFT + " .. key, "Move to workspace " .. i, hl.dsp.window.move({ workspace = i }))
end

-- Special workspace (scratchpad)
bindd(mainMod .. " + S", "Scratchpad", hl.dsp.workspace.toggle_special("magic"))
bindd(mainMod .. " + SHIFT + S", "Move to scratchpad", hl.dsp.window.move({ workspace = "special:magic" }))

-- Scroll workspaces with mainMod + scroll
bindd(mainMod .. " + mouse_down", "Next workspace (scroll)", hl.dsp.focus({ workspace = "e+1" }))
bindd(mainMod .. " + mouse_up", "Prev workspace (scroll)", hl.dsp.focus({ workspace = "e-1" }))

-- Move/resize windows
bindmd(mainMod .. " + mouse:272", "Move window (drag)", hl.dsp.window.drag())
bindmd(mainMod .. " + mouse:273", "Resize window (drag)", hl.dsp.window.resize())

-- Laptop multimedia: volume and brightness
bindeld(
  "XF86AudioRaiseVolume",
  "Volume up",
  hl.dsp.exec_cmd("/usr/bin/swayosd-client --output-volume +5 --max-volume 100")
)
bindeld(
  "XF86AudioLowerVolume",
  "Volume down",
  hl.dsp.exec_cmd("/usr/bin/swayosd-client --output-volume -5 --max-volume 100")
)
bindeld("XF86AudioMute", "Toggle mute", hl.dsp.exec_cmd("/usr/bin/swayosd-client --output-volume mute-toggle"))
bindeld("XF86AudioMicMute", "Toggle mic mute", hl.dsp.exec_cmd("/usr/bin/swayosd-client --input-volume mute-toggle"))
bindeld("XF86MonBrightnessUp", "Brightness up", hl.dsp.exec_cmd("/usr/bin/swayosd-client --brightness +5"))
bindeld("XF86MonBrightnessDown", "Brightness down", hl.dsp.exec_cmd("/usr/bin/swayosd-client --brightness -5"))

-- Media (requires playerctl)
bindld("XF86AudioNext", "Media next", hl.dsp.exec_cmd("playerctl next"))
bindld("XF86AudioPause", "Media pause", hl.dsp.exec_cmd("playerctl play-pause"))
bindld("XF86AudioPlay", "Media play", hl.dsp.exec_cmd("playerctl play-pause"))
bindld("XF86AudioPrev", "Media previous", hl.dsp.exec_cmd("playerctl previous"))

-- Application shortcuts (repo)
bindd("SUPER + SHIFT + RETURN", "Secondary terminal", hl.dsp.exec_cmd(terminalAlt))
bindd("SUPER + ALT + RETURN", "Floating terminal (alacritty)", hl.dsp.exec_cmd("alacritty --class alacritty-float"))
bindd("SUPER + SHIFT + F", "File manager", hl.dsp.exec_cmd(fileManager))
bindd("SUPER + SHIFT + B", "Browser", hl.dsp.exec_cmd("google-chrome-stable"))
bindd("SUPER + SHIFT + ALT + B", "Browser (private)", hl.dsp.exec_cmd("google-chrome-stable --incognito"))
bindd(
  "SUPER + SHIFT + N",
  "Editor",
  hl.dsp.exec_cmd([[sh -lc 'exec alacritty --class alacritty-float -e ${EDITOR:-nvim}']])
)
bindd("SUPER + SHIFT + T", "Activity", hl.dsp.exec_cmd([[sh -lc 'exec alacritty --class btop -e btop']]))
bindd(
  "SUPER + SHIFT + D",
  "Docker",
  hl.dsp.exec_cmd([[sh -lc 'exec alacritty --class lazydocker -e lazydocker']])
)
bindd(
  "SUPER + SHIFT + O",
  "Wi-Fi (NetworkManager)",
  hl.dsp.exec_cmd([[sh -lc 'exec alacritty --class wifi-tui -e "$HOME/.bin/wifi-tui.sh"']])
)
bindd("SUPER + SHIFT + SLASH", "Passwords", hl.dsp.exec_cmd("1password"))
bindd("SUPER + SHIFT + C", "Cursor editor", hl.dsp.exec_cmd("cursor"))
bindd(
  "SUPER + SHIFT + A",
  "Claude Code (AI)",
  hl.dsp.exec_cmd([[sh -lc 'exec alacritty --class claude-code -e claude']])
)

-- Reload waybar
bindd("SUPER + SHIFT + R", "Reload waybar", hl.dsp.exec_cmd("pkill -SIGUSR2 waybar"))

-- Keybinding cheatsheet
bindd("SUPER + F1", "Keybinding cheatsheet", hl.dsp.exec_cmd("~/.config/hypr/show-keybindings.lua"))

-- Lock screen
bindd(mainMod .. " + CTRL + Q", "Lock screen", hl.dsp.exec_cmd("hyprlock"))

-- Screenshot and recording
bindd("CTRL + SHIFT + 4", "Screenshot region + edit", hl.dsp.exec_cmd("~/.config/hypr/screenshot.sh region"))
bindd(
  "CTRL + SHIFT + 5",
  "Screenshot region (quick save + clipboard)",
  hl.dsp.exec_cmd("~/.config/hypr/screenshot.sh quick-region")
)
bindd("CTRL + SHIFT + 6", "Screenshot screen + edit", hl.dsp.exec_cmd("~/.config/hypr/screenshot.sh screen"))
bindd("CTRL + SHIFT + 7", "Toggle screen recording", hl.dsp.exec_cmd("~/.config/hypr/record-screen.sh screen"))
bindd("CTRL + SHIFT + 8", "Toggle region recording", hl.dsp.exec_cmd("~/.config/hypr/record-screen.sh region"))

-- Screenshot + Edit (Print key variants for keyboards that have it)
bindd("Print", "Screenshot region + edit", hl.dsp.exec_cmd("~/.config/hypr/screenshot.sh region"))
bindd("SHIFT + Print", "Screenshot screen + edit", hl.dsp.exec_cmd("~/.config/hypr/screenshot.sh screen"))
bindd("CTRL + Print", "Screenshot window + edit", hl.dsp.exec_cmd("~/.config/hypr/screenshot.sh window"))
