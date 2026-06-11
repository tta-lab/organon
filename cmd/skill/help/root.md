Discover and read skills from agent skill directories on the filesystem.
Skills are directories containing a SKILL.md file with YAML frontmatter
(name, description, category). The frontmatter name is required and is
the only skill identity; directory names are storage locations, not aliases.

## Discovery paths (priority order, first match wins)
  1. {cwd}/.agents/skills
  2. {cwd}/.crush/skills
  3. {cwd}/.claude/skills
  4. {cwd}/.cursor/skills
  5. ~/.agents/skills
  6. ~/.crush/skills
  7. ~/.claude/skills
  8. ~/.cursor/skills

Project-local paths (1–4) take priority over global paths (5–8).
Within each scope: .agents > .crush > .claude > .cursor.

## Subcommands
  list   List all discovered skills as name, description, and SKILL.md path
  get    Print a skill's full body to stdout (frontmatter stripped)
  find   Search skills by keyword (case-insensitive OR match)
