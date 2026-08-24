Discover and read skills from agent skill directories on the filesystem.
Skills are directories containing a SKILL.md file with YAML frontmatter
(name, description, category). The frontmatter name is required and is
the only skill identity; directory names are storage locations, not aliases.

## Discovery path

The CLI searches these directories in order; a same-named current-directory
skill overrides a global one:

  ./.agents/skills
  ~/.agents/skills

## Subcommands
  list   List all discovered skills as name and description
  get    Print a skill's full body to stdout (frontmatter stripped)
  find   Find and rank skills for a natural-language query (default limit 8)
  mcp    Serve typed read-only skill discovery tools over stdio MCP
