package sysconfig

import "strings"

// normalizeLocale canonicalizes a locale string for comparison: it lowercases,
// strips all whitespace, and treats "utf-8" == "utf8", so
// "en_us.utf-8" == "en_US.UTF-8". This mirrors the case/hyphen-insensitive
// matching common/exports.sh:locale_exists performs (see WS-SHELL / ADR-14,
// which fixed the shell side to collapse the codeset separator the same way).
func normalizeLocale(s string) string {
	n := strings.ToLower(s)
	n = strings.Join(strings.Fields(n), "")
	n = strings.ReplaceAll(n, "utf-8", "utf8")
	return n
}

// localeAvailable reports whether target appears (normalized) in the output of
// `locale -a`.
func localeAvailable(out []byte, target string) bool {
	want := normalizeLocale(target)
	if want == "" {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if normalizeLocale(line) == want {
			return true
		}
	}
	return false
}

// isBuiltinLocale reports whether s is a built-in locale (C, C.UTF-8, or
// POSIX) that never needs generating via locale-gen.
func isBuiltinLocale(s string) bool {
	switch normalizeLocale(s) {
	case "c", "c.utf8", "posix":
		return true
	default:
		return false
	}
}

// resolveLocale picks the best AVAILABLE locale to fall back to when the
// requested locale cannot be honored, using the precedence
// requested → en_US.UTF-8 → C.UTF-8 → C, mirroring the fallback precedence of
// common/exports.sh's pick_dotfiles_locale. C is the always-available last
// resort.
func resolveLocale(requested string, avail []byte) string {
	for _, candidate := range []string{requested, "en_US.UTF-8", "C.UTF-8"} {
		if candidate != "" && localeAvailable(avail, candidate) {
			return candidate
		}
	}
	return "C"
}

// uncommentLocaleGen returns content with the commented "#<target> <charset>"
// line in an /etc/locale.gen body uncommented, plus whether a change was made.
// Arch's /etc/locale.gen ships locale lines commented out; locale-gen only
// generates uncommented entries. Matching is normalized on the locale name.
func uncommentLocaleGen(content []byte, target string) ([]byte, bool) {
	want := normalizeLocale(target)
	lines := strings.Split(string(content), "\n")
	changed := false

	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		uncommented := strings.TrimSpace(strings.TrimLeft(line, "#"))
		fields := strings.Fields(uncommented)
		if len(fields) >= 1 && normalizeLocale(fields[0]) == want {
			lines[i] = uncommented
			changed = true
		}
	}

	if !changed {
		return content, false
	}
	return []byte(strings.Join(lines, "\n")), true
}

// langpackFor returns the Fedora/RHEL glibc langpack package name for locale
// ("de_DE.UTF-8" -> "glibc-langpack-de"), or "" when locale has no language
// prefix or is a built-in (C / C.UTF-8 / POSIX) that needs no package.
func langpackFor(locale string) string {
	if isBuiltinLocale(locale) {
		return ""
	}
	n := normalizeLocale(locale)
	lang := n
	if i := strings.IndexAny(lang, "_.@"); i >= 0 {
		lang = lang[:i]
	}
	if lang == "" {
		return ""
	}
	for _, ch := range lang {
		if ch < 'a' || ch > 'z' {
			return ""
		}
	}
	return "glibc-langpack-" + lang
}
