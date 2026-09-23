package srcview

import (
	"strings"
	"testing"
)

func TestMachineSymbolsAndReadShareSelectionAndPagination(t *testing.T) {
	source := []byte("package sample\n\n// Greet says hello.\nfunc Greet() string { return \"hello\" }\n")
	outline, err := BuildSymbolResult("sample.go", source, false)
	if err != nil {
		t.Fatal(err)
	}
	if outline.Path != "sample.go" || outline.Language != "go" ||
		outline.TotalBytes != len(source) || len(outline.Symbols) != 1 {
		t.Fatalf("outline = %+v", outline)
	}
	id := outline.Symbols[0].ID
	first, err := BuildReadResult("sample.go", source, id, 0, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if first.Path != "sample.go" || first.SymbolID != id || first.Content != "// Greet says hello." ||
		first.NextOffset != 2 || first.TotalLines != 2 {
		t.Fatalf("first = %+v", first)
	}
	rest, err := BuildReadResult("sample.go", source, id, first.NextOffset, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rest.Content, "func Greet()") || rest.NextOffset != 0 {
		t.Fatalf("rest = %+v", rest)
	}
}

func TestMachineMarkdownAndEmptyOutline(t *testing.T) {
	source := []byte("# Guide\n\n## Setup\nText\n")
	outline, err := BuildSymbolResult("guide.md", source, false)
	if err != nil {
		t.Fatal(err)
	}
	if outline.Language != "markdown" || outline.Title != "Guide" || len(outline.Symbols) != 2 {
		t.Fatalf("outline = %+v", outline)
	}
	read, err := BuildReadResult("guide.md", source, outline.Symbols[1].ID, 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read.Content, "Text") {
		t.Fatalf("read = %+v", read)
	}
	empty, err := BuildSymbolResult("empty.go", []byte("package p\n"), true)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Symbols == nil || len(empty.Symbols) != 0 {
		t.Fatalf("empty outline = %+v", empty)
	}
}

func TestMachineSymbolsIncludeDepthTwoFields(t *testing.T) {
	source := []byte("package p\n\ntype Config struct {\n Name string\n}\n")
	outline, err := BuildSymbolResult("config.go", source, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, symbol := range outline.Symbols {
		if symbol.Name == "Name" && symbol.Level == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("depth-two field absent: %+v", outline.Symbols)
	}
}
