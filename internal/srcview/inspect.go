package srcview

import (
	"bytes"
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

// ReadResult is a bounded symbol read with byte offsets into the file.
type ReadResult struct {
	Content   string
	Start     int
	End       int
	Truncated bool
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

// Outline extracts the file's typed symbol or heading structure.
func (i *Inspector) Outline() (Outline, error) {
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
	if len(nodes) == 0 {
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

// Read reads a targetable symbol or section, including an attached code doc comment.
func (i *Inspector) Read(symbolID string, limit int) (ReadResult, error) {
	outline, err := i.Outline()
	if err != nil {
		return ReadResult{}, err
	}
	for _, symbol := range outline.Symbols {
		if symbol.ID != symbolID || !symbol.Targetable {
			continue
		}
		end := symbol.EndByte
		truncated := false
		if limit > 0 && end-symbol.readStart > limit {
			end = symbol.readStart + limit
			for end > symbol.readStart && !isUTF8Boundary(i.source, end) {
				end--
			}
			if end == symbol.readStart {
				return ReadResult{}, fmt.Errorf("limit %d is too small for the next UTF-8 character", limit)
			}
			truncated = true
		}
		return ReadResult{
			Content: string(i.source[symbol.readStart:end]), Start: symbol.readStart, End: end, Truncated: truncated,
		}, nil
	}
	return ReadResult{}, fmt.Errorf("symbol %q not found", symbolID)
}

// SymbolRead is a line-oriented read of one symbol or Markdown section.
type SymbolRead struct {
	Content    string
	StartLine  int // 1-indexed line of the symbol/section start
	TotalLines int
}

// ReadSymbolLines returns the content of a targetable symbol or Markdown
// section together with its 1-indexed start line and total line count, using
// the same content boundaries as ReadContent.
func (i *Inspector) ReadSymbolLines(symbolID string) (SymbolRead, error) {
	if IsMarkdown(i.filename) {
		start, end, err := markdown.SectionBounds(i.source, symbolID)
		if err != nil {
			return SymbolRead{}, err
		}
		content := string(i.source[start:end])
		return SymbolRead{
			Content:    content,
			StartLine:  lineNumberAt(i.source, start),
			TotalLines: bytes.Count([]byte(content), []byte("\n")) + 1,
		}, nil
	}
	result, err := i.Read(symbolID, 0)
	if err != nil {
		return SymbolRead{}, err
	}
	return SymbolRead{
		Content:    result.Content,
		StartLine:  lineNumberAt(i.source, result.Start),
		TotalLines: bytes.Count([]byte(result.Content), []byte("\n")) + 1,
	}, nil
}

func lineNumberAt(source []byte, offset int) int {
	return bytes.Count(source[:offset], []byte("\n")) + 1
}

// ReadContent preserves the established CLI representation of a full symbol or section.
func (i *Inspector) ReadContent(symbolID string) (string, error) {
	if IsMarkdown(i.filename) {
		return markdown.ReadSection(i.source, symbolID)
	}
	result, err := i.Read(symbolID, 0)
	if err != nil {
		return "", err
	}
	return result.Content, nil
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

func isUTF8Boundary(source []byte, offset int) bool {
	return offset == 0 || offset == len(source) || source[offset]&0xc0 != 0x80
}
