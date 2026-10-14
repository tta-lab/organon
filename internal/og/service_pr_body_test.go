package og

import (
	"fmt"
	"testing"

	"github.com/tta-lab/organon/internal/gitprovider"
)

func TestPREditBodySnapshotAndTargeting(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredHTTPRepo(t, home, "feature/x")
			index := int64(0)
			if explicit {
				index = 7
				gitRun(t, repo, "checkout", "--detach")
				gitRun(t, repo, "remote", "set-url", "--push", remoteOrigin, "https://attacker.invalid/repo.git")
			}
			reads, writes, finds := 0, 0, 0
			const title = "  Keep title exactly  "
			const original = "# Heading\r\n\nalpha\nbeta\nremove\n終わり"
			const want = "# Heading\r\n\nbeta\nnew\n終わり"
			restore := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
				return fakeProvider{
					findPRByState: func(_, _, head, _, state string) (*gitprovider.PullRequest, error) {
						finds++
						if explicit || head != "feature/x" || state != stateAll {
							t.Fatalf("unexpected branch lookup %q %q", head, state)
						}
						// Branch lookup may return only PR metadata: fetch the body by ID.
						return &gitprovider.PullRequest{Index: 7}, nil
					},
					getPR: func(_, _ string, id int64) (*gitprovider.PullRequest, error) {
						reads++
						if id != 7 {
							t.Fatalf("read ID %d", id)
						}
						return &gitprovider.PullRequest{Index: id, Title: title, Body: original}, nil
					},
					editPR: func(_, _ string, id int64, gotTitle, body string) (*gitprovider.PullRequest, error) {
						writes++
						if id != 7 || gotTitle != title || body != want {
							t.Fatalf("update #%d %q %q", id, gotTitle, body)
						}
						return &gitprovider.PullRequest{Index: id, Title: gotTitle, Body: body}, nil
					},
				}, nil
			})
			defer restore()
			resp, err := NewService(&recordingBroker{}).PREditBody(Request{WorkDir: repo, Index: index, Edits: []BodyEdit{
				{OldText: "remove\n", NewText: ""},
				{OldText: "alpha\n", NewText: "beta\n"},
				{OldText: "beta\n", NewText: "new\n"},
			}})
			assertPREditResult(t, resp, err, want, reads, writes)
			if !explicit && finds != 1 {
				t.Fatalf("finds=%d", finds)
			}
		})
	}
}

func TestPREditBodyRejectsBeforeWrite(t *testing.T) {
	tests := []struct {
		name, body string
		edits      []BodyEdit
		index      int64
	}{
		{name: "negative ID", index: -1, edits: []BodyEdit{{OldText: "a", NewText: "b"}}},
		{name: "empty batch"},
		{name: "empty old", edits: []BodyEdit{{NewText: "x"}}},
		{name: "no-op", edits: []BodyEdit{{OldText: "a", NewText: "a"}}},
		{name: "duplicate", body: "abc", edits: []BodyEdit{{OldText: "a", NewText: "x"}, {OldText: "a", NewText: "y"}}},
		{name: "missing after valid", body: "abc",
			edits: []BodyEdit{{OldText: "a", NewText: "x"}, {OldText: "x", NewText: "y"}}},
		{name: "ambiguous", body: "a a", edits: []BodyEdit{{OldText: "a", NewText: "x"}}},
		{name: "overlapping occurrences", body: "aaa", edits: []BodyEdit{{OldText: "aa", NewText: "x"}}},
		{name: "nested", body: "abc", edits: []BodyEdit{{OldText: "abc", NewText: "x"}, {OldText: "b", NewText: "y"}}},
		{name: "overlap", body: "abcd", edits: []BodyEdit{{OldText: "abc", NewText: "x"}, {OldText: "bcd", NewText: "y"}}},
		{name: "literal line endings", body: "a\r\nb", edits: []BodyEdit{{OldText: "a\nb", NewText: "x"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredHTTPRepo(t, home, "feature")
			writes := 0
			restore := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
				return fakeProvider{
					getPR: func(_, _ string, id int64) (*gitprovider.PullRequest, error) {
						return &gitprovider.PullRequest{Index: id, Body: tt.body}, nil
					},
					editPR: func(_, _ string, _ int64, _, _ string) (*gitprovider.PullRequest, error) {
						writes++
						return nil, fmt.Errorf("unexpected write")
					},
				}, nil
			})
			defer restore()
			index := tt.index
			if index == 0 {
				index = 7
			}
			_, err := NewService(&recordingBroker{}).PREditBody(Request{WorkDir: repo, Index: index, Edits: tt.edits})
			if err == nil || writes != 0 {
				t.Fatalf("err=%v writes=%d", err, writes)
			}
		})
	}
}

func TestPREditBodyRejectsProviderFailures(t *testing.T) {
	for _, failure := range []string{
		"nil snapshot", "wrong snapshot ID", "read error", "nil result",
		"wrong result ID", "stale body", "changed title", "write error",
	} {
		t.Run(failure, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredHTTPRepo(t, home, "feature")
			writes := 0
			restore := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
				return fakeProvider{
					getPR: func(_, _ string, id int64) (*gitprovider.PullRequest, error) {
						switch failure {
						case "nil snapshot":
							return nil, nil
						case "wrong snapshot ID":
							id = 99
						case "read error":
							return nil, fmt.Errorf("read failed")
						}
						return &gitprovider.PullRequest{Index: id, Title: "title", Body: "old"}, nil
					},
					editPR: func(_, _ string, id int64, title, body string) (*gitprovider.PullRequest, error) {
						writes++
						switch failure {
						case "nil result":
							return nil, nil
						case "wrong result ID":
							id = 99
						case "stale body":
							body = "old"
						case "changed title":
							title = "changed"
						case "write error":
							return nil, fmt.Errorf("uncertain write")
						}
						return &gitprovider.PullRequest{Index: id, Title: title, Body: body}, nil
					},
				}, nil
			})
			defer restore()
			_, err := NewService(&recordingBroker{}).PREditBody(Request{
				WorkDir: repo, Index: 7, Edits: []BodyEdit{{OldText: "old", NewText: "new"}},
			})
			wantWrites := 1
			if failure == "nil snapshot" || failure == "wrong snapshot ID" || failure == "read error" {
				wantWrites = 0
			}
			if err == nil || writes != wantWrites {
				t.Fatalf("err=%v writes=%d want=%d", err, writes, wantWrites)
			}
		})
	}
}

func TestPREditBodyWritePolicyBeforeProvider(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(fmt.Sprint(archived), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			remote := "https://example.org/owner/repo.git"
			if archived {
				remote = "https://github.com/owner/repo.git"
			}
			repo := testRegisteredRepo(t, home, "feature", remote, archived)
			calls := 0
			restore := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
				calls++
				return nil, fmt.Errorf("unexpected provider")
			})
			defer restore()
			_, err := NewService(&recordingBroker{}).PREditBody(Request{
				WorkDir: repo, Index: 7, Edits: []BodyEdit{{OldText: "old", NewText: "new"}},
			})
			if err == nil || calls != 0 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}

func assertPREditResult(t *testing.T, resp Response, err error, want string, reads, writes int) {
	t.Helper()
	if err != nil || !resp.OK || resp.PR == nil || resp.PR.Body != want || reads != 1 || writes != 1 {
		t.Fatalf("response=%+v err=%v reads=%d writes=%d", resp, err, reads, writes)
	}
}
