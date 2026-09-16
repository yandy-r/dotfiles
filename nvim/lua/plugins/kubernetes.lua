return {
  {
    "ramilito/kubectl.nvim",
    dependencies = { "nvim-lua/plenary.nvim" },
    cmd = { "Kubectl", "Kubectx", "Kubens" },
    keys = {
      { "<leader>k", "<cmd>Kubectl<cr>", desc = "Kubectl" },
      { "<leader>kc", "<cmd>Kubectx<cr>", desc = "Kubectl Context" },
      { "<leader>kn", "<cmd>Kubens<cr>", desc = "Kubectl Namespace" },
    },
    config = function()
      require("kubectl").setup()
    end,
  },
  {
    "mason-org/mason.nvim",
    opts = function(_, opts)
      opts.ensure_installed = opts.ensure_installed or {}
      vim.list_extend(opts.ensure_installed, {
        "helm-ls",
      })
      return opts
    end,
  },
}
