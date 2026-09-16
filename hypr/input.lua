-- Control your input devices
-- See https://wiki.hypr.land/Configuring/Basics/Variables/#input
hl.config({
  input = {
    kb_layout = "us",
    kb_options = "compose:caps",

    repeat_rate = 40,
    repeat_delay = 600,
    numlock_by_default = true,

    follow_mouse = 1,
    sensitivity = 0,
    natural_scroll = true,

    touchpad = {
      natural_scroll = true,
      clickfinger_behavior = true,
      scroll_factor = 0.4,
    },
  },
})

-- Scroll nicely in the terminal
hl.window_rule({
  name = "scroll-touchpad-alacritty-kitty",
  match = { class = "(Alacritty|kitty)" },
  scroll_touchpad = 1.5,
})

hl.window_rule({
  name = "scroll-touchpad-ghostty",
  match = { class = "com.mitchellh.ghostty" },
  scroll_touchpad = 0.2,
})

-- Swipe gesture: 3 fingers horizontal = change workspace
hl.gesture({
  fingers = 3,
  direction = "horizontal",
  action = "workspace",
})

-- Per-device config (e.g. mouse sensitivity)
hl.device({
  name = "epic-mouse-v1",
  sensitivity = -0.5,
})
