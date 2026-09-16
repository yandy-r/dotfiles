-- Keymaps are automatically loaded on the VeryLazy event
-- Default keymaps that are always set: https://github.com/LazyVim/LazyVim/blob/main/lua/lazyvim/config/keymaps.lua
-- Add any additional keymaps here
-- map jj to exit insert mo
vim.api.nvim_set_keymap("i", "jj", "<Esc>", { noremap = true, silent = true })

-- Terminal keymaps
-- Open floating terminal
vim.keymap.set("n", "<leader>ft", function()
  local width = math.floor(vim.o.columns * 0.8)
  local height = math.floor(vim.o.lines * 0.8)
  local buf = vim.api.nvim_create_buf(false, true)
  local win = vim.api.nvim_open_win(buf, true, {
    relative = "editor",
    width = width,
    height = height,
    col = math.floor((vim.o.columns - width) / 2),
    row = math.floor((vim.o.lines - height) / 2),
    style = "minimal",
    border = "rounded",
  })
  vim.cmd("terminal")
  vim.cmd("startinsert")
end, { desc = "Open floating terminal" })

-- Open terminal in new tab
vim.keymap.set("n", "<leader>fT", function()
  vim.cmd("tabnew")
  vim.cmd("terminal")
  vim.cmd("startinsert")
end, { desc = "Open terminal in new tab" })

-- Terminal keymaps
-- Store terminal state globally
local terminal_state = {
  buf = nil,
  win = nil,
}

-- Function to toggle floating terminal
local function toggle_terminal()
  if terminal_state.win and vim.api.nvim_win_is_valid(terminal_state.win) then
    -- Hide the terminal window if it exists and is valid
    vim.api.nvim_win_hide(terminal_state.win)
    terminal_state.win = nil
  else
    local width = math.floor(vim.o.columns * 0.8)
    local height = math.floor(vim.o.lines * 0.8)

    -- Create new buffer only if it doesn't exist or is invalid
    if not terminal_state.buf or not vim.api.nvim_buf_is_valid(terminal_state.buf) then
      terminal_state.buf = vim.api.nvim_create_buf(false, true)
      -- Initialize terminal in the buffer
      vim.api.nvim_buf_call(terminal_state.buf, function()
        vim.cmd("terminal")
      end)
    end

    -- Create new window
    terminal_state.win = vim.api.nvim_open_win(terminal_state.buf, true, {
      relative = "editor",
      width = width,
      height = height,
      col = math.floor((vim.o.columns - width) / 2),
      row = math.floor((vim.o.lines - height) / 2),
      style = "minimal",
      border = "rounded",
    })

    -- Enter insert mode
    vim.cmd("startinsert")
  end
end

-- Map Ctrl+\ to toggle terminal
vim.keymap.set(
  { "n", "t" },
  "<C-\\>",
  toggle_terminal,
  { noremap = true, silent = true, desc = "Toggle floating terminal" }
)
