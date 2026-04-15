package stubs

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
)

// astParseTimeout caps per-file parsing. Tree-sitter is fast but adversarial
// input could in theory run long; 5s is a safe ceiling for any real source.
const astParseTimeout = 5 * time.Second

// jestMockQuery finds jest.mock(path, factory) calls and pulls out the
// mocked module path plus the factory arrow body. We capture the arrow
// function as @factory so the walker can inspect its return expression.
var jestMockQuery = func() *sitter.Query {
	q, err := sitter.NewQuery([]byte(`
(call_expression
  function: (member_expression
    object: (identifier) @obj
    property: (property_identifier) @prop
  )
  arguments: (arguments
    (string (string_fragment) @modpath)
    .
    (arrow_function) @factory
  )
) @call
`), tsx.GetLanguage())
	if err != nil {
		// Compilation-time query — if this fails the binary is broken.
		panic("stubs: jest.mock query failed to compile: " + err.Error())
	}
	return q
}()

// IsSelfMockStub reports whether a test file mocks its own subject in a way
// that neutralizes the test. Specifically: the file's basename (minus
// .test.ts[x]) matches a jest.mock() path argument, AND that mock's factory
// returns an object whose property values are null-rendering components —
// arrow functions like `() => null` or `({ children }) => <>{children}</>`.
//
// This catches the anti-pattern where agents mock out the subject they're
// supposed to be testing so the test can render without exercising the
// real component. Complements IsStub, which catches the "all assertions
// are weak" pattern; a file flagged by either is a stub.
//
// Returns false on any parse failure (fail open) so unrelated syntax
// issues never block commits.
func IsSelfMockStub(path, content string) bool {
	subject := testSubject(path)
	if subject == "" {
		return false
	}

	parser := sitter.NewParser()
	parser.SetLanguage(tsx.GetLanguage())
	ctx, cancel := context.WithTimeout(context.Background(), astParseTimeout)
	defer cancel()

	tree, err := parser.ParseCtx(ctx, nil, []byte(content))
	if err != nil || tree == nil {
		return false
	}
	defer tree.Close()

	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	cursor.Exec(jestMockQuery, tree.RootNode())

	src := []byte(content)
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}

		var obj, prop, modpath string
		var factory *sitter.Node
		for _, cap := range match.Captures {
			name := jestMockQuery.CaptureNameForId(cap.Index)
			switch name {
			case "obj":
				obj = cap.Node.Content(src)
			case "prop":
				prop = cap.Node.Content(src)
			case "modpath":
				modpath = cap.Node.Content(src)
			case "factory":
				factory = cap.Node
			}
		}

		if obj != "jest" || prop != "mock" {
			continue
		}
		if !mockPathIsSelf(path, modpath) {
			continue
		}
		if factory == nil {
			continue
		}
		if factoryReturnsNullComponents(factory, src) {
			return true
		}
	}
	return false
}

// testSubject extracts the module name a test file is supposed to cover.
// foo.test.tsx -> "foo", foo.test.ts -> "foo", non-test paths -> "".
func testSubject(path string) string {
	base := filepath.Base(path)
	for _, suffix := range []string{".test.tsx", ".test.ts"} {
		if strings.HasSuffix(base, suffix) {
			return strings.TrimSuffix(base, suffix)
		}
	}
	return ""
}

// mockPathIsSelf resolves a jest.mock() path argument relative to the test
// file's directory and checks whether it points at the same module the test
// claims to cover. Handles "./foo", "./foo.tsx", "../foo" properly — a
// sibling-in-parent-dir mock like "../foo" from apps/x/hooks/foo.test.ts
// is NOT the subject (foo.ts at apps/x/hooks/ is different from foo.ts at
// apps/x/), even though basenames match.
//
// Only relative paths ("./", "../") can be self-mocks; absolute paths and
// bare module specifiers are never the subject.
func mockPathIsSelf(testPath, modpath string) bool {
	modpath = strings.TrimSpace(modpath)
	if !strings.HasPrefix(modpath, "./") && !strings.HasPrefix(modpath, "../") {
		return false
	}

	testDir := filepath.Dir(testPath)
	resolved := filepath.Clean(filepath.Join(testDir, modpath))
	resolved = stripKnownExt(resolved)

	// Expected subject path: same dir as test, basename without .test.ts(x).
	subjectBase := testSubject(testPath)
	if subjectBase == "" {
		return false
	}
	expected := filepath.Join(testDir, subjectBase)

	return resolved == expected
}

// stripKnownExt drops the JS/TS extension from a path if present. Used
// because jest.mock paths may or may not include the extension and we
// want to compare against the extension-less subject.
func stripKnownExt(path string) string {
	for _, ext := range []string{".tsx", ".ts", ".jsx", ".js"} {
		if strings.HasSuffix(path, ext) {
			return strings.TrimSuffix(path, ext)
		}
	}
	return path
}

// factoryReturnsNullComponents inspects an arrow_function factory to see if
// it returns an object literal where at least one value is a null-rendering
// component, AND there is no spread element that would indicate a partial
// mock. This keeps jest.requireActual partial mocks out of the flagged set
// while catching the "replace everything with () => null" pattern.
func factoryReturnsNullComponents(factory *sitter.Node, src []byte) bool {
	body := factory.ChildByFieldName("body")
	if body == nil {
		return false
	}

	// Unwrap parenthesized_expression: (expression) around arrow body.
	if body.Type() == "parenthesized_expression" {
		if inner := body.NamedChild(0); inner != nil {
			body = inner
		}
	}

	// The body must be an object expression for the patterns we care about.
	if body.Type() != "object" {
		return false
	}

	hasSpread := false
	hasNullComponent := false

	count := int(body.NamedChildCount())
	for i := 0; i < count; i++ {
		prop := body.NamedChild(i)
		if prop == nil {
			continue
		}
		if prop.Type() == "spread_element" {
			hasSpread = true
			continue
		}
		if prop.Type() != "pair" {
			continue
		}
		value := prop.ChildByFieldName("value")
		if value == nil {
			continue
		}
		if isNullRenderingComponent(value, src) {
			hasNullComponent = true
		}
	}

	// Partial mocks (spread of requireActual) are legitimate testing tools
	// even if some properties are replaced with null stubs.
	if hasSpread {
		return false
	}
	return hasNullComponent
}

// isNullRenderingComponent reports whether node is an arrow function that
// returns null, undefined, or a JSX fragment with only {children} (the
// "passthrough wrapper" pattern). Tree-sitter's TSX grammar parses bare
// fragments "<>...</>" as jsx_element with empty opening/closing tags,
// so we handle both jsx_fragment and jsx_element shapes.
func isNullRenderingComponent(node *sitter.Node, src []byte) bool {
	if node.Type() != "arrow_function" {
		return false
	}
	body := node.ChildByFieldName("body")
	if body == nil {
		return false
	}

	// () => null, () => undefined
	if body.Type() == "null" || body.Type() == "undefined" {
		return true
	}

	// ({ children }) => <>{children}</>
	if body.Type() == "parenthesized_expression" {
		if inner := body.NamedChild(0); inner != nil {
			body = inner
		}
	}
	if body.Type() == "jsx_fragment" || isEmptyJSXElement(body, src) {
		return fragmentOnlyPassesThroughChildren(body, src)
	}

	return false
}

// isEmptyJSXElement returns true when node is a jsx_element whose opening
// tag has no tag name — i.e., a bare <>...</> fragment parsed as an
// element by TSX grammars that don't have a distinct fragment node.
func isEmptyJSXElement(node *sitter.Node, src []byte) bool {
	if node.Type() != "jsx_element" {
		return false
	}
	open := node.ChildByFieldName("open_tag")
	if open == nil {
		return false
	}
	// An empty opening tag has no children besides punctuation. If it had
	// a name, tree-sitter exposes it via a named "name" field.
	if open.ChildByFieldName("name") != nil {
		return false
	}
	return true
}

// fragmentOnlyPassesThroughChildren reports whether a JSX fragment-shaped
// node contains only an expression whose text is a single identifier — the
// "<>{children}</>" pattern where the component does nothing but forward
// what's inside it. Conservative: anything else returns false.
func fragmentOnlyPassesThroughChildren(fragment *sitter.Node, src []byte) bool {
	sawExpression := false
	count := int(fragment.NamedChildCount())
	for i := 0; i < count; i++ {
		child := fragment.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "jsx_opening_element", "jsx_closing_element":
			// Empty fragment delimiters — skip
			continue
		case "jsx_expression":
			if sawExpression {
				return false
			}
			sawExpression = true
			inner := child.NamedChild(0)
			if inner == nil || inner.Type() != "identifier" {
				return false
			}
		case "jsx_text":
			// Whitespace-only text is fine; anything else isn't a passthrough.
			txt := strings.TrimSpace(child.Content(src))
			if txt != "" {
				return false
			}
		case "jsx_self_closing_element", "jsx_element":
			// Real markup → not a passthrough
			return false
		}
	}
	return sawExpression
}
