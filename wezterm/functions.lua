-- ================================================================
-- WezTerm Utility Functions
-- ================================================================
-- Common utility functions used across WezTerm configuration modules

local wezterm = require('wezterm')
local M = {}

-- ================================================================
-- ENVIRONMENT & DETECTION
-- ================================================================

-- Detect operating system
function M.is_macos()
    return wezterm.target_triple:find("apple") ~= nil
end

function M.is_linux()
    return wezterm.target_triple:find("linux") ~= nil
end

function M.is_windows()
    return wezterm.target_triple:find("windows") ~= nil
end

-- Get home directory path
function M.get_home_dir()
    if M.is_windows() then
        return os.getenv('USERPROFILE') or 'C:\\'
    else
        return os.getenv('HOME') or '/tmp'
    end
end

-- Get config directory path
function M.get_config_dir()
    return wezterm.config_dir or M.get_home_dir() .. '/.config/wezterm'
end

-- ================================================================
-- FILE & PATH UTILITIES
-- ================================================================

-- Check if file exists
function M.file_exists(path)
    local file = io.open(path, 'r')
    if file then
        file:close()
        return true
    end
    return false
end

-- Get files in directory with pattern matching
function M.get_files(directory, pattern)
    local files = {}
    local handle
    
    if M.is_windows() then
        handle = io.popen('dir "' .. directory .. '" /b')
    else
        handle = io.popen('ls "' .. directory .. '"')
    end
    
    if handle then
        for file in handle:lines() do
            if not pattern or file:match(pattern) then
                table.insert(files, directory .. '/' .. file)
            end
        end
        handle:close()
    end
    
    return files
end

-- ================================================================
-- TIME & DATE UTILITIES
-- ================================================================

-- Get current time in 24-hour format
function M.get_current_time()
    return os.date('%H:%M')
end

-- Get current date
function M.get_current_date()
    return os.date('%Y-%m-%d')
end

-- Check if current time is during day hours (6 AM - 6 PM)
function M.is_daytime()
    local hour = tonumber(os.date('%H'))
    return hour >= 6 and hour < 18
end

-- ================================================================
-- STRING UTILITIES
-- ================================================================

-- Truncate string to max length with ellipsis
function M.truncate_string(str, max_length)
    if #str <= max_length then
        return str
    end
    return str:sub(1, max_length - 3) .. '...'
end

-- Split string by delimiter
function M.split_string(str, delimiter)
    local result = {}
    local pattern = '([^' .. delimiter .. ']+)'
    
    for match in str:gmatch(pattern) do
        table.insert(result, match)
    end
    
    return result
end

-- Trim whitespace from string
function M.trim_string(str)
    return str:match('^%s*(.-)%s*$')
end

-- ================================================================
-- TABLE UTILITIES
-- ================================================================

-- Merge two tables (shallow merge)
function M.merge_tables(t1, t2)
    local result = {}
    
    -- Copy first table
    for k, v in pairs(t1) do
        result[k] = v
    end
    
    -- Copy second table (overwrites duplicates)
    for k, v in pairs(t2) do
        result[k] = v
    end
    
    return result
end

-- Deep copy a table
function M.deep_copy(obj)
    if type(obj) ~= 'table' then
        return obj
    end
    
    local copy = {}
    for key, value in pairs(obj) do
        copy[M.deep_copy(key)] = M.deep_copy(value)
    end
    
    return copy
end

-- Check if table contains value
function M.table_contains(table, value)
    for _, v in pairs(table) do
        if v == value then
            return true
        end
    end
    return false
end

-- ================================================================
-- COLOR UTILITIES
-- ================================================================

-- Convert hex color to RGB
function M.hex_to_rgb(hex)
    hex = hex:gsub('#', '')
    
    if #hex == 6 then
        local r = tonumber(hex:sub(1, 2), 16) / 255
        local g = tonumber(hex:sub(3, 4), 16) / 255
        local b = tonumber(hex:sub(5, 6), 16) / 255
        return { r = r, g = g, b = b }
    end
    
    return { r = 0, g = 0, b = 0 }
end

-- Convert RGB to hex
function M.rgb_to_hex(r, g, b)
    return string.format('#%02x%02x%02x', 
        math.floor(r * 255), 
        math.floor(g * 255), 
        math.floor(b * 255)
    )
end

-- Blend two colors
function M.blend_colors(color1, color2, ratio)
    local rgb1 = M.hex_to_rgb(color1)
    local rgb2 = M.hex_to_rgb(color2)
    
    local r = rgb1.r * (1 - ratio) + rgb2.r * ratio
    local g = rgb1.g * (1 - ratio) + rgb2.g * ratio
    local b = rgb1.b * (1 - ratio) + rgb2.b * ratio
    
    return M.rgb_to_hex(r, g, b)
end

-- ================================================================
-- ENVIRONMENT VARIABLES
-- ================================================================

-- Get environment variable with default
function M.get_env(var_name, default_value)
    return os.getenv(var_name) or default_value
end

-- Get boolean environment variable
function M.get_env_bool(var_name, default_value)
    local value = os.getenv(var_name)
    if value == nil then
        return default_value
    end
    
    value = value:lower()
    return value == 'true' or value == '1' or value == 'yes' or value == 'on'
end

-- Get numeric environment variable
function M.get_env_number(var_name, default_value)
    local value = os.getenv(var_name)
    if value == nil then
        return default_value
    end
    
    local number = tonumber(value)
    return number or default_value
end

-- ================================================================
-- LOGGING UTILITIES
-- ================================================================

-- Simple logging function
function M.log(level, message)
    local timestamp = os.date('%Y-%m-%d %H:%M:%S')
    local log_message = string.format('[%s] [%s] %s', timestamp, level:upper(), message)
    
    -- In WezTerm, we can use wezterm.log_info, wezterm.log_warn, wezterm.log_error
    if level:lower() == 'error' then
        wezterm.log_error(log_message)
    elseif level:lower() == 'warn' then
        wezterm.log_warn(log_message)
    else
        wezterm.log_info(log_message)
    end
end

-- ================================================================
-- PROCESS UTILITIES
-- ================================================================

-- Execute command and get output
function M.execute_command(command)
    local handle = io.popen(command)
    if not handle then
        return nil, 'Failed to execute command'
    end
    
    local result = handle:read('*a')
    local success = handle:close()
    
    if success then
        return M.trim_string(result), nil
    else
        return nil, 'Command execution failed'
    end
end

-- Check if command exists
function M.command_exists(command)
    local check_cmd
    if M.is_windows() then
        check_cmd = 'where ' .. command .. ' >nul 2>&1'
    else
        check_cmd = 'command -v ' .. command .. ' >/dev/null 2>&1'
    end
    
    return os.execute(check_cmd) == 0
end

-- ================================================================
-- TAB UTILITIES
-- ================================================================

-- Get tab title with fallback
function M.get_tab_title(tab_info)
    local title = tab_info.tab_title
    
    -- If the tab title is explicitly set, use that
    if title and #title > 0 then
        return title
    end
    
    -- Otherwise, use the title from the active pane
    local pane_title = tab_info.active_pane.title
    if pane_title and #pane_title > 0 then
        return pane_title
    end
    
    -- Fallback to process name or generic title
    return tab_info.active_pane.foreground_process_name or 'Terminal'
end

-- Format tab title with truncation
function M.format_tab_title(title, max_width, show_index, tab_index)
    local formatted_title = title
    
    -- Add tab index if requested
    if show_index then
        formatted_title = string.format('%d: %s', tab_index + 1, title)
    end
    
    -- Truncate if too long
    return M.truncate_string(formatted_title, max_width - 2)
end

-- ================================================================
-- KEYBOARD UTILITIES
-- ================================================================

-- Create a key binding table
function M.create_key_binding(key, mods, action, description)
    return {
        key = key,
        mods = mods,
        action = action,
        description = description or ''
    }
end

-- Merge multiple key binding tables
function M.merge_key_bindings(...)
    local result = {}
    local tables = { ... }
    
    for _, key_table in ipairs(tables) do
        for _, binding in ipairs(key_table) do
            table.insert(result, binding)
        end
    end
    
    return result
end

return M