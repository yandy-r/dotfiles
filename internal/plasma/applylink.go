package plasma

// This file is the LINK-MODE half of apply planning: the --link/--unlink
// decision for one file, the guard that decides whether a mode transition is
// refused, and the §9.2 state matrix the two rest on. It lives apart from
// apply.go because it is a closed decision surface — every cell is a contract
// with the CLI's exit code — and because apply.go is the merge path, which
// reads better without a second state machine interleaved into it.

import "os"

// resolveLinkGuard is the LAST step of planning a file whose requested mode is a
// transition the merge path had to plan before the guard could judge it — a
// plain apply over a foreign symlink writes through it, and only a write is a
// transition worth refusing.
//
// It returns the action to plan and whether the guard has anything to say about
// this file. A conflicting file whose planned action turns out to be ActionSkip
// yields no conflict at all.
func resolveLinkGuard(act FileAction, conflict, guardSatisfied bool) (FileAction, bool, error) {
	if !conflict || !mutatesLive(act.Kind) {
		return act, false, nil
	}
	if guardSatisfied {
		return act, true, nil
	}
	return FileAction{
		Spec: act.Spec, Kind: ActionSkip, SrcAbs: act.SrcAbs, DestAbs: act.DestAbs,
		Mode: act.Mode, Reason: "link-mode conflict",
	}, true, nil
}

// linkVerdict is planLinkMode's answer for one file.
type linkVerdict struct {
	// Act is the possibly-updated action.
	Act FileAction
	// Conflict reports that the guard has something to say about this file:
	// either it refused the transition (Act is then a "link-mode conflict" skip)
	// or --backup/--dry-run authorised one.
	Conflict bool
	// Decided reports that Act is FINAL. False means "carry on with the normal
	// merge path", which is also the only way the merge path's own outcome can
	// feed back into the guard via resolveLinkGuard.
	Decided bool
	// Unlink reports that the live path is a symlink this run replaces with a
	// real file. It travels separately from Act.Kind because the merge path is
	// what computes the content, and it is what suppresses that path's
	// convergence short-circuits.
	Unlink bool
}

// planLinkMode resolves the --link/--unlink decision for one file.
//
// A file with no plasma/files/ copy is never linkable: there is no target, and
// a symlink to a non-existent path is a dangling link KDE would silently treat
// as an empty config.
//
// Two of the three transitions can be judged here because their action is known
// without planning it — a symlink replace and a symlink materialisation both
// always write. Only linkModeWrite defers, because whether a plain apply over a
// foreign symlink writes at all depends on what the merge produces.
func (e *Engine) planLinkMode(
	act FileAction,
	spec FileSpec,
	opts Options,
	guardSatisfied, repoPresent bool,
) linkVerdict {
	if !repoPresent {
		if opts.Link {
			act.Kind = ActionSkip
			act.Reason = "nothing to apply"
			return linkVerdict{Act: act, Decided: true}
		}
		return linkVerdict{Act: act}
	}

	mode, conflict := e.linkMode(e.livePath(spec), e.repoPath(spec), opts.Link, opts.Unlink)
	refuse := func() linkVerdict {
		act.Kind = ActionSkip
		act.Reason = "link-mode conflict"
		return linkVerdict{Act: act, Conflict: true, Decided: true}
	}

	switch mode {
	case linkModeAlreadyLinked:
		act.Kind = ActionSkip
		act.Reason = "already linked; the repo file is the live file (--unlink restores copy mode)"
		return linkVerdict{Act: act, Decided: true}
	case linkModeSymlink:
		if conflict && !guardSatisfied {
			return refuse()
		}
		act.Kind = ActionSymlink
		act.Reason = "link the live path into the repo"
		return linkVerdict{Act: act, Conflict: conflict, Decided: true}
	case linkModeUnlink:
		if conflict && !guardSatisfied {
			return refuse()
		}
		return linkVerdict{Act: act, Conflict: conflict, Unlink: true}
	}
	return linkVerdict{Act: act, Conflict: conflict}
}

// linkModeDecision is the outcome of inspecting a destination against --link.
type linkModeDecision int

const (
	// linkModeWrite: proceed with the normal merge/create path.
	linkModeWrite linkModeDecision = iota
	// linkModeSymlink: replace the destination with a symlink into the repo.
	linkModeSymlink
	// linkModeAlreadyLinked: the destination is already the right symlink.
	linkModeAlreadyLinked
	// linkModeUnlink: replace the destination symlink with a real file
	// (--unlink), restoring copy mode.
	linkModeUnlink
)

// linkMode implements the §9.2 matrix, with --unlink as --link's inverse:
//
//	live state                  | plain            | --link              | --unlink
//	regular file                | write            | symlink (conflict)  | write
//	symlink into plasma/files/  | already linked   | already linked      | unlink
//	symlink elsewhere           | write (conflict) | symlink (conflict)  | unlink (conflict)
//	absent                      | write            | symlink             | write
//
// The returned decision is always the mode the caller ASKED for; the boolean
// only says whether performing it is a transition. That split is what lets
// --backup and --dry-run authorise the switch (§9.3) instead of merely
// downgrading the refusal into a silent skip.
//
// Two cells earn their asymmetry. A plain apply over a file linked INTO the
// repo is not a conflict and not a write: the repo file IS the live file, so
// there is nothing to converge. And --unlink over a
// repo link is not a conflict either — it discards nothing (the bytes written
// are the ones KDE was already reading, and the repo copy is untouched) and the
// user asked for it by name, so demanding --backup on top would be ceremony.
// The copy → link direction keeps its conflict: it DOES discard every live-only
// key the merge preserves.
func (e *Engine) linkMode(livePath, repoPath string, wantLink, wantUnlink bool) (linkModeDecision, bool) {
	fi, err := os.Lstat(livePath)
	switch {
	case err != nil:
		// Absent: there is no link to undo, so --unlink is an ordinary write.
		if wantLink {
			return linkModeSymlink, false
		}
		return linkModeWrite, false
	case fi.Mode()&os.ModeSymlink == 0:
		if wantLink {
			// Linking would discard every live-only key the merge preserves.
			return linkModeSymlink, true
		}
		return linkModeWrite, false
	}

	target, rerr := os.Readlink(livePath)
	if rerr != nil || target != repoPath {
		// A symlink pointing somewhere else is never ours to reinterpret, in
		// any mode.
		switch {
		case wantLink:
			return linkModeSymlink, true
		case wantUnlink:
			return linkModeUnlink, true
		}
		return linkModeWrite, true
	}
	if wantUnlink {
		return linkModeUnlink, false
	}
	return linkModeAlreadyLinked, false
}
