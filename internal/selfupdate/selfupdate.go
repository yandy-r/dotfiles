// Package selfupdate discovers the latest published dotf release, verifies its
// tar.gz against the release checksums.txt with the SAME SHA-256 contract
// bootstrap.sh uses, and atomically replaces the running binary. It is scoped to
// the dotf binary only — it never resolves the manifest or touches
// manifest-declared tools (issue #53).
package selfupdate

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/version"
)

// defaultRepo is the release source. Releases are published to github by
// GoReleaser (.goreleaser.yml release.github), so self-update targets
// github.com/yandy-r/dotfiles regardless of the Forgejo origin remote.
const defaultRepo = "yandy-r/dotfiles"

// binaryName is the installed executable name, matching .goreleaser.yml
// builds.binary and the AssetName prefix.
const binaryName = "dotf"

// ChecksumsAsset is the release checksum sidecar filename. It MUST match
// .goreleaser.yml checksum.name_template and bootstrap.sh's "checksums.txt"
// literal (pinned by TestAssetNaming_MatchesBootstrap).
const ChecksumsAsset = "checksums.txt"

// httpDo is the GitHub REST seam (release-metadata calls, which need
// Accept/Authorization headers). Tests replace it to aim at an httptest.Server.
// The default is a client with a total timeout (net/http has none by default).
var httpDo = httpClient.Do

// httpClient bounds every self-update request, including the body read: the
// zero-value net/http client has no timeout, so a stalled mirror would hang the
// CLI forever.
var httpClient = &http.Client{Timeout: 10 * time.Minute}

// ErrDevBuild is returned by Run on the actual replace path when the running
// binary's version is unparseable (the "dev" default) and --force was not
// given. --check and --dry-run are pure reports and never return it.
var ErrDevBuild = errors.New("self-update: current version is a dev build; re-run with --force to replace it anyway")

// ErrUnsupportedOS is returned when the host has no prebuilt release binary
// (windows, or an unknown OS). Windows in-place replacement is deferred; the
// bootstrap installer guards the same way.
var ErrUnsupportedOS = errors.New("self-update is not supported on this operating system (only linux and macOS have prebuilt binaries)")

// Options controls a self-update run.
type Options struct {
	// Force replaces even when already current, newer, or an unparseable dev build.
	Force bool
	// DryRun resolves and reports the target without downloading or replacing.
	DryRun bool
	// Check reports the latest version and availability without any I/O beyond
	// the release-metadata query.
	Check bool
}

// Result is the outcome of a run, rendered by the command layer as key=value or
// JSON.
type Result struct {
	// Current is the running version.Version, leading-"v" normalized.
	Current string
	// Latest is the latest release tag, leading-"v" normalized.
	Latest string
	// Available reports whether Latest is strictly newer than Current (or Current
	// is an unparseable dev build).
	Available bool
	// Asset is the resolved release asset name for this host.
	Asset string
	// Target is the resolved real path of the running binary (populated for
	// dry-run and the actual replace path; empty for --check).
	Target string
	// Updated reports whether an actual replace happened.
	Updated bool
}

// AssetName is the release asset filename for this host, e.g.
// "dotf_linux_amd64.tar.gz".
//
// It MUST stay in lockstep with two other consumers of the SAME naming
// convention, whose single source of truth is .goreleaser.yml:
//   - .goreleaser.yml archives.name_template ("dotf_{{ .Os }}_{{ .Arch }}") +
//     formats [tar.gz]
//   - bootstrap.sh's ASSET= line (search bootstrap.sh for `ASSET=`)
//
// TestAssetNaming_MatchesBootstrap pins this equality against bootstrap.sh in
// CI, so drift fails the build.
func AssetName(info platform.Info) string {
	return fmt.Sprintf("%s_%s_%s.tar.gz", binaryName, info.OS.ReleaseOS(), info.Arch.ReleaseArch())
}

// Run discovers the latest release, decides whether to act, and — unless Check
// or DryRun — verifies and atomically replaces the running binary. info supplies
// the os/arch tokens for the asset name. Windows/unknown hosts fail fast with
// ErrUnsupportedOS. On the replace path a dev build without Force yields
// ErrDevBuild; an already-current/newer build without Force is an idempotent
// no-op (Updated=false, nil error).
func Run(opts Options, info platform.Info) (Result, error) {
	if releaseOS := info.OS.ReleaseOS(); releaseOS == "windows" || releaseOS == "" {
		return Result{}, fmt.Errorf("%w (detected %q)", ErrUnsupportedOS, info.OS)
	}

	repo := defaultRepo

	tag, err := latestRelease(repo)
	if err != nil {
		return Result{}, err
	}

	current := normalize(version.Version)
	latest := normalize(tag)

	res := Result{
		Current: current,
		Latest:  latest,
		Asset:   AssetName(info),
	}

	// A dev build has an unparseable core version; treat it as "an update is
	// available" but gate the actual replace behind --force.
	_, _, _, _, parseErr := parseVersion(current)
	isDev := parseErr != nil
	if isDev {
		res.Available = true
	} else {
		cmp, cmpErr := compareVersions(current, latest)
		if cmpErr != nil {
			return res, fmt.Errorf("self-update: compare %q and %q: %w", current, latest, cmpErr)
		}
		res.Available = cmp < 0
	}

	// --check and --dry-run are pure reports: no download, no replace, exit 0 —
	// even on a dev build (only the actual replace path enforces ErrDevBuild).
	if opts.Check || opts.DryRun {
		if opts.DryRun {
			target, tErr := resolveExecutable()
			if tErr != nil {
				return res, tErr
			}
			res.Target = target
		}
		return res, nil
	}

	if isDev && !opts.Force {
		return res, ErrDevBuild
	}

	// Already current or newer without --force: idempotent no-op.
	if !res.Available && !opts.Force {
		return res, nil
	}

	target, err := replaceBinary(repo, tag, info)
	res.Target = target
	if err != nil {
		return res, err
	}
	res.Updated = true
	return res, nil
}
