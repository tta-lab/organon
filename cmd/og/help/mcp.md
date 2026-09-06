Serve typed forge tools over stdio MCP. Repository operations accept a
case-insensitive project reference: canonical alias, checkout basename, or
remote repository basename. Exact alias matches take priority; ambiguous and
unknown references fail safely and point to `project find` or `project list`.
`clone` accepts exactly one of a registered project reference or an HTTP(S) URL.
URL mode may use an optional new `alias` or the `reference` flag. No tool accepts
a destination path, working directory, MCP root, token, or file URI as a project
reference.

The MCP server loads configuration and the project registry once at startup,
then reuses the configured OG service for its lifetime. All pull request tools
require a project reference. Get, modify, comment, checks, log, and failures
accept an optional positive PR ID and use the registered checkout's current
branch when it is omitted. Structured results return the canonical alias.
Unlike the shell `og pr` command, this typed interface also exposes
`pr_create` and `pr_modify` for pull-request mutations.

Tools:

  auth_status             # inspect secret-free forge authentication state
  clone                   # clone registered project reference or URL
  push                    # push current branch; optional force-with-lease
  pull                    # run the complete guarded CLI pull workflow
  pr_create               # push current branch and create its pull request
  pr_find                 # find a pull request for the current branch
  pr_get                  # get by ID, or view current branch PR and CI
  pr_modify               # replace title and/or body
  pr_comment              # create a pull request comment
  pr_checks               # inspect pull request checks
  pr_log                  # inspect CI state and failure log tail
  pr_failures             # inspect failing checks and log tails
  pr_merge                # submit an Impri approval-gated squash merge

Push, pull, create, find, and `pr_get` without an ID intentionally mirror their
CLI behavior and operate on the current named branch at the path registered for
the resolved canonical alias. Force push is rejected on the default branch and
otherwise uses `--force-with-lease`. Pull may switch to the default branch and
delete a closed PR branch locally and remotely after the same safety checks as
the CLI.

Archived aliases remain readable. Their pull is restricted to fast-forwarding
the known default branch; push, tag, PR mutation/comment, and branch cleanup are
blocked. Registry additions are visible on the next tool call without
restarting this MCP process.

`pr_merge` is destructive and always requires an Impri web decision. Its input
contains only the project, optional PR ID, dry-run flag,
and wait/timeout controls; it never accepts an API key. The structured result
contains the immutable approved snapshot, action ID, status, and inbox URL; the
action ID is informational for audit/web identification, not an input.
Agents must show that URL and wait for approval rather than invoking forge
tooling directly. Follow the structured `next_action` and `completion` fields:
pending waits, an approved temporary failure repeats the same request,
executed is complete only when `receipt_error` and `cleanup_error` are absent,
an executed `cleanup_error` means the forge merge and receipt are complete but
checkout cleanup remains, and the same request is safe to repeat without a
second forge merge. An `unavailable` result means Impri state is unknown and
the same request retries without a forge call, terminal rejection/expiry/failure
needs new approval, and `receipt_error` retries only to repair the receipt. If
an execute_failed receipt write fails, the result remains approved and repetition
revalidates/reports it. Real mode is
squash-only and permits CI state `success` or `not_configured`; pending remains
retryable and unknown/unverifiable CI fails closed. The CLI also supports merge because
its flags are short scalars; create/modify remain typed MCP/Pi-only to avoid
multiline shell quoting ambiguity, and both merge adapters share this gate.

Tag and raw provider access are not exposed. Telegram, webhooks, a daemon,
API-key provisioning/rotation, and worktree coordination are outside this
version. Real merge performs guarded single-checkout cleanup automatically;
`og pull` remains available for its existing closed-PR workflow.

Example MCP client configuration:

```json
{
  "mcpServers": {
    "organon-og": {
      "command": "og",
      "args": ["mcp"]
    }
  }
}
```
