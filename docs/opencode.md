# OpenCode AI Coding Agent

OpenCode is an AI coding agent built for the terminal, integrated into this dotfiles repository for seamless installation across all supported platforms.

## Installation

OpenCode is automatically installed when you run the bootstrap script:

```bash
./bootstrap.sh
```

### Manual Installation

You can manually install or update OpenCode using the custom installation script:

```bash
# Install OpenCode
./bin/install-opencode.sh --install

# Update to latest version
./bin/install-opencode.sh --update

# Force reinstall
./bin/install-opencode.sh --force

# Uninstall
./bin/install-opencode.sh --uninstall
```

## Platform-Specific Installation Methods

The installer automatically chooses the best method for your platform:

### macOS

1. **Homebrew** (preferred): `brew install sst/tap/opencode`
2. **Official script**: `curl -fsSL https://opencode.ai/install | bash`
3. **npm**: `npm install -g opencode-ai`

### Linux (Ubuntu, Fedora, openSUSE, WSL)

1. **Official script** (preferred): `curl -fsSL https://opencode.ai/install | bash`
2. **npm**: `npm install -g opencode-ai`

### Linux (Arch)

1. **Paru** (preferred): `paru -S opencode-bin`
2. **Official script**: `curl -fsSL https://opencode.ai/install | bash`
3. **npm**: `npm install -g opencode-ai`

## Getting Started

### Initialize a Project

Navigate to your project directory and initialize OpenCode:

```bash
cd /path/to/your/project
opencode
```

Then run the initialization command inside OpenCode:

```
/init
```

This creates an `AGENTS.md` file that helps OpenCode understand your project structure.

### Basic Usage

OpenCode runs in your terminal and provides several modes:

#### Build Mode (Default)

In Build mode, OpenCode can make changes to your codebase:

```
Can you add error handling to the login function?
```

#### Plan Mode

Press **Tab** to switch to Plan mode, where OpenCode suggests changes without implementing them:

```
<TAB>
How would you refactor the authentication system?
```

Press **Tab** again to return to Build mode.

### Common Commands

- `/init` - Initialize OpenCode for the current project
- `/share` - Create a shareable link to your conversation
- `/undo` - Undo the last changes made by OpenCode
- `/redo` - Redo previously undone changes
- `/help` - Show available commands

### Working with Files

Use the `@` key to fuzzy search and reference files:

```
How is authentication handled in @src/auth/login.ts
```

You can also drag and drop images into the terminal to add them to prompts.

## Configuration

OpenCode can be configured per project or globally. For detailed configuration options, visit:

- **Official Documentation**: <https://opencode.ai/docs>
- **Configuration Guide**: <https://opencode.ai/docs/configure>

### Common Configurations

Create a `.opencode` directory in your project:

```bash
mkdir .opencode
```

#### Custom Rules

Add project-specific rules in `.opencode/rules.md`:

```markdown
# Project Coding Standards

- Use TypeScript strict mode
- Follow ESLint rules
- Write tests for all new features
- Use conventional commits
```

#### Custom Agents

Define specialized agents in `.opencode/agents.md`:

```markdown
# Testing Agent

You are an expert at writing comprehensive tests.
Focus on edge cases and error handling.
```

## Integration with Dotfiles

OpenCode is integrated into this dotfiles repository in several ways:

### Tool Detection

OpenCode is registered in `common/tools.sh` as a development tool:

```bash
# Check if OpenCode is installed
tool_exists opencode

# Get OpenCode version
tool_version opencode

# Install OpenCode via tools system
install_tool opencode
```

### System Health Check

Check OpenCode installation status:

```bash
# Source the tools system
source ~/.config/dotfiles/common/tools.sh

# Run system health check (includes OpenCode)
system_health

# List missing tools
list_missing_tools
```

### Bootstrap Integration

OpenCode is automatically installed during initial setup when you run:

```bash
./bootstrap.sh
```

## Prerequisites

OpenCode works best with modern terminal emulators that support:

- **True color** (24-bit color)
- **Unicode and emoji** rendering
- **Modern escape sequences**

### Recommended Terminals

This dotfiles repository includes configurations for:

- **WezTerm** (cross-platform, configured in `wezterm/`)
- **Ghostty** (Linux/macOS, configured in `ghostty/`)
- **Alacritty** (cross-platform)
- **Kitty** (Linux/macOS)

## LLM Provider Setup

OpenCode supports multiple LLM providers. The easiest way to get started is with OpenCode Zen:

```bash
# Start authentication flow
opencode auth login

# Select 'opencode' provider
# Visit https://opencode.ai/auth
# Sign in and add billing details
# Copy and paste your API key
```

### Alternative Providers

OpenCode also supports:

- **OpenAI** (GPT-4, GPT-4 Turbo)
- **Anthropic** (Claude 3.5 Sonnet, Claude 3 Opus)
- **Azure OpenAI**
- **AWS Bedrock**
- **Google Vertex AI**
- **Ollama** (local models)

Run `opencode auth login` and select your preferred provider.

## Tips and Best Practices

### 1. Give Context

Provide enough details for OpenCode to understand what you want:

```
We need to add pagination to the user list endpoint.
The endpoint is in @src/api/users.ts.
Look at how pagination is implemented in @src/api/posts.ts as a reference.
```

### 2. Use Plan Mode for Complex Changes

For significant refactoring or new features, use Plan mode first:

```
<TAB>  # Switch to Plan mode
Refactor the authentication system to use JWT tokens instead of sessions.
We'll need to update the middleware, API endpoints, and frontend.
```

Review the plan, iterate on it, then switch to Build mode:

```
<TAB>  # Switch to Build mode
Looks good! Go ahead and implement the changes.
```

### 3. Undo and Iterate

Don't hesitate to use `/undo` if changes aren't quite right:

```
/undo
Actually, can you use a different approach for error handling?
Instead of throwing exceptions, return Result types.
```

### 4. Share Conversations

Share useful conversations with your team:

```
/share
```

This creates a link you can send to colleagues for reference.

## Troubleshooting

### Command Not Found

If `opencode` is not found after installation:

1. **Check PATH**: Ensure your shell's PATH includes the installation directory
   - npm global bin: `npm config get prefix`/bin
   - Homebrew: `/opt/homebrew/bin` (macOS ARM) or `/usr/local/bin` (macOS Intel)

2. **Restart shell**: `exec zsh` or open a new terminal

3. **Verify installation**:

   ```bash
   which opencode
   opencode --version
   ```

### Installation Failed

If automatic installation fails:

```bash
# Try manual installation with npm
npm install -g opencode-ai

# Or use the official script
curl -fsSL https://opencode.ai/install | bash
```

### Update Issues

If updates fail, try forcing a reinstall:

```bash
./bin/install-opencode.sh --force
```

## Resources

- **Official Website**: <https://opencode.ai>
- **Documentation**: <https://opencode.ai/docs>
- **GitHub**: <https://github.com/sst/opencode>
- **Discord Community**: Join for support and discussions

## Related Tools

This dotfiles repository also includes:

- **Claude CLI**: AI-powered coding assistant from Anthropic
  - Install: `./bin/install-claude.sh`
  - Documentation: See Claude CLI documentation

Both OpenCode and Claude CLI provide AI pair programming capabilities with different approaches and features. Feel free to use whichever fits your workflow best!

---

**Happy coding with AI! 🤖✨**
