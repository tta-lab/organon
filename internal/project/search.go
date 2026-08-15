package project

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tta-lab/organon/internal/textmatch"
)

const (
	DefaultFindLimit = 8
	MaxFindLimit     = 32
)

type projectSearchRank struct {
	entry         Entry
	kind          int
	fieldPriority int
	matchedTokens int
	distance      int
}

// Find returns active projects ranked for a natural-language query.
func (c *Catalog) Find(query string, limit int) ([]Entry, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query must not be blank")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than zero")
	}
	limit = min(limit, MaxFindLimit)
	queryNormalized := textmatch.Normalize(query)
	queryTokens := textmatch.Tokens(query)
	if len(queryTokens) == 0 {
		return []Entry{}, nil
	}

	ranked := make([]projectSearchRank, 0, len(c.activeEntries))
	for _, entry := range c.activeEntries {
		if rank, ok := rankProject(entry, queryNormalized, queryTokens); ok {
			ranked = append(ranked, rank)
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		return projectSearchLess(ranked[i], ranked[j])
	})
	if limit > len(ranked) {
		limit = len(ranked)
	}
	result := make([]Entry, 0, limit)
	for _, rank := range ranked[:limit] {
		result = append(result, rank.entry)
	}
	return result, nil
}

func rankProject(entry Entry, query string, queryTokens []string) (projectSearchRank, bool) {
	best := projectSearchRank{entry: entry}
	for _, field := range projectSearchFields(entry) {
		kind, matchedTokens, distance, ok := rankProjectField(field.value, query, queryTokens)
		if !ok {
			continue
		}
		candidate := projectSearchRank{
			entry:         entry,
			kind:          kind,
			fieldPriority: field.priority,
			matchedTokens: matchedTokens,
			distance:      distance,
		}
		if best.kind == 0 || projectSearchLess(candidate, best) {
			best = candidate
		}
	}
	return best, best.kind != 0
}

type projectSearchField struct {
	value    string
	priority int
}

func projectSearchFields(entry Entry) []projectSearchField {
	return []projectSearchField{
		{value: entry.Alias, priority: 4},
		{value: entry.Name, priority: 3},
		{value: filepath.Base(entry.Path), priority: 2},
		{value: remoteBasename(entry.Remote), priority: 1},
	}
}

// Match kinds descend from exact field identity through advisory spelling
// proximity. Project ranking intentionally stays local to this domain.
func rankProjectField(value, query string, queryTokens []string) (kind, matchedTokens, distance int, ok bool) {
	field := strings.TrimSpace(textmatch.Normalize(value))
	if field == "" {
		return 0, 0, 0, false
	}
	fieldTokens := textmatch.Tokens(value)
	fieldSet := make(map[string]struct{}, len(fieldTokens))
	for _, token := range fieldTokens {
		fieldSet[token] = struct{}{}
	}
	for _, token := range queryTokens {
		if _, exists := fieldSet[token]; exists {
			matchedTokens++
		}
	}

	switch {
	case field == query:
		return 6, matchedTokens, 0, true
	case strings.HasPrefix(field, query) || tokenPrefix(fieldTokens, queryTokens):
		return 5, matchedTokens, 0, true
	case textmatch.ContainsTokenSequence(fieldTokens, queryTokens):
		return 4, matchedTokens, 0, true
	case matchedTokens == len(queryTokens):
		return 3, matchedTokens, 0, true
	case strings.Contains(field, query):
		return 2, matchedTokens, 0, true
	}

	distance = projectSearchDistance(field, query)
	if distance <= projectSearchDistanceLimit(query) {
		return 1, matchedTokens, distance, true
	}
	return 0, 0, 0, false
}

func tokenPrefix(fieldTokens, queryTokens []string) bool {
	if len(queryTokens) == 0 || len(queryTokens) > len(fieldTokens) {
		return false
	}
	for i, queryToken := range queryTokens {
		if !strings.HasPrefix(fieldTokens[i], queryToken) {
			return false
		}
	}
	return true
}

func projectSearchLess(left, right projectSearchRank) bool {
	if left.kind != right.kind {
		return left.kind > right.kind
	}
	if left.fieldPriority != right.fieldPriority {
		return left.fieldPriority > right.fieldPriority
	}
	if left.matchedTokens != right.matchedTokens {
		return left.matchedTokens > right.matchedTokens
	}
	if left.distance != right.distance {
		return left.distance < right.distance
	}
	return left.entry.Alias < right.entry.Alias
}

func projectSearchDistance(left, right string) int {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	if len(leftRunes) == 0 {
		return len(rightRunes)
	}
	if len(rightRunes) == 0 {
		return len(leftRunes)
	}
	previous := make([]int, len(rightRunes)+1)
	current := make([]int, len(rightRunes)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, leftRune := range leftRunes {
		current[0] = i + 1
		for j, rightRune := range rightRunes {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			current[j+1] = min(current[j]+1, min(previous[j+1]+1, previous[j]+cost))
		}
		previous, current = current, previous
	}
	return previous[len(rightRunes)]
}

func projectSearchDistanceLimit(query string) int {
	length := len([]rune(query))
	limit := length / 3
	if limit < 1 {
		return 1
	}
	return limit
}
