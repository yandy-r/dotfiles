-- ================================================================
-- WezTerm Status Bar Configuration
-- ================================================================
-- Custom status bar with useful information and styling

local wezterm = require('wezterm')
local functions = require('functions')

local M = {}

-- ================================================================
-- CONFIGURATION
-- ================================================================

M.config = {
    enabled = functions.get_env_bool('WEZTERM_STATUSBAR', true),
    position = functions.get_env('WEZTERM_STATUSBAR_POSITION', 'bottom'), -- 'top' or 'bottom'
    show_time = functions.get_env_bool('WEZTERM_STATUSBAR_TIME', true),
    show_date = functions.get_env_bool('WEZTERM_STATUSBAR_DATE', false),
    show_battery = functions.get_env_bool('WEZTERM_STATUSBAR_BATTERY', true),
    show_hostname = functions.get_env_bool('WEZTERM_STATUSBAR_HOSTNAME', false),
    show_cwd = functions.get_env_bool('WEZTERM_STATUSBAR_CWD', true),
    show_git = functions.get_env_bool('WEZTERM_STATUSBAR_GIT', true),
    update_interval = functions.get_env_number('WEZTERM_STATUSBAR_INTERVAL', 1000), -- milliseconds
}

-- ================================================================
-- NERD FONT ICONS
-- ================================================================

M.icons = {
    time = wezterm.nerdfonts.md_clock_outline,
    date = wezterm.nerdfonts.md_calendar,
    battery = {
        charging = wezterm.nerdfonts.md_battery_charging,
        full = wezterm.nerdfonts.md_battery,
        high = wezterm.nerdfonts.md_battery_80,
        medium = wezterm.nerdfonts.md_battery_50,
        low = wezterm.nerdfonts.md_battery_20,
        critical = wezterm.nerdfonts.md_battery_alert,
    },
    git = {
        branch = wezterm.nerdfonts.dev_git_branch,
        modified = wezterm.nerdfonts.md_pencil,
        added = wezterm.nerdfonts.md_plus,
        deleted = wezterm.nerdfonts.md_minus,
        untracked = wezterm.nerdfonts.md_help,
    },
    folder = wezterm.nerdfonts.md_folder,
    hostname = wezterm.nerdfonts.md_desktop_tower,
    user = wezterm.nerdfonts.md_account,
    shell = wezterm.nerdfonts.md_console,
}

-- ================================================================
-- COLOR SCHEME
-- ================================================================

M.colors = {
    background = '#1e1e2e',
    foreground = '#cdd6f4',
    accent = '#89b4fa',
    warning = '#f9e2af',
    error = '#f38ba8',
    success = '#a6e3a1',
    muted = '#6c7086',
    separator = '#45475a',
}

-- ================================================================
-- UTILITY FUNCTIONS
-- ================================================================

-- Format time
local function format_time()
    if not M.config.show_time then
        return ''
    end
    
    local time = functions.get_current_time()
    return M.icons.time .. ' ' .. time
end

-- Format date
local function format_date()
    if not M.config.show_date then
        return ''
    end
    
    local date = functions.get_current_date()
    return M.icons.date .. ' ' .. date
end

-- Get battery information
local function get_battery_info()
    if not M.config.show_battery or not functions.is_macos() then
        return ''
    end
    
    local battery_output, err = functions.execute_command('pmset -g batt')
    if not battery_output then
        return ''
    end
    
    -- Parse battery percentage
    local percentage = battery_output:match('(%d+)%%')
    if not percentage then
        return ''
    end
    
    percentage = tonumber(percentage)
    local is_charging = battery_output:match('AC Power') ~= nil
    
    -- Select appropriate icon
    local icon
    if is_charging then
        icon = M.icons.battery.charging
    elseif percentage >= 90 then
        icon = M.icons.battery.full
    elseif percentage >= 60 then
        icon = M.icons.battery.high
    elseif percentage >= 30 then
        icon = M.icons.battery.medium
    elseif percentage >= 15 then
        icon = M.icons.battery.low
    else
        icon = M.icons.battery.critical
    end
    
    -- Select color based on battery level
    local color = M.colors.foreground
    if percentage < 15 then
        color = M.colors.error
    elseif percentage < 30 then
        color = M.colors.warning
    end
    
    return wezterm.format({
        { Foreground = { Color = color } },
        { Text = icon .. ' ' .. percentage .. '%' },
    })
end

-- Get hostname
local function get_hostname()
    if not M.config.show_hostname then
        return ''
    end
    
    local hostname = functions.execute_command('hostname') or 'unknown'
    hostname = functions.trim_string(hostname)
    
    return M.icons.hostname .. ' ' .. hostname
end

-- Get current working directory
local function get_cwd(pane)
    if not M.config.show_cwd then
        return ''
    end
    
    local cwd = pane:get_current_working_dir()
    if not cwd then
        return ''
    end
    
    -- Convert to string and extract path
    local path = tostring(cwd):gsub('file://[^/]*', '')
    
    -- Shorten path if too long
    local home = functions.get_home_dir()
    path = path:gsub('^' .. home, '~')
    
    -- Limit path length
    if #path > 40 then
        local parts = functions.split_string(path, '/')
        if #parts > 3 then
            path = '.../' .. table.concat({ parts[#parts-1], parts[#parts] }, '/')
        end
    end
    
    return M.icons.folder .. ' ' .. path
end

-- Get git information
local function get_git_info(pane)
    if not M.config.show_git then
        return ''
    end
    
    local cwd = pane:get_current_working_dir()
    if not cwd then
        return ''
    end
    
    -- Get git branch
    local branch_output = functions.execute_command('cd "' .. tostring(cwd):gsub('file://[^/]*', '') .. '" && git branch --show-current 2>/dev/null')
    if not branch_output or branch_output == '' then
        return ''
    end
    
    local branch = functions.trim_string(branch_output)
    
    -- Get git status
    local status_output = functions.execute_command('cd "' .. tostring(cwd):gsub('file://[^/]*', '') .. '" && git status --porcelain 2>/dev/null')
    
    local status_info = ''
    if status_output and status_output ~= '' then
        local lines = functions.split_string(status_output, '\n')
        local modified = 0
        local added = 0
        local deleted = 0
        local untracked = 0
        
        for _, line in ipairs(lines) do
            if line:match('^M') then
                modified = modified + 1
            elseif line:match('^A') then
                added = added + 1
            elseif line:match('^D') then
                deleted = deleted + 1
            elseif line:match('^%?%?') then
                untracked = untracked + 1
            end
        end
        
        local status_parts = {}
        if modified > 0 then
            table.insert(status_parts, M.icons.git.modified .. modified)
        end
        if added > 0 then
            table.insert(status_parts, M.icons.git.added .. added)
        end
        if deleted > 0 then
            table.insert(status_parts, M.icons.git.deleted .. deleted)
        end
        if untracked > 0 then
            table.insert(status_parts, M.icons.git.untracked .. untracked)
        end
        
        if #status_parts > 0 then
            status_info = ' [' .. table.concat(status_parts, ' ') .. ']'
        end
    end
    
    return M.icons.git.branch .. ' ' .. branch .. status_info
end

-- ================================================================
-- STATUS BAR COMPONENTS
-- ================================================================

-- Left status (usually contains session/tab info)
function M.format_left_status(window, pane)
    local components = {}
    
    -- Hostname
    local hostname = get_hostname()
    if hostname ~= '' then
        table.insert(components, hostname)
    end
    
    -- Current working directory
    local cwd = get_cwd(pane)
    if cwd ~= '' then
        table.insert(components, cwd)
    end
    
    -- Git information
    local git_info = get_git_info(pane)
    if git_info ~= '' then
        table.insert(components, git_info)
    end
    
    if #components == 0 then
        return ''
    end
    
    return table.concat(components, '  ')
end

-- Right status (usually contains system info)
function M.format_right_status()
    local components = {}
    
    -- Battery
    local battery = get_battery_info()
    if battery ~= '' then
        table.insert(components, battery)
    end
    
    -- Date
    local date = format_date()
    if date ~= '' then
        table.insert(components, date)
    end
    
    -- Time
    local time = format_time()
    if time ~= '' then
        table.insert(components, time)
    end
    
    if #components == 0 then
        return ''
    end
    
    return table.concat(components, '  ')
end

-- ================================================================
-- TAB BAR FORMATTING
-- ================================================================

-- Tab bar symbols
M.tab_symbols = {
    left_arrow = wezterm.nerdfonts.pl_right_hard_divider,
    right_arrow = wezterm.nerdfonts.pl_left_hard_divider,
}

-- Format individual tab title
function M.format_tab_title(tab, tabs, panes, config, hover, max_width)
    local background = M.colors.background
    local foreground = M.colors.muted
    
    if tab.is_active then
        background = M.colors.accent
        foreground = M.colors.background
    elseif hover then
        background = M.colors.separator
        foreground = M.colors.foreground
    end
    
    local edge_background = M.colors.background
    local edge_foreground = background
    
    -- Get tab title
    local title = functions.get_tab_title(tab)
    
    -- Get pane info to detect SSH/remote connections
    local pane = tab.active_pane
    local process_name = ''
    local hostname = ''
    local is_remote = false
    
    if pane then
        -- Check if this is an SSH session
        local foreground_process = pane.foreground_process_name or ''
        if foreground_process:find('ssh') then
            is_remote = true
            process_name = 'ssh'
        end
        
        -- Try to get hostname from pane title or user vars
        if pane.title and pane.title ~= '' then
            -- Check if title contains hostname pattern (user@host)
            local user_host = pane.title:match('([^@]+@[^:]+)')
            if user_host then
                hostname = user_host
                is_remote = true
            end
        end
        
        -- Check user vars for domain information
        local user_vars = pane.user_vars or {}
        if user_vars.WEZTERM_HOST then
            hostname = user_vars.WEZTERM_HOST
            is_remote = true
        end
    end
    
    -- Format title with connection info
    local connection_indicator = ''
    if is_remote then
        if hostname ~= '' then
            connection_indicator = wezterm.nerdfonts.md_cloud .. ' ' .. hostname .. ' | '
        else
            connection_indicator = wezterm.nerdfonts.md_cloud .. ' SSH | '
        end
    else
        connection_indicator = wezterm.nerdfonts.md_laptop .. ' local | '
    end
    
    title = connection_indicator .. functions.format_tab_title(title, max_width - #connection_indicator, true, tab.tab_index)
    
    return {
        { Background = { Color = edge_background } },
        { Foreground = { Color = edge_foreground } },
        { Text = M.tab_symbols.left_arrow },
        { Background = { Color = background } },
        { Foreground = { Color = foreground } },
        { Text = ' ' .. title .. ' ' },
        { Background = { Color = edge_background } },
        { Foreground = { Color = edge_foreground } },
        { Text = M.tab_symbols.right_arrow },
    }
end

-- ================================================================
-- CONFIGURATION SETUP
-- ================================================================

-- Setup status bar events
function M.setup_events()
    -- Update left status
    wezterm.on('update-left-status', function(window, pane)
        if not M.config.enabled then
            return
        end
        
        local status = M.format_left_status(window, pane)
        window:set_left_status(wezterm.format({
            { Background = { Color = M.colors.background } },
            { Foreground = { Color = M.colors.foreground } },
            { Text = ' ' .. status .. ' ' },
        }))
    end)
    
    -- Update right status
    wezterm.on('update-right-status', function(window, pane)
        if not M.config.enabled then
            return
        end
        
        local status = M.format_right_status()
        window:set_right_status(wezterm.format({
            { Background = { Color = M.colors.background } },
            { Foreground = { Color = M.colors.foreground } },
            { Text = ' ' .. status .. ' ' },
        }))
    end)
    
    -- Format tab title
    wezterm.on('format-tab-title', M.format_tab_title)
end

-- Apply status bar configuration
function M.apply_to_config(config)
    if not M.config.enabled then
        config.enable_tab_bar = false
        return config
    end
    
    -- Enable tab bar for status
    config.enable_tab_bar = true
    config.hide_tab_bar_if_only_one_tab = false
    config.tab_bar_at_bottom = M.config.position == 'bottom'
    config.use_fancy_tab_bar = true
    
    -- Setup events
    M.setup_events()
    
    return config
end

return M