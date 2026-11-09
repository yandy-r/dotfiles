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
| `mods/broadcast/` | `mods/broadcast/` | Send one command to many terminals (like tmux `synchronize-panes`) |

The app writes through the symlinks. Changes made in Settings or with the zoom keys land
here and show up in `git status`.

## Prefix keys

The app can't bind multi-key sequences, so `ctrl-b` opens the `prefix` popup. It runs the
action for the next key, then closes. Esc cancels, and `ctrl-b` twice sends a literal
`ctrl-b` to the program. The table in `mods/prefix/main.js` holds the mappings.
Terminal-herdr actions with no app equivalent (`o` last pane, `[` copy mode, `shift-o`,
`shift-r`, the nvim plugin keys) are left unbound.

`herdr-app check ~/.config/herdr-app/mods/prefix` validates the mod after an edit.

## Broadcast

The `broadcast` dialog types one command into many terminals at once. Each target gets it
through herdr-core's `terminal.send` API (via `hc connect`, as a person rather than a program,
so remote cores don't wrap it in `<hc from=…>` agent tags), which pastes the text and presses
Enter. A toast reports how many sends
succeeded and lists any that failed. Open it from the prefix with a preset, or with
`herdr-app do mod.open --name broadcast`, which reuses your last choices:

| Prefix key | Sends to |
| --- | --- |
| `ctrl-b a` | Every terminal in this tab |
| `ctrl-b A` | Every terminal in this space |
| `ctrl-b e` | Every space on this computer (local only) |
| `ctrl-b R` | Every space on remote machines only |
| `ctrl-b E` | Every space on every machine |

In the dialog you can change any preset before sending. You can also turn individual machines
on or off. A live list shows exactly which terminals the command will reach. Agents (Claude
Code and other terminals that report a status) are skipped by default. The dialog's keys:

- `Enter` sends, `Esc` closes, `Up`/`Down` step through the last 30 commands
- `alt-1`/`alt-2`/`alt-3` pick this tab, this space or all spaces
- `alt-l`/`alt-r`/`alt-a` pick local, remote or all machines
- `alt-g` toggles skipping agents, `alt-i` toggles the focused terminal

Your choices and history persist in Herdr's shared state (`broadcast.prefs`,
`broadcast.history`). Remote terminals are addressed as `machine/tN`, the same way `hc ls`
shows them.
