//go:build !windows

package processlock

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func tryLock(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrBusy
	}
	return err
}
