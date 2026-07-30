Search the web and return ranked results with titles, URLs, and snippets.
Results limited to 10. Use quotes for exact phrases.

Set a search provider in ~/.config/ttal/web.toml:

  [search]
  provider = "exa" # exa, brave, or duckduckgo

When configured, only that provider is used. Exa requires EXA_API_KEY and
Brave requires BRAVE_API_KEY. Keys can be set in the environment or in
~/.config/ttal/.env.

Without a configured provider, backends are selected in this order:
  EXA_API_KEY set     → Exa (highest quality)
  BRAVE_API_KEY set   → Brave Search API
  Neither set         → DuckDuckGo (free, no key needed)

Setting a key to an empty string returns an error. Leave the variable
unset to use the next backend.
