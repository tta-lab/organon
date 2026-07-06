Create or replace server playlists from YAML specs.

Examples:
  nd-playlist apply --dry-run playlists/navidrome/*.yaml
  nd-playlist apply --yes playlists/navidrome/game-ost-story-tears.yaml
  nd-playlist apply --allow-fuzzy --dry-run playlists/navidrome/draft.yaml

Behavior:

- creates a playlist when no matching playlist exists
- replaces full contents when `navidrome_id` or exact owner + name matches
- updates metadata after contents when `comment` or `public` is present
- refuses to replace an existing playlist without `--yes` unless running in CI

Use `--dry-run` before mutation.
