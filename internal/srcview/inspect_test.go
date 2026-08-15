package srcview

import (
	"strings"
	"testing"
)

func TestInspectorCodeOutlineAndDocRead(t *testing.T) {
	source := []byte("package sample\n\n// Greet says hello.\n" +
		"func Greet(name string) string {\n\treturn \"hello \" + name\n}\n")
	inspector := NewInspector("sample.go", source, 2)
	outline, err := inspector.Outline()
	if err != nil {
		t.Fatal(err)
	}
	if outline.Language != "go" || len(outline.Symbols) != 1 {
		t.Fatalf("outline = %#v", outline)
	}
	symbol := outline.Symbols[0]
	if !symbol.Targetable || !symbol.HasDoc || symbol.Kind != "function" || symbol.StartByte <= 0 {
		t.Fatalf("symbol = %#v", symbol)
	}
	content, err := inspector.ReadContent(symbol.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(content, "// Greet says hello.") || !strings.Contains(content, "return \"hello \"") {
		t.Fatalf("content = %q", content)
	}
}

func TestInspectorMarkdownTargetsH1AndNestedSections(t *testing.T) {
	source := []byte("# Guide\n\n## Setup\n\nInstall.\n\n### Linux\n\nRun.\n")
	outline, err := NewInspector("guide.md", source, 2).Outline()
	if err != nil {
		t.Fatal(err)
	}
	if outline.Language != "markdown" || outline.Title != "Guide" || len(outline.Symbols) != 3 {
		t.Fatalf("outline = %#v", outline)
	}
	h1 := outline.Symbols[0]
	if !h1.Targetable || h1.ID == "" {
		t.Fatalf("H1 = %#v, want targetable", h1)
	}
	document, err := NewInspector("guide.md", source, 2).ReadContent(h1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if document != string(source) {
		t.Fatalf("H1 read = %q, want whole document", document)
	}
	setup := outline.Symbols[1]
	if !setup.Targetable || setup.Kind != "section" || setup.Parent != "Guide" {
		t.Fatalf("setup = %#v", setup)
	}
	content, err := NewInspector("guide.md", source, 2).ReadContent(setup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "### Linux") {
		t.Fatalf("section read = %q", content)
	}
}

func TestInspectorMarkdownUsesFirstH1AsTitleEvenAfterEarlierHeading(t *testing.T) {
	source := []byte("## Preface\n\nText.\n\n# Guide\n")
	outline, err := NewInspector("guide.md", source, 2).Outline()
	if err != nil {
		t.Fatal(err)
	}
	if outline.Title != "Guide" {
		t.Fatalf("title = %q, want Guide", outline.Title)
	}
}
