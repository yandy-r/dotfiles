# Smartsheet CLI - Engineer's Project Management Tool

A comprehensive TypeScript CLI tool for managing Smartsheet projects, designed specifically for engineers who need to track tasks, create projects, and manage workflows programmatically.

## Features

- **List Resources**: View all sheets, workspaces, and templates
- **Create Sheets**: Generate new project sheets from engineer-focused templates
- **Add Tasks**: Quickly add tasks with status, assignee, priority, and due dates
- **Update Tasks**: Modify task status and properties
- **Project Templates**: Pre-configured templates for different engineering workflows

## Installation

### Quick Install

```bash
cd ~/.config/dotfiles/bin
./install-smartsheet-cli.sh
```

This will:

1. Check Node.js version (requires 18+)
2. Install npm dependencies
3. Create symlink to `/usr/local/bin/smartsheet-cli`

### Manual Installation

```bash
cd ~/.config/dotfiles/bin
npm install
chmod +x smartsheet-cli.ts
ln -s $(pwd)/smartsheet-cli.ts /usr/local/bin/smartsheet-cli
```

## Configuration

### API Key Setup

1. Get your API key from Smartsheet:
   - Go to <https://app.smartsheet.com>
   - Click on your profile → **Account** → **Personal Settings** → **API Access**
   - Generate a new access token

2. Add to your environment (choose one method):

   **Option A: Using .env file (Recommended)**

   ```bash
   # In your project or ~/.config/dotfiles/.env
   SMARTSHEETS_API_KEY="your-api-key-here"
   ```

   **Option B: Shell export**

   ```bash
   export SMARTSHEETS_API_KEY="your-api-key-here"
   ```

   **Option C: Using 1Password (Most Secure)**

   ```bash
   # In your .env file
   SMARTSHEETS_API_KEY="op://RFMedia Secrets/Whiskey Detective - Secrets/SMARTSHEETS_API_KEY"
   ```

## Usage

### List Available Templates

View all pre-configured project templates:

```bash
smartsheet-cli list templates
```

Available templates:

- **standard**: Basic task tracking for engineering projects
- **sprint**: Agile sprint planning and tracking
- **bugs**: Bug tracking and management
- **releases**: Release planning and milestone tracking

### List Sheets

View all your Smartsheet sheets:

```bash
# Basic list
smartsheet-cli list sheets

# Verbose output with details
smartsheet-cli list sheets -v
```

### List Workspaces

View all available workspaces:

```bash
smartsheet-cli list workspaces
```

### Get Sheet Details

View detailed information about a specific sheet:

```bash
smartsheet-cli get sheet <sheet-id>
```

This displays:

- Sheet metadata (name, ID, row count, column count)
- All columns and their types
- Preview of recent tasks (first 10 rows)

### Create a New Sheet

#### Interactive Mode (Recommended for First Time)

```bash
smartsheet-cli create sheet "My New Project" -i
```

This will prompt you to select a template interactively.

#### Using Specific Template

```bash
# Standard engineering project
smartsheet-cli create sheet "Backend API Project" -t standard

# Sprint planning
smartsheet-cli create sheet "Sprint 23" -t sprint

# Bug tracking
smartsheet-cli create sheet "Bug Triage Q1 2025" -t bugs

# Release planning
smartsheet-cli create sheet "v2.0 Release" -t releases
```

#### Default Template

```bash
# Creates with standard template by default
smartsheet-cli create sheet "Quick Project"
```

### Add Tasks to a Sheet

#### Basic Task

```bash
smartsheet-cli add task <sheet-id> "Implement user authentication"
```

#### Task with All Properties

```bash
smartsheet-cli add task <sheet-id> "Fix login bug" \
  --status "In Progress" \
  --assignee "engineer@company.com" \
  --priority "High" \
  --due-date "2025-02-01"
```

#### Task with Some Properties

```bash
# Just status and priority
smartsheet-cli add task <sheet-id> "Code review" \
  --status "Not Started" \
  --priority "Medium"

# Just assignee
smartsheet-cli add task <sheet-id> "Write tests" \
  --assignee "qa@company.com"
```

### Update Task Status

```bash
smartsheet-cli update status <sheet-id> <row-id> "Completed"
```

Available statuses (depends on template):

- **Standard**: Not Started, In Progress, Completed, Blocked, On Hold
- **Bugs**: Open, In Progress, Fixed, Verified, Closed

## Project Templates

### Standard Engineering Project

Best for: General task tracking, feature development, maintenance work

**Columns:**

- Task Name (primary)
- Status (Not Started, In Progress, Completed, Blocked, On Hold)
- Assignee (contact)
- Priority (Low, Medium, High, Critical)
- Due Date
- Est. Hours
- Description

**Use Cases:**

- Backend API development
- Infrastructure projects
- General engineering tasks
- Maintenance and tech debt

### Sprint Planning

Best for: Agile/Scrum teams, iteration planning

**Columns:**

- Story/Task (primary)
- Type (Story, Task, Bug, Tech Debt)
- Status (Not Started, In Progress, Completed, Blocked, On Hold)
- Assignee (contact)
- Story Points
- Sprint
- Priority (Low, Medium, High, Critical)

**Use Cases:**

- 2-week sprint planning
- Story point estimation
- Backlog grooming
- Sprint retrospectives

### Bug Tracking

Best for: Issue tracking, QA workflows

**Columns:**

- Bug Title (primary)
- Severity (Critical, High, Medium, Low)
- Status (Open, In Progress, Fixed, Verified, Closed)
- Assignee (contact)
- Found In (version)
- Fixed In (version)
- Reporter (contact)
- Description

**Use Cases:**

- Production bug tracking
- QA testing cycles
- Regression testing
- Release blocker management

### Release Planning

Best for: Milestone tracking, version releases, roadmap planning

**Columns:**

- Feature/Task (primary)
- Release (version)
- Status (Not Started, In Progress, Completed, Blocked, On Hold)
- Owner (contact)
- Target Date
- Dependencies
- Notes

**Use Cases:**

- Major version planning
- Feature roadmaps
- Release coordination
- Dependency tracking

## Common Workflows

### Starting a New Project

```bash
# 1. Create the sheet
smartsheet-cli create sheet "Q1 2025 API Redesign" -t standard

# Output: Sheet ID: 1234567890

# 2. Add initial tasks
smartsheet-cli add task 1234567890 "Design API endpoints" \
  --status "Not Started" \
  --priority "High" \
  --assignee "lead@company.com"

smartsheet-cli add task 1234567890 "Set up development environment" \
  --status "In Progress" \
  --assignee "devops@company.com"

smartsheet-cli add task 1234567890 "Write API documentation" \
  --status "Not Started" \
  --due-date "2025-02-15"
```

### Managing a Sprint

```bash
# 1. Create sprint sheet
smartsheet-cli create sheet "Sprint 24 - Feb 2025" -t sprint

# 2. Add stories
smartsheet-cli add task <sheet-id> "User can reset password" \
  --status "Not Started" \
  --assignee "dev1@company.com"

# 3. Track progress
smartsheet-cli get sheet <sheet-id>

# 4. Update completed tasks
smartsheet-cli update status <sheet-id> <row-id> "Completed"
```

### Bug Triage

```bash
# 1. Create bug tracking sheet
smartsheet-cli create sheet "Production Bugs - Q1 2025" -t bugs

# 2. Log bugs as they're found
smartsheet-cli add task <sheet-id> "Login fails on Safari" \
  --priority "High"

smartsheet-cli add task <sheet-id> "Slow database query on reports page" \
  --priority "Medium"

# 3. Review all bugs
smartsheet-cli get sheet <sheet-id>
```

## Advanced Usage

### Using with Scripts

```bash
#!/usr/bin/env bash
# Example: Daily task creation from GitHub issues

SHEET_ID="1234567890"

# Fetch GitHub issues and create Smartsheet tasks
gh issue list --json title,assignees | jq -r '.[] |
  "smartsheet-cli add task '$SHEET_ID' \"\\(.title)\" --assignee \\(.assignees[0].email)"
' | bash
```

### Automation with Cron

```bash
# Add to crontab: Daily project status report
0 9 * * * smartsheet-cli get sheet 1234567890 | mail -s "Daily Status" team@company.com
```

### Integration with CI/CD

```yaml
# .github/workflows/update-smartsheet.yml
name: Update Smartsheet on Deploy
on:
  push:
    branches: [main]
jobs:
  update:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v3
      - run: npm install -g smartsheet-cli
      - run: |
          smartsheet-cli add task ${{ secrets.SHEET_ID }} \
            "Deploy ${{ github.sha }}" \
            --status "Completed" \
            --due-date "$(date +%Y-%m-%d)"
        env:
          SMARTSHEETS_API_KEY: ${{ secrets.SMARTSHEETS_API_KEY }}
```

## Troubleshooting

### API Key Not Found

**Error:** `Error: SMARTSHEETS_API_KEY environment variable not set`

**Solutions:**

1. Check if `.env` file exists and contains the key
2. Verify direnv is installed and allowed: `direnv allow`
3. Try exporting directly: `export SMARTSHEETS_API_KEY="your-key"`
4. Reload shell: `exec $SHELL -l`

### Authentication Failed

**Error:** `401 Unauthorized` or `Invalid access token`

**Solutions:**

1. Verify API key is correct (no extra spaces/quotes)
2. Check API key hasn't expired in Smartsheet settings
3. Ensure API access is enabled for your account
4. Generate a new API key if needed

### Sheet Not Found

**Error:** `404 Not Found` or `Sheet does not exist`

**Solutions:**

1. Verify sheet ID is correct: `smartsheet-cli list sheets`
2. Check you have access to the sheet in Smartsheet UI
3. Ensure sheet wasn't deleted or moved

### Rate Limiting

**Error:** `429 Too Many Requests`

**Solution:**

- Smartsheet has a 300 requests/minute limit
- Wait a minute and retry
- Use batch operations when possible

### Node.js Version Issues

**Error:** `Unsupported Node.js version`

**Solutions:**

```bash
# Check version
node --version

# Upgrade if needed (macOS)
brew upgrade node

# Or use nvm
nvm install 18
nvm use 18
```

## API Reference

### List Commands

```bash
smartsheet-cli list sheets [-v|--verbose]
smartsheet-cli list workspaces
smartsheet-cli list templates
```

### Get Commands

```bash
smartsheet-cli get sheet <sheet-id>
```

### Create Commands

```bash
smartsheet-cli create sheet <name> [-t|--template <type>] [-i|--interactive]
```

### Add Commands

```bash
smartsheet-cli add task <sheet-id> <task-name> [options]

Options:
  -s, --status <status>       Task status
  -a, --assignee <email>      Assignee email
  -p, --priority <priority>   Task priority
  -d, --due-date <date>       Due date (YYYY-MM-DD)
```

### Update Commands

```bash
smartsheet-cli update status <sheet-id> <row-id> <status>
```

## Tips & Best Practices

### 1. Use Consistent Naming

```bash
# Good: Clear, descriptive names
smartsheet-cli create sheet "Backend API - Q1 2025"
smartsheet-cli create sheet "Bug Tracking - Production"

# Avoid: Vague names
smartsheet-cli create sheet "Project"
smartsheet-cli create sheet "Tasks"
```

### 2. Leverage Templates

Choose the right template for your workflow:

- **standard**: 80% of projects
- **sprint**: Agile teams
- **bugs**: QA and issue tracking
- **releases**: Long-term planning

### 3. Keep Sheet IDs Handy

```bash
# Save frequently used sheet IDs
export MAIN_PROJECT_SHEET="1234567890"
export BUG_SHEET="0987654321"

# Use in commands
smartsheet-cli add task $MAIN_PROJECT_SHEET "New feature"
```

### 4. Automate Repetitive Tasks

Create aliases for common operations:

```bash
# In your .bashrc or .zshrc
alias ss-list='smartsheet-cli list sheets'
alias ss-add='smartsheet-cli add task'

# Usage
ss-list
ss-add $MAIN_PROJECT_SHEET "Quick task"
```

### 5. Use with Other Tools

```bash
# Combine with jq for filtering
smartsheet-cli list sheets -v | grep "API"

# Pipe to file for reporting
smartsheet-cli get sheet 1234567890 > status-report.txt
```

## Future Enhancements

Ideas for additional features (contributions welcome):

- [ ] Bulk task import from CSV/JSON
- [ ] Task dependency management
- [ ] Attachment uploads
- [ ] Comments and discussions
- [ ] Custom column creation
- [ ] Row deletion and archiving
- [ ] Search and filter tasks
- [ ] Export to various formats
- [ ] Workspace and folder management
- [ ] User and sharing management
- [ ] Template customization
- [ ] Gantt chart data export
- [ ] Integration with Jira/GitHub Issues

## Contributing

Found a bug or want to add a feature? Here's how:

1. The script is located at: `~/.config/dotfiles/bin/smartsheet-cli.ts`
2. Edit the TypeScript file
3. Test your changes: `tsx smartsheet-cli.ts <command>`
4. Submit improvements to the dotfiles repo

## Resources

- **Smartsheet API Docs**: <https://developers.smartsheet.com/api>
- **Node.js SDK**: <https://github.com/smartsheet/smartsheet-javascript-sdk>
- **API Rate Limits**: 300 requests/minute
- **Support**: <https://help.smartsheet.com>

## License

Part of the dotfiles repository - use freely for personal and commercial projects.

---

**Happy Project Managing!** 🚀
