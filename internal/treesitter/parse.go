package treesitter

import (
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// ParseFile parses source bytes using tree-sitter for the given filename.
// Returns the parsed tree and language name.
func ParseFile(filename string, source []byte) (tree *gotreesitter.Tree, langName string, err error) {
	entry := grammars.DetectLanguage(filename)
	if entry == nil {
		return nil, "", fmt.Errorf("unsupported file type: %s", filename)
	}

	lang := entry.Language()
	if lang == nil {
		return nil, "", fmt.Errorf("language grammar unavailable for: %s", filename)
	}

	// Some grammars with external scanners may panic — recover gracefully.
	defer func() {
		if r := recover(); r != nil {
			tree = nil
			langName = ""
			err = fmt.Errorf("parse %s: grammar panicked: %v", filename, r)
		}
	}()

	parser := gotreesitter.NewParser(lang)
	// defaultTokenSourceFactory returns nil for languages without an external scanner
	// implementation (TypeScript, TSX, and others hit the default: case). When nil,
	// fall back to parser.Parse() which uses the DFA lexer. Template literals and
	// scanner-dependent constructs may parse as errors in this mode.
	if entry.TokenSourceFactory != nil {
		ts := entry.TokenSourceFactory(source, lang)
		tree, err = parser.ParseWithTokenSource(source, ts)
	} else {
		tree, err = parser.Parse(source)
	}
	if err != nil {
		return nil, "", fmt.Errorf("parse %s: %w", filename, err)
	}
	if tree == nil {
		return nil, "", fmt.Errorf("parse %s: returned nil tree", filename)
	}

	return tree, entry.Name, nil
}

// LangNameFromExt returns the language name string for a filename.
// Used by srcop to detect comment style.
func LangNameFromExt(filename string) (string, error) {
	entry := grammars.DetectLanguage(filename)
	if entry == nil {
		ext := filename
		if idx := strings.LastIndex(filename, "."); idx >= 0 {
			ext = filename[idx:]
		}
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}
	return entry.Name, nil
}
