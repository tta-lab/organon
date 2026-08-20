# @tta-lab/dsh-web

DeepSeek Harness rc.8 Web bundle and browser settings client for Organon search.

Install it into the existing Web profile:

```bash
dsh plugin --profile web add @tta-lab/dsh-web
```

The package owns the Web profile patch and does not require a custom profile or
PTC preset. It leaves the root `tool-web` row disabled and routes stock PTC's
batched `web_search` through the Organon provider seam.

Provider selection is explicit and persists in the `organon-web` settings
namespace. Exa and Brave API keys use namespaced write-only DSH credentials;
settings only expose configured/source/writable metadata. DuckDuckGo needs no key.
