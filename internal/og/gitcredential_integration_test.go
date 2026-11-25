//go:build !windows

package og

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

//nolint:gocyclo // The fixture checks setup, successful authentication, and both provider failure classes.
func TestGitCredentialUsedByGitHubPRAndErrorsAreSecretFree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(home, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	wrapper := fmt.Sprintf("#!/bin/sh\nexport GIT_CONFIG_NOSYSTEM=1\nexec %q \"$@\"\n", realGit)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	repo := testRegisteredHTTPRepo(t, home, "feature")
	helper := filepath.Join(home, "helper")
	script := "#!/bin/sh\necho username=fixture\necho password=synthetic-token\n"
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	config := "[credential]\n helper = " + helper + "\n"
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	status := http.StatusOK
	http.DefaultTransport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer synthetic-token" {
			t.Errorf("unexpected request destination or credential")
		}
		body := `{"number":7,"title":"fixture","state":"open","html_url":"https://github.com/tta-lab/example/pull/7"}`
		if status != http.StatusOK {
			body = `{"message":"synthetic-token"}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	service := NewService(nil)
	response, err := service.PRGet(Request{WorkDir: repo, Index: 7})
	if err != nil || response.PR == nil || response.PR.Index != 7 {
		t.Fatalf("PR get: %+v, %v", response.PR, err)
	}
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		status = code
		_, err := service.PRGet(Request{WorkDir: repo, Index: 7})
		if err == nil || strings.Contains(err.Error(), "synthetic-token") ||
			!strings.Contains(err.Error(), fmt.Sprint(code)) {
			t.Fatalf("status %d: %v", code, err)
		}
	}
}
