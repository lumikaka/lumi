package project

import "os"

// Windows MoveFile does not replace an existing destination directory.
func renameDirectoryNoReplace(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
