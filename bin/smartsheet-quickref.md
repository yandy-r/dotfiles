# Smartsheet CLI - Quick Reference

## Installation

```bash
cd ~/.config/dotfiles/bin && ./install-smartsheet-cli.sh
```

## Setup API Key

```bash
# Add to .env
SMARTSHEETS_API_KEY="your-api-key-here"
```

Get key: <https://app.smartsheet.com> → Account → API Access

## Common Commands

### List & View

```bash
smartsheet-cli list sheets              # All sheets
smartsheet-cli list workspaces          # All workspaces
smartsheet-cli list templates           # Available templates
smartsheet-cli get sheet <id>           # Sheet details
```

### Create

```bash
# Interactive template selection
smartsheet-cli create sheet "Project Name" -i

# Specific templates
smartsheet-cli create sheet "My Project" -t standard
smartsheet-cli create sheet "Sprint 1" -t sprint
smartsheet-cli create sheet "Bugs Q1" -t bugs
smartsheet-cli create sheet "v2.0" -t releases
```

### Add Tasks

```bash
# Basic task
smartsheet-cli add task <sheet-id> "Task name"

# With all properties
smartsheet-cli add task <sheet-id> "Fix bug" \
  --status "In Progress" \
  --assignee "dev@company.com" \
  --priority "High" \
  --due-date "2025-02-01"
```

### Update

```bash
smartsheet-cli update status <sheet-id> <row-id> "Completed"
```

## Templates

| Template   | Best For       | Key Columns                                 |
| ---------- | -------------- | ------------------------------------------- |
| `standard` | General tasks  | Task, Status, Assignee, Priority, Due Date  |
| `sprint`   | Agile teams    | Story, Type, Points, Sprint, Status         |
| `bugs`     | Issue tracking | Bug, Severity, Status, Found/Fixed In       |
| `releases` | Roadmaps       | Feature, Release, Target Date, Dependencies |

## Status Values

**Standard/Sprint/Releases:**

- Not Started
- In Progress
- Completed
- Blocked
- On Hold

**Bugs:**

- Open
- In Progress
- Fixed
- Verified
- Closed

## Priority Values

- Low
- Medium
- High
- Critical

## Quick Examples

```bash
# Create project and add tasks
smartsheet-cli create sheet "Backend API" -t standard
smartsheet-cli add task 1234567890 "Setup DB" --priority "High"
smartsheet-cli add task 1234567890 "Write tests" --assignee "qa@co.com"

# Sprint workflow
smartsheet-cli create sheet "Sprint 24" -t sprint
smartsheet-cli add task 9876543210 "User login" --status "In Progress"
smartsheet-cli update status 9876543210 12345 "Completed"

# Bug tracking
smartsheet-cli create sheet "Production Bugs" -t bugs
smartsheet-cli add task 5555555555 "Login broken" --priority "Critical"
```

## Troubleshooting

| Issue               | Solution                                   |
| ------------------- | ------------------------------------------ |
| API key not found   | Check `.env` file, run `direnv allow`      |
| 401 Unauthorized    | Verify API key in Smartsheet settings      |
| 404 Sheet not found | Run `smartsheet-cli list sheets` to get ID |
| 429 Rate limit      | Wait 1 minute (300 req/min limit)          |

## Full Docs

See `SMARTSHEET_CLI.md` for complete documentation.
