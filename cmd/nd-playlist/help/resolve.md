Resolve a YAML playlist spec to concrete Navidrome song IDs without mutation.

Examples:
  nd-playlist resolve playlists/navidrome/mandopop-soft-night.yaml
  nd-playlist resolve --json playlists/navidrome/mandopop-soft-night.yaml
  nd-playlist resolve --allow-fuzzy playlists/navidrome/draft.yaml

Resolution rules:

- pinned `id` values are verified with `getSong.view`
- unpinned tracks match exact title + artist + optional album
- `--allow-fuzzy` accepts exactly one non-exact search result
- missing tracks and ambiguous candidates are printed clearly
