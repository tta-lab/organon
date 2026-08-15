Serve typed forge tools over MCP stdio. Repository operations accept an exact
registered single-layer project alias. `clone` accepts exactly one of a registered
project alias or an HTTP(S) URL. URL mode may use an optional new alias or the
reference flag. No tool accepts a destination path, working directory, MCP
root, token, or file URI.

The MCP server loads configuration and the project registry once at startup,
then reuses the configured OG service for its lifetime. All pull request tools
require an exact project alias. Get, modify, comment, checks, log, and failures
accept an optional positive PR ID and use the registered checkout's current
branch when it is omitted.

Tools:

  auth_status             # inspect secret-free forge authentication state
  clone                   # clone registered alias or URL to its controlled path
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

Push, pull, create, find, and `pr_get` without an ID intentionally mirror their
CLI behavior and operate on the current named branch at the path registered for
the alias. Force push is rejected on the default branch and otherwise uses
`--force-with-lease`. Pull may switch to the default branch and delete a closed
PR branch locally and remotely after the same safety checks as the CLI.

Archived aliases remain readable. Their pull is restricted to fast-forwarding
the known default branch; push, tag, PR mutation/comment, and branch cleanup are
blocked. Registry additions are visible on the next tool call without
restarting this MCP process.

Tag, merge, and raw provider access are not exposed.

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
