# Herdr Provisioning and Tmux-Compatible Configuration Design

**Status:** Approved design for implementation

**Issue:** [#37](https://github.com/yandy-r/dotfiles/issues/37)

**Date:** 2026-08-29

## Purpose

Provision [Herdr](https://github.com/herdrdev/herdr) with the existing `dotf`
pipeline on every supported macOS, Linux, and WSL target, and ship a portable
configuration that preserves the muscle memory of `common/tmux.conf` wherever
Herdr 0.8.2 has an equivalent action.

## Scope

Supported installation targets are:

| Release OS           | Architecture  | Upstream asset        |
| -------------------- | ------------- | --------------------- |
| Linux, including WSL | x86_64        | `herdr-linux-x86_64`  |
| Linux, including WSL | ARM64         | `herdr-linux-aarch64` |
| macOS                | Intel x86_64  | `herdr-macos-x86_64`  |
| macOS                | Apple Silicon | `herdr-macos-aarch64` |

The repository's supported package-manager hosts remain apt, dnf, Homebrew,
pacman, and zypper. Herdr uses a release backend rather than those managers, so
the same artifact path also works on generic Linux and WSL when the architecture
matches an upstream asset.

Native Windows provisioning is out of scope. Although upstream publishes a
Windows binary, `dotf` does not currently have a supported Windows provisioning
contract or package backend.

## Installation Design

Add `herdr` and its macOS sibling `herdr-macos` to the `dev` intent beside
`tmux`. Define two OS-gated `github-release` entries pinned to upstream release
`v0.8.2`:

- Repository: `herdrdev/herdr`
- Installed binary: `herdr`
- Linux entry: `name = "herdr"`, `os = "linux"`, and the two Linux checksums
- macOS entry: `name = "herdr-macos"`, `os = "macos"`, and the two macOS checksums
- Asset: auto-detected from the gated release OS and architecture tokens
- Upgrade policy: explicit tag and checksum changes in reviewed pull requests

The checksums are the immutable asset digests published on upstream's official
[`v0.8.2` GitHub release](https://github.com/herdrdev/herdr/releases/tag/v0.8.2).
The backend must fail closed when it cannot select an asset, cannot find the
target's checksum, or receives content whose SHA-256 does not match the manifest.

This uses the repository's native backend ladder and does not add a custom
installer script or change the custom-script ratchet. Two entries are necessary
because the existing manifest gate accepts one OS token per package: leaving one
entry ungated would evaluate Herdr on native Windows, while adding a new manifest
gate merely for this tool would expand the schema unnecessarily.

## Configuration Ownership

Create `herdr/config.toml` and link that single file to
`~/.config/herdr/config.toml` on every in-scope operating system.

Do not link the entire `~/.config/herdr` directory. Herdr writes rotating logs
such as `herdr.log`, `herdr-client.log`, and `herdr-server.log` beside the config;
linking the directory would place runtime output inside the repository.

The link is part of the native `internal/symlink.DefaultTable` under a small
`FacetUnix` gate that applies on macOS and every canonical Linux family,
including generic Linux and WSL, but not Windows. It is backup-eligible and is
mirrored under the existing macOS/Linux groups in the historical rollback
playbook. Existing destination handling remains unchanged: dry-run previews the
action, and `--backup` preserves a real pre-existing file before replacement.

## User Experience

The shipped config:

- sets `onboarding = false` so a provisioned machine does not rewrite the file
  through first-run setup;
- uses Herdr's built-in Catppuccin theme, matching the tmux theme family;
- places the tab bar at the bottom, matching tmux's status-line position;
- preserves Herdr's prefix-first model so shell and editor input is not stolen;
- keeps `Ctrl+B` as the prefix used by `common/tmux.conf`.

## Keybinding Contract

| Behavior                    | Current tmux binding              | Herdr binding                            |
| --------------------------- | --------------------------------- | ---------------------------------------- |
| Prefix                      | `Ctrl+B`                          | `ctrl+b`                                 |
| Goto picker/window          | `g`                               | `prefix+g`                               |
| New window/tab              | `c`                               | `prefix+c`                               |
| Next/previous window or tab | `n` / `p`                         | `prefix+n` / `prefix+p`                  |
| Select window/tab 1-9       | tmux defaults                     | `prefix+1..9`                            |
| Focus pane                  | `h/j/k/l`                         | `prefix+h/j/k/l`                         |
| Side-by-side split          | `v` or <code>&#124;</code>        | `prefix+v` or <code>prefix+&#124;</code> |
| Stacked split               | `s` or `-`                        | `prefix+s` or `prefix+minus`             |
| Close pane                  | `x`                               | `prefix+x`                               |
| Close window/tab            | `X`                               | `prefix+shift+x`                         |
| Last pane                   | `o`                               | `prefix+o`                               |
| Zoom pane                   | tmux built-in `z`                 | `prefix+z`                               |
| Copy mode                   | `[`                               | `prefix+[`                               |
| Detach                      | tmux built-in `d`                 | `prefix+d` or Herdr alias `prefix+q`     |
| Reload config               | `R`                               | `prefix+shift+r`                         |
| Toggle Herdr sidebar        | no tmux equivalent                | `prefix+shift+b`                         |
| Open notification target    | no tmux equivalent                | `prefix+shift+o`                         |
| Open Herdr settings         | displaced by tmux-style `s` split | `prefix+shift+s`                         |

Herdr's default sidebar binding moves away from lowercase `b`, because the tmux
configuration uses lowercase `b` for synchronized panes. Its notification action
moves away from lowercase `o` so `o` can retain the tmux last-pane behavior.
Herdr's Settings action moves away from lowercase `s` so `s` can retain the tmux
stacked-split behavior.

The config does not add prefix-free `Ctrl+H/J/K/L` bindings. Upstream warns that
those chords intercept shell and editor input, while the existing tmux setup can
delegate them based on the active pane process.

## Intentional Differences

Herdr 0.8.2 does not expose direct equivalents for these tmux behaviors, so the
design does not invent misleading bindings:

- synchronized/broadcast panes (`b`);
- next-layout cycling (`Enter`);
- tmux server/session killing (`Q`);
- TPM plugins, resurrect, or continuum configuration.

Herdr provides its own persistent server, session restore, workspace model, and
agent integration. Those native behaviors remain at their documented defaults.

## Data Flow

1. `dotf install --intent dev` selects exactly one OS-gated Herdr package:
   `herdr` on Linux/WSL or `herdr-macos` on macOS.
2. The GitHub Release backend derives the upstream `linux` or `darwin` release
   OS and the accepted x86_64 or ARM64 asset aliases from the detected platform.
3. It selects the matching `v0.8.2` asset, verifies the target-specific checksum,
   and installs the executable through the existing user-binary path.
4. `dotf link --backup` preserves an existing config-file's bytes before it
   replaces the file with the single repository config-file link.
5. Herdr reads the linked TOML file while writing logs to the surrounding real
   `~/.config/herdr` directory.

Package installation and config linking remain independently reportable actions;
a package failure participates in the existing non-fatal apply summary rather
than weakening checksum verification or silently skipping the config.

## Files and Responsibilities

- `manifest/packages.toml`: two OS-gated `dev` intent members and pinned release metadata.
- `herdr/config.toml`: portable presentation and keybinding configuration.
- `internal/symlink/table.go`: `FacetUnix` and the active config-file link contract.
- `playbooks/symlinks.yml`: historical rollback parent-directory creation.
- `playbooks/vars/symlinks.yml`: historical rollback parity.
- `internal/manifest/manifest_test.go`: supported-target release/checksum contract.
- `internal/symlink/table_test.go`: table count, backup eligibility, and parity.
- `internal/symlink/symlink_test.go`: per-OS plan expectations.
- `README.md`: user-facing terminal-tool and managed-config summary.

No backend, platform, CLI, or manifest-schema change is required. The symlink
table gains one composite facet so the file's scope matches the approved
Linux/macOS boundary without changing native Windows behavior.

## Critical Verification

Only acceptance-critical test coverage is added or changed:

1. A manifest test proves both OS-gated Herdr entries are in the `dev` intent,
   are pinned to the expected upstream release, install the `herdr` binary, and
   together have non-empty SHA-256 pins for `linux/x86_64`, `linux/arm64`,
   `darwin/x86_64`, and `darwin/arm64`. Applicability checks prove exactly one
   entry applies on every supported target and neither applies on Windows.
2. Existing symlink table/golden tests prove the Herdr config is included,
   backup-eligible, and planned for macOS and supported Linux families.
3. A focused native symlink integration test uses `DefaultTable` with a
   temporary home, seeds a real Herdr config, then proves `--backup` preserves
   the original bytes before the destination becomes the intended repository symlink.
4. The pinned real Herdr binary loads the committed config without a startup
   warning on the available Linux x86_64 host.
5. Repository gates run: `npm run lint`, the explicit product-doc Markdown
   commands below, `npm run go:build`, `go build ./...`, `go vet ./...`, and
   `go test -race ./...`:

   ```bash
   feature_base="$(git merge-base origin/main HEAD)"
   git -c core.whitespace=blank-at-eol,blank-at-eof,space-before-tab diff --check "${feature_base}..HEAD"
   herdr_product_markdown_files=(
     README.md
     docs/superpowers/plans/2026-08-29-herdr-provisioning.md
     docs/superpowers/specs/2026-08-29-herdr-provisioning-design.md
   )
   npx --no -- markdownlint-cli2 "${herdr_product_markdown_files[@]}"
   npx --no -- prettier --check --ignore-path .prettierignore --config .prettierrc \
     docs/superpowers/plans/2026-08-29-herdr-provisioning.md \
     docs/superpowers/specs/2026-08-29-herdr-provisioning-design.md
   ```

   Record the fresh, clean diff-hygiene result over `${feature_base}..HEAD`.
   Full-repository `npm run lint:md` remains a known non-green baseline of 444
   pre-existing errors and is recorded honestly rather than treated as passed.
   The exact Prettier check covers the newly authored plan and spec. The
   unchanged README's pre-existing Prettier result is recorded separately and
   is not an all-green feature gate, without folding unrelated README table
   reformatting into this feature. Committed review artifacts are validated by
   the review workflow rather than this product-doc gate.

6. `dotf link --dry-run` visibly includes `.config/herdr/config.toml` when run
   with `DOTFILES_ROOT` pointed at the feature worktree.

The implementation does not add tests for generic backend behavior already
covered by `internal/backend`; it adds only the Herdr target-matrix invariant and
updates existing symlink contracts.

## Acceptance Criteria

- An applicability-aware `dotf apply --headless --dry-run --intent dev` preview
  includes exactly one Herdr provider on all four supported release targets
  without relying on a distro-specific package name; the guided plan may report
  the raw cross-platform provider union before applicability filtering.
- Every supported target has a committed checksum and installation fails closed
  outside that matrix.
- The managed config file is linked on macOS, Linux, and WSL without capturing
  Herdr runtime logs.
- The active keymap matches the table above and Herdr accepts the file.
- Native Windows remains unchanged and undocumented as supported by `dotf`.
- Required local checks and pull-request CI are green before squash merge.
