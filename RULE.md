# Organon Quick Reference

Two CLI tools for AI agents: `src` (symbol-aware source reading/editing), `web` (web search and page fetching).

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

## web

```bash
web search "<query>"              # search (10 results)
web fetch <url>                   # fetch page (auto-tree if >5000 chars)
web fetch <url> --tree            # force heading tree
web fetch <url> -s <id>           # read section by ID
web fetch <url> --full            # full content, skip auto-tree
web fetch <url> --tree-threshold 8000  # custom auto-tree threshold
```

Search backends: `EXA_API_KEY` → Exa, `BRAVE_API_KEY` → Brave, fallback → DuckDuckGo. (Empty key causes error — unset entirely for the next backend.)

Fetch backend: `BROWSER_GATEWAY_URL` → browser gateway (no cache). Otherwise defuddle CLI with daily cache at `~/.cache/organon/scrapes/`.
