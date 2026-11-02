package gitprovider

import (
	"context"
	"net/http"
	"time"
)

type ProviderType string

const (
	ProviderForgejo ProviderType = "forgejo"
	ProviderGeneric ProviderType = "generic"
	ProviderGitHub  ProviderType = "github"
)

// CI status state constants.
const (
	StatePending = "pending"
	StateSuccess = "success"
	StateFailure = "failure"
	StateError   = "error"
	// StateNotConfigured means the provider successfully found no checks for
	// the commit. It is distinct from an API error or an unknown nonempty state.
	StateNotConfigured = "not_configured"
)

type PullRequest struct {
	Index     int64
	Title     string
	Body      string
	State     string
	HTMLURL   string
	Head      string
	HeadSHA   string
	Base      string
	Mergeable bool
	Merged    bool
}

type Comment struct {
	ID        int64
	PRID      int64
	Body      string
	User      string
	CreatedAt time.Time
	HTMLURL   string
}

// Issue is a repository issue. Pull requests are deliberately not issues here.
type Issue struct {
	Index         int64
	Title         string
	Body          string
	State         string
	HTMLURL       string
	IsPullRequest bool
}

// IssuePage is one bounded provider page.
type IssuePage struct {
	Issues     []*Issue
	HasNext    bool
	Incomplete bool
}
type CommentPage struct {
	Comments []*Comment
	HasNext  bool
}

// IssueProvider is the issue-specific provider capability.
type IssueProvider interface {
	ListIssues(owner, repo, query, state string, page, perPage int) (*IssuePage, error)
	GetIssue(owner, repo string, index int64) (*Issue, error)
	CreateIssue(owner, repo, title, body string) (*Issue, error)
	UpdateIssueTitle(owner, repo string, index int64, title string) (*Issue, error)
	ReplaceIssueBody(owner, repo string, index int64, body string) (*Issue, error)
	ListIssueComments(owner, repo string, index int64, page, perPage int) (*CommentPage, error)
	CreateIssueComment(owner, repo string, index int64, body string) (*Comment, error)
}

// CommitStatus represents the status of a single CI check on a commit.
type CommitStatus struct {
	Context     string // Check name (e.g. "ci/woodpecker", "lint")
	State       string // "pending", "success", "error", "failure"
	Description string
	TargetURL   string
}

// CombinedStatus represents the overall status of all checks on a commit.
type CombinedStatus struct {
	State    string // Overall: "pending", "success", "error", "failure", or "not_configured"
	Statuses []*CommitStatus
}

func notConfiguredCombinedStatus() *CombinedStatus {
	return &CombinedStatus{State: StateNotConfigured, Statuses: []*CommitStatus{}}
}

type Provider interface {
	Name() string
	CreatePR(owner, repo, head, base, title, body string) (*PullRequest, error)
	FindPR(owner, repo, head, base string) (*PullRequest, error)
	FindPRByState(owner, repo, head, base, state string) (*PullRequest, error)
	EditPR(owner, repo string, index int64, title, body string) (*PullRequest, error)
	GetPR(owner, repo string, index int64) (*PullRequest, error)
	CreateComment(owner, repo string, index int64, body string) (*Comment, error)
	ListComments(owner, repo string, index int64) ([]*Comment, error)
	GetCombinedStatus(owner, repo, ref string) (*CombinedStatus, error)
}

// PullRequestMerger is the optional write capability implemented by supported
// forge providers. Keeping it separate preserves the read-only provider seam.
type PullRequestMerger interface {
	MergePullRequest(owner, repo string, index int64, headSHA string) error
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

type contextTransport struct {
	context context.Context
	base    http.RoundTripper
}

func (t contextTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := contextOrBackground(t.context)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req.Clone(ctx))
}

func newContextHTTPClient(ctx context.Context, transport http.RoundTripper) *http.Client {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &http.Client{Transport: contextTransport{context: contextOrBackground(ctx), base: transport}}
}
