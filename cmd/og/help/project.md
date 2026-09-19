Manage registered projects and discover local reference repositories.

  og project list
  og project find <query>...
  og project get <project-reference>
  og project resolve <project-reference-or-path>
  og project jump <project-reference|org/repo>

Successful registered-project results use the canonical configured alias.
`resolve` outputs structured JSON identity/path data and `jump` emits only a
path suitable for shell navigation. `og project` never clones a repository.
