package treesitter

import (
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/tta-lab/organon/internal/id"
	"github.com/tta-lab/organon/internal/tree"
)

//go:embed queries/*.scm
var queryFS embed.FS

// Symbol kind constants — must match normalizeKind output.
const (
	kindMethod      = "method"
	kindFunction    = "function"
	kindClass       = "class"
	kindInterface   = "interface"
	kindType        = "type"
	kindImpl        = "impl"
	kindModule      = "module"
	kindConstant    = "constant"
	kindVariable    = "variable"
	kindMacro       = "macro"
	kindConstructor = "constructor"
	kindField       = "field"
)

// Symbol represents an extracted code symbol.
type Symbol struct {
	Name      string // e.g., "main", "Config", "Validate"
	Kind      string // e.g., "function", "type", kindMethod, "field"
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
// Query resolution priority:
// 1. Vendored upstream tags.scm (9 languages — embedded in queries/)
// 2. ResolveTagsQuery (auto-inferred for any language with a grammar)
// 3. Heuristic AST walker
// Depth-2 fields use heuristic body-walker in all query-based paths.
func ExtractSymbols(filename string, source []byte, maxDepth int) ([]Symbol, error) {
	parsedTree, langName, err := ParseFile(filename, source)
	if err != nil {
		return nil, err
	}
	defer parsedTree.Release()

	// Tier 1: vendored tags.scm
	queryFile := fmt.Sprintf("queries/%s.scm", langName)
	// TSX uses the same tags query as TypeScript
	if langName == "tsx" {
		queryFile = "queries/typescript.scm"
	}

	queryStr := ""
	if queryBytes, err := queryFS.ReadFile(queryFile); err == nil {
		queryStr = string(queryBytes)
	}

	// Tier 2: ResolveTagsQuery (auto-inferred from grammar symbols).
	// DetectLanguage is called a second time because ParseFile only returns langName;
	// the entry is not threaded through. This is safe — registry is read-only after init.
	if queryStr == "" {
		if entry := grammars.DetectLanguage(filename); entry != nil {
			queryStr = grammars.ResolveTagsQuery(*entry)
		}
		if queryStr == "" {
			slog.Debug("no tags query found, using heuristic", "file", filename, "lang", langName)
		}
	}

	// Use query-based extraction if we have a query
	if queryStr != "" {
		symbols, err := extractWithQuery(parsedTree, source, queryStr)
		if err != nil {
			// Query compile failed — fall through to heuristic and warn so the
			// caller knows output is degraded (heuristic labels everything "symbol").
			slog.Warn("query compile failed, using heuristic fallback",
				"file", filename, "lang", langName, "err", err)
			return extractHeuristic(parsedTree, source, maxDepth), nil
		}
		// Add depth-2 fields via heuristic body-walker, then infer method parents
		// for languages without Go-style receiver syntax (Java, TypeScript, etc.).
		if maxDepth >= 2 {
			fields := extractFields(parsedTree, symbols)
			symbols = append(symbols, fields...)
			sort.Slice(symbols, func(i, j int) bool {
				return symbols[i].StartByte < symbols[j].StartByte
			})
		}
		inferMethodParents(symbols)
		return symbols, nil
	}

	// Tier 3: heuristic walker (handles both levels)
	return extractHeuristic(parsedTree, source, maxDepth), nil
}

func extractWithQuery(parsedTree *gotreesitter.Tree, source []byte, queryStr string) ([]Symbol, error) {
	lang := parsedTree.Language()
	query, err := gotreesitter.NewQuery(queryStr, lang)
	if err != nil {
		return nil, fmt.Errorf("compile query: %w", err)
	}

	matches := query.Execute(parsedTree)

	var symbols []Symbol
	seenBytes := map[uint32]bool{}

	for _, match := range matches {
		captureMap := map[string]*gotreesitter.QueryCapture{}
		for i := range match.Captures {
			cap := &match.Captures[i]
			captureMap[cap.Name] = cap
		}

		// Find the @definition.* capture — skip @reference.* and others
		var defCap *gotreesitter.QueryCapture
		var kind string
		for capName, cap := range captureMap {
			if strings.HasPrefix(capName, "definition.") {
				defCap = cap
				kind = strings.TrimPrefix(capName, "definition.")
				break
			}
		}
		if defCap == nil {
			continue
		}

		nameCap, ok := captureMap["name"]
		if !ok {
			continue
		}

		if seenBytes[defCap.Node.StartByte()] {
			continue
		}
		seenBytes[defCap.Node.StartByte()] = true

		name := string(source[nameCap.Node.StartByte():nameCap.Node.EndByte()])

		// Normalize upstream kind names to organon conventions
		kind = normalizeKind(kind)

		parent := ""
		if kind == kindMethod {
			parent = extractReceiverType(defCap.Node, lang, source)
		}

		docStart := findDocComment(source, int(defCap.Node.StartByte()))

		sym := Symbol{
			Name:      name,
			Kind:      kind,
			Parent:    parent,
			StartByte: uint(defCap.Node.StartByte()),
			EndByte:   uint(defCap.Node.EndByte()),
			StartLine: int(defCap.Node.StartPoint().Row) + 1,
			EndLine:   int(defCap.Node.EndPoint().Row) + 1,
			Level:     1,
			DocStart:  docStart,
		}
		symbols = append(symbols, sym)
	}

	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i].StartByte < symbols[j].StartByte
	})

	return symbols, nil
}

// normalizeKind maps upstream tags.scm definition kinds to organon conventions.
func normalizeKind(kind string) string {
	switch kind {
	case "function":
		return kindFunction
	case "method":
		return kindMethod
	case "class":
		return kindClass
	case "interface":
		return kindInterface
	case "type":
		return kindType
	case "impl":
		return kindImpl
	case "module":
		return kindModule
	case "constant":
		return kindConstant
	case "variable":
		return kindVariable
	case "macro":
		return kindMacro
	case "constructor":
		return kindConstructor
	case "field":
		return kindField
	default:
		return "symbol"
	}
}

func extractHeuristic(parsedTree *gotreesitter.Tree, source []byte, maxDepth int) []Symbol {
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

	return symbols
}

// extractFields extracts depth-2 field symbols from level-1 parent symbols.
// Uses a heuristic body-walker: finds nodes with a "name" field inside the
// body or field-list of each parent's root-level container node.
// Handles both direct-root definitions (Python, Java) and nested definitions
// like Go's type_spec inside type_declaration.
func extractFields(parsedTree *gotreesitter.Tree, parents []Symbol) []Symbol {
	lang := parsedTree.Language()
	root := parsedTree.RootNode()
	bt := gotreesitter.Bind(parsedTree)

	// Build lookup from parent start byte → parent symbol name.
	// Also build a set of L1 start bytes so we can skip adding fields that were
	// already captured as L1 symbols (e.g. TSX class methods appear as both
	// @definition.method at L1 and as named children of the class body).
	parentByByte := make(map[uint]string, len(parents))
	l1StartBytes := make(map[uint]bool, len(parents))
	for i := range parents {
		parentByByte[parents[i].StartByte] = parents[i].Name
		l1StartBytes[parents[i].StartByte] = true
	}

	var fields []Symbol

	for i := 0; i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		childStart := uint(child.StartByte())
		childEnd := uint(child.EndByte())

		// Exact start byte match (definition IS the root child)
		parentName, found := parentByByte[childStart]
		if !found {
			// Containment match: definition is nested inside root child
			// (e.g. Go's type_spec nested inside type_declaration)
			for _, p := range parents {
				if p.StartByte > childStart && p.StartByte < childEnd {
					parentName = p.Name
					found = true
					break
				}
			}
		}
		if !found {
			continue
		}

		// Find the member container (body or field-list) within the root child
		container := findMemberContainer(child, lang)
		if container == nil {
			continue
		}

		for j := 0; j < container.NamedChildCount(); j++ {
			field := container.NamedChild(j)
			fieldStart := uint(field.StartByte())
			// Skip symbols already captured as L1 (e.g. TSX class methods)
			if l1StartBytes[fieldStart] {
				continue
			}
			fieldName := field.ChildByFieldName("name", lang)
			if fieldName == nil {
				continue
			}
			fields = append(fields, Symbol{
				Name:      bt.NodeText(fieldName),
				Kind:      kindField,
				Parent:    parentName,
				Level:     2,
				StartByte: fieldStart,
				EndByte:   uint(field.EndByte()),
				StartLine: int(field.StartPoint().Row) + 1,
				EndLine:   int(field.EndPoint().Row) + 1,
				DocStart:  -1,
			})
		}
	}

	return fields
}

// inferMethodParents sets Parent on method symbols that lack one by finding the
// class or interface that contains them (by byte range). This handles languages
// without Go-style receiver syntax (Java, TypeScript, C++ member functions).
func inferMethodParents(symbols []Symbol) {
	for i := range symbols {
		if symbols[i].Kind != kindMethod || symbols[i].Parent != "" {
			continue
		}
		ms, me := symbols[i].StartByte, symbols[i].EndByte
		for _, container := range symbols {
			if (container.Kind == kindClass || container.Kind == kindInterface) &&
				ms > container.StartByte && me <= container.EndByte {
				symbols[i].Parent = container.Name
				break
			}
		}
	}
}

// findMemberContainer locates the body or field-list node within a definition node.
// Searches up to 4 levels deep to handle nested structures (e.g. Go's
// type_declaration → type_spec → struct_type → field_declaration_list).
// Returns the container node whose named children have "name" fields, or nil.
func findMemberContainer(node *gotreesitter.Node, lang *gotreesitter.Language) *gotreesitter.Node {
	return findMemberContainerDepth(node, lang, 0)
}

// maxMemberContainerDepth is the maximum AST levels searched for a body/field-list.
// Go's deepest path is 3 levels: type_declaration → type_spec → struct_type → field_declaration_list.
// 4 provides one level of headroom for template-heavy or unusual grammars.
const maxMemberContainerDepth = 4

func findMemberContainerDepth(node *gotreesitter.Node, lang *gotreesitter.Language, depth int) *gotreesitter.Node {
	if depth > maxMemberContainerDepth {
		slog.Debug("findMemberContainer: depth limit exceeded, fields may be missing",
			"nodeType", node.Type(lang), "depth", depth)
		return nil
	}
	// Standard body field (Python class_definition, Go function_declaration, Java class_declaration)
	if body := node.ChildByFieldName("body", lang); body != nil {
		return body
	}
	// No body field — search named children for a container whose children have "name" fields.
	// This handles Go struct_type → field_declaration_list (no "body" field name).
	for i := 0; i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if hasNamedMembers(child, lang) {
			return child
		}
		if result := findMemberContainerDepth(child, lang, depth+1); result != nil {
			return result
		}
	}
	return nil
}

// hasNamedMembers reports whether node's named children each have a "name" field.
func hasNamedMembers(node *gotreesitter.Node, lang *gotreesitter.Language) bool {
	if node.NamedChildCount() == 0 {
		return false
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		if node.NamedChild(i).ChildByFieldName("name", lang) != nil {
			return true
		}
	}
	return false
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
		if typeName == "pointer_type" && typeNode.NamedChildCount() > 0 {
			inner := typeNode.NamedChild(0)
			return string(source[inner.StartByte():inner.EndByte()])
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
		isComment := strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "/**") || strings.HasPrefix(line, " *") || strings.HasPrefix(line, "*/")
		if isComment {
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
	case kindFunction:
		return fmt.Sprintf("func %s()", s.Name)
	case kindMethod:
		if s.Parent != "" {
			return fmt.Sprintf("func (%s) %s()", s.Parent, s.Name)
		}
		return fmt.Sprintf("func %s()", s.Name)
	case kindType, "struct", "enum":
		return fmt.Sprintf("type %s", s.Name)
	case kindClass:
		return fmt.Sprintf("class %s", s.Name)
	case kindInterface:
		return fmt.Sprintf("interface %s", s.Name)
	case kindImpl:
		return fmt.Sprintf("impl %s", s.Name)
	case kindField:
		return s.Name
	case kindModule:
		return fmt.Sprintf("module %s", s.Name)
	case kindMacro:
		return fmt.Sprintf("macro %s", s.Name)
	case kindConstructor:
		return fmt.Sprintf("constructor %s()", s.Name)
	case kindConstant:
		return fmt.Sprintf("const %s", s.Name)
	case kindVariable:
		return fmt.Sprintf("var %s", s.Name)
	default:
		return s.Name
	}
}
