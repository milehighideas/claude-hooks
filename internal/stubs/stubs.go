// Package stubs detects test files whose every expect() call is the
// expect(true).toBe(true) placeholder. Shared between cmd/validate-test-files
// (which rejects new stubs at Write/Edit time and exposes -list-stubs) and
// cmd/pre-commit (which can gate commits on stub-free scope).
//
// The detector is intentionally simple regex matching on JS/TS source text,
// not a full AST parse — the false-positive cost of flagging a comment that
// mentions the stub pattern is cheaper than the complexity of an AST walk.
package stubs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// walkSkipDirs are directory basenames List/Find never descend into. Matches
// the original list that lived in cmd/validate-test-files — generated code,
// VCS, build outputs, and installed dependencies.
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

var (
	stubExpectPattern = regexp.MustCompile(`expect\s*\(\s*true\s*\)\s*\.\s*toBe\s*\(\s*true\s*\)`)
	anyExpectPattern  = regexp.MustCompile(`\bexpect\s*\(`)
)

// IsStub reports whether every expect() call in content is the placeholder
// form expect(true).toBe(true). Empty content and content with no expect()
// calls are NOT stubs — a stub requires at least one placeholder assertion
// AND every assertion being a placeholder.
func IsStub(content string) bool {
	stubMatches := stubExpectPattern.FindAllString(content, -1)
	if len(stubMatches) == 0 {
		return false
	}
	expectMatches := anyExpectPattern.FindAllString(content, -1)
	return len(expectMatches) == len(stubMatches)
}

// IsTestFile reports whether path is a test file this package considers for
// stub detection. Matches *.test.ts and *.test.tsx (the convention the
// hook enforces); other extensions are ignored.
func IsTestFile(path string) bool {
	return strings.HasSuffix(path, ".test.ts") || strings.HasSuffix(path, ".test.tsx")
}

// List walks root for test files and prints the path of each stub to out,
// returning the count of stubs discovered. Unreadable files and subtrees
// are silently skipped so a permission error deep in the tree can't mask
// results elsewhere.
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
		if !IsTestFile(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if IsStub(string(data)) {
			fmt.Fprintln(out, path)
			count++
		}
		return nil
	})
	return count, err
}

// Find is the non-streaming counterpart to List: returns the collected stub
// paths instead of printing. Convenient for callers that want to format
// their own output (e.g. pre-commit's status messages).
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
		if !IsTestFile(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if IsStub(string(data)) {
			found = append(found, path)
		}
		return nil
	})
	return found, err
}

// CheckFile returns true if path points to a test file that is a pure stub.
// Non-test files and unreadable files return false. Callers use this to
// check individual staged files without walking the tree.
func CheckFile(path string) bool {
	if !IsTestFile(path) {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return IsStub(string(data))
}
