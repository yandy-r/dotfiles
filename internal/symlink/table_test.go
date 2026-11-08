package symlink

import (
	"testing"

	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestTable_Count(t *testing.T) {
	if got, want := len(DefaultTable()), 50; got != want {
		t.Errorf("len(DefaultTable()) = %d, want %d", got, want)
	}
}

func TestTable_FacetCounts(t *testing.T) {
	counts := map[Facet]int{}
	for _, spec := range DefaultTable() {
		counts[spec.Facet]++
	}

	want := map[Facet]int{
		FacetAll:         30,
		FacetMacOS:       1,
		FacetArch:        0,
		FacetWindows:     0,
		FacetLinux:       1,
		FacetUnix:        7,
		FacetHyprland:    10,
		FacetArchDesktop: 1,
	}
	for facet, wantN := range want {
		if got := counts[facet]; got != wantN {
			t.Errorf("facet %d count = %d, want %d", facet, got, wantN)
		}
	}
}

// TestTable_MatchesPlaybook is the parity guard: it encodes the expected
// 44-entry set inline (transcribed from playbooks/vars/symlinks.yml) and diffs
// it against DefaultTable, so any drift fails CI. Zero new dependencies.
func TestTable_MatchesPlaybook(t *testing.T) {
	want := []LinkSpec{
		{Src: "bash/bashrc", Dest: ".bashrc", Facet: FacetAll},
		{Src: "bash/bash_profile", Dest: ".bash_profile", Facet: FacetAll},
		{Src: "bash/profile", Dest: ".profile", Facet: FacetAll},
		{Src: "bash/.fzf.bash", Dest: ".fzf.bash", Facet: FacetAll},
		{Src: "zsh/zshrc", Dest: ".zshrc", Facet: FacetAll},
		{Src: "zsh/zprofile", Dest: ".zprofile", Facet: FacetAll},
		{Src: "zsh/functions.zsh", Dest: ".functions.zsh", Facet: FacetAll},
		{Src: "zsh/fzf.zsh", Dest: ".fzf.zsh", Facet: FacetAll},
		{Src: "common/gitconfig", Dest: ".gitconfig", Facet: FacetAll},
		{Src: "common/gitignore", Dest: ".gitignore", Facet: FacetAll},
		{Src: "common/tmux.conf", Dest: ".tmux.conf", Facet: FacetAll},
		{Src: "common/screenrc", Dest: ".screenrc", Facet: FacetAll},
		{Src: "common/wgetrc", Dest: ".wgetrc", Facet: FacetAll},
		{Src: "shellcheck/shellcheckrc", Dest: ".shellcheckrc", Facet: FacetAll},
		{Src: "starship.toml", Dest: ".config/starship.toml", Facet: FacetAll},
		{Src: "nvim", Dest: ".config/nvim", IsDir: true, Facet: FacetAll},
		{Src: "wezterm", Dest: ".config/wezterm", IsDir: true, Facet: FacetAll},
		{Src: "ghostty", Dest: ".config/ghostty", IsDir: true, Facet: FacetAll},
		{Src: "mise", Dest: ".config/mise", IsDir: true, Facet: FacetAll},
		{Src: "alacritty", Dest: ".config/alacritty", IsDir: true, Facet: FacetAll},
		{Src: "blesh", Dest: ".config/blesh", IsDir: true, Facet: FacetAll},
		{Src: "aerospace", Dest: ".config/aerospace", IsDir: true, Facet: FacetMacOS},
		{Src: "herdr/config.toml", Dest: ".config/herdr/config.toml", Facet: FacetUnix},
		{Src: "herdr-app/keys.json", Dest: ".config/herdr-app/keys.json", Facet: FacetUnix},
		{Src: "herdr-app/settings.json", Dest: ".config/herdr-app/settings.json", Facet: FacetUnix},
		{Src: "herdr-app/themes/catppuccin-mocha.json", Dest: ".config/herdr-app/themes/catppuccin-mocha.json", Facet: FacetUnix},
		{Src: "herdr-app/mods/prefix", Dest: ".config/herdr-app/mods/prefix", IsDir: true, Facet: FacetUnix},
		{Src: "herdr-app/mods/broadcast", Dest: ".config/herdr-app/mods/broadcast", IsDir: true, Facet: FacetUnix},
		{Src: "espanso", Dest: ".config/espanso", IsDir: true, Facet: FacetUnix},
		{Src: "hypr", Dest: ".config/hypr", IsDir: true, Facet: FacetHyprland},
		{Src: "waybar", Dest: ".config/waybar", IsDir: true, Facet: FacetHyprland},
		{Src: "swayosd", Dest: ".config/swayosd", IsDir: true, Facet: FacetHyprland},
		{Src: "mako", Dest: ".config/mako", IsDir: true, Facet: FacetHyprland},
		{Src: "wlogout", Dest: ".config/wlogout", IsDir: true, Facet: FacetHyprland},
		{Src: "fuzzel", Dest: ".config/fuzzel", IsDir: true, Facet: FacetHyprland},
		{Src: "satty", Dest: ".config/satty", IsDir: true, Facet: FacetHyprland},
		{Src: "launchers/applications/alacritty-float.desktop", Dest: ".local/share/applications/alacritty-float.desktop", Facet: FacetHyprland},
		{Src: "launchers/applications/arch-update.desktop", Dest: ".local/share/applications/arch-update.desktop", Facet: FacetArchDesktop},
		{Src: "launchers/applications/gedit-float.desktop", Dest: ".local/share/applications/gedit-float.desktop", Facet: FacetHyprland},
		{Src: "launchers/applications/wifi-tui.desktop", Dest: ".local/share/applications/wifi-tui.desktop", Facet: FacetHyprland},
		{Src: "common/systemd/user/tmux.service", Dest: ".config/systemd/user/tmux.service", Facet: FacetLinux},
		{Src: "bin", Dest: ".bin", IsDir: true, Facet: FacetAll},
		{Src: "ssh/config", Dest: ".ssh/config", Facet: FacetAll},
		{Src: "ansible.cfg", Dest: ".ansible.cfg", Facet: FacetAll},
		{Src: "dotenv", Dest: ".config/direnv", IsDir: true, Facet: FacetAll},
		{Src: "branchlet/settings.json", Dest: ".branchlet/settings.json", Facet: FacetAll},
		{Src: "atuin/config.toml", Dest: ".config/atuin/config.toml", Facet: FacetAll},
		{Src: "lazygit/config.yml", Dest: ".config/lazygit/config.yml", Facet: FacetAll},
		{Src: "zed/keymap.json", Dest: ".config/zed/keymap.json", Facet: FacetAll},
		{Src: "zed/settings.json", Dest: ".config/zed/settings.json", Copy: true, Facet: FacetAll},
	}

	got := DefaultTable()
	if len(got) != len(want) {
		t.Fatalf("len(DefaultTable()) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestApplies(t *testing.T) {
	tests := []struct {
		name  string
		facet Facet
		os    platform.OS
		ld    bool
		want  bool
	}{
		{"all-on-debian", FacetAll, platform.OSDebian, false, true},
		{"all-on-macos", FacetAll, platform.OSMacOS, false, true},
		{"macos-on-macos", FacetMacOS, platform.OSMacOS, false, true},
		{"macos-on-arch", FacetMacOS, platform.OSArch, false, false},
		{"arch-on-arch", FacetArch, platform.OSArch, false, true},
		{"arch-on-debian", FacetArch, platform.OSDebian, false, false},
		{"windows-on-windows", FacetWindows, platform.OSWindows, false, true},
		{"windows-on-linux", FacetWindows, platform.OSLinux, false, false},
		{"desktop-arch-ld-on", FacetHyprland, platform.OSArch, true, true},
		{"desktop-arch-ld-off", FacetHyprland, platform.OSArch, false, false},
		{"desktop-debian-ld-on", FacetHyprland, platform.OSDebian, true, true},
		{"desktop-macos-ld-on", FacetHyprland, platform.OSMacOS, true, false},
		{"desktop-generic-linux-ld-on", FacetHyprland, platform.OSLinux, true, false},
		{"linux-on-redhat", FacetLinux, platform.OSRedHat, false, true},
		{"linux-on-arch", FacetLinux, platform.OSArch, false, true},
		{"linux-on-macos", FacetLinux, platform.OSMacOS, false, false},
		{"linux-on-generic-linux", FacetLinux, platform.OSLinux, false, false},
		{"unix-on-macos", FacetUnix, platform.OSMacOS, false, true},
		{"unix-on-debian", FacetUnix, platform.OSDebian, false, true},
		{"unix-on-generic-linux", FacetUnix, platform.OSLinux, false, true},
		{"unix-on-wsl", FacetUnix, platform.OSWSL, false, true},
		{"unix-on-windows", FacetUnix, platform.OSWindows, false, false},
		{"hyprland-on-redhat-ld-on", FacetHyprland, platform.OSRedHat, true, true},
		{"archdesktop-on-arch-ld-on", FacetArchDesktop, platform.OSArch, true, true},
		{"archdesktop-on-arch-ld-off", FacetArchDesktop, platform.OSArch, false, false},
		{"archdesktop-on-redhat-ld-on", FacetArchDesktop, platform.OSRedHat, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := LinkSpec{Facet: tt.facet}
			info := platform.Info{OS: tt.os}
			opts := Options{ManageHyprland: tt.ld}
			if got := applies(spec, info, opts); got != tt.want {
				t.Errorf("applies(facet=%d, os=%s, ld=%t) = %t, want %t",
					tt.facet, tt.os, tt.ld, got, tt.want)
			}
		})
	}
}

// TestFaceting_WSL documents the decision to treat OSWSL as Linux-family so a
// WSL host lacking /etc/os-release still gets the Linux desktop set (parity).
func TestFaceting_WSL(t *testing.T) {
	if !isLinuxFamily(platform.OSWSL) {
		t.Error("isLinuxFamily(OSWSL) = false, want true (WSL treated as Linux-family)")
	}
	spec := LinkSpec{Facet: FacetHyprland}
	info := platform.Info{OS: platform.OSWSL}
	if !applies(spec, info, Options{ManageHyprland: true}) {
		t.Error("FacetHyprland does not apply on OSWSL with --hyprland, want it to apply")
	}
}
