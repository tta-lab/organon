package og

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tta-lab/organon/internal/ogconfig"
)

const (
	impriAPIVersionPath = "/v1"
	impriActionsPath    = "/actions"
	impriPollInterval   = 250 * time.Millisecond
	impriDefaultWait    = DefaultPRMergeTimeout
	impriMaxBodyBytes   = 1 << 20
)

type impriClient struct {
	baseURL string
	rootURL string
	apiKey  string
	http    *http.Client
}

type impriCreateAction struct {
	Kind           string         `json:"kind"`
	Title          string         `json:"title"`
	Preview        impriPreview   `json:"preview"`
	Payload        map[string]any `json:"payload"`
	TargetURL      string         `json:"target_url"`
	ExpiresIn      int            `json:"expires_in"`
	IdempotencyKey string         `json:"idempotency_key"`
}

type impriCreateReceipt struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	InboxURL string `json:"inbox_url"`
}

type impriPreview struct {
	Format string `json:"format"`
	Body   string `json:"body"`
}

type impriResult struct {
	Status  string         `json:"status"`
	Detail  string         `json:"detail,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

type impriAction struct {
	ID        string
	Kind      string
	Status    string
	InboxURL  string
	TargetURL string
	Payload   map[string]any
}

func newImpriClient(ctx context.Context, cfg *ogconfig.ImpriConfig) (*impriClient, error) {
	return newImpriClientWithHTTPClient(ctx, cfg, nil)
}

func newImpriClientWithHTTPClient(
	ctx context.Context, cfg *ogconfig.ImpriConfig, httpClient *http.Client,
) (*impriClient, error) {
	if cfg == nil {
		return nil, fmt.Errorf("impri configuration is not configured")
	}
	copyCfg := *cfg
	if err := copyCfg.Validate(); err != nil {
		return nil, err
	}
	root := strings.TrimRight(copyCfg.BaseURL, "/")
	client := &http.Client{}
	if httpClient != nil {
		client = httpClient
	}
	_ = ctx
	return &impriClient{
		baseURL: root + impriAPIVersionPath,
		rootURL: root,
		apiKey:  copyCfg.APIKey,
		http:    client,
	}, nil
}

func (c *impriClient) createAction(ctx context.Context, body impriCreateAction) (impriAction, error) {
	var receipt impriCreateReceipt
	if err := c.requestJSON(ctx, http.MethodPost, impriActionsPath, body, &receipt); err != nil {
		return impriAction{}, fmt.Errorf("create Impri approval action: %w", err)
	}
	if err := validateImpriCreateReceipt(receipt); err != nil {
		return impriAction{}, err
	}
	action, err := c.getAction(ctx, receipt.ID)
	if err != nil {
		return impriAction{ID: receipt.ID, Status: receipt.Status, InboxURL: receipt.InboxURL},
			fmt.Errorf("read created Impri approval action: %w", err)
	}
	if action.ID != receipt.ID {
		return impriAction{}, fmt.Errorf("impri create receipt action ID does not match the canonical action")
	}
	return action, nil
}

func validateImpriCreateReceipt(receipt impriCreateReceipt) error {
	if strings.TrimSpace(receipt.ID) == "" {
		return fmt.Errorf("impri create response is missing a valid action ID")
	}
	if !validImpriActionStatus(receipt.Status) {
		return fmt.Errorf("impri create response has invalid action status %q", receipt.Status)
	}
	return nil
}

func validImpriActionStatus(status string) bool {
	switch status {
	case PRMergeStatusPending, PRMergeStatusApproved, PRMergeStatusRejected,
		PRMergeStatusExpired, PRMergeStatusExecuted, PRMergeStatusExecuteFailed:
		return true
	default:
		return false
	}
}

func (c *impriClient) getAction(ctx context.Context, id string) (impriAction, error) {
	if strings.TrimSpace(id) == "" {
		return impriAction{}, fmt.Errorf("impri action ID is required")
	}
	var action impriAction
	path := impriActionsPath + "/" + url.PathEscape(id)
	if err := c.requestJSON(ctx, http.MethodGet, path, nil, &action); err != nil {
		return impriAction{}, fmt.Errorf("read Impri approval action: %w", err)
	}
	return action, nil
}

func (c *impriClient) reportResult(
	ctx context.Context, id, status, detail string, payload map[string]any,
) error {
	if id == "" {
		return fmt.Errorf("cannot report Impri result without an action ID")
	}
	body := impriResult{Status: status, Detail: detail, Payload: payload}
	path := impriActionsPath + "/" + url.PathEscape(id) + "/result"
	if err := c.requestJSON(ctx, http.MethodPost, path, body, nil); err != nil {
		return fmt.Errorf("report Impri execution result: %w", err)
	}
	return nil
}

func (c *impriClient) inboxURL() string {
	return c.rootURL + impriActionsPath
}

// retryableImpriReadError distinguishes transient transport/service outages
// from response-contract violations and deterministic HTTP failures. A
// malformed action or 4xx response must still fail closed so a wrong kind,
// target, credential, or missing action can never become an endless retry.
func retryableImpriReadError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var httpErr *impriHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.status == http.StatusTooManyRequests || httpErr.status >= http.StatusInternalServerError
	}
	var transportErr *impriTransportError
	if errors.As(err, &transportErr) {
		return true
	}
	var readErr *impriReadError
	return errors.As(err, &readErr)
}

func (c *impriClient) requestJSON(
	ctx context.Context, method, path string, input, output any,
) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req) //nolint:gosec // base URL is validated operator configuration.
	if err != nil {
		redacted := redactImpriError(err, c.apiKey)
		return &impriTransportError{
			cause:   redacted,
			message: fmt.Sprintf("request failed: %v", redacted),
		}
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, impriMaxBodyBytes))
	if readErr != nil {
		redacted := redactImpriError(readErr, c.apiKey)
		return &impriReadError{
			cause:   redacted,
			message: fmt.Sprintf("read response: %v", redacted),
		}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &impriHTTPError{
			status:  resp.StatusCode,
			message: fmt.Sprintf("impri returned HTTP %d: %s", resp.StatusCode, safeImpriBody(data, c.apiKey)),
		}
	}
	if output == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return &impriDecodeError{
			cause:   err,
			message: fmt.Sprintf("decode response: %v", err),
		}
	}
	return nil
}

type impriTransportError struct {
	cause   error
	message string
}

func (e *impriTransportError) Error() string { return e.message }

func (e *impriTransportError) Unwrap() error { return e.cause }

type impriReadError struct {
	cause   error
	message string
}

func (e *impriReadError) Error() string { return e.message }

func (e *impriReadError) Unwrap() error { return e.cause }

type impriHTTPError struct {
	status  int
	message string
}

func (e *impriHTTPError) Error() string { return e.message }

func (e *impriHTTPError) StatusCode() int { return e.status }

type impriDecodeError struct {
	cause   error
	message string
}

func (e *impriDecodeError) Error() string { return e.message }

func (e *impriDecodeError) Unwrap() error { return e.cause }

func safeImpriBody(data []byte, secret string) string {
	message := redactImpriText(strings.TrimSpace(string(data)), secret)
	if len(message) > 512 {
		message = message[:512] + "…"
	}
	return message
}

type impriRedactedError struct {
	cause   error
	message string
}

func (e *impriRedactedError) Error() string { return e.message }

func (e *impriRedactedError) Unwrap() error { return e.cause }

func redactImpriError(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	message := redactImpriText(err.Error(), secret)
	if message == err.Error() {
		return err
	}
	return &impriRedactedError{cause: err, message: message}
}

func redactImpriText(message, secret string) string {
	if secret == "" {
		return message
	}
	for _, candidate := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret), strconv.Quote(secret)} {
		if candidate != "" {
			message = strings.ReplaceAll(message, candidate, "[REDACTED]")
		}
	}
	return message
}

func decodeImpriAction(data []byte) (impriAction, error) {
	var wire struct {
		ID        string         `json:"id"`
		Kind      string         `json:"kind"`
		Status    string         `json:"status"`
		InboxURL  string         `json:"inbox_url"`
		TargetURL string         `json:"target_url"`
		Payload   map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return impriAction{}, fmt.Errorf("decode Impri action: %w", err)
	}
	if strings.TrimSpace(wire.ID) == "" || wire.Kind != PRMergeKind ||
		strings.TrimSpace(wire.Status) == "" || strings.TrimSpace(wire.TargetURL) == "" ||
		wire.Payload == nil {
		return impriAction{}, fmt.Errorf("impri action response is missing a valid id, kind, status, target_url, or payload")
	}
	return impriAction{
		ID: wire.ID, Kind: wire.Kind, Status: wire.Status, InboxURL: wire.InboxURL,
		TargetURL: wire.TargetURL, Payload: wire.Payload,
	}, nil
}

func (a *impriAction) UnmarshalJSON(data []byte) error {
	decoded, err := decodeImpriAction(data)
	if err != nil {
		return err
	}
	*a = decoded
	return nil
}

func mergeIdempotencyKey(snapshot PRMergeSnapshot) (string, error) {
	identity := struct {
		Provider      string `json:"provider"`
		ForgeBaseURL  string `json:"forge_base_url"`
		Owner         string `json:"owner"`
		Repo          string `json:"repo"`
		PRNumber      int64  `json:"pr_number"`
		HeadSHA       string `json:"head_sha"`
		BaseBranch    string `json:"base_branch"`
		MergeMethod   string `json:"merge_method"`
		ExecutionMode string `json:"execution_mode"`
		PRURL         string `json:"pr_url"`
	}{
		Provider: snapshot.Provider, ForgeBaseURL: snapshot.ForgeBaseURL,
		Owner: snapshot.Owner, Repo: snapshot.Repo, PRNumber: snapshot.PRNumber,
		HeadSHA: snapshot.HeadSHA, BaseBranch: snapshot.BaseBranch,
		MergeMethod: snapshot.MergeMethod, ExecutionMode: snapshot.ExecutionMode, PRURL: snapshot.PRURL,
	}
	data, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("encode merge identity: %w", err)
	}
	digest := sha256.Sum256(data)
	return "og-pr-merge-" + hex.EncodeToString(digest[:]), nil
}

func mergeActionPayload(snapshot PRMergeSnapshot) map[string]any {
	return map[string]any{
		"provider":       snapshot.Provider,
		"forge_base_url": snapshot.ForgeBaseURL,
		"owner":          snapshot.Owner,
		"repo":           snapshot.Repo,
		"pr_number":      snapshot.PRNumber,
		"head_sha":       snapshot.HeadSHA,
		"base_branch":    snapshot.BaseBranch,
		"merge_method":   snapshot.MergeMethod,
		"execution_mode": snapshot.ExecutionMode,
		"pr_url":         snapshot.PRURL,
	}
}

func mergePreview(snapshot PRMergeSnapshot) string {
	mode := "real squash merge"
	if snapshot.ExecutionMode == PRMergeModeDryRun {
		mode = "dry-run mock merge (the forge will not be changed)"
	}
	return strings.Join([]string{
		"## Approval-gated pull-request merge",
		"",
		"Approve this action to run a **" + mode + "**.",
		"",
		"- Provider: `" + snapshot.Provider + "`",
		"- Forge: `" + snapshot.ForgeBaseURL + "`",
		"- Repository: `" + snapshot.Owner + "/" + snapshot.Repo + "`",
		"- Pull request: `#" + strconv.FormatInt(snapshot.PRNumber, 10) + "`",
		"- Head SHA: `" + snapshot.HeadSHA + "`",
		"- Base branch: `" + snapshot.BaseBranch + "`",
		"- Merge method: `squash`",
		"- Execution mode: `" + snapshot.ExecutionMode + "`",
	}, "\n")
}
