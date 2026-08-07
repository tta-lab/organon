Serve read-only source inspection tools over stdio using the Model Context
Protocol. Every call requires an exact registered project alias and a
repository-relative file path. The project registry is reloaded for every call.

## Tools
  symbols   Inspect code symbols or Markdown headings and obtain stable IDs
  read      Read one symbol/section by ID or a bounded UTF-8 byte range

This server has no edit tools and no revision or hash argument. Use normal
workspace editing tools when a file needs to change. Source files larger than
16 MiB are rejected before reading.
