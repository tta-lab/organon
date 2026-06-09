Read an existing doc comment or write a new one on a code symbol.

## When to use
  - Reading the current doc comment before editing
  - Writing or replacing a doc comment

## When not to use
  - Markdown files (use replace --symbol-id instead)

## Examples
  src comment main.go --symbol-id aB --read                # read existing
  echo "// Foo does X" | src comment main.go --symbol-id aB  # write

## Output
For --read: the comment text only. For write: colored diff + updated tree.
