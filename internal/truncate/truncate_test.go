package truncate

import (
	"strings"
	"testing"
)

func TestHeadUntruncated(t *testing.T) {
	content := "a\nb\nc\n"
	r := Head(content, 10, 1024)
	if r.Truncated || r.Content != content || r.TotalLines != 3 || r.OutputLines != 3 || r.TotalBytes != len(content) {
		t.Fatalf("result = %+v", r)
	}
}

func TestHeadDoesNotCountTerminalEmptySegment(t *testing.T) {
	content := strings.Repeat("x\n", 2000)
	r := Head(content, 2000, 1<<20)
	if r.Truncated || r.Content != content || r.TotalLines != 2000 || r.OutputLines != 2000 {
		t.Fatalf("result = %+v", r)
	}
}

func TestHeadLineLimit(t *testing.T) {
	content := strings.Repeat("x\n", 10)
	r := Head(content, 3, 1<<20)
	if !r.Truncated || r.TruncatedBy != "lines" {
		t.Fatalf("result = %+v", r)
	}
	if r.Content != "x\nx\nx" {
		t.Fatalf("content = %q", r.Content)
	}
	if r.OutputLines != 3 || r.TotalLines != 10 {
		t.Fatalf("lines = %d/%d", r.OutputLines, r.TotalLines)
	}
}

func TestHeadByteLimitNeverPartialLines(t *testing.T) {
	content := "aaaa\nbbbb\ncccc\n"
	r := Head(content, 100, 12)
	if !r.Truncated || r.TruncatedBy != "bytes" {
		t.Fatalf("result = %+v", r)
	}
	// "aaaa\nbbbb" is 9 bytes; "cccc" would push past 12, so it is dropped whole.
	if r.Content != "aaaa\nbbbb" {
		t.Fatalf("content = %q", r.Content)
	}
}

func TestHeadFirstLineExceedsLimit(t *testing.T) {
	content := "1234567890\n"
	r := Head(content, 100, 5)
	if !r.Truncated || !r.FirstLineExceedsLimit || r.Content != "" || r.TruncatedBy != "bytes" {
		t.Fatalf("result = %+v", r)
	}
}

func TestHeadBytesHitBeforeLines(t *testing.T) {
	content := "a\n" + strings.Repeat("b", 100) + "\nc\n"
	r := Head(content, 100, 20)
	if !r.Truncated || r.TruncatedBy != "bytes" {
		t.Fatalf("result = %+v", r)
	}
	if r.Content != "a" { // "a\n" + "bb..." would exceed 20 bytes
		t.Fatalf("content = %q", r.Content)
	}
}

func TestHeadEmptyContent(t *testing.T) {
	r := Head("", 10, 1024)
	if r.Truncated || r.Content != "" || r.TotalLines != 0 || r.OutputLines != 0 {
		t.Fatalf("result = %+v", r)
	}
}
