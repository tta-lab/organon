---
name: organon-src
description: Use src to read and edit source files with symbol-aware navigation. Run `src <file>` for a tree, then `-s <id>` to read a symbol. Subcommands replace, insert, delete, comment modify symbols in-place. Use `src edit` for raw text replacement on any file type.
---

# src — Symbol-Aware Source File Reading and Editing

Use `src` to read code and markdown files without loading the entire file. Scan the symbol tree, then read or mutate specific symbols by ID.

Supported languages: Go, Rust, TypeScript, TSX, Python, C, C++, Java, Ruby, JavaScript, and many others via auto-inference. Markdown (.md, .markdown, .mdx) uses heading-based sections.

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

## edit — raw text replacement (any file type)

Use `src edit` when symbol-based editing is overkill: config files, unsupported languages, or when you already know the exact text to replace.

```bash
cat <<'EDIT' | src edit <file>
===BEFORE===
old text here
===AFTER===
new text here
EDIT
```

**When to use `edit` vs `replace -s`:**
- `replace -s <id>` — use for code symbols in supported languages (Go, Python, etc.). Precise, no text matching needed.
- `edit` — use for config files, unsupported file types, or when you want to replace a specific text fragment rather than a whole symbol.

**Matching strategy (4 passes, in order):**
1. Exact byte match
2. Trailing whitespace trimmed
3. Full whitespace trimmed (catches indentation drift)
4. Unicode folding (curly quotes → straight, em dashes → hyphen, etc.)

Single edit per invocation. Use multiple `src edit` calls for multiple replacements.
