//go:build embed_frontend

package webassets

import (
	"embed"
	"io/fs"
)

//go:embed dist
var frontendFiles embed.FS

func embedded() (fs.FS, bool) {
	dist, err := fs.Sub(frontendFiles, "dist")
	if err != nil {
		panic(err)
	}
	return dist, true
}
