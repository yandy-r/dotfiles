# Addendum — Lifecycle Verbs & User-Extensible Config (`~/.config/dotf`)

**Date**: 2026-07-04
**Status**: research extension to `report.md` (groundwork for a future build — **not** a build task)
**Scope**: two capabilities the maintainer requested after the main research landed:

1. **Anytime, per-tool lifecycle invocation** — `dotf install claude --update`,
   `dotf setup ssh`, and eventually `dotf install claude --remove`: the CLI run at any
   time (not only at bootstrap) to install / update / remove a **single named** tool or
   configure a **single named subsystem**, reusing the lifecycle the `bin/install-*.sh`
   scripts already implement.
2. **An XDG user-config overlay** — `~/.config/dotf/` where a user drops TOML fragment(s)
   to **extend** the embedded manifest (add their own packages, intents, symlinks, setup
   modules) without editing the repo's `go:embed`-ed `manifest/packages.toml`.

Both are **grounded** the same way the parent report is: every code claim is cited at
`file:line`; every external claim is tagged to a comparator tool. Both slot into the
report's existing Phase 0–4 sequence and **depend on the same keystone** (the `probe` /
`Installed()` gate). Neither requires the rejected framework (stdlib `flag` is sufficient),
reconciliation, templating, or secrets orchestration.

---

## Part A — Lifecycle verbs: `install <name>`, `--update`, `--remove`, `setup <domain>`

### A.1 The ask, precisely

The maintainer wants the CLI to be a **day-2, per-target operator**, not only a
bootstrap-time batch provisioner:

```
dotf install claude            # install one named tool, anytime
dotf install claude --update   # update it in place (what the script's --update does today)
dotf install claude --remove   # remove it (what the script's --uninstall does today)
dotf setup ssh                 # configure a named subsystem/domain, anytime
```

This is **adjacent to but distinct from** the report's Theme 4 items. The report proposed
`--only <pkg>` / `--skip` as _batch filters_ (report.md:186–189) and `dotf self-update`
for the _binary itself_ (report.md:187). Neither gives the maintainer a **positional,
named, lifecycle-aware** target. This addendum fills that gap.

### A.2 Current state (code-verified)

**The scripts already have the lifecycle; `dotf` throws it away.** The
`bin/install-*.sh` corpus splits into lifecycle tiers:

| Tier                                       | Capability                                                                                                     | Tools (with `file:line`)                                                                                                                               |
| ------------------------------------------ | -------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **A — full `ACTION` dispatch**             | `--update` **and** `--uninstall` (+`--force`,`--help`) parsed into an `ACTION` var, `case "$ACTION"` at bottom | `install-claude.sh:525–541`, `install-codex.sh:508–536`, `install-opencode.sh:595–611`, `install-fabric.sh:513–532`, `install-cursor-agent.sh:543–594` |
| **A′ — remove only**                       | `--uninstall` but no `--update`                                                                                | `install-coderabbit.sh:473–506`, `install-lua.sh:588–630`                                                                                              |
| **B — self-update on re-run**              | no `--update` flag; a second run detects the tool and does a best-effort in-place upgrade                      | `install-atuin.sh:376–380`, `install-mise.sh:231–238`, `install-rust.sh:50–58`                                                                         |
| **C — install/`--check`/`--dry-run` only** | no lifecycle beyond install                                                                                    | most `pkg`-ish bundles (aws/azure/hashicorp/container-tools)                                                                                           |

**But the `custom-script` backend only ever runs a plain install.** It shells out with a
hard-coded argv and **passes no action argument**:

```go
// internal/backend/custom_script.go:42
b.r.Run("env", "NON_INTERACTIVE=true", "bash", scriptPath)
```

So `--update` / `--uninstall` — already written, already tested, already shipped in the
scripts — are **dead capability** from `dotf`'s point of view. The backend interface has
no verb for them either: it is exactly two methods (`internal/backend/backend.go:15–21`):

```go
type Backend interface {
    Install(pkg, info) error
    Installed(pkg, info) (bool, error)
}
```

**And `dotf install` cannot name a single tool.** Its flag surface is `--manifest`,
`--dry-run`, `--intent` (repeatable) only (`install.go:114–131`); after `fs.Parse` it never
reads `fs.Args()`, so `dotf install claude` **silently installs nothing matching** and the
positional `claude` is discarded. Selection is intent-scoped (`packagesForInstall`,
`install.go:168–191`), never name-scoped, on the headless path. (The per-package select
_does_ exist — but only inside the interactive TUI, `tui.Selection.Packages` via
`packagesByName`, `install.go:199–212`.)

**`setup` does not exist as a verb.** The dispatch switch (`main.go:59–86`) knows
`version|platform|plan|install|status|diff|link|sysconfig|apply|completion`. There is no
`setup`, and "configure a subsystem" is today spread across three unrelated surfaces:
`sysconfig` (hostname/timezone/locale/shell/firewall), the symlink table, and a couple of
one-off `bin/*setup*.sh` scripts (`bin/setup-fingerprint.sh`,
`bin/onedrive-main-setup.sh`). For the specific `ssh` example: there is **no ssh script**;
SSH is a single symlink (`internal/symlink/table.go:122`, `ssh/config → ~/.ssh/config`)
plus a firewall port, and sshd enablement is **explicitly deferred** (out of Phase-6 scope,
`docs/prps/plans/provisioning-cli-headless.plan.md:243–246`; tracked as issue **#34** per
the provisioning-parity work).

### A.3 What comparable tools do

| Tool                   | Lifecycle surface                                                                                            | Relevance                                                                                                                                |
| ---------------------- | ------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- |
| **Homebrew**           | `brew install / upgrade / uninstall / reinstall <name>` — the canonical named triad; `brew bundle` for batch | The archetype for "named tool, anytime, three verbs." dotf already bootstraps brew (`internal/brew`) — parity is expected.               |
| **mise**               | `mise install <tool>@<v>`, `mise use`, `mise upgrade`, `mise uninstall`; rolling tools upgrade in place      | Confirms the install/upgrade/remove split maps cleanly onto version managers — the same class dotf keeps as scripts.                     |
| **aqua**               | `aqua i` / `aqua update` / `aqua g` (generate an entry)                                                      | Update-in-place + "add a package" as first-class verbs.                                                                                  |
| **nix / home-manager** | `nix profile install / upgrade / remove`                                                                     | Even the declarative camp ships imperative per-target verbs.                                                                             |
| **topgrade**           | one umbrella "update everything installed"                                                                   | The report's C4 warning: a _global_ `dotf update` overlaps topgrade — so scope `--update` to a **named** target, not "update the world." |
| **chezmoi**            | no package concept; `run_once_`/`run_onchange_` scripts, `chezmoi update` = pull+apply                       | Reinforces report Theme 3: imperative setup-steps are a permanent, first-class category — which is what `setup <domain>` formalizes.     |

**Takeaway:** the install/update/remove triad is _table stakes_, and a "configure a
subsystem" verb (`setup`) is the natural home for the imperative setup-steps the report
already blessed as permanent (Theme 3). Neither is feature-creep; both are parity.

### A.4 Design groundwork (the seams to build on — not a build)

The unifying insight: **decouple the _action_ from the _verb_.** Whether the UX is
`install --update` or a `brew`-style `update` verb, the engine needs one new concept — a
**lifecycle action** threaded from the command layer to the backend — and everything else
is a thin alias over it. Concretely, four seams:

1. **A `Lifecycle` action type** (`Install | Update | Remove`), defaulting to `Install`.
   This is the single new primitive. It rides alongside the report's keystone `probe`:
   `Installed()` answers "is it here?", `Lifecycle` answers "what do I want done?"

2. **Grow the `Backend` interface with _optional_ capability interfaces**, mirroring the
   existing `platformGated` pattern (`backend.go:64–69`) so backends opt in without
   breaking the two-method contract:

   ```go
   type Updater interface { Update(pkg, info) error }
   type Remover interface { Remove(pkg, info) error }
   ```

   `Engine.Update`/`Engine.Remove` type-assert like `Applicable` already does
   (`backend.go:75–84`). A backend that doesn't implement `Remover` yields a clean
   "remove not supported for backend X" — fail-fast, not a silent no-op. Native mappings
   are obvious and already latent in the code: `pkg` → the package manager's
   `remove`/`upgrade` verb (the authority-query at `pkg.go:109–142` already knows how to
   talk to it); `toolchain` → `npm -g update` / `uninstall`; `github-release` →
   re-resolve + re-download (update) / delete the managed binary (remove).

3. **Teach `custom-script` to pass the action through** — the smallest, highest-leverage
   change, because it _reactivates already-written script code_. Extend `custom_script.go`
   so `Update`/`Remove` append the script's own flag:

   ```go
   // install: env NON_INTERACTIVE=true bash <script>
   // update:  env NON_INTERACTIVE=true bash <script> --update
   // remove:  env NON_INTERACTIVE=true bash <script> --uninstall
   ```

   Gate on a **declared capability** so dotf never passes `--update` to a Tier-B/C script
   that doesn't parse it (which would either error or, worse, re-install). The manifest
   field that declares this is the groundwork's one schema addition — e.g.

   ```toml
   [[package]]
   name = "claude"
   backend = "custom-script"
   script = "bin/install-claude.sh"
   actions = ["install", "update", "remove"]   # which flags the script honors
   ```

   This is DRY with the report's `probe` field: both are "small typed facts about a
   package the engine currently has to guess."

4. **A `setup` verb backed by a "module/task" concept** — distinct from `package`.
   A package is _a thing you install_; a **module** is _a subsystem you configure_ (ssh,
   git, gpg, shell, fingerprint, onedrive). A module is an **ordered pipeline** that may
   compose existing primitives — a `pkg`/`custom-script` prerequisite, a symlink, a
   `sysconfig` domain, a firewall rule, a post-config step. The two existing
   `bin/*setup*.sh` scripts (`setup-fingerprint.sh`, `onedrive-main-setup.sh`) are the
   **seed modules**; `ssh` is the aspirational one that lands when #34 lands (sshd install
   - `ssh-keygen -A` + service enable + the existing firewall port + the existing config
     symlink). Schema sketch:

   ```toml
   [[module]]
   name = "ssh"
   description = "OpenSSH client config + (future) server enablement"
   steps = [
     { symlink = "ssh/config" },
     { sysconfig = "firewall", ports = ["22/tcp"] },
     { custom-script = "bin/setup-ssh.sh" },   # future, gated on #34
   ]
   ```

   Keeping module steps a **typed, constrained list** (not free-form shell) is the same
   fence the report drew around `post_install` (report.md:279) — it stops `setup` from
   becoming `custom-script v2`.

**Verb-shape decision (surface as a tension, recommend A):**

- **Shape A — lifecycle flags on `install`** (`install claude --update` / `--remove`).
  Matches the maintainer's literal ask; **adds zero new verbs**; keeps `main.go`'s switch
  small. Slight awkwardness: `install … --remove` reads oddly.
- **Shape B — brew-style verbs** (`update claude`, `remove claude`). Reads naturally;
  grows the verb set and the completion/help surface.

Because both compile down to the same `Lifecycle` action, **the verb shape is a late-bound
UX choice, not an architecture choice** — build the action-parametric engine first (A.4.1–3),
pick the surface last. Recommendation: ship **Shape A** (`install <name> [--update|--remove]`)
to match the ask and minimize surface, keep `setup <domain>` as its own verb, and leave
`update`/`remove` verbs as optional thin aliases if demand appears.

### A.5 Contradictions & tensions (preserved, not smoothed)

| #      | Tension                                                                                                                                                                | Where the evidence leans                                                                                                                                                                                                                                                                                                                                                                      |
| ------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **A1** | Per-tool `--remove` (imperative, reuses `--uninstall`) **vs** the project's "no removal / additive-only, `--prune` deferred" scope (objective.md:64–66; report.md:197) | **Not the same thing — build it.** `--prune`/reconciliation removes _whatever isn't in the manifest_ (declarative, dangerous, deferred). `dotf install claude --remove` is an _explicit, named, imperative_ action the user typed — exactly like `brew uninstall` coexisting with a Brewfile. Additive-only governs the _declarative_ path; it does not forbid an _explicit imperative_ verb. |
| **A2** | Giving `custom-script` `--update`/`--remove` passthrough **vs** the report's "fence / shrink the hatch" governance (Theme 3)                                           | **Parity, not re-inflation.** Lifecycle is a _cross-backend_ capability (native backends get it too); custom-script merely reaches the parity it already had in the scripts. The probe/ratchet/`status --backends` count still apply unchanged.                                                                                                                                               |
| **A3** | A `setup ssh` example **vs** sshd being out of scope / deferred to #34                                                                                                 | **Build the verb now, land the domain later.** `setup` + the module concept are the extensible groundwork; the _first_ modules are the ones with real logic today (fingerprint, onedrive, git, shell); `ssh`'s server half is a module step that activates when #34 does.                                                                                                                     |
| **A4** | New verbs/flags **vs** small stdlib-`flag` surface (CLAUDE.md)                                                                                                         | Shape A adds **flags, not verbs** — no framework pressure. `setup` is one new verb, justified by a whole imperative category the report already recognized.                                                                                                                                                                                                                                   |
| **A5** | `--update` a named tool **vs** report C4 (`update` collides with topgrade/additive)                                                                                    | **Resolved by naming.** C4's concern was a _global_ "update everything." A _named_ `--update <tool>` that delegates to the tool's own updater is precise and topgrade-orthogonal.                                                                                                                                                                                                             |

### A.6 Effort / value / risk

- **Reactivate script lifecycle via `custom-script` passthrough + `actions` field** —
  _Effort: low. Value: high (turns dead script code into working `dotf` verbs). Risk: low
  (gated by declared `actions`)._
- **Optional `Updater`/`Remover` backend interfaces + `Engine` dispatch** —
  _Effort: low-med. Value: high (uniform lifecycle across all backends). Risk: low
  (opt-in, mirrors `platformGated`)._
- **Positional named target on `install` (`dotf install <name>`)** —
  _Effort: low. Value: high (the ergonomic core of the ask). Risk: low (reuses
  `packagesByName`, `install.go:199–212`)._
- **`setup` verb + typed `module` schema** —
  _Effort: med. Value: high (formalizes the permanent imperative category). Risk: med
  (design the step vocabulary; keep it typed/constrained)._
- **Depends on** the report's keystone `probe`/`Installed()` gate (Phase 1) so that
  `install <name>` is idempotent and `--update`/`--remove` are honest.

---

## Part B — User-extensible config: the `~/.config/dotf` overlay

### B.1 The ask, precisely

A user (this repo's owner on any machine, or anyone cloning it) should be able to
**extend** what `dotf` manages **without editing the embedded, versioned manifest** — by
dropping their own TOML under `$XDG_CONFIG_HOME/dotf/` (default `~/.config/dotf/`):

```
~/.config/dotf/
  config.toml            # dotf's own settings (default intents, flags, roots)
  packages.d/*.toml      # user packages / intents (merged onto the embedded manifest)
  modules.d/*.toml       # user setup modules (Part A)
  bin/                   # user-local scripts referenced by custom-script entries
```

"Expand manifests and tools by adding TOML file(s)" = **a merge layer**, not a replacement.

### B.2 Current state (code-verified)

**Manifest resolution is single-source, pick-one, fail-fast — no overlay.**
`manifest.Resolve` (`internal/manifest/load.go:64–78`) honors a strict precedence and
returns **exactly one** manifest:

```
--manifest flag  >  $DOTF_MANIFEST env  >  embedded manifest/packages.toml
```

Whichever wins is loaded _whole_; the others are ignored. There is no `Merge`, no layering,
no drop-in directory. The embedded manifest (`dotfiles.ManifestFS`, `go:embed`-ed and passed
into `Resolve` at `install.go:42`) is authoritative and **closed** — a user cannot add a
package without either forking the repo or replacing the entire manifest via `--manifest`
(losing every curated default in the process).

**The decoder is strict.** `Parse` sets `DisallowUnknownFields()` (`load.go:24`), so a user
fragment must conform exactly to the typed schema (`types.go`) — good for fail-fast, but it
means the overlay's shape has to be a _first-class part of the schema_, not a loose sidecar.

**There is no notion of dotf "settings"** — only the manifest and a scatter of env vars
(`DOTF_MANIFEST`, `DOTFILES_ROOT`, `DOTFILES_DIR`, per-script `NON_INTERACTIVE`/`FORCE_INSTALL`/
`DRY_RUN`). No `~/.config/dotf/config.toml` exists.

**The seam is already there.** `Resolve` is the _single_ chokepoint every command funnels
through for the manifest. Turning "pick one" into "merge a precedence stack" is a localized
change to one function plus one new `Merge`, reusing the existing typed schema and validator
(`Validate`, `validate.go`) unchanged.

### B.3 What comparable tools do (the overlay is a solved problem)

| Tool                        | Layering model                                                                                                            | Borrowable idea                                                                         |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| **aqua**                    | `AQUA_GLOBAL_CONFIG` = a list of extra config files **merged** with the local `aqua.yaml`; plus a registry concept        | The exact "base + user global overlay files" model dotf needs.                          |
| **mise**                    | Hierarchical: `~/.config/mise/config.toml` (global) → `mise.toml` (project) → `mise.local.toml`, **merged by precedence** | Confirms TOML + XDG global + precedence-merge is idiomatic and expected.                |
| **chezmoi**                 | `~/.config/chezmoi/chezmoi.toml` is _settings_, separate from the source-state dir                                        | Clean split: dotf's `config.toml` (settings) vs `packages.d/` (manifest overlay).       |
| **Homebrew**                | `brew tap owner/repo` adds third-party formula **sources**; Brewfile anywhere                                             | The "user adds a source of installable things" model → future `dotf tap`.               |
| **git**                     | `include`/`includeIf`, `~/.config/git/config` + repo `.git/config` **layered**                                            | Precedent for config include + machine-conditional overlay (`includeIf` ≈ per-host).    |
| **systemd / apt / sudoers** | `*.d/` drop-in directories; last-wins, additive                                                                           | Precedent for **`packages.d/*.toml` fragment discovery** over a single monolithic file. |

**Takeaway:** every mature tool in this space has a merge/overlay layer; a single closed,
embedded manifest is the anomaly. The `*.d/` drop-in dir (systemd/apt) is the most
extensible discovery shape and the best fit for "drop a file to add a tool."

### B.4 Design groundwork (the seams to build on — not a build)

1. **`manifest.Merge(base, overlays ...*Manifest) (*Manifest, error)`** — a new pure
   function beside `Parse`/`Validate`. Semantics, chosen to honor the additive philosophy:
   - **Packages**: append; on **name collision**, _later layer wins_ (user overrides repo)
     — mirrors mise/git last-wins. Collisions are **logged**, never silent (fail-fast
     ethos, `load.go:52–63`).
   - **Intents**: deep-merge — a user file adding to intent `dev` **extends** its package
     list rather than replacing it (so a user can bolt tools onto `dev` without redefining
     it). A user may also define brand-new intents.
   - **Symlinks / modules**: append; collision by `Dest` (symlink) or `name` (module) →
     later wins, logged.
   - Run `Validate` on the **merged** result so an overlay can't smuggle in an invalid
     package.

2. **Extend `Resolve` into a precedence _stack_, not a switch.** Keep the fail-fast
   contract for explicit overrides; add the overlay as an additive top layer:

   ```
   base   = --manifest | $DOTF_MANIFEST | embedded     (existing precedence, unchanged)
   overlay = merge( glob(~/.config/dotf/packages.d/*.toml, modules.d/*.toml) )
   final  = Merge(base, overlay)
   ```

   `--manifest` still _replaces the base_ (explicit, fail-fast); the XDG overlay _adds on
   top_ of whatever base won. A `--no-user-config` flag (and/or `DOTF_NO_USER_CONFIG`)
   gives CI a way to pin to the pristine embedded manifest — important for reproducible
   provisioning and for the report's determinism concerns (C7).

3. **Discovery = XDG + drop-in dir.** Resolve the root as
   `${XDG_CONFIG_HOME:-~/.config}/dotf`. Prefer a `packages.d/*.toml` directory (many small
   fragments, trivially added/removed) over a single file; support a top-level
   `packages.toml` too for the simple case. Missing dir = **no overlay**, not an error
   (unlike an explicit `--manifest`, an _absent_ user layer is the normal default).

4. **A `config.toml` for dotf's own settings** (separate from the manifest overlay, per
   chezmoi): default intents, default `--backup`/`--headless`, a user `repo_root`, etc.
   This is where the scattered env vars get a typed home. Small, optional, additive.

5. **User-local script root for `custom-script`/modules.** Today the backend resolves
   scripts **repo-relative** and _rejects_ absolute paths and `..` escapes
   (`custom_script.go:57–88`) — a deliberate security boundary. A user overlay that
   references its own scripts needs a **second, explicitly-configured root**
   (`~/.config/dotf/bin/`), resolved by the same clean-and-contain logic. This keeps the
   "no arbitrary absolute paths" invariant while letting user scripts live outside the repo.

### B.5 Contradictions & tensions (preserved, not smoothed)

| #      | Tension                                                                                                   | Where the evidence leans                                                                                                                                                                                                                                                                                                                                     |
| ------ | --------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **B1** | User overlay **vs** reproducibility / supply-chain trust (report Theme 5, C7)                             | **Split trust like the report split lock-by-class.** The embedded manifest is _curated_; the user overlay is _"your machine, your risk."_ Mitigations: `--no-user-config` for CI/pristine runs; the same `probe`/lockfile discipline applies to user entries; a `dotf status --backends` line can flag overlay-sourced `custom-script`/`script-url` entries. |
| **B2** | `DisallowUnknownFields` strictness (`load.go:24`) **vs** users wanting forward-compatible/loose fragments | Keep strict — it's the fail-fast contract. The overlay conforms to the _same_ schema; a typo in a user fragment _should_ error loudly, not be silently dropped.                                                                                                                                                                                              |
| **B3** | Override-by-name (user wins) **vs** additive-only purity                                                  | **Additive with last-wins override is still additive** in spirit: nothing is _pruned_; a user can only _add_ or _shadow_ an entry, never delete the base's management of something else. Log every override so it's never surprising.                                                                                                                        |
| **B4** | Overlay merge **vs** the existing "explicit override fails fast" philosophy (`load.go:52–63`)             | Preserve both by _role_: explicit `--manifest`/`$DOTF_MANIFEST` = replace + fail-fast (a demand for one file); XDG overlay = additive + tolerant-of-absent (a standing extension point). Different intent, different rule.                                                                                                                                   |

### B.6 Effort / value / risk

- **`manifest.Merge` + validate-the-merged-result** —
  _Effort: med. Value: high (the whole feature rests on it). Risk: med — the merge
  semantics (intent deep-merge, collision policy) are the real design work; prototype
  against 2–3 sample overlays._
- **`Resolve` → precedence stack + XDG discovery + `--no-user-config`** —
  _Effort: low-med. Value: high (unlocks user extension with no repo edit). Risk: low
  (localized to one function + one glob)._
- **`config.toml` settings file** —
  _Effort: low. Value: med (tidies scattered env vars; nice-to-have). Risk: low._
- **User-local script root** —
  _Effort: low-med. Value: med (needed only for user `custom-script`/modules). Risk:
  med — must preserve the clean/contain security invariant._

---

## Integrated sequencing (how this slots into the report's Phase 0–4)

Both parts **hang off the same keystone** as the rest of the report (probe / `Installed()`
gate, Phase 1). They do not reorder the existing plan; they extend two phases and add
governance to Phase 4.

- **Phase 0 (docs, zero code):** record the two extensions in scope (this addendum);
  reframe `custom-script` as also the _lifecycle adapter_ (it forwards `--update`/
  `--uninstall`), and note the `setup`/module category as the permanent home for imperative
  subsystem config.
- **Phase 1 (+ lifecycle groundwork):** with the probe landing anyway, add the `Lifecycle`
  action type and the positional named target on `install` (`dotf install <name>`), and the
  `actions = [...]` package field. Ship `dotf install claude --update` first — it is the
  lowest-risk, highest-delight slice (reactivates existing, tested script code).
- **Phase 2/3 (+ overlay & lifecycle spread):** `manifest.Merge` + XDG overlay
  (`~/.config/dotf/packages.d/`) rides alongside the backend-capability work, since both
  touch the manifest layer. Extend `Updater`/`Remover` across native backends as each
  migration lands.
- **Phase 4 (+ verbs & modules):** the `setup <domain>` verb + typed `module` schema
  (seeded by `setup-fingerprint`/`onedrive`, with `ssh` gated on #34); `config.toml`;
  `--no-user-config`; a `dotf status --backends` line that flags overlay-sourced escape-hatch
  entries.

**One-line summary:** the two features are _the same architecture pointed two ways_ — a
single new `Lifecycle` primitive threaded through action-parametric backends gives every
tool `install/update/remove` **anytime**, and a single new `Merge` layer on `Resolve` lets
users **extend** the manifest from `~/.config/dotf` without forking. Both reuse the report's
keystone probe, both are stdlib-`flag`-clean, and both stay inside the additive, single-host,
governed-escape-hatch scope the parent report defends.

---

## New decision-blocking unknowns (resolve before building — each < 1 day)

1. **Which scripts' `--update`/`--uninstall` actually converge cleanly** (vs re-run the
   whole installer)? Audit the Tier-A `case "$ACTION"` bodies (claude/codex/opencode/
   fabric/cursor-agent) end-to-end before declaring `actions = ["update","remove"]`.
2. **Do native `Update`/`Remove` map onto every backend, or only some?** `pkg`/`toolchain`
   are clear; `github-release` "update" = re-resolve latest (needs the report's auto-detect,
   Phase 2); `apt-repo`/`aur` removal semantics need a spike. Enumerate the matrix.
3. **Verb shape** (A vs B, §A.4) — decide the UX once the action-parametric engine exists;
   it's reversible and cheap, so don't block the engine on it.
4. **Merge collision policy** — is _user-wins_ right for every kind (packages, intents,
   symlinks, modules), or should symlink `Dest` collisions with the base **error** (a user
   silently shadowing a repo dotfile is more surprising than shadowing a package)? Prototype.
5. **User-script trust boundary** — confirm the second script root (`~/.config/dotf/bin/`)
   can reuse `custom_script.go`'s clean/contain logic without weakening the no-absolute-path,
   no-`..` invariant. Spike against a sample user script.
6. **`setup`/module step vocabulary** — the minimal typed set (`symlink`, `sysconfig`,
   `firewall`, `custom-script`, `pkg`) that covers fingerprint/onedrive/git/shell today and
   ssh when #34 lands. Derive it from the two existing `bin/*setup*.sh` scripts, don't invent.

```

```
