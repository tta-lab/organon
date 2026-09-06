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
	if _, err := NormalizePRMergeRequest(Request{ActionID: " act "}); err == nil ||
		!strings.Contains(err.Error(), "action ID must not contain surrounding whitespace") {
		t.Fatalf("whitespace action ID error = %v", err)
	}
}
