package srp

import (
	"context"
	"regexp"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
)

const astParseTimeout = 5 * time.Second

// Regexes reused for name extraction from a node's (whitespace-normalized)
// text. Tree-sitter gives us correct statement boundaries — even for multiline
// imports/exports the old line-based scanner missed — and these parse the
// names out of that text.
var (
	importRe     = regexp.MustCompile(`^import\s+(?:{([^}]+)}|(\w+))?\s*(?:,\s*{([^}]+)})?\s*from\s+['"]([^'"]+)['"]`)
	exportRe     = regexp.MustCompile(`^export\s+(?:(type|interface)\s+)?(?:(const|let|var|function|class|default)\s+)?(\w+)`)
	exportTypeRe = regexp.MustCompile(`^export\s+type\s+`)
	exportFromRe = regexp.MustCompile(`from\s+['"]([^'"]+)['"]`)
	responsibRe  = regexp.MustCompile(`Single Responsibility:`)
)

var stateHooks = map[string]bool{
	"useState": true, "useReducer": true, "useContext": true,
	"useCallback": true, "useEffect": true, "useMemo": true,
}

func mustQuery(pattern string) *sitter.Query {
	q, err := sitter.NewQuery([]byte(pattern), tsx.GetLanguage())
	if err != nil {
		panic("srp: query failed to compile: " + err.Error())
	}
	return q
}

var (
	importQuery  = mustQuery(`(import_statement) @n`)
	exportQuery  = mustQuery(`(export_statement) @n`)
	hookQuery    = mustQuery(`(call_expression function: (identifier) @id) @call`)
	commentQuery = mustQuery(`(comment) @c`)
)

// Analyze parses a TS/TSX file and returns its structural summary. On any parse
// failure it returns an analysis with only LineCount populated (fail open) so a
// syntax error never crashes or wrongly blocks a commit.
func Analyze(code, filePath string) *Analysis {
	a := &Analysis{
		FilePath:  filePath,
		LineCount: strings.Count(code, "\n") + 1,
	}

	parser := sitter.NewParser()
	parser.SetLanguage(tsx.GetLanguage())
	ctx, cancel := context.WithTimeout(context.Background(), astParseTimeout)
	defer cancel()

	src := []byte(code)
	tree, err := parser.ParseCtx(ctx, nil, src)
	if err != nil || tree == nil {
		return a
	}
	defer tree.Close()
	root := tree.RootNode()

	a.Imports = extractImports(root, src)
	a.Exports = extractExports(root, src)
	a.StateManagement = extractStateHooks(root, src)
	a.HasResponsibilityComment = hasResponsibilityComment(root, src)
	return a
}

func eachCapture(q *sitter.Query, root *sitter.Node, fn func(name string, node *sitter.Node)) {
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	cursor.Exec(q, root)
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		for _, cap := range m.Captures {
			fn(q.CaptureNameForId(cap.Index), cap.Node)
		}
	}
}

func normalizeWS(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.Join(strings.Fields(s), " ")
}

func splitNames(group string) []string {
	var names []string
	for _, part := range strings.Split(group, ",") {
		if n := strings.TrimSpace(part); n != "" {
			names = append(names, n)
		}
	}
	return names
}

func extractImports(root *sitter.Node, src []byte) []ImportInfo {
	var out []ImportInfo
	eachCapture(importQuery, root, func(name string, node *sitter.Node) {
		if name != "n" {
			return
		}
		text := normalizeWS(node.Content(src))
		m := importRe.FindStringSubmatch(text)
		if m == nil {
			return
		}
		var names []string
		if m[1] != "" {
			names = append(names, splitNames(m[1])...)
		}
		if m[2] != "" {
			names = append(names, strings.TrimSpace(m[2]))
		}
		if m[3] != "" {
			names = append(names, splitNames(m[3])...)
		}
		out = append(out, ImportInfo{Source: m[4], Names: names})
	})
	return out
}

func extractExports(root *sitter.Node, src []byte) []ExportInfo {
	var out []ExportInfo
	eachCapture(exportQuery, root, func(name string, node *sitter.Node) {
		if name != "n" {
			return
		}
		text := normalizeWS(node.Content(src))
		m := exportRe.FindStringSubmatch(text)
		if m == nil {
			return
		}
		exportType := m[1]
		if exportType == "" {
			exportType = m[2]
		}
		var source string
		if fm := exportFromRe.FindStringSubmatch(text); fm != nil {
			source = fm[1]
		}
		out = append(out, ExportInfo{
			Name:       m[3],
			Type:       exportType,
			IsTypeOnly: exportTypeRe.MatchString(text),
			Source:     source,
		})
	})
	return out
}

func extractStateHooks(root *sitter.Node, src []byte) []StateInfo {
	var out []StateInfo
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	cursor.Exec(hookQuery, root)
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		for _, cap := range m.Captures {
			if hookQuery.CaptureNameForId(cap.Index) != "id" {
				continue
			}
			hook := cap.Node.Content(src)
			if stateHooks[hook] {
				out = append(out, StateInfo{
					Hook: hook,
					Line: int(cap.Node.StartPoint().Row) + 1,
				})
			}
		}
	}
	return out
}

func hasResponsibilityComment(root *sitter.Node, src []byte) bool {
	found := false
	eachCapture(commentQuery, root, func(name string, node *sitter.Node) {
		if name == "c" && responsibRe.MatchString(node.Content(src)) {
			found = true
		}
	})
	return found
}
