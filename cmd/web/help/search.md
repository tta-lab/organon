Search the web and return ranked results with titles, URLs, and snippets.
Results limited to 10. Use quotes for exact phrases.

Choose a provider for one invocation with `--provider`:

  web search "your query" --provider exa

Supported providers are `exa` and `brave`. Exa requires `EXA_API_KEY`; Brave
requires `BRAVE_API_KEY`. Keys can be set in the environment or in
`~/.config/ttal/.env`.

Without `--provider`, backends are selected in this order:
  `EXA_API_KEY` set     → Exa (highest quality)
  `BRAVE_API_KEY` set   → Brave Search API
  Neither set           → error

Setting a key to an empty string returns an error. Leave `EXA_API_KEY` unset to
use Brave; configure one of the two keys before searching.
