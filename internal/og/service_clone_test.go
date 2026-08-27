package og

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

func TestGitCloneDerivesProjectPathAndRegistersAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store := writeCloneProjects(t, "")
	var got cloneInvocation
	withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
		got = invocation
		return createClonedRepository(ctx, invocation)
	})

	resp, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
		URL: "https://codeberg.org/tta-lab/example.git",
	})
	if err != nil {
		t.Fatalf("GitClone: %v", err)
	}
	wantPath := filepath.Join(home, "code", "projects", "tta-lab", "example")
	assertTemporaryCloneInvocation(t, got, wantPath)
	assertRegisteredCloneResult(t, resp.Clone, wantPath)
	entry, err := store.Get("example")
	if err != nil || entry.Path != wantPath || entry.Archived {
		t.Fatalf("registered entry = %+v, %v", entry, err)
	}
}

func TestGitCloneAcceptsCaseInsensitiveCloneParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	parent := filepath.Join(home, "code", "projects", "LamplitIsles")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "code", "projects", "lamplitisles")); err != nil {
		t.Skip("filesystem is case-sensitive")
	}
	store := writeCloneProjects(t, "")
	withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
		return createClonedRepository(ctx, invocation)
	})

	resp, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
		URL: "https://github.com/LamplitIsles/kepos-imagegen.git",
	})
	if err != nil {
		t.Fatalf("GitClone: %v", err)
	}
	if resp.Clone == nil || !resp.Clone.Registered {
		t.Fatalf("clone result = %+v", resp.Clone)
	}
}

func assertTemporaryCloneInvocation(t *testing.T, got cloneInvocation, wantPath string) {
	t.Helper()
	if got.Destination == wantPath {
		t.Fatal("clone runner received final destination instead of a temporary sibling")
	}
	temporaryName := filepath.Base(got.Destination)
	if filepath.Dir(got.Destination) != filepath.Dir(wantPath) ||
		!strings.HasPrefix(temporaryName, ".example.clone-") {
		t.Fatalf("temporary destination = %q, want sibling of %q", got.Destination, wantPath)
	}
	if got.Remote != "https://codeberg.org/tta-lab/example.git" ||
		got.Provider != "generic" || got.Token != "" {
		t.Fatalf("clone invocation = %+v", got)
	}
}

func assertRegisteredCloneResult(t *testing.T, result *CloneResult, wantPath string) {
	t.Helper()
	if result == nil {
		t.Fatal("clone result is nil")
	}
	if result.Path != wantPath || result.Alias != "example" || !result.Registered ||
		result.Archived || result.AlreadyExisted {
		t.Fatalf("clone result = %+v", result)
	}
	if result.Provider != "generic" || result.Host != "codeberg.org" ||
		result.Owner != "tta-lab" || result.Repo != "example" {
		t.Fatalf("clone identity = %+v", result)
	}
}

func TestGitCloneReferenceUsesHostPathAndNeverRegisters(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store := writeCloneProjects(t, "")
	withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
		return createClonedRepository(ctx, invocation)
	})

	resp, err := NewServiceWithConfig(nil, store, ogconfig.Config{
		Forgejo: ogconfig.ForgejoConfig{AllowedBaseURLs: []string{"http://forgejo.localhost:17480"}},
	}).GitClone(Request{URL: "http://forgejo.localhost:17480/tta-lab/example", Reference: true})
	if err != nil {
		t.Fatalf("GitClone: %v", err)
	}
	wantPath := filepath.Join(home, "code", "references", "forgejo.localhost_17480", "tta-lab", "example")
	if resp.Clone == nil || resp.Clone.Path != wantPath || resp.Clone.Registered ||
		resp.Clone.Alias != "" || resp.Clone.Provider != "forgejo" {
		t.Fatalf("clone result = %+v", resp.Clone)
	}
	entries, err := store.List(true)
	if err != nil || len(entries) != 0 {
		t.Fatalf("registry entries = %+v, %v", entries, err)
	}
}

func TestGitCloneNormalizesDefaultPortAndRemote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store := writeCloneProjects(t, "")
	var got cloneInvocation
	withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
		got = invocation
		return createClonedRepository(ctx, invocation)
	})

	resp, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
		URL:       "https://Codeberg.org:443/tta-lab/example",
		Reference: true,
	})
	if err != nil {
		t.Fatalf("GitClone: %v", err)
	}
	if got.Remote != "https://codeberg.org/tta-lab/example.git" {
		t.Fatalf("clone remote = %q", got.Remote)
	}
	wantPath := filepath.Join(home, "code", "references", "codeberg.org", "tta-lab", "example")
	if resp.Clone == nil || resp.Clone.Remote != got.Remote ||
		resp.Clone.Host != "codeberg.org" || resp.Clone.Path != wantPath {
		t.Fatalf("clone result = %+v", resp.Clone)
	}
}

func TestGitCloneRejectsUnsafeInputsBeforeSideEffects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := writeCloneProjects(t, "")
	calls := 0
	withCloneRunner(t, func(context.Context, cloneInvocation) error {
		calls++
		return nil
	})
	service := NewServiceWithConfig(nil, store, ogconfig.Config{
		Forgejo: ogconfig.ForgejoConfig{AllowedBaseURLs: []string{"http://forgejo.localhost:17480"}},
	})
	tests := []struct {
		name string
		req  Request
	}{
		{name: "ssh shorthand", req: Request{URL: "git@github.com:tta-lab/example.git"}},
		{name: "ssh URL", req: Request{URL: "ssh://git@github.com/tta-lab/example.git"}},
		{name: "file URL", req: Request{URL: "file:///tmp/example.git"}},
		{name: "local path", req: Request{URL: "/tmp/example.git"}},
		{name: "unlisted HTTP", req: Request{URL: "http://codeberg.org/tta-lab/example.git"}},
		{name: "credentials", req: Request{URL: "https://user:secret@codeberg.org/tta-lab/example.git"}},
		{name: "query", req: Request{URL: "https://codeberg.org/tta-lab/example.git?ref=main"}},
		{name: "fragment", req: Request{URL: "https://codeberg.org/tta-lab/example.git#main"}},
		{name: "extra path", req: Request{URL: "https://codeberg.org/team/tta-lab/example.git"}},
		{name: "double slash", req: Request{URL: "https://codeberg.org//tta-lab/example.git"}},
		{name: "traversal", req: Request{URL: "https://codeberg.org/tta-lab/../example.git"}},
		{name: "encoded path", req: Request{URL: "https://codeberg.org/tta-lab%2fescape/example.git"}},
		{name: "reference alias", req: Request{URL: "https://codeberg.org/tta-lab/example.git", Alias: "x", Reference: true}},
		{name: "invalid alias", req: Request{URL: "https://codeberg.org/tta-lab/example.git", Alias: "bad.alias"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.GitClone(tt.req); err == nil {
				t.Fatal("GitClone succeeded, want rejection")
			}
		})
	}
	if calls != 0 {
		t.Fatalf("clone runner calls = %d, want 0", calls)
	}
}

func TestGitCloneReusesExistingCheckoutAndRepairsRegistration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store := writeCloneProjects(t, "")
	destination := filepath.Join(home, "code", "projects", "tta-lab", "example")
	initCloneRepository(t, destination, "https://codeberg.org/tta-lab/example.git")
	withCloneRunner(t, func(context.Context, cloneInvocation) error {
		t.Fatal("clone runner called for existing checkout")
		return nil
	})

	resp, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
		URL:   "https://codeberg.org/tta-lab/example",
		Alias: "sample",
	})
	if err != nil {
		t.Fatalf("GitClone: %v", err)
	}
	if resp.Clone == nil || !resp.Clone.AlreadyExisted || !resp.Clone.Registered || resp.Clone.Alias != "sample" {
		t.Fatalf("clone result = %+v", resp.Clone)
	}
	entry, err := store.Get("sample")
	if err != nil || entry.Path != destination {
		t.Fatalf("registered entry = %+v, %v", entry, err)
	}
}

func TestGitCloneTreatsNormalizedExistingOriginAsSameRepository(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store := writeCloneProjects(t, "")
	destination := filepath.Join(home, "code", "projects", "tta-lab", "example")
	initCloneRepository(t, destination, "https://codeberg.org:443/tta-lab/example.git")
	withCloneRunner(t, func(context.Context, cloneInvocation) error {
		t.Fatal("clone runner called for normalized existing checkout")
		return nil
	})

	resp, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
		URL: "https://codeberg.org/tta-lab/example.git",
	})
	if err != nil {
		t.Fatalf("GitClone: %v", err)
	}
	if resp.Clone == nil || !resp.Clone.AlreadyExisted || resp.Clone.Remote != "https://codeberg.org/tta-lab/example.git" {
		t.Fatalf("clone result = %+v", resp.Clone)
	}
}

func TestGitCloneRejectsExistingOriginWithCredentialMaterial(t *testing.T) {
	tests := []struct {
		name   string
		remote string
	}{
		{name: "userinfo", remote: "https://user:secret@codeberg.org/tta-lab/example.git"},
		{name: "query", remote: "https://codeberg.org/tta-lab/example.git?token=secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			destination := filepath.Join(home, "code", "projects", "tta-lab", "example")
			initCloneRepository(t, destination, tt.remote)
			withCloneRunner(t, func(context.Context, cloneInvocation) error {
				t.Fatal("clone runner called for unsafe existing origin")
				return nil
			})
			_, err := NewServiceWithConfig(nil, writeCloneProjects(t, ""), ogconfig.Config{}).GitClone(Request{
				URL: "https://codeberg.org/tta-lab/example.git",
			})
			if err == nil {
				t.Fatal("GitClone accepted existing origin with credential material")
			}
		})
	}
}

func TestGitClonePreservesArchivedRegistration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	destination := filepath.Join(home, "code", "projects", "tta-lab", "example")
	store := writeCloneProjects(t, "[archived.example-old]\npath = "+quoteTOML(destination)+
		"\nremote = \"https://codeberg.org/tta-lab/example.git\"\n")
	withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
		return createClonedRepository(ctx, invocation)
	})

	resp, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
		URL: "https://codeberg.org/tta-lab/example.git",
	})
	if err != nil {
		t.Fatalf("GitClone: %v", err)
	}
	if resp.Clone == nil || resp.Clone.Alias != "example-old" || !resp.Clone.Archived || !resp.Clone.Registered {
		t.Fatalf("clone result = %+v", resp.Clone)
	}
	if _, err := store.Get("different"); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("unexpected alias registration error = %v", err)
	}
}

func TestGitCloneRejectsConflictingExplicitAliasForRegisteredRemote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	destination := filepath.Join(home, "registered", "example")
	store := writeCloneProjects(t, "[archived.example-old]\npath = "+quoteTOML(destination)+
		"\nremote = \"https://codeberg.org/tta-lab/example.git\"\n")
	withCloneRunner(t, func(context.Context, cloneInvocation) error {
		t.Fatal("clone runner called before alias conflict rejection")
		return nil
	})

	_, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
		URL: "https://codeberg.org/tta-lab/example.git", Alias: "different",
	})
	if err == nil || !strings.Contains(err.Error(), "alias") {
		t.Fatalf("GitClone error = %v, want explicit alias conflict", err)
	}
}

func TestGitCloneReusesRegisteredPathFoundByRemote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	destination := filepath.Join(home, "custom", "example")
	store := writeCloneProjects(t, "[existing]\npath = "+quoteTOML(destination)+
		"\nremote = \"https://codeberg.org/tta-lab/example.git\"\n")
	var got cloneInvocation
	withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
		got = invocation
		return createClonedRepository(ctx, invocation)
	})

	resp, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
		URL: "https://codeberg.org/tta-lab/example.git",
	})
	if err != nil {
		t.Fatalf("GitClone: %v", err)
	}
	assertTemporaryCloneInvocationFor(t, got, destination, "https://codeberg.org/tta-lab/example.git")
	if resp.Clone == nil || resp.Clone.Alias != "existing" || resp.Clone.Path != destination {
		t.Fatalf("clone result = %+v", resp.Clone)
	}
}

func TestGitCloneRejectsDerivedPathCollisionBeforeClone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	destination := filepath.Join(home, "code", "projects", "tta-lab", "example")
	store := writeCloneProjects(t, "[other]\npath = "+quoteTOML(destination)+
		"\nremote = \"https://github.com/tta-lab/example.git\"\n")
	withCloneRunner(t, func(context.Context, cloneInvocation) error {
		t.Fatal("clone runner called before path collision rejection")
		return nil
	})

	_, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
		URL: "https://codeberg.org/tta-lab/example.git",
	})
	if err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("GitClone error = %v, want path collision", err)
	}
}

func TestGitCloneRoutesOnlyResolvedCredentials(t *testing.T) {
	t.Run("GitHub App", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("GITHUB_TOKEN", "ambient-pat")
		broker := &recordingBroker{token: "installation-token"}
		var got cloneInvocation
		withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
			got = invocation
			return createClonedRepository(ctx, invocation)
		})
		_, err := NewServiceWithConfig(broker, writeCloneProjects(t, ""), ogconfig.Config{}).GitClone(Request{
			URL: "https://github.com/tta-lab/example.git",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.Token != "installation-token" || got.Provider != "github" ||
			strings.Contains(got.Remote, "installation-token") {
			t.Fatalf("clone invocation = %+v", got)
		}
		if len(broker.tokenCalls) != 1 || broker.tokenCalls[0].purpose != githubapp.PurposeGitRead {
			t.Fatalf("broker calls = %+v", broker.tokenCalls)
		}
	})

	t.Run("allowed Forgejo", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("FORGEJO_TOKEN", "forgejo-token")
		t.Setenv("GITHUB_TOKEN", "ambient-pat")
		var got cloneInvocation
		withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
			got = invocation
			return createClonedRepository(ctx, invocation)
		})
		_, err := NewServiceWithConfig(nil, writeCloneProjects(t, ""), ogconfig.Config{
			Forgejo: ogconfig.ForgejoConfig{AllowedBaseURLs: []string{"http://forgejo.localhost:17480"}},
		}).GitClone(Request{URL: "http://forgejo.localhost:17480/tta-lab/example.git"})
		if err != nil {
			t.Fatal(err)
		}
		if got.Token != "forgejo-token" || got.Provider != "forgejo" || strings.Contains(got.Remote, "forgejo-token") {
			t.Fatalf("clone invocation = %+v", got)
		}
	})

	t.Run("generic ignores ambient tokens", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("FORGEJO_TOKEN", "forgejo-token")
		t.Setenv("GITHUB_TOKEN", "ambient-pat")
		var got cloneInvocation
		withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
			got = invocation
			return createClonedRepository(ctx, invocation)
		})
		_, err := NewServiceWithConfig(nil, writeCloneProjects(t, ""), ogconfig.Config{}).GitClone(Request{
			URL: "https://codeberg.org/tta-lab/example.git",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.Token != "" || got.Provider != "generic" {
			t.Fatalf("clone invocation = %+v", got)
		}
	})
}

func TestGitCloneRejectsDestinationConflictsWithoutMutation(t *testing.T) {
	t.Run("different origin", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		destination := filepath.Join(home, "code", "projects", "tta-lab", "example")
		initCloneRepository(t, destination, "https://codeberg.org/other/example.git")
		calls := 0
		withCloneRunner(t, func(context.Context, cloneInvocation) error { calls++; return nil })
		_, err := NewServiceWithConfig(nil, writeCloneProjects(t, ""), ogconfig.Config{}).GitClone(Request{
			URL: "https://codeberg.org/tta-lab/example.git",
		})
		if err == nil || !strings.Contains(err.Error(), "does not match") || calls != 0 {
			t.Fatalf("GitClone error = %v, runner calls = %d", err, calls)
		}
	})

	t.Run("non repository", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		destination := filepath.Join(home, "code", "projects", "tta-lab", "example")
		if err := os.MkdirAll(destination, 0o755); err != nil {
			t.Fatal(err)
		}
		store := writeCloneProjects(t, "")
		withCloneRunner(t, func(context.Context, cloneInvocation) error {
			t.Fatal("clone runner called for existing non-repository")
			return nil
		})
		_, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
			URL: "https://codeberg.org/tta-lab/example.git",
		})
		if err == nil {
			t.Fatal("GitClone accepted an existing non-repository")
		}
		if _, getErr := store.Get("example"); !errors.Is(getErr, project.ErrNotFound) {
			t.Fatalf("existing non-repository was registered: %v", getErr)
		}
	})

	t.Run("destination symlink", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		destination := filepath.Join(home, "code", "projects", "tta-lab", "example")
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), destination); err != nil {
			t.Fatal(err)
		}
		_, err := NewServiceWithConfig(nil, writeCloneProjects(t, ""), ogconfig.Config{}).GitClone(Request{
			URL: "https://codeberg.org/tta-lab/example.git",
		})
		if err == nil || !strings.Contains(err.Error(), "safe directory") {
			t.Fatalf("GitClone error = %v, want destination symlink rejection", err)
		}
	})

	t.Run("wrong top-level", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		parent := filepath.Join(home, "code", "projects", "tta-lab")
		destination := filepath.Join(parent, "example")
		gitRun(t, "", "init", parent)
		gitRun(t, parent, "remote", "add", "origin", "https://codeberg.org/tta-lab/example.git")
		if err := os.MkdirAll(destination, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := NewServiceWithConfig(nil, writeCloneProjects(t, ""), ogconfig.Config{}).GitClone(Request{
			URL: "https://codeberg.org/tta-lab/example.git",
		})
		if err == nil || !strings.Contains(err.Error(), "top-level") {
			t.Fatalf("GitClone error = %v, want top-level mismatch", err)
		}
	})

}

func TestGitCloneLeavesCompletedCheckoutWhenRegistrationFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	destination := filepath.Join(home, "code", "projects", "tta-lab", "example")
	registry := filepath.Join(t.TempDir(), "projects.toml")
	if err := os.WriteFile(registry, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	store := project.NewStore(registry)
	withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
		if err := createClonedRepository(ctx, invocation); err != nil {
			return err
		}
		return os.WriteFile(registry, []byte("[example]\npath = \"/other/repository\"\n"+
			"remote = \"https://codeberg.org/other/repository.git\"\n"), 0o644)
	})

	_, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
		URL: "https://codeberg.org/tta-lab/example.git",
	})
	if err == nil || !strings.Contains(err.Error(), "alias") {
		t.Fatalf("GitClone error = %v, want alias collision", err)
	}
	if _, statErr := os.Stat(filepath.Join(destination, ".git")); statErr != nil {
		t.Fatalf("completed checkout was removed: %v", statErr)
	}
}

func TestGitCloneRejectsExistingAliasBeforeClone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store := writeCloneProjects(t, "[example]\npath = \"/other/repository\"\n"+
		"remote = \"https://codeberg.org/other/repository.git\"\n")
	withCloneRunner(t, func(context.Context, cloneInvocation) error {
		t.Fatal("clone runner called for an existing alias")
		return nil
	})

	_, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
		URL: "https://codeberg.org/tta-lab/example.git",
	})
	if err == nil || !strings.Contains(err.Error(), "alias") {
		t.Fatalf("GitClone error = %v, want alias collision", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, "code")); !os.IsNotExist(statErr) {
		t.Fatalf("clone mutated filesystem before rejecting alias: %v", statErr)
	}
}

func TestGitCloneCancellationCleansTemporaryCheckout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store := writeCloneProjects(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	withCloneRunner(t, func(_ context.Context, invocation cloneInvocation) error {
		if err := os.WriteFile(filepath.Join(invocation.Destination, "partial"), []byte("x"), 0o644); err != nil {
			return err
		}
		cancel()
		return context.Canceled
	})

	_, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{
		Context: ctx,
		URL:     "https://codeberg.org/tta-lab/example.git",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GitClone error = %v, want context canceled", err)
	}
	parent := filepath.Join(home, "code", "projects", "tta-lab")
	entries, readErr := os.ReadDir(parent)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary checkout was not cleaned: %+v", entries)
	}
}

func TestRunGitCloneUsesAnonymousEnvironmentForPublicForgejo(t *testing.T) {
	bin := t.TempDir()
	capture := filepath.Join(t.TempDir(), "environment")
	script := "#!/bin/sh\nenv > \"$CLONE_ENV_CAPTURE\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLONE_ENV_CAPTURE", capture)
	t.Setenv("FORGEJO_TOKEN", "ambient-token")

	err := runGitClone(context.Background(), cloneInvocation{
		Destination: filepath.Join(t.TempDir(), "checkout"),
		Remote:      "http://forgejo.localhost:17480/tta-lab/example.git",
		Provider:    "forgejo",
		Owner:       "tta-lab",
		Repo:        "example",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	environment := string(data)
	if strings.Contains(environment, "ambient-token") || strings.Contains(environment, "GIT_TOKEN_INJECT=") {
		t.Fatal("public Forgejo clone retained credentials")
	}
	if !strings.Contains(environment, "GIT_CONFIG_COUNT=3") ||
		!strings.Contains(environment, "GIT_CONFIG_KEY_1=core.askPass") {
		t.Fatal("public Forgejo clone did not use anonymous Git environment")
	}
}

func TestRunGitCloneDoesNotExposeInjectedTokenInError(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s' \"$GIT_TOKEN_INJECT\" >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	const token = "repo-scoped-secret"

	err := runGitClone(context.Background(), cloneInvocation{
		Destination: filepath.Join(t.TempDir(), "checkout"),
		Remote:      "https://github.com/tta-lab/example.git",
		Provider:    "github",
		Owner:       "tta-lab",
		Repo:        "example",
		Token:       token,
	})
	if err == nil {
		t.Fatal("runGitClone succeeded, want failure")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("clone error exposed injected token: %v", err)
	}
}

func writeCloneProjects(t *testing.T, content string) *project.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "projects.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return project.NewStore(path)
}

func TestGitCloneByProjectReferenceUsesRegisteredPathAndRemote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	destination := filepath.Join(home, "custom", "organon")
	store := writeCloneProjects(t, "[orga]\npath = "+quoteTOML(destination)+
		"\nremote = \"https://github.com/tta-lab/organon.git\"\n")
	var got cloneInvocation
	withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
		got = invocation
		return createClonedRepository(ctx, invocation)
	})

	resp, err := NewServiceWithConfig(nil, store, ogconfig.Config{}).GitClone(Request{Project: "organon"})
	if err != nil {
		t.Fatalf("GitClone project reference: %v", err)
	}
	assertTemporaryCloneInvocationFor(t, got, destination, "https://github.com/tta-lab/organon.git")
	if resp.Clone == nil || resp.Clone.Alias != "orga" || resp.Clone.Path != destination ||
		!resp.Clone.Registered || resp.Clone.Archived {
		t.Fatalf("clone result = %+v", resp.Clone)
	}
}

func assertTemporaryCloneInvocationFor(t *testing.T, got cloneInvocation, wantPath, wantRemote string) {
	t.Helper()
	if got.Destination == wantPath || filepath.Dir(got.Destination) != filepath.Dir(wantPath) {
		t.Fatalf("temporary destination = %q, want sibling of %q", got.Destination, wantPath)
	}
	if got.Remote != wantRemote {
		t.Fatalf("clone remote = %q, want %q", got.Remote, wantRemote)
	}
}

func TestGitCloneRejectsInvalidSelectorCombinations(t *testing.T) {
	service := NewServiceWithConfig(nil, writeCloneProjects(t, ""), ogconfig.Config{})
	for _, req := range []Request{
		{},
		{Project: "orga", URL: "https://github.com/tta-lab/organon.git"},
		{Project: "orga", Alias: "other"},
		{Project: "orga", Reference: true},
	} {
		if _, err := service.GitClone(req); err == nil {
			t.Fatalf("GitClone(%+v) succeeded, want selector rejection", req)
		}
	}
}

func TestGitCloneAllowsSymlinkedParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "code"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "code", "projects")); err != nil {
		t.Fatal(err)
	}
	withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
		return createClonedRepository(ctx, invocation)
	})

	service := NewServiceWithConfig(nil, writeCloneProjects(t, ""), ogconfig.Config{})
	resp, err := service.GitClone(Request{
		URL: "https://codeberg.org/tta-lab/example.git",
	})
	if err != nil {
		t.Fatalf("GitClone: %v", err)
	}
	if resp.Clone == nil || !resp.Clone.Registered {
		t.Fatalf("clone result = %+v", resp.Clone)
	}
	ctx, err := service.resolveRemoteRepoContextFor(context.Background(), resp.Clone.Path)
	if err != nil {
		t.Fatalf("resolve cloned project through symlink parent: %v", err)
	}
	if ctx.ProjectAlias != "example" {
		t.Fatalf("resolved context = %+v", ctx)
	}
}

func TestGitCloneAcceptsExistingLinkedWorktree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	base := filepath.Join(t.TempDir(), "base")
	destination := filepath.Join(home, "code", "projects", "tta-lab", "example")
	gitRun(t, "", "init", base)
	gitRun(t, base, "config", "user.email", "test@example.com")
	gitRun(t, base, "config", "user.name", "Test User")
	gitRun(t, base, "commit", "--allow-empty", "-m", "initial")
	gitRun(t, base, "remote", "add", "origin", "https://codeberg.org/tta-lab/example.git")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, base, "worktree", "add", "-b", "linked", destination)
	withCloneRunner(t, func(context.Context, cloneInvocation) error {
		t.Fatal("clone runner called for existing worktree")
		return nil
	})

	resp, err := NewServiceWithConfig(nil, writeCloneProjects(t, ""), ogconfig.Config{}).GitClone(Request{
		URL: "https://codeberg.org/tta-lab/example.git",
	})
	if err != nil {
		t.Fatalf("GitClone linked worktree: %v", err)
	}
	if resp.Clone == nil || !resp.Clone.AlreadyExisted || !resp.Clone.Registered {
		t.Fatalf("clone result = %+v", resp.Clone)
	}
}

func withCloneRunner(t *testing.T, runner func(context.Context, cloneInvocation) error) {
	t.Helper()
	old := runGitCloneFunc
	runGitCloneFunc = runner
	t.Cleanup(func() { runGitCloneFunc = old })
}

func createClonedRepository(ctx context.Context, invocation cloneInvocation) error {
	cmd := exec.CommandContext(ctx, "git", "init", invocation.Destination)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New(string(out))
	}
	cmd = exec.CommandContext(ctx, "git", "-C", invocation.Destination, "remote", "add", "origin", invocation.Remote)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New(string(out))
	}
	return nil
}

func initCloneRepository(t *testing.T, destination, remote string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	invocation := cloneInvocation{Destination: destination, Remote: remote}
	if err := createClonedRepository(context.Background(), invocation); err != nil {
		t.Fatal(err)
	}
}

func quoteTOML(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
