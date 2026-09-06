package og

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/tta-lab/organon/internal/project"
)

const (
	// PRStateOpen is the default state for current-branch PR searches.
	PRStateOpen = "open"
	// PRStateClosed selects closed pull requests.
	PRStateClosed = "closed"
	// PRStateAll selects pull requests in every state.
	PRStateAll = "all"

	// DefaultPRLogTail is the default number of CI log lines to request.
	DefaultPRLogTail = 50
	// MaxPRLogTail is the largest accepted CI log tail window.
	MaxPRLogTail = 1000
)

// ValidatePositivePRID requires an explicit pull request ID to be positive.
func ValidatePositivePRID(id int64) error {
	if id <= 0 {
		return fmt.Errorf("PR ID must be positive")
	}
	return nil
}

// ValidatePRTitle requires title content without changing caller-supplied text.
func ValidatePRTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("PR title must not be blank")
	}
	return nil
}

// NormalizePRState applies the default state and rejects unsupported values.
func NormalizePRState(state string) (string, error) {
	if state == "" {
		return PRStateOpen, nil
	}
	switch state {
	case PRStateOpen, PRStateClosed, PRStateAll:
		return state, nil
	default:
		return "", fmt.Errorf("PR state must be open, closed, or all")
	}
}

// ValidatePRModifyInput requires at least one replacement and a non-blank
// replacement title. Bodies deliberately retain their original bytes, including
// leading and trailing whitespace, because an empty body explicitly clears it.
func ValidatePRModifyInput(title, body *string) error {
	if title == nil && body == nil {
		return fmt.Errorf("nothing to update: provide title and/or body")
	}
	if title != nil {
		return ValidatePRTitle(*title)
	}
	return nil
}

// ValidatePRCommentBody requires non-blank content without trimming the body
// that callers forward to the provider.
func ValidatePRCommentBody(body *string) error {
	if body == nil || strings.TrimSpace(*body) == "" {
		return fmt.Errorf("comment body must not be blank")
	}
	return nil
}

// ValidatePRLogTail requires the shared bounded log window.
func ValidatePRLogTail(tail int) error {
	if tail < 0 || tail > MaxPRLogTail {
		return fmt.Errorf("tail must be between 0 and %d", MaxPRLogTail)
	}
	return nil
}

// NormalizePRMergeRequest validates transport-independent merge arguments and
// applies their shared defaults. The normalized request is what every adapter
// and the service must execute.
func NormalizePRMergeRequest(req Request) (Request, error) {
	if req.Index < 0 {
		return Request{}, fmt.Errorf("PR ID must not be negative")
	}
	if req.Timeout < 0 {
		return Request{}, fmt.Errorf("merge timeout must not be negative")
	}
	if req.Wait && req.Timeout > 24*time.Hour {
		return Request{}, fmt.Errorf("merge timeout must not exceed 24h")
	}
	if req.Wait && req.Timeout == 0 {
		req.Timeout = DefaultPRMergeTimeout
	}
	return req, nil
}

// ValidatePRMergeRequest checks transport-independent merge arguments.
func ValidatePRMergeRequest(req Request) error {
	_, err := NormalizePRMergeRequest(req)
	return err
}

// Response validators shared by the CLI, MCP, and Pi extension adapters so
// result contracts are enforced in one place. Message texts are part of the
// adapter contract and are asserted by both the CLI and MCP tests.

// ValidateAuthResponse requires a secret-free auth status and, when an expected
// project is known, that the operation returned it.
func ValidateAuthResponse(resp Response, projectAlias string) error {
	if resp.Auth == nil {
		return fmt.Errorf("og returned no authentication status")
	}
	if projectAlias != "" && resp.Auth.Project != projectAlias {
		return fmt.Errorf(
			"og returned authentication status for project %q, want %q",
			resp.Auth.Project, projectAlias)
	}
	return nil
}

// ValidatePRResponse requires a valid pull request and, when an expected ID is
// known, that the operation returned it.
func ValidatePRResponse(resp Response, expectedID int64) error {
	if resp.PR == nil {
		return fmt.Errorf("og returned no pull request")
	}
	if resp.PR.Index <= 0 {
		return fmt.Errorf("og returned invalid PR ID %d", resp.PR.Index)
	}
	if expectedID > 0 && resp.PR.Index != expectedID {
		return fmt.Errorf("og returned PR ID %d, want %d", resp.PR.Index, expectedID)
	}
	return nil
}

// ValidatePRModifyResponse requires a valid pull request that reflects the
// requested title/body changes.
func ValidatePRModifyResponse(resp Response, expectedID int64, title, body *string) error {
	if err := ValidatePRResponse(resp, expectedID); err != nil {
		return err
	}
	if title != nil && resp.PR.Title != *title {
		return fmt.Errorf("og returned pull request with unexpected title")
	}
	if body != nil && resp.PR.Body != *body {
		return fmt.Errorf("og returned pull request with unexpected body")
	}
	return nil
}

// ValidateCommentResponse requires a comment matching the expected PR and body.
func ValidateCommentResponse(resp Response, expectedPRID int64, expectedBody string) error {
	if resp.Comment == nil {
		return fmt.Errorf("og returned no comment")
	}
	comment := resp.Comment
	identityMismatch := comment.ID <= 0 || comment.PRID <= 0 ||
		(expectedPRID > 0 && comment.PRID != expectedPRID)
	contentMismatch := comment.Body != expectedBody || strings.TrimSpace(comment.URL) == ""
	if identityMismatch || contentMismatch {
		return fmt.Errorf("og returned an invalid comment result")
	}
	return nil
}

// ValidateMessageResponse requires a non-blank operation result message.
func ValidateMessageResponse(resp Response) error {
	if strings.TrimSpace(resp.Message) == "" {
		return fmt.Errorf("og returned no operation result")
	}
	return nil
}

// ValidatePRMergeResponse requires the stable approval-gated merge shape.
func ValidatePRMergeResponse(resp Response, expectedID int64) error {
	if resp.Merge == nil {
		return fmt.Errorf("og returned no pull request merge result")
	}
	merge := resp.Merge
	if strings.TrimSpace(merge.ActionID) == "" {
		return fmt.Errorf("og returned an invalid pull request merge result")
	}
	switch merge.Status {
	case PRMergeStatusPending, PRMergeStatusApproved, PRMergeStatusRejected,
		PRMergeStatusExpired, PRMergeStatusExecuted, PRMergeStatusExecuteFailed,
		PRMergeStatusUnavailable:
	default:
		return fmt.Errorf("og returned invalid pull request merge status %q", merge.Status)
	}
	if merge.Status != PRMergeStatusUnavailable || merge.Snapshot.PRNumber != 0 {
		if merge.Snapshot.PRNumber <= 0 || strings.TrimSpace(merge.Snapshot.PRURL) == "" {
			return fmt.Errorf("og returned an invalid pull request merge result")
		}
		if expectedID > 0 && merge.Snapshot.PRNumber != expectedID {
			return fmt.Errorf("og returned PR ID %d, want %d", merge.Snapshot.PRNumber, expectedID)
		}
	}
	if merge.InboxURL == "" {
		return fmt.Errorf("og returned pull request merge result without inbox URL")
	}
	if strings.TrimSpace(merge.NextAction) == "" || strings.TrimSpace(merge.Completion) == "" {
		return fmt.Errorf("og returned pull request merge result without recovery instructions")
	}
	return validatePRMergeOutcome(*merge)
}

func validatePRMergeOutcome(merge PRMergeResult) error { //nolint:gocyclo
	switch merge.Status {
	case PRMergeStatusPending:
		if merge.Retryable || merge.NextAction != PRMergeNextWait {
			return fmt.Errorf("og returned an invalid pending merge recovery state")
		}
	case PRMergeStatusApproved:
		if !merge.Retryable || merge.NextAction != PRMergeNextRetry {
			return fmt.Errorf("og returned an invalid retryable merge recovery state")
		}
	case PRMergeStatusUnavailable:
		if !merge.Retryable || merge.NextAction != PRMergeNextRetry {
			return fmt.Errorf("og returned an invalid unavailable merge recovery state")
		}
	case PRMergeStatusRejected, PRMergeStatusExpired, PRMergeStatusExecuteFailed:
		if merge.Retryable || merge.NextAction != PRMergeNextNewApproval {
			return fmt.Errorf("og returned an invalid terminal merge recovery state")
		}
	case PRMergeStatusExecuted:
		return validateExecutedPRMergeOutcome(merge)
	}
	return nil
}

func validateExecutedPRMergeOutcome(merge PRMergeResult) error {
	if merge.CleanupError != "" {
		if merge.ReceiptError != "" {
			return fmt.Errorf("og returned both receipt and cleanup errors for one executed merge")
		}
		if !merge.Retryable || merge.NextAction != PRMergeNextRetry {
			return fmt.Errorf("og returned an invalid cleanup-retry merge state")
		}
		return nil
	}
	if merge.ReceiptError != "" {
		if !merge.Retryable || merge.NextAction != PRMergeNextRepairReceipt {
			return fmt.Errorf("og returned an invalid receipt-repair merge state")
		}
		return nil
	}
	if merge.Retryable || merge.NextAction != PRMergeNextNone {
		return fmt.Errorf("og returned an invalid completed merge state")
	}
	return nil
}

// ValidateCloneResponse requires a complete, secret-free clone identity with a
// canonical HTTPS remote matching the reported host/owner/repo.
func ValidateCloneResponse(resp Response) error {
	if resp.Clone == nil {
		return fmt.Errorf("og returned no clone result")
	}
	result := resp.Clone
	if !filepath.IsAbs(result.Path) || strings.TrimSpace(result.Host) == "" ||
		strings.TrimSpace(result.Owner) == "" || strings.TrimSpace(result.Repo) == "" ||
		strings.TrimSpace(result.Provider) == "" || strings.TrimSpace(result.Remote) == "" {
		return fmt.Errorf("og returned an invalid clone result")
	}
	switch result.Provider {
	case "github", "forgejo", "generic":
	default:
		return fmt.Errorf("og returned invalid clone provider %q", result.Provider)
	}
	if result.Registered {
		if err := project.ValidateAlias(result.Alias); err != nil {
			return fmt.Errorf("og returned an invalid registered clone alias: %w", err)
		}
	} else if result.Alias != "" || result.Archived {
		return fmt.Errorf("og returned invalid unregistered clone state")
	}
	return validateCloneRemote(result)
}

// validateCloneRemote verifies the canonical remote matches the reported
// host/owner/repo and the provider's transport policy.
func validateCloneRemote(result *CloneResult) error {
	u, err := url.Parse(result.Remote)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, result.Host) {
		return fmt.Errorf("og returned an invalid clone remote")
	}
	if (result.Provider == "github" || result.Provider == "generic") && u.Scheme != "https" {
		return fmt.Errorf("og returned an insecure clone remote")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != result.Owner || strings.TrimSuffix(parts[1], ".git") != result.Repo {
		return fmt.Errorf("og returned mismatched clone identity")
	}
	return nil
}
