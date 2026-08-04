# og

Organon forge operations.

`og` is the local entrypoint for typed repository and forge workflows. It
contains pull request, git, auth, and daemon operations. Merge is intentionally
out of scope.

GitHub commands authenticate in the daemon with repository-scoped GitHub App
installation tokens. Worker environments and request payloads do not provide
GitHub credentials. Run `og auth status` inside a registered repository to
check the App installation, repository scope, and required permissions.
