# CLAUDE.md

## Project Overview

Organon is a Go monorepo producing six CLI tools for AI agents: `src` (tree-sitter source editing), `web` (web search and page fetching), `skill` (filesystem-based skill discovery), `token` (LLM token counting), `project` (project management CLI), and `goal` (Lenos session goal file management).

## Essential Commands

```bash
make all          # fmt, vet, tidy, build
make test         # CGO_ENABLED=0 go test -v ./...
make build        # CGO_ENABLED=0 go build ./cmd/...
make install      # CGO_ENABLED=0 go install ./cmd/...
make ci           # fmt, vet, lint, test, build
make ci-scope SCOPE_CMD=web SCOPE_PACKAGES='./cmd/web ./internal/search ./internal/config'
                  # scoped format check, vet, lint, test, and binary build
```

## Architecture

### Binaries
- `cmd/src/` — tree-sitter symbol-aware file reading/editing plus read-only project-scoped MCP
- `cmd/web/` — unified web tool: `web search` (Exa/Brave/DuckDuckGo) and `web fetch` (page reading)
- `cmd/skill/` — filesystem-based skill discovery plus project-aware read-only MCP
- `cmd/token/` — LLM token counting using tiktoken-go with cl100k_base tokenizer (Claude / GPT-4)
- `cmd/project/` — project management CLI: list, get, resolve, and jump to registered projects
- `cmd/goal/` — Lenos session goal file CLI: add/update/append/get/status via `$LENOS_GOAL`

### Shared Packages
- `internal/id/` — base62 ID generation and collision resolution
- `internal/tree/` — generic box-drawing tree renderer
- `internal/indent/` — file indent-style detection (layered: hardcoded table for opinionated languages, per-file majority scan for open languages) and reindent transform
- `internal/skill/` — filesystem-based skill discovery, frontmatter parsing, and shared CLI/MCP search behavior
- `internal/token/` — LLM token counting with tiktoken-go; sync.OnceValues lazy init, regex fallback
- `internal/srcview/` — trusted-byte source outlines and safe registered-project file reads shared by CLI and MCP
- `internal/safefile/` — descriptor-relative contained file opening shared by project-scoped readers

### Tool-Specific Packages
- `internal/treesitter/` — tree-sitter parsing, symbol extraction, query files
- `internal/srcop/` — src file operations (replace, insert, delete, comment)
- `internal/fetch/` — url fetch backends (defuddle, browser-gateway, cache)
- `internal/markdown/` — heading parsing via goldmark
- `internal/search/` — web search backends (Exa, Brave, DuckDuckGo)
- `internal/docs/` — Context7 documentation client
- `internal/sgraph/` — Sourcegraph public GraphQL code search
- `internal/web/` — shared typed web application service used by CLI and MCP adapters
- `internal/project/` — hot, archive-aware path and canonical-remote registry shared by CLI, MCP, and og
- `internal/og/` — daemon-owned Git/forge policy and typed local protocol
- `internal/ogconfig/` — whole-file og configuration and remote trust classification

### CLI and MCP Parity

When a capability is exposed through both CLI and MCP, keep its domain behavior
the same unless the transport requires a documented difference. Put discovery,
normalization, validation, defaults and limits, ordering, and error semantics in
a shared `internal/` package. Keep `cmd/` handlers thin: parse transport-specific
inputs, call the shared behavior, and render transport-specific outputs.

Before adding MCP-only behavior, check the equivalent CLI operation and update
the shared core so both adapters inherit the change. Project aliases versus CLI
filesystem paths, structured MCP results versus human CLI output, and process
lifecycle are valid adapter differences; search or mutation semantics are not.

## Testing

Fixture files live in `testdata/`. Tests include both unit tests and CLI integration tests.

For a change confined to one CLI, do not run the full repository test suite locally. Run
`make ci-scope` with that binary and every directly changed or behaviorally affected Go
package. This avoids unrelated CLI tests and external integrations while retaining the
relevant format, vet, lint, test, and build gates.

Use full `make ci` only when the change spans multiple CLIs, changes shared behavior used
by multiple CLIs, changes repository-wide build/module tooling, or the user explicitly
requests it. Remote PR CI may still run the full suite.

```bash
make test                            # gotestsum with go test fallback
make ci-scope SCOPE_CMD=web \
  SCOPE_PACKAGES='./cmd/web ./internal/search ./internal/config'
CGO_ENABLED=0 go test ./internal/id/...
CGO_ENABLED=0 go test -v -run TestSymbols ./internal/treesitter/...
```

## CLI Design

For commands that accept potentially multiline content, read that content from stdin. Do not add positional body/text arguments for multiline payloads. Document examples with quoted heredocs:

```bash
cat <<'EOF' | tool command --flag value
multiline content
EOF
```

## Package Documentation

Every `internal/` package has a `doc.go` with a plane annotation:

```go
// Package <name> <description>.
//
// Plane: shared
package <name>
```

When creating new packages, add a `doc.go` with the appropriate plane tag.

## Common Pitfalls

1. **No cgo** — this project uses gotreesitter (pure Go). Never set `CGO_ENABLED=1`.
2. **Pushing directly to main** — branch protection requires a PR with passing CI.
3. **gotreesitter API** — use `grammars.DetectLanguage(filename)` to get a `*LangEntry`,
   then `entry.Language()` for the language and `entry.TokenSourceFactory(src, lang)` for the
   token source. Use `parser.ParseWithTokenSource(source, ts)` to parse.
