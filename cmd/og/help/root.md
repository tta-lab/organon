# og

Organon forge operations.

`og` is the local entrypoint for typed repository and forge workflows. It
contains registry- and URL-based clone, pull request inspection/commenting,
approval-gated merge, guarded push/pull/tag, and auth operations. Pull-request
creation and modification are available only through the typed MCP
`pr_create`/`pr_modify` tools or Pi's `og_pr` create/modify actions; merge also
has a CLI adapter because it accepts only structured scalar flags. Agents should
default to typed MCP, and both merge adapters use the same Impri gate.

Merge is squash-only and always goes through Impri. Configure the optional
`[impri]` section in `~/.config/ttal/og.toml` with `base_url` and `api_key`.
`og pr merge --dry-run` creates a non-destructive approval card, prints its
inbox URL, and records a mock execution only after a web approval. Use
`--wait --timeout 30s` or pass `--action-id` to resume a pending card. Real
merges use the same gate and recheck the PR, CI, and head SHA immediately
before the forge call. Rejection, expiry, timeout, and execution failure leave
the PR untouched. Temporary provider/API failures keep the approved action
resumable; retry its same action ID. If Impri approval state cannot be read,
the outcome is `unavailable` and no forge call was made; retry the same action.
`og pull` remains the separate step for
closed-PR branch and worktree cleanup. Impri configuration is read only from this `og.toml`; no
`IMPRI_*` environment-variable fallback or second config file exists.

`og clone <project-reference>` clones the registered remote to the registered
path. A project reference is a case-insensitive canonical alias, checkout
basename, or remote repository basename; successful results use the canonical
alias. `og clone <http(s)-url>` derives `~/code/projects/<owner>/<repo>` and
registers the project alias. `og clone --reference <url>` derives
`~/code/references/<host>/<owner>/<repo>` and never registers an alias. The
caller cannot choose a destination path.

For registered projects, the registry remote is the repository identity. Before
each Git network operation, og requires the effective origin fetch URL—and every
push URL for writes—to match it. Repository-local hooks run normally.

GitHub commands authenticate with repository-scoped GitHub App installation
tokens. Worker environments and request payloads do not provide GitHub
credentials. Run `og auth status` inside a registered repository to check the
App installation, repository scope, and required permissions.

`og mcp` serves typed forge workflows over stdio. MCP callers
select a repository only through a project reference; filesystem paths, URLs,
and file URIs are not project selectors. Push, pull, PR creation, and PR
operations with an omitted ID use the registered checkout's current branch.
Explicit-ID PR operations remain independent of the checked-out branch.

Unknown or ambiguous references fail before Git, credentials, or forge work and
point callers to `project find` or `project list`. Archived projects remain
resolvable and visibly marked; active-only discovery does not search archives.
Archived projects may use read-only forge/CI operations plus a fast-forward-only
pull on their known default branch. Push, tag, PR mutation/comment, and pull
branch cleanup are rejected.
