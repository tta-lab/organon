---
name: organon-url
description: Use url to fetch web pages as markdown. For long pages, auto-shows a heading tree — then read specific sections with -s. Set BROWSER_GATEWAY_URL for live fetches, or use the default defuddle CLI with daily disk cache.
---

# url — Fetch Web Pages as Markdown

Use `url` to read web pages without loading the entire content into context. Long pages (>5000 chars) auto-show a heading tree; use `-s` to read specific sections.

## Basic Usage

```bash
url https://example.com            # fetch and render
url https://example.com --tree     # force heading tree view
url https://example.com --full     # full content, no auto-tree
```

## Two-Step Pattern for Long Pages

### 1. Get heading tree

```bash
url https://docs.example.com/api
```

If content exceeds 5000 characters, automatically shows a heading tree:

```
├─ [aB] ## Installation
├─ [cD] ## Configuration
│  └─ [eF] ### Options
└─ [gH] ## API Reference
```

### 2. Read a section

```bash
url https://docs.example.com/api -s cD
```

Prints the content of that heading section only.

## Flags

```bash
url <url>                          # fetch (auto-tree if long)
url <url> --tree                   # force tree view
url <url> -s <id>                  # read section by ID (2-char base62)
url <url> --full                   # full content, skip auto-tree
url <url> --tree-threshold 8000    # customize auto-tree threshold (default: 5000)
```

## Backend Resolution

- **`BROWSER_GATEWAY_URL` set** — fetches via browser gateway (JavaScript-rendered pages, no cache)
- **`BROWSER_GATEWAY_URL` unset** — uses `defuddle` CLI with daily disk cache at `~/.cache/organon/scrapes/`

The cache key is the URL + today's date. Cache is invalidated daily automatically.
