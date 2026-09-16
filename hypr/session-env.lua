-- Dropbox-backed session paths for Hyprland screenshot and recording helpers.
local M = {}

local function config_home()
  local xdg = os.getenv("XDG_CONFIG_HOME")
  if xdg and xdg ~= "" then
    return xdg
  end
  local home = os.getenv("HOME")
  if not home or home == "" then
    return nil
  end
  return home .. "/.config"
end

local function read_dropbox_dir()
  local base = config_home()
  if not base then
    return nil
  end

  local path = base .. "/env/dropbox_dir"
  local file = io.open(path, "r")
  if not file then
    return nil
  end

  local line = file:read("l")
  file:close()

  if not line then
    return nil
  end

  line = line:gsub("\r$", "")
  if line == "" then
    return nil
  end

  return line
end

function M.derive()
  local dropbox_dir = read_dropbox_dir()
  if not dropbox_dir then
    return nil
  end

  return {
    DROPBOX_DIR = dropbox_dir,
    HYPR_SCREENSHOT_DIR = dropbox_dir .. "/Screenshots",
    HYPR_SCREENRECORD_DIR = dropbox_dir .. "/Recordings",
  }
end

function M.apply(hl)
  local env = M.derive()
  if not env then
    return false
  end

  hl.env("DROPBOX_DIR", env.DROPBOX_DIR)
  hl.env("HYPR_SCREENSHOT_DIR", env.HYPR_SCREENSHOT_DIR)
  hl.env("HYPR_SCREENRECORD_DIR", env.HYPR_SCREENRECORD_DIR)
  return true
end

function M.sync_dbus(hl)
  if not M.derive() then
    return
  end

  hl.exec_cmd(
    "dbus-update-activation-environment --systemd DROPBOX_DIR HYPR_SCREENSHOT_DIR HYPR_SCREENRECORD_DIR"
  )
end

return M
