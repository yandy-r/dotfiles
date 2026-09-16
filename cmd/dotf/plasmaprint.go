package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/yandy-r/dotfiles/internal/plasma"
)

// This file is the KEY=VALUE half of `dotf plasma`'s rendering: token encoding
// and the line builders. The `--json` document shape lives in plasmajson.go,
// the command dispatch and flag surface in plasma.go, and the per-verb
// orchestration in plasmaverbs.go, so every one of them stays well under the
// ~500-line soft cap as the printers grow.
//
// Every printer here is a pure function from engine values to lines. Nothing
// reads the filesystem, the environment, or the clock, so the whole output
// contract is table-testable without an Engine.

// plasmaToken percent-encodes one token embedded in a `plasma.*` value.
//
// KConfig file, group, and key names may legally contain the very bytes that
// delimit this CLI's contract — `,` separates list items and `=` separates the
// key from the value — and a group name may even carry a control byte. Encoding
// exactly `%`, `,`, `=`, and bytes below 0x20 (and nothing else) keeps the
// common case byte-identical to the raw name while making the line
// unambiguously machine-parseable. `%` is encoded first in the switch so the
// escape is self-describing and decodable.
func plasmaToken(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '%':
			b.WriteString("%25")
		case c == ',':
			b.WriteString("%2C")
		case c == '=':
			b.WriteString("%3D")
		case c < 0x20:
			fmt.Fprintf(&b, "%%%02X", c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// plasmaGroupHeader formats a decoded group path as the header text a human
// reads: "[Desktops]", "[Tiling][x][y]", or "[<default>]" for the root group.
//
// It intentionally re-implements internal/plasma's unexported renderGroup
// rather than exporting it: the engine's copy exists for error messages, and
// widening the package's API surface for one three-line formatter would tie the
// CLI's stdout contract to an engine detail.
func plasmaGroupHeader(path []string) string {
	if len(path) == 0 {
		return "[<default>]"
	}
	var b strings.Builder
	for _, seg := range path {
		b.WriteByte('[')
		b.WriteString(seg)
		b.WriteByte(']')
	}
	return b.String()
}

// plasmaKeyRef renders one key identity as "<file>:<group>/<key>", the atom of
// the plasma.change=/plasma.drift=/plasma.only-live= lines. A locale variant
// renders as "<key>[<locale>]" so two variants of the same key never collapse
// into one indistinguishable entry. Each of the three tokens is encoded
// separately, so the ":" and "/" structure survives a name containing them.
func plasmaKeyRef(file string, group []string, key, locale string) string {
	name := key
	if locale != "" {
		name = key + "[" + locale + "]"
	}
	return plasmaToken(file) + ":" + plasmaToken(plasmaGroupHeader(group)) + "/" + plasmaToken(name)
}

// plasmaBucketOrder is the fixed emission order of the plasma.<bucket>= lines.
// The names are exactly plasma.ActionKind.String()'s outputs, so the bucket is
// read straight off FileAction.Kind — never switched off FileAction.Reason,
// which is human-readable prose the renderer must not depend on.
var plasmaBucketOrder = []string{
	"export", "prune", "create", "merge", "copy", "link", "unlink", "backup", "skip", "unknown",
}

// plasmaActionLines groups actions by kind and returns one sorted
// "prefix.bucket=path,path,..." line per non-empty bucket, in plasmaBucketOrder.
// It is the single formatter shared by `dotf plasma`'s own output and
// `dotf apply --dry-run`'s plasma section, so the two can never disagree.
func plasmaActionLines(prefix string, actions []plasma.FileAction) []string {
	buckets := make(map[string][]string, len(plasmaBucketOrder))
	for _, a := range actions {
		bucket := a.Kind.String()
		buckets[bucket] = append(buckets[bucket], plasmaToken(a.Spec.Path))
	}

	lines := make([]string, 0, len(plasmaBucketOrder))
	for _, bucket := range plasmaBucketOrder {
		paths := buckets[bucket]
		if len(paths) == 0 {
			continue
		}
		sort.Strings(paths)
		lines = append(lines, fmt.Sprintf("%s.%s=%s", prefix, bucket, strings.Join(paths, ",")))
	}
	return lines
}

// plasmaChangeLine renders the plasma.change= line: every (group, key, locale)
// the run will write, across every file, sorted. It returns "" when the run
// writes no key, so a converged apply emits no line at all rather than an empty
// one.
func plasmaChangeLine(actions []plasma.FileAction) string {
	var refs []string
	for _, a := range actions {
		for _, k := range a.Keys {
			refs = append(refs, plasmaKeyRef(a.Spec.Path, k.Group, k.Key, k.Locale))
		}
	}
	return plasmaJoinRefs("plasma.change", refs)
}

// plasmaDiffLines renders diff's key detail as ONE LINE PER DIFFERENCE, split
// across three keys:
//
//   - plasma.drift= — the differences apply would act on.
//   - plasma.only-live= — keys that exist only on the live side and that
//     nothing in the strip table explains, i.e. genuinely UNMANAGED state.
//     Informational: apply never deletes a live-only key.
//   - plasma.expected-volatile= — keys that are live-only precisely BECAUSE
//     this file's own strip rules remove them on export, so their absence from
//     plasma/files/ is the design working. The engine suppresses these unless
//     --show-volatile is given (Options.ShowVolatile), so on the default path
//     this bucket is empty and the line never appears.
//
// The third bucket is not cosmetic either: a real desktop carries hundreds of
// [Tiling][<uuid>] and Session* entries that are volatile by construction, and
// folding them in with the unmanaged keys made a single genuinely-unmanaged key
// unfindable. Bucketing on DiffKind — never on a count or a name pattern —
// keeps the classification the engine's, not the renderer's.
//
// Per-line is not cosmetic. A real desktop routinely carries hundreds of
// live-only keys, and comma-joining them produced a single multi-kilobyte line
// that no terminal could display and no `grep` could narrow. Repeating the key
// keeps every line independently greppable ("grep '^plasma.drift=' | wc -l" is
// the drift count) while staying inside the same key=value contract as
// plasma.reload.<n>=, which is likewise one line per item.
//
// Each line reads "<key>=<file>:<group>/<key> <attr>=<value>…": the reference
// first so lines sort by identity, then the engine's classification of that
// entry (see plasmaKeyAttrs).
func plasmaDiffLines(diffs []plasma.FileDiff) []string {
	var drift, onlyLive, volatile []string
	for _, fd := range diffs {
		for _, k := range fd.Keys {
			entry := plasmaKeyRef(fd.Spec.Path, k.Group, k.Key, k.Locale) + plasmaKeyAttrs(k)
			switch k.Kind {
			case plasma.DiffMissingRepo:
				onlyLive = append(onlyLive, entry)
			case plasma.DiffExpectedVolatile:
				volatile = append(volatile, entry)
			default:
				drift = append(drift, entry)
			}
		}
	}
	lines := plasmaEntryLines("plasma.drift", drift)
	lines = append(lines, plasmaEntryLines("plasma.only-live", onlyLive)...)
	return append(lines, plasmaEntryLines("plasma.expected-volatile", volatile)...)
}

// plasmaKeyAttrs renders one diff entry's trailing attributes as a leading-space
// run of "name=value" pairs.
//
// Every fact the engine reports ABOUT a difference — as opposed to its identity,
// which plasmaKeyRef owns — belongs in this one slice, so widening the line is a
// single append here and touches neither the line assembly, the sort, nor the
// bucket split. The expected-vs-unmanaged classification of a live-only key
// needed no widening in the end: the engine expresses it as a DiffKind, so
// kind=expected-volatile carries it and the bucket split above reads the same
// field.
func plasmaKeyAttrs(k plasma.KeyDiff) string {
	attrs := []string{"kind=" + plasmaToken(k.Kind.String())}
	return " " + strings.Join(attrs, " ")
}

// plasmaEntryLines sorts entries and renders one "key=entry" line each, or
// returns nil when there is nothing to say. Sorting the RENDERED entries (not
// the underlying diffs) makes the block stable regardless of the managed set's
// table ordering.
func plasmaEntryLines(key string, entries []string) []string {
	if len(entries) == 0 {
		return nil
	}
	sort.Strings(entries)
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = key + "=" + e
	}
	return lines
}

// plasmaJoinRefs sorts and comma-joins key references into a "key=..." line, or
// returns "" when there is nothing to say. Sorting the RENDERED references (not
// the underlying actions) makes the line stable regardless of the plan's
// ApplyOrder-driven file ordering.
func plasmaJoinRefs(key string, refs []string) string {
	if len(refs) == 0 {
		return ""
	}
	sort.Strings(refs)
	return key + "=" + strings.Join(refs, ",")
}

// plasmaStatusLines renders the status verb's summary block. The two "missing"
// lines are omitted when empty so a fully-provisioned host emits a short,
// all-counters block.
func plasmaStatusLines(st *plasma.StatusReport) []string {
	lines := []string{
		fmt.Sprintf("plasma.managed=%d", st.Managed),
		fmt.Sprintf("plasma.repo-present=%d", st.RepoPresent),
		fmt.Sprintf("plasma.live-present=%d", st.LivePresent),
	}
	if line := plasmaJoinRefs("plasma.missing-live", plasmaTokens(st.MissingLive)); line != "" {
		lines = append(lines, line)
	}
	if line := plasmaJoinRefs("plasma.missing-repo", plasmaTokens(st.MissingRepo)); line != "" {
		lines = append(lines, line)
	}
	// Emitted for the same reason the JSON row carries action=absent: a file
	// present on neither side is a real state, and leaving it out of the text
	// block while the JSON called it "present" made the two renderers disagree
	// about six of the twenty-seven managed files on a fresh host.
	if line := plasmaJoinRefs("plasma.absent", plasmaTokens(st.Absent)); line != "" {
		lines = append(lines, line)
	}
	return append(lines,
		fmt.Sprintf("plasma.drift.files=%d", len(st.DriftedFiles)),
		fmt.Sprintf("plasma.drift.keys=%d", st.DriftedKeys),
	)
}

// plasmaTokens encodes every element of paths for embedding in a list value.
func plasmaTokens(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = plasmaToken(p)
	}
	return out
}

// plasmaReloadLines renders "plasma.reload.<n>=<argv>" lines in execution order,
// n starting at 1. apply emits them whether or not --reload was given: without
// the flag they are the commands the run deliberately declined to execute, which
// keeps the knowledge in the tool instead of a README.
// A skipped command is rendered as a reason plus a note instead of a numbered
// command, because printing it as plasma.reload.<n>= would advertise a command
// that cannot work — which is the exact failure this reporting exists to end.
// Numbering stays over the runnable commands only, so the numbered lines remain
// a copy-pasteable sequence.
func plasmaReloadLines(cmds []plasma.ReloadCommand) []string {
	lines := make([]string, 0, len(cmds))
	n := 0
	for _, c := range cmds {
		if c.Skipped {
			lines = append(lines, fmt.Sprintf("plasma.reload.skipped=%s reason=%s",
				c.Component, c.SkipReason))
			if c.Note != "" {
				lines = append(lines, "plasma.reload.note="+c.Note)
			}
			continue
		}
		n++
		lines = append(lines, fmt.Sprintf("plasma.reload.%d=%s", n,
			strings.Join(plasmaTokens(c.Argv), " ")))
	}
	return lines
}

// writePlasmaLines emits the assembled lines with ONE checked write, matching
// printLinkDryRun's discipline so a broken stdout surfaces rather than being
// silently dropped. A printer with nothing to say writes nothing at all.
func writePlasmaLines(stdout io.Writer, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return err
}

// printPlasmaSummary writes the mutating run's aggregate counts to stderr,
// mirroring printLinkSummary.
func printPlasmaSummary(stderr io.Writer, applied, planned int) {
	_, _ = fmt.Fprintf(stderr, "%s: plasma summary: applied=%d planned=%d\n",
		progName, applied, planned)
}

// printPlasmaFailures writes one progName-prefixed line per non-fatal failure,
// after the summary. Every engine error already begins with "plasma: " and
// embeds the offending path, so the rendered line reads
// "dotf: plasma: <path>: <detail>" without the CLI having to re-attribute an
// error to a file it cannot reliably pair it with.
func printPlasmaFailures(stderr io.Writer, errs []error) {
	for _, err := range errs {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
	}
}

// plasmaErrors flattens a joined error into its leaves so each non-fatal
// failure gets its own stderr line. Execute and Reload both return
// errors.Join's multi-error, whose Unwrap() []error is the only way to reach the
// individual causes.
func plasmaErrors(err error) []error {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		return joined.Unwrap()
	}
	return []error{err}
}

// plasmaLinkConflictLines renders the preview's refusal warning: the
// machine-readable path list plus the exact sentence the real run would print.
// Both come from the engine's own *LinkModeConflictError, so the preview cannot
// promise a different refusal from the one that will actually happen.
//
// It goes to STDOUT rather than the advisory stream because a dry run's stderr
// is asserted empty by contract — the preview channel is where a preview's bad
// news belongs.
func plasmaLinkConflictLines(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	err := &plasma.LinkModeConflictError{Paths: paths}
	return []string{
		"plasma.link-conflict=" + strings.Join(plasmaTokens(paths), ","),
		"plasma.link-conflict.refusal=" + plasmaToken(err.Error()),
	}
}
