Serve read-only skill discovery tools over stdio using the Model Context Protocol.

Global skills are searched when no project is provided. When a registered project
alias is provided, its project-local skill directories take priority over global
directories. The project registry is reloaded for every request.
Individual SKILL.md files larger than 1 MiB are rejected before parsing.

## Tools
  skill_list   List discovered skill metadata
  skill_find   Find skills by case-insensitive keyword OR match
  skill_get    Read one skill by its exact frontmatter name
