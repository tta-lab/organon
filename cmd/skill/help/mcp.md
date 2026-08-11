Serve read-only skill discovery tools over stdio using the Model Context Protocol.

Skills are discovered from the global ~/.agents/skills directory.
Individual SKILL.md files larger than 1 MiB are rejected before parsing.

Extra directories can be configured in ~/.config/ttal/skills.toml
(global = [...]) and are searched after the default. The config file
is reloaded on every request, so edits take effect without a restart.

The `source` field of each result is the absolute path of the discovery
directory the skill came from.

## Tools
  skill_list   List discovered skill metadata
  skill_find   Find and rank skills for a natural-language query
  skill_get    Read one skill by its exact frontmatter name
