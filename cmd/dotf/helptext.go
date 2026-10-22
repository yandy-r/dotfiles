package main

import "flag"

// This file is the PROSE single source of truth for `dotf` help: the
// commandHelp type, the ordered helpRegistry, and the header/conventions/
// environment string consts. It holds DATA only — no rendering logic (that
// lives in help.go) — so both files stay well under the ~500-line soft cap as
// the per-command descriptions grow.

// commandHelp is one command's documentation. Flag descriptions are NOT stored
// here: setFlags registers the command's real flags on a flagset so the help
// renderer reads their usage strings verbatim (DRY). setFlags is nil for
// commands with no flags (version, platform, help).
type commandHelp struct {
	name     string
	summary  string
	usage    string
	long     string
	examples []string
	setFlags func(*flag.FlagSet)
}

const tagline = "dotfiles provisioning CLI"

// conventions is the stdout/stderr contract and exit-code legend, shown in
// global help.
const conventions = "Conventions:\n" +
	"  stdout carries machine-greppable key=value data; stderr carries diagnostics.\n" +
	"  Flags: shown with double dashes (--name); a single dash (-name) is equivalent.\n" +
	"  Exit codes: 0 success; 1 completed-with-failures or soft error;\n" +
	"              2 usage error (bad flag); 3 hard abort (apply preconditions)."

// envHelp lists the environment variables that influence the CLI, shown
// (indented) under the global-help Environment section.
const envHelp = "DOTF_MANIFEST   default manifest path (--manifest overrides)\n" +
	"DOTF_NO_USER_CONFIG  disable the ~/.config/dotf user-config overlay (--no-user-config)\n" +
	"DOTFILES_ROOT   dotfiles repo root for source resolution\n" +
	"ACCESSIBLE      set to force plan/apply into accessible (non-TUI) mode\n" +
	"apply env twins NON_INTERACTIVE, FORCE_INSTALL, ENABLE_GAMING, ENABLE_OLLAMA,\n" +
	"                HYPRLAND (LINUX_DESKTOP is its deprecated alias), SNAPPER,\n" +
	"                PLASMA (each =true mirrors its flag)"

// helpRegistry is the ordered command catalog: display order in global help,
// documentation order here, and the drift guard's expected set. Every
// dispatchable command in main.go's switch MUST have an entry here (enforced by
// TestHelpRegistry_CoversDispatch); "help" is listed too but is handled before
// the switch.
var helpRegistry = []commandHelp{
	{
		name:    "version",
		summary: "Print the dotf version and build commit",
		usage:   "dotf version",
		long: "Print the dotf CLI version and the git commit it was built from, as a single\n" +
			"line to stdout. Reads nothing about the host.",
		examples: []string{"dotf version"},
	},
	{
		name:    "platform",
		summary: "Print detected platform facts (os, arch, package manager, distro)",
		usage:   "dotf platform",
		long: "Print the detected host platform as machine-greppable key=value pairs:\n" +
			"operating system, CPU architecture, package manager, container status, and\n" +
			"Linux distro. Reads only; changes nothing.",
		examples: []string{"dotf platform"},
	},
	{
		name:    "plan",
		summary: "Preview the resolved package selection; applies nothing",
		usage:   "dotf plan [flags]",
		long: "Resolve the provisioning manifest, detect the host platform, and drive the\n" +
			"interactive selection wizard, then print the resolved package selection as\n" +
			"key=value lines to stdout. Applies nothing to the host - use apply to\n" +
			"provision. Falls back to accessible (non-TUI) mode when stdout is not a\n" +
			"terminal or ACCESSIBLE is set.",
		examples: []string{
			"dotf plan",
			"dotf plan --manifest ./manifest/packages.toml",
			"dotf plan --json",
		},
		setFlags: func(fs *flag.FlagSet) { setReadFlags(fs); setNoUserConfigFlag(fs) },
	},
	{
		name:    "install",
		summary: "Install packages declared in the manifest",
		usage:   "dotf install [flags] [name...]",
		long: "Resolve the manifest, detect the host platform, and install the declared\n" +
			"packages via the backend engine. With no --intent flags every package is\n" +
			"installed; each --intent narrows the set to that intent's packages, and\n" +
			"positional names install just those packages (folded into the --only set).\n" +
			"Packages whose backend cannot run on this host are skipped as\n" +
			"\"not-applicable\", not failed. GUI packages (claude-desktop, espanso) are also\n" +
			"not-applicable without a graphical session (SSH, TTY, container), even with\n" +
			"--only. A name selects its exact <name>-arch/-debian/-linux/-macos/-fedora/\n" +
			"-arm64 siblings too, so\n" +
			"claude-desktop picks the apt package on Debian/Ubuntu, the Homebrew cask on\n" +
			"macOS, and a local RPM repacked from the verified official .deb on Fedora.\n" +
			"Failures are collected non-fatally and the run\n" +
			"exits 1 if any package failed.\n" +
			"\n" +
			"--update updates named package(s) in place and --remove removes them; both\n" +
			"dispatch only to backends that declare support (pkg and toolchain natively,\n" +
			"custom-script packages that list the action in their manifest actions), and a\n" +
			"backend without the capability fails that package (exit 1). --update and\n" +
			"--remove are mutually exclusive and require an explicit target (a name,\n" +
			"--only, or --intent) — they never operate on the implicit whole-manifest set.\n" +
			"--update skips a package that is not installed; --remove skips one already\n" +
			"absent.",
		examples: []string{
			"dotf install",
			"dotf install --dry-run",
			"dotf install --intent ai --intent desktop",
			"dotf install claude",
			"dotf install --only claude-desktop --only espanso",
			"dotf install claude --update",
			"dotf install claude --remove",
		},
		setFlags: func(fs *flag.FlagSet) { setInstallFlags(fs); setNoUserConfigFlag(fs) },
	},
	{
		name:    "status",
		summary: "Report installed state for every declared package",
		usage:   "dotf status [flags]",
		long: "Resolve the manifest and probe installed state for every declared package\n" +
			"(read-only), printing one sorted \"package=NAME backend=BACKEND\n" +
			"installed=BOOL\" line per package to stdout. Packages whose backend cannot\n" +
			"run on this host (e.g. apt-repo on a non-apt host) are not probed and are\n" +
			"printed with \"applicable=false\" instead of installed state. --counts appends\n" +
			"an aggregate backend/class histogram (count.* lines, or a summary object with\n" +
			"--json) for governance visibility.",
		examples: []string{
			"dotf status",
			"dotf status --manifest ./manifest/packages.toml",
			"dotf status --json",
			"dotf status --counts",
		},
		setFlags: func(fs *flag.FlagSet) { setDriftFlags(fs); setNoUserConfigFlag(fs) },
	},
	{
		name:    "diff",
		summary: "Report declared-but-missing package drift (exit 1 on drift)",
		usage:   "dotf diff [flags]",
		long: "Resolve the manifest and probe installed state (read-only), printing only\n" +
			"declared-but-not-installed drift as \"drift=declared-not-installed\" lines to\n" +
			"stdout. Packages whose backend cannot run on this host are not applicable\n" +
			"and never count as drift. Exits 1 when any drift is present, so it doubles\n" +
			"as a CI convergence check. --counts appends an aggregate backend/class\n" +
			"histogram over the drift set (count.* lines, or a summary object with --json).",
		examples: []string{
			"dotf diff",
			"dotf diff --manifest ./manifest/packages.toml",
			"dotf diff --json",
			"dotf diff --counts",
		},
		setFlags: func(fs *flag.FlagSet) { setDriftFlags(fs); setNoUserConfigFlag(fs) },
	},
	{
		name:    "doctor",
		summary: "Diagnose host health: platform, prerequisites, packages, symlinks, sysconfig",
		usage:   "dotf doctor [flags]",
		long: "Run read-only health checks over the host and print one\n" +
			"\"check.CATEGORY.NAME=ok|warn|fail\" line per check to stdout, followed by a\n" +
			"\"doctor.total/ok/warn/fail\" summary line. stdout is status-only and stable\n" +
			"(greppable check.*=status lines only); all human detail goes to stderr. Every\n" +
			"non-ok check prints an aggregate remedy line on stderr, and — for\n" +
			"packages.drift, symlinks.pending, symlinks.stale and plasma.drift — one\n" +
			"indented \"  - item\" line per missing package (name, backend, class), pending\n" +
			"link (action + dest), stale link (reason + dest) or drifted plasma file\n" +
			"(action + dest). With\n" +
			"--verbose, ok checks also print their evidence on stderr (the platform tuple,\n" +
			"prerequisite paths found, the full applicable-package tally, symlink health,\n" +
			"and sysconfig support level). Checks cover platform compatibility, required\n" +
			"prerequisites (git, a download tool, the package manager, sudo),\n" +
			"declared-package drift (via the same truthful probe as status/diff), symlink\n" +
			"health (broken, not-yet-created, and stale links into the repo — the latter\n" +
			"warn with a dotf link --prune remedy), the symlink backup dirs (count and\n" +
			"size; warns above 10 with a dotf backup prune remedy), sysconfig support for\n" +
			"this OS, and KDE Plasma drift plus whether plasma/files/ has been exported\n" +
			"(both omitted entirely on an OS plasma does not target).\n" +
			"Applies nothing. The symlink checks use the built-in link table, so\n" +
			"--manifest affects only the package/sysconfig checks. Exits 1 when any check\n" +
			"fails; warnings (e.g. an un-provisioned host) stay green. --json emits the\n" +
			"full report — including the additive per-check items array — to stdout; the\n" +
			"default --json shape is unchanged for checks that carry no items.",
		examples: []string{
			"dotf doctor",
			"dotf doctor --verbose",
			"dotf doctor --json",
			"dotf doctor --manifest ./manifest/packages.toml",
		},
		setFlags: func(fs *flag.FlagSet) { setDoctorFlags(fs); setNoUserConfigFlag(fs) },
	},
	{
		name:    "link",
		summary: "Provision the dotfiles symlink set into $HOME",
		usage:   "dotf link [flags]",
		long: "Link the static dotfiles table (bash, zsh, shared configs, and application\n" +
			"directories) into $HOME using the engine's plan/apply/validate split.\n" +
			"Symlinks live under $HOME, so the operation is unprivileged. The link set is\n" +
			"the built-in default table, so there is deliberately no --manifest flag.\n" +
			"Every real file or directory a link replaces is first copied into\n" +
			"~/.config/dotfiles_backup_<epoch> (named on stderr when used) together with a\n" +
			".dotf-backup.json manifest; recover with dotf backup restore <id> (see dotf\n" +
			"backup list). --no-backup opts out, and --backup is accepted as the explicit\n" +
			"spelling of the default (--backup=false does NOT disable backups; use\n" +
			"--no-backup).\n" +
			"After applying, links are validated; a real file where a symlink was expected,\n" +
			"or a dangling link whose source exists, exits 1. --prune first removes stale\n" +
			"symlinks into the repo, found in the parent dirs of the table's dests: a\n" +
			"non-table link whose target in the repo is missing, or that still points at\n" +
			"a managed source from an old location; and a table link whose source was\n" +
			"deleted. Parent-dir links (e.g. ~/.config/zed -> repo zed), links under\n" +
			"plasma/files/ (left to dotf plasma), links to the repo root and live links\n" +
			"to other repo paths (e.g. ~/.claude) are never pruned, nor are real files.\n" +
			"With --dry-run the prune candidates are listed as link.prune= and nothing is\n" +
			"removed. To remove managed links on purpose, use dotf unlink. The opt-in\n" +
			"desktop set is the Hyprland/wlroots stack and is not what a KDE/Plasma host\n" +
			"wants\n" +
			"(that is dotf plasma).",
		examples: []string{
			"dotf link --dry-run",
			"dotf link",
			"dotf link --no-backup",
			"dotf link --dry-run --hyprland",
			"dotf link --prune --dry-run",
			"dotf link --prune",
		},
		setFlags: func(fs *flag.FlagSet) { setLinkFlags(fs) },
	},
	{
		name:    "unlink",
		summary: "Remove managed symlinks, optionally restoring backups",
		usage:   "dotf unlink <dest...|--all> [flags]",
		long: "Remove the named managed symlinks from $HOME. Dests are $HOME-relative link\n" +
			"table names (a leading ~/ is accepted); --all targets every table entry\n" +
			"across all facets. Unknown names fail before anything changes; giving no\n" +
			"dest and no --all, or dests together with --all, is a usage error.\n" +
			"\n" +
			"Only a symlink that points into the dotfiles repo is removed. Real files and\n" +
			"directories are never touched; foreign symlinks, missing dests and the Zed\n" +
			"copy entry are skipped and reported on stderr (dotf: unlink: skip <dest>:\n" +
			"<reason>); under --all, missing dests and the copy entry are only counted in\n" +
			"the summary. No backup is taken: a symlink holds no data.\n" +
			"\n" +
			"--restore puts back each unlinked dest from the newest manifest backup that\n" +
			"holds it, through dotf backup restore's safety checks; a dest with no backup\n" +
			"is left unlinked and named on stderr. --dry-run previews everything.\n" +
			"\n" +
			"stdout carries unlink.remove=, unlink.skip= and unlink.restore= lines (sorted\n" +
			"dests; dry-run=true header on a preview); stderr ends with a\n" +
			"\"dotf: unlink summary: removed=N skipped=N restored=N\" line.\n" +
			"\n" +
			"Exit codes: 0 ok, 1 failure, 2 usage error or unknown dest.",
		examples: []string{
			"dotf unlink .bashrc .zshrc",
			"dotf unlink ~/.config/nvim --restore",
			"dotf unlink --all --dry-run",
			"dotf unlink --all --restore",
		},
		setFlags: func(fs *flag.FlagSet) { setUnlinkFlags(fs) },
	},
	{
		name:    "backup",
		summary: "List, restore, and prune the symlink backup dirs",
		usage:   "dotf backup <list|restore|prune> [args] [flags]",
		long: "Manage the ~/.config/dotfiles_backup_<epoch>[_N] dirs that link, apply and\n" +
			"setup create when they replace real files. Each dir carries a\n" +
			".dotf-backup.json manifest recording every backed-up $HOME-relative path.\n" +
			"\n" +
			"list prints every backup dir newest first (id, created time, entry count,\n" +
			"size); dirs from before manifests existed are shown as legacy=true.\n" +
			"\n" +
			"restore <id> [path...] copies the manifest entries (or just the named\n" +
			"paths) back into $HOME. A missing dest or a symlink is replaced; a real file\n" +
			"or directory is refused unless --force, which first backs that content up to\n" +
			"a NEW backup dir. Legacy dirs and unsafe paths are refused. The backup dir is\n" +
			"copied from, never modified. --dry-run prints the plan.\n" +
			"\n" +
			"prune removes all but the newest --keep dirs (default 10), optionally only\n" +
			"those older than --older-than (Go duration or <N>d). It asks before deleting\n" +
			"unless --yes, and refuses non-interactively without --yes. Only\n" +
			"dotfiles_backup_<epoch>[_N] dirs are ever removed.\n" +
			"\n" +
			"Exit codes: 0 ok, 1 refusal or failure, 2 usage error.",
		examples: []string{
			"dotf backup list",
			"dotf backup list --json",
			"dotf backup restore 1729543200 --dry-run",
			"dotf backup restore 1729543200 .bashrc .zshrc",
			"dotf backup restore 1729543200 --force",
			"dotf backup prune --keep 5 --dry-run",
			"dotf backup prune --older-than 30d --yes",
		},
		setFlags: func(fs *flag.FlagSet) { setBackupFlags(fs) },
	},
	{
		name:    "sysconfig",
		summary: "Apply per-OS system config: hostname, timezone, locale, shell, firewall, ssh server",
		usage:   "dotf sysconfig [flags]",
		long: "Resolve the desired system configuration from flags and the manifest's\n" +
			"[sysconfig] table (flag > manifest > skip) and apply per-OS hostname,\n" +
			"timezone, locale, login shell, firewall, and ssh server via the sysconfig\n" +
			"engine's plan/apply split. The apply path is gated behind an interactive\n" +
			"confirmation unless --yes is given, and refuses to apply non-interactively\n" +
			"without --yes. --dry-run previews the plan and never primes sudo.",
		examples: []string{
			"dotf sysconfig --dry-run",
			"dotf sysconfig --ssh-server on --dry-run",
			"dotf sysconfig --timezone America/New_York --yes",
			"dotf sysconfig --hostname workstation --firewall ufw --yes",
		},
		setFlags: func(fs *flag.FlagSet) { setSysconfigFlags(fs); setNoUserConfigFlag(fs) },
	},
	{
		name:    "apply",
		summary: "Run the full provisioning pipeline end to end",
		usage:   "dotf apply [flags]",
		long: "Run the full provisioning pipeline end to end in PRD order: identity\n" +
			"sysconfig, packages, symlinks, plasma, then firewall and ssh server. The plasma\n" +
			"phase is opt-in via --plasma and is a no-op off Linux. Boolean flags mirror\n" +
			"bootstrap.sh environment twins (NON_INTERACTIVE, FORCE_INSTALL, ENABLE_GAMING,\n" +
			"ENABLE_OLLAMA, HYPRLAND with LINUX_DESKTOP as its deprecated alias, SNAPPER,\n" +
			"PLASMA). Without --headless/--force on a terminal it drops into the guided\n" +
			"wizard; non-interactively it refuses unless headless. --dry-run composes the\n" +
			"plan and never primes sudo. The symlink phase backs up every real file it\n" +
			"replaces into ~/.config/dotfiles_backup_<epoch> (flag-only --no-backup opts\n" +
			"out); recover with dotf backup restore <id>. Exit codes:\n" +
			"0 converged, 1 completed-with-failures, 2 usage error, 3 hard abort.",
		examples: []string{
			"dotf apply",
			"dotf apply --headless",
			"dotf apply --dry-run",
			"dotf apply --headless --gaming --snapper",
		},
		setFlags: func(fs *flag.FlagSet) { setApplyFlags(fs) },
	},
	{
		name:    "lock",
		summary: "Resolve and record pinned-artifact checksums into manifest/dotf.lock",
		usage:   "dotf lock [flags]",
		long: "Resolve every pinnable package (github-release, checksummed script-url) for\n" +
			"the target platform(s), download each artifact, hash it, and write the frozen\n" +
			"tag/asset/binary/sha256/os/arch to manifest/dotf.lock as deterministic TOML.\n" +
			"Rolling managers and setup-step scripts are intentionally floating and are\n" +
			"never locked. The host is the only target by default (you can only checksum\n" +
			"what you can download); --platform adds more. --check re-resolves and diffs\n" +
			"against the committed lock without writing, exiting 1 on drift (the CI\n" +
			"freshness gate); --dry-run prints the resolved artifacts without writing.\n" +
			"Per-package resolution failures are non-fatal; the run exits 1 if any failed.\n" +
			"The host's own target is always the one consulted at install time; a\n" +
			"--platform record for a different OS family is recorded but not yet honored\n" +
			"until release-OS token normalization lands.",
		examples: []string{
			"dotf lock",
			"dotf lock --check",
			"dotf lock --dry-run --json",
			"dotf lock --only some-tool",
		},
		setFlags: func(fs *flag.FlagSet) { setLockFlags(fs); setNoUserConfigFlag(fs) },
	},
	{
		name:    "self-update",
		summary: "Update the dotf binary in place to the latest release",
		usage:   "dotf self-update [flags]",
		long: "Discover the latest published dotf release on GitHub, download the release\n" +
			"tarball for this host, verify it against the release checksums.txt with the\n" +
			"same SHA-256 contract bootstrap.sh uses, and atomically replace the running\n" +
			"binary (same-directory temp file + rename). Scoped to the dotf binary ONLY -\n" +
			"it never resolves the manifest or touches manifest-declared tools, so there is\n" +
			"deliberately no --manifest flag. Linux and macOS only; Windows is deferred and\n" +
			"fails fast. --check reports whether an update exists and writes nothing;\n" +
			"--dry-run additionally resolves the target path without downloading; --force\n" +
			"replaces even when already current, newer, or an unparseable dev build. Exits\n" +
			"0 when updated, already current, or reporting; 1 on a download/checksum/replace\n" +
			"failure, permission denial, unsupported OS, or a dev build without --force.",
		examples: []string{
			"dotf self-update",
			"dotf self-update --check",
			"dotf self-update --dry-run",
			"dotf self-update --force",
		},
		setFlags: func(fs *flag.FlagSet) { setSelfUpdateFlags(fs) },
	},
	{
		name:    "setup",
		summary: "Run a named setup module's ordered, typed steps",
		usage:   "dotf setup <domain> [flags]",
		long: "Resolve the manifest and run the named [[module]]'s ordered steps through the\n" +
			"shipped engines: a custom-script step runs a contained setup script, a pkg\n" +
			"step ensures a manifest package is installed (probe-gated, idempotent), and a\n" +
			"symlink step provisions a built-in link-table entry. Steps are typed and\n" +
			"closed (symlink/pkg/sysconfig/custom-script), so setup cannot execute arbitrary\n" +
			"shell. Sysconfig steps are not yet wired and fail that step fast. Step failures\n" +
			"are collected non-fatally into a run-wide summary. --dry-run prints the ordered\n" +
			"step plan and never primes sudo; executing non-interactively requires --yes.\n" +
			"All symlink steps of one run share a single backup dir, named once on stderr\n" +
			"(recover with dotf backup restore <id>).\n" +
			"Exit codes: 0 converged, 1 step failures, 2 usage, 3 hard abort.",
		examples: []string{
			"dotf setup fingerprint",
			"dotf setup onedrive --dry-run",
		},
		setFlags: func(fs *flag.FlagSet) { setSetupFlags(fs); setNoUserConfigFlag(fs) },
	},
	{
		name:    "plasma",
		summary: "Manage the curated KDE Plasma 6 configuration set",
		usage:   "dotf plasma <export|apply|diff|status> [flags]",
		long: "Manage a curated subset of KDE Plasma 6 configuration as repo-tracked data.\n" +
			"export captures the live desktop into plasma/files/, stripping volatile state\n" +
			"(machine UUIDs, monitor identifiers, window geometry, migration ledgers) so\n" +
			"nothing machine-local reaches git. apply merges the repo's keys back into the\n" +
			"live files: every repo key is written and every live-only key is preserved, so\n" +
			"an apply is never a blind overwrite. diff reports per-key drift in both\n" +
			"directions and exits 1 when any file differs (the CI convergence check);\n" +
			"status summarises presence and drift counts. diff separates the live-only keys\n" +
			"that nothing explains (plasma.only-live, i.e. unmanaged state) from the ones\n" +
			"export strips by design, which are hidden unless --show-volatile is given;\n" +
			"neither kind counts as drift or affects the exit code. All INI work is a native\n" +
			"Go KConfig codec - no kwriteconfig6 subprocesses - and the managed set is a\n" +
			"built-in table, so there is deliberately no --manifest flag. The repo holds\n" +
			"copies, not symlink targets, so experimenting in System Settings never dirties\n" +
			"the worktree; --link opts into symlinking per run and --unlink is its inverse,\n" +
			"replacing each symlink with a real file again. Neither happens by accident: a\n" +
			"plain apply leaves a linked file exactly as it is, and switching INTO link mode\n" +
			"needs --backup (it discards live-only keys) or --dry-run to preview the refusal.\n" +
			"Linux only. apply confirms before writing unless --yes, and --reload (opt-in)\n" +
			"restarts the affected KDE components afterwards - without it the exact commands\n" +
			"are printed instead.",
		examples: []string{
			"dotf plasma status",
			"dotf plasma diff --json",
			"dotf plasma diff --show-volatile",
			"dotf plasma export --dry-run",
			"dotf plasma apply --backup --yes",
			"dotf plasma apply --unlink --yes",
			"dotf plasma apply --recommended --reload --yes",
		},
		setFlags: func(fs *flag.FlagSet) { setPlasmaFlags(fs) },
	},
	{
		name:    "deb",
		summary: "Convert a Debian .deb into a pacman package (Arch hosts)",
		usage:   "dotf deb <url|path> [flags]",
		long: "Convert a Debian package into a pacman-installable Arch package with debtap,\n" +
			"verifying it at every stage. The argument is an https URL or a filesystem\n" +
			"path; plaintext http is refused. Requires a pacman-based host with debtap,\n" +
			"bsdtar and pkgfile installed and debtap's translation database populated.\n" +
			"Each of those preconditions is checked up front and, when it has a known\n" +
			"fix, the exact command is explained and offered before anything runs.\n" +
			"\n" +
			"The input is identified as a .deb by its ar header rather than its file\n" +
			"extension, its SHA-256 is always reported, and --sha256 verifies it against an\n" +
			"expected digest before anything else runs. If the package name already exists\n" +
			"in the official repositories the run stops unless --force is given, because a\n" +
			"converted .deb shadows the native package and never receives updates; an AUR\n" +
			"match only warns. The built package is inspected via its own .PKGINFO and its\n" +
			"translated dependencies are checked with pacman -T, which reports\n" +
			"debtap's approximate Debian-to-Arch mapping failures as warnings.\n" +
			"\n" +
			"Conversion alone changes nothing on the host. --install additionally installs\n" +
			"the result, prompting first unless --yes is given, and then confirms with\n" +
			"pacman what was actually registered and how many files it owns. Installing\n" +
			"runs the package's install scripts as root. Without a terminal, --install\n" +
			"requires --yes and otherwise refuses rather than installing unattended.\n" +
			"\n" +
			"The command refuses to run as root: only the install step elevates, and it\n" +
			"invokes sudo itself. --keep retains the staged input and the debtap work\n" +
			"directory and reports both paths.",
		examples: []string{
			"dotf deb ./slack-desktop.deb",
			"dotf deb https://example.com/app.deb --sha256 9f2b...",
			"dotf deb ./app.deb --out-dir ~/pkgs",
			"dotf deb ./app.deb --install",
			"dotf deb ./app.deb --install --yes",
			"dotf deb ./app.deb --dry-run",
		},
		setFlags: func(fs *flag.FlagSet) { setDebFlags(fs) },
	},
	{
		name:    "completion",
		summary: "Generate a shell completion script (bash, zsh, or fish)",
		usage:   "dotf completion [shell] [flags]",
		long: "Generate a completion script for the given shell and print it to stdout, or\n" +
			"with --install write it into that shell's standard completion directory. The\n" +
			"shell is bash, zsh, or fish; when omitted it is detected from $SHELL. Install\n" +
			"targets follow the XDG base directories: bash-completion's user dir, a zsh\n" +
			"$fpath site-functions dir, or fish's completions dir. --install refuses to\n" +
			"overwrite an existing script unless --force is given, and --dry-run reports the\n" +
			"target path without writing. The command and flag set is generated from this\n" +
			"same help registry, so completions never drift from the real CLI.",
		examples: []string{
			"dotf completion zsh",
			"dotf completion bash --install",
			"dotf completion fish --install --force",
			"dotf completion zsh --install --dry-run",
		},
		setFlags: func(fs *flag.FlagSet) { setCompletionFlags(fs) },
	},
	{
		name:    "help",
		summary: "Show global help, or detailed help for a command",
		usage:   "dotf help [command]",
		long: "Show global help, or detailed help for a specific command. Equivalent to the\n" +
			"global --help/-h flags, and to \"<command> --help\" for per-command help.",
		examples: []string{
			"dotf help",
			"dotf help apply",
		},
	},
}
