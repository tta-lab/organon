# CLAUDE.md

## Project Overview

Organon is a Go monorepo producing three CLI tools for AI agents: `src` (tree-sitter source editing), `url` (web page reading), `web` (web search). Pre-installed in temenos sandbox for logos agents.

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
- `cmd/url/` — web page reading with heading-based navigation
- `cmd/web/` — web search (Brave API + DuckDuckGo fallback)

### Shared Packages
- `internal/id/` — base62 ID generation and collision resolution
- `internal/tree/` — generic box-drawing tree renderer

### Tool-Specific Packages
- `internal/treesitter/` — tree-sitter parsing, symbol extraction, query files
- `internal/srcop/` — src file operations (replace, insert, delete, comment)
- `internal/fetch/` — url fetch backends (defuddle, browser-gateway, cache)
- `internal/markdown/` — heading parsing via goldmark
- `internal/search/` — web search backends (Brave, DuckDuckGo)

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
