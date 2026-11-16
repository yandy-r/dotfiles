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
match/prompts/project-complete.yml :projdone
match/prompts/env-setup.yml    :envsetup
scripts/agent-prompt           renders the end-to-end workflow prompt
scripts/check-prompt-length    checks every prompt fits the 4000-char limit
scripts/profiles/<name>.conf   optional pinned Linear names for agent-prompt
match/local/                   machine-private snippets (git-ignored)
match/packages/                `espanso install` packages (git-ignored)
```

## Agent prompts

`:e2e` opens a form (profile, task, skill hints, target) and pastes the
end-to-end Linear-to-merge workflow prompt. The text comes from
`scripts/agent-prompt`, so one template covers every repo.

`:projdone` is the project-sized sibling: give it a Linear project name and the
agent loops the same ship workflow over every remaining issue (dependency
order, re-reading the project after each merge) until the project is done. It
has no profiles; it uses the `itcn` workspace and the team comes from the repo.
A blocked issue is marked and skipped rather than ending the run, follow-ups
the project needs join the
loop, and the project is marked Completed only when every issue is Done or
Canceled with a reason.

**Profile** says where the Linear names come from:

- `auto` (default) works in any repo with nothing to maintain: the agent uses
  the `itcn` workspace and finds the team (`itcn` or `yandy-r`) and projects
  from `RELEASING.md`/`AGENTS.md` or the repo name, and reports which ones it
  used. Secrets and Codex review rules are phrased conditionally ("if present").
- A named profile (`scripts/profiles/<name>.conf`) pins those names for repos
  where they can't be inferred (shared teams, pooled projects). Profile
  values are hints: if a project or label was renamed, the agent uses the
  closest match and reports it instead of failing. See
  [scripts/profiles/README.md](scripts/profiles/README.md).

**Task** picks the kind of work; **Target** (optional) narrows it and always
wins over the profile:

| Target                     | Source of work                                                   |
| -------------------------- | ---------------------------------------------------------------- |
| empty                      | the profile's bugs/features project or pool, else the repo's own |
| a Linear project name      | that project, ranked for the task (`bugs`, `features`, `any`)    |
| issue IDs (`YAN-1, YAN-2`) | exactly those issues, resuming any work already in flight        |

The prompt also makes the agent treat each issue's acceptance criteria as the
spec (writing them first if missing), start bug fixes with a failing
regression test, and stop and report when blocked rather than guess.

The renderer also works outside espanso:

```bash
~/.config/espanso/scripts/agent-prompt --task bugs | wl-copy
~/.config/espanso/scripts/agent-prompt --profile tokenhop --target "YAN-723, YAN-724"
```

## Prompt length limit

Agent CLIs cap pasted goals; Claude Code's `/goal` accepts at most 4000
characters. Every prompt here stays within a **3000-character budget**, leaving
at least 1000 characters for context you append before sending.
`agent-prompt` refuses to render anything over 4000, and
`npm run test:espanso` (`scripts/check-prompt-length`) renders every profile
(plus `auto`), task, target, and skills combination, measures each
`match/prompts/*.yml` body, and checks that the `:e2e` profile choices match
`scripts/profiles/` (it fails over the 3000 budget). Run it after editing a
prompt or profile.

Prompts use a terse "caveman-lite" style: imperative fragments, no articles or
filler, `→` for "becomes/goes to", `X? do Y` for conditionals, `w/` for "with",
one rule per bullet, and a `## Never` list of bare forbidden actions. Skill
commands, paths, and flags stay verbatim.

## Extending

- **New snippets:** add a `match/<topic>.yml` file. Every `*.yml` under `match/`
  is loaded automatically, including subdirectories.
- **New project for `:e2e`:** usually nothing; `auto` covers it. Add
  `scripts/profiles/<name>.conf` only when the agent can't infer the Linear
  names from the repo, then add the name to the `profile` choices in
  `match/prompts/agent-e2e.yml` (`npm run test:espanso` catches a mismatch).
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
| `:projdone`  | `/goal` to ship every remaining issue in a Linear project |
| `:envsetup`  | `/env-setup` for Cursor cloud agents (not `/goal`)        |
| `:search`    | fuzzy search over all snippets (also `ALT+SPACE`)         |
