package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// E2E test extensions by app type
var e2eExtensions = map[string]string{
	"mobile": ".maestro.yaml",
	"native": ".maestro.yaml",
	"web":    ".e2e.ts",
	"portal": ".e2e.ts",
}

// Violation represents a test requirement violation
type Violation struct {
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	Reason       string `json:"reason"`
	ExpectedPath string `json:"expected_path"`
	AppType      string `json:"app_type,omitempty"`
}

// ToolInput represents the input to a tool call
type ToolInput struct {
	FilePath string `json:"file_path"`
}

// HookData represents the JSON input from Claude
type HookData struct {
	ToolName  string    `json:"tool_name"`
	ToolInput ToolInput `json:"tool_input"`
}

// getAppType determines app type from file path
func getAppType(filePath string) string {
	apps := []string{"mobile", "native", "web", "portal"}
	for _, app := range apps {
		if strings.Contains(filePath, "/"+app+"/") {
			return app
		}
	}
	return ""
}

// isTestFile checks if file is already a test file
func isTestFile(filePath string) bool {
	testPatterns := []string{".test.", ".spec.", ".e2e.", ".maestro."}
	for _, pattern := range testPatterns {
		if strings.Contains(filePath, pattern) {
			return true
		}
	}
	return false
}

// isTypeOrBarrelFile checks if file is a type definition or barrel export
func isTypeOrBarrelFile(filePath string) bool {
	basename := filepath.Base(filePath)
	if basename == "index.ts" || basename == "index.tsx" {
		return true
	}
	return strings.Contains(filePath, "/types/")
}

// getUnitTestPath returns expected unit test file path
func getUnitTestPath(filePath string) string {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".tsx":
		return strings.TrimSuffix(filePath, ".tsx") + ".test.tsx"
	case ".ts":
		return strings.TrimSuffix(filePath, ".ts") + ".test.ts"
	default:
		return ""
	}
}

// getE2ETestPath returns expected E2E test file path
func getE2ETestPath(filePath, appType string) string {
	if appType == "" {
		return ""
	}

	extension, exists := e2eExtensions[appType]
	if !exists {
		return ""
	}

	ext := filepath.Ext(filePath)
	return strings.TrimSuffix(filePath, ext) + extension
}

// isScreen checks if file is a screen component
func isScreen(filePath string) bool {
	return strings.Contains(filePath, "/screens/")
}

// isCRUDFolder checks if file is in a CRUD folder
func isCRUDFolder(filePath string) bool {
	crudFolders := []string{"/create/", "/update/", "/delete/"}
	for _, folder := range crudFolders {
		if strings.Contains(filePath, folder) {
			return true
		}
	}
	return false
}

// isHookOrUtil checks if file is a hook or utility
func isHookOrUtil(filePath string) bool {
	folders := []string{"/hooks/", "/utils/"}
	for _, folder := range folders {
		if strings.Contains(filePath, folder) {
			return true
		}
	}
	return false
}

// isInteractiveComponent determines if component is interactive using code patterns
func isInteractiveComponent(filePath string) (bool, error) {
	// Forms are always interactive
	if isCRUDFolder(filePath) && !strings.Contains(filePath, "/read/") {
		return true, nil
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}

	code := string(content)

	// Check for state management hooks
	stateHooks := []string{
		`useState`,
		`useReducer`,
		`useContext`,
		`useMutation`,
		`useQuery`,
	}

	for _, hook := range stateHooks {
		pattern := regexp.MustCompile(`\b` + hook + `\s*\(`)
		if pattern.MatchString(code) {
			return true, nil
		}
	}

	// Check for form hooks
	formHooks := []string{
		`useForm`,
		`useFormState`,
		`useFormContext`,
		`useController`,
	}

	for _, hook := range formHooks {
		pattern := regexp.MustCompile(`\b` + hook + `\b`)
		if pattern.MatchString(code) {
			return true, nil
		}
	}

	return false, nil
}

// checkTestRequirements checks if file meets test requirements
func checkTestRequirements(filePath string) ([]Violation, error) {
	violations := []Violation{}

	// Skip test files themselves
	if isTestFile(filePath) {
		return violations, nil
	}

	// Skip type files and barrels
	if isTypeOrBarrelFile(filePath) {
		return violations, nil
	}

	// Only check .tsx and .ts files
	if !strings.HasSuffix(filePath, ".tsx") && !strings.HasSuffix(filePath, ".ts") {
		return violations, nil
	}

	// Determine app type
	appType := getAppType(filePath)

	// Get expected test paths
	unitTestPath := getUnitTestPath(filePath)
	e2eTestPath := getE2ETestPath(filePath, appType)

	// Determine test requirements
	needsUnitTest := false
	needsE2ETest := false
	reason := ""

	if isScreen(filePath) {
		// Screens always need both unit and E2E tests
		needsUnitTest = true
		needsE2ETest = true
		reason = "Screen components"
	} else if isCRUDFolder(filePath) {
		// CRUD components need unit tests
		needsUnitTest = true
		// Create/Update (forms) need E2E tests
		if strings.Contains(filePath, "/create/") || strings.Contains(filePath, "/update/") {
			needsE2ETest = true
			reason = "Form components (create/update)"
		} else {
			reason = "CRUD components"
		}
	} else if isHookOrUtil(filePath) {
		// Hooks and utils need unit tests only
		needsUnitTest = true
		needsE2ETest = false
		reason = "Hooks and utilities"
	} else {
		// Other components - check if interactive
		interactive, err := isInteractiveComponent(filePath)
		if err != nil {
			// If we can't determine, skip validation
			return violations, nil
		}

		if interactive {
			needsUnitTest = true
			needsE2ETest = true
			reason = "Interactive components"
		} else {
			// Display-only components just need unit tests
			needsUnitTest = true
			needsE2ETest = false
			reason = "Display components"
		}
	}

	// Validate unit test exists
	if needsUnitTest && unitTestPath != "" {
		if _, err := os.Stat(unitTestPath); os.IsNotExist(err) {
			violations = append(violations, Violation{
				Severity:     "error",
				Message:      fmt.Sprintf("Missing unit test: %s", filepath.Base(unitTestPath)),
				Reason:       reason,
				ExpectedPath: unitTestPath,
			})
		}
	}

	// Validate E2E test exists
	if needsE2ETest && e2eTestPath != "" {
		if _, err := os.Stat(e2eTestPath); os.IsNotExist(err) {
			violations = append(violations, Violation{
				Severity:     "error",
				Message:      fmt.Sprintf("Missing E2E test: %s", filepath.Base(e2eTestPath)),
				Reason:       reason,
				ExpectedPath: e2eTestPath,
				AppType:      appType,
			})
		}
	}

	return violations, nil
}

// isComponentWriteOperation checks if operation creates/modifies a component file
func isComponentWriteOperation(data HookData) (bool, string) {
	// Only check Write and Edit operations
	if data.ToolName != "Write" && data.ToolName != "Edit" {
		return false, ""
	}

	filePath := data.ToolInput.FilePath

	// Only check TypeScript/TSX files in components/
	if strings.Contains(filePath, "/components/") {
		if strings.HasSuffix(filePath, ".tsx") || strings.HasSuffix(filePath, ".ts") {
			// Skip if it's already a test file
			if !isTestFile(filePath) {
				return true, filePath
			}
		}
	}

	return false, ""
}

// checkDisabled checks if the hook is disabled via environment variable
func checkDisabled() bool {
	return os.Getenv("CLAUDE_HOOKS_AST_VALIDATION") == "false"
}

func main() {
	// Check for disable flag
	if checkDisabled() {
		os.Exit(0)
	}

	// Read JSON from stdin
	var data HookData
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&data); err != nil {
		// Allow if we can't parse
		os.Exit(0)
	}

	// Only validate on component write operations
	isComponentOp, filePath := isComponentWriteOperation(data)
	if !isComponentOp {
		os.Exit(0)
	}

	// Check test requirements
	violations, err := checkTestRequirements(filePath)
	if err != nil {
		// Allow if we can't validate (don't block on errors)
		os.Exit(0)
	}

	if len(violations) > 0 {
		// Filter errors
		var errors []Violation
		for _, v := range violations {
			if v.Severity == "error" {
				errors = append(errors, v)
			}
		}

		if len(errors) > 0 {
			msg := fmt.Sprintf("BLOCKED: Test file requirements not met\n\nFile: %s\n\nMissing tests:\n",
				filepath.Base(filePath))

			for _, v := range errors {
				msg += fmt.Sprintf("\n  ❌ %s", v.Message)
				msg += fmt.Sprintf("\n     Reason: %s require tests", v.Reason)
				msg += fmt.Sprintf("\n     Expected: %s", v.ExpectedPath)
				if v.AppType != "" {
					msg += fmt.Sprintf("\n     App type: %s", v.AppType)
				}
				msg += "\n"
			}

			msg += `
Test requirements:
  - Screens: Unit test (.test.tsx) + E2E test
  - Forms (create/update): Unit test + E2E test
  - Hooks/Utils: Unit test only
  - Interactive components: Unit test + E2E test
  - Display components: Unit test only

E2E test types:
  - mobile/native: .maestro.yaml
  - web/portal: .e2e.ts

To fix:
1. Create the missing test files
2. Or set CLAUDE_HOOKS_AST_VALIDATION=false to disable

See: ~/.claude/hooks/validate-test-files.py
See: packages/backend/docs/AST_INFRASTRUCTURE_AND_CODE_QUALITY.md
`
			fmt.Fprintln(os.Stderr, msg)
			os.Exit(2)
		}
	}

	os.Exit(0)
}
