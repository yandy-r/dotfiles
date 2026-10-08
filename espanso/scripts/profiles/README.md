# agent-prompt profiles

One `<name>.conf` per project, read by `../agent-prompt` (never executed).
Add a profile, then add its name to the `profile` choices in
`../../match/prompts/agent-e2e.yml`.

| Key                | Meaning                                           |
| ------------------ | ------------------------------------------------- |
| `NAME`             | Display name                                      |
| `WORKSPACE`        | Linear workspace (required)                       |
| `ISSUE_PREFIX`     | Issue key for `Closes <PREFIX>-N` (required)      |
| `RELEASE_LABEL`    | Release label; empty = RELEASING.md tracker rules |
| `PROJECT`          | Pool for "any of the open X projects"             |
| `BUGS_PROJECT`     | Linear project for `--task bugs`                  |
| `FEATURES_PROJECT` | Linear project for `--task features`              |
| `CODERABBIT`       | `yes` to include the CodeRabbit review rules      |
| `SECRETS`          | `op`, `dotenvx`, or `none`                        |
| `SECRETS_FILES`    | dotenvx files to look for (`.env.encrypted.*`)    |
