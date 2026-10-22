# Plan: `dotf unlink` / `dotf link --prune` (YAN-1306, GitHub #74)

Target release: next minor (v0.4.0). Base/PR target: `main` (RELEASING.md).

## Acceptance criteria (from YAN-1306)

1. `dotf link --prune` removes stale managed symlinks: symlinks one level deep in the parent
   dirs of every `DefaultTable` dest (all facets) whose target points into the dotfiles repo and
   either (a) is dangling, or (b) is not the link of any table entry. Parent dirs that resolve
   into the repo are not scanned; links into `plasma/files/` are never touched; real files and
   dirs are never removed.
2. `dotf link --prune --dry-run` prints `link.prune=<dest,…>` and changes nothing; the real run
   prints the removed dests and then runs the normal link pass.
3. `dotf unlink <dest…>` removes the named managed links (`$HOME`-relative, `~/` accepted);
   `--all` targets every table entry. Only a symlink pointing into the repo is removed; real
   files, foreign symlinks, missing dests and the Zed copy are skipped and reported. Unknown
   names fail before any change; no names and no `--all` is a usage error.
4. `dotf unlink --restore` restores each unlinked dest from the newest manifest backup holding
   it (existing `Engine.Restore`); dests with no backup are only unlinked. `--dry-run` previews.
5. `dotf doctor` adds a `symlinks.stale` warn listing stale links, remedy `dotf link --prune`.
6. `Engine.Validate` no longer reports a dangling symlink as `Valid`.
7. Help, completion and docs cover `unlink` and `--prune`; Go build/vet/test -race/lint pass.

## Decisions

| Decision | Why |
|---|---|
| Scan only the parent dirs of table dests, one level deep (`os.ReadDir`, no recursion) | Bounded, fast, never walks `$HOME`; matches the issue's "bounded to known dest parents" |
| Scan dirs come from **all** table entries, not only the host's facets; a link that is the dest of any table entry is "managed" regardless of facet | Turning off `--hyprland` must not make `dotf link --prune` delete the Hyprland links |
| "Points into the repo" = the link's **first hop** lies within `repoRoot` or its real path: the raw target (relative targets joined onto `EvalSymlinks(dir(link))`) with only its parent dir resolved and its final component never followed; no whole-chain `EvalSymlinks` | Dangling links must count; a link to another managed link (`~/.zshenv -> .zshrc`) is not into the repo; `..` through a symlinked scan dir resolves physically, as the kernel does |
| Never report a link at a strict ancestor of any table dest (`~/.config/zed`, `~/.config/systemd`, `~/.branchlet`) | A parent-dir link into the repo is a supported layout (Plan skips its children as already correct); pruning it would silently drop their config |
| A non-table, non-ancestor link into the repo is "orphaned" only if its first hop does not exist (ENOENT) or equals a table source (`Join(repoRoot, Src)` or its real path) | Live links to other repo paths (`~/.claude`, `~/.gemini`, created/migrated by `bin/` scripts) are not ours to remove; only dangling leftovers and a managed source linked from an old location are |
| Never prune a link whose target is the repo root itself, or that lies under `plasma/files/` | Root link may be how the repo is reached (`~/.config/dotfiles -> ~/src/dotfiles`); `plasma/files` links belong to `dotf plasma --link` |
| Skip a scan dir whose real path lies inside the repo | Otherwise we would delete repo-tracked symlinks |
| Table dest + dangling + spec source missing → stale ("dangling"); table dest + source present → not stale (link repairs it) | `link` cannot fix a link to a deleted source; it can fix the rest |
| Prune re-`Lstat`s/re-reads each candidate immediately before `os.Remove` and skips it if it changed; `link` re-plans after a prune that removed anything | TOCTOU safety; never removes something that became a real file; the pre-prune plan may be stale |
| No backup before removing a symlink | A symlink holds no data; its target stays in the repo |
| `unlink --all` covers all table entries (all facets); missing dests are silently skipped from output except a count | Removal is facet-independent; noise-free output |
| `unlink --restore` uses the newest non-legacy backup containing the dest, through `Engine.Restore` (all its guards); every restore group is dry-run first and any error aborts before a link is removed | Reuse the hardened restore path; a bad backup must not leave dests unlinked with nothing restored |
| On a removal error, the failed item and later unremoved ones become skips (`remove failed` / `not attempted`), the dests already removed are still restored, and the removal and restore errors are returned joined; `Restored` is set only on items whose restore completed (or, in DryRun, is planned), and `BackupID` keeps naming the planned source | The report never names an unperformed removal or restore; a removal failure never strands an already-removed, backed-up dest; the CLI can print a retry hint for an unperformed restore |
| `unlink` accepts `~/x`, `x`, or an absolute path under `$HOME` (shell-expanded `~/x`); absolute outside `$HOME` is an unknown dest | `dotf unlink ~/.config/nvim` works unquoted |
| Validate: dangling symlink with missing source → `Valid=false, Broken=false` | Not repairable by `link` (so not Broken) but not valid either |

## Contract (engine ↔ CLI)

```go
// internal/symlink/prune.go
const (
    StaleReasonOrphaned = "orphaned" // non-table link into the repo: first hop missing, or a table source at an old location
    StaleReasonDangling = "dangling" // table dest whose source no longer exists
)
type StaleLink struct {
    Dest    string // $HOME-relative, slash form (".config/foo")
    DestAbs string
    Target  string // raw os.Readlink value
    Reason  string // StaleReason*
}
// FindStale is read-only. specs is the full table (all facets).
func (e *Engine) FindStale(specs []LinkSpec) ([]StaleLink, error) // sorted by Dest
// Prune re-checks each link right before removing it; returns the removed subset (in input order)
// and the first error (already-removed ones are still returned).
func (e *Engine) Prune(stale []StaleLink) ([]StaleLink, error)

// internal/symlink/unlink.go
type UnlinkOptions struct {
    Dests   []string // $HOME-relative names; "~/" prefix accepted; ignored when All
    All     bool
    Restore bool
    DryRun  bool
}
const (
    UnlinkActionRemove = "remove"
    UnlinkActionSkip   = "skip"
)
type UnlinkItem struct {
    Dest     string // table Dest
    DestAbs  string
    Action   string // UnlinkAction*
    Reason   string // skip reason: "missing", "real file", "not a dotfiles link", "copy entry", "inside repo"
    BackupID string // with Restore: newest backup set holding the dest (the planned restore source); "" when none
    Restored bool   // restore from BackupID completed (or, in DryRun, is planned); false + BackupID != "" = left unlinked
}
type UnlinkReport struct {
    Items    []UnlinkItem // table order
    Restored []RestoreItem // performed (or, in DryRun, planned) restores
    DryRun   bool
}
type UnknownDestError struct{ Dests []string } // returned before any change
func (e *Engine) Unlink(specs []LinkSpec, opts UnlinkOptions) (*UnlinkReport, error)
```

## Tasks

- **A. Engine** (`internal/symlink`): `prune.go`, `unlink.go`, `validate.go` tweak, tests
  (table-driven, tmp dirs).
- **B. CLI** (`cmd/dotf`): `link --prune` (dry-run `link.prune=` line; real run prunes then links,
  printing `link.prune=`), new `unlink` command (`main.go`, `unlink.go`, `helptext.go`,
  completion), doctor `symlinks.stale`, README/CLAUDE.md/docs, tests.

## Output contract

- `dotf link --prune --dry-run` → stdout adds `link.prune=a,b`.
- `dotf link --prune` → stdout `link.prune=a,b` (removed), then the normal buckets.
- `dotf unlink …` → stdout `unlink.remove=…`, `unlink.skip=…`, `unlink.restore=…` (sorted dests),
  `dry-run=true` header on preview; stderr `dotf: unlink summary: removed=N skipped=N restored=N`.
  Exit 0 ok, 1 runtime error, 2 usage error / unknown dest.
