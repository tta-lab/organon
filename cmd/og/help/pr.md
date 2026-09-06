# og pr

Pull request operations.

The CLI supports view/list, find, get, comment, checks/status, and failure logs.
Merge is not available. Pull-request creation and modification are typed-only:
use the MCP `pr_create`/`pr_modify` tools or Pi's `og_pr` create/modify actions.

Behavior:

- `og pr view --json` and `og pr list --json` show PR details and CI summary.
- `og pr find --state <open|closed|all>` resolves a PR for the current branch.
- `og pr get <index> --json` fetches a PR by index.
- `og pr comment`, `checks`, `status`, `log`, and `failures` accept an optional
  positive `--pr-id`; when omitted they use the pull request for the current
  branch.
- `og pr checks` and `og pr status` report CI status without exposing arbitrary
  provider API paths. `og pr failures --tail <lines>` and
  `og pr log --tail <lines>` show CI failure detail.

GitHub API calls use a repository-scoped App installation token minted in the
OG process. GitHub tokens supplied through the environment or request payload
are ignored or rejected.
