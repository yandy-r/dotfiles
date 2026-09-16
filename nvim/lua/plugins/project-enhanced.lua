return {
  {
    "ahmedkhalf/project.nvim",
    event = "VeryLazy",
    dependencies = {
      "nvim-telescope/telescope.nvim",
    },
    keys = {
      { "<leader>fp", "<cmd>Telescope projects<cr>", desc = "Projects" },
    },
    config = function()
      require("project_nvim").setup({
        detection_methods = { "pattern", "lsp" },
        patterns = {
          ".git",
          "_darcs",
          ".hg",
          ".bzr",
          ".svn",
          "Makefile",
          "package.json",
          "Cargo.toml",
          "go.mod",
          "requirements.txt",
          "pyproject.toml",
          "setup.py",
          "terraform.tf",
          "main.tf",
          "*.tf",
          "playbook.yml",
          "ansible.cfg",
          "docker-compose.yml",
          "Dockerfile",
          "pom.xml",
          "build.gradle",
        },
        ignore_lsp = {},
        exclude_dirs = {
          "~/.cargo/*",
          "~/.local/*",
          "~/.cache/*",
          "*/node_modules/*",
          "*/venv/*",
          "*/.venv/*",
          "*/target/*",
        },
        show_hidden = false,
        silent_chdir = true,
        scope_chdir = "global",
        datapath = vim.fn.stdpath("data"),
      })

      -- Safely load telescope extension
      local ok, telescope = pcall(require, "telescope")
      if ok then
        telescope.load_extension("projects")
      end
    end,
  },
}
