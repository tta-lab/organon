Find active registered projects and locally cloned reference repositories by
relevance. Search covers aliases, display names, checkout basenames, and remote
repository basenames. A registered project shadows a same-named reference, so
its checkout path is returned. Results are ordered deterministically and never
select a project for an operation.

  project find <query>...              # default maximum of 8 results
  project find <query>... --limit 16   # maximum 32; limit must be positive
  project find <query>... --json       # structured {projects: [...]}

An unmatched query succeeds with an empty result. Registered results return a
canonical alias; reference results are marked `reference` and expose their path.
