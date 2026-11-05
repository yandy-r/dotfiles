# `dotf rc` profiles — design

## Goal

Run Remote Control servers under more than one Claude subscription. Each subscription lives in its
own config dir (`CLAUDE_CONFIG_DIR`), the same dir a shell alias such as `claude-work` would use.
`rc.toml` names those subscriptions as **profiles**, assigns each repo to one, and gives each profile
its own permission defaults and tool permission rules.

Single user (the repo owner); no migration or compatibility concerns beyond "an existing `rc.toml`
keeps working unchanged".

## Verified constraints (claude 2.1.318, checked against running servers)

- A Remote Control server spawns each session as a child `claude --print …` process.
- **Environment is inherited** by spawned sessions (`DO_NOT_TRACK=0` from `[env]` shows up in the
  child). So `CLAUDE_CONFIG_DIR` set on the server line selects the subscription for every session.
- **CLI flags are not forwarded**, except `--permission-mode`, which the server re-issues. Neither
  `--allow-dangerously-skip-permissions` nor a `--settings` placed before `remote-control` reaches
  the child. Tool permission rules therefore cannot be passed on the command line; they must live
  in a settings file the child reads: `$CLAUDE_CONFIG_DIR/settings.json`.
- Observed: sessions started from claude.ai cap at `auto` even when the server passes
  `bypassPermissions`. Out of scope; documented as a limitation.

## Config

```toml
permission_mode = 'auto'       # top level = the implicit default profile "claude"
allow_bypass = true
spawn = 'worktree'
[env]
DO_NOT_TRACK = '0'

[profile.work]
config_dir = '~/.claude-work'  # required; exported as CLAUDE_CONFIG_DIR
permission_mode = 'acceptEdits'  # optional
allow_bypass = false             # optional
spawn = 'same-dir'               # optional
[profile.work.env]               # optional; merged over top-level [env]
FOO = 'bar'
[profile.work.permissions]       # optional; synced into <config_dir>/settings.json
allow = ['Bash(git *)', 'Edit']
deny = ['Bash(rm -rf *)']
ask = []

[[repo]]
path = '~/Projects/work/api'
profile = 'work'               # omit → default profile "claude"
```

### Rules

- The default profile is named `claude`, is implicit, and has no config: it is the top-level keys,
  exactly as today. `[profile.claude]` is rejected (reserved).
- Profile names match the existing name regex (`^[A-Za-z0-9][A-Za-z0-9._-]*$`).
- `repo.profile` must name a defined profile (or `claude`).
- `config_dir` is required, `~`-expanded, and must be absolute after expansion. Two profiles may not
  share a `config_dir`.
- Neither the top-level `[env]` nor a profile's `env` may set `CLAUDE_CONFIG_DIR` (use a profile's
  `config_dir`; the default profile always uses Claude's own default).
- Precedence for `permission_mode` and `spawn`: repo → profile → top level → built-in default.
  `allow_bypass`: profile → top level → true. `env`: top level, overlaid by profile, then
  `CLAUDE_CONFIG_DIR` from `config_dir`.
- Permission rule strings are passed through verbatim (no validation of rule syntax; Claude Code
  validates them). Empty strings are rejected.

## Command line

`Resolved` gains `Profile` and `ConfigDir`. The typed line becomes, for a profiled repo:

```text
CLAUDE_CONFIG_DIR=/home/u/.claude-work DO_NOT_TRACK=0 FOO=bar command claude remote-control --name api --permission-mode acceptEdits --spawn same-dir
```

Env keys stay sorted. Default-profile repos produce exactly today's line.

## Permissions sync

When a profile has a `[profile.X.permissions]` table, `dotf rc up` makes
`<config_dir>/settings.json` hold exactly those `permissions.allow`, `permissions.deny` and
`permissions.ask` arrays, for every profile used by at least one repo `up` acts on (respecting
`--only`). rc.toml is the source of truth for those three arrays only:

- Every other key in `settings.json`, and every other key under `permissions` (`defaultMode`,
  `additionalDirectories`, …), is preserved.
- A missing file or dir is created (dir 0700, file 0600). Invalid JSON is an error for that profile
  (its repos are skipped and `up` exits 1); nothing is written.
- An omitted array in rc.toml (e.g. no `ask`) leaves that array untouched in settings.json; an
  explicit empty array clears it.
- The file is rewritten (atomic temp + rename, mode preserved) only when an array changes. Output
  key order may change on rewrite; content does not.
- Runs before any server starts, so a restarted server's sessions see the new rules.
- `--dry-run` reports what would change without writing.
- Profiles without a permissions table are never touched. The default profile never is.

Output lines (key=value, matching the existing style):

```text
rc.profile=work settings=/home/u/.claude-work/settings.json permissions=updated|unchanged|would-update
```

`--json` gains a `profiles` array with the same fields.

## CLI surface

- `dotf rc add --profile <name> <path>…` sets `repo.profile`; the profile must already exist in
  rc.toml (profiles are hand-edited; no `profile add` verb).
- `list`, `up`, `down`, `status` text lines gain `profile=<name>`; `--json` repo objects gain
  `profile` and `config_dir` (empty for the default).
- Completion/help pick up `--profile` from `setRcFlags`/`rcFlagVerbs` automatically.

## Out of scope

- Same repo served under two profiles (names stay unique per repo; dedupe by path is unchanged).
- Managing shell aliases from rc.toml.
- Per-repo permission rules (repos already have `.claude/settings.json`).
- Lifting the claude.ai `auto` cap.

## Files

- `internal/rc/config.go` — `Profile` type, `Config.Profiles map[string]Profile`, `Repo.Profile`,
  validation, resolution.
- `internal/rc/command.go` — env from resolved profile.
- `internal/rc/settings.go` (new) — `SyncPermissions(configDir string, p Permissions, dryRun bool)
  (changed bool, path string, err error)`.
- `cmd/dotf/rc.go`, `rcup.go`, `rcdown.go` — `--profile`, output fields, sync step in `up`.
- `docs/features/dotf-rc.md` — profiles section; correct the `allow_bypass` claim.

## Testing

- Config: table tests for reserved name, unknown profile, missing/duplicate `config_dir`,
  `CLAUDE_CONFIG_DIR` in profile env, empty rule; resolution precedence; round-trip save/load.
- Command: `ShellLine` for default and profiled repos.
- Settings sync: create-from-missing, preserve unrelated keys, unchanged → no write (mtime),
  omitted vs. empty array, invalid JSON error, dry run writes nothing.
- cmd: `add --profile` with unknown profile fails; `up --dry-run` prints the profile line.
