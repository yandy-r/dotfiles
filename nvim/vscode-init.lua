-- VSCode Neovim Integration Settings
-- This file is loaded when Neovim is used within VSCode/Cursor
-- to suppress annoying output messages and optimize for VSCode integration

-- Suppress search count messages ([1/26], [2/26], etc.)
-- The 'S' flag in shortmess suppresses search count messages
vim.opt.shortmess:append("S")

-- Also suppress other potentially annoying messages
vim.opt.shortmess:append({
  W = true, -- Don't give "written" or "[w]" when writing a file
  I = true, -- Don't give the intro message when starting Vim
  c = true, -- Don't give ins-completion-menu messages
})

-- Keep command line at height 1 to prevent messages from going to VSCode output panel
-- (cmdheight = 0 forces messages to output panel, which is what we want to avoid)
vim.opt.cmdheight = 1

-- Additional settings for better VSCode integration
vim.opt.laststatus = 0  -- Disable status line (VSCode has its own)
vim.opt.showcmd = false  -- Don't show command in status line
vim.opt.showmode = false -- Don't show mode (INSERT, VISUAL, etc.)
