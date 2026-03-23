# Organon Quick Reference

Three CLI tools for AI agents: `src` (symbol-aware source reading/editing), `url` (web page reading), `web` (web search).

## src

```bash
src <file>                           # symbol tree (depth 2)
src <file> --depth 3                 # deeper tree
src <file> -s <id>                   # read symbol by ID
src <file> --tree                    # force tree view

echo "..." | src replace <file> -s <id>      # replace symbol (stdin)
echo "..." | src insert <file> --after <id>  # insert after symbol
echo "..." | src insert <file> --before <id> # insert before symbol
src delete <file> -s <id>                    # delete symbol
src comment <file> -s <id> --read            # read doc comment
echo "// doc" | src comment <file> -s <id>  # write doc comment
```

Markdown files (.md, .markdown, .mdx) use heading-based sections (not tree-sitter). `comment` not supported for markdown.

## url

```bash
url <url>                            # fetch page (auto-tree if >5000 chars)
url <url> --tree                     # force heading tree
url <url> -s <id>                    # read section by ID
url <url> --full                     # full content, skip auto-tree
url <url> --tree-threshold 8000      # custom auto-tree threshold
```

Backend: `BROWSER_GATEWAY_URL` → browser gateway (no cache). Otherwise defuddle CLI with daily cache at `~/.cache/organon/scrapes/`.

## web

```bash
web "<query>"                        # search (10 results)
web "<query>" -n 5                   # limit results
web "<query>" --max 20               # max 20 results
```

Backend: `BRAVE_API_KEY` → Brave Search. Unset → DuckDuckGo fallback. (Empty string causes error — unset entirely for DDG.)
