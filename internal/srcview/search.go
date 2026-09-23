package srcview

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const DefaultSearchLimit = 50
const MaxSearchLimit = 200

type SearchMatch struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}
type SearchResult struct {
	Matches   []SearchMatch `json:"matches"`
	Truncated bool          `json:"truncated"`
}

//nolint:gocyclo // Streaming rg events requires distinct process, parsing, and limit outcomes.
func Search(ctx context.Context, root, pattern string, limit int) (SearchResult, error) {
	if strings.TrimSpace(pattern) == "" {
		return SearchResult{}, fmt.Errorf("pattern must not be blank")
	}
	if len(pattern) > 4096 {
		return SearchResult{}, fmt.Errorf("pattern exceeds 4096 bytes")
	}
	if limit < 1 || limit > MaxSearchLimit {
		return SearchResult{}, fmt.Errorf("limit must be between 1 and %d", MaxSearchLimit)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rg", "--no-config", "--json", "--line-buffered",
		"--no-follow", "--max-columns", "4096", "--max-columns-preview", "--", pattern, ".")
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return SearchResult{}, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		return SearchResult{}, fmt.Errorf("start rg: %w", err)
	}
	result := SearchResult{Matches: make([]SearchMatch, 0)}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	stopped := false
	var stopContextErr error
	var stopKillErr error
	for scanner.Scan() {
		var event struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				LineNumber int `json:"line_number"`
				Submatches []struct {
					Start int `json:"start"`
				} `json:"submatches"`
			} `json:"data"`
		}
		if err = json.Unmarshal(scanner.Bytes(), &event); err != nil {
			break
		}
		if event.Type != "match" {
			continue
		}
		if len(result.Matches) == limit {
			result.Truncated = true
			stopped = true
			stopContextErr = ctx.Err()
			stopKillErr = cmd.Process.Kill()
			break
		}
		if event.Data.Path.Text == "" || event.Data.Lines.Text == "" {
			err = fmt.Errorf("rg returned a match without UTF-8 path or text")
			break
		}
		path := strings.TrimPrefix(event.Data.Path.Text, "./")
		col := 1
		if len(event.Data.Submatches) > 0 {
			col = event.Data.Submatches[0].Start + 1
		}
		result.Matches = append(result.Matches, SearchMatch{
			Path: path, Line: event.Data.LineNumber, Column: col,
			Text: strings.TrimSuffix(event.Data.Lines.Text, "\n"),
		})
	}
	scanErr := scanner.Err()
	scanContextErr := ctx.Err()
	if scanErr != nil || err != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if stopped {
		if stopContextErr != nil {
			return SearchResult{}, fmt.Errorf("rg: %w", stopContextErr)
		}
		if waitErr != nil {
			exit, ok := waitErr.(*exec.ExitError)
			if stopKillErr != nil || !ok || exit.ExitCode() != -1 {
				return SearchResult{}, fmt.Errorf("rg: %w: %s", waitErr, stderr.String())
			}
		}
		return result, nil
	}
	if scanErr != nil {
		if scanContextErr != nil {
			return SearchResult{}, fmt.Errorf("rg: %w", scanContextErr)
		}
		return SearchResult{}, fmt.Errorf("read rg output: %w", scanErr)
	}
	if err != nil {
		return SearchResult{}, fmt.Errorf("parse rg output: %w", err)
	}
	if waitErr != nil {
		if e, ok := waitErr.(*exec.ExitError); ok && e.ExitCode() == 1 {
			return result, nil
		}
		if ctx.Err() != nil {
			return SearchResult{}, fmt.Errorf("rg: %w", ctx.Err())
		}
		return SearchResult{}, fmt.Errorf("rg: %w: %s", waitErr, stderr.String())
	}
	return result, nil
}
