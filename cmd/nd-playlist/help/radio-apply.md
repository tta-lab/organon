Create missing Navidrome internet radio stations from a YAML spec.

  nd-playlist radio apply --dry-run playlists/navidrome/radios/cliamp.yaml
  nd-playlist radio apply --yes playlists/navidrome/radios/cliamp.yaml

Existing stream URLs are left unchanged. `--yes` is required to create stations.
