package srcop

import (
	"bytes"
	"strings"
	"testing"
)

func TestApplyBatchMultipleDisjointReplacements(t *testing.T) {
	source := []byte("a\nbbb\nc\nddd\ne\n")
	result, err := ApplyBatch("f.txt", source, []BatchEdit{
		{OldText: "bbb", NewText: "BB"},
		{OldText: "ddd", NewText: "DD"},
	})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	want := "a\nBB\nc\nDD\ne\n"
	if string(result.Content) != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
	if result.FirstChangedLine != 2 {
		t.Fatalf("first changed line = %d, want 2", result.FirstChangedLine)
	}
	if !strings.Contains(result.Diff, "-bbb") || !strings.Contains(result.Diff, "+BB") {
		t.Fatalf("diff missing edits: %q", result.Diff)
	}
	if result.Patch == "" {
		t.Fatal("patch is empty")
	}
}

func TestApplyBatchMatchesAllAgainstOriginal(t *testing.T) {
	// Edit 2's newText equals edit 1's oldText; both still match the original
	// independently, never content produced by preceding entries.
	source := []byte("x\nfoo\nbar\nbaz\n")
	result, err := ApplyBatch("f.txt", source, []BatchEdit{
		{OldText: "foo", NewText: "FOO"},
		{OldText: "bar", NewText: "foo"},
	})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	want := "x\nFOO\nfoo\nbaz\n"
	if string(result.Content) != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
}

func TestApplyBatchEmptyEditsRejected(t *testing.T) {
	_, err := ApplyBatch("f.txt", []byte("abc\n"), nil)
	if err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("empty edits error = %v", err)
	}
	_, err = ApplyBatch("f.txt", []byte("abc\n"), []BatchEdit{{OldText: "", NewText: "x"}})
	if err == nil || !strings.Contains(err.Error(), "oldText is empty") {
		t.Fatalf("empty oldText error = %v", err)
	}
}

func TestApplyBatchMissingDuplicateOverlapNestedNoopRejected(t *testing.T) {
	source := []byte("line1\nline2\nline3\nline2\n")
	cases := []struct {
		name  string
		edits []BatchEdit
		want  string
	}{
		{"missing", []BatchEdit{{OldText: "nope", NewText: "x"}}, "text not found"},
		{"duplicate", []BatchEdit{{OldText: "line2", NewText: "x"}}, "2 matches"},
		{"overlap", []BatchEdit{
			{OldText: "line1\nline2", NewText: "A"},
			{OldText: "line2\nline3", NewText: "B"},
		}, "overlap"},
		{"nested", []BatchEdit{
			{OldText: "line2\nline3", NewText: "N"},
			{OldText: "line3", NewText: "M"},
		}, "overlap"},
		{"noop", []BatchEdit{{OldText: "line1", NewText: "line1"}}, "identical"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ApplyBatch("f.txt", source, tc.edits); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s: error = %v, want contains %q", tc.name, err, tc.want)
			}
		})
	}
}

func TestApplyBatchValidationFailureLeavesSourceUntouched(t *testing.T) {
	source := []byte("a\nbbb\nc\n")
	if _, err := ApplyBatch("f.txt", source, []BatchEdit{
		{OldText: "bbb", NewText: "BB"},
		{OldText: "c", NewText: "c"}, // no-op after a valid edit
	}); err == nil {
		t.Fatal("expected error")
	}
	if !bytes.Equal(source, []byte("a\nbbb\nc\n")) {
		t.Fatal("source mutated by failed batch")
	}
}

func TestApplyBatchPreservesBOMAndCRLF(t *testing.T) {
	source := []byte("\xef\xbb\xbfline1\r\nline2\r\nline3\r\n")
	result, err := ApplyBatch("f.txt", source, []BatchEdit{
		{OldText: "line2", NewText: "TWO"},
	})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	if !bytes.HasPrefix(result.Content, []byte("\xef\xbb\xbf")) {
		t.Fatal("BOM not preserved")
	}
	want := "\xef\xbb\xbfline1\r\nTWO\r\nline3\r\n"
	if string(result.Content) != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
	if result.FirstChangedLine != 2 {
		t.Fatalf("first changed line = %d, want 2", result.FirstChangedLine)
	}
}

func TestApplyBatchCRLFMatchesLFNormalizedOldText(t *testing.T) {
	source := []byte("a\r\nb\r\nc\r\n")
	result, err := ApplyBatch("f.txt", source, []BatchEdit{
		{OldText: "a\nb", NewText: "A\nB"},
	})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	if string(result.Content) != "A\r\nB\r\nc\r\n" {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestApplyBatchLargeFileNoCap(t *testing.T) {
	// Batch edits must not hit the single-edit 100-KB cap.
	big := bytes.Repeat([]byte("x"), 150*1024)
	source := append(append([]byte("head "), big...), []byte(" tail\n")...)
	result, err := ApplyBatch("big.txt", source, []BatchEdit{
		{OldText: "head", NewText: "HEAD"},
	})
	if err != nil {
		t.Fatalf("ApplyBatch on large file: %v", err)
	}
	if !bytes.HasPrefix(result.Content, []byte("HEAD ")) || !bytes.HasSuffix(result.Content, []byte(" tail\n")) {
		t.Fatalf("large content not preserved correctly")
	}
}

func TestApplyBatchFirstChangedLineAcrossEdits(t *testing.T) {
	source := []byte("one\ntwo\nthree\nfour\n")
	result, err := ApplyBatch("f.txt", source, []BatchEdit{
		{OldText: "three", NewText: "THREE"},
		{OldText: "one", NewText: "ONE"},
	})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	// Edits are applied in file order; the first change is line 1.
	if result.FirstChangedLine != 1 {
		t.Fatalf("first changed line = %d, want 1", result.FirstChangedLine)
	}
	if string(result.Content) != "ONE\ntwo\nTHREE\nfour\n" {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestApplyBatchRejectsOverlappingOccurrences(t *testing.T) {
	// "aa" occurs at offsets 0 and 1 of "aaa"; the oldText is ambiguous.
	source := []byte("aaa\\n")
	if _, err := ApplyBatch("f.txt", source, []BatchEdit{{OldText: "aa", NewText: "X"}}); err == nil ||
		!strings.Contains(err.Error(), "2 matches") {
		t.Fatalf("error = %v, want ambiguous overlap rejection", err)
	}
}

func TestApplyBatchCRLFNewTextIsNormalized(t *testing.T) {
	// A CRLF newText must not become double-CRLF after the line-ending restore.
	source := []byte("a\\r\\nb\\r\\nc\\r\\n")
	result, err := ApplyBatch("f.txt", source, []BatchEdit{
		{OldText: "a", NewText: "A\\r\\nB"},
	})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	if string(result.Content) != "A\\r\\nB\\r\\nb\\r\\nc\\r\\n" {
		t.Fatalf("content = %q, want normalized CRLF (no \r\r\n)", result.Content)
	}
}
