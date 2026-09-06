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
contains only the project, optional PR ID, optional action ID, dry-run flag,
and wait/timeout controls; it never accepts an API key. The structured result
contains the immutable approved snapshot, action ID, status, and inbox URL.
Agents must show that URL and wait for approval rather than invoking forge
tooling directly. Real mode is squash-only and revalidates the PR and CI.

Tag and raw provider access are not exposed. Telegram, webhooks, a daemon,
API-key provisioning/rotation, and branch/worktree cleanup are outside this
version; `og pull` remains the separate cleanup step.

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
