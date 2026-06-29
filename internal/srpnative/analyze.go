package srpnative

import (
	"context"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/swift"
)

const astParseTimeout = 5 * time.Second

// Node-type sets, verified against the vendored grammars (see the keyword-token
// probe in the plan). Swift collapses class/struct/enum/actor/extension into a
// single class_declaration distinguished by its first keyword child; Kotlin
// collapses class/interface/enum into class_declaration with object_declaration
// separate.
var (
	typeDeclNodes = map[string]bool{
		"class_declaration":    true,
		"protocol_declaration": true, // swift
		"object_declaration":   true, // kotlin
	}
	typeBodyNodes = map[string]bool{
		"class_body":      true,
		"enum_class_body": true,
		"protocol_body":   true,
	}
)

func languageFor(lang Lang) *sitter.Language {
	if lang == Kotlin {
		return kotlin.GetLanguage()
	}
	return swift.GetLanguage()
}

// Analyze parses a native source file and returns its structural summary. On any
// parse failure it returns an analysis with only LineCount populated (fail open)
// so a syntax error never crashes or wrongly blocks a commit — matching
// internal/srp.Analyze.
func Analyze(code, filePath string, lang Lang) *Analysis {
	a := &Analysis{
		FilePath:  filePath,
		Lang:      lang,
		LineCount: strings.Count(code, "\n") + 1,
	}

	parser := sitter.NewParser()
	parser.SetLanguage(languageFor(lang))
	ctx, cancel := context.WithTimeout(context.Background(), astParseTimeout)
	defer cancel()

	src := []byte(code)
	tree, err := parser.ParseCtx(ctx, nil, src)
	if err != nil || tree == nil {
		return a
	}
	defer tree.Close()

	walk(tree.RootNode(), src, a)
	return a
}

// walk descends the whole tree once, collecting type and function declarations
// at any depth. A recursive walk (rather than a tree-sitter query) keeps parent
// context cheaply available for the IsTopLevel determination.
func walk(n *sitter.Node, src []byte, a *Analysis) {
	switch {
	case typeDeclNodes[n.Type()]:
		a.Types = append(a.Types, typeDeclFrom(n, src))
	case n.Type() == "function_declaration":
		if fd, ok := funcDeclFrom(n, src); ok {
			a.Funcs = append(a.Funcs, fd)
		}
	}
	for i := 0; i < int(n.NamedChildCount()); i++ {
		walk(n.NamedChild(i), src, a)
	}
}

func typeDeclFrom(n *sitter.Node, src []byte) TypeDecl {
	td := TypeDecl{
		StartLine:  int(n.StartPoint().Row) + 1,
		IsTopLevel: n.Parent() != nil && n.Parent().Type() == "source_file",
		Kind:       declKeyword(n),
	}
	td.IsExtension = td.Kind == "extension"
	if name := n.ChildByFieldName("name"); name != nil {
		td.Name = strings.TrimSpace(name.Content(src))
	}
	if body := firstChildOfTypes(n, typeBodyNodes); body != nil {
		td.BodyLines = lineSpan(body)
	}
	td.Conformances = conformancesOf(n, src)
	return td
}

// conformancesOf returns the names a type inherits from / conforms to, read from
// the declaration's inheritance_specifier children (field inherits_from).
func conformancesOf(n *sitter.Node, src []byte) []string {
	var out []string
	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		if c.Type() != "inheritance_specifier" {
			continue
		}
		base := c.ChildByFieldName("inherits_from")
		if base == nil {
			base = c.NamedChild(0)
		}
		if base != nil {
			out = append(out, strings.TrimSpace(base.Content(src)))
		}
	}
	return out
}

func funcDeclFrom(n *sitter.Node, src []byte) (FuncDecl, bool) {
	body := firstChildOfTypes(n, map[string]bool{"function_body": true})
	if body == nil {
		// Expression-bodied / protocol-requirement functions have no brace body
		// and no length to measure.
		return FuncDecl{}, false
	}
	fd := FuncDecl{
		StartLine: int(n.StartPoint().Row) + 1,
		BodyLines: lineSpan(body),
	}
	if name := n.ChildByFieldName("name"); name != nil {
		fd.Name = strings.TrimSpace(name.Content(src))
	}
	return fd, true
}

// declKeyword returns the leading declaration keyword token of a type node:
// "class"/"struct"/"enum"/"actor"/"extension" for a Swift class_declaration,
// "protocol" for protocol_declaration, "object"/"class"/"interface" for Kotlin.
// The keyword is the first anonymous child token of the declaration node.
func declKeyword(n *sitter.Node) string {
	switch n.Type() {
	case "protocol_declaration":
		return "protocol"
	case "object_declaration":
		return "object"
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if c.IsNamed() {
			continue
		}
		switch c.Type() {
		case "class", "struct", "enum", "actor", "extension", "interface":
			return c.Type()
		}
	}
	return n.Type()
}

func firstChildOfTypes(n *sitter.Node, types map[string]bool) *sitter.Node {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		if types[c.Type()] {
			return c
		}
	}
	return nil
}

// lineSpan returns the inclusive number of source lines a node covers.
func lineSpan(n *sitter.Node) int {
	return int(n.EndPoint().Row-n.StartPoint().Row) + 1
}
