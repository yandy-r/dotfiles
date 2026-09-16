return {
  {
    "mistweaverco/kulala.nvim",
    ft = { "http" },
    dependencies = { "nvim-lua/plenary.nvim" },
    keys = {
      { "<leader>rr", "<cmd>Rest run<cr>", desc = "Run REST request" },
      { "<leader>rl", "<cmd>Rest run last<cr>", desc = "Run last REST request" },
      { "<leader>re", "<cmd>Rest env show<cr>", desc = "Show REST env" },
      { "<leader>rs", "<cmd>Rest env select<cr>", desc = "Select REST env" },
    },
    opts = {
      default_view = "body",
      split_direction = "vertical",
      default_env = "dev",
      debug = false,
    },
  },
}
