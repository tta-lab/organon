package skill

import (
	"bytes"
	"testing"
)

func TestParseFrontmatter_FullFrontmatter(t *testing.T) {
	content := []byte(`---
name: my-skill
description: A useful skill
category: tools
---
# Body content here`)
	meta, body := ParseFrontmatter(content)
	if meta.Name != "my-skill" {
		t.Errorf("Name = %q, want %q", meta.Name, "my-skill")
	}
	if meta.Description != "A useful skill" {
		t.Errorf("Description = %q, want %q", meta.Description, "A useful skill")
	}
	if meta.Category != "tools" {
		t.Errorf("Category = %q, want %q", meta.Category, "tools")
	}
	expectedBody := "# Body content here"
	if string(body) != expectedBody {
		t.Errorf("body = %q, want %q", string(body), expectedBody)
	}
}

func TestParseFrontmatter_NameOnly(t *testing.T) {
	content := []byte(`---
name: only-name
---
Some body`)
	meta, body := ParseFrontmatter(content)
	if meta.Name != "only-name" {
		t.Errorf("Name = %q, want %q", meta.Name, "only-name")
	}
	if meta.Description != "" {
		t.Errorf("Description = %q, want %q", meta.Description, "")
	}
	if meta.Category != "" {
		t.Errorf("Category = %q, want %q", meta.Category, "")
	}
	if !bytes.HasPrefix(body, []byte("Some body")) {
		t.Errorf("body = %q, want prefix %q", string(body), "Some body")
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := []byte(`# Just a plain markdown file`)
	meta, body := ParseFrontmatter(content)
	if meta.Name != "" || meta.Description != "" || meta.Category != "" {
		t.Errorf("expected empty Meta, got %+v", meta)
	}
	if string(body) != string(content) {
		t.Errorf("body = %q, want %q", string(body), string(content))
	}
}

func TestParseFrontmatter_MalformedYAML(t *testing.T) {
	content := []byte(`---
name: bad
  this is not valid yaml: [
---
# Body`)
	meta, body := ParseFrontmatter(content)
	if meta.Name != "" || meta.Description != "" || meta.Category != "" {
		t.Errorf("expected empty Meta on malformed YAML, got %+v", meta)
	}
	if string(body) != string(content) {
		t.Errorf("body = %q, want %q", string(body), string(content))
	}
}
