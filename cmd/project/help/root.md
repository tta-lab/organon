Manage registered projects — list, find, get, resolve, and navigate.

Existing project references resolve case-insensitively from a canonical alias,
checkout basename, or remote repository basename. Alias matches take priority;
ambiguous references fail and return canonical aliases. Display names are useful
for discovery but do not authorize an operation.

## List
  project list                         # active projects
  project list --include-archived      # active and archived projects
  project list --json                  # JSON output

## Find
  project find <query>...              # ranked active-project discovery
  project find <query>... --limit 16   # bound the result count (maximum 32)
  project find <query>... --json        # JSON output

## Get
  project get <project-reference>      # show one project by exact reference

## Resolve
  project resolve <project-reference-or-path>  # resolve a reference or explicit catalog path

## Jump
  project jump <project-reference>     # print a registered checkout path
  project jump org/repo                # print an existing local reference path

When a reference is not found, use `project find` or `project list`. Successful
registered-project results use the canonical configured alias for later calls.

## MCP
  project mcp                         # serve typed project discovery over stdio
