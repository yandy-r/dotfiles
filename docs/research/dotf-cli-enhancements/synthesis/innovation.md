# Innovation — Emergent Feature Designs for `dotf`

_Innovation Agent (Phase 3, emergent-insight). Asymmetric Research Squad._
_Subject: enhancing the `dotf` CLI + retiring `bin/install-*.sh` into native TOML
backends. Date: 2026-07-04._

_Inputs: all 8 persona findings, `crucible-analysis.md`, `contradiction-mapping.md`,
`evidence/verification-log.md`, and direct reads of `internal/manifest/{types,validate}.go`,
`internal/backend/{backend,github_release,custom_script,pkg,download,register}.go`,
`internal/platform/types.go`, `cmd/dotf/{install,status,applyphases}.go`._

---

## Executive Summary

The squad produced an unusually strong consensus: the migration is blocked on
**verification and backend capability**, not effort; `custom-script` should be _triaged_,
not eliminated; and one field — variously named `probe`/`Binary`/`Provides`/`creates:` by
six personas — is the hinge of the whole roadmap. The personas each nominated a _piece_;
none assembled the pieces into coherent, buildable schema. That assembly is this
document's job.

The central design move is to notice that **four separate persona proposals are one
data model wearing four hats**, and that a fifth insight — the crucible's "dotf has two
tool classes" — is not a framing but a _missing schema field_ that collapses three open
tensions (lockfile, update, determinism) into a single declaration. Recombining these
yields seven concrete designs that fit dotf's hard constraints (Go stdlib `flag`, TOML
manifest, single-host/additive scope, solo maintainer) and — critically — **make each
other cheaper**: the state-probe verifies the auto-detect backend; the `class` field
tells the probe when to skip vs. let-float; the lockfile turns auto-detect deterministic;
`--only`+`--json` turn the probe into a scriptable migration test-harness; `doctor` is a
read-only aggregate over the probe; and governance-as-code freezes the gradient the probe
made legible.

The buildable core is **five schema additions and one engine gate**:
`provides []string`, `installed_when {path|command|version_matches}`, `class`,
`post_install [typed steps]`, plus a `github-binary` backend and a `git-clone` backend,
all read by one shared `Installed()` resolver wired into `installPackage`. Everything
else (`doctor`, `self-update`, `--json`, `--only`, the CI ratchet, `dotf lock`,
`dotf add`) is thin surface over that core.

**One correction to the ground truth, verified in code this pass:** the verification
log's F3 states `info.OS ∈ {linux, macos}`. The code (`platform/types.go:18-28`) shows
`OS` is a **distro-family** enum (`macos`, `debian`, `redhat`, `arch`, `suse`, `linux`,
`wsl`). So `templateGitHubAsset` renders `debian`/`arch`/`redhat`/`macos` on recognized
hosts — it can spell **`linux` only on an _unrecognized_ distro, and `darwin` never**.
The OS-vocabulary gap is therefore _worse_ than the synthesis concluded, and Design 2
must carry an explicit family→release-OS mapper, not merely a case helper. (Details in
Unexpected Insights §1.)

---

## Novel Feature Designs

### Design 1 — The Keystone: a state-probe ladder (`provides` + `installed_when`) wired into the apply loop

**Combines.** Systems-thinker's `probe`/`check` field + apply-loop gate (leverage #1);
Negative-space's `Package.Binary`/`Provides` ("gates _verification_"); Archaeologist's
Ansible-`creates:` sentinel + "spread the `pkg` authority-query"; Contrarian's "fix
`Installed()`, it's a _bug_"; Analogist's `creates`/`Binary` guard (S8); Historian's
"explicit `binary`/`check` field, asdf/aqua declare provided binaries." Four personas
named the same field; this design unifies them into **one ordered resolver** rather than
four competing fields.

**Sketch — TOML.** A small predicate group on `Package`, all optional:

```toml
# multi-binary bundle: all-present ⇒ installed (answers the crucible's open Q4)
[[package]]
name = "hashicorp-tools"
backend = "custom-script"
script = "bin/install-hashicorp-tools.sh"
provides = ["terraform", "vault", "packer"]

# no binary at all: a path sentinel (Ansible creates:)
[[package]]
name = "tpm"
backend = "git-clone"
installed_when = { path = "~/.tmux/plugins/tpm/.git" }

# rolling manager: a capability probe with a version floor
[[package]]
name = "rust"
backend = "custom-script"
script = "bin/install-rust.sh"
installed_when = { command = "rustc --version", version_matches = "^rustc 1\\." }
```

**Sketch — Go.** Add to `manifest.Package`:

```go
Provides      []string       `toml:"provides"`       // all-present ⇒ installed
InstalledWhen *StatePredicate `toml:"installed_when"`

type StatePredicate struct {
    Path           string `toml:"path"`            // os.Stat (~ expanded)
    Command        string `toml:"command"`         // argv[0] LookPath'd, run via Runner.Output — NO shell
    VersionMatches string `toml:"version_matches"` // optional regexp over stdout
}
```

Resolve **at the `Engine` level**, not per-backend — this is the elegant insertion
point the code reveals. `Engine.Installed` consults a declared predicate first and falls
back to the backend's own default only when none is declared:

```go
func (e *Engine) Installed(pkg manifest.Package, info platform.Info) (bool, error) {
    if pkg.InstalledWhen != nil || len(pkg.Provides) > 0 {
        return evalPredicate(e.r, pkg, info) // shared ladder, one place
    }
    return e.backends[pkg.Backend].Installed(pkg, info) // pkg → authority-query; others → LookPath
}
```

`evalPredicate` runs the ladder: `Provides` (all `LookPath` succeed) → `path` (`os.Stat`)
→ `command`+`version_matches` (`Runner.Output`, argv-only, optional regexp). The
`pkg`-backend authority-query (`pkg.go:109-142`) already exists; leaving unchanged
backends to fall through to it is the "diffusion, not invention" the Archaeologist asked
for.

Then wire the read path into the write path — the single missing edge
(`install.go:93-112`, `applyphases.go:135-143`):

```go
func installPackage(engine *backend.Engine, pkg manifest.Package, info platform.Info, force bool) installResult {
    // ... Applicable check unchanged ...
    if !force {
        if ok, _ := engine.Installed(pkg, info); ok {
            return installResult{name: pkg.Name, backend: pkg.Backend, status: "already-installed"}
        }
    }
    // ... Install unchanged ...
}
```

`--force` (already parsed in `applyopts.go`) bypasses the gate = "ignore probe, re-run"
(Systems' unintended-consequence guard). A new `"already-installed"` status joins
`ok`/`not-applicable`/`failed`.

**Benefit (testable).** On a fully-provisioned Arch host: `dotf status` reports
`installed=true` for all 8 tools that lie today (`aws-tools`, `azure-cli`,
`hashicorp-tools`, `container-tools`, `rust`, `tpm`, `tmux-sessionizer`,
`gnu-utilities`); a second `dotf apply` records `already-installed` for every converged
`pinned` package and issues **zero** Install calls for them — assertable directly with
`exec.Fake` (no argv emitted). This is the crucible's confirmed shipping bug (F1) fixed
and the precondition (F2) for `doctor`/`update`/lockfile met in one change.

**Feasibility: High.** One field group + one ~40-line resolver + one gate + the
regression test Negative-space flags as missing. No new backend, no framework.

**Scope-creep risk: Low-Med.** The single risk is `installed_when.command` sliding into
custom-script v2. The fence is mechanical: `command` is split to argv, `argv[0]` is
`LookPath`'d, executed via `Runner.Output` — **no shell, no pipes, no `&&`**;
`version_matches` is a Go `regexp`. You cannot smuggle logic through it.

---

### Design 2 — A `github-binary` auto-detecting backend, with a "detect once, pin the result" lifecycle

**Combines.** Analogist's ubi/eget keystone (S1, "the single highest-leverage fix");
Futurist's "auto-resolution won _only married to verification_" (mise `ubi`→`github`
deprecation); Archaeologist's per-script capability table (bare binary, `.gz`,
`x86_64`/`aarch64`, `Linux`/`Darwin`, non-GitHub host); Journalist's `{{.OS}}`/`{{.Arch}}`

- version-pin gap; the crucible's decisive reconciliation of C6/PT2, **"detect at
  add-time, pin at apply-time."** No persona specified the lockfile _shape_ or the resolve
  lifecycle; this does.

**Sketch — TOML (the minimum a tool needs).** Owner/repo only; everything else
auto-resolved and frozen into `dotf.lock`:

```toml
[[package]]
name = "lefthook"
backend = "github-binary"
repo = "evilmartians/lefthook"
class = "pinned"                # Design 3

# with overrides when auto-detect needs a hint:
[[package]]
name = "rg"
backend = "github-binary"
repo = "BurntSushi/ripgrep"
exe = "rg"                      # repo name ≠ binary name (ubi's exe field)
asset_prefer = ["musl"]         # libc / disambiguation tokens (eget-style)
```

**Sketch — the lockfile (`manifest/dotf.lock`, committed).** Per-(name × os × arch)
resolved artifact + checksum — this is the `go.sum` to `packages.toml`'s `go.mod`
(Analogist E1), and it is what makes a _heuristic_ reproducible:

```toml
[[locked]]
name = "lefthook"
tag  = "v1.7.18"

  [locked.assets.linux-amd64]
  asset  = "lefthook_1.7.18_Linux_x86_64.gz"
  sha256 = "…"

  [locked.assets.darwin-arm64]
  asset  = "lefthook_1.7.18_Darwin_arm64.gz"
  sha256 = "…"
```

**Sketch — Go.**

- New `github_binary.go` backend. `Install` reads `dotf.lock` for the current
  os/arch, fetches the pinned asset, verifies sha256, extracts/copies. **Apply never
  auto-resolves** — it only consumes the lock (deterministic). If the lock lacks this
  os/arch, it errors with "run `dotf lock`" (fail-fast, not silent float).
- The ubi filter pipeline (asset enumeration via the GitHub API list — the logic already
  lives in `bootstrap.sh:234-244`, port it) runs **only** in a new `dotf lock` command:
  extension filter → OS-token filter → arch-token filter → libc → archive-vs-bare →
  deterministic tie-break → introspect archive for `exe`. It writes/updates `dotf.lock`.
- Vocabulary expansion (shared with Design 1's needs):
  `platform.OS.ReleaseOS()` maps family→`{linux, darwin}` **and** exposes case + token
  variants; `Arch` gains alias sets (`{amd64, x86_64}`, `{arm64, aarch64}`) so the matcher
  accepts either. This is the real fix the F3 correction demands (Unexpected Insights §1).
- Extend `download.go` `archiveFormat` with `"gz"` (single-member gunzip → the inner
  binary) and `"raw"` (bare binary, chmod +x) — retires lefthook (`.gz`) and
  hadolint/ecs-cli/skaffold (bare) that `.tar.gz`/`.zip`-only handling blocks today.

**Benefit (testable).** Adding a release-binary tool drops from _author an `asset`
template + pin a `tag` + hand-compute a `sha256`_ (more effort than a `custom-script`
one-liner — the Analogist's "inverted gradient") to **3 TOML lines + `dotf lock`**. This
inverts the activation-energy gradient so native becomes the path of least resistance —
the mechanism the Analogist predicts makes "the migration largely happen by itself."
Measurable: lines/keystrokes to onboard `lefthook` falls from a ~200-line script to 3
lines + one command; `dotf lock` output is byte-reproducible across runs.

**Feasibility: Med.** Filter pipeline is genuine work; the GitHub-API asset listing is
already written in bash and portable; the lockfile + `dotf lock` are the new surface.
Cap v1 at linux/darwin × amd64/arm64 + gz/raw/tar.gz/zip; defer musl/`.exe`/multi-artifact
tails (the code-spike the crucible flags as the within-phase-order discriminator).

**Scope-creep risk: Med.** ubi's own "detection is not part of the API" caveat is the
trap. The fence: **auto-resolution is confined to `dotf lock` (add-time); `apply` only
ever reads the pinned lock.** Never resolve at apply time — that would reintroduce the
non-determinism the whole design exists to remove.

---

### Design 3 — First-class two tool classes: `class = "pinned" | "rolling" | "managed"`

**Combines.** The crucible's PT3 ("two tool classes, two contracts") and its observation
that this _one_ insight resolves _three_ tensions (C3 lockfile, C4 update, C7
determinism); Systems-thinker's "determinism doesn't emerge, it must be designed in" +
rolling tools self-update; Contrarian's "pin only immutable artifacts"; Historian's
Homebrew "no lockfile, rolling release" _decision_; Analogist's homeostatic "tolerance
band." **Every persona treated pinned-vs-rolling as a _framing_. None proposed it as a
schema field.** That is the emergent move: turn the framing into a primitive that drives
four subsystems at once.

**Sketch — TOML.**

```toml
[[package]]
name = "lefthook"
backend = "github-binary"
class = "pinned"     # immutable; lockfile-tracked; apply skips when Installed(); doctor flags stale pin

[[package]]
name = "mise"
backend = "custom-script"
script = "bin/install-mise.sh"
class = "rolling"    # floats/self-updates; NOT lock-tracked; apply always runs (self-update); never "drift"
```

**Sketch — Go.** One field, `class string` (default _inferred_ from backend and
overridable). The default table encodes the triage the whole squad converged on:

| Backend                                         | Default class    | Apply behavior                                 | Lock?         | `status`/`doctor`        |
| ----------------------------------------------- | ---------------- | ---------------------------------------------- | ------------- | ------------------------ |
| `github-binary`, `github-release`, `script-url` | `pinned`         | skip if `Installed()`                          | yes           | flag stale pin / missing |
| `pkg`, `aur`, `apt-repo`                        | `managed`        | authority-query gate; manager owns updates     | no            | authority truth          |
| `toolchain`                                     | `managed`        | skip if `Installed()`                          | no            | binary truth             |
| `custom-script`, `git-clone`                    | **must declare** | `pinned`⇒skip-if-present; `rolling`⇒always run | `pinned` only | `rolling` never "drift"  |

The gate in `installPackage` reads `class`: `pinned`/`managed` consult `Installed()`
(Design 1) and skip; `rolling` always runs Install (its self-update _is_ the point) and
is excluded from `doctor`/`status` drift. This de-overloads `custom-script`: `tpm` is a
`pinned` setup-step (probe by path, never re-run); `mise` is `rolling` (re-run every
apply, by design).

**Benefit (testable).** After tagging: `dotf status` stops reporting rolling managers as
`drift=declared-not-installed` (they float by design) _and_ stops reporting converged
pinned tools as missing; `dotf apply` on a converged host does network I/O **only** for
the handful of `rolling` tools — turning "re-downloads everything every run" (F2) into
"re-runs exactly the tools that asked to float." The reproducibility narrative (C7)
becomes _honest by classification alone_, even before any lockfile ships.

**Feasibility: High.** A field + a default-inference table + branch in the gate. The only
subtlety is the inference defaults; make `custom-script`/`git-clone` require an explicit
`class` (validation error otherwise) so the triage decision is forced, never accidental.

**Scope-creep risk: Low.** Risk is class-proliferation; cap hard at three. `class` is the
"split the population" insight as one word — it is _less_ surface than the three separate
features (lockfile scoping, update scoping, drift scoping) it subsumes.

---

### Design 4 — `git-clone` backend + a _typed_ `post_install` step list (and the exact fence)

**Combines.** Archaeologist's "not everything is a package" (git-clone, `post-install`
vocab, revival #3/#6); Analogist's `post_install` bounded hook (S7, Homebrew
`post_install`, Bazel `run_binary` middle rung); Negative-space's missing
git-clone/cargo-git/completion needs; Historian's honest `setup-step` tier; the
tea-completions / atuin-import / tmux-source cases. The objective's explicit ask —
_"where to draw the line so `post_install` doesn't become custom-script v2"_ — is answered
here by construction.

**Sketch — TOML.** `git-clone` as its own backend:

```toml
[[package]]
name = "tpm"
backend = "git-clone"
repo = "https://github.com/tmux-plugins/tpm"
dest = "~/.tmux/plugins/tpm"
ref  = "v3.1.0"        # optional pin; else default branch (class="pinned" ⇒ ref recommended)
depth = 1
class = "pinned"
installed_when = { path = "~/.tmux/plugins/tpm/.git" }
```

`post_install` as a **closed, typed** step list on any package:

```toml
[[package]]
name = "tea"
backend = "github-binary"
repo = "gitea/tea"
post_install = [
  { completions = { shells = ["bash", "zsh", "fish"] } },
]

[[package]]
name = "atuin"
backend = "script-url"
url = "https://…"
sha256 = "…"
class = "rolling"
post_install = [
  { run = { exe = "atuin", args = ["import", "auto"] } },
]
```

**Sketch — Go.** `git_clone.go`: `git clone --depth <n> --branch <ref> <repo> <dest>`,
idempotent when `dest/.git` exists (~30 LOC). A `post_install` executor invoked by the
engine _after_ a successful `Install`, dispatching a tagged-union step type:

```go
type PostInstallStep struct {   // exactly one field non-nil
    Symlink     *SymlinkStep     `toml:"symlink"`
    PathExport  *PathExportStep  `toml:"path_export"`
    Completions *CompletionsStep `toml:"completions"`
    Run         *RunStep         `toml:"run"`   // {exe, args} — argv only
}
```

**The fence (this is the whole design).** `post_install` is a **fixed enumerated verb
set** the engine implements in Go — `symlink`, `path_export`, `completions`, and a single
`run { exe, args }`. There is deliberately **no `script` verb**. The `run` step is
**argv, not a shell string** — `exe` is `LookPath`'d and executed via `Runner.Run(exe,
args...)`, so you _cannot_ express a pipe, a `&&`, a redirect, a loop, or a conditional.
The moment a tool needs any of those (mise's 4-way cascade, lua's source build), it stays
`custom-script`. That bright line — _typed declarative steps vs. arbitrary bash_ — is
what stops `post_install` from becoming custom-script v2, and it is enforced by the schema
itself, not by discipline.

**Benefit (testable).** `tpm` (git-clone), `tea` (completions), `atuin`
(import via typed `run`) leave `custom-script`; a schema-level test proves no
`post_install` step can carry a shell string (there is no field for one). The
setup-step/mechanical split becomes real: the migratable bucket shrinks, the permanent
bucket is named (Design 6).

**Feasibility: Med.** `git-clone` is trivial; the `post_install` executor + four typed
steps is the bulk. `completions` shells out to `<tool> completion <shell>` (already the
convention dotf's own `completion` command uses).

**Scope-creep risk: Med-High — the highest of any design here.** This is the one that
_could_ rot into custom-script v2. Guardrails: no `script`/`shell` verb, `run` is
argv-only, the verb set is closed and grows only by explicit maintainer decision + a test.
If a proposed tool needs a fifth verb that's really "run arbitrary bash," refuse it and
keep the tool in `custom-script` — that refusal _is_ the design working.

---

### Design 5 — `dotf doctor` (read-only, probe-backed) + `dotf self-update` (shim-checksum-backed)

**Combines.** Journalist's "`doctor` is table stakes; `self-update` is a solved 1-day
feature via go-selfupdate + the GoReleaser checksums the shim already verifies";
Negative-space's `doctor` as a _migration enabler_ (live-pin check would have caught
starship) + PATH/toolchain sanity; Analogist's read-only AIS drift report (S10, no
prune); Futurist's `doctor --json` + optional cosign verify; Systems-thinker's decisive
sequencing rule — **`doctor` is only as truthful as `Installed()`, so it ships _after_
Design 1, never before** (a lying diagnostic burns trust); Contrarian's "self-update is
cheap but don't over-rank it."

**Sketch — commands (no TOML).**

`dotf doctor [--json]` — a read-only aggregator of typed checks, each `ok|warn|fail` +
a remediation hint. It **reports, never repairs** (respects additive-only; `--repair` is
explicitly out of scope):

| Check        | What it verifies                                                           | Reuses               |
| ------------ | -------------------------------------------------------------------------- | -------------------- |
| `manifest`   | parses + `manifest.Validate` passes                                        | existing validator   |
| `drift`      | Design-1 `Installed()` over all packages (honest now)                      | Design 1             |
| `stale-pin`  | range/HEAD-fetch each `pinned` asset, compare sha256 to `dotf.lock`        | Design 2 lock        |
| `path`       | `~/.local/bin` on PATH; cargo/go on PATH for toolchain pkgs                | —                    |
| `shell-hook` | dotf-managed fragments actually sourced in live rc (starship/atuin/direnv) | —                    |
| `sudo`       | `sudo.Prime` reachable for the privileged phases                           | `internal/privilege` |
| `snapper`    | snapper present (Linux/btrfs)                                              | `internal/snapper`   |

`dotf self-update [--dry-run]` — `creativeprojects/go-selfupdate`: `DetectLatest`
(semver git tag) → download the per-os/arch asset → `ChecksumValidator{UniqueFilename:
"checksums.txt"}` (the **exact** artifact `bootstrap.sh:283-294` already trusts) → atomic
replace. Same trust anchor as the shim — this _de-duplicates_ the shim's verify logic into
the binary (Systems' "net simplification if done once"), rather than inventing a second
signature story. cosign verify is an optional later add (Futurist tier-2), not a
prerequisite.

**Benefit (testable).** `dotf doctor` on the _current_ repo flags the stale starship pin
(Contrarian's live finding, F4) as `fail: stale-pin` **before** a fresh `apply` silently
leaves starship uninstalled — converting a 2am default-path breakage into a preflight
warning. `dotf self-update --dry-run` prints `current=v0.2.0 latest=v0.3.0` with zero host
mutation.

**Feasibility.** `doctor` Med (aggregator; each check small and reuses existing code);
`self-update` Low (one GoReleaser-native library). Both gain `--json` free from Design 7.

**Scope-creep risk: Low** for self-update (bounded, cheap, de-dupes existing logic);
**Med** for doctor (checks proliferate) — cap the check set, and hold the "reports, never
repairs" line so it never drifts toward the reconciliation the PRD defers.

---

### Design 6 — Governance-as-code: a monotonic CI ratchet + a `dotf status --backends` count line

**Combines.** Analogist's ansible-lint-style "block _new_ custom-script" ratchet (S4) +
status count (C2); Historian's "freeze new entries — the single highest-leverage
anti-stall move"; Systems-thinker's CI gate (B2) **and** his flagged failure mode ("a
count can incentivize folding three tools into one script to game it"); the squad-wide
metric reframe ("custom-script ≤ ~5, delete-on-cutover," not "≥80% LOC"). The novel part
is the **anti-gaming guard** no single persona fully specified plus the coupling of the
count to Design 3's `class`.

**Sketch.** Two coupled mechanisms:

1. **A legibility line.** `dotf status --backends` emits:

   ```
   backend.custom-script=migratable:6 permanent:9 target:0-migratable
   ```

   distinguishing the `class="rolling"`/`permanent=true` tier (blessed forever) from the
   _migratable_ remainder — so the number that matters is visible and separate from the
   number that shouldn't move (Systems #2: makes the 19-count legible as ~10 migratable +
   ~9 permanent, dissolving the "migration is failing" framing).

2. **A monotonic CI ratchet** — a `go test` over the embedded manifest + a committed
   baseline:
   ```
   # .dotf-ratchet
   migratable_custom_scripts = 6   # only ever decreases; CI fails if actual > this
   ```
   A migratable custom-script is one _not_ marked permanent. Permanence must be declared
   explicitly and reviewably:
   ```toml
   [[package]]
   name = "mise"
   backend = "custom-script"
   class = "rolling"
   permanent = true
   reason = "4-way install cascade (brew→AUR→vendor) + self-update; a native port reimplements it worse"
   ```

**The anti-gaming guard (Systems' flagged second-order failure).** The ratchet counts
**distinct `bin/install-*.sh` files referenced** (weighted), so folding three tools into
one script does _not_ lower the count. And `permanent=true` requires a one-line `reason`
(validated non-empty), so "reframe as permanent" can never silently absorb a _migratable_
tool — the permanent allowlist is explicit, reviewable, and diffs in PRs.

**Benefit (testable).** A PR adding a new `custom-script` without `permanent=true` fails
CI unless it retires an equal count elsewhere — the "boiling frog" gradient (Analogist
D2) is arrested. The status line makes the 19→(migratable + permanent) split legible,
which is the reframe the whole squad demanded as Phase 0.

**Feasibility: High.** A `go test` that parses the embedded manifest against a baseline
file; a few lines in `status`. Zero runtime cost.

**Scope-creep risk: Low.** It is process + reporting. The substance is the two guards
(count-scripts-not-entries; require-explicit-reason) that stop the governance lever from
being gamed — the productive-tension guard the contradiction map (Pattern 6) insists every
governance mechanism needs.

---

### Design 7 — The migration test-harness bundle: `--only`/`--skip`, `--json`, `dotf add`

**Combines.** Negative-space's "`--only` is _blocking_ — you cannot vertical-slice
without it" + "`--json` is the most out-of-character gap for an agent-driven repo";
Journalist's cheap-hygiene wins (`--only`/`--skip`/`--json`/`NO_COLOR`, all stdlib-flag);
Futurist's "`--json` is the best AI bet — CLI is the new MCP"; Analogist's topgrade
`--only` (S9); Historian's vertical-slice, delete-on-cutover loop. The reframe: these are
**not UX polish — they are the migration test-harness**, and they only pay off _coupled_
to Design 1's probe.

**Sketch — Go.**

- `--only <pkg>` / `--skip <pkg>` (comma-split, repeatable) on `install`/`apply`, filtering
  the resolved package set _after_ intent resolution; a new `"skipped-by-user"` status
  distinct from `"not-applicable"` (Systems' exit-code-vocabulary note).
- `--json` on `status`/`diff`/`plan`/`install`/`doctor` via a shared `output.Mode`
  helper + `encoding/json`; auto-disable color; **stdout = data, stderr = progress**
  (audit the current `key=value` printers for stream leakage).
- `NO_COLOR` env respect (`os.Getenv` + isatty).
- `dotf add <name> --backend github-binary --repo owner/repo` scaffolds a `[[package]]`
  block (and, for `github-binary`, runs `dotf lock`); it is the _proactive_ home of the
  Design-6 freeze policy — it **refuses** to scaffold a `custom-script` without
  `--permanent --reason "…"`.

**Benefit (testable).** The vertical-slice migration loop becomes one scriptable line:

```
dotf install --only lefthook --json | jq -e '.results[0].status | test("ok|already-installed")'
```

— port → prove with `--only` → confirm via Design-1 probe → delete the `bin/install-*.sh`.
Historian's "one tool per PR, delete on cutover" loop is _physically impossible today_
(no single-package targeting) and becomes trivial. `dotf status --json | jq` gives agents
the machine-readable surface the repo's own CLAUDE.md ecosystem needs.

**Feasibility: High.** All stdlib. `--json` is the broadest touch (each printer) but
purely mechanical; `dotf add` is the only piece with judgment (templates) — keep it thin.

**Scope-creep risk: Low.** `dotf add` is the only surface with taste; cap it at the
common backends and let the freeze policy be its one opinion.

---

## Innovative Approaches

Cross-cutting architectural moves that make the designs cohere — the "recombination"
value that no single persona holds:

1. **Resolve state at the `Engine`, not the backend.** The personas said "fix
   `Installed()`" (implying four backend edits). The code (`backend.go:56-62`,
   `install.go:104`) reveals the cheaper, more elegant move: consult a _declared_
   predicate in `Engine.Installed` and fall through to the backend default only when none
   is declared. One insertion point verifies every backend; unchanged backends inherit
   the `pkg`-authority-query for free. This is the difference between a 4-file patch and a
   1-function patch.

2. **`dotf.lock` is the seam that dissolves auto-detect-vs-pin.** The sharpest
   reconciliation in the corpus ("detect once, pin the result") is not a slogan but a
   _lifecycle boundary in code_: auto-resolution lives **only** in `dotf lock` (add-time);
   `apply` **only** consumes the pinned lock (apply-time). The heuristic's
   non-determinism (ubi's own caveat) is quarantined to a command you run deliberately and
   commit the result of.

3. **`class` is a leverage point disguised as a field.** One TOML word drives four
   subsystems (apply-gate, lockfile-scope, update-scope, drift-scope). It is the "split
   the population" insight compiled into the schema — and it is _less_ surface than the
   three features it subsumes, the rare case where the right abstraction shrinks the
   codebase.

4. **The fence against custom-script v2 is typed steps, not discipline.** `post_install`
   resists rot because the schema has _no field_ for a shell string — `run` is argv-only.
   The guarantee is structural, checkable by a unit test, not dependent on the maintainer
   resisting temptation.

5. **Governance carries its own anti-gaming guard.** Counting scripts-referenced (not
   entries) and requiring an explicit non-empty `reason` for permanence are the guards
   against the two failure modes Systems and the contradiction-map flagged: folding-to-game
   and reframe-as-excuse. The lever is designed to resist its own misuse.

6. **The "boring UX" bundle is really the migration test-harness.** `--only` + `--json` +
   the Design-1 probe compose into a one-line, machine-verifiable "did this port land?"
   check — which is exactly the vertical-slice loop the Historian's playbook requires and
   the CLI physically cannot perform today.

---

## Unexpected Insights

1. **The ground truth is itself slightly wrong about `{{.OS}}` — and the real gap is
   worse.** Verification-log F3 asserts `info.OS ∈ {linux, macos}`. The code
   (`platform/types.go:18-28`) defines `OS` as a **distro-family** enum
   (`macos`/`debian`/`redhat`/`arch`/`suse`/`linux`/`wsl`), and `templateGitHubAsset`
   substitutes `string(info.OS)` verbatim (`github_release.go:96-102`). So on a recognized
   host `{{.OS}}` renders `debian`/`arch`/`redhat`/`macos` — it can spell **`linux` only
   on an _unrecognized_ distro, and `darwin` never.** The Archaeologist's "distro-family"
   read (self-flagged Medium, "worth a maintainer confirming") was _correct_; the ground
   truth over-corrected it. Consequence: `github-release`'s OS token is unusable for
   essentially every asset that carries `linux`/`darwin` (i.e. most Go-tool releases), so
   Design 2 must ship a `ReleaseOS()` family→`{linux,darwin}` mapper, not merely a case
   helper. This is a concrete, code-verifiable correction that changes the capability work.

2. **The cheapest fix for the biggest bug is at the call site, not the four
   implementations.** Every persona framed F1 as "`Installed()` is wrong in four
   backends." The engine's dispatch shape means the honest fix is _one_ gate in
   `Engine.Installed` + _one_ gate in `installPackage` — the four backend implementations
   need not change at all (they become the fallback). The most-cited bug in the corpus has
   a one-function fix.

3. **The `class` field makes the reproducibility narrative honest with zero new
   machinery.** The determinism critique (C7) is real _only because the narrative is
   applied uniformly_. Tagging `mise` `rolling` and `lefthook` `pinned` makes `status`
   stop lying about both **before any lockfile exists** — the honesty is a property of the
   classification, not of the lock. The lock is for _reproducing_; the class is for
   _telling the truth_.

4. **`dotf add` is where the freeze policy actually bites; the CI ratchet is only the
   backstop.** A ratchet is _reactive_ (fails a PR after the wrong entry is written);
   refusing to _scaffold_ a `custom-script` without `--permanent --reason` is _proactive_.
   Negative-space listed `add` and the freeze policy as separate gaps; coupling them turns
   scaffolding into the primary governance surface and the CI ratchet into belt-and-braces.

5. **`gnu-utilities` needs a bigger `pkg`, not a new backend.** Negative-space/Archaeologist
   reached for a "brew-bundle backend." The minimal fix is smaller: extend `pkg` to accept
   a space-joined `names.homebrew = "coreutils findutils gnu-sed …"` plus a _package-level_
   `os = "macos"` field (which only `Symlink` has today, `types.go:148-150`). Multi-name +
   per-package OS gating retires `gnu-utilities` without a new backend kind — a cheaper cut
   than the synthesis assumed.

6. **The `already-installed` status is a new vocabulary the exit-code contract needs.**
   Today `installPackage` emits `ok`/`not-applicable`/`failed`. Wiring the probe adds a
   fourth, semantically-distinct outcome (`already-installed`) that CI/agents will want to
   distinguish from a fresh `ok` — a small contract change that the `--json` surface
   (Design 7) should encode as a first-class field, not fold into `ok`.

---

## Key Insights

1. **The roadmap is a dependency chain with one root, and these designs are its links.**
   Design 1 (probe + gate) is the keystone six personas converge on; Design 3 (`class`)
   tells the probe _when to skip vs. let-float_; Design 2 (`github-binary` + lock) is the
   capability that inverts the migration gradient; Design 4 (git-clone + typed
   `post_install`) retires the setup-steps without a new escape hatch; Designs 5-7 are thin
   surface over that core. Build 1→3→7 first (all High feasibility), then 2→4, then 5-6.

2. **One data model, four persona names.** `provides` / `installed_when` / `creates:` /
   `Binary` are the same field. Unifying them into one ordered resolver — rather than
   shipping four — is the single highest-leverage recombination, and it is what makes
   every downstream feature (`doctor`, `update`, lockfile, `--only` verification) possible.

3. **`class` is the emergent primitive nobody proposed.** The crucible identified "two
   tool classes" as the insight that resolves three tensions; turning it into a schema
   field is the design leap. It is the rare abstraction that is _smaller_ than what it
   replaces.

4. **The fences are the features.** Three designs live or die by a bright line: Design 1's
   `command` is argv-not-shell; Design 4's `post_install` has no `script` verb; Design 6's
   ratchet counts scripts-not-entries and demands an explicit reason. Each fence is
   mechanically enforced (schema/type/test), not a matter of maintainer willpower — which
   is what keeps "shrink the escape hatch" from quietly becoming "grow a second one."

5. **Respect the triage; the goal is a _shape_, not zero.** Every design assumes the
   permanent `rolling`/setup-step tier survives forever (the squad's un-collapsed
   productive tension). The success metric these designs serve is "every _migratable_ tool
   is native, every residual is named + justified + idempotent" — convergent and
   completable, unlike "≥80% LOC."

---

### Digest — the designs most worth building, in priority order

1. **Design 1 — the state-probe ladder wired into the apply loop** (`provides` +
   `installed_when`, resolved at `Engine.Installed`, gated in `installPackage`). The
   keystone: fixes the confirmed `status`-lies bug (F1), makes `apply` idempotent (F2), and
   is the precondition to _verify_ any migration. **High feasibility, Low-Med risk. Build
   first.**

2. **Design 3 — `class = pinned|rolling|managed`.** One field that resolves lockfile +
   update + determinism at once and de-overloads `custom-script`; makes `status` honest
   _before_ any lockfile ships. **High feasibility, Low risk. Build with #1.**

3. **Design 7 — `--only`/`--skip` + `--json` + `dotf add`.** The migration test-harness:
   `dotf install --only lefthook --json` is the scriptable "did the port land?" check the
   vertical-slice loop requires; `--json` is the agent surface the repo's own tooling
   needs. **High feasibility, Low risk. Build with #1 (cheap co-enablers).**

4. **Design 2 — `github-binary` auto-detect backend + `dotf lock`** (family→`{linux,darwin}`
   OS mapper, `x86_64`/`aarch64` aliases, `.gz`/bare-binary assets, "detect once, pin the
   result"). Inverts the activation-energy gradient so native beats the escape hatch; the
   biggest single script-retirement lever. **Med feasibility, Med risk. Build second.**

5. **Design 4 — `git-clone` backend + typed `post_install`** (closed verb set, `run` is
   argv-only, no `script` verb). Retires the setup-steps (tpm, tea-completions,
   atuin-import) without reinventing custom-script. **Med feasibility, Med-High risk — the
   fence is the discipline. Build after #2.**

6. **Design 6 — governance-as-code** (monotonic CI ratchet counting scripts-referenced +
   explicit `permanent`/`reason`, plus a `dotf status --backends` legibility line). Arrests
   the escape-hatch growth gradient and makes the migratable/permanent split legible.
   **High feasibility, Low risk. Ship alongside #4.**

7. **Design 5 — `dotf doctor` (read-only, probe-backed) + `dotf self-update`
   (shim-checksum-backed).** `doctor` catches the stale-pin time-bomb preflight;
   `self-update` de-dupes the shim's verify logic. **`doctor` MUST follow #1** (a
   diagnostic built on the old probe would lie). **Med/Low feasibility, Low-Med risk. Build
   last.**

8. **Unexpected-insight rider — fix the OS vocabulary regardless.** The `{{.OS}}` token
   can spell neither `linux` (except on unknown distros) nor `darwin` today; ship the
   `ReleaseOS()` family→`{linux,darwin}` mapper (a Design-2 dependency) as a standalone
   correctness fix even if `github-binary` slips — it unblocks the existing
   `github-release` backend too.
