# dotfiles

Cross-platform dotfiles and development environment configuration (Bash, Zsh, Neovim, terminal emulators, shell tooling) for macOS and Linux.

Full agent rules live in [`CLAUDE.md`](../../CLAUDE.md). Key points surfaced for AI tooling:

## Rules

- Follow Conventional Commits for every commit: `feat|fix|docs|refactor|perf|test|build|ci|chore(scope): …`.
- Never commit `.env` / `.envrc` / secrets / tokens.
- Use `.github/ISSUE_TEMPLATE/*.yml` when present; link `Closes #…` in every PR.
- Keep shared shell logic in `common/`; use `common/macos/` or `common/linux/` for OS-specific code.
- Default to a git worktree for any non-trivial task (multi-step features, refactors, multi-file changes). Prefer `~/.claude-worktrees/<repo>-<branch>/` as the parent — never a path inside the current repo. See `CLAUDE.md` → Git Worktrees.
- Shell: `shellcheck`-clean; quote expansions; use `[[ ]]` and `$(…)`; guard optional tools with `command -v`.
- JavaScript/TypeScript (secondary): strict mode; `PascalCase` components; `camelCase` hooks/functions; prefer `unknown` over `any`.
- Python (secondary): PEP 8; type hints on all public API signatures; `ruff` + `mypy --strict`.

## Verification

Run `npm test`, `npm run lint`, `bash -l -c 'true'`, and `zsh -l -c 'true'` before marking work complete.
Symlink changes: validate with `dotf link --dry-run`.
