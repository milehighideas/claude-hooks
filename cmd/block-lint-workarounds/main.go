package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
)

// HookInput represents the JSON input from stdin
type HookInput struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

// HookOutput represents the JSON output to stdout
type HookOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

// Pattern represents a regex pattern with its error message
type Pattern struct {
	Regex   *regexp.Regexp
	Message string
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(stdin io.Reader, stdout io.Writer) error {
	input, err := readHookInput(stdin)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	output := processHook(input)

	if err := writeHookOutput(stdout, output); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}

func readHookInput(r io.Reader) (*HookInput, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}

	var input HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("unmarshaling JSON: %w", err)
	}

	return &input, nil
}

func writeHookOutput(w io.Writer, output *HookOutput) error {
	data, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshaling output: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}

func processHook(input *HookInput) *HookOutput {
	// Only check Edit and Write tools
	if input.ToolName != "Edit" && input.ToolName != "Write" {
		return &HookOutput{Decision: "approve"}
	}

	// Get the text to check
	text := getTextToCheck(input.ToolInput)
	if text == "" {
		return &HookOutput{Decision: "approve"}
	}

	// Get file path to check for Convex context
	filePath := getFilePath(input.ToolInput)
	isConvexFile := isInConvexDirectory(filePath)

	// Check for underscore prefix workarounds
	if output := checkUnderscorePrefixes(text, isConvexFile); output != nil {
		return output
	}

	// Check for eslint-disable comments
	if output := checkESLintDisable(text); output != nil {
		return output
	}

	// Check for TypeScript suppression comments (warn only)
	if output := checkTSIgnore(text); output != nil {
		return output
	}

	return &HookOutput{Decision: "approve"}
}

func getTextToCheck(toolInput map[string]interface{}) string {
	// Try new_string first (for Edit tool)
	if newString, ok := toolInput["new_string"].(string); ok && newString != "" {
		return newString
	}

	// Try content (for Write tool)
	if content, ok := toolInput["content"].(string); ok && content != "" {
		return content
	}

	return ""
}

func getFilePath(toolInput map[string]interface{}) string {
	if filePath, ok := toolInput["file_path"].(string); ok {
		return filePath
	}
	return ""
}

func isInConvexDirectory(filePath string) bool {
	if filePath == "" {
		return false
	}
	// Check if file is in a convex directory or subdirectory
	return regexp.MustCompile(`/convex/`).MatchString(filePath)
}

// Convex system fields that start with underscore (allowed in convex directories)
var convexSystemFields = regexp.MustCompile(`_id|_creationTime`)

func checkUnderscorePrefixes(text string, isConvexFile bool) *HookOutput {
	patterns := []Pattern{
		{
			Regex:   regexp.MustCompile(`as\s+_\w+`),
			Message: "aliasing with underscore prefix",
		},
		{
			Regex:   regexp.MustCompile(`:\s*_\w+`),
			Message: "destructuring with underscore prefix",
		},
		{
			Regex:   regexp.MustCompile(`type\s+\w+\s+as\s+_`),
			Message: "type alias with underscore prefix",
		},
	}

	for _, p := range patterns {
		if match := p.Regex.FindString(text); match != "" {
			// In Convex files, allow Convex system fields like _id and _creationTime
			if isConvexFile && convexSystemFields.MatchString(match) {
				continue
			}

			return &HookOutput{
				Decision: "block",
				Reason: fmt.Sprintf(`BLOCKED: Underscore prefix workaround detected

Found: %s

Do not prefix unused imports/variables with underscore to silence lint errors.
Instead, REMOVE the unused import or variable entirely.

If you need the import for type-only usage, use 'import type { ... }' syntax.`, match),
			}
		}
	}

	return nil
}

func checkESLintDisable(text string) *HookOutput {
	patterns := []Pattern{
		{
			Regex:   regexp.MustCompile(`//\s*eslint-disable`),
			Message: "inline eslint-disable comment",
		},
		{
			Regex:   regexp.MustCompile(`/\*\s*eslint-disable`),
			Message: "block eslint-disable comment",
		},
	}

	for _, p := range patterns {
		if match := p.Regex.FindString(text); match != "" {
			return &HookOutput{
				Decision: "block",
				Reason: fmt.Sprintf(`BLOCKED: ESLint suppression comment detected

Found: %s

Do not add eslint-disable comments to suppress errors.
Instead, fix the underlying issue:
- Remove unused imports/variables
- Fix the code properly`, match),
			}
		}
	}

	return nil
}

func checkTSIgnore(text string) *HookOutput {
	patterns := []Pattern{
		{
			Regex:   regexp.MustCompile(`//\s*@ts-ignore`),
			Message: "ts-ignore comment",
		},
		{
			Regex:   regexp.MustCompile(`//\s*@ts-expect-error`),
			Message: "ts-expect-error comment",
		},
		{
			Regex:   regexp.MustCompile(`//\s*@ts-nocheck`),
			Message: "ts-nocheck comment",
		},
	}

	for _, p := range patterns {
		if match := p.Regex.FindString(text); match != "" {
			return &HookOutput{
				Decision: "approve",
				Reason: fmt.Sprintf(`WARNING: TypeScript suppression comment detected

Found: %s

Consider fixing the underlying type error if possible.
Proceeding anyway since ts-ignore is sometimes necessary for:
- Deep type instantiation errors (TS2589)
- Third-party library type issues
- Complex generic inference limits`, match),
			}
		}
	}

	return nil
}
