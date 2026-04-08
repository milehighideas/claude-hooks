package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// MaestroValidation
//
// Static validation that every `id:` selector used in a Maestro flow still
// resolves to a `testID` in source code. This catches the most common form
// of Maestro drift: a screen is refactored, the testID is renamed or deleted,
// and the flow silently breaks until someone tries to run the E2E.
//
// The check is opt-in. Enable it by setting:
//
//	{
//	  "features": { "maestroValidation": true },
//	  "maestroValidation": {
//	    "flowsDir": "apps/mobile/.maestro/flows",
//	    "sourceDirs": [
//	      "apps/mobile/app",
//	      "apps/mobile/components",
//	      "packages/mobile-ui/src"
//	    ]
//	  }
//	}
//
// To run as non-blocking until drift is cleaned up, add to warningChecks:
//
//	{ "warningChecks": ["maestroValidation"] }

// Built-in directory basenames that are always skipped while walking source
// trees. User-supplied excludeDirs are merged on top of this list.
var defaultMaestroExcludeDirs = []string{
	"node_modules",
	".next",
	".expo",
	".git",
	"android",
	"ios",
	"build",
	"dist",
	"coverage",
	"__mocks__",
	"__tests__",
	"analysis-reports",
}

// Matches a Maestro id line in a flow YAML. Handles both quoted and unquoted
// forms and allows trailing comments.
var maestroIDLineRegex = regexp.MustCompile(
	`^\s*id:\s*(?:"([^"]+)"|'([^']+)'|([^\s#]+))\s*(?:#.*)?$`,
)

// maestroFlowRef is a single `id: "..."` selector found in a flow file.
type maestroFlowRef struct {
	ID   string
	File string
	Line int
}

// maestroTestIDLiteral is a literal testID="foo" definition found in source.
type maestroTestIDLiteral struct {
	Value string
	File  string
	Line  int
}

// maestroTestIDPattern is a templated testID definition from source (e.g.
// `foo-${x}`) compiled into a regex for membership testing.
type maestroTestIDPattern struct {
	Raw   string
	Regex *regexp.Regexp
	File  string
	Line  int
}

// maestroUnresolved groups all flow references to a single missing testID
// for reporting.
type maestroUnresolved struct {
	ID         string
	References []maestroFlowRef
}

// runMaestroValidation is the entry point called from the pre-commit dispatcher.
// It walks the configured flows directory, walks the configured source directories,
// and fails if any flow references an id that isn't defined in source.
func runMaestroValidation(cfg MaestroValidationConfig) error {
	if cfg.FlowsDir == "" || len(cfg.SourceDirs) == 0 {
		if !compactMode() {
			fmt.Println("Maestro validation skipped: flowsDir and sourceDirs must both be set in .pre-commit.json")
		}
		return nil
	}

	if _, err := os.Stat(cfg.FlowsDir); os.IsNotExist(err) {
		if !compactMode() {
			fmt.Printf("Maestro validation skipped: %s does not exist\n", cfg.FlowsDir)
		}
		return nil
	}

	refs, err := loadMaestroFlowRefs(cfg.FlowsDir)
	if err != nil {
		return fmt.Errorf("scan maestro flows: %w", err)
	}

	excludeDirs := mergeMaestroExcludeDirs(cfg.ExcludeDirs)
	literals, patterns, err := loadMaestroSourceTestIDs(cfg.SourceDirs, excludeDirs)
	if err != nil {
		return fmt.Errorf("scan source testIDs: %w", err)
	}

	unresolved := resolveMaestroRefs(refs, literals, patterns)

	if compactMode() {
		return reportMaestroCompact(refs, unresolved)
	}
	return reportMaestroVerbose(refs, literals, patterns, unresolved)
}

// mergeMaestroExcludeDirs returns the union of the built-in skip list and any
// user-supplied additions. Ordering doesn't matter because the result is used
// as a set.
func mergeMaestroExcludeDirs(extra []string) map[string]bool {
	set := make(map[string]bool, len(defaultMaestroExcludeDirs)+len(extra))
	for _, d := range defaultMaestroExcludeDirs {
		set[d] = true
	}
	for _, d := range extra {
		set[d] = true
	}
	return set
}

// loadMaestroFlowRefs walks flowsDir recursively and extracts every id:
// selector from every .yaml file. References to env-var placeholders
// (e.g. `id: "${E2E_FOO}"`) are intentionally dynamic and skipped.
func loadMaestroFlowRefs(flowsDir string) ([]maestroFlowRef, error) {
	var refs []maestroFlowRef

	err := filepath.Walk(flowsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil // skip unreadable files silently
		}

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			match := maestroIDLineRegex.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			id := firstNonEmpty(match[1], match[2], match[3])
			if id == "" || strings.Contains(id, "${") {
				continue
			}
			refs = append(refs, maestroFlowRef{ID: id, File: path, Line: i + 1})
		}
		return nil
	})

	return refs, err
}

// firstNonEmpty returns the first non-empty string from the arguments, or ""
// if all are empty. Used to pull the captured id out of the alternation in
// maestroIDLineRegex.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// loadMaestroSourceTestIDs walks each source directory and extracts every
// testID definition from .ts/.tsx files. Test files (`.test.*`, `.spec.*`)
// are excluded because their testIDs are fixture values, not real identifiers
// that Maestro targets.
func loadMaestroSourceTestIDs(
	sourceDirs []string,
	excludeDirs map[string]bool,
) (map[string]maestroTestIDLiteral, []maestroTestIDPattern, error) {
	literals := make(map[string]maestroTestIDLiteral)
	var patterns []maestroTestIDPattern

	for _, root := range sourceDirs {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if excludeDirs[info.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !isMaestroSourceFile(path) {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			if !strings.Contains(string(content), "testID") {
				return nil
			}
			fileLiterals, filePatterns := extractMaestroTestIDs(string(content), path)
			for _, lit := range fileLiterals {
				// First occurrence wins for error-location purposes.
				if _, exists := literals[lit.Value]; !exists {
					literals[lit.Value] = lit
				}
			}
			patterns = append(patterns, filePatterns...)
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}

	return literals, patterns, nil
}

// isMaestroSourceFile returns true for .ts/.tsx files that are not unit test
// files. Test files define fixture testIDs that shouldn't count as real
// Maestro targets.
func isMaestroSourceFile(path string) bool {
	if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
		return false
	}
	base := filepath.Base(path)
	if strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".test.tsx") ||
		strings.HasSuffix(base, ".spec.ts") || strings.HasSuffix(base, ".spec.tsx") {
		return false
	}
	return true
}

// extractMaestroTestIDs scans a source file for every statically resolvable
// testID definition. Returns literals (fixed strings) and patterns (regexes
// built from template literals) separately so they can be matched with
// different strategies.
//
// Handles:
//   - testID="foo"              → literal
//   - testID='foo'              → literal
//   - testID={"foo"}            → literal
//   - testID={cond ? "a" : "b"} → two literals (both branches)
//   - testID={`foo-${x}`}       → pattern /^foo-.+$/
//   - testID={`a-${x}-b`}       → pattern /^a-.+-b$/
//   - testID={`foo` || 'bar'}   → literal(s) for each side
//   - testID={variable}         → skipped (cannot resolve statically)
func extractMaestroTestIDs(content, file string) ([]maestroTestIDLiteral, []maestroTestIDPattern) {
	var literals []maestroTestIDLiteral
	var patterns []maestroTestIDPattern

	// Pre-compute line-start offsets so we can report line numbers for each
	// discovered testID without re-scanning the content on every match.
	lineStarts := []int{0}
	for i, c := range content {
		if c == '\n' {
			lineStarts = append(lineStarts, i+1)
		}
	}
	lineAt := func(pos int) int {
		// Binary search for the greatest lineStart <= pos.
		lo, hi := 0, len(lineStarts)-1
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if lineStarts[mid] <= pos {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		return lo + 1
	}

	const tag = "testID="
	idx := 0
	for {
		found := strings.Index(content[idx:], tag)
		if found == -1 {
			break
		}
		pos := idx + found

		// Reject identifier prefixes like "mytestID=" by checking the char
		// immediately before the match.
		if pos > 0 {
			prev := content[pos-1]
			if isMaestroIdentChar(prev) {
				idx = pos + len(tag)
				continue
			}
		}

		valueStart := pos + len(tag)
		if valueStart >= len(content) {
			break
		}
		first := content[valueStart]
		line := lineAt(pos)

		switch first {
		case '"', '\'':
			// Plain string attribute: testID="foo"
			end := readMaestroQuotedString(content, valueStart+1, first)
			if end == -1 {
				idx = valueStart + 1
				continue
			}
			literals = append(literals, maestroTestIDLiteral{
				Value: content[valueStart+1 : end],
				File:  file,
				Line:  line,
			})
			idx = end + 1
		case '{':
			// JSX expression: testID={...}
			expr, end, ok := readMaestroBalanced(content, valueStart)
			if !ok {
				idx = valueStart + 1
				continue
			}
			classifyMaestroExpr(strings.TrimSpace(expr), file, line, &literals, &patterns)
			idx = end + 1
		default:
			idx = valueStart
		}
	}

	return literals, patterns
}

// isMaestroIdentChar returns true for characters that would make `testID=`
// a tail of a larger identifier (e.g. `mytestID=`), which we don't want to
// match.
func isMaestroIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_' || c == '$'
}

// readMaestroQuotedString walks forward from `start` (the first character
// after an opening quote) and returns the index of the matching closing
// quote, handling backslash escapes. Returns -1 if no closing quote is found.
func readMaestroQuotedString(content string, start int, quote byte) int {
	for i := start; i < len(content); i++ {
		c := content[i]
		if c == '\\' {
			i++ // skip escaped char
			continue
		}
		if c == quote {
			return i
		}
	}
	return -1
}

// readMaestroBalanced walks forward from an opening `{` at start and returns
// the content between the braces, the index of the matching `}`, and ok=true
// on success. Correctly handles:
//
//   - Nested braces from object literals
//   - String literals ("..." and '...') — braces inside are ignored
//   - Template literals (`...`) with ${} interpolations that themselves
//     contribute to brace depth
//
// This is necessary because the simple regex approach fails on common patterns
// like `testID={`foo-${x}`}` where the `${` inside the template would confuse
// a naive brace counter.
func readMaestroBalanced(content string, start int) (string, int, bool) {
	if start >= len(content) || content[start] != '{' {
		return "", 0, false
	}
	// stack depth tracks the number of open braces we still need to match.
	// inTemplate tracks whether we're currently inside a backtick string
	// literal; template expressions (${...}) push another level onto the
	// depth and return to template mode when closed.
	depth := 1
	inTemplate := false
	// depthModes[i] == true means the brace at that depth was opened by ${
	// inside a template literal and should return us to template mode on
	// its matching close.
	depthModes := []bool{false}
	i := start + 1
	for i < len(content) {
		c := content[i]

		if inTemplate {
			if c == '\\' {
				i += 2
				continue
			}
			if c == '`' {
				inTemplate = false
				i++
				continue
			}
			if c == '$' && i+1 < len(content) && content[i+1] == '{' {
				depth++
				depthModes = append(depthModes, true)
				inTemplate = false
				i += 2
				continue
			}
			i++
			continue
		}

		switch c {
		case '"', '\'':
			end := readMaestroQuotedString(content, i+1, c)
			if end == -1 {
				return "", 0, false
			}
			i = end + 1
		case '`':
			inTemplate = true
			i++
		case '{':
			depth++
			depthModes = append(depthModes, false)
			i++
		case '}':
			depth--
			if depth == 0 {
				return content[start+1 : i], i, true
			}
			// Returning from a ${...} expression puts us back inside the template literal.
			if len(depthModes) > 0 {
				lastWasTemplate := depthModes[len(depthModes)-1]
				depthModes = depthModes[:len(depthModes)-1]
				if lastWasTemplate {
					inTemplate = true
				}
			}
			i++
		default:
			i++
		}
	}
	return "", 0, false
}

// Pre-compiled regexes used by classifyMaestroExpr.
var (
	maestroQuotedRegex   = regexp.MustCompile(`^["']([^"']*)["']$`)
	maestroTernaryRegex  = regexp.MustCompile(`^[^?]+\?\s*["']([^"']+)["']\s*:\s*["']([^"']+)["']$`)
	maestroTemplateRegex = regexp.MustCompile("^`([^`]*)`$")
	maestroTemplateOr    = regexp.MustCompile("^`([^`]*)`\\s*\\|\\|\\s*[\"']([^\"']+)[\"']$")
)

// classifyMaestroExpr takes a trimmed expression from `testID={<expr>}` and
// appends any statically resolvable values to literals/patterns.
func classifyMaestroExpr(
	expr, file string,
	line int,
	literals *[]maestroTestIDLiteral,
	patterns *[]maestroTestIDPattern,
) {
	// 1) Quoted string: "foo" or 'foo'
	if m := maestroQuotedRegex.FindStringSubmatch(expr); m != nil {
		*literals = append(*literals, maestroTestIDLiteral{Value: m[1], File: file, Line: line})
		return
	}

	// 2) Ternary between two string literals: cond ? "foo" : "bar"
	if m := maestroTernaryRegex.FindStringSubmatch(expr); m != nil {
		*literals = append(*literals,
			maestroTestIDLiteral{Value: m[1], File: file, Line: line},
			maestroTestIDLiteral{Value: m[2], File: file, Line: line},
		)
		return
	}

	// 3) Template literal (may contain ${...} interpolations).
	if m := maestroTemplateRegex.FindStringSubmatch(expr); m != nil {
		body := m[1]
		if !strings.Contains(body, "${") {
			*literals = append(*literals, maestroTestIDLiteral{Value: body, File: file, Line: line})
			return
		}
		if rx := maestroTemplateToRegex(body); rx != nil {
			*patterns = append(*patterns, maestroTestIDPattern{Raw: body, Regex: rx, File: file, Line: line})
		}
		return
	}

	// 4) Template with logical fallback: `foo-${x}` || 'default'
	if m := maestroTemplateOr.FindStringSubmatch(expr); m != nil {
		body := m[1]
		if strings.Contains(body, "${") {
			if rx := maestroTemplateToRegex(body); rx != nil {
				*patterns = append(*patterns, maestroTestIDPattern{Raw: body, Regex: rx, File: file, Line: line})
			}
		} else {
			*literals = append(*literals, maestroTestIDLiteral{Value: body, File: file, Line: line})
		}
		*literals = append(*literals, maestroTestIDLiteral{Value: m[2], File: file, Line: line})
		return
	}

	// 5) Unresolvable (bare identifier, function call, etc.) — intentionally ignored.
}

// maestroTemplateToRegex converts a template literal body like
// "community-card-${id}" into an anchored regex that matches any concrete
// instantiation. Each ${...} becomes `.+`.
func maestroTemplateToRegex(template string) *regexp.Regexp {
	if template == "" {
		return nil
	}
	// Replace each ${...} with a placeholder that can't collide with any
	// escaped metacharacter, then escape the remaining literal text, then
	// swap placeholders for `.+`.
	const placeholder = "\x00MAESTRO_DYN\x00"
	dynRegex := regexp.MustCompile(`\$\{[^}]*\}`)
	withPlaceholders := dynRegex.ReplaceAllString(template, placeholder)
	escaped := regexp.QuoteMeta(withPlaceholders)
	escapedPlaceholder := regexp.QuoteMeta(placeholder)
	pattern := strings.ReplaceAll(escaped, escapedPlaceholder, ".+")
	if pattern == "" {
		return nil
	}
	rx, err := regexp.Compile("^" + pattern + "$")
	if err != nil {
		return nil
	}
	return rx
}

// resolveMaestroRefs returns flow references whose id is not defined by any
// literal or pattern in source. References for the same id are grouped.
func resolveMaestroRefs(
	refs []maestroFlowRef,
	literals map[string]maestroTestIDLiteral,
	patterns []maestroTestIDPattern,
) []maestroUnresolved {
	grouped := make(map[string][]maestroFlowRef)
	for _, ref := range refs {
		grouped[ref.ID] = append(grouped[ref.ID], ref)
	}

	var unresolved []maestroUnresolved
	for id, references := range grouped {
		if _, ok := literals[id]; ok {
			continue
		}
		matched := false
		for _, p := range patterns {
			if p.Regex.MatchString(id) {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		unresolved = append(unresolved, maestroUnresolved{ID: id, References: references})
	}

	sort.Slice(unresolved, func(i, j int) bool {
		return unresolved[i].ID < unresolved[j].ID
	})
	return unresolved
}

// reportMaestroCompact produces a one-line summary suitable for the pre-commit
// status rail. Called when compactMode() is true.
func reportMaestroCompact(refs []maestroFlowRef, unresolved []maestroUnresolved) error {
	if len(unresolved) == 0 {
		printStatus("Maestro validation", true, fmt.Sprintf("%d refs", len(refs)))
		return nil
	}
	flows := make(map[string]bool)
	totalRefs := 0
	for _, u := range unresolved {
		for _, ref := range u.References {
			flows[ref.File] = true
			totalRefs++
		}
	}
	detail := fmt.Sprintf("%d ids / %d flows / %d refs", len(unresolved), len(flows), totalRefs)
	printStatus("Maestro validation", false, detail)
	return fmt.Errorf("%d missing Maestro testIDs", len(unresolved))
}

// reportMaestroVerbose produces a summary grouped by flow file, which is the
// most actionable view when there are many broken references.
func reportMaestroVerbose(
	refs []maestroFlowRef,
	literals map[string]maestroTestIDLiteral,
	patterns []maestroTestIDPattern,
	unresolved []maestroUnresolved,
) error {
	fmt.Println("================================")
	fmt.Println("  MAESTRO TESTID VALIDATION")
	fmt.Println("================================")
	fmt.Printf("scanned %d Maestro id references\n", len(refs))
	fmt.Printf("scanned %d literal and %d templated testIDs\n", len(literals), len(patterns))

	if len(unresolved) == 0 {
		fmt.Println("✅ all Maestro testIDs resolve to source")
		return nil
	}

	type flowEntry struct {
		file     string
		ids      []string
		lines    map[string][]int
		totalRef int
	}
	byFlow := make(map[string]*flowEntry)

	for _, u := range unresolved {
		for _, ref := range u.References {
			entry := byFlow[ref.File]
			if entry == nil {
				entry = &flowEntry{file: ref.File, lines: map[string][]int{}}
				byFlow[ref.File] = entry
			}
			if _, seen := entry.lines[u.ID]; !seen {
				entry.ids = append(entry.ids, u.ID)
			}
			entry.lines[u.ID] = append(entry.lines[u.ID], ref.Line)
			entry.totalRef++
		}
	}

	entries := make([]*flowEntry, 0, len(byFlow))
	for _, e := range byFlow {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].totalRef > entries[j].totalRef
	})

	fmt.Printf("\n❌ %d missing testID(s) across %d flow(s):\n\n", len(unresolved), len(entries))

	for _, e := range entries {
		name := strings.TrimPrefix(e.file, "./")
		fmt.Printf("  ● %s  (%d ids, %d refs)\n", name, len(e.ids), e.totalRef)
		limit := len(e.ids)
		if limit > 6 {
			limit = 6
		}
		for _, id := range e.ids[:limit] {
			lines := e.lines[id]
			lineStrs := make([]string, len(lines))
			for i, l := range lines {
				lineStrs[i] = fmt.Sprintf("%d", l)
			}
			fmt.Printf("      %s %s\n", strings.Join(lineStrs, ","), id)
		}
		if len(e.ids) > limit {
			fmt.Printf("      ... +%d more\n", len(e.ids)-limit)
		}
		fmt.Println()
	}

	fmt.Println("These flows reference testIDs that no longer exist in source.")
	fmt.Println("Either restore the testID, update the flow, or delete the flow.")

	return fmt.Errorf("%d missing Maestro testIDs", len(unresolved))
}
