-- Extra autostart processes
-- hl.on("hyprland.start", function() hl.exec_cmd("uwsm-app -- my-service") end)
local session_env = require("session-env")

local systemd_env = table.concat({
  "WAYLAND_DISPLAY",
  "DISPLAY",
  "XDG_CURRENT_DESKTOP",
  "XDG_SESSION_TYPE",
  "GTK_THEME",
  "GTK_IM_MODULE",
  "XCURSOR_SIZE",
  "HYPRCURSOR_SIZE",
  "NVD_BACKEND",
  "LIBVA_DRIVER_NAME",
  "VDPAU_DRIVER",
  "__GLX_VENDOR_LIBRARY_NAME",
}, " ")

local function sync_desktop_session(hl)
  hl.exec_cmd("dbus-update-activation-environment --systemd " .. systemd_env)
end

local function start_user_services(hl)
  hl.exec_cmd("systemctl --user start xdg-desktop-portal-hyprland.service xdg-desktop-portal-gtk.service arch-update-tray.service")
  hl.exec_cmd([[sh -c 'pgrep -f "^/usr/lib/xdg-desktop-portal$" >/dev/null 2>&1 || exec /usr/lib/xdg-desktop-portal']])
end

-- Export the Wayland session environment to D-Bus-activated apps such as
-- GNOME Sushi (Nautilus Quick Look) so they don't fall back to Mesa/Zink.
hl.on("hyprland.start", function()
  -- >>> GPU DBUS (managed by gpu-switch.sh — do not edit manually)
  sync_desktop_session(hl)
  -- <<< GPU DBUS
  session_env.apply(hl)
  session_env.sync_dbus(hl)
  start_user_services(hl)
  hl.exec_cmd("systemctl --user enable --now hyprpolkitagent.service")
  hl.exec_cmd([[sh -lc 'pgrep -x hyprpaper >/dev/null 2>&1 || exec hyprpaper']])
  hl.exec_cmd("waybar")
  -- Start Mako on initial login, and also recover it on Hypr reloads.
  hl.exec_cmd("/usr/bin/mako")
  -- Start SwayOSD on initial login, and also recover it on Hypr reloads.
  hl.exec_cmd("/usr/bin/swayosd-server")
  hl.exec_cmd("dropbox")
  -- Tell GTK/portal apps to use dark color scheme.
  -- gsettings persists in dconf, but re-applying on login is
  -- harmless and ensures fresh bootstraps inherit the preference.
  hl.exec_cmd("gsettings set org.gnome.desktop.interface color-scheme 'prefer-dark'")
  hl.exec_cmd("gsettings set org.gnome.desktop.interface gtk-theme 'Adwaita-dark'")
end)

-- Re-run on every config reload (legacy `exec = ...` semantics).
sync_desktop_session(hl)
session_env.apply(hl)
session_env.sync_dbus(hl)
start_user_services(hl)
hl.exec_cmd([[sh -lc 'pgrep -x mako >/dev/null 2>&1 || exec /usr/bin/mako']])
hl.exec_cmd([[sh -lc 'pgrep -x swayosd-server >/dev/null 2>&1 || exec /usr/bin/swayosd-server']])
hl.exec_cmd([[sh -lc 'pgrep -x hyprpaper >/dev/null 2>&1 || exec hyprpaper']])
