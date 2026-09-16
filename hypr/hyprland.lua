-- Repo-owned Hyprland configuration graph (Hyprland 0.55+ Lua root).
-- Hyprland 0.55+ loads this file as the compositor root config.

require("monitors")
require("input")
require("bindings")
require("envs")
require("looknfeel")
require("windows")
require("autostart")

-- The root Hypr config owns monitor-auto-switch startup hook inclusion.
hl.on("hyprland.start", function()
  hl.exec_cmd("/etc/acpi/check-lid-on-startup.sh")
end)
hl.exec_cmd("/etc/acpi/check-lid-on-startup.sh")
