src reads and edits source files with symbol-aware navigation.
It understands the structure of source files — functions, types, methods — and
lets you navigate and modify them precisely without guessing line numbers.

## When to use
  - Scanning a file's structure before editing
  - Reading a specific function/type without loading the whole file
  - Editing symbols by ID (no text matching needed)
  - Small targeted text replacements in any file type

## When not to use
  - Grepping for text across files (use rg/grep)
  - Creating new files (use cat > file <<'EOF')

## Common workflow
  1. src path/to/file.go                    # inspect symbol tree
  2. src path/to/file.go --symbol-id Ab      # read one symbol by ID
  3. src replace path/to/file.go --symbol-id Ab < new.go  # replace it

## Supported languages
Go, Rust, TypeScript, TSX, Python, C, C++, Java, Ruby, JavaScript, and
others via tree-sitter auto-inference. Markdown (.md, .markdown, .mdx) uses
heading-based sections instead of tree-sitter symbols.

## Output
Tree view with 2-char base62 symbol IDs (0–9, A–Z, a–z), or full source of a
read symbol. Edits show a colored diff then the updated tree.
