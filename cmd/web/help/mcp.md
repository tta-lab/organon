Run a typed MCP server over stdin/stdout for web search, page fetches,
Context7 documentation, and Sourcegraph code search.

Select the provider used by every search call with `--provider`:

  web mcp --provider brave

Supported providers are `exa`, `brave`, and `duckduckgo`. Without the flag,
search backends use the `EXA_API_KEY` → `BRAVE_API_KEY` → DuckDuckGo fallback.
The server loads the ttal environment once at startup; restart it after changing
environment variables. Every tool is read-only but performs open-world network
access.

Tools:

  search          # search the web and return typed ranked results
  fetch           # fetch and render a web page as Markdown
  docs_resolve    # resolve a library name to Context7 IDs
  docs_fetch      # fetch Context7 documentation for one library ID
  sgraph_search   # search public source code through Sourcegraph

Example MCP client configuration:

```json
{
  "mcpServers": {
    "organon-web": {
      "command": "web",
      "args": ["mcp", "--provider", "brave"]
    }
  }
}
```

The server writes MCP protocol messages only to stdout. Diagnostics go to
stderr.
