// Command block-redundant-createdat is a Claude Code PreToolUse hook that
// rejects any Write or Edit of a Convex schema file which *adds* a
// `createdAt:` field inside a `defineTable({ ... })` block.
//
// Rationale: Convex automatically populates a `_creationTime: number` column
// on every row. A custom `createdAt` field that mirrors it is redundant —
// it duplicates data, drifts when callers pass a different value, and adds
// noise to every validator. This hook prevents *new* occurrences from
// landing; existing `createdAt` fields in the schema are grandfathered until
// a separate cleanup sweep removes them.
//
// Semantics:
//   - Only fires on Write / Edit tool calls targeting a Convex schema file
//     (path contains `/convex/schema/` or basename is `schema.ts`).
//   - Counts `createdAt:` occurrences inside `defineTable({ ... })` blocks
//     in both the current file and the resulting content. If the count
//     increases, the edit is blocked. Non-increasing edits (including
//     removals) are always allowed.
//   - Disabled by `CLAUDE_HOOKS_AST_VALIDATION=false` (shared bypass with
//     the other PreToolUse validators).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ToolInput mirrors the Write / Edit tool payloads we care about.
type ToolInput struct {
	FilePath  string `json:"file_path"`
	Content   string `json:"content,omitempty"`
	OldString string `json:"old_string,omitempty"`
	NewString string `json:"new_string,omitempty"`
}

// HookData is the envelope Claude Code sends on stdin.
type HookData struct {
	ToolName  string    `json:"tool_name"`
	ToolInput ToolInput `json:"tool_input"`
}

// isSchemaFile returns true when the path looks like a Convex schema file.
// Supports the multiple layouts we've seen in practice:
//
//   - `.../convex/schema/<name>.ts`        (dashtag convention)
//   - `.../backend/schema/<name>.ts`       (upc-me / mhi convention)
//   - `.../schemas/<domain>/<name>.ts`     (camcoapp convention — plural)
//   - basename `schema.ts` / `schema.tsx`  (single-file schema)
//   - basename `*.schema.ts` / `*.schema.tsx`  (per-entity schema module)
//
// The match is intentionally broad — false positives end up with a zero
// count (no `defineTable({...})` block, nothing to block) so the worst
// case is wasted regex work on a handful of unrelated files.
func isSchemaFile(filePath string) bool {
	if filePath == "" {
		return false
	}
	// Normalize Windows separators so the substring checks below work
	// regardless of platform.
	p := strings.ReplaceAll(filePath, `\`, "/")
	if strings.Contains(p, "/schema/") || strings.Contains(p, "/schemas/") {
		return true
	}
	base := filepath.Base(filePath)
	if base == "schema.ts" || base == "schema.tsx" {
		return true
	}
	if strings.HasSuffix(base, ".schema.ts") || strings.HasSuffix(base, ".schema.tsx") {
		return true
	}
	return false
}

// defineTableBlockPattern matches `defineTable({` through the matching `})`.
// Naive — doesn't handle strings containing literal braces — but schema
// files are constrained enough that this is fine. We search balanced.
var defineTableStart = regexp.MustCompile(`\bdefineTable\s*\(\s*\{`)

// extractDefineTableBlocks returns the inner body of every defineTable({...})
// block in the source. Bodies are returned in source order; if the source is
// malformed (unbalanced braces), whatever we got is still returned.
func extractDefineTableBlocks(src string) []string {
	var blocks []string
	rest := src
	offset := 0
	for {
		loc := defineTableStart.FindStringIndex(rest)
		if loc == nil {
			break
		}
		// Position of the `{` that opens the object literal.
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
		// Advance past this block.
		offset = closeAt + 1
		if offset >= len(src) {
			break
		}
		rest = src[offset:]
	}
	return blocks
}

// createdAtPattern counts `createdAt:` field declarations anywhere in a
// block. The `\b` word boundary keeps us from matching `notCreatedAt:` or
// similar. Comments are stripped before matching, so commented-out entries
// don't count.
var createdAtPattern = regexp.MustCompile(`\bcreatedAt\s*:`)

// countCreatedAtInDefineTable returns the number of `createdAt:` declarations
// found inside any `defineTable({ ... })` block in src. Occurrences outside
// defineTable are ignored — they're regular object literals / constants.
func countCreatedAtInDefineTable(src string) int {
	blocks := extractDefineTableBlocks(src)
	total := 0
	for _, b := range blocks {
		total += len(createdAtPattern.FindAllString(stripComments(b), -1))
	}
	return total
}

// stripComments removes `// line` comments and `/* block */` comments so a
// commented-out `createdAt:` inside a defineTable block isn't counted. This
// is the cheap path; a real lexer would handle strings correctly, but schema
// files don't have `createdAt:` literals inside strings.
func stripComments(src string) string {
	// Block comments first — simple greedy replace is safe because nested
	// block comments aren't legal in TS.
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

// resultingContent computes the file content that would exist after the tool
// call completes. Write supplies content directly; Edit applies a single
// replacement to the existing file.
func resultingContent(data HookData) (string, error) {
	switch data.ToolName {
	case "Write":
		return data.ToolInput.Content, nil
	case "Edit":
		existing, err := os.ReadFile(data.ToolInput.FilePath)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", data.ToolInput.FilePath, err)
		}
		return strings.Replace(string(existing), data.ToolInput.OldString, data.ToolInput.NewString, 1), nil
	default:
		return "", fmt.Errorf("unsupported tool: %s", data.ToolName)
	}
}

// currentFileContent returns the bytes of the target file if it exists, or
// an empty string if it doesn't (e.g. a Write creating a new schema file).
func currentFileContent(filePath string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	return string(data)
}

// checkDisabled checks if the hook is disabled via env var. Shares the
// CLAUDE_HOOKS_AST_VALIDATION bypass with the other PreToolUse validators
// so an operator who temporarily disables them doesn't have to remember
// multiple flags.
func checkDisabled() bool {
	return os.Getenv("CLAUDE_HOOKS_AST_VALIDATION") == "false"
}

// walkSkipDirs are directory basenames the audit walker never descends into.
// Mirrors the set used by the stubs detector so audits don't drown in
// generated code or dependency trees.
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

// listViolations walks each root for schema files and prints every file that
// has at least one `createdAt:` inside a `defineTable({...})` block, suffixed
// with the count. Returns the total number of violating files. Unreadable
// files are silently skipped so a permission error deep in the tree can't
// mask results elsewhere.
//
// Output format: `path\t<count>\n` — tab-separated so callers can pipe
// through `awk`/`cut` without worrying about paths containing spaces.
func listViolations(roots []string, out io.Writer) (int, error) {
	total := 0
	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			// Don't fail the whole scan for one bad root — just skip.
			continue
		}
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
			if !isSchemaFile(path) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			count := countCreatedAtInDefineTable(string(data))
			if count > 0 {
				fmt.Fprintf(out, "%s\t%d\n", path, count)
				total++
			}
			return nil
		})
		if err != nil {
			return total, fmt.Errorf("walking %s: %w", root, err)
		}
	}
	return total, nil
}

func main() {
	listMode := flag.Bool("list-violations", false,
		"scan positional paths (or cwd) for schema files with createdAt in defineTable and exit")
	flag.Parse()

	if *listMode {
		roots := flag.Args()
		if len(roots) == 0 {
			roots = []string{"."}
		}
		count, err := listViolations(roots, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "block-redundant-createdat: %v\n", err)
			os.Exit(1)
		}
		if count > 0 {
			// Non-zero exit so CI / shell pipelines can gate on cleanliness.
			os.Exit(1)
		}
		os.Exit(0)
	}

	if checkDisabled() {
		os.Exit(0)
	}

	var data HookData
	if err := json.NewDecoder(os.Stdin).Decode(&data); err != nil {
		os.Exit(0)
	}
	if data.ToolName != "Write" && data.ToolName != "Edit" {
		os.Exit(0)
	}
	if !isSchemaFile(data.ToolInput.FilePath) {
		os.Exit(0)
	}

	resulting, err := resultingContent(data)
	if err != nil {
		// Can't compute the post-state; be permissive rather than blocking
		// a user on our failure to read the file.
		os.Exit(0)
	}

	before := countCreatedAtInDefineTable(currentFileContent(data.ToolInput.FilePath))
	after := countCreatedAtInDefineTable(resulting)

	if after > before {
		fmt.Fprintf(os.Stderr, `BLOCKED: Redundant createdAt field in defineTable

File: %s
Before: %d createdAt field(s) inside defineTable({...}) blocks
After:  %d

Convex automatically maintains `+"`"+`_creationTime: number`+"`"+` on every row and
exposes a `+"`"+`by_creation_time`+"`"+` index for free. Adding a `+"`"+`createdAt`+"`"+` column
duplicates that data, risks drift when callers pass a different value,
and adds noise to every validator.

Use `+"`"+`row._creationTime`+"`"+` in queries and sort via the `+"`"+`by_creation_time`+"`"+`
index instead. If you need a semantically different timestamp (e.g. when
the record was ACTIVATED, not inserted), rename the field to reflect
that meaning: `+"`"+`activatedAt`+"`"+`, `+"`"+`publishedAt`+"`"+`, `+"`"+`verifiedAt`+"`"+`.

To bypass (not recommended): set CLAUDE_HOOKS_AST_VALIDATION=false
`, data.ToolInput.FilePath, before, after)
		os.Exit(2)
	}

	os.Exit(0)
}
