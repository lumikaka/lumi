//go:build windows

package durablefs

// SyncDirectory is a no-op because Windows does not support flushing an open
// directory with the file-oriented FlushFileBuffers API. Callers sync file
// contents before publishing the corresponding directory entry.
func SyncDirectory(string) error {
	return nil
}
