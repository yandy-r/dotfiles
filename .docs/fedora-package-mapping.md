# Fedora Package Mapping

Research completed: 2026-08-28

Verified against: Fedora `<release>` (aarch64), `<date>` — PENDING WS0 device capture

> **TODO — replace the `Verified against` line above.** The release number and date must be transcribed from the WS0
> device capture on the target M1, specifically its `dnf --version`, `cat /etc/os-release`, and
> `dnf list --available lazygit yazi lutris docker-ce moby-engine` output. Until that capture lands, treat the header
> as unverified against a concrete release.

## Provenance

This table commits to package **names**, not versions. Package names are stable across recent Fedora branches;
versions are not, and the two research datasets behind this document disagreed on them — one reported `fc43` NEVRs,
the other `fc44`. **No version is asserted anywhere in this document.** Where a version-like string does appear it is
an upstream **release tag** pinned in `manifest/packages.toml`, not a Fedora package version.

Every `Manifest line` cell is a line range in `manifest/packages.toml` as of the research date above; entry **names**
are the durable identifier if the ranges drift.

## Legend

The `Fedora path` column uses these values:

- `official` — Fedora's own release/updates repositories
- `copr:<owner>/<project>` — a Fedora COPR project
- `github-release` — an upstream release asset installed through the existing `github-release` backend
- `vendor .repo` — a third-party dnf repository dropped into `/etc/yum.repos.d`
- `toolchain` — a language package manager (cargo, npm, `go install`)
- `n/a` — correctly not-applicable on Fedora

## pkg backend (13 entries)

These are the shared daily-driver `pkg` entries. The `names` table on each entry **is** its applicability
declaration, so a manager whose distro genuinely does not package the tool is omitted rather than invented — see
`lazygit` and `yazi` below, whose Fedora path is a separate `os = "redhat"` sibling entry. `lutris-pkg` is also a
`pkg` entry but is a Fedora-only sibling of an `aur` package, so it is documented in the next table alongside its
parent.

| Package     | Manifest line | dnf name    | Fedora path                                | aarch64 | Verdict                                                                                                                                                                                                      |
| ----------- | ------------- | ----------- | ------------------------------------------ | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| ripgrep     | `:118-120`    | `ripgrep`   | official                                   | yes     | correct                                                                                                                                                                                                      |
| eza         | `:123-125`    | `eza`       | official (updates)                         | yes     | correct                                                                                                                                                                                                      |
| bat         | `:128-130`    | `bat`       | official                                   | yes     | correct — Fedora's binary is `bat`, so no alias shim is needed (Debian's is `batcat`)                                                                                                                        |
| fd          | `:133-135`    | `fd-find`   | official                                   | yes     | correct — Fedora's `fd-find` ships `/usr/bin/fd`, so no Debian-style rename shim (Debian ships `fdfind`)                                                                                                     |
| fzf         | `:138-140`    | `fzf`       | official                                   | yes     | correct                                                                                                                                                                                                      |
| delta       | `:143-145`    | `git-delta` | official                                   | yes     | correct — Fedora **also** ships an unrelated package literally named `delta`, so do **not** shorten this name                                                                                                |
| zoxide      | `:148-150`    | `zoxide`    | official                                   | yes     | correct                                                                                                                                                                                                      |
| jq          | `:153-155`    | `jq`        | official                                   | yes     | correct                                                                                                                                                                                                      |
| gh          | `:158-160`    | `gh`        | official                                   | yes     | correct — Fedora packages it as `gh` (Arch uses `github-cli`); no vendor repo needed                                                                                                                         |
| **lazygit** | `:163-165`    | none        | `github-release` — `jesseduffield/lazygit` | yes     | **was broken**, fixed — no Fedora RPM exists, so the dnf key is omitted and the `os = "redhat"` sibling `lazygit-bin` (`:245-251`) installs the pinned `v0.64.1` asset, verified against its `checksums.txt` |
| tmux        | `:168-170`    | `tmux`      | official                                   | yes     | correct                                                                                                                                                                                                      |
| **yazi**    | `:182-184`    | none        | `github-release` — `sxyazi/yazi`           | yes     | **was broken**, fixed — no Fedora RPM exists; the `os = "redhat"` sibling `yazi-bin` (`:262-268`) installs the pinned `v26.8.15` asset with **inline per-target `checksums`**                                |
| direnv      | `:187-189`    | `direnv`    | official                                   | yes     | correct                                                                                                                                                                                                      |

Two notes on the `github-release` siblings:

- `lazygit-bin` reads its sha256 from the release's own `checksums.txt` sidecar. `copr:atim/lazygit` was rejected: its
  chroots are live but its newest build failed, and a `github-release` is lock-pinnable and independent of a third
  party's build queue.
- `yazi-bin` pins shas **inline**, per target, because upstream publishes no combined sidecar — only per-asset
  `.sha256` files carrying a bare hash with no filename column, which `checksums_asset` cannot read. The upstream
  archive also ships `ya`, yazi's plugin manager; the single-binary `github-release` backend installs only `yazi`.

## Non-pkg backends (23 entries)

| Package                 | Manifest line          | Backend                       | Fedora path                                                                                                               | Verdict                                                                        |
| ----------------------- | ---------------------- | ----------------------------- | ------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| neovim                  | `:173-179`             | custom-script                 | the installer emits `linux-arm64`; Fedora also ships `neovim` (official)                                                  | works unchanged                                                                |
| atuin                   | `:346-351`             | custom-script                 | official `atuin` exists — a future ratchet-**down** candidate                                                             | works unchanged                                                                |
| mise                    | `:355-360`             | custom-script                 | absent from Fedora's repos; `copr:jdxcode/mise` or the upstream RPM repo would be the native paths                        | vendor installer works on aarch64                                              |
| rust                    | `:367-372`             | custom-script                 | rustup supports `aarch64-unknown-linux-gnu`; Fedora also ships `rustup`                                                   | works unchanged                                                                |
| lua                     | `:385-392`             | custom-script                 | the installer already has a dnf branch; `lua`, `lua-devel`, `luarocks` are official                                       | works unchanged                                                                |
| claude                  | `:396-402`             | custom-script                 | vendor installer, arm64 asset                                                                                             | works unchanged                                                                |
| codex                   | `:409-412`             | toolchain (npm)               | needs `nodejs` + `nodejs-npm` (official, versioned as `nodejs22`/`nodejs24`) — not provisioned on any distro              | pre-existing cross-platform hole (non-goal)                                    |
| opencode                | `:415-421`             | custom-script                 | vendor installer, arm64 asset                                                                                             | works unchanged                                                                |
| fabric                  | `:429-435`             | custom-script                 | `go install`; needs Go on PATH, which is not provisioned on any distro                                                    | pre-existing cross-platform hole (non-goal)                                    |
| coderabbit              | `:438-444`             | custom-script                 | vendor installer; upstream `linux-arm64` availability is unverified                                                       | **arm64 risk**; reachable from the opt-in `ai` intent only                     |
| aws-tools               | `:450-455`             | custom-script                 | AWS CLI v2 and SAM publish aarch64 builds; `ecs-cli` is x86_64-only and EOL                                               | opt-in `cloud` intent; the all-present probe can never satisfy                 |
| azure-cli               | `:460-465`             | custom-script                 | official `azure-cli` (noarch) is a ratchet-down candidate; the script's RHEL-9 Microsoft repo path has no aarch64 RPM     | opt-in `cloud` intent                                                          |
| hashicorp-tools         | `:469-474`             | custom-script                 | vendor `.repo` at `rpm.releases.hashicorp.com/fedora/hashicorp.repo`; the script's `yum-config-manager` is absent on dnf5 | opt-in `cloud` intent                                                          |
| container-tools         | `:478-483`             | custom-script                 | no distro branches; the arm64 tokens match. `hadolint` is also official                                                   | works unchanged                                                                |
| turso                   | `:493-498`             | github-release                | upstream publishes `turso-cli_Linux_arm64.tar.gz` plus a `checksums.txt` sidecar                                          | works; a lock miss still verifies via the sidecar                              |
| tea                     | `:502-507`             | custom-script                 | has a `redhat` branch that falls back to a binary download                                                                | works unchanged                                                                |
| lefthook                | `:513-515`             | toolchain (npm)               | the same latent node/npm dependency as `codex`                                                                            | pre-existing cross-platform hole (non-goal)                                    |
| tpm                     | `:523-526`             | git-clone                     | OS-agnostic HTTPS clone; needs `git` (official)                                                                           | works unchanged                                                                |
| tmux-sessionizer        | `:531-536`             | custom-script                 | `cargo install`; needs the `runtimes` intent's rust first                                                                 | pre-existing cross-intent ordering hole (non-goal)                             |
| gnu-utilities           | `:547-553`             | custom-script, `os = "macos"` | n/a                                                                                                                       | correctly not-applicable; no work                                              |
| **lutris / lutris-pkg** | `:272-273`, `:282-285` | aur, plus a `pkg` sibling     | official `lutris` **exists on Fedora**; the `aur` backend gates on Arch, so a `pkg` sibling supplies the Fedora path      | **fixed here** by `lutris-pkg` (`os = "redhat"`, `names = { dnf = "lutris" }`) |
| **docker-ce**           | `:357-362`             | repo (`dnf_repo`)             | `vendor .repo` at `download.docker.com/linux/fedora/docker-ce.repo`; Fedora also ships `moby-engine` and `podman`         | **fixed here** by the `repo` backend's `dnf_repo` sub-table                    |
| starship                | `:318-320`             | script-url                    | upstream publishes `starship-aarch64-unknown-linux-musl.tar.gz`; `copr:atim/starship` also exists                         | works unchanged                                                                |

## Summary

- **11 of 13** `pkg` entries were already Fedora-correct.
- **2** were outright wrong (`lazygit`, `yazi`): Fedora ships no RPM for either, so every Fedora `dotf apply` exited
  non-zero. Both are fixed by `os = "redhat"` `github-release` siblings.
- **2** were structurally unreachable (`docker-ce`, `lutris`) and vanished **silently** as `not-applicable`. **Both
  are fixed here**: `lutris` by the `lutris-pkg` sibling, and `docker-ce` by the `repo` backend's `dnf_repo`
  sub-table, which configures `/etc/yum.repos.d/docker-ce.repo` and then installs from it. A Fedora
  `dotf apply --dry-run --headless` now plans `plan.repo=docker-ce`.
- Everything else either works unchanged or degrades in ways that are not Fedora-specific.

## Closing notes

- **No version is asserted.** The only pinned strings in this document are upstream release tags recorded in
  `manifest/packages.toml`, not Fedora package versions.
- **RPM Fusion is deliberately not enabled.** Nothing in the manifest needs it: every dnf name that resolves above
  comes from Fedora's own release/updates repositories.

## References

- [Fedora Packages](https://packages.fedoraproject.org/)
- [Fedora COPR](https://copr.fedorainfracloud.org/)
- [Fedora Asahi Remix](https://asahilinux.org/fedora/)
