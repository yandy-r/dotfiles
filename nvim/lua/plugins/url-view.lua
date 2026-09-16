return {
  {
    "axieax/urlview.nvim",
    cmd = "UrlView",
    keys = {
      { "<leader>fu", "<cmd>UrlView<cr>", desc = "View URLs" },
      { "<leader>fU", "<cmd>UrlView buffer<cr>", desc = "View Buffer URLs" },
    },
    opts = {
      default_picker = "telescope",
      default_action = "system",
      default_register = "+",
      log_level = "info",
      jump = {
        prev = "[u",
        next = "]u",
      },
    },
    config = function(_, opts)
      require("urlview").setup(opts)
    end,
  },
}
