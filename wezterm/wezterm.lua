-- ================================================================
-- WezTerm Configuration
-- ================================================================
-- Modern, modular WezTerm configuration with comprehensive features

local wezterm = require('wezterm')

-- Load configuration modules
local functions = require('functions')
local appearance = require('appearance')
local keybindings = require('keybindings')
local statusbar = require('statusbar')

-- Initialize configuration
local config = wezterm.config_builder()

-- ================================================================
-- CORE CONFIGURATION
-- ================================================================

-- Basic settings
config.automatically_reload_config = true
config.check_for_updates = false
config.native_macos_fullscreen_mode = true
config.window_close_confirmation = 'NeverPrompt'
config.notification_handling = 'AlwaysShow'
config.adjust_window_size_when_changing_font_size = false
config.allow_square_glyphs_to_overflow_width = 'Never'
config.bold_brightens_ansi_colors = 'BrightAndBold'

-- Behavior settings
config.audible_bell = 'Disabled'
config.visual_bell = {
    fade_in_function = 'EaseIn',
    fade_in_duration_ms = 150,
    fade_out_function = 'EaseOut',
    fade_out_duration_ms = 150,
}

-- Text selection
config.selection_word_boundary = ' \t\n{}[]()"\'"`'
config.default_cwd = functions.get_home_dir()

-- ================================================================
-- MODULE INTEGRATION
-- ================================================================

-- Apply appearance settings (themes, fonts, window)
config = appearance.apply_to_config(config)

-- Apply keybindings (comprehensive keyboard shortcuts)
config = keybindings.apply_to_config(config)

-- Apply status bar configuration (if enabled)
config = statusbar.apply_to_config(config)

-- ================================================================
-- DOMAIN AND SSH CONFIGURATION
-- ================================================================

-- SSH domain configuration (customize as needed)
config.ssh_domains = {
    {
        name = 'dev-server',
        remote_address = functions.get_env('WEZTERM_SSH_HOST', 'localhost'),
        username = functions.get_env('WEZTERM_SSH_USER', functions.get_env('USER', 'user')),
    },
}

-- Unix domain configuration
if functions.is_macos() or functions.is_linux() then
    config.unix_domains = {
        {
            name = 'unix',
        },
    }
end

-- Default domain
config.default_domain = 'local'

-- ================================================================
-- WORKSPACE CONFIGURATION
-- ================================================================

-- Default workspace configuration
config.default_workspace = 'main'

-- ================================================================
-- LAUNCH MENU
-- ================================================================

-- Configure launch menu entries
config.launch_menu = {}

-- Add common shells
if functions.command_exists('zsh') then
    table.insert(config.launch_menu, {
        label = 'Zsh',
        args = { 'zsh', '-l' },
    })
end

if functions.command_exists('bash') then
    table.insert(config.launch_menu, {
        label = 'Bash',
        args = { 'bash', '-l' },
    })
end

if functions.command_exists('fish') then
    table.insert(config.launch_menu, {
        label = 'Fish',
        args = { 'fish', '-l' },
    })
end

-- Add development tools
if functions.command_exists('nvim') then
    table.insert(config.launch_menu, {
        label = 'Neovim',
        args = { 'nvim' },
    })
end

if functions.command_exists('htop') then
    table.insert(config.launch_menu, {
        label = 'System Monitor (htop)',
        args = { 'htop' },
    })
end

-- ================================================================
-- MULTIPLEXER CONFIGURATION
-- ================================================================

-- Multiplexer settings
config.enable_kitty_keyboard = true
config.enable_csi_u_key_encoding = false

-- ================================================================
-- LOGGING AND DEBUG
-- ================================================================

-- Configure logging
config.log_unknown_escape_sequences = false

-- ================================================================
-- ERROR HANDLING AND VALIDATION
-- ================================================================

-- Log configuration loading
functions.log('info', 'WezTerm configuration loaded successfully')

-- Validate configuration
local function validate_config(cfg)
    local errors = {}
    
    -- Check for required settings
    if not cfg.font then
        table.insert(errors, 'Font configuration is missing')
    end
    
    if not cfg.color_scheme then
        table.insert(errors, 'Color scheme is not set')
    end
    
    -- Log any errors
    for _, error in ipairs(errors) do
        functions.log('error', error)
    end
    
    return #errors == 0
end

-- Validate the final configuration
validate_config(config)

-- ================================================================
-- CUSTOM EVENT HANDLERS
-- ================================================================

-- Handle window configuration overrides
wezterm.on('window-config-reloaded', function(window, pane)
    functions.log('info', 'Window configuration reloaded')
end)

-- Handle tab title updates
wezterm.on('format-tab-title', statusbar.format_tab_title)

-- Handle status updates
wezterm.on('update-status', function(window, pane)
    -- This is handled by the statusbar module
end)

-- ================================================================
-- ENVIRONMENT-SPECIFIC OVERRIDES
-- ================================================================

-- macOS specific settings
if functions.is_macos() then
    -- macOS specific overrides
    config.send_composed_key_when_left_alt_is_pressed = false
    config.send_composed_key_when_right_alt_is_pressed = true
end

-- Windows specific settings
if functions.is_windows() then
    -- Windows specific overrides
    config.default_prog = { 'powershell.exe' }
end

-- Linux specific settings
if functions.is_linux() then
    -- Linux specific overrides
    config.enable_wayland = functions.get_env_bool('WEZTERM_ENABLE_WAYLAND', true)
end

return config
