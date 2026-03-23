---
name: organon-src
description: Use src to read and edit source files with symbol-aware navigation. Run `src <file>` for a tree, then `-s <id>` to read a symbol. Subcommands replace, insert, delete, comment modify symbols in-place.
---

# src — Symbol-Aware Source File Reading and Editing

Use `src` to read code and markdown files without loading the entire file. Scan the symbol tree, then read or mutate specific symbols by ID.

Supported languages: Go, Rust, TypeScript, Python, Markdown (.md, .markdown, .mdx).

## Two-Step Pattern

### 1. Scan symbol tree

```bash
src main.go
```

Shows a box-drawing tree of top-level symbols (depth 2 by default):

```
├─ [aB] func main
│  └─ [cD] var config
└─ [eF] func run
```

IDs are 2-character base62 (0–9, A–Z, a–z). Use `--depth N` to expand nested symbols.

### 2. Read a symbol

```bash
src main.go -s aB
```

Prints the full source of that symbol (including doc comment if present).

## Flags

```bash
src <file>                     # show symbol tree
src <file> --tree              # force tree view
src <file> --depth 3           # tree depth (default: 2)
src <file> -s <id>             # read symbol by ID
src <file> -s <id> --depth 3   # (depth inherited by subcommands too)
```

## Subcommands

### replace — replace a symbol (reads from stdin)

```bash
echo "func newFoo() {}" | src replace main.go -s aB
cat new_impl.go | src replace main.go -s aB
```

Requires `-s`. Prints updated symbol tree after write.

### insert — insert content before/after a symbol (stdin)

```bash
echo "// new block" | src insert main.go --after aB
echo "// new block" | src insert main.go --before aB
```

Exactly one of `--after` or `--before` is required. Prints updated tree after write.

### delete — delete a symbol

```bash
src delete main.go -s aB
```

Requires `-s`. Prints updated tree after delete.

### comment — read or write a doc comment

```bash
src comment main.go -s aB --read        # read existing doc comment
echo "// Foo does X" | src comment main.go -s aB  # write doc comment
```

Requires `-s`. Not supported for markdown files (use `replace -s` instead). Prints updated tree after write.

## Markdown Files

For `.md`, `.markdown`, `.mdx` files, `src` uses heading-based sections instead of tree-sitter symbols:

```bash
src README.md                  # heading tree
src README.md -s 3K            # read section by ID
echo "new content" | src replace README.md -s 3K  # replace section
src insert README.md --after 3K <<'EOF'
## New Section
content
EOF
src delete README.md -s 3K
```

`--tree` and `--depth` flags are no-ops for markdown. `comment` subcommand is not supported.
