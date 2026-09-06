# og pr

Pull request operations.

The CLI supports view/list, find, get, comment, checks/status, failure logs,
and approval-gated squash merge. Pull-request creation and modification are typed-only:
use the MCP `pr_create`/`pr_modify` tools or Pi's `og_pr` create/modify actions.
Their multiline free-text bodies stay out of shell arguments; merge remains a
CLI adapter because it accepts only short scalar flags, while agents should
default to typed MCP. Both merge adapters use the same Impri gate.

Behavior:

- `og pr view --json` and `og pr list --json` show PR details and CI summary.
- `og pr find --state <open|closed|all>` resolves a PR for the current branch.
- `og pr get <index> --json` fetches a PR by index.
- `og pr comment`, `checks`, `status`, `log`, and `failures` accept an optional
  positive `--pr-id`; when omitted they use the pull request for the current
  branch.
- `og pr checks` and `og pr status` report CI status without exposing arbitrary
  provider API paths. `og pr failures --tail <lines>` and
  `og pr log --tail <lines>` show CI failure detail. The structured state is in
  `pr.ci.state`; `not_configured` is an explicit no-check state whose policy
  message says a merge may proceed without checks.
- `og pr merge [--pr-id <number>]` creates or repeats an idempotent Impri approval action.
  The immutable action includes the forge, repository, PR number, head SHA,
  base branch, squash method, and execution mode. Use `--dry-run` for the
  non-destructive PoC, `--wait --timeout <duration>` to poll, or
  Always surface the printed Impri inbox URL for
  web approval. Only an approved action executes; rejected, expired, pending,
  and timed-out actions do not touch the forge. Real mode revalidates that the
  PR is open, mergeable, has CI state `success` or `not_configured`, and is
  still at the approved SHA. If the result is
  approved and retryable, repeat the same project, PR, and mode request after the provider recovers;
  executed means complete, while rejected/expired/execute_failed require new
  approval. If Impri state is temporarily unavailable, no forge call was made:
  repeat the same request. A failed execute_failed receipt write also remains
  approved and is revalidated on repetition. An executed receipt error is repaired
  by repeating the same request and never invokes a second merge. The returned
  action ID is audit/web identification only.

Configure Impri in the existing user-owned `~/.config/ttal/og.toml`:

```toml
[impri]
base_url = "http://impri.localhost:17480"
api_key = "im_..." # actions-scoped operator key; never pass this to a tool
```

The section is read only from this existing `og.toml`; `IMPRI_*` environment
variables and a second Impri config file are not supported.

Telegram notifications, webhooks, a polling daemon, API-key provisioning or
rotation, and branch/worktree cleanup are outside this version. Run `og pull`
separately when the existing guarded closed-PR cleanup is wanted.

GitHub API calls use a repository-scoped App installation token minted in the
OG process. GitHub tokens supplied through the environment or request payload
are ignored or rejected.
