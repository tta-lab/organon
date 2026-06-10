Replace an entire symbol (function, type, method, or markdown section)
by its 2-char ID. New content is read from stdin.

## When to use
  - Changing a whole function implementation
  - Replacing a type definition
  - Updating a markdown section

## When not to use
  - Small text fragments within a symbol (use edit)
  - Adding new symbols (use insert)

## Examples
  echo "func newImpl() {}" | src replace main.go -s aB
  cat new_type.go | src replace types.go -s cD
