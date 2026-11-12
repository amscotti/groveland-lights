// Package assets embeds the web templates and static files so the server is a
// single binary. This file lives at the module root so go:embed can reach web/.
package assets

import (
	"embed"
	"io/fs"
)

//go:embed web/templates web/static
var webFS embed.FS

// Templates returns the templates subtree.
func Templates() fs.FS {
	sub, err := fs.Sub(webFS, "web/templates")
	if err != nil {
		panic(err)
	}
	return sub
}

// Static returns the static subtree.
func Static() fs.FS {
	sub, err := fs.Sub(webFS, "web/static")
	if err != nil {
		panic(err)
	}
	return sub
}
