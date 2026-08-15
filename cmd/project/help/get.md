Get a project by an exact case-insensitive project reference. A reference is
a canonical alias, checkout basename, or remote repository basename; the result
always contains the canonical alias. Display names, paths, URLs, and owner/repo
strings are not general project references.

Human mode retains the local reference-repository fallback for explicit reference
selectors that are not registered.

  project get <project-reference>          # print checkout path
  project get <project-reference> --json   # JSON with alias, name, path, remote, archived

Ambiguous and unknown references are never guessed. Use `project find` or
`project list` when the error provides recovery guidance.
