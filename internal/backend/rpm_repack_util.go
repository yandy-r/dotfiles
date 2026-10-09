package backend

import "slices"

// uniqSorted returns v without duplicates, sorted, so an order change or a
// repeated dep in the rebuilt header is not mistaken for a metadata change.
func uniqSorted(v []string) []string {
	out := slices.Clone(v)
	slices.Sort(out)
	return slices.Compact(out)
}
