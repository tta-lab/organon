---
name: organon-web
description: Use web to search the internet. Uses Brave Search API when BRAVE_API_KEY is set, falls back to DuckDuckGo. Default 10 results, max 20.
---

# web — Web Search

Use `web` to search the internet from the command line.

## Basic Usage

```bash
web "golang generics tutorial"
web "site:github.com go treesitter"
web "latest release of cobra cli" -n 5
```

## Flags

```bash
web <query>                   # search (10 results by default)
web <query> -n 5              # limit to 5 results
web <query> --max 20          # up to 20 results (hard max)
```

## Backends

- **Brave Search API** — set `BRAVE_API_KEY` in your environment for higher quality results
- **DuckDuckGo** — automatic fallback when `BRAVE_API_KEY` is not set

> **Note:** Setting `BRAVE_API_KEY` to an empty string returns an error. To use DuckDuckGo fallback, leave the variable unset entirely.
