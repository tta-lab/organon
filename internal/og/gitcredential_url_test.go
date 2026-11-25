//go:build !windows

package og

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/gitutil"
	"github.com/tta-lab/organon/internal/ogconfig"
)

func isolatedURLCredentials(t *testing.T, credentials map[string]string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(home, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	wrapper := fmt.Sprintf("#!/bin/sh\nexport GIT_CONFIG_NOSYSTEM=1\nexec %q \"$@\"\n", git)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	config := fmt.Sprintf("[credential]\n helper = store --file %s\n useHttpPath = true\n", filepath.Join(home, "store"))
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	for remote, token := range credentials {
		cmd := exec.Command("git", "credential", "approve")
		cmd.Stdin = strings.NewReader("url=" + remote + "\nusername=fixture\npassword=" + token + "\n\n")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("approve: %v: %s", err, out)
		}
	}
	return home
}

func TestGitHubCredentialUsesEffectiveFetchURLForAPI(t *testing.T) {
	remote := "https://GitHub.COM:443/Owner/Repo"
	home := isolatedURLCredentials(t, map[string]string{remote: "api-fixture-token"})
	repo := testRegisteredRepo(t, home, branchMain, remote, false)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer api-fixture-token" {
			t.Error("unexpected API target or credential")
		}
		body := `{"number":7,"title":"fixture","state":"open"}`
		return &http.Response{StatusCode: 200, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	result, err := NewService(nil).PRGet(Request{WorkDir: repo, Index: 7})
	if err != nil || result.PR == nil || result.PR.Index != 7 {
		t.Fatalf("PR get: %v", err)
	}
}

func TestGitHubCloneUsesInputURLCredentialAndCanonicalMetadata(t *testing.T) {
	remote := "https://github.com/Owner/Repo"
	isolatedURLCredentials(t, map[string]string{remote: "clone-fixture-token"})
	var got cloneInvocation
	withCloneRunner(t, func(ctx context.Context, invocation cloneInvocation) error {
		got = invocation
		return createClonedRepository(ctx, invocation)
	})
	result, err := NewServiceWithConfig(nil, writeCloneProjects(t, ""), ogconfig.Config{}).GitClone(Request{URL: remote})
	if err != nil {
		t.Fatal(err)
	}
	if got.Remote != remote || got.Token != "clone-fixture-token" {
		t.Fatal("clone did not use the input credential")
	}
	if result.Clone == nil || result.Clone.Remote != "https://github.com/owner/repo.git" || result.Clone.Owner != "owner" {
		t.Fatalf("clone identity: %+v", result.Clone)
	}
}

func TestGitHubPushUsesEveryValidatedPushURLCredential(t *testing.T) {
	fetch := "https://github.com/owner/repo.git"
	first := "https://github.com/Owner/Repo"
	second := "https://GitHub.COM:443/owner/Repo.git"
	home := isolatedURLCredentials(t, map[string]string{first: "first-fixture-token", second: "second-fixture-token"})
	repo := testRegisteredRepo(t, home, "feature/x", fetch, false)
	gitRun(t, repo, "remote", "set-url", "--add", "--push", "origin", first)
	gitRun(t, repo, "remote", "set-url", "--add", "--push", "origin", second)
	old := runGitWithCredsFunc
	t.Cleanup(func() { runGitWithCredsFunc = old })
	runGitWithCredsFunc = func(info *repoContext, auth gitAuthentication, args ...string) error {
		if !reflect.DeepEqual(args, []string{"push", "-u", "origin", "feature/x"}) {
			t.Fatalf("git args: %v", args)
		}
		env := gitutil.GitHubGitEnv(os.Environ(), auth.targets)
		for remote, token := range map[string]string{first: "first-fixture-token", second: "second-fixture-token"} {
			cmd := exec.Command("git", "credential", "fill")
			cmd.Env = env
			cmd.Stdin = strings.NewReader("url=" + remote + "\n\n")
			out, err := cmd.Output()
			if err != nil || !strings.Contains(string(out), "password="+token) {
				t.Fatalf("wrong scoped credential for %s: %v", remote, err)
			}
		}
		for _, remote := range []string{"https://evil.example/Owner/Repo", "https://github.com/Owner/Other"} {
			cmd := exec.Command("git", "credential", "fill")
			cmd.Env = env
			cmd.Stdin = strings.NewReader("url=" + remote + "\n\n")
			out, _ := cmd.Output()
			if strings.Contains(string(out), "fixture-token") {
				t.Fatal("credential escaped its validated target")
			}
		}
		cmd := exec.Command("git", "-C", info.WorkDir, "remote", "get-url", "--push", "--all", "origin")
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil || strings.TrimSpace(string(out)) != first+"\n"+second {
			t.Fatalf("push destinations: %s, %v", out, err)
		}
		return nil
	}
	if _, err := NewService(nil).GitPush(Request{WorkDir: repo}); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubFetchUsesEffectiveURLCredential(t *testing.T) {
	remote := "https://github.com/Owner/Repo"
	home := isolatedURLCredentials(t, map[string]string{remote: "fetch-fixture-token"})
	repo := testRegisteredRepo(t, home, branchMain, remote, false)
	old := runGitWithCredsFunc
	t.Cleanup(func() { runGitWithCredsFunc = old })
	runGitWithCredsFunc = func(info *repoContext, auth gitAuthentication, args ...string) error {
		if info.RemoteURL != remote || !reflect.DeepEqual(args, []string{"pull", "--ff-only", "origin", branchMain}) {
			t.Fatalf("fetch target or args: %s %v", info.RemoteURL, args)
		}
		cmd := exec.Command("git", "credential", "fill")
		cmd.Env = gitutil.GitHubGitEnv(os.Environ(), auth.targets)
		cmd.Stdin = strings.NewReader("url=" + remote + "\n\n")
		out, err := cmd.Output()
		if err != nil || !strings.Contains(string(out), "password=fetch-fixture-token") {
			t.Fatalf("fetch credential: %v", err)
		}
		return nil
	}
	if _, err := NewService(nil).GitPull(Request{WorkDir: repo}); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubPushRejectsAnyUnvalidatedPushURLBeforeLookup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredRepo(t, home, "feature/x", "https://github.com/owner/repo.git", false)
	gitRun(t, repo, "remote", "set-url", "--add", "--push", "origin", "https://github.com/Owner/Repo")
	gitRun(t, repo, "remote", "set-url", "--add", "--push", "origin", "https://evil.example/Owner/Repo")
	broker := &recordingBroker{}
	if _, err := NewService(broker).GitPush(Request{WorkDir: repo}); err == nil {
		t.Fatal("push accepted an unvalidated destination")
	}
	if len(broker.tokenCalls) != 0 {
		t.Fatal("credentials requested before validating all push URLs")
	}
}
