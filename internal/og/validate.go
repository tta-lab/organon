package og

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

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

// Response validators shared by the CLI, MCP, and Pi extension adapters so
// daemon result contracts are enforced in one place. Message texts are part of
// the adapter contract and are asserted by both the CLI and MCP tests.

// ValidateAuthResponse requires a secret-free auth status and, when an expected
// project is known, that the daemon returned it.
func ValidateAuthResponse(resp Response, projectAlias string) error {
	if resp.Auth == nil {
		return fmt.Errorf("og daemon returned no authentication status")
	}
	if projectAlias != "" && resp.Auth.Project != projectAlias {
		return fmt.Errorf(
			"og daemon returned authentication status for project %q, want %q",
			resp.Auth.Project, projectAlias)
	}
	return nil
}

// ValidatePRResponse requires a valid pull request and, when an expected ID is
// known, that the daemon returned it.
func ValidatePRResponse(resp Response, expectedID int64) error {
	if resp.PR == nil {
		return fmt.Errorf("og daemon returned no pull request")
	}
	if resp.PR.Index <= 0 {
		return fmt.Errorf("og daemon returned invalid PR ID %d", resp.PR.Index)
	}
	if expectedID > 0 && resp.PR.Index != expectedID {
		return fmt.Errorf("og daemon returned PR ID %d, want %d", resp.PR.Index, expectedID)
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
		return fmt.Errorf("og daemon returned pull request with unexpected title")
	}
	if body != nil && resp.PR.Body != *body {
		return fmt.Errorf("og daemon returned pull request with unexpected body")
	}
	return nil
}

// ValidateCommentResponse requires a comment matching the expected PR and body.
func ValidateCommentResponse(resp Response, expectedPRID int64, expectedBody string) error {
	if resp.Comment == nil {
		return fmt.Errorf("og daemon returned no comment")
	}
	comment := resp.Comment
	identityMismatch := comment.ID <= 0 || comment.PRID <= 0 ||
		(expectedPRID > 0 && comment.PRID != expectedPRID)
	contentMismatch := comment.Body != expectedBody || strings.TrimSpace(comment.URL) == ""
	if identityMismatch || contentMismatch {
		return fmt.Errorf("og daemon returned an invalid comment result")
	}
	return nil
}

// ValidateMessageResponse requires a non-blank operation result message.
func ValidateMessageResponse(resp Response) error {
	if strings.TrimSpace(resp.Message) == "" {
		return fmt.Errorf("og daemon returned no operation result")
	}
	return nil
}

// ValidateCloneResponse requires a complete, secret-free clone identity with a
// canonical HTTPS remote matching the reported host/owner/repo.
func ValidateCloneResponse(resp Response) error {
	if resp.Clone == nil {
		return fmt.Errorf("og daemon returned no clone result")
	}
	result := resp.Clone
	if !filepath.IsAbs(result.Path) || strings.TrimSpace(result.Host) == "" ||
		strings.TrimSpace(result.Owner) == "" || strings.TrimSpace(result.Repo) == "" ||
		strings.TrimSpace(result.Provider) == "" || strings.TrimSpace(result.Remote) == "" {
		return fmt.Errorf("og daemon returned an invalid clone result")
	}
	switch result.Provider {
	case "github", "forgejo", "generic":
	default:
		return fmt.Errorf("og daemon returned invalid clone provider %q", result.Provider)
	}
	if result.Registered {
		if err := project.ValidateAlias(result.Alias); err != nil {
			return fmt.Errorf("og daemon returned an invalid registered clone alias: %w", err)
		}
	} else if result.Alias != "" || result.Archived {
		return fmt.Errorf("og daemon returned invalid unregistered clone state")
	}
	return validateCloneRemote(result)
}

// validateCloneRemote verifies the canonical remote matches the reported
// host/owner/repo and the provider's transport policy.
func validateCloneRemote(result *CloneResult) error {
	u, err := url.Parse(result.Remote)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, result.Host) {
		return fmt.Errorf("og daemon returned an invalid clone remote")
	}
	if (result.Provider == "github" || result.Provider == "generic") && u.Scheme != "https" {
		return fmt.Errorf("og daemon returned an insecure clone remote")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != result.Owner || strings.TrimSuffix(parts[1], ".git") != result.Repo {
		return fmt.Errorf("og daemon returned mismatched clone identity")
	}
	return nil
}
