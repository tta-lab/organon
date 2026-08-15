Find active registered projects by relevance. Search covers aliases,
display names, checkout basenames, and remote repository basenames. Results are
ordered deterministically and never select a project for an operation.

  project find <query>...              # default maximum of 8 results
  project find <query>... --limit 16   # maximum 32; limit must be positive
  project find <query>... --json       # structured {projects: [...]}

An unmatched query succeeds with an empty result. Use `project get` with the
canonical alias returned in a result to target a project.
