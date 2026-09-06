# CLAUDE.md

## Project Overview

Organon is a Go monorepo producing CLI tools for AI agents: `src` (tree-sitter source editing), `skill` (filesystem-based skill discovery), `token` (LLM token counting), `project` (project management CLI), and `goal` (Lenos session goal file management).

## Essential Commands

```bash
make all          # fmt, vet, tidy, build
make test         # CGO_ENABLED=0 go test -v ./...
make build        # CGO_ENABLED=0 go build ./cmd/...
make install      # CGO_ENABLED=0 go install ./cmd/...
make ci           # fmt, vet, lint, test, build
make ci-scope SCOPE_CMD=project SCOPE_PACKAGES='./cmd/project ./internal/project ./internal/config'
                  # scoped format check, vet, lint, test, and binary build
```

## Architecture

### Binaries
- `cmd/src/` — tree-sitter symbol-aware file reading/editing with local path resolution and `--json` output for Pi extension adapters
- `cmd/skill/` — filesystem-based skill discovery plus read-only MCP
- `cmd/token/` — LLM token counting using tiktoken-go with cl100k_base tokenizer (Claude / GPT-4)
- `cmd/project/` — project management CLI: list, get, resolve, and jump to registered projects
- `cmd/goal/` — Lenos session goal file CLI: add/update/append/get/status via `$LENOS_GOAL`

### Shared Packages
- `internal/id/` — base62 ID generation and collision resolution
- `internal/tree/` — generic box-drawing tree renderer
- `internal/indent/` — file indent-style detection (layered: hardcoded table for opinionated languages, per-file majority scan for open languages) and reindent transform
- `internal/skill/` — filesystem-based skill discovery, frontmatter parsing, and shared CLI/MCP search behavior
- `internal/token/` — LLM token counting with tiktoken-go; sync.OnceValues lazy init, regex fallback
- `internal/srcview/` — trusted source outlines and line-oriented reads shared by the CLI and the Pi src extension
- `internal/truncate/` — Pi-equivalent head truncation (2,000 lines / 50 KB) for CLI JSON output

### Tool-Specific Packages
- `internal/treesitter/` — tree-sitter parsing, symbol extraction, query files
- `internal/srcop/` — src file operations (replace, insert, delete, comment)
- `internal/markdown/` — heading parsing and section navigation via goldmark
- `internal/project/` — hot, archive-aware path and canonical-remote registry shared by CLI, MCP, and og
- `internal/og/` — direct Git/forge policy, configuration, and typed domain operations
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

Prefer small tool-specific services over a generic CLI-to-MCP bridge. Keep MCP
results typed and structured; do not implement MCP by spawning the CLI or
parsing human-readable output. Extract shared adapter helpers only after the
same protocol boilerplate repeats across multiple tools without erasing their
different schemas, safety annotations, or targeting rules.

The approval-gated merge domain operation is the only supported merge operation;
its adapters are typed MCP (the agent default) and the structured-flag CLI.
Both adapters call the same Impri gate, and neither accepts an Impri API key or
bypasses web approval with GitHub, Forgejo, `gh`, or raw API tooling.

Agents must treat `PRMergeResult` as the merge feedback contract. Every retry
is the same `pr_merge` request with the exact `project`, PR ID, and mode; the
returned `action_id` is informational for audit and the web inbox, never an
input. Surface `inbox_url` for `status=pending` and wait. For an approved
temporary failure or local `status=unavailable` (approval state unknown; no
forge call), use `next_action=retry_same_request` and repeat that request
without new approval. `status=executed` with no receipt or cleanup error is
complete; rejected, expired, and
`execute_failed` are terminal and require new approval for another attempt.
If a deterministic `execute_failed` receipt could not be recorded, the result
stays approved and `retry_same_request` revalidates/reports it. An executed
result with `receipt_error` uses `next_action=repair_receipt`: repeat the same
request only to repair the receipt; it must not merge again. An executed result
with `cleanup_error` uses `next_action=retry_same_request`: the forge merge and
receipt are complete, so repeat the exact request to finish checkout cleanup and
never invoke the forge merge again. Follow `next_action`, `completion`, and
`detail` as the machine/readable contract and consider the operation complete
only when executed has neither receipt nor cleanup error. Impri provider/API
availability failures remain approved and retryable. For real execution, CI
state `success` or `not_configured` permits the forge gate; `pending` is
retryable, `failure`/`error` is terminal, and unknown or unverifiable state
fails closed. A successful real merge automatically pulls the registered
single-checkout default branch and removes matching approved local and origin
head refs; do not run a separate `og pull` afterward. Dry-run remains
non-destructive, and worktree coordination is outside this version.

The CLI/MCP transport boundary and its rationale are authoritative in the
README guidance. `og pull` remains available for its existing guarded closed-PR
workflow, while successful real merge cleanup is automatic.
Telegram, webhooks, a daemon, and Impri key provisioning or rotation are
outside this version.

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
make ci-scope SCOPE_CMD=project \
  SCOPE_PACKAGES='./cmd/project ./internal/project ./internal/config'
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
