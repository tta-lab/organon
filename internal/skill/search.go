package skill

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	DefaultSearchLimit = 8
	MaxSearchLimit     = 32
)

type skillSearchRank struct {
	skill              Skill
	exactName          bool
	namePhrase         bool
	matchedTokens      int
	nameMatches        int
	categoryMatches    int
	descriptionMatches int
}

// SearchSkills validates a natural-language query and returns ranked skills
// that match at least one query token.
func SearchSkills(skills []Skill, query string, limit int) ([]Skill, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query must not be blank")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than zero")
	}
	limit = min(limit, MaxSearchLimit)
	queryTokens := skillSearchTokens(query)
	querySet := tokenSet(queryTokens)
	queryName := strings.Join(queryTokens, " ")
	ranked := make([]skillSearchRank, 0, len(skills))
	for _, candidate := range skills {
		rank := rankSkillSearch(candidate, queryTokens, querySet, queryName)
		if rank.matchedTokens > 0 {
			ranked = append(ranked, rank)
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		return skillSearchLess(ranked[i], ranked[j])
	})
	limit = min(limit, len(ranked))
	result := make([]Skill, 0, limit)
	for _, rank := range ranked[:limit] {
		result = append(result, rank.skill)
	}
	return result, nil
}

func rankSkillSearch(
	candidate Skill, queryTokens []string, querySet map[string]struct{}, queryName string,
) skillSearchRank {
	nameTokens := skillSearchTokens(candidate.Name)
	nameSet := tokenSet(nameTokens)
	if len(nameTokens) > 1 {
		nameSet[strings.Join(nameTokens, "")] = struct{}{}
	}
	descriptionSet := tokenSet(skillSearchTokens(candidate.Description))
	categorySet := tokenSet(skillSearchTokens(candidate.Category))
	rank := skillSearchRank{
		skill:      candidate,
		exactName:  strings.Join(nameTokens, " ") == queryName,
		namePhrase: containsTokenSequence(nameTokens, queryTokens),
	}
	for token := range querySet {
		_, inName := nameSet[token]
		_, inDescription := descriptionSet[token]
		_, inCategory := categorySet[token]
		if inName || inDescription || inCategory {
			rank.matchedTokens++
		}
		if inName {
			rank.nameMatches++
		}
		if inCategory {
			rank.categoryMatches++
		}
		if inDescription {
			rank.descriptionMatches++
		}
	}
	return rank
}

func skillSearchLess(left, right skillSearchRank) bool {
	if left.exactName != right.exactName {
		return left.exactName
	}
	if left.namePhrase != right.namePhrase {
		return left.namePhrase
	}
	if left.matchedTokens != right.matchedTokens {
		return left.matchedTokens > right.matchedTokens
	}
	if left.nameMatches != right.nameMatches {
		return left.nameMatches > right.nameMatches
	}
	if left.categoryMatches != right.categoryMatches {
		return left.categoryMatches > right.categoryMatches
	}
	if left.descriptionMatches != right.descriptionMatches {
		return left.descriptionMatches > right.descriptionMatches
	}
	if len(left.skill.Name) != len(right.skill.Name) {
		return len(left.skill.Name) < len(right.skill.Name)
	}
	return left.skill.Name < right.skill.Name
}

func skillSearchTokens(value string) []string {
	runes := []rune(value)
	tokens := make([]string, 0, 8)
	var token strings.Builder
	flush := func() {
		if token.Len() == 0 {
			return
		}
		tokens = append(tokens, token.String())
		token.Reset()
	}
	for i, current := range runes {
		if !unicode.IsLetter(current) && !unicode.IsNumber(current) {
			flush()
			continue
		}
		startsWord := token.Len() > 0 && unicode.IsUpper(current) &&
			(i > 0 && unicode.IsLower(runes[i-1]) || i+1 < len(runes) && unicode.IsLower(runes[i+1]))
		if startsWord {
			flush()
		}
		token.WriteRune(unicode.ToLower(current))
	}
	flush()
	return tokens
}

func tokenSet(tokens []string) map[string]struct{} {
	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		set[token] = struct{}{}
	}
	return set
}

func containsTokenSequence(tokens, sequence []string) bool {
	if len(sequence) == 0 || len(sequence) > len(tokens) {
		return false
	}
	for start := 0; start <= len(tokens)-len(sequence); start++ {
		matches := true
		for offset := range sequence {
			if tokens[start+offset] != sequence[offset] {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}
