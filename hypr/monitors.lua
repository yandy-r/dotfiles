-- See https://wiki.hypr.land/Configuring/Basics/Monitors/
-- List current monitors and resolutions possible: hyprctl monitors
-- Keep this fallback file in sync with monitor-auto-switch/acpi/hypr-utils.sh.
-- Acceptance criteria for changes here:
-- - monitor names, resolutions, positions, and scale match the hypr-utils defaults
-- - dual-monitor (laptop): external display left, laptop right
-- - dual/triple-monitor (desktop): HDMI-A-1 left, primary DP-2 center, (laptop right if present)
-- - workspace defaults stay aligned with the runtime monitor script (external 1-9, laptop none)

-- Optimized for retina-class 2x displays, like 13" 2.8K, 27" 5K, 32" 6K.
hl.env("GDK_SCALE", "1")

-- Human-readable fallback/default layout.
-- Runtime scripts in /etc/acpi/hypr-utils.sh apply the same values dynamically
-- on startup and lid events, but this file remains the checked-in source of truth
-- for names, positions, scale, and expected workspace ownership.
-- Desktop dual 4K: HDMI-A-1 landscape left of primary DP-2
-- Previous portrait layout (uncomment if rotated):
-- hl.monitor({ output = "HDMI-A-1", mode = "3840x2160@60", position = "-2160x0", scale = "1", transform = 3 })

-- -----------------------------------------------------------------
-- Testing resolution for responsive applications
-- hl.monitor({ output = "HDMI-A-1", mode = "1280x720@60", position = "-3840x0", scale = "1" })
-- -----------------------------------------------------------------

hl.monitor({ output = "HDMI-A-1", mode = "3840x2160@60", position = "-3840x0", scale = "1" })
hl.monitor({ output = "DP-2", mode = "3840x2160@180", position = "0x0", scale = "1" })
hl.monitor({ output = "DP-3", mode = "3840x2160@180", position = "0x0", scale = "1" })
hl.monitor({ output = "DP-4", mode = "3840x2160@180", position = "-3840x0", scale = "1" })
-- hl.monitor({ output = "DP-5", mode = "3840x2160@180", position = "0x0", scale = "1" })
hl.monitor({ output = "eDP-1", mode = "3456x2160@60", position = "3840x0", scale = "1.125" })
-- Catch-all for any unrecognized monitor (e.g. different port names); preferred resolution, auto position.
-- Scale 1.125 matches CachyOS default for single-monitor / laptop.
hl.monitor({ output = "", mode = "preferred", position = "auto", scale = "1" })

-- Laptop-only fallback keeps all workspaces local when no external monitor is present.
-- The runtime monitor script owns that dynamic state; do not try to encode it here.

-- LG 27G810A-B (UltraGear G8, 27" 4K IPS, native 10-bit, DisplayHDR 400)
-- 180 Hz over DP 1.4 w/ DSC; HDMI 2.1 also supported.
-- FreeSync Premium / G-SYNC Compatible, VRR range 48-180 Hz.
-- After connecting, run `hyprctl monitors` to find the actual port name,
-- then uncomment and adjust the line below.
-- hl.monitor({ output = "<PORT>", mode = "3840x2160@180", position = "auto", scale = "1" })
-- Optional: set bitdepth = 10 for native 10-bit color output.
-- Note: dual-mode (FHD@360 Hz) requires an OSD toggle and may not
-- re-register cleanly on Wayland — stick with 4K mode on Linux.

-- Example for Framework 13 w/ 6K XDR Apple display
-- hl.monitor({ output = "DP-5", mode = "6016x3384@60", position = "auto", scale = "2" })
-- hl.monitor({ output = "eDP-1", mode = "2880x1920@120", position = "auto", scale = "2" })
