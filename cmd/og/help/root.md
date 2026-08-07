# og

Organon forge operations.

`og` is the local entrypoint for typed repository and forge workflows. It
contains URL-based clone, pull request, guarded push/pull/tag, auth, and daemon
operations. Merge is intentionally out of scope.

`og clone <http(s)-url>` derives `~/code/projects/<owner>/<repo>` and registers
the project alias. `og clone --reference <url>` derives
`~/code/references/<host>/<owner>/<repo>` and never registers an alias. The
caller cannot choose a destination path.

GitHub commands authenticate in the daemon with repository-scoped GitHub App
installation tokens. Worker environments and request payloads do not provide
GitHub credentials. Run `og auth status` inside a registered repository to
check the App installation, repository scope, and required permissions.

`og mcp` mirrors the supported CLI forge workflows over stdio. MCP callers
select a repository only through its exact registered project alias; filesystem
paths are not exposed. Push, pull, PR create/find/view, and PR operations with
an omitted ID use the registered checkout's current branch. Explicit-ID PR
operations remain independent of the checked-out branch.

Archived projects remain discoverable and may use read-only forge/CI operations
plus a fast-forward-only pull on their known default branch. Push, tag, PR
mutation/comment, and pull branch cleanup are rejected by the daemon.
