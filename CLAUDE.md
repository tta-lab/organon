# CLAUDE.md

## Project Overview

Organon is a Go monorepo producing three CLI tools for AI agents: `src` (tree-sitter source editing), `web` (web search and page fetching), and `alert` (agent-to-bridge messaging for alerts via Telegram).

## Essential Commands

```bash
make all          # fmt, vet, tidy, build
make test         # go test -v ./...
make build        # go build ./cmd/...
make install      # go install ./cmd/...
make ci           # fmt, vet, lint, test, build
```

## Architecture

### Binaries
- `cmd/src/` — tree-sitter symbol-aware file reading/editing
- `cmd/web/` — unified web tool: `web search` (Exa/Brave/DuckDuckGo) and `web fetch` (page reading)
- `cmd/alert/` — agent-to-bridge messaging: POSTs alerts to a configurable endpoint via env var

### Shared Packages
- `internal/id/` — base62 ID generation and collision resolution
- `internal/tree/` — generic box-drawing tree renderer
- `internal/indent/` — file indent-style detection (layered: hardcoded table for opinionated languages, per-file majority scan for open languages) and reindent transform

### Tool-Specific Packages
- `internal/treesitter/` — tree-sitter parsing, symbol extraction, query files
- `internal/srcop/` — src file operations (replace, insert, delete, comment)
- `internal/fetch/` — url fetch backends (defuddle, browser-gateway, cache)
- `internal/markdown/` — heading parsing via goldmark
- `internal/search/` — web search backends (Exa, Brave, DuckDuckGo)

## Testing

Fixture files live in `testdata/`. Tests include both unit tests and CLI integration tests.

```bash
make test
go test ./internal/id/...
go test -v -run TestSymbols ./internal/treesitter/...
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
