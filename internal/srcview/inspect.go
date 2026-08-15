package srcview

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tta-lab/organon/internal/markdown"
	"github.com/tta-lab/organon/internal/tree"
	"github.com/tta-lab/organon/internal/treesitter"
)

// ErrNoStructure reports a supported read with no targetable symbol structure.
var ErrNoStructure = errors.New("file does not have a symbol tree")

// Symbol is one source declaration or Markdown section in an outline.
type Symbol struct {
	ID         string `json:"id"`
	Targetable bool   `json:"targetable"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Parent     string `json:"parent"`
	Level      int    `json:"level"`
	StartByte  int    `json:"start_byte"`
	EndByte    int    `json:"end_byte"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	HasDoc     bool   `json:"has_doc"`
	readStart  int
}

// Outline is the typed structure of one source file.
type Outline struct {
	Language string   `json:"language"`
	Title    string   `json:"title,omitempty"`
	Symbols  []Symbol `json:"symbols"`
}

// Inspector reads structure from caller-trusted filename and source bytes.
type Inspector struct {
	filename string
	source   []byte
	depth    int
}

// NewInspector returns an inspector over trusted in-memory source.
func NewInspector(filename string, source []byte, depth int) *Inspector {
	if depth < 1 {
		depth = 2
	}
	return &Inspector{filename: filename, source: source, depth: depth}
}

// IsMarkdown reports whether filename uses Markdown section structure.
func IsMarkdown(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".md" || ext == ".markdown" || ext == ".mdx" || ext == ".tpl"
}

// Outline extracts the file's typed symbol or heading structure. It preserves
// ErrNoStructure for supported code files with no targetable symbols so the
// outline command can keep its existing user-facing behavior.
func (i *Inspector) Outline() (Outline, error) {
	return i.outline(false)
}

// OutlineAllowEmpty extracts a typed outline for a post-mutation result. A
// supported source file with no remaining symbols is represented by an empty
// Symbols slice instead of ErrNoStructure; unsupported file types still fail.
func (i *Inspector) OutlineAllowEmpty() (Outline, error) {
	return i.outline(true)
}

func (i *Inspector) outline(allowEmpty bool) (Outline, error) {
	if IsMarkdown(i.filename) {
		title, headings, err := markdown.Outline(i.source)
		if err != nil {
			return Outline{}, err
		}
		symbols := make([]Symbol, 0, len(headings))
		for _, heading := range headings {
			symbols = append(symbols, Symbol{
				ID: heading.ID, Targetable: heading.Targetable, Name: heading.Name, Kind: "section",
				Parent: heading.Parent, Level: heading.Level, StartByte: heading.StartByte,
				EndByte: heading.EndByte, StartLine: heading.StartLine, EndLine: heading.EndLine,
				readStart: heading.StartByte,
			})
		}
		return Outline{Language: "markdown", Title: title, Symbols: symbols}, nil
	}

	language, err := treesitter.LangNameFromExt(i.filename)
	if err != nil {
		return Outline{}, fmt.Errorf("%w: %s", ErrNoStructure, i.filename)
	}
	extracted, err := treesitter.ExtractSymbols(i.filename, i.source, i.depth)
	if err != nil {
		return Outline{}, err
	}
	nodes := treesitter.SymbolTree(extracted)
	if len(nodes) == 0 && !allowEmpty {
		return Outline{}, fmt.Errorf("%w: %s", ErrNoStructure, i.filename)
	}
	symbols := make([]Symbol, 0, len(extracted))
	for index, extractedSymbol := range extracted {
		readStart := int(extractedSymbol.StartByte)
		if extractedSymbol.DocStart >= 0 {
			readStart = extractedSymbol.DocStart
		}
		symbols = append(symbols, Symbol{
			ID: nodes[index].ID, Targetable: true, Name: extractedSymbol.Name, Kind: extractedSymbol.Kind,
			Parent: extractedSymbol.Parent, Level: extractedSymbol.Level,
			StartByte: int(extractedSymbol.StartByte), EndByte: int(extractedSymbol.EndByte),
			StartLine: extractedSymbol.StartLine, EndLine: extractedSymbol.EndLine,
			HasDoc: extractedSymbol.DocStart >= 0, readStart: readStart,
		})
	}
	return Outline{Language: language, Symbols: symbols}, nil
}

// ReadContent returns a full targetable symbol or Markdown section, including
// an attached code doc comment.
func (i *Inspector) ReadContent(symbolID string) (string, error) {
	if IsMarkdown(i.filename) {
		return markdown.ReadSection(i.source, symbolID)
	}
	outline, err := i.Outline()
	if err != nil {
		return "", err
	}
	for _, symbol := range outline.Symbols {
		if symbol.ID == symbolID && symbol.Targetable {
			return string(i.source[symbol.readStart:symbol.EndByte]), nil
		}
	}
	return "", fmt.Errorf("symbol %q not found", symbolID)
}

// RenderTree renders the inspector outline in the existing CLI format.
func (i *Inspector) RenderTree() (string, error) {
	if IsMarkdown(i.filename) {
		return markdown.HeadingTree(i.source)
	}
	extracted, err := treesitter.ExtractSymbols(i.filename, i.source, i.depth)
	if err != nil {
		return "", err
	}
	nodes := treesitter.SymbolTree(extracted)
	if len(nodes) == 0 {
		return "", fmt.Errorf("%w: %s", ErrNoStructure, i.filename)
	}
	return tree.Render(nodes), nil
}
