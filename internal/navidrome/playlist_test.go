package navidrome

import (
	"errors"
	"testing"
)

func TestDiffTracksReportsAddRemoveAndReorder(t *testing.T) {
	diff := DiffTracks(
		[]Song{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		[]Song{{ID: "b"}, {ID: "a"}, {ID: "d"}},
	)

	if diff.Added != 1 || diff.Removed != 1 || diff.Reordered != 2 {
		t.Fatalf("diff = %+v", diff)
	}
	if !diff.HasChanges() {
		t.Fatal("HasChanges = false")
	}
}

func TestDiffTracksNoChanges(t *testing.T) {
	diff := DiffTracks([]Song{{ID: "a"}, {ID: "b"}}, []Song{{ID: "a"}, {ID: "b"}})
	if diff != (TrackDiff{}) {
		t.Fatalf("diff = %+v", diff)
	}
	if diff.HasChanges() {
		t.Fatal("HasChanges = true")
	}
}

func TestDiffTracksNoOverlap(t *testing.T) {
	diff := DiffTracks([]Song{{ID: "a"}}, []Song{{ID: "b"}})
	if diff.Added != 1 || diff.Removed != 1 || diff.Reordered != 0 {
		t.Fatalf("diff = %+v", diff)
	}
}

func TestChoosePlaylistRefusesDuplicateNamesWithoutPinnedID(t *testing.T) {
	spec := PlaylistSpec{Name: "Night"}
	_, err := ChoosePlaylist(spec, []Playlist{
		{ID: "one", Name: "Night", Owner: "ooneil"},
		{ID: "two", Name: "Night", Owner: "ooneil"},
	}, "ooneil")
	if err == nil {
		t.Fatal("ChoosePlaylist succeeded, want duplicate-name error")
	}
}

func TestChoosePlaylistUsesPinnedID(t *testing.T) {
	spec := PlaylistSpec{Name: "Night", NavidromeID: "two"}
	playlist, err := ChoosePlaylist(spec, []Playlist{
		{ID: "one", Name: "Night", Owner: "ooneil"},
		{ID: "two", Name: "Other", Owner: "ooneil"},
	}, "ooneil")
	if err != nil {
		t.Fatalf("ChoosePlaylist: %v", err)
	}
	if playlist.ID != "two" {
		t.Fatalf("playlist = %+v", playlist)
	}
}

func TestChoosePlaylistFindsSingleNameMatch(t *testing.T) {
	playlist, err := ChoosePlaylist(PlaylistSpec{Name: "Night"}, []Playlist{
		{ID: "one", Name: "Other", Owner: "ooneil"},
		{ID: "two", Name: "Night", Owner: "ooneil"},
	}, "ooneil")
	if err != nil {
		t.Fatalf("ChoosePlaylist: %v", err)
	}
	if playlist.ID != "two" {
		t.Fatalf("playlist = %+v", playlist)
	}
}

func TestChoosePlaylistEmptyOwnerDoesNotFilter(t *testing.T) {
	playlist, err := ChoosePlaylist(PlaylistSpec{Name: "Night"}, []Playlist{
		{ID: "one", Name: "Night", Owner: "someone"},
	}, "")
	if err != nil {
		t.Fatalf("ChoosePlaylist: %v", err)
	}
	if playlist.ID != "one" {
		t.Fatalf("playlist = %+v", playlist)
	}
}

func TestChoosePlaylistReturnsNotFound(t *testing.T) {
	_, err := ChoosePlaylist(PlaylistSpec{Name: "Night"}, []Playlist{{ID: "one", Name: "Other"}}, "")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}
