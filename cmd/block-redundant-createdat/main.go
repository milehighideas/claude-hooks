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
	"strings"

	"github.com/milehighideas/claude-hooks/internal/schemachecks"
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

// Schema-file detection and createdAt counting live in the shared
// schemachecks package so this hook and the commit-time ratchet stay in
// lockstep on detection rules (including exemption markers). See
// internal/schemachecks/createdat.go for the full rule set.

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
			if !schemachecks.IsSchemaFile(path) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			count := schemachecks.CountCreatedAt(string(data))
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
	if !schemachecks.IsSchemaFile(data.ToolInput.FilePath) {
		os.Exit(0)
	}

	resulting, err := resultingContent(data)
	if err != nil {
		// Can't compute the post-state; be permissive rather than blocking
		// a user on our failure to read the file.
		os.Exit(0)
	}

	before := schemachecks.CountCreatedAt(currentFileContent(data.ToolInput.FilePath))
	after := schemachecks.CountCreatedAt(resulting)

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

Legacy/transitional exemption: a `+"`"+`createdAt: v.optional(...)`+"`"+` field marked
with either an inline `+"`"+`// hooks-allow: redundant-createdat`+"`"+` comment or a
preceding `+"`"+`/** @deprecated ... */`+"`"+` JSDoc block is allowed. This lets you
widen a required column to optional during a widen-migrate-narrow cleanup
without the hook blocking the transitional state.
`, data.ToolInput.FilePath, before, after)
		os.Exit(2)
	}

	os.Exit(0)
}
