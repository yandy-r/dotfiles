# Renovate (self-hosted, Forgejo origin)

Automated dependency updates for this repository run as **self-hosted Renovate**
against the Forgejo origin. The scaffold is **inert by default** and does nothing
until the maintainer explicitly activates it (see [Activation](#activation)).

- Config: [`renovate.json`](../renovate.json)
- Workflow: [`.forgejo/workflows/renovate.yml`](../.forgejo/workflows/renovate.yml)

## Why self-hosted on Forgejo

This was resolved by spike #58. The short version:

- **Renovate supports Forgejo first-class** as a self-hosted bot
  (`RENOVATE_PLATFORM=forgejo`). This is the supported path.
- **The hosted Mend app is out.** It has no Forgejo support, and it cannot reach
  the tailnet-only instance endpoint.
- **The GitHub-mirror route is a dead end.** `origin` -> `github` is a one-way
  force-push mirror, so pull requests opened on the GitHub mirror cannot flow
  back to the origin, and the mirror is force-overwritten on every push.

So the only working path is self-hosted Renovate running as a Forgejo Action on
the instance, authenticated with a dedicated bot PAT.

## What Renovate manages

Renovate is restricted (via `enabledManagers`) to exactly three managers:

- **`gomod`** — `go.mod` / `go.sum`.
- **`npm`** — `package.json` (the dev tooling plus the utility-script runtime
  dependencies). Non-major dev-dependency bumps are grouped into one pull
  request to keep noise down.
- **`github-actions`** — action pins in both `.github/workflows/` and
  `.forgejo/workflows/`. Renovate's `github-actions` manager already matches the
  `.github/`, `.gitea/`, and `.forgejo/` workflow directories by default, so no
  custom `managerFilePatterns` is needed for the `.forgejo/` path.

### What Renovate deliberately does NOT manage

- **`manifest/dotf.lock`** — this lockfile is owned by `dotf lock`. There is no
  Renovate custom manager for it, and there must never be one. Refreshing it is
  the responsibility of the `dotf` CLI, not the dependency bot.

## Commit convention

The repository enforces Conventional Commits through commitlint
(`commitlint.config.cjs`). Renovate is configured to stay compatible:

- `semanticCommits` is `enabled`.
- `semanticCommitType` is forced to `chore` for every update (a catch-all
  `packageRules` entry overrides the `fix`-for-runtime-deps behaviour that
  `config:recommended` would otherwise apply). `chore` is in the commitlint
  `type-enum`, and dependency churn stays out of release notes.
- `semanticCommitScope` is `deps`, so commits read `chore(deps): ...`.

## Activation

The scaffold ships inert. To turn it on, the maintainer performs the following
steps. Nothing runs and no pull request is created until they are complete.

1. **Create a bot account.** In Forgejo, create a `renovate-bot` user and set a
   full name and an email address on the account. Renovate cannot author commits
   without both.
2. **Generate a PAT.** As `renovate-bot`, create a Personal Access Token with the
   `repo`, `user`, `issue`, and `organization` scopes (add `read:packages` only
   if package updates are ever needed).
3. **Add the Actions secrets.** In this repository's Forgejo settings, add:
   - `RENOVATE_TOKEN` — the PAT from step 2 (**required**).
   - `RENOVATE_GIT_AUTHOR` — optional override, for example
     `renovate-bot <renovate-bot@users.noreply.INSTANCE>`. If omitted, Renovate
     derives the author from the bot account configured in step 1.
4. **Run it.** Trigger `.forgejo/workflows/renovate.yml` manually via
   `workflow_dispatch` for the first run. Once satisfied, uncomment the
   `schedule` block in that workflow to enable low-noise weekly runs.

The endpoint and repository are templated from the workflow context
(`RENOVATE_ENDPOINT: ${{ github.server_url }}/api/v1/`,
`RENOVATE_REPOSITORIES: ${{ github.repository }}`), so the instance hostname is
never committed — the same pattern used by
[`.forgejo/workflows/release.yml`](../.forgejo/workflows/release.yml).

## Safety model

- The workflow triggers on `workflow_dispatch` (manual) only; the `schedule`
  trigger is commented out.
- With no `RENOVATE_TOKEN` secret set, even a manual run is a no-op — Renovate
  exits without a token and opens nothing.
- `RENOVATE_AUTODISCOVER` is `false`, so Renovate only ever touches the single
  repository named by `RENOVATE_REPOSITORIES`.
- `renovate.json` also carries a conservative `schedule` and low
  `prConcurrentLimit` / `prHourlyLimit` so that, once active, the bot stays
  low-noise.
