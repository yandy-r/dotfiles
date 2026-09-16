# The Negative Space Explorer — dotf CLI Enhancements & Script-Retirement

_Persona: Negative Space (Asymmetric Research Squad). What is NOT there — gaps, silences,
missing commands, untested error paths. Research date: 2026-07-04._

---

## Executive Summary

dotf is a well-built, well-tested tool — which is exactly why its **absences** are so
diagnostic. The code that exists is clean; the void around it is where the roadmap lives.
Mapping the negative space against the project's own stated goal (retire the 19
`custom-script` shell-outs into native backends), three shapes dominate:

1. **The schema cannot describe what the 19 scripts do, and — more damning — cannot verify
   that a migration succeeded.** There is no `Package.Binary`/`Provides` field, so
   `Installed()` probes the manifest `name` (a group label like `aws-tools`,
   `hashicorp-tools`, `gnu-utilities`) as if it were a binary on PATH
   (`internal/backend/custom_script.go:49-55`). **You cannot safely retire a script when you
   cannot tell whether its native replacement actually installed the tool.** This single
   missing field is the tightest blocker on the whole migration — it gates _verification_,
   not just cosmetics. (The Contrarian and Historian both hit the bug from the runtime side;
   the negative-space read is that it is a _missing schema field_, and its absence is what
   makes migration unverifiable.)

2. **The command surface has conspicuous, load-bearing holes.** Ten dispatchable verbs
   (`main.go:59-86`): version/platform/plan/install/status/diff/link/sysconfig/apply/
   completion. **Absent: `update`/`upgrade`, `doctor`/`health`, `add`/`new` (scaffold),
   `export`/`snapshot`, `edit`, `list`/`search`, and any single-package targeting
   (`--only`/`--skip`/`--package`).** Two of these — `--only <pkg>` and `doctor` — are direct
   enablers of the migration workflow itself (you cannot install/test one ported tool in
   isolation, and nothing validates that a checksum pin is still live).

3. **For a repo whose CLAUDE.md is 30k+ tokens of agent instructions, there is no
   machine-readable (`--json`) output and no verbosity control anywhere in the CLI.** Output
   is ad-hoc `key=value` lines (`status.go:115`, `install.go:287`); agents and CI parse those
   far less robustly than JSON, and there is no `--verbose` to surface the argv a backend
   actually runs. This is the biggest _invisible_ UX gap: nobody in the planning docs mentions
   it, yet the tool is built to be driven by agents.

Confidence: **High** on all absence claims (verified by reading every `cmd/dotf/*.go` dispatch
path, all seven backends, `types.go`, `validate.go`, the manifest, README, and the PRD).

---

## Missing Subcommands (ranked by pain)

Dispatch is a flat switch in `main.go:59-86`; the help registry (`helptext.go:47-211`) is the
exhaustive command catalog. What is missing, worst-first:

| Rank | Missing verb                                                       | Why it hurts / what it unblocks                                                                                                                                                                                                                                                                                                                                                                                                                             | Effort | Pain         |
| ---- | ------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ | ------------ |
| 1    | **`--only <pkg>` / `--skip <pkg>` targeting** (a flag, not a verb) | Headless install/apply can ONLY select by `--intent` (`install.go:53`, `applyopts.go:37`). There is **no way to install or test a single package by name** from the CLI — per-package selection exists _only_ in the interactive TUI (`apply.go:111`, `packagesByName`). This directly blocks the Historian's prescribed "vertical slice, one tool per PR" migration: you cannot `dotf install --only lefthook` to prove a port before deleting its script. | Low    | **Blocking** |
| 2    | **`doctor` / `health`**                                            | Nothing validates the environment or manifest health: no check that pinned `script-url`/`github-release` checksums still resolve (would have caught the stale starship pin the Contrarian found), no PATH sanity, no "is `~/.local/bin` on PATH so github-release binaries resolve" check, no missing-toolchain warning (tmux-sessionizer needs cargo). `dotf status` reports _package_ drift but never _config/environment_ health.                        | Med    | High         |
| 3    | **`update` / `upgrade`** (update installed tools)                  | The topgrade-shaped hole. Today the only "update" is the _silent, non-deterministic_ self-update baked into mise/rust/atuin custom-scripts on every apply (`custom_script.go:30-35`). There is no explicit, opt-in "update everything" and no way to update _without_ re-running the whole apply.                                                                                                                                                           | Med    | High         |
| 4    | **`export` / `snapshot`** (host → manifest)                        | `brew bundle dump`'s reverse operation. dotf only goes forward (manifest → host). A stranger bootstrapping their own manifest must hand-write TOML; there is no "capture what I already have installed." High onboarding value (Historian's Forgotten Alternative #2).                                                                                                                                                                                      | Med    | Med-High     |
| 5    | **`add` / `new`** (scaffold a `[[package]]`)                       | No guided "add a package" path. Combined with the missing onboarding docs (below), the _only_ way to learn the schema is to read `manifest/packages.toml` comments or `types.go`. A scaffold command is also the natural home for the Historian's "freeze policy" (reject a new `custom-script` unless justified).                                                                                                                                          | Med    | Med          |
| 6    | **`self-update`** (update the dotf binary)                         | PRD explicitly notes this is absent (`provisioning-cli.prd.md` deferred list). The bootstrap shim re-fetches `releases/latest`, so it is a re-run away — genuinely low priority, but its total absence means there is no in-tool "am I on the latest dotf" signal.                                                                                                                                                                                          | Low    | Low-Med      |
| 7    | **`unlink` / `uninstall`**                                         | Additive-only is a defended design choice for _packages_ (chezmoi parity). But **symlink `unlink` is safe and absent**: `dotf link` creates, nothing removes. A user who wants to cleanly back out the symlink set has no inverse operation (only manual `rm`).                                                                                                                                                                                             | Low    | Med          |
| 8    | **`list` / `search`**                                              | No way to enumerate intents, packages, or backends without opening the TOML. `dotf status` lists packages but couples that to a live host probe. A pure `dotf list intents` / `list packages --intent ai` is missing.                                                                                                                                                                                                                                       | Low    | Low-Med      |
| 9    | **`edit`**                                                         | chezmoi/git-style "open the manifest in `$EDITOR`." Trivial, absent, minor.                                                                                                                                                                                                                                                                                                                                                                                 | Low    | Low          |

**Meta-absence:** there is no `--only`/`--skip` at all, which is notable because the guided
TUI already supports per-package selection (`tui.Selection.Packages`, `apply.go:99-111`) — the
capability exists internally but is not exposed to the headless/scriptable path.

---

## Manifest Schema Gaps (exhaustive, field-level vs. what the 19 scripts need)

Source of truth: `internal/manifest/types.go:73-134` (the `Package` struct) and
`validate.go:154-254`. The `Package` struct is a flat superset of every backend's fields.
Enumerating what each of the 19 `custom-script` tools needs that the schema **cannot express**:

### Verification / state (the migration-blocking gap)

- **No `Binary string` / `Provides []string`.** `Installed()` for custom-script, github-release,
  script-url, and toolchain all `LookPath(pkg.Name)`. For 8/19 custom-scripts the installed
  binary ≠ `name` (`rust`→`cargo`/`rustc`, `aws-tools`→`aws`/`sam`, `azure-cli`→`az`,
  `hashicorp-tools`→`terraform`/`vault`, `container-tools`→`podman`, `tpm`→*no binary*,
  `tmux-sessionizer`→`tms`, `gnu-utilities`→`g*`). **Without this field you cannot tell whether
  a ported tool installed** — the single most important missing field for the migration.
- **No `check`/`test` command field.** Some tools (tpm = a git clone into
  `~/.tmux/plugins/tpm`, `install-tpm.sh:14`) have _no binary at all_; the only correct
  "installed?" probe is a path/dir existence test the schema can't hold.

### Versioning / pinning

- **No version/tag pin for `pkg` or `toolchain`.** Only `github-release` has `Tag`
  (`types.go:98`). You cannot say "install ripgrep 14.x" via `pkg`, or pin a cargo crate
  version. mise/aqua both pin; dotf cannot for its two most-used backends.
- **No lockfile / no resolved-version record.** Nothing captures "what version did apply
  actually install," so re-apply is non-reproducible (Contrarian FLAW 3).

### Multi-arch / multi-OS asset resolution

- `github-release` **does** template `{{.OS}}`/`{{.Arch}}` (`github_release.go:96-102`) — so the
  objective's "no `{{.OS}}`/`{{.Arch}}` templating" premise is **partially wrong** (see
  Contradictions). BUT: `Asset` is a single string and `SHA256` is a single string, so **one
  entry cannot carry per-arch/per-OS checksums** — the manifest itself admits this
  (`packages.toml:184-185`: "a single sha256 cannot cover {{.Arch}}-templated assets").
  Missing: a `SHA256` _map_ (keyed by os/arch) or a `checksums.txt`-URL field.
- **No libc variant** (musl vs gnu). Many release assets differ by libc; no field selects it.

### Ordering / dependencies

- **No `depends_on` / ordering.** `tmux-sessionizer` needs the rust toolchain first
  (`install-tmux-sessionizer.sh` `ensure_cargo`); packages install in manifest order with no
  declared graph. tea/hashicorp/aws also have implicit ordering the schema can't state.

### Applicability gating (per-package)

- **`Package` has NO `os`/`arch`/`when`/`enabled` field** — only `Symlink` has `OS`
  (`types.go:150`). `gnu-utilities` is macOS-only and `docker-ce` is apt-only, but that gating
  lives _inside the backend_ (`apt_repo.applicable`) or _inside the script_ (`uname -s` check),
  never declaratively. The Historian's "make applicability first-class in the manifest" is a
  genuine missing field.

### Post-install / setup steps

- **No post-install hook field** (chmod, PATH export, `tmux source`, symlink, `mkdir`). Several
  scripts do post-install work a bare github-release/script-url backend cannot reproduce.
- **No env/args passthrough to `custom-script`.** `Install` runs `env NON_INTERACTIVE=true bash
  <script>` (`custom_script.go:42`) with a *fixed* env — no way to pass `INSTALL_DIR`, a
  version, or per-host args from the manifest.
  


### Missing backend _kinds_ (not just fields)

- **No `git-clone` setup-step backend** (tpm: `git clone --depth 1` into `~/.tmux/...`).
- **No VCS-toolchain support** (tmux-sessionizer: `cargo install --git <repo>`; the toolchain
  backend only does `cargo install <name>` / `npm install -g <name>`, `toolchain.go:48-57`).
- **No `brew-bundle`/formula-list backend** (gnu-utilities loops 12 brew formulae,
  `install-gnu-utilities.sh`).
- **No `rpm-repo` variant** (Fedora azure/aws degrade to custom-script; register.go registers 7
  backends, none rpm). Confirmed by the Contrarian; it is an _absence_ in the taxonomy.

### Cosmetic / metadata

- **No per-package `description`** (only `Intent` has one, `types.go:57`). `dotf list`/`status`
  cannot explain what a package is.
- **No `tags`/`groups` beyond intents.** Intents are the only grouping primitive; there is no
  cross-cutting tag (e.g. "rolling-release", "needs-sudo", "setup-step").
- **No honest `setup-step` tier** distinct from `custom-script` (Historian #5) — tpm/
  tmux-sessionizer/gnu-utilities/mise/rust are lumped with tools that are really just
  github-release binaries.

---

## Testing / Observability Gaps

**Test coverage holes (the untested error paths):**

- **The `Installed()` mismatch bug is entirely untested.** `custom_script_test.go` has 4 tests
  (`RejectsUnsafePaths`, `ExecutesRepoRelativeScript`, `UsesDOTFILES_ROOT`, `Installed_LookPath`)
  — none asserts the _correct_ behavior for a tool whose binary ≠ name. There is no test that
  would fail because `aws-tools` reports `installed=false` on a host with `aws` present. **The
  known bug cannot be caught by CI** because no test encodes the intended contract.
- **No custom-script `Install` failure-propagation test.** The 4 tests cover the happy path and
  path-safety; there is no test that a non-zero script exit surfaces as an error through
  `applyPackages` → `failure.Collector` → exit 1.
- **No orchestration-level idempotency test.** `TestApplyDirs_Idempotent` (dirs only) and
  `TestAptRepoBackend_Install_RerunConverges` (apt only) exist, but there is **no "run `apply`
  twice, assert the second is a no-op/skip" test** — and there _can't_ be, because apply never
  consults `Installed()` before installing (Contrarian FLAW 3). The absence of the gate and the
  absence of the test are the same hole.
- **No `status` accuracy test against the 8 mismatched tools** — status/diff correctness for
  custom-script is unverified.

**Observability holes:**

- **No `--json` / structured output anywhere.** Every command emits hand-rolled `key=value`
  lines (`status.go:115`, `install.go:280-295`, `applyphases.go:286-306`). For an
  agent-driven repo this is the most conspicuous missing affordance — agents and CI want stable
  JSON, not `sort`-order-dependent greppable text.
- **No verbosity / log-level control.** `grep` for `verbose|quiet|--log|loglevel` across
  `cmd/`+`internal/` returns **nothing**. There is no `--verbose` to print the argv a backend
  runs, no `--quiet`, no structured logging. During `apply`, a failing package prints its Go
  error to stderr but you cannot see _what command was attempted_ without reading source.
- **No progress / timing during `apply`.** A long headless apply (19 scripts) gives no
  per-step progress, ETA, or timing — just the final summary (`apply.go:159-185`).
- **No integration test on a real distro.** `go.yml` is `ubuntu-latest` unit tests only; there
  is no `schedule:`/nightly and no dockertest (grep confirms none). The whole
  per-(distro × backend) matrix is asserted via the `exec.Fake` mock — argv is checked, real
  execution on Fedora/openSUSE/Arch/macOS is never exercised in CI. (The PRD _acknowledges_
  this deferral; the negative-space point is that it remains a total absence.)

---

## Docs / Onboarding Gaps

- **No doc teaches a stranger how to add their own package or intent.** README.md (466 lines)
  never shows a `[[package]]` example, never explains the 7-backend taxonomy for end users. The
  only backend documentation is (a) inline TOML comments in `packages.toml` and (b) agent-facing
  CLAUDE.md. A newcomer wanting to add `foo` via `github-release` has no user-facing guide.
- **README documents the _legacy_ install path as if it were current.** §"🔧 Specialized
  Installer Scripts" (README:362-403) still presents `bin/install-*.sh` as the user-facing way
  to install tools (`# Install all AWS tools`, `# Preview without installing`), contradicting
  the manifest-driven `dotf apply` reality. A reader following the README would run the very
  scripts the project is trying to retire.
- **Doc drift on the shim size — in _two_ docs.** CLAUDE.md calls `bootstrap.sh` a "~45-line
  POSIX shim"; the **PRD** (`provisioning-cli.prd.md:204`) also says "~45-line." Actual:
  **307 lines** (`wc -l bootstrap.sh`). (Contrarian flagged CLAUDE.md; the PRD is _also_ stale.)
- **PRD calls the manifest "a 7-package skeleton"** (`prd.md:204`) — it now has ~37 packages
  across 8 intents. The planning docs describing the current state are themselves out of date,
  which matters because they are what the next contributor reads.
- **No CONTRIBUTING rule / freeze policy** for new packages (Historian #3) — nothing documents
  "new tools must use a native backend unless justified."
- **No end-user CHANGELOG surfacing the bash→Go migration**; no man page (completion scripts
  exist, but no `man dotf`).
- **`dotf help` is excellent and internal** (rich per-command help, `helptext.go`), but the
  README/onboarding layer that a _first-time_ user reads is the stale part.

---

## Idempotency / State Absences

- **No run-once / content-hash state** (chezmoi `run_once_`/`run_onchange_` equivalent;
  Historian #1). `custom-script` re-invokes every script every apply and relies entirely on each
  script's internal self-skip. There is no per-package state file recording "already ran this,
  content unchanged."
- **No `Installed()` gate in the install/apply loop.** `installPackage` (`install.go:93-112`)
  gates only on `Applicable`, then calls `Install` unconditionally. Every apply re-runs all 14
  pkg installs and re-execs all 19 scripts. Convergent-by-luck, never by design.
- **No record of "what the last apply did."** No transaction log, no `~/.local/state/dotf/`
  ledger. `dotf status` re-probes live each time; there is no history.
- **No selection persistence between `plan` and `apply`.** Confirmed: `plan.go` writes nothing
  that `apply` reads (`printPlanPreview` → stdout only). `plan` is purely advisory; the user
  re-selects in apply's TUI. The PRD lists this as knowingly absent.
- **No rollback beyond snapper.** `--snapper` (`apply.go:127-129`) is a Linux/btrfs-only,
  best-effort, whole-filesystem snapshot — non-fatal if unavailable (`applyphases.go:25-34`).
  There is no dotf-level undo, no per-package rollback, nothing on macOS or non-btrfs Linux.
- **No lockfile** (deliberate, per Homebrew lineage — Historian #6 — but _undocumented_ as a
  decision, so it reads as a gap rather than a choice).

---

## Conspicuous Absences in Planning Docs (the silences)

Things NO planning doc (PRD, plans/, CLAUDE.md) meaningfully discusses that a mature tool would:

1. **Machine-readable output for agents.** The repo is saturated with agent tooling
   (`.claude/`, huge CLAUDE.md), yet `--json` / structured output is discussed _nowhere_. The
   one silence most at odds with the repo's own identity.
2. **Observability** — logging, verbosity, per-step timing/progress. Unmentioned.
3. **Updating tools (not just installing).** The PRD is a _provisioning_ spec; "keep my tools
   current" (topgrade's whole reason to exist) is never scoped in or out. Silent.
4. **The uninstall/unlink story.** Package removal is explicitly deferred behind `--prune`
   (defensible), but _symlink_ unlink — safe and simple — is never mentioned as a gap.
5. **The escape hatch's growth trajectory.** The research briefs discuss it, but no _plan_
   contains a freeze policy, a `setup-step` tier, or a per-tool migration checklist. There is no
   scheduled decommission (the exact "if it's not scheduled it won't happen" failure the
   Historian documents).
6. **Multi-user / non-root operation** beyond the AUR privilege-drop. What happens on a locked-
   down host with no sudo? Undiscussed.
7. **Checksum-rot maintenance.** No plan addresses _how_ pins stay current (aqua/Renovate
   pattern) — so pinning is added as a burden with no maintenance answer (proven by stale
   starship).

---

## Friction Points (invisible until you hit them)

- **"Just install one tool" is impossible headlessly.** `dotf install --only lefthook` doesn't
  exist; you must select an entire intent or drop into the TUI. (Biggest day-to-day friction,
  and it blocks the migration test loop.)
- **`dotf status` lies about 8 tools**, so the one command whose job is to report truth is
  untrustworthy for the opt-in intents — and there's no `--json` to even scan it reliably.
- **A stale checksum silently uninstalls a default tool** (starship) with only a non-fatal
  stderr line and exit 1 — no `doctor` would have warned you in advance.
- **Following the README installs via the legacy scripts** the project is retiring.
- **No way to see what `apply` is actually running** (no `--verbose`), so debugging a failed
  backend means reading Go source to reconstruct the argv.
- **Re-running `apply` re-does all 19 scripts** (fork + re-detect each), with rolling tools
  silently changing version — slow and non-deterministic.

---

## Barriers by Category

| Category                  | Barrier (the absence)                                                                                        | Blocks                          | Severity     |
| ------------------------- | ------------------------------------------------------------------------------------------------------------ | ------------------------------- | ------------ |
| **Migration-enabling**    | No `Binary`/`Provides` field → can't verify a port installed                                                 | Safe script retirement          | **Critical** |
| **Migration-enabling**    | No `--only <pkg>` targeting                                                                                  | Per-tool test-then-delete loop  | **Critical** |
| **Schema expressiveness** | No git-clone / cargo-git / brew-list / rpm-repo backends; no post-install; no per-pkg os/arch; no depends_on | Porting the setup-step tools    | High         |
| **Correctness/verify**    | Installed() untested; no idempotency-on-rerun test                                                           | Trusting status/diff            | High         |
| **UX / agents**           | No `--json`, no `--verbose`, no progress                                                                     | Agent/CI consumption, debugging | High         |
| **Command surface**       | No doctor/update/export/add/unlink                                                                           | Day-2 operations                | Med-High     |
| **Onboarding**            | README teaches legacy path; no "add a package" doc                                                           | New contributors                | High         |
| **State/idempotency**     | No run-once state, no last-run ledger, no plan→apply persistence                                             | Reproducibility                 | Med          |
| **Docs accuracy**         | "45-line shim" (307), "7-package skeleton" (37) in CLAUDE.md + PRD                                           | Trust in docs                   | Med          |

---

## Key Insights

1. **The migration is blocked at _verification_, not _translation_.** The loudest missing thing
   is a `Package.Binary`/`Provides` field: without it you can port a script to `github-release`
   but still can't tell dotf whether the tool landed. Ship this field (and fix `Installed()` to
   use it) **before** porting anything — it is the prerequisite the other personas hit as a bug
   but is really a schema hole.
2. **Two absences are quietly the migration's _enablers_, not just nice-to-haves:**
   `--only <pkg>` (so you can install/test one ported tool) and `doctor` (so you can verify a
   pin is still live). The Historian prescribes "vertical slice, delete on cutover"; the CLI
   physically cannot do a vertical slice today.
3. **The single most out-of-character gap is `--json`/observability.** An agent-first repo with
   no machine-readable output and no verbosity control is a self-inflicted friction that _no
   planning doc even names_. Cheap to add, disproportionately valuable here.
4. **The command surface is missing its entire "day-2" half.** dotf can provision (day-1) but
   has no `update`, `doctor`, `export`, `unlink`, or `list` — the verbs you reach for _after_
   the first install. topgrade/brew-bundle/chezmoi all ship these.
5. **The docs describe a tool that no longer exists.** README + PRD still frame bin/install-\*.sh
   and a "45-line shim / 7-package skeleton" as current. The onboarding surface is the staleest
   part of the repo, and it points newcomers at exactly the scripts being retired.
6. **Absence of tests around the known bug means the bug is load-bearing.** No test encodes the
   correct `Installed()` contract, so even after a fix there is nothing to prevent regression —
   the test gap _is_ part of the barrier.

---

## Evidence Quality

- **Command-surface absences (subcommands, flags):** **High** — read the full dispatch switch
  (`main.go:59-86`), the help registry (`helptext.go`), and every `set*Flags` function; grepped
  for `--only/--skip/--package/--json/verbose/update/doctor` and confirmed none exist.
- **Schema field gaps:** **High** — read `types.go:73-134` and `validate.go` field-by-field, and
  cross-checked against the actual setup-step scripts (`install-tpm.sh`, `-tmux-sessionizer.sh`,
  `-gnu-utilities.sh`) to confirm git-clone / cargo-git / brew-list / no-binary needs.
- **`github-release` templating:** **High** — `templateGitHubAsset` (`github_release.go:96-102`)
  _does_ implement `{{.OS}}`/`{{.Arch}}`; the objective's premise is corrected here.
- **Test gaps:** **High** — enumerated test function names in `custom_script_test.go` and grepped
  for idempotency/rerun/Installed tests across `cmd/` and `internal/backend`.
- **Doc drift:** **High** — `wc -l bootstrap.sh` = 307 vs "45-line" in CLAUDE.md and
  `prd.md:204`; README §362-403 read directly.
- **Observability/JSON absence:** **High** — grep returned zero hits for verbosity/json flags.
- **Overlap with siblings (Contrarian's Installed() bug, stale pin, no gate; Historian's
  content-hash state, export, setup-step tier):** their findings are cited and re-framed as
  _absences_; not independently re-derived.

---

## Contradictions & Uncertainties

1. **Objective premise partially wrong:** it lists "no `{{.OS}}`/`{{.Arch}}` templating" as a
   gap, but `github-release` _has_ it (`github_release.go:96-102`). The _real_ gap is downstream:
   a single `Asset`+`SHA256` string can't carry per-arch checksums. Corrected, not smoothed.
2. **"Additive-only / no lockfile is a gap" vs "it's a deliberate Homebrew-lineage choice"**
   (Historian/Contrarian). Resolution: it's a defensible choice that is _undocumented as such_,
   so it reads as a gap. The fix is a doc, not a feature.
3. **`self-update` — is its absence a real gap?** The Contrarian calls it "over-valued"; the
   bootstrap shim re-fetches `releases/latest`. I rank it low precisely because a re-run
   substitutes. Genuine judgment split, flagged not resolved.
4. **`--prune`/uninstall for packages** — deferred by design (chezmoi parity). I do **not**
   argue for it. But I separate _symlink_ unlink (safe, absent, worth adding) from _package_
   removal (correctly deferred) — the planning docs conflate them under one "no reconciliation."
5. **Uncertainty:** exact per-tool binary names for `coderabbit`/`turso`/`tea`/`opencode`/
   `codex` were not each byte-verified (matches the Contrarian's caveat); the `Binary`/`Provides`
   argument holds regardless of the exact per-tool value.

---

## Search Queries Executed (codebase reads + greps)

1. `main.go` dispatch switch — enumerate existing verbs (`grep 'case "'`).
2. `grep -riE 'doctor|uninstall|self-update|"update"|"upgrade"'` across `cmd/` — confirm absent.
3. `grep -riE '"(package|only|skip|pkg|exclude)"' ... flag` — confirm no single-package targeting.
4. `grep -riE 'verbose|quiet|--log|loglevel'` across `cmd/`+`internal/` — confirm no verbosity.
5. `grep -riE 'json'` across `cmd/`+`internal/` — confirm no `--json` output path.
6. `grep -riE 'lockfile|statefile|run-once|persist|state\.json'` — confirm no state/lockfile.
7. Read `types.go` + `validate.go` field-by-field vs `packages.toml` backend usage.
8. Read setup-step scripts (`install-tpm.sh`, `-tmux-sessionizer.sh`, `-gnu-utilities.sh`) for
   git-clone / cargo-git / brew-list / no-binary needs.
9. `wc -l bootstrap.sh` (307) vs "45-line" claims in CLAUDE.md + PRD.
10. README heading scan + `grep '\[\[package\]\]|add a package'` — confirm no onboarding doc.
11. Test-name enumeration in `custom_script_test.go`; `grep 'func Test.*(Idempot|Rerun|Installed)'`.
12. `.github/workflows/` scan for `nightly|dockertest|integration|schedule:` — confirm none.
13. PRD non-goals/deferred scan (`grep 'non-goal|deferred|NOT Building'`).

---

## Digest — the most painful absences (lead first: what most blocks script-retirement)

- **No `Package.Binary`/`Provides` field → migration is unverifiable.** You cannot safely retire
  a `bin/install-*.sh` when `Installed()` probes the group-label `name` and can't tell whether
  the native port actually installed the tool (`custom_script.go:49-55`, 8/19 mismatched). Ship
  this field + fix the probe _before_ porting anything — it is the tightest blocker.
- **No single-package targeting (`--only <pkg>`/`--skip`).** Headless install/apply select only
  by `--intent`; you can't install/test one ported tool in isolation — which kills the
  vertical-slice, delete-on-cutover migration loop the whole plan depends on.
- **No `doctor`/`health` command.** Nothing validates the manifest or environment — no live-pin
  check (would have caught the stale starship checksum), no PATH/toolchain sanity.
- **No `--json` and no `--verbose`/log control anywhere** — in an explicitly agent-driven repo,
  output is fragile `key=value` text and you can't see the argv a backend runs. No planning doc
  even mentions it.
- **Schema can't hold the setup-step tools:** no git-clone / `cargo install --git` / brew-list /
  rpm-repo backends, no post-install hooks, no per-package `os`/`arch`/`depends_on`, no per-arch
  checksum map. These are the concrete capabilities the 19 scripts need and the 7 backends lack.
- **The known `Installed()` bug is untested, and there is no apply-twice idempotency test** — so
  even after a fix nothing prevents regression, and re-apply re-runs everything with no
  `Installed()` skip gate.
- **Missing the entire "day-2" verb set:** no `update`/`upgrade`, `export`/`snapshot`,
  `unlink`, `add`, or `list` — dotf can provision but can't maintain, capture, or reverse.
- **Docs describe a tool that no longer exists:** README still teaches the legacy
  `bin/install-*.sh` path, and CLAUDE.md _and_ the PRD call the 307-line shim "~45 lines" and the
  ~37-package manifest a "7-package skeleton."
