# WezTerm keybindings

The default keyboard layout follows the explicit bindings in [Ghostty's config](../ghostty/config).
Press **Ctrl+a**, release it, then press a command key. The prefix waits indefinitely;
**Escape** cancels it. Uppercase commands require Shift.

## Prefix commands

| Command after Ctrl+a  | Action                                                        |
| --------------------- | ------------------------------------------------------------- |
| `v` or `\|`           | Split right                                                   |
| `s` or `-`            | Split down                                                    |
| `h` / `j` / `k` / `l` | Focus left / down / up / right pane                           |
| `o`                   | Cycle to the previous pane                                    |
| `c`                   | New tab                                                       |
| `n` or Space          | Next tab                                                      |
| `p` or Backspace      | Previous tab                                                  |
| `g`                   | Pick a tab in the current window                              |
| `1`–`8`               | Select numbered tab; numbers beyond the count select the last |
| `9`                   | Select last tab                                               |
| `x` / `X`             | Close pane / tab                                              |
| `Q`                   | Quit WezTerm                                                  |
| `z`                   | Toggle pane zoom                                              |
| `]` or `P`            | Paste from clipboard                                          |
| `R`                   | Reload configuration                                          |
| `L`                   | Clear screen and scrollback                                   |
| `a`                   | Send a literal Ctrl+a to the terminal                         |

## Direct shortcuts

| Shortcut                    | Action                                    |
| --------------------------- | ----------------------------------------- |
| Alt+arrows                  | Focus adjacent pane                       |
| Ctrl+Alt+h/j/k/l            | Resize left/down/up/right by one cell     |
| Ctrl+Shift+Left/Right       | Move tab left/right, wrapping at the ends |
| Ctrl+Tab / Ctrl+Shift+Tab   | Next / previous tab                       |
| Ctrl+Shift+F                | Search                                    |
| Ctrl+Shift+C / Ctrl+Shift+V | Copy / paste                              |
| Ctrl+Shift+Enter            | Toggle pane zoom                          |
| Ctrl+Enter                  | Toggle fullscreen                         |
| Shift+PageUp/PageDown       | Scroll a full page                        |
| Shift+Home/End              | Scroll to the top / bottom                |

**Super** means Command on macOS and the Super/Windows key on Linux. The secondary
Ghostty shortcuts are Super+N (new window), Super+, (open the active config in the
OS file handler), Super+=/-/0 (font size), Super+C/V (clipboard), and Super+Shift+F
(fullscreen). Hyprland may intercept Super shortcuts; use the primary shortcuts there.
Alt+Enter is passed through to terminal applications.

## Retained WezTerm extras

- Ctrl+Shift+Space or Ctrl+Shift+X: copy mode; built-in copy/search mode keys remain available.
- Ctrl+Shift+h/j/k/l and Ctrl+Shift+Up/Down: pane navigation.
- Ctrl+Alt+arrows: one-cell pane resizing.
- Ctrl+Shift+T: cycle theme; Ctrl+Alt+=/-: change opacity.
- Ctrl+Shift+P: command palette; Ctrl+Alt+S: SSH launch; Ctrl+Alt+R: command prompt.
- Super+T/W: create/close tab; Super+1–9 and Super+Left/Right: select/cycle tabs.
- Super+D / Super+Shift+D: split right/down; Super+X: close pane; Super+Z: zoom pane.
- Super+E: rename tab; Super+O: quick select; Super+Shift+O: open a selected URL.
- Super+F/G and F3: additional search modes; Super+U: Unicode picker; Super+K: clear/redraw.
- Super+Enter / Super+Shift+Enter: new terminal tab/window; Super+Q: quit; Super+M: hide.
- Shift+Up/Down: line scrolling; Ctrl+=/-/0 and Ctrl+Shift+=/-: font-size shortcuts.
- Existing link opening, selection copy/paste, and mouse font sizing remain enabled.

Super shortcuts replace the previous Linux Alt aliases. Ctrl+Shift+Left/Right now
reorder tabs. Duplicate debug, pane-rotation, and scrollback-clear shortcuts were
removed in favor of pane navigation. The unused WEZTERM_TMUX_MODE and
WEZTERM_VIM_MODE switches no longer configure these bindings.

## Differences from Ghostty

- Ctrl+a then E or Enter does not equalize splits: WezTerm has no native equivalent.
- An unmatched prefix command exits prefix mode and follows normal handling for
  its second key. Unlike Ghostty, the consumed Ctrl+a is not replayed.
- Resize steps are one terminal cell, approximating Ghostty's 10-pixel step.
- The tab picker is WezTerm's stable tab-only launcher, rather than Ghostty's overview.
- Ctrl+a then Q implements the config's intended quit behavior. Ghostty's legacy
  close_all_windows action is deprecated; it is not copied as a no-op.

## Validation

From the repository root, run Lua syntax checks and load the checkout with a real
WezTerm binary. XDG_CONFIG_HOME isolates the modular configuration from any installed copy:

```sh
luac -p wezterm/*.lua
XDG_CONFIG_HOME="$PWD" wezterm show-keys --lua
```

In WezTerm, verify prefix waiting/cancellation, shifted split/close commands,
numbered-tab fallback, tab-reordering wraparound, pane navigation/resizing,
clipboard/search/copy modes, theme switching, SSH launch, and modifier behavior.
