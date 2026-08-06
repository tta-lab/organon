Run a typed MCP server over stdin/stdout for web search, page fetches,
Context7 documentation, and Sourcegraph code search.

The server loads `~/.config/ttal/web.toml` and the ttal environment once at
startup. Restart it after changing configuration or environment variables.
Every tool is read-only but performs open-world network access.

Example MCP client configuration:

```json
{
  "mcpServers": {
    "organon-web": {
      "command": "web",
      "args": ["mcp"]
    }
  }
}
```

The server writes MCP protocol messages only to stdout. Diagnostics go to
stderr.
