# Pre-commit Hooks — Recommendation for dotfiles

Pre-commit hooks catch formatting errors, lint violations, and broken tests before they reach CI, shortening the feedback loop for every contributor.

Since this repo is **mixed-language** (shell-first, with JavaScript tooling and Python helper scripts), the recommended stack is `lefthook` + ecosystem-specific linters. Pick one of the configurations below.

---

## Shell (Bash/Zsh) — lefthook (recommended)

Install: `brew install lefthook` (macOS) or follow <https://github.com/evilmartians/lefthook#install>.

`lefthook.yml`:

```yaml
pre-commit:
  commands:
    shellcheck:
      glob: '*.{sh,bash,zsh}'
      run: shellcheck {staged_files}
    shfmt:
      glob: '*.{sh,bash,zsh}'
      run: shfmt -d -i 2 -bn -ci {staged_files}
pre-push:
  commands:
    test:
      run: npm test
```

## JavaScript / Node — husky + lint-staged (optional)

Install: `npm add -D husky lint-staged`.

`.lintstagedrc.json`:

```json
{
  "*.{js,cjs,mjs,ts}": ["npm run lint"],
  "*.md": ["npm run lint"]
}
```

`.husky/pre-push`: `npm test`

## Python — pre-commit.com (optional, for python scripts under `bin/`)

Install: `pip install pre-commit` then `pre-commit install`.

`.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/astral-sh/ruff-pre-commit
    rev: v0.4.0
    hooks:
      - id: ruff
        args: [--fix]
      - id: ruff-format
```

## Commit-message validation (commitlint)

`commitlint.config.cjs` is already in the repo. Wire it into lefthook:

```yaml
commit-msg:
  commands:
    commitlint:
      run: npx commitlint --edit {1}
```

Or into husky: `npx husky add .husky/commit-msg 'npx --no -- commitlint --edit "$1"'`.

---

Install manually — the init workflow deliberately does NOT install hooks automatically. Review the snippet for your ecosystem above, adapt paths and commands to your project layout, then commit the hook config alongside your code. Refer to `CLAUDE.md` for this project's commit message rules and branching conventions.
