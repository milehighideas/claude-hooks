// Package schemachecks detects redundant patterns in Convex schema files.
//
// The primary rule right now: no `createdAt:` fields inside `defineTable({...})`
// blocks, because Convex automatically maintains `_creationTime: number` on
// every row. Shared between:
//
//   - cmd/block-redundant-createdat   (PreToolUse hook — blocks edits that
//                                      ADD a new createdAt)
//   - cmd/pre-commit                  (commit-time ratchet — reports every
//                                      schema file currently violating the
//                                      rule)
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
// are stripped by the caller before matching.
var createdAtPattern = regexp.MustCompile(`\bcreatedAt\s*:`)

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

// CountCreatedAt returns the number of `createdAt:` declarations found
// inside any `defineTable({...})` block in src. Occurrences outside
// defineTable are ignored — they're regular object literals or constants.
// Comments are stripped before matching so commented-out entries don't count.
func CountCreatedAt(src string) int {
	blocks := DefineTableBlocks(src)
	total := 0
	for _, b := range blocks {
		total += len(createdAtPattern.FindAllString(stripComments(b), -1))
	}
	return total
}

// HasRedundantCreatedAt reports whether src contains any `createdAt:` fields
// inside `defineTable({...})` blocks. Convenience wrapper — callers that
// need the count should use CountCreatedAt directly.
func HasRedundantCreatedAt(src string) bool {
	return CountCreatedAt(src) > 0
}

// stripComments removes `// line` and `/* block */` comments. The cheap
// path — a real lexer would handle strings correctly, but Convex schema
// files don't have `createdAt:` literals inside strings.
func stripComments(src string) string {
	// Block comments first; TS disallows nested block comments, so a greedy
	// single-pass replace is safe.
	for {
		start := strings.Index(src, "/*")
		if start == -1 {
			break
		}
		end := strings.Index(src[start:], "*/")
		if end == -1 {
			src = src[:start]
			break
		}
		src = src[:start] + src[start+end+2:]
	}
	// Line comments.
	var out strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if idx := strings.Index(line, "//"); idx != -1 {
			line = line[:idx]
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
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
			fmt.Fprintln(out, path)
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
