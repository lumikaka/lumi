package webassets

import "io/fs"

func Embedded() (fs.FS, bool) {
	return embedded()
}
