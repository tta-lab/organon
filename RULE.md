# Organon Quick Reference

CLI tools for AI agents: `src` (symbol-aware source reading/editing), `skill` (filesystem-based skill discovery), and `project` (project management).

## src

```bash
src <file>                           # symbol tree (depth 2)
src <file> --depth 3                 # deeper tree
src <file> -s <id>                   # read symbol by ID
src <file> --tree                    # force tree view

echo "..." | src replace <file> -s <id>      # replace symbol (stdin)
echo "..." | src insert <file> --after <id>  # insert after symbol
echo "..." | src insert <file> --before <id> # insert before symbol
src delete <file> -s <id>                    # delete symbol
src comment <file> -s <id> --read            # read doc comment
echo "// doc" | src comment <file> -s <id>  # write doc comment
cat <<'EDIT' | src edit <file>               # text replace (===BEFORE===/===AFTER===)
===BEFORE===
old text
===AFTER===
new text
EDIT
```

Markdown files (.md, .markdown, .mdx) use heading-based sections (not tree-sitter). `comment` not supported for markdown.

`src edit` works on any text file regardless of language support — escape hatch for config files, unsupported languages, or targeted text replacement without symbol IDs.

## skill

```bash
skill list                          # list all discovered skills
skill get <name>                    # print skill body to stdout
skill find <keyword>...             # find skills by keyword (OR match)
```

Discovery path: global `~/.agents/skills`. Extra directories can be added via `~/.config/ttal/skills.toml` (`global = [...]`), searched after the default. Skills are directories containing `SKILL.md` with YAML frontmatter.
