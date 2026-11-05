# `dotf rc` — Claude Code Remote Control servers per repo

`dotf rc` keeps one Claude Code Remote Control server (`claude remote-control`) running for each
repo you list, so you can drive any of them from claude.ai/code or the Claude mobile app.

## Why one server per repo

A Remote Control server is tied to the directory it starts in: every session it spawns works there.
A single server at a projects root (say `~/Projects/github.com`) would lose each repo's
`.claude/settings.json`, `.mcp.json`, hooks and permission rules, and `--spawn worktree` would not
work because the root is not a git repo. `dotf rc` runs one server per repo instead, each in an `rc`
tab of that repo's [Herdr](../../herdr/) workspace, so they are also visible locally.

## Quick start

```bash
dotf rc add ~/.config/dotfiles ~/Projects/github.com/me/app   # paths can be added at any time
dotf rc up --dry-run                                          # preview: the exact command per repo
dotf rc up                                                    # start them now
dotf rc enable --now                                          # optional: start them at every login
```

Before the first `up`, run `claude` once in each repo and accept the folder trust prompt, and make
sure you are logged in (`/login`) with a Claude subscription. Otherwise the server stops at Claude's
trust prompt inside its `rc` tab until you answer it there.

## The config file

`${XDG_CONFIG_HOME:-~/.config}/dotf/rc.toml` (`--config` overrides). It is user-local rather than in
the repo because the paths are machine-specific. `dotf rc add` creates it with explicit defaults:

```toml
permission_mode = 'auto'   # --permission-mode for spawned sessions
allow_bypass = true        # adds --allow-dangerously-skip-permissions: bypass stays reachable
spawn = 'worktree'         # --spawn: same-dir, worktree, or session
# claude = 'claude'        # the claude executable (a bare name, an absolute path, or ~/...)

[env]                      # non-secret values only: the line is typed into a shell (history)
DO_NOT_TRACK = '0'         # the dotfiles export DO_NOT_TRACK=1; Remote Control needs it off

[[repo]]
path = '~/.config/dotfiles'

[[repo]]
path = '~/Projects/github.com/me/api'
name = 'api'                     # session and Herdr workspace name (default: directory name)
permission_mode = 'acceptEdits'  # per-repo override
spawn = 'same-dir'               # per-repo override
```

Names must be unique, because they label the session and the Herdr workspace and select repos with
`--only`. `add` stores paths under your home directory as `~/...`. `add` and `remove` rewrite the
file, so hand edits to values are kept but comments are not. Unknown keys are rejected.

## Verbs

| Verb                   | Effect                                                                                              |
| ---------------------- | --------------------------------------------------------------------------------------------------- |
| `list`                 | The configured repos and their effective settings (`--json` includes each command)                  |
| `add <path>…`          | Add repos; `--name`, `--permission-mode`, `--spawn` set per-repo overrides (one path with `--name`) |
| `remove <name\|path>…` | Remove repos                                                                                        |
| `up`                   | Start each repo's server (`--only`, `--yes`, `--dry-run`, `--json`)                                 |
| `status`               | Per repo: does the directory exist, is a server running (with PIDs); plus the service state         |
| `enable`               | Install and enable the `dotf-rc.service` systemd user unit; `--now` also starts it                  |
| `disable`              | Disable and remove the unit; `--now` also stops it                                                  |

## What `up` does

1. Starts the Herdr server if none is running, and waits for it to answer.
2. For each repo, finds its Herdr workspace (labelled with the repo name, or holding a pane in the
   repo directory) or creates one, without stealing focus.
3. Looks for a Remote Control server already running in the repo: a process named `claude` whose
   working directory is the repo (read from `/proc` on Linux) and whose arguments are
   `remote-control` or `rc`. Only `--allow-dangerously-skip-permissions`,
   `--dangerously-skip-permissions`, `--verbose` and `--debug` may come before that argument. Any
   other flag before it means no match, so an interactive session started with `--name rc` or
   `--resume rc` is never stopped. Repo paths are resolved through symlinks first, because `/proc`
   reports resolved paths.
4. If one is running, asks before stopping it and starting a new one. Stopping sends SIGTERM, then
   SIGKILL after 5 seconds. Before each signal it checks again that the PID is still that server,
   so a process that exited, and whose PID was reused, is never signalled. It then replaces the
   `rc` tab with a fresh one. `--yes` restarts without asking. Without a terminal and without
   `--yes` (for example under the service), the running server is kept.
5. Otherwise it reuses the idle `rc` tab or creates one, waits for its shell to finish starting,
   types the command, and checks that a server process appears within 10 seconds and is still
   running 3 seconds later. If not, that repo counts as failed and `up` exits 1. Typical causes are
   being logged out, an untrusted folder, or `spawn = 'worktree'` in a folder that isn't a git
   repo.

Ctrl+C is intercepted only while a restart prompt is open (exit 130). During every other phase it
interrupts `up` as usual.

The command is typed through `command`, for example:

```text
DO_NOT_TRACK=0 command claude --allow-dangerously-skip-permissions remote-control --name app --permission-mode auto --spawn worktree
```

`command` skips shell aliases. The dotfiles alias `claude` to add
`--dangerously-skip-permissions`, which would otherwise force every session into bypass mode.

`--dry-run` prints the plan and every command without touching Herdr or any process.

## What `down` does

`down` is the inverse of `up`. For each configured repo (or `--only`), it stops every Remote Control
server running there, using the same `/proc` match and per-signal PID re-check as a restart
(SIGTERM, then SIGKILL after 5 seconds). It then closes the repo's `rc` tab, which also ends a
server `/proc` could not see. Only an `rc` tab whose pane is in the repo folder counts, so a tab
you named `rc` elsewhere is never closed (`up`'s restart uses the same rule). It lists what it
will stop and asks once; `--yes` skips the question, and without a terminal and without `--yes` it
refuses (exit 1). `--dry-run` prints the same list without stopping anything.

`down` never starts Herdr. If no Herdr server is running it only stops processes. It never touches
a server or tab outside the configured repos.

## Starting at login vs. by hand

- **By hand:** run `dotf rc up` after you boot. Run it again whenever you add a path; repos that
  are already running are left alone unless you confirm a restart.
- **As a service:** `dotf rc enable` writes `~/.config/systemd/user/dotf-rc.service`, a oneshot
  that runs `dotf rc up` at login. Stopping the unit runs `dotf rc down --yes`, so
  `systemctl --user stop dotf-rc.service` and `dotf rc disable --now` bring the servers down.
  `KillMode=process` means stopping the unit never kills Herdr itself; only `down` stops the
  servers. `up` exiting 1 because some repos failed still leaves the unit active
  (`SuccessExitStatus=1`), so its stop step runs for the repos that did start; per-repo failures
  are in `journalctl --user -u dotf-rc` and `dotf rc status`. A unit written before `down` existed
  has no stop step, so rerun `dotf rc enable` to update it. The unit records the `dotf` path, the config path, and a `PATH` that includes the
  directories holding `dotf`, `herdr` and `claude`, so also rerun `dotf rc enable` if you move
  them. To start servers at boot without logging in, also run `loginctl enable-linger "$USER"`.

`enable` and `disable` refuse to touch a `dotf-rc.service` that `dotf` did not write unless you pass
`--force`.

## Platform support

`up`, `list`, `add`, `remove` and `status` work wherever Herdr runs. The running-server scan needs
Linux's `/proc`. On other hosts `status` reports `running=unknown`, and `up` treats an existing `rc`
tab as possibly running, so it asks before restarting it. `enable`/`disable` need Linux with systemd.
On macOS, run `dotf rc up` by hand.

## Exit codes

`0` success; `1` a repo failed or a refusal; `2` usage error; `130` Ctrl+C at a restart (`up`) or
stop (`down`) prompt.
