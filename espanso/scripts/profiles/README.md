# agent-prompt profiles

Profiles are optional. With the `auto` profile, `../agent-prompt` asks the
agent to use the `itcn` workspace and find the Linear team and projects from
the repo itself (`RELEASING.md`, `AGENTS.md`, or the repo name), so a new repo
needs no profile. Add `<name>.conf` only to pin names the agent can't infer,
then add its name to the `profile` choices in `../../match/prompts/agent-e2e.yml`
(`npm run test:espanso` fails until the two match).

Files are read as `KEY=value` lines, never executed. Every key is optional;
an empty or `auto` value lets the agent decide. Values are hints: when a
Linear project or label is renamed, the agent uses the closest match and
reports it, so a stale profile degrades instead of failing. Keep only what
the repo can't say for itself.

| Key                | Meaning                                                      |
| ------------------ | ------------------------------------------------------------ |
| `NAME`             | Project name; the agent stops if the repo is a different one |
| `WORKSPACE`        | Linear workspace (`itcn`)                                    |
| `TEAM`             | Linear team (`itcn` or `yandy-r`)                            |
| `ISSUE_PREFIX`     | Issue key for `Closes <PREFIX>-N` (default `<TEAM>`)         |
| `RELEASE_LABEL`    | Release label; empty = RELEASING.md tracker rules            |
| `POOL`             | Pool for "any open X project" (all tasks)                    |
| `BUGS_PROJECT`     | Linear project for task `bugs`                               |
| `FEATURES_PROJECT` | Linear project for task `features`                           |
| `CODEX`            | `yes`, `no`, or `auto` (Codex review rules "if present")     |
| `SECRETS`          | `op`, `none`, or `auto` (detect from repo files)             |
| `SECRETS_FILES`    | `op://` reference templates to use (`op/<env>.env`)          |

Lookup order for the work source: Target, then `BUGS_PROJECT`/`FEATURES_PROJECT`
(both for task `any`), then `POOL`, then the repo's own Linear projects.
