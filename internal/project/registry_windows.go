package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func readCatalogFile(path string) ([]byte, error) {
	// Replacement fallback or another process can briefly deny read sharing.
	deadline := time.Now().Add(time.Second)
	for {
		data, err := os.ReadFile(path)
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) || !time.Now().Before(deadline) {
			return data, err
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func replaceRegistryFile(ctx context.Context, tempPath, path string) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, registryLockTimeout)
		defer cancel()
	}
	// Root.Rename prefers POSIX replacement on Windows when the filesystem supports it.
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := root.Rename(filepath.Base(tempPath), filepath.Base(path))
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
