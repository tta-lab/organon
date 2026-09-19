# Unify project capabilities under og

Registered projects represent repositories with canonical remotes, a configured
alias, and a local checkout path. A standalone local directory is not a
registered project.

`og project` owns list, find, get, resolve, and jump. `og mcp` owns the typed
`project_list`, `project_find`, and `project_get` contracts beside forge tools.
The registry remains a focused internal module; project discovery is local and
does not require forge credentials, a registered current directory, or network
availability. Reference repository discovery remains separate from registered
projects.

The standalone `project` command/server and future `pi-project`/`pi-og`
packages are removed. `pi-src` remains the only native Pi extension family;
Pi uses its existing MCP integration for og. `resolve` and `jump` remain
CLI-only. Existing MCP-only issue writes and approval-gated merge policy are
unchanged. Previously published npm versions and live installations are outside
this decision.
