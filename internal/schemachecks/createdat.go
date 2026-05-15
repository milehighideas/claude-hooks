// Package schemachecks detects redundant patterns in Convex schema files.
//
// The primary rule right now: no `createdAt:` fields inside `defineTable({...})`
// blocks, because Convex automatically maintains `_creationTime: number` on
// every row. Shared between:
//
//   - cmd/block-redundant-createdat   (PreToolUse hook — blocks edits that
//     ADD a new createdAt)
//   - cmd/pre-commit                  (commit-time ratchet — reports every
//     schema file currently violating the
//     rule)
//
// Like internal/stubs, the detector is regex-based rather than AST-based.
// Schema files are constrained enough that a brace-depth walker + comment
// stripper is both correct and faster than a full TS parser.
package schemachecks

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// walkSkipDirs are directory basenames List/Find never descend into. Mirrors
// the list in internal/stubs so schema walks don't wander into node_modules
// or generated output.
var walkSkipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"_generated":   true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".turbo":       true,
	".vercel":      true,
}

// defineTableStart matches the `defineTable({` token that opens a schema
// table definition. The brace-depth walker takes over from there.
var defineTableStart = regexp.MustCompile(`\bdefineTable\s*\(\s*\{`)

// createdAtPattern counts `createdAt:` field declarations anywhere in a
// block. `\b` keeps us from matching `notCreatedAt:` or similar. Comments
// are blanked by the caller before matching.
var createdAtPattern = regexp.MustCompile(`\bcreatedAt\s*:`)

// hooksAllowMarker is the inline opt-out comment for a transitional/legacy
// createdAt field. Placed as `// hooks-allow: redundant-createdat` on the
// same line as the declaration. Mirrors the eslint-disable-next-line style.
const hooksAllowMarker = "hooks-allow: redundant-createdat"

// IsSchemaFile reports whether path looks like a Convex schema file.
// Supports the multiple layouts seen in practice:
//
//   - `.../convex/schema/<name>.ts`        (dashtag convention)
//   - `.../backend/schema/<name>.ts`       (upc-me / mhi convention)
//   - `.../schemas/<domain>/<name>.ts`     (camcoapp convention — plural)
//   - basename `schema.ts` / `schema.tsx`  (single-file schema)
//   - basename `*.schema.ts` / `*.schema.tsx`  (per-entity schema module)
//
// The match is intentionally broad — false positives end up with a zero
// count (no `defineTable({...})` block, nothing to report) so the worst
// case is a handful of wasted regex passes on unrelated files.
func IsSchemaFile(path string) bool {
	if path == "" {
		return false
	}
	p := strings.ReplaceAll(path, `\`, "/")
	if strings.Contains(p, "/schema/") || strings.Contains(p, "/schemas/") {
		return true
	}
	base := filepath.Base(path)
	if base == "schema.ts" || base == "schema.tsx" {
		return true
	}
	if strings.HasSuffix(base, ".schema.ts") || strings.HasSuffix(base, ".schema.tsx") {
		return true
	}
	return false
}

// DefineTableBlocks returns the inner body (between the outer `{` and `}`)
// of every `defineTable({...})` block in src, in source order. Handles
// arbitrarily nested object literals via brace depth tracking. If the source
// is malformed (unbalanced braces), returns whatever was collected so far.
func DefineTableBlocks(src string) []string {
	var blocks []string
	rest := src
	offset := 0
	for {
		loc := defineTableStart.FindStringIndex(rest)
		if loc == nil {
			break
		}
		openBrace := offset + loc[1] - 1
		depth := 0
		closeAt := -1
		for i := openBrace; i < len(src); i++ {
			switch src[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					closeAt = i
				}
			}
			if closeAt != -1 {
				break
			}
		}
		if closeAt == -1 {
			break
		}
		blocks = append(blocks, src[openBrace+1:closeAt])
		offset = closeAt + 1
		if offset >= len(src) {
			break
		}
		rest = src[offset:]
	}
	return blocks
}

// CountCreatedAt returns the number of non-exempt `createdAt:` declarations
// found inside any `defineTable({...})` block in src. Occurrences outside
// defineTable are ignored — they're regular object literals or constants.
// Comments are blanked before matching so commented-out entries don't count.
//
// A `createdAt:` declaration is EXEMPT (not counted) when BOTH hold:
//  1. The field is wrapped in v.optional(...) on the same line.
//  2. One of the following intent markers is present:
//     - an inline `// hooks-allow: redundant-createdat` comment on the same line, OR
//     - a preceding JSDoc block (/** ... */) containing `@deprecated`.
//
// This lets schemas grandfather legacy createdAt columns during widen-migrate-narrow
// without silently defeating the rule: a bare v.optional() is still flagged, so
// new columns can't slip through.
func CountCreatedAt(src string) int {
	blocks := DefineTableBlocks(src)
	total := 0
	for _, b := range blocks {
		total += countNonExemptCreatedAt(b)
	}
	return total
}

// countNonExemptCreatedAt counts createdAt occurrences in a block body that
// aren't inside a comment and aren't covered by an exemption marker.
func countNonExemptCreatedAt(block string) int {
	// Blank comments so createdAt: inside a comment doesn't match, but keep
	// the original block around for exemption inspection (the exemption
	// markers LIVE in comments).
	blanked := blankComments(block)
	count := 0
	for _, loc := range createdAtPattern.FindAllStringIndex(blanked, -1) {
		if !isExemptCreatedAt(block, loc[0]) {
			count++
		}
	}
	return count
}

// isExemptCreatedAt reports whether the createdAt: occurrence at byte position
// pos in block is exempt from the redundancy rule. See CountCreatedAt for the
// full exemption rules.
func isExemptCreatedAt(block string, pos int) bool {
	lineStart := 0
	if nl := strings.LastIndexByte(block[:pos], '\n'); nl != -1 {
		lineStart = nl + 1
	}
	lineEnd := len(block)
	if nl := strings.IndexByte(block[pos:], '\n'); nl != -1 {
		lineEnd = pos + nl
	}
	line := block[lineStart:lineEnd]

	// Structural requirement: wrapped in v.optional(...)
	if !strings.Contains(line, "v.optional(") {
		return false
	}

	// Intent marker 1: inline hooks-allow comment on same line
	if strings.Contains(line, hooksAllowMarker) {
		return true
	}

	// Intent marker 2: preceding JSDoc block containing @deprecated
	return hasPrecedingDeprecatedJSDoc(block, lineStart)
}

// hasPrecedingDeprecatedJSDoc reports whether the code immediately preceding
// lineStart (ignoring whitespace) is a JSDoc block `/** ... */` containing the
// `@deprecated` tag. A plain `/* */` block is not accepted — JSDoc requires
// the double-star opener. Must be directly preceding: any intervening code
// breaks the association.
func hasPrecedingDeprecatedJSDoc(block string, lineStart int) bool {
	trimmed := strings.TrimRight(block[:lineStart], " \t\n\r")
	if !strings.HasSuffix(trimmed, "*/") {
		return false
	}
	jsdocStart := strings.LastIndex(trimmed, "/**")
	if jsdocStart == -1 {
		return false
	}
	jsdocBody := trimmed[jsdocStart : len(trimmed)-2]
	return strings.Contains(jsdocBody, "@deprecated")
}

// blankComments replaces comment characters in src with spaces (preserving
// newlines) so positions in the returned string map 1:1 to positions in src.
// Line and block comments only — string literals are not tracked because
// Convex schema files don't embed `createdAt:` in strings.
func blankComments(src string) string {
	b := []byte(src)
	i := 0
	for i < len(b) {
		if i+1 < len(b) && b[i] == '/' && b[i+1] == '*' {
			end := i + 2
			for end+1 < len(b) && (b[end] != '*' || b[end+1] != '/') {
				end++
			}
			closeEnd := end + 2
			if closeEnd > len(b) {
				closeEnd = len(b)
			}
			for j := i; j < closeEnd; j++ {
				if b[j] != '\n' {
					b[j] = ' '
				}
			}
			i = closeEnd
			continue
		}
		if i+1 < len(b) && b[i] == '/' && b[i+1] == '/' {
			for i < len(b) && b[i] != '\n' {
				b[i] = ' '
				i++
			}
			continue
		}
		i++
	}
	return string(b)
}

// HasRedundantCreatedAt reports whether src contains any `createdAt:` fields
// inside `defineTable({...})` blocks. Convenience wrapper — callers that
// need the count should use CountCreatedAt directly.
func HasRedundantCreatedAt(src string) bool {
	return CountCreatedAt(src) > 0
}

// List walks root for Convex schema files and prints the path of each file
// containing at least one `createdAt:` inside a `defineTable({...})` block.
// Returns the count of violating files. Unreadable files and subtrees are
// silently skipped.
func List(root string, out io.Writer) (int, error) {
	if _, err := os.Stat(root); err != nil {
		return 0, err
	}
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && walkSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !IsSchemaFile(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if HasRedundantCreatedAt(string(data)) {
			_, _ = fmt.Fprintln(out, path)
			count++
		}
		return nil
	})
	return count, err
}

// Find is the non-streaming counterpart to List: returns the violating
// schema file paths instead of printing. Used by pre-commit to format its
// own output.
func Find(root string) ([]string, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && walkSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !IsSchemaFile(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if HasRedundantCreatedAt(string(data)) {
			found = append(found, path)
		}
		return nil
	})
	return found, err
}

// CheckFile returns true if path is a schema file containing at least one
// redundant `createdAt:` field. Non-schema files and unreadable files
// return false.
func CheckFile(path string) bool {
	if !IsSchemaFile(path) {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return HasRedundantCreatedAt(string(data))
}
