package plasma

// Validate is the post-apply audit: it re-reads both sides and reports whether
// every key the repo asserts actually landed.
//
// It is a thin caller of Diff — deliberately NOT a second implementation — so
// the validated set can never drift from the applied set. A second comparator
// would be free to disagree with the one that planned the run, and a
// post-apply audit that disagrees with the plan is worse than no audit.
//
// It is not gated by Options.DryRun: it reports actual on-disk state. Callers
// that ran a dry run simply do not call it. Like Diff it returns the full
// results alongside a *DriftError.
func (e *Engine) Validate(files []FileSpec, opts Options) ([]FileDiff, error) {
	return e.Diff(files, opts)
}
