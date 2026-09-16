# Research Objective: dotf CLI — features, enhancements, and fixes

## Subject

What additional **features, enhancements, or fixes** should be added to the `dotf`
CLI (a Go 1.26 single-binary provisioning tool that replaced a bash bootstrap in
this dotfiles repo), including **finishing the migration of the `bin/install-*.sh`
scripts into native TOML-manifest backends** — that would be genuinely useful and
enhance the user experience.

This is a **hybrid codebase + competitive research** subject. Ground every claim in
BOTH (a) the actual `dotf` code in this repo and (b) how comparable tools solve the
same problems. Do not invent features that ignore the project's explicit scope.

## Grounding Brief (read before searching — this is the current state as of 2026-07-04)

**What dotf is.** Module `github.com/yandy-r/dotfiles`, entrypoint `cmd/dotf/main.go`.
Subcommands: `version`, `platform`, `plan`, `install`, `status`, `diff`, `link`,
`sysconfig`, `apply`, `completion`, `help`. Uses the Go stdlib `flag` package by
deliberate decision (NOT Cobra/pflag — see CLAUDE.md; do not propose adopting a
framework as a "fix"). Interactive TUI via `charmbracelet/huh`. Release via GoReleaser
(`.goreleaser.yml`), `bootstrap.sh` is a POSIX shim (currently ~307 lines) that
downloads the checksum-verified binary and execs `dotf apply`.

**Architecture.**

- `internal/manifest` — TOML schema (`types.go`): `Manifest{Intents, Packages, Symlinks, Sysconfig}`.
- `internal/backend` — 7 canonical backends, one file each, dispatched by an `Engine`
  via a static map. Interface: `Install`, `Installed`, optional `applicable(info)`.
  The 7 backends: `pkg` (native pkg-mgr, per-distro `names` map), `github-release`
  (repo+tag+asset+sha256), `aur` (yay), `toolchain` (cargo/npm), `apt-repo`
  (keyring+sources.list, Debian-only), `script-url` (vendor installer + sha256),
  `custom-script` (runs a repo-relative `bin/install-*.sh` via `env NON_INTERACTIVE=true bash`).
- `internal/{platform,privilege,snapper,sysconfig,symlink,exec,failure,tui,brew}`.
- `internal/exec` — `Runner` interface (real + `Fake` for tests) — everything is
  mockable; tests assert exact argv per (distro × backend).

**The manifest** (`manifest/packages.toml`, `go:embed`-ed, 378 lines).

- Intents: `base` (default), `dev`, `gaming`, and OPT-IN `runtimes`, `ai`, `cloud`,
  `git-tools`, `system`.
- Backend usage counts in the manifest today: **`custom-script` = 19**, `pkg` = 14,
  `aur` = 1, `apt-repo` = 1, `script-url` = 1, `github-release` = 1, `toolchain` = 0
  (commented reference only).

**THE CENTRAL TENSION (the user's core ask).** `custom-script` was designed as "the
shrinking escape hatch" but is today the **single most-used backend (19 of ~37
packages)**. Each of those 19 entries just shells out to a hardened
`bin/install-*.sh`. There are **29 `bin/install-*.sh` scripts totalling ~13,211 LOC**.
The PRD's headline success metric — "**≥80% of bootstrap.sh + bin/install-\*.sh LOC
retired**" — is explicitly **DEFERRED** to a "manifest-population / script-retirement
follow-up" that has not happened. At Phase 7 only ~12% of bash LOC was removed. So the
migration the user asks about is the project's own stated-but-unfinished goal.

The 19 custom-script tools are: atuin, mise, rust, lua, claude, codex, opencode,
fabric, coderabbit, aws-tools, azure-cli, hashicorp-tools, container-tools, turso, tea,
lefthook, tpm, tmux-sessionizer, gnu-utilities. Many of these are GitHub-release
binaries or vendor `curl | sh` installers that map cleanly to `github-release` /
`script-url` / `toolchain` backends — but some (tpm = git clone, tmux-sessionizer,
gnu-utilities, mise/rust as rolling-release version managers) are genuinely
setup-steps, not simple package installs.

**Explicit project scope (from the PRD — respect these; challenge only with evidence).**

- NOT building: package removal / full reconciliation (deferred behind a future
  `--prune`; additive-only, matching chezmoi's stance); secrets orchestration
  (1Password/Bitwarden wiring); fleet/multi-host; Windows-native provisioning
  (WSL-first, `sysconfig` Windows is an `ErrNotImplemented` stub); a GUI.
- Known deferred/absent: no `rpm-repo` backend variant (Fedora azure/aws fall to
  script-url/custom-script); no dockertest distro matrix (deferred to nightly CI);
  no binary self-update command; no selection persistence between `plan` and `apply`.

## Core Research Questions

1. **Script retirement**: For each of the 19 `custom-script` tools, what is the
   correct native backend (`github-release` / `pkg` / `toolchain` / `script-url` /
   `apt-repo`), and which genuinely must remain shell-outs? What backend capabilities
   are MISSING that block a clean migration (e.g. version pinning, `{{.Arch}}`/`{{.OS}}`
   asset templating, post-install steps, git-clone setup, `rpm-repo`, multi-artifact
   extraction)? How do chezmoi/aqua/mise/topgrade model these?
2. **Feature gaps vs. comparable tools**: What do chezmoi, dotbot, yadm, GNU Stow,
   home-manager/nix, mise, aqua, topgrade, rcm, and similar tools offer that `dotf`
   lacks and that fits its single-host, additive, TOML-first, Go scope? (e.g.
   templating, per-host/profile config, secrets refs, `update`/`upgrade`, self-update,
   idempotent re-apply, `doctor`/health, encrypted files, lockfiles, bundle export.)
3. **UX enhancements**: What would most improve day-to-day UX — richer `diff`/`status`,
   a lockfile, `dotf update`, `dotf doctor`, better dry-run/plan output, progress/TUI
   during `apply`, JSON output, shell-completion coverage, `--only`/`--skip` filters,
   idempotency guarantees, profiles/tags?
4. **Fixes / correctness / hardening**: What are the real risks and rough edges in the
   current design — the `custom-script` `Installed()` PATH-probe heuristic, unpinned
   `script-url`/vendor installers (supply chain), `apt-repo` codename hard-coding,
   privilege/sudo handling, the 307-line "shim", drift between the manifest and the
   scripts, test-coverage gaps, error handling?
5. **Sequencing**: Given a solo maintainer's complexity budget, what is the
   highest-value, lowest-risk ORDER to ship these — what's the 20% that delivers 80%?

## Success Criteria

- [ ] All 8 personas deployed with distinct strategies (codebase + web)
- [ ] Each of the 19 custom-script tools assessed for a native-backend target
- [ ] Concrete comparators cited (chezmoi/aqua/mise/dotbot/yadm/home-manager/topgrade)
- [ ] Contradictions captured (e.g. "add reconciliation" vs "additive-only is a feature")
- [ ] Findings mapped to the 7-backend taxonomy and stdlib-flag / TOML constraints
- [ ] Every recommendation tagged with effort, risk, and value; sequenced

## Evidence Standards

- Primary sources preferred: the actual `dotf` Go code + TOML, and official docs of
  comparator tools (chezmoi.io, aquaproj.github.io, mise.jdx.dev, GNU Stow manual).
- Cite file:line for codebase claims and URLs for external claims.
- Confidence ratings on findings; contradictions documented, not smoothed.
- Distinguish "genuinely useful for THIS tool's scope" from "cool but out of scope."

## Perspectives to Consider

- Historical evolution of dotfiles managers (symlink-farmers → templating → binary tools)
- Current state (chezmoi dominance, mise/aqua, the Go-CLI dotfiles ecosystem)
- Future (declarative/Nix pressure, AI-assisted config, reproducibility trends)
- Contrarian (is retiring custom-script even worth it? is additive-only right?)
- What's NOT discussed (rollback, uninstall, testing on real distros, onboarding docs)

## Potential Biases to Guard Against

- **Feature-creep bias**: recommending everything competitors have, ignoring the
  deliberately narrow single-host/additive/solo-maintainer scope.
- **Rewrite bias**: assuming "native Go backend" always beats a working shell script;
  some scripts encode real, hard-won platform logic that is cheaper to keep.
- **Framework bias**: proposing Cobra/pflag/viper — explicitly rejected by the project.
- **Novelty bias**: over-weighting shiny 2030 ideas over boring high-value fixes.

## Scope Extension (2026-07-04) — post-report maintainer asks

After the report landed, the maintainer added two capability questions, researched in
`addendum-lifecycle-and-user-config.md` (same grounding standards):

1. **Anytime per-tool lifecycle**: should `dotf` run at any time (not only bootstrap) to
   install / update / remove a single **named** tool and configure a single **named**
   subsystem — e.g. `dotf install claude --update`, `dotf install claude --remove`,
   `dotf setup ssh` — reusing the `--update`/`--uninstall` lifecycle the `bin/install-*.sh`
   scripts already ship? What is the groundwork that makes this expandable to future
   tools/domains without per-tool Go code?
2. **User-extensible config**: should there be an XDG surface (`~/.config/dotf/`) where a
   user drops TOML fragment(s) to **extend** the embedded manifest (add packages, intents,
   symlinks, setup modules) without editing or forking the `go:embed`-ed manifest?

Both are **groundwork for a future build, not a build task** — the ask is to extend the
research, sequence the work into the existing Phase 0–4 plan, and preserve the additive,
single-host, stdlib-`flag`, governed-escape-hatch constraints.
