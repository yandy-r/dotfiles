-- Extra env variables
-- hl.env("MY_GLOBAL_ENV", "setting")
local session_env = require("session-env")

hl.env("XCURSOR_SIZE", "24")
hl.env("HYPRCURSOR_SIZE", "24")
hl.env("GTK_THEME", "Adwaita-dark")

-- Dropbox-backed screenshot/recording paths from ~/.config/env/dropbox_dir
session_env.apply(hl)

-- Optional override for hypr/record-screen.sh. When unset, the helper tries
-- h264_nvenc first when ffmpeg supports it, then falls back to libx264 if
-- wf-recorder cannot initialize NVENC in the current process context.
-- hl.env("HYPR_WF_RECORDER_CODEC", "h264_nvenc")

-- Fix GTK 4.20 key sequence handling (breaks Ghostty keybind sequences without this)
hl.env("GTK_IM_MODULE", "simple")

-- >>> GPU CONFIG (managed by gpu-switch.sh — do not edit manually)
hl.env("NVD_BACKEND", "direct")
hl.env("LIBVA_DRIVER_NAME", "nvidia")
hl.env("VDPAU_DRIVER", "nvidia")
hl.env("__GLX_VENDOR_LIBRARY_NAME", "nvidia")
-- <<< GPU CONFIG
