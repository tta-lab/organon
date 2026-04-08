---
name: organon-web
description: Use web to search the internet or fetch web pages. Search uses Exa when EXA_API_KEY is set, Brave when BRAVE_API_KEY is set, or falls back to DuckDuckGo. Fetch reads pages as markdown with heading-based navigation.
---

# web — Web Search and Page Fetching

Use `web` to search the internet or read web pages from the command line.

## Search

```bash
web search "golang generics tutorial"
web search "site:github.com go treesitter"
web search "latest release of cobra cli"
```

Returns up to 10 results with title, URL, and snippet.

## Fetch

```bash
web fetch https://example.com            # fetch and render as markdown
web fetch https://example.com --tree     # force heading tree view
web fetch https://example.com --full     # full content, no auto-tree
```

Long pages (>5000 chars) auto-show a heading tree. Use `-s` to read specific sections.

### Two-Step Pattern for Long Pages

#### 1. Get heading tree

```bash
web fetch https://docs.example.com/api
```

If content exceeds 5000 characters, automatically shows a heading tree:

```
├─ [aB] ## Installation
├─ [cD] ## Configuration
│  └─ [eF] ### Options
└─ [gH] ## API Reference
```

#### 2. Read a section

```bash
web fetch https://docs.example.com/api -s cD
```

## Fetch Flags

```bash
web fetch <url>                          # fetch (auto-tree if long)
web fetch <url> --tree                   # force tree view
web fetch <url> -s <id>                  # read section by ID (2-char base62)
web fetch <url> --full                   # full content, skip auto-tree
web fetch <url> --tree-threshold 8000    # customize auto-tree threshold (default: 5000)
```

## Search Backends

- **Exa** — set `EXA_API_KEY` for highest quality results (used first)
- **Brave Search API** — set `BRAVE_API_KEY` for good results (used when no Exa key)
- **DuckDuckGo** — automatic fallback when neither key is set

> **Note:** Setting a key to an empty string returns an error. To use the next backend, leave the variable unset entirely.

## Fetch Backend

- **`BROWSER_GATEWAY_URL` set** — fetches via browser gateway (JavaScript-rendered pages, no cache)
- **`BROWSER_GATEWAY_URL` unset** — uses `defuddle` CLI with daily disk cache at `~/.cache/organon/scrapes/`

## Docs — Context7 Library Documentation

Resolve library names to Context7 IDs and fetch documentation via a two-step workflow.

### Resolve

```bash
web docs resolve react        # list libraries matching "react"
```

Returns a numbered list of candidates with ID, trust score, snippet count, and available versions. Pick an ID and pass it to `fetch`.

### Fetch

```bash
web docs fetch /reactjs/react.dev hooks
web docs fetch reactjs/react.dev "how to handle errors & retries"
web docs fetch /reactjs/react.dev/18.2.0 --tokens 400
```

- `<library-id>` may be passed with or without the leading `/`
- `[topic]` is freeform natural language
- `--tokens N` limits response length (0 = backend default)
- Pin a specific version by using the version-suffixed ID from `resolve`

### Docs Backends

- **`CONTEXT7_API_KEY` set** — higher rate limits
- **`CONTEXT7_API_KEY` unset** — anonymous access (rate limited)

> **Note:** Setting `CONTEXT7_API_KEY=""` returns an error. Leave it unset for anonymous access.
