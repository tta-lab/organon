//go:build !windows

package project

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func tryLockRegistry(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	return errors.Is(err, unix.EWOULDBLOCK), err
}

func unlockRegistry(file *os.File) { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN) }

func readCatalogFile(path string) ([]byte, error) { return os.ReadFile(path) }

func replaceRegistryFile(_ context.Context, tempPath, path string) error {
	return os.Rename(tempPath, path)
}

func syncRegistryDirectory(dir string) error {
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("opening project registry directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("syncing project registry directory: %w", err)
	}
	return nil
}
