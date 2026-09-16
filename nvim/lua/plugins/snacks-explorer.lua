return {
  "folke/snacks.nvim",
  opts = {
    explorer = {},
    picker = {
      sources = {
        explorer = {
          hidden = true,
          ignored = false,
        },
      },
    },
  },
  keys = {
    {
      "<leader>e",
      function()
        Snacks.explorer({ cwd = LazyVim.root() })
      end,
      desc = "Explorer (root dir)",
    },
    {
      "<leader>E",
      function()
        Snacks.explorer({ hidden = true, ignored = true })
      end,
      desc = "Explorer (show all including gitignored)",
    },
  },
}
