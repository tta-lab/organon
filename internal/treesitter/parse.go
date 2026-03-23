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
	// When a grammar has no registered token source factory (e.g. TypeScript, TSX),
	// fall back to the DFA lexer via Parse(). This handles the majority of syntax
	// without the external scanner (template literals etc. may parse as errors).
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
