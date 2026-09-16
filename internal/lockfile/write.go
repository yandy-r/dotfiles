package lockfile

import (
	"fmt"
	"io"
	"sort"

	toml "github.com/pelletier/go-toml/v2"
)

// Write emits l as deterministic TOML: packages sorted by name and each
// package's artifacts sorted by os then arch. The ordering is stable so a
// re-lock of a converged repo produces byte-identical output (the diff a CI
// freshness gate relies on) and Parse(Write(l)) round-trips l. It sorts a copy,
// never mutating the caller's Lock.
func Write(w io.Writer, l *Lock) error {
	out := Lock{Version: l.Version}
	out.Packages = append(out.Packages, l.Packages...)
	sort.Slice(out.Packages, func(i, j int) bool {
		return out.Packages[i].Name < out.Packages[j].Name
	})
	for i := range out.Packages {
		arts := append([]Artifact(nil), out.Packages[i].Artifacts...)
		sort.Slice(arts, func(a, b int) bool {
			if arts[a].OS != arts[b].OS {
				return arts[a].OS < arts[b].OS
			}
			return arts[a].Arch < arts[b].Arch
		})
		out.Packages[i].Artifacts = arts
	}

	enc := toml.NewEncoder(w)
	enc.SetIndentTables(true)
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("lockfile: encode: %w", err)
	}
	return nil
}
