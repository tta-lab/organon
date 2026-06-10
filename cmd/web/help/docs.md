Look up library documentation via Context7 API.

## Two-step workflow
  1. web docs resolve <library-name>     # find library IDs
  2. web docs fetch <library-id> <topic>  # read docs

## Backend
  CONTEXT7_API_KEY set   → authenticated, higher rate limits
  CONTEXT7_API_KEY unset → anonymous, rate limited
