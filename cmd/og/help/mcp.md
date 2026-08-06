Serve typed forge tools over MCP stdio. Every tool accepts an exact registered
single-layer project alias. The server resolves that alias internally; it does
not accept a path, working directory, MCP root, token, or file URI.

The `og` daemon must already be running. All pull request tools require an
explicit positive PR ID, so they do not depend on the registered checkout's
current branch or HEAD.

Tools:

  auth_status             # inspect secret-free forge authentication state
  pr_get                  # get one pull request by ID
  pr_modify               # replace title and/or body
  pr_comment              # create a pull request comment
  pr_checks               # inspect pull request checks
  pr_log                  # inspect CI state and failure log tail
  pr_failures             # inspect failing checks and log tails

Git operations, branch discovery, PR create/find/view, merge, raw provider
access, and daemon lifecycle operations are intentionally not exposed.

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
