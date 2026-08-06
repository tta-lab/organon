# og

Organon forge operations.

`og` is the local entrypoint for typed repository and forge workflows. It
contains pull request, guarded push/pull/tag, auth, and daemon operations. Merge
is intentionally out of scope.

GitHub commands authenticate in the daemon with repository-scoped GitHub App
installation tokens. Worker environments and request payloads do not provide
GitHub credentials. Run `og auth status` inside a registered repository to
check the App installation, repository scope, and required permissions.

`og mcp` serves the explicit-ID remote PR subset over stdio. MCP callers select
a repository only through its exact registered project alias; filesystem paths
and worktree-dependent operations are not exposed.
