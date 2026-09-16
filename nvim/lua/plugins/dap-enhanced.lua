-- ================================================================
-- DAP (Debug Adapter Protocol) Enhanced Configuration
-- ================================================================
-- Extends LazyVim's dap.core extra with improved UI, custom
-- launch configurations, and enhanced keybindings.
--
-- Already enabled via lazyvim.json:
-- - dap.core (nvim-dap, nvim-dap-ui, nvim-dap-virtual-text)
-- - dap.nlua (Lua debugging)
--
-- Language-specific DAP adapters are auto-configured by:
-- - extras.lang.python (debugpy)
-- - extras.lang.go (delve)
-- - extras.lang.rust (via rustaceanvim)
-- ================================================================

return {
  -- ================================================================
  -- nvim-dap-ui: Enhanced UI Configuration
  -- ================================================================
  {
    "rcarriga/nvim-dap-ui",
    dependencies = {
      "mfussenegger/nvim-dap",
      "nvim-neotest/nvim-nio",
    },
    opts = {
      -- Custom layout: more useful arrangement for debugging
      layouts = {
        {
          -- Left sidebar: scopes, breakpoints, stacks, watches
          elements = {
            { id = "scopes", size = 0.35 },
            { id = "breakpoints", size = 0.15 },
            { id = "stacks", size = 0.25 },
            { id = "watches", size = 0.25 },
          },
          position = "left",
          size = 50,
        },
        {
          -- Bottom panel: repl and console
          elements = {
            { id = "repl", size = 0.5 },
            { id = "console", size = 0.5 },
          },
          position = "bottom",
          size = 12,
        },
      },
      -- Floating windows configuration
      floating = {
        max_height = 0.8,
        max_width = 0.8,
        border = "rounded",
        mappings = {
          close = { "q", "<Esc>" },
        },
      },
      -- Controls configuration
      controls = {
        enabled = true,
        element = "repl",
        icons = {
          pause = "",
          play = "",
          step_into = "",
          step_over = "",
          step_out = "",
          step_back = "",
          run_last = "",
          terminate = "",
          disconnect = "",
        },
      },
      -- Render settings
      render = {
        max_type_length = nil, -- Show full type names
        max_value_lines = 100, -- Max lines for values
        indent = 1,
      },
      -- Icons
      icons = {
        expanded = "▾",
        collapsed = "▸",
        current_frame = "→",
      },
    },
    config = function(_, opts)
      local dap = require("dap")
      local dapui = require("dapui")

      dapui.setup(opts)

      -- Auto open/close UI on debug sessions
      dap.listeners.after.event_initialized["dapui_config"] = function()
        dapui.open({})
      end
      dap.listeners.before.event_terminated["dapui_config"] = function()
        dapui.close({})
      end
      dap.listeners.before.event_exited["dapui_config"] = function()
        dapui.close({})
      end
    end,
    keys = {
      -- UI toggle keybindings
      {
        "<leader>du",
        function()
          require("dapui").toggle({})
        end,
        desc = "Toggle DAP UI",
      },
      {
        "<leader>de",
        function()
          require("dapui").eval()
        end,
        desc = "Eval Expression",
        mode = { "n", "v" },
      },
      {
        "<leader>dE",
        function()
          require("dapui").eval(vim.fn.input("Expression: "))
        end,
        desc = "Eval Input Expression",
      },
      {
        "<leader>df",
        function()
          require("dapui").float_element(nil, { enter = true })
        end,
        desc = "Float Element",
      },
    },
  },

  -- ================================================================
  -- nvim-dap: Core Debug Adapter Configuration
  -- ================================================================
  {
    "mfussenegger/nvim-dap",
    dependencies = {
      -- Virtual text for debugging
      {
        "theHamsta/nvim-dap-virtual-text",
        opts = {
          enabled = true,
          enabled_commands = true,
          highlight_changed_variables = true,
          highlight_new_as_changed = false,
          show_stop_reason = true,
          commented = false, -- Show virtual text alongside comments
          only_first_definition = true,
          all_references = false,
          virt_text_pos = "eol", -- Position: 'eol', 'overlay', 'inline'
          -- Custom virtual text handler
          display_callback = function(variable, buf, stackframe, node, options)
            if options.virt_text_pos == "inline" then
              return " = " .. variable.value
            else
              return variable.name .. " = " .. variable.value
            end
          end,
        },
      },
    },
    config = function()
      local dap = require("dap")

      -- Custom breakpoint signs
      vim.fn.sign_define("DapBreakpoint", {
        text = "",
        texthl = "DapBreakpoint",
        linehl = "",
        numhl = "",
      })
      vim.fn.sign_define("DapBreakpointCondition", {
        text = "",
        texthl = "DapBreakpointCondition",
        linehl = "",
        numhl = "",
      })
      vim.fn.sign_define("DapLogPoint", {
        text = "",
        texthl = "DapLogPoint",
        linehl = "",
        numhl = "",
      })
      vim.fn.sign_define("DapStopped", {
        text = "",
        texthl = "DapStopped",
        linehl = "DapStoppedLine",
        numhl = "",
      })
      vim.fn.sign_define("DapBreakpointRejected", {
        text = "",
        texthl = "DapBreakpointRejected",
        linehl = "",
        numhl = "",
      })

      -- Highlight groups for breakpoint signs
      vim.api.nvim_set_hl(0, "DapBreakpoint", { fg = "#e51400" })
      vim.api.nvim_set_hl(0, "DapBreakpointCondition", { fg = "#f1c40f" })
      vim.api.nvim_set_hl(0, "DapLogPoint", { fg = "#61afef" })
      vim.api.nvim_set_hl(0, "DapStopped", { fg = "#98c379" })
      vim.api.nvim_set_hl(0, "DapStoppedLine", { bg = "#2e3d2e" })
      vim.api.nvim_set_hl(0, "DapBreakpointRejected", { fg = "#656565" })

      -- ============================================================
      -- Node.js / JavaScript / TypeScript Debug Configurations
      -- ============================================================
      -- Requires: npm install -g @vscode/js-debug
      -- Or install via Mason: MasonInstall js-debug-adapter

      -- Check if js-debug-adapter is available via Mason
      local js_debug_path = vim.fn.stdpath("data") .. "/mason/packages/js-debug-adapter"
      local js_debug_available = vim.fn.isdirectory(js_debug_path) == 1

      if js_debug_available then
        -- Use pwa-node adapter from js-debug
        for _, adapter in ipairs({ "pwa-node", "pwa-chrome", "pwa-msedge" }) do
          dap.adapters[adapter] = {
            type = "server",
            host = "localhost",
            port = "${port}",
            executable = {
              command = "node",
              args = {
                js_debug_path .. "/js-debug/src/dapDebugServer.js",
                "${port}",
              },
            },
          }
        end

        -- Node.js launch configurations
        for _, lang in ipairs({ "javascript", "typescript", "javascriptreact", "typescriptreact" }) do
          dap.configurations[lang] = {
            -- Launch current file
            {
              type = "pwa-node",
              request = "launch",
              name = "Launch File",
              program = "${file}",
              cwd = "${workspaceFolder}",
              sourceMaps = true,
              skipFiles = { "<node_internals>/**", "node_modules/**" },
            },
            -- Attach to running process
            {
              type = "pwa-node",
              request = "attach",
              name = "Attach to Process",
              processId = require("dap.utils").pick_process,
              cwd = "${workspaceFolder}",
              sourceMaps = true,
              skipFiles = { "<node_internals>/**", "node_modules/**" },
            },
            -- Debug npm script (Jest, Vitest, etc.)
            {
              type = "pwa-node",
              request = "launch",
              name = "Debug npm Script",
              runtimeExecutable = "npm",
              runtimeArgs = function()
                local script = vim.fn.input("npm script: ", "test")
                return { "run", script }
              end,
              cwd = "${workspaceFolder}",
              sourceMaps = true,
              skipFiles = { "<node_internals>/**", "node_modules/**" },
              console = "integratedTerminal",
            },
            -- Debug with ts-node
            {
              type = "pwa-node",
              request = "launch",
              name = "Launch with ts-node",
              runtimeExecutable = "npx",
              runtimeArgs = { "ts-node" },
              program = "${file}",
              cwd = "${workspaceFolder}",
              sourceMaps = true,
              skipFiles = { "<node_internals>/**", "node_modules/**" },
              resolveSourceMapLocations = {
                "${workspaceFolder}/**",
                "!**/node_modules/**",
              },
            },
            -- Debug Jest test file
            {
              type = "pwa-node",
              request = "launch",
              name = "Debug Jest Tests",
              runtimeExecutable = "npx",
              runtimeArgs = {
                "jest",
                "--runInBand",
                "--no-coverage",
                "${file}",
              },
              cwd = "${workspaceFolder}",
              console = "integratedTerminal",
              internalConsoleOptions = "neverOpen",
              sourceMaps = true,
              skipFiles = { "<node_internals>/**", "node_modules/**" },
            },
            -- Debug Vitest test file
            {
              type = "pwa-node",
              request = "launch",
              name = "Debug Vitest Tests",
              runtimeExecutable = "npx",
              runtimeArgs = {
                "vitest",
                "run",
                "--no-coverage",
                "${file}",
              },
              cwd = "${workspaceFolder}",
              console = "integratedTerminal",
              sourceMaps = true,
              skipFiles = { "<node_internals>/**", "node_modules/**" },
            },
            -- Debug with Chrome
            {
              type = "pwa-chrome",
              request = "launch",
              name = "Launch Chrome",
              url = function()
                return vim.fn.input("URL: ", "http://localhost:3000")
              end,
              webRoot = "${workspaceFolder}",
              sourceMaps = true,
            },
          }
        end
      end
    end,
    -- Enhanced keybindings for debugging
    keys = {
      -- Breakpoints
      {
        "<leader>db",
        function()
          require("dap").toggle_breakpoint()
        end,
        desc = "Toggle Breakpoint",
      },
      {
        "<leader>dB",
        function()
          require("dap").set_breakpoint(vim.fn.input("Breakpoint condition: "))
        end,
        desc = "Conditional Breakpoint",
      },
      {
        "<leader>dl",
        function()
          require("dap").set_breakpoint(nil, nil, vim.fn.input("Log point message: "))
        end,
        desc = "Log Point",
      },
      {
        "<leader>dL",
        function()
          require("dap").list_breakpoints()
          vim.cmd("copen")
        end,
        desc = "List Breakpoints",
      },
      {
        "<leader>dx",
        function()
          require("dap").clear_breakpoints()
        end,
        desc = "Clear All Breakpoints",
      },

      -- Session control
      {
        "<leader>dc",
        function()
          require("dap").continue()
        end,
        desc = "Continue / Start",
      },
      {
        "<leader>dC",
        function()
          require("dap").run_to_cursor()
        end,
        desc = "Run to Cursor",
      },
      {
        "<leader>dp",
        function()
          require("dap").pause()
        end,
        desc = "Pause",
      },
      {
        "<leader>dq",
        function()
          require("dap").terminate()
        end,
        desc = "Terminate",
      },
      {
        "<leader>dr",
        function()
          require("dap").restart()
        end,
        desc = "Restart",
      },
      {
        "<leader>dR",
        function()
          require("dap").run_last()
        end,
        desc = "Run Last",
      },

      -- Stepping
      {
        "<leader>ds",
        function()
          require("dap").step_over()
        end,
        desc = "Step Over",
      },
      {
        "<leader>di",
        function()
          require("dap").step_into()
        end,
        desc = "Step Into",
      },
      {
        "<leader>do",
        function()
          require("dap").step_out()
        end,
        desc = "Step Out",
      },
      {
        "<leader>dj",
        function()
          require("dap").down()
        end,
        desc = "Down (Stack)",
      },
      {
        "<leader>dk",
        function()
          require("dap").up()
        end,
        desc = "Up (Stack)",
      },

      -- Function keys for common operations (IDE-like)
      {
        "<F5>",
        function()
          require("dap").continue()
        end,
        desc = "Debug: Continue",
      },
      {
        "<S-F5>",
        function()
          require("dap").terminate()
        end,
        desc = "Debug: Stop",
      },
      {
        "<F9>",
        function()
          require("dap").toggle_breakpoint()
        end,
        desc = "Debug: Toggle Breakpoint",
      },
      {
        "<F10>",
        function()
          require("dap").step_over()
        end,
        desc = "Debug: Step Over",
      },
      {
        "<F11>",
        function()
          require("dap").step_into()
        end,
        desc = "Debug: Step Into",
      },
      {
        "<S-F11>",
        function()
          require("dap").step_out()
        end,
        desc = "Debug: Step Out",
      },

      -- REPL
      {
        "<leader>dw",
        function()
          require("dap").repl.toggle()
        end,
        desc = "Toggle REPL",
      },

      -- Hover / inspect
      {
        "<leader>dh",
        function()
          require("dap.ui.widgets").hover()
        end,
        desc = "Hover Variables",
      },
      {
        "<leader>dS",
        function()
          local widgets = require("dap.ui.widgets")
          widgets.centered_float(widgets.scopes)
        end,
        desc = "Scopes (Float)",
      },
    },
  },

  -- ================================================================
  -- Which-key Group Registration
  -- ================================================================
  {
    "folke/which-key.nvim",
    optional = true,
    opts = {
      spec = {
        { "<leader>d", group = "debug" },
      },
    },
  },
}
