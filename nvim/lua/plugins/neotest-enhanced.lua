-- ================================================================
-- Neotest Enhanced Configuration
-- ================================================================
-- Extends LazyVim's test.core extra with additional adapters
-- and improved keybindings for comprehensive test coverage.
--
-- Already enabled via lazyvim.json:
-- - neotest-python (via extras.lang.python)
-- - neotest-golang (via extras.lang.go)
-- - rustaceanvim.neotest (via extras.lang.rust)
-- - neotest-jest (need to add for JS/TS)
-- - neotest-vitest (need to add for modern JS/TS)
-- ================================================================

return {
  {
    "nvim-neotest/neotest",
    dependencies = {
      "nvim-neotest/nvim-nio",
      -- Additional adapters for JavaScript/TypeScript
      "nvim-neotest/neotest-jest",
      "marilari88/neotest-vitest",
    },
    opts = function(_, opts)
      opts.adapters = opts.adapters or {}

      -- Jest adapter for JavaScript/TypeScript
      -- Detects jest.config.js/ts files
      table.insert(
        opts.adapters,
        require("neotest-jest")({
          jestCommand = "npm test --",
          jestConfigFile = function(file)
            -- Handle monorepos with /packages/ structure
            if string.find(file, "/packages/") then
              return string.match(file, "(.-/[^/]+/)src") .. "jest.config.ts"
            end
            -- Check common config file locations
            local root = vim.fn.getcwd()
            for _, config in ipairs({ "jest.config.ts", "jest.config.js", "jest.config.mjs" }) do
              if vim.fn.filereadable(root .. "/" .. config) == 1 then
                return root .. "/" .. config
              end
            end
            return vim.fn.getcwd() .. "/jest.config.ts"
          end,
          env = { CI = true },
          cwd = function()
            return vim.fn.getcwd()
          end,
        })
      )

      -- Vitest adapter for modern JavaScript/TypeScript projects
      -- Auto-detects vitest.config.ts/js files
      table.insert(opts.adapters, require("neotest-vitest"))

      -- Output configuration
      opts.output = opts.output or {}
      opts.output.open_on_run = true

      -- Summary configuration
      opts.summary = opts.summary or {}
      opts.summary.animated = true

      -- Quickfix configuration
      opts.quickfix = opts.quickfix or {}
      opts.quickfix.enabled = true
      opts.quickfix.open = false -- Don't auto-open, use keybinding

      return opts
    end,
    -- Enhanced keybindings for testing
    keys = {
      -- Run tests
      {
        "<leader>tr",
        function()
          require("neotest").run.run()
        end,
        desc = "Run Nearest Test",
      },
      {
        "<leader>tR",
        function()
          require("neotest").run.run(vim.fn.expand("%"))
        end,
        desc = "Run File Tests",
      },
      {
        "<leader>tT",
        function()
          require("neotest").run.run(vim.uv.cwd())
        end,
        desc = "Run All Tests",
      },
      {
        "<leader>tl",
        function()
          require("neotest").run.run_last()
        end,
        desc = "Run Last Test",
      },

      -- Debug tests (integrates with DAP)
      {
        "<leader>td",
        function()
          require("neotest").run.run({ strategy = "dap" })
        end,
        desc = "Debug Nearest Test",
      },
      {
        "<leader>tD",
        function()
          require("neotest").run.run({ vim.fn.expand("%"), strategy = "dap" })
        end,
        desc = "Debug File Tests",
      },

      -- Test UI
      {
        "<leader>ts",
        function()
          require("neotest").summary.toggle()
        end,
        desc = "Toggle Test Summary",
      },
      {
        "<leader>to",
        function()
          require("neotest").output.open({ enter = true, auto_close = true })
        end,
        desc = "Show Test Output",
      },
      {
        "<leader>tO",
        function()
          require("neotest").output_panel.toggle()
        end,
        desc = "Toggle Output Panel",
      },

      -- Test control
      {
        "<leader>tS",
        function()
          require("neotest").run.stop()
        end,
        desc = "Stop Tests",
      },
      {
        "<leader>tw",
        function()
          require("neotest").watch.toggle(vim.fn.expand("%"))
        end,
        desc = "Toggle Watch Mode",
      },

      -- Navigation
      {
        "[t",
        function()
          require("neotest").jump.prev({ status = "failed" })
        end,
        desc = "Previous Failed Test",
      },
      {
        "]t",
        function()
          require("neotest").jump.next({ status = "failed" })
        end,
        desc = "Next Failed Test",
      },

      -- Quickfix
      {
        "<leader>tq",
        function()
          require("neotest").output.open({ enter = false, short = true })
        end,
        desc = "Quick Test Output",
      },
    },
  },

  -- Which-key group registration
  {
    "folke/which-key.nvim",
    optional = true,
    opts = {
      spec = {
        { "<leader>t", group = "test" },
      },
    },
  },
}
