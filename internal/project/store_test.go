package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestStoreReloadsCurrentSnapshotForEveryRead(t *testing.T) {
	path := writeProjectFile(t, "[one]\npath = \"/projects/one\"\nremote = \"https://example.com/owner/one.git\"\n")
	store := NewStore(path)

	first, err := store.List(false)
	if err != nil {
		t.Fatalf("List first snapshot: %v", err)
	}
	if len(first) != 1 || first[0].Alias != "one" {
		t.Fatalf("first List = %+v", first)
	}

	secondConfig := "[two]\npath = \"/projects/two\"\n" +
		"remote = \"https://example.com/owner/two.git\"\n"
	if err := os.WriteFile(path, []byte(secondConfig), 0644); err != nil {
		t.Fatalf("replace projects file: %v", err)
	}
	second, err := store.List(false)
	if err != nil {
		t.Fatalf("List second snapshot: %v", err)
	}
	if len(second) != 1 || second[0].Alias != "two" {
		t.Fatalf("second List = %+v", second)
	}
}

func TestStoreRegisterAppendsMinimalTableAndPreservesContext(t *testing.T) {
	path := writeProjectFile(t, `# registry context
[one]
name = "One"
path = "/projects/one"
remote = "https://example.com/owner/one.git"

[archived.legacy]
path = "/projects/legacy"
remote = "https://old.example/owner/legacy.git"
`)
	store := NewStore(path)

	entry, created, err := store.Register(context.Background(), Entry{
		Alias:  "two",
		Name:   "Two",
		Path:   "/projects/two",
		Remote: "https://example.com/owner/two.git",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !created || entry.Alias != "two" || entry.Archived {
		t.Fatalf("Register = (%+v, %v)", entry, created)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read projects file: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"# registry context",
		"[archived.legacy]",
		`remote = "https://old.example/owner/legacy.git"`,
		"[two]",
		`name = "Two"`,
		`path = "/projects/two"`,
		`remote = "https://example.com/owner/two.git"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("projects file missing %q:\n%s", want, text)
		}
	}
}

func TestStoreRegisterReusesExactExistingIdentity(t *testing.T) {
	path := writeProjectFile(t, "[existing]\npath = \"/projects/repo\"\nremote = \"https://example.com/owner/repo.git\"\n")
	store := NewStore(path)

	entry, created, err := store.Register(context.Background(), Entry{
		Alias:  "existing",
		Path:   "/projects/repo",
		Remote: "https://example.com/owner/repo.git",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if created || entry.Alias != "existing" {
		t.Fatalf("Register = (%+v, %v), want reused existing", entry, created)
	}
}

func TestStoreRegisterPreservesArchivedStatusAtPath(t *testing.T) {
	path := writeProjectFile(t, `[archived.ttal]
path = "/projects/ttal"
remote = "https://example.com/owner/ttal.git"
`)
	store := NewStore(path)

	entry, created, err := store.Register(context.Background(), Entry{
		Alias:  "ttal",
		Path:   "/projects/ttal",
		Remote: "https://example.com/owner/ttal.git",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if created || entry.Alias != "ttal" || !entry.Archived {
		t.Fatalf("Register = (%+v, %v), want archived existing", entry, created)
	}
}

func TestStoreRegisterSerializesConcurrentWriters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.toml")
	store := NewStore(path)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			alias := string(rune('a' + i))
			_, _, err := store.Register(context.Background(), Entry{
				Alias:  alias,
				Path:   "/projects/" + alias,
				Remote: "https://example.com/owner/" + alias + ".git",
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Register: %v", err)
		}
	}

	entries, err := store.List(false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 8 {
		t.Fatalf("List returned %d entries: %+v", len(entries), entries)
	}
}

func TestStoreRegisterHonorsCanceledContextBeforeMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.toml")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewStore(path).Register(ctx, Entry{
		Alias: "one", Path: "/projects/one", Remote: "https://example.com/owner/one.git",
	})
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("Register error = %v, want context canceled", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("projects file was mutated: %v", statErr)
	}
}

func TestStoreRegisterWritesCanonicalRemoteAndRequiresExactTripleForRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.toml")
	store := NewStore(path)
	requested := Entry{
		Alias:  "organon",
		Name:   "Organon",
		Path:   "/projects/organon",
		Remote: "https://GitHub.com/TTA-Lab/Organon",
	}
	entry, created, err := store.Register(context.Background(), requested)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !created || entry.Remote != "https://github.com/tta-lab/organon.git" {
		t.Fatalf("Register = (%+v, %v)", entry, created)
	}
	retried, created, err := store.Register(context.Background(), requested)
	if err != nil || created || retried != entry {
		t.Fatalf("retry Register = (%+v, %v, %v), want exact reuse", retried, created, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `remote = "https://github.com/tta-lab/organon.git"`) {
		t.Fatalf("projects file =\n%s", data)
	}
}

func TestStoreRegisterRejectsPartialAliasPathOrRemoteCollision(t *testing.T) {
	path := writeProjectFile(t, `[one]
path = "/projects/one"
remote = "https://example.com/owner/one.git"
`)
	store := NewStore(path)
	for name, entry := range map[string]Entry{
		"alias":  {Alias: "one", Path: "/projects/two", Remote: "https://example.com/owner/two.git"},
		"path":   {Alias: "two", Path: "/projects/one", Remote: "https://example.com/owner/two.git"},
		"remote": {Alias: "two", Path: "/projects/two", Remote: "https://example.com/owner/one.git"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := store.Register(context.Background(), entry); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Register error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}
