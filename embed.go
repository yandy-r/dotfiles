// Package dotfiles owns the embedded default provisioning manifest.
//
// It lives at the module root (rather than under internal/) because an
// embed directive cannot reach outside the directory that contains it, and
// the top-level manifest/ directory is PRD-locked as the canonical location
// for the shipped TOML manifest. This file imports
// nothing from internal/ so it stays cycle-free: internal/manifest (the
// typed schema) and any future loader can import dotfiles.ManifestFS
// without dotfiles importing back into internal/.
package dotfiles

import "embed"

// ManifestFS embeds every *.toml file directly under manifest/ plus the
// committed dotf.lock seed, including the default packages.toml shipped with
// the CLI and the embedded default lock the pinned-artifact verify path falls
// back to. Paths retain the "manifest/" prefix (e.g. "manifest/packages.toml",
// "manifest/dotf.lock"); callers that want a rooted view should wrap this with
// fs.Sub(ManifestFS, "manifest").
//
//go:embed manifest/*.toml manifest/dotf.lock
var ManifestFS embed.FS
