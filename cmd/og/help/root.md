# og

Organon forge operations.

`og` is the local entrypoint for typed repository and forge workflows. It
contains pull request, guarded push/pull/tag, auth, and daemon operations. Merge
is intentionally out of scope.

GitHub commands authenticate in the daemon with repository-scoped GitHub App
installation tokens. Worker environments and request payloads do not provide
GitHub credentials. Run `og auth status` inside a registered repository to
check the App installation, repository scope, and required permissions.

`og mcp` mirrors the supported CLI forge workflows over stdio. MCP callers
select a repository only through its exact registered project alias; filesystem
paths are not exposed. Push, pull, PR create/find/view, and PR operations with
an omitted ID use the registered checkout's current branch. Explicit-ID PR
operations remain independent of the checked-out branch.
