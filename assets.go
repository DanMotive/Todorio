// Package assets embeds the SQL migrations and the built frontend directly into
// the todorio binary, so a single downloaded binary is fully self-contained: no
// separate files need to be copied onto the server (spec section 1: "single Go
// binary — API + frontend serving + migrations"; section 3: install from a
// released binary).
//
// migrations/*.sql are checked into the repo, so they are always present at
// build time — that embed is unconditional and always real.
//
// web/dist is a build artifact (gitignored) and is only populated when
// `npm run build` runs in web/ before `go build`, exactly as the release
// pipeline (.github/workflows/release.yml) does. A committed placeholder file
// (web/dist/placeholder.txt) keeps the embed pattern valid even when the
// frontend hasn't been built — e.g. a plain backend-only `go build` during
// local development. Callers must treat the embed as "real" only if it
// contains an index.html, and fall back to disk-based serving otherwise; see
// internal/server.webFS for that fallback logic.
package assets

import "embed"

//go:embed all:migrations
var Migrations embed.FS

//go:embed all:web/dist
var Web embed.FS
