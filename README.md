# organon

Structure-aware tools for AI agents, plus small workflow CLIs used in the tta-lab workspace.

Organon provides commands that give [logos](https://github.com/tta-lab/logos) agents structured perception of code, running inside a [temenos](https://github.com/tta-lab/temenos) sandbox.

```
$ src main.go --tree
├── [aE] func main()               [L1-L15]
├── [bK] func handleRequest()      [L17-L45]
└── [c3] type Config struct        [L47-L55]

$ src main.go -s bK
func handleRequest(w http.ResponseWriter, r *http.Request) {
    ...
}

$ src replace main.go -s bK <<'EOF'
func handleRequest(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    // new implementation
}
EOF
```

## Commands

### `src` — Source files

Read and edit code, config, and docs by symbol. Tree-sitter parses the file into an AST, assigns each symbol a 2-char ID, and you reference symbols by ID — no text matching, no multiline quoting problems.

```bash
src main.go --tree                      # symbol tree with IDs and line ranges
src main.go -s bK                       # read a symbol by ID
src replace main.go -s bK <<'EOF'       # replace a symbol (stdin)
...
EOF
src insert main.go --after bK <<'EOF'   # insert after a symbol (stdin)
...
EOF
src delete main.go -s c3                # delete a symbol
cat <<'EDIT' | src edit config.yaml     # text replace (===BEFORE===/===AFTER===)
===BEFORE===
old text
===AFTER===
new text
EDIT
```

Supports symbol-aware extraction for Go, Rust, TypeScript, TSX, Python, C, C++, Java, Ruby, JavaScript, and many more via auto-inference. Language is detected from file extension. Markdown uses heading-based sections.

`src edit` is a text-based escape hatch for files where symbol editing is overkill (config files, unsupported languages, quick edits). It uses exact match with whitespace normalization fallbacks and works on any text file regardless of language support.

### `skill` — Skill discovery

List, find, and read agent skills from project-local and global skill directories.

```bash
skill list
skill find "review and repair pull requests" --limit 3
skill get organon-src
```

### `nd-playlist` — Navidrome playlists as code

Create, update, diff, and export Navidrome playlists through the Subsonic/OpenSubsonic API.

```bash
nd-playlist ping
nd-playlist search --json "小半 陈粒"
nd-playlist resolve playlists/navidrome/night.yaml
nd-playlist diff playlists/navidrome/night.yaml
nd-playlist apply --dry-run playlists/navidrome/night.yaml
nd-playlist apply --yes playlists/navidrome/night.yaml
nd-playlist export "Mandopop: Soft Night" > playlists/navidrome/mandopop-soft-night.yaml
nd-playlist export-all --output playlists/navidrome
nd-playlist radio diff playlists/navidrome/radios/cliamp.yaml
nd-playlist radio apply --yes playlists/navidrome/radios/cliamp.yaml
nd-playlist radio export > playlists/navidrome/radios/stations.yaml
```

Default config lives at `~/.config/nd-playlist/config.toml`:

```toml
server = "https://music.example"
username = "ooneil"
password = "..."
```

`--server`, `--username`, `--password`, `NAVIDROME_URL`, `NAVIDROME_USER`, and
`NAVIDROME_PASS` override local config. If no password source is configured and
stdin is a terminal, `nd-playlist` prompts for the password. Playlist YAML
exports include song IDs but never include secrets.

Radio YAML uses `name`, `stream_url`, and optional `homepage_url`. `radio diff`
matches stations by stream URL and `radio apply --yes` creates only missing
stations. Keep machine-owned station files under `playlists/navidrome/`, which
is ignored by Git. Navidrome requires an admin account for this global change.

### `og` — guarded forge operations

Run `og pull`, `og push`, or `og pr find` inside a linked worktree or its
subdirectories to use that worktree's branch. From elsewhere, use
`og --worktree /absolute/path/to/feature push`. The worktree must belong to a
registered local repository. Use `--project <reference>` to select its registered
checkout. Linked-worktree pulls only fast-forward the selected branch.

For a local, read-only ownership query, pass an existing absolute directory in
an active registered project's linked worktree (including inside a nested repo):

```bash
og project resolve-worktree /absolute/path/to/feature/subdirectory
# {"project":"canonical-alias","worktree":"/canonical/path/to/feature"}
```

The JSON contains the canonical project alias and physical worktree root. The
command exits nonzero for invalid paths, registered checkouts, independent
clones, stale or unverifiable worktrees, and archived or ambiguous ownership.
It needs no forge access or credentials.

`og issue` reads GitHub and Forgejo repository issues without depending on the
checkout branch:

```bash
og --project organon issue list --state open --page 1 --per-page 30
og --project organon issue search "credential broker" --state all
og --project organon issue get 42 --json
og --project organon issue comments 42 --page 2
```

Writes are typed-MCP-only: `issue_create`, `issue_update_title`,
`issue_replace_body`, `issue_edit_body`, and `issue_comment`. Creation requires
both title and body (the body may be empty); title, whole body, exact batch body
edits, and comments are independent. Exact edits match each old text once in
the body just fetched, reject duplicate/missing/ambiguous/overlapping or no-op
edits before one body update, and preserve all other Markdown bytes. They do
not provide remote compare-and-swap: a simultaneous remote edit can race.
List defaults to open, search to all; pagination defaults to page 1 / 30 items
and accepts at most 100. Comments are oldest first. Provider search ranking and
tokenization differ, and neither uncertain creates nor comments are retried.

`pr_edit_body` is the typed MCP counterpart to `issue_edit_body`, using the
same exact `oldText`/`newText` batch envelope:

```json
{
  "project": "orga",
  "pr_id": 123,
  "edits": [
    {"oldText": "Old paragraph\n", "newText": "New paragraph\n"},
    {"oldText": "Obsolete paragraph\n", "newText": ""}
  ]
}
```

For an issue, call `issue_edit_body` with `issue_id` instead of `pr_id`.
Omit `pr_id` to target the current branch's PR; an explicit positive ID works
without a current branch. Both tools require a nonempty batch and nonempty,
unique original-body matches; empty `newText` deletes. PR edits preserve the
title and comments. An uncertain write is not automatically retried; a
provider error after writing does not guarantee rollback.

`og` runs GitHub PR and Git network operations directly inside the calling CLI or MCP
process. Configure exactly one GitHub mode: an owner-scoped fine-grained PAT or
a GitHub App. `GITHUB_TOKEN`, `GH_TOKEN`, and `github_token_env` are not used.
Forgejo continues to use its token environment variables. GitHub merge and CI
queries are unsupported in both modes; GitHub PR view omits CI. Forgejo keeps
merge and CI status, while CI logs and failure details are outside og.

Controlled `og` Git transport honors only a supported user-global `http.version`
value (`HTTP/1.1` or `HTTP/2`); all other global and system Git configuration
remains isolated. To select HTTP/1.1 for a proxy path with HTTP/2 framing
failures, run:

```bash
git config --global http.version HTTP/1.1
```

This is a transport preference for HTTP/2 framing failures, not a fix for TCP
443, TLS handshake, proxy-node, routing, or forge availability failures. No
API/MCP schema, deployment, or secret-handling behavior changes with this
exception.

Pull-request merging honors the global operator-owned `merge.approval` policy
in `~/.config/ttal/og.toml` through both CLI and typed MCP. Omitted policy defaults
to `impri`; `impri`, `none`, and `deny` apply to Forgejo, with no per-project
overrides. GitHub merge is always rejected, including dry-run. To reject merge requests, including dry-run,
before contacting Impri or a forge, configure:

```toml
[merge]
approval = "deny"
```

The rejection is non-retryable while `deny` remains configured. No Impri section
is required; if present, it is still validated. This policy governs merges
requested through `og`; forge permissions remain separate.

For direct execution, configure:

```toml
[merge]
approval = "none"
```

Under `none`, requesting a merge immediately runs the existing guards and
expected-head squash operation without initializing or contacting Impri. No
Impri section or credentials are required, and no notifications are sent. An
explicit Impri section is still validated as part of the complete config file.
`--wait` and `--timeout` retain their argument validation but do not wait under
`none`; they control approval waiting only under `impri`.

Direct results identify `approval_policy=none`. `blocked` is retryable with
`next_action=retry_same_request` for pending or unverifiable CI and temporary
provider/checkout failures. CI `failure`/`error`, a closed-unmerged or
nonmergeable PR, or identity changes during the request return terminal `failed`
with `next_action=none`; resolve the reported condition before a fresh request.
CI `success` or `not_configured` permits execution. Remote trust, provider
support, immutable expected head, forge protections, and safe checkout checks
still apply. A subsequent request snapshots the current PR anew.

An `executed` direct result has no action ID, inbox, or receipt. If cleanup
fails, it stays `executed` with `cleanup_error` and
`next_action=retry_same_request`. Retry with the same checkout selector,
**explicit PR ID**, and mode. A confirmed merge resumes cleanup without a
second forge merge, including after an uncertain provider response. Completion
requires `executed` with no cleanup error. Direct dry-run uses the existing mock executor
without forge mutation or checkout switch, pull, or deletion; it is not an
expanded preflight check.

Under the default `impri` policy, merging is approval-gated. Configure the optional
operator-owned section in the existing `~/.config/ttal/og.toml`; the API key is
never accepted by a tool request or shown in output:

```toml
[impri]
base_url = "http://impri.localhost:17480"
api_key = "im_..."
```

Organon reads this optional section only from `og.toml`; there is no Impri
environment-variable fallback or second configuration file.

`og pr merge --dry-run --pr-id 123 --json` submits an immutable `git.merge_pr`
action containing the provider, forge, repository, PR number, head branch, head
SHA, base branch, squash method, and execution mode. The structured result includes the
action ID and Impri inbox URL. An operator approves or rejects the card in the
web inbox; agents may use `--wait --timeout 30s` and repeat the same
checkout selector, explicit PR ID, and mode to recover the action. The returned
action ID identifies the action for audit and in the web inbox.
Dry-run approval records a mock merge without changing the forge. Real mode
uses the same gate, then refetches the PR and CI and requires an open,
mergeable PR at the approved head SHA with CI state `success` or
`not_configured` before a squash merge. `pr_checks` exposes that machine state;
for `not_configured` it also explains that merge policy allows proceeding
without checks. Pending, rejected,
expired, timed-out, malformed, stale, and failed actions do not merge.
Provider/network/API availability failures leave an approved action retryable;
repeat the same request without new approval. Deterministic guard or executor
failures end the approved action as terminal `execute_failed`; this does not
mean the PR can never merge, only that a later attempt needs new approval. A
rejected or expired action is automatically replaced when the same request is
repeated, even if the immutable PR snapshot is unchanged. A successful attempt is reported as
`executed`. Real mode records the receipt and runs checkout cleanup. Dry-run
leaves the forge and checkout intact. If receipt reporting fails, the response
identifies the merged outcome and a repeated request repairs the receipt.
If cleanup fails after the receipt, the response remains `executed` with a
secret-free `cleanup_error`, `retryable=true`, and `next_action=retry_same_request`.
Repeat with the same checkout selector, explicit PR ID, and mode to continue
cleanup. Completion requires the executed receipt and successful checkout
cleanup. If Impri approval GET or wait polling is temporarily unavailable, the result is
the local `unavailable` state: no forge call was made, and the same request is
retryable. If an `execute_failed` receipt cannot be recorded, the result stays
approved and retryable; repeating the request revalidates and reports the
deterministic failure rather than pretending Impri accepted it.

Real cleanup removes only local and `origin` head refs that still match the
approved SHA. In the registered checkout, it also fast-forwards the default
branch. From a linked feature worktree, it detaches at the merged PR head,
cleans up the matching branch refs, then removes the directory. The registered
checkout stays on its current branch and commit. Git protects tracked edits and
ordinary untracked files. Ignored files, including local configuration and build
output, can be removed with the directory. If cleanup fails after the merge, use
the same worktree path and explicit PR ID to resume. If the directory is
already gone and the response was lost, inspect the PR by canonical project
alias and PR ID. `og pull` keeps its guarded closed-PR workflow in the
registered checkout. Telegram, webhooks, a daemon, and Impri API-key
provisioning or rotation are outside this version. The final operator PoC uses
the configured deployed Impri instance, a web-approved dry-run card, and no
real merge.

### GitHub authentication setup

Choose **one** section in `~/.config/ttal/og.toml`: `[github_pat]` or
`[github_app]`. Configuring both is an error. PAT mode uses exactly one resource
owner, checked before reading credentials. Out-of-owner Git clone and pull may
proceed anonymously for public repositories; writes and API operations are
rejected. In-scope authentication failures never fall back to anonymous access
or an App. App mode
retains its allowed-owner and repository-scoped installation token behavior.

To create a fine-grained PAT, open GitHub **Settings → Developer settings →
Personal access tokens → Fine-grained tokens → Generate new token**. Choose a
name and expiration, select the organization or user as **Resource owner**,
and select only the repositories og needs. Request the repository permissions
below. The token cannot exceed the creator's own repository access. An
organization may block PATs, restrict expiration, or require owner approval; a
pending token can only read public resources until approved. See GitHub's
[token instructions](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens)
and [organization policy](https://docs.github.com/en/organizations/managing-programmatic-access-to-your-organization/setting-a-personal-access-token-policy-for-your-organization).

| Repository permission | Read-only use | Write use |
| --- | --- | --- |
| Contents | Read for clone and pull | Read and write for push and tags |
| Pull requests | Read for PR lookup and details, and for private active non-default-branch pulls | Read and write for PR creation and edits |
| Issues | Read for issue lookup and comments | Read and write for issue writes and comments; PR comments use the issues API |
| Workflows | Not needed | Read and write only when pushing changes to `.github/workflows` |
| Metadata | Read (automatically included) | Read (automatically included) |

For a private repository, pulling the default branch requires Contents read.
Pulling an active non-default branch also requires Pull requests read: og looks
up the branch's PR to detect a closed PR before pulling or cleaning up. GitHub's
[PR list](https://docs.github.com/en/rest/pulls/pulls#list-pull-requests) and
[PRs associated with a commit](https://docs.github.com/en/rest/commits/commits#list-pull-requests-associated-with-a-commit)
endpoints both require that permission for private resources.
GitHub's [fine-grained REST permission table](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens)
documents the endpoint requirements. Checks and Actions permissions are not
needed by og. If an operation is unused, omit its permission. Branch rules and
organization policy still govern writes.

Store the token in a private file outside the checkout; do not put it in shell
history, command arguments, environment variables, URLs, Git config, logs, or
agent messages. Create a `0700` directory and a current-user-owned regular
`0600` file, then paste the token into that file using a private editor. The
file must not be a symlink and contains only the token, optionally followed by
one newline. One way to create it without placing the token in a command is:

```bash
umask 077
install -d -m 700 ~/.config/ttal/og
${EDITOR:-vi} ~/.config/ttal/og/github-pat # paste and save the token privately
chmod 600 ~/.config/ttal/og/github-pat
```

For example, with the token privately placed in
`~/.config/ttal/og/github-pat`, configure:

```toml
[github_pat]
owner = "example-org"
token_file = "og/github-pat"

[forgejo]
allowed_base_urls = ["https://forge.example.com"]
```

Relative token paths are resolved against the directory containing `og.toml`.
Restart a running MCP server after changing the config. `og auth status --project
<alias>` checks owner and file accessibility but does not validate the token
with GitHub or prove permissions; its PAT `ready` field is false. Verify access
with a permitted read such as `og pr get <number> --project <alias>`. A 401 or
403, or a private repository appearing absent, can mean expiry, revocation,
missing selected repository/permission, or organization approval/policy. Check
those settings without printing the token. For rotation, create and approve a
replacement with the same intended scope, replace the protected file, verify a
read, then revoke the old token. og reads the file for each operation.

For multiple owners, configure a GitHub App instead (not alongside PAT):

```toml
[github_app]
app_id = 123456
key_source = "file"
key_ref = "og/github-app.pem"
allowed_owners = ["example-org", "another-org"]
```

Install the App on selected repositories and obtain owner approval. For the
full retained feature set grant Contents, Pull requests, and Issues read/write;
Workflows read/write is optional for workflow-file pushes. App private keys use
the same private directory and file protections. `og auth status` checks App
installation permissions; it does not require Checks, Actions, or optional
Workflows. Installation tokens are repository and purpose scoped and are
kept in memory. GitHub's own rulesets remain the default-branch boundary.

Clone URLs through og so destination, authentication, and registration share one
boundary: `og clone <registered-alias>` or `og clone --reference
https://github.com/owner/repo.git`. Registered project remotes are verified
against Git's effective fetch and push URLs before credentials are resolved.
Archived projects can read and fast-forward their known default branch but cannot
push, tag, mutate PRs, or perform branch cleanup.

## MCP servers

`og` and `skill` provide typed stdio MCP servers. `og mcp` combines local
project discovery, read-only source search/inspection, and guarded forge
workflows. Project discovery and source inspection work without forge credentials
or network availability:

```json
{
  "mcpServers": {
    "organon-og": { "command": "og", "args": ["mcp"] },
    "organon-skill": { "command": "skill", "args": ["mcp"] }
  }
}
```

Use `project_get` or `project_list` to discover the exact five-field project
record: alias, name, path, canonical remote, and archive state. Active aliases
are single-layer names and cannot contain dots. Project registry updates are
visible on the next MCP call. `push`, `pull`, and PR tools take one of
`project` (a registered reference) and `worktree` (an absolute linked checkout root
on the server). For example, `push({"worktree":"/absolute/path/to/feature"})`
infers the project from Git and returns its canonical alias. Source tools also
accept an absolute path exactly equal to a registered checkout; their file paths
must remain within that checkout. `clone` accepts a URL instead. The `og` MCP process initializes configuration on its
first forge call, then retains Git, registration, policy, credentials, or that
initialization error for its lifetime.

`skill mcp` exposes `skill_list`, `skill_find`, and `skill_get`. With no project
argument it searches only global skill directories. With an exact registered
alias, project-local skills take priority over global skills. Results use stable
source labels such as `project:.agents` instead of discovery-root paths.
`skill_find` accepts one natural-language `query` and an optional `limit`, then
ranks token matches across skill names, descriptions, and categories.
The CLI `skill find` command uses the same query validation, defaults, limits,
and ranking behavior.
Individual `SKILL.md` files larger than 1 MiB are rejected before parsing.

`og mcp` exposes typed project discovery, read-only `source_list`,
`source_search`, `source_symbols`, `source_read`, `repo_status`,
`repo_commits`, `repo_compare`, and `repo_diff`, plus auth, clone, push, pull, PR
create/find, PR get/modify/comment/checks/log/failures, and policy-controlled
`pr_merge`. `source_search(project, pattern, limit?)` accepts a ripgrep
regular-expression pattern as one argument; Organon controls all rg flags and
the registered checkout root. `repo_diff(project, path?)` returns the tracked
unified diff from the local default-branch merge base through the working tree,
along with file summaries and untracked paths. It does not fetch, and it bounds
the patch to the standard tool-output limit. `source_list` browses direct
children of a directory; `repo_status` separates staged, unstaged, untracked,
and conflicted paths. `repo_commits` lists HEAD or local origin default-branch
history, and `repo_compare` compares two full commit IDs without including
working-tree changes. Create and modify are typed MCP-only because their
user-controlled title/body fields may contain multiline free text; exposing
those bodies through shell CLI arguments would reintroduce quoting and
escaping ambiguity. `pr_merge` remains available through both typed MCP and
`og pr merge` because its inputs are structured short scalar flags; agents
should default to MCP, and both adapters call the identical shared merge domain operation.

Merge callers consume `approval_policy` independently of `snapshot.execution_mode`,
and the structured result fields `status`, `retryable`,
`next_action`, `completion`, `detail`, `action_id`, `inbox_url`,
`receipt_error`, `cleanup_error`, and
`snapshot`. Direct policy omits approval metadata and uses `blocked`, `failed`,
and `executed` as described above. The remaining approval lifecycle guidance
applies to `approval_policy=impri`. The `action_id` is audit/web identification only. Surface the inbox
and wait for pending actions; `retry_same_request` means repeat the exact same
project, PR ID, and mode request; treat executed with no receipt or cleanup
error as complete. Repeating the same request after rejection or expiry creates
a replacement approval, while terminal execution failure ends only that
approval action and needs new approval for a later merge attempt. The local
`unavailable` status means approval state is unknown, no forge call was made,
and the same request is retryable. An executed result with `receipt_error` uses
`repair_receipt` and is repeated only to repair the Impri receipt, never to
merge again. An executed result with `cleanup_error` uses
`retry_same_request`; it reports that the forge merge is complete and cleanup
must finish without another forge call. Completion means executed with neither
receipt nor cleanup error.
MCP current-
branch workflows use the registered checkout's current branch. Force push uses
force-with-lease and is rejected on the default branch. Pull retains the CLI's
guarded closed-PR branch cleanup. A positive PR ID selects a branch-free remote
operation; `pr_get`, modify, comment, checks, log, and failures use the current
branch when the ID is omitted. Tag remains CLI-only. Start a new `og` process after
changing `og.toml`. Run
`<tool> mcp --help` for each server's tool list and configuration details.

## Pi extensions

Pi's only native Organon package is `@tta-lab/pi-src`, which replaces Pi's
built-in `read` and `edit` with symbol-aware operations and exact multi-edit
batches. Pi accesses project discovery and forge workflows through its existing
`og mcp` integration; Organon does not ship a Pi bridge for those tools. See
[`pi/README.md`](pi/README.md) for pi-src installation, platforms, development,
and release guidance.

## Why

AI agents that work via shell commands (like logos) can't do multiline file edits. Every existing edit tool uses structured JSON parameters — `{"path":"file","edits":[{"oldText":"...","newText":"..."}]}` — which requires a tool-calling protocol, not shell.

Organon solves this by replacing text matching with **symbol targeting**. The LLM doesn't need to reproduce the old code — it asks for the symbol tree, picks an ID, and pipes the new code via a single heredoc. One stdin arg instead of two JSON fields.

## Install

`og mcp` source search and `make test` require ripgrep (`rg`) on `PATH`.
Install it with `brew install ripgrep` on macOS or `sudo apt-get install ripgrep`
on Debian/Ubuntu.

### Homebrew

```bash
brew install tta-lab/ttal/organon
```

### From source

```bash
CGO_ENABLED=0 go install github.com/tta-lab/organon/cmd/src@latest
CGO_ENABLED=0 go install github.com/tta-lab/organon/cmd/skill@latest
CGO_ENABLED=0 go install github.com/tta-lab/organon/cmd/og@latest
CGO_ENABLED=0 go install github.com/tta-lab/organon/cmd/nd-playlist@latest
```

### From release

Download binaries from [GitHub Releases](https://github.com/tta-lab/organon/releases).

## How it fits

```
temenos (sandbox)
├── organon tools (pre-installed)
│   ├── src    ← structure-aware file read/edit
│   ├── skill  ← skill discovery
│   └── og     ← project discovery plus guarded Git and forge operations
├── standard tools (cat, ls, grep)
└── user code

logos (agent loop)
├── LLM writes: $ src main.go --tree
├── temenos executes in sandbox
├── output fed back to LLM
└── LLM writes: $ src replace main.go -s bK <<'EOF' ... EOF
```

## Design

- **Small cores, thin adapters** — CLI and MCP commands share typed internal
  services instead of duplicating business rules. Most commands parse, act,
  and exit; `og` composes its credentials and policy in the calling process.
- **Stdin for content** — new code goes through heredoc. One multiline arg, not two.
- **2-char IDs** — base62 identifiers for symbols/sections, same system as [flicknote](https://github.com/tta-lab/flicknote).
- **Tree-sitter** — syntax-level AST parsing. No LSP server needed.
- **Language detection** — from file extension. No `--language` flag.

## The name

Aristotle's *Organon* (ὄργανον, "instrument") was his collected works on logic — the toolkit that made reasoning possible. These tools are the instruments through which logos reasons about code.

| Project | Role |
|---------|------|
| [temenos](https://github.com/tta-lab/temenos) | The boundary — sandbox isolation |
| [logos](https://github.com/tta-lab/logos) | The reason — agent loop |
| **organon** | The instruments — perception and action |

## License

Apache-2.0
