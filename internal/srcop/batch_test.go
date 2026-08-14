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
	wantDiff := " 1 a\n-2 bbb\n+2 BB\n 3 c\n-4 ddd\n+4 DD\n 5 e"
	if result.Diff != wantDiff {
		t.Fatalf("display diff = %q, want %q", result.Diff, wantDiff)
	}
	if strings.Contains(result.Diff, "--- ") ||
		strings.Contains(result.Diff, "+++ ") ||
		strings.Contains(result.Diff, "@@") {
		t.Fatalf("display diff contains unified patch headers: %q", result.Diff)
	}
	if !strings.Contains(result.Patch, "--- a/f.txt") || !strings.Contains(result.Patch, "+++ b/f.txt") ||
		!strings.Contains(result.Patch, "@@") || !strings.Contains(result.Patch, "-bbb") ||
		!strings.Contains(result.Patch, "+BB") {
		t.Fatalf("unified patch missing headers or edits: %q", result.Patch)
	}
	if result.Diff == result.Patch {
		t.Fatal("display diff and unified patch must use distinct formats")
	}
}

func TestApplyBatchUsesPiDisplayDiffContext(t *testing.T) {
	source := []byte("one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven\ntwelve\n")
	result, err := ApplyBatch("f.txt", source, []BatchEdit{{OldText: "one", NewText: "ONE"}})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	want := "- 1 one\n+ 1 ONE\n  2 two\n  3 three\n  4 four\n  5 five\n    ..."
	if result.Diff != want {
		t.Fatalf("display diff = %q, want %q", result.Diff, want)
	}
}

func TestApplyBatchUsesPiRepeatedLineAlignment(t *testing.T) {
	source := []byte("b\na\nc\nb\n")
	replacement := "a\na\na\na\nb\nc\nb\na\n"
	result, err := ApplyBatch("f.txt", source, []BatchEdit{{OldText: string(source), NewText: replacement}})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	want := "-1 b\n 2 a\n+2 a\n+3 a\n+4 a\n+5 b\n 3 c\n 4 b\n+8 a"
	if result.Diff != want {
		t.Fatalf("display diff = %q, want %q", result.Diff, want)
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

func TestApplyBatchRejectsLineEndingNormalizedNoops(t *testing.T) {
	cases := []struct {
		name   string
		source string
		edit   BatchEdit
	}{
		{
			name:   "CRLF",
			source: "a\r\nb\r\n",
			edit:   BatchEdit{OldText: "a\r\n", NewText: "a\n"},
		},
		{
			name:   "lone CR",
			source: "a\rb\r",
			edit:   BatchEdit{OldText: "a\rb", NewText: "a\nb"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)
			before := append([]byte(nil), source...)
			result, err := ApplyBatch("f.txt", source, []BatchEdit{tc.edit})
			if err == nil || !strings.Contains(err.Error(), "no-op") {
				t.Fatalf("error = %v, want normalized no-op rejection", err)
			}
			if result != nil {
				t.Fatalf("result = %#v, want nil", result)
			}
			if !bytes.Equal(source, before) {
				t.Fatal("source mutated by rejected normalized no-op")
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
	// Real CRLF source: a CRLF newText must not become double-CRLF after the
	// line-ending restore.
	source := []byte("a\r\nb\r\nc\r\n")
	result, err := ApplyBatch("f.txt", source, []BatchEdit{
		{OldText: "a", NewText: "A\r\nB"},
	})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	if string(result.Content) != "A\r\nB\r\nb\r\nc\r\n" {
		t.Fatalf("content = %q, want single CRLF separators", result.Content)
	}
}

func TestApplyBatchMixedLineEndingsFollowFirstStyle(t *testing.T) {
	// First line ending is LF but the file later contains CRLF: the Pi built-in
	// edit restores the whole file to the first style (LF).
	source := []byte("a\nb\r\nc\n")
	result, err := ApplyBatch("f.txt", source, []BatchEdit{
		{OldText: "a", NewText: "A"},
	})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	want := "A\nb\nc\n"
	if string(result.Content) != want {
		t.Fatalf("content = %q, want %q (LF first style wins)", result.Content, want)
	}

	// First line ending is CRLF with a later lone LF: CRLF wins.
	source2 := []byte("a\r\nb\nc\r\n")
	result2, err := ApplyBatch("f.txt", source2, []BatchEdit{
		{OldText: "a", NewText: "A"},
	})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	want2 := "A\r\nb\r\nc\r\n"
	if string(result2.Content) != want2 {
		t.Fatalf("content = %q, want %q (CRLF first style wins)", result2.Content, want2)
	}
}
