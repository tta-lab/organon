Discover and read skills from agent skill directories on the filesystem.
Skills are directories containing a SKILL.md file with YAML frontmatter
(name, description, category). The frontmatter name is required and is
the only skill identity; directory names are storage locations, not aliases.

## Discovery path
  ~/.agents/skills

## Configuration

Additional directories can be configured in ~/.config/ttal/skills.toml.
Configured entries are appended after the built-in default, in the order
listed, so the default wins on name collisions. A leading ~ expands to
your home directory. Example:

    # Extra directories to search after ~/.agents/skills.
    global = ["~/work/skills", "/srv/shared-skills"]

## Subcommands
  list   List all discovered skills as name and description
  get    Print a skill's full body to stdout (frontmatter stripped)
  find   Find and rank skills for a natural-language query (default limit 8)
  mcp    Serve typed read-only skill discovery tools over stdio MCP
