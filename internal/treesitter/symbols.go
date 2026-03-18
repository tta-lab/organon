package treesitter

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/tta-lab/organon/internal/id"
	"github.com/tta-lab/organon/internal/tree"
)

//go:embed queries/*.scm
var queryFS embed.FS

// Symbol represents an extracted code symbol.
type Symbol struct {
	Name      string // e.g., "main", "Config", "Validate"
	Kind      string // e.g., "function", "type", "method", "field"
	Parent    string // e.g., "Config" for method Validate (empty for top-level)
	StartByte uint
	EndByte   uint
	StartLine int
	EndLine   int
	Level     int // 1 = top-level declaration, 2 = field/member
	DocStart  int // byte offset where doc comment starts (-1 if none)
}

// CanonicalName returns the label used for ID hashing.
// Includes parent context for methods: "Config.Validate".
func (s Symbol) CanonicalName() string {
	if s.Parent != "" {
		return fmt.Sprintf("%s %s.%s", s.Kind, s.Parent, s.Name)
	}
	return fmt.Sprintf("%s %s", s.Kind, s.Name)
}

// ExtractSymbols parses a file and returns its symbols up to maxDepth.
// Uses query-based extraction for Go, TypeScript, Python, Rust.
// Falls back to heuristic AST walker for other languages.
func ExtractSymbols(filename string, source []byte, maxDepth int) ([]Symbol, error) {
	parsedTree, langName, err := ParseFile(filename, source)
	if err != nil {
		return nil, err
	}
	defer parsedTree.Release()

	// Try query-based extraction first
	queryFile := fmt.Sprintf("queries/%s.scm", langName)
	if queryBytes, err := queryFS.ReadFile(queryFile); err == nil {
		return extractWithQuery(parsedTree, source, string(queryBytes), maxDepth)
	}

	// Fallback: heuristic walker
	return extractHeuristic(parsedTree, source, maxDepth)
}

func extractWithQuery(parsedTree *gotreesitter.Tree, source []byte, queryStr string, maxDepth int) ([]Symbol, error) {
	lang := parsedTree.Language()
	query, err := gotreesitter.NewQuery(queryStr, lang)
	if err != nil {
		return nil, fmt.Errorf("compile query: %w", err)
	}

	matches := query.Execute(parsedTree)

	var symbols []Symbol
	seenBytes := map[uint32]bool{} // avoid duplicates from overlapping patterns

	for _, match := range matches {
		// Index captures by name (last one wins for duplicates)
		captureMap := map[string]*gotreesitter.QueryCapture{}
		for i := range match.Captures {
			cap := &match.Captures[i]
			captureMap[cap.Name] = cap
		}

		declCap, isSymbol := captureMap["symbol.decl"]
		fieldCap, isField := captureMap["field.decl"]

		if !isSymbol && !isField {
			continue
		}

		var level int
		var declNode *gotreesitter.Node
		var nameCapKey string

		if isSymbol {
			level = 1
			declNode = declCap.Node
			nameCapKey = "symbol.name"
		} else {
			level = 2
			declNode = fieldCap.Node
			nameCapKey = "field.name"
		}

		if level > maxDepth {
			continue
		}

		// Avoid duplicates (same start byte processed already)
		if seenBytes[declNode.StartByte()] {
			continue
		}
		seenBytes[declNode.StartByte()] = true

		nameCap, ok := captureMap[nameCapKey]
		if !ok {
			continue // var_declaration / const_declaration with no name capture
		}

		name := string(source[nameCap.Node.StartByte():nameCap.Node.EndByte()])
		nodeType := declNode.Type(lang)
		kind := nodeTypeToKind(nodeType, isField)

		parent := ""
		if kind == "method" {
			parent = extractReceiverType(declNode, lang, source)
		}

		docStart := findDocComment(source, int(declNode.StartByte()))

		sym := Symbol{
			Name:      name,
			Kind:      kind,
			Parent:    parent,
			StartByte: uint(declNode.StartByte()),
			EndByte:   uint(declNode.EndByte()),
			StartLine: int(declNode.StartPoint().Row) + 1,
			EndLine:   int(declNode.EndPoint().Row) + 1,
			Level:     level,
			DocStart:  docStart,
		}
		symbols = append(symbols, sym)
	}

	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i].StartByte < symbols[j].StartByte
	})

	return symbols, nil
}

func extractHeuristic(parsedTree *gotreesitter.Tree, source []byte, maxDepth int) ([]Symbol, error) {
	lang := parsedTree.Language()
	root := parsedTree.RootNode()
	bt := gotreesitter.Bind(parsedTree)

	var symbols []Symbol

	for i := 0; i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		nameNode := child.ChildByFieldName("name", lang)
		if nameNode == nil {
			continue
		}

		name := bt.NodeText(nameNode)
		docStart := findDocComment(source, int(child.StartByte()))

		sym := Symbol{
			Name:      name,
			Kind:      "symbol",
			Level:     1,
			StartByte: uint(child.StartByte()),
			EndByte:   uint(child.EndByte()),
			StartLine: int(child.StartPoint().Row) + 1,
			EndLine:   int(child.EndPoint().Row) + 1,
			DocStart:  docStart,
		}
		symbols = append(symbols, sym)

		if maxDepth >= 2 {
			bodyNode := child.ChildByFieldName("body", lang)
			if bodyNode != nil {
				for j := 0; j < bodyNode.NamedChildCount(); j++ {
					field := bodyNode.NamedChild(j)
					fieldName := field.ChildByFieldName("name", lang)
					if fieldName == nil {
						continue
					}
					fsym := Symbol{
						Name:      bt.NodeText(fieldName),
						Kind:      "field",
						Parent:    name,
						Level:     2,
						StartByte: uint(field.StartByte()),
						EndByte:   uint(field.EndByte()),
						StartLine: int(field.StartPoint().Row) + 1,
						EndLine:   int(field.EndPoint().Row) + 1,
						DocStart:  -1,
					}
					symbols = append(symbols, fsym)
				}
			}
		}
	}

	return symbols, nil
}

// nodeTypeToKind maps a tree-sitter node type to our kind string.
func nodeTypeToKind(nodeType string, isField bool) string {
	if isField {
		return "field"
	}
	switch nodeType {
	case "function_declaration", "function_definition", "function_item":
		return "function"
	case "method_declaration", "method_definition":
		return "method"
	case "type_declaration", "type_alias_declaration":
		return "type"
	case "class_declaration", "class_definition":
		return "class"
	case "interface_declaration", "interface_declaration_statement", "trait_item":
		return "interface"
	case "struct_item":
		return "struct"
	case "enum_item":
		return "enum"
	case "impl_item":
		return "impl"
	default:
		return "symbol"
	}
}

// extractReceiverType extracts the receiver type name from a method_declaration node.
// For `(c *Config)` returns "Config", for `(c Config)` returns "Config".
func extractReceiverType(methodNode *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	receiverList := methodNode.ChildByFieldName("receiver", lang)
	if receiverList == nil {
		return ""
	}
	// receiver is a parameter_list: find first parameter_declaration
	for i := 0; i < receiverList.NamedChildCount(); i++ {
		param := receiverList.NamedChild(i)
		typeNode := param.ChildByFieldName("type", lang)
		if typeNode == nil {
			continue
		}
		typeName := typeNode.Type(lang)
		// Handle pointer receiver (*Config)
		if typeName == "pointer_type" {
			for j := 0; j < typeNode.NamedChildCount(); j++ {
				inner := typeNode.NamedChild(j)
				return string(source[inner.StartByte():inner.EndByte()])
			}
		}
		return string(source[typeNode.StartByte():typeNode.EndByte()])
	}
	return ""
}

// findDocComment scans backward from declStart to find consecutive comment lines.
// Returns the byte offset of the first comment line, or -1 if none found.
func findDocComment(source []byte, declStart int) int {
	// Find the start of the declaration line
	lineStart := declStart
	for lineStart > 0 && source[lineStart-1] != '\n' {
		lineStart--
	}

	commentStart := -1
	pos := lineStart - 1 // position just before the declaration line's newline

	for pos >= 0 {
		if pos >= 0 && source[pos] == '\n' {
			pos--
		}
		if pos < 0 {
			break
		}

		// Find the start of this line
		lineEnd := pos + 1
		lineBegin := pos
		for lineBegin > 0 && source[lineBegin-1] != '\n' {
			lineBegin--
		}

		line := strings.TrimSpace(string(source[lineBegin:lineEnd]))
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "/**") || strings.HasPrefix(line, " *") || strings.HasPrefix(line, "*/") {
			commentStart = lineBegin
			pos = lineBegin - 1
		} else if line == "" {
			break
		} else {
			break
		}
	}

	return commentStart
}

// SymbolTree builds tree.Node list from extracted symbols with assigned IDs.
func SymbolTree(symbols []Symbol) []tree.Node {
	labels := make([]string, len(symbols))
	for i, s := range symbols {
		labels[i] = s.CanonicalName()
	}
	ids := id.AssignIDs(labels)

	nodes := make([]tree.Node, len(symbols))
	for i, s := range symbols {
		lineRange := fmt.Sprintf("[L%d-L%d]", s.StartLine, s.EndLine)
		if s.StartLine == s.EndLine {
			lineRange = fmt.Sprintf("[L%d]", s.StartLine)
		}

		nodes[i] = tree.Node{
			ID:    ids[i],
			Label: formatSymbolLabel(s),
			Level: s.Level,
			Meta:  lineRange,
		}
	}
	return nodes
}

// formatSymbolLabel produces a human-readable label for tree display.
func formatSymbolLabel(s Symbol) string {
	switch s.Kind {
	case "function":
		return fmt.Sprintf("func %s()", s.Name)
	case "method":
		if s.Parent != "" {
			return fmt.Sprintf("func (%s) %s()", s.Parent, s.Name)
		}
		return fmt.Sprintf("func %s()", s.Name)
	case "type", "struct", "enum":
		return fmt.Sprintf("type %s", s.Name)
	case "class":
		return fmt.Sprintf("class %s", s.Name)
	case "interface":
		return fmt.Sprintf("interface %s", s.Name)
	case "impl":
		return fmt.Sprintf("impl %s", s.Name)
	case "field":
		return s.Name
	default:
		return s.Name
	}
}
