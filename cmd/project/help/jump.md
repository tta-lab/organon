Resolve and print a path suitable for cd.

Resolution order:
  1. Exact, single-layer alias in projects.toml
  2. org/repo pattern → find an existing GitHub reference
  3. Bare name → find unique match in references directory

  project jump <alias>          # print path from projects.toml
  project jump org/repo         # print an existing reference path

Missing references are never cloned by project. Use:

  og clone --reference https://github.com/org/repo
