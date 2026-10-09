-- ================================================================
-- WezTerm Appearance Configuration
-- ================================================================
-- Theme, colors, fonts, and visual settings

local wezterm = require("wezterm")
local functions = require("functions")

local M = {}

-- ================================================================
-- THEME CONFIGURATION
-- ================================================================

-- Available color schemes with time-based switching
M.color_schemes = {
	dark = {
		"Catppuccin Mocha",
		"Tokyo Night",
		"Nord",
		"Dracula",
		"One Dark (Gogh)",
		"Gruvbox Dark",
	},
	light = {
		"Catppuccin Latte",
		"Tokyo Night Day",
		"Nord Light",
		"One Light (Gogh)",
		"Gruvbox Light",
		"Solarized Light (Gogh)",
	},
}

-- Default theme selection
M.default_themes = {
	dark = "Catppuccin Mocha",
	light = "Catppuccin Latte",
}

-- Get current theme based on time and environment
function M.get_current_theme()
	-- Check for manual override
	local manual_theme = functions.get_env("WEZTERM_THEME")
	if manual_theme then
		return manual_theme
	end

	-- Time-based theme switching (disabled by default - prefer dark theme)
	local use_light = functions.get_env_bool("WEZTERM_AUTO_THEME", false)
	if use_light and functions.is_daytime() then
		return M.default_themes.light
	else
		return M.default_themes.dark
	end
end

-- ================================================================
-- FONT CONFIGURATION
-- ================================================================

-- Font settings with fallbacks
M.fonts = {
	primary = {
		family = "JetBrainsMono Nerd Font",
		size = functions.get_env_number("WEZTERM_FONT_SIZE", 15.0),
		weight = "Regular",
	},
	fallbacks = {
		"Menlo",
		"Monaco",
		"Consolas",
		"Liberation Mono",
		"DejaVu Sans Mono",
	},
}

-- Create font configuration
function M.create_font_config()
	local font_list = {
		{
			family = M.fonts.primary.family,
			weight = M.fonts.primary.weight,
		},
	}

	-- Add fallback fonts
	for _, fallback in ipairs(M.fonts.fallbacks) do
		table.insert(font_list, { family = fallback })
	end

	return wezterm.font_with_fallback(font_list)
end

-- Font size adjustments
M.font_size_adjustments = {
	tiny = -4,
	small = -2,
	normal = 0,
	large = 2,
	huge = 4,
}

-- ================================================================
-- WINDOW APPEARANCE
-- ================================================================

-- Window configuration
M.window = {
	opacity = functions.get_env_number("WEZTERM_OPACITY", 0.95),
	blur = functions.get_env_number("WEZTERM_BLUR", 40),
	-- On Linux, let the compositor draw the titlebar: WezTerm's own Wayland
	-- decorations (RESIZE / INTEGRATED_BUTTONS) oversize maximized windows on
	-- KDE, pushing the bottom tab bar under the panel.
	decorations = functions.get_env("WEZTERM_DECORATIONS", functions.is_linux() and "TITLE | RESIZE" or "RESIZE"),
	padding = {
		left = functions.get_env_number("WEZTERM_PADDING_H", 8),
		right = functions.get_env_number("WEZTERM_PADDING_H", 8),
		top = functions.get_env_number("WEZTERM_PADDING_V", 8),
		bottom = functions.get_env_number("WEZTERM_PADDING_V", 8),
	},
	initial_size = {
		cols = functions.get_env_number("WEZTERM_COLS", 120),
		rows = functions.get_env_number("WEZTERM_ROWS", 40),
	},
}

-- ================================================================
-- CURSOR CONFIGURATION
-- ================================================================

M.cursor = {
	style = functions.get_env("WEZTERM_CURSOR_STYLE", "SteadyBlock"),
	blink_rate = functions.get_env_number("WEZTERM_CURSOR_BLINK", 800),
	thickness = functions.get_env_number("WEZTERM_CURSOR_THICKNESS", 2),
}

-- Available cursor styles
M.cursor_styles = {
	"SteadyBlock",
	"BlinkingBlock",
	"SteadyUnderline",
	"BlinkingUnderline",
	"SteadyBar",
	"BlinkingBar",
}

-- ================================================================
-- TAB BAR APPEARANCE
-- ================================================================

M.tab_bar = {
	enabled = functions.get_env_bool("WEZTERM_SHOW_TABS", true),
	position = "top",
	style = "fancy", -- 'fancy', 'retro', 'custom'
	show_tab_index = true,
	max_width = 32,
}

-- Tab bar color scheme
M.tab_colors = {
	background = "rgba(11, 0, 34, 0.3)",
	inactive_tab = {
		bg_color = "rgba(27, 16, 50, 0.4)",
		fg_color = "#808080",
		intensity = "Normal",
	},
	active_tab = {
		bg_color = "rgba(43, 32, 66, 0.7)",
		fg_color = "#ffffff",
		intensity = "Bold",
	},
	inactive_tab_hover = {
		bg_color = "rgba(59, 48, 82, 0.5)",
		fg_color = "#a0a0a0",
		italic = true,
	},
	new_tab = {
		bg_color = "rgba(27, 16, 50, 0.4)",
		fg_color = "#808080",
	},
	new_tab_hover = {
		bg_color = "rgba(59, 48, 82, 0.5)",
		fg_color = "#a0a0a0",
	},
}

-- ================================================================
-- COLOR CUSTOMIZATION
-- ================================================================

-- Custom color overrides
M.color_overrides = {
	-- You can override specific colors here
	-- foreground = '#ffffff',
	-- background = '#000000',
	-- cursor_bg = '#ffffff',
	-- cursor_fg = '#000000',
	-- selection_bg = '#404040',
	-- selection_fg = '#ffffff',
}

-- Apply color overrides
function M.apply_color_overrides(config)
	if next(M.color_overrides) then
		config.colors = functions.merge_tables(config.colors or {}, M.color_overrides)
	end
end

-- ================================================================
-- PERFORMANCE SETTINGS
-- ================================================================

M.performance = {
	-- GPU acceleration
	webgpu_power_preference = "HighPerformance",
	front_end = "WebGpu",

	-- Animation settings
	animation_fps = functions.get_env_number("WEZTERM_ANIMATION_FPS", 60),

	-- Scrollback settings
	scrollback_lines = functions.get_env_number("WEZTERM_SCROLLBACK", 10000),

	-- Rendering settings
	max_fps = functions.get_env_number("WEZTERM_MAX_FPS", 60),
}

-- ================================================================
-- CONFIGURATION BUILDER
-- ================================================================

-- Apply all appearance settings to config
function M.apply_to_config(config)
	-- Theme
	config.color_scheme = M.get_current_theme()

	-- Font
	config.font = M.create_font_config()
	config.font_size = M.fonts.primary.size

	-- Window
	config.window_background_opacity = M.window.opacity
	config.macos_window_background_blur = M.window.blur
	config.window_decorations = M.window.decorations
	config.window_padding = M.window.padding
	config.initial_cols = M.window.initial_size.cols
	config.initial_rows = M.window.initial_size.rows

	-- Cursor
	config.default_cursor_style = M.cursor.style
	config.cursor_blink_rate = M.cursor.blink_rate
	config.cursor_thickness = M.cursor.thickness

	-- Tab bar
	config.enable_tab_bar = M.tab_bar.enabled
	config.hide_tab_bar_if_only_one_tab = false
	config.use_fancy_tab_bar = true
	config.tab_bar_at_bottom = M.tab_bar.position == "bottom"
	config.tab_max_width = M.tab_bar.max_width

	-- Apply tab bar colors
	config.colors = {
		tab_bar = M.tab_colors,
	}

	-- Performance
	config.webgpu_power_preference = M.performance.webgpu_power_preference
	config.front_end = M.performance.front_end
	config.animation_fps = M.performance.animation_fps
	config.scrollback_lines = M.performance.scrollback_lines
	config.max_fps = M.performance.max_fps

	-- Color overrides
	M.apply_color_overrides(config)

	return config
end

-- ================================================================
-- DYNAMIC THEME FUNCTIONS
-- ================================================================

-- Switch to light theme
function M.switch_to_light_theme()
	return M.default_themes.light
end

-- Switch to dark theme
function M.switch_to_dark_theme()
	return M.default_themes.dark
end

-- Cycle through available themes
function M.cycle_theme(current_theme)
	local is_light = functions.is_daytime()
	local themes = is_light and M.color_schemes.light or M.color_schemes.dark

	for i, theme in ipairs(themes) do
		if theme == current_theme then
			return themes[(i % #themes) + 1]
		end
	end

	return themes[1]
end

return M
