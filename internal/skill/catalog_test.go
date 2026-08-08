package skill

import "testing"

func TestCatalogDeduplicatesGroupsByPriority(t *testing.T) {
	project := Skill{Name: "shared", Description: "project"}
	global := Skill{Name: "shared", Description: "global"}
	other := Skill{Name: "other"}

	catalog := NewCatalog([]Skill{project}, []Skill{global, other})
	got := catalog.List()

	if len(got) != 2 || got[0].Name != "other" || got[1].Description != "project" {
		t.Fatalf("List() = %#v", got)
	}
}

func TestCatalogGetNormalizesExactName(t *testing.T) {
	catalog := NewCatalog([]Skill{{Name: "shared", Body: "body"}})

	got, err := catalog.Get(" shared ")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "shared" || got.Body != "body" {
		t.Fatalf("Get() = %#v", got)
	}
}

func TestCatalogFindUsesRankedSearch(t *testing.T) {
	catalog := NewCatalog([]Skill{
		{Name: "plan-triage", Description: "Triage implementation plans"},
		{Name: "pr-review-loop", Description: "Review pull requests in a repeated loop"},
		{Name: "review-notes", Description: "Review collected notes"},
	})

	got, err := catalog.Find("review loop triage", 2)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(got) != 2 || got[0].Name != "pr-review-loop" || got[1].Name != "plan-triage" {
		t.Fatalf("Find() = %#v", got)
	}
}
