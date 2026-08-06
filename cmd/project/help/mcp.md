Serve typed, read-only project and org discovery tools over MCP stdio.

The server loads `projects.toml` and `orgs.toml` once at startup. Restart it
after changing either file. Project lookup is exact and accepts only registered,
single-layer aliases.

Tools:

  project_list             # list projects, optionally filtered by org
  project_get              # get one project by exact alias
  org_list                 # list organizations
  org_get                  # get one organization by exact name

Example MCP client configuration:

```json
{
  "mcpServers": {
    "organon-project": {
      "command": "project",
      "args": ["mcp"]
    }
  }
}
```
