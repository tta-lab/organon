# og pr

Pull request operations.

V1 includes create, view/list, find, get, modify, comment, checks/status, and
failure logs. Merge is not available in V1.

Planned `ttal` parity:

- `og pr create <title>` reads the body from stdin and pushes the branch before
  creating the PR.
- `og pr view --json` and `og pr list --json` show PR details and CI summary.
- `og pr find --state <open|closed|all>` resolves a PR for the current branch.
- `og pr get <index> --json` fetches a PR by index.
- `og pr modify --title <title> --pr-id <id>` updates title/body, with body read
  from stdin.
- `og pr checks` and `og pr status` report CI status without exposing arbitrary
  provider API paths.
- `og pr failures --tail <lines>` and `og pr log --tail <lines>` show CI failure
  detail.
