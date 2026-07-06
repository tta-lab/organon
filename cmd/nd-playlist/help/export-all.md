Export server playlists to YAML files in a directory.

Examples:
  nd-playlist export-all --owner ooneil --output playlists/navidrome
  nd-playlist export-all --output playlists/navidrome

When `--owner` is omitted, the configured username is used. Existing files with
the same generated name are overwritten.
