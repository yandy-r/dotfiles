-- blink.cmp v2 — tracks `main`, requires nvim 0.12+ and `saghen/blink.lib`.
-- v2 moved the build step from `cargo build --release` to the plugin's own
-- builder (native lib path changed from `target/release/` to `lib/`), so the
-- LazyVim extra's v1 defaults are overridden here.
return {
  "saghen/blink.cmp",
  version = false, -- follow `main`; v2 has no release tag yet
  dependencies = { "saghen/blink.lib" },
  build = function()
    require("blink.cmp").build():pwait()
  end,
}
