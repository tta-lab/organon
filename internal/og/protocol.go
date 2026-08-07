package og

import (
	"context"
	"time"
)

// Request is the typed local daemon request.
type Request struct {
	Context   context.Context `json:"-"`
	WorkDir   string          `json:"work_dir,omitempty"`
	URL       string          `json:"url,omitempty"`
	Alias     string          `json:"alias,omitempty"`
	Reference bool            `json:"reference,omitempty"`
	Force     bool            `json:"force,omitempty"`
	Tag       string          `json:"tag,omitempty"`
	Bump      string          `json:"bump,omitempty"`
	Title     *string         `json:"title,omitempty"`
	Body      *string         `json:"body,omitempty"`
	Index     int64           `json:"index,omitempty"`
	State     string          `json:"state,omitempty"`
	Tail      int             `json:"tail,omitempty"`
}

// Response is the typed local daemon response.
type Response struct {
	OK      bool         `json:"ok"`
	Error   string       `json:"error,omitempty"`
	Message string       `json:"message,omitempty"`
	PR      *PullRequest `json:"pr,omitempty"`
	Comment *Comment     `json:"comment,omitempty"`
	Auth    *AuthStatus  `json:"auth,omitempty"`
	Lines   []string     `json:"lines,omitempty"`
	Clone   *CloneResult `json:"clone,omitempty"`
}

// CloneResult is the stable, secret-free identity of a cloned checkout.
type CloneResult struct {
	Alias          string `json:"alias,omitempty"`
	Path           string `json:"path"`
	Host           string `json:"host"`
	Owner          string `json:"owner"`
	Repo           string `json:"repo"`
	Provider       string `json:"provider"`
	Remote         string `json:"remote"`
	Registered     bool   `json:"registered"`
	Archived       bool   `json:"archived"`
	AlreadyExisted bool   `json:"already_existed"`
}

// Comment is the stable comment shape returned by the daemon.
type Comment struct {
	ID        int64     `json:"id"`
	PRID      int64     `json:"pr_id"`
	Body      string    `json:"body"`
	URL       string    `json:"url"`
	User      string    `json:"user,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// AuthStatus is the stable, secret-free authentication status for one repository.
type AuthStatus struct {
	Project     string             `json:"project"`
	Provider    string             `json:"provider"`
	Host        string             `json:"host"`
	Owner       string             `json:"owner"`
	Repo        string             `json:"repo"`
	AuthMode    string             `json:"auth_mode"`
	Ready       bool               `json:"ready"`
	TokenEnv    string             `json:"token_env,omitempty"`
	TokenSet    bool               `json:"token_set,omitempty"`
	Permissions []PermissionStatus `json:"permissions,omitempty"`
}

// PermissionStatus reports one required provider permission without secrets.
type PermissionStatus struct {
	Name     string `json:"name"`
	Required string `json:"required"`
	Actual   string `json:"actual,omitempty"`
	Ready    bool   `json:"ready"`
}

// PullRequest is the stable PR shape returned to the CLI.
type PullRequest struct {
	Index        int64             `json:"index"`
	Number       int64             `json:"number,omitempty"`
	Title        string            `json:"title"`
	State        string            `json:"state"`
	Merged       bool              `json:"merged"`
	URL          string            `json:"url"`
	HTMLURL      string            `json:"html_url,omitempty"`
	Head         string            `json:"head"`
	Base         string            `json:"base"`
	Body         string            `json:"body"`
	SHA          string            `json:"head_sha,omitempty"`
	CI           *CIStatusResponse `json:"ci,omitempty"`
	CIFetchError string            `json:"ci_fetch_error,omitempty"`
}

// CIStatusResponse is the stable CI summary shape returned with PR JSON.
type CIStatusResponse struct {
	OK       bool       `json:"ok"`
	Error    string     `json:"error,omitempty"`
	State    string     `json:"state,omitempty"`
	Statuses []CIStatus `json:"statuses,omitempty"`
}

// CIStatus is a single CI check status.
type CIStatus struct {
	Context     string `json:"context"`
	State       string `json:"state"`
	Description string `json:"description"`
	TargetURL   string `json:"target_url"`
}

func success(resp Response) Response {
	resp.OK = true
	return resp
}

func DisplayPRURL(pr *PullRequest) string {
	if pr.HTMLURL != "" {
		return pr.HTMLURL
	}
	return pr.URL
}
