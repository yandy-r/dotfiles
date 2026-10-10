# espanso

Text-expander config, linked to `~/.config/espanso` by `dotf link` (espanso reads
that path on Linux and macOS). Install espanso with `bin/install-espanso.sh`.

```text
config/default.yml             global options (toggle key, search, clipboard, layout)
match/base.yml                 everyday snippets (:date, :time, :now)
match/dev.yml                  developer snippets (:bash, :cc)
match/prompts/agent-e2e.yml    :e2e form -> scripts/agent-prompt
match/prompts/release.yml      :release
match/prompts/op-secrets.yml   :opsecrets
match/prompts/project-init.yml :projinit
match/prompts/linear-issue.yml :linissue
scripts/agent-prompt           renders the end-to-end workflow prompt
scripts/check-prompt-length    checks every prompt fits the 4000-char limit
scripts/profiles/<name>.conf   per-project values for agent-prompt
match/local/                   machine-private snippets (git-ignored)
match/packages/                `espanso install` packages (git-ignored)
```

## Agent prompts

`:e2e` opens a form (profile, task, skill hints, target) and pastes the
end-to-end Linear-to-merge workflow prompt. The text comes from
`scripts/agent-prompt`, so one template covers every project:

| Task       | Source of work                                         |
| ---------- | ------------------------------------------------------ |
| `bugs`     | the bugs project, or the pool with a bug focus         |
| `features` | the features project, or the pool with a feature focus |
| `any`      | the pool, or both the bugs and features projects       |
| `project`  | the Linear project typed in Target                     |
| `issues`   | the issue IDs typed in Target (`YAN-723 and YAN-724`)  |

Per-project values (workspace, issue prefix, release label, CodeRabbit,
secrets) live in `scripts/profiles/*.conf`; see
[scripts/profiles/README.md](scripts/profiles/README.md). The renderer also
works outside espanso:

```bash
~/.config/espanso/scripts/agent-prompt --profile tokenhop --task bugs | wl-copy
```

## Prompt length limit

Agent CLIs cap pasted goals; Claude Code's `/goal` accepts at most 4000
characters. Every prompt here stays under that, so it works pasted into any
tool. `agent-prompt` refuses to render anything longer, and
`npm run test:espanso` (`scripts/check-prompt-length`) renders every profile,
task, and skills combination and measures each `match/prompts/*.yml` body. Run
it after editing a prompt; keep new prompts terse (one rule per bullet, no
repeated context).

## Extending

- **New snippets:** add a `match/<topic>.yml` file. Every `*.yml` under `match/`
  is loaded automatically, including subdirectories.
- **New project for `:e2e`:** add `scripts/profiles/<name>.conf` and add the
  name to the `profile` choices in `match/prompts/agent-e2e.yml`.
- **New prompts:** add `match/prompts/<name>.yml`. Use a `form` variable and
  `{{form.<field>}}` for simple fill-ins; for conditional text, render it with
  a script in `scripts/` called from a `shell` variable with
  `depends_on: [form]` (form values arrive as `ESPANSO_FORM_<FIELD>`).
- **Partials:** files starting with `_` are not loaded on their own; pull them in
  with `imports:`.
- **App-specific options:** add `config/<app>.yml` with a `filter_class`,
  `filter_exec`, or `filter_title`. Window filters do not work on Wayland.

espanso reloads on save. Check a change with `espanso log` or `espanso match list`.

## Snippets

| Trigger      | Expands to                                                |
| ------------ | --------------------------------------------------------- |
| `:date`      | `2026-10-07`                                              |
| `:time`      | `14:05`                                                   |
| `:now`       | ISO 8601 timestamp                                        |
| `:shrug`     | `¯\_(ツ)_/¯`                                              |
| `:bash`      | bash shebang + `set -euo pipefail`                        |
| `:cc`        | Conventional Commits title (type, scope, summary)         |
| `:e2e`       | End-to-end agent workflow (profile, task, skills, target) |
| `:release`   | `/goal` to cut the next patch/minor/major release         |
| `:projinit`  | `/goal` to initialize a project or update an existing one |
| `:opsecrets` | `/goal` to back up secrets to a 1Password secure note     |
| `:linissue`  | `/goal` to file an agent-ready Linear issue (no code)     |
| `:search`    | fuzzy search over all snippets (also `ALT+SPACE`)         |
