// Package symlink is the native Go port of the Ansible symlink system
// (playbooks/symlinks.yml + playbooks/vars/symlinks.yml + playbooks/tasks/*.yml).
//
// It reproduces the playbook's link behavior — link creation, real-file
// replacement, dry-run, post-run validation, broken-link handling, and the Zed
// copy exception — as a Plan/Apply/Validate engine driven by a static, typed
// link table (DefaultTable). Unlike the playbook, every real file or directory
// a link replaces is backed up first unless Options.Backup is false; the Zed
// settings copy is skipped when identical, and a differing file is backed up
// before it is overwritten.
// Because every managed destination lives under $HOME, the engine uses pure
// os.* calls and needs no privilege escalation.
//
// The engine takes repoRoot (the dotfiles source tree) and home ($HOME) as
// constructor parameters, keeping it free of environment lookups so tests can
// drive it hermetically against t.TempDir().
package symlink

import "github.com/yandy-r/dotfiles/internal/platform"

// Facet gates a LinkSpec to the OS families / flags on which it applies,
// mirroring the per-group `when:` conditions in playbooks/symlinks.yml.
type Facet int

// Facet values mirror logical OS/flag gates represented in playbooks/vars/symlinks.yml;
// they do not map one-to-one to YAML groups.
const (
	// FacetAll applies on every OS: bash/zsh/common/prompt/app/utility/
	// completions/special groups plus the Zed settings copy.
	FacetAll Facet = iota
	// FacetMacOS applies only on macOS (Ansible os_family == Darwin).
	FacetMacOS
	// FacetArch applies only on Arch-family hosts (Ansible os_family == Archlinux).
	// Empty today (ADR-12 moved the tmux user unit to FacetLinux), kept because
	// it is still a live gate in applies and in TestApplies — FacetWindows is
	// the precedent for a zero-entry facet.
	FacetArch
	// FacetWindows applies only on Windows (Ansible os_family == Windows; empty today).
	FacetWindows
	// FacetHyprland applies on Linux-family hosts only when the opt-in
	// ManageHyprland flag is set. It is the Hyprland/wlroots stack (hypr,
	// waybar, swayosd, mako, wlogout, fuzzel, satty plus the generic launchers)
	// and is categorically wrong for a KDE/Plasma host, which wants dotf plasma.
	FacetHyprland
	// FacetLinux applies on every Linux-family host, resolved through the local
	// isLinuxFamily (which deliberately excludes platform.OSLinux), NOT
	// platform.IsLinuxFamily.
	FacetLinux
	// FacetUnix applies on macOS and on every canonical Linux-family host,
	// including generic Linux and WSL. It excludes native Windows and unknown OSes.
	FacetUnix
	// FacetArchDesktop applies on Arch-family hosts only when the opt-in
	// ManageHyprland flag is set: Arch-branded desktop entries that must not
	// install on any other Linux family.
	FacetArchDesktop
)

// LinkSpec is one managed dotfiles-to-$HOME mapping, ported from
// playbooks/vars/symlinks.yml. Src is repo-relative; Dest is $HOME-relative.
type LinkSpec struct {
	// Src is the source path relative to the dotfiles repo root.
	Src string
	// Dest is the destination path relative to $HOME.
	Dest string
	// IsDir records the playbook's `type: directory` metadata. It is
	// DESCRIPTIVE ONLY and does not change link behavior (state: link links
	// files and directories identically).
	IsDir bool
	// Copy marks the Zed exception: the source is copied (overwrite, 0644),
	// not symlinked.
	Copy bool
	// Facet is the OS/flag gate controlling when this entry applies.
	Facet Facet
}

// DefaultTable returns the static link table, transcribed 1:1 from
// playbooks/vars/symlinks.yml in the assembly order documented in the
// symlink-semantics research note §2, plus the Zed settings copy entry.
//
// SYNC: DefaultTable is a frozen 1:1 transcription of
// playbooks/vars/symlinks.yml. TestTable_MatchesPlaybook locks it against an
// inline Go golden copy (NOT the YAML itself), so the two must be kept in sync
// manually until the retired playbook is deleted (Phase 4 rollback window).
//
// ADR-12 introduces one synchronized migration and two deliberate, permanent
// divergences from that 1:1 rule:
//
//  1. The tmux user unit moves from archlinux_configs to linux_configs. This one
//     IS mirrored in the YAML, so the playbook and the table still agree.
//  2. arch-update.desktop becomes Arch-gated here (FacetArchDesktop) and is
//     deliberately NOT mirrored: the YAML keeps it in the ungated
//     launcher_configs group, because splitting it out on a retired,
//     rollback-only playbook would need a new launcher group plus a new
//     tasks/*.yml for zero rollback value.
//  3. The native FacetUnix Herdr and espanso configs are represented in the
//     rollback YAML under both macOS and Linux groups.
func DefaultTable() []LinkSpec {
	return []LinkSpec{
		// bash_files
		{Src: "bash/bashrc", Dest: ".bashrc", Facet: FacetAll},
		{Src: "bash/bash_profile", Dest: ".bash_profile", Facet: FacetAll},
		{Src: "bash/profile", Dest: ".profile", Facet: FacetAll},
		{Src: "bash/.fzf.bash", Dest: ".fzf.bash", Facet: FacetAll},

		// zsh_files
		{Src: "zsh/zshrc", Dest: ".zshrc", Facet: FacetAll},
		{Src: "zsh/zprofile", Dest: ".zprofile", Facet: FacetAll},
		{Src: "zsh/functions.zsh", Dest: ".functions.zsh", Facet: FacetAll},
		{Src: "zsh/fzf.zsh", Dest: ".fzf.zsh", Facet: FacetAll},

		// common_files
		{Src: "common/gitconfig", Dest: ".gitconfig", Facet: FacetAll},
		{Src: "common/gitignore", Dest: ".gitignore", Facet: FacetAll},
		{Src: "common/tmux.conf", Dest: ".tmux.conf", Facet: FacetAll},
		{Src: "common/screenrc", Dest: ".screenrc", Facet: FacetAll},
		{Src: "common/wgetrc", Dest: ".wgetrc", Facet: FacetAll},
		{Src: "shellcheck/shellcheckrc", Dest: ".shellcheckrc", Facet: FacetAll},

		// prompt_config
		{Src: "starship.toml", Dest: ".config/starship.toml", Facet: FacetAll},

		// app_configs (directories)
		{Src: "nvim", Dest: ".config/nvim", IsDir: true, Facet: FacetAll},
		{Src: "wezterm", Dest: ".config/wezterm", IsDir: true, Facet: FacetAll},
		{Src: "ghostty", Dest: ".config/ghostty", IsDir: true, Facet: FacetAll},
		{Src: "mise", Dest: ".config/mise", IsDir: true, Facet: FacetAll},
		{Src: "alacritty", Dest: ".config/alacritty", IsDir: true, Facet: FacetAll},

		// macos_configs (Darwin only)
		{Src: "aerospace", Dest: ".config/aerospace", IsDir: true, Facet: FacetMacOS},

		// unix_configs (macOS and all Linux families; herdr is a single file so runtime
		// logs stay local; espanso reads ~/.config/espanso on both)
		{Src: "herdr/config.toml", Dest: ".config/herdr/config.toml", Facet: FacetUnix},
		{Src: "espanso", Dest: ".config/espanso", IsDir: true, Facet: FacetUnix},

		// generic_linux_desktop_configs (Linux family, opt-in; directories —
		// LinuxDesktopGuardError refuses to write this set with backups off)
		{Src: "hypr", Dest: ".config/hypr", IsDir: true, Facet: FacetHyprland},
		{Src: "waybar", Dest: ".config/waybar", IsDir: true, Facet: FacetHyprland},
		{Src: "swayosd", Dest: ".config/swayosd", IsDir: true, Facet: FacetHyprland},
		{Src: "mako", Dest: ".config/mako", IsDir: true, Facet: FacetHyprland},
		{Src: "wlogout", Dest: ".config/wlogout", IsDir: true, Facet: FacetHyprland},
		{Src: "fuzzel", Dest: ".config/fuzzel", IsDir: true, Facet: FacetHyprland},
		{Src: "satty", Dest: ".config/satty", IsDir: true, Facet: FacetHyprland},

		// launcher_configs (Linux family, opt-in)
		{Src: "launchers/applications/alacritty-float.desktop", Dest: ".local/share/applications/alacritty-float.desktop", Facet: FacetHyprland},
		{Src: "launchers/applications/arch-update.desktop", Dest: ".local/share/applications/arch-update.desktop", Facet: FacetArchDesktop},
		{Src: "launchers/applications/gedit-float.desktop", Dest: ".local/share/applications/gedit-float.desktop", Facet: FacetHyprland},
		{Src: "launchers/applications/wifi-tui.desktop", Dest: ".local/share/applications/wifi-tui.desktop", Facet: FacetHyprland},

		// linux_configs (Linux family — ADR-12: the unit is not Arch-specific)
		{Src: "common/systemd/user/tmux.service", Dest: ".config/systemd/user/tmux.service", Facet: FacetLinux},

		// utility_dirs (directory)
		{Src: "bin", Dest: ".bin", IsDir: true, Facet: FacetAll},

		// special_symlinks
		{Src: "ssh/config", Dest: ".ssh/config", Facet: FacetAll},
		{Src: "ansible.cfg", Dest: ".ansible.cfg", Facet: FacetAll},
		{Src: "dotenv", Dest: ".config/direnv", IsDir: true, Facet: FacetAll},
		{Src: "branchlet/settings.json", Dest: ".branchlet/settings.json", Facet: FacetAll},
		{Src: "atuin/config.toml", Dest: ".config/atuin/config.toml", Facet: FacetAll},
		{Src: "lazygit/config.yml", Dest: ".config/lazygit/config.yml", Facet: FacetAll},
		{Src: "zed/keymap.json", Dest: ".config/zed/keymap.json", Facet: FacetAll},

		// Zed settings copy exception (copy+overwrite, 0644; not symlinked)
		{Src: "zed/settings.json", Dest: ".config/zed/settings.json", Copy: true, Facet: FacetAll},
	}
}

// isLinuxFamily reports whether os is treated as a Linux-family host, mirroring
// the Ansible `is_linux_family` predicate (os_family in RedHat/Debian/Suse/
// Archlinux).
//
// OSWSL is deliberately included: modern WSL with /etc/os-release resolves to a
// concrete distro family (matching Ansible), so only a WSL host lacking
// os-release yields OSWSL. Treating OSWSL as Linux-family preserves parity for
// that edge case; see TestFaceting_WSL.
func isLinuxFamily(os platform.OS) bool {
	switch os {
	case platform.OSDebian, platform.OSRedHat, platform.OSArch, platform.OSSUSE, platform.OSWSL:
		return true
	default:
		return false
	}
}

// applies reports whether spec is active on the host described by info under
// opts, implementing the group `when:` gates of playbooks/symlinks.yml.
func applies(spec LinkSpec, info platform.Info, opts Options) bool {
	switch spec.Facet {
	case FacetAll:
		return true
	case FacetMacOS:
		return info.OS == platform.OSMacOS
	case FacetArch:
		return info.OS == platform.OSArch
	case FacetWindows:
		return info.OS == platform.OSWindows
	case FacetHyprland:
		return isLinuxFamily(info.OS) && opts.ManageHyprland
	case FacetLinux:
		return isLinuxFamily(info.OS)
	case FacetUnix:
		return info.OS == platform.OSMacOS || platform.IsLinuxFamily(info.OS)
	case FacetArchDesktop:
		return info.OS == platform.OSArch && opts.ManageHyprland
	default:
		return false
	}
}
