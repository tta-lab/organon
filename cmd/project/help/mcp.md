Serve typed, read-only project discovery tools over MCP stdio.

Each tool reads the current `projects.toml`, so registry changes are visible on
the next call without restarting the MCP process. `project_get` accepts an exact
case-insensitive project reference: canonical alias, checkout basename, or
remote repository basename. It returns the canonical alias and preserves the
archive marker. Display names and paths are not project references.

`project_find` searches active projects and locally cloned references by alias,
display name, checkout basename, or remote repository basename. Registered
projects shadow same-named references. It returns ranked `{projects: [...]}`
results with a default limit of 8 and a maximum of 32. Reference results include
`reference: true`; empty searches are rejected and unmatched queries return an
empty array.

Tools:

  project_list             # list projects, optionally including archived
  project_get              # get one project by exact project reference
  project_find             # find ranked active projects and references

Ambiguous and unknown references never select a project automatically. Errors
include exact candidates or up to three active suggestions and point to
`project_find` or `project_list` for recovery.

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
