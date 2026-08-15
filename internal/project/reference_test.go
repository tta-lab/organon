package project

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func aliases(entries []Entry) []string {
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Alias)
	}
	return result
}

func TestCatalogResolveMatchesAlternateIdentitiesCaseInsensitively(t *testing.T) {
	path := writeProjectFile(t, `[fb]
name = "FlickNote Backend"
path = "/projects/flick-backend"
remote = "https://example.com/owner/flick-backend.git"
`)
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}

	for _, reference := range []string{"fb", "FB", "flick-backend", "FLICK-BACKEND"} {
		entry, err := catalog.Resolve(reference)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", reference, err)
		}
		if entry.Alias != "fb" || entry.Path != "/projects/flick-backend" {
			t.Fatalf("Resolve(%q) = %+v, want canonical fb at checkout path", reference, entry)
		}
	}
}

func TestCatalogResolveAliasPrecedenceAndArchivedEntries(t *testing.T) {
	path := writeProjectFile(t, `[same]
path = "/projects/alias-project"
remote = "https://example.com/owner/alias-project.git"

[archived.old]
name = "Old Backend"
path = "/projects/old-backend"
remote = "https://example.com/owner/old-backend.git"
`)
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}

	entry, err := catalog.Resolve("old-backend")
	if err != nil {
		t.Fatalf("Resolve archived checkout: %v", err)
	}
	if entry.Alias != "old" || !entry.Archived {
		t.Fatalf("archived checkout resolved to %+v", entry)
	}

	aliasPath := writeProjectFile(t, `[old-backend]
path = "/projects/another"
remote = "https://example.com/owner/another.git"

[archived.same]
path = "/projects/same"
remote = "https://example.com/owner/same.git"
`)
	catalog, err = OpenCatalog(aliasPath)
	if err != nil {
		t.Fatalf("OpenCatalog alias precedence: %v", err)
	}
	entry, err = catalog.Resolve("OLD-BACKEND")
	if err != nil {
		t.Fatalf("Resolve alias precedence: %v", err)
	}
	if entry.Alias != "old-backend" || entry.Archived {
		t.Fatalf("alias did not take precedence: %+v", entry)
	}
}

func TestCatalogResolveReportsAmbiguousExactCandidates(t *testing.T) {
	path := writeProjectFile(t, `[Foo]
path = "/projects/foo-one"
remote = "https://example.com/one/foo-one.git"

[foo]
path = "/projects/foo-two"
remote = "https://example.com/two/foo-two.git"

[one]
path = "/projects/one/repo"
remote = "https://example.com/one/repo.git"

[two]
path = "/projects/two/repo"
remote = "https://example.com/two/repo.git"
`)
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}

	for _, test := range []struct {
		reference string
		want      []string
	}{
		{reference: "fOo", want: []string{"Foo", "foo"}},
		{reference: "REPO", want: []string{"one", "two"}},
	} {
		t.Run(test.reference, func(t *testing.T) {
			_, err := catalog.Resolve(test.reference)
			if !errors.Is(err, ErrAmbiguous) {
				t.Fatalf("Resolve(%q) error = %v, want ErrAmbiguous", test.reference, err)
			}
			var resolutionErr *ResolutionError
			if !errors.As(err, &resolutionErr) {
				t.Fatalf("Resolve(%q) error = %T, want ResolutionError", test.reference, err)
			}
			if got := aliases(resolutionErr.Candidates); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("candidates = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCatalogResolveDoesNotUseDisplayNamesOrPathsAsIdentities(t *testing.T) {
	path := writeProjectFile(t, `[fb]
name = "FlickNote Backend"
path = "/projects/flick-backend"
remote = "https://example.com/owner/flick-backend.git"
`)
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}
	for _, reference := range []string{
		"FlickNote Backend",
		"/projects/flick-backend",
		"file:///projects/flick-backend",
		"https://example.com/owner/flick-backend.git",
		"owner/flick-backend",
	} {
		t.Run(reference, func(t *testing.T) {
			_, err := catalog.Resolve(reference)
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("Resolve(%q) error = %v, want ErrNotFound", reference, err)
			}
		})
	}
}

func TestCatalogFindRanksProjectIdentitiesAndRecoversTypos(t *testing.T) {
	path := writeProjectFile(t, `[fb]
name = "FlickNote Backend"
path = "/projects/flick-backend"
remote = "https://example.com/owner/flick-backend.git"

[backend]
name = "Backend Worker"
path = "/projects/worker"
remote = "https://example.com/owner/worker.git"

[notes]
name = "FlickNote"
path = "/projects/notes"
remote = "https://example.com/owner/notes.git"
`)
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}

	tests := []struct {
		query string
		want  []string
	}{
		{query: "backend", want: []string{"backend", "fb"}},
		{query: "FlickNote Backend", want: []string{"fb"}},
		{query: "flick-backnd", want: []string{"fb"}},
	}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			got, err := catalog.Find(test.query, 8)
			if err != nil {
				t.Fatalf("Find(%q): %v", test.query, err)
			}
			if aliases(got)[:len(test.want)] == nil || !reflect.DeepEqual(aliases(got)[:len(test.want)], test.want) {
				t.Fatalf("Find(%q) = %v, want prefix %v", test.query, aliases(got), test.want)
			}
		})
	}
}

func TestCatalogFindValidatesQueryAndLimitAndReturnsEmptyNoMatch(t *testing.T) {
	catalog, err := OpenCatalog(writeProjectFile(t, `[one]
path = "/projects/one"
remote = "https://example.com/owner/one.git"
`))
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}
	for _, query := range []string{"", "   "} {
		if _, err := catalog.Find(query, 8); err == nil || !strings.Contains(err.Error(), "query must not be blank") {
			t.Fatalf("Find(%q) error = %v, want blank-query error", query, err)
		}
	}
	if _, err := catalog.Find("one", 0); err == nil || !strings.Contains(err.Error(), "limit must be greater than zero") {
		t.Fatalf("Find zero limit error = %v", err)
	}
	got, err := catalog.Find("unrelated", 8)
	if err != nil {
		t.Fatalf("Find unrelated: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Find unrelated = %v, want empty", aliases(got))
	}
}

func TestCatalogResolveIncludesRankedActiveSuggestionsOnlyWhenNotFound(t *testing.T) {
	catalog, err := OpenCatalog(writeProjectFile(t, `[fb]
name = "FlickNote Backend"
path = "/projects/flick-backend"
remote = "https://example.com/owner/flick-backend.git"

[archived.old]
name = "FlickNote Backend old"
path = "/projects/flick-backend-old"
remote = "https://example.com/owner/flick-backend-old.git"
`))
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}
	_, err = catalog.Resolve("flick-backnd")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Resolve error = %v, want ErrNotFound", err)
	}
	var resolutionErr *ResolutionError
	if !errors.As(err, &resolutionErr) {
		t.Fatalf("Resolve error = %T, want ResolutionError", err)
	}
	if got := aliases(resolutionErr.Suggestions); !reflect.DeepEqual(got, []string{"fb"}) {
		t.Fatalf("suggestions = %v, want [fb]", got)
	}
	if strings.Contains(err.Error(), "old") ||
		!strings.Contains(err.Error(), "project find") ||
		!strings.Contains(err.Error(), "project list") {
		t.Fatalf("error = %v, want active suggestion and recovery hint", err)
	}
}
