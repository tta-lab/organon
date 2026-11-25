package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

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

func TestWindowsRegistryReplacementWaitsForReader(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, _, err := store.Register(ctx, two); !errors.Is(err, context.DeadlineExceeded) {
		reader.Close()
		t.Fatalf("replacement should wait for open reader: %v", err)
	}
	if catalog, err := store.Snapshot(); err != nil {
		reader.Close()
		t.Fatalf("original registry unavailable: %v", err)
	} else if _, err := catalog.GetExact("one"); err != nil {
		reader.Close()
		t.Fatalf("original registry changed: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, _, err := store.Register(context.Background(), two); done <- err }()
	select {
	case err := <-done:
		reader.Close()
		t.Fatalf("replacement finished before reader closed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("replacement after reader closed: %v", err)
	}
	if _, err := store.Get("two"); err != nil {
		t.Fatalf("new registry missing entry: %v", err)
	}
}
