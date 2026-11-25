# og

Organon forge operations.

`og` is the local entrypoint for typed repository and forge workflows. It
contains registry- and URL-based clone, issue discovery/reading, pull request inspection/commenting,
policy-controlled merge, guarded push/pull/tag, and auth operations. Pull-request
creation and modification are available only through the typed MCP
`pr_create`/`pr_modify` tools; merge also
has a CLI adapter because it accepts only structured scalar flags. Agents should
default to typed MCP, and both merge adapters honor the same global policy.

The global `[merge]` setting in `~/.config/ttal/og.toml` selects approval policy:

```toml
[merge]
approval = "none" # direct execution; omitted or "impri" requires web approval
```

`impri`, `none`, and `deny` govern Forgejo, with no per-project
overrides. `deny` rejects all merge requests, including dry-run, before contacting
Impri or a forge. Neither `deny` nor direct policy needs Impri configuration.
Direct policy makes no Impri requests and sends no notifications. A configured
Impri section still receives whole-file validation.
Both transports share eligibility checks and expected-head squash execution.
CI `success` or `not_configured` permits real execution; pending or unverifiable
CI returns `blocked`, `retryable=true`, and `next_action=retry_same_request`
immediately. CI `failure`/`error`, closed-unmerged/nonmergeable PRs, and changed
identity return `failed` with `next_action=none`. Resolve the condition before a
fresh request. Remote trust, forge protections, supported providers, and safe
checkout guards remain enforced.

Read `approval_policy` separately from `snapshot.execution_mode`. Direct results
have no action ID, inbox URL, or receipt. `--wait`/timeout retain validation but
wait only for Impri approval. Under `impri` and `none`, dry-run uses the
non-destructive mock without forge mutation or checkout changes. Real merge
cleans up matching local and origin head refs and either pulls the default
branch in the registered checkout or removes the selected feature worktree.
For an `executed` result with `cleanup_error`, repeat the request with the same
checkout selector, **explicit PR ID**, and mode to finish cleanup. An observed
merged PR resumes cleanup, including after an uncertain provider response. Completion requires executed
with no receipt or cleanup error. Follow `next_action`, `completion`, and `detail`.

Under default `impri`, configure `[impri]` with `base_url` and `api_key` in
og.toml. Surface the inbox URL and wait for web approval. Temporary approved
failures and `unavailable` approval state repeat the same request; terminal
execution failure requires new approval. Executed `receipt_error` uses
`repair_receipt` without another forge merge, then continues checkout cleanup.
The action ID is audit/web identification only. Impri has no environment-variable
fallback or second config file. See `og pr --help` for merge cleanup and retry
instructions.

`og project` owns local registered-project discovery and navigation. Use
`og project list`, `find`, `get`, `resolve`, `resolve-worktree`, and `jump`; resolve emits JSON
identity/path data and jump emits a path only. These operations use local
catalog/reference state and do not need forge credentials or network access.

`og clone <project-reference>` clones the registered remote to the registered
path. A project reference is a case-insensitive canonical alias, checkout
basename, or remote repository basename; successful results use the canonical
alias. `og clone <http(s)-url>` derives `~/code/projects/<owner>/<repo>` with
lowercase owner and repository directory components and registers the project
alias. `og clone --reference <url>` derives
`~/code/references/<host>/<owner>/<repo>` with the same lowercase owner and
repository directory components and never registers an alias. The local path
is distinct from provider-specific repository identity, whose remote spelling
is preserved. The caller cannot choose a destination path.

For registered projects, the registry remote is the repository identity. Before
each Git network operation, og requires the effective origin fetch URL—and every
push URL for writes—to match it. Repository-local hooks run normally.

GitHub uses the execution user's Git credential helper. See the README for
setup and permissions. Forgejo provides merge and CI status. Use Woodpecker
for CI logs.

Run push, pull, and PR commands inside a linked worktree or its subdirectories.
Use `og --worktree /absolute/feature push` to select it from another directory.
The path selects a linked checkout of a registered local repository and infers
its project. Use `--project <reference>` to select the registered checkout.

`og mcp` serves typed forge workflows over stdio. Push, pull, and PR tools take
one of `project` and `worktree`. Push, pull, PR creation, and PR lookup with an
omitted ID use the selected checkout's branch. A worktree pull only fast-forwards
that branch. Real worktree merge cleanup removes the directory and its ignored
files after verifying the matching feature head.

Unknown or ambiguous references fail before Git, credentials, or forge work and
point callers to `og project find` or `og project list`. Archived projects remain
resolvable and visibly marked; active-only discovery does not search archives.
Archived projects may use read-only forge/CI operations plus a fast-forward-only
pull on their known default branch. Push, tag, PR mutation/comment, and pull
branch cleanup are rejected.

`og issue list`, `og issue search <query>`, `og issue get <index>`, and
`og issue comments <index>` read repository issues without consulting the current
branch. List defaults to open, search defaults to all, and `--page`/`--per-page`
are bounded at 100. Writes are intentionally MCP-only.
