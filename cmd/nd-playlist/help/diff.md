Compare a YAML playlist spec with the current server playlist without mutation.

Examples:
  nd-playlist diff playlists/navidrome/game-ost-story-tears.yaml
  nd-playlist diff --json playlists/navidrome/game-ost-story-tears.yaml

Diff output reports full-replacement semantics: added, removed, and reordered
track counts.
