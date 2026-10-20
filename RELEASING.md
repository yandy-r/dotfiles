# Branching and releases

These rules decide where every change to dotfiles goes and how versions ship.
They apply to humans and agents alike, on every task, without being restated in prompts.

The core idea: **a change's target release is decided before work starts, and the target
decides the base branch.** A release is a tag on a branch — never a set of commits picked
from somewhere else after the fact.

## Current state

The block below is read by tools; the table is generated from it. Change both with
`release-state-update.sh` (or `/release-model`), never by hand.

<!-- ycc-release-state
model: trunk-only
trunk: main
support: latest-minor
backport_label: backport:{X.Y}
tracker: none
-->

<!-- ycc-release-state:table:begin -->

Model: **trunk-only**. Support window: latest-minor.

| Role  | Branch | Notes                        |
| ----- | ------ | ---------------------------- |
| Trunk | `main` | Every release is tagged here |

<!-- ycc-release-state:table:end -->

## Rules

1. **Know the target before you branch.** Every issue carries a target release (see
   [Planning](#planning)). No target: a bug in a shipped version is a patch; everything else
   goes to the next release.
2. **Branch off `main` and PR back into it.** Topic branches are short-lived (days, not
   weeks) and named `<type>/<issue-id>-<slug>`. If one falls behind, rebase it.
3. **Never "sync" one long-lived branch into another.** No `sync main into …` PRs.
4. **Every release comes from `main`.** Fixes ship in the next release; there are no
   maintenance branches and no backports. When an older version needs patches, upgrade the
   model with `/release-model --upgrade` instead of branching ad hoc.
5. **No new long-lived branches.** Large work lands on `main` in small PRs. See
   [Unfinished work](#unfinished-work).
6. **`main` is always releasable.** CI green, and nothing half-built reachable by users.
7. **Only maintainers cut releases**, following [Releasing](#releasing). Agents never tag.

## Where does my change go?

Every change targets the next release and goes into `main`. Bugs and security fixes
may prompt an earlier release; they never create a branch of their own.

## Unfinished work

Big features merge incrementally instead of living on a side branch. Anything a user could
reach before it is finished stays hidden: behind a setting or environment variable that
defaults to off, or simply not wired into navigation or routes until the last PR. When a
change cannot be hidden (a rename, a data-directory move), prepare everything behind the
scenes first and make the switch in one final PR shortly before the release.

List the project's feature switches here as they are added, with their default and the
release that flips them.

## Releasing

Tags follow `vX.Y.Z`. Version files: none (tags only; `package.json` holds tooling metadata,
not the release version). Changelog: `CHANGELOG.md`. `.github/workflows/release.yml` runs GoReleaser on the tag and publishes the `dotf` binaries, using the curated notes at `docs/releases/<tag>.md` as the release body (CI fails if that file is missing).

### Every release — from `main`

1. On `main`, commit `chore(release): vX.Y.Z` with the changelog entry and the curated
   release notes at `docs/releases/vX.Y.Z.md`.
2. Push an annotated tag `vX.Y.Z`.
3. Publish the release notes.

`/releaser` automates these steps and checks you are on the right branch.

## Planning

Targets are decided in the PR description: say which release a change is for.
