-- Window rules and workspace behavior (merged from CachyOS baseline)
-- Note: workspace_swipe is enabled via gesture in input.lua

hl.config({
  misc = {
    middle_click_paste = false,
    focus_on_activate = true,
  },
})

hl.window_rule({
  name = "suppress-maximize-events",
  match = { class = ".*" },
  suppress_event = "maximize",
})

hl.window_rule({
  name = "fix-xwayland-drags",
  match = {
    class = "^$",
    title = "^$",
    xwayland = true,
    float = true,
    fullscreen = false,
    pin = false,
  },
  no_focus = true,
})

hl.window_rule({
  name = "move-hyprland-run",
  match = { class = "hyprland-run" },
  move = "20 monitor_h-120",
  float = true,
})

hl.window_rule({
  name = "float-dolphin",
  match = { class = ".*dolphin.*" },
  float = true,
  size = "(monitor_w*0.492) (monitor_h*0.492)",
  center = true,
})

hl.window_rule({
  name = "float-nautilus",
  match = { class = "org.gnome.Nautilus" },
  float = true,
  size = "(monitor_w*0.492) (monitor_h*0.492)",
  center = true,
})

hl.window_rule({
  name = "float-sushi-preview",
  match = { class = "org.gnome.NautilusPreviewer" },
  float = true,
  center = true,
})

hl.window_rule({
  name = "float-1password",
  match = { class = "^[1][Pp]assword$" },
  float = true,
})

hl.window_rule({
  name = "float-gedit",
  match = { class = "org.gnome.gedit" },
  float = true,
  size = "(monitor_w*0.4) (monitor_h*0.65)",
  center = true,
})

hl.window_rule({
  name = "float-qalculate",
  match = { class = "^qalculate-gtk$" },
  float = true,
  size = "(monitor_w*0.3) (monitor_h*0.5)",
  center = true,
})

hl.window_rule({
  name = "float-satty",
  match = { class = "com.gabm.satty" },
  float = true,
  size = "(monitor_w*0.75) (monitor_h*0.75)",
  center = true,
})

hl.window_rule({
  name = "float-arch-update",
  match = { class = "arch-update" },
  float = true,
  size = "(monitor_w*0.45) (monitor_h*0.55)",
  center = true,
})

hl.window_rule({
  name = "float-cachyos-hello",
  match = { class = "org.cachyos.hello" },
  float = true,
  size = "(monitor_w*0.45) (monitor_h*0.55)",
  center = true,
})

hl.window_rule({
  name = "float-cachyos-package-manager",
  match = { class = "org.cachyos.cachyos-pi" },
  float = true,
  size = "(monitor_w*0.45) (monitor_h*0.55)",
  center = true,
})

hl.window_rule({
  name = "float-octopi",
  match = { class = "octopi" },
  float = true,
  size = "(monitor_w*0.45) (monitor_h*0.55)",
  center = true,
})

hl.window_rule({
  name = "float-cachyos-kernel-manager",
  match = { class = "org.cachyos.KernelManager" },
  float = true,
  size = "(monitor_w*0.45) (monitor_h*0.55)",
  center = true,
})

hl.window_rule({
  name = "float-alacritty",
  match = { class = "alacritty-float" },
  float = true,
  size = "(monitor_w*0.45) (monitor_h*0.55)",
  center = true,
})

hl.window_rule({
  name = "float-fuzzel-terminal",
  match = { class = "com.fuzzel.terminal" },
  float = true,
  size = "(monitor_w*0.45) (monitor_h*0.55)",
  center = true,
})

hl.window_rule({
  name = "float-wifi-tui",
  match = { class = "wifi-tui" },
  float = true,
  size = "(monitor_w*0.35) (monitor_h*0.35)",
  center = true,
})

hl.window_rule({
  name = "float-btop",
  match = { class = "btop" },
  float = true,
  size = "(monitor_w*0.45) (monitor_h*0.55)",
  center = true,
})

hl.window_rule({
  name = "float-lazydocker",
  match = { class = "lazydocker" },
  float = true,
  size = "(monitor_w*0.65) (monitor_h*0.65)",
  center = true,
})

hl.window_rule({
  name = "float-claude-code",
  match = { class = "claude-code" },
  float = true,
  size = "(monitor_w*0.55) (monitor_h*0.65)",
  center = true,
})

hl.window_rule({
  name = "float-simplenote",
  match = { class = "Simplenote" },
  float = true,
  size = "(monitor_w*0.4) (monitor_h*0.5)",
  center = true,
})
