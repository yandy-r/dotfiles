#!/usr/bin/env lua
-- Searchable keybinding popup sourced from bindings.lua (repo-owned binds).
local script_dir = debug.getinfo(1, "S").source:match("^@(.+)/[^/]+$") or "."

local function trim(value)
  return value:match("^%s*(.-)%s*$")
end

local function format_key(key)
  local parts = {}
  for raw_part in key:gmatch("[^+]+") do
    local part = trim(raw_part)
    part = part
      :gsub("^SUPER$", "Super")
      :gsub("^CTRL$", "Ctrl")
      :gsub("^ALT$", "Alt")
      :gsub("^SHIFT$", "Shift")
    table.insert(parts, part)
  end
  return table.concat(parts, "+")
end

local binds = {}

local function mk_fn()
  return function(...)
    return {}
  end
end

local function mk_ns()
  return setmetatable({}, {
    __index = function(table, key)
      table[key] = mk_fn()
      return table[key]
    end,
  })
end

hl = {
  bind = function(keys, _, options)
    table.insert(binds, {
      key = format_key(keys),
      description = options and options.description or "",
    })
  end,
  dsp = setmetatable({}, {
    __index = function(table, key)
      if key == "window" or key == "group" or key == "workspace" then
        table[key] = mk_ns()
      else
        table[key] = mk_fn()
      end
      return table[key]
    end,
  }),
  get_active_window = function()
    return nil
  end,
  dispatch = function() end,
}

package.path = script_dir .. "/?.lua;" .. package.path
dofile(script_dir .. "/bindings.lua")

table.sort(binds, function(a, b)
  if a.key == b.key then
    return a.description < b.description
  end
  return a.key < b.key
end)

local lines = {}
for _, bind in ipairs(binds) do
  table.insert(lines, bind.key .. "\t" .. bind.description)
end

local output = table.concat(lines, "\n")

if arg[1] == "--print" then
  io.write(output)
  if #output > 0 then
    io.write("\n")
  end
  os.exit(0)
end

local rofi = io.popen(
  [[rofi -dmenu -i \
    -p "Keybindings" \
    -font "JetBrainsMono Nerd Font 13" \
    -theme-str '
      * {
        bg:      #181c27e6;
        fg:      #f5f1e8;
        fg-muted: #b8c0cc;
        accent:  #f5d887;
        sel:     #1e2233;
      }
      window {
        background-color: @bg;
        border: 1px;
        border-color: #f5d88759;
        border-radius: 10px;
        width: 35%;
      }
      inputbar {
        padding: 8px 12px;
        background-color: transparent;
        text-color: @fg;
      }
      prompt {
        text-color: @accent;
      }
      entry {
        text-color: @fg;
        placeholder: "Type to filter...";
        placeholder-color: @fg-muted;
      }
      listview {
        lines: 16;
        padding: 4px 0;
        background-color: transparent;
      }
      element {
        padding: 4px 12px;
        background-color: transparent;
        text-color: #7a8194;
      }
      element selected {
        background-color: @sel;
        text-color: #b8c0cc;
      }
      element-text {
        tab-stops: [14em];
        highlight: bold #c4a95b;
      }
    ']],
  "w"
)

if not rofi then
  io.stderr:write("show-keybindings.lua: failed to launch rofi\n")
  os.exit(1)
end

local write_ok, write_err = rofi:write(output)
if not write_ok then
  io.stderr:write("show-keybindings.lua: failed to write to rofi: " .. tostring(write_err) .. "\n")
  rofi:close()
  os.exit(1)
end

local close_ok, close_reason, close_status = rofi:close()
if not close_ok then
  local detail = close_reason and (" (" .. tostring(close_reason) .. ": " .. tostring(close_status) .. ")") or ""
  io.stderr:write("show-keybindings.lua: rofi exited unsuccessfully" .. detail .. "\n")
  os.exit(1)
end
