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
	# The pill lives in the prompt (see dotf/blesh/vi-mode-in-prompt), so the
	# bottom-line indicator is cleared and the prompt redraws on mode changes.
	bleopt prompt_vi_mode_indicator=
	bleopt keymap_vi_mode_update_prompt=1
	ble-bind -m vi_imap --cursor 6
	ble-bind -m vi_nmap --cursor 2
	ble-bind -m vi_omap --cursor 4
	ble-bind -m vi_xmap --cursor 2
	ble-bind -m vi_smap --cursor 2
	ble-bind -m vi_cmap --cursor 0

	# Enter runs the command even in multiline mode, unless it is incomplete
	# (open quote, trailing \, && or |), where it inserts a newline instead.
	# C-j inserts a newline explicitly (upstream default: C-j accepts, RET
	# inserts a newline once the buffer spans multiple lines).
	local keymap
	for keymap in vi_imap vi_nmap; do
		ble-bind -m "$keymap" -f 'C-m' 'accept-line syntax'
		ble-bind -m "$keymap" -f 'RET' 'accept-line syntax'
	done
	ble-bind -m vi_imap -f 'C-j' 'newline'
}
blehook/eval-after-load keymap_vi dotf/blesh/vi-load-hook

# ----------------------------------------------------------------
# Vi mode pill inside the prompt's segment line
# ----------------------------------------------------------------
# Starship rebuilds PS1 in its own PRECMD hook, so after it runs, insert
# \q{dotf/vi-mode} (expanded by ble.sh at every redraw) before the last newline
# (the end of the segment line, just above the ❯ character line). A one-line
# PS1 gets the pill prepended instead.
function dotf/blesh/vi-mode-in-prompt {
	local tag='\q{dotf/vi-mode}' nl=$'\n'
	[[ $PS1 == *"$tag"* ]] && return 0
	if [[ $PS1 == *"$nl"* ]]; then
		PS1="${PS1%"$nl"*} $tag$nl${PS1##*"$nl"}"
	else
		PS1="$tag $PS1"
	fi
}
# Registered at attach time, i.e. after bashrc's starship init, so it runs
# after starship_precmd in the PRECMD chain.
function dotf/blesh/register-prompt-hook {
	blehook PRECMD+=dotf/blesh/vi-mode-in-prompt
}
blehook ATTACH+=dotf/blesh/register-prompt-hook
