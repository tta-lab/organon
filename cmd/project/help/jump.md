Resolve and print a path suitable for cd.

Registered project references match a canonical alias, checkout basename, or
remote repository basename case-insensitively. Ambiguous and unknown registered
references are never guessed. Explicit org/repo and bare reference-repository
lookup remains available:

  project jump <project-reference>
  project jump org/repo

Missing references are never cloned by project. Use:

  og clone --reference https://github.com/org/repo
