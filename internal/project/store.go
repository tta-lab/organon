package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/tta-lab/organon/internal/gitprovider"
)

const registryLockTimeout = 5 * time.Second

// Store reads and updates one projects.toml registry.
type Store struct {
	path string
}

// NewStore returns a hot project registry backed by path.
func NewStore(path string) *Store { return &Store{path: path} }

// Snapshot reads and validates the current registry contents.
func (s *Store) Snapshot() (*Catalog, error) { return OpenCatalog(s.path) }

// List reads the current registry and lists active or all projects.
func (s *Store) List(includeArchived bool) ([]Entry, error) {
	catalog, err := s.Snapshot()
	if err != nil {
		return nil, err
	}
	return catalog.ListAll(includeArchived), nil
}

// Get reads the current registry and returns an exact active or archived alias.
func (s *Store) Get(alias string) (Entry, error) {
	catalog, err := s.Snapshot()
	if err != nil {
		return Entry{}, err
	}
	return catalog.GetExact(alias)
}

// Resolve reads the current registry and resolves an existing project reference.
func (s *Store) Resolve(reference string) (Entry, error) {
	catalog, err := s.Snapshot()
	if err != nil {
		return Entry{}, err
	}
	return catalog.Resolve(reference)
}

// Find reads the current registry and returns ranked active projects.
func (s *Store) Find(query string, limit int) ([]Entry, error) {
	catalog, err := s.Snapshot()
	if err != nil {
		return nil, err
	}
	return catalog.Find(query, limit)
}

// GetByPath reads the current registry and returns an exact cleaned path.
func (s *Store) GetByPath(path string) (Entry, error) {
	catalog, err := s.Snapshot()
	if err != nil {
		return Entry{}, err
	}
	return catalog.GetByPath(path)
}

// GetByRemote reads the current registry and returns a canonical repository identity.
func (s *Store) GetByRemote(remote string) (Entry, error) {
	catalog, err := s.Snapshot()
	if err != nil {
		return Entry{}, err
	}
	return catalog.GetByRemote(remote)
}

// Register atomically appends one active entry. Existing paths are reused,
// including archived entries, so clone retries never reactivate a repository.
func (s *Store) Register(ctx context.Context, requested Entry) (Entry, bool, error) {
	if requested.Archived {
		return Entry{}, false, fmt.Errorf("%w: cannot register an archived entry", ErrInvalidConfig)
	}
	if !validAlias(requested.Alias) {
		return Entry{}, false, fmt.Errorf("%w %q", ErrInvalidAlias, requested.Alias)
	}
	if requested.Path == "" || !filepath.IsAbs(requested.Path) {
		return Entry{}, false, fmt.Errorf(
			"%w for %q: path must be a non-empty absolute path",
			ErrInvalidConfig, requested.Alias,
		)
	}
	requested.Path = filepath.Clean(requested.Path)
	info, err := gitprovider.ParseHTTPRemoteURL(requested.Remote)
	if err != nil {
		return Entry{}, false, fmt.Errorf("%w for %q: invalid remote: %v", ErrInvalidConfig, requested.Alias, err)
	}
	requested.Remote = info.CanonicalURL
	if err := ctx.Err(); err != nil {
		return Entry{}, false, fmt.Errorf("registering project: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return Entry{}, false, fmt.Errorf("creating project registry directory: %w", err)
	}
	lock, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Entry{}, false, fmt.Errorf("opening project registry lock: %w", err)
	}
	defer lock.Close()
	if err := lockFile(ctx, lock); err != nil {
		return Entry{}, false, err
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN) //nolint:errcheck

	catalog, err := s.Snapshot()
	if err != nil {
		return Entry{}, false, err
	}
	if existing, found, err := registrationCollision(catalog, requested); err != nil || found {
		return existing, false, err
	}

	if err := s.appendEntry(requested); err != nil {
		return Entry{}, false, err
	}
	return requested, true, nil
}

func registrationCollision(catalog *Catalog, requested Entry) (Entry, bool, error) {
	if existing, err := catalog.GetExact(requested.Alias); err == nil {
		if existing.Path == requested.Path && existing.Remote == requested.Remote {
			return existing, true, nil
		}
		return Entry{}, false, fmt.Errorf(
			"%w: alias %q conflicts with registered project",
			ErrInvalidConfig, requested.Alias,
		)
	} else if !errors.Is(err, ErrNotFound) {
		return Entry{}, false, err
	}
	if existing, err := catalog.GetByPath(requested.Path); err == nil {
		return Entry{}, false, fmt.Errorf(
			"%w: path %q already belongs to alias %q",
			ErrInvalidConfig, requested.Path, existing.Alias,
		)
	} else if !errors.Is(err, ErrNotFound) {
		return Entry{}, false, err
	}
	if existing, err := catalog.GetByRemote(requested.Remote); err == nil {
		return Entry{}, false, fmt.Errorf(
			"%w: remote %q already belongs to alias %q",
			ErrInvalidConfig, requested.Remote, existing.Alias,
		)
	} else if !errors.Is(err, ErrNotFound) {
		return Entry{}, false, err
	}
	return Entry{}, false, nil
}

func lockFile(ctx context.Context, file *os.File) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, registryLockTimeout)
		defer cancel()
	}
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("locking project registry: %w", err)
		}
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			return fmt.Errorf("locking project registry: %w", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("locking project registry: %w", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (s *Store) appendEntry(entry Entry) error {
	data, mode, err := readRegistryFile(s.path)
	if err != nil {
		return err
	}

	var appended strings.Builder
	appended.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		appended.WriteByte('\n')
	}
	if len(data) > 0 {
		appended.WriteByte('\n')
	}
	fmt.Fprintf(&appended, "[%s]\n", entry.Alias)
	if entry.Name != "" {
		fmt.Fprintf(&appended, "name = %s\n", strconv.Quote(entry.Name))
	}
	fmt.Fprintf(&appended, "path = %s\n", strconv.Quote(entry.Path))
	fmt.Fprintf(&appended, "remote = %s\n", strconv.Quote(entry.Remote))

	return writeRegistryFile(s.path, appended.String(), mode)
}

func readRegistryFile(path string) ([]byte, os.FileMode, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, 0, fmt.Errorf("reading projects file: %w", err)
	}
	mode := os.FileMode(0644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(statErr) {
		return nil, 0, fmt.Errorf("stat projects file: %w", statErr)
	}
	return data, mode, nil
}

func writeRegistryFile(path, content string, mode os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temporary projects file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return fmt.Errorf("setting temporary projects file mode: %w", err)
	}
	if _, err := temp.WriteString(content); err != nil {
		temp.Close()
		return fmt.Errorf("writing temporary projects file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("syncing temporary projects file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("closing temporary projects file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replacing projects file: %w", err)
	}
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
