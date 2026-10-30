// Package processlock provides persistent lock files backed by OS locks.
// Never unlink a lock file: another process may already have its inode open.
package processlock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrBusy = errors.New("lock is held by another process")

type Lock struct{ file *os.File }

func Try(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := tryLock(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Lock{file: f}, nil
}

func Acquire(ctx context.Context, path string) (*Lock, error) {
	for {
		lock, err := Try(path)
		if !errors.Is(err, ErrBusy) {
			return lock, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for %s: %w", path, ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (lock *Lock) Close() error {
	if lock == nil {
		return nil
	}
	return lock.file.Close() // Closing releases the kernel lock, including on crashes.
}
