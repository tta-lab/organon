package sgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// httpClient is injectable for testing. If nil, Search uses a default client.
var httpClient *http.Client

// endpoint is injectable for testing. Defaults to the public Sourcegraph API.
var endpoint = "https://sourcegraph.com/.api/graphql"

type graphqlRequest struct {
	Query     string `json:"query"`
	Variables struct {
		Query string `json:"query"`
	} `json:"variables"`
}

// Search queries Sourcegraph's public GraphQL API and returns formatted
// markdown results. count clamps to [1, 20], contextWindow defaults to 10,
// timeout in seconds (0 = no timeout, max 120).
func Search(ctx context.Context, query string, count, contextWindow, timeout int) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	if count <= 0 {
		count = 10
	} else if count > 20 {
		count = 20
	}

	if contextWindow <= 0 {
		contextWindow = 10
	}

	requestCtx := ctx
	if timeout > 0 {
		maxTimeout := 120
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	var client *http.Client
	if httpClient != nil {
		client = httpClient
	} else {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConns = 100
		transport.MaxIdleConnsPerHost = 10
		transport.IdleConnTimeout = 90 * time.Second
		client = &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		}
	}

	reqBody := graphqlRequest{
		Query: "query Search($query: String!) { search(query: $query, version: V2, patternType: keyword ) { results { matchCount, limitHit, resultCount, approximateResultCount, missing { name }, timedout { name }, indexUnavailable, results { __typename, ... on FileMatch { repository { name }, file { path, url, content }, lineMatches { preview, lineNumber, offsetAndLengths } } } } } }",
	}
	reqBody.Variables.Query = query

	graphqlQueryBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		requestCtx,
		"POST",
		endpoint,
		bytes.NewBuffer(graphqlQueryBytes),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "organon/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		if len(body) > 0 {
			return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]any
	if err = json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return formatSourcegraphResults(result, contextWindow)
}

func formatSourcegraphResults(result map[string]any, contextWindow int) (string, error) {
	var buffer strings.Builder

	if errors, ok := result["errors"].([]any); ok && len(errors) > 0 {
		buffer.WriteString("## Sourcegraph API Error\n\n")
		for _, err := range errors {
			if errMap, ok := err.(map[string]any); ok {
				if message, ok := errMap["message"].(string); ok {
					fmt.Fprintf(&buffer, "- %s\n", message)
				}
			}
		}
		return buffer.String(), nil
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid response format: missing data field")
	}

	search, ok := data["search"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid response format: missing search field")
	}

	searchResults, ok := search["results"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid response format: missing results field")
	}

	matchCount, _ := searchResults["matchCount"].(float64)
	resultCount, _ := searchResults["resultCount"].(float64)
	limitHit, _ := searchResults["limitHit"].(bool)

	buffer.WriteString("# Sourcegraph Search Results\n\n")
	fmt.Fprintf(&buffer, "Found %d matches across %d results\n", int(matchCount), int(resultCount))

	if limitHit {
		buffer.WriteString("(Result limit reached, try a more specific query)\n")
	}

	buffer.WriteString("\n")

	results, ok := searchResults["results"].([]any)
	if !ok || len(results) == 0 {
		buffer.WriteString("No results found. Try a different query.\n")
		return buffer.String(), nil
	}

	maxResults := 10
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	for i, res := range results {
		fileMatch, ok := res.(map[string]any)
		if !ok {
			continue
		}

		typeName, _ := fileMatch["__typename"].(string)
		if typeName != "FileMatch" {
			continue
		}

		repo, _ := fileMatch["repository"].(map[string]any)
		file, _ := fileMatch["file"].(map[string]any)
		lineMatches, _ := fileMatch["lineMatches"].([]any)

		if repo == nil || file == nil {
			continue
		}

		repoName, _ := repo["name"].(string)
		filePath, _ := file["path"].(string)
		fileURL, _ := file["url"].(string)
		fileContent, _ := file["content"].(string)

		fmt.Fprintf(&buffer, "## Result %d: %s/%s\n\n", i+1, repoName, filePath)

		if fileURL != "" {
			fmt.Fprintf(&buffer, "URL: %s\n\n", fileURL)
		}

		if len(lineMatches) > 0 {
			for _, lm := range lineMatches {
				lineMatch, ok := lm.(map[string]any)
				if !ok {
					continue
				}

				lineNumber, _ := lineMatch["lineNumber"].(float64)
				preview, _ := lineMatch["preview"].(string)

				if fileContent != "" {
					lines := strings.Split(fileContent, "\n")

					buffer.WriteString("```\n")

					startLine := max(1, int(lineNumber)-contextWindow)

					for j := startLine - 1; j < int(lineNumber)-1 && j < len(lines); j++ {
						if j >= 0 {
							fmt.Fprintf(&buffer, "%d| %s\n", j+1, lines[j])
						}
					}

					fmt.Fprintf(&buffer, "%d|  %s\n", int(lineNumber), preview)

					endLine := int(lineNumber) + contextWindow

					for j := int(lineNumber); j < endLine && j < len(lines); j++ {
						if j < len(lines) {
							fmt.Fprintf(&buffer, "%d| %s\n", j+1, lines[j])
						}
					}

					buffer.WriteString("```\n\n")
				} else {
					buffer.WriteString("```\n")
					fmt.Fprintf(&buffer, "%d| %s\n", int(lineNumber), preview)
					buffer.WriteString("```\n\n")
				}
			}
		}
	}

	return buffer.String(), nil
}
