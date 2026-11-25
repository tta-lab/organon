package project

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func tryLockRegistry(file *os.File) (bool, error) {
	err := windows.LockFileEx(windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION), err
}

func unlockRegistry(file *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &windows.Overlapped{})
}

func replaceRegistryFile(ctx context.Context, tempPath, path string) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, registryLockTimeout)
		defer cancel()
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := os.Rename(tempPath, path)
		if err == nil || (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) &&
			!errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_LOCK_VIOLATION)) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Windows does not support flushing a directory handle using File.Sync.
func syncRegistryDirectory(string) error { return nil }
