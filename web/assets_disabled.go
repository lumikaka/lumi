//go:build !embed_frontend

package webassets

import "io/fs"

func embedded() (fs.FS, bool) {
	return nil, false
}
