# Navidrome Playlists

This directory is the initial source-of-truth location for `nd-playlist` YAML
specs.

Bootstrap from the server:

```bash
nd-playlist ping
nd-playlist export-all --owner ooneil --output playlists/navidrome
nd-playlist apply --dry-run playlists/navidrome/*.yaml
```

Apply one playlist after review:

```bash
nd-playlist search --json "song title artist"
nd-playlist diff playlists/navidrome/game-ost-story-tears.yaml
nd-playlist apply --yes playlists/navidrome/game-ost-story-tears.yaml
```

Keep secrets in `~/.config/nd-playlist/config.toml`, environment variables, or
CLI flags. Do not store Navidrome passwords in playlist specs.
