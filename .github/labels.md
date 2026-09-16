# GitHub Labels for dotfiles

Apply these labels to keep issues and pull requests consistently categorized. The `area:` and `platform:` families below are tuned for this repository.

## Label Families

| Label                 | Family     | Description                       |
| --------------------- | ---------- | --------------------------------- |
| `type:bug`            | type       | Something isn't working           |
| `type:feature`        | type       | New functionality                 |
| `type:docs`           | type       | Documentation only                |
| `type:refactor`       | type       | No functional changes             |
| `type:perf`           | type       | Performance improvement           |
| `type:test`           | type       | Adding or fixing tests            |
| `type:build`          | type       | Build system or dependencies      |
| `type:ci`             | type       | CI/CD pipeline changes            |
| `type:chore`          | type       | Routine maintenance               |
| `type:migration`      | type       | Migration or upgrade              |
| `type:security`       | type       | Security fix or audit             |
| `type:compatibility`  | type       | Cross-platform compatibility fix  |
| `area:shell`          | area       | bash/zsh configuration            |
| `area:nvim`           | area       | Neovim configuration              |
| `area:terminal`       | area       | wezterm/ghostty/iterm2/alacritty  |
| `area:wm`             | area       | aerospace/hyprland/waybar         |
| `area:tooling`        | area       | aliases, completions, common/     |
| `area:bootstrap`      | area       | install scripts, bootstrap.sh     |
| `area:ansible`        | area       | playbooks/ and symlink management |
| `area:docs`           | area       | CLAUDE.md/AGENTS.md/README        |
| `platform:linux`      | platform   | Linux-specific                    |
| `platform:macos`      | platform   | macOS-specific                    |
| `platform:wsl`        | platform   | WSL-specific                      |
| `priority:critical`   | priority   | Must fix immediately              |
| `priority:high`       | priority   | Fix in current milestone          |
| `priority:medium`     | priority   | Fix in next milestone             |
| `priority:low`        | priority   | Nice to have                      |
| `status:needs-triage` | status     | Awaiting review                   |
| `status:in-progress`  | status     | Actively being worked on          |
| `status:blocked`      | status     | Blocked by external factor        |
| `status:needs-info`   | status     | Waiting for more information      |
| `good first issue`    | standalone | Suitable for newcomers            |
| `help wanted`         | standalone | Extra attention needed            |
| `duplicate`           | standalone | Already reported                  |
| `wontfix`             | standalone | Intentionally not addressed       |
| `regression`          | standalone | Broke something that worked       |

## Apply with `gh label create`

```bash
# type:
gh label create "type:bug"           --color "d73a4a" --description "Something isn't working"
gh label create "type:feature"       --color "a2eeef" --description "New functionality"
gh label create "type:docs"          --color "0075ca" --description "Documentation only"
gh label create "type:refactor"      --color "e4e669" --description "No functional changes"
gh label create "type:perf"          --color "f9d0c4" --description "Performance improvement"
gh label create "type:test"          --color "bfd4f2" --description "Adding or fixing tests"
gh label create "type:build"         --color "c5def5" --description "Build system or dependencies"
gh label create "type:ci"            --color "c2e0c6" --description "CI/CD pipeline changes"
gh label create "type:chore"         --color "eeeeee" --description "Routine maintenance"
gh label create "type:migration"     --color "fbca04" --description "Migration or upgrade"
gh label create "type:security"      --color "ee0701" --description "Security fix or audit"
gh label create "type:compatibility" --color "bfe5bf" --description "Cross-platform compatibility fix"

# area:
gh label create "area:shell"     --color "1d76db" --description "bash/zsh configuration"
gh label create "area:nvim"      --color "1d76db" --description "Neovim configuration"
gh label create "area:terminal"  --color "1d76db" --description "Terminal emulators"
gh label create "area:wm"        --color "1d76db" --description "Window manager configs"
gh label create "area:tooling"   --color "1d76db" --description "Aliases, completions, common/"
gh label create "area:bootstrap" --color "1d76db" --description "bootstrap.sh and installers"
gh label create "area:ansible"   --color "1d76db" --description "playbooks/ and symlink management"
gh label create "area:docs"      --color "1d76db" --description "CLAUDE.md/AGENTS.md/README"

# platform:
gh label create "platform:linux" --color "fef2c0" --description "Linux-specific"
gh label create "platform:macos" --color "fef2c0" --description "macOS-specific"
gh label create "platform:wsl"   --color "fef2c0" --description "WSL-specific"

# priority:
gh label create "priority:critical" --color "b60205" --description "Must fix immediately"
gh label create "priority:high"     --color "e11d48" --description "Fix in current milestone"
gh label create "priority:medium"   --color "f97316" --description "Fix in next milestone"
gh label create "priority:low"      --color "fef08a" --description "Nice to have"

# status:
gh label create "status:needs-triage" --color "ededed" --description "Awaiting review"
gh label create "status:in-progress"  --color "0e8a16" --description "Actively being worked on"
gh label create "status:blocked"      --color "d93f0b" --description "Blocked by external factor"
gh label create "status:needs-info"   --color "fbca04" --description "Waiting for more information"

# standalone:
gh label create "good first issue" --color "7057ff" --description "Suitable for newcomers"
gh label create "help wanted"      --color "008672" --description "Extra attention needed"
gh label create "regression"       --color "b60205" --description "Broke something that worked"
```

> **Note:** `duplicate` and `wontfix` exist by default on new GitHub repos — no need to re-create them.
