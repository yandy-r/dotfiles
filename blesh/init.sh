# shellcheck shell=bash
# ================================================================
# ble.sh user configuration (~/.config/blesh/init.sh)
# ================================================================
# Loaded by ble.sh itself (bash/bashrc sources ble.sh). Styled to match the
# Catppuccin Mocha powerline segments in starship.toml.
# Reference: https://github.com/akinomyoga/ble.sh/blob/master/blerc.template

# ----------------------------------------------------------------
# Vi mode indicator: a colored powerline pill instead of "-- INSERT --"
# ----------------------------------------------------------------

# \q{dotf/vi-mode} — prompt escape used by bleopt prompt_vi_mode_indicator.
# get-mode yields i (insert), R/^R (replace), n (normal), v/V/^V (visual),
# s/S/^S (select); a leading i/R before n/v marks an i_CTRL-O one-shot command.
function ble/prompt/backslash:dotf/vi-mode {
	local mode
	ble/keymap:vi/script/get-mode
	local name rgb
	case $mode in
	i) name=INSERT rgb='166;227;161' ;;   # green
	R) name=REPLACE rgb='243;139;168' ;;  # red
	$'\x12') name=V-REPLACE rgb='243;139;168' ;;
	*n) name=NORMAL rgb='137;180;250' ;;  # blue
	*v) name=VISUAL rgb='203;166;247' ;;  # mauve
	*V) name=V-LINE rgb='203;166;247' ;;
	*$'\x16') name=V-BLOCK rgb='203;166;247' ;;
	*s) name=SELECT rgb='250;179;135' ;;  # peach
	*S) name=S-LINE rgb='250;179;135' ;;
	*$'\x13') name=S-BLOCK rgb='250;179;135' ;;
	*) return 0 ;;
	esac
	[[ $mode == [iR]?* ]] && name="($name)"

	# Rounded powerline caps U+E0B6/U+E0B4 (Nerd Font), as UTF-8 bytes.
	local lcap=$'\xee\x82\xb6' rcap=$'\xee\x82\xb4'
	local fg=$'\e[38;2;'$rgb'm' bg=$'\e[48;2;'$rgb'm' dark=$'\e[1;38;2;17;17;27m' reset=$'\e[m'
	ble/prompt/print "$fg$lcap$reset$bg$dark $name $reset$fg$rcap$reset"
}

# Vi options only exist once ble.sh loads its vi keymap, so they are applied
# from this hook. Cursor shape per mode (DECSCUSR: 2 block, 4 underline, 6 bar).
function dotf/blesh/vi-load-hook {
	bleopt prompt_vi_mode_indicator='\q{dotf/vi-mode}'
	ble-bind -m vi_imap --cursor 6
	ble-bind -m vi_nmap --cursor 2
	ble-bind -m vi_omap --cursor 4
	ble-bind -m vi_xmap --cursor 2
	ble-bind -m vi_smap --cursor 2
	ble-bind -m vi_cmap --cursor 0
}
blehook/eval-after-load keymap_vi dotf/blesh/vi-load-hook
