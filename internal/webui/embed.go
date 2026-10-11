// Package webui serves the SPA dashboard and bridges the browser to cluster
// nodes that are otherwise only reachable server-side.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

// distFS returns the embedded build output. The dist directory always contains
// at least a committed placeholder so the package compiles on a fresh clone;
// `make web-build` replaces it with the real Vite output.
func distFS() (fs.FS, error) {
	return fs.Sub(assets, "dist")
}
