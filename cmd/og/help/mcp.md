Serve typed forge tools over MCP stdio. Every tool accepts an exact registered
single-layer project alias. The server resolves that alias internally; it does
not accept a path, working directory, MCP root, token, or file URI.

The `og` daemon must already be running. All pull request tools require an
exact project alias. `pr_get` requires a positive PR ID. Modify, comment,
checks, log, and failures accept an optional positive PR ID and use the
registered checkout's current branch when it is omitted.

Tools:

  auth_status             # inspect secret-free forge authentication state
  push                    # push current branch; optional force-with-lease
  pull                    # run the complete guarded CLI pull workflow
  pr_create               # push current branch and create its pull request
  pr_find                 # find a pull request for the current branch
  pr_view                 # view the current branch pull request and CI status
  pr_get                  # get one pull request by ID
  pr_modify               # replace title and/or body
  pr_comment              # create a pull request comment
  pr_checks               # inspect pull request checks
  pr_log                  # inspect CI state and failure log tail
  pr_failures             # inspect failing checks and log tails

Push, pull, create, find, and view intentionally mirror their CLI behavior and
operate on the current named branch at the path registered for the alias.
Force push is rejected on the default branch and otherwise uses
`--force-with-lease`. Pull may switch to the default branch and delete a closed
PR branch locally and remotely after the same safety checks as the CLI.

Tag, merge, raw provider access, and daemon lifecycle operations are not
exposed.

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
