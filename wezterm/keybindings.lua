-- ================================================================
-- WezTerm Keybindings Configuration
-- ================================================================
-- Comprehensive keyboard shortcuts for all WezTerm functionality

local wezterm = require('wezterm')
local functions = require('functions')
local act = wezterm.action

local M = {}

-- ================================================================
-- CONFIGURATION
-- ================================================================

M.config = {
    -- Use Alt instead of Cmd on non-macOS systems
    leader_key = functions.is_macos() and 'CMD' or 'ALT',
    -- Enable vim-style navigation
    vim_mode = functions.get_env_bool('WEZTERM_VIM_MODE', true),
    -- Enable tmux-style bindings
    tmux_mode = functions.get_env_bool('WEZTERM_TMUX_MODE', false),
}

-- ================================================================
-- HELPER FUNCTIONS
-- ================================================================

-- Create key binding with OS-aware modifiers
local function key(key_char, mods, action, desc)
    return functions.create_key_binding(key_char, mods, action, desc)
end

-- OS-aware modifier key
local function mod(base_mod)
    if base_mod == 'SUPER' then
        return functions.is_macos() and 'CMD' or 'ALT'
    end
    return base_mod
end

-- ================================================================
-- APPLICATION CONTROL
-- ================================================================

M.app_control = {
    -- Application-level controls
    key('q', mod('SUPER'), act.QuitApplication, 'Quit WezTerm'),
    key('n', mod('SUPER'), act.SpawnWindow, 'New window'),
    key(',', mod('SUPER'), act.SpawnCommandInNewTab({
        cwd = wezterm.home_dir,
        args = { wezterm.executable_dir .. '/wezterm', 'start', '--always-new-process' },
    }), 'Open preferences'),
    
    -- Configuration management
    key('r', 'CTRL|SHIFT', act.ReloadConfiguration, 'Reload configuration'),
    key('u', mod('SUPER'), act.CharSelect({
        copy_on_select = true,
        copy_to = 'ClipboardAndPrimarySelection',
    }), 'Unicode character selector'),
    
    -- Debug and development
    key('l', 'CTRL|SHIFT', act.ShowDebugOverlay, 'Show debug overlay'),
    key('p', 'CTRL|SHIFT', act.ActivateCommandPalette, 'Command palette'),
}

-- ================================================================
-- WINDOW MANAGEMENT
-- ================================================================

M.window_management = {
    -- Disable alt+enter fullscreen to allow terminal apps (e.g., Claude Code) to use it for newlines
    key('Enter', 'ALT', act.SendKey({ key = 'Enter', mods = 'ALT' }), 'Pass alt+enter to terminal'),

    -- Window actions
    key('f', 'SHIFT|' .. mod('SUPER'), act.ToggleFullScreen, 'Toggle fullscreen'),
    key('z', mod('SUPER'), act.TogglePaneZoomState, 'Toggle pane zoom'),
    key('m', mod('SUPER'), act.Hide, 'Minimize window'),
    
    -- Window sizing
    key('=', mod('SUPER'), act.IncreaseFontSize, 'Increase font size'),
    key('-', mod('SUPER'), act.DecreaseFontSize, 'Decrease font size'),
    key('0', mod('SUPER'), act.ResetFontSize, 'Reset font size'),
    
    -- Window opacity (if supported)
    key('=', 'CTRL|SHIFT', act.IncreaseFontSize, 'Increase opacity'),
    key('-', 'CTRL|SHIFT', act.DecreaseFontSize, 'Decrease opacity'),
}

-- ================================================================
-- TAB MANAGEMENT
-- ================================================================

M.tab_management = {
    -- Tab creation and destruction
    key('t', mod('SUPER'), act.SpawnTab('CurrentPaneDomain'), 'New tab'),
    key('w', mod('SUPER'), act.CloseCurrentTab({ confirm = false }), 'Close tab'),
    key('w', 'SHIFT|CTRL', act.CloseCurrentTab({ confirm = false }), 'Close tab (alt)'),
    
    -- Tab navigation
    key('1', mod('SUPER'), act.ActivateTab(0), 'Go to tab 1'),
    key('2', mod('SUPER'), act.ActivateTab(1), 'Go to tab 2'),
    key('3', mod('SUPER'), act.ActivateTab(2), 'Go to tab 3'),
    key('4', mod('SUPER'), act.ActivateTab(3), 'Go to tab 4'),
    key('5', mod('SUPER'), act.ActivateTab(4), 'Go to tab 5'),
    key('6', mod('SUPER'), act.ActivateTab(5), 'Go to tab 6'),
    key('7', mod('SUPER'), act.ActivateTab(6), 'Go to tab 7'),
    key('8', mod('SUPER'), act.ActivateTab(7), 'Go to tab 8'),
    key('9', mod('SUPER'), act.ActivateTab(-1), 'Go to last tab'),
    
    -- Tab navigation (arrows)
    key('LeftArrow', mod('SUPER'), act.ActivateTabRelative(-1), 'Previous tab'),
    key('RightArrow', mod('SUPER'), act.ActivateTabRelative(1), 'Next tab'),
    key('Tab', 'CTRL', act.ActivateTabRelative(1), 'Next tab'),
    key('Tab', 'CTRL|SHIFT', act.ActivateTabRelative(-1), 'Previous tab'),
    
    -- Tab reordering
    key('{', 'SHIFT|ALT', act.MoveTabRelative(-1), 'Move tab left'),
    key('}', 'SHIFT|ALT', act.MoveTabRelative(1), 'Move tab right'),
    
    -- Tab renaming
    key('e', mod('SUPER'), act.PromptInputLine({
        description = 'Enter new name for tab',
        action = wezterm.action_callback(function(window, pane, line)
            if line then
                window:active_tab():set_title(line)
            end
        end),
    }), 'Rename tab'),
}

-- ================================================================
-- PANE MANAGEMENT
-- ================================================================

M.pane_management = {
    -- Pane creation
    key('d', mod('SUPER'), act.SplitHorizontal({ domain = 'CurrentPaneDomain' }), 'Split right'),
    key('D', mod('SUPER'), act.SplitVertical({ domain = 'CurrentPaneDomain' }), 'Split down'),
    key('|', 'CTRL|SHIFT|ALT', act.SplitHorizontal({ domain = 'CurrentPaneDomain' }), 'Split right (alt)'),
    key('_', 'CTRL|SHIFT|ALT', act.SplitVertical({ domain = 'CurrentPaneDomain' }), 'Split down (alt)'),
    
    -- Pane destruction
    key('x', mod('SUPER'), act.CloseCurrentPane({ confirm = false }), 'Close pane'),
    
    -- Pane navigation (vim-style)
    key('h', 'CTRL|SHIFT', act.ActivatePaneDirection('Left'), 'Move to left pane'),
    key('j', 'CTRL|SHIFT', act.ActivatePaneDirection('Down'), 'Move to down pane'),
    key('k', 'CTRL|SHIFT', act.ActivatePaneDirection('Up'), 'Move to up pane'),
    key('l', 'CTRL|SHIFT', act.ActivatePaneDirection('Right'), 'Move to right pane'),
    
    -- Pane navigation (arrows)
    key('LeftArrow', 'CTRL|SHIFT', act.ActivatePaneDirection('Left'), 'Move to left pane'),
    key('DownArrow', 'CTRL|SHIFT', act.ActivatePaneDirection('Down'), 'Move to down pane'),
    key('UpArrow', 'CTRL|SHIFT', act.ActivatePaneDirection('Up'), 'Move to up pane'),
    key('RightArrow', 'CTRL|SHIFT', act.ActivatePaneDirection('Right'), 'Move to right pane'),
    
    -- Pane resizing (vim-style)
    key('h', 'CTRL|ALT', act.AdjustPaneSize({ 'Left', 5 }), 'Resize pane left'),
    key('j', 'CTRL|ALT', act.AdjustPaneSize({ 'Down', 5 }), 'Resize pane down'),
    key('k', 'CTRL|ALT', act.AdjustPaneSize({ 'Up', 5 }), 'Resize pane up'),
    key('l', 'CTRL|ALT', act.AdjustPaneSize({ 'Right', 5 }), 'Resize pane right'),
    
    -- Pane resizing (arrows)
    key('LeftArrow', 'CTRL|ALT', act.AdjustPaneSize({ 'Left', 5 }), 'Resize pane left'),
    key('DownArrow', 'CTRL|ALT', act.AdjustPaneSize({ 'Down', 5 }), 'Resize pane down'),
    key('UpArrow', 'CTRL|ALT', act.AdjustPaneSize({ 'Up', 5 }), 'Resize pane up'),
    key('RightArrow', 'CTRL|ALT', act.AdjustPaneSize({ 'Right', 5 }), 'Resize pane right'),
    
    -- Pane swapping
    key('H', 'CTRL|SHIFT', act.RotatePanes('Clockwise'), 'Rotate panes clockwise'),
    key('L', 'CTRL|SHIFT', act.RotatePanes('CounterClockwise'), 'Rotate panes counter-clockwise'),
}

-- ================================================================
-- COPY MODE AND SEARCH
-- ================================================================

M.copy_search = {
    -- Copy mode
    key('Space', 'CTRL|SHIFT', act.ActivateCopyMode, 'Enter copy mode'),
    key('X', 'CTRL', act.ActivateCopyMode, 'Enter copy mode (alt)'),
    key('v', mod('SUPER'), act.PasteFrom('Clipboard'), 'Paste from clipboard'),
    key('c', mod('SUPER'), act.CopyTo('Clipboard'), 'Copy to clipboard'),
    
    -- Search
    key('f', mod('SUPER'), act.Search({ CaseSensitiveString = '' }), 'Search'),
    key('g', mod('SUPER'), act.Search({ Regex = '' }), 'Search with regex'),
    key('F3', '', act.Search({ CaseInSensitiveString = '' }), 'Search (case insensitive)'),
    
    -- Quick select
    key('o', mod('SUPER'), act.QuickSelect, 'Quick select mode'),
    key('O', mod('SUPER'), act.QuickSelectArgs({
        patterns = { 'https?://\\S+' },
        action = wezterm.action_callback(function(window, pane)
            local url = window:get_selection_text_for_pane(pane)
            wezterm.open_with(url)
        end),
    }), 'Quick select URLs'),
}

-- ================================================================
-- SCROLLING AND NAVIGATION
-- ================================================================

M.scrolling = {
    -- Scrolling
    key('UpArrow', 'SHIFT', act.ScrollByLine(-1), 'Scroll up'),
    key('DownArrow', 'SHIFT', act.ScrollByLine(1), 'Scroll down'),
    key('PageUp', 'SHIFT', act.ScrollByPage(-0.5), 'Scroll up half page'),
    key('PageDown', 'SHIFT', act.ScrollByPage(0.5), 'Scroll down half page'),
    key('Home', 'SHIFT', act.ScrollToTop, 'Scroll to top'),
    key('End', 'SHIFT', act.ScrollToBottom, 'Scroll to bottom'),
    
    -- Clear screen
    key('k', mod('SUPER'), act.Multiple({
        act.ClearScrollback('ScrollbackAndViewport'),
        act.SendKey({ key = 'L', mods = 'CTRL' }),
    }), 'Clear screen'),
    key('K', 'CTRL|SHIFT', act.ClearScrollback('ScrollbackOnly'), 'Clear scrollback'),
}

-- ================================================================
-- APPEARANCE CONTROLS
-- ================================================================

M.appearance = {
    -- Theme switching
    key('t', 'CTRL|SHIFT', wezterm.action_callback(function(window, pane)
        local appearance = require('appearance')
        local current_scheme = window:effective_config().color_scheme
        local new_scheme = appearance.cycle_theme(current_scheme)
        
        local overrides = window:get_config_overrides() or {}
        overrides.color_scheme = new_scheme
        window:set_config_overrides(overrides)
    end), 'Cycle color theme'),
    
    -- Font size (additional bindings)
    key('=', 'CTRL', act.IncreaseFontSize, 'Increase font size'),
    key('-', 'CTRL', act.DecreaseFontSize, 'Decrease font size'),
    key('0', 'CTRL', act.ResetFontSize, 'Reset font size'),
    
    -- Opacity controls (if supported)
    key('=', 'CTRL|ALT', wezterm.action_callback(function(window)
        local overrides = window:get_config_overrides() or {}
        local current_opacity = overrides.window_background_opacity or 0.95
        overrides.window_background_opacity = math.min(1.0, current_opacity + 0.05)
        window:set_config_overrides(overrides)
    end), 'Increase opacity'),
    
    key('-', 'CTRL|ALT', wezterm.action_callback(function(window)
        local overrides = window:get_config_overrides() or {}
        local current_opacity = overrides.window_background_opacity or 0.95
        overrides.window_background_opacity = math.max(0.1, current_opacity - 0.05)
        window:set_config_overrides(overrides)
    end), 'Decrease opacity'),
}

-- ================================================================
-- QUICK LAUNCH
-- ================================================================

M.quick_launch = {
    -- Common applications and commands
    key('Return', mod('SUPER'), act.SpawnCommandInNewTab({
        cwd = wezterm.home_dir,
    }), 'New terminal'),
    
    key('Return', 'SHIFT|' .. mod('SUPER'), act.SpawnCommandInNewWindow({
        cwd = wezterm.home_dir,
    }), 'New terminal window'),
    
    -- SSH quick connect (customize as needed)
    key('s', 'CTRL|ALT', act.PromptInputLine({
        description = 'Enter hostname to SSH to:',
        action = wezterm.action_callback(function(window, pane, line)
            if line then
                window:perform_action(
                    act.SpawnCommandInNewTab({
                        args = { 'ssh', line },
                    }),
                    pane
                )
            end
        end),
    }), 'SSH quick connect'),
    
    -- Command runner
    key('r', 'CTRL|ALT', act.PromptInputLine({
        description = 'Enter command to run:',
        action = wezterm.action_callback(function(window, pane, line)
            if line then
                pane:send_text(line .. '\n')
            end
        end),
    }), 'Run command'),
}

-- ================================================================
-- MOUSE BINDINGS
-- ================================================================

M.mouse_bindings = {
    -- Link opening
    {
        event = { Up = { streak = 1, button = 'Left' } },
        mods = 'CTRL',
        action = act.OpenLinkAtMouseCursor,
    },
    
    -- Right click context menu
    {
        event = { Down = { streak = 1, button = 'Right' } },
        mods = 'NONE',
        action = wezterm.action_callback(function(window, pane)
            local has_selection = window:get_selection_text_for_pane(pane) ~= ''
            
            if has_selection then
                window:perform_action(act.CopyTo('ClipboardAndPrimarySelection'), pane)
            else
                window:perform_action(act.PasteFrom('Clipboard'), pane)
            end
        end),
    },
    
    -- Middle click paste
    {
        event = { Up = { streak = 1, button = 'Middle' } },
        mods = 'NONE',
        action = act.PasteFrom('PrimarySelection'),
    },
    
    -- Scroll wheel
    {
        event = { Down = { streak = 1, button = { WheelUp = 1 } } },
        mods = 'CTRL',
        action = act.IncreaseFontSize,
    },
    
    {
        event = { Down = { streak = 1, button = { WheelDown = 1 } } },
        mods = 'CTRL',
        action = act.DecreaseFontSize,
    },
}

-- ================================================================
-- CONFIGURATION BUILDER
-- ================================================================

-- Merge all keybinding groups
function M.get_all_keybindings()
    return functions.merge_key_bindings(
        M.app_control,
        M.window_management,
        M.tab_management,
        M.pane_management,
        M.copy_search,
        M.scrolling,
        M.appearance,
        M.quick_launch
    )
end

-- Apply keybindings to config
function M.apply_to_config(config)
    config.keys = M.get_all_keybindings()
    config.mouse_bindings = M.mouse_bindings
    
    -- Disable default key assignments that conflict
    config.disable_default_key_bindings = false
    
    return config
end

-- ================================================================
-- TMUX-STYLE BINDINGS (OPTIONAL)
-- ================================================================

M.tmux_bindings = {
    -- Leader key + commands (similar to tmux)
    key('a', 'CTRL', act.SendKey({ key = 'a', mods = 'CTRL' }), 'Send Ctrl-A'),
    key('c', 'CTRL', act.SpawnTab('CurrentPaneDomain'), 'New tab (tmux-style)'),
    key('x', 'CTRL', act.CloseCurrentPane({ confirm = false }), 'Close pane (tmux-style)'),
    key('%', 'CTRL', act.SplitHorizontal({ domain = 'CurrentPaneDomain' }), 'Split horizontal (tmux-style)'),
    key('"', 'CTRL', act.SplitVertical({ domain = 'CurrentPaneDomain' }), 'Split vertical (tmux-style)'),
}

-- Apply tmux-style bindings if enabled
function M.apply_tmux_bindings(config)
    if M.config.tmux_mode then
        local tmux_keys = functions.merge_key_bindings(config.keys or {}, M.tmux_bindings)
        config.keys = tmux_keys
    end
    
    return config
end

return M