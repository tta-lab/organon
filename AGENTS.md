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
```

## Architecture

### Binaries
- `cmd/src/` — tree-sitter symbol-aware file reading/editing
- `cmd/web/` — unified web tool: `web search` (Exa/Brave/DuckDuckGo) and `web fetch` (page reading)
- `cmd/skill/` — filesystem-based skill discovery: list/get/find SKILL.md files from project-local and global agent skill directories
- `cmd/token/` — LLM token counting using tiktoken-go with cl100k_base tokenizer (Claude / GPT-4)
- `cmd/project/` — project management CLI: list, get, resolve, and jump to registered projects
- `cmd/goal/` — Lenos session goal file CLI: add/update/append/get/status via `$LENOS_GOAL`

### Shared Packages
- `internal/id/` — base62 ID generation and collision resolution
- `internal/tree/` — generic box-drawing tree renderer
- `internal/indent/` — file indent-style detection (layered: hardcoded table for opinionated languages, per-file majority scan for open languages) and reindent transform
- `internal/skill/` — filesystem-based skill discovery and frontmatter parsing
- `internal/token/` — LLM token counting with tiktoken-go; sync.OnceValues lazy init, regex fallback

### Tool-Specific Packages
- `internal/treesitter/` — tree-sitter parsing, symbol extraction, query files
- `internal/srcop/` — src file operations (replace, insert, delete, comment)
- `internal/fetch/` — url fetch backends (defuddle, browser-gateway, cache)
- `internal/markdown/` — heading parsing via goldmark
- `internal/search/` — web search backends (Exa, Brave, DuckDuckGo)
- `internal/docs/` — Context7 documentation client
- `internal/sgraph/` — Sourcegraph public GraphQL code search

## Testing

Fixture files live in `testdata/`. Tests include both unit tests and CLI integration tests.

```bash
make test                            # gotestsum with go test fallback
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
