package og

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

func linkedFixture(t *testing.T) (string, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORGEJO_TOKEN", "fixture-token")
	repo := testRegisteredRepo(t, home, "main", "https://forge.example/tta-lab/example.git", false)
	worktree := filepath.Join(t.TempDir(), "feature")
	gitRun(t, repo, "worktree", "add", "-b", "feature/merge", worktree)
	sha := gitOut(t, repo, "rev-parse", "HEAD")
	gitRun(t, repo, "update-ref", "refs/remotes/origin/main", sha)
	gitRun(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	gitRun(t, repo, "update-ref", "refs/remotes/origin/feature/merge", sha)
	return repo, worktree, sha
}

func TestLinkedImpriReceiptRecovery(t *testing.T) { //nolint:gocyclo
	repo, worktree, sha := linkedFixture(t)
	var payload any
	reports, merges := 0, 0
	merged := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/actions":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			payload = body["payload"]
			writeMergeAction(t, w, "act-linked", PRMergeStatusApproved, payload)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/actions/act-linked":
			writeMergeAction(t, w, "act-linked", PRMergeStatusApproved, payload)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/actions/act-linked/result":
			reports++
			if reports == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: func(_, _ string, id int64) (*gitprovider.PullRequest, error) {
				state := "open"
				if merged {
					state = "closed"
				}
				return &gitprovider.PullRequest{Index: id, Head: "feature/merge", HeadSHA: sha, Base: "main",
					State: state, Merged: merged, Mergeable: true, HTMLURL: "https://forge.example/tta-lab/example/pull/7"}, nil
			},
			getCombinedStatus: func(_, _, _ string) (*gitprovider.CombinedStatus, error) {
				return &gitprovider.CombinedStatus{State: "success"}, nil
			},
			mergePR: func(_, _ string, _ int64, _ string) error { merges++; merged = true; return nil },
		}, nil
	}))
	t.Cleanup(stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		if args[0] == "push" {
			gitRun(t, repo, "update-ref", "-d", "refs/remotes/origin/feature/merge")
		}
		return nil
	}))
	cfg := ogconfig.Config{Forgejo: ogconfig.ForgejoConfig{AllowedBaseURLs: []string{"https://forge.example"}},
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "fixture"}}
	svc := NewServiceWithConfig(nil, nil, cfg)
	req := Request{WorkDir: worktree, Index: 7}
	result, err := svc.PRMerge(req)
	if result.Merge == nil || result.Merge.ReceiptError == "" || merges != 1 {
		t.Fatalf("receipt failure: %+v %v", result.Merge, err)
	}
	if _, err := os.Stat(worktree); err != nil {
		t.Fatal(err)
	}
	if got := gitOut(t, worktree, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature/merge" {
		t.Fatalf("branch %s", got)
	}
	result, err = svc.PRMerge(req)
	if err != nil || result.Merge.CleanupError != "" || merges != 1 || reports != 2 {
		t.Fatalf("recovery: %+v %v merges=%d reports=%d", result.Merge, err, merges, reports)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree exists: %v", err)
	}
}

func TestResolveLinkedWorktree(t *testing.T) {
	repo, worktree, _ := linkedFixture(t)
	store := project.NewStore(config.ProjectsPath())
	root, entry, err := ResolveWorktree(context.Background(), worktree, store)
	if err != nil || root != worktree || entry.Alias != "test" {
		t.Fatalf("resolve = %q %+v %v", root, entry, err)
	}
	nested := filepath.Join(worktree, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	root, entry, err = resolveRegisteredRepo(context.Background(), nested, store)
	if err != nil || root != worktree || entry.Alias != "test" {
		t.Fatalf("nested resolve = %q %+v %v", root, entry, err)
	}
	for _, invalid := range []string{nested, repo, "relative", filepath.Join(worktree, "missing")} {
		if _, _, err := ResolveWorktree(context.Background(), invalid, store); err == nil {
			t.Errorf("accepted %q", invalid)
		}
	}
	clone := filepath.Join(t.TempDir(), "clone")
	gitRun(t, "", "clone", "--quiet", repo, clone)
	gitRun(t, clone, "remote", "set-url", "origin", "https://forge.example/tta-lab/example.git")
	if _, _, err := ResolveWorktree(context.Background(), clone, store); err == nil {
		t.Fatal("accepted independent clone")
	}
}

func TestMergeCleanupRejectsLostNestedWorktreeIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredRepo(t, home, "main", "https://forge.example/tta-lab/example.git", false)
	worktree := filepath.Join(repo, "nested-worktree")
	gitRun(t, repo, "worktree", "add", "-b", "feature/nested", worktree)
	service := NewService(nil)
	selected, err := service.resolveRemoteRepoContextFor(context.Background(), worktree)
	if err != nil || !selected.LinkedWorktree {
		t.Fatalf("initial linked context = %+v, %v", selected, err)
	}
	if err := os.Remove(filepath.Join(worktree, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.resolveMergeCleanupContext(selected); err == nil || !strings.Contains(err.Error(), "same root") {
		t.Fatalf("cleanup resolution should reject lost worktree identity: %v", err)
	}
	if got := gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Fatalf("registered branch changed: %s", got)
	}
	if _, err := os.Stat(worktree); err != nil {
		t.Fatalf("selected directory was not preserved: %v", err)
	}
}

func TestLinkedPushAndPullUseSelectedBranch(t *testing.T) {
	_, worktree, _ := linkedFixture(t)
	var calls []string
	t.Cleanup(stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}))
	cfg := ogconfig.Config{Forgejo: ogconfig.ForgejoConfig{AllowedBaseURLs: []string{"https://forge.example"}}}
	svc := NewServiceWithConfig(nil, nil, cfg)
	if _, err := svc.GitPush(Request{WorkDir: worktree}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GitPull(Request{WorkDir: worktree}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, ";") != "push -u origin feature/merge;pull --ff-only origin feature/merge" {
		t.Fatalf("calls: %v", calls)
	}
}

func TestGitTagRejectsLinkedWorktreeWithoutMutation(t *testing.T) {
	repo, worktree, _ := linkedFixture(t)
	called := false
	t.Cleanup(stubRunGitWithCreds(t, func(_ *repoContext, _ ...string) error {
		called = true
		return nil
	}))
	cfg := ogconfig.Config{Forgejo: ogconfig.ForgejoConfig{AllowedBaseURLs: []string{"https://forge.example"}}}
	_, err := NewServiceWithConfig(nil, nil, cfg).GitTag(Request{WorkDir: worktree, Tag: "v1.2.3"})
	if err == nil || !strings.Contains(err.Error(), "--project") {
		t.Fatalf("expected registered checkout guidance, got %v", err)
	}
	if called {
		t.Fatal("attempted remote Git operation")
	}
	if gitCmd(repo, "show-ref", "--verify", "--quiet", "refs/tags/v1.2.3") == nil {
		t.Fatal("created local tag")
	}
}

func TestLinkedPRFindUsesSelectedBranch(t *testing.T) {
	_, worktree, _ := linkedFixture(t)
	var head string
	t.Cleanup(stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{findPRByState: func(_, _, branch, _, _ string) (*gitprovider.PullRequest, error) {
			head = branch
			return &gitprovider.PullRequest{Index: 7, Head: branch}, nil
		}}, nil
	}))
	cfg := ogconfig.Config{Forgejo: ogconfig.ForgejoConfig{AllowedBaseURLs: []string{"https://forge.example"}}}
	if _, err := NewServiceWithConfig(nil, nil, cfg).PRFind(Request{WorkDir: worktree}); err != nil {
		t.Fatal(err)
	}
	if head != "feature/merge" {
		t.Fatalf("selected PR branch %q", head)
	}
}

func TestLinkedCleanupRefusesHeadCheckedOutElsewhere(t *testing.T) {
	repo, worktree, sha := linkedFixture(t)
	gitRun(t, worktree, "switch", "--detach")
	other := filepath.Join(t.TempDir(), "other")
	gitRun(t, repo, "worktree", "add", other, "feature/merge")
	ctx := &repoContext{Context: context.Background(), WorkDir: worktree, RegisteredDir: repo}
	_, err := checkLinkedCleanup(ctx, branchCleanupTarget{HeadBranch: "feature/merge", HeadSHA: sha}, true)
	if err == nil || !strings.Contains(err.Error(), "checked out in another worktree") {
		t.Fatalf("expected checkout protection, got %v", err)
	}
	if err := deleteCleanupLocal(ctx, branchCleanupTarget{
		HeadBranch: "feature/merge", HeadSHA: sha, RequireExactHeadSHA: true,
	}); err == nil {
		t.Fatal("deleted ref checked out in another worktree")
	}
	if got := gitOut(t, other, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature/merge" {
		t.Fatalf("other branch changed: %s", got)
	}
}

func TestLinkedRefDeletionRejectsMovedBranch(t *testing.T) {
	repo, worktree, sha := linkedFixture(t)
	gitRun(t, worktree, "switch", "--detach")
	newSHA := gitOut(t, repo, "commit-tree", sha+"^{tree}", "-p", sha, "-m", "moved")
	gitRun(t, repo, "update-ref", "refs/heads/feature/merge", newSHA)
	ctx := &repoContext{Context: context.Background(), WorkDir: worktree, RegisteredDir: repo}
	if err := deleteCleanupLocal(ctx, branchCleanupTarget{
		HeadBranch: "feature/merge", HeadSHA: sha, RequireExactHeadSHA: true,
	}); err == nil {
		t.Fatal("deleted moved branch")
	}
	if got := gitOut(t, repo, "rev-parse", "refs/heads/feature/merge"); got != newSHA {
		t.Fatalf("branch changed: %s", got)
	}
}

func TestLinkedDirectMergeAndRecovery(t *testing.T) { //nolint:gocyclo
	for _, scenario := range []string{"success", "dirty", "tracked", "locked", "moved", "wrong-branch", "dry",
		"retry", "locked-after-push", "dirty-after-merge", "remote-moved-after-merge", "moved-after-merge"} {
		t.Run(scenario, func(t *testing.T) {
			repo, worktree, sha := linkedFixture(t)
			if scenario == "dirty" {
				gitRun(t, worktree, "config", "status.showUntrackedFiles", "no")
				if err := os.WriteFile(filepath.Join(worktree, "untracked"), []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "moved" {
				gitRun(t, worktree, "commit", "--allow-empty", "-m", "moved")
			}
			if scenario == "wrong-branch" {
				gitRun(t, worktree, "switch", "-c", "other")
			}
			if scenario == "tracked" {
				gitRun(t, worktree, "commit", "--allow-empty", "-m", "fixture")
				sha = gitOut(t, worktree, "rev-parse", "HEAD")
				gitRun(t, repo, "update-ref", "refs/remotes/origin/feature/merge", sha)
				if err := os.WriteFile(filepath.Join(worktree, "tracked"), []byte("one"), 0600); err != nil {
					t.Fatal(err)
				}
				gitRun(t, worktree, "add", "tracked")
				gitRun(t, worktree, "commit", "-m", "add tracked")
				sha = gitOut(t, worktree, "rev-parse", "HEAD")
				gitRun(t, repo, "update-ref", "refs/remotes/origin/feature/merge", sha)
				if err := os.WriteFile(filepath.Join(worktree, "tracked"), []byte("two"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "locked" {
				gitRun(t, repo, "worktree", "lock", worktree)
			}
			if scenario == "success" {
				gitRun(t, repo, "config", "core.excludesFile", filepath.Join(t.TempDir(), "excludes"))
				if err := os.WriteFile(filepath.Join(repo, ".git", "info", "exclude"), []byte("ignored\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(worktree, "ignored"), []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			merged, merges, pushes := false, 0, 0
			t.Cleanup(stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
				return fakeProvider{
					getPR: func(_, _ string, id int64) (*gitprovider.PullRequest, error) {
						state := "open"
						if merged {
							state = "closed"
						}
						return &gitprovider.PullRequest{Index: id, Head: "feature/merge", HeadSHA: sha, Base: "main",
							State: state, Merged: merged, Mergeable: true, HTMLURL: "https://forge.example/tta-lab/example/pull/7"}, nil
					},
					getCombinedStatus: func(_, _, _ string) (*gitprovider.CombinedStatus, error) {
						return &gitprovider.CombinedStatus{State: "success"}, nil
					},
					mergePR: func(_, _ string, _ int64, _ string) error {
						merged = true
						merges++
						if scenario == "dirty-after-merge" {
							if err := os.WriteFile(filepath.Join(worktree, "untracked"), []byte("x"), 0600); err != nil {
								t.Fatal(err)
							}
						}
						if scenario == "remote-moved-after-merge" {
							other := gitOut(t, repo, "commit-tree", sha+"^{tree}", "-p", sha, "-m", "other")
							gitRun(t, repo, "update-ref", "refs/remotes/origin/feature/merge", other)
						}
						if scenario == "moved-after-merge" {
							gitRun(t, worktree, "commit", "--allow-empty", "-m", "concurrent commit")
						}
						return nil
					},
				}, nil
			}))
			t.Cleanup(stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
				if args[0] == "push" {
					lease := "--force-with-lease=refs/heads/feature/merge:" + sha
					if !strings.Contains(strings.Join(args, " "), lease) {
						t.Fatalf("missing exact lease: %v", args)
					}
					pushes++
					if scenario == "retry" && pushes == 1 {
						return errors.New("temporary push failure")
					}
					gitRun(t, repo, "update-ref", "-d", "refs/remotes/origin/feature/merge")
					if scenario == "locked-after-push" && pushes == 1 {
						gitRun(t, repo, "worktree", "lock", worktree)
					}
				}
				return nil
			}))
			cfg := ogconfig.Config{Forgejo: ogconfig.ForgejoConfig{AllowedBaseURLs: []string{"https://forge.example"}},
				Merge: ogconfig.MergeConfig{Approval: "none"}}
			svc := NewServiceWithConfig(nil, nil, cfg)
			req := Request{WorkDir: worktree, Index: 7, DryRun: scenario == "dry"}
			result, err := svc.PRMerge(req)
			if scenario == "dirty" || scenario == "tracked" || scenario == "locked" ||
				scenario == "moved" || scenario == "wrong-branch" {
				if err == nil || merges != 0 {
					t.Fatalf("unsafe merge: %+v %v merges=%d", result.Merge, err, merges)
				}
				return
			}
			if scenario == "dirty-after-merge" || scenario == "remote-moved-after-merge" || scenario == "moved-after-merge" {
				if err == nil || result.Merge.CleanupError == "" || merges != 1 {
					t.Fatalf("dirty after merge: %+v %v", result.Merge, err)
				}
				if _, err := os.Stat(worktree); err != nil {
					t.Fatal(err)
				}
				return
			}
			if scenario == "retry" || scenario == "locked-after-push" {
				if err == nil || result.Merge.CleanupError == "" || !strings.Contains(result.Merge.Completion, worktree) {
					t.Fatalf("partial result %+v %v", result.Merge, err)
				}
				if got := gitOut(t, worktree, "rev-parse", "--abbrev-ref", "HEAD"); got != "HEAD" {
					t.Fatalf("expected detached: %s", got)
				}
				if scenario == "locked-after-push" {
					gitRun(t, repo, "worktree", "unlock", worktree)
				}
				result, err = svc.PRMerge(req)
			}
			if err != nil {
				t.Fatalf("merge: %+v %v", result.Merge, err)
			}
			if scenario == "dry" {
				if merges != 0 {
					t.Fatal("dry-run merged")
				}
				if _, err := os.Stat(worktree); err != nil {
					t.Fatal(err)
				}
				return
			}
			if merges != 1 {
				t.Fatalf("merges=%d", merges)
			}
			if _, err := os.Stat(worktree); !os.IsNotExist(err) {
				t.Fatalf("worktree still exists: %v", err)
			}
			if got := gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
				t.Fatalf("registered branch %s", got)
			}
			if got := gitOut(t, repo, "rev-parse", "HEAD"); got != sha {
				t.Fatalf("registered HEAD %s", got)
			}
			if gitCmd(repo, "show-ref", "--verify", "--quiet", "refs/heads/feature/merge") == nil {
				t.Fatal("local ref remains")
			}
		})
	}
}
