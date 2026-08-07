package safefile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type resolvedFile struct {
	root     string
	relative string
}

// CheckContained verifies that path currently resolves inside root.
func CheckContained(root, path string) error {
	_, err := resolve(root, path)
	return err
}

// OpenContained resolves path inside root and opens it through no-follow directory descriptors.
func OpenContained(root, path string) (*os.File, error) {
	resolved, err := resolve(root, path)
	if err != nil {
		return nil, err
	}
	return openResolved(resolved)
}

func resolve(root, path string) (resolvedFile, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return resolvedFile{}, fmt.Errorf("resolve root %q: %w", root, err)
	}
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(realRoot, target)
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return resolvedFile{}, fmt.Errorf("resolve path %q: %w", path, err)
	}
	relative, err := filepath.Rel(realRoot, realTarget)
	if err != nil || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return resolvedFile{}, fmt.Errorf("resolved path %q is outside root %q", realTarget, realRoot)
	}
	return resolvedFile{root: realRoot, relative: relative}, nil
}

func openResolved(resolved resolvedFile) (*os.File, error) {
	directory, err := openAbsoluteDirectory(resolved.root)
	if err != nil {
		return nil, err
	}
	components := strings.Split(filepath.Clean(resolved.relative), string(filepath.Separator))
	for _, component := range components[:len(components)-1] {
		next, err := unix.Openat(directory, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(directory)
		if err != nil {
			return nil, err
		}
		directory = next
	}
	fileDescriptor, err := unix.Openat(
		directory, components[len(components)-1],
		unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0,
	)
	_ = unix.Close(directory)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fileDescriptor), filepath.Join(resolved.root, resolved.relative)), nil
}

func openAbsoluteDirectory(path string) (int, error) {
	directory, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	cleanPath := strings.TrimPrefix(filepath.Clean(path), string(filepath.Separator))
	components := strings.Split(cleanPath, string(filepath.Separator))
	for _, component := range components {
		if component == "" {
			continue
		}
		next, err := unix.Openat(directory, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(directory)
		if err != nil {
			return -1, err
		}
		directory = next
	}
	return directory, nil
}
