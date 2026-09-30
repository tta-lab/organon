package og

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizePRMergeRequestSharesTimeoutDefaultsAndValidation(t *testing.T) {
	normalized, err := NormalizePRMergeRequest(Request{Wait: true})
	if err != nil {
		t.Fatalf("normalize default timeout: %v", err)
	}
	if normalized.Timeout != DefaultPRMergeTimeout {
		t.Fatalf("normalized timeout = %s, want %s", normalized.Timeout, DefaultPRMergeTimeout)
	}
	zero, err := NormalizePRMergeRequest(Request{Wait: true, Timeout: 0})
	if err != nil || zero.Timeout != DefaultPRMergeTimeout {
		t.Fatalf("explicit zero timeout = %s, err = %v, want shared default", zero.Timeout, err)
	}
	if _, err := NormalizePRMergeRequest(Request{Wait: true, Timeout: -time.Second}); err == nil ||
		!strings.Contains(err.Error(), "merge timeout must not be negative") {
		t.Fatalf("negative timeout error = %v", err)
	}
}

func TestValidateDirectMergeRejectsMisleadingOutcomes(t *testing.T) {
	valid := PRMergeResult{ApprovalPolicy: "none", Status: PRMergeStatusExecuted,
		NextAction: PRMergeNextNone, Completion: "complete",
		Snapshot: PRMergeSnapshot{PRNumber: 7, PRURL: "https://fixture/pull/7"}}
	for _, mutate := range []func(*PRMergeResult){
		func(r *PRMergeResult) { r.ApprovalPolicy = "" },
		func(r *PRMergeResult) { r.ActionID = "action" },
		func(r *PRMergeResult) { r.InboxURL = "https://fixture/inbox" },
		func(r *PRMergeResult) { r.ReceiptError = "receipt" },
		func(r *PRMergeResult) { r.Status = PRMergeStatusApproved },
		func(r *PRMergeResult) { r.Status = PRMergeStatusFailed; r.NextAction = PRMergeNextNewApproval },
		func(r *PRMergeResult) { r.Status = PRMergeStatusBlocked },
		func(r *PRMergeResult) { r.CleanupError = "cleanup" },
		func(r *PRMergeResult) { r.Snapshot.PRNumber = 8 },
	} {
		invalid := valid
		mutate(&invalid)
		if err := ValidatePRMergeResponse(Response{Merge: &invalid}, 7); err == nil {
			t.Fatalf("accepted %+v", invalid)
		}
	}
}
