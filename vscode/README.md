# VSCode Settings Configuration

This directory contains VSCode settings that should be merged into your VSCode `settings.json` file.

> **Note**: Unlike Cursor settings (which are automatically symlinked), VSCode settings are **not** automatically linked because VSCode has built-in Settings Sync that handles syncing across devices. This file serves as a reference template - merge the settings once, and VSCode's native sync will handle distribution to your other devices.

## Setup Instructions

### Option 1: Manual Merge (Recommended)

1. Open VSCode Settings JSON:
   - Press `Ctrl+Shift+P` (or `Cmd+Shift+P` on macOS)
   - Type "Preferences: Open User Settings (JSON)"
   - Select it to open your `settings.json` file

2. Copy the relevant settings from `settings.json` in this directory

3. Merge them into your existing `settings.json`, being careful not to create duplicate keys

4. Save the file - changes take effect immediately

5. **Apply keybindings.json** (important for Neovim keybindings to work):
   - Press `Ctrl+Shift+P` (or `Cmd+Shift+P` on macOS)
   - Type "Preferences: Open Keyboard Shortcuts (JSON)"
   - Copy the contents from `keybindings.json` in this directory
   - Merge into your existing keybindings array

6. **Enable VSCode Settings Sync** (if not already enabled):
   - Press `Ctrl+Shift+P` → "Settings Sync: Turn On..."
   - Sign in with your GitHub or Microsoft account
   - Your settings will automatically sync to all your devices

### Option 2: Direct Copy (If you don't have custom settings)

```bash
# Linux/macOS
cp vscode/settings.json ~/.config/Code/User/settings.json

# Or for VSCode Insiders
cp vscode/settings.json ~/.config/Code\ -\ Insiders/User/settings.json
```

**Warning**: This will overwrite any existing settings! Use Option 1 if you have custom settings.

## Configuration Overview

### Word Wrap Configuration

- **Default**: Word wrap is **disabled** for all file types (`[*]`)
- **Markdown files**: Word wrap is **enabled** with 120 character column
- **Plain text files**: Word wrap is **enabled** with 120 character column

This ensures that only text and markdown files have word wrapping, preventing the vscode-neovim extension from wrapping code files (which makes editing difficult).

## vscode-neovim Settings

### Core Configuration

- **Neovim Executable Paths**: Configured for Linux, macOS, and Windows
- **Neovim Init Paths**: Points to `~/.config/dotfiles/nvim` on all platforms
- **Type Checking**: Enabled for Lua code validation in init files

### Performance Tuning

- **Debounce Times**: Optimized for smooth selection synchronization
  - `normalSelectionDebounceTime`: 50ms (for normal mode)
  - `visualSelectionDebounceTime`: 50ms (for visual mode)
  - `mouseSelectionDebounceTime`: 100ms (can be set to 0 to disable mouse synchronization)
- **Virtual Edit**: Disabled by default for better compatibility
- **Log Level**: Set to "Info" for useful debugging without spam

### Keyboard Customization

**Control Key Handling:**

- Defines which `Ctrl+key` combinations are handled by Neovim vs VSCode
- Comprehensive list for normal mode (navigate, search, delete, etc.)
- Simplified list for insert mode (common editing shortcuts)
- Adjust these arrays to customize your workflow

**Example:** `Ctrl+h` will be intercepted by Neovim to move cursor left instead of triggering VSCode's "Show Parameter Hints" command.

### Advanced Features

- **Relative Line Numbers**: Enabled for Vim-like navigation
- **Highlight Groups**: Enabled for better visual feedback
- **Mouse Selection**: Can be disabled by setting `mouseSelectionDebounceTime` to 0
- **WSL Support**: Available on Windows via `useWslBinary`
- **Logging**: Optional debugging logs can be enabled via `logPath` and `logOutputToConsole`

### Neovim-Specific Optimizations

The `nvim/lua/plugins/vscode.lua` file also applies VSCode-specific optimizations when Neovim runs inside VSCode:

- **Message Suppression**: Cleans up console output (`shortmess`)
- **Update Performance**: Lower updatetime (50ms) for better responsiveness
- **Syntax Optimization**: Optimized syntax highlighting for VSCode context
- **Automatic Wrap Control**: Smart word wrap based on file type

## Verifying the Configuration

After merging the settings:

1. Open a code file (`.py`, `.lua`, `.js`, etc.) - word wrap should be **OFF**
2. Open a markdown file (`.md`) - word wrap should be **ON**
3. Open a text file (`.txt`) - word wrap should be **ON**
4. Check Vim mode is working: Press `Esc` and try `dd`, `yy`, `p`, etc.
5. Test relative line numbers: Enter normal mode and see line numbers change

## Troubleshooting

### Word Wrap Issues

If word wrap is still enabled in code files:

1. Check that the `[*]` section is in your `settings.json` and doesn't have syntax errors
2. Verify the `editor.wordWrap` is set to `"off"` (string, not boolean)
3. Reload VSCode: `Ctrl+Shift+P` → "Developer: Reload Window"
4. Check the Neovim plugin configuration: `nvim/lua/plugins/vscode.lua`

### Performance Issues

If VSCode feels sluggish with vscode-neovim:

1. Increase debounce times:

   ```json
   "vscode-neovim.normalSelectionDebounceTime": 100
   "vscode-neovim.visualSelectionDebounceTime": 100
   "vscode-neovim.mouseSelectionDebounceTime": 200
   ```

2. Disable mouse synchronization:

   ```json
   "vscode-neovim.mouseSelectionDebounceTime": 0
   ```

3. Reduce logging:

   ```json
   "vscode-neovim.logLevel": "Error"
   ```

### Keyboard Shortcuts Conflicts

If Neovim commands don't work as expected (e.g., `Ctrl+D` does multi-cursor instead of page down):

1. **Apply the keybindings.json file** (most important):
   - Open Command Palette: `Ctrl+Shift+P`
   - Type "Preferences: Open Keyboard Shortcuts (JSON)"
   - Merge the contents from `keybindings.json` in this directory
   - The keybindings disable VS Code defaults when Neovim is active

2. Check `vscode-neovim.ctrlKeysForNormalMode` and `vscode-neovim.ctrlKeysForInsertMode` settings
3. Add conflicting keys to the appropriate arrays
4. Open Keyboard Shortcuts in VSCode and check for conflicts

**Common Conflicts:**

| Key      | VS Code Default     | Neovim Function            |
| -------- | ------------------- | -------------------------- |
| `Ctrl+D` | Multi-cursor select | Page down (half screen)    |
| `Ctrl+U` | Cursor undo         | Page up (half screen)      |
| `Ctrl+F` | Find                | Page forward (full screen) |
| `Ctrl+B` | Toggle sidebar      | Page back (full screen)    |
| `Ctrl+A` | Select all          | Increment number           |
| `Ctrl+W` | Close editor        | Window commands            |
| `Ctrl+O` | Open file           | Jump back                  |
| `Ctrl+I` | Trigger suggest     | Jump forward               |

## Common Developer Configurations

### Minimal Setup

Keep default settings for balanced performance and compatibility.

### High Performance

Disable mouse synchronization and increase debounce times for better performance on slower machines.

### Maximum Vim Experience

Enable all Vim features, customize control key arrays, and use comprehensive Neovim configuration.

## Additional Resources

- [vscode-neovim GitHub Repository](https://github.com/vscode-neovim/vscode-neovim)
- [Neovim Configuration](./nvim/README.md)
- [Neovim VSCode Plugin](./nvim/lua/plugins/vscode.lua)
