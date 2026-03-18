# organon

Structure-aware tools for AI agents. Tree-sitter code editing, web page navigation, search. No daemon, no JSON, just stdin.

Organon provides three commands that give [logos](https://github.com/tta-lab/logos) agents structured perception of code and the web, running inside a [temenos](https://github.com/tta-lab/temenos) sandbox.

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
```

Supports any language with a tree-sitter grammar: Go, TypeScript, Rust, Python, TOML, YAML, JSON, Markdown, and many more. Language is detected from file extension.

### `url` — Web pages

Fetch and navigate web pages with heading-based structure. Same `--tree` / `-s` pattern.

```bash
url https://docs.example.com --tree     # heading tree with IDs
url https://docs.example.com -s bK      # read a section
url https://docs.example.com            # read full page
```

### `web` — Web search

Search the web and return results.

```bash
web "tree-sitter Go bindings"
```

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
go install github.com/tta-lab/organon/cmd/src@latest
go install github.com/tta-lab/organon/cmd/url@latest
go install github.com/tta-lab/organon/cmd/web@latest
```

### From release

Download binaries from [GitHub Releases](https://github.com/tta-lab/organon/releases).

## How it fits

```
temenos (sandbox)
├── organon tools (pre-installed)
│   ├── src    ← structure-aware file read/edit
│   ├── url    ← web page reading
│   └── web    ← web search
├── standard tools (cat, ls, grep)
└── user code

logos (agent loop)
├── LLM writes: $ src main.go --tree
├── temenos executes in sandbox
├── output fed back to LLM
└── LLM writes: $ src replace main.go -s bK <<'EOF' ... EOF
```

## Design

- **Stateless** — no daemon, no config, no session files. Parse, act, exit.
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
