Manage registered projects — list, get, resolve, and navigate.
Registered project aliases are exact, single-layer names and cannot contain dots.

## List
  project list                         # active projects
  project list --include-archived      # active and archived projects
  project list --json                  # JSON output

## Get
  project get <alias>          # show project details by alias

## Resolve
  project resolve <alias-or-path>  # resolve alias/path to project identity and path

## Jump
  project jump <alias|org/repo>     # print filesystem path for a project
  project jump org/repo             # print an existing local reference path

## MCP
  project mcp                  # serve typed project discovery over stdio
