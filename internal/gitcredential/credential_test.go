//go:build !windows

package gitcredential

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func isolatedGit(t *testing.T) string {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	wrapper := fmt.Sprintf("#!/bin/sh\nexport GIT_CONFIG_NOSYSTEM=1\nexec %q \"$@\"\n", realGit)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	return home
}

func TestGitUsesUserHelperAndIgnoresInjectedConfiguration(t *testing.T) {
	home := isolatedGit(t)
	marker := filepath.Join(home, "called")
	helper := filepath.Join(home, "helper")
	script := fmt.Sprintf("#!/bin/sh\necho called >> %q\necho username=fixture\necho password=synthetic-secret\n", marker)
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("[credential]\n helper = %s\n", helper)
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "credential.helper")
	t.Setenv("GIT_CONFIG_VALUE_0", "!echo password=wrong-secret")
	token, err := (Git{}).Token(context.Background(), "owner", "repo", "https://github.com/owner/repo.git", PurposeAPI)
	if err != nil || token != "synthetic-secret" {
		t.Fatalf("token result error=%v, correct=%v", err, token == "synthetic-secret")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("user helper was not called")
	}
}

func TestGitUsesStoredHostAndPathCredentials(t *testing.T) {
	for _, tc := range []struct {
		name       string
		usePath    bool
		otherMatch bool
	}{
		{name: "default host scope", otherMatch: true},
		{name: "configured path scope", usePath: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := isolatedGit(t)
			store := filepath.Join(home, "credentials")
			config := fmt.Sprintf("[credential]\n helper = store --file %s\n", store)
			if tc.usePath {
				config += " useHttpPath = true\n"
			}
			if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			approve := exec.Command("git", "credential", "approve")
			approve.Stdin = strings.NewReader("protocol=https\nhost=github.com\npath=owner/repo.git\n" +
				"username=fixture\npassword=synthetic-secret\n\n")
			if out, err := approve.CombinedOutput(); err != nil {
				t.Fatalf("credential approve: %v: %s", err, out)
			}
			token, err := (Git{}).Token(context.Background(), "owner", "repo", "https://github.com/owner/repo.git", PurposeAPI)
			if err != nil || token != "synthetic-secret" {
				t.Fatalf("matching token: error=%v correct=%v", err, token == "synthetic-secret")
			}
			_, err = (Git{}).Token(context.Background(), "owner", "other", "https://github.com/owner/other.git", PurposeAPI)
			if (err == nil) != tc.otherMatch {
				t.Fatalf("other repository: error=%v, want match=%v", err, tc.otherMatch)
			}
		})
	}
}

func TestGitUsesExactAcceptedURLSpelling(t *testing.T) {
	for _, remote := range []string{
		"https://github.com/Owner/Repo", "https://GitHub.COM:443/Owner/Repo.git",
	} {
		t.Run(remote, func(t *testing.T) {
			home := isolatedGit(t)
			config := fmt.Sprintf("[credential]\n helper = store --file %s\n useHttpPath = true\n",
				filepath.Join(home, "credentials"))
			if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			approve := exec.Command("git", "credential", "approve")
			approve.Stdin = strings.NewReader("url=" + remote + "\nusername=fixture\npassword=synthetic-token\n\n")
			if out, err := approve.CombinedOutput(); err != nil {
				t.Fatalf("approve: %v: %s", err, out)
			}
			token, err := (Git{}).Token(context.Background(), "owner", "repo", remote, PurposeAPI)
			if err != nil || token != "synthetic-token" {
				t.Fatalf("lookup error=%v correct=%v", err, token == "synthetic-token")
			}
		})
	}
}

func TestGitMissingAndInvalidIdentityAreSecretFree(t *testing.T) {
	isolatedGit(t)
	_, err := (Git{}).Token(context.Background(), "owner", "repo", "https://github.com/owner/repo.git", PurposeAPI)
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("missing credential: %v", err)
	}
	_, err = (Git{}).Token(context.Background(), "owner\npassword=secret", "repo",
		"https://github.com/owner/repo.git", PurposeAPI)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("invalid identity error: %v", err)
	}
}

func TestGitRejectsHelperDestinationChange(t *testing.T) {
	home := isolatedGit(t)
	helper := filepath.Join(home, "helper")
	script := "#!/bin/sh\necho host=evil.example\necho username=fixture\necho password=synthetic-secret\n"
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("[credential]\n helper = %s\n", helper)
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := (Git{}).Token(context.Background(), "owner", "repo", "https://github.com/owner/repo.git", PurposeAPI)
	if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatalf("destination: %v", err)
	}
}

func TestGitRejectsHelperPathChange(t *testing.T) {
	home := isolatedGit(t)
	helper := filepath.Join(home, "helper")
	script := "#!/bin/sh\necho path=owner/other.git\necho username=fixture\necho password=synthetic-secret\n"
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("[credential]\n helper = %s\n useHttpPath = true\n", helper)
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := (Git{}).Token(context.Background(), "owner", "repo", "https://github.com/owner/repo.git", PurposeAPI)
	if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatalf("destination: %v", err)
	}
}

func TestGitIgnoresCheckoutHelper(t *testing.T) {
	home := isolatedGit(t)
	repo := filepath.Join(home, "checkout")
	cmd := exec.Command("git", "init", "-q", repo)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	marker := filepath.Join(home, "local-helper-called")
	local := fmt.Sprintf("\n[credential]\n helper = !touch %s\n", marker)
	f, err := os.OpenFile(filepath.Join(repo, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(local); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)
	_, err = (Git{}).Token(context.Background(), "owner", "repo", "https://github.com/owner/repo.git", PurposeAPI)
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("lookup: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("checkout helper invoked: %v", err)
	}
}
