//go:build darwin

package vacsimport

import "golang.org/x/sys/unix"

func publishDirectory(source, target string) error {
	return unix.RenamexNp(source, target, unix.RENAME_EXCL)
}
