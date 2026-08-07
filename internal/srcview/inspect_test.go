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
	read, err := inspector.Read(symbol.ID, 32)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(read.Content, "// Greet says hello.") || !read.Truncated {
		t.Fatalf("read = %#v", read)
	}
}

func TestInspectorMarkdownExposesUntargetableH1AndStableSections(t *testing.T) {
	source := []byte("# Guide\n\n## Setup\n\nInstall.\n\n### Linux\n\nRun.\n")
	outline, err := NewInspector("guide.md", source, 2).Outline()
	if err != nil {
		t.Fatal(err)
	}
	if outline.Language != "markdown" || outline.Title != "Guide" || len(outline.Symbols) != 3 {
		t.Fatalf("outline = %#v", outline)
	}
	if outline.Symbols[0].Targetable || outline.Symbols[0].ID != "" {
		t.Fatalf("H1 = %#v, want untargetable", outline.Symbols[0])
	}
	setup := outline.Symbols[1]
	if !setup.Targetable || setup.Kind != "section" || setup.Parent != "Guide" {
		t.Fatalf("setup = %#v", setup)
	}
	read, err := NewInspector("guide.md", source, 2).Read(setup.ID, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read.Content, "### Linux") {
		t.Fatalf("section read = %q", read.Content)
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
