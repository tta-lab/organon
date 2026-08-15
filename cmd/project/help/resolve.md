Resolve a project reference or an explicit absolute catalog path to project
identity, path, and archive state. Always outputs JSON. Registered references
match aliases, checkout basenames, or remote repository basenames exactly,
case-insensitively, and return the canonical alias. Reference-repository
org/repo lookup remains a separate fallback mode.

  project resolve <project-reference>
  project resolve <absolute-catalog-path>
