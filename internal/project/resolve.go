package project

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tta-lab/organon/internal/gitprovider"
)

var ErrAmbiguous = errors.New("ambiguous project reference")

// ResolutionError reports why an existing-project reference could not be
// resolved. Candidates are exact matches; Suggestions are active discovery
// results for a not-found reference.
type ResolutionError struct {
	Reference   string
	Candidates  []Entry
	Suggestions []Entry
}

func (e *ResolutionError) Error() string {
	if len(e.Candidates) > 0 {
		return fmt.Sprintf(
			"ambiguous project reference %q: matches %s; retry with a canonical alias",
			e.Reference, formatEntries(e.Candidates),
		)
	}
	message := fmt.Sprintf("project %q not found", e.Reference)
	if len(e.Suggestions) > 0 {
		message += fmt.Sprintf("; plausible active projects: %s", formatEntries(e.Suggestions))
	}
	return message + "; use project find or project list"
}

func (e *ResolutionError) Unwrap() error {
	if len(e.Candidates) > 0 {
		return ErrAmbiguous
	}
	return ErrNotFound
}

func formatEntries(entries []Entry) string {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		label := entry.Alias
		if entry.Archived {
			label += " [archived]"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, ", ")
}

// Resolve returns one project for a configured alias, checkout basename, or
// remote repository basename. Matching is case-insensitive and aliases have
// precedence over all alternate identities.
func (c *Catalog) Resolve(reference string) (Entry, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Entry{}, fmt.Errorf("project reference must not be blank")
	}
	entries := c.ListAll(true)

	aliasMatches := matchingEntries(entries, func(entry Entry) bool {
		return strings.EqualFold(entry.Alias, reference)
	})
	if len(aliasMatches) == 1 {
		return aliasMatches[0], nil
	}
	if len(aliasMatches) > 1 {
		return Entry{}, &ResolutionError{Reference: reference, Candidates: aliasMatches}
	}

	identityMatches := matchingEntries(entries, func(entry Entry) bool {
		return strings.EqualFold(filepath.Base(entry.Path), reference) ||
			strings.EqualFold(remoteBasename(entry.Remote), reference)
	})
	if len(identityMatches) == 1 {
		return identityMatches[0], nil
	}
	if len(identityMatches) > 1 {
		return Entry{}, &ResolutionError{Reference: reference, Candidates: identityMatches}
	}
	suggestions, findErr := c.Find(reference, 3)
	if findErr != nil {
		suggestions = nil
	}
	return Entry{}, &ResolutionError{Reference: reference, Suggestions: suggestions}
}

func matchingEntries(entries []Entry, matches func(Entry) bool) []Entry {
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if matches(entry) {
			result = append(result, entry)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Alias < result[j].Alias })
	return result
}

func remoteBasename(remote string) string {
	info, err := gitprovider.ParseHTTPRemoteURL(remote)
	if err != nil {
		return ""
	}
	return info.Repo
}
