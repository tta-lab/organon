package srcview

import (
	"fmt"
	"strings"

	"github.com/tta-lab/organon/internal/truncate"
)

// OffsetOutOfRangeError reports a one-indexed read offset beyond the selected
// content's addressable lines.
type OffsetOutOfRangeError struct {
	Offset     int
	TotalLines int
}

func (err *OffsetOutOfRangeError) Error() string {
	return fmt.Sprintf("offset %d is beyond end (%d lines)", err.Offset, err.TotalLines)
}

// ReadWindow is the complete Pi-equivalent read window for selected source
// content. TotalLines uses addressable strings.Split lines;
// TruncationTotalLines and OutputLines use Pi truncateHead's counted-line
// model, which omits a terminal empty segment after a newline.
type ReadWindow struct {
	Content               string
	StartLine             int
	TotalLines            int
	TruncationTotalLines  int
	TotalBytes            int
	Truncated             bool
	TruncatedBy           string
	OutputLines           int
	OutputBytes           int
	OutputEndLine         int
	RemainingLines        int
	NextOffset            int
	FirstLineExceedsLimit bool
}

// NewReadWindow applies one-indexed pagination and Pi's standard head
// truncation to content. An offset of zero is the caller's omitted-offset
// sentinel; callers validate explicit offsets before invoking this function.
func NewReadWindow(content string, offset, limit int) (ReadWindow, error) {
	lines := strings.Split(content, "\n")
	startIndex := 0
	if offset > 0 {
		startIndex = offset - 1
	}
	if startIndex >= len(lines) {
		return ReadWindow{}, &OffsetOutOfRangeError{Offset: offset, TotalLines: len(lines)}
	}

	selected := lines[startIndex:]
	userLimited := false
	if limit > 0 && limit < len(selected) {
		selected = selected[:limit]
		userLimited = true
	}
	selectedContent := strings.Join(selected, "\n")
	truncated := truncate.Head(selectedContent, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
	window := ReadWindow{
		Content:               truncated.Content,
		StartLine:             startIndex + 1,
		TotalLines:            len(lines),
		TruncationTotalLines:  truncated.TotalLines,
		TotalBytes:            len(content),
		Truncated:             truncated.Truncated,
		TruncatedBy:           truncated.TruncatedBy,
		OutputLines:           truncated.OutputLines,
		OutputBytes:           truncated.OutputBytes,
		FirstLineExceedsLimit: truncated.FirstLineExceedsLimit,
	}

	if truncated.Truncated {
		window.OutputEndLine = window.StartLine + truncated.OutputLines - 1
	} else {
		window.OutputEndLine = window.StartLine + len(selected) - 1
	}
	if !truncated.FirstLineExceedsLimit && (truncated.Truncated || userLimited) {
		window.NextOffset = window.OutputEndLine + 1
		window.RemainingLines = window.TotalLines - window.OutputEndLine
	}
	return window, nil
}
