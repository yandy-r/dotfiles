package sysconfig

import "strings"

// hostsLoopbackName is the Debian-convention loopback IP that carries the
// static hostname in /etc/hosts (distinct from 127.0.0.1 localhost).
const hostsLoopbackName = "127.0.1.1"

// hostsNeedsUpdate reports whether content lacks a canonical
// "127.0.1.1 <name>" line — i.e. whether renderHosts would change anything.
func hostsNeedsUpdate(content []byte, name string) bool {
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == hostsLoopbackName && fields[1] == name {
			return false
		}
	}
	return true
}

// renderHosts returns content with a canonical "127.0.1.1 <name>" line
// ensured. An existing 127.0.1.1 line is replaced in place (not duplicated);
// all other lines — including "127.0.0.1 localhost" — are preserved. The
// trailing newline layout of content is preserved.
func renderHosts(content []byte, name string) []byte {
	desired := hostsLoopbackName + "\t" + name

	lines := strings.Split(string(content), "\n")
	out := make([]string, 0, len(lines)+1)
	replaced := false

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 1 && fields[0] == hostsLoopbackName {
			if !replaced {
				out = append(out, desired)
				replaced = true
			}
			// Drop any further duplicate 127.0.1.1 lines.
			continue
		}
		out = append(out, line)
	}

	if !replaced {
		if n := len(out); n > 0 && out[n-1] == "" {
			// Insert before the trailing empty element so a trailing newline
			// is preserved.
			out = append(out[:n-1], desired, "")
		} else {
			out = append(out, desired)
		}
	}

	return []byte(strings.Join(out, "\n"))
}
