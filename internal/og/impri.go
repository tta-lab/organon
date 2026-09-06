package og

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	impriDefaultWait    = 30 * time.Second
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
	var action impriAction
	if err := c.requestJSON(ctx, http.MethodPost, impriActionsPath, body, &action); err != nil {
		return impriAction{}, fmt.Errorf("create Impri approval action: %w", err)
	}
	return action, nil
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
		return fmt.Errorf("request failed: %w", redactImpriError(err, c.apiKey))
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, impriMaxBodyBytes))
	if readErr != nil {
		return fmt.Errorf("read response: %w", readErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("impri returned HTTP %d: %s", resp.StatusCode, safeImpriBody(data, c.apiKey))
	}
	if output == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

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

func decodeImpriAction(data []byte) (impriAction, error) { //nolint:gocyclo
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return impriAction{}, fmt.Errorf("decode Impri action: %w", err)
	}
	if nested, ok := raw["action"]; ok && len(bytes.TrimSpace(nested)) > 0 && nested[0] == '{' {
		var nestedRaw map[string]json.RawMessage
		if err := json.Unmarshal(nested, &nestedRaw); err == nil {
			for key, value := range nestedRaw {
				if _, exists := raw[key]; !exists {
					raw[key] = value
				}
			}
		}
	}
	if nested, ok := raw["data"]; ok && len(bytes.TrimSpace(nested)) > 0 && nested[0] == '{' {
		var nestedRaw map[string]json.RawMessage
		if err := json.Unmarshal(nested, &nestedRaw); err == nil {
			for key, value := range nestedRaw {
				if _, exists := raw[key]; !exists {
					raw[key] = value
				}
			}
		}
	}
	action := impriAction{}
	action.ID = rawString(raw, "id", "action_id")
	action.Status = rawString(raw, "status", "state")
	action.InboxURL = rawString(raw, "inbox_url", "web_url", "action_url", "url")
	action.TargetURL = rawString(raw, "target_url")
	if payload, ok := raw["payload"]; ok {
		if err := json.Unmarshal(payload, &action.Payload); err != nil {
			return impriAction{}, fmt.Errorf("decode Impri action payload: %w", err)
		}
	}
	if action.ID == "" || action.Status == "" || action.Payload == nil {
		return impriAction{}, fmt.Errorf("impri action response is missing id, status, or payload")
	}
	return action, nil
}

func rawString(raw map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			var result string
			if json.Unmarshal(value, &result) == nil && strings.TrimSpace(result) != "" {
				return result
			}
		}
	}
	return ""
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
	}{
		Provider: snapshot.Provider, ForgeBaseURL: snapshot.ForgeBaseURL,
		Owner: snapshot.Owner, Repo: snapshot.Repo, PRNumber: snapshot.PRNumber,
		HeadSHA: snapshot.HeadSHA, BaseBranch: snapshot.BaseBranch,
		MergeMethod: snapshot.MergeMethod, ExecutionMode: snapshot.ExecutionMode,
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
		"repository":     snapshot.Repo,
		"pr_number":      snapshot.PRNumber,
		"head_sha":       snapshot.HeadSHA,
		"base_branch":    snapshot.BaseBranch,
		"merge_method":   snapshot.MergeMethod,
		"execution_mode": snapshot.ExecutionMode,
		"dry_run":        snapshot.ExecutionMode == PRMergeModeDryRun,
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
