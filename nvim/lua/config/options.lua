-- Options are automatically loaded before lazy.nvim startup
-- Default options that are always set: https://github.com/LazyVim/LazyVim/blob/main/lua/lazyvim/config/options.lua
-- Add any additional options here

-- disable word wrap
-- Note: VSCode-specific wrap handling (enabling wrap only for text/markdown) is configured
-- in plugins/vscode.lua to work with the vscode-neovim extension
vim.opt.wrap = false

-- Enable Copilot popup completions via nvim-cmp (instead of inline suggestions)
-- This integrates with the copilot-native extra to show completions in the popup menu
vim.g.ai_cmp = true

-- VSCode-specific settings to suppress output panel messages
if vim.g.vscode then
  -- Set cmdheight to 1 to keep messages in Neovim's command line
  -- (prevents them from being sent to VSCode's output panel)
  vim.opt.cmdheight = 1

  -- Suppress search count messages ([1/26], [2/26], etc.)
  vim.opt.shortmess:append("S")

  -- Suppress other verbose messages
  vim.opt.shortmess:append({
    W = true,  -- Don't show "written" messages
    I = true,  -- Don't show intro message
    c = true,  -- Don't show completion menu messages
  })

  -- Disable command display in status line
  vim.opt.showcmd = false
  vim.opt.showmode = false

  -- Reduce notifications about line changes
  vim.opt.report = 999
end

-- ================================================================
-- Clipboard Configuration
-- ================================================================

-- Enable clipboard integration with system clipboard
vim.opt.clipboard = "unnamedplus"

-- Set up OSC 52 clipboard for SSH sessions
-- This must be done after options are set to override the default clipboard provider
-- vim.schedule(function()
--   require("config.clipboard").setup()
-- end)
