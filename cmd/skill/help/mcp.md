Serve read-only skill discovery tools over stdio using the Model Context Protocol.

Skills are discovered from the MCP process's startup directory and the user's
home directory, in order:

  ./.agents/skills
  ~/.agents/skills

The startup-directory skill wins when names collide.
Individual SKILL.md files larger than 1 MiB are rejected before parsing.

The `source` field of each result is the absolute path of the discovery
directory the skill came from.

## Tools
  skill_list   List discovered skill metadata
  skill_find   Find and rank skills for a natural-language query
  skill_get    Read one skill by its exact frontmatter name
