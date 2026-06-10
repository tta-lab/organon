Fetch a web page and render it as markdown. Long pages (>5000 chars)
auto-show a heading tree so you can read specific sections.

## Two-step workflow for long pages
  1. web fetch https://docs.example.com/api       # shows heading tree
  2. web fetch https://docs.example.com/api -s 3f  # read one section

## Fetch backends
  BROWSER_GATEWAY_URL set   → browser gateway (JS-rendered, no cache)
  BROWSER_GATEWAY_URL unset → defuddle CLI (daily disk cache at ~/.cache/organon/scrapes/)
