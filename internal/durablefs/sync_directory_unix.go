//go:build !windows

package durablefs

import "os"

// SyncDirectory persists directory-entry changes on platforms that support
// fsync on an open directory.
func SyncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
