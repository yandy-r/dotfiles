return {
  {
    "MTDL9/vim-log-highlighting",
    ft = { "log", "*.log" },
    lazy = true,
  },
  {
    "fei6409/log-highlight.nvim",
    ft = { "log" },
    config = function()
      require("log-highlight").setup({
        extension = "log",
        filename = {
          "messages",
          "syslog",
          "boot.log",
          "kern.log",
        },
        pattern = {
          "/var/log/.*",
          ".*%.log",
          ".*%.log%.%d+",
        },
      })
    end,
  },
}
