return {
  {
    "mistricky/codesnap.nvim",
    build = "make",
    cmd = { "CodeSnap", "CodeSnapSave" },
    keys = {
      { "<leader>cs", "<cmd>CodeSnap<cr>", mode = "x", desc = "Save selected code snapshot" },
      { "<leader>cS", "<cmd>CodeSnapSave<cr>", mode = "x", desc = "Copy code snapshot to clipboard" },
    },
    config = function()
      require("codesnap").setup({
        border = "rounded",
        has_breadcrumbs = true,
        bg_theme = "grape",
        watermark = "",
        save_path = os.getenv("HOME") .. "/Pictures/CodeSnap/",
      })
    end,
  },
}
