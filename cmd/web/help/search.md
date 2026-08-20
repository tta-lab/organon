Search the web and return ranked results with titles, URLs, and snippets.
Results limited to 10. Use quotes for exact phrases.

Choose a provider for one invocation with `--provider`:

  web search "your query" --provider exa

Supported providers are `exa`, `brave`, and `duckduckgo`. Exa requires
`EXA_API_KEY`; Brave requires `BRAVE_API_KEY`. Keys can be set in the
environment or in `~/.config/ttal/.env`.

Without `--provider`, backends are selected in this order:
  `EXA_API_KEY` set     → Exa (highest quality)
  `BRAVE_API_KEY` set   → Brave Search API
  Neither set           → DuckDuckGo (free, no key needed)

Setting a key to an empty string returns an error. Leave the variable
unset to use the next backend.
