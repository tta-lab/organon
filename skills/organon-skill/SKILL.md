---
name: organon-skill
description: Filesystem-based skill discovery CLI — list, get, and find skills from agent skill directories.
---

# skill — Agent Skill Discovery CLI

Discover and read skills from agent skill directories. Skills are directories containing a `SKILL.md` file with YAML frontmatter.

## Discovery Paths (priority order)

Discovery walks 8 paths in priority order. First match wins on name conflict.

Project-local (checked first):
1. `$PWD/.agents/skills`
2. `$PWD/.crush/skills`
3. `$PWD/.claude/skills`
4. `$PWD/.cursor/skills`

Global (checked second):
5. `~/.agents/skills`
6. `~/.crush/skills`
7. `~/.claude/skills`
8. `~/.cursor/skills`

Two orthogonal rules:
- pwd beats global (project-local takes priority)
- within each scope, `.agents` > `.crush` > `.claude` > `.cursor`

## SKILL.md Format

A skill is a directory containing a `SKILL.md` file with YAML frontmatter:

    ~~~
    ---
    name: my-skill
    description: A one-sentence description
    category: tools
    ---
    # Skill body content here
    ~~~

Minimum fields: `name` and `description`. `category` is optional. If `name` is not set, the directory name is used.

## Commands

### list — list all discovered skills

```bash
skill list
```

Prints a table of all skills with name, category, source path, and description.

### get — print skill content

```bash
skill get my-skill
```

Prints the skill body (frontmatter stripped) to stdout. Agents pipe this into their context.

### find — search skills by keyword

```bash
skill find git
skill find task "command line"
```

Case-insensitive OR match across skill name and description. Prints same table as `list`.

## Output Format

`skill list` and `skill find` output a tab-separated table:

```
NAME                CATEGORY  SOURCE            DESCRIPTION
breathe             -         ~/.agents/skills  Refresh context window...
organon-src         -         ~/.agents/skills  Use src to read...
```

- Empty category renders as `-`
- Source path is abbreviated with `~` for `$HOME`
- Description truncated to 80 characters

## Design Notes

- No caching — filesystem is re-scanned on every invocation
- No environment variable overrides — fixed discovery paths
- Only directory-based `{name}/SKILL.md` format (not flat `{name}.md`)
