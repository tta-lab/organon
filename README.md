# organon

Structure-aware tools for AI agents, plus small workflow CLIs used in the tta-lab workspace.

Organon provides commands that give [logos](https://github.com/tta-lab/logos) agents structured perception of code and the web, running inside a [temenos](https://github.com/tta-lab/temenos) sandbox.

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

### `web fetch` — Web pages

Fetch and navigate web pages with heading-based structure. Same `--tree` / `-s` pattern.

```bash
web fetch https://docs.example.com --tree     # heading tree with IDs
web fetch https://docs.example.com -s bK      # read a section
web fetch https://docs.example.com            # read full page
```

### `web` — Web search

Search the web and return results.

```bash
web "tree-sitter Go bindings"
```

### `web docs` — Library documentation

Resolve library names to Context7 IDs and fetch documentation.

```bash
web docs resolve react       # list matching libraries with IDs
web docs fetch /reactjs/react.dev hooks  # fetch docs for a library
CONTEXT7_API_KEY=... web docs resolve react  # with API key (higher rate limits)
```

### `skill` — Skill discovery

List, find, and read agent skills from project-local and global skill directories.

```bash
skill list
skill find web
skill get organon-web
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

`og` runs GitHub PR and Git network operations through its local daemon. GitHub
authentication uses repository-scoped installation tokens minted by a GitHub
App; `GITHUB_TOKEN`, `GH_TOKEN`, and `github_token_env` are not used. Forgejo
continues to use its existing token environment variables.

Create the real App only after both implementation PRs are merged, green, and
the merged binary is ready to install. Register one App under a stable GitHub
account and allow it to be installed by the other managed accounts. It needs no
Marketplace listing, OAuth flow, callback, webhook, or organization permission.
Grant only these repository permissions:

- Contents: read and write
- Pull requests: read and write
- Checks: read-only
- Actions: read-only
- Workflows: read and write

Install it on selected repositories only in `tta-lab`, `GuionAI`, and
`LamplitIsles`. Each installation owner must approve the permissions. Do not
grant Administration, Members, Secrets, Deployments, or other organization
access.

Before testing writes, configure every managed default-branch ruleset to require
a PR and required checks, restrict updates, and disallow force pushes and
deletion. Keep the App out of every bypass list. `og` can reach `main` and its
App token has Contents write permission, so GitHub rulesets are the hard merge
and default-branch boundary.

After downloading a private key, keep it outside the repository and configure
the daemon:

```bash
install -d -m 700 ~/.config/ttal/og
install -m 600 ~/Downloads/your-app.private-key.pem \
  ~/.config/ttal/og/github-app.pem
cat <<'EOF' > ~/.config/ttal/og.toml
[github_app]
app_id = 123456
key_source = "file"
key_ref = "og/github-app.pem"
allowed_owners = ["tta-lab", "GuionAI", "LamplitIsles"]
EOF
chmod 600 ~/.config/ttal/og.toml

make install
og daemon restart
og daemon health
```

Keep the migration PAT active during rollout. In one selected repository from
each owner, run `og auth status` and require every permission to report ready.
Then use a disposable feature branch to verify `og push`, `og pr create`,
`og pr view`, `og pr checks`, and `og pr comment`. Confirm GitHub attributes the
push and PR activity to the App bot. Also verify that an SSH-configured origin
is unchanged on disk, an uninstalled managed write fails without fallback, and
a third-party public repository can pull anonymously but cannot write. Inspect
daemon logs and local Git configuration for credential material without
printing any secret values. Remove the downloaded key copy after this passes;
GitHub can issue a replacement, so no private-key backup is required.

Only after all three owners pass, remove local `github_token_env` keys and
GitHub PAT variables from `~/.config/ttal/.env` and shell startup files. Restart
the daemon, repeat a representative App-only push and PR check, and then revoke
the migration PAT.

Installation tokens are automatic, memory-only, repository-scoped, and expire
after about one hour. The App private key is long-lived: it must never enter an
agent environment or Git child process. Direct `git`, `gh`, and arbitrary API
calls are outside the daemon credential boundary and may still use personal
credentials; mandatory GitHub rulesets protect the default branch in those
paths.

For key rotation, create a replacement App key, install it with mode `0600`,
restart the daemon, verify all owners with `og auth status`, and then revoke and
remove the old key. Before PAT revocation, rollback means reinstalling the
previous `og` binary and restarting its daemon with the existing migration PAT.
After revocation, emergency rollback requires a new narrow temporary PAT; never
reactivate or reuse the exposed migration PAT.

## MCP servers

`project`, `og`, and `web` each provide a typed stdio MCP server. Configure
them as separate processes so clients can grant only the tools a session needs:

```json
{
  "mcpServers": {
    "organon-project": { "command": "project", "args": ["mcp"] },
    "organon-og": { "command": "og", "args": ["mcp"] },
    "organon-web": { "command": "web", "args": ["mcp"] }
  }
}
```

Use `project_get` or `project_list` to discover an exact registered project
alias. Active aliases are single-layer names and cannot contain dots. The `og`
tools accept only that alias; they do not accept a filesystem path, working
directory, MCP root, file URI, or credential. The `og` daemon must already be
running, resolves the registered repository, and owns forge credentials.

`og mcp` mirrors the CLI push, pull, PR create, find, and current-PR view
workflows against the registered checkout's current branch. Force push uses
force-with-lease and is rejected on the default branch. Pull retains the CLI's
guarded closed-PR branch cleanup. A positive PR ID selects a branch-free remote
operation; `pr_get`, modify, comment, checks, log, and failures use the current
branch when the ID is omitted. Tag remains CLI-only. Restart a long-running MCP
server after changing project, org, or web configuration. Run
`project mcp --help`, `og mcp --help`, or `web mcp --help` for the tool lists and
configuration details.

## Why

AI agents that work via shell commands (like logos) can't do multiline file edits. Every existing edit tool uses structured JSON parameters — `{"old_text": "...", "new_text": "..."}` — which requires a tool-calling protocol, not shell.

Organon solves this by replacing text matching with **symbol targeting**. The LLM doesn't need to reproduce the old code — it asks for the symbol tree, picks an ID, and pipes the new code via a single heredoc. One stdin arg instead of two JSON fields.

## Install

### Homebrew

```bash
brew install tta-lab/ttal/organon
```

### From source

```bash
CGO_ENABLED=0 go install github.com/tta-lab/organon/cmd/src@latest
CGO_ENABLED=0 go install github.com/tta-lab/organon/cmd/web@latest
CGO_ENABLED=0 go install github.com/tta-lab/organon/cmd/skill@latest
CGO_ENABLED=0 go install github.com/tta-lab/organon/cmd/project@latest
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
│   ├── web    ← web search and page reading
│   ├── skill  ← skill discovery
│   ├── project ← registered project discovery
│   └── og     ← guarded Git and forge operations
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
  and exit; `og` uses a local credential-owning daemon.
- **Stdin for content** — new code goes through heredoc. One multiline arg, not two.
- **2-char IDs** — base62 identifiers for symbols/sections, same system as [flicknote](https://github.com/tta-lab/flicknote).
- **Tree-sitter** — syntax-level AST parsing. No LSP server needed.
- **Language detection** — from file extension. No `--language` flag.

## The name

Aristotle's *Organon* (ὄργανον, "instrument") was his collected works on logic — the toolkit that made reasoning possible. These tools are the instruments through which logos reasons about code and the web.

| Project | Role |
|---------|------|
| [temenos](https://github.com/tta-lab/temenos) | The boundary — sandbox isolation |
| [logos](https://github.com/tta-lab/logos) | The reason — agent loop |
| **organon** | The instruments — perception and action |

## License

Apache-2.0
