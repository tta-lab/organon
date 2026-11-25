package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsRegistryReadSharingConflict(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects.toml")
	entry := Entry{Alias: "one", Path: filepath.Join(root, "one"), Remote: "https://github.com/owner/one.git"}
	if _, _, err := NewStore(path).Register(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			_ = windows.CloseHandle(handle)
		}
	})
	if _, err := os.ReadFile(path); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("fixture did not block reads with a sharing violation: %v", err)
	}
	if _, err := OpenCatalog(path); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("exhausted read must retain sharing violation: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		catalog, err := OpenCatalog(path)
		if err == nil {
			_, err = catalog.GetExact("one")
		}
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("read returned before sharing conflict cleared: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	closed = true
	if err := <-done; err != nil {
		t.Fatalf("read after sharing conflict cleared: %v", err)
	}
}

func TestWindowsRegistryContentionAndReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects.toml")
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := lockFile(context.Background(), lock); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	entry := Entry{Alias: "one", Path: filepath.Join(root, "one"), Remote: "https://github.com/owner/one.git"}
	if _, _, err := NewStore(path).Register(ctx, entry); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock contention: %v", err)
	}
	unlockRegistry(lock)
	if _, _, err := NewStore(path).Register(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	readErr := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				readErr <- nil
				return
			default:
			}
			if _, err := NewStore(path).Snapshot(); err != nil {
				readErr <- err
				return
			}
		}
	}()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("repo%d", i)
			entry := Entry{Alias: name, Path: filepath.Join(root, name), Remote: "https://github.com/owner/" + name + ".git"}
			_, _, err := NewStore(path).Register(context.Background(), entry)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	close(stop)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := <-readErr; err != nil {
		t.Fatalf("reader observed incomplete registry: %v", err)
	}
	entries, err := NewStore(path).List(false)
	if err != nil || len(entries) != 9 {
		t.Fatalf("registry entries=%d error=%v", len(entries), err)
	}
}

func TestWindowsRegistryConcurrentSnapshotsAreComplete(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects.toml")
	fixture := func(suffix string) (string, []Entry) {
		entries := []Entry{
			{Alias: "first", Path: filepath.Join(root, "first-"+suffix),
				Remote: "https://github.com/owner/first-" + suffix + ".git"},
			{Alias: "second", Path: filepath.Join(root, "second-"+suffix),
				Remote: "https://github.com/owner/second-" + suffix + ".git"},
		}
		content := fmt.Sprintf("[first]\npath = %q\nremote = %q\n[second]\npath = %q\nremote = %q\n",
			entries[0].Path, entries[0].Remote, entries[1].Path, entries[1].Remote)
		return content, entries
	}
	oldContent, oldEntries := fixture("old")
	newContent, newEntries := fixture("new")
	if err := writeRegistryFile(context.Background(), path, oldContent, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	proceed := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		err := writeRegistryFile(ctx, path, newContent, 0600)
		close(started)
		if err == nil {
			select {
			case <-proceed:
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		for i := 0; err == nil && i < 50; i++ {
			content := oldContent
			if i%2 != 0 {
				content = newContent
			}
			err = writeRegistryFile(ctx, path, content, 0600)
		}
		done <- err
	}()
	defer func() { cancel(); <-done }()
	<-started
	// The writer pauses after its first replacement, guaranteeing a read before
	// the remaining replacements while subsequent reads race the writer.
	for first := true; ; first = false {
		catalog, err := NewStore(path).Snapshot()
		if err != nil {
			t.Fatalf("reading registry during replacement: %v", err)
		}
		entries := catalog.ListAll(true)
		if !slices.Equal(entries, oldEntries) && !slices.Equal(entries, newEntries) {
			t.Fatalf("reader observed incomplete or unexpected snapshot: %+v", entries)
		}
		if first {
			close(proceed)
		}
		select {
		case err := <-done:
			// Keep one result available for the cleanup that waits for the writer.
			done <- err
			if err != nil {
				t.Fatalf("replacing registry: %v", err)
			}
			return
		default:
		}
	}
}

func TestWindowsRegistryReplacementWithReader(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects.toml")
	store := NewStore(path)
	one := Entry{Alias: "one", Path: filepath.Join(root, "one"), Remote: "https://github.com/owner/one.git"}
	two := Entry{Alias: "two", Path: filepath.Join(root, "two"), Remote: "https://github.com/owner/two.git"}
	if _, _, err := store.Register(context.Background(), one); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _, registerErr := store.Register(ctx, two)
	if registerErr != nil && !errors.Is(registerErr, context.DeadlineExceeded) {
		t.Fatalf("replacement with open reader: %v", registerErr)
	}
	oldData, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read original handle: %v", err)
	}
	oldCatalogPath := filepath.Join(root, "old.toml")
	if err := os.WriteFile(oldCatalogPath, oldData, 0600); err != nil {
		t.Fatal(err)
	}
	assertWindowsRegistryEntries(t, oldCatalogPath, "one")
	if registerErr != nil {
		assertWindowsRegistryEntries(t, path, "one")
	} else {
		assertWindowsRegistryEntries(t, path, "one", "two")
	}
	if registerErr != nil {
		retryWindowsReplacementWithReader(t, store, reader, two)
	} else if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	assertWindowsRegistryEntries(t, path, "one", "two")
}

func retryWindowsReplacementWithReader(t *testing.T, store *Store, reader *os.File, entry Entry) {
	t.Helper()
	done := make(chan error, 1)
	go func() { _, _, err := store.Register(context.Background(), entry); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("replacement with open reader: %v", err)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
	case <-time.After(100 * time.Millisecond):
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatalf("replacement after reader closed: %v", err)
		}
	}
}

func assertWindowsRegistryEntries(t *testing.T, path string, aliases ...string) {
	t.Helper()
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	if len(catalog.ListAll(false)) != len(aliases) {
		t.Fatalf("registry has %d entries, want %d", len(catalog.ListAll(false)), len(aliases))
	}
	for _, alias := range aliases {
		if _, err := catalog.GetExact(alias); err != nil {
			t.Fatalf("registry missing %s: %v", alias, err)
		}
	}
}
