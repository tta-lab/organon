package navidrome

import (
	"context"
	"errors"
	"testing"
)

type fakeSearcher map[string][]Song

func (f fakeSearcher) SearchSongs(ctx context.Context, query string) ([]Song, error) {
	return f[query], nil
}

func (f fakeSearcher) GetSong(ctx context.Context, id string) (Song, error) {
	for _, songs := range f {
		for _, song := range songs {
			if song.ID == id {
				return song, nil
			}
		}
	}
	return Song{}, ErrNotFound
}

func TestResolveTracksUsesPinnedIDs(t *testing.T) {
	spec := PlaylistSpec{Tracks: []TrackSpec{{ID: "song-1", Title: "Old", Artist: "Name"}}}
	result, err := ResolveTracks(context.Background(), fakeSearcher{
		"unused": {{ID: "song-1", Title: "New", Artist: "Name"}},
	}, spec, false)
	if err != nil {
		t.Fatalf("ResolveTracks: %v", err)
	}
	if len(result.Songs) != 1 || result.Songs[0].ID != "song-1" {
		t.Fatalf("resolved songs = %+v", result.Songs)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected metadata mismatch warning for pinned id")
	}
}

func TestResolveTracksExactMatch(t *testing.T) {
	spec := PlaylistSpec{Tracks: []TrackSpec{{Title: "Small Half", Artist: "Chen Li", Album: "Album"}}}
	result, err := ResolveTracks(context.Background(), fakeSearcher{
		"Small Half Chen Li Album": {
			{ID: "live", Title: "Small Half Live", Artist: "Chen Li", Album: "Album"},
			{ID: "exact", Title: "Small Half", Artist: "Chen Li", Album: "Album"},
		},
	}, spec, false)
	if err != nil {
		t.Fatalf("ResolveTracks: %v", err)
	}
	if len(result.Songs) != 1 || result.Songs[0].ID != "exact" {
		t.Fatalf("resolved songs = %+v", result.Songs)
	}
}

func TestResolveTracksFailsAmbiguousMatchesWithoutPicking(t *testing.T) {
	spec := PlaylistSpec{Tracks: []TrackSpec{{Title: "Song", Artist: "Artist"}}}
	_, err := ResolveTracks(context.Background(), fakeSearcher{
		"Song Artist": {
			{ID: "a", Title: "Song", Artist: "Artist"},
			{ID: "b", Title: "Song", Artist: "Artist"},
		},
	}, spec, false)
	if err == nil {
		t.Fatal("ResolveTracks succeeded, want ambiguous error")
	}
	var ambiguous AmbiguousTracksError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error = %T %v, want AmbiguousTracksError", err, err)
	}
}

func TestResolveTracksFailsMissingTrack(t *testing.T) {
	spec := PlaylistSpec{Tracks: []TrackSpec{{Title: "Missing", Artist: "Artist"}}}
	_, err := ResolveTracks(context.Background(), fakeSearcher{"Missing Artist": nil}, spec, false)
	if err == nil {
		t.Fatal("ResolveTracks succeeded, want missing error")
	}
	var missing MissingTracksError
	if !errors.As(err, &missing) {
		t.Fatalf("error = %T %v, want MissingTracksError", err, err)
	}
}

func TestResolveTracksAllowFuzzyAcceptsSingleNonExactCandidate(t *testing.T) {
	spec := PlaylistSpec{Tracks: []TrackSpec{{Title: "Song", Artist: "Artist"}}}
	result, err := ResolveTracks(context.Background(), fakeSearcher{
		"Song Artist": {
			{ID: "a", Title: "Song - Remastered", Artist: "Artist"},
		},
	}, spec, true)
	if err != nil {
		t.Fatalf("ResolveTracks: %v", err)
	}
	if len(result.Songs) != 1 || result.Songs[0].ID != "a" {
		t.Fatalf("resolved songs = %+v", result.Songs)
	}
}
