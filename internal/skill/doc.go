// Package skill provides filesystem-based skill discovery, frontmatter parsing,
// and ranked search shared by CLI and MCP adapters.
//
// Skills are directories containing a SKILL.md file with YAML frontmatter.
// Discovery walks multiple prioritized paths and deduplicates by skill name.
//
// Plane: shared
package skill
