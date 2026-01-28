// markdown-formatter is a Claude Code PostToolUse hook that formats markdown files.
// It adds language tags to unlabeled code fences and fixes excessive blank lines.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// toolInput represents the JSON structure from Claude Code's PostToolUse hook.
type toolInput struct {
	Tool      string `json:"tool"`
	ToolInput struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

// Language detection patterns
var (
	// JSON: starts with { or [
	jsonStartPattern = regexp.MustCompile(`^\s*[{\[]`)

	// Python: def function or import/from statements
	pythonDefPattern    = regexp.MustCompile(`(?m)^\s*def\s+\w+\s*\(`)
	pythonImportPattern = regexp.MustCompile(`(?m)^\s*(import|from)\s+\w+`)

	// JavaScript/TypeScript: function, const, arrow functions, console
	jsFunctionPattern = regexp.MustCompile(`\b(function\s+\w+\s*\(|const\s+\w+\s*=)`)
	jsArrowPattern    = regexp.MustCompile(`=>|console\.(log|error)`)

	// TypeScript-specific: interface, type, generic syntax
	tsInterfacePattern = regexp.MustCompile(`\b(interface|type)\s+\w+`)
	tsGenericPattern   = regexp.MustCompile(`<\w+>`)

	// Bash: shebang or control structures
	bashShebangPattern = regexp.MustCompile(`(?m)^#!.*\b(bash|sh)\b`)
	bashControlPattern = regexp.MustCompile(`\b(if|then|fi|for|in|do|done)\b`)
	bashCommandPattern = regexp.MustCompile(`\b(echo|export|source|chmod|mkdir|cd|ls|grep|sed|awk)\b`)

	// SQL: common SQL keywords
	sqlPattern = regexp.MustCompile(`(?i)\b(SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|DROP)\s+`)

	// Go: package, func, import with quotes
	goPackagePattern = regexp.MustCompile(`(?m)^package\s+\w+`)
	goFuncPattern    = regexp.MustCompile(`(?m)^func\s+`)
	goImportPattern  = regexp.MustCompile(`(?m)^import\s+(\(|")`)

	// Rust: fn, let mut, impl, use
	rustFnPattern   = regexp.MustCompile(`(?m)^(pub\s+)?fn\s+\w+`)
	rustLetPattern  = regexp.MustCompile(`\blet\s+(mut\s+)?\w+`)
	rustImplPattern = regexp.MustCompile(`(?m)^impl\s+`)

	// HTML/JSX
	htmlPattern = regexp.MustCompile(`<(!DOCTYPE|html|head|body|div|span|p|a|script|style)\b`)
	jsxPattern  = regexp.MustCompile(`<[A-Z]\w*[\s/>]|className=`)

	// CSS
	cssPattern     = regexp.MustCompile(`[.#]?\w+\s*\{[^}]*\}`)
	cssPropPattern = regexp.MustCompile(`\b(color|background|margin|padding|display|flex|grid):\s*`)

	// YAML: key: value patterns, common YAML keys
	yamlPattern     = regexp.MustCompile(`(?m)^\s*\w+:\s*(\w+|$)`)
	yamlListPattern = regexp.MustCompile(`(?m)^\s*-\s+\w+`)

	// TOML: [section] and key = value
	tomlSectionPattern = regexp.MustCompile(`(?m)^\s*\[\w+\]`)
	tomlKeyPattern     = regexp.MustCompile(`(?m)^\s*\w+\s*=\s*`)

	// Excessive blank lines pattern
	excessiveBlanksPattern = regexp.MustCompile(`\n{3,}`)
)

// detectLanguage attempts to identify the programming language of code content.
func detectLanguage(code string) string {
	s := strings.TrimSpace(code)

	// JSON detection - try to parse
	if jsonStartPattern.MatchString(s) {
		var js json.RawMessage
		if json.Unmarshal([]byte(s), &js) == nil {
			return "json"
		}
	}

	// Go detection (before other C-like languages)
	if goPackagePattern.MatchString(s) || goFuncPattern.MatchString(s) || goImportPattern.MatchString(s) {
		return "go"
	}

	// Rust detection
	if rustFnPattern.MatchString(s) || rustImplPattern.MatchString(s) || rustLetPattern.MatchString(s) {
		return "rust"
	}

	// Python detection
	if pythonDefPattern.MatchString(s) || pythonImportPattern.MatchString(s) {
		return "python"
	}

	// TypeScript detection (before JavaScript)
	if tsInterfacePattern.MatchString(s) || (tsGenericPattern.MatchString(s) && jsFunctionPattern.MatchString(s)) {
		return "typescript"
	}

	// JSX/TSX detection
	if jsxPattern.MatchString(s) {
		if tsInterfacePattern.MatchString(s) || tsGenericPattern.MatchString(s) {
			return "tsx"
		}
		return "jsx"
	}

	// JavaScript detection
	if jsFunctionPattern.MatchString(s) || jsArrowPattern.MatchString(s) {
		return "javascript"
	}

	// Bash detection
	if bashShebangPattern.MatchString(s) || bashControlPattern.MatchString(s) || bashCommandPattern.MatchString(s) {
		return "bash"
	}

	// SQL detection
	if sqlPattern.MatchString(s) {
		return "sql"
	}

	// HTML detection
	if htmlPattern.MatchString(s) {
		return "html"
	}

	// CSS detection
	if cssPattern.MatchString(s) && cssPropPattern.MatchString(s) {
		return "css"
	}

	// YAML detection
	if yamlPattern.MatchString(s) && yamlListPattern.MatchString(s) {
		return "yaml"
	}

	// TOML detection
	if tomlSectionPattern.MatchString(s) && tomlKeyPattern.MatchString(s) {
		return "toml"
	}

	return "text"
}

// codeFence represents a parsed code fence block.
type codeFence struct {
	startLine int
	endLine   int
	indent    string
	lang      string
	body      string
	hasLang   bool
}

// parseCodeFences finds all code fence blocks in the content.
func parseCodeFences(lines []string) []codeFence {
	var fences []codeFence
	var current *codeFence
	var bodyLines []string

	fenceStart := regexp.MustCompile("^([ \t]{0,3})```(.*)$")

	for i, line := range lines {
		if current == nil {
			// Look for opening fence
			if matches := fenceStart.FindStringSubmatch(line); matches != nil {
				current = &codeFence{
					startLine: i,
					indent:    matches[1],
					lang:      strings.TrimSpace(matches[2]),
					hasLang:   strings.TrimSpace(matches[2]) != "",
				}
				bodyLines = nil
			}
		} else {
			// Look for closing fence with same or less indent
			closingFence := current.indent + "```"
			trimmedLine := strings.TrimRight(line, " \t")
			if trimmedLine == closingFence || (strings.HasPrefix(line, current.indent) && strings.TrimPrefix(strings.TrimSpace(line), current.indent) == "```") {
				// Check if line matches closing pattern
				if strings.TrimSpace(strings.TrimPrefix(line, current.indent)) == "```" {
					current.endLine = i
					current.body = strings.Join(bodyLines, "\n")
					fences = append(fences, *current)
					current = nil
					bodyLines = nil
					continue
				}
			}
			bodyLines = append(bodyLines, line)
		}
	}

	return fences
}

// formatMarkdown formats markdown content by adding language tags and fixing blank lines.
func formatMarkdown(content string) string {
	lines := strings.Split(content, "\n")
	fences := parseCodeFences(lines)

	// Process fences in reverse order to preserve line numbers
	for i := len(fences) - 1; i >= 0; i-- {
		fence := fences[i]
		if !fence.hasLang {
			lang := detectLanguage(fence.body)
			// Update the opening fence line
			lines[fence.startLine] = fence.indent + "```" + lang
		}
	}

	result := strings.Join(lines, "\n")

	// Fix excessive blank lines (3+ newlines -> 2 newlines)
	result = excessiveBlanksPattern.ReplaceAllString(result, "\n\n")

	// Ensure file ends with single newline
	return strings.TrimRight(result, "\n") + "\n"
}

func main() {
	var input toolInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		// Allow if we can't parse
		os.Exit(0)
	}

	filePath := input.ToolInput.FilePath
	if filePath == "" {
		os.Exit(0)
	}

	// Only process markdown files
	if !strings.HasSuffix(filePath, ".md") && !strings.HasSuffix(filePath, ".mdx") {
		os.Exit(0)
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		os.Exit(0)
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Format the markdown
	formatted := formatMarkdown(string(content))

	// Only write if content changed
	if formatted != string(content) {
		if err := os.WriteFile(filePath, []byte(formatted), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Fixed markdown formatting in %s\n", filePath)
	}

	os.Exit(0)
}
