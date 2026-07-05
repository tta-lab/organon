package navidrome

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound indicates that a song or playlist was not found.
var ErrNotFound = errors.New("not found")

// ResolveResult contains resolved songs and non-fatal warnings.
type ResolveResult struct {
	Songs    []Song   `json:"songs"`
	Warnings []string `json:"warnings,omitempty"`
}

// MissingTracksError reports unresolved tracks.
type MissingTracksError struct {
	Tracks []TrackSpec
}

func (e MissingTracksError) Error() string {
	return fmt.Sprintf("%d track(s) missing", len(e.Tracks))
}

// AmbiguousTracksError reports tracks with multiple valid matches.
type AmbiguousTracksError struct {
	Tracks map[TrackSpec][]Song
}

func (e AmbiguousTracksError) Error() string {
	return fmt.Sprintf("%d track(s) ambiguous", len(e.Tracks))
}

// ResolveTracks resolves a playlist spec to concrete Navidrome song IDs.
func ResolveTracks(ctx context.Context, searcher Searcher, spec PlaylistSpec, allowFuzzy bool) (ResolveResult, error) {
	result := ResolveResult{Songs: make([]Song, 0, len(spec.Tracks))}
	var missing []TrackSpec
	ambiguous := map[TrackSpec][]Song{}

	for _, track := range spec.Tracks {
		song, warning, err := resolveTrack(ctx, searcher, track, allowFuzzy)
		if err == nil {
			result.Songs = append(result.Songs, song)
			if warning != "" {
				result.Warnings = append(result.Warnings, warning)
			}
			continue
		}
		if errors.Is(err, ErrNotFound) {
			missing = append(missing, track)
			continue
		}
		var candidates ambiguousCandidates
		if errors.As(err, &candidates) {
			ambiguous[track] = candidates.Songs
			continue
		}
		return ResolveResult{}, err
	}

	if len(ambiguous) > 0 {
		return ResolveResult{}, AmbiguousTracksError{Tracks: ambiguous}
	}
	if len(missing) > 0 {
		return ResolveResult{}, MissingTracksError{Tracks: missing}
	}
	return result, nil
}

type ambiguousCandidates struct {
	Songs []Song
}

func (e ambiguousCandidates) Error() string {
	return "ambiguous candidates"
}

func resolveTrack(ctx context.Context, searcher Searcher, track TrackSpec, allowFuzzy bool) (Song, string, error) {
	if track.ID != "" {
		song, err := searcher.GetSong(ctx, track.ID)
		if err != nil {
			return Song{}, "", err
		}
		if metadataMatches(track, song) {
			return song, "", nil
		}
		return song, fmt.Sprintf("metadata for pinned song %s differs from spec", track.ID), nil
	}

	query := trackQuery(track)
	candidates, err := searcher.SearchSongs(ctx, query)
	if err != nil {
		return Song{}, "", err
	}

	var exact []Song
	for _, song := range candidates {
		if metadataMatches(track, song) {
			exact = append(exact, song)
		}
	}
	if len(exact) == 1 {
		return exact[0], "", nil
	}
	if len(exact) > 1 {
		return Song{}, "", ambiguousCandidates{Songs: exact}
	}
	if allowFuzzy && len(candidates) == 1 {
		return candidates[0], "", nil
	}
	if len(candidates) > 1 {
		return Song{}, "", ambiguousCandidates{Songs: candidates}
	}
	return Song{}, "", ErrNotFound
}

func metadataMatches(track TrackSpec, song Song) bool {
	if !same(track.Title, song.Title) || !same(track.Artist, song.Artist) {
		return false
	}
	return track.Album == "" || same(track.Album, song.Album)
}

func same(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func trackQuery(track TrackSpec) string {
	parts := []string{track.Title, track.Artist}
	if track.Album != "" {
		parts = append(parts, track.Album)
	}
	return strings.Join(parts, " ")
}
