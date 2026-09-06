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
  `og pr log --tail <lines>` show CI failure detail.
- `og pr merge [--pr-id <number>]` creates or resumes an Impri approval action.
  The immutable action includes the forge, repository, PR number, head SHA,
  base branch, squash method, and execution mode. Use `--dry-run` for the
  non-destructive PoC, `--wait --timeout <duration>` to poll, or
  `--action-id <id>` to resume. Always surface the printed Impri inbox URL for
  web approval. Only an approved action executes; rejected, expired, pending,
  and timed-out actions do not touch the forge. Real mode revalidates that the
  PR is open, green, mergeable, and still at the approved SHA. If the result is
  approved and retryable, retry the same action ID after the provider recovers;
  executed means complete, while rejected/expired/execute_failed require new
  approval. If Impri state is temporarily unavailable, no forge call was made:
  retry the same action ID. A failed execute_failed receipt write also remains
  approved and is revalidated on resume. An executed receipt error is repaired
  with the same action ID and never invokes a second merge.

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
