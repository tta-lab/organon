package og

import (
	"context"
	"fmt"
	"testing"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
)

type recordingIssueProvider struct {
	body   string
	writes int
}

func (p *recordingIssueProvider) ListIssues(string, string, string, string, int, int) (*gitprovider.IssuePage, error) {
	return &gitprovider.IssuePage{}, nil
}
func (p *recordingIssueProvider) GetIssue(_ string, _ string, id int64) (*gitprovider.Issue, error) {
	return &gitprovider.Issue{Index: id, Body: p.body, HTMLURL: "https://example/issue"}, nil
}
func (p *recordingIssueProvider) CreateIssue(string, string, string, string) (*gitprovider.Issue, error) {
	return nil, fmt.Errorf("unused")
}
func (p *recordingIssueProvider) UpdateIssueTitle(_ string, _ string, _ int64, _ string) (*gitprovider.Issue, error) {
	return nil, fmt.Errorf("unused")
}
func (p *recordingIssueProvider) ReplaceIssueBody(_, _ string, id int64, body string) (*gitprovider.Issue, error) {
	p.writes++
	p.body = body
	return &gitprovider.Issue{Index: id, Body: body, HTMLURL: "https://example/issue"}, nil
}
func (p *recordingIssueProvider) ListIssueComments(
	_ string, _ string, _ int64, _ int, _ int,
) (*gitprovider.CommentPage, error) {
	return &gitprovider.CommentPage{}, nil
}
func (p *recordingIssueProvider) CreateIssueComment(
	_ string, _ string, _ int64, _ string,
) (*gitprovider.Comment, error) {
	return nil, fmt.Errorf("unused")
}

func TestApplyIssueEditsPreservesMarkdownAndDeletes(t *testing.T) {
	body := "# Heading\n\nold one\n\nold two\n"
	edits := []BodyEdit{{OldText: "old one", NewText: "new **one**"}, {OldText: "old two\n", NewText: ""}}
	got, err := applyIssueEdits(body, edits)
	if err != nil {
		t.Fatal(err)
	}
	if want := "# Heading\n\nnew **one**\n\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestApplyIssueEditsRejectsInvalidBatches(t *testing.T) {
	for _, edits := range [][]BodyEdit{
		{{OldText: "missing", NewText: "x"}},
		{{OldText: "same", NewText: "x"}},
		{{OldText: "same", NewText: "x"}, {OldText: "same", NewText: "y"}},
		{{OldText: "abc", NewText: "x"}, {OldText: "bc", NewText: "y"}},
		{{OldText: "same", NewText: "same"}},
	} {
		if err := ValidateIssueEdits(edits); err == nil && len(edits) == 1 && edits[0].OldText == edits[0].NewText {
			continue
		}
		if _, err := applyIssueEdits("same abc same", edits); err == nil {
			t.Fatalf("edits %#v unexpectedly succeeded", edits)
		}
	}
}

func TestApplyIssueEditsRejectsOverlappingOccurrences(t *testing.T) {
	if _, err := applyIssueEdits("aaa", []BodyEdit{{OldText: "aa", NewText: "b"}}); err == nil {
		t.Fatal("overlapping occurrences must be ambiguous")
	}
}

func TestNormalizeIssuePageDefaultsAndLimits(t *testing.T) {
	state, page, per, err := normalizeIssuePage("", 0, 0, false)
	if err != nil || state != "open" || page != 1 || per != DefaultIssuePerPage {
		t.Fatalf("got %q %d %d %v", state, page, per, err)
	}
	if _, _, _, err := normalizeIssuePage("open", 1, MaxIssuePerPage+1, false); err == nil {
		t.Fatal("expected per-page error")
	}
}

func TestServiceIssueEditBodyWritesOnceOnlyAfterValidation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature")
	provider := &recordingIssueProvider{body: "alpha beta"}
	original := issueProviderFor
	issueProviderFor = func(_ *repoContext, purpose githubapp.Purpose) (gitprovider.IssueProvider, error) {
		if purpose != githubapp.PurposeIssueWrite {
			t.Fatalf("purpose=%s", purpose)
		}
		return provider, nil
	}
	t.Cleanup(func() { issueProviderFor = original })
	service := NewService(&recordingBroker{})
	req := Request{Context: context.Background(), WorkDir: repo, Index: 7,
		Edits: []BodyEdit{{OldText: "beta", NewText: "gamma"}}}
	resp, err := service.IssueEditBody(req)
	if err != nil || provider.writes != 1 || resp.Issue == nil || resp.Issue.Body != "alpha gamma" {
		t.Fatalf("resp=%+v writes=%d err=%v", resp, provider.writes, err)
	}
	_, err = service.IssueEditBody(Request{WorkDir: repo, Index: 7, Edits: []BodyEdit{{OldText: "missing", NewText: "x"}}})
	if err == nil || provider.writes != 1 {
		t.Fatalf("err=%v writes=%d", err, provider.writes)
	}
}
