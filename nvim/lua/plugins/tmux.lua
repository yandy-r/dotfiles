return {
  {
    "christoomey/vim-tmux-navigator",
    lazy = false,
    keys = {
      { "<C-h>", "<cmd>TmuxNavigateLeft<cr>", desc = "Tmux Navigate Left" },
      { "<C-j>", "<cmd>TmuxNavigateDown<cr>", desc = "Tmux Navigate Down" },
      { "<C-k>", "<cmd>TmuxNavigateUp<cr>", desc = "Tmux Navigate Up" },
      { "<C-l>", "<cmd>TmuxNavigateRight<cr>", desc = "Tmux Navigate Right" },
      { "<C-\\>", "<cmd>TmuxNavigatePrevious<cr>", desc = "Tmux Navigate Previous" },
    },
  },
  {
    "aserowy/tmux.nvim",
    lazy = false,
    keys = {
      { "<A-h>", "<cmd>lua require('tmux').resize_left()<cr>", desc = "Resize Tmux Left" },
      { "<A-j>", "<cmd>lua require('tmux').resize_bottom()<cr>", desc = "Resize Tmux Down" },
      { "<A-k>", "<cmd>lua require('tmux').resize_top()<cr>", desc = "Resize Tmux Up" },
      { "<A-l>", "<cmd>lua require('tmux').resize_right()<cr>", desc = "Resize Tmux Right" },
    },
    config = function()
      require("tmux").setup({
        copy_sync = {
          enable = true,
          redirect_to_clipboard = true,
        },
        navigation = {
          enable_default_keybindings = false,
        },
        resize = {
          enable_default_keybindings = false,
        },
      })
    end,
  },
}
