Get a project by exact, single-layer alias. Active aliases cannot contain dots.
Falls back to reference repos only for a valid alias not found in projects.toml.

  project get <alias>          # print filesystem path
  project get <alias> --json   # JSON with alias, name, path, org
