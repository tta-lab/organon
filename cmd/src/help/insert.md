Insert new content before or after a symbol by its 2-char ID.
Content is read from stdin. Exactly one of --after or --before is required.

## When to use
  - Adding a new import or constant before an existing block
  - Inserting a new function in a specific position
  - Prepending or appending to a markdown section

## Examples
  echo "// new block" | src insert main.go --after aB
  cat new_func.go | src insert main.go --before aB

## Output
Colored diff of the change, then updated symbol tree.
