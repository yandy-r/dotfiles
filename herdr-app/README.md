# Herdr app

Config for the [Herdr](https://herdr.dev) desktop app (`herdr-app`). The app owns
`~/.config/herdr-app` and rewrites much of it (layouts, `.state/`, `AGENTS.md`, the `.d.ts`
files, view and window state), so `dotf link` links only these files, one by one:

| Source | Linked to `~/.config/herdr-app/` | What it holds |
| --- | --- | --- |
| `keys.json` | `keys.json` | Every shortcut, plus `ctrl-b` opening the prefix mod |
| `settings.json` | `settings.json` | Theme, font, sounds, telemetry off |
| `themes/catppuccin-mocha.json` | `themes/catppuccin-mocha.json` | Catppuccin Mocha, the same palette as `ghostty/config` |
| `mods/prefix/` | `mods/prefix/` | tmux-style `ctrl+b` prefix, matching `herdr/config.toml` |

The app writes through the symlinks. Changes made in Settings or with the zoom keys land
here and show up in `git status`.

## Prefix keys

The app can't bind multi-key sequences, so `ctrl-b` opens the `prefix` popup. It runs the
action for the next key, then closes. Esc cancels, and `ctrl-b` twice sends a literal
`ctrl-b` to the program. The table in `mods/prefix/main.js` holds the mappings.
Terminal-herdr actions with no app equivalent (`o` last pane, `[` copy mode, `shift-o`,
`shift-r`, the nvim plugin keys) are left unbound.

`herdr-app check ~/.config/herdr-app/mods/prefix` validates the mod after an edit.
