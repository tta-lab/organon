package og

import (
	"context"
	"time"
)

// Request is the typed input for one direct OG operation.
type Request struct {
	Context   context.Context `json:"-"`
	WorkDir   string          `json:"work_dir,omitempty"`
	Project   string          `json:"project,omitempty"`
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
	DryRun    bool            `json:"dry_run,omitempty"`
	Wait      bool            `json:"wait,omitempty"`
	Timeout   time.Duration   `json:"timeout,omitempty"`
}

// Response is the typed result of one direct OG operation.
type Response struct {
	OK      bool           `json:"ok"`
	Error   string         `json:"error,omitempty"`
	Message string         `json:"message,omitempty"`
	PR      *PullRequest   `json:"pr,omitempty"`
	Comment *Comment       `json:"comment,omitempty"`
	Auth    *AuthStatus    `json:"auth,omitempty"`
	Lines   []string       `json:"lines,omitempty"`
	Clone   *CloneResult   `json:"clone,omitempty"`
	Merge   *PRMergeResult `json:"merge,omitempty"`
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

// Comment is the stable comment shape returned by OG.
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
	Mergeable    bool              `json:"mergeable"`
}

const (
	// PRMergeKind is the Impri action kind used for approval-gated merges.
	PRMergeKind = "git.merge_pr"
	// PRMergeMethodSquash is the only merge method exposed by OG.
	PRMergeMethodSquash = "squash"
	// PRMergeModeDryRun records a non-destructive mock execution.
	PRMergeModeDryRun = "dry-run"
	// PRMergeModeReal records a forge squash merge.
	PRMergeModeReal = "real"

	PRMergeStatusPending       = "pending"
	PRMergeStatusApproved      = "approved"
	PRMergeStatusRejected      = "rejected"
	PRMergeStatusExpired       = "expired"
	PRMergeStatusExecuted      = "executed"
	PRMergeStatusExecuteFailed = "execute_failed"
	// PRMergeStatusUnavailable is a local outcome used when Impri's current
	// approval state cannot be read. It is never sent to Impri as a result.
	PRMergeStatusUnavailable = "unavailable"

	// PRMergeNextAction values are machine-readable routing instructions for
	// agents consuming PRMergeResult.
	PRMergeNextWait          = "wait_for_approval"
	PRMergeNextRetry         = "retry_same_request"
	PRMergeNextRepairReceipt = "repair_receipt"
	PRMergeNextNone          = "none"
	PRMergeNextNewApproval   = "new_approval_required"

	// DefaultPRMergeTimeout is the shared wait timeout applied when a caller
	// requests waiting but leaves its timeout unset or explicitly zero.
	DefaultPRMergeTimeout = 30 * time.Second
)

// PRMergeSnapshot is the immutable forge identity submitted for approval.
// Fields such as state and CIState are descriptive; the provider, forge,
// repository, PR number, PR URL, head branch, head SHA, base branch, method,
// and mode form the authorization identity.
type PRMergeSnapshot struct {
	Provider      string `json:"provider"`
	ForgeBaseURL  string `json:"forge_base_url"`
	Owner         string `json:"owner"`
	Repo          string `json:"repo"`
	PRNumber      int64  `json:"pr_number"`
	HeadSHA       string `json:"head_sha"`
	BaseBranch    string `json:"base_branch"`
	MergeMethod   string `json:"merge_method"`
	ExecutionMode string `json:"execution_mode"`
	PRURL         string `json:"pr_url,omitempty"`
	Title         string `json:"title,omitempty"`
	Head          string `json:"head,omitempty"`
	State         string `json:"state,omitempty"`
	CIState       string `json:"ci_state,omitempty"`
	Mergeable     bool   `json:"mergeable"`
}

// PRMergeResult is the approval and execution state returned by CLI and MCP.
// An executed result can remain retryable when receipt or automatic checkout
// cleanup has not finished; cleanup errors are distinct from receipt errors.
type PRMergeResult struct {
	ActionID     string          `json:"action_id"`
	Status       string          `json:"status"`
	InboxURL     string          `json:"inbox_url"`
	Snapshot     PRMergeSnapshot `json:"snapshot"`
	Retryable    bool            `json:"retryable"`
	NextAction   string          `json:"next_action"`
	Completion   string          `json:"completion"`
	Detail       string          `json:"detail,omitempty"`
	ReceiptError string          `json:"receipt_error,omitempty"`
	CleanupError string          `json:"cleanup_error,omitempty"`
}

// PRMergeRetryableError carries the structured merge outcome alongside its
// non-success signal. MCP adapters can return the result with IsError=true,
// while CLI callers can print the same result before returning the error.
type PRMergeRetryableError struct {
	Result PRMergeResult
}

func (e *PRMergeRetryableError) Error() string { return e.Result.Detail }

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
