Export one server playlist as a YAML spec.

Examples:
  nd-playlist export "Game OST: Story & Tears" > playlists/navidrome/game-ost-story-tears.yaml
  nd-playlist export pl-d74697c7f403497c85cb9aa87715be4f

Exports include `navidrome_id` and track IDs for stable future applies. Secrets
are never written to playlist specs.
