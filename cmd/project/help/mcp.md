Serve typed, read-only project discovery tools over MCP stdio.

Each tool reads the current `projects.toml`, so registry changes are visible on
the next call without restarting the MCP process. Project lookup is exact and
accepts only registered, single-layer aliases.
Project results contain exactly alias, name, path, canonical remote, and archived.

Tools:

  project_list             # list projects, optionally including archived
  project_get              # get one project by exact alias

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
