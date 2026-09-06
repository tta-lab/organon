package og

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

// DryRunMergeExecutor is the injected non-destructive executor used by the
// dry-run approval path. It must not call a forge.
type DryRunMergeExecutor func(context.Context, PRMergeSnapshot) error

// NewServiceWithConfigAndDryRunExecutor is the test/operator seam for the
// approval-gated dry-run executor. A nil executor records a successful mock.
func NewServiceWithConfigAndDryRunExecutor(
	githubBroker githubapp.CredentialBroker,
	projects *project.Store,
	cfg ogconfig.Config,
	executor DryRunMergeExecutor,
) Service {
	return newServiceWithDryRunExecutor(githubBroker, projects, cfg, executor)
}

// NewServiceWithDryRunExecutor is a convenience constructor using the default
// project registry and trust configuration.
func NewServiceWithDryRunExecutor(
	githubBroker githubapp.CredentialBroker, executor DryRunMergeExecutor,
) Service {
	return newServiceWithDryRunExecutor(githubBroker, nil, ogconfig.Config{}, executor)
}

const defaultImpriExpirySeconds = 259200

func newServiceWithDryRunExecutor(
	githubBroker githubapp.CredentialBroker,
	projects *project.Store,
	cfg ogconfig.Config,
	executor DryRunMergeExecutor,
) Service {
	service := NewServiceWithConfig(githubBroker, projects, cfg)
	service.dryRunMergeExecutor = executor
	return service
}

func defaultDryRunMergeExecutor(context.Context, PRMergeSnapshot) error { return nil }

func (s Service) PRMerge(req Request) (Response, error) { //nolint:gocyclo
	var err error
	req, err = NormalizePRMergeRequest(req)
	if err != nil {
		return Response{}, err
	}
	ctxInfo, err := s.resolvePRContextForRequest(req)
	if err != nil {
		return Response{}, err
	}
	if err := requireRemoteWrite(ctxInfo, "merge pull request"); err != nil {
		return Response{}, err
	}
	client, err := newImpriClient(operationContext(ctxInfo), ctxInfo.config.Impri)
	if err != nil {
		return Response{}, err
	}

	mode := PRMergeModeReal
	if req.DryRun {
		mode = PRMergeModeDryRun
	}
	provider, providerErr := newProvider(ctxInfo)
	if providerErr != nil {
		return Response{}, providerErr
	}
	index := req.Index
	if index == 0 {
		found, findErr := findPR(ctxInfo, stateAll)
		if findErr != nil {
			return Response{}, findErr
		}
		index = found.Index
	}
	snapshot, err := loadMergeSnapshot(ctxInfo, provider, index, mode)
	if err != nil {
		return Response{}, err
	}
	if req.Index > 0 && snapshot.PRNumber != req.Index {
		return Response{}, fmt.Errorf("provider returned PR #%d, want #%d", snapshot.PRNumber, req.Index)
	}
	key, keyErr := mergeIdempotencyKey(snapshot)
	if keyErr != nil {
		return Response{}, keyErr
	}
	action, err := createMergeAction(operationContext(ctxInfo), client, snapshot, key)
	if err != nil {
		if response, retryErr, unavailable := unavailableAfterCreateError(action, err, client, snapshot,
			"approval card was created but its canonical state could not be read"); unavailable {
			return response, retryErr
		}
		return Response{}, err
	}
	for requiresNewApproval(action.Status) {
		previousActionID := action.ID
		action, err = createMergeAction(operationContext(ctxInfo), client, snapshot,
			mergeRetryIdempotencyKey(key, previousActionID))
		if err != nil {
			if response, retryErr, unavailable := unavailableAfterCreateError(action, err, client, snapshot,
				"replacement approval card was created but its canonical state could not be read"); unavailable {
				return response, retryErr
			}
			return Response{}, err
		}
		if action.ID == previousActionID {
			return Response{}, fmt.Errorf("replacement approval action did not advance from %s", previousActionID)
		}
	}

	if action.InboxURL == "" {
		action.InboxURL = client.inboxURL()
	}
	return s.processMergeAction(ctxInfo, client, action, snapshot, req)
}

func createMergeAction(
	ctx context.Context, client *impriClient, snapshot PRMergeSnapshot, key string,
) (impriAction, error) {
	action, err := client.createAction(ctx, impriCreateAction{
		Kind:    PRMergeKind,
		Title:   mergeActionTitle(snapshot),
		Preview: impriPreview{Format: "markdown", Body: mergePreview(snapshot)},
		Payload: mergeActionPayload(snapshot), TargetURL: snapshot.PRURL,
		ExpiresIn: defaultImpriExpirySeconds, IdempotencyKey: key,
	})
	if err != nil {
		return action, err
	}
	if action.TargetURL != snapshot.PRURL {
		return impriAction{}, fmt.Errorf("impri action target_url does not match the proposed PR snapshot")
	}
	if action.Payload != nil {
		returned, payloadErr := snapshotFromAction(action)
		if payloadErr != nil {
			return impriAction{}, payloadErr
		}
		if !mergeIdentityEqual(snapshot, returned) {
			return impriAction{}, fmt.Errorf("impri action identity does not match the proposed PR snapshot")
		}
	}
	return action, nil
}

func requiresNewApproval(status string) bool {
	return status == PRMergeStatusRejected || status == PRMergeStatusExpired
}

func unavailableAfterCreateError(
	action impriAction, err error, client *impriClient, snapshot PRMergeSnapshot, detail string,
) (Response, error, bool) {
	if action.ID == "" || !retryableImpriReadError(err) {
		return Response{}, nil, false
	}
	inboxURL := action.InboxURL
	if inboxURL == "" {
		inboxURL = client.inboxURL()
	}
	response, retryErr := retryableImpriUnavailable(action.ID, inboxURL, snapshot, detail)
	return response, retryErr, true
}

func loadMergeSnapshot(
	ctxInfo *repoContext, provider gitprovider.Provider, index int64, mode string,
) (PRMergeSnapshot, error) {
	if err := ValidatePositivePRID(index); err != nil {
		return PRMergeSnapshot{}, err
	}
	pr, err := provider.GetPR(ctxInfo.Owner, ctxInfo.Repo, index)
	if err != nil {
		return PRMergeSnapshot{}, err
	}
	if !validProviderPRIdentity(pr, index) {
		return PRMergeSnapshot{}, fmt.Errorf("provider returned invalid PR snapshot for #%d", index)
	}
	if strings.TrimSpace(pr.Head) == "" || strings.TrimSpace(pr.HeadSHA) == "" || strings.TrimSpace(pr.Base) == "" {
		return PRMergeSnapshot{}, fmt.Errorf("provider returned incomplete PR snapshot for #%d", index)
	}
	prURL := pr.HTMLURL
	if prURL == "" {
		prURL = fallbackPRURL(ctxInfo, index)
	}
	if prURL == "" {
		return PRMergeSnapshot{}, fmt.Errorf("provider returned PR #%d without a target URL", index)
	}
	return PRMergeSnapshot{
		Provider: string(ctxInfo.Provider), ForgeBaseURL: ctxInfo.BaseURL,
		Owner: ctxInfo.Owner, Repo: ctxInfo.Repo, PRNumber: pr.Index,
		HeadSHA: pr.HeadSHA, BaseBranch: pr.Base, MergeMethod: PRMergeMethodSquash,
		ExecutionMode: mode, PRURL: prURL, Title: pr.Title, Head: pr.Head,
		State: pr.State, Mergeable: pr.Mergeable,
	}, nil
}

func fallbackPRURL(ctxInfo *repoContext, index int64) string {
	if ctxInfo == nil || ctxInfo.BaseURL == "" || ctxInfo.Owner == "" || ctxInfo.Repo == "" {
		return ""
	}
	segment := "pulls"
	if ctxInfo.Provider == gitprovider.ProviderGitHub {
		segment = "pull"
	}
	return fmt.Sprintf("%s/%s/%s/%s/%d", strings.TrimRight(ctxInfo.BaseURL, "/"),
		ctxInfo.Owner, ctxInfo.Repo, segment, index)
}

func snapshotFromAction(action impriAction) (PRMergeSnapshot, error) { //nolint:gocyclo
	if action.ID == "" || action.Kind != PRMergeKind || action.Status == "" ||
		strings.TrimSpace(action.TargetURL) == "" || action.Payload == nil {
		return PRMergeSnapshot{}, fmt.Errorf("malformed Impri action response")
	}
	payload := action.Payload
	snapshot := PRMergeSnapshot{
		Provider:      stringValueFromMap(payload, "provider"),
		ForgeBaseURL:  stringValueFromMap(payload, "forge_base_url"),
		Owner:         stringValueFromMap(payload, "owner"),
		Repo:          stringValueFromMap(payload, "repo"),
		HeadSHA:       stringValueFromMap(payload, "head_sha"),
		BaseBranch:    stringValueFromMap(payload, "base_branch"),
		MergeMethod:   stringValueFromMap(payload, "merge_method"),
		ExecutionMode: stringValueFromMap(payload, "execution_mode"),
		PRURL:         stringValueFromMap(payload, "pr_url"),
		Title:         stringValueFromMap(payload, "title"),
		Head:          stringValueFromMap(payload, "head"),
		State:         stringValueFromMap(payload, "state"),
		CIState:       stringValueFromMap(payload, "ci_state"),
	}
	var ok bool
	snapshot.PRNumber, ok = int64ValueFromMap(payload, "pr_number")
	if !ok || snapshot.PRNumber <= 0 || snapshot.Provider == "" || snapshot.ForgeBaseURL == "" ||
		snapshot.Owner == "" || snapshot.Repo == "" || snapshot.HeadSHA == "" ||
		snapshot.BaseBranch == "" || snapshot.Head == "" || snapshot.MergeMethod == "" ||
		snapshot.ExecutionMode == "" {
		return PRMergeSnapshot{}, fmt.Errorf("malformed or incomplete Impri action identity")
	}
	if snapshot.MergeMethod != PRMergeMethodSquash ||
		(snapshot.ExecutionMode != PRMergeModeDryRun && snapshot.ExecutionMode != PRMergeModeReal) {
		return PRMergeSnapshot{}, fmt.Errorf("impri action uses an unsupported merge identity")
	}
	if action.TargetURL != snapshot.PRURL {
		return PRMergeSnapshot{}, fmt.Errorf("impri action target_url does not match its PR payload")
	}
	if value, exists := payload["mergeable"]; exists {
		if mergeable, valid := value.(bool); valid {
			snapshot.Mergeable = mergeable
		}
	}
	return snapshot, nil
}

func stringValueFromMap(values map[string]any, key string) string {
	if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return ""
}

func int64ValueFromMap(values map[string]any, key string) (int64, bool) {
	switch value := values[key].(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case float64:
		if value == float64(int64(value)) {
			return int64(value), true
		}
	case json.Number:
		parsed, err := value.Int64()
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func mergeIdentityEqual(left, right PRMergeSnapshot) bool {
	return left.Provider == right.Provider && left.ForgeBaseURL == right.ForgeBaseURL &&
		left.Owner == right.Owner && left.Repo == right.Repo && left.PRNumber == right.PRNumber &&
		left.HeadSHA == right.HeadSHA && left.BaseBranch == right.BaseBranch &&
		left.Head == right.Head && left.MergeMethod == right.MergeMethod && left.ExecutionMode == right.ExecutionMode &&
		left.PRURL == right.PRURL
}

//nolint:gocyclo
func (s Service) processMergeAction(
	ctxInfo *repoContext, client *impriClient, action impriAction,
	snapshot PRMergeSnapshot, req Request,
) (Response, error) {
	status := action.Status
	waitTimedOut := false
	if status == PRMergeStatusPending && req.Wait {
		waitingAction := action
		var err error
		action, waitTimedOut, err = waitForImpriAction(operationContext(ctxInfo), client, action.ID, req.Timeout)
		if err != nil {
			if !retryableImpriReadError(err) {
				return Response{}, err
			}
			return retryableImpriUnavailable(waitingAction.ID, waitingAction.InboxURL, snapshot,
				"approval state could not be read while waiting")
		}
		if action.TargetURL != snapshot.PRURL {
			return Response{}, fmt.Errorf("impri action target_url does not match the approved PR snapshot")
		}
		status = action.Status
		if action.Payload != nil {
			updated, parseErr := snapshotFromAction(action)
			if parseErr != nil {
				return Response{}, parseErr
			}
			if !mergeIdentityEqual(snapshot, updated) {
				return Response{}, fmt.Errorf("impri action identity changed while waiting")
			}
			updated.Title = snapshot.Title
			snapshot = updated
		}
		if action.InboxURL == "" {
			action.InboxURL = client.inboxURL()
		}
	}
	result := &PRMergeResult{ActionID: action.ID, Status: status, InboxURL: action.InboxURL, Snapshot: snapshot}
	setMergeOutcome(result)
	switch status {
	case PRMergeStatusPending:
		result.Detail = "approval is pending; open the Impri inbox and approve or reject the action"
		if waitTimedOut {
			result.Detail = "approval wait timed out; repeat the same request to check the pending action"
		}
	case PRMergeStatusRejected:
		result.Detail = "approval was rejected; the pull request was not changed"
	case PRMergeStatusExpired:
		result.Detail = "approval expired; the pull request was not changed"
	case PRMergeStatusExecuteFailed:
		result.Detail = "execution already failed; a newly approved action is required"
	case PRMergeStatusExecuted:
		if snapshot.ExecutionMode == PRMergeModeReal {
			return s.finishExecutedRealMerge(ctxInfo, action, snapshot)
		}
		result.Detail = "approval was already executed; no merge was attempted"
	case PRMergeStatusApproved:
		return s.executeApprovedMerge(ctxInfo, client, action, snapshot)
	default:
		return Response{}, fmt.Errorf("unknown Impri action status %q; refusing to execute", status)
	}
	return success(Response{Message: result.Detail, Merge: result}), nil
}

func waitForImpriAction(
	ctx context.Context, client *impriClient, actionID string, timeout time.Duration,
) (impriAction, bool, error) {
	if timeout <= 0 {
		timeout = impriDefaultWait
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(impriPollInterval)
	defer ticker.Stop()
	last, err := client.getAction(waitCtx, actionID)
	if err != nil {
		return impriAction{}, false, err
	}
	if last.Status != PRMergeStatusPending {
		return last, false, nil
	}
	for {
		select {
		case <-waitCtx.Done():
			if waitCtx.Err() == context.DeadlineExceeded {
				return last, true, nil
			}
			return impriAction{}, false, waitCtx.Err()
		case <-ticker.C:
			next, getErr := client.getAction(waitCtx, actionID)
			if getErr != nil {
				return impriAction{}, false, getErr
			}
			last = next
			if last.Status != PRMergeStatusPending {
				return last, false, nil
			}
		}
	}
}

//nolint:gocyclo
func (s Service) executeApprovedMerge(
	ctxInfo *repoContext, client *impriClient, action impriAction,
	approved PRMergeSnapshot,
) (Response, error) {
	if !mergeSnapshotRepositoryMatches(ctxInfo, approved) {
		return retryableMergeError(action, approved, "the approved repository does not match the checkout")
	}
	provider, err := newProvider(ctxInfo)
	if err != nil {
		if unsupportedProviderSetup(ctxInfo, err) {
			return s.executionFailure(ctxInfo, client, action, approved, "unsupported merge provider: "+err.Error())
		}
		return retryableMergeError(action, approved, "provider setup is temporarily unavailable: "+err.Error())
	}
	current, err := provider.GetPR(ctxInfo.Owner, ctxInfo.Repo, approved.PRNumber)
	if err != nil {
		return retryableMergeError(action, approved, "pull request refetch is temporarily unavailable: "+err.Error())
	}
	if !validProviderPRIdentity(current, approved.PRNumber) {
		return retryableMergeError(action, approved, "the current pull request could not be verified")
	}
	currentSnapshot := snapshotFromProvider(ctxInfo, current, approved.ExecutionMode)
	if strings.TrimSpace(currentSnapshot.Head) == "" || strings.TrimSpace(currentSnapshot.HeadSHA) == "" ||
		strings.TrimSpace(currentSnapshot.BaseBranch) == "" || strings.TrimSpace(currentSnapshot.PRURL) == "" {
		return retryableMergeError(action, approved, "the current pull request identity could not be verified")
	}
	if !mergeIdentityEqual(approved, currentSnapshot) {
		return s.executionFailure(ctxInfo, client, action, approved, "pull request identity changed after approval")
	}
	if current.Merged || strings.EqualFold(current.State, "merged") {
		if approved.ExecutionMode == PRMergeModeDryRun {
			return s.reportExecution(ctxInfo, client, action, approved, PRMergeStatusExecuted,
				"dry-run mock merge observed an already merged PR; the forge was not changed")
		}
		return s.finishRealMerge(ctxInfo, client, action, approved,
			"pull request is already merged; repaired the approval receipt without merging again")
	}
	if !strings.EqualFold(current.State, "open") {
		if strings.EqualFold(current.State, "closed") {
			return s.executionFailure(ctxInfo, client, action, approved, "pull request is closed and unmerged")
		}
		return retryableMergeError(action, approved, "pull request state could not be verified")
	}
	if approved.ExecutionMode == PRMergeModeDryRun {
		executor := s.dryRunMergeExecutor
		if executor == nil {
			executor = defaultDryRunMergeExecutor
		}
		if err := executor(operationContext(ctxInfo), approved); err != nil {
			return s.executionFailure(ctxInfo, client, action, approved, "dry-run executor failed: "+err.Error())
		}
		return s.reportExecution(ctxInfo, client, action, approved, PRMergeStatusExecuted,
			"dry-run mock merge executed; the forge was not changed")
	}
	if !current.Mergeable {
		return s.executionFailure(ctxInfo, client, action, approved, "pull request is not mergeable")
	}
	ci, ciErr := provider.GetCombinedStatus(ctxInfo.Owner, ctxInfo.Repo, current.HeadSHA)
	if ciErr != nil {
		return retryableMergeError(action, approved, "CI status is temporarily unavailable: "+ciErr.Error())
	}
	if ci == nil || strings.TrimSpace(ci.State) == "" {
		return retryableMergeError(action, approved, "CI status could not be verified")
	}
	switch ci.State {
	case gitprovider.StateFailure, gitprovider.StateError:
		return s.executionFailure(ctxInfo, client, action, approved, "CI is not green (state: "+ci.State+")")
	case gitprovider.StatePending:
		return retryableMergeError(action, approved, "CI is still pending; retry after it settles")
	case gitprovider.StateSuccess, gitprovider.StateNotConfigured:
		// Continue to the forge's expected-head merge operation.
	default:
		return retryableMergeError(action, approved, "CI status could not be verified (state: "+ci.State+")")
	}
	merger, ok := provider.(gitprovider.PullRequestMerger)
	if !ok {
		return s.executionFailure(ctxInfo, client, action, approved, "configured provider does not support squash merge")
	}
	if cleanupErr := s.prepareApprovedMergeCleanup(ctxInfo, approved); cleanupErr != nil {
		return retryableMergeError(action, approved,
			"checkout is not ready for automatic cleanup: "+cleanupErr.Error())
	}
	if err := merger.MergePullRequest(ctxInfo.Owner, ctxInfo.Repo, approved.PRNumber, approved.HeadSHA); err != nil {
		return s.recoverMergeAfterProviderError(
			ctxInfo, provider, client, action, approved, "forge squash merge failed: "+err.Error(),
		)
	}
	return s.finishRealMerge(ctxInfo, client, action, approved,
		"squash merge executed successfully")
}

func (s Service) prepareApprovedMergeCleanup(
	ctxInfo *repoContext, approved PRMergeSnapshot,
) error {
	cleanupCtx, err := s.resolveMergeCleanupContext(ctxInfo)
	if err != nil {
		return err
	}
	target, err := approvedMergeCleanupTarget(cleanupCtx, approved)
	if err != nil {
		return err
	}
	if err := requireGitPushTarget(cleanupCtx, "automatic merge cleanup"); err != nil {
		return err
	}
	if err := ensureBranchCleanupTarget(cleanupCtx, target); err != nil {
		return err
	}
	return nil
}

func (s Service) resolveMergeCleanupContext(ctxInfo *repoContext) (*repoContext, error) {
	if ctxInfo == nil {
		return nil, fmt.Errorf("repository context is missing")
	}
	cleanupCtx, err := resolveRepoContextWith(
		operationContext(ctxInfo), ctxInfo.WorkDir, s.projectStore(), s.config,
	)
	if err != nil {
		return nil, err
	}
	cleanupCtx.githubBroker = s.githubBroker
	return cleanupCtx, nil
}

func (s Service) finishRealMerge(
	ctxInfo *repoContext, client *impriClient, action impriAction,
	approved PRMergeSnapshot, detail string,
) (Response, error) {
	receipt, err := s.reportExecution(ctxInfo, client, action, approved,
		PRMergeStatusExecuted, detail)
	if err != nil {
		return receipt, err
	}
	if receipt.Merge == nil {
		return Response{}, fmt.Errorf("executed merge response is missing its result")
	}
	// A failed receipt write leaves the action approved/retryable in Impri. Do
	// not move on to cleanup until the receipt can be recorded; the retry then
	// repairs the receipt and continues through the same cleanup path.
	if receipt.Merge.ReceiptError != "" {
		return receipt, nil
	}
	cleanupCtx, err := s.resolveMergeCleanupContext(ctxInfo)
	if err != nil {
		return cleanupRetryableResult(receipt.Merge, ctxInfo, err)
	}
	if err := cleanupApprovedMergeBranch(cleanupCtx, approved); err != nil {
		return cleanupRetryableResult(receipt.Merge, cleanupCtx, err)
	}
	setCompletedRealMergeResult(receipt.Merge, detail)
	receipt.Message = receipt.Merge.Detail
	return receipt, nil
}

func (s Service) finishExecutedRealMerge(
	ctxInfo *repoContext, action impriAction,
	approved PRMergeSnapshot,
) (Response, error) {
	result := &PRMergeResult{
		ActionID: action.ID, Status: PRMergeStatusExecuted, InboxURL: action.InboxURL,
		Snapshot: approved,
		Detail:   "approval was already executed; no forge merge was attempted",
	}
	setMergeOutcome(result)
	cleanupCtx, err := s.resolveMergeCleanupContext(ctxInfo)
	if err != nil {
		return cleanupRetryableResult(result, ctxInfo, err)
	}
	if err := cleanupApprovedMergeBranch(cleanupCtx, approved); err != nil {
		return cleanupRetryableResult(result, cleanupCtx, err)
	}
	setCompletedRealMergeResult(result, "approval was already executed; no forge merge was attempted")
	return success(Response{Message: result.Detail, Merge: result}), nil
}

func setCompletedRealMergeResult(result *PRMergeResult, detail string) {
	result.Status = PRMergeStatusExecuted
	result.Retryable = false
	result.NextAction = PRMergeNextNone
	result.ReceiptError = ""
	result.CleanupError = ""
	result.Detail = detail + "; Impri executed receipt recorded; default branch pulled; " +
		"approved head branch cleanup completed locally and remotely"
	result.Completion = "the approved merge, executed receipt, default-branch pull, and " +
		"approved head cleanup are complete; do not execute this action again"
}

func cleanupRetryableResult(
	result *PRMergeResult, ctxInfo *repoContext, err error,
) (Response, error) {
	cleanupErr := ""
	if err != nil {
		cleanupErr = err.Error()
	}
	if ctxInfo != nil {
		cleanupErr = redactSecret(cleanupErr, ctxInfo.Token)
	}
	if cleanupErr == "" {
		cleanupErr = "checkout cleanup did not complete"
	}
	result.Status = PRMergeStatusExecuted
	result.Retryable = true
	result.NextAction = PRMergeNextRetry
	result.CleanupError = cleanupErr
	result.Completion = "the forge merge and Impri executed receipt are complete; repeat the " +
		"same project, PR, and mode request to finish checkout cleanup; the forge merge " +
		"must not run again"
	result.Detail = "the forge merge is complete and must not run again; automatic checkout " +
		"cleanup remains incomplete: " + cleanupErr
	response := Response{Error: result.Detail, Message: result.Detail, Merge: result}
	return response, &PRMergeRetryableError{Result: *result}
}

func (s Service) recoverMergeAfterProviderError(
	ctxInfo *repoContext, provider gitprovider.Provider, client *impriClient,
	action impriAction, approved PRMergeSnapshot, failure string,
) (Response, error) {
	current, err := provider.GetPR(ctxInfo.Owner, ctxInfo.Repo, approved.PRNumber)
	if err != nil {
		return retryableMergeError(action, approved,
			failure+"; could not verify the post-error PR state: "+err.Error())
	}
	if !validProviderPRIdentity(current, approved.PRNumber) {
		return retryableMergeError(action, approved, failure+"; the post-error pull request state could not be verified")
	}
	currentSnapshot := snapshotFromProvider(ctxInfo, current, approved.ExecutionMode)
	if strings.TrimSpace(currentSnapshot.Head) == "" || strings.TrimSpace(currentSnapshot.HeadSHA) == "" ||
		strings.TrimSpace(currentSnapshot.BaseBranch) == "" || strings.TrimSpace(currentSnapshot.PRURL) == "" {
		return retryableMergeError(action, approved, failure+"; the post-error pull request identity could not be verified")
	}
	if !mergeIdentityEqual(approved, currentSnapshot) {
		return s.executionFailure(ctxInfo, client, action, approved,
			failure+"; pull request identity changed after the merge error")
	}
	if current.Merged || strings.EqualFold(current.State, "merged") {
		return s.finishRealMerge(ctxInfo, client, action, approved,
			"forge reported an error, but the approved PR is merged; repaired the approval receipt")
	}
	if strings.EqualFold(current.State, "open") {
		return retryableMergeError(action, approved,
			failure+"; forge outcome is ambiguous and the approved PR remains open")
	}
	if strings.EqualFold(current.State, "closed") {
		return s.executionFailure(ctxInfo, client, action, approved,
			failure+"; pull request is closed and unmerged")
	}
	return retryableMergeError(action, approved, failure+"; the post-error pull request state could not be verified")
}

func retryableMergeError(action impriAction, snapshot PRMergeSnapshot, detail string) (Response, error) {
	return retryableMergeErrorWithCompletion(action, snapshot, detail,
		"repeat the same project, PR, and mode request after the temporary failure; "+
			"execution completes only when status is executed")
}

func retryableMergeErrorWithCompletion(
	action impriAction, snapshot PRMergeSnapshot, detail, completion string,
) (Response, error) {
	result := PRMergeResult{
		ActionID: action.ID, Status: PRMergeStatusApproved, InboxURL: action.InboxURL,
		Snapshot: snapshot, Retryable: true,
		NextAction: PRMergeNextRetry,
		Completion: completion,
		Detail:     fmt.Sprintf("impri action %s remains approved; repeat the same request: %s", action.ID, detail),
	}
	return Response{Error: result.Detail, Message: result.Detail, Merge: &result}, &PRMergeRetryableError{Result: result}
}

func retryableImpriUnavailable(
	actionID, inboxURL string, snapshot PRMergeSnapshot, detail string,
) (Response, error) {
	result := PRMergeResult{
		ActionID: actionID, Status: PRMergeStatusUnavailable, InboxURL: inboxURL,
		Snapshot: snapshot, Retryable: true,
		NextAction: PRMergeNextRetry,
		Completion: "repeat the same project, PR, and mode request when Impri approval state is available; " +
			"completion requires a known Impri status",
		Detail: fmt.Sprintf("Impri approval state is temporarily unavailable for action %s; "+
			"no forge call was made; repeat the same request: %s", actionID, detail),
	}
	return Response{Error: result.Detail, Message: result.Detail, Merge: &result},
		&PRMergeRetryableError{Result: result}
}

func unsupportedProviderSetup(ctxInfo *repoContext, err error) bool {
	if ctxInfo != nil && ctxInfo.Provider == gitprovider.ProviderGeneric {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unsupported provider") || strings.Contains(message, "no provider api")
}

func snapshotFromProvider(ctxInfo *repoContext, pr *gitprovider.PullRequest, mode string) PRMergeSnapshot {
	prURL := pr.HTMLURL
	if prURL == "" {
		prURL = fallbackPRURL(ctxInfo, pr.Index)
	}
	return PRMergeSnapshot{
		Provider: string(ctxInfo.Provider), ForgeBaseURL: ctxInfo.BaseURL, Owner: ctxInfo.Owner,
		Repo: ctxInfo.Repo, PRNumber: pr.Index, HeadSHA: pr.HeadSHA, BaseBranch: pr.Base,
		MergeMethod: PRMergeMethodSquash, ExecutionMode: mode, PRURL: prURL,
		Title: pr.Title, Head: pr.Head, State: pr.State, Mergeable: pr.Mergeable,
	}
}

func mergeSnapshotRepositoryMatches(ctxInfo *repoContext, snapshot PRMergeSnapshot) bool {
	return ctxInfo != nil && snapshot.Provider == string(ctxInfo.Provider) &&
		strings.TrimRight(snapshot.ForgeBaseURL, "/") == strings.TrimRight(ctxInfo.BaseURL, "/") &&
		snapshot.Owner == ctxInfo.Owner && snapshot.Repo == ctxInfo.Repo
}

func (s Service) reportExecution(
	ctxInfo *repoContext, client *impriClient, action impriAction, snapshot PRMergeSnapshot,
	status, detail string,
) (Response, error) {
	result := &PRMergeResult{
		ActionID: action.ID, Status: status, InboxURL: action.InboxURL,
		Snapshot: snapshot, Detail: detail,
	}
	setMergeOutcome(result)
	reportErr := client.reportResult(operationContext(ctxInfo), action.ID, status, detail, mergeActionPayload(snapshot))
	if reportErr != nil {
		if status != PRMergeStatusExecuted {
			return retryableMergeErrorWithCompletion(action, snapshot,
				"deterministic execution failure could not be recorded in Impri; "+
					"repeat the same request to revalidate and report it: "+detail+
					"; Impri receipt error: "+reportErr.Error(),
				"repeat the same request to revalidate and record the deterministic "+
					"execute_failed result; completion is confirmed when Impri reports execute_failed")
		}
		result.ReceiptError = reportErr.Error()
		result.Retryable = true
		result.NextAction = PRMergeNextRepairReceipt
		result.Completion = "repeat the same request to record the executed receipt; the forge merge is complete " +
			"and must not run again"
		result.Detail += "; Impri receipt could not be recorded; repeat the same request to repair it"
		return success(Response{Message: result.Detail, Merge: result}), nil
	}
	return success(Response{Message: result.Detail, Merge: result}), nil
}

func (s Service) executionFailure(
	ctxInfo *repoContext, client *impriClient, action impriAction,
	snapshot PRMergeSnapshot, detail string,
) (Response, error) {
	return s.reportExecution(ctxInfo, client, action, snapshot, PRMergeStatusExecuteFailed, detail)
}

func setMergeOutcome(result *PRMergeResult) {
	switch result.Status {
	case PRMergeStatusPending:
		result.NextAction = PRMergeNextWait
		result.Completion = "surface the inbox URL and repeat the same request after Impri records approved or rejected"
	case PRMergeStatusRejected, PRMergeStatusExpired, PRMergeStatusExecuteFailed:
		result.NextAction = PRMergeNextNewApproval
		result.Completion = "this action is terminal; create a new approval action for another merge attempt"
	case PRMergeStatusExecuted:
		result.NextAction = PRMergeNextNone
		result.Completion = "the approved merge is complete; do not execute this action again"
	case PRMergeStatusUnavailable:
		result.Retryable = true
		result.NextAction = PRMergeNextRetry
		result.Completion = "repeat the same project, PR, and mode request when Impri approval state is available; " +
			"completion requires a known Impri status"
	}
}
