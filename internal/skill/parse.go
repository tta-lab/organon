package skill

import (
	"bytes"

	"github.com/adrg/frontmatter"
)

// Meta holds the parsed frontmatter fields from a SKILL.md file.
type Meta struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Category    string `yaml:"category"`
}

// ParseFrontmatter extracts YAML frontmatter from content.
// On parse error, returns empty Meta and the original content (never panics).
func ParseFrontmatter(content []byte) (Meta, []byte) {
	var meta Meta
	rest, err := frontmatter.Parse(bytes.NewReader(content), &meta)
	if err != nil {
		return Meta{}, bytes.TrimSpace(content)
	}
	return meta, bytes.TrimSpace(rest)
}
