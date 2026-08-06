Resolve and print a path suitable for cd.

Resolution order:
  1. Exact, single-layer alias in projects.toml
  2. org/repo pattern → clone from GitHub if missing
  3. Bare name → find unique match in references directory

  project jump <alias>          # print path from projects.toml
  project jump org/repo         # clone from GitHub, then print path
