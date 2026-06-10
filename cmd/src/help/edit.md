Replace exactly matched text in any file using ===BEFORE===/===AFTER=== blocks.
One edit per invocation. Use multiple calls for multiple replacements.

## When to use
  - Config files (YAML, JSON, TOML, etc.)
  - Unsupported languages (no tree-sitter grammar)
  - Small targeted text changes where symbol replace would be overkill
  - Files without a symbol tree

## When not to use
  - Replacing a whole function/type in a supported language (use replace)
  - Inserting new content (use insert)
  - Deleting a symbol (use delete)

## Input format
  cat <<'EOF' | src edit path/to/file.go
  ===BEFORE===
  exact old text
  ===AFTER===
  new text
  EOF

## Scoped editing
  src edit path/to/file.go -s Ab
Limits the search to one symbol/section, eliminating ambiguity when the
same text appears in multiple places within a file.

## Matching strategy (4 tolerant passes)
  1. Exact byte match
  2. Trailing whitespace trimmed per line
  3. Full whitespace trimmed + auto-reindent to file style
  4. Unicode folding (curly quotes → straight, em dashes → hyphen, etc.)

When a non-exact pass fires, the match method and any reindent transform
are printed to stderr.

## File-based editing
  --before-file and --after-file read content from files instead of stdin.
  Both must be provided together.

## Output
Colored diff of old→new, then updated symbol tree (for supported files).
