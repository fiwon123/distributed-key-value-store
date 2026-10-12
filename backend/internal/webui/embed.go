// Package webui serves the SPA dashboard and bridges the browser to cluster
// nodes that are otherwise only reachable server-side.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

// docsHTML is the Swagger UI page served at GET /docs. It loads Swagger UI from
// a CDN and points it at the node's own /v1/openapi.json.
//
//go:embed docs.html
var docsHTML []byte

// distFS returns the embedded build output. The dist directory always contains
// at least a committed placeholder so the package compiles on a fresh clone;
// `make web-build` replaces it with the real Vite output.
func distFS() (fs.FS, error) {
	return fs.Sub(assets, "dist")
}
