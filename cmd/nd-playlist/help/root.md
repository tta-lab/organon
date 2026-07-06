Manage Navidrome playlists from YAML specs through the Subsonic/OpenSubsonic API.

Config is read from `~/.config/nd-playlist/config.toml` by default. Flags and
environment variables override config values:

  `--server`, `--username`, `--password`
  `NAVIDROME_URL`, `NAVIDROME_USER`, `NAVIDROME_PASS`

If no password source exists and stdin is a terminal, `nd-playlist` prompts for
the password.

Examples:
  nd-playlist ping
  nd-playlist list --json
  nd-playlist search --json "小半 陈粒"
  nd-playlist resolve playlists/navidrome/mandopop-soft-night.yaml
  nd-playlist diff playlists/navidrome/game-ost-story-tears.yaml
  nd-playlist apply --dry-run playlists/navidrome/*.yaml
  nd-playlist apply --yes playlists/navidrome/game-ost-story-tears.yaml
  nd-playlist export "Game OST: Story & Tears" > playlists/navidrome/game-ost-story-tears.yaml
  nd-playlist export-all --owner ooneil --output playlists/navidrome

Playlist specs never contain secrets.
