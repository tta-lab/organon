package truncate

import "strings"

// Default limits mirror the Pi coding agent's truncateHead defaults.
const (
	DefaultMaxLines = 2000
	DefaultMaxBytes = 50 * 1024
)

// Result describes a head truncation applied to text.
type Result struct {
	Content               string
	Truncated             bool
	TruncatedBy           string // "lines", "bytes", or ""
	TotalLines            int
	TotalBytes            int
	OutputLines           int
	OutputBytes           int
	FirstLineExceedsLimit bool
	MaxLines              int
	MaxBytes              int
}

// splitLinesForCounting mirrors Pi's truncateHead line model: empty content has
// zero lines, and a terminal empty segment after a newline is not counted.
func splitLinesForCounting(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		return lines[:len(lines)-1]
	}
	return lines
}

// Head applies Pi-equivalent head truncation: keep the first lines under both
// the line and byte limits, never returning partial lines. If the first line
// alone exceeds the byte limit, content is empty and FirstLineExceedsLimit is
// set, matching Pi's truncateHead behavior.
func Head(content string, maxLines, maxBytes int) Result {
	totalBytes := len(content)
	lines := splitLinesForCounting(content)
	totalLines := len(lines)

	if totalLines <= maxLines && totalBytes <= maxBytes {
		return Result{
			Content: content, TotalLines: totalLines, TotalBytes: totalBytes,
			OutputLines: totalLines, OutputBytes: totalBytes,
			MaxLines: maxLines, MaxBytes: maxBytes,
		}
	}

	if len(lines[0]) > maxBytes {
		return Result{
			Truncated: true, TruncatedBy: "bytes",
			TotalLines: totalLines, TotalBytes: totalBytes,
			FirstLineExceedsLimit: true,
			MaxLines:              maxLines, MaxBytes: maxBytes,
		}
	}

	output := make([]string, 0, min(maxLines, totalLines))
	outputBytes := 0
	truncatedBy := "lines"
	for i := 0; i < len(lines) && i < maxLines; i++ {
		lineBytes := len(lines[i])
		if i > 0 {
			lineBytes++ // +1 for the newline separating this line
		}
		if outputBytes+lineBytes > maxBytes {
			truncatedBy = "bytes"
			break
		}
		output = append(output, lines[i])
		outputBytes += lineBytes
	}
	content = strings.Join(output, "\n")
	return Result{
		Content: content, Truncated: true, TruncatedBy: truncatedBy,
		TotalLines: totalLines, TotalBytes: totalBytes,
		OutputLines: len(output), OutputBytes: len(content),
		MaxLines: maxLines, MaxBytes: maxBytes,
	}
}
